package schemas

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These narrow workspace DTOs mirror internal/app's private request types.
// Owner-parity tests must cover them whenever the app request contract changes.
type openProjectInput struct {
	RequestID    string `json:"requestId"`
	SelectedPath string `json:"selectedPath"`
	BasePath     string `json:"basePath"`
}
type policyInput struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision string `json:"expectedRevision"`
	Mode             string `json:"mode"`
}
type createDraftInput struct {
	RequestID    string `json:"requestId"`
	ProjectID    string `json:"projectId"`
	OriginalText string `json:"originalText"`
}
type updateDraftInput struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision string `json:"expectedRevision"`
	OriginalText     string `json:"originalText"`
	UseCurrentPolicy *bool  `json:"useCurrentPolicy,omitzero"`
}
type stopAttemptInput struct {
	RequestID string `json:"requestId"`
}
type stopServiceInput struct {
	RequestID       string `json:"requestId"`
	OwnerInstanceID string `json:"ownerInstanceId"`
}

// validateBody preserves the exact owner decoder and then applies its pure
// validator. JSON Schema alone cannot express byte limits, graph reachability,
// authority-vector identity uniqueness, or every conditional presence rule.
func validateBody[T any](body []byte, history bool, validate func(T) *domain.Error) *domain.Error {
	var input T
	var err *domain.Error
	if history {
		err = domain.StrictDecodeHistory(body, &input)
	} else {
		err = domain.StrictDecode(body, &input)
	}
	if err != nil {
		return err
	}
	return validate(input)
}

