package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func notFound() *domain.Error {
	return domain.Fail("not_found", "The retained task, attempt or evidence does not exist. Reload its list.")
}
func (s *Store) executionWrite(ctx context.Context, fn func(*sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return beforeCommitError(ctx, storageError())
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return beforeCommitError(ctx, err)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if s.BeforeCommit != nil {
		if err = s.BeforeCommit(); err != nil {
			return beforeCommitHookError(ctx, err)
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// Once Commit is attempted, cancellation cannot prove that it did not commit.
	if err = tx.Commit(); err != nil {
		return domain.Fail("write_uncertain", "Execution persistence is uncertain. Stop conflicting dispatch and inspect retained state.")
	}
	if s.AfterCommit != nil {
		if err = s.AfterCommit(); err != nil {
			return domain.Fail("write_uncertain", "Execution persistence is uncertain. Stop conflicting dispatch and inspect retained state.")
		}
	}
	return nil
}
func beforeCommitError(ctx context.Context, err error) error {
	if interrupted := ctx.Err(); interrupted != nil {
		// SQL callbacks normalize canceled statements to storageError. That plain
		// wrapper is not proof of a separate disk fault. Explicit joined durability
		// markers and write uncertainty carry stronger evidence and must survive.
		code := durabilityErrorCode(err)
		if code == "write_uncertain" || hasJoinedDurabilityFault(err) {
			return errors.Join(interrupted, err)
		}
		return interrupted
	}
	return err
}
func beforeCommitHookError(ctx context.Context, err error) error {
	if durabilityErrorCode(err) == "write_uncertain" {
		return beforeCommitError(ctx, err)
	}
	if cancellationOnly(err) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		return context.DeadlineExceeded
	}
	// A non-cancellation error from the explicit persistence hook is a known
	// fault source, unlike an ambiguous canceled SQL callback wrapper.
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), storageError())
	}
	return storageError()
}
func durabilityErrorCode(err error) string {
	if err == nil {
		return ""
	}
	code := ""
	if de, ok := err.(*domain.Error); ok && (de.Code == "storage_failed" || de.Code == "write_uncertain") {
		code = de.Code
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			child := durabilityErrorCode(cause)
			if child == "write_uncertain" {
				return child
			}
			if child != "" {
				code = child
			}
		}
	} else if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		child := durabilityErrorCode(wrapped.Unwrap())
		if child != "" {
			code = child
		}
	}
	return code
}
func hasJoinedDurabilityFault(err error) bool {
	var joined interface {
		error
		Unwrap() []error
	}
	return errors.As(err, &joined) && durabilityErrorCode(joined) != ""
}
func cancellationOnly(err error) bool {
	if err == context.Canceled || err == context.DeadlineExceeded {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !cancellationOnly(cause) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return cancellationOnly(wrapped.Unwrap())
	}
	return false
}
func (s *Store) task(ctx context.Context, q queryer, id string) (domain.Task, error) {
	var raw string
	var rev, state, version int64
	e := q.QueryRowContext(ctx, "SELECT revision,state_revision,current_version,data FROM tasks WHERE id=?", id).Scan(&rev, &state, &version, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.Task{}, notFound()
	}
	if e != nil {
		return domain.Task{}, storageError()
	}
	t, e := decode[domain.Task](raw)
	if e != nil || !validStoredRevision(rev, t.Revision) || !validStoredRevision(state, t.StateRevision) || !validStoredRevision(version, t.CurrentVersion) {
		return domain.Task{}, storageError()
	}
	return t, nil
}
func (s *Store) version(ctx context.Context, q queryer, id, version string) (domain.TaskVersion, error) {
	n, e := domain.ParseRevision(version)
	if e != nil {
		return domain.TaskVersion{}, e
	}
	var raw string
	e2 := q.QueryRowContext(ctx, "SELECT data FROM task_versions WHERE task_id=? AND version=?", id, n).Scan(&raw)
	if errors.Is(e2, sql.ErrNoRows) {
		return domain.TaskVersion{}, notFound()
	}
	if e2 != nil {
		return domain.TaskVersion{}, storageError()
	}
	v, e2 := decode[domain.TaskVersion](raw)
	if e2 != nil || v.Version != version || v.TaskID != id {
		return domain.TaskVersion{}, storageError()
	}
	return v, nil
}
func saveTask(ctx context.Context, tx *sql.Tx, t domain.Task) error {
	rev, e := domain.ParseRevision(t.Revision)
	if e != nil {
		return storageError()
	}
	state, e := domain.ParseRevision(t.StateRevision)
	if e != nil {
		return storageError()
	}
	version, e := domain.ParseRevision(t.CurrentVersion)
	if e != nil {
		return storageError()
	}
	if _, e2 := tx.ExecContext(ctx, "UPDATE tasks SET revision=?,state_revision=?,current_version=?,data=? WHERE id=?", rev, state, version, encode(t), t.ID); e2 != nil {
		return storageError()
	}
	return nil
}
func (s *Store) attempt(ctx context.Context, q queryer, id string) (domain.Attempt, error) {
	var raw, phase string
	var rev int64
	e := q.QueryRowContext(ctx, "SELECT revision,phase,data FROM attempts WHERE id=?", id).Scan(&rev, &phase, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.Attempt{}, notFound()
	}
	if e != nil {
		return domain.Attempt{}, storageError()
	}
	a, e := decode[domain.Attempt](raw)
	if e != nil || a.ID != id || !validStoredRevision(rev, a.Revision) || a.Phase != phase {
		return domain.Attempt{}, storageError()
	}
	return observe(a), nil
}
func observe(a domain.Attempt) domain.Attempt {
	now := time.Now().UTC()
	a.ObservedAt = now.Format("2006-01-02T15:04:05.000Z")
	deadline, e := time.Parse(time.RFC3339Nano, a.DeadlineAt)
	a.RemainingSeconds = 0
	if e == nil && deadline.After(now) {
		a.RemainingSeconds = int((deadline.Sub(now) + time.Second - 1) / time.Second)
	}
	return a
}
func saveAttempt(ctx context.Context, tx *sql.Tx, a domain.Attempt) error {
	rev, e := domain.ParseRevision(a.Revision)
	if e != nil {
		return storageError()
	}
	if _, e2 := tx.ExecContext(ctx, "UPDATE attempts SET revision=?,phase=?,data=? WHERE id=?", rev, a.Phase, encode(a), a.ID); e2 != nil {
		return storageError()
	}
	return nil
}
func (s *Store) bumpState(ctx context.Context, tx *sql.Tx, a *domain.Attempt) error {
	revision, e := increment(a.Revision)
	if e != nil {
		return e
	}
	a.Revision = revision
	t, e := s.task(ctx, tx, a.TaskID)
	if e != nil {
		return e
	}
	t.StateRevision, e = increment(t.StateRevision)
	if e != nil {
		return e
	}
	if e = saveTask(ctx, tx, t); e != nil {
		return e
	}
	return saveAttempt(ctx, tx, *a)
}
func (s *Store) gate(ctx context.Context, q queryer) (domain.ExecutionGate, error) {
	var g domain.ExecutionGate
	var incident sql.NullString
	e := q.QueryRowContext(ctx, "SELECT state,owner_instance_id,incident_id FROM execution_control WHERE singleton=1").Scan(&g.State, &g.OwnerInstanceID, &incident)
	if e != nil {
		return g, storageError()
	}
	if incident.Valid {
		g.IncidentID = &incident.String
	}
	var active string
	e = q.QueryRowContext(ctx, "SELECT attempt_id FROM capacity_claims WHERE resource_id='managed-native'").Scan(&active)
	if e == nil {
		g.ActiveAttemptID = &active
	} else if !errors.Is(e, sql.ErrNoRows) {
		return g, storageError()
	}
	return g, nil
}
func checkGate(g domain.ExecutionGate) error {
	if g.State == "closing" {
		return domain.Fail("owner_closing", "The owner is closing admission. Wait for settlement and open a new owner explicitly.")
	}
	if g.State != "open" {
		return domain.Fail("dispatch_fenced", "Dispatch is fenced by retained uncertainty. Inspect the incident before starting more work.")
	}
	return nil
}
func (s *Store) Gate(ctx context.Context) (domain.ExecutionGate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gate(ctx, s.db)
}
func ptr(s string) *string { return &s }
func resolvedVersion(taskID string, policy domain.Policy, in domain.StartTaskInput) domain.TaskVersion {
	acceptance, _ := domain.ResolveAcceptance(in.Acceptance)
	v := domain.TaskVersion{TaskID: taskID, Version: "1", CreatedAt: domain.Now(), CreatedBy: "user", Acceptance: acceptance, RequestedPolicy: policy, TimeoutSeconds: 1800}
	if in.TimeoutSeconds != nil {
		v.TimeoutSeconds = *in.TimeoutSeconds
	}
	if in.Pins != nil {
		v.Pins = *in.Pins
	}
	if in.PolicyOverride != "" {
		v.TaskPolicyOverride = ptr(in.PolicyOverride)
	}
	return v
}
func newAttempt(t domain.Task, v domain.TaskVersion, requestID, ownerID, mode, scenario string) domain.Attempt {
	if mode == "" {
		mode = "native"
	}
	if mode == "fake" && scenario == "" {
		scenario = "response"
	}
	now := time.Now().UTC()
	connectionID, _ := domain.ResolveConnectionID(v.Pins) // Authoring capability was checked before admission.
	unknown := domain.ObservedFact{Status: "unknown"}
	return domain.Attempt{ID: domain.NewID(), TaskID: t.ID, TaskVersion: v.Version, PreviousAttemptID: t.LatestAttemptID, StartRequestID: requestID, OwnerInstanceID: ownerID, ConnectionID: connectionID, ExecutionMode: mode, FakeScenario: scenario, Phase: "queued", Revision: "1", AdmittedAt: now.Format("2006-01-02T15:04:05.000Z"), DeadlineAt: now.Add(time.Duration(v.TimeoutSeconds) * time.Second).Format("2006-01-02T15:04:05.000Z"), RequestedTarget: v.Pins, TargetObservations: domain.TargetObservations{Provider: unknown, Account: unknown, Model: unknown, NativeSession: unknown, Permissions: domain.PermissionObservation{RequestedMode: v.RequestedPolicy.Mode, Limitations: []string{}}, Usage: domain.UsageObservation{Completeness: "unknown"}}, Cleanup: domain.Cleanup{State: "not_started", Scope: "not_launched"}, EvidenceStatus: "collecting"}
}
func (s *Store) insertAttempt(ctx context.Context, tx *sql.Tx, a domain.Attempt) error {
	seq, e := nextSequence(ctx, tx, "attempts")
	if e != nil {
		return e
	}
	version, _ := domain.ParseRevision(a.TaskVersion)
	if _, e = tx.ExecContext(ctx, "INSERT INTO attempts(sequence,id,task_id,task_version,revision,phase,data) VALUES(?,?,?,?,1,?,?)", seq, a.ID, a.TaskID, version, a.Phase, encode(a)); e != nil {
		return storageError()
	}
	_, e = s.appendEvent(ctx, tx, a.ID, "admitted", nil)
	return e
}
func taskSummary(t domain.Task, v domain.TaskVersion, a *domain.Attempt, accepted bool) domain.TaskSummary {
	out := domain.TaskSummary{ID: t.ID, ProjectID: t.ProjectID, CurrentVersion: t.CurrentVersion, Revision: t.Revision, StateRevision: t.StateRevision, LatestAttemptID: t.LatestAttemptID, RequestedMode: v.RequestedPolicy.Mode, PolicySource: v.RequestedPolicy.SelectionSource, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, Status: "awaiting_execution", Stage: "awaiting_execution", NextAction: "Start an explicit execution against the current task version."}
	if v.TaskPolicyOverride != nil {
		out.RequestedMode = *v.TaskPolicyOverride
	}
	if a == nil {
		return out
	}
	if a.Cleanup.State == "unknown" {
		out.Status = "blocked"
		out.Stage = "cleanup_unresolved"
		out.NextAction = "Inspect the retained cleanup incident. Dispatch remains fenced."
		return out
	}
	if a.TaskVersion != t.CurrentVersion {
		return out
	}
	out.Stage = a.Phase
	out.Status = a.Phase
	out.NextAction = "Observe this attempt or stop it explicitly."
	if a.Outcome != nil {
		out.Status = *a.Outcome
		out.NextAction = "Inspect the outcome and retained evidence."
		if *a.Outcome == "succeeded" {
			out.Status = "verification_pending"
			out.NextAction = "Inspect the candidate. Verification and integration remain pending."
			if v.Acceptance.Deliverable == "response_only" && len(v.Acceptance.RequiredEvidence) == 0 && !a.AuthoredDelta {
				out.Status = "response_acceptance_pending"
				out.NextAction = "Inspect the exact response and optionally record user acceptance."
			}
			if accepted {
				out.Status = "completed"
				out.NextAction = "The exact response and conditions have a retained user-review acceptance."
			}
		}
	}
	return out
}
func (s *Store) latest(ctx context.Context, q queryer, t domain.Task) (*domain.Attempt, error) {
	if t.LatestAttemptID == nil {
		return nil, nil
	}
	a, e := s.attempt(ctx, q, *t.LatestAttemptID)
	return &a, e
}
func (s *Store) acceptance(ctx context.Context, q queryer, t domain.Task) (*domain.ResponseAcceptance, error) {
	if t.LatestAttemptID == nil {
		return nil, nil
	}
	var raw string
	e := q.QueryRowContext(ctx, "SELECT data FROM response_acceptances WHERE attempt_id=?", *t.LatestAttemptID).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, storageError()
	}
	a, e := decode[domain.ResponseAcceptance](raw)
	if e != nil {
		return nil, storageError()
	}
	if a.TaskVersion != t.CurrentVersion {
		return nil, nil
	}
	return &a, nil
}
func (s *Store) Task(ctx context.Context, id string) (domain.TaskDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out domain.TaskDetail
	var e error
	out.Task, e = s.task(ctx, s.db, id)
	if e != nil {
		return out, e
	}
	out.Version, e = s.version(ctx, s.db, id, out.Task.CurrentVersion)
	if e != nil {
		return out, e
	}
	out.LatestAttempt, e = s.latest(ctx, s.db, out.Task)
	if e != nil {
		return out, e
	}
	out.Acceptance, e = s.acceptance(ctx, s.db, out.Task)
	return out, e
}
func (s *Store) TaskVersion(ctx context.Context, id, version string) (domain.TaskVersionDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, e := s.task(ctx, s.db, id)
	if e != nil {
		return domain.TaskVersionDetail{}, e
	}
	v, e := s.version(ctx, s.db, id, version)
	return domain.TaskVersionDetail{TaskID: id, OriginalText: t.OriginalText, Version: v}, e
}
func (s *Store) Attempt(ctx context.Context, id string) (domain.Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempt(ctx, s.db, id)
}
func intRevision(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }

func validateFailure(in *domain.PrimaryFailure) error {
	if len(in.Code) == 0 || len(in.Code) > 64 || len(in.Message) == 0 || len(in.Message) > 512 || !utf8.ValidString(in.Message) {
		return domain.Invalid("bounded primary failure")
	}
	for _, c := range in.Code {
		if !(c >= 'a' && c <= 'z' || c == '_' || c >= '0' && c <= '9') {
			return domain.Invalid("primary failure code")
		}
	}
	return nil
}
