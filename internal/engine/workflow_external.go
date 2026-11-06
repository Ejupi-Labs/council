package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (e *Engine) runWorkflowExternal(ctx context.Context, a domain.Attempt, step domain.WorkflowStep, subject workflowSubject) {
	action := subject.Workflow.Authority.Delivery.ExternalAction
	if action == nil {
		e.settleUnlaunched(a, primary("delivery_scope_required", "A task instruction or agent response cannot declare an executable external action."))
		return
	}
	if err := e.guardLocalStage(ctx, a); err != nil {
		e.settleUnlaunched(a, primary("launch_blocked", "The explicit external action lost current execution authority."), err)
		return
	}
	snapshot, err := e.handoffWorkflowSubject(ctx, a, subject)
	if err != nil {
		e.settleUnlaunched(a, primary("source_capture_blocked", "The actual external-action subject could not be materialized."), err)
		return
	}
	program, err := workflowProgram(action.Command.Executable)
	if err != nil {
		e.settleUnlaunched(a, primary("capability_unavailable", "The explicitly declared native program is unavailable."))
		return
	}
	timeout := 300
	if action.Command.TimeoutSeconds != nil {
		timeout = *action.Command.TimeoutSeconds
	}
	command := domain.CommandDefinition{Executable: program.Executable, PhysicalIdentity: program.PhysicalIdentity, Arguments: append([]string{}, action.Command.Arguments...), RelativeWorkingDirectory: ".", TimeoutSeconds: timeout, SuccessExitCodes: []int{0}}
	observed, commandErr := e.executeWorkflowCommand(ctx, a, snapshot, command)
	if !observed.Started && observed.Settlement.Cleanup.State == "" {
		e.settleUnlaunched(a, primary("launch_failed", "The explicit external action could not be prepared."), commandErr)
		return
	}
	record := domain.ExternalDeliveryRecord{WorkflowID: a.WorkflowID, StepID: step.ID, AttemptID: a.ID, Action: *action, Command: command, Outcome: "unavailable", CreatedAt: domain.Now()}
	if domain.ValidID(observed.Stdout.ID) {
		record.StdoutArtifactID = &observed.Stdout.ID
	}
	if domain.ValidID(observed.Stderr.ID) {
		record.StderrArtifactID = &observed.Stderr.ID
	}
	settlement := observed.Settlement
	if observed.Started {
		record.Outcome = "uncertain"
		if commandErr == nil && settlement.RootTerminal && settlement.InputComplete && settlement.ExitCode != nil && settlement.Cleanup.State == "verified" && observed.OutputComplete {
			record.Outcome = "command_failed"
			settlement.Outcome = "failed"
			if *settlement.ExitCode == 0 {
				record.Outcome, settlement.Outcome = "command_succeeded", "succeeded"
			}
		}
	}
	current, readErr := e.db.Attempt(context.Background(), a.ID)
	if readErr != nil {
		e.fence(a.ID, "storage_failed", "The external action outcome cannot be inspected. No automatic replay is permitted.")
		return
	}
	if current.Stop != nil {
		record.Outcome = "cancelled"
	}
	if settlement.Cleanup.State == "verified" {
		candidate, captureErr := e.retainLocalCandidate(context.Background(), a, snapshot)
		if captureErr != nil {
			settlement.EvidenceStatus = "incomplete"
			settlement.Failure = primary("candidate_incomplete", "The external command's post-execution source/index could not be retained completely.")
			if uncertainEvidence(captureErr) {
				settlement.EvidenceStatus = "uncertain"
				e.fence(a.ID, "external_evidence_uncertain", "External command evidence is unresolved; its effects must not be repeated.")
			}
		} else if candidate.HasDelta || candidate.HasIndexDelta {
			settlement.Outcome = "failed"
			settlement.Failure = primary("candidate_stale", "The external command changed its managed source or index after verification. Retain the actual command outcome and inspect its new candidate before acceptance.")
		}
	}
	facts := domain.WorkflowStepFacts{StepID: step.ID, OwnerInstanceID: e.config.OwnerInstanceID, CandidateBinding: subject.Binding, ResponseBinding: subject.Response, External: &record, Settlement: settlement}
	e.finishWorkflowLocal(a, subject, facts)
}
