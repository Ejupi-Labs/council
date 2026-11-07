package store

import (
	"context"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// An explicit graph continuation can perform local work before the next native
// admission opens a replacement common window. Its preparation deadline was
// fixed by that window's grant; neither an operation nor a read renews it.
func (s *Store) graphUnopenedLocalDeadline(ctx context.Context, q queryer, a domain.Attempt, b *domain.TaskBudget) (*time.Time, error) {
	if b == nil || b.CurrentWindow.DeadlineAt != nil {
		return nil, nil
	}
	switch attemptExecutor(a) {
	case "local_check", "local_application", "local_external":
	default:
		return nil, nil
	}
	member, err := s.graphMembership(ctx, q, a.TaskID)
	if err != nil || member == nil || member.Kind != "node" || member.NodeID == nil {
		return nil, err
	}
	record, err := s.graphDispatchForAttempt(ctx, q, a.ID)
	if err != nil {
		return nil, err
	}
	d := record.Public.Dispatch
	if record.OwnerResumeRequestID == nil || !domain.ValidID(*record.OwnerResumeRequestID) || b.CurrentWindow.FirstAdmissionAt != nil || d.GraphID != member.GraphID || d.NodeID != *member.NodeID || d.TaskID != a.TaskID || d.TaskVersion != a.TaskVersion || d.WorkflowID != a.WorkflowID || d.StepID != a.StepID || d.OwnerInstanceID != a.OwnerInstanceID || d.ExecutorKind != attemptExecutor(a) || d.Role != a.Role || d.WindowNumber != b.CurrentWindow.Number {
		return nil, graphControlError()
	}
	h, err := s.graphHeader(ctx, q, member.GraphID)
	if err != nil {
		return nil, err
	}
	if h.Summary.Stop != nil || h.Summary.GraphVersion != d.GraphVersion || h.Summary.ControlRevision != d.GraphControlRevision || h.Summary.BudgetAuthorityEpoch != d.BudgetAuthorityEpoch {
		return nil, graphControlError()
	}
	deadline, invalid := domain.ParseRoutingTime(h.PreparationDeadlineAt)
	if invalid != nil {
		return nil, storageError()
	}
	return &deadline, nil
}
