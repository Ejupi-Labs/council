package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

// Only actual fixed graph-wait author preparations are recorded. The wrapper
// delegates unchanged to the real process ownership implementation.
type graphParallelPreparations struct {
	mu       sync.Mutex
	cwds     map[string]int
	released map[string]bool
	prior    func(context.Context, process.NativeSpec) (*process.Owned, error)
	hook     func(context.Context, process.NativeSpec, *process.Owned, error) (*process.Owned, error)
}

func (p *graphParallelPreparations) prepare(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
	if slices.Contains(spec.Args, "graph-wait") {
		p.mu.Lock()
		p.cwds[filepath.Clean(spec.Cwd)]++
		p.mu.Unlock()
	}
	owned, err := p.prior(ctx, spec)
	p.mu.Lock()
	hook := p.hook
	p.mu.Unlock()
	if hook != nil {
		return hook(ctx, spec, owned, err)
	}
	return owned, err
}

func (p *graphParallelPreparations) snapshot() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := map[string]int{}
	for cwd, count := range p.cwds {
		result[cwd] = count
	}
	return result
}

type graphParallelWorker struct {
	node    domain.GraphNodeSummary
	attempt domain.Attempt
	cwd     string
}

type graphParallelFixture struct {
	e                    *Engine
	graphID, repo        string
	working, stagedIndex []byte
	preparations         *graphParallelPreparations
	accountID            string
}

func graphParallelStart(t *testing.T, maxParallelism string, sharedAccount bool, options ...func(*domain.GraphStartInput)) graphParallelFixture {
	t.Helper()
	e, projectID, repo := testEngine(t) // Keep the actual actor running.
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte("\ufeffParallel fixture user's staged bytes 東京\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	interopGit(t, repo, "add", "source.txt")
	working := []byte("Parallel fixture user's distinct working bytes 🙂\r\n")
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), working, 0600); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	accountID := ""
	if sharedAccount {
		body, err := json.Marshal(domain.AccountCreateInput{RequestID: domain.NewID(), Label: "Controlled shared account declaration"})
		if err != nil {
			t.Fatal(err)
		}
		created := e.MutateRouting(ctx, "routing.accounts.create", "", body)
		if created.Status != 201 || created.Envelope.Error != nil {
			t.Fatal("Actual user-declared account setup failed", created)
		}
		account := created.Envelope.Data.(domain.DeclaredAccount)
		accountID = account.ID
		contexts, err := e.db.RoutingContexts(ctx)
		if err != nil || len(contexts.Items) != 2 {
			t.Fatal(contexts, err)
		}
		for _, native := range contexts.Items {
			body, err := json.Marshal(domain.AccountLinkInput{RequestID: domain.NewID(), ExpectedRevision: native.AccountLink.Revision, DeclaredAccountID: accountID})
			if err != nil {
				t.Fatal(err)
			}
			linked := e.MutateRouting(ctx, "routing.links.update", native.ID, body)
			if linked.Envelope.Error != nil {
				t.Fatal("Actual saved context/account link failed", linked)
			}
		}
	}
	preparations := &graphParallelPreparations{cwds: map[string]int{}, released: map[string]bool{}, prior: process.Prepare}
	e.config.Prepare = preparations.prepare
	nodes := []domain.GraphNodeInput{}
	for i, key := range []string{"a", "b"} {
		node := domain.GraphNodeInput{Key: key, OriginalText: "Author the controlled independent file after the fixed release, then independently inspect the whole candidate for node " + key + ".",
			FakeScenario: "graph-wait", Workflow: &domain.WorkflowInput{Review: "required"},
			Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect the entire controlled candidate and preserve all user source and staged bytes."}}
		if maxParallelism == "1" || sharedAccount {
			// Two explicit harnesses isolate the narrower graph/account restriction.
			node.Pins = &domain.ExecutionPins{HarnessID: []string{"codex", "claude-code"}[i]}
		}
		nodes = append(nodes, node)
	}
	input := domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", MaxParallelism: maxParallelism,
		Routing: &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "4"}}, Nodes: nodes}
	for _, option := range options {
		option(&input)
	}
	created := e.createGraph(ctx, input)
	if created.Status != 201 || created.Envelope.Error != nil {
		t.Fatal("The actual two-node actor declaration failed", created)
	}
	admission := created.Envelope.Data.(domain.GraphAdmission)
	f := graphParallelFixture{e: e, graphID: admission.Graph.ID, repo: repo, working: working, stagedIndex: index, preparations: preparations, accountID: accountID}
	t.Cleanup(func() {
		// Release only the exact fixed markers in still-ready owned directories.
		// Engine cleanup remains responsible for every actual child/claim.
		for cwd := range preparations.snapshot() {
			if body, err := os.ReadFile(filepath.Join(cwd, fixture.GraphReadyFile)); err == nil && string(body) == "ready\n" {
				if err := graphParallelRelease(f, cwd); err != nil {
					t.Error(err)
				}
			}
		}
	})
	return f
}

