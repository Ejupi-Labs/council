package store

import (
	"context"
	"database/sql"
	"reflect"
	"sort"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type graphSelectedHead struct {
	Vector   domain.GraphContributorBinding
	Input    domain.GraphInputRecord
	Workflow domain.Workflow
	Closure  domain.GraphOutputClosure
}

// Resolve the exact current accepted head, independently of the task's latest
// supporting attempt. A retained closure or historical acceptance alone is not a
// dependency. This metadata check is also used for transitive vector members;
// direct selected input/output bytes additionally require fresh proofs below.
func (s *Store) graphSelectedHead(ctx context.Context, q queryer, h graphHeader, n graphNodeRecord) (graphSelectedHead, error) {
	var out graphSelectedHead
	r := n.Summary.AcceptedResult
	if n.Summary.Stop != nil || n.Summary.State != "completed" || r == nil || r.TaskVersion != n.Summary.TaskVersion || n.Summary.InputID == nil || *n.Summary.InputID != r.InputID || n.Summary.NodeAdmissionID == nil {
		return out, domain.Fail("graph_dependency_unaccepted", "A required predecessor has no current accepted result.")
	}
	t, e := s.task(ctx, q, n.Summary.TaskID)
	if e != nil {
		return out, e
	}
	if t.CurrentVersion != r.TaskVersion {
		return out, domain.Fail("graph_input_stale", "The accepted predecessor version changed.")
	}
	view, e := s.selectedWorkflow(ctx, q, t.ID, domain.WorkflowSelector{})
	if e != nil {
		return out, e
	}
	if view.Workflow == nil || view.Workflow.State != "completed" || view.Workflow.TaskVersion != t.CurrentVersion {
		return out, evidenceUnavailable()
	}
	w := *view.Workflow
	v := domain.GraphContributorBinding{NodeID: n.Summary.ID, TaskID: t.ID, TaskVersion: t.CurrentVersion, AcceptanceKind: r.AcceptanceKind, AcceptanceID: r.AcceptanceID, ProducerAttemptID: r.ProducerAttemptID, InputID: r.InputID}
	switch r.AcceptanceKind {
	case "workflow":
		if w.LatestAcceptanceID == nil || *w.LatestAcceptanceID != r.AcceptanceID || w.SelectedCandidateID == nil || r.CandidateID == nil || *r.CandidateID != *w.SelectedCandidateID || r.ResponseArtifactID != nil {
			return out, evidenceUnavailable()
		}
		a, e := storedWorkflowRecord[domain.WorkflowAcceptance](ctx, q, "workflow_acceptances", r.AcceptanceID)
		if e != nil {
			return out, e
		}
		c, e := s.candidate(ctx, q, *r.CandidateID)
		if e != nil {
			return out, e
		}
		if a.ID != r.AcceptanceID || a.TaskID != t.ID || a.TaskVersion != t.CurrentVersion || a.WorkflowID != w.ID || a.CandidateBinding.CandidateID != c.ID || c.AttemptID != r.ProducerAttemptID || a.CandidateBinding.ConditionsSHA256 != w.Authority.ConditionsSHA256 {
			return out, evidenceUnavailable()
		}
		v.CandidateBinding = &a.CandidateBinding
	case "response":
		if w.Completion == nil || w.Completion.ResponseAcceptanceID != r.AcceptanceID || w.SelectedResponseBinding == nil || r.ResponseArtifactID == nil || r.CandidateID != nil || !reflect.DeepEqual(w.Completion.ResponseBinding, *w.SelectedResponseBinding) {
			return out, evidenceUnavailable()
		}
		b := *w.SelectedResponseBinding
		a, e := storedWorkflowRecord[domain.ResponseAcceptance](ctx, q, "response_acceptances", r.AcceptanceID)
		if e != nil {
			return out, e
		}
		if a.ID != r.AcceptanceID || a.TaskID != t.ID || a.TaskVersion != t.CurrentVersion || a.AttemptID != r.ProducerAttemptID || a.AttemptID != b.ProducerAttemptID || a.ResponseArtifactID != *r.ResponseArtifactID || a.ResponseArtifactID != b.ResponseArtifactID || a.ResponseSHA256 != b.ResponseSHA256 || b.ConditionsSHA256 != w.Authority.ConditionsSHA256 {
			return out, evidenceUnavailable()
		}
		v.ResponseBinding = &b
	default:
		return out, evidenceUnavailable()
	}
	a, e := s.attempt(ctx, q, r.ProducerAttemptID)
	if e != nil {
		return out, e
	}
	if a.TaskID != t.ID || a.TaskVersion != t.CurrentVersion || a.WorkflowID != w.ID || a.Phase != "settled" || a.Outcome == nil || *a.Outcome != "succeeded" || a.Stop != nil || a.PrimaryFailure != nil || a.Cleanup.State != "verified" || a.EvidenceStatus != "complete" {
		return out, evidenceUnavailable()
	}
	dispatch, e := s.graphDispatchForAttempt(ctx, q, a.ID)
	if e != nil {
		return out, e
	}
	d, observation := dispatch.Public.Dispatch, dispatch.Public.OutputObservation
	if d.GraphID != h.Summary.ID || d.NodeID != n.Summary.ID || d.TaskID != t.ID || d.TaskVersion != t.CurrentVersion || d.WorkflowID != w.ID || d.StepID != a.StepID || d.InputID != r.InputID || d.NodeAdmissionID != *n.Summary.NodeAdmissionID || observation == nil || observation.ProducerAttemptID != a.ID {
		return out, evidenceUnavailable()
	}
	var ref *domain.GraphArtifactRef
	var closure *domain.GraphOutputClosure
	if v.CandidateBinding != nil {
		o := observation.Candidate
		if o == nil || o.State != "complete" || o.CandidateID != v.CandidateBinding.CandidateID {
			return out, evidenceUnavailable()
		}
		ref, closure = o.ClosureRef, dispatch.CandidateClosure
		if closure == nil || closure.Kind != "candidate" || closure.CandidateID == nil || *closure.CandidateID != v.CandidateBinding.CandidateID || closure.Manifest.ArtifactID != v.CandidateBinding.ManifestArtifactID || closure.Manifest.SHA256 != v.CandidateBinding.ManifestSHA256 || closure.Index.ArtifactID != v.CandidateBinding.IndexArtifactID || closure.Index.SHA256 != v.CandidateBinding.IndexSHA256 {
			return out, evidenceUnavailable()
		}
	} else {
		o := observation.Response
		if o == nil || o.State != "complete" || o.ResponseArtifactID != v.ResponseBinding.ResponseArtifactID {
			return out, evidenceUnavailable()
		}
		ref, closure = o.ClosureRef, dispatch.ResponseClosure
		b := v.ResponseBinding
		if closure == nil || closure.Kind != "response" || closure.ResponseArtifact == nil || closure.ResponseArtifact.ArtifactID != b.ResponseArtifactID || closure.ResponseArtifact.SHA256 != b.ResponseSHA256 || closure.ResponseArtifact.ByteLength != b.ResponseByteLength || closure.Index.ArtifactID != b.IndexArtifactID || closure.Index.SHA256 != b.IndexSHA256 {
			return out, evidenceUnavailable()
		}
	}
	if ref == nil || closure == nil || !graphRefHasBytes(*ref, []byte(encode(closure))) || closure.GraphID != h.Summary.ID || closure.NodeID != n.Summary.ID || closure.ProducerAttemptID != a.ID || closure.ReceivedInputID != r.InputID {
		return out, evidenceUnavailable()
	}
	seal, e := domain.GraphOutputDigest(*closure)
	if e != nil || seal != closure.ClosureSHA256 {
		return out, evidenceUnavailable()
	}
	if e = s.graphStoredRef(ctx, q, h.Summary.ID, *ref, "graph_output_closure"); e != nil {
		return out, e
	}
	input, e := s.graphInputRecord(ctx, q, h.Summary.ID, r.InputID)
	if e != nil {
		return out, e
	}
	b := input.Public.InputBinding
	if !input.Public.Summary.Current || input.Public.Summary.State != "ready" || b == nil || b.ReceivingNodeID != n.Summary.ID || b.ReceivingTaskID != t.ID || b.ReceivingTaskVersion != t.CurrentVersion || input.ConditionsBinding == nil || input.Public.ConditionsRef == nil {
		return out, evidenceUnavailable()
	}
	v.OutputClosureRef = *ref
	return graphSelectedHead{Vector: v, Input: input, Workflow: w, Closure: *closure}, nil
}

func (s *Store) graphDependencySelection(ctx context.Context, tx *sql.Tx, h graphHeader, definition domain.GraphDefinition, nodes []graphNodeRecord, receiver graphNodeRecord, f domain.GraphInputPreparationFacts) ([]domain.AcceptedContribution, []domain.GraphContributorBinding, error) {
	byKey, byID := map[string]graphNodeRecord{}, map[string]graphNodeRecord{}
	for _, n := range nodes {
		byKey[n.Summary.Key], byID[n.Summary.ID] = n, n
	}
	provided := map[string]domain.AcceptedContribution{}
	for _, c := range f.Contributions {
		if _, duplicate := provided[c.EdgeID]; duplicate {
			return nil, nil, evidenceUnavailable()
		}
		provided[c.EdgeID] = c
	}
	vector := map[string]domain.GraphContributorBinding{}
	add := func(v domain.GraphContributorBinding) error {
		if previous, present := vector[v.NodeID]; present && !reflect.DeepEqual(previous, v) {
			return evidenceUnavailable()
		}
		vector[v.NodeID] = v
		if len(vector) > domain.MaxGraphNodes {
			return domain.Invalid("bounded current causal vector")
		}
		return nil
	}
	selected := []domain.AcceptedContribution{}
	if definition.Original.Dependencies != nil {
		for _, edge := range *definition.Original.Dependencies {
			if edge.To != receiver.Summary.Key {
				continue
			}
			source, ok := byKey[edge.From]
			if !ok {
				return nil, nil, storageError()
			}
			kind := edge.Kind
			if kind == "" {
				kind = "files"
			}
			id, e := domain.GraphEdgeID(h.Summary.ID, h.Summary.GraphVersion, source.Summary.ID, receiver.Summary.ID, kind)
			if e != nil {
				return nil, nil, e
			}
			c, present := provided[id]
			if !present || c.EdgeKind != kind {
				return nil, nil, domain.Fail("graph_dependencies_pending", "The exact complete direct dependency selection is required.")
			}
			head, e := s.graphSelectedHead(ctx, tx, h, source)
			if e != nil {
				return nil, nil, e
			}
			actual := domain.GraphContributorBinding{NodeID: c.FromNodeID, TaskID: c.TaskID, TaskVersion: c.TaskVersion, AcceptanceKind: c.AcceptanceKind, AcceptanceID: c.AcceptanceID, ProducerAttemptID: c.ProducerAttemptID, InputID: c.InputID, CandidateBinding: c.CandidateBinding, ResponseBinding: c.ResponseBinding, OutputClosureRef: c.OutputClosureRef}
			if !reflect.DeepEqual(actual, head.Vector) || kind == "files" && c.CandidateBinding == nil {
				return nil, nil, evidenceUnavailable()
			}
			accepted, e := s.graphAcceptedResult(ctx, tx, h, source, head.Workflow, f.VerifiedArtifacts)
			if e != nil || !reflect.DeepEqual(accepted, *source.Summary.AcceptedResult) {
				if e != nil {
					return nil, nil, e
				}
				return nil, nil, evidenceUnavailable()
			}
			if e = s.graphContributionContext(ctx, tx, h, c, head, f.VerifiedArtifacts); e != nil {
				return nil, nil, e
			}
			for _, ancestor := range head.Input.Public.InputBinding.ContributorVector {
				n, present := byID[ancestor.NodeID]
				if !present {
					return nil, nil, evidenceUnavailable()
				}
				current, e := s.graphSelectedHead(ctx, tx, h, n)
				if e != nil || !reflect.DeepEqual(current.Vector, ancestor) {
					if e != nil {
						return nil, nil, e
					}
					return nil, nil, evidenceUnavailable()
				}
				if e = add(ancestor); e != nil {
					return nil, nil, e
				}
			}
			if e = add(head.Vector); e != nil {
				return nil, nil, e
			}
			delete(provided, id)
			selected = append(selected, c)
		}
	}
	if len(provided) != 0 || len(f.ContributorVector) != len(vector) {
		return nil, nil, evidenceUnavailable()
	}
	seen := map[string]bool{}
	for _, v := range f.ContributorVector {
		if seen[v.NodeID] || !reflect.DeepEqual(vector[v.NodeID], v) {
			return nil, nil, evidenceUnavailable()
		}
		seen[v.NodeID] = true
	}
	canonical := []domain.GraphContributorBinding{}
	for _, v := range vector {
		canonical = append(canonical, v)
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].NodeID < canonical[j].NodeID })
	sort.Slice(selected, func(i, j int) bool { return selected[i].EdgeID < selected[j].EdgeID })
	return selected, canonical, nil
}

