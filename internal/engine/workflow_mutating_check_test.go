package engine

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	"github.com/Ejupi-Labs/council/v2/internal/workflow"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

const mutatingCheckTestArg = "__council_workflow_mutating_check_test_v1"

// TestMain-only fixed child, not a product fixture/scenario/command. It accepts
// no path, executable or generated command and refuses unknown actual bytes.
func handleWorkflowMutatingCheck(args []string) (bool, int) {
	if len(args) == 0 || args[0] != mutatingCheckTestArg {
		return false, 0
	}
	if len(args) != 3 || args[1] != "--case" || args[2] != "source" && args[2] != "index" {
		return true, 2
	}
	cwd, err := os.Getwd()
	control, controlErr := os.Lstat(".git")
	if err != nil || controlErr != nil || !control.Mode().IsRegular() || filepath.Base(filepath.Dir(cwd)) != "workspaces" || !domain.ValidID(filepath.Base(cwd)) {
		return true, 2
	}
	actual, err := os.ReadFile(fixture.WorkflowFile)
	if err != nil || string(actual) != fixture.WorkflowInvalid && string(actual) != fixture.WorkflowValid {
		return true, 2
	}
	if args[2] == "source" {
		if string(actual) == fixture.WorkflowInvalid {
			if err = os.WriteFile(fixture.WorkflowFile, []byte(fixture.WorkflowValid), 0600); err != nil {
				return true, 3
			}
		}
		observed, err := os.ReadFile(fixture.WorkflowFile)
		if err != nil || string(observed) != fixture.WorkflowValid {
			return true, 3
		}
		fmt.Fprintln(os.Stdout, "Controlled owned check observed/made exact VALID source bytes.")
		return true, 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.autocrlf=false", "add", "--", fixture.WorkflowFile)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(strings.ToUpper(name), "GIT_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=0", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err = command.Run(); err != nil {
		return true, 3
	}
	unchanged, err := os.ReadFile(fixture.WorkflowFile)
	if err != nil || !bytes.Equal(actual, unchanged) {
		return true, 3
	}
	fmt.Fprintln(os.Stdout, "Controlled owned check staged the actual fixed path in the managed index.")
	return true, 0
}

type beforeMutatingCheck struct {
	Review domain.ReviewRecord
	Bytes  []byte
	Err    error
}

