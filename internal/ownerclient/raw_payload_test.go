package ownerclient

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"net/http"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestClientPreservesRawOwnerPayloadAndReceipt(t *testing.T) {
	requestID := domain.NewID()
	taskID := domain.NewID()
	c, _, token, actions := fixtureClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":1,"requestId":"` + requestID + `","data":{"metadata":{"large":9007199254740993,"nested":[9223372036854775807,null,{"fraction":0.1234567890123456789}]}},"receipt":{"requestId":"` + requestID + `","operation":"tasks.guidance","committedAt":"2026-01-02T03:04:05.000Z"}}`))
	})
	body := []byte(`{"requestId":"` + requestID + `"}`)
	result := c.Call(context.Background(), Request{Operation: "tasks.guidance", PathIDs: []string{taskID}, RequestID: requestID, Body: body})
	if result.Status != 200 || result.Envelope.Receipt == nil || result.Envelope.Receipt.RequestID != requestID || result.Envelope.Receipt.Operation != "tasks.guidance" || result.Envelope.Receipt.CommittedAt != "2026-01-02T03:04:05.000Z" {
		t.Fatalf("receipt changed: %+v", result.Envelope.Receipt)
	}
	raw, ok := result.Envelope.Data.(jsontext.Value)
	if !ok || !raw.IsValid() || !strings.Contains(string(raw), `"large":9007199254740993`) || !strings.Contains(string(raw), `9223372036854775807`) || !strings.Contains(string(raw), `0.1234567890123456789`) || !strings.Contains(string(raw), `null`) {
		t.Fatal("owner JSON data was altered")
	}
	encoded, err := json.Marshal(result.Envelope)
	if err != nil || !strings.Contains(string(encoded), `"large":9007199254740993`) || strings.Contains(string(encoded), token) {
		t.Fatal("serialized envelope changed data or exposed fixture token")
	}
	replay := c.Call(context.Background(), Request{Operation: "tasks.guidance", PathIDs: []string{taskID}, RequestID: requestID, Body: body})
	replayed, err := json.Marshal(replay.Envelope)
	if err != nil || string(replayed) != string(encoded) {
		t.Fatal("replayed owner receipt or data changed")
	}
	if actions.Load() != 2 {
		t.Fatal("unexpected owner action count")
	}
}
