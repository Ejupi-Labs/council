package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphAdvanceAdmissionFixture(t *testing.T, count int) (*Store, domain.GraphDetail, []domain.GraphNodeAdmissionFacts, string) {
	t.Helper()
	s, graph, root, proofs, owner := graphReadyBasis(t, func(in *domain.GraphStartInput) {
		if count == 1 {
			in.Nodes = in.Nodes[:1]
		}
		if count == 3 {
			in.Nodes = append(in.Nodes, domain.GraphNodeInput{Key: "c", OriginalText: "Exact independent C"})
		}
	})
	facts := []domain.GraphNodeAdmissionFacts{}
	for _, node := range graph.Nodes {
		admission, _ := graphReadyNodeFacts(t, s, graph, root, proofs, owner, node)
		facts = append(facts, admission)
	}
	view, err := s.Graph(context.Background(), graph.Graph.ID)
	if err != nil || view.Budget.Budget.ConsumedAdmissions != "0" || view.Budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("ready-input fixture created native work", view, err)
	}
	return s, view, facts, owner
}

// Actual Store admissions and declared account/context reservations, backed by
// controlled artifact metadata. No native child or physical-byte claim.
func TestGraphAdvanceInitialAdmissionsReserveDistinctContextsAndOneBudget(t *testing.T) {
	s, before, facts, owner := graphAdvanceAdmissionFixture(t, 2)
	ctx := context.Background()
	in := graphAdvanceInput(before)
	result := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{Admissions: facts})
	if result.Status != 200 {
		t.Fatal("explicit initial admissions unavailable", result)
	}
	data, err := decode[domain.GraphAdvanceResult](encode(result.Envelope.Data))
	if err != nil || len(data.Dispatches) != 2 || len(data.PreparationAttemptIDs) != 0 || len(data.ReboundNodes) != 0 {
		t.Fatal("initial Advance did not report exactly its native reservations", data, err)
	}
	claims, err := s.GraphClaims(ctx)
	if err != nil || len(claims) != 2 || claims[0].ContextID == nil || claims[1].ContextID == nil || *claims[0].ContextID == *claims[1].ContextID {
		t.Fatal("initial batch did not atomically reserve distinct managed contexts", claims, err)
	}
	for _, view := range data.Dispatches {
		d := view.Dispatch
		if d.Role != "author" || d.ExecutorKind != "native" || d.NodeAdmissionID != d.ID || d.FreshTriggerID != in.RequestID || d.NativeConsumptionKey == nil || *d.NativeConsumptionKey != d.AttemptID || d.OwnerInstanceID != owner {
			t.Fatal("initial dispatch lost its actual fresh execution identity", d)
		}
	}
	after, err := s.Graph(ctx, before.Graph.ID)
	if err != nil || after.Budget.Budget.ConsumedAdmissions != "2" || after.Budget.Budget.AutomaticAdmissions != "0" || after.Budget.Budget.GrantedAdmissions != before.Budget.Budget.GrantedAdmissions || after.Budget.Budget.CurrentWindow.FirstAdmissionAt == nil {
		t.Fatal("initial batch did not spend exactly two root-owned manual units", after, err)
	}
	expected, err := decode[domain.Envelope](encode(result.Envelope))
	if err != nil {
		t.Fatal(err)
	}
	replay := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{})
	if replay.Status != 200 || !reflect.DeepEqual(replay.Envelope, expected) {
		t.Fatal("initial batch replay created fresh intent", replay)
	}
	final, err := s.GraphBudget(ctx, before.Graph.ID)
	if err != nil || final.Budget.ConsumedAdmissions != "2" {
		t.Fatal("replay duplicated model debit", final, err)
	}
}

