package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type graphIntentPlan struct {
	AutoProof  bool
	NodeKey    string
	RebindKeys []string
}

type graphIntentRecord struct {
	FormatVersion   string         `json:"formatVersion"`
	RequestID       string         `json:"requestId"`
	Operation       string         `json:"operation"`
	Method          string         `json:"method"`
	Path            string         `json:"path"`
	GraphID         string         `json:"graphId"`
	NodeKey         string         `json:"nodeKey"`
	AutoProof       bool           `json:"autoProof"`
	RebindKeys      []string       `json:"rebindKeys"`
	OriginalInput   jsontext.Value `json:"originalInput"`
	InputSHA256     string         `json:"inputSha256"`
	CapturedOwnerID string         `json:"capturedOwnerId"`
	Body            jsontext.Value `json:"body"`
	BodySHA256      string         `json:"bodySha256"`
}

func completeGraphIntentRecord(body []byte) bool {
	var fields map[string]jsontext.Value
	if json.Unmarshal(body, &fields) != nil || len(fields) != 14 {
		return false
	}
	for _, key := range []string{"formatVersion", "requestId", "operation", "method", "path", "graphId", "nodeKey", "inputSha256", "capturedOwnerId", "bodySha256"} {
		if value, ok := fields[key]; !ok || value.Kind() != '"' {
			return false
		}
	}
	return (fields["autoProof"].Kind() == 't' || fields["autoProof"].Kind() == 'f') && fields["rebindKeys"].Kind() == '[' && fields["originalInput"].Kind() == '{' && fields["body"].Kind() == '{'
}

