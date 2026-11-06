package adapters

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func roleFixture(t *testing.T, harness string) RoleInput {
	t.Helper()
	root := t.TempDir()
	return RoleInput{Role: "review", Original: "\ufeffOriginal café 東京 �\r\n", Version: domain.TaskVersion{RequestedPolicy: domain.Policy{Mode: "autonomous"}, Pins: domain.ExecutionPins{HarnessID: harness, Model: "literal-alias"}}, Subject: domain.ReviewSubject{Kind: "candidate", ID: strings.Repeat("a", 32), SHA256: strings.Repeat("b", 64)}, ConditionsSHA256: strings.Repeat("c", 64), Evidence: []SuppliedEvidence{{Ref: "candidate-manifest", Path: "candidate-manifest.json", Scope: "whole_candidate", Paths: []string{}}}, Findings: []domain.ReviewFinding{}, SchemaPath: filepath.Join(root, "schema.json"), OutputPath: filepath.Join(root, "response.json")}
}

func TestReviewNativeBuilderUsesFreshSchemaAndReadAccess(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			in := roleFixture(t, harness)
			launch, err := BuildRole(context.Background(), in, func(context.Context, domain.ExecutionPins) (string, error) {
				return filepath.Join(t.TempDir(), "controlled-native"), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(launch.Args, "\x00")
			if harness == "codex" && !strings.Contains(joined, "--output-schema\x00"+in.SchemaPath) {
				t.Fatalf("review used author profile without native schema: %q", launch.Args)
			}
			if harness == "claude-code" && (!strings.Contains(joined, "--json-schema\x00") || !strings.Contains(joined, "--permission-mode\x00dontAsk")) {
				t.Fatalf("review used author profile without read-only tools/schema: %q", launch.Args)
			}
			if strings.Contains(joined, "danger-full-access") || strings.Contains(joined, "bypassPermissions") || strings.Contains(joined, "resume") {
				t.Fatalf("review retained author privilege/session: %q", launch.Args)
			}
		})
	}
}

func TestReviewPermissionProfileDoesNotRewriteLiteralModelSelector(t *testing.T) {
	for _, example := range []struct{ harness, model, permissionFlag, permissionValue string }{
		{"codex", "danger-full-access", "--sandbox", "read-only"},
		{"claude-code", "bypassPermissions", "--permission-mode", "dontAsk"},
	} {
		t.Run(example.harness, func(t *testing.T) {
			in := roleFixture(t, example.harness)
			in.Version.Pins.Model = example.model
			launch, err := BuildRole(context.Background(), in, func(context.Context, domain.ExecutionPins) (string, error) {
				return filepath.Join(t.TempDir(), "controlled-native"), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			values := map[string]string{}
			for index := 0; index+1 < len(launch.Args); index++ {
				if launch.Args[index] == "--model" || launch.Args[index] == example.permissionFlag {
					values[launch.Args[index]] = launch.Args[index+1]
				}
			}
			if values["--model"] != example.model || values[example.permissionFlag] != example.permissionValue || in.Version.Pins.Model != example.model {
				t.Fatalf("review permission profile rewrote literal model intent: %q", launch.Args)
			}
		})
	}
}

func TestReviewPromptCarriesExactCheckReceiptsAndNormalizesEmptyArray(t *testing.T) {
	for _, populated := range []bool{false, true} {
		in := roleFixture(t, "claude-code")
		if populated {
			exit := 1
			in.Checks = []domain.CheckReceipt{{ID: strings.Repeat("d", 32), DefinitionID: strings.Repeat("e", 32), DefinitionSHA256: strings.Repeat("f", 64), Outcome: "failed", RootTerminal: true, InputComplete: true, OutputComplete: true, ExitCode: &exit, Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, Applicability: domain.Applicability{Paths: []string{}, ConditionKeys: []string{}}, ObservedExecutable: &domain.CommandDefinition{Executable: "controlled-native", Arguments: []string{"exact café 東京 �\\r\\n"}, SuccessExitCodes: []int{0}}}}
		}
		launch, err := BuildRole(context.Background(), in, func(context.Context, domain.ExecutionPins) (string, error) {
			return filepath.Join(t.TempDir(), "controlled-native"), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		start := bytes.IndexByte(launch.Prompt, '{')
		if start < 0 {
			t.Fatal("structured role prompt missing")
		}
		var body struct {
			Checks []domain.CheckReceipt `json:"checkReceipts"`
		}
		if err = json.Unmarshal(launch.Prompt[start:], &body); err != nil {
			t.Fatal(err)
		}
		if body.Checks == nil || len(body.Checks) != len(in.Checks) || populated && !reflect.DeepEqual(body.Checks, in.Checks) {
			t.Fatal("complete check receipts were omitted/rewritten or empty array became null", body.Checks, in.Checks)
		}
	}
	in := roleFixture(t, "claude-code")
	in.Checks = make([]domain.CheckReceipt, 2*domain.MaxWorkflowChecks+1)
	resolved := false
	if _, err := BuildRole(context.Background(), in, func(context.Context, domain.ExecutionPins) (string, error) { resolved = true; return "unused", nil }); err == nil || resolved {
		t.Fatal("over-bound current+origin check evidence was truncated or reached executable resolution", err, resolved)
	}
}

func TestClaudeReviewReadsStructuredOutputWithoutChangingAuthorResultContract(t *testing.T) {
	report := domain.ReviewReport{SchemaVersion: domain.ReviewContract, Subject: domain.ReviewSubject{Kind: "response", ID: strings.Repeat("a", 32), SHA256: strings.Repeat("b", 64)}, ConditionsSHA256: strings.Repeat("c", 64), Outcome: "approved", InspectedEvidence: []domain.ReviewInspection{{Ref: "response", Scope: "whole_response", Paths: []string{}}}, Findings: []domain.ReviewFinding{}, Gaps: []domain.ReviewGap{}, ResolvedFindingIDs: []string{}}
	terminal, err := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": false, "session_id": "fresh-review-session", "structured_output": report})
	if err != nil {
		t.Fatal(err)
	}
	stream := append([]byte("{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"fresh-review-session\"}\n"), append(terminal, '\n')...)
	parser, err := NewReviewParser("claude-native", domain.ExecutionPins{HarnessID: "claude-code"}, strings.Repeat("d", 32), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = parser.Write(stream); err != nil {
		t.Fatalf("native structured result was rejected: %v", err)
	}
	if err = parser.Finish(); err != nil || !parser.Result().Succeeded {
		t.Fatal("structured review did not settle", err)
	}
	author := NewClaudeParser(domain.ExecutionPins{HarnessID: "claude-code"}, strings.Repeat("d", 32), nil)
	if _, err = author.Write(stream); err == nil {
		t.Fatal("author contract silently accepted a missing final-response result string")
	}
}
