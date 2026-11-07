package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"testing"
)

func graphSealValue[T any](t *testing.T, seal func(T) (string, error), value T) string {
	t.Helper()
	digest, err := seal(value)
	if err != nil || !ValidSHA256(digest) {
		t.Fatal("canonical seal unavailable", digest, err)
	}
	return digest
}

func graphSealBytes(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestGraphRootDigestUsesFixedOrderPrivateIdentitiesAndExcludesSelf(t *testing.T) {
	root := GraphRootBinding{GraphID: "g", RootTaskID: "r", OperationAttemptID: "o", SourceSnapshotID: "s", ProjectID: "p", ProjectIdentity: "project-1", RepositoryIdentity: "repo-1", SelectedRelativePath: "café/東京", BaseCommit: "base", BasisSHA256: "exclude-me"}
	got := graphSealValue(t, GraphRootDigest, root)
	// Literal protocol bytes distinguish declaration order from a sorted-key map,
	// include private identities and preserve Unicode without normalization.
	canonical := `{"graphId":"g","rootTaskId":"r","operationAttemptId":"o","sourceSnapshotId":"s","manifest":{"artifactId":"","producerAttemptId":"","sha256":"","byteLength":""},"index":{"artifactId":"","producerAttemptId":"","sha256":"","byteLength":""},"stagedBlobs":{"artifactId":"","producerAttemptId":"","sha256":"","byteLength":""},"projectId":"p","projectIdentity":"project-1","repositoryIdentity":"repo-1","selectedRelativePath":"café/東京","baseCommit":"base","branch":null}`
	expected := sha256.Sum256([]byte(canonical))
	if got != hex.EncodeToString(expected[:]) {
		t.Fatal("root seal did not use the exact fixed-field private binding")
	}
	root.BasisSHA256 = "a-different-self-seal"
	if other := graphSealValue(t, GraphRootDigest, root); other != got {
		t.Fatal("root seal hashes itself")
	}
	for _, field := range []string{"project", "repository"} {
		changed := root
		if field == "project" {
			changed.ProjectIdentity = "replaced-project"
		} else {
			changed.RepositoryIdentity = "replaced-repository"
		}
		if graphSealValue(t, GraphRootDigest, changed) == got {
			t.Fatal("private physical replacement did not invalidate root seal", field)
		}
	}
}

func TestGraphInputDigestHasOneWayContextDependency(t *testing.T) {
	input := GraphInputBinding{ID: "input", GraphID: "graph", ReceivingTaskID: "receiver", Contributions: nil, ContributorVector: nil}
	got := graphSealValue(t, GraphInputDigest, input)
	input.InputSHA256 = "self"
	input.ContextPacket = GraphArtifactRef{ArtifactID: "later-context", SHA256: "later-context-sha"}
	input.Contributions = []AcceptedContribution{}
	input.ContributorVector = []GraphContributorBinding{}
	if graphSealValue(t, GraphInputDigest, input) != got {
		t.Fatal("input seal includes its later context/self or distinguishes required nil/empty arrays")
	}
	input.ReceivingTaskID = "different-receiver"
	if graphSealValue(t, GraphInputDigest, input) == got {
		t.Fatal("input seal omitted the receiving task")
	}
	packet := GraphContextPacket{InputID: "input", InputSHA256: got}
	first := graphSealValue(t, GraphContextDigest, packet)
	packet.InputSHA256 = "different-file-input"
	if graphSealValue(t, GraphContextDigest, packet) == first {
		t.Fatal("context did not bind its already sealed file input")
	}
}

func TestGraphSealsCanonicalizeCopiesOfUnorderedVectors(t *testing.T) {
	input := GraphInputBinding{
		Contributions:     []AcceptedContribution{{EdgeID: "b", ConstraintRefs: []GraphScopedReference{{Ref: "z", Paths: []string{"z", "a"}}, {Ref: "a", Paths: []string{}}}, EvidenceRefs: []GraphContributedEvidence{{Ref: "y", Paths: []string{"y", "b"}}, {Ref: "b", Paths: []string{}}}}, {EdgeID: "a"}},
		ContributorVector: []GraphContributorBinding{{NodeID: "b"}, {NodeID: "a"}},
	}
	before := graphSealBytes(t, input)
	got := graphSealValue(t, GraphInputDigest, input)
	if !bytes.Equal(before, graphSealBytes(t, input)) {
		t.Fatal("seal sorting mutated caller-owned contribution data")
	}
	permuted := GraphInputBinding{
		Contributions:     []AcceptedContribution{{EdgeID: "a"}, {EdgeID: "b", ConstraintRefs: []GraphScopedReference{{Ref: "a", Paths: []string{}}, {Ref: "z", Paths: []string{"a", "z"}}}, EvidenceRefs: []GraphContributedEvidence{{Ref: "b", Paths: []string{}}, {Ref: "y", Paths: []string{"b", "y"}}}}},
		ContributorVector: []GraphContributorBinding{{NodeID: "a"}, {NodeID: "b"}},
	}
	if graphSealValue(t, GraphInputDigest, permuted) != got {
		t.Fatal("unordered contribution/path order changed the same binding")
	}
	permuted.ContributorVector[0].AcceptanceID = "new-acceptance"
	if graphSealValue(t, GraphInputDigest, permuted) == got {
		t.Fatal("new accepted result was omitted from the input seal")
	}
}

func TestGraphConditionsAndContextKeepScopeContentAndSelfExclusions(t *testing.T) {
	conditions := GraphConditionsBinding{Sources: []GraphConditionSource{{Ref: "b", AuthorityScope: "upstream_local"}, {Ref: "a", AuthorityScope: "graph_shared"}}, ConditionsSHA256: "self"}
	before := graphSealBytes(t, conditions)
	got := graphSealValue(t, GraphConditionsDigest, conditions)
	conditions.ConditionsSHA256 = "another-self"
	conditions.Sources = []GraphConditionSource{{Ref: "a", AuthorityScope: "graph_shared"}, {Ref: "b", AuthorityScope: "upstream_local"}}
	if graphSealValue(t, GraphConditionsDigest, conditions) != got {
		t.Fatal("conditions hash includes self or input order")
	}
	conditions.Sources[1].AuthorityScope = "receiver"
	if graphSealValue(t, GraphConditionsDigest, conditions) == got {
		t.Fatal("condition authority scope change was omitted")
	}
	if bytes.Contains(before, []byte(`"authorityScope":"receiver"`)) {
		t.Fatal("fixture snapshot unexpectedly changed")
	}
	packet := GraphContextPacket{ProvidedAccess: []GraphProvidedAccess{{Ref: "b", Paths: []string{"z", "a"}}, {Ref: "a", Paths: []string{}}}, PacketSHA256: "self"}
	packetBefore := graphSealBytes(t, packet)
	packetHash := graphSealValue(t, GraphContextDigest, packet)
	if !bytes.Equal(packetBefore, graphSealBytes(t, packet)) {
		t.Fatal("context seal mutated provided access")
	}
	packet.PacketSHA256 = "other-self"
	packet.ProvidedAccess = []GraphProvidedAccess{{Ref: "a", Paths: []string{}}, {Ref: "b", Paths: []string{"a", "z"}}}
	if graphSealValue(t, GraphContextDigest, packet) != packetHash {
		t.Fatal("context hash includes self or access/path order")
	}
	packet.ProvidedAccess[1].ArtifactRef.SHA256 = "changed-original"
	if graphSealValue(t, GraphContextDigest, packet) == packetHash {
		t.Fatal("context omitted its exact original artifact")
	}
}

func TestGraphOutputAndFinalSealsExcludeOnlyTheirOwnSeal(t *testing.T) {
	output := GraphOutputClosure{Kind: "candidate", ClosureSHA256: "self"}
	first := graphSealValue(t, GraphOutputDigest, output)
	output.ClosureSHA256 = "other-self"
	if graphSealValue(t, GraphOutputDigest, output) != first {
		t.Fatal("output seal hashes itself")
	}
	output.StagedBlobs.SHA256 = "different-staged-content"
	if graphSealValue(t, GraphOutputDigest, output) == first {
		t.Fatal("output omitted closed staged content")
	}
	final := GraphFinalVerificationBinding{RootBinding: GraphRootView{BasisSHA256: "sealed-root"}, ContributorVector: []GraphContributorBinding{{NodeID: "b"}, {NodeID: "a"}}, BindingSHA256: "self"}
	before := graphSealBytes(t, final)
	finalHash := graphSealValue(t, GraphFinalVerificationDigest, final)
	if !bytes.Equal(before, graphSealBytes(t, final)) {
		t.Fatal("final basis seal mutated caller data")
	}
	final.BindingSHA256 = "other-self"
	final.ContributorVector = []GraphContributorBinding{{NodeID: "a"}, {NodeID: "b"}}
	if graphSealValue(t, GraphFinalVerificationDigest, final) != finalHash {
		t.Fatal("final seal includes own hash or vector order")
	}
	final.RootBinding.BasisSHA256 = "changed-nested-root-seal"
	if graphSealValue(t, GraphFinalVerificationDigest, final) == finalHash {
		t.Fatal("final seal incorrectly excluded a nested root seal")
	}
	delivery := GraphDeliveryBinding{RootBinding: final.RootBinding, BindingSHA256: "self", OtherNodeAcceptances: []GraphContributorBinding{{NodeID: "b"}, {NodeID: "a"}}}
	deliveryHash := graphSealValue(t, GraphDeliveryDigest, delivery)
	delivery.BindingSHA256 = "other-self"
	delivery.OtherNodeAcceptances = []GraphContributorBinding{{NodeID: "a"}, {NodeID: "b"}}
	if graphSealValue(t, GraphDeliveryDigest, delivery) != deliveryHash {
		t.Fatal("delivery seal includes self or acceptance order")
	}
	delivery.DeliveryScopeSHA256 = "expanded-scope"
	if graphSealValue(t, GraphDeliveryDigest, delivery) == deliveryHash {
		t.Fatal("delivery seal omitted its captured effect authority")
	}
}
