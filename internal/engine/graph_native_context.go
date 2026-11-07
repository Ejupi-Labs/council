package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"
	"sort"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// Paths and handles remain private launch state. Any partial preparation is
// returned with its real effects so the caller can settle without retrying I/O.
type graphNativeContext struct {
	Prepared       workspace.GraphPreparedContext
	Final          *graphFinalVerification
	Dispatch       domain.GraphDispatchRecord
	EffectsKnown   bool
	EvidenceStatus string
	Artifacts      []domain.GraphArtifactRef
}

func (p *graphNativeContext) Close() error {
	if p == nil {
		return nil
	}
	return p.Prepared.Close()
}

func (e *Engine) prepareGraphNativeContext(ctx context.Context, a domain.Attempt, launch adapters.Launch, final ...*graphFinalVerification) (adapters.Launch, *graphNativeContext, error) {
	return e.prepareGraphStageContext(ctx, a, launch, "native", final...)
}

func (e *Engine) prepareGraphCheckContext(ctx context.Context, a domain.Attempt, final *graphFinalVerification) (*graphNativeContext, error) {
	_, prepared, err := e.prepareGraphStageContext(ctx, a, adapters.Launch{}, "local_check", final)
	return prepared, err
}

func (e *Engine) prepareGraphStageContext(ctx context.Context, a domain.Attempt, launch adapters.Launch, executor string, final ...*graphFinalVerification) (adapters.Launch, *graphNativeContext, error) {
	if len(final) > 1 {
		return adapters.Launch{}, nil, domain.Invalid("one exact supplemental final context")
	}
	member, err := e.graphStageContextMembership(ctx, a, executor)
	if err != nil {
		return adapters.Launch{}, nil, err
	}
	if member == nil {
		if len(final) == 1 && final[0] != nil {
			return adapters.Launch{}, nil, domain.Fail("graph_input_stale", "Cumulative context cannot attach to a standalone launch.")
		}
		return launch, nil, nil
	}
	prepared := &graphNativeContext{EffectsKnown: true, EvidenceStatus: "complete", Artifacts: []domain.GraphArtifactRef{}}
	dispatch, input, packet, err := e.loadGraphStageContext(ctx, a, *member, executor)
	if err != nil {
		return adapters.Launch{}, prepared, err
	}
	prepared.Dispatch = dispatch
	var selected *graphFinalVerification
	if len(final) == 1 {
		selected = final[0]
	}
	if selected == nil {
		if dispatch.Public.Dispatch.SourceBasis.FinalVerificationRef != nil {
			return adapters.Launch{}, prepared, domain.Fail("graph_context_unavailable", "This receiving dispatch requires its exact cumulative final context.")
		}
	} else {
		fresh, checkErr := e.graphFinalContextForDispatch(ctx, a, selected.subject)
		if checkErr != nil {
			return adapters.Launch{}, prepared, checkErr
		}
		if fresh == nil || fresh.BindingRef != selected.BindingRef || fresh.Binding.BindingSHA256 != selected.Binding.BindingSHA256 {
			return adapters.Launch{}, prepared, domain.Fail("graph_input_stale", "The selected cumulative final context changed before private copying.")
		}
		prepared.Final = fresh
	}
	other := 0
	if executor == "native" {
		other, err = graphContextOtherReferences(launch)
		if err != nil {
			return adapters.Launch{}, prepared, err
		}
	}
	request := workspace.GraphContextPrepareRequest{ReceivingAttemptID: a.ID, InputBinding: *input.Public.InputBinding, OtherReferenceCount: other}
	if prepared.Final != nil {
		ref := prepared.Final.BindingRef
		request.FinalVerificationRef = &ref
		request.Supplemental, err = graphFinalContextSources(prepared.Final)
		if err != nil {
			return adapters.Launch{}, prepared, err
		}
	}
	prepared.Prepared, err = e.workspace.PrepareGraphContext(ctx, request)
	if prepared.Prepared.State != "" {
		prepared.EffectsKnown, prepared.EvidenceStatus = prepared.Prepared.EffectsKnown, prepared.Prepared.EvidenceStatus
	}
	if prepared.Prepared.EffectsArtifact != nil {
		ref := *prepared.Prepared.EffectsArtifact
		// Cancellation must not turn an already acknowledged preparation record
		// into invented durability uncertainty. This performs bounded evidence
		// verification only; it cannot create or launch another native worker.
		settlementCtx, settlementCancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		_, verifyErr := e.verifyGraphArtifact(settlementCtx, ref, "graph_operation_effects")
		settlementCancel()
		if verifyErr != nil {
			prepared.EvidenceStatus = "uncertain"
			if err == nil {
				err = verifyErr
			}
		} else {
			prepared.Artifacts = append(prepared.Artifacts, ref)
		}
	}
	if err != nil {
		return adapters.Launch{}, prepared, err
	}
	if prepared.Prepared.Packet.PacketSHA256 != packet.PacketSHA256 || prepared.Prepared.Packet.InputID != packet.InputID {
		return adapters.Launch{}, prepared, evidence.ErrCorrupt
	}
	context := adapters.GraphNativeContext{Packet: prepared.Prepared.Packet, Directory: prepared.Prepared.Directory, Entries: []adapters.GraphContextEntry{}, ReceivingAttemptID: a.ID,
		FinalVerificationRef: prepared.Prepared.FinalVerificationRef, Supplemental: []adapters.GraphContextEntry{}}
	for _, entry := range prepared.Prepared.Entries {
		context.Entries = append(context.Entries, adapters.GraphContextEntry{Ref: entry.Ref, ArtifactRef: entry.ArtifactRef, Scope: entry.Scope,
			Paths: append([]string{}, entry.Paths...), Mode: entry.Mode, Path: entry.Path, Text: entry.Text})
	}
	for _, entry := range prepared.Prepared.Supplemental {
		context.Supplemental = append(context.Supplemental, adapters.GraphContextEntry{Ref: entry.Ref, ArtifactRef: entry.ArtifactRef, Scope: entry.Scope,
			Paths: append([]string{}, entry.Paths...), Mode: entry.Mode, Path: entry.Path, Text: entry.Text})
	}
	updated := launch
	if executor == "native" {
		updated, err = adapters.AddGraphContext(launch, a.ConnectionID, context)
		if err != nil {
			return adapters.Launch{}, prepared, err
		}
	}
	refs, err := e.recheckGraphStageContext(ctx, a, prepared, executor)
	if err != nil {
		return adapters.Launch{}, prepared, err
	}
	prepared.Artifacts = refs
	return updated, prepared, nil
}

