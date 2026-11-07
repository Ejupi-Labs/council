package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphNativeCompositionRequest(t *testing.T, f graphMaterializeCase, branches ...graphPhysicalContribution) GraphNativeInputRetainRequest {
	t.Helper()
	explicit := graphPhysicalComposeRequest(t, f, branches...)
	base := explicit.Base
	base.OperationAttemptID, base.InputID = domain.NewID(), domain.NewID()
	base.Context.InputID = base.InputID
	return GraphNativeInputRetainRequest{Base: base, Contributions: explicit.Contributions}
}

func graphNativeCompositionEffects(t *testing.T, m *Manager, result GraphComposedInputRetention) (*domain.GraphArtifactRef, *string) {
	t.Helper()
	if result.OperationEffects == nil {
		t.Fatal("The actual composition has no acknowledged operation effects")
	}
	body, err := m.graphIndexLoad(context.Background(), *result.OperationEffects, "graph_operation_effects")
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Contract       string                   `json:"contract"`
		WorkspaceID    *string                  `json:"workspaceId"`
		NamespaceProbe *domain.GraphArtifactRef `json:"namespaceProbe"`
	}
	if err = json.Unmarshal(body, &record); err != nil || record.Contract != "006-composed-input-effects-v1" {
		t.Fatal("Native composition did not retain its actual operation record", err)
	}
	return record.NamespaceProbe, record.WorkspaceID
}

