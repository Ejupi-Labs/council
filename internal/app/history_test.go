package app

import (
	"context"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type historyControllerSpy struct {
	ExecutionController
	calls  int
	op, id string
	input  any
}

func (s *historyControllerSpy) record(op, id, request string, input any) domain.Result {
	s.calls++
	s.op, s.id, s.input = op, id, input
	return domain.Success(200, request, "captured", &domain.Receipt{RequestID: request, Operation: op, CommittedAt: domain.Now()})
}
func (s *historyControllerSpy) CaptureHistory(_ context.Context, in domain.CaptureHistoryInput) domain.Result {
	return s.record("history.snapshots.create", "", in.RequestID, in)
}
func (s *historyControllerSpy) HistorySnapshot(_ context.Context, id string) domain.Result {
	return s.record("history.snapshots.get", id, domain.NewID(), nil)
}
func (s *historyControllerSpy) HistoryRecords(_ context.Context, id string, q domain.HistoryRecordsQuery) domain.Result {
	return s.record("history.snapshots.records", id, domain.NewID(), q)
}
func (s *historyControllerSpy) PreviewHistoryExport(_ context.Context, in domain.PreviewHistoryExportInput) domain.Result {
	return s.record("history.exports.preview", "", in.RequestID, in)
}
func (s *historyControllerSpy) ExecuteHistoryExport(_ context.Context, in domain.ExecuteHistoryExportInput) domain.Result {
	return s.record("history.exports.create", "", in.RequestID, in)
}
func (s *historyControllerSpy) HistoryOperation(_ context.Context, id string) domain.Result {
	return s.record("history.operations.get", id, domain.NewID(), nil)
}
func (s *historyControllerSpy) ReconcileHistoryOperation(_ context.Context, id string, in domain.ReconcileHistoryOperationInput) domain.Result {
	return s.record("history.operations.reconcile", id, in.RequestID, in)
}
func TestHistoryServiceRoutesStrictCapturedIntent(t *testing.T) {
	id, req := strings.Repeat("1", 32), strings.Repeat("2", 32)
	for _, tc := range []struct {
		op string
		p  Params
	}{
		{"history.snapshots.create", Params{Body: []byte(`{"requestId":"` + req + `","projectId":"` + id + `"}`)}},
		{"history.snapshots.get", Params{ID: id}},
		{"history.snapshots.records", Params{ID: id, Query: map[string]string{"kind": "tasks", "limit": "2"}}},
		{"history.exports.preview", Params{Body: []byte(`{"requestId":"` + req + `","snapshotId":"` + id + `","snapshotRevision":"1","filename":"history-a.json"}`)}},
		{"history.exports.create", Params{Body: []byte(`{"requestId":"` + req + `","previewId":"` + id + `","previewRevision":"1"}`)}},
		{"history.operations.get", Params{ID: id}},
		{"history.operations.reconcile", Params{ID: id, Body: []byte(`{"requestId":"` + req + `","expectedRevision":"1","action":"verify_no_publication"}`)}},
	} {
		t.Run(tc.op, func(t *testing.T) {
			spy := &historyControllerSpy{}
			r := (&Service{Execution: spy}).Execute(context.Background(), tc.op, tc.p)
			if r.Envelope.Error != nil || spy.calls != 1 || spy.op != tc.op || spy.id != tc.p.ID {
				t.Fatal(r, spy)
			}
			if len(tc.p.Body) > 0 && (r.Envelope.RequestID != req || r.Envelope.Receipt == nil || r.Envelope.Receipt.Operation != tc.op) {
				t.Fatal("lost request receipt", r)
			}
		})
	}
}
func TestHistoryServiceRejectsUntrustedProofAndMalformedInputs(t *testing.T) {
	id, req := strings.Repeat("1", 32), strings.Repeat("2", 32)
	for _, tc := range []struct {
		op string
		p  Params
	}{
		{"history.snapshots.create", Params{Body: []byte(`{"requestId":"` + req + `","projectId":"` + id + `","proof":true}`)}},
		{"history.exports.preview", Params{Body: []byte(`{"requestId":"` + req + `","snapshotId":"` + id + `","snapshotRevision":"1","filename":"../x"}`)}},
		{"history.operations.reconcile", Params{ID: id, Body: []byte(`{"requestId":"` + req + `","expectedRevision":"1","action":"trust_me"}`)}},
		{"history.snapshots.records", Params{ID: id, Query: map[string]string{"kind": "tasks", "limit": "101"}}},
		{"history.snapshots.records", Params{ID: id, Query: map[string]string{"kind": "tasks", "cursor": ""}}},
		{"history.snapshots.get", Params{ID: id, Body: []byte(`{}`)}},
	} {
		spy := &historyControllerSpy{}
		r := (&Service{Execution: spy}).Execute(context.Background(), tc.op, tc.p)
		if r.Envelope.Error == nil || spy.calls != 0 {
			t.Fatal("invalid history reached owner", tc.op, r)
		}
	}
	if r := (&Service{}).Execute(context.Background(), "history.snapshots.get", Params{ID: id}); r.Envelope.Error == nil || r.Envelope.Error.Code != "capability_unavailable" {
		t.Fatal("history ran without owner", r)
	}
}