func graphParallelRelease(f graphParallelFixture, cwd string) error {
	relative, err := filepath.Rel(filepath.Join(f.e.lock.Root(), "workspaces"), cwd)
	if err != nil || !filepath.IsLocal(relative) || strings.ContainsAny(relative, "/\\") || !domain.ValidID(relative) {
		return errors.New("The controlled release is outside its actual private author workspace.")
	}
	f.preparations.mu.Lock()
	if f.preparations.released[cwd] {
		f.preparations.mu.Unlock()
		return nil
	}
	// No observer may reopen Ready after publication: on Windows such a
	// transient reader can prevent the child from removing its own marker.
	f.preparations.released[cwd] = true
	f.preparations.mu.Unlock()
	temporary := filepath.Join(cwd, "."+fixture.GraphReleaseFile+"-"+domain.NewID()+".tmp")
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temporary) }() // Only this newly created owned file.
	_, writeErr := file.Write([]byte("release\n"))
	if err = errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return err
	}
	destination := filepath.Join(cwd, fixture.GraphReleaseFile)
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		return errors.New("The fixed controlled release name already exists or cannot be inspected.")
	}
	// The child sees only a complete marker with no writer still open. This
	// changes the test barrier, never its fixed worker command or deadline.
	return os.Rename(temporary, destination)
}

func graphParallelReadyWorkers(t *testing.T, f graphParallelFixture) ([]graphParallelWorker, domain.GraphDetail) {
	t.Helper()
	ctx := context.Background()
	view, err := f.e.db.Graph(ctx, f.graphID)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := f.e.db.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatalf("Actual overlap/serialization fixture fenced: %s", graphParallelDiagnostics(f))
	}
	prepared := f.preparations.snapshot()
	ready := []graphParallelWorker{}
	for _, node := range view.Nodes {
		if node.LatestAttemptID == nil {
			continue
		}
		a, err := f.e.db.Attempt(ctx, *node.LatestAttemptID)
		if err != nil {
			t.Fatal(err)
		}
		if a.Role != "author" || a.Phase != "running" || a.StartedAt == nil || a.SettledAt != nil || a.SourceSnapshotID == nil {
			continue
		}
		cwd := filepath.Join(f.e.lock.Root(), "workspaces", *a.SourceSnapshotID)
		f.preparations.mu.Lock()
		released := f.preparations.released[cwd]
		f.preparations.mu.Unlock()
		if prepared[cwd] != 1 || released {
			continue
		}
		body, err := os.ReadFile(filepath.Join(cwd, fixture.GraphReadyFile))
		if err == nil && string(body) == "ready\n" {
			ready = append(ready, graphParallelWorker{node: node, attempt: a, cwd: cwd})
		}
	}
	return ready, view
}

