package store

import (
	"context"
	"os"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// These use registered SQLite workflow/output/acknowledgement transitions with
// typed controller proofs. They do not qualify a child process or file effects.
func TestGraphResponseFinalPureAcceptanceDoesNotFabricateFileDelivery(t *testing.T) {
	s, d, f, _ := graphOutputConfiguredFixture(t, true, func(in *domain.GraphStartInput) {
		in.Nodes = in.Nodes[:1]
		in.FinalNodeKey = in.Nodes[0].Key
		in.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "review_only"}
	})
	ctx := context.Background()
	w, err := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, f)
	if err != nil || w.NextAction != "response_acceptance_pending" || w.SelectedCandidateID != nil || w.SelectedResponseBinding == nil || w.Completion != nil {
		t.Fatal("The actual pure response did not await its exact user acknowledgement", w, err)
	}
	task, err := s.Task(ctx, d.Dispatch.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	binding := *f.ResponseBinding
	input := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: task.Task.Revision, TaskVersion: task.Version.Version,
		AttemptID: binding.ProducerAttemptID, ResponseArtifactID: binding.ResponseArtifactID, ResponseSHA256: binding.ResponseSHA256, Confirmation: "reviewed_response_and_conditions"}
	result := s.AcceptResponseWithWorkflow(ctx, d.Dispatch.TaskID, input, f.VerifiedArtifacts)
	if result.Status != 200 || result.Envelope.Error != nil {
		t.Fatal("The actual pure response acknowledgement was refused", result)
	}
	view, err := s.Graph(ctx, d.Dispatch.GraphID)
	if err != nil || len(view.Nodes) != 1 || view.Graph.State != "completed" || view.Graph.Counts.Completed != "1" || view.Nodes[0].AcceptedResult == nil {
		t.Fatal("The exact accepted pure response did not complete its all-required graph", view, err)
	}
	accepted := *view.Nodes[0].AcceptedResult
	if accepted.AcceptanceKind != "response" || accepted.ProducerAttemptID != d.Dispatch.AttemptID || accepted.InputID != d.Dispatch.InputID || accepted.CandidateID != nil ||
		accepted.ResponseArtifactID == nil || *accepted.ResponseArtifactID != binding.ResponseArtifactID {
		t.Fatal("Response completion invented file authority or selected another producer", accepted)
	}
	acknowledgement, err := s.GraphResponseAcceptance(ctx, view.Graph.ID, view.Nodes[0].ID, accepted.AcceptanceID)
	if err != nil || acknowledgement.RequestID != input.RequestID || acknowledgement.AttemptID != binding.ProducerAttemptID || acknowledgement.ResponseArtifactID != binding.ResponseArtifactID {
		t.Fatal("The final projection has no exact stored user response acknowledgement", acknowledgement, err)
	}
	selected, err := s.Workflow(ctx, d.Dispatch.TaskID, domain.WorkflowSelector{WorkflowID: d.Dispatch.WorkflowID})
	if err != nil || selected.Workflow == nil || selected.Workflow.State != "completed" || selected.Workflow.Completion == nil || selected.Workflow.Completion.ResponseAcceptanceID != acknowledgement.ID ||
		selected.Workflow.SelectedCandidateID != nil || selected.Workflow.HasTaskDelta || selected.Workflow.HasTaskIndexDelta {
		t.Fatal("Pure response acceptance manufactured a selected file delta", selected, err)
	}
	graphResponseNoDeliveryRows(t, s, d.Dispatch.WorkflowID)
	budget, err := s.GraphBudget(ctx, d.Dispatch.GraphID)
	if err != nil || budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("Pure user acknowledgement fabricated another model admission", budget, err)
	}
}

