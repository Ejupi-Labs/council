package cli

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphDeclarationFixture() map[string]any {
	return map[string]any{
		"requestId": strings.Repeat("1", 32), "projectId": strings.Repeat("2", 32),
		"sharedInstructions": "\ufeff  Preserve\r\n東京 😀\n  ", "title": "Independent results",
		"nodes": []map[string]any{
			{"key": "A_1", "originalText": "\ufeffFirst\r\n東京 😀  ", "workflow": map[string]any{"checks": []any{}}},
			{"key": "B-2", "originalText": "Second\r\nKeep this exact."},
		},
		"dependencies": []any{}, "executionMode": "fake",
	}
}

func graphTestJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func graphParsedObject(t *testing.T, c command) map[string]jsontext.Value {
	t.Helper()
	var body map[string]jsontext.Value
	if err := json.Unmarshal(c.params.Body, &body); err != nil || body == nil {
		t.Fatalf("missing strict graph request: %s %v", c.params.Body, err)
	}
	return body
}

func TestGraphStartWholeFilePreservesDeclarationAndAbsentDefaults(t *testing.T) {
	declaration := graphDeclarationFixture()
	raw := graphTestJSON(t, declaration)
	filename := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(filename, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"-", filename} {
		t.Run(source, func(t *testing.T) {
			c, err := parse([]string{"graphs", "start", "--graph-file", source, "--json"}, strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if c.op != "graphs.start" || c.method != "POST" || c.route != "/graphs" || c.requestID != declaration["requestId"] || !c.json {
				t.Fatalf("wrong graph admission dispatch: %+v", c)
			}
			body := graphParsedObject(t, c)
			for _, key := range []string{"routing", "delivery", "finalNodeKey", "maxParallelism", "policyOverride"} {
				if _, ok := body[key]; ok {
					t.Fatalf("transport injected absent %s", key)
				}
			}
			var decoded, expected map[string]any
			if err := json.Unmarshal([]byte(raw), &expected); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(c.params.Body, &decoded); err != nil || !reflect.DeepEqual(decoded, expected) {
				t.Fatalf("declaration changed: %s %v", c.params.Body, err)
			}
		})
	}
}

func TestGraphStartTransportFlagsOnlySupplyAbsentIdentity(t *testing.T) {
	project, request := strings.Repeat("2", 32), strings.Repeat("1", 32)
	for _, fieldsInFile := range []bool{false, true} {
		declaration := graphDeclarationFixture()
		if !fieldsInFile {
			delete(declaration, "projectId")
			delete(declaration, "requestId")
		}
		c, err := parse([]string{"graphs", "start", "--graph-file", "-", "--project", project, "--request-id", request}, strings.NewReader(graphTestJSON(t, declaration)))
		if err != nil {
			t.Fatal(err)
		}
		body := graphParsedObject(t, c)
		if string(body["projectId"]) != `"`+project+`"` || string(body["requestId"]) != `"`+request+`"` || c.requestID != request {
			t.Fatal("transport lost supplied exact identities", string(c.params.Body))
		}
	}
	for _, flag := range []string{"project", "request-id"} {
		if _, err := parse([]string{"graphs", "start", "--graph-file", "-", "--" + flag, strings.Repeat("3", 32)}, strings.NewReader(graphTestJSON(t, graphDeclarationFixture()))); err == nil {
			t.Fatal("conflicting declaration identity overwritten", flag)
		}
	}
}

func TestGraphStartRejectsMalformedAndForbiddenWholeInput(t *testing.T) {
	valid := graphTestJSON(t, graphDeclarationFixture())
	for name, raw := range map[string]string{
		"null": "null", "array": "[]", "trailing": valid + " {}", "invalid_utf8": valid + string([]byte{0xff}),
		"duplicate_top":    strings.Replace(valid, `"title":`, `"title":"first","title":`, 1),
		"duplicate_nested": strings.Replace(valid, `"key":"A_1"`, `"key":"first","key":"A_1"`, 1),
		"unknown_top":      strings.Replace(valid, `"title":`, `"graphId":null,"title":`, 1),
		"oversize":         valid + strings.Repeat(" ", (2<<20)+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parse([]string{"graphs", "start", "--graph-file", "-"}, strings.NewReader(raw)); err == nil {
				t.Fatal("invalid whole declaration accepted")
			}
		})
	}
	for _, change := range []string{"unknown_node", "null_child_budget", "null_child_delivery", "author_only", "duplicate_key", "unknown_edge", "duplicate_edge", "self_edge", "cycle", "delivery_without_final", "unknown_final", "unreachable_final", "numeric_parallelism", "unsafe_policy_revision", "null_original"} {
		t.Run(change, func(t *testing.T) {
			declaration := graphDeclarationFixture()
			nodes := declaration["nodes"].([]map[string]any)
			switch change {
			case "unknown_node":
				nodes[0]["taskId"] = nil
			case "null_child_budget":
				nodes[0]["routing"] = map[string]any{"budget": nil}
			case "null_child_delivery":
				nodes[0]["workflow"] = map[string]any{"delivery": nil}
			case "author_only":
				nodes[0]["workflow"] = map[string]any{"mode": "author_only"}
			case "duplicate_key":
				nodes[1]["key"] = nodes[0]["key"]
			case "unknown_edge":
				declaration["dependencies"] = []any{map[string]any{"from": "A_1", "to": "missing"}}
			case "duplicate_edge":
				declaration["dependencies"] = []any{map[string]any{"from": "A_1", "to": "B-2"}, map[string]any{"from": "A_1", "to": "B-2", "kind": "files"}}
			case "self_edge":
				declaration["dependencies"] = []any{map[string]any{"from": "A_1", "to": "A_1"}}
			case "cycle":
				declaration["dependencies"] = []any{map[string]any{"from": "A_1", "to": "B-2"}, map[string]any{"from": "B-2", "to": "A_1"}}
			case "delivery_without_final":
				declaration["delivery"] = map[string]any{"localMode": "review_only"}
			case "unknown_final":
				declaration["finalNodeKey"] = "missing"
			case "unreachable_final":
				declaration["finalNodeKey"] = "B-2"
			case "numeric_parallelism":
				declaration["maxParallelism"] = 2
			case "unsafe_policy_revision":
				declaration["expectedPolicyRevision"] = "9223372036854775808"
			case "null_original":
				nodes[0]["originalText"] = nil
			}
			if _, err := parse([]string{"graphs", "start", "--graph-file", "-"}, strings.NewReader(graphTestJSON(t, declaration))); err == nil {
				t.Fatal("forbidden graph declaration accepted")
			}
		})
	}
	for _, args := range [][]string{
		{"graphs", "start"}, {"graphs", "start", "--graph-file", "-", "--graph-file", "-"},
		{"graphs", "start", "--graph-file", "-", "--task", "injected"},
		{"graphs", "start", "--graph-file", "-", "--fake"},
		{"tasks", "start", "--graph-file", "-", "--project", strings.Repeat("2", 32), "--task", "standalone"},
	} {
		if _, err := parse(args, strings.NewReader(valid)); err == nil {
			t.Fatal("graph declaration transport accepted contradictory options", args)
		}
	}
}

