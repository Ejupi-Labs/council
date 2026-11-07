package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func graphContextReferenceBudget(packet domain.GraphContextPacket, supplements []GraphContextSupplement, other int) error {
	if other < 0 || other > 64 {
		return domain.Fail("context_capacity", "The complete context exceeds64 supplied references.")
	}
	names := map[string]domain.GraphArtifactRef{}
	add := func(name string, ref domain.GraphArtifactRef) error {
		if name == "" || len(name) > 128 || !utf8.ValidString(name) || strings.ContainsAny(name, "\x00\r\n") || domain.ValidateGraphArtifactRef(ref) != nil {
			return ErrBlocked
		}
		if old, found := names[name]; found && old != ref {
			return evidence.ErrCorrupt
		}
		names[name] = ref
		if len(names)+other > 64 {
			return domain.Fail("context_capacity", "The combined ordinary, graph and final context exceeds64 supplied references.")
		}
		return nil
	}
	for _, item := range []struct {
		name string
		ref  *domain.GraphArtifactRef
	}{{"graph-instructions", &packet.GraphInstructionsRef}, {"node-instructions", &packet.NodeInstructionsRef}, {"receiver-guidance", packet.GuidanceRef}, {"receiver-acceptance", packet.AcceptanceRef}} {
		if item.ref != nil {
			if err := add(item.name, *item.ref); err != nil {
				return err
			}
		}
	}
	for _, item := range packet.ConstraintRefs {
		if err := add(item.Ref, item.ArtifactRef); err != nil {
			return err
		}
	}
	for _, item := range packet.ContributedEvidence {
		if err := add(item.Ref, item.ArtifactRef); err != nil {
			return err
		}
	}
	for _, item := range packet.ProvidedAccess {
		if err := add(item.Ref, item.ArtifactRef); err != nil {
			return err
		}
	}
	for _, item := range supplements {
		if _, collision := names[item.Ref]; collision {
			return ErrBlocked
		}
		if item.Scope != "cumulative_final" || len(item.Paths) > 256 {
			return ErrBlocked
		}
		seen := map[string]bool{}
		for _, path := range item.Paths {
			if !domain.CanonicalWorkflowPath(path) || seen[path] {
				return ErrBlocked
			}
			seen[path] = true
		}
		if err := add(item.Ref, item.ArtifactRef); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) graphValidateFinalContext(ctx context.Context, request GraphContextPrepareRequest, packet domain.GraphContextPacket) error {
	if request.FinalVerificationRef == nil {
		if len(request.Supplemental) != 0 {
			return ErrBlocked
		}
		return nil
	}
	body, err := m.graphIndexLoad(ctx, *request.FinalVerificationRef, "graph_final_verification")
	if err != nil {
		return err
	}
	var binding domain.GraphFinalVerificationBinding
	if json.Unmarshal(body, &binding, json.RejectUnknownMembers(true)) != nil {
		return evidence.ErrCorrupt
	}
	seal, err := domain.GraphFinalVerificationDigest(binding)
	input := request.InputBinding
	if err != nil || seal != binding.BindingSHA256 || binding.GraphID != input.GraphID || binding.GraphVersion != input.GraphVersion || binding.FinalNodeID != input.ReceivingNodeID || binding.FinalTaskID != input.ReceivingTaskID || binding.FinalTaskVersion != input.ReceivingTaskVersion || binding.FinalInputID != input.ID || binding.RootBinding.BasisSHA256 != input.RootBasisSHA256 || !reflect.DeepEqual(binding.ContributorVector, input.ContributorVector) || packet.InputID != input.ID {
		return evidence.ErrCorrupt
	}
	found := false
	for _, source := range request.Supplemental {
		if source.Ref == "final-verification" && source.ArtifactRef == *request.FinalVerificationRef && len(source.Paths) == 0 {
			found = true
		}
	}
	if !found {
		return ErrBlocked
	}
	return nil
}

func (m *Manager) graphCopyContextEntry(ctx context.Context, prepared *GraphPreparedContext, effects *graphContextEffects, access domain.GraphProvidedAccess, ordinal int, cache map[string]GraphContextEntry) (GraphContextEntry, error) {
	entry := GraphContextEntry{Ref: access.Ref, ArtifactRef: access.ArtifactRef, Scope: access.Scope, Paths: append([]string{}, access.Paths...), Mode: "file"}
	if previous, found := cache[entry.ArtifactRef.ArtifactID]; found {
		entry.Path, entry.Identity = previous.Path, previous.Identity
		return entry, nil
	}
	name := fmt.Sprintf("%06d.txt", ordinal)
	entry.Path = filepath.Join(prepared.Directory, name)
	effects.Entries = append(effects.Entries, graphContextEffect{Ref: entry.Ref, ArtifactRef: entry.ArtifactRef, Name: name, Mode: "file", State: "incomplete"})
	file, err := prepared.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return entry, err
	}
	if err = localstate.ProtectPrivateFile(file, false); err != nil {
		file.Close()
		return entry, err
	}
	entry.Identity, err = graphContextFileIdentity(file)
	if err != nil {
		file.Close()
		return entry, err
	}
	meta, copyErr := m.artifacts.CopyTo(ctx, entry.ArtifactRef.ArtifactID, file, MaxFileBytes)
	syncErr, closeErr := file.Sync(), file.Close()
	if syncErr != nil || closeErr != nil {
		prepared.State, prepared.EvidenceStatus = "uncertain", "uncertain"
	}
	if copyErr != nil || syncErr != nil || closeErr != nil || graphIndexRef(meta) != entry.ArtifactRef {
		return entry, errors.Join(evidence.ErrCorrupt, copyErr, syncErr, closeErr)
	}
	effects.Entries[len(effects.Entries)-1].State = "copied"
	cache[entry.ArtifactRef.ArtifactID] = entry
	return entry, nil
}

