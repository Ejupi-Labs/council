package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
)

var noRoutingContinuation = errors.New("no fresh automatic routing authority")

func capturedProfile(d domain.RoutingDecision) *domain.TargetProfile {
	if d.SelectedTarget.ProfileID == nil {
		return nil
	}
	for _, candidate := range d.CandidateAssessments {
		if candidate.ProfileID != nil && *candidate.ProfileID == *d.SelectedTarget.ProfileID && candidate.ContextID == d.SelectedTarget.ContextID {
			return candidate.ProfileSnapshot
		}
	}
	return nil
}
func automaticReplacementPermitted(policy domain.ContinuationPolicy, previous domain.RoutingDecision, next domain.RoutingSelectionResult, now string) bool {
	if previous.SelectedTarget.ContextID == next.SelectedTarget.ContextID && previous.SelectedTarget.ModelSelector == next.SelectedTarget.ModelSelector {
		return true
	}
	if !policy.AllowReplacement {
		return false
	}
	if policy.AllowEscalation {
		return true
	}
	// Compare against immutable admission evidence. A user editing today's old
	// profile cannot retrospectively make a replacement appear less expensive.
	oldProfile := capturedProfile(previous)
	var newProfile *domain.TargetProfile
	for _, candidate := range next.Candidates {
		if next.SelectedTarget.ProfileID != nil && candidate.ProfileID != nil && *candidate.ProfileID == *next.SelectedTarget.ProfileID && candidate.ContextID == next.SelectedTarget.ContextID {
			newProfile = candidate.ProfileSnapshot
			break
		}
	}
	if oldProfile == nil || newProfile == nil {
		return false
	}
	oldTier, newTier := routing.Tier(oldProfile.Role), routing.Tier(newProfile.Role)
	if oldTier == nil || newTier == nil || *newTier > *oldTier {
		return false
	}
	relation, comparable := routing.ComparePrice(newProfile.ReferencePrice, oldProfile.ReferencePrice, now)
	return comparable && relation <= 0
}

