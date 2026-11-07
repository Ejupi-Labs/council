package store

import (
	"context"
	"database/sql"
	"errors"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Two independently retained 4 MiB text fields can each expand sixfold in
// JSON. 64 MiB covers those 48 MiB plus the bounded workflow/routing/policy
// envelope; this is independent of the 2 MiB graph declaration limit.
const maxGraphVersionRecordBytes = 64 << 20

// Only newly appended graph task versions use the private extension. Existing
// version readers decode the public TaskVersion and preserve historical bytes.
type graphTaskVersionRecord struct {
	domain.TaskVersion
	GraphOptions     *domain.GraphNodeExecutionOptions `json:"graphOptions,omitzero"`
	RebindProvenance *domain.GraphRebindProvenance     `json:"rebindProvenance,omitzero"`
}

func (s *Store) GraphNodeOptions(ctx context.Context, graphID, nodeID, taskVersion string) (domain.GraphNodeExecutionOptions, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return domain.GraphNodeExecutionOptions{}, e
	}
	out, e := s.graphNodeOptions(ctx, s.db, graphID, nodeID, taskVersion)
	return out, beforeCommitError(ctx, e)
}

func (s *Store) graphNodeOptions(ctx context.Context, q queryer, graphID, nodeID, taskVersion string) (domain.GraphNodeExecutionOptions, error) {
	var out domain.GraphNodeExecutionOptions
	if !domain.ValidID(graphID) || !domain.ValidID(nodeID) {
		return out, domain.Invalid("exact graph node options membership")
	}
	version, invalid := domain.ParseRevision(taskVersion)
	if invalid != nil {
		return out, invalid
	}
	h, e := s.graphHeader(ctx, q, graphID)
	if e != nil {
		return out, e
	}
	nodes, e := s.graphNodes(ctx, q, graphID)
	if e != nil {
		return out, e
	}
	var node *graphNodeRecord
	for i := range nodes {
		if nodes[i].Summary.ID == nodeID {
			node = &nodes[i]
			break
		}
	}
	if node == nil {
		return out, notFound()
	}
	var bounded sql.NullString
	// Refuse oversized corrupt rows inside SQLite rather than first copying an
	// unbounded value into the controller. CAST measures actual UTF-8 bytes.
	if e = q.QueryRowContext(ctx, "SELECT CASE WHEN length(CAST(data AS BLOB))<=? THEN data END FROM task_versions WHERE task_id=? AND version=?", maxGraphVersionRecordBytes, node.Summary.TaskID, version).Scan(&bounded); errors.Is(e, sql.ErrNoRows) {
		return out, notFound()
	} else if e != nil {
		return out, storageError()
	}
	if !bounded.Valid || len(bounded.String) > maxGraphVersionRecordBytes {
		return out, domain.Fail("evidence_unavailable", "The complete private graph version options exceed the supported 64 MiB bound.")
	}
	raw := bounded.String
	var record graphTaskVersionRecord
	if e = json.Unmarshal([]byte(raw), &record, json.RejectUnknownMembers(true)); e != nil || record.TaskID != node.Summary.TaskID || record.Version != taskVersion {
		return out, storageError()
	}
	if record.CreatedBy == "dependency_rebind" {
		p := record.RebindProvenance
		if p == nil || p.Kind != "dependency_rebind" || !domain.ValidID(p.RequestID) || !domain.ValidID(p.PreviousInputID) || !graphSHA256(p.ContributorVectorSHA256) {
			return out, evidenceUnavailable()
		}
	} else if record.RebindProvenance != nil {
		return out, evidenceUnavailable()
	}
	definition, e := s.graphDefinition(ctx, q, graphID, h.Summary.GraphVersion)
	if e != nil {
		return out, e
	}
	var original *domain.GraphNodeInput
	for i := range definition.Original.Nodes {
		if definition.Original.Nodes[i].Key == node.Summary.Key {
			original = &definition.Original.Nodes[i]
			break
		}
	}
	if original == nil {
		return out, storageError()
	}
	if record.GraphOptions == nil {
		if taskVersion != "1" {
			return out, domain.Fail("evidence_unavailable", "This graph task version lacks its immutable captured execution options.")
		}
		out = domain.GraphNodeExecutionOptions{GraphID: graphID, NodeID: nodeID, TaskID: node.Summary.TaskID, TaskVersion: taskVersion, ExecutionMode: definition.Original.ExecutionMode, FakeScenario: original.FakeScenario, Workflow: original.Workflow, Routing: original.Routing}
	} else {
		out = *record.GraphOptions
	}
	if out.GraphID != graphID || out.NodeID != nodeID || out.TaskID != node.Summary.TaskID || out.TaskVersion != taskVersion || out.ExecutionMode != definition.Original.ExecutionMode || out.FakeScenario != original.FakeScenario {
		return out, storageError()
	}
	if invalid := domain.ValidateExecutionMode(out.ExecutionMode, out.FakeScenario); invalid != nil {
		return out, invalid
	}
	// Persisted choices may originate in Start or Guidance. The executor kind
	// is not an operation context, and inherited choices retain their original
	// vocabulary rather than replaying a policy-refresh command here.
	if invalid := domain.ValidateRoutingOptions(out.Routing, "start"); invalid != nil {
		if guidanceInvalid := domain.ValidateRoutingOptions(out.Routing, "guidance"); guidanceInvalid != nil {
			return out, invalid
		}
	}
	if out.Routing != nil && out.Routing.Budget != nil {
		return out, domain.Invalid("graph-owned shared budget options")
	}
	if invalid := domain.ValidateWorkflowInput(out.Workflow); invalid != nil {
		return out, invalid
	}
	if out.Workflow != nil && (out.Workflow.Delivery != nil || out.Workflow.Mode == "author_only") {
		return out, domain.Invalid("graph-owned Auto workflow and delivery scope")
	}
	if out.RoutingBasis != nil {
		basis := out.RoutingBasis
		if _, invalid := domain.ParseRevision(basis.PolicySnapshot.Revision); invalid != nil {
			return out, invalid
		}
		if basis.OptionSnapshot.Budget != nil {
			return out, domain.Invalid("root-owned graph budget instead of a node routing budget")
		}
		if _, invalid := effectiveRoutingPolicy(basis.PolicySnapshot, basis.OptionSnapshot); invalid != nil {
			return out, invalid
		}
		if expected := basis.OptionSnapshot.ExpectedPolicyRevision; expected != "" && expected != basis.PolicySnapshot.Revision {
			return out, evidenceUnavailable()
		}
	}
	return out, nil
}
