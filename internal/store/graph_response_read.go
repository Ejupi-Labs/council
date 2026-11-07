package store

import (
	"context"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// GraphResponseAcceptance resolves an exact immutable acknowledgement. A later
// supporting review/check may be the latest attempt and never replaces producer.
func (s *Store) GraphResponseAcceptance(ctx context.Context, graphID, nodeID, acceptanceID string) (domain.ResponseAcceptance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out domain.ResponseAcceptance
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if !domain.ValidID(graphID) || !domain.ValidID(nodeID) || !domain.ValidID(acceptanceID) {
		return out, domain.Invalid("exact graph response acknowledgement IDs")
	}
	nodes, e := s.graphNodes(ctx, s.db, graphID)
	if e != nil {
		return out, e
	}
	var node *domain.GraphNodeSummary
	for i := range nodes {
		if nodes[i].Summary.ID == nodeID {
			node = &nodes[i].Summary
			break
		}
	}
	if node == nil {
		return out, notFound()
	}
	task, e := s.task(ctx, s.db, node.TaskID)
	if e != nil {
		return out, e
	}
	if task.CurrentVersion != node.TaskVersion {
		return out, domain.Fail("graph_input_stale", "The selected response is not the current receiving version.")
	}
	view, e := s.selectedWorkflow(ctx, s.db, task.ID, domain.WorkflowSelector{})
	if e != nil {
		return out, e
	}
	if view.Workflow == nil || view.Workflow.TaskVersion != task.CurrentVersion || view.Workflow.Completion == nil || view.Workflow.SelectedResponseBinding == nil {
		return out, domain.Fail("acceptance_unavailable", "This node has no current recorded response completion.")
	}
	w := *view.Workflow
	completion := w.Completion
	binding := *w.SelectedResponseBinding
	if completion.ResponseAcceptanceID != acceptanceID || !reflect.DeepEqual(completion.ResponseBinding, binding) {
		return out, evidenceUnavailable()
	}
	out, e = storedWorkflowRecord[domain.ResponseAcceptance](ctx, s.db, "response_acceptances", acceptanceID)
	if e != nil {
		return out, e
	}
	if out.ID != acceptanceID || out.TaskID != task.ID || out.TaskVersion != task.CurrentVersion || out.AttemptID != binding.ProducerAttemptID || out.ResponseArtifactID != binding.ResponseArtifactID || out.ResponseSHA256 != binding.ResponseSHA256 || binding.ConditionsSHA256 != w.Authority.ConditionsSHA256 {
		return out, evidenceUnavailable()
	}
	return out, nil
}
