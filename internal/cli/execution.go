package cli

import (
	json "encoding/json/v2"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const executionHelp = `
Managed execution:
  council tasks start --project ID (--task TEXT | --task-file PATH|-)
  council tasks list [--project ID] | get ID | version ID --version V
  council tasks guidance ID --expected-revision R (--guidance TEXT | --guidance-file PATH|-)
  council tasks run ID --expected-revision R --task-version V
  council tasks attempts ID [--limit N] [--cursor CURSOR]
  council attempts get ID | stop ID | wait ID [--after-revision R] [--timeout-ms N]
  council attempts events ID | artifacts ID [--limit N] [--cursor CURSOR]
  council artifacts get ID | read ID [--offset N] [--limit N] [--through N]
  council tasks accept-response ID --expected-revision R --task-version V
    --attempt ID --response ID --response-sha256 HASH --reviewed
  council service stop [--owner-instance ID] [--request-id ID]

Start/run accept --fake [--fake-scenario response|candidate|partial_failure|slow|
late_success|owned_descendant|cleanup_unknown]. Fake work is explicitly labelled.
Start accepts --source-draft ID --source-draft-revision R, or --expected-policy-revision R.
Start/guidance accept --model SELECTOR, --harness ID, --account ID, --payment-path native|paid_api,
--policy supervised, --deliverable candidate|response_only, --conditions-file PATH|-,
--require-evidence VALUE (unique, repeatable), --final-only and --timeout-seconds 1..86400.
Guidance also accepts --use-current-policy and --clear-policy-override. Saving guidance does not run it.
Admission and wait success acknowledge that operation; inspect the retained attempt outcome.
Observer timeout or disconnection leaves work under its continuing owner.
`

func addExecutionFlags(bools, values map[string]bool) {
	for _, key := range []string{"fake", "final-only", "clear-policy-override", "reviewed"} {
		bools[key] = true
	}
	for _, key := range []string{"source-draft", "source-draft-revision", "expected-policy-revision", "model", "harness", "account", "payment-path", "policy", "deliverable", "conditions-file", "require-evidence", "timeout-seconds", "fake-scenario", "guidance", "guidance-file", "task-version", "after-revision", "timeout-ms", "offset", "through", "attempt", "response", "response-sha256", "owner-instance"} {
		values[key] = true
	}
}
func executionCommand(name string) bool {
	return strings.HasPrefix(name, "tasks ") || strings.HasPrefix(name, "attempts ") || strings.HasPrefix(name, "artifacts ") || name == "service stop"
}
func boundedInteger(value string, min, max int) (int, *domain.Error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max || strconv.Itoa(n) != value {
		return 0, domain.Invalid("canonical integer within the documented range")
	}
	return n, nil
}
func mutationRequestID(flags map[string]string) (string, *domain.Error) {
	id, ok := flags["request-id"]
	if !ok {
		id = domain.NewID()
	}
	if !domain.ValidID(id) {
		return "", domain.Invalid("request identity")
	}
	return id, nil
}
func encodeExecution(c command, body any) (command, *domain.Error) {
	data, err := json.Marshal(body)
	if err != nil {
		return c, domain.Invalid("lossless Unicode request")
	}
	c.params.Body = data
	return c, nil
}

func readExecutionFile(path string, stdin io.Reader) (string, *domain.Error) {
	reader := stdin
	var file *os.File
	if path != "-" {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return "", domain.Fail("prerequisite_missing", "Select an accessible ordinary UTF-8 text file, or use - for stdin.")
		}
		file, err = os.Open(path)
		if err != nil {
			return "", domain.Fail("prerequisite_missing", "Cannot read the selected text file. Check its path and access.")
		}
		defer file.Close()
		reader = file
	}
	if reader == nil {
		return "", domain.Invalid("text input")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, domain.MaxTextBytes+1))
	if err != nil {
		return "", domain.Invalid("readable text input")
	}
	if len(raw) > domain.MaxTextBytes {
		return "", domain.Fail("input_too_large", "Text exceeds 4 MiB. Reduce it explicitly and submit again.")
	}
	if !utf8.Valid(raw) {
		return "", domain.Fail("invalid_encoding", "Supply valid UTF-8 text without replacement decoding.")
	}
	return string(raw), nil
}
func executionText(flags map[string]string, textKey, fileKey string, stdin io.Reader, allowEmpty bool) (string, *domain.Error) {
	text, hasText := flags[textKey]
	path, hasFile := flags[fileKey]
	if hasText == hasFile {
		return "", domain.Invalid("exactly one of --" + textKey + " or --" + fileKey)
	}
	if hasFile {
		var err *domain.Error
		text, err = readExecutionFile(path, stdin)
		if err != nil {
			return "", err
		}
	}
	if allowEmpty {
		return text, domain.ValidateAdditionalText(text)
	}
	return text, domain.ValidateText(text)
}
func versionOptions(flags map[string]string, repeated map[string][]string, stdin io.Reader) (*domain.ExecutionPins, *domain.AcceptanceInput, *int, *domain.Error) {
	var pins *domain.ExecutionPins
	for _, key := range []string{"model", "harness", "account", "payment-path"} {
		if _, ok := flags[key]; ok {
			pins = &domain.ExecutionPins{Model: flags["model"], HarnessID: flags["harness"], AccountID: flags["account"], PaymentPath: flags["payment-path"]}
			break
		}
	}
	if err := domain.ValidatePins(pins); err != nil {
		return nil, nil, nil, err
	}
	var acceptance *domain.AcceptanceInput
	for _, key := range []string{"deliverable", "conditions-file", "require-evidence", "final-only"} {
		if _, ok := flags[key]; ok {
			acceptance = &domain.AcceptanceInput{Deliverable: flags["deliverable"], RequiredEvidence: repeated["require-evidence"]}
			break
		}
	}
	if acceptance != nil {
		if path, ok := flags["conditions-file"]; ok {
			text, err := readExecutionFile(path, stdin)
			if err != nil {
				return nil, nil, nil, err
			}
			acceptance.ConditionsText = text
		}
		if flags["final-only"] == "true" {
			value := false
			acceptance.AllowCandidatePreparation = &value
		}
		if _, err := domain.ResolveAcceptance(acceptance); err != nil {
			return nil, nil, nil, err
		}
	}
	var timeout *int
	if value, ok := flags["timeout-seconds"]; ok {
		n, err := boundedInteger(value, 1, 86400)
		if err != nil {
			return nil, nil, nil, err
		}
		timeout = &n
	}
	if value, ok := flags["policy"]; ok && value != "supervised" {
		return nil, nil, nil, domain.Invalid("narrower supervised policy")
	}
	return pins, acceptance, timeout, nil
}
func validatePageFlags(c *command, flags map[string]string) *domain.Error {
	if value, ok := flags["limit"]; ok {
		if _, err := domain.ParseLimit(value); err != nil {
			return err
		}
		c.params.Query["limit"] = value
	}
	if value, ok := flags["cursor"]; ok {
		if len(value) > 2048 {
			return domain.Fail("invalid_cursor", "The cursor exceeds its documented bound.")
		}
		for _, r := range value {
			if r > 127 {
				return domain.Fail("invalid_cursor", "Use an unchanged cursor returned by this operation.")
			}
		}
		c.params.Query["cursor"] = value
	}
	return nil
}