func (e *Engine) recheckGraphNativeContext(ctx context.Context, a domain.Attempt, prepared *graphNativeContext) ([]domain.GraphArtifactRef, error) {
	return e.recheckGraphStageContext(ctx, a, prepared, "native")
}

func (e *Engine) recheckGraphCheckContext(ctx context.Context, a domain.Attempt, prepared *graphNativeContext) ([]domain.GraphArtifactRef, error) {
	return e.recheckGraphStageContext(ctx, a, prepared, "local_check")
}

func (e *Engine) recheckGraphStageContext(ctx context.Context, a domain.Attempt, prepared *graphNativeContext, executor string) ([]domain.GraphArtifactRef, error) {
	member, err := e.graphStageContextMembership(ctx, a, executor)
	if err != nil {
		return nil, err
	}
	if member == nil {
		if prepared != nil {
			return nil, domain.Fail("graph_input_stale", "A retained graph context cannot become a standalone launch.")
		}
		return nil, nil
	}
	if prepared == nil {
		return nil, domain.Fail("graph_context_unavailable", "This graph worker has no exact prepared context. A standalone fallback is forbidden.")
	}
	prepared.Artifacts = nil
	dispatch, _, packet, err := e.loadGraphStageContext(ctx, a, *member, executor)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(dispatch.Public.Dispatch, prepared.Dispatch.Public.Dispatch) || prepared.Prepared.ReceivingAttemptID != a.ID ||
		prepared.Prepared.Packet.PacketSHA256 != packet.PacketSHA256 || prepared.Prepared.Packet.InputID != packet.InputID {
		return nil, domain.Fail("graph_input_stale", "The retained native context does not identify this exact receiving dispatch.")
	}
	var finalRefs []domain.GraphArtifactRef
	capturedFinal := dispatch.Public.Dispatch.SourceBasis.FinalVerificationRef
	if prepared.Final == nil {
		if capturedFinal != nil || prepared.Prepared.FinalVerificationRef != nil {
			return nil, domain.Fail("graph_input_stale", "A captured cumulative final context cannot fall back to node-only evidence.")
		}
	} else {
		fresh, finalErr := e.graphFinalContextForDispatch(ctx, a, prepared.Final.subject)
		if finalErr != nil {
			return nil, finalErr
		}
		if fresh == nil || fresh.BindingRef != prepared.Final.BindingRef || fresh.Binding.BindingSHA256 != prepared.Final.Binding.BindingSHA256 ||
			prepared.Prepared.FinalVerificationRef == nil || *prepared.Prepared.FinalVerificationRef != fresh.BindingRef {
			return nil, domain.Fail("graph_input_stale", "The exact cumulative basis changed before native instructions.")
		}
		prepared.Final = fresh
		finalRefs = fresh.ArtifactRefs
	}
	refs, err := e.workspace.RecheckGraphContext(ctx, prepared.Prepared)
	if err != nil {
		prepared.EvidenceStatus = "incomplete"
		return nil, err
	}
	refs = append(refs, dispatch.Public.Dispatch.ContextPacket)
	refs = append(refs, finalRefs...)
	fresh, err := e.graphNativeContextRefs(ctx, refs)
	if err != nil {
		prepared.EvidenceStatus = "incomplete"
		return nil, err
	}
	prepared.Artifacts = fresh
	return fresh, nil
}

