package engine

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func (e *Engine) runGraphLocal(ctx context.Context, a domain.Attempt) {
	op, err := e.db.GraphInputOperationForAttempt(ctx, e.config.OwnerInstanceID, a.ID)
	if err != nil {
		e.fence(a.ID, "graph_operation_unavailable", "The owned graph preparation cannot inspect its exact retained operation. Preserve the operation before continuing.")
		return
	}
	facts := domain.GraphInputCompletionFacts{EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, State: "blocked"}
	guard := func() error {
		e.control.Lock()
		defer e.control.Unlock()
		if e.fenced {
			return domain.Fail("dispatch_fenced", "Resolve retained ownership or persistence uncertainty before local graph preparation.")
		}
		return e.db.CheckGraphLocalOperation(ctx, e.config.OwnerInstanceID, a.ID)
	}
	if err = guard(); err != nil {
		facts.Failure = domain.SafeError(err)
		e.finishGraphLocal(a, facts)
		return
	}
	if op.Kind == "node_input" {
		e.runGraphNodeInput(ctx, a, op)
		return
	}
	if op.Kind != "root_basis" {
		facts.Failure = domain.Fail("capability_unavailable", "This graph input preparation kind has no enabled local implementation.")
		e.finishGraphLocal(a, facts)
		return
	}
	saved, identity, err := e.db.Project(ctx, op.Definition.Original.ProjectID)
	if err != nil || identity != op.Definition.Delivery.ProjectIdentity {
		facts.Failure = domain.Fail("project_identity_changed", "The graph project no longer matches its captured source authority.")
		e.finishGraphLocal(a, facts)
		return
	}
	// The workspace manager records an intent before creating a managed source.
	// Its effect report is retained even when capture returns partial evidence.
	captured, captureErr := e.workspace.CaptureGraphRoot(ctx, workspace.GraphRootCaptureRequest{
		GraphID: op.GraphID, RootTaskID: op.RootTaskID, OperationAttemptID: a.ID,
		ProjectID: saved.ID, ProjectPath: saved.ResolvedPath,
		ProjectIdentity: identity, RepositoryIdentity: op.Definition.Delivery.RepositoryIdentity,
	})
	facts.OperationEffects, facts.EffectCount, facts.EffectsKnown, facts.EvidenceStatus = captured.OperationEffects, captured.EffectCount, captured.EffectsKnown, captured.EvidenceStatus
	if captureErr != nil {
		facts.State, facts.Failure = "uncertain", domain.Fail("source_capture_blocked", "Graph source capture did not retain a complete known closure. Inspect its retained local effects before continuing.")
		if facts.EffectsKnown {
			facts.State = "blocked"
		}
	} else if captured.Snapshot == nil {
		facts.State, facts.EffectsKnown, facts.EvidenceStatus = "uncertain", false, "uncertain"
		facts.Failure = domain.Fail("source_capture_blocked", "The source capture returned no retained managed snapshot.")
	} else if binding := captured.Binding; binding.GraphID != op.GraphID || binding.RootTaskID != op.RootTaskID || binding.OperationAttemptID != a.ID || binding.ProjectIdentity != op.CreateFacts.ProjectIdentity || binding.RepositoryIdentity != op.CreateFacts.RepositoryIdentity || binding.SelectedRelativePath != op.CreateFacts.SelectedRelativePath || binding.BaseCommit != op.CreateFacts.BaseCommit || !reflect.DeepEqual(binding.Branch, op.CreateFacts.Branch) {
		facts.State, facts.Failure = "blocked", domain.Fail("graph_source_changed", "The project source basis changed after this graph was admitted. Inspect the retained capture and create a new graph against the intended base.")
	} else {
		binding := captured.Binding
		facts.RootBinding = &binding
		facts.SourceSnapshot = &domain.SourceSnapshot{ID: binding.SourceSnapshotID, AttemptID: a.ID, ManifestArtifactID: binding.Manifest.ArtifactID, IndexArtifactID: binding.Index.ArtifactID, ManifestSHA256: binding.Manifest.SHA256, BaseCommit: binding.BaseCommit, Branch: binding.Branch, ProjectIdentity: binding.ProjectIdentity, SelectedRelativePath: binding.SelectedRelativePath, CreatedAt: domain.Now()}
		facts.VerifiedArtifacts, err = e.verifyGraphRootClosure(ctx, binding, captured.Snapshot)
		if err != nil {
			facts.RootBinding, facts.SourceSnapshot = nil, nil
			facts.State, facts.EvidenceStatus = "incomplete", "incomplete"
			facts.Failure = domain.Fail("evidence_corrupt", "The captured graph source closure failed fresh complete evidence verification.")
		} else {
			facts.State = "ready"
		}
	}
	// Settlement uses its own bounded context: cancellation must not erase known
	// effects, and a stopped caller cannot prevent the exact claim from settling.
	e.finishGraphLocal(a, facts)
}

func (e *Engine) verifyGraphRootClosure(ctx context.Context, root domain.GraphRootBinding, source *workspace.Snapshot) ([]domain.VerifiedResponse, error) {
	proof, err := e.workspace.VerifyGraphRoot(ctx, root, source)
	if err != nil {
		return nil, err
	}
	return e.verifyGraphClosureAnchors(ctx, proof.Artifacts, []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs})
}

func (e *Engine) finishGraphLocal(a domain.Attempt, facts domain.GraphInputCompletionFacts) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if facts.EffectCount == "" {
		facts.EffectCount = "0"
	}
	if facts.OperationEffects != nil {
		meta, err := e.verifyGraphArtifact(ctx, *facts.OperationEffects, "graph_operation_effects")
		if err == nil {
			facts.VerifiedArtifacts = append(facts.VerifiedArtifacts, domain.VerifiedResponse{ArtifactID: meta.ID, SHA256: *meta.SHA256, PhysicalIdentity: meta.Identity, ByteLength: meta.ByteLength})
		} else {
			facts.State, facts.EvidenceStatus, facts.EffectsKnown = "uncertain", "uncertain", false
			facts.Failure = domain.Fail("evidence_uncertain", "The local graph operation effects cannot be verified durably.")
		}
	}
	e.control.Lock()
	err := e.db.CompleteGraphInput(ctx, e.config.OwnerInstanceID, a.ID, facts)
	e.control.Unlock()
	if err != nil {
		var de *domain.Error
		if errors.As(err, &de) && de.Code == "write_uncertain" {
			e.fence(a.ID, "graph_completion_uncertain", "Graph preparation completion lost its durable acknowledgement. Inspect the retained operation; do not recapture it.")
			return
		}
		e.fence(a.ID, "graph_completion_failed", "The local graph preparation could not settle against its retained authority. Preserve its effects and inspect the operation.")
		return
	}
	if !facts.EffectsKnown || facts.EvidenceStatus == "uncertain" {
		e.fence(a.ID, "graph_effects_uncertain", "Graph preparation has unresolved local effects or evidence. No successor is authorized.")
		return
	}
	e.signal()
}
