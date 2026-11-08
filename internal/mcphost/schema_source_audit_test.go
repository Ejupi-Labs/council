package mcphost

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This deliberately fails when a normative owner input/validation source
// changes. Re-review the affected schemas, rerun owner-parity checks, and then
// update the recorded digest rather than silently relying on stale schema data.
func TestSchemaOwnerSourcesRequireReaudit(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("schemas", "owner-source-digests.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := sha256.Sum256(bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n")))
	if fmt.Sprintf("%x", manifest) != "83ab476893807f982f1265d6193069075d3ed533abb23e3c285ff3ec75ae11bb" {
		t.Fatal("owner source audit inventory changed; review the full inventory and update its pinned digest")
	}
	var sources map[string]string
	if err := json.Unmarshal(body, &sources); err != nil || len(sources) != 38 {
		t.Fatalf("invalid owner source audit inventory: %v", err)
	}
	for name, expected := range sources {
		if !strings.HasPrefix(name, "internal/") || filepath.IsAbs(name) || strings.Contains(name, "..") {
			t.Fatalf("invalid owner source audit path %q", name)
		}
		data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
		if fmt.Sprintf("%x", sum) != expected {
			t.Errorf("%s changed; re-audit its MCP schemas and validators", name)
		}
	}
}
