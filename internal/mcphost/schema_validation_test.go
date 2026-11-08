package mcphost

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

func validationRequest(t *testing.T, tool string, raw []byte) (ToolMapping, ownerclient.Request) {
	t.Helper()
	for _, mapping := range ToolCatalog() {
		if mapping.Name == tool {
			req, err := OwnerRequest(mapping, raw)
			if err != nil {
				t.Fatalf("OwnerRequest %s: %v", tool, err)
			}
			return mapping, req
		}
	}
	t.Fatalf("unknown fixture tool %s", tool)
	return ToolMapping{}, ownerclient.Request{}
}

func TestValidateArgumentsPreservesExactMutation(t *testing.T) {
	raw := []byte(`{"input": { "requestId": "0123456789abcdef0123456789abcdef", "projectId":"fedcba9876543210fedcba9876543210", "originalText":"Keep  all  spaces  and \u00e9" }}`)
	mapping, req := validationRequest(t, "council_drafts_create", raw)
	beforeBody := append([]byte(nil), req.Body...)
	beforeRaw := append([]byte(nil), raw...)
	beforeReq := req
	if err := ValidateArguments(mapping.Name, mapping.Operation, raw, req); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(req.Body, beforeBody) || !bytes.Equal(raw, beforeRaw) || !reflect.DeepEqual(req, beforeReq) {
		t.Fatal("validation changed frozen input")
	}
	if !bytes.Contains(req.Body, []byte(`\u00e9`)) {
		t.Fatal("original JSON escape was changed")
	}
	changed := req
	changed.Body = bytes.ReplaceAll(req.Body, []byte("Keep  all"), []byte("Keep all"))
	if err := ValidateArguments(mapping.Name, mapping.Operation, raw, changed); err == nil {
		t.Fatal("changed body accepted")
	}
	changed = req
	changed.RequestID = "fedcba9876543210fedcba9876543210"
	if err := ValidateArguments(mapping.Name, mapping.Operation, raw, changed); err == nil {
		t.Fatal("changed request identity accepted")
	}
	if err := ValidateArguments(mapping.Name, "policy.set", raw, req); err == nil {
		t.Fatal("changed operation accepted")
	}
}

func TestValidateArgumentsReadBindingsAndOmission(t *testing.T) {
	const first = "0123456789abcdef0123456789abcdef"
	const second = "fedcba9876543210fedcba9876543210"
	raw := []byte(`{"graphId":"` + first + `","inputId":"` + second + `","limit":"5"}`)
	mapping, req := validationRequest(t, "council_graphs_inputs_members", raw)
	if err := ValidateArguments(mapping.Name, mapping.Operation, raw, req); err != nil {
		t.Fatal(err)
	}
	changed := req
	changed.PathIDs = []string{second, first}
	if err := ValidateArguments(mapping.Name, mapping.Operation, raw, changed); err == nil {
		t.Fatal("swapped resource identities accepted")
	}
	changed = req
	changed.Query = map[string]string{"limit": "6"}
	if err := ValidateArguments(mapping.Name, mapping.Operation, raw, changed); err == nil {
		t.Fatal("changed query accepted")
	}
	changed = req
	changed.Query = map[string]string{"limit": "5", "cursor": "abc"}
	if err := ValidateArguments(mapping.Name, mapping.Operation, raw, changed); err == nil {
		t.Fatal("added query accepted")
	}

	emptyMapping, emptyReq := validationRequest(t, "council_owner_meta", nil)
	if err := ValidateArguments(emptyMapping.Name, emptyMapping.Operation, nil, emptyReq); err != nil {
		t.Fatalf("omitted arguments: %v", err)
	}
	if err := ValidateArguments(emptyMapping.Name, emptyMapping.Operation, []byte(`{}`), emptyReq); err != nil {
		t.Fatalf("empty arguments: %v", err)
	}
	if err := ValidateArguments(emptyMapping.Name, emptyMapping.Operation, []byte(`null`), emptyReq); err == nil {
		t.Fatal("null arguments accepted")
	}
	if err := ValidateArguments(mapping.Name, mapping.Operation, nil, req); err == nil {
		t.Fatal("missing required arguments accepted")
	}
}

