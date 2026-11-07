package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
)

func TestGraphFreshReviewedCandidateCompletionPreparesExactDependentInput(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx, e.cancel = ctx, cancel
	edges := []domain.GraphDependencyInput{{From: "producer", To: "receiver", Kind: "files"}}
	group, source := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{Nodes: []domain.GraphNodeInput{
		{Key: "producer", OriginalText: "Author and independently review the exact controlled file contribution.", FakeScenario: "graph-branch-a", Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect the entire authored file change."}},
		{Key: "receiver", OriginalText: "Receive the exact accepted file contribution before authoring the next controlled change.", FakeScenario: "graph-branch-b"},
	}, Dependencies: &edges})
	e.continueGraphAdmissions(source)
	view, err := e.db.Graph(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	var producer, receiver domain.GraphNodeSummary
	for _, node := range view.Nodes {
		if node.Key == "producer" {
			producer = node
		} else {
			receiver = node
		}
	}
	if producer.LatestAttemptID == nil || receiver.InputID != nil {
		t.Fatal("The declared receiver was not held behind its exact unaccepted predecessor", view)
	}
	author, err := e.db.ClaimAttempt(ctx, *producer.LatestAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, author)
	e.continueWorkflow(author.ID)
	e.continueGraphInputs(author.ID)
	beforeReview, err := e.db.Graph(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range beforeReview.Nodes {
		if node.ID == receiver.ID && node.InputID != nil {
			t.Fatal("Unreviewed output qualified a dependent input", node)
		}
	}
	w, err := e.db.Workflow(ctx, author.TaskID, domain.WorkflowSelector{WorkflowID: author.WorkflowID})
	if err != nil || w.Workflow == nil || w.Workflow.CurrentStepID == nil {
		t.Fatal("The actual authored candidate did not queue its independent review", w, err)
	}
	step, err := e.db.WorkflowStep(ctx, *w.Workflow.CurrentStepID)
	if err != nil || step.Role != "review" || step.AttemptID == nil {
		t.Fatal("The actual required review is unavailable", step, err)
	}
	review, err := e.db.ClaimAttempt(ctx, *step.AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, review)
	e.continueWorkflow(review.ID)
	e.continueGraphInputs(review.ID)
	accepted, err := e.db.Graph(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range accepted.Nodes {
		if node.ID == producer.ID {
			producer = node
		} else if node.ID == receiver.ID {
			receiver = node
		}
	}
	if producer.AcceptedResult == nil || producer.AcceptedResult.AcceptanceKind != "workflow" || producer.AcceptedResult.ProducerAttemptID != author.ID || producer.LatestAttemptID == nil || *producer.LatestAttemptID != review.ID {
		t.Fatal("The real review did not accept its exact original authored candidate", producer)
	}
	if receiver.InputID == nil {
		t.Fatal("The fresh actual reviewed completion did not prepare its registered dependent input", receiver)
	}
	op, err := e.db.GraphInputRecord(ctx, group.Graph.ID, *receiver.InputID)
	if err != nil || op.Trigger.Kind != "completion" || op.Trigger.ID != review.ID {
		t.Fatal("Dependent preparation did not consume the actual current review completion", op, err)
	}
	local, err := e.db.ClaimAttempt(ctx, op.Public.Summary.OperationAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, local)
	input, err := e.db.GraphInput(ctx, group.Graph.ID, *receiver.InputID)
	if err != nil || input.Summary.State != "ready" || input.InputBinding == nil || len(input.InputBinding.Contributions) != 1 || input.ContextPacket == nil {
		t.Fatal("Actual accepted candidate composition did not produce a complete dependent input", input, err)
	}
	c := input.InputBinding.Contributions[0]
	if c.EdgeKind != "files" || c.ProducerAttemptID != author.ID || c.AcceptanceID != producer.AcceptedResult.AcceptanceID || c.CandidateBinding == nil || c.ResponseBinding != nil {
		t.Fatal("The receiver selected supporting review output instead of the accepted author", c)
	}
	verified, err := e.workspace.VerifyGraphInput(ctx, *input.InputBinding)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range verified.Manifest.Entries {
		if entry.Path == fixture.GraphFileA {
			found = true
		}
	}
	if !found {
		t.Fatal("The actual files edge lost the accepted authored file", verified.Manifest)
	}
	before, err := e.db.GraphInputs(ctx, group.Graph.ID, 30, "")
	if err != nil {
		t.Fatal(err)
	}
	e.continueGraphInputs(author.ID)
	e.continueGraphInputs(review.ID)
	after, err := e.db.GraphInputs(ctx, group.Graph.ID, 30, "")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("Old author/review callbacks duplicated accepted dependent preparation", after, err)
	}
	budget, err := e.db.GraphBudget(ctx, group.Graph.ID)
	if err != nil || budget.Budget.ConsumedAdmissions != "2" {
		t.Fatal("Local composition consumed another model admission", budget, err)
	}
}
