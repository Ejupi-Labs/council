package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphResponseAcknowledgementAddsOneFreshEventWithoutRewritingTerminal(t *testing.T) {
	s, dispatch, f, owner := graphOutputSettlementFixture(t, true)
	ctx := context.Background()
	if _, e := s.SettleWorkflowStep(ctx, dispatch.Dispatch.AttemptID, f); e != nil {
		t.Fatal(e)
	}
	var terminalBefore string
	if e := s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", dispatch.Dispatch.AttemptID).Scan(&terminalBefore); e != nil {
		t.Fatal(e)
	}
	task, e := s.Task(ctx, dispatch.Dispatch.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	b := f.ResponseBinding
	in := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: task.Task.Revision, TaskVersion: task.Version.Version, AttemptID: b.ProducerAttemptID, ResponseArtifactID: b.ResponseArtifactID, ResponseSHA256: b.ResponseSHA256, Confirmation: "reviewed_response_and_conditions"}
	r := s.AcceptResponseWithWorkflow(ctx, task.Task.ID, in, f.VerifiedArtifacts)
	if r.Status != 200 {
		t.Fatal(r)
	}
	view, e := s.Graph(ctx, dispatch.Dispatch.GraphID)
	if e != nil || view.Nodes[0].AcceptedResult == nil {
		t.Fatal(view, e)
	}
	ack := view.Nodes[0].AcceptedResult.AcceptanceID
	var acceptanceID, requestID, graphID, nodeID, version, eventOwner string
	e = s.db.QueryRow("SELECT json_extract(data,'$.metadata.acceptanceId'),json_extract(data,'$.metadata.requestId'),json_extract(data,'$.metadata.graphId'),json_extract(data,'$.metadata.nodeId'),json_extract(data,'$.metadata.taskVersion'),json_extract(data,'$.metadata.ownerInstanceId') FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='graph_response_acknowledged'", b.ProducerAttemptID).Scan(&acceptanceID, &requestID, &graphID, &nodeID, &version, &eventOwner)
	if e != nil || acceptanceID != ack || requestID != in.RequestID || graphID != dispatch.Dispatch.GraphID || nodeID != dispatch.Dispatch.NodeID || version != dispatch.Dispatch.TaskVersion || eventOwner != owner {
		t.Fatal("actual acknowledgement lacks its exact fresh graph event", acceptanceID, requestID, graphID, nodeID, version, eventOwner, e)
	}
	if replay := s.AcceptResponseWithWorkflow(ctx, task.Task.ID, in, nil); replay.Status != 200 {
		t.Fatal("exact receipt replay needed fresh evidence or current native authority", replay)
	}
	var count int
	var terminalAfter string
	if e = s.db.QueryRow("SELECT count(*) FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='graph_response_acknowledged'", b.ProducerAttemptID).Scan(&count); e != nil || count != 1 {
		t.Fatal("receipt replay manufactured a new dispatch event", count, e)
	}
	if e = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", b.ProducerAttemptID).Scan(&terminalAfter); e != nil || terminalAfter != terminalBefore {
		t.Fatal("human acknowledgement rewrote immutable producer terminal", e)
	}
}

func TestGraphResponseAcknowledgementNeedsFreshAckOrLaterActualRelease(t *testing.T) {
	for _, trigger := range []string{"old_completion", "old_release", "response_ack", "later_release"} {
		t.Run(trigger, func(t *testing.T) {
			s, view, facts, owner := graphDependentInputFixture(t)
			ctx := context.Background()
			ack := view.Nodes[0].AcceptedResult.AcceptanceID
			switch trigger {
			case "old_completion":
				facts.Trigger.Kind, facts.Trigger.ID = "completion", facts.Control.OperationAttemptID
			case "old_release":
				facts.Trigger.Kind, facts.Trigger.ID = "claim_release", facts.Control.OperationAttemptID
			case "response_ack":
				facts.Trigger.Kind = "response_ack"
				facts.Trigger.ID = ack
			case "later_release":
				in := taskInput(view.Graph.ProjectID)
				r := s.CreateTask(ctx, owner, in)
				if r.Status != 201 {
					t.Fatal(r)
				}
				a := r.Envelope.Data.(domain.Admission).Attempt
				if _, e := s.ClaimAttempt(ctx, a.ID, owner); e != nil {
					t.Fatal(e)
				}
				if _, e := s.SettleAttempt(ctx, a.ID, domain.Settlement{Outcome: "failed", NoProcessCreated: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}); e != nil {
					t.Fatal(e)
				}
				facts.Trigger.Kind, facts.Trigger.ID = "claim_release", a.ID
				facts.Control.OperationAttemptID = a.ID
			}
			var before int
			if e := s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&before); e != nil {
				t.Fatal(e)
			}
			op, e := s.BeginGraphInput(ctx, owner, view.Graph.ID, view.Nodes[1].ID, facts)
			if trigger == "old_completion" || trigger == "old_release" {
				if e == nil {
					t.Fatal("older unaccepted producer callback became authority after human acknowledgement", op)
				}
				var after int
				if e = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&after); e != nil || after != before {
					t.Fatal("old callback partly created work", before, after, e)
				}
				return
			}
			if e != nil || op.Contributions[0].AcceptanceID != ack {
				t.Fatal("fresh acknowledgement/later real release could not register current accepted input", op, e)
			}
			if _, e = s.BeginGraphInput(ctx, owner, view.Graph.ID, view.Nodes[1].ID, facts); e == nil {
				t.Fatal("same acknowledged callback registered duplicate work")
			}
		})
	}
}
