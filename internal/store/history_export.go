package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/historyout"
)

const HistoryPreviewOperation = "history.exports.preview"
const HistoryCreateOperation = "history.exports.create"
const HistoryReconcileOperation = "history.operations.reconcile"

func HistoryPreviewFingerprint(in domain.PreviewHistoryExportInput) string {
	return Fingerprint(HistoryPreviewOperation, in)
}
func HistoryCreateFingerprint(in domain.ExecuteHistoryExportInput) string {
	return Fingerprint(HistoryCreateOperation, in)
}
func HistoryReconcileFingerprint(id string, in domain.ReconcileHistoryOperationInput) string {
	return Fingerprint(HistoryReconcileOperation, struct {
		OperationID string                                `json:"operationId"`
		Input       domain.ReconcileHistoryOperationInput `json:"input"`
	}{id, in})
}

// The native preview is controller-private and is never part of an API receipt.
type historyStoredPreview struct {
	Public domain.HistoryExportPreview `json:"public"`
	Native historyout.Preview          `json:"native"`
}

func historyCurrentOwner(ctx context.Context, q queryer, ownerID string) (recoveryOwnerEpoch, error) {
	epoch, err := readRecoveryOwnerEpoch(ctx, q)
	if err != nil {
		return epoch, err
	}
	if !domain.ValidID(ownerID) || epoch.State != "dirty" || epoch.OwnerID != ownerID {
		return epoch, domain.Fail("owner_unavailable", "Contact the current Council owner before changing history operations.")
	}
	var current string
	if err = q.QueryRowContext(ctx, "SELECT owner_instance_id FROM execution_control WHERE singleton=1").Scan(&current); err != nil {
		return epoch, storageError()
	}
	if current != ownerID {
		return epoch, domain.Fail("owner_unavailable", "Contact the current Council owner before changing history operations.")
	}
	return epoch, nil
}

func historyReadPreview(ctx context.Context, q queryer, id string) (historyStoredPreview, error) {
	var out historyStoredPreview
	var snapshotID, raw string
	var revision int64
	err := q.QueryRowContext(ctx, "SELECT snapshot_id,revision,data FROM history_previews WHERE id=?", id).Scan(&snapshotID, &revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, notFound()
	}
	if err != nil || len(raw) > 16384 || json.Unmarshal([]byte(raw), &out, json.RejectUnknownMembers(true)) != nil || revision != 1 || out.Public.PreviewID != id || out.Public.SnapshotID != snapshotID || domain.ValidateHistoryExportPreview(out.Public) != nil || out.Native.Name != out.Public.Filename || out.Native.Root.StateRoot == "" || out.Native.Root.ExportRoot == "" {
		return historyStoredPreview{}, storageError()
	}
	return out, nil
}

