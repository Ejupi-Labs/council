//go:build darwin || linux

package workspace

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestApplyDarwinACLResponse(t *testing.T) {
	response := func(count uint32, length int) []byte {
		data := make([]byte, 12+length)
		binary.LittleEndian.PutUint32(data[:4], uint32(len(data)))
		binary.LittleEndian.PutUint32(data[4:8], 8)
		binary.LittleEndian.PutUint32(data[8:12], uint32(length))
		if length >= 44 {
			binary.LittleEndian.PutUint32(data[12:16], 0x012cc16d)
			binary.LittleEndian.PutUint32(data[48:52], count)
		}
		return data
	}
	modify := func(data []byte, change func([]byte)) []byte { change(data); return data }
	for _, test := range []struct {
		name       string
		data       []byte
		wantAbsent bool
	}{
		{"reported absence", response(0, 0), true},
		{"explicit no ACL sentinel", response(^uint32(0), 44), true},
		{"explicit empty ACL", response(0, 44), false},
		{"populated ACL", response(1, 68), false},
		{"short result", make([]byte, 11), false},
		{"short security", response(0, 40), false},
		{"truncated response", response(^uint32(0), 44)[:55], false},
		{"unknown magic", modify(response(^uint32(0), 44), func(b []byte) { b[12] ^= 1 }), false},
		{"unknown owner", modify(response(^uint32(0), 44), func(b []byte) { b[16] = 1 }), false},
		{"unknown group", modify(response(^uint32(0), 44), func(b []byte) { b[32] = 1 }), false},
		{"private ACL flags", modify(response(^uint32(0), 44), func(b []byte) { b[52] = 1 }), false},
		{"unknown ACL flags", modify(response(^uint32(0), 44), func(b []byte) { b[55] = 0x80 }), false},
		{"negative relative offset", modify(response(0, 0), func(b []byte) { binary.LittleEndian.PutUint32(b[4:8], ^uint32(0)) }), false},
		{"absolute offset mistaken for relative", modify(response(0, 0), func(b []byte) { binary.LittleEndian.PutUint32(b[4:8], 12) }), false},
		{"trailing bytes", response(^uint32(0), 48), false},
		{"count beyond native maximum", response(129, 44), false},
		{"count and length mismatch", response(1, 44), false},
		{"wrapped length", modify(response(0, 0), func(b []byte) { binary.LittleEndian.PutUint32(b[8:12], ^uint32(0)) }), false},
		{"invalid full size", modify(response(0, 0), func(b []byte) { binary.LittleEndian.PutUint32(b[:4], 8193) }), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := applyParseDarwinACL(test.data)
			if test.wantAbsent && err != nil {
				t.Fatalf("supported absence: %v", err)
			}
			if !test.wantAbsent && !errors.Is(err, ErrApplyUnsupported) {
				t.Fatalf("unpreservable response was accepted: %v", err)
			}
		})
	}
}

func applyTestDirectory(t *testing.T) (string, *os.File) {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory, err := applyOpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { directory.Close() })
	return path, directory
}

