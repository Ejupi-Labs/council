package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphAdvancePreparesCurrentUnstartedVersionWithoutRebindOrNativeDebit(t *testing.T) {
	s, graph, root, proofs, owner := graphReadyBasis(t)
	ctx := context.Background()
	view, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Task(ctx, view.Nodes[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	guidance := graphGuidanceInput(view, task)
	if guided := s.UpdateGraphNodeGuidance(ctx, owner, graph.Graph.ID, view.Nodes[0].Key, guidance, domain.GraphAdvanceFacts{}); guided.Status != 200 {
		t.Fatal(guided)
	}
	view, err = s.Graph(ctx, graph.Graph.ID)
	if err != nil || view.Nodes[0].TaskVersion != "2" || view.Nodes[0].InputID != nil || view.Nodes[0].NodeAdmissionID != nil {
		t.Fatal("fixture lacks an ordinary current version awaiting its first input", view, err)
	}
	var beforeVersion string
	if err = s.db.QueryRow("SELECT data FROM task_versions WHERE task_id=? AND version=2", task.Task.ID).Scan(&beforeVersion); err != nil {
		t.Fatal(err)
	}
	in := graphAdvanceInput(view)
	preparation := graphBeginFacts(domain.GraphAdmission{Graph: view.Graph}, view.Nodes[0], root, proofs, owner)
	preparation.Control.Generation = view.DispatchAuthority.Generation
	preparation.Control.GraphControlRevision, preparation.Control.BudgetAuthorityEpoch, preparation.Control.WindowNumber = view.Graph.ControlRevision, view.Graph.BudgetAuthorityEpoch, view.Budget.Budget.CurrentWindow.Number
	result := s.AdvanceGraph(ctx, owner, view.Graph.ID, in, domain.GraphAdvanceFacts{Preparations: []domain.GraphInputPreparationFacts{preparation}})
	if result.Status != 200 {
		t.Fatal("ordinary first-input preparation incorrectly requires a dependency rebind", result)
	}
	data, err := decode[domain.GraphAdvanceResult](encode(result.Envelope.Data))
	if err != nil || data.DispatchAuthority == nil || len(data.PreparationAttemptIDs) != 1 || len(data.ReboundNodes) != 0 || len(data.Dispatches) != 0 {
		t.Fatal("ordinary preparation claimed a new version or native execution", data, err)
	}
	op, err := s.GraphInputOperationForAttempt(ctx, owner, data.PreparationAttemptIDs[0])
	if err != nil || op.Kind != "node_input" || op.NodeAuthority == nil || op.NodeAuthority.TaskVersion != "2" || op.Control.Generation != data.DispatchAuthority.Generation || op.InputRecord.Trigger.Kind != "explicit" || op.InputRecord.Trigger.ID != in.RequestID || op.RootBinding == nil || !reflect.DeepEqual(*op.RootBinding, root) || op.Attempt.ExecutorKind != "local_graph" || op.Attempt.LaunchIntent {
		t.Fatal("ordinary preparation did not retain the exact current explicit authority", op, err)
	}
	after, err := s.Graph(ctx, view.Graph.ID)
	if err != nil || after.Nodes[0].TaskVersion != "2" || after.Nodes[0].ControlRevision != view.Nodes[0].ControlRevision || !reflect.DeepEqual(after.Budget, view.Budget) || !reflect.DeepEqual(after.Nodes[1], view.Nodes[1]) || after.Budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("ordinary input capture changed version/control/budget or an unrelated node", after, err)
	}
	var afterVersion string
	if err = s.db.QueryRow("SELECT data FROM task_versions WHERE task_id=? AND version=2", task.Task.ID).Scan(&afterVersion); err != nil || beforeVersion != afterVersion {
		t.Fatal("ordinary preparation rewrote captured guidance choices", err)
	}
	if current, err := s.Workflow(ctx, task.Task.ID, domain.WorkflowSelector{}); err != nil || current.Workflow != nil {
		t.Fatal("ordinary preparation finalized workflow before retained input completion", current, err)
	}
	expected, err := decode[domain.Envelope](encode(result.Envelope))
	if err != nil {
		t.Fatal(err)
	}
	replay := s.AdvanceGraph(ctx, owner, view.Graph.ID, in, domain.GraphAdvanceFacts{})
	if replay.Status != 200 || !reflect.DeepEqual(expected, replay.Envelope) {
		t.Fatal("ordinary preparation replay refreshed its operation", replay)
	}
}

func TestGraphAdvancePreparationRejectsUncapturedOrUnprovenReceiver(t *testing.T) {
	for _, change := range []string{"uncaptured_node", "wrong_version", "wrong_owner", "missing_root_proof", "wrong_receiver_task", "future_input_id"} {
		t.Run(change, func(t *testing.T) {
			s, graph, root, proofs, owner := graphReadyBasis(t)
			ctx := context.Background()
			view, err := s.Graph(ctx, graph.Graph.ID)
			if err != nil {
				t.Fatal(err)
			}
			in := graphAdvanceInput(view)
			preparation := graphBeginFacts(graph, view.Nodes[0], root, proofs, owner)
			switch change {
			case "uncaptured_node":
				in.NodeAuthorities = in.NodeAuthorities[1:]
			case "wrong_version":
				wrong := "2"
				preparation.Control.TaskVersion = &wrong
			case "wrong_owner":
				preparation.Control.OwnerInstanceID = domain.NewID()
			case "missing_root_proof":
				filtered := []domain.VerifiedResponse{}
				for _, proof := range preparation.VerifiedArtifacts {
					if proof.ArtifactID != root.Manifest.ArtifactID {
						filtered = append(filtered, proof)
					}
				}
				preparation.VerifiedArtifacts = filtered
			case "wrong_receiver_task":
				preparation.Control.TaskID = &view.Nodes[1].TaskID
			case "future_input_id":
				invented := domain.NewID()
				preparation.Control.InputID = &invented
			}
			var count int
			if err = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&count); err != nil {
				t.Fatal(err)
			}
			result := s.AdvanceGraph(ctx, owner, view.Graph.ID, in, domain.GraphAdvanceFacts{Preparations: []domain.GraphInputPreparationFacts{preparation}})
			if result.Envelope.Error == nil || result.Envelope.Receipt != nil {
				t.Fatal("uncaptured or unproven preparation committed", change, result)
			}
			var after int
			if err = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&after); err != nil || after != count {
				t.Fatal("refused preparation allocated an operation", count, after, err)
			}
			current, err := s.Graph(ctx, view.Graph.ID)
			if err != nil || !reflect.DeepEqual(current, view) {
				t.Fatal("refused preparation refreshed the registration/current graph", current, err)
			}
		})
	}
}
