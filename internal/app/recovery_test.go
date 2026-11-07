package app

import (
	"context"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type recoveryControllerSpy struct {
	ExecutionController
	calls  int
	op, id string
	input  any
}

func (s *recoveryControllerSpy) result(op, id, requestID string, input any) domain.Result {
	s.calls++
	s.op, s.id, s.input = op, id, input
	return domain.Success(200, requestID, "captured", &domain.Receipt{RequestID: requestID, Operation: op, CommittedAt: "test"})
}
func (s *recoveryControllerSpy) PauseTask(_ context.Context, id string, in domain.PauseTaskInput) domain.Result {
	return s.result("tasks.pause", id, in.RequestID, in)
}
func (s *recoveryControllerSpy) PauseGraph(_ context.Context, id string, in domain.PauseGraphInput) domain.Result {
	return s.result("graphs.pause", id, in.RequestID, in)
}
func (s *recoveryControllerSpy) ResumeCheckpoint(_ context.Context, id string, in domain.ResumeCheckpointInput) domain.Result {
	return s.result("checkpoints.resume", id, in.RequestID, in)
}
func (s *recoveryControllerSpy) ReconcileIncident(_ context.Context, id string, in domain.ReconciliationInput) domain.Result {
	return s.result("incidents.reconcile", id, in.RequestID, in)
}
func (s *recoveryControllerSpy) Read(_ context.Context, op, id string, _ map[string]string) domain.Result {
	s.calls++
	s.op, s.id = op, id
	return domain.Success(200, domain.NewID(), "observed", nil)
}

func TestRecoveryServiceRoutesCapturedIntentAndReceipt(t *testing.T) {
	resource, request, node := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	for _, tc := range []struct{ op, body string }{
		{"tasks.pause", `{"requestId":"` + request + `","taskVersion":"1","expectedTaskRevision":"2"}`},
		{"graphs.pause", `{"requestId":"` + request + `","graphVersion":"1","expectedGraphControlRevision":"2","expectedBudgetAuthorityEpoch":"3","expectedWindowNumber":"4","nodeAuthorities":[{"nodeId":"` + node + `","taskVersion":"1","nodeControlRevision":"2"}]}`},
		{"checkpoints.resume", `{"requestId":"` + request + `","expectedCheckpointRevision":"3","scope":{"kind":"task","taskId":"` + resource + `","taskVersion":"1","expectedTaskRevision":"2"}}`},
		{"incidents.reconcile", `{"requestId":"` + request + `","expectedIncidentRevision":"3","action":"verify_and_reconcile"}`},
	} {
		t.Run(tc.op, func(t *testing.T) {
			spy := &recoveryControllerSpy{}
			result := (&Service{Execution: spy}).Execute(context.Background(), tc.op, Params{ID: resource, Body: []byte(tc.body)})
			if result.Envelope.Error != nil || spy.calls != 1 || spy.op != tc.op || spy.id != resource || result.Envelope.RequestID != request || result.Envelope.Receipt == nil || result.Envelope.Receipt.RequestID != request || result.Envelope.Receipt.Operation != tc.op {
				t.Fatalf("wrong captured request or acknowledgement: %#v %#v", result, spy)
			}
		})
	}
	for _, op := range []string{"checkpoints.get", "incidents.get"} {
		spy := &recoveryControllerSpy{}
		result := (&Service{Execution: spy}).Execute(context.Background(), op, Params{ID: resource})
		if result.Envelope.Error != nil || spy.calls != 1 || spy.op != op || spy.id != resource {
			t.Fatalf("wrong recovery read: %#v %#v", result, spy)
		}
	}
}

func TestRecoveryServiceRejectsInvalidTransportBeforeOwner(t *testing.T) {
	id, request := strings.Repeat("1", 32), strings.Repeat("2", 32)
	validTask := `{"requestId":"` + request + `","taskVersion":"1","expectedTaskRevision":"2"}`
	for _, tc := range []struct {
		op     string
		params Params
	}{
		{"tasks.pause", Params{ID: "bad", Body: []byte(validTask)}},
		{"tasks.pause", Params{ID: id, Body: []byte(`{"requestId":"` + request + `","taskVersion":"1","expectedTaskRevision":"2","proof":true}`)}},
		{"tasks.pause", Params{ID: id, Body: []byte(`{"requestId":"` + request + `","taskVersion":"01","expectedTaskRevision":"2"}`)}},
		{"graphs.pause", Params{ID: id, Body: []byte(`{"requestId":"` + request + `","graphVersion":"1","expectedGraphControlRevision":"2","expectedBudgetAuthorityEpoch":"1","expectedWindowNumber":"1","nodeAuthorities":[]}`)}},
		{"checkpoints.resume", Params{ID: id, Body: []byte(`{"requestId":"` + request + `","expectedCheckpointRevision":"1","scope":{"kind":"task","taskId":"` + id + `","taskVersion":"1","expectedTaskRevision":"2","nodeId":"` + id + `"}}`)}},
		{"incidents.reconcile", Params{ID: id, Body: []byte(`{"requestId":"` + request + `","expectedIncidentRevision":"1","action":"trust_me"}`)}},
		{"incidents.get", Params{ID: id, Body: []byte(`{}`)}},
		{"checkpoints.get", Params{ID: id, Query: map[string]string{"proof": "yes"}}},
	} {
		spy := &recoveryControllerSpy{}
		result := (&Service{Execution: spy}).Execute(context.Background(), tc.op, tc.params)
		if result.Envelope.Error == nil || spy.calls != 0 {
			t.Fatalf("invalid recovery request reached owner: %s %#v", tc.op, result)
		}
	}
	for _, op := range []string{"tasks.pause", "graphs.pause", "checkpoints.get", "incidents.reconcile"} {
		result := (&Service{}).Execute(context.Background(), op, Params{ID: id, Body: []byte(validTask)})
		if result.Envelope.Error == nil || result.Envelope.Error.Code != "capability_unavailable" {
			t.Fatalf("bootstrap acquired recovery authority: %s %#v", op, result)
		}
	}
}
