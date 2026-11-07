package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type GraphContextPrepareRequest struct {
	ReceivingAttemptID   string
	InputBinding         domain.GraphInputBinding
	FinalVerificationRef *domain.GraphArtifactRef
	Supplemental         []GraphContextSupplement
	OtherReferenceCount  int
}

// GraphContextEntry is private launch data. Physical paths/identity and inline
// bytes are not added to the public graph input/context packet.
type GraphContextEntry struct {
	Ref         string
	ArtifactRef domain.GraphArtifactRef
	Scope       string
	Paths       []string
	Mode        string
	Path        string
	Text        *string
	Identity    string
}

type GraphPreparedContext struct {
	ReceivingAttemptID           string
	Packet                       domain.GraphContextPacket
	Directory, DirectoryIdentity string
	Entries                      []GraphContextEntry
	FinalVerificationRef         *domain.GraphArtifactRef
	Supplemental                 []GraphContextEntry
	DescriptorPath               string
	DescriptorIdentity           string
	DescriptorSHA256             string
	descriptorEntries            []GraphContextEntry
	supplementalPlan             []GraphContextSupplement
	otherReferenceCount          int
	EffectsArtifact              *domain.GraphArtifactRef
	EffectsKnown                 bool
	EvidenceStatus, State        string
	root                         *os.Root
}

func (p *GraphPreparedContext) Close() error {
	if p.root == nil {
		return nil
	}
	err := p.root.Close()
	p.root = nil
	return err
}

type graphContextEffect struct {
	Ref         string                  `json:"ref"`
	ArtifactRef domain.GraphArtifactRef `json:"artifactRef"`
	Name        string                  `json:"name"`
	Mode        string                  `json:"mode"`
	State       string                  `json:"state"`
}
type graphContextEffects struct {
	Contract             string                   `json:"contract"`
	ReceivingAttemptID   string                   `json:"receivingAttemptId"`
	InputID              string                   `json:"inputId"`
	PacketSHA256         string                   `json:"packetSha256"`
	Entries              []graphContextEffect     `json:"entries"`
	EffectsKnown         bool                     `json:"effectsKnown"`
	State                string                   `json:"state"`
	FinalVerificationRef *domain.GraphArtifactRef `json:"finalVerificationRef,omitzero"`
	Supplemental         []GraphContextSupplement `json:"supplemental,omitzero"`
	DescriptorName       string                   `json:"descriptorName,omitzero"`
	DescriptorSHA256     string                   `json:"descriptorSha256,omitzero"`
	DescriptorState      string                   `json:"descriptorState,omitzero"`
}

