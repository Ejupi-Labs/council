package engine

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"reflect"
	"testing"
)

func TestWorkflowOmittedAutoAdmissionReplayHasIdenticalRequiredShape(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	created := e.db.CreateDraft(ctx, domain.NewID(), "Auto replay shape", project, "\ufeff Exact original\r\n café \ufffd instructions")
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	draft := created.Envelope.Data.(domain.Draft)
	in := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: draft.OriginalText, SourceDraftRef: &domain.DraftRef{DraftID: draft.ID, Revision: draft.Revision}, ExecutionMode: "fake"}
	first := e.StartTask(ctx, in)
	if first.Status != 201 || first.Envelope.Receipt == nil {
		t.Fatal(first)
	}
	admission := decodeAdmission(t, first)
	if admission.Workflow == nil || admission.Step == nil {
		t.Fatal("omitted workflow did not capture Auto", admission)
	}
	// The database is this test's known, locked private fixture. Read its stored
	// receipt directly to distinguish immutable SQL bytes from JSON map ordering.
	rawDB, err := sql.Open("sqlite", e.lock.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	var storedBefore string
	if err = rawDB.QueryRowContext(ctx, "SELECT envelope FROM receipts WHERE request_id=?", in.RequestID).Scan(&storedBefore); err != nil {
		t.Fatal(err)
	}
	policy, err := e.db.Policy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if changed := e.db.SetPolicy(ctx, domain.NewID(), "changed after Auto admission", policy.Revision, "supervised"); changed.Envelope.Error != nil {
		t.Fatal(changed)
	}
	if changed := e.db.UpdateDraft(ctx, domain.NewID(), "newer draft after Auto admission", draft.ID, draft.Revision, "Different later original", true); changed.Envelope.Error != nil {
		t.Fatal(changed)
	}
	replay := e.StartTask(ctx, in)
	if replay.Status != first.Status || replay.Envelope.Receipt == nil || *replay.Envelope.Receipt != *first.Envelope.Receipt {
		t.Fatal("receipt replay changed identity/status", replay)
	}
	var storedAfter string
	if err = rawDB.QueryRowContext(ctx, "SELECT envelope FROM receipts WHERE request_id=?", in.RequestID).Scan(&storedAfter); err != nil || storedBefore != storedAfter {
		t.Fatal("replay changed original SQL envelope bytes", err)
	}
	canonical := func(value any) []byte {
		t.Helper()
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err = json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		out, err := json.Marshal(decoded, json.Deterministic(true))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if !bytes.Equal(canonical(first.Envelope), canonical(replay.Envelope)) || !bytes.Equal(canonical(first.Envelope), canonical(jsontext.Value(storedBefore))) {
		t.Fatal("canonical wire payload differs from immutable receipt")
	}
	var decoded domain.Admission
	if err = decodeInteropData(replay.Envelope.Data, &decoded); err != nil {
		t.Fatal(err)
	}
	if admission.Attempt.ObservedAt != decoded.Attempt.ObservedAt || admission.Attempt.RemainingSeconds != decoded.Attempt.RemainingSeconds || admission.Version.RequestedPolicy != decoded.Version.RequestedPolicy || admission.Version.Pins != decoded.Version.Pins {
		t.Fatal("replay refreshed captured observation/policy/target", admission, decoded)
	}
	normalized := admission
	workflow := *admission.Workflow
	normalized.Workflow = &workflow
	if workflow.EvidenceUses == nil {
		workflow.EvidenceUses = []domain.EvidenceUse{}
	}
	if !reflect.DeepEqual(normalized, decoded) {
		t.Fatal("unexpected typed difference beyond required empty workflow evidence index", normalized, decoded)
	}
	if admission.Workflow.EvidenceUses == nil || decoded.Workflow.EvidenceUses == nil {
		t.Fatalf("required evidenceUses differs in first DTO/replay: firstNil=%v replayNil=%v; observedAtSame=%v; canonical wire and SQL receipt are unchanged", admission.Workflow.EvidenceUses == nil, decoded.Workflow.EvidenceUses == nil, admission.Attempt.ObservedAt == decoded.Attempt.ObservedAt)
	}
}