func parseExecution(c command, pos []string, flags map[string]string, repeated map[string][]string, stdin io.Reader) (command, *domain.Error) {
	allowed := map[string]bool{"json": true, "api-version": true}
	allow := func(keys ...string) {
		for _, key := range keys {
			allowed[key] = true
		}
	}
	versionKeys := []string{"model", "harness", "account", "payment-path", "policy", "deliverable", "conditions-file", "require-evidence", "final-only", "timeout-seconds"}
	needsID := true
	c.method = "GET"
	switch c.name {
	case "tasks start":
		needsID = false
		c.op = "tasks.start"
		c.method = "POST"
		c.route = "/tasks"
		allow("request-id", "project", "task", "task-file", "source-draft", "source-draft-revision", "expected-policy-revision", "fake", "fake-scenario")
		allow(versionKeys...)
	case "tasks list":
		needsID = false
		c.op = "tasks.list"
		c.route = "/tasks"
		allow("project", "limit", "cursor")
	case "tasks get":
		c.op = "tasks.get"
	case "tasks version":
		c.op = "tasks.version.get"
		allow("version")
	case "tasks guidance":
		c.op = "tasks.guidance"
		c.method = "PUT"
		allow("request-id", "expected-revision", "guidance", "guidance-file", "use-current-policy", "clear-policy-override")
		allow(versionKeys...)
	case "tasks run":
		c.op = "tasks.attempts.start"
		c.method = "POST"
		allow("request-id", "expected-revision", "task-version", "fake", "fake-scenario")
	case "tasks attempts":
		c.op = "tasks.attempts.list"
		allow("limit", "cursor")
	case "tasks accept-response":
		c.op = "tasks.response.accept"
		c.method = "POST"
		allow("request-id", "expected-revision", "task-version", "attempt", "response", "response-sha256", "reviewed")
	case "attempts get":
		c.op = "attempts.get"
	case "attempts stop":
		c.op = "attempts.stop"
		c.method = "POST"
		allow("request-id")
	case "attempts wait":
		c.op = "attempts.wait"
		allow("after-revision", "timeout-ms")
	case "attempts events":
		c.op = "attempts.events.list"
		allow("limit", "cursor")
	case "attempts artifacts":
		c.op = "attempts.artifacts.list"
		allow("limit", "cursor")
	case "artifacts get":
		c.op = "artifacts.get"
	case "artifacts read":
		c.op = "artifacts.bytes"
		allow("offset", "limit", "through")
	case "service stop":
		needsID = false
		c.op = "service.stop"
		c.method = "POST"
		c.route = "/service/stop"
		allow("request-id", "owner-instance")
	default:
		return c, domain.Invalid("documented execution command")
	}
	for key := range flags {
		if !allowed[key] {
			return c, domain.Invalid("options supported by this execution command")
		}
	}
	if needsID {
		if len(pos) != 3 || !domain.ValidID(pos[2]) {
			return c, domain.Invalid("resource identity")
		}
		c.params.ID = pos[2]
		c.route = "/" + pos[0] + "/" + pos[2]
	} else if len(pos) != 2 {
		return c, domain.Invalid("execution command arguments")
	}
	stdinSources := 0
	for _, key := range []string{"task-file", "guidance-file", "conditions-file"} {
		if flags[key] == "-" {
			stdinSources++
		}
	}
	if stdinSources > 1 {
		return c, domain.Invalid("one stdin text source")
	}
	if c.method != "GET" {
		var err *domain.Error
		c.requestID, err = mutationRequestID(flags)
		if err != nil {
			return c, err
		}
	}
	if strings.HasSuffix(c.op, ".list") {
		if err := validatePageFlags(&c, flags); err != nil {
			return c, err
		}
	}
	switch c.name {
	case "tasks start":
		if !domain.ValidID(flags["project"]) {
			return c, domain.Invalid("project identity")
		}
		text, err := executionText(flags, "task", "task-file", stdin, false)
		if err != nil {
			return c, err
		}
		pins, acceptance, timeout, err := versionOptions(flags, repeated, stdin)
		if err != nil {
			return c, err
		}
		in := domain.StartTaskInput{RequestID: c.requestID, ProjectID: flags["project"], OriginalText: text, ExpectedPolicyRevision: flags["expected-policy-revision"], Pins: pins, Acceptance: acceptance, PolicyOverride: flags["policy"], TimeoutSeconds: timeout, FakeScenario: flags["fake-scenario"]}
		_, draft := flags["source-draft"]
		_, revision := flags["source-draft-revision"]
		if draft != revision {
			return c, domain.Invalid("paired source draft and revision")
		}
		if draft {
			in.SourceDraftRef = &domain.DraftRef{DraftID: flags["source-draft"], Revision: flags["source-draft-revision"]}
		}
		if flags["fake"] == "true" {
			in.ExecutionMode = "fake"
		}
		if err := domain.ValidateStartTask(in); err != nil {
			return c, err
		}
		return encodeExecution(c, in)
	case "tasks list":
		if value, ok := flags["project"]; ok {
			if !domain.ValidID(value) {
				return c, domain.Invalid("project identity")
			}
			c.params.Query["projectId"] = value
		}
	case "tasks version":
		version := flags["version"]
		if _, err := domain.ParseRevision(version); err != nil {
			return c, err
		}
		c.params.ID += "/" + version
		c.route += "/versions/" + version
	case "tasks guidance":
		text, err := executionText(flags, "guidance", "guidance-file", stdin, true)
		if err != nil {
			return c, err
		}
		pins, acceptance, timeout, err := versionOptions(flags, repeated, stdin)
		if err != nil {
			return c, err
		}
		in := domain.GuidanceInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], GuidanceText: text, UseCurrentPolicy: flags["use-current-policy"] == "true", ClearPolicyOverride: flags["clear-policy-override"] == "true", PolicyOverride: flags["policy"], Pins: pins, Acceptance: acceptance, TimeoutSeconds: timeout}
		if err := domain.ValidateGuidance(in); err != nil {
			return c, err
		}
		c.route += "/guidance"
		return encodeExecution(c, in)
	case "tasks run":
		in := domain.StartAttemptInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], TaskVersion: flags["task-version"], FakeScenario: flags["fake-scenario"]}
		if flags["fake"] == "true" {
			in.ExecutionMode = "fake"
		}
		if err := domain.ValidateStartAttempt(in); err != nil {
			return c, err
		}
		c.route += "/attempts"
		return encodeExecution(c, in)
	case "tasks attempts":
		c.route += "/attempts"
	case "tasks accept-response":
		in := domain.ResponseAcceptanceInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], TaskVersion: flags["task-version"], AttemptID: flags["attempt"], ResponseArtifactID: flags["response"], ResponseSHA256: flags["response-sha256"]}
		if flags["reviewed"] == "true" {
			in.Confirmation = "reviewed_response_and_conditions"
		}
		if err := domain.ValidateResponseAcceptance(in); err != nil {
			return c, err
		}
		c.route += "/response-acceptance"
		return encodeExecution(c, in)
	case "attempts stop":
		c.route += "/stop"
		return encodeExecution(c, map[string]string{"requestId": c.requestID})
	case "attempts wait":
		c.route += "/wait"
		if value, ok := flags["after-revision"]; ok {
			if _, err := domain.ParseCount(value); err != nil {
				return c, err
			}
			c.params.Query["afterRevision"] = value
		}
		if value, ok := flags["timeout-ms"]; ok {
			if _, err := boundedInteger(value, 0, 30000); err != nil {
				return c, err
			}
			c.params.Query["timeoutMs"] = value
		}
	case "attempts events":
		c.route += "/events"
	case "attempts artifacts":
		c.route += "/artifacts"
	case "artifacts read":
		c.route += "/bytes"
		for _, key := range []string{"offset", "through"} {
			if value, ok := flags[key]; ok {
				if _, err := domain.ParseCount(value); err != nil {
					return c, err
				}
				c.params.Query[key] = value
			}
		}
		if value, ok := flags["limit"]; ok {
			if _, err := boundedInteger(value, 1, 65536); err != nil {
				return c, err
			}
			c.params.Query["limit"] = value
		}
		if flags["through"] != "" {
			through, _ := domain.ParseCount(flags["through"])
			offset := int64(0)
			if flags["offset"] != "" {
				offset, _ = domain.ParseCount(flags["offset"])
			}
			if offset > through {
				return c, domain.Invalid("byte offset no greater than through")
			}
		}
	case "service stop":
		body := map[string]string{"requestId": c.requestID}
		if value, ok := flags["owner-instance"]; ok {
			if !domain.ValidID(value) {
				return c, domain.Invalid("owner instance identity")
			}
			body["ownerInstanceId"] = value
		}
		return encodeExecution(c, body)
	}
	return c, nil
}