// Scope is observed from actual store membership. Missing graph dispatch rows or
// failed membership reads can never select the ordinary standalone launch path.
func (e *Engine) nativeContextMembership(ctx context.Context, a domain.Attempt) (*domain.GraphRef, error) {
	return e.graphStageContextMembership(ctx, a, "native")
}

func (e *Engine) graphCheckContextMembership(ctx context.Context, a domain.Attempt) (*domain.GraphRef, error) {
	return e.graphStageContextMembership(ctx, a, "local_check")
}

func (e *Engine) graphStageContextMembership(ctx context.Context, a domain.Attempt, executor string) (*domain.GraphRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	err := e.db.CheckStandaloneAuthority(ctx, a.TaskID, false)
	if err == nil {
		return nil, nil
	}
	scope := domain.SafeError(err)
	if scope.GraphRef == nil {
		return nil, err
	}
	ref := scope.GraphRef
	if ref.Kind != "node" || ref.NodeID == nil || !domain.ValidID(*ref.NodeID) || !domain.ValidID(ref.GraphID) || !domain.ValidID(ref.RootTaskID) ||
		a.ExecutorKind != executor || executor != "native" && !graphLocalCommandRole(a) || a.OwnerInstanceID != e.config.OwnerInstanceID {
		return nil, domain.Fail("graph_context_unavailable", "Native graph context belongs only to the current owner's exact node worker.")
	}
	return ref, nil
}

func (e *Engine) loadGraphNativeContext(ctx context.Context, a domain.Attempt, member domain.GraphRef) (domain.GraphDispatchRecord, domain.GraphInputRecord, domain.GraphContextPacket, error) {
	return e.loadGraphStageContext(ctx, a, member, "native")
}

