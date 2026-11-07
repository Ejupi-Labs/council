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
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// A's file is retained only as context. The final really authors matching A
// bytes on top of its B-only input; this must not manufacture a files edge.
func TestGraphExternalContextOnlyFileHeadCannotPublishMatchingFinalBytes(t *testing.T) {
	t.Setenv("COUNCIL_TEST_FINAL_CHECK_SENTINEL", "preserved")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	timeout := 10
	action := domain.ExternalActionInput{Action: "controlled-graph-ancestry-publication", Destination: "owned-offline-fixture", AcceptanceBasis: "command_completion",
		Command: domain.ExternalCommandInput{Executable: executable, Arguments: []string{graphFinalContextCheckTestArg, "--outcome", "pass"}, TimeoutSeconds: &timeout}}
	f := graphFinalReviewAttemptWithOptionsFixture(t, graphFinalVerificationOptions{ContextOnlyARecreatedByFinal: true,
		Delivery: &domain.WorkflowDeliveryInput{LocalMode: "review_only", ExternalAction: &action}})
	e, ctx := f.engine, context.Background()
	beforeCandidate, beforeBinding := *f.subject.Candidate, *f.subject.Binding
	if !beforeCandidate.HasDelta || beforeCandidate.HasIndexDelta || !f.verified.Binding.HasCumulativeWorkingDelta || f.verified.Binding.HasCumulativeIndexDelta {
		t.Fatal("The actual final did not create A on top of its distinct materialized input", beforeCandidate, f.verified.Binding)
	}
	input, err := e.workspace.VerifyGraphInput(ctx, f.verified.Input)
	if err != nil {
		t.Fatal("The actual final's retained input is unavailable", err)
	}
	inputHasB := false
	for _, entry := range input.Manifest.Entries {
		if entry.Path == fixture.GraphFileA {
			t.Fatal("The fixture inherited A as files instead of recreating it", entry)
		}
		inputHasB = inputHasB || entry.Path == fixture.GraphFileB && entry.Type == "file"
	}
	if !inputHasB {
		t.Fatal("The actual final input did not inherit B through its file dependency")
	}
	view, err := e.db.Graph(ctx, f.dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	fileAncestors, err := graphMemberFileAncestors(definition, view.Nodes, view.Graph.GraphVersion, f.dispatch.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	var aNode, bNode *domain.GraphNodeSummary
	for i := range view.Nodes {
		switch view.Nodes[i].Key {
		case "a":
			aNode = &view.Nodes[i]
		case "b":
			bNode = &view.Nodes[i]
		}
	}
	if aNode == nil || bNode == nil || aNode.AcceptedResult == nil || bNode.AcceptedResult == nil || fileAncestors[aNode.ID] || !fileAncestors[bNode.ID] {
		t.Fatal("The actual captured topology/accepted cohort is not context A plus files B", definition, view.Nodes, fileAncestors)
	}
	var aContribution *domain.AcceptedContribution
	for i := range f.verified.Input.Contributions {
		if f.verified.Input.Contributions[i].FromNodeID == aNode.ID {
			aContribution = &f.verified.Input.Contributions[i]
		}
	}
	if aContribution == nil || aContribution.EdgeKind != "context" || aContribution.CandidateBinding == nil || aContribution.AcceptanceID != aNode.AcceptedResult.AcceptanceID {
		t.Fatal("A is not an exact actual accepted context contribution", aContribution)
	}
	upstream, err := e.db.GraphInputRecord(ctx, view.Graph.ID, aContribution.InputID)
	if err != nil || upstream.Public.InputBinding == nil {
		t.Fatal(upstream, err)
	}
	aOutput, err := e.workspace.VerifyGraphOutput(ctx, workspace.GraphOutputVerifyRequest{RootBinding: f.verified.Root, Contribution: *aContribution, InputBinding: *upstream.Public.InputBinding})
	if err != nil {
		t.Fatal("The actual accepted A output is unavailable", err)
	}
	ancestorFile, ancestorBytes := graphExternalAncestryReadFile(t, e, aOutput.OutputManifest, fixture.GraphFileA)
	finalFile, finalBytes := graphExternalAncestryReadFile(t, e, f.verified.OutputProof.Manifest, fixture.GraphFileA)
	if !bytes.Equal(ancestorBytes, []byte(fixture.GraphContentA)) || !bytes.Equal(finalBytes, ancestorBytes) || ancestorFile.SHA256 != finalFile.SHA256 || ancestorFile.ArtifactID == finalFile.ArtifactID {
		t.Fatal("The final did not actually recreate matching A bytes under its own output evidence", ancestorFile, finalFile)
	}
	finalMetadata, err := e.db.Artifact(ctx, finalFile.ArtifactID)
	if err != nil || finalMetadata.AttemptID != beforeCandidate.AttemptID {
		t.Fatal("The matching final bytes are borrowed ancestor evidence instead of actual final authorship", finalMetadata, err)
	}
	// Establish real cumulative approval before testing publication permission.
	// A fake response, recreated bytes or predecessor approval cannot replace it.
	e.run(ctx, f.reviewer)
	reviewed, err := e.db.Attempt(ctx, f.reviewer.ID)
	if err != nil || reviewed.Phase != "settled" || reviewed.Outcome == nil || *reviewed.Outcome != "succeeded" || reviewed.PrimaryFailure != nil || reviewed.EvidenceStatus != "complete" {
		t.Fatal("The actual matching-file cumulative reviewer did not qualify", reviewed, err)
	}
	view, err = e.db.Graph(ctx, view.Graph.ID)
	if err != nil {
		t.Fatal(err)
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
	wire, _ := json.Marshal(result.Envelope.Data)
	var advanced domain.GraphAdvanceResult
	if result.Status != 200 || result.Envelope.Error != nil || json.Unmarshal(wire, &advanced, json.RejectUnknownMembers(true)) != nil || len(advanced.Dispatches) != 1 || len(advanced.PreparationAttemptIDs) != 0 {
		t.Fatal("The real reviewed final did not admit its declared publisher", result, advanced)
	}
	dispatch := advanced.Dispatches[0].Dispatch
	if dispatch.Role != "external_delivery" || dispatch.ExecutorKind != "local_external" || dispatch.SourceBasis.CandidateBinding == nil || *dispatch.SourceBasis.CandidateBinding != beforeBinding ||
		dispatch.SourceBasis.FinalVerificationRef == nil || *dispatch.SourceBasis.FinalVerificationRef != f.verified.BindingRef {
		t.Fatal("The admitted publisher changed its actual selected final basis", dispatch)
	}
	publisher, err := e.db.ClaimAttempt(ctx, dispatch.AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal("The actual publisher claim prerequisite failed before its ancestry check", err)
	}
	task, err := e.db.Task(ctx, publisher.TaskID)
	if err != nil || task.Task.GraphRef == nil {
		t.Fatal(task, err)
	}
	current, err := e.db.Workflow(ctx, publisher.TaskID, domain.WorkflowSelector{WorkflowID: publisher.WorkflowID})
	if err != nil || current.Workflow == nil {
		t.Fatal(current, err)
	}
	subject, err := e.selectedWorkflowSubject(ctx, *current.Workflow)
	if err != nil {
		t.Fatal(err)
	}
	_, scopeErr := e.graphExternalScope(ctx, publisher, *task.Task.GraphRef, subject)
	if scopeErr == nil || domain.SafeError(scopeErr).Code != "graph_file_ancestry_unproven" {
		t.Error("The actual Engine publication scope accepted matching bytes without a declared file ancestry", scopeErr)
	}
	prepares := 0
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		prepares++
		return process.Prepare(ctx, spec)
	}
	e.run(ctx, publisher)
	actual, err := e.db.Attempt(ctx, publisher.ID)
	if err != nil || prepares != 0 || actual.LaunchIntent || actual.Phase != "settled" || actual.Outcome == nil || *actual.Outcome == "succeeded" || actual.PrimaryFailure == nil || actual.Cleanup.State != "verified" || actual.EvidenceStatus == "uncertain" {
		t.Fatal("Context-only file ancestry obtained command preparation/GO/publication or false uncertainty", actual, prepares, err, graphDeliveryFailureDiagnostics(e, actual))
	}
	deliveries, err := e.db.WorkflowDeliveries(ctx, publisher.TaskID, domain.WorkflowSelector{WorkflowID: publisher.WorkflowID}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, delivery := range deliveries.Items {
		if delivery.Acceptance != nil || delivery.External != nil && delivery.External.Outcome == "command_succeeded" {
			t.Fatal("An unqualified files branch became an accepted or successful external publication", delivery)
		}
	}
	for name, expected := range map[string][]byte{"source.txt": f.working, filepath.Join(".git", "index"): f.index} {
		actual, err := os.ReadFile(filepath.Join(f.repo, name))
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatal("An ancestry refusal changed the user's original working/index basis", name, err)
		}
	}
	for _, name := range []string{fixture.GraphFileA, fixture.GraphFileB} {
		if _, err := os.Lstat(filepath.Join(f.repo, name)); !os.IsNotExist(err) {
			t.Fatal("The graph publisher implicitly applied a managed file", name, err)
		}
	}
	candidate, err := e.db.Candidate(ctx, beforeCandidate.ID)
	if err != nil || !reflect.DeepEqual(candidate, beforeCandidate) {
		t.Fatal("An ancestry refusal rewrote final-node authorship or root IDs", candidate, err)
	}
	budget, err := e.db.GraphBudget(ctx, view.Graph.ID)
	if err != nil || !reflect.DeepEqual(budget.Budget, advanced.Budget.Budget) {
		t.Fatal("The zero-unit ancestry refusal changed model allowance", budget, err)
	}
}

func graphExternalAncestryReadFile(t *testing.T, e *Engine, manifest workspace.Manifest, path string) (workspace.Entry, []byte) {
	t.Helper()
	for _, entry := range manifest.Entries {
		if entry.Path != path || entry.Type != "file" {
			continue
		}
		var body bytes.Buffer
		meta, err := e.evidence.CopyTo(context.Background(), entry.ArtifactID, &body, 64<<20)
		if err != nil || meta.SHA256 == nil || *meta.SHA256 != entry.SHA256 || meta.ByteLength != domain.Decimal(entry.Size) {
			t.Fatal("The actual retained ancestry file payload is unavailable", entry, meta, err)
		}
		return entry, body.Bytes()
	}
	t.Fatal("The actual retained manifest omitted the required file", path)
	return workspace.Entry{}, nil
}
