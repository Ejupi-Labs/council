package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
)

// The declared publisher is this owned test binary's fixed, read-only descriptor
// inspection command. No remote publication, installed harness or shell runs.
func TestGraphExternalFinalExecutesOnlyCapturedCommandWithExactCumulativeContext(t *testing.T) {
	t.Setenv("COUNCIL_TEST_FINAL_CHECK_SENTINEL", "preserved")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	timeout := 10
	action := domain.ExternalActionInput{Action: "controlled-graph-publication", Destination: "owned-offline-fixture", AcceptanceBasis: "command_completion",
		Command: domain.ExternalCommandInput{Executable: executable, Arguments: []string{graphFinalContextCheckTestArg, "--outcome", "pass"}, TimeoutSeconds: &timeout}}
	f := graphFinalReviewAttemptWithOptionsFixture(t, graphFinalVerificationOptions{Delivery: &domain.WorkflowDeliveryInput{LocalMode: "review_only", ExternalAction: &action}})
	e, ctx := f.engine, context.Background()
	e.run(ctx, f.reviewer)
	reviewer, err := e.db.Attempt(ctx, f.reviewer.ID)
	if err != nil || reviewer.Phase != "settled" || reviewer.Outcome == nil || *reviewer.Outcome != "succeeded" || reviewer.PrimaryFailure != nil || reviewer.EvidenceStatus != "complete" {
		t.Fatal("The real cumulative review did not qualify the explicit publisher", reviewer, err)
	}
	view, err := e.db.Graph(ctx, f.dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}}
	for _, node := range view.Nodes {
		in.NodeAuthorities = append(in.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	result := e.MutateGraph(ctx, "graphs.advance", view.Graph.ID, body)
	wire, _ := json.Marshal(result.Envelope.Data)
	var advanced domain.GraphAdvanceResult
	if result.Status != 200 || result.Envelope.Error != nil || json.Unmarshal(wire, &advanced, json.RejectUnknownMembers(true)) != nil || len(advanced.Dispatches) != 1 || len(advanced.PreparationAttemptIDs) != 0 {
		t.Fatal("Explicit Advance did not admit exactly the declared final publisher", result, advanced)
	}
	d := advanced.Dispatches[0].Dispatch
	if d.Role != "external_delivery" || d.ExecutorKind != "local_external" || d.NodeID != f.dispatch.NodeID || d.InputID != f.dispatch.InputID ||
		d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != f.verified.BindingRef || d.SourceBasis.CandidateBinding == nil || *d.SourceBasis.CandidateBinding != *f.subject.Binding ||
		!reflect.DeepEqual(advanced.Budget.Budget, view.Budget.Budget) {
		t.Fatal("The publisher substituted its exact final or consumed a model admission", d, advanced.Budget)
	}
	publisher, err := e.db.ClaimAttempt(ctx, d.AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, publisher)
	actual, err := e.db.Attempt(ctx, publisher.ID)
	if err != nil || actual.Phase != "settled" || actual.Outcome == nil || *actual.Outcome != "succeeded" || actual.PrimaryFailure != nil || actual.StartedAt == nil || actual.Cleanup.State != "verified" || actual.EvidenceStatus != "complete" {
		t.Fatalf("The explicitly scoped real final publisher did not run: attempt=%+v err=%v details=%s", actual, err, graphDeliveryFailureDiagnostics(e, actual))
	}
	deliveries, err := e.db.WorkflowDeliveries(ctx, publisher.TaskID, domain.WorkflowSelector{WorkflowID: publisher.WorkflowID}, 100, "")
	if err != nil || len(deliveries.Items) != 1 || deliveries.Items[0].External == nil {
		t.Fatal("The real command outcome did not retain its external record", deliveries, err)
	}
	observed := deliveries.Items[0].External
	if observed.AttemptID != actual.ID || observed.Outcome != "command_succeeded" || !reflect.DeepEqual(observed.Action, action) || observed.StdoutArtifactID == nil ||
		observed.Command.Executable != executable || !reflect.DeepEqual(observed.Command.Arguments, action.Command.Arguments) {
		t.Fatal("The retained publisher changed its captured command or asserted remote attestation", observed)
	}
	var output bytes.Buffer
	if _, err := e.evidence.CopyTo(ctx, *observed.StdoutArtifactID, &output, evidence.StdoutLimit); err != nil || !bytes.Contains(output.Bytes(), []byte(`"fixture_final_context_reads"`)) {
		t.Fatal("The actual publisher never read the complete supplied cumulative descriptor", output.String(), err)
	}
	for name, expected := range map[string][]byte{"source.txt": f.working, filepath.Join(".git", "index"): f.index} {
		if actual, err := os.ReadFile(filepath.Join(f.repo, name)); err != nil || !bytes.Equal(actual, expected) {
			t.Fatal("The publisher changed the user's original dirty/staged work", name, err)
		}
	}
	for _, name := range []string{fixture.GraphFileA, fixture.GraphFileB} {
		if _, err := os.Lstat(filepath.Join(f.repo, name)); !os.IsNotExist(err) {
			t.Fatal("Review-only publisher implicitly applied managed file changes", name, err)
		}
	}
	current, err := e.db.Candidate(ctx, f.subject.Candidate.ID)
	if err != nil || current.HasDelta || current.HasIndexDelta || current.RootSourceSnapshotID != f.subject.Candidate.RootSourceSnapshotID {
		t.Fatal("The external command invented final authorship or changed node ancestry", current, err)
	}
}