func (m *Manager) PrepareGraphContext(ctx context.Context, request GraphContextPrepareRequest) (result GraphPreparedContext, operationErr error) {
	if !validManagedID(request.ReceivingAttemptID) {
		return result, ErrBlocked
	}
	verified, err := m.VerifyGraphInput(ctx, request.InputBinding)
	if err != nil {
		return result, err
	}
	packet := verified.ContextPacket
	if err = graphContextReferenceBudget(packet, request.Supplemental, request.OtherReferenceCount); err != nil {
		return result, err
	}
	if err = m.graphValidateFinalContext(ctx, request, packet); err != nil {
		return result, err
	}
	accesses := append([]domain.GraphProvidedAccess{}, packet.ProvidedAccess...)
	for _, source := range request.Supplemental {
		accesses = append(accesses, domain.GraphProvidedAccess{Ref: source.Ref, ArtifactRef: source.ArtifactRef, Scope: source.Scope, Paths: append([]string{}, source.Paths...), Mode: "file"})
	}
	var total int64
	seen := map[string]bool{}
	for _, access := range accesses {
		if access.Mode != "file" && access.Mode != "inline" {
			return result, ErrBlocked
		}
		count, invalid := domain.ParseCount(access.ArtifactRef.ByteLength)
		if invalid != nil {
			return result, invalid
		}
		if count > MaxFileBytes {
			return result, domain.Fail("context_capacity", "A required context artifact exceeds the64MiB local file transport bound.")
		}
		if !seen[access.ArtifactRef.ArtifactID] {
			total += count
			seen[access.ArtifactRef.ArtifactID] = true
		}
		if total > MaxTotalBytes {
			return result, domain.Fail("context_capacity", "The complete context exceeds the512MiB local transport bound.")
		}
	}
	result = GraphPreparedContext{ReceivingAttemptID: request.ReceivingAttemptID, Packet: packet, Entries: []GraphContextEntry{}, EffectsKnown: true, EvidenceStatus: "complete", State: "incomplete",
		Supplemental: []GraphContextEntry{}, supplementalPlan: []GraphContextSupplement{}, otherReferenceCount: request.OtherReferenceCount}
	if request.FinalVerificationRef != nil {
		ref := *request.FinalVerificationRef
		result.FinalVerificationRef = &ref
		for _, source := range request.Supplemental {
			source.Paths = append([]string{}, source.Paths...)
			result.supplementalPlan = append(result.supplementalPlan, source)
		}
	}
	effects := graphContextEffects{Contract: "006-context-preparation-v1", ReceivingAttemptID: request.ReceivingAttemptID, InputID: packet.InputID, PacketSHA256: packet.PacketSHA256, Entries: []graphContextEffect{}, EffectsKnown: true, State: "incomplete",
		FinalVerificationRef: result.FinalVerificationRef, Supplemental: result.supplementalPlan}
	stream, err := m.artifacts.Create(ctx, request.ReceivingAttemptID, "graph_operation_effects", MaxFileBytes)
	if err != nil {
		result.State, result.EvidenceStatus = "uncertain", "uncertain"
		return result, err
	}
	defer func() {
		if operationErr != nil && result.State == "complete" {
			result.State = "incomplete"
		}
		effects.State, effects.EffectsKnown = result.State, result.EffectsKnown
		body, writeErr := json.Marshal(effects)
		if writeErr == nil {
			_, writeErr = stream.Write(body)
		}
		settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		meta, closeErr := stream.Close(settlementCtx, writeErr == nil)
		if writeErr == nil && closeErr == nil {
			ref := graphIndexRef(meta)
			result.EffectsArtifact = &ref
		} else {
			result.State, result.EvidenceStatus = "uncertain", "uncertain"
			operationErr = errors.Join(operationErr, writeErr, closeErr)
		}
		// A final descriptor failure returns the actual held prefix for its
		// owner to settle. It never makes the directory available for reuse.
		if operationErr != nil && result.FinalVerificationRef == nil {
			operationErr = errors.Join(operationErr, result.Close())
		}
	}()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	dir := "workspaces/graph-context-" + request.ReceivingAttemptID
	result.Directory = filepath.Join(m.lock.Root(), filepath.FromSlash(dir))
	if err = m.root.Mkdir(dir, 0700); err != nil {
		result.State, result.EffectsKnown = "uncertain", false
		return result, ErrBlocked
	}
	directory, err := m.root.Open(dir)
	if err != nil {
		return result, err
	}
	if err = localstate.ProtectPrivateFile(directory, true); err != nil {
		directory.Close()
		return result, err
	}
	result.DirectoryIdentity, err = graphContextFileIdentity(directory)
	closeErr := directory.Close()
	if err != nil || closeErr != nil {
		return result, errors.Join(err, closeErr)
	}
	result.root, err = m.root.OpenRoot(dir)
	if err != nil {
		return result, err
	}
	cache := map[string]GraphContextEntry{}
	for ordinal, access := range accesses {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		entry := GraphContextEntry{Ref: access.Ref, ArtifactRef: access.ArtifactRef, Scope: access.Scope, Paths: append([]string{}, access.Paths...), Mode: access.Mode}
		if access.Mode == "inline" {
			var body bytes.Buffer
			meta, readErr := m.artifacts.CopyTo(ctx, access.ArtifactRef.ArtifactID, &body, MaxFileBytes)
			if readErr != nil || graphIndexRef(meta) != access.ArtifactRef {
				return result, errors.Join(evidence.ErrCorrupt, readErr)
			}
			if !utf8.Valid(body.Bytes()) {
				return result, domain.Fail("context_unavailable", "Required inline context is not lossless UTF8 text.")
			}
			text := body.String()
			entry.Text = &text
			if result.FinalVerificationRef == nil {
				result.Entries = append(result.Entries, entry)
				effects.Entries = append(effects.Entries, graphContextEffect{Ref: entry.Ref, ArtifactRef: entry.ArtifactRef, Mode: entry.Mode, State: "inline_ready"})
				continue
			}
		}
		copied, copyErr := m.graphCopyContextEntry(ctx, &result, &effects, access, ordinal, cache)
		if copyErr != nil {
			return result, copyErr
		}
		if result.FinalVerificationRef != nil {
			result.descriptorEntries = append(result.descriptorEntries, copied)
		}
		if ordinal < len(packet.ProvidedAccess) {
			if access.Mode == "file" {
				entry = copied
			}
			result.Entries = append(result.Entries, entry)
		} else {
			result.Supplemental = append(result.Supplemental, copied)
		}
	}
	if result.FinalVerificationRef != nil {
		if err = m.graphWriteFinalDescriptor(ctx, &result, &effects, total); err != nil {
			return result, err
		}
	}
	if err = localstate.SyncPrivateDirectory(result.Directory); err != nil {
		result.State, result.EvidenceStatus = "uncertain", "uncertain"
		return result, err
	}
	if err = localstate.SyncPrivateDirectory(filepath.Dir(result.Directory)); err != nil {
		result.State, result.EvidenceStatus = "uncertain", "uncertain"
		return result, err
	}
	if _, err = m.graphVerifyContextCopies(ctx, result); err != nil {
		return result, err
	}
	result.State = "complete"
	return result, nil
}

