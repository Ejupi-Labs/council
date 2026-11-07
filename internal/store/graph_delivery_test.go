package store

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

type graphDeliveryDAOCase struct {
	store     *Store
	owner     string
	dispatch  domain.GraphDispatch
	attempt   domain.Attempt
	operation domain.DeliveryOperation
	facts     domain.GraphDeliveryFacts
}

// Actual graph/author/final-output/continuation/claim transactions establish
// authority. Typed controller proofs qualify SQL joins only: these metadata
// fixtures do not assert OS application, model inspection or acceptance.
func graphDeliveryDAOFixture(t *testing.T) graphDeliveryDAOCase {
	t.Helper()
	ctx := context.Background()
	s, graph, root, rootProofs, owner := graphReadyBasis(t, func(in *domain.GraphStartInput) {
		in.Nodes = in.Nodes[:1]
		in.FinalNodeKey = in.Nodes[0].Key
		in.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "apply_verified_local"}
		in.Nodes[0].Acceptance = &domain.AcceptanceInput{Deliverable: "candidate"}
	})
	input, trigger := graphReadyNodeFacts(t, s, graph, root, rootProofs, owner, graph.Nodes[0])
	// The generic admission helper captures review-only condition facts. This
	// fixture captures its requested apply scope BEFORE workflow admission and
	// computes the full matching registry and effective graph condition digest.
	task, err := s.Task(ctx, graph.Nodes[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	options := input.Capture.Authority.OptionSnapshot
	options.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "apply_verified_local"}
	registry, invalid := workflowpolicy.Conditions(task.Task.OriginalText, task.Version, options, input.Capture.Authority.Checks)
	if invalid != nil {
		t.Fatal(invalid)
	}
	instruction, conditions, err := workflowpolicy.ConditionDigests(task.Task.OriginalText, task.Version, registry, input.Capture.Authority.Checks)
	if err != nil {
		t.Fatal(err)
	}
	input.Capture.Authority.OptionSnapshot, input.Capture.Authority.Conditions = options, registry
	input.Capture.Authority.InstructionSHA256 = instruction
	input.Capture.Authority.ConditionsSHA256 = graphEffectiveWorkflowConditions(conditions, input.ConditionsBinding.ConditionsSHA256)
	producer, err := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, input)
	if err != nil {
		t.Fatal("The exact apply-scope workflow could not be admitted", err)
	}
	materialization := graphMaterializationForDispatch(t, s, producer, input, root, owner)
	author := graphOutputForDispatch(t, s, producer, materialization, owner, true)
	c, err := s.Candidate(ctx, author.GraphOutput.Observation.Candidate.CandidateID)
	if err != nil || c.HasDelta || c.HasIndexDelta {
		t.Fatal("The registered SQL fixture did not retain a truthful own-zero candidate", c, err)
	}
	w, err := s.Workflow(ctx, producer.Dispatch.TaskID, domain.WorkflowSelector{})
	if err != nil || w.Workflow == nil || w.Workflow.Authority.Delivery.LocalMode != "apply_verified_local" {
		t.Fatal(w, err)
	}
	closure := author.GraphOutput.CandidateClosure
	author.CandidateBinding = &domain.CandidateBinding{CandidateID: c.ID, RootSourceSnapshotID: c.RootSourceSnapshotID, SourceSnapshotID: c.SourceSnapshotID,
		ManifestArtifactID: closure.Manifest.ArtifactID, ManifestSHA256: closure.Manifest.SHA256, IndexArtifactID: closure.Index.ArtifactID, IndexSHA256: closure.Index.SHA256,
		ConditionsSHA256: w.Workflow.Authority.ConditionsSHA256}
	author.ResponseBinding = nil
	author.GraphOutput.ResponseClosure = nil
	author.GraphOutput.Observation.Response.State, author.GraphOutput.Observation.Response.ClosureRef = "incomplete", nil
	graphFinalOutputFacts(t, s, producer, &author)
	author.DirectProven, author.CumulativeDirectProven = false, true
	settled, err := s.SettleWorkflowStep(ctx, producer.Dispatch.AttemptID, author)
	if err != nil || settled.NextAction != "apply" || settled.HasTaskDelta || settled.HasTaskIndexDelta {
		t.Fatal("Registered cumulative-only qualification did not install local delivery", settled, err)
	}
	stage := graphReviewAdmissionFacts(t, s, producer, author)
	stage.Plan.Role, stage.Plan.ExecutorKind = "local_delivery", "local_application"
	key, invalid := domain.WorkflowStageKey("local_delivery", 0, "", author.CandidateBinding.CandidateID)
	if invalid != nil {
		t.Fatal(invalid)
	}
	stage.Plan.LogicalKey = key
	stage.Graph.SourceBasis.FinalVerificationRef = author.GraphOutput.Observation.Candidate.FinalVerificationRef
	stage.Graph.VerifiedArtifacts = append(stage.Graph.VerifiedArtifacts, author.GraphOutput.VerifiedArtifacts...)
	admitted, err := s.ContinueWorkflow(ctx, owner, producer.Dispatch.StepID, stage)
	if err != nil || admitted == nil || admitted.Attempt.ExecutorKind != "local_application" || admitted.Step.Role != "local_delivery" {
		t.Fatal("The actual final predecessor proof could not admit local delivery", admitted, err)
	}
	a, err := s.ClaimAttempt(ctx, admitted.Attempt.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil || d.Public.Dispatch.SourceBasis.FinalVerificationRef == nil || *d.Public.Dispatch.SourceBasis.FinalVerificationRef != *stage.Graph.SourceBasis.FinalVerificationRef {
		t.Fatal(d, err)
	}
	root, err = s.GraphRoot(ctx, d.Public.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, err := domain.GraphDeliveryScopeDigest(admitted.Workflow.Authority.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	binding := domain.GraphDeliveryBinding{GraphID: d.Public.Dispatch.GraphID, GraphVersion: d.Public.Dispatch.GraphVersion, GraphControlRevision: d.Public.Dispatch.GraphControlRevision,
		RootBinding: domain.GraphRootPublic(root), FinalNodeID: d.Public.Dispatch.NodeID, FinalTaskID: a.TaskID, FinalTaskVersion: a.TaskVersion, FinalWorkflowID: a.WorkflowID,
		FinalCandidateBinding: *author.CandidateBinding, FinalInputID: d.Public.Dispatch.InputID, ContributorVector: []domain.GraphContributorBinding{},
		FinalVerificationRef: *stage.Graph.SourceBasis.FinalVerificationRef, OtherNodeAcceptances: []domain.GraphContributorBinding{}, PreDeliveryEvidenceUses: []domain.EvidenceUse{}, DeliveryScopeSHA256: scopeDigest}
	proofs, err := graphFinalPlannerProofs(author)
	if err != nil {
		t.Fatal(err)
	}
	hash, invalid := domain.WorkflowAuthoritySHA256(admitted.Workflow.Authority)
	if invalid != nil {
		t.Fatal(invalid)
	}
	f := graphDeliveryDAOCase{store: s, owner: owner, dispatch: d.Public.Dispatch, attempt: a,
		operation: domain.DeliveryOperation{ID: domain.NewID(), WorkflowID: a.WorkflowID, StepID: a.StepID, AttemptID: a.ID, RequestID: a.StartRequestID,
			CandidateBinding: *author.CandidateBinding, EvidenceUses: []domain.EvidenceUse{}, Scope: admitted.Workflow.Authority.Delivery, AuthoritySHA256: hash,
			Status: "preparing", LastEffectSequence: "0", ConflictCount: "0", Conflicts: []domain.DeliveryConflict{}, CreatedAt: domain.Now()},
		facts: domain.GraphDeliveryFacts{Binding: binding, VerifiedArtifacts: proofs}}
	f.reseal(t)
	return f
}

func (f *graphDeliveryDAOCase) reseal(t *testing.T) {
	t.Helper()
	var err error
	f.facts.Binding.BindingSHA256, err = domain.GraphDeliveryDigest(f.facts.Binding)
	if err != nil {
		t.Fatal(err)
	}
	ref, proof := graphPayload(t, f.store, f.attempt.ID, "graph_delivery", []byte(encode(f.facts.Binding)))
	f.facts.BindingRef = ref
	f.facts.VerifiedArtifacts = append(f.facts.VerifiedArtifacts, proof)
	f.operation.GraphDeliveryRef = &ref
}

func (f *graphDeliveryDAOCase) ready(t *testing.T) {
	t.Helper()
	plan, _ := graphPayload(t, f.store, f.attempt.ID, "delivery_plan", []byte("controlled retained native application plan"))
	manifest, _ := graphPayload(t, f.store, f.attempt.ID, "destination_manifest", []byte("controlled actual-before metadata"))
	f.operation.PlanArtifactID = plan.ArtifactID
	f.operation.BeforeDestination = &domain.DestinationEvidence{ProjectIdentity: f.operation.Scope.ProjectIdentity, RepositoryIdentity: f.operation.Scope.RepositoryIdentity,
		Branch: f.operation.Scope.Branch, BaseCommit: f.facts.Binding.RootBinding.BaseCommit, ManifestArtifactID: manifest.ArtifactID, ManifestSHA256: manifest.SHA256}
	f.operation.Status = "ready"
}

func (f graphDeliveryDAOCase) permit(t *testing.T) domain.DeliveryPermitFacts {
	t.Helper()
	task, err := f.store.Task(context.Background(), f.attempt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.store.Workflow(context.Background(), f.attempt.TaskID, domain.WorkflowSelector{WorkflowID: f.attempt.WorkflowID})
	if err != nil || w.Workflow == nil {
		t.Fatal(w, err)
	}
	return domain.DeliveryPermitFacts{Graph: &f.facts, OperationID: f.operation.ID, OwnerInstanceID: f.owner, TaskRevision: task.Task.Revision, WorkflowRevision: w.Workflow.Revision,
		Effect: domain.DeliveryEffect{OperationID: f.operation.ID, EffectID: domain.NewID(), RelativePath: "council-graph-a.txt", Kind: "add", Action: "publish_after", State: "intent"}}
}

func TestGraphDeliveryGuardBindsActualFinalRegistration(t *testing.T) {
	f := graphDeliveryDAOFixture(t)
	ctx := context.Background()
	// This real registered fixture reaches the new guard's explicit unavailable
	// stub before activation; it is not a fabricated delivery or acceptance.
	if err := f.store.executionWrite(ctx, func(tx *sql.Tx) error {
		return f.store.graphDeliveryGuard(ctx, tx, f.owner, f.operation, &f.facts)
	}); err != nil {
		t.Fatal("The exact current registered graph delivery basis was rejected", err)
	}
	if err := f.store.BeginDelivery(ctx, f.owner, f.operation, &f.facts); err != nil {
		t.Fatal(err)
	}
	f.ready(t)
	if err := f.store.ReadyDelivery(ctx, f.owner, f.operation, &f.facts); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.PermitDeliveryEffect(ctx, f.permit(t)); err != nil {
		t.Fatal("The exact current graph binding could not grant one005 effect", err)
	}
}

func TestGraphDeliveryBeginRejectsResealedWrongCurrentBinding(t *testing.T) {
	for _, field := range []string{"graph", "version", "control", "root", "final_node", "task", "task_version", "workflow", "candidate_source", "input", "final_ref", "scope", "unaccepted_contributor", "unregistered_evidence"} {
		t.Run(field, func(t *testing.T) {
			f := graphDeliveryDAOFixture(t)
			b := &f.facts.Binding
			switch field {
			case "graph":
				b.GraphID = domain.NewID()
			case "version":
				b.GraphVersion = "2"
			case "control":
				b.GraphControlRevision = "2"
			case "root":
				b.RootBinding.SourceSnapshotID = domain.NewID()
			case "final_node":
				b.FinalNodeID = domain.NewID()
			case "task":
				b.FinalTaskID = domain.NewID()
			case "task_version":
				b.FinalTaskVersion = "2"
			case "workflow":
				b.FinalWorkflowID = domain.NewID()
			case "candidate_source":
				b.FinalCandidateBinding.RootSourceSnapshotID = b.RootBinding.SourceSnapshotID
			case "input":
				b.FinalInputID = domain.NewID()
			case "final_ref":
				b.FinalVerificationRef = b.RootBinding.Manifest
			case "scope":
				b.DeliveryScopeSHA256 = b.RootBinding.BasisSHA256
			case "unaccepted_contributor":
				b.OtherNodeAcceptances = []domain.GraphContributorBinding{{NodeID: f.dispatch.NodeID, TaskID: f.attempt.TaskID, TaskVersion: f.attempt.TaskVersion, ProducerAttemptID: f.attempt.ID, InputID: f.dispatch.InputID}}
			case "unregistered_evidence":
				b.PreDeliveryEvidenceUses = []domain.EvidenceUse{{Kind: "review", EvidenceID: domain.NewID(), CandidateID: b.FinalCandidateBinding.CandidateID}}
			}
			f.reseal(t)
			if err := f.store.BeginDelivery(context.Background(), f.owner, f.operation, &f.facts); err == nil {
				t.Fatal("A resealed wrong graph binding acquired a destination operation", field)
			}
			var count int
			if err := f.store.db.QueryRow("SELECT count(*) FROM delivery_operations WHERE id=?", f.operation.ID).Scan(&count); err != nil || count != 0 {
				t.Fatal("Rejected graph preparation partly committed an operation", count, err)
			}
		})
	}
}

func TestGraphDeliveryBeginRequiresFreshClosureAndOriginalProofs(t *testing.T) {
	for _, missing := range []string{"binding", "root", "input_context", "conditions", "original", "final", "delta"} {
		t.Run(missing, func(t *testing.T) {
			f := graphDeliveryDAOFixture(t)
			current, err := f.store.GraphDispatchForAttempt(context.Background(), f.facts.Binding.FinalVerificationRef.ProducerAttemptID)
			if err != nil || current.FinalVerification == nil {
				t.Fatal(current, err)
			}
			final := *current.FinalVerification
			id := map[string]string{"binding": f.facts.BindingRef.ArtifactID, "root": final.RootBinding.Manifest.ArtifactID, "conditions": final.GraphConditionsRef.ArtifactID, "final": f.facts.Binding.FinalVerificationRef.ArtifactID, "delta": final.CumulativeDeltaRef.ArtifactID}[missing]
			if missing == "input_context" {
				input, err := f.store.GraphInputRecord(context.Background(), f.dispatch.GraphID, f.dispatch.InputID)
				if err != nil || input.Public.InputBinding == nil {
					t.Fatal(input, err)
				}
				id = input.Public.InputBinding.ContextPacket.ArtifactID
			}
			if missing == "original" {
				id = final.OriginalAccess[0].ArtifactRef.ArtifactID
			}
			proofs := []domain.VerifiedResponse{}
			for _, proof := range f.facts.VerifiedArtifacts {
				if proof.ArtifactID != id {
					proofs = append(proofs, proof)
				}
			}
			if len(proofs) == len(f.facts.VerifiedArtifacts) {
				t.Fatal("The decisive actual proof was absent from setup", missing)
			}
			f.facts.VerifiedArtifacts = proofs
			if err := f.store.BeginDelivery(context.Background(), f.owner, f.operation, &f.facts); err == nil {
				t.Fatal("Graph delivery proceeded without a fresh actual closure/original anchor", missing)
			}
		})
	}
}

func TestGraphDeliveryReadyAndPermitRecheckExactBinding(t *testing.T) {
	for _, boundary := range []string{"ready", "permit"} {
		t.Run(boundary, func(t *testing.T) {
			f := graphDeliveryDAOFixture(t)
			ctx := context.Background()
			if err := f.store.BeginDelivery(ctx, f.owner, f.operation, &f.facts); err != nil {
				t.Fatal(err)
			}
			f.ready(t)
			if boundary == "permit" {
				if err := f.store.ReadyDelivery(ctx, f.owner, f.operation, &f.facts); err != nil {
					t.Fatal(err)
				}
			}
			old := f.facts.BindingRef
			f.facts.Binding.FinalWorkflowID = domain.NewID()
			f.reseal(t)
			if boundary == "ready" {
				if err := f.store.ReadyDelivery(ctx, f.owner, f.operation, &f.facts); err == nil {
					t.Fatal("Ready replaced the immutable graph delivery binding")
				}
			} else {
				request := f.permit(t)
				if _, err := f.store.PermitDeliveryEffect(ctx, request); err == nil {
					t.Fatal("A fresh effect used a different graph delivery binding")
				}
			}
			stored, err := f.store.DeliveryOperation(ctx, f.operation.ID)
			if err != nil || stored.GraphDeliveryRef == nil || !reflect.DeepEqual(*stored.GraphDeliveryRef, old) {
				t.Fatal("A rejected transition rebound the retained operation", stored, err)
			}
			var count int
			if err = f.store.db.QueryRow("SELECT count(*) FROM delivery_effects WHERE operation_id=?", f.operation.ID).Scan(&count); err != nil || count != 0 {
				t.Fatal("A rejected transition granted an effect", count, err)
			}
		})
	}
}
