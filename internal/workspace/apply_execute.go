package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"
)

// executePermitted performs exactly one already-durable controller intent. It
// never obtains another permit, advances workflow state or restores a user path.
func (a *Application) executePermitted(ctx context.Context, actionID string) (result ApplyResult, resultErr error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	result = ApplyResult{ActionID: actionID, State: "failed", Known: true, NamespaceDurability: "pending"}
	if runtime.GOOS == "windows" {
		result.NamespaceDurability = "not_available"
	}
	if a.closed || a.failed || a.next >= len(a.actions) || a.actions[a.next].ID != actionID {
		return result, ErrApplyConflict
	}
	action := a.actions[a.next]
	result.RelativePath = action.RelativePath
	result.QuarantineRef = action.QuarantineRef
	defer func() {
		if resultErr != nil {
			a.failed = true
		}
	}()
	if err := a.recheckContext(ctx); err != nil {
		return result, err
	}
	var path *applicationPath
	for _, entry := range a.paths {
		if entry.change.Path == action.RelativePath {
			path = entry
			break
		}
	}
	if path == nil {
		return result, ErrApplyConflict
	}
	if err := a.verifyParent(path); err != nil {
		return result, err
	}
	settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	uncertain := func(err error) (ApplyResult, error) {
		result.State = "uncertain"
		result.Known = false
		return result, err
	}
	switch action.Action {
	case "quarantine_before":
		if path.original == nil {
			return result, ErrApplyConflict
		}
		before, err := applyNativeInfo(path.original, false)
		if err != nil {
			return result, err
		}
		data, err := applyReadStable(path.original)
		if err != nil {
			return result, err
		}
		if !applyMetadataEqual(before, path.beforeInfo) || before.Identity != path.beforeInfo.Identity || !applyEntryBytes(*path.change.Before, data, before.Mode) {
			return result, ErrApplyConflict
		}
		if matches, e := applyMatchesName(path.parent, path.name, path.original); e != nil || !matches {
			return result, ErrApplyConflict
		}
		if err = applyRenameExclusive(path.parent, path.name, path.original, a.private, path.quarantineName); err != nil {
			// No automatic retry follows even a known no-replace conflict. Other
			// native failures remain uncertain unless the current source slot is
			// positively observed as the same retained original.
			if matches, e := applyMatchesName(path.parent, path.name, path.original); e == nil && matches {
				result.State = "conflict"
				return result, err
			}
			return uncertain(err)
		}
		if matches, e := applyMatchesName(a.private, path.quarantineName, path.original); e != nil || !matches {
			// On Unix the source name can swap after the last guard. The actual
			// displaced object stays live in quarantine; never follow a redirect.
			moved, openErr := applyOpenRelative(a.private, path.quarantineName, false, false)
			if openErr == nil {
				observed, readErr := applyReadStable(moved)
				if readErr == nil {
					artifact, saveErr := a.manager.artifacts.Put(settlementCtx, a.request.AttemptID, "destination_file", observed)
					if saveErr == nil {
						result.ObservedEvidence = &artifact
					}
				}
				moved.Close()
			}
			return uncertain(fmt.Errorf("%w The displaced object differs from its checked original; retained quarantine must be inspected.", ErrApplyConflict))
		}
		observed, err := applyReadStable(path.original)
		if err != nil {
			return uncertain(err)
		}
		metadata, err := applyNativeInfo(path.original, false)
		if err != nil {
			return uncertain(err)
		}
		artifact, err := a.manager.artifacts.Put(settlementCtx, a.request.AttemptID, "destination_file", observed)
		if err != nil {
			return uncertain(err)
		}
		result.ObservedEvidence = &artifact
		result.Identity = metadata.Identity
		result.SHA256 = *artifact.SHA256
		if !applyEntryBytes(*path.change.Before, observed, metadata.Mode) || !applyMetadataEqual(metadata, path.beforeInfo) {
			return uncertain(ErrApplyConflict)
		}
		if err = applySyncDirectory(path.parent); err != nil {
			return uncertain(err)
		}
		if err = applySyncDirectory(a.private); err != nil {
			return uncertain(err)
		}
	case "publish_after":
		if path.stage == nil {
			return result, ErrApplyConflict
		}
		if path.original != nil {
			if matches, e := applyMatchesName(a.private, path.quarantineName, path.original); e != nil || !matches {
				return result, ErrApplyConflict
			}
			before, e := applyReadStable(path.original)
			metadata, metadataErr := applyNativeInfo(path.original, false)
			if e != nil || metadataErr != nil || !applyMetadataEqual(metadata, path.beforeInfo) || !applyEntryBytes(*path.change.Before, before, metadata.Mode) {
				return result, ErrApplyConflict
			}
		}
		prepared, err := applyNativeInfo(path.stage, false)
		if err != nil {
			return result, err
		}
		bytes, err := applyReadStable(path.stage)
		if err != nil {
			return result, err
		}
		if prepared.Identity != path.preparedStageInfo.Identity || !applyMetadataEqual(prepared, path.preparedStageInfo) || !applyEntryBytes(*path.change.After, bytes, prepared.Mode) {
			return result, ErrApplyConflict
		}
		if matches, e := applyMatchesName(a.private, path.stageName, path.stage); e != nil || !matches {
			return result, ErrApplyConflict
		}
		if err = applyRenameExclusive(a.private, path.stageName, path.stage, path.parent, path.name); err != nil {
			if matches, e := applyMatchesName(a.private, path.stageName, path.stage); e == nil && matches {
				result.State = "conflict"
				return result, err
			}
			return uncertain(err)
		}
		path.stagePublished = true
		// The Windows inheritance restore is part of this SAME permitted
		// publication. Cancellation must not skip it after the namespace move.
		if err = applyRestoreMetadata(path.stage, path.afterInfo); err != nil {
			return uncertain(err)
		}
		if err = path.stage.Sync(); err != nil {
			return uncertain(err)
		}
		if matches, e := applyMatchesName(path.parent, path.name, path.stage); e != nil || !matches {
			return uncertain(ErrApplyConflict)
		}
		observed, err := applyReadStable(path.stage)
		if err != nil {
			return uncertain(err)
		}
		metadata, err := applyNativeInfo(path.stage, false)
		if err != nil {
			return uncertain(err)
		}
		if !applyEntryBytes(*path.change.After, observed, metadata.Mode) || !applyMetadataEqual(metadata, path.afterInfo) {
			return uncertain(ErrApplyConflict)
		}
		artifact, err := a.manager.artifacts.Put(settlementCtx, a.request.AttemptID, "destination_file", observed)
		if err != nil {
			return uncertain(err)
		}
		path.after = &artifact
		path.afterInfo = metadata
		result.ObservedEvidence = &artifact
		result.Identity = metadata.Identity
		result.SHA256 = *artifact.SHA256
		if err = applySyncDirectory(path.parent); err != nil {
			return uncertain(err)
		}
		if err = applySyncDirectory(a.private); err != nil {
			return uncertain(err)
		}
	default:
		return result, ErrApplyUnsupported
	}
	if err := a.verifyParent(path); err != nil {
		return uncertain(err)
	}
	if runtime.GOOS != "windows" {
		result.NamespaceDurability = "directory_fsync_observed"
	}
	result.State = "applied"
	a.next++
	if a.next == len(a.actions) {
		// Final verification is observational, with no additional write permit.
		if err := a.verifyApplied(settlementCtx); err != nil {
			return uncertain(err)
		}
		a.AfterManifestArtifact, resultErr = a.retainManifest(settlementCtx, "observed_after")
		if resultErr != nil {
			return uncertain(resultErr)
		}
	}
	return result, nil
}

