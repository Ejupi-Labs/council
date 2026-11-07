package store

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These fixtures use actual SQLite acceptance, input-completion, Stop and
// admission transactions with registered metadata proofs. They do not claim
// native execution, physical artifact access or filesystem composition.
type graphInitialAncestorStopCase struct {
	store *Store
	owner string
	view  domain.GraphDetail
	b, c  domain.GraphNodeAdmissionFacts
}

func graphInitialAncestorStopFixture(t *testing.T) graphInitialAncestorStopCase {
	t.Helper()
	ctx := context.Background()
	s, operation, completion, owner := graphDependentCompletionFixture(t, "", func(in *domain.GraphStartInput) {
		in.Nodes[0].Pins = &domain.ExecutionPins{HarnessID: "codex"}
		in.Nodes[1].Pins = &domain.ExecutionPins{HarnessID: "claude-code"}
		in.Nodes = append(in.Nodes, domain.GraphNodeInput{Key: "c", OriginalText: "Retain independent C under its captured context.", Pins: &domain.ExecutionPins{HarnessID: "codex"}})
	})
	if err := s.CompleteGraphInput(ctx, owner, operation.Attempt.ID, completion); err != nil {
		t.Fatal("The real dependent input did not complete", err)
	}
	b, _ := graphNodeFactsForInput(t, s, operation, completion)
	view, err := s.Graph(ctx, operation.GraphID)
	if err != nil || len(view.Nodes) != 3 {
		t.Fatal("The accepted predecessor/dependent fixture is incomplete", view, err)
	}
	root, err := s.GraphRoot(ctx, view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	rootProofs := []domain.VerifiedResponse{}
	for _, ref := range []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs} {
		artifact, err := s.Artifact(ctx, ref.ArtifactID)
		if err != nil || artifact.SHA256 == nil {
			t.Fatal("A registered root proof is unavailable", artifact, err)
		}
		rootProofs = append(rootProofs, domain.VerifiedResponse{ArtifactID: artifact.ID, SHA256: *artifact.SHA256, ByteLength: artifact.ByteLength, PhysicalIdentity: artifact.PhysicalIdentity})
	}
	graph := domain.GraphAdmission{Graph: view.Graph, Nodes: view.Nodes, PreparationAttemptID: root.OperationAttemptID}
	c, _ := graphReadyNodeFacts(t, s, graph, root, rootProofs, owner, view.Nodes[2])
	view, err = s.Graph(ctx, view.Graph.ID)
	if err != nil || view.Nodes[0].AcceptedResult == nil || view.Nodes[1].State != "ready" || view.Nodes[2].State != "ready" ||
		view.Nodes[1].NodeAdmissionID != nil || view.Nodes[2].NodeAdmissionID != nil || len(view.ActiveClaims) != 0 ||
		view.Budget.Budget.ConsumedAdmissions != "1" || view.Budget.Budget.RemainingAdmissions != "3" {
		t.Fatal("The fixture lacks accepted A and two unadmitted current inputs with capacity and allowance", view, err)
	}
	if len(b.InputBinding.ContributorVector) != 1 || b.InputBinding.ContributorVector[0].NodeID != view.Nodes[0].ID ||
		b.InputBinding.ContributorVector[0].AcceptanceID != view.Nodes[0].AcceptedResult.AcceptanceID || len(c.InputBinding.ContributorVector) != 0 {
		t.Fatal("The controlled dependency is not exactly A to B with independent C", b.InputBinding, c.InputBinding)
	}
	return graphInitialAncestorStopCase{store: s, owner: owner, view: view, b: b, c: c}
}

// Refresh only the explicit current control tuple. The accepted contribution,
// sealed receiving input, capture and all registered proof identities stay as
// originally completed. This represents fresh post-Stop Engine facts, not the
// stale pre-Stop vector which existing revision checks already reject.
func graphInitialAncestorCurrentFact(t *testing.T, f domain.GraphNodeAdmissionFacts, view domain.GraphDetail) domain.GraphNodeAdmissionFacts {
	t.Helper()
	for _, node := range view.Nodes {
		if node.ID != f.NodeID {
			continue
		}
		if view.DispatchAuthority == nil || node.TaskVersion != f.TaskVersion || node.InputID == nil || *node.InputID != f.InputBinding.ID {
			t.Fatal("The exact current input/version prerequisite is unavailable", node, f)
		}
		f.Control.GraphVersion = view.Graph.GraphVersion
		f.Control.Generation = view.DispatchAuthority.Generation
		f.Control.GraphControlRevision = view.Graph.ControlRevision
		f.Control.BudgetAuthorityEpoch = view.Graph.BudgetAuthorityEpoch
		f.Control.WindowNumber = view.Budget.Budget.CurrentWindow.Number
		f.Control.NodeControlRevision = ptr(node.ControlRevision)
		return f
	}
	t.Fatal("The controlled node is absent from its own graph", f.NodeID)
	return f
}

