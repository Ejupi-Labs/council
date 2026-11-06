package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"reflect"
	"testing"
	"time"
)

func TestWorkflowKnownContinuationBlockRequiresExplicitCurrentAuthority(t *testing.T) {
	for _, reason := range []string{"budget_exhausted", "budget_expired", "routing_unavailable"} {
		t.Run(reason, func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			a, owner, settlement := workflowAuthorSettlementFixture(t, s, func(in *domain.StartTaskInput, _ *domain.WorkflowAdmissionFacts) {
				in.Workflow.Review = "required"
				if reason == "budget_exhausted" {
					in.Routing = &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "1"}}
				}
			})
			w, e := s.SettleWorkflowStep(ctx, a.Attempt.ID, settlement)
			if e != nil {
				t.Fatal(e)
			}
			step, e := s.WorkflowStep(ctx, a.Step.ID)
			if e != nil || step.AdvanceProof == nil {
				t.Fatal(step, e)
			}
			immutable := step
			if reason == "budget_expired" {
				if e = s.executionWrite(ctx, func(tx *sql.Tx) error {
					b, e := s.taskBudget(ctx, tx, a.Task.ID)
					if e != nil {
						return e
					}
					past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
					b.CurrentWindow.DeadlineAt = &past
					return saveBudget(ctx, tx, *b)
				}); e != nil {
					t.Fatal(e)
				}
			}
			if reason == "routing_unavailable" {
				no := false
				for _, id := range []string{"codex-native", "claude-native"} {
					r := s.UpdateRoutingContext(ctx, id, domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: &no})
					if r.Status != 200 {
						t.Fatal(r)
					}
				}
			}
			before, e := s.TaskBudget(ctx, a.Task.ID)
			if e != nil {
				t.Fatal(e)
			}
			plan := domain.WorkflowStagePlan{Role: "review", ExecutorKind: "native", Cycle: 0, LogicalKey: step.AdvanceProof.NextLogicalKey, InputCandidateID: w.SelectedCandidateID, PredecessorStepID: &step.ID}
			facts := domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: "1", ExpectedWorkflowRevision: w.Revision, Routing: wrapperRoutingFacts("fake")}
			next, e := s.ContinueWorkflow(ctx, owner, step.ID, facts)
			var fault *domain.Error
			if next != nil || !errors.As(e, &fault) || fault.Code != reason {
				t.Fatal("wrong known admission block", next, e)
			}
			current, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{})
			if e != nil || current.Workflow.NextOwner != "user" || current.Workflow.NextAction != reason || current.Workflow.Revision == w.Revision {
				t.Fatal("unadmitted stage still promises Council work or keeps automatic proof fresh", current, e)
			}
			after, e := s.TaskBudget(ctx, a.Task.ID)
			if e != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("failed admission debited/refilled authority", before, after, e)
			}
			old, e := s.WorkflowStep(ctx, step.ID)
			if e != nil || !reflect.DeepEqual(old, immutable) {
				t.Fatal("blocked admission rewrote predecessor/proof", old, e)
			}
			if _, attempts, steps, _ := workflowCounts(t, s); attempts != 1 || steps != 1 {
				t.Fatal("known failure created work", attempts, steps)
			}
			if reason == "budget_exhausted" {
				detail, e := s.Task(ctx, a.Task.ID)
				if e != nil {
					t.Fatal(e)
				}
				grant := domain.BudgetAmendmentInput{RequestID: domain.NewID(), Action: "grant", ExpectedRevision: detail.Task.Revision, ExpectedBudgetRevision: after.Budget.Revision, AdditionalAdmissions: "1"}
				if result := s.AmendBudget(ctx, a.Task.ID, grant); result.Status != 200 {
					t.Fatal(result)
				}
				repeated, e := s.ContinueWorkflow(ctx, owner, step.ID, facts)
				if e != nil || repeated != nil {
					t.Fatal("grant rearmed old automatic proof", repeated, e)
				}
				detail, e = s.Task(ctx, a.Task.ID)
				if e != nil {
					t.Fatal(e)
				}
				current, e = s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{})
				if e != nil {
					t.Fatal(e)
				}
				facts.ExpectedTaskRevision, facts.ExpectedWorkflowRevision = detail.Task.Revision, current.Workflow.Revision
				explicit := domain.WorkflowAdvanceInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, WorkflowRevision: current.Workflow.Revision, TaskVersion: current.TaskVersion}
				result := s.AdvanceWorkflow(ctx, owner, a.Task.ID, explicit, facts)
				if result.Status != 201 {
					t.Fatal("explicit current authority could not advance", result)
				}
			}
		})
	}
}
