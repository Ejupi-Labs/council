package workflow

import (
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestPlanUnresolvedReviewFindingRequiresFreshReviewDespiteDirectProof(t *testing.T) {
	for _, responseOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct_candidate", true: "response_compatibility"}[responseOnly], func(t *testing.T) {
			input := PlanInput{
				Authority:            domain.WorkflowAuthority{Review: "auto"},
				HasSubject:           true,
				HasDelta:             !responseOnly,
				ResponseOnly:         responseOnly,
				DirectProven:         !responseOnly,
				CorrectionCyclesUsed: 1,
				BlockingFindingIDs:   []string{strings.Repeat("a", 32)},
				NeedsFindingReview:   true,
			}
			decision := Plan(input)
			if decision.Role != "review" || decision.ExecutorKind != "native" || decision.Action != "review" || decision.Owner != "council" {
				t.Fatalf("unresolved review-origin blocker became a direct-proof shortcut/compatibility bypass/dead end: %+v", decision)
			}
		})
	}
}

func TestPlanResolvedCheckOnlyFindingDoesNotInventModelReviewForDirectProof(t *testing.T) {
	// The controller has established a passing affected check and resolved its
	// check-only finding. No independent review-origin finding remains.
	definition := domain.CheckDefinition{ID: strings.Repeat("a", 32), Key: "controlled-check", Kind: "command", Source: "task_declaration", Required: true, DefinitionSHA256: strings.Repeat("b", 64), Command: &domain.CommandDefinition{SuccessExitCodes: []int{0}}}
	input := PlanInput{
		Authority:            domain.WorkflowAuthority{Review: "auto", Delivery: domain.DeliveryScope{LocalMode: "review_only"}, Checks: []domain.CheckDefinition{definition}},
		HasSubject:           true,
		HasDelta:             true,
		DirectProven:         true,
		CorrectionCyclesUsed: 1,
		NeedsFindingReview:   false,
		BlockingFindingIDs:   []string{strings.Repeat("c", 32)},
	}
	if before := Plan(input); before.Role != "check" || before.CheckID != definition.ID {
		t.Fatalf("check-only correction skipped the affected required check: %+v", before)
	}
	zero := 0
	input.Checks = []domain.CheckReceipt{{DefinitionID: definition.ID, DefinitionSHA256: definition.DefinitionSHA256, Outcome: "passed", RootTerminal: true, InputComplete: true, OutputComplete: true, ExitCode: &zero, Cleanup: domain.Cleanup{State: "verified"}}}
	input.BlockingFindingIDs = nil // Current controller proof resolves this check-origin finding.
	decision := Plan(input)
	if decision.Action != "accept" || decision.Role != "" || decision.ExecutorKind != "" {
		t.Fatalf("resolved check-only blocker invented a model review: %+v", decision)
	}
}
