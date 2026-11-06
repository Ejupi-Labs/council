package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type workflowCorrectionObservation struct {
	Review domain.ReviewRecord
	Check  domain.CheckReceipt
	Bytes  []byte
	Prompt struct {
		Role             string                 `json:"role"`
		Subject          domain.ReviewSubject   `json:"subject"`
		Findings         []domain.ReviewFinding `json:"findings"`
		PriorFindingIDs  []string               `json:"priorFindingIds"`
		ConditionsSHA256 string                 `json:"conditionsSHA256"`
		CheckReceipts    []domain.CheckReceipt  `json:"checkReceipts"`
		Evidence         []struct {
			Ref   string   `json:"ref"`
			Path  string   `json:"path"`
			Scope string   `json:"scope"`
			Paths []string `json:"paths"`
		} `json:"evidence"`
	}
	Err error
}

func workflowReviewInput(t *testing.T, project string) domain.StartTaskInput {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	timeout := 10
	checks := []domain.CheckDeclaration{{Key: "controlled-validation", Executable: executable, Arguments: []string{"__fixture", "check", "--case", "workflow-validation"}, RelativeWorkingDirectory: ".", TimeoutSeconds: &timeout}}
	return domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Produce council-workflow.txt containing the controlled VALID fixture, and preserve unrelated project files.", ExecutionMode: "fake", FakeScenario: "workflow-defect", Workflow: &domain.WorkflowInput{Review: "required", Checks: &checks, Delivery: &domain.WorkflowDeliveryInput{LocalMode: "review_only"}}}
}

// This observes the real correction preparation without replacing its owned
// process. It captures failures BEFORE correction can run, so later comparisons
// prove those immutable records were retained rather than rewritten into passes.
func observeWorkflowCorrection(e *Engine, taskID *string, ready <-chan struct{}) <-chan workflowCorrectionObservation {
	observed := make(chan workflowCorrectionObservation, 8)
	e.prepareGate.Lock()
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		correction := false
		for index := 0; index+1 < len(spec.Args); index++ {
			correction = correction || spec.Args[index] == "--role" && spec.Args[index+1] == "correction"
		}
		if correction {
			select {
			case <-ready:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			capture := workflowCorrectionObservation{}
			reviews, reviewErr := e.db.WorkflowReviews(ctx, *taskID, domain.WorkflowSelector{}, 100, "")
			checks, checkErr := e.db.WorkflowChecks(ctx, *taskID, domain.WorkflowSelector{}, 100, "")
			if reviewErr != nil || checkErr != nil || len(reviews.Items) != 1 || len(checks.Items) != 1 {
				capture.Err = fmt.Errorf("correction preparation lacks one real prior review/check: reviews=%+v err=%v checks=%+v err=%v", reviews, reviewErr, checks, checkErr)
			} else {
				capture.Review, capture.Check = reviews.Items[0], checks.Items[0]
				capture.Bytes, capture.Err = os.ReadFile(filepath.Join(spec.Cwd, fixture.WorkflowFile))
				start := bytes.IndexByte(spec.Stdin, '{')
				if capture.Err == nil {
					if start < 0 {
						capture.Err = fmt.Errorf("correction received no structured original/predecessor/finding context")
					} else {
						capture.Err = json.Unmarshal(spec.Stdin[start:], &capture.Prompt)
					}
				}
			}
			observed <- capture
		}
		return process.Prepare(ctx, spec)
	}
	e.prepareGate.Unlock()
	return observed
}

