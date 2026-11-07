package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphRowError(e error) error {
	if errors.Is(e, sql.ErrNoRows) {
		return notFound()
	}
	return storageError()
}

type graphPageIdentity struct {
	sequence int64
	id       string
}

func graphPageIDs(ctx context.Context, q queryer, query string, args ...any) ([]graphPageIdentity, error) {
	rows, e := q.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, storageError()
	}
	items := []graphPageIdentity{}
	for rows.Next() {
		var item graphPageIdentity
		if rows.Scan(&item.sequence, &item.id) != nil {
			rows.Close()
			return nil, storageError()
		}
		items = append(items, item)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, storageError()
	}
	return items, nil
}
func (s *Store) graphs(ctx context.Context, projectID string, limit int, cursor string) (domain.Page[domain.GraphSummary], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.GraphSummary]{Items: []domain.GraphSummary{}}
	if limit < 1 || limit > 100 || projectID != "" && !domain.ValidID(projectID) {
		return out, domain.Invalid("bounded graph page/project")
	}
	max, after, e := pageBounds(ctx, s.db, "task_graphs", projectID, cursor)
	if e != nil {
		return out, e
	}
	ids, e := graphPageIDs(ctx, s.db, "SELECT sequence,id FROM task_graphs WHERE sequence>? AND sequence<=? AND (?='' OR project_id=?) ORDER BY sequence LIMIT ?", after, max, projectID, projectID, limit+1)
	if e != nil {
		return out, e
	}
	last := after
	for i, item := range ids {
		if i == limit {
			out.NextCursor = makeCursor("task_graphs", projectID, max, last)
			break
		}
		h, e := s.graphHeader(ctx, s.db, item.id)
		if e != nil {
			return out, e
		}
		nodes, e := s.graphNodes(ctx, s.db, item.id)
		if e != nil {
			return out, e
		}
		h.Summary.Counts, e = graphCounts(nodes)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, h.Summary)
		last = item.sequence
	}
	return out, nil
}
func (s *Store) graphInputs(ctx context.Context, graphID string, limit int, cursor string) (domain.Page[domain.GraphInputSummary], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.GraphInputSummary]{Items: []domain.GraphInputSummary{}}
	if limit < 1 || limit > 100 {
		return out, domain.Invalid("bounded graph input page")
	}
	h, e := s.graphHeader(ctx, s.db, graphID)
	if e != nil {
		return out, e
	}
	filter := graphID + "|" + h.Summary.GraphVersion
	max, after, e := pageBounds(ctx, s.db, "graph_inputs", filter, cursor)
	if e != nil {
		return out, e
	}
	ids, e := graphPageIDs(ctx, s.db, "SELECT sequence,id FROM graph_inputs WHERE graph_id=? AND graph_version=? AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", graphID, intRevision(h.Summary.GraphVersion), after, max, limit+1)
	if e != nil {
		return out, e
	}
	last := after
	for i, item := range ids {
		if i == limit {
			out.NextCursor = makeCursor("graph_inputs", filter, max, last)
			break
		}
		record, e := s.graphInputRecord(ctx, s.db, graphID, item.id)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, record.Public.Summary)
		last = item.sequence
	}
	return out, nil
}
func (s *Store) graphDispatchRecord(ctx context.Context, q queryer, graphID, dispatchID string) (domain.GraphDispatchRecord, error) {
	var sequence int64
	var raw, attempt, node, input string
	e := q.QueryRowContext(ctx, "SELECT sequence,attempt_id,node_id,input_id,data FROM graph_dispatches WHERE id=? AND graph_id=?", dispatchID, graphID).Scan(&sequence, &attempt, &node, &input, &raw)
	if e != nil {
		return domain.GraphDispatchRecord{}, graphRowError(e)
	}
	record, e := decode[domain.GraphDispatchRecord](raw)
	d := record.Public.Dispatch
	if e != nil || record.Sequence != domain.Decimal(sequence) || d.ID != dispatchID || d.GraphID != graphID || d.AttemptID != attempt || d.NodeID != node || d.InputID != input {
		return record, storageError()
	}
	return record, nil
}
func (s *Store) graphDispatches(ctx context.Context, graphID, inputID, nodeKey string, limit int, cursor string) (domain.Page[domain.GraphDispatchView], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.GraphDispatchView]{Items: []domain.GraphDispatchView{}}
	if limit < 1 || limit > 100 || inputID != "" && !domain.ValidID(inputID) || nodeKey != "" && !domain.GraphKey(nodeKey) {
		return out, domain.Invalid("bounded exact graph dispatch filters")
	}
	h, e := s.graphHeader(ctx, s.db, graphID)
	if e != nil {
		return out, e
	}
	filter := graphID + "|" + h.Summary.GraphVersion + "|" + inputID + "|" + nodeKey
	if inputID != "" {
		if _, e = s.graphInputRecord(ctx, s.db, graphID, inputID); e != nil {
			return out, e
		}
	}
	nodeID := ""
	if nodeKey != "" {
		nodes, e := s.graphNodes(ctx, s.db, graphID)
		if e != nil {
			return out, e
		}
		for _, n := range nodes {
			if n.Summary.Key == nodeKey {
				nodeID = n.Summary.ID
			}
		}
		if nodeID == "" {
			return out, notFound()
		}
	}
	max, after, e := pageBounds(ctx, s.db, "graph_dispatches", filter, cursor)
	if e != nil {
		return out, e
	}
	ids, e := graphPageIDs(ctx, s.db, "SELECT sequence,id FROM graph_dispatches WHERE graph_id=? AND graph_version=? AND (?='' OR input_id=?) AND (?='' OR node_id=?) AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", graphID, intRevision(h.Summary.GraphVersion), inputID, inputID, nodeID, nodeID, after, max, limit+1)
	if e != nil {
		return out, e
	}
	last := after
	bytes := 128
	for i, item := range ids {
		record, e := s.graphDispatchRecord(ctx, s.db, graphID, item.id)
		if e != nil {
			return out, e
		}
		size := len(encode(record.Public)) + 1
		if size+128 > 4<<20 {
			return out, domain.Fail("evidence_unavailable", "This complete graph dispatch exceeds its bounded read envelope.")
		}
		if i == limit || bytes+size > 4<<20 {
			out.NextCursor = makeCursor("graph_dispatches", filter, max, last)
			break
		}
		out.Items = append(out.Items, record.Public)
		bytes += size
		last = item.sequence
	}
	return out, nil
}
