package store

import (
	"context"
	"database/sql"
	"reflect"
	"slices"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const maxRecoveryCheckpointAttempts = 4096
const maxRecoveryActiveCandidates = domain.MaxGraphNodes + 1

func recoveryTrackActiveCheckpoint(seen map[string]bool, cp domain.RecoveryCheckpoint) error {
	if !domain.ValidID(cp.ID) || domain.ValidateRecoveryScope(cp.Scope) != nil || cp.State != "pausing" && cp.State != "paused" {
		return storageError()
	}
	key := cp.Scope.Kind + "/" + cp.Scope.TaskID + "/" + cp.Scope.GraphID + "/" + cp.Scope.NodeID
	if seen[key] {
		return domain.Fail("checkpoint_scope_overflow", "Duplicate active pause records affect this scope; inspect recovery state before continuing.")
	}
	seen[key] = true
	return nil
}

// recoveryCheckpointSnapshot must run in the transaction accepting the pause:
// its captured identities and the admission fence share one durable boundary.
func (s *Store) recoveryCheckpointSnapshot(ctx context.Context, tx *sql.Tx, scope domain.RecoveryScope, ownerID string) (domain.RecoveryCheckpoint, error) {
	now := domain.Now()
	cp := domain.RecoveryCheckpoint{
		ID: domain.NewID(), Scope: scope, Revision: "1", OwnerInstanceID: ownerID,
		CreatedAt: now, UpdatedAt: now, AdmittedAttemptIDs: []string{},
		CompletedAttemptIDs: []string{}, UnfinishedTaskIDs: []string{}, Blockers: []string{},
	}
	var query string
	var args []any
	switch scope.Kind {
	case "task":
		query = "SELECT id FROM attempts WHERE task_id=? ORDER BY sequence"
		args = []any{scope.TaskID}
	case "member":
		// A graph input operation belongs to the root task but is admitted
		// specifically for this member and must drain with its scope.
		query = "SELECT a.id FROM attempts a WHERE a.task_id=? OR EXISTS (SELECT 1 FROM graph_inputs i WHERE i.operation_attempt_id=a.id AND i.graph_id=? AND i.node_id=?) ORDER BY a.sequence"
		args = []any{scope.TaskID, scope.GraphID, scope.NodeID}
	case "graph":
		query = "SELECT a.id FROM attempts a WHERE a.task_id=(SELECT root_task_id FROM task_graphs WHERE id=?) OR EXISTS (SELECT 1 FROM graph_nodes n WHERE n.graph_id=? AND n.task_id=a.task_id) ORDER BY a.sequence"
		args = []any{scope.GraphID, scope.GraphID}
	default:
		return cp, domain.Invalid("checkpoint scope")
	}
	// One extra row detects overflow; a checkpoint must never silently omit
	// historical identities merely to fit an internal collection limit.
	query += " LIMIT 4097"
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return cp, storageError()
	}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return cp, storageError()
		}
		cp.AdmittedAttemptIDs = append(cp.AdmittedAttemptIDs, id)
		if len(cp.AdmittedAttemptIDs) > maxRecoveryCheckpointAttempts {
			rows.Close()
			return cp, domain.Fail("checkpoint_too_large", "This scope has more than 4096 retained attempts. Inspect its history; no checkpoint has been created.")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return cp, storageError()
	}
	if err = s.recoveryCheckpointProjection(ctx, tx, &cp); err != nil {
		return cp, err
	}
	return cp, nil
}

