package domain

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"reflect"
	"strings"
	"testing"
)

func graphValidationStart() GraphStartInput {
	return GraphStartInput{RequestID: strings.Repeat("1", 32), ProjectID: strings.Repeat("2", 32), ExecutionMode: "fake", Nodes: []GraphNodeInput{{Key: "A", OriginalText: "\ufeff Keep café \ufffd\r\n東京 exactly.\n"}, {Key: "B", OriginalText: "Inspect the accepted predecessor."}}}
}

func TestGraphDeclarationWholeDAGAndFiniteScope(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*GraphStartInput)
	}{
		{"no_nodes", func(in *GraphStartInput) { in.Nodes = nil }},
		{"too_many_nodes", func(in *GraphStartInput) {
			for len(in.Nodes) <= MaxGraphNodes {
				in.Nodes = append(in.Nodes, GraphNodeInput{Key: "X" + Decimal(int64(len(in.Nodes))), OriginalText: "Work"})
			}
		}},
		{"duplicate_key", func(in *GraphStartInput) { in.Nodes[1].Key = "A" }},
		{"path_key", func(in *GraphStartInput) { in.Nodes[1].Key = "../B" }},
		{"foreign_dependency", func(in *GraphStartInput) { v := []GraphDependencyInput{{From: "X", To: "B"}}; in.Dependencies = &v }},
		{"self_edge", func(in *GraphStartInput) { v := []GraphDependencyInput{{From: "A", To: "A"}}; in.Dependencies = &v }},
		{"cycle", func(in *GraphStartInput) {
			v := []GraphDependencyInput{{From: "A", To: "B"}, {From: "B", To: "A"}}
			in.Dependencies = &v
		}},
		{"duplicate_edge", func(in *GraphStartInput) {
			v := []GraphDependencyInput{{From: "A", To: "B"}, {From: "A", To: "B", Kind: "files"}}
			in.Dependencies = &v
		}},
		{"child_budget", func(in *GraphStartInput) { in.Nodes[0].Routing = &RoutingOptions{Budget: &RoutingBudgetInput{}} }},
		{"intermediate_delivery", func(in *GraphStartInput) {
			in.Nodes[0].Workflow = &WorkflowInput{Delivery: &WorkflowDeliveryInput{LocalMode: "apply_verified_local"}}
		}},
		{"author_only", func(in *GraphStartInput) { in.Nodes[0].Workflow = &WorkflowInput{Mode: "author_only"} }},
		{"delivery_without_final", func(in *GraphStartInput) { in.Delivery = &WorkflowDeliveryInput{LocalMode: "review_only"} }},
		{"final_not_downstream", func(in *GraphStartInput) { in.FinalNodeKey = "B" }},
		{"invalid_parallelism", func(in *GraphStartInput) { in.MaxParallelism = "3" }},
		{"unsafe_request_id", func(in *GraphStartInput) { in.RequestID = "1" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			in := graphValidationStart()
			test.change(&in)
			if e := ValidateGraphStartInput(in); e == nil {
				t.Fatal("unsafe whole declaration accepted")
			}
		})
	}
	valid := graphValidationStart()
	edges := []GraphDependencyInput{{From: "A", To: "B", Kind: "files"}}
	valid.Dependencies = &edges
	valid.FinalNodeKey = "B"
	if e := ValidateGraphStartInput(valid); e != nil {
		t.Fatal("valid bounded graph", e)
	}
}

