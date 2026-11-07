package workspace

import (
	"sort"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphComposePreservesSupportedSymlinkAncestorTarget(t *testing.T) {
	root, _ := composeRoot(t)
	owner := root.Binding.OperationAttemptID
	manifest, staged := composeManifest(owner, map[string]string{"real/file.txt": "controlled target\n"}, map[string]string{"real/file.txt": "controlled target\n"})
	manifest.Entries = append(manifest.Entries, Entry{Path: "alias", Type: "symlink", Target: "real", Size: int64(len("real"))}, Entry{Path: "via.txt", Type: "symlink", Target: "alias/file.txt", Size: int64(len("alias/file.txt"))})
	sort.Slice(manifest.Entries, func(i, j int) bool { return manifest.Entries[i].Path < manifest.Entries[j].Path })
	root.Manifest, root.StagedBlobs = manifest, staged
	root.Binding.Manifest = composeRef(owner, "symlink-root-manifest", composeJSON(t, manifest))
	root.Binding.StagedBlobs = composeRef(owner, "symlink-root-staged", composeJSON(t, staged))
	var err error
	root.Binding.BasisSHA256, err = domain.GraphRootDigest(root.Binding)
	if err != nil {
		t.Fatal(err)
	}
	plan, conflicts, err := ComposeGraphInput(composeRequest(root))
	if err != nil || len(conflicts) != 0 || !sameManifest(plan.Manifest, manifest) {
		t.Fatal("unchanged supported symlink-ancestor target was rejected", err, conflicts)
	}
}

func TestGraphComposeResolvesUnambiguousSymlinkTargetUsingDeclaredNativeCasePolicy(t *testing.T) {
	root, _ := composeRoot(t)
	owner := root.Binding.OperationAttemptID
	manifest, staged := composeManifest(owner, map[string]string{"real/file.txt": "controlled target\n"}, map[string]string{"real/file.txt": "controlled target\n"})
	manifest.Entries = append(manifest.Entries, Entry{Path: "via.txt", Type: "symlink", Target: "REAL/FILE.txt", Size: int64(len("REAL/FILE.txt"))})
	sort.Slice(manifest.Entries, func(i, j int) bool { return manifest.Entries[i].Path < manifest.Entries[j].Path })
	root.Manifest, root.StagedBlobs = manifest, staged
	root.Binding.Manifest = composeRef(owner, "native-case-root-manifest", composeJSON(t, manifest))
	root.Binding.StagedBlobs = composeRef(owner, "native-case-root-staged", composeJSON(t, staged))
	var err error
	root.Binding.BasisSHA256, err = domain.GraphRootDigest(root.Binding)
	if err != nil {
		t.Fatal(err)
	}
	request := composeRequest(root)
	request.CaseSensitive = false
	plan, conflicts, err := ComposeGraphInput(request)
	if err != nil || len(conflicts) != 0 || !sameManifest(plan.Manifest, manifest) {
		t.Fatal("unambiguous native-insensitive target was rejected or rewritten", err, conflicts)
	}
	request.CaseSensitive = true
	plan, conflicts, err = ComposeGraphInput(request)
	if err != nil || len(conflicts) == 0 || len(plan.Manifest.Entries) != 0 {
		t.Fatal("case-sensitive missing target was silently reinterpreted", err, conflicts)
	}
}
