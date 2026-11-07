package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphBudgetProofUsesAuthorityEpochInsteadOfSiblingDebitRevision(t *testing.T) {
	s, graph, _, owner := graphLocalFixture(t)
	_ = owner
	ctx := context.Background()
	node := graph.Nodes[0]
	// A controlled sibling-admission accounting snapshot changes revision/usage,
	// while the graph's explicitly granted authority/window remain the same.
	if e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		b, e := s.taskBudget(ctx, tx, node.TaskID)
		if e != nil {
			return e
		}
		b.Revision = "2"
		b.ConsumedAdmissions = "1"
		b.RemainingAdmissions = "3"
		now := time.Now().UTC()
		first := now.Format(time.RFC3339Nano)
		deadline := now.Add(30 * time.Minute).Format(time.RFC3339Nano)
		b.CurrentWindow.FirstAdmissionAt = &first
		b.CurrentWindow.DeadlineAt = &deadline
		return saveBudget(ctx, tx, *b)
	}); e != nil {
		t.Fatal(e)
	}
	if current, e := s.graphBudgetProofCurrent(ctx, s.db, node.TaskID, "1", "1", "1"); e != nil || !current {
		t.Fatal("unrelated accounting debit invalidated unchanged graph authority", current, e)
	}
	if e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		h, e := s.graphHeader(ctx, tx, graph.Graph.ID)
		if e != nil {
			return e
		}
		h.Summary.BudgetAuthorityEpoch = "2"
		return saveGraphHeader(ctx, tx, h)
	}); e != nil {
		t.Fatal(e)
	}
	if current, e := s.graphBudgetProofCurrent(ctx, s.db, node.TaskID, "2", "1", "1"); e != nil || current {
		t.Fatal("explicit new authority rearmed an older proof", current, e)
	}
	if current, e := s.graphBudgetProofCurrent(ctx, s.db, node.TaskID, "2", "2", "2"); e != nil || current {
		t.Fatal("proof silently moved to another budget window", current, e)
	}
}
func TestGraphBudgetProofPreservesStandaloneRevisionBinding(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := openFixture(t, s, "standalone-proof-binding")
	r := s.CreateTask(ctx, domain.NewID(), taskInput(p.ID))
	if r.Status != 201 {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.Admission)
	b, e := s.TaskBudget(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	if current, e := s.graphBudgetProofCurrent(ctx, s.db, a.Task.ID, b.Budget.Revision, "", b.Budget.CurrentWindow.Number); e != nil || !current {
		t.Fatal("accepted standalone proof comparison changed", current, e)
	}
	if current, e := s.graphBudgetProofCurrent(ctx, s.db, a.Task.ID, "1", "", b.Budget.CurrentWindow.Number); e != nil || current {
		t.Fatal("stale standalone budget revision was accepted", current, e)
	}
}
