package adapters

import (
	"context"
	json "encoding/json/v2"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func BuildNative(ctx context.Context, original string, version domain.TaskVersion, outputPath string, resolve Resolver) (Launch, error) {
	if err := domain.ValidateModelArgument(version.Pins.Model); err != nil {
		return Launch{}, err
	}
	connection, err := domain.ResolveConnectionID(version.Pins)
	if err != nil {
		return Launch{}, err
	}
	if connection == "claude-native" {
		return BuildClaude(ctx, original, version, outputPath, resolve)
	}
	return BuildCodex(ctx, original, version, outputPath, resolve)
}

func NewParser(connectionID string, pins domain.ExecutionPins, stdoutArtifactID string, emit func(Event) error) (Parser, error) {
	switch connectionID {
	case "codex-native":
		return NewCodexParser(pins, emit), nil
	case "claude-native":
		if !domain.ValidID(stdoutArtifactID) {
			return nil, domain.Invalid("registered native stdout artifact identity")
		}
		return NewClaudeParser(pins, stdoutArtifactID, emit), nil
	default:
		return nil, domain.Fail("pin_unavailable", "The requested native connection is unsupported.")
	}
}

// BuildClaude deliberately has no response-file option. Its outputPath parameter
// preserves the common builder signature and is never sent to the native CLI.
func BuildClaude(ctx context.Context, original string, version domain.TaskVersion, _ string, resolve Resolver) (Launch, error) {
	if err := domain.ValidateModelArgument(version.Pins.Model); err != nil {
		return Launch{}, err
	}
	mode := version.RequestedPolicy.Mode
	if version.TaskPolicyOverride != nil {
		mode = *version.TaskPolicyOverride
	}
	if mode != "autonomous" {
		return Launch{}, domain.Fail("permission_unavailable", "This native execution contract supports autonomous full access. Supervised tool approval is unavailable.")
	}
	if version.Pins.HarnessID != "claude-code" {
		return Launch{}, domain.Fail("pin_unavailable", "The requested harness is not supported by the Claude execution contract.")
	}
	if version.Pins.AccountID != "" || version.Pins.PaymentPath == "paid_api" {
		return Launch{}, domain.Fail("pin_unavailable", "Native Claude cannot guarantee the requested account or payment path. Remove that explicit pin to use its existing configuration.")
	}
	payload := struct {
		Original   string                `json:"originalInstructions"`
		Guidance   string                `json:"guidance"`
		Acceptance domain.TaskAcceptance `json:"acceptance"`
	}{original, version.GuidanceText, version.Acceptance}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Launch{}, domain.Invalid("valid task instructions")
	}
	prompt := append([]byte("Complete the programming task described by this JSON object. Preserve the complete original instructions and apply its guidance and acceptance requirements. Work only on the requested task in the supplied managed workspace. Return a useful final response; do not publish externally unless explicitly authorized. Council retains your output as candidate evidence and verifies it separately.\n"), encoded...)
	if len(prompt) > domain.MaxPromptBytes {
		return Launch{}, domain.Fail("input_too_large", "The assembled execution prompt exceeds 256 KiB. Reduce the instructions explicitly; stored input has not been truncated.")
	}
	if resolve == nil {
		resolve = ResolveNative
	}
	executable, err := resolve(ctx, version.Pins)
	if err != nil {
		return Launch{}, err
	}
	args := []string{"--print", "--input-format", "text", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--permission-mode", "bypassPermissions", "--permission-prompts", "none"}
	if version.Pins.Model != "" {
		args = append(args, "--model", version.Pins.Model)
	}
	observed := UnknownObservations(mode)
	configured := "bypassPermissions / permission-prompts none"
	observed.Permissions.ConfiguredMode = &configured
	return Launch{Executable: executable, Args: args, Prompt: prompt, Observations: observed}, nil
}