// These are real closed filesystem contributions with deliberately synthetic
// acceptance tuples. Engine/Store separately qualify current accepted outcomes.
func TestGraphNativeCompositionUsesActualCaseOnlyWhenPlansDependOnIt(t *testing.T) {
	f := graphMaterializeFixture(t)
	a := graphPhysicalContributionFixture(t, f, 1, "branch-case.txt")
	b := graphPhysicalContributionFixture(t, f, 2, "branch-case.txt")
	c := graphPhysicalContributionFixture(t, f, 3, "BRANCH-CASE.TXT")
	originalIndex, err := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	originalWorking, err := os.ReadFile(filepath.Join(f.repo, "file.bin"))
	if err != nil {
		t.Fatal(err)
	}
	assertOriginal := func(t *testing.T) {
		t.Helper()
		index, indexErr := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
		working, workingErr := os.ReadFile(filepath.Join(f.repo, "file.bin"))
		if indexErr != nil || workingErr != nil || !bytes.Equal(index, originalIndex) || !bytes.Equal(working, originalWorking) {
			t.Fatal("Composed-input preparation changed the original working/index basis", indexErr, workingErr)
		}
	}
	t.Run("identical-policies-retain-once-without-probe", func(t *testing.T) {
		before := graphNamespaceNames(t, filepath.Join(f.m.lock.Root(), "workspaces"))
		request := graphNativeCompositionRequest(t, f, a)
		result, err := f.m.RetainNativeGraphInput(context.Background(), request)
		if err != nil || result.Binding == nil || result.Snapshot == nil || result.State != "complete" || !result.EffectsKnown ||
			result.EvidenceStatus != "complete" || result.Snapshot.AttemptID != request.Base.OperationAttemptID {
			t.Fatal("A case-independent closed contribution has no actual retained input", result, err)
		}
		probe, workspaceID := graphNativeCompositionEffects(t, f.m, result)
		if probe != nil || workspaceID == nil || *workspaceID != result.Snapshot.ID {
			t.Fatal("Case-independent plans performed a probe or lost actual assembly identity", probe, workspaceID)
		}
		if after := graphNamespaceNames(t, filepath.Join(f.m.lock.Root(), "workspaces")); len(after) != len(before)+1 {
			t.Fatal("The same input created more than one managed assembly", before, after)
		}
		verified, err := f.m.VerifyGraphInput(context.Background(), *result.Binding)
		if err != nil || len(verified.Manifest.Index) == 0 {
			t.Fatal("The retained case-independent input lacks complete staged closure", err)
		}
		body, err := os.ReadFile(filepath.Join(result.Snapshot.WorkspacePath, a.path))
		if err != nil || !bytes.Equal(body, a.working) {
			t.Fatal("Actual closed working bytes were not materialized", err)
		}
		if body = runGit(t, result.Snapshot.WorkspacePath, nil, "show", ":"+a.path); !bytes.Equal(body, a.staged) {
			t.Fatal("Actual staged bytes were replaced by working bytes")
		}
		assertOriginal(t)
	})
	t.Run("same-causal-conflict-has-no-workspace-or-probe", func(t *testing.T) {
		before := graphNamespaceNames(t, filepath.Join(f.m.lock.Root(), "workspaces"))
		result, err := f.m.RetainNativeGraphInput(context.Background(), graphNativeCompositionRequest(t, f, a, b))
		if err != nil || result.Binding != nil || result.Snapshot != nil || result.State != "conflicted" || result.ConflictArtifact == nil ||
			len(result.Conflicts) == 0 || !result.EffectsKnown || result.EvidenceStatus != "complete" {
			t.Fatal("An unavoidable causal conflict became usable input or lost typed conflict evidence", result, err)
		}
		for _, conflict := range result.Conflicts {
			if conflict.Code != "input_conflict" || len(conflict.ContributionRefs) < 2 {
				t.Fatal("The actual divergent branch conflict lost its causal provenance", conflict)
			}
		}
		probe, workspaceID := graphNativeCompositionEffects(t, f.m, result)
		if probe != nil || workspaceID != nil {
			t.Fatal("A known causal conflict allocated/probed a workspace", probe, workspaceID)
		}
		if after := graphNamespaceNames(t, filepath.Join(f.m.lock.Root(), "workspaces")); !reflect.DeepEqual(before, after) {
			t.Fatal("Conflicted input created a managed assembly", before, after)
		}
		assertOriginal(t)
	})
	t.Run("case-dependent-plans-probe-and-reuse-exact-assembly", func(t *testing.T) {
		before := graphNamespaceNames(t, filepath.Join(f.m.lock.Root(), "workspaces"))
		request := graphNativeCompositionRequest(t, f, a, c)
		result, err := f.m.RetainNativeGraphInput(context.Background(), request)
		if err != nil || result.Snapshot == nil || !result.EffectsKnown || result.EvidenceStatus != "complete" ||
			result.Snapshot.AttemptID != request.Base.OperationAttemptID {
			t.Fatal("Case-dependent plans never reached an acknowledged actual assembly probe", result, err)
		}
		probe, workspaceID := graphNativeCompositionEffects(t, f.m, result)
		if probe == nil || workspaceID == nil || *workspaceID != result.Snapshot.ID || probe.ProducerAttemptID != request.Base.OperationAttemptID {
			t.Fatal("The native decision has no exact same-operation probe/assembly binding", probe, workspaceID)
		}
		body, err := f.m.graphIndexLoad(context.Background(), *probe, "graph_operation_effects")
		if err != nil {
			t.Fatal(err)
		}
		var observation struct {
			Contract      string `json:"contract"`
			WorkspaceID   string `json:"workspaceId"`
			Scope         string `json:"scope"`
			CaseSensitive *bool  `json:"caseSensitive"`
			State         string `json:"state"`
		}
		if err = json.Unmarshal(body, &observation); err != nil || observation.Contract != "006-namespace-probe-v1" ||
			observation.WorkspaceID != result.Snapshot.ID || observation.Scope != "ascii_case_in_exact_directory" || observation.CaseSensitive == nil || observation.State != "complete" {
			t.Fatal("The selected case policy was not observed in this exact assembly", observation, err)
		}
		// An independent post-operation check uses the SAME directory; this is not
		// an assumption that every volume/directory on this host has one policy.
		actualSensitive := graphNamespaceObservedCase(t, result.Snapshot.WorkspacePath)
		if *observation.CaseSensitive != actualSensitive {
			t.Fatal("The selected explicit policy disagrees with the actual assembly directory")
		}
		if after := graphNamespaceNames(t, filepath.Join(f.m.lock.Root(), "workspaces")); len(after) != len(before)+1 {
			t.Fatal("Probing and materialization used different/repeated managed assemblies", before, after)
		}
		if actualSensitive {
			if result.State != "complete" || result.Binding == nil || len(result.Conflicts) != 0 {
				t.Fatal("A supported native case-distinct tree was narrowed to a portable-only blocker", result)
			}
			if _, err = f.m.VerifyGraphInput(context.Background(), *result.Binding); err != nil {
				t.Fatal(err)
			}
			for _, branch := range []graphPhysicalContribution{a, c} {
				working, err := os.ReadFile(filepath.Join(result.Snapshot.WorkspacePath, branch.path))
				if err != nil || !bytes.Equal(working, branch.working) {
					t.Fatal("Native-selected working bytes changed or aliases were coalesced", branch.path, err)
				}
				if staged := runGit(t, result.Snapshot.WorkspacePath, nil, "show", ":"+branch.path); !bytes.Equal(staged, branch.staged) {
					t.Fatal("Native-selected staged bytes changed or aliases were coalesced", branch.path)
				}
			}
		} else {
			if result.State != "conflicted" || result.Binding != nil || result.ConflictArtifact == nil || len(result.Conflicts) == 0 {
				t.Fatal("An actual native case collision was silently materialized/selected", result)
			}
			for _, name := range []string{a.path, c.path} {
				if _, err := os.Lstat(filepath.Join(result.Snapshot.WorkspacePath, name)); !os.IsNotExist(err) {
					t.Fatal("The native conflict had file payload effects before its explicit policy was selected", name, err)
				}
			}
		}
		assertOriginal(t)
	})
}
