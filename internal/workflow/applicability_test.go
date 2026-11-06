package workflow

import (
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestApplicabilityReusesOnlyUnchangedCapturedSubsetAndResponseBytes(t *testing.T) {
	in := ApplicabilityInput{Scope: domain.CheckScope{Kind: "declared_subset", Paths: []string{"docs/note.md"}}, ConditionKeys: []string{"task-instructions"}, DefinitionSHA256: strings.Repeat("a", 64), ConditionsSHA256: strings.Repeat("b", 64), SourceControlSHA256: strings.Repeat("c", 64), Files: []ProofFile{proofFile("docs/note.md", []byte("same")), proofFile("other.txt", []byte("old"))}, Index: []ProofIndex{}}
	before, err := Basis(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Files[1] = proofFile("other.txt", []byte("later user bytes"))
	after, err := Basis(in)
	if err != nil || !Applicable(before, after) {
		t.Fatal("unaffected explicit subset could not be reused", err)
	}
	in.Files[0] = proofFile("docs/note.md", []byte("changed"))
	after, err = Basis(in)
	if err != nil || Applicable(before, after) {
		t.Fatal("changed inspected bytes reused prior proof", err)
	}
	in.Scope = domain.CheckScope{Kind: "whole_response", Paths: []string{}}
	in.Response = &domain.ResponseBinding{ResponseSHA256: strings.Repeat("d", 64), ResponseByteLength: "42", ManifestSHA256: strings.Repeat("e", 64), IndexSHA256: strings.Repeat("f", 64), ConditionsSHA256: in.ConditionsSHA256}
	before, err = Basis(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Response.ResponseSHA256 = strings.Repeat("1", 64)
	after, err = Basis(in)
	if err != nil || Applicable(before, after) {
		t.Fatal("changed response reused stale review while files stayed unchanged", err)
	}
}
