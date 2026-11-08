package ownerclient

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/owner"
)

const maxResponseBytes = 64 * 1024 * 1024

// Request addresses one fixed owner operation. PathIDs are ordered by the
// route's PathKinds, and Body is the unchanged owner JSON object for mutations.
// RequestID is a durable Council ID, never an MCP correlation ID.
type Request struct {
	Operation string
	PathIDs   []string
	Query     map[string]string
	Body      []byte
	RequestID string
}

type Client struct{ root string }

// New validates the state-root spelling without opening it or starting an
// owner. Initial MCP negotiation and tools/list need not touch local state.
func New(root string) (*Client, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, errors.New("Council state root must be an absolute clean path.")
	}
	return &Client{root: root}, nil
}
func NewDefault() (*Client, error) {
	root, err := localstate.DefaultRoot()
	if err != nil {
		return nil, err
	}
	return New(root)
}

func client() *http.Client {
	return &http.Client{Timeout: 35 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func requestIdentity(id string) string {
	if domain.ValidID(id) {
		return id
	}
	return domain.NewID()
}
func failure(id string, err *domain.Error) domain.Result {
	return domain.Failure(requestIdentity(id), err)
}
func ownerUnavailable(id string) domain.Result {
	return failure(id, domain.Fail("owner_unavailable", "The private Council owner is unavailable or incompatible. Preserve its lock and use the recognized maintenance Stop when available."))
}

func (c *Client) attach(ctx context.Context, operation string) (localstate.Descriptor, error) {
	if operation == "service.stop" {
		desc, err := localstate.ReadDescriptor(c.root)
		if err != nil {
			return localstate.Descriptor{}, err
		}
		if !VerifyMaintenanceOwner(ctx, desc) {
			return localstate.Descriptor{}, errors.New("incompatible maintenance owner")
		}
		return desc, nil
	}
	verify := func(ctx context.Context, desc localstate.Descriptor) bool {
		if strings.HasPrefix(operation, "history.") {
			return VerifyHistoryOwner(ctx, desc)
		}
		return VerifyExecutionOwner(ctx, desc)
	}
	return owner.Ensure(ctx, c.root, verify)
}

func validateRequest(req Request) (Route, string, *domain.Error) {
	r, ok := RouteFor(req.Operation)
	if !ok {
		return Route{}, "", domain.Fail("not_found", "The owner operation is not in the supported catalog.")
	}
	path, invalid := resolveRoute(r, req.PathIDs, req.Query)
	if invalid != nil {
		return Route{}, "", invalid
	}
	if r.Method == "GET" {
		if len(req.Body) != 0 || req.RequestID != "" {
			return Route{}, "", domain.Invalid("read operation without a body or mutation identity")
		}
		return r, path, nil
	}
	if !domain.ValidID(req.RequestID) {
		return Route{}, "", domain.Invalid("durable request identity")
	}
	if len(req.Body) > r.BodyLimit {
		return Route{}, "", domain.Fail("input_too_large", "The owner request exceeds its documented bound.")
	}
	var members map[string]jsontext.Value
	if invalid = domain.StrictDecode(req.Body, &members); invalid != nil {
		return Route{}, "", invalid
	}
	var id string
	if value, ok := members["requestId"]; !ok || json.Unmarshal(value, &id) != nil || id != req.RequestID {
		return Route{}, "", domain.Invalid("matching durable request identity")
	}
	if req.Operation == "service.stop" {
		var target string
		if value, ok := members["ownerInstanceId"]; !ok || json.Unmarshal(value, &target) != nil || !domain.ValidID(target) {
			return Route{}, "", domain.Invalid("captured owner identity")
		}
	}
	return r, path, nil
}

// Call lazily attaches to the verified current owner only after local request
// validation. It never retries a mutation or rebinds captured authority.
func (c *Client) Call(ctx context.Context, req Request) domain.Result {
	if c == nil || c.root == "" {
		return ownerUnavailable(req.RequestID)
	}
	r, path, invalid := validateRequest(req)
	if invalid != nil {
		return failure(req.RequestID, invalid)
	}
	if err := ctx.Err(); err != nil {
		if r.Method == "GET" {
			return failure("", domain.Fail("observation_interrupted", "Observation ended; owned work continues."))
		}
		return failure(req.RequestID, domain.Fail("operation_interrupted", "The operation was interrupted before it was sent."))
	}
	desc, err := c.attach(ctx, req.Operation)
	if err != nil {
		return ownerUnavailable(req.RequestID)
	}
	return forward(ctx, desc, r, path, req)
}

func forward(ctx context.Context, desc localstate.Descriptor, r Route, path string, req Request) domain.Result {
	uncertain := func() domain.Result {
		if r.Method != "GET" {
			return failure(req.RequestID, domain.Fail("write_uncertain", "The owner acknowledgement is uncertain. Retry the same request identity and original input."))
		}
		if ctx.Err() != nil {
			return failure("", domain.Fail("observation_interrupted", "Observation ended; owned work continues."))
		}
		return ownerUnavailable("")
	}
	request, err := http.NewRequestWithContext(ctx, r.Method, desc.Origin+path, bytes.NewReader(req.Body))
	if err != nil {
		return uncertain()
	}
	request.Header.Set("Authorization", "Bearer "+desc.Token)
	request.Header.Set("Origin", desc.Origin)
	if r.Method != "GET" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client().Do(request)
	if err != nil {
		return uncertain()
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes || desc.Token != "" && bytes.Contains(body, []byte(desc.Token)) {
		return uncertain()
	}
	// Decode the owner payload as raw JSON: an interface{} decode would turn
	// every nested JSON number into float64 and silently change large integers.
	var wire struct {
		APIVersion int             `json:"apiVersion"`
		RequestID  string          `json:"requestId"`
		Data       jsontext.Value  `json:"data"`
		Receipt    *domain.Receipt `json:"receipt"`
		Error      *domain.Error   `json:"error"`
	}
	if json.Unmarshal(body, &wire, json.RejectUnknownMembers(true)) != nil || wire.APIVersion != domain.APIVersion || !domain.ValidID(wire.RequestID) || (len(wire.Data) == 0 || wire.Data.Kind() == 'n') == (wire.Error == nil) {
		return uncertain()
	}
	envelope := domain.Envelope{APIVersion: wire.APIVersion, RequestID: wire.RequestID, Receipt: wire.Receipt, Error: wire.Error}
	if len(wire.Data) != 0 && wire.Data.Kind() != 'n' {
		envelope.Data = wire.Data.Clone()
	}
	if req.RequestID != "" && envelope.RequestID != req.RequestID {
		return uncertain()
	}
	if envelope.Error != nil && response.StatusCode != domain.Status(envelope.Error.Code) {
		return uncertain()
	}
	if envelope.Error == nil && response.StatusCode != 200 && response.StatusCode != 201 && response.StatusCode != 202 {
		return uncertain()
	}
	if envelope.Error != nil || r.Method == "GET" {
		if envelope.Receipt != nil {
			return uncertain()
		}
	} else {
		if envelope.Receipt == nil || envelope.Receipt.RequestID != req.RequestID || envelope.Receipt.Operation != req.Operation {
			return uncertain()
		}
		timestamp, err := time.Parse("2006-01-02T15:04:05.000Z", envelope.Receipt.CommittedAt)
		if err != nil || timestamp.UTC().Format("2006-01-02T15:04:05.000Z") != envelope.Receipt.CommittedAt {
			return uncertain()
		}
	}
	return domain.Result{Status: response.StatusCode, Envelope: envelope}
}
