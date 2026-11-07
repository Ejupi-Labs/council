package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"path/filepath"
	"strconv"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type workflowSubject struct {
	Workflow     domain.Workflow
	Candidate    *domain.Candidate
	Binding      *domain.CandidateBinding
	Response     *domain.ResponseBinding
	Source       domain.SourceSnapshot
	Manifest     workspace.Manifest
	RootSource   domain.SourceSnapshot
	RootManifest workspace.Manifest
}

func (e *Engine) workflowManifest(ctx context.Context, id string) (workspace.Manifest, evidence.Metadata, error) {
	var manifest workspace.Manifest
	var content bytes.Buffer
	meta, err := e.evidence.CopyTo(ctx, id, &content, 64<<20)
	if err != nil {
		return manifest, meta, err
	}
	if meta.SHA256 == nil || !meta.Closed || meta.Completeness != "complete" || meta.Kind != "source_manifest" && meta.Kind != "candidate_manifest" {
		return manifest, meta, evidence.ErrCorrupt
	}
	if err = json.Unmarshal(content.Bytes(), &manifest, json.RejectUnknownMembers(true)); err != nil {
		return manifest, meta, evidence.ErrCorrupt
	}
	if len(manifest.Entries) > workspace.MaxEntries || len(manifest.Index) > workspace.MaxEntries {
		return manifest, meta, evidence.ErrCorrupt
	}
	return manifest, meta, nil
}

func (e *Engine) verifyWorkflowArtifacts(ctx context.Context, ids ...string) ([]domain.VerifiedResponse, error) {
	verified := make([]domain.VerifiedResponse, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		meta, err := e.evidence.VerifyClosed(ctx, id)
		if err != nil {
			return nil, err
		}
		if meta.SHA256 == nil || !meta.Closed || meta.Completeness != "complete" {
			return nil, evidence.ErrCorrupt
		}
		verified = append(verified, domain.VerifiedResponse{ArtifactID: id, SHA256: *meta.SHA256, PhysicalIdentity: meta.Identity, ByteLength: meta.ByteLength})
	}
	return verified, nil
}

func (e *Engine) candidateWorkflowSubject(ctx context.Context, w domain.Workflow, candidateID string) (workflowSubject, error) {
	out := workflowSubject{Workflow: w}
	candidate, err := e.db.Candidate(ctx, candidateID)
	if err != nil {
		return out, err
	}
	if candidate.WorkflowID != w.ID || candidate.ConditionsSHA256 != w.Authority.ConditionsSHA256 || candidate.Completeness != "complete" {
		return out, domain.Fail("candidate_stale", "The selected candidate does not match the complete captured workflow subject.")
	}
	source, err := e.db.SourceSnapshot(ctx, candidate.SourceSnapshotID)
	if err != nil {
		return out, err
	}
	manifest, meta, err := e.workflowManifest(ctx, candidate.ManifestArtifactID)
	if err != nil {
		return out, err
	}
	index, err := e.evidence.VerifyClosed(ctx, candidate.IndexArtifactID)
	if err != nil || index.SHA256 == nil || index.AttemptID != candidate.AttemptID {
		return out, evidence.ErrCorrupt
	}
	root, err := e.db.SourceSnapshot(ctx, candidate.RootSourceSnapshotID)
	if err != nil {
		return out, err
	}
	rootManifest, rootMeta, err := e.workflowManifest(ctx, root.ManifestArtifactID)
	if err != nil || rootMeta.SHA256 == nil || *rootMeta.SHA256 != root.ManifestSHA256 {
		return out, evidence.ErrCorrupt
	}
	if err := e.verifyWorkflowClosure(ctx, rootManifest, manifest); err != nil {
		return out, err
	}
	out.Candidate, out.Source, out.Manifest, out.RootSource, out.RootManifest = &candidate, source, manifest, root, rootManifest
	out.Binding = &domain.CandidateBinding{RootSourceSnapshotID: root.ID, CandidateID: candidate.ID, SourceSnapshotID: source.ID, ManifestArtifactID: candidate.ManifestArtifactID, ManifestSHA256: *meta.SHA256, IndexArtifactID: index.ID, IndexSHA256: *index.SHA256, ConditionsSHA256: w.Authority.ConditionsSHA256}
	return out, nil
}

