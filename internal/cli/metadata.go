package cli

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"net/http"
	"strconv"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type ownerMetadata struct {
	OwnerInstanceID     string   `json:"ownerInstanceId"`
	APIVersion          int      `json:"apiVersion"`
	AppVersion          string   `json:"appVersion"`
	Scope               string   `json:"scope"`
	ManagedExecution    string   `json:"managedExecution"`
	Capabilities        []string `json:"capabilities,omitempty"`
	ExecutionContract   string   `json:"executionContract,omitempty"`
	WorkflowContract    string   `json:"workflowContract,omitempty"`
	GraphContract       string   `json:"graphContract,omitempty"`
	RecoveryContract    string   `json:"recoveryContract,omitempty"`
	HistoryContract     string   `json:"historyContract,omitempty"`
	graphCapacityValid  bool
	MaintenanceContract string `json:"maintenanceContract,omitempty"`
	Admission           *struct {
		State      string                    `json:"state"`
		IncidentID *string                   `json:"incidentId"`
		Incident   *domain.ExecutionIncident `json:"incident,omitempty"`
	} `json:"admission,omitempty"`
	Capacity *struct {
		ConnectionID    string                  `json:"connectionId"`
		ResourceID      string                  `json:"resourceId"`
		Limit           int                     `json:"limit"`
		ActiveAttemptID *string                 `json:"activeAttemptId"`
		OccupiedSlots   *string                 `json:"occupiedSlots,omitempty"`
		ActiveClaims    []domain.GraphClaimView `json:"activeClaims,omitempty"`
	} `json:"capacity,omitempty"`
}

func readOwnerMetadata(ctx context.Context, desc localstate.Descriptor) (ownerMetadata, bool) {
	return readMetadata(ctx, desc, false)
}

