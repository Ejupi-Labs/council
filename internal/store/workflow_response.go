package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"reflect"
)

// AcceptResponseWithWorkflow keeps the established response route/receipt and
// immutable acknowledgement shape. Additional controller proofs qualify the
// selected original producer, independently of later supporting attempts.
func (s *Store) AcceptResponseWithWorkflow(ctx context.Context, taskID string, in domain.ResponseAcceptanceInput, proofs []domain.VerifiedResponse) domain.Result {
	if invalid := domain.ValidateResponseAcceptance(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	return s.mutate(ctx, in.RequestID, "tasks.response.accept", ResponseAcceptanceFingerprint(taskID, in), func(tx *sql.Tx) (int, any, error) {
		t, e := s.task(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		if t.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(t.Revision)
		}
		if t.CurrentVersion != in.TaskVersion {
			return 0, nil, domain.Fail("task_version_conflict", "Accept only the current selected response and conditions.")
		}
		selected, e := s.selectedWorkflow(ctx, tx, taskID, domain.WorkflowSelector{})
		if e != nil {
			return 0, nil, e
		}
		if selected.Workflow == nil {
			return 0, nil, domain.Fail("acceptance_unavailable", "Use the existing standalone acknowledgement boundary for standalone work.")
		}
		w := *selected.Workflow
		b := w.SelectedResponseBinding
		if b == nil || w.HasTaskDelta || w.HasTaskIndexDelta || b.ProducerAttemptID != in.AttemptID || b.ResponseArtifactID != in.ResponseArtifactID || b.ResponseSHA256 != in.ResponseSHA256 || w.State == "completed" || w.State == "cancelled" || w.State == "cancelling" {
			return 0, nil, domain.Fail("acceptance_unavailable", "The current selected response is absent, changed, cancelled or already accepted.")
		}
		g, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if e = checkGate(g); e != nil {
			return 0, nil, e
		}
		producer, e := s.attempt(ctx, tx, b.ProducerAttemptID)
		if e != nil {
			return 0, nil, e
		}
		if producer.Phase != "settled" || producer.Outcome == nil || *producer.Outcome != "succeeded" || producer.Stop != nil || producer.PrimaryFailure != nil || producer.Cleanup.State != "verified" || producer.EvidenceStatus != "complete" {
			return 0, nil, evidenceUnavailable()
		}
		supporting, e := s.latest(ctx, tx, t)
		if e != nil {
			return 0, nil, e
		}
		if supporting == nil || supporting.WorkflowID != w.ID || supporting.Phase != "settled" || supporting.Outcome == nil || *supporting.Outcome != "succeeded" || supporting.Stop != nil || supporting.PrimaryFailure != nil || supporting.Cleanup.State != "verified" || supporting.EvidenceStatus != "complete" {
			return 0, nil, domain.Fail("acceptance_unavailable", "Supporting work is not completely settled for the selected response.")
		}
		if e = s.responseBinding(ctx, tx, w, *b, proofs); e != nil {
			return 0, nil, e
		}
		review, checks, e := s.workflowUses(ctx, tx, w, w.EvidenceUses, proofs)
		if e != nil {
			return 0, nil, e
		}
		v, e := s.version(ctx, tx, taskID, t.CurrentVersion)
		if e != nil {
			return 0, nil, e
		}
		if v.Acceptance.Deliverable != "response_only" {
			return 0, nil, evidenceUnavailable()
		}
		unresolved, resolutions, e := s.workflowBlockers(ctx, tx, w)
		if e != nil {
			return 0, nil, e
		}
		needsFindingReview, e := s.workflowNeedsFindingReview(ctx, tx, w, unresolved)
		if e != nil {
			return 0, nil, e
		}
		input := workflowpolicy.PlanInput{NeedsFindingReview: needsFindingReview, Authority: w.Authority, ResponseOnly: true, HasSubject: true, Review: review, Checks: checks, CorrectionCyclesUsed: w.CorrectionCyclesUsed, BlockingFindingIDs: unresolved}
		externalRefs := []string{}
		if w.Authority.Delivery.ExternalAction != nil {
			rows, e := tx.QueryContext(ctx, "SELECT data FROM workflow_steps WHERE workflow_id=? AND role='external_delivery' AND input_response_artifact_id=? ORDER BY sequence DESC LIMIT 1", w.ID, b.ResponseArtifactID)
			if e != nil {
				return 0, nil, storageError()
			}
			var step domain.WorkflowStep
			for rows.Next() {
				var raw string
				if rows.Scan(&raw) != nil {
					rows.Close()
					return 0, nil, storageError()
				}
				step, e = decode[domain.WorkflowStep](raw)
				if e != nil {
					rows.Close()
					return 0, nil, storageError()
				}
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return 0, nil, storageError()
			}
			r := step.ExternalResult
			if r == nil || r.Outcome != "command_succeeded" || !reflect.DeepEqual(step.InputResponseBinding, b) || !reflect.DeepEqual(r.Action, *w.Authority.Delivery.ExternalAction) {
				return 0, nil, evidenceUnavailable()
			}
			for _, id := range []*string{r.StdoutArtifactID, r.StderrArtifactID} {
				if id == nil {
					return 0, nil, evidenceUnavailable()
				}
				if e = s.verifyWorkflowProofs(ctx, tx, proofs, *id); e != nil {
					return 0, nil, e
				}
			}
			input.ExternalComplete = true
			input.ExternalOutcome = r.Outcome
			externalRefs = append(externalRefs, step.ID)
		}
		if decision := workflowpolicy.Plan(input); decision.Action != "response_acceptance_pending" {
			return 0, nil, domain.Fail("acceptance_unavailable", "Current response checks, review, finding resolutions and required evidence do not qualify for user acknowledgement.")
		}
		record := domain.ResponseAcceptance{ID: domain.NewID(), RequestID: in.RequestID, TaskID: taskID, TaskVersion: in.TaskVersion, AttemptID: in.AttemptID, ResponseArtifactID: in.ResponseArtifactID, ResponseSHA256: in.ResponseSHA256, Basis: "user_review", Confirmation: in.Confirmation, AcceptedAt: domain.Now()}
		if _, e = tx.ExecContext(ctx, "INSERT INTO response_acceptances VALUES(?,?,?,?,?,?)", record.ID, taskID, intRevision(in.TaskVersion), in.AttemptID, in.ResponseArtifactID, encode(record)); e != nil {
			return 0, nil, storageError()
		}
		completion := domain.WorkflowCompletion{Kind: "response_only_user_review", ResponseAcceptanceID: record.ID, ResponseBinding: *b, ReviewUses: []domain.EvidenceUse{}, CheckUses: []domain.EvidenceUse{}, FindingResolutionIDs: resolutions, ExternalResultRefs: externalRefs}
		for _, use := range w.EvidenceUses {
			if use.Kind == "review" {
				completion.ReviewUses = append(completion.ReviewUses, use)
			} else {
				completion.CheckUses = append(completion.CheckUses, use)
			}
		}
		if len(w.EvidenceUses) != 0 || len(externalRefs) != 0 || len(resolutions) != 0 {
			completion.Kind = "response_only_user_review_with_evidence"
		}
		w.Completion = &completion
		w.State = "completed"
		w.NextAction = "inspect_accepted_response"
		w.NextOwner = "user"
		w.UpdatedAt = record.AcceptedAt
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
		t.UpdatedAt = record.AcceptedAt
		if e = saveTask(ctx, tx, t); e != nil {
			return 0, nil, e
		}
		return 200, struct {
			Task       domain.TaskSummary        `json:"task"`
			Acceptance domain.ResponseAcceptance `json:"acceptance"`
		}{taskSummary(t, v, supporting, true), record}, nil
	})
}
