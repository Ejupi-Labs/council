package workspace

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/project"
)

// These are actual local Git and destination guards, not synthetic owner or
// identity observations. Application preparation must never authorize a changed
// branch, repository directory or separately selected nested project directory.
func TestApplicationScopeBranchChangeBeforePreparePreservesDestination(t *testing.T) {
	m, repo, request := applicationFixture(t)
	applicationScopeUserEdits(t, repo)
	before := applicationScopeBytes(t, repo, "notes.txt", "delete.txt", "file.bin", "later-user.txt", ".git/index")
	applicationScopeChangeBranch(t, repo, request.Source)
	application, err := m.PrepareApply(context.Background(), request)
	if application != nil {
		t.Cleanup(func() { application.Close() })
		if closeErr := application.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if !errors.Is(err, ErrApplyConflict) || application != nil {
		t.Fatalf("changed branch prepared a destination operation: %v / %v", application, err)
	}
	applicationScopeUnchanged(t, repo, before, "added.txt")
}

func TestApplicationScopeBranchChangeAfterReadyPreventsFirstEffect(t *testing.T) {
	m, repo, request := applicationFixture(t)
	applicationScopeUserEdits(t, repo)
	before := applicationScopeBytes(t, repo, "notes.txt", "delete.txt", "file.bin", "later-user.txt", ".git/index")
	application, err := m.PrepareApply(context.Background(), request)
	if application != nil {
		t.Cleanup(func() { application.Close() })
	}
	if err != nil {
		t.Fatal("unchanged captured scope did not become ready", err)
	}
	actions := application.Actions()
	if len(actions) == 0 {
		t.Fatal("ready operation has no native effects to guard")
	}
	applicationScopeChangeBranch(t, repo, request.Source)
	// The branch switch itself does not rewrite the index. In particular, this
	// case cannot pass merely because a checkout changed an affected file/index.
	index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil || !bytes.Equal(index, before[".git/index"]) {
		t.Fatal("fixture branch change also changed the captured index", err)
	}
	result, err := application.ExecuteAction(context.Background(), actions[0].ID)
	if !errors.Is(err, ErrApplyConflict) || !result.Known || result.State == "applied" || result.State == "uncertain" || result.ObservedEvidence != nil || application.next != 0 {
		t.Fatalf("changed branch was not a known refusal before the first effect: %+v / %v", result, err)
	}
	if err = application.Close(); err != nil {
		t.Fatal("refused operation did not release retained destination handles", err)
	}
	applicationScopeUnchanged(t, repo, before, "added.txt")
	for _, action := range actions {
		if action.Action == "quarantine_before" {
			if _, err = os.Lstat(filepath.Join(m.lock.Root(), filepath.FromSlash(action.QuarantineRef))); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("branch refusal displaced an original into quarantine: %v", err)
			}
		}
	}
}