func graphParallelDiagnostics(f graphParallelFixture) string {
	ctx := context.Background()
	text := graphActorDiagnostics(f.e, f.graphID)
	view, _ := f.e.db.Graph(ctx, f.graphID)
	for _, node := range view.Nodes {
		steps, _ := f.e.db.WorkflowSteps(ctx, node.TaskID, domain.WorkflowSelector{}, 10, "")
		for _, step := range steps.Items {
			if step.AttemptID == nil {
				continue
			}
			a, err := f.e.db.Attempt(ctx, *step.AttemptID)
			text += fmt.Sprintf("\nactual node=%s role=%s attempt=%+v failure=%+v err=%v", node.Key, step.Role, a, a.PrimaryFailure, err)
			artifacts, _ := f.e.db.Artifacts(ctx, a.ID, 100, "")
			for _, artifact := range artifacts.Items {
				if artifact.Kind != "stderr" {
					continue
				}
				meta, verifyErr := f.e.evidence.VerifyClosed(ctx, artifact.ID)
				if verifyErr != nil {
					text += fmt.Sprintf("\nstderr=%s verifyErr=%v", artifact.ID, verifyErr)
					continue
				}
				body, readErr := f.e.readAllResponse(meta)
				if len(body) > 4096 {
					body = body[:4096] // Diagnostic preview only; retained evidence stays exact.
				}
				text += fmt.Sprintf("\nstderr=%q err=%v", body, readErr)
			}
		}
	}
	for cwd, count := range f.preparations.snapshot() {
		for _, name := range []string{fixture.GraphReadyFile, fixture.GraphReleaseFile, fixture.GraphFileA} {
			body, err := os.ReadFile(filepath.Join(cwd, name))
			text += fmt.Sprintf("\ncwd=%s prepareCount=%d file=%s bytes=%q err=%v", cwd, count, name, body, err)
		}
	}
	return text
}

