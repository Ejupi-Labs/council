package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) updateGraphGuidance(ctx context.Context, ownerID, graphID, nodeKey string, in domain.GraphGuidanceInput, f domain.GraphAdvanceFacts) domain.Result {
	if !domain.ValidID(ownerID) || !domain.ValidID(graphID) {
		return domain.Failure(in.RequestID, domain.Invalid("exact graph guidance owner"))
	}
	return s.mutate(ctx, in.RequestID, "graphs.nodes.guidance", GraphGuidanceFingerprint(graphID, nodeKey, in), func(tx *sql.Tx) (int, any, error) {
		h, e := s.graphHeader(ctx, tx, graphID)
		if e != nil {
			return 0, nil, e
		}
		gate, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if e = checkGate(gate); e != nil {
			return 0, nil, e
		}
		if gate.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "Use the current mutation owner for graph guidance.")
		}
		if h.Summary.Stop != nil || h.Summary.ControlRevision != in.ExpectedGraphControlRevision {
			return 0, nil, graphControlError()
		}
		if len(f.Preparations)+len(f.Admissions)+len(f.Stages) != 0 {
			return 0, nil, domain.Invalid("guidance without execution admissions")
		}
		nodes, e := s.graphNodes(ctx, tx, graphID)
		if e != nil {
			return 0, nil, e
		}
		index := -1
		for i := range nodes {
			if nodes[i].Summary.ID == in.NodeID && nodes[i].Summary.Key == nodeKey {
				index = i
			}
		}
		if index < 0 {
			return 0, nil, graphControlError()
		}
		n := nodes[index].Summary
		t, e := s.task(ctx, tx, n.TaskID)
		if e != nil {
			return 0, nil, e
		}
		if n.Stop != nil || n.ControlRevision != in.ExpectedNodeControlRevision || t.Revision != in.ExpectedRevision || t.CurrentVersion != in.TaskVersion || n.TaskVersion != in.TaskVersion {
			return 0, nil, graphControlError()
		}
		v, e := s.version(ctx, tx, t.ID, t.CurrentVersion)
		if e != nil {
			return 0, nil, e
		}
		options, e := s.graphNodeOptions(ctx, tx, graphID, n.ID, v.Version)
		if e != nil {
			return 0, nil, e
		}
		basis, e := s.graphGuidanceRoutingBasis(ctx, tx, h, options, in.Routing)
		if e != nil {
			return 0, nil, e
		}
		priorWorkflow, e := s.selectedWorkflow(ctx, tx, t.ID, domain.WorkflowSelector{})
		if e != nil {
			return 0, nil, e
		}
		workflowOptions := options.Workflow
		if priorWorkflow.Workflow != nil {
			captured := priorWorkflow.Workflow.Authority.OptionSnapshot
			workflowOptions = &captured
		}
		patched := graphWorkflowGuidancePatch(workflowOptions, in.Workflow)
		options.Workflow, options.RoutingBasis = &patched, &basis
		if in.Routing != nil {
			options.Routing = in.Routing
		}
		v.Version, e = increment(v.Version)
		if e != nil {
			return 0, nil, e
		}
		v.CreatedAt, v.CreatedBy, v.GuidanceText = domain.Now(), "user_guidance", in.GuidanceText
		if in.UseCurrentPolicy {
			v.RequestedPolicy, e = s.policy(ctx, tx)
			if e != nil {
				return 0, nil, e
			}
		}
		if in.ClearPolicyOverride {
			v.TaskPolicyOverride = nil
		} else if in.PolicyOverride != "" {
			v.TaskPolicyOverride = ptr(in.PolicyOverride)
		}
		if in.Pins != nil {
			v.Pins = *in.Pins
		}
		if in.Acceptance != nil {
			v.Acceptance, _ = domain.ResolveAcceptance(in.Acceptance)
		}
		if in.TimeoutSeconds != nil {
			v.TimeoutSeconds = *in.TimeoutSeconds
		}
		options.TaskVersion = v.Version
		encoded := encode(graphTaskVersionRecord{TaskVersion: v, GraphOptions: &options})
		if len(encoded) > maxGraphVersionRecordBytes {
			return 0, nil, domain.Fail("input_too_large", "The complete immutable graph version exceeds 64 MiB.")
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO task_versions VALUES(?,?,?)", t.ID, intRevision(v.Version), encoded); e != nil {
			return 0, nil, storageError()
		}
		t.CurrentVersion = v.Version
		t.Revision, e = increment(t.Revision)
		if e != nil {
			return 0, nil, e
		}
		t.StateRevision, e = increment(t.StateRevision)
		if e != nil {
			return 0, nil, e
		}
		t.UpdatedAt = v.CreatedAt
		if e = saveTask(ctx, tx, t); e != nil {
			return 0, nil, e
		}
		definition, e := s.graphDefinition(ctx, tx, graphID, h.Summary.GraphVersion)
		if e != nil {
			return 0, nil, e
		}
		affected := map[string]bool{n.Key: true}
		if definition.Original.Dependencies != nil {
			for range nodes {
				for _, edge := range *definition.Original.Dependencies {
					if affected[edge.From] {
						affected[edge.To] = true
					}
				}
			}
		}
		removed := map[string]bool{}
		for i := range nodes {
			item := &nodes[i]
			if !affected[item.Summary.Key] {
				continue
			}
			removed[item.Summary.ID] = true
			if item.Summary.InputID != nil {
				if e = s.staleGraphInput(ctx, tx, graphID, *item.Summary.InputID, v.CreatedAt); e != nil {
					return 0, nil, e
				}
			}
			item.Summary.ControlRevision, e = increment(item.Summary.ControlRevision)
			if e != nil {
				return 0, nil, e
			}
			item.Summary.StateRevision, e = increment(item.Summary.StateRevision)
			if e != nil {
				return 0, nil, e
			}
			item.Summary.AcceptedResult = nil
			preserveTerminal := item.Summary.ID != n.ID && (item.Summary.Stop != nil || item.Summary.State == "failed" || item.Summary.State == "interrupted" || item.Summary.State == "cancelled")
			if !preserveTerminal {
				item.Summary.State, item.Summary.NextOwner = "blocked", "user"
				item.Summary.NextAction = "Advance explicitly with current accepted dependencies and this guidance version."
				item.Blocker = &domain.GraphBlockerSummary{Code: "graph_input_stale", Message: "Guidance changed a required current condition or dependency.", NodeID: &item.Summary.ID, InputID: item.Summary.InputID}
			}
			if item.Summary.ID == n.ID {
				item.Summary.TaskVersion, item.Summary.TaskRevision = v.Version, t.Revision
				item.Summary.InputID, item.Summary.NodeAdmissionID = nil, nil
				item.Blocker.InputID = nil
			}
			if e = saveGraphNode(ctx, tx, *item); e != nil {
				return 0, nil, e
			}
		}
		if h.DispatchAuthority != nil {
			kept := []domain.GraphNodeAuthority{}
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
			return 0, nil, e
		}
		h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
		if e != nil {
			return 0, nil, e
		}
		h.Summary.State, h.Summary.NextOwner = "blocked", "user"
		h.Summary.NextAction, h.Summary.UpdatedAt = "Review the changed node scope and explicitly Advance when ready.", v.CreatedAt
		if e = saveGraphHeader(ctx, tx, h); e != nil {
			return 0, nil, e
		}
		latest, e := s.latest(ctx, tx, t)
		if e != nil {
			return 0, nil, e
		}
		return 200, domain.GraphGuidanceResult{Graph: h.Summary, Node: nodes[index].Summary, Result: domain.GuidanceResult{Task: taskSummary(t, v, latest, false), Version: v}}, nil
	})
}

func (s *Store) staleGraphInput(ctx context.Context, tx *sql.Tx, graphID, inputID, now string) error {
	record, e := s.graphInputRecord(ctx, tx, graphID, inputID)
	if e != nil {
		return e
	}
	if !record.Public.Summary.Current {
		return nil
	}
	record.Public.Summary.Current, record.Public.Summary.UpdatedAt = false, now
	if _, e = tx.ExecContext(ctx, "UPDATE graph_inputs SET data=? WHERE id=?", encode(record), inputID); e != nil {
		return storageError()
	}
	return nil
}
