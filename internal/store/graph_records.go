package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Private records keep captured physical/control facts separate from public
// projections. A read never grants dispatch or edits a historical declaration.
type graphHeader struct {
	DispatchEventHighWater string                             `json:"dispatchEventHighWater"`
	DispatchRequestID      string                             `json:"dispatchRequestId"`
	Summary                domain.GraphSummary                `json:"summary"`
	Definition             domain.GraphDefinitionRef          `json:"definition"`
	CreateFacts            domain.GraphCreateFacts            `json:"createFacts"`
	DispatchAuthority      *domain.GraphDispatchAuthorityView `json:"dispatchAuthority"`
	PreparationDeadlineAt  string                             `json:"preparationDeadlineAt"`
}
type graphNodeRecord struct {
	Blocker         *domain.GraphBlockerSummary `json:"blocker"`
	Summary         domain.GraphNodeSummary     `json:"summary"`
	Ordinal         string                      `json:"ordinal"`
	NativeDebits    string                      `json:"nativeDebits"`
	AutomaticDebits string                      `json:"automaticDebits"`
}

func (s *Store) graphMembership(ctx context.Context, q queryer, taskID string) (*domain.GraphRef, error) {
	return graphTaskMembership(ctx, q, taskID)
}
func graphTaskMembership(ctx context.Context, q queryer, taskID string) (*domain.GraphRef, error) {
	var graph, root, node string
	e := q.QueryRowContext(ctx, "SELECT id,root_task_id FROM task_graphs WHERE root_task_id=?", taskID).Scan(&graph, &root)
	if e == nil {
		return &domain.GraphRef{GraphID: graph, RootTaskID: root, Kind: "root"}, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, storageError()
	}
	e = q.QueryRowContext(ctx, "SELECT n.graph_id,g.root_task_id,n.id FROM graph_nodes n JOIN task_graphs g ON g.id=n.graph_id WHERE n.task_id=?", taskID).Scan(&graph, &root, &node)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, storageError()
	}
	return &domain.GraphRef{GraphID: graph, RootTaskID: root, NodeID: ptr(node), Kind: "node"}, nil
}
func graphScopeError(ref *domain.GraphRef) *domain.Error {
	e := domain.Fail("graph_scope_required", "This task belongs to a graph. Use its graph controls and shared budget.")
	e.GraphRef = ref
	e.GraphBudgetRef = &domain.GraphBudgetRef{GraphID: ref.GraphID, BudgetOwnerTaskID: ref.RootTaskID}
	return e
}

