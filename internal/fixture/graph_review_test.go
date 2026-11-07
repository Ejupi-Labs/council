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

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func TestGraphReviewOwnedWorkersInspectExactCandidateContextAndRejectChangedFiles(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		for _, selected := range []string{".", "nested src 東京"} {
			for _, result := range []struct{ name, path, content string }{{"branch-a", GraphFileA, GraphContentA}, {"diverge-a", GraphConflictFile, GraphContentA}, {"diverge-b", GraphConflictFile, GraphContentB}} {
				for _, changed := range []bool{false, true} {
					name := harness + "/complete"
					if changed {
						name = harness + "/changed-after-manifest"
					}
					if result.name != "branch-a" {
						name += "/" + result.name
					}
					if selected != "." {
						name += "/selected-subfolder"
					}
					t.Run(name, func(t *testing.T) {
						f := graphContextWorkerFixture(t, harness)
						workspaceRoot := t.TempDir()
						working := filepath.Join(workspaceRoot, selected)
						if err := os.MkdirAll(working, 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(working, result.path), []byte(result.content), 0600); err != nil {
							t.Fatal(err)
						}
						manifest := workspace.Manifest{Entries: []workspace.Entry{{Path: filepath.ToSlash(filepath.Join(selected, result.path)), Type: "file", Mode: 0600, Size: int64(len(result.content)), SHA256: contextRefSHA([]byte(result.content)), ArtifactID: strings.Repeat("1", 32)}}, Index: []workspace.IndexEntry{}, Exclusions: []workspace.Exclusion{}}
						body, err := json.Marshal(manifest)
						if err != nil {
							t.Fatal(err)
						}
						manifestPath := filepath.Join(t.TempDir(), "manifest.json")
						if err = os.WriteFile(manifestPath, body, 0600); err != nil {
							t.Fatal(err)
						}
						line := bytes.IndexByte(f.prompt, '\n')
						var base map[string]jsontext.Value
						if line < 0 || json.Unmarshal(f.prompt[line+1:], &base) != nil {
							t.Fatal("The actual graph context prompt fixture is unavailable")
						}
						put := func(key string, value any) {
							raw, err := json.Marshal(value)
							if err != nil {
								t.Fatal(err)
							}
							base[key] = raw
						}
						subject := domain.ReviewSubject{Kind: "candidate", ID: strings.Repeat("2", 32), SHA256: contextRefSHA(body)}
						put("role", "review")
						put("subject", subject)
						put("conditionsSHA256", strings.Repeat("3", 64))
						put("evidence", []workflowEvidence{{Ref: "selected-candidate-manifest", Path: manifestPath, Scope: "whole_candidate", Paths: []string{}}})
						raw, err := json.Marshal(base)
						if err != nil {
							t.Fatal(err)
						}
						prompt := append(append([]byte{}, f.prompt[:line+1]...), raw...)
						if changed {
							if err = os.WriteFile(filepath.Join(working, result.path), []byte("Changed after the supplied manifest\n"), 0600); err != nil {
								t.Fatal(err)
							}
						}
						before, err := os.ReadFile(filepath.Join(working, result.path))
						if err != nil {
							t.Fatal(err)
						}
						response := filepath.Join(t.TempDir(), "response.json")
						if harness == "claude-code" {
							response = ""
						}
						args, err := GraphReviewArgsWithContext(harness, response, f.directory, workspaceRoot)
						if err != nil {
							t.Fatal(err)
						}
						self, err := os.Executable()
						if err != nil {
							t.Fatal(err)
						}
						var stdout, stderr bytes.Buffer
						owned, err := process.Prepare(context.Background(), process.NativeSpec{Executable: self, Args: args, Cwd: working, Stdin: prompt, Stdout: &stdout, Stderr: &stderr})
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = owned.Close() })
						if err = owned.Start(); err != nil {
							t.Fatal(err)
						}
						select {
						case <-owned.RootDone():
						case <-time.After(10 * time.Second):
							t.Fatal("Controlled graph reviewer did not finish")
						}
						ctx, cancel := context.WithTimeout(context.Background(), process.CleanupLimit)
						defer cancel()
						cleanup := owned.Cleanup(ctx)
						if !cleanup.Verified || !cleanup.StdoutDrained || !cleanup.StderrDrained || cleanup.Root == nil || !cleanup.Root.Known {
							t.Fatal("The controlled graph reviewer lost process ownership", cleanup, stderr.String())
						}
						after, err := os.ReadFile(filepath.Join(working, result.path))
						if err != nil || !bytes.Equal(before, after) {
							t.Fatal("A supporting reviewer changed its supplied candidate", err)
						}
						if changed {
							if cleanup.Root.ExitCode == 0 {
								t.Fatal("The reviewer approved a different physical candidate")
							}
							return
						}
						if cleanup.Root.ExitCode != 0 {
							t.Fatal("Complete actual graph evidence was rejected", cleanup, stderr.String())
						}
						var reportBody []byte
						inspectedContext := false
						scanner := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
						scanner.Buffer(make([]byte, 4096), 1<<20)
						for scanner.Scan() {
							var event map[string]jsontext.Value
							if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
								t.Fatal(err)
							}
							if _, ok := event["usage"]; ok {
								t.Fatal("The controlled reviewer invented provider telemetry")
							}
							if raw := event["fixture_context_reads"]; raw != nil {
								var reads []GraphContextInspection
								if err := json.Unmarshal(raw, &reads); err != nil {
									t.Fatal(err)
								}
								inspectedContext = len(reads) == len(f.want)
								for _, read := range reads {
									if want, ok := f.want[read.Ref]; !ok || read != want {
										t.Fatal("Graph review did not actually read the exact context bytes", read)
									}
								}
							}
							if raw := event["structured_output"]; raw != nil {
								reportBody = raw
							}
						}
						if scanner.Err() != nil || !inspectedContext {
							t.Fatal("The reviewer omitted its actual complete context read receipt", scanner.Err())
						}
						if harness == "codex" {
							reportBody, err = os.ReadFile(response)
							if err != nil {
								t.Fatal(err)
							}
						}
						var report domain.ReviewReport
						if err := json.Unmarshal(reportBody, &report, json.RejectUnknownMembers(true)); err != nil || report.Subject != subject || report.Outcome != "approved" || report.ConditionsSHA256 != strings.Repeat("3", 64) || len(report.InspectedEvidence) != 1 {
							t.Fatal("The reviewer returned no exact structured candidate verdict", report, err)
						}
					})
				}
			}
		}
	}
}
