package mcphost

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

func TestToolCatalogMatchesFixedOwnerRegistry(t *testing.T) {
	tools := ToolCatalog()
	routes := ownerclient.Routes()
	if len(tools) == 0 || len(tools) != len(routes) {
		t.Fatalf("catalog/owner registry differ: %d tools, %d routes", len(tools), len(routes))
	}
	seenNames, seenOps := map[string]bool{}, map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" || seenNames[tool.Name] || seenOps[tool.Operation] {
			t.Fatalf("duplicate or invalid tool mapping: %+v", tool)
		}
		seenNames[tool.Name], seenOps[tool.Operation] = true, true
		if _, ok := ownerclient.RouteFor(tool.Operation); !ok {
			t.Fatalf("tool has no fixed owner route: %+v", tool)
		}
	}
	for _, route := range routes {
		if !seenOps[route.Operation] {
			t.Fatalf("owner route missing from tool catalog: %s", route.Operation)
		}
	}
	tools[0].Name = "changed"
	if ToolCatalog()[0].Name == "changed" {
		t.Fatal("catalog exposed mutable shared storage")
	}
}

// The committed contract is independent of the generated catalog and route
// registry: a coordinated omission or swapped tool mapping must fail.
func TestToolCatalogMatchesCommittedContract(t *testing.T) {
	path := filepath.Join("..", "..", "specs", "008-host-integration", "contracts", "catalog.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile("^\\| `([^`]+)` \\| `([^`]+)` \\|")
	want := map[string]string{}
	for _, line := range strings.Split(string(content), "\n") {
		match := row.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil || !strings.HasPrefix(match[1], "council_") {
			continue
		}
		if _, duplicate := want[match[1]]; duplicate {
			t.Fatalf("duplicate contract tool %s", match[1])
		}
		want[match[1]] = match[2]
	}
	if len(want) != 91 {
		t.Fatalf("unexpected contract inventory: %d", len(want))
	}
	tools := ToolCatalog()
	if len(tools) != len(want) {
		t.Fatalf("catalog/contract count: %d/%d", len(tools), len(want))
	}
	for _, tool := range tools {
		if want[tool.Name] != tool.Operation {
			t.Errorf("%s maps to %q, contract requires %q", tool.Name, tool.Operation, want[tool.Name])
		}
	}
}

func TestDraftSchemaInventoryMatchesCatalog(t *testing.T) {
	tools, names := ToolCatalog(), schemas.DraftNames()
	if len(tools) != len(names) {
		t.Fatalf("catalog has %d tools but %d draft schemas", len(tools), len(names))
	}
	for _, tool := range tools {
		if _, ok := schemas.DraftLookup(tool.Name); !ok {
			t.Errorf("missing draft for %s", tool.Name)
		}
	}
}
