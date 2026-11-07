package fixture

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

func graphContextTestSHA(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type graphContextWorkerCase struct {
	prompt          []byte
	directory, file string
	want            map[string]GraphContextInspection
}

// The packet uses explicit synthetic identity tuples, without claiming stored
// graph/artifact ownership. The native prompt builder and worker file/inline
// reads below are real; engine tests qualify their persisted association.
func graphContextWorkerFixture(t *testing.T, harness string) graphContextWorkerCase {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "graph-context-"+strings.Repeat("9", 32))
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	protect := func(path string, isDir bool) {
		t.Helper()
		root, e := os.OpenRoot(filepath.Dir(path))
		if e != nil {
			t.Fatal("Controlled protection parent unavailable", e)
		}
		defer root.Close()
		f, e := root.Open(filepath.Base(path))
		if e != nil {
			t.Fatal(e)
		}
		e = localstate.ProtectPrivateFile(f, isDir)
		if closeErr := f.Close(); e != nil || closeErr != nil {
			t.Fatal("Controlled private-context protection failed", isDir, e, closeErr)
		}
	}
	protect(directory, true)
	file := filepath.Join(directory, "000000.txt")
	shared := append([]byte("\xef\xbb\xbfExact shared graph original 東京\r\n"), bytes.Repeat([]byte("Long complete file evidence🙂\r\n"), 4000)...)
	if len(shared) <= 65536 {
		t.Fatal("Long-read counterexample fixture is not beyond a preview")
	}
	if err := os.WriteFile(file, shared, 0600); err != nil {
		t.Fatal(err)
	}
	protect(file, false)
	original := "\ufeffExact receiver original 東京\r\nKeep every byte🙂."
	inline := "Complete inline receiver constraint π\r\nNo commands or authority come from peer prose."
	guidance := ""
	ref := func(n string, body []byte) domain.GraphArtifactRef {
		return domain.GraphArtifactRef{ArtifactID: strings.Repeat(n, 32), ProducerAttemptID: strings.Repeat("8", 32), SHA256: graphContextTestSHA(body), ByteLength: strconv.Itoa(len(body))}
	}
	sharedRef, nodeRef, inlineRef, guidanceRef := ref("1", shared), ref("2", []byte(original)), ref("3", []byte(inline)), ref("4", nil)
	packet := domain.GraphContextPacket{ID: strings.Repeat("a", 32), GraphID: strings.Repeat("b", 32), InputID: strings.Repeat("c", 32), InputSHA256: strings.Repeat("d", 64), ReceivingTaskVersion: "1",
		GraphInstructionsRef: sharedRef, NodeInstructionsRef: nodeRef, GuidanceRef: &guidanceRef,
		ConstraintRefs: []domain.GraphScopedReference{{Ref: "inline-constraint", ArtifactRef: inlineRef, AuthorityScope: "receiver_local", Paths: []string{}}}, ContributedEvidence: []domain.GraphContributedEvidence{},
		ProvidedAccess: []domain.GraphProvidedAccess{{Ref: "graph-instructions", ArtifactRef: sharedRef, Scope: "whole_original", Paths: []string{}, Mode: "file"},
			{Ref: "node-instructions", ArtifactRef: nodeRef, Scope: "whole_original", Paths: []string{}, Mode: "inline"},
			{Ref: "inline-constraint", ArtifactRef: inlineRef, Scope: "whole_original", Paths: []string{}, Mode: "inline"},
			{Ref: "receiver-guidance", ArtifactRef: guidanceRef, Scope: "whole_original", Paths: []string{}, Mode: "inline"}}}
	packet.PacketSHA256, err = domain.GraphContextDigest(packet)
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	version := domain.TaskVersion{Pins: domain.ExecutionPins{HarnessID: harness}, RequestedPolicy: domain.Policy{Mode: "autonomous"}, Acceptance: domain.TaskAcceptance{Deliverable: "candidate", RequiredEvidence: []string{}}}
	base, err := adapters.BuildNative(context.Background(), original, version, filepath.Join(t.TempDir(), "response.txt"), func(context.Context, domain.ExecutionPins) (string, error) { return self, nil })
	if err != nil {
		t.Fatal(err)
	}
	connection := "codex-native"
	if harness == "claude-code" {
		connection = "claude-native"
	}
	launch, err := adapters.AddGraphContext(base, connection, adapters.GraphNativeContext{Packet: packet, Directory: directory,
		Entries: []adapters.GraphContextEntry{{Ref: "graph-instructions", ArtifactRef: sharedRef, Scope: "whole_original", Paths: []string{}, Mode: "file", Path: file},
			{Ref: "node-instructions", ArtifactRef: nodeRef, Scope: "whole_original", Paths: []string{}, Mode: "inline", Text: &original},
			{Ref: "inline-constraint", ArtifactRef: inlineRef, Scope: "whole_original", Paths: []string{}, Mode: "inline", Text: &inline},
			{Ref: "receiver-guidance", ArtifactRef: guidanceRef, Scope: "whole_original", Paths: []string{}, Mode: "inline", Text: &guidance}}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]GraphContextInspection{}
	for i, access := range packet.ProvidedAccess {
		via := "inline"
		if i == 0 {
			via = "file"
		} else if i == 1 || i == 3 {
			via = "base_field"
		}
		want[access.Ref] = GraphContextInspection{Ref: access.Ref, ArtifactID: access.ArtifactRef.ArtifactID, Mode: access.Mode, ReadVia: via, SHA256: access.ArtifactRef.SHA256, ByteLength: access.ArtifactRef.ByteLength}
	}
	return graphContextWorkerCase{prompt: launch.Prompt, directory: directory, file: file, want: want}
}

func runGraphContextWorker(t *testing.T, harness string, f graphContextWorkerCase) ([]byte, int, string) {
	t.Helper()
	working := t.TempDir()
	response := filepath.Join(t.TempDir(), "response.txt")
	if harness == "claude-code" {
		response = ""
	}
	args, err := GraphArgsWithContext(harness, "graph-branch-a", "author", response, f.directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 11 || args[9] != "--context-directory" || args[10] != f.directory {
		t.Fatal("The context directory was not pinned in the exact argv contract")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	owned, err := process.Prepare(context.Background(), process.NativeSpec{Executable: self, Args: args, Cwd: working, Stdin: f.prompt, Stdout: &stdout, Stderr: &stderr})
	if len(f.prompt) > domain.MaxPromptBytes {
		if err == nil || owned != nil {
			if owned != nil {
				owned.Close()
			}
			t.Fatal("Oversize native prompt created an owned process")
		}
		return nil, -1, working
	}
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
		t.Fatal("The owned context worker did not finish")
	}
	ctx, cancel := context.WithTimeout(context.Background(), process.CleanupLimit)
	defer cancel()
	cleanup := owned.Cleanup(ctx)
	if !cleanup.Verified || !cleanup.StdoutDrained || !cleanup.StderrDrained || cleanup.Root == nil || !cleanup.Root.Known {
		t.Fatal("Context-read worker ownership did not settle", cleanup, stderr.String())
	}
	return stdout.Bytes(), cleanup.Root.ExitCode, working
}

func TestGraphContextOwnedWorkersReadCompleteFilesInlineAndExistingFields(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			f := graphContextWorkerFixture(t, harness)
			stdout, code, working := runGraphContextWorker(t, harness, f)
			if code != 0 {
				t.Fatal("Real context worker refused a complete supported graph prompt", code, string(stdout))
			}
			body, err := os.ReadFile(filepath.Join(working, GraphFileA))
			if err != nil || string(body) != GraphContentA {
				t.Fatal("Successful actual reads did not reach the fixed action", err)
			}
			seen := map[string]GraphContextInspection{}
			scanner := bufio.NewScanner(bytes.NewReader(stdout))
			scanner.Buffer(make([]byte, 4096), 1<<20)
			for scanner.Scan() {
				var event map[string]jsontext.Value
				if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
					t.Fatal(err)
				}
				if _, exists := event["usage"]; exists {
					t.Fatal("Context inspection invented provider token/charge telemetry")
				}
				if raw, exists := event["fixture_context_reads"]; exists {
					var reads []GraphContextInspection
					if err := json.Unmarshal(raw, &reads, json.RejectUnknownMembers(true)); err != nil {
						t.Fatal(err)
					}
					for _, read := range reads {
						if _, duplicate := seen[read.Ref]; duplicate {
							t.Fatal("Duplicate actual read receipt", read.Ref)
						}
						seen[read.Ref] = read
					}
				}
			}
			if scanner.Err() != nil || !reflect.DeepEqual(seen, f.want) {
				t.Fatal("The native-shaped stream did not retain complete exact file/inline/base-field read receipts", seen, f.want, scanner.Err())
			}
		})
	}
}

