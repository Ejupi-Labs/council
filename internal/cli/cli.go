// Package cli provides the native Council entrypoint without an intervening shell.
package cli

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/owner"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

const help = `Council — managed local workspace

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
Private state: <user-home>/.council/v2/. Draft saving and explicit execution are separate.
` + executionHelp + routingHelp

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
				if retry := stopRetryMessage(c); retry != "" {
					failure.Message = retry
				}
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
	if requiresExecutionOwner(c.op) {
		var desc localstate.Descriptor
		var e error
		if c.op == "service.stop" {
			desc, e = localstate.ReadDescriptor(root)
			if e == nil && !verifyMaintenanceOwner(ctx, desc) {
				e = errors.New("incompatible owner")
			}
		} else {
			desc, e = owner.Ensure(ctx, root, verifyExecutionOwner)
		}
		if e != nil {
			return emit(stdout, stderr, c.json, ownerFailure(c.requestID))
		}
		if c.op == "service.stop" {
			var bindErr *domain.Error
			c, bindErr = bindServiceOwner(c, desc.OwnerInstanceID)
			if bindErr != nil {
				return emit(stdout, stderr, c.json, domain.Failure(requestIdentity(c), bindErr))
			}
		}
		result := forward(ctx, desc, c)
		if ctx.Err() != nil && c.requestID == "" {
			_ = emit(stdout, stderr, c.json, domain.Failure(requestIdentity(c), domain.Fail("observation_interrupted", "Observation ended; owned work continues. Inspect the attempt again.")))
			return 130
		}
		return emit(stdout, stderr, c.json, result)
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
		if c.op == "service.stop" {
			var bindErr *domain.Error
			c, bindErr = bindServiceOwner(c, desc.OwnerInstanceID)
			if bindErr != nil {
				return emit(stdout, stderr, c.json, domain.Failure(requestIdentity(c), bindErr))
			}
		}
		result := forward(ctx, desc, c)
		if ctx.Err() != nil && c.requestID == "" {
			_ = emit(stdout, stderr, c.json, domain.Failure(requestIdentity(c), domain.Fail("observation_interrupted", "Observation ended; owned work continues. Inspect the attempt again.")))
			return 130
		}
		return emit(stdout, stderr, c.json, result)
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
	return serveOwner(ctx, lock, db, &c, stdout, stderr)
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
	return domain.Failure(id, domain.Fail("owner_unavailable", "The private Council owner is unavailable or incompatible. Use council service stop for a recognized managed owner, or close its foreground process normally, then retry; do not remove its lock."))
}

func client() *http.Client {
	return &http.Client{Timeout: 35 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func verifyOwner(ctx context.Context, desc localstate.Descriptor) bool {
	metadata, ok := readOwnerMetadata(ctx, desc)
	return ok && ((metadata.Scope == "workspace_foundation" && metadata.ManagedExecution == "not_implemented" && (metadata.AppVersion == domain.AppVersion || metadata.AppVersion == "2.0.0-dev")) || executionMetadata(metadata))
}

func verifyExecutionOwner(ctx context.Context, desc localstate.Descriptor) bool {
	metadata, ok := readOwnerMetadata(ctx, desc)
	return ok && executionMetadata(metadata)
}

func forward(ctx context.Context, desc localstate.Descriptor, c command) domain.Result {
	uncertain := func() domain.Result {
		if c.requestID != "" {
			message := "The owner acknowledgement is uncertain. Retry the same command with the same --request-id and input."
			if retry := stopRetryMessage(c); retry != "" {
				message = retry
			}
			return domain.Failure(c.requestID, domain.Fail("write_uncertain", message))
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
	if envelope.Error == nil && (response.StatusCode != 200 && response.StatusCode != 201 && response.StatusCode != 202) {
		return uncertain()
	}
	if envelope.Error != nil || c.requestID == "" {
		if envelope.Receipt != nil {
			return uncertain()
		}
		if envelope.Error != nil && (envelope.Error.Code == "write_uncertain" || envelope.Error.Code == "storage_failed" || envelope.Error.Code == "internal_error") {
			if retry := stopRetryMessage(c); retry != "" {
				envelope.Error.Message = retry
			}
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
