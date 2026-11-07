package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

type GraphIndexBlob struct {
	ObjectID      string                  `json:"objectId"`
	ContentSHA256 string                  `json:"contentSha256"`
	ByteLength    string                  `json:"byteLength"`
	ArtifactRef   domain.GraphArtifactRef `json:"artifactRef"`
}

type GraphIndexBlobManifest struct {
	Contract string           `json:"contract"`
	Entries  []GraphIndexBlob `json:"entries"`
}

type GraphIndexCaptureRequest struct {
	ProducerAttemptID string
	Source            *Snapshot
	WorkingManifest   domain.GraphArtifactRef
	RawIndex          domain.GraphArtifactRef
}

type GraphIndexCapture struct {
	StagedBlobs        domain.GraphArtifactRef
	IndexEntriesSHA256 string
	CombinedBytes      string
	BlobCount          string
}

type GraphIndexRestoreRequest struct {
	OperationAttemptID string
	Target             *Snapshot
	WorkingManifest    domain.GraphArtifactRef
	RawIndex           domain.GraphArtifactRef
	StagedBlobs        domain.GraphArtifactRef
}

type GraphIndexRestore struct {
	EffectsArtifact *domain.GraphArtifactRef
	EffectCount     string
	EffectsKnown    bool
}

type GraphObjectEffect struct {
	Kind             string `json:"kind"`
	ObjectID         string `json:"objectId"`
	Action           string `json:"action"`
	SourceArtifactID string `json:"sourceArtifactId"`
	ByteLength       string `json:"byteLength"`
	ObservedAt       string `json:"observedAt"`
}

type GraphObjectEffects struct {
	Contract        string              `json:"contract"`
	Effects         []GraphObjectEffect `json:"effects"`
	EffectsKnown    bool                `json:"effectsKnown"`
	PendingObjectID *string             `json:"pendingObjectId"`
}

func (m *Manager) CaptureGraphIndex(ctx context.Context, request GraphIndexCaptureRequest) (GraphIndexCapture, error) {
	var result GraphIndexCapture
	if !validManagedID(request.ProducerAttemptID) || request.Source == nil || request.Source.AttemptID != request.ProducerAttemptID || request.WorkingManifest.ProducerAttemptID != request.ProducerAttemptID || request.RawIndex.ProducerAttemptID != request.ProducerAttemptID {
		return result, ErrBlocked
	}
	root, err := m.graphIndexTarget(request.Source)
	if err != nil {
		return result, err
	}
	defer root.Close()
	manifest, _, err := m.graphIndexBasis(ctx, request.WorkingManifest, request.RawIndex)
	if err != nil {
		return result, err
	}
	indexPath, err := m.value(ctx, request.Source.WorkspacePath, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return result, err
	}
	// Capture retained the user's exact raw index, while its managed worktree
	// rebuilt an equivalent index. Preserve both raw identities rather than
	// requiring stat-cache/extensions from those independent files to be equal.
	managedBefore, err := readIndex(indexPath)
	if err != nil {
		return result, err
	}
	managedEntries, err := handoffIndexEntries(managedBefore)
	if err != nil || !indexEqual(managedEntries, manifest.Index) {
		return result, fmt.Errorf("%w The managed semantic index differs from its retained graph basis.", ErrBlocked)
	}
	checkRaw := func() error {
		current, err := readIndex(indexPath)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, managedBefore) {
			return fmt.Errorf("%w The managed raw index changed during graph blob capture.", ErrBlocked)
		}
		return nil
	}
	if err = checkRaw(); err != nil {
		return result, err
	}
	working, total, err := m.graphWorkingPayloads(ctx, manifest)
	if err != nil {
		return result, err
	}
	staged := GraphIndexBlobManifest{Contract: "006-index-blobs-v1", Entries: []GraphIndexBlob{}}
	extra := map[string]bool{}
	objects := graphIndexObjects(manifest.Index)
	err = m.graphReadBlobs(ctx, request.Source.WorkspacePath, objects, func(object string, body []byte) error {
		digest := hashBytes(body)
		length := strconv.FormatInt(int64(len(body)), 10)
		key := digest + ":" + length
		ref, reused := working[key]
		if !reused && !extra[key] {
			total += int64(len(body))
			if total > MaxTotalBytes {
				return fmt.Errorf("%w Working and additional staged payloads exceed512MiB.", ErrBlocked)
			}
			extra[key] = true
		}
		if !reused {
			meta, err := m.artifacts.Put(ctx, request.ProducerAttemptID, "graph_index_blob", body)
			if err != nil {
				return err
			}
			ref = graphIndexRef(meta)
		}
		staged.Entries = append(staged.Entries, GraphIndexBlob{ObjectID: object, ContentSHA256: digest, ByteLength: length, ArtifactRef: ref})
		return nil
	})
	if err != nil {
		return result, err
	}
	if err = checkRaw(); err != nil {
		return result, err
	}
	if current, e := root.ReadFile(".git"); e != nil || hashBytes(current) != request.Source.GitControlSHA256 {
		return result, ErrBlocked
	}
	body, err := json.Marshal(staged)
	if err != nil || len(body) > 64<<20 {
		return result, ErrBlocked
	}
	meta, err := m.artifacts.Put(ctx, request.ProducerAttemptID, "graph_index_blobs", body)
	if err != nil {
		return result, err
	}
	semantic, err := graphIndexDigest(manifest.Index)
	if err != nil {
		return result, err
	}
	return GraphIndexCapture{StagedBlobs: graphIndexRef(meta), IndexEntriesSHA256: semantic, CombinedBytes: strconv.FormatInt(total, 10), BlobCount: strconv.Itoa(len(staged.Entries))}, nil
}

