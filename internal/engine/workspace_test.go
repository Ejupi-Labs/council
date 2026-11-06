package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func TestWorkspaceEvidenceStoreBridge(t *testing.T) {
	e, project, repo := testEngine(t)
	e.control.Lock()
	defer e.control.Unlock()
	result := e.db.CreateTask(context.Background(), e.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Capture the actual current fixture source.", ExecutionMode: "fake"})
	if result.Envelope.Error != nil {
		t.Fatal(result.Envelope.Error)
	}
	pending, err := e.db.QueuedAttempts(context.Background(), e.config.OwnerInstanceID, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("%+v %v", pending, err)
	}
	a, err := e.db.ClaimAttempt(context.Background(), pending[0].ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := e.workspace.Capture(context.Background(), workspace.Request{AttemptID: a.ID, ProjectPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.evidence.VerifyClosed(context.Background(), snapshot.ManifestArtifactID); err != nil {
		t.Fatal(err)
	}
}
