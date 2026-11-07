package engine

import (
	"context"
	json "encoding/json/v2"
	"path/filepath"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type graphInputEffects struct {
	Contract           string                    `json:"contract"`
	InputID            string                    `json:"inputId"`
	NodeID             string                    `json:"nodeId"`
	TaskVersion        string                    `json:"taskVersion"`
	State              string                    `json:"state"`
	EffectsKnown       bool                      `json:"effectsKnown"`
	Input              *domain.GraphInputBinding `json:"input"`
	ConditionsRef      *domain.GraphArtifactRef  `json:"conditionsRef"`
	CompositionEffects *domain.GraphArtifactRef  `json:"compositionEffects,omitzero"`
}

func (e *Engine) retainGraphInputEffect(ctx context.Context, attemptID string, record graphInputEffects) (domain.GraphArtifactRef, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return domain.GraphArtifactRef{}, err
	}
	meta, err := e.evidence.Put(ctx, attemptID, "graph_operation_effects", body)
	if err != nil {
		return domain.GraphArtifactRef{}, err
	}
	if meta.SHA256 == nil {
		return domain.GraphArtifactRef{}, evidence.ErrCorrupt
	}
	return domain.GraphArtifactRef{ArtifactID: meta.ID, ProducerAttemptID: meta.AttemptID, SHA256: *meta.SHA256, ByteLength: meta.ByteLength}, nil
}

func (e *Engine) runGraphNodeInput(ctx context.Context, a domain.Attempt, op domain.GraphInputOperation) {
	facts := domain.GraphInputCompletionFacts{State: "blocked", EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}}
	if op.RootBinding == nil || op.Control.TaskID == nil || op.Control.TaskVersion == nil || op.Control.NodeID == nil {
		facts.Failure = domain.Fail("graph_input_stale", "This node preparation has no complete captured root and receiver authority.")
		e.finishGraphLocal(a, facts)
		return
	}
	receiver, err := e.db.TaskVersion(ctx, *op.Control.TaskID, *op.Control.TaskVersion)
	if err != nil {
		facts.Failure = domain.Fail("graph_input_stale", "The exact receiving task version is unavailable.")
		e.finishGraphLocal(a, facts)
		return
	}
	rootSource, err := e.graphRootSourceSnapshot(ctx, *op.RootBinding, op.Definition.Delivery)
	if err != nil {
		facts.Failure = domain.Fail("evidence_unavailable", "The retained graph root cannot be read faithfully.")
		e.finishGraphLocal(a, facts)
		return
	}
	rootProof, rootErr := e.workspace.VerifyGraphRoot(ctx, *op.RootBinding, rootSource)
	if rootErr != nil {
		facts.Failure = domain.Fail("graph_source_unavailable", "The retained graph root no longer has complete current source access. Restore that source before continuing.")
		e.finishGraphLocal(a, facts)
		return
	}
	e.control.Lock()
	guardErr := e.db.CheckGraphLocalOperation(ctx, e.config.OwnerInstanceID, a.ID)
	if e.fenced && guardErr == nil {
		guardErr = domain.Fail("dispatch_fenced", "Resolve local ownership before preparing graph input.")
	}
	e.control.Unlock()
	if guardErr != nil {
		facts.Failure = domain.SafeError(guardErr)
		e.finishGraphLocal(a, facts)
		return
	}
	if len(op.Contributions) != 0 || len(op.ContributorVector) != 0 {
		e.runGraphComposedInput(ctx, a, op, receiver, rootSource)
		return
	}
	operation := graphInputEffects{Contract: "006-input-retention-effects-v1", InputID: op.ID, NodeID: *op.Control.NodeID, TaskVersion: *op.Control.TaskVersion, State: "in_flight", EffectsKnown: false}
	intent, err := e.retainGraphInputEffect(ctx, a.ID, operation)
	if err != nil {
		facts.State, facts.EvidenceStatus, facts.EffectsKnown = "uncertain", "uncertain", false
		facts.Failure = domain.Fail("evidence_uncertain", "The graph input retention intent was not acknowledged durably.")
		e.finishGraphLocal(a, facts)
		return
	}
	facts.OperationEffects, facts.EffectsKnown = &intent, false
	retained, retainErr := e.retainGraphReceiverContext(ctx, op, receiver)
	var input domain.GraphInputBinding
	var verified workspace.VerifiedGraphInput
	if retainErr == nil {
		input, retainErr = e.workspace.RetainGraphInput(ctx, workspace.GraphInputRetainRequest{GraphID: op.GraphID, GraphVersion: op.Control.GraphVersion, RootTaskID: op.RootTaskID, OperationAttemptID: a.ID, InputID: op.ID, ReceivingNodeID: *op.Control.NodeID, ReceivingTaskID: receiver.TaskID, ReceivingTaskVersion: receiver.Version.Version, RootBinding: *op.RootBinding, RootSource: rootSource, Context: retained.Request})
	}
	if retainErr == nil {
		verified, retainErr = e.workspace.VerifyGraphInput(ctx, input)
	}
	if retainErr != nil {
		// The opaque retention boundary may have written evidence before a lost
		// acknowledgement. Keep its acknowledged intent; never replay that I/O.
		facts.State, facts.EvidenceStatus = "uncertain", "uncertain"
		facts.Failure = domain.Fail("evidence_uncertain", "Graph input retention did not produce a complete known closure. Inspect its retained operation before continuing.")
		e.finishGraphLocal(a, facts)
		return
	}
	operation.State, operation.EffectsKnown, operation.Input, operation.ConditionsRef = "complete", true, &input, &retained.ConditionsRef
	completed, err := e.retainGraphInputEffect(ctx, a.ID, operation)
	if err != nil {
		facts.State, facts.EvidenceStatus = "uncertain", "uncertain"
		facts.Failure = domain.Fail("evidence_uncertain", "The complete graph input operation lost its durable acknowledgement.")
	} else {
		facts.OperationEffects, facts.EffectCount, facts.EffectsKnown = &completed, "1", true
		anchors := []domain.GraphArtifactRef{input.Manifest, input.Index, input.StagedBlobs, input.Provenance, input.ContextPacket, retained.ConditionsRef, completed}
		all := append(append(append(append([]domain.GraphArtifactRef{}, rootProof.Artifacts...), verified.ArtifactRefs...), retained.ArtifactRefs...), anchors...)
		facts.VerifiedArtifacts, err = e.verifyGraphClosureAnchors(ctx, all, anchors)
		if err != nil {
			facts.State, facts.EvidenceStatus = "uncertain", "uncertain"
			facts.Failure = domain.Fail("evidence_uncertain", "The retained graph input payload changed before its final effects acknowledgement could be verified.")
		} else {
			facts.InputBinding, facts.ConditionsRef, facts.ContextPacket = &input, &retained.ConditionsRef, &verified.ContextPacket
			facts.ConditionsBinding = &retained.Conditions
			facts.State = "ready"
		}
	}
	e.finishGraphLocal(a, facts)
}

