// Package domain defines the preparation contract shared by the local interfaces.
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const APIVersion = 1
const AppVersion = "2.0.0-dev.6"
const MaxTextBytes = 4 * 1024 * 1024
const MaxBodyBytes = 32 * 1024 * 1024

type Error struct {
	GraphRef        *GraphRef       `json:"graphRef,omitzero"`
	GraphBudgetRef  *GraphBudgetRef `json:"graphBudgetRef,omitzero"`
	Code            string          `json:"code"`
	Message         string          `json:"message"`
	CurrentRevision string          `json:"currentRevision,omitempty"`
}

func (e *Error) Error() string         { return e.Message }
func Fail(code, message string) *Error { return &Error{Code: code, Message: message} }
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure identity generation unavailable")
	}
	return hex.EncodeToString(b[:])
}
func Now() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

type Receipt struct {
	RequestID   string `json:"requestId"`
	Operation   string `json:"operation"`
	CommittedAt string `json:"committedAt"`
}
type Envelope struct {
	APIVersion int      `json:"apiVersion"`
	RequestID  string   `json:"requestId"`
	Data       any      `json:"data,omitempty"`
	Receipt    *Receipt `json:"receipt,omitempty"`
	Error      *Error   `json:"error,omitempty"`
}
type Result struct {
	Status   int
	Envelope Envelope
}

func Success(status int, id string, data any, receipt *Receipt) Result {
	return Result{status, Envelope{APIVersion: 1, RequestID: id, Data: data, Receipt: receipt}}
}
func Failure(id string, err *Error) Result {
	return Result{Status: Status(err.Code), Envelope: Envelope{APIVersion: 1, RequestID: id, Error: err}}
}
func Status(code string) int {
	switch code {
	case "invalid_request", "invalid_encoding", "invalid_cursor":
		return 400
	case "unauthenticated":
		return 401
	case "invalid_host", "forbidden_origin":
		return 403
	case "not_found":
		return 404
	case "method_not_allowed":
		return 405
	case "graph_budget_required", "graph_scope_required", "graph_input_stale", "graph_control_conflict", "revision_conflict", "request_conflict", "project_identity_changed", "revision_exhausted", "source_draft_conflict", "task_version_conflict", "attempt_active", "acceptance_conflict", "routing_adoption_required", "budget_conflict", "report_conflict", "workflow_revision_conflict", "candidate_stale", "integration_conflict", "scope_paused", "checkpoint_not_paused", "checkpoint_scope_conflict", "checkpoint_blocked", "scope_stopped", "continuation_not_prepared", "cleanup_unknown":
		return 409
	case "input_too_large":
		return 413
	case "unsupported_media_type":
		return 415
	case "project_unavailable", "not_git_project", "prerequisite_missing", "discovery_unavailable", "capability_unavailable", "pin_unavailable", "permission_unavailable", "source_capture_blocked", "acceptance_unavailable", "evidence_unavailable", "budget_exhausted", "budget_expired", "routing_unavailable", "authority_required", "review_unavailable", "check_unavailable", "application_unsupported", "delivery_scope_required", "external_postcondition_unsupported", "checkpoint_too_large":
		return 422
	case "owner_unavailable", "storage_failed", "write_uncertain", "operation_interrupted", "dispatch_fenced", "owner_closing", "effect_uncertain":
		return 503
	default:
		return 500
	}
}
func SafeError(err error) *Error {
	if e, ok := err.(*Error); ok {
		return e
	}
	return Fail("internal_error", "The operation could not complete. Report this request identity.")
}

type Policy struct {
	Mode            string `json:"mode"`
	SelectionSource string `json:"selectionSource"`
	Revision        string `json:"revision"`
	UpdatedAt       string `json:"updatedAt"`
}
type Project struct {
	ID             string  `json:"id"`
	SelectedPath   string  `json:"selectedPath"`
	ResolvedPath   string  `json:"resolvedPath"`
	RepositoryRoot string  `json:"repositoryRoot"`
	Branch         *string `json:"branch"`
	Commit         *string `json:"commit"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
	ObservedAt     string  `json:"observedAt"`
	Revision       string  `json:"revision"`
}
type ProjectObservation struct {
	ResolvedPath   string  `json:"resolvedPath"`
	RepositoryRoot string  `json:"repositoryRoot"`
	Branch         *string `json:"branch"`
	Commit         *string `json:"commit"`
	ObservedAt     string  `json:"observedAt"`
}
type OpenSelection struct {
	SelectedPath string `json:"selectedPath"`
	BasePath     string `json:"basePath"`
}
type ProjectOpen struct {
	Project       Project            `json:"project"`
	OpenSelection OpenSelection      `json:"openSelection"`
	Observation   ProjectObservation `json:"observation"`
}
type ProjectGet struct {
	Project          Project             `json:"project"`
	Observation      *ProjectObservation `json:"observation"`
	ObservationError *Error              `json:"observationError"`
}
type ResolvedProject struct {
	Observation      ProjectObservation
	PhysicalIdentity string
}
type Draft struct {
	ID              string `json:"id"`
	ProjectID       string `json:"projectId"`
	OriginalText    string `json:"originalText"`
	Revision        string `json:"revision"`
	Status          string `json:"status"`
	Selection       string `json:"selection"`
	RequestedPolicy Policy `json:"requestedPolicy"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}
type DraftSummary struct {
	ID              string `json:"id"`
	ProjectID       string `json:"projectId"`
	Revision        string `json:"revision"`
	Status          string `json:"status"`
	Selection       string `json:"selection"`
	RequestedPolicy Policy `json:"requestedPolicy"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

func (d Draft) Summary() DraftSummary {
	return DraftSummary{d.ID, d.ProjectID, d.Revision, d.Status, d.Selection, d.RequestedPolicy, d.CreatedAt, d.UpdatedAt}
}

type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"nextCursor"`
}
type ConnectionObservation struct {
	HarnessID            string `json:"harnessId"`
	InstallationStatus   string `json:"installationStatus"`
	LauncherPath         string `json:"launcherPath,omitempty"`
	Source               string `json:"source"`
	ObservedAt           string `json:"observedAt"`
	ManagedExecution     string `json:"managedExecution"`
	Authentication       string `json:"authentication"`
	Provider             string `json:"provider"`
	Account              string `json:"account"`
	Model                string `json:"model"`
	EffectivePermissions string `json:"effectivePermissions"`
	ExternalOccupancy    string `json:"externalOccupancy"`
	Allowance            string `json:"allowance"`
	Usage                string `json:"usage"`
	NextAction           string `json:"nextAction"`
}
type DiscoveryEntry struct {
	HarnessID   string                 `json:"harnessId"`
	Observation *ConnectionObservation `json:"observation"`
	Error       *Error                 `json:"error"`
}
type DiscoveryResult struct {
	ObservedAt   string           `json:"observedAt"`
	Source       string           `json:"source"`
	Observations []DiscoveryEntry `json:"observations"`
}

func RevisionConflict(current string) *Error {
	return &Error{Code: "revision_conflict", Message: "This resource has a newer saved version. Reload it before saving.", CurrentRevision: current}
}
func Invalid(field string) *Error {
	return Fail("invalid_request", fmt.Sprintf("Supply a valid %s.", field))
}
