// Package localstate owns only the replacement's private v2 state and native owner lock.
package localstate

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/localpath"
)

var ErrLocked = errors.New("another Council owner holds the state lock")

// Lock remains held until Close. The persistent lock file is never removed.
type Lock struct {
	mu   sync.Mutex
	root string
	file *os.File
}

type Descriptor struct {
	OwnerInstanceID string `json:"ownerInstanceId"`
	PID             int    `json:"pid"`
	StartedAt       string `json:"startedAt"`
	Origin          string `json:"origin"`
	APIVersion      int    `json:"apiVersion"`
	Token           string `json:"token"`
}

func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("Cannot locate the current user's home directory.")
	}
	return filepath.Join(home, ".council", "v2"), nil
}

// Acquire establishes privacy before writing state, and ownership before opening a database.
// Ancestor aliases are resolved to one physical root; the state-directory leaf itself must
// remain an ordinary directory, and private state files must remain ordinary files.
func Acquire(root string) (*Lock, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("Council state root must be absolute.")
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, errors.New("Cannot create Council's private v2 directory.")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("Council's v2 root must be an ordinary private directory.")
	}
	resolved, err := localpath.Resolve(root)
	if err != nil {
		return nil, errors.New("Cannot resolve Council's private v2 directory.")
	}
	resolvedInfo, err := os.Lstat(resolved)
	if err != nil || !resolvedInfo.IsDir() || resolvedInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, resolvedInfo) {
		return nil, errors.New("Council's v2 directory changed while resolving its physical location.")
	}
	root = resolved
	if err := securePrivate(root, true); err != nil {
		return nil, fmt.Errorf("Cannot protect Council's v2 directory: %w", err)
	}
	file, err := openPrivate(filepath.Join(root, "owner.lock"))
	if err != nil {
		return nil, errors.New("Cannot open Council's private owner lock.")
	}
	if err := lockNative(file); err != nil {
		file.Close()
		if isLockBusy(err) {
			return nil, ErrLocked
		}
		return nil, errors.New("Cannot acquire Council's native owner lock.")
	}
	l := &Lock{root: root, file: file}
	// SQLite receives an already-private ordinary file, never a symlink or legacy database.
	db, err := openPrivate(l.DatabasePath())
	if err != nil {
		l.Close()
		return nil, errors.New("Cannot prepare Council's private database.")
	}
	if err := db.Close(); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

func openPrivate(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("Private state must be an ordinary file.")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := securePrivate(path, false); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (l *Lock) Root() string         { return l.root }
func (l *Lock) DatabasePath() string { return filepath.Join(l.root, "workspace.sqlite3") }
func (l *Lock) CheckHeld() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return errors.New("Council state ownership is closed.")
	}
	return nil
}
func (l *Lock) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := unlockNative(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		return err
	}
	return closeErr
}

func validateDescriptor(d Descriptor) error {
	identity, err := hex.DecodeString(d.OwnerInstanceID)
	if err != nil || len(identity) != 16 || strings.ToLower(d.OwnerInstanceID) != d.OwnerInstanceID {
		return errors.New("Invalid owner descriptor identity.")
	}
	if d.PID <= 0 || d.APIVersion != 1 || len(d.Token) < 43 || len(d.Token) > 256 {
		return errors.New("Invalid owner descriptor metadata.")
	}
	for _, c := range d.Token {
		if c <= 32 || c >= 127 {
			return errors.New("Invalid owner descriptor token.")
		}
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", d.StartedAt); err != nil {
		return errors.New("Invalid owner start time.")
	}
	u, err := url.Parse(d.Origin)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("Invalid owner origin.")
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || host != "127.0.0.1" || port == "" {
		return errors.New("Invalid owner origin.")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return errors.New("Invalid owner port.")
	}
	return nil
}

// ReadDescriptor verifies privacy before reading the private bearer token. It is not lock proof.
func ReadDescriptor(root string) (Descriptor, error) {
	var d Descriptor
	if err := verifyPrivate(root, true); err != nil {
		return d, errors.New("Council's v2 directory is not private.")
	}
	path := filepath.Join(root, "owner.json")
	if err := verifyPrivate(path, false); err != nil {
		return d, err
	}
	f, err := os.Open(path)
	if err != nil {
		return d, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(data) > 8192 {
		return d, errors.New("Cannot read a valid owner descriptor.")
	}
	if err := json.Unmarshal(data, &d, json.RejectUnknownMembers(true)); err != nil {
		return d, errors.New("Invalid owner descriptor format.")
	}
	if err := validateDescriptor(d); err != nil {
		return Descriptor{}, err
	}
	return d, nil
}

func (l *Lock) WriteDescriptor(d Descriptor) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return errors.New("Council state ownership is closed.")
	}
	if err := validateDescriptor(d); err != nil {
		return err
	}
	if err := verifyPrivate(l.root, true); err != nil {
		return err
	}
	data, err := json.Marshal(d)
	if err != nil {
		return errors.New("Cannot encode owner descriptor.")
	}
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return err
	}
	path := filepath.Join(l.root, "owner-"+hex.EncodeToString(suffix[:])+".tmp")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	if err := securePrivate(path, false); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := atomicReplace(path, filepath.Join(l.root, "owner.json")); err != nil {
		return errors.New("Cannot publish owner descriptor.")
	}
	return syncDirectory(l.root)
}

// RemoveOwnDescriptor never removes a descriptor published by a different owner instance.
func (l *Lock) RemoveOwnDescriptor(instance string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return errors.New("Council state ownership is closed.")
	}
	d, err := ReadDescriptor(l.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if d.OwnerInstanceID != instance {
		return nil
	}
	if err := os.Remove(filepath.Join(l.root, "owner.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(l.root)
}
