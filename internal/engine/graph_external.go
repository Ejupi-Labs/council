package engine

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func graphLocalCommandRole(a domain.Attempt) bool {
	return a.ExecutorKind == "local_check" && a.Role == "check" || a.ExecutorKind == "local_external" && a.Role == "external_delivery"
}

// Publication uses the captured final scope, never an action inferred from
// instructions, a predecessor's response or a mutable command environment.
func (e *Engine) graphExternalScope(ctx context.Context, a domain.Attempt, member domain.GraphRef, subject workflowSubject) (*graphFinalVerification, error) {
	view, err := e.db.Graph(ctx, member.GraphID)
	if err != nil {
		return nil, err
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, member.GraphID)
	if err != nil {
		return nil, err
	}
	if member.Kind != "node" || member.NodeID == nil || view.Graph.Stop != nil || view.Graph.FinalNodeID == nil || *view.Graph.FinalNodeID != *member.NodeID ||
		a.Role != "external_delivery" || a.ExecutorKind != "local_external" || subject.Workflow.Authority.Delivery.ExternalAction == nil ||
		!reflect.DeepEqual(definition.Delivery, subject.Workflow.Authority.Delivery) {
		return nil, domain.Fail("delivery_scope_required", "Only the designated final may run its exact captured external command.")
	}
	dispatch, input, _, err := e.loadGraphStageContext(ctx, a, member, "local_external")
	if err != nil {
		return nil, err
	}
	if dispatch.Public.Dispatch.SourceBasis.BasisKind != "node_subject" || !reflect.DeepEqual(dispatch.Public.Dispatch.SourceBasis.CandidateBinding, subject.Binding) ||
		!reflect.DeepEqual(dispatch.Public.Dispatch.SourceBasis.ResponseBinding, subject.Response) || len(input.Public.InputBinding.ContributorVector) != len(view.Nodes)-1 {
		return nil, domain.Fail("graph_final_unqualified", "The publisher requires its exact selected final and all required accepted inputs.")
	}
	final, err := e.graphFinalContextForDispatch(ctx, a, subject)
	if err != nil {
		return nil, err
	}
	fileAncestors, err := graphMemberFileAncestors(definition, view.Nodes, view.Graph.GraphVersion, *member.NodeID)
	if err != nil {
		return nil, err
	}
	for _, contributor := range input.Public.InputBinding.ContributorVector {
		if contributor.CandidateBinding != nil && !fileAncestors[contributor.NodeID] {
			return nil, domain.Fail("graph_file_ancestry_unproven", "Required file results must reach the final publisher through declared file dependencies.")
		}
		current := false
		for _, node := range view.Nodes {
			accepted := node.AcceptedResult
			if node.ID == contributor.NodeID && node.TaskVersion == contributor.TaskVersion && node.State == "completed" && accepted != nil &&
				accepted.TaskVersion == contributor.TaskVersion && accepted.AcceptanceID == contributor.AcceptanceID && accepted.ProducerAttemptID == contributor.ProducerAttemptID && accepted.InputID == contributor.InputID {
				current = true
			}
		}
		if !current || final == nil && contributor.CandidateBinding != nil {
			return nil, domain.Fail("graph_final_unqualified", "Every required result must remain accepted; a response cannot qualify retained file changes for publication.")
		}
	}
	if final == nil && subject.Response == nil {
		return nil, domain.Fail("graph_final_unqualified", "A file publisher requires current cumulative final verification.")
	}
	return final, nil
}

func (e *Engine) prepareGraphExternalContext(ctx context.Context, a domain.Attempt, subject workflowSubject) (*graphCheckCommandContext, error) {
	member, err := e.graphStageContextMembership(ctx, a, "local_external")
	if err != nil || member == nil {
		return nil, err
	}
	prepared := &graphCheckCommandContext{EffectsKnown: true}
	prepared.Final, err = e.graphExternalScope(ctx, a, *member, subject)
	if err != nil {
		return prepared, err
	}
	if err = e.guardGraphNativePreparation(ctx, a); err != nil {
		return prepared, err
	}
	manifestID := subject.Source.ManifestArtifactID
	if subject.Binding != nil {
		manifestID = subject.Binding.ManifestArtifactID
	}
	// As for checks, attempted Handoff is unknown until the exact receiving
	// observation and source metadata have one acknowledged association.
	prepared.EffectsKnown = false
	prepared.Snapshot, err = e.workspace.Handoff(ctx, workspace.HandoffRequest{AttemptID: a.ID, Source: subject.snapshot(subject.Workflow.Authority.Delivery), CandidateManifestArtifactID: manifestID, IndexArtifactID: subject.ResponseIndexID()})
	if prepared.Snapshot == nil {
		return prepared, err
	}
	observeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	receiving, observeErr := e.prepareGraphStageReceivingSource(observeCtx, a, prepared.Snapshot, "local_external")
	prepared.Receiving = receiving
	var recordErr error
	if receiving != nil && validGraphArtifactReference(receiving.Materialized.ReceiptArtifact) {
		recordErr = e.recordGraphReceivingSource(observeCtx, a, receiving)
	}
	if receiving != nil {
		m := receiving.Materialized
		prepared.EffectsKnown = observeErr == nil && recordErr == nil && m.Receipt.State == "complete" && m.Receipt.EffectsKnown && m.Receipt.EvidenceStatus == "complete" && m.Snapshot != nil && validGraphArtifactReference(m.ReceiptArtifact)
	}
	if err = errors.Join(err, observeErr, recordErr); err != nil {
		return prepared, err
	}
	if err = e.guardGraphNativePreparation(ctx, a); err != nil {
		return prepared, err
	}
	_, prepared.Context, err = e.prepareGraphStageContext(ctx, a, adapters.Launch{}, "local_external", prepared.Final)
	if prepared.Context != nil {
		prepared.EffectsKnown = prepared.EffectsKnown && prepared.Context.EffectsKnown && prepared.Context.EvidenceStatus != "uncertain" && !uncertainEvidence(err)
	}
	if err == nil && prepared.Context == nil {
		err = domain.Fail("graph_context_unavailable", "The exact final publisher context could not be prepared.")
	}
	return prepared, err
}

func (e *Engine) recheckGraphExternalCommandScope(ctx context.Context, a domain.Attempt) error {
	if a.ExecutorKind != "local_external" {
		return nil
	}
	member, err := e.graphStageContextMembership(ctx, a, "local_external")
	if err != nil || member == nil {
		if err == nil {
			err = domain.Fail("graph_input_stale", "A retained graph publisher cannot become a standalone command.")
		}
		return err
	}
	detail, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || detail.Workflow == nil {
		if err == nil {
			err = domain.Fail("graph_input_stale", "The exact publisher workflow is unavailable.")
		}
		return err
	}
	subject, err := e.selectedWorkflowSubject(ctx, *detail.Workflow)
	if err != nil {
		return err
	}
	_, err = e.graphExternalScope(ctx, a, *member, subject)
	return err
}
