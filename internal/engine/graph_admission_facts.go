package engine

import (
	"context"
	json "encoding/json/v2"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func (e *Engine) graphInitialAdmissionFacts(ctx context.Context, definition domain.GraphDefinition, node domain.GraphNodeSummary, control domain.GraphDispatchBinding) (domain.GraphNodeAdmissionFacts, error) {
	facts := domain.GraphNodeAdmissionFacts{Control: control, NodeID: node.ID, TaskVersion: node.TaskVersion}
	if node.InputID == nil {
		return facts, domain.Fail("graph_input_stale", "Prepare this receiver's exact input before native admission.")
	}
	record, err := e.db.GraphInputRecord(ctx, definition.GraphID, *node.InputID)
	if err != nil {
		return facts, err
	}
	input := record.Public
	if input.InputBinding == nil || input.ConditionsRef == nil || record.ConditionsBinding == nil || input.Summary.State != "ready" || !input.Summary.Current {
		return facts, domain.Fail("graph_input_stale", "The current node input is not ready for native admission.")
	}
	facts.InputBinding, facts.ConditionsRef, facts.ConditionsBinding = *input.InputBinding, *input.ConditionsRef, *record.ConditionsBinding
	// A capacity-release trigger may belong to another task. Content authority
	// consumes this receiver's actual input operation, never that unrelated ID.
	facts.Control.OperationAttemptID = input.Summary.OperationAttemptID
	inputID := input.Summary.ID
	facts.Control.InputID = &inputID
	facts.Control.ContributorVector = append([]domain.GraphContributorBinding{}, input.InputBinding.ContributorVector...)
	body, err := e.readGraphArtifact(ctx, facts.InputBinding.Manifest, "graph_input_manifest", 64<<20)
	if err != nil {
		return facts, err
	}
	var manifest workspace.Manifest
	if err = json.Unmarshal(body, &manifest, json.RejectUnknownMembers(true)); err != nil {
		return facts, evidence.ErrCorrupt
	}
	working, index, err := workspace.GraphManifestDigests(manifest)
	if err != nil {
		return facts, err
	}
	facts.SourceBasis = domain.GraphSourceBasis{BasisKind: "graph_input", InputID: facts.InputBinding.ID, InputSHA256: facts.InputBinding.InputSHA256,
		ExpectedManifest: facts.InputBinding.Manifest, ExpectedIndex: facts.InputBinding.Index, ExpectedWorkingEntriesSHA256: working, ExpectedIndexEntriesSHA256: index}
	facts.VerifiedArtifacts, err = e.verifyWorkflowArtifacts(ctx, facts.InputBinding.Manifest.ArtifactID, facts.InputBinding.Index.ArtifactID, facts.InputBinding.StagedBlobs.ArtifactID, facts.InputBinding.Provenance.ArtifactID, facts.InputBinding.ContextPacket.ArtifactID, facts.ConditionsRef.ArtifactID)
	if err != nil {
		return facts, err
	}
	detail, err := e.db.TaskVersion(ctx, node.TaskID, node.TaskVersion)
	if err != nil {
		return facts, err
	}
	// Capture performs full physical input verification and observes check programs
	// outside SQL. The store independently validates current scope and its seals.
	capture, scoped, err := e.captureGraphWorkflow(ctx, definition, node, detail, input)
	if err != nil {
		return facts, err
	}
	facts.Capture, facts.ConditionsBinding = capture, scoped
	versionOptions, err := e.db.GraphNodeOptions(ctx, definition.GraphID, node.ID, node.TaskVersion)
	if err != nil {
		return facts, err
	}
	facts.Routing, err = e.graphRoutingFacts(ctx, definition, versionOptions)
	if err != nil {
		return facts, err
	}
	return facts, nil
}
