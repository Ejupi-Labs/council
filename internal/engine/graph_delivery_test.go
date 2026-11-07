package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// Diagnostics load only existing bounded incident/control records. Explicitly
// formatting the pointed-to values keeps real failure codes/messages visible.
func graphDeliveryFailureDiagnostics(e *Engine, a domain.Attempt) string {
	primary, stop := "<nil>", "<nil>"
	if a.PrimaryFailure != nil {
		primary = fmt.Sprintf("%+v", *a.PrimaryFailure)
	}
	if a.Stop != nil {
		stop = fmt.Sprintf("%+v", *a.Stop)
	}
	gate, gateErr := e.db.Gate(context.Background())
	ids := []string{}
	if a.Cleanup.IncidentID != nil {
		ids = append(ids, *a.Cleanup.IncidentID)
	}
	if gate.IncidentID != nil && (len(ids) == 0 || ids[0] != *gate.IncidentID) {
		ids = append(ids, *gate.IncidentID)
	}
	incidents := []string{}
	for _, id := range ids {
		incident, err := e.db.Incident(context.Background(), id)
		incidents = append(incidents, fmt.Sprintf("id=%s record=%+v error=%v", id, incident, err))
	}
	journal := []string{}
	deliveries, deliveryErr := e.db.WorkflowDeliveries(context.Background(), a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID}, 10, "")
	for _, row := range deliveries.Items {
		if row.Operation != nil {
			op := row.Operation
			journal = append(journal, fmt.Sprintf("id=%s state=%s next=%s effectSequence=%s before=%t after=%t", op.ID, op.Status, op.NextAction, op.LastEffectSequence, op.BeforeDestination != nil, op.AfterDestination != nil))
		}
	}
	return fmt.Sprintf("primary=%s stop=%s evidence=%s cleanup=%+v gateState=%s gateError=%v incidents=%v journal=%v journalError=%v", primary, stop, a.EvidenceStatus, a.Cleanup, gate.State, gateErr, incidents, journal, deliveryErr)
}

// Every author, review, predecessor acceptance and local-delivery admission is
// real. The fixed offline workers qualify only this controlled fixture; no live
// provider, remote publication or usage measurement is involved.
type graphDeliveryCase struct {
	final            graphFinalReviewCase
	binding          domain.CandidateBinding
	candidate        domain.Candidate
	reviews          domain.Page[domain.ReviewRecord]
	admitted         domain.GraphAdvanceResult
	dispatch         domain.GraphDispatch
	delivery         domain.Attempt
	otherAcceptances map[string]domain.GraphAcceptedResultRef
	assertUserBasis  func()
}

