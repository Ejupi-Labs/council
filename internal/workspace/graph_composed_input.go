package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/project"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type GraphContributionSource struct {
	Binding      domain.AcceptedContribution `json:"binding"`
	InputBinding domain.GraphInputBinding    `json:"inputBinding"`
}

type GraphComposedInputRetainRequest struct {
	Base          GraphInputRetainRequest
	Contributions []GraphContributionSource
	CaseSensitive bool
}

type GraphComposedInputRetention struct {
	Binding                            *domain.GraphInputBinding
	Conflicts                          []InputConflict
	ConflictArtifact, OperationEffects *domain.GraphArtifactRef
	EffectsKnown                       bool
	EvidenceStatus, State, EffectCount string
	Snapshot                           *Snapshot
	Artifacts                          []domain.GraphArtifactRef
}

// Retention is an actual root local_graph operation. It creates no authored
// candidate or native admission, and never reads a former worker's current files.
func (m *Manager) RetainComposedGraphInput(ctx context.Context, request GraphComposedInputRetainRequest) (result GraphComposedInputRetention, operationErr error) {
	return m.retainGraphComposedInput(ctx, request, false)
}

func (m *Manager) retainGraphComposedInput(ctx context.Context, request GraphComposedInputRetainRequest, native bool) (result GraphComposedInputRetention, operationErr error) {
	result = GraphComposedInputRetention{EffectsKnown: true, EvidenceStatus: "complete", State: "blocked", EffectCount: "0", Conflicts: []InputConflict{}, Artifacts: []domain.GraphArtifactRef{}}
	base := request.Base
	for _, id := range []string{base.GraphID, base.RootTaskID, base.OperationAttemptID, base.InputID, base.ReceivingNodeID, base.ReceivingTaskID} {
		if !validManagedID(id) {
			return result, ErrBlocked
		}
	}
	if _, err := domain.ParseRevision(base.GraphVersion); err != nil {
		return result, err
	}
	if _, err := domain.ParseRevision(base.ReceivingTaskVersion); err != nil {
		return result, err
	}
	if len(request.Contributions) > 64 || base.GraphID != base.RootBinding.GraphID || base.RootTaskID != base.RootBinding.RootTaskID ||
		base.Context.GraphID != base.GraphID || base.Context.InputID != base.InputID || base.Context.ReceivingNodeID != base.ReceivingNodeID || base.Context.ReceivingTaskVersion != base.ReceivingTaskVersion {
		return result, ErrBlocked
	}
	selected := make([]domain.AcceptedContribution, 0, len(request.Contributions))
	for _, source := range request.Contributions {
		selected = append(selected, source.Binding)
	}
	if err := graphSelectedContextCoverage(selected, base.Context.ConstraintRefs, base.Context.ContributedEvidence, base.Context.SupportedAccess); err != nil {
		return result, err
	}
	root, err := m.VerifyGraphRoot(ctx, base.RootBinding, base.RootSource)
	if err != nil {
		return result, err
	}
	// Complete source-access capacity is checked before creating any workspace.
	// The final packet is separately sealed to the actual input digest below.
	contextCheck := base.Context
	contextCheck.InputSHA256 = base.RootBinding.BasisSHA256
	if _, err = m.BuildGraphContext(ctx, contextCheck); err != nil {
		return result, err
	}
	compose := GraphCompositionRequest{InputID: base.InputID, Root: GraphCompositionRoot{Binding: base.RootBinding, Manifest: root.Manifest, StagedBlobs: root.StagedBlobs}, Contributions: []VerifiedContribution{}, CaseSensitive: request.CaseSensitive}
	all := append([]domain.GraphArtifactRef{}, root.Artifacts...)
	for _, source := range request.Contributions {
		verified, verifyErr := m.VerifyGraphOutput(ctx, GraphOutputVerifyRequest{RootBinding: base.RootBinding, Contribution: source.Binding, InputBinding: source.InputBinding})
		if verifyErr != nil {
			return result, verifyErr
		}
		compose.Contributions = append(compose.Contributions, verified)
		all = append(all, verified.Artifacts...)
	}
	var plan CompositionPlan
	var conflicts []InputConflict
	var policies graphNativeCompositionPolicy
	if native {
		policies, err = graphNativeCompositionPolicies(compose)
		plan, conflicts = policies.sensitivePlan, policies.sensitiveConflicts
	} else {
		plan, conflicts, err = ComposeGraphInput(compose)
	}
	if err != nil {
		return result, err
	}
	record := graphCompositionEffects{Contract: "006-composed-input-effects-v1", GraphID: base.GraphID, InputID: base.InputID, State: "in_flight", Registration: "not_started", EffectsKnown: false, EvidenceStatus: "incomplete", EffectCount: "0"}
	intent, err := m.retainGraphCompositionEffects(ctx, base.OperationAttemptID, record)
	if err != nil {
		result.EvidenceStatus, result.EffectsKnown = "uncertain", false
		return result, err
	}
	result.OperationEffects = &intent
	defer func() {
		if operationErr != nil {
			result.Binding = nil
			if result.State != "uncertain" {
				result.State = "incomplete"
			}
			if result.EvidenceStatus == "complete" {
				result.EvidenceStatus = "incomplete"
			}
			if errors.Is(operationErr, evidence.ErrDurability) {
				result.EvidenceStatus = "uncertain"
			}
		}
		if !result.EffectsKnown || result.EvidenceStatus == "uncertain" {
			result.State = "uncertain"
		}
		record.State, record.EffectsKnown, record.EvidenceStatus, record.EffectCount = result.State, result.EffectsKnown, result.EvidenceStatus, result.EffectCount
		settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		terminal, saveErr := m.retainGraphCompositionEffects(settlementCtx, base.OperationAttemptID, record)
		if saveErr != nil {
			result.Binding = nil
			result.State, result.EvidenceStatus = "uncertain", "uncertain"
			operationErr = errors.Join(operationErr, saveErr)
		} else {
			result.OperationEffects = &terminal
			result.Artifacts = append(result.Artifacts, terminal)
		}
	}()
	var assembly *graphComposedAssembly
	if native && policies.probe {
		assembly, err = m.graphAllocateComposedInput(ctx, base, &record, &result)
		if err != nil {
			return result, err
		}
		probe, probeErr := m.ProbeGraphNamespace(ctx, GraphNamespaceProbeRequest{OperationAttemptID: base.OperationAttemptID, Snapshot: assembly.snapshot, WorkspaceIdentity: assembly.identity})
		record.NamespaceProbe = probe.EffectsArtifact
		if probe.EffectsArtifact != nil {
			all = append(all, *probe.EffectsArtifact)
			result.Artifacts = append(result.Artifacts, *probe.EffectsArtifact)
		}
		result.EffectsKnown = result.EffectsKnown && probe.EffectsKnown
		if probe.EvidenceStatus == "uncertain" {
			result.EvidenceStatus = "uncertain"
		}
		if countErr := graphCompositionAddEffects(&result, probe.EffectCount); countErr != nil {
			return result, errors.Join(probeErr, countErr)
		}
		if probeErr != nil {
			return result, probeErr
		}
		if probe.CaseSensitive == nil || probe.Scope != "ascii_case_in_exact_directory" || probe.State != "complete" ||
			!probe.EffectsKnown || probe.EvidenceStatus != "complete" || probe.EffectsArtifact == nil || probe.WorkspaceIdentity != assembly.identity {
			return result, ErrBlocked
		}
		plan, conflicts = policies.selectCase(*probe.CaseSensitive)
	}
	if len(conflicts) != 0 {
		body, marshalErr := json.Marshal(struct {
			Contract        string                    `json:"contract"`
			GraphID         string                    `json:"graphId"`
			InputID         string                    `json:"inputId"`
			RootBasisSHA256 string                    `json:"rootBasisSha256"`
			Contributions   []GraphContributionSource `json:"contributions"`
			Conflicts       []InputConflict           `json:"conflicts"`
		}{"006-input-conflict-v1", base.GraphID, base.InputID, base.RootBinding.BasisSHA256, request.Contributions, conflicts})
		if marshalErr != nil || len(body) > 64<<20 {
			return result, ErrBlocked
		}
		meta, saveErr := m.artifacts.Put(ctx, base.OperationAttemptID, "graph_input_provenance", body)
		if saveErr != nil {
			result.EvidenceStatus = "uncertain"
			return result, saveErr
		}
		ref := graphIndexRef(meta)
		result.ConflictArtifact, result.Conflicts, result.State = &ref, conflicts, "conflicted"
		if err = graphCompositionAddEffects(&result, "1"); err != nil {
			return result, err
		}
		record.Conflict = &ref
		result.Artifacts, operationErr = m.graphUniqueVerifiedRefs(ctx, append(all, ref))
		return result, operationErr
	}
	// Retain the selected logical state before any shared-object restoration.
	// Its real raw index is created only after the verified objects are available.
	manifestRef, err := m.graphRetainJSON(ctx, base.OperationAttemptID, "graph_input_manifest", plan.Manifest)
	if err != nil {
		result.EvidenceStatus = "uncertain"
		return result, err
	}
	record.Manifest = &manifestRef
	if err = graphCompositionAddEffects(&result, "1"); err != nil {
		return result, err
	}
	stagedRef, err := m.graphRetainJSON(ctx, base.OperationAttemptID, "graph_index_blobs", plan.StagedBlobs)
	if err != nil {
		result.EvidenceStatus = "uncertain"
		return result, err
	}
	record.StagedBlobs = &stagedRef
	if _, err = m.graphVerifyStaged(ctx, plan.Manifest, stagedRef); err != nil {
		return result, err
	}
	var source *Snapshot
	if assembly == nil {
		source, err = m.graphAssembleComposedInput(ctx, base, plan.Manifest, stagedRef, &record, &result)
	} else {
		source, err = m.graphPopulateComposedInput(ctx, base, assembly, plan.Manifest, stagedRef, &record, &result)
	}
	if err != nil {
		return result, err
	}
	provenanceRef, err := m.graphRetainJSON(ctx, base.OperationAttemptID, "graph_input_provenance", plan.Provenance)
	if err != nil {
		result.EvidenceStatus = "uncertain"
		return result, err
	}
	indexMeta, err := m.artifacts.VerifyClosed(ctx, source.IndexArtifactID)
	if err != nil {
		return result, err
	}
	indexRef := graphIndexRef(indexMeta)
	binding := domain.GraphInputBinding{ID: base.InputID, GraphID: base.GraphID, GraphVersion: base.GraphVersion, RootBasisSHA256: base.RootBinding.BasisSHA256, OperationAttemptID: base.OperationAttemptID,
		ReceivingNodeID: base.ReceivingNodeID, ReceivingTaskID: base.ReceivingTaskID, ReceivingTaskVersion: base.ReceivingTaskVersion, Contributions: plan.Contributions, ContributorVector: plan.ContributorVector,
		Manifest: manifestRef, Index: indexRef, StagedBlobs: stagedRef, Provenance: provenanceRef}
	binding.InputSHA256, err = domain.GraphInputDigest(binding)
	if err != nil {
		return result, err
	}
	contextRequest := base.Context
	if contextRequest.InputSHA256 != "" && contextRequest.InputSHA256 != binding.InputSHA256 {
		return result, ErrBlocked
	}
	contextRequest.InputSHA256 = binding.InputSHA256
	packet, err := m.BuildGraphContext(ctx, contextRequest)
	if err != nil {
		return result, err
	}
	packetRef, err := m.graphRetainJSON(ctx, base.OperationAttemptID, "graph_context", packet)
	if err != nil {
		result.EvidenceStatus = "uncertain"
		return result, err
	}
	binding.ContextPacket = packetRef
	verified, err := m.VerifyGraphInput(ctx, binding)
	if err != nil {
		return result, err
	}
	all = append(all, verified.ArtifactRefs...)
	if record.ObjectEffects != nil {
		all = append(all, *record.ObjectEffects)
	}
	sourceMeta, err := m.artifacts.VerifyClosed(ctx, source.ManifestArtifactID)
	if err != nil {
		return result, err
	}
	all = append(all, graphIndexRef(sourceMeta))
	payloads, err := m.graphClosurePayloadRefs(ctx, verified.Manifest, verified.StagedBlobs)
	if err != nil {
		return result, err
	}
	all = append(all, payloads...)
	result.Artifacts, err = m.graphUniqueVerifiedRefs(ctx, all)
	if err != nil {
		return result, err
	}
	if err = m.graphRootSource(ctx, base.RootBinding, base.RootSource); err != nil {
		return result, err
	}
	result.Binding, result.Snapshot, result.State = &binding, source, "complete"
	record.Input = &binding
	return result, nil
}