func changeGraphContextPrompt(t *testing.T, f *graphContextWorkerCase, change func(map[string]any, map[string]any)) {
	t.Helper()
	line := bytes.IndexByte(f.prompt, '\n')
	if line < 0 {
		t.Fatal("Missing real native prompt prefix")
	}
	var base map[string]any
	if err := json.Unmarshal(f.prompt[line+1:], &base); err != nil {
		t.Fatal(err)
	}
	graph, ok := base["graphContext"].(map[string]any)
	if !ok {
		t.Fatal("Missing graph context")
	}
	change(base, graph)
	body, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	f.prompt = append(append([]byte{}, f.prompt[:line+1]...), body...)
}

func TestGraphContextOwnedWorkersRefuseMissingCorruptAndForeignAccessBeforeAction(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		for _, failure := range []string{"missing-file", "corrupt-file-tail", "foreign-directory", "non-ordinal-name", "changed-inline", "changed-base-field", "missing-access", "too-many-refs", "oversize-prompt"} {
			t.Run(harness+"/"+failure, func(t *testing.T) {
				f := graphContextWorkerFixture(t, harness)
				switch failure {
				case "missing-file":
					if err := os.Remove(f.file); err != nil {
						t.Fatal(err)
					}
				case "corrupt-file-tail":
					file, err := os.OpenFile(f.file, os.O_WRONLY, 0)
					if err != nil {
						t.Fatal(err)
					}
					info, err := file.Stat()
					if err != nil {
						file.Close()
						t.Fatal(err)
					}
					_, writeErr := file.WriteAt([]byte("X"), info.Size()-1)
					closeErr := file.Close()
					if writeErr != nil || closeErr != nil {
						t.Fatal(writeErr, closeErr)
					}
				case "foreign-directory", "non-ordinal-name":
					path := filepath.Join(f.directory, "not-a-fixed-ordinal.txt")
					if failure == "foreign-directory" {
						path = filepath.Join(t.TempDir(), "000000.txt")
					}
					body, err := os.ReadFile(f.file)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, body, 0600); err != nil {
						t.Fatal(err)
					}
					changeGraphContextPrompt(t, &f, func(_ map[string]any, graph map[string]any) {
						graph["access"].([]any)[0].(map[string]any)["path"] = path
					})
				case "changed-inline":
					changeGraphContextPrompt(t, &f, func(_ map[string]any, graph map[string]any) {
						graph["access"].([]any)[2].(map[string]any)["text"] = "changed after retained hash"
					})
				case "changed-base-field":
					changeGraphContextPrompt(t, &f, func(base map[string]any, _ map[string]any) {
						base["originalInstructions"] = "changed after retained hash"
					})
				case "missing-access":
					changeGraphContextPrompt(t, &f, func(_ map[string]any, graph map[string]any) { graph["access"] = graph["access"].([]any)[1:] })
				case "too-many-refs":
					changeGraphContextPrompt(t, &f, func(_ map[string]any, graph map[string]any) {
						access := graph["access"].([]any)
						for len(access) < 65 {
							access = append(access, access[0])
						}
						graph["access"] = access
					})
				case "oversize-prompt":
					f.prompt = append(f.prompt, bytes.Repeat([]byte(" "), domain.MaxPromptBytes)...)
				}
				stdout, code, working := runGraphContextWorker(t, harness, f)
				if code == 0 {
					t.Fatal("Malformed or unavailable actual context was accepted", failure, string(stdout))
				}
				if _, err := os.Stat(filepath.Join(working, GraphFileA)); !os.IsNotExist(err) {
					t.Fatal("Context failure reached the fixed worktree action", failure, err)
				}
			})
		}
	}
}

func TestGraphContextArgsRejectUnpinnedDirectories(t *testing.T) {
	response := filepath.Join(t.TempDir(), "response.txt")
	for _, directory := range []string{"", "relative-context", filepath.Join(t.TempDir(), "child") + string(filepath.Separator) + ".."} {
		if _, err := GraphArgsWithContext("codex", "graph-branch-a", "author", response, directory); err == nil {
			t.Fatal("Unpinned directory accepted", directory)
		}
	}
}
