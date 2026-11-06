package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/localpath"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type Manager struct {
	lock      *localstate.Lock
	artifacts *evidence.Store
	root      *os.Root
	hooks     string
}

func New(lock *localstate.Lock, artifacts *evidence.Store) (*Manager, error) {
	if lock == nil || artifacts == nil {
		return nil, errors.New("Managed source needs private ownership and evidence storage.")
	}
	if e := lock.CheckHeld(); e != nil {
		return nil, e
	}
	r, e := os.OpenRoot(lock.Root())
	if e != nil {
		return nil, e
	}
	m := &Manager{lock: lock, artifacts: artifacts, root: r, hooks: filepath.Join(lock.Root(), "empty-hooks")}
	for _, dir := range []string{"workspaces", "empty-hooks"} {
		if e = m.privateDir(dir); e != nil {
			r.Close()
			return nil, e
		}
	}
	return m, nil
}
func (m *Manager) Close() error { return m.root.Close() }
func (m *Manager) privateDir(path string) error {
	if e := m.root.Mkdir(path, 0700); e != nil && !errors.Is(e, os.ErrExist) {
		return e
	}
	i, e := m.root.Lstat(path)
	if e != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return errors.New("Managed workspace storage must be an ordinary directory.")
	}
	f, e := m.root.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return localstate.ProtectPrivateFile(f, true)
}

func ioReadBounded(r io.Reader, limit int64) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%w A source file exceeds 64 MiB.", ErrBlocked)
	}
	return b, nil
}
func hashBytes(b []byte) string { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }

func (m *Manager) Capture(ctx context.Context, req Request) (*Snapshot, error) {
	if e := m.lock.CheckHeld(); e != nil {
		return nil, e
	}
	if !filepath.IsAbs(req.ProjectPath) {
		return nil, fmt.Errorf("%w Project path must be absolute.", ErrBlocked)
	}
	project, e := localpath.Resolve(req.ProjectPath)
	if e != nil {
		return nil, fmt.Errorf("%w Project path cannot be resolved.", ErrBlocked)
	}
	repo, e := m.value(ctx, project, "rev-parse", "--show-toplevel")
	if e != nil {
		return nil, e
	}
	repo, e = localpath.Resolve(repo)
	if e != nil || !within(repo, project) {
		return nil, fmt.Errorf("%w Project does not resolve inside its repository.", ErrBlocked)
	}
	if within(repo, m.lock.Root()) || within(m.lock.Root(), repo) {
		return nil, fmt.Errorf("%w Source and Council private state must be separate.", ErrBlocked)
	}
	selected := project
	if req.SelectedSubfolder != "" {
		if !safePath(filepath.ToSlash(req.SelectedSubfolder)) {
			return nil, fmt.Errorf("%w Selected source folder is unsupported.", ErrBlocked)
		}
		selected = filepath.Join(project, req.SelectedSubfolder)
		selected, e = localpath.Resolve(selected)
		if e != nil || !within(project, selected) {
			return nil, fmt.Errorf("%w Selected source folder escapes the project.", ErrBlocked)
		}
	}
	si, e := os.Stat(selected)
	if e != nil || !si.IsDir() {
		return nil, fmt.Errorf("%w Selected source folder is unavailable.", ErrBlocked)
	}
	rel, e := filepath.Rel(repo, selected)
	if e != nil {
		return nil, ErrBlocked
	}
	rel = filepath.ToSlash(rel)
	if e = validateSourceLayout(repo); e != nil {
		return nil, e
	}
	format, e := m.value(ctx, repo, "rev-parse", "--show-object-format")
	if e != nil {
		return nil, e
	}
	if format != "sha1" {
		return nil, fmt.Errorf("%w This Git object format is not supported by current-source capture.", ErrBlocked)
	}
	for tries := 0; tries < 3; tries++ {
		s, e := m.captureOnce(ctx, req, project, repo, rel)
		if errors.Is(e, errRacing) {
			continue
		}
		return s, e
	}
	return nil, fmt.Errorf("%w Source changed during three capture observations; retry after local edits settle.", ErrBlocked)
}

var errRacing = errors.New("source changed during capture")

