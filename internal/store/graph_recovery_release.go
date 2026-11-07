package store

import "context"

// RecoveryReleaseScope identifies a persisted member registration. It is an
// observation only: the admission transaction revalidates its full authority.
type RecoveryReleaseScope struct {
	CheckpointID string
	GraphID      string
	NodeID       string
}

// RecoveryReleaseScopes pages live registrations without scanning graph history.
func (s *Store) RecoveryReleaseScopes(ctx context.Context, ownerID, after string) ([]RecoveryReleaseScope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.QueryContext(ctx, `SELECT r.checkpoint_id,r.graph_id,r.node_id FROM recovery_graph_scopes r JOIN recovery_checkpoints c ON c.id=r.checkpoint_id WHERE r.owner_instance_id=? AND r.state='active' AND c.state='resumed' AND r.checkpoint_id>? ORDER BY r.checkpoint_id LIMIT 32`, ownerID, after)
	if err != nil {
		return nil, storageError()
	}
	defer rows.Close()
	var out []RecoveryReleaseScope
	for rows.Next() {
		var item RecoveryReleaseScope
		if err := rows.Scan(&item.CheckpointID, &item.GraphID, &item.NodeID); err != nil {
			return nil, storageError()
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, storageError()
	}
	return out, nil
}
