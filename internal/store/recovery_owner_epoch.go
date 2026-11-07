package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// recoveryOwnerEpoch records whether the previous owner durably completed a
// clean shutdown. An untracked v6 upgrade is not evidence of a clean owner.
type recoveryOwnerEpoch struct {
	OwnerID, State string
	Generation     int64
}

func readRecoveryOwnerEpoch(ctx context.Context, q queryer) (recoveryOwnerEpoch, error) {
	var value recoveryOwnerEpoch
	if err := q.QueryRowContext(ctx, "SELECT owner_instance_id,state,generation FROM recovery_owner_epoch WHERE singleton=1").Scan(&value.OwnerID, &value.State, &value.Generation); err != nil {
		return value, storageError()
	}
	switch value.State {
	case "untracked":
		if value.OwnerID != "" || value.Generation != 0 {
			return value, storageError()
		}
	case "clean":
		if value.Generation == 0 {
			if value.OwnerID != "" {
				return value, storageError()
			}
		} else if !domain.ValidID(value.OwnerID) {
			return value, storageError()
		}
	case "dirty":
		if value.Generation < 1 || !domain.ValidID(value.OwnerID) {
			return value, storageError()
		}
	default:
		return value, storageError()
	}
	var begins, closes, future int
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM recovery_owner_epoch_events WHERE kind='begin' AND generation=? AND owner_instance_id=?", value.Generation, value.OwnerID).Scan(&begins); err != nil {
		return value, storageError()
	}
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM recovery_owner_epoch_events WHERE kind='clean' AND generation=? AND owner_instance_id=?", value.Generation, value.OwnerID).Scan(&closes); err != nil {
		return value, storageError()
	}
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM recovery_owner_epoch_events WHERE generation>?", value.Generation).Scan(&future); err != nil {
		return value, storageError()
	}
	if value.Generation == 0 {
		if begins != 0 || closes != 0 || future != 0 {
			return value, storageError()
		}
	} else if begins != 1 || future != 0 || (value.State == "clean" && closes != 1) || (value.State == "dirty" && closes != 0) {
		return value, storageError()
	}
	return value, nil
}

// recoveryCleanOwnerChain proves every recorded owner epoch from the original
// attempt through the current owner's predecessor ended in a verified clean
// close. Missing, legacy, dirty or mismatched generations never qualify.
func recoveryCleanOwnerChain(ctx context.Context, q queryer, originalOwnerID, currentOwnerID string) (bool, error) {
	if !domain.ValidID(originalOwnerID) || !domain.ValidID(currentOwnerID) {
		return false, nil
	}
	epoch, err := readRecoveryOwnerEpoch(ctx, q)
	if err != nil {
		return false, err
	}
	if epoch.OwnerID != currentOwnerID || epoch.State != "dirty" {
		return false, nil
	}
	if originalOwnerID == currentOwnerID {
		return true, nil
	}
	var first int64
	if err = q.QueryRowContext(ctx, "SELECT generation FROM recovery_owner_epoch_events WHERE owner_instance_id=? AND kind='begin'", originalOwnerID).Scan(&first); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, storageError()
	}
	if first < 1 || first >= epoch.Generation {
		return false, nil
	}
	var begins, clean, matching int64
	if err = q.QueryRowContext(ctx, "SELECT count(*) FROM recovery_owner_epoch_events WHERE kind='begin' AND generation>=? AND generation<?", first, epoch.Generation).Scan(&begins); err != nil {
		return false, storageError()
	}
	if err = q.QueryRowContext(ctx, "SELECT count(*) FROM recovery_owner_epoch_events WHERE kind='clean' AND generation>=? AND generation<?", first, epoch.Generation).Scan(&clean); err != nil {
		return false, storageError()
	}
	if err = q.QueryRowContext(ctx, "SELECT count(*) FROM recovery_owner_epoch_events b JOIN recovery_owner_epoch_events c ON c.generation=b.generation AND c.owner_instance_id=b.owner_instance_id AND c.kind='clean' WHERE b.kind='begin' AND b.generation>=? AND b.generation<?", first, epoch.Generation).Scan(&matching); err != nil {
		return false, storageError()
	}
	expected := epoch.Generation - first
	return begins == expected && clean == expected && matching == expected, nil
}

// initializeFreshRecoveryEpoch applies only to a newly created, verified empty
// database in the same Open call. A migrated v6 database remains untracked.
func (s *Store) initializeFreshRecoveryEpoch(ctx context.Context) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		epoch, err := readRecoveryOwnerEpoch(ctx, tx)
		if err != nil {
			return err
		}
		if epoch.State != "untracked" {
			return storageError()
		}
		var count int
		for _, q := range []string{"SELECT count(*) FROM tasks", "SELECT count(*) FROM attempts", "SELECT count(*) FROM receipts", "SELECT count(*) FROM execution_incidents", "SELECT count(*) FROM capacity_claims"} {
			if err = tx.QueryRowContext(ctx, q).Scan(&count); err != nil || count != 0 {
				return storageError()
			}
		}
		var state, owner string
		var incident sql.NullString
		if err = tx.QueryRowContext(ctx, "SELECT state,owner_instance_id,incident_id FROM execution_control WHERE singleton=1").Scan(&state, &owner, &incident); err != nil || state != "open" || owner != "" || incident.Valid {
			return storageError()
		}
		result, err := tx.ExecContext(ctx, "UPDATE recovery_owner_epoch SET state='clean' WHERE singleton=1 AND state='untracked' AND owner_instance_id='' AND generation=0")
		if err != nil {
			return storageError()
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return storageError()
		}
		return nil
	})
}

