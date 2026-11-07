package store

import (
	"context"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Retaining an observed receiving/effect receipt does not grant execution. In
// particular, Stop must not prevent recording an already attempted Handoff.
// Existing dispatch/launch guards independently enforce current live controls.
func (s *Store) graphExternalArtifactScope(ctx context.Context, q queryer, a domain.Attempt, graphID, nodeID string) error {
	if a.ExecutorKind != "local_external" || a.Role != "external_delivery" {
		return domain.Invalid("exact graph external publisher artifact producer")
	}
	h, err := s.graphHeader(ctx, q, graphID)
	if err != nil {
		return err
	}
	if h.Summary.FinalNodeID == nil || *h.Summary.FinalNodeID != nodeID {
		return domain.Invalid("designated final graph publisher")
	}
	definition, err := s.graphDefinition(ctx, q, graphID, h.Summary.GraphVersion)
	if err != nil {
		return err
	}
	w, err := s.workflowByID(ctx, q, a.WorkflowID)
	if err != nil {
		return err
	}
	if w.TaskID != a.TaskID || w.TaskVersion != a.TaskVersion || definition.Delivery.ExternalAction == nil ||
		!reflect.DeepEqual(definition.Delivery, w.Authority.Delivery) {
		return domain.Invalid("unchanged captured graph publisher scope")
	}
	nodes, err := s.graphNodes(ctx, q, graphID)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if node.Summary.ID == nodeID && node.Summary.TaskID == a.TaskID && node.Summary.Key == definition.Original.FinalNodeKey {
			return nil
		}
	}
	return evidenceUnavailable()
}

// This live guard is distinct from receipt retention above. It runs under the
// caller's existing claim/version/window checks immediately before Prepare/GO.
func (s *Store) graphExternalDispatchScope(ctx context.Context, q queryer, a domain.Attempt, h graphHeader, d domain.GraphDispatch, w domain.Workflow, input domain.GraphInputBinding, nodes []graphNodeRecord) error {
	if a.ExecutorKind != "local_external" && a.Role != "external_delivery" {
		return nil
	}
	if err := s.graphExternalArtifactScope(ctx, q, a, h.Summary.ID, d.NodeID); err != nil {
		return err
	}
	definition, err := s.graphDefinition(ctx, q, h.Summary.ID, h.Summary.GraphVersion)
	if err != nil {
		return err
	}
	fileAncestors, err := graphDeliveryFileAncestors(definition, nodes, d.NodeID)
	if err != nil {
		return err
	}
	if len(input.ContributorVector) != len(nodes)-1 {
		return evidenceUnavailable()
	}
	switch {
	case w.SelectedCandidateID != nil && w.SelectedResponseBinding == nil:
		if d.SourceBasis.FinalVerificationRef == nil || d.SourceBasis.CandidateBinding == nil || d.SourceBasis.CandidateBinding.CandidateID != *w.SelectedCandidateID || d.SourceBasis.ResponseBinding != nil {
			return evidenceUnavailable()
		}
	case w.SelectedResponseBinding != nil && w.SelectedCandidateID == nil:
		if d.SourceBasis.CandidateBinding != nil || !reflect.DeepEqual(d.SourceBasis.ResponseBinding, w.SelectedResponseBinding) || d.SourceBasis.FinalVerificationRef != nil {
			return evidenceUnavailable()
		}
	default:
		return evidenceUnavailable()
	}
	anchor, err := s.graphNodeAdmissionAnchor(ctx, q, d)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, contributor := range input.ContributorVector {
		if contributor.CandidateBinding != nil && !fileAncestors[contributor.NodeID] {
			return domain.Fail("graph_file_ancestry_unproven", "Required file results must reach the final publisher through declared file dependencies.")
		}
		if contributor.NodeID == d.NodeID || seen[contributor.NodeID] || w.SelectedResponseBinding != nil && contributor.CandidateBinding != nil {
			return domain.Fail("graph_final_unqualified", "A response cannot qualify required retained file results for publication.")
		}
		seen[contributor.NodeID] = true
		found := false
		for _, node := range nodes {
			if node.Summary.ID != contributor.NodeID {
				continue
			}
			// The existing dispatch guard already rejects ancestor Stop before
			// first GO. It never erases this receiver's known captured authority.
			if node.Summary.Stop != nil && anchor.KnownFirstGOAt != nil && anchor.KnownFirstGOAttemptID != nil {
				node.Summary.Stop = nil
			}
			head, err := s.graphSelectedHead(ctx, q, h, node)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(head.Vector, contributor) {
				return evidenceUnavailable()
			}
			found = true
		}
		if !found {
			return evidenceUnavailable()
		}
	}
	return nil
}
