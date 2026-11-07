package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type graphClaimRecord struct {
	View                   domain.GraphClaimView `json:"view"`
	OwnerInstanceID        string                `json:"ownerInstanceId"`
	CapturedMaxParallelism string                `json:"capturedMaxParallelism"`
	CreatedAt              string                `json:"createdAt"`
}

func attemptExecutor(a domain.Attempt) string {
	if a.ExecutorKind == "" {
		return "native"
	}
	return a.ExecutorKind
}

func (s *Store) claimGraphScope(ctx context.Context, q queryer, taskID string) (graphID, nodeID *string, limit int, err error) {
	var graph, node string
	limit = 2
	e := q.QueryRowContext(ctx, "SELECT n.graph_id,n.id,g.max_parallelism FROM graph_nodes n JOIN task_graphs g ON g.id=n.graph_id WHERE n.task_id=?", taskID).Scan(&graph, &node, &limit)
	if e == nil {
		return ptr(graph), ptr(node), limit, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, nil, 0, storageError()
	}
	e = q.QueryRowContext(ctx, "SELECT id,max_parallelism FROM task_graphs WHERE root_task_id=?", taskID).Scan(&graph, &limit)
	if e == nil {
		return ptr(graph), nil, limit, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, nil, 0, storageError()
	}
	return nil, nil, 2, nil
}

func sameNullable(value *string, column sql.NullString) bool {
	return value == nil && !column.Valid || value != nil && column.Valid && *value == column.String
}

func (s *Store) graphClaimRecords(ctx context.Context, q queryer) ([]graphClaimRecord, error) {
	rows, e := q.QueryContext(ctx, "SELECT slot,attempt_id,owner_instance_id,context_id,captured_account_id,captured_link_revision,graph_id,legacy_barrier,data FROM capacity_claims ORDER BY slot")
	if e != nil {
		return nil, beforeCommitError(ctx, storageError())
	}
	records := []graphClaimRecord{}
	for rows.Next() {
		var slot, barrier int64
		var attempt, owner, raw string
		var contextID, accountID, graphID sql.NullString
		var link sql.NullInt64
		if e = rows.Scan(&slot, &attempt, &owner, &contextID, &accountID, &link, &graphID, &barrier, &raw); e != nil {
			rows.Close()
			return nil, storageError()
		}
		record, err := decode[graphClaimRecord](raw)
		if err != nil || (record.CapturedMaxParallelism != "1" && record.CapturedMaxParallelism != "2") {
			rows.Close()
			return nil, storageError()
		}
		if _, err = time.Parse(time.RFC3339Nano, record.CreatedAt); err != nil {
			rows.Close()
			return nil, storageError()
		}
		var linkValue *string
		if link.Valid {
			linkValue = ptr(domain.Decimal(link.Int64))
		}
		if record.View.Slot != domain.Decimal(slot) || record.View.AttemptID != attempt || record.OwnerInstanceID != owner || owner == "" || !sameNullable(record.View.ContextID, contextID) || !sameNullable(record.View.CapturedDeclaredAccountID, accountID) || !sameNullable(record.View.GraphID, graphID) || !reflect.DeepEqual(record.View.CapturedLinkRevision, linkValue) || record.View.LegacyBarrier != (barrier == 1) {
			rows.Close()
			return nil, storageError()
		}
		records = append(records, record)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, beforeCommitError(ctx, storageError())
	}
	views := make([]domain.GraphClaimView, 0, len(records))
	for _, r := range records {
		a, err := s.attempt(ctx, q, r.View.AttemptID)
		if err != nil {
			return nil, err
		}
		if a.OwnerInstanceID != r.OwnerInstanceID || attemptExecutor(a) != r.View.ExecutorKind {
			return nil, storageError()
		}
		graph, node, _, err := s.claimGraphScope(ctx, q, a.TaskID)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(graph, r.View.GraphID) || !reflect.DeepEqual(node, r.View.NodeID) {
			return nil, storageError()
		}
		views = append(views, r.View)
	}
	if domain.ValidateGraphClaims(views) != nil {
		return nil, storageError()
	}
	return records, nil
}

func (s *Store) currentClaimAccount(ctx context.Context, q queryer, contextID string) (accountID *string, revision *string, err error) {
	var account sql.NullString
	var rev int64
	var raw string
	if e := q.QueryRowContext(ctx, "SELECT account_id,revision,data FROM routing_account_links WHERE context_id=?", contextID).Scan(&account, &rev, &raw); e != nil {
		return nil, nil, storageError()
	}
	link, e := decode[domain.AccountLink](raw)
	if e != nil || link.ContextID != contextID || !validStoredRevision(rev, link.Revision) || !sameNullable(link.DeclaredAccountID, account) {
		return nil, nil, storageError()
	}
	if !account.Valid {
		return nil, nil, nil
	}
	return ptr(account.String), ptr(link.Revision), nil
}