func graphParallelAwait(t *testing.T, f graphParallelFixture, duration time.Duration, description string, predicate func([]graphParallelWorker, domain.GraphDetail) bool) []graphParallelWorker {
	t.Helper()
	deadline := time.Now().Add(duration)
	for {
		ready, view := graphParallelReadyWorkers(t, f)
		if predicate(ready, view) {
			return ready
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %s", description, graphParallelDiagnostics(f))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func graphParallelFinish(t *testing.T, f graphParallelFixture) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(15 * time.Second)
	for {
		view, err := f.e.db.Graph(ctx, f.graphID)
		if err != nil {
			t.Fatal(err)
		}
		if view.Graph.State == "completed" {
			if view.Graph.Counts.Completed != "2" || view.Budget.Budget.ConsumedAdmissions != "4" || view.Budget.Budget.GrantedAdmissions != "4" {
				t.Fatal("Actual native completion lost required nodes or the shared finite budget", view)
			}
			for _, node := range view.Nodes {
				workflow, err := f.e.db.Workflow(ctx, node.TaskID, domain.WorkflowSelector{})
				steps, stepErr := f.e.db.WorkflowSteps(ctx, node.TaskID, domain.WorkflowSelector{}, 10, "")
				if err != nil || stepErr != nil || workflow.Acceptance == nil || workflow.Workflow == nil || node.AcceptedResult == nil || node.AcceptedResult.AcceptanceID != workflow.Acceptance.ID || steps.NextCursor != nil || len(steps.Items) != 2 {
					t.Fatal("Actual independent result has no author/review acceptance lineage", node, workflow, steps, err, stepErr)
				}
				roles := map[string]bool{}
				for _, step := range steps.Items {
					if step.AttemptID == nil || step.ExecutorKind != "native" || step.Status != "succeeded" || roles[step.Role] {
						t.Fatal("A real native author/reviewer occurrence was missing or duplicated", step)
					}
					roles[step.Role] = true
					a, err := f.e.db.Attempt(ctx, *step.AttemptID)
					if err != nil || a.StartedAt == nil || a.Outcome == nil || *a.Outcome != "succeeded" || a.Cleanup.State != "verified" || a.EvidenceStatus != "complete" || a.ExecutionMode != "fake" || a.TargetObservations.Model.Value != nil || a.TargetObservations.Account.Value != nil || a.TargetObservations.Usage.InputTokens != nil || a.TargetObservations.Usage.OutputTokens != nil {
						t.Fatal("Actual offline workers acquired false success, live identity or measured telemetry", a, err)
					}
				}
				if !roles["author"] || !roles["review"] {
					t.Fatal("The controlled independent result skipped its required review", roles)
				}
			}
			break
		}
		gate, err := f.e.db.Gate(ctx)
		if err != nil || gate.State != "open" || time.Now().After(deadline) {
			t.Fatalf("Actual released workers did not finish their two required reviews: %s", graphParallelDiagnostics(f))
		}
		time.Sleep(10 * time.Millisecond)
	}
	working, workingErr := os.ReadFile(filepath.Join(f.repo, "source.txt"))
	index, indexErr := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if workingErr != nil || indexErr != nil || !bytes.Equal(working, f.working) || !bytes.Equal(index, f.stagedIndex) {
		t.Fatal("Actual independent workers changed the original dirty source or staged index", workingErr, indexErr)
	}
	for _, name := range []string{fixture.GraphFileA, fixture.GraphReadyFile, fixture.GraphReleaseFile} {
		if _, err := os.Stat(filepath.Join(f.repo, name)); !os.IsNotExist(err) {
			t.Fatal("A fixed worker result/marker escaped its owned workspace", name, err)
		}
	}
}

func TestGraphParallelDefaultAutoRunsTwoActualContextsTogether(t *testing.T) {
	f := graphParallelStart(t, "", false)
	first := graphParallelAwait(t, f, 15*time.Second, "The first actual fixed author did not reach its barrier", func(ready []graphParallelWorker, _ domain.GraphDetail) bool { return len(ready) != 0 })
	_ = first // The second barrier must arrive within the first child's fixed 8s wait.
	ready := graphParallelAwait(t, f, 6*time.Second, "Default unpinned Auto did not overlap two actual ready workers", func(ready []graphParallelWorker, view domain.GraphDetail) bool {
		if len(ready) != 2 || len(view.ActiveClaims) != 2 || view.Budget.Budget.ConsumedAdmissions != "2" {
			return false
		}
		return true
	})
	if ready[0].cwd == ready[1].cwd || ready[0].attempt.ConnectionID == ready[1].attempt.ConnectionID {
		t.Fatal("Two ready files did not belong to distinct actual native contexts/workspaces", ready)
	}
	view, err := f.e.db.Graph(context.Background(), f.graphID)
	if err != nil || view.Graph.MaxParallelism != "2" {
		t.Fatal("The overlap fixture changed the default graph concurrency", view, err)
	}
	contexts := map[string]bool{}
	for _, worker := range ready {
		matched := false
		for _, claim := range view.ActiveClaims {
			matched = matched || claim.AttemptID == worker.attempt.ID && claim.ExecutorKind == "native" && claim.ContextID != nil && *claim.ContextID == worker.attempt.ConnectionID
		}
		if !matched || worker.attempt.ResolvedTarget == nil || worker.attempt.ResolvedTarget.ContextID != worker.attempt.ConnectionID {
			t.Fatal("Ready process lacked its exact current native reservation", worker, view.ActiveClaims)
		}
		contexts[worker.attempt.ConnectionID] = true
		if err := graphParallelRelease(f, worker.cwd); err != nil {
			t.Fatal(err)
		}
	}
	if len(contexts) != 2 {
		t.Fatal("Actual overlap did not use both supported contexts", contexts)
	}
	graphParallelFinish(t, f)
}

func TestGraphParallelNarrowerGraphAndSharedAccountConstraintsActuallySerialize(t *testing.T) {
	for _, kind := range []string{"one_graph_slot", "one_declared_account"} {
		t.Run(kind, func(t *testing.T) {
			limit, account := "1", false
			if kind == "one_declared_account" {
				limit, account = "", true
			}
			f := graphParallelStart(t, limit, account)
			ready := graphParallelAwait(t, f, 15*time.Second, "The first actual serialized author did not reach its barrier", func(ready []graphParallelWorker, _ domain.GraphDetail) bool { return len(ready) != 0 })
			if len(ready) != 1 {
				t.Fatal("A narrower declared constraint already overlapped two real authors", ready)
			}
			first := ready[0]
			holdUntil, waitDeadline := time.Now().Add(300*time.Millisecond), time.Now().Add(3*time.Second)
			for {
				current, view := graphParallelReadyWorkers(t, f)
				if len(current) != 1 || current[0].attempt.ID != first.attempt.ID || len(f.preparations.snapshot()) != 1 || view.Budget.Budget.ConsumedAdmissions != "1" {
					t.Fatalf("A second native preparation/admission passed a held graph/account constraint: %s", graphParallelDiagnostics(f))
				}
				waiting := false
				for _, blocker := range view.Blockers {
					waiting = waiting || blocker.NodeID != nil && *blocker.NodeID != first.node.ID && (blocker.Code == "capacity_wait" || blocker.Code == "attempt_active")
				}
				if !account {
					pending, err := f.e.db.PendingAttempts(context.Background(), f.e.config.OwnerInstanceID, 100)
					if err != nil {
						t.Fatal(err)
					}
					for _, a := range pending {
						if a.ExecutorKind == "local_graph" && a.Phase == "queued" {
							op, err := f.e.db.GraphInputOperationForAttempt(context.Background(), f.e.config.OwnerInstanceID, a.ID)
							waiting = waiting || err == nil && op.GraphID == f.graphID && op.Control.NodeID != nil && *op.Control.NodeID != first.node.ID
						}
					}
				}
				if waiting && time.Now().After(holdUntil) {
					break // A real contender stayed blocked across several actor ticks.
				}
				if time.Now().After(waitDeadline) {
					t.Fatalf("No actual pending/capacity-wait contender proved serialization: %s", graphParallelDiagnostics(f))
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := graphParallelRelease(f, first.cwd); err != nil {
				t.Fatal(err)
			}
			second := graphParallelAwait(t, f, 15*time.Second, "Actual release did not wake the other captured context", func(ready []graphParallelWorker, _ domain.GraphDetail) bool {
				for _, worker := range ready {
					if worker.attempt.ID != first.attempt.ID {
						return true
					}
				}
				return false
			})
			if len(second) != 1 || second[0].attempt.ID == first.attempt.ID || second[0].attempt.ConnectionID == first.attempt.ConnectionID {
				t.Fatal("Serialization did not eventually run the distinct explicitly captured context", first, second)
			}
			settledFirst, err := f.e.db.Attempt(context.Background(), first.attempt.ID)
			if err != nil || settledFirst.Phase != "settled" || settledFirst.SettledAt == nil {
				t.Fatal("The second process started before the actual first process settled", settledFirst, err)
			}
			ended, endErr := time.Parse(time.RFC3339Nano, *settledFirst.SettledAt)
			started, startErr := time.Parse(time.RFC3339Nano, *second[0].attempt.StartedAt)
			if endErr != nil || startErr != nil || started.Before(ended) {
				t.Fatal("Actual serialized execution intervals overlapped", settledFirst, second[0].attempt, endErr, startErr)
			}
			if account {
				for _, worker := range []graphParallelWorker{first, second[0]} {
					if worker.attempt.ResolvedTarget == nil || worker.attempt.ResolvedTarget.DeclaredAccountAssociation == nil || worker.attempt.ResolvedTarget.DeclaredAccountAssociation.AccountID != f.accountID {
						t.Fatal("Shared-account serialization did not bind the same actual saved declaration", worker.attempt)
					}
				}
			}
			if err := graphParallelRelease(f, second[0].cwd); err != nil {
				t.Fatal(err)
			}
			graphParallelFinish(t, f)
		})
	}
}