// Projection is based on persisted attempts and terminals, never process IDs or
// a caller's assertion. It does not mutate task, attempt or incident history.
func (s *Store) recoveryCheckpointProjection(ctx context.Context, tx *sql.Tx, cp *domain.RecoveryCheckpoint) error {
	cp.CompletedAttemptIDs = []string{}
	cp.UnfinishedTaskIDs = []string{}
	cp.Blockers = []string{}
	active := false
	for _, id := range cp.AdmittedAttemptIDs {
		a, err := s.attempt(ctx, tx, id)
		if err != nil {
			return err
		}
		if a.Phase != "settled" {
			active = true
			continue
		}
		cp.CompletedAttemptIDs = append(cp.CompletedAttemptIDs, id)
		var incidentCount, unresolvedIncidents int
		if err := tx.QueryRowContext(ctx, `SELECT count(*), coalesce(sum(CASE WHEN NOT EXISTS (
			SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled'
		) THEN 1 ELSE 0 END),0) FROM execution_incidents i WHERE i.attempt_id=?`, id).Scan(&incidentCount, &unresolvedIncidents); err != nil {
			return storageError()
		}
		if unresolvedIncidents != 0 {
			cp.Blockers = append(cp.Blockers, "incident_unresolved:"+id)
		}
		resolvedCleanup := incidentCount != 0 && unresolvedIncidents == 0
		if a.Cleanup.State != "verified" && !resolvedCleanup {
			cp.Blockers = append(cp.Blockers, "cleanup_unverified:"+id)
		}
		var claimCount int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims WHERE attempt_id=?", id).Scan(&claimCount); err != nil {
			return storageError()
		}
		if claimCount != 0 {
			cp.Blockers = append(cp.Blockers, "claim_retained:"+id)
		}
		uncertain, err := s.workflowEffectUncertain(ctx, tx, a)
		if err != nil {
			return err
		}
		materializationUncertain, err := s.graphMaterializationUncertain(ctx, tx, a)
		if err != nil {
			return err
		}
		membership, err := s.graphMembership(ctx, tx, a.TaskID)
		if err != nil {
			return err
		}
		var rawTerminal string
		if err := tx.QueryRowContext(ctx, "SELECT data FROM attempt_terminals WHERE attempt_id=?", id).Scan(&rawTerminal); err != nil {
			return storageError()
		}
		terminal, err := decode[struct {
			Settlement domain.Settlement `json:"settlement"`
		}](rawTerminal)
		if err != nil {
			return storageError()
		}
		knownEffects := terminal.Settlement.EffectsKnown || terminal.Settlement.NoProcessCreated || !a.LaunchIntent && a.Cleanup.Scope == "not_launched"
		knownFailed := a.Outcome != nil && (*a.Outcome == "failed" || *a.Outcome == "cancelled") && a.Cleanup.State == "verified" && knownEffects
		knownUnstarted := a.Outcome != nil && *a.Outcome == "blocked" && !a.LaunchIntent && a.Cleanup.State == "verified" && a.Cleanup.Scope == "not_launched"
		if a.EvidenceStatus == "uncertain" || a.EvidenceStatus != "complete" && !knownFailed && !knownUnstarted {
			cp.Blockers = append(cp.Blockers, "evidence_incomplete:"+id)
		}
		requiresEffectProof := a.ExecutorKind == "local_application" || a.ExecutorKind == "local_graph" || membership != nil && membership.Kind == "node"
		if uncertain || materializationUncertain || requiresEffectProof && !terminal.Settlement.EffectsKnown {
			cp.Blockers = append(cp.Blockers, "effects_uncertain:"+id)
		}
	}
	if cp.Scope.Kind == "graph" {
		rows, err := tx.QueryContext(ctx, "SELECT task_id FROM graph_nodes WHERE graph_id=? AND state!='completed' ORDER BY ordinal", cp.Scope.GraphID)
		if err != nil {
			return storageError()
		}
		for rows.Next() {
			var id string
			if rows.Scan(&id) != nil {
				rows.Close()
				return storageError()
			}
			cp.UnfinishedTaskIDs = append(cp.UnfinishedTaskIDs, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return storageError()
		}
	} else {
		// Without an accepted workflow completion, preserve the task as
		// unfinished rather than infer acceptance from a settled attempt.
		var completed int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM workflows w JOIN tasks t ON t.id=w.task_id AND t.current_version=w.task_version WHERE w.task_id=? AND w.state='completed'", cp.Scope.TaskID).Scan(&completed); err != nil {
			return storageError()
		}
		if completed == 0 {
			cp.UnfinishedTaskIDs = append(cp.UnfinishedTaskIDs, cp.Scope.TaskID)
		}
	}
	cp.State = "pausing"
	cp.NextAction = "Wait for admitted work to settle."
	if len(cp.Blockers) != 0 {
		cp.NextAction = "Inspect unresolved cleanup, evidence or effects before resuming."
	} else if !active {
		cp.State = "paused"
		cp.NextAction = "Resume explicitly when the current authority permits."
	}
	return nil
}

