package engine

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// The exact fixture has seven declared transitions. A repeated observation can
// never renew a transition's allowance; the immutable total is an extra ceiling.
type graphActorStageClock struct {
	started, last time.Time
	next          int
	identities    [7]string
}

func (c graphActorStageClock) deadline() time.Time {
	deadline := c.started.Add(56 * time.Second)
	if c.next < len(c.identities) && c.last.Add(8*time.Second).Before(deadline) {
		deadline = c.last.Add(8 * time.Second)
	}
	return deadline
}

func (c graphActorStageClock) check(now time.Time) error {
	if now.After(c.deadline()) {
		return fmt.Errorf("declared graph transition %d exceeded its fixed observation bound", c.next)
	}
	return nil
}

func (c *graphActorStageClock) observe(index int, identity string, now time.Time) error {
	if index < 0 || index >= len(c.identities) || identity == "" || index > c.next {
		return fmt.Errorf("unexpected graph fixture transition %d", index)
	}
	if err := c.check(now); err != nil {
		return err
	}
	if index < c.next {
		if c.identities[index] != identity {
			return fmt.Errorf("replacement graph fixture transition %d", index)
		}
		return nil
	}
	c.identities[index], c.last = identity, now
	c.next++
	return nil
}

type graphActorStageObservation struct {
	view                          domain.GraphDetail
	gate                          domain.ExecutionGate
	ready                         bool
	identity, key, role, workflow string
	producer                      string
	err                           error
}

type graphActorWatchdog struct {
	t         *testing.T
	e         *Engine
	graphID   string
	clock     graphActorStageClock
	last      graphActorStageObservation
	nodes     map[string]string
	workflows map[string]string
	authors   map[string]string
}

func newGraphActorWatchdog(t *testing.T, e *Engine, graphID string) *graphActorWatchdog {
	now := time.Now()
	return &graphActorWatchdog{t: t, e: e, graphID: graphID, clock: graphActorStageClock{started: now, last: now}, nodes: map[string]string{}, workflows: map[string]string{}, authors: map[string]string{}}
}

// Observer reads cannot prevent the watchdog from reporting a stuck Store. The
// failure path uses only the last snapshot and goroutine stacks, never more SQL.
func (w *graphActorWatchdog) observe() (domain.GraphDetail, domain.ExecutionGate) {
	w.t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), w.clock.deadline())
	defer cancel()
	index := w.clock.next
	result := make(chan graphActorStageObservation, 1)
	go func() { result <- graphActorReadStage(ctx, w.e, w.graphID, index) }()
	var observed graphActorStageObservation
	select {
	case <-ctx.Done():
		w.fail(ctx.Err())
	case observed = <-result:
	}
	if observed.err != nil {
		w.fail(observed.err)
	}
	w.last = observed
	for _, node := range observed.view.Nodes {
		identity := node.ID + ":" + node.TaskID + ":" + node.TaskVersion
		if node.Key != "a" && node.Key != "b" || w.nodes[node.Key] != "" && w.nodes[node.Key] != identity {
			w.fail(fmt.Errorf("the declared node identity changed"))
		}
		w.nodes[node.Key] = identity
	}
	if err := w.clock.check(time.Now()); err != nil {
		w.fail(err)
	}
	if observed.ready {
		if observed.workflow != "" {
			if prior := w.workflows[observed.key]; prior != "" && prior != observed.workflow {
				w.fail(fmt.Errorf("the declared workflow was replaced"))
			}
			w.workflows[observed.key] = observed.workflow
			if observed.role == "author" {
				w.authors[observed.key] = observed.producer
			} else if w.authors[observed.key] == "" || w.authors[observed.key] != observed.producer {
				w.fail(fmt.Errorf("the accepted producer differs from the observed author"))
			}
		}
		previous := w.clock.last
		if err := w.clock.observe(index, observed.identity, time.Now()); err != nil {
			w.fail(err)
		}
		w.t.Logf("Declared graph milestone %d/7: elapsed=%s gap=%s identity=%s", w.clock.next, time.Since(w.clock.started), time.Since(previous), observed.identity)
	}
	if observed.view.Graph.State == "completed" && w.clock.next != 7 {
		if !observed.ready {
			w.fail(fmt.Errorf("the completed group lacks its next declared stage proof"))
		}
		// A snapshot can already contain later completed transitions. Observe
		// them in order without allowing an early group flag to skip proofs.
		return w.observe()
	}
	return observed.view, observed.gate
}

func (w *graphActorWatchdog) fail(err error) {
	stack := make([]byte, 128<<10)
	n := runtime.Stack(stack, true)
	w.t.Fatalf("Graph fixture progress failed: %v; next=%d elapsed=%s lastGraph=%+v lastNodes=%+v lastGate=%+v\n%s", err, w.clock.next, time.Since(w.clock.started), w.last.view.Graph, w.last.view.Nodes, w.last.gate, string(stack[:n]))
}

