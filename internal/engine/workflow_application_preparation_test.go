package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// Only the first actual destination-file metadata acknowledgement is faulted.
// Bytes still use the real private evidence store; after=true commits through
// actual SQLite before losing the acknowledgement. No destination effect is fake.
type applicationPreparationSink struct {
	evidence.Sink
	after bool
	calls atomic.Int32
	fault chan evidence.Metadata
}

func (s *applicationPreparationSink) SaveArtifact(ctx context.Context, metadata evidence.Metadata) error {
	if metadata.Kind != "destination_file" {
		return s.Sink.SaveArtifact(ctx, metadata)
	}
	if s.calls.Add(1) != 1 {
		return s.Sink.SaveArtifact(ctx, metadata)
	}
	s.fault <- metadata
	if s.after {
		if err := s.Sink.SaveArtifact(ctx, metadata); err != nil {
			return err
		}
		return domain.Fail("write_uncertain", "Controlled preparation evidence committed but its acknowledgement was lost.")
	}
	return domain.Fail("storage_failed", "Controlled preparation metadata failed before commit.")
}

func TestWorkflowApplicationPreparationEvidenceFaultFencesWithoutDestinationPermit(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		name := "before_metadata_commit"
		if afterCommit {
			name = "after_commit_lost_acknowledgement"
		}
		t.Run(name, func(t *testing.T) {
			e, projectID, projectPath, original := workflowControlDocument(t)
			stopFixtureScheduler(t, e)
			if err := e.workspace.Close(); err != nil {
				t.Fatal(err)
			}
			if err := e.evidence.Close(); err != nil {
				t.Fatal(err)
			}
			sink := &applicationPreparationSink{Sink: artifactSink{e.db}, after: afterCommit, fault: make(chan evidence.Metadata, 1)}
			artifacts, err := evidence.New(e.lock, sink)
			if err != nil {
				t.Fatal(err)
			}
			manager, err := workspace.New(e.lock, artifacts)
			if err != nil {
				artifacts.Close()
				t.Fatal(err)
			}
			e.evidence, e.workspace = artifacts, manager
			var permittedActions atomic.Int64
			e.config.BeforeApplicationAction = func(context.Context, domain.Attempt, workspace.ApplyAction) { permittedActions.Add(1) }
			beforeIndex, err := os.ReadFile(filepath.Join(projectPath, ".git", "index"))
			if err != nil {
				t.Fatal(err)
			}
			restartFixtureScheduler(e)
			ctx := context.Background()
			input := workflowControlDocumentInput(projectID, "apply_verified_local")
			result := e.StartTask(ctx, input)
			if result.Status != 201 {
				t.Fatal(result)
			}
			admission := decodeAdmission(t, result)
			var publication evidence.Metadata
			awaitFixtureCondition(t, "actual preparation destination_file publication was not reached", func() bool {
				select {
				case publication = <-sink.fault:
					return true
				default:
					return false
				}
			})
			application := settled(t, e, publication.AttemptID)
			if application.ExecutorKind != "local_application" || application.LaunchIntent || application.Outcome == nil || *application.Outcome == "succeeded" || application.EvidenceStatus != "uncertain" || application.Cleanup.State != "verified" || application.Cleanup.Scope != "not_launched" {
				t.Fatalf("preparation acknowledgement fault became complete evidence or false child/effect proof: %+v failure=%+v", application, application.PrimaryFailure)
			}
			gate, err := e.db.Gate(ctx)
			if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
				t.Fatal("ambiguous preparation evidence did not fence dispatch", gate, err)
			}
			operation := workflowControlOperation(t, e, admission.Task.ID)
			if operation.Status != "interrupted" || operation.LastEffectSequence != "0" || operation.BeforeDestination != nil || operation.AfterDestination != nil || operation.PlanArtifactID != "" || operation.NextAction != "inspect_preparation" || permittedActions.Load() != 0 {
				t.Fatal("private preparation uncertainty invented destination effects or an acknowledged plan", operation, permittedActions.Load())
			}
			view, err := e.db.Workflow(ctx, admission.Task.ID, domain.WorkflowSelector{})
			if err != nil || view.Acceptance != nil || view.Workflow == nil || view.Workflow.State == "completed" || view.Workflow.Completion != nil {
				t.Fatal("uncertain preparation authorized workflow acceptance", view, err)
			}
			registered, registerErr := e.db.Artifact(ctx, publication.ID)
			if afterCommit {
				if registerErr != nil || !registered.Closed || registered.SHA256 == nil || publication.SHA256 == nil || *registered.SHA256 != *publication.SHA256 || registered.AttemptID != application.ID {
					t.Fatal("lost acknowledgement was falsely treated as no metadata commit", registered, registerErr)
				}
			} else if registerErr == nil {
				t.Fatal("precommit refusal invented registered destination evidence", registered)
			}
			retained, err := os.ReadFile(filepath.Join(e.lock.Root(), filepath.FromSlash(publication.RelativePath)))
			if err != nil || !bytes.Equal(retained, original) || !publication.Closed || publication.SHA256 == nil {
				t.Fatal("actual durable before bytes were lost after acknowledgement failure", err)
			}
			actual, err := os.ReadFile(filepath.Join(projectPath, "docs", "note.md"))
			afterIndex, indexErr := os.ReadFile(filepath.Join(projectPath, ".git", "index"))
			if err != nil || indexErr != nil || !bytes.Equal(actual, original) || !bytes.Equal(beforeIndex, afterIndex) {
				t.Fatal("preparation failure modified original text or user index", err, indexErr)
			}
			if sink.calls.Load() != 1 {
				t.Fatal("preparation evidence publication automatically retried", sink.calls.Load())
			}
			if replay := e.StartTask(ctx, input); replay.Envelope.Error != nil || replay.Envelope.Receipt == nil || result.Envelope.Receipt == nil || *replay.Envelope.Receipt != *result.Envelope.Receipt {
				t.Fatal("original admission receipt was not reconcilable after preparation uncertainty", replay)
			}
			fresh := input
			fresh.RequestID = domain.NewID()
			if blocked := e.StartTask(ctx, fresh); blocked.Envelope.Error == nil || blocked.Envelope.Error.Code != "dispatch_fenced" {
				t.Fatal("ambiguous preparation permitted fresh work", blocked)
			}
		})
	}
}
