package cli

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type graphIntentOwner struct {
	t                          *testing.T
	root, graph, node, request string
	mu                         sync.Mutex
	detail                     domain.GraphDetail
	bodies                     [][]byte
	detailReads                atomic.Int32
	dropNext                   atomic.Bool
	arrived                    chan struct{}
	release                    chan struct{}
	releaseOnce                sync.Once
	hold                       atomic.Bool
}

func newGraphIntentOwner(t *testing.T) *graphIntentOwner {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	root, err := localstate.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	f := &graphIntentOwner{t: t, root: lock.Root(), graph: strings.Repeat("1", 32), node: strings.Repeat("2", 32), request: strings.Repeat("3", 32), arrived: make(chan struct{}, 1), release: make(chan struct{})}
	ownerID, rootTask, project := domain.NewID(), domain.NewID(), domain.NewID()
	counts := domain.GraphCounts{Required: "1", Waiting: "1", Ready: "0", Preparing: "0", Queued: "0", Running: "0", Blocked: "0", Cancelling: "0", Cancelled: "0", Failed: "0", Interrupted: "0", Completed: "0"}
	f.detail = domain.GraphDetail{Graph: domain.GraphSummary{ID: f.graph, RootTaskID: rootTask, ProjectID: project, GraphVersion: "1", ControlRevision: "2", StateRevision: "3", BudgetAuthorityEpoch: "4", MaxParallelism: "2", State: "blocked", NextOwner: "user", NextAction: "advance", Counts: counts, CreatedAt: domain.Now(), UpdatedAt: domain.Now()}, Definition: domain.GraphDefinitionRef{GraphID: f.graph, GraphVersion: "1", DeclarationSHA256: strings.Repeat("a", 64)}, Nodes: []domain.GraphNodeSummary{{ID: f.node, GraphID: f.graph, Key: "A", TaskID: domain.NewID(), TaskVersion: "5", TaskRevision: "6", ControlRevision: "7", StateRevision: "8", State: "waiting", NextOwner: "user", NextAction: "advance"}}, Budget: domain.GraphBudgetView{GraphID: f.graph, BudgetOwnerTaskID: rootTask, BudgetAuthorityEpoch: "4", Budget: domain.TaskBudget{TaskID: rootTask, Revision: "9", GrantedAdmissions: "4", ConsumedAdmissions: "0", RemainingAdmissions: "4", AutomaticAdmissions: "0", HistoricalAdmissions: "0", CurrentWindow: domain.BudgetWindow{Number: "1", WindowSeconds: 900}, UpdatedAt: domain.Now()}, NodeConsumption: []domain.GraphNodeConsumption{}}, ActiveClaims: []domain.GraphClaimView{}, Blockers: []domain.GraphBlockerSummary{}}
	token := "offline-graph-intent-token-" + domain.NewID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("unauthenticated controlled request")
			w.WriteHeader(401)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/api/v1/meta" {
			_ = json.MarshalWrite(w, domain.Success(200, domain.NewID(), graphMetadataFixture(ownerID, 0), nil).Envelope)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/api/v1/graphs/"+f.graph {
			f.detailReads.Add(1)
			f.mu.Lock()
			detail := f.detail
			f.mu.Unlock()
			if f.hold.Load() {
				select {
				case f.arrived <- struct{}{}:
				default:
				}
				select {
				case <-f.release:
				case <-r.Context().Done():
					return
				}
			}
			_ = json.MarshalWrite(w, domain.Success(200, domain.NewID(), detail, nil).Envelope)
			return
		}
		if r.Method != "POST" {
			t.Error("unexpected controlled route", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, domain.MaxBodyBytes+1))
		if err != nil {
			t.Error(err)
			return
		}
		f.mu.Lock()
		f.bodies = append(f.bodies, append([]byte{}, body...))
		f.mu.Unlock()
		if f.dropNext.Swap(false) {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		}
		var input struct {
			RequestID string `json:"requestId"`
		}
		if json.Unmarshal(body, &input) != nil {
			t.Error("invalid controlled mutation")
			return
		}
		op := "graphs.advance"
		if strings.HasSuffix(r.URL.Path, "/stop") {
			op = "graphs.stop"
			if strings.Contains(r.URL.Path, "/nodes/") {
				op = "graphs.nodes.stop"
			}
		}
		if strings.HasSuffix(r.URL.Path, "/guidance") {
			op = "graphs.nodes.guidance"
		}
		if strings.HasSuffix(r.URL.Path, "/budget-amendments") {
			op = "graphs.budget.amend"
		}
		receipt := &domain.Receipt{RequestID: input.RequestID, Operation: op, CommittedAt: domain.Now()}
		_ = json.MarshalWrite(w, domain.Success(200, input.RequestID, map[string]bool{"acknowledged": true}, receipt).Envelope)
	}))
	t.Cleanup(func() { f.releaseOnce.Do(func() { close(f.release) }); server.Close() })
	if err := lock.WriteDescriptor(localstate.Descriptor{OwnerInstanceID: ownerID, PID: os.Getpid(), StartedAt: domain.Now(), Origin: server.URL, APIVersion: 1, Token: token}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *graphIntentOwner) run(args ...string) (int, domain.Envelope, string) {
	f.t.Helper()
	var out, diagnostics bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	code := Run(ctx, append(args, "--json"), nil, &out, &diagnostics)
	var envelope domain.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		f.t.Fatal("invalid CLI output", err, out.String())
	}
	return code, envelope, diagnostics.String()
}

