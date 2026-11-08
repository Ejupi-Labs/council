package historyout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func fixtureRoot(t *testing.T) (*Root, string) {
	t.Helper()
	lock, err := localstate.Acquire(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	path := lock.Root()
	root, err := Open(lock)
	if err != nil {
		t.Fatalf("open publisher root: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root, path
}
func TestHistoryPublisherUsesExistingRootWithoutProvisioning(t *testing.T) {
	lock, err := localstate.Acquire(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	before, err := os.ReadDir(lock.Root())
	if err != nil {
		t.Fatal(err)
	}
	root, err := Open(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadDir(lock.Root())
	if err != nil || len(before) != len(after) {
		t.Fatal("Open changed private root contents", err)
	}
}
func TestHistoryPublisherFilenameBoundary(t *testing.T) {
	valid := []string{"history-a.json", "history-name-1.json", "history-My_File.json"}
	invalid := []string{"", ".json", "../x.json", `a\b.json`, "file.JSON", "a.json:stream", "CON.json", "com1.json", "NUL.json", "a b.json", "bad.json.", "a/other.json", "owner.json", "HISTORY-a.json", "history-.json"}
	for _, name := range valid {
		if !ValidFilename(name) {
			t.Fatal("rejected valid", name)
		}
	}
	for _, name := range invalid {
		if ValidFilename(name) {
			t.Fatal("accepted invalid", name)
		}
	}
}
func TestHistoryPublisherPermitAndExactExclusiveOutput(t *testing.T) {
	root, path := fixtureRoot(t)
	preview, err := root.Preview("history-one.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Preview
	if err = json.Unmarshal(raw, &decoded); err != nil || decoded != preview {
		t.Fatal("private preview did not round-trip", err)
	}
	payload := []byte(`{"safe":"controlled metadata"}`)
	var calls int
	result, err := root.Publish(context.Background(), decoded, payload, func(context.Context) error { calls++; return nil })
	if err != nil || calls != 1 || !result.PermitAccepted || !result.CreateAttempted || !result.Created || result.BytesWritten != len(payload) || !result.FileFlushed || !result.NameVerified || result.FileIdentity == "" {
		t.Fatal(result, calls, err)
	}
	if runtime.GOOS == "windows" && result.DirectorySync != "unavailable" || runtime.GOOS != "windows" && result.DirectorySync != "synced" {
		t.Fatal("incorrect directory observation", result)
	}
	body, err := os.ReadFile(filepath.Join(path, preview.Name))
	if err != nil || !bytes.Equal(body, payload) {
		t.Fatal(string(body), err)
	}
	if _, err = root.Publish(context.Background(), preview, payload, func(context.Context) error { calls++; return nil }); !errors.Is(err, ErrConflict) || calls != 1 {
		t.Fatal("replay bypassed collision boundary", calls, err)
	}
	if _, err = root.Preview(preview.Name); !errors.Is(err, ErrConflict) {
		t.Fatal("existing output admitted", err)
	}
}
func TestHistoryPublisherNeverCreatesOnPermitFailureOrCancellation(t *testing.T) {
	for _, mode := range []string{"permit_error", "context_after_permit", "wrong_root"} {
		t.Run(mode, func(t *testing.T) {
			root, path := fixtureRoot(t)
			preview, err := root.Preview("history-one.json")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "wrong_root" {
				preview.Root.ExportRoot += "changed"
			}
			calls := 0
			result, err := root.Publish(ctx, preview, []byte("payload"), func(context.Context) error {
				calls++
				if mode == "context_after_permit" {
					cancel()
					return nil
				}
				return errors.New("permit not acknowledged")
			})
			if err == nil || result.CreateAttempted || result.Created {
				t.Fatal(result, err)
			}
			if mode == "wrong_root" && calls != 0 || mode != "wrong_root" && calls != 1 {
				t.Fatal("permit called wrong number of times", calls)
			}
			if _, err = os.Lstat(filepath.Join(path, "history-one.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("no-permit path created a file", err)
			}
		})
	}
}
func TestHistoryPublisherRetainsPartialOrUncertainOutput(t *testing.T) {
	for _, step := range []string{"after_create", "after_write", "before_file_sync", "before_name_verify", "before_directory_sync"} {
		t.Run(step, func(t *testing.T) {
			root, path := fixtureRoot(t)
			preview, err := root.Preview("history-one.json")
			if err != nil {
				t.Fatal(err)
			}
			root.testHook = func(actual string) error {
				if actual == step {
					return errors.New("injected boundary failure")
				}
				return nil
			}
			payload := bytes.Repeat([]byte("x"), 131072)
			result, err := root.Publish(context.Background(), preview, payload, func(context.Context) error { return nil })
			if err == nil || !result.PermitAccepted || !result.Created {
				t.Fatal("uncertain effect not reported", result, err)
			}
			body, readErr := os.ReadFile(filepath.Join(path, "history-one.json"))
			if readErr != nil {
				t.Fatal("partial object was removed", readErr)
			}
			if len(body) != result.BytesWritten || !bytes.Equal(body, payload[:len(body)]) {
				t.Fatal("partial output observation mismatch", len(body), result)
			}
			if step == "after_write" && (len(body) == 0 || len(body) >= len(payload)) {
				t.Fatal("fixture did not stop at a partial write", len(body))
			}
			if _, err = root.Preview("history-one.json"); !errors.Is(err, ErrConflict) {
				t.Fatal("partial output silently replaceable", err)
			}
		})
	}
}
func TestHistoryPublisherConcurrentPreviewsDoNotOverwrite(t *testing.T) {
	root, path := fixtureRoot(t)
	first, err := root.Preview("history-one.json")
	if err != nil {
		t.Fatal(err)
	}
	second, err := root.Preview("history-one.json")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var wg sync.WaitGroup
	out := make(chan error, 2)
	start := make(chan struct{})
	for _, p := range []Preview{first, second} {
		wg.Add(1)
		go func(p Preview) {
			defer wg.Done()
			<-start
			_, err := root.Publish(context.Background(), p, []byte("once"), func(context.Context) error { calls.Add(1); return nil })
			out <- err
		}(p)
	}
	close(start)
	wg.Wait()
	close(out)
	success, conflict := 0, 0
	for err := range out {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 || calls.Load() != 1 {
		t.Fatal(success, conflict, calls.Load())
	}
	body, err := os.ReadFile(filepath.Join(path, "history-one.json"))
	if err != nil || string(body) != "once" {
		t.Fatal(string(body), err)
	}
}

func TestHistoryPublisherTargetAppearsAfterPreview(t *testing.T) {
	root, path := fixtureRoot(t)
	preview, err := root.Preview("history-race.json")
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(path, preview.Name)
	result, err := root.Publish(context.Background(), preview, []byte("ours"), func(context.Context) error { return os.WriteFile(target, []byte("other"), 0600) })
	if err == nil || !result.PermitAccepted || !result.CreateAttempted || result.Created {
		t.Fatal("exclusive create did not refuse a raced target", result, err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "other" {
		t.Fatal("concurrent target was overwritten or removed", string(content), err)
	}
}

func TestHistoryPublisherRejectsChangedRootBeforePermit(t *testing.T) {
	root, path := fixtureRoot(t)
	preview, err := root.Preview("history-root.json")
	if err != nil {
		t.Fatal(err)
	}
	renamed := path + "-moved"
	if err = os.Rename(path, renamed); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("held Windows root denied rename")
		}
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	calls := 0
	result, err := root.Publish(context.Background(), preview, []byte("ours"), func(context.Context) error { calls++; return nil })
	if err == nil || calls != 0 || result.CreateAttempted {
		t.Fatal("changed root reached permit or create", result, calls, err)
	}
	if _, err = os.Lstat(filepath.Join(path, preview.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("new root modified", err)
	}
	if _, err = os.Lstat(filepath.Join(renamed, preview.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old root modified", err)
	}
}

func TestHistoryPublisherDetectsReplacedOutputOrPreventsRename(t *testing.T) {
	root, path := fixtureRoot(t)
	preview, err := root.Preview("history-identity.json")
	if err != nil {
		t.Fatal(err)
	}
	target, moved := filepath.Join(path, preview.Name), filepath.Join(path, "history-moved.json")
	movedOK := false
	root.testHook = func(step string) error {
		if step != "before_name_verify" {
			return nil
		}
		if err := os.Rename(target, moved); err != nil {
			return errors.New("held output denied replacement")
		}
		movedOK = true
		return os.WriteFile(target, []byte("other"), 0600)
	}
	result, err := root.Publish(context.Background(), preview, []byte("ours"), func(context.Context) error { return nil })
	if err == nil || !result.Created || result.NameVerified {
		t.Fatal("replaced output was reported verified", result, err)
	}
	if movedOK {
		old, oldErr := os.ReadFile(moved)
		other, otherErr := os.ReadFile(target)
		if oldErr != nil || otherErr != nil || string(old) != "ours" || string(other) != "other" {
			t.Fatal("uncertain objects not retained", oldErr, otherErr)
		}
	} else {
		body, readErr := os.ReadFile(target)
		if readErr != nil || string(body) != "ours" {
			t.Fatal("held original was not retained", readErr)
		}
	}
}