func (e *Engine) selectedWorkflowSubject(ctx context.Context, w domain.Workflow) (workflowSubject, error) {
	if w.SelectedCandidateID != nil {
		return e.candidateWorkflowSubject(ctx, w, *w.SelectedCandidateID)
	}
	out := workflowSubject{Workflow: w, Response: w.SelectedResponseBinding}
	if out.Response == nil {
		return out, domain.Fail("evidence_unavailable", "The workflow has no complete selected subject.")
	}
	binding := out.Response
	producer, err := e.db.Attempt(ctx, binding.ProducerAttemptID)
	if err != nil {
		return out, err
	}
	if producer.TaskID != w.TaskID || producer.TaskVersion != w.TaskVersion || producer.WorkflowID != w.ID || producer.ResponseArtifactID == nil || *producer.ResponseArtifactID != binding.ResponseArtifactID || binding.ConditionsSHA256 != w.Authority.ConditionsSHA256 {
		return out, domain.Fail("candidate_stale", "The selected response no longer matches its original producer and conditions.")
	}
	verified, err := e.verifyWorkflowArtifacts(ctx, binding.ResponseArtifactID, binding.IndexArtifactID)
	if err != nil {
		return out, err
	}
	if len(verified) != 2 || verified[0].SHA256 != binding.ResponseSHA256 || verified[0].ByteLength != binding.ResponseByteLength || verified[1].SHA256 != binding.IndexSHA256 {
		return out, evidence.ErrCorrupt
	}
	source, err := e.db.SourceSnapshot(ctx, binding.SourceSnapshotID)
	if err != nil {
		return out, err
	}
	manifest, meta, err := e.workflowManifest(ctx, source.ManifestArtifactID)
	if err != nil || meta.SHA256 == nil || *meta.SHA256 != binding.ManifestSHA256 {
		return out, evidence.ErrCorrupt
	}
	if err := e.verifyWorkflowClosure(ctx, manifest); err != nil {
		return out, err
	}
	out.Source, out.RootSource, out.Manifest, out.RootManifest = source, source, manifest, manifest
	return out, nil
}

func (s workflowSubject) artifactIDs() []string {
	ids := []string{s.Source.ManifestArtifactID, s.Source.IndexArtifactID, s.RootSource.ManifestArtifactID, s.RootSource.IndexArtifactID}
	if s.Binding != nil {
		ids = append(ids, s.Binding.ManifestArtifactID, s.Binding.IndexArtifactID)
		if s.Candidate != nil {
			ids = append(ids, s.Candidate.DeltaArtifactID)
		}
	}
	if s.Response != nil {
		ids = append(ids, s.Response.ResponseArtifactID, s.Response.IndexArtifactID)
	}
	return ids
}

func (s workflowSubject) reviewSubject() domain.ReviewSubject {
	if s.Binding != nil {
		return domain.ReviewSubject{Kind: "candidate", ID: s.Binding.CandidateID, SHA256: s.Binding.ManifestSHA256}
	}
	return domain.ReviewSubject{Kind: "response", ID: s.Response.ResponseArtifactID, SHA256: s.Response.ResponseSHA256}
}

func workflowProofEntries(manifest workspace.Manifest) ([]workflowpolicy.ProofFile, []workflowpolicy.ProofIndex) {
	files := make([]workflowpolicy.ProofFile, 0, len(manifest.Entries))
	index := make([]workflowpolicy.ProofIndex, 0, len(manifest.Index))
	for _, entry := range manifest.Entries {
		files = append(files, workflowpolicy.ProofFile{Path: entry.Path, Type: entry.Type, Mode: entry.Mode, Size: entry.Size, SHA256: entry.SHA256, Target: entry.Target})
	}
	for _, entry := range manifest.Index {
		index = append(index, workflowpolicy.ProofIndex{Path: entry.Path, Mode: entry.Mode, Object: entry.Object})
	}
	return files, index
}

