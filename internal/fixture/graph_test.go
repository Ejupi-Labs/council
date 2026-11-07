package fixture

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/process"
)

func TestGraphFixedOwnedWorkersUseNativeProtocolsAndActualFiles(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		for _, scenario := range []string{"graph-branch-a", "graph-branch-b", "graph-join", "graph-join-no-edit", "graph-fail", "graph-wait", "graph-response", "graph-diverge-a", "graph-diverge-b"} {
			t.Run(harness+"/"+scenario, func(t *testing.T) {
				working, private := t.TempDir(), t.TempDir()
				if strings.HasPrefix(scenario, "graph-join") {
					for name, content := range map[string]string{GraphFileA: GraphContentA, GraphFileB: GraphContentB} {
						if err := os.WriteFile(filepath.Join(working, name), []byte(content), 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
				response := filepath.Join(private, "response.txt")
				if harness == "claude-code" {
					response = ""
				}
				args, err := GraphArgs(harness, scenario, "author", response)
				if err != nil {
					t.Fatal(err)
				}
				executable, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				var stdout, stderr bytes.Buffer
				owned, err := process.Prepare(context.Background(), process.NativeSpec{Executable: executable, Args: args, Cwd: working, Stdin: []byte("\ufeffControlled graph original 東京\r\n"), Stdout: &stdout, Stderr: &stderr})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = owned.Close() })
				if err := owned.Start(); err != nil {
					t.Fatal(err)
				}
				if scenario == "graph-wait" {
					until := time.Now().Add(5 * time.Second)
					for {
						if body, err := os.ReadFile(filepath.Join(working, GraphReadyFile)); err == nil && string(body) == "ready\n" {
							break
						}
						if time.Now().After(until) {
							t.Fatal("Actual controlled worker did not reach its bounded barrier")
						}
						time.Sleep(5 * time.Millisecond)
					}
					if err := os.WriteFile(filepath.Join(working, GraphReleaseFile), []byte("release\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				select {
				case <-owned.RootDone():
				case <-time.After(10 * time.Second):
					t.Fatal("Controlled graph worker did not finish")
				}
				ctx, cancel := context.WithTimeout(context.Background(), process.CleanupLimit)
				defer cancel()
				cleanup := owned.Cleanup(ctx)
				wantCode, wantText := 0, GraphResponse
				if scenario == "graph-fail" {
					wantCode, wantText = 17, GraphPartialResponse
				}
				if !cleanup.Verified || !cleanup.StdoutDrained || !cleanup.StderrDrained || cleanup.Root == nil || !cleanup.Root.Known || cleanup.Root.ExitCode != wantCode {
					t.Fatal("Controlled graph worker lost actual terminal ownership", cleanup, stderr.String())
				}
				scanner := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
				var events []map[string]jsontext.Value
				for scanner.Scan() {
					var event map[string]jsontext.Value
					if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
						t.Fatal("Invalid native-shaped graph event", err)
					}
					events = append(events, event)
				}
				if scanner.Err() != nil || len(events) < 2 {
					t.Fatal("Graph fixture did not retain its native event stream", scanner.Err())
				}
				if harness == "codex" {
					body, err := os.ReadFile(response)
					if err != nil || string(body) != wantText || string(events[0]["type"]) != `"thread.started"` {
						t.Fatal("Codex graph output contract changed", err, string(body))
					}
				} else {
					files, err := os.ReadDir(private)
					if err != nil || len(files) != 0 || string(events[0]["type"]) != `"system"` || string(events[len(events)-1]["result"]) == "" {
						t.Fatal("Claude graph invented a response file or lost streamed result", err, files)
					}
				}
				if scenario == "graph-diverge-a" || scenario == "graph-diverge-b" {
					want := GraphContentA
					if scenario == "graph-diverge-b" {
						want = GraphContentB
					}
					body, err := os.ReadFile(filepath.Join(working, "council-graph-conflict.txt"))
					if err != nil || string(body) != want {
						t.Fatal("The fixed divergence scenario did not write its exact distinct file bytes", err)
					}
				} else if scenario == "graph-join" {
					body, err := os.ReadFile(filepath.Join(working, GraphJoinedFile))
					if err != nil || string(body) != GraphContentA+GraphContentB {
						t.Fatal("Join did not read the exact predecessor bytes", err, string(body))
					}
				} else if scenario == "graph-response" {
					entries, readErr := os.ReadDir(working)
					if readErr != nil || len(entries) != 0 {
						t.Fatal("Response-only graph worker changed its supplied workspace", entries, readErr)
					}
				} else if scenario == "graph-join-no-edit" {
					if _, err := os.Stat(filepath.Join(working, GraphJoinedFile)); !os.IsNotExist(err) {
						t.Fatal("No-own-edit verification fabricated a file change", err)
					}
				}
			})
		}
	}
}

func TestGraphFixedArgsRejectScriptsRolesAndUnsupportedOutputs(t *testing.T) {
	absolute := filepath.Join(t.TempDir(), "response")
	for _, c := range [][4]string{{"codex", "echo arbitrary", "author", absolute}, {"codex", "graph-branch-a", "review", absolute}, {"junie", "graph-branch-a", "author", absolute}, {"claude-code", "graph-branch-a", "author", absolute}} {
		if _, err := GraphArgs(c[0], c[1], c[2], c[3]); err == nil {
			t.Fatal("Unsupported fixture contract was accepted", c)
		}
	}
}

func TestGraphWaitingWorkerCancelsWithOwnedCleanupBeforeRelease(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			working, private := t.TempDir(), t.TempDir()
			response := filepath.Join(private, "response.txt")
			if harness == "claude-code" {
				response = ""
			}
			args, err := GraphArgs(harness, "graph-wait", "author", response)
			if err != nil {
				t.Fatal(err)
			}
			executable, _ := os.Executable()
			var stdout, stderr bytes.Buffer
			owned, err := process.Prepare(context.Background(), process.NativeSpec{Executable: executable, Args: args, Cwd: working, Stdin: []byte("controlled cancellable graph worker"), Stdout: &stdout, Stderr: &stderr})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owned.Close() })
			if err = owned.Start(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				if body, err := os.ReadFile(filepath.Join(working, GraphReadyFile)); err == nil && string(body) == "ready\n" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("Controlled worker never reached its actual cancellation barrier")
				}
				time.Sleep(5 * time.Millisecond)
			}
			ctx, cancel := context.WithTimeout(context.Background(), process.CleanupLimit)
			defer cancel()
			cleanup := owned.Cleanup(ctx)
			if !cleanup.Verified || !cleanup.StdoutDrained || !cleanup.StderrDrained || cleanup.Root == nil || !cleanup.Root.Known {
				t.Fatal("Controlled waiting worker retained unresolved ownership", cleanup)
			}
			if _, err := os.Stat(filepath.Join(working, GraphFileA)); !os.IsNotExist(err) {
				t.Fatal("Cancelled worker performed the post-release action", err)
			}
		})
	}
}
