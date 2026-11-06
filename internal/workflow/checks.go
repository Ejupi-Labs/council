package workflow

import (
	json "encoding/json/v2"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type CheckMetadata struct {
	Path               string
	Bytes              []byte
	ArtifactID, SHA256 string
}
type CheckProgram struct{ Name, Executable, PhysicalIdentity string }
type CheckGap struct{ Key, Code, Path, Message string }
type CheckInput struct {
	Original  string
	Version   domain.TaskVersion
	Options   domain.WorkflowInput
	Metadata  []CheckMetadata
	Programs  []CheckProgram
	TestFiles []string
}

// DefinitionDigest excludes occurrence ID and its own digest. Source material
// remains bound through captured evidence hashes and exact command identity.
func DefinitionDigest(definition domain.CheckDefinition) (string, error) {
	definition.ID = ""
	definition.DefinitionSHA256 = ""
	raw, err := json.Marshal(definition)
	if err != nil {
		return "", err
	}
	return digest(raw), nil
}

func CaptureCheck(declaration domain.CheckDeclaration, id string, program CheckProgram) (domain.CheckDefinition, *domain.Error) {
	var out domain.CheckDefinition
	if e := domain.ValidateCheckDeclaration(declaration); e != nil {
		return out, e
	}
	if id != "" && !domain.ValidID(id) {
		return out, domain.Invalid("captured check identity")
	}
	if !filepath.IsAbs(program.Executable) || program.PhysicalIdentity == "" || !utf8.ValidString(program.Executable) || strings.ContainsRune(program.Executable, 0) {
		return out, domain.Fail("check_unavailable", "The declared native program has no current filesystem identity.")
	}
	ext := strings.ToLower(filepath.Ext(program.Executable))
	if ext == ".cmd" || ext == ".bat" || ext == ".ps1" {
		return out, domain.Fail("check_unavailable", "A shell wrapper is not a supported direct native check executable.")
	}
	timeout := 300
	if declaration.TimeoutSeconds != nil {
		timeout = *declaration.TimeoutSeconds
	}
	codes := []int{0}
	if declaration.SuccessExitCodes != nil {
		codes = append([]int{}, (*declaration.SuccessExitCodes)...)
	}
	required := true
	if declaration.Required != nil {
		required = *declaration.Required
	}
	cwd := declaration.RelativeWorkingDirectory
	if cwd == "" {
		cwd = "."
	}
	scope := domain.CheckScope{Kind: "whole_candidate", Paths: []string{}}
	if declaration.Scope != nil {
		scope = *declaration.Scope
		scope.Paths = append([]string{}, scope.Paths...)
		slices.Sort(scope.Paths)
	}
	covered := append([]string{}, declaration.CoveredConditionKeys...)
	if len(covered) == 0 {
		covered = []string{declaration.Key}
	}
	slices.Sort(covered)
	out = domain.CheckDefinition{ID: id, Key: declaration.Key, Kind: "command", Source: "task_declaration", RecognizerID: "explicit-native-command/1", Command: &domain.CommandDefinition{Executable: program.Executable, Arguments: append([]string{}, declaration.Arguments...), RelativeWorkingDirectory: cwd, TimeoutSeconds: timeout, SuccessExitCodes: codes, PhysicalIdentity: program.PhysicalIdentity}, Scope: scope, Required: required, CoveredConditionKeys: covered, EvidenceRefs: []domain.EvidenceReference{}}
	hash, err := DefinitionDigest(out)
	if err != nil {
		return out, domain.Invalid("captured native check definition")
	}
	out.DefinitionSHA256 = hash
	return out, nil
}

// Checks selects only fixed conventions or explicit declarations. Filesystem
// lookup and proof of ordinary authorized metadata are supplied by the controller.
func Checks(in CheckInput) ([]domain.CheckDefinition, []CheckGap, *domain.Error) {
	definitions := []domain.CheckDefinition{}
	gaps := []CheckGap{}
	if e := domain.ValidateWorkflowInput(&in.Options); e != nil {
		return nil, nil, e
	}
	if in.Options.Mode == "author_only" {
		return definitions, gaps, nil
	}
	metadata := map[string]CheckMetadata{}
	for _, entry := range in.Metadata {
		if !safeReviewPath(entry.Path) || metadata[entry.Path].Path != "" || len(entry.Bytes) > 1<<20 || !validHash(entry.SHA256) || len(entry.Bytes) > 0 && digest(entry.Bytes) != entry.SHA256 {
			return nil, nil, domain.Fail("evidence_unavailable", "Check marker metadata is invalid, ambiguous or changed.")
		}
		metadata[entry.Path] = entry
	}
	programs := map[string]CheckProgram{}
	for _, program := range in.Programs {
		if program.Name == "" || programs[program.Name].Name != "" {
			return nil, nil, domain.Invalid("unique captured native programs")
		}
		programs[program.Name] = program
	}
	programFor := func(executable string) CheckProgram {
		if value, ok := programs[executable]; ok {
			return value
		}
		for _, value := range programs {
			if value.Executable == executable {
				return value
			}
		}
		return CheckProgram{}
	}
	if _, ok := RecognizeDocument(in.Original, in.Version); ok {
		definition := domain.CheckDefinition{Key: "literal-document", Kind: "builtin", Source: "recognized_convention", RecognizerID: "literal_document_replace/1", BuiltinID: "literal_document_replace/1", Scope: domain.CheckScope{Kind: "whole_candidate", Paths: []string{}}, Required: true, CoveredConditionKeys: []string{"task-instructions"}, EvidenceRefs: []domain.EvidenceReference{}}
		definition.DefinitionSHA256, _ = DefinitionDigest(definition)
		definitions = append(definitions, definition)
	}
	if in.Options.Checks != nil {
		for _, declaration := range *in.Options.Checks {
			definition, e := CaptureCheck(declaration, "", programFor(declaration.Executable))
			if e != nil {
				gaps = append(gaps, CheckGap{Key: declaration.Key, Code: "check_unavailable", Message: e.Message})
				continue
			}
			definitions = append(definitions, definition)
		}
		if len(definitions) > domain.MaxWorkflowChecks {
			return nil, nil, domain.Fail("input_too_large", "At most sixteen total check definitions may be captured.")
		}
		return definitions, gaps, nil
	}
	add := func(key, programName, recognizer string, args []string, markers ...string) {
		declaration := domain.CheckDeclaration{Key: key, Executable: programName, Arguments: args}
		definition, e := CaptureCheck(declaration, "", programFor(programName))
		if e != nil {
			gaps = append(gaps, CheckGap{Key: key, Code: "check_unavailable", Message: e.Message})
			return
		}
		definition.Source = "recognized_convention"
		definition.RecognizerID = recognizer
		for _, name := range markers {
			entry := metadata[name]
			definition.EvidenceRefs = append(definition.EvidenceRefs, domain.EvidenceReference{Ref: name, ArtifactID: entry.ArtifactID, SHA256: entry.SHA256, RelativePath: name})
		}
		definition.DefinitionSHA256, _ = DefinitionDigest(definition)
		definitions = append(definitions, definition)
	}
	if _, ok := metadata["go.mod"]; ok {
		markers := []string{"go.mod"}
		if _, ok = metadata["go.sum"]; ok {
			markers = append(markers, "go.sum")
		}
		add("go-tests", "go", "go-module-test/1", []string{"test", "-count=1", "-json", "-mod=readonly", "./..."}, markers...)
	}
	if _, ok := metadata["Cargo.toml"]; ok {
		if _, locked := metadata["Cargo.lock"]; !locked {
			gaps = append(gaps, CheckGap{Key: "cargo-tests", Code: "check_unavailable", Path: "Cargo.lock", Message: "The supported Cargo convention requires its retained lock file."})
		} else {
			add("cargo-tests", "cargo", "cargo-locked-test/1", []string{"test", "--locked", "--message-format=json"}, "Cargo.toml", "Cargo.lock")
		}
	}
	if entry, ok := metadata["package.json"]; ok {
		var packageJSON struct {
			Scripts map[string]string `json:"scripts"`
		}
		if len(entry.Bytes) == 0 || json.Unmarshal(entry.Bytes, &packageJSON) != nil {
			return definitions, append(gaps, CheckGap{Key: "node-tests", Code: "check_unavailable", Path: "package.json", Message: "A supported direct Node test declaration could not be read."}), nil
		}
		if script, present := packageJSON.Scripts["test"]; present {
			args, valid := directNodeTest(script, in.TestFiles)
			if !valid {
				gaps = append(gaps, CheckGap{Key: "node-tests", Code: "check_unavailable", Path: "package.json", Message: "This npm script needs unsupported shell/glob semantics. Declare a supported native program and static argv explicitly."})
			} else {
				add("node-tests", "node", "direct-node-test/1", args, "package.json")
			}
		}
	}
	if len(definitions) > 16 {
		return nil, nil, domain.Fail("input_too_large", "At most sixteen check definitions may be captured.")
	}
	return definitions, gaps, nil
}

func directNodeTest(script string, captured []string) ([]string, bool) {
	if strings.ContainsAny(script, "\r\n\"'`$*?;&|<>\\") {
		return nil, false
	}
	tokens := strings.Fields(script)
	if len(tokens) < 2 || tokens[0] != "node" || tokens[1] != "--test" {
		return nil, false
	}
	files := tokens[2:]
	if len(files) > 0 && files[0] == "--test-reporter=tap" {
		files = files[1:]
	}
	if len(files) == 0 {
		files = append([]string{}, captured...)
	}
	if len(files) == 0 || len(files) > 62 {
		return nil, false
	}
	seen := map[string]bool{}
	out := []string{"--test", "--test-reporter=tap"}
	for _, file := range files {
		if !safeReviewPath(file) || strings.HasPrefix(file, "-") || seen[file] || !slices.Contains(captured, file) {
			return nil, false
		}
		seen[file] = true
		out = append(out, file)
	}
	return out, true
}
