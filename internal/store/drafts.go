package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) CreateDraft(ctx context.Context, id, fp, projectID, text string) domain.Result {
	return s.mutate(ctx, id, "drafts.create", fp, func(tx *sql.Tx) (int, any, error) {
		var exists int
		e := tx.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE id=?", projectID).Scan(&exists)
		if errors.Is(e, sql.ErrNoRows) {
			return 0, nil, domain.Fail("not_found", "The saved project does not exist. Choose an admitted project.")
		}
		if e != nil {
			return 0, nil, storageError()
		}
		p, e := s.policy(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		seq, e := nextSequence(ctx, tx, "drafts")
		if e != nil {
			return 0, nil, e
		}
		now := domain.Now()
		d := domain.Draft{ID: domain.NewID(), ProjectID: projectID, OriginalText: text, Revision: "1", Status: "draft", Selection: "auto", RequestedPolicy: p, CreatedAt: now, UpdatedAt: now}
		if _, e = tx.ExecContext(ctx, "INSERT INTO drafts(sequence,id,project_id,revision) VALUES(?,?,?,1)", seq, d.ID, projectID); e != nil {
			return 0, nil, storageError()
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO draft_versions(draft_id,revision,data) VALUES(?,1,?)", d.ID, encode(d)); e != nil {
			return 0, nil, storageError()
		}
		return 201, d, nil
	})
}
func (s *Store) draft(ctx context.Context, q queryer, id, revision string) (domain.Draft, error) {
	var raw string
	var n int64
	var e error
	if revision == "" {
		e = q.QueryRowContext(ctx, "SELECT v.revision,v.data FROM drafts d JOIN draft_versions v ON v.draft_id=d.id AND v.revision=d.revision WHERE d.id=?", id).Scan(&n, &raw)
	} else {
		n, _ = domain.ParseRevision(revision)
		e = q.QueryRowContext(ctx, "SELECT revision,data FROM draft_versions WHERE draft_id=? AND revision=?", id, n).Scan(&n, &raw)
	}
	if errors.Is(e, sql.ErrNoRows) {
		return domain.Draft{}, domain.Fail("not_found", "The saved draft or version does not exist. Reload the draft list.")
	}
	if e != nil {
		return domain.Draft{}, storageError()
	}
	d, e := decode[domain.Draft](raw)
	if e != nil || !validStoredRevision(n, d.Revision) {
		return domain.Draft{}, storageError()
	}
	return d, nil
}
func (s *Store) Draft(ctx context.Context, id, revision string) (domain.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.draft(ctx, s.db, id, revision)
}
func (s *Store) UpdateDraft(ctx context.Context, id, fp, draftID, expected, text string, useCurrent bool) domain.Result {
	return s.mutate(ctx, id, "drafts.update", fp, func(tx *sql.Tx) (int, any, error) {
		d, e := s.draft(ctx, tx, draftID, "")
		if e != nil {
			return 0, nil, e
		}
		if expected != d.Revision {
			return 0, nil, domain.RevisionConflict(d.Revision)
		}
		revision, e := increment(d.Revision)
		if e != nil {
			return 0, nil, e
		}
		if useCurrent {
			d.RequestedPolicy, e = s.policy(ctx, tx)
			if e != nil {
				return 0, nil, e
			}
		}
		d.OriginalText = text
		d.Revision = revision
		d.UpdatedAt = domain.Now()
		n, _ := domain.ParseRevision(revision)
		if _, e = tx.ExecContext(ctx, "INSERT INTO draft_versions(draft_id,revision,data) VALUES(?,?,?)", draftID, n, encode(d)); e != nil {
			return 0, nil, storageError()
		}
		if _, e = tx.ExecContext(ctx, "UPDATE drafts SET revision=? WHERE id=?", n, draftID); e != nil {
			return 0, nil, storageError()
		}
		return 200, d, nil
	})
}
func (s *Store) Drafts(ctx context.Context, projectID string, limit int, cursor string) (domain.Page[domain.DraftSummary], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.DraftSummary]{Items: []domain.DraftSummary{}}
	max, after, e := pageBounds(ctx, s.db, "drafts", projectID, cursor)
	if e != nil {
		return out, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT d.sequence,v.revision,v.data FROM drafts d JOIN draft_versions v ON v.draft_id=d.id AND v.revision=d.revision WHERE d.sequence>? AND d.sequence<=? AND (?='' OR d.project_id=?) ORDER BY d.sequence ASC LIMIT ?`, after, max, projectID, projectID, limit+1)
	if e != nil {
		return out, storageError()
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		var seq, n int64
		var raw string
		if e = rows.Scan(&seq, &n, &raw); e != nil {
			return out, storageError()
		}
		if len(out.Items) == limit {
			out.NextCursor = makeCursor("drafts", projectID, max, last)
			break
		}
		d, e := decode[domain.Draft](raw)
		if e != nil || !validStoredRevision(n, d.Revision) {
			return out, storageError()
		}
		out.Items = append(out.Items, d.Summary())
		last = seq
	}
	if e = rows.Err(); e != nil {
		return out, storageError()
	}
	return out, nil
}
