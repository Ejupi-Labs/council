package schemas

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

const fixtureID = "0123456789abcdef0123456789abcdef"

func queryPath(t *testing.T, route ownerclient.Route) []string {
	t.Helper()
	ids := make([]string, 0, len(route.PathKinds))
	for _, kind := range route.PathKinds {
		switch kind {
		case "id":
			ids = append(ids, fixtureID)
		case "revision":
			ids = append(ids, "1")
		case "context":
			ids = append(ids, "codex-native")
		case "profile":
			ids = append(ids, "codex-native-default")
		case "graphkey":
			ids = append(ids, "fixture-node")
		default:
			t.Fatalf("unreviewed path kind %s", kind)
		}
	}
	return ids
}

func queryFixture(operation string) map[string]string {
	switch operation {
	case "history.snapshots.records":
		return map[string]string{"kind": "tasks"}
	case "routing.capacity.get", "routing.reports.list":
		return map[string]string{"contextId": "codex-native"}
	default:
		return map[string]string{}
	}
}

func TestOwnerQueryFixedRouteCoverageAndPaths(t *testing.T) {
	count := 0
	for _, route := range ownerclient.Routes() {
		ids := queryPath(t, route)
		if route.Method != "GET" {
			if _, ok := ValidateOwnerQuery(route.Operation, ids, nil); ok {
				t.Fatalf("mutation %s enabled as read", route.Operation)
			}
			continue
		}
		count++
		query := queryFixture(route.Operation)
		if err, ok := ValidateOwnerQuery(route.Operation, ids, query); !ok || err != nil {
			t.Fatalf("valid %s: reviewed=%v err=%v", route.Operation, ok, err)
		}
		query["unexpected"] = "1"
		if err, ok := ValidateOwnerQuery(route.Operation, ids, query); !ok || err == nil {
			t.Fatalf("%s accepted unknown query", route.Operation)
		}
		if len(ids) > 0 {
			bad := append([]string(nil), ids...)
			bad[0] = "invalid/identity"
			if err, ok := ValidateOwnerQuery(route.Operation, bad, queryFixture(route.Operation)); !ok || err == nil {
				t.Fatalf("%s accepted invalid path", route.Operation)
			}
			if err, ok := ValidateOwnerQuery(route.Operation, ids[:len(ids)-1], queryFixture(route.Operation)); !ok || err == nil {
				t.Fatalf("%s accepted incomplete path", route.Operation)
			}
		}
	}
	if count != len(reviewedReadOperations) {
		t.Fatalf("read registry drift: %d routes, %d validators", count, len(reviewedReadOperations))
	}
	if _, ok := ValidateOwnerQuery("unrecognized", nil, nil); ok {
		t.Fatal("unknown read enabled")
	}
}

