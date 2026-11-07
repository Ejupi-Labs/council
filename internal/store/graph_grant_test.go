package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphGrantInput(graph domain.GraphSummary) domain.GraphBudgetAmendmentInput {
	return domain.GraphBudgetAmendmentInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: graph.ControlRevision, ExpectedBudgetAuthorityEpoch: graph.BudgetAuthorityEpoch, ExpectedWindowNumber: "1", AdditionalAdmissions: "2"}
}

func TestGraphGrantUsesRootEpochAndLedgerWithoutDispatchOrChildAuthority(t *testing.T) {
	s, graph, _, _, owner := graphReadyBasis(t)
	ctx := context.Background()
	before, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil || before.DispatchAuthority == nil {
		t.Fatal(before, e)
	}
	var attemptsBefore, versionsBefore int
	if e = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&attemptsBefore); e != nil {
		t.Fatal(e)
	}
	if e = s.db.QueryRow("SELECT count(*) FROM task_versions").Scan(&versionsBefore); e != nil {
		t.Fatal(e)
	}
	in := graphGrantInput(before.Graph)
	r := s.AmendGraphBudget(ctx, owner, graph.Graph.ID, in)
	if r.Status != 200 {
		t.Fatal("explicit root-owned graph grant unavailable", r)
	}
	after, e := s.Graph(ctx, graph.Graph.ID)
	want, invalid := domain.AddRoutingCounts(before.Budget.Budget.GrantedAdmissions, in.AdditionalAdmissions)
	if e != nil || invalid != nil || after.Graph.BudgetAuthorityEpoch != "2" || after.Graph.ControlRevision != before.Graph.ControlRevision || after.Budget.Budget.TaskID != graph.Graph.RootTaskID || after.Budget.Budget.GrantedAdmissions != want || after.Budget.Budget.ConsumedAdmissions != before.Budget.Budget.ConsumedAdmissions || after.Budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("grant changed wrong authority or opened execution", before, after, e, invalid)
	}
	if after.DispatchAuthority == nil || after.DispatchAuthority.Generation != before.DispatchAuthority.Generation || after.DispatchAuthority.BudgetAuthorityEpoch != before.DispatchAuthority.BudgetAuthorityEpoch || after.DispatchAuthority.State != "suspended" {
		t.Fatal("grant refreshed automatic dispatch registration", after.DispatchAuthority)
	}
	var attemptsAfter, versionsAfter int
	if e = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&attemptsAfter); e != nil {
		t.Fatal(e)
	}
	if e = s.db.QueryRow("SELECT count(*) FROM task_versions").Scan(&versionsAfter); e != nil || attemptsAfter != attemptsBefore || versionsAfter != versionsBefore {
		t.Fatal("grant created execution or versions", attemptsBefore, attemptsAfter, versionsBefore, versionsAfter, e)
	}
	for i, node := range after.Nodes {
		if node.TaskVersion != before.Nodes[i].TaskVersion || node.TaskRevision != before.Nodes[i].TaskRevision {
			t.Fatal("root grant rewrote child task controls", before.Nodes[i], node)
		}
		budget, e := s.TaskBudget(ctx, node.TaskID)
		if e != nil || budget.Budget != nil || budget.GraphBudgetRef == nil || budget.GraphBudgetRef.BudgetOwnerTaskID != graph.Graph.RootTaskID {
			t.Fatal("root grant invented a child balance", budget, e)
		}
	}
	ledger, e := s.GraphBudgetLedger(ctx, graph.Graph.ID, 100, "")
	if e != nil {
		t.Fatal(e)
	}
	grants := 0
	for _, item := range ledger.Items {
		if item.RequestID == in.RequestID {
			grants++
			if item.TaskID != graph.Graph.RootTaskID || item.Kind != "grant" || item.AttemptID != nil {
				t.Fatal("grant ledger claims child/native work", item)
			}
		}
	}
	if grants != 1 {
		t.Fatal("grant lacks exact root ledger entry", ledger)
	}
	if replay := s.AmendGraphBudget(ctx, owner, graph.Graph.ID, in); replay.Status != 200 || replay.Envelope.Receipt == nil || replay.Envelope.Receipt.RequestID != in.RequestID {
		t.Fatal("exact grant receipt failed replay after epoch change", replay)
	}
	unchanged, e := s.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || !reflect.DeepEqual(unchanged, after.Budget) {
		t.Fatal("grant replay added units twice", unchanged, after.Budget, e)
	}
	stale := in
	stale.RequestID = domain.NewID()
	if refused := s.AmendGraphBudget(ctx, owner, graph.Graph.ID, stale); refused.Envelope.Error == nil {
		t.Fatal("stale captured authority epoch accepted another grant", refused)
	}
	// Stop keeps its captured control target despite an unrelated budget epoch.
	stop := s.StopGraph(ctx, owner, graph.Graph.ID, domain.GraphStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: before.Graph.ControlRevision})
	if stop.Status != 202 {
		t.Fatal("grant invalidated a captured cancellation target", stop)
	}
}

