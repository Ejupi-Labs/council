package fixture

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/process"
)

func TestMain(m *testing.M) {
	if handled, code := process.HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	if handled, code := HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestFixedScenariosRetainVersionedOutputAndRealResponse(t *testing.T) {
	for _, scenario := range []string{"response", "candidate", "partial_failure", "late_success", "cleanup_unknown"} {
		t.Run(scenario, func(t *testing.T) {
			working := t.TempDir()
			response := filepath.Join(t.TempDir(), "response.txt")
			args, err := Args(scenario, response)
			if err != nil {
				t.Fatal(err)
			}
			self, _ := os.Executable()
			input := []byte("\ufeffExact fixture prompt\r\n東京\n")
			var out, diagnostics bytes.Buffer
			o, err := process.Prepare(context.Background(), process.NativeSpec{Executable: self, Args: args, Cwd: working, Stdin: input, Stdout: &out, Stderr: &diagnostics})
			if err != nil {
				t.Fatal(err)
			}
			defer o.Close()
			if o.Start() != nil {
				t.Fatal("fixture start failed")
			}
			select {
			case <-o.RootDone():
			case <-time.After(7 * time.Second):
				t.Fatal("fixture did not exit")
			}
			ctx, cancel := context.WithTimeout(context.Background(), process.CleanupLimit)
			defer cancel()
			cleanup := o.Cleanup(ctx)
			if !cleanup.Verified || !cleanup.StdoutDrained || !cleanup.StderrDrained {
				t.Fatalf("fixture cleanup failed: %+v", cleanup)
			}
			root, _ := o.RootOutcome()
			wantCode := 0
			wantText := ResponseText
			if scenario == "partial_failure" {
				wantCode = 17
				wantText = PartialText
			}
			if !root.Known || root.ExitCode != wantCode {
				t.Fatalf("wrong fixture root: %+v", root)
			}
			text, err := os.ReadFile(response)
			if err != nil || string(text) != wantText {
				t.Fatalf("response artifact changed: %v", err)
			}
			sum := sha256.Sum256(input)
			seenInput := false
			seenFinal := false
			seenUnknown := false
			scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
			scanner.Buffer(make([]byte, 4096), 1024*1024)
			for scanner.Scan() {
				var event map[string]any
				if json.Unmarshal(scanner.Bytes(), &event) != nil {
					t.Fatal("fixture emitted invalid JSONL")
				}
				if event["council_fixture"] != true || event["fixture_contract"] != Contract {
					t.Fatal("fixture disguised its identity")
				}
				switch event["type"] {
				case "turn.started":
					seenInput = event["fixture_input_sha256"] == hex.EncodeToString(sum[:])
				case "turn.completed":
					seenFinal = scenario != "partial_failure"
				case "turn.failed":
					seenFinal = scenario == "partial_failure"
				case "council.fixture.cleanup_unknown":
					seenUnknown = true
				}
			}
			if scanner.Err() != nil || !seenInput || !seenFinal {
				t.Fatal("fixture lost exact input proof or terminal contract")
			}
			if (scenario == "cleanup_unknown") != seenUnknown {
				t.Fatal("cleanup fault injection marker changed")
			}
			if !strings.Contains(diagnostics.String(), "stderr tail") {
				t.Fatal("stderr tail was lost")
			}
			candidate, err := os.ReadFile(filepath.Join(working, "council-fixture.txt"))
			if scenario == "candidate" || scenario == "partial_failure" {
				if err != nil || string(candidate) != CandidateText {
					t.Fatal("fixture candidate missing")
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("response fixture changed managed source")
			}
		})
	}
}

func TestFakeSelectionNeverAcceptsAnExecutableOrUnknownScenario(t *testing.T) {
	for _, scenario := range []string{"native", "codex.exe", "../../agent", ""} {
		args, err := Args(scenario, filepath.Join(t.TempDir(), "response"))
		if scenario == "" {
			if err != nil || args[2] != "response" {
				t.Fatal("explicit fake default changed")
			}
		} else if err == nil {
			t.Fatalf("unknown fake scenario accepted: %q", scenario)
		}
	}
	if _, err := Args("response", "relative"); err == nil {
		t.Fatal("relative artifact destination accepted")
	}
}

func TestSlowFixtureUsesTheSameOwnedCleanup(t *testing.T) {
	dir := t.TempDir()
	args, _ := Args("slow", filepath.Join(dir, "response"))
	self, _ := os.Executable()
	o, err := process.Prepare(context.Background(), process.NativeSpec{Executable: self, Args: args, Cwd: dir, Stdin: []byte("fake"), Stdout: io.Discard, Stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), process.CleanupLimit)
	defer cancel()
	if !o.Cleanup(ctx).Verified {
		t.Fatal("slow fixture did not settle its owned scope")
	}
	if _, err := os.Stat(filepath.Join(dir, "response")); !os.IsNotExist(err) {
		t.Fatal("cancelled slow fixture fabricated a response")
	}
}
