package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type deliveryDAOState struct {
	store                                 *Store
	admission                             domain.Admission
	owner, taskRevision, workflowRevision string
	operation                             domain.DeliveryOperation
}

// This is an explicitly metadata-only DAO fixture. Existing store helpers create
// the durable task/candidate/claim; direct setup establishes a no-process local
// executor and captured local scope. No native child, filesystem application,
// quarantine, model inspection, token usage or platform behavior is qualified.
func deliveryDAOFixture(t *testing.T) deliveryDAOState {
	t.Helper()
	ctx := context.Background()
	s := testStore(t)
	a, owner, facts := workflowAuthorSettlementFixture(t, s)
	selected, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{})
	if e != nil || selected.Workflow == nil {
		t.Fatal(selected, e)
	}
	w := *selected.Workflow
	step, e := s.WorkflowStep(ctx, a.Step.ID)
	if e != nil {
		t.Fatal(e)
	}
	attempt, e := s.Attempt(ctx, a.Attempt.ID)
	if e != nil {
		t.Fatal(e)
	}
	w.Authority.Delivery.LocalMode = "apply_verified_local"
	w.Authority.OptionSnapshot.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "apply_verified_local"}
	w.SelectedCandidateID = ptr(facts.CandidateBinding.CandidateID)
	w.SelectedResponseBinding = nil
	w.HasTaskDelta = true
	w.State = "running"
	step.Role, step.ExecutorKind, step.LogicalKey, step.Status = "local_delivery", "local_application", "local_delivery:controlled_dao", "active"
	step.InputCandidateID = ptr(facts.CandidateBinding.CandidateID)
	attempt.Role, attempt.ExecutorKind, attempt.ConnectionID, attempt.Phase = "local_delivery", "local_application", "local-application", "running"
	attempt.LaunchIntent = false
	attempt.InputCandidateID = ptr(facts.CandidateBinding.CandidateID)
	workflowListExec(t, s, "UPDATE workflow_steps SET role=?,executor_kind=?,logical_key=?,status=?,input_candidate_id=?,data=? WHERE id=?", step.Role, step.ExecutorKind, step.LogicalKey, step.Status, *step.InputCandidateID, encode(step), step.ID)
	workflowListExec(t, s, "UPDATE attempts SET phase=?,data=? WHERE id=?", attempt.Phase, encode(attempt), attempt.ID)
	workflowListExec(t, s, "UPDATE workflows SET state=?,authority_data=?,data=? WHERE id=?", w.State, encode(w.Authority), encode(w), w.ID)
	hash, invalid := domain.WorkflowAuthoritySHA256(w.Authority)
	if invalid != nil {
		t.Fatal(invalid)
	}
	detail, e := s.Task(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	op := domain.DeliveryOperation{ID: domain.NewID(), WorkflowID: w.ID, StepID: step.ID, AttemptID: attempt.ID, RequestID: domain.NewID(), CandidateBinding: *facts.CandidateBinding, Scope: w.Authority.Delivery, AuthoritySHA256: hash, Status: "ready", LastEffectSequence: "0", ConflictCount: "0", Conflicts: []domain.DeliveryConflict{}, CreatedAt: domain.Now()}
	return deliveryDAOState{store: s, admission: a, owner: owner, taskRevision: detail.Task.Revision, workflowRevision: w.Revision, operation: op}
}
func (f deliveryDAOState) permit(effectID, action string) domain.DeliveryPermitFacts {
	return domain.DeliveryPermitFacts{OperationID: f.operation.ID, OwnerInstanceID: f.owner, TaskRevision: f.taskRevision, WorkflowRevision: f.workflowRevision, Effect: domain.DeliveryEffect{OperationID: f.operation.ID, EffectID: effectID, Kind: "replace", RelativePath: "docs/note.md", Action: action, State: "intent", BeforeEvidence: &domain.EvidenceReference{Ref: "controlled-before-metadata", SHA256: strings.Repeat("a", 64)}, IntendedAfterEvidence: &domain.EvidenceReference{Ref: "controlled-after-metadata", SHA256: strings.Repeat("b", 64)}}}
}
func deliveryDAORows(t *testing.T, f deliveryDAOState) ([]domain.DeliveryEffect, sql.NullString) {
	t.Helper()
	rows, e := f.store.db.Query("SELECT data FROM delivery_effects WHERE operation_id=? ORDER BY sequence", f.operation.ID)
	if e != nil {
		t.Fatal(e)
	}
	out := []domain.DeliveryEffect{}
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			t.Fatal("effect row scan")
		}
		effect, e := decode[domain.DeliveryEffect](raw)
		if e != nil {
			t.Fatal(e)
		}
		out = append(out, effect)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		t.Fatal(e)
	}
	var active sql.NullString
	if e = f.store.db.QueryRow("SELECT active_effect_id FROM delivery_operations WHERE id=?", f.operation.ID).Scan(&active); e != nil {
		t.Fatal(e)
	}
	return out, active
}
func deliveryDAOBegin(t *testing.T, f deliveryDAOState) {
	t.Helper()
	if e := f.store.BeginDelivery(context.Background(), f.owner, f.operation); e != nil {
		t.Fatal("controlled DAO operation prerequisite", e)
	}
}

