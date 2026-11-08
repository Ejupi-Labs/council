// Package schemas contains individually reviewed MCP input schemas.
// Absence means that the tool must not be advertised or dispatched.
package schemas

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io/fs"
	"sort"
)

// Version identifies the reviewed schema inventory, not an owner capability.
const Version = "008-v1"

//go:embed v1/*.json
var documents embed.FS

var drafts = mustLoad(documents)

// The approved map pins the independently reviewed compact-JSON digests.
// Adding or changing a draft never authorizes a tool by itself.

// Fail closed on corrupt, duplicate, or structurally open root schemas.
func mustLoad(source fs.FS) map[string]json.RawMessage {
	paths, err := fs.Glob(source, "v1/*.json")
	if err != nil || len(paths) == 0 {
		panic("no embedded MCP schemas")
	}
	result := map[string]json.RawMessage{}
	for _, path := range paths {
		data, err := fs.ReadFile(source, path)
		if err != nil {
			panic(err)
		}
		var family map[string]json.RawMessage
		if err := jsonv2.Unmarshal(data, &family); err != nil || len(family) == 0 {
			panic(fmt.Sprintf("invalid MCP schema family %s: %v", path, err))
		}
		for name, raw := range family {
			var root struct {
				Schema     string                     `json:"$schema"`
				Type       string                     `json:"type"`
				Properties map[string]json.RawMessage `json:"properties"`
				Additional *bool                      `json:"additionalProperties"`
			}
			if err := jsonv2.Unmarshal(raw, &root); err != nil || name == "" ||
				root.Schema != "https://json-schema.org/draft/2020-12/schema" ||
				root.Type != "object" || root.Properties == nil || root.Additional == nil || *root.Additional {
				panic(fmt.Sprintf("invalid closed MCP schema %s", name))
			}
			if _, exists := result[name]; exists {
				panic("duplicate MCP schema " + name)
			}
			result[name] = append(json.RawMessage(nil), raw...)
		}
	}
	return result
}

// Lookup exposes only independently reviewed schemas, returning a defensive copy.
func Lookup(name string) (json.RawMessage, bool) {
	expected, ok := approved[name]
	if !ok {
		return nil, false
	}
	value, ok := DraftLookup(name)
	if !ok || digest(value) != expected {
		return nil, false
	}
	return value, true
}

// Names returns approved names in deterministic order.
func Names() []string {
	names := make([]string, 0, len(approved))
	for name := range approved {
		if _, valid := Lookup(name); !valid {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Digest returns the SHA-256 of the compacted approved schema JSON tokens.
func Digest(name string) (string, bool) {
	value, ok := Lookup(name)
	if !ok {
		return "", false
	}
	return digest(value), true
}

// DraftLookup is for review and tests only; never advertise or dispatch it.
func DraftLookup(name string) (json.RawMessage, bool) {
	value, ok := drafts[name]
	if !ok {
		return nil, false
	}
	return append(json.RawMessage(nil), value...), true
}

// DraftNames returns the inventory of unapproved schema definitions for review.
func DraftNames() []string {
	names := make([]string, 0, len(drafts))
	for name := range drafts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DraftDigest identifies the compacted candidate JSON tokens under review.
func DraftDigest(name string) (string, bool) {
	value, ok := DraftLookup(name)
	if !ok {
		return "", false
	}
	return digest(value), true
}

func digest(value []byte) string {
	// Whitespace differs with Git checkout line endings; compacting preserves
	// the exact JSON tokens while making the reviewed digest portable.
	compact := append(jsontext.Value(nil), value...)
	if err := compact.Compact(); err != nil {
		return ""
	}
	sum := sha256.Sum256(compact)
	return hex.EncodeToString(sum[:])
}
