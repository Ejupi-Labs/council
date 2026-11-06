package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"reflect"
	"slices"
)

func (s *Store) SettleWorkflowStep(ctx context.Context, attemptID string, f domain.WorkflowStepFacts) (domain.Workflow, error) {
	var out domain.Workflow
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		a, e := s.attempt(ctx, tx, attemptID)
		if e != nil {
			return e
		}
		if a.WorkflowID == "" {
			_, e = s.settle(ctx, tx, a, f.Settlement)
			return e
		}
		w, e := s.workflowByID(ctx, tx, a.WorkflowID)
		if e != nil {
			return e
		}
		out = w
		step, e := s.workflowStep(ctx, tx, a.StepID)
		if e != nil {
			return e
		}
		if f.StepID != step.ID || step.WorkflowID != w.ID || step.AttemptID == nil || *step.AttemptID != a.ID || f.OwnerInstanceID != a.OwnerInstanceID {
			return domain.Fail("workflow_revision_conflict", "The settlement does not belong to this owned step execution.")
		}
		if a.Phase == "settled" {
			return nil
		}
		if a.ExecutorKind == "local_application" {
			var active int
			if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM delivery_operations WHERE attempt_id=? AND active_effect_id IS NOT NULL", a.ID).Scan(&active); e != nil {
				return storageError()
			}
			if active != 0 {
				return domain.Fail("effect_uncertain", "A permitted filesystem action remains in flight; retain ownership until its actual result is recorded.")
			}
		}
		t, e := s.task(ctx, tx, a.TaskID)
		if e != nil {
			return e
		}
		current := t.CurrentVersion == w.TaskVersion && w.CurrentStepID != nil && *w.CurrentStepID == step.ID && t.LatestAttemptID != nil && *t.LatestAttemptID == a.ID
		if f.TaskRevision != "" && f.TaskRevision != t.Revision || f.WorkflowRevision != "" && f.WorkflowRevision != w.Revision {
			current = false
		}
		clean := f.Settlement.Cleanup.State == "verified" && f.Settlement.EvidenceStatus == "complete" && a.Stop == nil && a.PrimaryFailure == nil && f.Settlement.Failure == nil
		qualify := clean && current && f.Settlement.Outcome == "succeeded"
		checkFailure := clean && current && step.Role == "check" && f.Check != nil && f.Check.Outcome == "failed" && f.Settlement.Outcome == "failed" && !f.Check.SourceChanged
		checkRecorded := false
		if f.Check != nil && (f.Check.Outcome != "passed" && f.Check.Outcome != "failed" || f.Check.SourceChanged) {
			qualify, checkFailure = false, false
		}
		if qualify || checkFailure {
			if f.CandidateBinding != nil && f.ResponseBinding != nil {
				return evidenceUnavailable()
			}
			switch step.Role {
			case "author", "correction":
				if f.CandidateBinding == nil && f.ResponseBinding == nil {
					return evidenceUnavailable()
				}
				if f.CandidateBinding != nil {
					c, e := s.candidate(ctx, tx, f.CandidateBinding.CandidateID)
					if e != nil {
						return e
					}
					if c.AttemptID != a.ID {
						return evidenceUnavailable()
					}
					if e = s.candidateBinding(ctx, tx, w, *f.CandidateBinding, f.VerifiedArtifacts); e != nil {
						return e
					}
					w.SelectedCandidateID = ptr(c.ID)
					w.SelectedResponseBinding = nil
					step.OutputCandidateID = ptr(c.ID)
				} else {
					if f.ResponseBinding.ProducerAttemptID != a.ID {
						return evidenceUnavailable()
					}
					if e = s.responseBinding(ctx, tx, w, *f.ResponseBinding, f.VerifiedArtifacts); e != nil {
						return e
					}
					w.SelectedCandidateID = nil
					response := *f.ResponseBinding
					w.SelectedResponseBinding = &response
					step.OutputResponseBinding = &response
				}
				w.HasTaskDelta = f.HasTaskDelta
				w.HasTaskIndexDelta = f.HasTaskIndexDelta
				w.EvidenceUses = append([]domain.EvidenceUse{}, f.ReuseEvidenceUses...)
			default:
				if !reflect.DeepEqual(step.InputCandidateID, w.SelectedCandidateID) || !reflect.DeepEqual(step.InputResponseBinding, w.SelectedResponseBinding) {
					return evidenceUnavailable()
				}
				if f.CandidateBinding != nil {
					if w.SelectedCandidateID == nil || f.CandidateBinding.CandidateID != *w.SelectedCandidateID {
						return evidenceUnavailable()
					}
					if e = s.candidateBinding(ctx, tx, w, *f.CandidateBinding, f.VerifiedArtifacts); e != nil {
						return e
					}
				}
				if f.ResponseBinding != nil {
					if !reflect.DeepEqual(f.ResponseBinding, w.SelectedResponseBinding) {
						return evidenceUnavailable()
					}
					if e = s.responseBinding(ctx, tx, w, *f.ResponseBinding, f.VerifiedArtifacts); e != nil {
						return e
					}
				}
			}
			if step.Role == "review" {
				if f.Report == nil {
					return evidenceUnavailable()
				}
				use, e := s.retainWorkflowReview(ctx, tx, w, &step, a, f)
				if e != nil {
					return e
				}
				w.EvidenceUses = replaceWorkflowUse(w.EvidenceUses, use, "")
			}
			if step.Role == "check" {
				if f.Check == nil {
					return evidenceUnavailable()
				}
				use, e := s.retainWorkflowCheck(ctx, tx, w, &step, a, f)
				if e != nil {
					return e
				}
				w.EvidenceUses = replaceWorkflowUse(w.EvidenceUses, use, f.Check.DefinitionID)
				checkRecorded = true
			}
		}
		if f.Check != nil && !checkRecorded {
			if step.Role != "check" {
				return evidenceUnavailable()
			}
			if _, err := s.retainWorkflowCheck(ctx, tx, w, &step, a, f); err != nil {
				return err
			}
		}
		sourceChangedSelected := false
		if current && clean && f.Check != nil && f.Check.SourceChanged && f.ResultCandidateBinding != nil {
			if f.Check.ResultingCandidateID == nil || f.ResultCandidateBinding.CandidateID != *f.Check.ResultingCandidateID {
				return evidenceUnavailable()
			}
			if err := s.candidateBinding(ctx, tx, w, *f.ResultCandidateBinding, f.VerifiedArtifacts); err != nil {
				return err
			}
			candidate, err := s.candidate(ctx, tx, f.ResultCandidateBinding.CandidateID)
			if err != nil {
				return err
			}
			if candidate.AttemptID != a.ID {
				return evidenceUnavailable()
			}
			w.SelectedCandidateID = ptr(candidate.ID)
			w.SelectedResponseBinding = nil
			step.OutputCandidateID = ptr(candidate.ID)
			w.HasTaskDelta, w.HasTaskIndexDelta = f.HasTaskDelta, f.HasTaskIndexDelta
			w.EvidenceUses = append([]domain.EvidenceUse{}, f.ReuseEvidenceUses...)
			if _, _, err := s.workflowUses(ctx, tx, w, w.EvidenceUses, f.VerifiedArtifacts); err != nil {
				return err
			}
			sourceChangedSelected = true
		}
		settled, e := s.settleCore(ctx, tx, a, f.Settlement)
		if e != nil {
			return e
		}
		if f.Operation != nil {
			if e := s.finishDelivery(ctx, tx, w, step, settled, *f.Operation, f.VerifiedArtifacts); e != nil {
				return e
			}
		}
		if f.External != nil {
			if e := s.retainWorkflowExternal(ctx, tx, w, &step, settled, f.Settlement, *f.External, f.VerifiedArtifacts); e != nil {
				return e
			}
		}
		step.Status = *settled.Outcome
		step.SettledAt = settled.SettledAt
		step.AdvanceProof = nil
		if !current {
			if e = s.saveWorkflowStep(ctx, tx, step); e != nil {
				return e
			}
			out = w
			return nil
		}
		w.State = "blocked"
		w.NextAction = "inspect_retained_stage"
		w.NextOwner = "user"
		if qualify || checkFailure {
			review, checks, e := s.workflowUses(ctx, tx, w, w.EvidenceUses, f.VerifiedArtifacts)
			if e != nil {
				return e
			}
			v, e := s.version(ctx, tx, w.TaskID, w.TaskVersion)
			if e != nil {
				return e
			}
			direct := f.DirectProven
			for _, check := range checks {
				d, _ := workflowDefinition(w, check.DefinitionID)
				if d.Kind == "builtin" && check.BuiltinCompleted && check.Outcome == "passed" && !check.SourceChanged {
					direct = true
				}
			}
			if e := s.resolveWorkflowFindings(ctx, tx, w, review, checks, direct); e != nil {
				return e
			}
			blockers, _, e := s.workflowBlockers(ctx, tx, w)
			if e != nil {
				return e
			}
			needsFindingReview, e := s.workflowNeedsFindingReview(ctx, tx, w, blockers)
			if e != nil {
				return e
			}
			input := workflowpolicy.PlanInput{NeedsFindingReview: needsFindingReview, BlockingFindingIDs: blockers, Authority: w.Authority, ResponseOnly: v.Acceptance.Deliverable == "response_only", HasSubject: w.SelectedCandidateID != nil || w.SelectedResponseBinding != nil, HasDelta: w.HasTaskDelta, HasIndexDelta: w.HasTaskIndexDelta, DirectProven: direct, Review: review, Checks: checks, CorrectionCyclesUsed: w.CorrectionCyclesUsed}
			if f.Operation != nil {
				input.LocalOutcome = f.Operation.Status
				input.LocalApplied = f.Operation.Status == "applied"
			}
			if f.External != nil {
				input.ExternalOutcome = f.External.Outcome
				input.ExternalComplete = f.External.Outcome == "command_succeeded"
			}
			decision := workflowpolicy.Plan(input)
			w.NextAction = decision.Action
			w.NextOwner = decision.Owner
			if f.Check != nil && f.Check.SourceChanged {
				w.NextAction = "candidate_stale"
				w.NextOwner = "user"
			}
			if settled.Stop != nil || settled.PrimaryFailure != nil && !checkFailure || settled.Cleanup.State != "verified" || settled.EvidenceStatus != "complete" {
				w.NextAction = "inspect_retained_stage"
				w.NextOwner = "user"
			}
			if (decision.Role != "" || decision.Action == "accept") && w.NextOwner == "council" {
				b, e := s.taskBudget(ctx, tx, w.TaskID)
				if e != nil {
					return e
				}
				g, e := s.gate(ctx, tx)
				if e != nil {
					return e
				}
				if b != nil && checkGate(g) == nil && (g.OwnerInstanceID == "" || g.OwnerInstanceID == a.OwnerInstanceID) {
					subject := ""
					if w.SelectedCandidateID != nil {
						subject = *w.SelectedCandidateID
					} else if w.SelectedResponseBinding != nil {
						subject = w.SelectedResponseBinding.ResponseArtifactID
					}
					cycle := w.CorrectionCyclesUsed
					if decision.Role == "correction" {
						cycle++
					}
					role := decision.Role
					if decision.Action == "accept" {
						role = "accept"
					}
					key, invalid := domain.WorkflowStageKey(role, cycle, decision.CheckID, subject)
					if invalid != nil {
						return invalid
					}
					step.AdvanceProof = &domain.WorkflowAdvanceProof{PredecessorStepID: step.ID, OwnerInstanceID: a.OwnerInstanceID, TaskRevision: t.Revision, TaskVersion: t.CurrentVersion, WorkflowRevision: w.Revision, BudgetRevision: b.Revision, WindowNumber: b.CurrentWindow.Number, CandidateID: w.SelectedCandidateID, ResponseBinding: w.SelectedResponseBinding, NextLogicalKey: key, Trigger: "workflow_stage"}
				}
			}
		} else if settled.Outcome != nil {
			w.State = *settled.Outcome
			if w.State == "succeeded" {
				w.State = "blocked"
			}
		}
		if sourceChangedSelected {
			w.State = "blocked"
			w.NextAction = "candidate_stale"
			w.NextOwner = "user"
			step.AdvanceProof = nil
		}
		w.UpdatedAt = domain.Now()
		w.StateRevision, e = increment(w.StateRevision)
		if e != nil {
			return e
		}
		if e = s.saveWorkflowStep(ctx, tx, step); e != nil {
			return e
		}
		if e = saveWorkflow(ctx, tx, w); e != nil {
			return e
		}
		out = w
		return nil
	})
	return out, err
}

