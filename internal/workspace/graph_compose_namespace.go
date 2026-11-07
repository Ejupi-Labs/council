package workspace

import (
	json "encoding/json/v2"
	"path/filepath"
	"sort"
	"strings"
)

func graphComposeNamespace(request GraphCompositionRequest, slots []GraphProvenanceSlot, manifest Manifest) []InputConflict {
	byKey := map[string]GraphProvenanceSlot{}
	for _, slot := range slots {
		byKey[graphComposeKey(slot.Dimension, slot.Path)] = slot
	}
	conflicts := []InputConflict{}
	for _, dimension := range []string{"working", "index"} {
		paths := []string{}
		if dimension == "working" {
			for _, e := range manifest.Entries {
				paths = append(paths, e.Path)
			}
		} else {
			for _, e := range manifest.Index {
				paths = append(paths, e.Path)
			}
		}
		sort.Strings(paths)
		present := map[string]bool{}
		for _, path := range paths {
			present[path] = true
		}
		type nativeName struct{ prefix, path string }
		folded := map[string]nativeName{}
		for _, path := range paths {
			for parent := filepath.ToSlash(filepath.Dir(path)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
				if present[parent] {
					origins := append(graphComposeCopyOrigins(byKey[graphComposeKey(dimension, parent)].Origins), byKey[graphComposeKey(dimension, path)].Origins...)
					conflicts = append(conflicts, InputConflict{Code: "path_type_conflict", Dimension: dimension, Path: parent, ContributionRefs: origins, InputID: request.InputID})
					break
				}
			}
			if !request.CaseSensitive {
				prefix := ""
				for _, part := range strings.Split(path, "/") {
					if prefix != "" {
						prefix += "/"
					}
					prefix += part
					key := strings.ToUpper(prefix)
					if prior, ok := folded[key]; ok && prior.prefix != prefix {
						origins := append(graphComposeCopyOrigins(byKey[graphComposeKey(dimension, prior.path)].Origins), byKey[graphComposeKey(dimension, path)].Origins...)
						conflicts = append(conflicts, InputConflict{Code: "native_name_conflict", Dimension: dimension, Path: prefix, ContributionRefs: origins, InputID: request.InputID})
					} else {
						folded[key] = nativeName{prefix, path}
					}
				}
			}
		}
	}
	working := map[string]Entry{}
	directories := map[string]bool{".": true}
	for _, entry := range manifest.Entries {
		working[entry.Path] = entry
		for parent := filepath.ToSlash(filepath.Dir(entry.Path)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			directories[parent] = true
		}
	}
	// Resolve to existing spelling only when the captured native policy is
	// insensitive. Ambiguous folded names remain conflicts, never a choice.
	var nativeNames map[string]string
	if !request.CaseSensitive {
		nativeNames = map[string]string{}
		addName := func(path string) {
			key := strings.ToUpper(path)
			if prior, exists := nativeNames[key]; !exists || prior == path {
				nativeNames[key] = path
			} else {
				nativeNames[key] = ""
			}
		}
		for path := range working {
			addName(path)
		}
		for path := range directories {
			addName(path)
		}
	}
	for _, entry := range manifest.Entries {
		if entry.Type != "symlink" {
			continue
		}
		code, involved := graphComposeResolveSymlink(working, directories, nativeNames, entry.Path)
		if code == "" {
			continue
		}
		origins := []GraphProvenanceOrigin{}
		seenOrigins := map[string]bool{}
		for _, path := range involved {
			for _, origin := range byKey[graphComposeKey("working", path)].Origins {
				if !seenOrigins[origin.OriginSHA256] {
					origins = append(origins, origin)
					seenOrigins[origin.OriginSHA256] = true
				}
			}
		}
		sort.Slice(origins, func(i, j int) bool { return origins[i].OriginSHA256 < origins[j].OriginSHA256 })
		conflicts = append(conflicts, InputConflict{Code: code, Dimension: "working", Path: entry.Path, ContributionRefs: origins, InputID: request.InputID})
	}
	sort.Slice(conflicts, func(i, j int) bool {
		a, b := conflicts[i], conflicts[j]
		if a.Dimension != b.Dimension {
			return a.Dimension < b.Dimension
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Code < b.Code
	})
	return conflicts
}

func graphComposeConflictBounds(conflicts []InputConflict) error {
	body, err := json.Marshal(conflicts)
	if err != nil {
		return err
	}
	if len(body) > 64<<20 {
		return ErrBlocked
	}
	return nil
}

// Resolve components before "..": lexical cleaning before following an ancestor
// symlink changes the actual supported target. The fixed hop bound is local
// validation capacity, never a claim about a native vendor's context capacity.
func graphComposeResolveSymlink(working map[string]Entry, directories map[string]bool, nativeNames map[string]string, source string) (string, []string) {
	entry := working[source]
	stack := []string{}
	parent := filepath.ToSlash(filepath.Dir(source))
	if parent != "." {
		stack = strings.Split(parent, "/")
	}
	pending := strings.Split(filepath.ToSlash(entry.Target), "/")
	involved := []string{source}
	seen := map[string]bool{}
	hops := 0
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(stack) == 0 {
				return "symlink_target_outside", involved
			}
			stack = stack[:len(stack)-1]
			continue
		}
		path := strings.Join(append(append([]string{}, stack...), part), "/")
		if excluded(path) != "" {
			return "symlink_target_excluded", append(involved, path)
		}
		if nativeNames != nil {
			actual, exists := nativeNames[strings.ToUpper(path)]
			if !exists || actual == "" {
				return "symlink_target_unavailable", append(involved, path)
			}
			path = actual
			if excluded(path) != "" {
				return "symlink_target_excluded", append(involved, path)
			}
		}
		current, exists := working[path]
		if exists && current.Type == "symlink" {
			involved = append(involved, path)
			hops++
			if hops > 64 {
				return "symlink_resolution_limit", involved
			}
			stateBytes, _ := json.Marshal(struct {
				Path    string   `json:"path"`
				Pending []string `json:"pending"`
			}{path, pending})
			state := hashBytes(stateBytes)
			if seen[state] {
				return "symlink_cycle", involved
			}
			seen[state] = true
			if filepath.IsAbs(current.Target) {
				return "symlink_target_outside", involved
			}
			pending = append(strings.Split(filepath.ToSlash(current.Target), "/"), pending...)
			continue
		}
		stack = strings.Split(path, "/")
		if len(pending) > 0 && !directories[path] {
			return "symlink_target_unavailable", append(involved, path)
		}
		if !exists && !directories[path] {
			return "symlink_target_unavailable", append(involved, path)
		}
	}
	path := strings.Join(stack, "/")
	if path == "" {
		path = "."
	}
	if _, exists := working[path]; !exists && !directories[path] {
		return "symlink_target_unavailable", append(involved, path)
	}
	return "", involved
}
