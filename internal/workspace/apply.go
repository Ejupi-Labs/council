package workspace

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

var ErrApplyUnsupported = errors.New("The destination has an unsupported application state.")
var ErrApplyConflict = errors.New("The destination no longer matches the captured application basis.")

type ApplyRequest struct {
	AttemptID, OperationID                 string
	Source                                 *Snapshot
	SelectedCandidateID, SelectedAttemptID string
	CandidateManifestSHA256                string
	CandidateManifestArtifactID            string
	ProjectIdentity, RepositoryIdentity    string
}
type ApplyAction struct {
	ID, RelativePath, Kind, Action        string
	BeforeEvidence, IntendedAfterEvidence *evidence.Metadata
	QuarantineRef                         string `json:"-"`
}
type ApplyResult struct {
	ActionID, RelativePath, State, Identity, SHA256 string
	NamespaceDurability                             string
	ObservedEvidence                                *evidence.Metadata
	Known                                           bool
	QuarantineRef                                   string `json:"-"`
}

// Private native metadata is observed under retained handles, never a trusted
// public input or a persisted handle. Native contains platform-owned descriptor data.
type applyFileInfo struct {
	Identity, Volume, Owner, Group, Security string
	Mode, Control                            uint32
	Native                                   any
}
type applicationPath struct {
	change                              Change
	parent, original, stage             *os.File
	preparedStageInfo, privateStageInfo applyFileInfo
	stagePrepared, stagePublished       bool
	parentInfo, beforeInfo, afterInfo   applyFileInfo
	name, stageName, quarantineName     string
	before, after                       *evidence.Metadata
}
type Application struct {
	BeforeManifestArtifact, IntendedManifestArtifact, AfterManifestArtifact evidence.Metadata
	mu                                                                      sync.Mutex
	manager                                                                 *Manager
	request                                                                 ApplyRequest
	root, private                                                           *os.File
	paths                                                                   []*applicationPath
	actions                                                                 []ApplyAction
	next                                                                    int
	failed, closed                                                          bool
}

func applyMetadataEqual(actual, expected applyFileInfo) bool {
	return actual.Owner == expected.Owner && actual.Group == expected.Group && actual.Mode == expected.Mode && actual.Security == expected.Security && actual.Control == expected.Control
}

func (m *Manager) PrepareApply(ctx context.Context, request ApplyRequest) (*Application, error) {
	return m.prepareApplication(ctx, request)
}
func (a *Application) Actions() []ApplyAction { return append([]ApplyAction{}, a.actions...) }
func (a *Application) ExecuteAction(ctx context.Context, actionID string) (ApplyResult, error) {
	return a.executePermitted(ctx, actionID)
}
func (a *Application) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	var result error
	for _, path := range a.paths {
		if path.stage != nil && path.stagePrepared && !path.stagePublished {
			matches, err := applyMatchesName(a.private, path.stageName, path.stage)
			if err != nil || !matches {
				result = errors.Join(result, ErrApplyConflict)
			} else {
				if err := applyPrepareMetadata(path.stage, path.privateStageInfo); err != nil {
					result = errors.Join(result, err)
				} else {
					result = errors.Join(result, applyRestoreMetadata(path.stage, path.privateStageInfo))
				}
			}
		}
		for _, file := range []*os.File{path.stage, path.original, path.parent} {
			if file != nil {
				result = errors.Join(result, file.Close())
			}
		}
	}
	if a.private != nil {
		result = errors.Join(result, a.private.Close())
	}
	if a.root != nil {
		result = errors.Join(result, a.root.Close())
	}
	return result
}
