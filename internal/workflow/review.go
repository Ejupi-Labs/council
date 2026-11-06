package workflow

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func invalidReview() *domain.Error {
	return domain.Fail("review_unavailable", "The complete structured review has invalid bindings, shape, scope or evidence. Inspect its retained raw result.")
}

// ValidateReview performs no I/O; the controller supplies the registered access
// manifest and independently verifies artifact/source integrity before admission.
func ValidateReview(raw []byte, expected domain.ReviewSubject, conditionsSHA256 string, registry domain.ConditionRegistry, supplied []domain.ReviewInspection, priorFindingIDs []string, allowedPaths ...[]string) (domain.ReviewReport, *domain.Error) {
	var report domain.ReviewReport
	if len(raw) > domain.MaxReviewBytes {
		return report, domain.Fail("input_too_large", "The complete review exceeds 1 MiB.")
	}
	if err := domain.StrictDecode(raw, &report); err != nil {
		return report, invalidReview()
	}
	if !reviewRequiredShape(raw) || !domain.ValidID(expected.ID) || !validHash(expected.SHA256) || !validHash(conditionsSHA256) || expected.Kind != "candidate" && expected.Kind != "response" || report.Subject != expected || report.ConditionsSHA256 != conditionsSHA256 || report.SchemaVersion != domain.ReviewContract {
		return report, invalidReview()
	}
	if len(report.Findings) > 64 || len(report.Gaps) > 64 || len(report.InspectedEvidence) > 64 || len(report.ResolvedFindingIDs) > 64 || len(supplied) == 0 || len(supplied) > 64 {
		return report, invalidReview()
	}
	conditions := map[string]bool{}
	for _, entry := range registry {
		if !validKey(entry.Key) || conditions[entry.Key] {
			return report, invalidReview()
		}
		conditions[entry.Key] = true
	}
	known := map[string]domain.ReviewInspection{}
	for _, entry := range supplied {
		if !reviewRef(entry.Ref) || known[entry.Ref].Ref != "" || !validInspection(entry, expected.Kind) {
			return report, invalidReview()
		}
		known[entry.Ref] = entry
	}
	inspected := map[string]bool{}
	for _, entry := range report.InspectedEvidence {
		captured, ok := known[entry.Ref]
		if !ok || inspected[entry.Ref] || !validInspection(entry, expected.Kind) || captured.Scope != entry.Scope || !sameStrings(captured.Paths, entry.Paths) {
			return report, invalidReview()
		}
		inspected[entry.Ref] = true
	}
	paths := map[string]bool{}
	for _, group := range allowedPaths {
		for _, name := range group {
			if !safeReviewPath(name) {
				return report, invalidReview()
			}
			paths[name] = true
		}
	}
	keys := map[string]bool{}
	blocking := false
	for _, finding := range report.Findings {
		if !validKey(finding.Key) || keys[finding.Key] || finding.Severity != "blocking" && finding.Severity != "advisory" || !reviewText(finding.Summary) || len(finding.Locations) > 64 || !validConditions(finding.ConditionKeys, conditions) || !validRefs(finding.EvidenceRefs, known) || len(finding.Locations) == 0 && len(finding.ConditionKeys) == 0 {
			return report, invalidReview()
		}
		keys[finding.Key] = true
		if finding.Severity == "blocking" {
			blocking = true
		}
		for _, location := range finding.Locations {
			if !safeReviewPath(location.Path) || !paths[location.Path] || location.StartLine < 1 || location.EndLine < location.StartLine || int64(location.EndLine) > 2147483647 {
				return report, invalidReview()
			}
		}
	}
	for _, gap := range report.Gaps {
		if !validKey(gap.Key) || keys[gap.Key] || !reviewText(gap.Reason) || !validConditions(gap.ConditionKeys, conditions) || !validRefs(gap.EvidenceRefs, known) {
			return report, invalidReview()
		}
		keys[gap.Key] = true
	}
	for reference := range known {
		if inspected[reference] {
			continue
		}
		represented := false
		for _, gap := range report.Gaps {
			if slices.Contains(gap.EvidenceRefs, reference) {
				represented = true
			}
		}
		if !represented {
			return report, invalidReview()
		}
	}
	switch report.Outcome {
	case "approved":
		if blocking || len(report.Gaps) != 0 {
			return report, invalidReview()
		}
	case "changes_required":
		if !blocking || len(report.Gaps) != 0 {
			return report, invalidReview()
		}
	case "incomplete":
		if len(report.Gaps) == 0 {
			return report, invalidReview()
		}
	default:
		return report, invalidReview()
	}
	prior := map[string]bool{}
	for _, id := range priorFindingIDs {
		if !domain.ValidID(id) || prior[id] {
			return report, invalidReview()
		}
		prior[id] = true
	}
	resolved := map[string]bool{}
	for _, id := range report.ResolvedFindingIDs {
		if !prior[id] || resolved[id] {
			return report, invalidReview()
		}
		resolved[id] = true
	}
	return report, nil
}

