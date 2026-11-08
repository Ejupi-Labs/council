package mcphost

import (
	"os"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/process"
)

// Unix process ownership re-executes the current binary as its guardian.
// The test binary must recognize that private entrypoint before running tests.
func TestMain(m *testing.M) {
	if handled, code := process.HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}
