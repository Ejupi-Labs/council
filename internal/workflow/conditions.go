package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"sort"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Conditions captures full-material buckets; it never splits prose with a model.
func Conditions(original string, version domain.TaskVersion, input domain.WorkflowInput, definitions []domain.CheckDefinition) (domain.ConditionRegistry, *domain.Error) {
	registry := domain.ConditionRegistry{}
	seen := map[string]bool{}
	add := func(key, kind, source string, refs ...string) {
		if !seen[key] {
			seen[key] = true
			registry = append(registry, domain.ConditionDefinition{Key: key, Kind: kind, Source: source, EvidenceRefs: refs})
		}
	}
	add("task-instructions", "instructions", "task", "original", "guidance")
	if version.Acceptance.ConditionsText != "" {
		add("task-conditions", "conditions", "task", "acceptance")
	}
	for _, required := range version.Acceptance.RequiredEvidence {
		key := map[string]string{"check": "required-check", "independent_review": "independent-review", "local_integration": "local-integration", "external_delivery": "external-delivery"}[required]
		if key == "" {
			return nil, domain.Invalid("known required evidence")
		}
		add(key, "required_evidence", "task", "acceptance")
	}
	if input.Mode != "author_only" {
		add("workflow-verification", "verification", "workflow", "workflow")
	}
	if input.Review == "required" {
		add("workflow-review", "independent_review", "workflow", "workflow")
	}
	if input.Delivery != nil {
		if input.Delivery.LocalMode == "apply_verified_local" {
			add("delivery-local", "local_integration", "task", "delivery")
		}
		if input.Delivery.ExternalAction != nil {
			add("delivery-external", "external_delivery", "task", "delivery")
		}
	}
	for _, definition := range definitions {
		if !definition.Required {
			continue
		}
		if !validKey(definition.Key) || seen[definition.Key] || reservedConditionKey(definition.Key) {
			return nil, domain.Invalid("unique nonreserved check condition key")
		}
		add(definition.Key, "command_completion", definition.Source, definition.Key)
	}
	if len(registry) > 32 {
		return nil, domain.Fail("input_too_large", "The captured condition registry exceeds 32 entries.")
	}
	sort.Slice(registry, func(i, j int) bool { return registry[i].Key < registry[j].Key })
	return registry, nil
}

func ConditionDigests(original string, version domain.TaskVersion, registry domain.ConditionRegistry, definitions []domain.CheckDefinition) (string, string, error) {
	ordered := append(domain.ConditionRegistry(nil), registry...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Key < ordered[j].Key })
	checks := append([]domain.CheckDefinition(nil), definitions...)
	sort.Slice(checks, func(i, j int) bool { return checks[i].Key < checks[j].Key })
	type definitionDigest struct {
		Key    string `json:"key"`
		SHA256 string `json:"sha256"`
	}
	digests := make([]definitionDigest, 0, len(checks))
	for _, check := range checks {
		digests = append(digests, definitionDigest{check.Key, check.DefinitionSHA256})
	}
	instructions, err := json.Marshal(struct {
		Original string `json:"originalText"`
		Guidance string `json:"guidanceText"`
	}{original, version.GuidanceText})
	if err != nil {
		return "", "", err
	}
	conditions, err := json.Marshal(struct {
		Acceptance domain.TaskAcceptance    `json:"acceptance"`
		Registry   domain.ConditionRegistry `json:"registry"`
		Checks     []definitionDigest       `json:"checks"`
	}{version.Acceptance, ordered, digests})
	if err != nil {
		return "", "", err
	}
	return digest(instructions), digest(conditions), nil
}

func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func validKey(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func reservedConditionKey(value string) bool {
	for _, prefix := range []string{"task-", "required-", "workflow-", "delivery-"} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return value == "independent-review" || value == "local-integration" || value == "external-delivery"
}
