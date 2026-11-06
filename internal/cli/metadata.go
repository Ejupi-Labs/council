package cli

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"net/http"

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
	MaintenanceContract string   `json:"maintenanceContract,omitempty"`
	Admission           *struct {
		State      string                    `json:"state"`
		IncidentID *string                   `json:"incidentId"`
		Incident   *domain.ExecutionIncident `json:"incident,omitempty"`
	} `json:"admission,omitempty"`
	Capacity *struct {
		ConnectionID    string  `json:"connectionId"`
		ResourceID      string  `json:"resourceId"`
		Limit           int     `json:"limit"`
		ActiveAttemptID *string `json:"activeAttemptId"`
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
	return metadata, err == nil && metadata.APIVersion == 1 && metadata.OwnerInstanceID == desc.OwnerInstanceID
}

func executionMetadata(m ownerMetadata) bool {
	return hasCapability(m, "economical_routing") && managedMetadata(m) && m.AppVersion == domain.AppVersion && m.ManagedExecution == "native_codex_claude" && m.ExecutionContract == domain.ExecutionContract && m.MaintenanceContract == "owner-stop/1" && m.Capacity.ResourceID == domain.ManagedNativeResourceID && m.Capacity.ConnectionID == ""
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
	has := func(s string) bool {
		for _, c := range m.Capabilities {
			if c == s {
				return true
			}
		}
		return false
	}
	return m.Scope == "managed_execution" && has("workspace_foundation") && has("managed_execution") && m.Admission != nil && (m.Admission.State == "open" || m.Admission.State == "closing" || m.Admission.State == "fenced") && m.Capacity != nil && m.Capacity.Limit == 1 && (m.Capacity.ActiveAttemptID == nil || domain.ValidID(*m.Capacity.ActiveAttemptID)) && (m.Admission.IncidentID == nil || domain.ValidID(*m.Admission.IncidentID))
}

func maintenanceMetadata(m ownerMetadata) bool {
	if executionMetadata(m) {
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
