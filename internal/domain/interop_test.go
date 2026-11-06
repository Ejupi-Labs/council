package domain

import (
	"bytes"
	json "encoding/json/v2"
	"reflect"
	"testing"
)

func TestCapturedHarnessResolutionPreservesPins(t *testing.T) {
	for _, tc := range []struct{ harness, want string }{{"", "codex-native"}, {"codex", "codex-native"}, {"claude-code", "claude-native"}} {
		pins := ExecutionPins{HarnessID: tc.harness, Model: "provider/model:alias", PaymentPath: "native"}
		original := pins
		got, err := ResolveConnectionID(pins)
		if err != nil || got != tc.want || pins != original {
			t.Fatalf("%q: %q %v %#v", tc.harness, got, err, pins)
		}
	}
	for _, harness := range []string{"deepseek-harness", "junie", "antigravity"} {
		pins := ExecutionPins{HarnessID: harness}
		if err := ValidatePins(&pins); err != nil {
			t.Fatal("recognized guidance pin rejected", err)
		}
		if _, err := ResolveConnectionID(pins); err == nil || err.Code != "pin_unavailable" {
			t.Fatal("unsupported dispatch did not block", err)
		}
	}
	if _, err := ResolveConnectionID(ExecutionPins{HarnessID: "invented"}); err == nil || err.Code != "invalid_request" {
		t.Fatal(err)
	}
}

func TestLiteralModelSelectorCannotBecomeAnArgvOption(t *testing.T) {
	for _, model := range []string{"--resume", "-p", " \t--model=other", "--", "\x00value", "\nsonnet"} {
		if err := ValidateModelArgument(model); err == nil && ValidatePins(&ExecutionPins{Model: model}) == nil {
			t.Fatal("accepted option-like selector", model)
		}
	}
	if err := ValidatePins(&ExecutionPins{Model: "--resume"}); err != nil {
		t.Fatal("historical structural syntax rejected before replay", err)
	}
	for _, model := range []string{"sonnet", "provider/model:alias", "claude-model[1m]"} {
		pins := ExecutionPins{Model: model}
		if err := ValidatePins(&pins); err != nil || pins.Model != model {
			t.Fatal("changed literal native selector", pins, err)
		}
	}
}

func TestOptionalNativeObservationsPreserveHistoricalAbsence(t *testing.T) {
	raw := []byte(`{"provider":{"status":"unknown","value":null,"source":null,"observedAt":null},"account":{"status":"unknown","value":null,"source":null,"observedAt":null},"model":{"status":"unknown","value":null,"source":null,"observedAt":null},"nativeSession":{"status":"unknown","value":null,"source":null,"observedAt":null},"permissions":{"requestedMode":"autonomous","configuredMode":null,"nativeReportedMode":null,"limitations":[]},"usage":{"inputTokens":null,"outputTokens":null,"cachedInputTokens":null,"source":null,"observedAt":null,"completeness":"unknown"}}`)
	var observed TargetObservations
	if err := json.Unmarshal(raw, &observed); err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(observed)
	if err != nil || !bytes.Equal(raw, got) {
		t.Fatalf("historical absent fields were normalized: %s %v", got, err)
	}
	count, cost := "9007199254740993", "1.234e-7"
	observed.Usage.Scope = "top_level"
	observed.Usage.CacheCreationInputTokens = &count
	observed.NativeModels = &NativeModelsObservation{Scope: "whole_tree", Source: "claude.result.modelUsage", ObservedAt: "2026-11-06T12:00:00.000Z", Completeness: "partial", EvidenceArtifactIDs: []string{"11111111111111111111111111111111"}, OmittedModelCount: &count, Models: []NativeModelUsage{{Model: "reported", InputTokens: &count, CostEstimateUSD: &cost}}}
	got, err = json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TargetObservations
	if err := json.Unmarshal(got, &decoded); err != nil || !reflect.DeepEqual(decoded, observed) {
		t.Fatal("exact observation roundtrip failed", string(got), err)
	}
}
