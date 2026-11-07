package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestGraphCapturedOutputProofReadsCandidateAndResponseFromClosedMembersOnly(t *testing.T) {
	f := graphOutputFixture(t)
	ctx := context.Background()
	candidate, err := f.f.m.CaptureGraphOutput(ctx, f.candidateRequest)
	if err != nil {
		t.Fatal(err)
	}
	response, err := f.f.m.CaptureGraphOutput(ctx, f.responseRequest)
	if err != nil {
		t.Fatal(err)
	}
	// Both formerly readable mutable sources now disagree with the captured
	// evidence. This verifier must not recover state from either directory.
	put(t, f.f.repo, "file.bin", []byte("Later user source must remain independent.\n"))
	put(t, f.author.Snapshot.WorkspacePath, "file.bin", []byte("Later old worker bytes are not retained output proof.\n"))
	entries, err := os.ReadDir(filepath.Join(f.f.m.lock.Root(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, selected := range []struct {
		name            string
		captured        GraphOutputCapture
		working, staged []byte
	}{{"candidate_after", candidate, f.afterWorking, f.afterStaged}, {"response_source", response, f.f.working, f.f.staged}} {
		t.Run(selected.name, func(t *testing.T) {
			proof, err := f.f.m.VerifyCapturedGraphOutput(ctx, selected.captured)
			if err != nil {
				t.Fatal("Actual captured output has no full retained integrity proof", err)
			}
			refs := map[string]bool{}
			for _, ref := range proof.Artifacts {
				if refs[ref.ArtifactID] {
					t.Fatal("Full output proof duplicated a retained member", ref)
				}
				refs[ref.ArtifactID] = true
				if err := f.f.m.graphVerifyRef(ctx, ref); err != nil {
					t.Fatal("Returned output proof contains an unverified tuple", ref, err)
				}
			}
			for _, id := range []string{selected.captured.ClosureArtifact.ArtifactID, selected.captured.Closure.Manifest.ArtifactID, selected.captured.Closure.Index.ArtifactID, selected.captured.Closure.StagedBlobs.ArtifactID} {
				if !refs[id] {
					t.Fatal("Full output proof omitted a closed envelope/index anchor", id)
				}
			}
			var actualWorking bytes.Buffer
			object := ""
			for _, member := range proof.Manifest.Entries {
				if member.Type == "file" && !refs[member.ArtifactID] {
					t.Fatal("Output proof omitted a retained ordinary member", member)
				}
				if member.Path == "file.bin" {
					if _, err := f.f.m.artifacts.CopyTo(ctx, member.ArtifactID, &actualWorking, MaxFileBytes); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, member := range proof.Manifest.Index {
				if member.Path == "file.bin" {
					object = member.Object
				}
			}
			for _, blob := range proof.StagedBlobs.Entries {
				if !refs[blob.ArtifactRef.ArtifactID] {
					t.Fatal("Output proof omitted an actual staged object payload", blob)
				}
			}
			blob := graphIndexBlob(t, proof.StagedBlobs, object)
			var staged, raw bytes.Buffer
			if _, err := f.f.m.artifacts.CopyTo(ctx, blob.ArtifactRef.ArtifactID, &staged, MaxFileBytes); err != nil {
				t.Fatal(err)
			}
			if _, err := f.f.m.artifacts.CopyTo(ctx, selected.captured.Closure.Index.ArtifactID, &raw, MaxFileBytes); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actualWorking.Bytes(), selected.working) || !bytes.Equal(staged.Bytes(), selected.staged) || !bytes.Equal(proof.RawIndex, raw.Bytes()) || bytes.Equal(actualWorking.Bytes(), staged.Bytes()) {
				t.Fatal("Captured candidate-after/source-response working and staged states were conflated")
			}
			if selected.captured.Closure.ResponseArtifact != nil && !refs[selected.captured.Closure.ResponseArtifact.ArtifactID] {
				t.Fatal("Source-bound response proof omitted the exact retained response")
			}
		})
	}
	after, err := os.ReadDir(filepath.Join(f.f.m.lock.Root(), "artifacts"))
	if err != nil || len(after) != len(entries) {
		t.Fatal("Read-only output verification created or removed evidence", err)
	}
}

func TestGraphCapturedOutputProofRejectsCorruptWorkingOrStagedMember(t *testing.T) {
	for _, dimension := range []string{"working", "index"} {
		t.Run(dimension, func(t *testing.T) {
			f := graphOutputFixture(t)
			ctx := context.Background()
			captured, err := f.f.m.CaptureGraphOutput(ctx, f.candidateRequest)
			if err != nil {
				t.Fatal(err)
			}
			body, err := f.f.m.graphIndexLoad(ctx, captured.Closure.Manifest, "candidate_manifest")
			var manifest Manifest
			if err != nil || json.Unmarshal(body, &manifest) != nil {
				t.Fatal("Actual closed output manifest prerequisite failed", err)
			}
			id, object := "", ""
			for _, member := range manifest.Entries {
				if member.Path == "file.bin" {
					id = member.ArtifactID
				}
			}
			for _, member := range manifest.Index {
				if member.Path == "file.bin" {
					object = member.Object
				}
			}
			if dimension == "index" {
				staged := graphIndexManifest(t, graphIndexCase{m: f.f.m}, captured.Closure.StagedBlobs)
				id = graphIndexBlob(t, staged, object).ArtifactRef.ArtifactID
			}
			meta, err := f.f.m.artifacts.VerifyClosed(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(f.f.m.lock.Root(), filepath.FromSlash(meta.RelativePath))
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(path)
			if err != nil || len(original) == 0 {
				t.Fatal("Actual member has no bytes to corrupt", err)
			}
			file, err := os.OpenFile(path, os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := file.WriteAt([]byte{original[0] ^ 0x20}, 0)
			syncErr, closeErr := file.Sync(), file.Close()
			after, statErr := os.Stat(path)
			if writeErr != nil || syncErr != nil || closeErr != nil || statErr != nil || !os.SameFile(before, after) || before.Size() != after.Size() {
				t.Fatal("Controlled corruption replaced/resized its actual member", writeErr, syncErr, closeErr, statErr)
			}
			if proof, err := f.f.m.VerifyCapturedGraphOutput(ctx, captured); err == nil || len(proof.Artifacts) != 0 {
				t.Fatal("Corrupt retained output member produced a usable complete proof", proof, err)
			}
			if _, err = f.f.m.graphIndexLoad(ctx, captured.ClosureArtifact, "graph_output_closure"); err != nil {
				t.Fatal("The fault changed the envelope instead of its retained payload", err)
			}
		})
	}
}

func TestGraphCapturedOutputProofRejectsDifferentActualClosedEnvelope(t *testing.T) {
	f := graphOutputFixture(t)
	ctx := context.Background()
	candidate, err := f.f.m.CaptureGraphOutput(ctx, f.candidateRequest)
	if err != nil {
		t.Fatal(err)
	}
	response, err := f.f.m.CaptureGraphOutput(ctx, f.responseRequest)
	if err != nil || response.ClosureArtifact.ArtifactID == candidate.ClosureArtifact.ArtifactID {
		t.Fatal("Distinct real source-response envelope prerequisite failed", err)
	}
	wrong := candidate
	wrong.ClosureArtifact = response.ClosureArtifact
	if proof, err := f.f.m.VerifyCapturedGraphOutput(ctx, wrong); err == nil || len(proof.Artifacts) != 0 {
		t.Fatal("A different genuine closed envelope qualified the selected captured tuple", proof, err)
	}
}