func (s *Store) claimCapacity(ctx context.Context, tx *sql.Tx, a domain.Attempt, ownerID string) error {
	records, e := s.graphClaimRecords(ctx, tx)
	if e != nil {
		return e
	}
	used := map[string]bool{}
	for _, r := range records {
		used[r.View.Slot] = true
		if r.View.AttemptID == a.ID {
			if r.OwnerInstanceID != ownerID {
				return domain.Fail("dispatch_fenced", "This claim belongs to another owner.")
			}
			return nil
		}
		if r.View.LegacyBarrier {
			return domain.Fail("attempt_active", "A held legacy ownership barrier prevents new dispatch.")
		}
	}
	if len(records) >= 2 {
		return domain.Fail("attempt_active", "Managed capacity is occupied; keep this attempt queued.")
	}
	graph, node, limit, e := s.claimGraphScope(ctx, tx, a.TaskID)
	if e != nil {
		return e
	}
	count := 0
	for _, r := range records {
		if graph != nil && reflect.DeepEqual(graph, r.View.GraphID) {
			count++
		}
	}
	for _, r := range records {
		if graph != nil && reflect.DeepEqual(graph, r.View.GraphID) && r.CapturedMaxParallelism == "1" {
			limit = 1
		}
	}
	if graph != nil && count >= limit {
		return domain.Fail("attempt_active", "The captured graph concurrency limit is occupied.")
	}
	slot := "1"
	if used[slot] {
		slot = "2"
	}
	view := domain.GraphClaimView{Slot: slot, AttemptID: a.ID, GraphID: graph, NodeID: node, ExecutorKind: attemptExecutor(a)}
	if view.ExecutorKind == "native" {
		contextID := a.ConnectionID
		if a.ResolvedTarget != nil {
			contextID = a.ResolvedTarget.ContextID
			if contextID != a.ConnectionID {
				return storageError()
			}
		}
		if !domain.ValidContextID(contextID) {
			return domain.Fail("capability_unavailable", "No supported native context is available for this claim.")
		}
		view.ContextID = ptr(contextID)
		currentAccount, currentRevision, err := s.currentClaimAccount(ctx, tx, contextID)
		if err != nil {
			return err
		}
		view.CapturedDeclaredAccountID, view.CapturedLinkRevision = currentAccount, currentRevision
		// An admission's captured association remains a reservation even when a
		// user unlinks or relabels this context before its queued work is claimed.
		if a.ResolvedTarget != nil && a.ResolvedTarget.DeclaredAccountAssociation != nil {
			association := a.ResolvedTarget.DeclaredAccountAssociation
			view.CapturedDeclaredAccountID = ptr(association.AccountID)
			view.CapturedLinkRevision = ptr(association.LinkRevision)
		}
		candidateAccounts := []*string{view.CapturedDeclaredAccountID, currentAccount}
		for _, r := range records {
			if r.View.ContextID != nil && *r.View.ContextID == contextID {
				return domain.Fail("attempt_active", "This native context already has an owned execution.")
			}
			scopes := []*string{r.View.CapturedDeclaredAccountID}
			if r.View.ContextID != nil {
				current, _, err := s.currentClaimAccount(ctx, tx, *r.View.ContextID)
				if err != nil {
					return err
				}
				scopes = append(scopes, current)
			}
			for _, candidate := range candidateAccounts {
				for _, account := range scopes {
					if candidate != nil && account != nil && *account == *candidate {
						return domain.Fail("attempt_active", "Declared account reservation is occupied.")
					}
				}
			}
		}
	}
	views := []domain.GraphClaimView{view}
	for _, r := range records {
		views = append(views, r.View)
	}
	if domain.ValidateGraphClaims(views) != nil {
		return domain.Fail("graph_authority_required", "This task cannot claim that graph execution scope.")
	}
	record := graphClaimRecord{View: view, OwnerInstanceID: ownerID, CapturedMaxParallelism: domain.Decimal(int64(limit)), CreatedAt: domain.Now()}
	var link any
	if view.CapturedLinkRevision != nil {
		n, err := domain.ParseRevision(*view.CapturedLinkRevision)
		if err != nil {
			return err
		}
		link = n
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO capacity_claims(slot,attempt_id,owner_instance_id,context_id,captured_account_id,captured_link_revision,graph_id,legacy_barrier,data) VALUES(?,?,?,?,?,?,?,0,?)", mustRevision(slot), a.ID, ownerID, view.ContextID, view.CapturedDeclaredAccountID, link, graph, encode(record)); e != nil {
		return storageError()
	}
	return nil
}

func mustRevision(value string) int64 { n, _ := domain.ParseRevision(value); return n }
