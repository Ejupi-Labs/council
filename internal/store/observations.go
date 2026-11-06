package store

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"

	"encoding/json/jsontext"
	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func unknownFact(f domain.ObservedFact) bool {
	return f.Status == "unknown" && f.Value == nil && f.Source == nil && f.ObservedAt == nil
}
func validObservationStamp(value string) bool {
	if len(value) > 64 || !strings.HasSuffix(value, "Z") {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}
func validateOptionalFact(f *domain.ObservedFact) bool {
	if f == nil {
		return true
	}
	if f.Status == "unknown" {
		return unknownFact(*f)
	}
	return f.Status == "observed" && f.Value != nil && *f.Value != "" && len(*f.Value) <= domain.MaxNativeIdentityBytes && utf8.ValidString(*f.Value) && f.Source != nil && *f.Source != "" && len(*f.Source) <= 128 && utf8.ValidString(*f.Source) && f.ObservedAt != nil && validObservationStamp(*f.ObservedAt)
}
func validOptionalCount(value *string) bool {
	if value == nil {
		return true
	}
	_, err := domain.ParseCount(*value)
	return err == nil
}
func validCostEstimate(value *string) bool {
	if value == nil {
		return true
	}
	raw := []byte(*value)
	if len(raw) == 0 || len(raw) > 64 || raw[0] == '-' || !bytes.Equal(raw, bytes.TrimSpace(raw)) {
		return false
	}
	var number jsontext.Value
	return json.Unmarshal(raw, &number) == nil && number.Kind() == '0'
}

// checkedObservations snapshots caller-owned pointers inside the transaction and
// validates only new003 fields and their registered evidence bindings. Historical
// rows are never passed through this function by migration or ordinary reads.
func (s *Store) checkedObservations(ctx context.Context, tx *sql.Tx, a domain.Attempt, input domain.TargetObservations) (domain.TargetObservations, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return domain.TargetObservations{}, domain.Invalid("valid native observations")
	}
	var out domain.TargetObservations
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, domain.Invalid("valid native observations")
	}
	if a.ExecutionMode == "fake" {
		u := out.Usage
		if !unknownFact(out.Provider) || !unknownFact(out.Account) || !unknownFact(out.Model) || !unknownFact(out.NativeSession) || out.ConfiguredModel != nil || out.NativeVersion != nil || out.NativeModels != nil || out.Permissions.NativeReportedMode != nil || u.InputTokens != nil || u.OutputTokens != nil || u.CachedInputTokens != nil || u.CacheCreationInputTokens != nil || u.Scope != "" || u.Source != nil || u.ObservedAt != nil || u.Completeness != "unknown" {
			return out, domain.Invalid("unknown real-world facts for local fake execution")
		}
		return out, nil
	}
	hasNew := out.ConfiguredModel != nil || out.NativeVersion != nil || out.NativeModels != nil || out.Usage.Scope != "" || out.Usage.CacheCreationInputTokens != nil
	if !hasNew {
		return out, nil
	}
	if len(raw) > domain.MaxEventBytes || !validateOptionalFact(out.ConfiguredModel) || !validateOptionalFact(out.NativeVersion) {
		return out, domain.Invalid("bounded native observation fields")
	}
	if out.Usage.Scope != "" && out.Usage.Scope != "top_level" && out.Usage.Scope != "whole_tree" && out.Usage.Scope != "unknown" {
		return out, domain.Invalid("native usage scope")
	}
	if !validOptionalCount(out.Usage.InputTokens) || !validOptionalCount(out.Usage.OutputTokens) || !validOptionalCount(out.Usage.CachedInputTokens) || !validOptionalCount(out.Usage.CacheCreationInputTokens) {
		return out, domain.Invalid("exact native usage counts")
	}
	if out.Usage.Completeness != "unknown" && out.Usage.Completeness != "partial" && out.Usage.Completeness != "complete" {
		return out, domain.Invalid("native usage completeness")
	}
	hasUsage := out.Usage.InputTokens != nil || out.Usage.OutputTokens != nil || out.Usage.CachedInputTokens != nil || out.Usage.CacheCreationInputTokens != nil || out.Usage.Completeness != "unknown" || out.Usage.Source != nil || out.Usage.ObservedAt != nil
	if out.Usage.Scope != "" && out.Usage.Scope != "unknown" && hasUsage {
		if out.Usage.Source == nil || *out.Usage.Source == "" || len(*out.Usage.Source) > 128 || out.Usage.ObservedAt == nil || !validObservationStamp(*out.Usage.ObservedAt) {
			return out, domain.Invalid("sourced native usage scope")
		}
	}
	models := out.NativeModels
	if models == nil {
		return out, nil
	}
	if models.Scope != "whole_tree" || models.Source == "" || len(models.Source) > 128 || !utf8.ValidString(models.Source) || !validObservationStamp(models.ObservedAt) || len(models.Models) > domain.MaxNativeModels || len(models.EvidenceArtifactIDs) != 1 {
		return out, domain.Invalid("bounded whole-tree model preview")
	}
	if models.Completeness != "complete" && models.Completeness != "partial" && models.Completeness != "unknown" {
		return out, domain.Invalid("native model preview completeness")
	}
	if !validOptionalCount(models.OmittedModelCount) || (models.Completeness == "complete" && (models.OmittedModelCount == nil || *models.OmittedModelCount != "0")) {
		return out, domain.Invalid("exact model-preview omissions")
	}
	artifact, err := s.artifact(ctx, tx, models.EvidenceArtifactIDs[0])
	if err != nil || artifact.AttemptID != a.ID || artifact.Kind != "stdout" {
		return out, domain.Invalid("registered stdout evidence for this attempt's model preview")
	}
	previous := ""
	for i, model := range models.Models {
		if model.Model == "" || len(model.Model) > domain.MaxNativeIdentityBytes || !utf8.ValidString(model.Model) || (i > 0 && model.Model <= previous) || !validOptionalCount(model.InputTokens) || !validOptionalCount(model.OutputTokens) || !validOptionalCount(model.CachedInputTokens) || !validOptionalCount(model.CacheCreationInputTokens) || !validCostEstimate(model.CostEstimateUSD) {
			return out, domain.Invalid("bounded exact native-model preview entry")
		}
		previous = model.Model
	}
	return out, nil
}
