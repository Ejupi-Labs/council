package engine

import (
	"context"
	"path"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func recheckNativeOriginal(ctx context.Context, projectPath string, assessment domain.SuitabilityAssessment, derived bool) *domain.Error {
	if derived {
		return nil
	}
	return routing.RecheckOriginal(ctx, projectPath, assessment)
}

func workflowChangeFlags(changes []workspace.Change) (bool, bool) {
	index := false
	for _, change := range changes {
		// A changed working file also carries its unchanged index context.
		// Presence of that context alone is not a staged change.
		changed := (change.BeforeIndex == nil) != (change.AfterIndex == nil)
		if change.BeforeIndex != nil && change.AfterIndex != nil {
			changed = *change.BeforeIndex != *change.AfterIndex
		}
		index = index || changed
	}
	return len(changes) != 0, index
}

func (e *Engine) workflowFailedSettlementFacts(a domain.Attempt, settlement domain.Settlement) domain.WorkflowStepFacts {
	if a.WorkflowID == "" {
		return domain.WorkflowStepFacts{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	facts, err := e.workflowNativeFacts(ctx, a, nil, settlement)
	if err != nil {
		e.fence(a.ID, "workflow_settlement_unavailable", "The owned workflow settlement could not inspect its current version. Preserve evidence and resolve the incident before further work.")
	}
	return facts
}

func (e *Engine) verifyWorkflowMarkers(ctx context.Context, a domain.Attempt, snapshot *workspace.Snapshot) error {
	if a.WorkflowID == "" {
		return nil
	}
	view, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil {
		return err
	}
	if view.Workflow == nil {
		return domain.Fail("evidence_unavailable", "The captured workflow cannot be read.")
	}
	entries := make(map[string]workspace.Entry, len(snapshot.Manifest.Entries))
	for _, entry := range snapshot.Manifest.Entries {
		entries[entry.Path] = entry
	}
	for _, definition := range view.Workflow.Authority.Checks {
		for _, ref := range definition.EvidenceRefs {
			entry, present := entries[path.Join(snapshot.SelectedSubfolder, ref.RelativePath)]
			if !present || entry.Type != "file" || entry.SHA256 != ref.SHA256 || !domain.ValidID(entry.ArtifactID) {
				return domain.Fail("candidate_stale", "The actual retained check marker does not match its captured convention definition.")
			}
		}
	}
	return nil
}
