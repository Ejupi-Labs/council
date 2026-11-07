package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Graph content has its own sealed source basis. Reusing the standalone live
// project recognizer would inspect a different source after upstream edits.
// Keep suitability unknown unless the user supplied a declared quality hint;
// ordinary economical ranking still chooses among suitable configured targets.
func (e *Engine) graphRoutingFacts(ctx context.Context, definition domain.GraphDefinition, options domain.GraphNodeExecutionOptions) (domain.RoutingFacts, error) {
	facts := domain.RoutingFacts{Assessment: domain.SuitabilityAssessment{Classification: "unknown", Source: "unknown", ReasonCodes: []string{"sealed_graph_input_without_supported_routing_proof"}}}
	declared := ""
	if options.RoutingBasis != nil {
		// This choice was frozen at guidance/rebind time. Reading it never
		// replays a saved UseCurrentPolicy action against current preferences.
		declared = options.RoutingBasis.OptionSnapshot.Suitability
	} else {
		if options.TaskVersion != "1" {
			return facts, domain.Fail("evidence_unavailable", "This graph version lacks its captured routing policy basis.")
		}
		if definition.Original.Routing != nil {
			declared = definition.Original.Routing.Suitability
		}
		if options.Routing != nil && options.Routing.Suitability != "" {
			declared = options.Routing.Suitability
		}
	}
	if declared != "" && declared != "unknown" {
		if declared != "routine" && declared != "general" {
			return facts, domain.Invalid("suitability declaration")
		}
		facts.Assessment = domain.SuitabilityAssessment{Classification: declared, Source: "user_declared", ReasonCodes: []string{"explicit_user_hint_not_quality_proof"}}
	}
	var err error
	facts.Contexts, err = e.routingContexts(ctx, options.ExecutionMode)
	return facts, err
}
