package domain

// Routing DTOs deliberately depend only on domain. Pure policy and store code
// share these detached values without callback or filesystem capabilities.
const RoutingContract = "004-v1"
const MaxRoutingDecisionBytes = 64 << 10
const MaxUsagePageBytes = 4 << 20
const MaxDeclaredAccounts = 64

type RoutingBudgetInput struct {
	AdmissionLimit string `json:"admissionLimit,omitempty"`
	WindowSeconds  *int   `json:"windowSeconds,omitempty"`
}
type RoutingBudgetDefaults struct {
	AdmissionLimit string `json:"admissionLimit"`
	WindowSeconds  int    `json:"windowSeconds"`
}
type ContinuationInput struct {
	MaxAutomaticAdmissions string    `json:"maxAutomaticAdmissions,omitempty"`
	Categories             *[]string `json:"categories,omitzero"`
	AllowReplacement       *bool     `json:"allowReplacement,omitzero"`
	AllowEscalation        *bool     `json:"allowEscalation,omitzero"`
}
type ContinuationPolicy struct {
	MaxAutomaticAdmissions string   `json:"maxAutomaticAdmissions"`
	Categories             []string `json:"categories"`
	AllowReplacement       bool     `json:"allowReplacement"`
	AllowEscalation        bool     `json:"allowEscalation"`
}
type RoutingOptions struct {
	Suitability            string              `json:"suitability,omitempty"`
	TargetProfileID        string              `json:"targetProfileId,omitempty"`
	ExpectedPolicyRevision string              `json:"expectedPolicyRevision,omitempty"`
	Budget                 *RoutingBudgetInput `json:"budget,omitempty"`
	Continuation           *ContinuationInput  `json:"continuation,omitempty"`
	UseCurrentPolicy       *bool               `json:"useCurrentPolicy,omitzero"`
}
type RoutingPolicy struct {
	Revision            string                `json:"revision"`
	EconomyOrder        []string              `json:"economyOrder"`
	UnknownAvailability string                `json:"unknownAvailability"`
	DefaultBudget       RoutingBudgetDefaults `json:"defaultBudget"`
	Continuation        ContinuationPolicy    `json:"continuation"`
	UpdatedAt           string                `json:"updatedAt"`
}
type RoutingPolicyInput struct {
	RequestID           string              `json:"requestId"`
	ExpectedRevision    string              `json:"expectedRevision"`
	EconomyOrder        *[]string           `json:"economyOrder,omitzero"`
	UnknownAvailability string              `json:"unknownAvailability,omitempty"`
	DefaultBudget       *RoutingBudgetInput `json:"defaultBudget,omitempty"`
	Continuation        *ContinuationInput  `json:"continuation,omitempty"`
}
type NativeContext struct {
	ID           string                       `json:"id"`
	HarnessID    string                       `json:"harnessId"`
	ConnectionID string                       `json:"connectionId"`
	Label        string                       `json:"label"`
	Enabled      bool                         `json:"enabled"`
	Revision     string                       `json:"revision"`
	UpdatedAt    string                       `json:"updatedAt"`
	Capabilities NativeConnectionCapabilities `json:"capabilities"`
}
type NativeConnectionCapabilities struct {
	ConnectionID         string                 `json:"connectionId"`
	HarnessID            string                 `json:"harnessId"`
	Interface            string                 `json:"interface"`
	NativeContract       string                 `json:"nativeContract"`
	MinimumVersion       *string                `json:"minimumVersion"`
	DocumentedVersion    *string                `json:"documentedVersion"`
	VersionSource        string                 `json:"versionSource"`
	SupportedPolicyModes []string               `json:"supportedPolicyModes"`
	ModelSelection       string                 `json:"modelSelection"`
	AccountSelection     string                 `json:"accountSelection"`
	SessionContinuity    string                 `json:"sessionContinuity"`
	SessionPersistence   string                 `json:"sessionPersistence"`
	CancellationScope    string                 `json:"cancellationScope"`
	LiveQualification    string                 `json:"liveQualification"`
	FakeMode             bool                   `json:"fakeMode"`
	Limits               NativeConnectionLimits `json:"limits"`
}
type NativeConnectionLimits struct {
	DefaultTimeoutSeconds        int   `json:"defaultTimeoutSeconds"`
	MaxTimeoutSeconds            int   `json:"maxTimeoutSeconds"`
	PromptBytes                  int   `json:"promptBytes"`
	StdoutBytes                  int64 `json:"stdoutBytes"`
	StderrBytes                  int64 `json:"stderrBytes"`
	JSONLLineBytes               int   `json:"jsonlLineBytes"`
	EventBytes                   int   `json:"eventBytes"`
	ChunkBytes                   int   `json:"chunkBytes"`
	FinalResponseBytesUpperBound int64 `json:"finalResponseBytesUpperBound"`
}
type ProfileSource struct {
	Kind                 string  `json:"kind"`
	Label                string  `json:"label"`
	URL                  string  `json:"url"`
	ReadDate             string  `json:"readDate"`
	MinimumNativeVersion *string `json:"minimumNativeVersion"`
}
type ReferencePrice struct {
	Currency         string  `json:"currency"`
	Unit             *string `json:"unit"`
	Basis            *string `json:"basis"`
	InputPerMillion  *string `json:"inputPerMillion"`
	OutputPerMillion *string `json:"outputPerMillion"`
	SourceKind       string  `json:"sourceKind"`
	SourceLabel      string  `json:"sourceLabel"`
	SourceURL        *string `json:"sourceUrl"`
	ObservedAt       string  `json:"observedAt"`
	ValidUntil       *string `json:"validUntil"`
}
type ReferencePriceInput struct {
	Currency         string  `json:"currency"`
	Unit             string  `json:"unit,omitempty"`
	Basis            string  `json:"basis,omitempty"`
	InputPerMillion  *string `json:"inputPerMillion,omitzero"`
	OutputPerMillion *string `json:"outputPerMillion,omitzero"`
	SourceLabel      string  `json:"sourceLabel"`
	SourceURL        string  `json:"sourceUrl,omitempty"`
	ObservedAt       string  `json:"observedAt"`
	ValidUntil       string  `json:"validUntil,omitempty"`
}
type TargetProfile struct {
	ID             string          `json:"id"`
	ContextID      string          `json:"contextId"`
	ModelSelector  string          `json:"modelSelector"`
	Role           string          `json:"role"`
	Enabled        bool            `json:"enabled"`
	Revision       string          `json:"revision"`
	Source         ProfileSource   `json:"source"`
	ReferencePrice *ReferencePrice `json:"referencePrice"`
	UpdatedAt      string          `json:"updatedAt"`
}
type ContextUpdateInput struct {
	RequestID        string  `json:"requestId"`
	ExpectedRevision string  `json:"expectedRevision"`
	Label            *string `json:"label,omitzero"`
	Enabled          *bool   `json:"enabled,omitzero"`
}
type ProfileUpdateInput struct {
	RequestID           string               `json:"requestId"`
	ExpectedRevision    string               `json:"expectedRevision"`
	Enabled             *bool                `json:"enabled,omitzero"`
	ReferencePrice      *ReferencePriceInput `json:"referencePrice,omitempty"`
	ClearReferencePrice *bool                `json:"clearReferencePrice,omitzero"`
}
type DeclaredAccount struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`
	ProviderLabel *string `json:"providerLabel"`
	Entitlement   string  `json:"entitlement"`
	Revision      string  `json:"revision"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}