func TestGraphIntermediateEffectsRejectedBeforeGraphRegistration(t *testing.T) {
	for _, name := range []string{"local_application", "external_command"} {
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			input, facts, owner := graphTestStart(t, s)
			edges := []domain.GraphDependencyInput{{From: input.Nodes[0].Key, To: input.Nodes[1].Key, Kind: "files"}}
			input.Dependencies, input.FinalNodeKey = &edges, input.Nodes[1].Key
			delivery := domain.WorkflowDeliveryInput{LocalMode: "apply_verified_local"}
			if name == "external_command" {
				executable, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				delivery = domain.WorkflowDeliveryInput{LocalMode: "review_only", ExternalAction: &domain.ExternalActionInput{Action: "controlled metadata-only publication", Destination: "no external execution",
					AcceptanceBasis: "command_completion", Command: domain.ExternalCommandInput{Executable: executable, Arguments: []string{}}}}
			}
			if invalid := domain.ValidateWorkflowDeliveryInput(delivery); invalid != nil {
				t.Fatal("The standalone scope is invalid before testing its forbidden intermediate placement", invalid)
			}
			input.Nodes[0].Workflow = &domain.WorkflowInput{Delivery: &delivery}
			result := s.CreateGraph(context.Background(), owner, input, facts)
			if result.Status != 400 || result.Envelope.Error == nil || result.Envelope.Error.Code != "invalid_request" {
				t.Fatal("A contradictory intermediate effect declaration registered graph work", result)
			}
			for _, table := range []string{"task_graphs", "tasks", "attempts", "workflows", "artifacts", "capacity_claims", "delivery_operations"} {
				var count int
				if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
					t.Fatal("The rejected intermediate scope retained execution/effect authority", table, count, err)
				}
			}
		})
	}
}

func TestGraphResponseFinalMixedFilesRespectSavedDeliveryBoundary(t *testing.T) {
	for _, mode := range []string{"review_only", "apply_verified_local"} {
		t.Run(mode, func(t *testing.T) {
			s, d, response, _ := graphMixedResponseFinalFixture(t, mode)
			ctx := context.Background()
			w, err := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, response)
			if err != nil || w.SelectedResponseBinding == nil || w.SelectedCandidateID != nil || w.HasTaskDelta || w.HasTaskIndexDelta {
				t.Fatal("The actual response-only final did not preserve its own no-delta subject", w, err)
			}
			view, err := s.Graph(ctx, d.Dispatch.GraphID)
			if err != nil || len(view.Nodes) != 2 || view.Nodes[0].AcceptedResult == nil || view.Nodes[0].AcceptedResult.CandidateID == nil || view.Nodes[1].AcceptedResult != nil {
				t.Fatal("Mixed final setup lost its actual accepted file predecessor", view, err)
			}
			task, err := s.Task(ctx, d.Dispatch.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			binding := *response.ResponseBinding
			request := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: task.Task.Revision, TaskVersion: task.Version.Version,
				AttemptID: binding.ProducerAttemptID, ResponseArtifactID: binding.ResponseArtifactID, ResponseSHA256: binding.ResponseSHA256, Confirmation: "reviewed_response_and_conditions"}
			result := s.AcceptResponseWithWorkflow(ctx, d.Dispatch.TaskID, request, response.VerifiedArtifacts)
			if mode == "review_only" {
				if w.NextAction != "response_acceptance_pending" || result.Status != 200 || result.Envelope.Error != nil {
					t.Fatal("Separate qualified files plus an actual response could not complete review-only delivery", w, result)
				}
				after, err := s.Graph(ctx, view.Graph.ID)
				if err != nil || after.Graph.State != "completed" || after.Graph.Counts.Completed != "2" || after.Nodes[1].AcceptedResult == nil ||
					after.Nodes[1].AcceptedResult.AcceptanceKind != "response" || after.Nodes[1].AcceptedResult.CandidateID != nil {
					t.Fatal("Review-only mixed results invented one assembled file acceptance or failed to complete", after, err)
				}
			} else {
				if result.Status == 200 || result.Envelope.Error == nil {
					t.Fatal("A response-only summary acknowledged required cumulative file application", w, result)
				}
				after, err := s.Graph(ctx, view.Graph.ID)
				var acknowledgements int
				if err != nil || after.Graph.State == "completed" || after.Nodes[1].AcceptedResult != nil ||
					s.db.QueryRow("SELECT count(*) FROM response_acceptances WHERE attempt_id=?", d.Dispatch.AttemptID).Scan(&acknowledgements) != nil || acknowledgements != 0 {
					t.Fatal("Refused mixed file delivery partly accepted a response or completed the graph", after, acknowledgements, err)
				}
			}
			graphResponseNoDeliveryRows(t, s, d.Dispatch.WorkflowID)
			budget, err := s.GraphBudget(ctx, view.Graph.ID)
			if err != nil || budget.Budget.ConsumedAdmissions != "3" {
				t.Fatal("Response acknowledgement changed the actual author/review/author model count", budget, err)
			}
		})
	}
}

