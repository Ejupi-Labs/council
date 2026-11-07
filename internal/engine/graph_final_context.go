package engine

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// This reducer selects supplementary closed evidence only. The loader and
// dispatch guards separately establish the selected final and current authority.
func graphFinalContextSources(verified *graphFinalVerification) ([]workspace.GraphContextSupplement, error) {
	if verified == nil {
		return nil, nil
	}
	inventory, err := graphFinalContextInventory(verified.ArtifactRefs)
	if err != nil {
		return nil, err
	}
	binding, closure := verified.Binding, verified.OutputClosure
	seal, err := domain.GraphFinalVerificationDigest(binding)
	if err != nil || seal != binding.BindingSHA256 || !reflect.DeepEqual(binding.RootBinding, domain.GraphRootPublic(verified.Root)) ||
		binding.FinalInputID != verified.Input.ID || closure.CandidateID == nil || *closure.CandidateID != binding.FinalCandidateBinding.CandidateID ||
		closure.GraphID != binding.GraphID || closure.NodeID != binding.FinalNodeID || closure.ReceivedInputID != binding.FinalInputID ||
		closure.Manifest.ArtifactID != binding.FinalCandidateBinding.ManifestArtifactID || closure.Manifest.SHA256 != binding.FinalCandidateBinding.ManifestSHA256 ||
		closure.Index.ArtifactID != binding.FinalCandidateBinding.IndexArtifactID || closure.Index.SHA256 != binding.FinalCandidateBinding.IndexSHA256 ||
		!reflect.DeepEqual(verified.Changes, workspace.CompareManifests(verified.RootProof.Manifest, verified.OutputProof.Manifest)) {
		return nil, evidence.ErrCorrupt
	}
	result := []workspace.GraphContextSupplement{}
	for _, header := range []struct {
		name string
		ref  domain.GraphArtifactRef
	}{
		{"final-verification", verified.BindingRef},
		{"final-cumulative-delta", binding.CumulativeDeltaRef},
		{"final-root-manifest", verified.Root.Manifest},
		{"final-root-index", verified.Root.Index},
		{"final-root-staged-blobs", verified.Root.StagedBlobs},
		{"final-output-closure", binding.OutputClosureRef},
		{"final-output-manifest", closure.Manifest},
		{"final-output-index", closure.Index},
		{"final-output-staged-blobs", closure.StagedBlobs},
	} {
		if actual, ok := inventory[header.ref.ArtifactID]; !ok || actual != header.ref {
			return nil, evidence.ErrCorrupt
		}
		result = append(result, workspace.GraphContextSupplement{Ref: header.name, ArtifactRef: header.ref, Scope: "cumulative_final", Paths: []string{}})
	}
	members, err := graphFinalContextMemberSources(verified.Changes, verified.RootProof.StagedBlobs, verified.OutputProof.StagedBlobs, verified.ArtifactRefs)
	if err != nil {
		return nil, err
	}
	result = append(result, members...)
	if len(result) > 64 {
		return nil, domain.Fail("context_capacity", "The complete cumulative final context exceeds64 references; no evidence was clipped.")
	}
	return result, nil
}

// Member reduction is independently testable without inventing a graph
// acceptance or physical proof. Callers supply the loader's verified inventory.
func graphFinalContextMemberSources(changes []workspace.Change, before, after workspace.GraphIndexBlobManifest, refs []domain.GraphArtifactRef) ([]workspace.GraphContextSupplement, error) {
	if len(changes) == 0 {
		return nil, nil
	}
	inventory, err := graphFinalContextInventory(refs)
	if err != nil {
		return nil, err
	}
	blobs := func(manifest workspace.GraphIndexBlobManifest) (map[string]workspace.GraphIndexBlob, error) {
		if manifest.Contract != "006-index-blobs-v1" {
			return nil, evidence.ErrCorrupt
		}
		result := map[string]workspace.GraphIndexBlob{}
		for _, blob := range manifest.Entries {
			actual, ok := inventory[blob.ArtifactRef.ArtifactID]
			if !ok || actual != blob.ArtifactRef || blob.ContentSHA256 != actual.SHA256 || blob.ByteLength != actual.ByteLength || blob.ObjectID == "" {
				return nil, evidence.ErrCorrupt
			}
			if old, exists := result[blob.ObjectID]; exists && old != blob {
				return nil, evidence.ErrCorrupt
			}
			result[blob.ObjectID] = blob
		}
		return result, nil
	}
	beforeBlobs, err := blobs(before)
	if err != nil {
		return nil, err
	}
	afterBlobs, err := blobs(after)
	if err != nil {
		return nil, err
	}
	ordered := append([]workspace.Change{}, changes...)
	slices.SortFunc(ordered, func(a, b workspace.Change) int { return strings.Compare(a.Path, b.Path) })
	result := []workspace.GraphContextSupplement{}
	add := func(name, sourcePath string, ref domain.GraphArtifactRef) {
		result = append(result, workspace.GraphContextSupplement{Ref: name, ArtifactRef: ref, Scope: "cumulative_final", Paths: []string{sourcePath}})
	}
	for i, change := range ordered {
		if change.Path == "" || path.Clean(change.Path) != change.Path || strings.HasPrefix(change.Path, "/") || change.Path == "." ||
			change.Path == ".." || strings.HasPrefix(change.Path, "../") || strings.Contains(change.Path, "\\") || i > 0 && ordered[i-1].Path == change.Path {
			return nil, evidence.ErrCorrupt
		}
		if entry := change.Before; entry != nil {
			if entry.Path != change.Path {
				return nil, evidence.ErrCorrupt
			}
			if entry.Type == "file" {
				ref, ok := inventory[entry.ArtifactID]
				if !ok || entry.Size < 0 || ref.SHA256 != entry.SHA256 || ref.ByteLength != strconv.FormatInt(entry.Size, 10) {
					return nil, evidence.ErrCorrupt
				}
				add(fmt.Sprintf("final-before-%04d", i+1), change.Path, ref)
			}
		}
		for _, side := range []struct {
			label string
			entry *workspace.IndexEntry
			blobs map[string]workspace.GraphIndexBlob
		}{{"before", change.BeforeIndex, beforeBlobs}, {"after", change.AfterIndex, afterBlobs}} {
			if side.entry == nil {
				continue
			}
			blob, ok := side.blobs[side.entry.Object]
			if !ok || side.entry.Path != change.Path {
				return nil, evidence.ErrCorrupt
			}
			add(fmt.Sprintf("final-staged-%s-%04d", side.label, i+1), change.Path, blob.ArtifactRef)
		}
	}
	if len(result) > 64 {
		return nil, domain.Fail("context_capacity", "The complete cumulative before/staged context exceeds64 references; no evidence was clipped.")
	}
	return result, nil
}

