package store

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type graphStageScope struct {
	Header               graphHeader
	Node                 graphNodeRecord
	Input                domain.GraphInputRecord
	Anchor               domain.GraphDispatchRecord
	Facts                domain.GraphStageAdmissionFacts
	OwnerResumeRequestID *string
}

func (s *Store) graphWorkflowStageScope(ctx context.Context, tx *sql.Tx, ownerID string, t domain.Task, w domain.Workflow, b *domain.TaskBudget, f domain.WorkflowStepAdmissionFacts) (*graphStageScope, error) {
	member, e := s.graphMembership(ctx, tx, t.ID)
	if e != nil || member == nil {
		return nil, e
	}
	if member.Kind != "node" || member.NodeID == nil || f.Graph == nil {
		return nil, graphScopeError(member)
	}
	gf := *f.Graph
	c := gf.Control
	h, e := s.graphHeader(ctx, tx, member.GraphID)
	if e != nil {
		return nil, e
	}
	gate, e := s.gate(ctx, tx)
	if e != nil {
		return nil, e
	}
	if e = checkGate(gate); e != nil {
		return nil, e
	}
	if gate.OwnerInstanceID != ownerID || h.Summary.Stop != nil || h.Summary.GraphVersion != c.GraphVersion || h.Summary.ControlRevision != c.GraphControlRevision || h.Summary.BudgetAuthorityEpoch != c.BudgetAuthorityEpoch || c.GraphID != member.GraphID || c.RootTaskID != member.RootTaskID || c.OwnerInstanceID != ownerID || c.NodeID == nil || *c.NodeID != *member.NodeID || c.TaskID == nil || *c.TaskID != t.ID || c.TaskVersion == nil || *c.TaskVersion != t.CurrentVersion || c.NodeControlRevision == nil || c.InputID == nil || b == nil || c.WindowNumber != b.CurrentWindow.Number {
		return nil, graphControlError()
	}
	nodes, e := s.graphNodes(ctx, tx, member.GraphID)
	if e != nil {
		return nil, e
	}
	var node *graphNodeRecord
	for i := range nodes {
		if nodes[i].Summary.ID == *member.NodeID {
			node = &nodes[i]
			break
		}
	}
	if node == nil {
		return nil, notFound()
	}
	n := node.Summary
	if n.Stop != nil || n.TaskVersion != t.CurrentVersion || n.ControlRevision != *c.NodeControlRevision || n.InputID == nil || *n.InputID != *c.InputID || n.NodeAdmissionID == nil || *n.NodeAdmissionID != gf.NodeAdmissionID {
		return nil, graphControlError()
	}
	anchor, e := s.graphDispatchRecord(ctx, tx, h.Summary.ID, gf.NodeAdmissionID)
	if e != nil {
		return nil, e
	}
	anchor, e = s.graphNodeAdmissionAnchor(ctx, tx, anchor.Public.Dispatch)
	if e != nil {
		return nil, e
	}
	d := anchor.Public.Dispatch
	if d.NodeID != n.ID || d.TaskID != t.ID || d.TaskVersion != t.CurrentVersion || d.WorkflowID != w.ID || d.InputID != *c.InputID {
		return nil, evidenceUnavailable()
	}
	if w.CurrentStepID == nil {
		return nil, graphControlError()
	}
	previous, e := s.workflowStep(ctx, tx, *w.CurrentStepID)
	if e != nil {
		return nil, e
	}
	if previous.AttemptID == nil || *previous.AttemptID != c.OperationAttemptID {
		return nil, graphControlError()
	}
	actual, e := s.attempt(ctx, tx, c.OperationAttemptID)
	if e != nil {
		return nil, e
	}
	if actual.Phase != "settled" || actual.Cleanup.State != "verified" {
		return nil, graphControlError()
	}
	predecessor, e := s.graphDispatchForAttempt(ctx, tx, actual.ID)
	if e != nil {
		return nil, e
	}
	pd := predecessor.Public.Dispatch
	if pd.GraphID != d.GraphID || pd.NodeID != d.NodeID || pd.TaskID != t.ID || pd.TaskVersion != t.CurrentVersion || pd.WorkflowID != w.ID || pd.InputID != d.InputID || pd.NodeAdmissionID != d.ID || pd.StepID != previous.ID || pd.OwnerInstanceID != actual.OwnerInstanceID {
		return nil, evidenceUnavailable()
	}
	resumeRequest := predecessor.OwnerResumeRequestID
	if gf.Trigger.Kind == "explicit" {
		resumeRequest = ptr(gf.Trigger.ID)
	} else {
		if actual.OwnerInstanceID != ownerID {
			return nil, graphControlError()
		}
		if e = s.graphDispatchOwnerAuthorization(ctx, tx, d, pd); e != nil {
			return nil, e
		}
	}
	switch gf.Trigger.Kind {
	case "completion":
		// Existing005 predecessor proof owns this continuation. A separate node's
		// newer live-registration highwater cannot invalidate that admitted sibling.
		if gf.Trigger.ID != actual.ID {
			return nil, graphControlError()
		}
	case "claim_release", "explicit":
		if h.DispatchAuthority == nil {
			return nil, graphControlError()
		}
		if e = s.graphFreshTrigger(ctx, tx, h, ownerID, gf.Trigger, n.ID); e != nil {
			return nil, e
		}
	default:
		return nil, domain.Invalid("fresh graph stage trigger")
	}
	input, e := s.graphInputRecord(ctx, tx, h.Summary.ID, *c.InputID)
	if e != nil {
		return nil, e
	}
	if !input.Public.Summary.Current || input.Public.Summary.State != "ready" || input.Public.InputBinding == nil || input.Public.ConditionsRef == nil || input.ConditionsBinding == nil || !reflect.DeepEqual(*input.Public.InputBinding, gf.InputBinding) || !reflect.DeepEqual(c.ContributorVector, gf.InputBinding.ContributorVector) {
		return nil, evidenceUnavailable()
	}
	if e = s.graphContributorsCurrent(ctx, tx, h.Summary.ID, gf.InputBinding.ContributorVector); e != nil {
		return nil, e
	}
	// This admitted workflow's predecessor is already later than its received
	// input/acknowledgements; a capacity wake must also be later than that
	// predecessor. Do not reselect ancestor heads here: their later Stop may
	// block new receivers while this node's known-first-GO authority continues.
	if anchor.KnownFirstGOAt == nil {
		definition, e := s.graphDefinition(ctx, tx, h.Summary.ID, h.Summary.GraphVersion)
		if e != nil {
			return nil, e
		}
		ancestors := map[string]bool{n.Key: true}
		if definition.Original.Dependencies != nil {
			for i := 0; i < len(nodes); i++ {
				for _, edge := range *definition.Original.Dependencies {
					if ancestors[edge.To] {
						ancestors[edge.From] = true
					}
				}
			}
		}
		for _, candidate := range nodes {
			if candidate.Summary.ID != n.ID && ancestors[candidate.Summary.Key] && candidate.Summary.Stop != nil {
				return nil, graphControlError()
			}
		}
	}
	for _, pair := range []struct {
		ref  domain.GraphArtifactRef
		kind string
	}{{gf.InputBinding.Manifest, "graph_input_manifest"}, {gf.InputBinding.Index, "index_snapshot"}, {gf.InputBinding.StagedBlobs, "graph_index_blobs"}, {gf.InputBinding.Provenance, "graph_input_provenance"}, {gf.InputBinding.ContextPacket, "graph_context"}, {*input.Public.ConditionsRef, "graph_conditions"}} {
		if e = s.verifyGraphRef(ctx, tx, pair.ref, gf.VerifiedArtifacts, pair.kind); e != nil {
			return nil, e
		}
	}
	basis := gf.SourceBasis
	if basis.WorkflowID != "" && basis.WorkflowID != w.ID || basis.StepID != "" || basis.InputID != d.InputID || basis.InputSHA256 != d.InputSHA256 || !graphSHA256(basis.ExpectedWorkingEntriesSHA256) || !graphSHA256(basis.ExpectedIndexEntriesSHA256) {
		return nil, evidenceUnavailable()
	}
	if f.Plan.Role == "author" {
		if basis.BasisKind != "graph_input" || basis.CandidateBinding != nil || basis.ResponseBinding != nil || basis.ExpectedManifest != gf.InputBinding.Manifest || basis.ExpectedIndex != gf.InputBinding.Index {
			return nil, evidenceUnavailable()
		}
	} else {
		if basis.BasisKind != "node_subject" {
			return nil, evidenceUnavailable()
		}
		switch {
		case w.SelectedCandidateID != nil:
			binding := basis.CandidateBinding
			if binding == nil || basis.ResponseBinding != nil || binding.CandidateID != *w.SelectedCandidateID || basis.ExpectedManifest.ArtifactID != binding.ManifestArtifactID || basis.ExpectedManifest.SHA256 != binding.ManifestSHA256 || basis.ExpectedIndex.ArtifactID != binding.IndexArtifactID || basis.ExpectedIndex.SHA256 != binding.IndexSHA256 {
				return nil, evidenceUnavailable()
			}
			if e = s.candidateBinding(ctx, tx, w, *binding, gf.VerifiedArtifacts); e != nil {
				return nil, e
			}
			if e = s.verifyGraphRef(ctx, tx, basis.ExpectedManifest, gf.VerifiedArtifacts, "candidate_manifest"); e != nil {
				return nil, e
			}
		case w.SelectedResponseBinding != nil:
			binding := basis.ResponseBinding
			if binding == nil || basis.CandidateBinding != nil || !reflect.DeepEqual(*binding, *w.SelectedResponseBinding) || basis.ExpectedManifest.SHA256 != binding.ManifestSHA256 || basis.ExpectedIndex.ArtifactID != binding.IndexArtifactID || basis.ExpectedIndex.SHA256 != binding.IndexSHA256 {
				return nil, evidenceUnavailable()
			}
			if e = s.responseBinding(ctx, tx, w, *binding, gf.VerifiedArtifacts); e != nil {
				return nil, e
			}
			source, e := s.sourceSnapshot(ctx, tx, binding.SourceSnapshotID)
			if e != nil {
				return nil, e
			}
			if basis.ExpectedManifest.ArtifactID != source.ManifestArtifactID {
				return nil, evidenceUnavailable()
			}
			if e = s.verifyGraphRef(ctx, tx, basis.ExpectedManifest, gf.VerifiedArtifacts, "source_manifest"); e != nil {
				return nil, e
			}
		default:
			return nil, evidenceUnavailable()
		}
		if e = s.verifyGraphRef(ctx, tx, basis.ExpectedIndex, gf.VerifiedArtifacts, "candidate_index", "index_snapshot"); e != nil {
			return nil, e
		}
	}
	if e = s.graphStageFinalRef(ctx, tx, w, basis, gf.VerifiedArtifacts, true); e != nil {
		return nil, e
	}
	return &graphStageScope{Header: h, Node: *node, Input: input, Anchor: anchor, Facts: gf, OwnerResumeRequestID: resumeRequest}, nil
}

