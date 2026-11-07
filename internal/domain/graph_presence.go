package domain

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
)

// Presence is checked before decoding can erase an explicit empty option. Null
// rejection and duplicate-name/unknown-member checking remain in StrictDecode.
func graphPresence(target any, members map[string]jsontext.Value) *Error {
	var required, nonempty []string
	switch target.(type) {
	case *GraphStartInput:
		// The CLI may merge absent transport IDs; ValidateGraphStartInput then
		// requires both. Explicit empty/null IDs never count as absence.
		required = []string{"nodes"}
		nonempty = []string{"requestId", "projectId", "finalNodeKey", "expectedPolicyRevision", "policyOverride", "executionMode", "maxParallelism"}
		if err := routingOptionsPresence(members); err != nil {
			return err
		}
		if raw, ok := members["delivery"]; ok {
			if err := workflowObject(raw, workflowDeliveryPresence); err != nil {
				return err
			}
		}
		if raw, ok := members["nodes"]; ok {
			var nodes []jsontext.Value
			if raw.Kind() != '[' || json.Unmarshal(raw, &nodes) != nil || len(nodes) < 1 || len(nodes) > MaxGraphNodes {
				return Invalid("one through sixteen node objects")
			}
			for _, value := range nodes {
				if err := workflowObject(value, graphNodePresence); err != nil {
					return err
				}
			}
		}
		if raw, ok := members["dependencies"]; ok {
			var edges []jsontext.Value
			if raw.Kind() != '[' || json.Unmarshal(raw, &edges) != nil || len(edges) > MaxGraphEdges {
				return Invalid("bounded dependency array")
			}
			for _, value := range edges {
				if err := workflowObject(value, func(edge map[string]jsontext.Value) *Error {
					if err := routingRequired(edge, "from", "to"); err != nil {
						return err
					}
					return routingNonempty(edge, "from", "to", "kind")
				}); err != nil {
					return err
				}
			}
		}
	case *GraphNodeInput:
		return graphNodePresence(members)
	case *GraphNodeAuthority:
		required = []string{"nodeId", "taskVersion", "nodeControlRevision"}
		nonempty = required
	case *GraphAdvanceInput:
		required = []string{"requestId", "graphVersion", "expectedGraphControlRevision", "expectedBudgetAuthorityEpoch", "expectedWindowNumber", "nodeAuthorities"}
		nonempty = []string{"requestId", "graphVersion", "expectedGraphControlRevision", "expectedBudgetAuthorityEpoch", "expectedWindowNumber"}
		if raw, ok := members["nodeAuthorities"]; ok {
			if _, err := StrictDecodeGraphNodeAuthorities(raw); err != nil {
				return err
			}
		}
	case *GraphStopInput:
		required = []string{"requestId", "expectedGraphControlRevision"}
		nonempty = required
	case *GraphNodeStopInput:
		required = []string{"requestId", "expectedGraphControlRevision", "nodeId", "expectedNodeControlRevision"}
		nonempty = required
	case *GraphGuidanceInput:
		required = []string{"requestId", "expectedGraphControlRevision", "nodeId", "expectedNodeControlRevision", "taskVersion", "expectedRevision", "guidanceText"}
		nonempty = []string{"requestId", "expectedGraphControlRevision", "nodeId", "expectedNodeControlRevision", "taskVersion", "expectedRevision"}
		if err := graphOptionsPresence(members); err != nil {
			return err
		}
		if err := executionPresence(&GuidanceInput{}, members); err != nil {
			return err
		}
	case *GraphBudgetAmendmentInput:
		required = []string{"requestId", "expectedGraphControlRevision", "expectedBudgetAuthorityEpoch", "expectedWindowNumber", "additionalAdmissions"}
		nonempty = required
	default:
		return nil
	}
	if err := routingRequired(members, required...); err != nil {
		return err
	}
	return routingNonempty(members, nonempty...)
}

func graphNodePresence(members map[string]jsontext.Value) *Error {
	if err := routingRequired(members, "key", "originalText"); err != nil {
		return err
	}
	if err := routingNonempty(members, "key", "fakeScenario"); err != nil {
		return err
	}
	if err := graphOptionsPresence(members); err != nil {
		return err
	}
	return executionPresence(&StartTaskInput{}, members)
}

func graphOptionsPresence(members map[string]jsontext.Value) *Error {
	if raw, ok := members["routing"]; ok {
		if err := workflowObject(raw, func(options map[string]jsontext.Value) *Error {
			if _, exists := options["budget"]; exists {
				return Invalid("graph-only shared budget, not node routing budget")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if raw, ok := members["workflow"]; ok {
		if err := workflowObject(raw, func(options map[string]jsontext.Value) *Error {
			if _, exists := options["delivery"]; exists {
				return Invalid("graph-level final-only delivery")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