func TestGraphAdvanceInitialBatchRetainsCapacityWaitWithoutThirdDebit(t *testing.T) {
	s, before, facts, owner := graphAdvanceAdmissionFixture(t, 3)
	in := graphAdvanceInput(before)
	result := s.AdvanceGraph(context.Background(), owner, before.Graph.ID, in, domain.GraphAdvanceFacts{Admissions: facts})
	if result.Status != 200 {
		t.Fatal("known capacity wait aborted the acknowledged initial batch", result)
	}
	data, err := decode[domain.GraphAdvanceResult](encode(result.Envelope.Data))
	if err != nil || len(data.Dispatches) != 2 {
		t.Fatal(data, err)
	}
	after, err := s.Graph(context.Background(), before.Graph.ID)
	if err != nil || after.Nodes[2].NodeAdmissionID != nil || after.Nodes[2].State != "ready" || after.Nodes[2].NextOwner != "council" || after.Budget.Budget.ConsumedAdmissions != "2" || after.DispatchAuthority.State == "suspended" {
		t.Fatal("capacity waiting lost live scope or invented a third native admission", after, err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM workflows WHERE task_id=?", before.Nodes[2].TaskID).Scan(&count); err != nil || count != 0 {
		t.Fatal("capacity rollback left false workflow authority", count, err)
	}
}

func TestGraphAdvanceInitialFreshIntentCanRecheckKnownPreparationBlock(t *testing.T) {
	s, before, facts, owner := graphAdvanceAdmissionFixture(t, 1)
	ctx := context.Background()
	first := graphAdvanceInput(before)
	prepared := facts[0].Capture
	facts[0].Capture = nil
	facts[0].PreparationFailure = domain.Fail("source_capture_blocked", "The controlled original access was temporarily unavailable.")
	blocked := s.AdvanceGraph(ctx, owner, before.Graph.ID, first, domain.GraphAdvanceFacts{Admissions: facts})
	if blocked.Status != 200 {
		t.Fatal("known initial blocker did not remain inspectable", blocked)
	}
	view, err := s.Graph(ctx, before.Graph.ID)
	if err != nil || view.Nodes[0].State != "blocked" || view.Nodes[0].NextOwner != "user" || view.Budget.Budget.ConsumedAdmissions != "0" {
		t.Fatal(view, err)
	}
	facts[0].Capture, facts[0].PreparationFailure = prepared, nil
	// Changed physical availability alone is not a new dispatch event.
	old := domain.GraphFreshTrigger{Kind: "explicit", ID: first.RequestID, OwnerGeneration: view.DispatchAuthority.Generation}
	oldFacts := facts[0]
	oldFacts.Control.Generation = view.DispatchAuthority.Generation
	if replayed, err := s.AdmitGraphNode(ctx, owner, view.Graph.ID, old, oldFacts); err == nil && replayed.Dispatch.ID != "" {
		t.Fatal("consumed old blocker event automatically rearmed work", replayed)
	}
	second := graphAdvanceInput(view)
	resumed := s.AdvanceGraph(ctx, owner, view.Graph.ID, second, domain.GraphAdvanceFacts{Admissions: facts})
	if resumed.Status != 200 {
		t.Fatal("fresh explicit current ready-input scope could not recheck availability", resumed)
	}
	data, err := decode[domain.GraphAdvanceResult](encode(resumed.Envelope.Data))
	if err != nil || len(data.Dispatches) != 1 || data.Budget.Budget.ConsumedAdmissions != "1" || data.Dispatches[0].Dispatch.TaskVersion != before.Nodes[0].TaskVersion {
		t.Fatal("fresh intent did not create exactly one initial execution", data, err)
	}
}

func TestGraphAdvanceInitialBatchRollsBackEarlierReservationOnLaterInvalidProof(t *testing.T) {
	s, before, facts, owner := graphAdvanceAdmissionFixture(t, 2)
	filtered := []domain.VerifiedResponse{}
	for _, proof := range facts[1].VerifiedArtifacts {
		if proof.ArtifactID != facts[1].ConditionsRef.ArtifactID {
			filtered = append(filtered, proof)
		}
	}
	facts[1].VerifiedArtifacts = filtered
	result := s.AdvanceGraph(context.Background(), owner, before.Graph.ID, graphAdvanceInput(before), domain.GraphAdvanceFacts{Admissions: facts})
	if result.Envelope.Error == nil || result.Envelope.Receipt != nil {
		t.Fatal("later invalid proof committed a partial initial batch", result)
	}
	current, err := s.Graph(context.Background(), before.Graph.ID)
	if err != nil || current.Budget.Budget.ConsumedAdmissions != "0" || current.Budget.Budget.CurrentWindow.DeadlineAt != nil || current.DispatchAuthority.Generation != before.DispatchAuthority.Generation {
		t.Fatal("later invalid proof retained a prior debit/window/registration", current, err)
	}
	claims, err := s.GraphClaims(context.Background())
	if err != nil || len(claims) != 0 {
		t.Fatal("failed batch retained an earlier model reservation", claims, err)
	}
	for _, node := range before.Nodes {
		var count int
		if err = s.db.QueryRow("SELECT count(*) FROM workflows WHERE task_id=?", node.TaskID).Scan(&count); err != nil || count != 0 {
			t.Fatal("failed batch retained an earlier workflow authority", node, count, err)
		}
	}
}
