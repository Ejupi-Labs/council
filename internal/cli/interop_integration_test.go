package cli

import (
	"context"
	"database/sql"
	"encoding/base64"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func TestClaudeDraftCLIExactEvidenceAndExplicitRetargetHistory(t *testing.T) {
	f := newCLIFixture(t)
	var opened struct {
		Project domain.Project `json:"project"`
	}
	decodeCLIData(t, f.run("projects", "open", f.repo), &opened)
	text := "\ufeffExact saved Claude instructions\r\nGrüße 東京 🙂\n"
	var draft domain.Draft
	decodeCLIData(t, f.run("drafts", "create", "--project", opened.Project.ID, "--task", text), &draft)
	request := domain.NewID()
	args := []string{"tasks", "start", "--project", opened.Project.ID, "--task", text, "--source-draft", draft.ID, "--source-draft-revision", draft.Revision, "--harness", "claude-code", "--model", "haiku", "--deliverable", "response_only", "--fake", "--timeout-seconds", "45", "--request-id", request}
	first := f.run(args...)
	var admission domain.Admission
	decodeCLIData(t, first, &admission)
	if admission.Attempt.ConnectionID != "claude-native" || admission.Version.Pins.Model != "haiku" {
		t.Fatal("CLI changed saved intent or target", admission)
	}
	if replay := f.run(args...); !reflect.DeepEqual(first, replay) {
		t.Fatal("replay changed captured Claude admission")
	}
	result := f.settled(admission.Attempt.ID)
	if result.Outcome == nil || *result.Outcome != "succeeded" || result.ResponseArtifactID == nil || result.TargetObservations.Provider.Status != "unknown" || result.TargetObservations.Usage.InputTokens != nil {
		t.Fatal("Claude fixture did not settle honestly", result)
	}
	var chunk struct {
		BytesBase64 string `json:"bytesBase64"`
		EOF         bool   `json:"eof"`
	}
	decodeCLIData(t, f.run("artifacts", "read", *result.ResponseArtifactID), &chunk)
	response, err := base64.StdEncoding.DecodeString(chunk.BytesBase64)
	if err != nil || !chunk.EOF || string(response) != fixture.ClaudeResponseText {
		t.Fatalf("Claude response bytes changed: %q %v", response, err)
	}
	var detail domain.TaskDetail
	decodeCLIData(t, f.run("tasks", "get", admission.Task.ID), &detail)
	if detail.Task.OriginalText != text || detail.Task.SourceDraftRef == nil || detail.Task.SourceDraftRef.DraftID != draft.ID || detail.Task.SourceDraftRef.Revision != draft.Revision {
		t.Fatal("CLI changed original saved instructions or provenance")
	}
	// Guidance is an explicit new captured version, never an implicit dispatch.
	f.run("tasks", "guidance", admission.Task.ID, "--expected-revision", detail.Task.Revision, "--guidance", "Use Codex for the next explicit attempt.", "--harness", "codex")
	decodeCLIData(t, f.run("tasks", "get", admission.Task.ID), &detail)
	var prior domain.TaskVersionDetail
	decodeCLIData(t, f.run("tasks", "version", admission.Task.ID, "--version", "1"), &prior)
	if prior.Version.Pins.HarnessID != "claude-code" || prior.OriginalText != text || detail.Task.CurrentVersion != "2" {
		t.Fatal("guidance rewrote earlier Claude history")
	}
	var next domain.Admission
	decodeCLIData(t, f.run("tasks", "run", admission.Task.ID, "--expected-revision", detail.Task.Revision, "--task-version", "2", "--fake"), &next)
	if next.Attempt.ConnectionID != "codex-native" {
		t.Fatal("explicit retarget was not captured")
	}
	if done := f.settled(next.Attempt.ID); done.Outcome == nil || *done.Outcome != "succeeded" {
		t.Fatal("retargeted fixture failed", done)
	}
}

func TestMaintenanceStopWithoutOwnerDoesNotBootstrapState(t *testing.T) {
	f := newCLIFixture(t)
	root := filepath.Join(f.home, ".council", "v2")
	result := concurrentCLICommand(context.Background(), f, 0, "service", "stop", "--request-id", domain.NewID())
	var envelope domain.Envelope
	if json.Unmarshal(result.stdout, &envelope) != nil || envelope.Error == nil || envelope.Error.Code != "owner_unavailable" {
		t.Fatal("missing owner was not refused", string(result.stdout))
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("maintenance Stop initialized or opened private state", err)
	}
}

func TestPopulatedPreviousSchemasUpgradeThroughCLIWithoutReplayingHistory(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(map[int]string{1: "foundation1", 2: "execution2"}[version], func(t *testing.T) {
			f := newCLIFixture(t)
			var opened struct {
				Project domain.Project `json:"project"`
			}
			decodeCLIData(t, f.run("projects", "open", f.repo), &opened)
			var draft domain.Draft
			decodeCLIData(t, f.run("drafts", "create", "--project", opened.Project.ID, "--task", "\ufeffKeep previous saved instructions\r\n東京\n"), &draft)
			var first domain.Envelope
			var original domain.Admission
			var previous domain.Attempt
			var args []string
			if version == 2 {
				args = []string{"tasks", "start", "--project", opened.Project.ID, "--task", "Preserve previous native Codex history", "--workflow", "author_only", "--harness", "codex", "--deliverable", "response_only", "--fake", "--request-id", domain.NewID()}
				first = f.run(args...)
				decodeCLIData(t, first, &original)
				previous = f.settled(original.Attempt.ID)
				if previous.Outcome == nil || *previous.Outcome != "succeeded" || previous.ResponseArtifactID == nil {
					t.Fatal("previous history did not complete", previous)
				}
				f.cleanup()
			}
			root := filepath.Join(f.home, ".council", "v2")
			lock, err := localstate.Acquire(root)
			if err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", lock.DatabasePath())
			if err != nil {
				lock.Close()
				t.Fatal(err)
			}
			db.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = db.Close(); _ = lock.Close() })
			var currentSchema int
			if err = db.QueryRow("PRAGMA user_version").Scan(&currentSchema); err != nil || currentSchema != 5 {
				t.Fatal("prior-schema fixture must start from its own known schema5 state", currentSchema, err)
			}
			if _, err = db.Exec("PRAGMA foreign_keys=ON"); err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			// This disposable fixture starts with current shared APIs. Remove every
			// known005 and004 child table before restoring its frozen historical DDL; a
			// schema2/version marker beside routing tables is rightly refused by the
			// production migrator. Keep retained history/receipt bytes unchanged.
			statements := []string{
				"DROP TABLE delivery_effects", "DROP TABLE delivery_operations",
				"DROP TABLE workflow_acceptances", "DROP TABLE workflow_resolutions",
				"DROP TABLE workflow_checks", "DROP TABLE workflow_reviews",
				"DROP TABLE workflow_steps", "DROP TABLE workflows", "DROP TABLE delivery_preferences",
				"DROP TABLE budget_ledger", "DROP TABLE budget_amendments",
				"DROP TABLE routing_continuation_proofs", "DROP TABLE routing_decisions",
				"DROP TABLE task_budgets", "DROP TABLE task_routing_versions",
				"DROP TABLE routing_report_bindings", "DROP TABLE routing_capacity_reports",
				"DROP TABLE routing_capacity_state", "DROP TABLE routing_account_links",
				"DROP TABLE routing_accounts", "DROP TABLE routing_profiles",
				"DROP TABLE routing_contexts", "DROP TABLE routing_policy",
			}
			if version == 2 {
				statements = append(statements, "DROP TABLE capacity_claims", "CREATE TABLE capacity_claims(connection_id TEXT PRIMARY KEY CHECK(connection_id='codex-native'),attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id))", "PRAGMA user_version=2")
			} else {
				statements = append(statements, "DROP TABLE response_acceptances", "DROP TABLE candidates", "DROP TABLE source_snapshots", "DROP TABLE artifacts", "DROP TABLE attempt_events", "DROP TABLE attempt_terminals", "DROP TABLE capacity_claims", "DROP TABLE execution_control", "DROP TABLE execution_incidents", "DROP TABLE attempts", "DROP TABLE task_versions", "DROP TABLE tasks", "PRAGMA user_version=1")
			}
			for _, statement := range statements {
				if _, err = tx.Exec(statement); err != nil {
					break
				}
			}
			if err == nil {
				err = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
			closeErr := db.Close()
			lockErr := lock.Close()
			if err != nil || closeErr != nil || lockErr != nil {
				t.Fatal("could not freeze owned prior schema", err, closeErr, lockErr)
			}
			var retained domain.Draft
			decodeCLIData(t, f.run("drafts", "get", draft.ID), &retained)
			if !reflect.DeepEqual(retained, draft) {
				t.Fatal("shared-service upgrade changed saved draft", retained, draft)
			}
			if version == 2 {
				if replay := f.run(args...); !reflect.DeepEqual(replay, first) {
					t.Fatal("schema upgrade changed original receipt")
				}
				var observed domain.Attempt
				decodeCLIData(t, f.run("attempts", "get", previous.ID), &observed)
				if observed.Revision != previous.Revision || observed.ConnectionID != "codex-native" || observed.StartedAt == nil || *observed.StartedAt != *previous.StartedAt {
					t.Fatal("old execution was replayed or relabelled", observed)
				}
				var chunk struct {
					BytesBase64 string `json:"bytesBase64"`
				}
				decodeCLIData(t, f.run("artifacts", "read", *previous.ResponseArtifactID), &chunk)
				body, err := base64.StdEncoding.DecodeString(chunk.BytesBase64)
				if err != nil || string(body) != fixture.ResponseText {
					t.Fatal("upgraded old evidence changed", err)
				}
			}
			var claude domain.Admission
			decodeCLIData(t, f.run("tasks", "start", "--project", opened.Project.ID, "--task", "Explicit new Claude task after upgrade", "--harness", "claude-code", "--fake"), &claude)
			if done := f.settled(claude.Attempt.ID); done.Outcome == nil || *done.Outcome != "succeeded" || done.ConnectionID != "claude-native" {
				t.Fatal("explicit new target failed after upgrade", done)
			}
		})
	}
}
