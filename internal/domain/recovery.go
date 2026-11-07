package domain

// RecoveryScope binds a checkpoint to the exact task or graph authority.
type RecoveryScope struct {
	Kind                         string               `json:"kind"`
	TaskID                       string               `json:"taskId,omitempty"`
	TaskVersion                  string               `json:"taskVersion,omitempty"`
	ExpectedTaskRevision         string               `json:"expectedTaskRevision,omitempty"`
	GraphID                      string               `json:"graphId,omitempty"`
	GraphVersion                 string               `json:"graphVersion,omitempty"`
	ExpectedGraphControlRevision string               `json:"expectedGraphControlRevision,omitempty"`
	NodeID                       string               `json:"nodeId,omitempty"`
	ExpectedNodeControlRevision  string               `json:"expectedNodeControlRevision,omitempty"`
	ExpectedBudgetAuthorityEpoch string               `json:"expectedBudgetAuthorityEpoch,omitempty"`
	ExpectedWindowNumber         string               `json:"expectedWindowNumber,omitempty"`
	NodeAuthorities              []GraphNodeAuthority `json:"nodeAuthorities,omitempty"`
}

type PauseTaskInput struct {
	RequestID            string `json:"requestId"`
	TaskVersion          string `json:"taskVersion"`
	ExpectedTaskRevision string `json:"expectedTaskRevision"`
}

type PauseGraphInput struct {
	RequestID                    string               `json:"requestId"`
	GraphVersion                 string               `json:"graphVersion"`
	ExpectedGraphControlRevision string               `json:"expectedGraphControlRevision"`
	ExpectedBudgetAuthorityEpoch string               `json:"expectedBudgetAuthorityEpoch"`
	ExpectedWindowNumber         string               `json:"expectedWindowNumber"`
	NodeAuthorities              []GraphNodeAuthority `json:"nodeAuthorities"`
}

type ResumeCheckpointInput struct {
	RequestID                  string        `json:"requestId"`
	ExpectedCheckpointRevision string        `json:"expectedCheckpointRevision"`
	Scope                      RecoveryScope `json:"scope"`
}

type RecoveryCheckpoint struct {
	ID                  string        `json:"id"`
	Scope               RecoveryScope `json:"scope"`
	Revision            string        `json:"revision"`
	State               string        `json:"state"`
	OwnerInstanceID     string        `json:"ownerInstanceId"`
	CreatedAt           string        `json:"createdAt"`
	UpdatedAt           string        `json:"updatedAt"`
	AdmittedAttemptIDs  []string      `json:"admittedAttemptIds"`
	CompletedAttemptIDs []string      `json:"completedAttemptIds"`
	UnfinishedTaskIDs   []string      `json:"unfinishedTaskIds"`
	Blockers            []string      `json:"blockers"`
	NextAction          string        `json:"nextAction"`
}

type RecoveryPauseResult struct {
	Checkpoint RecoveryCheckpoint `json:"checkpoint"`
}

type ReconciliationInput struct {
	RequestID                string `json:"requestId"`
	ExpectedIncidentRevision string `json:"expectedIncidentRevision"`
	Action                   string `json:"action"`
}

type ReconciliationObservation struct {
	ID              string `json:"id"`
	IncidentID      string `json:"incidentId"`
	OwnerInstanceID string `json:"ownerInstanceId"`
	AttemptID       string `json:"attemptId"`
	Outcome         string `json:"outcome"`
	Reason          string `json:"reason"`
	CreatedAt       string `json:"createdAt"`
}

type ReconciliationDecision struct {
	ID            string `json:"id"`
	IncidentID    string `json:"incidentId"`
	RequestID     string `json:"requestId"`
	ObservationID string `json:"observationId"`
	Outcome       string `json:"outcome"`
	CreatedAt     string `json:"createdAt"`
}

func validRecoveryRevision(value string) bool { _, err := ParseRevision(value); return err == nil }

func ValidatePauseTaskInput(in PauseTaskInput) *Error {
	if !ValidID(in.RequestID) || !validRecoveryRevision(in.TaskVersion) || !validRecoveryRevision(in.ExpectedTaskRevision) {
		return Invalid("pause task identity and revisions")
	}
	return nil
}

