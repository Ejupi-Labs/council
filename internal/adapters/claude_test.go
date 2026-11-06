package adapters

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const claudeStdoutID = "0123456789abcdef0123456789abcdef"

func finishClaude(t *testing.T, transcript string, emit func(Event) error) (*ClaudeParser, error) {
	t.Helper()
	p := NewClaudeParser(domain.ExecutionPins{HarnessID: "claude-code", Model: "haiku"}, claudeStdoutID, emit)
	_, err := p.Write([]byte(transcript))
	if err == nil {
		err = p.Finish()
	}
	return p, err
}

func claudeTerminal(extra string) string {
	return `{"type":"result","session_id":"session","subtype":"success","is_error":false,"result":"response"` + extra + "}\n"
}

func TestClaudeLaunchPreservesCapturedInputAndNativeProfile(t *testing.T) {
	original := "\ufeffOriginal 東京\r\n$(no shell)\n�"
	v := domain.TaskVersion{RequestedPolicy: domain.Policy{Mode: "autonomous"}, GuidanceText: "exact guidance\r\n", Pins: domain.ExecutionPins{HarnessID: "claude-code", Model: "haiku"}}
	launch, err := BuildNative(context.Background(), original, v, "must-not-be-an-output-file", func(_ context.Context, pins domain.ExecutionPins) (string, error) {
		if pins != v.Pins {
			t.Fatal("builder changed captured pins")
		}
		return "authored-native-executable", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--print", "--input-format", "text", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--permission-mode", "bypassPermissions", "--permission-prompts", "none", "--model", "haiku"}
	if !reflect.DeepEqual(launch.Args, want) {
		t.Fatalf("native profile changed: %q", launch.Args)
	}
	var payload struct {
		Original string `json:"originalInstructions"`
		Guidance string `json:"guidance"`
	}
	index := bytes.IndexByte(launch.Prompt, '\n')
	if index < 0 || json.Unmarshal(launch.Prompt[index+1:], &payload) != nil || payload.Original != original || payload.Guidance != v.GuidanceText {
		t.Fatal("assembled prompt changed original text or guidance")
	}
	for _, selector := range []string{"-bad", "  --bad"} {
		v.Pins.Model = selector
		if _, err := BuildClaude(context.Background(), original, v, "", nil); err == nil {
			t.Fatal("option-like model reached native argv")
		}
	}
	v.Pins.Model = ""
	v.RequestedPolicy.Mode = "supervised"
	if _, err := BuildClaude(context.Background(), original, v, "", nil); err == nil {
		t.Fatal("unsupported permission mode launched")
	}
}

func TestClaudeAuthoredGoldenRetainsFinalBytesModelsAndTrailingEvents(t *testing.T) {
	transcript, err := os.ReadFile("testdata/claude/success.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	p := NewClaudeParser(domain.ExecutionPins{HarnessID: "claude-code", Model: "haiku"}, claudeStdoutID, func(event Event) error { events = append(events, event); return nil })
	for _, value := range transcript { // Splits inside every multibyte UTF-8 scalar.
		if _, err := p.Write([]byte{value}); err != nil {
			t.Fatal(err)
		}
	}
	if p.Result().Succeeded {
		t.Fatal("result label became success before EOF finalization")
	}
	if err := p.Finish(); err != nil {
		t.Fatal(err)
	}
	result := p.Result()
	if !result.Terminal || !result.Succeeded || result.Failure != nil || result.FinalResponse == nil || *result.FinalResponse != "\ufeffExact final café 東京 �\r\nlast\n" || len(events) != 5 {
		t.Fatalf("response or trailing evidence lost: %+v events=%d", result, len(events))
	}
	observed := result.Observations
	if observed.ConfiguredModel == nil || *observed.ConfiguredModel.Value != "haiku" || observed.Model.Value == nil || *observed.Model.Value != "reported-main-model" || *observed.Model.Source != "claude.assistant.message.model" {
		t.Fatalf("requested/configured/main models conflated: %+v", observed)
	}
	if observed.Usage.Scope != "top_level" || *observed.Usage.InputTokens != "9007199254740993" || *observed.Usage.CachedInputTokens != "0" || *observed.Usage.CacheCreationInputTokens != "2" {
		t.Fatal("exact top-level counters lost")
	}
	models := observed.NativeModels
	if models == nil || models.Scope != "whole_tree" || models.Completeness != "complete" || len(models.Models) != 2 || models.EvidenceArtifactIDs[0] != claudeStdoutID || models.Models[0].Model != "helper-model" || *models.Models[0].CostEstimateUSD != "1e999" {
		t.Fatalf("whole-tree model preview lost its source/scope/lexemes: %+v", models)
	}
	// Callback and Result snapshots must not share mutable observation containers.
	models.Models[0].Model = "changed-copy"
	if p.Result().Observations.NativeModels.Models[0].Model == "changed-copy" || events[0].Observations.NativeModels != nil {
		t.Fatal("later output or caller mutation rewrote earlier observation evidence")
	}
}

func TestClaudeRejectsAmbiguousOrIncompleteTerminals(t *testing.T) {
	for name, transcript := range map[string]string{
		"missing":       "{\"type\":\"system\",\"subtype\":\"informational\"}\n",
		"missing error": "{\"type\":\"result\",\"session_id\":\"session\",\"subtype\":\"success\",\"result\":\"x\"}\n",
		"wrong result":  "{\"type\":\"result\",\"session_id\":\"session\",\"subtype\":\"success\",\"is_error\":false,\"result\":null}\n",
		"duplicate key": "{\"type\":\"result\",\"type\":\"result\"}\n",
		"repeated":      claudeTerminal("") + claudeTerminal(""),
		"session":       "{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"another\"}\n" + claudeTerminal(""),
		"truncated":     strings.TrimSuffix(claudeTerminal(""), "\n"),
		"surrogate":     claudeTerminal(`,"ignored":"\ud800"`),
		"line limit":    strings.Repeat("x", nativeLineLimit+1),
	} {
		t.Run(name, func(t *testing.T) {
			p, err := finishClaude(t, transcript, nil)
			if err == nil || p.Result().Succeeded || p.Result().Failure == nil {
				t.Fatalf("unsupported output became success: %+v err=%v", p.Result(), err)
			}
		})
	}
}

func TestClaudeNativeFailureCannotBecomeLateSuccess(t *testing.T) {
	for _, transcript := range []string{
		strings.Replace(claudeTerminal(""), `"is_error":false`, `"is_error":true`, 1),
		claudeTerminal("") + "{\"type\":\"assistant\",\"error\":\"authentication_failed\"}\n",
		"{\"type\":\"assistant\",\"error\":\"rate_limit\"}\n" + claudeTerminal(""),
		"{\"type\":\"result\",\"session_id\":\"session\",\"subtype\":\"error_during_execution\",\"is_error\":true}\n",
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"session\",\"claude_code_version\":\"2.1.258\"}\n" + claudeTerminal(""),
	} {
		p, _ := finishClaude(t, transcript, nil)
		if p.Result().Succeeded || p.Result().Failure == nil {
			t.Fatal("native failure became authoring success")
		}
		select {
		case <-p.Failed():
		default:
			t.Fatal("native failure was not observable during execution")
		}
	}
	p, err := finishClaude(t, strings.Replace(claudeTerminal(""), `"result":"response"`, `"result":""`, 1), nil)
	if err != nil || !p.Result().Succeeded || p.Result().FinalResponse == nil || *p.Result().FinalResponse != "" {
		t.Fatal("valid empty native text was conflated with missing output; candidate usability is the engine's decision")
	}
}

func TestClaudeKnownMainIdentityCannotBeReinterpreted(t *testing.T) {
	for _, kind := range []string{"result", "system", "assistant"} {
		for _, parent := range []string{"0", `{}`, `[]`, `""`, "false"} {
			object := map[string]jsontext.Value{"type": jsontext.Value(`"` + kind + `"`), "parent_tool_use_id": jsontext.Value(parent), "session_id": jsontext.Value(`"session"`), "subtype": jsontext.Value(`"success"`), "is_error": jsontext.Value(`false`), "result": jsontext.Value(`"response"`)}
			if kind == "system" {
				object["subtype"] = jsontext.Value(`"init"`)
			}
			encoded, _ := json.Marshal(object)
			p, err := finishClaude(t, string(encoded)+"\n"+claudeTerminal(""), nil)
			if err == nil || p.Result().Succeeded {
				t.Fatalf("invalid parent became a main message: type=%s parent=%s", kind, parent)
			}
		}
	}
	for _, kind := range []string{"system", "user", "tool_progress", "rate_limit_event", "stream_event", "auth_status"} {
		p, err := finishClaude(t, claudeTerminal("")+`{"type":"`+kind+`","session_id":"different","subtype":"informational"}`+"\n", nil)
		if err == nil || p.Result().Succeeded {
			t.Fatalf("recognized trailing message changed main session: %s", kind)
		}
	}
	// Unknown additive messages do not establish a conversation or model. Their
	// future parent shape has no authority and remains independently raw evidence.
	p, err := finishClaude(t, `{"type":"future_observation","parent_tool_use_id":{},"session_id":"not-an-owned-session","model":"not-observed"}`+"\n"+claudeTerminal(""), nil)
	if err != nil || !p.Result().Succeeded || p.Result().Observations.Model.Status != "unknown" || *p.Result().Observations.NativeSession.Value != "session" {
		t.Fatal("unknown additive message acquired identity authority")
	}
}

func TestClaudeModelAmbiguityAndMalformedOptionalCountersStayHonest(t *testing.T) {
	for _, prefix := range []string{
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"session\",\"model\":\"setting-only\"}\n",
		"{\"type\":\"assistant\",\"message\":{\"model\":\"unattributed\"}}\n",
		"{\"type\":\"assistant\",\"parent_tool_use_id\":null,\"message\":{\"model\":\"a\"}}\n{\"type\":\"assistant\",\"parent_tool_use_id\":null,\"message\":{\"model\":\"b\"}}\n",
	} {
		p, err := finishClaude(t, prefix+claudeTerminal(`,"usage":{"input_tokens":1e3,"output_tokens":9223372036854775808},"modelUsage":{"helper":{"inputTokens":"12","outputTokens":7,"costUSD":-1}}`), nil)
		if err != nil || !p.Result().Succeeded || p.Result().Observations.Model.Status != "unknown" {
			t.Fatal("optional telemetry invented a primary identity or blocked usable native output")
		}
		observed := p.Result().Observations
		if observed.Usage.Completeness != "partial" || observed.Usage.InputTokens != nil || observed.NativeModels.Completeness != "partial" || observed.NativeModels.Models[0].InputTokens != nil || observed.NativeModels.Models[0].CostEstimateUSD != nil {
			t.Fatal("malformed optional observations became measured facts")
		}
	}
}

func TestClaudeOversizedInitFactsKeepBoundedRawEvidenceLink(t *testing.T) {
	for _, field := range []string{"model", "claude_code_version", "both"} {
		t.Run(field, func(t *testing.T) {
			init := map[string]any{"type": "system", "subtype": "init", "session_id": "session"}
			for _, key := range []string{"model", "claude_code_version"} {
				if field == key || field == "both" {
					init[key] = strings.Repeat("x", domain.MaxNativeIdentityBytes+1)
				}
			}
			encoded, err := json.Marshal(init)
			if err != nil {
				t.Fatal(err)
			}
			var events []Event
			p, err := finishClaude(t, string(encoded)+"\n"+claudeTerminal(""), func(event Event) error {
				events = append(events, event)
				return nil
			})
			if err != nil || !p.Result().Succeeded || len(events) != 2 {
				t.Fatalf("optional identity omission blocked valid native output: events=%d err=%v", len(events), err)
			}
			observed := p.Result().Observations
			if observed.NativeModels != nil {
				t.Fatal("init omission fabricated model-usage provenance")
			}
			for _, fact := range []*domain.ObservedFact{observed.ConfiguredModel, observed.NativeVersion} {
				if fact != nil && (fact.Status != "unknown" || fact.Value != nil) {
					t.Fatal("oversized optional identity was truncated or promoted")
				}
			}
			for _, event := range events {
				var metadata struct {
					Omitted             bool     `json:"optionalObservationOmitted"`
					EvidenceArtifactIDs []string `json:"evidenceArtifactIds"`
				}
				if err := json.Unmarshal(event.Metadata, &metadata); err != nil || !metadata.Omitted || !reflect.DeepEqual(metadata.EvidenceArtifactIDs, []string{claudeStdoutID}) {
					t.Fatalf("optional identity omitted without its raw stdout link: %s err=%v", event.Metadata, err)
				}
				frame, err := json.Marshal(event)
				if err != nil || len(frame) > domain.MaxEventBytes {
					t.Fatalf("omission callback exceeded its bounded frame: bytes=%d err=%v", len(frame), err)
				}
			}
		})
	}
}

func TestClaudePreviewBudgetKeepsExactOmissionAndRawEvidence(t *testing.T) {
	models := map[string]any{}
	for index := range 20 {
		// Escaping makes a byte-valid identity expensive in serialized metadata.
		name := strings.Repeat("\x01", 240) + string(rune('a'+index))
		models[name] = map[string]any{"inputTokens": jsontext.Value("9007199254740993"), "outputTokens": 0}
	}
	encoded, _ := json.Marshal(models)
	var maxEvent int
	p, err := finishClaude(t, claudeTerminal(`,"modelUsage":`+string(encoded)), func(event Event) error {
		body, _ := json.Marshal(event)
		if len(body) > maxEvent {
			maxEvent = len(body)
		}
		return nil
	})
	if err != nil || !p.Result().Succeeded {
		t.Fatal(err)
	}
	preview := p.Result().Observations.NativeModels
	kept := int64(len(preview.Models))
	omitted, parseErr := domain.ParseCount(*preview.OmittedModelCount)
	if parseErr != nil || kept > 16 || kept+omitted != 20 || preview.Completeness != "partial" || len(preview.EvidenceArtifactIDs) != 1 || preview.EvidenceArtifactIDs[0] != claudeStdoutID || maxEvent > domain.MaxEventBytes {
		t.Fatalf("preview omitted facts without bounded provenance: %+v event=%d", preview, maxEvent)
	}
}

func TestClaudeFakeFactsAndPersistenceFaultsNeverLookMeasured(t *testing.T) {
	golden, _ := os.ReadFile("testdata/claude/success.jsonl")
	transcript := strings.ReplaceAll(string(golden), `"council_test_fixture":true`, `"council_fixture":true`)
	p, err := finishClaude(t, transcript, nil)
	if err != nil || !p.Result().Succeeded {
		t.Fatal(err)
	}
	observed := p.Result().Observations
	if observed.Model.Status != "unknown" || observed.NativeSession.Status != "unknown" || observed.ConfiguredModel != nil || observed.NativeVersion != nil || observed.NativeModels != nil || observed.Usage.InputTokens != nil {
		t.Fatal("fixture protocol facts became actual native usage/model/version/session")
	}
	p, err = finishClaude(t, claudeTerminal(""), func(Event) error { return errors.New("authored persistence fault") })
	if err == nil || p.Result().Succeeded || p.Result().Failure.Code != "evidence_failed" {
		t.Fatal("callback persistence failure became successful authoring")
	}
}
