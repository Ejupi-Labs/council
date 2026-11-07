package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

type graphManifestProgressCase struct {
	engine    *Engine
	graph     domain.GraphDetail
	source    domain.Attempt
	root      domain.GraphRootBinding
	scope     domain.DeliveryScope
	manifest  evidence.Metadata
	artifacts []string
	cancel    context.CancelFunc
}

func graphManifestProgressFixture(t *testing.T, noTargets bool) graphManifestProgressCase {
	t.Helper()
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.ctx, e.cancel = ctx, cancel
	var graphID, sourceID string
	if noTargets {
		group, source := graphReleaseReadyGroup(t, e, projectID)
		graphID, sourceID = group.Graph.ID, source
	} else {
		creation, err := e.captureGraphCreateFacts(ctx, projectID)
		if err != nil {
			t.Fatal(err)
		}
		result := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: []domain.GraphNodeInput{
			{Key: "a", OriginalText: "Receive the actual retained root before authoring a controlled response.", FakeScenario: "graph-response"},
			{Key: "b", OriginalText: "Independently receive the same complete root.", FakeScenario: "graph-response"},
		}}, creation)
		if result.Status != 201 || result.Envelope.Error != nil {
			t.Fatal("Actual root registration failed", result)
		}
		group := result.Envelope.Data.(domain.GraphAdmission)
		claimed, err := e.db.ClaimAttempt(ctx, group.PreparationAttemptID, e.config.OwnerInstanceID)
		if err != nil || claimed.Phase != "preparing" {
			t.Fatal("Actual root operation could not be claimed", claimed, err)
		}
		e.run(ctx, claimed)
		graphID, sourceID = group.Graph.ID, claimed.ID
	}
	view, err := e.db.Graph(ctx, graphID)
	if err != nil || view.Graph.RootInputID == nil || len(view.ActiveClaims) != 0 || view.Budget.Budget.ConsumedAdmissions != "0" {
		t.Fatal("Actual local-only root fixture is incomplete", view, err)
	}
	for _, node := range view.Nodes {
		if noTargets && (node.InputID == nil || node.State != "ready") || !noTargets && (node.InputID != nil || node.State != "waiting") {
			t.Fatal("The fixture does not have the requested actual receiver boundary", node)
		}
	}
	source, err := e.db.Attempt(ctx, sourceID)
	if err != nil || source.Phase != "settled" || source.Outcome == nil || *source.Outcome != "succeeded" || source.Cleanup.State != "verified" || source.OwnerInstanceID != e.config.OwnerInstanceID {
		t.Fatal("The fixture lacks a genuine current-owner completion", source, err)
	}
	record, err := e.db.GraphInputRecord(ctx, graphID, *view.Graph.RootInputID)
	if err != nil || record.PrivateRoot == nil || record.Public.Summary.State != "ready" || !record.Public.Summary.Current {
		t.Fatal("The root was not actually retained", record, err)
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, graphID)
	if err != nil {
		t.Fatal(err)
	}
	root := *record.PrivateRoot
	snapshot, err := e.graphRootSourceSnapshot(ctx, root, definition.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.workspace.VerifyGraphRoot(ctx, root, snapshot); err != nil {
		t.Fatal("The complete original root is not physically valid before the fault", err)
	}
	manifest, err := e.evidence.VerifyClosed(ctx, root.Manifest.ArtifactID)
	if err != nil || !graphArtifactMatches(manifest, root.Manifest, "source_manifest") {
		t.Fatal("The actual root manifest is not a complete matching artifact", err)
	}
	return graphManifestProgressCase{e, view, source, root, definition.Delivery, manifest, graphGuardArtifactNames(t, e), cancel}
}