func ValidatePauseGraphInput(in PauseGraphInput) *Error {
	if !ValidID(in.RequestID) || !validRecoveryRevision(in.GraphVersion) || !validRecoveryRevision(in.ExpectedGraphControlRevision) || !validRecoveryRevision(in.ExpectedBudgetAuthorityEpoch) || !validRecoveryRevision(in.ExpectedWindowNumber) {
		return Invalid("pause graph identity and revisions")
	}
	return validateGraphNodeAuthorities(in.NodeAuthorities)
}

func ValidateRecoveryScope(in RecoveryScope) *Error {
	switch in.Kind {
	case "task":
		if !ValidID(in.TaskID) || !validRecoveryRevision(in.TaskVersion) || !validRecoveryRevision(in.ExpectedTaskRevision) || in.GraphID != "" || in.GraphVersion != "" || in.ExpectedGraphControlRevision != "" || in.NodeID != "" || in.ExpectedNodeControlRevision != "" || in.ExpectedBudgetAuthorityEpoch != "" || in.ExpectedWindowNumber != "" || in.NodeAuthorities != nil {
			return Invalid("standalone checkpoint scope")
		}
	case "member":
		if !ValidID(in.TaskID) || !validRecoveryRevision(in.TaskVersion) || !validRecoveryRevision(in.ExpectedTaskRevision) || !ValidID(in.GraphID) || !validRecoveryRevision(in.GraphVersion) || !validRecoveryRevision(in.ExpectedGraphControlRevision) || !ValidID(in.NodeID) || !validRecoveryRevision(in.ExpectedNodeControlRevision) || !validRecoveryRevision(in.ExpectedBudgetAuthorityEpoch) || !validRecoveryRevision(in.ExpectedWindowNumber) || in.NodeAuthorities != nil {
			return Invalid("graph member checkpoint scope")
		}
	case "graph":
		if in.TaskID != "" || in.TaskVersion != "" || in.ExpectedTaskRevision != "" || !ValidID(in.GraphID) || !validRecoveryRevision(in.GraphVersion) || !validRecoveryRevision(in.ExpectedGraphControlRevision) || in.NodeID != "" || in.ExpectedNodeControlRevision != "" || !validRecoveryRevision(in.ExpectedBudgetAuthorityEpoch) || !validRecoveryRevision(in.ExpectedWindowNumber) {
			return Invalid("graph checkpoint scope")
		}
		return validateGraphNodeAuthorities(in.NodeAuthorities)
	default:
		return Invalid("checkpoint scope kind")
	}
	return nil
}

func ValidateResumeCheckpointInput(in ResumeCheckpointInput) *Error {
	if !ValidID(in.RequestID) || !validRecoveryRevision(in.ExpectedCheckpointRevision) {
		return Invalid("resume identity and checkpoint revision")
	}
	return ValidateRecoveryScope(in.Scope)
}

func ValidateReconciliationInput(in ReconciliationInput) *Error {
	if !ValidID(in.RequestID) || !validRecoveryRevision(in.ExpectedIncidentRevision) || in.Action != "verify_and_reconcile" {
		return Invalid("reconciliation identity, revision and action")
	}
	return nil
}

// RecoveryFreshFacts are controller-only observations; never decode these from
// a public request. Every artifact proof comes from a fresh closed-file check.
type RecoveryFreshFacts struct {
	AttemptID              string
	AttemptRevision        string
	CleanupEventSequence   string
	TerminalSHA256         string
	VerifiedArtifacts      []VerifiedResponse
	StandaloneSealSequence string
	StandaloneBinding      *CandidateBinding
}

type RecoveryEvidenceInventory struct {
	AttemptID            string
	AttemptRevision      string
	CleanupEventSequence string
	TerminalSHA256       string
	Artifacts            []ArtifactRecord
}

type RecoveryIncidentView struct {
	Incident   ExecutionIncident `json:"incident"`
	Revision   string            `json:"revision"`
	State      string            `json:"state"`
	Blockers   []string          `json:"blockers"`
	NextAction string            `json:"nextAction"`
}

type RecoveryReconciliationResult struct {
	Incident RecoveryIncidentView   `json:"incident"`
	Decision ReconciliationDecision `json:"decision"`
}