func TestGraphReadCommandsMapExactObservationSelectors(t *testing.T) {
	id, other := strings.Repeat("1", 32), strings.Repeat("2", 32)
	for _, tc := range []struct {
		args      []string
		op, route string
		query     map[string]string
	}{
		{[]string{"graphs", "list", "--project", id, "--limit", "100"}, "graphs.list", "/graphs", map[string]string{"projectId": id, "limit": "100"}},
		{[]string{"graphs", "get", id}, "graphs.get", "/graphs/" + id, map[string]string{}},
		{[]string{"graphs", "definition", id}, "graphs.definition.get", "/graphs/" + id + "/definition", map[string]string{}},
		{[]string{"graphs", "wait", id, "--after-revision", "9007199254740993", "--timeout-ms", "0"}, "graphs.wait", "/graphs/" + id + "/wait", map[string]string{"afterRevision": "9007199254740993", "timeoutMs": "0"}},
		{[]string{"graphs", "budget", id}, "graphs.budget.get", "/graphs/" + id + "/budget", map[string]string{}},
		{[]string{"graphs", "budget-ledger", id}, "graphs.budget.ledger", "/graphs/" + id + "/budget-ledger", map[string]string{}},
		{[]string{"graphs", "inputs", id}, "graphs.inputs.list", "/graphs/" + id + "/inputs", map[string]string{}},
		{[]string{"graphs", "input", id, "--input", other}, "graphs.inputs.get", "/graphs/" + id + "/inputs/" + other, map[string]string{}},
		{[]string{"graphs", "input-members", id, "--input", other}, "graphs.inputs.members", "/graphs/" + id + "/inputs/" + other + "/members", map[string]string{}},
		{[]string{"graphs", "dispatches", id, "--input", other, "--node", "A_1"}, "graphs.dispatches.list", "/graphs/" + id + "/dispatches", map[string]string{"inputId": other, "nodeKey": "A_1"}},
		{[]string{"graphs", "dispatch", id, "--dispatch", other}, "graphs.dispatches.get", "/graphs/" + id + "/dispatches/" + other, map[string]string{}},
	} {
		t.Run(tc.op, func(t *testing.T) {
			c, err := parse(tc.args, nil)
			if err != nil {
				t.Fatal(err)
			}
			if c.op != tc.op || c.route != tc.route || c.method != "GET" || c.requestID != "" || len(c.params.Body) != 0 || !reflect.DeepEqual(c.params.Query, tc.query) {
				t.Fatalf("read acquired mutation or wrong selector: %+v", c)
			}
		})
	}
	for _, args := range [][]string{
		{"graphs", "get", "not-an-id"}, {"graphs", "get", id, "--request-id", other},
		{"graphs", "wait", id, "--after-revision", "9223372036854775808"},
		{"graphs", "wait", id, "--timeout-ms", "-1"}, {"graphs", "list", "--limit", "101"},
		{"graphs", "dispatches", id, "--node", "../A"}, {"graphs", "input", id}, {"graphs", "dispatch", id, "--dispatch", "bad"},
	} {
		if _, err := parse(args, nil); err == nil {
			t.Fatal("invalid graph read accepted", args)
		}
	}
}

