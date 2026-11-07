package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Two independent SQLite clients contest one real remaining root-budget unit.
// Artifact/materialization proofs are controlled metadata, not filesystem or
// native-process qualification. Engine's overlap tests cover actual workers.
func TestGraphBudgetTwoClientsContestLastUnitWithoutDuplicateDebit(t *testing.T) {
	s, first, output, owner := graphOutputConfiguredFixture(t, true, func(in *domain.GraphStartInput) {
		in.Nodes = append(in.Nodes, domain.GraphNodeInput{Key: "c", OriginalText: "Exact C"})
		in.Routing = &domain.RoutingOptions{
			Budget:       &domain.RoutingBudgetInput{AdmissionLimit: "2"},
			Continuation: &domain.ContinuationInput{MaxAutomaticAdmissions: "0"},
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, e := s.SettleWorkflowStep(ctx, first.Dispatch.AttemptID, output); e != nil {
		t.Fatal(e)
	}
	task, e := s.Task(ctx, first.Dispatch.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	response := output.ResponseBinding
	ack := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: task.Task.Revision, TaskVersion: task.Version.Version, AttemptID: response.ProducerAttemptID, ResponseArtifactID: response.ResponseArtifactID, ResponseSHA256: response.ResponseSHA256, Confirmation: "reviewed_response_and_conditions"}
	if accepted := s.AcceptResponseWithWorkflow(ctx, task.Task.ID, ack, output.VerifiedArtifacts); accepted.Status != 200 {
		t.Fatal("first unit was not settled and acknowledged", accepted)
	}
	view, e := s.Graph(ctx, first.Dispatch.GraphID)
	if e != nil || len(view.Nodes) != 3 || view.Nodes[0].AcceptedResult == nil || view.Graph.MaxParallelism != "2" {
		t.Fatal("contest requires one accepted result and two independent pending nodes", view, e)
	}
	root, e := s.GraphRoot(ctx, view.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	proofs := []domain.VerifiedResponse{}
	for _, ref := range []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs} {
		artifact, e := s.Artifact(ctx, ref.ArtifactID)
		if e != nil {
			t.Fatal(e)
		}
		proofs = append(proofs, domain.VerifiedResponse{ArtifactID: artifact.ID, SHA256: *artifact.SHA256, ByteLength: artifact.ByteLength, PhysicalIdentity: artifact.PhysicalIdentity})
	}
	graph := domain.GraphAdmission{Graph: view.Graph, Nodes: view.Nodes, PreparationAttemptID: root.OperationAttemptID}
	b, triggerB := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[1])
	c, triggerC := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[2])
	before, e := s.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || before.Budget.GrantedAdmissions != "2" || before.Budget.ConsumedAdmissions != "1" || before.Budget.RemainingAdmissions != "1" || before.Budget.CurrentWindow.DeadlineAt == nil {
		t.Fatal("contest did not begin with exactly one remaining shared unit", before, e)
	}
	claims, e := s.GraphClaims(ctx)
	if e != nil || len(claims) != 0 {
		t.Fatal("capacity must not compete with the last-unit boundary", claims, e)
	}
	for _, facts := range []domain.GraphNodeAdmissionFacts{b, c} {
		if facts.Control.BudgetAuthorityEpoch != before.BudgetAuthorityEpoch || facts.Control.WindowNumber != before.Budget.CurrentWindow.Number {
			t.Fatal("contenders did not capture the same current authority", facts.Control, before)
		}
	}

	// Open the same physical database again; sharing s would merely exercise its
	// process-local mutex. Do not RecoverOwner: both clients use the existing owner.
	var databasePath string
	if e = s.db.QueryRowContext(ctx, "SELECT file FROM pragma_database_list WHERE name='main'").Scan(&databasePath); e != nil {
		t.Fatal(e)
	}
	other, e := Open(databasePath)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { other.Close() })
	if other == s || other.db == s.db {
		t.Fatal("contest must use independent Store clients and connection pools")
	}
	type contender struct {
		client  *Store
		facts   domain.GraphNodeAdmissionFacts
		trigger domain.GraphFreshTrigger
	}
	type result struct {
		contender int
		dispatch  domain.GraphDispatchView
		err       error
	}
	contenders := []contender{{s, b, triggerB}, {other, c, triggerC}}
	ready := make(chan struct{}, len(contenders))
	start := make(chan struct{})
	results := make(chan result, len(contenders))
	for i, candidate := range contenders {
		go func(i int, candidate contender) {
			ready <- struct{}{}
			<-start
			dispatch, e := candidate.client.AdmitGraphNode(ctx, owner, graph.Graph.ID, candidate.trigger, candidate.facts)
			results <- result{i, dispatch, e}
		}(i, candidate)
	}
	for range contenders {
		<-ready
	}
	close(start)
	winner, loser := -1, -1
	var winning domain.GraphDispatchView
	for range contenders {
		out := <-results
		if out.err != nil {
			t.Fatalf("client %d failed instead of recording admission or budget refusal: %v", out.contender, out.err)
		}
		if out.dispatch.Dispatch.ID == "" {
			if loser != -1 {
				t.Fatal("both contenders were refused despite one available unit")
			}
			loser = out.contender
		} else {
			if winner != -1 {
				t.Fatal("both clients consumed the same last shared unit")
			}
			winner, winning = out.contender, out.dispatch
		}
	}
	if winner < 0 || loser < 0 {
		t.Fatal("expected exactly one winner and one refusal", winner, loser)
	}
	current, e := other.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	losing := contenders[loser].facts
	var refused *domain.GraphBlockerSummary
	for i := range current.Blockers {
		block := &current.Blockers[i]
		if block.NodeID != nil && *block.NodeID == losing.NodeID {
			refused = block
		}
	}
	if refused == nil || refused.Code != "budget_exhausted" || refused.InputID == nil || *refused.InputID != losing.InputBinding.ID {
		t.Fatal("loser was not refused at the shared allowance boundary", refused, current)
	}
	after, e := other.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || after.Budget.GrantedAdmissions != "2" || after.Budget.ConsumedAdmissions != "2" || after.Budget.RemainingAdmissions != "0" || after.BudgetAuthorityEpoch != before.BudgetAuthorityEpoch || !reflect.DeepEqual(after.Budget.CurrentWindow, before.Budget.CurrentWindow) || after.Budget.Revision == before.Budget.Revision {
		t.Fatal("contest duplicated/refilled authority or reset its common window", after, e)
	}
	for _, node := range after.NodeConsumption {
		want := "0"
		if node.NodeID == first.Dispatch.NodeID || node.NodeID == winning.Dispatch.NodeID {
			want = "1"
		}
		if node.ConsumedAdmissions != want {
			t.Fatal("last-unit accounting was attributed to the wrong node", node, want)
		}
	}
	claims, e = other.GraphClaims(ctx)
	if e != nil || len(claims) != 1 || claims[0].AttemptID != winning.Dispatch.AttemptID {
		t.Fatal("loser reserved a native claim", claims, e)
	}
	var losingAttempts, nativeAttempts int
	if e = other.db.QueryRowContext(ctx, "SELECT count(*) FROM attempts WHERE task_id=? AND json_extract(data,'$.executorKind')='native'", *losing.Control.TaskID).Scan(&losingAttempts); e != nil || losingAttempts != 0 {
		t.Fatal("budget refusal allocated a loser native attempt", losingAttempts, e)
	}
	if e = other.db.QueryRowContext(ctx, "SELECT count(*) FROM graph_dispatches WHERE graph_id=? AND native_consumption_key IS NOT NULL", graph.Graph.ID).Scan(&nativeAttempts); e != nil || nativeAttempts != 2 {
		t.Fatal("contest allocated extra native dispatches", nativeAttempts, e)
	}
	ledger, e := other.GraphBudgetLedger(ctx, graph.Graph.ID, 100, "")
	if e != nil {
		t.Fatal(e)
	}
	debits := map[string]int{}
	for _, entry := range ledger.Items {
		if entry.TaskID != graph.Graph.RootTaskID {
			t.Fatal("graph ledger acquired a child budget owner", entry)
		}
		if entry.Kind == "admission" {
			if entry.AttemptID == nil || entry.ConsumedDelta != "1" || entry.GrantedDelta != "0" {
				t.Fatal("invalid native debit", entry)
			}
			debits[*entry.AttemptID]++
		}
	}
	if len(debits) != 2 || debits[first.Dispatch.AttemptID] != 1 || debits[winning.Dispatch.AttemptID] != 1 {
		t.Fatal("native admissions were not debited exactly once", debits)
	}
	// Replaying either consumed trigger cannot spend, reserve, or revive a loser.
	for _, candidate := range contenders {
		if duplicate, e := candidate.client.AdmitGraphNode(ctx, owner, graph.Graph.ID, candidate.trigger, candidate.facts); e != nil || duplicate.Dispatch.ID != "" {
			t.Fatal("consumed trigger rearmed native work", duplicate, e)
		}
	}
	replayed, e := s.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || !reflect.DeepEqual(after, replayed) {
		t.Fatal("replayed admission changed the canonical ledger", replayed, e)
	}
}
