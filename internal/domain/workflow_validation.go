package domain

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	json "encoding/json/v2"
)

func workflowBound(value any) *Error {
	b, err := json.Marshal(value)
	if err != nil {
		return Invalid("valid workflow values")
	}
	if len(b) > MaxWorkflowBytes {
		return Fail("input_too_large", "The encoded workflow declaration exceeds 64 KiB. Reduce it explicitly.")
	}
	return nil
}
func workflowString(s string, max int) bool {
	return s != "" && len(s) <= max && utf8.ValidString(s) && strings.TrimSpace(s) != "" && !strings.ContainsRune(s, 0)
}
func WorkflowKey(s string) bool {
	if len(s) < 1 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func CanonicalWorkflowPath(s string) bool {
	if !workflowString(s, 4096) || strings.ContainsAny(s, "\\:") || strings.HasPrefix(s, "/") || path.Clean(s) != s || s == "." || s == ".." || strings.HasPrefix(s, "../") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func validateLocalMode(mode string, optional bool) *Error {
	if optional && mode == "" {
		return nil
	}
	if mode != "review_only" && mode != "apply_verified_local" {
		return Invalid("localMode review_only or apply_verified_local")
	}
	return nil
}
func ValidateWorkflowInput(in *WorkflowInput) *Error {
	if in == nil {
		return nil
	}
	if in.Mode != "" && in.Mode != "auto" && in.Mode != "author_only" {
		return Invalid("workflow mode")
	}
	if in.Review != "" && in.Review != "auto" && in.Review != "required" {
		return Invalid("workflow review")
	}
	if in.MaxCorrectionCycles != nil && (*in.MaxCorrectionCycles < 0 || *in.MaxCorrectionCycles > 1) {
		return Invalid("maxCorrectionCycles zero or one")
	}
	if in.Mode == "author_only" && (in.Delivery != nil || in.Checks != nil || in.Review != "" || in.MaxCorrectionCycles != nil) {
		return Invalid("author_only without automatic workflow declarations")
	}
	if in.Delivery != nil {
		if e := ValidateWorkflowDeliveryInput(*in.Delivery); e != nil {
			return e
		}
	}
	if in.Checks != nil {
		if len(*in.Checks) > MaxWorkflowChecks {
			return Invalid("at most sixteen checks")
		}
		seen := map[string]bool{}
		for _, check := range *in.Checks {
			if e := ValidateCheckDeclaration(check); e != nil {
				return e
			}
			if seen[check.Key] {
				return Invalid("unique check keys")
			}
			seen[check.Key] = true
		}
	}
	return workflowBound(in)
}
func ValidateWorkflowDeliveryInput(in WorkflowDeliveryInput) *Error {
	if e := validateLocalMode(in.LocalMode, true); e != nil {
		return e
	}
	if in.ExternalAction != nil {
		a := in.ExternalAction
		if !workflowString(a.Action, 1024) || !workflowString(a.Destination, 1024) {
			return Invalid("named external action and destination")
		}
		if a.AcceptanceBasis != "command_completion" {
			return Fail("external_postcondition_unsupported", "Only declared command completion is supported; remote publication proof remains unavailable.")
		}
		if e := validateWorkflowCommand(a.Command.Executable, a.Command.Arguments, a.Command.TimeoutSeconds); e != nil {
			return e
		}
	}
	return workflowBound(in)
}
func validateWorkflowCommand(executable string, args []string, timeout *int) *Error {
	if !workflowString(executable, 4096) {
		return Invalid("explicit native executable")
	}
	for _, r := range executable {
		if unicode.IsControl(r) {
			return Invalid("native executable without control characters")
		}
	}
	base := strings.ToLower(path.Base(strings.ReplaceAll(executable, "\\", "/")))
	for _, shell := range []string{"sh", "bash", "dash", "zsh", "fish", "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe"} {
		if base == shell {
			return Invalid("native executable without an intervening shell")
		}
	}
	for _, ext := range []string{".cmd", ".bat", ".ps1", ".sh"} {
		if strings.HasSuffix(base, ext) {
			return Invalid("native executable instead of shell wrapper")
		}
	}
	abs := strings.HasPrefix(executable, "/") || len(executable) >= 3 && ((executable[0] >= 'A' && executable[0] <= 'Z') || (executable[0] >= 'a' && executable[0] <= 'z')) && executable[1] == ':' && (executable[2] == '\\' || executable[2] == '/') || strings.HasPrefix(executable, `\\`)
	if !abs && executable != "go" && executable != "go.exe" && executable != "cargo" && executable != "cargo.exe" && executable != "node" && executable != "node.exe" {
		return Invalid("native absolute executable or recognized go/cargo/node")
	}
	if len(args) > 64 {
		return Invalid("at most sixty-four arguments")
	}
	total := 0
	for _, arg := range args {
		if len(arg) > 4096 || !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
			return Invalid("bounded native arguments")
		}
		total += len(arg)
	}
	if total > 16<<10 {
		return Invalid("at most sixteen KiB aggregate arguments")
	}
	if timeout != nil && (*timeout < 1 || *timeout > 1800) {
		return Invalid("workflow timeoutSeconds from one through 1800")
	}
	return nil
}
func ValidateCheckDeclaration(in CheckDeclaration) *Error {
	if !WorkflowKey(in.Key) {
		return Invalid("check key")
	}
	for _, prefix := range []string{"task-", "required-", "workflow-", "delivery-"} {
		if strings.HasPrefix(in.Key, prefix) {
			return Invalid("nonreserved check key")
		}
	}
	for _, key := range []string{"independent-review", "local-integration", "external-delivery"} {
		if in.Key == key {
			return Invalid("nonreserved check key")
		}
	}
	if e := validateWorkflowCommand(in.Executable, in.Arguments, in.TimeoutSeconds); e != nil {
		return e
	}
	if in.RelativeWorkingDirectory != "" && in.RelativeWorkingDirectory != "." && !CanonicalWorkflowPath(in.RelativeWorkingDirectory) {
		return Invalid("managed-relative working directory")
	}
	if in.Scope != nil {
		if in.Scope.Kind != "whole_candidate" && in.Scope.Kind != "declared_subset" {
			return Invalid("check scope kind")
		}
		if len(in.Scope.Paths) > 256 || in.Scope.Kind == "whole_candidate" && len(in.Scope.Paths) != 0 || in.Scope.Kind == "declared_subset" && len(in.Scope.Paths) == 0 {
			return Invalid("captured check scope paths")
		}
		seen := map[string]bool{}
		for _, p := range in.Scope.Paths {
			if !CanonicalWorkflowPath(p) || seen[p] {
				return Invalid("unique canonical check paths")
			}
			seen[p] = true
		}
	}
	if len(in.CoveredConditionKeys) > 16 {
		return Invalid("at most sixteen condition keys")
	}
	seen := map[string]bool{}
	for _, key := range in.CoveredConditionKeys {
		if !WorkflowKey(key) || seen[key] {
			return Invalid("unique condition keys")
		}
		seen[key] = true
	}
	if in.SuccessExitCodes != nil {
		if len(*in.SuccessExitCodes) < 1 || len(*in.SuccessExitCodes) > 16 {
			return Invalid("one through sixteen success exit codes")
		}
		codes := map[int]bool{}
		for _, code := range *in.SuccessExitCodes {
			if code < 0 || code > 255 || codes[code] {
				return Invalid("unique success exit codes zero through 255")
			}
			codes[code] = true
		}
	}
	return workflowBound(in)
}
func ValidateDeliveryPreferenceInput(in DeliveryPreferenceInput) *Error {
	if !ValidID(in.RequestID) {
		return Invalid("requestId")
	}
	if _, e := ParseRevision(in.ExpectedRevision); e != nil {
		return e
	}
	return validateLocalMode(in.LocalMode, false)
}
func ValidateWorkflowAdoptInput(in WorkflowAdoptInput) *Error {
	if !ValidID(in.RequestID) || in.Workflow == nil {
		return Invalid("requestId and explicit workflow authority")
	}
	if _, e := ParseRevision(in.ExpectedRevision); e != nil {
		return e
	}
	if _, e := ParseRevision(in.TaskVersion); e != nil {
		return e
	}
	if in.Workflow.Mode == "author_only" {
		return Invalid("workflow adoption into Auto")
	}
	return ValidateWorkflowInput(in.Workflow)
}
func ValidateWorkflowAdvanceInput(in WorkflowAdvanceInput) *Error {
	if !ValidID(in.RequestID) {
		return Invalid("requestId")
	}
	for _, rev := range []string{in.ExpectedRevision, in.WorkflowRevision, in.TaskVersion} {
		if _, e := ParseRevision(rev); e != nil {
			return e
		}
	}
	return nil
}
func ValidateWorkflowCheckInput(in WorkflowCheckInput) *Error {
	if !ValidID(in.CheckID) {
		return Invalid("checkId")
	}
	return ValidateWorkflowAdvanceInput(WorkflowAdvanceInput{RequestID: in.RequestID, ExpectedRevision: in.ExpectedRevision, WorkflowRevision: in.WorkflowRevision, TaskVersion: in.TaskVersion})
}
func ValidateWorkflowSelector(in WorkflowSelector) *Error {
	if in.WorkflowID != "" && in.TaskVersion != "" {
		return Invalid("one historical workflow selector")
	}
	if in.WorkflowID != "" && !ValidID(in.WorkflowID) {
		return Invalid("workflowId")
	}
	if in.TaskVersion != "" {
		if _, e := ParseRevision(in.TaskVersion); e != nil {
			return e
		}
	}
	return nil
}
