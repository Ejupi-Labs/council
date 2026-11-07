package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphGuidanceInput(view domain.GraphDetail, task domain.TaskDetail) domain.GraphGuidanceInput {
	var node domain.GraphNodeSummary
	for _, n := range view.Nodes {
		if n.TaskID == task.Task.ID {
			node = n
		}
	}
	return domain.GraphGuidanceInput{GuidanceInput: domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: task.Task.Revision, GuidanceText: "Exact new receiver guidance"}, ExpectedGraphControlRevision: view.Graph.ControlRevision, NodeID: node.ID, ExpectedNodeControlRevision: node.ControlRevision, TaskVersion: task.Version.Version}
}

func TestGraphGuidanceCapturesVersionChoicesWithoutRefillOrAutomaticWork(t *testing.T) {
	s, graph, originalOptions, _ := graphVersionOptionsFixture(t)
	ctx := context.Background()
	before, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	task, e := s.Task(ctx, graph.Nodes[0].TaskID)
	if e != nil {
		t.Fatal(e)
	}
	gate, e := s.Gate(ctx)
	if e != nil {
		t.Fatal(e)
	}
	in := graphGuidanceInput(before, task)
	var attempts int
	if e = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&attempts); e != nil {
		t.Fatal(e)
	}
	result := s.UpdateGraphNodeGuidance(ctx, gate.OwnerInstanceID, graph.Graph.ID, graph.Nodes[0].Key, in, domain.GraphAdvanceFacts{})
	if result.Status != 200 {
		t.Fatal("graph guidance version unavailable", result)
	}
	current, e := s.Task(ctx, task.Task.ID)
	if e != nil || current.Version.Version != "2" || current.Version.GuidanceText != in.GuidanceText || current.Version.CreatedBy != "user_guidance" || current.Task.OriginalText != task.Task.OriginalText || !reflect.DeepEqual(current.Version.Pins, task.Version.Pins) || !reflect.DeepEqual(current.Version.Acceptance, task.Version.Acceptance) || !reflect.DeepEqual(current.Version.RequestedPolicy, task.Version.RequestedPolicy) || current.Version.TimeoutSeconds != task.Version.TimeoutSeconds {
		t.Fatal("guidance changed unrelated captured task choices", current, e)
	}
	options, e := s.GraphNodeOptions(ctx, graph.Graph.ID, graph.Nodes[0].ID, "2")
	expectedWorkflow := *originalOptions.Workflow
	expectedWorkflow.Mode = "auto"
	if e != nil || options.RoutingBasis == nil || !reflect.DeepEqual(options.Workflow, &expectedWorkflow) || options.Workflow.Checks == nil || len(*options.Workflow.Checks) != 0 || options.Workflow.MaxCorrectionCycles == nil || *options.Workflow.MaxCorrectionCycles != 0 || options.ExecutionMode != originalOptions.ExecutionMode || options.FakeScenario != originalOptions.FakeScenario {
		t.Fatal("omitted workflow choices or explicit empty/zero were replaced", options, e)
	}
	after, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil || after.Nodes[0].TaskVersion != "2" || after.Nodes[0].ControlRevision == before.Nodes[0].ControlRevision || after.Nodes[0].NextOwner != "user" || !reflect.DeepEqual(after.Nodes[1], before.Nodes[1]) || !reflect.DeepEqual(after.Budget, before.Budget) || after.DispatchAuthority.Generation != before.DispatchAuthority.Generation {
		t.Fatal("guidance refilled, registered or invalidated an independent sibling", before, after, e)
	}
	for _, authority := range after.DispatchAuthority.NodeAuthorities {
		if authority.NodeID == after.Nodes[0].ID && authority.TaskVersion == "2" {
			t.Fatal("guidance silently authorised its new version", after.DispatchAuthority)
		}
	}
	var count int
	if e = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&count); e != nil || count != attempts {
		t.Fatal("guidance dispatched native/local work", attempts, count, e)
	}
	if replay := s.UpdateGraphNodeGuidance(ctx, gate.OwnerInstanceID, graph.Graph.ID, graph.Nodes[0].Key, in, domain.GraphAdvanceFacts{}); replay.Status != 200 {
		t.Fatal("guidance replay chased current controls", replay)
	}
}