func reviewRef(value string) bool {
	return value != "" && len(value) <= 128 && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
func reviewText(value string) bool {
	return value != "" && len(value) <= 4096 && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
func safeReviewPath(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || path.Clean(value) != value || filepath.IsAbs(value) || strings.ContainsAny(value, "\\:\x00") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") || strings.EqualFold(part, ".council") {
			return false
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}
func validInspection(value domain.ReviewInspection, kind string) bool {
	if value.Paths == nil || len(value.Paths) > 256 {
		return false
	}
	switch value.Scope {
	case "whole_candidate":
		return kind == "candidate" && len(value.Paths) == 0
	case "whole_response":
		return kind == "response" && len(value.Paths) == 0
	case "declared_subset":
		if len(value.Paths) == 0 {
			return false
		}
		seen := map[string]bool{}
		for _, name := range value.Paths {
			if !safeReviewPath(name) || seen[name] {
				return false
			}
			seen[name] = true
		}
		return true
	}
	return false
}
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	left := append([]string{}, a...)
	right := append([]string{}, b...)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}
func validConditions(values []string, known map[string]bool) bool {
	if values == nil || len(values) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !known[value] || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
func validRefs(values []string, known map[string]domain.ReviewInspection) bool {
	if values == nil || len(values) > 64 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if known[value].Ref == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
func objectFields(raw jsontext.Value, keys ...string) (map[string]jsontext.Value, bool) {
	var members map[string]jsontext.Value
	if json.Unmarshal(raw, &members) != nil || len(members) != len(keys) {
		return nil, false
	}
	for _, key := range keys {
		if _, ok := members[key]; !ok {
			return nil, false
		}
	}
	return members, true
}
func objectArray(raw jsontext.Value) []jsontext.Value {
	var values []jsontext.Value
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	return values
}
func reviewRequiredShape(raw []byte) bool {
	top, ok := objectFields(raw, "schemaVersion", "subject", "conditionsSHA256", "outcome", "inspectedEvidence", "findings", "gaps", "resolvedFindingIds")
	if !ok {
		return false
	}
	if _, ok = objectFields(top["subject"], "kind", "id", "sha256"); !ok {
		return false
	}
	for _, entry := range objectArray(top["inspectedEvidence"]) {
		if _, ok = objectFields(entry, "ref", "scope", "paths"); !ok {
			return false
		}
	}
	for _, entry := range objectArray(top["findings"]) {
		finding, valid := objectFields(entry, "key", "severity", "conditionKeys", "locations", "summary", "evidenceRefs")
		if !valid {
			return false
		}
		for _, location := range objectArray(finding["locations"]) {
			if _, ok = objectFields(location, "path", "startLine", "endLine"); !ok {
				return false
			}
		}
	}
	for _, entry := range objectArray(top["gaps"]) {
		if _, ok = objectFields(entry, "key", "conditionKeys", "reason", "evidenceRefs"); !ok {
			return false
		}
	}
	return true
}