// The durable, controller-captured native root identity and ASCII filename
// define a destination. A possibly effect-capable older operation keeps that
// exact destination fenced even if the output file is currently absent.
func historyRequireFreeDestination(ctx context.Context, q queryer, native historyout.Preview, excludeOperation string) error {
	var conflicts int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM history_export_operations o JOIN history_previews p ON p.id=o.preview_id AND p.snapshot_id=o.snapshot_id
		WHERE o.state IN ('prepared','effect_started','uncertain') AND o.id!=?
		AND json_extract(p.data,'$.native.root.stateRoot')=?
		AND json_extract(p.data,'$.native.root.exportRoot')=?
		AND json_extract(p.data,'$.native.name')=? COLLATE NOCASE LIMIT 1
	)`, excludeOperation, native.Root.StateRoot, native.Root.ExportRoot, native.Name).Scan(&conflicts)
	if err != nil {
		return storageError()
	}
	if conflicts != 0 {
		return domain.Fail("effect_uncertain", "An unresolved history operation retains this destination. Inspect and reconcile it before creating another output there.")
	}
	return nil
}

// PreviewHistoryExport only records a previously verified native preview. The
// caller must first replay the request, then verify the held root and target.
func (s *Store) PreviewHistoryExport(ctx context.Context, ownerID string, in domain.PreviewHistoryExportInput, destinationDir string, native historyout.Preview) domain.Result {
	if err := domain.ValidatePreviewHistoryExportInput(in); err != nil {
		return domain.Failure(in.RequestID, err)
	}
	return s.mutate(ctx, in.RequestID, HistoryPreviewOperation, HistoryPreviewFingerprint(in), func(tx *sql.Tx) (int, any, error) {
		if _, err := historyCurrentOwner(ctx, tx, ownerID); err != nil {
			return 0, nil, err
		}
		if native.Name != in.Filename || native.Root.StateRoot == "" || native.Root.ExportRoot == "" || len(native.Root.StateRoot) > 4096 || len(native.Root.ExportRoot) > 4096 || !filepath.IsAbs(destinationDir) || filepath.Clean(destinationDir) != destinationDir {
			return 0, nil, domain.Invalid("verified history destination")
		}
		if err := historyRequireFreeDestination(ctx, tx, native, ""); err != nil {
			return 0, nil, err
		}
		var raw string
		var size int64
		if err := tx.QueryRowContext(ctx, "SELECT length(CAST(data AS BLOB)) FROM history_snapshots WHERE id=? AND revision=1", in.SnapshotID).Scan(&size); errors.Is(err, sql.ErrNoRows) {
			return 0, nil, notFound()
		} else if err != nil || size < 1 || size > domain.HistoryMaxPayloadBytes {
			return 0, nil, storageError()
		}
		if err := tx.QueryRowContext(ctx, "SELECT data FROM history_snapshots WHERE id=?", in.SnapshotID).Scan(&raw); err != nil || int64(len(raw)) != size {
			return 0, nil, storageError()
		}
		var snapshot domain.HistorySnapshot
		if json.Unmarshal([]byte(raw), &snapshot, json.RejectUnknownMembers(true)) != nil || domain.ValidateHistorySnapshot(snapshot) != nil || snapshot.Summary.SnapshotID != in.SnapshotID {
			return 0, nil, storageError()
		}
		p := domain.HistoryExportPreview{PreviewID: domain.NewID(), Revision: "1", SnapshotID: in.SnapshotID, SnapshotRevision: "1", Filename: in.Filename, DestinationKind: "private_history_exports", DestinationDirectory: destinationDir, DestinationPath: filepath.Join(destinationDir, in.Filename), Completeness: domain.HistoryCompleteness, IncludedCategories: snapshot.Summary.IncludedCategories, OmittedCategories: snapshot.Summary.OmittedCategories, AdditionalOmissions: snapshot.Summary.AdditionalOmissions, TargetState: "absent", CreatedAt: domain.Now()}
		if domain.ValidateHistoryExportPreview(p) != nil {
			return 0, nil, storageError()
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO history_previews(id,snapshot_id,revision,data) VALUES(?,?,1,?)", p.PreviewID, p.SnapshotID, encode(historyStoredPreview{Public: p, Native: native})); err != nil {
			return 0, nil, storageError()
		}
		return 201, struct {
			Preview domain.HistoryExportPreview `json:"preview"`
		}{p}, nil
	})
}

func (s *Store) HistoryExportPreview(ctx context.Context, id string) (domain.HistoryExportPreview, historyout.Preview, error) {
	if !domain.ValidID(id) {
		return domain.HistoryExportPreview{}, historyout.Preview{}, domain.Invalid("history preview identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := historyReadPreview(ctx, s.db, id)
	return p.Public, p.Native, err
}

func (s *Store) CreateHistoryExport(ctx context.Context, ownerID string, in domain.ExecuteHistoryExportInput) domain.Result {
	if err := domain.ValidateExecuteHistoryExportInput(in); err != nil {
		return domain.Failure(in.RequestID, err)
	}
	return s.mutate(ctx, in.RequestID, HistoryCreateOperation, HistoryCreateFingerprint(in), func(tx *sql.Tx) (int, any, error) {
		epoch, err := historyCurrentOwner(ctx, tx, ownerID)
		if err != nil {
			return 0, nil, err
		}
		p, err := historyReadPreview(ctx, tx, in.PreviewID)
		if err != nil {
			return 0, nil, err
		}
		if err := historyRequireFreeDestination(ctx, tx, p.Native, ""); err != nil {
			return 0, nil, err
		}
		o := domain.HistoryExportOperation{OperationID: domain.NewID(), RequestID: in.RequestID, Revision: "1", SnapshotID: p.Public.SnapshotID, PreviewID: p.Public.PreviewID, State: "prepared", Completeness: domain.HistoryCompleteness, Blockers: []string{}, NextAction: "wait", Filename: p.Public.Filename, DestinationDirectory: p.Public.DestinationDirectory, DestinationPath: p.Public.DestinationPath}
		if domain.ValidateHistoryExportOperation(o) != nil {
			return 0, nil, storageError()
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO history_export_operations(id,request_id,preview_id,snapshot_id,owner_instance_id,owner_generation,revision,state,frozen,journal_revision,data) VALUES(?,?,?,?,?,?,1,'prepared',0,0,?)`, o.OperationID, in.RequestID, o.PreviewID, o.SnapshotID, ownerID, epoch.Generation, encode(o))
		if err != nil {
			return 0, nil, storageError()
		}
		return 202, struct {
			Operation domain.HistoryExportOperation `json:"operation"`
		}{o}, nil
	})
}

