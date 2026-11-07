package store

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These hooks run in the actual outcome transaction. Reads never manufacture
// acceptance, and current projections never rewrite old receipts or terminals.
func (s *Store) projectGraphWorkflow(ctx context.Context, tx *sql.Tx, t domain.Task, w *domain.Workflow, proofs []domain.VerifiedResponse) error {
	member, e := s.graphMembership(ctx, tx, t.ID)
	if e != nil || member == nil {
		return e
	}
	if member.Kind != "node" || member.NodeID == nil {
		return graphScopeError(member)
	}
	if w.TaskID != t.ID {
		return evidenceUnavailable()
	}
	if w.TaskVersion != t.CurrentVersion {
		return nil
	}
	h, e := s.graphHeader(ctx, tx, member.GraphID)
	if e != nil {
		return e
	}
	nodes, e := s.graphNodes(ctx, tx, member.GraphID)
	if e != nil {
		return e
	}
	index := -1
	for i := range nodes {
		if nodes[i].Summary.ID == *member.NodeID {
			index = i
			break
		}
	}
	if index < 0 {
		return notFound()
	}
	n := &nodes[index]
	if n.Summary.TaskVersion != t.CurrentVersion {
		return domain.Fail("graph_input_stale", "The graph node does not identify this current workflow version.")
	}
	accepted := w.State == "completed"
	if !accepted && h.Summary.Stop == nil && n.Summary.Stop == nil && n.Summary.InputID != nil {
		input, e := s.graphInputRecord(ctx, tx, member.GraphID, *n.Summary.InputID)
		if e != nil {
			return e
		}
		stale := !input.Public.Summary.Current || input.Public.Summary.State != "ready" || input.Public.InputBinding == nil
		if !stale {
			if e = s.graphContributorsCurrent(ctx, tx, member.GraphID, input.Public.InputBinding.ContributorVector); e != nil {
				if domain.SafeError(e).Code != "graph_input_stale" {
					return e
				}
				stale = true
			}
		}
		if stale {
			w.State, w.NextOwner, w.NextAction = "blocked", "user", "graph_input_stale"
			if e = saveWorkflow(ctx, tx, *w); e != nil {
				return e
			}
		}
	}
	if accepted && (h.Summary.Stop != nil || n.Summary.Stop != nil) {
		return graphControlError()
	}
	if accepted {
		ref, e := s.graphAcceptedResult(ctx, tx, h, *n, *w, proofs)
		if e != nil {
			return e
		}
		n.Summary.AcceptedResult = &ref
		n.Summary.State = "completed"
	} else if h.Summary.Stop != nil || n.Summary.Stop != nil {
		// Stop owns the current scope; a later local/worker observation cannot
		// relabel retained failed/completed outcomes or reopen cancelled work.
		if !graphRetainedTerminal(n.Summary.State) {
			n.Summary.State = "cancelled"
		}
	} else {
		n.Summary.AcceptedResult = nil
		switch {
		case w.NextAction == "capacity_wait":
			n.Summary.State = "ready"
		case w.NextOwner == "council":
			current, e := s.latest(ctx, tx, t)
			if e != nil {
				return e
			}
			n.Summary.State = "ready"
			if current != nil && current.Phase != "settled" {
				n.Summary.State = "queued"
				if current.Phase == "preparing" {
					n.Summary.State = "preparing"
				}
				if current.Phase == "running" {
					n.Summary.State = "running"
				}
				if current.Phase == "cancelling" {
					n.Summary.State = "cancelling"
				}
			}
		case w.State == "failed" || w.State == "interrupted" || w.State == "cancelled":
			n.Summary.State = w.State
		default:
			n.Summary.State = "blocked"
		}
	}
	n.Summary.TaskRevision = t.Revision
	n.Summary.LatestAttemptID = t.LatestAttemptID
	if h.Summary.Stop == nil && n.Summary.Stop == nil {
		n.Summary.NextAction = w.NextAction
		n.Summary.NextOwner = w.NextOwner
	}
	n.Summary.StateRevision, e = increment(n.Summary.StateRevision)
	if e != nil {
		return e
	}
	if e = saveGraphNode(ctx, tx, *n); e != nil {
		return e
	}
	h.Summary.Counts, e = graphCounts(nodes)
	if e != nil {
		return e
	}
	all := true
	remaining := false
	anyFailed, anyInterrupted := false, false
	for _, node := range nodes {
		v := node.Summary
		if v.State != "completed" || v.AcceptedResult == nil || v.AcceptedResult.TaskVersion != v.TaskVersion {
			all = false
		}
		if v.NextOwner == "council" && !graphRetainedTerminal(v.State) {
			remaining = true
		}
		anyFailed = anyFailed || v.State == "failed"
		anyInterrupted = anyInterrupted || v.State == "interrupted"
	}
	switch {
	case h.Summary.Stop != nil:
		pending, e := s.graphPending(ctx, tx, h)
		if e != nil {
			return e
		}
		h.Summary.State = "cancelled"
		if len(pending) != 0 {
			h.Summary.State = "cancelling"
		}
		h.Summary.NextOwner = "user"
		h.Summary.NextAction = "Inspect stopped work and retained partial results."
	case all:
		h.Summary.State = "completed"
		h.Summary.NextOwner = "user"
		h.Summary.NextAction = "Inspect every current accepted node result and captured delivery outcome."
	case anyFailed:
		h.Summary.State = "failed"
		h.Summary.NextOwner = "user"
		h.Summary.NextAction = "Inspect the failed required node and retained independent work."
	case anyInterrupted:
		h.Summary.State = "interrupted"
		h.Summary.NextOwner = "user"
		h.Summary.NextAction = "Inspect interrupted required work before explicit resumption."
	case remaining:
		h.Summary.State = "queued"
		if h.Summary.Counts.Running != "0" {
			h.Summary.State = "running"
		}
		h.Summary.NextOwner = "council"
		h.Summary.NextAction = "Observe the remaining authorised node work."
	default:
		h.Summary.State = "blocked"
		h.Summary.NextOwner = "user"
		h.Summary.NextAction = "Review the current node result or resolve its recorded blocker."
	}
	if h.DispatchAuthority != nil && (all || !remaining) {
		h.DispatchAuthority.State = "suspended"
	}
	h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
	if e != nil {
		return e
	}
	h.Summary.UpdatedAt = domain.Now()
	return saveGraphHeader(ctx, tx, h)
}

