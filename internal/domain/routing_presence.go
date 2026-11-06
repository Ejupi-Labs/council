package domain

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
)

// Routing presence rules apply to wire requests only. The value validators keep
// accepting zero values for omitted options in trusted Go calls; explicit empty
// IDs/enums/counts/timestamps must not disappear through omitempty fingerprints.
func routingPresence(target any, members map[string]jsontext.Value) *Error {
	var required, nonempty []string
	switch target.(type) {
	case *StartTaskInput, *GuidanceInput:
		return routingOptionsPresence(members)
	case *RoutingPolicyInput:
		required = []string{"requestId", "expectedRevision"}
		nonempty = []string{"unknownAvailability"}
		if e := routingBudgetPresence(members, "defaultBudget"); e != nil {
			return e
		}
		if e := routingContinuationPresence(members); e != nil {
			return e
		}
	case *ContextUpdateInput:
		required = []string{"requestId", "expectedRevision"}
		nonempty = []string{"label"}
	case *ProfileUpdateInput:
		required = []string{"requestId", "expectedRevision"}
		if e := routingObjectPresence(members, "referencePrice", referencePricePresence); e != nil {
			return e
		}
	case *ReferencePriceInput:
		return referencePricePresence(members)
	case *AccountCreateInput:
		required = []string{"requestId", "label"}
		nonempty = []string{"entitlement"}
	case *AccountUpdateInput:
		required = []string{"requestId", "expectedRevision"}
		nonempty = []string{"label", "entitlement"}
	case *AccountLinkInput:
		required = []string{"requestId", "expectedRevision"}
		nonempty = []string{"declaredAccountId"}
	case *CapacityReportInput:
		required = []string{"requestId", "subject", "expectedRevision", "dimension", "state"}
		nonempty = []string{"dimension", "state", "observedAt", "retryAfter", "correctionOf", "correctionAction"}
		if e := routingObjectPresence(members, "subject", func(subject map[string]jsontext.Value) *Error {
			if e := routingRequired(subject, "kind", "id"); e != nil {
				return e
			}
			return routingNonempty(subject, "kind", "id")
		}); e != nil {
			return e
		}
	case *BudgetAmendmentInput:
		required = []string{"requestId", "action", "expectedRevision", "additionalAdmissions"}
		nonempty = []string{"action", "expectedBudgetRevision", "additionalAdmissions"}
		if e := routingOptionsPresence(members); e != nil {
			return e
		}
	default:
		return nil
	}
	if e := routingRequired(members, required...); e != nil {
		return e
	}
	if e := routingNonempty(members, required...); e != nil {
		return e
	}
	return routingNonempty(members, nonempty...)
}

// Standalone CLI price files and nested HTTP declarations must share exact
// presence rules before either path can erase optional strings during encoding.
func referencePricePresence(price map[string]jsontext.Value) *Error {
	if e := routingRequired(price, "currency", "sourceLabel", "observedAt"); e != nil {
		return e
	}
	return routingNonempty(price, "currency", "sourceLabel", "sourceUrl", "unit", "basis", "observedAt", "validUntil", "inputPerMillion", "outputPerMillion")
}

func routingRequired(members map[string]jsontext.Value, keys ...string) *Error {
	for _, key := range keys {
		if _, present := members[key]; !present {
			return Invalid("required " + key + " member")
		}
	}
	return nil
}
func routingNonempty(members map[string]jsontext.Value, keys ...string) *Error {
	for _, key := range keys {
		if value, present := members[key]; present && bytes.Equal(bytes.TrimSpace(value), []byte(`""`)) {
			return Invalid("nonempty " + key)
		}
	}
	return nil
}
func routingObjectPresence(members map[string]jsontext.Value, key string, validate func(map[string]jsontext.Value) *Error) *Error {
	value, present := members[key]
	if !present {
		return nil
	}
	var nested map[string]jsontext.Value
	if json.Unmarshal(value, &nested) != nil || value.Kind() != '{' {
		return Invalid(key + " object")
	}
	return validate(nested)
}
func routingBudgetPresence(members map[string]jsontext.Value, key string) *Error {
	return routingObjectPresence(members, key, func(budget map[string]jsontext.Value) *Error { return routingNonempty(budget, "admissionLimit") })
}
func routingContinuationPresence(members map[string]jsontext.Value) *Error {
	return routingObjectPresence(members, "continuation", func(continuation map[string]jsontext.Value) *Error {
		return routingNonempty(continuation, "maxAutomaticAdmissions")
	})
}
func routingOptionsPresence(members map[string]jsontext.Value) *Error {
	return routingObjectPresence(members, "routing", func(options map[string]jsontext.Value) *Error {
		if e := routingNonempty(options, "suitability", "targetProfileId", "expectedPolicyRevision"); e != nil {
			return e
		}
		if e := routingBudgetPresence(options, "budget"); e != nil {
			return e
		}
		return routingContinuationPresence(options)
	})
}
