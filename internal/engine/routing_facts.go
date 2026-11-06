package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/discovery"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/project"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
)

// routingContexts reads installation metadata only. It never opens native login
// state, invokes a vendor process or treats Council inactivity as external idle.
func (e *Engine) routingContexts(ctx context.Context, mode string) ([]domain.NativeContextFact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	contexts := []struct{ id, harness string }{{"codex-native", "codex"}, {"claude-native", "claude-code"}}
	facts := make([]domain.NativeContextFact, 0, len(contexts))
	stamp := domain.Now()
	if mode == "fake" {
		for _, context := range contexts {
			facts = append(facts, domain.NativeContextFact{ContextID: context.id, InstallationStatus: "installed", ObservedAt: stamp, Source: "local_fixture"})
		}
		return facts, nil
	}
	if e.config.Resolve != nil {
		for _, context := range contexts {
			fact := domain.NativeContextFact{ContextID: context.id, InstallationStatus: "unknown", ObservedAt: stamp, Source: "configured_resolver"}
			if _, err := e.config.Resolve(ctx, domain.ExecutionPins{HarnessID: context.harness}); err == nil {
				fact.InstallationStatus = "installed"
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			facts = append(facts, fact)
		}
		return facts, nil
	}
	observed, err := discovery.Discover(ctx)
	if err != nil {
		return nil, err
	}
	for _, context := range contexts {
		fact := domain.NativeContextFact{ContextID: context.id, InstallationStatus: "unknown", ObservedAt: observed.ObservedAt, Source: "installation_metadata_unavailable"}
		for _, entry := range observed.Observations {
			if entry.HarnessID == context.harness && entry.Error == nil && entry.Observation != nil {
				fact.InstallationStatus = entry.Observation.InstallationStatus
				fact.Source = entry.Observation.Source
				fact.ObservedAt = entry.Observation.ObservedAt
				break
			}
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

// routingFactsForVersion gathers source evidence outside the owner's SQL
// transaction. Transactional revision checks still decide which version wins.
func (e *Engine) routingFactsForVersion(ctx context.Context, projectID, original string, version domain.TaskVersion, declared, mode string) (domain.RoutingFacts, error) {
	var facts domain.RoutingFacts
	p, identity, err := e.db.Project(ctx, projectID)
	if err != nil {
		return facts, err
	}
	if err := project.ValidateIdentity(p.ResolvedPath, identity); err != nil {
		return facts, domain.Fail("project_identity_changed", "The saved project identity changed. Restore its original folder before admitting work.")
	}
	assessment, assessmentErr := routing.Assess(ctx, p.ResolvedPath, original, version, declared)
	if assessmentErr != nil {
		return facts, assessmentErr
	}
	facts.Assessment = assessment
	facts.Contexts, err = e.routingContexts(ctx, mode)
	return facts, err
}

func (e *Engine) startRoutingFacts(ctx context.Context, in domain.StartTaskInput) (domain.RoutingFacts, error) {
	acceptance, err := domain.ResolveAcceptance(in.Acceptance)
	if err != nil {
		return domain.RoutingFacts{}, err
	}
	declared := ""
	if in.Routing != nil {
		declared = in.Routing.Suitability
	}
	return e.routingFactsForVersion(ctx, in.ProjectID, in.OriginalText, domain.TaskVersion{Acceptance: acceptance}, declared, in.ExecutionMode)
}

// guardRouting may durably settle a known pre-launch eligibility block. Its
// caller must return without preparing a process when stopped is true.
func (e *Engine) guardRouting(ctx context.Context, a domain.Attempt, assessment domain.SuitabilityAssessment) (stopped bool, err error) {
	contexts, err := e.routingContexts(ctx, a.ExecutionMode)
	if err != nil {
		return false, err
	}
	e.control.Lock()
	current, err := e.db.DispatchRouting(ctx, a.ID, domain.RoutingFacts{Assessment: assessment, Contexts: contexts})
	if persistenceFault(err) {
		e.fenceLocked(a.ID, "routing_persistence_uncertain", "The dispatch eligibility guard could not be acknowledged durably. No replacement or replay is permitted.")
	}
	e.control.Unlock()
	return current.Phase == "settled", err
}

func (e *Engine) frozenRoutingFacts(ctx context.Context, task domain.Task, authority domain.TaskRoutingAuthority, mode string) (domain.RoutingFacts, error) {
	facts := domain.RoutingFacts{Assessment: authority.SuitabilityAssessment}
	p, identity, err := e.db.Project(ctx, task.ProjectID)
	if err != nil {
		return facts, err
	}
	if err := project.ValidateIdentity(p.ResolvedPath, identity); err != nil {
		return facts, domain.Fail("project_identity_changed", "Restore the saved project identity before starting new work.")
	}
	if err := routing.RecheckOriginal(ctx, p.ResolvedPath, authority.SuitabilityAssessment); err != nil {
		return facts, err
	}
	facts.Contexts, err = e.routingContexts(ctx, mode)
	return facts, err
}

// Guidance creates a new attributable assessment, but editing instructions does
// not require the project to be currently available and never starts a worker.
func (e *Engine) guidanceRoutingFacts(ctx context.Context, detail domain.TaskDetail, in domain.GuidanceInput, prior domain.TaskRoutingAuthority) (domain.RoutingFacts, error) {
	version := detail.Version
	version.GuidanceText = in.GuidanceText
	if in.Acceptance != nil {
		acceptance, err := domain.ResolveAcceptance(in.Acceptance)
		if err != nil {
			return domain.RoutingFacts{}, err
		}
		version.Acceptance = acceptance
	}
	declared := prior.OptionSnapshot.Suitability
	if in.Routing != nil {
		declared = in.Routing.Suitability
	}
	facts := domain.RoutingFacts{Assessment: domain.SuitabilityAssessment{Classification: "unknown", Source: "unknown", ReasonCodes: []string{"project_evidence_unavailable"}}, Contexts: []domain.NativeContextFact{}}
	if err := ctx.Err(); err != nil {
		return facts, err
	}
	p, identity, err := e.db.Project(ctx, detail.Task.ProjectID)
	if err != nil {
		return facts, err
	}
	if project.ValidateIdentity(p.ResolvedPath, identity) != nil {
		return facts, nil
	}
	assessment, assessmentErr := routing.Assess(ctx, p.ResolvedPath, detail.Task.OriginalText, version, declared)
	if assessmentErr != nil {
		return facts, assessmentErr
	}
	facts.Assessment = assessment
	return facts, nil
}
