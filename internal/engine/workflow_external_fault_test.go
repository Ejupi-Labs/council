package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/store"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type externalOutputAcknowledgementSink struct {
	evidence.Sink
	db    *store.Store
	after bool
	calls atomic.Int32
	fault chan evidence.Metadata
}

func (s *externalOutputAcknowledgementSink) SaveArtifact(ctx context.Context, metadata evidence.Metadata) error {
	if metadata.Kind != "stdout" || !metadata.Closed || metadata.Completeness != "complete" {
		return s.Sink.SaveArtifact(ctx, metadata)
	}
	attempt, err := s.db.Attempt(ctx, metadata.AttemptID)
	if err != nil {
		return err
	}
	if attempt.Role != "external_delivery" || s.calls.Add(1) != 1 {
		return s.Sink.SaveArtifact(ctx, metadata)
	}
	s.fault <- metadata
	if s.after {
		if err := s.Sink.SaveArtifact(ctx, metadata); err != nil {
			return err
		}
		return domain.Fail("write_uncertain", "Controlled external stdout committed but its acknowledgement was lost.")
	}
	return domain.Fail("storage_failed", "Controlled external stdout metadata failed before final commit.")
}

// The publisher is the fixed owned self-binary helper and creates only an
// exclusive local test marker. No remote publication or provider is attested.
func TestWorkflowExternalOutputAcknowledgementFaultNeverAcceptsOrRelaunches(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		name := "before_final_stdout_commit"
		if afterCommit {
			name = "after_commit_lost_acknowledgement"
		}
		t.Run(name, func(t *testing.T) {
			old, projectID, projectPath, original := workflowControlDocument(t)
			stopFixtureScheduler(t, old)
			if err := old.workspace.Close(); err != nil {
				t.Fatal(err)
			}
			if err := old.evidence.Close(); err != nil {
				t.Fatal(err)
			}
			sink := &externalOutputAcknowledgementSink{Sink: artifactSink{old.db}, db: old.db, after: afterCommit, fault: make(chan evidence.Metadata, 1)}
			artifacts, err := evidence.New(old.lock, sink)
			if err != nil {
				t.Fatal(err)
			}
			manager, err := workspace.New(old.lock, artifacts)
			if err != nil {
				artifacts.Close()
				t.Fatal(err)
			}
			old.evidence, old.workspace = artifacts, manager
			restartFixtureScheduler(old)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(t.TempDir(), "exclusive-local-publisher-marker.txt")
			timeout := 10
			input := workflowControlDocumentInput(projectID, "review_only")
			input.Workflow.Delivery.ExternalAction = &domain.ExternalActionInput{Action: "controlled-local-marker", Destination: marker, AcceptanceBasis: "command_completion", Command: domain.ExternalCommandInput{Executable: executable, Arguments: []string{"__fixture", "publish", "--case", "workflow-publication", "--destination", marker}, TimeoutSeconds: &timeout}}
			ctx := context.Background()
			result := old.StartTask(ctx, input)
			if result.Status != 201 {
				t.Fatal(result)
			}
			admission := decodeAdmission(t, result)
			var publication evidence.Metadata
			awaitFixtureCondition(t, "real publisher did not reach final stdout acknowledgement boundary", func() bool {
				select {
				case publication = <-sink.fault:
					return true
				default:
					return false
				}
			})
			actual := settled(t, old, publication.AttemptID)
			if actual.Role != "external_delivery" || actual.ExecutorKind != "local_external" || actual.StartedAt == nil || !actual.LaunchIntent || actual.Outcome == nil || *actual.Outcome == "succeeded" || actual.EvidenceStatus != "uncertain" || actual.Cleanup.State != "verified" {
				t.Fatalf("actual command acknowledgement loss became success or false nonexecution: %+v failure=%+v", actual, actual.PrimaryFailure)
			}
			markerBefore, err := os.Stat(marker)
			markerBytes, readErr := os.ReadFile(marker)
			if err != nil || readErr != nil || string(markerBytes) != "Council controlled local publication marker (005-v1).\n" {
				t.Fatal("the real exclusive local command effect did not occur before the acknowledgement fault", err, readErr)
			}
			view, err := old.db.Workflow(ctx, admission.Task.ID, domain.WorkflowSelector{})
			if err != nil || view.Workflow == nil || view.Acceptance != nil || view.Workflow.Completion != nil || view.Workflow.State == "completed" {
				t.Fatal("external acknowledgement uncertainty qualified acceptance", view, err)
			}
			deliveries, err := old.db.WorkflowDeliveries(ctx, admission.Task.ID, domain.WorkflowSelector{}, 100, "")
			if err != nil || len(deliveries.Items) != 1 || deliveries.Items[0].External == nil || deliveries.Items[0].External.Outcome != "uncertain" || deliveries.Items[0].External.AttemptID != actual.ID || !reflect.DeepEqual(deliveries.Items[0].External.Action, *input.Workflow.Delivery.ExternalAction) {
				t.Fatal("the actually launched external command lost its attributable uncertain result", deliveries, err)
			}
			gate, err := old.db.Gate(ctx)
			if err != nil || gate.State != "fenced" || gate.IncidentID == nil || sink.calls.Load() != 1 {
				t.Fatal("output acknowledgement ambiguity was replayed or left dispatch open", gate, sink.calls.Load(), err)
			}
			registered, err := old.db.Artifact(ctx, publication.ID)
			if err != nil {
				t.Fatal("external raw stdout identity was erased", err)
			}
			if afterCommit && (!registered.Closed || registered.SHA256 == nil || publication.SHA256 == nil || *registered.SHA256 != *publication.SHA256) || !afterCommit && registered.Closed {
				t.Fatal("before/after final metadata acknowledgement was misrepresented", registered)
			}
			raw, err := os.ReadFile(filepath.Join(old.lock.Root(), filepath.FromSlash(publication.RelativePath)))
			if err != nil || string(raw) != "Controlled local command completed; no remote publication is attested.\n" {
				t.Fatal("durable external stdout bytes were lost", string(raw), err)
			}
			beforeAttempts, err := old.db.Attempts(ctx, admission.Task.ID, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			stopFixtureScheduler(t, old)
			if err = old.workspace.Close(); err != nil {
				t.Fatal(err)
			}
			if err = old.evidence.Close(); err != nil {
				t.Fatal(err)
			}
			next, err := New(old.db, old.lock, Config{OwnerInstanceID: domain.NewID()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { next.Close() })
			if replay := next.StartTask(ctx, input); replay.Envelope.Error != nil || replay.Envelope.Receipt == nil || result.Envelope.Receipt == nil || *replay.Envelope.Receipt != *result.Envelope.Receipt {
				t.Fatal("new owner could not reconcile the exact acknowledged original admission", replay)
			}
			fresh := input
			fresh.RequestID = domain.NewID()
			if blocked := next.StartTask(ctx, fresh); blocked.Envelope.Error == nil || blocked.Envelope.Error.Code != "dispatch_fenced" {
				t.Fatal("new owner treated unknown external acknowledgement as fresh permission", blocked)
			}
			next.signal()
			observationDeadline := time.NewTimer(200 * time.Millisecond)
			ticker := time.NewTicker(20 * time.Millisecond)
			defer observationDeadline.Stop()
			defer ticker.Stop()
		observation:
			for {
				select {
				case <-observationDeadline.C:
					break observation
				case <-ticker.C:
					retained, readErr := next.db.Attempt(ctx, actual.ID)
					if readErr != nil || retained.Revision != actual.Revision || retained.StartedAt == nil || retained.OwnerInstanceID != actual.OwnerInstanceID {
						t.Fatal("recovery rewrote or restarted the old external attempt", retained, readErr)
					}
				}
			}
			afterAttempts, err := next.db.Attempts(ctx, admission.Task.ID, 100, "")
			if err != nil || len(afterAttempts.Items) != len(beforeAttempts.Items) {
				t.Fatal("recovery or receipt replay created another attempt", afterAttempts, err)
			}
			markerAfter, err := os.Stat(marker)
			actualMarker, readErr := os.ReadFile(marker)
			if err != nil || readErr != nil || !os.SameFile(markerBefore, markerAfter) || !markerBefore.ModTime().Equal(markerAfter.ModTime()) || !bytes.Equal(markerBytes, actualMarker) {
				t.Fatal("exclusive marker changed during recovery/replay", err, readErr)
			}
			if source, err := os.ReadFile(filepath.Join(projectPath, "docs", "note.md")); err != nil || !bytes.Equal(source, original) {
				t.Fatal("external scope silently applied candidate changes to the original project", err)
			}
		})
	}
}
