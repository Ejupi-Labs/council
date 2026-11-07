package store

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Typed unactivated boundaries let callers/tests integrate against the frozen
// contract. Each remains fail-closed until its actual transaction is enabled.
func graphUnavailable() *domain.Error {
	return domain.Fail("capability_unavailable", "Graph storage is not yet enabled.")
}

func (s *Store) CreateGraph(ctx context.Context, ownerID string, in domain.GraphStartInput, facts domain.GraphCreateFacts) domain.Result {
	return s.createGraph(ctx, ownerID, in, facts)
}
func (s *Store) AdvanceGraph(ctx context.Context, ownerID, graphID string, in domain.GraphAdvanceInput, facts domain.GraphAdvanceFacts) domain.Result {
	if e := domain.ValidateGraphAdvanceInput(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.advanceGraph(ctx, ownerID, graphID, in, facts)
}
func (s *Store) StopGraph(ctx context.Context, ownerID, graphID string, in domain.GraphStopInput) domain.Result {
	if e := domain.ValidateGraphStopInput(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.stopGraph(ctx, ownerID, graphID, in)
}
func (s *Store) StopGraphNode(ctx context.Context, ownerID, graphID, nodeKey string, in domain.GraphNodeStopInput) domain.Result {
	if e := domain.ValidateGraphNodeStopInput(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.stopGraphNode(ctx, ownerID, graphID, nodeKey, in)
}
func (s *Store) UpdateGraphNodeGuidance(ctx context.Context, ownerID, graphID, nodeKey string, in domain.GraphGuidanceInput, facts domain.GraphAdvanceFacts) domain.Result {
	if e := domain.ValidateGraphGuidanceInput(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.updateGraphGuidance(ctx, ownerID, graphID, nodeKey, in, facts)
}
func (s *Store) AmendGraphBudget(ctx context.Context, ownerID, graphID string, in domain.GraphBudgetAmendmentInput) domain.Result {
	if e := domain.ValidateGraphBudgetAmendmentInput(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.amendGraphBudget(ctx, ownerID, graphID, in)
}
func (s *Store) BeginGraphInput(ctx context.Context, ownerID, graphID, nodeID string, facts domain.GraphInputPreparationFacts) (domain.GraphInputOperation, error) {
	return s.beginGraphInput(ctx, ownerID, graphID, nodeID, facts)
}
func (s *Store) GraphInputOperationForAttempt(ctx context.Context, ownerID, attemptID string) (domain.GraphInputOperation, error) {
	return s.graphInputOperationForAttemptPublic(ctx, ownerID, attemptID)
}
func (s *Store) CheckGraphLocalOperation(ctx context.Context, ownerID, attemptID string) error {
	return s.checkGraphLocalOperationPublic(ctx, ownerID, attemptID)
}

func (s *Store) CompleteGraphInput(ctx context.Context, ownerID, operationAttemptID string, facts domain.GraphInputCompletionFacts) error {
	return s.completeGraphInput(ctx, ownerID, operationAttemptID, facts)
}
func (s *Store) RecordGraphMaterialization(ctx context.Context, ownerID, receivingAttemptID string, facts domain.GraphMaterializationFacts) error {
	return s.recordGraphMaterialization(ctx, ownerID, receivingAttemptID, facts)
}
func (s *Store) AdmitGraphNode(ctx context.Context, ownerID, graphID string, trigger domain.GraphFreshTrigger, facts domain.GraphNodeAdmissionFacts) (domain.GraphDispatchView, error) {
	return s.admitGraphNode(ctx, ownerID, graphID, trigger, facts)
}
func (s *Store) ContinueGraph(ctx context.Context, ownerID, graphID string, facts domain.GraphContinuationFacts) (domain.GraphProgress, error) {
	return domain.GraphProgress{}, graphUnavailable()
}

func (s *Store) Graph(ctx context.Context, graphID string) (domain.GraphDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.graphDetail(ctx, s.db, graphID)
}
func (s *Store) Graphs(ctx context.Context, projectID string, limit int, cursor string) (domain.Page[domain.GraphSummary], error) {
	return s.graphs(ctx, projectID, limit, cursor)
}
func (s *Store) GraphDefinition(ctx context.Context, graphID string) (domain.GraphDefinitionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, e := s.graphHeader(ctx, s.db, graphID)
	if e != nil {
		return domain.GraphDefinitionView{}, e
	}
	d, e := s.graphDefinition(ctx, s.db, graphID, h.Summary.GraphVersion)
	return domain.GraphDefinitionPublic(d), e
}
func (s *Store) GraphDefinitionPrivate(ctx context.Context, graphID string) (domain.GraphDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, e := s.graphHeader(ctx, s.db, graphID)
	if e != nil {
		return domain.GraphDefinition{}, e
	}
	return s.graphDefinition(ctx, s.db, graphID, h.Summary.GraphVersion)
}
func (s *Store) GraphInput(ctx context.Context, graphID, inputID string) (domain.GraphInputDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.graphInputRecord(ctx, s.db, graphID, inputID)
	return r.Public, e
}
func (s *Store) GraphInputs(ctx context.Context, graphID string, limit int, cursor string) (domain.Page[domain.GraphInputSummary], error) {
	return s.graphInputs(ctx, graphID, limit, cursor)
}
func (s *Store) GraphDispatch(ctx context.Context, graphID, dispatchID string) (domain.GraphDispatchView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, e := s.graphDispatchRecord(ctx, s.db, graphID, dispatchID)
	return record.Public, e
}
func (s *Store) GraphDispatches(ctx context.Context, graphID, inputID, nodeKey string, limit int, cursor string) (domain.Page[domain.GraphDispatchView], error) {
	return s.graphDispatches(ctx, graphID, inputID, nodeKey, limit, cursor)
}
func (s *Store) GraphBudget(ctx context.Context, graphID string) (domain.GraphBudgetView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, e := s.graphHeader(ctx, s.db, graphID)
	if e != nil {
		return domain.GraphBudgetView{}, e
	}
	return s.graphBudgetView(ctx, s.db, h)
}
func (s *Store) GraphBudgetLedger(ctx context.Context, graphID string, limit int, cursor string) (domain.Page[domain.BudgetLedgerEntry], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, e := s.graphHeader(ctx, s.db, graphID)
	if e != nil {
		return domain.Page[domain.BudgetLedgerEntry]{}, e
	}
	return routingHistoryPage[domain.BudgetLedgerEntry](ctx, s.db, "budget_ledger", h.Summary.RootTaskID, "task_id", limit, cursor)
}

// Private controller reads are never serialized as HTTP result data.
func (s *Store) GraphRoot(ctx context.Context, graphID string) (domain.GraphRootBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, e := s.graphHeader(ctx, s.db, graphID)
	if e != nil {
		return domain.GraphRootBinding{}, e
	}
	if h.Summary.RootInputID == nil {
		return domain.GraphRootBinding{}, graphUnavailable()
	}
	record, e := s.graphInputRecord(ctx, s.db, graphID, *h.Summary.RootInputID)
	if e != nil {
		return domain.GraphRootBinding{}, e
	}
	if record.PrivateRoot == nil {
		return domain.GraphRootBinding{}, graphUnavailable()
	}
	return *record.PrivateRoot, nil
}
func (s *Store) GraphInputRecord(ctx context.Context, graphID, inputID string) (domain.GraphInputRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.graphInputRecord(ctx, s.db, graphID, inputID)
}
func (s *Store) GraphDispatchForAttempt(ctx context.Context, attemptID string) (domain.GraphDispatchRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var graphID, id string
	e := s.db.QueryRowContext(ctx, "SELECT graph_id,id FROM graph_dispatches WHERE attempt_id=?", attemptID).Scan(&graphID, &id)
	if e != nil {
		return domain.GraphDispatchRecord{}, graphRowError(e)
	}
	return s.graphDispatchRecord(ctx, s.db, graphID, id)
}

func GraphStartFingerprint(in domain.GraphStartInput) string { return Fingerprint("graphs.start", in) }
func graphTargetFingerprint(op, graphID, nodeKey string, in any) string {
	return Fingerprint(op, struct {
		GraphID string
		NodeKey string
		Input   any
	}{graphID, nodeKey, in})
}
func GraphAdvanceFingerprint(graphID string, in domain.GraphAdvanceInput) string {
	return graphTargetFingerprint("graphs.advance", graphID, "", in)
}
func GraphStopFingerprint(graphID string, in domain.GraphStopInput) string {
	return graphTargetFingerprint("graphs.stop", graphID, "", in)
}
func GraphNodeStopFingerprint(graphID, nodeKey string, in domain.GraphNodeStopInput) string {
	return graphTargetFingerprint("graphs.nodes.stop", graphID, nodeKey, in)
}
func GraphGuidanceFingerprint(graphID, nodeKey string, in domain.GraphGuidanceInput) string {
	return graphTargetFingerprint("graphs.nodes.guidance", graphID, nodeKey, in)
}
func GraphBudgetAmendmentFingerprint(graphID string, in domain.GraphBudgetAmendmentInput) string {
	return graphTargetFingerprint("graphs.budget.amend", graphID, "", in)
}
