package store

import (
	"context"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestNativeObservationBindingsBoundsAndFakeSeparation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := openFixture(t, s, "observations")
	owner := domain.NewID()
	in := taskInput(p.ID)
	in.ExecutionMode = "native"
	in.Pins = &domain.ExecutionPins{HarnessID: "claude-code"}
	r := s.CreateTaskWithRouting(ctx, owner, in, wrapperRoutingFacts("fake"))
	if r.Status != 201 {
		t.Fatal(r)
	}
	admission := r.Envelope.Data.(domain.Admission)
	if _, err := s.ClaimAttempt(ctx, admission.Attempt.ID, owner); err != nil {
		t.Fatal(err)
	}
	stdout := domain.ArtifactRecord{ArtifactMetadata: domain.ArtifactMetadata{ID: domain.NewID(), AttemptID: admission.Attempt.ID, Kind: "stdout", ByteLength: "0", Completeness: "collecting", CreatedAt: domain.Now()}, PrivateRef: "", PhysicalIdentity: "fixture-observation-stdout"}
	stdout.PrivateRef = "artifacts/" + stdout.ID + ".bin"
	if err := s.PutArtifact(ctx, stdout); err != nil {
		t.Fatal(err)
	}
	stderr := closedArtifact(t, s, admission.Attempt.ID, "stderr")
	other := s.CreateTaskWithRouting(ctx, owner, inWithNewRequest(in), wrapperRoutingFacts("fake"))
	if other.Status != 201 {
		t.Fatal(other)
	}
	otherStdout := closedArtifact(t, s, other.Envelope.Data.(domain.Admission).Attempt.ID, "stdout")
	base := admission.Attempt.TargetObservations
	zero, cost := "0", "1e999"
	base.Usage.Scope = "top_level" // Absent optional usage remains unknown, without invented source/counts.
	base.ConfiguredModel = &domain.ObservedFact{Status: "unknown"}
	base.NativeVersion = &domain.ObservedFact{Status: "unknown"}
	base.NativeModels = &domain.NativeModelsObservation{Scope: "whole_tree", Source: "claude.result.modelUsage", ObservedAt: domain.Now(), Completeness: "complete", EvidenceArtifactIDs: []string{stdout.ID}, OmittedModelCount: &zero, Models: []domain.NativeModelUsage{{Model: "reported-model", InputTokens: &zero, CostEstimateUSD: &cost}}}
	if _, err := s.RecordAttempt(ctx, admission.Attempt.ID, domain.AttemptUpdate{Kind: "native_event", TargetObservations: &base}); err != nil {
		t.Fatal("valid unknown facts or mathematical cost lexeme rejected", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*domain.TargetObservations)
	}{
		{"wrong_attempt", func(o *domain.TargetObservations) { o.NativeModels.EvidenceArtifactIDs = []string{otherStdout.ID} }},
		{"wrong_kind", func(o *domain.TargetObservations) { o.NativeModels.EvidenceArtifactIDs = []string{stderr.ID} }},
		{"unknown_artifact", func(o *domain.TargetObservations) { o.NativeModels.EvidenceArtifactIDs = []string{domain.NewID()} }},
		{"negative_count", func(o *domain.TargetObservations) { v := "-1"; o.NativeModels.Models[0].InputTokens = &v }},
		{"fractional_cost", func(o *domain.TargetObservations) { v := "NaN"; o.NativeModels.Models[0].CostEstimateUSD = &v }},
		{"negative_cost", func(o *domain.TargetObservations) { v := "-0"; o.NativeModels.Models[0].CostEstimateUSD = &v }},
		{"omitted_but_complete", func(o *domain.TargetObservations) { v := "1"; o.NativeModels.OmittedModelCount = &v }},
		{"oversized_identity", func(o *domain.TargetObservations) { o.NativeModels.Models[0].Model = strings.Repeat("x", 257) }},
		{"too_many_models", func(o *domain.TargetObservations) {
			for range 16 {
				o.NativeModels.Models = append(o.NativeModels.Models, o.NativeModels.Models[0])
			}
		}},
		{"aggregate_budget", func(o *domain.TargetObservations) {
			o.Permissions.Limitations = []string{strings.Repeat("x", domain.MaxEventBytes)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy, err := decode[domain.TargetObservations](encode(base))
			if err != nil {
				t.Fatal(err)
			}
			tc.change(&copy)
			if _, err := s.RecordAttempt(ctx, admission.Attempt.ID, domain.AttemptUpdate{Kind: "native_event", TargetObservations: &copy}); err == nil {
				t.Fatal("invalid new observations persisted")
			}
		})
	}
	base.NativeModels.EvidenceArtifactIDs[0] = otherStdout.ID
	base.NativeModels.Models[0].Model = "caller mutation"
	retained, err := s.Attempt(ctx, admission.Attempt.ID)
	if err != nil || retained.TargetObservations.NativeModels.EvidenceArtifactIDs[0] != stdout.ID || retained.TargetObservations.NativeModels.Models[0].Model != "reported-model" {
		t.Fatal("caller pointer alias changed retained observations", retained, err)
	}
	fake := s.CreateTask(ctx, owner, taskInput(p.ID))
	if fake.Status != 201 {
		t.Fatal(fake)
	}
	a := fake.Envelope.Data.(domain.Admission).Attempt
	malicious := a.TargetObservations
	value, source, at := "fixture-provider", "local_fake", domain.Now()
	malicious.Provider = domain.ObservedFact{Status: "observed", Value: &value, Source: &source, ObservedAt: &at}
	if _, err := s.RecordAttempt(ctx, a.ID, domain.AttemptUpdate{Kind: "native_event", TargetObservations: &malicious}); err == nil {
		t.Fatal("fake facts promoted to real telemetry")
	}
	malicious = a.TargetObservations
	malicious.ConfiguredModel = &domain.ObservedFact{Status: "unknown"}
	if _, err := s.RecordAttempt(ctx, a.ID, domain.AttemptUpdate{Kind: "native_event", TargetObservations: &malicious}); err == nil {
		t.Fatal("fake added actual-observation extras")
	}
}

func inWithNewRequest(in domain.StartTaskInput) domain.StartTaskInput {
	in.RequestID = domain.NewID()
	return in
}