type AccountCreateInput struct {
	RequestID     string `json:"requestId"`
	Label         string `json:"label"`
	ProviderLabel string `json:"providerLabel,omitempty"`
	Entitlement   string `json:"entitlement,omitempty"`
}
type AccountUpdateInput struct {
	RequestID        string  `json:"requestId"`
	ExpectedRevision string  `json:"expectedRevision"`
	Label            *string `json:"label,omitzero"`
	ProviderLabel    *string `json:"providerLabel,omitzero"`
	Entitlement      string  `json:"entitlement,omitempty"`
}
type AccountLink struct {
	ContextID         string  `json:"contextId"`
	DeclaredAccountID *string `json:"declaredAccountId"`
	Revision          string  `json:"revision"`
	UpdatedAt         string  `json:"updatedAt"`
	Source            string  `json:"source"`
}
type AccountLinkInput struct {
	RequestID         string `json:"requestId"`
	ExpectedRevision  string `json:"expectedRevision"`
	DeclaredAccountID string `json:"declaredAccountId,omitempty"`
	Unlink            *bool  `json:"unlink,omitzero"`
}
type CapacitySubject struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type CapacityReport struct {
	ID               string          `json:"id"`
	Sequence         string          `json:"sequence"`
	RequestID        string          `json:"requestId"`
	Subject          CapacitySubject `json:"subject"`
	Dimension        string          `json:"dimension"`
	State            string          `json:"state"`
	Source           string          `json:"source"`
	ObservedAt       string          `json:"observedAt"`
	ReceivedAt       string          `json:"receivedAt"`
	ValidForSeconds  int             `json:"validForSeconds"`
	ExpiresAt        *string         `json:"expiresAt"`
	RetryAfter       *string         `json:"retryAfter"`
	CorrectionOf     *string         `json:"correctionOf"`
	CorrectionAction *string         `json:"correctionAction"`
	Bindings         []string        `json:"bindings"`
	Note             string          `json:"note"`
}
type CapacityReportInput struct {
	RequestID        string          `json:"requestId"`
	Subject          CapacitySubject `json:"subject"`
	ExpectedRevision string          `json:"expectedRevision"`
	Dimension        string          `json:"dimension"`
	State            string          `json:"state"`
	ObservedAt       string          `json:"observedAt,omitempty"`
	ValidForSeconds  *int            `json:"validForSeconds,omitempty"`
	RetryAfter       string          `json:"retryAfter,omitempty"`
	Note             string          `json:"note,omitempty"`
	CorrectionOf     string          `json:"correctionOf,omitempty"`
	CorrectionAction string          `json:"correctionAction,omitempty"`
}
type LinkInheritance struct {
	AccountID    string `json:"accountId"`
	LinkRevision string `json:"linkRevision"`
	LinkedAt     string `json:"linkedAt"`
}
type RetainedCapacityReport struct {
	Report        CapacityReport   `json:"report"`
	InheritedFrom *LinkInheritance `json:"inheritedFrom"`
}
type CapacityHeads struct {
	Occupancy *CapacityReport `json:"occupancy"`
	Allowance *CapacityReport `json:"allowance"`
	Service   *CapacityReport `json:"service"`
}
type CapacityState struct {
	ScopeKey          string                   `json:"scopeKey"`
	Subject           CapacitySubject          `json:"subject"`
	Revision          string                   `json:"revision"`
	Heads             CapacityHeads            `json:"heads"`
	RetainedNegatives []RetainedCapacityReport `json:"retainedNegatives"`
}
type ReportRef struct {
	ReportID      string           `json:"reportId"`
	Subject       CapacitySubject  `json:"subject"`
	Dimension     string           `json:"dimension"`
	State         string           `json:"state"`
	Source        string           `json:"source"`
	ObservedAt    string           `json:"observedAt"`
	ReceivedAt    string           `json:"receivedAt"`
	ExpiresAt     *string          `json:"expiresAt"`
	RetryAfter    *string          `json:"retryAfter"`
	Freshness     string           `json:"freshness"`
	Retained      bool             `json:"retained"`
	InheritedFrom *LinkInheritance `json:"inheritedFrom"`
}
type CapacityDimension struct {
	EffectiveState    string      `json:"effectiveState"`
	LatestReport      *ReportRef  `json:"latestReport"`
	BlockingReports   []ReportRef `json:"blockingReports"`
	PositiveReportIDs []string    `json:"positiveReportIds"`
}
type PositiveCoverage struct {
	Occupancy bool `json:"occupancy"`
	Allowance bool `json:"allowance"`
}
type ReportEligibility struct {
	Policy           string           `json:"policy"`
	Permitted        bool             `json:"permitted"`
	PositiveCoverage PositiveCoverage `json:"positiveCoverage"`
	ReasonCodes      []string         `json:"reasonCodes"`
}
type ManagedOccupancy struct {
	ResourceID      string  `json:"resourceId"`
	Limit           int     `json:"limit"`
	ActiveAttemptID *string `json:"activeAttemptId"`
	ActiveContextID *string `json:"activeContextId"`
	ClaimsSubject   bool    `json:"claimsSubject"`
}
type CapacityDimensions struct {
	Occupancy CapacityDimension `json:"occupancy"`
	Allowance CapacityDimension `json:"allowance"`
	Service   CapacityDimension `json:"service"`
}
type CapacityView struct {
	Subject           CapacitySubject    `json:"subject"`
	Revision          string             `json:"revision"`
	AsOf              string             `json:"asOf"`
	AccountLink       *AccountLink       `json:"accountLink"`
	Dimensions        CapacityDimensions `json:"dimensions"`
	ReportEligibility ReportEligibility  `json:"reportEligibility"`
	ManagedOccupancy  ManagedOccupancy   `json:"managedOccupancy"`
}
type ContextView struct {
	NativeContext
	AccountLink AccountLink  `json:"accountLink"`
	Capacity    CapacityView `json:"capacity"`
}
type CapacityReportResult struct {
	Report   CapacityReport `json:"report"`
	Capacity CapacityView   `json:"capacity"`
}
type CapacityReportDetail struct {
	Report              CapacityReport `json:"report"`
	UnresolvedScopeKeys []string       `json:"unresolvedScopeKeys"`
}

