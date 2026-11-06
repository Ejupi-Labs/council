package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// This is a review-driven regression authored after the closure verifier. It
// uses real registered private bytes and filesystem corruption, not fabricated
// hash/identity tuples. No process execution or successful settlement is invented.
func TestWorkflowClosureRejectsCorruptUnchangedRetainedMemberWithValidManifests(t *testing.T) {
	for _, memberBasis := range []string{"root_source", "selected_candidate"} {
		t.Run(memberBasis, func(t *testing.T) {
			ctx := context.Background()
			e, projectID, projectPath := testEngine(t)
			stopFixtureScheduler(t, e)
			if err := os.Mkdir(filepath.Join(projectPath, "docs"), 0700); err != nil {
				t.Fatal(err)
			}
			beforeDocument := []byte("\ufeffControlled task document\r\nold\r\n東京 �\r\n")
			afterDocument := bytes.Replace(beforeDocument, []byte("old"), []byte("new"), 1)
			if err := os.WriteFile(filepath.Join(projectPath, "docs", "note.md"), beforeDocument, 0600); err != nil {
				t.Fatal(err)
			}
			input := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Replace \"old\" with \"new\" in docs/note.md only", ExecutionMode: "fake", FakeScenario: "workflow-document"}
			result := e.StartTask(ctx, input)
			if result.Status != 201 {
				t.Fatal(result)
			}
			admission := decodeAdmission(t, result)
			if admission.Workflow == nil {
				t.Fatal("controlled author did not capture actual workflow authority")
			}
			claimed, err := e.db.ClaimAttempt(ctx, admission.Attempt.ID, e.config.OwnerInstanceID)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := e.workspace.Capture(ctx, workspace.Request{AttemptID: claimed.ID, ProjectPath: projectPath})
			if err != nil {
				t.Fatal(err)
			}
			_, identity, err := e.db.Project(ctx, projectID)
			if err != nil {
				t.Fatal(err)
			}
			if err = e.registerWorkflowSnapshot(ctx, claimed, snapshot, identity); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(snapshot.ExecutionPath, "docs", "note.md"), afterDocument, 0600); err != nil {
				t.Fatal(err)
			}
			candidate, err := e.retainLocalCandidate(ctx, claimed, snapshot)
			if err != nil || candidate.Completeness != "complete" || candidate.IndexArtifactID == "" {
				t.Fatal("controlled candidate/current index was not retained completely", candidate, err)
			}
			subject, err := e.candidateWorkflowSubject(ctx, *admission.Workflow, candidate.ID)
			if err != nil {
				t.Fatal("positive complete subject did not verify", err)
			}
			// Select the actual registered candidate in the controller's inspection
			// input. This does not settle the author or persist acceptance authority.
			current := *admission.Workflow
			current.SelectedCandidateID = &candidate.ID
			progress, err := e.inspectWorkflowProgress(ctx, admission.Task.ID, current)
			if err != nil || !progress.direct || !progress.hasDelta || progress.hasIndexDelta {
				t.Fatal("positive direct-proof/acceptance prerequisites were not viable", progress.decision, err)
			}
			rootMember := closureTestEntry(t, subject.RootManifest, "source.txt")
			selectedMember := closureTestEntry(t, subject.Manifest, "source.txt")
			if rootMember.Type != "file" || selectedMember.Type != "file" || rootMember.SHA256 != selectedMember.SHA256 || rootMember.Size != selectedMember.Size || rootMember.Mode != selectedMember.Mode {
				t.Fatal("fixture member was not unchanged outside the target document", rootMember, selectedMember)
			}
			member := rootMember
			if memberBasis == "selected_candidate" {
				member = selectedMember
			}
			metadata, err := e.evidence.VerifyClosed(ctx, member.ArtifactID)
			if err != nil || metadata.Kind != "source_file" || metadata.SHA256 == nil || *metadata.SHA256 != member.SHA256 {
				t.Fatal("unchanged member has no actual retained source_file proof", metadata, err)
			}
			rootManifestID, selectedManifestID := subject.RootSource.ManifestArtifactID, candidate.ManifestArtifactID
			_, rootBefore, err := e.workflowManifest(ctx, rootManifestID)
			if err != nil {
				t.Fatal(err)
			}
			_, selectedBefore, err := e.workflowManifest(ctx, selectedManifestID)
			if err != nil {
				t.Fatal(err)
			}
			// Modify exactly one existing owned artifact, preserving its inode and
			// byte length. A manifest-only or length/identity-only check must fail.
			if !filepath.IsLocal(filepath.FromSlash(metadata.RelativePath)) {
				t.Fatal("controlled artifact did not name a private relative file")
			}
			privatePath := filepath.Join(e.lock.Root(), filepath.FromSlash(metadata.RelativePath))
			before, err := os.Stat(privatePath)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(privatePath)
			if err != nil || string(original) != "original\n" {
				t.Fatal("fixture did not select the unrelated original file", string(original), err)
			}
			changed := append([]byte{}, original...)
			changed[0] ^= 0x20
			file, err := os.OpenFile(privatePath, os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			n, writeErr := file.WriteAt(changed, 0)
			syncErr, closeErr := file.Sync(), file.Close()
			if writeErr != nil || syncErr != nil || closeErr != nil || n != len(original) {
				t.Fatal("controlled private corruption did not complete", writeErr, syncErr, closeErr)
			}
			after, err := os.Stat(privatePath)
			if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() {
				t.Fatal("fixture replaced/resized the artifact instead of testing content integrity", err)
			}
			for _, check := range []struct {
				id   string
				meta evidence.Metadata
			}{{rootManifestID, rootBefore}, {selectedManifestID, selectedBefore}} {
				_, actual, manifestErr := e.workflowManifest(ctx, check.id)
				if manifestErr != nil || actual.SHA256 == nil || check.meta.SHA256 == nil || *actual.SHA256 != *check.meta.SHA256 {
					t.Fatal("test corrupted a manifest instead of its unchanged member", manifestErr)
				}
			}
			for _, manifest := range []workspace.Manifest{subject.RootManifest, subject.Manifest} {
				target := closureTestEntry(t, manifest, "docs/note.md")
				if target.ArtifactID == member.ArtifactID {
					t.Fatal("corruption selected the changed task document")
				}
				if _, err = e.evidence.VerifyClosed(ctx, target.ArtifactID); err != nil {
					t.Fatal("the actual target document was corrupted", err)
				}
			}
			if proofs, proofErr := e.verifyWorkflowArtifacts(ctx, subject.artifactIDs()...); proofErr != nil || len(proofs) > 256 {
				t.Fatal("bounded top-level proof list changed rather than exposing member corruption", proofs, proofErr)
			}
			if _, err = e.evidence.VerifyClosed(ctx, member.ArtifactID); !errors.Is(err, evidence.ErrCorrupt) {
				t.Fatal("real altered member did not produce content-integrity rejection", err)
			}
			if _, err = e.candidateWorkflowSubject(ctx, current, candidate.ID); !errors.Is(err, evidence.ErrCorrupt) {
				t.Fatal("candidate subject accepted corrupted unchanged evidence behind valid manifests", err)
			}
			if _, err = e.selectedWorkflowSubject(ctx, current); !errors.Is(err, evidence.ErrCorrupt) {
				t.Fatal("current selected subject bypassed complete retained closure", err)
			}
			if _, err = e.inspectWorkflowProgress(ctx, admission.Task.ID, current); !errors.Is(err, evidence.ErrCorrupt) {
				t.Fatal("acceptance/advance prerequisites accepted a corrupted unchanged member", err)
			}
			if actual, err := os.ReadFile(filepath.Join(projectPath, "source.txt")); err != nil || string(actual) != "original\n" {
				t.Fatal("controlled corruption touched the user's original project", err)
			}
			if actual, err := os.ReadFile(filepath.Join(projectPath, "docs", "note.md")); err != nil || !bytes.Equal(actual, beforeDocument) {
				t.Fatal("controlled fixture changed the original task document", err)
			}
		})
	}
}

func closureTestEntry(t *testing.T, manifest workspace.Manifest, name string) workspace.Entry {
	t.Helper()
	for _, entry := range manifest.Entries {
		if entry.Path == name {
			return entry
		}
	}
	t.Fatal("controlled manifest omitted required member", name)
	return workspace.Entry{}
}
