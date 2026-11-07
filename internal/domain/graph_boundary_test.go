package domain

import (
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestGraphClaimTuplesAreActualScopeFacts(t *testing.T) {
	first := GraphClaimView{Slot: "1", AttemptID: strings.Repeat("1", 32), ExecutorKind: "native", ContextID: ptrGraph("codex-native")}
	second := GraphClaimView{Slot: "2", AttemptID: strings.Repeat("2", 32), ExecutorKind: "native", ContextID: ptrGraph("claude-native")}
	if e := ValidateGraphClaims([]GraphClaimView{first, second}); e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		name  string
		build func() []GraphClaimView
	}{
		{"duplicate_context", func() []GraphClaimView { b := second; b.ContextID = first.ContextID; return []GraphClaimView{first, b} }},
		{"unknown_native", func() []GraphClaimView { a := first; a.ContextID = nil; return []GraphClaimView{a} }},
		{"account_without_link", func() []GraphClaimView {
			a := first
			a.CapturedDeclaredAccountID = ptrGraph(strings.Repeat("3", 32))
			return []GraphClaimView{a}
		}},
		{"local_native_context", func() []GraphClaimView { a := first; a.ExecutorKind = "local_check"; return []GraphClaimView{a} }},
		{"local_graph_without_graph", func() []GraphClaimView {
			a := first
			a.ExecutorKind = "local_graph"
			a.ContextID = nil
			return []GraphClaimView{a}
		}},
		{"node_without_graph", func() []GraphClaimView {
			a := first
			a.NodeID = ptrGraph(strings.Repeat("4", 32))
			return []GraphClaimView{a}
		}},
		{"graph_native_without_node", func() []GraphClaimView {
			a := first
			a.GraphID = ptrGraph(strings.Repeat("4", 32))
			return []GraphClaimView{a}
		}},
		{"barrier_with_other_claim", func() []GraphClaimView { a := first; a.LegacyBarrier = true; return []GraphClaimView{a, second} }},
		{"barrier_slot2", func() []GraphClaimView { a := second; a.LegacyBarrier = true; return []GraphClaimView{a} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if e := ValidateGraphClaims(test.build()); e == nil {
				t.Fatal("impossible ownership tuple accepted")
			}
		})
	}
	legacy := first
	legacy.LegacyBarrier = true
	legacy.ContextID = nil
	if e := ValidateGraphClaims([]GraphClaimView{legacy}); e != nil {
		t.Fatal("conservative old held claim must remain representable", e)
	}
}

func ptrGraph(s string) *string { return &s }

func TestGraphStrictNestedTypesAndAuthorityArray(t *testing.T) {
	for _, raw := range []string{
		`{"nodes":[{"key":"A","originalText":"Work","pins":{"model":""}}]}`,
		`{"nodes":[{"key":"A","originalText":"Work","acceptance":{"requiredEvidence":null}}]}`,
		`{"nodes":[{"key":"A","originalText":"Work","unknown":false}]}`,
		`{"nodes":[{"key":"A","originalText":"Work"}],"dependencies":[{"from":"A","to":"B","kind":""}]}`,
		`{"nodes":[{"key":"A","originalText":"Work"}],"maxParallelism":2}`,
		`{"nodes":[{"key":"A","originalText":"Work"}],"projectId":null}`,
	} {
		if _, e := StrictDecodeGraphStart([]byte(raw)); e == nil {
			t.Fatal("invalid nested type/presence accepted", raw)
		}
	}
	for _, raw := range []string{`{}`, `[null]`, `[{"nodeId":"","taskVersion":"1","nodeControlRevision":"1"}]`, `[{"nodeId":"11111111111111111111111111111111","taskVersion":9007199254740993,"nodeControlRevision":"1"}]`} {
		if _, e := StrictDecodeGraphNodeAuthorities([]byte(raw)); e == nil {
			t.Fatal("unsafe authority file accepted", raw)
		}
	}
	value := GraphNodeAuthority{NodeID: strings.Repeat("1", 32), TaskVersion: "1", NodeControlRevision: "9007199254740993"}
	b, e := json.Marshal([]GraphNodeAuthority{value, value})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := StrictDecodeGraphNodeAuthorities(b); e == nil {
		t.Fatal("duplicate authority file accepted")
	}
}

func TestGraphRootBindingRejectsTraversalAndUnownedRefs(t *testing.T) {
	producer := strings.Repeat("3", 32)
	ref := func(id string) GraphArtifactRef {
		return GraphArtifactRef{ArtifactID: strings.Repeat(id, 32), ProducerAttemptID: producer, SHA256: strings.Repeat("a", 64), ByteLength: "0"}
	}
	root := GraphRootBinding{GraphID: strings.Repeat("1", 32), RootTaskID: strings.Repeat("2", 32), OperationAttemptID: producer, SourceSnapshotID: strings.Repeat("7", 32), ProjectID: strings.Repeat("8", 32), Manifest: ref("4"), Index: ref("5"), StagedBlobs: ref("6"), ProjectIdentity: "labelled-project-fact", RepositoryIdentity: "labelled-repository-fact", BaseCommit: strings.Repeat("a", 40), BasisSHA256: strings.Repeat("b", 64)}
	if e := ValidateGraphRootBinding(root); e != nil {
		t.Fatal("valid typed metadata, not physical proof", e)
	}
	for _, change := range []func(*GraphRootBinding){func(v *GraphRootBinding) { v.SelectedRelativePath = "../outside" }, func(v *GraphRootBinding) { v.Index.ProducerAttemptID = strings.Repeat("9", 32) }, func(v *GraphRootBinding) { v.ProjectIdentity = "" }, func(v *GraphRootBinding) { v.BaseCommit = strings.Repeat("a", 64) }} {
		v := root
		change(&v)
		if e := ValidateGraphRootBinding(v); e == nil {
			t.Fatal("unsafe root binding accepted")
		}
	}
}
