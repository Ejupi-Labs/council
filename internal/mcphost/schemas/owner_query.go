package schemas

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

// A new fixed route is not automatically an approved read validator.
var reviewedReadOperations = map[string]struct{}{
	"agents.list":                    {},
	"artifacts.bytes":                {},
	"artifacts.get":                  {},
	"attempts.artifacts.list":        {},
	"attempts.events.list":           {},
	"attempts.get":                   {},
	"attempts.wait":                  {},
	"candidates.changes.list":        {},
	"checkpoints.get":                {},
	"drafts.get":                     {},
	"drafts.list":                    {},
	"execution.capabilities":         {},
	"graphs.budget.get":              {},
	"graphs.budget.ledger":           {},
	"graphs.definition.get":          {},
	"graphs.dispatches.get":          {},
	"graphs.dispatches.list":         {},
	"graphs.get":                     {},
	"graphs.inputs.get":              {},
	"graphs.inputs.list":             {},
	"graphs.inputs.members":          {},
	"graphs.list":                    {},
	"graphs.wait":                    {},
	"history.operations.get":         {},
	"history.snapshots.get":          {},
	"history.snapshots.records":      {},
	"incidents.get":                  {},
	"meta":                           {},
	"policy.get":                     {},
	"projects.get":                   {},
	"projects.list":                  {},
	"routing.accounts.list":          {},
	"routing.capacity.get":           {},
	"routing.contexts.list":          {},
	"routing.policy.get":             {},
	"routing.profiles.list":          {},
	"routing.reports.get":            {},
	"routing.reports.list":           {},
	"tasks.attempts.list":            {},
	"tasks.budget.get":               {},
	"tasks.budget.ledger.list":       {},
	"tasks.get":                      {},
	"tasks.list":                     {},
	"tasks.routing.decisions.list":   {},
	"tasks.routing.get":              {},
	"tasks.usage.list":               {},
	"tasks.version.get":              {},
	"tasks.workflow.candidates.list": {},
	"tasks.workflow.checks.list":     {},
	"tasks.workflow.deliveries.list": {},
	"tasks.workflow.findings.list":   {},
	"tasks.workflow.get":             {},
	"tasks.workflow.reviews.list":    {},
	"tasks.workflow.steps.list":      {},
	"workflow.preferences.get":       {},
}

// ValidateOwnerQuery performs stateless path and query validation for reviewed
// fixed GET operations. pathIDs are in Route.PathKinds order. The bool is false
// for a non-GET, unknown or not-yet-reviewed operation; callers must fail
// closed. This is not a schema approval, owner call, or authorization to read.
// Duplicate JSON members must already have been rejected by the raw decoder.
// Cursor authentication, binding, retained ranges and owner state remain owner
// responsibilities and cannot be established without reading retained state.
func ValidateOwnerQuery(operation string, pathIDs []string, query map[string]string) (*domain.Error, bool) {
	if _, ok := reviewedReadOperations[operation]; !ok {
		return nil, false
	}
	route, ok := ownerclient.RouteFor(operation)
	if !ok || route.Method != "GET" {
		return nil, false
	}
	if len(pathIDs) != len(route.PathKinds) {
		return domain.Invalid("exact read path identities"), true
	}
	for index, kind := range route.PathKinds {
		id := pathIDs[index]
		var valid bool
		switch kind {
		case "id":
			valid = domain.ValidID(id)
		case "revision":
			_, err := domain.ParseRevision(id)
			valid = err == nil
		case "graphkey":
			valid = domain.GraphKey(id)
		case "context":
			valid = domain.ValidContextID(id)
		case "profile":
			valid = domain.ValidProfileID(id)
		default:
			return nil, false
		}
		if !valid {
			return domain.Invalid("exact read path identity"), true
		}
	}
	if invalid := domain.ValidateQuery(query, route.QueryKeys...); invalid != nil {
		return invalid, true
	}
	for key, value := range query {
		if value == "" {
			return domain.Invalid("nonempty read query values"), true
		}
		switch key {
		case "projectId", "accountId", "inputId", "workflowId":
			if !domain.ValidID(value) {
				return domain.Invalid(key), true
			}
		case "contextId":
			if !domain.ValidContextID(value) {
				return domain.Invalid(key), true
			}
		case "nodeKey":
			if !domain.GraphKey(value) {
				return domain.Invalid(key), true
			}
		case "revision", "taskVersion":
			if _, err := domain.ParseRevision(value); err != nil {
				return err, true
			}
		case "afterRevision", "offset", "through":
			if _, err := domain.ParseCount(value); err != nil {
				return err, true
			}
		case "timeoutMs":
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || n > 30000 || strconv.Itoa(n) != value {
				return domain.Invalid("timeoutMs from 0 through 30000"), true
			}
		case "limit":
			if operation == "artifacts.bytes" {
				n, err := strconv.Atoi(value)
				if err != nil || n < 1 || n > evidence.ChunkLimit || strconv.Itoa(n) != value {
					return domain.Invalid("byte limit from 1 through " + strconv.Itoa(evidence.ChunkLimit)), true
				}
			} else if _, err := domain.ParseLimit(value); err != nil {
				return err, true
			}
		case "cursor":
			if !validReadCursorFormat(operation, value) {
				return domain.Fail("invalid_cursor", "Supply a complete canonical cursor for this read."), true
			}
		case "kind":
			if operation != "history.snapshots.records" || (value != "tasks" && value != "attempts") {
				return domain.Invalid("history record kind"), true
			}
		default:
			// Fail closed if the owner route adds a query we have not reviewed.
			return nil, false
		}
	}
	if operation == "history.snapshots.records" {
		if query["kind"] != "tasks" && query["kind"] != "attempts" {
			return domain.Invalid("history record kind"), true
		}
	}
	if operation == "routing.capacity.get" || operation == "routing.reports.list" {
		_, context := query["contextId"]
		_, account := query["accountId"]
		if context == account {
			return domain.Invalid("exactly one capacity selector"), true
		}
	}
	if strings.HasPrefix(operation, "tasks.workflow.") {
		selector := domain.WorkflowSelector{WorkflowID: query["workflowId"], TaskVersion: query["taskVersion"]}
		if invalid := domain.ValidateWorkflowSelector(selector); invalid != nil {
			return invalid, true
		}
	}
	if operation == "artifacts.bytes" && query["through"] != "" {
		from := int64(0)
		if query["offset"] != "" {
			from, _ = domain.ParseCount(query["offset"])
		}
		through, _ := domain.ParseCount(query["through"])
		if from > through {
			return domain.Invalid("byte range order"), true
		}
	}
	return nil, true
}

func validReadCursorFormat(operation, raw string) bool {
	limit := 2048
	parts := []string{raw}
	if operation == "history.snapshots.records" {
		limit = domain.HistoryMaxCursorBytes
		parts = strings.Split(raw, ".")
		if len(parts) != 2 {
			return false
		}
	}
	if len(raw) == 0 || len(raw) > limit {
		return false
	}
	for index, part := range parts {
		if part == "" {
			return false
		}
		decoded, err := base64.RawURLEncoding.Strict().DecodeString(part)
		if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != part {
			return false
		}
		if operation == "history.snapshots.records" && index == 1 && len(decoded) != 32 {
			return false
		}
	}
	return true
}
