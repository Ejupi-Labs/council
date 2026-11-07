package engine

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These are three complete jobs, not the author's intermediate phases. Neither
// polling nor admission/start/cleanup/output observations renew the author job.
type graphAncestorJobClock struct {
	started, last time.Time
	next          int
	identities    [3]string
}

func (c graphAncestorJobClock) deadline() time.Time {
	deadline := c.started.Add(24 * time.Second)
	if c.next < len(c.identities) && c.last.Add(8*time.Second).Before(deadline) {
		deadline = c.last.Add(8 * time.Second)
	}
	return deadline
}

func (c graphAncestorJobClock) check(now time.Time) error {
	if now.After(c.deadline()) {
		return fmt.Errorf("ancestor prerequisite job %d exceeded its fixed observation bound", c.next)
	}
	return nil
}

func (c *graphAncestorJobClock) observe(index int, identity string, now time.Time) error {
	if index < 0 || index >= len(c.identities) || index > c.next || identity == "" {
		return fmt.Errorf("unexpected ancestor prerequisite job %d", index)
	}
	if err := c.check(now); err != nil {
		return err
	}
	if index < c.next {
		if c.identities[index] != identity {
			return fmt.Errorf("replacement ancestor prerequisite job %d", index)
		}
		return nil
	}
	c.identities[index], c.last = identity, now
	c.next++
	return nil
}

type graphAncestorJobObservation struct {
	view     domain.GraphDetail
	attempt  *domain.Attempt
	root     *domain.GraphRootBinding
	input    *domain.GraphInputBinding
	dispatch *domain.GraphDispatch
	ready    bool
	identity string
	err      error
}

