package engine

import (
	"context"
	"encoding/base64"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

func decodeChunk(s string) ([]byte, error) { return base64.StdEncoding.Strict().DecodeString(s) }

func (e *Engine) Meta(ctx context.Context) (map[string]any, error) {
	if !e.beginOwnerOperation() {
		return nil, ownerClosing()
	}
	defer e.endOwnerOperation()
	g, err := e.db.Gate(ctx)
	if err != nil {
		return nil, err
	}
	e.control.Lock()
	fenced := e.fenced
	e.control.Unlock()
	if fenced {
		g.State = "fenced"
	}
	var incident *domain.ExecutionIncident
	if g.IncidentID != nil {
		found, err := e.db.Incident(ctx, *g.IncidentID)
		if err != nil {
			return nil, err
		}
		incident = &found
	}
	claims, err := e.db.GraphClaims(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"scope": "managed_execution", "managedExecution": "native_codex_claude", "capabilities": []string{"workspace_foundation", "managed_execution", "economical_routing", "verified_delivery", "task_graphs", "safe_recovery"}, "executionContract": domain.ExecutionContract, "workflowContract": domain.WorkflowContract, "graphContract": domain.GraphContract, "recoveryContract": "007-v1", "maintenanceContract": "owner-stop/1", "admission": map[string]any{"state": g.State, "incidentId": g.IncidentID, "incident": incident}, "capacity": managedCapacity(claims)}, nil
}

func managedCapacity(claims []domain.GraphClaimView) map[string]any {
	var sole *string
	if len(claims) == 1 {
		sole = &claims[0].AttemptID
	}
	return map[string]any{"resourceId": domain.ManagedNativeResourceID, "limit": 2, "activeAttemptId": sole, "occupiedSlots": strconv.Itoa(len(claims)), "activeClaims": claims}
}

func cancellationScope() string {
	if runtime.GOOS == "windows" {
		return "windows_job"
	}
	return "unix_process_group"
}
func capabilities() map[string]any {
	connection := func(id, harness, contract string, minimum, documented *string, source string, responseLimit int64) map[string]any {
		return map[string]any{"connectionId": id, "harnessId": harness, "interface": "native_exec", "nativeContract": contract, "minimumVersion": minimum, "documentedVersion": documented, "versionSource": source, "supportedPolicyModes": []string{"autonomous"}, "modelSelection": "explicit_selector", "accountSelection": "unsupported", "sessionContinuity": "unsupported", "sessionPersistence": "disabled_for_managed_attempt", "cancellationScope": cancellationScope(), "liveQualification": "unqualified", "fakeMode": true, "limits": map[string]any{"defaultTimeoutSeconds": 1800, "maxTimeoutSeconds": 86400, "promptBytes": domain.MaxPromptBytes, "stdoutBytes": evidence.StdoutLimit, "stderrBytes": evidence.StderrLimit, "jsonlLineBytes": evidence.LineLimit, "eventBytes": domain.MaxEventBytes, "chunkBytes": evidence.ChunkLimit, "finalResponseBytesUpperBound": responseLimit}}
	}
	minimum, documented := "2.1.259", "2.1.318"
	return map[string]any{"resourceId": domain.ManagedNativeResourceID, "limit": 2, "defaultHarnessId": "codex", "routingContract": "004-v1", "workflowContract": domain.WorkflowContract, "graphContract": domain.GraphContract, "supportedContinuationCategories": []string{"pre_launch_ineligible"}, "recognizedContinuationCategories": []string{"pre_launch_ineligible", "terminal_transient"}, "actualAccountSelection": "unsupported", "paidAPIExecution": "unsupported", "connections": []any{
		connection("codex-native", "codex", adapters.CodexContract, nil, nil, "accepted_002_contract", evidence.StdoutLimit),
		connection("claude-native", "claude-code", adapters.ClaudeContract, &minimum, &documented, "official_documentation_2026-11-06", evidence.LineLimit),
	}}
}

func (e *Engine) Read(ctx context.Context, op, id string, q map[string]string) domain.Result {
	if !e.beginOwnerOperation() {
		return e.failure(domain.NewID(), ownerClosing())
	}
	defer e.endOwnerOperation()
	if strings.HasPrefix(op, "graphs.") {
		return e.readGraph(ctx, op, id, q)
	}
	if workflowReadOperation(op) {
		return e.readWorkflow(ctx, op, id, q)
	}
	if routingReadOperation(op) {
		return e.readRouting(ctx, op, id, q)
	}
	requestID := domain.NewID()
	fail := func(err error) domain.Result { return e.failure(requestID, err) }
	keys := []string{}
	switch op {
	case "tasks.list":
		keys = []string{"projectId", "limit", "cursor"}
	case "tasks.attempts.list", "attempts.events.list", "attempts.artifacts.list":
		keys = []string{"limit", "cursor"}
	case "attempts.wait":
		keys = []string{"afterRevision", "timeoutMs"}
	case "artifacts.bytes":
		keys = []string{"offset", "through", "limit"}
	}
	if err := domain.ValidateQuery(q, keys...); err != nil {
		return fail(err)
	}
	for key, value := range q {
		if value == "" {
			if key == "cursor" {
				return fail(domain.Fail("invalid_cursor", "Supply the complete cursor or omit it to start a list."))
			}
			return fail(domain.Invalid("nonempty query values"))
		}
	}
	if op != "tasks.list" && op != "execution.capabilities" && op != "tasks.version.get" && !domain.ValidID(id) {
		return fail(domain.Invalid("resource identity"))
	}
	var data any
	var err error
	limit := 30
	if strings.HasSuffix(op, ".list") {
		parsed, validationErr := domain.ParseLimit(q["limit"])
		if validationErr != nil {
			return fail(validationErr)
		}
		limit = parsed
	}
	switch op {
	case "execution.capabilities":
		data = capabilities()
	case "checkpoints.get":
		data, err = e.db.Checkpoint(ctx, id)
	case "incidents.get":
		data, err = e.db.RecoveryIncident(ctx, id)
	case "tasks.list":
		if q["projectId"] != "" && !domain.ValidID(q["projectId"]) {
			return fail(domain.Invalid("project identity"))
		}
		data, err = e.db.Tasks(ctx, q["projectId"], limit, q["cursor"])
	case "tasks.get":
		detail, readErr := e.db.Task(ctx, id)
		err = readErr
		if err == nil {
			detail.Checkpoints, err = e.db.ActiveRecoveryCheckpoints(ctx, id, "")
		}
		if err == nil && detail.LatestAttempt != nil {
			enriched, enrichErr := e.inspectAttempt(ctx, *detail.LatestAttempt)
			err = enrichErr
			detail.LatestAttempt = &enriched
		}
		data = detail
	case "tasks.version.get":
		parts := strings.Split(id, "/")
		if len(parts) != 2 || !domain.ValidID(parts[0]) {
			return fail(domain.Invalid("task identity and version"))
		}
		if _, e := domain.ParseRevision(parts[1]); e != nil {
			return fail(e)
		}
		data, err = e.db.TaskVersion(ctx, parts[0], parts[1])
	case "tasks.attempts.list":
		data, err = e.db.Attempts(ctx, id, limit, q["cursor"])
	case "attempts.get":
		attempt, readErr := e.db.Attempt(ctx, id)
		err = readErr
		if err == nil {
			attempt, err = e.inspectAttempt(ctx, attempt)
		}
		data = attempt
	case "attempts.events.list":
		data, err = e.db.Events(ctx, id, limit, q["cursor"])
	case "attempts.artifacts.list":
		data, err = e.db.Artifacts(ctx, id, limit, q["cursor"])
	case "artifacts.get":
		r, e := e.db.Artifact(ctx, id)
		err = e
		data = r.ArtifactMetadata
	case "artifacts.bytes":
		offset := q["offset"]
		if offset == "" {
			offset = "0"
		}
		chunkLimit := evidence.ChunkLimit
		if q["limit"] != "" {
			n, e := strconv.Atoi(q["limit"])
			if e != nil || n < 1 || n > evidence.ChunkLimit || strconv.Itoa(n) != q["limit"] {
				return fail(domain.Invalid("byte limit from 1 through 65536"))
			}
			chunkLimit = n
		}
		var through *string
		if v, ok := q["through"]; ok {
			through = &v
		}
		record, recordErr := e.db.Artifact(ctx, id)
		if recordErr != nil {
			return fail(recordErr)
		}
		from, validationErr := domain.ParseCount(offset)
		if validationErr != nil {
			return fail(validationErr)
		}
		end, validationErr := domain.ParseCount(record.ByteLength)
		if validationErr != nil {
			return fail(validationErr)
		}
		if through != nil {
			end, validationErr = domain.ParseCount(*through)
			if validationErr != nil {
				return fail(validationErr)
			}
		}
		retained, _ := domain.ParseCount(record.ByteLength)
		if from > end || end > retained {
			return fail(domain.Invalid("a byte range inside the durable retained prefix"))
		}
		data, err = e.evidence.Read(ctx, id, evidence.ReadOptions{Offset: offset, Through: through, Limit: chunkLimit})
		if err != nil {
			if errors.Is(err, evidence.ErrCorrupt) {
				e.fence(record.AttemptID, "evidence_corrupt", "Registered retained evidence failed identity or requested-range integrity verification. Dispatch remains fenced.")
			}
			return fail(domain.Fail("evidence_unavailable", "The requested evidence prefix is unavailable or its identity changed. Preserve the retained evidence and inspect this attempt."))
		}
	case "attempts.wait":
		data, err = e.wait(ctx, id, q)
	default:
		return fail(domain.Fail("not_found", "The requested execution read does not exist."))
	}
	if err != nil {
		return fail(err)
	}
	return domain.Success(200, requestID, data, nil)
}

func (e *Engine) wait(ctx context.Context, id string, q map[string]string) (any, error) {
	after := q["afterRevision"]
	if after != "" {
		if _, err := domain.ParseCount(after); err != nil {
			return nil, err
		}
	}
	timeout := 30000
	if text, ok := q["timeoutMs"]; ok {
		n, err := strconv.Atoi(text)
		if err != nil || n < 0 || n > 30000 || strconv.Itoa(n) != text {
			return nil, domain.Invalid("timeoutMs from 0 through 30000")
		}
		timeout = n
	}
	timer := time.NewTimer(time.Duration(timeout) * time.Millisecond)
	defer timer.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		a, err := e.db.Attempt(ctx, id)
		if err != nil {
			return nil, err
		}
		afterCount := int64(0)
		if after != "" {
			afterCount, _ = domain.ParseCount(after)
		}
		currentCount, countErr := domain.ParseRevision(a.Revision)
		if countErr != nil {
			return nil, countErr
		}
		changed := currentCount > afterCount
		a, err = e.inspectAttempt(ctx, a)
		if err != nil {
			return nil, err
		}
		if changed || a.Phase == "settled" {
			return map[string]any{"attempt": a, "changed": changed, "timedOut": false}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return map[string]any{"attempt": a, "changed": false, "timedOut": true}, nil
		case <-tick.C:
		}
	}
}