type RoutingSourceProof struct {
	RelativePath     string `json:"relativePath"`
	SHA256           string `json:"sha256"`
	PhysicalIdentity string `json:"physicalIdentity"`
	ByteLength       string `json:"byteLength"`
}
type SuitabilityAssessment struct {
	Classification string              `json:"classification"`
	Source         string              `json:"source"`
	RuleID         *string             `json:"ruleId"`
	SourceProof    *RoutingSourceProof `json:"sourceProof"`
	ReasonCodes    []string            `json:"reasonCodes"`
}
type NativeContextFact struct {
	ContextID          string        `json:"contextId"`
	InstallationStatus string        `json:"installationStatus"`
	NativeVersion      *ObservedFact `json:"nativeVersion"`
	ObservedAt         string        `json:"observedAt"`
	Source             string        `json:"source"`
}
type RoutingFacts struct {
	Assessment SuitabilityAssessment `json:"assessment"`
	Contexts   []NativeContextFact   `json:"contexts"`
}
type RoutingSnapshot struct {
	Policy           RoutingPolicy     `json:"policy"`
	Contexts         []NativeContext   `json:"contexts"`
	Profiles         []TargetProfile   `json:"profiles"`
	Accounts         []DeclaredAccount `json:"accounts"`
	Links            []AccountLink     `json:"links"`
	CapacityStates   []CapacityState   `json:"capacityStates"`
	ManagedOccupancy ManagedOccupancy  `json:"managedOccupancy"`
}
type RoutingSelectionInput struct {
	Snapshot   RoutingSnapshot
	Pins       ExecutionPins
	Options    RoutingOptions
	Assessment SuitabilityAssessment
	Facts      []NativeContextFact
	Now        string
}
type DeclaredAccountAssociation struct {
	AccountID    string `json:"accountId"`
	Label        string `json:"label"`
	LinkRevision string `json:"linkRevision"`
	Source       string `json:"source"`
}
type ResolvedTarget struct {
	ContextID                  string                      `json:"contextId"`
	ConnectionID               string                      `json:"connectionId"`
	HarnessID                  string                      `json:"harnessId"`
	ModelSelector              string                      `json:"modelSelector"`
	ProfileID                  *string                     `json:"profileId"`
	DeclaredAccountAssociation *DeclaredAccountAssociation `json:"declaredAccountAssociation"`
}
type CandidateAssessment struct {
	ProfileID       *string        `json:"profileId"`
	ProfileSnapshot *TargetProfile `json:"profileSnapshot"`
	ContextID       string         `json:"contextId"`
	Eligible        bool           `json:"eligible"`
	ReasonCodes     []string       `json:"reasonCodes"`
	Uncertainties   []string       `json:"uncertainties"`
}
type CapacityEffectiveStates struct {
	Occupancy string `json:"occupancy"`
	Allowance string `json:"allowance"`
	Service   string `json:"service"`
}
type CandidateCapacityEvidence struct {
	ContextID           string                  `json:"contextId"`
	Revision            string                  `json:"revision"`
	AsOf                string                  `json:"asOf"`
	DeclaredAccountID   *string                 `json:"declaredAccountId"`
	AccountLinkRevision *string                 `json:"accountLinkRevision"`
	EffectiveStates     CapacityEffectiveStates `json:"effectiveStates"`
	PositiveCoverage    PositiveCoverage        `json:"positiveCoverage"`
	Permitted           bool                    `json:"permitted"`
	BlockingReportIDs   []string                `json:"blockingReportIds"`
	SupportingReportIDs []string                `json:"supportingReportIds"`
}
type RoutingSelectionResult struct {
	CapacityEvidence []CandidateCapacityEvidence `json:"capacityEvidence"`
	SelectedTarget   ResolvedTarget              `json:"selectedTarget"`
	Candidates       []CandidateAssessment       `json:"candidates"`
	Uncertainties    []string                    `json:"uncertainties"`
}
type TaskRoutingAuthority struct {
	ID                    string                `json:"id"`
	TaskID                string                `json:"taskId"`
	TaskVersion           string                `json:"taskVersion"`
	CapturedAt            string                `json:"capturedAt"`
	PolicySnapshot        RoutingPolicy         `json:"policySnapshot"`
	OptionSnapshot        RoutingOptions        `json:"optionSnapshot"`
	SuitabilityAssessment SuitabilityAssessment `json:"suitabilityAssessment"`
	CreatedBy             string                `json:"createdBy"`
}
type BudgetWindow struct {
	Number           string  `json:"number"`
	WindowSeconds    int     `json:"windowSeconds"`
	FirstAdmissionAt *string `json:"firstAdmissionAt"`
	DeadlineAt       *string `json:"deadlineAt"`
}
type TaskBudget struct {
	TaskID               string       `json:"taskId"`
	Revision             string       `json:"revision"`
	GrantedAdmissions    string       `json:"grantedAdmissions"`
	ConsumedAdmissions   string       `json:"consumedAdmissions"`
	RemainingAdmissions  string       `json:"remainingAdmissions"`
	AutomaticAdmissions  string       `json:"automaticAdmissions"`
	HistoricalAdmissions string       `json:"historicalAdmissions"`
	CurrentWindow        BudgetWindow `json:"currentWindow"`
	LatestAmendmentID    *string      `json:"latestAmendmentId"`
	UpdatedAt            string       `json:"updatedAt"`
}
type BudgetEffect struct {
	AdmissionUnits      string       `json:"admissionUnits"`
	RemainingAdmissions string       `json:"remainingAdmissions"`
	BudgetRevision      string       `json:"budgetRevision"`
	Window              BudgetWindow `json:"window"`
}
type RoutingDecision struct {
	CapacityEvidence     []CandidateCapacityEvidence `json:"capacityEvidence"`
	ID                   string                      `json:"id"`
	TaskID               string                      `json:"taskId"`
	TaskVersion          string                      `json:"taskVersion"`
	AttemptID            string                      `json:"attemptId"`
	RequestID            string                      `json:"requestId"`
	AuthorityID          string                      `json:"authorityId"`
	Kind                 string                      `json:"kind"`
	PreviousAttemptID    *string                     `json:"previousAttemptId"`
	CreatedAt            string                      `json:"createdAt"`
	CandidateAssessments []CandidateAssessment       `json:"candidateAssessments"`
	SelectedTarget       ResolvedTarget              `json:"selectedTarget"`
	BudgetEffect         BudgetEffect                `json:"budgetEffect"`
	EvidenceSnapshot     RoutingFacts                `json:"evidenceSnapshot"`
	Uncertainties        []string                    `json:"uncertainties"`
}
type TaskRoutingView struct {
	Authority        *TaskRoutingAuthority `json:"authority"`
	LatestDecision   *RoutingDecision      `json:"latestDecision"`
	AdoptionRequired bool                  `json:"adoptionRequired"`
}
type TaskBudgetView struct {
	Budget           *TaskBudget `json:"budget"`
	AdoptionRequired bool        `json:"adoptionRequired"`
}
type BudgetAmendmentInput struct {
	RequestID              string          `json:"requestId"`
	Action                 string          `json:"action"`
	ExpectedRevision       string          `json:"expectedRevision"`
	ExpectedBudgetRevision string          `json:"expectedBudgetRevision,omitempty"`
	AdditionalAdmissions   string          `json:"additionalAdmissions"`
	NewWindowSeconds       *int            `json:"newWindowSeconds,omitempty"`
	Routing                *RoutingOptions `json:"routing,omitempty"`
}
type BudgetAmendment struct {
	ID                   string      `json:"id"`
	TaskID               string      `json:"taskId"`
	RequestID            string      `json:"requestId"`
	Action               string      `json:"action"`
	AdditionalAdmissions string      `json:"additionalAdmissions"`
	NewWindowSeconds     *int        `json:"newWindowSeconds"`
	PriorBudget          *TaskBudget `json:"priorBudget"`
	Budget               TaskBudget  `json:"budget"`
	AuthorityID          *string     `json:"authorityId"`
	TaskVersion          string      `json:"taskVersion"`
	CreatedAt            string      `json:"createdAt"`
}
type BudgetAmendmentResult struct {
	Task      TaskSummary           `json:"task"`
	Version   *TaskVersion          `json:"version,omitempty"`
	Amendment BudgetAmendment       `json:"amendment"`
	Budget    TaskBudget            `json:"budget"`
	Authority *TaskRoutingAuthority `json:"authority,omitempty"`
}
type BudgetLedgerEntry struct {
	ID             string       `json:"id"`
	Sequence       string       `json:"sequence"`
	TaskID         string       `json:"taskId"`
	BudgetRevision string       `json:"budgetRevision"`
	Kind           string       `json:"kind"`
	RequestID      string       `json:"requestId"`
	AttemptID      *string      `json:"attemptId"`
	GrantedDelta   string       `json:"grantedDelta"`
	ConsumedDelta  string       `json:"consumedDelta"`
	Window         BudgetWindow `json:"window"`
	CreatedAt      string       `json:"createdAt"`
}
type ContinuationProof struct {
	AttemptID             string   `json:"attemptId"`
	OwnerInstanceID       string   `json:"ownerInstanceId"`
	TaskID                string   `json:"taskId"`
	TaskVersion           string   `json:"taskVersion"`
	TaskRevision          string   `json:"taskRevision"`
	BudgetRevision        string   `json:"budgetRevision"`
	Category              string   `json:"category"`
	PrepareAttempted      bool     `json:"prepareAttempted"`
	CreateAttempted       bool     `json:"createAttempted"`
	ExpectedOutputMissing bool     `json:"expectedOutputMissing"`
	EvidenceReportIDs     []string `json:"evidenceReportIds"`
	ReasonCode            string   `json:"reasonCode"`
	Eligible              bool     `json:"eligible"`
	CreatedAt             string   `json:"createdAt"`
}