func TestValidateArgumentsRejectsMalformedAndSanitizes(t *testing.T) {
	const secret = "fixture-secret-do-not-echo"
	base := []byte(`{"input":{"requestId":"0123456789abcdef0123456789abcdef","projectId":"fedcba9876543210fedcba9876543210","originalText":"work"}}`)
	mapping, req := validationRequest(t, "council_drafts_create", base)
	for _, raw := range [][]byte{
		[]byte(`{"input":{"requestId":"0123456789abcdef0123456789abcdef","requestId":"0123456789abcdef0123456789abcdef","projectId":"fedcba9876543210fedcba9876543210","originalText":"work"}}`),
		[]byte(`{"input":{"requestId":"0123456789abcdef0123456789abcdef","projectId":"fedcba9876543210fedcba9876543210","originalText":null}}`),
		[]byte(`{"input":{"requestId":"0123456789abcdef0123456789abcdef","projectId":"fedcba9876543210fedcba9876543210","originalText":"work","` + secret + `":true}}`),
		[]byte(`{"input":{"requestId":"0123456789abcdef0123456789abcdef","projectId":"fedcba9876543210fedcba9876543210","originalText":"\ud800"}}`),
		append(append([]byte(nil), base...), []byte(` {}`)...),
	} {
		err := ValidateArguments(mapping.Name, mapping.Operation, raw, req)
		if err == nil {
			t.Fatalf("invalid input accepted: %s", raw)
		}
		if strings.Contains(err.Message, secret) {
			t.Fatal("error included user input")
		}
	}
}

func TestValidateArgumentsUsesOwnerSemanticChecks(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	raw := []byte(`{"graphId":"` + id + `","input":{"requestId":"` + id + `","graphVersion":"1","expectedGraphControlRevision":"1","expectedBudgetAuthorityEpoch":"1","expectedWindowNumber":"1","nodeAuthorities":[{"nodeId":"` + id + `","taskVersion":"1","nodeControlRevision":"1"},{"nodeId":"` + id + `","taskVersion":"2","nodeControlRevision":"2"}]}}`)
	mapping, req := validationRequest(t, "council_graphs_advance", raw)
	if err := ValidateArguments(mapping.Name, mapping.Operation, raw, req); err == nil {
		t.Fatal("duplicate node authority accepted")
	}
	queryRaw := []byte(`{"snapshotId":"` + id + `","kind":"tasks","cursor":"invalid"}`)
	mapping, req = validationRequest(t, "council_history_records", queryRaw)
	if err := ValidateArguments(mapping.Name, mapping.Operation, queryRaw, req); err == nil {
		t.Fatal("malformed history cursor accepted")
	}
}

func TestStrictArgumentValueKeepsIntegersExact(t *testing.T) {
	value, err := strictArgumentValue([]byte(`{"n":9007199254740993,"max":9223372036854775807}`))
	if err != nil {
		t.Fatal(err)
	}
	obj := value.(map[string]any)
	if obj["n"] != int64(9007199254740993) || obj["max"] != int64(9223372036854775807) {
		t.Fatalf("rounded input: %v", obj)
	}
	for _, value := range []string{`{"n":9223372036854775808}`, `{"n":1.0}`, `{"n":1e0}`, `{"n":null}`} {
		if _, err := strictArgumentValue([]byte(value)); err == nil {
			t.Fatalf("invalid numeric/null input %s", value)
		}
	}
	// The standard decoder alone represents these numbers as json.Number; the
	// validator deliberately never converts them via float64.
	var raw json.Number = "9007199254740993"
	value, err = exactSchemaNumbers(raw)
	if err != nil || value != int64(9007199254740993) {
		t.Fatal("json.Number lost precision")
	}
}