func TestWorkflowDeliveryDAOSeparateIntentQuarantineAndPublication(t *testing.T) {
	ctx := context.Background()
	f := deliveryDAOFixture(t)
	deliveryDAOBegin(t, f)
	id := domain.NewID()
	if _, e := f.store.PermitDeliveryEffect(ctx, f.permit(id, "publish_after")); workflowListErrorCode(e) != "effect_uncertain" {
		t.Fatal("replacement publication lacked durable quarantine prerequisite", e)
	}
	intent, e := f.store.PermitDeliveryEffect(ctx, f.permit(id, "quarantine_before"))
	if e != nil || intent.Sequence != "1" {
		t.Fatal(intent, e)
	}
	if _, e = f.store.PermitDeliveryEffect(ctx, f.permit(domain.NewID(), "quarantine_before")); workflowListErrorCode(e) != "attempt_active" {
		t.Fatal("a second filesystem effect entered one in-flight slot", e)
	}
	rows, active := deliveryDAORows(t, f)
	if len(rows) != 1 || rows[0].State != "intent" || !active.Valid || active.String != id {
		t.Fatal("effect was executed/acknowledged before its durable intent", rows, active)
	}
	savedIntent := encode(rows[0])
	ack := intent
	ack.State = "applied"
	ack.ObservedAfterEvidence = &domain.EvidenceReference{Ref: "controlled-quarantine-observation-no-os-proof"}
	if e = f.store.RecordDeliveryEffect(ctx, ack); e != nil {
		t.Fatal(e)
	}
	publication, e := f.store.PermitDeliveryEffect(ctx, f.permit(id, "publish_after"))
	if e != nil || publication.Sequence != "3" {
		t.Fatal("separate publication permit missing", publication, e)
	}
	publication.State = "applied"
	if e = f.store.RecordDeliveryEffect(ctx, publication); e != nil {
		t.Fatal(e)
	}
	rows, active = deliveryDAORows(t, f)
	if len(rows) != 4 || active.Valid || encode(rows[0]) != savedIntent {
		t.Fatal("append-only intent/result order changed", rows, active)
	}
	for i, row := range rows {
		if row.Sequence != domain.Decimal(int64(i+1)) {
			t.Fatal("noncontiguous exact effect sequence", rows)
		}
	}
	op, e := f.store.DeliveryOperation(ctx, f.operation.ID)
	if e != nil || op.LastEffectSequence != "4" || op.Status == "applied" {
		t.Fatal("per-path acknowledgement falsely completed entire operation", op, e)
	}
}

