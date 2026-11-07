package engine

import (
	"context"
	json "encoding/json/v2"
	"path"
	"path/filepath"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// Graph workflow capture reads the sealed receiving input and creation-time
// delivery scope. A later live edit or preference change cannot replace either.
func (e *Engine) captureGraphWorkflow(ctx context.Context, definition domain.GraphDefinition, node domain.GraphNodeSummary, detail domain.TaskVersionDetail, input domain.GraphInputDetail) (*domain.WorkflowCapture, domain.GraphConditionsBinding, error) {
	var conditions domain.GraphConditionsBinding
	if input.InputBinding == nil || input.ConditionsRef == nil || input.ContextPacket == nil || input.Summary.State != "ready" || !input.Summary.Current ||
		input.InputBinding.GraphID != definition.GraphID || input.InputBinding.GraphVersion != definition.GraphVersion || input.InputBinding.ReceivingNodeID != node.ID || input.InputBinding.ReceivingTaskID != detail.TaskID || input.InputBinding.ReceivingTaskVersion != detail.Version.Version || node.TaskID != detail.TaskID || node.TaskVersion != detail.Version.Version {
		return nil, conditions, domain.Fail("graph_input_stale", "Capture workflow authority only from this receiver's exact ready input.")
	}
	verified, err := e.workspace.VerifyGraphInput(ctx, *input.InputBinding)
	if err != nil {
		return nil, conditions, err
	}
	declared := -1
	for i, candidate := range definition.Original.Nodes {
		if candidate.Key == node.Key {
			declared = i
			break
		}
	}
	if declared < 0 {
		return nil, conditions, domain.Invalid("declared receiving node key")
	}
	declaration := definition.Original.Nodes[declared]
	if declaration.OriginalText != detail.OriginalText {
		return nil, conditions, evidence.ErrCorrupt
	}
	versionOptions, err := e.db.GraphNodeOptions(ctx, definition.GraphID, node.ID, detail.Version.Version)
	if err != nil {
		return nil, conditions, err
	}
	options := domain.WorkflowInput{Mode: "auto", Review: "auto"}
	if versionOptions.Workflow != nil {
		options = *versionOptions.Workflow
	}
	if options.Mode == "" {
		options.Mode = "auto"
	}
	if options.Review == "" {
		options.Review = "auto"
	}
	if options.Mode != "auto" || options.Delivery != nil {
		return nil, conditions, domain.Invalid("graph-owned Auto workflow and delivery scope")
	}
	scope := definition.Delivery
	if definition.Original.FinalNodeKey != node.Key {
		scope.LocalMode, scope.ExternalAction, scope.Source, scope.PreferenceRevision = "review_only", nil, "graph_scope", ""
	}
	options.Delivery = &domain.WorkflowDeliveryInput{LocalMode: scope.LocalMode, ExternalAction: scope.ExternalAction}
	checkInput := workflowpolicy.CheckInput{Original: detail.OriginalText, Version: detail.Version, Options: options, Metadata: []workflowpolicy.CheckMetadata{}, Programs: []workflowpolicy.CheckProgram{}, TestFiles: []string{}}
	entries := map[string]workspace.Entry{}
	for _, entry := range verified.Manifest.Entries {
		entries[entry.Path] = entry
	}
	nameInInput := func(name string) string {
		if scope.SelectedRelativePath == "" || scope.SelectedRelativePath == "." {
			return name
		}
		return path.Join(scope.SelectedRelativePath, name)
	}
	for _, marker := range []string{"go.mod", "go.sum", "Cargo.toml", "Cargo.lock", "package.json"} {
		entry, present := entries[nameInInput(marker)]
		if !present {
			continue
		}
		if entry.Type != "file" || entry.Size > 1<<20 {
			return nil, conditions, domain.Fail("evidence_unavailable", "A retained check convention marker is redirected or exceeds its complete capture bound.")
		}
		meta, err := e.evidence.VerifyClosed(ctx, entry.ArtifactID)
		if err != nil || meta.Kind != "source_file" || meta.SHA256 == nil || *meta.SHA256 != entry.SHA256 {
			return nil, conditions, evidence.ErrCorrupt
		}
		ref := domain.GraphArtifactRef{ArtifactID: meta.ID, ProducerAttemptID: meta.AttemptID, SHA256: *meta.SHA256, ByteLength: meta.ByteLength}
		body, err := e.readGraphArtifact(ctx, ref, "source_file", 1<<20)
		if err != nil {
			return nil, conditions, err
		}
		checkInput.Metadata = append(checkInput.Metadata, workflowpolicy.CheckMetadata{Path: marker, Bytes: body, SHA256: entry.SHA256})
		if marker == "package.json" {
			var declaredScripts struct{ Scripts map[string]string }
			if json.Unmarshal(body, &declaredScripts) == nil {
				for _, token := range strings.Fields(declaredScripts.Scripts["test"]) {
					if strings.HasPrefix(token, "-") || token == "node" || !filepath.IsLocal(token) || token == "." {
						continue
					}
					if file, exists := entries[nameInInput(filepath.ToSlash(token))]; exists && file.Type == "file" {
						checkInput.TestFiles = append(checkInput.TestFiles, filepath.ToSlash(token))
					}
				}
			}
		}
	}
	programs := []string{"go", "cargo", "node"}
	if options.Checks != nil {
		for _, check := range *options.Checks {
			programs = append(programs, check.Executable)
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
	checks, gaps, invalid := workflowpolicy.Checks(checkInput)
	if invalid != nil {
		return nil, conditions, invalid
	}
	for i := range checks {
		checks[i].ID = domain.NewID()
	}
	registry, invalid := workflowpolicy.Conditions(detail.OriginalText, detail.Version, options, checks)
	if invalid != nil {
		return nil, conditions, invalid
	}
	instructions, nodeConditions, err := workflowpolicy.ConditionDigests(detail.OriginalText, detail.Version, registry, checks)
	if err != nil {
		return nil, conditions, err
	}
	body, err := e.readGraphArtifact(ctx, *input.ConditionsRef, "graph_conditions", 64<<20)
	if err != nil {
		return nil, conditions, err
	}
	if err = json.Unmarshal(body, &conditions, json.RejectUnknownMembers(true)); err != nil {
		return nil, conditions, evidence.ErrCorrupt
	}
	seal, err := domain.GraphConditionsDigest(conditions)
	if err != nil || seal != conditions.ConditionsSHA256 || conditions.GraphID != definition.GraphID || conditions.GraphVersion != definition.GraphVersion || conditions.ReceiverTaskID != detail.TaskID || conditions.ReceiverTaskVersion != detail.Version.Version {
		return nil, conditions, evidence.ErrCorrupt
	}
	effective, err := graphEffectiveConditionsDigest(nodeConditions, conditions.ConditionsSHA256)
	if err != nil {
		return nil, conditions, err
	}
	retainedGaps := make([]domain.WorkflowGap, 0, len(gaps))
	for _, gap := range gaps {
		retainedGaps = append(retainedGaps, domain.WorkflowGap{Key: gap.Key, Code: gap.Code, Path: gap.Path, Message: gap.Message})
	}
	return &domain.WorkflowCapture{Authority: domain.WorkflowAuthority{ExecutionMode: versionOptions.ExecutionMode, FakeScenario: versionOptions.FakeScenario, OptionSnapshot: options,
		InstructionSHA256: instructions, ConditionsSHA256: effective, Conditions: registry, Delivery: scope, Checks: checks, CheckGaps: retainedGaps, Review: options.Review, ReasonCodes: []string{"captured_graph_workflow", "sealed_receiving_input", "receiver_scoped_conditions"}},
		Step: domain.WorkflowStep{Role: "author", ExecutorKind: "native", LogicalKey: "author:0", ReasonCodes: []string{"initial_author"}}}, conditions, nil
}
