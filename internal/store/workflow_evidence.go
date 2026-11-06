package store

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"reflect"
)

func evidenceUnavailable() *domain.Error {
	return domain.Fail("evidence_unavailable", "The complete fresh evidence does not match its immutable registered workflow subject.")
}

func (s *Store) verifyWorkflowProofs(ctx context.Context, q queryer, proofs []domain.VerifiedResponse, ids ...string) error {
	if len(proofs) > 256 {
		return domain.Invalid("bounded workflow artifact proofs")
	}
	known := map[string]domain.VerifiedResponse{}
	for _, p := range proofs {
		if previous, ok := known[p.ArtifactID]; ok && previous != p {
			return evidenceUnavailable()
		}
		known[p.ArtifactID] = p
	}
	for _, id := range ids {
		a, e := s.artifact(ctx, q, id)
		if e != nil {
			return e
		}
		p, ok := known[id]
		if !ok || !a.Closed || a.Completeness != "complete" || a.SHA256 == nil || p.SHA256 != *a.SHA256 || p.PhysicalIdentity != a.PhysicalIdentity || p.ByteLength != a.ByteLength {
			return evidenceUnavailable()
		}
	}
	return nil
}
func (s *Store) candidateBinding(ctx context.Context, q queryer, w domain.Workflow, b domain.CandidateBinding, proofs []domain.VerifiedResponse) error {
	c, e := s.candidate(ctx, q, b.CandidateID)
	if e != nil {
		return e
	}
	if c.WorkflowID != w.ID || c.Completeness != "complete" || c.ConditionsSHA256 != w.Authority.ConditionsSHA256 || b.ConditionsSHA256 != w.Authority.ConditionsSHA256 || c.RootSourceSnapshotID != b.RootSourceSnapshotID || c.SourceSnapshotID != b.SourceSnapshotID || c.ManifestArtifactID != b.ManifestArtifactID || c.IndexArtifactID != b.IndexArtifactID {
		return evidenceUnavailable()
	}
	source, e := s.sourceSnapshot(ctx, q, c.SourceSnapshotID)
	if e != nil {
		return e
	}
	root, e := s.sourceSnapshot(ctx, q, c.RootSourceSnapshotID)
	if e != nil {
		return e
	}
	for _, source := range []domain.SourceSnapshot{source, root} {
		a, e := s.attempt(ctx, q, source.AttemptID)
		if e != nil {
			return e
		}
		if a.WorkflowID != w.ID || a.TaskID != w.TaskID || a.TaskVersion != w.TaskVersion {
			return evidenceUnavailable()
		}
	}
	manifest, e := s.artifact(ctx, q, b.ManifestArtifactID)
	if e != nil {
		return e
	}
	index, e := s.artifact(ctx, q, b.IndexArtifactID)
	if e != nil {
		return e
	}
	if manifest.SHA256 == nil || *manifest.SHA256 != b.ManifestSHA256 || index.SHA256 == nil || *index.SHA256 != b.IndexSHA256 {
		return evidenceUnavailable()
	}
	return s.verifyWorkflowProofs(ctx, q, proofs, c.ManifestArtifactID, c.IndexArtifactID, source.ManifestArtifactID, source.IndexArtifactID, root.ManifestArtifactID, root.IndexArtifactID)
}
func (s *Store) responseBinding(ctx context.Context, q queryer, w domain.Workflow, b domain.ResponseBinding, proofs []domain.VerifiedResponse) error {
	a, e := s.attempt(ctx, q, b.ProducerAttemptID)
	if e != nil {
		return e
	}
	if a.WorkflowID != w.ID || a.TaskVersion != w.TaskVersion || (a.Role != "author" && a.Role != "correction") || a.ResponseArtifactID == nil || *a.ResponseArtifactID != b.ResponseArtifactID || a.SourceSnapshotID == nil || *a.SourceSnapshotID != b.SourceSnapshotID || a.CandidateID == nil || b.ConditionsSHA256 != w.Authority.ConditionsSHA256 {
		return evidenceUnavailable()
	}
	c, e := s.candidate(ctx, q, *a.CandidateID)
	if e != nil {
		return e
	}
	source, e := s.sourceSnapshot(ctx, q, b.SourceSnapshotID)
	if e != nil {
		return e
	}
	if c.IndexArtifactID != b.IndexArtifactID || source.ManifestSHA256 != b.ManifestSHA256 {
		return evidenceUnavailable()
	}
	artifact, e := s.artifact(ctx, q, b.ResponseArtifactID)
	if e != nil {
		return e
	}
	index, e := s.artifact(ctx, q, b.IndexArtifactID)
	if e != nil {
		return e
	}
	if artifact.Kind != "final_response" || artifact.AttemptID != a.ID || artifact.SHA256 == nil || *artifact.SHA256 != b.ResponseSHA256 || artifact.ByteLength != b.ResponseByteLength || index.SHA256 == nil || *index.SHA256 != b.IndexSHA256 {
		return evidenceUnavailable()
	}
	root, e := s.sourceSnapshot(ctx, q, c.RootSourceSnapshotID)
	if e != nil {
		return e
	}
	return s.verifyWorkflowProofs(ctx, q, proofs, b.ResponseArtifactID, b.IndexArtifactID, source.ManifestArtifactID, source.IndexArtifactID, root.ManifestArtifactID, root.IndexArtifactID)
}
func selectedUse(w domain.Workflow, kind, id string, app domain.Applicability) domain.EvidenceUse {
	use := domain.EvidenceUse{Kind: kind, EvidenceID: id, Applicability: app}
	if w.SelectedCandidateID != nil {
		use.CandidateID = *w.SelectedCandidateID
	}
	if w.SelectedResponseBinding != nil {
		use.ResponseArtifactID = w.SelectedResponseBinding.ResponseArtifactID
	}
	return use
}
func useSubjectMatches(w domain.Workflow, u domain.EvidenceUse) bool {
	return w.SelectedCandidateID != nil && u.CandidateID == *w.SelectedCandidateID && u.ResponseArtifactID == "" || w.SelectedResponseBinding != nil && u.ResponseArtifactID == w.SelectedResponseBinding.ResponseArtifactID && u.CandidateID == ""
}
func workflowDefinition(w domain.Workflow, id string) (domain.CheckDefinition, error) {
	for _, d := range w.Authority.Checks {
		if d.ID == id {
			return d, nil
		}
	}
	return domain.CheckDefinition{}, evidenceUnavailable()
}
func (s *Store) workflowUses(ctx context.Context, q queryer, w domain.Workflow, uses []domain.EvidenceUse, proofs []domain.VerifiedResponse) (*domain.ReviewRecord, []domain.CheckReceipt, error) {
	if len(uses) > domain.MaxWorkflowChecks+1 {
		return nil, nil, evidenceUnavailable()
	}
	var review *domain.ReviewRecord
	checks := []domain.CheckReceipt{}
	seen := map[string]bool{}
	for _, u := range uses {
		if !useSubjectMatches(w, u) || seen[u.EvidenceID] {
			return nil, nil, evidenceUnavailable()
		}
		seen[u.EvidenceID] = true
		switch u.Kind {
		case "review":
			r, e := storedWorkflowRecord[domain.ReviewRecord](ctx, q, "workflow_reviews", u.EvidenceID)
			if e != nil {
				return nil, nil, e
			}
			if review != nil || r.WorkflowID != w.ID || r.ReviewContract != domain.ReviewContract || !workflowpolicy.Applicable(r.Applicability, u.Applicability) {
				return nil, nil, evidenceUnavailable()
			}
			if e = s.verifyWorkflowProofs(ctx, q, proofs, r.ResultArtifactID); e != nil {
				return nil, nil, e
			}
			review = &r
		case "check":
			r, e := storedWorkflowRecord[domain.CheckReceipt](ctx, q, "workflow_checks", u.EvidenceID)
			if e != nil {
				return nil, nil, e
			}
			d, e := workflowDefinition(w, r.DefinitionID)
			if e != nil || r.WorkflowID != w.ID || r.DefinitionSHA256 != d.DefinitionSHA256 || (r.Outcome != "passed" && r.Outcome != "failed") || r.SourceChanged || r.Cleanup.State != "verified" || d.Kind == "builtin" && !r.BuiltinCompleted || d.Kind == "command" && (!r.RootTerminal || !r.InputComplete || !r.OutputComplete || r.ExitCode == nil) || !workflowpolicy.Applicable(r.Applicability, u.Applicability) {
				return nil, nil, evidenceUnavailable()
			}
			ids := []string{}
			if r.StdoutArtifactID != nil {
				ids = append(ids, *r.StdoutArtifactID)
			}
			if r.StderrArtifactID != nil {
				ids = append(ids, *r.StderrArtifactID)
			}
			if e = s.verifyWorkflowProofs(ctx, q, proofs, ids...); e != nil {
				return nil, nil, e
			}
			for _, old := range checks {
				if old.DefinitionID == r.DefinitionID {
					return nil, nil, evidenceUnavailable()
				}
			}
			checks = append(checks, r)
		default:
			return nil, nil, evidenceUnavailable()
		}
	}
	return review, checks, nil
}
func (s *Store) retainWorkflowReview(ctx context.Context, tx *sql.Tx, w domain.Workflow, step *domain.WorkflowStep, a domain.Attempt, f domain.WorkflowStepFacts) (domain.EvidenceUse, error) {
	r := *f.Report
	if !domain.ValidID(r.ID) || r.WorkflowID != w.ID || r.StepID != step.ID || r.AttemptID != a.ID || r.ReviewContract != domain.ReviewContract || r.Outcome != r.Report.Outcome || !reflect.DeepEqual(r.CandidateBinding, f.CandidateBinding) || !reflect.DeepEqual(r.ResponseBinding, f.ResponseBinding) || f.Applicability == nil || !reflect.DeepEqual(r.Applicability, *f.Applicability) {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	subject := domain.ReviewSubject{}
	if r.CandidateBinding != nil {
		subject = domain.ReviewSubject{Kind: "candidate", ID: r.CandidateBinding.CandidateID, SHA256: r.CandidateBinding.ManifestSHA256}
	} else if r.ResponseBinding != nil {
		subject = domain.ReviewSubject{Kind: "response", ID: r.ResponseBinding.ResponseArtifactID, SHA256: r.ResponseBinding.ResponseSHA256}
	} else {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	artifact, e := s.artifact(ctx, tx, r.ResultArtifactID)
	if e != nil {
		return domain.EvidenceUse{}, e
	}
	if artifact.AttemptID != a.ID || artifact.Kind != "review_result" || artifact.SHA256 == nil || *artifact.SHA256 != r.ResultSHA256 {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	if e = s.verifyWorkflowProofs(ctx, tx, f.VerifiedArtifacts, r.ResultArtifactID); e != nil {
		return domain.EvidenceUse{}, e
	}
	prior, e := s.workflowFindingIDs(ctx, tx, w.ID)
	if e != nil {
		return domain.EvidenceUse{}, e
	}
	raw, e := json.Marshal(r.Report)
	if e != nil {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	validated, invalid := workflowpolicy.ValidateReview(raw, subject, w.Authority.ConditionsSHA256, w.Authority.Conditions, r.SuppliedEvidence, prior, f.ReviewAllowedPaths)
	if invalid != nil {
		return domain.EvidenceUse{}, invalid
	}
	if !reflect.DeepEqual(validated, r.Report) || len(r.Findings) != len(r.Report.Findings) {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	for i := range r.Findings {
		finding := &r.Findings[i]
		if !domain.ValidID(finding.ID) || finding.ReviewID != r.ID || finding.CheckID != "" || finding.ModelKey != r.Report.Findings[i].Key || !reflect.DeepEqual(finding.Finding, r.Report.Findings[i]) || len(finding.Resolutions) != 0 {
			return domain.EvidenceUse{}, evidenceUnavailable()
		}
	}
	seq, e := nextSequence(ctx, tx, "workflow_reviews")
	if e != nil {
		return domain.EvidenceUse{}, e
	}
	var candidate, response *string
	if r.CandidateBinding != nil {
		candidate = ptr(r.CandidateBinding.CandidateID)
	} else {
		response = ptr(r.ResponseBinding.ResponseArtifactID)
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO workflow_reviews VALUES(?,?,?,?,?,?,?,?,?)", seq, r.ID, w.ID, step.ID, a.ID, candidate, response, r.ResultArtifactID, encode(r)); e != nil {
		return domain.EvidenceUse{}, storageError()
	}
	step.ReviewID = ptr(r.ID)
	return selectedUse(w, "review", r.ID, r.Applicability), nil
}

// One correction cycle bounds the complete finding set; no observer page is used
// as an authority shortcut. Oversized stored facts are rejected rather than clipped.
func (s *Store) workflowFindingIDs(ctx context.Context, q queryer, workflowID string) ([]string, error) {
	findings, err := s.workflowAllFindings(ctx, q, workflowID)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, finding := range findings {
		out = append(out, finding.ID)
	}
	return out, nil
}
