package engine

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func TestGraphDeliveryStopBetweenActualActionsRetainsOnlyAcknowledgedPrefix(t *testing.T) {
	f := graphDeliveryAttemptFixture(t)
	e, ctx := f.final.engine, context.Background()
	requestID := domain.NewID()
	actions := []workspace.ApplyAction{}
	var applied workspace.ApplyAction
	var observed workspace.ApplyResult
	var callbackErr error
	stopAccepted := false
	e.config.BeforeApplicationAction = func(_ context.Context, a domain.Attempt, action workspace.ApplyAction) {
		if a.ID != f.delivery.ID {
			callbackErr = fmt.Errorf("a different attempt received a graph action permit")
		}
		actions = append(actions, action)
	}
	e.config.AfterApplicationAction = func(ctx context.Context, a domain.Attempt, action workspace.ApplyAction, result workspace.ApplyResult) {
		if applied.ID != "" {
			return
		}
		applied, observed = action, result
		if a.ID != f.delivery.ID || len(actions) != 1 || action.Kind != "add" || action.Action != "publish_after" || !result.Known || result.State != "applied" || result.ObservedEvidence == nil {
			callbackErr = fmt.Errorf("the first boundary did not acknowledge an actual graph publication: action=%+v result=%+v", action, result)
			return
		}
		journal, err := e.db.WorkflowDeliveries(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID}, 10, "")
		if err != nil || len(journal.Items) != 1 || journal.Items[0].Operation == nil || journal.Items[0].Operation.LastEffectSequence != "2" {
			callbackErr = fmt.Errorf("the first actual action lacks its durable intent/result prefix: journal=%+v error=%v", journal, err)
			return
		}
		view, err := e.db.Graph(ctx, f.dispatch.GraphID)
		if err != nil {
			callbackErr = err
			return
		}
		body, err := json.Marshal(domain.GraphStopInput{RequestID: requestID, ExpectedGraphControlRevision: view.Graph.ControlRevision})
		if err != nil {
			callbackErr = err
			return
		}
		stopped := e.MutateGraph(ctx, "graphs.stop", view.Graph.ID, body)
		if stopped.Status != 202 || stopped.Envelope.Error != nil {
			callbackErr = fmt.Errorf("the real group Stop was not accepted after the first effect: %+v", stopped)
			return
		}
		stopAccepted = true
	}
	e.run(ctx, f.delivery)
	if callbackErr != nil || !stopAccepted || len(actions) != 1 || applied.ID == "" || !observed.Known || observed.State != "applied" {
		t.Fatal("The actual first-action/accepted-Stop boundary was not established", callbackErr, stopAccepted, actions, observed)
	}
	f.assertUserBasis()
	actual, err := e.db.Attempt(ctx, f.delivery.ID)
	if err != nil || actual.Phase != "settled" || actual.Outcome == nil || *actual.Outcome != "cancelled" || actual.Stop == nil || actual.Stop.RequestID != requestID ||
		actual.PrimaryFailure != nil || actual.EvidenceStatus == "uncertain" || actual.Cleanup.State != "verified" || actual.LaunchIntent {
		t.Fatal("Accepted Stop did not retain an honest known partial local result", actual, err, graphDeliveryFailureDiagnostics(e, actual))
	}
	op := workflowControlOperation(t, e, f.delivery.TaskID)
	if op.Status != "partial" || op.NextAction != "inspect_remaining_paths" || op.LastEffectSequence != "2" || op.BeforeDestination == nil || op.AfterDestination != nil || op.IncidentID != nil ||
		op.CandidateBinding != f.binding || op.GraphDeliveryRef == nil || op.GraphDeliveryRef.ProducerAttemptID != actual.ID {
		t.Fatal("Stop lost its immutable graph binding or admitted a second destination action", op)
	}
	for name, expected := range map[string]string{fixture.GraphFileA: fixture.GraphContentA, fixture.GraphFileB: fixture.GraphContentB} {
		body, readErr := os.ReadFile(filepath.Join(f.final.repo, name))
		if name == applied.RelativePath {
			if readErr != nil || string(body) != expected {
				t.Fatal("The acknowledged first publication was hidden or rolled back", name, readErr)
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatal("A second destination path was published after accepted Stop", name, readErr)
		}
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || len(gate.ActiveClaims) != 0 || gate.IncidentID != nil {
		t.Fatal("Known Stop falsely retained an uncertain application claim", gate, err)
	}
	view, err := e.db.Graph(ctx, f.dispatch.GraphID)
	if err != nil || view.Graph.Stop == nil || view.Graph.Stop.RequestID != requestID || !reflect.DeepEqual(view.Budget.Budget, f.admitted.Budget.Budget) {
		t.Fatal("The effect boundary changed model allowance or lost the accepted group Stop", view, err)
	}
	for _, node := range view.Nodes {
		if node.ID == f.dispatch.NodeID && node.AcceptedResult != nil {
			t.Fatal("A partial graph delivery was accepted as a complete final", node)
		}
	}
}

func TestGraphDeliveryCorruptRetainedMemberBeforeReadyHasNoDestinationPermit(t *testing.T) {
	f := graphDeliveryAttemptFixture(t)
	e, ctx := f.final.engine, context.Background()
	var memberID string
	for _, entry := range f.final.verified.RootProof.Manifest.Entries {
		if entry.Path == "source.txt" && entry.Type == "file" {
			memberID = entry.ArtifactID
		}
	}
	if memberID == "" || memberID == f.final.verified.Root.Manifest.ArtifactID || memberID == f.final.verified.Root.Index.ArtifactID || memberID == f.final.verified.Root.StagedBlobs.ArtifactID {
		t.Fatal("The actual original graph basis has no unchanged non-anchor working member")
	}
	member, err := e.evidence.VerifyClosed(ctx, memberID)
	if err != nil || member.Kind != "source_file" || member.SHA256 == nil {
		t.Fatal("The controlled member is not initially complete physical source evidence", member, err)
	}
	corrupted, bindingObserved, permits := false, false, 0
	var callbackErr error
	e.config.BeforeApplicationReady = func(ctx context.Context, a domain.Attempt) error {
		journal, err := e.db.WorkflowDeliveries(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID}, 10, "")
		if err != nil || a.ID != f.delivery.ID || len(journal.Items) != 1 || journal.Items[0].Operation == nil {
			callbackErr = fmt.Errorf("the real post-plan/pre-Ready journal is unavailable: journal=%+v error=%v", journal, err)
			return callbackErr
		}
		op := journal.Items[0].Operation
		if op.Status != "preparing" || op.LastEffectSequence != "0" || op.GraphDeliveryRef == nil || op.GraphDeliveryRef.ProducerAttemptID != a.ID || op.CandidateBinding != f.binding {
			callbackErr = fmt.Errorf("the corruption boundary has no exact durable graph binding: %+v", op)
			return callbackErr
		}
		if _, err = e.readGraphArtifact(ctx, *op.GraphDeliveryRef, "graph_delivery", 64<<20); err != nil {
			callbackErr = err
			return err
		}
		bindingObserved = true
		callbackErr = graphDeliveryCorruptExistingMember(e.lock.Root(), member)
		if callbackErr != nil {
			return callbackErr
		}
		corrupted = true
		// Return success: only the product's fresh full-member verification may
		// stop Ready. A hook error must not manufacture the refusal under test.
		return nil
	}
	e.config.BeforeApplicationAction = func(context.Context, domain.Attempt, workspace.ApplyAction) { permits++ }
	e.run(ctx, f.delivery)
	if callbackErr != nil || !bindingObserved || !corrupted {
		t.Fatal("The actual post-binding/pre-Ready corruption was not established", bindingObserved, corrupted, callbackErr)
	}
	if _, err = e.evidence.VerifyClosed(ctx, memberID); !errors.Is(err, evidence.ErrCorrupt) {
		t.Fatal("The controlled unchanged member was not actually corrupted", err)
	}
	if _, err = e.evidence.VerifyClosed(ctx, f.final.verified.Root.Manifest.ArtifactID); err != nil {
		t.Fatal("The controlled fault changed a manifest anchor instead of its member", err)
	}
	f.assertUserBasis()
	for _, name := range []string{fixture.GraphFileA, fixture.GraphFileB} {
		if _, err := os.Lstat(filepath.Join(f.final.repo, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("Corrupt cumulative evidence obtained a destination publication", name, err)
		}
	}
	actual, err := e.db.Attempt(ctx, f.delivery.ID)
	if err != nil || permits != 0 || actual.Phase != "settled" || actual.Outcome == nil || *actual.Outcome == "succeeded" || actual.EvidenceStatus != "uncertain" || actual.LaunchIntent || actual.Cleanup.State != "verified" {
		t.Fatal("Corrupt retained evidence was qualified or its zero-action uncertainty was hidden", actual, permits, err, graphDeliveryFailureDiagnostics(e, actual))
	}
	op := workflowControlOperation(t, e, f.delivery.TaskID)
	if op.Status != "uncertain" || op.LastEffectSequence != "0" || op.PlanArtifactID != "" || op.BeforeDestination != nil || op.AfterDestination != nil ||
		op.CandidateBinding != f.binding || op.GraphDeliveryRef == nil || op.GraphDeliveryRef.ProducerAttemptID != actual.ID {
		t.Fatal("Corruption admitted Ready/effects or discarded the exact acknowledged binding", op)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State == "open" || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != actual.ID || gate.IncidentID == nil {
		t.Fatal("Uncertain cumulative evidence released its exact owned application claim", gate, err)
	}
	budget, err := e.db.GraphBudget(ctx, f.dispatch.GraphID)
	if err != nil || !reflect.DeepEqual(budget.Budget, f.admitted.Budget.Budget) {
		t.Fatal("A rejected graph Ready changed model allowance", budget, err)
	}
}

// Change actual retained bytes in place, retaining identity, length and every
// registered header. No mutable worker or user-checkout file is modified.
func graphDeliveryCorruptExistingMember(root string, member evidence.Metadata) error {
	if member.Kind != "source_file" || !filepath.IsLocal(filepath.FromSlash(member.RelativePath)) {
		return fmt.Errorf("the controlled member is not an owned source_file")
	}
	path := filepath.Join(root, filepath.FromSlash(member.RelativePath))
	before, err := os.Stat(path)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
		return errors.Join(err, fmt.Errorf("the controlled member has no complete bytes"))
	}
	body[0] ^= 0x20
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	n, writeErr := file.WriteAt(body, 0)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil || n != len(body) {
		return errors.Join(err, fmt.Errorf("the controlled in-place write was incomplete"))
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() {
		return errors.Join(err, fmt.Errorf("the controlled corruption changed identity or length"))
	}
	return nil
}