func TestGraphGuidanceFreezesRoutingPolicyUntilFreshInputAdmission(t *testing.T) {
	s, graph, root, rootProofs, owner := graphReadyBasis(t)
	ctx := context.Background()
	if changed := s.SetRoutingPolicy(ctx, domain.RoutingPolicyInput{RequestID: domain.NewID(), ExpectedRevision: "1", UnknownAvailability: "allow"}); changed.Status != 200 {
		t.Fatal(changed)
	}
	view, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	task, e := s.Task(ctx, view.Nodes[0].TaskID)
	if e != nil {
		t.Fatal(e)
	}
	in := graphGuidanceInput(view, task)
	useCurrent := true
	in.Routing = &domain.RoutingOptions{UseCurrentPolicy: &useCurrent, ExpectedPolicyRevision: "2"}
	if guided := s.UpdateGraphNodeGuidance(ctx, owner, graph.Graph.ID, view.Nodes[0].Key, in, domain.GraphAdvanceFacts{}); guided.Status != 200 {
		t.Fatal("actual graph routing-guidance transaction unavailable", guided)
	}
	options, e := s.GraphNodeOptions(ctx, graph.Graph.ID, view.Nodes[0].ID, "2")
	if e != nil || options.RoutingBasis == nil || options.RoutingBasis.PolicySnapshot.Revision != "2" || options.RoutingBasis.PolicySnapshot.UnknownAvailability != "allow" {
		t.Fatal("guidance did not freeze its explicit current policy once", options, e)
	}
	if changed := s.SetRoutingPolicy(ctx, domain.RoutingPolicyInput{RequestID: domain.NewID(), ExpectedRevision: "2", UnknownAvailability: "require_positive"}); changed.Status != 200 {
		t.Fatal(changed)
	}
	view, e = s.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	advanceInput := graphAdvanceInput(view)
	if advanced := s.AdvanceGraph(ctx, owner, graph.Graph.ID, advanceInput, domain.GraphAdvanceFacts{}); advanced.Status != 200 {
		t.Fatal(advanced)
	}
	view, e = s.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	node := view.Nodes[0]
	begin := graphBeginFacts(domain.GraphAdmission{Graph: view.Graph}, node, root, rootProofs, owner)
	begin.Trigger = domain.GraphFreshTrigger{Kind: "explicit", ID: advanceInput.RequestID, OwnerGeneration: view.DispatchAuthority.Generation}
	begin.Control.Generation = view.DispatchAuthority.Generation
	facts, trigger := graphReadyNodeFacts(t, s, domain.GraphAdmission{Graph: view.Graph}, root, rootProofs, owner, node, begin)
	facts.Routing.Assessment = domain.SuitabilityAssessment{Classification: "routine", Source: "recognized", ReasonCodes: []string{"controlled_fresh_receiving_input"}}
	dispatch, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, facts)
	if e != nil || dispatch.Dispatch.ID == "" {
		t.Fatal("new global require-positive policy replaced the captured guidance policy", dispatch, e)
	}
	authority, e := s.routingAuthority(ctx, s.db, node.TaskID, "2")
	if e != nil || authority == nil || authority.PolicySnapshot.Revision != "2" || authority.PolicySnapshot.UnknownAvailability != "allow" || authority.SuitabilityAssessment.Classification != "routine" || authority.SuitabilityAssessment.Source != "recognized" {
		t.Fatal("admission lost frozen policy or freshly observed suitability", authority, e)
	}
	budget, e := s.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || budget.Budget.ConsumedAdmissions != "1" || budget.Budget.TaskID != graph.Graph.RootTaskID {
		t.Fatal("guidance/admission created another finite child budget", budget, e)
	}
}

