package domain

// GraphDeliveryScopeView exposes captured user destination choices without
// serializing the native identity facts retained in the private definition.
type GraphDeliveryScopeView struct {
	LocalMode            string               `json:"localMode"`
	PreferenceRevision   string               `json:"preferenceRevision,omitzero"`
	Source               string               `json:"source"`
	ProjectID            string               `json:"projectId"`
	SelectedRelativePath string               `json:"selectedRelativePath"`
	Branch               *string              `json:"branch"`
	Destination          string               `json:"destination"`
	ExternalAction       *ExternalActionInput `json:"externalAction,omitzero"`
}
type GraphDefinitionView struct {
	GraphID         string                 `json:"graphId"`
	GraphVersion    string                 `json:"graphVersion"`
	RootTaskID      string                 `json:"rootTaskId"`
	Original        GraphStartInput        `json:"original"`
	RequestedPolicy Policy                 `json:"requestedPolicy"`
	RoutingPolicy   RoutingPolicy          `json:"routingPolicy"`
	Delivery        GraphDeliveryScopeView `json:"delivery"`
	CreatedAt       string                 `json:"createdAt"`
}

func GraphDefinitionPublic(d GraphDefinition) GraphDefinitionView {
	scope := d.Delivery
	return GraphDefinitionView{GraphID: d.GraphID, GraphVersion: d.GraphVersion, RootTaskID: d.RootTaskID, Original: d.Original, RequestedPolicy: d.RequestedPolicy, RoutingPolicy: d.RoutingPolicy, CreatedAt: d.CreatedAt, Delivery: GraphDeliveryScopeView{LocalMode: scope.LocalMode, PreferenceRevision: scope.PreferenceRevision, Source: scope.Source, ProjectID: scope.ProjectID, SelectedRelativePath: scope.SelectedRelativePath, Branch: scope.Branch, Destination: scope.Destination, ExternalAction: scope.ExternalAction}}
}