func TestApplyUnixTraversesUntouchedStickyAncestorAndChecksCurrentNames(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sticky := filepath.Join(base, "sticky-ancestor")
	leaf := filepath.Join(sticky, "private-leaf")
	if err = os.Mkdir(sticky, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(sticky, 0777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(leaf, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := applyOpenDirectory(leaf)
	if err != nil {
		t.Fatal("untouched sticky ancestor blocked ordinary target", err)
	}
	defer directory.Close()
	if _, err = applyNativeInfo(directory, true); err != nil {
		t.Fatal("ordinary target was not eligible", err)
	}
	file, err := applyOpenRelative(directory, "retained", true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if match, matchErr := applyMatchesName(directory, "retained", file); matchErr != nil || !match {
		t.Fatal("current name did not identify retained file", matchErr)
	}
	if err = os.Rename(filepath.Join(leaf, "retained"), filepath.Join(leaf, "moved")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(leaf, "retained"), []byte("competing object"), 0600); err != nil {
		t.Fatal(err)
	}
	if match, matchErr := applyMatchesName(directory, "retained", file); matchErr != nil || match {
		t.Fatal("competing name identified retained inode", matchErr)
	}
	if match, matchErr := applyMatchesName(directory, "moved", file); matchErr != nil || !match {
		t.Fatal("renamed actual inode was not identifiable", matchErr)
	}
}

func TestApplyUnixRetainedMetadataAndExclusiveMove(t *testing.T) {
	path, directory := applyTestDirectory(t)
	if err := os.WriteFile(filepath.Join(path, "original"), []byte("original bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	original, err := applyOpenRelative(directory, "original", false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	info, err := applyNativeInfo(original, false)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := applyOpenRelative(directory, "stage", true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	if _, err = stage.WriteString("candidate bytes"); err != nil {
		t.Fatal(err)
	}
	info.Mode = 0640
	if err = applyPrepareMetadata(original, info); !errors.Is(err, ErrApplyUnsupported) {
		t.Fatalf("read-only original was admitted for metadata mutation: %v", err)
	}
	if err = applyPrepareMetadata(stage, info); err != nil {
		t.Fatal(err)
	}
	prepared, err := applyNativeInfo(stage, false)
	if err != nil || prepared.Owner != info.Owner || prepared.Group != info.Group || prepared.Mode != 0640 || prepared.Security != "" {
		t.Fatalf("prepared metadata %#v: %v", prepared, err)
	}
	if err = applyRenameExclusive(directory, "stage", stage, directory, "original"); !errors.Is(err, ErrApplyConflict) {
		t.Fatalf("existing name must refuse exclusive publication: %v", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(path, "original")); readErr != nil || string(data) != "original bytes" {
		t.Fatalf("exclusive refusal changed the original: %q, %v", data, readErr)
	}
	if err = applyRenameExclusive(directory, "stage", stage, directory, "published"); err != nil {
		t.Fatal(err)
	}
	published, err := applyOpenRelative(directory, "published", false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer published.Close()
	observed, err := applyNativeInfo(published, false)
	if err != nil || observed.Identity != prepared.Identity || observed.Volume != prepared.Volume || observed.Mode != 0640 {
		t.Fatalf("publication lost retained identity or mode %#v: %v", observed, err)
	}
	if err = applySyncDirectory(directory); err != nil {
		t.Fatal(err)
	}
}

func TestApplyUnixRefusesRedirectHardlinkAndExtendedMetadata(t *testing.T) {
	path, directory := applyTestDirectory(t)
	file, err := applyOpenRelative(directory, "plain", true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = applyNativeInfo(file, false); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("plain", filepath.Join(path, "redirect")); err != nil {
		t.Fatal(err)
	}
	if redirected, openErr := applyOpenRelative(directory, "redirect", false, false); !errors.Is(openErr, ErrApplyUnsupported) {
		if redirected != nil {
			redirected.Close()
		}
		t.Fatalf("symlink was not refused: %v", openErr)
	}
	if err = os.Link(filepath.Join(path, "plain"), filepath.Join(path, "other-link")); err != nil {
		t.Fatal(err)
	}
	if _, err = applyNativeInfo(file, false); !errors.Is(err, ErrApplyUnsupported) {
		t.Fatalf("multi-link file was accepted: %v", err)
	}
	if err = os.Remove(filepath.Join(path, "other-link")); err != nil {
		t.Fatal(err)
	}
	if err = unix.Fsetxattr(int(file.Fd()), "user.council.apply-test", []byte("retained metadata"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err = applyNativeInfo(file, false); !errors.Is(err, ErrApplyUnsupported) {
		t.Fatalf("extended metadata was accepted: %v", err)
	}
}

func TestApplyUnixRefusesCompetingSourceName(t *testing.T) {
	path, directory := applyTestDirectory(t)
	source, err := applyOpenRelative(directory, "source", true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err = os.Rename(filepath.Join(path, "source"), filepath.Join(path, "retained")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(path, "source"), []byte("competing user bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = applyRenameExclusive(directory, "source", source, directory, "destination"); !errors.Is(err, ErrApplyConflict) {
		t.Fatalf("competing inode was moved: %v", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(path, "source")); readErr != nil || string(data) != "competing user bytes" {
		t.Fatalf("source conflict changed user bytes: %q, %v", data, readErr)
	}
	if _, err = os.Stat(filepath.Join(path, "destination")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflict published a destination: %v", err)
	}
}
