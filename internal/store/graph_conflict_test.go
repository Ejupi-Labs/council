package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphConflictedInputFixture(t *testing.T) (*Store, domain.GraphInputOperation, domain.GraphInputCompletionFacts, string) {
	t.Helper()
	s, graph, root, proofs, owner := graphReadyBasis(t)
	ctx := context.Background()
	op, e := s.BeginGraphInput(ctx, owner, graph.Graph.ID, graph.Nodes[0].ID, graphBeginFacts(graph, graph.Nodes[0], root, proofs, owner))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ClaimAttempt(ctx, op.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	conflict, cp := graphPayload(t, s, op.Attempt.ID, "graph_input_provenance", []byte(`{"contract":"006-input-conflict-v1","conflicts":[{"dimension":"working","path":"same.txt"}]}`))
	effects, ep := graphPayload(t, s, op.Attempt.ID, "graph_operation_effects", []byte(`{"controlled":"known conflict inspection effects"}`))
	return s, op, domain.GraphInputCompletionFacts{ConflictArtifact: &conflict, OperationEffects: &effects, EffectCount: "1", EffectsKnown: true, EvidenceStatus: "complete", Failure: domain.Fail("input_conflict", "The accepted branches conflict in this exact retained input."), Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, State: "conflicted", VerifiedArtifacts: []domain.VerifiedResponse{cp, ep}}, owner
}

func TestGraphConflictedInputRetainsKnownConflictWithoutUsableBindingOrDebit(t *testing.T) {
	s, op, f, owner := graphConflictedInputFixture(t)
	ctx := context.Background()
	before, e := s.GraphBudget(ctx, op.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CompleteGraphInput(ctx, owner, op.Attempt.ID, f); e != nil {
		t.Fatal("known composition conflict could not be retained", e)
	}
	input, e := s.GraphInput(ctx, op.GraphID, op.ID)
	if e != nil || input.Summary.State != "conflicted" || input.Summary.Current || input.ConflictArtifact == nil || *input.ConflictArtifact != *f.ConflictArtifact || input.InputBinding != nil || input.OperationEffects == nil || *input.OperationEffects != *f.OperationEffects {
		t.Fatal("conflict was hidden or presented as usable input", input, e)
	}
	a, e := s.Attempt(ctx, op.Attempt.ID)
	if e != nil || a.Phase != "settled" || a.Outcome == nil || *a.Outcome != "blocked" || a.SourceSnapshotID != nil || a.LaunchIntent {
		t.Fatal("conflict invented source/native success", a, e)
	}
	claims, e := s.GraphClaims(ctx)
	if e != nil || claimedAttempt(claims, op.Attempt.ID) {
		t.Fatal("known settled conflict retained a false active claim", claims, e)
	}
	after, e := s.GraphBudget(ctx, op.GraphID)
	if e != nil || after.Budget.ConsumedAdmissions != before.Budget.ConsumedAdmissions || after.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("composition conflict spent or opened model authority", before, after, e)
	}
	view, e := s.Graph(ctx, op.GraphID)
	if e != nil || view.Nodes[0].State != "blocked" || view.Nodes[0].NextOwner != "user" {
		t.Fatal("known conflict still promised automatic source readiness", view, e)
	}
}

func TestGraphConflictedInputRequiresFreshConflictAndCompleteAssemblySource(t *testing.T) {
	for _, kind := range []string{"missing_conflict", "missing_proof", "partial_source"} {
		t.Run(kind, func(t *testing.T) {
			s, op, f, owner := graphConflictedInputFixture(t)
			switch kind {
			case "missing_conflict":
				f.ConflictArtifact = nil
			case "missing_proof":
				f.VerifiedArtifacts = f.VerifiedArtifacts[1:]
			case "partial_source":
				f.SourceSnapshot = &domain.SourceSnapshot{ID: domain.NewID(), AttemptID: op.Attempt.ID, ProjectIdentity: op.RootBinding.ProjectIdentity, BaseCommit: op.RootBinding.BaseCommit, SelectedRelativePath: op.RootBinding.SelectedRelativePath, Branch: op.RootBinding.Branch}
			}
			if e := s.CompleteGraphInput(context.Background(), owner, op.Attempt.ID, f); e == nil {
				t.Fatal("conflict retained without its exact proof or with an invented partial source", kind)
			}
			a, e := s.Attempt(context.Background(), op.Attempt.ID)
			if e != nil || a.Phase == "settled" || a.SourceSnapshotID != nil {
				t.Fatal("invalid conflict partly settled", a, e)
			}
		})
	}
}