func TestGraphGuidanceWorkflowPatchPreservesOmittedChecksAndZeroCorrection(t *testing.T) {
	checks := []domain.CheckDeclaration{{Key: "compile", Executable: "go", Arguments: []string{"test", "./..."}}}
	zero := 0
	s, graph, _, _, owner := graphReadyBasis(t, func(in *domain.GraphStartInput) {
		in.Nodes[0].Workflow = &domain.WorkflowInput{Review: "auto", Checks: &checks, MaxCorrectionCycles: &zero}
	})
	ctx := context.Background()
	view, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	task, e := s.Task(ctx, graph.Nodes[0].TaskID)
	if e != nil {
		t.Fatal(e)
	}
	in := graphGuidanceInput(view, task)
	in.Workflow = &domain.WorkflowInput{Review: "required"}
	if result := s.UpdateGraphNodeGuidance(ctx, owner, graph.Graph.ID, graph.Nodes[0].Key, in, domain.GraphAdvanceFacts{}); result.Status != 200 {
		t.Fatal(result)
	}
	options, e := s.GraphNodeOptions(ctx, graph.Graph.ID, graph.Nodes[0].ID, "2")
	if e != nil || options.Workflow == nil || options.Workflow.Review != "required" || options.Workflow.Mode != "auto" || options.Workflow.Checks == nil || !reflect.DeepEqual(*options.Workflow.Checks, checks) || options.Workflow.MaxCorrectionCycles == nil || *options.Workflow.MaxCorrectionCycles != 0 {
		t.Fatal("partial review patch replaced omitted captured checks or zero correction", options, e)
	}
	view, e = s.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	task, e = s.Task(ctx, graph.Nodes[0].TaskID)
	if e != nil {
		t.Fatal(e)
	}
	clear := graphGuidanceInput(view, task)
	empty := []domain.CheckDeclaration{}
	clear.Workflow = &domain.WorkflowInput{Checks: &empty}
	if result := s.UpdateGraphNodeGuidance(ctx, owner, graph.Graph.ID, graph.Nodes[0].Key, clear, domain.GraphAdvanceFacts{}); result.Status != 200 {
		t.Fatal(result)
	}
	options, e = s.GraphNodeOptions(ctx, graph.Graph.ID, graph.Nodes[0].ID, "3")
	if e != nil || options.Workflow == nil || options.Workflow.Checks == nil || len(*options.Workflow.Checks) != 0 || options.Workflow.Review != "required" || options.Workflow.MaxCorrectionCycles == nil || *options.Workflow.MaxCorrectionCycles != 0 {
		t.Fatal("explicit empty check patch was dropped or reset omitted choices", options, e)
	}
}

func TestGraphGuidanceInvalidatesOnlyAffectedCurrentInputsAndRetainsOldAcceptance(t *testing.T) {
	s, dependent, completion, owner := graphDependentCompletionFixture(t, "", func(in *domain.GraphStartInput) {
		in.Nodes = append(in.Nodes, domain.GraphNodeInput{Key: "independent", OriginalText: "Exact independent receiver"})
	})
	ctx := context.Background()
	before, e := s.Graph(ctx, dependent.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	accepted := *before.Nodes[0].AcceptedResult
	var terminal string
	if e = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", accepted.ProducerAttemptID).Scan(&terminal); e != nil {
		t.Fatal(e)
	}
	task, e := s.Task(ctx, before.Nodes[0].TaskID)
	if e != nil {
		t.Fatal(e)
	}
	in := graphGuidanceInput(before, task)
	if guided := s.UpdateGraphNodeGuidance(ctx, owner, before.Graph.ID, before.Nodes[0].Key, in, domain.GraphAdvanceFacts{}); guided.Status != 200 {
		t.Fatal(guided)
	}
	after, e := s.Graph(ctx, before.Graph.ID)
	if e != nil || after.Nodes[0].TaskVersion != "2" || after.Nodes[0].AcceptedResult != nil || after.Nodes[1].TaskVersion != "1" || after.Nodes[1].ControlRevision == before.Nodes[1].ControlRevision || !reflect.DeepEqual(after.Nodes[2], before.Nodes[2]) || !reflect.DeepEqual(after.Budget, before.Budget) {
		t.Fatal("guidance retained stale authority or invalidated independent work", before, after, e)
	}
	if e = s.CheckGraphLocalOperation(ctx, owner, dependent.Attempt.ID); e == nil {
		t.Fatal("claimed receiver continued after its accepted ancestor version changed")
	}
	if e = s.CompleteGraphInput(ctx, owner, dependent.Attempt.ID, completion); e != nil {
		t.Fatal("actual stale prefix could not settle", e)
	}
	input, e := s.GraphInput(ctx, before.Graph.ID, dependent.ID)
	if e != nil || input.Summary.Current || input.Summary.State == "ready" {
		t.Fatal("late completion rearmed stale received input", input, e)
	}
	var kept string
	var acknowledgements int
	if e = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", accepted.ProducerAttemptID).Scan(&kept); e != nil || kept != terminal {
		t.Fatal("guidance rewrote old accepted producer", e)
	}
	if e = s.db.QueryRow("SELECT count(*) FROM response_acceptances WHERE id=?", accepted.AcceptanceID).Scan(&acknowledgements); e != nil || acknowledgements != 1 {
		t.Fatal("guidance deleted historical acknowledgement", acknowledgements, e)
	}
	current, e := s.Graph(ctx, before.Graph.ID)
	if e != nil || !reflect.DeepEqual(current.Nodes[2], before.Nodes[2]) {
		t.Fatal("late dependent settlement retconned independent sibling", current, e)
	}
}
