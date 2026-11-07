package domain

// Pointer containers retain omitted versus explicitly empty declaration choices.
type GraphStartInput struct {
	RequestID              string                  `json:"requestId"`
	ProjectID              string                  `json:"projectId"`
	SharedInstructions     *string                 `json:"sharedInstructions,omitzero"`
	Title                  *string                 `json:"title,omitzero"`
	Nodes                  []GraphNodeInput        `json:"nodes"`
	Dependencies           *[]GraphDependencyInput `json:"dependencies,omitzero"`
	FinalNodeKey           string                  `json:"finalNodeKey,omitzero"`
	Delivery               *WorkflowDeliveryInput  `json:"delivery,omitzero"`
	Routing                *RoutingOptions         `json:"routing,omitzero"`
	ExpectedPolicyRevision string                  `json:"expectedPolicyRevision,omitzero"`
	PolicyOverride         string                  `json:"policyOverride,omitzero"`
	ExecutionMode          string                  `json:"executionMode,omitzero"`
	MaxParallelism         string                  `json:"maxParallelism,omitzero"`
}

type GraphNodeInput struct {
	Key            string           `json:"key"`
	OriginalText   string           `json:"originalText"`
	Pins           *ExecutionPins   `json:"pins,omitzero"`
	Acceptance     *AcceptanceInput `json:"acceptance,omitzero"`
	TimeoutSeconds *int             `json:"timeoutSeconds,omitzero"`
	FakeScenario   string           `json:"fakeScenario,omitzero"`
	Routing        *RoutingOptions  `json:"routing,omitzero"`
	Workflow       *WorkflowInput   `json:"workflow,omitzero"`
}

type GraphDependencyInput struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind,omitzero"`
}

type GraphNodeAuthority struct {
	NodeID              string `json:"nodeId"`
	TaskVersion         string `json:"taskVersion"`
	NodeControlRevision string `json:"nodeControlRevision"`
}

type GraphAdvanceInput struct {
	RequestID                    string               `json:"requestId"`
	GraphVersion                 string               `json:"graphVersion"`
	ExpectedGraphControlRevision string               `json:"expectedGraphControlRevision"`
	ExpectedBudgetAuthorityEpoch string               `json:"expectedBudgetAuthorityEpoch"`
	ExpectedWindowNumber         string               `json:"expectedWindowNumber"`
	NodeAuthorities              []GraphNodeAuthority `json:"nodeAuthorities"`
	RebindNodeIDs                *[]string            `json:"rebindNodeIds,omitzero"`
}

type GraphStopInput struct {
	RequestID                    string `json:"requestId"`
	ExpectedGraphControlRevision string `json:"expectedGraphControlRevision"`
}

type GraphNodeStopInput struct {
	RequestID                    string `json:"requestId"`
	ExpectedGraphControlRevision string `json:"expectedGraphControlRevision"`
	NodeID                       string `json:"nodeId"`
	ExpectedNodeControlRevision  string `json:"expectedNodeControlRevision"`
}

// GuidanceInput stays flattened on the wire and keeps its accepted field names.
type GraphGuidanceInput struct {
	GuidanceInput
	ExpectedGraphControlRevision string `json:"expectedGraphControlRevision"`
	NodeID                       string `json:"nodeId"`
	ExpectedNodeControlRevision  string `json:"expectedNodeControlRevision"`
	TaskVersion                  string `json:"taskVersion"`
}

type GraphBudgetAmendmentInput struct {
	RequestID                    string `json:"requestId"`
	ExpectedGraphControlRevision string `json:"expectedGraphControlRevision"`
	ExpectedBudgetAuthorityEpoch string `json:"expectedBudgetAuthorityEpoch"`
	ExpectedWindowNumber         string `json:"expectedWindowNumber"`
	AdditionalAdmissions         string `json:"additionalAdmissions"`
	NewWindowSeconds             *int   `json:"newWindowSeconds,omitzero"`
}

type GraphDefinitionRef struct {
	GraphID           string `json:"graphId"`
	GraphVersion      string `json:"graphVersion"`
	DeclarationSHA256 string `json:"declarationSha256"`
}

type GraphDefinition struct {
	GraphID         string          `json:"graphId"`
	GraphVersion    string          `json:"graphVersion"`
	RootTaskID      string          `json:"rootTaskId"`
	Original        GraphStartInput `json:"original"`
	RequestedPolicy Policy          `json:"requestedPolicy"`
	RoutingPolicy   RoutingPolicy   `json:"routingPolicy"`
	Delivery        DeliveryScope   `json:"delivery"`
	CreatedAt       string          `json:"createdAt"`
}

