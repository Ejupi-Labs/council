package mcphost

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/Ejupi-Labs/council/v2/internal/store"
	"github.com/google/jsonschema-go/jsonschema"
)

func workspaceSchemaAccepts(t *testing.T, name string, args map[string]any) bool {
	t.Helper()
	raw, ok := schemas.DraftLookup(name)
	if !ok {
		t.Fatalf("missing draft schema %s", name)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return resolved.Validate(args) == nil
}

func workspaceBody(t *testing.T, input map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

type workspaceStopFixture struct {
	calls              int
	ownerID, requestID string
}

func (*workspaceStopFixture) StartTask(context.Context, domain.StartTaskInput) domain.Result {
	panic("unexpected task start")
}
func (*workspaceStopFixture) Guidance(context.Context, string, domain.GuidanceInput) domain.Result {
	panic("unexpected guidance")
}
func (*workspaceStopFixture) StartAttempt(context.Context, string, domain.StartAttemptInput) domain.Result {
	panic("unexpected attempt")
}
func (*workspaceStopFixture) Stop(context.Context, string, string) domain.Result {
	panic("unexpected stop")
}
func (*workspaceStopFixture) AcceptResponse(context.Context, string, domain.ResponseAcceptanceInput) domain.Result {
	panic("unexpected acceptance")
}
func (f *workspaceStopFixture) Shutdown(_ context.Context, ownerID, requestID string) domain.Result {
	f.calls++
	f.ownerID, f.requestID = ownerID, requestID
	return domain.Success(200, requestID, nil, nil)
}
func (*workspaceStopFixture) Read(context.Context, string, string, map[string]string) domain.Result {
	panic("unexpected read")
}
func (*workspaceStopFixture) Meta(context.Context) (map[string]any, error) { return nil, nil }

func TestServiceStopDraftSchemaAgainstOwner(t *testing.T) {
	fixture := &workspaceStopFixture{}
	s := &app.Service{Execution: fixture}
	ownerID, requestID := domain.NewID(), domain.NewID()
	input := map[string]any{"requestId": requestID, "ownerInstanceId": ownerID}
	if !workspaceSchemaAccepts(t, "council_service_stop", map[string]any{"input": input}) {
		t.Fatal("valid stop rejected")
	}
	result := s.Execute(context.Background(), "service.stop", app.Params{Body: workspaceBody(t, input)})
	if result.Status != 200 || fixture.calls != 1 || fixture.ownerID != ownerID || fixture.requestID != requestID {
		t.Fatalf("unexpected stop result: %+v", result)
	}
	input["ownerInstanceId"] = "invalid"
	if workspaceSchemaAccepts(t, "council_service_stop", map[string]any{"input": input}) {
		t.Fatal("invalid owner ID accepted")
	}
	result = s.Execute(context.Background(), "service.stop", app.Params{Body: workspaceBody(t, input)})
	if result.Status != 400 || fixture.calls != 1 {
		t.Fatal("invalid stop reached execution controller")
	}
}

// This exercises the actual application/store owner boundary with a disposable
// database and controlled project inspector; no real provider is involved.
func TestWorkspaceDraftSchemasAgainstOwner(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "workspace.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := app.New(db)
	s.InspectProject = func(context.Context, string, string) (domain.ResolvedProject, error) {
		return domain.ResolvedProject{PhysicalIdentity: "workspace-schema-fixture", Observation: domain.ProjectObservation{ResolvedPath: root, RepositoryRoot: root, ObservedAt: domain.Now()}}, nil
	}
	s.ValidateProjectIdentity = func(string, string) error { return nil }
	call := func(name, operation string, args map[string]any, params app.Params, status int) domain.Result {
		t.Helper()
		if !workspaceSchemaAccepts(t, name, args) {
			t.Fatalf("%s rejected valid args", name)
		}
		result := s.Execute(ctx, operation, params)
		if result.Status != status {
			t.Fatalf("%s: status %d, error %+v", name, result.Status, result.Envelope.Error)
		}
		return result
	}
	openInput := map[string]any{"requestId": domain.NewID(), "selectedPath": ".", "basePath": root}
	opened := call("council_projects_open", "projects.open", map[string]any{"input": openInput}, app.Params{Body: workspaceBody(t, openInput)}, 201)
	project := opened.Envelope.Data.(domain.ProjectOpen).Project
	call("council_projects_list", "projects.list", map[string]any{"limit": "100"}, app.Params{Query: map[string]string{"limit": "100"}}, 200)
	call("council_projects_get", "projects.get", map[string]any{"id": project.ID}, app.Params{ID: project.ID}, 200)
	policy := s.Execute(ctx, "policy.get", app.Params{}).Envelope.Data.(domain.Policy)
	policyInput := map[string]any{"requestId": domain.NewID(), "expectedRevision": policy.Revision, "mode": "autonomous"}
	call("council_policy_set", "policy.set", map[string]any{"input": policyInput}, app.Params{Body: workspaceBody(t, policyInput)}, 200)
	createInput := map[string]any{"requestId": domain.NewID(), "projectId": project.ID, "originalText": "A real task"}
	created := call("council_drafts_create", "drafts.create", map[string]any{"input": createInput}, app.Params{Body: workspaceBody(t, createInput)}, 201)
	draft := created.Envelope.Data.(domain.Draft)
	call("council_drafts_list", "drafts.list", map[string]any{"projectId": project.ID, "limit": "1"}, app.Params{Query: map[string]string{"projectId": project.ID, "limit": "1"}}, 200)
	call("council_drafts_get", "drafts.get", map[string]any{"id": draft.ID, "revision": "1"}, app.Params{ID: draft.ID, Query: map[string]string{"revision": "1"}}, 200)
	updateInput := map[string]any{"requestId": domain.NewID(), "expectedRevision": draft.Revision, "originalText": "Updated task", "useCurrentPolicy": false}
	call("council_drafts_update", "drafts.update", map[string]any{"id": draft.ID, "input": updateInput}, app.Params{ID: draft.ID, Body: workspaceBody(t, updateInput)}, 200)

	// The owner rejects these invalid shapes before any application effect.
	for _, tc := range []struct {
		name, operation string
		args            map[string]any
		params          app.Params
	}{
		{"council_projects_list", "projects.list", map[string]any{"limit": "0"}, app.Params{Query: map[string]string{"limit": "0"}}},
		{"council_projects_get", "projects.get", map[string]any{"id": "bad"}, app.Params{ID: "bad"}},
		{"council_drafts_get", "drafts.get", map[string]any{"id": draft.ID, "revision": "9223372036854775808"}, app.Params{ID: draft.ID, Query: map[string]string{"revision": "9223372036854775808"}}},
		{"council_drafts_list", "drafts.list", map[string]any{"projectId": "bad"}, app.Params{Query: map[string]string{"projectId": "bad"}}},
		{"council_drafts_create", "drafts.create", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "projectId": project.ID, "originalText": nil}}, app.Params{}},
		{"council_drafts_create", "drafts.create", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "projectId": project.ID, "originalText": "task", "unknown": true}}, app.Params{}},
		{"council_policy_set", "policy.set", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "mode": "autonomous"}}, app.Params{}},
	} {
		if workspaceSchemaAccepts(t, tc.name, tc.args) {
			t.Errorf("%s accepted invalid arguments", tc.name)
		}
		if input, ok := tc.args["input"].(map[string]any); ok {
			tc.params.Body = workspaceBody(t, input)
		}
		if result := s.Execute(ctx, tc.operation, tc.params); result.Status != 400 {
			t.Errorf("%s owner accepted invalid arguments: %d", tc.name, result.Status)
		}
	}

	// These rules need the mandatory owner validator in addition to JSON Schema:
	// byte-aware/non-whitespace text and platform-dependent absolute paths.
	blank := map[string]any{"requestId": domain.NewID(), "projectId": project.ID, "originalText": " \t"}
	if !workspaceSchemaAccepts(t, "council_drafts_create", map[string]any{"input": blank}) {
		t.Fatal("blank text should reach owner validation")
	}
	if result := s.Execute(ctx, "drafts.create", app.Params{Body: workspaceBody(t, blank)}); result.Status != 400 {
		t.Fatal("owner accepted blank text")
	}
	relative := map[string]any{"requestId": domain.NewID(), "selectedPath": ".", "basePath": "relative"}
	if !workspaceSchemaAccepts(t, "council_projects_open", map[string]any{"input": relative}) {
		t.Fatal("relative path should reach owner validation")
	}
	if result := s.Execute(ctx, "projects.open", app.Params{Body: workspaceBody(t, relative)}); result.Status != 400 {
		t.Fatal("owner accepted relative base path")
	}
}
