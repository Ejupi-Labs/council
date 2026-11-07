package store

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// This controlled persisted projection/terminal fixture is not a public native
// admission or evidence that a provider executed. It writes labelled historical
// records directly and never supplies an unregistered attempt to settlement.
// Only later control immutability is qualified.
func graphFailedProjection(t *testing.T, s *Store, graphID, nodeID, owner string) string {
	t.Helper()
	ctx := context.Background()
	terminal := ""
	e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		nodes, e := s.graphNodes(ctx, tx, graphID)
		if e != nil {
			return e
		}
		for i := range nodes {
			n := &nodes[i]
			if n.Summary.ID != nodeID {
				continue
			}
			task, e := s.task(ctx, tx, n.Summary.TaskID)
			if e != nil {
				return e
			}
			version, e := s.version(ctx, tx, task.ID, task.CurrentVersion)
			if e != nil {
				return e
			}
			a := newAttempt(task, version, domain.NewID(), owner, "fake", "response")
			a.Role = "author"
			a.ExecutorKind = "native"
			if e = s.insertAttempt(ctx, tx, a); e != nil {
				return e
			}
			task.LatestAttemptID = &a.ID
			if e = saveTask(ctx, tx, task); e != nil {
				return e
			}
			now := domain.Now()
			failure := domain.PrimaryFailure{Code: "controlled_failure", Message: "Controlled prior failure remains primary."}
			settlement := domain.Settlement{Outcome: "failed", Failure: &failure, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched", FinishedAt: &now}, EvidenceStatus: "incomplete", EffectsKnown: true, NoProcessCreated: true}
			sequence, err := s.appendEvent(ctx, tx, a.ID, "primary_failure", failureMetadata(&failure))
			if err != nil {
				return err
			}
			observed := failure
			observed.EventSequence, observed.ObservedAt = sequence, now
			a.PrimaryFailure, a.Outcome, a.SettledAt = &observed, ptr("failed"), &now
			a.Phase, a.EvidenceStatus, a.Cleanup = "settled", settlement.EvidenceStatus, settlement.Cleanup
			if e = saveAttempt(ctx, tx, a); e != nil {
				return e
			}
			if _, e = s.appendEvent(ctx, tx, a.ID, "settled", nil); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, "INSERT INTO attempt_terminals VALUES(?,?)", a.ID, encode(struct {
				Attempt    domain.Attempt    `json:"attempt"`
				Settlement domain.Settlement `json:"settlement"`
			}{a, settlement})); e != nil {
				return e
			}
			n.Summary.State = "failed"
			n.Summary.LatestAttemptID = &a.ID
			n.Summary.NextOwner = "user"
			n.Summary.NextAction = "inspect_retained_failure"
			if e = saveGraphNode(ctx, tx, *n); e != nil {
				return e
			}
			return tx.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", a.ID).Scan(&terminal)
		}
		return notFound()
	})
	if e != nil {
		t.Fatal(e)
	}
	return terminal
}
func TestGraphLateGroupStopWithoutRemainingScopeIsNoop(t *testing.T) {
	s, graph, _, _, owner := graphReadyBasis(t)
	ctx := context.Background()
	for _, n := range graph.Nodes {
		graphFailedProjection(t, s, graph.Graph.ID, n.ID, owner)
	}
	before, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	r := s.StopGraph(ctx, owner, graph.Graph.ID, domain.GraphStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: "1"})
	if r.Status != 200 || r.Envelope.Data.(domain.GraphStopResult).StopAccepted || r.Envelope.Data.(domain.GraphStopResult).Reason != "already_settled" {
		t.Fatal("fresh group Stop invented work after all scope settled", r)
	}
	after, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil || !reflect.DeepEqual(before.Nodes, after.Nodes) || before.Graph.ControlRevision != after.Graph.ControlRevision || before.Graph.StateRevision != after.Graph.StateRevision {
		t.Fatal("group no-op changed prior outcomes or controls", before, after, e)
	}
}
func TestGraphStopPreservesFailedNodeAndImmutableTerminal(t *testing.T) {
	s, graph, _, _, owner := graphReadyBasis(t)
	ctx := context.Background()
	prior := graphFailedProjection(t, s, graph.Graph.ID, graph.Nodes[0].ID, owner)
	r := s.StopGraph(ctx, owner, graph.Graph.ID, domain.GraphStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: "1"})
	if r.Status != 202 {
		t.Fatal(r)
	}
	view, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil || view.Nodes[0].State != "failed" || view.Nodes[1].State != "cancelled" {
		t.Fatal("Stop erased prior failure instead of only stopping remaining work", view, e)
	}
	var after string
	if e = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", *view.Nodes[0].LatestAttemptID).Scan(&after); e != nil || after != prior {
		t.Fatal("Stop rewrote a prior terminal", e)
	}
}
func TestGraphNodeStopPreservesFailedDescendant(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in, f, owner := graphTestStart(t, s)
	in.Nodes = append(in.Nodes, domain.GraphNodeInput{Key: "c", OriginalText: "Controlled failed descendant"})
	edges := []domain.GraphDependencyInput{{From: "a", To: "c"}}
	in.Dependencies = &edges
	created := s.CreateGraph(ctx, owner, in, f)
	if created.Status != 201 {
		t.Fatal(created)
	}
	graph := created.Envelope.Data.(domain.GraphAdmission)
	graphFailedProjection(t, s, graph.Graph.ID, graph.Nodes[2].ID, owner)
	r := s.StopGraphNode(ctx, owner, graph.Graph.ID, "a", domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: "1", NodeID: graph.Nodes[0].ID, ExpectedNodeControlRevision: "1"})
	if r.Status != 202 {
		t.Fatal(r)
	}
	view, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil || view.Nodes[0].State != "cancelled" || view.Nodes[1].State != "waiting" || view.Nodes[2].State != "failed" {
		t.Fatal("ancestor Stop reclassified a retained failed descendant", view, e)
	}
}
func TestGraphNodeLateStopWithoutRemainingScopeIsNoop(t *testing.T) {
	s, graph, _, _, owner := graphReadyBasis(t)
	ctx := context.Background()
	if e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		nodes, e := s.graphNodes(ctx, tx, graph.Graph.ID)
		if e != nil {
			return e
		}
		nodes[0].Summary.State = "completed"
		nodes[0].Summary.NextAction = "inspect_retained_acceptance"
		nodes[0].Summary.NextOwner = "user"
		return saveGraphNode(ctx, tx, nodes[0])
	}); e != nil {
		t.Fatal(e)
	}
	before, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	r := s.StopGraphNode(ctx, owner, graph.Graph.ID, "a", domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: "1", NodeID: graph.Nodes[0].ID, ExpectedNodeControlRevision: "1"})
	if r.Status != 200 || r.Envelope.Data.(domain.GraphStopResult).StopAccepted || r.Envelope.Data.(domain.GraphStopResult).Reason != "already_settled" {
		t.Fatal("late Stop invented a current control scope", r)
	}
	after, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil || !reflect.DeepEqual(before.Nodes, after.Nodes) || before.Graph.StateRevision != after.Graph.StateRevision {
		t.Fatal("no-op changed retained node/group state", before, after, e)
	}
}
