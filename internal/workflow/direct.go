package workflow

import (
	"bytes"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type DocumentOperation struct{ Path, Old, New string }
type ProofFile struct {
	Path, Type     string
	Mode           uint32
	Size           int64
	SHA256, Target string
	Bytes          []byte
}
type ProofIndex struct{ Path, Mode, Object string }
type DocumentProofInput struct {
	Original                string
	Version                 domain.TaskVersion
	Before, After           []ProofFile
	BeforeIndex, AfterIndex []ProofIndex
}

var replaceFirst = regexp.MustCompile(`(?i)^replace[ \t]+"([^"\r\n]+)"[ \t]+with[ \t]+"([^"\r\n]+)"[ \t]+in[ \t]+([^ \t\r\n]+)[ \t]+only\.?$`)
var replaceLast = regexp.MustCompile(`(?i)^in[ \t]+([^ \t\r\n]+)[ \t]+only,[ \t]+replace[ \t]+"([^"\r\n]+)"[ \t]+with[ \t]+"([^"\r\n]+)"\.?$`)

func RecognizeDocument(original string, version domain.TaskVersion) (DocumentOperation, bool) {
	var op DocumentOperation
	if len(original) > 2048 || !utf8.ValidString(original) || version.GuidanceText != "" || version.Acceptance.ConditionsText != "" || len(version.Acceptance.RequiredEvidence) != 0 {
		return op, false
	}
	if m := replaceFirst.FindStringSubmatch(strings.TrimSpace(original)); m != nil {
		op = DocumentOperation{Path: m[3], Old: m[1], New: m[2]}
	} else if m := replaceLast.FindStringSubmatch(strings.TrimSpace(original)); m != nil {
		op = DocumentOperation{Path: m[1], Old: m[2], New: m[3]}
	} else {
		return op, false
	}
	if op.Old == op.New || len(op.Old) > 256 || len(op.New) > 256 || !documentPath(op.Path) {
		return DocumentOperation{}, false
	}
	for _, text := range []string{op.Old, op.New} {
		for _, r := range text {
			if unicode.IsControl(r) {
				return DocumentOperation{}, false
			}
		}
	}
	return op, true
}

func documentPath(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || path.Clean(value) != value || filepath.IsAbs(value) || strings.ContainsAny(value, "\\:*?[]{}~\x00") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.TrimRight(part, " .") != part {
			return false
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return false
			}
		}
		stem := strings.ToLower(strings.Split(part, ".")[0])
		if stem == "con" || stem == "prn" || stem == "aux" || stem == "nul" || len(stem) == 4 && (strings.HasPrefix(stem, "com") || strings.HasPrefix(stem, "lpt")) && stem[3] >= '0' && stem[3] <= '9' {
			return false
		}
	}
	switch strings.ToLower(path.Ext(value)) {
	case ".md", ".txt", ".rst":
		return true
	}
	return false
}

func ProveDocument(in DocumentProofInput) *domain.Error {
	fail := func() *domain.Error {
		return domain.Fail("evidence_unavailable", "The exact complete literal-document operation and unchanged surrounding/index basis were not proved. Retain the candidate for required verification.")
	}
	op, ok := RecognizeDocument(in.Original, in.Version)
	if !ok || len(in.Before) != len(in.After) || len(in.BeforeIndex) != len(in.AfterIndex) {
		return fail()
	}
	before := map[string]ProofFile{}
	after := map[string]ProofFile{}
	for _, entry := range in.Before {
		if entry.Path == "" || before[entry.Path].Path != "" {
			return fail()
		}
		before[entry.Path] = entry
	}
	for _, entry := range in.After {
		if entry.Path == "" || after[entry.Path].Path != "" {
			return fail()
		}
		after[entry.Path] = entry
	}
	for name, left := range before {
		right, exists := after[name]
		if !exists {
			return fail()
		}
		if name != op.Path {
			left.Bytes = nil
			right.Bytes = nil
			if left.Path != right.Path || left.Type != right.Type || left.Mode != right.Mode || left.Size != right.Size || left.SHA256 != right.SHA256 || left.Target != right.Target {
				return fail()
			}
			continue
		}
		if left.Type != "file" || right.Type != "file" || left.Mode != right.Mode || left.Target != "" || right.Target != "" || len(left.Bytes) > 1<<20 || len(right.Bytes) > 1<<20 || !utf8.Valid(left.Bytes) || !utf8.Valid(right.Bytes) || bytes.ContainsRune(left.Bytes, 0) || bytes.ContainsRune(right.Bytes, 0) || int64(len(left.Bytes)) != left.Size || int64(len(right.Bytes)) != right.Size || digest(left.Bytes) != left.SHA256 || digest(right.Bytes) != right.SHA256 || bytes.Count(left.Bytes, []byte(op.Old)) != 1 {
			return fail()
		}
		if !bytes.Equal(bytes.Replace(left.Bytes, []byte(op.Old), []byte(op.New), 1), right.Bytes) {
			return fail()
		}
	}
	if before[op.Path].Path == "" || after[op.Path].Path == "" {
		return fail()
	}
	leftIndex := append([]ProofIndex{}, in.BeforeIndex...)
	rightIndex := append([]ProofIndex{}, in.AfterIndex...)
	sortIndex := func(a, b ProofIndex) int { return strings.Compare(a.Path, b.Path) }
	slices.SortFunc(leftIndex, sortIndex)
	slices.SortFunc(rightIndex, sortIndex)
	for i := range leftIndex {
		if leftIndex[i] != rightIndex[i] || i > 0 && leftIndex[i-1].Path == leftIndex[i].Path {
			return fail()
		}
	}
	return nil
}
