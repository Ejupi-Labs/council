package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphLiveRegistrationsAreOwnerScopedPassiveAndHighwaterBound(t *testing.T) {
	s, first, f, owner := graphLocalFixture(t)
	ctx := context.Background()
	definition, e := s.GraphDefinitionPrivate(ctx, first.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	create := func() domain.GraphAdmission {
		in := definition.Original
		in.RequestID = domain.NewID()
		r := s.CreateGraph(ctx, owner, in, f)
		if r.Status != 201 {
			t.Fatal(r)
		}
		return r.Envelope.Data.(domain.GraphAdmission)
	}
	second, third := create(), create()
	if r := s.StopGraph(ctx, owner, second.Graph.ID, domain.GraphStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: "1"}); r.Status != 202 {
		t.Fatal(r)
	}
	page, e := s.LiveGraphRegistrations(ctx, owner, 1, "")
	if e != nil || len(page.Items) != 1 || page.Items[0].Graph.ID != first.Graph.ID || page.NextCursor == nil {
		t.Fatal("current owner live registrations unavailable", page, e)
	}
	later := create()
	next, e := s.LiveGraphRegistrations(ctx, owner, 1, *page.NextCursor)
	if e != nil || len(next.Items) != 1 || next.Items[0].Graph.ID != third.Graph.ID || next.Items[0].Graph.ID == later.Graph.ID || next.NextCursor != nil {
		t.Fatal("page resumed suspended/late registration", next, e)
	}
	foreign := domain.NewID()
	if page, e := s.LiveGraphRegistrations(ctx, foreign, 100, ""); e != nil || len(page.Items) != 0 {
		t.Fatal("foreign epoch registrations were treated as live", page, e)
	}
	if _, e = s.LiveGraphRegistrations(ctx, foreign, 1, *page.NextCursor); e == nil {
		t.Fatal("cursor lost its exact owner filter")
	}
	budget, e := s.GraphBudget(ctx, first.Graph.ID)
	if e != nil || budget.Budget.ConsumedAdmissions != "0" || budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("live enumeration granted execution", budget, e)
	}
}