func (e *Engine) loadGraphStageContext(ctx context.Context, a domain.Attempt, member domain.GraphRef, executor string) (domain.GraphDispatchRecord, domain.GraphInputRecord, domain.GraphContextPacket, error) {
	var emptyDispatch domain.GraphDispatchRecord
	var emptyInput domain.GraphInputRecord
	var emptyPacket domain.GraphContextPacket
	fail := func(err error) (domain.GraphDispatchRecord, domain.GraphInputRecord, domain.GraphContextPacket, error) {
		return emptyDispatch, emptyInput, emptyPacket, err
	}
	dispatch, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil {
		return fail(err)
	}
	d := dispatch.Public.Dispatch
	if d.GraphID != member.GraphID || member.NodeID == nil || d.NodeID != *member.NodeID || d.BudgetOwnerTaskID != member.RootTaskID || d.AttemptID != a.ID || d.TaskID != a.TaskID ||
		d.TaskVersion != a.TaskVersion || d.OwnerInstanceID != a.OwnerInstanceID || d.OwnerInstanceID != e.config.OwnerInstanceID || d.ExecutorKind != executor ||
		d.ExecutorKind != a.ExecutorKind || d.Role != a.Role || d.WorkflowID != a.WorkflowID || d.StepID != a.StepID || !domain.ValidID(d.InputID) || !domain.ValidSHA256(d.InputSHA256) ||
		d.SourceBasis.InputID != d.InputID || d.SourceBasis.InputSHA256 != d.InputSHA256 || d.SourceBasis.WorkflowID != d.WorkflowID || d.SourceBasis.StepID != d.StepID {
		return fail(domain.Fail("graph_input_stale", "Native context requires the exact receiving attempt, workflow, role and input dispatch."))
	}
	graph, err := e.db.Graph(ctx, d.GraphID)
	if err != nil {
		return fail(err)
	}
	current := false
	for _, node := range graph.Nodes {
		if node.ID == d.NodeID && node.TaskID == a.TaskID && node.TaskVersion == a.TaskVersion {
			current = true
			break
		}
	}
	if graph.Graph.RootTaskID != member.RootTaskID || !current {
		return fail(domain.Fail("graph_input_stale", "The receiving node or task version is no longer current."))
	}
	input, err := e.db.GraphInputRecord(ctx, d.GraphID, d.InputID)
	if err != nil {
		return fail(err)
	}
	binding := input.Public.InputBinding
	if input.Public.Summary.State != "ready" || !input.Public.Summary.Current || binding == nil || input.Public.ContextPacket == nil || input.ConditionsBinding == nil ||
		binding.ID != d.InputID || binding.GraphID != d.GraphID || binding.GraphVersion != d.GraphVersion || binding.ReceivingNodeID != d.NodeID ||
		binding.ReceivingTaskID != a.TaskID || binding.ReceivingTaskVersion != a.TaskVersion || binding.InputSHA256 != d.InputSHA256 || binding.ContextPacket != d.ContextPacket {
		return fail(domain.Fail("graph_input_stale", "The dispatch has no exact current sealed receiving input and context packet."))
	}
	if invalid := domain.ValidateGraphInputBinding(*binding); invalid != nil {
		return fail(invalid)
	}
	body, err := e.readGraphArtifact(ctx, d.ContextPacket, "graph_context", domain.MaxPromptBytes)
	if err != nil {
		return fail(err)
	}
	var packet domain.GraphContextPacket
	if err = json.Unmarshal(body, &packet, json.RejectUnknownMembers(true)); err != nil {
		return fail(evidence.ErrCorrupt)
	}
	seal, err := domain.GraphContextDigest(packet)
	savedSeal, savedErr := domain.GraphContextDigest(*input.Public.ContextPacket)
	if err != nil || savedErr != nil || seal != packet.PacketSHA256 || savedSeal != seal || input.Public.ContextPacket.PacketSHA256 != seal ||
		packet.GraphID != d.GraphID || packet.InputID != d.InputID || packet.InputSHA256 != d.InputSHA256 || packet.ReceivingTaskVersion != a.TaskVersion {
		return fail(evidence.ErrCorrupt)
	}
	return dispatch, input, packet, nil
}

// The returned inventory is complete fresh physical evidence, never a proposed
// SQL mutation. The caller chooses bounded closure anchors at its own boundary.
func (e *Engine) graphNativeContextRefs(ctx context.Context, refs []domain.GraphArtifactRef) ([]domain.GraphArtifactRef, error) {
	seen := map[string]domain.GraphArtifactRef{}
	for _, ref := range refs {
		if prior, ok := seen[ref.ArtifactID]; ok {
			if prior != ref {
				return nil, evidence.ErrCorrupt
			}
			continue
		}
		meta, err := e.evidence.VerifyClosed(ctx, ref.ArtifactID)
		if err != nil {
			return nil, err
		}
		if !validGraphArtifactReference(ref) || !graphArtifactMatches(meta, ref, meta.Kind) {
			return nil, evidence.ErrCorrupt
		}
		seen[ref.ArtifactID] = ref
	}
	result := make([]domain.GraphArtifactRef, 0, len(seen))
	for _, ref := range seen {
		result = append(result, ref)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ArtifactID < result[j].ArtifactID })
	return result, nil
}
