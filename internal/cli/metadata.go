package cli

import (
	"context"
	json "encoding/json/v2"
	"io"
	"net/http"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type ownerMetadata struct {
	OwnerInstanceID   string   `json:"ownerInstanceId"`
	APIVersion        int      `json:"apiVersion"`
	AppVersion        string   `json:"appVersion"`
	Scope             string   `json:"scope"`
	ManagedExecution  string   `json:"managedExecution"`
	Capabilities      []string `json:"capabilities,omitempty"`
	ExecutionContract string   `json:"executionContract,omitempty"`
	Admission         *struct {
		State      string                    `json:"state"`
		IncidentID *string                   `json:"incidentId"`
		Incident   *domain.ExecutionIncident `json:"incident,omitempty"`
	} `json:"admission,omitempty"`
	Capacity *struct {
		ConnectionID    string  `json:"connectionId"`
		Limit           int     `json:"limit"`
		ActiveAttemptID *string `json:"activeAttemptId"`
	} `json:"capacity,omitempty"`
}

func readOwnerMetadata(ctx context.Context, desc localstate.Descriptor) (ownerMetadata, bool) {
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
		APIVersion int           `json:"apiVersion"`
		RequestID  string        `json:"requestId"`
		Data       ownerMetadata `json:"data"`
	}
	err = json.Unmarshal(body, &envelope, json.RejectUnknownMembers(true))
	return envelope.Data, err == nil && envelope.APIVersion == 1 && envelope.Data.APIVersion == 1 && envelope.Data.OwnerInstanceID == desc.OwnerInstanceID && domain.ValidID(envelope.RequestID)
}

func executionMetadata(m ownerMetadata) bool {
	has := func(s string) bool {
		for _, c := range m.Capabilities {
			if c == s {
				return true
			}
		}
		return false
	}
	return m.AppVersion == domain.AppVersion && m.Scope == "managed_execution" && m.ManagedExecution == "native_codex" && m.ExecutionContract == domain.ExecutionContract && has("workspace_foundation") && has("managed_execution") && m.Admission != nil && (m.Admission.State == "open" || m.Admission.State == "closing" || m.Admission.State == "fenced") && m.Capacity != nil && m.Capacity.ConnectionID == "codex-native" && m.Capacity.Limit == 1
}
