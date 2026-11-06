package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func decodeInteropData(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}
func interopArtifactBytes(t *testing.T, e *Engine, id string) []byte {
	t.Helper()
	meta, err := e.evidence.VerifyClosed(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	body, err := e.readAllResponse(meta)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func interopRaw(t *testing.T, e *Engine, a domain.Attempt, kind string) []byte {
	t.Helper()
	artifacts, err := e.db.Artifacts(context.Background(), a.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, meta := range artifacts.Items {
		if meta.Kind == kind {
			return interopArtifactBytes(t, e, meta.ID)
		}
	}
	t.Fatal("missing retained artifact", kind)
	return nil
}

func TestInteropClaudeResponseArtifactComesFromExactResultWithoutOutputFile(t *testing.T) {
	e, project, _ := testEngine(t)
	admission := admitClaude(t, e, project, "trailing_observation")
	a := settled(t, e, admission.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "succeeded" || a.ResponseArtifactID == nil || a.CandidateID == nil || a.Cleanup.State != "verified" || a.ConnectionID != "claude-native" {
		t.Fatalf("%+v failure=%+v", a, a.PrimaryFailure)
	}
	if _, err := os.Stat(filepath.Join(e.lock.Root(), "response-staging")); !os.IsNotExist(err) {
		t.Fatal("Claude created a native output-file reservation", err)
	}
	raw := interopRaw(t, e, a, "stdout")
	var resultText *string
	resultLine := -1
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte{'\n'}), []byte{'\n'})
	for i, line := range lines {
		var msg struct {
			Type   string  `json:"type"`
			Result *string `json:"result"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type == "result" {
			resultText = msg.Result
			resultLine = i
		}
	}
	if resultText == nil || resultLine < 0 || resultLine >= len(lines)-1 {
		t.Fatal("fixture lacks a trailing observation after result", string(raw))
	}
	response := interopArtifactBytes(t, e, *a.ResponseArtifactID)
	if string(response) != fixture.ClaudeResponseText {
		t.Fatalf("authored Unicode/BOM/CRLF response changed: %q", response)
	}
	if !bytes.Equal(response, []byte(*resultText)) {
		t.Fatalf("decoded response bytes changed: %q != %q", response, *resultText)
	}
	if len(interopRaw(t, e, a, "stderr")) == 0 {
		t.Fatal("stderr tail missing")
	}
	if a.TargetObservations.Provider.Status != "unknown" || a.TargetObservations.Account.Status != "unknown" || a.TargetObservations.Model.Status != "unknown" || a.TargetObservations.NativeSession.Status != "unknown" || a.TargetObservations.Usage.InputTokens != nil || a.TargetObservations.Usage.OutputTokens != nil || a.TargetObservations.ConfiguredModel != nil || a.TargetObservations.NativeVersion != nil || a.TargetObservations.NativeModels != nil {
		t.Fatal("local fixture invented real telemetry", a.TargetObservations)
	}
	meta, err := e.evidence.VerifyClosed(context.Background(), *a.ResponseArtifactID)
	if err != nil || meta.AttemptID != a.ID || meta.Kind != "final_response" || meta.SHA256 == nil || meta.Completeness != "complete" {
		t.Fatal(meta, err)
	}
	detail, err := e.db.Task(context.Background(), a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	accept := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, TaskVersion: a.TaskVersion, AttemptID: a.ID, ResponseArtifactID: meta.ID, ResponseSHA256: *meta.SHA256, Confirmation: "reviewed_response_and_conditions"}
	first := e.AcceptResponse(context.Background(), a.TaskID, accept)
	if first.Envelope.Error != nil || first.Envelope.Receipt == nil {
		t.Fatal(first)
	}
	if replay := e.AcceptResponse(context.Background(), a.TaskID, accept); replay.Envelope.Error != nil || replay.Envelope.Receipt == nil || *replay.Envelope.Receipt != *first.Envelope.Receipt {
		t.Fatal("acceptance replay changed", replay)
	}
}

func TestInteropClaudeErrorsNeverPublishSuccessfulResponse(t *testing.T) {
	for _, scenario := range []string{"malformed", "missing_result", "repeated_result", "trailing_error", "success_error", "empty_response"} {
		t.Run(scenario, func(t *testing.T) {
			e, project, _ := testEngine(t)
			admission := admitClaude(t, e, project, scenario)
			a := settled(t, e, admission.Attempt.ID)
			if a.Outcome == nil || *a.Outcome != "failed" || a.PrimaryFailure == nil || a.Cleanup.State != "verified" {
				t.Fatalf("%+v failure=%+v", a, a.PrimaryFailure)
			}
			if scenario != "empty_response" && a.ResponseArtifactID != nil {
				t.Fatal("failed protocol published an authoritative response", a)
			}
			artifacts, err := e.db.Artifacts(context.Background(), a.ID, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			stdoutFound := false
			for _, meta := range artifacts.Items {
				if meta.Kind == "stdout" {
					stdoutFound = true
					if meta.ByteLength == "0" {
						t.Fatal("available protocol bytes lost")
					}
				}
			}
			if !stdoutFound {
				t.Fatal("missing raw protocol evidence")
			}
			if _, err := os.Stat(filepath.Join(e.lock.Root(), "response-staging")); !os.IsNotExist(err) {
				t.Fatal("Claude created output reservation", err)
			}
		})
	}
}

func TestInteropClaudeEmptyTextRequiresUsefulCandidate(t *testing.T) {
	e, project, _ := testEngine(t)
	a := settled(t, e, admitClaude(t, e, project, "empty_candidate").Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "succeeded" || !a.AuthoredDelta || a.CandidateID == nil || a.Cleanup.State != "verified" {
		t.Fatalf("%+v failure=%+v", a, a.PrimaryFailure)
	}
	if a.ResponseArtifactID != nil && len(interopArtifactBytes(t, e, *a.ResponseArtifactID)) != 0 {
		t.Fatal("fixture did not exercise empty result text")
	}
	candidate, err := e.db.Candidate(context.Background(), *a.CandidateID)
	if err != nil || !candidate.HasDelta || candidate.Verification != "pending" || candidate.Integration != "not_performed" {
		t.Fatal(candidate, err)
	}
}

// interopPublicationSink injects only the new Claude final-response publication
// boundary. Other artifacts commit through the actual SQLite/evidence path.
type interopPublicationSink struct {
	evidence.Sink
	afterCommit bool
	calls       atomic.Int32
	published   chan evidence.Metadata
}

func (s *interopPublicationSink) SaveArtifact(ctx context.Context, m evidence.Metadata) error {
	if m.Kind != "final_response" {
		return s.Sink.SaveArtifact(ctx, m)
	}
	s.calls.Add(1)
	s.published <- m
	if s.afterCommit {
		if err := s.Sink.SaveArtifact(ctx, m); err != nil {
			return err
		}
		return domain.Fail("write_uncertain", "The final response committed but its acknowledgement was lost.")
	}
	return domain.Fail("storage_failed", "The final response metadata failed before its commit.")
}

func installInteropPublicationSink(t *testing.T, e *Engine, afterCommit bool) *interopPublicationSink {
	t.Helper()
	stopFixtureScheduler(t, e)
	if err := e.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := e.evidence.Close(); err != nil {
		t.Fatal(err)
	}
	sink := &interopPublicationSink{Sink: artifactSink{e.db}, afterCommit: afterCommit, published: make(chan evidence.Metadata, 4)}
	artifacts, err := evidence.New(e.lock, sink)
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := workspace.New(e.lock, artifacts)
	if err != nil {
		artifacts.Close()
		t.Fatal(err)
	}
	e.evidence, e.workspace = artifacts, workspaces
	restartFixtureScheduler(e)
	return sink
}

func TestInteropClaudeResponsePublicationFaultCannotBecomeCandidateSuccess(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		name := "precommit"
		if afterCommit {
			name = "lost_acknowledgement"
		}
		t.Run(name, func(t *testing.T) {
			e, project, repo := testEngine(t)
			sink := installInteropPublicationSink(t, e, afterCommit)
			ctx := context.Background()
			in := claudeFixtureInput(project, "candidate")
			first := e.StartTask(ctx, in)
			admission := decodeAdmission(t, first)
			a := settled(t, e, admission.Attempt.ID)
			if a.Outcome == nil || *a.Outcome != "failed" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "evidence_failed" || a.EvidenceStatus != "uncertain" || a.Cleanup.State != "verified" || a.CandidateID == nil || !a.AuthoredDelta || a.ResponseArtifactID != nil {
				t.Fatalf("Response publication fault became candidate success or false cleanup: %+v failure=%+v", a, a.PrimaryFailure)
			}
			candidate, err := e.db.Candidate(ctx, *a.CandidateID)
			if err != nil || !candidate.HasDelta || candidate.Verification != "pending" || candidate.Integration != "not_performed" {
				t.Fatal("useful candidate evidence lost or promoted", candidate, err)
			}
			if sink.calls.Load() != 1 {
				t.Fatal("final-response publication automatically retried", sink.calls.Load())
			}
			var publication evidence.Metadata
			select {
			case publication = <-sink.published:
			default:
				t.Fatal("final-response branch was not reached")
			}
			if !publication.Closed || publication.Completeness != "complete" || publication.SHA256 == nil {
				t.Fatal("fault was injected before immutable bytes/root proof", publication)
			}
			retained, err := os.ReadFile(filepath.Join(e.lock.Root(), filepath.FromSlash(publication.RelativePath)))
			if err != nil || string(retained) != fixture.ClaudeResponseText {
				t.Fatal("durable response bytes were erased after metadata fault", err)
			}
			registered, err := e.db.Artifact(ctx, publication.ID)
			if afterCommit {
				if err != nil || registered.AttemptID != a.ID || !registered.Closed || registered.SHA256 == nil || *registered.SHA256 != *publication.SHA256 {
					t.Fatal("lost acknowledgement was treated as no commit", registered, err)
				}
				if got := interopArtifactBytes(t, e, publication.ID); !bytes.Equal(got, retained) {
					t.Fatal("committed orphan response bytes changed")
				}
			} else if err == nil {
				t.Fatal("known precommit fault invented a registered artifact", registered)
			}
			// Root success, final text and stderr are still inspectable even though
			// publication/authoring failed. No second worker is permission to repair it.
			raw := interopRaw(t, e, a, "stdout")
			if !bytes.Contains(raw, []byte(`"type":"result"`)) || !bytes.Contains(raw, []byte(`"is_error":false`)) || len(interopRaw(t, e, a, "stderr")) == 0 {
				t.Fatal("native terminal/diagnostic bytes lost")
			}
			events, err := e.db.Events(ctx, a.ID, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			inputComplete, workers := false, 0
			for _, event := range events.Items {
				if event.Kind == "worker_started" {
					workers++
				}
				if event.Kind == "input_delivery" {
					var proof struct {
						Complete bool `json:"complete"`
					}
					if err := json.Unmarshal(event.Metadata, &proof); err != nil {
						t.Fatal(err)
					}
					inputComplete = proof.Complete
				}
			}
			if !inputComplete || workers != 1 {
				t.Fatal("input/root execution provenance was rewritten", inputComplete, workers)
			}
			gate, err := e.db.Gate(ctx)
			if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
				t.Fatal("publication fault did not fence dispatch", gate, err)
			}
			if _, err := os.Stat(filepath.Join(repo, "council-fixture.txt")); !os.IsNotExist(err) {
				t.Fatal("candidate escaped source isolation", err)
			}
			replayed := e.StartTask(ctx, in)
			if replayed.Envelope.Error != nil || replayed.Envelope.Receipt == nil || *replayed.Envelope.Receipt != *first.Envelope.Receipt {
				t.Fatal("admission receipt could not be reconciled", replayed)
			}
			attempts, err := e.db.Attempts(ctx, a.TaskID, 100, "")
			if err != nil || len(attempts.Items) != 1 || attempts.Items[0].ID != a.ID {
				t.Fatal("publication fault replayed native work", attempts, err)
			}
			unsafeAccept := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: "1", TaskVersion: a.TaskVersion, AttemptID: a.ID, ResponseArtifactID: publication.ID, ResponseSHA256: *publication.SHA256, Confirmation: "reviewed_response_and_conditions"}
			if r := e.AcceptResponse(ctx, a.TaskID, unsafeAccept); r.Envelope.Error == nil || r.Envelope.Receipt != nil {
				t.Fatal("orphan response accepted failed work", r)
			}
			fresh := claudeFixtureInput(project, "response")
			fresh.Pins = &domain.ExecutionPins{HarnessID: "codex"}
			if r := e.StartTask(ctx, fresh); r.Envelope.Error == nil || r.Envelope.Error.Code != "dispatch_fenced" {
				t.Fatal("other harness bypassed response fault fence", r)
			}
		})
	}
}

func TestInteropClaudeModelUsageFixtureStaysUnmeasured(t *testing.T) {
	e, project, _ := testEngine(t)
	a := settled(t, e, admitClaude(t, e, project, "model_usage").Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "succeeded" {
		t.Fatalf("%+v failure=%+v", a, a.PrimaryFailure)
	}
	if !strings.Contains(string(interopRaw(t, e, a, "stdout")), "modelUsage") {
		t.Fatal("fixture did not exercise model usage")
	}
	if a.TargetObservations.Model.Status != "unknown" || a.TargetObservations.Usage.InputTokens != nil || a.TargetObservations.NativeModels != nil || a.TargetObservations.NativeVersion != nil {
		t.Fatal("fake reported models/counters promoted to provider facts", a.TargetObservations)
	}
}
