package workspace

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func graphPreparedContextFixture(t *testing.T) (graphMaterializeCase, GraphMaterialization, GraphContextPrepareRequest) {
	t.Helper()
	f := graphMaterializeFixture(t)
	retention := graphInitialRetainRequest(t, f)
	input, err := f.m.RetainGraphInput(context.Background(), retention)
	if err != nil {
		t.Fatal(err)
	}
	request := f.request
	request.InputBinding = input
	request.SourceBasis.InputID = input.ID
	request.SourceBasis.InputSHA256 = input.InputSHA256
	request.SourceBasis.ExpectedManifest = input.Manifest
	request.SourceBasis.ExpectedIndex = input.Index
	author, err := f.m.MaterializeGraphInput(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return f, author, GraphContextPrepareRequest{ReceivingAttemptID: author.Snapshot.AttemptID, InputBinding: input}
}

func TestGraphPreparedContextCopiesExactOriginalsOutsideCandidateAndRechecks(t *testing.T) {
	f, author, request := graphPreparedContextFixture(t)
	prepared, err := f.m.PrepareGraphContext(context.Background(), request)
	if err != nil {
		t.Fatal("actual supported native context preparation unavailable", err)
	}
	defer prepared.Close()
	if prepared.State != "complete" || !prepared.EffectsKnown || prepared.EvidenceStatus != "complete" || prepared.DirectoryIdentity == "" || prepared.EffectsArtifact == nil || prepared.EffectsArtifact.ProducerAttemptID != request.ReceivingAttemptID || len(prepared.Entries) != 2 {
		t.Fatalf("prepared context lost private identity/actual ownership: %+v", prepared)
	}
	rel, err := filepath.Rel(author.Snapshot.WorkspacePath, prepared.Directory)
	if err != nil || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatal("context bytes were injected into candidate workspace", err)
	}
	if filepath.Dir(prepared.Directory) != filepath.Dir(author.Snapshot.WorkspacePath) {
		t.Fatal("context did not use an isolated private sibling directory")
	}
	for _, entry := range prepared.Entries {
		if entry.Mode != "file" || entry.Path == "" || entry.Text != nil || entry.Identity == "" {
			t.Fatal("file access was fabricated as inline/native consumption")
		}
		file, err := os.Open(entry.Path)
		if err != nil {
			t.Fatal("actual supplied context path unreadable", err)
		}
		privacyErr := localstate.VerifyPrivateFile(file, false)
		closeErr := file.Close()
		if privacyErr != nil || closeErr != nil {
			t.Fatal(privacyErr, closeErr)
		}
		actual, err := os.ReadFile(entry.Path)
		if err != nil || hashBytes(actual) != entry.ArtifactRef.SHA256 {
			t.Fatal("context copy differs from exact closed original", err)
		}
		if !bytes.Contains(actual, []byte("\r\n")) {
			t.Fatal("context transport normalized original line endings")
		}
	}
	if _, err = f.m.RecheckGraphContext(context.Background(), prepared); err != nil {
		t.Fatal("actual fresh copied context proof unavailable", err)
	}
	candidate, err := f.m.Candidate(context.Background(), author.Snapshot)
	if err != nil || len(candidate.Changes) != 0 {
		t.Fatal("context preparation became authored candidate delta", err)
	}
	file, err := os.OpenFile(prepared.Entries[0].Path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteAt([]byte("X"), 0)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	if _, err = f.m.RecheckGraphContext(context.Background(), prepared); err == nil {
		t.Fatal("changed actual private context copy survived preGO proof")
	}
	if _, err = f.m.artifacts.VerifyClosed(context.Background(), prepared.Entries[0].ArtifactRef.ArtifactID); err != nil {
		t.Fatal("copy counterexample changed original evidence", err)
	}
	if actual, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(actual, f.originalIndex) {
		t.Fatal("context preparation changed user's exact index", err)
	}
}

func TestGraphPreparedContextPreservesUnresolvedExistingDirectory(t *testing.T) {
	f, _, request := graphPreparedContextFixture(t)
	path := filepath.Join(f.m.lock.Root(), "workspaces", "graph-context-"+request.ReceivingAttemptID)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(path, "retained-prefix.txt")
	if err := os.WriteFile(marker, []byte("unresolved owned prefix\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := f.m.PrepareGraphContext(context.Background(), request)
	defer result.Close()
	if err == nil || result.State == "complete" {
		t.Fatal("unresolved context directory was silently reused", err)
	}
	entries, readErr := os.ReadDir(path)
	if readErr != nil || len(entries) != 1 || entries[0].Name() != "retained-prefix.txt" {
		t.Fatal("context preparation modified unresolved prefix", readErr)
	}
	actual, readErr := os.ReadFile(marker)
	if readErr != nil || string(actual) != "unresolved owned prefix\r\n" {
		t.Fatal("context prefix was overwritten", readErr)
	}
}
