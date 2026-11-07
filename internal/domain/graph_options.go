package domain

// GraphNodeRoutingBasis freezes prospective policy choices without pretending
// that receiving-input suitability was assessed before that input existed.
type GraphNodeRoutingBasis struct {
	PolicySnapshot RoutingPolicy  `json:"policySnapshot"`
	OptionSnapshot RoutingOptions `json:"optionSnapshot"`
}

// GraphNodeExecutionOptions is a private per-version controller projection.
// Public task/version DTOs never expose this persistence envelope.
type GraphNodeExecutionOptions struct {
	GraphID       string                 `json:"graphId"`
	NodeID        string                 `json:"nodeId"`
	TaskID        string                 `json:"taskId"`
	TaskVersion   string                 `json:"taskVersion"`
	ExecutionMode string                 `json:"executionMode"`
	FakeScenario  string                 `json:"fakeScenario"`
	RoutingBasis  *GraphNodeRoutingBasis `json:"routingBasis,omitzero"`
	Routing       *RoutingOptions        `json:"routing"`
	Workflow      *WorkflowInput         `json:"workflow"`
}
