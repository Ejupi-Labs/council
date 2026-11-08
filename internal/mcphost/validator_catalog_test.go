package mcphost

import (
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

// Fail closed if a new mutation is added without the corresponding pure
// owner validation at the MCP boundary. This does not approve its schema.
func TestEveryMutationHasOwnerBodyValidation(t *testing.T) {
	for _, route := range ownerclient.Routes() {
		if route.Method == "GET" {
			continue
		}
		if _, known := schemas.ValidateOwnerBody(route.Operation, "", nil); !known {
			t.Errorf("mutation %s lacks owner body validation", route.Operation)
		}
	}
}
