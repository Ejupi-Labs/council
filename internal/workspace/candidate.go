package workspace

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

// Candidate retains both bytes and index-only changes against captured source.
// A capture fault returns an attributable partial candidate when evidence can
// still be persisted; callers must retain the fault as a failed observation.
func (m *Manager) Candidate(ctx context.Context, source *Snapshot) (*Candidate, error) {
	if source == nil || !validManagedID(source.ID) {
		return nil, errors.New("Invalid managed source identity.")
	}
	if err := m.lock.CheckHeld(); err != nil {
		return nil, err
	}
	dir := "workspaces/" + source.ID
	expected := filepath.Join(m.lock.Root(), filepath.FromSlash(dir))
	if source.WorkspacePath != expected {
		return nil, errors.New("Managed source reference changed.")
	}
	i, err := m.root.Lstat(dir)
	if err != nil || !i.IsDir() {
		return nil, fmt.Errorf("%w Managed source directory is unavailable.", ErrBlocked)
	}
	r, err := m.root.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	controlInfo, controlErr := r.Lstat(".git")
	if controlErr != nil || !controlInfo.Mode().IsRegular() || controlInfo.Size() > 4096 {
		return nil, fmt.Errorf("%w Managed Git control changed.", ErrBlocked)
	}
	control, controlErr := r.ReadFile(".git")
	if controlErr != nil || source.GitControlSHA256 == "" || hashBytes(control) != source.GitControlSHA256 {
		return nil, fmt.Errorf("%w Managed Git control changed.", ErrBlocked)
	}
	idx, primary := m.index(ctx, expected)
	after, err := m.inventory(ctx, source.AttemptID, expected, r, idx, true, source.Manifest.Entries)
	if primary == nil {
		primary = err
	}
	changes := delta(source.Manifest, after)
	id, err := evidence.NewID()
	if err != nil {
		return nil, err
	}
	manifestBytes, err := json.Marshal(after)
	if err != nil {
		return nil, err
	}
	ma, err := m.artifacts.Put(ctx, source.AttemptID, "candidate_manifest", manifestBytes)
	if err != nil {
		return nil, err
	}
	deltaBytes, err := json.Marshal(struct {
		SourceSnapshotID string   `json:"sourceSnapshotId"`
		Changes          []Change `json:"changes"`
		Partial          bool     `json:"partial"`
	}{source.ID, changes, primary != nil})
	if err != nil {
		return nil, err
	}
	da, err := m.artifacts.Put(ctx, source.AttemptID, "candidate_delta", deltaBytes)
	if err != nil {
		return nil, err
	}
	return &Candidate{ID: id, SourceSnapshotID: source.ID, WorkspacePath: expected, ManifestArtifactID: ma.ID, DeltaArtifactID: da.ID, Changes: changes, Verification: "pending", Integration: "not_performed", Partial: primary != nil}, primary
}

func validManagedID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}

func delta(before, after Manifest) []Change {
	bm, am := map[string]Entry{}, map[string]Entry{}
	bi, ai := map[string]IndexEntry{}, map[string]IndexEntry{}
	paths := map[string]bool{}
	for _, v := range before.Entries {
		bm[v.Path] = v
		paths[v.Path] = true
	}
	for _, v := range after.Entries {
		am[v.Path] = v
		paths[v.Path] = true
	}
	for _, v := range before.Index {
		bi[v.Path] = v
		paths[v.Path] = true
	}
	for _, v := range after.Index {
		ai[v.Path] = v
		paths[v.Path] = true
	}
	names := make([]string, 0, len(paths))
	for p := range paths {
		names = append(names, p)
	}
	sort.Strings(names)
	out := []Change{}
	for _, p := range names {
		b, bok := bm[p]
		a, aok := am[p]
		b.ArtifactID = ""
		a.ArtifactID = ""
		ib, ibok := bi[p]
		ia, iaok := ai[p]
		workChanged := bok != aok || b != a
		indexChanged := ibok != iaok || ib != ia
		if !workChanged && !indexChanged {
			continue
		}
		c := Change{Path: p, Kind: "modified"}
		if !bok && aok {
			c.Kind = "added"
		}
		if bok && !aok {
			c.Kind = "deleted"
		}
		if !workChanged && indexChanged {
			c.Kind = "index_only"
		}
		if bok {
			v := bm[p]
			c.Before = &v
		}
		if aok {
			v := am[p]
			c.After = &v
		}
		if ibok {
			v := ib
			c.BeforeIndex = &v
		}
		if iaok {
			v := ia
			c.AfterIndex = &v
		}
		out = append(out, c)
	}
	return out
}
