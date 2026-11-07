package store

import (
	"context"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphArtifactKind(kind string) bool {
	switch kind {
	case "graph_index_blob", "graph_index_blobs", "graph_input_manifest", "graph_input_provenance", "graph_context", "graph_context_digest", "graph_operation_effects", "graph_materialization", "graph_output_closure", "graph_conditions", "graph_final_verification", "graph_cumulative_delta", "graph_original", "graph_acceptance", "graph_delivery":
		return true
	}
	return false
}
func (s *Store) graphArtifactOwner(ctx context.Context, q queryer, a domain.Attempt, kind string) error {
	if !strings.HasPrefix(kind, "graph_") {
		return nil
	}
	ref, e := s.graphMembership(ctx, q, a.TaskID)
	if e != nil {
		return e
	}
	if ref == nil || !graphArtifactKind(kind) {
		return domain.Invalid("registered graph artifact producer")
	}
	if a.Phase == "queued" {
		return domain.Invalid("claimed graph artifact producer")
	}
	if a.Phase != "settled" {
		if e = s.checkAttemptClaim(ctx, q, a.OwnerInstanceID, a.ID); e != nil {
			return e
		}
	}
	if ref.Kind == "root" {
		if a.ExecutorKind != "local_graph" || a.Role != "graph_basis" && a.Role != "graph_input" {
			return domain.Invalid("actual graph root operation producer")
		}
		var recordKind string
		if e = q.QueryRowContext(ctx, "SELECT kind FROM graph_inputs WHERE graph_id=? AND operation_attempt_id=?", ref.GraphID, a.ID).Scan(&recordKind); e != nil {
			return storageError()
		}
		if recordKind == "root_basis" && a.Role != "graph_basis" || recordKind == "node_input" && a.Role != "graph_input" {
			return domain.Invalid("matching graph operation role")
		}
		switch kind {
		case "graph_index_blob", "graph_index_blobs", "graph_input_manifest", "graph_input_provenance", "graph_context", "graph_context_digest", "graph_operation_effects", "graph_conditions", "graph_original", "graph_acceptance":
			return nil
		default:
			return domain.Invalid("graph root operation artifact kind")
		}
	}
	var workflow, step, node string
	if e = q.QueryRowContext(ctx, "SELECT workflow_id,step_id,node_id FROM graph_dispatches WHERE attempt_id=? AND graph_id=?", a.ID, ref.GraphID).Scan(&workflow, &step, &node); e != nil {
		return domain.Invalid("registered receiving graph execution")
	}
	if ref.NodeID == nil || node != *ref.NodeID || workflow != a.WorkflowID || step != a.StepID {
		return evidenceUnavailable()
	}
	switch kind {
	case "graph_index_blob", "graph_index_blobs", "graph_operation_effects", "graph_materialization":
		if a.ExecutorKind == "native" || a.ExecutorKind == "local_check" {
			return nil
		}
		if a.ExecutorKind == "local_external" && a.Role == "external_delivery" {
			return s.graphExternalArtifactScope(ctx, q, a, ref.GraphID, node)
		}
	case "graph_output_closure":
		if a.ExecutorKind == "native" && (a.Role == "author" || a.Role == "correction") || a.ExecutorKind == "local_check" && a.Role == "check" {
			return nil
		}
	case "graph_delivery":
		h, e := s.graphHeader(ctx, q, ref.GraphID)
		if e != nil {
			return e
		}
		if h.Summary.FinalNodeID != nil && *h.Summary.FinalNodeID == node && a.ExecutorKind == "local_application" && a.Role == "local_delivery" {
			return nil
		}
	case "graph_final_verification", "graph_cumulative_delta":
		h, e := s.graphHeader(ctx, q, ref.GraphID)
		if e != nil {
			return e
		}
		if h.Summary.FinalNodeID != nil && *h.Summary.FinalNodeID == node && (a.ExecutorKind == "native" && (a.Role == "author" || a.Role == "correction") || a.ExecutorKind == "local_check" && a.Role == "check") {
			return nil
		}
	}
	return domain.Invalid("graph node artifact producer role")
}
