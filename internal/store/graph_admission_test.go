package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// Metadata/proof fixtures exercise SQLite admission, not filesystem integrity or
// a real native model. Real workspace/controller qualification is separate.
func graphPayload(t *testing.T, s *Store, attempt, kind string, body []byte) (domain.GraphArtifactRef, domain.VerifiedResponse) {
	t.Helper()
	hash := sha256.Sum256(body)
	record := graphArtifactMetadata(attempt, kind)
	record.ByteLength = domain.Decimal(int64(len(body)))
	record.SHA256 = ptr(hex.EncodeToString(hash[:]))
	if e := s.PutArtifact(context.Background(), record); e != nil {
		t.Fatal(e)
	}
	return domain.GraphArtifactRef{ArtifactID: record.ID, ProducerAttemptID: attempt, SHA256: *record.SHA256, ByteLength: record.ByteLength}, domain.VerifiedResponse{ArtifactID: record.ID, SHA256: *record.SHA256, ByteLength: record.ByteLength, PhysicalIdentity: record.PhysicalIdentity}
}
func graphReadyNodeFacts(t *testing.T, s *Store, graph domain.GraphAdmission, root domain.GraphRootBinding, rootProofs []domain.VerifiedResponse, owner string, node domain.GraphNodeSummary, preparation ...domain.GraphInputPreparationFacts) (domain.GraphNodeAdmissionFacts, domain.GraphFreshTrigger) {
	t.Helper()
	ctx := context.Background()
	begin := graphBeginFacts(graph, node, root, rootProofs, owner)
	if len(preparation) != 0 {
		begin = preparation[0]
	}
	op, e := s.BeginGraphInput(ctx, owner, graph.Graph.ID, node.ID, begin)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ClaimAttempt(ctx, op.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	task, e := s.Task(ctx, node.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	shared := ""
	if op.Definition.Original.SharedInstructions != nil {
		shared = *op.Definition.Original.SharedInstructions
	}
	sharedRef, _ := graphPayload(t, s, op.Attempt.ID, "graph_original", []byte(shared))
	originalRef, _ := graphPayload(t, s, op.Attempt.ID, "graph_original", []byte(task.Task.OriginalText))
	acceptanceRef, _ := graphPayload(t, s, op.Attempt.ID, "graph_acceptance", []byte(encode(task.Version.Acceptance)))
	var guidanceRef *domain.GraphArtifactRef
	if task.Version.GuidanceText != "" {
		ref, _ := graphPayload(t, s, op.Attempt.ID, "graph_original", []byte(task.Version.GuidanceText))
		guidanceRef = &ref
	}
	conditions := domain.GraphConditionsBinding{GraphID: graph.Graph.ID, GraphVersion: "1", ReceiverTaskID: node.TaskID, ReceiverTaskVersion: node.TaskVersion, Sources: []domain.GraphConditionSource{{Ref: "graph-instructions", AuthorityScope: "graph_shared", OriginalRef: sharedRef}, {Ref: "node-instructions", AuthorityScope: "receiver", NodeID: &node.ID, TaskVersion: &node.TaskVersion, OriginalRef: originalRef, GuidanceRef: guidanceRef, AcceptanceRef: &acceptanceRef}}}
	conditions.ConditionsSHA256, e = domain.GraphConditionsDigest(conditions)
	if e != nil {
		t.Fatal(e)
	}
	conditionRef, conditionProof := graphPayload(t, s, op.Attempt.ID, "graph_conditions", []byte(encode(conditions)))
	manifest, manifestProof := graphPayload(t, s, op.Attempt.ID, "graph_input_manifest", []byte(`{"controlled":"input manifest metadata"}`))
	index, indexProof := graphPayload(t, s, op.Attempt.ID, "index_snapshot", []byte("controlled index metadata"))
	blobs, blobProof := graphPayload(t, s, op.Attempt.ID, "graph_index_blobs", []byte(`{"controlled":"staged closure metadata"}`))
	provenance, provenanceProof := graphPayload(t, s, op.Attempt.ID, "graph_input_provenance", []byte(`{"controlled":"root origins metadata"}`))
	input := domain.GraphInputBinding{ID: op.ID, GraphID: graph.Graph.ID, GraphVersion: "1", RootBasisSHA256: root.BasisSHA256, OperationAttemptID: op.Attempt.ID, ReceivingNodeID: node.ID, ReceivingTaskID: node.TaskID, ReceivingTaskVersion: node.TaskVersion, Contributions: []domain.AcceptedContribution{}, ContributorVector: []domain.GraphContributorBinding{}, Manifest: manifest, Index: index, StagedBlobs: blobs, Provenance: provenance}
	input.InputSHA256, e = domain.GraphInputDigest(input)
	if e != nil {
		t.Fatal(e)
	}
	packet := domain.GraphContextPacket{ID: domain.NewID(), GraphID: graph.Graph.ID, InputID: input.ID, InputSHA256: input.InputSHA256, ReceivingTaskVersion: node.TaskVersion, GraphInstructionsRef: sharedRef, NodeInstructionsRef: originalRef, GuidanceRef: guidanceRef, AcceptanceRef: &acceptanceRef, ConstraintRefs: []domain.GraphScopedReference{{Ref: "applicable-conditions", ArtifactRef: conditionRef, AuthorityScope: "receiver", NodeID: &node.ID, TaskVersion: &node.TaskVersion, Paths: []string{}}}, ContributedEvidence: []domain.GraphContributedEvidence{}, ProvidedAccess: []domain.GraphProvidedAccess{{Ref: "graph-instructions", ArtifactRef: sharedRef, Scope: "graph_shared", Paths: []string{}, Mode: "inline"}, {Ref: "node-instructions", ArtifactRef: originalRef, Scope: "receiver", Paths: []string{}, Mode: "inline"}, {Ref: "receiver-acceptance", ArtifactRef: acceptanceRef, Scope: "receiver", Paths: []string{}, Mode: "inline"}, {Ref: "applicable-conditions", ArtifactRef: conditionRef, Scope: "receiver", Paths: []string{}, Mode: "inline"}}}
	if guidanceRef != nil {
		packet.ProvidedAccess = append(packet.ProvidedAccess, domain.GraphProvidedAccess{Ref: "receiver-guidance", ArtifactRef: *guidanceRef, Scope: "receiver", Paths: []string{}, Mode: "inline"})
	}
	packet.PacketSHA256, e = domain.GraphContextDigest(packet)
	if e != nil {
		t.Fatal(e)
	}
	contextRef, contextProof := graphPayload(t, s, op.Attempt.ID, "graph_context", []byte(encode(packet)))
	input.ContextPacket = contextRef
	proofs := []domain.VerifiedResponse{manifestProof, indexProof, blobProof, provenanceProof, contextProof, conditionProof}
	if e = s.CompleteGraphInput(ctx, owner, op.Attempt.ID, domain.GraphInputCompletionFacts{InputBinding: &input, ConditionsRef: &conditionRef, ConditionsBinding: &conditions, ContextPacket: &packet, EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, State: "ready", VerifiedArtifacts: proofs}); e != nil {
		t.Fatal(e)
	}
	return graphNodeFactsForInput(t, s, op, domain.GraphInputCompletionFacts{InputBinding: &input, ConditionsRef: &conditionRef, ConditionsBinding: &conditions, VerifiedArtifacts: proofs})
}

func graphNodeFactsForInput(t *testing.T, s *Store, op domain.GraphInputOperation, completion domain.GraphInputCompletionFacts) (domain.GraphNodeAdmissionFacts, domain.GraphFreshTrigger) {
	t.Helper()
	ctx := context.Background()
	input, conditions, conditionRef := *completion.InputBinding, *completion.ConditionsBinding, *completion.ConditionsRef
	proofs := completion.VerifiedArtifacts
	node := domain.GraphNodeSummary{ID: input.ReceivingNodeID, TaskID: input.ReceivingTaskID, TaskVersion: input.ReceivingTaskVersion}
	task, e := s.Task(ctx, node.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	execution, e := s.GraphNodeOptions(ctx, input.GraphID, node.ID, node.TaskVersion)
	if e != nil {
		t.Fatal(e)
	}
	options := domain.WorkflowInput{Mode: "auto", Review: "auto"}
	if execution.Workflow != nil {
		options = *execution.Workflow
	}
	if options.Mode == "" {
		options.Mode = "auto"
	}
	if options.Review == "" {
		options.Review = "auto"
	}
	options.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "review_only"}
	registry, invalid := workflowpolicy.Conditions(task.Task.OriginalText, task.Version, options, nil)
	if invalid != nil {
		t.Fatal(invalid)
	}
	instruction, nodeConditions, e := workflowpolicy.ConditionDigests(task.Task.OriginalText, task.Version, registry, nil)
	if e != nil {
		t.Fatal(e)
	}
	effective := sha256.Sum256([]byte(encode(struct {
		Node  string `json:"nodeConditionsSha256"`
		Graph string `json:"graphConditionsSha256"`
	}{nodeConditions, conditions.ConditionsSHA256})))
	capture := &domain.WorkflowCapture{Authority: domain.WorkflowAuthority{ExecutionMode: execution.ExecutionMode, FakeScenario: execution.FakeScenario, OptionSnapshot: options, InstructionSHA256: instruction, ConditionsSHA256: hex.EncodeToString(effective[:]), Conditions: registry, Delivery: op.Definition.Delivery, Checks: []domain.CheckDefinition{}, Review: "auto", ReasonCodes: []string{"controlled_graph_capture"}}, Step: domain.WorkflowStep{Role: "author", ExecutorKind: "native", LogicalKey: "author:0", Cycle: 0, ReasonCodes: []string{"initial_graph_author"}}}
	control := op.Control
	facts := domain.GraphNodeAdmissionFacts{Control: control, NodeID: node.ID, TaskVersion: node.TaskVersion, InputBinding: input, SourceBasis: domain.GraphSourceBasis{BasisKind: "graph_input", InputID: input.ID, InputSHA256: input.InputSHA256, ExpectedManifest: input.Manifest, ExpectedIndex: input.Index, ExpectedWorkingEntriesSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpectedIndexEntriesSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, ConditionsBinding: conditions, ConditionsRef: conditionRef, Capture: capture, Routing: wrapperRoutingFacts("fake"), VerifiedArtifacts: proofs}
	return facts, domain.GraphFreshTrigger{Kind: "completion", ID: op.Attempt.ID, OwnerGeneration: op.Control.Generation}
}

func TestGraphNodeAdmissionReservesDistinctContextsAndOneBudget(t *testing.T) {
	s, graph, root, proofs, owner := graphReadyBasis(t)
	ctx := context.Background()
	a, triggerA := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[0])
	b, triggerB := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[1])
	if r := s.UpdateDeliveryPreference(ctx, domain.DeliveryPreferenceInput{RequestID: domain.NewID(), ExpectedRevision: "1", LocalMode: "apply_verified_local"}); r.Status != 200 {
		t.Fatal(r)
	}
	first, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, triggerA, a)
	if e != nil || first.Dispatch.ID == "" {
		t.Fatal("initial graph node was not admitted", first, e)
	}
	second, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, triggerB, b)
	if e != nil || second.Dispatch.ID == "" {
		t.Fatal("independent graph node was not admitted", second, e)
	}
	claims, e := s.GraphClaims(ctx)
	if e != nil || len(claims) != 2 || claims[0].ContextID == nil || claims[1].ContextID == nil || *claims[0].ContextID == *claims[1].ContextID {
		t.Fatal("selection/reservation was not atomic across contexts", claims, e)
	}
	budget, e := s.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || budget.Budget.ConsumedAdmissions != "2" || budget.Budget.RemainingAdmissions != "2" || budget.Budget.CurrentWindow.DeadlineAt == nil || budget.NodeConsumption[0].ConsumedAdmissions != "1" || budget.NodeConsumption[1].ConsumedAdmissions != "1" {
		t.Fatal("node admissions multiplied/refilled/misowned budget", budget, e)
	}
	for _, d := range []domain.GraphDispatchView{first, second} {
		a, e := s.Attempt(ctx, d.Dispatch.AttemptID)
		if e != nil || a.Phase != "queued" || a.LaunchIntent || a.WorkflowID != d.Dispatch.WorkflowID || a.StepID != d.Dispatch.StepID || d.Dispatch.NodeAdmissionID != d.Dispatch.ID || d.Dispatch.NativeConsumptionKey == nil || *d.Dispatch.NativeConsumptionKey != a.ID || d.Dispatch.SourceBasis.WorkflowID != a.WorkflowID || d.Dispatch.SourceBasis.StepID != a.StepID {
			t.Fatal("fresh attempt/step/anchor/dispatch identities were not allocated together", a, d, e)
		}
		decision, e := s.routingDecision(ctx, s.db, *a.RoutingDecisionID)
		if e != nil || decision.TaskID != a.TaskID || decision.BudgetEffect.GraphBudgetRef == nil || decision.BudgetEffect.GraphBudgetRef.BudgetOwnerTaskID != graph.Graph.RootTaskID {
			t.Fatal("child decision hid the budget owner", decision, e)
		}
		w, e := s.Workflow(ctx, a.TaskID, domain.WorkflowSelector{})
		if e != nil || w.Workflow == nil || w.Workflow.Authority.Delivery.LocalMode != "review_only" {
			t.Fatal("saved preference retroactively changed graph delivery scope", w, e)
		}
	}
	if duplicate, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, triggerA, a); e != nil || duplicate.Dispatch.ID != "" {
		t.Fatal("same callback rescheduled existing native work", duplicate, e)
	}
	var n int
	if e = s.db.QueryRow("SELECT count(*) FROM task_budgets").Scan(&n); e != nil || n != 1 {
		t.Fatal("child acquired independent budget", n, e)
	}
}
func TestGraphNodeAdmissionPreparationBlockDoesNotSpendOrRearm(t *testing.T) {
	s, graph, root, proofs, owner := graphReadyBasis(t)
	ctx := context.Background()
	facts, trigger := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[0])
	originalCapture := facts.Capture
	facts.PreparationFailure = domain.Fail("check_unavailable", "A required captured check program is unavailable.")
	facts.Capture = nil
	d, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, facts)
	if e != nil || d.Dispatch.ID != "" {
		t.Fatal(d, e)
	}
	view, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil || view.Nodes[0].State != "blocked" || view.Nodes[0].NextOwner != "user" || len(view.Blockers) != 1 || view.Blockers[0].Code != "check_unavailable" || view.Blockers[0].InputID == nil || *view.Blockers[0].InputID != facts.InputBinding.ID {
		t.Fatal("pre-admission blocker was invisible", view, e)
	}
	facts.PreparationFailure = nil
	facts.Capture = originalCapture
	if retry, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, facts); e != nil || retry.Dispatch.ID != "" {
		t.Fatal("same callback rearmed blocked work", retry, e)
	}
	budget, e := s.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || budget.Budget.ConsumedAdmissions != "0" || budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("preparation failure spent/opened budget", budget, e)
	}
}
func TestGraphNodeAdmissionCommitFaultsAreAtomicAndDoNotReschedule(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit"}[after], func(t *testing.T) {
			s, graph, root, proofs, owner := graphReadyBasis(t)
			ctx := context.Background()
			facts, trigger := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[0])
			fault := func() error { return errors.New("fixed graph admission acknowledgement fault") }
			if after {
				s.AfterCommit = fault
			} else {
				s.BeforeCommit = fault
			}
			_, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, facts)
			var de *domain.Error
			expected := "storage_failed"
			consumed := "0"
			claimsCount := 0
			if after {
				expected = "write_uncertain"
				consumed = "1"
				claimsCount = 1
			}
			if !errors.As(e, &de) || de.Code != expected {
				t.Fatal("admission fault lost its durable boundary", e)
			}
			s.BeforeCommit = nil
			s.AfterCommit = nil
			budget, e := s.GraphBudget(ctx, graph.Graph.ID)
			if e != nil || budget.Budget.ConsumedAdmissions != consumed {
				t.Fatal(budget, e)
			}
			claims, e := s.GraphClaims(ctx)
			if e != nil || len(claims) != claimsCount {
				t.Fatal("reservation escaped admission transaction", claims, e)
			}
			retry, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, facts)
			if e != nil || after && retry.Dispatch.ID != "" || !after && retry.Dispatch.ID == "" {
				t.Fatal("retry duplicated or failed to reconcile admission", retry, e)
			}
			budget, e = s.GraphBudget(ctx, graph.Graph.ID)
			if e != nil || budget.Budget.ConsumedAdmissions != "1" {
				t.Fatal("retry debited twice", budget, e)
			}
		})
	}
}