func (m *Manager) graphRetainJSON(ctx context.Context, attemptID, kind string, value any) (domain.GraphArtifactRef, error) {
	body, err := json.Marshal(value)
	if err != nil || len(body) > 64<<20 {
		return domain.GraphArtifactRef{}, ErrBlocked
	}
	meta, err := m.artifacts.Put(ctx, attemptID, kind, body)
	if err != nil {
		return domain.GraphArtifactRef{}, err
	}
	return graphIndexRef(meta), nil
}

type graphCompositionEffects struct {
	Contract       string                    `json:"contract"`
	GraphID        string                    `json:"graphId"`
	InputID        string                    `json:"inputId"`
	State          string                    `json:"state"`
	Registration   string                    `json:"registration"`
	WorkspaceID    *string                   `json:"workspaceId"`
	Manifest       *domain.GraphArtifactRef  `json:"manifest"`
	Index          *domain.GraphArtifactRef  `json:"index"`
	StagedBlobs    *domain.GraphArtifactRef  `json:"stagedBlobs"`
	ObjectEffects  *domain.GraphArtifactRef  `json:"objectEffects"`
	NamespaceProbe *domain.GraphArtifactRef  `json:"namespaceProbe"`
	Input          *domain.GraphInputBinding `json:"input"`
	Conflict       *domain.GraphArtifactRef  `json:"conflict"`
	EffectCount    string                    `json:"effectCount"`
	EffectsKnown   bool                      `json:"effectsKnown"`
	EvidenceStatus string                    `json:"evidenceStatus"`
}

