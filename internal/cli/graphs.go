package cli

import (
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const graphHelp = `
Task graphs (one shared finite allowance):
  council graphs start [--project ID] --graph-file PATH|-
  council graphs list [--project ID] | get ID | definition ID
  council graphs wait ID [--after-revision R] [--timeout-ms N]
  council graphs budget ID | budget-ledger ID | inputs ID
  council graphs input ID --input INPUT | input-members ID --input INPUT
  council graphs dispatches ID [--input INPUT] [--node KEY]
  council graphs dispatch ID --dispatch DISPATCH
  council graphs advance ID --graph-version V --graph-control-revision R
    --budget-authority-epoch E --window-number W --node-authorities-file PATH|-
  council graphs stop ID --graph-control-revision R
  council graphs stop-node ID --node KEY --node-id NODE --graph-control-revision R --node-control-revision N
  council graphs guide ID --node KEY --node-id NODE --graph-control-revision R --node-control-revision N
    --task-version V --expected-revision R (--guidance TEXT | --guidance-file PATH|-)
  council graphs grant ID --graph-control-revision R --budget-authority-epoch E --window-number W
    --additional-admissions N [--window-seconds N]

Graph declarations are strict whole JSON objects, at most 2 MiB. Transport identity flags
only supply absent fields; conflicting file/flag identities reject. Reads create no work.
Control proof groups accept all flags or none. Omission captures current detail once and
retains the full request privately before sending. Repeatable --rebind-node KEY resolves only
that captured detail. Reuse the same request ID and unchanged input for exact retry; later
graph changes never refresh the captured authority. Observation does not imply completion.
`

func addGraphFlags(values map[string]bool) {
	for _, key := range []string{"graph-file", "graph-version", "graph-control-revision", "budget-authority-epoch", "window-number", "node-authorities-file", "node", "node-id", "node-control-revision", "input", "dispatch", "rebind-node"} {
		values[key] = true
	}
}

func graphCommand(name string) bool { return strings.HasPrefix(name, "graphs ") }

func readGraphFile(path string, stdin io.Reader) ([]byte, *domain.Error) {
	reader := stdin
	if path != "-" {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, domain.Fail("prerequisite_missing", "Select an accessible ordinary graph JSON file, or use - for stdin.")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, domain.Fail("prerequisite_missing", "Select an accessible ordinary graph JSON file, or use - for stdin.")
		}
		defer file.Close()
		info, err = file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return nil, domain.Fail("prerequisite_missing", "Select an accessible ordinary graph JSON file, or use - for stdin.")
		}
		reader = file
	}
	if reader == nil {
		return nil, domain.Invalid("whole graph JSON input")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, domain.MaxGraphDeclarationBytes+1))
	if err != nil {
		return nil, domain.Invalid("readable graph JSON input")
	}
	if len(raw) > domain.MaxGraphDeclarationBytes {
		return nil, domain.Fail("input_too_large", "Graph JSON exceeds 2 MiB. Reduce it explicitly and submit again.")
	}
	if !utf8.Valid(raw) {
		return nil, domain.Fail("invalid_encoding", "Supply valid UTF-8 graph JSON without replacement decoding.")
	}
	return raw, nil
}

func graphProofGroup(flags map[string]string, keys ...string) *domain.Error {
	provided := 0
	for _, key := range keys {
		if _, ok := flags[key]; ok {
			provided++
		}
	}
	if provided == 0 {
		return nil
	}
	if provided != len(keys) {
		return domain.Invalid("all graph proof flags or none")
	}
	return nil
}

func graphProofAuto(flags map[string]string, first string) bool {
	_, explicit := flags[first]
	return !explicit
}

