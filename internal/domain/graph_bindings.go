package domain

// GraphRootBinding is an internal persisted/sealed binding, including physical
// identity. Public responses use GraphRootView, never this internal record.
type GraphRootBinding struct {
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
	BasisSHA256          string           `json:"basisSha256"`
}

type GraphOutputClosure struct {
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
	ClosureSHA256     string            `json:"closureSha256"`
}

type GraphCandidateOutputObservation struct {
	CandidateID          string            `json:"candidateId"`
	State                string            `json:"state"`
	ClosureRef           *GraphArtifactRef `json:"closureRef"`
	FinalVerificationRef *GraphArtifactRef `json:"finalVerificationRef"`
}

type GraphResponseOutputObservation struct {
	ResponseArtifactID string            `json:"responseArtifactId"`
	State              string            `json:"state"`
	ClosureRef         *GraphArtifactRef `json:"closureRef"`
}

type GraphOutputObservation struct {
	ProducerAttemptID string                           `json:"producerAttemptId"`
	Candidate         *GraphCandidateOutputObservation `json:"candidate"`
	Response          *GraphResponseOutputObservation  `json:"response"`
}

// AcceptedContribution is one exact direct edge selection. The transitive
// contributor vector omits edge identity so a shared ancestor appears once.
type AcceptedContribution struct {
	EdgeID            string                     `json:"edgeId"`
	EdgeKind          string                     `json:"edgeKind"`
	FromNodeID        string                     `json:"fromNodeId"`
	TaskID            string                     `json:"taskId"`
	TaskVersion       string                     `json:"taskVersion"`
	AcceptanceKind    string                     `json:"acceptanceKind"`
	AcceptanceID      string                     `json:"acceptanceId"`
	ProducerAttemptID string                     `json:"producerAttemptId"`
	InputID           string                     `json:"inputId"`
	CandidateBinding  *CandidateBinding          `json:"candidateBinding"`
	ResponseBinding   *ResponseBinding           `json:"responseBinding"`
	OutputClosureRef  GraphArtifactRef           `json:"outputClosureRef"`
	ConstraintRefs    []GraphScopedReference     `json:"constraintRefs"`
	EvidenceRefs      []GraphContributedEvidence `json:"evidenceRefs"`
}

type GraphContributorBinding struct {
	NodeID            string            `json:"nodeId"`
	TaskID            string            `json:"taskId"`
	TaskVersion       string            `json:"taskVersion"`
	AcceptanceKind    string            `json:"acceptanceKind"`
	AcceptanceID      string            `json:"acceptanceId"`
	ProducerAttemptID string            `json:"producerAttemptId"`
	InputID           string            `json:"inputId"`
	CandidateBinding  *CandidateBinding `json:"candidateBinding"`
	ResponseBinding   *ResponseBinding  `json:"responseBinding"`
	OutputClosureRef  GraphArtifactRef  `json:"outputClosureRef"`
}

type GraphInputBinding struct {
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
	ContextPacket        GraphArtifactRef          `json:"contextPacket"`
	InputSHA256          string                    `json:"inputSha256"`
}

type GraphSourceBasis struct {
	FinalVerificationRef         *GraphArtifactRef `json:"finalVerificationRef,omitzero"`
	BasisKind                    string            `json:"basisKind"`
	WorkflowID                   string            `json:"workflowId"`
	StepID                       string            `json:"stepId"`
	InputID                      string            `json:"inputId"`
	InputSHA256                  string            `json:"inputSha256"`
	CandidateBinding             *CandidateBinding `json:"candidateBinding"`
	ResponseBinding              *ResponseBinding  `json:"responseBinding"`
	ExpectedManifest             GraphArtifactRef  `json:"expectedManifest"`
	ExpectedIndex                GraphArtifactRef  `json:"expectedIndex"`
	ExpectedWorkingEntriesSHA256 string            `json:"expectedWorkingEntriesSha256"`
	ExpectedIndexEntriesSHA256   string            `json:"expectedIndexEntriesSha256"`
}

