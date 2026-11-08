package domain

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

const historyTestID = "0123456789abcdef0123456789abcdef"
const historyOtherID = "abcdef0123456789abcdef0123456789"
const historyAttemptID = "11111111111111111111111111111111"

func historyTestSnapshot() HistorySnapshot {
	return HistorySnapshot{
		Summary:  HistorySnapshotSummary{SnapshotID: historyTestID, Revision: "1", ProjectID: historyOtherID, Profile: HistoryProfile, Completeness: HistoryCompleteness, Notice: HistoryNotice, CapturedAt: "2026-11-08T00:00:00.000Z", TaskCount: "1", AttemptCount: "1", IncludedCategories: append([]string{}, historyIncluded...), OmittedCategories: append([]string{}, historyOmitted...), AdditionalOmissions: []HistoryOmission{}},
		Tasks:    []HistoryTaskRow{{Ordinal: "1", TaskID: historyTestID, ProjectID: historyOtherID, Revision: "9223372036854775807", StateRevision: "1", CurrentVersion: "1"}},
		Attempts: []HistoryAttemptRow{{Ordinal: "1", AttemptID: historyAttemptID, TaskID: historyTestID, TaskVersion: "1", Revision: "9007199254740993", Phase: "settled"}},
	}
}

func TestHistorySnapshotValidation(t *testing.T) {
	if e := ValidateHistorySnapshot(historyTestSnapshot()); e != nil {
		t.Fatal(e)
	}
	cases := map[string]func(*HistorySnapshot){
		"wrong project":     func(s *HistorySnapshot) { s.Tasks[0].ProjectID = historyTestID },
		"foreign attempt":   func(s *HistorySnapshot) { s.Attempts[0].TaskID = historyOtherID },
		"nonlocal ordinal":  func(s *HistorySnapshot) { s.Attempts[0].Ordinal = "2" },
		"unsafe phase":      func(s *HistorySnapshot) { s.Attempts[0].Phase = "accepted" },
		"overflow":          func(s *HistorySnapshot) { s.Tasks[0].Revision = "9223372036854775808" },
		"leading zero":      func(s *HistorySnapshot) { s.Tasks[0].Revision = "01" },
		"wrong count":       func(s *HistorySnapshot) { s.Summary.TaskCount = "0" },
		"missing omissions": func(s *HistorySnapshot) { s.Summary.AdditionalOmissions = nil },
		"missing notice":    func(s *HistorySnapshot) { s.Summary.Notice = "" },
		"changed notice":    func(s *HistorySnapshot) { s.Summary.Notice = "Complete backup" },
		"duplicate attempt": func(s *HistorySnapshot) {
			a := s.Attempts[0]
			a.Ordinal = "2"
			s.Attempts = append(s.Attempts, a)
			s.Summary.AttemptCount = "2"
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			s := historyTestSnapshot()
			edit(&s)
			if ValidateHistorySnapshot(s) == nil {
				t.Fatal("accepted invalid history")
			}
		})
	}
	s := historyTestSnapshot()
	s.Tasks = []HistoryTaskRow{}
	s.Attempts = []HistoryAttemptRow{}
	s.Summary.TaskCount = "0"
	s.Summary.AttemptCount = "0"
	if e := ValidateHistorySnapshot(s); e != nil {
		t.Fatalf("valid empty snapshot: %v", e)
	}
}

func TestHistoryFilenameAndStrictRequests(t *testing.T) {
	for _, name := range []string{"history-1.json", "history-A_1-2.json", "history-" + strings.Repeat("a", 51) + ".json"} {
		if !strings.HasPrefix(name, "history-") {
			t.Fatalf("valid test name lacks reserved history prefix: %s", name)
		}
		if !ValidHistoryFilename(name) {
			t.Errorf("valid filename rejected: %s", name)
		}
	}
	for _, name := range []string{"CON.json", "lpt9.json", "active.json", "owner.lock", "state.db", "history-a.JSON", "../history-a.json", "history-a..json", "history-a:b.json", "history-" + strings.Repeat("a", 52) + ".json"} {
		if ValidHistoryFilename(name) {
			t.Errorf("invalid filename accepted: %s", name)
		}
	}
	var in CaptureHistoryInput
	good := fmt.Sprintf(`{"requestId":"%s","projectId":"%s"}`, historyTestID, historyOtherID)
	if e := StrictDecodeHistory([]byte(good), &in); e != nil {
		t.Fatal(e)
	}
	if e := ValidateCaptureHistoryInput(in); e != nil {
		t.Fatal(e)
	}
	for _, body := range []string{
		fmt.Sprintf(`{"requestId":"%s","requestId":"%s","projectId":"%s"}`, historyTestID, historyTestID, historyOtherID),
		fmt.Sprintf(`{"requestId":"%s","projectId":"%s","extra":true}`, historyTestID, historyOtherID),
		fmt.Sprintf(`{"requestId":"%s","projectId":null}`, historyTestID),
		strings.Repeat(" ", HistoryMaxRequestBytes+1),
	} {
		var dst CaptureHistoryInput
		if StrictDecodeHistory([]byte(body), &dst) == nil {
			t.Errorf("invalid request accepted: %.100s", body)
		}
	}
	if ValidatePreviewHistoryExportInput(PreviewHistoryExportInput{RequestID: historyTestID, SnapshotID: historyOtherID, SnapshotRevision: "01", Filename: "history-1.json"}) == nil {
		t.Fatal("noncanonical revision accepted")
	}
	if ValidateReconcileHistoryOperationInput(ReconcileHistoryOperationInput{RequestID: historyTestID, ExpectedRevision: "1", Action: "verify_no_publication"}) != nil {
		t.Fatal("valid reconciliation rejected")
	}
}

