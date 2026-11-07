package fixture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

// GraphContextInspection records actual bytes read by this explicit local fake
// worker. It is neither provider telemetry nor an assertion of review acceptance.
type GraphContextInspection struct {
	Ref        string `json:"ref"`
	ArtifactID string `json:"artifactId"`
	Mode       string `json:"mode"`
	ReadVia    string `json:"readVia"`
	SHA256     string `json:"sha256"`
	ByteLength string `json:"byteLength"`
}

func GraphArgsWithContext(harness, scenario, role, response, expectedDirectory string) ([]string, error) {
	args, err := GraphArgs(harness, scenario, role, response)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(expectedDirectory) || filepath.Clean(expectedDirectory) != expectedDirectory {
		return nil, errors.New("Select the exact absolute controlled context directory.")
	}
	return append(args, "--context-directory", expectedDirectory), nil
}

func runGraphWithContext(harness, scenario, response, expectedDirectory string) int {
	input, err := io.ReadAll(io.LimitReader(os.Stdin, domain.MaxPromptBytes+1))
	if err != nil || len(input) > domain.MaxPromptBytes {
		return 2
	}
	reads, err := readGraphFixtureContext(input, expectedDirectory)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Controlled graph context is unavailable or does not match its complete retained bytes.")
		return 9
	}
	return runGraphInput(harness, scenario, response, input, reads)
}

type graphFixtureAccess struct {
	Ref         string                  `json:"ref"`
	ArtifactRef domain.GraphArtifactRef `json:"artifactRef"`
	Scope       string                  `json:"scope"`
	Paths       []string                `json:"paths"`
	Mode        string                  `json:"mode"`
	Path        *string                 `json:"path"`
	Text        *string                 `json:"text"`
	BaseField   *string                 `json:"baseField"`
}
type graphFixturePrompt struct {
	Contract     string                    `json:"contract"`
	Instructions string                    `json:"instructions"`
	Packet       domain.GraphContextPacket `json:"packet"`
	Access       []graphFixtureAccess      `json:"access"`
}

type graphFixtureFinalContext struct {
	Contract             string                  `json:"contract"`
	ReceivingAttemptID   string                  `json:"receivingAttemptId"`
	InputID              string                  `json:"inputId"`
	InputSHA256          string                  `json:"inputSha256"`
	PacketSHA256         string                  `json:"packetSha256"`
	FinalVerificationRef domain.GraphArtifactRef `json:"finalVerificationRef"`
	Access               []graphFixtureAccess    `json:"access"`
}