func replaceWorkflowUse(old []domain.EvidenceUse, use domain.EvidenceUse, definition string) []domain.EvidenceUse {
	out := []domain.EvidenceUse{}
	for _, u := range old {
		if u.Kind == "review" && use.Kind == "review" {
			continue
		}
		if u.Kind == "check" && use.Kind == "check" && u.Applicability.DefinitionSHA256 == use.Applicability.DefinitionSHA256 {
			continue
		}
		out = append(out, u)
	}
	return append(out, use)
}

func (s *Store) retainWorkflowCheck(ctx context.Context, tx *sql.Tx, w domain.Workflow, step *domain.WorkflowStep, a domain.Attempt, f domain.WorkflowStepFacts) (domain.EvidenceUse, error) {
	r := *f.Check
	d, err := workflowDefinition(w, r.DefinitionID)
	if err != nil {
		return domain.EvidenceUse{}, err
	}
	if !domain.ValidID(r.ID) || r.WorkflowID != w.ID || r.StepID != step.ID || r.AttemptID != a.ID || step.DefinitionID == nil || *step.DefinitionID != d.ID || r.DefinitionSHA256 != d.DefinitionSHA256 || r.Applicability.DefinitionSHA256 != d.DefinitionSHA256 || !reflect.DeepEqual(r.InputCandidateBinding, f.CandidateBinding) || !reflect.DeepEqual(r.InputResponseBinding, f.ResponseBinding) || r.Finding != nil {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	if r.InputCandidateBinding != nil && r.InputResponseBinding != nil {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	if r.InputCandidateBinding != nil {
		if step.InputCandidateID == nil || *step.InputCandidateID != r.InputCandidateBinding.CandidateID {
			return domain.EvidenceUse{}, evidenceUnavailable()
		}
	} else if r.InputResponseBinding == nil || !reflect.DeepEqual(step.InputResponseBinding, r.InputResponseBinding) {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	observed := r.Outcome == "passed" || r.Outcome == "failed"
	switch r.Outcome {
	case "passed", "failed", "unavailable", "cancelled", "interrupted", "not_run", "uncertain":
	default:
		return domain.EvidenceUse{}, domain.Invalid("recorded check outcome")
	}
	if !reflect.DeepEqual(r.Cleanup, f.Settlement.Cleanup) {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	if d.Kind == "builtin" {
		if r.ExitCode != nil || r.ObservedExecutable != nil || r.StdoutArtifactID != nil || r.StderrArtifactID != nil || a.LaunchIntent || observed && (!r.BuiltinCompleted || !f.Settlement.BuiltinCompleted) || !observed && r.BuiltinCompleted {
			return domain.EvidenceUse{}, evidenceUnavailable()
		}
	} else {
		if r.ObservedExecutable != nil && !reflect.DeepEqual(r.ObservedExecutable, d.Command) {
			return domain.EvidenceUse{}, evidenceUnavailable()
		}
		if observed {
			if r.ObservedExecutable == nil || !a.LaunchIntent || !r.RootTerminal || !r.InputComplete || !r.OutputComplete || r.ExitCode == nil || r.StdoutArtifactID == nil || r.StderrArtifactID == nil || r.Cleanup.State != "verified" {
				return domain.EvidenceUse{}, evidenceUnavailable()
			}
			accepted := slices.Contains(d.Command.SuccessExitCodes, *r.ExitCode)
			if r.Outcome == "passed" && !accepted || r.Outcome == "failed" && accepted {
				return domain.EvidenceUse{}, evidenceUnavailable()
			}
		}
	}
	if observed && r.Cleanup.State != "verified" {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	if err = s.workflowStreamRefs(ctx, tx, a.ID, r.StdoutArtifactID, r.StderrArtifactID, observed && d.Kind == "command", f.VerifiedArtifacts); err != nil {
		return domain.EvidenceUse{}, err
	}
	if r.SourceChanged {
		if r.ResultingCandidateID == nil {
			return domain.EvidenceUse{}, evidenceUnavailable()
		}
		candidate, err := s.candidate(ctx, tx, *r.ResultingCandidateID)
		if err != nil {
			return domain.EvidenceUse{}, err
		}
		if candidate.AttemptID != a.ID || candidate.WorkflowID != w.ID {
			return domain.EvidenceUse{}, evidenceUnavailable()
		}
	} else if r.ResultingCandidateID != nil {
		return domain.EvidenceUse{}, evidenceUnavailable()
	}
	if observed && r.Outcome == "failed" && d.Required && d.Kind == "command" && !r.SourceChanged {
		r.Finding = &domain.WorkflowFinding{ID: domain.NewID(), CheckID: r.ID, ModelKey: d.Key, Finding: domain.ReviewFinding{Key: d.Key, Severity: "blocking", ConditionKeys: d.CoveredConditionKeys, Locations: []domain.ReviewLocation{}, Summary: "The required declared check failed its captured success criterion.", EvidenceRefs: []string{"artifact:" + *r.StdoutArtifactID, "artifact:" + *r.StderrArtifactID}}}
	}
	seq, err := nextSequence(ctx, tx, "workflow_checks")
	if err != nil {
		return domain.EvidenceUse{}, err
	}
	var candidate, response *string
	if r.InputCandidateBinding != nil {
		candidate = ptr(r.InputCandidateBinding.CandidateID)
	} else {
		response = ptr(r.InputResponseBinding.ResponseArtifactID)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO workflow_checks VALUES(?,?,?,?,?,?,?,?,?)", seq, r.ID, w.ID, step.ID, a.ID, d.ID, candidate, response, encode(r)); err != nil {
		return domain.EvidenceUse{}, storageError()
	}
	step.CheckID = ptr(r.ID)
	return selectedUse(w, "check", r.ID, r.Applicability), nil
}

func (s *Store) workflowStreamRefs(ctx context.Context, q queryer, attemptID string, stdout, stderr *string, complete bool, proofs []domain.VerifiedResponse) error {
	for _, ref := range []struct {
		id   *string
		kind string
	}{{stdout, "stdout"}, {stderr, "stderr"}} {
		if ref.id == nil {
			if complete {
				return evidenceUnavailable()
			}
			continue
		}
		artifact, err := s.artifact(ctx, q, *ref.id)
		if err != nil {
			return err
		}
		if artifact.AttemptID != attemptID || artifact.Kind != ref.kind {
			return evidenceUnavailable()
		}
		if complete {
			if err = s.verifyWorkflowProofs(ctx, q, proofs, *ref.id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) retainWorkflowExternal(ctx context.Context, tx *sql.Tx, w domain.Workflow, step *domain.WorkflowStep, a domain.Attempt, in domain.Settlement, r domain.ExternalDeliveryRecord, proofs []domain.VerifiedResponse) error {
	if r.WorkflowID != w.ID || r.StepID != step.ID || r.AttemptID != a.ID || step.Role != "external_delivery" || a.ExecutorKind != "local_external" || w.Authority.Delivery.ExternalAction == nil || !reflect.DeepEqual(r.Action, *w.Authority.Delivery.ExternalAction) || r.Action.AcceptanceBasis != "command_completion" {
		return evidenceUnavailable()
	}
	observed := r.Outcome == "command_succeeded" || r.Outcome == "command_failed"
	switch r.Outcome {
	case "command_succeeded", "command_failed", "cancelled", "unavailable", "uncertain":
	default:
		return domain.Invalid("recorded external outcome")
	}
	timeout := 300
	if r.Action.Command.TimeoutSeconds != nil {
		timeout = *r.Action.Command.TimeoutSeconds
	}
	if r.Command.Executable != "" && (r.Command.PhysicalIdentity == "" || !reflect.DeepEqual(r.Command.Arguments, r.Action.Command.Arguments) || r.Command.RelativeWorkingDirectory != "." || r.Command.TimeoutSeconds != timeout || !slices.Equal(r.Command.SuccessExitCodes, []int{0})) {
		return evidenceUnavailable()
	}
	if observed && (!a.LaunchIntent || !in.RootTerminal || !in.InputComplete || in.ExitCode == nil || in.Cleanup.State != "verified" || in.EvidenceStatus != "complete" || r.Command.Executable == "") {
		return evidenceUnavailable()
	}
	if observed && (r.Outcome == "command_succeeded" && *in.ExitCode != 0 || r.Outcome == "command_failed" && *in.ExitCode == 0) {
		return evidenceUnavailable()
	}
	if err := s.workflowStreamRefs(ctx, tx, a.ID, r.StdoutArtifactID, r.StderrArtifactID, observed, proofs); err != nil {
		return err
	}
	step.ExternalResult = &r
	return nil
}
