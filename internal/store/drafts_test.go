package store

import (
	"context"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func openFixture(t *testing.T, s *Store, identity string) domain.Project {
	t.Helper()
	r := s.OpenProject(context.Background(), domain.NewID(), "open:"+identity, domain.OpenSelection{SelectedPath: identity, BasePath: "/base"}, domain.ResolvedProject{PhysicalIdentity: identity, Observation: domain.ProjectObservation{ResolvedPath: "/base/" + identity, RepositoryRoot: "/base", ObservedAt: domain.Now()}})
	if r.Status != 201 {
		t.Fatal(r)
	}
	return r.Envelope.Data.(domain.ProjectOpen).Project
}
func TestDraftImmutableVersionsAndPolicySnapshots(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "folder")
	text := "\ufeff Original \ufffd\r\n  exact\n"
	id := domain.NewID()
	r := s.CreateDraft(ctx, id, "create", project.ID, text)
	if r.Status != 201 {
		t.Fatal(r)
	}
	d := r.Envelope.Data.(domain.Draft)
	if d.OriginalText != text || d.Status != "draft" || d.Selection != "auto" || d.RequestedPolicy.Mode != "autonomous" {
		t.Fatal(d)
	}
	if r = s.SetPolicy(ctx, domain.NewID(), "policy", "1", "supervised"); r.Status != 200 {
		t.Fatal(r)
	}
	r = s.UpdateDraft(ctx, domain.NewID(), "update", d.ID, "1", "new", false)
	if r.Status != 200 {
		t.Fatal(r)
	}
	updated := r.Envelope.Data.(domain.Draft)
	if updated.Revision != "2" || updated.RequestedPolicy != d.RequestedPolicy || updated.CreatedAt != d.CreatedAt {
		t.Fatal(updated)
	}
	old, e := s.Draft(ctx, d.ID, "1")
	if e != nil || !reflect.DeepEqual(old, d) {
		t.Fatal(old, e)
	}
	r = s.UpdateDraft(ctx, domain.NewID(), "snapshot", d.ID, "2", "newest", true)
	if r.Status != 200 {
		t.Fatal(r)
	}
	newest := r.Envelope.Data.(domain.Draft)
	if newest.Revision != "3" || newest.RequestedPolicy.Mode != "supervised" || newest.RequestedPolicy.Revision != "2" {
		t.Fatal(newest)
	}
	if r = s.UpdateDraft(ctx, domain.NewID(), "stale", d.ID, "1", "lost", false); r.Envelope.Error == nil || r.Envelope.Error.Code != "revision_conflict" || r.Envelope.Error.CurrentRevision != "3" {
		t.Fatal(r)
	}
	r = s.CreateDraft(ctx, id, "create", project.ID, text)
	if r.Status != 201 {
		t.Fatal(r)
	}
	raw := encode(r.Envelope.Data)
	restored, e := decode[domain.Draft](raw)
	if e != nil || !reflect.DeepEqual(restored, d) {
		t.Fatal(restored, e)
	}
}
func TestStablePaginationAndExactLargeSequences(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "folder")
	for i := 0; i < 3; i++ {
		r := s.CreateDraft(ctx, domain.NewID(), strconv.Itoa(i), project.ID, strconv.Itoa(i))
		if r.Status != 201 {
			t.Fatal(r)
		}
	}
	// Exercise values above the safe JavaScript integer limit without floating-point conversion.
	base := int64(9007199254740993)
	if _, e := s.db.Exec("UPDATE drafts SET sequence=sequence+?", base); e != nil {
		t.Fatal(e)
	}
	first, e := s.Drafts(ctx, project.ID, 1, "")
	if e != nil || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatal(first, e)
	}
	max, after, e := decodeCursor(*first.NextCursor, "drafts", project.ID)
	if e != nil || max != base+3 || after != base+1 {
		t.Fatal(max, after, e)
	}
	if r := s.CreateDraft(ctx, domain.NewID(), "late", project.ID, "late"); r.Status != 201 {
		t.Fatal(r)
	}
	second, e := s.Drafts(ctx, project.ID, 100, *first.NextCursor)
	if e != nil || len(second.Items) != 2 || second.NextCursor != nil {
		t.Fatal(second, e)
	}
	if _, e = s.Drafts(ctx, "", 1, *first.NextCursor); e == nil {
		t.Fatal("cursor filter mismatch accepted")
	}
	for _, raw := range []string{"!", "1", *makeCursor("drafts", project.ID, math.MaxInt64, math.MaxInt64) + "=", *makeCursor("drafts", project.ID, 2, 3)} {
		if _, _, e = decodeCursor(raw, "drafts", project.ID); e == nil {
			t.Fatal("accepted cursor", raw)
		}
	}
}
func TestCheckedRevisionAndSequenceOverflow(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p, e := s.Policy(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p.Revision = strconv.FormatInt(math.MaxInt64, 10)
	if _, e = s.db.Exec("UPDATE policy SET revision=?,data=?", int64(math.MaxInt64), encode(p)); e != nil {
		t.Fatal(e)
	}
	r := s.SetPolicy(ctx, domain.NewID(), "overflow", p.Revision, "supervised")
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "revision_exhausted" {
		t.Fatal(r)
	}
	if _, e = s.db.Exec("UPDATE policy SET revision=revision+1"); e == nil {
		t.Fatal("SQLite REAL revision accepted")
	}
	project := openFixture(t, s, "folder")
	if _, e = s.db.Exec("UPDATE projects SET sequence=?", int64(math.MaxInt64)); e != nil {
		t.Fatal(e)
	}
	r = s.OpenProject(ctx, domain.NewID(), "overflow-project", domain.OpenSelection{}, domain.ResolvedProject{PhysicalIdentity: "another", Observation: domain.ProjectObservation{ResolvedPath: "/another"}})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "revision_exhausted" {
		t.Fatal(project, r)
	}
}
func TestProjectExistingAdmissionAndChangedIdentity(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "folder")
	selection := domain.OpenSelection{SelectedPath: "alias", BasePath: "/base"}
	fresh := domain.ResolvedProject{PhysicalIdentity: "folder", Observation: domain.ProjectObservation{ResolvedPath: p.ResolvedPath, RepositoryRoot: "/changed", ObservedAt: domain.Now()}}
	r := s.OpenProject(ctx, domain.NewID(), "alias", selection, fresh)
	if r.Status != 200 {
		t.Fatal(r)
	}
	opened := r.Envelope.Data.(domain.ProjectOpen)
	if !reflect.DeepEqual(opened.Project, p) || opened.OpenSelection != selection || opened.Observation.RepositoryRoot != "/changed" {
		t.Fatal(opened)
	}
	fresh.PhysicalIdentity = "replacement"
	r = s.OpenProject(ctx, domain.NewID(), "replacement", selection, fresh)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "project_identity_changed" {
		t.Fatal(r)
	}
}
