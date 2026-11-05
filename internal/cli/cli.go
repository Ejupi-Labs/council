// Package cli provides the native Council entrypoint without an intervening shell.
package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/store"
	"github.com/Ejupi-Labs/council/v2/internal/webui"
)

const help = `Council — local workspace foundation

  council ui [--no-open] [--json]
  council projects open PATH | list | get ID
  council policy get | set autonomous|supervised --expected-revision R
  council agents list | refresh
  council drafts create --project ID (--task TEXT | --task-file PATH|-)
  council drafts list [--project ID] | get ID [--revision R]
  council drafts update ID --expected-revision R (--task TEXT | --task-file PATH|-)
  council --version

Commands support --json and --api-version 1. Lists accept --limit N and --cursor CURSOR.
Mutations accept --request-id ID for exact retries. Draft update accepts --use-current-policy.
Private state: <user-home>/.council/v2/. Drafts are not executed by this preview.
`

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) (exit int) {
	c, err := parse(args, stdin)
	if err != nil {
		return emit(stdout, stderr, c.json, domain.Failure(domain.NewID(), err))
	}
	defer func() {
		if recover() != nil {
			failure := domain.Fail("internal_error", "The operation could not complete. Report this request identity.")
			if c.requestID != "" {
				failure = domain.Fail("write_uncertain", "The mutation outcome is uncertain. Retry the same request identity and input.")
			}
			exit = emit(stdout, stderr, c.json, domain.Failure(requestIdentity(c), failure))
		}
	}()
	if c.name == "help" {
		if c.json {
			return emit(stdout, stderr, true, domain.Success(200, domain.NewID(), map[string]string{"help": help}, nil))
		}
		_, _ = io.WriteString(stdout, help)
		return 0
	}
	if c.name == "version" {
		return emit(stdout, stderr, c.json, domain.Success(200, domain.NewID(), map[string]any{"appVersion": domain.AppVersion, "apiVersion": domain.APIVersion}, nil))
	}
	root, rootErr := localstate.DefaultRoot()
	if rootErr != nil {
		return emit(stdout, stderr, c.json, domain.Failure(requestIdentity(c), domain.Fail("owner_unavailable", "Cannot locate the private Council state root.")))
	}
	lock, lockErr := localstate.Acquire(root)
	if errors.Is(lockErr, localstate.ErrLocked) {
		desc, e := localstate.ReadDescriptor(root)
		if e != nil {
			return emit(stdout, stderr, c.json, ownerFailure(c.requestID))
		}
		if !verifyOwner(ctx, desc) {
			return emit(stdout, stderr, c.json, ownerFailure(c.requestID))
		}
		if c.name == "ui" {
			return showUI(c, desc, stdout, stderr)
		}
		return emit(stdout, stderr, c.json, forward(ctx, desc, c))
	}
	if lockErr != nil {
		return emit(stdout, stderr, c.json, domain.Failure(requestIdentity(c), domain.Fail("owner_unavailable", "Cannot establish exclusive private state access. Check permissions and disk availability for <user-home>/.council/v2/, preserve its contents, then retry.")))
	}
	defer lock.Close()
	db, e := store.Open(lock.DatabasePath())
	if e != nil {
		return emit(stdout, stderr, c.json, domain.Failure(requestIdentity(c), domain.Fail("storage_failed", "Cannot open this Council state schema. Preserve the directory and check storage compatibility.")))
	}
	defer db.Close()
	service := app.New(db)
	if c.name != "ui" {
		return emit(stdout, stderr, c.json, service.Execute(ctx, c.op, c.params))
	}
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return emit(stdout, stderr, c.json, ownerFailure(c.requestID))
	}
	var token [32]byte
	if _, e := rand.Read(token[:]); e != nil {
		listener.Close()
		return emit(stdout, stderr, c.json, ownerFailure(c.requestID))
	}
	desc := localstate.Descriptor{OwnerInstanceID: domain.NewID(), PID: os.Getpid(), StartedAt: domain.Now(), Origin: "http://" + listener.Addr().String(), APIVersion: 1, Token: hex.EncodeToString(token[:])}
	server := &http.Server{Handler: httpapi.New(service, httpapi.Config{Host: listener.Addr().String(), Token: desc.Token, OwnerInstanceID: desc.OwnerInstanceID, Assets: webui.Files()}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	if e := lock.WriteDescriptor(desc); e != nil {
		server.Close()
		return emit(stdout, stderr, c.json, ownerFailure(c.requestID))
	}
	defer lock.RemoveOwnDescriptor(desc.OwnerInstanceID)
	showUI(c, desc, stdout, stderr)
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := server.Shutdown(shutdown); e != nil {
			server.Close()
		}
		<-done
		return 130
	case <-done:
		return emit(stdout, stderr, c.json, ownerFailure(c.requestID))
	}
}

func requestIdentity(c command) string {
	if domain.ValidID(c.requestID) {
		return c.requestID
	}
	return domain.NewID()
}

