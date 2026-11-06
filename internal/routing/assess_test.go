package routing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestAssessmentRequiresWholeBoundedOperationAndRetainsSourceProof(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("\ufeffold café\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, instruction := range []string{`Replace "old" with "new" in notes.md only.`, `IN notes.md ONLY, replace "old" with "new"`} {
		got, err := Assess(context.Background(), root, instruction, domain.TaskVersion{}, "")
		if err != nil || got.Classification != "routine" || got.Source != "recognized_operation" || got.SourceProof == nil || got.SourceProof.PhysicalIdentity == "" || got.SourceProof.ByteLength != "14" {
			t.Fatalf("%q: %#v %v", instruction, got, err)
		}
		if err := RecheckOriginal(context.Background(), root, got); err != nil {
			t.Fatal(err)
		}
		managed := t.TempDir()
		raw, _ := os.ReadFile(filepath.Join(root, "notes.md"))
		if err := os.WriteFile(filepath.Join(managed, "notes.md"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := RecheckManaged(context.Background(), managed, *got.SourceProof); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(managed, "notes.md"), []byte("changed"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := RecheckManaged(context.Background(), managed, *got.SourceProof); err == nil {
			t.Fatal("changed managed bytes accepted")
		}
		if err := os.Rename(filepath.Join(root, "notes.md"), filepath.Join(root, "retained.md")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "notes.md"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := RecheckOriginal(context.Background(), root, got); err == nil {
			t.Fatal("replacement source identity accepted")
		}
		if err := os.Remove(filepath.Join(root, "notes.md")); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(root, "retained.md"), filepath.Join(root, "notes.md")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAssessmentConservativeUnknownAndDeclaredAttribution(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "notes.md"), []byte("old"), 0600)
	instructions := []string{`Replace "old" with "new" in notes.md only; then publish.`, `Replace "old" with "new" in ../notes.md only`, `Replace "old" with "new" in .git/notes.md only`, `Replace "old" with "new" in .council/notes.md only`, `Replace "old" with "new" in ~/.ssh/notes.md only`, `Replace "old" with "new" in C:/notes.md only`, `Replace "old" with "new" in notes.md:stream only`, `Replace "old" with "new" in *.md only`, `Replace "old" with "new" in missing.md only`, "short task"}
	for _, instruction := range instructions {
		got, err := Assess(context.Background(), root, instruction, domain.TaskVersion{}, "")
		if err != nil || got.Classification != "unknown" || got.SourceProof != nil {
			t.Fatalf("%q: %#v %v", instruction, got, err)
		}
	}
	for _, version := range []domain.TaskVersion{{GuidanceText: "check it"}, {Acceptance: domain.TaskAcceptance{ConditionsText: "publish"}}, {Acceptance: domain.TaskAcceptance{RequiredEvidence: []string{"review"}}}} {
		got, err := Assess(context.Background(), root, `Replace "old" with "new" in notes.md only`, version, "")
		if err != nil || got.Classification != "unknown" {
			t.Fatalf("constraints ignored %#v %v", got, err)
		}
	}
	got, err := Assess(context.Background(), "does-not-exist", "a broad task", domain.TaskVersion{}, "routine")
	if err != nil || got.Classification != "routine" || got.Source != "user_declared" || got.SourceProof != nil {
		t.Fatalf("hint %#v %v", got, err)
	}
	os.WriteFile(filepath.Join(root, "large.md"), []byte(strings.Repeat("x", (1<<20)+1)), 0600)
	got, err = Assess(context.Background(), root, `Replace "old" with "new" in large.md only`, domain.TaskVersion{}, "")
	if err != nil || got.Classification != "unknown" {
		t.Fatalf("large source %#v %v", got, err)
	}
}

func TestAssessmentRejectsHardlinksAndRedirects(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "sentinel.md")
	if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, filepath.Join(root, "hard.md")); err != nil {
		t.Fatal(err)
	}
	got, err := Assess(context.Background(), root, `Replace "old" with "new" in hard.md only`, domain.TaskVersion{}, "")
	if err != nil || got.Classification != "unknown" {
		t.Fatalf("hardlink read %#v %v", got, err)
	}
	// Symlink creation requires privileges on some Windows hosts. The hardlink
	// case above remains mandatory; the platform-specific junction test covers Windows.
	if linkErr := os.Symlink(outside, filepath.Join(root, "alias")); linkErr == nil {
		got, err := Assess(context.Background(), root, `Replace "old" with "new" in alias/sentinel.md only`, domain.TaskVersion{}, "")
		if err != nil || got.Classification != "unknown" {
			t.Fatalf("redirect read %#v %v", got, err)
		}
	}
}
