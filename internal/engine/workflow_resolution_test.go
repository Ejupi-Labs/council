package engine

import (
	"bytes"
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

type resolutionHistoryFixture struct {
	Review     domain.ReviewRecord
	Older      domain.Candidate
	Resolution domain.FindingResolution
	Err        error
}

type resolutionPreparedFixture struct {
	AttemptID string
	Bytes     []byte
	Prompt    struct {
		Subject         domain.ReviewSubject   `json:"subject"`
		Findings        []domain.ReviewFinding `json:"findings"`
		PriorFindingIDs []string               `json:"priorFindingIds"`
	}
	Err error
}

// This deliberately inserts a labelled synthetic HISTORICAL resolution into a
// new owned private test database. Its different subject is an actual immutable
// earlier review-produced candidate with verified artifacts. This is a context
// selection fixture, never qualified review/correction/acceptance authority.
func insertResolutionHistoryFixture(ctx context.Context, e *Engine, taskID string) resolutionHistoryFixture {
	out := resolutionHistoryFixture{}
	reviews, err := e.db.WorkflowReviews(ctx, taskID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(reviews.Items) != 1 || len(reviews.Items[0].Findings) != 1 || reviews.Items[0].CandidateBinding == nil {
		out.Err = fmt.Errorf("controlled history needs one real blocking review: %+v / %v", reviews, err)
		return out
	}
	out.Review = reviews.Items[0]
	attempt, err := e.db.Attempt(ctx, out.Review.AttemptID)
	if err != nil || attempt.CandidateID == nil {
		out.Err = fmt.Errorf("controlled earlier review candidate missing: %+v / %v", attempt, err)
		return out
	}
	out.Older, err = e.db.Candidate(ctx, *attempt.CandidateID)
	if err != nil || out.Older.ID == out.Review.CandidateBinding.CandidateID {
		out.Err = fmt.Errorf("controlled older subject must differ from selected candidate: %+v / %v", out.Older, err)
		return out
	}
	view, err := e.db.Workflow(ctx, taskID, domain.WorkflowSelector{})
	if err != nil || view.Workflow == nil {
		out.Err = fmt.Errorf("controlled workflow unavailable: %v", err)
		return out
	}
	older, err := e.candidateWorkflowSubject(ctx, *view.Workflow, out.Older.ID)
	if err != nil {
		out.Err = err
		return out
	}
	reviewID, olderID := out.Review.ID, out.Older.ID
	out.Resolution = domain.FindingResolution{ID: domain.NewID(), WorkflowID: view.Workflow.ID, FindingID: out.Review.Findings[0].ID, OriginReviewID: &reviewID, CorrectedCandidateID: &olderID, CorrectionStepID: out.Review.StepID, QualifyingReviewIDs: []string{}, QualifyingCheckIDs: []string{}, EvidenceUses: []domain.EvidenceUse{}, CreatedAt: domain.Now()}
	raw, err := json.Marshal(out.Resolution)
	if err != nil {
		out.Err = err
		return out
	}
	path := filepath.ToSlash(e.lock.DatabasePath())
	if len(path) >= 2 && path[1] == ':' {
		path = "/" + path
	}
	query := url.Values{"_pragma": {"busy_timeout(5000)", "foreign_keys(1)", "synchronous(2)"}}
	fixtureDB, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String())
	if err != nil {
		out.Err = err
		return out
	}
	defer fixtureDB.Close()
	_, out.Err = fixtureDB.ExecContext(ctx, `INSERT INTO workflow_resolutions(sequence,id,workflow_id,finding_id,origin_review_id,corrected_candidate_id,corrected_subject_sha256,data) VALUES((SELECT COALESCE(MAX(sequence),0)+1 FROM workflow_resolutions),?,?,?,?,?,?,?)`, out.Resolution.ID, out.Resolution.WorkflowID, out.Resolution.FindingID, reviewID, olderID, older.Binding.ManifestSHA256, string(raw))
	return out
}

func TestWorkflowNativePreparationKeepsFindingWithHistoricalDifferentSubjectResolution(t *testing.T) {
	e, projectID, projectPath := testEngine(t)
	var taskID string
	ready := make(chan struct{})
	history := make(chan resolutionHistoryFixture, 1)
	prepared := make(chan resolutionPreparedFixture, 1)
	var installed atomic.Bool
	e.prepareGate.Lock()
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		if len(spec.Args) == 4 && spec.Args[0] == "__fixture" && spec.Args[1] == "check" && installed.CompareAndSwap(false, true) {
			select {
			case <-ready:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			fixture := insertResolutionHistoryFixture(ctx, e, taskID)
			history <- fixture
			if fixture.Err != nil {
				return nil, fixture.Err
			}
		}
		for index := 0; index+1 < len(spec.Args); index++ {
			if spec.Args[index] != "--role" || spec.Args[index+1] != "correction" {
				continue
			}
			out := resolutionPreparedFixture{}
			view, err := e.db.Workflow(ctx, taskID, domain.WorkflowSelector{})
			if err != nil || view.Workflow == nil || view.Workflow.CurrentStepID == nil {
				out.Err = fmt.Errorf("current correction step unavailable: %v", err)
			} else {
				step, stepErr := e.db.WorkflowStep(ctx, *view.Workflow.CurrentStepID)
				if stepErr != nil || step.AttemptID == nil {
					out.Err = fmt.Errorf("correction attempt unavailable: %v", stepErr)
				} else {
					out.AttemptID = *step.AttemptID
				}
			}
			out.Bytes, err = os.ReadFile(filepath.Join(spec.Cwd, fixture.WorkflowFile))
			if out.Err == nil {
				out.Err = err
			}
			start := bytes.IndexByte(spec.Stdin, '{')
			if out.Err == nil && start >= 0 {
				out.Err = json.Unmarshal(spec.Stdin[start:], &out.Prompt)
			} else if out.Err == nil {
				out.Err = errors.New("actual native correction prompt omitted structured context")
			}
			prepared <- out
			// Observe real Handoff/BuildRole preparation, then create no correction
			// process. The author, review and failed verifier above remain real.
			return nil, errors.New("controlled correction preparation observed; no correction process created")
		}
		return process.Prepare(ctx, spec)
	}
	e.prepareGate.Unlock()
	result := e.StartTask(context.Background(), workflowReviewInput(t, projectID))
	if result.Status != 201 {
		close(ready)
		t.Fatal(result)
	}
	a := decodeAdmission(t, result)
	taskID = a.Task.ID
	close(ready)
	var earlier resolutionHistoryFixture
	select {
	case earlier = <-history:
	case <-time.After(30 * time.Second):
		workflowReviewDiagnostics(t, e, taskID)
		t.Fatal("real workflow did not reach historical-context fixture boundary")
	}
	if earlier.Err != nil {
		t.Fatal("controlled historical metadata fixture failed", earlier.Err)
	}
	var current resolutionPreparedFixture
	select {
	case current = <-prepared:
	case <-time.After(30 * time.Second):
		workflowReviewDiagnostics(t, e, taskID)
		t.Fatal("real native correction preparation was not reached")
	}
	if current.Err != nil || string(current.Bytes) != fixture.WorkflowInvalid {
		t.Fatal("actual selected predecessor was not materialized", current, current.Err)
	}
	if current.Prompt.Subject.Kind != "candidate" || current.Prompt.Subject.ID != earlier.Review.CandidateBinding.CandidateID || current.Prompt.Subject.SHA256 != earlier.Review.CandidateBinding.ManifestSHA256 || current.Prompt.Subject.ID == earlier.Older.ID || !slices.Contains(current.Prompt.PriorFindingIDs, earlier.Review.Findings[0].ID) {
		t.Fatal("historical different-subject resolution suppressed the current review blocker", earlier, current.Prompt)
	}
	found := false
	for _, finding := range current.Prompt.Findings {
		found = found || finding.Key == earlier.Review.Findings[0].ModelKey
	}
	if !found {
		t.Fatal("actual native prompt omitted the unresolved registered finding body", current.Prompt)
	}
	stopped := settled(t, e, current.AttemptID)
	if stopped.Outcome == nil || *stopped.Outcome == "succeeded" || stopped.StartedAt != nil || stopped.Cleanup.State != "verified" {
		t.Fatal("controlled preparation-only observation invented a successful correction", stopped)
	}
	if data, err := os.ReadFile(filepath.Join(projectPath, "source.txt")); err != nil || string(data) != "original\n" {
		t.Fatal("context-selection fixture changed the user's project", err)
	}
	if _, err := os.Lstat(filepath.Join(projectPath, fixture.WorkflowFile)); !os.IsNotExist(err) {
		t.Fatal("preparation-only fixture published candidate bytes to the user project", err)
	}
}

func TestWorkflowResolutionRequiresExactFullResponseBinding(t *testing.T) {
	// Pure metadata equality qualification only; these labels do not claim any
	// filesystem, process, native review or historical-resolution proof.
	baseline := domain.ResponseBinding{ProducerAttemptID: domain.NewID(), ResponseArtifactID: domain.NewID(), ResponseSHA256: strings.Repeat("a", 64), ResponseByteLength: "10", SourceSnapshotID: domain.NewID(), ManifestSHA256: strings.Repeat("b", 64), IndexArtifactID: domain.NewID(), IndexSHA256: strings.Repeat("c", 64), ConditionsSHA256: strings.Repeat("d", 64)}
	subject := workflowSubject{Response: &baseline}
	if !workflowResolutionMatches(subject, domain.FindingResolution{CorrectedResponseBinding: &baseline}) {
		t.Fatal("exact current response binding was not recognized")
	}
	for _, example := range []struct {
		name   string
		change func(*domain.ResponseBinding)
	}{
		{"producer", func(b *domain.ResponseBinding) { b.ProducerAttemptID = domain.NewID() }},
		{"artifact", func(b *domain.ResponseBinding) { b.ResponseArtifactID = domain.NewID() }},
		{"response hash", func(b *domain.ResponseBinding) { b.ResponseSHA256 = strings.Repeat("e", 64) }},
		{"response length", func(b *domain.ResponseBinding) { b.ResponseByteLength = "11" }},
		{"source snapshot", func(b *domain.ResponseBinding) { b.SourceSnapshotID = domain.NewID() }},
		{"manifest hash", func(b *domain.ResponseBinding) { b.ManifestSHA256 = strings.Repeat("e", 64) }},
		{"index artifact", func(b *domain.ResponseBinding) { b.IndexArtifactID = domain.NewID() }},
		{"index hash", func(b *domain.ResponseBinding) { b.IndexSHA256 = strings.Repeat("e", 64) }},
		{"conditions hash", func(b *domain.ResponseBinding) { b.ConditionsSHA256 = strings.Repeat("e", 64) }},
	} {
		t.Run(example.name, func(t *testing.T) {
			older := baseline
			example.change(&older)
			if workflowResolutionMatches(subject, domain.FindingResolution{CorrectedResponseBinding: &older}) {
				t.Fatal("older response resolution matched after changing", example.name)
			}
		})
	}
	olderCandidate := domain.NewID()
	if workflowResolutionMatches(subject, domain.FindingResolution{CorrectedCandidateID: &olderCandidate}) || workflowResolutionMatches(subject, domain.FindingResolution{}) {
		t.Fatal("candidate-only or absent resolution suppressed a response finding")
	}
}