func TestHistoryPublicationDurabilityAndReconciliation(t *testing.T) {
	dir := t.TempDir()
	outcome := "published"
	op := HistoryExportOperation{OperationID: historyTestID, RequestID: historyOtherID, Revision: "1", SnapshotID: historyTestID, PreviewID: historyOtherID, State: "published", Completeness: HistoryCompleteness, Outcome: &outcome, Durability: &HistoryDurability{FileFlush: "succeeded", DirectorySync: "unavailable"}, Blockers: []string{}, NextAction: "inspect", Filename: "history-1.json", DestinationDirectory: dir, DestinationPath: filepath.Join(dir, "history-1.json")}
	if e := ValidateHistoryExportOperation(op); e != nil {
		t.Fatal(e)
	}
	op.NextAction = "unexpected_private_detail"
	if ValidateHistoryExportOperation(op) == nil {
		t.Fatal("unknown next action accepted")
	}
	op.NextAction = "wait"
	op.Blockers = []string{"history_effect_uncertain", "history_effect_uncertain"}
	if ValidateHistoryExportOperation(op) == nil {
		t.Fatal("duplicate blocker accepted")
	}
	op.Blockers = []string{"unknown_error"}
	if ValidateHistoryExportOperation(op) == nil {
		t.Fatal("unknown blocker accepted")
	}
	op.Blockers = []string{}
	op.Durability.PowerLossNamespaceGuarantee = true
	if ValidateHistoryExportOperation(op) == nil {
		t.Fatal("unsupported power-loss guarantee accepted")
	}
	op.Durability = nil
	op.State = "no_publication"
	noEffect := "no_publication"
	op.Outcome = &noEffect
	r := HistoryReconciliationResult{Operation: op, Decision: "no_publication", ObservationCodes: []string{"no_permit_proven"}}
	if e := ValidateHistoryReconciliationResult(r); e != nil {
		t.Fatal(e)
	}
	r.ObservationCodes = []string{"actor_not_drained"}
	if ValidateHistoryReconciliationResult(r) == nil {
		t.Fatal("unproven no-publication accepted")
	}
}

func TestHistoryPagesAndQueryBounds(t *testing.T) {
	for _, q := range []HistoryRecordsQuery{{Kind: "tasks"}, {Kind: "attempts", Limit: "100", Cursor: "cursor"}} {
		if e := ValidateHistoryRecordsQuery(q); e != nil {
			t.Fatal(e)
		}
	}
	for _, q := range []HistoryRecordsQuery{{Kind: "events"}, {Kind: "tasks", Limit: "0"}, {Kind: "tasks", Limit: "01"}, {Kind: "tasks", Limit: "101"}, {Kind: "tasks", Cursor: strings.Repeat("x", HistoryMaxCursorBytes+1)}} {
		if ValidateHistoryRecordsQuery(q) == nil {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
	s := historyTestSnapshot()
	p := HistorySnapshotPage{SnapshotID: historyTestID, Revision: "1", Completeness: HistoryCompleteness, Notice: HistoryNotice, Kind: "tasks", Items: s.Tasks}
	if e := ValidateHistorySnapshotPage(p); e != nil {
		t.Fatal(e)
	}
	bad := p
	bad.Notice = ""
	if ValidateHistorySnapshotPage(bad) == nil {
		t.Fatal("page without notice accepted")
	}
	bad = p
	bad.Completeness = "complete"
	if ValidateHistorySnapshotPage(bad) == nil {
		t.Fatal("page with wrong completeness accepted")
	}
	p.Items = s.Attempts
	if ValidateHistorySnapshotPage(p) == nil {
		t.Fatal("mismatched page accepted")
	}
	if ValidateHistoryReceipt(HistoryReceipt{RequestID: historyTestID, Operation: "history.exports.create", CommittedAt: "2026-11-08T00:00:00.000Z"}) != nil {
		t.Fatal("valid receipt rejected")
	}
}