func (m *Manager) retainGraphCompositionEffects(ctx context.Context, attemptID string, record graphCompositionEffects) (domain.GraphArtifactRef, error) {
	return m.graphRetainJSON(ctx, attemptID, "graph_operation_effects", record)
}

func (m *Manager) graphAssembleComposedInput(ctx context.Context, base GraphInputRetainRequest, manifest Manifest, stagedRef domain.GraphArtifactRef, record *graphCompositionEffects, result *GraphComposedInputRetention) (*Snapshot, error) {
	assembly, err := m.graphAllocateComposedInput(ctx, base, record, result)
	if err != nil {
		return nil, err
	}
	return m.graphPopulateComposedInput(ctx, base, assembly, manifest, stagedRef, record, result)
}

func graphCompositionAddEffects(result *GraphComposedInputRetention, count string) error {
	value, invalid := domain.AddRoutingCounts(result.EffectCount, count)
	if invalid != nil {
		return invalid
	}
	result.EffectCount = value
	return nil
}

type graphComposedAssembly struct {
	snapshot *Snapshot
	identity string
}

func (m *Manager) graphAllocateComposedInput(ctx context.Context, base GraphInputRetainRequest, record *graphCompositionEffects, result *GraphComposedInputRetention) (*graphComposedAssembly, error) {
	id, err := evidence.NewID()
	if err != nil {
		return nil, err
	}
	record.WorkspaceID = &id
	dir := "workspaces/" + id
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	result.EffectsKnown = false
	if err = m.privateDir(dir); err != nil {
		return nil, err
	}
	destination := filepath.Join(m.lock.Root(), filepath.FromSlash(dir))
	record.Registration = "in_flight"
	if _, err = m.git(ctx, base.RootSource.RepositoryPath, nil, "worktree", "add", "--no-checkout", "--detach", destination, base.RootBinding.BaseCommit); err != nil {
		return nil, err
	}
	record.Registration = "registered"
	result.EffectsKnown = true
	if err = graphCompositionAddEffects(result, "1"); err != nil {
		return nil, err
	}
	root, err := m.root.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	control, err := root.ReadFile(".git")
	if err != nil || len(control) > 4096 {
		return nil, ErrBlocked
	}
	source := &Snapshot{ID: id, AttemptID: base.OperationAttemptID, ProjectPath: base.RootSource.ProjectPath, RepositoryPath: base.RootSource.RepositoryPath,
		SelectedSubfolder: base.RootSource.SelectedSubfolder, WorkspacePath: destination, ExecutionPath: filepath.Join(destination, filepath.FromSlash(base.RootSource.SelectedSubfolder)),
		BaseCommit: base.RootBinding.BaseCommit, Branch: base.RootSource.Branch, GitControlSHA256: hashBytes(control)}
	result.Snapshot = source // Retain actual registration even if a later phase fails.
	physical, err := project.Inspect(ctx, destination, destination)
	if err != nil {
		return nil, err
	}
	return &graphComposedAssembly{snapshot: source, identity: physical.Identity}, nil
}