func graphIntentHash(body []byte) string {
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

func graphIntentDescription(c command) graphIntentRecord {
	record := graphIntentRecord{FormatVersion: "006-cli-intent-v1", RequestID: c.requestID, Operation: c.op, Method: c.method, Path: c.route, GraphID: c.params.ID, OriginalInput: append(jsontext.Value{}, c.params.Body...), RebindKeys: []string{}}
	if c.graphIntent != nil {
		record.AutoProof = c.graphIntent.AutoProof
		record.NodeKey = c.graphIntent.NodeKey
		record.RebindKeys = append(record.RebindKeys, c.graphIntent.RebindKeys...)
	}
	return record
}

func graphIntentInputHash(record graphIntentRecord) string {
	body, err := json.Marshal(struct {
		RequestID  string         `json:"requestId"`
		Operation  string         `json:"operation"`
		Method     string         `json:"method"`
		Path       string         `json:"path"`
		GraphID    string         `json:"graphId"`
		NodeKey    string         `json:"nodeKey"`
		AutoProof  bool           `json:"autoProof"`
		RebindKeys []string       `json:"rebindKeys"`
		Input      jsontext.Value `json:"input"`
	}{record.RequestID, record.Operation, record.Method, record.Path, record.GraphID, record.NodeKey, record.AutoProof, record.RebindKeys, record.OriginalInput})
	if err != nil {
		return ""
	}
	return graphIntentHash(body)
}

// Every graph mutation reaches this guard before forwarding. The native request
// lock spans capture/publication/one POST, so another CLI cannot publish a second
// authority under the same ID. The immutable body remains after acknowledgement.
func forwardGraphIntent(ctx context.Context, root string, desc localstate.Descriptor, c command) domain.Result {
	uncertain := func() domain.Result {
		return domain.Failure(c.requestID, domain.Fail("write_uncertain", "The retained graph request is unresolved. Preserve its exact request identity and original input; no authority was refreshed."))
	}
	intent, err := localstate.OpenGraphIntent(ctx, root, c.requestID)
	if err != nil {
		return uncertain()
	}
	defer intent.Close()
	description := graphIntentDescription(c)
	description.InputSHA256 = graphIntentInputHash(description)
	if description.InputSHA256 == "" {
		return uncertain()
	}
	previous, seen, err := intent.Read()
	if err != nil {
		return uncertain()
	}
	if seen {
		var record graphIntentRecord
		if !completeGraphIntentRecord(previous) || json.Unmarshal(previous, &record, json.RejectUnknownMembers(true)) != nil || record.FormatVersion != "006-cli-intent-v1" || !domain.ValidID(record.CapturedOwnerID) || !domain.ValidSHA256(record.InputSHA256) || !domain.ValidSHA256(record.BodySHA256) || record.RebindKeys == nil || !record.OriginalInput.IsValid() || !record.Body.IsValid() || graphIntentInputHash(record) != record.InputSHA256 || graphIntentHash(record.Body) != record.BodySHA256 {
			return uncertain()
		}
		if record.InputSHA256 != description.InputSHA256 {
			return domain.Failure(c.requestID, domain.Fail("request_conflict", "This request identity already names different graph input. Retain the original intent or use a new identity for changed input."))
		}
		c.params.Body = append([]byte{}, record.Body...)
		if err := validateFrozenGraphBody(c); err != nil {
			return uncertain()
		}
		return forward(ctx, desc, c)
	}
	if err = intent.MarkSeen(); err != nil {
		return uncertain()
	}
	if c.graphIntent != nil && (c.graphIntent.AutoProof || len(c.graphIntent.RebindKeys) > 0) {
		detail, readErr := readGraphIntentDetail(ctx, desc, c.params.ID)
		if readErr != nil {
			return domain.Failure(c.requestID, readErr)
		}
		var bindingError *domain.Error
		c, bindingError = bindGraphIntent(c, detail)
		if bindingError != nil {
			return domain.Failure(c.requestID, bindingError)
		}
	}
	if err := validateFrozenGraphBody(c); err != nil {
		return domain.Failure(c.requestID, err)
	}
	description.CapturedOwnerID = desc.OwnerInstanceID
	description.Body = append(jsontext.Value{}, c.params.Body...)
	description.BodySHA256 = graphIntentHash(description.Body)
	encoded, err := json.Marshal(description)
	if err != nil {
		return uncertain()
	}
	if err = intent.Write(encoded); err != nil {
		return uncertain()
	}
	// Re-read the same protected, published bytes before POST. Partial publication
	// and private namespace changes must not send even a currently held in-memory body.
	verified, retained, err := intent.Read()
	if err != nil || !retained || !bytes.Equal(verified, encoded) {
		return uncertain()
	}
	return forward(ctx, desc, c)
}

func readGraphIntentDetail(ctx context.Context, desc localstate.Descriptor, id string) (domain.GraphDetail, *domain.Error) {
	var detail domain.GraphDetail
	read := command{op: "graphs.get", method: "GET", route: "/graphs/" + id}
	result := forward(ctx, desc, read)
	if result.Envelope.Error != nil {
		return detail, result.Envelope.Error
	}
	body, err := json.Marshal(result.Envelope.Data)
	// Graph detail is a response with required nullable fields. The mutation
	// decoder rejects null recursively and must not be used for this view.
	if err != nil || len(body) > 4<<20 || json.Unmarshal(body, &detail, json.RejectUnknownMembers(true)) != nil {
		return detail, domain.Fail("invalid_response", "The graph observation is not a complete bounded detail.")
	}
	if detail.Graph.ID != id || !domain.ValidID(detail.Graph.RootTaskID) || detail.Definition.GraphID != id || detail.Definition.GraphVersion != detail.Graph.GraphVersion || !domain.ValidSHA256(detail.Definition.DeclarationSHA256) || detail.Budget.GraphID != id || detail.Budget.BudgetOwnerTaskID != detail.Graph.RootTaskID || detail.Budget.Budget.TaskID != detail.Graph.RootTaskID || detail.Budget.BudgetAuthorityEpoch != detail.Graph.BudgetAuthorityEpoch || len(detail.Nodes) < 1 || len(detail.Nodes) > domain.MaxGraphNodes {
		return detail, domain.Fail("invalid_response", "The graph observation does not identify its exact graph, nodes and shared budget.")
	}
	for _, version := range []string{detail.Graph.GraphVersion, detail.Graph.ControlRevision, detail.Graph.BudgetAuthorityEpoch, detail.Budget.Budget.CurrentWindow.Number} {
		if _, e := domain.ParseRevision(version); e != nil {
			return detail, domain.Fail("invalid_response", "The graph observation has invalid authority counters.")
		}
	}
	seenIDs, seenKeys := map[string]bool{}, map[string]bool{}
	for _, node := range detail.Nodes {
		if node.GraphID != id || !domain.ValidID(node.ID) || !domain.ValidID(node.TaskID) || !domain.GraphKey(node.Key) || seenIDs[node.ID] || seenKeys[node.Key] {
			return detail, domain.Fail("invalid_response", "The graph observation has foreign or repeated node identities.")
		}
		seenIDs[node.ID], seenKeys[node.Key] = true, true
		for _, version := range []string{node.TaskVersion, node.TaskRevision, node.ControlRevision} {
			if _, e := domain.ParseRevision(version); e != nil {
				return detail, domain.Fail("invalid_response", "The graph observation has invalid node authority counters.")
			}
		}
	}
	return detail, nil
}

func bindGraphIntent(c command, detail domain.GraphDetail) (command, *domain.Error) {
	plan := c.graphIntent
	var node *domain.GraphNodeSummary
	if plan.NodeKey != "" {
		for i := range detail.Nodes {
			if detail.Nodes[i].Key == plan.NodeKey {
				node = &detail.Nodes[i]
				break
			}
		}
		if node == nil {
			return c, domain.Fail("not_found", "The captured graph has no node with this exact key.")
		}
	}
	switch c.op {
	case "graphs.advance":
		var in domain.GraphAdvanceInput
		if json.Unmarshal(c.params.Body, &in, json.RejectUnknownMembers(true)) != nil {
			return c, domain.Invalid("graph advance input")
		}
		if plan.AutoProof {
			in.GraphVersion, in.ExpectedGraphControlRevision = detail.Graph.GraphVersion, detail.Graph.ControlRevision
			in.ExpectedBudgetAuthorityEpoch, in.ExpectedWindowNumber = detail.Graph.BudgetAuthorityEpoch, detail.Budget.Budget.CurrentWindow.Number
			in.NodeAuthorities = make([]domain.GraphNodeAuthority, 0, len(detail.Nodes))
			for _, n := range detail.Nodes {
				in.NodeAuthorities = append(in.NodeAuthorities, domain.GraphNodeAuthority{NodeID: n.ID, TaskVersion: n.TaskVersion, NodeControlRevision: n.ControlRevision})
			}
		} else if in.GraphVersion != detail.Graph.GraphVersion || in.ExpectedGraphControlRevision != detail.Graph.ControlRevision || in.ExpectedBudgetAuthorityEpoch != detail.Graph.BudgetAuthorityEpoch || in.ExpectedWindowNumber != detail.Budget.Budget.CurrentWindow.Number {
			return c, domain.Fail("revision_conflict", "The explicit rebind proofs do not identify the captured current graph.")
		}
		if len(plan.RebindKeys) > 0 {
			ids := make([]string, 0, len(plan.RebindKeys))
			for _, key := range plan.RebindKeys {
				found := false
				for _, n := range detail.Nodes {
					if n.Key != key {
						continue
					}
					for _, authority := range in.NodeAuthorities {
						if authority.NodeID == n.ID && authority.TaskVersion == n.TaskVersion && authority.NodeControlRevision == n.ControlRevision {
							ids = append(ids, n.ID)
							found = true
							break
						}
					}
					break
				}
				if !found {
					return c, domain.Fail("revision_conflict", "A rebind key has no matching exact captured node authority.")
				}
			}
			in.RebindNodeIDs = &ids
		}
		return encodeExecution(c, in)
	case "graphs.stop":
		var in domain.GraphStopInput
		if json.Unmarshal(c.params.Body, &in) != nil {
			return c, domain.Invalid("graph Stop input")
		}
		in.ExpectedGraphControlRevision = detail.Graph.ControlRevision
		return encodeExecution(c, in)
	case "graphs.nodes.stop":
		if node == nil {
			return c, domain.Invalid("graph node selector")
		}
		var in domain.GraphNodeStopInput
		if json.Unmarshal(c.params.Body, &in) != nil {
			return c, domain.Invalid("graph node Stop input")
		}
		in.ExpectedGraphControlRevision = detail.Graph.ControlRevision
		in.NodeID = node.ID
		in.ExpectedNodeControlRevision = node.ControlRevision
		return encodeExecution(c, in)
	case "graphs.nodes.guidance":
		if node == nil {
			return c, domain.Invalid("graph node selector")
		}
		var in domain.GraphGuidanceInput
		if json.Unmarshal(c.params.Body, &in) != nil {
			return c, domain.Invalid("graph guidance input")
		}
		in.ExpectedGraphControlRevision = detail.Graph.ControlRevision
		in.NodeID = node.ID
		in.ExpectedNodeControlRevision = node.ControlRevision
		in.TaskVersion = node.TaskVersion
		in.ExpectedRevision = node.TaskRevision
		return encodeExecution(c, in)
	case "graphs.budget.amend":
		var in domain.GraphBudgetAmendmentInput
		if json.Unmarshal(c.params.Body, &in) != nil {
			return c, domain.Invalid("graph grant input")
		}
		in.ExpectedGraphControlRevision = detail.Graph.ControlRevision
		in.ExpectedBudgetAuthorityEpoch = detail.Graph.BudgetAuthorityEpoch
		in.ExpectedWindowNumber = detail.Budget.Budget.CurrentWindow.Number
		return encodeExecution(c, in)
	}
	return c, domain.Invalid("graph control operation")
}

func validateFrozenGraphBody(c command) *domain.Error {
	if !strings.HasPrefix(c.op, "graphs.") || c.method != "POST" || !domain.ValidID(c.requestID) {
		return domain.Invalid("graph mutation identity")
	}
	var id string
	var invalid *domain.Error
	switch c.op {
	case "graphs.start":
		in, e := domain.StrictDecodeGraphStart(c.params.Body)
		if e != nil {
			return e
		}
		id = in.RequestID
		invalid = domain.ValidateGraphStartInput(in)
	case "graphs.advance":
		var in domain.GraphAdvanceInput
		if e := domain.StrictDecode(c.params.Body, &in); e != nil {
			return e
		}
		id = in.RequestID
		invalid = domain.ValidateGraphAdvanceInput(in)
	case "graphs.stop":
		var in domain.GraphStopInput
		if e := domain.StrictDecode(c.params.Body, &in); e != nil {
			return e
		}
		id = in.RequestID
		invalid = domain.ValidateGraphStopInput(in)
	case "graphs.nodes.stop":
		var in domain.GraphNodeStopInput
		if e := domain.StrictDecode(c.params.Body, &in); e != nil {
			return e
		}
		id = in.RequestID
		invalid = domain.ValidateGraphNodeStopInput(in)
	case "graphs.nodes.guidance":
		var in domain.GraphGuidanceInput
		if e := domain.StrictDecode(c.params.Body, &in); e != nil {
			return e
		}
		id = in.RequestID
		invalid = domain.ValidateGraphGuidanceInput(in)
	case "graphs.budget.amend":
		var in domain.GraphBudgetAmendmentInput
		if e := domain.StrictDecode(c.params.Body, &in); e != nil {
			return e
		}
		id = in.RequestID
		invalid = domain.ValidateGraphBudgetAmendmentInput(in)
	default:
		return domain.Invalid("graph mutation operation")
	}
	if id != c.requestID {
		return domain.Invalid("matching frozen graph request identity")
	}
	return invalid
}
