package app

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestDraftServiceValidationAndReplay(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	r := s.Execute(ctx, "projects.open", Params{Body: body(openRequest{RequestID: domain.NewID(), SelectedPath: "fixture", BasePath: t.TempDir()})})
	if r.Status != 201 {
		t.Fatal(r)
	}
	project := r.Envelope.Data.(domain.ProjectOpen).Project
	id := domain.NewID()
	input := createDraftRequest{RequestID: id, ProjectID: project.ID, OriginalText: "\ufeff exact\r\n text\ufffd "}
	r = s.Execute(ctx, "drafts.create", Params{Body: body(input)})
	if r.Status != 201 {
		t.Fatal(r)
	}
	draft := r.Envelope.Data.(domain.Draft)
	update := updateDraftRequest{RequestID: domain.NewID(), ExpectedRevision: "1", OriginalText: "new"}
	first := s.Execute(ctx, "drafts.update", Params{ID: draft.ID, Body: body(update)})
	if first.Status != 200 {
		t.Fatal(first)
	}
	r = s.Execute(ctx, "drafts.update", Params{ID: draft.ID, Body: body(update)})
	if r.Status != 200 || r.Envelope.Receipt.RequestID != first.Envelope.Receipt.RequestID {
		t.Fatal(r)
	}
	other := domain.NewID()
	r = s.Execute(ctx, "drafts.update", Params{ID: other, Body: body(update)})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "request_conflict" {
		t.Fatal(r)
	}
	r = s.Execute(ctx, "drafts.get", Params{ID: draft.ID, Query: map[string]string{"revision": "1"}})
	if r.Status != 200 || r.Envelope.Data.(domain.Draft).OriginalText != input.OriginalText {
		t.Fatal(r)
	}
	nullBody := []byte(`{"requestId":"` + domain.NewID() + `","expectedRevision":"2","originalText":"new","useCurrentPolicy":null}`)
	if r = s.Execute(ctx, "drafts.update", Params{ID: draft.ID, Body: nullBody}); r.Status != 400 {
		t.Fatal("accepted null boolean", r)
	}
	invalid := []byte(`{"requestId":"` + id + `","projectId":"` + project.ID + `","originalText":"text","other":true}`)
	r = s.Execute(ctx, "drafts.create", Params{Body: invalid})
	if r.Status != 400 || r.Envelope.RequestID != id || r.Envelope.Receipt != nil {
		t.Fatal(r)
	}
}
