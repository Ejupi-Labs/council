package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Registered artifact/controller facts exercise transactional Store projection.
// They do not establish that an OS process or a provider model ran.
func TestGraphLiveClaimProjectsCurrentNativePreparation(t *testing.T) {
	s, graph, root, proofs, owner := graphReadyBasis(t)
	ctx := context.Background()
	facts, trigger := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[0])
	dispatch, err := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, facts)
	if err != nil || dispatch.Dispatch.ID == "" {
		t.Fatal(dispatch, err)
	}
	claimed, err := s.ClaimAttempt(ctx, dispatch.Dispatch.AttemptID, owner)
	if err != nil || claimed.Phase != "preparing" {
		t.Fatal("registered native claim did not enter preparation", claimed, err)
	}
	view, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil || view.Nodes[0].State != "preparing" || view.Graph.Counts.Preparing != "1" || view.Graph.Counts.Queued != "0" || view.Nodes[0].LatestAttemptID == nil || *view.Nodes[0].LatestAttemptID != claimed.ID {
		t.Fatal("current native preparation was left queued in the graph", view, err)
	}
}

func TestGraphLiveStartProjectsCurrentNativeRunning(t *testing.T) {
	s, dispatch, materialization, owner := graphMaterializationFixture(t)
	ctx := context.Background()
	if err := s.RecordGraphMaterialization(ctx, owner, dispatch.Dispatch.AttemptID, materialization); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordLaunchIntent(ctx, dispatch.Dispatch.AttemptID); err != nil {
		t.Fatal(err)
	}
	before, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	at := domain.Now()
	started, err := s.RecordAttempt(ctx, dispatch.Dispatch.AttemptID, domain.AttemptUpdate{Kind: "worker_started", Phase: "running", StartedAt: &at})
	if err != nil || started.Phase != "running" || started.StartedAt == nil || *started.StartedAt != at {
		t.Fatal("registered Start observation did not enter running", started, err)
	}
	view, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
	if err != nil || view.Nodes[0].State != "running" || view.Graph.Counts.Running != "1" || view.Graph.Counts.Preparing != "0" || view.Graph.Counts.Queued != "0" || view.Graph.State != "running" {
		t.Fatal("current native Start was left queued in the graph", view, err)
	}
	if view.Graph.ControlRevision != before.Graph.ControlRevision || view.Graph.BudgetAuthorityEpoch != before.Graph.BudgetAuthorityEpoch || !reflect.DeepEqual(view.Budget, before.Budget) || !reflect.DeepEqual(view.DispatchAuthority, before.DispatchAuthority) {
		t.Fatal("a phase observation changed execution authority or allowance", before, view)
	}
	if view.Graph.StateRevision == before.Graph.StateRevision || view.Nodes[0].StateRevision == before.Nodes[0].StateRevision || view.Nodes[0].AcceptedResult != nil {
		t.Fatal("Start failed to invalidate cached progress or fabricated acceptance", before, view)
	}
}

func TestGraphLiveClaimProjectsCurrentLocalCheckPreparation(t *testing.T) {
	s, dispatch, _, _ := graphLocalCheckClaimFixture(t)
	ctx := context.Background()
	attempt, err := s.Attempt(ctx, dispatch.Dispatch.AttemptID)
	if err != nil || attempt.ExecutorKind != "local_check" || attempt.Phase != "preparing" {
		t.Fatal("fixture did not claim an actual registered local check", attempt, err)
	}
	view, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
	if err != nil || view.Nodes[0].LatestAttemptID == nil || *view.Nodes[0].LatestAttemptID != attempt.ID || view.Nodes[0].State != "preparing" || view.Graph.Counts.Preparing != "1" || view.Graph.Counts.Queued != "0" || view.Budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("current local check phase was hidden or spent a native unit", view, err)
	}
}

func TestGraphLivePhaseObservationPreservesAcceptedNodeStop(t *testing.T) {
	s, dispatch, _, owner := graphMaterializationFixture(t)
	ctx := context.Background()
	view, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	node := view.Nodes[0]
	stopped := s.StopGraphNode(ctx, owner, view.Graph.ID, node.Key, domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: view.Graph.ControlRevision, NodeID: node.ID, ExpectedNodeControlRevision: node.ControlRevision})
	if stopped.Status != 202 {
		t.Fatal(stopped)
	}
	before, err := s.Graph(ctx, view.Graph.ID)
	if err != nil || before.Nodes[0].Stop == nil {
		t.Fatal("node Stop was not durably accepted", before, err)
	}
	if _, err = s.RecordAttempt(ctx, dispatch.Dispatch.AttemptID, domain.AttemptUpdate{Kind: "controlled_cleanup_pending", Phase: "cancelling"}); err != nil {
		t.Fatal(err)
	}
	after, err := s.Graph(ctx, view.Graph.ID)
	if err != nil || !reflect.DeepEqual(after.Nodes[0].Stop, before.Nodes[0].Stop) || after.Nodes[0].State != before.Nodes[0].State || after.Nodes[0].NextOwner != before.Nodes[0].NextOwner || after.Nodes[0].NextAction != before.Nodes[0].NextAction || after.Nodes[0].AcceptedResult != nil || after.Graph.ControlRevision != before.Graph.ControlRevision || after.Nodes[0].ControlRevision != before.Nodes[0].ControlRevision {
		t.Fatal("a late phase observation reopened accepted Stop scope", before, after, err)
	}
}

func TestGraphLiveStartProjectionRollsBackWithAttemptObservation(t *testing.T) {
	s, dispatch, materialization, owner := graphMaterializationFixture(t)
	ctx := context.Background()
	if err := s.RecordGraphMaterialization(ctx, owner, dispatch.Dispatch.AttemptID, materialization); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordLaunchIntent(ctx, dispatch.Dispatch.AttemptID); err != nil {
		t.Fatal(err)
	}
	before, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	var eventsBefore int
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM attempt_events WHERE attempt_id=?", dispatch.Dispatch.AttemptID).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	s.BeforeCommit = func() error { return errors.New("controlled live phase commit refusal") }
	at := domain.Now()
	_, err = s.RecordAttempt(ctx, dispatch.Dispatch.AttemptID, domain.AttemptUpdate{Kind: "worker_started", Phase: "running", StartedAt: &at})
	s.BeforeCommit = nil
	if err == nil || domain.SafeError(err).Code != "storage_failed" {
		t.Fatal("fixture did not fail before the phase transaction committed", err)
	}
	after, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed Start observation partly committed graph progress", before, after, err)
	}
	retained, err := s.Attempt(ctx, dispatch.Dispatch.AttemptID)
	if err != nil || retained.Phase != "preparing" || retained.StartedAt != nil {
		t.Fatal("failed Start observation partly committed attempt progress", retained, err)
	}
	var eventsAfter int
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM attempt_events WHERE attempt_id=?", dispatch.Dispatch.AttemptID).Scan(&eventsAfter); err != nil || eventsAfter != eventsBefore {
		t.Fatal("failed phase transaction left an event behind", eventsBefore, eventsAfter, err)
	}
}
