package routing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const sourceLimit = 1 << 20

var replacementFirst = regexp.MustCompile(`(?i)^replace[ \t]+"([^"\r\n]+)"[ \t]+with[ \t]+"([^"\r\n]+)"[ \t]+in[ \t]+([^ \t\r\n]+)[ \t]+only\.?$`)
var replacementLast = regexp.MustCompile(`(?i)^in[ \t]+([^ \t\r\n]+)[ \t]+only,[ \t]+replace[ \t]+"([^"\r\n]+)"[ \t]+with[ \t]+"([^"\r\n]+)"\.?$`)

func safeSourcePath(relative string) bool {
	if !utf8.ValidString(relative) || len(relative) > 1024 || relative == "" || path.Clean(relative) != relative || filepath.IsAbs(relative) || strings.ContainsAny(relative, `\:*?[]{}~`+"\x00") {
		return false
	}
	for _, component := range strings.Split(relative, "/") {
		lower := strings.ToLower(component)
		// Hidden configuration and vendor/private state are outside this deliberately
		// narrow document grammar. This also rejects ADS, traversal and Windows aliases.
		if component == "" || strings.HasPrefix(component, ".") || strings.TrimRight(component, " .") != component {
			return false
		}
		for _, r := range component {
			if unicode.IsControl(r) {
				return false
			}
		}
		stem := strings.Split(lower, ".")[0]
		if stem == "con" || stem == "prn" || stem == "aux" || stem == "nul" || len(stem) == 4 && (strings.HasPrefix(stem, "com") || strings.HasPrefix(stem, "lpt")) && stem[3] >= '0' && stem[3] <= '9' {
			return false
		}
	}
	switch strings.ToLower(path.Ext(relative)) {
	case ".md", ".txt", ".rst":
		return true
	}
	return false
}
func safeLiteral(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Assess never invokes a model or executable. Unrecognized or initially
// unavailable evidence stays unknown; a retained proof failing revalidation blocks.
func Assess(ctx context.Context, projectRoot, original string, version domain.TaskVersion, declared string) (domain.SuitabilityAssessment, *domain.Error) {
	unknown := domain.SuitabilityAssessment{Classification: "unknown", Source: "unknown", ReasonCodes: []string{"unrecognized_or_unproven_operation"}}
	if declared != "" && declared != "unknown" {
		if declared != "routine" && declared != "general" {
			return unknown, domain.Invalid("suitability declaration")
		}
		return domain.SuitabilityAssessment{Classification: declared, Source: "user_declared", ReasonCodes: []string{"explicit_user_hint_not_quality_proof"}}, nil
	}
	if len(original) > 2048 || !utf8.ValidString(original) || version.GuidanceText != "" || version.Acceptance.ConditionsText != "" || len(version.Acceptance.RequiredEvidence) > 0 {
		return unknown, nil
	}
	var old, newValue, relative string
	if m := replacementFirst.FindStringSubmatch(strings.TrimSpace(original)); m != nil {
		old, newValue, relative = m[1], m[2], m[3]
	} else if m := replacementLast.FindStringSubmatch(strings.TrimSpace(original)); m != nil {
		relative, old, newValue = m[1], m[2], m[3]
	} else {
		return unknown, nil
	}
	if !safeLiteral(old) || !safeLiteral(newValue) || !safeSourcePath(relative) {
		return unknown, nil
	}
	proof, err := readSourceProof(ctx, projectRoot, relative)
	if err != nil {
		return unknown, nil
	}
	rule := "literal_document_replace/1"
	return domain.SuitabilityAssessment{Classification: "routine", Source: "recognized_operation", RuleID: &rule, SourceProof: &proof, ReasonCodes: []string{"whole_task_bounded_document_replacement", "source_identity_and_bytes_retained"}}, nil
}

func readSourceProof(ctx context.Context, rootPath, relative string) (domain.RoutingSourceProof, error) {
	var proof domain.RoutingSourceProof
	if !safeSourcePath(relative) {
		return proof, errors.New("unsafe source path")
	}
	if err := ctx.Err(); err != nil {
		return proof, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return proof, err
	}
	defer root.Close()
	// Keep every ancestor's physical identity and reject every redirect, including
	// Windows junction/reparse entries. Recheck after reading through the root.
	parts := strings.Split(relative, "/")
	infos := make([]os.FileInfo, 0, len(parts))
	validate := func(before []os.FileInfo) ([]os.FileInfo, error) {
		found := make([]os.FileInfo, 0, len(parts))
		for i := range parts {
			info, e := root.Lstat(filepath.FromSlash(strings.Join(parts[:i+1], "/")))
			if e != nil {
				return nil, e
			}
			if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || i < len(parts)-1 && !info.IsDir() || i == len(parts)-1 && !info.Mode().IsRegular() {
				return nil, errors.New("redirected or nonordinary source")
			}
			if before != nil && !os.SameFile(before[i], info) {
				return nil, errors.New("source ancestor changed")
			}
			found = append(found, info)
		}
		return found, nil
	}
	infos, err = validate(nil)
	if err != nil {
		return proof, err
	}
	f, err := root.Open(filepath.FromSlash(relative))
	if err != nil {
		return proof, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return proof, err
	}
	if !info.Mode().IsRegular() || info.Size() > sourceLimit || !os.SameFile(infos[len(infos)-1], info) {
		return proof, errors.New("source identity or size unavailable")
	}
	identity, err := sourceIdentity(f)
	if err != nil {
		return proof, err
	}
	data, err := io.ReadAll(io.LimitReader(f, sourceLimit+1))
	if err != nil || len(data) > sourceLimit {
		return proof, errors.New("bounded source read failed")
	}
	if err := ctx.Err(); err != nil {
		return proof, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(info, after) || after.Size() != int64(len(data)) || !after.ModTime().Equal(info.ModTime()) {
		return proof, errors.New("source changed while reading")
	}
	if _, err = validate(infos); err != nil {
		return proof, err
	}
	identityAfter, err := sourceIdentity(f)
	if err != nil || identityAfter != identity {
		return proof, errors.New("source identity changed")
	}
	sum := sha256.Sum256(data)
	return domain.RoutingSourceProof{RelativePath: relative, SHA256: hex.EncodeToString(sum[:]), PhysicalIdentity: identity, ByteLength: strconv.Itoa(len(data))}, nil
}

func RecheckOriginal(ctx context.Context, projectRoot string, assessment domain.SuitabilityAssessment) *domain.Error {
	if assessment.SourceProof == nil {
		if assessment.Source == "recognized_operation" {
			return domain.Fail("source_changed", "The derived assessment has no retained source proof.")
		}
		return nil
	}
	got, err := readSourceProof(ctx, projectRoot, assessment.SourceProof.RelativePath)
	if err != nil || got != *assessment.SourceProof {
		return domain.Fail("source_changed", "The assessed source identity or bytes changed. Refresh the task assessment explicitly.")
	}
	return nil
}
func RecheckManaged(ctx context.Context, managedRoot string, proof domain.RoutingSourceProof) *domain.Error {
	got, err := readSourceProof(ctx, managedRoot, proof.RelativePath)
	if err != nil || got.RelativePath != proof.RelativePath || got.SHA256 != proof.SHA256 || got.ByteLength != proof.ByteLength {
		return domain.Fail("source_changed", "The managed source no longer matches the retained suitability evidence.")
	}
	return nil
}
