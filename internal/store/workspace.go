package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) OpenProject(ctx context.Context, id, fp string, selection domain.OpenSelection, resolved domain.ResolvedProject) domain.Result {
	return s.mutate(ctx, id, "projects.open", fp, func(tx *sql.Tx) (int, any, error) {
		var raw string
		e := tx.QueryRowContext(ctx, "SELECT data FROM projects WHERE physical_identity=?", resolved.PhysicalIdentity).Scan(&raw)
		if e == nil {
			p, e := decode[domain.Project](raw)
			if e != nil {
				return 0, nil, storageError()
			}
			return 200, domain.ProjectOpen{Project: p, OpenSelection: selection, Observation: resolved.Observation}, nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return 0, nil, storageError()
		}
		var count int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM projects WHERE resolved_path=?", resolved.Observation.ResolvedPath).Scan(&count); e != nil {
			return 0, nil, storageError()
		}
		if count != 0 {
			return 0, nil, domain.Fail("project_identity_changed", "This saved project folder has a different physical identity. Restore the original folder before reopening it.")
		}
		seq, e := nextSequence(ctx, tx, "projects")
		if e != nil {
			return 0, nil, e
		}
		now := domain.Now()
		o := resolved.Observation
		p := domain.Project{ID: domain.NewID(), SelectedPath: selection.SelectedPath, ResolvedPath: o.ResolvedPath, RepositoryRoot: o.RepositoryRoot, Branch: o.Branch, Commit: o.Commit, CreatedAt: now, UpdatedAt: now, ObservedAt: o.ObservedAt, Revision: "1"}
		if _, e = tx.ExecContext(ctx, "INSERT INTO projects(sequence,id,physical_identity,resolved_path,data) VALUES(?,?,?,?,?)", seq, p.ID, resolved.PhysicalIdentity, p.ResolvedPath, encode(p)); e != nil {
			return 0, nil, storageError()
		}
		return 201, domain.ProjectOpen{Project: p, OpenSelection: selection, Observation: o}, nil
	})
}
func (s *Store) Project(ctx context.Context, id string) (domain.Project, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw, physical string
	e := s.db.QueryRowContext(ctx, "SELECT data,physical_identity FROM projects WHERE id=?", id).Scan(&raw, &physical)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.Project{}, "", domain.Fail("not_found", "The saved project does not exist. Choose an admitted project.")
	}
	if e != nil {
		return domain.Project{}, "", storageError()
	}
	p, e := decode[domain.Project](raw)
	if e != nil {
		return domain.Project{}, "", storageError()
	}
	return p, physical, nil
}
func (s *Store) Projects(ctx context.Context, limit int, cursor string) (domain.Page[domain.Project], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.Project]{Items: []domain.Project{}}
	max, after, e := pageBounds(ctx, s.db, "projects", "", cursor)
	if e != nil {
		return out, e
	}
	rows, e := s.db.QueryContext(ctx, "SELECT sequence,data FROM projects WHERE sequence>? AND sequence<=? ORDER BY sequence ASC LIMIT ?", after, max, limit+1)
	if e != nil {
		return out, storageError()
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		var seq int64
		var raw string
		if e = rows.Scan(&seq, &raw); e != nil {
			return out, storageError()
		}
		if len(out.Items) == limit {
			out.NextCursor = makeCursor("projects", "", max, last)
			break
		}
		p, e := decode[domain.Project](raw)
		if e != nil {
			return out, storageError()
		}
		out.Items = append(out.Items, p)
		last = seq
	}
	if e = rows.Err(); e != nil {
		return out, storageError()
	}
	return out, nil
}

// ProjectByIdentity retrieves an admission for read-only prerequisite checks outside transactions.
func (s *Store) ProjectByIdentity(ctx context.Context, identity string) (*domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw string
	e := s.db.QueryRowContext(ctx, "SELECT data FROM projects WHERE physical_identity=?", identity).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, storageError()
	}
	p, e := decode[domain.Project](raw)
	if e != nil {
		return nil, storageError()
	}
	return &p, nil
}