func graphFinalContextInventory(refs []domain.GraphArtifactRef) (map[string]domain.GraphArtifactRef, error) {
	result := map[string]domain.GraphArtifactRef{}
	for _, ref := range refs {
		if invalid := domain.ValidateGraphArtifactRef(ref); invalid != nil {
			return nil, invalid
		}
		if old, ok := result[ref.ArtifactID]; ok && old != ref {
			return nil, evidence.ErrCorrupt
		}
		result[ref.ArtifactID] = ref
	}
	return result, nil
}

func graphContextOtherReferences(launch adapters.Launch) (int, error) {
	line := bytes.IndexByte(launch.Prompt, '\n')
	if line < 0 {
		return 0, domain.Invalid("the complete encoded native prompt before context preparation")
	}
	var fields map[string]jsontext.Value
	if json.Unmarshal(launch.Prompt[line+1:], &fields) != nil || fields == nil {
		return 0, domain.Invalid("the exact native prompt before context preparation")
	}
	raw, present := fields["evidence"]
	if !present {
		return 0, nil
	}
	var supplied []adapters.SuppliedEvidence
	if raw.Kind() != '[' || json.Unmarshal(raw, &supplied) != nil || len(supplied) > 64 {
		return 0, domain.Invalid("the bounded actual native role reference count")
	}
	return len(supplied), nil
}

func (e *Engine) graphFinalContextForDispatch(ctx context.Context, a domain.Attempt, subject workflowSubject) (*graphFinalVerification, error) {
	verified, err := e.loadGraphFinalVerification(ctx, subject)
	if err != nil {
		return nil, err
	}
	record, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	d := record.Public.Dispatch
	if d.AttemptID != a.ID || d.WorkflowID != a.WorkflowID || d.StepID != a.StepID || d.TaskID != a.TaskID || d.TaskVersion != a.TaskVersion {
		return nil, domain.Fail("graph_input_stale", "The cumulative context has no exact receiving dispatch.")
	}
	if verified == nil {
		if d.SourceBasis.FinalVerificationRef != nil {
			return nil, domain.Fail("graph_input_stale", "A captured final basis cannot fall back to ordinary node context.")
		}
		return nil, nil
	}
	if d.SourceBasis.BasisKind != "node_subject" || d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != verified.BindingRef ||
		d.SourceBasis.CandidateBinding == nil || *d.SourceBasis.CandidateBinding != verified.Binding.FinalCandidateBinding {
		return nil, domain.Fail("graph_input_stale", "The receiving stage does not capture this exact selected cumulative final basis.")
	}
	return verified, nil
}

func (e *Engine) recheckWorkflowFinalContext(ctx context.Context, a domain.Attempt, stage *workflowNativeContext) error {
	if stage == nil || stage.final == nil {
		return nil
	}
	fresh, err := e.graphFinalContextForDispatch(ctx, a, stage.subject)
	if err != nil {
		return err
	}
	if fresh == nil || fresh.BindingRef != stage.final.BindingRef || fresh.Binding.BindingSHA256 != stage.final.Binding.BindingSHA256 {
		return domain.Fail("graph_input_stale", "The selected cumulative final basis changed before settlement.")
	}
	stage.final = fresh
	return nil
}

type graphCheckCommandContext struct {
	Final        *graphFinalVerification
	Receiving    *graphReceivingSource
	Context      *graphNativeContext
	Snapshot     *workspace.Snapshot
	EffectsKnown bool
}