func TestGraphIntentLostAcknowledgementRetainsBodyWithoutNewDetail(t *testing.T) {
	f := newGraphIntentOwner(t)
	f.dropNext.Store(true)
	args := []string{"graphs", "advance", f.graph, "--request-id", f.request, "--rebind-node", "A"}
	code, first, _ := f.run(args...)
	if code == 0 || first.Error == nil || first.Error.Code != "write_uncertain" || first.RequestID != f.request {
		t.Fatalf("first acknowledgement was not uncertain: %d %+v", code, first)
	}
	f.mu.Lock()
	f.detail.Graph.ControlRevision = "90"
	f.detail.Nodes[0].TaskVersion = "91"
	f.detail.Nodes[0].ControlRevision = "92"
	f.mu.Unlock()
	code, retry, _ := f.run(args...)
	if code != 0 || retry.Error != nil {
		t.Fatalf("exact retained retry failed: %d %+v", code, retry)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.detailReads.Load() != 1 || len(f.bodies) != 2 || !bytes.Equal(f.bodies[0], f.bodies[1]) {
		t.Fatal("retry refreshed authority or body", f.detailReads.Load(), len(f.bodies))
	}
	var in domain.GraphAdvanceInput
	if domain.StrictDecode(f.bodies[0], &in) != nil || in.ExpectedGraphControlRevision != "2" || in.NodeAuthorities[0].TaskVersion != "5" || in.RebindNodeIDs == nil || len(*in.RebindNodeIDs) != 1 || (*in.RebindNodeIDs)[0] != f.node {
		t.Fatal("captured rebind did not retain exact original detail")
	}
}

func TestGraphIntentHeldDetailAndConcurrentCLIShareOneFrozenCapture(t *testing.T) {
	f := newGraphIntentOwner(t)
	f.hold.Store(true)
	args := []string{"graphs", "advance", f.graph, "--request-id", f.request}
	done := make(chan int, 2)
	go func() { code, _, _ := f.run(args...); done <- code }()
	select {
	case <-f.arrived:
	case <-time.After(3 * time.Second):
		t.Fatal("no actual detail read")
	}
	go func() { code, _, _ := f.run(args...); done <- code }()
	f.mu.Lock()
	f.detail.Graph.ControlRevision = "90"
	f.mu.Unlock()
	f.releaseOnce.Do(func() { close(f.release) })
	for range 2 {
		select {
		case code := <-done:
			if code != 0 {
				t.Fatal("concurrent exact intent failed", code)
			}
		case <-time.After(6 * time.Second):
			t.Fatal("concurrent capture did not finish")
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.detailReads.Load() != 1 || len(f.bodies) != 2 || !bytes.Equal(f.bodies[0], f.bodies[1]) {
		t.Fatal("two CLI invocations captured different authority", f.detailReads.Load(), len(f.bodies))
	}
}

func TestGraphIntentChangedInputAndDamagedCacheCannotDispatch(t *testing.T) {
	f := newGraphIntentOwner(t)
	file := filepath.Join(t.TempDir(), "guidance.txt")
	if os.WriteFile(file, []byte("private exact original guidance"), 0600) != nil {
		t.Fatal("fixture")
	}
	args := []string{"graphs", "guide", f.graph, "--node", "A", "--request-id", f.request, "--guidance-file", file}
	if code, envelope, diagnostics := f.run(args...); code != 0 || envelope.Error != nil || strings.Contains(diagnostics, "private exact original guidance") {
		t.Fatalf("first frozen guidance failed: %d %+v", code, envelope)
	}
	if os.WriteFile(file, []byte("newer edited guidance"), 0600) != nil {
		t.Fatal("fixture")
	}
	code, envelope, _ := f.run(args...)
	if code == 0 || envelope.Error == nil || envelope.Error.Code != "request_conflict" {
		t.Fatalf("changed input reused prior identity: %d %+v", code, envelope)
	}
	if f.detailReads.Load() != 1 {
		t.Fatal("changed input refreshed detail")
	}
	if os.WriteFile(file, []byte("private exact original guidance"), 0600) != nil {
		t.Fatal("fixture")
	}
	cache := filepath.Join(f.root, "client-intents", "graphs", f.request, "body.json")
	if err := os.WriteFile(cache, []byte(`{"incomplete":`), 0600); err != nil {
		t.Fatal("missing actual frozen cache", err)
	}
	code, envelope, _ = f.run(args...)
	if code == 0 || envelope.Error == nil || envelope.Error.Code != "write_uncertain" {
		t.Fatalf("damaged cache reconstructed authority: %d %+v", code, envelope)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bodies) != 1 || f.detailReads.Load() != 1 {
		t.Fatal("unresolved private intent caused new I/O authority")
	}
}

func TestGraphIntentMissingRedirectedPublicOrLinkedCacheStaysUnresolved(t *testing.T) {
	for _, change := range []string{"missing_body", "redirected_request", "public_body", "linked_body"} {
		t.Run(change, func(t *testing.T) {
			f := newGraphIntentOwner(t)
			args := []string{"graphs", "stop", f.graph, "--request-id", f.request}
			if code, envelope, _ := f.run(args...); code != 0 || envelope.Error != nil {
				t.Fatalf("initial private intent unavailable: %d %+v", code, envelope)
			}
			dir := filepath.Join(f.root, "client-intents", "graphs", f.request)
			body := filepath.Join(dir, "body.json")
			switch change {
			case "missing_body":
				if err := os.Remove(body); err != nil {
					t.Fatal(err)
				}
			case "redirected_request":
				retained := dir + "-retained"
				if err := os.Rename(dir, retained); err != nil {
					t.Fatal(err)
				}
				if err := graphIntentTestAlias(retained, dir); err != nil {
					t.Fatal("owned cache redirect fixture failed", err)
				}
			case "public_body":
				if err := graphIntentTestMakePublic(body); err != nil {
					t.Fatal("owned privacy fixture failed", err)
				}
			case "linked_body":
				if err := os.Link(body, filepath.Join(dir, "second-link")); err != nil {
					t.Fatal("owned hardlink fixture failed", err)
				}
			}
			code, envelope, _ := f.run(args...)
			if code == 0 || envelope.Error == nil || envelope.Error.Code != "write_uncertain" || envelope.RequestID != f.request {
				t.Fatalf("unresolved cache created fresh authority: %d %+v", code, envelope)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.bodies) != 1 || f.detailReads.Load() != 1 {
				t.Fatal("invalid private cache caused new detail/POST", len(f.bodies), f.detailReads.Load())
			}
		})
	}
}

func TestGraphIntentInvalidDetailCannotCreateFrozenMutation(t *testing.T) {
	for _, change := range []string{"foreign_graph", "wrong_budget_owner", "no_nodes", "duplicate_nodes", "unsafe_node_version", "empty_graph_control"} {
		t.Run(change, func(t *testing.T) {
			f := newGraphIntentOwner(t)
			switch change {
			case "foreign_graph":
				f.detail.Graph.ID = domain.NewID()
			case "wrong_budget_owner":
				f.detail.Budget.BudgetOwnerTaskID = domain.NewID()
			case "no_nodes":
				f.detail.Nodes = []domain.GraphNodeSummary{}
			case "duplicate_nodes":
				f.detail.Nodes = append(f.detail.Nodes, f.detail.Nodes[0])
			case "unsafe_node_version":
				f.detail.Nodes[0].TaskVersion = "9223372036854775808"
			case "empty_graph_control":
				f.detail.Graph.ControlRevision = ""
			}
			code, envelope, _ := f.run("graphs", "advance", f.graph, "--request-id", f.request)
			if code == 0 || envelope.Error == nil || envelope.Error.Code != "invalid_response" {
				t.Fatalf("invalid authority detail accepted: %d %+v", code, envelope)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.bodies) != 0 || f.detailReads.Load() != 1 {
				t.Fatal("invalid detail created dispatch or refreshed authority", len(f.bodies), f.detailReads.Load())
			}
		})
	}
}
