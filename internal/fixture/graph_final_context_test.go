package fixture

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

// These identities are explicit synthetic transport tuples. The private files,
// complete byte reads and owned worker are real; persisted final authority is
// separately established by the Engine's actual graph pipeline.
func graphFinalContextWorkerFixture(t *testing.T, harness string) (graphContextWorkerCase, string) {
	t.Helper()
	f := graphContextWorkerFixture(t, harness)
	line := bytes.IndexByte(f.prompt, '\n')
	var base map[string]jsontext.Value
	if line < 0 || json.Unmarshal(f.prompt[line+1:], &base) != nil {
		t.Fatal("The ordinary native-shaped context prerequisite is unavailable")
	}
	var original graphFixturePrompt
	if err := json.Unmarshal(base["graphContext"], &original); err != nil {
		t.Fatal(err)
	}
	access := []graphFixtureAccess{}
	var finalRef domain.GraphArtifactRef
	var memberPath string
	for i, source := range []struct {
		name string
		body []byte
	}{
		{"final-verification", []byte("Exact controlled cumulative final header 東京\r\n")},
		{"final-before-0001", bytes.Repeat([]byte("Complete deleted BEFORE source café🙂\r\n"), 2500)},
	} {
		path := filepath.Join(f.directory, "00000"+strconv.Itoa(i+4)+".txt")
		if err := os.WriteFile(path, source.body, 0600); err != nil {
			t.Fatal(err)
		}
		parent, err := os.OpenRoot(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		file, err := parent.Open(filepath.Base(path))
		if err != nil {
			parent.Close()
			t.Fatal(err)
		}
		protectErr, closeErr, rootErr := localstate.ProtectPrivateFile(file, false), file.Close(), parent.Close()
		if protectErr != nil || closeErr != nil || rootErr != nil {
			t.Fatal("Could not protect the controlled final source", protectErr, closeErr, rootErr)
		}
		ref := domain.GraphArtifactRef{ArtifactID: strings.Repeat(strconv.Itoa(i+5), 32), ProducerAttemptID: strings.Repeat("8", 32), SHA256: graphContextTestSHA(source.body), ByteLength: strconv.Itoa(len(source.body))}
		paths := []string{}
		if i == 0 {
			finalRef = ref
		} else {
			paths, memberPath = []string{"deleted.txt"}, path
		}
		access = append(access, graphFixtureAccess{Ref: source.name, ArtifactRef: ref, Scope: "cumulative_final", Paths: paths, Mode: "file", Path: &path})
		f.want[source.name] = GraphContextInspection{Ref: source.name, ArtifactID: ref.ArtifactID, Mode: "file", ReadVia: "file", SHA256: ref.SHA256, ByteLength: ref.ByteLength}
	}
	payload := struct {
		Contract             string                  `json:"contract"`
		ReceivingAttemptID   string                  `json:"receivingAttemptId"`
		InputID              string                  `json:"inputId"`
		InputSHA256          string                  `json:"inputSha256"`
		PacketSHA256         string                  `json:"packetSha256"`
		FinalVerificationRef domain.GraphArtifactRef `json:"finalVerificationRef"`
		Access               []graphFixtureAccess    `json:"access"`
	}{"006-final-context-v1", strings.Repeat("9", 32), original.Packet.InputID, original.Packet.InputSHA256, original.Packet.PacketSHA256, finalRef, access}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	base["graphFinalVerification"] = body
	body, err = json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	f.prompt = append(append([]byte{}, f.prompt[:line+1]...), body...)
	return f, memberPath
}

func TestGraphFinalContextOwnedWorkersReadCompleteSupplementalFiles(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			f, _ := graphFinalContextWorkerFixture(t, harness)
			stdout, code, working := runGraphContextWorker(t, harness, f)
			if code != 0 {
				t.Fatal("A complete controlled cumulative final context was refused", code)
			}
			if body, err := os.ReadFile(filepath.Join(working, GraphFileA)); err != nil || string(body) != GraphContentA {
				t.Fatal("Complete final context reads did not precede the fixed local action", err)
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
					t.Fatal("The controlled final reader invented provider usage")
				}
				if raw := event["fixture_context_reads"]; raw != nil {
					var reads []GraphContextInspection
					if err := json.Unmarshal(raw, &reads, json.RejectUnknownMembers(true)); err != nil {
						t.Fatal(err)
					}
					for _, read := range reads {
						if _, duplicate := seen[read.Ref]; duplicate {
							t.Fatal("Duplicate final read receipt", read.Ref)
						}
						seen[read.Ref] = read
					}
				}
			}
			if scanner.Err() != nil || !reflect.DeepEqual(seen, f.want) {
				t.Fatal("The owned worker omitted complete exact cumulative final source reads", seen, f.want, scanner.Err())
			}
		})
	}
}

func TestGraphFinalContextOwnedWorkersRejectWrongSourcesBeforeAction(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		for _, fault := range []string{"missing", "corrupt", "foreign_directory", "wrong_hash", "other_input", "missing_final_binding"} {
			t.Run(harness+"/"+fault, func(t *testing.T) {
				f, path := graphFinalContextWorkerFixture(t, harness)
				switch fault {
				case "missing":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				case "corrupt":
					body, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					body[0] ^= 1
					if err = os.WriteFile(path, body, 0600); err != nil {
						t.Fatal(err)
					}
				default:
					changeGraphContextPrompt(t, &f, func(base, _ map[string]any) {
						final := base["graphFinalVerification"].(map[string]any)
						access := final["access"].([]any)
						source := access[1].(map[string]any)
						switch fault {
						case "foreign_directory":
							foreign := filepath.Join(t.TempDir(), "000005.txt")
							body, err := os.ReadFile(path)
							if err != nil {
								t.Fatal(err)
							}
							if err = os.WriteFile(foreign, body, 0600); err != nil {
								t.Fatal(err)
							}
							source["path"] = foreign
						case "wrong_hash":
							source["artifactRef"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
						case "other_input":
							final["inputId"] = strings.Repeat("e", 32)
						case "missing_final_binding":
							final["access"] = access[1:]
						}
					})
				}
				_, code, working := runGraphContextWorker(t, harness, f)
				if code == 0 {
					t.Fatal("A wrong cumulative final source reached a successful owned action", fault)
				}
				if _, err := os.Stat(filepath.Join(working, GraphFileA)); !os.IsNotExist(err) {
					t.Fatal("A wrong cumulative final source reached the fixed workfile action", fault, err)
				}
			})
		}
	}
}
