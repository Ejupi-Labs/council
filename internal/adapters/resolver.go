package adapters

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Ejupi-Labs/council/v2/internal/discovery"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func ResolveCodex(ctx context.Context, pins domain.ExecutionPins) (string, error) {
	observed, err := discovery.Discover(ctx)
	if err != nil {
		return "", err
	}
	for _, entry := range observed.Observations {
		if entry.HarnessID != "codex" || entry.Observation == nil {
			continue
		}
		o := entry.Observation
		if o.InstallationStatus != "installed" {
			return "", domain.Fail("capability_unavailable", "Install a native Codex executable. Shell wrappers are not launched by this execution contract.")
		}
		if !filepath.IsAbs(o.LauncherPath) || !validNativeString(o.LauncherPath) {
			break
		}
		info, e := os.Stat(o.LauncherPath)
		if e == nil && info.Mode().IsRegular() {
			return o.LauncherPath, nil
		}
	}
	return "", domain.Fail("capability_unavailable", "A native Codex executable is unavailable. Install it using its vendor instructions and refresh discovery.")
}