// Snapshot every logical table, including raw JSON/bytes and SQL row identities.
// No volatile public Attempt observation fields need to be discarded. Close the
// table inventory before the per-table reads so the sole SQL connection is free.
func graphInitialAncestorRows(t *testing.T, s *Store) map[string][]string {
	t.Helper()
	rows, err := s.db.Query("SELECT name FROM sqlite_schema WHERE type='table' AND name NOT GLOB 'sqlite_*' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		names = append(names, name)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	out := map[string][]string{}
	for _, name := range names {
		quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
		data, err := s.db.Query("SELECT quote(rowid),* FROM " + quoted + " ORDER BY rowid")
		if err != nil {
			t.Fatal(name, err)
		}
		columns, err := data.Columns()
		if err != nil {
			data.Close()
			t.Fatal(name, err)
		}
		out[name] = []string{}
		for data.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err = data.Scan(pointers...); err != nil {
				data.Close()
				t.Fatal(name, err)
			}
			out[name] = append(out[name], encode(values))
		}
		err = data.Err()
		closeErr = data.Close()
		if err != nil || closeErr != nil {
			t.Fatal(name, err, closeErr)
		}
	}
	return out
}

func graphInitialAncestorAssertRows(t *testing.T, before, after map[string][]string, message string) {
	t.Helper()
	if reflect.DeepEqual(before, after) {
		return
	}
	changed := []string{}
	for name, rows := range before {
		if !reflect.DeepEqual(rows, after[name]) {
			changed = append(changed, name)
		}
	}
	for name := range after {
		if _, exists := before[name]; !exists {
			changed = append(changed, name)
		}
	}
	slices.Sort(changed)
	t.Error(message, "changed SQL tables:", changed)
}

func TestGraphInitialAdmissionAncestorStopUnstoppedBatchHasValidProofsAndCapacity(t *testing.T) {
	f := graphInitialAncestorStopFixture(t)
	result := f.store.AdvanceGraph(context.Background(), f.owner, f.view.Graph.ID, graphAdvanceInput(f.view), domain.GraphAdvanceFacts{Admissions: []domain.GraphNodeAdmissionFacts{f.c, f.b}})
	if result.Status != 200 || result.Envelope.Error != nil {
		t.Fatal("The no-Stop control did not admit the same two valid inputs", result)
	}
	data, err := decode[domain.GraphAdvanceResult](encode(result.Envelope.Data))
	if err != nil || len(data.Dispatches) != 2 || data.Dispatches[0].Dispatch.NodeID != f.c.NodeID || data.Dispatches[1].Dispatch.NodeID != f.b.NodeID || data.Budget.Budget.ConsumedAdmissions != "3" {
		t.Fatal("The no-Stop control lacks its two exact initial admissions", data, err)
	}
	claims, err := f.store.GraphClaims(context.Background())
	if err != nil || len(claims) != 2 || claims[0].ContextID == nil || claims[1].ContextID == nil || *claims[0].ContextID == *claims[1].ContextID {
		t.Fatal("The no-Stop control did not reserve distinct captured contexts", claims, err)
	}
}

