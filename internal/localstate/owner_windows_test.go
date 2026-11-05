//go:build windows

package localstate

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func fixtureSID(t *testing.T, value string) *windows.SID {
	t.Helper()
	sid, err := windows.StringToSid(value)
	if err != nil {
		t.Fatal(err)
	}
	return sid
}

func TestOwnerAdmissionUsesExactTokenDefault(t *testing.T) {
	user := fixtureSID(t, "S-1-5-21-111-222-333-1000")
	foreign := fixtureSID(t, "S-1-5-21-111-222-333-1001")
	admins := fixtureSID(t, "S-1-5-32-544")
	system := fixtureSID(t, "S-1-5-18")
	for _, tc := range []struct {
		name                string
		owner, defaultOwner *windows.SID
		attrs               uint32
		want                bool
	}{
		{"current user", user, admins, windows.SE_GROUP_OWNER, true},
		{"exact eligible default owner", admins, admins, windows.SE_GROUP_OWNER, true},
		{"admin membership without default", admins, user, windows.SE_GROUP_OWNER, false},
		{"default group not owner eligible", admins, admins, windows.SE_GROUP_ENABLED, false},
		{"deny-only default group", admins, admins, windows.SE_GROUP_OWNER | windows.SE_GROUP_USE_FOR_DENY_ONLY, false},
		{"unrelated user", foreign, admins, windows.SE_GROUP_OWNER, false},
		{"SYSTEM is not default", system, admins, windows.SE_GROUP_OWNER, false},
		{"SYSTEM cannot be a foreign default-owner fallback", system, system, windows.SE_GROUP_OWNER, false},
		{"absent owner", nil, admins, windows.SE_GROUP_OWNER, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups := []windows.SIDAndAttributes{{Sid: tc.defaultOwner, Attributes: tc.attrs}}
			if got := admissibleOwner(tc.owner, user, tc.defaultOwner, groups); got != tc.want {
				t.Fatalf("owner admission=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestPrivateStateNormalizesNativeTokenDefaultOwner(t *testing.T) {
	user, err := currentSID()
	if err != nil {
		t.Fatal(err)
	}
	defaultOwner, err := tokenDefaultOwner(windows.GetCurrentProcessToken())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	// Nil security attributes use this process token's native default owner. On an
	// elevated Windows runner these fixtures begin owned by Administrators.
	for _, path := range []string{root, filepath.Join(root, "owner.lock"), filepath.Join(root, "workspace.sqlite3")} {
		if path != root {
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		owner, _, err := sd.Owner()
		if err != nil || owner == nil || !owner.Equals(defaultOwner) {
			t.Fatalf("new fixture did not retain its native token default owner: %v", err)
		}
	}
	if defaultOwner.Equals(user) {
		t.Log("native fixtures start owned by the current user")
	} else {
		t.Log("native fixtures start owned by the token-default group")
	}
	lock, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	d := Descriptor{OwnerInstanceID: "11111111111111111111111111111111", PID: os.Getpid(), StartedAt: "2026-11-05T20:00:00.000Z", Origin: "http://127.0.0.1:34567", APIVersion: 1, Token: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	if err := lock.WriteDescriptor(d); err != nil {
		t.Fatal(err)
	}
	defer lock.RemoveOwnDescriptor(d.OwnerInstanceID)
	for _, path := range []string{lock.Root(), filepath.Join(lock.Root(), "owner.lock"), lock.DatabasePath(), filepath.Join(lock.Root(), "owner.json")} {
		if err := verifyPrivate(path, path == lock.Root()); err != nil {
			t.Fatalf("owner was not normalized before private state use: %v", err)
		}
	}
}
