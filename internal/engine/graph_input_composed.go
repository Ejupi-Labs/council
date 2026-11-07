package engine

import (
	"context"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func (e *Engine) runGraphComposedInput(ctx context.Context, a domain.Attempt, op domain.GraphInputOperation, receiver domain.TaskVersionDetail, rootSource *workspace.Snapshot) {
	facts := domain.GraphInputCompletionFacts{State: "blocked", EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}}
	operation := graphInputEffects{Contract: "006-input-retention-effects-v1", InputID: op.ID, NodeID: *op.Control.NodeID, TaskVersion: *op.Control.TaskVersion, State: "in_flight", EffectsKnown: false}
	intent, err := e.retainGraphInputEffect(ctx, a.ID, operation)
	if err != nil {
		facts.State, facts.EffectsKnown, facts.EvidenceStatus = "uncertain", false, "uncertain"
		facts.Failure = domain.Fail("evidence_uncertain", "The dependent input operation intent lost its durable acknowledgement.")
		e.finishGraphLocal(a, facts)
		return
	}
	facts.OperationEffects, facts.EffectsKnown = &intent, false
	retained, err := e.retainGraphReceiverContext(ctx, op, receiver)
	if err != nil {
		facts.State, facts.EvidenceStatus = "uncertain", "uncertain"
		facts.Failure = domain.Fail("evidence_uncertain", "Complete dependent receiver context could not be retained with known effects.")
		e.finishGraphLocal(a, facts)
		return
	}
	sources := make([]workspace.GraphContributionSource, 0, len(op.Contributions))
	for _, contribution := range op.Contributions {
		input, readErr := e.db.GraphInputRecord(ctx, op.GraphID, contribution.InputID)
		if readErr != nil || input.Public.InputBinding == nil {
			facts.State, facts.EvidenceStatus = "uncertain", "uncertain"
			facts.Failure = domain.Fail("graph_input_stale", "A captured contributing input could not be read after receiver context retention.")
			e.finishGraphLocal(a, facts)
			return
		}
		sources = append(sources, workspace.GraphContributionSource{Binding: contribution, InputBinding: *input.Public.InputBinding})
	}
	composition, composeErr := e.workspace.RetainNativeGraphInput(ctx, workspace.GraphNativeInputRetainRequest{
		Base: workspace.GraphInputRetainRequest{GraphID: op.GraphID, GraphVersion: op.Control.GraphVersion, RootTaskID: op.RootTaskID, OperationAttemptID: a.ID,
			InputID: op.ID, ReceivingNodeID: *op.Control.NodeID, ReceivingTaskID: receiver.TaskID, ReceivingTaskVersion: receiver.Version.Version,
			RootBinding: *op.RootBinding, RootSource: rootSource, Context: retained.Request}, Contributions: sources})
	all := append(append([]domain.GraphArtifactRef{}, retained.ArtifactRefs...), composition.Artifacts...)
	operation.CompositionEffects = composition.OperationEffects
	facts.EffectCount, facts.EffectsKnown, facts.EvidenceStatus = composition.EffectCount, composition.EffectsKnown, composition.EvidenceStatus
	facts.ConflictArtifact = composition.ConflictArtifact
	if composeErr != nil || composition.State != "complete" || composition.Binding == nil || composition.Snapshot == nil {
		facts.State = "blocked"
		if composition.State == "conflicted" {
			facts.State = "conflicted"
		}
		if !facts.EffectsKnown || facts.EvidenceStatus == "uncertain" {
			facts.State, facts.EvidenceStatus = "uncertain", "uncertain"
		}
		facts.Failure = domain.Fail("graph_input_unavailable", "The selected predecessor results did not produce a complete usable input. Inspect the retained composition or conflict evidence.")
		e.finishGraphComposedInput(ctx, a, operation, facts, all)
		return
	}
	binding, source := *composition.Binding, composition.Snapshot
	verified, err := e.workspace.VerifyGraphInput(ctx, binding)
	if err != nil {
		facts.State, facts.EvidenceStatus = "incomplete", "incomplete"
		if uncertainEvidence(err) {
			facts.State, facts.EvidenceStatus = "uncertain", "uncertain"
		}
		facts.Failure = domain.Fail("evidence_unavailable", "The completed dependent input closure could not be freshly verified.")
		e.finishGraphComposedInput(ctx, a, operation, facts, all)
		return
	}
	all = append(all, verified.ArtifactRefs...)
	meta, err := e.evidence.VerifyClosed(ctx, source.ManifestArtifactID)
	if err != nil || meta.SHA256 == nil || source.AttemptID != a.ID || source.IndexArtifactID != binding.Index.ArtifactID {
		facts.State, facts.EvidenceStatus = "uncertain", "uncertain"
		facts.Failure = domain.Fail("evidence_uncertain", "The actual assembly source could not be associated with its exact retained input index.")
		e.finishGraphComposedInput(ctx, a, operation, facts, all)
		return
	}
	facts.SourceSnapshot = &domain.SourceSnapshot{ID: source.ID, AttemptID: a.ID, ManifestArtifactID: source.ManifestArtifactID, IndexArtifactID: source.IndexArtifactID,
		ManifestSHA256: *meta.SHA256, BaseCommit: source.BaseCommit, Branch: op.RootBinding.Branch, ProjectIdentity: op.RootBinding.ProjectIdentity,
		SelectedRelativePath: source.SelectedSubfolder, CreatedAt: domain.Now()}
	facts.InputBinding, facts.ConditionsBinding, facts.ConditionsRef, facts.ContextPacket = &binding, &retained.Conditions, &retained.ConditionsRef, &verified.ContextPacket
	facts.State, facts.EffectsKnown, facts.EvidenceStatus = "ready", true, "complete"
	operation.Input, operation.ConditionsRef = &binding, &retained.ConditionsRef
	e.finishGraphComposedInput(ctx, a, operation, facts, all)
}