func workflowReviewDiagnostics(t *testing.T, e *Engine, taskID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	view, viewErr := e.db.Workflow(ctx, taskID, domain.WorkflowSelector{})
	steps, stepsErr := e.db.WorkflowSteps(ctx, taskID, domain.WorkflowSelector{}, 100, "")
	gate, gateErr := e.db.Gate(ctx)
	budget, budgetErr := e.db.TaskBudget(ctx, taskID)
	t.Logf("workflow=%+v viewErr=%v gate=%+v gateErr=%v budget=%+v budgetErr=%v steps=%+v stepsErr=%v", view, viewErr, gate, gateErr, budget, budgetErr, steps, stepsErr)
	for _, step := range steps.Items {
		if step.AttemptID != nil {
			attempt, err := e.db.Attempt(ctx, *step.AttemptID)
			t.Logf("step=%s role=%s status=%s attempt=%+v error=%v primaryFailure=%+v", step.ID, step.Role, step.Status, attempt, err, attempt.PrimaryFailure)
		}
	}
	reviews, reviewErr := e.db.WorkflowReviews(ctx, taskID, domain.WorkflowSelector{}, 100, "")
	checks, checkErr := e.db.WorkflowChecks(ctx, taskID, domain.WorkflowSelector{}, 100, "")
	t.Logf("reviews=%+v err=%v checks=%+v err=%v", reviews, reviewErr, checks, checkErr)
	if view.Workflow != nil {
		progress, err := e.inspectWorkflowProgress(ctx, taskID, *view.Workflow)
		t.Logf("current decision=%+v progressErr=%v", progress.decision, err)
	}
}

func waitWorkflowReviewState(t *testing.T, e *Engine, taskID string, predicate func(domain.WorkflowDetail, []domain.WorkflowStep) bool) domain.WorkflowDetail {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		view, err := e.db.Workflow(ctx, taskID, domain.WorkflowSelector{})
		if err != nil {
			workflowReviewDiagnostics(t, e, taskID)
			t.Fatal(err)
		}
		steps, err := e.db.WorkflowSteps(ctx, taskID, domain.WorkflowSelector{}, 100, "")
		if err != nil {
			workflowReviewDiagnostics(t, e, taskID)
			t.Fatal(err)
		}
		if predicate(view, steps.Items) {
			return view
		}
		if view.Workflow != nil && view.Workflow.CurrentStepID != nil {
			for _, step := range steps.Items {
				if step.ID != *view.Workflow.CurrentStepID || step.AttemptID == nil {
					continue
				}
				attempt, attemptErr := e.db.Attempt(ctx, *step.AttemptID)
				if attemptErr != nil {
					workflowReviewDiagnostics(t, e, taskID)
					t.Fatal(attemptErr)
				}
				if attempt.Phase == "settled" && (attempt.PrimaryFailure != nil || attempt.Stop != nil || attempt.Cleanup.State != "verified" || attempt.EvidenceStatus != "complete" || view.Workflow.NextOwner == "user") {
					workflowReviewDiagnostics(t, e, taskID)
					t.Fatalf("real owned workflow reached an unexpected terminal block: role=%s phase=%s outcome=%v primaryFailure=%+v nextAction=%s", step.Role, attempt.Phase, attempt.Outcome, attempt.PrimaryFailure, view.Workflow.NextAction)
				}
			}
		}
		gate, err := e.db.Gate(ctx)
		if err != nil || gate.State == "fenced" {
			workflowReviewDiagnostics(t, e, taskID)
			t.Fatalf("real owned review workflow fenced: %+v / %v", gate, err)
		}
		select {
		case <-ctx.Done():
			workflowReviewDiagnostics(t, e, taskID)
			t.Fatal("real owned review workflow did not reach its bounded expected state", ctx.Err())
		case <-ticker.C:
		}
	}
}

