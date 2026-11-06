package domain

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func ParseCount(s string) (int64, *Error) {
	if s == "0" {
		return 0, nil
	}
	return ParseRevision(s)
}
func ValidSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func ValidateAdditionalText(s string) *Error {
	if !utf8.ValidString(s) {
		return Fail("invalid_encoding", "Supply valid Unicode text.")
	}
	if len(s) > MaxTextBytes {
		return Fail("input_too_large", "Additional text exceeds 4 MiB. Reduce it explicitly.")
	}
	return nil
}
func ValidatePins(p *ExecutionPins) *Error {
	if p == nil {
		return nil
	}
	if p.HarnessID != "" {
		valid := false
		for _, h := range []string{"codex", "claude-code", "deepseek-harness", "junie", "antigravity"} {
			if p.HarnessID == h {
				valid = true
			}
		}
		if !valid {
			return Invalid("harnessId")
		}
	}
	if p.Model != "" {
		if !utf8.ValidString(p.Model) || len(p.Model) > 256 {
			return Invalid("model selector")
		}
		nonspace := false
		for _, r := range p.Model {
			if unicode.IsControl(r) {
				return Invalid("model selector without control characters")
			}
			if !unicode.IsSpace(r) {
				nonspace = true
			}
		}
		if !nonspace {
			return Invalid("model selector")
		}
	}
	if p.AccountID != "" && !ValidID(p.AccountID) {
		return Invalid("accountId")
	}
	if p.PaymentPath != "" && p.PaymentPath != "native" && p.PaymentPath != "paid_api" {
		return Invalid("paymentPath")
	}
	return nil
}
func ResolveAcceptance(in *AcceptanceInput) (TaskAcceptance, *Error) {
	out := TaskAcceptance{Deliverable: "candidate", RequiredEvidence: []string{}, AllowCandidatePreparation: true}
	if in == nil {
		return out, nil
	}
	if in.Deliverable != "" {
		out.Deliverable = in.Deliverable
	}
	if out.Deliverable != "candidate" && out.Deliverable != "response_only" {
		return out, Invalid("deliverable")
	}
	if e := ValidateAdditionalText(in.ConditionsText); e != nil {
		return out, e
	}
	out.ConditionsText = in.ConditionsText
	if in.AllowCandidatePreparation != nil {
		out.AllowCandidatePreparation = *in.AllowCandidatePreparation
	}
	seen := map[string]bool{}
	for _, v := range in.RequiredEvidence {
		if seen[v] {
			return out, Invalid("unique requiredEvidence")
		}
		seen[v] = true
		switch v {
		case "check", "independent_review", "local_integration", "external_delivery":
		default:
			return out, Invalid("requiredEvidence")
		}
		out.RequiredEvidence = append(out.RequiredEvidence, v)
	}
	return out, nil
}
func ValidateExecutionMode(mode, scenario string) *Error {
	if mode != "" && mode != "native" && mode != "fake" {
		return Invalid("executionMode")
	}
	if scenario != "" {
		if mode != "fake" {
			return Invalid("fakeScenario only in fake mode")
		}
		switch scenario {
		case "response", "candidate", "partial_failure", "slow", "late_success", "owned_descendant", "cleanup_unknown",
			"trailing_observation", "malformed", "missing_result", "repeated_result", "trailing_error", "success_error", "model_usage", "empty_response", "empty_candidate":
		default:
			return Invalid("fakeScenario")
		}
	}
	return nil
}

