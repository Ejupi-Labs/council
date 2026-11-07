package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func insertGraphTask(ctx context.Context, tx *sql.Tx, t domain.Task, v domain.TaskVersion) error {
	sequence, e := nextSequence(ctx, tx, "tasks")
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO tasks(sequence,id,project_id,revision,state_revision,current_version,data) VALUES(?,?,?,1,1,1,?)", sequence, t.ID, t.ProjectID, encode(t)); e != nil {
		return storageError()
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO task_versions VALUES(?,1,?)", t.ID, encode(v)); e != nil {
		return storageError()
	}
	return nil
}
func graphZeroCounts(required int) domain.GraphCounts {
	return domain.GraphCounts{Required: domain.Decimal(int64(required)), Waiting: domain.Decimal(int64(required)), Ready: "0", Preparing: "0", Queued: "0", Running: "0", Blocked: "0", Cancelling: "0", Cancelled: "0", Failed: "0", Interrupted: "0", Completed: "0"}
}
func (s *Store) createGraph(ctx context.Context, ownerID string, in domain.GraphStartInput, f domain.GraphCreateFacts) domain.Result {
	if invalid := domain.ValidateGraphStartInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	return s.mutate(ctx, in.RequestID, "graphs.start", GraphStartFingerprint(in), func(tx *sql.Tx) (int, any, error) {
		g, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if e = checkGate(g); e != nil {
			return 0, nil, e
		}
		if !domain.ValidID(ownerID) || g.OwnerInstanceID != "" && g.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "The current owner must register this graph intent.")
		}
		var raw, physical string
		e = tx.QueryRowContext(ctx, "SELECT data,physical_identity FROM projects WHERE id=?", in.ProjectID).Scan(&raw, &physical)
		if errors.Is(e, sql.ErrNoRows) {
			return 0, nil, notFound()
		}
		if e != nil {
			return 0, nil, storageError()
		}
		project, e := decode[domain.Project](raw)
		if e != nil {
			return 0, nil, storageError()
		}
		if f.ProjectIdentity != physical || f.RepositoryIdentity == "" || len(f.RepositoryIdentity) > 4096 || len(f.BaseCommit) != 40 || strings.Trim(f.BaseCommit, "0123456789abcdef") != "" || f.SelectedRelativePath != "" && f.SelectedRelativePath != "." && !domain.CanonicalWorkflowPath(f.SelectedRelativePath) {
			return 0, nil, domain.Fail("project_identity_changed", "The graph creation basis does not match the retained project and repository.")
		}
		policy, e := s.policy(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if in.ExpectedPolicyRevision != "" && in.ExpectedPolicyRevision != policy.Revision {
			return 0, nil, domain.RevisionConflict(policy.Revision)
		}
		now := domain.Now()
		created, _ := domain.ParseRoutingTime(now)
		preparationDeadline := created.Add(domain.GraphPreparationSeconds * time.Second).Format("2006-01-02T15:04:05.000Z")
		graphID, rootID, inputID := domain.NewID(), domain.NewID(), domain.NewID()
		original := ""
		if in.SharedInstructions != nil {
			original = *in.SharedInstructions
		}
		root := domain.Task{ID: rootID, ProjectID: in.ProjectID, OriginalText: original, CurrentVersion: "1", Revision: "1", StateRevision: "1", CreatedAt: now, UpdatedAt: now, GraphRef: &domain.GraphRef{GraphID: graphID, RootTaskID: rootID, Kind: "root"}}
		rootInput := domain.StartTaskInput{ProjectID: in.ProjectID, OriginalText: original, PolicyOverride: in.PolicyOverride}
		rootVersion := resolvedVersion(root.ID, policy, rootInput)
		operation := newAttempt(root, rootVersion, in.RequestID, ownerID, in.ExecutionMode, "")
		operation.ConnectionID = "local-graph"
		operation.ExecutorKind = "local_graph"
		operation.Role = "graph_basis"
		operation.RequestedTarget = domain.ExecutionPins{}
		operation.ResolvedTarget = nil
		operation.DeadlineAt = preparationDeadline
		root.LatestAttemptID = ptr(operation.ID)
		if e = insertGraphTask(ctx, tx, root, rootVersion); e != nil {
			return 0, nil, e
		}
		authority, e := s.captureRouting(ctx, tx, root, rootVersion, in.Routing, wrapperRoutingFacts(in.ExecutionMode), nil, "graph")
		if e != nil {
			return 0, nil, e
		}
		budget, e := s.initialBudget(ctx, tx, authority, in.RequestID)
		if e != nil {
			return 0, nil, e
		}
		parallel := in.MaxParallelism
		if parallel == "" {
			parallel = "2"
		}
		h := graphHeader{Summary: domain.GraphSummary{ID: graphID, RootTaskID: rootID, ProjectID: in.ProjectID, GraphVersion: "1", ControlRevision: "1", StateRevision: "1", BudgetAuthorityEpoch: "1", MaxParallelism: parallel, State: "preparing", NextAction: "Prepare and retain the exact graph source basis.", NextOwner: "council", Counts: graphZeroCounts(len(in.Nodes)), CreatedAt: now, UpdatedAt: now}, CreateFacts: f, PreparationDeadlineAt: preparationDeadline}
		declarationHash := sha256.Sum256([]byte(encode(in)))
		var eventHighwater int64
		if e = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(sequence),0) FROM attempt_events").Scan(&eventHighwater); e != nil {
			return 0, nil, storageError()
		}
		h.DispatchEventHighWater = domain.Decimal(eventHighwater)
		h.DispatchRequestID = in.RequestID
		h.Definition = domain.GraphDefinitionRef{GraphID: graphID, GraphVersion: "1", DeclarationSHA256: hex.EncodeToString(declarationHash[:])}
		h.DispatchAuthority = &domain.GraphDispatchAuthorityView{OwnerInstanceID: ownerID, Generation: "1", GraphControlRevision: "1", BudgetAuthorityEpoch: "1", WindowNumber: budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}, State: "active"}
		sequence, e := nextSequence(ctx, tx, "task_graphs")
		if e != nil {
			return 0, nil, e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO task_graphs(sequence,id,root_task_id,project_id,graph_version,control_revision,state_revision,budget_epoch,max_parallelism,state,dispatch_owner_instance_id,dispatch_generation,dispatch_data,data) VALUES(?,?,?,?,1,1,1,1,?,'preparing',?,1,?,?)", sequence, graphID, rootID, in.ProjectID, intRevision(parallel), ownerID, encode(h.DispatchAuthority), encode(h)); e != nil {
			return 0, nil, storageError()
		}
		preference, e := s.deliveryPreference(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		delivery := domain.DeliveryScope{LocalMode: "review_only", Source: "graph_scope", ProjectID: in.ProjectID, ProjectIdentity: f.ProjectIdentity, RepositoryIdentity: f.RepositoryIdentity, SelectedRelativePath: f.SelectedRelativePath, Branch: f.Branch, Destination: project.ResolvedPath}
		if in.FinalNodeKey != "" {
			delivery.LocalMode = preference.LocalMode
			delivery.PreferenceRevision = preference.Revision
			delivery.Source = "saved_preference"
		}
		if in.Delivery != nil {
			if in.Delivery.LocalMode != "" {
				delivery.LocalMode = in.Delivery.LocalMode
			}
			delivery.ExternalAction = in.Delivery.ExternalAction
			delivery.Source = "task_scope"
		}
		definition := domain.GraphDefinition{GraphID: graphID, GraphVersion: "1", RootTaskID: rootID, Original: in, RequestedPolicy: policy, RoutingPolicy: authority.PolicySnapshot, Delivery: delivery, CreatedAt: now}
		if _, e = tx.ExecContext(ctx, "INSERT INTO graph_versions VALUES(?,1,?,?)", graphID, in.RequestID, encode(definition)); e != nil {
			return 0, nil, storageError()
		}
		for ordinal, node := range in.Nodes {
			nodeID, taskID := domain.NewID(), domain.NewID()
			task := domain.Task{ID: taskID, ProjectID: in.ProjectID, OriginalText: node.OriginalText, CurrentVersion: "1", Revision: "1", StateRevision: "1", CreatedAt: now, UpdatedAt: now, GraphRef: &domain.GraphRef{GraphID: graphID, NodeID: ptr(nodeID), RootTaskID: rootID, Kind: "node"}}
			version := resolvedVersion(taskID, policy, domain.StartTaskInput{Pins: node.Pins, Acceptance: node.Acceptance, TimeoutSeconds: node.TimeoutSeconds, PolicyOverride: in.PolicyOverride})
			if e = insertGraphTask(ctx, tx, task, version); e != nil {
				return 0, nil, e
			}
			n := graphNodeRecord{Summary: domain.GraphNodeSummary{ID: nodeID, GraphID: graphID, Key: node.Key, TaskID: taskID, TaskVersion: "1", TaskRevision: "1", ControlRevision: "1", StateRevision: "1", State: "waiting", NextAction: "Wait for the exact retained graph input.", NextOwner: "council"}, Ordinal: domain.Decimal(int64(ordinal + 1)), NativeDebits: "0", AutomaticDebits: "0"}
			if _, e = tx.ExecContext(ctx, "INSERT INTO graph_nodes(id,graph_id,node_key,task_id,ordinal,control_revision,state_revision,native_debits,automatic_debits,state,data) VALUES(?,?,?,?,?,1,1,0,0,'waiting',?)", nodeID, graphID, node.Key, taskID, ordinal+1, encode(n)); e != nil {
				return 0, nil, storageError()
			}
			h.DispatchAuthority.NodeAuthorities = append(h.DispatchAuthority.NodeAuthorities, domain.GraphNodeAuthority{NodeID: nodeID, TaskVersion: "1", NodeControlRevision: "1"})
			if node.Key == in.FinalNodeKey {
				h.Summary.FinalNodeID = ptr(nodeID)
			}
		}
		if e = s.insertAttempt(ctx, tx, operation); e != nil {
			return 0, nil, e
		}
		sequence, e = nextSequence(ctx, tx, "graph_inputs")
		if e != nil {
			return 0, nil, e
		}
		control := domain.GraphDispatchBinding{GraphID: graphID, GraphVersion: "1", RootTaskID: rootID, OperationAttemptID: operation.ID, OwnerInstanceID: ownerID, Generation: "1", GraphControlRevision: "1", BudgetAuthorityEpoch: "1", WindowNumber: budget.CurrentWindow.Number, ContributorVector: []domain.GraphContributorBinding{}}
		record := domain.GraphInputRecord{Sequence: domain.Decimal(sequence), Public: domain.GraphInputDetail{Summary: domain.GraphInputSummary{ID: inputID, GraphID: graphID, GraphVersion: "1", RootTaskID: rootID, OperationAttemptID: operation.ID, Kind: "root_basis", State: "preparing", Current: true, CreatedAt: now, UpdatedAt: now}}, ExpectedAuthority: control}
		if _, e = tx.ExecContext(ctx, "INSERT INTO graph_inputs(sequence,id,graph_id,graph_version,operation_attempt_id,kind,state,data) VALUES(?,?,?,1,?,'root_basis','preparing',?)", sequence, inputID, graphID, operation.ID, encode(record)); e != nil {
			return 0, nil, storageError()
		}
		h.Summary.RootInputID = ptr(inputID)
		if e = saveGraphHeader(ctx, tx, h); e != nil {
			return 0, nil, e
		}
		detail, e := s.graphDetail(ctx, tx, graphID)
		if e != nil {
			return 0, nil, e
		}
		return 201, domain.GraphAdmission{Graph: detail.Graph, Definition: detail.Definition, Nodes: detail.Nodes, PreparationAttemptID: operation.ID, Budget: detail.Budget, DispatchAuthority: *h.DispatchAuthority}, nil
	})
}
