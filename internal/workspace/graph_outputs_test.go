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

type graphOutputCase struct {
	f                                 graphMaterializeCase
	author                            GraphMaterialization
	candidateRequest, responseRequest GraphOutputCaptureRequest
	afterWorking, afterStaged         []byte
}

func graphOutputFixture(t *testing.T) graphOutputCase {
	t.Helper()
	f := graphMaterializeFixture(t)
	ctx := context.Background()
	author, err := f.m.MaterializeGraphInput(ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	afterStaged := []byte("candidate after staged 東京\r\n\x00\xfb")
	afterWorking := []byte("candidate after working🙂\r\n\x00\xfa")
	put(t, author.Snapshot.WorkspacePath, "file.bin", afterStaged)
	runGit(t, author.Snapshot.WorkspacePath, nil, "add", "file.bin")
	put(t, author.Snapshot.WorkspacePath, "file.bin", afterWorking)
	candidate, err := f.m.Candidate(ctx, author.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	index, err := f.m.RetainIndex(ctx, author.Snapshot.AttemptID, author.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	manifestRef := graphArtifactRef(t, f.m, candidate.ManifestArtifactID)
	indexRef := graphIndexRef(index)
	conditions := strings.Repeat("c", 64)
	candidateBinding := domain.CandidateBinding{RootSourceSnapshotID: author.Snapshot.ID, CandidateID: candidate.ID, SourceSnapshotID: author.Snapshot.ID, ManifestArtifactID: manifestRef.ArtifactID, ManifestSHA256: manifestRef.SHA256, IndexArtifactID: indexRef.ArtifactID, IndexSHA256: indexRef.SHA256, ConditionsSHA256: conditions}
	response, err := f.m.artifacts.Put(ctx, author.Snapshot.AttemptID, "final_response", []byte("\xef\xbb\xbfexact native response🙂\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	responseRef := graphIndexRef(response)
	sourceManifest := graphArtifactRef(t, f.m, author.Snapshot.ManifestArtifactID)
	sourceIndex := graphArtifactRef(t, f.m, author.Snapshot.IndexArtifactID)
	responseBinding := domain.ResponseBinding{ProducerAttemptID: author.Snapshot.AttemptID, ResponseArtifactID: response.ID, ResponseSHA256: responseRef.SHA256, ResponseByteLength: responseRef.ByteLength, SourceSnapshotID: author.Snapshot.ID, ManifestSHA256: sourceManifest.SHA256, IndexArtifactID: sourceIndex.ArtifactID, IndexSHA256: sourceIndex.SHA256, ConditionsSHA256: conditions}
	base := GraphOutputCaptureRequest{GraphID: f.input.GraphID, NodeID: f.input.ReceivingNodeID, ProducerAttemptID: author.Snapshot.AttemptID, ReceivedInputID: f.input.ID, Source: author.Snapshot}
	candidateRequest := base
	candidateRequest.Kind = "candidate"
	candidateRequest.CandidateBinding = &candidateBinding
	candidateRequest.Manifest = manifestRef
	candidateRequest.Index = indexRef
	responseRequest := base
	responseRequest.Kind = "response"
	responseRequest.ResponseBinding = &responseBinding
	responseRequest.Manifest = sourceManifest
	responseRequest.Index = sourceIndex
	responseRequest.SourceMaterialization = &author.Receipt
	return graphOutputCase{f: f, author: author, candidateRequest: candidateRequest, responseRequest: responseRequest, afterWorking: afterWorking, afterStaged: afterStaged}
}

func graphOutputPayload(t *testing.T, f graphOutputCase, closure domain.GraphOutputClosure) ([]byte, []byte) {
	t.Helper()
	ctx := context.Background()
	body, err := f.f.m.graphIndexLoad(ctx, closure.Manifest, "source_manifest", "candidate_manifest")
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err = json.Unmarshal(body, &manifest, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	var working bytes.Buffer
	var object string
	for _, entry := range manifest.Entries {
		if entry.Path == "file.bin" {
			if _, err = f.f.m.artifacts.CopyTo(ctx, entry.ArtifactID, &working, MaxFileBytes); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, entry := range manifest.Index {
		if entry.Path == "file.bin" {
			object = entry.Object
		}
	}
	staged := graphIndexManifest(t, graphIndexCase{m: f.f.m}, closure.StagedBlobs)
	blob := graphIndexBlob(t, staged, object)
	var payload bytes.Buffer
	if _, err = f.f.m.artifacts.CopyTo(ctx, blob.ArtifactRef.ArtifactID, &payload, MaxFileBytes); err != nil {
		t.Fatal(err)
	}
	return working.Bytes(), payload.Bytes()
}

func TestGraphOutputCapturesCandidateAfterAndResponseSourceSeparately(t *testing.T) {
	f := graphOutputFixture(t)
	ctx := context.Background()
	candidate, err := f.f.m.CaptureGraphOutput(ctx, f.candidateRequest)
	if err != nil {
		t.Fatal("actual after-state candidate output closure unavailable", err)
	}
	response, err := f.f.m.CaptureGraphOutput(ctx, f.responseRequest)
	if err != nil {
		t.Fatal("actual source-bound response output closure unavailable", err)
	}
	for _, result := range []GraphOutputCapture{candidate, response} {
		if result.ClosureArtifact.ProducerAttemptID != f.author.Snapshot.AttemptID || result.Closure.StagedBlobs.ProducerAttemptID != f.author.Snapshot.AttemptID || result.Closure.ProducerAttemptID != f.author.Snapshot.AttemptID || result.Closure.ReceivedInputID != f.f.input.ID {
			t.Fatal("output closure lost actual graph producer/input")
		}
		seal, err := domain.GraphOutputDigest(result.Closure)
		if err != nil || seal != result.Closure.ClosureSHA256 {
			t.Fatal("output closure is not exact sealed evidence", err)
		}
		body, err := f.f.m.graphIndexLoad(ctx, result.ClosureArtifact, "graph_output_closure")
		if err != nil {
			t.Fatal(err)
		}
		var stored domain.GraphOutputClosure
		if err = json.Unmarshal(body, &stored, json.RejectUnknownMembers(true)); err != nil || stored.ClosureSHA256 != seal {
			t.Fatal("published output association has no exact closed record", err)
		}
	}
	if candidate.Closure.CandidateID == nil || *candidate.Closure.CandidateID != f.candidateRequest.CandidateBinding.CandidateID || candidate.Closure.ResponseArtifact != nil || response.Closure.CandidateID != nil || response.Closure.ResponseArtifact == nil || response.Closure.ResponseArtifact.ArtifactID != f.responseRequest.ResponseBinding.ResponseArtifactID {
		t.Fatal("candidate/response identity was fabricated or conflated")
	}
	cw, cs := graphOutputPayload(t, f, candidate.Closure)
	rw, rs := graphOutputPayload(t, f, response.Closure)
	if !bytes.Equal(cw, f.afterWorking) || !bytes.Equal(cs, f.afterStaged) || !bytes.Equal(rw, f.f.working) || !bytes.Equal(rs, f.f.staged) {
		t.Fatal("response source was substituted with candidate-after working/index state")
	}
	if actual, err := os.ReadFile(filepath.Join(f.f.repo, ".git", "index")); err != nil || !bytes.Equal(actual, f.f.originalIndex) {
		t.Fatal("graph output capture changed user raw index", err)
	}
}

func TestGraphOutputResponseRequiresClosedSourceStagedProof(t *testing.T) {
	f := graphOutputFixture(t)
	f.responseRequest.SourceMaterialization = nil
	result, err := f.f.m.CaptureGraphOutput(context.Background(), f.responseRequest)
	if err == nil || result.ClosureArtifact.ArtifactID != "" {
		t.Fatal("response inferred original staging from mutable current index", err)
	}
}

func TestGraphOutputResponsePreservesDistinctNoDeltaIndexBinding(t *testing.T) {
	f := graphMaterializeFixture(t)
	ctx := context.Background()
	author, err := f.m.MaterializeGraphInput(ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := f.m.Candidate(ctx, author.Snapshot)
	if err != nil || candidate == nil || len(candidate.Changes) != 0 {
		t.Fatal("Actual unchanged response producer did not retain a zero-delta candidate", err)
	}
	after, err := f.m.RetainIndex(ctx, author.Snapshot.AttemptID, author.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	sourceManifest := graphArtifactRef(t, f.m, author.Snapshot.ManifestArtifactID)
	sourceIndex := graphArtifactRef(t, f.m, author.Snapshot.IndexArtifactID)
	afterIndex := graphIndexRef(after)
	if sourceIndex.ArtifactID == afterIndex.ArtifactID {
		t.Fatal("Fixture did not retain distinct pre-command and after-command raw index artifacts")
	}
	response, err := f.m.artifacts.Put(ctx, author.Snapshot.AttemptID, "final_response", []byte("Exact zero-delta response 東京\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	responseRef := graphIndexRef(response)
	binding := domain.ResponseBinding{ProducerAttemptID: author.Snapshot.AttemptID, ResponseArtifactID: response.ID, ResponseSHA256: responseRef.SHA256, ResponseByteLength: responseRef.ByteLength,
		SourceSnapshotID: author.Snapshot.ID, ManifestSHA256: sourceManifest.SHA256, IndexArtifactID: afterIndex.ArtifactID, IndexSHA256: afterIndex.SHA256, ConditionsSHA256: strings.Repeat("d", 64)}
	request := GraphOutputCaptureRequest{GraphID: f.input.GraphID, NodeID: f.input.ReceivingNodeID, ProducerAttemptID: author.Snapshot.AttemptID, ReceivedInputID: f.input.ID, Kind: "response", Source: author.Snapshot,
		ResponseBinding: &binding, Manifest: sourceManifest, Index: afterIndex, SourceMaterialization: &author.Receipt}
	result, err := f.m.CaptureGraphOutput(ctx, request)
	if err != nil {
		t.Fatal("Legal no-delta response with a separately retained exact index binding was rejected", err)
	}
	if result.Closure.Index != afterIndex || binding.IndexArtifactID != afterIndex.ArtifactID || result.Closure.ResponseArtifact == nil || *result.Closure.ResponseArtifact != responseRef {
		t.Fatal("Exact acknowledged response/index binding was rewritten")
	}
	working, staged := graphOutputPayload(t, graphOutputCase{f: f, author: author}, result.Closure)
	if !bytes.Equal(working, f.working) || !bytes.Equal(staged, f.staged) {
		t.Fatal("Distinct raw artifact identity substituted the pre-command source payloads")
	}
	// A genuinely different semantic index must still fail. Only raw artifact
	// identity/stat-cache differences are permitted across the two retained views.
	put(t, author.Snapshot.WorkspacePath, "file.bin", []byte("Changed staged payload must never become response-source evidence.\n"))
	runGit(t, author.Snapshot.WorkspacePath, nil, "add", "file.bin")
	changed, err := f.m.RetainIndex(ctx, author.Snapshot.AttemptID, author.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	changedIndex := graphIndexRef(changed)
	changedBinding := binding
	changedBinding.IndexArtifactID, changedBinding.IndexSHA256 = changedIndex.ArtifactID, changedIndex.SHA256
	request.Index, request.ResponseBinding = changedIndex, &changedBinding
	if invalid, err := f.m.CaptureGraphOutput(ctx, request); err == nil || invalid.ClosureArtifact.ArtifactID != "" {
		t.Fatal("Changed semantic staging qualified as the unchanged response source", err)
	}
	if actual, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(actual, f.originalIndex) {
		t.Fatal("Response output retention changed the original user's raw index", err)
	}
}
