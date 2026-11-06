package cli

import (
	"bufio"
	"bytes"
	"context"
	json "encoding/json/v2"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/engine"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

const maintenanceFixtureArg = "__council_known002_owner_test"

// This separately running fixture freezes the accepted002 authenticated wire
// profile. Its real controller/store retain the Stop receipt and exact-owner
// replay semantics. Schema2 data migration is independently covered by store
// tests; this is not a claim to execute an archived002 application binary.
func runMaintenanceFixture(args []string) int {
	if len(args) != 1 || args[0] != "drop-ack" {
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root, err := localstate.DefaultRoot()
	if err != nil {
		return 1
	}
	lock, err := localstate.Acquire(root)
	if err != nil {
		return 1
	}
	defer lock.Close()
	db, err := store.Open(lock.DatabasePath())
	if err != nil {
		return 1
	}
	defer db.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 1
	}
	defer listener.Close()
	desc := localstate.Descriptor{OwnerInstanceID: domain.NewID(), PID: os.Getpid(), StartedAt: domain.Now(), Origin: "http://" + listener.Addr().String(), APIVersion: 1, Token: domain.NewID() + domain.NewID()}
	controller, err := engine.New(db, lock, engine.Config{OwnerInstanceID: desc.OwnerInstanceID})
	if err != nil {
		return 1
	}
	defer controller.Close()
	service := app.New(db)
	service.Execution = controller
	native := httpapi.New(service, httpapi.Config{Host: listener.Addr().String(), Token: desc.Token, OwnerInstanceID: desc.OwnerInstanceID})
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/v1/meta" && r.Header.Get("Authorization") == "Bearer "+desc.Token {
			body, _ := json.Marshal(domain.Success(200, domain.NewID(), metadataFixture(desc.OwnerInstanceID, true), nil).Envelope)
			_, _ = w.Write(body)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/v1/service/stop" {
			recorded := httptest.NewRecorder()
			native.ServeHTTP(recorded, r)
			// The actual mutation/receipt commits; its acknowledgement is lost.
			if recorded.Code == 202 {
				if h, ok := w.(http.Hijacker); ok {
					if connection, _, err := h.Hijack(); err == nil {
						_ = connection.Close()
						return
					}
				}
			}
			for key, values := range recorded.Header() {
				w.Header()[key] = values
			}
			w.WriteHeader(recorded.Code)
			_, _ = io.Copy(w, recorded.Body)
			return
		}
		native.ServeHTTP(w, r)
	})}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	if err = lock.WriteDescriptor(desc); err != nil {
		_ = server.Close()
		<-done
		return 1
	}
	defer lock.RemoveOwnDescriptor(desc.OwnerInstanceID)
	ready, _ := json.Marshal(map[string]string{"ownerInstanceId": desc.OwnerInstanceID})
	_, _ = os.Stdout.Write(append(ready, '\n'))
	select {
	case <-controller.ShutdownRequested():
	case <-ctx.Done():
		_ = controller.Shutdown(context.Background(), desc.OwnerInstanceID, domain.NewID())
	}
	shutdown, end := context.WithTimeout(context.Background(), 5*time.Second)
	if server.Shutdown(shutdown) != nil {
		_ = server.Close()
	}
	end()
	<-done
	return 0
}

func TestKnownOlderOwnerStopLostAcknowledgementAndReplacement(t *testing.T) {
	f := newCLIFixture(t)
	// Prepare a project without leaving a continuing owner.
	f.run("projects", "open", f.repo)
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, f.executable, maintenanceFixtureArg, "drop-ack")
	child.Env = f.command(ctx).Env
	var diagnostics bytes.Buffer
	child.Stderr = &diagnostics
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	childDone := make(chan error, 1)
	go func() { childDone <- child.Wait() }()
	t.Cleanup(func() {
		f.cleanup()
		cancel()
		select {
		case <-childDone:
		case <-time.After(5 * time.Second):
			t.Error("owned older-profile fixture did not exit")
		}
	})
	ready := make(chan []byte, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		if scanner.Scan() {
			ready <- bytes.Clone(scanner.Bytes())
		} else {
			ready <- nil
		}
	}()
	select {
	case body := <-ready:
		if len(body) == 0 {
			t.Fatalf("older fixture did not start: %s", diagnostics.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("older fixture readiness timed out")
	}
	root := filepath.Join(f.home, ".council", "v2")
	old, err := localstate.ReadDescriptor(root)
	if err != nil {
		t.Fatal(err)
	}
	blocked := concurrentCLICommand(ctx, f, 0, "tasks", "list")
	var denied domain.Envelope
	if json.Unmarshal(blocked.stdout, &denied) != nil || denied.Error == nil || denied.Error.Code != "owner_unavailable" {
		t.Fatalf("new execution trusted old profile: %s", blocked.stdout)
	}
	request := domain.NewID()
	lost := concurrentCLICommand(ctx, f, 0, "service", "stop", "--request-id", request)
	var uncertain domain.Envelope
	if json.Unmarshal(lost.stdout, &uncertain) != nil || uncertain.Error == nil || uncertain.Error.Code != "write_uncertain" || uncertain.RequestID != request {
		t.Fatalf("lost Stop acknowledgement was misreported: %s", lost.stdout)
	}
	if !strings.Contains(uncertain.Error.Message, "council service stop --owner-instance "+old.OwnerInstanceID+" --request-id "+request) || strings.Contains(uncertain.Error.Message, old.Token) {
		t.Fatal("ordinary Stop lost its captured non-secret retry target", uncertain.Error.Message)
	}
	select {
	case err := <-childDone:
		if err != nil {
			t.Fatalf("older-profile owner failed: %v %s", err, diagnostics.String())
		}
		childDone <- nil
	case <-time.After(10 * time.Second):
		t.Fatal("accepted older Stop did not finish")
	}
	// This normal read starts the replacement only after the old owner released.
	f.run("tasks", "list")
	current, err := localstate.ReadDescriptor(root)
	if err != nil || current.OwnerInstanceID == old.OwnerInstanceID {
		t.Fatal("no distinct replacement owner", err)
	}
	replayed := f.run("service", "stop", "--owner-instance", old.OwnerInstanceID, "--request-id", request)
	if replayed.Receipt == nil || replayed.Receipt.RequestID != request || replayed.Receipt.Operation != "service.stop" {
		t.Fatal("old exact receipt was not reconciled")
	}
	if !verifyExecutionOwner(ctx, current) {
		t.Fatal("replaying old Stop shut down replacement")
	}
	fresh := concurrentCLICommand(ctx, f, 0, "service", "stop", "--owner-instance", old.OwnerInstanceID, "--request-id", domain.NewID())
	var refused domain.Envelope
	if json.Unmarshal(fresh.stdout, &refused) != nil || refused.Error == nil || refused.Error.Code != "owner_unavailable" || !verifyExecutionOwner(ctx, current) {
		t.Fatal("fresh mismatched Stop affected replacement", string(fresh.stdout))
	}
}
