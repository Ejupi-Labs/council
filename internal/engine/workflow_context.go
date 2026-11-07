package engine

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Supply current applicable checks and the actual origins of unresolved
// blockers. This is bounded linked context, never a dump of workflow history or
// an assertion that an old check is applicable to the current selected subject.
func (e *Engine) workflowLinkedContext(ctx context.Context, stage *workflowNativeContext, findings []domain.WorkflowFinding, provided []adapters.SuppliedEvidence) ([]adapters.SuppliedEvidence, []domain.CheckReceipt, error) {
	subject := stage.subject
	_, _, current, _, err := e.currentWorkflowEvidence(ctx, subject, stage.final)
	if err != nil {
		return nil, nil, err
	}
	checks := map[string]domain.CheckReceipt{}
	reviews := map[string]domain.ReviewRecord{}
	for _, check := range current {
		checks[check.ID] = check
	}
	for _, finding := range findings {
		if finding.CheckID != "" {
			check, readErr := e.db.WorkflowCheck(ctx, finding.CheckID)
			if readErr != nil {
				return nil, nil, readErr
			}
			if check.WorkflowID != subject.Workflow.ID {
				return nil, nil, domain.Fail("evidence_unavailable", "A linked check belongs to another workflow.")
			}
			checks[check.ID] = check
		}
		if finding.ReviewID != "" {
			review, readErr := e.db.WorkflowReview(ctx, finding.ReviewID)
			if readErr != nil {
				return nil, nil, readErr
			}
			if review.WorkflowID != subject.Workflow.ID {
				return nil, nil, domain.Fail("evidence_unavailable", "A linked review belongs to another workflow.")
			}
			reviews[review.ID] = review
		}
	}
	if len(checks) > 2*domain.MaxWorkflowChecks {
		return nil, nil, domain.Fail("input_too_large", "Current and originating check receipts exceed the complete native context bound; none were truncated.")
	}
	scope := "whole_candidate"
	if subject.Response != nil {
		scope = "whole_response"
	}
	provide := func(id, ref, attemptID string) error {
		if len(provided) >= 64 {
			return domain.Fail("input_too_large", "Linked evidence exceeds sixty-four accessible references; none were truncated.")
		}
		meta, verifyErr := e.evidence.VerifyClosed(ctx, id)
		if verifyErr != nil {
			return verifyErr
		}
		if meta.AttemptID != attemptID {
			return domain.Fail("evidence_unavailable", "A linked artifact has a different producing attempt.")
		}
		provided = append(provided, adapters.SuppliedEvidence{Ref: ref, Path: filepath.Join(e.lock.Root(), filepath.FromSlash(meta.RelativePath)), Scope: scope, Paths: []string{}})
		stage.supplied = append(stage.supplied, domain.ReviewInspection{Ref: ref, Scope: scope, Paths: []string{}})
		stage.artifactIDs = append(stage.artifactIDs, id)
		return nil
	}
	checkIDs := make([]string, 0, len(checks))
	for id := range checks {
		checkIDs = append(checkIDs, id)
	}
	slices.Sort(checkIDs)
	receipts := make([]domain.CheckReceipt, 0, len(checks))
	for _, id := range checkIDs {
		check := checks[id]
		for _, stream := range []struct {
			id   *string
			name string
		}{{check.StdoutArtifactID, "stdout"}, {check.StderrArtifactID, "stderr"}} {
			if stream.id != nil {
				if err := provide(*stream.id, "check-"+id+"-"+stream.name, check.AttemptID); err != nil {
					return nil, nil, err
				}
			}
		}
		receipts = append(receipts, check)
	}
	reviewIDs := make([]string, 0, len(reviews))
	for id := range reviews {
		reviewIDs = append(reviewIDs, id)
	}
	slices.Sort(reviewIDs)
	for _, id := range reviewIDs {
		review := reviews[id]
		if err := provide(review.ResultArtifactID, "review-"+id, review.AttemptID); err != nil {
			return nil, nil, err
		}
	}
	return provided, receipts, nil
}
