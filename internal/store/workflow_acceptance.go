package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"reflect"
	"slices"
)

func (s *Store) AcceptWorkflow(ctx context.Context, taskID string, f domain.WorkflowAcceptanceFacts) domain.Result {
	operation := "tasks.workflow.advance"
	fingerprint := WorkflowAdvanceFingerprint(taskID, domain.WorkflowAdvanceInput{RequestID: f.RequestID, ExpectedRevision: f.ExpectedTaskRevision, WorkflowRevision: f.ExpectedWorkflowRevision, TaskVersion: f.Acceptance.TaskVersion})
	if f.AutomaticPredecessorStepID != "" {
		operation = "tasks.workflow.accept"
		fingerprint = Fingerprint(operation, struct{ TaskID, Version, TaskRevision, WorkflowRevision, PredecessorStepID string }{taskID, f.Acceptance.TaskVersion, f.ExpectedTaskRevision, f.ExpectedWorkflowRevision, f.AutomaticPredecessorStepID})
	}
	return s.mutate(ctx, f.RequestID, operation, fingerprint, func(tx *sql.Tx) (int, any, error) {
		in := f.Acceptance
		t, e := s.task(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		if t.Revision != f.ExpectedTaskRevision {
			return 0, nil, domain.RevisionConflict(t.Revision)
		}
		w, e := s.workflowByID(ctx, tx, in.WorkflowID)
		if e != nil {
			return 0, nil, e
		}
		if w.TaskID != taskID || w.TaskVersion != t.CurrentVersion || w.Revision != f.ExpectedWorkflowRevision || w.State == "completed" || w.State == "cancelled" || w.State == "cancelling" || w.SelectedCandidateID == nil || *w.SelectedCandidateID != in.CandidateBinding.CandidateID {
			return 0, nil, domain.Fail("acceptance_unavailable", "Only the current eligible selected candidate can be accepted.")
		}
		g, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if e = checkGate(g); e != nil {
			return 0, nil, e
		}
		if !domain.ValidID(f.OwnerInstanceID) || g.OwnerInstanceID != "" && g.OwnerInstanceID != f.OwnerInstanceID {
			return 0, nil, domain.Fail("owner_unavailable", "The current owner must acknowledge verified delivery.")
		}
		a, e := s.latest(ctx, tx, t)
		if e != nil {
			return 0, nil, e
		}
		if a == nil || a.Phase != "settled" || a.Cleanup.State != "verified" || a.Stop != nil || a.Outcome == nil || *a.Outcome != "succeeded" || a.EvidenceStatus != "complete" {
			return 0, nil, domain.Fail("acceptance_unavailable", "The current supporting execution is not completely settled.")
		}
		if f.AutomaticPredecessorStepID != "" {
			step, e := s.workflowStep(ctx, tx, f.AutomaticPredecessorStepID)
			if e != nil {
				return 0, nil, e
			}
			p := step.AdvanceProof
			key, invalid := domain.WorkflowStageKey("accept", w.CorrectionCyclesUsed, "", *w.SelectedCandidateID)
			if invalid != nil {
				return 0, nil, invalid
			}
			b, e := s.taskBudget(ctx, tx, t.ID)
			if e != nil {
				return 0, nil, e
			}
			if p == nil || w.CurrentStepID == nil || *w.CurrentStepID != step.ID || step.AttemptID == nil || *step.AttemptID != a.ID || p.OwnerInstanceID != f.OwnerInstanceID || p.TaskRevision != t.Revision || p.TaskVersion != t.CurrentVersion || p.WorkflowRevision != w.Revision || p.NextLogicalKey != key || b == nil || p.BudgetRevision != b.Revision || p.WindowNumber != b.CurrentWindow.Number || p.CandidateID == nil || *p.CandidateID != *w.SelectedCandidateID {
				return 0, nil, domain.Fail("workflow_revision_conflict", "The fresh automatic acceptance proof was suppressed, consumed or superseded.")
			}
		}
		if e = s.candidateBinding(ctx, tx, w, in.CandidateBinding, f.VerifiedArtifacts); e != nil {
			return 0, nil, e
		}
		hash, invalid := domain.WorkflowAuthoritySHA256(w.Authority)
		if invalid != nil {
			return 0, nil, invalid
		}
		if !domain.ValidID(in.ID) || in.TaskID != taskID || in.TaskVersion != t.CurrentVersion || in.AuthoritySHA256 != hash || in.AcceptedByRequestID != f.RequestID {
			return 0, nil, evidenceUnavailable()
		}
		uses := append(append([]domain.EvidenceUse{}, in.ReviewUses...), in.CheckUses...)
		if !equalWorkflowUses(uses, w.EvidenceUses) {
			return 0, nil, evidenceUnavailable()
		}
		for _, u := range in.ReviewUses {
			if u.Kind != "review" {
				return 0, nil, evidenceUnavailable()
			}
		}
		for _, u := range in.CheckUses {
			if u.Kind != "check" {
				return 0, nil, evidenceUnavailable()
			}
		}
		review, checks, e := s.workflowUses(ctx, tx, w, uses, f.VerifiedArtifacts)
		if e != nil {
			return 0, nil, e
		}
		v, e := s.version(ctx, tx, t.ID, t.CurrentVersion)
		if e != nil {
			return 0, nil, e
		}
		direct := false
		for _, check := range checks {
			definition, _ := workflowDefinition(w, check.DefinitionID)
			if definition.Kind == "builtin" && check.BuiltinCompleted && check.Outcome == "passed" && !check.SourceChanged {
				direct = true
			}
		}
		input := workflowpolicy.PlanInput{Authority: w.Authority, ResponseOnly: v.Acceptance.Deliverable == "response_only", HasSubject: true, HasDelta: w.HasTaskDelta, HasIndexDelta: w.HasTaskIndexDelta, DirectProven: direct, Review: review, Checks: checks, CorrectionCyclesUsed: w.CorrectionCyclesUsed}
		if in.DeliveryOperationID != nil {
			op, e := storedWorkflowRecord[domain.DeliveryOperation](ctx, tx, "delivery_operations", *in.DeliveryOperationID)
			if e != nil {
				return 0, nil, e
			}
			if op.WorkflowID != w.ID || op.CandidateBinding != in.CandidateBinding || op.AuthoritySHA256 != hash || op.Status != "applied" || op.AfterDestination == nil {
				return 0, nil, evidenceUnavailable()
			}
			deliveryIDs := []string{op.AfterDestination.ManifestArtifactID}
			if op.BeforeDestination != nil {
				deliveryIDs = append(deliveryIDs, op.BeforeDestination.ManifestArtifactID)
			}
			if op.PlanArtifactID != "" {
				deliveryIDs = append(deliveryIDs, op.PlanArtifactID)
			}
			if e = s.verifyWorkflowProofs(ctx, tx, f.VerifiedArtifacts, deliveryIDs...); e != nil {
				return 0, nil, e
			}
			input.LocalOutcome = op.Status
			input.LocalApplied = true
		}
		if w.Authority.Delivery.LocalMode == "apply_verified_local" {
			if in.DeliveryResult != "applied" || !input.LocalApplied {
				return 0, nil, evidenceUnavailable()
			}
		} else if in.DeliveryResult != "review_only" || in.DeliveryOperationID != nil {
			return 0, nil, evidenceUnavailable()
		}
		if w.Authority.Delivery.ExternalAction != nil {
			if len(in.ExternalResultRefs) != 1 {
				return 0, nil, evidenceUnavailable()
			}
			step, e := s.workflowStep(ctx, tx, in.ExternalResultRefs[0])
			if e != nil {
				return 0, nil, e
			}
			r := step.ExternalResult
			if r == nil || r.WorkflowID != w.ID || r.Outcome != "command_succeeded" || !reflect.DeepEqual(step.InputCandidateID, w.SelectedCandidateID) || !reflect.DeepEqual(r.Action, *w.Authority.Delivery.ExternalAction) {
				return 0, nil, evidenceUnavailable()
			}
			for _, id := range []*string{r.StdoutArtifactID, r.StderrArtifactID} {
				if id == nil {
					return 0, nil, evidenceUnavailable()
				}
				if e = s.verifyWorkflowProofs(ctx, tx, f.VerifiedArtifacts, *id); e != nil {
					return 0, nil, e
				}
			}
			input.ExternalComplete = true
			input.ExternalOutcome = r.Outcome
		} else if len(in.ExternalResultRefs) != 0 {
			return 0, nil, evidenceUnavailable()
		}
		unresolved, resolutions, e := s.workflowBlockers(ctx, tx, w)
		if e != nil {
			return 0, nil, e
		}
		input.BlockingFindingIDs = unresolved
		input.NeedsFindingReview, e = s.workflowNeedsFindingReview(ctx, tx, w, unresolved)
		if e != nil {
			return 0, nil, e
		}
		if !sameStringSet(in.FindingResolutionIDs, resolutions) {
			return 0, nil, evidenceUnavailable()
		}
		if decision := workflowpolicy.Plan(input); decision.Action != "accept" {
			return 0, nil, domain.Fail("acceptance_unavailable", "Required current checks, independent review, findings and destination evidence do not yet qualify.")
		}
		in.AcceptedAt = domain.Now()
		seq, e := nextSequence(ctx, tx, "workflow_acceptances")
		if e != nil {
			return 0, nil, e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO workflow_acceptances VALUES(?,?,?,?,?,?,?)", seq, in.ID, w.ID, taskID, intRevision(w.TaskVersion), in.CandidateBinding.CandidateID, encode(in)); e != nil {
			return 0, nil, storageError()
		}
		w.LatestAcceptanceID = ptr(in.ID)
		w.State = "completed"
		w.NextAction = "inspect_verified_delivery"
		w.NextOwner = "user"
		w.UpdatedAt = in.AcceptedAt
		w.Revision, e = increment(w.Revision)
		if e != nil {
			return 0, nil, e
		}
		w.StateRevision, e = increment(w.StateRevision)
		if e != nil {
			return 0, nil, e
		}
		if e = saveWorkflow(ctx, tx, w); e != nil {
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
		t.UpdatedAt = in.AcceptedAt
		if e = saveTask(ctx, tx, t); e != nil {
			return 0, nil, e
		}
		return 200, domain.WorkflowDetail{TaskID: t.ID, TaskVersion: w.TaskVersion, Workflow: &w, Acceptance: &in}, nil
	})
}
func equalWorkflowUses(a, b []domain.EvidenceUse) bool {
	if len(a) != len(b) {
		return false
	}
	for _, use := range a {
		found := false
		for _, other := range b {
			if reflect.DeepEqual(use, other) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func sameStringSet(a, b []string) bool {
	a = append([]string{}, a...)
	b = append([]string{}, b...)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func (s *Store) workflowBlockers(ctx context.Context, q queryer, w domain.Workflow) ([]string, []string, error) {
	findings, e := s.workflowAllFindings(ctx, q, w.ID)
	if e != nil {
		return nil, nil, e
	}
	unresolved := []string{}
	resolutionIDs := []string{}
	for _, finding := range findings {
		if finding.Finding.Severity != "blocking" {
			continue
		}
		rows, e := q.QueryContext(ctx, "SELECT data FROM workflow_resolutions WHERE workflow_id=? AND finding_id=? ORDER BY sequence", w.ID, finding.ID)
		if e != nil {
			return nil, nil, storageError()
		}
		resolved := false
		for rows.Next() {
			var raw string
			if rows.Scan(&raw) != nil {
				rows.Close()
				return nil, nil, storageError()
			}
			r, e := decode[domain.FindingResolution](raw)
			if e != nil {
				rows.Close()
				return nil, nil, storageError()
			}
			if r.CorrectedCandidateID != nil && w.SelectedCandidateID != nil && *r.CorrectedCandidateID == *w.SelectedCandidateID || r.CorrectedResponseBinding != nil && reflect.DeepEqual(r.CorrectedResponseBinding, w.SelectedResponseBinding) {
				resolved = true
				resolutionIDs = append(resolutionIDs, r.ID)
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, nil, storageError()
		}
		if !resolved {
			unresolved = append(unresolved, finding.ID)
		}
	}
	return unresolved, resolutionIDs, nil
}
func (s *Store) workflowAllFindings(ctx context.Context, q queryer, id string) ([]domain.WorkflowFinding, error) {
	out := []domain.WorkflowFinding{}
	size := 0
	for _, table := range []string{"workflow_reviews", "workflow_checks"} {
		rows, e := q.QueryContext(ctx, "SELECT data FROM "+table+" WHERE workflow_id=? ORDER BY sequence", id)
		if e != nil {
			return nil, storageError()
		}
		for rows.Next() {
			var raw string
			if rows.Scan(&raw) != nil {
				rows.Close()
				return nil, storageError()
			}
			size += len(raw)
			if size > 4<<20 {
				rows.Close()
				return nil, evidenceUnavailable()
			}
			if table == "workflow_reviews" {
				r, e := decode[domain.ReviewRecord](raw)
				if e != nil {
					rows.Close()
					return nil, storageError()
				}
				out = append(out, r.Findings...)
			} else {
				r, e := decode[domain.CheckReceipt](raw)
				if e != nil {
					rows.Close()
					return nil, storageError()
				}
				if r.Finding != nil {
					out = append(out, *r.Finding)
				}
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, storageError()
		}
	}
	return out, nil
}
