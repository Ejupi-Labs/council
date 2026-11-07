package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGraphRootProofVerifiesEveryUnchangedClosedMember(t *testing.T) {
	m, repo, _ := fixture(t)
	root, err := m.CaptureGraphRoot(context.Background(), graphRootRequest(t, repo, repo))
	if err != nil {
		t.Fatal(err)
	}
	proof, err := m.VerifyGraphRoot(context.Background(), root.Binding, root.Snapshot)
	if err != nil || len(proof.Artifacts) < 3 || !sameManifest(proof.Manifest, root.Snapshot.Manifest) {
		t.Fatal("actual fresh graph root closure proof unavailable", err)
	}
	var member string
	for _, entry := range root.Snapshot.Manifest.Entries {
		if entry.Path == ".gitignore" {
			member = entry.ArtifactID
		}
	}
	meta, err := m.artifacts.VerifyClosed(context.Background(), member)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(m.lock.Root(), filepath.FromSlash(meta.RelativePath)), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteAt([]byte("X"), 0)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	if _, err = m.VerifyGraphRoot(context.Background(), root.Binding, root.Snapshot); err == nil {
		t.Fatal("corrupt unchanged member survived valid root manifest/index hashes")
	}
}
