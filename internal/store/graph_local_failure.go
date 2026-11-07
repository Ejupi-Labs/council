package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// A known receiver-preparation failure consumes only that affected scope. It
// cannot revoke an independent node's existing admission or captured intent.
func (s *Store) scopeGraphInputFailure(ctx context.Context, tx *sql.Tx, h graphHeader, op domain.GraphInputOperation) (graphHeader, error) {
	if op.Control.NodeID == nil || op.Control.NodeControlRevision == nil || op.Control.TaskVersion == nil {
		return h, graphControlError()
	}
	nodes, e := s.graphNodes(ctx, tx, op.GraphID)
	if e != nil {
		return h, e
	}
	failedKey := ""
	for _, node := range nodes {
		n := node.Summary
		if n.ID == *op.Control.NodeID && n.ControlRevision == *op.Control.NodeControlRevision && n.TaskVersion == *op.Control.TaskVersion {
			failedKey = n.Key
			break
		}
	}
	if failedKey == "" {
		return h, nil
	} // A later explicit scope owns the current projection.
	affected := map[string]bool{failedKey: true}
	if op.Definition.Original.Dependencies != nil {
		for i := 0; i < len(nodes); i++ {
			for _, edge := range *op.Definition.Original.Dependencies {
				if affected[edge.From] {
					affected[edge.To] = true
				}
			}
		}
	}
	removed := map[string]bool{}
	for i := range nodes {
		node := &nodes[i]
		n := &node.Summary
		if !affected[n.Key] {
			continue
		}
		if n.ID != *op.Control.NodeID && n.NodeAdmissionID != nil {
			continue
		}
		removed[n.ID] = true
		if n.ID != *op.Control.NodeID && !graphRetainedTerminal(n.State) {
			n.State = "blocked"
			n.NextOwner = "user"
			n.NextAction = "Wait for a current accepted dependency before explicit resumption."
			node.Blocker = &domain.GraphBlockerSummary{Code: "graph_dependencies_pending", Message: "A required dependency has a known preparation failure.", NodeID: &n.ID, InputID: n.InputID}
			n.StateRevision, e = increment(n.StateRevision)
			if e != nil {
				return h, e
			}
			if e = saveGraphNode(ctx, tx, *node); e != nil {
				return h, e
			}
		}
	}
	if h.DispatchAuthority != nil {
		kept := make([]domain.GraphNodeAuthority, 0, len(h.DispatchAuthority.NodeAuthorities))
		for _, authority := range h.DispatchAuthority.NodeAuthorities {
			if !removed[authority.NodeID] {
				kept = append(kept, authority)
			}
		}
		h.DispatchAuthority.NodeAuthorities = kept
		if len(kept) == 0 {
			h.DispatchAuthority.State = "suspended"
		}
	}
	h.Summary.Counts, e = graphCounts(nodes)
	if e != nil {
		return h, e
	}
	h.Summary.State = "blocked"
	h.Summary.NextOwner = "user"
	h.Summary.NextAction = "Inspect the affected preparation evidence. Independent work keeps its captured authority."
	return h, nil
}
