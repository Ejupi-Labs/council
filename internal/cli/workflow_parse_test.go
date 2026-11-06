package cli

import (
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestWorkflowCLIParseProspectivePreferenceAndHistoricalSelectors(t *testing.T) {
	id := strings.Repeat("1", 32)
	request := strings.Repeat("2", 32)
	for _, tc := range []struct {
		args              []string
		op, route, method string
	}{
		{[]string{"workflow", "preferences", "get"}, "workflow.preferences.get", "/workflow/preferences", "GET"},
		{[]string{"workflow", "preferences", "update", "--delivery", "apply_verified_local", "--expected-revision", "1", "--request-id", request}, "workflow.preferences.update", "/workflow/preferences", "PATCH"},
		{[]string{"tasks", "workflow", id, "--task-version", "2"}, "tasks.workflow.get", "/tasks/" + id + "/workflow", "GET"},
		{[]string{"tasks", "reviews", id, "--workflow-id", request, "--limit", "1"}, "tasks.workflow.reviews.list", "/tasks/" + id + "/workflow/reviews", "GET"},
		{[]string{"candidates", "changes", id, "--limit", "1"}, "candidates.changes.list", "/candidates/" + id + "/changes", "GET"},
	} {
		c, e := parse(tc.args, nil)
		if e != nil {
			t.Fatalf("valid frozen workflow command rejected: %v: %+v", tc.args, e)
		}
		if c.op != tc.op || c.route != tc.route || c.method != tc.method {
			t.Fatalf("wrong workflow delegation: %+v", c)
		}
		if tc.method == "PATCH" {
			var body map[string]any
			if json.Unmarshal(c.params.Body, &body) != nil || body["localMode"] != "apply_verified_local" || body["requestId"] != request {
				t.Fatalf("prospective scope/request changed: %s", c.params.Body)
			}
		}
	}
}

func TestWorkflowCLIChecksFileStrictArray(t *testing.T) {
	id := strings.Repeat("1", 32)
	check := `{"key":"verify","executable":"go","arguments":[]}`
	for _, tc := range []struct {
		name, raw string
		count     int
	}{
		{"explicit empty", "[]", 0},
		{"one verifier", "[" + check + "]", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, e := parse([]string{"tasks", "start", "--project", id, "--task", "original", "--checks-file", "-"}, strings.NewReader(tc.raw))
			if e != nil {
				t.Fatalf("valid checks array rejected: %+v", e)
			}
			var body struct {
				Workflow struct {
					Checks []map[string]any `json:"checks"`
				} `json:"workflow"`
			}
			if err := json.Unmarshal(c.params.Body, &body); err != nil || body.Workflow.Checks == nil || len(body.Workflow.Checks) != tc.count {
				t.Fatalf("explicit array changed: %s %v", c.params.Body, err)
			}
		})
	}
	for _, raw := range []string{
		"null", "{}", "[null]", "[" + check + "] []",
		`[{"key":"a","key":"b","executable":"go","arguments":[]}]`,
		`[{"key":"a","executable":"go","arguments":[],"unexpected":true}]`,
		`[{"key":"a","executable":"go","arguments":null}]`,
	} {
		if _, e := parse([]string{"tasks", "start", "--project", id, "--task", "original", "--checks-file", "-"}, strings.NewReader(raw)); e == nil {
			t.Fatalf("accepted malformed checks array: %s", raw)
		}
	}
}

func TestWorkflowCLIRejectsRebindingAndContradictoryOptions(t *testing.T) {
	id := strings.Repeat("1", 32)
	for _, args := range [][]string{
		{"tasks", "reviews", id, "--workflow-id", id, "--task-version", "1"},
		{"tasks", "advance", id, "--expected-revision", "1", "--workflow-revision", "1", "--task-version", "1", "--role", "correction"},
		{"tasks", "start", "--project", id, "--task", "original", "--workflow", "author_only", "--review", "required"},
		{"tasks", "start", "--project", id, "--task", "original", "--delivery", "review_only", "--delivery-file", "-"},
		{"tasks", "start", "--project", id, "--task", "original", "--max-corrections", "2"},
		{"workflow", "preferences", "update", "--delivery", "saved", "--expected-revision", "1"},
	} {
		if _, e := parse(args, strings.NewReader(`{"localMode":"apply_verified_local"}`)); e == nil {
			t.Fatalf("accepted ambiguous/new authority: %v", args)
		}
	}
}
