package workflow

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestChecksCaptureOnlyDirectSupportedConventions(t *testing.T) {
	marker := func(path, text string) CheckMetadata {
		return CheckMetadata{Path: path, Bytes: []byte(text), SHA256: digest([]byte(text))}
	}
	input := CheckInput{Programs: []CheckProgram{{Name: "go", Executable: filepath.Join(t.TempDir(), "go.exe"), PhysicalIdentity: "controlled-go"}, {Name: "cargo", Executable: filepath.Join(t.TempDir(), "cargo.exe"), PhysicalIdentity: "controlled-cargo"}, {Name: "node", Executable: filepath.Join(t.TempDir(), "node.exe"), PhysicalIdentity: "controlled-node"}}, Metadata: []CheckMetadata{marker("go.mod", "module controlled.test\n"), marker("Cargo.toml", "[package]\nname='controlled'\n"), marker("Cargo.lock", "# controlled retained lock\n"), marker("package.json", `{"scripts":{"test":"node --test tests/controlled.test.mjs"}}`)}, TestFiles: []string{"tests/controlled.test.mjs"}}
	definitions, gaps, err := Checks(input)
	if err != nil || len(gaps) != 0 || len(definitions) != 3 {
		t.Fatalf("direct supported conventions unavailable: %+v %+v %v", definitions, gaps, err)
	}
	if !slices.Equal(definitions[0].Command.Arguments, []string{"test", "-count=1", "-json", "-mod=readonly", "./..."}) {
		t.Fatal("Go convention changed", definitions[0])
	}
	input.Metadata = []CheckMetadata{marker("package.json", `{"scripts":{"test":"node --test tests/*.test.mjs && npm run lint"}}`)}
	definitions, gaps, err = Checks(input)
	if err != nil || len(definitions) != 0 || len(gaps) != 1 {
		t.Fatal("shell/glob script was translated or ignored", definitions, gaps, err)
	}
}

func TestExplicitEmptyChecksCannotWaiveExactBuiltinProof(t *testing.T) {
	empty := []domain.CheckDeclaration{}
	definitions, gaps, err := Checks(CheckInput{Original: `Replace "old" with "new" in docs/note.md only`, Options: domain.WorkflowInput{Checks: &empty}})
	if err != nil || len(gaps) != 0 || len(definitions) != 1 || definitions[0].BuiltinID != "literal_document_replace/1" {
		t.Fatal("explicit empty commands waived builtin verification", definitions, gaps, err)
	}
}
