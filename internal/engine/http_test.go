package engine

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
)

func TestAuthenticatedHTTPFixtureAdmission(t *testing.T) {
	e, project, _ := testEngine(t)
	service := app.New(e.db)
	service.Execution = e
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler = httpapi.New(service, httpapi.Config{Host: server.Listener.Addr().String(), Token: "test-private-token", OwnerInstanceID: e.config.OwnerInstanceID})
	server.Start()
	defer server.Close()
	input := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Explicit authenticated HTTP fixture.", ExecutionMode: "fake", FakeScenario: "response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	r, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/tasks", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer test-private-token")
	r.Header.Set("Origin", server.URL)
	r.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 201 {
		t.Fatalf("HTTP %d: %s", response.StatusCode, body)
	}
	var envelope struct {
		Data domain.Admission `json:"data"`
	}
	if err = json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/tasks", "/tasks/" + envelope.Data.Task.ID + "/attempts"} {
		request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1"+route, nil)
		request.Header.Set("Authorization", "Bearer test-private-token")
		listed, listErr := server.Client().Do(request)
		if listErr != nil {
			t.Fatal(listErr)
		}
		listedBody, _ := io.ReadAll(listed.Body)
		listed.Body.Close()
		if listed.StatusCode != 200 {
			t.Fatalf("%s HTTP%d %s", route, listed.StatusCode, listedBody)
		}
	}
	a := settled(t, e, envelope.Data.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "succeeded" {
		t.Fatalf("%+v failure=%+v", a, a.PrimaryFailure)
	}
}
