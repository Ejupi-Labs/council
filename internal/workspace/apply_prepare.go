package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

type applyMetadataObservation struct {
	Path, Identity, Volume, Owner, Group, Security string
	Mode, Control                                  uint32
}
type applyDestinationManifest struct {
	Contract, OperationID, SelectedCandidateID, RootSourceSnapshotID, Stage, NamespaceDurability string
	ScopePaths                                                                                   []string
	Entries                                                                                      []Entry
	Index                                                                                        []IndexEntry
	Metadata                                                                                     []applyMetadataObservation
}

func (m *Manager) prepareApplication(ctx context.Context, request ApplyRequest) (application *Application, resultErr error) {
	if request.Source == nil || !validManagedID(request.Source.ID) || !validManagedID(request.AttemptID) || !validManagedID(request.OperationID) || !validManagedID(request.SelectedCandidateID) || !validManagedID(request.SelectedAttemptID) || !validManagedID(request.CandidateManifestArtifactID) || len(request.CandidateManifestSHA256) != 64 || request.ProjectIdentity == "" || request.RepositoryIdentity == "" {
		return nil, ErrApplyUnsupported
	}
	if err := m.lock.CheckHeld(); err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	selectedMetadata, err := m.artifacts.CopyTo(ctx, request.CandidateManifestArtifactID, &encoded, 64<<20)
	if err != nil {
		return nil, err
	}
	if selectedMetadata.AttemptID != request.SelectedAttemptID || selectedMetadata.Kind != "candidate_manifest" || selectedMetadata.SHA256 == nil || *selectedMetadata.SHA256 != request.CandidateManifestSHA256 {
		return nil, evidence.ErrCorrupt
	}
	var selected Manifest
	if err = json.Unmarshal(encoded.Bytes(), &selected, json.RejectUnknownMembers(true)); err != nil {
		return nil, evidence.ErrCorrupt
	}
	if err = validateHandoffManifest(selected); err != nil {
		return nil, err
	}
	if err = validateHandoffManifest(request.Source.Manifest); err != nil {
		return nil, err
	}
	if !indexEqual(request.Source.Manifest.Index, selected.Index) {
		return nil, fmt.Errorf("%w Authored index changes are retained for manual application.", ErrApplyUnsupported)
	}
	changes := CompareManifests(request.Source.Manifest, selected)
	if len(changes) == 0 {
		return nil, fmt.Errorf("%w No deliverable delta exists.", ErrApplyUnsupported)
	}
	application = &Application{manager: m, request: request, paths: []*applicationPath{}, actions: []ApplyAction{}}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, application.Close())
			application = nil
		}
	}()
	application.root, err = applyOpenDirectory(request.Source.RepositoryPath)
	if err != nil {
		return application, err
	}
	rootInfo, err := applyNativeInfo(application.root, true)
	if err != nil {
		return application, err
	}
	if rootInfo.Identity != request.RepositoryIdentity {
		return application, ErrApplyConflict
	}
	if err = application.recheckContext(ctx); err != nil {
		return application, err
	}
	if err = m.privateDir("applications"); err != nil {
		return application, err
	}
	dir := "applications/" + request.OperationID
	// Existing operation namespaces are never reopened to reconstruct authority.
	if err = m.root.Mkdir(dir, 0700); err != nil {
		return application, fmt.Errorf("%w The private operation namespace already exists or is unavailable.", ErrApplyConflict)
	}
	if err = m.privateDir(dir); err != nil {
		return application, err
	}
	application.private, err = applyOpenDirectory(filepath.Join(m.lock.Root(), filepath.FromSlash(dir)))
	if err != nil {
		return application, err
	}
	privateInfo, err := applyNativeInfo(application.private, true)
	if err != nil {
		return application, err
	}
	if rootInfo.Volume != privateInfo.Volume {
		return application, fmt.Errorf("%w Private staging and destination are on different mounted filesystems.", ErrApplyUnsupported)
	}
	for _, change := range changes {
		if err = ctx.Err(); err != nil {
			return application, err
		}
		if !safeApplyPath(change.Path) || !application.selectedPath(change.Path) || change.Kind == "index_only" || change.Before != nil && change.Before.Type != "file" || change.After != nil && change.After.Type != "file" || change.Before != nil && change.After != nil && change.Before.Mode != change.After.Mode {
			return application, ErrApplyUnsupported
		}
		path := &applicationPath{change: change, name: filepath.Base(filepath.FromSlash(change.Path))}
		application.paths = append(application.paths, path)
		path.parent, err = application.openParent(change.Path)
		if err != nil {
			return application, err
		}
		path.parentInfo, err = applyNativeInfo(path.parent, true)
		if err != nil {
			return application, err
		}
		if path.parentInfo.Volume != privateInfo.Volume {
			return application, ErrApplyUnsupported
		}
		if change.Before != nil {
			path.original, err = applyOpenRelative(path.parent, path.name, false, false)
			if err != nil {
				if errors.Is(err, ErrApplyUnsupported) {
					return application, err
				}
				return application, ErrApplyConflict
			}
			path.beforeInfo, err = applyNativeInfo(path.original, false)
			if err != nil {
				return application, err
			}
			data, readErr := applyReadStable(path.original)
			if readErr != nil {
				return application, readErr
			}
			if !applyEntryBytes(*change.Before, data, path.beforeInfo.Mode) {
				return application, ErrApplyConflict
			}
			if matches, e := applyMatchesName(path.parent, path.name, path.original); e != nil || !matches {
				return application, ErrApplyConflict
			}
			before, e := m.artifacts.Put(ctx, request.AttemptID, "destination_file", data)
			if e != nil {
				return application, e
			}
			path.before = &before
		} else {
			if file, e := applyOpenRelative(path.parent, path.name, false, false); e == nil {
				file.Close()
				return application, ErrApplyConflict
			} else if !errors.Is(e, os.ErrNotExist) {
				return application, ErrApplyConflict
			}
		}
		if change.After != nil {
			var content bytes.Buffer
			copied, e := m.artifacts.CopyTo(ctx, change.After.ArtifactID, &content, MaxFileBytes)
			if e != nil {
				return application, e
			}
			if copied.SHA256 == nil || *copied.SHA256 != change.After.SHA256 || int64(content.Len()) != change.After.Size || !applyText(content.Bytes()) {
				return application, ErrApplyUnsupported
			}
			id, e := evidence.NewID()
			if e != nil {
				return application, e
			}
			path.stageName = id + "-after"
			path.stage, e = applyOpenRelative(application.private, path.stageName, true, false)
			if e != nil {
				return application, e
			}
			path.privateStageInfo, e = applyNativeInfo(path.stage, false)
			if e != nil {
				return application, e
			}
			if _, e = path.stage.Write(content.Bytes()); e != nil {
				return application, e
			}
			if e = path.stage.Sync(); e != nil {
				return application, e
			}
			if change.Before != nil {
				path.afterInfo = path.beforeInfo
			} else {
				path.afterInfo, e = applyNewFileMetadata(path.parent, change.After.Mode)
				if e != nil {
					return application, e
				}
			}
			path.stagePrepared = true
			if e = applyPrepareMetadata(path.stage, path.afterInfo); e != nil {
				return application, e
			}
			path.preparedStageInfo, e = applyNativeInfo(path.stage, false)
			if e != nil {
				return application, e
			}
			after, e := m.artifacts.Put(ctx, request.AttemptID, "destination_file", content.Bytes())
			if e != nil {
				return application, e
			}
			path.after = &after
		}
		if change.Before != nil {
			id, e := evidence.NewID()
			if e != nil {
				return application, e
			}
			path.quarantineName = id + "-before"
			application.actions = append(application.actions, ApplyAction{ID: id, RelativePath: change.Path, Kind: applyKind(change), Action: "quarantine_before", BeforeEvidence: path.before, IntendedAfterEvidence: path.after, QuarantineRef: dir + "/" + path.quarantineName})
		}
		if change.After != nil {
			id, e := evidence.NewID()
			if e != nil {
				return application, e
			}
			application.actions = append(application.actions, ApplyAction{ID: id, RelativePath: change.Path, Kind: applyKind(change), Action: "publish_after", BeforeEvidence: path.before, IntendedAfterEvidence: path.after, QuarantineRef: dir + "/" + path.quarantineName})
		}
	}
	if err = application.recheckContext(ctx); err != nil {
		return application, err
	}
	application.BeforeManifestArtifact, err = application.retainManifest(ctx, "before")
	if err != nil {
		return application, err
	}
	application.IntendedManifestArtifact, err = application.retainManifest(ctx, "intended_after")
	if err != nil {
		return application, err
	}
	return application, nil
}

