package domain

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
)

func workflowObject(raw jsontext.Value, validate func(map[string]jsontext.Value) *Error) *Error {
	var members map[string]jsontext.Value
	if raw.Kind() != '{' || json.Unmarshal(raw, &members) != nil {
		return Invalid("workflow object")
	}
	return validate(members)
}
func workflowPresence(target any, members map[string]jsontext.Value) *Error {
	var required []string
	switch target.(type) {
	case *StartTaskInput, *GuidanceInput:
		if raw, ok := members["workflow"]; ok {
			return workflowObject(raw, workflowInputPresence)
		}
	case *WorkflowInput:
		return workflowInputPresence(members)
	case *WorkflowDeliveryInput:
		return workflowDeliveryPresence(members)
	case *CheckDeclaration:
		return checkDeclarationPresence(members)
	case *DeliveryPreferenceInput:
		required = []string{"requestId", "expectedRevision", "localMode"}
	case *WorkflowAdoptInput:
		required = []string{"requestId", "expectedRevision", "taskVersion", "workflow"}
		if raw, ok := members["workflow"]; ok {
			if e := workflowObject(raw, workflowInputPresence); e != nil {
				return e
			}
		}
	case *WorkflowAdvanceInput:
		required = []string{"requestId", "expectedRevision", "workflowRevision", "taskVersion"}
	case *WorkflowCheckInput:
		required = []string{"requestId", "expectedRevision", "workflowRevision", "taskVersion", "checkId"}
	case *ReviewReport:
		return reviewReportPresence(members)
	}
	if e := routingRequired(members, required...); e != nil {
		return e
	}
	return routingNonempty(members, required...)
}
func workflowInputPresence(members map[string]jsontext.Value) *Error {
	if e := routingNonempty(members, "mode", "review"); e != nil {
		return e
	}
	if raw, ok := members["delivery"]; ok {
		if e := workflowObject(raw, workflowDeliveryPresence); e != nil {
			return e
		}
	}
	if raw, ok := members["checks"]; ok {
		var values []jsontext.Value
		if raw.Kind() != '[' || json.Unmarshal(raw, &values) != nil || len(values) > MaxWorkflowChecks {
			return Invalid("bounded check declaration array")
		}
		for _, value := range values {
			if e := workflowObject(value, checkDeclarationPresence); e != nil {
				return e
			}
		}
	}
	return nil
}
func workflowDeliveryPresence(members map[string]jsontext.Value) *Error {
	if e := routingNonempty(members, "localMode"); e != nil {
		return e
	}
	if raw, ok := members["externalAction"]; ok {
		return workflowObject(raw, func(action map[string]jsontext.Value) *Error {
			if e := routingRequired(action, "action", "destination", "acceptanceBasis", "command"); e != nil {
				return e
			}
			if e := routingNonempty(action, "action", "destination", "acceptanceBasis"); e != nil {
				return e
			}
			return workflowObject(action["command"], func(command map[string]jsontext.Value) *Error {
				return routingRequired(command, "executable", "arguments")
			})
		})
	}
	return nil
}
func checkDeclarationPresence(members map[string]jsontext.Value) *Error {
	if e := routingRequired(members, "key", "executable", "arguments"); e != nil {
		return e
	}
	if e := routingNonempty(members, "key", "executable", "relativeWorkingDirectory"); e != nil {
		return e
	}
	if raw, ok := members["scope"]; ok {
		return workflowObject(raw, func(scope map[string]jsontext.Value) *Error {
			if e := routingRequired(scope, "kind", "paths"); e != nil {
				return e
			}
			return routingNonempty(scope, "kind")
		})
	}
	return nil
}
func reviewReportPresence(m map[string]jsontext.Value) *Error {
	if e := routingRequired(m, "schemaVersion", "subject", "conditionsSHA256", "outcome", "inspectedEvidence", "findings", "gaps", "resolvedFindingIds"); e != nil {
		return e
	}
	if e := workflowObject(m["subject"], func(v map[string]jsontext.Value) *Error { return routingRequired(v, "kind", "id", "sha256") }); e != nil {
		return e
	}
	for _, spec := range []struct {
		key    string
		fields []string
	}{
		{"inspectedEvidence", []string{"ref", "scope", "paths"}},
		{"findings", []string{"key", "severity", "conditionKeys", "locations", "summary", "evidenceRefs"}},
		{"gaps", []string{"key", "conditionKeys", "reason", "evidenceRefs"}},
	} {
		var values []jsontext.Value
		if m[spec.key].Kind() != '[' || json.Unmarshal(m[spec.key], &values) != nil {
			return Invalid("review arrays")
		}
		for _, raw := range values {
			if e := workflowObject(raw, func(v map[string]jsontext.Value) *Error {
				if e := routingRequired(v, spec.fields...); e != nil {
					return e
				}
				if spec.key == "findings" {
					var locations []jsontext.Value
					if v["locations"].Kind() != '[' || json.Unmarshal(v["locations"], &locations) != nil {
						return Invalid("review locations")
					}
					for _, loc := range locations {
						if e := workflowObject(loc, func(l map[string]jsontext.Value) *Error { return routingRequired(l, "path", "startLine", "endLine") }); e != nil {
							return e
						}
					}
				}
				return nil
			}); e != nil {
				return e
			}
		}
	}
	return nil
}

// StrictDecodeCheckDeclarations is the array equivalent of StrictDecode for CLI files.
func StrictDecodeCheckDeclarations(body []byte) ([]CheckDeclaration, *Error) {
	if len(body) > MaxWorkflowBytes {
		return nil, Fail("input_too_large", "Check declarations exceed 64 KiB.")
	}
	var raw []jsontext.Value
	if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '[' || json.Unmarshal(body, &raw) != nil || len(raw) > MaxWorkflowChecks {
		return nil, Invalid("at most sixteen strict check objects")
	}
	out := make([]CheckDeclaration, 0, len(raw))
	for _, value := range raw {
		var in CheckDeclaration
		if e := StrictDecode(value, &in); e != nil {
			return nil, e
		}
		if e := ValidateCheckDeclaration(in); e != nil {
			return nil, e
		}
		out = append(out, in)
	}
	return out, nil
}
