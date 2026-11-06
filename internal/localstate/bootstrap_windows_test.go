//go:build windows

package localstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDirectoryVerificationSharesDeleteDuringFirstNormalization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unprotected")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	// Fresh child directories inherit their ACL. The first owner must install
	// Council's protected inheritable DACL before creating tokens or a database.
	if err := verifyPrivate(path, true); err == nil {
		t.Fatal("new fixture unexpectedly already has Council's protected DACL")
	}
	legacy, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	security, openErr := openSecurityFile(path, true)
	if security != nil {
		security.Close()
	}
	legacy.Close()
	if !errors.Is(openErr, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("ordinary Go directory handle did not reproduce the normalization conflict: %v", openErr)
	}
	// Retain the actual verification handle while another first-acquire path
	// normalizes the same directory. Verification must not deny DELETE sharing.
	verification, err := openVerificationFile(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer verification.Close()
	if err := securePrivate(path, true); err != nil {
		t.Fatalf("read-only verification blocked concurrent first normalization: %v", err)
	}
	if err := VerifyPrivateFile(verification, true); err != nil {
		t.Fatal(err)
	}
	if err := verifyPrivate(path, true); err != nil {
		t.Fatal(err)
	}
}