func (e *Engine) workflowDirectProof(ctx context.Context, s workflowSubject, original string, version domain.TaskVersion) *domain.Error {
	if invalid := e.graphDirectProofGuard(ctx, s.Workflow, original, version); invalid != nil {
		return invalid
	}
	op, recognized := workflowpolicy.RecognizeDocument(original, version)
	if !recognized || s.Binding == nil {
		return domain.Fail("evidence_unavailable", "This task does not have a supported complete direct proof.")
	}
	before, beforeIndex := workflowProofEntries(s.RootManifest)
	after, afterIndex := workflowProofEntries(s.Manifest)
	for _, group := range []struct {
		manifest workspace.Manifest
		files    []workflowpolicy.ProofFile
	}{{s.RootManifest, before}, {s.Manifest, after}} {
		for i, entry := range group.manifest.Entries {
			if entry.Path != op.Path {
				continue
			}
			var content bytes.Buffer
			meta, err := e.evidence.CopyTo(ctx, entry.ArtifactID, &content, 1<<20)
			if err != nil || meta.SHA256 == nil || *meta.SHA256 != entry.SHA256 {
				return domain.Fail("evidence_unavailable", "The exact document bytes are unavailable or changed.")
			}
			group.files[i].Bytes = content.Bytes()
		}
	}
	return workflowpolicy.ProveDocument(workflowpolicy.DocumentProofInput{Original: original, Version: version, Before: before, After: after, BeforeIndex: beforeIndex, AfterIndex: afterIndex})
}

func (s workflowSubject) snapshot(scope domain.DeliveryScope) *workspace.Snapshot {
	repository := scope.Destination
	if scope.SelectedRelativePath != "." {
		for path := filepath.FromSlash(scope.SelectedRelativePath); path != "."; path = filepath.Dir(path) {
			repository = filepath.Dir(repository)
		}
	}
	branch := ""
	if s.Source.Branch != nil {
		branch = *s.Source.Branch
	}
	return &workspace.Snapshot{ID: s.Source.ID, AttemptID: s.Source.AttemptID, ProjectPath: scope.Destination, RepositoryPath: repository, SelectedSubfolder: scope.SelectedRelativePath, BaseCommit: s.Source.BaseCommit, Branch: branch, ManifestArtifactID: s.Source.ManifestArtifactID, IndexArtifactID: s.Source.IndexArtifactID, Manifest: s.Manifest}
}

type candidateChangesCursor struct{ CandidateID, RootManifest, SelectedManifest, After string }

func (e *Engine) candidateChanges(ctx context.Context, id string, limit int, cursor string) (domain.Page[workspace.Change], error) {
	out := domain.Page[workspace.Change]{Items: []workspace.Change{}}
	candidate, err := e.db.Candidate(ctx, id)
	if err != nil {
		return out, err
	}
	root, err := e.db.SourceSnapshot(ctx, candidate.RootSourceSnapshotID)
	if err != nil {
		return out, err
	}
	before, beforeMeta, err := e.workflowManifest(ctx, root.ManifestArtifactID)
	if err != nil {
		return out, err
	}
	after, afterMeta, err := e.workflowManifest(ctx, candidate.ManifestArtifactID)
	if err != nil {
		return out, err
	}
	changes := workspace.CompareManifests(before, after)
	start := int64(0)
	if cursor != "" {
		var value candidateChangesCursor
		raw, decodeErr := base64.RawURLEncoding.DecodeString(cursor)
		if decodeErr != nil || len(cursor) > 2048 || base64.RawURLEncoding.EncodeToString(raw) != cursor || json.Unmarshal(raw, &value, json.RejectUnknownMembers(true)) != nil || value.CandidateID != id || value.RootManifest != *beforeMeta.SHA256 || value.SelectedManifest != *afterMeta.SHA256 {
			return out, domain.Fail("invalid_cursor", "Use the complete cursor for this retained candidate and basis.")
		}
		offset, parseErr := domain.ParseCount(value.After)
		if parseErr != nil || offset < 1 || offset >= int64(len(changes)) {
			return out, domain.Fail("invalid_cursor", "The candidate change cursor is outside this immutable result.")
		}
		start = offset
	}
	end := min(start+int64(limit), int64(len(changes)))
	out.Items = append(out.Items, changes[start:end]...)
	if end < int64(len(changes)) {
		raw, marshalErr := json.Marshal(candidateChangesCursor{CandidateID: id, RootManifest: *beforeMeta.SHA256, SelectedManifest: *afterMeta.SHA256, After: strconv.FormatInt(end, 10)})
		if marshalErr != nil {
			return out, domain.Fail("evidence_unavailable", "The retained change page cannot be encoded.")
		}
		next := base64.RawURLEncoding.EncodeToString(raw)
		out.NextCursor = &next
	}
	return out, nil
}
