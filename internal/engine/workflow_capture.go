package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localpath"
	"github.com/Ejupi-Labs/council/v2/internal/project"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// captureWorkflow observes a bounded, fixed set of project convention markers.
// It runs only after receipt replay, writes no artifacts, invokes no vendor, and
// grants no execution. The worker must match these hashes to retained source.
func (e *Engine) captureWorkflow(ctx context.Context, projectID, original string, version domain.TaskVersion, input *domain.WorkflowInput, mode, scenario string, routing domain.RoutingFacts) (domain.WorkflowAdmissionFacts, error) {
	facts := domain.WorkflowAdmissionFacts{Routing: routing}
	if input != nil && input.Mode == "author_only" {
		return facts, nil
	}
	options := domain.WorkflowInput{Mode: "auto", Review: "auto"}
	if input != nil {
		options = *input
		if options.Mode == "" {
			options.Mode = "auto"
		}
		if options.Review == "" {
			options.Review = "auto"
		}
	}
	p, identity, err := e.db.Project(ctx, projectID)
	if err != nil {
		return facts, err
	}
	inspection, err := project.Inspect(ctx, p.ResolvedPath, p.ResolvedPath)
	if err != nil {
		return facts, err
	}
	if inspection.Identity != identity {
		return facts, domain.Fail("project_identity_changed", "The selected project changed while preparing workflow authority.")
	}
	repository, err := os.Open(inspection.Observation.RepositoryRoot)
	if err != nil {
		return facts, domain.Fail("project_unavailable", "The selected repository cannot be inspected.")
	}
	repositoryIdentity, identityErr := workflowFileIdentity(repository)
	repository.Close()
	if identityErr != nil {
		return facts, domain.Fail("project_unavailable", "The selected repository has no stable filesystem identity.")
	}
	selected, err := filepath.Rel(inspection.Observation.RepositoryRoot, p.ResolvedPath)
	if err != nil {
		return facts, domain.Fail("project_unavailable", "The selected project does not have a repository-relative scope.")
	}
	preference, err := e.db.DeliveryPreference(ctx)
	if err != nil {
		return facts, err
	}
	localMode := preference.LocalMode
	source := "saved_preference"
	if preference.SelectionSource == "default" {
		source = "default"
	}
	var external *domain.ExternalActionInput
	if options.Delivery != nil {
		if options.Delivery.LocalMode != "" {
			localMode, source = options.Delivery.LocalMode, "task_scope"
		}
		external = options.Delivery.ExternalAction
	}
	options.Delivery = &domain.WorkflowDeliveryInput{LocalMode: localMode, ExternalAction: external}
	checkInput := workflowpolicy.CheckInput{Original: original, Version: version, Options: options, Metadata: []workflowpolicy.CheckMetadata{}, Programs: []workflowpolicy.CheckProgram{}, TestFiles: []string{}}
	root, err := os.OpenRoot(p.ResolvedPath)
	if err != nil {
		return facts, domain.Fail("project_unavailable", "The selected project cannot be read through its retained root.")
	}
	defer root.Close()
	for _, name := range []string{"go.mod", "go.sum", "Cargo.toml", "Cargo.lock", "package.json"} {
		if err := ctx.Err(); err != nil {
			return facts, err
		}
		data, readErr := workflowMarker(root, name)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return facts, domain.Fail("evidence_unavailable", "A project check marker is redirected, oversized or changed during inspection.")
		}
		sum := sha256.Sum256(data)
		checkInput.Metadata = append(checkInput.Metadata, workflowpolicy.CheckMetadata{Path: name, Bytes: data, SHA256: hex.EncodeToString(sum[:])})
		if name == "package.json" {
			// Only fixed explicit file arguments are recognized here. The pure
			// recognizer rejects shell syntax and missing/ambiguous test lists.
			var value struct{ Scripts map[string]string }
			if json.Unmarshal(data, &value) == nil {
				for _, token := range strings.Fields(value.Scripts["test"]) {
					if strings.HasPrefix(token, "-") || token == "node" || !filepath.IsLocal(token) || token == "." {
						continue
					}
					if file, openErr := root.Lstat(token); openErr == nil && file.Mode().IsRegular() {
						checkInput.TestFiles = append(checkInput.TestFiles, filepath.ToSlash(token))
					}
				}
			}
		}
	}
	programs := []string{"go", "cargo", "node"}
	if options.Checks != nil {
		for _, declaration := range *options.Checks {
			programs = append(programs, declaration.Executable)
		}
	}
	seen := map[string]bool{}
	for _, name := range programs {
		if seen[name] {
			continue
		}
		seen[name] = true
		if program, lookupErr := workflowProgram(name); lookupErr == nil {
			checkInput.Programs = append(checkInput.Programs, program)
		}
	}
	checks, gaps, checkErr := workflowpolicy.Checks(checkInput)
	if checkErr != nil {
		return facts, checkErr
	}
	for i := range checks {
		checks[i].ID = domain.NewID()
	}
	registry, conditionErr := workflowpolicy.Conditions(original, version, options, checks)
	if conditionErr != nil {
		return facts, conditionErr
	}
	instructions, conditions, err := workflowpolicy.ConditionDigests(original, version, registry, checks)
	if err != nil {
		return facts, domain.Invalid("canonical captured workflow conditions")
	}
	retainedGaps := make([]domain.WorkflowGap, 0, len(gaps))
	for _, gap := range gaps {
		retainedGaps = append(retainedGaps, domain.WorkflowGap{Key: gap.Key, Code: gap.Code, Path: gap.Path, Message: gap.Message})
	}
	branch := inspection.Observation.Branch
	if branch != nil {
		full := "refs/heads/" + *branch
		branch = &full
	}
	facts.Capture = &domain.WorkflowCapture{Authority: domain.WorkflowAuthority{
		ExecutionMode: mode, FakeScenario: scenario, OptionSnapshot: options,
		InstructionSHA256: instructions, ConditionsSHA256: conditions, Conditions: registry,
		Delivery: domain.DeliveryScope{LocalMode: localMode, Source: source, PreferenceRevision: preference.Revision, ProjectID: projectID, ProjectIdentity: identity, RepositoryIdentity: repositoryIdentity, SelectedRelativePath: filepath.ToSlash(selected), Branch: branch, Destination: p.ResolvedPath, ExternalAction: external},
		Checks:   checks, CheckGaps: retainedGaps, Review: options.Review, ReasonCodes: []string{"captured_auto_workflow"},
	}, Step: domain.WorkflowStep{Role: "author", ExecutorKind: "native", LogicalKey: "author:0", ReasonCodes: []string{"initial_author"}}}
	return facts, nil
}

func workflowMarker(root *os.Root, name string) ([]byte, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > 1<<20 {
		return nil, os.ErrInvalid
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	current, err := file.Stat()
	if err != nil || !os.SameFile(before, current) {
		return nil, os.ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	after, statErr := file.Stat()
	atPath, pathErr := root.Lstat(name)
	if err != nil || statErr != nil || pathErr != nil || len(data) > 1<<20 || !os.SameFile(before, after) || !os.SameFile(before, atPath) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, os.ErrInvalid
	}
	return data, nil
}

func workflowProgram(name string) (workflowpolicy.CheckProgram, error) {
	var out workflowpolicy.CheckProgram
	path, err := exec.LookPath(name)
	if err != nil {
		return out, err
	}
	path, err = localpath.Resolve(path)
	if err != nil {
		return out, err
	}
	file, err := os.Open(path)
	if err != nil {
		return out, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return out, os.ErrInvalid
	}
	identity, err := workflowFileIdentity(file)
	if err != nil {
		return out, err
	}
	return workflowpolicy.CheckProgram{Name: name, Executable: path, PhysicalIdentity: identity}, nil
}
