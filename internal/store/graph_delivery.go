package store

import (
	"context"
	"database/sql"
	"reflect"
	"sort"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// This cache associates the exact closed graph binding with its real005
// operation. Fresh filesystem proofs are supplied separately for each guard.
type graphDeliveryRecord struct {
	OperationID string                      `json:"operationId"`
	Binding     domain.GraphDeliveryBinding `json:"binding"`
	BindingRef  domain.GraphArtifactRef     `json:"bindingRef"`
}

func graphDeliveryArgument(values []*domain.GraphDeliveryFacts) (*domain.GraphDeliveryFacts, error) {
	if len(values) > 1 {
		return nil, domain.Invalid("one exact private graph delivery binding")
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return nil, nil
}

// No filesystem reads occur here. The controller supplies fresh physical proofs
// after full-member verification; this transaction joins their registered tuples.
func (s *Store) graphDeliveryGuard(ctx context.Context, tx *sql.Tx, ownerID string, operation domain.DeliveryOperation, facts *domain.GraphDeliveryFacts) error {
	w, step, a, err := s.deliveryControls(ctx, tx, ownerID, operation)
	if err != nil {
		return err
	}
	member, err := s.graphMembership(ctx, tx, w.TaskID)
	if err != nil {
		return err
	}
	if member == nil {
		if facts != nil || operation.GraphDeliveryRef != nil {
			return domain.Invalid("graph delivery facts only for a registered graph final")
		}
		return nil
	}
	if member.Kind != "node" || member.NodeID == nil || facts == nil || operation.GraphDeliveryRef == nil ||
		step.Role != "local_delivery" || step.ExecutorKind != "local_application" || a.Role != "local_delivery" ||
		facts.BindingRef != *operation.GraphDeliveryRef || facts.BindingRef.ProducerAttemptID != a.ID ||
		len(facts.VerifiedArtifacts) > 256 {
		return evidenceUnavailable()
	}
	if err = s.graphDispatchControls(ctx, tx, ownerID, a, false); err != nil {
		return err
	}
	record, err := s.graphDispatchForAttempt(ctx, tx, a.ID)
	if err != nil {
		return err
	}
	d, binding := record.Public.Dispatch, facts.Binding
	h, err := s.graphHeader(ctx, tx, member.GraphID)
	if err != nil {
		return err
	}
	if h.Summary.FinalNodeID == nil || *h.Summary.FinalNodeID != *member.NodeID ||
		binding.GraphID != h.Summary.ID || binding.GraphVersion != h.Summary.GraphVersion || binding.GraphControlRevision != h.Summary.ControlRevision ||
		binding.FinalNodeID != *member.NodeID || binding.FinalTaskID != w.TaskID || binding.FinalTaskVersion != w.TaskVersion ||
		binding.FinalWorkflowID != w.ID || binding.FinalInputID != d.InputID ||
		binding.FinalCandidateBinding != operation.CandidateBinding ||
		d.SourceBasis.BasisKind != "node_subject" || d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != binding.FinalVerificationRef ||
		d.SourceBasis.CandidateBinding == nil || *d.SourceBasis.CandidateBinding != binding.FinalCandidateBinding ||
		!reflect.DeepEqual(binding.ContributorVector, d.ContributorVector) ||
		!graphRefHasBytes(facts.BindingRef, []byte(encode(binding))) {
		return evidenceUnavailable()
	}
	seal, err := domain.GraphDeliveryDigest(binding)
	if err != nil || seal != binding.BindingSHA256 {
		return evidenceUnavailable()
	}
	definition, err := s.graphDefinition(ctx, tx, h.Summary.ID, h.Summary.GraphVersion)
	if err != nil {
		return err
	}
	scope, err := domain.GraphDeliveryScopeDigest(w.Authority.Delivery)
	if err != nil || scope != binding.DeliveryScopeSHA256 || !reflect.DeepEqual(definition.Delivery, operation.Scope) {
		return evidenceUnavailable()
	}
	if err = s.verifyGraphRef(ctx, tx, facts.BindingRef, facts.VerifiedArtifacts, "graph_delivery"); err != nil {
		return err
	}
	// The already admitted local role came from the preceding Store-derived
	// qualification proof. Do not compare obsolete pre-admission WF revisions.
	if err = s.graphDeliveryPredecessor(ctx, tx, w, step, binding); err != nil {
		return err
	}
	final, err := s.graphFinalPlanFacts(ctx, tx, w, a, domain.WorkflowStepFacts{VerifiedArtifacts: facts.VerifiedArtifacts, CumulativeDirectProven: true})
	if err != nil {
		return err
	}
	if final == nil || final.CurrentSubject != binding.FinalCandidateBinding || final.VerificationBindingRef != binding.FinalVerificationRef {
		return evidenceUnavailable()
	}
	candidate, err := s.candidate(ctx, tx, binding.FinalCandidateBinding.CandidateID)
	if err != nil {
		return err
	}
	producer, err := s.graphDispatchForAttempt(ctx, tx, candidate.AttemptID)
	if err != nil || producer.FinalVerification == nil ||
		!reflect.DeepEqual(binding.RootBinding, producer.FinalVerification.RootBinding) ||
		binding.FinalInputID != producer.FinalVerification.FinalInputID ||
		!reflect.DeepEqual(binding.ContributorVector, producer.FinalVerification.ContributorVector) {
		if err != nil {
			return err
		}
		return evidenceUnavailable()
	}
	if err = s.graphDeliveryEvidence(ctx, tx, w, binding, facts.VerifiedArtifacts, final); err != nil {
		return err
	}
	return s.graphDeliveryAcceptances(ctx, tx, h, definition, d, binding, facts.VerifiedArtifacts)
}

func (s *Store) graphDeliveryPredecessor(ctx context.Context, q queryer, w domain.Workflow, step domain.WorkflowStep, binding domain.GraphDeliveryBinding) error {
	key, invalid := domain.WorkflowStageKey("local_delivery", step.Cycle, "", binding.FinalCandidateBinding.CandidateID)
	if invalid != nil || step.LogicalKey != key || step.InputCandidateID == nil || *step.InputCandidateID != binding.FinalCandidateBinding.CandidateID ||
		step.InputResponseBinding != nil || step.PredecessorStepID == nil {
		return evidenceUnavailable()
	}
	previous, err := s.workflowStep(ctx, q, *step.PredecessorStepID)
	if err != nil {
		return err
	}
	proof := previous.AdvanceProof
	if previous.WorkflowID != w.ID || previous.Status != "succeeded" || previous.AttemptID == nil || proof == nil ||
		proof.PredecessorStepID != previous.ID || proof.TaskVersion != w.TaskVersion || proof.NextLogicalKey != key ||
		proof.CandidateID == nil || *proof.CandidateID != binding.FinalCandidateBinding.CandidateID || proof.ResponseBinding != nil {
		return evidenceUnavailable()
	}
	a, err := s.attempt(ctx, q, *previous.AttemptID)
	if err != nil {
		return err
	}
	if a.WorkflowID != w.ID || a.TaskID != w.TaskID || a.TaskVersion != w.TaskVersion || a.Phase != "settled" ||
		a.Outcome == nil || *a.Outcome != "succeeded" || a.Stop != nil || a.PrimaryFailure != nil || a.Cleanup.State != "verified" || a.EvidenceStatus != "complete" {
		return evidenceUnavailable()
	}
	return nil
}

func graphDeliveryUsesEqual(first, second []domain.EvidenceUse) bool {
	if len(first) != len(second) || len(first) > domain.MaxWorkflowChecks+1 {
		return false
	}
	ordered := func(values []domain.EvidenceUse) []domain.EvidenceUse {
		out := append([]domain.EvidenceUse{}, values...)
		sort.Slice(out, func(i, j int) bool {
			if out[i].Kind == out[j].Kind {
				return out[i].EvidenceID < out[j].EvidenceID
			}
			return out[i].Kind < out[j].Kind
		})
		return out
	}
	return reflect.DeepEqual(ordered(first), ordered(second))
}

func (s *Store) graphDeliveryEvidence(ctx context.Context, tx *sql.Tx, w domain.Workflow, binding domain.GraphDeliveryBinding, proofs []domain.VerifiedResponse, final *workflowpolicy.GraphFinalPlanFacts) error {
	if !graphDeliveryUsesEqual(binding.PreDeliveryEvidenceUses, w.EvidenceUses) {
		return evidenceUnavailable()
	}
	for _, use := range binding.PreDeliveryEvidenceUses {
		var attemptID string
		switch use.Kind {
		case "review":
			record, err := storedWorkflowRecord[domain.ReviewRecord](ctx, tx, "workflow_reviews", use.EvidenceID)
			if err != nil {
				return err
			}
			if record.CandidateBinding == nil || *record.CandidateBinding != binding.FinalCandidateBinding || record.ResponseBinding != nil {
				return evidenceUnavailable()
			}
			attemptID = record.AttemptID
		case "check":
			record, err := storedWorkflowRecord[domain.CheckReceipt](ctx, tx, "workflow_checks", use.EvidenceID)
			if err != nil {
				return err
			}
			if record.InputCandidateBinding == nil || *record.InputCandidateBinding != binding.FinalCandidateBinding || record.InputResponseBinding != nil || record.SourceChanged {
				return evidenceUnavailable()
			}
			attemptID = record.AttemptID
		default:
			return evidenceUnavailable()
		}
		origin, err := s.graphDispatchForAttempt(ctx, tx, attemptID)
		if err != nil {
			return err
		}
		d := origin.Public.Dispatch
		if d.GraphID != binding.GraphID || d.NodeID != binding.FinalNodeID || d.WorkflowID != binding.FinalWorkflowID ||
			d.TaskID != binding.FinalTaskID || d.TaskVersion != binding.FinalTaskVersion || d.InputID != binding.FinalInputID ||
			d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != binding.FinalVerificationRef ||
			d.SourceBasis.CandidateBinding == nil || *d.SourceBasis.CandidateBinding != binding.FinalCandidateBinding {
			return evidenceUnavailable()
		}
	}
	review, checks, err := s.workflowUses(ctx, tx, w, binding.PreDeliveryEvidenceUses, proofs)
	if err != nil {
		return err
	}
	blockers, _, err := s.workflowBlockers(ctx, tx, w)
	if err != nil {
		return err
	}
	needsReview, err := s.workflowNeedsFindingReview(ctx, tx, w, blockers)
	if err != nil {
		return err
	}
	// The canonical predecessor proof anchors the prior cumulative-direct
	// decision; all current required records, findings and policy are repeated.
	decision := workflowpolicy.Plan(workflowpolicy.PlanInput{Authority: w.Authority, HasSubject: true, HasDelta: w.HasTaskDelta, HasIndexDelta: w.HasTaskIndexDelta,
		GraphFinal: final, Review: review, Checks: checks, BlockingFindingIDs: blockers, NeedsFindingReview: needsReview, CorrectionCyclesUsed: w.CorrectionCyclesUsed})
	if decision.Role != "local_delivery" || decision.ExecutorKind != "local_application" {
		return domain.Fail("graph_delivery_unqualified", "The exact cumulative final evidence no longer qualifies local delivery.")
	}
	return nil
}

func graphDeliveryFileAncestors(definition domain.GraphDefinition, nodes []graphNodeRecord, finalID string) (map[string]bool, error) {
	byKey := map[string]string{}
	for _, node := range nodes {
		if byKey[node.Summary.Key] != "" {
			return nil, evidenceUnavailable()
		}
		byKey[node.Summary.Key] = node.Summary.ID
	}
	if len(byKey) != len(definition.Original.Nodes) {
		return nil, evidenceUnavailable()
	}
	for _, declared := range definition.Original.Nodes {
		if byKey[declared.Key] == "" {
			return nil, evidenceUnavailable()
		}
	}
	ancestors := map[string]bool{finalID: true}
	if definition.Original.Dependencies != nil {
		for range nodes {
			for _, edge := range *definition.Original.Dependencies {
				from, to := byKey[edge.From], byKey[edge.To]
				if from == "" || to == "" {
					return nil, evidenceUnavailable()
				}
				if (edge.Kind == "" || edge.Kind == "files") && ancestors[to] {
					ancestors[from] = true
				}
			}
		}
	}
	delete(ancestors, finalID)
	return ancestors, nil
}

func (s *Store) graphDeliveryAcceptances(ctx context.Context, tx *sql.Tx, h graphHeader, definition domain.GraphDefinition, dispatch domain.GraphDispatch, binding domain.GraphDeliveryBinding, proofs []domain.VerifiedResponse) error {
	nodes, err := s.graphNodes(ctx, tx, h.Summary.ID)
	if err != nil {
		return err
	}
	if len(binding.OtherNodeAcceptances) != len(nodes)-1 || len(binding.OtherNodeAcceptances) != len(binding.ContributorVector) {
		return evidenceUnavailable()
	}
	fileAncestors, err := graphDeliveryFileAncestors(definition, nodes, dispatch.NodeID)
	if err != nil {
		return err
	}
	anchor, err := s.graphNodeAdmissionAnchor(ctx, tx, dispatch)
	if err != nil {
		return err
	}
	selected := map[string]domain.GraphContributorBinding{}
	for _, current := range binding.ContributorVector {
		if _, exists := selected[current.NodeID]; exists || current.NodeID == dispatch.NodeID {
			return evidenceUnavailable()
		}
		selected[current.NodeID] = current
	}
	for _, current := range binding.OtherNodeAcceptances {
		expected, exists := selected[current.NodeID]
		if !exists || !reflect.DeepEqual(expected, current) {
			return evidenceUnavailable()
		}
		var node *graphNodeRecord
		for i := range nodes {
			if nodes[i].Summary.ID == current.NodeID {
				node = &nodes[i]
			}
		}
		if node == nil || node.Summary.State != "completed" || node.Summary.AcceptedResult == nil {
			return evidenceUnavailable()
		}
		// A Stop after the final receiver's known first GO does not erase its
		// exact already accepted input. Guidance/version/acceptance changes do.
		observed := *node
		if observed.Summary.Stop != nil {
			if anchor.KnownFirstGOAt == nil || anchor.KnownFirstGOAttemptID == nil {
				return graphControlError()
			}
			observed.Summary.Stop = nil
		}
		head, err := s.graphSelectedHead(ctx, tx, h, observed)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(head.Vector, current) || current.CandidateBinding != nil && !fileAncestors[current.NodeID] {
			return evidenceUnavailable()
		}
		if err = s.graphDeliveryHeadProofs(ctx, tx, head, binding.RootBinding, proofs); err != nil {
			return err
		}
		delete(selected, current.NodeID)
	}
	if len(selected) != 0 {
		return evidenceUnavailable()
	}
	return nil
}

func (s *Store) graphDeliveryHeadProofs(ctx context.Context, q queryer, head graphSelectedHead, root domain.GraphRootView, proofs []domain.VerifiedResponse) error {
	input := head.Input
	if input.PrivateRoot == nil || !reflect.DeepEqual(domain.GraphRootPublic(*input.PrivateRoot), root) ||
		input.Public.InputBinding == nil || input.Public.ConditionsRef == nil || input.ConditionsBinding == nil || input.Public.ContextPacket == nil {
		return evidenceUnavailable()
	}
	b := *input.Public.InputBinding
	if err := s.graphContributorsCurrent(ctx, q, b.GraphID, b.ContributorVector); err != nil {
		return err
	}
	if head.Vector.CandidateBinding != nil {
		if err := s.candidateBinding(ctx, q, head.Workflow, *head.Vector.CandidateBinding, proofs); err != nil {
			return err
		}
	} else if head.Vector.ResponseBinding != nil {
		if err := s.responseBinding(ctx, q, head.Workflow, *head.Vector.ResponseBinding, proofs); err != nil {
			return err
		}
	} else {
		return evidenceUnavailable()
	}
	for _, item := range []struct {
		ref   domain.GraphArtifactRef
		kinds []string
	}{
		{b.Manifest, []string{"graph_input_manifest"}}, {b.Index, []string{"index_snapshot"}}, {b.StagedBlobs, []string{"graph_index_blobs"}},
		{b.Provenance, []string{"graph_input_provenance"}}, {b.ContextPacket, []string{"graph_context"}}, {*input.Public.ConditionsRef, []string{"graph_conditions"}},
		{head.Vector.OutputClosureRef, []string{"graph_output_closure"}}, {head.Closure.Manifest, []string{"source_manifest", "candidate_manifest"}},
		{head.Closure.Index, []string{"index_snapshot", "candidate_index"}}, {head.Closure.StagedBlobs, []string{"graph_index_blobs"}},
	} {
		if err := s.verifyGraphRef(ctx, q, item.ref, proofs, item.kinds...); err != nil {
			return err
		}
	}
	if head.Closure.ResponseArtifact != nil {
		if err := s.verifyGraphRef(ctx, q, *head.Closure.ResponseArtifact, proofs, "final_response"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) graphDeliveryBegin(ctx context.Context, tx *sql.Tx, ownerID string, operation domain.DeliveryOperation, facts *domain.GraphDeliveryFacts) error {
	if err := s.graphDeliveryGuard(ctx, tx, ownerID, operation, facts); err != nil {
		return err
	}
	if facts == nil {
		return nil
	}
	record := graphDeliveryRecord{OperationID: operation.ID, Binding: facts.Binding, BindingRef: facts.BindingRef}
	if _, err := tx.ExecContext(ctx, "INSERT INTO graph_delivery_bindings(operation_id,binding_artifact_id,data) VALUES(?,?,?)", operation.ID, facts.BindingRef.ArtifactID, encode(record)); err != nil {
		return storageError()
	}
	return nil
}

func (s *Store) loadGraphDelivery(ctx context.Context, q queryer, operation domain.DeliveryOperation) (*graphDeliveryRecord, error) {
	if operation.GraphDeliveryRef == nil {
		return nil, nil
	}
	var artifactID, raw string
	if err := q.QueryRowContext(ctx, "SELECT binding_artifact_id,data FROM graph_delivery_bindings WHERE operation_id=?", operation.ID).Scan(&artifactID, &raw); err != nil {
		return nil, graphRowError(err)
	}
	record, err := decode[graphDeliveryRecord](raw)
	if err != nil || record.OperationID != operation.ID || record.BindingRef != *operation.GraphDeliveryRef ||
		artifactID != record.BindingRef.ArtifactID || record.BindingRef.ProducerAttemptID != operation.AttemptID ||
		record.Binding.FinalWorkflowID != operation.WorkflowID || record.Binding.FinalCandidateBinding != operation.CandidateBinding ||
		!graphRefHasBytes(record.BindingRef, []byte(encode(record.Binding))) {
		return nil, evidenceUnavailable()
	}
	seal, err := domain.GraphDeliveryDigest(record.Binding)
	if err != nil || seal != record.Binding.BindingSHA256 {
		return nil, evidenceUnavailable()
	}
	if err = s.graphStoredRef(ctx, q, record.Binding.GraphID, record.BindingRef, "graph_delivery"); err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Store) graphDeliveryContinue(ctx context.Context, tx *sql.Tx, ownerID string, operation domain.DeliveryOperation, facts *domain.GraphDeliveryFacts) error {
	cached, err := s.loadGraphDelivery(ctx, tx, operation)
	if err != nil {
		return err
	}
	if cached != nil && (facts == nil || cached.BindingRef != facts.BindingRef || !reflect.DeepEqual(cached.Binding, facts.Binding)) {
		return evidenceUnavailable()
	}
	return s.graphDeliveryGuard(ctx, tx, ownerID, operation, facts)
}
