package cli

import (
	"io"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const recoveryHelp = `
Pause and recovery:
  council tasks pause ID --task-version V --expected-revision R [--request-id ID]
  council graphs pause ID --graph-version V --graph-control-revision R
    --budget-authority-epoch E --window-number W --node-authorities-file PATH|- [--request-id ID]
  council checkpoints get ID
  council checkpoints resume ID --checkpoint-revision R --scope-file PATH|- [--request-id ID]
  council incidents get ID
  council incidents reconcile ID --incident-revision R [--request-id ID]

Scope files contain the complete exact task, member or graph authority object shown by Council.
Pause drains admitted work; it does not suspend a process. Resume is explicit. Reconciliation
checks retained owner evidence, never a user assertion. Exact request bodies are retained privately;
retry the same command, input and request ID when an acknowledgement is uncertain.
`

func addRecoveryFlags(values map[string]bool) {
	for _, key := range []string{"checkpoint-revision", "incident-revision", "scope-file"} {
		values[key] = true
	}
}

func recoveryCommand(name string) bool {
	switch name {
	case "tasks pause", "graphs pause", "checkpoints get", "checkpoints resume", "incidents get", "incidents reconcile":
		return true
	}
	return false
}

func recoveryMutation(op string) bool {
	switch op {
	case "tasks.pause", "graphs.pause", "checkpoints.resume", "incidents.reconcile":
		return true
	}
	return false
}

func parseRecovery(c command, pos []string, flags map[string]string, stdin io.Reader) (command, *domain.Error) {
	if len(pos) != 3 || !domain.ValidID(pos[2]) {
		return c, domain.Invalid("exact recovery resource identity")
	}
	c.params.ID = pos[2]
	c.method = "GET"
	allowed := map[string]bool{"json": true, "api-version": true}
	allow := func(keys ...string) {
		for _, key := range keys {
			allowed[key] = true
		}
	}
	switch c.name {
	case "tasks pause":
		c.op, c.method, c.route = "tasks.pause", "POST", "/tasks/"+pos[2]+"/pause"
		allow("request-id", "task-version", "expected-revision")
	case "graphs pause":
		c.op, c.method, c.route = "graphs.pause", "POST", "/graphs/"+pos[2]+"/pause"
		allow("request-id", "graph-version", "graph-control-revision", "budget-authority-epoch", "window-number", "node-authorities-file")
	case "checkpoints get":
		c.op, c.route = "checkpoints.get", "/checkpoints/"+pos[2]
	case "checkpoints resume":
		c.op, c.method, c.route = "checkpoints.resume", "POST", "/checkpoints/"+pos[2]+"/resume"
		allow("request-id", "checkpoint-revision", "scope-file")
	case "incidents get":
		c.op, c.route = "incidents.get", "/incidents/"+pos[2]
	case "incidents reconcile":
		c.op, c.method, c.route = "incidents.reconcile", "POST", "/incidents/"+pos[2]+"/reconciliations"
		allow("request-id", "incident-revision")
	default:
		return c, domain.Invalid("documented recovery command")
	}
	for key := range flags {
		if !allowed[key] {
			return c, domain.Invalid("options supported by this recovery command")
		}
	}
	if c.method == "GET" {
		return c, nil
	}
	var err *domain.Error
	c.requestID, err = mutationRequestID(flags)
	if err != nil {
		return c, err
	}
	switch c.op {
	case "tasks.pause":
		in := domain.PauseTaskInput{RequestID: c.requestID, TaskVersion: flags["task-version"], ExpectedTaskRevision: flags["expected-revision"]}
		if err = domain.ValidatePauseTaskInput(in); err != nil {
			return c, err
		}
		return encodeExecution(c, in)
	case "graphs.pause":
		if flags["node-authorities-file"] == "" {
			return c, domain.Invalid("exact graph node authorities file")
		}
		raw, readErr := readGraphFile(flags["node-authorities-file"], stdin)
		if readErr != nil {
			return c, readErr
		}
		authorities, parseErr := domain.StrictDecodeGraphNodeAuthorities(raw)
		if parseErr != nil {
			return c, parseErr
		}
		in := domain.PauseGraphInput{RequestID: c.requestID, GraphVersion: flags["graph-version"], ExpectedGraphControlRevision: flags["graph-control-revision"], ExpectedBudgetAuthorityEpoch: flags["budget-authority-epoch"], ExpectedWindowNumber: flags["window-number"], NodeAuthorities: authorities}
		if err = domain.ValidatePauseGraphInput(in); err != nil {
			return c, err
		}
		return encodeExecution(c, in)
	case "checkpoints.resume":
		if flags["scope-file"] == "" {
			return c, domain.Invalid("exact checkpoint scope file")
		}
		raw, readErr := readGraphFile(flags["scope-file"], stdin)
		if readErr != nil {
			return c, readErr
		}
		var scope domain.RecoveryScope
		if err = domain.StrictDecode(raw, &scope); err != nil {
			return c, err
		}
		in := domain.ResumeCheckpointInput{RequestID: c.requestID, ExpectedCheckpointRevision: flags["checkpoint-revision"], Scope: scope}
		if err = domain.ValidateResumeCheckpointInput(in); err != nil {
			return c, err
		}
		return encodeExecution(c, in)
	case "incidents.reconcile":
		in := domain.ReconciliationInput{RequestID: c.requestID, ExpectedIncidentRevision: flags["incident-revision"], Action: "verify_and_reconcile"}
		if err = domain.ValidateReconciliationInput(in); err != nil {
			return c, err
		}
		return encodeExecution(c, in)
	}
	return c, domain.Invalid("recovery mutation")
}