func (m *Manager) RecheckGraphContext(ctx context.Context, prepared GraphPreparedContext) ([]domain.GraphArtifactRef, error) {
	if prepared.State != "complete" || !prepared.EffectsKnown || prepared.EvidenceStatus != "complete" || prepared.EffectsArtifact == nil || prepared.EffectsArtifact.ProducerAttemptID != prepared.ReceivingAttemptID {
		return nil, ErrBlocked
	}
	body, err := m.graphIndexLoad(ctx, *prepared.EffectsArtifact, "graph_operation_effects")
	if err != nil {
		return nil, err
	}
	var effects graphContextEffects
	if err = json.Unmarshal(body, &effects, json.RejectUnknownMembers(true)); err != nil || effects.Contract != "006-context-preparation-v1" || effects.State != "complete" || !effects.EffectsKnown || effects.ReceivingAttemptID != prepared.ReceivingAttemptID || effects.InputID != prepared.Packet.InputID || effects.PacketSHA256 != prepared.Packet.PacketSHA256 {
		return nil, evidence.ErrCorrupt
	}
	if !reflect.DeepEqual(effects.FinalVerificationRef, prepared.FinalVerificationRef) || !reflect.DeepEqual(effects.Supplemental, prepared.supplementalPlan) {
		return nil, evidence.ErrCorrupt
	}
	if prepared.FinalVerificationRef != nil {
		if effects.DescriptorName != "final-context.json" || effects.DescriptorState != "copied" || effects.DescriptorSHA256 != prepared.DescriptorSHA256 {
			return nil, evidence.ErrCorrupt
		}
	} else if effects.DescriptorName != "" || effects.DescriptorState != "" || effects.DescriptorSHA256 != "" {
		return nil, evidence.ErrCorrupt
	}
	refs, err := m.graphVerifyContextCopies(ctx, prepared)
	if err != nil {
		return nil, err
	}
	return append(refs, *prepared.EffectsArtifact), nil
}

