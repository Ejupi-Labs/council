package engine

import (
	"context"
	json "encoding/json/v2"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

// MutateGraph decodes only public intent. Filesystem proofs and admission facts
// are constructed by the current owner and can never arrive in this body.
func (e *Engine) MutateGraph(ctx context.Context, op, target string, body []byte) domain.Result {
	requestID := domain.MutationIdentity(body)
	var input any
	switch op {
	case "graphs.start":
		input = &domain.GraphStartInput{}
	case "graphs.advance":
		input = &domain.GraphAdvanceInput{}
	case "graphs.stop":
		input = &domain.GraphStopInput{}
	case "graphs.nodes.stop":
		input = &domain.GraphNodeStopInput{}
	case "graphs.nodes.guidance":
		input = &domain.GraphGuidanceInput{}
	case "graphs.budget.amend":
		input = &domain.GraphBudgetAmendmentInput{}
	default:
		return e.failure(requestID, domain.Fail("not_found", "This graph mutation does not exist."))
	}
	if op == "graphs.start" && len(body) > domain.MaxGraphDeclarationBytes {
		return e.failure(requestID, domain.Fail("input_too_large", "The graph declaration exceeds 2 MiB. Reduce it explicitly."))
	}
	if invalid := domain.StrictDecode(body, input); invalid != nil {
		return e.failure(requestID, invalid)
	}
	graphID, key := target, ""
	switch op {
	case "graphs.start":
		if target != "" {
			return e.failure(requestID, domain.Invalid("no graph target on creation"))
		}
	case "graphs.nodes.stop", "graphs.nodes.guidance":
		parts := strings.Split(target, "/")
		if len(parts) != 2 || !domain.ValidID(parts[0]) || !domain.GraphKey(parts[1]) {
			return e.failure(requestID, domain.Invalid("exact graph identity and node key"))
		}
		graphID, key = parts[0], parts[1]
	default:
		if !domain.ValidID(graphID) {
			return e.failure(requestID, domain.Invalid("graph identity"))
		}
	}
	var invalid *domain.Error
	var fingerprint string
	switch in := input.(type) {
	case *domain.GraphStartInput:
		invalid, fingerprint = domain.ValidateGraphStartInput(*in), store.GraphStartFingerprint(*in)
	case *domain.GraphAdvanceInput:
		invalid, fingerprint = domain.ValidateGraphAdvanceInput(*in), store.GraphAdvanceFingerprint(graphID, *in)
	case *domain.GraphStopInput:
		invalid, fingerprint = domain.ValidateGraphStopInput(*in), store.GraphStopFingerprint(graphID, *in)
	case *domain.GraphNodeStopInput:
		invalid, fingerprint = domain.ValidateGraphNodeStopInput(*in), store.GraphNodeStopFingerprint(graphID, key, *in)
	case *domain.GraphGuidanceInput:
		invalid, fingerprint = domain.ValidateGraphGuidanceInput(*in), store.GraphGuidanceFingerprint(graphID, key, *in)
	case *domain.GraphBudgetAmendmentInput:
		invalid, fingerprint = domain.ValidateGraphBudgetAmendmentInput(*in), store.GraphBudgetAmendmentFingerprint(graphID, *in)
	}
	if invalid != nil {
		return e.failure(requestID, invalid)
	}
	if previous, err := e.db.Replay(ctx, requestID, op, fingerprint); err != nil {
		return e.failure(requestID, err)
	} else if previous != nil {
		if op == "graphs.stop" || op == "graphs.nodes.stop" {
			e.cancelGraphTargets(*previous)
		}
		return *previous
	}
	if in, ok := input.(*domain.GraphStartInput); ok {
		return e.createGraph(ctx, *in)
	}
	if in, ok := input.(*domain.GraphAdvanceInput); ok {
		return e.advanceGraph(ctx, graphID, *in)
	}
	if _, guidance := input.(*domain.GraphGuidanceInput); guidance {
		e.prepareGate.Lock()
		defer e.prepareGate.Unlock()
	}
	e.control.Lock()
	var result domain.Result
	switch in := input.(type) {
	case *domain.GraphStopInput:
		result = e.db.StopGraph(ctx, e.config.OwnerInstanceID, graphID, *in)
	case *domain.GraphNodeStopInput:
		result = e.db.StopGraphNode(ctx, e.config.OwnerInstanceID, graphID, key, *in)
	case *domain.GraphBudgetAmendmentInput:
		result = e.db.AmendGraphBudget(ctx, e.config.OwnerInstanceID, graphID, *in)
	case *domain.GraphGuidanceInput:
		result = e.db.UpdateGraphNodeGuidance(ctx, e.config.OwnerInstanceID, graphID, key, *in, domain.GraphAdvanceFacts{})
	default:
		// Advance requires fresh verified input and routing facts.
		// Their public decoder is available before those transitions are enabled.
		result = e.failure(requestID, domain.Fail("capability_unavailable", "Verified graph continuation is not yet enabled."))
	}
	e.guardMutationLocked(result)
	e.control.Unlock()
	if op == "graphs.stop" || op == "graphs.nodes.stop" {
		e.cancelGraphTargets(result)
	}
	// Grants and guidance never register live execution or rearm a graph.
	return result
}

func (e *Engine) cancelGraphTargets(result domain.Result) {
	if result.Envelope.Error != nil {
		return
	}
	var stopped domain.GraphStopResult
	body, err := json.Marshal(result.Envelope.Data)
	if err == nil {
		err = json.Unmarshal(body, &stopped, json.RejectUnknownMembers(true))
	}
	if err != nil {
		e.fence("", "graph_stop_receipt_uncertain", "A successful graph Stop receipt could not identify its exact owned targets.")
		return
	}
	e.control.Lock()
	for _, id := range stopped.TargetAttemptIDs {
		e.cancelWorkerLocked(id)
	}
	e.control.Unlock()
	for _, id := range stopped.TargetAttemptIDs {
		if a, err := e.db.Attempt(context.Background(), id); err == nil && a.Stop != nil {
			e.watchStop(id, a.Stop.AcceptedAt)
		}
	}
}
