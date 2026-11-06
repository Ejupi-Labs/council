package cli

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestExecutionCommandsMapToContractOperations(t *testing.T) {
	id := strings.Repeat("1", 32)
	other := strings.Repeat("2", 32)
	hash := strings.Repeat("a", 64)
	for _, tc := range []struct {
		args              []string
		op, method, route string
	}{
		{[]string{"tasks", "start", "--project", id, "--task", "exact", "--fake"}, "tasks.start", "POST", "/tasks"},
		{[]string{"tasks", "list", "--project", id, "--limit", "100"}, "tasks.list", "GET", "/tasks"},
		{[]string{"tasks", "get", id}, "tasks.get", "GET", "/tasks/" + id},
		{[]string{"tasks", "version", id, "--version", "9223372036854775807"}, "tasks.version.get", "GET", "/tasks/" + id + "/versions/9223372036854775807"},
		{[]string{"tasks", "guidance", id, "--expected-revision", "1", "--guidance", ""}, "tasks.guidance", "PUT", "/tasks/" + id + "/guidance"},
		{[]string{"tasks", "run", id, "--expected-revision", "1", "--task-version", "2", "--fake", "--fake-scenario", "slow"}, "tasks.attempts.start", "POST", "/tasks/" + id + "/attempts"},
		{[]string{"tasks", "attempts", id}, "tasks.attempts.list", "GET", "/tasks/" + id + "/attempts"},
		{[]string{"tasks", "accept-response", id, "--expected-revision", "1", "--task-version", "2", "--attempt", id, "--response", other, "--response-sha256", hash, "--reviewed"}, "tasks.response.accept", "POST", "/tasks/" + id + "/response-acceptance"},
		{[]string{"attempts", "get", id}, "attempts.get", "GET", "/attempts/" + id},
		{[]string{"attempts", "stop", id}, "attempts.stop", "POST", "/attempts/" + id + "/stop"},
		{[]string{"attempts", "wait", id, "--after-revision", "0", "--timeout-ms", "0"}, "attempts.wait", "GET", "/attempts/" + id + "/wait"},
		{[]string{"attempts", "events", id}, "attempts.events.list", "GET", "/attempts/" + id + "/events"},
		{[]string{"attempts", "artifacts", id}, "attempts.artifacts.list", "GET", "/attempts/" + id + "/artifacts"},
		{[]string{"artifacts", "get", id}, "artifacts.get", "GET", "/artifacts/" + id},
		{[]string{"artifacts", "read", id, "--offset", "9007199254740993", "--through", "9223372036854775807", "--limit", "65536"}, "artifacts.bytes", "GET", "/artifacts/" + id + "/bytes"},
		{[]string{"service", "stop"}, "service.stop", "POST", "/service/stop"},
	} {
		t.Run(tc.op, func(t *testing.T) {
			c, err := parse(tc.args, nil)
			if err != nil {
				t.Fatal(err)
			}
			if c.op != tc.op || c.method != tc.method || c.route != tc.route {
				t.Fatalf("wrong dispatch: %+v", c)
			}
			if c.method == "GET" {
				if c.requestID != "" || len(c.params.Body) != 0 {
					t.Fatal("read/observation became a mutation")
				}
			} else if !domain.ValidID(c.requestID) {
				t.Fatal("mutation retry identity missing")
			}
			if c.op == "artifacts.bytes" && c.params.Query["offset"] != "9007199254740993" {
				t.Fatal("large byte offset lost precision")
			}
		})
	}
}

