package cli

import (
	"context"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func metadataFixture(ownerID string, old bool) map[string]any {
	m := map[string]any{"ownerInstanceId": ownerID, "apiVersion": 1, "appVersion": domain.AppVersion,
		"scope": "managed_execution", "managedExecution": "native_codex_claude", "executionContract": domain.ExecutionContract,
		"maintenanceContract": "owner-stop/1", "capabilities": []string{"workspace_foundation", "managed_execution"},
		"admission": map[string]any{"state": "open", "incidentId": nil},
		"capacity":  map[string]any{"resourceId": "managed-native", "limit": 1, "activeAttemptId": nil}}
	if old {
		m["appVersion"], m["managedExecution"], m["executionContract"] = "2.0.0-dev.2", "native_codex", "002-v1"
		delete(m, "maintenanceContract")
		m["capacity"] = map[string]any{"connectionId": "codex-native", "limit": 1, "activeAttemptId": nil}
	}
	return m
}

func TestMaintenanceWhitelistDoesNotGrantExecutionCompatibility(t *testing.T) {
	for _, old := range []bool{false, true} {
		for _, change := range []string{"none", "additive", "unknown_version", "wrong_owner", "wrong_api", "wrong_contract", "missing_capacity", "wrong_capacity", "wrong_admission", "missing_capability", "future_maintenance", "duplicate", "oversized"} {
			t.Run(map[bool]string{false: "current", true: "known002"}[old]+"/"+change, func(t *testing.T) {
				id := domain.NewID()
				m := metadataFixture(id, old)
				switch change {
				case "additive":
					m["futureMetadata"] = map[string]any{"additive": true}
				case "unknown_version":
					m["appVersion"] = "2.0.0-dev.999"
				case "wrong_owner":
					m["ownerInstanceId"] = domain.NewID()
				case "wrong_api":
					m["apiVersion"] = 2
				case "wrong_contract":
					m["executionContract"] = "future-v1"
				case "missing_capacity":
					delete(m, "capacity")
				case "wrong_capacity":
					m["capacity"].(map[string]any)["limit"] = 2
				case "wrong_admission":
					m["admission"].(map[string]any)["state"] = "unrecognized"
				case "missing_capability":
					m["capabilities"] = []string{"workspace_foundation"}
				case "future_maintenance":
					m["maintenanceContract"] = "owner-stop/2"
				}
				body, _ := json.Marshal(domain.Success(200, domain.NewID(), m, nil).Envelope)
				if change == "duplicate" {
					body = []byte(strings.Replace(string(body), `"apiVersion":1`, `"apiVersion":1,"apiVersion":1`, 1))
				}
				if change == "oversized" {
					body = append(body, []byte(strings.Repeat(" ", 8193))...)
				}
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Method != "GET" || r.URL.Path != "/api/v1/meta" || r.Header.Get("Authorization") != "Bearer fixture" {
						t.Error("maintenance made a nonmetadata or unauthenticated request")
					}
					_, _ = w.Write(body)
				}))
				defer server.Close()
				desc := localstate.Descriptor{Origin: server.URL, OwnerInstanceID: id, Token: "fixture"}
				wantMaintenance := change == "none" || change == "additive"
				if got := verifyMaintenanceOwner(context.Background(), desc); got != wantMaintenance {
					t.Fatalf("maintenance=%v wanted=%v", got, wantMaintenance)
				}
				wantExecution := !old && change == "none"
				if got := verifyExecutionOwner(context.Background(), desc); got != wantExecution {
					t.Fatalf("execution=%v wanted=%v", got, wantExecution)
				}
				if requests.Load() != 2 {
					t.Fatal("unexpected maintenance side effects", requests.Load())
				}
			})
		}
	}
}

func TestUncertainStopResponseReportsCapturedExactRetry(t *testing.T) {
	old, request := domain.NewID(), domain.NewID()
	for _, responseKind := range []string{"malformed", "uncertain_envelope", "storage_failed"} {
		t.Run(responseKind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch responseKind {
				case "malformed":
					_, _ = w.Write([]byte("not an acknowledgement"))
				default:
					code := "write_uncertain"
					if responseKind == "storage_failed" {
						code = "storage_failed"
					}
					result := domain.Failure(request, domain.Fail(code, "Server could not acknowledge Stop."))
					body, _ := json.Marshal(result.Envelope)
					w.WriteHeader(result.Status)
					_, _ = w.Write(body)
				}
			}))
			defer server.Close()
			c, err := parse([]string{"service", "stop", "--request-id", request}, nil)
			if err != nil {
				t.Fatal(err)
			}
			c, err = bindServiceOwner(c, old)
			if err != nil {
				t.Fatal(err)
			}
			result := forward(context.Background(), localstate.Descriptor{Origin: server.URL, Token: "never-print-this-fixture-token"}, c)
			if result.Envelope.Error == nil || !strings.Contains(result.Envelope.Error.Message, "council service stop --owner-instance "+old+" --request-id "+request) || strings.Contains(result.Envelope.Error.Message, "never-print-this-fixture-token") {
				t.Fatal("uncertainty lost captured retry intent", result)
			}
		})
	}
}