// ContinueRouting consumes only a freshly emitted qualifying proof. Callers must
// not scan old terminals or retry uncertain commits. The SQL proof, revision and
// unique predecessor guards still prevent a duplicated callback from new work.
func (s *Store) ContinueRouting(ctx context.Context, ownerID, previousID string, facts domain.RoutingFacts) (*domain.Admission, error) {
	var out *domain.Admission
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		g, e := s.gate(ctx, tx)
		if e != nil {
			return e
		}
		if checkGate(g) != nil || g.OwnerInstanceID != "" && g.OwnerInstanceID != ownerID {
			return noRoutingContinuation
		}
		previous, e := s.attempt(ctx, tx, previousID)
		if e != nil {
			return e
		}
		if previous.OwnerInstanceID != ownerID || previous.Phase != "settled" || previous.Outcome == nil || *previous.Outcome != "blocked" || previous.Cleanup.State != "verified" || previous.Cleanup.Scope != "not_launched" || previous.LaunchIntent || previous.Stop != nil || previous.EvidenceStatus == "uncertain" || previous.RoutingDecisionID == nil {
			return noRoutingContinuation
		}
		var proofRaw string
		e = tx.QueryRowContext(ctx, "SELECT data FROM routing_continuation_proofs WHERE attempt_id=?", previousID).Scan(&proofRaw)
		if errors.Is(e, sql.ErrNoRows) {
			return noRoutingContinuation
		}
		if e != nil {
			return storageError()
		}
		proof, e := decode[domain.ContinuationProof](proofRaw)
		if e != nil {
			return storageError()
		}
		if !proof.Eligible || proof.Category != "pre_launch_ineligible" || proof.OwnerInstanceID != ownerID || proof.PrepareAttempted || proof.CreateAttempted || proof.ExpectedOutputMissing || proof.AttemptID != previousID || proof.TaskID != previous.TaskID || proof.TaskVersion != previous.TaskVersion {
			return noRoutingContinuation
		}
		t, e := s.task(ctx, tx, previous.TaskID)
		if e != nil {
			return e
		}
		b, e := s.taskBudget(ctx, tx, t.ID)
		if e != nil {
			return e
		}
		if b == nil || t.Revision != proof.TaskRevision || t.CurrentVersion != proof.TaskVersion || b.Revision != proof.BudgetRevision || t.LatestAttemptID == nil || *t.LatestAttemptID != previousID || b.RemainingAdmissions == "0" {
			return noRoutingContinuation
		}
		if b.CurrentWindow.DeadlineAt == nil {
			return storageError()
		}
		deadline, validationErr := domain.ParseRoutingTime(*b.CurrentWindow.DeadlineAt)
		if validationErr != nil {
			return storageError()
		}
		if !time.Now().UTC().Before(deadline) {
			return noRoutingContinuation
		}
		authority, e := s.routingAuthority(ctx, tx, t.ID, t.CurrentVersion)
		if e != nil {
			return e
		}
		if authority == nil {
			return noRoutingContinuation
		}
		policy, validationErr := effectiveRoutingPolicy(authority.PolicySnapshot, authority.OptionSnapshot)
		if validationErr != nil {
			return validationErr
		}
		allowedCategory := false
		for _, category := range policy.Continuation.Categories {
			if category == proof.Category {
				allowedCategory = true
			}
		}
		if !allowedCategory || countInt(b.AutomaticAdmissions) >= countInt(policy.Continuation.MaxAutomaticAdmissions) {
			return noRoutingContinuation
		}
		var successors int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM routing_decisions WHERE previous_attempt_id=? AND kind='automatic'", previousID).Scan(&successors); e != nil {
			return storageError()
		}
		if successors != 0 {
			return noRoutingContinuation
		}
		v, e := s.version(ctx, tx, t.ID, t.CurrentVersion)
		if e != nil {
			return e
		}
		oldDecision, e := s.routingDecision(ctx, tx, *previous.RoutingDecisionID)
		if e != nil {
			return e
		}
		snapshot, e := s.routingSnapshot(ctx, tx)
		if e != nil {
			return e
		}
		snapshot.Policy = policy
		now := domain.Now()
		selection, selectionErr := routing.Select(domain.RoutingSelectionInput{Snapshot: snapshot, Pins: v.Pins, Options: authority.OptionSnapshot, Assessment: authority.SuitabilityAssessment, Facts: facts.Contexts, Now: now})
		if selectionErr != nil {
			return noRoutingContinuation
		}
		if !automaticReplacementPermitted(policy.Continuation, oldDecision, selection, now) {
			return noRoutingContinuation
		}
		requestID := domain.NewID()
		a := newAttempt(t, v, requestID, ownerID, previous.ExecutionMode, previous.FakeScenario)
		decision, e := s.admitRouting(ctx, tx, t, v, &a, *authority, b, facts, "automatic")
		if e != nil {
			return e
		}
		if e := checkResolvedScenario(a, a.ExecutionMode, a.FakeScenario); e != nil {
			return noRoutingContinuation
		}
		t.LatestAttemptID = &a.ID
		t.Revision, e = increment(t.Revision)
		if e != nil {
			return e
		}
		t.StateRevision, e = increment(t.StateRevision)
		if e != nil {
			return e
		}
		t.UpdatedAt = domain.Now()
		if e = s.insertAttempt(ctx, tx, a); e != nil {
			return e
		}
		if e = storeRoutingDecision(ctx, tx, decision); e != nil {
			return e
		}
		if e = saveTask(ctx, tx, t); e != nil {
			return e
		}
		admission := domain.Admission{Task: taskSummary(t, v, &a, false), Version: v, Attempt: observe(a), RoutingDecision: &decision, Budget: b}
		receipt := domain.Receipt{RequestID: requestID, Operation: "tasks.attempts.start", CommittedAt: domain.Now()}
		result := domain.Success(201, requestID, admission, &receipt)
		fingerprint := Fingerprint("tasks.attempts.start", struct{ PreviousAttemptID, OwnerInstanceID, TaskRevision, BudgetRevision string }{previousID, ownerID, proof.TaskRevision, proof.BudgetRevision})
		if _, e = tx.ExecContext(ctx, "INSERT INTO receipts VALUES(?,?,?,?,?)", requestID, receipt.Operation, fingerprint, 201, encode(result.Envelope)); e != nil {
			return storageError()
		}
		out = &admission
		return nil
	})
	if errors.Is(err, noRoutingContinuation) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}