func workflowCandidateFile(t *testing.T, e *Engine, candidateID, relative string) []byte {
	t.Helper()
	ctx := context.Background()
	candidate, err := e.db.Candidate(ctx, candidateID)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, err := e.workflowManifest(ctx, candidate.ManifestArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Entries {
		if entry.Path == relative {
			var content bytes.Buffer
			meta, err := e.evidence.CopyTo(ctx, entry.ArtifactID, &content, workspace.MaxFileBytes)
			if err != nil || meta.SHA256 == nil || *meta.SHA256 != entry.SHA256 {
				t.Fatal("selected candidate file lacks complete retained byte proof", meta, err)
			}
			return content.Bytes()
		}
	}
	t.Fatal("selected candidate omitted required actual file", relative)
	return nil
}

func TestWorkflowReviewRequiredActualFailureCorrectionRecheckAndFreshReview(t *testing.T) {
	e, projectID, projectPath := testEngine(t)
	var taskID string
	ready := make(chan struct{})
	observed := observeWorkflowCorrection(e, &taskID, ready)
	result := e.StartTask(context.Background(), workflowReviewInput(t, projectID))
	if result.Status != 201 {
		close(ready)
		t.Fatal(result)
	}
	admission := decodeAdmission(t, result)
	taskID = admission.Task.ID
	close(ready)
	if admission.Workflow == nil || admission.Workflow.Authority.Correction.MaxCycles != 1 || len(admission.Workflow.Authority.Checks) != 1 {
		t.Fatal("real workflow did not capture its one finite correction and explicit verifier", admission)
	}
	current := waitWorkflowReviewState(t, e, taskID, func(view domain.WorkflowDetail, _ []domain.WorkflowStep) bool {
		return view.Workflow != nil && view.Workflow.State == "completed"
	})
	if current.Acceptance == nil || current.Acceptance.DeliveryResult != "review_only" || current.Acceptance.DeliveryOperationID != nil || current.Workflow.CorrectionCyclesUsed != 1 || current.Workflow.SelectedCandidateID == nil {
		t.Fatal("workflow fabricated application or omitted its finite verified completion", current)
	}
	var prior workflowCorrectionObservation
	select {
	case prior = <-observed:
	default:
		t.Fatal("no actual correction preparation was observed")
	}
	if prior.Err != nil || string(prior.Bytes) != fixture.WorkflowInvalid || prior.Prompt.Role != "correction" {
		t.Fatal("correction did not receive actual retained predecessor bytes", prior, prior.Err)
	}
	if len(observed) != 0 {
		t.Fatal("workflow prepared more than one correction")
	}
	if len(prior.Prompt.CheckReceipts) != 1 || !reflect.DeepEqual(prior.Prompt.CheckReceipts[0], prior.Check) {
		t.Fatal("actual correction prompt omitted or changed the full failed check receipt", prior.Prompt.CheckReceipts, prior.Check)
	}
	if prior.Check.StdoutArtifactID == nil || prior.Check.StderrArtifactID == nil {
		t.Fatal("actual failed receipt omitted raw stream identities", prior.Check)
	}
	for _, expected := range []struct{ ref, artifactID, text string }{
		{"check-" + prior.Check.ID + "-stdout", *prior.Check.StdoutArtifactID, "Council fixed local verifier (005-v1).\n"},
		{"check-" + prior.Check.ID + "-stderr", *prior.Check.StderrArtifactID, "Observed controlled validation defect.\n"},
	} {
		found := false
		for _, supplied := range prior.Prompt.Evidence {
			if supplied.Ref != expected.ref {
				continue
			}
			data, err := os.ReadFile(supplied.Path)
			metadata, verifyErr := e.evidence.VerifyClosed(context.Background(), expected.artifactID)
			if err != nil || verifyErr != nil || supplied.Scope != "whole_candidate" || len(supplied.Paths) != 0 || metadata.Kind != strings.TrimPrefix(expected.ref, "check-"+prior.Check.ID+"-") || string(data) != expected.text {
				t.Fatal("actual correction did not receive readable exact failed check output", supplied, string(data), err, verifyErr)
			}
			found = true
		}
		if !found {
			t.Fatal("actual correction prompt omitted prior raw check access", expected.ref)
		}
	}
	ctx := context.Background()
	steps, err := e.db.WorkflowSteps(ctx, taskID, domain.WorkflowSelector{}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	wantRoles := []string{"author", "review", "check", "correction", "check", "review"}
	if len(steps.Items) != len(wantRoles) {
		workflowReviewDiagnostics(t, e, taskID)
		t.Fatal("required actual six-stage lineage differs", steps)
	}
	nativeIDs := map[string]bool{}
	for index, step := range steps.Items {
		if step.Role != wantRoles[index] || step.Cycle != []int{0, 0, 0, 1, 1, 1}[index] || step.AttemptID == nil || index > 0 && (step.PredecessorStepID == nil || *step.PredecessorStepID != steps.Items[index-1].ID) {
			t.Fatal("stage order/predecessor was skipped or recreated", steps)
		}
		attempt, err := e.db.Attempt(ctx, *step.AttemptID)
		if err != nil || attempt.Phase != "settled" || attempt.Cleanup.State != "verified" || attempt.EvidenceStatus != "complete" || attempt.StepID != step.ID || attempt.WorkflowID != current.Workflow.ID {
			t.Fatal("stage lacks final actual ownership/evidence proof", attempt, err)
		}
		wantOutcome := "succeeded"
		if index == 2 {
			wantOutcome = "failed"
		}
		if attempt.Outcome == nil || *attempt.Outcome != wantOutcome || attempt.PrimaryFailure != nil || attempt.Stop != nil {
			t.Fatalf("stage %d lost actual process outcome: %+v failure=%+v", index, attempt, attempt.PrimaryFailure)
		}
		if step.Role == "check" {
			if step.ExecutorKind != "local_check" || attempt.ExecutorKind != "local_check" {
				t.Fatal("required fixed verifier was not its separate zero-model local attempt", step, attempt)
			}
		} else {
			if step.ExecutorKind != "native" || attempt.ExecutionMode != "fake" {
				t.Fatal("controlled model stage did not use its owned fixed native fixture", step, attempt)
			}
			nativeIDs[attempt.ID] = true
		}
		observations := attempt.TargetObservations
		if observations.Provider.Status != "unknown" || observations.Account.Status != "unknown" || observations.Model.Status != "unknown" || observations.NativeSession.Status != "unknown" || observations.Usage.InputTokens != nil || observations.Usage.OutputTokens != nil || observations.Usage.CachedInputTokens != nil || observations.NativeModels != nil {
			t.Fatal("fixed workflow invented live vendor/account/model/token facts", attempt)
		}
	}
	if len(nativeIDs) != 4 {
		t.Fatal("workflow did not use exactly four independently owned native-role attempts", nativeIDs)
	}
	reviews, err := e.db.WorkflowReviews(ctx, taskID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(reviews.Items) != 2 || reviews.Items[0].Outcome != "changes_required" || reviews.Items[1].Outcome != "approved" || reviews.Items[0].ID == reviews.Items[1].ID || reviews.Items[0].AttemptID == reviews.Items[1].AttemptID || len(reviews.Items[0].Findings) != 1 {
		t.Fatal("fresh re-review did not preserve the original blocker", reviews, err)
	}
	checks, err := e.db.WorkflowChecks(ctx, taskID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(checks.Items) != 2 {
		t.Fatal("required failed and affected passing checks were not both retained", checks, err)
	}
	for index, check := range checks.Items {
		if check.Outcome != []string{"failed", "passed"}[index] || check.ExitCode == nil || *check.ExitCode != []int{1, 0}[index] || !check.RootTerminal || !check.InputComplete || !check.OutputComplete || check.Cleanup.State != "verified" || check.BuiltinCompleted || check.SourceChanged || check.ObservedExecutable == nil || !reflect.DeepEqual(check.ObservedExecutable.Arguments, []string{"__fixture", "check", "--case", "workflow-validation"}) || check.StdoutArtifactID == nil || check.StderrArtifactID == nil {
			t.Fatal("check did not retain its actual nonaccepted/accepted native command observation", check)
		}
		stdout, verifyErr := e.evidence.VerifyClosed(ctx, *check.StdoutArtifactID)
		if verifyErr != nil {
			t.Fatal(verifyErr)
		}
		text, readErr := e.readResponse(ctx, stdout)
		if readErr != nil || string(text) != "Council fixed local verifier (005-v1).\n" {
			t.Fatal("check stdout was replaced by fabricated protocol/test evidence", string(text), readErr)
		}
	}
	if len(current.Acceptance.ReviewUses) != 1 || len(current.Acceptance.CheckUses) != 1 || current.Acceptance.ReviewUses[0].EvidenceID != reviews.Items[1].ID || current.Acceptance.CheckUses[0].EvidenceID != checks.Items[1].ID {
		t.Fatal("automatic acceptance used stale failed evidence instead of the fresh current checks/review", current.Acceptance)
	}
	if !reflect.DeepEqual(prior.Review, reviews.Items[0]) || !reflect.DeepEqual(prior.Check, checks.Items[0]) {
		t.Fatal("correction/re-review rewrote earlier immutable failed evidence", prior.Review, reviews.Items[0], prior.Check, checks.Items[0])
	}
	initialCandidate := reviews.Items[0].CandidateBinding
	if initialCandidate == nil || checks.Items[0].InputCandidateBinding == nil || *initialCandidate != *checks.Items[0].InputCandidateBinding || prior.Prompt.Subject.ID != initialCandidate.CandidateID || prior.Prompt.Subject.SHA256 != initialCandidate.ManifestSHA256 || prior.Prompt.ConditionsSHA256 != current.Workflow.Authority.ConditionsSHA256 {
		t.Fatal("initial review/check/correction did not share the exact actual predecessor", reviews, checks, prior.Prompt)
	}
	if prior.Check.Finding == nil || !slices.Contains(prior.Prompt.PriorFindingIDs, prior.Check.Finding.ID) || !slices.Contains(prior.Prompt.PriorFindingIDs, prior.Review.Findings[0].ID) || len(prior.Prompt.Findings) != 2 {
		t.Fatal("correction omitted the actual registered review/check findings", prior)
	}
	selectedID := *current.Workflow.SelectedCandidateID
	selected, err := e.db.Candidate(ctx, selectedID)
	if err != nil || selected.PredecessorCandidateID == nil || *selected.PredecessorCandidateID != initialCandidate.CandidateID || selected.CreatedByStepID != steps.Items[3].ID || selected.RootSourceSnapshotID != initialCandidate.RootSourceSnapshotID || selected.SourceSnapshotID == selected.RootSourceSnapshotID || reviews.Items[1].CandidateBinding == nil || checks.Items[1].InputCandidateBinding == nil || reviews.Items[1].CandidateBinding.CandidateID != selectedID || checks.Items[1].InputCandidateBinding.CandidateID != selectedID || current.Acceptance.CandidateBinding.CandidateID != selectedID {
		t.Fatal("final review/check/acceptance lost cumulative correction lineage", selected, current, err)
	}
	if string(workflowCandidateFile(t, e, selectedID, fixture.WorkflowFile)) != fixture.WorkflowValid || string(workflowCandidateFile(t, e, initialCandidate.CandidateID, fixture.WorkflowFile)) != fixture.WorkflowInvalid {
		t.Fatal("actual INVALID predecessor or cumulative VALID selected bytes were lost")
	}
	findings, err := e.db.WorkflowFindings(ctx, taskID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(findings.Items) != 2 || len(current.Acceptance.FindingResolutionIDs) != 2 {
		t.Fatal("both actual blocker resolutions are required", findings, current.Acceptance, err)
	}
	for _, finding := range findings.Items {
		if len(finding.Resolutions) != 1 || finding.Resolutions[0].CorrectedCandidateID == nil || *finding.Resolutions[0].CorrectedCandidateID != selectedID || !slices.Contains(current.Acceptance.FindingResolutionIDs, finding.Resolutions[0].ID) {
			t.Fatal("finding was given a generic or stale resolution", finding)
		}
	}
	budget, err := e.db.TaskBudget(ctx, taskID)
	if err != nil || budget.Budget == nil || budget.Budget.GrantedAdmissions != "4" || budget.Budget.ConsumedAdmissions != "4" || budget.Budget.RemainingAdmissions != "0" || budget.Budget.CurrentWindow.Number != "1" {
		t.Fatal("four native admissions or common finite window were not preserved", budget, err)
	}
	ledger, err := e.db.BudgetLedger(ctx, taskID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	debits := map[string]bool{}
	for _, entry := range ledger.Items {
		if entry.ConsumedDelta == "0" {
			continue
		}
		if entry.ConsumedDelta != "1" || entry.AttemptID == nil || !nativeIDs[*entry.AttemptID] || debits[*entry.AttemptID] || entry.Window.Number != "1" {
			t.Fatal("local work charged a model unit or prior admission/window was rewritten", entry)
		}
		debits[*entry.AttemptID] = true
	}
	if len(debits) != 4 {
		t.Fatal("four actual native admissions lack exact ledger debits", ledger)
	}
	if _, err = os.Lstat(filepath.Join(projectPath, fixture.WorkflowFile)); !os.IsNotExist(err) {
		t.Fatal("review-only workflow changed the original project", err)
	}
	if source, err := os.ReadFile(filepath.Join(projectPath, "source.txt")); err != nil || string(source) != "original\n" {
		t.Fatal("unrelated original source changed", err)
	}
}

func TestWorkflowReviewSavedTwoAdmissionBudgetExhaustsBeforeCorrection(t *testing.T) {
	e, projectID, projectPath := testEngine(t)
	ctx := context.Background()
	policy, err := e.db.RoutingPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce a retained finite two-unit default without schema surgery. Normal
	// task admission must honor it instead of silently granting four for Auto.
	body, err := json.Marshal(domain.RoutingPolicyInput{RequestID: domain.NewID(), ExpectedRevision: policy.Revision, DefaultBudget: &domain.RoutingBudgetInput{AdmissionLimit: "2"}})
	if err != nil {
		t.Fatal(err)
	}
	if saved := e.MutateRouting(ctx, "routing.policy.update", "", body); saved.Envelope.Error != nil {
		t.Fatal(saved)
	}
	admitted := e.StartTask(ctx, workflowReviewInput(t, projectID))
	if admitted.Status != 201 {
		t.Fatal(admitted)
	}
	a := decodeAdmission(t, admitted)
	view := waitWorkflowReviewState(t, e, a.Task.ID, func(view domain.WorkflowDetail, steps []domain.WorkflowStep) bool {
		if view.Workflow == nil || len(steps) != 3 || steps[2].Role != "check" || steps[2].Status != "failed" {
			return false
		}
		gate, gateErr := e.db.Gate(context.Background())
		return gateErr == nil && gate.ActiveAttemptID == nil
	})
	budget, err := e.db.TaskBudget(ctx, a.Task.ID)
	if err != nil || budget.Budget == nil || budget.Budget.GrantedAdmissions != "2" || budget.Budget.ConsumedAdmissions != "2" || budget.Budget.RemainingAdmissions != "0" || view.Acceptance != nil || view.Workflow.State == "completed" || view.Workflow.CorrectionCyclesUsed != 0 {
		t.Fatal("exhausted saved budget was expanded or failed evidence became success", budget, view, err)
	}
	steps, err := e.db.WorkflowSteps(ctx, a.Task.ID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(steps.Items) != 3 || steps.Items[0].Role != "author" || steps.Items[1].Role != "review" || steps.Items[2].Role != "check" {
		t.Fatal("exhausted budget created correction or skipped actual verification", steps, err)
	}
	checks, err := e.db.WorkflowChecks(ctx, a.Task.ID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(checks.Items) != 1 || checks.Items[0].ExitCode == nil || *checks.Items[0].ExitCode != 1 || checks.Items[0].Outcome != "failed" || checks.Items[0].Cleanup.State != "verified" {
		t.Fatal("exhaustion hid the actually observed failed verifier", checks, err)
	}
	if view.Workflow.SelectedCandidateID == nil || string(workflowCandidateFile(t, e, *view.Workflow.SelectedCandidateID, fixture.WorkflowFile)) != fixture.WorkflowInvalid {
		t.Fatal("exhaustion did not retain the useful INVALID candidate")
	}
	if _, err = os.Lstat(filepath.Join(projectPath, fixture.WorkflowFile)); !os.IsNotExist(err) {
		t.Fatal("budget exhaustion changed original project", err)
	}
}
