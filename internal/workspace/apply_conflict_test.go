package workspace

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationDoesNotOverwriteRecreatedDestinationAfterQuarantine(t *testing.T) {
	m, repo, request := applicationFixture(t)
	application, err := m.PrepareApply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { application.Close() })
	actions := application.Actions()
	quarantined := false
	for _, action := range actions {
		if action.RelativePath == "notes.txt" && action.Action == "publish_after" {
			if !quarantined {
				t.Fatal("replacement did not retain the original first")
			}
			put(t, repo, "notes.txt", []byte("concurrent user recreation\r\n"))
			result, executeErr := application.ExecuteAction(context.Background(), action.ID)
			if executeErr == nil || !result.Known || result.State != "conflict" {
				t.Fatalf("recreated destination was not a known exclusive refusal: %+v / %v", result, executeErr)
			}
			if err = application.Close(); err != nil {
				t.Fatal(err)
			}
			actual, readErr := os.ReadFile(filepath.Join(repo, "notes.txt"))
			if readErr != nil || string(actual) != "concurrent user recreation\r\n" {
				t.Fatalf("user recreation changed: %q / %v", actual, readErr)
			}
			for _, previous := range actions {
				if previous.RelativePath == "notes.txt" && previous.Action == "quarantine_before" {
					original, readErr := os.ReadFile(filepath.Join(m.lock.Root(), filepath.FromSlash(previous.QuarantineRef)))
					if readErr != nil || string(original) != "original text\r\n" {
						t.Fatalf("quarantine lost actual original: %q / %v", original, readErr)
					}
				}
			}
			// The instance cannot be reused to retry possible effects.
			if _, err = application.ExecuteAction(context.Background(), action.ID); !errors.Is(err, ErrApplyConflict) {
				t.Fatal("failed/closed application rearmed", err)
			}
			return
		}
		result, executeErr := application.ExecuteAction(context.Background(), action.ID)
		if executeErr != nil || result.State != "applied" || !result.Known {
			t.Fatalf("prior action failed: %+v / %v", result, executeErr)
		}
		if action.RelativePath == "notes.txt" && action.Action == "quarantine_before" {
			quarantined = true
		}
	}
	t.Fatal("fixture had no replacement publication")
}

func TestApplicationRejectsAffectedUserEditBeforeAnyDestinationEffect(t *testing.T) {
	m, repo, request := applicationFixture(t)
	put(t, repo, "notes.txt", []byte("later user edit\r\n"))
	application, err := m.PrepareApply(context.Background(), request)
	if application != nil {
		application.Close()
	}
	if !errors.Is(err, ErrApplyConflict) {
		t.Fatal("changed application basis accepted", err)
	}
	for name, expected := range map[string]string{"notes.txt": "later user edit\r\n", "delete.txt": "retained deletion before bytes\r\n"} {
		actual, readErr := os.ReadFile(filepath.Join(repo, name))
		if readErr != nil || string(actual) != expected {
			t.Fatalf("prepare changed %s: %q / %v", name, actual, readErr)
		}
	}
	if _, err = os.Lstat(filepath.Join(repo, "added.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("prepare published an addition", err)
	}
}

func TestApplicationRejectsHardlinkedOriginalWithoutChangingEitherName(t *testing.T) {
	m, repo, request := applicationFixture(t)
	outside := filepath.Join(t.TempDir(), "outside-original.txt")
	if err := os.Link(filepath.Join(repo, "notes.txt"), outside); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	application, err := m.PrepareApply(context.Background(), request)
	if application != nil {
		application.Close()
	}
	if !errors.Is(err, ErrApplyUnsupported) {
		t.Fatal("hardlinked application original accepted", err)
	}
	for _, name := range []string{outside, filepath.Join(repo, "notes.txt")} {
		data, readErr := os.ReadFile(name)
		after, statErr := os.Stat(name)
		if readErr != nil || statErr != nil || string(data) != "original text\r\n" || !os.SameFile(before, after) || after.Mode() != before.Mode() {
			t.Fatalf("hardlink refusal changed %s: %q / %v / %v", name, data, readErr, statErr)
		}
	}
}

func TestApplicationUsesRootSourceAndSelectedCorrectionManifest(t *testing.T) {
	m, repo, request := applicationFixture(t)
	ctx := context.Background()
	// The initial selected manifest already adds/deletes/replaces files. A fresh
	// correction workspace changes only the replacement; applying that relative
	// correction delta would incorrectly omit the initial addition and deletion.
	index, err := m.RetainIndex(ctx, request.Source.AttemptID, request.Source)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := m.Handoff(ctx, HandoffRequest{AttemptID: strings.Repeat("b", 32), Source: request.Source, CandidateManifestArtifactID: request.CandidateManifestArtifactID, IndexArtifactID: index.ID})
	if err != nil {
		t.Fatal(err)
	}
	put(t, correction.WorkspacePath, "notes.txt", []byte("corrected full candidate\r\n"))
	selected, err := m.Candidate(ctx, correction)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := m.artifacts.VerifyClosed(ctx, selected.ManifestArtifactID)
	if err != nil || metadata.SHA256 == nil {
		t.Fatal(err)
	}
	if len(selected.Changes) != 1 || selected.Changes[0].Path != "notes.txt" {
		t.Fatalf("fixture did not isolate the immediate correction delta: %+v", selected.Changes)
	}
	request.SelectedCandidateID = selected.ID
	request.SelectedAttemptID = correction.AttemptID
	request.CandidateManifestArtifactID = selected.ManifestArtifactID
	request.CandidateManifestSHA256 = *metadata.SHA256
	application, err := m.PrepareApply(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { application.Close() })
	if len(application.Actions()) != 4 {
		t.Fatalf("full cumulative delta was lost: %+v", application.Actions())
	}
	for _, action := range application.Actions() {
		result, executeErr := application.ExecuteAction(ctx, action.ID)
		if executeErr != nil || !result.Known || result.State != "applied" {
			t.Fatalf("cumulative action: %+v / %v", result, executeErr)
		}
	}
	if err = application.Close(); err != nil {
		t.Fatal(err)
	}
	for name, expected := range map[string][]byte{"notes.txt": []byte("corrected full candidate\r\n"), "added.txt": []byte("new controlled text\r\n")} {
		actual, readErr := os.ReadFile(filepath.Join(repo, name))
		if readErr != nil || !bytes.Equal(actual, expected) {
			t.Fatalf("cumulative output %s differs: %q / %v", name, actual, readErr)
		}
	}
	if _, err = os.Lstat(filepath.Join(repo, "delete.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("initial author deletion omitted", err)
	}
}