func (e *Engine) graphRootSourceSnapshot(ctx context.Context, root domain.GraphRootBinding, scope domain.DeliveryScope) (*workspace.Snapshot, error) {
	source, err := e.db.SourceSnapshot(ctx, root.SourceSnapshotID)
	if err != nil {
		return nil, err
	}
	if source.AttemptID != root.OperationAttemptID || source.ManifestArtifactID != root.Manifest.ArtifactID || source.IndexArtifactID != root.Index.ArtifactID || source.ManifestSHA256 != root.Manifest.SHA256 || scope.ProjectIdentity != root.ProjectIdentity || scope.RepositoryIdentity != root.RepositoryIdentity || scope.SelectedRelativePath != root.SelectedRelativePath {
		return nil, evidence.ErrCorrupt
	}
	manifest, _, err := e.workflowManifest(ctx, source.ManifestArtifactID)
	if err != nil {
		return nil, err
	}
	repository := scope.Destination
	if root.SelectedRelativePath != "" && root.SelectedRelativePath != "." {
		if !domain.CanonicalWorkflowPath(root.SelectedRelativePath) {
			return nil, evidence.ErrCorrupt
		}
		for range strings.Split(root.SelectedRelativePath, "/") {
			repository = filepath.Dir(repository)
		}
	}
	branch := "HEAD"
	if root.Branch != nil {
		branch = *root.Branch
	}
	return &workspace.Snapshot{ID: source.ID, AttemptID: source.AttemptID, ProjectPath: scope.Destination, RepositoryPath: repository, SelectedSubfolder: source.SelectedRelativePath, BaseCommit: source.BaseCommit, Branch: branch, ManifestArtifactID: source.ManifestArtifactID, IndexArtifactID: source.IndexArtifactID, Manifest: manifest}, nil
}

// Physical verification covers every member, while the transaction receives
// bounded immutable closure anchors rather than an unbounded payload inventory.
func (e *Engine) verifyGraphClosureAnchors(ctx context.Context, all, anchors []domain.GraphArtifactRef) ([]domain.VerifiedResponse, error) {
	wanted := map[string]domain.GraphArtifactRef{}
	for _, ref := range anchors {
		wanted[ref.ArtifactID] = ref
	}
	proofs := make([]domain.VerifiedResponse, 0, len(wanted))
	seen := map[string]domain.GraphArtifactRef{}
	for _, ref := range all {
		if old, ok := seen[ref.ArtifactID]; ok {
			if old != ref {
				return nil, evidence.ErrCorrupt
			}
			continue
		}
		seen[ref.ArtifactID] = ref
		meta, err := e.evidence.VerifyClosed(ctx, ref.ArtifactID)
		if err != nil {
			return nil, err
		}
		if !validGraphArtifactReference(ref) || !graphArtifactMatches(meta, ref, meta.Kind) {
			return nil, evidence.ErrCorrupt
		}
		if anchor, ok := wanted[ref.ArtifactID]; ok {
			if anchor != ref {
				return nil, evidence.ErrCorrupt
			}
			proofs = append(proofs, domain.VerifiedResponse{ArtifactID: meta.ID, SHA256: *meta.SHA256, PhysicalIdentity: meta.Identity, ByteLength: meta.ByteLength})
			delete(wanted, ref.ArtifactID)
		}
	}
	if len(wanted) != 0 {
		return nil, evidence.ErrCorrupt
	}
	return proofs, nil
}