func (m *Manager) captureOnce(ctx context.Context, req Request, project, repo, selected string) (*Snapshot, error) {
	base, e := m.value(ctx, repo, "rev-parse", "--verify", "HEAD^{commit}")
	if e != nil {
		return nil, fmt.Errorf("%w Unborn or unavailable HEAD cannot be captured.", ErrBlocked)
	}
	branch, e := m.value(ctx, repo, "rev-parse", "--symbolic-full-name", "HEAD")
	if e != nil {
		return nil, e
	}
	ip, e := m.value(ctx, repo, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if e != nil {
		return nil, e
	}
	rawIndex, e := readIndex(ip)
	if e != nil {
		return nil, e
	}
	idx, e := m.index(ctx, repo)
	if e != nil {
		return nil, e
	}
	for _, entry := range idx {
		if excluded(entry.Path) != "" {
			return nil, fmt.Errorf("%w Tracked source includes excluded private state.", ErrBlocked)
		}
	}
	patch, e := m.git(ctx, repo, nil, "diff", "--cached", "--binary", "--full-index", "--no-ext-diff", "--no-textconv", base, "--")
	if e != nil {
		return nil, e
	}
	r, e := os.OpenRoot(repo)
	if e != nil {
		return nil, ErrBlocked
	}
	defer r.Close()
	original, e := r.Stat(".")
	if e != nil {
		return nil, ErrBlocked
	}
	manifest, e := m.inventory(ctx, req.AttemptID, repo, r, idx, true)
	if e != nil {
		return nil, e
	}
	check, e := m.inventory(ctx, req.AttemptID, repo, r, idx, false)
	if e != nil {
		return nil, e
	}
	if !sameManifest(manifest, check) {
		return nil, errRacing
	}
	endIndex, e := readIndex(ip)
	if e != nil || !bytes.Equal(rawIndex, endIndex) {
		return nil, errRacing
	}
	endBase, e := m.value(ctx, repo, "rev-parse", "--verify", "HEAD^{commit}")
	if e != nil || endBase != base {
		return nil, errRacing
	}
	endBranch, e := m.value(ctx, repo, "rev-parse", "--symbolic-full-name", "HEAD")
	if e != nil || endBranch != branch {
		return nil, errRacing
	}
	now, e := os.Stat(repo)
	if e != nil || !os.SameFile(original, now) {
		return nil, errRacing
	}
	id, e := evidence.NewID()
	if e != nil {
		return nil, e
	}
	dir := "workspaces/" + id
	if e = m.privateDir(dir); e != nil {
		return nil, e
	}
	destination := filepath.Join(m.lock.Root(), filepath.FromSlash(dir))
	if _, e = m.git(ctx, repo, nil, "worktree", "add", "--no-checkout", "--detach", destination, base); e != nil {
		return nil, e
	}
	if _, e = m.git(ctx, destination, nil, "read-tree", base); e != nil {
		return nil, e
	}
	if len(patch) > 0 {
		if _, e = m.git(ctx, destination, patch, "apply", "--cached", "--binary", "--whitespace=nowarn", "-"); e != nil {
			return nil, e
		}
	}
	got, e := m.index(ctx, destination)
	if e != nil || !indexEqual(idx, got) {
		return nil, fmt.Errorf("%w Managed staged-index reconstruction did not preserve the original staging state.", ErrBlocked)
	}
	managed, e := m.root.OpenRoot(dir)
	if e != nil {
		return nil, e
	}
	defer managed.Close()
	if e = m.materialize(ctx, managed, manifest); e != nil {
		return nil, e
	}
	final, e := m.inventory(ctx, req.AttemptID, destination, managed, idx, false)
	final.Exclusions = manifest.Exclusions
	if e != nil || !sameManifest(manifest, final) {
		return nil, fmt.Errorf("%w Managed working bytes do not match the captured source.", ErrBlocked)
	}
	indexArtifact, e := m.artifacts.Put(ctx, req.AttemptID, "index_snapshot", rawIndex)
	if e != nil {
		return nil, e
	}
	encoded, e := json.Marshal(manifest)
	if e != nil {
		return nil, e
	}
	manifestArtifact, e := m.artifacts.Put(ctx, req.AttemptID, "source_manifest", encoded)
	if e != nil {
		return nil, e
	}
	control, e := managed.ReadFile(".git")
	if e != nil || len(control) > 4096 {
		return nil, fmt.Errorf("%w Managed Git control could not be retained.", ErrBlocked)
	}
	return &Snapshot{GitControlSHA256: hashBytes(control), ID: id, AttemptID: req.AttemptID, ProjectPath: project, RepositoryPath: repo, SelectedSubfolder: selected, WorkspacePath: destination, ExecutionPath: filepath.Join(destination, filepath.FromSlash(selected)), BaseCommit: base, Branch: branch, ManifestArtifactID: manifestArtifact.ID, IndexArtifactID: indexArtifact.ID, Manifest: manifest}, nil
}

func (m *Manager) inventory(ctx context.Context, attemptID, dir string, r *os.Root, idx []IndexEntry, retain bool, baseline ...[]Entry) (Manifest, error) {
	manifest := Manifest{Entries: []Entry{}, Index: idx, Exclusions: []Exclusion{}}
	list, e := m.git(ctx, dir, nil, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if e != nil {
		return manifest, e
	}
	tracked := map[string]bool{}
	for _, entry := range idx {
		tracked[entry.Path] = true
	}
	paths := map[string]bool{}
	for _, p := range bytes.Split(list, []byte{0}) {
		if len(p) == 0 {
			continue
		}
		path := string(p)
		if !safePath(path) {
			return manifest, fmt.Errorf("%w Unsupported source filename.", ErrBlocked)
		}
		paths[path] = true
	}
	for _, entries := range baseline {
		for _, entry := range entries {
			paths[entry.Path] = true
		}
	}
	if len(paths) > MaxEntries {
		return manifest, fmt.Errorf("%w Source has more than 100000 entries.", ErrBlocked)
	}
	names := make([]string, 0, len(paths))
	for path := range paths {
		names = append(names, path)
	}
	sort.Strings(names)
	var total int64
	for _, path := range names {
		if err := ctx.Err(); err != nil {
			return manifest, err
		}
		if reason := excluded(path); reason != "" {
			manifest.Exclusions = append(manifest.Exclusions, Exclusion{path, reason})
			if tracked[path] {
				return manifest, fmt.Errorf("%w Tracked source includes excluded private state.", ErrBlocked)
			}
			continue
		}
		i, e := r.Lstat(path)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return manifest, fmt.Errorf("%w A source entry cannot be observed safely.", ErrBlocked)
		}
		entry := Entry{Path: path, Mode: uint32(i.Mode().Perm()), Size: i.Size()}
		if i.Mode()&os.ModeSymlink != 0 {
			target, e := r.Readlink(path)
			if e != nil || filepath.IsAbs(target) {
				return manifest, fmt.Errorf("%w Absolute or unreadable source symlink.", ErrBlocked)
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(filepath.FromSlash(path)), target))
			if resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
				return manifest, fmt.Errorf("%w A source symlink points outside the repository.", ErrBlocked)
			}
			ti, e := r.Stat(path)
			if e != nil || !ti.Mode().IsRegular() && !ti.IsDir() {
				return manifest, fmt.Errorf("%w A source symlink has an unsupported target.", ErrBlocked)
			}
			if reason := excluded(filepath.ToSlash(resolved)); reason != "" {
				return manifest, fmt.Errorf("%w A source symlink targets excluded private state.", ErrBlocked)
			}
			entry.Type = "symlink"
			entry.Target = target
			entry.Size = int64(len(target))
			entry.Mode = 0
		} else if i.Mode().IsRegular() {
			if i.Size() > MaxFileBytes {
				return manifest, fmt.Errorf("%w A source file exceeds 64 MiB.", ErrBlocked)
			}
			total += i.Size()
			if total > MaxTotalBytes {
				return manifest, fmt.Errorf("%w Source exceeds 512 MiB.", ErrBlocked)
			}
			f, e := r.Open(path)
			if e != nil {
				return manifest, fmt.Errorf("%w A source file cannot be opened safely.", ErrBlocked)
			}
			j, e := f.Stat()
			if e == nil {
				if volumeErr := sameSourceVolume(r, f); volumeErr != nil {
					f.Close()
					return manifest, volumeErr
				}
			}
			if e != nil || !os.SameFile(i, j) || !j.Mode().IsRegular() {
				f.Close()
				return manifest, errRacing
			}
			data, e := ioReadBounded(f, MaxFileBytes)
			k, statErr := f.Stat()
			f.Close()
			if e != nil {
				return manifest, e
			}
			if statErr != nil || !os.SameFile(i, k) || i.Size() != k.Size() || !i.ModTime().Equal(k.ModTime()) || int64(len(data)) != i.Size() {
				return manifest, errRacing
			}
			entry.Type = "file"
			entry.SHA256 = hashBytes(data)
			if retain {
				a, e := m.artifacts.Put(ctx, attemptID, "source_file", data)
				if e != nil {
					return manifest, e
				}
				entry.ArtifactID = a.ID
			}
		} else {
			return manifest, fmt.Errorf("%w Special source files or unsupported mount entries cannot be captured.", ErrBlocked)
		}
		manifest.Entries = append(manifest.Entries, entry)
	}
	return manifest, nil
}

