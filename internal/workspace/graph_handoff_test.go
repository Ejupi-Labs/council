package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/project"
)

func TestGraphHandoffObservationUsesAdmittedSelectedSubject(t *testing.T) {
	f := graphMaterializeFixture(t)
	ctx := context.Background()
	author, err := f.m.MaterializeGraphInput(ctx, f.request)
	if err != nil {
		t.Fatal("actual receiving author input", err)
	}
	selectedStaged := []byte("selected author staged 東京\r\n\x00\xfd")
	selectedWorking := []byte("selected author working🙂\r\n\x00\xfc")
	put(t, author.Snapshot.WorkspacePath, "file.bin", selectedStaged)
	runGit(t, author.Snapshot.WorkspacePath, nil, "add", "file.bin")
	put(t, author.Snapshot.WorkspacePath, "file.bin", selectedWorking)
	candidate, err := f.m.Candidate(ctx, author.Snapshot)
	if err != nil {
		t.Fatal("actual selected candidate retention", err)
	}
	index, err := f.m.RetainIndex(ctx, author.Snapshot.AttemptID, author.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	manifestRef := graphArtifactRef(t, f.m, candidate.ManifestArtifactID)
	indexRef := graphIndexRef(index)
	var body bytes.Buffer
	if _, err = f.m.artifacts.CopyTo(ctx, manifestRef.ArtifactID, &body, MaxFileBytes); err != nil {
		t.Fatal(err)
	}
	var selected Manifest
	if err = json.Unmarshal(body.Bytes(), &selected, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	workingSHA, indexSHA := graphMaterializeExpectedDigests(t, selected)
	receivingAttempt := strings.Repeat("e", 32)
	actualHandoff, err := f.m.Handoff(ctx, HandoffRequest{AttemptID: receivingAttempt, Source: author.Snapshot, CandidateManifestArtifactID: manifestRef.ArtifactID, IndexArtifactID: indexRef.ArtifactID})
	if err != nil {
		t.Fatal("unchanged005 selected-subject handoff", err)
	}
	conditions := strings.Repeat("c", 64)
	selectedBinding := domain.CandidateBinding{RootSourceSnapshotID: author.Snapshot.ID, CandidateID: candidate.ID, SourceSnapshotID: author.Snapshot.ID,
		ManifestArtifactID: manifestRef.ArtifactID, ManifestSHA256: manifestRef.SHA256, IndexArtifactID: indexRef.ArtifactID, IndexSHA256: indexRef.SHA256, ConditionsSHA256: conditions}
	basis := domain.GraphSourceBasis{BasisKind: "node_subject", WorkflowID: f.request.SourceBasis.WorkflowID, StepID: strings.Repeat("d", 32), InputID: f.input.ID, InputSHA256: f.input.InputSHA256,
		CandidateBinding: &selectedBinding, ExpectedManifest: manifestRef, ExpectedIndex: indexRef, ExpectedWorkingEntriesSHA256: workingSHA, ExpectedIndexEntriesSHA256: indexSHA}
	result, err := f.m.ObserveGraphHandoff(ctx, GraphHandoffObservationRequest{GraphID: f.input.GraphID, ReceivingAttemptID: receivingAttempt, Role: "correction", InputBinding: f.input, SourceBasis: basis, Snapshot: actualHandoff})
	if err != nil {
		t.Fatal("actual selected-subject graph handoff observation unavailable", err)
	}
	if result.Snapshot == nil || result.Snapshot.ID != actualHandoff.ID || result.Receipt.State != "complete" || !result.Receipt.EffectsKnown || result.Receipt.EvidenceStatus != "complete" || result.Receipt.SourceBasis.CandidateBinding == nil || *result.Receipt.SourceBasis.CandidateBinding != selectedBinding || result.Receipt.InputID != f.input.ID || result.WorkspaceIdentity == "" || result.ReceiptArtifact.ProducerAttemptID != receivingAttempt {
		t.Fatalf("selected subject/origin/actual receiving identity lost: %+v", result)
	}
	if actual, err := os.ReadFile(filepath.Join(result.Snapshot.WorkspacePath, "file.bin")); err != nil || !bytes.Equal(actual, selectedWorking) || bytes.Equal(actual, f.working) {
		t.Fatal("later role reset to original graph input", err)
	}
	if actual := runGit(t, result.Snapshot.WorkspacePath, nil, "show", ":file.bin"); !bytes.Equal(actual, selectedStaged) {
		t.Fatal("later role lost selected staging")
	}
	if _, err = f.m.RecheckGraphMaterialization(ctx, result.Receipt, result.Snapshot, result.WorkspaceIdentity); err != nil {
		t.Fatal("selected-subject actual preGO proof unavailable", err)
	}
	badBinding := selectedBinding
	badBinding.ManifestSHA256 = strings.Repeat("f", 64)
	badBasis := basis
	badBasis.CandidateBinding = &badBinding
	bad, badErr := f.m.ObserveGraphHandoff(ctx, GraphHandoffObservationRequest{GraphID: f.input.GraphID, ReceivingAttemptID: receivingAttempt, Role: "review", InputBinding: f.input, SourceBasis: badBasis, Snapshot: actualHandoff})
	if badErr == nil || bad.Receipt.State == "complete" {
		t.Fatal("inconsistent admitted selected-subject hash was reinterpreted", badErr)
	}
	if actual, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(actual, f.originalIndex) {
		t.Fatal("later graph stage changed user index", err)
	}
}

func TestGraphMaterializationRecheckRejectsActualManagedIndexMutation(t *testing.T) {
	f := graphMaterializeFixture(t)
	result, err := f.m.MaterializeGraphInput(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.m.RecheckGraphMaterialization(context.Background(), result.Receipt, result.Snapshot, result.WorkspaceIdentity); err != nil {
		t.Fatal("initial actual proof", err)
	}
	// The actual owned stage, not the user's project, is changed after proof.
	put(t, result.Snapshot.WorkspacePath, "file.bin", []byte("new staged content after proof\r\n"))
	runGit(t, result.Snapshot.WorkspacePath, nil, "add", "file.bin")
	put(t, result.Snapshot.WorkspacePath, "file.bin", f.working)
	if _, err = f.m.RecheckGraphMaterialization(context.Background(), result.Receipt, result.Snapshot, result.WorkspaceIdentity); err == nil {
		t.Fatal("managed index mutation survived before-GO recheck")
	}
	if actual, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(actual, f.originalIndex) {
		t.Fatal("index counterexample changed user index", err)
	}
}

func TestGraphMaterializationRecheckRejectsReplacedDirectoryWithCopiedGitControl(t *testing.T) {
	f := graphMaterializeFixture(t)
	result, err := f.m.MaterializeGraphInput(context.Background(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.m.RecheckGraphMaterialization(context.Background(), result.Receipt, result.Snapshot, result.WorkspaceIdentity); err != nil {
		t.Fatal("initial actual proof", err)
	}
	path := result.Snapshot.WorkspacePath
	before, err := project.Inspect(context.Background(), path, path)
	if err != nil {
		t.Fatal(err)
	}
	retained := path + "-retained"
	if err = os.Rename(path, retained); err != nil {
		t.Fatal("owned directory replacement setup", err)
	}
	if err = os.CopyFS(path, os.DirFS(retained)); err != nil {
		t.Fatal("owned copied-control fixture", err)
	}
	after, err := project.Inspect(context.Background(), path, path)
	if err != nil || before.Identity == after.Identity || before.Identity != result.WorkspaceIdentity {
		t.Fatal("fixture did not replace actual directory identity", err)
	}
	control, err := os.ReadFile(filepath.Join(path, ".git"))
	if err != nil || hashBytes(control) != result.Snapshot.GitControlSHA256 {
		t.Fatal("fixture did not retain exact copied .git bytes", err)
	}
	if _, err = f.m.RecheckGraphMaterialization(context.Background(), result.Receipt, result.Snapshot, result.WorkspaceIdentity); err == nil {
		t.Fatal("copied .git control replaced native directory identity proof")
	}
}
