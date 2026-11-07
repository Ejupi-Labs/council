package store

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphAcknowledgedResponseFixture(t *testing.T) (*Store, domain.GraphDispatchView, domain.ResponseAcceptance) {
	t.Helper()
	s, d, f, _ := graphOutputSettlementFixture(t, true)
	ctx := context.Background()
	if _, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, f); e != nil {
		t.Fatal(e)
	}
	detail, e := s.Task(ctx, d.Dispatch.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	binding := f.ResponseBinding
	in := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, TaskVersion: detail.Version.Version, AttemptID: binding.ProducerAttemptID, ResponseArtifactID: binding.ResponseArtifactID, ResponseSHA256: binding.ResponseSHA256, Confirmation: "reviewed_response_and_conditions"}
	result := s.AcceptResponseWithWorkflow(ctx, d.Dispatch.TaskID, in, f.VerifiedArtifacts)
	if result.Status != 200 {
		t.Fatal("actual metadata acknowledgement transaction failed", result)
	}
	data, e := decode[struct {
		Task       domain.TaskSummary        `json:"task"`
		Acceptance domain.ResponseAcceptance `json:"acceptance"`
	}](encode(result.Envelope.Data))
	if e != nil {
		t.Fatal(e)
	}
	return s, d, data.Acceptance
}
func TestGraphResponseAcceptanceReadsExactCurrentAcknowledgement(t *testing.T) {
	s, d, expected := graphAcknowledgedResponseFixture(t)
	actual, e := s.GraphResponseAcceptance(context.Background(), d.Dispatch.GraphID, d.Dispatch.NodeID, expected.ID)
	if e != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatal("exact current response acknowledgement lost its immutable producer tuple", actual, e)
	}
}
func TestGraphResponseAcceptanceRefusesForeignOrMissingRecord(t *testing.T) {
	for _, kind := range []string{"foreign_graph", "foreign_node", "foreign_acknowledgement", "missing_row", "changed_row"} {
		t.Run(kind, func(t *testing.T) {
			s, d, record := graphAcknowledgedResponseFixture(t)
			graph, node, id := d.Dispatch.GraphID, d.Dispatch.NodeID, record.ID
			switch kind {
			case "foreign_graph":
				graph = domain.NewID()
			case "foreign_node":
				node = domain.NewID()
			case "foreign_acknowledgement":
				id = domain.NewID()
			case "missing_row":
				if e := s.executionWrite(context.Background(), func(tx *sql.Tx) error { _, e := tx.Exec("DELETE FROM response_acceptances WHERE id=?", id); return e }); e != nil {
					t.Fatal(e)
				}
			case "changed_row":
				if e := s.executionWrite(context.Background(), func(tx *sql.Tx) error {
					record.AttemptID = domain.NewID()
					_, e := tx.Exec("UPDATE response_acceptances SET data=? WHERE id=?", encode(record), id)
					return e
				}); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := s.GraphResponseAcceptance(context.Background(), graph, node, id); e == nil {
				t.Fatal("foreign/missing/corrupt acknowledgement was qualified")
			}
		})
	}
}