// Accept A through its real author and required review, then receive that exact
// file contribution in B. All artifact contents here are labelled SQL facts;
// Engine/workspace journeys separately qualify actual inherited file bytes.
func graphMixedResponseFinalFixture(t *testing.T, mode string) (*Store, domain.GraphDispatchView, domain.WorkflowStepFacts, string) {
	t.Helper()
	ctx := context.Background()
	s, graph, root, rootProofs, owner := graphReadyBasis(t, func(in *domain.GraphStartInput) {
		in.FinalNodeKey = in.Nodes[1].Key
		in.Delivery = &domain.WorkflowDeliveryInput{LocalMode: mode}
		in.Nodes[0].Acceptance = &domain.AcceptanceInput{Deliverable: "candidate"}
		in.Nodes[1].Acceptance = &domain.AcceptanceInput{Deliverable: "response_only"}
		edges := []domain.GraphDependencyInput{{From: in.Nodes[0].Key, To: in.Nodes[1].Key, Kind: "files"}}
		in.Dependencies = &edges
	})
	authorInput, authorTrigger := graphReadyNodeFacts(t, s, graph, root, rootProofs, owner, graph.Nodes[0])
	// Capture the legitimate intermediate scope before admission, rather than
	// borrowing the graph's designated-final application permission.
	authorInput.Capture.Authority.Delivery.LocalMode = "review_only"
	authorInput.Capture.Authority.Delivery.ExternalAction = nil
	authorInput.Capture.Authority.Delivery.Source = "graph_scope"
	authorInput.Capture.Authority.Delivery.PreferenceRevision = ""
	producer, err := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, authorTrigger, authorInput)
	if err != nil {
		t.Fatal("The captured intermediate file author could not be admitted", err)
	}
	mat := graphMaterializationForDispatch(t, s, producer, authorInput, root, owner)
	author := graphOutputForDispatch(t, s, producer, mat, owner, false)
	if _, err = s.SettleWorkflowStep(ctx, producer.Dispatch.AttemptID, author); err != nil {
		t.Fatal(err)
	}
	reviewer, reviewFacts, reviewed := graphAcceptanceApprovedReview(t, s, producer, author, owner)
	task, err := s.Task(ctx, producer.Dispatch.TaskID)
	if err != nil || reviewed.NextAction != "accept" || !reviewed.HasTaskDelta || len(reviewed.EvidenceUses) != 1 {
		t.Fatal("The actual file review did not qualify the current intermediate", reviewed, err)
	}
	authority, invalid := domain.WorkflowAuthoritySHA256(reviewed.Authority)
	if invalid != nil {
		t.Fatal(invalid)
	}
	requestID := domain.NewID()
	acceptance := domain.WorkflowAcceptance{ID: domain.NewID(), WorkflowID: reviewed.ID, TaskID: reviewed.TaskID, TaskVersion: reviewed.TaskVersion,
		CandidateBinding: *author.CandidateBinding, AuthoritySHA256: authority, ReviewUses: reviewed.EvidenceUses, CheckUses: []domain.EvidenceUse{},
		FindingResolutionIDs: []string{}, ExternalResultRefs: []string{}, DeliveryResult: "review_only", AcceptedAt: domain.Now(), AcceptedByRequestID: requestID}
	result := s.AcceptWorkflow(ctx, task.Task.ID, domain.WorkflowAcceptanceFacts{RequestID: requestID, ExpectedTaskRevision: task.Task.Revision,
		ExpectedWorkflowRevision: reviewed.Revision, OwnerInstanceID: owner, Acceptance: acceptance, VerifiedArtifacts: reviewFacts.VerifiedArtifacts})
	if result.Status != 200 {
		t.Fatal("The actual reviewed file predecessor was not accepted", result)
	}
	view, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil || view.Nodes[0].AcceptedResult == nil || view.Nodes[0].AcceptedResult.AcceptanceID != acceptance.ID {
		t.Fatal("No exact accepted file predecessor was projected", view, err)
	}
	begin := graphResponseCandidateDependency(t, s, view, root, rootProofs, author, reviewFacts, reviewer.Dispatch.AttemptID, owner)
	op, err := s.BeginGraphInput(ctx, owner, view.Graph.ID, view.Nodes[1].ID, begin)
	if err != nil {
		t.Fatal("The actual accepted file contribution could not begin B input", err)
	}
	if _, err = s.ClaimAttempt(ctx, op.Attempt.ID, owner); err != nil {
		t.Fatal(err)
	}
	completion := graphResponseCompletionForOperation(t, s, op, begin)
	if err = s.CompleteGraphInput(ctx, owner, op.Attempt.ID, completion); err != nil {
		t.Fatal("The complete exact file-dependent input was not retained", err)
	}
	input, trigger := graphNodeFactsForInput(t, s, op, completion)
	receiver, err := s.Task(ctx, view.Nodes[1].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	options := input.Capture.Authority.OptionSnapshot
	options.Delivery = &domain.WorkflowDeliveryInput{LocalMode: mode}
	registry, invalid := workflowpolicy.Conditions(receiver.Task.OriginalText, receiver.Version, options, input.Capture.Authority.Checks)
	if invalid != nil {
		t.Fatal(invalid)
	}
	instruction, nodeConditions, err := workflowpolicy.ConditionDigests(receiver.Task.OriginalText, receiver.Version, registry, input.Capture.Authority.Checks)
	if err != nil {
		t.Fatal(err)
	}
	input.Capture.Authority.OptionSnapshot, input.Capture.Authority.Conditions = options, registry
	input.Capture.Authority.InstructionSHA256 = instruction
	input.Capture.Authority.ConditionsSHA256 = graphEffectiveWorkflowConditions(nodeConditions, input.ConditionsBinding.ConditionsSHA256)
	final, err := s.AdmitGraphNode(ctx, owner, view.Graph.ID, trigger, input)
	if err != nil {
		t.Fatal("The actual response-only file receiver could not be admitted", err)
	}
	if len(final.Dispatch.ContributorVector) != 1 || final.Dispatch.ContributorVector[0].CandidateBinding == nil || final.Dispatch.ContributorVector[0].AcceptanceID != acceptance.ID {
		t.Fatal("The actual final admission lost its accepted file ancestry", final)
	}
	receiving := graphMaterializationForDispatch(t, s, final, input, root, owner)
	response := graphOutputForDispatch(t, s, final, receiving, owner, true)
	return s, final, response, owner
}