func (e *Engine) finishGraphComposedInput(parent context.Context, a domain.Attempt, operation graphInputEffects, facts domain.GraphInputCompletionFacts, all []domain.GraphArtifactRef) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 15*time.Second)
	defer cancel()
	operation.State, operation.EffectsKnown = facts.State, facts.EffectsKnown
	terminal, err := e.retainGraphInputEffect(ctx, a.ID, operation)
	if err != nil {
		facts.State, facts.EvidenceStatus, facts.EffectsKnown = "uncertain", "uncertain", false
		facts.InputBinding, facts.SourceSnapshot = nil, nil
		facts.ConditionsRef, facts.ConditionsBinding, facts.ContextPacket, facts.ConflictArtifact = nil, nil, nil, nil
		facts.Failure = domain.Fail("evidence_uncertain", "The dependent input operation lost its terminal effects acknowledgement.")
	} else {
		facts.OperationEffects = &terminal
		anchors := []domain.GraphArtifactRef{terminal}
		if operation.CompositionEffects != nil {
			anchors = append(anchors, *operation.CompositionEffects)
		}
		if facts.ConflictArtifact != nil {
			anchors = append(anchors, *facts.ConflictArtifact)
		}
		if facts.InputBinding != nil {
			b := facts.InputBinding
			anchors = append(anchors, b.Manifest, b.Index, b.StagedBlobs, b.Provenance, b.ContextPacket)
		}
		if facts.ConditionsRef != nil {
			anchors = append(anchors, *facts.ConditionsRef)
		}
		if facts.SourceSnapshot != nil {
			for _, id := range []string{facts.SourceSnapshot.ManifestArtifactID, facts.SourceSnapshot.IndexArtifactID} {
				meta, verifyErr := e.evidence.VerifyClosed(ctx, id)
				if verifyErr != nil || meta.SHA256 == nil {
					err = evidence.ErrCorrupt
					break
				}
				anchors = append(anchors, domain.GraphArtifactRef{ArtifactID: meta.ID, ProducerAttemptID: meta.AttemptID, SHA256: *meta.SHA256, ByteLength: meta.ByteLength})
			}
		}
		if err == nil {
			facts.VerifiedArtifacts, err = e.verifyGraphClosureAnchors(ctx, append(all, anchors...), anchors)
		}
		if err != nil {
			facts.State, facts.EvidenceStatus = "uncertain", "uncertain"
			facts.InputBinding, facts.SourceSnapshot = nil, nil
			facts.ConditionsRef, facts.ConditionsBinding, facts.ContextPacket, facts.ConflictArtifact = nil, nil, nil, nil
			facts.Failure = domain.Fail("evidence_uncertain", "Dependent composition effects and input/source references could not be freshly acknowledged.")
		}
	}
	e.finishGraphLocal(a, facts)
}
