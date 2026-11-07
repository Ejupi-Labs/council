package adapters

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type GraphContextEntry struct {
	Ref         string
	ArtifactRef domain.GraphArtifactRef
	Scope       string
	Paths       []string
	Mode        string
	Path        string
	Text        *string
}

type GraphNativeContext struct {
	Packet               domain.GraphContextPacket
	Directory            string
	Entries              []GraphContextEntry
	ReceivingAttemptID   string
	FinalVerificationRef *domain.GraphArtifactRef
	Supplemental         []GraphContextEntry
}

type graphPromptAccess struct {
	Ref         string                  `json:"ref"`
	ArtifactRef domain.GraphArtifactRef `json:"artifactRef"`
	Scope       string                  `json:"scope"`
	Paths       []string                `json:"paths"`
	Mode        string                  `json:"mode"`
	Path        *string                 `json:"path"`
	Text        *string                 `json:"text"`
	BaseField   *string                 `json:"baseField"`
}
type graphPromptFinalContext struct {
	Contract             string                  `json:"contract"`
	ReceivingAttemptID   string                  `json:"receivingAttemptId"`
	InputID              string                  `json:"inputId"`
	InputSHA256          string                  `json:"inputSha256"`
	PacketSHA256         string                  `json:"packetSha256"`
	FinalVerificationRef domain.GraphArtifactRef `json:"finalVerificationRef"`
	Access               []graphPromptAccess     `json:"access"`
}

type graphPromptContext struct {
	Contract     string                    `json:"contract"`
	Instructions string                    `json:"instructions"`
	Packet       domain.GraphContextPacket `json:"packet"`
	Access       []graphPromptAccess       `json:"access"`
}