func TestWorkflowDeliveryDAOStopAllowsOnlyAlreadyPermittedAcknowledgement(t *testing.T) {
	ctx := context.Background()
	f := deliveryDAOFixture(t)
	deliveryDAOBegin(t, f)
	intent, e := f.store.PermitDeliveryEffect(ctx, f.permit(domain.NewID(), "quarantine_before"))
	if e != nil {
		t.Fatal(e)
	}
	stopID := domain.NewID()
	stopped := f.store.StopAttempt(ctx, f.admission.Attempt.ID, stopID)
	if stopped.Status != 202 {
		t.Fatal(stopped)
	}
	ack := intent
	ack.State = "applied"
	if e = f.store.RecordDeliveryEffect(ctx, ack); e != nil {
		t.Fatal("already permitted result could not settle after Stop", e)
	}
	detail, e := f.store.Task(ctx, f.admission.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	selected, e := f.store.Workflow(ctx, f.admission.Task.ID, domain.WorkflowSelector{})
	if e != nil {
		t.Fatal(e)
	}
	next := f.permit(intent.EffectID, "publish_after")
	next.TaskRevision = detail.Task.Revision
	next.WorkflowRevision = selected.Workflow.Revision
	if _, e = f.store.PermitDeliveryEffect(ctx, next); e == nil {
		t.Fatal("fresh revision bypassed accepted Stop for later publication")
	}
	rows, active := deliveryDAORows(t, f)
	if len(rows) != 2 || active.Valid {
		t.Fatal("Stop lost result or admitted later effect", rows, active)
	}
	attempt, e := f.store.Attempt(ctx, f.admission.Attempt.ID)
	if e != nil || attempt.Stop == nil || attempt.Stop.RequestID != stopID || attempt.Phase != "cancelling" {
		t.Fatal("effect acknowledgement rewrote primary Stop", attempt, e)
	}
	var claim string
	if e = f.store.db.QueryRow("SELECT attempt_id FROM capacity_claims WHERE resource_id='managed-native'").Scan(&claim); e != nil || claim != attempt.ID {
		t.Fatal("Stop released ownership before controller settlement", claim, e)
	}
}

func TestWorkflowDeliveryDAOUnacknowledgedPermitCannotReleaseOwnership(t *testing.T) {
	for _, name := range []string{"with_operation", "omitted_operation", "generic_settlement"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			f := deliveryDAOFixture(t)
			deliveryDAOBegin(t, f)
			if _, e := f.store.PermitDeliveryEffect(ctx, f.permit(domain.NewID(), "quarantine_before")); e != nil {
				t.Fatal(e)
			}
			in := domain.WorkflowStepFacts{StepID: f.operation.StepID, OwnerInstanceID: f.owner, TaskRevision: f.taskRevision, WorkflowRevision: f.workflowRevision, Settlement: domain.Settlement{Outcome: "cancelled", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "complete", NoProcessCreated: true, EffectsKnown: true}}
			if name == "with_operation" {
				op := f.operation
				op.Status = "cancelled"
				in.Operation = &op
			}
			var settleErr error
			if name == "generic_settlement" {
				_, settleErr = f.store.SettleAttempt(ctx, f.operation.AttemptID, in.Settlement)
			} else {
				_, settleErr = f.store.SettleWorkflowStep(ctx, f.operation.AttemptID, in)
			}
			if settleErr == nil {
				t.Fatal("unacknowledged filesystem permit allowed terminal ownership release")
			}
			attempt, e := f.store.Attempt(ctx, f.operation.AttemptID)
			if e != nil || attempt.Phase == "settled" || attempt.Outcome != nil {
				t.Fatal("rejected settlement partially committed terminal", attempt, e)
			}
			var claim string
			if e = f.store.db.QueryRow("SELECT attempt_id FROM capacity_claims WHERE resource_id='managed-native'").Scan(&claim); e != nil || claim != attempt.ID {
				t.Fatal("rejected settlement lost shared ownership", claim, e)
			}
			rows, active := deliveryDAORows(t, f)
			if len(rows) != 1 || !active.Valid {
				t.Fatal("rejected settlement erased unresolved intent", rows, active)
			}
		})
	}
}

func TestWorkflowDeliveryDAOCommitUncertaintyRetainsExactlyOneIntentOrResult(t *testing.T) {
	for _, stage := range []string{"permit", "acknowledgement"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			f := deliveryDAOFixture(t)
			deliveryDAOBegin(t, f)
			request := f.permit(domain.NewID(), "quarantine_before")
			if stage == "permit" {
				f.store.AfterCommit = func() error { return errors.New("controlled lost DAO intent acknowledgement") }
				_, e := f.store.PermitDeliveryEffect(ctx, request)
				f.store.AfterCommit = nil
				if workflowListErrorCode(e) != "write_uncertain" {
					t.Fatal("committed intent uncertainty became known rollback/success", e)
				}
				rows, active := deliveryDAORows(t, f)
				if len(rows) != 1 || !active.Valid || rows[0].EffectID != request.Effect.EffectID {
					t.Fatal("uncertain intent was not durably identifiable", rows, active)
				}
				if _, e = f.store.PermitDeliveryEffect(ctx, request); e == nil {
					t.Fatal("lost intent acknowledgement admitted another permit")
				}
				again, _ := deliveryDAORows(t, f)
				if len(again) != 1 {
					t.Fatal("intent retry duplicated durable action", again)
				}
			} else {
				intent, e := f.store.PermitDeliveryEffect(ctx, request)
				if e != nil {
					t.Fatal(e)
				}
				intent.State = "applied"
				f.store.AfterCommit = func() error { return errors.New("controlled lost DAO effect acknowledgement") }
				e = f.store.RecordDeliveryEffect(ctx, intent)
				f.store.AfterCommit = nil
				if workflowListErrorCode(e) != "write_uncertain" {
					t.Fatal("committed result uncertainty became known rollback/success", e)
				}
				rows, active := deliveryDAORows(t, f)
				if len(rows) != 2 || active.Valid || rows[1].State != "applied" {
					t.Fatal("known retained result lost after acknowledgement failure", rows, active)
				}
				before := encode(rows)
				_ = f.store.RecordDeliveryEffect(ctx, intent)
				after, _ := deliveryDAORows(t, f)
				if encode(after) != before {
					t.Fatal("same acknowledgement duplicated or rewrote immutable outcome", after)
				}
			}
		})
	}
}

