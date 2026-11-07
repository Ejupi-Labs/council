package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestRecoveryMemberCapacityWaitWakesOnNewSiblingRelease(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx, e.cancel = ctx, cancel
	graph, oldCompletion := graphReleaseReadyGroup(t, e, projectID)
	report := e.db.CreateCapacityReport(ctx, domain.CapacityReportInput{RequestID: domain.NewID(), Subject: domain.CapacitySubject{Kind: "context", ID: "claude-native"}, ExpectedRevision: "1", Dimension: "occupancy", State: "busy"})
	if report.Envelope.Error != nil {
		t.Fatal(report)
	}
	view, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	member := view.Nodes[0]
	paused := e.db.PauseTask(ctx, e.config.OwnerInstanceID, member.TaskID, domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: member.TaskVersion, ExpectedTaskRevision: member.TaskRevision})
	if paused.Envelope.Error != nil {
		t.Fatal(paused)
	}
	cp := paused.Envelope.Data.(domain.RecoveryPauseResult).Checkpoint
	view, err = e.db.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	advance := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number}
	for _, node := range view.Nodes {
		advance.NodeAuthorities = append(advance.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	started := e.advanceGraph(ctx, graph.Graph.ID, advance)
	if started.Envelope.Error != nil {
		t.Fatal(started)
	}
	view, err = e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || view.Nodes[1].LatestAttemptID == nil {
		t.Fatal("sibling was not admitted", view, err)
	}
	siblingID := *view.Nodes[1].LatestAttemptID
	shared := *view.DispatchAuthority
	siblingBefore := view.Nodes[1]
	result := e.ResumeCheckpoint(ctx, cp.ID, domain.ResumeCheckpointInput{RequestID: domain.NewID(), ExpectedCheckpointRevision: cp.Revision, Scope: cp.Scope})
	if result.Envelope.Error != nil {
		t.Fatal(result)
	}
	waiting, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || waiting.Nodes[0].NodeAdmissionID != nil || waiting.DispatchAuthority == nil || !reflect.DeepEqual(*waiting.DispatchAuthority, shared) || !reflect.DeepEqual(waiting.Nodes[1], siblingBefore) || len(waiting.Blockers) == 0 || waiting.Blockers[0].Code != "capacity_wait" {
		t.Fatal("resume did not retain an exact capacity wait", waiting, err)
	}
	data, err := json.Marshal(result.Envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Blockers           []string `json:"blockers"`
		AdmittedAttemptIDs []string `json:"admittedAttemptIds"`
	}
	if err := json.Unmarshal(data, &receipt); err != nil || len(receipt.Blockers) != 1 || receipt.Blockers[0] != "capacity_wait" || len(receipt.AdmittedAttemptIDs) != 0 {
		t.Fatal("resume receipt did not report its exact capacity wait", string(data), err)
	}
	// An old completion cannot be reused to grant the newer registration authority.
	e.continueGraphAdmissions(oldCompletion)
	still, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || still.Nodes[0].NodeAdmissionID != nil {
		t.Fatal("old event woke scoped work", still, err)
	}
	sibling, err := e.db.ClaimAttempt(ctx, siblingID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, sibling)
	e.continueGraphAdmissions(sibling.ID)
	after, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || after.Nodes[0].NodeAdmissionID == nil || after.Nodes[1].LatestAttemptID == nil || *after.Nodes[1].LatestAttemptID != siblingID {
		t.Fatal("verified release did not wake only the registered member", after, err)
	}
	first := *after.Nodes[0].NodeAdmissionID
	e.continueGraphAdmissions(sibling.ID)
	again, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || again.Nodes[0].NodeAdmissionID == nil || *again.Nodes[0].NodeAdmissionID != first {
		t.Fatal("replayed release duplicated member admission", again, err)
	}
}
