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

func graphMetadataFixture(ownerID string, occupied int) map[string]any {
	claims := []map[string]any{}
	for i := 0; i < occupied; i++ {
		claims = append(claims, map[string]any{
			"slot": []string{"1", "2"}[i], "attemptId": strings.Repeat([]string{"1", "2"}[i], 32),
			"graphId": nil, "nodeId": nil, "executorKind": "native",
			"contextId":                 []string{"codex-native", "claude-native"}[i],
			"capturedDeclaredAccountId": nil, "capturedLinkRevision": nil, "legacyBarrier": false,
		})
	}
	var sole any
	if occupied == 1 {
		sole = claims[0]["attemptId"]
	}
	return map[string]any{
		"ownerInstanceId": ownerID, "apiVersion": 1, "appVersion": "2.0.0-dev.6",
		"scope": "managed_execution", "managedExecution": "native_codex_claude",
		"executionContract": "006-v1", "workflowContract": "006-v1", "graphContract": "006-v1",
		"maintenanceContract": "owner-stop/1",
		"capabilities":        []string{"workspace_foundation", "managed_execution", "economical_routing", "verified_delivery", "task_graphs"},
		"admission":           map[string]any{"state": "open", "incidentId": nil},
		"capacity": map[string]any{"resourceId": "managed-native", "limit": 2, "activeAttemptId": sole,
			"occupiedSlots": []string{"0", "1", "2"}[occupied], "activeClaims": claims},
	}
}

func checkGraphMetadata(t *testing.T, metadata map[string]any, wantExecution, wantMaintenance bool) {
	t.Helper()
	id := metadata["ownerInstanceId"].(string)
	body, err := json.Marshal(domain.Success(200, domain.NewID(), metadata, nil).Envelope)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/meta" || r.Header.Get("Authorization") != "Bearer offline-metadata-fixture" {
			t.Error("metadata qualification attempted a nonmetadata or unauthenticated action")
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()
	desc := localstate.Descriptor{Origin: server.URL, OwnerInstanceID: id, Token: "offline-metadata-fixture"}
	if actual := verifyExecutionOwner(context.Background(), desc); actual != wantExecution {
		t.Fatalf("execution compatibility=%v want=%v", actual, wantExecution)
	}
	if actual := verifyMaintenanceOwner(context.Background(), desc); actual != wantMaintenance {
		t.Fatalf("maintenance compatibility=%v want=%v", actual, wantMaintenance)
	}
	if requests.Load() != 2 {
		t.Fatal("metadata qualification did not remain two read-only observations", requests.Load())
	}
}

