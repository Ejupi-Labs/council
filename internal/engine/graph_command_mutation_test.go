package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// The fixed TestMain command accepts no filename. It can only create the named
// controlled file inside the receiving managed workspace beside its own exact
// private context directory. Existing pass/fail commands never call this helper.
func graphCommandWriteFixedMutation(contextDirectory string) error {
	cwd, err := os.Getwd()
	if err != nil || !filepath.IsAbs(cwd) || filepath.Clean(cwd) != cwd || !domain.ValidID(filepath.Base(cwd)) || filepath.Dir(cwd) != filepath.Dir(contextDirectory) || filepath.Base(filepath.Dir(cwd)) != "workspaces" {
		return errors.New("The fixed mutation requires its actual receiving managed workspace.")
	}
	info, err := os.Lstat(cwd)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("The controlled receiving directory is unavailable.")
	}
	root, err := os.OpenRoot(cwd)
	if err != nil {
		return err
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	if err = errors.Join(localstate.VerifyPrivateFile(directory, true), directory.Close()); err != nil {
		return err
	}
	file, err := root.OpenFile(fixture.WorkflowFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write([]byte(fixture.WorkflowInvalid))
	return errors.Join(writeErr, file.Sync(), file.Close())
}

func graphCommandAdvanceStage(t *testing.T, e *Engine, graphID string) (domain.GraphAdvanceResult, domain.GraphDetail) {
	t.Helper()
	ctx := context.Background()
	view, err := e.db.Graph(ctx, graphID)
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
	result := e.MutateGraph(ctx, "graphs.advance", graphID, body)
	var advanced domain.GraphAdvanceResult
	wire, _ := json.Marshal(result.Envelope.Data)
	decodeErr := json.Unmarshal(wire, &advanced, json.RejectUnknownMembers(true))
	if result.Status != 200 || result.Envelope.Error != nil || decodeErr != nil || len(advanced.Dispatches) != 1 || len(advanced.PreparationAttemptIDs) != 0 || len(advanced.ReboundNodes) != 0 {
		t.Fatal("The exact explicit stage did not admit one linked execution", result, advanced, decodeErr)
	}
	return advanced, view
}

func TestGraphMutatingCommandRetainsNewOutputWithoutQualifyingItsInput(t *testing.T) {
	f := graphFinalCommandAttemptForOutcomeFixture(t, "mutate")
	e, ctx := f.engine, context.Background()
	t.Setenv("COUNCIL_TEST_FINAL_CHECK_SENTINEL", "preserved")
	rootBefore, err := e.db.GraphRoot(ctx, f.dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	prior := *f.subject.Candidate
	var spec process.NativeSpec
	prepares := 0
	e.config.Prepare = func(ctx context.Context, actual process.NativeSpec) (*process.Owned, error) {
		prepares++
		spec = actual
		spec.Args = append([]string{}, actual.Args...)
		return process.Prepare(ctx, actual)
	}
	e.run(ctx, f.check)
	actual, err := e.db.Attempt(ctx, f.check.ID)
	if err != nil || prepares != 1 || actual.StartedAt == nil || actual.Phase != "settled" || actual.Outcome == nil || *actual.Outcome != "succeeded" || actual.Cleanup.State != "verified" || actual.EvidenceStatus != "complete" || !reflect.DeepEqual(spec.Args, f.definition.Command.Arguments) {
		t.Fatal("The fixed mutating command did not complete an actual owned process", actual, prepares, err)
	}
	if body, err := os.ReadFile(filepath.Join(spec.Cwd, fixture.WorkflowFile)); err != nil || string(body) != fixture.WorkflowInvalid {
		t.Fatal("The fixed command did not mutate its own receiving workspace", err)
	}
	checks, err := e.db.WorkflowChecks(ctx, f.check.TaskID, domain.WorkflowSelector{WorkflowID: f.check.WorkflowID}, 100, "")
	if err != nil || len(checks.Items) != 1 {
		t.Fatal(checks, err)
	}
	receipt := checks.Items[0]
	if receipt.Outcome != "passed" || !receipt.SourceChanged || receipt.ResultingCandidateID == nil || receipt.InputCandidateBinding == nil || *receipt.InputCandidateBinding != *f.subject.Binding || receipt.AttemptID != f.check.ID || receipt.StdoutArtifactID == nil {
		t.Fatal("The observed mutation lost its exact input/result receipt", receipt)
	}
	w, err := e.db.Workflow(ctx, f.check.TaskID, domain.WorkflowSelector{WorkflowID: f.check.WorkflowID})
	if err != nil || w.Workflow == nil || w.Workflow.SelectedCandidateID == nil || *w.Workflow.SelectedCandidateID != *receipt.ResultingCandidateID || *w.Workflow.SelectedCandidateID == prior.ID || w.Workflow.State == "completed" || w.Acceptance != nil {
		t.Fatal("A changed check subject was accepted or its new candidate was not selected", w, err)
	}
	selected, err := e.selectedWorkflowSubject(ctx, *w.Workflow)
	if err != nil || selected.Candidate == nil || selected.Binding == nil || !selected.Candidate.HasDelta || selected.Candidate.HasIndexDelta || selected.Candidate.AttemptID != f.check.ID || selected.Candidate.CreatedByStepID != f.check.StepID || selected.Candidate.PredecessorCandidateID == nil || *selected.Candidate.PredecessorCandidateID != prior.ID || selected.Candidate.RootSourceSnapshotID != prior.RootSourceSnapshotID {
		t.Fatal("The mutating check invented a new root or lost selected candidate lineage", selected.Candidate, err)
	}
	_, retainedBytes := graphExternalAncestryReadFile(t, e, selected.Manifest, fixture.WorkflowFile)
	if string(retainedBytes) != fixture.WorkflowInvalid {
		t.Fatal("The exact post-command candidate did not retain changed file bytes")
	}
	dispatch, err := e.db.GraphDispatchForAttempt(ctx, f.check.ID)
	if err != nil || dispatch.Public.OutputObservation == nil || dispatch.Public.OutputObservation.Candidate == nil || dispatch.Public.OutputObservation.Candidate.CandidateID != selected.Candidate.ID || dispatch.Public.OutputObservation.Candidate.State != "complete" || dispatch.CandidateClosure == nil || dispatch.FinalVerification == nil {
		t.Fatal("The mutating command did not atomically retain its actual graph output", dispatch, err)
	}
	fresh, err := e.loadGraphFinalVerification(ctx, selected)
	if err != nil || fresh == nil || fresh.BindingRef == f.verified.BindingRef || fresh.Binding.FinalCandidateBinding != *selected.Binding {
		t.Fatal("The new check output reused the pre-command cumulative final", err)
	}
	scope := domain.CheckScope{Kind: receipt.Applicability.ScopeKind, Paths: receipt.Applicability.Paths}
	newBasis, err := e.workflowApplicability(ctx, selected, scope, receipt.Applicability.ConditionKeys, receipt.DefinitionSHA256, fresh)
	if err != nil || workflowpolicy.Applicable(receipt.Applicability, newBasis) {
		t.Fatal("A mutating check receipt qualified a different after-state", receipt.Applicability, newBasis, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, f.check.TaskID, *w.Workflow)
	if err != nil || progress.decision.Role == "accept" || progress.decision.Role == "local_delivery" {
		t.Fatal("Mutation skipped qualification of the new subject", progress.decision, err)
	}
	unchanged, err := e.db.Candidate(ctx, prior.ID)
	if err != nil || !reflect.DeepEqual(unchanged, prior) {
		t.Fatal("The command rewrote the pre-check candidate history", unchanged, err)
	}
	rootAfter, err := e.db.GraphRoot(ctx, f.dispatch.GraphID)
	if err != nil || !reflect.DeepEqual(rootBefore, rootAfter) {
		t.Fatal("The mutating command recaptured the graph root", err)
	}
	budget, err := e.db.GraphBudget(ctx, f.dispatch.GraphID)
	if err != nil || !reflect.DeepEqual(budget.Budget, f.budget.Budget) {
		t.Fatal("The local mutation consumed or refilled model allowance", budget, err)
	}
	graphFinalCheckUserState(t, f)
	if _, err = os.Lstat(filepath.Join(f.repo, fixture.WorkflowFile)); !os.IsNotExist(err) {
		t.Fatal("A command-only mutation leaked into the user's original checkout", err)
	}
}

func graphActualCorrectionCommandFixture(t *testing.T) graphFinalCheckCase {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	timeout := 10
	checks := []domain.CheckDeclaration{{Key: "controlled-final-context", Executable: executable, Arguments: []string{graphFinalContextCheckTestArg, "--outcome", "fail"}, RelativeWorkingDirectory: ".", TimeoutSeconds: &timeout}}
	options := graphFinalVerificationOptions{
		FinalWorkflow: &domain.WorkflowInput{Review: "required", Checks: &checks},
		FinalOriginal: "Inspect both inherited controlled A/B files. When the observed required check fails, correct only council-workflow.txt from the exact INVALID marker to the exact VALID marker: " + fixture.WorkflowValid + " Preserve source.txt, both inherited files and the user's staged index. Retain the failed check and all prior evidence; command failure after correction is still a failed check.",
		BeforeGraph: func(t *testing.T, repo string) {
			if err := os.WriteFile(filepath.Join(repo, fixture.WorkflowFile), []byte(fixture.WorkflowInvalid), 0600); err != nil {
				t.Fatal(err)
			}
		},
	}
	f := graphFinalReviewAttemptWithOptionsFixture(t, options)
	e, ctx := f.engine, context.Background()
	e.run(ctx, f.reviewer)
	reviewed, err := e.db.Attempt(ctx, f.reviewer.ID)
	if err != nil || reviewed.Outcome == nil || *reviewed.Outcome != "succeeded" || reviewed.EvidenceStatus != "complete" {
		t.Fatal("The actual scoped correction fixture did not complete its required review", reviewed, err)
	}
	advanced, _ := graphCommandAdvanceStage(t, e, f.dispatch.GraphID)
	d := advanced.Dispatches[0].Dispatch
	if d.Role != "check" || d.ExecutorKind != "local_check" || d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != f.verified.BindingRef {
		t.Fatal("The actual failed-check prerequisite lost its cumulative subject", d)
	}
	check, err := e.db.ClaimAttempt(ctx, d.AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	w, err := e.db.Workflow(ctx, check.TaskID, domain.WorkflowSelector{WorkflowID: check.WorkflowID})
	if err != nil || w.Workflow == nil || len(w.Workflow.Authority.Checks) != 1 {
		t.Fatal(w, err)
	}
	return graphFinalCheckCase{f, check, d, w.Workflow.Authority.Checks[0], advanced.Budget}
}

func TestGraphActualFailedCheckCorrectionExecutesSelectedHandoffWithoutRefillOrReplay(t *testing.T) {
	f := graphActualCorrectionCommandFixture(t)
	e, ctx := f.engine, context.Background()
	t.Setenv("COUNCIL_TEST_FINAL_CHECK_SENTINEL", "preserved")
	e.run(ctx, f.check)
	failed, err := e.db.Attempt(ctx, f.check.ID)
	checks, checkErr := e.db.WorkflowChecks(ctx, f.check.TaskID, domain.WorkflowSelector{WorkflowID: f.check.WorkflowID}, 100, "")
	if err != nil || checkErr != nil || failed.Outcome == nil || *failed.Outcome != "failed" || failed.StartedAt == nil || failed.EvidenceStatus != "complete" || failed.Cleanup.State != "verified" || len(checks.Items) != 1 || checks.Items[0].Outcome != "failed" || checks.Items[0].SourceChanged || checks.Items[0].Finding == nil || checks.Items[0].Finding.CheckID != checks.Items[0].ID {
		t.Fatal("Correction lacks a genuine complete failed check and its retained finding", failed, checks, err, checkErr)
	}
	receipt := checks.Items[0]
	rootBefore, err := e.db.GraphRoot(ctx, f.dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.db.Graph(ctx, f.dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	grant := domain.GraphBudgetAmendmentInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: view.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, AdditionalAdmissions: "1"}
	body, err := json.Marshal(grant)
	if err != nil {
		t.Fatal(err)
	}
	if result := e.MutateGraph(ctx, "graphs.budget.amend", view.Graph.ID, body); result.Status != 200 || result.Envelope.Error != nil {
		t.Fatal(result)
	}
	e.continueWorkflow(f.check.ID)
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 0 {
		t.Fatal("A grant replayed completion or started correction without fresh intent", pending, err)
	}
	admitted, before := graphCommandAdvanceStage(t, e, view.Graph.ID)
	d := admitted.Dispatches[0].Dispatch
	if d.Role != "correction" || d.ExecutorKind != "native" || d.WorkflowID != f.check.WorkflowID || d.NodeAdmissionID != f.dispatch.NodeAdmissionID || d.InputID != f.dispatch.InputID || d.SourceBasis.CandidateBinding == nil || *d.SourceBasis.CandidateBinding != *f.subject.Binding || d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != f.verified.BindingRef {
		t.Fatal("The explicit correction changed its exact selected lineage/final input", d)
	}
	correction, err := e.db.ClaimAttempt(ctx, d.AttemptID, e.config.OwnerInstanceID)
	if err != nil || correction.PreviousAttemptID == nil || *correction.PreviousAttemptID != f.check.ID {
		t.Fatal(correction, err)
	}
	var spec process.NativeSpec
	prepares := 0
	e.config.Prepare = func(ctx context.Context, actual process.NativeSpec) (*process.Owned, error) {
		prepares++
		spec = actual
		spec.Stdin = append([]byte{}, actual.Stdin...)
		return process.Prepare(ctx, actual)
	}
	e.run(ctx, correction)
	actual, err := e.db.Attempt(ctx, correction.ID)
	if err != nil || prepares != 1 || actual.Phase != "settled" || actual.StartedAt == nil || actual.Outcome == nil || *actual.Outcome != "succeeded" || actual.EvidenceStatus != "complete" || actual.Cleanup.State != "verified" || actual.CandidateID == nil {
		t.Fatal("The actual fixed correction worker did not execute and retain its result", actual, prepares, err)
	}
	line := bytes.IndexByte(spec.Stdin, '\n')
	var prompt struct {
		Original string                 `json:"originalInstructions"`
		Role     string                 `json:"role"`
		Findings []domain.ReviewFinding `json:"findings"`
		Receipts []domain.CheckReceipt  `json:"checkReceipts"`
		Final    *struct {
			ReceivingAttemptID   string                  `json:"receivingAttemptId"`
			FinalVerificationRef domain.GraphArtifactRef `json:"finalVerificationRef"`
		} `json:"graphFinalVerification"`
	}
	if line < 0 {
		t.Fatal("The correction prompt has no expected complete JSON boundary")
	}
	decodeErr := json.Unmarshal(spec.Stdin[line+1:], &prompt)
	detail, detailErr := e.db.Task(ctx, correction.TaskID)
	if detailErr != nil {
		t.Fatal(detailErr)
	}
	exactOriginal := prompt.Original == detail.Task.OriginalText
	exactFinding, exactReceipt := false, false
	for _, finding := range prompt.Findings {
		exactFinding = exactFinding || reflect.DeepEqual(finding, receipt.Finding.Finding)
	}
	for _, check := range prompt.Receipts {
		exactReceipt = exactReceipt || reflect.DeepEqual(check, receipt)
	}
	exactFinal := prompt.Final != nil && prompt.Final.ReceivingAttemptID == correction.ID && prompt.Final.FinalVerificationRef == f.verified.BindingRef
	hasRawCheck := bytes.Contains(spec.Stdin, []byte("check-"+receipt.ID+"-stdout"))
	if decodeErr != nil || prompt.Role != "correction" || !exactOriginal || !exactFinding || !exactReceipt || !exactFinal || !hasRawCheck {
		t.Fatalf("The correction input mismatched retained evidence: decode=%v role=%s originalExact=%t originalBytes=%d/%d findings=%d exactFinding=%t receipts=%d exactReceipt=%t finalExact=%t actualFinal=%+v expectedReceiver=%s expectedFinal=%+v rawCheck=%t checkId=%s", decodeErr, prompt.Role, exactOriginal, len(prompt.Original), len(detail.Task.OriginalText), len(prompt.Findings), exactFinding, len(prompt.Receipts), exactReceipt, exactFinal, prompt.Final, correction.ID, f.verified.BindingRef, hasRawCheck, receipt.ID)
	}
	w, err := e.db.Workflow(ctx, correction.TaskID, domain.WorkflowSelector{WorkflowID: correction.WorkflowID})
	if err != nil || w.Workflow == nil || w.Workflow.SelectedCandidateID == nil || *w.Workflow.SelectedCandidateID != *actual.CandidateID || w.Workflow.State == "completed" || w.Acceptance != nil {
		t.Fatal("Correction process success became final acceptance without fresh required checks", w, err)
	}
	selected, err := e.selectedWorkflowSubject(ctx, *w.Workflow)
	if err != nil || selected.Candidate == nil || selected.Candidate.PredecessorCandidateID == nil || *selected.Candidate.PredecessorCandidateID != f.subject.Candidate.ID || selected.Candidate.RootSourceSnapshotID != f.subject.Candidate.RootSourceSnapshotID {
		t.Fatal("The actual corrective output lost its candidate ancestry", selected.Candidate, err)
	}
	_, output := graphExternalAncestryReadFile(t, e, selected.Manifest, fixture.WorkflowFile)
	if string(output) != fixture.WorkflowValid {
		t.Fatal("The native corrector did not read INVALID and write exact controlled VALID bytes")
	}
	for _, name := range []string{fixture.GraphFileA, fixture.GraphFileB, "source.txt"} {
		_, original := graphExternalAncestryReadFile(t, e, f.subject.Manifest, name)
		_, after := graphExternalAncestryReadFile(t, e, selected.Manifest, name)
		if !bytes.Equal(original, after) {
			t.Fatal("The correction changed an unrelated inherited/source file", name)
		}
	}
	registered, err := e.db.GraphDispatchForAttempt(ctx, correction.ID)
	if err != nil || registered.Public.Materialization == nil || registered.Public.Materialization.State != "complete" || registered.Public.Materialization.SourceBasis.BasisKind != "node_subject" || registered.Public.Materialization.SourceBasis.CandidateBinding == nil || *registered.Public.Materialization.SourceBasis.CandidateBinding != *f.subject.Binding || registered.Public.OutputObservation == nil || registered.Public.OutputObservation.Candidate == nil || registered.Public.OutputObservation.Candidate.CandidateID != selected.Candidate.ID || registered.CandidateClosure == nil || registered.FinalVerification == nil {
		t.Fatal("The actual correction lost selected Handoff or same-terminal output/final association", registered, err)
	}
	old, err := e.db.WorkflowChecks(ctx, f.check.TaskID, domain.WorkflowSelector{WorkflowID: f.check.WorkflowID}, 100, "")
	if err != nil || len(old.Items) != 1 || !reflect.DeepEqual(old.Items[0], receipt) {
		t.Fatal("Correction rewrote the actual failed check receipt", old, err)
	}
	retainedFailure, err := e.db.Attempt(ctx, f.check.ID)
	if err != nil {
		t.Fatal(err)
	}
	retainedFailure.ObservedAt, retainedFailure.RemainingSeconds = failed.ObservedAt, failed.RemainingSeconds
	if !reflect.DeepEqual(retainedFailure, failed) {
		t.Fatal("Correction rewrote the failed command attempt history", retainedFailure, failed)
	}
	rootAfter, err := e.db.GraphRoot(ctx, f.dispatch.GraphID)
	if err != nil || !reflect.DeepEqual(rootBefore, rootAfter) {
		t.Fatal("Correction recaptured the immutable graph root", err)
	}
	budget, err := e.db.GraphBudget(ctx, f.dispatch.GraphID)
	units, invalid := domain.SubtractRoutingCounts(budget.Budget.ConsumedAdmissions, before.Budget.Budget.ConsumedAdmissions)
	if err != nil || invalid != nil || units != "1" || budget.Budget.RemainingAdmissions != "0" || budget.Budget.GrantedAdmissions != admitted.Budget.Budget.GrantedAdmissions || budget.BudgetAuthorityEpoch != admitted.Budget.BudgetAuthorityEpoch || !reflect.DeepEqual(budget.Budget.CurrentWindow, before.Budget.Budget.CurrentWindow) {
		t.Fatal("The correction refilled/reset or double-debited shared authority", budget, units, err, invalid)
	}
	graphFinalCheckUserState(t, f)
	if body, err := os.ReadFile(filepath.Join(f.repo, fixture.WorkflowFile)); err != nil || string(body) != fixture.WorkflowInvalid {
		t.Fatal("Correction changed the user's original fixed source instead of its isolated candidate", err)
	}
}