func (s *Store) graphAcceptedResult(ctx context.Context, tx *sql.Tx, h graphHeader, n graphNodeRecord, w domain.Workflow, proofs []domain.VerifiedResponse) (domain.GraphAcceptedResultRef, error) {
	var ref domain.GraphAcceptedResultRef
	if n.Summary.InputID == nil || n.Summary.NodeAdmissionID == nil {
		return ref, evidenceUnavailable()
	}
	input, e := s.graphInputRecord(ctx, tx, h.Summary.ID, *n.Summary.InputID)
	if e != nil {
		return ref, e
	}
	if !input.Public.Summary.Current || input.Public.Summary.State != "ready" || input.Public.InputBinding == nil || input.Public.ConditionsRef == nil {
		return ref, domain.Fail("graph_input_stale", "The accepted node requires its exact current input and conditions.")
	}
	b := *input.Public.InputBinding
	if b.ReceivingTaskID != w.TaskID || b.ReceivingTaskVersion != w.TaskVersion || b.ReceivingNodeID != n.Summary.ID {
		return ref, evidenceUnavailable()
	}
	if e = s.graphContributorsCurrent(ctx, tx, h.Summary.ID, b.ContributorVector); e != nil {
		return ref, e
	}
	for _, pair := range []struct {
		ref  domain.GraphArtifactRef
		kind string
	}{{b.Manifest, "graph_input_manifest"}, {b.Index, "index_snapshot"}, {b.StagedBlobs, "graph_index_blobs"}, {b.Provenance, "graph_input_provenance"}, {b.ContextPacket, "graph_context"}, {*input.Public.ConditionsRef, "graph_conditions"}} {
		if e = s.verifyGraphRef(ctx, tx, pair.ref, proofs, pair.kind); e != nil {
			return ref, e
		}
	}
	var producer string
	var closureRef *domain.GraphArtifactRef
	var closure *domain.GraphOutputClosure
	switch {
	case w.SelectedCandidateID != nil && w.SelectedResponseBinding == nil:
		if w.LatestAcceptanceID == nil {
			return ref, evidenceUnavailable()
		}
		record, e := storedWorkflowRecord[domain.WorkflowAcceptance](ctx, tx, "workflow_acceptances", *w.LatestAcceptanceID)
		if e != nil {
			return ref, e
		}
		c, e := s.candidate(ctx, tx, *w.SelectedCandidateID)
		if e != nil {
			return ref, e
		}
		if record.ID != *w.LatestAcceptanceID || record.WorkflowID != w.ID || record.TaskID != w.TaskID || record.TaskVersion != w.TaskVersion || record.CandidateBinding.CandidateID != c.ID {
			return ref, evidenceUnavailable()
		}
		producer = c.AttemptID
		dispatch, e := s.graphDispatchForAttempt(ctx, tx, producer)
		if e != nil {
			return ref, e
		}
		o := dispatch.Public.OutputObservation
		if o == nil || o.Candidate == nil || o.Candidate.CandidateID != c.ID || o.Candidate.State != "complete" {
			return ref, evidenceUnavailable()
		}
		closureRef = o.Candidate.ClosureRef
		closure = dispatch.CandidateClosure
		ref = domain.GraphAcceptedResultRef{AcceptanceKind: "workflow", AcceptanceID: record.ID, TaskVersion: w.TaskVersion, ProducerAttemptID: producer, CandidateID: ptr(c.ID), InputID: b.ID}
	case w.SelectedResponseBinding != nil && w.SelectedCandidateID == nil:
		if e = s.graphResponseFinalAcceptance(ctx, tx, h, n, w, b); e != nil {
			return ref, e
		}
		binding := *w.SelectedResponseBinding
		if w.Completion == nil || !reflect.DeepEqual(w.Completion.ResponseBinding, binding) {
			return ref, evidenceUnavailable()
		}
		record, e := storedWorkflowRecord[domain.ResponseAcceptance](ctx, tx, "response_acceptances", w.Completion.ResponseAcceptanceID)
		if e != nil {
			return ref, e
		}
		if record.ID != w.Completion.ResponseAcceptanceID || record.TaskID != w.TaskID || record.TaskVersion != w.TaskVersion || record.AttemptID != binding.ProducerAttemptID || record.ResponseArtifactID != binding.ResponseArtifactID || record.ResponseSHA256 != binding.ResponseSHA256 {
			return ref, evidenceUnavailable()
		}
		producer = binding.ProducerAttemptID
		dispatch, e := s.graphDispatchForAttempt(ctx, tx, producer)
		if e != nil {
			return ref, e
		}
		o := dispatch.Public.OutputObservation
		if o == nil || o.Response == nil || o.Response.ResponseArtifactID != binding.ResponseArtifactID || o.Response.State != "complete" {
			return ref, evidenceUnavailable()
		}
		closureRef = o.Response.ClosureRef
		closure = dispatch.ResponseClosure
		if closure == nil || closure.Index.ArtifactID != binding.IndexArtifactID || closure.Index.SHA256 != binding.IndexSHA256 || closure.ResponseArtifact == nil || closure.ResponseArtifact.ArtifactID != binding.ResponseArtifactID || closure.ResponseArtifact.SHA256 != binding.ResponseSHA256 || closure.ResponseArtifact.ByteLength != binding.ResponseByteLength {
			return ref, evidenceUnavailable()
		}
		ref = domain.GraphAcceptedResultRef{AcceptanceKind: "response", AcceptanceID: record.ID, TaskVersion: w.TaskVersion, ProducerAttemptID: producer, ResponseArtifactID: ptr(binding.ResponseArtifactID), InputID: b.ID}
	default:
		return ref, evidenceUnavailable()
	}
	dispatch, e := s.graphDispatchForAttempt(ctx, tx, producer)
	if e != nil {
		return ref, e
	}
	d := dispatch.Public.Dispatch
	if d.GraphID != h.Summary.ID || d.NodeID != n.Summary.ID || d.TaskID != w.TaskID || d.TaskVersion != w.TaskVersion || d.WorkflowID != w.ID || d.InputID != b.ID || d.NodeAdmissionID != *n.Summary.NodeAdmissionID || d.GraphControlRevision != h.Summary.ControlRevision || d.NodeControlRevision != n.Summary.ControlRevision {
		return ref, graphControlError()
	}
	if closureRef == nil || closure == nil || !graphRefHasBytes(*closureRef, []byte(encode(closure))) {
		return ref, evidenceUnavailable()
	}
	if invalid := domain.ValidateGraphOutputClosure(*closure); invalid != nil {
		return ref, invalid
	}
	seal, e := domain.GraphOutputDigest(*closure)
	if e != nil || seal != closure.ClosureSHA256 {
		return ref, evidenceUnavailable()
	}
	if closure.GraphID != h.Summary.ID || closure.NodeID != n.Summary.ID || closure.ReceivedInputID != b.ID || closure.ProducerAttemptID != producer {
		return ref, evidenceUnavailable()
	}
	for _, pair := range []struct {
		ref   domain.GraphArtifactRef
		kinds []string
	}{{*closureRef, []string{"graph_output_closure"}}, {closure.Manifest, []string{"candidate_manifest", "source_manifest"}}, {closure.Index, []string{"candidate_index", "index_snapshot"}}, {closure.StagedBlobs, []string{"graph_index_blobs"}}} {
		if e = s.verifyGraphRef(ctx, tx, pair.ref, proofs, pair.kinds...); e != nil {
			return ref, e
		}
	}
	if h.Summary.FinalNodeID != nil && *h.Summary.FinalNodeID == n.Summary.ID {
		nodes, e := s.graphNodes(ctx, tx, h.Summary.ID)
		if e != nil {
			return ref, e
		}
		for _, other := range nodes {
			if other.Summary.ID == n.Summary.ID {
				continue
			}
			if other.Summary.State != "completed" || other.Summary.AcceptedResult == nil || other.Summary.AcceptedResult.TaskVersion != other.Summary.TaskVersion {
				return ref, domain.Fail("graph_dependencies_pending", "Every other required node must have a current accepted result before final acceptance.")
			}
		}
	}
	return ref, nil
}
