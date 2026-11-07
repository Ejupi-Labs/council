package engine

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Graph observations never register execution intent, release capacity or wake
// retained work. Composite path identities select one exact immutable resource.
func (e *Engine) readGraph(ctx context.Context, op, target string, q map[string]string) domain.Result {
	requestID := domain.NewID()
	fail := func(err error) domain.Result { return e.failure(requestID, err) }
	var keys []string
	composite := false
	switch op {
	case "graphs.list":
		keys = []string{"projectId", "limit", "cursor"}
	case "graphs.get", "graphs.definition.get", "graphs.budget.get":
	case "graphs.wait":
		keys = []string{"afterRevision", "timeoutMs"}
	case "graphs.budget.ledger", "graphs.inputs.list":
		keys = []string{"limit", "cursor"}
	case "graphs.inputs.members":
		keys, composite = []string{"limit", "cursor"}, true
	case "graphs.inputs.get", "graphs.dispatches.get":
		composite = true
	case "graphs.dispatches.list":
		keys = []string{"inputId", "nodeKey", "limit", "cursor"}
	default:
		return fail(domain.Fail("not_found", "The graph read does not exist."))
	}
	if invalid := domain.ValidateQuery(q, keys...); invalid != nil {
		return fail(invalid)
	}
	for key, value := range q {
		if value == "" {
			if key == "cursor" {
				return fail(domain.Fail("invalid_cursor", "Supply the complete graph cursor or omit it to start a list."))
			}
			return fail(domain.Invalid("nonempty graph query values"))
		}
	}
	id, selected := target, ""
	if op == "graphs.list" {
		if target != "" || q["projectId"] != "" && !domain.ValidID(q["projectId"]) {
			return fail(domain.Invalid("graph list project identity"))
		}
	} else if composite {
		parts := strings.Split(target, "/")
		if len(parts) != 2 || !domain.ValidID(parts[0]) || !domain.ValidID(parts[1]) {
			return fail(domain.Invalid("exact graph and selected resource identities"))
		}
		id, selected = parts[0], parts[1]
	} else if !domain.ValidID(id) {
		return fail(domain.Invalid("graph identity"))
	}
	if value := q["inputId"]; value != "" && !domain.ValidID(value) {
		return fail(domain.Invalid("graph input identity"))
	}
	if value := q["nodeKey"]; value != "" && !domain.GraphKey(value) {
		return fail(domain.Invalid("graph node key"))
	}
	limit := 30
	if value, present := q["limit"]; present {
		parsed, invalid := domain.ParseLimit(value)
		if invalid != nil {
			return fail(invalid)
		}
		limit = parsed
	}
	var data any
	var err error
	switch op {
	case "graphs.list":
		data, err = e.db.Graphs(ctx, q["projectId"], limit, q["cursor"])
	case "graphs.get":
		data, err = e.db.Graph(ctx, id)
	case "graphs.definition.get":
		data, err = e.db.GraphDefinition(ctx, id)
	case "graphs.wait":
		data, err = e.waitGraph(ctx, id, q)
	case "graphs.budget.get":
		data, err = e.db.GraphBudget(ctx, id)
	case "graphs.budget.ledger":
		data, err = e.db.GraphBudgetLedger(ctx, id, limit, q["cursor"])
	case "graphs.inputs.list":
		data, err = e.db.GraphInputs(ctx, id, limit, q["cursor"])
	case "graphs.inputs.get":
		data, err = e.db.GraphInput(ctx, id, selected)
	case "graphs.dispatches.list":
		data, err = e.db.GraphDispatches(ctx, id, q["inputId"], q["nodeKey"], limit, q["cursor"])
	case "graphs.dispatches.get":
		data, err = e.db.GraphDispatch(ctx, id, selected)
	case "graphs.inputs.members":
		data, err = e.graphInputMembers(ctx, id, selected, limit, q["cursor"])
	}
	if err != nil {
		return fail(err)
	}
	return domain.Success(200, requestID, data, nil)
}

func (e *Engine) waitGraph(ctx context.Context, id string, q map[string]string) (any, error) {
	after := int64(0)
	if text := q["afterRevision"]; text != "" {
		parsed, invalid := domain.ParseCount(text)
		if invalid != nil {
			return nil, invalid
		}
		after = parsed
	}
	timeout := 30000
	if text, present := q["timeoutMs"]; present {
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed < 0 || parsed > 30000 || strconv.Itoa(parsed) != text {
			return nil, domain.Invalid("timeoutMs from 0 through 30000")
		}
		timeout = parsed
	}
	timer := time.NewTimer(time.Duration(timeout) * time.Millisecond)
	defer timer.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		view, err := e.db.Graph(ctx, id)
		if err != nil {
			return nil, err
		}
		current, invalid := domain.ParseRevision(view.Graph.StateRevision)
		if invalid != nil {
			return nil, invalid
		}
		if current > after {
			return map[string]any{"graph": view, "changed": true, "timedOut": false}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return map[string]any{"graph": view, "changed": false, "timedOut": true}, nil
		case <-tick.C:
		}
	}
}
