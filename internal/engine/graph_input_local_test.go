package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

func TestGraphNodeInputRetainsIndependentExactOriginalsAndOneCapturedRoot(t *testing.T) {
	for _, shared := range []string{"\ufeffShared exact constraint 東京\r\nDo not publish externally.\r\n", ""} {
		name := "shared-conditions"
		if shared == "" {
			name = "empty-shared-cheap-proof-path"
		}
		t.Run(name, func(t *testing.T) {
			e, projectID, repo := testEngine(t)
			ctx := context.Background()
			dirty := []byte("\xef\xbb\xbfcaptured once before local inputs 東京\r\n")
			if err := os.WriteFile(filepath.Join(repo, "captured-dirty.txt"), dirty, 0600); err != nil {
				t.Fatal(err)
			}
			beforeIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
			if err != nil {
				t.Fatal(err)
			}
			conditions := "Keep the complete receiver acceptance 🙂\r\n"
			in := domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", SharedInstructions: &shared, Nodes: []domain.GraphNodeInput{
				{Key: "a", OriginalText: "Receiver A complete original 東京\r\n", Acceptance: &domain.AcceptanceInput{ConditionsText: conditions}},
				{Key: "b", OriginalText: "Receiver B private complete original 🙂\r\n"},
			}}
			createFacts, err := e.captureGraphCreateFacts(ctx, projectID)
			if err != nil {
				t.Fatal(err)
			}
			var admission domain.GraphAdmission
			var basis domain.Attempt
			func() {
				e.control.Lock()
				defer e.control.Unlock()
				r := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, in, createFacts)
				if r.Envelope.Error != nil {
					t.Fatal(r)
				}
				admission = r.Envelope.Data.(domain.GraphAdmission)
				basis, err = e.db.ClaimAttempt(ctx, admission.PreparationAttemptID, e.config.OwnerInstanceID)
				if err != nil {
					t.Fatal(err)
				}
			}()
			e.run(ctx, basis)
			rootRecord, err := e.db.GraphInputRecord(ctx, admission.Graph.ID, *admission.Graph.RootInputID)
			if err != nil || rootRecord.PrivateRoot == nil || rootRecord.Public.Summary.State != "ready" {
				t.Fatal("Initial real basis unavailable", rootRecord, err)
			}
			definition, err := e.db.GraphDefinitionPrivate(ctx, admission.Graph.ID)
			if err != nil {
				t.Fatal(err)
			}
			source, err := e.graphRootSourceSnapshot(ctx, *rootRecord.PrivateRoot, definition.Delivery)
			if err != nil {
				t.Fatal(err)
			}
			// Independent inputs must reuse the retained basis, even if the user's live
			// working tree receives a later untracked file before their local preparation.
			if err := os.WriteFile(filepath.Join(repo, "later-live-file.txt"), []byte("This is not part of the captured graph basis\n"), 0600); err != nil {
				t.Fatal(err)
			}
			for name, body := range map[string]string{"package.json": `{"scripts":{"test":"node ignored.mjs"}}`, "ignored.mjs": "process.exit(0)\n"} {
				if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			preference, err := e.db.DeliveryPreference(ctx)
			if err != nil {
				t.Fatal(err)
			}
			changedPreference := e.db.UpdateDeliveryPreference(ctx, domain.DeliveryPreferenceInput{RequestID: domain.NewID(), ExpectedRevision: preference.Revision, LocalMode: "apply_verified_local"})
			if changedPreference.Envelope.Error != nil {
				t.Fatal(changedPreference)
			}
			manifestIDs := map[string]bool{}
			for ordinal, node := range admission.Nodes {
				view, err := e.db.Graph(ctx, admission.Graph.ID)
				if err != nil || view.DispatchAuthority == nil {
					t.Fatal(view, err)
				}
				proofs, err := e.verifyGraphRootClosure(ctx, *rootRecord.PrivateRoot, source)
				if err != nil {
					t.Fatal(err)
				}
				nodeID, taskID, version, controlRevision := node.ID, node.TaskID, node.TaskVersion, node.ControlRevision
				facts := domain.GraphInputPreparationFacts{Trigger: domain.GraphFreshTrigger{Kind: "completion", ID: basis.ID, OwnerGeneration: view.DispatchAuthority.Generation}, RootBinding: *rootRecord.PrivateRoot,
					Control:       domain.GraphDispatchBinding{GraphID: admission.Graph.ID, GraphVersion: view.Graph.GraphVersion, RootTaskID: admission.Graph.RootTaskID, OperationAttemptID: basis.ID, OwnerInstanceID: e.config.OwnerInstanceID, Generation: view.DispatchAuthority.Generation, GraphControlRevision: view.Graph.ControlRevision, BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, WindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeID: &nodeID, TaskID: &taskID, TaskVersion: &version, NodeControlRevision: &controlRevision, ContributorVector: []domain.GraphContributorBinding{}},
					Contributions: []domain.AcceptedContribution{}, ContributorVector: []domain.GraphContributorBinding{}, VerifiedArtifacts: proofs}
				var operation domain.GraphInputOperation
				var attempt domain.Attempt
				func() {
					e.control.Lock()
					defer e.control.Unlock()
					operation, err = e.db.BeginGraphInput(ctx, e.config.OwnerInstanceID, admission.Graph.ID, node.ID, facts)
					if err != nil {
						t.Fatal("A current independent receiver input could not be registered", err)
					}
					attempt, err = e.db.ClaimAttempt(ctx, operation.Attempt.ID, e.config.OwnerInstanceID)
					if err != nil {
						t.Fatal(err)
					}
				}()
				e.run(ctx, attempt)
				retained, err := e.db.GraphInput(ctx, admission.Graph.ID, operation.ID)
				if err != nil || retained.Summary.State != "ready" || retained.InputBinding == nil || retained.ContextPacket == nil || retained.ConditionsRef == nil {
					t.Fatal("A real independent local input did not become ready", retained, err)
				}
				input, packet := *retained.InputBinding, *retained.ContextPacket
				if input.RootBasisSHA256 != rootRecord.PrivateRoot.BasisSHA256 || input.Manifest.ProducerAttemptID != attempt.ID || manifestIDs[input.Manifest.ArtifactID] || len(input.Contributions) != 0 {
					t.Fatal("Independent receiver lost its distinct closure or shared root", input)
				}
				manifestIDs[input.Manifest.ArtifactID] = true
				manifest, err := e.readGraphArtifact(ctx, input.Manifest, "graph_input_manifest", 64<<20)
				if err != nil || !bytes.Contains(manifest, []byte("captured-dirty.txt")) || bytes.Contains(manifest, []byte("later-live-file.txt")) {
					t.Fatal("A node recaptured live source or lost the original dirty basis", string(manifest), err)
				}
				sharedBytes, err := e.readGraphArtifact(ctx, packet.GraphInstructionsRef, "graph_original", 2<<20)
				if err != nil || string(sharedBytes) != shared {
					t.Fatal("Shared instructions broadcast unrelated originals or changed bytes", string(sharedBytes), err)
				}
				nodeBytes, err := e.readGraphArtifact(ctx, packet.NodeInstructionsRef, "graph_original", 2<<20)
				if err != nil || string(nodeBytes) != in.Nodes[ordinal].OriginalText {
					t.Fatal("Receiver original was changed or replaced by a sibling", string(nodeBytes), err)
				}
				detail, err := e.db.TaskVersion(ctx, taskID, version)
				if err != nil || packet.AcceptanceRef == nil {
					t.Fatal("Full acceptance source missing", err)
				}
				wantAcceptance, _ := json.Marshal(detail.Version.Acceptance)
				gotAcceptance, err := e.readGraphArtifact(ctx, *packet.AcceptanceRef, "graph_acceptance", 2<<20)
				if err != nil || !bytes.Equal(wantAcceptance, gotAcceptance) {
					t.Fatal("Resolved acceptance was clipped or summarized", err)
				}
				capture, scoped, err := e.captureGraphWorkflow(ctx, definition, node, detail, retained)
				if err != nil || capture == nil || capture.Authority.Delivery.LocalMode != "review_only" || capture.Authority.Delivery.ExternalAction != nil || capture.Authority.Delivery.Source != "graph_scope" || len(capture.Authority.Checks) != 0 {
					t.Fatal("Workflow capture read a new live marker or changed saved delivery scope", capture, err)
				}
				_, nodeOnly, err := workflowpolicy.ConditionDigests(detail.OriginalText, detail.Version, capture.Authority.Conditions, capture.Authority.Checks)
				if err != nil {
					t.Fatal(err)
				}
				effective, err := graphEffectiveConditionsDigest(nodeOnly, scoped.ConditionsSHA256)
				if err != nil || capture.Authority.ConditionsSHA256 != effective || capture.Authority.ConditionsSHA256 == nodeOnly {
					t.Fatal("Applicable shared/receiver conditions were not bound before workflow capture", err)
				}
				proofWorkflow := domain.Workflow{TaskID: detail.TaskID, TaskVersion: detail.Version.Version, Authority: capture.Authority}
				guardErr := e.graphDirectProofGuard(ctx, proofWorkflow, detail.OriginalText, detail.Version)
				if shared != "" && guardErr == nil {
					t.Fatal("Hashing opaque shared text was treated as a complete deterministic proof")
				}
				if shared == "" && guardErr != nil {
					t.Fatal("Empty shared instructions incorrectly disabled the narrow deterministic proof path", guardErr)
				}
				stale := retained
				stale.Summary.Current = false
				if capture, _, err := e.captureGraphWorkflow(ctx, definition, node, detail, stale); err == nil || capture != nil {
					t.Fatal("A historical input supplied current workflow authority", capture, err)
				}
				settled, err := e.db.Attempt(ctx, attempt.ID)
				if err != nil || settled.Outcome == nil || *settled.Outcome != "succeeded" || settled.LaunchIntent || settled.ResolvedTarget != nil {
					t.Fatal("Input retention became native work or did not settle", settled, err)
				}
			}
			budget, err := e.db.GraphBudget(ctx, admission.Graph.ID)
			if err != nil || budget.Budget.ConsumedAdmissions != "0" || budget.Budget.CurrentWindow.DeadlineAt != nil {
				t.Fatal("Local receiver input spent or opened native allowance", budget, err)
			}
			afterIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
			if err != nil || !bytes.Equal(beforeIndex, afterIndex) {
				t.Fatal("Input preparation changed the user's raw index", err)
			}
			afterDirty, err := os.ReadFile(filepath.Join(repo, "captured-dirty.txt"))
			if err != nil || !bytes.Equal(dirty, afterDirty) {
				t.Fatal("Input preparation changed the user's source", err)
			}
		})
	}
}
