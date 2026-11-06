package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"path/filepath"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type workflowNativeContext struct {
	subject     workflowSubject
	step        domain.WorkflowStep
	snapshot    *workspace.Snapshot
	supplied    []domain.ReviewInspection
	prior       []string
	paths       []string
	artifactIDs []string
}

func workflowHash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func (e *Engine) prepareWorkflowNative(ctx context.Context, a domain.Attempt, detail domain.TaskVersionDetail, outputPath string, resolve adapters.Resolver) (adapters.Launch, *workflowNativeContext, error) {
	var zero adapters.Launch
	selected, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil {
		return zero, nil, err
	}
	if selected.Workflow == nil || selected.Workflow.TaskVersion != a.TaskVersion {
		return zero, nil, domain.Fail("candidate_stale", "The admitted native stage has no matching workflow authority.")
	}
	w := *selected.Workflow
	step, err := e.db.WorkflowStep(ctx, a.StepID)
	if err != nil {
		return zero, nil, err
	}
	if step.AttemptID == nil || *step.AttemptID != a.ID || step.WorkflowID != w.ID || step.Role != a.Role || w.CurrentStepID == nil || *w.CurrentStepID != step.ID {
		return zero, nil, domain.Fail("candidate_stale", "The admitted native stage is no longer the current owned step.")
	}
	subject, err := e.selectedWorkflowSubject(ctx, w)
	if err != nil {
		return zero, nil, err
	}
	if subject.Binding != nil {
		if step.InputCandidateID == nil || *step.InputCandidateID != subject.Binding.CandidateID {
			return zero, nil, domain.Fail("candidate_stale", "The native stage input candidate changed.")
		}
	} else if step.InputResponseBinding == nil || *step.InputResponseBinding != *subject.Response {
		return zero, nil, domain.Fail("candidate_stale", "The native stage input response changed.")
	}
	manifestID, indexID := subject.Source.ManifestArtifactID, subject.ResponseIndexID()
	if subject.Binding != nil {
		manifestID = subject.Binding.ManifestArtifactID
	}
	snapshot, err := e.workspace.Handoff(ctx, workspace.HandoffRequest{AttemptID: a.ID, Source: subject.snapshot(w.Authority.Delivery), CandidateManifestArtifactID: manifestID, IndexArtifactID: indexID})
	if err != nil {
		return zero, nil, err
	}
	stage := &workflowNativeContext{subject: subject, step: step, snapshot: snapshot, supplied: []domain.ReviewInspection{}, prior: []string{}, paths: []string{}}
	for _, entry := range snapshot.Manifest.Entries {
		stage.paths = append(stage.paths, entry.Path)
	}
	evidenceID, scope, ref := snapshot.ManifestArtifactID, "whole_candidate", "selected-candidate-manifest"
	if subject.Response != nil {
		evidenceID, scope, ref = subject.Response.ResponseArtifactID, "whole_response", "selected-response"
	}
	meta, err := e.evidence.VerifyClosed(ctx, evidenceID)
	if err != nil {
		return zero, nil, err
	}
	provided := []adapters.SuppliedEvidence{{Ref: ref, Path: filepath.Join(e.lock.Root(), filepath.FromSlash(meta.RelativePath)), Scope: scope, Paths: []string{}}}
	stage.supplied = append(stage.supplied, domain.ReviewInspection{Ref: ref, Scope: scope, Paths: []string{}})
	stage.artifactIDs = append(stage.artifactIDs, meta.ID)
	findings := []domain.ReviewFinding{}
	origins := []domain.WorkflowFinding{}
	cursor := ""
	for {
		page, listErr := e.db.WorkflowFindings(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: w.ID}, 100, cursor)
		if listErr != nil {
			return zero, nil, listErr
		}
		for _, finding := range page.Items {
			if finding.Finding.Severity != "blocking" {
				continue
			}
			resolved := false
			for _, resolution := range finding.Resolutions {
				if workflowResolutionMatches(subject, resolution) {
					resolved = true
					break
				}
			}
			if resolved {
				continue
			}
			if len(findings) >= domain.MaxReviewFindings {
				return zero, nil, domain.Fail("input_too_large", "All unresolved findings exceed the bounded native context; none were truncated.")
			}
			findings = append(findings, finding.Finding)
			origins = append(origins, finding)
			stage.prior = append(stage.prior, finding.ID)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	provided, checks, err := e.workflowLinkedContext(ctx, stage, origins, provided)
	if err != nil {
		return zero, nil, err
	}
	schemaPath := ""
	if a.Role == "review" && a.ConnectionID == "codex-native" {
		schema, putErr := e.evidence.Put(ctx, a.ID, "review_schema", adapters.ReviewSchema())
		if putErr != nil {
			return zero, nil, putErr
		}
		schemaPath = filepath.Join(e.lock.Root(), filepath.FromSlash(schema.RelativePath))
	}
	launch, err := adapters.BuildRole(ctx, adapters.RoleInput{Role: a.Role, Original: detail.OriginalText, Version: detail.Version, ResolvedTarget: a.ResolvedTarget, Subject: subject.reviewSubject(), ConditionsSHA256: w.Authority.ConditionsSHA256, Evidence: provided, Checks: checks, PriorFindingIDs: stage.prior, Findings: findings, SchemaPath: schemaPath, OutputPath: outputPath}, resolve)
	return launch, stage, err
}

func (s workflowSubject) ResponseIndexID() string {
	if s.Binding != nil {
		return s.Binding.IndexArtifactID
	}
	return s.Response.IndexArtifactID
}

func (e *Engine) workflowApplicability(ctx context.Context, subject workflowSubject, scope domain.CheckScope, keys []string, definition string) (domain.Applicability, error) {
	index, err := e.evidence.VerifyClosed(ctx, subject.RootSource.IndexArtifactID)
	if err != nil || index.SHA256 == nil {
		return domain.Applicability{}, evidence.ErrCorrupt
	}
	control, err := json.Marshal(struct {
		BaseCommit                                                         string
		Branch                                                             *string
		ProjectIdentity, SelectedRelativePath, ManifestSHA256, IndexSHA256 string
	}{subject.RootSource.BaseCommit, subject.RootSource.Branch, subject.RootSource.ProjectIdentity, subject.RootSource.SelectedRelativePath, subject.RootSource.ManifestSHA256, *index.SHA256})
	if err != nil {
		return domain.Applicability{}, err
	}
	files, entries := workflowProofEntries(subject.Manifest)
	applicability, invalid := workflowpolicy.Basis(workflowpolicy.ApplicabilityInput{Scope: scope, ConditionKeys: keys, DefinitionSHA256: definition, ConditionsSHA256: subject.Workflow.Authority.ConditionsSHA256, SourceControlSHA256: workflowHash(control), Files: files, Index: entries, Response: subject.Response})
	if invalid != nil {
		return applicability, invalid
	}
	return applicability, nil
}

func (e *Engine) workflowNativeFacts(ctx context.Context, a domain.Attempt, stage *workflowNativeContext, settlement domain.Settlement) (domain.WorkflowStepFacts, error) {
	facts := domain.WorkflowStepFacts{StepID: a.StepID, OwnerInstanceID: e.config.OwnerInstanceID, Settlement: settlement}
	detail, err := e.db.Task(ctx, a.TaskID)
	if err != nil {
		return facts, err
	}
	view, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || view.Workflow == nil {
		return facts, domain.Fail("evidence_unavailable", "The native settlement cannot inspect its captured workflow.")
	}
	w := *view.Workflow
	facts.TaskRevision, facts.WorkflowRevision = detail.Task.Revision, w.Revision
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil {
		return facts, err
	}
	if current.PrimaryFailure != nil || current.Stop != nil || settlement.Outcome != "succeeded" || settlement.EvidenceStatus != "complete" || settlement.Cleanup.State != "verified" {
		return facts, nil
	}
	if current.CandidateID == nil {
		return facts, domain.Fail("evidence_unavailable", "The native result has no complete current source/index capture.")
	}
	candidate, err := e.db.Candidate(ctx, *current.CandidateID)
	if err != nil {
		return facts, err
	}
	if a.Role == "review" {
		if stage == nil || current.ResponseArtifactID == nil || candidate.HasDelta || candidate.HasIndexDelta {
			return facts, domain.Fail("review_unavailable", "Review did not preserve the actual supplied subject or retain its complete result.")
		}
		meta, err := e.evidence.VerifyClosed(ctx, *current.ResponseArtifactID)
		if err != nil || meta.SHA256 == nil {
			return facts, evidence.ErrCorrupt
		}
		raw, err := e.readResponse(ctx, meta)
		if err != nil {
			return facts, err
		}
		report, invalid := workflowpolicy.ValidateReview(raw, stage.subject.reviewSubject(), w.Authority.ConditionsSHA256, w.Authority.Conditions, stage.supplied, stage.prior, stage.paths)
		if invalid != nil {
			return facts, invalid
		}
		scope := "whole_candidate"
		if stage.subject.Response != nil {
			scope = "whole_response"
		}
		keys := make([]string, 0, len(w.Authority.Conditions))
		for _, condition := range w.Authority.Conditions {
			keys = append(keys, condition.Key)
		}
		applicability, err := e.workflowApplicability(ctx, stage.subject, domain.CheckScope{Kind: scope, Paths: []string{}}, keys, workflowHash(adapters.ReviewSchema()))
		if err != nil {
			return facts, err
		}
		facts.CandidateBinding, facts.ResponseBinding, facts.Applicability = stage.subject.Binding, stage.subject.Response, &applicability
		facts.HasTaskDelta, facts.HasTaskIndexDelta = workflowChangeFlags(workspace.CompareManifests(stage.subject.RootManifest, stage.subject.Manifest))
		facts.DirectProven = e.workflowDirectProof(ctx, stage.subject, detail.Task.OriginalText, detail.Version) == nil
		record := domain.ReviewRecord{ID: domain.NewID(), WorkflowID: w.ID, StepID: a.StepID, AttemptID: a.ID, CandidateBinding: facts.CandidateBinding, ResponseBinding: facts.ResponseBinding, ReviewContract: domain.ReviewContract, ResultArtifactID: meta.ID, ResultSHA256: *meta.SHA256, Applicability: applicability, Report: report, SuppliedEvidence: stage.supplied, Findings: []domain.WorkflowFinding{}, Outcome: report.Outcome, CreatedAt: domain.Now()}
		for _, finding := range report.Findings {
			record.Findings = append(record.Findings, domain.WorkflowFinding{ID: domain.NewID(), ModelKey: finding.Key, ReviewID: record.ID, Finding: finding})
		}
		facts.Report = &record
		allowed := map[string]bool{}
		for _, finding := range report.Findings {
			for _, location := range finding.Locations {
				if !allowed[location.Path] {
					allowed[location.Path] = true
					facts.ReviewAllowedPaths = append(facts.ReviewAllowedPaths, location.Path)
				}
			}
		}
		var reuseIDs []string
		facts.ReuseEvidenceUses, _, _, reuseIDs, err = e.currentWorkflowEvidence(ctx, stage.subject)
		if err != nil {
			return facts, err
		}
		ids := append(stage.subject.artifactIDs(), meta.ID)
		ids = append(ids, reuseIDs...)
		ids = append(ids, stage.artifactIDs...)
		facts.VerifiedArtifacts, err = e.verifyWorkflowArtifacts(ctx, ids...)
		return facts, err
	}
	subject, err := e.candidateWorkflowSubject(ctx, w, candidate.ID)
	if err != nil {
		return facts, err
	}
	facts.CandidateBinding = subject.Binding
	facts.HasTaskDelta, facts.HasTaskIndexDelta = workflowChangeFlags(workspace.CompareManifests(subject.RootManifest, subject.Manifest))
	facts.DirectProven = e.workflowDirectProof(ctx, subject, detail.Task.OriginalText, detail.Version) == nil
	ids := subject.artifactIDs()
	if stage != nil {
		ids = append(ids, stage.artifactIDs...)
	}
	if detail.Version.Acceptance.Deliverable == "response_only" && !facts.HasTaskDelta && !facts.HasTaskIndexDelta {
		if current.ResponseArtifactID == nil {
			return facts, domain.Fail("evidence_unavailable", "The response-only producer retained no complete response.")
		}
		meta, verifyErr := e.evidence.VerifyClosed(ctx, *current.ResponseArtifactID)
		if verifyErr != nil || meta.SHA256 == nil {
			return facts, evidence.ErrCorrupt
		}
		facts.CandidateBinding = nil
		facts.ResponseBinding = &domain.ResponseBinding{ProducerAttemptID: a.ID, ResponseArtifactID: meta.ID, ResponseSHA256: *meta.SHA256, ResponseByteLength: meta.ByteLength, SourceSnapshotID: subject.Source.ID, ManifestSHA256: subject.Source.ManifestSHA256, IndexArtifactID: subject.Binding.IndexArtifactID, IndexSHA256: subject.Binding.IndexSHA256, ConditionsSHA256: w.Authority.ConditionsSHA256}
		ids = append(ids, meta.ID)
	}
	if facts.ResponseBinding != nil {
		subject.Response, subject.Binding = facts.ResponseBinding, nil
	}
	var reuseIDs []string
	facts.ReuseEvidenceUses, _, _, reuseIDs, err = e.currentWorkflowEvidence(ctx, subject)
	if err != nil {
		return facts, err
	}
	ids = append(ids, reuseIDs...)
	facts.VerifiedArtifacts, err = e.verifyWorkflowArtifacts(ctx, ids...)
	return facts, err
}

// Called with control held only after all evidence I/O has finished.
func (e *Engine) settleExecutionLocked(ctx context.Context, a domain.Attempt, settlement domain.Settlement, facts domain.WorkflowStepFacts) error {
	if a.WorkflowID == "" {
		_, err := e.db.SettleAttempt(ctx, a.ID, settlement)
		return err
	}
	facts.StepID, facts.OwnerInstanceID, facts.Settlement = a.StepID, e.config.OwnerInstanceID, settlement
	_, err := e.db.SettleWorkflowStep(ctx, a.ID, facts)
	return err
}
