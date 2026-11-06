// Package workflow contains bounded deterministic policy. It performs no I/O or admission.
package workflow

import (
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"slices"
	"strings"
)

type PlanInput struct {
	Authority                                                       domain.WorkflowAuthority
	ResponseOnly, HasSubject, HasDelta, HasIndexDelta, DirectProven bool
	NeedsFindingReview                                              bool
	Review                                                          *domain.ReviewRecord
	Checks                                                          []domain.CheckReceipt
	BlockingFindingIDs                                              []string
	CorrectionCyclesUsed                                            int
	StageFailure                                                    string
	LocalOutcome, ExternalOutcome                                   string
	LocalApplied, ExternalComplete                                  bool
}

type PlanDecision struct {
	Role, ExecutorKind, Action, Owner, CheckID string
	ReasonCodes                                []string
}

func Plan(in PlanInput) PlanDecision {
	decision := func(role, executor, action, owner, reason string) PlanDecision {
		return PlanDecision{Role: role, ExecutorKind: executor, Action: action, Owner: owner, ReasonCodes: []string{reason}}
	}
	block := func(action, reason string) PlanDecision { return decision("", "", action, "user", reason) }
	if in.StageFailure != "" {
		return block("stage_unavailable", in.StageFailure)
	}
	if !in.HasSubject {
		return block("evidence_unavailable", "no_complete_selected_subject")
	}
	if in.Authority.OptionSnapshot.Mode == "author_only" {
		return block("author_only", "standalone_result_has_no_automatic_workflow")
	}
	if !in.ResponseOnly && !in.HasDelta && !in.HasIndexDelta {
		return block("candidate_change_required", "no_authored_candidate_delta")
	}
	if in.ResponseOnly && (in.HasDelta || in.HasIndexDelta) {
		return block("candidate_stale", "response_only_contains_source_or_index_delta")
	}
	hasCondition := func(key string) bool {
		for _, condition := range in.Authority.Conditions {
			if condition.Key == key {
				return true
			}
		}
		return false
	}
	requireCheck := hasCondition("required-check")
	requireReview := in.Authority.Review == "required" || in.NeedsFindingReview || hasCondition("independent-review") || hasCondition("workflow-review")
	explicitChecks := false
	if declared := in.Authority.OptionSnapshot.Checks; declared != nil {
		for _, check := range *declared {
			if check.Required == nil || *check.Required {
				explicitChecks = true
			}
		}
	}
	for _, check := range in.Authority.Checks {
		if check.Required && check.Source == "task_declaration" {
			explicitChecks = true
		}
	}
	external := in.Authority.Delivery.ExternalAction != nil
	if in.ResponseOnly && hasCondition("local-integration") {
		return block("application_unsupported", "response_only_has_no_integrable_delta")
	}
	for _, gap := range in.Authority.CheckGaps {
		explicitRequired := false
		if declared := in.Authority.OptionSnapshot.Checks; declared != nil {
			for _, check := range *declared {
				if check.Key == gap.Key && (check.Required == nil || *check.Required) {
					explicitRequired = true
				}
			}
		}
		if explicitRequired || requireCheck || !in.DirectProven && !in.ResponseOnly {
			return block("check_unavailable", gap.Code)
		}
	}
	if in.ResponseOnly && !explicitChecks && !requireCheck && !requireReview && !external && !hasCondition("external-delivery") {
		return block("response_acceptance_pending", "existing_user_response_review_requires_no_model")
	}
	if in.Authority.Review == "required" && in.CorrectionCyclesUsed == 0 && in.Review == nil {
		return decision("review", "native", "review", "council", "explicit_initial_review_before_required_checks")
	}
	checks := append([]domain.CheckDefinition{}, in.Authority.Checks...)
	slices.SortFunc(checks, func(a, b domain.CheckDefinition) int {
		if a.Kind == "builtin" && b.Kind != "builtin" {
			return -1
		}
		if b.Kind == "builtin" && a.Kind != "builtin" {
			return 1
		}
		return strings.Compare(a.Key, b.Key)
	})
	receipts := map[string]domain.CheckReceipt{}
	for _, receipt := range in.Checks {
		if _, exists := receipts[receipt.DefinitionID]; exists {
			return block("evidence_unavailable", "ambiguous_current_check_receipts")
		}
		receipts[receipt.DefinitionID] = receipt
	}
	qualifyingFailure := false
	requiredCount := 0
	operationalFailure := ""
	for _, check := range checks {
		if !check.Required {
			continue
		}
		if check.Kind == "builtin" && check.Source == "recognized_convention" && !in.DirectProven {
			continue
		}
		if (in.ResponseOnly || in.DirectProven) && check.Source == "recognized_convention" && check.Kind != "builtin" && !requireCheck {
			continue
		}
		requiredCount++
		receipt, exists := receipts[check.ID]
		if !exists {
			next := decision("check", "local_check", "check", "council", "required_check_not_yet_observed")
			next.CheckID = check.ID
			return next
		}
		if receipt.DefinitionSHA256 != check.DefinitionSHA256 || receipt.SourceChanged {
			return block("candidate_stale", "check_basis_or_source_changed")
		}
		complete := receipt.Cleanup.State == "verified" && (check.Kind == "builtin" && receipt.BuiltinCompleted || check.Kind == "command" && receipt.RootTerminal && receipt.InputComplete && receipt.OutputComplete && receipt.ExitCode != nil)
		if !complete {
			operationalFailure = "check_settlement_or_evidence_incomplete"
			continue
		}
		if check.Kind == "command" {
			codes := []int{0}
			if check.Command != nil {
				codes = check.Command.SuccessExitCodes
			}
			accepted := slices.Contains(codes, *receipt.ExitCode)
			if receipt.Outcome == "passed" && !accepted || receipt.Outcome == "failed" && accepted {
				operationalFailure = "check_outcome_disagrees_with_captured_exit_criterion"
				continue
			}
		}
		if receipt.Outcome == "failed" && check.Kind == "builtin" {
			operationalFailure = "builtin_proof_is_no_longer_available"
			continue
		}
		if receipt.Outcome == "failed" {
			qualifyingFailure = true
		} else if receipt.Outcome != "passed" {
			operationalFailure = "required_check_not_successful"
		}
	}
	if requireCheck && requiredCount == 0 {
		return block("check_unavailable", "no_supported_required_check_definition")
	}
	if operationalFailure != "" {
		return block("check_unavailable", operationalFailure)
	}
	correct := func(trigger string) PlanDecision {
		if in.Authority.Correction.MaxCycles < 1 || in.CorrectionCyclesUsed >= in.Authority.Correction.MaxCycles || !slices.Contains(in.Authority.Correction.Triggers, trigger) {
			return block("correction_unavailable", "captured_correction_cycle_unavailable_or_exhausted")
		}
		return decision("correction", "native", "correction", "council", trigger)
	}
	if qualifyingFailure {
		return correct("failed_required_check")
	}
	needReview := requireReview || !in.ResponseOnly && !in.DirectProven
	if needReview {
		if in.Review == nil {
			return decision("review", "native", "review", "council", "independent_review_of_current_subject_required")
		}
		if in.Review.Outcome == "changes_required" && in.Review.Report.Outcome == "changes_required" && len(in.Review.Report.Gaps) == 0 {
			hasBlocking := false
			for _, finding := range in.Review.Report.Findings {
				hasBlocking = hasBlocking || finding.Severity == "blocking"
			}
			if hasBlocking {
				return correct("blocking_review_finding")
			}
		}
		if in.Review.Outcome != "approved" || in.Review.Report.Outcome != "approved" || len(in.Review.Report.Gaps) != 0 {
			return block("review_unavailable", "review_is_incomplete_invalid_or_nonqualifying")
		}
	}
	if len(in.BlockingFindingIDs) != 0 {
		return block("finding_resolution_required", "current_blockers_have_no_applicable_resolution")
	}
	if in.Authority.Delivery.LocalMode == "apply_verified_local" && !in.ResponseOnly && !in.LocalApplied {
		if in.LocalOutcome != "" {
			return block("delivery_unresolved", "local_operation_has_a_retained_nonapplied_outcome")
		}
		return decision("local_delivery", "local_application", "apply", "council", "captured_verified_local_scope")
	}
	if external && !in.ExternalComplete {
		if in.ExternalOutcome != "" {
			return block("effect_uncertain", "external_operation_cannot_be_automatically_reexecuted")
		}
		return decision("external_delivery", "local_external", "external_delivery", "council", "explicit_captured_external_command_scope")
	}
	if hasCondition("external-delivery") && !external {
		return block("delivery_scope_required", "required_external_evidence_has_no_supported_explicit_action")
	}
	if in.ResponseOnly {
		return block("response_acceptance_pending", "required_response_evidence_qualifies_for_existing_user_acknowledgement")
	}
	return decision("", "", "accept", "council", "current_required_verification_and_delivery_qualify")
}