type GraphMaterializationReceipt struct {
	ID                           string            `json:"id"`
	GraphID                      string            `json:"graphId"`
	InputID                      string            `json:"inputId"`
	InputSHA256                  string            `json:"inputSha256"`
	OperationAttemptID           string            `json:"operationAttemptId"`
	ReceivingAttemptID           string            `json:"receivingAttemptId"`
	SourceBasis                  GraphSourceBasis  `json:"sourceBasis"`
	SourceSnapshotID             *string           `json:"sourceSnapshotId"`
	ObservedManifest             *GraphArtifactRef `json:"observedManifest"`
	ObservedIndex                *GraphArtifactRef `json:"observedIndex"`
	ObservedWorkingEntriesSHA256 *string           `json:"observedWorkingEntriesSha256"`
	ObservedIndexEntriesSHA256   *string           `json:"observedIndexEntriesSha256"`
	StagedBlobsSHA256            *string           `json:"stagedBlobsSha256"`
	OperationEffects             *GraphArtifactRef `json:"operationEffects"`
	EffectCount                  string            `json:"effectCount"`
	EffectsKnown                 bool              `json:"effectsKnown"`
	EvidenceStatus               string            `json:"evidenceStatus"`
	Failure                      *Error            `json:"failure"`
	State                        string            `json:"state"`
	CompletedAt                  string            `json:"completedAt"`
}

type GraphConditionSource struct {
	Ref            string            `json:"ref"`
	AuthorityScope string            `json:"authorityScope"`
	NodeID         *string           `json:"nodeId"`
	TaskVersion    *string           `json:"taskVersion"`
	OriginalRef    GraphArtifactRef  `json:"originalRef"`
	GuidanceRef    *GraphArtifactRef `json:"guidanceRef"`
	AcceptanceRef  *GraphArtifactRef `json:"acceptanceRef"`
}

type GraphConditionsBinding struct {
	GraphID             string                 `json:"graphId"`
	GraphVersion        string                 `json:"graphVersion"`
	ReceiverTaskID      string                 `json:"receiverTaskId"`
	ReceiverTaskVersion string                 `json:"receiverTaskVersion"`
	Sources             []GraphConditionSource `json:"sources"`
	ConditionsSHA256    string                 `json:"conditionsSha256"`
}

type GraphScopedReference struct {
	Ref            string           `json:"ref"`
	ArtifactRef    GraphArtifactRef `json:"artifactRef"`
	AuthorityScope string           `json:"authorityScope"`
	NodeID         *string          `json:"nodeId"`
	TaskVersion    *string          `json:"taskVersion"`
	Paths          []string         `json:"paths"`
}

type GraphContributedEvidence struct {
	Ref               string           `json:"ref"`
	EdgeID            string           `json:"edgeId"`
	SourceNodeID      string           `json:"sourceNodeId"`
	AcceptanceKind    string           `json:"acceptanceKind"`
	AcceptanceID      string           `json:"acceptanceId"`
	ProducerAttemptID string           `json:"producerAttemptId"`
	ResultKind        string           `json:"resultKind"`
	ResultID          string           `json:"resultId"`
	ArtifactRef       GraphArtifactRef `json:"artifactRef"`
	Scope             string           `json:"scope"`
	Paths             []string         `json:"paths"`
}

type GraphProvidedAccess struct {
	Ref         string           `json:"ref"`
	ArtifactRef GraphArtifactRef `json:"artifactRef"`
	Scope       string           `json:"scope"`
	Paths       []string         `json:"paths"`
	Mode        string           `json:"mode"`
}

type GraphContextPacket struct {
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
	PacketSHA256         string                     `json:"packetSha256"`
}

type GraphFinalVerificationBinding struct {
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
	BindingSHA256             string                    `json:"bindingSha256"`
}

type GraphDeliveryBinding struct {
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
	BindingSHA256           string                    `json:"bindingSha256"`
}
