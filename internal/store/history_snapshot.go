package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const historyCaptureOperation = "history.snapshots.create"

// CaptureHistory persists an immutable, project-scoped projection and its
// receipt in one transaction. It does not decode source records or payloads.
func (s *Store) CaptureHistory(ctx context.Context, in domain.CaptureHistoryInput) domain.Result {
	if err := domain.ValidateCaptureHistoryInput(in); err != nil {
		return domain.Failure(in.RequestID, err)
	}
	return s.mutate(ctx, in.RequestID, historyCaptureOperation, Fingerprint(historyCaptureOperation, in), func(tx *sql.Tx) (int, any, error) {
		snapshot, err := captureHistorySnapshot(ctx, tx, in.ProjectID)
		if err != nil {
			return 0, nil, err
		}
		cursorKey := make([]byte, 32)
		if _, err = rand.Read(cursorKey); err != nil {
			return 0, nil, storageError()
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO history_snapshots(id,project_id,revision,data,cursor_key) VALUES(?,?,1,?,?)", snapshot.Summary.SnapshotID, in.ProjectID, encode(snapshot), cursorKey); err != nil {
			return 0, nil, storageError()
		}
		return 201, struct {
			Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
		}{snapshot.Summary}, nil
	})
}

func historyTooLarge() *domain.Error {
	return domain.Fail("history_too_large", "This project's metadata exceeds the supported history snapshot limit.")
}

func captureHistorySnapshot(ctx context.Context, tx *sql.Tx, projectID string) (domain.HistorySnapshot, error) {
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE id=?", projectID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return domain.HistorySnapshot{}, notFound()
	} else if err != nil {
		return domain.HistorySnapshot{}, storageError()
	}

	s := domain.HistorySnapshot{
		Summary: domain.HistorySnapshotSummary{SnapshotID: domain.NewID(), Revision: "1", ProjectID: projectID, Profile: domain.HistoryProfile, Completeness: domain.HistoryCompleteness, Notice: domain.HistoryNotice, CapturedAt: domain.Now(), IncludedCategories: []string{"project_identity", "task_metadata", "attempt_metadata"}, OmittedCategories: []string{"task_text", "project_paths", "user_files", "artifact_payloads", "provider_output", "vendor_authentication", "owner_authority", "legacy_state"}, AdditionalOmissions: []domain.HistoryOmission{}},
		Tasks:   []domain.HistoryTaskRow{}, Attempts: []domain.HistoryAttemptRow{},
	}
	// LEFT JOIN detects a missing current version instead of hiding the row.
	rows, err := tx.QueryContext(ctx, `SELECT t.id,t.project_id,t.revision,t.state_revision,t.current_version,v.version
FROM tasks t LEFT JOIN task_versions v ON v.task_id=t.id AND v.version=t.current_version
WHERE t.project_id=? ORDER BY t.sequence LIMIT ?`, projectID, domain.HistoryMaxTasks+1)
	if err != nil {
		return domain.HistorySnapshot{}, storageError()
	}
	for rows.Next() {
		var r domain.HistoryTaskRow
		var revision, stateRevision, currentVersion int64
		var version sql.NullInt64
		if err = rows.Scan(&r.TaskID, &r.ProjectID, &revision, &stateRevision, &currentVersion, &version); err != nil || !version.Valid || version.Int64 != currentVersion {
			rows.Close()
			return domain.HistorySnapshot{}, storageError()
		}
		if len(s.Tasks) == domain.HistoryMaxTasks {
			rows.Close()
			return domain.HistorySnapshot{}, historyTooLarge()
		}
		r.Ordinal = strconv.Itoa(len(s.Tasks) + 1)
		r.Revision, r.StateRevision, r.CurrentVersion = strconv.FormatInt(revision, 10), strconv.FormatInt(stateRevision, 10), strconv.FormatInt(currentVersion, 10)
		s.Tasks = append(s.Tasks, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return domain.HistorySnapshot{}, storageError()
	}
	if err = rows.Close(); err != nil {
		return domain.HistorySnapshot{}, storageError()
	}

	rows, err = tx.QueryContext(ctx, `SELECT a.id,a.task_id,a.task_version,a.revision,a.phase,v.version
FROM attempts a JOIN tasks t ON t.id=a.task_id
LEFT JOIN task_versions v ON v.task_id=a.task_id AND v.version=a.task_version
WHERE t.project_id=? ORDER BY a.sequence LIMIT ?`, projectID, domain.HistoryMaxAttempts+1)
	if err != nil {
		return domain.HistorySnapshot{}, storageError()
	}
	for rows.Next() {
		var r domain.HistoryAttemptRow
		var taskVersion, revision int64
		var version sql.NullInt64
		if err = rows.Scan(&r.AttemptID, &r.TaskID, &taskVersion, &revision, &r.Phase, &version); err != nil || !version.Valid || version.Int64 != taskVersion {
			rows.Close()
			return domain.HistorySnapshot{}, storageError()
		}
		if len(s.Attempts) == domain.HistoryMaxAttempts {
			rows.Close()
			return domain.HistorySnapshot{}, historyTooLarge()
		}
		r.Ordinal = strconv.Itoa(len(s.Attempts) + 1)
		r.TaskVersion, r.Revision = strconv.FormatInt(taskVersion, 10), strconv.FormatInt(revision, 10)
		s.Attempts = append(s.Attempts, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return domain.HistorySnapshot{}, storageError()
	}
	if err = rows.Close(); err != nil {
		return domain.HistorySnapshot{}, storageError()
	}
	s.Summary.TaskCount, s.Summary.AttemptCount = strconv.Itoa(len(s.Tasks)), strconv.Itoa(len(s.Attempts))
	if domain.ValidateHistorySnapshot(s) != nil {
		return domain.HistorySnapshot{}, storageError()
	}
	// All fields are validated ASCII; key order does not change encoded length.
	// Include the exact outer format and a fixed-size digest in the bound.
	p := domain.HistoryPayload{Format: domain.HistoryOutputFormat, Snapshot: s.Summary, Tasks: s.Tasks, Attempts: s.Attempts, Integrity: domain.HistoryIntegrity{Algorithm: "sha256", Encoding: "hex-lower", SnapshotSHA256: strings.Repeat("0", 64)}}
	b, err := json.Marshal(p)
	if err != nil {
		return domain.HistorySnapshot{}, storageError()
	}
	if len(b) > domain.HistoryMaxPayloadBytes {
		return domain.HistorySnapshot{}, historyTooLarge()
	}
	return s, nil
}