func TestWorkflowOwnedMutatingCheckRetainsNewSubjectAndInvalidatesOldEvidence(t *testing.T) {
	for _, mutation := range []string{"source", "index"} {
		t.Run(mutation, func(t *testing.T) {
			e, projectID, projectPath := testEngine(t)
			beforeIndex, err := os.ReadFile(filepath.Join(projectPath, ".git", "index"))
			if err != nil {
				t.Fatal(err)
			}
			input := workflowReviewInput(t, projectID)
			(*input.Workflow.Checks)[0].Arguments = []string{mutatingCheckTestArg, "--case", mutation}
			var taskID string
			ready := make(chan struct{})
			var readyOnce sync.Once
			releaseReady := func() { readyOnce.Do(func() { close(ready) }) }
			// Registered after testEngine cleanup, so failure releases this
			// fixture gate before exact Engine.Close waits for its workers.
			t.Cleanup(releaseReady)
			// Retain unexpected finite correction/recheck observations rather
			// than blocking a worker and masking its actual failed receipt.
			before := make(chan beforeMutatingCheck, 8)
			e.prepareGate.Lock()
			e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
				if len(spec.Args) == 3 && spec.Args[0] == mutatingCheckTestArg {
					select {
					case <-ready:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					capture := beforeMutatingCheck{}
					reviews, readErr := e.db.WorkflowReviews(ctx, taskID, domain.WorkflowSelector{}, 100, "")
					if readErr != nil || len(reviews.Items) != 1 {
						capture.Err = fmt.Errorf("controlled pre-check review unavailable: %+v / %v", reviews, readErr)
					} else {
						capture.Review = reviews.Items[0]
						capture.Bytes, capture.Err = os.ReadFile(filepath.Join(spec.Cwd, fixture.WorkflowFile))
					}
					select {
					case before <- capture:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				return process.Prepare(ctx, spec)
			}
			e.prepareGate.Unlock()
			result := e.StartTask(context.Background(), input)
			if result.Status != 201 {
				releaseReady()
				t.Fatal(result)
			}
			admission := decodeAdmission(t, result)
			taskID = admission.Task.ID
			releaseReady()
			view := waitWorkflowReviewState(t, e, taskID, func(view domain.WorkflowDetail, steps []domain.WorkflowStep) bool {
				return view.Workflow != nil && view.Workflow.NextAction == "candidate_stale" && view.Workflow.NextOwner == "user" && len(steps) == 3 && steps[2].SettledAt != nil
			})
			var captured beforeMutatingCheck
			select {
			case captured = <-before:
			default:
				t.Fatal("actual owned mutating check was not prepared")
			}
			if len(before) != 0 {
				t.Fatal("source/index-mutating check unexpectedly prepared another check or correction cycle", len(before))
			}
			if captured.Err != nil || string(captured.Bytes) != fixture.WorkflowInvalid || captured.Review.Outcome != "changes_required" || captured.Review.CandidateBinding == nil {
				t.Fatal("mutating check lacked the actual blocked selected predecessor", captured, captured.Err)
			}
			ctx := context.Background()
			checks, err := e.db.WorkflowChecks(ctx, taskID, domain.WorkflowSelector{}, 100, "")
			if err != nil || len(checks.Items) != 1 {
				t.Fatal("actual mutating command receipt missing", checks, err)
			}
			check := checks.Items[0]
			if !check.SourceChanged || check.ResultingCandidateID == nil || check.InputCandidateBinding == nil || *check.InputCandidateBinding != *captured.Review.CandidateBinding || check.Outcome != "passed" || check.ExitCode == nil || *check.ExitCode != 0 || !check.RootTerminal || !check.InputComplete || !check.OutputComplete || check.Cleanup.State != "verified" || check.BuiltinCompleted {
				t.Fatal("mutating check replaced old input binding or lost actual exit/cleanup/new candidate facts", check)
			}
			if view.Acceptance != nil || view.Workflow.Completion != nil || view.Workflow.State != "blocked" || view.Workflow.CorrectionCyclesUsed != 0 || view.Workflow.SelectedCandidateID == nil || *view.Workflow.SelectedCandidateID != *check.ResultingCandidateID || *view.Workflow.SelectedCandidateID == check.InputCandidateBinding.CandidateID {
				t.Fatal("mutating check reused old qualification, corrected automatically or accepted", view, check)
			}
			selected, err := e.selectedWorkflowSubject(ctx, *view.Workflow)
			if err != nil || selected.Candidate == nil || selected.Candidate.Completeness != "complete" || selected.Candidate.AttemptID != check.AttemptID || selected.Candidate.PredecessorCandidateID == nil || *selected.Candidate.PredecessorCandidateID != check.InputCandidateBinding.CandidateID || selected.Candidate.RootSourceSnapshotID != check.InputCandidateBinding.RootSourceSnapshotID {
				t.Fatal("actual new complete subject lost its producer/predecessor/root lineage", selected.Candidate, err)
			}
			old, err := e.candidateWorkflowSubject(ctx, *view.Workflow, check.InputCandidateBinding.CandidateID)
			if err != nil {
				t.Fatal(err)
			}
			if string(workflowCandidateFile(t, e, old.Candidate.ID, fixture.WorkflowFile)) != fixture.WorkflowInvalid {
				t.Fatal("old retained candidate was rewritten by a source-isolated check")
			}
			want := fixture.WorkflowValid
			if mutation == "index" {
				want = fixture.WorkflowInvalid
			}
			if string(workflowCandidateFile(t, e, selected.Candidate.ID, fixture.WorkflowFile)) != want || selected.Candidate.HasIndexDelta != (mutation == "index") || view.Workflow.HasTaskIndexDelta != (mutation == "index") {
				t.Fatal("new candidate bytes/index do not reflect actual child effects", selected.Candidate, view.Workflow)
			}
			if mutation == "index" {
				var found *workspace.IndexEntry
				for index := range selected.Manifest.Index {
					if selected.Manifest.Index[index].Path == fixture.WorkflowFile {
						found = &selected.Manifest.Index[index]
					}
				}
				hash := sha1.New()
				hash.Write([]byte(fmt.Sprintf("blob %d\x00", len(fixture.WorkflowInvalid))))
				hash.Write([]byte(fixture.WorkflowInvalid))
				if found == nil || found.Mode != "100644" || found.Object != hex.EncodeToString(hash.Sum(nil)) {
					t.Fatal("actual managed index did not contain the exact staged blob", found)
				}
			} else if !reflect.DeepEqual(old.Manifest.Index, selected.Manifest.Index) {
				t.Fatal("source-only child unexpectedly changed the managed index")
			}
			reviewAfter, err := e.db.WorkflowReview(ctx, captured.Review.ID)
			if err != nil || !reflect.DeepEqual(captured.Review, reviewAfter) {
				t.Fatal("mutating check rewrote the earlier immutable blocked review", reviewAfter, err)
			}
			findings, err := e.db.WorkflowFindings(ctx, taskID, domain.WorkflowSelector{}, 100, "")
			if err != nil || len(findings.Items) != 1 || len(findings.Items[0].Resolutions) != 0 {
				t.Fatal("a source-modifying check silently resolved a prior blocker", findings, err)
			}
			for _, use := range view.Workflow.EvidenceUses {
				if use.EvidenceID == captured.Review.ID || use.EvidenceID == check.ID {
					t.Fatal("old checked-state evidence qualified the new selected subject", use)
				}
			}
			current, err := e.workflowApplicability(ctx, selected, domain.CheckScope{Kind: check.Applicability.ScopeKind, Paths: check.Applicability.Paths}, check.Applicability.ConditionKeys, check.DefinitionSHA256)
			if err != nil || workflow.Applicable(check.Applicability, current) {
				t.Fatal("whole checked-state basis stayed applicable after actual source/index mutation", current, err)
			}
			steps, err := e.db.WorkflowSteps(ctx, taskID, domain.WorkflowSelector{}, 100, "")
			if err != nil || len(steps.Items) != 3 || steps.Items[0].Role != "author" || steps.Items[1].Role != "review" || steps.Items[2].Role != "check" || steps.Items[2].ExecutorKind != "local_check" || steps.Items[2].AdvanceProof != nil || steps.Items[2].OutputCandidateID == nil || *steps.Items[2].OutputCandidateID != selected.Candidate.ID {
				t.Fatal("mutating check admitted a correction/advance or omitted its output candidate", steps, err)
			}
			budget, err := e.db.TaskBudget(ctx, taskID)
			if err != nil || budget.Budget == nil || budget.Budget.ConsumedAdmissions != "2" || budget.Budget.RemainingAdmissions != "2" {
				t.Fatal("local child/index Git subprocess consumed model units or created more native work", budget, err)
			}
			originalIndex, err := os.ReadFile(filepath.Join(projectPath, ".git", "index"))
			if err != nil || !bytes.Equal(beforeIndex, originalIndex) {
				t.Fatal("check modified the user's original Git index", err)
			}
			if _, err = os.Lstat(filepath.Join(projectPath, fixture.WorkflowFile)); !os.IsNotExist(err) {
				t.Fatal("check wrote candidate bytes into the original project", err)
			}
			if data, err := os.ReadFile(filepath.Join(projectPath, "source.txt")); err != nil || string(data) != "original\n" {
				t.Fatal("check changed unrelated original source", err)
			}
		})
	}
}
