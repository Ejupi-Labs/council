package adapters

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestCodexPreservesFullInputAndNativePolicy(t *testing.T) {
	original := "first\n\"$(no shell)\"\nlast"
	v := domain.TaskVersion{RequestedPolicy: domain.Policy{Mode: "autonomous"}, GuidanceText: "keep all instructions", Pins: domain.ExecutionPins{Model: "small-model"}}
	l, err := BuildCodex(context.Background(), original, v, "response.txt", func(context.Context, domain.ExecutionPins) (string, error) { return "native-executable", nil })
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(l.Prompt, []byte(`first\n\"$(no shell)\"\nlast`)) {
		t.Fatalf("Original input lost: %s", l.Prompt)
	}
	joined := strings.Join(l.Args, " ")
	for _, flag := range []string{"exec", "--json", "--ephemeral", "--sandbox danger-full-access", "--config approval_policy=\"never\"", "--model small-model", "--output-last-message response.txt"} {
		if !strings.Contains(joined, flag) {
			t.Fatal(joined)
		}
	}
	if l.Args[len(l.Args)-1] != "-" || l.Observations.Model.Value != nil {
		t.Fatal("Prompt must use stdin; requested model is not an observation")
	}
	v.RequestedPolicy.Mode = "supervised"
	if _, err = BuildCodex(context.Background(), original, v, "r", nil); err == nil {
		t.Fatal("Unsupported permission admitted")
	}
	v.RequestedPolicy.Mode = "autonomous"
	if _, err = BuildCodex(context.Background(), strings.Repeat("a", domain.MaxPromptBytes), v, "r", nil); err == nil {
		t.Fatal("Oversized assembled input admitted")
	}
}

func TestCodexTerminalAndUnknownObservations(t *testing.T) {
	p := NewCodexParser(domain.ExecutionPins{}, nil)
	transcript := `{"type":"thread.started","thread_id":"native-session"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"done"}}
{"type":"turn.completed","usage":{"input_tokens":9007199254740993,"output_tokens":7,"cached_input_tokens":0}}
`
	for _, b := range []byte(transcript) {
		if _, err := p.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.Finish(); err != nil {
		t.Fatal(err)
	}
	r := p.Result()
	if !r.Terminal || !r.Succeeded || r.Failure != nil || r.Observations.Model.Status != "unknown" || *r.Observations.Usage.InputTokens != "9007199254740993" {
		t.Fatalf("%+v", r)
	}
}

func TestCodexRejectsMalformedAndContradictoryOutput(t *testing.T) {
	for name, transcript := range map[string][]byte{
		"duplicate":       []byte("{\"type\":\"turn.completed\",\"type\":\"turn.failed\"}\n"),
		"unicode":         append([]byte("{\"type\":\""), 0xff, '"', '}', '\n'),
		"truncated":       []byte(`{"type":"turn.completed"}`),
		"line":            []byte(strings.Repeat("x", (1<<20)+1)),
		"double terminal": []byte("{\"type\":\"turn.completed\"}\n{\"type\":\"turn.completed\"}\n"),
	} {
		t.Run(name, func(t *testing.T) {
			p := NewCodexParser(domain.ExecutionPins{}, nil)
			_, err := p.Write(transcript)
			if err == nil {
				err = p.Finish()
			}
			if err == nil || p.Result().Succeeded {
				t.Fatal("Malformed output became success")
			}
		})
	}
	p := NewCodexParser(domain.ExecutionPins{}, nil)
	_, _ = p.Write([]byte("{\"type\":\"turn.failed\",\"error\":{\"message\":\"partial\"}}\n{\"type\":\"turn.completed\"}\n"))
	if p.Result().Failure == nil || p.Result().Succeeded {
		t.Fatal("Late completion erased failure")
	}
}
