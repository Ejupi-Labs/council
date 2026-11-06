package fixture

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

func TestClaudeOwnedNativeShapedFixtureNeverCreatesResponseFile(t *testing.T) {
	for _, scenario := range []string{"response", "empty_candidate", "partial_failure", "trailing_observation", "repeated_result", "trailing_error", "success_error", "missing_result", "malformed", "model_usage"} {
		t.Run(scenario, func(t *testing.T) {
			working, private := t.TempDir(), t.TempDir()
			args, err := ArgsForHarness("claude-code", scenario, private)
			if err != nil {
				t.Fatal(err)
			}
			self, _ := os.Executable()
			input := []byte("\ufeffExact Claude prompt\r\n東京 �\n")
			var stdout, stderr bytes.Buffer
			worker, err := process.Prepare(context.Background(), process.NativeSpec{Executable: self, Args: args, Cwd: working, Stdin: input, Stdout: &stdout, Stderr: &stderr})
			if err != nil {
				t.Fatal(err)
			}
			defer worker.Close()
			if err := worker.Start(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-worker.RootDone():
			case <-time.After(7 * time.Second):
				t.Fatal("Claude fixture did not finish")
			}
			ctx, cancel := context.WithTimeout(context.Background(), process.CleanupLimit)
			defer cancel()
			cleanup := worker.Cleanup(ctx)
			if !cleanup.Verified || !cleanup.StdoutDrained || !cleanup.StderrDrained || cleanup.StdinState != process.StdinComplete {
				t.Fatalf("Claude fixture lost native scope/streams/input proof: %+v", cleanup)
			}
			files, err := os.ReadDir(private)
			if err != nil || len(files) != 0 {
				t.Fatal("Claude fixture fabricated a response file")
			}
			var first map[string]any
			scanner := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
			if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &first) != nil {
				t.Fatal("Claude fixture lost its init record")
			}
			sum := sha256.Sum256(input)
			if first["council_fixture"] != true || first["fixture_contract"] != ClaudeContract || first["fixture_input_sha256"] != hex.EncodeToString(sum[:]) {
				t.Fatal("fixture identity or exact prompt digest changed")
			}
			parser := adapters.NewClaudeParser(domain.ExecutionPins{HarnessID: "claude-code"}, "0123456789abcdef0123456789abcdef", nil)
			_, parseErr := parser.Write(stdout.Bytes())
			if parseErr == nil {
				parseErr = parser.Finish()
			}
			result := parser.Result()
			wantSuccess := scenario == "response" || scenario == "empty_candidate" || scenario == "trailing_observation" || scenario == "model_usage"
			if result.Succeeded != wantSuccess || wantSuccess && parseErr != nil {
				t.Fatalf("wrong fixed fixture protocol outcome: %+v err=%v", result, parseErr)
			}
			if scenario == "response" && (result.FinalResponse == nil || *result.FinalResponse != ClaudeResponseText) {
				t.Fatal("Claude final-event bytes changed")
			}
			if scenario == "empty_candidate" {
				candidate, err := os.ReadFile(filepath.Join(working, "council-fixture.txt"))
				if err != nil || string(candidate) != CandidateText || result.FinalResponse == nil || *result.FinalResponse != "" {
					t.Fatal("empty terminal text lost useful candidate evidence")
				}
			}
			if result.Observations.NativeModels != nil || result.Observations.Model.Status != "unknown" || result.Observations.NativeVersion != nil {
				t.Fatal("local fake facts became measured native observations")
			}
		})
	}
}

func TestClaudeFakeArgumentsCannotSelectAVendorOrShell(t *testing.T) {
	private := t.TempDir()
	for _, harness := range []string{"claude.exe", "native", "deepseek-harness", "../claude"} {
		if _, err := ArgsForHarness(harness, "response", private); err == nil {
			t.Fatal("fake harness selector accepted an executable or unsupported target")
		}
	}
	if _, err := ArgsForHarness("claude-code", "response", "relative"); err == nil {
		t.Fatal("relative private auxiliary directory accepted")
	}
	if _, err := ArgsForHarness("codex", "repeated_result", filepath.Join(private, "response")); err == nil {
		t.Fatal("Claude-only scenario silently changed the Codex fixture")
	}
}