// CheckStandaloneAuthority is a readonly early guard. Mutation transactions
// repeat the same check after exact receipt replay and before committing changes.
func (s *Store) CheckStandaloneAuthority(ctx context.Context, taskID string, budget bool, requestedVersion ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return e
	}
	return beforeCommitError(ctx, s.requireStandaloneAuthority(ctx, s.db, taskID, budget, requestedVersion...))
}
func (s *Store) requireStandaloneAuthority(ctx context.Context, q queryer, taskID string, budget bool, requestedVersion ...string) error {
	ref, e := s.graphMembership(ctx, q, taskID)
	if e != nil {
		return e
	}
	if ref == nil {
		return nil
	}
	if budget {
		return graphBudgetError(ref)
	}
	if len(requestedVersion) != 0 && ref.Kind == "node" && ref.NodeID != nil {
		if len(requestedVersion) != 1 {
			return domain.Invalid("one requested task version")
		}
		if _, invalid := domain.ParseRevision(requestedVersion[0]); invalid != nil {
			return invalid
		}
		task, e := s.task(ctx, q, taskID)
		if e != nil {
			return e
		}
		stale := requestedVersion[0] != task.CurrentVersion
		if !stale {
			var input sql.NullString
			if e = q.QueryRowContext(ctx, "SELECT current_input_id FROM graph_nodes WHERE id=? AND graph_id=?", *ref.NodeID, ref.GraphID).Scan(&input); e != nil {
				return storageError()
			}
			if input.Valid {
				record, e := s.graphInputRecord(ctx, q, ref.GraphID, input.String)
				if e != nil {
					return e
				}
				stale = !record.Public.Summary.Current
			}
		}
		if stale {
			error := graphScopeError(ref)
			error.Code, error.Message = "graph_input_stale", "This requested graph version or captured input is stale. Inspect current dependencies and use explicit graph Advance/rebind."
			return error
		}
	}
	return graphScopeError(ref)
}
func graphBudgetError(ref *domain.GraphRef) *domain.Error {
	e := graphScopeError(ref)
	e.Code = "graph_budget_required"
	return e
}
func (s *Store) graphHeader(ctx context.Context, q queryer, id string) (graphHeader, error) {
	var raw, root, project, state string
	var version, control, revision, epoch, parallel int64
	e := q.QueryRowContext(ctx, "SELECT root_task_id,project_id,graph_version,control_revision,state_revision,budget_epoch,max_parallelism,state,data FROM task_graphs WHERE id=?", id).Scan(&root, &project, &version, &control, &revision, &epoch, &parallel, &state, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return graphHeader{}, notFound()
	}
	if e != nil {
		return graphHeader{}, storageError()
	}
	h, e := decode[graphHeader](raw)
	if e != nil || h.Summary.ID != id || h.Summary.RootTaskID != root || h.Summary.ProjectID != project || h.Summary.State != state || !validStoredRevision(version, h.Summary.GraphVersion) || !validStoredRevision(control, h.Summary.ControlRevision) || !validStoredRevision(revision, h.Summary.StateRevision) || !validStoredRevision(epoch, h.Summary.BudgetAuthorityEpoch) || domain.Decimal(parallel) != h.Summary.MaxParallelism {
		return h, storageError()
	}
	return h, nil
}
func saveGraphHeader(ctx context.Context, tx *sql.Tx, h graphHeader) error {
	var owner any
	var dispatch any
	generation := "1"
	if h.DispatchAuthority != nil {
		owner = h.DispatchAuthority.OwnerInstanceID
		generation = h.DispatchAuthority.Generation
		dispatch = encode(h.DispatchAuthority)
	}
	_, e := tx.ExecContext(ctx, "UPDATE task_graphs SET graph_version=?,control_revision=?,state_revision=?,budget_epoch=?,state=?,root_input_id=?,final_node_id=?,dispatch_owner_instance_id=?,dispatch_generation=?,dispatch_data=?,data=? WHERE id=?", intRevision(h.Summary.GraphVersion), intRevision(h.Summary.ControlRevision), intRevision(h.Summary.StateRevision), intRevision(h.Summary.BudgetAuthorityEpoch), h.Summary.State, h.Summary.RootInputID, h.Summary.FinalNodeID, owner, intRevision(generation), dispatch, encode(h), h.Summary.ID)
	if e != nil {
		return storageError()
	}
	return nil
}
func (s *Store) graphNodes(ctx context.Context, q queryer, graphID string) ([]graphNodeRecord, error) {
	rows, e := q.QueryContext(ctx, "SELECT id,task_id,ordinal,control_revision,state_revision,native_debits,automatic_debits,state,data FROM graph_nodes WHERE graph_id=? ORDER BY ordinal", graphID)
	if e != nil {
		return nil, storageError()
	}
	out := []graphNodeRecord{}
	for rows.Next() {
		var id, task, state, raw string
		var ordinal, control, revision, native, automatic int64
		if e = rows.Scan(&id, &task, &ordinal, &control, &revision, &native, &automatic, &state, &raw); e != nil {
			rows.Close()
			return nil, storageError()
		}
		n, err := decode[graphNodeRecord](raw)
		if err != nil || n.Summary.ID != id || n.Summary.GraphID != graphID || n.Summary.TaskID != task || n.Summary.State != state || n.Ordinal != domain.Decimal(ordinal) || n.NativeDebits != domain.Decimal(native) || n.AutomaticDebits != domain.Decimal(automatic) || !validStoredRevision(control, n.Summary.ControlRevision) || !validStoredRevision(revision, n.Summary.StateRevision) {
			rows.Close()
			return nil, storageError()
		}
		out = append(out, n)
	}
	e = rows.Err()
	rows.Close()
	if e != nil || len(out) > domain.MaxGraphNodes {
		return nil, storageError()
	}
	return out, nil
}
func (s *Store) graphDefinition(ctx context.Context, q queryer, graphID, version string) (domain.GraphDefinition, error) {
	var raw string
	e := q.QueryRowContext(ctx, "SELECT data FROM graph_versions WHERE graph_id=? AND version=?", graphID, intRevision(version)).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.GraphDefinition{}, notFound()
	}
	if e != nil {
		return domain.GraphDefinition{}, storageError()
	}
	d, e := decode[domain.GraphDefinition](raw)
	if e != nil || d.GraphID != graphID || d.GraphVersion != version {
		return d, storageError()
	}
	return d, nil
}

