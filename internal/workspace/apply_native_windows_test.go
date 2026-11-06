//go:build windows

package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyWindowsNativeMetadataAndExclusivePublication(t *testing.T) {
	fixture := inheritedSecurityCase(t)
	expected, err := applyNativeInfo(fixture.source, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = applyPrepareMetadata(fixture.stage, expected); err != nil {
		t.Fatal("native protected stage preparation", err)
	}
	if err = applyRenameExclusive(nil, "staged.txt", fixture.stage, fixture.parent, "original.txt"); err == nil {
		t.Fatal("native no-replace overwrote original")
	}
	if after := nativeSecurityQuery(t, fixture.source); after.String() != fixture.expected.String() {
		t.Fatal("exclusive refusal changed original metadata")
	}
	if err = applyRenameExclusive(nil, "staged.txt", fixture.stage, fixture.parent, "published-東京-�.txt"); err != nil {
		t.Fatal(err)
	}
	if err = applyRestoreMetadata(fixture.stage, expected); err != nil {
		t.Fatal("final inherited metadata was not exactly restored", err)
	}
	if err = fixture.stage.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(fixture.destination), "published-東京-�.txt"))
	if err != nil || string(data) != "controlled text\r\n" {
		t.Fatal("native publication lost Unicode destination or bytes", err)
	}
}

func TestApplyWindowsNewFileRuleMatchesNativeDestinationCreation(t *testing.T) {
	fixture := inheritedSecurityCase(t)
	reference := filepath.Join(filepath.Dir(fixture.destination), "reference-new-file.txt")
	if err := os.WriteFile(reference, []byte("reference"), 0644); err != nil {
		t.Fatal(err)
	}
	referenceFile, err := os.Open(reference)
	if err != nil {
		t.Fatal(err)
	}
	defer referenceFile.Close()
	actualRule, err := applyNativeInfo(referenceFile, false)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := applyNewFileMetadata(fixture.parent, actualRule.Mode)
	if err != nil {
		t.Fatal("new-file security derivation unavailable", err)
	}
	if !applyMetadataEqual(actualRule, expected) {
		t.Fatalf("declared new-file rule differs from native creation: expected=%s control=%x; native=%s control=%x", expected.Security, expected.Control, actualRule.Security, actualRule.Control)
	}
	if err = applyPrepareMetadata(fixture.stage, expected); err != nil {
		t.Fatal(err)
	}
	if err = applyRenameExclusive(nil, "staged.txt", fixture.stage, fixture.parent, "new-file.txt"); err != nil {
		t.Fatal(err)
	}
	if err = applyRestoreMetadata(fixture.stage, expected); err != nil {
		t.Fatal(err)
	}
}