func TestExecutionStartPreservesExactFileTextAndOptions(t *testing.T) {
	dir := t.TempDir()
	task := filepath.Join(dir, "task.txt")
	conditions := filepath.Join(dir, "conditions.txt")
	original := "\ufeff  Exact\r\n東京 � 😀\n  "
	requirements := "\ufeffConditions\r\nPreserve all bytes.\n"
	if os.WriteFile(task, []byte(original), 0600) != nil || os.WriteFile(conditions, []byte(requirements), 0600) != nil {
		t.Fatal("fixture failed")
	}
	id := strings.Repeat("1", 32)
	request := strings.Repeat("2", 32)
	c, err := parse([]string{"tasks", "start", "--project", id, "--task-file", task, "--conditions-file", conditions, "--model", "  exact-selector  ", "--harness", "codex", "--payment-path", "native", "--deliverable", "response_only", "--require-evidence", "check", "--require-evidence", "independent_review", "--final-only", "--timeout-seconds", "86400", "--fake", "--fake-scenario", "candidate", "--expected-policy-revision", "9223372036854775807", "--request-id", request}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var in domain.StartTaskInput
	if e := domain.StrictDecode(c.params.Body, &in); e != nil {
		t.Fatal(e)
	}
	if in.OriginalText != original || in.Acceptance == nil || in.Acceptance.ConditionsText != requirements || in.Pins == nil || in.Pins.Model != "  exact-selector  " || in.ExpectedPolicyRevision != "9223372036854775807" || in.ExecutionMode != "fake" || in.FakeScenario != "candidate" || in.RequestID != request || in.TimeoutSeconds == nil || *in.TimeoutSeconds != 86400 || in.Acceptance.AllowCandidatePreparation == nil || *in.Acceptance.AllowCandidatePreparation || len(in.Acceptance.RequiredEvidence) != 2 {
		t.Fatalf("captured command intent changed: %+v", in)
	}
}

func TestExecutionSourceAndGuidanceIntent(t *testing.T) {
	id := strings.Repeat("1", 32)
	draft := strings.Repeat("2", 32)
	c, err := parse([]string{"tasks", "start", "--project", id, "--task", "\ufeff", "--source-draft", draft, "--source-draft-revision", "3"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var start domain.StartTaskInput
	if domain.StrictDecode(c.params.Body, &start) != nil || start.SourceDraftRef == nil || start.SourceDraftRef.DraftID != draft || start.SourceDraftRef.Revision != "3" || start.OriginalText != "\ufeff" || start.ExecutionMode != "" {
		t.Fatal("draft provenance or native default changed")
	}
	c, err = parse([]string{"tasks", "guidance", id, "--expected-revision", "2", "--guidance", "", "--use-current-policy", "--clear-policy-override"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var guidance domain.GuidanceInput
	if domain.StrictDecode(c.params.Body, &guidance) != nil || guidance.GuidanceText != "" || !guidance.UseCurrentPolicy || !guidance.ClearPolicyOverride || guidance.Pins != nil || guidance.Acceptance != nil {
		t.Fatal("guidance omitted options overwrote version choices")
	}
}

func TestExecutionRejectsAmbiguousOrLossyInputsBeforeOwnerContact(t *testing.T) {
	id := strings.Repeat("1", 32)
	base := []string{"tasks", "start", "--project", id, "--task", "exact"}
	for _, extra := range [][]string{{"--fake-scenario", "response"}, {"--fake", "--fake"}, {"--fake", "--fake-scenario", "codex.exe"}, {"--source-draft", id}, {"--source-draft-revision", "2"}, {"--source-draft", id, "--source-draft-revision", "2", "--expected-policy-revision", "3"}, {"--require-evidence", "check", "--require-evidence", "check"}, {"--require-evidence", "unknown"}, {"--policy", "autonomous"}, {"--model", "\x00"}, {"--timeout-seconds", "0"}, {"--timeout-seconds", "01"}, {"--request-id", ""}, {"--owner-instance", id}} {
		args := append(append([]string{}, base...), extra...)
		if _, err := parse(args, nil); err == nil {
			t.Errorf("ambiguous execution accepted: %v", extra)
		}
	}
	for _, args := range [][]string{{"tasks", "start", "--project", id, "--task-file", "-", "--conditions-file", "-"}, {"tasks", "run", id, "--expected-revision", "1"}, {"tasks", "guidance", id, "--expected-revision", "1", "--guidance", "x", "--policy", "supervised", "--clear-policy-override"}, {"attempts", "wait", id, "--timeout-ms", "30001"}, {"attempts", "stop", id, "--after-revision", "1"}, {"artifacts", "read", id, "--offset", "2", "--through", "1"}, {"artifacts", "read", id, "--limit", "65537"}, {"service", "stop", "--task", "unexpected"}} {
		if _, err := parse(args, strings.NewReader("text")); err == nil {
			t.Errorf("invalid command accepted: %v", args)
		}
	}
	if _, err := parse([]string{"tasks", "start", "--project", id, "--task-file", "-"}, bytes.NewReader([]byte{0xff})); err == nil || err.Code != "invalid_encoding" {
		t.Fatal("lossy file encoding accepted")
	}
	if _, err := parse([]string{"tasks", "guidance", id, "--expected-revision", "1", "--guidance-file", "-"}, strings.NewReader(strings.Repeat("x", domain.MaxTextBytes)+"é")); err == nil || err.Code != "input_too_large" {
		t.Fatal("oversized guidance was clipped or decoded before its limit")
	}
}

func TestServiceStopCapturesOneExactOwnerAndPreservesReplayTarget(t *testing.T) {
	current := strings.Repeat("1", 32)
	old := strings.Repeat("2", 32)
	for _, args := range [][]string{{"service", "stop"}, {"service", "stop", "--owner-instance", old}} {
		c, err := parse(args, nil)
		if err != nil {
			t.Fatal(err)
		}
		bound, err := bindServiceOwner(c, current)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]string
		if json.Unmarshal(bound.params.Body, &body) != nil {
			t.Fatal("stop body invalid")
		}
		want := current
		if len(args) > 2 {
			want = old
		}
		if body["ownerInstanceId"] != want || body["requestId"] != c.requestID {
			t.Fatal("service stop target or request identity changed")
		}
	}
}

func TestInvalidExecutionJSONReturnsOneLocalEnvelope(t *testing.T) {
	var out, diagnostics bytes.Buffer
	code := Run(context.Background(), []string{"attempts", "wait", strings.Repeat("1", 32), "--timeout-ms", "-1", "--json"}, nil, &out, &diagnostics)
	if code != 2 || strings.Count(out.String(), "\n") != 1 || !strings.Contains(out.String(), `"invalid_request"`) || diagnostics.Len() != 0 {
		t.Fatalf("invalid observation contacted owner or lost JSON result: %d %q", code, out.String())
	}
}

func TestExecutionExitCategories(t *testing.T) {
	for code, want := range map[string]int{"permission_unavailable": 3, "pin_unavailable": 3, "task_version_conflict": 4, "attempt_active": 4, "dispatch_fenced": 5, "owner_closing": 5, "evidence_corrupt": 1} {
		if got := exitCode(code); got != want {
			t.Errorf("%s exit %d, want %d", code, got, want)
		}
	}
}

func TestHumanExecutionLabelsAdmissionAndByteSnapshot(t *testing.T) {
	admission := domain.Success(201, domain.NewID(), map[string]string{"phase": "queued"}, &domain.Receipt{Operation: "tasks.start"})
	if output := human(admission); !strings.HasPrefix(output, "Admitted.") {
		t.Fatal("human start did not identify admission")
	}
	chunk := domain.Success(200, domain.NewID(), map[string]any{"offset": "9007199254740993", "nextOffset": "9007199254741000", "through": "9223372036854775807", "bytesBase64": "YWJj", "completeness": "incomplete"}, nil)
	if output := human(chunk); !strings.Contains(output, "offset 9007199254740993 to 9007199254741000, snapshot bound 9223372036854775807") {
		t.Fatal("human byte output lost its exact bounded offsets")
	}
}
