package workspace

import (
	json "encoding/json/v2"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// All fixtures in this file are PURE typed tuples, not filesystem/acceptance
// qualification. Actual closure, registration and materialization have separate tests.
func composeID(n int) string { return fmt.Sprintf("%032x", n) }
func composeRef(owner, label string, body []byte) domain.GraphArtifactRef {
	return domain.GraphArtifactRef{ArtifactID: hashBytes([]byte(owner + label))[:32], ProducerAttemptID: owner, SHA256: hashBytes(body), ByteLength: strconv.Itoa(len(body))}
}
func composeJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func composeManifest(owner string, working, staging map[string]string) (Manifest, GraphIndexBlobManifest) {
	m := Manifest{Entries: []Entry{}, Index: []IndexEntry{}, Exclusions: []Exclusion{}}
	blobs := GraphIndexBlobManifest{Contract: "006-index-blobs-v1", Entries: []GraphIndexBlob{}}
	for path, body := range working {
		ref := composeRef(owner, "working:"+path+body, []byte(body))
		m.Entries = append(m.Entries, Entry{Path: path, Type: "file", Mode: 0644, Size: int64(len(body)), SHA256: ref.SHA256, ArtifactID: ref.ArtifactID})
	}
	seen := map[string]bool{}
	for path, body := range staging {
		object := graphBlobObjectID([]byte(body))
		m.Index = append(m.Index, IndexEntry{Path: path, Mode: "100644", Object: object})
		if !seen[object] {
			ref := composeRef(owner, "staged:"+body, []byte(body))
			blobs.Entries = append(blobs.Entries, GraphIndexBlob{ObjectID: object, ContentSHA256: ref.SHA256, ByteLength: ref.ByteLength, ArtifactRef: ref})
			seen[object] = true
		}
	}
	sort.Slice(m.Entries, func(i, j int) bool { return m.Entries[i].Path < m.Entries[j].Path })
	sort.Slice(m.Index, func(i, j int) bool { return m.Index[i].Path < m.Index[j].Path })
	sort.Slice(blobs.Entries, func(i, j int) bool { return blobs.Entries[i].ObjectID < blobs.Entries[j].ObjectID })
	return m, blobs
}

func composeRoot(t *testing.T) (GraphCompositionRoot, GraphInputProvenance) {
	t.Helper()
	owner := composeID(700)
	m, blobs := composeManifest(owner, map[string]string{"shared.txt": "base", "c.txt": "base-c"}, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
	binding := domain.GraphRootBinding{GraphID: composeID(701), RootTaskID: composeID(702), OperationAttemptID: owner, SourceSnapshotID: composeID(703), ProjectID: composeID(704), ProjectIdentity: "pure-project", RepositoryIdentity: "pure-repository", SelectedRelativePath: ".", BaseCommit: strings.Repeat("1", 40), Manifest: composeRef(owner, "root-manifest", composeJSON(t, m)), Index: composeRef(owner, "synthetic-root-index-not-physical", []byte("typed synthetic index reference")), StagedBlobs: composeRef(owner, "root-blobs", composeJSON(t, blobs))}
	var err error
	binding.BasisSHA256, err = domain.GraphRootDigest(binding)
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := graphInitialProvenance(binding.BasisSHA256, m, blobs)
	if err != nil {
		t.Fatal(err)
	}
	return GraphCompositionRoot{Binding: binding, Manifest: m, StagedBlobs: blobs}, provenance
}

func composeNode(t *testing.T, root GraphCompositionRoot, n int, input Manifest, provenance GraphInputProvenance, upstream []VerifiedContribution, output Manifest, blobs GraphIndexBlobManifest) VerifiedContribution {
	t.Helper()
	node, task, producer := composeID(n), composeID(100+n), composeID(200+n)
	owner := root.Binding.OperationAttemptID
	vector := []domain.GraphContributorBinding{}
	direct := []domain.AcceptedContribution{}
	for _, u := range upstream {
		direct = append(direct, u.Binding)
		vector = append(vector, composeNodeBinding(u.Binding))
	}
	in := domain.GraphInputBinding{ID: composeID(1000 + n), GraphID: root.Binding.GraphID, GraphVersion: "1", RootBasisSHA256: root.Binding.BasisSHA256, OperationAttemptID: owner, ReceivingNodeID: node, ReceivingTaskID: task, ReceivingTaskVersion: "1", Contributions: direct, ContributorVector: vector,
		Manifest: composeRef(owner, "input-manifest-"+node, composeJSON(t, input)), Index: composeRef(owner, "synthetic-input-index-"+node, []byte("typed synthetic index reference")), StagedBlobs: root.Binding.StagedBlobs, Provenance: composeRef(owner, "input-provenance-"+node, composeJSON(t, provenance)), ContextPacket: composeRef(owner, "unqualified-unit-context-"+node, []byte("not physical context proof"))}
	var err error
	in.InputSHA256, err = domain.GraphInputDigest(in)
	if err != nil {
		t.Fatal(err)
	}
	manifestRef := composeRef(producer, "output-manifest", composeJSON(t, output))
	indexRef := composeRef(producer, "synthetic-output-index", []byte("typed synthetic index reference"))
	candidate := domain.CandidateBinding{RootSourceSnapshotID: composeID(300 + n), CandidateID: composeID(400 + n), SourceSnapshotID: composeID(500 + n), ManifestArtifactID: manifestRef.ArtifactID, ManifestSHA256: manifestRef.SHA256, IndexArtifactID: indexRef.ArtifactID, IndexSHA256: indexRef.SHA256, ConditionsSHA256: hashBytes([]byte("pure conditions"))}
	candidateID := candidate.CandidateID
	closure := domain.GraphOutputClosure{GraphID: root.Binding.GraphID, NodeID: node, ReceivedInputID: in.ID, Kind: "candidate", ProducerAttemptID: producer, CandidateID: &candidateID, Manifest: manifestRef, Index: indexRef, StagedBlobs: composeRef(producer, "output-blobs", composeJSON(t, blobs))}
	closure.ClosureSHA256, err = domain.GraphOutputDigest(closure)
	if err != nil {
		t.Fatal(err)
	}
	binding := domain.AcceptedContribution{EdgeID: composeID(600 + n), EdgeKind: "files", FromNodeID: node, TaskID: task, TaskVersion: "1", AcceptanceKind: "workflow", AcceptanceID: composeID(800 + n), ProducerAttemptID: producer, InputID: in.ID, CandidateBinding: &candidate, OutputClosureRef: composeRef(producer, "output-closure", composeJSON(t, closure)), ConstraintRefs: []domain.GraphScopedReference{}, EvidenceRefs: []domain.GraphContributedEvidence{}}
	return VerifiedContribution{Binding: binding, InputBinding: in, InputManifest: input, InputProvenance: provenance, OutputClosure: closure, OutputManifest: output, OutputStagedBlobs: blobs}
}

func composeNodeBinding(c domain.AcceptedContribution) domain.GraphContributorBinding {
	return domain.GraphContributorBinding{NodeID: c.FromNodeID, TaskID: c.TaskID, TaskVersion: c.TaskVersion, AcceptanceKind: c.AcceptanceKind, AcceptanceID: c.AcceptanceID, ProducerAttemptID: c.ProducerAttemptID, InputID: c.InputID, CandidateBinding: c.CandidateBinding, ResponseBinding: c.ResponseBinding, OutputClosureRef: c.OutputClosureRef}
}

// Independently seeded earlier A result: this is the verified-input fact for B/C,
// not a call to the composition reducer under test or a fabricated FS proof.
func composeAProvenance(t *testing.T, base GraphInputProvenance, a VerifiedContribution) GraphInputProvenance {
	t.Helper()
	var result GraphInputProvenance
	if err := json.Unmarshal(composeJSON(t, base), &result); err != nil {
		t.Fatal(err)
	}
	for i, slot := range result.Slots {
		if slot.Dimension == "working" && slot.Path == "shared.txt" {
			var selected Entry
			for _, e := range a.OutputManifest.Entries {
				if e.Path == slot.Path {
					selected = e
				}
			}
			state := selected
			state.ArtifactID = ""
			digest := hashBytes(composeJSON(t, state))
			parents := []string{slot.Origins[0].OriginSHA256}
			node := composeNodeBinding(a.Binding)
			body := composeJSON(t, struct {
				Kind      string                         `json:"kind"`
				Root      string                         `json:"rootBasisSha256"`
				Dimension string                         `json:"dimension"`
				Path      string                         `json:"path"`
				State     string                         `json:"stateSha256"`
				Node      domain.GraphContributorBinding `json:"node"`
				Parents   []string                       `json:"inputOriginIds"`
			}{"node", base.RootBasisSHA256, slot.Dimension, slot.Path, digest, node, parents})
			result.Slots[i] = GraphProvenanceSlot{Dimension: slot.Dimension, Path: slot.Path, StateSHA256: digest, Working: &selected, Origins: []GraphProvenanceOrigin{{Kind: "node", RootBasisSHA256: base.RootBasisSHA256, Node: &node, InputOriginIDs: parents, OriginSHA256: hashBytes(body)}}}
		}
	}
	return result
}

func composeRequest(root GraphCompositionRoot, inputs ...VerifiedContribution) GraphCompositionRequest {
	return GraphCompositionRequest{InputID: composeID(9999), Root: root, Contributions: inputs, CaseSensitive: true}
}
func composeWorking(plan CompositionPlan, path string) Entry {
	for _, entry := range plan.Manifest.Entries {
		if entry.Path == path {
			return entry
		}
	}
	return Entry{}
}

func TestGraphComposeUsesOriginalInputToFullSelectedOutput(t *testing.T) {
	root, prov := composeRoot(t)
	out, blobs := composeManifest(composeID(201), map[string]string{"shared.txt": "base", "c.txt": "base-c", "first-author.txt": "first", "correction.txt": "second"}, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
	node := composeNode(t, root, 1, root.Manifest, prov, nil, out, blobs)
	plan, conflicts, err := ComposeGraphInput(composeRequest(root, node))
	if err != nil || len(conflicts) != 0 || composeWorking(plan, "first-author.txt").SHA256 != hashBytes([]byte("first")) || composeWorking(plan, "correction.txt").SHA256 != hashBytes([]byte("second")) {
		t.Fatal("full selected lineage was reduced to an immediate correction delta", err, conflicts)
	}
}

func TestGraphComposeDiamondPreservesDescendantRefinementAndIndependentSibling(t *testing.T) {
	root, prov := composeRoot(t)
	aOut, aBlobs := composeManifest(composeID(201), map[string]string{"shared.txt": "A", "c.txt": "base-c"}, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
	a := composeNode(t, root, 1, root.Manifest, prov, nil, aOut, aBlobs)
	aProv := composeAProvenance(t, prov, a)
	bOut, bBlobs := composeManifest(composeID(202), map[string]string{"shared.txt": "B refinement", "c.txt": "base-c"}, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
	cOut, cBlobs := composeManifest(composeID(203), map[string]string{"shared.txt": "A", "c.txt": "C independent"}, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
	b := composeNode(t, root, 2, aOut, aProv, []VerifiedContribution{a}, bOut, bBlobs)
	c := composeNode(t, root, 3, aOut, aProv, []VerifiedContribution{a}, cOut, cBlobs)
	plan, conflicts, err := ComposeGraphInput(composeRequest(root, c, b))
	if err != nil || len(conflicts) != 0 || composeWorking(plan, "shared.txt").SHA256 != hashBytes([]byte("B refinement")) || composeWorking(plan, "c.txt").SHA256 != hashBytes([]byte("C independent")) {
		t.Fatal("unchanged sibling undid causal descendant or lost independent state", err, conflicts)
	}
	seen := 0
	for _, v := range plan.ContributorVector {
		if v.NodeID == a.Binding.FromNodeID {
			seen++
		}
	}
	if seen != 1 || len(plan.ContributorVector) != 3 {
		t.Fatal("diamond ancestor duplicated/lost", plan.ContributorVector)
	}
}

func TestGraphComposeIdenticalIndependentHeadsCoalesceWithoutDroppingOrigins(t *testing.T) {
	root, prov := composeRoot(t)
	aOut, aBlobs := composeManifest(composeID(201), map[string]string{"shared.txt": "same", "c.txt": "base-c"}, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
	bOut, bBlobs := composeManifest(composeID(202), map[string]string{"shared.txt": "same", "c.txt": "base-c"}, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
	a := composeNode(t, root, 1, root.Manifest, prov, nil, aOut, aBlobs)
	b := composeNode(t, root, 2, root.Manifest, prov, nil, bOut, bBlobs)
	plan, conflicts, err := ComposeGraphInput(composeRequest(root, b, a))
	if err != nil || len(conflicts) != 0 || composeWorking(plan, "shared.txt").SHA256 != hashBytes([]byte("same")) {
		t.Fatal("equal complete states incorrectly conflicted", err, conflicts)
	}
	heads := 0
	for _, slot := range plan.Provenance.Slots {
		if slot.Dimension == "working" && slot.Path == "shared.txt" {
			heads = len(slot.Origins)
		}
	}
	if heads != 2 {
		t.Fatal("identical-state provenance was silently collapsed", heads)
	}
	again, other, err := ComposeGraphInput(composeRequest(root, a, b))
	if err != nil || len(other) != 0 || string(composeJSON(t, plan)) != string(composeJSON(t, again)) {
		t.Fatal("composition depended on input arrival order", err)
	}
}

func TestGraphComposeDivergentWorkingAndStagedHeadsBlock(t *testing.T) {
	for _, dimension := range []string{"working", "index", "modify_delete"} {
		t.Run(dimension, func(t *testing.T) {
			root, prov := composeRoot(t)
			wa := map[string]string{"shared.txt": "base", "c.txt": "base-c"}
			wb := map[string]string{"shared.txt": "base", "c.txt": "base-c"}
			sa := map[string]string{"shared.txt": "base", "c.txt": "base-c"}
			sb := map[string]string{"shared.txt": "base", "c.txt": "base-c"}
			if dimension == "index" {
				sa["shared.txt"] = "staged-A"
				sb["shared.txt"] = "staged-B"
			} else {
				wa["shared.txt"] = "working-A"
				wb["shared.txt"] = "working-B"
				if dimension == "modify_delete" {
					delete(wb, "shared.txt")
				}
			}
			aOut, aBlobs := composeManifest(composeID(201), wa, sa)
			bOut, bBlobs := composeManifest(composeID(202), wb, sb)
			a := composeNode(t, root, 1, root.Manifest, prov, nil, aOut, aBlobs)
			b := composeNode(t, root, 2, root.Manifest, prov, nil, bOut, bBlobs)
			plan, conflicts, err := ComposeGraphInput(composeRequest(root, a, b))
			want := "working"
			if dimension == "index" {
				want = "index"
			}
			if err != nil || len(conflicts) != 1 || conflicts[0].Code != "input_conflict" || conflicts[0].Dimension != want || conflicts[0].Path != "shared.txt" || len(conflicts[0].ContributionRefs) != 2 || len(plan.Manifest.Entries) != 0 {
				t.Fatal("divergent heads selected an implicit winner or lost conflict evidence", err, conflicts)
			}
		})
	}
}

func TestGraphComposeContextReachabilityCannotSupersedeFileOrigin(t *testing.T) {
	root, prov := composeRoot(t)
	aOut, aBlobs := composeManifest(composeID(201), map[string]string{"shared.txt": "A", "c.txt": "base-c"}, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
	a := composeNode(t, root, 1, root.Manifest, prov, nil, aOut, aBlobs)
	contextA := a
	contextA.Binding.EdgeKind = "context"
	bOut, bBlobs := composeManifest(composeID(202), map[string]string{"shared.txt": "B without file ancestry", "c.txt": "base-c"}, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
	b := composeNode(t, root, 2, root.Manifest, prov, []VerifiedContribution{contextA}, bOut, bBlobs)
	_, conflicts, err := ComposeGraphInput(composeRequest(root, a, b))
	if err != nil || len(conflicts) == 0 {
		t.Fatal("context-only graph reachability became file ancestry", err, conflicts)
	}
}

func TestGraphComposeRejectsWholeTreePathAndNativeNameCollisions(t *testing.T) {
	for _, kind := range []string{"ancestor_file", "case_collision"} {
		t.Run(kind, func(t *testing.T) {
			root, prov := composeRoot(t)
			wa := map[string]string{"shared.txt": "base", "c.txt": "base-c"}
			wb := map[string]string{"shared.txt": "base", "c.txt": "base-c"}
			if kind == "ancestor_file" {
				wa["dir"] = "file"
				wb["dir/child.txt"] = "child"
			} else {
				wa["Name.txt"] = "one"
				wb["name.txt"] = "two"
			}
			aOut, aBlobs := composeManifest(composeID(201), wa, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
			bOut, bBlobs := composeManifest(composeID(202), wb, map[string]string{"shared.txt": "base", "c.txt": "base-c"})
			request := composeRequest(root, composeNode(t, root, 1, root.Manifest, prov, nil, aOut, aBlobs), composeNode(t, root, 2, root.Manifest, prov, nil, bOut, bBlobs))
			request.CaseSensitive = kind != "case_collision"
			plan, conflicts, err := ComposeGraphInput(request)
			if err != nil || len(conflicts) == 0 || len(plan.Manifest.Entries) != 0 {
				t.Fatal("invalid composed namespace escaped before native materialization", err, conflicts)
			}
		})
	}
}
