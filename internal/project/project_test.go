package project

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitFixture(t *testing.T, args ...string) []byte {
	t.Helper()
	c := exec.Command("git", args...)
	c.Env = cleanGitEnvironment()
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture git failed: %v %s", err, out)
	}
	return out
}
func repository(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace café 東京")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, "-C", root, "init", "-b", "test-main")
	return root
}

func TestReadOnlyGitMetadataAndPhysicalAliases(t *testing.T) {
	root := repository(t)
	sub := filepath.Join(root, "selected subfolder")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("original\r\nπ\n")
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, "-C", root, "add", "file.txt")
	gitFixture(t, "-C", root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture")
	if err := os.WriteFile(file, []byte("user edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, ".git", "index")
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	headBefore, err := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	// A configured fsmonitor hook is executable if Git invokes it; metadata inspection must not.
	sentinel := filepath.Join(root, "fsmonitor-called")
	hook := filepath.Join(root, ".git", "hooks", "fsmonitor-test")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho called > '"+filepath.ToSlash(sentinel)+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, "-C", root, "config", "core.fsmonitor", hook)
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "hostile"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "unrelated-index"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	a, err := Inspect(context.Background(), "selected subfolder", root)
	if err != nil {
		t.Fatal(err)
	}
	if a.Observation.ResolvedPath != sub || a.Observation.RepositoryRoot != root || a.Observation.Branch == nil || *a.Observation.Branch != "test-main" || a.Observation.Commit == nil {
		t.Fatalf("incorrect metadata: %+v", a)
	}
	b, err := Inspect(context.Background(), filepath.Join(sub, "..", "selected subfolder"), root)
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != b.Identity {
		t.Fatal("ordinary aliases have distinct identity")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(sub, alias); err == nil {
		c, err := Inspect(context.Background(), alias, root)
		if err != nil || c.Identity != a.Identity {
			t.Fatalf("physical alias mismatch: %v", err)
		}
	}
	indexAfter, _ := os.ReadFile(indexPath)
	headAfter, _ := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	textAfter, _ := os.ReadFile(file)
	if !bytes.Equal(indexBefore, indexAfter) || !bytes.Equal(headBefore, headAfter) || string(textAfter) != "user edit\n" {
		t.Fatal("inspection changed source, index or HEAD")
	}
	if _, err := os.Stat(sentinel); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fsmonitor hook was invoked")
	}
}

func TestUnbornDetachedAndIdentityReplacement(t *testing.T) {
	root := repository(t)
	a, err := Inspect(context.Background(), root, filepath.Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	if a.Observation.Commit != nil || a.Observation.Branch == nil {
		t.Fatal("unborn repository metadata incorrect")
	}
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, "-C", root, "add", "file")
	gitFixture(t, "-C", root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture")
	gitFixture(t, "-C", root, "checkout", "--detach")
	b, err := Inspect(context.Background(), root, filepath.Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	if b.Observation.Branch != nil || b.Observation.Commit == nil {
		t.Fatal("detached repository metadata incorrect")
	}
	if err := os.Rename(root, root+"-retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIdentity(root, a.Identity); err == nil {
		t.Fatal("replacement directory silently accepted")
	} else {
		var e *Error
		if !errors.As(err, &e) || e.Code != "project_identity_changed" {
			t.Fatalf("wrong replacement error %v", err)
		}
	}
}

func TestRejectInvalidAndBarePaths(t *testing.T) {
	root := t.TempDir()
	gitFixture(t, "init", "--bare", root)
	for _, tc := range []struct{ path, base string }{{root, filepath.Dir(root)}, {".", "relative-base"}, {"missing", root}, {string([]byte{0xff}), root}, {"\x00", root}} {
		if _, err := Inspect(context.Background(), tc.path, tc.base); err == nil {
			t.Fatalf("accepted invalid selection: %q", tc.path)
		}
	}
}
