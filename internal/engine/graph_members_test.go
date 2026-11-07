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

func graphMemberPage(t *testing.T, e *Engine, graphID, inputID string, query map[string]string) domain.Page[domain.GraphMemberView] {
	t.Helper()
	result := e.readGraph(context.Background(), "graphs.inputs.members", graphID+"/"+inputID, query)
	if result.Status != 200 || result.Envelope.Error != nil {
		t.Fatal("Complete retained input members are unavailable", result)
	}
	body, err := json.Marshal(result.Envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"projectIdentity", "repositoryIdentity", "workspacePath", "physicalIdentity", e.lock.Root()} {
		if strings.Contains(string(body), private) {
			t.Fatal("A public retained-member page exposed private physical metadata", private)
		}
	}
	var page domain.Page[domain.GraphMemberView]
	if err = json.Unmarshal(body, &page, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestGraphMembersReadExactRetainedRootAndInputWithoutEffects(t *testing.T) {
	f := graphOutputAdmittedAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	view, err := e.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || view.Graph.RootInputID == nil {
		t.Fatal(view, err)
	}
	beforeBudget, err := e.db.GraphBudget(ctx, view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeArtifacts := graphGuardArtifactNames(t, e)
	for _, inputID := range []string{*view.Graph.RootInputID, f.inputBinding.ID} {
		page := graphMemberPage(t, e, view.Graph.ID, inputID, map[string]string{"limit": "100"})
		if len(page.Items) != 2 || page.NextCursor != nil {
			t.Fatal("The actual one-file working/index closure is incomplete", page)
		}
		var working, index *domain.GraphMemberView
		for i := range page.Items {
			member := &page.Items[i]
			if member.Path != "source.txt" || member.State != "present" || member.ArtifactRef == nil || member.SHA256 == nil || member.ByteLength == nil || len(member.Origins) != 1 {
				t.Fatal("A complete retained row or attribution is absent", member)
			}
			origin := member.Origins[0]
			if origin.Kind != "root" || origin.NodeID != nil || origin.TaskID != view.Graph.RootTaskID || origin.InputID != *view.Graph.RootInputID || origin.ProducerAttemptID != f.rootBinding.OperationAttemptID || origin.StateSHA256 != f.rootBinding.BasisSHA256 {
				t.Fatal("An unchanged root member gained node/file ancestry", origin)
			}
			meta, err := e.db.Artifact(ctx, member.ArtifactRef.ArtifactID)
			if err != nil || meta.SHA256 == nil || *meta.SHA256 != *member.SHA256 || meta.ByteLength != *member.ByteLength {
				t.Fatal("The row did not link its exact actual closed payload", member, err)
			}
			if _, err = e.readGraphArtifact(ctx, *member.ArtifactRef, meta.Kind, 64<<20); err != nil {
				t.Fatal(err)
			}
			if member.Dimension == "working" {
				working = member
			} else if member.Dimension == "index" {
				index = member
			} else {
				t.Fatal("Unknown retained-state dimension", member)
			}
		}
		if working == nil || index == nil || *working.SHA256 == *index.SHA256 || index.ObjectID == nil || working.ObjectID != nil {
			t.Fatal("Distinct actual working and staged bytes were conflated", working, index)
		}
	}
	afterBudget, err := e.db.GraphBudget(ctx, view.Graph.ID)
	if err != nil || !reflect.DeepEqual(beforeBudget, afterBudget) || !reflect.DeepEqual(beforeArtifacts, graphGuardArtifactNames(t, e)) {
		t.Fatal("Member inspection wrote evidence or spent shared allowance", afterBudget, err)
	}
}

func TestGraphMembersCursorIsBoundToExactImmutableInput(t *testing.T) {
	f := graphOutputAdmittedAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	view, err := e.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || view.Graph.RootInputID == nil {
		t.Fatal(view, err)
	}
	first := graphMemberPage(t, e, view.Graph.ID, f.inputBinding.ID, map[string]string{"limit": "1"})
	if len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatal("The bounded exact-input page omitted its continuation", first)
	}
	second := graphMemberPage(t, e, view.Graph.ID, f.inputBinding.ID, map[string]string{"limit": "1", "cursor": *first.NextCursor})
	if len(second.Items) != 1 || second.NextCursor != nil || first.Items[0].Dimension == second.Items[0].Dimension {
		t.Fatal("The cursor skipped/repeated a complete working/index row", first, second)
	}
	for _, selected := range []struct{ input, cursor string }{{*view.Graph.RootInputID, *first.NextCursor}, {f.inputBinding.ID, "invalid"}, {f.inputBinding.ID, *first.NextCursor + "="}} {
		result := e.readGraph(ctx, "graphs.inputs.members", view.Graph.ID+"/"+selected.input, map[string]string{"limit": "1", "cursor": selected.cursor})
		if result.Envelope.Error == nil || result.Envelope.Error.Code != "invalid_cursor" {
			t.Fatal("A foreign or noncanonical immutable member cursor was accepted", result)
		}
	}
	foreign := e.readGraph(ctx, "graphs.inputs.members", domain.NewID()+"/"+f.inputBinding.ID, nil)
	if foreign.Envelope.Error == nil {
		t.Fatal("A foreign graph could inspect this retained input", foreign)
	}
}

func TestGraphMembersRejectCorruptWorkingOrStagedPayloadBeforePage(t *testing.T) {
	for _, dimension := range []string{"working", "index"} {
		t.Run(dimension, func(t *testing.T) {
			f := graphOutputAdmittedAttemptFixture(t)
			e, ctx := f.engine, context.Background()
			verified, err := e.workspace.VerifyGraphInput(ctx, f.inputBinding)
			if err != nil {
				t.Fatal(err)
			}
			id := verified.Manifest.Entries[0].ArtifactID
			if dimension == "index" {
				id = verified.StagedBlobs.Entries[0].ArtifactRef.ArtifactID
			}
			meta, err := e.db.Artifact(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.OpenFile(filepath.Join(e.lock.Root(), filepath.FromSlash(meta.PrivateRef)), os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := file.WriteAt([]byte("!"), 0)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatal(writeErr, closeErr)
			}
			result := e.readGraph(ctx, "graphs.inputs.members", f.dispatch.Dispatch.GraphID+"/"+f.inputBinding.ID, map[string]string{"limit": "1"})
			if result.Envelope.Error == nil || result.Status == 200 || result.Envelope.Data != nil {
				t.Fatal("A complete member page survived actual required payload corruption", result)
			}
		})
	}
}

func TestGraphMemberFileAncestorsUseActualMixedDefinitionAndDiamond(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	ownerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.ctx, e.cancel = ownerCtx, cancel
	edges := []domain.GraphDependencyInput{
		{From: "ancestor", To: "left", Kind: "files"}, {From: "ancestor", To: "right", Kind: "files"},
		{From: "left", To: "receiver", Kind: "files"}, {From: "right", To: "receiver", Kind: "files"},
		{From: "context", To: "receiver", Kind: "context"},
	}
	group, _ := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{Nodes: []domain.GraphNodeInput{
		{Key: "ancestor", OriginalText: "A declared file ancestor; this scope test claims no execution or acceptance."},
		{Key: "context", OriginalText: "A declared context-only peer; its prose cannot establish file ancestry."},
		{Key: "left", OriginalText: "The left declared files branch."}, {Key: "right", OriginalText: "The right declared files branch."},
		{Key: "receiver", OriginalText: "The actual captured receiver of both file branches and context."},
	}, Dependencies: &edges})
	view, err := e.db.Graph(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, node := range view.Nodes {
		ids[node.Key] = node.ID
		if node.AcceptedResult != nil || node.LatestAttemptID != nil {
			t.Fatal("A pure topology fixture manufactured accepted or native work", node)
		}
	}
	allowed, err := graphMemberFileAncestors(definition, view.Nodes, definition.GraphVersion, ids["receiver"])
	if err != nil {
		t.Fatal("The actual immutable mixed/diamond declaration is unavailable", err)
	}
	for _, key := range []string{"ancestor", "left", "right"} {
		if !allowed[ids[key]] {
			t.Fatal("Files-only reachability lost a real captured diamond ancestor", key, allowed)
		}
	}
	if allowed[ids["context"]] || allowed[ids["receiver"]] || len(allowed) != 3 {
		t.Fatal("A captured context edge was promoted to file ancestry", allowed)
	}
	if _, err = graphMemberFileAncestors(definition, view.Nodes, "2", ids["receiver"]); err == nil {
		t.Fatal("The scope guard substituted another definition version")
	}
	if _, err = graphMemberFileAncestors(definition, view.Nodes, definition.GraphVersion, domain.NewID()); err == nil {
		t.Fatal("The scope guard inferred a foreign receiving node")
	}
}
