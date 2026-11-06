package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Missing captured authority never grants work or reuses an old task implicitly.
func workflowUnavailable() *domain.Error {
	return domain.Fail("authority_required", "This task lacks complete captured workflow authority. Retain the request and inspect its current task version.")
}

func (s *Store) WorkflowSteps(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.WorkflowStep], error) {
	return s.workflowStepsList(ctx, taskID, selector, limit, cursor)
}
func (s *Store) WorkflowReviews(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.ReviewRecord], error) {
	return s.workflowReviewsList(ctx, taskID, selector, limit, cursor)
}
func (s *Store) WorkflowChecks(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.CheckReceipt], error) {
	return s.workflowChecksList(ctx, taskID, selector, limit, cursor)
}
func (s *Store) WorkflowFindings(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.WorkflowFinding], error) {
	return s.workflowFindingsList(ctx, taskID, selector, limit, cursor)
}
func (s *Store) WorkflowCandidates(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.Candidate], error) {
	return s.workflowCandidatesList(ctx, taskID, selector, limit, cursor)
}
func (s *Store) WorkflowDeliveries(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.WorkflowDeliveryView], error) {
	return s.workflowDeliveriesList(ctx, taskID, selector, limit, cursor)
}
