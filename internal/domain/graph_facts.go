package domain

// Controller facts are NEVER decoded from public request JSON. They describe
// actual I/O performed outside SQL; store checks their registered immutable
// tuples and current authority in the transition transaction.
type GraphCreateFacts struct {
	ProjectIdentity      string
	RepositoryIdentity   string
	SelectedRelativePath string
	BaseCommit           string
	Branch               *string
}

type GraphFreshTrigger struct {
	Kind            string // explicit, completion, claim_release or response_ack
	ID              string // exact request, settled attempt or response acceptance ID
	OwnerGeneration string
}

type GraphInputPreparationFacts struct {
	Trigger           GraphFreshTrigger
	Control           GraphDispatchBinding
	RootBinding       GraphRootBinding
	Contributions     []AcceptedContribution
	ContributorVector []GraphContributorBinding
	VerifiedArtifacts []VerifiedResponse
}

type GraphInputOperation struct {
	CreateFacts       GraphCreateFacts
	Definition        GraphDefinition
	InputRecord       GraphInputRecord
	ID                string
	Kind              string
	GraphID           string
	RootTaskID        string
	NodeAuthority     *GraphNodeAuthority
	Attempt           Attempt
	RootBinding       *GraphRootBinding
	Control           GraphDispatchBinding
	Contributions     []AcceptedContribution
	ContributorVector []GraphContributorBinding
	DeadlineAt        string
}

type GraphInputCompletionFacts struct {
	ConditionsBinding *GraphConditionsBinding
	RootBinding       *GraphRootBinding
	InputBinding      *GraphInputBinding
	SourceSnapshot    *SourceSnapshot
	ConditionsRef     *GraphArtifactRef
	ContextPacket     *GraphContextPacket
	OperationEffects  *GraphArtifactRef
	ConflictArtifact  *GraphArtifactRef
	EffectCount       string
	EffectsKnown      bool
	EvidenceStatus    string
	Failure           *Error
	Cleanup           Cleanup
	State             string
	VerifiedArtifacts []VerifiedResponse
}

type GraphNodeAdmissionFacts struct {
	PreparationFailure *Error
	Control            GraphDispatchBinding
	NodeID             string
	TaskVersion        string
	InputBinding       GraphInputBinding
	SourceBasis        GraphSourceBasis
	ConditionsBinding  GraphConditionsBinding
	ConditionsRef      GraphArtifactRef
	Capture            *WorkflowCapture
	Routing            RoutingFacts
	VerifiedArtifacts  []VerifiedResponse
}

type GraphAdvanceFacts struct {
	Stages       []WorkflowStepAdmissionFacts
	Preparations []GraphInputPreparationFacts
	Admissions   []GraphNodeAdmissionFacts
}

type GraphContinuationFacts struct {
	Trigger      GraphFreshTrigger
	Preparations []GraphInputPreparationFacts
	Admissions   []GraphNodeAdmissionFacts
}

type GraphProgress struct {
	Graph        GraphSummary
	Preparations []GraphInputOperation
	Admissions   []GraphDispatchView
}

// The private identity is persisted alongside the exact dispatch receipt, not
// inside its public view, public receipt or graph_operation_effects artifact.
type GraphPrivateMaterialization struct {
	WorkspaceIdentity string `json:"workspaceIdentity"`
	GitControlSHA256  string `json:"gitControlSha256"`
}

type GraphMaterializationFacts struct {
	Receipt           GraphMaterializationReceipt
	ReceiptArtifact   GraphArtifactRef
	SourceSnapshot    *SourceSnapshot
	WorkspaceIdentity string
	GitControlSHA256  string
	VerifiedArtifacts []VerifiedResponse
}

type GraphDispatchRecord struct {
	OwnerResumeRequestID    *string `json:"ownerResumeRequestId,omitzero"`
	CandidateClosure        *GraphOutputClosure
	ResponseClosure         *GraphOutputClosure
	FinalVerification       *GraphFinalVerificationBinding
	MaterializationArtifact *GraphArtifactRef
	Sequence                string
	Public                  GraphDispatchView
	PrivateMaterialization  *GraphPrivateMaterialization
	KnownFirstGOAt          *string
	KnownFirstGOAttemptID   *string
}

// The controller supplies current subjects and authority, never future receiving
// attempt/step/dispatch IDs or materialization claims. Store allocates those IDs.
type GraphStageAdmissionFacts struct {
	Trigger           GraphFreshTrigger
	Control           GraphDispatchBinding
	NodeAdmissionID   string
	InputBinding      GraphInputBinding
	SourceBasis       GraphSourceBasis
	VerifiedArtifacts []VerifiedResponse
}

type GraphStepOutputFacts struct {
	CandidateClosure  *GraphOutputClosure
	ResponseClosure   *GraphOutputClosure
	FinalVerification *GraphFinalVerificationBinding
	Observation       GraphOutputObservation
	VerifiedArtifacts []VerifiedResponse
}
