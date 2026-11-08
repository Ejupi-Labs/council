package cli

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

// Keep the CLI's maintenance entrypoint on the same independently checked
// owner compatibility contract used by the MCP host.
func verifyMaintenanceOwner(ctx context.Context, desc localstate.Descriptor) bool {
	return ownerclient.VerifyMaintenanceOwner(ctx, desc)
}
