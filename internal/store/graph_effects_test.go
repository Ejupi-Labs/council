package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphNativeUnknownContextEffectsRetainExactClaimWithoutInventingProcessUncertainty(t *testing.T) {
	s, d, f, owner := graphMaterializationFixture(t)
	ctx := context.Background()
	if e := s.RecordGraphMaterialization(ctx, owner, d.Dispatch.AttemptID, f); e != nil {
		t.Fatal(e)
	}
	// Typed controller fact: receiving materialization completed, but a later
	// separate context-copy operation has uncertain effects and created no child.
	// This test qualifies transaction semantics, not real filesystem uncertainty.
	_, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, domain.WorkflowStepFacts{StepID: d.Dispatch.StepID, OwnerInstanceID: owner, Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: false, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete", Failure: &domain.PrimaryFailure{Code: "context_effects_unknown", Message: "The controlled context copy effects could not be established."}}})
	if e != nil {
		t.Fatal(e)
	}
	gate, e := s.Gate(ctx)
	if e != nil || gate.State != "fenced" || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != d.Dispatch.AttemptID {
		t.Fatal("unknown graph context effects lost exact ownership", gate, e)
	}
	a, e := s.Attempt(ctx, d.Dispatch.AttemptID)
	if e != nil || a.Cleanup.State != "verified" || a.Cleanup.Scope != "not_launched" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "context_effects_unknown" {
		t.Fatal("filesystem uncertainty invented process uncertainty or replaced primary failure", a, e)
	}
}
