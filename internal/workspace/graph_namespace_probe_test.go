package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/project"
)

func graphNamespaceProbeFixture(t *testing.T) (*Manager, string, GraphNamespaceProbeRequest) {
	t.Helper()
	m, repo, _ := fixture(t)
	snapshot, err := m.Capture(context.Background(), Request{AttemptID: domain.NewID(), ProjectPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	physical, err := project.Inspect(context.Background(), snapshot.WorkspacePath, snapshot.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	return m, repo, GraphNamespaceProbeRequest{OperationAttemptID: snapshot.AttemptID, Snapshot: snapshot, WorkspaceIdentity: physical.Identity}
}

func graphNamespaceNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

// Observe the real directory with independent test-owned names, without GOOS
// assumptions, mount flag inference or a synthetic expected capability.
func graphNamespaceObservedCase(t *testing.T, path string) bool {
	t.Helper()
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	const lowerName = ".council-test-namespace-case-a"
	upperName := strings.ToUpper(lowerName)
	lower, err := root.OpenFile(lowerName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	var upper *os.File
	createdUpper := false
	defer func() {
		if upper != nil {
			if err := upper.Close(); err != nil {
				t.Error(err)
			}
		}
		if err := lower.Close(); err != nil {
			t.Error(err)
		}
		if createdUpper {
			if err := root.Remove(upperName); err != nil {
				t.Error(err)
			}
		}
		if err := root.Remove(lowerName); err != nil {
			t.Error(err)
		}
	}()
	original, err := lower.Stat()
	if err != nil {
		t.Fatal(err)
	}
	upper, err = root.Open(upperName)
	if err == nil {
		alias, err := upper.Stat()
		if err != nil || !os.SameFile(original, alias) {
			t.Fatal("The real uppercase lookup was neither absent nor the same file", err)
		}
		return false
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	upper, err = root.OpenFile(upperName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	createdUpper = true
	distinct, err := upper.Stat()
	if err != nil || os.SameFile(original, distinct) {
		t.Fatal("The real exclusive uppercase name did not create a distinct file", err)
	}
	return true
}

func TestGraphNamespaceProbeObservesActualDirectoryAndPreservesTreeAndIndex(t *testing.T) {
	m, repo, request := graphNamespaceProbeFixture(t)
	want := graphNamespaceObservedCase(t, request.Snapshot.WorkspacePath)
	beforeNames := graphNamespaceNames(t, request.Snapshot.WorkspacePath)
	beforeIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	beforeControl, err := os.ReadFile(filepath.Join(request.Snapshot.WorkspacePath, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	beforeSource, err := os.ReadFile(filepath.Join(repo, "file.bin"))
	if err != nil {
		t.Fatal(err)
	}
	beforeManaged, err := os.ReadFile(filepath.Join(request.Snapshot.WorkspacePath, "file.bin"))
	if err != nil {
		t.Fatal(err)
	}
	managedIndexPath, err := m.value(context.Background(), request.Snapshot.WorkspacePath, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		t.Fatal(err)
	}
	beforeManagedIndex, err := os.ReadFile(filepath.FromSlash(managedIndexPath))
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.ProbeGraphNamespace(context.Background(), request)
	if err != nil || result.CaseSensitive == nil || *result.CaseSensitive != want || result.Scope != "ascii_case_in_exact_directory" ||
		result.State != "complete" || !result.EffectsKnown || result.EvidenceStatus != "complete" || result.EffectsArtifact == nil || result.WorkspaceIdentity != request.WorkspaceIdentity {
		t.Fatal("The actual private-directory case observation is unavailable or unacknowledged", result, err)
	}
	wantEffects := "2" // One actual sentinel create and its acknowledged removal.
	if want {
		wantEffects = "4"
	}
	if result.EffectCount != wantEffects {
		t.Fatal("Probe hid or invented sentinel mutations", result.EffectCount, wantEffects)
	}
	body, err := m.graphIndexLoad(context.Background(), *result.EffectsArtifact, "graph_operation_effects")
	if err != nil {
		t.Fatal(err)
	}
	var effects struct {
		Contract           string `json:"contract"`
		OperationAttemptID string `json:"operationAttemptId"`
		WorkspaceID        string `json:"workspaceId"`
		State              string `json:"state"`
		CaseSensitive      *bool  `json:"caseSensitive"`
		LowerRemoved       bool   `json:"lowerRemoved"`
		UpperRemoved       bool   `json:"upperRemoved"`
	}
	if err = json.Unmarshal(body, &effects); err != nil || effects.Contract != "006-namespace-probe-v1" || effects.OperationAttemptID != request.OperationAttemptID ||
		effects.WorkspaceID != request.Snapshot.ID || effects.State != "complete" || effects.CaseSensitive == nil || *effects.CaseSensitive != want ||
		!effects.LowerRemoved || !effects.UpperRemoved {
		t.Fatal("The closed effect record did not acknowledge exact sentinel cleanup", effects, err)
	}
	if bytes.Contains(body, []byte(request.WorkspaceIdentity)) || bytes.Contains(body, []byte(request.Snapshot.WorkspacePath)) {
		t.Fatal("Private native identity/path leaked into the referenced effects artifact")
	}
	if after := graphNamespaceNames(t, request.Snapshot.WorkspacePath); !reflect.DeepEqual(after, beforeNames) {
		t.Fatal("The capability probe left new or missing workspace names", beforeNames, after)
	}
	afterIndex, indexErr := os.ReadFile(filepath.Join(repo, ".git", "index"))
	afterSource, sourceErr := os.ReadFile(filepath.Join(repo, "file.bin"))
	afterControl, controlErr := os.ReadFile(filepath.Join(request.Snapshot.WorkspacePath, ".git"))
	afterManaged, managedErr := os.ReadFile(filepath.Join(request.Snapshot.WorkspacePath, "file.bin"))
	afterManagedIndex, managedIndexErr := os.ReadFile(filepath.FromSlash(managedIndexPath))
	if indexErr != nil || sourceErr != nil || controlErr != nil || managedErr != nil || managedIndexErr != nil ||
		!bytes.Equal(beforeIndex, afterIndex) || !bytes.Equal(beforeSource, afterSource) || !bytes.Equal(beforeControl, afterControl) ||
		!bytes.Equal(beforeManaged, afterManaged) || !bytes.Equal(beforeManagedIndex, afterManagedIndex) {
		t.Fatal("The probe changed user source/index or managed working/index/control bytes", indexErr, sourceErr, controlErr, managedErr, managedIndexErr)
	}
	if err = project.ValidateIdentity(request.Snapshot.WorkspacePath, request.WorkspaceIdentity); err != nil {
		t.Fatal(err)
	}
}

func TestGraphNamespaceProbeRejectsCopiedControlDirectoryReplacement(t *testing.T) {
	m, _, request := graphNamespaceProbeFixture(t)
	snapshot := request.Snapshot
	control, err := os.ReadFile(filepath.Join(snapshot.WorkspacePath, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	relative := "workspaces/" + snapshot.ID
	if err = m.root.Rename(relative, relative+"-retained"); err != nil {
		t.Fatal(err)
	}
	if err = m.privateDir(relative); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(snapshot.WorkspacePath, ".git"), control, 0600); err != nil {
		t.Fatal(err)
	}
	if err = project.ValidateIdentity(snapshot.WorkspacePath, request.WorkspaceIdentity); err == nil {
		t.Fatal("The controlled replacement did not change actual directory identity")
	}
	before := graphNamespaceNames(t, snapshot.WorkspacePath)
	result, err := m.ProbeGraphNamespace(context.Background(), request)
	if err == nil || result.CaseSensitive != nil || result.EffectsArtifact != nil || result.EffectCount != "0" || !result.EffectsKnown {
		t.Fatal("A copied .git control bypassed the exact physical workspace identity", result, err)
	}
	if after := graphNamespaceNames(t, snapshot.WorkspacePath); !reflect.DeepEqual(before, after) {
		t.Fatal("Rejected replacement acquired sentinel effects", before, after)
	}
}

type graphNamespaceTerminalAckSink struct {
	base    *fixtureSink
	after   bool
	closed  int
	faulted bool
}

func (s *graphNamespaceTerminalAckSink) LoadArtifact(ctx context.Context, id string) (evidence.Metadata, error) {
	return s.base.LoadArtifact(ctx, id)
}

func (s *graphNamespaceTerminalAckSink) SaveArtifact(ctx context.Context, meta evidence.Metadata) error {
	if meta.Kind == "graph_operation_effects" && meta.Closed && meta.Completeness == "complete" {
		s.closed++
		if s.closed == 2 {
			s.faulted = true
			if s.after {
				if err := s.base.SaveArtifact(ctx, meta); err != nil {
					return err
				}
			}
			return domain.Fail("write_uncertain", "Controlled namespace terminal metadata acknowledgement was lost.")
		}
	}
	return s.base.SaveArtifact(ctx, meta)
}

func TestGraphNamespaceProbeLostTerminalAcknowledgementRetainsIntentWithoutCapability(t *testing.T) {
	for _, after := range []bool{false, true} {
		name := "before-metadata-commit"
		if after {
			name = "after-metadata-commit"
		}
		t.Run(name, func(t *testing.T) {
			m, _, request := graphNamespaceProbeFixture(t)
			sink := &graphNamespaceTerminalAckSink{base: &fixtureSink{rows: map[string]evidence.Metadata{}}, after: after}
			artifacts, err := evidence.New(m.lock, sink)
			if err != nil {
				t.Fatal(err)
			}
			prior := m.artifacts
			m.artifacts = artifacts
			t.Cleanup(func() {
				m.artifacts = prior
				if err := artifacts.Close(); err != nil {
					t.Error(err)
				}
			})
			before := graphNamespaceNames(t, request.Snapshot.WorkspacePath)
			result, err := m.ProbeGraphNamespace(context.Background(), request)
			if err == nil || !sink.faulted || result.CaseSensitive != nil || result.State != "uncertain" || result.EvidenceStatus != "uncertain" || result.EffectsKnown || result.EffectsArtifact == nil {
				t.Fatal("A lost terminal acknowledgement became a usable namespace capability", result, sink.faulted, err)
			}
			// Both cases reach real sentinel I/O and cleanup. The unresolved fact is
			// metadata acknowledgement, not an invented leftover name or process.
			if result.EffectCount != "2" && result.EffectCount != "4" {
				t.Fatal("Lost acknowledgement discarded actual sentinel effects", result)
			}
			if afterNames := graphNamespaceNames(t, request.Snapshot.WorkspacePath); !reflect.DeepEqual(before, afterNames) {
				t.Fatal("The terminal fault test failed before actual sentinel cleanup", before, afterNames)
			}
			body, readErr := m.graphIndexLoad(context.Background(), *result.EffectsArtifact, "graph_operation_effects")
			if readErr != nil || !bytes.Contains(body, []byte("\"state\":\"intent\"")) {
				t.Fatal("The exact acknowledged pre-I/O intent was not retained", readErr)
			}
		})
	}
}
