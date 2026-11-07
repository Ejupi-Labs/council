package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/engine"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/owner"
	"github.com/Ejupi-Labs/council/v2/internal/store"
	"github.com/Ejupi-Labs/council/v2/internal/webui"
)

func requiresExecutionOwner(op string) bool {
	return strings.HasPrefix(op, "graphs.") || strings.HasPrefix(op, "workflow.") || strings.HasPrefix(op, "candidates.") || strings.HasPrefix(op, "routing.") || strings.HasPrefix(op, "tasks.") || strings.HasPrefix(op, "attempts.") || strings.HasPrefix(op, "artifacts.") || strings.HasPrefix(op, "execution.") || op == "service.stop"
}

// HandleInternal starts a continuing owner of this exact executable. Task bodies,
// tokens and alternate executables are never accepted by this internal entrypoint.
func HandleInternal(ctx context.Context, args []string, stderr io.Writer) (bool, int) {
	if len(args) == 0 || args[0] != owner.InternalArg {
		return false, 0
	}
	if len(args) != 1 {
		return true, 2
	}
	root, err := localstate.DefaultRoot()
	if err != nil {
		return true, 1
	}
	lock, err := localstate.Acquire(root)
	if err != nil {
		return true, 1
	}
	defer lock.Close()
	db, err := store.Open(lock.DatabasePath())
	if err != nil {
		return true, 1
	}
	defer db.Close()
	return true, serveOwner(ctx, lock, db, nil, io.Discard, stderr)
}

func serveOwner(ctx context.Context, lock *localstate.Lock, db *store.Store, c *command, stdout, stderr io.Writer) int {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 1
	}
	defer listener.Close()
	var token [32]byte
	if _, err = rand.Read(token[:]); err != nil {
		return 1
	}
	desc := localstate.Descriptor{OwnerInstanceID: domain.NewID(), PID: os.Getpid(), StartedAt: domain.Now(), Origin: "http://" + listener.Addr().String(), APIVersion: 1, Token: hex.EncodeToString(token[:])}
	controller, err := engine.New(db, lock, engine.Config{OwnerInstanceID: desc.OwnerInstanceID})
	if err != nil {
		return 1
	}
	defer controller.Close()
	service := app.New(db)
	service.Execution = controller
	server := &http.Server{Handler: httpapi.New(service, httpapi.Config{Host: listener.Addr().String(), Token: desc.Token, OwnerInstanceID: desc.OwnerInstanceID, Assets: webui.Files()}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	if err = lock.WriteDescriptor(desc); err != nil {
		_ = server.Close()
		<-done
		return 1
	}
	defer lock.RemoveOwnDescriptor(desc.OwnerInstanceID)
	if c != nil {
		showUI(*c, desc, stdout, stderr)
	}
	code := 0
	select {
	case <-ctx.Done():
		_ = controller.Shutdown(context.Background(), desc.OwnerInstanceID, domain.NewID())
		code = 130
	case <-controller.ShutdownRequested():
	case <-done:
		return 1
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err = server.Shutdown(shutdown); err != nil {
		_ = server.Close()
	}
	cancel()
	<-done
	return code
}