type historyOperationRecord struct {
	Operation                             domain.HistoryExportOperation
	Owner                                 string
	Generation, Revision, JournalRevision int64
	Frozen                                bool
}

func historyReadOperation(ctx context.Context, q queryer, id string) (historyOperationRecord, error) {
	var rec historyOperationRecord
	var requestID, previewID, snapshotID, state, raw string
	var frozen int
	err := q.QueryRowContext(ctx, "SELECT request_id,preview_id,snapshot_id,owner_instance_id,owner_generation,revision,state,frozen,journal_revision,data FROM history_export_operations WHERE id=?", id).Scan(&requestID, &previewID, &snapshotID, &rec.Owner, &rec.Generation, &rec.Revision, &state, &frozen, &rec.JournalRevision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return rec, notFound()
	}
	if err != nil || len(raw) > 16384 || json.Unmarshal([]byte(raw), &rec.Operation, json.RejectUnknownMembers(true)) != nil || !domain.ValidID(rec.Owner) || rec.Generation < 1 || rec.Revision < 1 || rec.JournalRevision < 0 || (frozen != 0 && frozen != 1) || rec.Operation.OperationID != id || rec.Operation.RequestID != requestID || rec.Operation.PreviewID != previewID || rec.Operation.SnapshotID != snapshotID || rec.Operation.State != state || rec.Operation.Revision != strconv.FormatInt(rec.Revision, 10) || domain.ValidateHistoryExportOperation(rec.Operation) != nil {
		return historyOperationRecord{}, storageError()
	}
	rec.Frozen = frozen == 1
	return rec, nil
}
func (s *Store) HistoryExportOperation(ctx context.Context, id string) (domain.HistoryExportOperation, error) {
	if !domain.ValidID(id) {
		return domain.HistoryExportOperation{}, domain.Invalid("history operation identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := historyReadOperation(ctx, s.db, id)
	return rec.Operation, err
}

// MarkHistoryExportPrePermitFailure records that the native publisher returned
// before obtaining a permit. Reconciliation must still establish drain and
// owner-history proof before claiming no publication.
func (s *Store) MarkHistoryExportPrePermitFailure(ctx context.Context, ownerID, operationID, reason string) error {
	if !domain.ValidID(operationID) {
		return domain.Invalid("history operation identity")
	}
	switch reason {
	case "target_conflict", "root_unavailable", "request_interrupted":
	default:
		return domain.Invalid("history pre-publication reason")
	}
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		epoch, err := historyCurrentOwner(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		rec, err := historyReadOperation(ctx, tx, operationID)
		if err != nil {
			return err
		}
		permits, err := historyCheckJournal(ctx, tx, rec)
		if err != nil {
			return err
		}
		if rec.Operation.State != "prepared" || rec.Frozen || permits != 0 || rec.Generation > epoch.Generation || (rec.Owner == ownerID && rec.Generation != epoch.Generation) || (rec.Owner != ownerID && rec.Generation >= epoch.Generation) {
			return domain.Fail("effect_uncertain", "The pre-publication state cannot be verified. Inspect the recorded operation.")
		}
		if rec.Owner != ownerID {
			var begins int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM recovery_owner_epoch_events WHERE generation=? AND owner_instance_id=? AND kind='begin'", rec.Generation, rec.Owner).Scan(&begins); err != nil || begins != 1 {
				return domain.Fail("effect_uncertain", "The original history operation owner cannot be verified.")
			}
		}
		rec.Frozen = true
		rec.Operation.Blockers = []string{reason}
		rec.Operation.NextAction = "reconcile"
		return historySaveOperation(ctx, tx, &rec)
	})
}