type GraphCounts struct {
	Required    string `json:"required"`
	Waiting     string `json:"waiting"`
	Ready       string `json:"ready"`
	Preparing   string `json:"preparing"`
	Queued      string `json:"queued"`
	Running     string `json:"running"`
	Blocked     string `json:"blocked"`
	Cancelling  string `json:"cancelling"`
	Cancelled   string `json:"cancelled"`
	Failed      string `json:"failed"`
	Interrupted string `json:"interrupted"`
	Completed   string `json:"completed"`
}

type GraphStopRecord struct {
	RequestID            string   `json:"requestId"`
	AcceptedAt           string   `json:"acceptedAt"`
	GraphControlRevision string   `json:"graphControlRevision"`
	NodeControlRevision  *string  `json:"nodeControlRevision"`
	TargetAttemptIDs     []string `json:"targetAttemptIds"`
}

type GraphSummary struct {
	ID                   string           `json:"id"`
	RootTaskID           string           `json:"rootTaskId"`
	ProjectID            string           `json:"projectId"`
	GraphVersion         string           `json:"graphVersion"`
	ControlRevision      string           `json:"controlRevision"`
	StateRevision        string           `json:"stateRevision"`
	BudgetAuthorityEpoch string           `json:"budgetAuthorityEpoch"`
	MaxParallelism       string           `json:"maxParallelism"`
	State                string           `json:"state"`
	NextAction           string           `json:"nextAction"`
	NextOwner            string           `json:"nextOwner"`
	RootInputID          *string          `json:"rootInputId"`
	FinalNodeID          *string          `json:"finalNodeId"`
	Stop                 *GraphStopRecord `json:"stop"`
	Counts               GraphCounts      `json:"counts"`
	CreatedAt            string           `json:"createdAt"`
	UpdatedAt            string           `json:"updatedAt"`
}

type GraphAcceptedResultRef struct {
	AcceptanceKind     string  `json:"acceptanceKind"`
	AcceptanceID       string  `json:"acceptanceId"`
	TaskVersion        string  `json:"taskVersion"`
	ProducerAttemptID  string  `json:"producerAttemptId"`
	CandidateID        *string `json:"candidateId"`
	ResponseArtifactID *string `json:"responseArtifactId"`
	InputID            string  `json:"inputId"`
}

type GraphNodeSummary struct {
	ID              string                  `json:"id"`
	GraphID         string                  `json:"graphId"`
	Key             string                  `json:"key"`
	TaskID          string                  `json:"taskId"`
	TaskVersion     string                  `json:"taskVersion"`
	TaskRevision    string                  `json:"taskRevision"`
	ControlRevision string                  `json:"controlRevision"`
	StateRevision   string                  `json:"stateRevision"`
	State           string                  `json:"state"`
	NextAction      string                  `json:"nextAction"`
	NextOwner       string                  `json:"nextOwner"`
	InputID         *string                 `json:"inputId"`
	NodeAdmissionID *string                 `json:"nodeAdmissionId"`
	LatestAttemptID *string                 `json:"latestAttemptId"`
	AcceptedResult  *GraphAcceptedResultRef `json:"acceptedResult"`
	Stop            *GraphStopRecord        `json:"stop"`
}

type GraphDispatchAuthorityView struct {
	OwnerInstanceID      string               `json:"ownerInstanceId"`
	Generation           string               `json:"generation"`
	GraphControlRevision string               `json:"graphControlRevision"`
	BudgetAuthorityEpoch string               `json:"budgetAuthorityEpoch"`
	WindowNumber         string               `json:"windowNumber"`
	NodeAuthorities      []GraphNodeAuthority `json:"nodeAuthorities"`
	State                string               `json:"state"`
}

type GraphNodeConsumption struct {
	NodeID              string `json:"nodeId"`
	TaskID              string `json:"taskId"`
	ConsumedAdmissions  string `json:"consumedAdmissions"`
	AutomaticAdmissions string `json:"automaticAdmissions"`
}

type GraphBudgetView struct {
	GraphID              string                 `json:"graphId"`
	BudgetOwnerTaskID    string                 `json:"budgetOwnerTaskId"`
	BudgetAuthorityEpoch string                 `json:"budgetAuthorityEpoch"`
	Budget               TaskBudget             `json:"budget"`
	NodeConsumption      []GraphNodeConsumption `json:"nodeConsumption"`
}

type GraphBlockerSummary struct {
	Code       string  `json:"code"`
	Message    string  `json:"message"`
	NodeID     *string `json:"nodeId"`
	InputID    *string `json:"inputId"`
	ArtifactID *string `json:"artifactId"`
}

type GraphAdmission struct {
	Graph                GraphSummary               `json:"graph"`
	Definition           GraphDefinitionRef         `json:"definition"`
	Nodes                []GraphNodeSummary         `json:"nodes"`
	PreparationAttemptID string                     `json:"preparationAttemptId"`
	Budget               GraphBudgetView            `json:"budget"`
	DispatchAuthority    GraphDispatchAuthorityView `json:"dispatchAuthority"`
}

