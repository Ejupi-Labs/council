package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphOutputResponseClosureMustBindSelectedAcknowledgedIndex(t *testing.T) {
	s, dispatch, facts, _ := graphOutputSettlementFixture(t, true)
	ctx := context.Background()
	stored, e := s.GraphDispatchForAttempt(ctx, dispatch.Dispatch.AttemptID)
	if e != nil || stored.Public.Materialization == nil || stored.Public.Materialization.ObservedIndex == nil {
		t.Fatal(stored, e)
	}
	// Both are real registered, complete, producer-owned indexes. The receiving
	// pre-command index is not the selected response's separately retained index.
	sourceIndex := *stored.Public.Materialization.ObservedIndex
	if sourceIndex.ArtifactID == facts.ResponseBinding.IndexArtifactID {
		t.Fatal("fixture failed to retain distinct source and selected indexes")
	}
	closure := *facts.GraphOutput.ResponseClosure
	closure.Index = sourceIndex
	closure.ClosureSHA256, e = domain.GraphOutputDigest(closure)
	if e != nil {
		t.Fatal(e)
	}
	ref, proof := graphPayload(t, s, dispatch.Dispatch.AttemptID, "graph_output_closure", []byte(encode(closure)))
	facts.GraphOutput.ResponseClosure = &closure
	facts.GraphOutput.Observation.Response.ClosureRef = &ref
	facts.GraphOutput.VerifiedArtifacts = append(facts.GraphOutput.VerifiedArtifacts, proof)
	facts.VerifiedArtifacts = append(facts.VerifiedArtifacts, proof)
	if _, e = s.SettleWorkflowStep(ctx, dispatch.Dispatch.AttemptID, facts); e == nil {
		t.Fatal("resealed source-index closure qualified a different acknowledged response index")
	}
	a, e := s.Attempt(ctx, dispatch.Dispatch.AttemptID)
	if e != nil || a.Phase == "settled" {
		t.Fatal("refused closure partly settled producer", a, e)
	}
	stored, e = s.GraphDispatchForAttempt(ctx, dispatch.Dispatch.AttemptID)
	if e != nil || stored.Public.OutputObservation != nil {
		t.Fatal("refused selected closure partly installed an observation", stored, e)
	}
}
