package cli

import (
	"io"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

var workflowTaskFlags = []string{"workflow", "delivery", "delivery-file", "checks-file", "review", "max-corrections"}

func addWorkflowFlags(values map[string]bool) {
	for _, key := range append(append([]string{}, workflowTaskFlags...), "workflow-id", "workflow-revision", "check-id") {
		values[key] = true
	}
}

func workflowOptions(flags map[string]string, stdin io.Reader) (*domain.WorkflowInput, *domain.Error) {
	var in *domain.WorkflowInput
	for _, key := range workflowTaskFlags {
		if _, ok := flags[key]; ok {
			in = &domain.WorkflowInput{}
			break
		}
	}
	if in == nil {
		return nil, nil
	}
	in.Mode, in.Review = flags["workflow"], flags["review"]
	if _, a := flags["delivery"]; a {
		if _, b := flags["delivery-file"]; b {
			return nil, domain.Invalid("exactly one explicit delivery option")
		}
		in.Delivery = &domain.WorkflowDeliveryInput{LocalMode: flags["delivery"]}
	}
	if filename, ok := flags["delivery-file"]; ok {
		raw, e := readExecutionFile(filename, stdin)
		if e != nil {
			return nil, e
		}
		if strings.TrimSpace(raw) == "null" {
			return nil, domain.Invalid("non-null delivery declaration")
		}
		in.Delivery = &domain.WorkflowDeliveryInput{}
		if e = domain.StrictDecode([]byte(raw), in.Delivery); e != nil {
			return nil, e
		}
	}
	if filename, ok := flags["checks-file"]; ok {
		raw, e := readExecutionFile(filename, stdin)
		if e != nil {
			return nil, e
		}
		if strings.TrimSpace(raw) == "null" {
			return nil, domain.Invalid("non-null check declaration array")
		}
		checks, e := domain.StrictDecodeCheckDeclarations([]byte(raw))
		if e != nil {
			return nil, e
		}
		in.Checks = &checks
	}
	if value, ok := flags["max-corrections"]; ok {
		n, e := boundedInteger(value, 0, 1)
		if e != nil {
			return nil, e
		}
		in.MaxCorrectionCycles = &n
	}
	if e := domain.ValidateWorkflowInput(in); e != nil {
		return nil, e
	}
	return in, nil
}

func workflowCommand(name string) bool {
	switch name {
	case "workflow preferences", "tasks workflow", "tasks steps", "tasks reviews", "tasks checks", "tasks findings", "tasks deliveries", "tasks candidates", "tasks workflow-adopt", "tasks advance", "tasks check", "candidates changes":
		return true
	}
	return false
}

func workflowSelectors(c *command, flags map[string]string) *domain.Error {
	workflow, hasWorkflow := flags["workflow-id"]
	version, hasVersion := flags["task-version"]
	if hasWorkflow && hasVersion {
		return domain.Invalid("one historical workflow-id or task-version selector")
	}
	if hasWorkflow {
		if !domain.ValidID(workflow) {
			return domain.Invalid("workflow identity")
		}
		c.params.Query["workflowId"] = workflow
	}
	if hasVersion {
		if _, e := domain.ParseRevision(version); e != nil {
			return e
		}
		c.params.Query["taskVersion"] = version
	}
	return nil
}

func parseWorkflow(c command, pos []string, flags map[string]string, stdin io.Reader) (command, *domain.Error) {
	allowed := map[string]bool{"json": true, "api-version": true}
	allow := func(keys ...string) {
		for _, key := range keys {
			allowed[key] = true
		}
	}
	c.method = "GET"
	count := 3
	identity := true
	switch c.name {
	case "workflow preferences":
		identity = false
		c.route = "/workflow/preferences"
		if len(pos) < 3 {
			return c, domain.Invalid("workflow preferences get or update")
		}
		switch pos[2] {
		case "get":
			c.op = "workflow.preferences.get"
		case "update":
			c.op = "workflow.preferences.update"
			c.method = "PATCH"
			allow("request-id", "expected-revision", "delivery")
		default:
			return c, domain.Invalid("workflow preferences get or update")
		}
	case "tasks workflow":
		c.op = "tasks.workflow.get"
		allow("workflow-id", "task-version")
	case "tasks steps", "tasks reviews", "tasks checks", "tasks findings", "tasks deliveries", "tasks candidates":
		c.op = "tasks.workflow." + pos[1] + ".list"
		allow("workflow-id", "task-version", "limit", "cursor")
	case "candidates changes":
		c.op = "candidates.changes.list"
		allow("limit", "cursor")
	case "tasks workflow-adopt":
		c.op = "tasks.workflow.adopt"
		c.method = "POST"
		allow("request-id", "expected-revision", "task-version")
		allow(workflowTaskFlags...)
	case "tasks advance", "tasks check":
		c.op = "tasks.workflow." + pos[1]
		c.method = "POST"
		allow("request-id", "expected-revision", "workflow-revision", "task-version")
		if c.name == "tasks check" {
			allow("check-id")
		}
	default:
		return c, domain.Invalid("documented workflow command")
	}
	if len(pos) != count {
		return c, domain.Invalid("workflow command arguments")
	}
	for key := range flags {
		if !allowed[key] {
			return c, domain.Invalid("options supported by this workflow command")
		}
	}
	if identity {
		c.params.ID = pos[2]
		if !domain.ValidID(c.params.ID) {
			return c, domain.Invalid("workflow resource identity")
		}
		if c.name == "candidates changes" {
			c.route = "/candidates/" + c.params.ID + "/changes"
		} else {
			suffix := "workflow"
			if c.name != "tasks workflow" {
				suffix += "/" + pos[1]
			}
			if c.name == "tasks workflow-adopt" {
				suffix = "workflow/adopt"
			}
			c.route = "/tasks/" + c.params.ID + "/" + suffix
		}
	}
	if c.method == "GET" {
		if identity && c.name != "candidates changes" {
			if e := workflowSelectors(&c, flags); e != nil {
				return c, e
			}
		}
		if strings.HasSuffix(c.op, ".list") {
			if e := validatePageFlags(&c, flags); e != nil {
				return c, e
			}
		}
		return c, nil
	}
	var e *domain.Error
	c.requestID, e = mutationRequestID(flags)
	if e != nil {
		return c, e
	}
	switch c.op {
	case "workflow.preferences.update":
		in := domain.DeliveryPreferenceInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], LocalMode: flags["delivery"]}
		if e := domain.ValidateDeliveryPreferenceInput(in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	case "tasks.workflow.adopt":
		workflow, e := workflowOptions(flags, stdin)
		if e != nil {
			return c, e
		}
		if workflow == nil {
			workflow = &domain.WorkflowInput{Mode: "auto"}
		}
		in := domain.WorkflowAdoptInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], TaskVersion: flags["task-version"], Workflow: workflow}
		if e := domain.ValidateWorkflowAdoptInput(in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	case "tasks.workflow.advance":
		in := domain.WorkflowAdvanceInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], WorkflowRevision: flags["workflow-revision"], TaskVersion: flags["task-version"]}
		if e := domain.ValidateWorkflowAdvanceInput(in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	case "tasks.workflow.check":
		in := domain.WorkflowCheckInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], WorkflowRevision: flags["workflow-revision"], TaskVersion: flags["task-version"], CheckID: flags["check-id"]}
		if e := domain.ValidateWorkflowCheckInput(in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	}
	return c, domain.Invalid("workflow operation")
}
