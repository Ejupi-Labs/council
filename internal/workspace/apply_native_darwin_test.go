//go:build darwin

package workspace

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestApplyDarwinRefusesNativeAccessACL(t *testing.T) {
	path, directory := applyTestDirectory(t)
	file, err := applyOpenRelative(directory, "acl-file", true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	// The retained descriptor first qualifies the ordinary API-reported no-ACL
	// case. The system tool creates an actual ACL only on this owned fixture.
	if _, err = applyNativeInfo(file, false); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("/bin/chmod", "+a", "everyone allow read", filepath.Join(path, "acl-file")).CombinedOutput()
	if err != nil {
		t.Fatalf("create native ACL fixture: %v: %s", err, output)
	}
	if _, err = applyNativeInfo(file, false); !errors.Is(err, ErrApplyUnsupported) {
		t.Fatalf("native Darwin access ACL was accepted: %v", err)
	}
}