func graphFinalDescriptorBytes(prepared GraphPreparedContext) ([]byte, error) {
	if prepared.FinalVerificationRef == nil {
		return nil, ErrBlocked
	}
	value := GraphFinalContextDescriptor{Contract: "006-final-context-v1", ReceivingAttemptID: prepared.ReceivingAttemptID, InputID: prepared.Packet.InputID, InputSHA256: prepared.Packet.InputSHA256, PacketSHA256: prepared.Packet.PacketSHA256, FinalVerificationRef: *prepared.FinalVerificationRef, Access: []GraphContextFileAccess{}}
	for _, item := range prepared.descriptorEntries {
		value.Access = append(value.Access, GraphContextFileAccess{Ref: item.Ref, ArtifactRef: item.ArtifactRef, Scope: item.Scope, Paths: append([]string{}, item.Paths...), Path: item.Path})
	}
	return json.Marshal(value)
}

func (m *Manager) graphWriteFinalDescriptor(ctx context.Context, prepared *GraphPreparedContext, effects *graphContextEffects, sourceBytes int64) error {
	body, err := graphFinalDescriptorBytes(*prepared)
	if err != nil {
		return err
	}
	if int64(len(body)) > MaxFileBytes || sourceBytes+int64(len(body)) > MaxTotalBytes {
		return domain.Fail("context_capacity", "The complete context descriptor exceeds its local transport bounds.")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	name := "final-context.json"
	effects.DescriptorName, effects.DescriptorState = name, "incomplete"
	prepared.DescriptorPath = filepath.Join(prepared.Directory, name)
	file, err := prepared.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err = localstate.ProtectPrivateFile(file, false); err != nil {
		file.Close()
		return err
	}
	prepared.DescriptorIdentity, err = graphContextFileIdentity(file)
	if err != nil {
		file.Close()
		return err
	}
	_, writeErr := file.Write(body)
	syncErr, closeErr := file.Sync(), file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		prepared.State, prepared.EvidenceStatus = "uncertain", "uncertain"
		return errors.Join(writeErr, syncErr, closeErr)
	}
	prepared.DescriptorSHA256 = hashBytes(body)
	effects.DescriptorSHA256, effects.DescriptorState = prepared.DescriptorSHA256, "copied"
	return nil
}