type GraphDetail struct {
	Graph             GraphSummary                `json:"graph"`
	Definition        GraphDefinitionRef          `json:"definition"`
	Nodes             []GraphNodeSummary          `json:"nodes"`
	Budget            GraphBudgetView             `json:"budget"`
	ActiveClaims      []GraphClaimView            `json:"activeClaims"`
	DispatchAuthority *GraphDispatchAuthorityView `json:"dispatchAuthority"`
	Blockers          []GraphBlockerSummary       `json:"blockers"`
	Checkpoints       []RecoveryCheckpoint        `json:"checkpoints,omitempty"`
}

type GraphRebindProvenance struct {
	Kind                    string `json:"kind"`
	RequestID               string `json:"requestId"`
	PreviousInputID         string `json:"previousInputId"`
	ContributorVectorSHA256 string `json:"contributorVectorSha256"`
}

type GraphReboundNode struct {
	NodeID          string                `json:"nodeId"`
	TaskID          string                `json:"taskId"`
	FromTaskVersion string                `json:"fromTaskVersion"`
	ToTaskVersion   string                `json:"toTaskVersion"`
	Provenance      GraphRebindProvenance `json:"provenance"`
}

type GraphAdvanceResult struct {
	Graph                 GraphSummary                `json:"graph"`
	PreparationAttemptIDs []string                    `json:"preparationAttemptIds"`
	Dispatches            []GraphDispatchView         `json:"dispatches"`
	ReboundNodes          []GraphReboundNode          `json:"reboundNodes"`
	Budget                GraphBudgetView             `json:"budget"`
	DispatchAuthority     *GraphDispatchAuthorityView `json:"dispatchAuthority"`
}

type GraphStopResult struct {
	Graph            GraphSummary      `json:"graph"`
	Node             *GraphNodeSummary `json:"node"`
	StopAccepted     bool              `json:"stopAccepted"`
	Reason           string            `json:"reason"`
	TargetAttemptIDs []string          `json:"targetAttemptIds"`
	AffectedNodeIDs  []string          `json:"affectedNodeIds"`
}

type GraphGuidanceResult struct {
	Graph  GraphSummary     `json:"graph"`
	Node   GraphNodeSummary `json:"node"`
	Result GuidanceResult   `json:"result"`
}

type GraphBudgetAmendmentResult struct {
	Graph     GraphSummary    `json:"graph"`
	Budget    GraphBudgetView `json:"budget"`
	Amendment BudgetAmendment `json:"amendment"`
}

type GraphDispatch struct {
	ID                     string                    `json:"id"`
	GraphID                string                    `json:"graphId"`
	GraphVersion           string                    `json:"graphVersion"`
	NodeID                 string                    `json:"nodeId"`
	TaskID                 string                    `json:"taskId"`
	TaskVersion            string                    `json:"taskVersion"`
	NodeAdmissionID        string                    `json:"nodeAdmissionId"`
	WorkflowID             string                    `json:"workflowId"`
	StepID                 string                    `json:"stepId"`
	AttemptID              string                    `json:"attemptId"`
	ExecutorKind           string                    `json:"executorKind"`
	Role                   string                    `json:"role"`
	InputID                string                    `json:"inputId"`
	InputSHA256            string                    `json:"inputSha256"`
	ContextPacket          GraphArtifactRef          `json:"contextPacket"`
	ContributorVector      []GraphContributorBinding `json:"contributorVector"`
	OwnerInstanceID        string                    `json:"ownerInstanceId"`
	GraphControlRevision   string                    `json:"graphControlRevision"`
	NodeControlRevision    string                    `json:"nodeControlRevision"`
	BudgetOwnerTaskID      string                    `json:"budgetOwnerTaskId"`
	BudgetAuthorityEpoch   string                    `json:"budgetAuthorityEpoch"`
	WindowNumber           string                    `json:"windowNumber"`
	ObservedBudgetRevision string                    `json:"observedBudgetRevision"`
	SourceBasis            GraphSourceBasis          `json:"sourceBasis"`
	FreshTriggerID         string                    `json:"freshTriggerId"`
	LogicalSlot            string                    `json:"logicalSlot"`
	ExecutionOrdinal       string                    `json:"executionOrdinal"`
	NativeConsumptionKey   *string                   `json:"nativeConsumptionKey"`
	CreatedAt              string                    `json:"createdAt"`
}

type GraphDispatchView struct {
	Dispatch          GraphDispatch                `json:"dispatch"`
	Materialization   *GraphMaterializationReceipt `json:"materialization"`
	OutputObservation *GraphOutputObservation      `json:"outputObservation"`
}
