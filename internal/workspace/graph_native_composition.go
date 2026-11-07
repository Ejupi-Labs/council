package workspace

import (
	"context"
	"reflect"
)

// GraphNativeInputRetainRequest leaves filesystem case behavior to the actual
// private assembly directory, never the controller's operating-system guess.
type GraphNativeInputRetainRequest struct {
	Base          GraphInputRetainRequest
	Contributions []GraphContributionSource
}

func (m *Manager) RetainNativeGraphInput(ctx context.Context, request GraphNativeInputRetainRequest) (GraphComposedInputRetention, error) {
	return m.retainGraphComposedInput(ctx, GraphComposedInputRetainRequest{Base: request.Base, Contributions: request.Contributions}, true)
}

type graphNativeCompositionPolicy struct {
	sensitivePlan, insensitivePlan           CompositionPlan
	sensitiveConflicts, insensitiveConflicts []InputConflict
	probe                                    bool
}

// Native namespace policy never changes causal reduction or payloads. The
// reducer returns input_conflict before its case-dependent namespace phase.
func graphNativeCompositionPolicies(request GraphCompositionRequest) (graphNativeCompositionPolicy, error) {
	var result graphNativeCompositionPolicy
	request.CaseSensitive = true
	var err error
	result.sensitivePlan, result.sensitiveConflicts, err = ComposeGraphInput(request)
	if err != nil {
		return result, err
	}
	request.CaseSensitive = false
	result.insensitivePlan, result.insensitiveConflicts, err = ComposeGraphInput(request)
	if err != nil {
		return result, err
	}
	if len(result.sensitiveConflicts) == 0 && len(result.insensitiveConflicts) == 0 &&
		reflect.DeepEqual(result.sensitivePlan, result.insensitivePlan) {
		return result, nil
	}
	sameCausalConflict := len(result.sensitiveConflicts) != 0 &&
		reflect.DeepEqual(result.sensitiveConflicts, result.insensitiveConflicts)
	for _, conflict := range result.sensitiveConflicts {
		sameCausalConflict = sameCausalConflict && conflict.Code == "input_conflict"
	}
	result.probe = !sameCausalConflict
	return result, nil
}

func (p graphNativeCompositionPolicy) selectCase(sensitive bool) (CompositionPlan, []InputConflict) {
	if sensitive {
		return p.sensitivePlan, p.sensitiveConflicts
	}
	return p.insensitivePlan, p.insensitiveConflicts
}
