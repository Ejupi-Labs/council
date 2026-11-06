package adapters

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// BuildResolvedNative consumes a separately captured route without changing the
// caller's requested task version. The feature is enabled only after its guards.
func BuildResolvedNative(ctx context.Context, original string, version domain.TaskVersion, target domain.ResolvedTarget, outputPath string, resolve Resolver) (Launch, error) {
	if err := domain.ValidatePins(&version.Pins); err != nil {
		return Launch{}, err
	}
	if err := domain.ValidateModelArgument(target.ModelSelector); err != nil {
		return Launch{}, err
	}
	expectedHarness := ""
	switch target.ContextID {
	case "codex-native":
		expectedHarness = "codex"
	case "claude-native":
		expectedHarness = "claude-code"
	}
	if expectedHarness == "" || target.ConnectionID != target.ContextID || target.HarnessID != expectedHarness {
		return Launch{}, domain.Fail("pin_unavailable", "The captured route does not identify one compatible native context. No target was substituted.")
	}
	if (version.Pins.HarnessID != "" && version.Pins.HarnessID != target.HarnessID) || (version.Pins.Model != "" && version.Pins.Model != target.ModelSelector) {
		return Launch{}, domain.Fail("pin_unavailable", "The captured route conflicts with the original literal pins. No target was substituted.")
	}
	if version.Pins.AccountID != "" || version.Pins.PaymentPath == "paid_api" {
		return Launch{}, domain.Fail("pin_unavailable", "The native context cannot guarantee the requested actual account or paid API path.")
	}
	// A detached invocation value supplies argv and resolver inputs. Builders
	// preserve original/guidance/acceptance text; the retained task version and
	// its literal pins are never modified or persisted by this operation.
	invocation := version
	invocation.Pins.HarnessID = target.HarnessID
	invocation.Pins.Model = target.ModelSelector
	return BuildNative(ctx, original, invocation, outputPath, resolve)
}