func TestGraphWirePresenceAndLosslessChoices(t *testing.T) {
	for _, test := range []struct{ key, value string }{
		{"maxParallelism", `""`},
		{"projectId", `""`},
		{"nodes", `[{"key":"A","originalText":"Work","workflow":{"delivery":null}}]`},
		{"nodes", `[{"key":"A","originalText":"Work","routing":{"budget":{}}}]`},
		{"nodes", `[{"key":"A","originalText":"Work","workflow":{"mode":""}}]`},
	} {
		base := `{"requestId":"` + strings.Repeat("1", 32) + `","projectId":"` + strings.Repeat("2", 32) + `","nodes":[{"key":"A","originalText":"Work"}]}`
		var members map[string]jsontext.Value
		if err := json.Unmarshal([]byte(base), &members); err != nil {
			t.Fatal(err)
		}
		members[test.key] = jsontext.Value(test.value)
		raw, err := json.Marshal(members, json.Deterministic(true))
		if err != nil {
			t.Fatal(err)
		}
		if in, e := StrictDecodeGraphStart(raw); e == nil && ValidateGraphStartInput(in) == nil {
			t.Fatal("explicit invalid/forbidden unique member accepted", test.key, test.value)
		}
	}
	raw := []byte(`{"nodes":[{"key":"A","originalText":"\ufeff café \ufffd\r\n東京","workflow":{"checks":[],"maxCorrectionCycles":0}}],"sharedInstructions":"","dependencies":[]}`)
	in, e := StrictDecodeGraphStart(raw)
	if e != nil {
		t.Fatal(e)
	}
	if in.RequestID != "" || in.ProjectID != "" || in.SharedInstructions == nil || *in.SharedInstructions != "" || in.Dependencies == nil || len(*in.Dependencies) != 0 || in.Nodes[0].Workflow.Checks == nil || len(*in.Nodes[0].Workflow.Checks) != 0 {
		t.Fatal("omission/empty choices lost")
	}
	if in.Nodes[0].OriginalText != "\ufeff café \ufffd\r\n東京" {
		t.Fatal("exact original changed")
	}
	b, e2 := json.Marshal(in)
	if e2 != nil || !strings.Contains(string(b), `"checks":[]`) || !strings.Contains(string(b), `"dependencies":[]`) {
		t.Fatal("empty choices erased", string(b), e2)
	}
}

func TestGraphDeclarationBodyLimitBeforeValueLoss(t *testing.T) {
	in := graphValidationStart()
	in.Nodes = in.Nodes[:1]
	in.Nodes[0].OriginalText = strings.Repeat("a", MaxGraphDeclarationBytes)
	raw, e := json.Marshal(in)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := StrictDecodeGraphStart(raw); e == nil || e.Code != "input_too_large" {
		t.Fatal("2MiB graph bound not enforced before admission", e)
	}
}

func TestGraphOperationExactCounterAndRebindValidation(t *testing.T) {
	a := GraphNodeAuthority{NodeID: strings.Repeat("3", 32), TaskVersion: "9007199254740993", NodeControlRevision: "1"}
	in := GraphAdvanceInput{RequestID: strings.Repeat("1", 32), GraphVersion: "1", ExpectedGraphControlRevision: "1", ExpectedBudgetAuthorityEpoch: "9007199254740993", ExpectedWindowNumber: "1", NodeAuthorities: []GraphNodeAuthority{a}}
	if e := ValidateGraphAdvanceInput(in); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*GraphAdvanceInput){func(v *GraphAdvanceInput) { v.ExpectedBudgetAuthorityEpoch = "9223372036854775808" }, func(v *GraphAdvanceInput) { v.NodeAuthorities = append(v.NodeAuthorities, a) }, func(v *GraphAdvanceInput) { ids := []string{strings.Repeat("4", 32)}; v.RebindNodeIDs = &ids }, func(v *GraphAdvanceInput) { v.ExpectedWindowNumber = "01" }} {
		copy := in
		copy.NodeAuthorities = append([]GraphNodeAuthority{}, in.NodeAuthorities...)
		mutate(&copy)
		if e := ValidateGraphAdvanceInput(copy); e == nil {
			t.Fatal("unsafe/stale proof shape accepted")
		}
	}
	stop := GraphStopInput{RequestID: in.RequestID, ExpectedGraphControlRevision: "1"}
	if e := ValidateGraphStopInput(stop); e != nil {
		t.Fatal("Stop incorrectly depends on budget/window", e)
	}
}

func TestGraphPublicRootProjectionKeepsPrivateRecoveryBinding(t *testing.T) {
	root := GraphRootBinding{GraphID: strings.Repeat("1", 32), ProjectIdentity: "private-project-native-id", RepositoryIdentity: "private-repository-native-id", SelectedRelativePath: "sub"}
	private, e := json.Marshal(root)
	if e != nil {
		t.Fatal(e)
	}
	var reopened GraphRootBinding
	if e = json.Unmarshal(private, &reopened); e != nil || !reflect.DeepEqual(root, reopened) {
		t.Fatal("private identity not durable", e)
	}
	public, e := json.Marshal(GraphRootPublic(root))
	if e != nil || strings.Contains(string(public), "private-") || strings.Contains(string(public), "projectIdentity") || strings.Contains(string(public), "repositoryIdentity") {
		t.Fatal("public projection exposes private identity", string(public), e)
	}
}