// Maintenance alone tolerates additive members. Known compatibility fields and
// duplicate JSON members remain strict; task execution uses the current decoder.
func readMetadata(ctx context.Context, desc localstate.Descriptor, maintenance bool) (ownerMetadata, bool) {
	r, err := http.NewRequestWithContext(ctx, "GET", desc.Origin+"/api/v1/meta", nil)
	if err != nil {
		return ownerMetadata{}, false
	}
	r.Header.Set("Authorization", "Bearer "+desc.Token)
	response, err := client().Do(r)
	if err != nil {
		return ownerMetadata{}, false
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return ownerMetadata{}, false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(body) > 8192 {
		return ownerMetadata{}, false
	}
	var envelope struct {
		APIVersion int            `json:"apiVersion"`
		RequestID  string         `json:"requestId"`
		Data       jsontext.Value `json:"data"`
	}
	if err = json.Unmarshal(body, &envelope, json.RejectUnknownMembers(true)); err != nil || envelope.APIVersion != 1 || !domain.ValidID(envelope.RequestID) {
		return ownerMetadata{}, false
	}
	var metadata ownerMetadata
	err = json.Unmarshal(envelope.Data, &metadata, json.RejectUnknownMembers(!maintenance))
	if err == nil && metadata.AppVersion == domain.AppVersion {
		metadata.graphCapacityValid = validGraphMetadataCapacity(envelope.Data, metadata)
	}
	return metadata, err == nil && metadata.APIVersion == 1 && metadata.OwnerInstanceID == desc.OwnerInstanceID
}

func executionMetadata(m ownerMetadata) bool {
	return currentGraphMetadata(m) && m.RecoveryContract == "007-v1" && hasCapability(m, "safe_recovery")
}

// A pre-recovery 006 owner still has a recognized, separately versioned Stop
// contract. It must not receive new execution or recovery mutations.
func currentGraphMetadata(m ownerMetadata) bool {
	return managedMetadata(m) && m.AppVersion == domain.AppVersion && m.ManagedExecution == "native_codex_claude" && m.ExecutionContract == domain.ExecutionContract && m.WorkflowContract == domain.WorkflowContract && m.GraphContract == domain.GraphContract && m.MaintenanceContract == "owner-stop/1" && hasCapability(m, "economical_routing") && hasCapability(m, "verified_delivery") && hasCapability(m, "task_graphs") && m.graphCapacityValid
}

// The current pool has required nullable fields. A typed zero value alone cannot
// distinguish an omitted observation from the explicitly reported empty pool.
func validGraphMetadataCapacity(data jsontext.Value, m ownerMetadata) bool {
	if m.Capacity == nil || m.Capacity.ResourceID != domain.ManagedNativeResourceID || m.Capacity.ConnectionID != "" || m.Capacity.Limit != 2 || m.Capacity.OccupiedSlots == nil {
		return false
	}
	var outer map[string]jsontext.Value
	if json.Unmarshal(data, &outer) != nil {
		return false
	}
	var members map[string]jsontext.Value
	if json.Unmarshal(outer["capacity"], &members) != nil || len(members) != 5 {
		return false
	}
	for _, key := range []string{"resourceId", "limit", "activeAttemptId", "occupiedSlots", "activeClaims"} {
		if _, ok := members[key]; !ok {
			return false
		}
	}
	var claims []jsontext.Value
	if json.Unmarshal(members["activeClaims"], &claims) != nil || claims == nil || len(claims) > 2 || *m.Capacity.OccupiedSlots != strconv.Itoa(len(claims)) || len(claims) != len(m.Capacity.ActiveClaims) {
		return false
	}
	previousSlot := ""
	seen := map[string]bool{}
	for i, raw := range claims {
		var fields map[string]jsontext.Value
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 9 {
			return false
		}
		for _, key := range []string{"slot", "attemptId", "graphId", "nodeId", "executorKind", "contextId", "capturedDeclaredAccountId", "capturedLinkRevision", "legacyBarrier"} {
			if _, ok := fields[key]; !ok {
				return false
			}
		}
		var claim domain.GraphClaimView
		if json.Unmarshal(raw, &claim, json.RejectUnknownMembers(true)) != nil || string(fields["legacyBarrier"]) != "true" && string(fields["legacyBarrier"]) != "false" {
			return false
		}
		if claim.Slot != "1" && claim.Slot != "2" || claim.Slot <= previousSlot || !domain.ValidID(claim.AttemptID) || seen[claim.AttemptID] {
			return false
		}
		previousSlot, seen[claim.AttemptID] = claim.Slot, true
		for _, id := range []*string{claim.GraphID, claim.NodeID, claim.CapturedDeclaredAccountID} {
			if id != nil && !domain.ValidID(*id) {
				return false
			}
		}
		if claim.NodeID != nil && claim.GraphID == nil || claim.ContextID != nil && *claim.ContextID != "codex-native" && *claim.ContextID != "claude-native" {
			return false
		}
		switch claim.ExecutorKind {
		case "native", "local_check", "local_application", "local_external", "local_graph":
		default:
			return false
		}
		if (claim.CapturedDeclaredAccountID == nil) != (claim.CapturedLinkRevision == nil) || claim.LegacyBarrier && (len(claims) != 1 || claim.Slot != "1") {
			return false
		}
		if claim.ExecutorKind == "native" {
			if !claim.LegacyBarrier && claim.ContextID == nil {
				return false
			}
		} else if claim.ContextID != nil || claim.CapturedDeclaredAccountID != nil || claim.CapturedLinkRevision != nil {
			return false
		}
		if claim.ExecutorKind == "local_graph" {
			if claim.GraphID == nil || claim.NodeID != nil {
				return false
			}
		} else if claim.GraphID != nil && claim.NodeID == nil {
			return false
		}
		if claim.CapturedLinkRevision != nil {
			if _, err := domain.ParseRevision(*claim.CapturedLinkRevision); err != nil {
				return false
			}
		}
		// Use the strict decoding above for maintenance as well, where additive
		// top-level observations are allowed but known claim shapes stay exact.
		m.Capacity.ActiveClaims[i] = claim
	}
	if len(claims) == 1 {
		return m.Capacity.ActiveAttemptID != nil && *m.Capacity.ActiveAttemptID == m.Capacity.ActiveClaims[0].AttemptID
	}
	return m.Capacity.ActiveAttemptID == nil
}

func hasCapability(m ownerMetadata, value string) bool {
	for _, capability := range m.Capabilities {
		if capability == value {
			return true
		}
	}
	return false
}

func managedMetadata(m ownerMetadata) bool {
	return m.Scope == "managed_execution" && hasCapability(m, "workspace_foundation") && hasCapability(m, "managed_execution") && m.Admission != nil && (m.Admission.State == "open" || m.Admission.State == "closing" || m.Admission.State == "fenced") && m.Capacity != nil && (m.Capacity.ActiveAttemptID == nil || domain.ValidID(*m.Capacity.ActiveAttemptID)) && (m.Admission.IncidentID == nil || domain.ValidID(*m.Admission.IncidentID))
}

func maintenanceMetadata(m ownerMetadata) bool {
	if currentGraphMetadata(m) {
		return true
	}
	// Known previous owners expose the single-worker shape and are Stop-only.
	if !managedMetadata(m) || m.Capacity.Limit != 1 || m.Capacity.OccupiedSlots != nil || m.Capacity.ActiveClaims != nil || m.GraphContract != "" {
		return false
	}
	if m.AppVersion == "2.0.0-dev.5" && m.ManagedExecution == "native_codex_claude" && m.ExecutionContract == "005-v1" && m.WorkflowContract == "005-v1" && m.MaintenanceContract == "owner-stop/1" && hasCapability(m, "economical_routing") && hasCapability(m, "verified_delivery") && m.Capacity.ResourceID == domain.ManagedNativeResourceID && m.Capacity.ConnectionID == "" {
		return true
	}
	if hasCapability(m, "economical_routing") && m.AppVersion == "2.0.0-dev.4" && m.ManagedExecution == "native_codex_claude" && m.ExecutionContract == "004-v1" && m.MaintenanceContract == "owner-stop/1" && m.Capacity.ResourceID == domain.ManagedNativeResourceID && m.Capacity.ConnectionID == "" {
		return true
	}
	if managedMetadata(m) && m.AppVersion == "2.0.0-dev.3" && m.ManagedExecution == "native_codex_claude" && m.ExecutionContract == "003-v1" && m.MaintenanceContract == "owner-stop/1" && m.Capacity.ResourceID == domain.ManagedNativeResourceID && m.Capacity.ConnectionID == "" {
		return true
	}
	return managedMetadata(m) && m.AppVersion == "2.0.0-dev.2" && m.ManagedExecution == "native_codex" && m.ExecutionContract == "002-v1" && m.MaintenanceContract == "" && m.Capacity.ConnectionID == "codex-native" && m.Capacity.ResourceID == ""
}

func verifyMaintenanceOwner(ctx context.Context, desc localstate.Descriptor) bool {
	metadata, ok := readMetadata(ctx, desc, true)
	return ok && maintenanceMetadata(metadata)
}