func TestGraphExplicitProofsAreCompleteAndLossless(t *testing.T) {
	id, request, node := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	vector := `[{"nodeId":"` + node + `","taskVersion":"9007199254740993","nodeControlRevision":"9223372036854775807"}]`
	base := []string{"graphs", "advance", id, "--request-id", request, "--graph-version", "1", "--graph-control-revision", "2", "--budget-authority-epoch", "3", "--window-number", "4", "--node-authorities-file", "-"}
	c, err := parse(base, strings.NewReader(vector))
	if err != nil {
		t.Fatal(err)
	}
	body := graphParsedObject(t, c)
	if c.op != "graphs.advance" || c.route != "/graphs/"+id+"/advance" || c.method != "POST" || c.requestID != request || string(body["nodeAuthorities"]) != vector {
		t.Fatalf("explicit proof changed: %+v %s", c, c.params.Body)
	}
	if _, ok := body["rebindNodeIds"]; ok {
		t.Fatal("ordinary advance injected rebind intent")
	}
	for _, flags := range [][]string{
		{"--graph-version", "1"}, {"--graph-control-revision", "2", "--budget-authority-epoch", "3", "--window-number", "4"},
		{"--graph-version", "1", "--graph-control-revision", "2", "--budget-authority-epoch", "3", "--window-number", "4"},
	} {
		args := append([]string{"graphs", "advance", id}, flags...)
		if _, err := parse(args, strings.NewReader(vector)); err == nil {
			t.Fatal("partial proof group accepted", args)
		}
	}
	for _, invalid := range []string{"null", "[]", vector + ` []`, strings.Replace(vector, `"nodeId":`, `"other":null,"nodeId":`, 1), strings.Replace(vector, `"taskVersion":"9007199254740993"`, `"taskVersion":9007199254740993`, 1), strings.Replace(vector, `"taskVersion":"9007199254740993"`, `"taskVersion":"9223372036854775808"`, 1)} {
		if _, err := parse(base, strings.NewReader(invalid)); err == nil {
			t.Fatal("malformed or incomplete authority vector accepted", invalid)
		}
	}
	if _, err := parse([]string{"graphs", "stop", id, "--request-id", request, "--graph-control-revision", "9223372036854775807"}, nil); err != nil {
		t.Fatal(err)
	}
	if !domain.ValidID(request) {
		t.Fatal("fixture identity")
	}
}

