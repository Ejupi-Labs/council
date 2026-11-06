package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) CreateTask(ctx context.Context, ownerID string, in domain.StartTaskInput) domain.Result {
	return s.CreateTaskWithRouting(ctx, ownerID, in, wrapperRoutingFacts(in.ExecutionMode))
}
func (s *Store) CreateTaskWithRouting(ctx context.Context, ownerID string, in domain.StartTaskInput, facts domain.RoutingFacts) domain.Result {
	if e := domain.ValidateStartTask(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.mutate(ctx, in.RequestID, "tasks.start", TaskStartFingerprint(in), func(tx *sql.Tx) (int, any, error) {
		g, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if e = checkGate(g); e != nil {
			return 0, nil, e
		}
		if g.OwnerInstanceID != "" && g.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "The owner epoch changed. Contact the current owner before starting work.")
		}
		var exists int
		e = tx.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE id=?", in.ProjectID).Scan(&exists)
		if errors.Is(e, sql.ErrNoRows) {
			return 0, nil, notFound()
		}
		if e != nil {
			return 0, nil, storageError()
		}
		var policy domain.Policy
		if in.SourceDraftRef != nil {
			d, e := s.draft(ctx, tx, in.SourceDraftRef.DraftID, in.SourceDraftRef.Revision)
			if e != nil {
				return 0, nil, e
			}
			if d.ProjectID != in.ProjectID || d.OriginalText != in.OriginalText {
				return 0, nil, domain.Fail("source_draft_conflict", "The exact saved draft does not match this project and original text. Reconcile the draft before starting.")
			}
			policy = d.RequestedPolicy
		} else {
			policy, e = s.policy(ctx, tx)
			if e != nil {
				return 0, nil, e
			}
			if in.ExpectedPolicyRevision != "" && in.ExpectedPolicyRevision != policy.Revision {
				return 0, nil, domain.RevisionConflict(policy.Revision)
			}
		}
		now := domain.Now()
		t := domain.Task{ID: domain.NewID(), ProjectID: in.ProjectID, OriginalText: in.OriginalText, SourceDraftRef: in.SourceDraftRef, CurrentVersion: "1", Revision: "1", StateRevision: "1", CreatedAt: now, UpdatedAt: now}
		v := resolvedVersion(t.ID, policy, in)
		if e := domain.CheckAuthoringCapability(v); e != nil {
			return 0, nil, e
		}
		seq, e := nextSequence(ctx, tx, "tasks")
		if e != nil {
			return 0, nil, e
		}
		a := newAttempt(t, v, in.RequestID, ownerID, in.ExecutionMode, in.FakeScenario)
		t.LatestAttemptID = ptr(a.ID)
		if _, e = tx.ExecContext(ctx, "INSERT INTO tasks(sequence,id,project_id,revision,state_revision,current_version,data) VALUES(?,?,?,1,1,1,?)", seq, t.ID, t.ProjectID, encode(t)); e != nil {
			return 0, nil, storageError()
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO task_versions VALUES(?,1,?)", t.ID, encode(v)); e != nil {
			return 0, nil, storageError()
		}
		authority, e := s.captureRouting(ctx, tx, t, v, in.Routing, facts, nil, "user")
		if e != nil {
			return 0, nil, e
		}
		budget, e := s.initialBudget(ctx, tx, authority, in.RequestID)
		if e != nil {
			return 0, nil, e
		}
		decision, e := s.admitRouting(ctx, tx, t, v, &a, authority, &budget, facts, "manual")
		if e != nil {
			return 0, nil, e
		}
		if e := checkResolvedScenario(a, in.ExecutionMode, in.FakeScenario); e != nil {
			return 0, nil, e
		}
		if e = s.insertAttempt(ctx, tx, a); e != nil {
			return 0, nil, e
		}
		if e = storeRoutingDecision(ctx, tx, decision); e != nil {
			return 0, nil, e
		}
		return 201, domain.Admission{Task: taskSummary(t, v, &a, false), Version: v, Attempt: observe(a), RoutingDecision: &decision, Budget: &budget}, nil
	})
}
func (s *Store) UpdateGuidance(ctx context.Context, taskID string, in domain.GuidanceInput) domain.Result {
	return s.UpdateGuidanceWithRouting(ctx, taskID, in, wrapperRoutingFacts(""))
}
func (s *Store) UpdateGuidanceWithRouting(ctx context.Context, taskID string, in domain.GuidanceInput, facts domain.RoutingFacts) domain.Result {
	if !domain.ValidID(taskID) {
		return domain.Failure(in.RequestID, domain.Invalid("task identity"))
	}
	if e := domain.ValidateGuidance(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	fp := GuidanceFingerprint(taskID, in)
	return s.mutate(ctx, in.RequestID, "tasks.guidance", fp, func(tx *sql.Tx) (int, any, error) {
		t, e := s.task(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		if t.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(t.Revision)
		}
		v, e := s.version(ctx, tx, taskID, t.CurrentVersion)
		if e != nil {
			return 0, nil, e
		}
		priorAuthority, e := s.routingAuthority(ctx, tx, taskID, v.Version)
		if e != nil {
			return 0, nil, e
		}
		if priorAuthority == nil && in.Routing != nil {
			return 0, nil, domain.Fail("routing_adoption_required", "Adopt an explicit task budget before adding routing authority to legacy work.")
		}
		v.Version, e = increment(v.Version)
		if e != nil {
			return 0, nil, e
		}
		t.Revision, e = increment(t.Revision)
		if e != nil {
			return 0, nil, e
		}
		t.StateRevision, e = increment(t.StateRevision)
		if e != nil {
			return 0, nil, e
		}
		t.CurrentVersion = v.Version
		now := domain.Now()
		t.UpdatedAt = now
		v.CreatedAt = now
		v.GuidanceText = in.GuidanceText
		if in.UseCurrentPolicy {
			v.RequestedPolicy, e = s.policy(ctx, tx)
			if e != nil {
				return 0, nil, e
			}
		}
		if in.ClearPolicyOverride {
			v.TaskPolicyOverride = nil
		} else if in.PolicyOverride != "" {
			v.TaskPolicyOverride = ptr(in.PolicyOverride)
		}
		if in.Pins != nil {
			v.Pins = *in.Pins
		}
		if in.Acceptance != nil {
			v.Acceptance, _ = domain.ResolveAcceptance(in.Acceptance)
		}
		if in.TimeoutSeconds != nil {
			v.TimeoutSeconds = *in.TimeoutSeconds
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO task_versions VALUES(?,?,?)", taskID, intRevision(v.Version), encode(v)); e != nil {
			return 0, nil, storageError()
		}
		if priorAuthority != nil {
			if _, e = s.captureRouting(ctx, tx, t, v, in.Routing, facts, priorAuthority, "user"); e != nil {
				return 0, nil, e
			}
		}
		if e = saveTask(ctx, tx, t); e != nil {
			return 0, nil, e
		}
		a, e := s.latest(ctx, tx, t)
		if e != nil {
			return 0, nil, e
		}
		return 200, domain.GuidanceResult{Task: taskSummary(t, v, a, false), Version: v}, nil
	})
}
func (s *Store) StartAttempt(ctx context.Context, ownerID, taskID string, in domain.StartAttemptInput) domain.Result {
	return s.StartAttemptWithRouting(ctx, ownerID, taskID, in, wrapperRoutingFacts(in.ExecutionMode))
}
func (s *Store) StartAttemptWithRouting(ctx context.Context, ownerID, taskID string, in domain.StartAttemptInput, facts domain.RoutingFacts) domain.Result {
	if e := domain.ValidateStartAttempt(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	fp := AttemptStartFingerprint(taskID, in)
	return s.mutate(ctx, in.RequestID, "tasks.attempts.start", fp, func(tx *sql.Tx) (int, any, error) {
		g, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if e = checkGate(g); e != nil {
			return 0, nil, e
		}
		if g.OwnerInstanceID != "" && g.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "The owner epoch changed. Contact the current owner before starting work.")
		}
		t, e := s.task(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		if t.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(t.Revision)
		}
		if t.CurrentVersion != in.TaskVersion {
			return 0, nil, domain.Fail("task_version_conflict", "Start against the current immutable task version.")
		}
		old, e := s.latest(ctx, tx, t)
		if e != nil {
			return 0, nil, e
		}
		if old != nil && (old.Phase != "settled" || old.Cleanup.State != "verified") {
			return 0, nil, domain.Fail("attempt_active", "This task still has unsettled ownership. Inspect or stop its existing attempt.")
		}
		v, e := s.version(ctx, tx, taskID, t.CurrentVersion)
		if e != nil {
			return 0, nil, e
		}
		if e := domain.CheckAuthoringCapability(v); e != nil {
			return 0, nil, e
		}
		authority, e := s.routingAuthority(ctx, tx, t.ID, v.Version)
		if e != nil {
			return 0, nil, e
		}
		budget, e := s.taskBudget(ctx, tx, t.ID)
		if e != nil {
			return 0, nil, e
		}
		if authority == nil || budget == nil {
			return 0, nil, domain.Fail("routing_adoption_required", "This legacy task needs explicit budget adoption before a new managed attempt.")
		}
		a := newAttempt(t, v, in.RequestID, ownerID, in.ExecutionMode, in.FakeScenario)
		decision, e := s.admitRouting(ctx, tx, t, v, &a, *authority, budget, facts, "manual")
		if e != nil {
			return 0, nil, e
		}
		if e := checkResolvedScenario(a, in.ExecutionMode, in.FakeScenario); e != nil {
			return 0, nil, e
		}
		t.LatestAttemptID = ptr(a.ID)
		t.Revision, e = increment(t.Revision)
		if e != nil {
			return 0, nil, e
		}
		t.StateRevision, e = increment(t.StateRevision)
		if e != nil {
			return 0, nil, e
		}
		t.UpdatedAt = domain.Now()
		if e = s.insertAttempt(ctx, tx, a); e != nil {
			return 0, nil, e
		}
		if e = saveTask(ctx, tx, t); e != nil {
			return 0, nil, e
		}
		if e = storeRoutingDecision(ctx, tx, decision); e != nil {
			return 0, nil, e
		}
		return 201, domain.Admission{Task: taskSummary(t, v, &a, false), Version: v, Attempt: observe(a), RoutingDecision: &decision, Budget: budget}, nil
	})
}
