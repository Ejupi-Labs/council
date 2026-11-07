package store

import (
	"context"
	"database/sql"
	"reflect"
	"strings"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

func (s *Store) CreateTaskWithWorkflow(ctx context.Context, ownerID string, in domain.StartTaskInput, facts domain.WorkflowAdmissionFacts) domain.Result {
	if in.Workflow != nil && in.Workflow.Mode == "author_only" {
		return s.createTask(ctx, ownerID, in, facts.Routing, nil)
	}
	capture := facts.Capture
	if capture == nil {
		capture = &domain.WorkflowCapture{}
	}
	return s.createTask(ctx, ownerID, in, facts.Routing, capture)
}

func (s *Store) bindInitialWorkflow(ctx context.Context, tx *sql.Tx, t domain.Task, v domain.TaskVersion, a *domain.Attempt, routingAuthorityID string, in domain.StartTaskInput, capture *domain.WorkflowCapture) (domain.Workflow, domain.WorkflowStep, error) {
	return s.bindInitialWorkflowWithGraph(ctx, tx, t, v, a, routingAuthorityID, in, capture, nil)
}
func (s *Store) bindInitialWorkflowWithGraph(ctx context.Context, tx *sql.Tx, t domain.Task, v domain.TaskVersion, a *domain.Attempt, routingAuthorityID string, in domain.StartTaskInput, capture *domain.WorkflowCapture, graph *graphWorkflowAuthority) (domain.Workflow, domain.WorkflowStep, error) {
	var w domain.Workflow
	var step domain.WorkflowStep
	if capture == nil || !domain.ValidID(ownerOf(a)) {
		return w, step, workflowUnavailable()
	}
	raw, e := json.Marshal(capture.Authority)
	if e != nil || len(raw) > domain.MaxWorkflowBytes {
		return w, step, domain.Invalid("bounded captured workflow authority")
	}
	authority, e := decode[domain.WorkflowAuthority](string(raw))
	if e != nil {
		return w, step, domain.Invalid("captured workflow authority")
	}
	if authority.Delivery.ProjectID != t.ProjectID || authority.Delivery.ProjectIdentity == "" || authority.Delivery.RepositoryIdentity == "" {
		return w, step, workflowUnavailable()
	}
	var physical, projectRaw string
	if e = tx.QueryRowContext(ctx, "SELECT physical_identity,data FROM projects WHERE id=?", t.ProjectID).Scan(&physical, &projectRaw); e != nil {
		return w, step, storageError()
	}
	project, e := decode[domain.Project](projectRaw)
	if e != nil {
		return w, step, storageError()
	}
	if authority.Delivery.ProjectIdentity != physical || authority.Delivery.Destination != project.ResolvedPath {
		return w, step, domain.Fail("project_identity_changed", "The captured workflow destination does not match the retained project. Inspect it and start again.")
	}
	if authority.ExecutionMode != "" && authority.ExecutionMode != a.ExecutionMode {
		return w, step, domain.Invalid("matching captured execution mode")
	}
	if authority.FakeScenario != "" && authority.FakeScenario != a.FakeScenario {
		return w, step, domain.Invalid("matching captured fake scenario")
	}
	options := domain.WorkflowInput{}
	if in.Workflow != nil {
		options = *in.Workflow
	}
	if options.Mode == "" {
		options.Mode = "auto"
	}
	if options.Mode != "auto" {
		return w, step, domain.Invalid("Auto workflow authority")
	}
	if options.Review == "" {
		options.Review = "auto"
	}
	if graph != nil {
		if !reflect.DeepEqual(authority.Delivery, graph.Scope) {
			return w, step, domain.Invalid("matching immutable graph delivery scope")
		}
		options.Delivery = &domain.WorkflowDeliveryInput{LocalMode: graph.Scope.LocalMode, ExternalAction: graph.Scope.ExternalAction}
	} else {
		pref, err := s.deliveryPreference(ctx, tx)
		if err != nil {
			return w, step, err
		}
		localMode, source := pref.LocalMode, "saved_preference"
		if pref.SelectionSource == "default" {
			source = "default"
		}
		var external *domain.ExternalActionInput
		if options.Delivery != nil {
			if options.Delivery.LocalMode != "" {
				localMode = options.Delivery.LocalMode
				source = "task_scope"
			}
			external = options.Delivery.ExternalAction
		}
		if authority.Delivery.LocalMode != "" && authority.Delivery.LocalMode != localMode {
			return w, step, domain.Fail("revision_conflict", "The saved delivery preference changed during preparation. Reload it before starting.")
		}
		if authority.Delivery.PreferenceRevision != "" && source != "task_scope" && authority.Delivery.PreferenceRevision != pref.Revision {
			return w, step, domain.RevisionConflict(pref.Revision)
		}
		if !reflect.DeepEqual(authority.Delivery.ExternalAction, external) {
			return w, step, domain.Invalid("matching explicit external declaration")
		}
		authority.Delivery.LocalMode = localMode
		authority.Delivery.Source = source
		authority.Delivery.PreferenceRevision = pref.Revision
		options.Delivery = &domain.WorkflowDeliveryInput{LocalMode: localMode, ExternalAction: external}
	}
	if e := domain.ValidateWorkflowInput(&options); e != nil {
		return w, step, e
	}
	if len(authority.Checks) > domain.MaxWorkflowChecks || len(authority.CheckGaps) > domain.MaxWorkflowChecks {
		return w, step, domain.Invalid("bounded captured checks and gaps")
	}
	seen := map[string]bool{}
	for i := range authority.Checks {
		check := &authority.Checks[i]
		if !domain.WorkflowKey(check.Key) || seen[check.Key] || (check.Source != "recognized_convention" && check.Source != "task_declaration") {
			return w, step, domain.Invalid("unique captured check definitions")
		}
		seen[check.Key] = true
		if check.ID == "" {
			check.ID = domain.NewID()
		}
		if !domain.ValidID(check.ID) {
			return w, step, domain.Invalid("captured check ID")
		}
		if check.Kind == "builtin" {
			if check.BuiltinID != "literal_document_replace/1" || check.Command != nil {
				return w, step, domain.Invalid("supported built-in check")
			}
		} else if check.Kind == "command" {
			if check.Command == nil || check.Command.PhysicalIdentity == "" {
				return w, step, domain.Invalid("observed native command identity")
			}
			command := check.Command
			required := check.Required
			scope := check.Scope
			codes := command.SuccessExitCodes
			timeout := command.TimeoutSeconds
			declaration := domain.CheckDeclaration{Key: check.Key, Executable: command.Executable, Arguments: command.Arguments, RelativeWorkingDirectory: command.RelativeWorkingDirectory, Scope: &scope, CoveredConditionKeys: check.CoveredConditionKeys, TimeoutSeconds: &timeout, SuccessExitCodes: &codes, Required: &required}
			if err := domain.ValidateCheckDeclaration(declaration); err != nil {
				return w, step, err
			}
		} else {
			return w, step, domain.Invalid("captured check kind")
		}
		digest, err := workflowpolicy.DefinitionDigest(*check)
		if err != nil {
			return w, step, domain.Invalid("captured check digest")
		}
		if check.DefinitionSHA256 != "" && check.DefinitionSHA256 != digest {
			return w, step, domain.Invalid("matching captured check digest")
		}
		check.DefinitionSHA256 = digest
	}
	for _, gap := range authority.CheckGaps {
		if !domain.WorkflowKey(gap.Key) || gap.Code == "" || len(gap.Code) > 128 || len(gap.Path) > 4096 || gap.Message == "" || len(gap.Message) > 4096 {
			return w, step, domain.Invalid("bounded retained check gap")
		}
	}
	registry, validationErr := workflowpolicy.Conditions(t.OriginalText, v, options, authority.Checks)
	if validationErr != nil {
		return w, step, validationErr
	}
	if authority.Conditions != nil && !reflect.DeepEqual(authority.Conditions, registry) {
		return w, step, domain.Invalid("matching complete condition registry")
	}
	instruction, conditions, err := workflowpolicy.ConditionDigests(t.OriginalText, v, registry, authority.Checks)
	if err != nil {
		return w, step, domain.Invalid("canonical workflow condition digests")
	}
	if graph != nil {
		conditions = graphEffectiveWorkflowConditions(conditions, graph.ConditionsSHA256)
	}
	if authority.InstructionSHA256 != "" && authority.InstructionSHA256 != instruction || authority.ConditionsSHA256 != "" && authority.ConditionsSHA256 != conditions {
		return w, step, domain.Invalid("matching workflow instructions and conditions")
	}
	authority.ID = domain.NewID()
	authority.TaskID = t.ID
	authority.TaskVersion = v.Version
	authority.CreatedByRequestID = in.RequestID
	authority.CapturedAt = a.AdmittedAt
	authority.RoutingAuthorityID = routingAuthorityID
	authority.ExecutionMode = a.ExecutionMode
	authority.FakeScenario = a.FakeScenario
	authority.OptionSnapshot = options
	authority.Conditions = registry
	authority.InstructionSHA256 = instruction
	authority.ConditionsSHA256 = conditions
	authority.Review = options.Review
	max := 1
	if options.MaxCorrectionCycles != nil {
		max = *options.MaxCorrectionCycles
	}
	authority.Correction = domain.CorrectionPolicy{MaxCycles: max, Triggers: []string{"blocking_review_finding", "failed_required_check"}}
	step = capture.Step
	if step.Role != "" && step.Role != "author" || step.ExecutorKind != "" && step.ExecutorKind != "native" || step.Cycle != 0 || step.InputCandidateID != nil || step.InputResponseBinding != nil || step.ReplacementOfStepID != nil || step.PredecessorStepID != nil {
		return w, step, domain.Invalid("initial author step")
	}
	wID := domain.NewID()
	step.ID = domain.NewID()
	step.WorkflowID = wID
	step.Ordinal = "1"
	step.ExecutionOrdinal = "1"
	step.Role = "author"
	step.ExecutorKind = "native"
	step.Status = "ready"
	step.Trigger = "user_request"
	step.AttemptID = ptr(a.ID)
	step.CreatedAt = a.AdmittedAt
	step.StartedAt = nil
	step.SettledAt = nil
	step.AdvanceProof = nil
	step.OutputCandidateID = nil
	step.OutputResponseBinding = nil
	if step.LogicalKey == "" {
		step.LogicalKey = "author:0"
	}
	if len(step.LogicalKey) > 256 || strings.ContainsRune(step.LogicalKey, 0) {
		return w, step, domain.Invalid("bounded logical stage identity")
	}
	a.WorkflowID = wID
	a.StepID = step.ID
	a.Role = "author"
	a.ExecutorKind = "native"
	a.Trigger = "user_request"
	if graph != nil {
		step.Trigger = "workflow_stage"
		a.Trigger = "workflow_stage"
	}
	w = domain.Workflow{EvidenceUses: []domain.EvidenceUse{}, ID: wID, TaskID: t.ID, TaskVersion: v.Version, Authority: authority, Revision: "1", StateRevision: "1", State: "queued", CurrentStepID: ptr(step.ID), NextAction: "observe_attempt", NextOwner: "council", CreatedAt: a.AdmittedAt, UpdatedAt: a.AdmittedAt}
	raw, e = json.Marshal(w.Authority)
	if e != nil || len(raw) > domain.MaxWorkflowBytes {
		return w, step, domain.Invalid("bounded captured workflow authority")
	}
	return w, step, nil
}
func ownerOf(a *domain.Attempt) string {
	if a == nil {
		return ""
	}
	return a.OwnerInstanceID
}
func insertWorkflowTx(ctx context.Context, tx *sql.Tx, w domain.Workflow) error {
	_, e := tx.ExecContext(ctx, "INSERT INTO workflows VALUES(?,?,?,1,1,?,?,?)", w.ID, w.TaskID, intRevision(w.TaskVersion), w.State, encode(w.Authority), encode(w))
	if e != nil {
		return storageError()
	}
	return nil
}
func insertWorkflowStepTx(ctx context.Context, tx *sql.Tx, step domain.WorkflowStep) error {
	seq, e := nextSequence(ctx, tx, "workflow_steps")
	if e != nil {
		return e
	}
	var responseID *string
	if step.InputResponseBinding != nil {
		responseID = ptr(step.InputResponseBinding.ResponseArtifactID)
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO workflow_steps(sequence,id,workflow_id,ordinal,execution_ordinal,logical_key,role,executor_kind,cycle,status,attempt_id,input_candidate_id,input_response_artifact_id,predecessor_step_id,replacement_of_step_id,consumption_key,data) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL,?)", seq, step.ID, step.WorkflowID, intRevision(step.Ordinal), intRevision(step.ExecutionOrdinal), step.LogicalKey, step.Role, step.ExecutorKind, step.Cycle, step.Status, step.AttemptID, step.InputCandidateID, responseID, step.PredecessorStepID, step.ReplacementOfStepID, encode(step))
	if e != nil {
		return storageError()
	}
	return nil
}
