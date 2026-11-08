package domain

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// History contains only an allowlisted, observational projection. It is not
// evidence, a backup, an import format, or authority to resume work.
const (
	HistoryProfile         = "metadata-history/1"
	HistoryCompleteness    = "incomplete_history"
	HistoryNotice          = "Incomplete observational history. Not a backup, evidence export, restore, or proof of acceptance."
	HistoryOutputFormat    = "council.metadata-history/1"
	HistoryMaxTasks        = 2048
	HistoryMaxAttempts     = 8192
	HistoryMaxPayloadBytes = 4 * 1024 * 1024
	HistoryMaxRequestBytes = 16 * 1024
	HistoryMaxPageSize     = 100
	HistoryDefaultPageSize = 50
	HistoryMaxCursorBytes  = 512
)

type HistoryOmission struct {
	Reason string `json:"reason"`
	Count  string `json:"count"`
}

type HistorySnapshotSummary struct {
	SnapshotID          string            `json:"snapshotId"`
	Revision            string            `json:"revision"`
	ProjectID           string            `json:"projectId"`
	Profile             string            `json:"profile"`
	Completeness        string            `json:"completeness"`
	Notice              string            `json:"notice"`
	CapturedAt          string            `json:"capturedAt"`
	TaskCount           string            `json:"taskCount"`
	AttemptCount        string            `json:"attemptCount"`
	IncludedCategories  []string          `json:"includedCategories"`
	OmittedCategories   []string          `json:"omittedCategories"`
	AdditionalOmissions []HistoryOmission `json:"additionalOmissions"`
}

type HistoryTaskRow struct {
	Ordinal        string `json:"ordinal"`
	TaskID         string `json:"taskId"`
	ProjectID      string `json:"projectId"`
	Revision       string `json:"revision"`
	StateRevision  string `json:"stateRevision"`
	CurrentVersion string `json:"currentVersion"`
}

type HistoryAttemptRow struct {
	Ordinal     string `json:"ordinal"`
	AttemptID   string `json:"attemptId"`
	TaskID      string `json:"taskId"`
	TaskVersion string `json:"taskVersion"`
	Revision    string `json:"revision"`
	Phase       string `json:"phase"`
}

type HistorySnapshot struct {
	Summary  HistorySnapshotSummary `json:"snapshot"`
	Tasks    []HistoryTaskRow       `json:"tasks"`
	Attempts []HistoryAttemptRow    `json:"attempts"`
}

type HistoryIntegrity struct {
	Algorithm      string `json:"algorithm"`
	Encoding       string `json:"encoding"`
	SnapshotSHA256 string `json:"snapshotSha256"`
}

type HistoryPayload struct {
	Format    string                 `json:"format"`
	Snapshot  HistorySnapshotSummary `json:"snapshot"`
	Tasks     []HistoryTaskRow       `json:"tasks"`
	Attempts  []HistoryAttemptRow    `json:"attempts"`
	Integrity HistoryIntegrity       `json:"integrity"`
}

// Items must be a non-nil []HistoryTaskRow or []HistoryAttemptRow matching Kind.
type HistorySnapshotPage struct {
	SnapshotID   string  `json:"snapshotId"`
	Revision     string  `json:"revision"`
	Completeness string  `json:"completeness"`
	Notice       string  `json:"notice"`
	Kind         string  `json:"kind"`
	Items        any     `json:"items"`
	NextCursor   *string `json:"nextCursor"`
}

type HistoryExportPreview struct {
	PreviewID            string            `json:"previewId"`
	Revision             string            `json:"revision"`
	SnapshotID           string            `json:"snapshotId"`
	SnapshotRevision     string            `json:"snapshotRevision"`
	Filename             string            `json:"filename"`
	DestinationKind      string            `json:"destinationKind"`
	DestinationDirectory string            `json:"destinationDirectory"`
	DestinationPath      string            `json:"destinationPath"`
	Completeness         string            `json:"completeness"`
	IncludedCategories   []string          `json:"includedCategories"`
	OmittedCategories    []string          `json:"omittedCategories"`
	AdditionalOmissions  []HistoryOmission `json:"additionalOmissions"`
	TargetState          string            `json:"targetState"`
	CreatedAt            string            `json:"createdAt"`
}