func TestWorkflowDeliveryDAORollbackAndMismatchedAcknowledgementPreserveIntent(t *testing.T) {
	ctx := context.Background()
	f := deliveryDAOFixture(t)
	deliveryDAOBegin(t, f)
	request := f.permit(domain.NewID(), "quarantine_before")
	f.store.BeforeCommit = func() error { return errors.New("controlled DAO rollback before effect intent") }
	_, e := f.store.PermitDeliveryEffect(ctx, request)
	f.store.BeforeCommit = nil
	if e == nil {
		t.Fatal("failed intent commit appeared successful")
	}
	rows, active := deliveryDAORows(t, f)
	if len(rows) != 0 || active.Valid {
		t.Fatal("rolled-back intent retained effect or active slot", rows, active)
	}
	intent, e := f.store.PermitDeliveryEffect(ctx, request)
	if e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*domain.DeliveryEffect){func(effect *domain.DeliveryEffect) { effect.RelativePath = "another.txt" }, func(effect *domain.DeliveryEffect) { effect.Kind = "delete" }, func(effect *domain.DeliveryEffect) {
		copy := *effect.BeforeEvidence
		copy.SHA256 = strings.Repeat("f", 64)
		effect.BeforeEvidence = &copy
	}} {
		ack := intent
		ack.State = "applied"
		change(&ack)
		if e = f.store.RecordDeliveryEffect(ctx, ack); e == nil {
			t.Fatal("different effect acknowledged the captured intent")
		}
	}
	rows, active = deliveryDAORows(t, f)
	if len(rows) != 1 || !active.Valid || !reflect.DeepEqual(rows[0], intent) {
		t.Fatal("mismatched acknowledgements changed original intent", rows, active)
	}
}

func TestWorkflowDeliveryDAOUncertainEffectFencesRatherThanPublishingAgain(t *testing.T) {
	ctx := context.Background()
	f := deliveryDAOFixture(t)
	deliveryDAOBegin(t, f)
	intent, e := f.store.PermitDeliveryEffect(ctx, f.permit(domain.NewID(), "quarantine_before"))
	if e != nil {
		t.Fatal(e)
	}
	ack := intent
	ack.State = "uncertain"
	if e = f.store.RecordDeliveryEffect(ctx, ack); e != nil {
		t.Fatal(e)
	}
	op, e := f.store.DeliveryOperation(ctx, f.operation.ID)
	if e != nil || op.Status != "uncertain" || op.IncidentID == nil {
		t.Fatal("uncertain effect lost retained incident", op, e)
	}
	var state string
	var incident sql.NullString
	if e = f.store.db.QueryRow("SELECT state,incident_id FROM execution_control WHERE singleton=1").Scan(&state, &incident); e != nil || state != "fenced" || !incident.Valid {
		t.Fatal("uncertain effect did not fence conflicting dispatch", state, incident, e)
	}
	if _, e = f.store.PermitDeliveryEffect(ctx, f.permit(intent.EffectID, "publish_after")); e == nil {
		t.Fatal("uncertain quarantine became publication authority")
	}
	rows, active := deliveryDAORows(t, f)
	if len(rows) != 2 || active.Valid || rows[1].IncidentID == nil {
		t.Fatal("uncertain result was erased or fabricated as success", rows, active)
	}
}
