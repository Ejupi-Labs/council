package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestGraphEdgeIDUsesFrozenVersionedSemanticTuple(t *testing.T) {
	graphID, from, to := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	// The literal wire-order fixture independently fixes the canonical field
	// sequence; sorted map-key serialization is not an equivalent encoding.
	canonical := `{"graphId":"11111111111111111111111111111111","graphVersion":"7","fromNodeId":"22222222222222222222222222222222","toNodeId":"33333333333333333333333333333333","kind":"files"}`
	sum := sha256.Sum256([]byte(canonical))
	want := hex.EncodeToString(sum[:16])
	got, err := GraphEdgeID(graphID, "7", from, to, "files")
	if err != nil || got != want || !ValidID(got) {
		t.Fatal("Stable edge identity does not match the frozen tuple/field order", got, want, err)
	}
	for _, tuple := range [][5]string{
		{strings.Repeat("4", 32), "7", from, to, "files"},
		{graphID, "8", from, to, "files"},
		{graphID, "7", strings.Repeat("4", 32), to, "files"},
		{graphID, "7", from, strings.Repeat("4", 32), "files"},
		{graphID, "7", from, to, "context"},
	} {
		changed, err := GraphEdgeID(tuple[0], tuple[1], tuple[2], tuple[3], tuple[4])
		if err != nil || changed == got || !ValidID(changed) {
			t.Fatal("A semantic edge component was omitted from identity", tuple, changed, err)
		}
	}
}

func TestGraphEdgeIDRejectsNoncanonicalIdentityVersionAndKind(t *testing.T) {
	good := [5]string{strings.Repeat("a", 32), "1", strings.Repeat("b", 32), strings.Repeat("c", 32), "files"}
	for _, item := range []struct {
		index int
		value string
	}{{0, strings.Repeat("A", 32)}, {0, "../graph"}, {1, "0"}, {1, "01"}, {1, "9223372036854775808"}, {2, ""}, {3, "node-key"}, {4, ""}, {4, "FILES"}, {4, "merge"}} {
		tuple := good
		tuple[item.index] = item.value
		if id, err := GraphEdgeID(tuple[0], tuple[1], tuple[2], tuple[3], tuple[4]); err == nil || id != "" {
			t.Fatal("Noncanonical edge tuple acquired an identity", tuple, id, err)
		}
	}
}