func (p *graphCheckCommandContext) close() error {
	if p == nil || p.Context == nil {
		return nil
	}
	err := p.Context.Close()
	if err != nil {
		p.EffectsKnown = false
	}
	return err
}

func (p *graphCheckCommandContext) effectRefs() []domain.GraphArtifactRef {
	if p != nil && p.Context != nil && p.Context.Prepared.EffectsArtifact != nil {
		return []domain.GraphArtifactRef{*p.Context.Prepared.EffectsArtifact}
	}
	return nil
}

func (e *Engine) guardGraphCheckPreparation(ctx context.Context, a domain.Attempt) error {
	member, err := e.graphCheckContextMembership(ctx, a)
	if err != nil {
		return err
	}
	if member == nil {
		return domain.Fail("graph_input_stale", "The guarded local check cannot become a standalone executor.")
	}
	return e.guardGraphNativePreparation(ctx, a)
}

func (e *Engine) prepareGraphCheckHandoff(ctx context.Context, a domain.Attempt, subject workflowSubject, prepared *graphCheckCommandContext) error {
	member, err := e.graphCheckContextMembership(ctx, a)
	if err != nil || member == nil {
		if err == nil {
			err = domain.Fail("graph_input_stale", "The exact graph local-check receiver is unavailable.")
		}
		return err
	}
	dispatch, _, _, err := e.loadGraphStageContext(ctx, a, *member, "local_check")
	if err != nil {
		return err
	}
	basis := dispatch.Public.Dispatch.SourceBasis
	if basis.BasisKind != "node_subject" || !reflect.DeepEqual(basis.CandidateBinding, subject.Binding) || !reflect.DeepEqual(basis.ResponseBinding, subject.Response) {
		return domain.Fail("graph_input_stale", "A local graph check requires its exact retained selected subject, never an input seeder.")
	}
	if err = e.guardGraphCheckPreparation(ctx, a); err != nil {
		return err
	}
	manifestID := subject.Source.ManifestArtifactID
	if subject.Binding != nil {
		manifestID = subject.Binding.ManifestArtifactID
	}
	// Once Handoff is attempted, only its exact complete observation and atomic
	// acknowledgement may establish known receiving ownership again.
	prepared.EffectsKnown = false
	snapshot, handoffErr := e.workspace.Handoff(ctx, workspace.HandoffRequest{AttemptID: a.ID, Source: subject.snapshot(subject.Workflow.Authority.Delivery), CandidateManifestArtifactID: manifestID, IndexArtifactID: subject.ResponseIndexID()})
	prepared.Snapshot = snapshot
	if snapshot == nil {
		return handoffErr
	}
	observeCtx, observeCancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer observeCancel()
	receiving, observeErr := e.prepareGraphCheckReceivingSource(observeCtx, a, snapshot)
	prepared.Receiving = receiving
	var recordErr error
	if receiving != nil && validGraphArtifactReference(receiving.Materialized.ReceiptArtifact) {
		recordErr = e.recordGraphReceivingSource(observeCtx, a, receiving)
	}
	if receiving != nil {
		m := receiving.Materialized
		prepared.EffectsKnown = observeErr == nil && recordErr == nil && m.Receipt.State == "complete" && m.Receipt.EffectsKnown &&
			m.Receipt.EvidenceStatus == "complete" && m.Snapshot != nil && validGraphArtifactReference(m.ReceiptArtifact)
	}
	return errors.Join(handoffErr, observeErr, recordErr)
}

func (e *Engine) recheckGraphCheckCommand(ctx context.Context, a domain.Attempt, prepared *graphCheckCommandContext) error {
	if prepared == nil {
		return nil
	}
	if err := e.recheckGraphReceivingSource(ctx, a, prepared.Receiving); err != nil {
		return err
	}
	if err := e.recheckGraphExternalCommandScope(ctx, a); err != nil {
		return err
	}
	if prepared.Final != nil {
		if prepared.Context == nil {
			return domain.Fail("graph_context_unavailable", "The final local check has no exact private descriptor.")
		}
		if _, err := e.recheckGraphStageContext(ctx, a, prepared.Context, a.ExecutorKind); err != nil {
			return err
		}
		prepared.Final = prepared.Context.Final
	}
	return nil
}

// Only this reserved key is changed. No environment values enter artifacts or
// logs; ordinary commands retain their existing nil/inherited environment path.
func graphFinalCommandEnvironment(descriptor string) ([]string, error) {
	if descriptor == "" {
		return nil, nil
	}
	if !filepath.IsAbs(descriptor) || filepath.Clean(descriptor) != descriptor || filepath.Base(descriptor) != "final-context.json" {
		return nil, domain.Fail("graph_context_unavailable", "The final check descriptor has no exact private path.")
	}
	result := []string{}
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		matches := name == "COUNCIL_GRAPH_CONTEXT_FILE"
		if runtime.GOOS == "windows" {
			matches = strings.EqualFold(name, "COUNCIL_GRAPH_CONTEXT_FILE")
		}
		if !matches {
			result = append(result, value)
		}
	}
	return append(result, "COUNCIL_GRAPH_CONTEXT_FILE="+descriptor), nil
}