type HistoryDurability struct {
	FileFlush                   string `json:"fileFlush"`
	DirectorySync               string `json:"directorySync"`
	PowerLossNamespaceGuarantee bool   `json:"powerLossNamespaceGuarantee"`
}

type HistoryExportOperation struct {
	OperationID          string             `json:"operationId"`
	RequestID            string             `json:"requestId"`
	Revision             string             `json:"revision"`
	SnapshotID           string             `json:"snapshotId"`
	PreviewID            string             `json:"previewId"`
	State                string             `json:"state"`
	Completeness         string             `json:"completeness"`
	Outcome              *string            `json:"outcome"`
	Durability           *HistoryDurability `json:"durability"`
	Blockers             []string           `json:"blockers"`
	NextAction           string             `json:"nextAction"`
	Filename             string             `json:"filename"`
	DestinationDirectory string             `json:"destinationDirectory"`
	DestinationPath      string             `json:"destinationPath"`
}

type HistoryReconciliationResult struct {
	Operation        HistoryExportOperation `json:"operation"`
	Decision         string                 `json:"decision"`
	ObservationCodes []string               `json:"observationCodes"`
}

// Publication records are controller-only observations, never client proofs.
type HistoryPublicationEffect struct {
	EffectID        string
	OperationID     string
	OwnerInstanceID string
	OwnerGeneration string
	Filename        string
}

type HistoryEffectObservation struct {
	ID              string
	OperationID     string
	EffectID        string
	OwnerInstanceID string
	Kind            string
	At              string
}

type HistoryReceipt = Receipt

type CaptureHistoryInput struct {
	RequestID string `json:"requestId"`
	ProjectID string `json:"projectId"`
}
type PreviewHistoryExportInput struct {
	RequestID        string `json:"requestId"`
	SnapshotID       string `json:"snapshotId"`
	SnapshotRevision string `json:"snapshotRevision"`
	Filename         string `json:"filename"`
}
type ExecuteHistoryExportInput struct {
	RequestID       string `json:"requestId"`
	PreviewID       string `json:"previewId"`
	PreviewRevision string `json:"previewRevision"`
}
type ReconcileHistoryOperationInput struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision string `json:"expectedRevision"`
	Action           string `json:"action"`
}

type HistoryRecordsQuery struct {
	Kind   string
	Limit  string
	Cursor string
}

// StrictDecodeHistory must be followed by the appropriate input validator.
func StrictDecodeHistory(body []byte, target any) *Error {
	if len(body) > HistoryMaxRequestBytes {
		return Fail("input_too_large", "History requests cannot exceed 16 KiB.")
	}
	return StrictDecode(body, target)
}

func ValidateCaptureHistoryInput(in CaptureHistoryInput) *Error {
	if !ValidID(in.RequestID) || !ValidID(in.ProjectID) {
		return Invalid("history request and project identity")
	}
	return nil
}
func ValidatePreviewHistoryExportInput(in PreviewHistoryExportInput) *Error {
	if !ValidID(in.RequestID) || !ValidID(in.SnapshotID) || in.SnapshotRevision != "1" || !ValidHistoryFilename(in.Filename) {
		return Invalid("history preview identity, revision and filename")
	}
	return nil
}
func ValidateExecuteHistoryExportInput(in ExecuteHistoryExportInput) *Error {
	if !ValidID(in.RequestID) || !ValidID(in.PreviewID) || in.PreviewRevision != "1" {
		return Invalid("history export request, preview and revision")
	}
	return nil
}
func ValidateReconcileHistoryOperationInput(in ReconcileHistoryOperationInput) *Error {
	if !ValidID(in.RequestID) || !validRecoveryRevision(in.ExpectedRevision) || in.Action != "verify_no_publication" {
		return Invalid("history reconciliation identity, revision and action")
	}
	return nil
}