func TestGraphMetadataRequiresExactCurrentPoolAndContracts(t *testing.T) {
	for _, occupied := range []int{0, 1, 2} {
		t.Run([]string{"empty", "sole", "two"}[occupied], func(t *testing.T) {
			checkGraphMetadata(t, graphMetadataFixture(domain.NewID(), occupied), true, true)
		})
	}
	for _, change := range []string{
		"missing_graph_contract", "future_graph_contract", "old_workflow_contract", "old_execution_contract", "missing_graph_capability",
		"old_pool_limit", "missing_count", "numeric_count", "noncanonical_count", "missing_claims", "null_claims", "count_mismatch",
		"duplicate_slot", "duplicate_attempt", "unordered_slots", "invalid_slot", "invalid_attempt", "missing_nullable_claim_field", "unknown_claim_field",
		"sole_marker_with_two", "invalid_link_revision", "numeric_link_revision", "node_without_graph", "null_legacy_barrier", "missing_marker",
		"local_native_context", "local_native_account", "local_native_link", "unpaired_account", "unpaired_link",
		"barrier_with_sibling", "barrier_slot_two", "native_missing_context", "graph_local_without_graph", "graph_local_with_node", "graph_nonlocal_without_node",
	} {
		t.Run(change, func(t *testing.T) {
			m := graphMetadataFixture(domain.NewID(), 2)
			pool := m["capacity"].(map[string]any)
			claims := pool["activeClaims"].([]map[string]any)
			switch change {
			case "missing_graph_contract":
				delete(m, "graphContract")
			case "future_graph_contract":
				m["graphContract"] = "007-v1"
			case "old_workflow_contract":
				m["workflowContract"] = "005-v1"
			case "old_execution_contract":
				m["executionContract"] = "005-v1"
			case "missing_graph_capability":
				m["capabilities"] = []string{"workspace_foundation", "managed_execution", "economical_routing", "verified_delivery"}
			case "old_pool_limit":
				pool["limit"] = 1
			case "missing_count":
				delete(pool, "occupiedSlots")
			case "numeric_count":
				pool["occupiedSlots"] = 2
			case "noncanonical_count":
				pool["occupiedSlots"] = "02"
			case "missing_claims":
				delete(pool, "activeClaims")
			case "null_claims":
				pool["activeClaims"] = nil
			case "count_mismatch":
				pool["occupiedSlots"] = "1"
			case "duplicate_slot":
				claims[1]["slot"] = claims[0]["slot"]
			case "duplicate_attempt":
				claims[1]["attemptId"] = claims[0]["attemptId"]
			case "unordered_slots":
				claims[0], claims[1] = claims[1], claims[0]
			case "invalid_slot":
				claims[0]["slot"] = "0"
			case "invalid_attempt":
				claims[0]["attemptId"] = "not-an-attempt"
			case "missing_nullable_claim_field":
				delete(claims[0], "graphId")
			case "unknown_claim_field":
				claims[0]["futureAuthority"] = true
			case "sole_marker_with_two":
				pool["activeAttemptId"] = claims[0]["attemptId"]
			case "invalid_link_revision":
				claims[0]["capturedLinkRevision"] = "9223372036854775808"
			case "numeric_link_revision":
				claims[0]["capturedLinkRevision"] = 1
			case "node_without_graph":
				claims[0]["nodeId"] = domain.NewID()
			case "null_legacy_barrier":
				claims[0]["legacyBarrier"] = nil
			case "missing_marker":
				delete(pool, "activeAttemptId")
			case "local_native_context":
				claims[0]["executorKind"] = "local_check"
			case "local_native_account":
				claims[0]["executorKind"], claims[0]["contextId"] = "local_application", nil
				claims[0]["capturedDeclaredAccountId"], claims[0]["capturedLinkRevision"] = domain.NewID(), "1"
			case "local_native_link":
				claims[0]["executorKind"], claims[0]["contextId"] = "local_external", nil
				claims[0]["capturedLinkRevision"] = "1"
			case "unpaired_account":
				claims[0]["capturedDeclaredAccountId"] = domain.NewID()
			case "unpaired_link":
				claims[0]["capturedLinkRevision"] = "1"
			case "barrier_with_sibling":
				claims[0]["legacyBarrier"] = true
			case "barrier_slot_two":
				claims[0]["slot"], claims[0]["legacyBarrier"] = "2", true
				pool["activeClaims"], pool["occupiedSlots"], pool["activeAttemptId"] = claims[:1], "1", claims[0]["attemptId"]
			case "native_missing_context":
				claims[0]["contextId"] = nil
			case "graph_local_without_graph":
				claims[0]["executorKind"], claims[0]["contextId"] = "local_graph", nil
			case "graph_nonlocal_without_node":
				claims[0]["graphId"] = domain.NewID()
			case "graph_local_with_node":
				claims[0]["executorKind"], claims[0]["contextId"] = "local_graph", nil
				claims[0]["graphId"], claims[0]["nodeId"] = domain.NewID(), domain.NewID()
			}
			checkGraphMetadata(t, m, false, false)
		})
	}
	for _, occupied := range []int{0, 1} {
		t.Run([]string{"empty_with_marker", "sole_wrong_marker"}[occupied], func(t *testing.T) {
			m := graphMetadataFixture(domain.NewID(), occupied)
			m["capacity"].(map[string]any)["activeAttemptId"] = domain.NewID()
			checkGraphMetadata(t, m, false, false)
		})
	}
}

func TestGraphMetadataSupportsActualLocalAndLegacyClaimScopes(t *testing.T) {
	for _, kind := range []string{"local_check", "local_application", "local_external", "local_graph", "legacy_unknown_native", "linked_native", "graph_native"} {
		t.Run(kind, func(t *testing.T) {
			m := graphMetadataFixture(domain.NewID(), 1)
			claim := m["capacity"].(map[string]any)["activeClaims"].([]map[string]any)[0]
			switch kind {
			case "legacy_unknown_native":
				claim["contextId"], claim["legacyBarrier"] = nil, true
			case "graph_native":
				claim["graphId"], claim["nodeId"] = domain.NewID(), domain.NewID()
			case "linked_native":
				claim["capturedDeclaredAccountId"], claim["capturedLinkRevision"] = domain.NewID(), "9223372036854775807"
			default:
				claim["executorKind"], claim["contextId"] = kind, nil
				if kind == "local_graph" {
					claim["graphId"] = domain.NewID()
				}
			}
			checkGraphMetadata(t, m, true, true)
		})
	}
}

func TestGraphMetadataPreservesKnownOldOwnerStopWithoutExecution(t *testing.T) {
	for _, version := range []string{"2", "3", "4", "5"} {
		t.Run("dev"+version, func(t *testing.T) {
			m := graphMetadataFixture(domain.NewID(), 0)
			m["appVersion"], m["executionContract"] = "2.0.0-dev."+version, "00"+version+"-v1"
			delete(m, "graphContract")
			m["capabilities"] = []string{"workspace_foundation", "managed_execution", "economical_routing", "verified_delivery"}
			m["capacity"] = map[string]any{"resourceId": "managed-native", "limit": 1, "activeAttemptId": nil}
			if version != "5" {
				delete(m, "workflowContract")
			} else {
				m["workflowContract"] = "005-v1"
			}
			if version == "2" {
				m["managedExecution"] = "native_codex"
				delete(m, "maintenanceContract")
				m["capabilities"] = []string{"workspace_foundation", "managed_execution"}
				m["capacity"] = map[string]any{"connectionId": "codex-native", "limit": 1, "activeAttemptId": nil}
			}
			checkGraphMetadata(t, m, false, true)
			m["appVersion"] = "2.0.0-dev.99"
			checkGraphMetadata(t, m, false, false)
		})
	}
}