func graphActorReadStage(ctx context.Context, e *Engine, graphID string, index int) (out graphActorStageObservation) {
	out.view, out.err = e.db.Graph(ctx, graphID)
	if out.err != nil {
		return out
	}
	out.gate, out.err = e.db.Gate(ctx)
	if out.err != nil || index >= 7 {
		return out
	}
	nodes := map[string]domain.GraphNodeSummary{}
	for _, node := range out.view.Nodes {
		nodes[node.Key] = node
	}
	if index == 0 || index == 1 || index == 4 {
		id := out.view.Graph.RootInputID
		if index == 1 {
			id = nodes["a"].InputID
		} else if index == 4 {
			id = nodes["b"].InputID
		}
		if id == nil {
			return out
		}
		input, err := e.db.GraphInputRecord(ctx, graphID, *id)
		if err != nil {
			out.err = err
			return out
		}
		if input.Public.Summary.State != "ready" || !input.Public.Summary.Current {
			return out
		}
		operation := ""
		if index == 0 && input.PrivateRoot != nil {
			operation = input.PrivateRoot.OperationAttemptID
		} else if index != 0 && input.Public.InputBinding != nil {
			operation = input.Public.InputBinding.OperationAttemptID
		}
		if operation == "" {
			out.err = fmt.Errorf("ready fixture input lacks its exact operation")
			return out
		}
		a, err := e.db.Attempt(ctx, operation)
		if err != nil || a.Phase != "settled" || a.Outcome == nil || *a.Outcome != "succeeded" {
			out.err = fmt.Errorf("ready fixture input lacks successful real settlement: %v", err)
			return out
		}
		out.ready, out.identity = true, *id+":"+operation
		return out
	}
	out.key, out.role = "a", "author"
	if index >= 5 {
		out.key = "b"
	}
	if index == 3 || index == 6 {
		out.role = "review"
		if nodes[out.key].AcceptedResult == nil || index == 6 && out.view.Graph.State != "completed" {
			return out
		}
	}
	node := nodes[out.key]
	if node.NodeAdmissionID == nil || node.LatestAttemptID == nil {
		return out // Include the not-yet-admitted native stage in this same bound.
	}
	steps, err := e.db.WorkflowSteps(ctx, node.TaskID, domain.WorkflowSelector{}, 20, "")
	if err != nil {
		out.err = err
		return out
	}
	var selected *domain.Attempt
	for _, step := range steps.Items {
		if step.Role != out.role {
			continue
		}
		if selected != nil || step.AttemptID == nil || step.ExecutorKind != "native" {
			out.err = fmt.Errorf("extra or substituted fixture native stage")
			return out
		}
		a, err := e.db.Attempt(ctx, *step.AttemptID)
		if err != nil {
			out.err = err
			return out
		}
		selected = &a
	}
	if selected == nil || selected.Phase != "settled" {
		return out
	}
	a := *selected
	if a.TaskID != node.TaskID || a.TaskVersion != node.TaskVersion || a.Outcome == nil || *a.Outcome != "succeeded" || a.StartedAt == nil || a.Cleanup.State != "verified" || a.EvidenceStatus != "complete" {
		out.err = fmt.Errorf("fixture native stage did not actually succeed")
		return out
	}
	out.workflow, out.producer = a.WorkflowID, a.ID
	if out.role == "review" {
		out.producer = node.AcceptedResult.ProducerAttemptID
	}
	out.ready, out.identity = true, a.WorkflowID+":"+a.StepID+":"+a.ID
	return out
}

func TestGraphActorStageClockRejectsUnboundedProgress(t *testing.T) {
	start := time.Unix(0, 0)
	fresh := func() graphActorStageClock { return graphActorStageClock{started: start, last: start} }
	t.Run("missing-initial-admission", func(t *testing.T) {
		c := fresh()
		if c.check(start.Add(8*time.Second+1)) == nil {
			t.Fatal("missing admission escaped its transition bound")
		}
	})
	t.Run("late-completion-cannot-renew", func(t *testing.T) {
		c := fresh()
		if c.observe(0, "root", start.Add(8*time.Second+1)) == nil || c.next != 0 {
			t.Fatal("late completion renewed observation authority")
		}
	})
	t.Run("poll-and-duplicate-noise-never-renew", func(t *testing.T) {
		c := fresh()
		if err := c.observe(0, "root", start.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		for second := 2; second <= 8; second++ {
			now := start.Add(time.Duration(second) * time.Second)
			if c.check(now) != nil || c.observe(0, "root", now) != nil || !c.last.Equal(start.Add(time.Second)) {
				t.Fatal("repeated observations changed the next admission deadline")
			}
		}
		if c.check(start.Add(9*time.Second+1)) == nil {
			t.Fatal("the missing next admission escaped its fixed bound")
		}
	})
	t.Run("immutable-total-and-no-eighth-stage", func(t *testing.T) {
		c := fresh()
		for index := 0; index < 7; index++ {
			if err := c.observe(index, fmt.Sprint(index), start.Add(time.Duration(index+1)*7*time.Second)); err != nil {
				t.Fatal(err)
			}
		}
		if c.next != 7 || c.observe(7, "extra", start.Add(50*time.Second)) == nil || c.check(start.Add(56*time.Second+1)) == nil {
			t.Fatal("completion escaped the immutable total or admitted an eighth stage")
		}
	})
	t.Run("replacement-and-skipped-stage", func(t *testing.T) {
		c := fresh()
		if c.observe(1, "input", start.Add(time.Second)) == nil {
			t.Fatal("a missing declared predecessor was skipped")
		}
		if err := c.observe(0, "root", start.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if c.observe(0, "replacement", start.Add(2*time.Second)) == nil {
			t.Fatal("a replacement occurrence renewed a completed milestone")
		}
	})
}
