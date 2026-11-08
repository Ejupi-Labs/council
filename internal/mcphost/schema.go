package mcphost

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"io"
	"strconv"
	"sync"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
	"github.com/google/jsonschema-go/jsonschema"
)

var resolvedArgumentSchemas sync.Map // name and approved digest -> *jsonschema.Resolved

// ValidateArguments validates the approved schema and the owner's pure input
// rules, then binds the exact original argument bytes to the proposed request.
// It does not attach an owner, dispatch, apply defaults, or establish any
// state-dependent authority. Returned errors intentionally omit input values.
func ValidateArguments(tool, operation string, raw []byte, req ownerclient.Request) *domain.Error {
	var mapping ToolMapping
	for _, item := range ToolCatalog() {
		if item.Name == tool {
			mapping = item
			break
		}
	}
	if mapping.Name == "" || mapping.Operation != operation || req.Operation != operation {
		return domain.Invalid("documented tool and operation")
	}
	approved, ok := schemas.Lookup(tool)
	if !ok {
		return domain.Fail("capability_unavailable", "The tool schema is not approved for use.")
	}
	identity, ok := schemas.Digest(tool)
	if !ok {
		return domain.Fail("capability_unavailable", "The tool schema is not approved for use.")
	}
	resolved, err := resolvedArgumentSchema(tool+":"+identity, approved)
	if err != nil {
		return domain.Fail("capability_unavailable", "The approved tool schema is unavailable.")
	}
	if len(raw) == 0 {
		// Omitted MCP arguments are equivalent to an empty object only if the
		// approved schema accepts that object. No owner body or identity is made.
		raw = []byte(`{}`)
	}
	if len(raw) > MaxFrameBytes {
		return domain.Fail("input_too_large", "Tool arguments exceed the documented wire limit.")
	}
	instance, err := strictArgumentValue(raw)
	if err != nil {
		return domain.Invalid("unique, non-null JSON tool arguments")
	}
	if err := resolved.Validate(instance); err != nil {
		return domain.Invalid("documented tool arguments")
	}
	expected, invalid := OwnerRequest(mapping, raw)
	if invalid != nil {
		return domain.Invalid("documented tool arguments")
	}
	if !sameOwnerRequest(expected, req) {
		return domain.Invalid("unchanged owner request and arguments")
	}
	route, ok := ownerclient.RouteFor(operation)
	if !ok {
		return domain.Fail("capability_unavailable", "The owner operation is unavailable.")
	}
	if route.Method == "GET" {
		if invalid, known := schemas.ValidateOwnerQuery(operation, req.PathIDs, req.Query); !known {
			return domain.Fail("capability_unavailable", "The read validator is unavailable.")
		} else if invalid != nil {
			return safeInputError(invalid)
		}
	} else {
		pathID := ""
		if len(req.PathIDs) > 0 {
			pathID = req.PathIDs[0]
		}
		if invalid, known := schemas.ValidateOwnerBody(operation, pathID, req.Body); !known {
			return domain.Fail("capability_unavailable", "The mutation validator is unavailable.")
		} else if invalid != nil {
			return safeInputError(invalid)
		}
	}
	return nil
}

func resolvedArgumentSchema(key string, raw []byte) (*jsonschema.Resolved, error) {
	if cached, ok := resolvedArgumentSchemas.Load(key); ok {
		return cached.(*jsonschema.Resolved), nil
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, err
	}
	actual, _ := resolvedArgumentSchemas.LoadOrStore(key, resolved)
	return actual.(*jsonschema.Resolved), nil
}

// Preserve known owner error classes without exposing arbitrary error text.
func safeInputError(invalid *domain.Error) *domain.Error {
	switch invalid.Code {
	case "invalid_request", "invalid_encoding", "invalid_cursor", "input_too_large", "external_postcondition_unsupported":
		return domain.Fail(invalid.Code, "Tool arguments do not satisfy Council's input requirements.")
	default:
		return domain.Invalid("documented tool arguments")
	}
}

func strictArgumentValue(raw []byte) (any, error) {
	if !utf8.Valid(raw) {
		return nil, io.ErrUnexpectedEOF
	}
	// jsontext rejects duplicate names and malformed Unicode at every depth.
	reader := jsontext.NewDecoder(bytes.NewReader(raw))
	if _, err := reader.ReadValue(); err != nil {
		return nil, err
	}
	if _, err := reader.ReadValue(); err != io.EOF {
		return nil, io.ErrUnexpectedEOF
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return exactSchemaNumbers(value)
}

// Every currently approved numeric input denotes a Go integer. jsonschema-go
// represents json.Number as a string during type checks, so decode integer
// tokens to int64 explicitly instead of rounding through float64. Owner DTOs
// reject fractional/exponent tokens and signed 64-bit overflow as well.
func exactSchemaNumbers(value any) (any, error) {
	switch v := value.(type) {
	case nil:
		return nil, io.ErrUnexpectedEOF
	case json.Number:
		n, err := strconv.ParseInt(v.String(), 10, 64)
		if err != nil {
			return nil, err
		}
		return n, nil
	case map[string]any:
		for key, item := range v {
			converted, err := exactSchemaNumbers(item)
			if err != nil {
				return nil, err
			}
			v[key] = converted
		}
	case []any:
		for i, item := range v {
			converted, err := exactSchemaNumbers(item)
			if err != nil {
				return nil, err
			}
			v[i] = converted
		}
	}
	return value, nil
}

func sameOwnerRequest(a, b ownerclient.Request) bool {
	if a.Operation != b.Operation || a.RequestID != b.RequestID || !bytes.Equal(a.Body, b.Body) || len(a.PathIDs) != len(b.PathIDs) || len(a.Query) != len(b.Query) {
		return false
	}
	for i := range a.PathIDs {
		if a.PathIDs[i] != b.PathIDs[i] {
			return false
		}
	}
	for key, value := range a.Query {
		if other, ok := b.Query[key]; !ok || value != other {
			return false
		}
	}
	return true
}