// All probes are metadata reads. Actual physical verification remains in the
// real jobs whose complete, current, successful records are required here.
func graphAncestorReadJob(ctx context.Context, e *Engine, graphID string, index int) (out graphAncestorJobObservation) {
	out.view, out.err = e.db.Graph(ctx, graphID)
	if out.err != nil {
		return out
	}
	v := out.view
	if v.Graph.ID != graphID || v.Graph.GraphVersion != "1" || v.Graph.ControlRevision != "1" || v.Graph.BudgetAuthorityEpoch != "1" || v.Graph.Stop != nil || len(v.Nodes) != 3 || v.Budget.Budget.GrantedAdmissions != "4" || v.Budget.Budget.CurrentWindow.Number != "1" {
		out.err = fmt.Errorf("the declared ancestor graph or authority changed")
		return out
	}
	for i, key := range []string{"a", "c", "d"} {
		n := v.Nodes[i]
		if n.Key != key || n.GraphID != graphID || n.TaskVersion != "1" || n.ControlRevision != "1" || n.Stop != nil || n.AcceptedResult != nil || i > 0 && (n.NodeAdmissionID != nil || n.LatestAttemptID != nil) {
			out.err = fmt.Errorf("the declared ancestor/follower scope changed")
			return out
		}
	}
	a := v.Nodes[0]
	if index < 2 {
		id := v.Graph.RootInputID
		kind, role := "root_basis", "graph_basis"
		if index == 1 {
			id, kind, role = a.InputID, "node_input", "graph_input"
		}
		if id == nil {
			return out
		}
		record, err := e.db.GraphInputRecord(ctx, graphID, *id)
		out.err = err
		s := record.Public.Summary
		if err != nil || s.State != "ready" || !s.Current {
			return out
		}
		if s.ID != *id || s.Kind != kind || s.GraphID != graphID || s.GraphVersion != v.Graph.GraphVersion || s.RootTaskID != v.Graph.RootTaskID || record.CompletionFingerprint == "" || record.PrivateRoot == nil {
			out.err = fmt.Errorf("a ready prerequisite lacks its exact completed local job")
			return out
		}
		root := record.PrivateRoot
		if root.GraphID != graphID || root.RootTaskID != v.Graph.RootTaskID || root.ProjectID != v.Graph.ProjectID || domain.ValidateGraphRootBinding(*root) != nil {
			out.err = fmt.Errorf("a prerequisite root binding differs from the declared graph")
			return out
		}
		out.root = root
		if index == 0 {
			if root.OperationAttemptID != s.OperationAttemptID {
				out.err = fmt.Errorf("the root operation was replaced")
				return out
			}
		} else {
			b := record.Public.InputBinding
			if b == nil || b.ID != *id || b.GraphID != graphID || b.GraphVersion != v.Graph.GraphVersion || b.OperationAttemptID != s.OperationAttemptID || b.ReceivingNodeID != a.ID || b.ReceivingTaskID != a.TaskID || b.ReceivingTaskVersion != a.TaskVersion || b.RootBasisSHA256 != root.BasisSHA256 || len(b.Contributions) != 0 || len(b.ContributorVector) != 0 || domain.ValidateGraphInputBinding(*b) != nil {
				out.err = fmt.Errorf("the ready ancestor input lacks its exact receiver/root binding")
				return out
			}
			out.input = b
		}
		actual, err := e.db.Attempt(ctx, s.OperationAttemptID)
		out.err, out.attempt = err, &actual
		if err != nil {
			return out
		}
		if actual.TaskID != v.Graph.RootTaskID || actual.OwnerInstanceID != e.config.OwnerInstanceID || actual.ExecutorKind != "local_graph" || actual.Role != role || actual.Phase != "settled" || actual.Outcome == nil || *actual.Outcome != "succeeded" || actual.Cleanup.State != "verified" || actual.EvidenceStatus != "complete" {
			out.err = fmt.Errorf("a ready prerequisite lacks its actual successful local settlement")
			return out
		}
		out.ready, out.identity = true, *id+":"+actual.ID
		return out
	}
	if a.LatestAttemptID == nil || a.NodeAdmissionID == nil || a.InputID == nil {
		return out // Admission remains inside this same author-job deadline.
	}
	actual, err := e.db.Attempt(ctx, *a.LatestAttemptID)
	out.err, out.attempt = err, &actual
	if err != nil {
		return out
	}
	if actual.TaskID != a.TaskID || actual.TaskVersion != a.TaskVersion || actual.OwnerInstanceID != e.config.OwnerInstanceID || actual.ExecutorKind != "native" || actual.Role != "author" || actual.ExecutionMode != "fake" || actual.FakeScenario != "graph-response" || actual.Stop != nil || actual.PrimaryFailure != nil {
		out.err = fmt.Errorf("the exact healthy ancestor author changed")
		return out
	}
	if actual.Phase != "settled" {
		return out
	}
	if actual.Outcome == nil || *actual.Outcome != "succeeded" || actual.StartedAt == nil || !actual.LaunchIntent || actual.SourceSnapshotID == nil || actual.Cleanup.State != "verified" || actual.EvidenceStatus != "complete" || actual.CandidateID == nil || actual.ResponseArtifactID == nil {
		out.err = fmt.Errorf("the ancestor author lacks its complete successful native settlement")
		return out
	}
	dispatch, err := e.db.GraphDispatchForAttempt(ctx, actual.ID)
	out.err = err
	if err != nil {
		return out
	}
	d, o := dispatch.Public.Dispatch, dispatch.Public.OutputObservation
	out.dispatch = &d
	if d.ID != *a.NodeAdmissionID || d.NodeAdmissionID != d.ID || d.GraphID != graphID || d.GraphVersion != v.Graph.GraphVersion || d.NodeID != a.ID || d.TaskID != a.TaskID || d.TaskVersion != a.TaskVersion || d.AttemptID != actual.ID || d.WorkflowID != actual.WorkflowID || d.StepID != actual.StepID || d.InputID != *a.InputID || d.OwnerInstanceID != e.config.OwnerInstanceID || d.ExecutorKind != "native" || d.Role != "author" {
		out.err = fmt.Errorf("the successful author lacks its exact registered dispatch")
		return out
	}
	if o == nil || o.ProducerAttemptID != actual.ID || o.Candidate == nil || o.Candidate.CandidateID != *actual.CandidateID || o.Candidate.State != "complete" || o.Candidate.ClosureRef == nil || o.Candidate.ClosureRef.ProducerAttemptID != actual.ID || o.Response == nil || o.Response.ResponseArtifactID != *actual.ResponseArtifactID || o.Response.State != "complete" || o.Response.ClosureRef == nil || o.Response.ClosureRef.ProducerAttemptID != actual.ID {
		out.err = fmt.Errorf("the successful author lacks both exact complete output closures")
		return out
	}
	w, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: actual.WorkflowID})
	out.err = err
	if err != nil || w.Workflow == nil {
		return out
	}
	wf := w.Workflow
	if wf.ID != actual.WorkflowID || wf.TaskID != a.TaskID || wf.TaskVersion != a.TaskVersion || wf.CurrentStepID == nil || *wf.CurrentStepID != actual.StepID || wf.SelectedResponseBinding == nil || wf.SelectedResponseBinding.ProducerAttemptID != actual.ID || wf.SelectedResponseBinding.ResponseArtifactID != *actual.ResponseArtifactID || wf.NextAction != "response_acceptance_pending" || wf.Completion != nil {
		out.err = fmt.Errorf("the selected ancestor workflow is not awaiting its exact response acknowledgement")
		return out
	}
	// Graph was read before the terminal attempt. Wait for one coherent graph
	// observation instead of accepting its possibly older live claim snapshot.
	out.ready = len(v.ActiveClaims) == 0 && v.Budget.Budget.ConsumedAdmissions == "1"
	out.identity = actual.ID + ":" + actual.WorkflowID + ":" + actual.StepID
	return out
}