func graphManifestCorrupt(t *testing.T, f graphManifestProgressCase) {
	t.Helper()
	// Mutate only this real registered manifest body. Preserve the closed header,
	// file identity and length so the failure requires a fresh content check.
	if f.manifest.Kind != "source_manifest" || f.manifest.RelativePath != "artifacts/"+f.manifest.ID+".bin" {
		t.Fatal("The controlled fault is not an exact retained root manifest", f.manifest)
	}
	path := filepath.Join(f.engine.lock.Root(), filepath.FromSlash(f.manifest.RelativePath))
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		t.Fatal("The controlled manifest is not an ordinary owned file", err)
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
		t.Fatal("The retained manifest has no complete bytes", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || opened.Size() != int64(len(body)) {
		file.Close()
		t.Fatal("The controlled manifest changed before the in-place write", err)
	}
	body[0] ^= 0x20
	written, writeErr := file.WriteAt(body, 0)
	if err = errors.Join(writeErr, file.Sync(), file.Close()); err != nil || written != len(body) {
		t.Fatal("The controlled in-place manifest write was incomplete", err)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() {
		t.Fatal("The controlled fault changed manifest identity or length", err)
	}
	if _, err := f.engine.evidence.VerifyClosed(context.Background(), f.root.Manifest.ArtifactID); !errors.Is(err, evidence.ErrCorrupt) {
		t.Fatal("The real closed manifest did not acquire an actual integrity mismatch", err)
	}
	if _, err := f.engine.graphRootSourceSnapshot(context.Background(), f.root, f.scope); !errors.Is(err, evidence.ErrCorrupt) {
		t.Fatal("The actual manifest fault did not reach the source-snapshot read boundary", err)
	}
}

func graphManifestSourceUnchanged(t *testing.T, f graphManifestProgressCase) {
	t.Helper()
	after, err := f.engine.db.Attempt(context.Background(), f.source.ID)
	before := f.source
	before.ObservedAt, after.ObservedAt = "", ""
	before.RemainingSeconds, after.RemainingSeconds = 0, 0
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("The failed continuation rewrote the actual original terminal", after, err)
	}
}

func graphManifestQuiet(t *testing.T, f graphManifestProgressCase) {
	t.Helper()
	e, ctx := f.engine, context.Background()
	after, err := e.db.Graph(ctx, f.graph.Graph.ID)
	gate, gateErr := e.db.Gate(ctx)
	pending, pendingErr := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || gateErr != nil || pendingErr != nil || gate.State != "open" || gate.IncidentID != nil || len(gate.ActiveClaims) != 0 || len(pending) != 0 ||
		!reflect.DeepEqual(f.graph, after) || !reflect.DeepEqual(f.artifacts, graphGuardArtifactNames(t, e)) {
		t.Fatal("Read-only failure/no-target observation invented an integrity incident or new work", after, gate, pending, err, gateErr, pendingErr)
	}
	graphManifestSourceUnchanged(t, f)
}

func TestGraphCompletionRootManifestCorruptionFencesEligibleInput(t *testing.T) {
	f := graphManifestProgressFixture(t, false)
	graphManifestCorrupt(t, f)
	e, ctx := f.engine, context.Background()
	e.continueGraphInputs(f.source.ID)
	view, err := e.db.Graph(ctx, f.graph.Graph.ID)
	gate, gateErr := e.db.Gate(ctx)
	inputs, inputErr := e.db.GraphInputs(ctx, f.graph.Graph.ID, 100, "")
	pending, pendingErr := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || gateErr != nil || inputErr != nil || pendingErr != nil || gate.State != "fenced" || gate.IncidentID == nil || len(gate.ActiveClaims) != 0 ||
		len(inputs.Items) != 1 || inputs.NextCursor != nil || len(pending) != 0 || view.Budget.Budget.ConsumedAdmissions != "0" {
		t.Fatal("An eligible completion silently dropped its corrupted root manifest instead of fencing", view, gate, inputs, pending, err, gateErr, inputErr, pendingErr)
	}
	incident, err := e.db.Incident(ctx, *gate.IncidentID)
	if err != nil || incident.Code != "graph_root_evidence_uncertain" || incident.AttemptID == nil || *incident.AttemptID != f.source.ID {
		t.Fatal("Manifest integrity failure lost its exact source/incident classification", incident, err)
	}
	for _, node := range view.Nodes {
		if node.InputID != nil || node.NodeAdmissionID != nil || node.LatestAttemptID != nil || node.AcceptedResult != nil {
			t.Fatal("Corrupt root manifest created usable receiving authority", node)
		}
	}
	if !reflect.DeepEqual(f.artifacts, graphGuardArtifactNames(t, e)) {
		t.Fatal("Corrupt root manifest was recaptured or copied into new evidence")
	}
	graphManifestSourceUnchanged(t, f)
}

func TestGraphCompletionRootManifestCorruptionWithoutTargetsIsReadOnly(t *testing.T) {
	f := graphManifestProgressFixture(t, true)
	graphManifestCorrupt(t, f)
	f.engine.continueGraphInputs(f.source.ID)
	graphManifestQuiet(t, f)
}

type graphManifestReadFaultSink struct {
	inner  artifactSink
	target string
	cancel context.CancelFunc
	fault  error
	hit    bool
}

func (s *graphManifestReadFaultSink) SaveArtifact(ctx context.Context, m evidence.Metadata) error {
	return s.inner.SaveArtifact(ctx, m)
}

func (s *graphManifestReadFaultSink) LoadArtifact(ctx context.Context, id string) (evidence.Metadata, error) {
	meta, err := s.inner.LoadArtifact(ctx, id)
	if err == nil && id == s.target && !s.hit {
		s.hit = true
		if s.cancel != nil {
			// The real metadata read succeeded. Cancel before CopyTo's first
			// body read; the exact cancellation must not become ErrCorrupt.
			s.cancel()
		}
		if s.fault != nil {
			return meta, s.fault
		}
	}
	return meta, err
}

func TestGraphCompletionRootManifestReadFailuresDoNotInventIntegrityFault(t *testing.T) {
	for _, mode := range []string{"cancelled-body-read", "ordinary-metadata-read-error"} {
		t.Run(mode, func(t *testing.T) {
			f := graphManifestProgressFixture(t, false)
			sink := &graphManifestReadFaultSink{inner: artifactSink{f.engine.db}, target: f.root.Manifest.ArtifactID}
			if mode == "cancelled-body-read" {
				sink.cancel = f.cancel
			} else {
				sink.fault = errors.New("Controlled ordinary artifact metadata read unavailable; no write was attempted.")
			}
			borrowed, err := evidence.New(f.engine.lock, sink)
			if err != nil {
				t.Fatal(err)
			}
			original := f.engine.evidence
			f.engine.evidence = borrowed
			t.Cleanup(func() {
				f.engine.evidence = original
				if err := borrowed.Close(); err != nil {
					t.Error(err)
				}
			})
			f.engine.continueGraphInputs(f.source.ID)
			if !sink.hit {
				t.Fatal("The fault did not reach the actual registered root manifest read")
			}
			if mode == "cancelled-body-read" && !errors.Is(f.engine.ctx.Err(), context.Canceled) {
				t.Fatal("The real owner context did not actually cancel at the body-read boundary")
			}
			graphManifestQuiet(t, f)
		})
	}
}