// CheckExecutionScenario is a launch capability check, deliberately after replay.
func CheckExecutionScenario(pins ExecutionPins, mode, scenario string) *Error {
	if err := ValidateExecutionMode(mode, scenario); err != nil {
		return err
	}
	if mode != "fake" || scenario == "" {
		return nil
	}
	switch scenario {
	case "response", "candidate", "partial_failure", "slow", "late_success", "owned_descendant", "cleanup_unknown":
		return nil
	}
	connectionID, err := ResolveConnectionID(pins)
	if err != nil {
		return err
	}
	if connectionID != "claude-native" {
		return Fail("capability_unavailable", "This fixed fake scenario requires Claude Code. Select its harness explicitly or use a supported fixture.")
	}
	return nil
}
func ValidateStartTask(in StartTaskInput) *Error {
	if err := ValidateRoutingOptions(in.Routing, "start"); err != nil {
		return err
	}
	if !ValidID(in.RequestID) || !ValidID(in.ProjectID) {
		return Invalid("requestId and projectId")
	}
	if e := ValidateText(in.OriginalText); e != nil {
		return e
	}
	if in.SourceDraftRef != nil {
		if !ValidID(in.SourceDraftRef.DraftID) {
			return Invalid("sourceDraftRef.draftId")
		}
		if _, e := ParseRevision(in.SourceDraftRef.Revision); e != nil {
			return e
		}
		if in.ExpectedPolicyRevision != "" {
			return Invalid("one policy source")
		}
	}
	if in.ExpectedPolicyRevision != "" {
		if _, e := ParseRevision(in.ExpectedPolicyRevision); e != nil {
			return e
		}
	}
	if e := ValidateVersionOptions(in.Pins, in.PolicyOverride, in.Acceptance, in.TimeoutSeconds); e != nil {
		return e
	}
	return ValidateExecutionMode(in.ExecutionMode, in.FakeScenario)
}
func ValidateVersionOptions(pins *ExecutionPins, override string, acceptance *AcceptanceInput, timeout *int) *Error {
	if e := ValidatePins(pins); e != nil {
		return e
	}
	if override != "" && override != "supervised" {
		return Invalid("narrower supervised policyOverride")
	}
	if _, e := ResolveAcceptance(acceptance); e != nil {
		return e
	}
	if timeout != nil && (*timeout < 1 || *timeout > 86400) {
		return Invalid("timeoutSeconds from 1 through 86400")
	}
	return nil
}
func ValidateGuidance(in GuidanceInput) *Error {
	if err := ValidateRoutingOptions(in.Routing, "guidance"); err != nil {
		return err
	}
	if !ValidID(in.RequestID) {
		return Invalid("requestId")
	}
	if _, e := ParseRevision(in.ExpectedRevision); e != nil {
		return e
	}
	if e := ValidateAdditionalText(in.GuidanceText); e != nil {
		return e
	}
	if in.ClearPolicyOverride && in.PolicyOverride != "" {
		return Invalid("one policy override operation")
	}
	return ValidateVersionOptions(in.Pins, in.PolicyOverride, in.Acceptance, in.TimeoutSeconds)
}
func ValidateStartAttempt(in StartAttemptInput) *Error {
	if !ValidID(in.RequestID) {
		return Invalid("requestId")
	}
	if _, e := ParseRevision(in.ExpectedRevision); e != nil {
		return e
	}
	if _, e := ParseRevision(in.TaskVersion); e != nil {
		return e
	}
	return ValidateExecutionMode(in.ExecutionMode, in.FakeScenario)
}
func ValidateResponseAcceptance(in ResponseAcceptanceInput) *Error {
	if !ValidID(in.RequestID) || !ValidID(in.AttemptID) || !ValidID(in.ResponseArtifactID) {
		return Invalid("response acceptance identities")
	}
	if _, e := ParseRevision(in.ExpectedRevision); e != nil {
		return e
	}
	if _, e := ParseRevision(in.TaskVersion); e != nil {
		return e
	}
	if !ValidSHA256(in.ResponseSHA256) || in.Confirmation != "reviewed_response_and_conditions" {
		return Invalid("exact response hash and review confirmation")
	}
	return nil
}

// ValidateModelArgument applies launch-only safety without changing the accepted
// structural request syntax used by historical receipt fingerprints.
func ValidateModelArgument(model string) *Error {
	if strings.HasPrefix(strings.TrimSpace(model), "-") {
		return Fail("pin_unavailable", "The model selector resembles a native option. Supply a supported literal model selector; no launch was made.")
	}
	return nil
}
func CheckAuthoringCapability(v TaskVersion) *Error {
	if err := ValidateModelArgument(v.Pins.Model); err != nil {
		return err
	}
	if v.RequestedPolicy.Mode != "autonomous" || v.TaskPolicyOverride != nil {
		return Fail("permission_unavailable", "This connection cannot provide interactive supervised approvals. Retain the task and choose a supported policy explicitly.")
	}
	if _, err := ResolveConnectionID(v.Pins); err != nil {
		return err
	}
	if v.Pins.AccountID != "" || v.Pins.PaymentPath == "paid_api" {
		return Fail("pin_unavailable", "Explicit account selection and paid API execution are unavailable. No account or payment substitution was made.")
	}
	if !v.Acceptance.AllowCandidatePreparation && (v.Acceptance.Deliverable == "candidate" || len(v.Acceptance.RequiredEvidence) > 0) {
		return Fail("capability_unavailable", "This preview cannot provide the required final verification or delivery. Allow candidate preparation explicitly or retain the task for a later stage.")
	}
	return nil
}
func Decimal(n int64) string { return strconv.FormatInt(n, 10) }

func executionPresence(target any, members map[string]jsontext.Value) *Error {
	if e := routingPresence(target, members); e != nil {
		return e
	}
	switch target.(type) {
	case *StartTaskInput, *GuidanceInput, *StartAttemptInput, *ResponseAcceptanceInput:
	default:
		return nil
	}
	if _, ok := target.(*GuidanceInput); ok {
		if _, present := members["guidanceText"]; !present {
			return Invalid("required guidanceText member")
		}
	}
	for _, key := range []string{"expectedPolicyRevision", "executionMode", "fakeScenario", "policyOverride"} {
		if raw, ok := members[key]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte{'"', '"'}) {
			return Invalid("nonempty " + key)
		}
	}
	for _, key := range []string{"pins", "acceptance"} {
		if raw, ok := members[key]; ok {
			var nested map[string]jsontext.Value
			if json.Unmarshal(raw, &nested) != nil {
				return Invalid(key + " object")
			}
			for name, value := range nested {
				if (key == "pins" || name == "deliverable") && bytes.Equal(bytes.TrimSpace(value), []byte{'"', '"'}) {
					return Invalid("nonempty " + key + "." + name)
				}
			}
		}
	}
	return nil
}