func graphAncestorWaitPrerequisites(t *testing.T, f graphParallelFixture) (domain.GraphNodeSummary, domain.Attempt) {
	t.Helper()
	started := time.Now()
	clock := graphAncestorJobClock{started: started, last: started}
	var last graphAncestorJobObservation
	nodes, rootID, inputID, authorID := "", "", "", ""
	var root *domain.GraphRootBinding
	var input *domain.GraphInputBinding
	fail := func(err error) {
		stack := make([]byte, 128<<10)
		n := runtime.Stack(stack, true)
		// No additional SQL on a failed observer; a blocked read must not hide
		// the fixed clock behind another synchronous diagnostic query.
		t.Fatalf("Ancestor prerequisite observer failed: %v; job=%d elapsed=%s lastGraph=%+v lastNodes=%+v lastAttempt=%+v\n%s", err, clock.next, time.Since(started), last.view.Graph, last.view.Nodes, last.attempt, stack[:n])
	}
	for clock.next < len(clock.identities) {
		if err := clock.check(time.Now()); err != nil {
			fail(err)
		}
		index := clock.next
		ctx, cancel := context.WithDeadline(context.Background(), clock.deadline())
		result := make(chan graphAncestorJobObservation, 1)
		go func() { result <- graphAncestorReadJob(ctx, f.e, f.graphID, index) }()
		var observed graphAncestorJobObservation
		select {
		case <-ctx.Done():
			cancel()
			fail(ctx.Err())
		case observed = <-result:
			cancel()
		}
		last = observed
		if observed.err != nil {
			fail(observed.err)
		}
		if err := clock.check(time.Now()); err != nil {
			fail(err)
		}
		identity := observed.view.Graph.ID + ":" + observed.view.Graph.RootTaskID + ":" + observed.view.Graph.ProjectID + ":" + observed.view.Definition.DeclarationSHA256 + ";"
		for _, node := range observed.view.Nodes {
			identity += node.ID + ":" + node.TaskID + ":" + node.TaskVersion + ";"
		}
		if nodes != "" && nodes != identity {
			fail(fmt.Errorf("a declared ancestor node was replaced"))
		}
		nodes = identity
		for _, pair := range []struct {
			current *string
			saved   *string
		}{{observed.view.Graph.RootInputID, &rootID}, {observed.view.Nodes[0].InputID, &inputID}, {observed.view.Nodes[0].LatestAttemptID, &authorID}} {
			if *pair.saved != "" && (pair.current == nil || *pair.current != *pair.saved) {
				fail(fmt.Errorf("a prerequisite input or author was replaced"))
			}
			if pair.current != nil {
				*pair.saved = *pair.current
			}
		}
		if !observed.ready {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if index == 0 {
			root = observed.root
		} else if index == 1 {
			if root == nil || observed.root == nil || observed.root.BasisSHA256 != root.BasisSHA256 || observed.root.OperationAttemptID != root.OperationAttemptID {
				fail(fmt.Errorf("the ancestor input replaced the completed root basis"))
			}
			input = observed.input
		} else {
			d := observed.dispatch
			if input == nil || input.ID != inputID || d == nil || d.InputID != input.ID || d.InputSHA256 != input.InputSHA256 || d.SourceBasis.BasisKind != "graph_input" || d.SourceBasis.InputID != input.ID || d.SourceBasis.InputSHA256 != input.InputSHA256 || d.SourceBasis.ExpectedManifest != input.Manifest || d.SourceBasis.ExpectedIndex != input.Index {
				fail(fmt.Errorf("the terminal author replaced the completed receiving input"))
			}
		}
		previous := clock.last
		if err := clock.observe(index, observed.identity, time.Now()); err != nil {
			fail(err)
		}
		t.Logf("Ancestor prerequisite job %d/3 completed: elapsed=%s gap=%s identity=%s", clock.next, time.Since(started), time.Since(previous), observed.identity)
	}
	return last.view.Nodes[0], *last.attempt
}

func TestGraphAncestorJobClockRejectsUnboundedProgress(t *testing.T) {
	start := time.Unix(0, 0)
	fresh := func() graphAncestorJobClock { return graphAncestorJobClock{started: start, last: start} }
	t.Run("missing-or-late-job", func(t *testing.T) {
		c := fresh()
		late := start.Add(8*time.Second + time.Nanosecond)
		if c.check(late) == nil || c.observe(0, "root", late) == nil || c.next != 0 || !c.last.Equal(start) {
			t.Fatal("late root completion renewed the prerequisite bound")
		}
	})
	t.Run("duplicates-and-phase-noise-never-renew", func(t *testing.T) {
		c := fresh()
		if err := c.observe(0, "root", start.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		deadline := c.deadline()
		for second := 2; second <= 8; second++ {
			now := start.Add(time.Duration(second) * time.Second)
			if c.check(now) != nil || c.observe(0, "root", now) != nil || !c.deadline().Equal(deadline) {
				t.Fatal("poll or duplicate job renewed the missing input deadline")
			}
		}
		if c.observe(1, "input", deadline.Add(time.Nanosecond)) == nil || c.next != 1 {
			t.Fatal("late input completion escaped its original bound")
		}
	})
	t.Run("author-intermediate-observations-never-renew", func(t *testing.T) {
		c := fresh()
		if c.observe(0, "root", start.Add(time.Second)) != nil || c.observe(1, "input", start.Add(2*time.Second)) != nil {
			t.Fatal("valid completed local jobs were refused")
		}
		deadline := c.deadline()
		for _, phase := range []string{"admitted", "started", "cleanup", "outputs-retained"} {
			if c.check(deadline.Add(-time.Second)) != nil || !c.deadline().Equal(deadline) || c.next != 2 {
				t.Fatal("partial author progress renewed the complete author deadline", phase)
			}
		}
		if c.observe(2, "author", deadline.Add(time.Nanosecond)) == nil || c.next != 2 {
			t.Fatal("late author settlement renewed its job allowance")
		}
	})
	t.Run("no-skipped-replacement-or-extra-job", func(t *testing.T) {
		c := fresh()
		if c.observe(1, "input", start.Add(time.Second)) == nil || c.observe(0, "", start.Add(time.Second)) == nil || c.next != 0 {
			t.Fatal("a missing/empty predecessor job was skipped")
		}
		if c.observe(0, "root", start.Add(time.Second)) != nil || c.observe(0, "other-root", start.Add(2*time.Second)) == nil || c.next != 1 {
			t.Fatal("a completed root job was replaced")
		}
		if c.observe(2, "author", start.Add(2*time.Second)) == nil || c.next != 1 {
			t.Fatal("the declared input job was skipped")
		}
	})
	t.Run("immutable-total-after-completion", func(t *testing.T) {
		c := fresh()
		for index := 0; index < 3; index++ {
			if err := c.observe(index, fmt.Sprint(index), start.Add(time.Duration(index+1)*8*time.Second)); err != nil {
				t.Fatal(err)
			}
		}
		if c.next != 3 || !c.deadline().Equal(start.Add(24*time.Second)) || c.observe(3, "extra", start.Add(24*time.Second)) == nil || c.check(start.Add(24*time.Second+time.Nanosecond)) == nil {
			t.Fatal("completed prerequisites escaped the immutable total or admitted a fourth job")
		}
	})
}