func (s *Store) registerGraphWorkflowStage(ctx context.Context, tx *sql.Tx, scope graphStageScope, t domain.Task, w domain.Workflow, step domain.WorkflowStep, a domain.Attempt, b domain.TaskBudget) (domain.GraphDispatchView, error) {
	var out domain.GraphDispatchView
	h, n, gf := scope.Header, scope.Node, scope.Facts
	source := gf.SourceBasis
	source.WorkflowID = w.ID
	source.StepID = step.ID
	d := domain.GraphDispatch{ID: domain.NewID(), GraphID: h.Summary.ID, GraphVersion: h.Summary.GraphVersion, NodeID: n.Summary.ID, TaskID: t.ID, TaskVersion: t.CurrentVersion, NodeAdmissionID: scope.Anchor.Public.Dispatch.ID, WorkflowID: w.ID, StepID: step.ID, AttemptID: a.ID, ExecutorKind: a.ExecutorKind, Role: a.Role, InputID: gf.InputBinding.ID, InputSHA256: gf.InputBinding.InputSHA256, ContextPacket: gf.InputBinding.ContextPacket, ContributorVector: gf.InputBinding.ContributorVector, OwnerInstanceID: a.OwnerInstanceID, GraphControlRevision: h.Summary.ControlRevision, NodeControlRevision: n.Summary.ControlRevision, BudgetOwnerTaskID: h.Summary.RootTaskID, BudgetAuthorityEpoch: h.Summary.BudgetAuthorityEpoch, WindowNumber: b.CurrentWindow.Number, ObservedBudgetRevision: b.Revision, SourceBasis: source, FreshTriggerID: gf.Trigger.ID, LogicalSlot: step.LogicalKey, ExecutionOrdinal: step.ExecutionOrdinal, CreatedAt: a.AdmittedAt}
	if a.ExecutorKind == "native" {
		d.NativeConsumptionKey = ptr(a.ID)
	}
	seq, e := nextSequence(ctx, tx, "graph_dispatches")
	if e != nil {
		return out, e
	}
	out.Dispatch = d
	record := domain.GraphDispatchRecord{Sequence: domain.Decimal(seq), Public: out, OwnerResumeRequestID: scope.OwnerResumeRequestID}
	consumption := Fingerprint("graphs.workflow.stage", struct{ Graph, Node, Version, Input, Slot, Execution string }{d.GraphID, d.NodeID, d.TaskVersion, d.InputID, d.LogicalSlot, d.ExecutionOrdinal})
	if _, e = tx.ExecContext(ctx, "INSERT INTO graph_dispatches(sequence,id,graph_id,graph_version,node_id,task_id,task_version,input_id,attempt_id,workflow_id,step_id,node_admission_id,logical_slot,execution_ordinal,owner_instance_id,graph_control_revision,node_control_revision,budget_epoch,window_number,fresh_trigger_id,consumption_key,native_consumption_key,data) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", seq, d.ID, d.GraphID, intRevision(d.GraphVersion), d.NodeID, d.TaskID, intRevision(d.TaskVersion), d.InputID, d.AttemptID, d.WorkflowID, d.StepID, d.NodeAdmissionID, d.LogicalSlot, intRevision(d.ExecutionOrdinal), d.OwnerInstanceID, intRevision(d.GraphControlRevision), intRevision(d.NodeControlRevision), intRevision(d.BudgetAuthorityEpoch), intRevision(d.WindowNumber), d.FreshTriggerID, consumption, d.NativeConsumptionKey, encode(record)); e != nil {
		return out, storageError()
	}
	if e = s.claimCapacity(ctx, tx, a, a.OwnerInstanceID); e != nil {
		return out, e
	}
	claims, e := s.graphClaims(ctx, tx)
	if e != nil {
		return out, e
	}
	slot := ""
	for _, claim := range claims {
		if claim.AttemptID == a.ID {
			slot = claim.Slot
		}
	}
	if slot == "" {
		return out, storageError()
	}
	if _, e = s.appendEvent(ctx, tx, a.ID, "capacity_reserved", jsontext.Value(encode(struct {
		Owner string `json:"ownerInstanceId"`
		Slot  string `json:"slot"`
	}{a.OwnerInstanceID, slot}))); e != nil {
		return out, e
	}
	if a.ExecutorKind == "native" {
		debits, invalid := domain.AddRoutingCounts(n.NativeDebits, "1")
		if invalid != nil {
			return out, invalid
		}
		n.NativeDebits = debits
	}
	n.Summary.LatestAttemptID = ptr(a.ID)
	n.Summary.TaskRevision = t.Revision
	n.Summary.State = "queued"
	n.Summary.NextOwner = "council"
	n.Summary.NextAction = "Observe the exact admitted workflow execution."
	n.Blocker = nil
	n.Summary.StateRevision, e = increment(n.Summary.StateRevision)
	if e != nil {
		return out, e
	}
	if e = saveGraphNode(ctx, tx, n); e != nil {
		return out, e
	}
	nodes, e := s.graphNodes(ctx, tx, h.Summary.ID)
	if e != nil {
		return out, e
	}
	h.Summary.Counts, e = graphCounts(nodes)
	if e != nil {
		return out, e
	}
	h.Summary.State = "queued"
	h.Summary.NextOwner = "council"
	h.Summary.NextAction = "Observe admitted graph workflow work."
	h.Summary.UpdatedAt = a.AdmittedAt
	h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
	if e != nil {
		return out, e
	}
	if e = saveGraphHeader(ctx, tx, h); e != nil {
		return out, e
	}
	return out, nil
}
