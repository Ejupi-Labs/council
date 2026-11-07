package adapters

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// This pure adapter tuple does not claim native reading or artifact ownership.
func graphAdapterFinalContext(t *testing.T, original string) GraphNativeContext {
	t.Helper()
	value := graphAdapterContext(t, original)
	body := []byte("Complete controlled final basis 東京\r\n")
	ref := domain.GraphArtifactRef{ArtifactID: strings.Repeat("7", 32), ProducerAttemptID: strings.Repeat("8", 32), SHA256: graphContextHash(body), ByteLength: strconv.Itoa(len(body))}
	path := filepath.Join(value.Directory, "000002.txt")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	value.ReceivingAttemptID, value.FinalVerificationRef = strings.Repeat("9", 32), &ref
	value.Supplemental = []GraphContextEntry{{Ref: "final-verification", ArtifactRef: ref, Scope: "cumulative_final", Paths: []string{}, Mode: "file", Path: path}}
	return value
}

func TestGraphNativeFinalContextPreservesOriginalPinsAndOneReadonlyDirectory(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			in := roleFixture(t, harness)
			in.Version.Pins.Model = "literal --model 東京"
			base, err := BuildRole(context.Background(), in, func(context.Context, domain.ExecutionPins) (string, error) {
				return filepath.Join(t.TempDir(), "controlled-native"), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			originalPrompt, originalArgs := append([]byte{}, base.Prompt...), append([]string{}, base.Args...)
			prepared := graphAdapterFinalContext(t, in.Original)
			connection := "codex-native"
			if harness == "claude-code" {
				connection = "claude-native"
			}
			got, err := AddGraphContext(base, connection, prepared)
			if err != nil {
				t.Fatal("Supported complete cumulative context transport unavailable", err)
			}
			if !bytes.Equal(base.Prompt, originalPrompt) || !reflect.DeepEqual(base.Args, originalArgs) || !bytes.HasPrefix(got.Prompt, originalPrompt[:len(originalPrompt)-1]) || !reflect.DeepEqual(got.Observations, base.Observations) {
				t.Fatal("Final context changed original task fields, pins, role or observations")
			}
			expectedArgs := originalArgs
			if harness == "claude-code" {
				expectedArgs = append(append([]string{}, originalArgs...), "--add-dir", prepared.Directory)
			}
			if !reflect.DeepEqual(got.Args, expectedArgs) {
				t.Fatal("Final context changed readonly flags or added another directory", got.Args)
			}
			var payload struct {
				Original string `json:"originalInstructions"`
				Graph    struct {
					Packet domain.GraphContextPacket `json:"packet"`
				} `json:"graphContext"`
				Final struct {
					Contract             string                  `json:"contract"`
					ReceivingAttemptID   string                  `json:"receivingAttemptId"`
					FinalVerificationRef domain.GraphArtifactRef `json:"finalVerificationRef"`
					Access               []graphPromptAccess     `json:"access"`
				} `json:"graphFinalVerification"`
			}
			at := bytes.IndexByte(got.Prompt, '\n')
			if at < 0 || json.Unmarshal(got.Prompt[at+1:], &payload) != nil || payload.Original != in.Original || !reflect.DeepEqual(payload.Graph.Packet, prepared.Packet) || payload.Final.Contract != "006-final-context-v1" || payload.Final.ReceivingAttemptID != prepared.ReceivingAttemptID || payload.Final.FinalVerificationRef != *prepared.FinalVerificationRef || len(payload.Final.Access) != 1 {
				t.Fatal("Final and ordinary context were not carried separately and completely")
			}
			access := payload.Final.Access[0]
			if access.Ref != "final-verification" || access.ArtifactRef != *prepared.FinalVerificationRef || access.Scope != "cumulative_final" || access.Mode != "file" || access.Path == nil || *access.Path != prepared.Supplemental[0].Path || access.Text != nil || access.BaseField != nil {
				t.Fatal("Supplemental final bytes were replaced by a digest or original-authority claim", access)
			}
		})
	}
}

func TestGraphNativeFinalContextCountsActualRoleAndSupplementalReferences(t *testing.T) {
	in := roleFixture(t, "codex")
	in.Evidence = []SuppliedEvidence{}
	// The ordinary graph fixture has four distinct supplied names: the two
	// required instruction names plus shared-original and node-original aliases.
	for i := 0; i < 60; i++ {
		in.Evidence = append(in.Evidence, SuppliedEvidence{Ref: "ordinary-" + strconv.Itoa(i), Path: filepath.Join(t.TempDir(), "source.bin"), Scope: "whole_candidate", Paths: []string{}})
	}
	base, err := BuildRole(context.Background(), in, func(context.Context, domain.ExecutionPins) (string, error) {
		return filepath.Join(t.TempDir(), "controlled-native"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	value := graphAdapterFinalContext(t, in.Original)
	ordinary := value
	ordinary.FinalVerificationRef, ordinary.Supplemental = nil, nil
	if _, err = AddGraphContext(base, "codex-native", ordinary); err != nil {
		t.Fatal("Exact 64-reference ordinary setup was not valid", err)
	}
	if _, err = AddGraphContext(base, "codex-native", value); err == nil || domain.SafeError(err).Code != "context_capacity" {
		t.Fatal("Final evidence escaped the actual combined 64-reference bound", err)
	}
}

func TestGraphNativeFinalContextEnforcesCompleteCombinedPromptBytes(t *testing.T) {
	in := roleFixture(t, "codex")
	in.Original = ""
	resolve := func(context.Context, domain.ExecutionPins) (string, error) {
		return filepath.Join(t.TempDir(), "controlled-native"), nil
	}
	empty, err := BuildRole(context.Background(), in, resolve)
	if err != nil {
		t.Fatal(err)
	}
	in.Original = strings.Repeat("x", domain.MaxPromptBytes-len(empty.Prompt)-5000)
	base, err := BuildRole(context.Background(), in, resolve)
	if err != nil {
		t.Fatal(err)
	}
	value := graphAdapterFinalContext(t, in.Original)
	ordinary := value
	ordinary.FinalVerificationRef, ordinary.Supplemental = nil, nil
	if _, err = AddGraphContext(base, "codex-native", ordinary); err != nil {
		t.Fatal("Ordinary near-capacity prompt setup was invalid", err)
	}
	// One ordinary role ref + four graph names + 59 supplemental names =64;
	// this case must reach the byte bound rather than the reference bound.
	for i := 1; i < 59; i++ {
		extra := value.Supplemental[0]
		extra.Ref = "final-before-" + strconv.Itoa(i)
		extra.Paths = []string{"docs/controlled-" + strconv.Itoa(i) + ".txt"}
		value.Supplemental = append(value.Supplemental, extra)
	}
	if _, err = AddGraphContext(base, "codex-native", value); err == nil || domain.SafeError(err).Code != "input_too_large" {
		t.Fatal("Final prompt bytes were admitted above 256 KiB", err)
	}
}