// ValidateHistoryRecordsQuery checks the shape only. Cursor authenticity,
// snapshot binding and pagination are owner responsibilities.
func ValidateHistoryRecordsQuery(q HistoryRecordsQuery) *Error {
	if q.Kind != "tasks" && q.Kind != "attempts" || len(q.Cursor) > HistoryMaxCursorBytes {
		return Invalid("history records query")
	}
	if q.Limit != "" {
		n, err := ParseRevision(q.Limit)
		if err != nil || n > HistoryMaxPageSize {
			return Invalid("history page limit")
		}
	}
	return nil
}

var historyFilename = regexp.MustCompile(`^history-[A-Za-z0-9][A-Za-z0-9_-]{0,50}\.json$`)

func ValidHistoryFilename(name string) bool {
	if !historyFilename.MatchString(name) {
		return false
	}
	stem := strings.ToUpper(strings.TrimSuffix(name, ".json"))
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
		return false
	}
	return true
}

func validHistoryTimestamp(value string) bool {
	t, err := time.Parse("2006-01-02T15:04:05.000Z", value)
	return err == nil && t.UTC().Format("2006-01-02T15:04:05.000Z") == value
}

var historyIncluded = []string{"project_identity", "task_metadata", "attempt_metadata"}
var historyOmitted = []string{"task_text", "project_paths", "user_files", "artifact_payloads", "provider_output", "vendor_authentication", "owner_authority", "legacy_state"}

func validHistoryCategories(included, omitted []string, extra []HistoryOmission) bool {
	if len(included) != len(historyIncluded) || len(omitted) != len(historyOmitted) || extra == nil || len(extra) > 3 {
		return false
	}
	for i, x := range historyIncluded {
		if included[i] != x {
			return false
		}
	}
	for i, x := range historyOmitted {
		if omitted[i] != x {
			return false
		}
	}
	last := -1
	for _, item := range extra {
		index := -1
		for i, reason := range []string{"unknown_origin", "unavailable", "invalid_link"} {
			if item.Reason == reason {
				index = i
			}
		}
		if index <= last {
			return false
		}
		if _, err := ParseCount(item.Count); err != nil {
			return false
		}
		last = index
	}
	return true
}

func ValidateHistorySnapshot(s HistorySnapshot) *Error {
	x := s.Summary
	if !ValidID(x.SnapshotID) || !ValidID(x.ProjectID) || x.Revision != "1" || x.Profile != HistoryProfile || x.Completeness != HistoryCompleteness || x.Notice != HistoryNotice || !validHistoryTimestamp(x.CapturedAt) || !validHistoryCategories(x.IncludedCategories, x.OmittedCategories, x.AdditionalOmissions) {
		return Invalid("history snapshot summary")
	}
	if s.Tasks == nil || s.Attempts == nil || len(s.Tasks) > HistoryMaxTasks || len(s.Attempts) > HistoryMaxAttempts || x.TaskCount != strconv.Itoa(len(s.Tasks)) || x.AttemptCount != strconv.Itoa(len(s.Attempts)) {
		return Invalid("history snapshot bounds and counts")
	}
	tasks := make(map[string]bool, len(s.Tasks))
	for i, row := range s.Tasks {
		if row.Ordinal != strconv.Itoa(i+1) || !ValidID(row.TaskID) || row.ProjectID != x.ProjectID || tasks[row.TaskID] || !validRecoveryRevision(row.Revision) || !validRecoveryRevision(row.StateRevision) || !validRecoveryRevision(row.CurrentVersion) {
			return Invalid("history task row")
		}
		tasks[row.TaskID] = true
	}
	attempts := make(map[string]bool, len(s.Attempts))
	for i, row := range s.Attempts {
		if row.Ordinal != strconv.Itoa(i+1) || !ValidID(row.AttemptID) || attempts[row.AttemptID] || !tasks[row.TaskID] || !validRecoveryRevision(row.TaskVersion) || !validRecoveryRevision(row.Revision) || !validHistoryPhase(row.Phase) {
			return Invalid("history attempt row")
		}
		attempts[row.AttemptID] = true
	}
	return nil
}

