package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"slices"
)

func (s *Store) workflowNeedsFindingReview(ctx context.Context, q queryer, w domain.Workflow, unresolved []string) (bool, error) {
	if len(unresolved) == 0 {
		return false, nil
	}
	findings, err := s.workflowAllFindings(ctx, q, w.ID)
	if err != nil {
		return false, err
	}
	for _, finding := range findings {
		if finding.ReviewID != "" && slices.Contains(unresolved, finding.ID) {
			return true, nil
		}
	}
	return false, nil
}

// Resolution joins a fresh qualifying inspection with observed affected checks.
// A model's list is only a claim; no list alone changes an old finding/outcome.
func (s *Store) resolveWorkflowFindings(ctx context.Context, tx *sql.Tx, w domain.Workflow, review *domain.ReviewRecord, checks []domain.CheckReceipt, direct bool) error {
	if w.CorrectionCyclesUsed == 0 {
		return nil
	}
	v, e := s.version(ctx, tx, w.TaskID, w.TaskVersion)
	if e != nil {
		return e
	}
	plan := workflowpolicy.Plan(workflowpolicy.PlanInput{Authority: w.Authority, ResponseOnly: v.Acceptance.Deliverable == "response_only", HasSubject: w.SelectedCandidateID != nil || w.SelectedResponseBinding != nil, HasDelta: w.HasTaskDelta, HasIndexDelta: w.HasTaskIndexDelta, DirectProven: direct, Review: review, Checks: checks, CorrectionCyclesUsed: w.CorrectionCyclesUsed})
	if plan.Action != "accept" && plan.Action != "response_acceptance_pending" && plan.Role != "local_delivery" && plan.Role != "external_delivery" {
		return nil
	}
	producerID, subjectHash := "", ""
	var candidateID, responseID *string
	if w.SelectedCandidateID != nil {
		c, e := s.candidate(ctx, tx, *w.SelectedCandidateID)
		if e != nil {
			return e
		}
		producerID = c.AttemptID
		candidateID = ptr(c.ID)
		a, e := s.artifact(ctx, tx, c.ManifestArtifactID)
		if e != nil || a.SHA256 == nil {
			return evidenceUnavailable()
		}
		subjectHash = *a.SHA256
	} else if w.SelectedResponseBinding != nil {
		producerID = w.SelectedResponseBinding.ProducerAttemptID
		responseID = ptr(w.SelectedResponseBinding.ResponseArtifactID)
		subjectHash = w.SelectedResponseBinding.ResponseSHA256
	} else {
		return nil
	}
	producer, e := s.attempt(ctx, tx, producerID)
	if e != nil {
		return e
	}
	if producer.WorkflowID != w.ID || producer.Role != "correction" {
		return nil
	}
	correctionStep, e := s.workflowStep(ctx, tx, producer.StepID)
	if e != nil {
		return e
	}
	if correctionStep.Role != "correction" || correctionStep.Cycle != 1 {
		return evidenceUnavailable()
	}
	findings, e := s.workflowAllFindings(ctx, tx, w.ID)
	if e != nil {
		return e
	}
	for _, finding := range findings {
		if finding.Finding.Severity != "blocking" {
			continue
		}
		var originalReview, originalCheck *string
		if finding.ReviewID != "" {
			if review == nil || review.Outcome != "approved" || len(review.Report.Gaps) != 0 || !slices.Contains(review.Report.ResolvedFindingIDs, finding.ID) {
				continue
			}
			originalReview = ptr(finding.ReviewID)
		} else if finding.CheckID != "" {
			origin, e := storedWorkflowRecord[domain.CheckReceipt](ctx, tx, "workflow_checks", finding.CheckID)
			if e != nil {
				return e
			}
			passed := false
			for _, check := range checks {
				if check.DefinitionID == origin.DefinitionID && check.DefinitionSHA256 == origin.DefinitionSHA256 && check.Outcome == "passed" && !check.SourceChanged && check.Cleanup.State == "verified" && check.RootTerminal && check.InputComplete && check.OutputComplete {
					passed = true
				}
			}
			if !passed {
				continue
			}
			originalCheck = ptr(finding.CheckID)
		} else {
			return evidenceUnavailable()
		}
		var exists int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM workflow_resolutions WHERE workflow_id=? AND finding_id=? AND corrected_subject_sha256=?", w.ID, finding.ID, subjectHash).Scan(&exists); e != nil {
			return storageError()
		}
		if exists != 0 {
			continue
		}
		record := domain.FindingResolution{ID: domain.NewID(), WorkflowID: w.ID, FindingID: finding.ID, OriginReviewID: originalReview, OriginCheckID: originalCheck, CorrectedCandidateID: candidateID, CorrectedResponseBinding: w.SelectedResponseBinding, CorrectionStepID: correctionStep.ID, QualifyingReviewIDs: []string{}, QualifyingCheckIDs: []string{}, EvidenceUses: append([]domain.EvidenceUse{}, w.EvidenceUses...), CreatedAt: domain.Now()}
		if review != nil {
			record.QualifyingReviewIDs = append(record.QualifyingReviewIDs, review.ID)
		}
		for _, check := range checks {
			if check.Outcome == "passed" {
				record.QualifyingCheckIDs = append(record.QualifyingCheckIDs, check.ID)
			}
		}
		seq, e := nextSequence(ctx, tx, "workflow_resolutions")
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO workflow_resolutions VALUES(?,?,?,?,?,?,?,?,?,?)", seq, record.ID, w.ID, finding.ID, originalReview, originalCheck, candidateID, responseID, subjectHash, encode(record)); e != nil {
			return storageError()
		}
	}
	return nil
}
