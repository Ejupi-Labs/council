package engine

import (
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"reflect"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type graphMemberCursor struct {
	Version string `json:"version"`
	GraphID string `json:"graphId"`
	InputID string `json:"inputId"`
	Seal    string `json:"seal"`
	Offset  string `json:"offset"`
}

func graphMemberOffset(raw, graphID, inputID, seal string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	invalid := domain.Fail("invalid_cursor", "Use the complete cursor for this exact retained graph input, or start its member list again.")
	if len(raw) > 2048 {
		return 0, invalid
	}
	body, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(body) != raw {
		return 0, invalid
	}
	var cursor graphMemberCursor
	if json.Unmarshal(body, &cursor, json.RejectUnknownMembers(true)) != nil || cursor.Version != "006-members-v1" || cursor.GraphID != graphID || cursor.InputID != inputID || cursor.Seal != seal {
		return 0, invalid
	}
	canonical, err := json.Marshal(cursor)
	if err != nil || string(canonical) != string(body) {
		return 0, invalid
	}
	offset, failure := domain.ParseCount(cursor.Offset)
	if failure != nil || offset == 0 {
		return 0, invalid
	}
	return offset, nil
}

func graphNextMemberCursor(graphID, inputID, seal string, offset int) (string, error) {
	body, err := json.Marshal(graphMemberCursor{"006-members-v1", graphID, inputID, seal, strconv.Itoa(offset)})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

// This read verifies the retained physical closure before projecting public
// rows. It neither creates artifacts nor changes registration, claims or budget.
func (e *Engine) graphInputMembers(ctx context.Context, graphID, inputID string, limit int, cursor string) (domain.Page[domain.GraphMemberView], error) {
	page := domain.Page[domain.GraphMemberView]{Items: []domain.GraphMemberView{}}
	if !domain.ValidID(graphID) || !domain.ValidID(inputID) || limit < 1 || limit > 100 {
		return page, domain.Invalid("bounded exact graph input member selector")
	}
	record, err := e.db.GraphInputRecord(ctx, graphID, inputID)
	if err != nil {
		return page, err
	}
	gap := domain.Fail("evidence_unavailable", "Complete retained input members could not be verified. Existing input, conflict and effect records remain available for inspection.")
	if record.Public.Summary.ID != inputID || record.Public.Summary.GraphID != graphID || record.Public.Summary.State != "ready" || record.PrivateRoot == nil {
		return page, gap
	}
	root := *record.PrivateRoot
	seal := root.BasisSHA256
	binding := record.Public.InputBinding
	if record.Public.Summary.Kind == "node_input" {
		if binding == nil || record.Public.RootBinding != nil || binding.ID != inputID || binding.GraphID != graphID || binding.GraphVersion != record.Public.Summary.GraphVersion ||
			binding.OperationAttemptID != record.Public.Summary.OperationAttemptID || binding.RootBasisSHA256 != root.BasisSHA256 ||
			record.Public.Summary.ReceivingNodeID == nil || binding.ReceivingNodeID != *record.Public.Summary.ReceivingNodeID ||
			record.Public.Summary.ReceivingTaskID == nil || binding.ReceivingTaskID != *record.Public.Summary.ReceivingTaskID ||
			record.Public.Summary.ReceivingTaskVersion == nil || binding.ReceivingTaskVersion != *record.Public.Summary.ReceivingTaskVersion {
			return page, gap
		}
		seal = binding.InputSHA256
	} else if record.Public.Summary.Kind != "root_basis" || binding != nil || record.Public.RootBinding == nil || !reflect.DeepEqual(*record.Public.RootBinding, domain.GraphRootPublic(root)) {
		return page, gap
	}
	if !domain.ValidSHA256(seal) {
		return page, gap
	}
	after, err := graphMemberOffset(cursor, graphID, inputID, seal)
	if err != nil {
		return page, err
	}
	view, err := e.db.Graph(ctx, graphID)
	if err != nil {
		return page, err
	}
	if view.Graph.RootInputID == nil || root.GraphID != graphID || root.RootTaskID != view.Graph.RootTaskID {
		return page, gap
	}
	rootRecord, err := e.db.GraphInputRecord(ctx, graphID, *view.Graph.RootInputID)
	if err != nil || rootRecord.PrivateRoot == nil || !reflect.DeepEqual(*rootRecord.PrivateRoot, root) {
		return page, gap
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, graphID)
	if err != nil {
		return page, err
	}
	rootSource, err := e.graphRootSourceSnapshot(ctx, root, definition.Delivery)
	if err != nil {
		return page, gap
	}
	rootProof, err := e.workspace.VerifyGraphRoot(ctx, root, rootSource)
	if err != nil {
		return page, gap
	}
	manifest, staged := rootProof.Manifest, rootProof.StagedBlobs
	var input *workspace.VerifiedGraphInput
	if binding != nil {
		verified, err := e.workspace.VerifyGraphInput(ctx, *binding)
		if err != nil {
			return page, gap
		}
		input = &verified
		manifest, staged = verified.Manifest, verified.StagedBlobs
	}
	slots, err := workspace.GraphMemberSlots(root, rootProof, input)
	if err != nil {
		return page, gap
	}
	allowed := map[string]bool{}
	if binding != nil {
		allowed, err = graphMemberFileAncestors(definition, view.Nodes, binding.GraphVersion, binding.ReceivingNodeID)
		if err != nil {
			return page, gap
		}
	}
	rows, err := e.graphMemberRows(ctx, root, *view.Graph.RootInputID, manifest, staged, slots, binding, allowed)
	if err != nil {
		return page, gap
	}
	if cursor != "" && after >= int64(len(rows)) {
		return page, domain.Fail("invalid_cursor", "This member cursor is outside its complete retained input. Start that exact list again.")
	}
	// Reserve enough space for the envelope and canonical next cursor; whole
	// rows are never clipped or silently skipped to make a page fit.
	bytes := 2048
	for i := int(after); i < len(rows); i++ {
		body, err := json.Marshal(rows[i])
		if err != nil || len(body)+2048 > 4<<20 {
			return domain.Page[domain.GraphMemberView]{Items: []domain.GraphMemberView{}}, domain.Fail("evidence_unavailable", "A complete retained member exceeds the supported page bound. Inspect the exact retained input artifacts instead.")
		}
		if len(page.Items) == limit || bytes+len(body)+1 > 4<<20 {
			next, err := graphNextMemberCursor(graphID, inputID, seal, i)
			if err != nil {
				return domain.Page[domain.GraphMemberView]{Items: []domain.GraphMemberView{}}, gap
			}
			page.NextCursor = &next
			break
		}
		page.Items = append(page.Items, rows[i])
		bytes += len(body) + 1
	}
	return page, nil
}

func (e *Engine) graphMemberRows(ctx context.Context, root domain.GraphRootBinding, rootInputID string, manifest workspace.Manifest, staged workspace.GraphIndexBlobManifest, slots []workspace.GraphProvenanceSlot, input *domain.GraphInputBinding, fileAncestors map[string]bool) ([]domain.GraphMemberView, error) {
	working := map[string]workspace.Entry{}
	index := map[string]workspace.IndexEntry{}
	blobs := map[string]workspace.GraphIndexBlob{}
	vector := map[string]domain.GraphContributorBinding{}
	headSeals := map[string]string{}
	for _, entry := range manifest.Entries {
		working[entry.Path] = entry
	}
	for _, entry := range manifest.Index {
		index[entry.Path] = entry
	}
	for _, blob := range staged.Entries {
		blobs[blob.ObjectID] = blob
	}
	if input != nil {
		for _, item := range input.ContributorVector {
			vector[item.NodeID] = item
		}
	}
	rows := make([]domain.GraphMemberView, 0, len(slots))
	for _, slot := range slots {
		row := domain.GraphMemberView{Dimension: slot.Dimension, Path: slot.Path, State: "absent", Origins: []domain.GraphMemberOrigin{}}
		if !slot.Absent {
			row.State = "present"
			if slot.Dimension == "working" {
				entry, exists := working[slot.Path]
				if !exists {
					return nil, evidence.ErrCorrupt
				}
				typeName, mode, length := entry.Type, domain.Decimal(int64(entry.Mode)), domain.Decimal(entry.Size)
				row.Type, row.Mode, row.ByteLength = &typeName, &mode, &length
				if entry.SHA256 != "" {
					sha := entry.SHA256
					row.SHA256 = &sha
				}
				if entry.Type == "symlink" {
					target := entry.Target
					row.Target = &target
				} else {
					// Resolve only the exact verified manifest member. A provenance
					// field can never nominate another artifact with matching text.
					meta, err := e.evidence.VerifyClosed(ctx, entry.ArtifactID)
					if err != nil || meta.SHA256 == nil || meta.Kind != "source_file" || *meta.SHA256 != entry.SHA256 || meta.ByteLength != length {
						return nil, evidence.ErrCorrupt
					}
					ref := domain.GraphArtifactRef{ArtifactID: meta.ID, ProducerAttemptID: meta.AttemptID, SHA256: *meta.SHA256, ByteLength: meta.ByteLength}
					row.ArtifactRef = &ref
				}
			} else if slot.Dimension == "index" {
				entry, exists := index[slot.Path]
				blob, hasBlob := blobs[entry.Object]
				if !exists || !hasBlob {
					return nil, evidence.ErrCorrupt
				}
				mode, object, length, sha, ref := entry.Mode, entry.Object, blob.ByteLength, blob.ContentSHA256, blob.ArtifactRef
				typeName := "file"
				if mode == "120000" {
					typeName = "symlink"
					meta, err := e.db.Artifact(ctx, ref.ArtifactID)
					if err != nil {
						return nil, err
					}
					body, err := e.readGraphArtifact(ctx, ref, meta.Kind, workspace.MaxFileBytes)
					if err != nil || !utf8.Valid(body) {
						return nil, evidence.ErrCorrupt
					}
					target := string(body)
					row.Target = &target
				}
				row.Type, row.Mode, row.ObjectID, row.ByteLength, row.SHA256, row.ArtifactRef = &typeName, &mode, &object, &length, &sha, &ref
			} else {
				return nil, evidence.ErrCorrupt
			}
		}
		for _, origin := range slot.Origins {
			if origin.Kind == "root" {
				row.Origins = append(row.Origins, domain.GraphMemberOrigin{Kind: "root", TaskID: root.RootTaskID, ProducerAttemptID: root.OperationAttemptID, InputID: rootInputID, StateSHA256: root.BasisSHA256})
				continue
			}
			if origin.Kind != "node" || origin.Node == nil {
				return nil, evidence.ErrCorrupt
			}
			node, exists := vector[origin.Node.NodeID]
			if !exists || !fileAncestors[node.NodeID] || !reflect.DeepEqual(node, *origin.Node) {
				return nil, evidence.ErrCorrupt
			}
			headSeal, known := headSeals[node.NodeID]
			if !known {
				var err error
				headSeal, err = e.graphMemberSourceHeadSeal(ctx, root.GraphID, node)
				if err != nil {
					return nil, err
				}
				headSeals[node.NodeID] = headSeal
			}
			id, version, kind, acceptance := node.NodeID, node.TaskVersion, node.AcceptanceKind, node.AcceptanceID
			row.Origins = append(row.Origins, domain.GraphMemberOrigin{Kind: "node", NodeID: &id, TaskID: node.TaskID, TaskVersion: &version,
				AcceptanceKind: &kind, AcceptanceID: &acceptance, ProducerAttemptID: node.ProducerAttemptID, InputID: node.InputID, StateSHA256: headSeal})
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Dimension != rows[j].Dimension {
			return rows[i].Dimension < rows[j].Dimension
		}
		return rows[i].Path < rows[j].Path
	})
	return rows, nil
}

// Contributor vectors bind the exact historical output envelope. Its sealed
// tuple identifies a source head without substituting a latest acceptance.
func (e *Engine) graphMemberSourceHeadSeal(ctx context.Context, graphID string, node domain.GraphContributorBinding) (string, error) {
	if node.OutputClosureRef.ProducerAttemptID != node.ProducerAttemptID {
		return "", evidence.ErrCorrupt
	}
	body, err := e.readGraphArtifact(ctx, node.OutputClosureRef, "graph_output_closure", 64<<20)
	if err != nil {
		return "", err
	}
	var closure domain.GraphOutputClosure
	if json.Unmarshal(body, &closure, json.RejectUnknownMembers(true)) != nil || domain.ValidateGraphOutputClosure(closure) != nil ||
		closure.GraphID != graphID || closure.NodeID != node.NodeID || closure.ProducerAttemptID != node.ProducerAttemptID || closure.ReceivedInputID != node.InputID {
		return "", evidence.ErrCorrupt
	}
	digest, err := domain.GraphOutputDigest(closure)
	if err != nil || digest != closure.ClosureSHA256 {
		return "", evidence.ErrCorrupt
	}
	if node.CandidateBinding != nil {
		b := node.CandidateBinding
		if node.ResponseBinding != nil || closure.Kind != "candidate" || closure.CandidateID == nil || *closure.CandidateID != b.CandidateID ||
			closure.Manifest.ArtifactID != b.ManifestArtifactID || closure.Manifest.SHA256 != b.ManifestSHA256 || closure.Index.ArtifactID != b.IndexArtifactID || closure.Index.SHA256 != b.IndexSHA256 {
			return "", evidence.ErrCorrupt
		}
	} else {
		b := node.ResponseBinding
		if b == nil || closure.Kind != "response" || closure.ResponseArtifact == nil || b.ProducerAttemptID != node.ProducerAttemptID ||
			closure.ResponseArtifact.ArtifactID != b.ResponseArtifactID || closure.ResponseArtifact.SHA256 != b.ResponseSHA256 || closure.ResponseArtifact.ByteLength != b.ResponseByteLength || closure.Manifest.SHA256 != b.ManifestSHA256 ||
			closure.Index.ArtifactID != b.IndexArtifactID || closure.Index.SHA256 != b.IndexSHA256 {
			return "", evidence.ErrCorrupt
		}
	}
	return closure.ClosureSHA256, nil
}

// Topology is immutable. Only files-only reachability can attribute a retained
// file state; context edges remain evidence without granting file ancestry.
// Current versions/acceptances are deliberately irrelevant to historical reads.
func graphMemberFileAncestors(definition domain.GraphDefinition, nodes []domain.GraphNodeSummary, graphVersion, receivingNodeID string) (map[string]bool, error) {
	if definition.GraphVersion != graphVersion || !domain.ValidID(definition.GraphID) || !domain.ValidID(definition.RootTaskID) || !domain.ValidID(receivingNodeID) ||
		domain.ValidateGraphStartInput(definition.Original) != nil || len(nodes) != len(definition.Original.Nodes) {
		return nil, evidence.ErrCorrupt
	}
	declared := map[string]bool{}
	for _, node := range definition.Original.Nodes {
		declared[node.Key] = true
	}
	ids, keys, tasks := map[string]string{}, map[string]bool{}, map[string]bool{}
	receiverKey := ""
	for _, node := range nodes {
		if node.GraphID != definition.GraphID || !domain.ValidID(node.ID) || !domain.ValidID(node.TaskID) || node.TaskID == definition.RootTaskID ||
			!declared[node.Key] || keys[node.ID] || ids[node.Key] != "" || tasks[node.TaskID] {
			return nil, evidence.ErrCorrupt
		}
		keys[node.ID], tasks[node.TaskID], ids[node.Key] = true, true, node.ID
		if node.ID == receivingNodeID {
			receiverKey = node.Key
		}
	}
	if receiverKey == "" {
		return nil, evidence.ErrCorrupt
	}
	incoming := map[string][]string{}
	if definition.Original.Dependencies != nil {
		for _, edge := range *definition.Original.Dependencies {
			if edge.Kind == "" || edge.Kind == "files" {
				incoming[edge.To] = append(incoming[edge.To], edge.From)
			}
		}
	}
	seen := map[string]bool{receiverKey: true}
	pending := []string{receiverKey}
	result := map[string]bool{}
	for len(pending) != 0 {
		key := pending[0]
		pending = pending[1:]
		for _, source := range incoming[key] {
			if source == receiverKey || ids[source] == "" {
				return nil, evidence.ErrCorrupt
			}
			if !seen[source] {
				seen[source] = true
				pending = append(pending, source)
				result[ids[source]] = true
			}
		}
	}
	return result, nil
}
