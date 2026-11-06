package engine

import (
	"context"
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

func TestMain(m *testing.M) {
	if handled, code := process.HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	if handled, code := handleWorkflowMutatingCheck(os.Args[1:]); handled {
		os.Exit(code)
	}
	if handled, code := fixture.HandleWorkflowInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	if handled, code := fixture.HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func testEngine(t *testing.T) (*Engine, string, string) {
	t.Helper()
	ctx := context.Background()
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		// Git for Windows recognizes /dev/null as a config path; Go's NUL
		// device spelling is not accepted by its configuration reader.
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=0")
		if b, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("git %v: %s (%v)", args, b, e)
		}
	}
	git("init", "--quiet")
	git("config", "user.email", "fixture@example.invalid")
	git("config", "user.name", "Council fixture")
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "source.txt")
	git("commit", "--quiet", "-m", "fixture")
	lock, err := localstate.Acquire(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	db, err := store.Open(lock.DatabasePath())
	if err != nil {
		lock.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := app.New(db)
	body, _ := json.Marshal(map[string]string{"requestId": domain.NewID(), "selectedPath": repo, "basePath": repo})
	opened := service.Execute(ctx, "projects.open", app.Params{Body: body})
	if opened.Envelope.Error != nil {
		t.Fatal(opened.Envelope.Error)
	}
	encoded, _ := json.Marshal(opened.Envelope.Data)
	var project domain.ProjectOpen
	if err = json.Unmarshal(encoded, &project); err != nil {
		t.Fatal(err)
	}
	policy, err := db.Policy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := db.SetPolicy(ctx, domain.NewID(), "fixture", policy.Revision, "autonomous")
	if result.Envelope.Error != nil {
		t.Fatal(result.Envelope.Error)
	}
	e, err := New(db, lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e, project.Project.ID, repo
}

func admit(t *testing.T, e *Engine, project, scenario string) domain.Admission {
	t.Helper()
	result := e.StartTask(context.Background(), domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Perform the explicit local fixture task.", ExecutionMode: "fake", FakeScenario: scenario, Workflow: &domain.WorkflowInput{Mode: "author_only"}, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}})
	if result.Envelope.Error != nil {
		t.Fatal(result.Envelope.Error)
	}
	b, _ := json.Marshal(result.Envelope.Data)
	var a domain.Admission
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func settled(t *testing.T, e *Engine, id string) domain.Attempt {
	t.Helper()
	deadline := time.After(25 * time.Second)
	for {
		a, err := e.db.Attempt(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if a.Phase == "settled" {
			return a
		}
		select {
		case <-deadline:
			g, _ := e.db.Gate(context.Background())
			var incident domain.ExecutionIncident
			if g.IncidentID != nil {
				incident, _ = e.db.Incident(context.Background(), *g.IncidentID)
			}
			t.Fatalf("Attempt did not settle: %+v gate=%+v incident=%+v", a, g, incident)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestRealOwnedFixtureResponseAndExactAcceptance(t *testing.T) {
	e, project, _ := testEngine(t)
	admitted := admit(t, e, project, "response")
	a := settled(t, e, admitted.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "succeeded" || a.ResponseArtifactID == nil || a.CandidateID == nil || a.Cleanup.State != "verified" {
		t.Fatalf("%+v failure=%+v", a, a.PrimaryFailure)
	}
	if a.TargetObservations.Provider.Status != "unknown" || a.TargetObservations.Usage.InputTokens != nil {
		t.Fatal("Fake invented provider telemetry")
	}
	m, err := e.evidence.VerifyClosed(context.Background(), *a.ResponseArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := e.readAllResponse(m)
	if err != nil || string(body) != fixture.ResponseText {
		t.Fatalf("%q %v", body, err)
	}
	detail, err := e.db.Task(context.Background(), a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, TaskVersion: a.TaskVersion, AttemptID: a.ID, ResponseArtifactID: m.ID, ResponseSHA256: *m.SHA256, Confirmation: "reviewed_response_and_conditions"}
	result := e.AcceptResponse(context.Background(), a.TaskID, in)
	if result.Envelope.Error != nil {
		t.Fatal(result.Envelope.Error)
	}
	if replay := e.AcceptResponse(context.Background(), a.TaskID, in); replay.Envelope.Error != nil || replay.Envelope.Receipt.RequestID != result.Envelope.Receipt.RequestID {
		t.Fatal("Acceptance replay failed")
	}
}

func TestRealOwnedFixtureCandidateAndPartialFailure(t *testing.T) {
	for _, scenario := range []string{"candidate", "partial_failure"} {
		t.Run(scenario, func(t *testing.T) {
			e, project, repo := testEngine(t)
			admitted := admit(t, e, project, scenario)
			a := settled(t, e, admitted.Attempt.ID)
			want := "succeeded"
			if scenario == "partial_failure" {
				want = "failed"
			}
			if a.Outcome == nil || *a.Outcome != want || a.CandidateID == nil || !a.AuthoredDelta {
				t.Fatalf("%+v failure=%+v", a, a.PrimaryFailure)
			}
			if _, err := os.Stat(filepath.Join(repo, "council-fixture.txt")); !os.IsNotExist(err) {
				t.Fatal("Fixture changed original source")
			}
			c, err := e.db.Candidate(context.Background(), *a.CandidateID)
			if err != nil || !c.HasDelta || c.Verification != "pending" || c.Integration != "not_performed" {
				t.Fatalf("%+v %v", c, err)
			}
		})
	}
}

func TestAcceptedStopAndSharedCapacity(t *testing.T) {
	e, project, _ := testEngine(t)
	first := admit(t, e, project, "slow")
	deadline := time.After(10 * time.Second)
	for {
		a, err := e.db.Attempt(context.Background(), first.Attempt.ID)
		if err != nil {
			t.Fatal(err)
		}
		if a.Phase == "running" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("No running fixture")
		case <-time.After(15 * time.Millisecond):
		}
	}
	second := admit(t, e, project, "response")
	a, err := e.db.Attempt(context.Background(), second.Attempt.ID)
	if err != nil || a.Phase != "queued" {
		t.Fatalf("Shared capacity exceeded: %+v %v", a, err)
	}
	result := e.Stop(context.Background(), first.Attempt.ID, domain.NewID())
	if result.Envelope.Error != nil {
		t.Fatal(result.Envelope.Error)
	}
	a = settled(t, e, first.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "cancelled" || a.Cleanup.State != "verified" {
		t.Fatalf("%+v failure=%+v", a, a.PrimaryFailure)
	}
	a = settled(t, e, second.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "succeeded" {
		t.Fatalf("%+v failure=%+v", a, a.PrimaryFailure)
	}
}
