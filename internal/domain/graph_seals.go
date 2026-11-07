package domain

import (
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"slices"
	"sort"
)

// These seals are canonical representations, not authorization or physical
// closure proofs. Callers validate the exact stored tuple and its artifacts.
// Explicit structs preserve protocol field order; maps are not canonical input.
func graphSeal(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func GraphRootDigest(value GraphRootBinding) (string, error) {
	return graphSeal(struct {
		GraphID              string           `json:"graphId"`
		RootTaskID           string           `json:"rootTaskId"`
		OperationAttemptID   string           `json:"operationAttemptId"`
		SourceSnapshotID     string           `json:"sourceSnapshotId"`
		Manifest             GraphArtifactRef `json:"manifest"`
		Index                GraphArtifactRef `json:"index"`
		StagedBlobs          GraphArtifactRef `json:"stagedBlobs"`
		ProjectID            string           `json:"projectId"`
		ProjectIdentity      string           `json:"projectIdentity"`
		RepositoryIdentity   string           `json:"repositoryIdentity"`
		SelectedRelativePath string           `json:"selectedRelativePath"`
		BaseCommit           string           `json:"baseCommit"`
		Branch               *string          `json:"branch"`
	}{value.GraphID, value.RootTaskID, value.OperationAttemptID, value.SourceSnapshotID,
		value.Manifest, value.Index, value.StagedBlobs, value.ProjectID, value.ProjectIdentity,
		value.RepositoryIdentity, value.SelectedRelativePath, value.BaseCommit, value.Branch})
}

func GraphInputDigest(value GraphInputBinding) (string, error) {
	return graphSeal(struct {
		ID                   string                    `json:"id"`
		GraphID              string                    `json:"graphId"`
		GraphVersion         string                    `json:"graphVersion"`
		RootBasisSHA256      string                    `json:"rootBasisSha256"`
		OperationAttemptID   string                    `json:"operationAttemptId"`
		ReceivingNodeID      string                    `json:"receivingNodeId"`
		ReceivingTaskID      string                    `json:"receivingTaskId"`
		ReceivingTaskVersion string                    `json:"receivingTaskVersion"`
		Contributions        []AcceptedContribution    `json:"contributions"`
		ContributorVector    []GraphContributorBinding `json:"contributorVector"`
		Manifest             GraphArtifactRef          `json:"manifest"`
		Index                GraphArtifactRef          `json:"index"`
		StagedBlobs          GraphArtifactRef          `json:"stagedBlobs"`
		Provenance           GraphArtifactRef          `json:"provenance"`
	}{value.ID, value.GraphID, value.GraphVersion, value.RootBasisSHA256, value.OperationAttemptID,
		value.ReceivingNodeID, value.ReceivingTaskID, value.ReceivingTaskVersion,
		graphSealContributions(value.Contributions), graphSealContributors(value.ContributorVector),
		value.Manifest, value.Index, value.StagedBlobs, value.Provenance})
}

func GraphConditionsDigest(value GraphConditionsBinding) (string, error) {
	sources := append([]GraphConditionSource{}, value.Sources...)
	sort.SliceStable(sources, func(i, j int) bool { return sources[i].Ref < sources[j].Ref })
	return graphSeal(struct {
		GraphID             string                 `json:"graphId"`
		GraphVersion        string                 `json:"graphVersion"`
		ReceiverTaskID      string                 `json:"receiverTaskId"`
		ReceiverTaskVersion string                 `json:"receiverTaskVersion"`
		Sources             []GraphConditionSource `json:"sources"`
	}{value.GraphID, value.GraphVersion, value.ReceiverTaskID, value.ReceiverTaskVersion, sources})
}

func GraphContextDigest(value GraphContextPacket) (string, error) {
	return graphSeal(struct {
		ID                   string                     `json:"id"`
		GraphID              string                     `json:"graphId"`
		InputID              string                     `json:"inputId"`
		InputSHA256          string                     `json:"inputSha256"`
		ReceivingTaskVersion string                     `json:"receivingTaskVersion"`
		GraphInstructionsRef GraphArtifactRef           `json:"graphInstructionsRef"`
		NodeInstructionsRef  GraphArtifactRef           `json:"nodeInstructionsRef"`
		GuidanceRef          *GraphArtifactRef          `json:"guidanceRef"`
		AcceptanceRef        *GraphArtifactRef          `json:"acceptanceRef"`
		ConstraintRefs       []GraphScopedReference     `json:"constraintRefs"`
		ContributedEvidence  []GraphContributedEvidence `json:"contributedEvidence"`
		DerivedDigestRef     *GraphArtifactRef          `json:"derivedDigestRef"`
		ProvidedAccess       []GraphProvidedAccess      `json:"providedAccess"`
	}{value.ID, value.GraphID, value.InputID, value.InputSHA256, value.ReceivingTaskVersion,
		value.GraphInstructionsRef, value.NodeInstructionsRef, value.GuidanceRef, value.AcceptanceRef,
		graphSealScoped(value.ConstraintRefs), graphSealEvidence(value.ContributedEvidence),
		value.DerivedDigestRef, graphSealAccess(value.ProvidedAccess)})
}

func GraphOutputDigest(value GraphOutputClosure) (string, error) {
	return graphSeal(struct {
		GraphID           string            `json:"graphId"`
		NodeID            string            `json:"nodeId"`
		ReceivedInputID   string            `json:"receivedInputId"`
		Kind              string            `json:"kind"`
		ProducerAttemptID string            `json:"producerAttemptId"`
		CandidateID       *string           `json:"candidateId"`
		ResponseArtifact  *GraphArtifactRef `json:"responseArtifact"`
		Manifest          GraphArtifactRef  `json:"manifest"`
		Index             GraphArtifactRef  `json:"index"`
		StagedBlobs       GraphArtifactRef  `json:"stagedBlobs"`
	}{value.GraphID, value.NodeID, value.ReceivedInputID, value.Kind, value.ProducerAttemptID,
		value.CandidateID, value.ResponseArtifact, value.Manifest, value.Index, value.StagedBlobs})
}

func GraphFinalVerificationDigest(value GraphFinalVerificationBinding) (string, error) {
	return graphSeal(struct {
		GraphID                   string                    `json:"graphId"`
		GraphVersion              string                    `json:"graphVersion"`
		FinalNodeID               string                    `json:"finalNodeId"`
		FinalTaskID               string                    `json:"finalTaskId"`
		FinalTaskVersion          string                    `json:"finalTaskVersion"`
		FinalWorkflowID           string                    `json:"finalWorkflowId"`
		RootBinding               GraphRootView             `json:"rootBinding"`
		FinalCandidateBinding     CandidateBinding          `json:"finalCandidateBinding"`
		OutputClosureRef          GraphArtifactRef          `json:"outputClosureRef"`
		FinalInputID              string                    `json:"finalInputId"`
		ContributorVector         []GraphContributorBinding `json:"contributorVector"`
		GraphConditionsRef        GraphArtifactRef          `json:"graphConditionsRef"`
		GraphConditionsSHA256     string                    `json:"graphConditionsSha256"`
		OriginalAccess            []GraphProvidedAccess     `json:"originalAccess"`
		CumulativeDeltaRef        GraphArtifactRef          `json:"cumulativeDeltaRef"`
		HasCumulativeWorkingDelta bool                      `json:"hasCumulativeWorkingDelta"`
		HasCumulativeIndexDelta   bool                      `json:"hasCumulativeIndexDelta"`
	}{value.GraphID, value.GraphVersion, value.FinalNodeID, value.FinalTaskID, value.FinalTaskVersion,
		value.FinalWorkflowID, value.RootBinding, value.FinalCandidateBinding, value.OutputClosureRef,
		value.FinalInputID, graphSealContributors(value.ContributorVector), value.GraphConditionsRef,
		value.GraphConditionsSHA256, graphSealAccess(value.OriginalAccess), value.CumulativeDeltaRef,
		value.HasCumulativeWorkingDelta, value.HasCumulativeIndexDelta})
}

func GraphDeliveryDigest(value GraphDeliveryBinding) (string, error) {
	return graphSeal(struct {
		GraphID                 string                    `json:"graphId"`
		GraphVersion            string                    `json:"graphVersion"`
		GraphControlRevision    string                    `json:"graphControlRevision"`
		RootBinding             GraphRootView             `json:"rootBinding"`
		FinalNodeID             string                    `json:"finalNodeId"`
		FinalTaskID             string                    `json:"finalTaskId"`
		FinalTaskVersion        string                    `json:"finalTaskVersion"`
		FinalWorkflowID         string                    `json:"finalWorkflowId"`
		FinalCandidateBinding   CandidateBinding          `json:"finalCandidateBinding"`
		FinalInputID            string                    `json:"finalInputId"`
		ContributorVector       []GraphContributorBinding `json:"contributorVector"`
		FinalVerificationRef    GraphArtifactRef          `json:"finalVerificationRef"`
		OtherNodeAcceptances    []GraphContributorBinding `json:"otherNodeAcceptances"`
		PreDeliveryEvidenceUses []EvidenceUse             `json:"preDeliveryEvidenceUses"`
		DeliveryScopeSHA256     string                    `json:"deliveryScopeSha256"`
	}{value.GraphID, value.GraphVersion, value.GraphControlRevision, value.RootBinding,
		value.FinalNodeID, value.FinalTaskID, value.FinalTaskVersion, value.FinalWorkflowID,
		value.FinalCandidateBinding, value.FinalInputID, graphSealContributors(value.ContributorVector),
		value.FinalVerificationRef, graphSealContributors(value.OtherNodeAcceptances),
		graphSealUses(value.PreDeliveryEvidenceUses), value.DeliveryScopeSHA256})
}

func graphSealPaths(paths []string) []string {
	copy := append([]string{}, paths...)
	slices.Sort(copy)
	return copy
}

func graphSealContributors(values []GraphContributorBinding) []GraphContributorBinding {
	copy := append([]GraphContributorBinding{}, values...)
	sort.SliceStable(copy, func(i, j int) bool { return copy[i].NodeID < copy[j].NodeID })
	return copy
}

func graphSealContributions(values []AcceptedContribution) []AcceptedContribution {
	copy := append([]AcceptedContribution{}, values...)
	for i := range copy {
		copy[i].ConstraintRefs = graphSealScoped(copy[i].ConstraintRefs)
		copy[i].EvidenceRefs = graphSealEvidence(copy[i].EvidenceRefs)
	}
	sort.SliceStable(copy, func(i, j int) bool { return copy[i].EdgeID < copy[j].EdgeID })
	return copy
}

func graphSealScoped(values []GraphScopedReference) []GraphScopedReference {
	copy := append([]GraphScopedReference{}, values...)
	for i := range copy {
		copy[i].Paths = graphSealPaths(copy[i].Paths)
	}
	sort.SliceStable(copy, func(i, j int) bool { return copy[i].Ref < copy[j].Ref })
	return copy
}

func graphSealEvidence(values []GraphContributedEvidence) []GraphContributedEvidence {
	copy := append([]GraphContributedEvidence{}, values...)
	for i := range copy {
		copy[i].Paths = graphSealPaths(copy[i].Paths)
	}
	sort.SliceStable(copy, func(i, j int) bool { return copy[i].Ref < copy[j].Ref })
	return copy
}

func graphSealAccess(values []GraphProvidedAccess) []GraphProvidedAccess {
	copy := append([]GraphProvidedAccess{}, values...)
	for i := range copy {
		copy[i].Paths = graphSealPaths(copy[i].Paths)
	}
	sort.SliceStable(copy, func(i, j int) bool { return copy[i].Ref < copy[j].Ref })
	return copy
}

func graphSealUses(values []EvidenceUse) []EvidenceUse {
	copy := append([]EvidenceUse{}, values...)
	for i := range copy {
		copy[i].Applicability.Paths = graphSealPaths(copy[i].Applicability.Paths)
		copy[i].Applicability.ConditionKeys = graphSealPaths(copy[i].Applicability.ConditionKeys)
	}
	sort.SliceStable(copy, func(i, j int) bool {
		if copy[i].Kind != copy[j].Kind {
			return copy[i].Kind < copy[j].Kind
		}
		return copy[i].EvidenceID < copy[j].EvidenceID
	})
	return copy
}