func (s *Store) graphContributionContext(ctx context.Context, tx *sql.Tx, h graphHeader, c domain.AcceptedContribution, head graphSelectedHead, proofs []domain.VerifiedResponse) error {
	type original struct {
		Ref                 domain.GraphArtifactRef
		Node, Version, Kind string
	}
	key := func(ref domain.GraphArtifactRef, node, version string) string {
		return Fingerprint("graph.upstream.ref", struct{ Artifact, Node, Version string }{ref.ArtifactID, node, version})
	}
	expected := map[string]original{}
	add := func(ref domain.GraphArtifactRef, node, version, kind string) {
		expected[key(ref, node, version)] = original{ref, node, version, kind}
	}
	for _, source := range head.Input.ConditionsBinding.Sources {
		node, version := c.FromNodeID, c.TaskVersion
		if source.NodeID != nil {
			node = *source.NodeID
		}
		if source.TaskVersion != nil {
			version = *source.TaskVersion
		}
		add(source.OriginalRef, node, version, "graph_original")
		if source.GuidanceRef != nil {
			add(*source.GuidanceRef, node, version, "graph_original")
		}
		if source.AcceptanceRef != nil {
			add(*source.AcceptanceRef, node, version, "graph_acceptance")
		}
	}
	add(*head.Input.Public.ConditionsRef, c.FromNodeID, c.TaskVersion, "graph_conditions")
	if len(c.ConstraintRefs)+len(c.EvidenceRefs) > 64 {
		return domain.Invalid("bounded complete contributed context")
	}
	names := map[string]bool{}
	for _, item := range c.ConstraintRefs {
		if item.Ref == "" || len(item.Ref) > 128 || names[item.Ref] || item.AuthorityScope != "upstream_local" || item.NodeID == nil || item.TaskVersion == nil || len(item.Paths) != 0 {
			return evidenceUnavailable()
		}
		names[item.Ref] = true
		k := key(item.ArtifactRef, *item.NodeID, *item.TaskVersion)
		want, present := expected[k]
		if !present || want.Ref != item.ArtifactRef {
			return evidenceUnavailable()
		}
		if e := s.verifyGraphRef(ctx, tx, item.ArtifactRef, proofs, want.Kind); e != nil {
			return e
		}
		delete(expected, k)
	}
	if len(expected) != 0 {
		return evidenceUnavailable()
	}
	result, scope, resultID := head.Closure.Manifest, "whole_candidate", ""
	if c.CandidateBinding != nil {
		resultID = c.CandidateBinding.CandidateID
	} else {
		result, scope, resultID = *head.Closure.ResponseArtifact, "whole_response", c.ResponseBinding.ResponseArtifactID
	}
	evidence := map[string]domain.GraphArtifactRef{result.ArtifactID: result, c.OutputClosureRef.ArtifactID: c.OutputClosureRef}
	for _, item := range c.EvidenceRefs {
		want, present := evidence[item.ArtifactRef.ArtifactID]
		if item.Ref == "" || len(item.Ref) > 128 || names[item.Ref] || !present || item.ArtifactRef != want || item.EdgeID != c.EdgeID || item.SourceNodeID != c.FromNodeID || item.AcceptanceKind != c.AcceptanceKind || item.AcceptanceID != c.AcceptanceID || item.ProducerAttemptID != c.ProducerAttemptID || item.ResultKind != head.Closure.Kind || item.ResultID != resultID || item.Scope != scope || len(item.Paths) != 0 {
			return evidenceUnavailable()
		}
		names[item.Ref] = true
		kind := "graph_output_closure"
		if item.ArtifactRef.ArtifactID == result.ArtifactID {
			kind = "candidate_manifest"
			if c.ResponseBinding != nil {
				kind = "final_response"
			}
		}
		if e := s.verifyGraphRef(ctx, tx, item.ArtifactRef, proofs, kind); e != nil {
			return e
		}
		delete(evidence, item.ArtifactRef.ArtifactID)
	}
	if len(evidence) != 0 {
		return evidenceUnavailable()
	}
	return nil
}
