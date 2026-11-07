package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// An automatic replacement and an exact pause race at the real SQLite write
// boundary. A committed replacement must be captured in the checkpoint; a
// replacement that loses the race must not create a successor or spend budget.
func TestRecoveryPauseRacesRoutingContinuation(t *testing.T) {
	ctx := context.Background()
	for round := 0; round < 4; round++ {
		s := testStore(t)
		owner, first, facts := routingBlockedAutomatic(t, s)
		task, err := s.Task(ctx, first.Task.ID)
		if err != nil {
			t.Fatal(err)
		}
		input := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: task.Task.CurrentVersion, ExpectedTaskRevision: task.Task.Revision}
		start := make(chan struct{})
		pauses := make(chan domain.Result, 1)
		type continuation struct {
			admission *domain.Admission
			err       error
		}
		continuations := make(chan continuation, 1)
		go func() { <-start; pauses <- s.PauseTask(ctx, owner, first.Task.ID, input) }()
		go func() {
			<-start
			a, e := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
			continuations <- continuation{a, e}
		}()
		close(start)
		pause, continued := <-pauses, <-continuations
		if pause.Envelope.Error == nil {
			cp := recoveryPauseResult(t, pause).Checkpoint
			if continued.admission != nil && !recoveryContains(cp.AdmittedAttemptIDs, continued.admission.Attempt.ID) {
				t.Fatal("pause accepted without capturing an earlier continuation", cp, continued.admission)
			}
			next, err := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
			if err == nil && next != nil {
				t.Fatal("paused scope admitted a new routing successor", next)
			}
			var count int
			if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM routing_decisions WHERE previous_attempt_id=? AND kind='automatic'", first.Attempt.ID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			expected := 0
			if continued.admission != nil {
				expected = 1
			}
			if count != expected {
				t.Fatal("pause race duplicated or invented routing continuation", count, expected)
			}
		} else if continued.err != nil || continued.admission == nil {
			t.Fatal("both controls lost without an accepted concurrent successor", pause, continued)
		}
	}
}

// A delivery stage admitted before pause may finish under its original exact
// authority. Pause does not authorize an additional stage or new work.
func TestRecoveryPauseDrainsPreviouslyAdmittedDeliveryAuthority(t *testing.T) {
	ctx := context.Background()
	f := deliveryDAOFixture(t)
	recoveryRegisterFixtureOwner(t, f.store, f.owner)
	deliveryDAOBegin(t, f)
	id := domain.NewID()
	authorized, err := f.store.PermitDeliveryEffect(ctx, f.permit(id, "quarantine_before"))
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.store.Task(ctx, f.admission.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	pause := f.store.PauseTask(ctx, f.owner, task.Task.ID, domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: task.Task.CurrentVersion, ExpectedTaskRevision: task.Task.Revision})
	cp := recoveryPauseResult(t, pause).Checkpoint
	if cp.State != "pausing" {
		t.Fatal("active delivery was not retained as draining", cp)
	}
	authorized.State = "applied"
	if err := f.store.RecordDeliveryEffect(ctx, authorized); err != nil {
		t.Fatal("previously authorized result could not drain", err)
	}
	publication, err := f.store.PermitDeliveryEffect(ctx, f.permit(id, "publish_after"))
	if err != nil {
		t.Fatal("the admitted exact delivery could not finish", err)
	}
	publication.State = "applied"
	if err = f.store.RecordDeliveryEffect(ctx, publication); err != nil {
		t.Fatal(err)
	}
	rows, active := deliveryDAORows(t, f)
	if len(rows) != 4 || active.Valid {
		t.Fatal("draining delivery lost its exact journal", rows, active)
	}
	attempts, err := f.store.Attempts(ctx, task.Task.ID, 100, "")
	if err != nil || len(attempts.Items) != 1 || attempts.Items[0].ID != f.admission.Attempt.ID {
		t.Fatal("drain invented another attempt", attempts, err)
	}
}

