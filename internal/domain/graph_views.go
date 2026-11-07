package domain

// GraphRootView is the explicit public projection. It deliberately has no
// project/repository native identity or private workspace fields.
type GraphRootView struct {
	GraphID              string           `json:"graphId"`
	RootTaskID           string           `json:"rootTaskId"`
	OperationAttemptID   string           `json:"operationAttemptId"`
	SourceSnapshotID     string           `json:"sourceSnapshotId"`
	Manifest             GraphArtifactRef `json:"manifest"`
	Index                GraphArtifactRef `json:"index"`
	StagedBlobs          GraphArtifactRef `json:"stagedBlobs"`
	ProjectID            string           `json:"projectId"`
	SelectedRelativePath string           `json:"selectedRelativePath"`
	BaseCommit           string           `json:"baseCommit"`
	Branch               *string          `json:"branch"`
	BasisSHA256          string           `json:"basisSha256"`
}

func GraphRootPublic(binding GraphRootBinding) GraphRootView {
	return GraphRootView{GraphID: binding.GraphID, RootTaskID: binding.RootTaskID, OperationAttemptID: binding.OperationAttemptID, SourceSnapshotID: binding.SourceSnapshotID, Manifest: binding.Manifest, Index: binding.Index, StagedBlobs: binding.StagedBlobs, ProjectID: binding.ProjectID, SelectedRelativePath: binding.SelectedRelativePath, BaseCommit: binding.BaseCommit, Branch: binding.Branch, BasisSHA256: binding.BasisSHA256}
}

type GraphInputSummary struct {
	ID                   string  `json:"id"`
	GraphID              string  `json:"graphId"`
	GraphVersion         string  `json:"graphVersion"`
	RootTaskID           string  `json:"rootTaskId"`
	OperationAttemptID   string  `json:"operationAttemptId"`
	Kind                 string  `json:"kind"`
	State                string  `json:"state"`
	Current              bool    `json:"current"`
	ReceivingNodeID      *string `json:"receivingNodeId"`
	ReceivingTaskID      *string `json:"receivingTaskId"`
	ReceivingTaskVersion *string `json:"receivingTaskVersion"`
	RootBasisSHA256      *string `json:"rootBasisSha256"`
	InputSHA256          *string `json:"inputSha256"`
	Failure              *Error  `json:"failure"`
	CreatedAt            string  `json:"createdAt"`
	UpdatedAt            string  `json:"updatedAt"`
}

type GraphInputDetail struct {
	Summary          GraphInputSummary   `json:"summary"`
	RootBinding      *GraphRootView      `json:"rootBinding"`
	InputBinding     *GraphInputBinding  `json:"inputBinding"`
	ConditionsRef    *GraphArtifactRef   `json:"conditionsRef"`
	ContextPacket    *GraphContextPacket `json:"contextPacket"`
	OperationEffects *GraphArtifactRef   `json:"operationEffects"`
	ConflictArtifact *GraphArtifactRef   `json:"conflictArtifact"`
}

type GraphMemberOrigin struct {
	Kind              string  `json:"kind"`
	NodeID            *string `json:"nodeId"`
	TaskID            string  `json:"taskId"`
	TaskVersion       *string `json:"taskVersion"`
	AcceptanceKind    *string `json:"acceptanceKind"`
	AcceptanceID      *string `json:"acceptanceId"`
	ProducerAttemptID string  `json:"producerAttemptId"`
	InputID           string  `json:"inputId"`
	StateSHA256       string  `json:"stateSha256"`
}

type GraphMemberView struct {
	Dimension   string              `json:"dimension"`
	Path        string              `json:"path"`
	State       string              `json:"state"`
	Type        *string             `json:"type"`
	Mode        *string             `json:"mode"`
	ByteLength  *string             `json:"byteLength"`
	SHA256      *string             `json:"sha256"`
	Target      *string             `json:"target"`
	ObjectID    *string             `json:"objectId"`
	ArtifactRef *GraphArtifactRef   `json:"artifactRef"`
	Origins     []GraphMemberOrigin `json:"origins"`
}

// GraphInputRecord is private persisted state. Only Public is returned as an
// input read; the private root binding remains available for recovery/proof.
type GraphInputRecord struct {
	ConditionsBinding     *GraphConditionsBinding
	Trigger               GraphFreshTrigger
	ConsumptionKey        string
	ExpectedContributions []AcceptedContribution
	CompletionFingerprint string
	Sequence              string
	Public                GraphInputDetail
	PrivateRoot           *GraphRootBinding
	ExpectedAuthority     GraphDispatchBinding
}

// GraphDispatchBinding binds actual local preparation or node work to current
// authority. Node fields are absent only for a real root-only operation.
type GraphDispatchBinding struct {
	GraphID              string                    `json:"graphId"`
	GraphVersion         string                    `json:"graphVersion"`
	RootTaskID           string                    `json:"rootTaskId"`
	OperationAttemptID   string                    `json:"operationAttemptId"`
	OwnerInstanceID      string                    `json:"ownerInstanceId"`
	Generation           string                    `json:"generation"`
	GraphControlRevision string                    `json:"graphControlRevision"`
	BudgetAuthorityEpoch string                    `json:"budgetAuthorityEpoch"`
	WindowNumber         string                    `json:"windowNumber"`
	NodeID               *string                   `json:"nodeId"`
	TaskID               *string                   `json:"taskId"`
	TaskVersion          *string                   `json:"taskVersion"`
	NodeControlRevision  *string                   `json:"nodeControlRevision"`
	InputID              *string                   `json:"inputId"`
	ContributorVector    []GraphContributorBinding `json:"contributorVector"`
}
