package app

import (
	"context"
	"path/filepath"
	"testing"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

func testService(t *testing.T) *Service {
	t.Helper()
	db, e := store.Open(filepath.Join(t.TempDir(), "workspace.sqlite3"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	s := New(db)
	s.InspectProject = func(context.Context, string, string) (domain.ResolvedProject, error) {
		return domain.ResolvedProject{PhysicalIdentity: "fixture", Observation: domain.ProjectObservation{ResolvedPath: filepath.Join(t.TempDir(), "fixture"), RepositoryRoot: "fixture-root", ObservedAt: domain.Now()}}, nil
	}
	s.ValidateProjectIdentity = func(string, string) error { return nil }
	return s
}
func body(v any) []byte {
	raw, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return raw
}
func TestOpenReplayBeforeExternalInspection(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	id := domain.NewID()
	input := openRequest{RequestID: id, SelectedPath: "./fixture", BasePath: t.TempDir()}
	first := s.Execute(ctx, "projects.open", Params{Body: body(input)})
	if first.Status != 201 {
		t.Fatal(first)
	}
	s.InspectProject = func(context.Context, string, string) (domain.ResolvedProject, error) {
		t.Fatal("replay inspected moved project")
		return domain.ResolvedProject{}, nil
	}
	repeated := s.Execute(ctx, "projects.open", Params{Body: body(input)})
	if repeated.Status != first.Status || repeated.Envelope.Receipt.CommittedAt != first.Envelope.Receipt.CommittedAt {
		t.Fatal(repeated)
	}
	input.SelectedPath = "different"
	r := s.Execute(ctx, "projects.open", Params{Body: body(input)})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "request_conflict" {
		t.Fatal(r)
	}
}
func TestProjectReadRetainsRecordWhenObservationFails(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	r := s.Execute(ctx, "projects.open", Params{Body: body(openRequest{RequestID: domain.NewID(), SelectedPath: "fixture", BasePath: t.TempDir()})})
	if r.Status != 201 {
		t.Fatal(r)
	}
	p := r.Envelope.Data.(domain.ProjectOpen).Project
	s.ValidateProjectIdentity = func(string, string) error {
		return domain.Fail("project_identity_changed", "Restore the original folder.")
	}
	r = s.Execute(ctx, "projects.get", Params{ID: p.ID})
	if r.Status != 200 {
		t.Fatal(r)
	}
	data := r.Envelope.Data.(domain.ProjectGet)
	if data.Project.ID != p.ID || data.Observation != nil || data.ObservationError.Code != "project_identity_changed" {
		t.Fatal(data)
	}
}
