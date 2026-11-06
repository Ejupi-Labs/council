package store

import (
	"context"
	"errors"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

func workflowAdmissionFixture(t *testing.T, s *Store) (string, domain.StartTaskInput, domain.WorkflowAdmissionFacts) {
	t.Helper()
	p := openFixture(t, s, domain.NewID())
	in := taskInput(p.ID)
	in.Workflow = &domain.WorkflowInput{Mode: "auto"}
	policy, e := s.Policy(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	v := resolvedVersion("", policy, in)
	registry, validationErr := workflowpolicy.Conditions(in.OriginalText, v, *in.Workflow, nil)
	if validationErr != nil {
		t.Fatal(validationErr)
	}
	instruction, conditions, e := workflowpolicy.ConditionDigests(in.OriginalText, v, registry, nil)
	if e != nil {
		t.Fatal(e)
	}
	var physical string
	if e = s.db.QueryRow("SELECT physical_identity FROM projects WHERE id=?", p.ID).Scan(&physical); e != nil {
		t.Fatal(e)
	}
	uri := domain.DeliveryScope{LocalMode: "review_only", Source: "default", PreferenceRevision: "1", ProjectID: p.ID, ProjectIdentity: physical, RepositoryIdentity: physical, SelectedRelativePath: ".", Destination: p.ResolvedPath, Branch: p.Branch}
	authority := domain.WorkflowAuthority{ExecutionMode: "fake", FakeScenario: "response", OptionSnapshot: *in.Workflow, InstructionSHA256: instruction, ConditionsSHA256: conditions, Conditions: registry, Delivery: uri, Checks: []domain.CheckDefinition{}, Review: "auto", ReasonCodes: []string{"fixture_bounded_authority"}, Correction: domain.CorrectionPolicy{MaxCycles: 1, Triggers: []string{"blocking_review_finding", "failed_required_check"}}}
	facts := domain.WorkflowAdmissionFacts{Routing: wrapperRoutingFacts("fake"), Capture: &domain.WorkflowCapture{Authority: authority, Step: domain.WorkflowStep{Role: "author", ExecutorKind: "native", LogicalKey: "author:0", Cycle: 0, ReasonCodes: []string{"initial_author"}}}}
	return domain.NewID(), in, facts
}
func workflowCounts(t *testing.T, s *Store) (int, int, int, int) {
	t.Helper()
	var tasks, attempts, steps, workflows int
	for query, target := range map[string]*int{"SELECT count(*) FROM tasks": &tasks, "SELECT count(*) FROM attempts": &attempts, "SELECT count(*) FROM workflow_steps": &steps, "SELECT count(*) FROM workflows": &workflows} {
		if e := s.db.QueryRow(query).Scan(target); e != nil {
			t.Fatal(e)
		}
	}
	return tasks, attempts, steps, workflows
}
func TestWorkflowStepAdmissionAtomicTagsDebitAndExactReplay(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, in, facts := workflowAdmissionFixture(t, s)
	r := s.CreateTaskWithWorkflow(ctx, owner, in, facts)
	if r.Status != 201 {
		t.Fatal("workflow author admission unavailable", r)
	}
	a, ok := r.Envelope.Data.(domain.Admission)
	if !ok || a.Workflow == nil || a.Step == nil {
		t.Fatal("admission lacks workflow/step binding", r)
	}
	if a.Attempt.WorkflowID != a.Workflow.ID || a.Attempt.StepID != a.Step.ID || a.Attempt.Role != "author" || a.Attempt.ExecutorKind != "native" || a.Step.AttemptID == nil || *a.Step.AttemptID != a.Attempt.ID || a.Workflow.Authority.ExecutionMode != "fake" {
		t.Fatal("incorrect owned-stage provenance", a)
	}
	if a.Budget == nil || a.Budget.ConsumedAdmissions != "1" || a.RoutingDecision == nil || a.RoutingDecision.Kind != "manual" {
		t.Fatal("missing exact one-unit native debit", a)
	}
	// Current policy/config/evidence prerequisites must not run before old replay.
	changed := s.SetPolicy(ctx, domain.NewID(), "workflow-replay-policy", "1", "supervised")
	if changed.Status != 200 {
		t.Fatal(changed)
	}
	r = s.CreateTaskWithWorkflow(ctx, domain.NewID(), in, domain.WorkflowAdmissionFacts{})
	if r.Status != 201 || r.Envelope.Receipt == nil {
		t.Fatal("replay required fresh capture/current policy", r)
	}
	if tasks, attempts, steps, workflows := workflowCounts(t, s); tasks != 1 || attempts != 1 || steps != 1 || workflows != 1 {
		t.Fatal("replay duplicated admission", tasks, attempts, steps, workflows)
	}
	in.OriginalText += " different intent"
	r = s.CreateTaskWithWorkflow(ctx, owner, in, facts)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "request_conflict" {
		t.Fatal("same ID accepted different task", r)
	}
}
func TestWorkflowStepAdmissionLostAcknowledgementNeverAddsAnotherUnit(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, in, facts := workflowAdmissionFixture(t, s)
	s.AfterCommit = func() error { return errors.New("lost workflow admission acknowledgement") }
	r := s.CreateTaskWithWorkflow(ctx, owner, in, facts)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" {
		t.Fatal("committed admission lost uncertainty", r)
	}
	s.AfterCommit = nil
	r = s.CreateTaskWithWorkflow(ctx, domain.NewID(), in, domain.WorkflowAdmissionFacts{})
	if r.Status != 201 {
		t.Fatal("known receipt did not replay", r)
	}
	var units int
	if e := s.db.QueryRow("SELECT sum(consumed) FROM task_budgets").Scan(&units); e != nil || units != 1 {
		t.Fatal("lost ack changed charged units", units, e)
	}
	if tasks, attempts, steps, workflows := workflowCounts(t, s); tasks != 1 || attempts != 1 || steps != 1 || workflows != 1 {
		t.Fatal("lost ack duplicated owned work", tasks, attempts, steps, workflows)
	}
}
func TestWorkflowStepAdmissionRollbackAndCapturedModeMismatch(t *testing.T) {
	for _, fault := range []string{"before_commit", "mode_mismatch"} {
		t.Run(fault, func(t *testing.T) {
			s := testStore(t)
			owner, in, facts := workflowAdmissionFixture(t, s)
			if fault == "before_commit" {
				s.BeforeCommit = func() error { return errors.New("known rollback fixture") }
			} else {
				facts.Capture.Authority.ExecutionMode = "native"
			}
			r := s.CreateTaskWithWorkflow(context.Background(), owner, in, facts)
			if r.Status == 201 || r.Envelope.Error == nil {
				t.Fatal("invalid/failed authority admitted work", r)
			}
			s.BeforeCommit = nil
			if tasks, attempts, steps, workflows := workflowCounts(t, s); tasks != 0 || attempts != 0 || steps != 0 || workflows != 0 {
				t.Fatal("failed transaction retained partial authority/debit", tasks, attempts, steps, workflows)
			}
		})
	}
}
