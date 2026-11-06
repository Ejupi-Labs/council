// Package workspace isolates exact current source and retains attributable candidates.
package workspace

import "errors"

const (
	MaxEntries          = 100000
	MaxFileBytes  int64 = 64 << 20
	MaxTotalBytes int64 = 512 << 20
)

var ErrBlocked = errors.New("Source capture is blocked.")

type Request struct {
	AttemptID         string
	ProjectPath       string
	SelectedSubfolder string
}

type Entry struct {
	Path       string `json:"path"`
	Type       string `json:"type"`
	Mode       uint32 `json:"mode"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256,omitempty"`
	ArtifactID string `json:"artifactId,omitempty"`
	Target     string `json:"target,omitempty"`
}

type Exclusion struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type IndexEntry struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Object string `json:"object"`
}
type Manifest struct {
	Entries    []Entry      `json:"entries"`
	Index      []IndexEntry `json:"index"`
	Exclusions []Exclusion  `json:"exclusions"`
}
type Snapshot struct {
	ID                 string   `json:"id"`
	AttemptID          string   `json:"attemptId"`
	ProjectPath        string   `json:"projectPath"`
	RepositoryPath     string   `json:"repositoryPath"`
	SelectedSubfolder  string   `json:"selectedSubfolder"`
	WorkspacePath      string   `json:"workspacePath"`
	ExecutionPath      string   `json:"executionPath"`
	BaseCommit         string   `json:"baseCommit"`
	Branch             string   `json:"branch"`
	ManifestArtifactID string   `json:"manifestArtifactId"`
	IndexArtifactID    string   `json:"indexArtifactId"`
	Manifest           Manifest `json:"manifest"`
	GitControlSHA256   string   `json:"-"`
}
type Change struct {
	Path        string      `json:"path"`
	Kind        string      `json:"kind"`
	Before      *Entry      `json:"before,omitempty"`
	After       *Entry      `json:"after,omitempty"`
	BeforeIndex *IndexEntry `json:"beforeIndex,omitempty"`
	AfterIndex  *IndexEntry `json:"afterIndex,omitempty"`
}
type Candidate struct {
	ID                 string   `json:"id"`
	SourceSnapshotID   string   `json:"sourceSnapshotId"`
	WorkspacePath      string   `json:"workspacePath"`
	ManifestArtifactID string   `json:"manifestArtifactId"`
	DeltaArtifactID    string   `json:"deltaArtifactId"`
	Changes            []Change `json:"changes"`
	Verification       string   `json:"verification"`
	Integration        string   `json:"integration"`
	Partial            bool     `json:"partial"`
}
