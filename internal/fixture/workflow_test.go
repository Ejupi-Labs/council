package fixture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

func TestWorkflowReviewOwnedFixturesObserveActualCandidateAndResponse(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		for _, subject := range []string{"document", "response", "actual_defect"} {
			t.Run(harness+"/"+subject, func(t *testing.T) {
				working, private := t.TempDir(), t.TempDir()
				provided := filepath.Join(private, "provided-evidence")
				data := []byte("controlled retained candidate manifest\n")
				kind, scope, ref := "candidate", "whole_candidate", "selected-candidate-manifest"
				original := "Produce the controlled VALID fixture."
				if subject == "response" {
					kind, scope, ref = "response", "whole_response", "selected-response"
					data = []byte(ClaudeResponseText)
				} else if subject == "document" {
					original = `Replace "old" with "new" in docs/note.md only`
					if err := os.Mkdir(filepath.Join(working, "docs"), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(working, "docs", "note.md"), []byte("\ufeffCONTROLLED LOCAL WORKFLOW TEST DATA\r\nnew\r\n東京 �\r\n"), 0600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(filepath.Join(working, WorkflowFile), []byte(WorkflowInvalid), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(provided, data, 0600); err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(data)
				prompt := workflowPrompt{Original: original, Role: "review", Subject: domain.ReviewSubject{Kind: kind, ID: strings.Repeat("a", 32), SHA256: hex.EncodeToString(sum[:])}, ConditionsSHA256: strings.Repeat("b", 64), Evidence: []workflowEvidence{{Ref: ref, Path: provided, Scope: scope, Paths: []string{}}}, Findings: []domain.ReviewFinding{}, PriorFindingIDs: []string{}}
				if subject == "response" {
					rawCheck := filepath.Join(private, "actual-check-stdout")
					if err := os.WriteFile(rawCheck, []byte("controlled check output differs from final response\n"), 0600); err != nil {
						t.Fatal(err)
					}
					prompt.Evidence = append(prompt.Evidence, workflowEvidence{Ref: "check-controlled-stdout", Path: rawCheck, Scope: "whole_response", Paths: []string{}})
				}
				input, err := json.Marshal(prompt)
				if err != nil {
					t.Fatal(err)
				}
				response := ""
				if harness == "codex" {
					response = filepath.Join(private, "response.json")
				}
				// Even the clean scenario must report the actual INVALID candidate.
				args, err := WorkflowArgs(harness, "workflow-review-clean", "review", response)
				if err != nil {
					t.Fatal(err)
				}
				self, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				var stdout, stderr bytes.Buffer
				worker, err := process.Prepare(context.Background(), process.NativeSpec{Executable: self, Args: args, Cwd: working, Stdin: input, Stdout: &stdout, Stderr: &stderr})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { worker.Close() })
				if err = worker.Start(); err != nil {
					t.Fatal(err)
				}
				select {
				case <-worker.RootDone():
				case <-time.After(10 * time.Second):
					t.Fatal("controlled reviewer did not exit")
				}
				ctx, cancel := context.WithTimeout(context.Background(), process.CleanupLimit)
				defer cancel()
				clean := worker.Cleanup(ctx)
				if !clean.Verified || clean.Root == nil || !clean.Root.Known || clean.Root.ExitCode != 0 || clean.StdinState != process.StdinComplete || !clean.StdoutDrained || !clean.StderrDrained {
					t.Fatalf("controlled review lost native proof: %+v; stderr=%s", clean, stderr.Bytes())
				}
				connection, pins := "codex-native", domain.ExecutionPins{HarnessID: "codex"}
				if harness == "claude-code" {
					connection, pins = "claude-native", domain.ExecutionPins{HarnessID: "claude-code"}
				}
				parser, err := adapters.NewReviewParser(connection, pins, strings.Repeat("c", 32), nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = parser.Write(stdout.Bytes()); err != nil {
					t.Fatal(err)
				}
				if err = parser.Finish(); err != nil || !parser.Result().Succeeded {
					t.Fatal("controlled native review protocol failed", err)
				}
				raw := parser.Result().StructuredOutput
				if harness == "codex" {
					raw, err = os.ReadFile(response)
					if err != nil {
						t.Fatal(err)
					}
				}
				var report domain.ReviewReport
				if invalid := domain.StrictDecode(raw, &report); invalid != nil {
					t.Fatal(invalid)
				}
				want := "approved"
				if subject == "actual_defect" {
					want = "changes_required"
				}
				if report.Outcome != want || report.Subject != prompt.Subject || len(report.InspectedEvidence) != len(prompt.Evidence) || report.InspectedEvidence[0].Ref != ref || subject == "actual_defect" && (len(report.Findings) != 1 || report.Findings[0].Locations[0].Path != WorkflowFile) {
					t.Fatalf("review did not reflect actual supplied bytes: %+v", report)
				}
			})
		}
	}
}

func TestWorkflowResponseReviewRequiresSelectedReferenceWithSupplementalOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "response-like-check-output")
	data := []byte(ClaudeResponseText)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	prompt := workflowPrompt{Subject: domain.ReviewSubject{Kind: "response", ID: strings.Repeat("a", 32), SHA256: hex.EncodeToString(sum[:])}, Evidence: []workflowEvidence{{Ref: "check-controlled-stdout", Path: path, Scope: "whole_response", Paths: []string{}}}}
	if _, err := workflowReview(prompt); err == nil {
		t.Fatal("response-like supplemental output substituted for the required selected-response reference")
	}
}

func TestWorkflowFakeSelectionCannotAddPublicRoleOrExecutable(t *testing.T) {
	for _, value := range []struct{ harness, scenario, role, output string }{
		{"claude.exe", "workflow-document", "author", ""},
		{"codex", "workflow-review-clean", "author", ""},
		{"claude-code", "workflow-defect", "review", ""},
		{"claude-code", "workflow-correction", "correction", "invented-output"},
	} {
		if _, err := WorkflowArgs(value.harness, value.scenario, value.role, value.output); err == nil {
			t.Fatalf("fixed workflow fixture accepted arbitrary selector: %+v", value)
		}
	}
}
