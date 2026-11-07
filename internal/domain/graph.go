package domain

const (
	GraphContract            = "006-v1"
	MaxGraphNodes            = 16
	MaxGraphEdges            = 64
	MaxGraphDeclarationBytes = 2 << 20
	GraphPreparationSeconds  = 300
	MaxManagedSlots          = 2
)

// GraphRef is a server-derived membership projection, never a client authority.
type GraphRef struct {
	GraphID    string  `json:"graphId"`
	NodeID     *string `json:"nodeId"`
	RootTaskID string  `json:"rootTaskId"`
	Kind       string  `json:"kind"`
}

type GraphBudgetRef struct {
	GraphID           string `json:"graphId"`
	BudgetOwnerTaskID string `json:"budgetOwnerTaskId"`
}

// GraphClaimView contains Council's actual reservations, not provider identity,
// subscription allowance or external occupancy observations.
type GraphClaimView struct {
	Slot                      string  `json:"slot"`
	AttemptID                 string  `json:"attemptId"`
	GraphID                   *string `json:"graphId"`
	NodeID                    *string `json:"nodeId"`
	ExecutorKind              string  `json:"executorKind"`
	ContextID                 *string `json:"contextId"`
	CapturedDeclaredAccountID *string `json:"capturedDeclaredAccountId"`
	CapturedLinkRevision      *string `json:"capturedLinkRevision"`
	LegacyBarrier             bool    `json:"legacyBarrier"`
}