func sameManifest(a, b Manifest) bool {
	if !indexEqual(a.Index, b.Index) || len(a.Entries) != len(b.Entries) || len(a.Exclusions) != len(b.Exclusions) {
		return false
	}
	for i := range a.Entries {
		x, y := a.Entries[i], b.Entries[i]
		x.ArtifactID = ""
		y.ArtifactID = ""
		if x != y {
			return false
		}
	}
	for i := range a.Exclusions {
		if a.Exclusions[i] != b.Exclusions[i] {
			return false
		}
	}
	return true
}

func (m *Manager) materialize(ctx context.Context, r *os.Root, manifest Manifest) error {
	for _, entry := range manifest.Entries {
		parent := filepath.Dir(filepath.FromSlash(entry.Path))
		if parent != "." {
			if e := r.MkdirAll(parent, 0700); e != nil {
				return e
			}
		}
		if entry.Type == "symlink" {
			if e := r.Symlink(entry.Target, entry.Path); e != nil {
				return fmt.Errorf("%w This platform cannot materialize the captured source symlink.", ErrBlocked)
			}
			continue
		}
		f, e := r.OpenFile(entry.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(entry.Mode)&0777)
		if e != nil {
			return e
		}
		if e = localstate.ProtectPrivateFile(f, false); e != nil {
			f.Close()
			return e
		}
		if _, e = m.artifacts.CopyTo(ctx, entry.ArtifactID, f, MaxFileBytes); e != nil {
			f.Close()
			return e
		}
		if e = f.Sync(); e != nil {
			f.Close()
			return e
		}
		if e = f.Chmod(os.FileMode(entry.Mode) & 0777); e != nil {
			f.Close()
			return e
		}
		if e = f.Close(); e != nil {
			return e
		}
	}
	return nil
}
