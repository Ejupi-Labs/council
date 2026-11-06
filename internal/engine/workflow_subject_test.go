package engine

import (
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func TestCandidateChangesReadsImmutableRootBasisAndBindsEveryCursor(t *testing.T) {
	ctx := context.Background()
	e, projectID, projectPath := testEngine(t)
	stopFixtureScheduler(t, e)
	input := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Inspect the exact retained changes.", ExecutionMode: "fake", Workflow: &domain.WorkflowInput{Mode: "author_only"}}
	result := e.StartTask(ctx, input)
	if result.Status != 201 {
		t.Fatal(result)
	}
	admission := decodeAdmission(t, result)
	if _, err := e.db.ClaimAttempt(ctx, admission.Attempt.ID, e.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := e.workspace.Capture(ctx, workspace.Request{AttemptID: admission.Attempt.ID, ProjectPath: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	_, identity, err := e.db.Project(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.registerWorkflowSnapshot(ctx, admission.Attempt, snapshot, identity); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{"source.txt": "\ufeffchanged\r\n東京\r\n", "added.txt": "added\n"} {
		if err = os.WriteFile(filepath.Join(snapshot.ExecutionPath, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	candidate, err := e.workspace.Candidate(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(e.lock.Root(), snapshot.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.db.PutCandidate(ctx, domain.Candidate{ID: candidate.ID, AttemptID: admission.Attempt.ID, SourceSnapshotID: snapshot.ID, ManifestArtifactID: candidate.ManifestArtifactID, DeltaArtifactID: candidate.DeltaArtifactID, WorkspaceRef: filepath.ToSlash(relative), HasDelta: true, CreatedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	// Reads must not recompute a patch from mutable user files or the old worker.
	if err = os.WriteFile(filepath.Join(projectPath, "source.txt"), []byte("later user edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(snapshot.ExecutionPath, "source.txt"), []byte("later workspace edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first := e.readWorkflow(ctx, "candidates.changes.list", candidate.ID, map[string]string{"limit": "1"})
	if first.Status != 200 || first.Envelope.Receipt != nil {
		t.Fatal(first)
	}
	page := first.Envelope.Data.(domain.Page[workspace.Change])
	if len(page.Items) != 1 || page.Items[0].Path != "added.txt" || page.NextCursor == nil {
		t.Fatal(page)
	}
	second := e.readWorkflow(ctx, "candidates.changes.list", candidate.ID, map[string]string{"limit": "1", "cursor": *page.NextCursor})
	if second.Status != 200 {
		t.Fatal(second)
	}
	last := second.Envelope.Data.(domain.Page[workspace.Change])
	if len(last.Items) != 1 || last.Items[0].Path != "source.txt" || last.NextCursor != nil {
		t.Fatal(last)
	}
	var value candidateChangesCursor
	raw, err := base64.RawURLEncoding.DecodeString(*page.NextCursor)
	if err != nil || json.Unmarshal(raw, &value) != nil {
		t.Fatal(err)
	}
	for _, wrong := range []string{"foreign_candidate", "foreign_basis", "unsafe_position", "malformed"} {
		copy := value
		switch wrong {
		case "foreign_candidate":
			copy.CandidateID = domain.NewID()
		case "foreign_basis":
			copy.RootManifest = strings.Repeat("0", 64)
		case "unsafe_position":
			copy.After = "9007199254740992"
		}
		encoded, err := json.Marshal(copy)
		if err != nil {
			t.Fatal(err)
		}
		cursor := base64.RawURLEncoding.EncodeToString(encoded)
		if wrong == "malformed" {
			cursor += "="
		}
		rejected := e.readWorkflow(ctx, "candidates.changes.list", candidate.ID, map[string]string{"cursor": cursor})
		if rejected.Envelope.Error == nil || rejected.Envelope.Error.Code != "invalid_cursor" {
			t.Fatal("Foreign or unsafe candidate cursor accepted", wrong, rejected)
		}
	}
}
