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
)

type graphPhysicalContribution struct {
	selection       GraphContributionSource
	snapshot        *Snapshot
	closure         GraphOutputCapture
	working, staged []byte
	path, object    string
}

// The acceptance tuple is deliberately synthetic: these cases qualify physical
// closure/retention, while actual current acceptance/SQL ownership is tested by Engine.
func graphPhysicalContributionFixture(t *testing.T, f graphMaterializeCase, n int, path string) graphPhysicalContribution {
	t.Helper()
	ctx := context.Background()
	retention := graphInitialRetainRequest(t, f)
	retention.OperationAttemptID = composeID(100 + n)
	retention.InputID = composeID(200 + n)
	retention.ReceivingNodeID = composeID(300 + n)
	retention.ReceivingTaskID = composeID(400 + n)
	retention.Context.InputID = retention.InputID
	retention.Context.ReceivingNodeID = retention.ReceivingNodeID
	input, err := f.m.RetainGraphInput(ctx, retention)
	if err != nil {
		t.Fatal(err)
	}
	request := f.request
	request.ReceivingAttemptID = composeID(500 + n)
	request.InputBinding = input
	request.SourceBasis.InputID = input.ID
	request.SourceBasis.InputSHA256 = input.InputSHA256
	request.SourceBasis.ExpectedManifest = input.Manifest
	request.SourceBasis.ExpectedIndex = input.Index
	author, err := f.m.MaterializeGraphInput(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	staged := []byte("\xef\xbb\xbfcontrolled staged branch " + string(rune('A'+n)) + " 東京\r\n")
	working := []byte("controlled working branch " + string(rune('A'+n)) + "🙂\r\n")
	put(t, author.Snapshot.WorkspacePath, path, staged)
	runGit(t, author.Snapshot.WorkspacePath, nil, "add", path)
	object := strings.TrimSpace(string(runGit(t, author.Snapshot.WorkspacePath, nil, "rev-parse", ":"+path)))
	put(t, author.Snapshot.WorkspacePath, path, working)
	candidate, err := f.m.Candidate(ctx, author.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	index, err := f.m.RetainIndex(ctx, author.Snapshot.AttemptID, author.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	manifest := graphArtifactRef(t, f.m, candidate.ManifestArtifactID)
	indexRef := graphIndexRef(index)
	binding := domain.CandidateBinding{RootSourceSnapshotID: author.Snapshot.ID, CandidateID: candidate.ID, SourceSnapshotID: author.Snapshot.ID, ManifestArtifactID: manifest.ArtifactID, ManifestSHA256: manifest.SHA256, IndexArtifactID: indexRef.ArtifactID, IndexSHA256: indexRef.SHA256, ConditionsSHA256: strings.Repeat("a", 64)}
	closure, err := f.m.CaptureGraphOutput(ctx, GraphOutputCaptureRequest{GraphID: input.GraphID, NodeID: input.ReceivingNodeID, ProducerAttemptID: author.Snapshot.AttemptID, ReceivedInputID: input.ID, Kind: "candidate", Source: author.Snapshot, CandidateBinding: &binding, Manifest: manifest, Index: indexRef})
	if err != nil {
		t.Fatal(err)
	}
	selection := GraphContributionSource{Binding: domain.AcceptedContribution{EdgeID: composeID(600 + n), EdgeKind: "files", FromNodeID: input.ReceivingNodeID, TaskID: input.ReceivingTaskID, TaskVersion: input.ReceivingTaskVersion, AcceptanceKind: "workflow", AcceptanceID: composeID(700 + n), ProducerAttemptID: author.Snapshot.AttemptID, InputID: input.ID, CandidateBinding: &binding, OutputClosureRef: closure.ClosureArtifact, ConstraintRefs: []domain.GraphScopedReference{}, EvidenceRefs: []domain.GraphContributedEvidence{}}, InputBinding: input}
	return graphPhysicalContribution{selection: selection, snapshot: author.Snapshot, closure: closure, working: working, staged: staged, path: path, object: object}
}

func graphPhysicalComposeRequest(t *testing.T, f graphMaterializeCase, inputs ...graphPhysicalContribution) GraphComposedInputRetainRequest {
	t.Helper()
	base := graphInitialRetainRequest(t, f)
	base.OperationAttemptID = composeID(900)
	base.InputID = composeID(901)
	base.ReceivingNodeID = composeID(902)
	base.ReceivingTaskID = composeID(903)
	base.Context.InputID = base.InputID
	base.Context.ReceivingNodeID = base.ReceivingNodeID
	request := GraphComposedInputRetainRequest{Base: base, Contributions: []GraphContributionSource{}, CaseSensitive: true}
	for _, input := range inputs {
		request.Contributions = append(request.Contributions, input.selection)
	}
	return request
}

func TestGraphComposedInputUsesClosedActualBranchesAndRestoresSelectedStagedBlob(t *testing.T) {
	f := graphMaterializeFixture(t)
	a := graphPhysicalContributionFixture(t, f, 1, "branch-a.txt")
	b := graphPhysicalContributionFixture(t, f, 2, "branch-b.txt")
	// Later worker edits are not selected output evidence.
	put(t, a.snapshot.WorkspacePath, a.path, []byte("later unselected worker edit\r\n"))
	ctx := context.Background()
	for _, branch := range []graphPhysicalContribution{a, b} {
		if _, err := f.m.VerifyGraphOutput(ctx, GraphOutputVerifyRequest{RootBinding: f.root.Binding, Contribution: branch.selection.Binding, InputBinding: branch.selection.InputBinding}); err != nil {
			t.Fatal("actual full closed selected output verification unavailable", err)
		}
	}
	// Only a new uncommitted loose object in this disposable repository is removed.
	objectPath := filepath.Join(f.repo, ".git", "objects", a.object[:2], a.object[2:])
	info, err := os.Lstat(objectPath)
	if err != nil || !info.Mode().IsRegular() || !within(filepath.Join(f.repo, ".git", "objects"), objectPath) {
		t.Fatal("controlled removable object precondition", err)
	}
	if err = os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	request := graphPhysicalComposeRequest(t, f, a, b)
	result, err := f.m.RetainComposedGraphInput(ctx, request)
	if err != nil {
		t.Fatal("actual closed fan-in retention unavailable", err)
	}
	if result.Binding == nil || len(result.Conflicts) != 0 || result.State != "complete" || !result.EffectsKnown || result.EvidenceStatus != "complete" || result.OperationEffects == nil || result.Snapshot == nil || result.Snapshot.AttemptID != request.Base.OperationAttemptID {
		t.Fatalf("root operation did not retain actual composed input: %+v", result)
	}
	input := *result.Binding
	if input.OperationAttemptID != request.Base.OperationAttemptID || len(input.Contributions) != 2 || len(input.ContributorVector) != 2 {
		t.Fatal("composed ownership/contributor vector lost")
	}
	verified, err := f.m.VerifyGraphInput(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	workingSHA, indexSHA, err := GraphManifestDigests(verified.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	receive := f.request
	receive.ReceivingAttemptID = composeID(904)
	receive.InputBinding = input
	receive.SourceBasis.InputID = input.ID
	receive.SourceBasis.InputSHA256 = input.InputSHA256
	receive.SourceBasis.ExpectedManifest = input.Manifest
	receive.SourceBasis.ExpectedIndex = input.Index
	receive.SourceBasis.ExpectedWorkingEntriesSHA256 = workingSHA
	receive.SourceBasis.ExpectedIndexEntriesSHA256 = indexSHA
	follower, err := f.m.MaterializeGraphInput(ctx, receive)
	if err != nil {
		t.Fatal("actual composed follower workspace unavailable", err)
	}
	for _, branch := range []graphPhysicalContribution{a, b} {
		actual, err := os.ReadFile(filepath.Join(follower.Snapshot.WorkspacePath, branch.path))
		if err != nil || !bytes.Equal(actual, branch.working) {
			t.Fatal("follower did not receive actual closed working branch", err)
		}
		if actual = runGit(t, follower.Snapshot.WorkspacePath, nil, "show", ":"+branch.path); !bytes.Equal(actual, branch.staged) {
			t.Fatal("follower staged bytes differ from selected closed branch")
		}
	}
	if _, err = os.Stat(objectPath); err != nil {
		t.Fatal("selected pruned blob was not actually restored", err)
	}
	if actual, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(actual, f.originalIndex) {
		t.Fatal("composition changed original user raw index", err)
	}
}

func TestGraphComposedInputRetainsPhysicalConflictWithoutWorkspaceOrUsableBinding(t *testing.T) {
	f := graphMaterializeFixture(t)
	a := graphPhysicalContributionFixture(t, f, 1, "file.bin")
	b := graphPhysicalContributionFixture(t, f, 2, "file.bin")
	before, err := os.ReadDir(filepath.Join(f.m.lock.Root(), "workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	request := graphPhysicalComposeRequest(t, f, a, b)
	result, err := f.m.RetainComposedGraphInput(context.Background(), request)
	if err != nil || result.Binding != nil || len(result.Conflicts) == 0 || result.ConflictArtifact == nil || result.State != "conflicted" || !result.EffectsKnown {
		t.Fatal("physical conflict became chosen/usable input or lost typed evidence", err, result)
	}
	body, err := f.m.graphIndexLoad(context.Background(), *result.ConflictArtifact, "graph_input_provenance")
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Contract  string          `json:"contract"`
		Conflicts []InputConflict `json:"conflicts"`
	}
	if err = json.Unmarshal(body, &record); err != nil || record.Contract != "006-input-conflict-v1" || len(record.Conflicts) == 0 {
		t.Fatal("conflict record is not complete typed provenance", err)
	}
	after, err := os.ReadDir(filepath.Join(f.m.lock.Root(), "workspaces"))
	if err != nil || len(after) != len(before) {
		t.Fatal("conflicting input created a managed receiver/root assembly workspace", err)
	}
}