func (m *Manager) graphVerifyContextCopies(ctx context.Context, prepared GraphPreparedContext) ([]domain.GraphArtifactRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := m.lock.CheckHeld(); err != nil {
		return nil, err
	}
	if !validManagedID(prepared.ReceivingAttemptID) || prepared.DirectoryIdentity == "" || prepared.Directory != filepath.Join(m.lock.Root(), "workspaces", "graph-context-"+prepared.ReceivingAttemptID) || len(prepared.Entries) != len(prepared.Packet.ProvidedAccess) || len(prepared.Entries) > 64 {
		return nil, ErrBlocked
	}
	seal, err := domain.GraphContextDigest(prepared.Packet)
	if err != nil || seal != prepared.Packet.PacketSHA256 {
		return nil, evidence.ErrCorrupt
	}
	pathInfo, err := m.root.Lstat("workspaces/graph-context-" + prepared.ReceivingAttemptID)
	if err != nil || !pathInfo.IsDir() || pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, ErrBlocked
	}
	root := prepared.root
	if root == nil {
		root, err = m.root.OpenRoot("workspaces/graph-context-" + prepared.ReceivingAttemptID)
		if err != nil {
			return nil, err
		}
		defer root.Close()
	}
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	id, idErr := graphContextFileIdentity(directory)
	privateErr := localstate.VerifyPrivateFile(directory, true)
	opened, statErr := directory.Stat()
	closeErr := directory.Close()
	if idErr != nil || privateErr != nil || statErr != nil || closeErr != nil || id != prepared.DirectoryIdentity || !os.SameFile(pathInfo, opened) {
		return nil, evidence.ErrCorrupt
	}
	refs := []domain.GraphArtifactRef{}
	for i, entry := range prepared.Entries {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		access := prepared.Packet.ProvidedAccess[i]
		if entry.Ref != access.Ref || entry.ArtifactRef != access.ArtifactRef || entry.Scope != access.Scope || entry.Mode != access.Mode || !graphContextPathsEqual(entry.Paths, access.Paths) {
			return nil, evidence.ErrCorrupt
		}
		if err = m.graphVerifyRef(ctx, entry.ArtifactRef); err != nil {
			return nil, err
		}
		refs = append(refs, entry.ArtifactRef)
		if entry.Mode == "inline" {
			if entry.Text == nil || entry.Path != "" || !utf8.ValidString(*entry.Text) || hashBytes([]byte(*entry.Text)) != entry.ArtifactRef.SHA256 || strconv.Itoa(len([]byte(*entry.Text))) != entry.ArtifactRef.ByteLength {
				return nil, evidence.ErrCorrupt
			}
			continue
		}
		if entry.Mode != "file" || entry.Text != nil || entry.Identity == "" || filepath.Dir(entry.Path) != prepared.Directory {
			return nil, evidence.ErrCorrupt
		}
		name := filepath.Base(entry.Path)
		if len(name) != 10 || name[6:] != ".txt" {
			return nil, ErrBlocked
		}
		for _, r := range name[:6] {
			if r < '0' || r > '9' {
				return nil, ErrBlocked
			}
		}
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() {
			return nil, evidence.ErrCorrupt
		}
		file, err := graphContextOpenRead(root, name)
		if err != nil {
			return nil, err
		}
		verifyErr := localstate.VerifyPrivateFile(file, false)
		fileID, fileIDErr := graphContextFileIdentity(file)
		before, beforeErr := file.Stat()
		if verifyErr != nil || fileIDErr != nil || beforeErr != nil || fileID != entry.Identity {
			file.Close()
			return nil, evidence.ErrCorrupt
		}
		h := sha256.New()
		count, readErr := io.Copy(h, io.LimitReader(file, MaxFileBytes+1))
		after, afterErr := file.Stat()
		pathErr := localstate.VerifyPrivateFile(file, false)
		fileCloseErr := file.Close()
		if readErr != nil || afterErr != nil || pathErr != nil || fileCloseErr != nil || count > MaxFileBytes || count != before.Size() || count != after.Size() || !before.ModTime().Equal(after.ModTime()) || hex.EncodeToString(h.Sum(nil)) != entry.ArtifactRef.SHA256 || strconv.FormatInt(count, 10) != entry.ArtifactRef.ByteLength {
			return nil, evidence.ErrCorrupt
		}
	}
	finalRefs, err := m.graphVerifyFinalContext(ctx, prepared, root)
	if err != nil {
		return nil, err
	}
	refs = append(refs, finalRefs...)
	current, err := m.root.Open("workspaces/graph-context-" + prepared.ReceivingAttemptID)
	if err != nil {
		return nil, err
	}
	currentID, identityErr := graphContextFileIdentity(current)
	currentPrivateErr := localstate.VerifyPrivateFile(current, true)
	currentCloseErr := current.Close()
	if identityErr != nil || currentPrivateErr != nil || currentCloseErr != nil || currentID != prepared.DirectoryIdentity {
		return nil, evidence.ErrCorrupt
	}
	return refs, nil
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
