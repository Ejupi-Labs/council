package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func graphNativeContextFixture(t *testing.T) (*Engine, string, string) {
	t.Helper()
	e, projectID, repo := testEngine(t)
	e.control.Lock()
	e.fenced = true
	e.control.Unlock()
	return e, projectID, repo
}

func contextWorkspaceEntries(t *testing.T, e *Engine) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(e.lock.Root(), "workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	result := []string{}
	for _, entry := range entries {
		result = append(result, entry.Name())
	}
	return result
}

func TestGraphNativeContextStandalonePreservesExactLaunchWithoutPreparation(t *testing.T) {
	e, projectID, _ := graphNativeContextFixture(t)
	ctx := context.Background()
	created := e.db.CreateTask(ctx, e.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID,
		OriginalText: "Keep the standalone original 東京\r\n", ExecutionMode: "fake", FakeScenario: "response", Workflow: &domain.WorkflowInput{Mode: "author_only"}})
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	admission := created.Envelope.Data.(domain.Admission)
	launch := adapters.Launch{Executable: "controlled-standalone", Args: []string{"--model", "literal selector 東京"}, Prompt: []byte("Complete standalone prompt\r\n🙂"), Observations: adapters.UnknownObservations("autonomous")}
	beforeFiles := contextWorkspaceEntries(t, e)
	beforeBudget, err := e.db.TaskBudget(ctx, admission.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, prepared, err := e.prepareGraphNativeContext(ctx, admission.Attempt, launch)
	if err != nil || prepared != nil || !reflect.DeepEqual(got, launch) {
		t.Fatal("Standalone launch changed or acquired graph context", prepared, err)
	}
	refs, err := e.recheckGraphNativeContext(ctx, admission.Attempt, nil)
	if err != nil || len(refs) != 0 {
		t.Fatal("Standalone nil recheck acquired graph effects", refs, err)
	}
	afterBudget, err := e.db.TaskBudget(ctx, admission.Task.ID)
	if err != nil || !reflect.DeepEqual(beforeBudget, afterBudget) || !reflect.DeepEqual(beforeFiles, contextWorkspaceEntries(t, e)) {
		t.Fatal("Standalone context helper changed authority or filesystem", err)
	}
}

func TestGraphNativeContextKnownMemberNeverFallsThroughWithoutExactDispatch(t *testing.T) {
	e, projectID, _ := graphNativeContextFixture(t)
	ctx := context.Background()
	facts, err := e.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake",
		Nodes: []domain.GraphNodeInput{{Key: "receiver", OriginalText: "Full receiving node original"}}}, facts)
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	graph := created.Envelope.Data.(domain.GraphAdmission)
	// This is an explicit rejection fixture, not a fabricated admitted execution:
	// the task membership is real while this attempted worker has NO dispatch row.
	a := domain.Attempt{ID: domain.NewID(), TaskID: graph.Nodes[0].TaskID, TaskVersion: graph.Nodes[0].TaskVersion, OwnerInstanceID: e.config.OwnerInstanceID,
		ConnectionID: "codex-native", ExecutionMode: "fake", ExecutorKind: "native", Role: "author", WorkflowID: domain.NewID(), StepID: domain.NewID()}
	before := contextWorkspaceEntries(t, e)
	launch := adapters.Launch{Executable: "must-not-launch", Args: []string{"--fixed"}, Prompt: []byte("No dispatch cannot authorize a standalone fallback")}
	got, prepared, err := e.prepareGraphNativeContext(ctx, a, launch)
	if err == nil {
		if prepared != nil {
			prepared.Close()
		}
		t.Fatal("Known graph membership fell through to standalone launch", got)
	}
	if prepared != nil {
		prepared.Close()
		if prepared.Prepared.Directory != "" || len(prepared.Artifacts) != 0 || !prepared.EffectsKnown {
			t.Fatal("Dispatch refusal created or hid context effects", prepared)
		}
	}
	if _, err := e.recheckGraphNativeContext(ctx, a, nil); err == nil {
		t.Fatal("Nil context bypassed known graph membership before native GO")
	}
	if !reflect.DeepEqual(before, contextWorkspaceEntries(t, e)) {
		t.Fatal("Invalid graph dispatch created private context copies")
	}
}