// A settled pre-pause author cannot admit its next workflow stage while the
// accepted checkpoint remains paused.
func TestRecoveryPauseRejectsNewWorkflowStage(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, settlement := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	task, err := s.Task(ctx, admission.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	cp := recoveryPauseResult(t, s.PauseTask(ctx, owner, task.Task.ID, domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: task.Task.CurrentVersion, ExpectedTaskRevision: task.Task.Revision})).Checkpoint
	workflow, err := s.SettleWorkflowStep(ctx, admission.Attempt.ID, settlement)
	if err != nil {
		t.Fatal(err)
	}
	step, err := s.WorkflowStep(ctx, admission.Step.ID)
	if err != nil || step.AdvanceProof == nil {
		t.Fatal(step, err)
	}
	plan := domain.WorkflowStagePlan{Role: "check", ExecutorKind: "local_check", LogicalKey: step.AdvanceProof.NextLogicalKey, Cycle: 0, DefinitionID: &workflow.Authority.Checks[0].ID, InputCandidateID: workflow.SelectedCandidateID, PredecessorStepID: &step.ID}
	next, err := s.ContinueWorkflow(ctx, owner, step.ID, domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: task.Task.Revision, ExpectedWorkflowRevision: workflow.Revision})
	if err == nil && next != nil {
		t.Fatal("paused scope admitted a new workflow stage", next)
	}
	current, err := s.Checkpoint(ctx, cp.ID)
	if err != nil || current.State != "paused" {
		t.Fatal("settled scope lost its pause", current, err)
	}
	if _, attempts, steps, _ := workflowCounts(t, s); attempts != 1 || steps != 1 {
		t.Fatal("paused scope invented a new attempt or stage", attempts, steps)
	}
}

// The pause and first effect permit contend for the same transactional store.
// Both orderings and a simultaneous start preserve only the admitted stage's
// exact owner and scope. No additional attempt is admitted by either control.
func TestRecoveryPauseAndDeliveryAuthorizationSerialized(t *testing.T) {
	for _, ordering := range []string{"pause-first", "permit-first", "concurrent"} {
		t.Run(ordering, func(t *testing.T) {
			ctx := context.Background()
			f := deliveryDAOFixture(t)
			recoveryRegisterFixtureOwner(t, f.store, f.owner)
			deliveryDAOBegin(t, f)
			task, err := f.store.Task(ctx, f.admission.Task.ID)
			if err != nil {
				t.Fatal(err)
			}
			input := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: task.Task.CurrentVersion, ExpectedTaskRevision: task.Task.Revision}
			facts := f.permit(domain.NewID(), "quarantine_before")
			pauseStart, permitStart := make(chan struct{}), make(chan struct{})
			pauses := make(chan domain.Result, 1)
			type permitResult struct {
				effect domain.DeliveryEffect
				err    error
			}
			permits := make(chan permitResult, 1)
			go func() { <-pauseStart; pauses <- f.store.PauseTask(ctx, f.owner, task.Task.ID, input) }()
			go func() {
				<-permitStart
				effect, err := f.store.PermitDeliveryEffect(ctx, facts)
				permits <- permitResult{effect, err}
			}()
			var pause domain.Result
			var permitted permitResult
			switch ordering {
			case "pause-first":
				close(pauseStart)
				pause = <-pauses
				close(permitStart)
				permitted = <-permits
			case "permit-first":
				close(permitStart)
				permitted = <-permits
				close(pauseStart)
				pause = <-pauses
			default:
				close(pauseStart)
				close(permitStart)
				pause, permitted = <-pauses, <-permits
			}
			cp := recoveryPauseResult(t, pause).Checkpoint
			if cp.State != "pausing" || !recoveryContains(cp.AdmittedAttemptIDs, f.admission.Attempt.ID) {
				t.Fatal("pause lost admitted delivery", cp)
			}
			if permitted.err != nil || permitted.effect.OperationID != f.operation.ID || permitted.effect.EffectID != facts.Effect.EffectID {
				t.Fatal("exact captured delivery authority did not drain", permitted)
			}
			unauthorized := f.permit(domain.NewID(), "quarantine_before")
			unauthorized.OwnerInstanceID = domain.NewID()
			if effect, err := f.store.PermitDeliveryEffect(ctx, unauthorized); workflowListErrorCode(err) != "workflow_revision_conflict" {
				t.Fatal("pause race failed to reject the unrelated owner", effect, err)
			}
			permitted.effect.State = "applied"
			if err := f.store.RecordDeliveryEffect(ctx, permitted.effect); err != nil {
				t.Fatal("accepted effect could not settle", err)
			}
			rows, active := deliveryDAORows(t, f)
			if len(rows) != 2 || active.Valid {
				t.Fatal("pause race created extra effect or lost result", rows, active)
			}
			history, err := f.store.Attempts(ctx, task.Task.ID, 100, "")
			if err != nil || len(history.Items) != 1 || history.Items[0].ID != f.admission.Attempt.ID {
				t.Fatal("pause race admitted another attempt", history, err)
			}
		})
	}
}
