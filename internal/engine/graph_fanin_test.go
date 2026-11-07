package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func TestGraphActorComposesAcceptedFilesAndBlocksActualDivergenceBeforeReceiver(t *testing.T) {
	for _, conflicting := range []bool{false, true} {
		name := "complete_file_fanin"
		if conflicting {
			name = "divergent_file_heads"
		}
		t.Run(name, func(t *testing.T) {
			e, projectID, repo := testEngine(t)
			ctx := context.Background()
			staged, working := []byte("\ufeffStaged source 東京\r\n"), []byte("Working source café 🙂\r\n")
			if err := os.WriteFile(filepath.Join(repo, "source.txt"), staged, 0600); err != nil {
				t.Fatal(err)
			}
			interopGit(t, repo, "add", "source.txt")
			if err := os.WriteFile(filepath.Join(repo, "source.txt"), working, 0600); err != nil {
				t.Fatal(err)
			}
			index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
			if err != nil {
				t.Fatal(err)
			}
			scenarios := []string{"graph-branch-a", "graph-branch-b", "graph-join"}
			if conflicting {
				scenarios = []string{"graph-diverge-a", "graph-diverge-b", "graph-response"}
			}
			nodes := []domain.GraphNodeInput{}
			for i, key := range []string{"a", "b", "c"} {
				harness := "codex"
				if key == "b" {
					harness = "claude-code"
				}
				nodes = append(nodes, domain.GraphNodeInput{Key: key, OriginalText: "Preserve all original source and staging; author the controlled result for " + key + " and independently inspect its full candidate.",
					FakeScenario: scenarios[i], Pins: &domain.ExecutionPins{HarnessID: harness}, Workflow: &domain.WorkflowInput{Review: "required"},
					Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect all inherited and newly authored files with their exact source conditions."}})
			}
			edges := []domain.GraphDependencyInput{{From: "a", To: "c", Kind: "files"}, {From: "b", To: "c", Kind: "files"}}
			created := e.createGraph(ctx, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: nodes, Dependencies: &edges,
				Routing: &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "6"}}})
			if created.Status != 201 || created.Envelope.Error != nil {
				t.Fatal(created)
			}
			group := created.Envelope.Data.(domain.GraphAdmission)
			deadline := time.Now().Add(30 * time.Second)
			var view domain.GraphDetail
			var receiver domain.GraphNodeSummary
			var input domain.GraphInputDetail
			for {
				view, err = e.db.Graph(ctx, group.Graph.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, node := range view.Nodes {
					if node.Key == "c" {
						receiver = node
					}
				}
				if receiver.InputID != nil {
					input, err = e.db.GraphInput(ctx, group.Graph.ID, *receiver.InputID)
					if err != nil {
						t.Fatal(err)
					}
				}
				if !conflicting && view.Graph.State == "completed" || conflicting && input.Summary.State == "conflicted" {
					break
				}
				gate, gateErr := e.db.Gate(ctx)
				if gateErr != nil || gate.State != "open" || view.Graph.State == "failed" || view.Graph.State == "interrupted" || view.Graph.State == "cancelled" || time.Now().After(deadline) {
					t.Fatalf("Actual file fan-in did not reach its declared outcome: %s", graphActorDiagnostics(e, group.Graph.ID))
				}
				time.Sleep(15 * time.Millisecond)
			}
			// Input and graph are separate observations. Refresh the aggregate after
			// the terminal input boundary before asserting released ownership.
			view, err = e.db.Graph(ctx, group.Graph.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, node := range view.Nodes {
				if node.Key == "c" {
					receiver = node
				}
			}
			accepted := map[string]domain.GraphNodeSummary{}
			for _, node := range view.Nodes {
				if node.Key == "c" {
					continue
				}
				if node.State != "completed" || node.AcceptedResult == nil || node.AcceptedResult.AcceptanceKind != "workflow" || node.AcceptedResult.CandidateID == nil {
					t.Fatal("A file predecessor was inferred instead of actually reviewed and accepted", node)
				}
				producer, err := e.db.Attempt(ctx, node.AcceptedResult.ProducerAttemptID)
				if err != nil || producer.Role != "author" || producer.StartedAt == nil || producer.Cleanup.State != "verified" || producer.CandidateID == nil || *producer.CandidateID != *node.AcceptedResult.CandidateID {
					t.Fatal("The predecessor did not retain its actual authored producer", producer, err)
				}
				accepted[node.ID] = node
			}
			if len(accepted) != 2 || receiver.InputID == nil {
				t.Fatal("The declared two-source input was incomplete", view)
			}
			if conflicting {
				if input.Summary.Current || input.InputBinding != nil || input.ConflictArtifact == nil || receiver.NodeAdmissionID != nil || receiver.LatestAttemptID != nil || receiver.AcceptedResult != nil || receiver.State != "blocked" || view.Budget.Budget.ConsumedAdmissions != "4" {
					t.Fatal("Actual divergent accepted heads created a usable input, native call or successful result", input, receiver, view.Budget)
				}
				body, err := e.readGraphArtifact(ctx, *input.ConflictArtifact, "graph_input_provenance", 64<<20)
				var conflict struct {
					Contract        string                              `json:"contract"`
					GraphID         string                              `json:"graphId"`
					InputID         string                              `json:"inputId"`
					RootBasisSHA256 string                              `json:"rootBasisSha256"`
					Contributions   []workspace.GraphContributionSource `json:"contributions"`
					Conflicts       []workspace.InputConflict           `json:"conflicts"`
				}
				if err != nil || json.Unmarshal(body, &conflict, json.RejectUnknownMembers(true)) != nil || conflict.Contract != "006-input-conflict-v1" || conflict.GraphID != group.Graph.ID || conflict.InputID != input.Summary.ID || len(conflict.Contributions) != 2 || len(conflict.Conflicts) != 1 {
					t.Fatal("The real causal conflict was not retained as complete attributable evidence", conflict, err)
				}
				c := conflict.Conflicts[0]
				if c.Code != "input_conflict" || c.Dimension != "working" || c.Path != fixture.GraphConflictFile || len(c.ContributionRefs) != 2 || c.InputID != input.Summary.ID {
					t.Fatal("The retained conflict did not identify both divergent working heads", c)
				}
				for _, source := range conflict.Contributions {
					node, found := accepted[source.Binding.FromNodeID]
					if !found || source.Binding.AcceptanceID != node.AcceptedResult.AcceptanceID || source.Binding.ProducerAttemptID != node.AcceptedResult.ProducerAttemptID {
						t.Fatal("The conflict promoted an unaccepted or supporting output", source)
					}
				}
				terminal, err := e.db.Attempt(ctx, input.Summary.OperationAttemptID)
				if err != nil || terminal.Phase != "settled" || terminal.Outcome == nil || *terminal.Outcome == "succeeded" || terminal.SourceSnapshotID != nil || terminal.Cleanup.State != "verified" || terminal.LaunchIntent || len(view.ActiveClaims) != 0 {
					t.Fatal("Known causal conflict lost settled effects or invented a source/process", terminal, err)
				}
			} else {
				if input.InputBinding == nil || len(input.InputBinding.Contributions) != 2 || len(input.InputBinding.ContributorVector) != 2 || input.ContextPacket == nil || receiver.AcceptedResult == nil || view.Graph.Counts.Completed != "3" || view.Budget.Budget.ConsumedAdmissions != "6" {
					t.Fatal("Actual accepted files did not reach the full reviewed receiving result", view, input)
				}
				for _, contribution := range input.InputBinding.Contributions {
					node, found := accepted[contribution.FromNodeID]
					if !found || contribution.EdgeKind != "files" || contribution.AcceptanceID != node.AcceptedResult.AcceptanceID || contribution.ProducerAttemptID != node.AcceptedResult.ProducerAttemptID || contribution.CandidateBinding == nil {
						t.Fatal("The actual input lost exact accepted file provenance", contribution)
					}
				}
				membersResult := e.readGraph(ctx, "graphs.inputs.members", group.Graph.ID+"/"+input.Summary.ID, map[string]string{"limit": "100"})
				if membersResult.Status != 200 || membersResult.Envelope.Error != nil {
					t.Fatal("Actual composed file members are unavailable", membersResult)
				}
				members, ok := membersResult.Envelope.Data.(domain.Page[domain.GraphMemberView])
				if !ok || members.NextCursor != nil {
					t.Fatal("The small actual input member page was clipped or changed shape", membersResult)
				}
				foundFiles := 0
				for _, member := range members.Items {
					if member.Dimension != "working" || member.Path != fixture.GraphFileA && member.Path != fixture.GraphFileB {
						continue
					}
					expectedKey, expectedBody := "a", fixture.GraphContentA
					if member.Path == fixture.GraphFileB {
						expectedKey, expectedBody = "b", fixture.GraphContentB
					}
					var expected domain.GraphNodeSummary
					for _, node := range accepted {
						if node.Key == expectedKey {
							expected = node
						}
					}
					if member.ArtifactRef == nil || len(member.Origins) != 1 || member.Origins[0].Kind != "node" || member.Origins[0].NodeID == nil || *member.Origins[0].NodeID != expected.ID ||
						member.Origins[0].AcceptanceID == nil || *member.Origins[0].AcceptanceID != expected.AcceptedResult.AcceptanceID || member.Origins[0].ProducerAttemptID != expected.AcceptedResult.ProducerAttemptID {
						t.Fatal("Public composed member provenance lost the exact original accepted author", member)
					}
					producer, err := e.db.GraphDispatchForAttempt(ctx, expected.AcceptedResult.ProducerAttemptID)
					if err != nil || producer.CandidateClosure == nil || member.Origins[0].StateSHA256 != producer.CandidateClosure.ClosureSHA256 {
						t.Fatal("The public source-head seal was replaced by a path or envelope-file hash", member, err)
					}
					body, err := e.readGraphArtifact(ctx, *member.ArtifactRef, "source_file", 64<<20)
					if err != nil || string(body) != expectedBody {
						t.Fatal("The public composed member lost full retained raw bytes", member, err)
					}
					foundFiles++
				}
				if foundFiles != 2 {
					t.Fatal("Public input members omitted a complete accepted branch", members)
				}
				w, err := e.db.Workflow(ctx, receiver.TaskID, domain.WorkflowSelector{})
				if err != nil || w.Workflow == nil {
					t.Fatal(err)
				}
				subject, err := e.candidateWorkflowSubject(ctx, *w.Workflow, *receiver.AcceptedResult.CandidateID)
				if err != nil {
					t.Fatal(err)
				}
				for path, want := range map[string][]byte{"source.txt": working, fixture.GraphFileA: []byte(fixture.GraphContentA), fixture.GraphFileB: []byte(fixture.GraphContentB), fixture.GraphJoinedFile: []byte(fixture.GraphContentA + fixture.GraphContentB)} {
					entry := closureTestEntry(t, subject.Manifest, path)
					meta, err := e.evidence.VerifyClosed(ctx, entry.ArtifactID)
					if err != nil {
						t.Fatal(err)
					}
					body, err := e.readAllResponse(meta)
					if err != nil || !bytes.Equal(body, want) {
						t.Fatal("The actual join did not read and retain both complete predecessor files", path, err)
					}
				}
			}
			after, err := os.ReadFile(filepath.Join(repo, "source.txt"))
			afterIndex, indexErr := os.ReadFile(filepath.Join(repo, ".git", "index"))
			if err != nil || indexErr != nil || !bytes.Equal(after, working) || !bytes.Equal(afterIndex, index) {
				t.Fatal("Automatic accepted file composition changed user source or staging", err, indexErr)
			}
			for _, path := range []string{fixture.GraphFileA, fixture.GraphFileB, fixture.GraphJoinedFile, fixture.GraphConflictFile} {
				if _, err := os.Stat(filepath.Join(repo, path)); !os.IsNotExist(err) {
					t.Fatal("Separate graph results were published to the user's checkout", path, err)
				}
			}
		})
	}
}
