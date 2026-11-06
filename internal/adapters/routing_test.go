package adapters

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestRoutingLaunchUsesResolutionWithoutRewritingRequestedPins(t *testing.T) {
	for _, target := range []domain.ResolvedTarget{
		{ContextID: "codex-native", ConnectionID: "codex-native", HarnessID: "codex", ModelSelector: "gpt-6-luna"},
		{ContextID: "claude-native", ConnectionID: "claude-native", HarnessID: "claude-code", ModelSelector: "haiku"},
	} {
		t.Run(target.ContextID, func(t *testing.T) {
			original := "\ufeffExact original 東京\r\n$(literal text)"
			v := domain.TaskVersion{RequestedPolicy: domain.Policy{Mode: "autonomous"}, GuidanceText: "New guidance\r\n", Pins: domain.ExecutionPins{PaymentPath: "native"}}
			before := v
			resolved := 0
			launch, err := BuildResolvedNative(context.Background(), original, v, target, "private response path", func(_ context.Context, pins domain.ExecutionPins) (string, error) {
				resolved++
				if pins.HarnessID != target.HarnessID || pins.Model != target.ModelSelector || pins.PaymentPath != v.Pins.PaymentPath || pins.AccountID != "" {
					t.Fatal("resolver did not receive the exact selected native target", pins)
				}
				return "local-fixture-executable", nil
			})
			if err != nil || resolved != 1 || !reflect.DeepEqual(v, before) {
				t.Fatal("resolution failed or changed requested intent", err, resolved, v)
			}
			modelArg := slices.Index(launch.Args, "--model")
			if modelArg < 0 || modelArg+1 >= len(launch.Args) || launch.Args[modelArg+1] != target.ModelSelector {
				t.Fatal("the selected native selector was not passed literally", launch.Args)
			}
			if (launch.Args[0] == "--print") != (target.HarnessID == "claude-code") {
				t.Fatal("native harness was silently substituted", launch.Args)
			}
			var payload struct {
				Original string `json:"originalInstructions"`
				Guidance string `json:"guidance"`
			}
			line := bytes.IndexByte(launch.Prompt, '\n')
			if line < 0 || json.Unmarshal(launch.Prompt[line+1:], &payload) != nil || payload.Original != original || payload.Guidance != v.GuidanceText {
				t.Fatal("routing changed the authoritative instructions")
			}
			if launch.Observations.Model.Status != "unknown" || launch.Observations.Account.Status != "unknown" {
				t.Fatal("selection invented an actual model or login", launch.Observations)
			}
		})
	}
}

func TestRoutingLaunchRejectsConflictsBeforeResolvingAnExecutable(t *testing.T) {
	base := domain.ResolvedTarget{ContextID: "claude-native", ConnectionID: "claude-native", HarnessID: "claude-code", ModelSelector: "haiku"}
	for _, test := range []struct {
		name   string
		pins   domain.ExecutionPins
		target domain.ResolvedTarget
	}{
		{"harness pin", domain.ExecutionPins{HarnessID: "codex"}, base},
		{"model pin", domain.ExecutionPins{Model: "sonnet"}, base},
		{"actual account", domain.ExecutionPins{AccountID: "0123456789abcdef0123456789abcdef"}, base},
		{"paid executor", domain.ExecutionPins{PaymentPath: "paid_api"}, base},
		{"mismatched context", domain.ExecutionPins{}, domain.ResolvedTarget{ContextID: "codex-native", ConnectionID: "claude-native", HarnessID: "claude-code", ModelSelector: "haiku"}},
		{"option-like selector", domain.ExecutionPins{}, domain.ResolvedTarget{ContextID: "claude-native", ConnectionID: "claude-native", HarnessID: "claude-code", ModelSelector: "--output-format=text"}},
		{"invalid selector", domain.ExecutionPins{}, domain.ResolvedTarget{ContextID: "claude-native", ConnectionID: "claude-native", HarnessID: "claude-code", ModelSelector: "model\u0000argument"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := domain.TaskVersion{RequestedPolicy: domain.Policy{Mode: "autonomous"}, Pins: test.pins}
			calls := 0
			_, err := BuildResolvedNative(context.Background(), "Bounded task", v, test.target, "unused", func(context.Context, domain.ExecutionPins) (string, error) {
				calls++
				return "must-not-resolve", nil
			})
			if err == nil || calls != 0 {
				t.Fatal("conflicting routing reached executable resolution", err, calls)
			}
		})
	}
}
