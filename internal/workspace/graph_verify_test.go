package workspace

import "testing"

func TestGraphManifestDigestAcceptsObservedStateWithoutArtifactOwnership(t *testing.T) {
	retained := Manifest{Entries: []Entry{{Path: "note.txt", Type: "file", Mode: 0644, Size: 3, SHA256: hashBytes([]byte("abc")), ArtifactID: "1234567890abcdef1234567890abcdef"}}, Index: []IndexEntry{}, Exclusions: []Exclusion{}}
	observed := retained
	observed.Entries = append([]Entry{}, retained.Entries...)
	observed.Entries[0].ArtifactID = ""
	a, ai, err := GraphManifestDigests(retained)
	if err != nil {
		t.Fatal(err)
	}
	b, bi, err := GraphManifestDigests(observed)
	if err != nil || a != b || ai != bi {
		t.Fatal("actual observed tuple incorrectly requires retained artifact identity", err)
	}
	observed.Entries[0].SHA256 = hashBytes([]byte("xyz"))
	c, _, err := GraphManifestDigests(observed)
	if err != nil || c == a {
		t.Fatal("semantic content change disappeared", err)
	}
}
