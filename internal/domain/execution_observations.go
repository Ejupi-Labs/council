package domain

const ManagedNativeResourceID = "managed-native"
const MaxNativeModels = 16
const MaxNativeIdentityBytes = 256

// ResolveConnectionID resolves a captured request without rewriting pins/defaults.
// Recognized but unavailable harnesses may be stored in guidance, not dispatched.
func ResolveConnectionID(pins ExecutionPins) (string, *Error) {
	if err := ValidatePins(&pins); err != nil {
		return "", err
	}
	switch pins.HarnessID {
	case "", "codex":
		return "codex-native", nil
	case "claude-code":
		return "claude-native", nil
	default:
		return "", Fail("pin_unavailable", "This preview can execute Codex and Claude Code only. Retain this choice or select a supported harness explicitly.")
	}
}