func TestGraphNodePreparationFailureProjectsWithoutUnavailablePhysicalProofs(t *testing.T) {
	s, graph, root, proofs, owner := graphReadyBasis(t)
	ctx := context.Background()
	facts, trigger := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[0])
	facts.PreparationFailure = domain.Fail("evidence_unavailable", "The exact retained input could not be read completely.")
	facts.Capture = nil
	facts.VerifiedArtifacts = nil
	facts.SourceBasis = domain.GraphSourceBasis{}
	if d, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, facts); e != nil || d.Dispatch.ID != "" {
		t.Fatal("known pre-admission proof unavailability did not project passive blocker", d, e)
	}
	view, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil || view.Nodes[0].State != "blocked" || view.Nodes[0].NextOwner != "user" || len(view.Blockers) != 1 || view.Blockers[0].Code != "evidence_unavailable" || view.Budget.Budget.ConsumedAdmissions != "0" || view.Budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("failed readiness proof spent authority or promised automatic work", view, e)
	}
}
func TestGraphAdmissionWaitConsumesOldTriggerBeforeCapacityChanges(t *testing.T) {
	s, graph, root, proofs, owner := graphReadyBasis(t)
	ctx := context.Background()
	facts, trigger := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[0])
	held := []domain.Attempt{}
	for _, harness := range []string{"codex", "claude-code"} {
		in := taskInput(graph.Graph.ProjectID)
		in.Pins = &domain.ExecutionPins{HarnessID: harness}
		r := s.CreateTask(ctx, owner, in)
		if r.Status != 201 {
			t.Fatal(r)
		}
		a := r.Envelope.Data.(domain.Admission).Attempt
		claimed, e := s.ClaimAttempt(ctx, a.ID, owner)
		if e != nil {
			t.Fatal(e)
		}
		held = append(held, claimed)
	}
	if d, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, facts); e != nil || d.Dispatch.ID != "" {
		t.Fatal("occupied pool did not wait without native admission", d, e)
	}
	before, e := s.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || before.Budget.ConsumedAdmissions != "0" {
		t.Fatal(before, e)
	}
	if _, e = s.SettleAttempt(ctx, held[0].ID, domain.Settlement{Outcome: "failed", NoProcessCreated: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}); e != nil {
		t.Fatal(e)
	}
	if d, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, facts); e != nil || d.Dispatch.ID != "" {
		t.Fatal("old callback rearmed after unrelated capacity changed", d, e)
	}
	after, e := s.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || after.Budget.ConsumedAdmissions != "0" {
		t.Fatal(after, e)
	}
	sameEvent := trigger
	sameEvent.Kind = "claim_release"
	if d, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, sameEvent, facts); e != nil || d.Dispatch.ID != "" {
		t.Fatal("reclassifying one completed/released event consumed it twice", d, e)
	}
	fresh := domain.GraphFreshTrigger{Kind: "claim_release", ID: held[0].ID, OwnerGeneration: trigger.OwnerGeneration}
	if d, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, fresh, facts); e != nil || d.Dispatch.ID == "" {
		t.Fatal("new actual release did not authorize fresh capacity re-evaluation", d, e)
	}
}