func TestGraphNodeControlsAndGrantUseExactDeclaredScope(t *testing.T) {
	graph, node, request := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	proofs := []string{"--request-id", request, "--graph-control-revision", "4", "--node", "A_1", "--node-id", node, "--node-control-revision", "5"}
	stopArgs := append([]string{"graphs", "stop-node", graph}, proofs...)
	stop, err := parse(stopArgs, nil)
	if err != nil {
		t.Fatal(err)
	}
	var stopInput domain.GraphNodeStopInput
	if domain.StrictDecode(stop.params.Body, &stopInput) != nil || stopInput.NodeID != node || stopInput.ExpectedGraphControlRevision != "4" || stopInput.ExpectedNodeControlRevision != "5" || stop.op != "graphs.nodes.stop" || stop.route != "/graphs/"+graph+"/nodes/A_1/stop" || stop.method != "POST" {
		t.Fatalf("node Stop lost its exact identity/proofs: %+v %s", stop, stop.params.Body)
	}
	guideArgs := append([]string{"graphs", "guide", graph}, proofs...)
	guideArgs = append(guideArgs, "--expected-revision", "6", "--task-version", "7", "--guidance", "", "--checks-file", "-")
	guide, err := parse(guideArgs, strings.NewReader("[]"))
	if err != nil {
		t.Fatal(err)
	}
	var guidance domain.GraphGuidanceInput
	if domain.StrictDecode(guide.params.Body, &guidance) != nil || guidance.GuidanceText != "" || guidance.Workflow == nil || guidance.Workflow.Checks == nil || len(*guidance.Workflow.Checks) != 0 || guidance.NodeID != node || guidance.TaskVersion != "7" || guide.op != "graphs.nodes.guidance" || guide.route != "/graphs/"+graph+"/nodes/A_1/guidance" {
		t.Fatalf("guidance lost empty choices or node ownership: %+v %s", guide, guide.params.Body)
	}
	grant, err := parse([]string{"graphs", "grant", graph, "--request-id", request, "--graph-control-revision", "4", "--budget-authority-epoch", "5", "--window-number", "6", "--additional-admissions", "16", "--window-seconds", "86400"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var amendment domain.GraphBudgetAmendmentInput
	if domain.StrictDecode(grant.params.Body, &amendment) != nil || amendment.ExpectedBudgetAuthorityEpoch != "5" || amendment.ExpectedWindowNumber != "6" || amendment.AdditionalAdmissions != "16" || amendment.NewWindowSeconds == nil || *amendment.NewWindowSeconds != 86400 || grant.op != "graphs.budget.amend" || grant.route != "/graphs/"+graph+"/budget-amendments" {
		t.Fatalf("grant lost finite graph-owned authority: %+v %s", grant, grant.params.Body)
	}
	for _, extra := range [][]string{{"--delivery", "apply_verified_local"}, {"--workflow", "author_only"}, {"--budget-admissions", "4"}} {
		args := append(append([]string{}, guideArgs...), extra...)
		if _, err := parse(args, strings.NewReader("[]")); err == nil {
			t.Fatal("node control expanded scope or created allowance", args)
		}
	}
	for _, op := range []string{stop.op, guide.op, grant.op, "graphs.start", "graphs.get"} {
		if !requiresExecutionOwner(op) {
			t.Fatal("graph operation bypassed exact execution owner", op)
		}
	}
}

func TestGraphAutomaticProofCaptureRemainsDeferredUntilOwnerGuard(t *testing.T) {
	id := strings.Repeat("1", 32)
	for _, args := range [][]string{
		{"graphs", "advance", id}, {"graphs", "stop", id},
		{"graphs", "stop-node", id, "--node", "A"},
		{"graphs", "guide", id, "--node", "A", "--guidance", "same exact text"},
		{"graphs", "grant", id, "--additional-admissions", "1"},
		{"graphs", "advance", id, "--rebind-node", "A"},
	} {
		c, err := parse(args, nil)
		if err != nil || c.graphIntent == nil || !domain.ValidID(c.requestID) || len(c.params.Body) == 0 {
			t.Fatalf("automatic control lost its pending owner-bound intent: %+v %v", c, err)
		}
		if validateFrozenGraphBody(c) == nil {
			t.Fatal("uncaptured control body passed the final forwarding guard")
		}
	}
}
