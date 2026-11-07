package workflow

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These are explicitly typed controller facts. They qualify only deterministic
// policy; physical final-basis capture and current SQL authority are separate.
func graphFinalPlanFixture() PlanInput {
	id, hash := strings.Repeat("a", 32), strings.Repeat("b", 64)
	return PlanInput{HasSubject: true, DirectProven: true,
		Authority: domain.WorkflowAuthority{ConditionsSHA256: hash, Review: "auto", Delivery: domain.DeliveryScope{LocalMode: "review_only"}},
		GraphFinal: &GraphFinalPlanFacts{DeclaredFileFinal: true, HasCumulativeWorkingDelta: true,
			VerificationBindingRef: domain.GraphArtifactRef{ArtifactID: id, ProducerAttemptID: id, SHA256: hash, ByteLength: "2048"},
			CurrentSubject:         domain.CandidateBinding{RootSourceSnapshotID: id, CandidateID: id, SourceSnapshotID: id, ManifestArtifactID: id, ManifestSHA256: hash, IndexArtifactID: id, IndexSHA256: hash, ConditionsSHA256: hash}}}
}

func TestGraphFinalPlanUsesCumulativeEligibilityWithoutClaimingNodeEditsOrDirectProof(t *testing.T) {
	in := graphFinalPlanFixture()
	before := *in.GraphFinal
	if got := Plan(in); got.Role != "review" || got.Action != "review" || got.Owner != "council" {
		t.Fatal("A verified cumulative file-final was blocked by zero own delta or reused node-only direct proof", got)
	}
	if in.HasDelta || in.HasIndexDelta || !reflect.DeepEqual(*in.GraphFinal, before) {
		t.Fatal("Planning manufactured node change facts or modified the verified basis")
	}
	in.GraphFinal.HasCumulativeWorkingDelta, in.GraphFinal.HasCumulativeIndexDelta = false, true
	if got := Plan(in); got.Role != "review" {
		t.Fatal("Index-only cumulative change was not eligible for fresh qualification", got)
	}
	in.GraphFinal.CumulativeDirectProven = true
	if got := Plan(in); got.Action != "accept" {
		t.Fatal("A fresh sufficient cumulative direct proof could not avoid an unnecessary model review", got)
	}
}

func TestGraphFinalPlanRetainsRequiredReviewCheckAndFailureGuards(t *testing.T) {
	in := graphFinalPlanFixture()
	in.Authority.Review = "required"
	in.GraphFinal.CumulativeDirectProven = true
	check := domain.CheckDefinition{ID: strings.Repeat("c", 32), Key: "required", Kind: "command", Source: "task_declaration", Required: true, DefinitionSHA256: strings.Repeat("d", 64)}
	in.Authority.Checks = []domain.CheckDefinition{check}
	in.Authority.Correction = domain.CorrectionPolicy{MaxCycles: 1, Triggers: []string{"failed_required_check"}}
	if got := Plan(in); got.Role != "review" {
		t.Fatal("Cumulative direct proof bypassed an explicitly required initial review", got)
	}
	in.Review = &domain.ReviewRecord{Outcome: "approved", Report: domain.ReviewReport{Outcome: "approved"}}
	if got := Plan(in); got.Role != "check" || got.CheckID != check.ID {
		t.Fatal("Predecessor approval replaced a required current cumulative check", got)
	}
	one := 1
	in.Checks = []domain.CheckReceipt{{DefinitionID: check.ID, DefinitionSHA256: check.DefinitionSHA256, Outcome: "failed", RootTerminal: true, InputComplete: true, OutputComplete: true, ExitCode: &one, Cleanup: domain.Cleanup{State: "verified"}}}
	if got := Plan(in); got.Role != "correction" {
		t.Fatal("A real failed cumulative check did not retain the captured correction policy", got)
	}
	in.CorrectionCyclesUsed = 1
	if got := Plan(in); got.Role != "" || got.Owner != "user" || got.Action != "correction_unavailable" {
		t.Fatal("Cumulative eligibility refilled a spent correction cycle", got)
	}
}

func TestGraphFinalPlanRejectsIncompleteUnscopedOrMismatchedEligibility(t *testing.T) {
	for _, kind := range []string{"not_file_final", "no_cumulative_delta", "missing_ref", "invalid_ref_hash", "empty_ref", "missing_candidate", "wrong_conditions", "invalid_index"} {
		t.Run(kind, func(t *testing.T) {
			in := graphFinalPlanFixture()
			switch kind {
			case "not_file_final":
				in.GraphFinal.DeclaredFileFinal = false
			case "no_cumulative_delta":
				in.GraphFinal.HasCumulativeWorkingDelta = false
			case "missing_ref":
				in.GraphFinal.VerificationBindingRef.ArtifactID = ""
			case "invalid_ref_hash":
				in.GraphFinal.VerificationBindingRef.SHA256 = "not a digest"
			case "empty_ref":
				in.GraphFinal.VerificationBindingRef.ByteLength = "0"
			case "missing_candidate":
				in.GraphFinal.CurrentSubject.CandidateID = ""
			case "wrong_conditions":
				in.GraphFinal.CurrentSubject.ConditionsSHA256 = strings.Repeat("e", 64)
			case "invalid_index":
				in.GraphFinal.CurrentSubject.IndexSHA256 = ""
			}
			if got := Plan(in); got.Action != "candidate_change_required" || got.Role != "" {
				t.Fatal("Incomplete internal final facts bypassed node-local file eligibility", got)
			}
		})
	}
}

func TestGraphFinalPlanPreservesStandaloneResponseOnlyAndAuthorOnlyPolicies(t *testing.T) {
	in := graphFinalPlanFixture()
	in.GraphFinal = nil
	if got := Plan(in); got.Action != "candidate_change_required" {
		t.Fatal("Standalone zero-delta candidate planning changed", got)
	}
	in = graphFinalPlanFixture()
	in.ResponseOnly = true
	with := Plan(in)
	in.GraphFinal = nil
	if without := Plan(in); !reflect.DeepEqual(with, without) || without.Action != "response_acceptance_pending" {
		t.Fatal("Graph final facts promoted a response into inherited file qualification", with, without)
	}
	in = graphFinalPlanFixture()
	in.Authority.OptionSnapshot.Mode = "author_only"
	if got := Plan(in); got.Action != "author_only" {
		t.Fatal("Graph final facts bypassed standalone author-only policy", got)
	}
}