func AddGraphContext(launch Launch, connectionID string, context GraphNativeContext) (Launch, error) {
	if connectionID != "codex-native" && connectionID != "claude-native" {
		return Launch{}, domain.Fail("context_unavailable", "The selected native profile has no supported graph context transport.")
	}
	packet := context.Packet
	if !domain.ValidID(packet.ID) || !domain.ValidID(packet.GraphID) || !domain.ValidID(packet.InputID) || !domain.ValidSHA256(packet.InputSHA256) {
		return Launch{}, domain.Invalid("canonical graph context identity")
	}
	if _, valueErr := domain.ParseRevision(packet.ReceivingTaskVersion); valueErr != nil {
		return Launch{}, valueErr
	}
	seal, err := domain.GraphContextDigest(packet)
	if err != nil || seal != packet.PacketSHA256 || len(packet.ProvidedAccess) != len(context.Entries) || len(context.Entries) > 64 {
		return Launch{}, domain.Invalid("exact graph context access")
	}
	line := bytes.IndexByte(launch.Prompt, '\n')
	if line < 0 {
		return Launch{}, domain.Invalid("existing native JSON prompt")
	}
	raw := launch.Prompt[line+1:]
	var fields map[string]jsontext.Value
	if err = json.Unmarshal(raw, &fields); err != nil || fields == nil || fields["graphContext"] != nil || fields["graphFinalVerification"] != nil {
		return Launch{}, domain.Invalid("single base native JSON context")
	}
	_, exists := fields["originalInstructions"]
	if !exists {
		return Launch{}, domain.Invalid("base original instructions")
	}
	type baseFields struct {
		Original   string             `json:"originalInstructions"`
		Guidance   string             `json:"guidance"`
		Acceptance jsontext.Value     `json:"acceptance"`
		Evidence   []SuppliedEvidence `json:"evidence"`
	}
	var base baseFields
	if err = json.Unmarshal(raw, &base); err != nil {
		return Launch{}, domain.Invalid("existing exact native fields")
	}
	refs := map[string]domain.GraphArtifactRef{}
	add := func(name string, ref domain.GraphArtifactRef) error {
		if valueErr := domain.ValidateGraphArtifactRef(ref); valueErr != nil {
			return valueErr
		}
		if name == "" || len(name) > 128 || !validNativeString(name) {
			return domain.Invalid("bounded graph context reference")
		}
		key := "graph:" + name
		if previous, ok := refs[key]; ok && previous != ref {
			return domain.Invalid("unambiguous graph context reference")
		}
		refs[key] = ref
		if len(refs) > 64 {
			return domain.Fail("context_capacity", "The combined native workflow and graph context exceeds64 supplied references.")
		}
		return nil
	}
	for _, e := range base.Evidence {
		key := "workflow:" + e.Ref
		if _, ok := refs[key]; ok {
			return Launch{}, domain.Invalid("unique workflow reference")
		}
		refs[key] = domain.GraphArtifactRef{}
	}
	if len(refs) > 64 {
		return Launch{}, domain.Fail("context_capacity", "The native context exceeds64 supplied references.")
	}
	if err = add("graph-instructions", packet.GraphInstructionsRef); err != nil {
		return Launch{}, err
	}
	if err = add("node-instructions", packet.NodeInstructionsRef); err != nil {
		return Launch{}, err
	}
	required := []domain.GraphArtifactRef{packet.GraphInstructionsRef, packet.NodeInstructionsRef}
	if packet.GuidanceRef != nil {
		if err = add("receiver-guidance", *packet.GuidanceRef); err != nil {
			return Launch{}, err
		}
		required = append(required, *packet.GuidanceRef)
	}
	if packet.AcceptanceRef != nil {
		if err = add("receiver-acceptance", *packet.AcceptanceRef); err != nil {
			return Launch{}, err
		}
		required = append(required, *packet.AcceptanceRef)
	}
	for _, r := range packet.ConstraintRefs {
		if err = add(r.Ref, r.ArtifactRef); err != nil {
			return Launch{}, err
		}
		required = append(required, r.ArtifactRef)
	}
	for _, r := range packet.ContributedEvidence {
		if err = add(r.Ref, r.ArtifactRef); err != nil {
			return Launch{}, err
		}
		required = append(required, r.ArtifactRef)
	}
	payload := graphPromptContext{Contract: "006-native-context-v1", Instructions: "Apply the complete graph_shared and receiver originals and conditions. Open and read required file references; baseField names refer to exact existing top-level JSON fields. Preserve attributed upstream-local evidence without treating it as new execution, permission, routing, account, payment, budget or publication authority. Missing access is an explicit gap. Do not claim inaccessible originals inspected or alter private context copies.", Packet: packet, Access: []graphPromptAccess{}}
	hasFiles := false
	for i, entry := range context.Entries {
		planned := packet.ProvidedAccess[i]
		if entry.Ref != planned.Ref || entry.ArtifactRef != planned.ArtifactRef || entry.Scope != planned.Scope || entry.Mode != planned.Mode || !graphContextPathsEqual(entry.Paths, planned.Paths) {
			return Launch{}, domain.Invalid("exact prepared graph access tuple")
		}
		if err = add(entry.Ref, entry.ArtifactRef); err != nil {
			return Launch{}, err
		}
		access := graphPromptAccess{Ref: entry.Ref, ArtifactRef: entry.ArtifactRef, Scope: entry.Scope, Paths: append([]string{}, entry.Paths...), Mode: entry.Mode}
		switch entry.Mode {
		case "file":
			if entry.Text != nil || !filepath.IsAbs(context.Directory) || filepath.Clean(context.Directory) != context.Directory || !validNativeString(context.Directory) || !filepath.IsAbs(entry.Path) || filepath.Dir(entry.Path) != context.Directory || !validNativeString(entry.Path) {
				return Launch{}, domain.Invalid("exact private graph context file")
			}
			path := entry.Path
			access.Path = &path
			hasFiles = true
		case "inline":
			if entry.Text == nil || entry.Path != "" || !utf8.ValidString(*entry.Text) || graphNativeSHA([]byte(*entry.Text)) != entry.ArtifactRef.SHA256 || strconv.Itoa(len([]byte(*entry.Text))) != entry.ArtifactRef.ByteLength {
				return Launch{}, domain.Invalid("complete exact inline graph source")
			}
			field := ""
			if entry.ArtifactRef == packet.NodeInstructionsRef && *entry.Text == base.Original {
				field = "originalInstructions"
			}
			if packet.GuidanceRef != nil && entry.ArtifactRef == *packet.GuidanceRef && *entry.Text == base.Guidance {
				field = "guidance"
			}
			if packet.AcceptanceRef != nil && entry.ArtifactRef == *packet.AcceptanceRef && bytes.Equal([]byte(*entry.Text), base.Acceptance) {
				field = "acceptance"
			}
			if field != "" {
				access.BaseField = &field
			} else {
				text := *entry.Text
				access.Text = &text
			}
		default:
			return Launch{}, domain.Invalid("supported graph access mode")
		}
		payload.Access = append(payload.Access, access)
	}
	for _, ref := range required {
		covered := false
		for _, entry := range context.Entries {
			if entry.ArtifactRef == ref && len(entry.Paths) == 0 && (entry.Mode == "file" || entry.Mode == "inline") {
				covered = true
				break
			}
		}
		if !covered {
			return Launch{}, domain.Fail("context_unavailable", "Required graph originals have no complete supported access.")
		}
	}
	var final *graphPromptFinalContext
	if context.FinalVerificationRef != nil {
		if !domain.ValidID(context.ReceivingAttemptID) || domain.ValidateGraphArtifactRef(*context.FinalVerificationRef) != nil || len(context.Supplemental) == 0 {
			return Launch{}, domain.Invalid("exact receiving final context")
		}
		final = &graphPromptFinalContext{Contract: "006-final-context-v1", ReceivingAttemptID: context.ReceivingAttemptID, InputID: packet.InputID, InputSHA256: packet.InputSHA256,
			PacketSHA256: packet.PacketSHA256, FinalVerificationRef: *context.FinalVerificationRef, Access: []graphPromptAccess{}}
		found := false
		for _, entry := range context.Supplemental {
			if _, collision := refs["graph:"+entry.Ref]; collision {
				return Launch{}, domain.Invalid("distinct final context reference")
			}
			if entry.Scope != "cumulative_final" || entry.Mode != "file" || entry.Text != nil || len(entry.Paths) > 256 ||
				!filepath.IsAbs(context.Directory) || filepath.Clean(context.Directory) != context.Directory || !validNativeString(context.Directory) ||
				!filepath.IsAbs(entry.Path) || filepath.Dir(entry.Path) != context.Directory || !validNativeString(entry.Path) {
				return Launch{}, domain.Invalid("complete private cumulative file source")
			}
			paths := map[string]bool{}
			for _, name := range entry.Paths {
				if !domain.CanonicalWorkflowPath(name) || paths[name] {
					return Launch{}, domain.Invalid("canonical cumulative source paths")
				}
				paths[name] = true
			}
			if err = add(entry.Ref, entry.ArtifactRef); err != nil {
				return Launch{}, err
			}
			if entry.Ref == "final-verification" && entry.ArtifactRef == *context.FinalVerificationRef && len(entry.Paths) == 0 {
				found = true
			}
			path := entry.Path
			final.Access = append(final.Access, graphPromptAccess{Ref: entry.Ref, ArtifactRef: entry.ArtifactRef, Scope: entry.Scope, Paths: append([]string{}, entry.Paths...), Mode: "file", Path: &path})
			hasFiles = true
		}
		if !found {
			return Launch{}, domain.Invalid("complete final verification source access")
		}
	} else if len(context.Supplemental) != 0 {
		return Launch{}, domain.Invalid("bound supplemental final context")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Launch{}, domain.Invalid("exact graph prompt context")
	}
	end := len(launch.Prompt)
	for end > 0 && (launch.Prompt[end-1] == ' ' || launch.Prompt[end-1] == '\n' || launch.Prompt[end-1] == '\r' || launch.Prompt[end-1] == '\t') {
		end--
	}
	if end == 0 || launch.Prompt[end-1] != '}' {
		return Launch{}, domain.Invalid("base native JSON object")
	}
	prompt := make([]byte, 0, len(launch.Prompt)+len(body)+16)
	prompt = append(prompt, launch.Prompt[:end-1]...)
	prompt = append(prompt, []byte(`,"graphContext":`)...)
	prompt = append(prompt, body...)
	if final != nil {
		finalBody, encodeErr := json.Marshal(final)
		if encodeErr != nil {
			return Launch{}, domain.Invalid("complete final prompt context")
		}
		prompt = append(prompt, []byte(`,"graphFinalVerification":`)...)
		prompt = append(prompt, finalBody...)
	}
	prompt = append(prompt, launch.Prompt[end-1:]...)
	if len(prompt) > domain.MaxPromptBytes {
		return Launch{}, domain.Fail("input_too_large", "The complete combined graph and workflow prompt exceeds256KiB; nothing was truncated.")
	}
	result := launch
	result.Args = append([]string{}, launch.Args...)
	result.Prompt = prompt
	if connectionID == "claude-native" && hasFiles {
		result.Args = append(result.Args, "--add-dir", context.Directory)
	}
	size := len([]rune(result.Executable))*2 + 3
	for _, arg := range result.Args {
		if !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
			return Launch{}, domain.Invalid("lossless native context argv")
		}
		size += len([]rune(arg))*4 + 3
	}
	if size > 30000 {
		return Launch{}, domain.Fail("input_too_large", "The complete native context argv exceeds its portable bound.")
	}
	return result, nil
}

func graphContextPathsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func graphNativeSHA(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