// ValidateHistoryPayload validates the fixed metadata envelope and digest
// shape; comparing the digest and enforcing the encoded byte limit are the
// serializer's responsibilities, since they apply to the exact output bytes.
func ValidateHistoryPayload(p HistoryPayload) *Error {
	if p.Format != HistoryOutputFormat || p.Integrity.Algorithm != "sha256" || p.Integrity.Encoding != "hex-lower" || !ValidSHA256(p.Integrity.SnapshotSHA256) {
		return Invalid("history payload integrity")
	}
	return ValidateHistorySnapshot(HistorySnapshot{Summary: p.Snapshot, Tasks: p.Tasks, Attempts: p.Attempts})
}

func ValidateHistorySnapshotPage(p HistorySnapshotPage) *Error {
	if !ValidID(p.SnapshotID) || p.Revision != "1" || p.Completeness != HistoryCompleteness || p.Notice != HistoryNotice || p.NextCursor != nil && (len(*p.NextCursor) == 0 || len(*p.NextCursor) > HistoryMaxCursorBytes) {
		return Invalid("history snapshot page")
	}
	switch p.Kind {
	case "tasks":
		rows, ok := p.Items.([]HistoryTaskRow)
		if !ok || rows == nil || len(rows) > HistoryMaxPageSize {
			return Invalid("history task page")
		}
		for _, r := range rows {
			if !ValidID(r.TaskID) || !ValidID(r.ProjectID) || !validRecoveryRevision(r.Ordinal) || !validRecoveryRevision(r.Revision) || !validRecoveryRevision(r.StateRevision) || !validRecoveryRevision(r.CurrentVersion) {
				return Invalid("history task page row")
			}
		}
	case "attempts":
		rows, ok := p.Items.([]HistoryAttemptRow)
		if !ok || rows == nil || len(rows) > HistoryMaxPageSize {
			return Invalid("history attempt page")
		}
		for _, r := range rows {
			if !ValidID(r.AttemptID) || !ValidID(r.TaskID) || !validRecoveryRevision(r.Ordinal) || !validRecoveryRevision(r.TaskVersion) || !validRecoveryRevision(r.Revision) || !validHistoryPhase(r.Phase) {
				return Invalid("history attempt page row")
			}
		}
	default:
		return Invalid("history page kind")
	}
	return nil
}

func ValidateHistoryReceipt(r HistoryReceipt) *Error {
	if !ValidID(r.RequestID) || !validHistoryTimestamp(r.CommittedAt) {
		return Invalid("history receipt")
	}
	switch r.Operation {
	case "history.snapshots.create", "history.exports.preview", "history.exports.create", "history.operations.reconcile":
		return nil
	default:
		return Invalid("history receipt operation")
	}
}
func validHistoryPhase(phase string) bool {
	switch phase {
	case "queued", "preparing", "running", "cancelling", "settled":
		return true
	}
	return false
}

func validHistoryDestination(dir, path, filename string) bool {
	return ValidHistoryFilename(filename) && filepath.IsAbs(dir) && filepath.IsAbs(path) && filepath.Join(dir, filename) == path
}