func parseGraph(c command, pos []string, flags map[string]string, repeated map[string][]string, stdin io.Reader) (command, *domain.Error) {
	allowed := map[string]bool{"json": true, "api-version": true}
	allow := func(keys ...string) {
		for _, key := range keys {
			allowed[key] = true
		}
	}
	c.method, c.route = "GET", "/graphs"
	count := 3
	switch c.name {
	case "graphs start":
		count, c.op, c.method = 2, "graphs.start", "POST"
		allow("graph-file", "project", "request-id")
	case "graphs list":
		count, c.op = 2, "graphs.list"
		allow("project", "limit", "cursor")
	case "graphs get":
		c.op = "graphs.get"
	case "graphs definition":
		c.op = "graphs.definition.get"
	case "graphs wait":
		c.op = "graphs.wait"
		allow("after-revision", "timeout-ms")
	case "graphs budget":
		c.op = "graphs.budget.get"
	case "graphs budget-ledger":
		c.op = "graphs.budget.ledger"
		allow("limit", "cursor")
	case "graphs inputs":
		c.op = "graphs.inputs.list"
		allow("limit", "cursor")
	case "graphs input", "graphs input-members":
		c.op = "graphs.inputs.get"
		allow("input")
		if c.name == "graphs input-members" {
			c.op = "graphs.inputs.members"
			allow("limit", "cursor")
		}
	case "graphs dispatches":
		c.op = "graphs.dispatches.list"
		allow("input", "node", "limit", "cursor")
	case "graphs dispatch":
		c.op = "graphs.dispatches.get"
		allow("dispatch")
	case "graphs advance":
		c.op, c.method = "graphs.advance", "POST"
		allow("request-id", "graph-version", "graph-control-revision", "budget-authority-epoch", "window-number", "node-authorities-file", "rebind-node")
	case "graphs stop":
		c.op, c.method = "graphs.stop", "POST"
		allow("request-id", "graph-control-revision")
	case "graphs stop-node":
		c.op, c.method = "graphs.nodes.stop", "POST"
		allow("request-id", "graph-control-revision", "node", "node-id", "node-control-revision")
	case "graphs guide":
		c.op, c.method = "graphs.nodes.guidance", "POST"
		allow("request-id", "graph-control-revision", "node", "node-id", "node-control-revision", "expected-revision", "task-version", "guidance", "guidance-file", "use-current-policy", "clear-policy-override", "policy", "model", "harness", "account", "payment-path", "deliverable", "conditions-file", "require-evidence", "final-only", "timeout-seconds", "use-current-routing-policy")
		allow(taskRoutingFlags...)
		allow(workflowTaskFlags...)
		delete(allowed, "delivery")
		delete(allowed, "delivery-file")
	case "graphs grant":
		c.op, c.method = "graphs.budget.amend", "POST"
		allow("request-id", "graph-control-revision", "budget-authority-epoch", "window-number", "additional-admissions", "window-seconds")
	default:
		return c, domain.Invalid("documented graph command")
	}
	if len(pos) != count {
		return c, domain.Invalid("graph command arguments")
	}
	for key := range flags {
		if !allowed[key] {
			return c, domain.Invalid("options supported by this graph command")
		}
	}
	if count == 3 {
		if !domain.ValidID(pos[2]) {
			return c, domain.Invalid("graph identity")
		}
		c.params.ID = pos[2]
		c.route += "/" + pos[2]
	}
	if c.name == "graphs start" {
		path, ok := flags["graph-file"]
		if !ok {
			return c, domain.Invalid("one whole graph-file declaration")
		}
		raw, err := readGraphFile(path, stdin)
		if err != nil {
			return c, err
		}
		in, err := domain.StrictDecodeGraphStart(raw)
		if err != nil {
			return c, err
		}
		for key, destination := range map[string]*string{"project": &in.ProjectID, "request-id": &in.RequestID} {
			if value, ok := flags[key]; ok {
				if !domain.ValidID(value) || *destination != "" && *destination != value {
					return c, domain.Invalid("matching graph declaration and transport identities")
				}
				if *destination == "" {
					*destination = value
				}
			}
		}
		if in.RequestID == "" {
			in.RequestID = domain.NewID()
		}
		if err := domain.ValidateGraphStartInput(in); err != nil {
			return c, err
		}
		c.requestID = in.RequestID
		return encodeExecution(c, in)
	}
	if c.method == "GET" {
		return parseGraphRead(c, flags)
	}
	var err *domain.Error
	c.requestID, err = mutationRequestID(flags)
	if err != nil {
		return c, err
	}
	c.graphIntent = &graphIntentPlan{NodeKey: flags["node"], RebindKeys: []string{}}
	if _, ok := flags["rebind-node"]; ok {
		if err := uniqueRepeated(repeated["rebind-node"]); err != nil {
			return c, err
		}
		for _, key := range repeated["rebind-node"] {
			if !domain.GraphKey(key) {
				return c, domain.Invalid("rebind node key")
			}
		}
		c.graphIntent.RebindKeys = append(c.graphIntent.RebindKeys, repeated["rebind-node"]...)
	}
	return parseGraphMutation(c, flags, repeated, stdin)
}

