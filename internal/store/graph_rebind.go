package store

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Rebinding changes immutable task authority, not an existing workflow's input.
// It requires a freshly verified changed accepted vector and remains separate
// from native admission. The caller records the new explicit registration and
// actual input operations in this same transaction.
func (s *Store) rebindGraphNode(ctx context.Context, tx *sql.Tx, ownerID string, h graphHeader, nodes []graphNodeRecord, index int, requestID string, f domain.GraphInputPreparationFacts) (domain.GraphReboundNode, error) {
	var out domain.GraphReboundNode
	n := &nodes[index]
	c := f.Control
	budget, err := s.rawTaskBudget(ctx, tx, h.Summary.RootTaskID)
	if err != nil {
		return out, err
	}
	if h.DispatchAuthority == nil || budget == nil || h.Summary.Stop != nil || n.Summary.Stop != nil ||
		c.GraphID != h.Summary.ID || c.GraphVersion != h.Summary.GraphVersion || c.RootTaskID != h.Summary.RootTaskID || c.GraphControlRevision != h.Summary.ControlRevision ||
		c.BudgetAuthorityEpoch != h.Summary.BudgetAuthorityEpoch || c.WindowNumber != budget.CurrentWindow.Number || c.OwnerInstanceID != ownerID || c.Generation != h.DispatchAuthority.Generation ||
		c.NodeID == nil || *c.NodeID != n.Summary.ID || c.TaskID == nil || *c.TaskID != n.Summary.TaskID || c.TaskVersion == nil || *c.TaskVersion != n.Summary.TaskVersion || c.NodeControlRevision == nil || *c.NodeControlRevision != n.Summary.ControlRevision || c.InputID != nil ||
		!reflect.DeepEqual(c.ContributorVector, f.ContributorVector) {
		return out, graphControlError()
	}
	task, err := s.task(ctx, tx, n.Summary.TaskID)
	if err != nil {
		return out, err
	}
	if task.CurrentVersion != n.Summary.TaskVersion || n.Summary.InputID == nil || n.Summary.NodeAdmissionID == nil {
		return out, domain.Fail("graph_input_stale", "Only a node with retained stale workflow authority needs an explicit dependency rebind.")
	}
	old, err := s.graphInputRecord(ctx, tx, h.Summary.ID, *n.Summary.InputID)
	if err != nil {
		return out, err
	}
	if old.Public.InputBinding == nil || old.Public.ConditionsRef == nil || old.ConditionsBinding == nil || old.Public.InputBinding.ReceivingNodeID != n.Summary.ID || old.Public.InputBinding.ReceivingTaskID != task.ID || old.Public.InputBinding.ReceivingTaskVersion != task.CurrentVersion {
		return out, evidenceUnavailable()
	}
	workflow, err := s.selectedWorkflow(ctx, tx, task.ID, domain.WorkflowSelector{TaskVersion: task.CurrentVersion})
	if err != nil {
		return out, err
	}
	anchor, err := s.graphDispatchRecord(ctx, tx, h.Summary.ID, *n.Summary.NodeAdmissionID)
	if err != nil {
		return out, err
	}
	if workflow.Workflow == nil || anchor.Public.Dispatch.WorkflowID != workflow.Workflow.ID || anchor.Public.Dispatch.NodeID != n.Summary.ID || anchor.Public.Dispatch.TaskID != task.ID || anchor.Public.Dispatch.TaskVersion != task.CurrentVersion || anchor.Public.Dispatch.InputID != old.Public.Summary.ID {
		return out, evidenceUnavailable()
	}
	var unfinished int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM attempts WHERE task_id=? AND phase!='settled'", task.ID).Scan(&unfinished); err != nil {
		return out, storageError()
	}
	local, err := s.attempt(ctx, tx, old.Public.Summary.OperationAttemptID)
	if err != nil {
		return out, err
	}
	if unfinished != 0 || local.Phase != "settled" || local.Cleanup.State != "verified" {
		return out, domain.Fail("attempt_active", "Settle the exact owned receiver work before rebinding its dependency input.")
	}
	claims, err := s.graphClaims(ctx, tx)
	if err != nil {
		return out, err
	}
	for _, claim := range claims {
		if claim.NodeID != nil && *claim.NodeID == n.Summary.ID || claim.AttemptID == local.ID {
			return out, domain.Fail("attempt_active", "A retained receiver claim prevents dependency rebinding.")
		}
	}
	definition, err := s.graphDefinition(ctx, tx, h.Summary.ID, h.Summary.GraphVersion)
	if err != nil {
		return out, err
	}
	if err = s.graphContributorsCurrent(ctx, tx, h.Summary.ID, f.ContributorVector); err != nil {
		return out, err
	}
	_, vector, err := s.graphDependencySelection(ctx, tx, h, definition, nodes, *n, f)
	if err != nil {
		return out, err
	}
	if reflect.DeepEqual(vector, old.Public.InputBinding.ContributorVector) {
		return out, domain.Fail("graph_input_current", "The accepted dependency input is unchanged; explicit rebind cannot replay completed or failed work on the same subject.")
	}
	if h.Summary.RootInputID == nil {
		return out, evidenceUnavailable()
	}
	root, err := s.graphInputRecord(ctx, tx, h.Summary.ID, *h.Summary.RootInputID)
	if err != nil {
		return out, err
	}
	if root.PrivateRoot == nil || root.Public.Summary.State != "ready" || !reflect.DeepEqual(*root.PrivateRoot, f.RootBinding) {
		return out, evidenceUnavailable()
	}
	for _, pair := range []struct {
		ref  domain.GraphArtifactRef
		kind string
	}{{f.RootBinding.Manifest, "source_manifest"}, {f.RootBinding.Index, "index_snapshot"}, {f.RootBinding.StagedBlobs, "graph_index_blobs"}} {
		if err = s.verifyGraphRef(ctx, tx, pair.ref, f.VerifiedArtifacts, pair.kind); err != nil {
			return out, err
		}
	}
	version, err := s.version(ctx, tx, task.ID, task.CurrentVersion)
	if err != nil {
		return out, err
	}
	options, err := s.graphNodeOptions(ctx, tx, h.Summary.ID, n.Summary.ID, task.CurrentVersion)
	if err != nil {
		return out, err
	}
	basis, err := s.graphVersionRoutingBasis(ctx, tx, h, options)
	if err != nil {
		return out, err
	}
	digest, err := domain.GraphContributorVectorDigest(vector)
	if err != nil {
		return out, storageError()
	}
	provenance := domain.GraphRebindProvenance{Kind: "dependency_rebind", RequestID: requestID, PreviousInputID: *n.Summary.InputID, ContributorVectorSHA256: digest}
	out = domain.GraphReboundNode{NodeID: n.Summary.ID, TaskID: task.ID, FromTaskVersion: task.CurrentVersion, Provenance: provenance}
	version.Version, err = increment(version.Version)
	if err != nil {
		return out, err
	}
	version.CreatedBy, version.CreatedAt = "dependency_rebind", domain.Now()
	options.TaskVersion, options.RoutingBasis = version.Version, &basis
	encoded := encode(graphTaskVersionRecord{TaskVersion: version, GraphOptions: &options, RebindProvenance: &provenance})
	if len(encoded) > maxGraphVersionRecordBytes {
		return out, domain.Fail("input_too_large", "The complete immutable graph version exceeds 64 MiB.")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO task_versions VALUES(?,?,?)", task.ID, intRevision(version.Version), encoded); err != nil {
		return out, storageError()
	}
	task.CurrentVersion, task.UpdatedAt = version.Version, version.CreatedAt
	task.Revision, err = increment(task.Revision)
	if err != nil {
		return out, err
	}
	task.StateRevision, err = increment(task.StateRevision)
	if err != nil {
		return out, err
	}
	if err = saveTask(ctx, tx, task); err != nil {
		return out, err
	}
	if err = s.staleGraphInput(ctx, tx, h.Summary.ID, *n.Summary.InputID, version.CreatedAt); err != nil {
		return out, err
	}
	n.Summary.TaskVersion, n.Summary.TaskRevision = version.Version, task.Revision
	n.Summary.ControlRevision, err = increment(n.Summary.ControlRevision)
	if err != nil {
		return out, err
	}
	n.Summary.StateRevision, err = increment(n.Summary.StateRevision)
	if err != nil {
		return out, err
	}
	n.Summary.InputID, n.Summary.NodeAdmissionID, n.Summary.AcceptedResult = nil, nil, nil
	n.Summary.State, n.Summary.NextOwner = "blocked", "user"
	n.Summary.NextAction = "Retain the exact new dependency input before capturing its workflow."
	n.Blocker = &domain.GraphBlockerSummary{Code: "graph_input_stale", Message: "The explicitly derived version awaits its new accepted input.", NodeID: &n.Summary.ID}
	if err = saveGraphNode(ctx, tx, *n); err != nil {
		return out, err
	}
	out.ToTaskVersion = version.Version
	return out, nil
}
