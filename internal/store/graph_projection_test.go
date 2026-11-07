package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphProjectionUsesActualResponseAcknowledgementAndAllRequiredNodes(t *testing.T) {
	s, d, record := graphAcknowledgedResponseFixture(t)
	ctx := context.Background()
	view, e := s.Graph(ctx, d.Dispatch.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	node := view.Nodes[0]
	if node.ID != d.Dispatch.NodeID || node.State != "completed" || node.AcceptedResult == nil || node.AcceptedResult.AcceptanceKind != "response" || node.AcceptedResult.AcceptanceID != record.ID || node.AcceptedResult.ProducerAttemptID != d.Dispatch.AttemptID || node.AcceptedResult.TaskVersion != d.Dispatch.TaskVersion || node.AcceptedResult.InputID != d.Dispatch.InputID || node.AcceptedResult.ResponseArtifactID == nil || *node.AcceptedResult.ResponseArtifactID != record.ResponseArtifactID || node.AcceptedResult.CandidateID != nil {
		t.Fatal("node did not bind its actual current response acknowledgement", node)
	}
	if view.Graph.State == "completed" || view.Graph.Counts.Completed != "1" || view.Graph.Counts.Required != "2" || view.Nodes[1].AcceptedResult != nil {
		t.Fatal("one accepted node completed an all-required group", view)
	}
}
func TestGraphProjectionDoesNotAcceptClosedOutputBeforeHumanResponseReview(t *testing.T) {
	s, d, f, _ := graphOutputSettlementFixture(t, true)
	ctx := context.Background()
	w, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, f)
	if e != nil || w.NextAction != "response_acceptance_pending" {
		t.Fatal(w, e)
	}
	view, e := s.Graph(ctx, d.Dispatch.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	n := view.Nodes[0]
	if n.AcceptedResult != nil || n.NextOwner != "user" || n.NextAction != "response_acceptance_pending" || n.State == "completed" {
		t.Fatal("closed bytes invented acceptance or wrong next owner", n)
	}
}
func TestGraphProjectionRejectsResponseAcknowledgementAfterAcceptedGroupStop(t *testing.T) {
	s, d, f, owner := graphOutputSettlementFixture(t, true)
	ctx := context.Background()
	if _, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, f); e != nil {
		t.Fatal(e)
	}
	if r := s.StopGraph(ctx, owner, d.Dispatch.GraphID, domain.GraphStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: "1"}); r.Status != 202 {
		t.Fatal(r)
	}
	task, e := s.Task(ctx, d.Dispatch.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	b := f.ResponseBinding
	in := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: task.Task.Revision, TaskVersion: task.Version.Version, AttemptID: b.ProducerAttemptID, ResponseArtifactID: b.ResponseArtifactID, ResponseSHA256: b.ResponseSHA256, Confirmation: "reviewed_response_and_conditions"}
	r := s.AcceptResponseWithWorkflow(ctx, d.Dispatch.TaskID, in, f.VerifiedArtifacts)
	if r.Status != 409 || r.Envelope.Error == nil || r.Envelope.Error.Code != "graph_control_conflict" {
		t.Fatal("human acknowledgement revived cancelled graph authority", r)
	}
	var records int
	if e = s.db.QueryRow("SELECT count(*) FROM response_acceptances WHERE attempt_id=?", d.Dispatch.AttemptID).Scan(&records); e != nil || records != 0 {
		t.Fatal("refused acknowledgement partly committed", records, e)
	}
}
func TestGraphProjectionPreservesFailedNodeOutcome(t *testing.T) {
	s, d, f, _ := graphOutputSettlementFixture(t)
	ctx := context.Background()
	f.Settlement.Outcome = "failed"
	bad := 17
	f.Settlement.ExitCode = &bad
	f.Settlement.Failure = &domain.PrimaryFailure{Code: "fixed_native_failure", Message: "Controlled metadata native failure."}
	if _, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, f); e != nil {
		t.Fatal(e)
	}
	view, e := s.Graph(ctx, d.Dispatch.GraphID)
	if e != nil || view.Nodes[0].State != "failed" || view.Nodes[0].NextOwner != "user" || view.Nodes[0].AcceptedResult != nil || view.Graph.Counts.Failed != "1" || view.Graph.State == "completed" {
		t.Fatal("failed supporting settlement still promised queued success", view, e)
	}
}