func parseGraphRead(c command, flags map[string]string) (command, *domain.Error) {
	if err := validatePageFlags(&c, flags); err != nil {
		return c, err
	}
	if project, ok := flags["project"]; ok {
		if !domain.ValidID(project) {
			return c, domain.Invalid("project identity")
		}
		c.params.Query["projectId"] = project
	}
	switch c.op {
	case "graphs.definition.get":
		c.route += "/definition"
	case "graphs.wait":
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
	case "graphs.budget.get":
		c.route += "/budget"
	case "graphs.budget.ledger":
		c.route += "/budget-ledger"
	case "graphs.inputs.list":
		c.route += "/inputs"
	case "graphs.inputs.get", "graphs.inputs.members":
		if !domain.ValidID(flags["input"]) {
			return c, domain.Invalid("exact graph input identity")
		}
		c.route += "/inputs/" + flags["input"]
		if c.op == "graphs.inputs.members" {
			c.route += "/members"
		}
	case "graphs.dispatches.list":
		c.route += "/dispatches"
		if value, ok := flags["input"]; ok {
			if !domain.ValidID(value) {
				return c, domain.Invalid("exact graph input identity")
			}
			c.params.Query["inputId"] = value
		}
		if value, ok := flags["node"]; ok {
			if !domain.GraphKey(value) {
				return c, domain.Invalid("graph node key")
			}
			c.params.Query["nodeKey"] = value
		}
	case "graphs.dispatches.get":
		if !domain.ValidID(flags["dispatch"]) {
			return c, domain.Invalid("exact graph dispatch identity")
		}
		c.route += "/dispatches/" + flags["dispatch"]
	}
	return c, nil
}