func safeApplyPath(value string) bool {
	if !safePath(value) || excluded(value) != "" || strings.ContainsAny(value, ":*?\"<>|") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if strings.TrimRight(part, " .") != part {
			return false
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return false
			}
		}
		stem := strings.ToLower(strings.Split(part, ".")[0])
		if stem == "con" || stem == "prn" || stem == "aux" || stem == "nul" || len(stem) == 4 && (strings.HasPrefix(stem, "com") || strings.HasPrefix(stem, "lpt")) && stem[3] >= '0' && stem[3] <= '9' {
			return false
		}
	}
	return true
}
func applyText(data []byte) bool { return utf8.Valid(data) && !bytes.ContainsRune(data, 0) }
func applyEntryBytes(expected Entry, data []byte, mode uint32) bool {
	return expected.Type == "file" && expected.Size == int64(len(data)) && expected.SHA256 == hashBytes(data) && expected.Mode&0777 == mode && applyText(data)
}
func applyKind(change Change) string {
	if change.Before == nil {
		return "add"
	}
	if change.After == nil {
		return "delete"
	}
	return "replace"
}

func applyReadStable(file *os.File) ([]byte, error) {
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > MaxFileBytes {
		return nil, ErrApplyUnsupported
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := ioReadBounded(file, MaxFileBytes)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != int64(len(data)) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return nil, ErrApplyConflict
	}
	return data, nil
}
func (a *Application) selectedPath(relative string) bool {
	selected := a.request.Source.SelectedSubfolder
	return selected == "." || selected == "" || strings.HasPrefix(relative, selected+"/")
}
func (a *Application) openParent(relative string) (*os.File, error) {
	parts := strings.Split(relative, "/")
	current, err := applyOpenDirectory(a.request.Source.RepositoryPath)
	if err != nil {
		return nil, err
	}
	for _, name := range parts[:len(parts)-1] {
		next, e := applyOpenRelative(current, name, false, true)
		closeErr := current.Close()
		if e != nil {
			return nil, errors.Join(e, closeErr)
		}
		if closeErr != nil {
			next.Close()
			return nil, closeErr
		}
		current = next
	}
	return current, nil
}