func graphResponseCandidateDependency(t *testing.T, s *Store, view domain.GraphDetail, root domain.GraphRootBinding, rootProofs []domain.VerifiedResponse,
	author, review domain.WorkflowStepFacts, completionAttemptID, owner string) domain.GraphInputPreparationFacts {
	t.Helper()
	source, receiver := view.Nodes[0], view.Nodes[1]
	accepted := *source.AcceptedResult
	edgeID, err := domain.GraphEdgeID(view.Graph.ID, view.Graph.GraphVersion, source.ID, receiver.ID, "files")
	if err != nil {
		t.Fatal(err)
	}
	closure := *author.GraphOutput.Observation.Candidate.ClosureRef
	contribution := domain.AcceptedContribution{EdgeID: edgeID, EdgeKind: "files", FromNodeID: source.ID, TaskID: source.TaskID, TaskVersion: source.TaskVersion,
		AcceptanceKind: accepted.AcceptanceKind, AcceptanceID: accepted.AcceptanceID, ProducerAttemptID: accepted.ProducerAttemptID, InputID: accepted.InputID,
		CandidateBinding: author.CandidateBinding, OutputClosureRef: closure, ConstraintRefs: []domain.GraphScopedReference{}, EvidenceRefs: []domain.GraphContributedEvidence{}}
	proofs := append(append([]domain.VerifiedResponse{}, rootProofs...), review.VerifiedArtifacts...)
	upstream, err := s.GraphInputRecord(context.Background(), view.Graph.ID, accepted.InputID)
	if err != nil || upstream.ConditionsBinding == nil || upstream.Public.ConditionsRef == nil {
		t.Fatal(upstream, err)
	}
	add := func(name string, ref domain.GraphArtifactRef) {
		contribution.ConstraintRefs = append(contribution.ConstraintRefs, domain.GraphScopedReference{Ref: name, ArtifactRef: ref, AuthorityScope: "upstream_local", NodeID: &source.ID, TaskVersion: &source.TaskVersion, Paths: []string{}})
		record, err := s.Artifact(context.Background(), ref.ArtifactID)
		if err != nil || record.SHA256 == nil {
			t.Fatal(record, err)
		}
		proofs = append(proofs, domain.VerifiedResponse{ArtifactID: record.ID, SHA256: *record.SHA256, ByteLength: record.ByteLength, PhysicalIdentity: record.PhysicalIdentity})
	}
	for _, condition := range upstream.ConditionsBinding.Sources {
		add("up-"+condition.Ref+"-original", condition.OriginalRef)
		if condition.GuidanceRef != nil {
			add("up-"+condition.Ref+"-guidance", *condition.GuidanceRef)
		}
		if condition.AcceptanceRef != nil {
			add("up-"+condition.Ref+"-acceptance", *condition.AcceptanceRef)
		}
	}
	add("up-conditions", *upstream.Public.ConditionsRef)
	for _, item := range []struct {
		name string
		ref  domain.GraphArtifactRef
	}{{"result", author.GraphOutput.CandidateClosure.Manifest}, {"closure", closure}} {
		contribution.EvidenceRefs = append(contribution.EvidenceRefs, domain.GraphContributedEvidence{Ref: "edge-" + edgeID + "-" + item.name, EdgeID: edgeID,
			SourceNodeID: source.ID, AcceptanceKind: accepted.AcceptanceKind, AcceptanceID: accepted.AcceptanceID, ProducerAttemptID: accepted.ProducerAttemptID,
			ResultKind: "candidate", ResultID: author.CandidateBinding.CandidateID, ArtifactRef: item.ref, Scope: "whole_candidate", Paths: []string{}})
	}
	vector := []domain.GraphContributorBinding{{NodeID: source.ID, TaskID: source.TaskID, TaskVersion: source.TaskVersion, AcceptanceKind: accepted.AcceptanceKind,
		AcceptanceID: accepted.AcceptanceID, ProducerAttemptID: accepted.ProducerAttemptID, InputID: accepted.InputID, CandidateBinding: author.CandidateBinding, OutputClosureRef: closure}}
	facts := graphBeginFacts(domain.GraphAdmission{Graph: view.Graph, Nodes: view.Nodes, PreparationAttemptID: root.OperationAttemptID}, receiver, root, proofs, owner)
	facts.Control.GraphControlRevision, facts.Control.BudgetAuthorityEpoch, facts.Control.WindowNumber = view.Graph.ControlRevision, view.Graph.BudgetAuthorityEpoch, view.Budget.Budget.CurrentWindow.Number
	facts.Control.Generation, facts.Trigger.OwnerGeneration = view.DispatchAuthority.Generation, view.DispatchAuthority.Generation
	facts.Trigger.Kind, facts.Trigger.ID = "completion", completionAttemptID
	facts.Control.OperationAttemptID = completionAttemptID
	facts.Contributions, facts.ContributorVector, facts.Control.ContributorVector = []domain.AcceptedContribution{contribution}, vector, vector
	return facts
}

