package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// A producer's new basis is not selected/associated until terminal settlement.
// Reuse its already physically verified capture, read its exact closed headers,
// and prove document bytes against the real graph root without loading an older
// selected final or changing the ordinary node's DirectProven facts.
func (e *Engine) graphCapturedFinalDirectProof(ctx context.Context, a domain.Attempt, output *domain.GraphStepOutputFacts) (bool, error) {
	if output == nil || output.FinalVerification == nil {
		return false, nil
	}
	final := *output.FinalVerification
	if output.CandidateClosure == nil || output.Observation.ProducerAttemptID != a.ID || output.Observation.Candidate == nil || output.Observation.Candidate.FinalVerificationRef == nil ||
		output.Observation.Candidate.CandidateID != final.FinalCandidateBinding.CandidateID || final.FinalTaskID != a.TaskID || final.FinalTaskVersion != a.TaskVersion || final.FinalWorkflowID != a.WorkflowID ||
		output.CandidateClosure.ProducerAttemptID != a.ID || output.CandidateClosure.GraphID != final.GraphID || output.CandidateClosure.NodeID != final.FinalNodeID ||
		output.CandidateClosure.Manifest.ArtifactID != final.FinalCandidateBinding.ManifestArtifactID || output.CandidateClosure.Manifest.SHA256 != final.FinalCandidateBinding.ManifestSHA256 {
		return false, evidence.ErrCorrupt
	}
	ref := *output.Observation.Candidate.FinalVerificationRef
	body, err := e.readGraphArtifact(ctx, ref, "graph_final_verification", workspace.MaxFileBytes)
	if err != nil {
		return false, err
	}
	var retained domain.GraphFinalVerificationBinding
	if json.Unmarshal(body, &retained, json.RejectUnknownMembers(true)) != nil || !reflect.DeepEqual(retained, final) || ref.ProducerAttemptID != a.ID {
		return false, evidence.ErrCorrupt
	}
	seal, err := domain.GraphFinalVerificationDigest(final)
	if err != nil || seal != final.BindingSHA256 {
		return false, evidence.ErrCorrupt
	}
	root, err := e.db.GraphRoot(ctx, final.GraphID)
	if err != nil {
		return false, err
	}
	if !reflect.DeepEqual(domain.GraphRootPublic(root), final.RootBinding) {
		return false, evidence.ErrCorrupt
	}
	selected, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil {
		return false, err
	}
	if selected.Workflow == nil || selected.Workflow.TaskVersion != a.TaskVersion || selected.Workflow.CurrentStepID == nil || *selected.Workflow.CurrentStepID != a.StepID {
		return false, domain.Fail("graph_input_stale", "The newly captured final no longer belongs to its exact current producer step.")
	}
	version, err := e.db.TaskVersion(ctx, a.TaskID, a.TaskVersion)
	if err != nil {
		return false, err
	}
	beforeBytes, err := e.readGraphArtifact(ctx, root.Manifest, "source_manifest", workspace.MaxFileBytes)
	if err != nil {
		return false, err
	}
	afterBytes, err := e.readGraphArtifact(ctx, output.CandidateClosure.Manifest, "candidate_manifest", workspace.MaxFileBytes)
	if err != nil {
		return false, err
	}
	var before, after workspace.Manifest
	if json.Unmarshal(beforeBytes, &before, json.RejectUnknownMembers(true)) != nil || json.Unmarshal(afterBytes, &after, json.RejectUnknownMembers(true)) != nil {
		return false, evidence.ErrCorrupt
	}
	return e.graphCumulativeDirectProof(ctx, *selected.Workflow, before, after, version.OriginalText, version.Version) == nil, nil
}