func ValidateHistoryExportPreview(p HistoryExportPreview) *Error {
	if !ValidID(p.PreviewID) || p.Revision != "1" || !ValidID(p.SnapshotID) || p.SnapshotRevision != "1" || p.DestinationKind != "private_history_exports" || !validHistoryDestination(p.DestinationDirectory, p.DestinationPath, p.Filename) || p.Completeness != HistoryCompleteness || !validHistoryCategories(p.IncludedCategories, p.OmittedCategories, p.AdditionalOmissions) || p.TargetState != "absent" || !validHistoryTimestamp(p.CreatedAt) {
		return Invalid("history export preview")
	}
	return nil
}

// HistoryCode is a bounded machine-readable token, never raw provider text.
func ValidHistoryCode(code string) bool {
	if len(code) < 1 || len(code) > 64 {
		return false
	}
	for i, c := range code {
		if !(c >= 'a' && c <= 'z' || i > 0 && (c >= '0' && c <= '9' || c == '_')) {
			return false
		}
	}
	return true
}

func validHistoryBlocker(code string) bool {
	switch code {
	case "owner_epoch_uncertain", "journal_unproven", "actor_not_drained", "history_effect_uncertain", "other_blocker", "target_conflict", "root_unavailable", "request_interrupted":
		return true
	default:
		return false
	}
}

func validHistoryNextAction(code string) bool {
	switch code {
	case "none", "inspect", "reconcile", "contact_support", "wait":
		return true
	default:
		return false
	}
}

func ValidateHistoryExportOperation(o HistoryExportOperation) *Error {
	if !ValidID(o.OperationID) || !ValidID(o.RequestID) || !validRecoveryRevision(o.Revision) || !ValidID(o.SnapshotID) || !ValidID(o.PreviewID) || o.Completeness != HistoryCompleteness || !validHistoryDestination(o.DestinationDirectory, o.DestinationPath, o.Filename) || o.Blockers == nil || len(o.Blockers) > 8 || !validHistoryNextAction(o.NextAction) {
		return Invalid("history export operation")
	}
	seenBlockers := make(map[string]bool, len(o.Blockers))
	for _, b := range o.Blockers {
		if !validHistoryBlocker(b) || seenBlockers[b] {
			return Invalid("history operation blocker")
		}
		seenBlockers[b] = true
	}
	switch o.State {
	case "prepared", "effect_started":
		if o.Outcome != nil || o.Durability != nil {
			return Invalid("history pending outcome")
		}
	case "published":
		if o.Outcome == nil || *o.Outcome != "published" || o.Durability == nil || o.Durability.FileFlush != "succeeded" || (o.Durability.DirectorySync != "succeeded" && o.Durability.DirectorySync != "unavailable") || o.Durability.PowerLossNamespaceGuarantee {
			return Invalid("history publication durability")
		}
	case "failed", "uncertain", "no_publication":
		if o.Outcome == nil || *o.Outcome != o.State || o.Durability != nil {
			return Invalid("history operation outcome")
		}
	default:
		return Invalid("history operation state")
	}
	return nil
}

func ValidateHistoryReconciliationResult(r HistoryReconciliationResult) *Error {
	if e := ValidateHistoryExportOperation(r.Operation); e != nil {
		return e
	}
	if r.ObservationCodes == nil || len(r.ObservationCodes) > 6 {
		return Invalid("history reconciliation observations")
	}
	valid := map[string]bool{"no_permit_proven": true, "permit_present_or_uncertain": true, "actor_not_drained": true, "owner_history_unproven": true, "journal_unproven": true, "other_blocker": true}
	seen := map[string]bool{}
	for _, c := range r.ObservationCodes {
		if !valid[c] || seen[c] {
			return Invalid("history reconciliation observation code")
		}
		seen[c] = true
	}
	if r.Decision == "no_publication" {
		if r.Operation.State != "no_publication" || !seen["no_permit_proven"] || len(seen) != 1 {
			return Invalid("history no-publication decision")
		}
	} else if r.Decision != "unresolved" || r.Operation.State == "no_publication" {
		return Invalid("history reconciliation decision")
	}
	return nil
}