func parseGraphMutation(c command, flags map[string]string, repeated map[string][]string, stdin io.Reader) (command, *domain.Error) {
	if c.op == "graphs.advance" {
		if err := graphProofGroup(flags, "graph-version", "graph-control-revision", "budget-authority-epoch", "window-number", "node-authorities-file"); err != nil {
			return c, err
		}
		c.graphIntent.AutoProof = graphProofAuto(flags, "graph-version")
		authorities := []domain.GraphNodeAuthority{}
		if !c.graphIntent.AutoProof {
			raw, err := readGraphFile(flags["node-authorities-file"], stdin)
			if err != nil {
				return c, err
			}
			authorities, err = domain.StrictDecodeGraphNodeAuthorities(raw)
			if err != nil {
				return c, err
			}
		}
		in := domain.GraphAdvanceInput{RequestID: c.requestID, GraphVersion: flags["graph-version"], ExpectedGraphControlRevision: flags["graph-control-revision"], ExpectedBudgetAuthorityEpoch: flags["budget-authority-epoch"], ExpectedWindowNumber: flags["window-number"], NodeAuthorities: authorities}
		if err := domain.ValidateGraphAdvanceInput(in); err != nil && !c.graphIntent.AutoProof {
			return c, err
		}
		c.route += "/advance"
		return encodeExecution(c, in)
	}
	if c.op == "graphs.stop" {
		if err := graphProofGroup(flags, "graph-control-revision"); err != nil {
			return c, err
		}
		c.graphIntent.AutoProof = graphProofAuto(flags, "graph-control-revision")
		in := domain.GraphStopInput{RequestID: c.requestID, ExpectedGraphControlRevision: flags["graph-control-revision"]}
		if err := domain.ValidateGraphStopInput(in); err != nil && !c.graphIntent.AutoProof {
			return c, err
		}
		c.route += "/stop"
		return encodeExecution(c, in)
	}
	if c.op == "graphs.budget.amend" {
		if err := graphProofGroup(flags, "graph-control-revision", "budget-authority-epoch", "window-number"); err != nil {
			return c, err
		}
		c.graphIntent.AutoProof = graphProofAuto(flags, "graph-control-revision")
		in := domain.GraphBudgetAmendmentInput{RequestID: c.requestID, ExpectedGraphControlRevision: flags["graph-control-revision"], ExpectedBudgetAuthorityEpoch: flags["budget-authority-epoch"], ExpectedWindowNumber: flags["window-number"], AdditionalAdmissions: flags["additional-admissions"]}
		if value, ok := flags["window-seconds"]; ok {
			n, err := boundedInteger(value, 1, 86400)
			if err != nil {
				return c, err
			}
			in.NewWindowSeconds = &n
		}
		if err := domain.ValidateGraphBudgetAmendmentInput(in); err != nil && !c.graphIntent.AutoProof {
			return c, err
		}
		c.route += "/budget-amendments"
		return encodeExecution(c, in)
	}
	proofs := []string{"graph-control-revision", "node-id", "node-control-revision"}
	if c.op == "graphs.nodes.guidance" {
		proofs = append(proofs, "expected-revision", "task-version")
	}
	if err := graphProofGroup(flags, proofs...); err != nil {
		return c, err
	}
	c.graphIntent.AutoProof = graphProofAuto(flags, "graph-control-revision")
	if !domain.GraphKey(flags["node"]) {
		return c, domain.Invalid("exact graph node key")
	}
	c.route += "/nodes/" + flags["node"]
	if c.op == "graphs.nodes.stop" {
		in := domain.GraphNodeStopInput{RequestID: c.requestID, ExpectedGraphControlRevision: flags["graph-control-revision"], NodeID: flags["node-id"], ExpectedNodeControlRevision: flags["node-control-revision"]}
		if err := domain.ValidateGraphNodeStopInput(in); err != nil && !c.graphIntent.AutoProof {
			return c, err
		}
		c.route += "/stop"
		return encodeExecution(c, in)
	}
	stdinSources := 0
	for _, key := range []string{"guidance-file", "conditions-file", "checks-file"} {
		if flags[key] == "-" {
			stdinSources++
		}
	}
	if stdinSources > 1 {
		return c, domain.Invalid("one stdin graph guidance source")
	}
	text, err := executionText(flags, "guidance", "guidance-file", stdin, true)
	if err != nil {
		return c, err
	}
	pins, acceptance, timeout, err := versionOptions(flags, repeated, stdin)
	if err != nil {
		return c, err
	}
	in := domain.GraphGuidanceInput{GuidanceInput: domain.GuidanceInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], GuidanceText: text, UseCurrentPolicy: flags["use-current-policy"] == "true", ClearPolicyOverride: flags["clear-policy-override"] == "true", PolicyOverride: flags["policy"], Pins: pins, Acceptance: acceptance, TimeoutSeconds: timeout}, ExpectedGraphControlRevision: flags["graph-control-revision"], NodeID: flags["node-id"], ExpectedNodeControlRevision: flags["node-control-revision"], TaskVersion: flags["task-version"]}
	in.Routing, err = routingOptions(flags, repeated, "guidance")
	if err != nil {
		return c, err
	}
	in.Workflow, err = workflowOptions(flags, stdin)
	if err != nil {
		return c, err
	}
	if err := domain.ValidateGraphGuidanceInput(in); err != nil && !c.graphIntent.AutoProof {
		return c, err
	}
	c.route += "/guidance"
	return encodeExecution(c, in)
}