func graphDeliveryAttemptFixture(t *testing.T) graphDeliveryCase {
	t.Helper()
	f := graphFinalReviewAttemptWithOptionsFixture(t, graphFinalVerificationOptions{
		Delivery: &domain.WorkflowDeliveryInput{LocalMode: "apply_verified_local"},
	})
	e, ctx := f.engine, context.Background()
	beforeBinding, beforeCandidate := *f.subject.Binding, *f.subject.Candidate
	if f.subject.Workflow.Authority.Delivery.LocalMode != "apply_verified_local" || beforeCandidate.HasDelta || beforeCandidate.HasIndexDelta ||
		beforeBinding.RootSourceSnapshotID == f.verified.Root.SourceSnapshotID || !f.verified.Binding.HasCumulativeWorkingDelta || f.verified.Binding.HasCumulativeIndexDelta {
		t.Fatal("The captured delivery fixture lost its distinct graph root or truthful zero-own-edit final", f.subject, f.verified.Binding)
	}
	assertUserBasis := func() {
		t.Helper()
		if body, err := os.ReadFile(filepath.Join(f.repo, "source.txt")); err != nil || !bytes.Equal(body, f.working) {
			t.Fatal("Graph delivery changed the original dirty working source", err)
		}
		if body, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(body, f.index) {
			t.Fatal("Graph delivery changed the exact original staged index", err)
		}
	}
	assertUserBasis()
	for _, name := range []string{fixture.GraphFileA, fixture.GraphFileB} {
		if _, err := os.Lstat(filepath.Join(f.repo, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("An intermediate worker applied files before designated final delivery", name, err)
		}
	}

	// The helper returned an actual unrun, separately admitted final reviewer.
	// Its complete cumulative approval must precede local delivery.
	e.run(ctx, f.reviewer)
	reviewed, err := e.db.Attempt(ctx, f.reviewer.ID)
	if err != nil || reviewed.Phase != "settled" || reviewed.Outcome == nil || *reviewed.Outcome != "succeeded" || reviewed.Cleanup.State != "verified" || reviewed.EvidenceStatus != "complete" || reviewed.PrimaryFailure != nil {
		t.Fatal("The actual cumulative final review did not qualify before delivery", reviewed, err)
	}
	reviews, err := e.db.WorkflowReviews(ctx, f.reviewer.TaskID, domain.WorkflowSelector{WorkflowID: f.reviewer.WorkflowID}, 100, "")
	if err != nil || len(reviews.Items) != 1 || reviews.NextCursor != nil || reviews.Items[0].AttemptID != reviewed.ID || reviews.Items[0].Outcome != "approved" ||
		reviews.Items[0].CandidateBinding == nil || *reviews.Items[0].CandidateBinding != beforeBinding {
		t.Fatal("Delivery has no exact current cumulative review receipt", reviews, err)
	}
	assertUserBasis()
	view, err := e.db.Graph(ctx, f.dispatch.GraphID)
	if err != nil || view.Budget.Budget.ConsumedAdmissions != "6" || view.Budget.Budget.RemainingAdmissions != "0" {
		t.Fatal("Five actual authors/reviews plus the explicit final-review grant did not use the shared allowance", view.Budget, err)
	}
	otherAcceptances := map[string]domain.GraphAcceptedResultRef{}
	for _, node := range view.Nodes {
		if node.ID == f.dispatch.NodeID {
			if node.AcceptedResult != nil {
				t.Fatal("The final node was accepted before its required delivery", node)
			}
			continue
		}
		if node.State != "completed" || node.AcceptedResult == nil || node.AcceptedResult.CandidateID == nil {
			t.Fatal("Delivery prerequisite is not an actual accepted file predecessor", node)
		}
		otherAcceptances[node.ID] = *node.AcceptedResult
	}
	if len(otherAcceptances) != 2 {
		t.Fatal("The fixture does not contain both actual accepted file branches", view.Nodes)
	}
	advance := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}}
	for _, node := range view.Nodes {
		advance.NodeAuthorities = append(advance.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	body, err := json.Marshal(advance)
	if err != nil {
		t.Fatal(err)
	}
	result := e.MutateGraph(ctx, "graphs.advance", view.Graph.ID, body)
	var admitted domain.GraphAdvanceResult
	wire, _ := json.Marshal(result.Envelope.Data)
	decodeErr := json.Unmarshal(wire, &admitted, json.RejectUnknownMembers(true))
	if result.Status != 200 || result.Envelope.Error != nil || decodeErr != nil || len(admitted.Dispatches) != 1 || len(admitted.PreparationAttemptIDs) != 0 ||
		!reflect.DeepEqual(admitted.Budget.Budget, view.Budget.Budget) {
		t.Fatal("Public Advance did not admit exactly one zero-unit final delivery", result, admitted, decodeErr)
	}
	d := admitted.Dispatches[0].Dispatch
	if d.Role != "local_delivery" || d.ExecutorKind != "local_application" || d.NodeID != f.dispatch.NodeID || d.TaskID != f.reviewer.TaskID ||
		d.WorkflowID != f.reviewer.WorkflowID || d.SourceBasis.BasisKind != "node_subject" || d.SourceBasis.CandidateBinding == nil ||
		*d.SourceBasis.CandidateBinding != beforeBinding || d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != f.verified.BindingRef {
		t.Fatal("The actual local delivery substituted its final node, selected candidate or cumulative basis", d)
	}
	delivery, err := e.db.ClaimAttempt(ctx, d.AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal("The actual graph local-delivery claim could not be acquired", err)
	}
	return graphDeliveryCase{final: f, binding: beforeBinding, candidate: beforeCandidate, reviews: reviews, admitted: admitted,
		dispatch: d, delivery: delivery, otherAcceptances: otherAcceptances, assertUserBasis: assertUserBasis}
}

func TestGraphDeliveryAppliesCumulativeRootWithoutInventingFinalAuthorship(t *testing.T) {
	prepared := graphDeliveryAttemptFixture(t)
	f, beforeBinding, beforeCandidate := prepared.final, prepared.binding, prepared.candidate
	e, ctx := f.engine, context.Background()
	reviews, admitted, d, delivery := prepared.reviews, prepared.admitted, prepared.dispatch, prepared.delivery
	otherAcceptances, assertUserBasis := prepared.otherAcceptances, prepared.assertUserBasis
	actions := []workspace.ApplyAction{}
	preReadyObserved := false
	e.config.BeforeApplicationReady = func(ctx context.Context, actual domain.Attempt) error {
		journal, err := e.db.WorkflowDeliveries(ctx, actual.TaskID, domain.WorkflowSelector{WorkflowID: actual.WorkflowID}, 10, "")
		if err != nil || actual.ID != delivery.ID || len(journal.Items) != 1 || journal.NextCursor != nil || journal.Items[0].Operation == nil {
			return fmt.Errorf("The pre-effect graph application has no exact durable journal: attempt=%s journal=%+v error=%v", actual.ID, journal, err)
		}
		op := journal.Items[0].Operation
		if op.AttemptID != delivery.ID || op.Status != "preparing" || op.LastEffectSequence != "0" || op.CandidateBinding != beforeBinding ||
			op.GraphDeliveryRef == nil || op.GraphDeliveryRef.ProducerAttemptID != delivery.ID || op.AfterDestination != nil {
			return fmt.Errorf("The pre-effect journal changed its binding or already permitted a destination action: %+v", *op)
		}
		preReadyObserved = true
		return nil
	}
	e.config.BeforeApplicationAction = func(_ context.Context, actual domain.Attempt, action workspace.ApplyAction) {
		if actual.ID != delivery.ID {
			t.Errorf("A different attempt obtained a graph destination permit: %s", actual.ID)
		}
		actions = append(actions, action)
	}
	e.run(ctx, delivery)
	assertUserBasis()
	actual, err := e.db.Attempt(ctx, delivery.ID)
	if err != nil {
		t.Fatal(err)
	}
	// This is the first delivery behavior assertion. A capability refusal or an
	// old node-root no-delta application cannot satisfy it with a proof-only DTO.
	for name, expected := range map[string]string{fixture.GraphFileA: fixture.GraphContentA, fixture.GraphFileB: fixture.GraphContentB} {
		got, readErr := os.ReadFile(filepath.Join(f.repo, name))
		if readErr != nil || string(got) != expected {
			t.Fatalf("The actual graph-root to selected-final delivery did not publish %s: bytes=%q err=%v attempt=%+v actions=%+v preReady=%t details=%s graph=%s", name, got, readErr, actual, actions, preReadyObserved, graphDeliveryFailureDiagnostics(e, actual), graphActorDiagnostics(e, d.GraphID))
		}
	}
	if actual.Phase != "settled" || actual.Outcome == nil || *actual.Outcome != "succeeded" || actual.PrimaryFailure != nil || actual.Stop != nil ||
		actual.Cleanup.State != "verified" || actual.Cleanup.Scope != "not_launched" || actual.EvidenceStatus != "complete" || actual.LaunchIntent || len(actions) != 2 || !preReadyObserved {
		t.Fatal("Applied cumulative delivery did not close and settle its actual local ownership", actual, actions, graphDeliveryFailureDiagnostics(e, actual))
	}
	for _, action := range actions {
		if action.Kind != "add" || action.Action != "publish_after" || action.RelativePath != fixture.GraphFileA && action.RelativePath != fixture.GraphFileB {
			t.Fatal("Delivery applied a path outside the exact cumulative root delta", action)
		}
	}
	current, err := e.db.Workflow(ctx, delivery.TaskID, domain.WorkflowSelector{WorkflowID: delivery.WorkflowID})
	if err != nil || current.Workflow == nil || current.Workflow.SelectedCandidateID == nil || *current.Workflow.SelectedCandidateID != beforeBinding.CandidateID {
		t.Fatal("Local delivery replaced the actual final producer candidate", current, err)
	}
	selected, err := e.selectedWorkflowSubject(ctx, *current.Workflow)
	if err != nil || selected.Binding == nil || *selected.Binding != beforeBinding || selected.Candidate == nil || selected.Candidate.ID != beforeCandidate.ID ||
		selected.Candidate.AttemptID != beforeCandidate.AttemptID || selected.Candidate.SourceSnapshotID != beforeCandidate.SourceSnapshotID ||
		selected.Candidate.RootSourceSnapshotID != beforeCandidate.RootSourceSnapshotID || selected.Candidate.HasDelta || selected.Candidate.HasIndexDelta {
		t.Fatal("Cumulative delivery rewrote final-node ancestry or manufactured authorship", selected, err)
	}
	op := workflowControlOperation(t, e, delivery.TaskID)
	if op.Status != "applied" || op.AttemptID != delivery.ID || op.StepID != d.StepID || op.CandidateBinding != beforeBinding || op.BeforeDestination == nil || op.AfterDestination == nil {
		t.Fatal("The exact applied graph delivery journal was not retained", op)
	}
	// Decode the additive operation reference without referring to a production
	// field before its implementation. Ordinary005 operation bytes stay separate.
	var operation struct {
		GraphDeliveryRef *domain.GraphArtifactRef `json:"graphDeliveryRef"`
	}
	opBytes, err := json.Marshal(op)
	if err != nil || json.Unmarshal(opBytes, &operation) != nil || operation.GraphDeliveryRef == nil || operation.GraphDeliveryRef.ProducerAttemptID != delivery.ID {
		t.Fatal("Applied graph delivery has no exact attempt-owned binding reference", op, err)
	}
	raw, err := e.readGraphArtifact(ctx, *operation.GraphDeliveryRef, "graph_delivery", 64<<20)
	if err != nil {
		t.Fatal("The applied operation's exact graph delivery binding is unavailable", err)
	}
	var binding domain.GraphDeliveryBinding
	if err = json.Unmarshal(raw, &binding, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	seal, err := domain.GraphDeliveryDigest(binding)
	if err != nil || seal != binding.BindingSHA256 || binding.GraphID != d.GraphID || binding.GraphVersion != d.GraphVersion || binding.GraphControlRevision != d.GraphControlRevision ||
		binding.FinalNodeID != d.NodeID || binding.FinalTaskID != delivery.TaskID || binding.FinalTaskVersion != delivery.TaskVersion || binding.FinalWorkflowID != delivery.WorkflowID ||
		binding.FinalCandidateBinding != beforeBinding || binding.FinalInputID != d.InputID || binding.FinalVerificationRef != f.verified.BindingRef ||
		!reflect.DeepEqual(binding.RootBinding, domain.GraphRootPublic(f.verified.Root)) || !reflect.DeepEqual(binding.ContributorVector, f.verified.Binding.ContributorVector) ||
		!domain.ValidSHA256(binding.DeliveryScopeSHA256) || len(binding.OtherNodeAcceptances) != 2 || bytes.Contains(raw, []byte(`"projectIdentity"`)) || bytes.Contains(raw, []byte(`"repositoryIdentity"`)) {
		t.Fatal("The applied graph binding changed its exact public root, selected final or cumulative authority", binding, err)
	}
	for _, contribution := range binding.OtherNodeAcceptances {
		accepted, ok := otherAcceptances[contribution.NodeID]
		if !ok || contribution.AcceptanceKind != accepted.AcceptanceKind || contribution.AcceptanceID != accepted.AcceptanceID || contribution.TaskVersion != accepted.TaskVersion ||
			contribution.ProducerAttemptID != accepted.ProducerAttemptID || contribution.InputID != accepted.InputID || contribution.CandidateBinding == nil || accepted.CandidateID == nil ||
			contribution.CandidateBinding.CandidateID != *accepted.CandidateID {
			t.Fatal("The delivery binding substituted a required accepted predecessor", contribution, accepted)
		}
		delete(otherAcceptances, contribution.NodeID)
	}
	if len(otherAcceptances) != 0 {
		t.Fatal("The delivery binding omitted a required accepted predecessor", otherAcceptances)
	}
	foundReview := false
	for _, use := range binding.PreDeliveryEvidenceUses {
		if use.Kind == "review" && use.EvidenceID == reviews.Items[0].ID && use.CandidateID == beforeBinding.CandidateID && reflect.DeepEqual(use.Applicability, reviews.Items[0].Applicability) {
			foundReview = true
		}
	}
	if !foundReview {
		t.Fatal("Delivery was not bound to the actual fresh cumulative final review", binding.PreDeliveryEvidenceUses)
	}
	budget, err := e.db.GraphBudget(ctx, d.GraphID)
	if err != nil || !reflect.DeepEqual(budget.Budget, admitted.Budget.Budget) {
		t.Fatal("Local graph delivery consumed or granted model allowance", budget, err)
	}
}