func contextRefSHA(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// This fixed fake worker reads only six-digit controller filenames in the
// private directory pinned in argv. Task/model prose never selects a command or
// an additional directory, and receipts contain no original text or native usage.
func readGraphFixtureContext(input []byte, expectedDirectory string) ([]GraphContextInspection, error) {
	invalid := errors.New("Unsupported controlled graph context.")
	if len(input) > domain.MaxPromptBytes || !filepath.IsAbs(expectedDirectory) || filepath.Clean(expectedDirectory) != expectedDirectory {
		return nil, invalid
	}
	baseName := filepath.Base(expectedDirectory)
	if !strings.HasPrefix(baseName, "graph-context-") || !domain.ValidID(strings.TrimPrefix(baseName, "graph-context-")) {
		return nil, invalid
	}
	line := bytes.IndexByte(input, '\n')
	if line < 1 {
		return nil, invalid
	}
	var base map[string]jsontext.Value
	if json.Unmarshal(input[line+1:], &base) != nil || base == nil {
		return nil, invalid
	}
	var graph graphFixturePrompt
	if json.Unmarshal(base["graphContext"], &graph, json.RejectUnknownMembers(true)) != nil || graph.Contract != "006-native-context-v1" || graph.Instructions == "" {
		return nil, invalid
	}
	packet := graph.Packet
	seal, err := domain.GraphContextDigest(packet)
	if err != nil || seal != packet.PacketSHA256 || !domain.ValidID(packet.ID) || !domain.ValidID(packet.GraphID) || !domain.ValidID(packet.InputID) || !domain.ValidSHA256(packet.InputSHA256) {
		return nil, invalid
	}
	if _, valueErr := domain.ParseRevision(packet.ReceivingTaskVersion); valueErr != nil {
		return nil, invalid
	}
	if len(graph.Access) != len(packet.ProvidedAccess) || len(graph.Access) > 64 {
		return nil, invalid
	}
	refs := map[string]domain.GraphArtifactRef{}
	add := func(key string, ref domain.GraphArtifactRef) error {
		if key == "" || len(key) > 128 || strings.ContainsRune(key, 0) || !utf8.ValidString(key) {
			return invalid
		}
		if prior, ok := refs["graph:"+key]; ok && prior != ref {
			return invalid
		}
		if domain.ValidateGraphArtifactRef(ref) != nil {
			return invalid
		}
		refs["graph:"+key] = ref
		if len(refs) > 64 {
			return invalid
		}
		return nil
	}
	if raw, present := base["evidence"]; present {
		var evidence []struct {
			Ref string `json:"ref"`
		}
		if json.Unmarshal(raw, &evidence) != nil {
			return nil, invalid
		}
		for _, item := range evidence {
			key := "workflow:" + item.Ref
			if item.Ref == "" {
				return nil, invalid
			}
			if _, duplicate := refs[key]; duplicate {
				return nil, invalid
			}
			refs[key] = domain.GraphArtifactRef{}
		}
	}
	if add("graph-instructions", packet.GraphInstructionsRef) != nil || add("node-instructions", packet.NodeInstructionsRef) != nil {
		return nil, invalid
	}
	required := []domain.GraphArtifactRef{packet.GraphInstructionsRef, packet.NodeInstructionsRef}
	if packet.GuidanceRef != nil {
		if add("receiver-guidance", *packet.GuidanceRef) != nil {
			return nil, invalid
		}
		required = append(required, *packet.GuidanceRef)
	}
	if packet.AcceptanceRef != nil {
		if add("receiver-acceptance", *packet.AcceptanceRef) != nil {
			return nil, invalid
		}
		required = append(required, *packet.AcceptanceRef)
	}
	for _, source := range packet.ConstraintRefs {
		if add(source.Ref, source.ArtifactRef) != nil {
			return nil, invalid
		}
		required = append(required, source.ArtifactRef)
	}
	for _, source := range packet.ContributedEvidence {
		if add(source.Ref, source.ArtifactRef) != nil {
			return nil, invalid
		}
		required = append(required, source.ArtifactRef)
	}
	var total int64
	payloads := map[string]domain.GraphArtifactRef{}
	for i, access := range graph.Access {
		planned := packet.ProvidedAccess[i]
		if access.Ref != planned.Ref || access.ArtifactRef != planned.ArtifactRef || access.Mode != planned.Mode || access.Scope != planned.Scope || !slices.Equal(access.Paths, planned.Paths) || add(access.Ref, access.ArtifactRef) != nil {
			return nil, invalid
		}
		length, e := domain.ParseCount(access.ArtifactRef.ByteLength)
		if e != nil || length > 64<<20 {
			return nil, invalid
		}
		if prior, seen := payloads[access.ArtifactRef.ArtifactID]; seen {
			if prior != access.ArtifactRef {
				return nil, invalid
			}
		} else {
			total += length
			payloads[access.ArtifactRef.ArtifactID] = access.ArtifactRef
		}
		if total > 512<<20 {
			return nil, invalid
		}
	}
	accesses := append([]graphFixtureAccess{}, graph.Access...)
	if raw, present := base["graphFinalVerification"]; present {
		var final graphFixtureFinalContext
		if json.Unmarshal(raw, &final, json.RejectUnknownMembers(true)) != nil || final.Contract != "006-final-context-v1" ||
			final.ReceivingAttemptID != strings.TrimPrefix(baseName, "graph-context-") || !domain.ValidID(final.ReceivingAttemptID) ||
			final.InputID != packet.InputID || final.InputSHA256 != packet.InputSHA256 || final.PacketSHA256 != packet.PacketSHA256 ||
			domain.ValidateGraphArtifactRef(final.FinalVerificationRef) != nil || len(final.Access) == 0 || len(final.Access)+len(graph.Access) > 64 {
			return nil, invalid
		}
		covered := false
		for _, access := range final.Access {
			if access.Scope != "cumulative_final" || access.Paths == nil || access.Mode != "file" || access.Path == nil || access.Text != nil || access.BaseField != nil {
				return nil, invalid
			}
			if _, duplicate := refs["graph:"+access.Ref]; duplicate || add(access.Ref, access.ArtifactRef) != nil {
				return nil, invalid
			}
			length, valueErr := domain.ParseCount(access.ArtifactRef.ByteLength)
			if valueErr != nil || length > 64<<20 {
				return nil, invalid
			}
			if previous, found := payloads[access.ArtifactRef.ArtifactID]; found {
				if previous != access.ArtifactRef {
					return nil, invalid
				}
			} else {
				total += length
				payloads[access.ArtifactRef.ArtifactID] = access.ArtifactRef
			}
			if total > 512<<20 {
				return nil, invalid
			}
			if access.Ref == "final-verification" && access.ArtifactRef == final.FinalVerificationRef && len(access.Paths) == 0 {
				covered = true
			}
		}
		if !covered {
			return nil, invalid
		}
		accesses = append(accesses, final.Access...)
	}
	for _, original := range required {
		covered := false
		for _, access := range graph.Access {
			if access.ArtifactRef == original && len(access.Paths) == 0 && (access.Mode == "file" || access.Mode == "inline") {
				covered = true
				break
			}
		}
		if !covered {
			return nil, invalid
		}
	}
	info, err := os.Lstat(expectedDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, invalid
	}
	root, err := os.OpenRoot(expectedDirectory)
	if err != nil {
		return nil, invalid
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return nil, invalid
	}
	opened, statErr := directory.Stat()
	privateErr := localstate.VerifyPrivateFile(directory, true)
	closeErr := directory.Close()
	// This immediate comparison also eagerly captures the Windows pathname
	// identity, before later observations can resolve a renamed replacement.
	if statErr != nil || privateErr != nil || closeErr != nil || !os.SameFile(info, opened) {
		return nil, invalid
	}
	reads := make([]GraphContextInspection, 0, len(accesses))
	seenAccess := map[string]bool{}
	for _, access := range accesses {
		if seenAccess[access.Ref] {
			return nil, invalid
		}
		seenAccess[access.Ref] = true
		via, hash, length := "", "", ""
		switch access.Mode {
		case "file":
			if access.Path == nil || access.Text != nil || access.BaseField != nil || filepath.Clean(*access.Path) != *access.Path || filepath.Dir(*access.Path) != expectedDirectory {
				return nil, invalid
			}
			name := filepath.Base(*access.Path)
			if len(name) != 10 || name[6:] != ".txt" {
				return nil, invalid
			}
			for _, digit := range name[:6] {
				if digit < '0' || digit > '9' {
					return nil, invalid
				}
			}
			hash, length, err = readGraphFixtureFile(root, name)
			if err != nil {
				return nil, invalid
			}
			via = "file"
		case "inline":
			if access.Path != nil || (access.Text == nil) == (access.BaseField == nil) {
				return nil, invalid
			}
			var body []byte
			if access.Text != nil {
				if !utf8.ValidString(*access.Text) {
					return nil, invalid
				}
				body = []byte(*access.Text)
				via = "inline"
			} else {
				key := *access.BaseField
				if key == "originalInstructions" && access.ArtifactRef != packet.NodeInstructionsRef || key == "guidance" && (packet.GuidanceRef == nil || access.ArtifactRef != *packet.GuidanceRef) || key == "acceptance" && (packet.AcceptanceRef == nil || access.ArtifactRef != *packet.AcceptanceRef) {
					return nil, invalid
				}
				raw, present := base[key]
				if !present {
					return nil, invalid
				}
				switch key {
				case "originalInstructions", "guidance":
					var value string
					if json.Unmarshal(raw, &value) != nil || !utf8.ValidString(value) {
						return nil, invalid
					}
					body = []byte(value)
				case "acceptance":
					if raw.Kind() != '{' {
						return nil, invalid
					}
					body = []byte(raw)
				default:
					return nil, invalid
				}
				via = "base_field"
			}
			hash, length = contextRefSHA(body), strconv.Itoa(len(body))
		default:
			return nil, invalid
		}
		if hash != access.ArtifactRef.SHA256 || length != access.ArtifactRef.ByteLength {
			return nil, invalid
		}
		reads = append(reads, GraphContextInspection{Ref: access.Ref, ArtifactID: access.ArtifactRef.ArtifactID, Mode: access.Mode, ReadVia: via, SHA256: hash, ByteLength: length})
	}
	current, err := os.Lstat(expectedDirectory)
	if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, current) {
		return nil, invalid
	}
	return reads, nil
}