func (m *Manager) graphPopulateComposedInput(ctx context.Context, base GraphInputRetainRequest, assembly *graphComposedAssembly, manifest Manifest, stagedRef domain.GraphArtifactRef, record *graphCompositionEffects, result *GraphComposedInputRetention) (*Snapshot, error) {
	source := assembly.snapshot
	destination := source.WorkspacePath
	if err := project.ValidateIdentity(destination, assembly.identity); err != nil {
		return nil, err
	}
	root, err := m.graphIndexTarget(source)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	source.Manifest = manifest
	objects, err := m.graphRestoreIndexObjects(ctx, GraphIndexRestoreRequest{OperationAttemptID: base.OperationAttemptID, Target: source, StagedBlobs: stagedRef}, manifest)
	record.ObjectEffects = objects.EffectsArtifact
	result.EffectsKnown = result.EffectsKnown && objects.EffectsKnown
	if countErr := graphCompositionAddEffects(result, objects.EffectCount); countErr != nil {
		return nil, errors.Join(err, countErr)
	}
	if err != nil {
		return nil, err
	}
	result.EffectsKnown = false
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = m.git(ctx, destination, nil, "read-tree", "--empty"); err != nil {
		return nil, err
	}
	var indexInput bytes.Buffer
	for _, entry := range manifest.Index {
		indexInput.WriteString(entry.Mode + " " + entry.Object + "\t" + entry.Path)
		indexInput.WriteByte(0)
	}
	if indexInput.Len() != 0 {
		if _, err = m.git(ctx, destination, indexInput.Bytes(), "update-index", "-z", "--index-info"); err != nil {
			return nil, err
		}
	}
	index, err := m.index(ctx, destination)
	if err != nil || !indexEqual(index, manifest.Index) {
		return nil, fmt.Errorf("%w The actual composed staging differs.", ErrBlocked)
	}
	if err = m.materialize(ctx, root, manifest); err != nil {
		return nil, err
	}
	actual, err := m.inventory(ctx, base.OperationAttemptID, destination, root, index, false, manifest.Entries)
	actual.Exclusions = manifest.Exclusions
	if err != nil || !sameManifest(actual, manifest) {
		return nil, fmt.Errorf("%w The actual composed working tree differs.", ErrBlocked)
	}
	if err = project.ValidateIdentity(destination, assembly.identity); err != nil {
		return nil, err
	}
	manifestRef, err := m.graphRetainJSON(ctx, base.OperationAttemptID, "source_manifest", manifest)
	if err != nil {
		result.EvidenceStatus = "uncertain"
		return nil, err
	}
	source.ManifestArtifactID = manifestRef.ArtifactID
	indexMeta, err := m.RetainIndex(ctx, base.OperationAttemptID, source)
	if err != nil {
		result.EvidenceStatus = "uncertain"
		return nil, err
	}
	source.IndexArtifactID = indexMeta.ID
	indexRef := graphIndexRef(indexMeta)
	record.Index = &indexRef
	result.EffectsKnown = true
	return source, nil
}