type UsageItem struct {
	ExecutorKind        string                   `json:"executorKind,omitzero"`
	Role                string                   `json:"role,omitzero"`
	AttemptID           string                   `json:"attemptId"`
	AttemptSequence     string                   `json:"attemptSequence"`
	ExecutionMode       string                   `json:"executionMode"`
	Outcome             *string                  `json:"outcome"`
	SelectedUsageScope  string                   `json:"selectedUsageScope"`
	Usage               UsageObservation         `json:"usage"`
	NativeModels        *NativeModelsObservation `json:"nativeModels"`
	EvidenceArtifactIDs []string                 `json:"evidenceArtifactIds"`
}
type CounterSubtotal struct {
	KnownValue             *string  `json:"knownValue"`
	ContributingAttemptIDs []string `json:"contributingAttemptIds"`
	MissingAttemptIDs      []string `json:"missingAttemptIds"`
}
type ScopeSubtotal struct {
	InputTokens              CounterSubtotal `json:"inputTokens"`
	OutputTokens             CounterSubtotal `json:"outputTokens"`
	CachedInputTokens        CounterSubtotal `json:"cachedInputTokens"`
	CacheCreationInputTokens CounterSubtotal `json:"cacheCreationInputTokens"`
}
type UsageScopeSubtotals struct {
	TopLevel  ScopeSubtotal `json:"top_level"`
	WholeTree ScopeSubtotal `json:"whole_tree"`
}
type UsageSubtotal struct {
	Scope        string              `json:"scope"`
	ByUsageScope UsageScopeSubtotals `json:"byUsageScope"`
}
type UsageCoverage struct {
	ExcludedLocalAttemptIDs []string `json:"excludedLocalAttemptIds,omitzero"`
	Scope                   string   `json:"scope"`
	Status                  string   `json:"status"`
	IncludedAttemptIDs      []string `json:"includedAttemptIds"`
	UnknownAttemptIDs       []string `json:"unknownAttemptIds"`
	ExcludedFakeAttemptIDs  []string `json:"excludedFakeAttemptIds"`
}
type EstimateItem struct {
	AttemptID           string   `json:"attemptId"`
	Model               string   `json:"model"`
	Currency            string   `json:"currency"`
	Value               string   `json:"value"`
	Scope               string   `json:"scope"`
	Source              string   `json:"source"`
	ObservedAt          string   `json:"observedAt"`
	EvidenceArtifactIDs []string `json:"evidenceArtifactIds"`
	PreviewCompleteness string   `json:"previewCompleteness"`
}
type UsageReferencePrice struct {
	AttemptID      string         `json:"attemptId"`
	DecisionID     string         `json:"decisionId"`
	ProfileID      *string        `json:"profileId"`
	ReferencePrice ReferencePrice `json:"referencePrice"`
}
type UsageCharges struct {
	Status string     `json:"status"`
	Items  []struct{} `json:"items"`
}
type UsagePage struct {
	TaskID          string                `json:"taskId"`
	Items           []UsageItem           `json:"items"`
	NextCursor      *string               `json:"nextCursor"`
	KnownSubtotal   UsageSubtotal         `json:"knownSubtotal"`
	Coverage        UsageCoverage         `json:"coverage"`
	Estimates       []EstimateItem        `json:"estimates"`
	ReferencePrices []UsageReferencePrice `json:"referencePrices"`
	Charges         UsageCharges          `json:"charges"`
}
