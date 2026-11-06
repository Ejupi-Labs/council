package adapters

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Ejupi-Labs/council/v2/internal/discovery"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func ResolveCodex(ctx context.Context, pins domain.ExecutionPins) (string, error) {
	return resolveHarness(ctx, "codex", "Codex")
}

func ResolveNative(ctx context.Context, pins domain.ExecutionPins) (string, error) {
	connection, err := domain.ResolveConnectionID(pins)
	if err != nil {
		return "", err
	}
	if connection == "claude-native" {
		return resolveHarness(ctx, "claude-code", "Claude Code")
	}
	return ResolveCodex(ctx, pins)
}

func resolveHarness(ctx context.Context, harnessID, name string) (string, error) {
	observed, err := discovery.Discover(ctx)
	if err != nil {
		return "", err
	}
	for _, entry := range observed.Observations {
		if entry.HarnessID != harnessID || entry.Observation == nil {
			continue
		}
		o := entry.Observation
		if o.InstallationStatus != "installed" {
			return "", domain.Fail("capability_unavailable", "Install a native "+name+" executable. Shell wrappers are not launched by this execution contract.")
		}
		if !filepath.IsAbs(o.LauncherPath) || !validNativeString(o.LauncherPath) {
			break
		}
		info, e := os.Stat(o.LauncherPath)
		if e == nil && info.Mode().IsRegular() {
			return o.LauncherPath, nil
		}
	}
	return "", domain.Fail("capability_unavailable", "A native "+name+" executable is unavailable. Install it using its vendor instructions and refresh discovery.")
}
