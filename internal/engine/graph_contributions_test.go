package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphContributionSelectionRequiresCurrentAcceptedPredecessors(t *testing.T) {
	for _, kind := range []string{"files", "context"} {
		t.Run(kind, func(t *testing.T) {
			e, projectID, _ := testEngine(t)
			stopFixtureScheduler(t, e)
			ctx := context.Background()
			creation, err := e.captureGraphCreateFacts(ctx, projectID)
			if err != nil {
				t.Fatal(err)
			}
			dependencies := []domain.GraphDependencyInput{{From: "producer", To: "receiver", Kind: kind}}
			created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake",
				Nodes: []domain.GraphNodeInput{{Key: "producer", OriginalText: "A predecessor that has not been accepted."}, {Key: "receiver", OriginalText: "Use only its exact accepted input."}}, Dependencies: &dependencies}, creation)
			if created.Envelope.Error != nil {
				t.Fatal(created)
			}
			graph := created.Envelope.Data.(domain.GraphAdmission)
			attempt, err := e.db.ClaimAttempt(ctx, graph.PreparationAttemptID, e.config.OwnerInstanceID)
			if err != nil {
				t.Fatal(err)
			}
			e.run(ctx, attempt)
			definition, err := e.db.GraphDefinitionPrivate(ctx, graph.Graph.ID)
			if err != nil {
				t.Fatal(err)
			}
			root, err := e.db.GraphRoot(ctx, graph.Graph.ID)
			if err != nil {
				t.Fatal(err)
			}
			view, err := e.db.Graph(ctx, graph.Graph.ID)
			if err != nil || len(view.Nodes) != 2 {
				t.Fatal(view, err)
			}
			beforeArtifacts := graphGuardArtifactNames(t, e)
			beforeBudget, err := e.db.GraphBudget(ctx, graph.Graph.ID)
			if err != nil {
				t.Fatal(err)
			}
			// A real independent root has no selected dependencies. It must remain
			// usable even while another declared node awaits an actual acceptance.
			independent, err := e.selectGraphContributions(ctx, definition, root, view.Nodes[0])
			if err != nil || len(independent.Sources) != 0 || len(independent.ContributorVector) != 0 || len(independent.ConstraintRefs) != 0 || len(independent.EvidenceRefs) != 0 {
				t.Fatal("Independent current graph node acquired unavailable/fictional upstream context", independent, err)
			}
			dependent, err := e.selectGraphContributions(ctx, definition, root, view.Nodes[1])
			if err == nil || len(dependent.Sources) != 0 || len(dependent.ContributorVector) != 0 {
				t.Fatal("An unaccepted actual predecessor supplied a usable selected contribution", dependent, err)
			}
			afterBudget, err := e.db.GraphBudget(ctx, graph.Graph.ID)
			if err != nil || !reflect.DeepEqual(beforeBudget, afterBudget) || !reflect.DeepEqual(beforeArtifacts, graphGuardArtifactNames(t, e)) {
				t.Fatal("Read-only selection spent admissions or created evidence", err)
			}
		})
	}
}