func historyCheckJournal(ctx context.Context, q queryer, rec historyOperationRecord) (int64, error) {
	var count, low, high, permits int64
	err := q.QueryRowContext(ctx, "SELECT count(*),coalesce(min(sequence),0),coalesce(max(sequence),0),coalesce(sum(CASE WHEN kind='permit' THEN 1 ELSE 0 END),0) FROM history_effect_journal WHERE operation_id=?", rec.Operation.OperationID).Scan(&count, &low, &high, &permits)
	if err != nil || count != rec.JournalRevision || count > 0 && (low != 1 || high != count) || permits > 1 {
		return 0, storageError()
	}
	return permits, nil
}

func historyAppendJournal(ctx context.Context, tx *sql.Tx, rec *historyOperationRecord, id, kind string, data any) error {
	if !domain.ValidID(id) || rec.JournalRevision >= 4096 {
		return storageError()
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO history_effect_journal(operation_id,sequence,id,kind,data) VALUES(?,?,?,?,?)", rec.Operation.OperationID, rec.JournalRevision+1, id, kind, encode(data)); err != nil {
		return storageError()
	}
	rec.JournalRevision++
	return nil
}
func historySaveOperation(ctx context.Context, tx *sql.Tx, rec *historyOperationRecord) error {
	rec.Revision++
	rec.Operation.Revision = strconv.FormatInt(rec.Revision, 10)
	if domain.ValidateHistoryExportOperation(rec.Operation) != nil {
		return storageError()
	}
	frozen := 0
	if rec.Frozen {
		frozen = 1
	}
	r, err := tx.ExecContext(ctx, "UPDATE history_export_operations SET revision=?,state=?,frozen=?,journal_revision=?,data=? WHERE id=? AND revision=?", rec.Revision, rec.Operation.State, frozen, rec.JournalRevision, encode(rec.Operation), rec.Operation.OperationID, rec.Revision-1)
	if err != nil {
		return storageError()
	}
	n, err := r.RowsAffected()
	if err != nil || n != 1 {
		return storageError()
	}
	return nil
}

// PermitHistoryExport is deliberately non-replayable: an ambiguous commit or
// acknowledgement never authorizes a filesystem call or a second permit.
func (s *Store) PermitHistoryExport(ctx context.Context, ownerID, operationID, expectedRevision string) (domain.HistoryPublicationEffect, error) {
	var effect domain.HistoryPublicationEffect
	if !domain.ValidID(operationID) {
		return effect, domain.Invalid("history operation identity")
	}
	if _, err := domain.ParseRevision(expectedRevision); err != nil {
		return effect, domain.Invalid("history operation revision")
	}
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		epoch, err := historyCurrentOwner(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		gate, err := s.gate(ctx, tx)
		if err != nil {
			return err
		}
		if err = checkGate(gate); err != nil {
			return err
		}
		rec, err := historyReadOperation(ctx, tx, operationID)
		if err != nil {
			return err
		}
		permits, err := historyCheckJournal(ctx, tx, rec)
		if err != nil {
			return err
		}
		if rec.Owner != ownerID || rec.Generation != epoch.Generation || rec.Operation.Revision != expectedRevision || rec.Frozen || rec.Operation.State != "prepared" || permits != 0 {
			return domain.Fail("effect_uncertain", "The publication authority is no longer available. Inspect the recorded operation.")
		}
		p, err := historyReadPreview(ctx, tx, rec.Operation.PreviewID)
		if err != nil {
			return err
		}
		if p.Public.SnapshotID != rec.Operation.SnapshotID || p.Public.Filename != rec.Operation.Filename {
			return storageError()
		}
		if err := historyRequireFreeDestination(ctx, tx, p.Native, operationID); err != nil {
			return err
		}
		effect = domain.HistoryPublicationEffect{EffectID: domain.NewID(), OperationID: operationID, OwnerInstanceID: ownerID, OwnerGeneration: strconv.FormatInt(epoch.Generation, 10), Filename: rec.Operation.Filename}
		if err = historyAppendJournal(ctx, tx, &rec, effect.EffectID, "permit", effect); err != nil {
			return err
		}
		rec.Operation.State = "effect_started"
		rec.Operation.NextAction = "inspect"
		return historySaveOperation(ctx, tx, &rec)
	})
	if err != nil {
		return domain.HistoryPublicationEffect{}, err
	}
	return effect, nil
}