func graphVerifyPrivateContextFile(ctx context.Context, root *os.Root, path, identity, sha, byteLength string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := root.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return evidence.ErrCorrupt
	}
	file, err := graphContextOpenRead(root, path)
	if err != nil {
		return err
	}
	privacyErr := localstate.VerifyPrivateFile(file, false)
	id, idErr := graphContextFileIdentity(file)
	before, statErr := file.Stat()
	if privacyErr != nil || idErr != nil || statErr != nil || id != identity {
		file.Close()
		return evidence.ErrCorrupt
	}
	h := sha256.New()
	count, readErr := io.Copy(h, io.LimitReader(file, MaxFileBytes+1))
	after, afterErr := file.Stat()
	privateAfter, closeErr := localstate.VerifyPrivateFile(file, false), file.Close()
	if readErr != nil || afterErr != nil || privateAfter != nil || closeErr != nil || count > MaxFileBytes || count != before.Size() || count != after.Size() || !before.ModTime().Equal(after.ModTime()) || hex.EncodeToString(h.Sum(nil)) != sha || strconv.FormatInt(count, 10) != byteLength {
		return evidence.ErrCorrupt
	}
	return nil
}

func (m *Manager) graphVerifyFinalContext(ctx context.Context, prepared GraphPreparedContext, root *os.Root) ([]domain.GraphArtifactRef, error) {
	if prepared.FinalVerificationRef == nil {
		if len(prepared.Supplemental) != 0 || prepared.DescriptorPath != "" || len(prepared.descriptorEntries) != 0 {
			return nil, ErrBlocked
		}
		return nil, nil
	}
	if err := graphContextReferenceBudget(prepared.Packet, prepared.supplementalPlan, prepared.otherReferenceCount); err != nil {
		return nil, err
	}
	if len(prepared.Supplemental) != len(prepared.supplementalPlan) || len(prepared.descriptorEntries) != len(prepared.Entries)+len(prepared.Supplemental) || prepared.DescriptorPath != filepath.Join(prepared.Directory, "final-context.json") || prepared.DescriptorIdentity == "" || !domain.ValidSHA256(prepared.DescriptorSHA256) {
		return nil, ErrBlocked
	}
	refs := []domain.GraphArtifactRef{*prepared.FinalVerificationRef}
	for i, item := range prepared.descriptorEntries {
		var want GraphContextSupplement
		if i < len(prepared.Entries) {
			ordinary := prepared.Entries[i]
			want = GraphContextSupplement{Ref: ordinary.Ref, ArtifactRef: ordinary.ArtifactRef, Scope: ordinary.Scope, Paths: ordinary.Paths}
		} else {
			want = prepared.supplementalPlan[i-len(prepared.Entries)]
		}
		if item.Ref != want.Ref || item.ArtifactRef != want.ArtifactRef || item.Scope != want.Scope || !graphContextPathsEqual(item.Paths, want.Paths) || item.Mode != "file" || item.Text != nil || filepath.Dir(item.Path) != prepared.Directory || item.Identity == "" {
			return nil, evidence.ErrCorrupt
		}
		if i >= len(prepared.Entries) && !reflect.DeepEqual(item, prepared.Supplemental[i-len(prepared.Entries)]) {
			return nil, evidence.ErrCorrupt
		}
		name := filepath.Base(item.Path)
		if len(name) != 10 || name[6:] != ".txt" {
			return nil, ErrBlocked
		}
		for _, ch := range name[:6] {
			if ch < '0' || ch > '9' {
				return nil, ErrBlocked
			}
		}
		if err := m.graphVerifyRef(ctx, item.ArtifactRef); err != nil {
			return nil, err
		}
		if err := graphVerifyPrivateContextFile(ctx, root, name, item.Identity, item.ArtifactRef.SHA256, item.ArtifactRef.ByteLength); err != nil {
			return nil, err
		}
		refs = append(refs, item.ArtifactRef)
	}
	if err := m.graphVerifyRef(ctx, *prepared.FinalVerificationRef); err != nil {
		return nil, err
	}
	body, err := graphFinalDescriptorBytes(prepared)
	if err != nil || hashBytes(body) != prepared.DescriptorSHA256 {
		return nil, evidence.ErrCorrupt
	}
	if err := graphVerifyPrivateContextFile(ctx, root, "final-context.json", prepared.DescriptorIdentity, prepared.DescriptorSHA256, strconv.Itoa(len(body))); err != nil {
		return nil, err
	}
	return refs, nil
}