func (a *Application) recheckContext(ctx context.Context) error {
	if err := a.manager.lock.CheckHeld(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, expected := range []struct{ path, identity string }{{a.request.Source.RepositoryPath, a.request.RepositoryIdentity}, {a.request.Source.ProjectPath, a.request.ProjectIdentity}} {
		file, err := applyOpenDirectory(expected.path)
		if err != nil {
			return ErrApplyConflict
		}
		info, err := applyNativeInfo(file, true)
		closeErr := file.Close()
		if err != nil || closeErr != nil || info.Identity != expected.identity {
			return ErrApplyConflict
		}
	}
	base, err := a.manager.value(ctx, a.request.Source.RepositoryPath, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || base != a.request.Source.BaseCommit {
		return ErrApplyConflict
	}
	branch, err := a.manager.value(ctx, a.request.Source.RepositoryPath, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil || branch != a.request.Source.Branch {
		return ErrApplyConflict
	}
	index, err := a.manager.index(ctx, a.request.Source.RepositoryPath)
	if err != nil {
		return err
	}
	find := func(entries []IndexEntry, name string) *IndexEntry {
		for _, entry := range entries {
			if entry.Path == name {
				return &entry
			}
		}
		return nil
	}
	for _, path := range a.paths {
		before, now := find(a.request.Source.Manifest.Index, path.change.Path), find(index, path.change.Path)
		if before == nil && now != nil || before != nil && now == nil || before != nil && now != nil && *before != *now {
			return ErrApplyConflict
		}
	}
	return nil
}

func (a *Application) retainManifest(ctx context.Context, stage string) (evidence.Metadata, error) {
	index, err := a.manager.index(ctx, a.request.Source.RepositoryPath)
	if err != nil {
		return evidence.Metadata{}, err
	}
	manifest := applyDestinationManifest{Contract: "005-destination-manifest-v1", OperationID: a.request.OperationID, SelectedCandidateID: a.request.SelectedCandidateID, RootSourceSnapshotID: a.request.Source.ID, Stage: stage, NamespaceDurability: "pending", ScopePaths: []string{}, Entries: []Entry{}, Index: index, Metadata: []applyMetadataObservation{}}
	if runtime.GOOS == "windows" {
		manifest.NamespaceDurability = "not_available"
	} else if stage == "observed_after" {
		manifest.NamespaceDurability = "directory_fsync_observed"
	}
	for _, path := range a.paths {
		manifest.ScopePaths = append(manifest.ScopePaths, path.change.Path)
		entry := path.change.Before
		metadata := path.beforeInfo
		artifact := path.before
		if stage != "before" {
			entry = path.change.After
			metadata = path.afterInfo
			artifact = path.after
		}
		if entry == nil {
			continue
		}
		copy := *entry
		if artifact != nil {
			copy.ArtifactID = artifact.ID
		}
		manifest.Entries = append(manifest.Entries, copy)
		manifest.Metadata = append(manifest.Metadata, applyMetadataObservation{Path: entry.Path, Identity: metadata.Identity, Volume: metadata.Volume, Owner: metadata.Owner, Group: metadata.Group, Security: metadata.Security, Mode: metadata.Mode, Control: metadata.Control})
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return evidence.Metadata{}, err
	}
	return a.manager.artifacts.Put(ctx, a.request.AttemptID, "destination_manifest", raw)
}
