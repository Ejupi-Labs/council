package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphAdvanceInput(view domain.GraphDetail) domain.GraphAdvanceInput {
	in := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}}
	for _, n := range view.Nodes {
		in.NodeAuthorities = append(in.NodeAuthorities, domain.GraphNodeAuthority{NodeID: n.ID, TaskVersion: n.TaskVersion, NodeControlRevision: n.ControlRevision})
	}
	return in
}

func TestGraphAdvanceRegistersFreshIntentWithoutReplayingAcceptedWork(t *testing.T) {
	s, before, preparation, owner := graphDependentInputFixture(t)
	ctx := context.Background()
	in := graphAdvanceInput(before)
	var attempts int
	if e := s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&attempts); e != nil {
		t.Fatal(e)
	}
	result := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{})
	if result.Status != 200 {
		t.Fatal("explicit fresh registration unavailable", result)
	}
	after, e := s.Graph(ctx, before.Graph.ID)
	if e != nil || after.DispatchAuthority == nil || after.DispatchAuthority.OwnerInstanceID != owner || after.DispatchAuthority.Generation != "2" || after.DispatchAuthority.State != "active" || after.DispatchAuthority.BudgetAuthorityEpoch != after.Graph.BudgetAuthorityEpoch || !reflect.DeepEqual(after.Budget, before.Budget) {
		t.Fatal("Advance did not bind current controls or changed allowance", after, e)
	}
	if !reflect.DeepEqual(after.Nodes[0].AcceptedResult, before.Nodes[0].AcceptedResult) || after.Nodes[0].TaskVersion != before.Nodes[0].TaskVersion || after.Nodes[0].State != "completed" {
		t.Fatal("Advance reran or replaced completed work without explicit rebind", before.Nodes[0], after.Nodes[0])
	}
	var current int
	if e = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&current); e != nil || current != attempts {
		t.Fatal("registration manufactured native/preparation work without retained facts", attempts, current, e)
	}
	// Prior committed ack/terminal events cannot masquerade as the new user
	// intent. A deliberate explicit trigger can use the same current result.
	preparation.Control.Generation = after.DispatchAuthority.Generation
	preparation.Control.GraphControlRevision = after.Graph.ControlRevision
	preparation.Trigger.OwnerGeneration = after.DispatchAuthority.Generation
	if _, e = s.BeginGraphInput(ctx, owner, after.Graph.ID, after.Nodes[1].ID, preparation); e == nil {
		t.Fatal("old response acknowledgement rearmed under a newer registration")
	}
	preparation.Trigger.Kind, preparation.Trigger.ID = "explicit", in.RequestID
	op, e := s.BeginGraphInput(ctx, owner, after.Graph.ID, after.Nodes[1].ID, preparation)
	if e != nil || op.ID == "" || op.Contributions[0].AcceptanceID != before.Nodes[0].AcceptedResult.AcceptanceID {
		t.Fatal("new explicit authority could not prepare its exact current retained dependency", op, e)
	}
	if replay := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{}); replay.Status != 200 {
		t.Fatal("exact Advance replay followed changed current state", replay)
	}
	replayed, e := s.Graph(ctx, before.Graph.ID)
	if e != nil || replayed.DispatchAuthority.Generation != after.DispatchAuthority.Generation || replayed.Nodes[1].InputID == nil || *replayed.Nodes[1].InputID != op.ID {
		t.Fatal("Advance replay refreshed authority or repeated preparation", replayed, e)
	}
}

func TestGraphAdvanceRejectsStaleOrForeignCapturedAuthorityBeforeRegistration(t *testing.T) {
	for _, kind := range []string{"owner", "graph_version", "graph_control", "budget_epoch", "window", "node_version", "node_control", "foreign_node"} {
		t.Run(kind, func(t *testing.T) {
			s, graph, _, _, owner := graphReadyBasis(t)
			ctx := context.Background()
			before, e := s.Graph(ctx, graph.Graph.ID)
			if e != nil {
				t.Fatal(e)
			}
			in := graphAdvanceInput(before)
			switch kind {
			case "owner":
				owner = domain.NewID()
			case "graph_version":
				in.GraphVersion = "2"
			case "graph_control":
				in.ExpectedGraphControlRevision = "2"
			case "budget_epoch":
				in.ExpectedBudgetAuthorityEpoch = "2"
			case "window":
				in.ExpectedWindowNumber = "2"
			case "node_version":
				in.NodeAuthorities[0].TaskVersion = "2"
			case "node_control":
				in.NodeAuthorities[0].NodeControlRevision = "2"
			case "foreign_node":
				in.NodeAuthorities[0].NodeID = domain.NewID()
			}
			if result := s.AdvanceGraph(ctx, owner, graph.Graph.ID, in, domain.GraphAdvanceFacts{}); result.Envelope.Error == nil || result.Envelope.Receipt != nil {
				t.Fatal("stale/foreign captured intent registered current work", kind, result)
			}
			after, e := s.Graph(ctx, graph.Graph.ID)
			if e != nil || !reflect.DeepEqual(before.DispatchAuthority, after.DispatchAuthority) || !reflect.DeepEqual(before.Budget, after.Budget) {
				t.Fatal("refused Advance partly changed authority", before, after, e)
			}
		})
	}
}
