package app

import (
	"context"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type graphServiceController struct {
	ExecutionController
	op, id string
	body   []byte
	calls  int
}

func (c *graphServiceController) Read(_ context.Context, op, id string, _ map[string]string) domain.Result {
	c.op, c.id, c.calls = op, id, c.calls+1
	return domain.Success(200, domain.NewID(), "read", nil)
}
func (c *graphServiceController) MutateGraph(_ context.Context, op, id string, body []byte) domain.Result {
	c.op, c.id, c.calls = op, id, c.calls+1
	c.body = append([]byte{}, body...)
	return domain.Success(200, domain.MutationIdentity(body), "controlled mutation dispatch", nil)
}

func TestGraphServiceDispatchPreservesExactReadAndMutationTarget(t *testing.T) {
	c := &graphServiceController{}
	s := &Service{Execution: c}
	g, selected := strings.Repeat("1", 32), strings.Repeat("2", 32)
	read := s.Execute(context.Background(), "graphs.inputs.members", Params{ID: g + "/" + selected, Query: map[string]string{"limit": "30"}})
	if read.Envelope.Error != nil || c.calls != 1 || c.op != "graphs.inputs.members" || c.id != g+"/"+selected {
		t.Fatal("Graph read lost its exact selector", read, c)
	}
	body := []byte(`{"requestId":"` + strings.Repeat("3", 32) + `","guidanceText":"\ufeff  exact\r\n東京  ","workflow":{"checks":[]}}`)
	r := s.Execute(context.Background(), "graphs.nodes.guidance", Params{ID: g + "/Case_1", Body: body})
	if r.Envelope.Error != nil || c.calls != 2 || c.op != "graphs.nodes.guidance" || c.id != g+"/Case_1" || string(c.body) != string(body) {
		t.Fatal("Graph mutation was reinterpreted before controller validation", r, c)
	}
}

func TestGraphServiceRejectsTransportViolationsBeforeDispatch(t *testing.T) {
	c := &graphServiceController{}
	s := &Service{Execution: c}
	for _, item := range []struct {
		op string
		p  Params
	}{
		{"graphs.get", Params{ID: strings.Repeat("1", 32), Body: []byte(`{}`)}},
		{"graphs.stop", Params{ID: strings.Repeat("1", 32), Query: map[string]string{"extraAuthority": "yes"}, Body: []byte(`{}`)}},
		{"graphs.start", Params{Body: []byte(strings.Repeat("x", domain.MaxGraphDeclarationBytes+1))}},
		{"graphs.unknown", Params{}},
	} {
		r := s.Execute(context.Background(), item.op, item.p)
		if r.Envelope.Error == nil || c.calls != 0 {
			t.Fatal("Invalid transport reached graph execution", item.op, r, c.calls)
		}
	}
}

func TestGraphServiceBootstrapCannotSupplyMutationAuthority(t *testing.T) {
	s := &Service{}
	r := s.Execute(context.Background(), "graphs.start", Params{Body: []byte(`{"requestId":"` + strings.Repeat("3", 32) + `"}`)})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "capability_unavailable" {
		t.Fatal("Bootstrap service supplied graph execution authority", r)
	}
}
