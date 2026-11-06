// Package discovery observes installation metadata without ever opening launcher contents or
// starting a vendor process. Installation evidence does not qualify a working adapter.
package discovery

import (
	"context"
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

const source = "path_and_native_install_metadata"

type DiscoveryResult = domain.DiscoveryResult
type ConnectionObservation = domain.ConnectionObservation
type DiscoveryEntry = domain.DiscoveryEntry
type harness struct{ id, name string }

var harnesses = []harness{{"codex", "codex"}, {"claude-code", "claude"}, {"deepseek-harness", "dsh"}, {"junie", "junie"}, {"antigravity", "agy"}}

// Scanner allows deterministic filesystem-only fixtures without vendor/global state.
// Native returns only documented install-registration candidate paths for this executable.
type Scanner struct {
	Path   string
	Native func(harnessID string) ([]string, error)
}

func Discover(ctx context.Context) (DiscoveryResult, error) {
	return (Scanner{Path: os.Getenv("PATH"), Native: nativeCandidates}).Discover(ctx)
}

func (s Scanner) Discover(ctx context.Context) (DiscoveryResult, error) {
	var result DiscoveryResult
	if len(s.Path) > 1024*1024 || !utf8.ValidString(s.Path) || strings.ContainsRune(s.Path, 0) {
		return result, domain.Fail("discovery_unavailable", "PATH cannot be inspected safely. Check the local executable search path.")
	}
	dirs := filepath.SplitList(s.Path)
	if len(dirs) > 2048 {
		return result, domain.Fail("discovery_unavailable", "PATH contains too many search locations to inspect.")
	}
	stamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	result = DiscoveryResult{ObservedAt: stamp, Source: source, Observations: make([]DiscoveryEntry, 0, 5)}
	for _, h := range harnesses {
		if err := ctx.Err(); err != nil {
			return DiscoveryResult{}, domain.Fail("discovery_unavailable", "Installation inspection was interrupted. Refresh to inspect again.")
		}
		path, status, err := s.find(ctx, h, dirs)
		e := DiscoveryEntry{HarnessID: h.id}
		if err != nil {
			e.Error = domain.Fail("discovery_unavailable", "Cannot inspect this harness's installation metadata. Check filesystem access and refresh.")
		} else {
			action := "install_with_vendor"
			if status == "installed" {
				action = "await_managed_execution"
			} else if status == "unsupported_launcher" {
				action = "review_launcher_support"
			}
			managed := "not_implemented"
			if h.id == "codex" {
				managed = "native_supported_live_unqualified"
				if status == "installed" {
					action = "start_explicit_task"
				}
			}
			e.Observation = &ConnectionObservation{HarnessID: h.id, InstallationStatus: status, LauncherPath: path, Source: source, ObservedAt: stamp, ManagedExecution: managed, Authentication: "unknown", Provider: "unknown", Account: "unknown", Model: "unknown", EffectivePermissions: "unknown", ExternalOccupancy: "unknown", Allowance: "unknown", Usage: "unknown", NextAction: action}
		}
		result.Observations = append(result.Observations, e)
	}
	return result, nil
}

func (s Scanner) find(ctx context.Context, h harness, dirs []string) (string, string, error) {
	var candidates []string
	// Native executable candidates have priority over shell-only wrappers. Inspection
	// follows filesystem metadata only; no version/help/auth/model probe is permitted.
	names := []string{h.name}
	if runtime.GOOS == "windows" {
		names = []string{h.name + ".exe", h.name + ".com"}
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if !filepath.IsAbs(dir) {
			var err error
			dir, err = filepath.Abs(dir)
			if err != nil {
				return "", "", err
			}
		}
		for _, name := range names {
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}
	var scanErr error
	var wrapper string
	if s.Native != nil {
		native, err := s.Native(h.id)
		if err != nil {
			scanErr = err
		}
		if len(native) > 64 {
			return "", "", errors.New("too many native candidates")
		}
		candidates = append(candidates, native...)
	}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		if !filepath.IsAbs(candidate) || !utf8.ValidString(candidate) || strings.ContainsRune(candidate, 0) {
			scanErr = errors.New("invalid native candidate")
			continue
		}
		ok, err := executableMetadata(candidate)
		if err != nil {
			scanErr = err
			continue
		}
		if ok {
			ext := strings.ToLower(filepath.Ext(candidate))
			if ext == ".cmd" || ext == ".bat" || ext == ".ps1" || ext == ".sh" {
				if wrapper == "" {
					wrapper = candidate
				}
				continue
			}
			return candidate, "installed", nil
		}
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if !filepath.IsAbs(dir) {
			var err error
			dir, err = filepath.Abs(dir)
			if err != nil {
				scanErr = err
				continue
			}
		}
		for _, suffix := range []string{".cmd", ".bat", ".ps1", ".sh"} {
			candidate := filepath.Join(dir, h.name+suffix)
			i, err := os.Stat(candidate)
			if err == nil && i.Mode().IsRegular() {
				if wrapper == "" {
					wrapper = candidate
				}
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				scanErr = err
			}
		}
	}
	if wrapper != "" {
		return wrapper, "unsupported_launcher", nil
	}
	if scanErr != nil {
		return "", "", scanErr
	}
	return "", "missing", nil
}

func executableMetadata(path string) (bool, error) {
	i, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !i.Mode().IsRegular() {
		return false, nil
	}
	if runtime.GOOS != "windows" && i.Mode().Perm()&0111 == 0 {
		return false, nil
	}
	return true, nil
}
