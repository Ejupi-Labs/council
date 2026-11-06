package store

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"runtime"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) ClaimAttempt(ctx context.Context, id, ownerID string) (domain.Attempt, error) {
	var out domain.Attempt
	e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		a, e := s.attempt(ctx, tx, id)
		if e != nil {
			return e
		}
		out = a
		g, e := s.gate(ctx, tx)
		if e != nil {
			return e
		}
		if e = checkGate(g); e != nil {
			return e
		}
		if a.OwnerInstanceID != ownerID || (g.OwnerInstanceID != "" && g.OwnerInstanceID != ownerID) {
			return domain.Fail("dispatch_fenced", "This attempt belongs to another owner epoch. Inspect its recovery state.")
		}
		if a.Phase != "queued" || a.Stop != nil {
			return domain.Fail("attempt_active", "This attempt is not eligible for a new dispatch claim.")
		}
		t, e := s.task(ctx, tx, a.TaskID)
		if e != nil {
			return e
		}
		if t.CurrentVersion != a.TaskVersion {
			out, e = s.settle(ctx, tx, a, domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete", Failure: &domain.PrimaryFailure{Code: "guidance_changed", Message: "Guidance changed before dispatch. Start an explicit attempt against the current version."}})
			return e
		}
		if expired(a) {
			out, e = s.settle(ctx, tx, a, domain.Settlement{Outcome: "failed", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete", Failure: &domain.PrimaryFailure{Code: "deadline_exceeded", Message: "The captured deadline expired before dispatch."}})
			return e
		}
		if e = s.workflowAttemptControls(ctx, tx, a); e != nil {
			return e
		}
		if g.ActiveAttemptID != nil {
			return domain.Fail("attempt_active", "The managed native resource already has an owned attempt. Keep this attempt queued.")
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO capacity_claims VALUES('managed-native',?)", id); e != nil {
			return storageError()
		}
		a.Phase = "preparing"
		if _, e = s.appendEvent(ctx, tx, id, "preparation_started", nil); e != nil {
			return e
		}
		if e = s.bumpState(ctx, tx, &a); e != nil {
			return e
		}
		if e = s.syncWorkflowClaim(ctx, tx, a); e != nil {
			return e
		}
		out = a
		return nil
	})
	return observe(out), e
}
func expired(a domain.Attempt) bool {
	deadline, e := time.Parse(time.RFC3339Nano, a.DeadlineAt)
	return e != nil || !time.Now().UTC().Before(deadline)
}
func (s *Store) RecordLaunchIntent(ctx context.Context, id string) (domain.Attempt, error) {
	var out domain.Attempt
	e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		a, e := s.attempt(ctx, tx, id)
		if e != nil {
			return e
		}
		g, e := s.gate(ctx, tx)
		if e != nil {
			return e
		}
		if e = checkGate(g); e != nil {
			return e
		}
		if a.Phase != "preparing" || a.Stop != nil || g.ActiveAttemptID == nil || *g.ActiveAttemptID != id {
			return domain.Fail("attempt_active", "The attempt is not eligible for native launch.")
		}
		t, e := s.task(ctx, tx, a.TaskID)
		if e != nil {
			return e
		}
		if t.CurrentVersion != a.TaskVersion {
			return domain.Fail("task_version_conflict", "Guidance changed during preparation. Do not launch this obsolete version.")
		}
		if expired(a) {
			return domain.Fail("capability_unavailable", "The execution deadline expired. Do not launch this attempt.")
		}
		if a.SourceSnapshotID == nil {
			return domain.Fail("source_capture_blocked", "A durable current-source snapshot is required before launch.")
		}
		if e = s.workflowAttemptControls(ctx, tx, a); e != nil {
			return e
		}
		if a.ExecutorKind == "" || a.ExecutorKind == "native" {
			if _, _, e = s.checkRoutingControls(ctx, tx, a, nil); e != nil {
				return e
			}
		}
		if a.LaunchIntent {
			out = a
			return nil
		}
		budget, e := s.taskBudget(ctx, tx, a.TaskID)
		if e != nil || budget == nil {
			if e != nil {
				return e
			}
			return storageError()
		}
		if e = appendBudgetLedger(ctx, tx, *budget, "launch_intent", a.StartRequestID, &a.ID, "0", "0"); e != nil {
			return e
		}
		a.LaunchIntent = true
		a.Cleanup.Scope = "unix_process_group"
		if runtime.GOOS == "windows" {
			a.Cleanup.Scope = "windows_job"
		}
		if _, e = s.appendEvent(ctx, tx, id, "launch_intent", nil); e != nil {
			return e
		}
		if e = s.bumpState(ctx, tx, &a); e != nil {
			return e
		}
		out = a
		return nil
	})
	return observe(out), e
}
func (s *Store) RecordAttempt(ctx context.Context, id string, u domain.AttemptUpdate) (domain.Attempt, error) {
	var out domain.Attempt
	e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		a, e := s.attempt(ctx, tx, id)
		if e != nil {
			return e
		}
		if a.Phase == "settled" {
			return domain.Fail("attempt_active", "This attempt is already settled. Its terminal outcome is immutable.")
		}
		if u.Phase != "" {
			if !(u.Phase == "preparing" || u.Phase == "running" || u.Phase == "cancelling") {
				return domain.Invalid("attempt observation phase")
			}
			if u.Phase == "running" && (!a.LaunchIntent || a.Stop != nil) {
				return domain.Fail("attempt_active", "This attempt cannot enter running after Stop or without launch intent.")
			}
			a.Phase = u.Phase
		}
		metadata := u.Metadata
		if u.Failure != nil {
			metadata = jsontext.Value(encode(struct {
				Observation jsontext.Value         `json:"observation,omitempty"`
				Failure     *domain.PrimaryFailure `json:"failure"`
			}{metadata, u.Failure}))
		}
		seq, e := s.appendEvent(ctx, tx, id, u.Kind, metadata)
		if e != nil {
			return e
		}
		if u.StartedAt != nil {
			if a.StartedAt == nil && a.RoutingDecisionID != nil {
				b, e := s.taskBudget(ctx, tx, a.TaskID)
				if e != nil {
					return e
				}
				if b == nil {
					return storageError()
				}
				if e = appendBudgetLedger(ctx, tx, *b, "launch_started", a.StartRequestID, &a.ID, "0", "0"); e != nil {
					return e
				}
			}
			a.StartedAt = u.StartedAt
		}
		if u.SourceSnapshotID != nil {
			var owner string
			if e := tx.QueryRowContext(ctx, "SELECT attempt_id FROM source_snapshots WHERE id=?", *u.SourceSnapshotID).Scan(&owner); e != nil || owner != id {
				return domain.Invalid("registered source snapshot for this attempt")
			}
			if a.SourceSnapshotID != nil && *a.SourceSnapshotID != *u.SourceSnapshotID {
				return domain.Invalid("immutable source snapshot")
			}
			a.SourceSnapshotID = u.SourceSnapshotID
		}
		if u.CandidateID != nil {
			candidate, e := s.candidate(ctx, tx, *u.CandidateID)
			if e != nil || candidate.AttemptID != id {
				return domain.Invalid("registered candidate for this attempt")
			}
			if a.CandidateID != nil && *a.CandidateID != *u.CandidateID {
				return domain.Invalid("immutable candidate")
			}
			a.AuthoredDelta = candidate.HasDelta || candidate.HasIndexDelta
			a.CandidateID = u.CandidateID
		}
		if u.ResponseArtifactID != nil {
			artifact, e := s.artifact(ctx, tx, *u.ResponseArtifactID)
			validKind := artifact.Kind == "final_response" || (artifact.Kind == "review_result" && a.Role == "review" && a.WorkflowID != "")
			if e != nil || artifact.AttemptID != id || !validKind || !artifact.Closed || artifact.Completeness != "complete" {
				return domain.Invalid("complete registered response for this attempt")
			}
			if a.ResponseArtifactID != nil && *a.ResponseArtifactID != *u.ResponseArtifactID {
				return domain.Invalid("immutable final response")
			}
			a.ResponseArtifactID = u.ResponseArtifactID
		}
		if u.TargetObservations != nil {
			observations, err := s.checkedObservations(ctx, tx, a, *u.TargetObservations)
			if err != nil {
				return err
			}
			a.TargetObservations = observations
		}
		if u.EvidenceStatus != "" {
			switch u.EvidenceStatus {
			case "collecting", "complete", "incomplete", "uncertain":
			default:
				return domain.Invalid("evidence status")
			}
			a.EvidenceStatus = u.EvidenceStatus
		}
		if u.Cleanup != nil {
			switch u.Cleanup.State {
			case "not_started", "active", "verified", "unknown":
			default:
				return domain.Invalid("cleanup state")
			}
			switch u.Cleanup.Scope {
			case "not_launched", "windows_job", "unix_process_group":
			default:
				return domain.Invalid("cleanup scope")
			}
			if a.LaunchIntent && u.Cleanup.Scope == "not_launched" {
				return domain.Invalid("launched cleanup scope")
			}
			a.Cleanup = *u.Cleanup
		}
		if u.Failure != nil {
			if e := validateFailure(u.Failure); e != nil {
				return e
			}
		}
		if u.Failure != nil && a.PrimaryFailure == nil && a.Stop == nil {
			failure := *u.Failure
			failure.EventSequence = seq
			failure.ObservedAt = domain.Now()
			a.PrimaryFailure = &failure
		}
		if e = s.bumpState(ctx, tx, &a); e != nil {
			return e
		}
		out = a
		return nil
	})
	return observe(out), e
}
func (s *Store) QueuedAttempts(ctx context.Context, ownerID string, limit int) ([]domain.Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit < 1 || limit > 100 {
		return nil, domain.Invalid("queue limit")
	}
	rows, e := s.db.QueryContext(ctx, "SELECT data FROM attempts WHERE phase='queued' ORDER BY sequence LIMIT ?", limit)
	if e != nil {
		return nil, storageError()
	}
	defer rows.Close()
	out := []domain.Attempt{}
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			return nil, storageError()
		}
		a, e := decode[domain.Attempt](raw)
		if e != nil {
			return nil, storageError()
		}
		if a.OwnerInstanceID == ownerID {
			out = append(out, observe(a))
		}
	}
	if rows.Err() != nil {
		return nil, storageError()
	}
	return out, nil
}
func (s *Store) Attempts(ctx context.Context, taskID string, limit int, cursor string) (domain.Page[domain.Attempt], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.Attempt]{Items: []domain.Attempt{}}
	if limit < 1 || limit > 100 {
		return out, domain.Invalid("limit")
	}
	if _, e := s.task(ctx, s.db, taskID); e != nil {
		return out, e
	}
	max, after, e := pageBounds(ctx, s.db, "attempts", taskID, cursor)
	if e != nil {
		return out, e
	}
	rows, e := s.db.QueryContext(ctx, "SELECT sequence,data FROM attempts WHERE task_id=? AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", taskID, after, max, limit+1)
	if e != nil {
		return out, storageError()
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		var seq int64
		var raw string
		if rows.Scan(&seq, &raw) != nil {
			return out, storageError()
		}
		if len(out.Items) == limit {
			out.NextCursor = makeCursor("attempts", taskID, max, last)
			break
		}
		a, e := decode[domain.Attempt](raw)
		if e != nil {
			return out, storageError()
		}
		out.Items = append(out.Items, observe(a))
		last = seq
	}
	if rows.Err() != nil {
		return out, storageError()
	}
	return out, nil
}
func (s *Store) Tasks(ctx context.Context, projectID string, limit int, cursor string) (domain.Page[domain.TaskSummary], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.TaskSummary]{Items: []domain.TaskSummary{}}
	if limit < 1 || limit > 100 {
		return out, domain.Invalid("limit")
	}
	max, after, e := pageBounds(ctx, s.db, "tasks", projectID, cursor)
	if e != nil {
		return out, e
	}
	rows, e := s.db.QueryContext(ctx, "SELECT sequence,data FROM tasks WHERE sequence>? AND sequence<=? AND (?='' OR project_id=?) ORDER BY sequence LIMIT ?", after, max, projectID, projectID, limit+1)
	if e != nil {
		return out, storageError()
	}
	type item struct {
		seq int64
		t   domain.Task
	}
	items := []item{}
	for rows.Next() {
		var x item
		var raw string
		if rows.Scan(&x.seq, &raw) != nil {
			rows.Close()
			return out, storageError()
		}
		x.t, e = decode[domain.Task](raw)
		if e != nil {
			rows.Close()
			return out, storageError()
		}
		items = append(items, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, storageError()
	}
	var last int64
	for i, x := range items {
		if i == limit {
			out.NextCursor = makeCursor("tasks", projectID, max, last)
			break
		}
		v, e := s.version(ctx, s.db, x.t.ID, x.t.CurrentVersion)
		if e != nil {
			return out, e
		}
		a, e := s.latest(ctx, s.db, x.t)
		if e != nil {
			return out, e
		}
		acceptance, e := s.acceptance(ctx, s.db, x.t)
		if e != nil {
			return out, e
		}
		summary, e := s.currentTaskSummary(ctx, s.db, x.t, v, a, acceptance != nil)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, summary)
		last = x.seq
	}
	return out, nil
}

