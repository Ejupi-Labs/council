package domain

const (
	WorkflowContract  = "006-v1"
	ReviewContract    = "005-review-v1"
	MaxWorkflowChecks = 16
	MaxReviewFindings = 64
	MaxReviewBytes    = 1 << 20
	MaxWorkflowBytes  = 64 << 10
)

// Pointer members preserve omission independently of an explicit empty/zero value.
type WorkflowInput struct {
	Mode                string                 `json:"mode,omitzero"`
	Delivery            *WorkflowDeliveryInput `json:"delivery,omitzero"`
	Checks              *[]CheckDeclaration    `json:"checks,omitzero"`
	Review              string                 `json:"review,omitzero"`
	MaxCorrectionCycles *int                   `json:"maxCorrectionCycles,omitzero"`
}
type WorkflowDeliveryInput struct {
	LocalMode      string               `json:"localMode,omitzero"`
	ExternalAction *ExternalActionInput `json:"externalAction,omitzero"`
}
type ExternalCommandInput struct {
	Executable     string   `json:"executable"`
	Arguments      []string `json:"arguments"`
	TimeoutSeconds *int     `json:"timeoutSeconds,omitzero"`
}
type CommandDeclaration = ExternalCommandInput
type ExternalActionInput struct {
	Action          string               `json:"action"`
	Destination     string               `json:"destination"`
	AcceptanceBasis string               `json:"acceptanceBasis"`
	Command         ExternalCommandInput `json:"command"`
}
type DeliveryPreference struct {
	LocalMode       string `json:"localMode"`
	SelectionSource string `json:"selectionSource"`
	Revision        string `json:"revision"`
	UpdatedAt       string `json:"updatedAt"`
}
type DeliveryPreferenceInput struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision string `json:"expectedRevision"`
	LocalMode        string `json:"localMode"`
}
type WorkflowAdoptInput struct {
	RequestID        string         `json:"requestId"`
	ExpectedRevision string         `json:"expectedRevision"`
	TaskVersion      string         `json:"taskVersion"`
	Workflow         *WorkflowInput `json:"workflow"`
}
type WorkflowAdvanceInput struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision string `json:"expectedRevision"`
	WorkflowRevision string `json:"workflowRevision"`
	TaskVersion      string `json:"taskVersion"`
}
type WorkflowCheckInput struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision string `json:"expectedRevision"`
	WorkflowRevision string `json:"workflowRevision"`
	TaskVersion      string `json:"taskVersion"`
	CheckID          string `json:"checkId"`
}
type WorkflowSelector struct {
	WorkflowID  string `json:"workflowId,omitzero"`
	TaskVersion string `json:"taskVersion,omitzero"`
}
type CheckScope struct {
	Kind  string   `json:"kind"`
	Paths []string `json:"paths"`
}
type CheckDeclaration struct {
	Key                      string      `json:"key"`
	Executable               string      `json:"executable"`
	Arguments                []string    `json:"arguments"`
	RelativeWorkingDirectory string      `json:"relativeWorkingDirectory,omitzero"`
	Scope                    *CheckScope `json:"scope,omitzero"`
	CoveredConditionKeys     []string    `json:"coveredConditionKeys,omitzero"`
	TimeoutSeconds           *int        `json:"timeoutSeconds,omitzero"`
	SuccessExitCodes         *[]int      `json:"successExitCodes,omitzero"`
	Required                 *bool       `json:"required,omitzero"`
}
type CommandDefinition struct {
	Executable               string   `json:"executable"`
	Arguments                []string `json:"arguments"`
	RelativeWorkingDirectory string   `json:"relativeWorkingDirectory"`
	TimeoutSeconds           int      `json:"timeoutSeconds"`
	SuccessExitCodes         []int    `json:"successExitCodes"`
	PhysicalIdentity         string   `json:"physicalIdentity"`
}
type EvidenceReference struct {
	Ref          string `json:"ref"`
	ArtifactID   string `json:"artifactId,omitzero"`
	SHA256       string `json:"sha256,omitzero"`
	RelativePath string `json:"relativePath,omitzero"`
}
type CheckDefinition struct {
	ID                   string              `json:"id"`
	Key                  string              `json:"key"`
	Kind                 string              `json:"kind"`
	Source               string              `json:"source"`
	RecognizerID         string              `json:"recognizerId"`
	BuiltinID            string              `json:"builtinId,omitzero"`
	Command              *CommandDefinition  `json:"command,omitzero"`
	Scope                CheckScope          `json:"scope"`
	Required             bool                `json:"required"`
	CoveredConditionKeys []string            `json:"coveredConditionKeys"`
	DefinitionSHA256     string              `json:"definitionSha256"`
	EvidenceRefs         []EvidenceReference `json:"evidenceRefs"`
}
type ConditionDefinition struct {
	Key          string   `json:"key"`
	Kind         string   `json:"kind"`
	Source       string   `json:"source"`
	EvidenceRefs []string `json:"evidenceRefs"`
}
type ConditionRegistry []ConditionDefinition
type CandidateBinding struct {
	RootSourceSnapshotID string `json:"rootSourceSnapshotId"`
	CandidateID          string `json:"candidateId"`
	SourceSnapshotID     string `json:"sourceSnapshotId"`
	ManifestArtifactID   string `json:"manifestArtifactId"`
	ManifestSHA256       string `json:"manifestSha256"`
	IndexArtifactID      string `json:"indexArtifactId"`
	IndexSHA256          string `json:"indexSha256"`
	ConditionsSHA256     string `json:"conditionsSha256"`
}
type ResponseBinding struct {
	ProducerAttemptID  string `json:"producerAttemptId"`
	ResponseArtifactID string `json:"responseArtifactId"`
	ResponseSHA256     string `json:"responseSha256"`
	ResponseByteLength string `json:"responseByteLength"`
	SourceSnapshotID   string `json:"sourceSnapshotId"`
	ManifestSHA256     string `json:"manifestSha256"`
	IndexArtifactID    string `json:"indexArtifactId"`
	IndexSHA256        string `json:"indexSha256"`
	ConditionsSHA256   string `json:"conditionsSha256"`
}
type Applicability struct {
	ScopeKind        string   `json:"scopeKind"`
	Paths            []string `json:"paths"`
	ConditionKeys    []string `json:"conditionKeys"`
	DefinitionSHA256 string   `json:"definitionSha256"`
	BasisSHA256      string   `json:"basisSha256"`
}
type DeliveryScope struct {
	LocalMode            string               `json:"localMode"`
	PreferenceRevision   string               `json:"preferenceRevision,omitzero"`
	Source               string               `json:"source"`
	ProjectID            string               `json:"projectId"`
	ProjectIdentity      string               `json:"projectIdentity"`
	RepositoryIdentity   string               `json:"repositoryIdentity"`
	SelectedRelativePath string               `json:"selectedRelativePath"`
	Branch               *string              `json:"branch"`
	Destination          string               `json:"destination"`
	ExternalAction       *ExternalActionInput `json:"externalAction,omitzero"`
}
type CorrectionPolicy struct {
	MaxCycles int      `json:"maxCycles"`
	Triggers  []string `json:"triggers"`
}
type WorkflowGap struct {
	Key     string `json:"key"`
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type WorkflowAuthority struct {
	CheckGaps          []WorkflowGap     `json:"checkGaps,omitzero"`
	ExecutionMode      string            `json:"executionMode"`
	FakeScenario       string            `json:"fakeScenario,omitzero"`
	ID                 string            `json:"id"`
	TaskID             string            `json:"taskId"`
	TaskVersion        string            `json:"taskVersion"`
	CapturedAt         string            `json:"capturedAt"`
	CreatedByRequestID string            `json:"createdByRequestId"`
	InstructionSHA256  string            `json:"instructionSha256"`
	ConditionsSHA256   string            `json:"conditionsSha256"`
	RoutingAuthorityID string            `json:"routingAuthorityId"`
	OptionSnapshot     WorkflowInput     `json:"optionSnapshot"`
	Conditions         ConditionRegistry `json:"conditions"`
	Delivery           DeliveryScope     `json:"delivery"`
	Checks             []CheckDefinition `json:"checks"`
	Review             string            `json:"review"`
	ReasonCodes        []string          `json:"reasonCodes"`
	Correction         CorrectionPolicy  `json:"correction"`
}
type EvidenceUse struct {
	Kind               string        `json:"kind"`
	EvidenceID         string        `json:"evidenceId"`
	CandidateID        string        `json:"candidateId,omitzero"`
	ResponseArtifactID string        `json:"responseArtifactId,omitzero"`
	Applicability      Applicability `json:"applicability"`
}
type WorkflowCompletion struct {
	Kind                 string          `json:"kind"`
	ResponseAcceptanceID string          `json:"responseAcceptanceId"`
	ResponseBinding      ResponseBinding `json:"responseBinding"`
	ReviewUses           []EvidenceUse   `json:"reviewUses"`
	CheckUses            []EvidenceUse   `json:"checkUses"`
	FindingResolutionIDs []string        `json:"findingResolutionIds"`
	ExternalResultRefs   []string        `json:"externalResultRefs"`
}
type Workflow struct {
	EvidenceUses            []EvidenceUse       `json:"evidenceUses"`
	HasTaskDelta            bool                `json:"hasTaskDelta"`
	HasTaskIndexDelta       bool                `json:"hasTaskIndexDelta"`
	ID                      string              `json:"id"`
	TaskID                  string              `json:"taskId"`
	TaskVersion             string              `json:"taskVersion"`
	Authority               WorkflowAuthority   `json:"authority"`
	Revision                string              `json:"revision"`
	StateRevision           string              `json:"stateRevision"`
	State                   string              `json:"state"`
	SelectedCandidateID     *string             `json:"selectedCandidateId"`
	SelectedResponseBinding *ResponseBinding    `json:"selectedResponseBinding,omitzero"`
	CurrentStepID           *string             `json:"currentStepId"`
	NextAction              string              `json:"nextAction"`
	NextOwner               string              `json:"nextOwner"`
	CorrectionCyclesUsed    int                 `json:"correctionCyclesUsed"`
	LatestAcceptanceID      *string             `json:"latestAcceptanceId"`
	Completion              *WorkflowCompletion `json:"completion,omitzero"`
	CreatedAt               string              `json:"createdAt"`
	UpdatedAt               string              `json:"updatedAt"`
}
type WorkflowStep struct {
	ReviewID              *string                 `json:"reviewId,omitzero"`
	CheckID               *string                 `json:"checkId,omitzero"`
	ExternalResult        *ExternalDeliveryRecord `json:"externalResult,omitzero"`
	ID                    string                  `json:"id"`
	WorkflowID            string                  `json:"workflowId"`
	Ordinal               string                  `json:"ordinal"`
	ExecutionOrdinal      string                  `json:"executionOrdinal"`
	ReplacementOfStepID   *string                 `json:"replacementOfStepId"`
	Role                  string                  `json:"role"`
	ExecutorKind          string                  `json:"executorKind"`
	Cycle                 int                     `json:"cycle"`
	LogicalKey            string                  `json:"logicalKey"`
	ReasonCodes           []string                `json:"reasonCodes"`
	DefinitionID          *string                 `json:"definitionId"`
	InputCandidateID      *string                 `json:"inputCandidateId"`
	OutputCandidateID     *string                 `json:"outputCandidateId"`
	InputResponseBinding  *ResponseBinding        `json:"inputResponseBinding,omitzero"`
	OutputResponseBinding *ResponseBinding        `json:"outputResponseBinding,omitzero"`
	PredecessorStepID     *string                 `json:"predecessorStepId"`
	AttemptID             *string                 `json:"attemptId"`
	Trigger               string                  `json:"trigger"`
	Status                string                  `json:"status"`
	CreatedAt             string                  `json:"createdAt"`
	StartedAt             *string                 `json:"startedAt"`
	SettledAt             *string                 `json:"settledAt"`
	AdvanceProof          *WorkflowAdvanceProof   `json:"advanceProof,omitzero"`
}
type WorkflowAdvanceProof struct {
	GraphBudgetAuthorityEpoch string           `json:"graphBudgetAuthorityEpoch,omitzero"`
	PredecessorStepID         string           `json:"predecessorStepId"`
	OwnerInstanceID           string           `json:"ownerInstanceId"`
	TaskRevision              string           `json:"taskRevision"`
	TaskVersion               string           `json:"taskVersion"`
	WorkflowRevision          string           `json:"workflowRevision"`
	BudgetRevision            string           `json:"budgetRevision"`
	WindowNumber              string           `json:"windowNumber"`
	CandidateID               *string          `json:"candidateId"`
	ResponseBinding           *ResponseBinding `json:"responseBinding,omitzero"`
	NextLogicalKey            string           `json:"nextLogicalKey"`
	Trigger                   string           `json:"trigger"`
}

// Native review JSON deliberately has no optional fields at any object depth.
type ReviewSubject struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}
type ReviewInspection struct {
	Ref   string   `json:"ref"`
	Scope string   `json:"scope"`
	Paths []string `json:"paths"`
}
type ReviewLocation struct {
	Path      string `json:"path"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}
type ReviewFinding struct {
	Key           string           `json:"key"`
	Severity      string           `json:"severity"`
	ConditionKeys []string         `json:"conditionKeys"`
	Locations     []ReviewLocation `json:"locations"`
	Summary       string           `json:"summary"`
	EvidenceRefs  []string         `json:"evidenceRefs"`
}
type ReviewGap struct {
	Key           string   `json:"key"`
	ConditionKeys []string `json:"conditionKeys"`
	Reason        string   `json:"reason"`
	EvidenceRefs  []string `json:"evidenceRefs"`
}
type ReviewReport struct {
	SchemaVersion      string             `json:"schemaVersion"`
	Subject            ReviewSubject      `json:"subject"`
	ConditionsSHA256   string             `json:"conditionsSHA256"`
	Outcome            string             `json:"outcome"`
	InspectedEvidence  []ReviewInspection `json:"inspectedEvidence"`
	Findings           []ReviewFinding    `json:"findings"`
	Gaps               []ReviewGap        `json:"gaps"`
	ResolvedFindingIDs []string           `json:"resolvedFindingIds"`
}
type WorkflowFinding struct {
	Resolutions []FindingResolution `json:"resolutions,omitzero"`
	ID          string              `json:"id"`
	ModelKey    string              `json:"modelKey"`
	ReviewID    string              `json:"reviewId,omitzero"`
	CheckID     string              `json:"checkId,omitzero"`
	Finding     ReviewFinding       `json:"finding"`
}
type ReviewRecord struct {
	SuppliedEvidence []ReviewInspection `json:"suppliedEvidence"`
	ID               string             `json:"id"`
	WorkflowID       string             `json:"workflowId"`
	StepID           string             `json:"stepId"`
	AttemptID        string             `json:"attemptId"`
	CandidateBinding *CandidateBinding  `json:"candidateBinding,omitzero"`
	ResponseBinding  *ResponseBinding   `json:"responseBinding,omitzero"`
	ReviewContract   string             `json:"reviewContract"`
	ResultArtifactID string             `json:"resultArtifactId"`
	ResultSHA256     string             `json:"resultSha256"`
	Applicability    Applicability      `json:"applicability"`
	Report           ReviewReport       `json:"report"`
	Findings         []WorkflowFinding  `json:"findings"`
	Outcome          string             `json:"outcome"`
	CreatedAt        string             `json:"createdAt"`
}
type FindingResolution struct {
	ID                       string           `json:"id"`
	WorkflowID               string           `json:"workflowId"`
	FindingID                string           `json:"findingId"`
	OriginReviewID           *string          `json:"originReviewId"`
	OriginCheckID            *string          `json:"originCheckId"`
	CorrectedCandidateID     *string          `json:"correctedCandidateId"`
	CorrectedResponseBinding *ResponseBinding `json:"correctedResponseBinding,omitzero"`
	CorrectionStepID         string           `json:"correctionStepId"`
	QualifyingReviewIDs      []string         `json:"qualifyingReviewIds"`
	QualifyingCheckIDs       []string         `json:"qualifyingCheckIds"`
	EvidenceUses             []EvidenceUse    `json:"evidenceUses"`
	CreatedAt                string           `json:"createdAt"`
}
type CheckReceipt struct {
	Finding               *WorkflowFinding   `json:"finding,omitzero"`
	ID                    string             `json:"id"`
	WorkflowID            string             `json:"workflowId"`
	StepID                string             `json:"stepId"`
	AttemptID             string             `json:"attemptId"`
	DefinitionID          string             `json:"definitionId"`
	DefinitionSHA256      string             `json:"definitionSha256"`
	InputCandidateBinding *CandidateBinding  `json:"inputCandidateBinding,omitzero"`
	InputResponseBinding  *ResponseBinding   `json:"inputResponseBinding,omitzero"`
	Applicability         Applicability      `json:"applicability"`
	ObservedExecutable    *CommandDefinition `json:"observedExecutable,omitzero"`
	StartedAt             *string            `json:"startedAt"`
	FinishedAt            *string            `json:"finishedAt"`
	Outcome               string             `json:"outcome"`
	ExitCode              *int               `json:"exitCode"`
	StdoutArtifactID      *string            `json:"stdoutArtifactId"`
	StderrArtifactID      *string            `json:"stderrArtifactId"`
	RootTerminal          bool               `json:"rootTerminal"`
	InputComplete         bool               `json:"inputComplete"`
	OutputComplete        bool               `json:"outputComplete"`
	BuiltinCompleted      bool               `json:"builtinCompleted"`
	Cleanup               Cleanup            `json:"cleanup"`
	ResultingCandidateID  *string            `json:"resultingCandidateId"`
	SourceChanged         bool               `json:"sourceChanged"`
	CreatedAt             string             `json:"createdAt"`
}
type DestinationEvidence struct {
	ProjectIdentity    string  `json:"projectIdentity"`
	RepositoryIdentity string  `json:"repositoryIdentity"`
	Branch             *string `json:"branch"`
	BaseCommit         string  `json:"baseCommit"`
	ManifestArtifactID string  `json:"manifestArtifactId"`
	ManifestSHA256     string  `json:"manifestSha256"`
}
type DeliveryConflict struct {
	RelativePath       string  `json:"relativePath"`
	Code               string  `json:"code"`
	EvidenceArtifactID *string `json:"evidenceArtifactId"`
}
type DeliveryOperation struct {
	GraphDeliveryRef   *GraphArtifactRef    `json:"graphDeliveryRef,omitzero"`
	PlanArtifactID     string               `json:"planArtifactId,omitzero"`
	ID                 string               `json:"id"`
	WorkflowID         string               `json:"workflowId"`
	StepID             string               `json:"stepId"`
	AttemptID          string               `json:"attemptId"`
	RequestID          string               `json:"requestId"`
	CandidateBinding   CandidateBinding     `json:"candidateBinding"`
	EvidenceUses       []EvidenceUse        `json:"evidenceUses"`
	Scope              DeliveryScope        `json:"scope"`
	AuthoritySHA256    string               `json:"authoritySha256"`
	Status             string               `json:"status"`
	BeforeDestination  *DestinationEvidence `json:"beforeDestination"`
	AfterDestination   *DestinationEvidence `json:"afterDestination"`
	CreatedAt          string               `json:"createdAt"`
	FinishedAt         *string              `json:"finishedAt"`
	LastEffectSequence string               `json:"lastEffectSequence"`
	Conflicts          []DeliveryConflict   `json:"conflicts"`
	ConflictCount      string               `json:"conflictCount"`
	IncidentID         *string              `json:"incidentId"`
	NextAction         string               `json:"nextAction"`
}
type DeliveryEffect struct {
	ObservedIdentity      string             `json:"observedIdentity,omitzero"`
	NamespaceDurability   string             `json:"namespaceDurability,omitzero"`
	OperationID           string             `json:"operationId"`
	Sequence              string             `json:"sequence"`
	EffectID              string             `json:"effectId"`
	Kind                  string             `json:"kind"`
	RelativePath          string             `json:"relativePath"`
	Action                string             `json:"action"`
	State                 string             `json:"state"`
	BeforeEvidence        *EvidenceReference `json:"beforeEvidence"`
	IntendedAfterEvidence *EvidenceReference `json:"intendedAfterEvidence"`
	ObservedAfterEvidence *EvidenceReference `json:"observedAfterEvidence"`
	ObservedAt            string             `json:"observedAt"`
	IncidentID            *string            `json:"incidentId"`
}
type ExternalDeliveryRecord struct {
	WorkflowID       string              `json:"workflowId"`
	StepID           string              `json:"stepId"`
	AttemptID        string              `json:"attemptId"`
	Action           ExternalActionInput `json:"action"`
	Command          CommandDefinition   `json:"command"`
	Outcome          string              `json:"outcome"`
	StdoutArtifactID *string             `json:"stdoutArtifactId"`
	StderrArtifactID *string             `json:"stderrArtifactId"`
	CreatedAt        string              `json:"createdAt"`
}
type WorkflowDeliveryView struct {
	ID         string                  `json:"id"`
	Kind       string                  `json:"kind"`
	StepID     *string                 `json:"stepId"`
	AttemptID  *string                 `json:"attemptId"`
	Operation  *DeliveryOperation      `json:"operation,omitzero"`
	External   *ExternalDeliveryRecord `json:"external,omitzero"`
	Acceptance *WorkflowAcceptance     `json:"acceptance,omitzero"`
}

type WorkflowAcceptance struct {
	ID                   string           `json:"id"`
	WorkflowID           string           `json:"workflowId"`
	TaskID               string           `json:"taskId"`
	TaskVersion          string           `json:"taskVersion"`
	CandidateBinding     CandidateBinding `json:"candidateBinding"`
	AuthoritySHA256      string           `json:"authoritySha256"`
	ReviewUses           []EvidenceUse    `json:"reviewUses"`
	CheckUses            []EvidenceUse    `json:"checkUses"`
	FindingResolutionIDs []string         `json:"findingResolutionIds"`
	DeliveryOperationID  *string          `json:"deliveryOperationId"`
	ExternalResultRefs   []string         `json:"externalResultRefs"`
	DeliveryResult       string           `json:"deliveryResult"`
	AcceptedAt           string           `json:"acceptedAt"`
	AcceptedByRequestID  string           `json:"acceptedByRequestId"`
}
type WorkflowDetail struct {
	Acceptance  *WorkflowAcceptance `json:"acceptance,omitzero"`
	TaskID      string              `json:"taskId"`
	TaskVersion string              `json:"taskVersion"`
	Workflow    *Workflow           `json:"workflow"`
}

// Controller facts below are never decoded from public requests.
type WorkflowCapture struct {
	Authority WorkflowAuthority
	Step      WorkflowStep
}
type WorkflowAdmissionFacts struct {
	Capture *WorkflowCapture
	Routing RoutingFacts
}
type WorkflowStepAdmission struct {
	GraphBudgetRef  *GraphBudgetRef  `json:"graphBudgetRef,omitzero"`
	Workflow        Workflow         `json:"workflow"`
	Step            WorkflowStep     `json:"step"`
	Attempt         Attempt          `json:"attempt"`
	Budget          TaskBudget       `json:"budget"`
	RoutingDecision *RoutingDecision `json:"routingDecision,omitzero"`
}
type WorkflowStagePlan struct {
	Role                 string
	ExecutorKind         string
	LogicalKey           string
	Cycle                int
	DefinitionID         *string
	InputCandidateID     *string
	InputResponseBinding *ResponseBinding
	PredecessorStepID    *string
	ReplacementOfStepID  *string
	ReasonCodes          []string
}

type WorkflowStepAdmissionFacts struct {
	Graph                    *GraphStageAdmissionFacts
	Plan                     WorkflowStagePlan
	ExpectedTaskRevision     string
	ExpectedWorkflowRevision string
	Routing                  RoutingFacts
}

type WorkflowAcceptanceFacts struct {
	AutomaticPredecessorStepID string
	VerifiedArtifacts          []VerifiedResponse
	RequestID                  string
	ExpectedTaskRevision       string
	ExpectedWorkflowRevision   string
	OwnerInstanceID            string
	Acceptance                 WorkflowAcceptance
}

type WorkflowStepFacts struct {
	GraphOutput            *GraphStepOutputFacts
	ResultCandidateBinding *CandidateBinding
	ReviewAllowedPaths     []string
	ReuseEvidenceUses      []EvidenceUse
	HasTaskDelta           bool
	HasTaskIndexDelta      bool
	DirectProven           bool
	CumulativeDirectProven bool
	VerifiedArtifacts      []VerifiedResponse
	Operation              *DeliveryOperation
	StepID                 string
	OwnerInstanceID        string
	TaskRevision           string
	WorkflowRevision       string
	CandidateBinding       *CandidateBinding
	ResponseBinding        *ResponseBinding
	Applicability          *Applicability
	Report                 *ReviewRecord
	Check                  *CheckReceipt
	External               *ExternalDeliveryRecord
	Settlement             Settlement
}
type DeliveryPermitFacts struct {
	Graph            *GraphDeliveryFacts
	OperationID      string
	OwnerInstanceID  string
	TaskRevision     string
	WorkflowRevision string
	Effect           DeliveryEffect
}