// The shared fixture uses actual admission, closed source bytes and the atomic
// materialization registrar. These checks create no native process and do not
// assert task acceptance or that a provider inspected the prepared context.
func graphAdmittedContextLaunch(t *testing.T, f graphOutputProducerCase) adapters.Launch {
	t.Helper()
	detail, err := f.engine.db.Task(context.Background(), f.attempt.TaskID)
	if err != nil || f.attempt.ResolvedTarget == nil {
		t.Fatal("Actual admitted target is unavailable", err)
	}
	launch, err := adapters.BuildResolvedNative(context.Background(), detail.Task.OriginalText, detail.Version, *f.attempt.ResolvedTarget,
		filepath.Join(t.TempDir(), "unlaunched-response.txt"), func(context.Context, domain.ExecutionPins) (string, error) { return os.Executable() })
	if err != nil {
		t.Fatal("Actual admitted native prompt setup failed", err)
	}
	return launch
}

func graphContextAssertUnlaunchedAndPreserved(t *testing.T, f graphOutputProducerCase, beforeBudget domain.GraphBudgetView, beforeWorking []byte) {
	t.Helper()
	ctx := context.Background()
	a, err := f.engine.db.Attempt(ctx, f.attempt.ID)
	if err != nil || a.Phase != "preparing" || a.LaunchIntent || a.SourceSnapshotID == nil || *a.SourceSnapshotID != f.materialized.Snapshot.ID {
		t.Fatal("Context-only preparation changed the actual receiving execution", a, err)
	}
	afterBudget, err := f.engine.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || !reflect.DeepEqual(beforeBudget, afterBudget) {
		t.Fatal("Context-only preparation changed shared accounting or authority", err)
	}
	working, err := os.ReadFile(filepath.Join(f.materialized.Snapshot.WorkspacePath, "source.txt"))
	if err != nil || !bytes.Equal(working, beforeWorking) {
		t.Fatal("Private context preparation changed the receiving source", err)
	}
	index, err := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if err != nil || !bytes.Equal(index, f.originalIndex) {
		t.Fatal("Private context preparation changed the exact user index", err)
	}
}

