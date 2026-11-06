package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) artifact(ctx context.Context, q queryer, id string) (domain.ArtifactRecord, error) {
	var out domain.ArtifactRecord
	var raw string
	var n int64
	e := q.QueryRowContext(ctx, "SELECT private_ref,physical_identity,byte_length,data FROM artifacts WHERE id=?", id).Scan(&out.PrivateRef, &out.PhysicalIdentity, &n, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return out, notFound()
	}
	if e != nil {
		return out, storageError()
	}
	out.ArtifactMetadata, e = decode[domain.ArtifactMetadata](raw)
	if e != nil || out.ID != id || domain.Decimal(n) != out.ByteLength || out.PrivateRef != "artifacts/"+id+".bin" || out.PhysicalIdentity == "" {
		return out, storageError()
	}
	return out, nil
}
func (s *Store) Artifact(ctx context.Context, id string) (domain.ArtifactRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.artifact(ctx, s.db, id)
}
func (s *Store) PutArtifact(ctx context.Context, in domain.ArtifactRecord) error {
	if !domain.ValidID(in.ID) || !domain.ValidID(in.AttemptID) || in.PrivateRef != "artifacts/"+in.ID+".bin" || in.PhysicalIdentity == "" {
		return domain.Invalid("owned artifact identity and private reference")
	}
	n, e := domain.ParseCount(in.ByteLength)
	if e != nil {
		return e
	}
	switch in.Kind {
	case "prompt", "stdout", "stderr", "final_response", "source_manifest", "source_file", "index_snapshot", "candidate_manifest", "candidate_delta":
	default:
		return domain.Invalid("artifact kind")
	}
	switch in.Completeness {
	case "collecting", "complete", "incomplete", "uncertain":
	default:
		return domain.Invalid("artifact completeness")
	}
	if in.SHA256 != nil && (!in.Closed || in.Completeness != "complete" || !domain.ValidSHA256(*in.SHA256)) {
		return domain.Invalid("closed complete artifact hash")
	}
	if in.Closed && in.Completeness == "complete" && in.SHA256 == nil {
		return domain.Invalid("complete artifact hash")
	}
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		a, e := s.attempt(ctx, tx, in.AttemptID)
		if e != nil {
			return e
		}
		old, e := s.artifact(ctx, tx, in.ID)
		if e != nil {
			var de *domain.Error
			if !errors.As(e, &de) || de.Code != "not_found" {
				return e
			}
		} else {
			if old.AttemptID != in.AttemptID || old.Kind != in.Kind || old.PrivateRef != in.PrivateRef || old.PhysicalIdentity != in.PhysicalIdentity || old.CreatedAt != in.CreatedAt {
				return domain.Fail("acceptance_conflict", "An artifact identity cannot be reassigned.")
			}
			if old.Closed {
				if encode(old.ArtifactMetadata) == encode(in.ArtifactMetadata) {
					return nil
				}
				return domain.Fail("acceptance_conflict", "Closed artifact metadata is immutable. Retain corruption as an incident.")
			}
			if n < intRevision(old.ByteLength) {
				return domain.Invalid("monotonically durable artifact prefix")
			}
		}
		if a.Phase == "settled" {
			return domain.Fail("attempt_active", "Evidence for a settled attempt is immutable.")
		}
		if e != nil {
			seq, e := nextSequence(ctx, tx, "artifacts")
			if e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, "INSERT INTO artifacts(sequence,id,attempt_id,private_ref,physical_identity,byte_length,data) VALUES(?,?,?,?,?,?,?)", seq, in.ID, in.AttemptID, in.PrivateRef, in.PhysicalIdentity, n, encode(in.ArtifactMetadata)); e != nil {
				return storageError()
			}
		} else {
			if _, e = tx.ExecContext(ctx, "UPDATE artifacts SET byte_length=?,data=? WHERE id=?", n, encode(in.ArtifactMetadata), in.ID); e != nil {
				return storageError()
			}
		}
		if _, e = s.appendEvent(ctx, tx, a.ID, "output_available", nil); e != nil {
			return e
		}
		return s.bumpState(ctx, tx, &a)
	})
}
func (s *Store) Artifacts(ctx context.Context, attemptID string, limit int, cursor string) (domain.Page[domain.ArtifactMetadata], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.ArtifactMetadata]{Items: []domain.ArtifactMetadata{}}
	if limit < 1 || limit > 100 {
		return out, domain.Invalid("limit")
	}
	if _, e := s.attempt(ctx, s.db, attemptID); e != nil {
		return out, e
	}
	max, after, e := pageBounds(ctx, s.db, "artifacts", attemptID, cursor)
	if e != nil {
		return out, e
	}
	rows, e := s.db.QueryContext(ctx, "SELECT sequence,data FROM artifacts WHERE attempt_id=? AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", attemptID, after, max, limit+1)
	if e != nil {
		return out, storageError()
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		var seq int64
		var raw string
		if rows.Scan(&seq, &raw) != nil {
			return out, storageError()
		}
		if len(out.Items) == limit {
			out.NextCursor = makeCursor("artifacts", attemptID, max, last)
			break
		}
		v, e := decode[domain.ArtifactMetadata](raw)
		if e != nil {
			return out, storageError()
		}
		out.Items = append(out.Items, v)
		last = seq
	}
	if rows.Err() != nil {
		return out, storageError()
	}
	return out, nil
}
func (s *Store) PutSourceSnapshot(ctx context.Context, in domain.SourceSnapshot) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		if !domain.ValidID(in.ID) || !domain.ValidSHA256(in.ManifestSHA256) {
			return domain.Invalid("source snapshot")
		}
		a, e := s.attempt(ctx, tx, in.AttemptID)
		if e != nil {
			return e
		}
		if a.Phase != "preparing" || a.LaunchIntent || a.SourceSnapshotID != nil {
			return domain.Fail("attempt_active", "The source snapshot must be recorded exactly once before launch.")
		}
		for _, id := range []string{in.ManifestArtifactID, in.IndexArtifactID} {
			artifact, e := s.artifact(ctx, tx, id)
			if e != nil {
				return e
			}
			if artifact.AttemptID != in.AttemptID || !artifact.Closed || artifact.Completeness != "complete" {
				return domain.Invalid("complete owned source artifacts")
			}
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO source_snapshots VALUES(?,?,?,?,?)", in.ID, in.AttemptID, in.ManifestArtifactID, in.IndexArtifactID, encode(in)); e != nil {
			return storageError()
		}
		a.SourceSnapshotID = ptr(in.ID)
		if _, e = s.appendEvent(ctx, tx, a.ID, "source_captured", nil); e != nil {
			return e
		}
		return s.bumpState(ctx, tx, &a)
	})
}
func (s *Store) PutCandidate(ctx context.Context, in domain.Candidate) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		if !domain.ValidID(in.ID) || strings.Contains(in.WorkspaceRef, "..") {
			return domain.Invalid("candidate identity and owned workspace reference")
		}
		a, e := s.attempt(ctx, tx, in.AttemptID)
		if e != nil {
			return e
		}
		if a.Phase == "settled" || a.CandidateID != nil || a.SourceSnapshotID == nil || *a.SourceSnapshotID != in.SourceSnapshotID {
			return domain.Fail("attempt_active", "A candidate must belong to this unsettled attempt's source snapshot.")
		}
		for _, id := range []string{in.ManifestArtifactID, in.DeltaArtifactID} {
			artifact, e := s.artifact(ctx, tx, id)
			if e != nil {
				return e
			}
			if artifact.AttemptID != in.AttemptID || !artifact.Closed {
				return domain.Invalid("closed owned candidate artifacts")
			}
		}
		in.Verification = "pending"
		in.Integration = "not_performed"
		if _, e = tx.ExecContext(ctx, "INSERT INTO candidates VALUES(?,?,?,?,?,?,?)", in.ID, in.AttemptID, in.SourceSnapshotID, in.ManifestArtifactID, in.DeltaArtifactID, in.WorkspaceRef, encode(in)); e != nil {
			return storageError()
		}
		a.CandidateID = ptr(in.ID)
		a.AuthoredDelta = in.HasDelta || in.HasIndexDelta
		if _, e = s.appendEvent(ctx, tx, a.ID, "candidate_retained", nil); e != nil {
			return e
		}
		return s.bumpState(ctx, tx, &a)
	})
}
func (s *Store) Candidate(ctx context.Context, id string) (domain.Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.candidate(ctx, s.db, id)
}
func (s *Store) candidate(ctx context.Context, q queryer, id string) (domain.Candidate, error) {
	var raw string
	e := q.QueryRowContext(ctx, "SELECT data FROM candidates WHERE id=?", id).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.Candidate{}, notFound()
	}
	if e != nil {
		return domain.Candidate{}, storageError()
	}
	c, e := decode[domain.Candidate](raw)
	if e != nil {
		return c, storageError()
	}
	return c, nil
}
