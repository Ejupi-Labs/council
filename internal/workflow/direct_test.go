package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func proofFile(path string, data []byte) ProofFile {
	sum := sha256.Sum256(data)
	return ProofFile{Path: path, Type: "file", Mode: 0644, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Bytes: data}
}
func directFixture() DocumentProofInput {
	return DocumentProofInput{Original: `Replace "old" with "new" in docs/note.md only`, Before: []ProofFile{proofFile("docs/note.md", []byte("\xef\xbb\xbfCONTROLLED LOCAL WORKFLOW TEST DATA\r\nold\r\n東京\r\n")), proofFile("unrelated.txt", []byte("user work"))}, After: []ProofFile{proofFile("docs/note.md", []byte("\xef\xbb\xbfCONTROLLED LOCAL WORKFLOW TEST DATA\r\nnew\r\n東京\r\n")), proofFile("unrelated.txt", []byte("user work"))}, BeforeIndex: []ProofIndex{}, AfterIndex: []ProofIndex{}}
}

func TestDirectRejectsUnrelatedDeltaDespiteCorrectRequestedReplacement(t *testing.T) {
	in := directFixture()
	in.After[1] = proofFile("unrelated.txt", []byte("destroyed user work"))
	if err := ProveDocument(in); err == nil {
		t.Fatal("direct proof accepted an extra unauthorized delta")
	}
}

func TestDirectProvesOnlyExactCompleteReplacement(t *testing.T) {
	if err := ProveDocument(directFixture()); err != nil {
		t.Fatal("supported exact replacement did not qualify", err)
	}
	cases := map[string]func(*DocumentProofInput){
		"line endings": func(in *DocumentProofInput) {
			in.After[0] = proofFile("docs/note.md", []byte("CONTROLLED LOCAL WORKFLOW TEST DATA\nnew\n東京\n"))
		},
		"multiple literals": func(in *DocumentProofInput) {
			in.Before[0] = proofFile("docs/note.md", []byte("old old"))
			in.After[0] = proofFile("docs/note.md", []byte("new old"))
		},
		"mode": func(in *DocumentProofInput) { in.After[0].Mode = 0755 },
		"index": func(in *DocumentProofInput) {
			in.AfterIndex = []ProofIndex{{Path: "staged.txt", Mode: "100644", Object: "object"}}
		},
		"extra instruction": func(in *DocumentProofInput) { in.Original += " Also run a build." },
		"guidance":          func(in *DocumentProofInput) { in.Version.GuidanceText = "and change another file" },
		"hash":              func(in *DocumentProofInput) { in.After[0].SHA256 = in.Before[0].SHA256 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			in := directFixture()
			change(&in)
			if ProveDocument(in) == nil {
				t.Fatal("unsupported or incomplete proof qualified")
			}
		})
	}
}
