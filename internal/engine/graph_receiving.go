package engine

import (
	"context"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type graphReceivingSource struct {
	Dispatch      domain.GraphDispatchRecord
	Materialized  workspace.GraphMaterialization
	CapturedScope domain.DeliveryScope
}

// Preparing a graph receiver never captures the live checkout. First authors
// receive the sealed input; later native roles observe the existing005 Handoff
// of their exact selected subject instead of resetting to the original input.
func (e *Engine) prepareGraphReceivingSource(ctx context.Context, a domain.Attempt, handoff *workspace.Snapshot) (*graphReceivingSource, error) {
	return e.prepareGraphStageReceivingSource(ctx, a, handoff, "native")
}

func (e *Engine) prepareGraphCheckReceivingSource(ctx context.Context, a domain.Attempt, handoff *workspace.Snapshot) (*graphReceivingSource, error) {
	return e.prepareGraphStageReceivingSource(ctx, a, handoff, "local_check")
}

func (e *Engine) prepareGraphStageReceivingSource(ctx context.Context, a domain.Attempt, handoff *workspace.Snapshot, executor string) (*graphReceivingSource, error) {
	member, err := e.graphStageContextMembership(ctx, a, executor)
	if err != nil || member == nil {
		return nil, err
	}
	dispatch, input, _, err := e.loadGraphStageContext(ctx, a, *member, executor)
	if err != nil {
		return nil, err
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, member.GraphID)
	if err != nil {
		return nil, err
	}
	receiving := &graphReceivingSource{Dispatch: dispatch, CapturedScope: definition.Delivery}
	e.control.Lock()
	guardErr := e.db.CheckGraphDispatchPreparation(ctx, e.config.OwnerInstanceID, a.ID)
	if e.fenced && guardErr == nil {
		guardErr = domain.Fail("dispatch_fenced", "Resolve retained ownership before graph receiving preparation.")
	}
	e.control.Unlock()
	if guardErr != nil {
		return receiving, guardErr
	}
	d := dispatch.Public.Dispatch
	switch d.SourceBasis.BasisKind {
	case "graph_input":
		if executor != "native" || a.Role != "author" || handoff != nil {
			return receiving, domain.Fail("graph_input_stale", "Only an exact first-author input may seed a fresh graph receiving workspace.")
		}
		graph, err := e.db.Graph(ctx, d.GraphID)
		if err != nil || graph.Graph.RootInputID == nil {
			return receiving, domain.Fail("graph_input_stale", "The receiving graph has no complete retained root basis.")
		}
		root, err := e.db.GraphInputRecord(ctx, d.GraphID, *graph.Graph.RootInputID)
		if err != nil || root.PrivateRoot == nil || root.Public.Summary.State != "ready" {
			return receiving, domain.Fail("graph_input_stale", "The exact retained graph root is unavailable.")
		}
		rootSource, err := e.graphRootSourceSnapshot(ctx, *root.PrivateRoot, definition.Delivery)
		if err != nil {
			return receiving, err
		}
		receiving.Materialized, err = e.workspace.MaterializeGraphInput(ctx, workspace.GraphMaterializeRequest{GraphID: d.GraphID, ReceivingAttemptID: a.ID, Role: a.Role, RootBinding: *root.PrivateRoot, RootSource: rootSource, InputBinding: *input.Public.InputBinding, SourceBasis: d.SourceBasis})
		return receiving, err
	case "node_subject":
		if handoff == nil {
			return receiving, domain.Fail("graph_input_stale", "A later graph role requires its exact selected-subject Handoff.")
		}
		receiving.Materialized, err = e.workspace.ObserveGraphHandoff(ctx, workspace.GraphHandoffObservationRequest{GraphID: d.GraphID, ReceivingAttemptID: a.ID, Role: a.Role, InputBinding: *input.Public.InputBinding, SourceBasis: d.SourceBasis, Snapshot: handoff})
		return receiving, err
	default:
		return receiving, domain.Fail("graph_input_stale", "The receiving dispatch has no supported exact source basis.")
	}
}

// Source metadata and its exact receipt are installed together. The caller must
// skip standalone PutSourceSnapshot for this receiver to keep one immutable row.
func (e *Engine) recordGraphReceivingSource(parent context.Context, a domain.Attempt, receiving *graphReceivingSource) error {
	if receiving == nil {
		return domain.Fail("graph_input_stale", "A graph receiver needs its exact observed materialization.")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 15*time.Second)
	defer cancel()
	m := receiving.Materialized
	if !domain.ValidID(m.Receipt.ID) || !validGraphArtifactReference(m.ReceiptArtifact) {
		return evidence.ErrCorrupt
	}
	facts := domain.GraphMaterializationFacts{Receipt: m.Receipt, ReceiptArtifact: m.ReceiptArtifact, WorkspaceIdentity: m.WorkspaceIdentity}
	refs := []domain.GraphArtifactRef{m.ReceiptArtifact}
	if m.Receipt.OperationEffects != nil {
		refs = append(refs, *m.Receipt.OperationEffects)
	}
	if m.Receipt.ObservedManifest != nil {
		refs = append(refs, *m.Receipt.ObservedManifest)
	}
	if m.Receipt.ObservedIndex != nil {
		refs = append(refs, *m.Receipt.ObservedIndex)
	}
	if m.Snapshot != nil {
		snapshot := m.Snapshot
		meta, err := e.evidence.VerifyClosed(ctx, snapshot.ManifestArtifactID)
		if err != nil || meta.SHA256 == nil || meta.AttemptID != a.ID || meta.Kind != "source_manifest" {
			return evidence.ErrCorrupt
		}
		facts.SourceSnapshot = &domain.SourceSnapshot{ID: snapshot.ID, AttemptID: a.ID, ManifestArtifactID: snapshot.ManifestArtifactID, IndexArtifactID: snapshot.IndexArtifactID, ManifestSHA256: *meta.SHA256,
			BaseCommit: snapshot.BaseCommit, Branch: receiving.CapturedScope.Branch, ProjectIdentity: receiving.CapturedScope.ProjectIdentity, SelectedRelativePath: snapshot.SelectedSubfolder, CreatedAt: m.Receipt.CompletedAt}
		facts.GitControlSHA256 = snapshot.GitControlSHA256
	}
	if m.Receipt.State == "complete" {
		proof, err := e.workspace.RecheckGraphMaterialization(ctx, m.Receipt, m.Snapshot, m.WorkspaceIdentity)
		if err != nil {
			return err
		}
		refs = append(refs, proof.Artifacts...)
	}
	var err error
	facts.VerifiedArtifacts, err = e.verifyGraphClosureAnchors(ctx, refs, refs)
	if err != nil {
		return err
	}
	e.control.Lock()
	err = e.db.RecordGraphMaterialization(ctx, e.config.OwnerInstanceID, a.ID, facts)
	e.control.Unlock()
	return err
}

func (e *Engine) recheckGraphReceivingSource(ctx context.Context, a domain.Attempt, receiving *graphReceivingSource) error {
	// A known Stop before GO must not become a failed SQL read and invented
	// durability uncertainty. No physical recheck has begun at this boundary.
	if err := ctx.Err(); err != nil {
		return err
	}
	if receiving == nil {
		return domain.Fail("graph_input_stale", "A graph worker has no exact receiving workspace to recheck.")
	}
	m := receiving.Materialized
	if _, err := e.workspace.RecheckGraphMaterialization(ctx, m.Receipt, m.Snapshot, m.WorkspaceIdentity); err != nil {
		return err
	}
	e.control.Lock()
	defer e.control.Unlock()
	if e.fenced {
		return domain.Fail("dispatch_fenced", "Resolve graph execution ownership before native creation.")
	}
	return e.db.CheckGraphDispatchPreparation(ctx, e.config.OwnerInstanceID, a.ID)
}