func TestGraphGrantCommitFaultsDoNotPartlyAmendOrReplayTwice(t *testing.T) {
	for _, stage := range []string{"before", "after"} {
		t.Run(stage, func(t *testing.T) {
			s, graph, _, _, owner := graphReadyBasis(t)
			ctx := context.Background()
			before, e := s.Graph(ctx, graph.Graph.ID)
			if e != nil {
				t.Fatal(e)
			}
			in := graphGrantInput(before.Graph)
			fault := func() error { return errors.New("controlled graph grant persistence fault") }
			if stage == "before" {
				s.BeforeCommit = fault
			} else {
				s.AfterCommit = fault
			}
			result := s.AmendGraphBudget(ctx, owner, graph.Graph.ID, in)
			s.BeforeCommit, s.AfterCommit = nil, nil
			if result.Envelope.Error == nil {
				t.Fatal("persistence fault reported successful grant", result)
			}
			after, e := s.Graph(ctx, graph.Graph.ID)
			if e != nil {
				t.Fatal(e)
			}
			if stage == "before" {
				if after.Graph.BudgetAuthorityEpoch != before.Graph.BudgetAuthorityEpoch || !reflect.DeepEqual(after.Budget, before.Budget) {
					t.Fatal("before-commit grant partly changed authority", before, after)
				}
				return
			}
			if result.Envelope.Error.Code != "write_uncertain" || after.Graph.BudgetAuthorityEpoch != "2" {
				t.Fatal("lost acknowledgement erased committed graph grant", result, after)
			}
			if replay := s.AmendGraphBudget(ctx, owner, graph.Graph.ID, in); replay.Status != 200 {
				t.Fatal(replay)
			}
			current, e := s.Graph(ctx, graph.Graph.ID)
			if e != nil || !reflect.DeepEqual(current.Budget, after.Budget) || current.Graph.BudgetAuthorityEpoch != after.Graph.BudgetAuthorityEpoch {
				t.Fatal("uncertain grant replay added authority again", current, after, e)
			}
		})
	}
}

func TestGraphGrantReplacementWindowCapturesOneBoundedPreparationDeadline(t *testing.T) {
	s, graph, _, _, owner := graphReadyBasis(t)
	ctx := context.Background()
	in := graphGrantInput(graph.Graph)
	seconds := 30
	in.NewWindowSeconds = &seconds
	started := time.Now().UTC()
	result := s.AmendGraphBudget(ctx, owner, graph.Graph.ID, in)
	finished := time.Now().UTC()
	if result.Status != 200 {
		t.Fatal(result)
	}
	header, e := s.graphHeader(ctx, s.db, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	deadline, invalid := domain.ParseRoutingTime(header.PreparationDeadlineAt)
	// Stored timestamps have millisecond precision.
	if invalid != nil || deadline.Before(started.Add(30*time.Second-time.Millisecond)) || deadline.After(finished.Add(30*time.Second)) {
		t.Fatal("explicit replacement window lacks its fixed bounded preparation clock", header.PreparationDeadlineAt, started, finished, invalid)
	}
	budget, e := s.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || budget.Budget.CurrentWindow.Number != "2" || budget.Budget.CurrentWindow.WindowSeconds != seconds || budget.Budget.CurrentWindow.FirstAdmissionAt != nil || budget.Budget.CurrentWindow.DeadlineAt != nil || budget.Budget.ConsumedAdmissions != "0" {
		t.Fatal("grant opened the native common clock or spent a model unit", budget, e)
	}
	if replay := s.AmendGraphBudget(ctx, owner, graph.Graph.ID, in); replay.Status != 200 {
		t.Fatal(replay)
	}
	after, e := s.graphHeader(ctx, s.db, graph.Graph.ID)
	if e != nil || after.PreparationDeadlineAt != header.PreparationDeadlineAt || after.Summary.BudgetAuthorityEpoch != header.Summary.BudgetAuthorityEpoch {
		t.Fatal("receipt replay renewed preparation authority", header, after, e)
	}
}

func TestGraphGrantNewWindowRequiresSettledRootAndNodeOwnership(t *testing.T) {
	for _, kind := range []string{"queued_root", "claimed_node", "fenced"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			var s *Store
			var graphID, owner string
			if kind == "queued_root" {
				var graph domain.GraphAdmission
				s, graph, _, owner = graphLocalFixture(t)
				graphID = graph.Graph.ID
			} else if kind == "claimed_node" {
				var dispatch domain.GraphDispatchView
				s, dispatch, _, owner = graphMaterializationFixture(t)
				graphID = dispatch.Dispatch.GraphID
			} else {
				var graph domain.GraphAdmission
				s, graph, _, _, owner = graphReadyBasis(t)
				graphID = graph.Graph.ID
				if e := s.Fence(ctx, domain.ExecutionIncident{ID: domain.NewID(), Code: "controlled_graph_uncertainty", Message: "Metadata-only retained fence fixture.", CreatedAt: domain.Now()}); e != nil {
					t.Fatal(e)
				}
			}
			before, e := s.Graph(ctx, graphID)
			if e != nil {
				t.Fatal(e)
			}
			in := graphGrantInput(before.Graph)
			seconds := 30
			in.NewWindowSeconds = &seconds
			if refused := s.AmendGraphBudget(ctx, owner, graphID, in); refused.Envelope.Error == nil || refused.Envelope.Receipt != nil {
				t.Fatal("unsettled or uncertain ownership opened another common window", refused)
			}
			after, e := s.Graph(ctx, graphID)
			if e != nil || after.Graph.BudgetAuthorityEpoch != before.Graph.BudgetAuthorityEpoch || !reflect.DeepEqual(after.Budget, before.Budget) {
				t.Fatal("refused replacement window partly changed authority", before, after, e)
			}
			if kind == "claimed_node" {
				// An explicit units-only grant records future authority without
				// asserting that the currently owned native work has settled.
				in.RequestID, in.NewWindowSeconds = domain.NewID(), nil
				if result := s.AmendGraphBudget(ctx, owner, graphID, in); result.Status != 200 {
					t.Fatal("known current ownership prevented an explicit units-only grant", result)
				}
				claims, e := s.GraphClaims(ctx)
				if e != nil || len(claims) != 1 {
					t.Fatal("units-only grant released existing ownership", claims, e)
				}
			}
		})
	}
}
