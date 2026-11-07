package localstate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/localpath"
)

var ErrGraphIntentUnresolved = errors.New("The retained graph request cannot be verified. Preserve it and retry its exact original identity and input.")

// Two bounded request bodies plus framing; no artifact/transcript is stored here.
const MaxGraphIntentBytes = (64 << 20) + (64 << 10)

// GraphIntent owns a per-request native lock, distinct from the execution-owner
// lock. Existing markers/bodies are immutable and never repaired or overwritten.
type GraphIntent struct {
	requestID string
	paths     []string
	roots     []*os.Root
	lock      *os.File
}

func validGraphIntentID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func OpenGraphIntent(ctx context.Context, path, requestID string) (result *GraphIntent, operationErr error) {
	if !validGraphIntentID(requestID) || !filepath.IsAbs(path) {
		return nil, ErrGraphIntentUnresolved
	}
	info, err := os.Lstat(path)
	if err != nil || !graphIntentOrdinary(info, true) {
		return nil, ErrGraphIntentUnresolved
	}
	canonical, err := localpath.Resolve(path)
	if err != nil {
		return nil, ErrGraphIntentUnresolved
	}
	resolved, err := os.Lstat(canonical)
	if err != nil || !graphIntentOrdinary(resolved, true) || !os.SameFile(info, resolved) {
		return nil, ErrGraphIntentUnresolved
	}
	if err = verifyPrivate(canonical, true); err != nil {
		return nil, ErrGraphIntentUnresolved
	}
	r, err := os.OpenRoot(canonical)
	if err != nil {
		return nil, ErrGraphIntentUnresolved
	}
	intent := &GraphIntent{requestID: requestID, paths: []string{canonical}, roots: []*os.Root{r}}
	defer func() {
		if operationErr != nil {
			_ = intent.Close()
		}
	}()
	for _, name := range []string{"client-intents", "graphs"} {
		if err = intent.appendDirectory(name); err != nil {
			return nil, ErrGraphIntentUnresolved
		}
	}
	parent := intent.roots[len(intent.roots)-1]
	lockName := requestID + ".lock"
	lockPath := filepath.Join(intent.paths[len(intent.paths)-1], lockName)
	created, err := parent.OpenFile(lockName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		protectErr := ProtectPrivateFile(created, false)
		syncErr := created.Sync()
		closeErr := created.Close()
		if errors.Join(protectErr, syncErr, closeErr, syncDirectory(intent.paths[len(intent.paths)-1])) != nil {
			return nil, ErrGraphIntentUnresolved
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, ErrGraphIntentUnresolved
	}
	lockInfo, err := parent.Lstat(lockName)
	if err != nil || !graphIntentOrdinary(lockInfo, false) {
		return nil, ErrGraphIntentUnresolved
	}
	file, err := graphIntentOpenLock(lockPath)
	if err != nil {
		return nil, ErrGraphIntentUnresolved
	}
	intent.lock = file
	opened, err := file.Stat()
	if err != nil || !os.SameFile(lockInfo, opened) || VerifyPrivateFile(file, false) != nil {
		return nil, ErrGraphIntentUnresolved
	}
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		err = lockNative(file)
		if err == nil {
			break
		}
		if !isLockBusy(err) {
			return nil, ErrGraphIntentUnresolved
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if err = intent.appendDirectory(requestID); err != nil {
		return nil, ErrGraphIntentUnresolved
	}
	if err = intent.check(); err != nil {
		return nil, err
	}
	return intent, nil
}

func graphIntentOrdinary(info os.FileInfo, dir bool) bool {
	return info != nil && info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0 && (dir && info.IsDir() || !dir && info.Mode().IsRegular())
}

func (g *GraphIntent) appendDirectory(name string) error {
	parent := g.roots[len(g.roots)-1]
	path := filepath.Join(g.paths[len(g.paths)-1], name)
	err := parent.Mkdir(name, 0700)
	created := err == nil
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := parent.Lstat(name)
	if err != nil || !graphIntentOrdinary(info, true) {
		return ErrGraphIntentUnresolved
	}
	f, err := graphIntentOpenRead(path, true)
	if err != nil {
		return err
	}
	if created {
		err = ProtectPrivateFile(f, true)
	} else {
		err = VerifyPrivateFile(f, true)
	}
	opened, statErr := f.Stat()
	closeErr := f.Close()
	if err != nil || statErr != nil || closeErr != nil || !os.SameFile(info, opened) {
		return ErrGraphIntentUnresolved
	}
	if created && syncDirectory(g.paths[len(g.paths)-1]) != nil {
		return ErrGraphIntentUnresolved
	}
	r, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	g.paths = append(g.paths, path)
	g.roots = append(g.roots, r)
	return g.check()
}

func (g *GraphIntent) check() error {
	for i, r := range g.roots {
		info, err := os.Lstat(g.paths[i])
		if err != nil || !graphIntentOrdinary(info, true) {
			return ErrGraphIntentUnresolved
		}
		f, err := r.Open(".")
		if err != nil {
			return ErrGraphIntentUnresolved
		}
		opened, statErr := f.Stat()
		privateErr := VerifyPrivateFile(f, true)
		closeErr := f.Close()
		if statErr != nil || privateErr != nil || closeErr != nil || !os.SameFile(info, opened) {
			return ErrGraphIntentUnresolved
		}
	}
	return nil
}

func (g *GraphIntent) read(name string, limit int64) ([]byte, bool, error) {
	if err := g.check(); err != nil {
		return nil, false, err
	}
	r := g.roots[len(g.roots)-1]
	info, err := r.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || !graphIntentOrdinary(info, false) || info.Size() > limit {
		return nil, false, ErrGraphIntentUnresolved
	}
	f, err := graphIntentOpenRead(filepath.Join(g.paths[len(g.paths)-1], name), false)
	if err != nil {
		return nil, false, ErrGraphIntentUnresolved
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || VerifyPrivateFile(f, false) != nil {
		return nil, false, ErrGraphIntentUnresolved
	}
	body, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(body)) > limit || int64(len(body)) != opened.Size() {
		return nil, false, ErrGraphIntentUnresolved
	}
	if err = g.check(); err != nil {
		return nil, false, err
	}
	return body, true, nil
}

// Read distinguishes a fresh identity from retained-but-unavailable intent.
func (g *GraphIntent) Read() ([]byte, bool, error) {
	marker, seen, err := g.read("seen", 64)
	if err != nil {
		return nil, false, err
	}
	body, exists, err := g.read("body.json", MaxGraphIntentBytes)
	if err != nil {
		return nil, false, err
	}
	if !seen && !exists {
		return nil, false, nil
	}
	if !seen || !bytes.Equal(marker, []byte(g.requestID+"\n")) || !exists || len(body) == 0 {
		return nil, true, ErrGraphIntentUnresolved
	}
	return body, true, nil
}

func (g *GraphIntent) publish(name string, body []byte) error {
	if err := g.check(); err != nil {
		return err
	}
	r := g.roots[len(g.roots)-1]
	dir := g.paths[len(g.paths)-1]
	if _, err := r.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return ErrGraphIntentUnresolved
	}
	// Stale partial temporary bytes are retained as unresolved evidence.
	temp := name + ".new"
	f, err := r.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ErrGraphIntentUnresolved
	}
	if err = ProtectPrivateFile(f, false); err == nil {
		_, err = f.Write(body)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return ErrGraphIntentUnresolved
	}
	if err = g.check(); err != nil {
		return err
	}
	if err = graphIntentPublish(filepath.Join(dir, temp), filepath.Join(dir, name)); err != nil {
		return ErrGraphIntentUnresolved
	}
	if err = syncDirectory(dir); err != nil {
		return ErrGraphIntentUnresolved
	}
	return g.check()
}

func (g *GraphIntent) MarkSeen() error { return g.publish("seen", []byte(g.requestID+"\n")) }
func (g *GraphIntent) Write(body []byte) error {
	if len(body) == 0 || len(body) > MaxGraphIntentBytes {
		return ErrGraphIntentUnresolved
	}
	marker, ok, err := g.read("seen", 64)
	if err != nil || !ok || !bytes.Equal(marker, []byte(g.requestID+"\n")) {
		return ErrGraphIntentUnresolved
	}
	return g.publish("body.json", body)
}

func (g *GraphIntent) Close() error {
	var result error
	if g.lock != nil {
		result = errors.Join(unlockNative(g.lock), g.lock.Close())
		g.lock = nil
	}
	for i := len(g.roots) - 1; i >= 0; i-- {
		result = errors.Join(result, g.roots[i].Close())
	}
	g.roots = nil
	return result
}