// taskBudget is the sole internal budget owner resolver. Usage/token queries do
// not call it; a graph node always keeps its actual task/attempt identity.
func (s *Store) taskBudget(ctx context.Context, q queryer, taskID string) (*domain.TaskBudget, error) {
	ref, e := s.graphMembership(ctx, q, taskID)
	if e != nil {
		return nil, e
	}
	if ref != nil {
		taskID = ref.RootTaskID
	}
	return s.rawTaskBudget(ctx, q, taskID)
}
func (s *Store) graphBudgetView(ctx context.Context, q queryer, h graphHeader) (domain.GraphBudgetView, error) {
	out := domain.GraphBudgetView{GraphID: h.Summary.ID, BudgetOwnerTaskID: h.Summary.RootTaskID, BudgetAuthorityEpoch: h.Summary.BudgetAuthorityEpoch, NodeConsumption: []domain.GraphNodeConsumption{}}
	b, e := s.rawTaskBudget(ctx, q, h.Summary.RootTaskID)
	if e != nil {
		return out, e
	}
	if b == nil {
		return out, storageError()
	}
	out.Budget = *b
	nodes, e := s.graphNodes(ctx, q, h.Summary.ID)
	if e != nil {
		return out, e
	}
	for _, n := range nodes {
		out.NodeConsumption = append(out.NodeConsumption, domain.GraphNodeConsumption{NodeID: n.Summary.ID, TaskID: n.Summary.TaskID, ConsumedAdmissions: n.NativeDebits, AutomaticAdmissions: n.AutomaticDebits})
	}
	return out, nil
}
func (s *Store) graphDetail(ctx context.Context, q queryer, id string) (domain.GraphDetail, error) {
	h, e := s.graphHeader(ctx, q, id)
	if e != nil {
		return domain.GraphDetail{}, e
	}
	out := domain.GraphDetail{Graph: h.Summary, Definition: h.Definition, DispatchAuthority: h.DispatchAuthority, Nodes: []domain.GraphNodeSummary{}, ActiveClaims: []domain.GraphClaimView{}, Blockers: []domain.GraphBlockerSummary{}}
	nodes, e := s.graphNodes(ctx, q, id)
	if e != nil {
		return out, e
	}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, n.Summary)
		if n.Blocker != nil {
			out.Blockers = append(out.Blockers, *n.Blocker)
		}
	}
	out.Budget, e = s.graphBudgetView(ctx, q, h)
	if e != nil {
		return out, e
	}
	claims, e := s.graphClaims(ctx, q)
	if e != nil {
		return out, e
	}
	for _, claim := range claims {
		if claim.GraphID != nil && *claim.GraphID == id {
			out.ActiveClaims = append(out.ActiveClaims, claim)
		}
	}
	return out, nil
}
func (s *Store) graphInputRecord(ctx context.Context, q queryer, graphID, inputID string) (domain.GraphInputRecord, error) {
	var raw, graph, attempt, kind, state string
	var sequence int64
	e := q.QueryRowContext(ctx, "SELECT sequence,graph_id,operation_attempt_id,kind,state,data FROM graph_inputs WHERE id=? AND graph_id=?", inputID, graphID).Scan(&sequence, &graph, &attempt, &kind, &state, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.GraphInputRecord{}, notFound()
	}
	if e != nil {
		return domain.GraphInputRecord{}, storageError()
	}
	r, e := decode[domain.GraphInputRecord](raw)
	if e != nil || r.Sequence != domain.Decimal(sequence) || r.Public.Summary.ID != inputID || r.Public.Summary.GraphID != graph || r.Public.Summary.OperationAttemptID != attempt || r.Public.Summary.Kind != kind || r.Public.Summary.State != state {
		return r, storageError()
	}
	if r.ExpectedAuthority.GraphID != graph || r.ExpectedAuthority.OperationAttemptID != attempt || r.ExpectedAuthority.RootTaskID != r.Public.Summary.RootTaskID {
		return r, storageError()
	}
	if r.Public.Summary.ReceivingNodeID != nil {
		if r.Public.Summary.ReceivingTaskID == nil || r.Public.Summary.ReceivingTaskVersion == nil || r.ExpectedAuthority.NodeID == nil || r.ExpectedAuthority.TaskID == nil || r.ExpectedAuthority.TaskVersion == nil || r.ExpectedAuthority.NodeControlRevision == nil || *r.Public.Summary.ReceivingNodeID != *r.ExpectedAuthority.NodeID || *r.Public.Summary.ReceivingTaskID != *r.ExpectedAuthority.TaskID || *r.Public.Summary.ReceivingTaskVersion != *r.ExpectedAuthority.TaskVersion {
			return r, storageError()
		}
	}
	if r.PrivateRoot != nil {
		if domain.ValidateGraphRootBinding(*r.PrivateRoot) != nil || r.PrivateRoot.GraphID != graph || r.PrivateRoot.RootTaskID != r.Public.Summary.RootTaskID {
			return r, storageError()
		}
		digest, invalid := domain.GraphRootDigest(*r.PrivateRoot)
		if invalid != nil || digest != r.PrivateRoot.BasisSHA256 {
			return r, storageError()
		}
	}
	return r, nil
}