func TestGraphNativeContextActualAdmittedPreparationRechecksExactPrivateCopies(t *testing.T) {
	f := graphOutputAdmittedProducerFixture(t, strings.Repeat("Complete receiving instruction 東京\r\n", 700))
	e, a := f.engine, f.attempt
	ctx := context.Background()
	base := graphAdmittedContextLaunch(t, f)
	basePrompt, baseArgs := append([]byte{}, base.Prompt...), append([]string{}, base.Args...)
	beforeWorking, err := os.ReadFile(filepath.Join(f.materialized.Snapshot.WorkspacePath, "source.txt"))
	if err != nil {
		t.Fatal(err)
	}
	beforeBudget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	launch, prepared, err := e.prepareGraphNativeContext(ctx, a, base)
	if prepared != nil {
		t.Cleanup(func() {
			if err := prepared.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	if err != nil || prepared == nil || !prepared.EffectsKnown || prepared.EvidenceStatus != "complete" || prepared.Prepared.State != "complete" || prepared.Prepared.EffectsArtifact == nil || len(prepared.Artifacts) == 0 {
		t.Fatal("Actual admitted context preparation has no complete physical proof", prepared, err)
	}
	if !bytes.Equal(base.Prompt, basePrompt) || !reflect.DeepEqual(base.Args, baseArgs) || launch.Executable != base.Executable || !reflect.DeepEqual(launch.Observations, base.Observations) {
		t.Fatal("Context preparation rewrote the captured native launch or observations")
	}
	if a.ConnectionID != "codex-native" || !reflect.DeepEqual(launch.Args, baseArgs) {
		t.Fatal("Controlled Codex context changed captured executable arguments")
	}
	if filepath.Dir(prepared.Prepared.Directory) != filepath.Dir(f.materialized.Snapshot.WorkspacePath) || prepared.Prepared.Directory == f.materialized.Snapshot.WorkspacePath {
		t.Fatal("Context copies are not separate private siblings of the receiving workspace")
	}
	line := bytes.IndexByte(launch.Prompt, '\n')
	var payload struct {
		Original   string         `json:"originalInstructions"`
		Guidance   string         `json:"guidance"`
		Acceptance jsontext.Value `json:"acceptance"`
		Graph      struct {
			Packet domain.GraphContextPacket `json:"packet"`
			Access []struct {
				Ref       string  `json:"ref"`
				Path      *string `json:"path"`
				Text      *string `json:"text"`
				BaseField *string `json:"baseField"`
			} `json:"access"`
		} `json:"graphContext"`
	}
	if line < 0 || json.Unmarshal(launch.Prompt[line+1:], &payload) != nil || !reflect.DeepEqual(payload.Graph.Packet, prepared.Prepared.Packet) || len(payload.Graph.Access) != len(prepared.Prepared.Entries) {
		t.Fatal("The native prompt lost the exact admitted packet or access plan")
	}
	detail, err := e.db.Task(ctx, a.TaskID)
	if err != nil || payload.Original != detail.Task.OriginalText {
		t.Fatal("The graph prompt changed the complete node original", err)
	}
	var mutableCopy string
	hasInline := false
	for i, entry := range prepared.Prepared.Entries {
		access := payload.Graph.Access[i]
		if access.Ref != entry.Ref {
			t.Fatal("Actual context access attribution changed in the native prompt", entry)
		}
		var body []byte
		if entry.Mode == "file" {
			if entry.Path == "" || access.Path == nil || *access.Path != entry.Path {
				t.Fatal("Actual file access did not reach the native prompt", entry)
			}
			body, err = os.ReadFile(entry.Path)
			if err != nil {
				t.Fatal(err)
			}
			if len(body) != 0 {
				mutableCopy = entry.Path
			}
		} else {
			hasInline = true
			if entry.Mode != "inline" || entry.Text == nil || access.Path != nil {
				t.Fatal("Actual inline access was changed or replaced by a file path", entry)
			}
			body = []byte(*entry.Text)
			if access.Text != nil {
				if *access.Text != *entry.Text {
					t.Fatal("Native inline bytes changed")
				}
			} else {
				if access.BaseField == nil {
					t.Fatal("Required inline source has neither complete text nor exact base-field access")
				}
				var supplied []byte
				switch *access.BaseField {
				case "originalInstructions":
					supplied = []byte(payload.Original)
				case "guidance":
					supplied = []byte(payload.Guidance)
				case "acceptance":
					supplied = payload.Acceptance
				default:
					t.Fatal("Native context selected an unsupported base field")
				}
				if !bytes.Equal(body, supplied) {
					t.Fatal("Native context base-field access changed the complete inline bytes")
				}
			}
		}
		digest := sha256.Sum256(body)
		if hex.EncodeToString(digest[:]) != entry.ArtifactRef.SHA256 || strconv.Itoa(len(body)) != entry.ArtifactRef.ByteLength {
			t.Fatal("Native context access differs from its complete retained artifact")
		}
	}
	if mutableCopy == "" || !hasInline {
		t.Fatal("Actual admitted context fixture does not exercise mixed file and inline access")
	}
	if refs, err := e.recheckGraphNativeContext(ctx, a, prepared); err != nil || len(refs) == 0 {
		t.Fatal("Actual copied context failed the immediate pre-native recheck", refs, err)
	}
	file, err := os.OpenFile(mutableCopy, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteAt([]byte("!"), 0)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	if _, err = e.recheckGraphNativeContext(ctx, a, prepared); err == nil || prepared.EvidenceStatus != "incomplete" {
		t.Fatal("A changed actual private context copy survived the pre-native guard", err)
	}
	for _, entry := range prepared.Prepared.Entries {
		if _, err := e.evidence.VerifyClosed(ctx, entry.ArtifactRef.ArtifactID); err != nil {
			t.Fatal("Private-copy mutation changed the retained original", err)
		}
	}
	graphContextAssertUnlaunchedAndPreserved(t, f, beforeBudget, beforeWorking)
}

// Cancellation is injected only after the real effects metadata acknowledges a
// durable commit. The sink still delegates every write/read to the actual store.
type graphContextCancelAfterEffectsSink struct {
	artifactSink
	attemptID    string
	cancel       context.CancelFunc
	acknowledged bool
}

func (s *graphContextCancelAfterEffectsSink) SaveArtifact(ctx context.Context, m evidence.Metadata) error {
	err := s.artifactSink.SaveArtifact(ctx, m)
	if err == nil && m.AttemptID == s.attemptID && m.Kind == "graph_operation_effects" && m.Closed && m.Completeness == "complete" && m.SHA256 != nil {
		s.acknowledged = true
		s.cancel()
	}
	return err
}

func TestGraphNativeContextActualCancellationRetainsAcknowledgedPreparationEffects(t *testing.T) {
	f := graphOutputAdmittedProducerFixture(t)
	e, a := f.engine, f.attempt
	base := graphAdmittedContextLaunch(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := &graphContextCancelAfterEffectsSink{artifactSink: artifactSink{db: e.db}, attemptID: a.ID, cancel: cancel}
	artifacts, err := evidence.New(e.lock, sink)
	if err != nil {
		t.Fatal(err)
	}
	managed, err := workspace.New(e.lock, artifacts)
	if err != nil {
		artifacts.Close()
		t.Fatal(err)
	}
	priorEvidence, priorWorkspace := e.evidence, e.workspace
	e.evidence, e.workspace = artifacts, managed
	t.Cleanup(func() {
		e.evidence, e.workspace = priorEvidence, priorWorkspace
		if err := managed.Close(); err != nil {
			t.Error(err)
		}
		if err := artifacts.Close(); err != nil {
			t.Error(err)
		}
	})
	beforeWorking, err := os.ReadFile(filepath.Join(f.materialized.Snapshot.WorkspacePath, "source.txt"))
	if err != nil {
		t.Fatal(err)
	}
	beforeBudget, err := e.db.GraphBudget(context.Background(), f.dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	launch, prepared, err := e.prepareGraphNativeContext(ctx, a, base)
	if prepared != nil {
		t.Cleanup(func() {
			if err := prepared.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	if !sink.acknowledged || !errors.Is(err, context.Canceled) || prepared == nil || launch.Executable != "" || len(launch.Prompt) != 0 {
		t.Fatal("Cancellation did not stop launch after actual acknowledged context I/O", prepared, err)
	}
	if !prepared.EffectsKnown || prepared.EvidenceStatus != "complete" || prepared.Prepared.State != "complete" || prepared.Prepared.EffectsArtifact == nil || prepared.Prepared.Directory == "" || len(prepared.Prepared.Entries) == 0 {
		t.Fatal("Ordinary cancellation erased actual context effects or invented uncertainty", prepared)
	}
	ref := *prepared.Prepared.EffectsArtifact
	found := false
	for _, retained := range prepared.Artifacts {
		found = found || retained == ref
	}
	if !found {
		t.Fatal("Cancellation lost the exact acknowledged effects reference")
	}
	if _, err := e.verifyGraphArtifact(context.Background(), ref, "graph_operation_effects"); err != nil {
		t.Fatal("Retained effects do not have actual closed evidence", err)
	}
	if _, err := os.Stat(prepared.Prepared.Directory); err != nil {
		t.Fatal("Cancellation removed the actual context directory", err)
	}
	graphContextAssertUnlaunchedAndPreserved(t, f, beforeBudget, beforeWorking)
}
