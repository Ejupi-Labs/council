package domain

import "encoding/json/jsontext"

const ExecutionContract = "005-v1"
const MaxPromptBytes = 256 * 1024
const MaxEventBytes = 16 * 1024

type DraftRef struct {
	DraftID  string `json:"draftId"`
	Revision string `json:"revision"`
}
type ExecutionPins struct {
	HarnessID   string `json:"harnessId,omitempty"`
	Model       string `json:"model,omitempty"`
	AccountID   string `json:"accountId,omitempty"`
	PaymentPath string `json:"paymentPath,omitempty"`
}
type AcceptanceInput struct {
	Deliverable               string   `json:"deliverable,omitempty"`
	ConditionsText            string   `json:"conditionsText,omitempty"`
	RequiredEvidence          []string `json:"requiredEvidence,omitempty"`
	AllowCandidatePreparation *bool    `json:"allowCandidatePreparation,omitempty"`
}
type TaskAcceptance struct {
	Deliverable               string   `json:"deliverable"`
	ConditionsText            string   `json:"conditionsText"`
	RequiredEvidence          []string `json:"requiredEvidence"`
	AllowCandidatePreparation bool     `json:"allowCandidatePreparation"`
}
type StartTaskInput struct {
	Workflow               *WorkflowInput   `json:"workflow,omitzero"`
	Routing                *RoutingOptions  `json:"routing,omitempty"`
	RequestID              string           `json:"requestId"`
	ProjectID              string           `json:"projectId"`
	OriginalText           string           `json:"originalText"`
	SourceDraftRef         *DraftRef        `json:"sourceDraftRef,omitempty"`
	ExpectedPolicyRevision string           `json:"expectedPolicyRevision,omitempty"`
	Pins                   *ExecutionPins   `json:"pins,omitempty"`
	PolicyOverride         string           `json:"policyOverride,omitempty"`
	Acceptance             *AcceptanceInput `json:"acceptance,omitempty"`
	TimeoutSeconds         *int             `json:"timeoutSeconds,omitempty"`
	ExecutionMode          string           `json:"executionMode,omitempty"`
	FakeScenario           string           `json:"fakeScenario,omitempty"`
}
type GuidanceInput struct {
	Workflow            *WorkflowInput   `json:"workflow,omitzero"`
	Routing             *RoutingOptions  `json:"routing,omitempty"`
	RequestID           string           `json:"requestId"`
	ExpectedRevision    string           `json:"expectedRevision"`
	GuidanceText        string           `json:"guidanceText"`
	UseCurrentPolicy    bool             `json:"useCurrentPolicy,omitempty"`
	PolicyOverride      string           `json:"policyOverride,omitempty"`
	ClearPolicyOverride bool             `json:"clearPolicyOverride,omitempty"`
	Pins                *ExecutionPins   `json:"pins,omitempty"`
	Acceptance          *AcceptanceInput `json:"acceptance,omitempty"`
	TimeoutSeconds      *int             `json:"timeoutSeconds,omitempty"`
}
type StartAttemptInput struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision string `json:"expectedRevision"`
	TaskVersion      string `json:"taskVersion"`
	ExecutionMode    string `json:"executionMode,omitempty"`
	FakeScenario     string `json:"fakeScenario,omitempty"`
}
type ResponseAcceptanceInput struct {
	RequestID          string `json:"requestId"`
	ExpectedRevision   string `json:"expectedRevision"`
	TaskVersion        string `json:"taskVersion"`
	AttemptID          string `json:"attemptId"`
	ResponseArtifactID string `json:"responseArtifactId"`
	ResponseSHA256     string `json:"responseSha256"`
	Confirmation       string `json:"confirmation"`
}
type Task struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"projectId"`
	OriginalText    string    `json:"originalText"`
	SourceDraftRef  *DraftRef `json:"sourceDraftRef"`
	CurrentVersion  string    `json:"currentVersion"`
	Revision        string    `json:"revision"`
	StateRevision   string    `json:"stateRevision"`
	LatestAttemptID *string   `json:"latestAttemptId"`
	CreatedAt       string    `json:"createdAt"`
	UpdatedAt       string    `json:"updatedAt"`
}
type TaskVersion struct {
	TaskID             string         `json:"taskId"`
	Version            string         `json:"version"`
	CreatedAt          string         `json:"createdAt"`
	CreatedBy          string         `json:"createdBy"`
	GuidanceText       string         `json:"guidanceText"`
	Acceptance         TaskAcceptance `json:"acceptance"`
	RequestedPolicy    Policy         `json:"requestedPolicy"`
	TaskPolicyOverride *string        `json:"taskPolicyOverride"`
	Pins               ExecutionPins  `json:"pins"`
	TimeoutSeconds     int            `json:"timeoutSeconds"`
}
type TaskSummary struct {
	ID              string  `json:"id"`
	ProjectID       string  `json:"projectId"`
	CurrentVersion  string  `json:"currentVersion"`
	Revision        string  `json:"revision"`
	StateRevision   string  `json:"stateRevision"`
	LatestAttemptID *string `json:"latestAttemptId"`
	RequestedMode   string  `json:"requestedMode"`
	PolicySource    string  `json:"policySource"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	Status          string  `json:"status"`
	Stage           string  `json:"stage"`
	NextAction      string  `json:"nextAction"`
}
type TaskDetail struct {
	Task          Task                `json:"task"`
	Version       TaskVersion         `json:"version"`
	LatestAttempt *Attempt            `json:"latestAttempt"`
	Acceptance    *ResponseAcceptance `json:"acceptance"`
}
type TaskVersionDetail struct {
	TaskID       string      `json:"taskId"`
	OriginalText string      `json:"originalText"`
	Version      TaskVersion `json:"version"`
}
type Admission struct {
	Workflow        *Workflow        `json:"workflow,omitzero"`
	Step            *WorkflowStep    `json:"step,omitempty"`
	RoutingDecision *RoutingDecision `json:"routingDecision,omitempty"`
	Budget          *TaskBudget      `json:"budget,omitempty"`
	Task            TaskSummary      `json:"task"`
	Version         TaskVersion      `json:"version"`
	Attempt         Attempt          `json:"attempt"`
}
type GuidanceResult struct {
	Task    TaskSummary `json:"task"`
	Version TaskVersion `json:"version"`
}
type ObservedFact struct {
	Status     string  `json:"status"`
	Value      *string `json:"value"`
	Source     *string `json:"source"`
	ObservedAt *string `json:"observedAt"`
}
type UsageObservation struct {
	InputTokens              *string `json:"inputTokens"`
	OutputTokens             *string `json:"outputTokens"`
	CachedInputTokens        *string `json:"cachedInputTokens"`
	CacheCreationInputTokens *string `json:"cacheCreationInputTokens,omitempty"`
	Scope                    string  `json:"scope,omitempty"`
	Source                   *string `json:"source"`
	ObservedAt               *string `json:"observedAt"`
	Completeness             string  `json:"completeness"`
}