// RestoreGraphIndexObjects restores only verified Git blob objects. It does not
// write a working file, index or ref; the receiving materializer owns its index.
func (m *Manager) RestoreGraphIndexObjects(ctx context.Context, request GraphIndexRestoreRequest) (GraphIndexRestore, error) {
	result := GraphIndexRestore{EffectsKnown: true, EffectCount: "0"}
	if !validManagedID(request.OperationAttemptID) {
		return result, ErrBlocked
	}
	if err := m.lock.CheckHeld(); err != nil {
		return result, err
	}
	manifest, _, err := m.graphIndexBasis(ctx, request.WorkingManifest, request.RawIndex)
	if err != nil {
		return result, err
	}
	return m.graphRestoreIndexObjects(ctx, request, manifest)
}

// The composition operation has no raw index until Git constructs it. Its
// manifest and staged envelope have already been closed and fully verified;
// this shared object-only path retains the same payload and outcome proof.
func (m *Manager) graphRestoreIndexObjects(ctx context.Context, request GraphIndexRestoreRequest, manifest Manifest) (GraphIndexRestore, error) {
	result := GraphIndexRestore{EffectsKnown: true, EffectCount: "0"}
	if !validManagedID(request.OperationAttemptID) {
		return result, ErrBlocked
	}
	root, err := m.graphIndexTarget(request.Target)
	if err != nil {
		return result, err
	}
	defer root.Close()
	body, err := m.graphIndexLoad(ctx, request.StagedBlobs, "graph_index_blobs")
	if err != nil {
		return result, err
	}
	var staged GraphIndexBlobManifest
	if err = json.Unmarshal(body, &staged, json.RejectUnknownMembers(true)); err != nil || staged.Contract != "006-index-blobs-v1" || len(staged.Entries) > MaxEntries {
		return result, evidence.ErrCorrupt
	}
	objects := graphIndexObjects(manifest.Index)
	if len(objects) != len(staged.Entries) {
		return result, evidence.ErrCorrupt
	}
	working, total, err := m.graphWorkingPayloads(ctx, manifest)
	if err != nil {
		return result, err
	}
	byObject := map[string]GraphIndexBlob{}
	extra := map[string]bool{}
	// Verify EVERY retained blob even if Git already has its object. Present Git
	// bytes do not replace the graph's required closed artifact proof.
	for i, blob := range staged.Entries {
		if blob.ObjectID != objects[i] || !graphObjectID(blob.ObjectID) || !domain.ValidSHA256(blob.ContentSHA256) || blob.ContentSHA256 != blob.ArtifactRef.SHA256 || blob.ByteLength != blob.ArtifactRef.ByteLength {
			return result, evidence.ErrCorrupt
		}
		data, err := m.graphIndexBlobBytes(ctx, blob)
		if err != nil {
			return result, err
		}
		key := blob.ContentSHA256 + ":" + blob.ByteLength
		if _, reused := working[key]; !reused && !extra[key] {
			total += int64(len(data))
			if total > MaxTotalBytes {
				return result, ErrBlocked
			}
			extra[key] = true
		}
		byObject[blob.ObjectID] = blob
	}
	present, err := m.graphPresentObjects(ctx, request.Target.WorkspacePath, objects, byObject)
	if err != nil {
		return result, err
	}
	var existing []string
	for _, object := range objects {
		if present[object] {
			existing = append(existing, object)
		}
	}
	if err = m.graphReadBlobs(ctx, request.Target.WorkspacePath, existing, func(object string, body []byte) error {
		if hashBytes(body) != byObject[object].ContentSHA256 || strconv.Itoa(len(body)) != byObject[object].ByteLength {
			return evidence.ErrCorrupt
		}
		return nil
	}); err != nil {
		return result, err
	}
	stream, err := m.artifacts.Create(ctx, request.OperationAttemptID, "graph_operation_effects", 64<<20)
	if err != nil {
		return result, err
	}
	effects := GraphObjectEffects{Contract: "006-graph-object-effects-v1", Effects: []GraphObjectEffect{}, EffectsKnown: true}
	var operationErr error
	for _, object := range objects {
		if err = ctx.Err(); err != nil {
			operationErr = err
			break
		}
		blob := byObject[object]
		action := "already_present"
		if !present[object] {
			payload, readErr := m.graphIndexBlobBytes(ctx, blob)
			if readErr != nil {
				operationErr = readErr
				break
			}
			if current, e := root.ReadFile(".git"); e != nil || hashBytes(current) != request.Target.GitControlSHA256 {
				operationErr = ErrBlocked
				break
			}
			written, writeErr := m.git(ctx, request.Target.WorkspacePath, payload, "hash-object", "-t", "blob", "-w", "--stdin")
			if writeErr != nil || strings.TrimSpace(string(written)) != object {
				pending := object
				effects.PendingObjectID = &pending
				effects.EffectsKnown = false
				operationErr = errors.Join(fmt.Errorf("%w A Git object restoration outcome is uncertain.", ErrBlocked), writeErr)
				break
			}
			if readErr = m.graphReadBlobs(ctx, request.Target.WorkspacePath, []string{object}, func(_ string, actual []byte) error {
				if !bytes.Equal(actual, payload) {
					return evidence.ErrCorrupt
				}
				return nil
			}); readErr != nil {
				pending := object
				effects.PendingObjectID = &pending
				effects.EffectsKnown = false
				operationErr = readErr
				break
			}
			action = "restored"
		}
		effects.Effects = append(effects.Effects, GraphObjectEffect{Kind: "git_object", ObjectID: object, Action: action, SourceArtifactID: blob.ArtifactRef.ArtifactID, ByteLength: blob.ByteLength, ObservedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z")})
	}
	result.EffectsKnown = effects.EffectsKnown
	result.EffectCount = strconv.Itoa(len(effects.Effects))
	record, marshalErr := json.Marshal(effects)
	if marshalErr == nil {
		_, marshalErr = stream.Write(record)
	}
	settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	meta, closeErr := stream.Close(settlementCtx, marshalErr == nil)
	if closeErr == nil && marshalErr == nil {
		ref := graphIndexRef(meta)
		result.EffectsArtifact = &ref
	}
	return result, errors.Join(operationErr, marshalErr, closeErr)
}

func (m *Manager) graphWorkingPayloads(ctx context.Context, manifest Manifest) (map[string]domain.GraphArtifactRef, int64, error) {
	working := map[string]domain.GraphArtifactRef{}
	var total int64
	for _, entry := range manifest.Entries {
		if entry.Type != "file" {
			continue
		}
		meta, err := m.artifacts.VerifyClosed(ctx, entry.ArtifactID)
		if err != nil {
			return nil, total, err
		}
		length := strconv.FormatInt(entry.Size, 10)
		if meta.Kind != "source_file" || meta.SHA256 == nil || *meta.SHA256 != entry.SHA256 || meta.ByteLength != length {
			return nil, total, evidence.ErrCorrupt
		}
		total += entry.Size
		if total > MaxTotalBytes {
			return nil, total, ErrBlocked
		}
		working[entry.SHA256+":"+length] = graphIndexRef(meta)
	}
	return working, total, nil
}

func (m *Manager) graphIndexBlobBytes(ctx context.Context, blob GraphIndexBlob) ([]byte, error) {
	body, err := m.graphIndexLoad(ctx, blob.ArtifactRef, "graph_index_blob", "source_file")
	if err != nil {
		return nil, err
	}
	if graphBlobObjectID(body) != blob.ObjectID || hashBytes(body) != blob.ContentSHA256 || strconv.Itoa(len(body)) != blob.ByteLength {
		return nil, evidence.ErrCorrupt
	}
	return body, nil
}

func (m *Manager) graphPresentObjects(ctx context.Context, dir string, objects []string, blobs map[string]GraphIndexBlob) (map[string]bool, error) {
	present := map[string]bool{}
	if len(objects) == 0 {
		return present, nil
	}
	var input bytes.Buffer
	for _, object := range objects {
		input.WriteString(object + "\n")
	}
	body, err := m.git(ctx, dir, input.Bytes(), "cat-file", "--batch-check")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	if len(lines) != len(objects) {
		return nil, evidence.ErrCorrupt
	}
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == objects[i] && fields[1] == "missing" {
			continue
		}
		if len(fields) != 3 || fields[0] != objects[i] || fields[1] != "blob" || fields[2] != blobs[objects[i]].ByteLength {
			return nil, evidence.ErrCorrupt
		}
		present[objects[i]] = true
	}
	return present, nil
}
