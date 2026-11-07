package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestGraphContributorVectorDigestKeepsCanonicalArrayAndInput(t *testing.T) {
	const empty = "4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"
	for _, vector := range [][]GraphContributorBinding{nil, {}} {
		if digest, err := GraphContributorVectorDigest(vector); err != nil || digest != empty {
			t.Fatal("empty contributor seal is not the canonical typed array", digest, err)
		}
	}
	a, b := GraphContributorBinding{NodeID: strings.Repeat("a", 32)}, GraphContributorBinding{NodeID: strings.Repeat("b", 32)}
	vector := []GraphContributorBinding{b, a}
	before := append([]GraphContributorBinding{}, vector...)
	digest, err := GraphContributorVectorDigest(vector)
	ordered, orderedErr := GraphContributorVectorDigest([]GraphContributorBinding{a, b})
	if err != nil || orderedErr != nil || digest != ordered || !reflect.DeepEqual(vector, before) {
		t.Fatal("contributor seal depends on order or mutates the caller", digest, ordered, vector, err, orderedErr)
	}
	vector[0].AcceptanceID = strings.Repeat("c", 32)
	changed, err := GraphContributorVectorDigest(vector)
	if err != nil || changed == digest {
		t.Fatal("changed acceptance retained the old vector seal", changed, err)
	}
}