// NativeModelUsage is a preview of one native-reported whole-tree model entry.
// CostEstimateUSD is the exact native JSON-number lexeme, never a billed charge.
type NativeModelUsage struct {
	Model                    string  `json:"model"`
	InputTokens              *string `json:"inputTokens"`
	OutputTokens             *string `json:"outputTokens"`
	CachedInputTokens        *string `json:"cachedInputTokens"`
	CacheCreationInputTokens *string `json:"cacheCreationInputTokens"`
	CostEstimateUSD          *string `json:"costEstimateUsd"`
}

type NativeModelsObservation struct {
	Scope               string             `json:"scope"`
	Source              string             `json:"source"`
	ObservedAt          string             `json:"observedAt"`
	Completeness        string             `json:"completeness"`
	EvidenceArtifactIDs []string           `json:"evidenceArtifactIds"`
	OmittedModelCount   *string            `json:"omittedModelCount"`
	Models              []NativeModelUsage `json:"models"`
}
type PermissionObservation struct {
	RequestedMode      string   `json:"requestedMode"`
	ConfiguredMode     *string  `json:"configuredMode"`
	NativeReportedMode *string  `json:"nativeReportedMode"`
	Limitations        []string `json:"limitations"`
}
type TargetObservations struct {
	Provider        ObservedFact             `json:"provider"`
	Account         ObservedFact             `json:"account"`
	Model           ObservedFact             `json:"model"`
	NativeSession   ObservedFact             `json:"nativeSession"`
	Permissions     PermissionObservation    `json:"permissions"`
	Usage           UsageObservation         `json:"usage"`
	ConfiguredModel *ObservedFact            `json:"configuredModel,omitempty"`
	NativeVersion   *ObservedFact            `json:"nativeVersion,omitempty"`
	NativeModels    *NativeModelsObservation `json:"nativeModels,omitempty"`
}
type Cleanup struct {
	State      string  `json:"state"`
	Scope      string  `json:"scope"`
	StartedAt  *string `json:"startedAt"`
	FinishedAt *string `json:"finishedAt"`
	IncidentID *string `json:"incidentId"`
}
type AcceptedStop struct {
	RequestID     string `json:"requestId"`
	AcceptedAt    string `json:"acceptedAt"`
	EventSequence string `json:"eventSequence"`
}
type PrimaryFailure struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	ObservedAt    string `json:"observedAt"`
	EventSequence string `json:"eventSequence"`
}
type Attempt struct {
	WorkflowID         string             `json:"workflowId,omitempty"`
	StepID             string             `json:"stepId,omitempty"`
	Role               string             `json:"role,omitempty"`
	ExecutorKind       string             `json:"executorKind,omitempty"`
	InputCandidateID   *string            `json:"inputCandidateId,omitempty"`
	Trigger            string             `json:"trigger,omitempty"`
	RoutingDecisionID  *string            `json:"routingDecisionId,omitempty"`
	ResolvedTarget     *ResolvedTarget    `json:"resolvedTarget,omitempty"`
	ID                 string             `json:"id"`
	TaskID             string             `json:"taskId"`
	TaskVersion        string             `json:"taskVersion"`
	PreviousAttemptID  *string            `json:"previousAttemptId"`
	StartRequestID     string             `json:"startRequestId"`
	OwnerInstanceID    string             `json:"ownerInstanceId"`
	ConnectionID       string             `json:"connectionId"`
	ExecutionMode      string             `json:"executionMode"`
	FakeScenario       string             `json:"fakeScenario,omitempty"`
	Phase              string             `json:"phase"`
	Outcome            *string            `json:"outcome"`
	Revision           string             `json:"revision"`
	AdmittedAt         string             `json:"admittedAt"`
	DeadlineAt         string             `json:"deadlineAt"`
	StartedAt          *string            `json:"startedAt"`
	SettledAt          *string            `json:"settledAt"`
	ObservedAt         string             `json:"observedAt"`
	RemainingSeconds   int                `json:"remainingSeconds"`
	RequestedTarget    ExecutionPins      `json:"requestedTarget"`
	TargetObservations TargetObservations `json:"targetObservations"`
	SourceSnapshotID   *string            `json:"sourceSnapshotId"`
	CandidateID        *string            `json:"candidateId"`
	ResponseArtifactID *string            `json:"responseArtifactId"`
	Stop               *AcceptedStop      `json:"stop"`
	PrimaryFailure     *PrimaryFailure    `json:"primaryFailure"`
	Cleanup            Cleanup            `json:"cleanup"`
	EvidenceStatus     string             `json:"evidenceStatus"`
	LaunchIntent       bool               `json:"launchIntent"`
	SourceSnapshot     *SourceSnapshot    `json:"sourceSnapshot,omitempty"`
	Candidate          *Candidate         `json:"candidate,omitempty"`
	Incident           *ExecutionIncident `json:"incident,omitempty"`
	AuthoredDelta      bool               `json:"authoredDelta"`
}
type AttemptSummary = Attempt
type AttemptEvent struct {
	Sequence   string         `json:"sequence"`
	AttemptID  string         `json:"attemptId"`
	Kind       string         `json:"kind"`
	ObservedAt string         `json:"observedAt"`
	Metadata   jsontext.Value `json:"metadata"`
}
type ArtifactMetadata struct {
	ID           string  `json:"id"`
	AttemptID    string  `json:"attemptId"`
	Kind         string  `json:"kind"`
	ByteLength   string  `json:"byteLength"`
	Closed       bool    `json:"closed"`
	Completeness string  `json:"completeness"`
	SHA256       *string `json:"sha256"`
	CreatedAt    string  `json:"createdAt"`
}
type ArtifactRecord struct {
	ArtifactMetadata
	PrivateRef       string `json:"-"`
	PhysicalIdentity string `json:"-"`
}
type SourceSnapshot struct {
	ID                   string  `json:"id"`
	AttemptID            string  `json:"attemptId"`
	ManifestArtifactID   string  `json:"manifestArtifactId"`
	IndexArtifactID      string  `json:"indexArtifactId"`
	ManifestSHA256       string  `json:"manifestSha256"`
	BaseCommit           string  `json:"baseCommit"`
	Branch               *string `json:"branch"`
	ProjectIdentity      string  `json:"projectIdentity"`
	SelectedRelativePath string  `json:"selectedRelativePath"`
	CreatedAt            string  `json:"createdAt"`
}
type Candidate struct {
	RootSourceSnapshotID   string  `json:"rootSourceSnapshotId,omitempty"`
	IndexArtifactID        string  `json:"indexArtifactId,omitempty"`
	WorkflowID             string  `json:"workflowId,omitempty"`
	PredecessorCandidateID *string `json:"predecessorCandidateId,omitempty"`
	CreatedByStepID        string  `json:"createdByStepId,omitempty"`
	ConditionsSHA256       string  `json:"conditionsSha256,omitempty"`
	Completeness           string  `json:"completeness,omitempty"`
	ID                     string  `json:"id"`
	AttemptID              string  `json:"attemptId"`
	SourceSnapshotID       string  `json:"sourceSnapshotId"`
	ManifestArtifactID     string  `json:"manifestArtifactId"`
	DeltaArtifactID        string  `json:"deltaArtifactId"`
	WorkspaceRef           string  `json:"workspaceRef"`
	HasDelta               bool    `json:"hasDelta"`
	HasIndexDelta          bool    `json:"hasIndexDelta"`
	Verification           string  `json:"verification"`
	Integration            string  `json:"integration"`
	CreatedAt              string  `json:"createdAt"`
}
type ResponseAcceptance struct {
	ID                 string `json:"id"`
	RequestID          string `json:"requestId"`
	TaskID             string `json:"taskId"`
	TaskVersion        string `json:"taskVersion"`
	AttemptID          string `json:"attemptId"`
	ResponseArtifactID string `json:"responseArtifactId"`
	ResponseSHA256     string `json:"responseSha256"`
	Basis              string `json:"basis"`
	Confirmation       string `json:"confirmation"`
	AcceptedAt         string `json:"acceptedAt"`
}
type ExecutionGate struct {
	State           string  `json:"state"`
	OwnerInstanceID string  `json:"ownerInstanceId"`
	IncidentID      *string `json:"incidentId"`
	ActiveAttemptID *string `json:"activeAttemptId"`
}
type ExecutionIncident struct {
	ID        string  `json:"id"`
	AttemptID *string `json:"attemptId"`
	Code      string  `json:"code"`
	Message   string  `json:"message"`
	CreatedAt string  `json:"createdAt"`
}

// AttemptUpdate carries controller observations, never a callback or external I/O.
type AttemptUpdate struct {
	Kind               string
	Metadata           jsontext.Value
	Phase              string
	StartedAt          *string
	SourceSnapshotID   *string
	CandidateID        *string
	ResponseArtifactID *string
	TargetObservations *TargetObservations
	EvidenceStatus     string
	Cleanup            *Cleanup
	Failure            *PrimaryFailure
}
type Settlement struct {
	BuiltinCompleted bool `json:"builtinCompleted,omitzero"`
	EffectsKnown     bool `json:"effectsKnown,omitzero"`
	NoProcessCreated bool
	Outcome          string
	Cleanup          Cleanup
	EvidenceStatus   string
	RootTerminal     bool
	InputComplete    bool
	ExitCode         *int
	UsableOutput     bool
	Failure          *PrimaryFailure
}

// VerifiedResponse is a controller proof from a fresh closed-file identity, length and SHA256 verification.
// It is never decoded from a public request.
type VerifiedResponse struct {
	ArtifactID       string
	SHA256           string
	PhysicalIdentity string
	ByteLength       string
}
