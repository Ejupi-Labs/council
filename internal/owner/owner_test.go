package owner

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == InternalArg {
		root := os.Getenv("COUNCIL_OWNER_TEST_ROOT")
		if root == "" || len(os.Args) != 2 {
			os.Exit(2)
		}
		os.Exit(runOwnerFixture(root))
	}
	os.Exit(m.Run())
}
func runOwnerFixture(root string) int {
	lock, err := localstate.Acquire(root)
	if err != nil {
		return 3
	}
	defer lock.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 3
	}
	defer listener.Close()
	d := localstate.Descriptor{OwnerInstanceID: domain.NewID(), PID: os.Getpid(), StartedAt: domain.Now(), Origin: "http://" + listener.Addr().String(), APIVersion: 1, Token: domain.NewID() + domain.NewID()}
	count, err := os.OpenFile(filepath.Join(root, "fixture-owner-starts"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return 3
	}
	if _, err = count.WriteString("start\n"); err != nil {
		count.Close()
		return 3
	}
	count.Close()
	mux := http.NewServeMux()
	done := make(chan struct{}, 1)
	mux.HandleFunc("/meta", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+d.Token {
			w.WriteHeader(401)
			return
		}
		data, _ := json.Marshal(map[string]string{"ownerInstanceId": d.OwnerInstanceID})
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+d.Token {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(204)
		select {
		case done <- struct{}{}:
		default:
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go server.Serve(listener)
	if lock.WriteDescriptor(d) != nil {
		server.Close()
		return 3
	}
	defer lock.RemoveOwnDescriptor(d.OwnerInstanceID)
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
	server.Close()
	return 0
}
func fixtureRequest(ctx context.Context, d localstate.Descriptor, path string) (*http.Response, error) {
	r, err := http.NewRequestWithContext(ctx, "GET", d.Origin+path, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+d.Token)
	return (&http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: time.Second}).Do(r)
}
func verifyFixture(ctx context.Context, d localstate.Descriptor) bool {
	r, err := fixtureRequest(ctx, d, "/meta")
	if err != nil {
		return false
	}
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 8192))
	if err != nil || r.StatusCode != 200 {
		return false
	}
	var got struct {
		OwnerInstanceID string `json:"ownerInstanceId"`
	}
	return json.Unmarshal(data, &got, json.RejectUnknownMembers(true)) == nil && got.OwnerInstanceID == d.OwnerInstanceID
}
func stopFixture(t *testing.T, d localstate.Descriptor, root string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := fixtureRequest(ctx, d, "/stop")
	if err == nil {
		r.Body.Close()
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		l, err := localstate.Acquire(root)
		if err == nil {
			l.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fixture owner did not release its actual native lock")
}

func TestConcurrentBootstrapConvergesAndObserverLossDoesNotStopOwner(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	t.Setenv("COUNCIL_OWNER_TEST_ROOT", root)
	var wg sync.WaitGroup
	var results [2]localstate.Descriptor
	var errs [2]error
	// Register authenticated fixture shutdown before any assertion can fail.
	// One client can observe a bootstrap error while the other starts the owner.
	t.Cleanup(func() {
		for _, descriptor := range results {
			if descriptor.OwnerInstanceID != "" {
				stopFixture(t, descriptor, root)
				return
			}
		}
		// Both observers can lose their acknowledgement after actual publication.
		// Read only this test's private descriptor and authenticate the fixture.
		if descriptor, err := localstate.ReadDescriptor(root); err == nil && verifyFixture(context.Background(), descriptor) {
			stopFixture(t, descriptor, root)
		}
	})
	for n := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			results[n], errs[n] = Ensure(ctx, root, verifyFixture)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0].OwnerInstanceID != results[1].OwnerInstanceID {
		t.Fatal("clients observed different competing owners")
	}
	starts, err := os.ReadFile(filepath.Join(root, "fixture-owner-starts"))
	if err != nil || string(starts) != "start\n" {
		t.Fatalf("bootstrap dispatched multiple lock-owning services: %v", err)
	}
	if !verifyFixture(context.Background(), results[0]) {
		t.Fatal("owner lifetime followed a completed/cancelled observer")
	}
	if l, err := localstate.Acquire(root); !errors.Is(err, localstate.ErrLocked) {
		if l != nil {
			l.Close()
		}
		t.Fatal("published owner did not retain the OS lock")
	}
}
func TestHeldUnreachableOwnerIsNeverReplaced(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	t.Setenv("COUNCIL_OWNER_TEST_ROOT", root)
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := Ensure(ctx, root, verifyFixture); err == nil {
		t.Fatal("unreachable locked owner was bypassed")
	}
	if _, err := os.Stat(filepath.Join(root, "fixture-owner-starts")); !os.IsNotExist(err) {
		t.Fatal("an incompatible owner caused a competing launch")
	}
}
func TestReadinessBoundIncludesVerifierCall(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	d := localstate.Descriptor{OwnerInstanceID: domain.NewID(), PID: os.Getpid(), StartedAt: domain.Now(), Origin: "http://127.0.0.1:34567", APIVersion: 1, Token: strings.Repeat("1", 64)}
	if lock.WriteDescriptor(d) != nil {
		t.Fatal("descriptor failed")
	}
	defer lock.RemoveOwnDescriptor(d.OwnerInstanceID)
	started := time.Now()
	_, err = ensureWithin(context.Background(), root, func(ctx context.Context, _ localstate.Descriptor) bool { <-ctx.Done(); return false }, 100*time.Millisecond)
	if err == nil || time.Since(started) > time.Second {
		t.Fatal("readiness verifier escaped its total bounded observation")
	}
}
func TestCancelledBootstrapDoesNotCreateState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "uncreated")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Ensure(ctx, root, verifyFixture); err == nil {
		t.Fatal("cancelled bootstrap succeeded")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("cancelled observer created state")
	}
}
