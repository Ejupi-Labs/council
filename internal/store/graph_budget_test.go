package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphTestStart(t *testing.T, s *Store) (domain.GraphStartInput, domain.GraphCreateFacts, string) {
	t.Helper()
	p := openFixture(t, s, "graph-budget-project")
	owner := domain.NewID()
	if e := s.RecoverOwner(context.Background(), owner); e != nil {
		t.Fatal(e)
	}
	shared := "\ufeff Exact shared text\r\n"
	return domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: p.ID, ExecutionMode: "fake", SharedInstructions: &shared, Nodes: []domain.GraphNodeInput{{Key: "a", OriginalText: "Exact A"}, {Key: "b", OriginalText: "Exact B"}}}, domain.GraphCreateFacts{ProjectIdentity: "graph-budget-project", RepositoryIdentity: "typed-fake-repository", BaseCommit: strings.Repeat("a", 40)}, owner
}

// Actual SQLite transactions with fake controller facts; no filesystem/model work.
func TestGraphCreationHasOneCanonicalBudgetAndNoChildGrant(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in, facts, owner := graphTestStart(t, s)
	first := s.CreateGraph(ctx, owner, in, facts)
	if first.Status != 201 {
		t.Fatal(first)
	}
	admission := first.Envelope.Data.(domain.GraphAdmission)
	if len(admission.Nodes) != 2 || admission.Graph.MaxParallelism != "2" || admission.Budget.BudgetOwnerTaskID != admission.Graph.RootTaskID || admission.Budget.Budget.GrantedAdmissions != "4" || admission.Budget.Budget.ConsumedAdmissions != "0" || admission.Budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("graph creation invented native authority/debit/window", admission)
	}
	var budgetCount, attemptCount int
	if e := s.db.QueryRow("SELECT count(*) FROM task_budgets").Scan(&budgetCount); e != nil || budgetCount != 1 {
		t.Fatal("graph multiplied its budget", budgetCount, e)
	}
	if e := s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&attemptCount); e != nil || attemptCount != 1 {
		t.Fatal("graph creation dispatched declared workers", attemptCount, e)
	}
	operation, e := s.Attempt(ctx, admission.PreparationAttemptID)
	if e != nil || operation.TaskID != admission.Graph.RootTaskID || operation.ExecutorKind != "local_graph" || operation.Role != "graph_basis" || operation.ResolvedTarget != nil {
		t.Fatal("root basis has no real distinct operation owner", operation, e)
	}
	root, e := s.Task(ctx, admission.Graph.RootTaskID)
	if e != nil || root.Task.GraphRef == nil || root.Task.GraphRef.Kind != "root" {
		t.Fatal(root, e)
	}
	for _, node := range admission.Nodes {
		child, e := s.Task(ctx, node.TaskID)
		if e != nil || child.Task.LatestAttemptID != nil || child.Task.GraphRef == nil || child.Task.GraphRef.NodeID == nil || *child.Task.GraphRef.NodeID != node.ID {
			t.Fatal("pending node lacks membership or invents worker", child, e)
		}
		public, e := s.TaskBudget(ctx, node.TaskID)
		if e != nil || public.Budget != nil || public.AdoptionRequired || public.GraphBudgetRef == nil || public.GraphBudgetRef.BudgetOwnerTaskID != admission.Graph.RootTaskID {
			t.Fatal("root budget disguised as a child balance", public, e)
		}
		if _, e = s.BudgetLedger(ctx, node.TaskID, 10, ""); e == nil {
			t.Fatal("child ledger pretends to own shared accounting")
		}
	}
	publicDefinition, e := s.GraphDefinition(ctx, admission.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	privateDefinition, e := s.GraphDefinitionPrivate(ctx, admission.Graph.ID)
	if e != nil || privateDefinition.Delivery.ProjectIdentity != facts.ProjectIdentity || privateDefinition.Delivery.RepositoryIdentity != facts.RepositoryIdentity {
		t.Fatal("private recovery basis was discarded", privateDefinition, e)
	}
	publicJSON := encode(publicDefinition)
	if strings.Contains(publicJSON, "projectIdentity") || strings.Contains(publicJSON, "repositoryIdentity") || strings.Contains(publicJSON, facts.RepositoryIdentity) {
		t.Fatal("public definition leaks native identities", publicJSON)
	}
	var before string
	if e := s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", in.RequestID).Scan(&before); e != nil {
		t.Fatal(e)
	}
	if r := s.SetPolicy(ctx, domain.NewID(), "new-policy", "1", "supervised"); r.Status != 200 {
		t.Fatal(r)
	}
	facts.ProjectIdentity = "changed-after-admission"
	replay := s.CreateGraph(ctx, owner, in, facts)
	replayed, decodeErr := decode[domain.GraphAdmission](encode(replay.Envelope.Data))
	var after string
	if e := s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", in.RequestID).Scan(&after); e != nil {
		t.Fatal(e)
	}
	if replay.Status != 201 || decodeErr != nil || !reflect.DeepEqual(admission, replayed) || !reflect.DeepEqual(first.Envelope.Receipt, replay.Envelope.Receipt) || before != after {
		t.Fatal("exact graph receipt reconsulted changed controls", replay, decodeErr)
	}
}

func TestGraphCreationFaultsNeverMultiplyAuthority(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit"}[after], func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			in, facts, owner := graphTestStart(t, s)
			fault := func() error { return errors.New("fixed graph receipt fault") }
			if after {
				s.AfterCommit = fault
			} else {
				s.BeforeCommit = fault
			}
			result := s.CreateGraph(ctx, owner, in, facts)
			want := "storage_failed"
			rows := 0
			if after {
				want = "write_uncertain"
				rows = 1
			}
			if result.Envelope.Error == nil || result.Envelope.Error.Code != want {
				t.Fatal(result)
			}
			var n int
			if e := s.db.QueryRow("SELECT count(*) FROM task_graphs").Scan(&n); e != nil || n != rows {
				t.Fatal("fault escaped graph transaction", n, e)
			}
			s.BeforeCommit = nil
			s.AfterCommit = nil
			if r := s.CreateGraph(ctx, owner, in, facts); r.Status != 201 {
				t.Fatal(r)
			}
			if e := s.db.QueryRow("SELECT count(*) FROM task_budgets").Scan(&n); e != nil || n != 1 {
				t.Fatal("retry multiplied graph authority", n, e)
			}
		})
	}
}