// ValidateOwnerBody performs the owner's pure, operation-specific BODY checks.
// It does not validate every path or query identity: all effects must additionally
// pass ownerclient's fixed-route validateRequest/resolveRoute before attachment.
// The boolean is false for operations without a validator here; such operations
// must remain disabled until their validation has been added and reviewed.
// This is not an owner call, a state-dependent check, or dispatch permission.
func ValidateOwnerBody(operation, pathID string, body []byte) (*domain.Error, bool) {
	switch operation {
	case "projects.open":
		return validateBody(body, false, func(in openProjectInput) *domain.Error {
			validPath := func(s string) bool { return s != "" && utf8.ValidString(s) && !strings.ContainsRune(s, 0) }
			if !domain.ValidID(in.RequestID) || !validPath(in.SelectedPath) || !validPath(in.BasePath) || !filepath.IsAbs(in.BasePath) {
				return domain.Invalid("request identity and selectedPath with absolute basePath")
			}
			return nil
		}), true
	case "policy.set":
		return validateBody(body, false, func(in policyInput) *domain.Error {
			if !domain.ValidID(in.RequestID) {
				return domain.Invalid("requestId")
			}
			if _, err := domain.ParseRevision(in.ExpectedRevision); err != nil {
				return err
			}
			if in.Mode != "autonomous" && in.Mode != "supervised" {
				return domain.Invalid("policy mode")
			}
			return nil
		}), true
	case "drafts.create":
		return validateBody(body, false, func(in createDraftInput) *domain.Error {
			if !domain.ValidID(in.RequestID) || !domain.ValidID(in.ProjectID) {
				return domain.Invalid("request and project identities")
			}
			return domain.ValidateText(in.OriginalText)
		}), true
	case "drafts.update":
		return validateBody(body, false, func(in updateDraftInput) *domain.Error {
			if !domain.ValidID(in.RequestID) || !domain.ValidID(pathID) {
				return domain.Invalid("request and draft identities")
			}
			if _, err := domain.ParseRevision(in.ExpectedRevision); err != nil {
				return err
			}
			return domain.ValidateText(in.OriginalText)
		}), true
	case "attempts.stop":
		return validateBody(body, false, func(in stopAttemptInput) *domain.Error {
			if !domain.ValidID(in.RequestID) || !domain.ValidID(pathID) {
				return domain.Invalid("request and attempt identities")
			}
			return nil
		}), true
	case "service.stop":
		return validateBody(body, false, func(in stopServiceInput) *domain.Error {
			if !domain.ValidID(in.RequestID) || !domain.ValidID(in.OwnerInstanceID) {
				return domain.Invalid("request and owner identities")
			}
			return nil
		}), true
	case "tasks.start":
		return validateBody(body, false, domain.ValidateStartTask), true
	case "tasks.guidance":
		return validateBody(body, false, domain.ValidateGuidance), true
	case "tasks.attempts.start":
		return validateBody(body, false, domain.ValidateStartAttempt), true
	case "tasks.response.accept":
		return validateBody(body, false, domain.ValidateResponseAcceptance), true
	case "routing.policy.update":
		return validateBody(body, false, domain.ValidateRoutingPolicyInput), true
	case "routing.context.update":
		return validateBody(body, false, func(in domain.ContextUpdateInput) *domain.Error { return domain.ValidateContextUpdate(pathID, in) }), true
	case "routing.profile.update":
		return validateBody(body, false, func(in domain.ProfileUpdateInput) *domain.Error { return domain.ValidateProfileUpdate(pathID, in) }), true
	case "routing.accounts.create":
		return validateBody(body, false, domain.ValidateAccountCreate), true
	case "routing.accounts.update":
		return validateBody(body, false, func(in domain.AccountUpdateInput) *domain.Error { return domain.ValidateAccountUpdate(pathID, in) }), true
	case "routing.links.update":
		return validateBody(body, false, func(in domain.AccountLinkInput) *domain.Error { return domain.ValidateAccountLink(pathID, in) }), true
	case "routing.reports.create":
		return validateBody(body, false, domain.ValidateCapacityReport), true
	case "tasks.budget.amend":
		return validateBody(body, false, domain.ValidateBudgetAmendment), true
	case "workflow.preferences.update":
		return validateBody(body, false, domain.ValidateDeliveryPreferenceInput), true
	case "tasks.workflow.adopt":
		return validateBody(body, false, domain.ValidateWorkflowAdoptInput), true
	case "tasks.workflow.advance":
		return validateBody(body, false, domain.ValidateWorkflowAdvanceInput), true
	case "tasks.workflow.check":
		return validateBody(body, false, domain.ValidateWorkflowCheckInput), true
	case "graphs.start":
		in, err := domain.StrictDecodeGraphStart(body)
		if err != nil {
			return err, true
		}
		return domain.ValidateGraphStartInput(in), true
	case "graphs.advance":
		return validateBody(body, false, domain.ValidateGraphAdvanceInput), true
	case "graphs.stop":
		return validateBody(body, false, domain.ValidateGraphStopInput), true
	case "graphs.nodes.stop":
		return validateBody(body, false, domain.ValidateGraphNodeStopInput), true
	case "graphs.nodes.guidance":
		return validateBody(body, false, domain.ValidateGraphGuidanceInput), true
	case "graphs.budget.amend":
		return validateBody(body, false, domain.ValidateGraphBudgetAmendmentInput), true
	case "tasks.pause":
		return validateBody(body, false, domain.ValidatePauseTaskInput), true
	case "graphs.pause":
		return validateBody(body, false, domain.ValidatePauseGraphInput), true
	case "checkpoints.resume":
		return validateBody(body, false, domain.ValidateResumeCheckpointInput), true
	case "incidents.reconcile":
		return validateBody(body, false, domain.ValidateReconciliationInput), true
	case "history.snapshots.create":
		return validateBody(body, true, domain.ValidateCaptureHistoryInput), true
	case "history.exports.preview":
		return validateBody(body, true, domain.ValidatePreviewHistoryExportInput), true
	case "history.exports.create":
		return validateBody(body, true, domain.ValidateExecuteHistoryExportInput), true
	case "history.operations.reconcile":
		return validateBody(body, true, domain.ValidateReconcileHistoryOperationInput), true
	default:
		return nil, false
	}
}
