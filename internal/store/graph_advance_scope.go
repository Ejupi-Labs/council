package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// A complete captured vector is retained verbatim. Current comparisons apply
// only to selected work and its causal ancestors; stale unrelated entries never
// become current authority merely because another node advances.
func (s *Store) graphAdvanceRelevantNodes(ctx context.Context, tx *sql.Tx, h graphHeader, nodes []graphNodeRecord, in domain.GraphAdvanceInput, f domain.GraphAdvanceFacts) (map[string]bool, map[string]bool, error) {
	selected := map[string]bool{}
	for _, prep := range f.Preparations {
		if prep.Control.NodeID != nil {
			selected[*prep.Control.NodeID] = true
		}
	}
	for _, admission := range f.Admissions {
		selected[admission.NodeID] = true
	}
	for _, stage := range f.Stages {
		if stage.Graph != nil && stage.Graph.Control.NodeID != nil {
			selected[*stage.Graph.Control.NodeID] = true
		}
	}
	if in.RebindNodeIDs != nil {
		for _, id := range *in.RebindNodeIDs {
			selected[id] = true
		}
	}
	if len(selected) == 0 {
		for _, node := range nodes {
			if node.Summary.Stop == nil && !graphRetainedTerminal(node.Summary.State) {
				selected[node.Summary.ID] = true
			}
		}
	}
	definition, err := s.graphDefinition(ctx, tx, h.Summary.ID, h.Summary.GraphVersion)
	if err != nil {
		return nil, nil, err
	}
	byID, byKey := map[string]graphNodeRecord{}, map[string]graphNodeRecord{}
	for _, node := range nodes {
		byID[node.Summary.ID], byKey[node.Summary.Key] = node, node
	}
	relevant, stopAllowed, visited := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for id := range selected {
		node, found := byID[id]
		if !found {
			return nil, nil, graphControlError()
		}
		relevant[id] = true
		knownGO := false
		if node.Summary.NodeAdmissionID != nil {
			record, err := s.graphDispatchRecord(ctx, tx, h.Summary.ID, *node.Summary.NodeAdmissionID)
			if err != nil {
				return nil, nil, err
			}
			anchor, err := s.graphNodeAdmissionAnchor(ctx, tx, record.Public.Dispatch)
			if err != nil {
				return nil, nil, err
			}
			knownGO = anchor.KnownFirstGOAt != nil
		}
		ancestors := map[string]bool{node.Summary.Key: true}
		if definition.Original.Dependencies != nil {
			for range nodes {
				for _, edge := range *definition.Original.Dependencies {
					if ancestors[edge.To] {
						ancestors[edge.From] = true
					}
				}
			}
		}
		for key := range ancestors {
			ancestor := byKey[key].Summary
			relevant[ancestor.ID] = true
			if !visited[ancestor.ID] {
				stopAllowed[ancestor.ID] = knownGO
				visited[ancestor.ID] = true
			} else {
				stopAllowed[ancestor.ID] = stopAllowed[ancestor.ID] && knownGO
			}
		}
	}
	for id := range selected {
		stopAllowed[id] = false // Own Stop/current control is always binding.
	}
	return relevant, stopAllowed, nil
}

func graphAdvanceStopOnlyDrift(node domain.GraphNodeSummary, captured domain.GraphNodeAuthority) bool {
	if node.Stop == nil || node.Stop.NodeControlRevision == nil || *node.Stop.NodeControlRevision != node.ControlRevision || node.TaskVersion != captured.TaskVersion {
		return false
	}
	next, err := increment(captured.NodeControlRevision)
	return err == nil && next == node.ControlRevision
}