func historyExactPermit(ctx context.Context, q queryer, rec historyOperationRecord, effect domain.HistoryPublicationEffect) error {
	if !domain.ValidID(effect.EffectID) || effect.OperationID != rec.Operation.OperationID || effect.OwnerInstanceID != rec.Owner || effect.OwnerGeneration != strconv.FormatInt(rec.Generation, 10) || effect.Filename != rec.Operation.Filename {
		return domain.Fail("effect_uncertain", "The exact publication authority is unavailable.")
	}
	var raw string
	if err := q.QueryRowContext(ctx, "SELECT data FROM history_effect_journal WHERE operation_id=? AND id=? AND kind='permit'", effect.OperationID, effect.EffectID).Scan(&raw); err != nil {
		return domain.Fail("effect_uncertain", "The exact publication authority is unavailable.")
	}
	var saved domain.HistoryPublicationEffect
	if len(raw) > 16384 || json.Unmarshal([]byte(raw), &saved, json.RejectUnknownMembers(true)) != nil || saved != effect {
		return storageError()
	}
	return nil
}

// FinishHistoryExport records only bounded native observations; an error or an
// incomplete/contradictory observation after a permit remains uncertain.
func (s *Store) FinishHistoryExport(ctx context.Context, ownerID string, effect domain.HistoryPublicationEffect, result historyout.Result, expectedBytes int, publishErr error) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		epoch, err := historyCurrentOwner(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		rec, err := historyReadOperation(ctx, tx, effect.OperationID)
		if err != nil {
			return err
		}
		permits, err := historyCheckJournal(ctx, tx, rec)
		if err != nil {
			return err
		}
		if rec.Owner != ownerID || rec.Generation != epoch.Generation || rec.Operation.State != "effect_started" || permits != 1 {
			return domain.Fail("effect_uncertain", "The publication observation cannot be attributed to the current owner.")
		}
		if err = historyExactPermit(ctx, tx, rec, effect); err != nil {
			return err
		}
		if expectedBytes < 1 || expectedBytes > domain.HistoryMaxPayloadBytes || result.BytesWritten < 0 || result.BytesWritten > expectedBytes || len(result.FileIdentity) > 4096 {
			return domain.Invalid("history publication observation")
		}
		observation := struct {
			EffectID      string            `json:"effectId"`
			Result        historyout.Result `json:"result"`
			ExpectedBytes int               `json:"expectedBytes"`
			Error         bool              `json:"error"`
			At            string            `json:"at"`
		}{effect.EffectID, result, expectedBytes, publishErr != nil, domain.Now()}
		if err = historyAppendJournal(ctx, tx, &rec, domain.NewID(), "observation", observation); err != nil {
			return err
		}
		if publishErr == nil && result.PermitAccepted && result.CreateAttempted && result.Created && result.BytesWritten == expectedBytes && result.FileFlushed && result.NameVerified && result.FileIdentity != "" && (result.DirectorySync == "synced" || result.DirectorySync == "unavailable") {
			state := "published"
			sync := "succeeded"
			if result.DirectorySync == "unavailable" {
				sync = "unavailable"
			}
			rec.Operation.State = state
			rec.Operation.Outcome = &state
			rec.Operation.Durability = &domain.HistoryDurability{FileFlush: "succeeded", DirectorySync: sync, PowerLossNamespaceGuarantee: false}
			rec.Operation.Blockers = []string{}
			rec.Operation.NextAction = "none"
		} else {
			state := "uncertain"
			rec.Operation.State = state
			rec.Operation.Outcome = &state
			rec.Operation.Durability = nil
			rec.Operation.Blockers = []string{"history_effect_uncertain"}
			rec.Operation.NextAction = "inspect"
		}
		rec.Frozen = true
		return historySaveOperation(ctx, tx, &rec)
	})
}