// The private cumulative tag is transport attribution. Reports retain their
// closed inspection enum, with exact path subsets for before/staged payloads.
func graphFinalFixtureInspections(input []byte) ([]domain.ReviewInspection, error) {
	line := bytes.IndexByte(input, '\n')
	if line < 0 {
		return nil, errors.New("The controlled final review prompt is unavailable.")
	}
	var base map[string]jsontext.Value
	if json.Unmarshal(input[line+1:], &base) != nil {
		return nil, errors.New("The controlled final review prompt is invalid.")
	}
	raw, exists := base["graphFinalVerification"]
	if !exists {
		return nil, nil
	}
	var final graphFixtureFinalContext
	if json.Unmarshal(raw, &final, json.RejectUnknownMembers(true)) != nil || final.Contract != "006-final-context-v1" {
		return nil, errors.New("The controlled final review context is invalid.")
	}
	result := make([]domain.ReviewInspection, 0, len(final.Access))
	for _, access := range final.Access {
		scope := "whole_candidate"
		if len(access.Paths) != 0 {
			scope = "declared_subset"
		}
		result = append(result, domain.ReviewInspection{Ref: access.Ref, Scope: scope, Paths: append([]string{}, access.Paths...)})
	}
	return result, nil
}

func readGraphFixtureFile(root *os.Root, name string) (string, string, error) {
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() {
		return "", "", errors.New("Controlled context must be ordinary.")
	}
	file, err := root.Open(name)
	if err != nil {
		return "", "", err
	}
	opened, statErr := file.Stat()
	privacyErr := localstate.VerifyPrivateFile(file, false)
	if statErr != nil || privacyErr != nil || !os.SameFile(before, opened) {
		file.Close()
		return "", "", errors.New("Controlled context identity or privacy changed.")
	}
	h := sha256.New()
	count, readErr := io.Copy(h, io.LimitReader(file, (64<<20)+1))
	after, afterErr := file.Stat()
	named, nameErr := root.Lstat(name)
	privacyErr = localstate.VerifyPrivateFile(file, false)
	closeErr := file.Close()
	if readErr != nil || afterErr != nil || nameErr != nil || privacyErr != nil || closeErr != nil || count > 64<<20 || count != opened.Size() || count != after.Size() || !opened.ModTime().Equal(after.ModTime()) || !named.Mode().IsRegular() || !os.SameFile(before, named) {
		return "", "", errors.New("Controlled context bytes changed.")
	}
	return hex.EncodeToString(h.Sum(nil)), strconv.FormatInt(count, 10), nil
}