func ownerFailure(requestIDs ...string) domain.Result {
	id := domain.NewID()
	if len(requestIDs) > 0 && domain.ValidID(requestIDs[0]) {
		id = requestIDs[0]
	}
	return domain.Failure(id, domain.Fail("owner_unavailable", "The private Council owner is unavailable or incompatible. Close its foreground process normally, then retry; do not remove its lock."))
}

func client() *http.Client {
	return &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func verifyOwner(ctx context.Context, desc localstate.Descriptor) bool {
	r, err := http.NewRequestWithContext(ctx, "GET", desc.Origin+"/api/v1/meta", nil)
	if err != nil {
		return false
	}
	r.Header.Set("Authorization", "Bearer "+desc.Token)
	response, err := client().Do(r)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(body) > 8192 {
		return false
	}
	var envelope struct {
		APIVersion int    `json:"apiVersion"`
		RequestID  string `json:"requestId"`
		Data       struct {
			OwnerInstanceID  string `json:"ownerInstanceId"`
			APIVersion       int    `json:"apiVersion"`
			AppVersion       string `json:"appVersion"`
			Scope            string `json:"scope"`
			ManagedExecution string `json:"managedExecution"`
		} `json:"data"`
	}
	return json.Unmarshal(body, &envelope, json.RejectUnknownMembers(true)) == nil && envelope.APIVersion == 1 && envelope.Data.APIVersion == 1 && envelope.Data.OwnerInstanceID == desc.OwnerInstanceID && envelope.Data.AppVersion == domain.AppVersion && envelope.Data.Scope == "workspace_foundation" && envelope.Data.ManagedExecution == "not_implemented" && domain.ValidID(envelope.RequestID)
}

func forward(ctx context.Context, desc localstate.Descriptor, c command) domain.Result {
	uncertain := func() domain.Result {
		if c.requestID != "" {
			return domain.Failure(c.requestID, domain.Fail("write_uncertain", "The owner acknowledgement is uncertain. Retry the same command with the same --request-id and input."))
		}
		return ownerFailure(c.requestID)
	}
	r, err := http.NewRequestWithContext(ctx, c.method, desc.Origin+"/api/v1"+c.route+c.queryString(), bytes.NewReader(c.params.Body))
	if err != nil {
		return uncertain()
	}
	r.Header.Set("Authorization", "Bearer "+desc.Token)
	r.Header.Set("Origin", desc.Origin)
	if c.method != "GET" {
		r.Header.Set("Content-Type", "application/json")
	}
	response, err := client().Do(r)
	if err != nil {
		return uncertain()
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024*1024+1))
	if err != nil || len(body) > 64*1024*1024 {
		return uncertain()
	}
	var envelope domain.Envelope
	if json.Unmarshal(body, &envelope, json.RejectUnknownMembers(true)) != nil || envelope.APIVersion != 1 || !domain.ValidID(envelope.RequestID) || (envelope.Data == nil) == (envelope.Error == nil) {
		return uncertain()
	}
	if c.requestID != "" && envelope.RequestID != c.requestID {
		return uncertain()
	}
	if envelope.Error != nil && response.StatusCode != domain.Status(envelope.Error.Code) {
		return uncertain()
	}
	if envelope.Error == nil && (response.StatusCode != 200 && response.StatusCode != 201) {
		return uncertain()
	}
	if envelope.Error != nil || c.requestID == "" {
		if envelope.Receipt != nil {
			return uncertain()
		}
	} else {
		if envelope.Receipt == nil || envelope.Receipt.RequestID != c.requestID || envelope.Receipt.Operation != c.op {
			return uncertain()
		}
		timestamp, err := time.Parse("2006-01-02T15:04:05.000Z", envelope.Receipt.CommittedAt)
		if err != nil || timestamp.UTC().Format("2006-01-02T15:04:05.000Z") != envelope.Receipt.CommittedAt {
			return uncertain()
		}
	}
	return domain.Result{Status: response.StatusCode, Envelope: envelope}
}

func showUI(c command, desc localstate.Descriptor, stdout, stderr io.Writer) int {
	url := desc.Origin + "/#token=" + desc.Token
	result := domain.Success(200, domain.NewID(), map[string]any{"url": url, "ownerInstanceId": desc.OwnerInstanceID}, nil)
	code := emit(stdout, stderr, c.json, result)
	if !c.noOpen {
		if err := openURL(url); err != nil {
			_, _ = fmt.Fprintln(stderr, "Council is running. Open the private launch URL shown above in your browser.")
		}
	}
	return code
}

func openURL(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	case "darwin":
		command = exec.Command("open", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

func emit(stdout, stderr io.Writer, asJSON bool, result domain.Result) int {
	if asJSON {
		encoded, err := json.Marshal(result.Envelope)
		if err != nil {
			return 1
		}
		if _, err := stdout.Write(append(encoded, '\n')); err != nil {
			return 1
		}
	} else {
		destination := stdout
		if result.Envelope.Error != nil {
			destination = stderr
		}
		if _, err := io.WriteString(destination, human(result)); err != nil {
			return 1
		}
	}
	if result.Envelope.Error != nil {
		return exitCode(result.Envelope.Error.Code)
	}
	return 0
}