// Call after attempt and terminal rows and claim disposition are persisted,
// inside that same transaction. Both member and group checkpoints may match.
func (s *Store) refreshRecoveryCheckpointsForAttempt(ctx context.Context, tx *sql.Tx, attemptID string) error {
	// An attempt can affect at most one graph (16 members and one group), or
	// one standalone task. Never scan unrelated projects or silently truncate
	// a corrupted excess of active scope records.
	rows, err := tx.QueryContext(ctx, `SELECT c.data FROM recovery_checkpoints c
		WHERE c.state IN ('pausing','paused') AND (
			c.task_id=(SELECT task_id FROM attempts WHERE id=?) OR c.graph_id IN (
				SELECT id FROM task_graphs WHERE root_task_id=(SELECT task_id FROM attempts WHERE id=?)
				UNION SELECT graph_id FROM graph_nodes WHERE task_id=(SELECT task_id FROM attempts WHERE id=?)
				UNION SELECT graph_id FROM graph_inputs WHERE operation_attempt_id=?))
		ORDER BY c.id LIMIT 18`, attemptID, attemptID, attemptID, attemptID)
	if err != nil {
		return storageError()
	}
	checkpoints := []domain.RecoveryCheckpoint{}
	seen := 0
	activeScopes := map[string]bool{}
	for rows.Next() {
		seen++
		if seen > maxRecoveryActiveCandidates {
			rows.Close()
			return domain.Fail("checkpoint_scope_overflow", "Too many active pause records affect this task or graph; inspect recovery state before continuing.")
		}
		var raw string
		if rows.Scan(&raw) != nil {
			rows.Close()
			return storageError()
		}
		cp, err := decode[domain.RecoveryCheckpoint](raw)
		if err != nil {
			rows.Close()
			return storageError()
		}
		if err = recoveryTrackActiveCheckpoint(activeScopes, cp); err != nil {
			rows.Close()
			return err
		}
		if slices.Contains(cp.AdmittedAttemptIDs, attemptID) {
			checkpoints = append(checkpoints, cp)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return storageError()
	}
	for _, cp := range checkpoints {
		before := cp
		if err := s.recoveryCheckpointProjection(ctx, tx, &cp); err != nil {
			return err
		}
		if reflect.DeepEqual(before, cp) {
			continue
		}
		cp.Revision, err = increment(cp.Revision)
		if err != nil {
			return err
		}
		cp.UpdatedAt = domain.Now()
		result, err := tx.ExecContext(ctx, "UPDATE recovery_checkpoints SET revision=?,state=?,data=? WHERE id=? AND revision=?", intRevision(cp.Revision), cp.State, encode(cp), cp.ID, intRevision(before.Revision))
		if err != nil {
			return storageError()
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return storageError()
		}
	}
	return nil
}

// Checkpoint is observation only; it never refreshes state or dispatches work.
func (s *Store) Checkpoint(ctx context.Context, id string) (domain.RecoveryCheckpoint, error) {
	if !domain.ValidID(id) {
		return domain.RecoveryCheckpoint{}, domain.Invalid("checkpoint identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw string
	if err := s.db.QueryRowContext(ctx, "SELECT data FROM recovery_checkpoints WHERE id=?", id).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return domain.RecoveryCheckpoint{}, notFound()
		}
		return domain.RecoveryCheckpoint{}, storageError()
	}
	return decode[domain.RecoveryCheckpoint](raw)
}
