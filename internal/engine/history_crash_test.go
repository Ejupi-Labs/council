package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

type historyCrashMarker struct {
	Input         domain.ExecuteHistoryExportInput `json:"input"`
	OperationID   string                           `json:"operationId"`
	Filename      string                           `json:"filename"`
	ExpectedBytes int                              `json:"expectedBytes"`
	Receipt       *domain.Receipt                  `json:"receipt"`
}

// The child uses only a fresh private state directory and fake task metadata.
// Its owner is killed at a named real publication boundary, without Close.
func TestHistoryPublicationAbruptOwnerBoundaries(t *testing.T) {
	if os.Getenv("COUNCIL_HISTORY_CRASH_CHILD") == "1" {
		historyCrashChild(t)
		return
	}
	for _, stage := range []string{"after_create_receipt", "after_permit", "after_create", "after_write", "before_directory_sync"} {
		t.Run(stage, func(t *testing.T) {
			base := t.TempDir()
			repo, root, marker := filepath.Join(base, "project"), filepath.Join(base, "private"), filepath.Join(base, "marker.json")
			if err := os.Mkdir(repo, 0700); err != nil {
				t.Fatal(err)
			}
			git := exec.Command("git", "init", "--quiet", repo)
			git.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=0")
			if out, err := git.CombinedOutput(); err != nil {
				t.Fatalf("fixture git init: %s: %v", out, err)
			}
			if err := os.WriteFile(filepath.Join(repo, "fixture.txt"), []byte("fixture\n"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"add", "fixture.txt"}, {"-c", "user.name=Council Test", "-c", "user.email=council@example.invalid", "commit", "--quiet", "-m", "fixture"}} {
				command := exec.Command("git", args...)
				command.Dir = repo
				command.Env = git.Env
				if out, err := command.CombinedOutput(); err != nil {
					t.Fatalf("fixture git: %s: %v", out, err)
				}
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestHistoryPublicationAbruptOwnerBoundaries$", "-test.count=1")
			cmd.Env = append(os.Environ(), "COUNCIL_HISTORY_CRASH_CHILD=1", "COUNCIL_HISTORY_CRASH_ROOT="+root, "COUNCIL_HISTORY_CRASH_REPO="+repo, "COUNCIL_HISTORY_CRASH_MARKER="+marker, "COUNCIL_HISTORY_CRASH_STAGE="+stage)
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			defer func() {
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			}()
			var record historyCrashMarker
			deadline := time.Now().Add(120 * time.Second)
			for {
				if data, e := os.ReadFile(marker); e == nil && json.Unmarshal(data, &record) == nil && domain.ValidID(record.OperationID) {
					break
				}
				if time.Now().After(deadline) {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					waited = true
					t.Fatalf("child did not reach %s: %s", stage, output.String())
				}
				time.Sleep(20 * time.Millisecond)
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			waited = true
			path := filepath.Join(root, record.Filename)
			before, readErr := os.ReadFile(path)
			switch stage {
			case "after_create_receipt", "after_permit":
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatalf("unexpected output at %s: %v", stage, readErr)
				}
			case "after_create":
				if readErr != nil || len(before) != 0 {
					t.Fatalf("expected empty exclusive-created file: %d %v", len(before), readErr)
				}
			case "after_write":
				if readErr != nil || len(before) == 0 || len(before) >= record.ExpectedBytes {
					t.Fatalf("expected actual partial output: %d / %d, %v", len(before), record.ExpectedBytes, readErr)
				}
			case "before_directory_sync":
				if readErr != nil || len(before) != record.ExpectedBytes {
					t.Fatalf("expected complete bytes before directory observation: %d / %d, %v", len(before), record.ExpectedBytes, readErr)
				}
			}
			lock, err := localstate.Acquire(root)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			db, err := store.Open(lock.DatabasePath())
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			next, err := New(db, lock, Config{OwnerInstanceID: domain.NewID()})
			if err != nil {
				t.Fatal(err)
			}
			defer next.Close()
			gate, err := db.Gate(context.Background())
			if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
				t.Fatalf("dirty owner did not fence: %+v %v", gate, err)
			}
			incident, err := db.Incident(context.Background(), *gate.IncidentID)
			if err != nil || incident.Code != "owner_epoch_uncertain" {
				t.Fatalf("missing owner epoch incident: %+v %v", incident, err)
			}
			op, err := db.HistoryExportOperation(context.Background(), record.OperationID)
			if err != nil {
				t.Fatal(err)
			}
			wantState := "effect_started"
			if stage == "after_create_receipt" {
				wantState = "prepared"
			}
			if op.State != wantState {
				t.Fatalf("unexpected retained effect state %s, want %s", op.State, wantState)
			}
			replayed := next.ExecuteHistoryExport(context.Background(), record.Input)
			if replayed.Envelope.Error != nil || replayed.Envelope.Receipt == nil || *replayed.Envelope.Receipt != *record.Receipt {
				t.Fatalf("original receipt not replayed: %+v", replayed)
			}
			if current := historyResultData[struct {
				Operation domain.HistoryExportOperation `json:"operation"`
			}](t, replayed).Operation; current.OperationID != record.OperationID || current.State != wantState {
				t.Fatal("replay changed operation", current)
			}
			after, afterErr := os.ReadFile(path)
			if errors.Is(readErr, os.ErrNotExist) {
				if !errors.Is(afterErr, os.ErrNotExist) {
					t.Fatal("restart created output", afterErr)
				}
			} else if afterErr != nil || !bytes.Equal(before, after) {
				t.Fatal("restart modified retained bytes", afterErr)
			}
		})
	}
}