// bindServiceOwner captures the verified current owner only when the caller did not
// explicitly retain an earlier owner target for exact idempotent receipt replay.
func bindServiceOwner(c command, ownerInstanceID string) (command, *domain.Error) {
	if c.op != "service.stop" {
		return c, nil
	}
	var body struct {
		RequestID       string `json:"requestId"`
		OwnerInstanceID string `json:"ownerInstanceId,omitempty"`
	}
	if err := domain.StrictDecode(c.params.Body, &body); err != nil {
		return c, err
	}
	if !domain.ValidID(body.RequestID) {
		return c, domain.Invalid("request identity")
	}
	if body.OwnerInstanceID == "" {
		body.OwnerInstanceID = ownerInstanceID
	}
	if !domain.ValidID(body.OwnerInstanceID) {
		return c, domain.Invalid("owner instance identity")
	}
	return encodeExecution(c, body)
}

// A CLI process cannot carry its in-memory captured target into another process.
// Report the complete non-secret retry intent instead of telling the caller to
// repeat a targetless Stop, which could bind a replacement owner.
func stopRetryMessage(c command) string {
	if c.op != "service.stop" {
		return ""
	}
	var in struct {
		RequestID       string `json:"requestId"`
		OwnerInstanceID string `json:"ownerInstanceId"`
	}
	if domain.StrictDecode(c.params.Body, &in) != nil || !domain.ValidID(in.RequestID) || !domain.ValidID(in.OwnerInstanceID) {
		return ""
	}
	return "The owner Stop acknowledgement is uncertain. Preserve its captured target. Retry exactly: council service stop --owner-instance " + in.OwnerInstanceID + " --request-id " + in.RequestID
}

func executionHumanPrefix(envelope domain.Envelope) string {
	if receipt := envelope.Receipt; receipt != nil && (receipt.Operation == "tasks.start" || receipt.Operation == "tasks.attempts.start") {
		return "Admitted. Inspect the attempt for progress and its retained outcome.\n"
	}
	data, ok := envelope.Data.(map[string]any)
	if !ok {
		return ""
	}
	if _, ok := data["bytesBase64"]; !ok {
		return ""
	}
	offset, a := data["offset"].(string)
	next, b := data["nextOffset"].(string)
	through, c := data["through"].(string)
	if !a || !b || !c {
		return ""
	}
	for _, value := range []string{offset, next, through} {
		if _, err := domain.ParseCount(value); err != nil {
			return ""
		}
	}
	return fmt.Sprintf("Artifact byte chunk: offset %s to %s, snapshot bound %s.\n", offset, next, through)
}
