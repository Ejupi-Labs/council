package engine

import (
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphMutationStrictIntentRejectsUnknownAuthorityBeforeCreatingTasks(t *testing.T) {
	e, projectID, _ := testEngine(t)
	ctx := context.Background()
	valid := `{"requestId":"` + domain.NewID() + `","projectId":"` + projectID + `","executionMode":"fake","nodes":[{"key":"a","originalText":"Complete first instruction"}]}`
	for name, body := range map[string]string{
		"private root proof":     strings.TrimSuffix(valid, "}") + `,"rootBinding":{}}`,
		"claimed budget":         strings.TrimSuffix(valid, "}") + `,"budgetOwnerTaskId":"` + domain.NewID() + `"}`,
		"null declaration":       strings.Replace(valid, `"nodes":[{"key":"a","originalText":"Complete first instruction"}]`, `"nodes":null`, 1),
		"unknown node authority": strings.Replace(valid, `"originalText":"Complete first instruction"`, `"originalText":"Complete first instruction","graphRef":{}`, 1),
		"duplicate member":       strings.TrimSuffix(valid, "}") + `,"projectId":"` + projectID + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			result := e.MutateGraph(ctx, "graphs.start", "", []byte(body))
			if result.Envelope.Error == nil || result.Status != 400 {
				t.Fatal("Untrusted graph authority or malformed intent was accepted", result)
			}
		})
	}
	if result := e.MutateGraph(ctx, "graphs.start", domain.NewID(), []byte(valid)); result.Envelope.Error == nil {
		t.Fatal("Creation accepted an existing target", result)
	}
	if result := e.MutateGraph(ctx, "graphs.start", "", []byte(strings.Repeat(" ", domain.MaxGraphDeclarationBytes+1))); result.Envelope.Error == nil || result.Envelope.Error.Code != "input_too_large" {
		t.Fatal("Graph declaration bound was not enforced before decoding", result)
	}
	tasks, err := e.db.Tasks(ctx, projectID, 30, "")
	if err != nil || len(tasks.Items) != 0 {
		t.Fatal("Rejected declaration created durable work", tasks, err)
	}
}

func TestGraphMutationReplayPrecedesChangedProjectAndDispatchPrerequisites(t *testing.T) {
	e, projectID, repo := testEngine(t)
	ctx := context.Background()
	facts, err := e.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: []domain.GraphNodeInput{{Key: "a", OriginalText: "Complete retained instruction"}}}
	// Persist the receipt directly without waking a new execution. The current
	// owner is deliberately fenced before any scheduler tick can claim it.
	e.control.Lock()
	e.fenced = true
	created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, in, facts)
	e.control.Unlock()
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	moved := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-moved")
	if err := os.Rename(repo, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Rename(moved, repo); err != nil {
			t.Error(err)
		}
	})
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	replayed := e.MutateGraph(ctx, "graphs.start", "", body)
	if replayed.Envelope.Error != nil || replayed.Status != 201 {
		t.Fatal("Known creation receipt was hidden by changed prerequisites", replayed)
	}
	before, _ := json.Marshal(created.Envelope.Data)
	after, _ := json.Marshal(replayed.Envelope.Data)
	var beforeData, afterData any
	if err := json.Unmarshal(before, &beforeData); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &afterData); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeData, afterData) {
		t.Fatal("Replay changed the admitted group, nodes or budget", string(before), string(after))
	}
}
