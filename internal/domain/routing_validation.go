package domain

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func ValidContextID(id string) bool { return id == "codex-native" || id == "claude-native" }
func ValidProfileID(id string) bool {
	switch id {
	case "codex-native-default", "codex-luna-economy", "codex-sol-general", "claude-native-default", "claude-haiku-economy", "claude-sonnet-general":
		return true
	}
	return false
}
func (s CapacitySubject) ScopeKey() string { return s.Kind + ":" + s.ID }
func ValidateCapacitySubject(s CapacitySubject) *Error {
	if s.Kind == "context" && ValidContextID(s.ID) || s.Kind == "account" && ValidID(s.ID) {
		return nil
	}
	return Invalid("capacity subject kind and identity")
}
func ParseRoutingTime(value string) (time.Time, *Error) {
	if len(value) == 0 || len(value) > 64 || !strings.HasSuffix(value, "Z") {
		return time.Time{}, Invalid("UTC timestamp ending in Z")
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, Invalid("UTC timestamp ending in Z")
	}
	return t, nil
}
func ValidRoutingDecimal(value string) bool {
	if len(value) == 0 || len(value) > 64 || value[0] == '-' || !bytes.Equal([]byte(value), bytes.TrimSpace([]byte(value))) {
		return false
	}
	var raw jsontext.Value
	return json.Unmarshal([]byte(value), &raw) == nil && raw.Kind() == '0'
}
func AddRoutingCounts(a, b string) (string, *Error) {
	x, e := ParseCount(a)
	if e != nil {
		return "", e
	}
	y, e := ParseCount(b)
	if e != nil {
		return "", e
	}
	if x > math.MaxInt64-y {
		return "", Fail("budget_conflict", "The cumulative admission count cannot be represented exactly. Preserve history and inspect this task.")
	}
	return strconv.FormatInt(x+y, 10), nil
}
func SubtractRoutingCounts(a, b string) (string, *Error) {
	x, e := ParseCount(a)
	if e != nil {
		return "", e
	}
	y, e := ParseCount(b)
	if e != nil {
		return "", e
	}
	if y > x {
		return "", Fail("budget_conflict", "Consumed admissions exceed the declared authority. Preserve this task and inspect its ledger.")
	}
	return strconv.FormatInt(x-y, 10), nil
}
func boundedRoutingText(s string, max int, nonempty, singleLine bool) *Error {
	if !utf8.ValidString(s) || len(s) > max || (nonempty && strings.TrimSpace(s) == "") {
		return Invalid("bounded routing text")
	}
	if singleLine {
		for _, r := range s {
			if unicode.IsControl(r) {
				return Invalid("single-line routing label")
			}
		}
	}
	return nil
}
func routingIdentity(requestID, revision string) *Error {
	if !ValidID(requestID) {
		return Invalid("requestId")
	}
	_, e := ParseRevision(revision)
	return e
}
func validateBudgetInput(b *RoutingBudgetInput) *Error {
	if b == nil {
		return nil
	}
	if b.AdmissionLimit != "" {
		n, e := ParseCount(b.AdmissionLimit)
		if e != nil || n < 1 || n > 16 {
			return Invalid("admissionLimit from 1 through 16 as a decimal string")
		}
	}
	if b.WindowSeconds != nil && (*b.WindowSeconds < 1 || *b.WindowSeconds > 86400) {
		return Invalid("windowSeconds from 1 through 86400")
	}
	return nil
}
func ValidateContinuationInput(in *ContinuationInput) *Error {
	if in == nil {
		return nil
	}
	if in.MaxAutomaticAdmissions != "" {
		n, e := ParseCount(in.MaxAutomaticAdmissions)
		if e != nil || n > 15 {
			return Invalid("maxAutomaticAdmissions from 0 through 15 as a decimal string")
		}
	}
	if in.Categories != nil {
		seen := map[string]bool{}
		for _, c := range *in.Categories {
			if seen[c] || (c != "pre_launch_ineligible" && c != "terminal_transient") {
				return Invalid("unique recognized continuation categories")
			}
			seen[c] = true
		}
	}
	return nil
}
func ResolveContinuation(base ContinuationPolicy, in *ContinuationInput) (ContinuationPolicy, *Error) {
	if e := ValidateContinuationInput(in); e != nil {
		return base, e
	}
	if base.MaxAutomaticAdmissions == "" {
		base.MaxAutomaticAdmissions = "0"
	}
	if base.Categories == nil {
		base.Categories = []string{}
	}
	if in != nil {
		if in.MaxAutomaticAdmissions != "" {
			base.MaxAutomaticAdmissions = in.MaxAutomaticAdmissions
		}
		if in.Categories != nil {
			base.Categories = append([]string{}, (*in.Categories)...)
		}
		if in.AllowReplacement != nil {
			base.AllowReplacement = *in.AllowReplacement
		}
		if in.AllowEscalation != nil {
			base.AllowEscalation = *in.AllowEscalation
		}
	}
	n, e := ParseCount(base.MaxAutomaticAdmissions)
	if e != nil || n > 15 {
		return base, Invalid("bounded automatic admissions")
	}
	if n > 0 && len(base.Categories) == 0 {
		return base, Invalid("a recognized category for enabled continuation")
	}
	return base, nil
}
func CheckContinuationCapability(p ContinuationPolicy) *Error {
	n, e := ParseCount(p.MaxAutomaticAdmissions)
	if e != nil {
		return e
	}
	if n > 0 {
		for _, c := range p.Categories {
			if c == "terminal_transient" {
				return Fail("capability_unavailable", "The accepted native profiles do not qualify a structured terminal-transient continuation. Use supported pre-launch policy or keep continuation disabled.")
			}
		}
	}
	return nil
}
func ValidateRoutingOptions(in *RoutingOptions, mode string) *Error {
	if in == nil {
		return nil
	}
	if in.Suitability != "" && in.Suitability != "unknown" && in.Suitability != "routine" && in.Suitability != "general" {
		return Invalid("routing suitability")
	}
	if in.TargetProfileID != "" && !ValidProfileID(in.TargetProfileID) {
		return Invalid("targetProfileId")
	}
	if in.ExpectedPolicyRevision != "" {
		if _, e := ParseRevision(in.ExpectedPolicyRevision); e != nil {
			return e
		}
		if mode == "guidance" && (in.UseCurrentPolicy == nil || !*in.UseCurrentPolicy) {
			return Invalid("useCurrentPolicy for a guidance routing-policy revision")
		}
	}
	if mode != "guidance" && in.UseCurrentPolicy != nil {
		return Invalid("useCurrentPolicy only in routing guidance")
	}
	if mode != "start" && in.Budget != nil {
		return Invalid("explicit budget amendment instead of routing budget in guidance or adoption")
	}
	if e := validateBudgetInput(in.Budget); e != nil {
		return e
	}
	return ValidateContinuationInput(in.Continuation)
}
func ValidateRoutingPolicyInput(in RoutingPolicyInput) *Error {
	if e := routingIdentity(in.RequestID, in.ExpectedRevision); e != nil {
		return e
	}
	if in.UnknownAvailability != "" && in.UnknownAvailability != "allow" && in.UnknownAvailability != "require_positive" {
		return Invalid("unknownAvailability")
	}
	if in.EconomyOrder != nil {
		if len(*in.EconomyOrder) > 6 {
			return Invalid("bounded economyOrder")
		}
		seen := map[string]bool{}
		for _, id := range *in.EconomyOrder {
			if !ValidProfileID(id) || seen[id] {
				return Invalid("unique fixed profile order")
			}
			seen[id] = true
		}
	}
	if e := validateBudgetInput(in.DefaultBudget); e != nil {
		return e
	}
	return ValidateContinuationInput(in.Continuation)
}
func ValidateContextUpdate(id string, in ContextUpdateInput) *Error {
	if !ValidContextID(id) {
		return Invalid("native context identity")
	}
	if e := routingIdentity(in.RequestID, in.ExpectedRevision); e != nil {
		return e
	}
	if in.Label != nil {
		return boundedRoutingText(*in.Label, 128, true, true)
	}
	return nil
}
func ValidateReferencePrice(in *ReferencePriceInput) *Error {
	if in == nil {
		return nil
	}
	if len(in.Currency) != 3 || strings.Trim(in.Currency, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return Invalid("three-letter reference currency")
	}
	if in.Unit != "" && in.Unit != "per_million_tokens" || in.Basis != "" && in.Basis != "api_list" && in.Basis != "declared_marginal" {
		return Invalid("reference unit and basis")
	}
	for _, v := range []*string{in.InputPerMillion, in.OutputPerMillion} {
		if v != nil && !ValidRoutingDecimal(*v) {
			return Invalid("bounded exact nonnegative reference decimal")
		}
	}
	if e := boundedRoutingText(in.SourceLabel, 128, true, true); e != nil {
		return e
	}
	if e := boundedRoutingText(in.SourceURL, 1024, false, true); e != nil {
		return e
	}
	if _, e := ParseRoutingTime(in.ObservedAt); e != nil {
		return e
	}
	if in.ValidUntil != "" {
		if _, e := ParseRoutingTime(in.ValidUntil); e != nil {
			return e
		}
	}
	return nil
}
func ValidateProfileUpdate(id string, in ProfileUpdateInput) *Error {
	if !ValidProfileID(id) {
		return Invalid("fixed target profile identity")
	}
	if e := routingIdentity(in.RequestID, in.ExpectedRevision); e != nil {
		return e
	}
	if in.ClearReferencePrice != nil && (!*in.ClearReferencePrice || in.ReferencePrice != nil) {
		return Invalid("referencePrice or clearReferencePrice true")
	}
	return ValidateReferencePrice(in.ReferencePrice)
}
func ValidateAccountCreate(in AccountCreateInput) *Error {
	if !ValidID(in.RequestID) {
		return Invalid("requestId")
	}
	if e := boundedRoutingText(in.Label, 128, true, true); e != nil {
		return e
	}
	if e := boundedRoutingText(in.ProviderLabel, 128, false, true); e != nil {
		return e
	}
	return validateEntitlement(in.Entitlement)
}
func validateEntitlement(s string) *Error {
	if s != "" && s != "unknown" && s != "subscription" && s != "local" {
		return Invalid("declared entitlement")
	}
	return nil
}
func ValidateAccountUpdate(id string, in AccountUpdateInput) *Error {
	if !ValidID(id) {
		return Invalid("declared account identity")
	}
	if e := routingIdentity(in.RequestID, in.ExpectedRevision); e != nil {
		return e
	}
	if in.Label != nil {
		if e := boundedRoutingText(*in.Label, 128, true, true); e != nil {
			return e
		}
	}
	if in.ProviderLabel != nil {
		if e := boundedRoutingText(*in.ProviderLabel, 128, false, true); e != nil {
			return e
		}
	}
	return validateEntitlement(in.Entitlement)
}
func ValidateAccountLink(id string, in AccountLinkInput) *Error {
	if !ValidContextID(id) {
		return Invalid("native context identity")
	}
	if e := routingIdentity(in.RequestID, in.ExpectedRevision); e != nil {
		return e
	}
	unlink := in.Unlink != nil && *in.Unlink
	if in.Unlink != nil && !*in.Unlink {
		return Invalid("unlink true or declaredAccountId")
	}
	if unlink == (in.DeclaredAccountID != "") {
		return Invalid("exactly one declaredAccountId or unlink true")
	}
	if in.DeclaredAccountID != "" && !ValidID(in.DeclaredAccountID) {
		return Invalid("declaredAccountId")
	}
	return nil
}
func ValidateCapacityReport(in CapacityReportInput) *Error {
	if e := routingIdentity(in.RequestID, in.ExpectedRevision); e != nil {
		return e
	}
	if e := ValidateCapacitySubject(in.Subject); e != nil {
		return e
	}
	valid := false
	switch in.Dimension {
	case "occupancy":
		valid = in.State == "busy" || in.State == "available" || in.State == "unknown"
	case "allowance":
		valid = in.State == "available" || in.State == "exhausted" || in.State == "unknown"
	case "service":
		switch in.State {
		case "temporary_throttle", "temporary_overload", "authentication_denied", "model_denied", "permission_denied", "unknown":
			valid = true
		}
	}
	if !valid {
		return Invalid("capacity dimension and state")
	}
	if in.ValidForSeconds != nil && (*in.ValidForSeconds < 1 || *in.ValidForSeconds > 86400) {
		return Invalid("validForSeconds from 1 through 86400")
	}
	if in.ObservedAt != "" {
		if _, e := ParseRoutingTime(in.ObservedAt); e != nil {
			return e
		}
	}
	if in.RetryAfter != "" {
		if in.Dimension != "service" || (in.State != "temporary_throttle" && in.State != "temporary_overload") {
			return Invalid("retryAfter for temporary service report")
		}
		if _, e := ParseRoutingTime(in.RetryAfter); e != nil {
			return e
		}
	}
	if (in.CorrectionOf == "") != (in.CorrectionAction == "") {
		return Invalid("correction identity and action together")
	}
	if in.CorrectionOf != "" {
		if !ValidID(in.CorrectionOf) || (in.CorrectionAction != "recover" && in.CorrectionAction != "withdraw") {
			return Invalid("correction identity and action")
		}
		if in.CorrectionAction == "withdraw" && in.State != "unknown" {
			return Invalid("unknown state when withdrawing a report")
		}
	}
	return boundedRoutingText(in.Note, 512, false, false)
}
func ValidateBudgetAmendment(in BudgetAmendmentInput) *Error {
	if e := routingIdentity(in.RequestID, in.ExpectedRevision); e != nil {
		return e
	}
	n, e := ParseCount(in.AdditionalAdmissions)
	if e != nil || n < 1 || n > 16 {
		return Invalid("additionalAdmissions from 1 through 16 as a decimal string")
	}
	if in.NewWindowSeconds != nil && (*in.NewWindowSeconds < 1 || *in.NewWindowSeconds > 86400) {
		return Invalid("newWindowSeconds from 1 through 86400")
	}
	switch in.Action {
	case "grant":
		if _, e := ParseRevision(in.ExpectedBudgetRevision); e != nil {
			return e
		}
		if in.Routing != nil {
			return Invalid("routing authority only with explicit adoption or guidance")
		}
	case "adopt":
		if in.ExpectedBudgetRevision != "" || in.NewWindowSeconds == nil {
			return Invalid("new window and no existing budget revision for adoption")
		}
		return ValidateRoutingOptions(in.Routing, "adopt")
	default:
		return Invalid("budget amendment action")
	}
	return nil
}