func TestApplicationScopePhysicalRepositoryReplacementBeforePreparePreservesBothRepositories(t *testing.T) {
	m, repo, request := applicationFixture(t)
	parent := filepath.Dir(repo)
	replacement := filepath.Join(parent, "scope-replacement-repository")
	// A local clone has the same branch, commit, tracked source and affected
	// index entries. Its different physical identity is the changed authority.
	runGit(t, parent, nil, "clone", "--no-hardlinks", "--", repo, replacement)
	applicationScopeUserEdits(t, repo)
	applicationScopeUserEdits(t, replacement)
	beforeOriginal := applicationScopeBytes(t, repo, "notes.txt", "delete.txt", "file.bin", "later-user.txt", ".git/index")
	beforeReplacement := applicationScopeBytes(t, replacement, "notes.txt", "delete.txt", "file.bin", "later-user.txt", ".git/index")
	foreign, err := project.Inspect(context.Background(), replacement, replacement)
	if err != nil || foreign.Identity == request.RepositoryIdentity {
		t.Fatal("fixture did not create a distinct actual repository identity", err)
	}
	if strings.TrimSpace(string(runGit(t, replacement, nil, "rev-parse", "HEAD"))) != request.Source.BaseCommit || strings.TrimSpace(string(runGit(t, replacement, nil, "symbolic-ref", "HEAD"))) != request.Source.Branch {
		t.Fatal("replacement fixture also changed the captured commit or branch")
	}
	retainedOriginal := applicationScopeSwapDirectories(t, parent, repo, replacement)
	observed, err := project.Inspect(context.Background(), repo, repo)
	if err != nil || observed.Identity != foreign.Identity {
		t.Fatal("replacement did not occupy the original repository pathname", err)
	}
	application, err := m.PrepareApply(context.Background(), request)
	if application != nil {
		t.Cleanup(func() { application.Close() })
		if closeErr := application.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if !errors.Is(err, ErrApplyConflict) || application != nil {
		t.Fatalf("replaced physical repository prepared an operation: %v / %v", application, err)
	}
	applicationScopeUnchanged(t, repo, beforeReplacement, "added.txt")
	applicationScopeUnchanged(t, retainedOriginal, beforeOriginal, "added.txt")
}

func TestApplicationScopeNestedProjectReplacementKeepsRepositoryIdentityAndPreservesBothDirectories(t *testing.T) {
	m, repo, _ := fixture(t)
	ctx := context.Background()
	nested := filepath.Join(repo, "nested")
	put(t, repo, "nested/notes.txt", []byte("original nested text\r\n"))
	runGit(t, repo, nil, "add", "nested/notes.txt")
	runGit(t, repo, nil, "commit", "-m", "nested application scope fixture")
	source, err := m.Capture(ctx, Request{AttemptID: attemptID, ProjectPath: nested})
	if err != nil {
		t.Fatal(err)
	}
	put(t, source.WorkspacePath, "nested/notes.txt", []byte("selected nested candidate\r\n"))
	candidate, err := m.Candidate(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := m.artifacts.VerifyClosed(ctx, candidate.ManifestArtifactID)
	if err != nil || metadata.SHA256 == nil {
		t.Fatal(err)
	}
	selected, err := project.Inspect(ctx, nested, repo)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := project.Inspect(ctx, repo, repo)
	if err != nil || selected.Identity == repository.Identity {
		t.Fatal("fixture did not separate project and repository identities", err)
	}
	request := ApplyRequest{AttemptID: strings.Repeat("e", 32), OperationID: strings.Repeat("f", 32), Source: source, SelectedCandidateID: candidate.ID, SelectedAttemptID: attemptID, CandidateManifestArtifactID: candidate.ManifestArtifactID, CandidateManifestSHA256: *metadata.SHA256, ProjectIdentity: selected.Identity, RepositoryIdentity: repository.Identity}
	applicationScopeUserEdits(t, repo)
	beforeRoot := applicationScopeBytes(t, repo, "file.bin", "later-user.txt", ".git/index")
	beforeNested := applicationScopeBytes(t, nested, "notes.txt")
	replacement := filepath.Join(repo, "scope-replacement-nested")
	if err = os.Mkdir(replacement, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, replacement, "notes.txt", beforeNested["notes.txt"])
	foreign, err := project.Inspect(ctx, replacement, repo)
	if err != nil || foreign.Identity == selected.Identity {
		t.Fatal("fixture did not create a distinct actual nested project", err)
	}
	retainedNested := applicationScopeSwapDirectories(t, repo, nested, replacement)
	afterRoot, err := project.Inspect(ctx, repo, repo)
	if err != nil || afterRoot.Identity != repository.Identity {
		t.Fatal("nested replacement changed the repository root identity", err)
	}
	afterProject, err := project.Inspect(ctx, nested, repo)
	if err != nil || afterProject.Identity != foreign.Identity {
		t.Fatal("replacement did not change the selected project's actual identity", err)
	}
	application, err := m.PrepareApply(ctx, request)
	if application != nil {
		t.Cleanup(func() { application.Close() })
		if closeErr := application.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	if !errors.Is(err, ErrApplyConflict) || application != nil {
		t.Fatalf("replaced nested project prepared an operation: %v / %v", application, err)
	}
	applicationScopeUnchanged(t, repo, beforeRoot)
	applicationScopeUnchanged(t, nested, beforeNested)
	applicationScopeUnchanged(t, retainedNested, beforeNested)
}

func applicationScopeChangeBranch(t *testing.T, repo string, source *Snapshot) {
	t.Helper()
	const branch = "refs/heads/council-scope-user-branch"
	runGit(t, repo, nil, "branch", "council-scope-user-branch", source.BaseCommit)
	// Metadata-only symbolic HEAD change is a real branch switch while retaining
	// exactly the captured commit, affected file bytes and raw user index.
	runGit(t, repo, nil, "symbolic-ref", "HEAD", branch)
	if strings.TrimSpace(string(runGit(t, repo, nil, "symbolic-ref", "HEAD"))) != branch || strings.TrimSpace(string(runGit(t, repo, nil, "rev-parse", "HEAD"))) != source.BaseCommit || source.Branch == branch {
		t.Fatal("fixture failed to isolate a real branch change at the same commit")
	}
}

func applicationScopeUserEdits(t *testing.T, repo string) {
	t.Helper()
	put(t, repo, "file.bin", []byte("later staged user bytes\x00\xff"))
	runGit(t, repo, nil, "add", "file.bin")
	put(t, repo, "file.bin", []byte("later unstaged user bytes\x00\xfe"))
	put(t, repo, "later-user.txt", []byte("untracked user bytes\r\n"))
}

func applicationScopeBytes(t *testing.T, directory string, paths ...string) map[string][]byte {
	t.Helper()
	result := map[string][]byte{}
	for _, name := range paths {
		data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal("cannot observe fixture bytes", name, err)
		}
		result[name] = data
	}
	return result
}

func applicationScopeUnchanged(t *testing.T, directory string, expected map[string][]byte, absent ...string) {
	t.Helper()
	for name, before := range expected {
		after, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("scope refusal changed %s: %q / %v", name, after, err)
		}
	}
	for _, name := range absent {
		if _, err := os.Lstat(filepath.Join(directory, filepath.FromSlash(name))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("scope refusal created %s: %v", name, err)
		}
	}
}

// Only these checked, same-parent, test-owned names are renamed. No recursive
// deletion is used, and restoration is registered before the first rename. The
// ordinary t.TempDir cleanup later owns removal of the separate replacement.
func applicationScopeSwapDirectories(t *testing.T, ownedParent, original, replacement string) string {
	t.Helper()
	parent, err := filepath.Abs(ownedParent)
	if err != nil {
		t.Fatal(err)
	}
	oldName, err := filepath.Abs(original)
	if err != nil {
		t.Fatal(err)
	}
	newName, err := filepath.Abs(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(oldName) != parent || filepath.Dir(newName) != parent || oldName == newName {
		t.Fatal("scope swap escaped its explicit fixture parent")
	}
	oldInfo, err := os.Lstat(oldName)
	if err != nil || !oldInfo.IsDir() || oldInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("scope original is not an owned ordinary directory", err)
	}
	newInfo, err := os.Lstat(newName)
	if err != nil || !newInfo.IsDir() || newInfo.Mode()&os.ModeSymlink != 0 || os.SameFile(oldInfo, newInfo) {
		t.Fatal("scope replacement is not a distinct ordinary directory", err)
	}
	retained := filepath.Join(parent, filepath.Base(oldName)+"-scope-retained-original")
	if _, err = os.Lstat(retained); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("scope retained-name fixture collision", err)
	}
	originalMoved, replacementMoved := false, false
	t.Cleanup(func() {
		if !originalMoved {
			return
		}
		retainedInfo, statErr := os.Lstat(retained)
		if statErr != nil || !os.SameFile(oldInfo, retainedInfo) {
			t.Error("refusing to restore an unrecognized retained original", statErr)
			return
		}
		if replacementMoved {
			current, currentErr := os.Lstat(oldName)
			if currentErr != nil || !os.SameFile(newInfo, current) {
				t.Error("refusing to move an unrecognized replacement", currentErr)
				return
			}
			if _, vacantErr := os.Lstat(newName); !errors.Is(vacantErr, os.ErrNotExist) {
				t.Error("replacement restoration slot is no longer vacant", vacantErr)
				return
			}
			if moveErr := os.Rename(oldName, newName); moveErr != nil {
				t.Error("cannot retain fixture replacement separately", moveErr)
				return
			}
		}
		if _, vacantErr := os.Lstat(oldName); !errors.Is(vacantErr, os.ErrNotExist) {
			t.Error("original restoration slot is no longer vacant", vacantErr)
			return
		}
		if restoreErr := os.Rename(retained, oldName); restoreErr != nil {
			t.Error("cannot restore original fixture pathname", restoreErr)
		}
	})
	if err = os.Rename(oldName, retained); err != nil {
		t.Fatal("cannot move the fixture original before application preparation", err)
	}
	originalMoved = true
	if err = os.Rename(newName, oldName); err != nil {
		t.Fatal("cannot place the actual replacement at the captured pathname", err)
	}
	replacementMoved = true
	return retained
}