func TestOwnerQueryNumbersSelectorsAndRanges(t *testing.T) {
	cases := []struct {
		operation string
		ids       []string
		query     map[string]string
		valid     bool
	}{
		{"projects.list", nil, map[string]string{"limit": "100"}, true},
		{"projects.list", nil, map[string]string{"limit": "101"}, false},
		{"projects.list", nil, map[string]string{"limit": "01"}, false},
		{"projects.list", nil, map[string]string{"limit": ""}, false},
		{"tasks.version.get", []string{fixtureID, "9223372036854775807"}, nil, true},
		{"tasks.version.get", []string{fixtureID, "9223372036854775808"}, nil, false},
		{"attempts.wait", []string{fixtureID}, map[string]string{"afterRevision": "0", "timeoutMs": "30000"}, true},
		{"attempts.wait", []string{fixtureID}, map[string]string{"afterRevision": "9223372036854775808"}, false},
		{"attempts.wait", []string{fixtureID}, map[string]string{"timeoutMs": "30001"}, false},
		{"attempts.wait", []string{fixtureID}, map[string]string{"timeoutMs": "+1"}, false},
		{"artifacts.bytes", []string{fixtureID}, map[string]string{"offset": "1", "through": "1", "limit": "65536"}, true},
		{"artifacts.bytes", []string{fixtureID}, map[string]string{"offset": "2", "through": "1"}, false},
		{"artifacts.bytes", []string{fixtureID}, map[string]string{"limit": "65537"}, false},
		{"artifacts.bytes", []string{fixtureID}, map[string]string{"offset": "-1"}, false},
		{"routing.capacity.get", nil, map[string]string{"contextId": "codex-native"}, true},
		{"routing.capacity.get", nil, map[string]string{"accountId": fixtureID}, true},
		{"routing.capacity.get", nil, nil, false},
		{"routing.capacity.get", nil, map[string]string{"contextId": "codex-native", "accountId": fixtureID}, false},
		{"routing.capacity.get", nil, map[string]string{"contextId": "unknown"}, false},
		{"tasks.workflow.get", []string{fixtureID}, map[string]string{"workflowId": fixtureID}, true},
		{"tasks.workflow.get", []string{fixtureID}, map[string]string{"taskVersion": "1"}, true},
		{"tasks.workflow.get", []string{fixtureID}, map[string]string{"workflowId": fixtureID, "taskVersion": "1"}, false},
		{"history.snapshots.records", []string{fixtureID}, map[string]string{"kind": "attempts", "limit": "100"}, true},
		{"history.snapshots.records", []string{fixtureID}, nil, false},
		{"history.snapshots.records", []string{fixtureID}, map[string]string{"kind": "other"}, false},
		{"graphs.dispatches.list", []string{fixtureID}, map[string]string{"nodeKey": "node-1", "inputId": fixtureID}, true},
		{"graphs.dispatches.list", []string{fixtureID}, map[string]string{"nodeKey": "../bad"}, false},
	}
	for _, tc := range cases {
		err, reviewed := ValidateOwnerQuery(tc.operation, tc.ids, tc.query)
		if !reviewed || (err == nil) != tc.valid {
			t.Errorf("%s %v: valid=%v, reviewed=%v err=%v", tc.operation, tc.query, tc.valid, reviewed, err)
		}
	}
}

func TestOwnerQueryCursorFormatWithoutState(t *testing.T) {
	encode := base64.RawURLEncoding.EncodeToString
	plain := encode([]byte("untrusted fixture"))
	history := encode([]byte("untrusted payload")) + "." + encode(make([]byte, 32))
	valid := []struct {
		operation, cursor string
		ids               []string
		query             map[string]string
	}{
		{"projects.list", plain, nil, map[string]string{}},
		{"graphs.inputs.members", plain, []string{fixtureID, fixtureID}, map[string]string{}},
		{"history.snapshots.records", history, []string{fixtureID}, map[string]string{"kind": "tasks"}},
	}
	for _, tc := range valid {
		tc.query["cursor"] = tc.cursor
		if err, ok := ValidateOwnerQuery(tc.operation, tc.ids, tc.query); !ok || err != nil {
			t.Fatalf("format-only %s: %v", tc.operation, err)
		}
	}
	invalid := []struct {
		operation, cursor string
		ids               []string
		query             map[string]string
	}{
		{"projects.list", "A", nil, map[string]string{}},
		{"projects.list", plain + "\n", nil, map[string]string{}},
		{"projects.list", strings.Repeat("a", 2049), nil, map[string]string{}},
		{"history.snapshots.records", plain, []string{fixtureID}, map[string]string{"kind": "tasks"}},
		{"history.snapshots.records", plain + "." + encode([]byte("short")), []string{fixtureID}, map[string]string{"kind": "tasks"}},
		{"history.snapshots.records", strings.Repeat("a", 513), []string{fixtureID}, map[string]string{"kind": "tasks"}},
	}
	for _, tc := range invalid {
		tc.query["cursor"] = tc.cursor
		if err, ok := ValidateOwnerQuery(tc.operation, tc.ids, tc.query); !ok || err == nil {
			t.Fatalf("accepted malformed cursor for %s", tc.operation)
		}
	}
}