// SweepQueued settles obsolete or expired unlaunched work across the complete queue.
// Batches bound row memory while SQL selects affected rows beyond any observer page.
func (s *Store) SweepQueued(ctx context.Context, ownerID string) (int, error) {
	total := 0
	for {
		count := 0
		e := s.executionWrite(ctx, func(tx *sql.Tx) error {
			rows, e := tx.QueryContext(ctx, "SELECT a.data FROM attempts a JOIN tasks t ON t.id=a.task_id WHERE json_extract(a.data,'$.ownerInstanceId')=? AND a.phase IN ('queued','cancelling') AND json_extract(a.data,'$.launchIntent')=0 AND NOT EXISTS(SELECT 1 FROM capacity_claims c WHERE c.attempt_id=a.id) AND (a.task_version!=t.current_version OR json_extract(a.data,'$.deadlineAt')<=? OR json_extract(a.data,'$.stop') IS NOT NULL) ORDER BY a.sequence LIMIT 100", ownerID, domain.Now())
			if e != nil {
				return storageError()
			}
			pending := []domain.Attempt{}
			for rows.Next() {
				var raw string
				if rows.Scan(&raw) != nil {
					rows.Close()
					return storageError()
				}
				a, e := decode[domain.Attempt](raw)
				if e != nil {
					rows.Close()
					return storageError()
				}
				pending = append(pending, a)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return storageError()
			}
			for _, a := range pending {
				t, e := s.task(ctx, tx, a.TaskID)
				if e != nil {
					return e
				}
				settlement := domain.Settlement{Outcome: "failed", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}
				if a.Stop != nil {
					settlement.Outcome = "cancelled"
				} else if a.TaskVersion != t.CurrentVersion {
					settlement.Outcome = "blocked"
					settlement.Failure = &domain.PrimaryFailure{Code: "guidance_changed", Message: "Guidance changed before dispatch. Start an explicit attempt against the current version."}
				} else {
					settlement.Failure = &domain.PrimaryFailure{Code: "deadline_exceeded", Message: "The captured deadline expired while waiting for capacity."}
				}
				if _, e = s.settle(ctx, tx, a, settlement); e != nil {
					return e
				}
				count++
			}
			return nil
		})
		if e != nil {
			return total, e
		}
		total += count
		if count < 100 {
			return total, nil
		}
	}
}
