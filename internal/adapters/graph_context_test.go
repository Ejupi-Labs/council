package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphContextHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// This is a pure adapter tuple fixture, not artifact ownership/native-read proof.
// Workspace and real fixed-native tests qualify the actual files separately.
func graphAdapterContext(t *testing.T, original string) GraphNativeContext {
	t.Helper()
	dir := t.TempDir()
	shared := []byte("\xef\xbb\xbfShared graph constraints 東京\r\n")
	sharedRef := domain.GraphArtifactRef{ArtifactID: strings.Repeat("1", 32), ProducerAttemptID: strings.Repeat("3", 32), SHA256: graphContextHash(shared), ByteLength: strconv.Itoa(len(shared))}
	nodeRef := domain.GraphArtifactRef{ArtifactID: strings.Repeat("2", 32), ProducerAttemptID: strings.Repeat("3", 32), SHA256: graphContextHash([]byte(original)), ByteLength: strconv.Itoa(len([]byte(original)))}
	file := filepath.Join(dir, "000000.bin")
	if err := os.WriteFile(file, shared, 0600); err != nil {
		t.Fatal(err)
	}
	packet := domain.GraphContextPacket{ID: strings.Repeat("6", 32), GraphID: strings.Repeat("5", 32), InputID: strings.Repeat("4", 32), InputSHA256: strings.Repeat("a", 64), ReceivingTaskVersion: "1", GraphInstructionsRef: sharedRef, NodeInstructionsRef: nodeRef,
		ConstraintRefs: []domain.GraphScopedReference{{Ref: "shared-original", ArtifactRef: sharedRef, AuthorityScope: "graph_shared", Paths: []string{}}}, ContributedEvidence: []domain.GraphContributedEvidence{},
		ProvidedAccess: []domain.GraphProvidedAccess{{Ref: "shared-original", ArtifactRef: sharedRef, Scope: "whole_original", Paths: []string{}, Mode: "file"}, {Ref: "node-original", ArtifactRef: nodeRef, Scope: "whole_original", Paths: []string{}, Mode: "inline"}}}
	var err error
	packet.PacketSHA256, err = domain.GraphContextDigest(packet)
	if err != nil {
		t.Fatal(err)
	}
	text := original
	return GraphNativeContext{Packet: packet, Directory: dir, Entries: []GraphContextEntry{{Ref: "shared-original", ArtifactRef: sharedRef, Scope: "whole_original", Paths: []string{}, Mode: "file", Path: file}, {Ref: "node-original", ArtifactRef: nodeRef, Scope: "whole_original", Paths: []string{}, Mode: "inline", Text: &text}}}
}

func TestGraphNativeContextPreservesPinnedReadonlyRoleAndExactBaseFields(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			in := roleFixture(t, harness)
			if harness == "codex" {
				in.Version.Pins.Model = "danger-full-access"
			} else {
				in.Version.Pins.Model = "bypassPermissions"
			}
			base, err := BuildRole(context.Background(), in, func(context.Context, domain.ExecutionPins) (string, error) {
				return filepath.Join(t.TempDir(), "controlled-native"), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			args := append([]string{}, base.Args...)
			prompt := append([]byte{}, base.Prompt...)
			graph := graphAdapterContext(t, in.Original)
			connection := "codex-native"
			if harness == "claude-code" {
				connection = "claude-native"
			}
			got, err := AddGraphContext(base, connection, graph)
			if err != nil {
				t.Fatal("supported separate graph native context unavailable", err)
			}
			if !reflect.DeepEqual(base.Args, args) || !bytes.Equal(base.Prompt, prompt) {
				t.Fatal("graph context mutated captured base launch")
			}
			if !bytes.HasPrefix(got.Prompt, prompt[:len(prompt)-1]) {
				t.Fatal("graph context rewrote original role JSON fields")
			}
			if got.Executable != base.Executable || !reflect.DeepEqual(got.Observations, base.Observations) {
				t.Fatal("context invented native target/permission/telemetry facts")
			}
			if harness == "codex" {
				if !reflect.DeepEqual(got.Args, args) {
					t.Fatal("read-only Codex context added a writable-root/changed pin flag")
				}
			} else {
				if len(got.Args) != len(args)+2 || !reflect.DeepEqual(got.Args[:len(args)], args) || got.Args[len(args)] != "--add-dir" || got.Args[len(args)+1] != graph.Directory {
					t.Fatal("Claude did not grant only the exact prepared context directory")
				}
			}
			at := bytes.IndexByte(got.Prompt, '\n')
			var payload struct {
				Original string `json:"originalInstructions"`
				Graph    struct {
					Packet domain.GraphContextPacket `json:"packet"`
					Access []struct {
						Ref       string  `json:"ref"`
						Path      *string `json:"path"`
						Text      *string `json:"text"`
						BaseField *string `json:"baseField"`
					} `json:"access"`
				} `json:"graphContext"`
			}
			if at < 0 || json.Unmarshal(got.Prompt[at+1:], &payload) != nil || payload.Original != in.Original || payload.Graph.Packet.PacketSHA256 != graph.Packet.PacketSHA256 || len(payload.Graph.Access) != 2 {
				t.Fatal("complete graph packet/base original not carried in supported prompt shape")
			}
			var node, shared bool
			for _, access := range payload.Graph.Access {
				if access.Ref == "node-original" {
					node = access.BaseField != nil && *access.BaseField == "originalInstructions" && access.Text == nil
				}
				if access.Ref == "shared-original" {
					shared = access.Path != nil && *access.Path == graph.Entries[0].Path && access.Text == nil
				}
			}
			if !node || !shared {
				t.Fatal("receiver original duplicated or graph original file unavailable")
			}
		})
	}
}

func TestGraphNativeContextCountsExistingRoleEvidenceInCombinedLimit(t *testing.T) {
	in := roleFixture(t, "codex")
	in.Evidence = []SuppliedEvidence{}
	for n := 0; n < 64; n++ {
		in.Evidence = append(in.Evidence, SuppliedEvidence{Ref: "existing-" + strconv.Itoa(n), Path: filepath.Join(t.TempDir(), "evidence.bin"), Scope: "whole_candidate", Paths: []string{}})
	}
	base, err := BuildRole(context.Background(), in, func(context.Context, domain.ExecutionPins) (string, error) {
		return filepath.Join(t.TempDir(), "controlled-native"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AddGraphContext(base, "codex-native", graphAdapterContext(t, in.Original)); err == nil {
		t.Fatal("graph and workflow sections each independently admitted64 refs")
	}
}

func TestGraphNativeContextEnforcesActualCombinedPromptByteLimit(t *testing.T) {
	in := roleFixture(t, "codex")
	in.Original = ""
	resolver := func(context.Context, domain.ExecutionPins) (string, error) {
		return filepath.Join(t.TempDir(), "controlled-native"), nil
	}
	empty, err := BuildRole(context.Background(), in, resolver)
	if err != nil {
		t.Fatal(err)
	}
	in.Original = strings.Repeat("x", domain.MaxPromptBytes-len(empty.Prompt)-128)
	base, err := BuildRole(context.Background(), in, resolver)
	if err != nil || len(base.Prompt) > domain.MaxPromptBytes {
		t.Fatal("valid near-boundary base setup", err)
	}
	if _, err = AddGraphContext(base, "codex-native", graphAdapterContext(t, in.Original)); err == nil {
		t.Fatal("complete combined graph+role context exceeded256KiB without blocking")
	}
}
