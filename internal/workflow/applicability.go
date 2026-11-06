package workflow

import (
	json "encoding/json/v2"
	"slices"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type ApplicabilityInput struct {
	Scope                                                   domain.CheckScope
	ConditionKeys                                           []string
	DefinitionSHA256, ConditionsSHA256, SourceControlSHA256 string
	Files                                                   []ProofFile
	Index                                                   []ProofIndex
	Response                                                *domain.ResponseBinding
}

// Basis excludes candidate/workspace/artifact occurrence IDs. Reuse still needs
// a new typed EvidenceUse naming both the inspected subject and this comparison.
func Basis(in ApplicabilityInput) (domain.Applicability, *domain.Error) {
	var out domain.Applicability
	if !validHash(in.DefinitionSHA256) || !validHash(in.ConditionsSHA256) || !validHash(in.SourceControlSHA256) || len(in.Scope.Paths) > 256 || len(in.ConditionKeys) > 32 {
		return out, domain.Invalid("complete applicability basis")
	}
	paths := append([]string{}, in.Scope.Paths...)
	slices.Sort(paths)
	for i, name := range paths {
		if !safeReviewPath(name) || i > 0 && paths[i-1] == name {
			return out, domain.Invalid("unique canonical applicability paths")
		}
	}
	if in.Scope.Kind == "declared_subset" {
		if len(paths) == 0 {
			return out, domain.Invalid("captured nonempty subset scope")
		}
	} else if in.Scope.Kind != "whole_candidate" && in.Scope.Kind != "whole_response" || len(paths) != 0 {
		return out, domain.Invalid("whole or captured subset scope")
	}
	keys := append([]string{}, in.ConditionKeys...)
	slices.Sort(keys)
	for i, key := range keys {
		if !validKey(key) || i > 0 && keys[i-1] == key {
			return out, domain.Invalid("unique applicability condition keys")
		}
	}
	inScope := func(name string) bool {
		if in.Scope.Kind != "declared_subset" {
			return true
		}
		for _, prefix := range paths {
			if name == prefix || strings.HasPrefix(name, prefix+"/") {
				return true
			}
		}
		return false
	}
	type fileBasis struct {
		Path, Type     string
		Mode           uint32
		Size           int64
		SHA256, Target string
	}
	files := []fileBasis{}
	seen := map[string]bool{}
	for _, file := range in.Files {
		if !safeReviewPath(file.Path) || seen[file.Path] || file.Size < 0 || file.Type != "file" && file.Type != "symlink" || file.Type == "file" && !validHash(file.SHA256) {
			return out, domain.Invalid("complete ordinary applicability entries")
		}
		seen[file.Path] = true
		if inScope(file.Path) {
			files = append(files, fileBasis{file.Path, file.Type, file.Mode, file.Size, file.SHA256, file.Target})
		}
	}
	slices.SortFunc(files, func(a, b fileBasis) int { return strings.Compare(a.Path, b.Path) })
	index := []ProofIndex{}
	seen = map[string]bool{}
	for _, entry := range in.Index {
		if !safeReviewPath(entry.Path) || seen[entry.Path] {
			return out, domain.Invalid("unique applicability index entries")
		}
		seen[entry.Path] = true
		if inScope(entry.Path) {
			index = append(index, entry)
		}
	}
	slices.SortFunc(index, func(a, b ProofIndex) int { return strings.Compare(a.Path, b.Path) })
	type responseBasis struct{ SHA256, ByteLength, SourceManifestSHA256, IndexSHA256, ConditionsSHA256 string }
	var response *responseBasis
	if in.Response != nil {
		value := in.Response
		if !validHash(value.ResponseSHA256) || !validHash(value.ManifestSHA256) || !validHash(value.IndexSHA256) || value.ConditionsSHA256 != in.ConditionsSHA256 {
			return out, domain.Invalid("exact response applicability")
		}
		if _, err := domain.ParseCount(value.ResponseByteLength); err != nil {
			return out, err
		}
		response = &responseBasis{value.ResponseSHA256, value.ResponseByteLength, value.ManifestSHA256, value.IndexSHA256, value.ConditionsSHA256}
	}
	if in.Scope.Kind == "whole_response" && response == nil {
		return out, domain.Invalid("response subject applicability")
	}
	encoded, err := json.Marshal(struct {
		Scope                                                   domain.CheckScope
		ConditionKeys                                           []string
		DefinitionSHA256, ConditionsSHA256, SourceControlSHA256 string
		Files                                                   []fileBasis
		Index                                                   []ProofIndex
		Response                                                *responseBasis
	}{domain.CheckScope{Kind: in.Scope.Kind, Paths: paths}, keys, in.DefinitionSHA256, in.ConditionsSHA256, in.SourceControlSHA256, files, index, response})
	if err != nil {
		return out, domain.Invalid("canonical applicability basis")
	}
	return domain.Applicability{ScopeKind: in.Scope.Kind, Paths: paths, ConditionKeys: keys, DefinitionSHA256: in.DefinitionSHA256, BasisSHA256: digest(encoded)}, nil
}

func Applicable(captured, current domain.Applicability) bool {
	return validHash(captured.BasisSHA256) && captured.BasisSHA256 == current.BasisSHA256 && validHash(captured.DefinitionSHA256) && captured.DefinitionSHA256 == current.DefinitionSHA256 && captured.ScopeKind == current.ScopeKind && sameStrings(captured.Paths, current.Paths) && sameStrings(captured.ConditionKeys, current.ConditionKeys)
}
