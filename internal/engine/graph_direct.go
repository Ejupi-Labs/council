package engine

import (
	"context"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// A digest binds opaque shared instructions to the result; it does not prove
// those instructions satisfied. Keep the cheap standalone document proof only
// when the graph adds no unproved shared prose to that receiver's obligations.
func (e *Engine) graphDirectProofGuard(ctx context.Context, w domain.Workflow, original string, version domain.TaskVersion) *domain.Error {
	err := e.db.CheckStandaloneAuthority(ctx, w.TaskID, false)
	if err == nil {
		return nil
	}
	scope := domain.SafeError(err)
	if scope.GraphRef == nil {
		return scope
	}
	ref := scope.GraphRef
	if ref.Kind != "node" || ref.NodeID == nil {
		return domain.Fail("evidence_unavailable", "A graph root has no authored receiver subject to prove.")
	}
	graph, err := e.db.Graph(ctx, ref.GraphID)
	if err != nil {
		return domain.SafeError(err)
	}
	var inputID string
	for _, node := range graph.Nodes {
		if node.ID == *ref.NodeID && node.TaskID == w.TaskID && node.TaskVersion == w.TaskVersion && node.TaskVersion == version.Version && node.InputID != nil {
			inputID = *node.InputID
			break
		}
	}
	if inputID == "" {
		return domain.Fail("graph_input_stale", "Direct proof requires the exact current receiving input and task version.")
	}
	input, err := e.db.GraphInputRecord(ctx, ref.GraphID, inputID)
	if err != nil {
		return domain.SafeError(err)
	}
	conditions := input.ConditionsBinding
	if input.Public.Summary.State != "ready" || !input.Public.Summary.Current || conditions == nil || conditions.ReceiverTaskID != w.TaskID || conditions.ReceiverTaskVersion != version.Version {
		return domain.Fail("graph_input_stale", "The current receiver has no complete applicable graph conditions.")
	}
	seal, err := domain.GraphConditionsDigest(*conditions)
	if err != nil || seal != conditions.ConditionsSHA256 {
		return domain.Fail("evidence_unavailable", "The applicable graph conditions cannot be verified.")
	}
	_, own, err := workflowpolicy.ConditionDigests(original, version, w.Authority.Conditions, w.Authority.Checks)
	if err != nil {
		return domain.SafeError(err)
	}
	effective, err := graphEffectiveConditionsDigest(own, seal)
	if err != nil || effective != w.Authority.ConditionsSHA256 {
		return domain.Fail("evidence_unavailable", "The direct proof does not bind the captured shared and receiver conditions.")
	}
	shared := 0
	for _, source := range conditions.Sources {
		if source.AuthorityScope != "graph_shared" {
			continue
		}
		shared++
		body, err := e.readGraphArtifact(ctx, source.OriginalRef, "graph_original", domain.MaxGraphDeclarationBytes)
		if err != nil {
			return domain.SafeError(err)
		}
		if strings.TrimSpace(string(body)) != "" {
			return domain.Fail("evidence_unavailable", "The narrow document proof does not prove the additional shared instructions. Include them in the graph's complete review.")
		}
	}
	if shared != 1 {
		return domain.Fail("evidence_unavailable", "Complete shared instruction provenance is required before direct proof.")
	}
	return nil
}
