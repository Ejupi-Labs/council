package routing

import (
	"runtime"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const referenceTime = "2026-11-06T00:00:00.000Z"

func stringPointer(value string) *string { return &value }

// Catalog returns detached dated declarations, never observed installation/access facts.
func Catalog() []domain.TargetProfile {
	rows := []struct{ id, context, selector, role, minimum string }{
		{"codex-native-default", "codex-native", "", "native_default", ""},
		{"codex-luna-economy", "codex-native", "gpt-6-luna", "economy", ""},
		{"codex-sol-general", "codex-native", "gpt-6.1-sol", "general", ""},
		{"claude-native-default", "claude-native", "", "native_default", ""},
		{"claude-haiku-economy", "claude-native", "haiku", "economy", "2.1.293"},
		{"claude-sonnet-general", "claude-native", "sonnet", "general", "2.1.284"},
	}
	profiles := make([]domain.TargetProfile, 0, len(rows))
	for _, row := range rows {
		source := domain.ProfileSource{Kind: "documented_profile", Label: "Official native model documentation", URL: "https://learn.chatgpt.com/docs/models", ReadDate: "2026-11-06"}
		if row.context == "claude-native" {
			source.URL = "https://code.claude.com/docs/en/model-config"
		}
		if row.minimum != "" {
			source.MinimumNativeVersion = stringPointer(row.minimum)
		}
		profiles = append(profiles, domain.TargetProfile{ID: row.id, ContextID: row.context, ModelSelector: row.selector, Role: row.role, Enabled: true, Revision: "1", Source: source, UpdatedAt: referenceTime})
	}
	return profiles
}

func Contexts() []domain.NativeContext {
	scope := "unix_process_group"
	if runtime.GOOS == "windows" {
		scope = "windows_job"
	}
	contexts := make([]domain.NativeContext, 0, 2)
	for _, row := range []struct{ id, harness, label, contract string }{
		{"codex-native", "codex", "Codex native context", "codex-exec-jsonl/002-v1"},
		{"claude-native", "claude-code", "Claude Code native context", "claude-print-stream-json/003-v1"},
	} {
		capability := domain.NativeConnectionCapabilities{ConnectionID: row.id, HarnessID: row.harness, Interface: "native_exec", NativeContract: row.contract, VersionSource: "accepted_002_contract", SupportedPolicyModes: []string{"autonomous"}, ModelSelection: "explicit_selector", AccountSelection: "unsupported", SessionContinuity: "unsupported", SessionPersistence: "disabled_for_managed_attempt", CancellationScope: scope, LiveQualification: "unqualified", FakeMode: true,
			Limits: domain.NativeConnectionLimits{DefaultTimeoutSeconds: 1800, MaxTimeoutSeconds: 86400, PromptBytes: domain.MaxPromptBytes, StdoutBytes: 64 << 20, StderrBytes: 16 << 20, JSONLLineBytes: 1 << 20, EventBytes: domain.MaxEventBytes, ChunkBytes: 64 << 10, FinalResponseBytesUpperBound: 64 << 20}}
		if row.id == "claude-native" {
			capability.MinimumVersion, capability.DocumentedVersion = stringPointer("2.1.259"), stringPointer("2.1.318")
			capability.VersionSource = "official_documentation_2026-11-06"
			capability.Limits.FinalResponseBytesUpperBound = 1 << 20
		}
		contexts = append(contexts, domain.NativeContext{ID: row.id, HarnessID: row.harness, ConnectionID: row.id, Label: row.label, Enabled: true, Revision: "1", UpdatedAt: referenceTime, Capabilities: capability})
	}
	return contexts
}

func DefaultPolicy() domain.RoutingPolicy {
	order := make([]string, 0, 6)
	for _, profile := range Catalog() {
		order = append(order, profile.ID)
	}
	return domain.RoutingPolicy{Revision: "1", EconomyOrder: order, UnknownAvailability: "allow", DefaultBudget: domain.RoutingBudgetDefaults{AdmissionLimit: "2", WindowSeconds: 1800}, Continuation: domain.ContinuationPolicy{MaxAutomaticAdmissions: "0", Categories: []string{}}, UpdatedAt: referenceTime}
}

// Tier is declared policy rank; nil is unknown and never an actual-cost assertion.
func Tier(role string) *int {
	if role == "economy" {
		n := 0
		return &n
	}
	if role == "general" {
		n := 1
		return &n
	}
	return nil
}