func (a *Application) verifyParent(path *applicationPath) error {
	current, err := a.openParent(path.change.Path)
	if err != nil {
		return ErrApplyConflict
	}
	info, err := applyNativeInfo(current, true)
	closeErr := current.Close()
	if err != nil || closeErr != nil || info.Identity != path.parentInfo.Identity || !applyMetadataEqual(info, path.parentInfo) {
		return ErrApplyConflict
	}
	return nil
}

func (a *Application) verifyApplied(ctx context.Context) error {
	if err := a.recheckContext(ctx); err != nil {
		return err
	}
	for _, path := range a.paths {
		if err := a.verifyParent(path); err != nil {
			return err
		}
		if path.change.After == nil {
			if file, err := applyOpenRelative(path.parent, path.name, false, false); err == nil {
				file.Close()
				return ErrApplyConflict
			} else if !errors.Is(err, os.ErrNotExist) {
				return ErrApplyConflict
			}
			continue
		}
		if !path.stagePublished {
			return ErrApplyConflict
		}
		if matches, err := applyMatchesName(path.parent, path.name, path.stage); err != nil || !matches {
			return ErrApplyConflict
		}
		data, err := applyReadStable(path.stage)
		if err != nil {
			return err
		}
		info, err := applyNativeInfo(path.stage, false)
		if err != nil {
			return err
		}
		if !applyEntryBytes(*path.change.After, data, info.Mode) || !applyMetadataEqual(info, path.afterInfo) {
			return ErrApplyConflict
		}
	}
	return nil
}
