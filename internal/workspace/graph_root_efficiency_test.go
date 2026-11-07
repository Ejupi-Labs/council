package workspace

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

// This counts actual evidence opens, not a replacement verifier. Metadata comes
// from the real completed capture; the new evidence.Store still reads and hashes
// every referenced physical payload in the same owned artifact directory.
type graphRootReadCounter struct {
	mu    sync.Mutex
	rows  map[string]evidence.Metadata
	loads map[string]int
	saves int
}

func (s *graphRootReadCounter) LoadArtifact(ctx context.Context, id string) (evidence.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return evidence.Metadata{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads[id]++
	m, ok := s.rows[id]
	if !ok {
		return m, os.ErrNotExist
	}
	return m, nil
}

func (s *graphRootReadCounter) SaveArtifact(context.Context, evidence.Metadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	return errors.New("read-only root verification attempted an evidence write")
}

func (s *graphRootReadCounter) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads = map[string]int{}
	s.saves = 0
}

func graphRootEfficiencyFixture(t *testing.T) (*Manager, string, GraphRootCapture, GraphRootProof, *graphRootReadCounter, string) {
	t.Helper()
	m, repo, _ := fixture(t)
	put(t, repo, "file.bin", []byte("Unique retained staged bytes 東京\r\n\x00\xfd"))
	runGit(t, repo, nil, "add", "file.bin")
	put(t, repo, "file.bin", []byte("Distinct retained working bytes 🙂\r\n\x00\xfe"))
	root, err := m.CaptureGraphRoot(context.Background(), graphRootRequest(t, repo, repo))
	if err != nil || root.Snapshot == nil {
		t.Fatal("actual closed root prerequisite failed", err)
	}
	proof, err := m.VerifyGraphRoot(context.Background(), root.Binding, root.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	tracker := &graphRootReadCounter{rows: map[string]evidence.Metadata{}, loads: map[string]int{}}
	for _, ref := range proof.Artifacts {
		meta, err := m.artifacts.VerifyClosed(context.Background(), ref.ArtifactID)
		if err != nil || graphIndexRef(meta) != ref {
			t.Fatal("counted metadata is not the real retained tuple", ref, err)
		}
		tracker.rows[meta.ID] = meta
	}
	workingID, object := "", ""
	for _, entry := range proof.Manifest.Entries {
		if entry.Path == "file.bin" {
			workingID = entry.ArtifactID
		}
	}
	for _, entry := range proof.Manifest.Index {
		if entry.Path == "file.bin" {
			object = entry.Object
		}
	}
	stagedID := ""
	for _, blob := range proof.StagedBlobs.Entries {
		if blob.ObjectID == object {
			stagedID = blob.ArtifactRef.ArtifactID
		}
	}
	if workingID == "" || stagedID == "" || stagedID == workingID {
		t.Fatal("the physical fixture must retain distinct working and staged payloads", workingID, stagedID)
	}
	counted, err := evidence.New(m.lock, tracker)
	if err != nil {
		t.Fatal(err)
	}
	prior := m.artifacts
	m.artifacts = counted
	t.Cleanup(func() {
		m.artifacts = prior
		if err := counted.Close(); err != nil {
			t.Error(err)
		}
	})
	return m, repo, root, proof, tracker, stagedID
}

func TestGraphRootVerificationReadsBasisOnceWithinEachFreshCall(t *testing.T) {
	m, repo, root, expected, tracker, stagedID := graphRootEfficiencyFixture(t)
	indexBefore, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	for call := 1; call <= 2; call++ {
		tracker.reset()
		proof, err := m.VerifyGraphRoot(context.Background(), root.Binding, root.Snapshot)
		if err != nil || !reflect.DeepEqual(proof, expected) {
			t.Fatal("fresh verification changed the complete retained root inventory", call, err)
		}
		for _, id := range []string{root.Binding.Manifest.ArtifactID, root.Binding.Index.ArtifactID, root.Binding.StagedBlobs.ArtifactID, stagedID} {
			if tracker.loads[id] != 1 {
				t.Fatalf("fresh root call %d opened basis artifact %s %d times; want exactly one full physical proof in this call", call, id, tracker.loads[id])
			}
		}
		for _, ref := range proof.Artifacts {
			if tracker.loads[ref.ArtifactID] == 0 {
				t.Fatal("returned proof inventory contains an unverified physical member", call, ref)
			}
		}
		if tracker.saves != 0 {
			t.Fatal("fresh read-only verification wrote evidence", tracker.saves)
		}
	}
	indexAfter, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil || !bytes.Equal(indexBefore, indexAfter) {
		t.Fatal("read-only root verification changed the user's raw index", err)
	}
}

func TestGraphRootVerificationRechecksWorkingAndStagedMembersOnEveryCall(t *testing.T) {
	m, _, root, proof, tracker, stagedID := graphRootEfficiencyFixture(t)
	workingID := ""
	for _, entry := range proof.Manifest.Entries {
		if entry.Path == "file.bin" {
			workingID = entry.ArtifactID
		}
	}
	for _, member := range []struct{ name, id string }{{"working", workingID}, {"staged", stagedID}} {
		t.Run(member.name, func(t *testing.T) {
			if _, err := m.VerifyGraphRoot(context.Background(), root.Binding, root.Snapshot); err != nil {
				t.Fatal("the previous fresh call did not verify the actual member", err)
			}
			meta := tracker.rows[member.id]
			path := filepath.Join(m.lock.Root(), filepath.FromSlash(meta.RelativePath))
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil || len(body) == 0 {
				t.Fatal("the actual retained payload is unavailable", err)
			}
			write := func(value byte) {
				t.Helper()
				file, err := os.OpenFile(path, os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, writeErr := file.WriteAt([]byte{value}, 0)
				syncErr, closeErr := file.Sync(), file.Close()
				if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
					t.Fatal(err)
				}
			}
			write(body[0] ^ 1)
			defer write(body[0])
			after, err := os.Stat(path)
			if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() {
				t.Fatal("the controlled fault did not preserve the actual payload identity and size", err)
			}
			tracker.reset()
			actual, err := m.VerifyGraphRoot(context.Background(), root.Binding, root.Snapshot)
			if !errors.Is(err, evidence.ErrCorrupt) || len(actual.Artifacts) != 0 || tracker.loads[member.id] == 0 {
				t.Fatal("a later call reused a prior proof instead of refusing changed physical bytes", member.name, actual, err, tracker.loads)
			}
		})
	}
}

func TestGraphRootVerificationKeepsExactPrivateSourceGuardBeforePayloadReads(t *testing.T) {
	m, _, root, _, tracker, _ := graphRootEfficiencyFixture(t)
	wrong := *root.Snapshot
	wrong.BaseCommit = strings.Repeat("0", 40)
	tracker.reset()
	proof, err := m.VerifyGraphRoot(context.Background(), root.Binding, &wrong)
	if err == nil || len(proof.Artifacts) != 0 || len(tracker.loads) != 0 || tracker.saves != 0 {
		t.Fatal("root content reuse bypassed its exact retained private source guard", proof, err, tracker.loads)
	}
}