// BeginOwnerEpoch is called while the native owner lock is held and before
// RecoverOwner or any service/worker can expose admission. A dirty/untracked
// prior epoch creates an auditable unknown incident and retains a global fence.
func (s *Store) BeginOwnerEpoch(ctx context.Context, ownerID string) error {
	if !domain.ValidID(ownerID) {
		return domain.Invalid("owner identity")
	}
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		epoch, err := readRecoveryOwnerEpoch(ctx, tx)
		if err != nil {
			return err
		}
		if epoch.State != "clean" {
			message := "The previous Council owner did not prove a clean shutdown. Inspect retained ownership and evidence; automatic recovery cannot establish the unrecorded effects."
			if epoch.State == "untracked" {
				message = "This upgraded state predates durable owner-epoch tracking. Its previous shutdown cannot be proven; inspect retained data and keep dispatch fenced."
			}
			if err = s.fence(ctx, tx, domain.ExecutionIncident{ID: domain.NewID(), Code: "owner_epoch_uncertain", Message: message, CreatedAt: domain.Now()}); err != nil {
				return err
			}
		}
		if epoch.Generation == int64(^uint64(0)>>1) {
			return domain.Fail("revision_exhausted", "The owner epoch cannot be advanced safely.")
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_owner_epoch_events(generation,owner_instance_id,kind,created_at) VALUES(?,?,'begin',?)", epoch.Generation+1, ownerID, domain.Now()); err != nil {
			return storageError()
		}
		result, err := tx.ExecContext(ctx, "UPDATE recovery_owner_epoch SET owner_instance_id=?,state='dirty',generation=? WHERE singleton=1 AND state=? AND generation=?", ownerID, epoch.Generation+1, epoch.State, epoch.Generation)
		if err != nil {
			return storageError()
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return storageError()
		}
		return nil
	})
}

// CompleteOwnerEpoch is only called after the Engine has accepted shutdown,
// stopped its scheduler and workers, closed owned handles and found no in-memory
// unrecorded fence cause. It returns false when any durable blocker remains.
func (s *Store) CompleteOwnerEpoch(ctx context.Context, ownerID string) (bool, error) {
	if !domain.ValidID(ownerID) {
		return false, domain.Invalid("owner identity")
	}
	complete := false
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		epoch, err := readRecoveryOwnerEpoch(ctx, tx)
		if err != nil {
			return err
		}
		if epoch.State != "dirty" || epoch.OwnerID != ownerID {
			return domain.Fail("owner_unavailable", "Only the current dirty owner epoch can close.")
		}
		var state, current string
		var incident sql.NullString
		if err = tx.QueryRowContext(ctx, "SELECT state,owner_instance_id,incident_id FROM execution_control WHERE singleton=1").Scan(&state, &current, &incident); err != nil {
			return storageError()
		}
		if current != ownerID {
			return domain.Fail("owner_unavailable", "The owner epoch no longer owns the durable gate.")
		}
		if state != "closing" && state != "fenced" {
			return nil
		}
		var count int
		for _, q := range []string{
			"SELECT count(*) FROM attempts WHERE phase!='settled'",
			"SELECT count(*) FROM delivery_operations WHERE active_effect_id IS NOT NULL OR status IN ('preparing','applying','uncertain')",
			"SELECT count(*) FROM graph_inputs WHERE state IN ('preparing','uncertain')",
		} {
			if err = tx.QueryRowContext(ctx, q).Scan(&count); err != nil {
				return storageError()
			}
			if count != 0 {
				return nil
			}
		}
		var unresolved, claims int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM execution_incidents i WHERE NOT EXISTS (SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled')").Scan(&unresolved); err != nil {
			return storageError()
		}
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims").Scan(&claims); err != nil {
			return storageError()
		}
		if state == "closing" {
			if incident.Valid || unresolved != 0 || claims != 0 {
				return nil
			}
		} else if unresolved == 0 {
			// A known resolved fence remains fenced until a later explicit
			// owner recheck, even though this epoch can close cleanly.
			if !incident.Valid || claims != 0 {
				return nil
			}
			var resolved int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM recovery_decisions WHERE incident_id=? AND json_extract(data,'$.outcome')='reconciled'", incident.String).Scan(&resolved); err != nil {
				return storageError()
			}
			if resolved != 1 {
				return nil
			}
		} else {
			// A complete owner epoch may retain recorded unresolved blockers,
			// but the shared gate must remain fenced across the handoff.
			if !incident.Valid || unresolved == 0 {
				return nil
			}
			var pointed, unmatched int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM execution_incidents i WHERE i.id=? AND NOT EXISTS (SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled')", incident.String).Scan(&pointed); err != nil {
				return storageError()
			}
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims c WHERE NOT EXISTS (SELECT 1 FROM execution_incidents i WHERE i.attempt_id=c.attempt_id AND NOT EXISTS (SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled'))").Scan(&unmatched); err != nil {
				return storageError()
			}
			if pointed != 1 || unmatched != 0 {
				return nil
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_owner_epoch_events(generation,owner_instance_id,kind,created_at) VALUES(?,?,'clean',?)", epoch.Generation, ownerID, domain.Now()); err != nil {
			return storageError()
		}
		result, err := tx.ExecContext(ctx, "UPDATE recovery_owner_epoch SET state='clean' WHERE singleton=1 AND owner_instance_id=? AND state='dirty' AND generation=?", ownerID, epoch.Generation)
		if err != nil {
			return storageError()
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return storageError()
		}
		complete = true
		return nil
	})
	return complete, err
}