func graphResponseCompletionForOperation(t *testing.T, s *Store, op domain.GraphInputOperation, begin domain.GraphInputPreparationFacts) domain.GraphInputCompletionFacts {
	t.Helper()
	ctx := context.Background()
	view, e := s.Graph(ctx, op.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	node := domain.GraphNodeSummary{}
	for _, current := range view.Nodes {
		if op.Control.NodeID != nil && current.ID == *op.Control.NodeID {
			node = current
		}
	}
	if node.ID == "" || len(begin.ContributorVector) != 1 {
		t.Fatal("The actual mixed dependency operation has no exact receiver/source")
	}
	task, e := s.Task(ctx, node.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	sharedText := ""
	if op.Definition.Original.SharedInstructions != nil {
		sharedText = *op.Definition.Original.SharedInstructions
	}
	shared, _ := graphPayload(t, s, op.Attempt.ID, "graph_original", []byte(sharedText))
	original, _ := graphPayload(t, s, op.Attempt.ID, "graph_original", []byte(task.Task.OriginalText))
	acceptance, _ := graphPayload(t, s, op.Attempt.ID, "graph_acceptance", []byte(encode(task.Version.Acceptance)))
	upstream, e := s.GraphInputRecord(ctx, view.Graph.ID, begin.ContributorVector[0].InputID)
	if e != nil || upstream.ConditionsBinding == nil {
		t.Fatal(upstream, e)
	}
	var source domain.GraphConditionSource
	for _, item := range upstream.ConditionsBinding.Sources {
		if item.AuthorityScope == "receiver" {
			source = item
		}
	}
	source.Ref, source.AuthorityScope = "upstream-node", "upstream_local"
	sources := []domain.GraphConditionSource{{Ref: "graph-instructions", AuthorityScope: "graph_shared", OriginalRef: shared}, {Ref: "node-instructions", AuthorityScope: "receiver", NodeID: &node.ID, TaskVersion: &node.TaskVersion, OriginalRef: original, AcceptanceRef: &acceptance}, source}
	conditions := domain.GraphConditionsBinding{GraphID: view.Graph.ID, GraphVersion: "1", ReceiverTaskID: node.TaskID, ReceiverTaskVersion: node.TaskVersion, Sources: sources}
	conditions.ConditionsSHA256, e = domain.GraphConditionsDigest(conditions)
	if e != nil {
		t.Fatal(e)
	}
	conditionRef, conditionProof := graphPayload(t, s, op.Attempt.ID, "graph_conditions", []byte(encode(conditions)))
	manifestBody := []byte(`{"controlled":"assembled input/source metadata"}`)
	manifest, mp := graphPayload(t, s, op.Attempt.ID, "graph_input_manifest", manifestBody)
	index, ip := graphPayload(t, s, op.Attempt.ID, "index_snapshot", []byte("assembled raw index metadata"))
	blobs, bp := graphPayload(t, s, op.Attempt.ID, "graph_index_blobs", []byte("assembled staged closure metadata"))
	provenance, pp := graphPayload(t, s, op.Attempt.ID, "graph_input_provenance", []byte("complete causal metadata"))
	input := domain.GraphInputBinding{ID: op.ID, GraphID: op.GraphID, GraphVersion: "1", RootBasisSHA256: begin.RootBinding.BasisSHA256, OperationAttemptID: op.Attempt.ID, ReceivingNodeID: node.ID, ReceivingTaskID: node.TaskID, ReceivingTaskVersion: node.TaskVersion, Contributions: op.Contributions, ContributorVector: op.ContributorVector, Manifest: manifest, Index: index, StagedBlobs: blobs, Provenance: provenance}
	input.InputSHA256, e = domain.GraphInputDigest(input)
	if e != nil {
		t.Fatal(e)
	}
	packet := domain.GraphContextPacket{ID: domain.NewID(), GraphID: op.GraphID, InputID: op.ID, InputSHA256: input.InputSHA256, ReceivingTaskVersion: node.TaskVersion, GraphInstructionsRef: shared, NodeInstructionsRef: original, AcceptanceRef: &acceptance, ConstraintRefs: []domain.GraphScopedReference{{Ref: "applicable-conditions", ArtifactRef: conditionRef, AuthorityScope: "receiver", NodeID: &node.ID, TaskVersion: &node.TaskVersion, Paths: []string{}}}, ContributedEvidence: []domain.GraphContributedEvidence{}, ProvidedAccess: []domain.GraphProvidedAccess{{Ref: "graph-instructions", ArtifactRef: shared, Scope: "graph_shared", Paths: []string{}, Mode: "inline"}, {Ref: "node-instructions", ArtifactRef: original, Scope: "receiver", Paths: []string{}, Mode: "inline"}, {Ref: "receiver-acceptance", ArtifactRef: acceptance, Scope: "receiver", Paths: []string{}, Mode: "inline"}, {Ref: "applicable-conditions", ArtifactRef: conditionRef, Scope: "receiver", Paths: []string{}, Mode: "inline"}}}
	for _, c := range op.Contributions {
		packet.ConstraintRefs = append(packet.ConstraintRefs, c.ConstraintRefs...)
		packet.ContributedEvidence = append(packet.ContributedEvidence, c.EvidenceRefs...)
		for _, item := range c.ConstraintRefs {
			packet.ProvidedAccess = append(packet.ProvidedAccess, domain.GraphProvidedAccess{Ref: item.Ref, ArtifactRef: item.ArtifactRef, Scope: item.AuthorityScope, Paths: item.Paths, Mode: "inline"})
		}
		for _, item := range c.EvidenceRefs {
			packet.ProvidedAccess = append(packet.ProvidedAccess, domain.GraphProvidedAccess{Ref: item.Ref, ArtifactRef: item.ArtifactRef, Scope: item.Scope, Paths: item.Paths, Mode: "inline"})
		}
	}
	packet.PacketSHA256, e = domain.GraphContextDigest(packet)
	if e != nil {
		t.Fatal(e)
	}
	packetRef, cp := graphPayload(t, s, op.Attempt.ID, "graph_context", []byte(encode(packet)))
	input.ContextPacket = packetRef
	sourceManifest, sp := graphPayload(t, s, op.Attempt.ID, "source_manifest", manifestBody)
	snapshot := domain.SourceSnapshot{ID: domain.NewID(), AttemptID: op.Attempt.ID, ManifestArtifactID: sourceManifest.ArtifactID, IndexArtifactID: index.ArtifactID, ManifestSHA256: sourceManifest.SHA256, ProjectIdentity: begin.RootBinding.ProjectIdentity, BaseCommit: begin.RootBinding.BaseCommit, Branch: begin.RootBinding.Branch, SelectedRelativePath: begin.RootBinding.SelectedRelativePath, CreatedAt: domain.Now()}
	return domain.GraphInputCompletionFacts{InputBinding: &input, SourceSnapshot: &snapshot, ConditionsRef: &conditionRef, ConditionsBinding: &conditions, ContextPacket: &packet, EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, State: "ready", VerifiedArtifacts: []domain.VerifiedResponse{mp, ip, bp, pp, cp, sp, conditionProof}}
}

func graphResponseNoDeliveryRows(t *testing.T, s *Store, workflowID string) {
	t.Helper()
	for _, table := range []string{"delivery_operations", "workflow_acceptances"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM "+table+" WHERE workflow_id=?", workflowID).Scan(&count); err != nil || count != 0 {
			t.Fatal("Response acceptance invented a file delivery or candidate acceptance", table, count, err)
		}
	}
	var effects int
	if err := s.db.QueryRow("SELECT count(*) FROM workflow_steps WHERE workflow_id=? AND role IN ('local_delivery','external_delivery')", workflowID).Scan(&effects); err != nil || effects != 0 {
		t.Fatal("Response acknowledgement admitted an undeclared effectful stage", effects, err)
	}
}
