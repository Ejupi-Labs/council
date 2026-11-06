package workflow

import (
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestPlanRequiredReviewThenObservedFailureThenSingleCorrection(t *testing.T) {
	check := domain.CheckDefinition{ID: strings.Repeat("a", 32), Key: "validation", Kind: "command", Source: "task_declaration", Required: true, DefinitionSHA256: strings.Repeat("b", 64)}
	in := PlanInput{HasSubject: true, HasDelta: true, Authority: domain.WorkflowAuthority{Review: "required", Checks: []domain.CheckDefinition{check}, Correction: domain.CorrectionPolicy{MaxCycles: 1, Triggers: []string{"failed_required_check", "blocking_review_finding"}}}}
	if got := Plan(in); got.Role != "review" {
		t.Fatal("explicit initial independent review was skipped", got)
	}
	in.Review = &domain.ReviewRecord{Outcome: "changes_required", Report: domain.ReviewReport{Outcome: "changes_required", Findings: []domain.ReviewFinding{{Severity: "blocking"}}, Gaps: []domain.ReviewGap{}}}
	if got := Plan(in); got.Role != "check" || got.CheckID != check.ID {
		t.Fatal("blocking review skipped actual required verifier", got)
	}
	one := 1
	in.Checks = []domain.CheckReceipt{{DefinitionID: check.ID, DefinitionSHA256: check.DefinitionSHA256, Outcome: "failed", RootTerminal: true, InputComplete: true, OutputComplete: true, ExitCode: &one, Cleanup: domain.Cleanup{State: "verified"}}}
	if got := Plan(in); got.Role != "correction" {
		t.Fatal("observed valid failure did not use captured correction", got)
	}
	in.CorrectionCyclesUsed = 1
	if got := Plan(in); got.Owner != "user" || got.Role != "" {
		t.Fatal("exhausted correction cycle rearmed", got)
	}
	in.Review = nil
	in.Checks = nil
	if got := Plan(in); got.Role != "check" {
		t.Fatal("corrected subject skipped affected check before re-review", got)
	}
	zero := 0
	in.Checks = []domain.CheckReceipt{{DefinitionID: check.ID, DefinitionSHA256: check.DefinitionSHA256, Outcome: "passed", RootTerminal: true, InputComplete: true, OutputComplete: true, ExitCode: &zero, Cleanup: domain.Cleanup{State: "verified"}}}
	if got := Plan(in); got.Role != "review" {
		t.Fatal("corrected checked subject skipped independent re-review", got)
	}
}

func TestPlanNeverRetriesUnknownOrRetainedExternalEffects(t *testing.T) {
	in := PlanInput{HasSubject: true, HasDelta: true, DirectProven: true, ExternalOutcome: "uncertain", Authority: domain.WorkflowAuthority{Delivery: domain.DeliveryScope{ExternalAction: &domain.ExternalActionInput{}}}}
	if got := Plan(in); got.Owner != "user" || got.Role != "" {
		t.Fatal("uncertain external operation was replayed", got)
	}
	in = PlanInput{StageFailure: "native_protocol_invalid"}
	if got := Plan(in); got.Role != "" || got.Owner != "user" {
		t.Fatal("native failure created author/review work", got)
	}
}

func TestPlanMissingExplicitCheckCannotDisappearBehindDirectProof(t *testing.T) {
	checks := []domain.CheckDeclaration{{Key: "mandatory-check", Executable: "node", Arguments: []string{"controlled.js"}}}
	in := PlanInput{HasSubject: true, HasDelta: true, DirectProven: true, Authority: domain.WorkflowAuthority{OptionSnapshot: domain.WorkflowInput{Checks: &checks}, CheckGaps: []domain.WorkflowGap{{Key: "mandatory-check", Code: "check_unavailable"}}}}
	if got := Plan(in); got.Role != "" || got.Action != "check_unavailable" {
		t.Fatal("missing explicit check disappeared behind builtin proof", got)
	}
	in.Authority.OptionSnapshot = domain.WorkflowInput{}
	in.Authority.CheckGaps[0].Key = "go-tests"
	if got := Plan(in); got.Action != "accept" {
		t.Fatal("unrelated implicit convention gap defeated sufficient builtin", got)
	}
}

func TestPlanUnavailableBuiltinUsesReviewAndNeverCreatesCorrectionTrigger(t *testing.T) {
	definition := domain.CheckDefinition{ID: strings.Repeat("a", 32), Key: "literal-document", Kind: "builtin", Source: "recognized_convention", Required: true, DefinitionSHA256: strings.Repeat("b", 64)}
	in := PlanInput{HasSubject: true, HasDelta: true, Authority: domain.WorkflowAuthority{Checks: []domain.CheckDefinition{definition}, Correction: domain.CorrectionPolicy{MaxCycles: 1, Triggers: []string{"failed_required_check"}}}}
	if got := Plan(in); got.Role != "review" {
		t.Fatal("false direct proof prevented normal independent review", got)
	}
	in.DirectProven = true
	in.Checks = []domain.CheckReceipt{{DefinitionID: definition.ID, DefinitionSHA256: definition.DefinitionSHA256, Outcome: "failed", BuiltinCompleted: true, Cleanup: domain.Cleanup{State: "verified"}}}
	if got := Plan(in); got.Role != "" || got.Owner != "user" || got.Action != "check_unavailable" {
		t.Fatal("failed builtin invented observed command-failure correction authority", got)
	}
}