func historyCrashChild(t *testing.T) {
	ctx := context.Background()
	root, repo, marker, stage := os.Getenv("COUNCIL_HISTORY_CRASH_ROOT"), os.Getenv("COUNCIL_HISTORY_CRASH_REPO"), os.Getenv("COUNCIL_HISTORY_CRASH_MARKER"), os.Getenv("COUNCIL_HISTORY_CRASH_STAGE")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(lock.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"requestId": domain.NewID(), "selectedPath": repo, "basePath": repo})
	opened := app.New(db).Execute(ctx, "projects.open", app.Params{Body: body})
	project := historyResultData[domain.ProjectOpen](t, opened).Project.ID
	e, err := New(db, lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	stopFixtureScheduler(t, e)
	if stage == "after_write" {
		for i := 0; i < 400; i++ {
			in := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Controlled fake metadata fixture.", ExecutionMode: "fake", FakeScenario: "response", Workflow: &domain.WorkflowInput{Mode: "author_only"}, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}
			if result := db.CreateTask(ctx, e.config.OwnerInstanceID, in); result.Envelope.Error != nil {
				t.Fatalf("fake metadata %d: %+v", i, result.Envelope.Error)
			}
		}
	}
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project})).Snapshot
	snapshot, err := db.HistorySnapshot(ctx, summary.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	payload, invalid := domain.EncodeHistoryExport(snapshot)
	if invalid != nil {
		t.Fatal(invalid)
	}
	name := "history-crash-test.json"
	preview := historyResultData[struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}](t, e.PreviewHistoryExport(ctx, domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: name})).Preview
	in := domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview.PreviewID, PreviewRevision: "1"}
	e.config.HistoryTestHook = func(actual string) error {
		if actual != stage {
			return nil
		}
		saved, err := db.Replay(ctx, in.RequestID, store.HistoryCreateOperation, store.HistoryCreateFingerprint(in))
		if err != nil || saved == nil || saved.Envelope.Receipt == nil {
			t.Fatalf("missing committed create receipt: %+v %v", saved, err)
		}
		record := historyCrashMarker{Input: in, OperationID: historyResultOperationID(*saved), Filename: name, ExpectedBytes: len(payload), Receipt: saved.Envelope.Receipt}
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(marker, encoded, 0600); err != nil {
			t.Fatal(err)
		}
		// Keep a live timer so the Go runtime cannot treat the controlled
		// barrier as a globally deadlocked child before the parent kills it.
		time.Sleep(24 * time.Hour)
		return errors.New("the fixture barrier unexpectedly expired")
	}
	_ = e.ExecuteHistoryExport(ctx, in)
	t.Fatalf("publication passed intended %s barrier", stage)
}