func TestGraphInitialAdmissionAncestorStopRejectsFreshBatchAndRollsBackThenAllowsIndependentReplay(t *testing.T) {
	f := graphInitialAncestorStopFixture(t)
	s, ctx := f.store, context.Background()
	a, b, c := f.view.Nodes[0], f.view.Nodes[1], f.view.Nodes[2]
	stopID := domain.NewID()
	stopped := s.StopGraphNode(ctx, f.owner, f.view.Graph.ID, a.Key, domain.GraphNodeStopInput{RequestID: stopID, ExpectedGraphControlRevision: f.view.Graph.ControlRevision, NodeID: a.ID, ExpectedNodeControlRevision: a.ControlRevision})
	stop, err := decode[domain.GraphStopResult](encode(stopped.Envelope.Data))
	if stopped.Status != 202 || stopped.Envelope.Error != nil || err != nil || !stop.StopAccepted || !slices.Contains(stop.AffectedNodeIDs, b.ID) || slices.Contains(stop.AffectedNodeIDs, c.ID) {
		t.Fatal("Real accepted ancestor Stop did not close only its unstarted dependent scope", stopped, stop, err)
	}
	before, err := s.Graph(ctx, f.view.Graph.ID)
	if err != nil || before.Nodes[0].Stop == nil || before.Nodes[0].Stop.RequestID != stopID || !reflect.DeepEqual(before.Nodes[0].AcceptedResult, a.AcceptedResult) ||
		before.Nodes[1].State != "blocked" || before.Nodes[1].NextAction != "dependency_stopped" || before.Nodes[1].Stop != nil || before.Nodes[1].ControlRevision == b.ControlRevision ||
		before.Nodes[1].InputID == nil || *before.Nodes[1].InputID != f.b.InputBinding.ID || before.Nodes[1].NodeAdmissionID != nil || before.Nodes[2].State != "ready" || len(before.ActiveClaims) != 0 {
		t.Fatal("Stop did not retain the accepted source and sealed dependent input while preserving C", before, err)
	}
	input, err := s.GraphInputRecord(ctx, before.Graph.ID, f.b.InputBinding.ID)
	if err != nil || !input.Public.Summary.Current || input.Public.Summary.State != "ready" || input.Public.InputBinding == nil || !reflect.DeepEqual(*input.Public.InputBinding, f.b.InputBinding) {
		t.Fatal("The counterexample does not retain its exact current sealed input", input, err)
	}
	freshB := graphInitialAncestorCurrentFact(t, f.b, before)
	freshC := graphInitialAncestorCurrentFact(t, f.c, before)
	in := graphAdvanceInput(before)
	rows := graphInitialAncestorRows(t, s)
	// C is deliberately first: refusal of B must roll back the earlier valid
	// reservation, debit, workflow, events and the new graph registration too.
	result := s.AdvanceGraph(ctx, f.owner, before.Graph.ID, in, domain.GraphAdvanceFacts{Admissions: []domain.GraphNodeAdmissionFacts{freshC, freshB}})
	if result.Status != 409 || result.Envelope.Error == nil || result.Envelope.Error.Code != "graph_control_conflict" || result.Envelope.Receipt != nil {
		t.Error("Fresh current authority admitted an initial receiver after its ancestor Stop", result)
	}
	graphInitialAncestorAssertRows(t, rows, graphInitialAncestorRows(t, s), "Refused initial batch did not roll back every logical SQL row")
	if t.Failed() {
		return
	}

	independent := graphAdvanceInput(before)
	allowed := s.AdvanceGraph(ctx, f.owner, before.Graph.ID, independent, domain.GraphAdvanceFacts{Admissions: []domain.GraphNodeAdmissionFacts{freshC}})
	data, err := decode[domain.GraphAdvanceResult](encode(allowed.Envelope.Data))
	if allowed.Status != 200 || allowed.Envelope.Error != nil || err != nil || len(data.Dispatches) != 1 || data.Dispatches[0].Dispatch.NodeID != c.ID ||
		data.Dispatches[0].Dispatch.Role != "author" || data.Dispatches[0].Dispatch.TaskVersion != c.TaskVersion || data.Budget.Budget.ConsumedAdmissions != "2" ||
		data.Budget.Budget.GrantedAdmissions != before.Budget.Budget.GrantedAdmissions || !reflect.DeepEqual(data.Budget.Budget.CurrentWindow, before.Budget.Budget.CurrentWindow) {
		t.Fatal("A stopped ancestor blocked independent C or changed its shared allowance", allowed, data, err)
	}
	after, err := s.Graph(ctx, before.Graph.ID)
	if err != nil || !reflect.DeepEqual(after.Nodes[0], before.Nodes[0]) || !reflect.DeepEqual(after.Nodes[1], before.Nodes[1]) {
		t.Fatal("Independent admission changed stopped/dependent authority or retained acceptance", after, err)
	}
	retained, err := s.GraphInputRecord(ctx, before.Graph.ID, f.b.InputBinding.ID)
	if err != nil || !reflect.DeepEqual(retained, input) {
		t.Fatal("Independent admission changed the dependent's retained input/provenance", retained, err)
	}
	nodes, err := s.graphNodes(ctx, s.db, before.Graph.ID)
	if err != nil || nodes[0].NativeDebits != "1" || nodes[1].NativeDebits != "0" || nodes[2].NativeDebits != "1" {
		t.Fatal("Independent admission did not debit only C exactly once", nodes, err)
	}
	claims, err := s.GraphClaims(ctx)
	if err != nil || len(claims) != 1 || claims[0].AttemptID != data.Dispatches[0].Dispatch.AttemptID || claims[0].ContextID == nil || *claims[0].ContextID != "codex-native" {
		t.Fatal("Independent admission did not reserve only its exact captured context", claims, err)
	}
	expected, err := decode[domain.Envelope](encode(allowed.Envelope))
	if err != nil {
		t.Fatal(err)
	}
	afterRows := graphInitialAncestorRows(t, s)
	replay := s.AdvanceGraph(ctx, f.owner, before.Graph.ID, independent, domain.GraphAdvanceFacts{})
	if replay.Status != 200 || !reflect.DeepEqual(replay.Envelope, expected) {
		t.Error("Exact independent replay did not return its committed receipt", replay)
	}
	graphInitialAncestorAssertRows(t, afterRows, graphInitialAncestorRows(t, s), "Exact replay changed an admission, ledger, claim or retained row")
}
