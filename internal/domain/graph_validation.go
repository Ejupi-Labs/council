package domain

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"unicode"
	"unicode/utf8"
)

func GraphKey(s string) bool {
	if len(s) < 1 || len(s) > 48 {
		return false
	}
	for i, c := range []byte(s) {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || i > 0 && (c == '_' || c == '-')) {
			return false
		}
	}
	return true
}
func graphRequest(id string, revisions ...string) *Error {
	if !ValidID(id) {
		return Invalid("requestId")
	}
	for _, r := range revisions {
		if _, e := ParseRevision(r); e != nil {
			return e
		}
	}
	return nil
}
func graphIDs(ids ...string) *Error {
	for _, id := range ids {
		if !ValidID(id) {
			return Invalid("graph record identity")
		}
	}
	return nil
}
func graphHashes(hashes ...string) *Error {
	for _, h := range hashes {
		if !ValidSHA256(h) {
			return Invalid("graph SHA256")
		}
	}
	return nil
}
func graphRefs(refs ...GraphArtifactRef) *Error {
	for _, r := range refs {
		if e := ValidateGraphArtifactRef(r); e != nil {
			return e
		}
	}
	return nil
}
func graphNodeOptions(r *RoutingOptions, w *WorkflowInput, mode string) *Error {
	if r != nil && r.Budget != nil {
		return Invalid("graph-only shared budget")
	}
	if e := ValidateRoutingOptions(r, mode); e != nil {
		return e
	}
	if w != nil && (w.Delivery != nil || w.Mode == "author_only") {
		return Invalid("Auto node without intermediate delivery")
	}
	return ValidateWorkflowInput(w)
}
func ValidateGraphStartInput(in GraphStartInput) *Error {
	if e := graphRequest(in.RequestID); e != nil {
		return e
	}
	if !ValidID(in.ProjectID) {
		return Invalid("projectId")
	}
	if in.SharedInstructions != nil {
		if e := ValidateAdditionalText(*in.SharedInstructions); e != nil {
			return e
		}
	}
	if in.Title != nil {
		if !utf8.ValidString(*in.Title) || utf8.RuneCountInString(*in.Title) > 160 {
			return Invalid("title of at most160 characters")
		}
		for _, c := range *in.Title {
			if unicode.IsControl(c) {
				return Invalid("title without controls")
			}
		}
	}
	if in.ExpectedPolicyRevision != "" {
		if _, e := ParseRevision(in.ExpectedPolicyRevision); e != nil {
			return e
		}
	}
	if e := ValidateVersionOptions(nil, in.PolicyOverride, nil, nil); e != nil {
		return e
	}
	if in.MaxParallelism != "" && in.MaxParallelism != "1" && in.MaxParallelism != "2" {
		return Invalid("maxParallelism string1 or2")
	}
	if e := ValidateExecutionMode(in.ExecutionMode, ""); e != nil {
		return e
	}
	if e := ValidateRoutingOptions(in.Routing, "start"); e != nil {
		return e
	}
	if len(in.Nodes) < 1 || len(in.Nodes) > MaxGraphNodes {
		return Invalid("one through sixteen nodes")
	}
	nodes := map[string]int{}
	for i, n := range in.Nodes {
		if !GraphKey(n.Key) {
			return Invalid("node key")
		}
		if _, ok := nodes[n.Key]; ok {
			return Invalid("unique node keys")
		}
		nodes[n.Key] = i
		if e := ValidateText(n.OriginalText); e != nil {
			return e
		}
		if e := ValidateVersionOptions(n.Pins, "", n.Acceptance, n.TimeoutSeconds); e != nil {
			return e
		}
		if e := ValidateExecutionMode(in.ExecutionMode, n.FakeScenario); e != nil {
			return e
		}
		if e := graphNodeOptions(n.Routing, n.Workflow, "start"); e != nil {
			return e
		}
	}
	edges := []GraphDependencyInput{}
	if in.Dependencies != nil {
		edges = *in.Dependencies
	}
	if len(edges) > MaxGraphEdges {
		return Invalid("at most64 dependencies")
	}
	adjacency := make([][]int, len(in.Nodes))
	indegree := make([]int, len(in.Nodes))
	seen := map[string]bool{}
	for _, edge := range edges {
		a, okA := nodes[edge.From]
		b, okB := nodes[edge.To]
		if !okA || !okB || a == b {
			return Invalid("known distinct dependency endpoints")
		}
		if edge.Kind != "" && edge.Kind != "files" && edge.Kind != "context" {
			return Invalid("dependency kind")
		}
		key := edge.From + "\x00" + edge.To
		if seen[key] {
			return Invalid("unique dependency endpoints")
		}
		seen[key] = true
		adjacency[a] = append(adjacency[a], b)
		indegree[b]++
	}
	queue := []int{}
	for i, n := range indegree {
		if n == 0 {
			queue = append(queue, i)
		}
	}
	visited := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adjacency[n] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(in.Nodes) {
		return Invalid("acyclic dependencies")
	}
	if in.FinalNodeKey == "" {
		if in.Delivery != nil {
			return Invalid("final node for explicit delivery")
		}
	} else {
		final, ok := nodes[in.FinalNodeKey]
		if !ok {
			return Invalid("known finalNodeKey")
		}
		for start := range in.Nodes {
			reached := make([]bool, len(in.Nodes))
			pending := []int{start}
			reached[start] = true
			for len(pending) > 0 {
				n := pending[0]
				pending = pending[1:]
				for _, next := range adjacency[n] {
					if !reached[next] {
						reached[next] = true
						pending = append(pending, next)
					}
				}
			}
			if !reached[final] {
				return Invalid("final node downstream of every required node")
			}
		}
		if in.Delivery != nil {
			if e := ValidateWorkflowDeliveryInput(*in.Delivery); e != nil {
				return e
			}
		}
	}
	b, e := json.Marshal(in)
	if e != nil {
		return Invalid("graph declaration")
	}
	if len(b) > MaxGraphDeclarationBytes {
		return Fail("input_too_large", "Graph declaration exceeds2 MiB.")
	}
	return nil
}
func validateGraphNodeAuthorities(values []GraphNodeAuthority) *Error {
	if len(values) < 1 || len(values) > MaxGraphNodes {
		return Invalid("one through sixteen node authorities")
	}
	seen := map[string]bool{}
	for _, v := range values {
		if !ValidID(v.NodeID) || seen[v.NodeID] {
			return Invalid("unique node authority IDs")
		}
		seen[v.NodeID] = true
		if _, e := ParseRevision(v.TaskVersion); e != nil {
			return e
		}
		if _, e := ParseRevision(v.NodeControlRevision); e != nil {
			return e
		}
	}
	return nil
}
func ValidateGraphAdvanceInput(in GraphAdvanceInput) *Error {
	if e := graphRequest(in.RequestID, in.GraphVersion, in.ExpectedGraphControlRevision, in.ExpectedBudgetAuthorityEpoch, in.ExpectedWindowNumber); e != nil {
		return e
	}
	if e := validateGraphNodeAuthorities(in.NodeAuthorities); e != nil {
		return e
	}
	if in.RebindNodeIDs != nil {
		if len(*in.RebindNodeIDs) > MaxGraphNodes {
			return Invalid("bounded rebind IDs")
		}
		allowed, seen := map[string]bool{}, map[string]bool{}
		for _, a := range in.NodeAuthorities {
			allowed[a.NodeID] = true
		}
		for _, id := range *in.RebindNodeIDs {
			if !ValidID(id) || !allowed[id] || seen[id] {
				return Invalid("unique rebind IDs in current authority vector")
			}
			seen[id] = true
		}
	}
	return nil
}
func ValidateGraphStopInput(in GraphStopInput) *Error {
	return graphRequest(in.RequestID, in.ExpectedGraphControlRevision)
}
func ValidateGraphNodeStopInput(in GraphNodeStopInput) *Error {
	if e := graphRequest(in.RequestID, in.ExpectedGraphControlRevision, in.ExpectedNodeControlRevision); e != nil {
		return e
	}
	return graphIDs(in.NodeID)
}
func ValidateGraphGuidanceInput(in GraphGuidanceInput) *Error {
	if e := graphRequest(in.RequestID, in.ExpectedGraphControlRevision, in.ExpectedNodeControlRevision, in.TaskVersion); e != nil {
		return e
	}
	if e := graphIDs(in.NodeID); e != nil {
		return e
	}
	if e := ValidateGuidance(in.GuidanceInput); e != nil {
		return e
	}
	return graphNodeOptions(in.Routing, in.Workflow, "guidance")
}
func ValidateGraphBudgetAmendmentInput(in GraphBudgetAmendmentInput) *Error {
	if e := graphRequest(in.RequestID, in.ExpectedGraphControlRevision, in.ExpectedBudgetAuthorityEpoch, in.ExpectedWindowNumber); e != nil {
		return e
	}
	n, e := ParseCount(in.AdditionalAdmissions)
	if e != nil || n < 1 || n > 16 {
		return Invalid("additionalAdmissions from1 through16")
	}
	if in.NewWindowSeconds != nil && (*in.NewWindowSeconds < 1 || *in.NewWindowSeconds > 86400) {
		return Invalid("newWindowSeconds from1 through86400")
	}
	return nil
}
func ValidateGraphArtifactRef(in GraphArtifactRef) *Error {
	if e := graphIDs(in.ArtifactID, in.ProducerAttemptID); e != nil {
		return e
	}
	if e := graphHashes(in.SHA256); e != nil {
		return e
	}
	_, e := ParseCount(in.ByteLength)
	return e
}
func ValidateGraphRootBinding(in GraphRootBinding) *Error {
	if e := graphIDs(in.GraphID, in.RootTaskID, in.OperationAttemptID, in.SourceSnapshotID, in.ProjectID); e != nil {
		return e
	}
	if e := graphRefs(in.Manifest, in.Index, in.StagedBlobs); e != nil {
		return e
	}
	for _, r := range []GraphArtifactRef{in.Manifest, in.Index, in.StagedBlobs} {
		if r.ProducerAttemptID != in.OperationAttemptID {
			return Invalid("root-operation evidence ownership")
		}
	}
	if in.ProjectIdentity == "" || in.RepositoryIdentity == "" || len(in.ProjectIdentity) > 4096 || len(in.RepositoryIdentity) > 4096 || !utf8.ValidString(in.ProjectIdentity) || !utf8.ValidString(in.RepositoryIdentity) {
		return Invalid("private root identity")
	}
	if in.SelectedRelativePath != "" && in.SelectedRelativePath != "." && !CanonicalWorkflowPath(in.SelectedRelativePath) {
		return Invalid("root selected scope")
	}
	if len(in.BaseCommit) != 40 || strings.Trim(in.BaseCommit, "0123456789abcdef") != "" {
		return Invalid("SHA1 Git basis")
	}
	return graphHashes(in.BasisSHA256)
}
func graphSubject(c *CandidateBinding, r *ResponseBinding) *Error {
	if (c == nil) == (r == nil) {
		return Invalid("exactly one graph candidate/response")
	}
	if c != nil {
		if e := graphIDs(c.RootSourceSnapshotID, c.CandidateID, c.SourceSnapshotID, c.ManifestArtifactID, c.IndexArtifactID); e != nil {
			return e
		}
		return graphHashes(c.ManifestSHA256, c.IndexSHA256, c.ConditionsSHA256)
	}
	if e := graphIDs(r.ProducerAttemptID, r.ResponseArtifactID, r.SourceSnapshotID, r.IndexArtifactID); e != nil {
		return e
	}
	if _, e := ParseCount(r.ResponseByteLength); e != nil {
		return e
	}
	return graphHashes(r.ResponseSHA256, r.ManifestSHA256, r.IndexSHA256, r.ConditionsSHA256)
}
func ValidateGraphInputBinding(in GraphInputBinding) *Error {
	if e := graphIDs(in.ID, in.GraphID, in.OperationAttemptID, in.ReceivingNodeID, in.ReceivingTaskID); e != nil {
		return e
	}
	if _, e := ParseRevision(in.GraphVersion); e != nil {
		return e
	}
	if _, e := ParseRevision(in.ReceivingTaskVersion); e != nil {
		return e
	}
	if e := graphHashes(in.RootBasisSHA256, in.InputSHA256); e != nil {
		return e
	}
	refs := []GraphArtifactRef{in.Manifest, in.Index, in.StagedBlobs, in.Provenance, in.ContextPacket}
	if e := graphRefs(refs...); e != nil {
		return e
	}
	for _, r := range refs {
		if r.ProducerAttemptID != in.OperationAttemptID {
			return Invalid("input composition ownership")
		}
	}
	if len(in.Contributions) > MaxGraphEdges || len(in.ContributorVector) > MaxGraphNodes {
		return Invalid("bounded graph contributors")
	}
	edges, nodes := map[string]bool{}, map[string]bool{}
	for _, c := range in.Contributions {
		if e := graphIDs(c.EdgeID, c.FromNodeID, c.TaskID, c.AcceptanceID, c.ProducerAttemptID, c.InputID); e != nil {
			return e
		}
		if edges[c.EdgeID] || c.EdgeKind != "files" && c.EdgeKind != "context" || c.AcceptanceKind != "workflow" && c.AcceptanceKind != "response" {
			return Invalid("unique typed direct contribution")
		}
		edges[c.EdgeID] = true
		if _, e := ParseRevision(c.TaskVersion); e != nil {
			return e
		}
		if e := graphSubject(c.CandidateBinding, c.ResponseBinding); e != nil {
			return e
		}
		if e := ValidateGraphArtifactRef(c.OutputClosureRef); e != nil {
			return e
		}
		if c.OutputClosureRef.ProducerAttemptID != c.ProducerAttemptID {
			return Invalid("direct output closure producer")
		}
	}
	for _, c := range in.ContributorVector {
		if e := graphIDs(c.NodeID, c.TaskID, c.AcceptanceID, c.ProducerAttemptID, c.InputID); e != nil {
			return e
		}
		if nodes[c.NodeID] || c.AcceptanceKind != "workflow" && c.AcceptanceKind != "response" {
			return Invalid("unique transitive contribution")
		}
		nodes[c.NodeID] = true
		if _, e := ParseRevision(c.TaskVersion); e != nil {
			return e
		}
		if e := graphSubject(c.CandidateBinding, c.ResponseBinding); e != nil {
			return e
		}
		if e := ValidateGraphArtifactRef(c.OutputClosureRef); e != nil {
			return e
		}
		if c.OutputClosureRef.ProducerAttemptID != c.ProducerAttemptID {
			return Invalid("transitive output closure producer")
		}
	}
	return nil
}
func ValidateGraphSourceBasis(in GraphSourceBasis) *Error {
	if e := graphIDs(in.WorkflowID, in.StepID, in.InputID); e != nil {
		return e
	}
	if e := graphHashes(in.InputSHA256, in.ExpectedWorkingEntriesSHA256, in.ExpectedIndexEntriesSHA256); e != nil {
		return e
	}
	if e := graphRefs(in.ExpectedManifest, in.ExpectedIndex); e != nil {
		return e
	}
	switch in.BasisKind {
	case "graph_input":
		if in.CandidateBinding != nil || in.ResponseBinding != nil {
			return Invalid("graph basis without node subject")
		}
	case "node_subject":
		return graphSubject(in.CandidateBinding, in.ResponseBinding)
	default:
		return Invalid("source basis kind")
	}
	return nil
}
func ValidateGraphMaterializationReceipt(in GraphMaterializationReceipt) *Error {
	if e := graphIDs(in.ID, in.GraphID, in.InputID, in.OperationAttemptID, in.ReceivingAttemptID); e != nil {
		return e
	}
	if in.OperationAttemptID != in.ReceivingAttemptID {
		return Invalid("actual receiving operation")
	}
	if e := ValidateGraphSourceBasis(in.SourceBasis); e != nil {
		return e
	}
	if in.InputID != in.SourceBasis.InputID || in.InputSHA256 != in.SourceBasis.InputSHA256 {
		return Invalid("exact materialization input")
	}
	if _, e := ParseCount(in.EffectCount); e != nil {
		return e
	}
	if _, e := ParseRoutingTime(in.CompletedAt); e != nil {
		return e
	}
	if in.State != "complete" && in.State != "blocked" && in.State != "incomplete" && in.State != "uncertain" {
		return Invalid("materialization state")
	}
	if in.EvidenceStatus != "complete" && in.EvidenceStatus != "incomplete" && in.EvidenceStatus != "uncertain" {
		return Invalid("materialization evidence")
	}
	for _, r := range []*GraphArtifactRef{in.ObservedManifest, in.ObservedIndex, in.OperationEffects} {
		if r != nil {
			if e := ValidateGraphArtifactRef(*r); e != nil {
				return e
			}
			if r.ProducerAttemptID != in.ReceivingAttemptID {
				return Invalid("receiving materialization ownership")
			}
		}
	}
	for _, h := range []*string{in.ObservedWorkingEntriesSHA256, in.ObservedIndexEntriesSHA256, in.StagedBlobsSHA256} {
		if h != nil && !ValidSHA256(*h) {
			return Invalid("observed source hash")
		}
	}
	if in.SourceSnapshotID != nil && !ValidID(*in.SourceSnapshotID) {
		return Invalid("observed source snapshot")
	}
	if in.State == "complete" {
		if !in.EffectsKnown || in.EvidenceStatus != "complete" || in.Failure != nil || in.SourceSnapshotID == nil || in.ObservedManifest == nil || in.ObservedIndex == nil || in.OperationEffects == nil || in.ObservedWorkingEntriesSHA256 == nil || in.ObservedIndexEntriesSHA256 == nil || in.StagedBlobsSHA256 == nil {
			return Invalid("complete known materialization")
		}
		if *in.ObservedWorkingEntriesSHA256 != in.SourceBasis.ExpectedWorkingEntriesSHA256 || *in.ObservedIndexEntriesSHA256 != in.SourceBasis.ExpectedIndexEntriesSHA256 {
			return Invalid("actual semantic materialization equality")
		}
	}
	return nil
}
func ValidateGraphOutputClosure(in GraphOutputClosure) *Error {
	if e := graphIDs(in.GraphID, in.NodeID, in.ReceivedInputID, in.ProducerAttemptID); e != nil {
		return e
	}
	refs := []GraphArtifactRef{in.Manifest, in.Index, in.StagedBlobs}
	if e := graphRefs(refs...); e != nil {
		return e
	}
	for _, r := range refs {
		if r.ProducerAttemptID != in.ProducerAttemptID {
			return Invalid("output closure ownership")
		}
	}
	if e := graphHashes(in.ClosureSHA256); e != nil {
		return e
	}
	switch in.Kind {
	case "candidate":
		if in.CandidateID == nil || !ValidID(*in.CandidateID) || in.ResponseArtifact != nil {
			return Invalid("candidate closure")
		}
	case "response":
		if in.CandidateID != nil || in.ResponseArtifact == nil {
			return Invalid("response without invented candidate")
		}
		if e := ValidateGraphArtifactRef(*in.ResponseArtifact); e != nil {
			return e
		}
		if in.ResponseArtifact.ProducerAttemptID != in.ProducerAttemptID {
			return Invalid("response producer")
		}
	default:
		return Invalid("output closure kind")
	}
	return nil
}
func ValidateGraphConditionsBinding(in GraphConditionsBinding) *Error {
	if e := graphIDs(in.GraphID, in.ReceiverTaskID); e != nil {
		return e
	}
	if _, e := ParseRevision(in.GraphVersion); e != nil {
		return e
	}
	if _, e := ParseRevision(in.ReceiverTaskVersion); e != nil {
		return e
	}
	if e := graphHashes(in.ConditionsSHA256); e != nil {
		return e
	}
	if len(in.Sources) < 1 || len(in.Sources) > 64 {
		return Invalid("bounded scoped conditions")
	}
	seen := map[string]bool{}
	receiver := false
	for _, s := range in.Sources {
		if s.Ref == "" || len(s.Ref) > 256 || seen[s.Ref] {
			return Invalid("unique scoped condition source")
		}
		seen[s.Ref] = true
		if e := ValidateGraphArtifactRef(s.OriginalRef); e != nil {
			return e
		}
		for _, r := range []*GraphArtifactRef{s.GuidanceRef, s.AcceptanceRef} {
			if r != nil {
				if e := ValidateGraphArtifactRef(*r); e != nil {
					return e
				}
			}
		}
		switch s.AuthorityScope {
		case "graph_shared":
			if s.NodeID != nil || s.TaskVersion != nil {
				return Invalid("graph-shared scope")
			}
		case "receiver", "upstream_local":
			if s.NodeID == nil || !ValidID(*s.NodeID) || s.TaskVersion == nil {
				return Invalid("node condition scope")
			}
			if _, e := ParseRevision(*s.TaskVersion); e != nil {
				return e
			}
			if s.AuthorityScope == "receiver" {
				receiver = true
				if *s.TaskVersion != in.ReceiverTaskVersion {
					return Invalid("receiver condition version")
				}
			}
		default:
			return Invalid("condition scope")
		}
	}
	if !receiver {
		return Invalid("receiver condition source")
	}
	return nil
}
func ValidateGraphClaims(in []GraphClaimView) *Error {
	if len(in) > MaxManagedSlots {
		return Invalid("at most two claims")
	}
	slots, attempts := map[string]bool{}, map[string]bool{}
	contexts, accounts := map[string]bool{}, map[string]bool{}
	for _, c := range in {
		if c.Slot != "1" && c.Slot != "2" || slots[c.Slot] || !ValidID(c.AttemptID) || attempts[c.AttemptID] {
			return Invalid("unique actual slot/attempt")
		}
		slots[c.Slot] = true
		attempts[c.AttemptID] = true
		if c.LegacyBarrier && (len(in) != 1 || c.Slot != "1" || c.GraphID != nil || c.NodeID != nil) {
			return Invalid("sole legacy slot1 barrier")
		}
		if (c.CapturedDeclaredAccountID == nil) != (c.CapturedLinkRevision == nil) {
			return Invalid("paired captured account/link")
		}
		if c.CapturedDeclaredAccountID != nil {
			if c.ContextID == nil || accounts[*c.CapturedDeclaredAccountID] {
				return Invalid("actual distinct captured account reservations")
			}
			accounts[*c.CapturedDeclaredAccountID] = true
			if !ValidID(*c.CapturedDeclaredAccountID) {
				return Invalid("captured account ID")
			}
			if _, e := ParseRevision(*c.CapturedLinkRevision); e != nil {
				return e
			}
		}
		if c.GraphID != nil && !ValidID(*c.GraphID) || c.NodeID != nil && !ValidID(*c.NodeID) || c.NodeID != nil && c.GraphID == nil {
			return Invalid("actual claim membership")
		}
		if c.ContextID != nil {
			if contexts[*c.ContextID] {
				return Invalid("one claim per native context")
			}
			contexts[*c.ContextID] = true
		}
		switch c.ExecutorKind {
		case "native":
			if c.ContextID == nil {
				if !c.LegacyBarrier {
					return Invalid("native context")
				}
			} else if *c.ContextID != "codex-native" && *c.ContextID != "claude-native" {
				return Invalid("supported native context")
			}
		case "local_check", "local_application", "local_external", "local_graph":
			if c.ContextID != nil || c.CapturedDeclaredAccountID != nil || c.CapturedLinkRevision != nil {
				return Invalid("local without native identity")
			}
		default:
			return Invalid("executor kind")
		}
		if c.ExecutorKind == "local_graph" {
			if c.GraphID == nil || c.NodeID != nil {
				return Invalid("root graph-local claim")
			}
		} else if c.GraphID != nil && c.NodeID == nil {
			return Invalid("node claim membership")
		}
	}
	return nil
}
func StrictDecodeGraphStart(body []byte) (GraphStartInput, *Error) {
	var in GraphStartInput
	if len(body) > MaxGraphDeclarationBytes {
		return in, Fail("input_too_large", "Graph declaration exceeds2 MiB.")
	}
	if e := StrictDecode(body, &in); e != nil {
		return in, e
	}
	return in, nil
}
func StrictDecodeGraphNodeAuthorities(body []byte) ([]GraphNodeAuthority, *Error) {
	if len(body) > MaxWorkflowBytes {
		return nil, Fail("input_too_large", "Node authority file exceeds64 KiB.")
	}
	var values []jsontext.Value
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '[' || json.Unmarshal(body, &values) != nil {
		return nil, Invalid("node authority array")
	}
	if len(values) < 1 || len(values) > MaxGraphNodes {
		return nil, Invalid("bounded node authorities")
	}
	out := make([]GraphNodeAuthority, 0, len(values))
	for _, v := range values {
		var in GraphNodeAuthority
		if e := StrictDecode(v, &in); e != nil {
			return nil, e
		}
		out = append(out, in)
	}
	if e := validateGraphNodeAuthorities(out); e != nil {
		return nil, e
	}
	return out, nil
}
