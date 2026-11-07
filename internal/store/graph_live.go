package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Private controller enumeration never registers authority or starts work. The
// caller may use it only for a genuinely fresh current-owner release callback.
func (s *Store) LiveGraphRegistrations(ctx context.Context, ownerID string, limit int, cursor string) (domain.Page[domain.GraphDetail], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.GraphDetail]{Items: []domain.GraphDetail{}}
	if !domain.ValidID(ownerID) || limit < 1 || limit > 100 {
		return out, domain.Invalid("bounded exact current-owner live registrations")
	}
	filter := "live:" + ownerID
	highwater, after, e := pageBounds(ctx, s.db, "task_graphs", filter, cursor)
	if e != nil {
		return out, e
	}
	ids, e := graphPageIDs(ctx, s.db, "SELECT sequence,id FROM task_graphs WHERE dispatch_owner_instance_id=? AND json_extract(dispatch_data,'$.state') IN ('active','capacity_wait') AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", ownerID, after, highwater, limit+1)
	if e != nil {
		return out, e
	}
	bytes, last := 128, after
	for i, item := range ids {
		h, e := s.graphHeader(ctx, s.db, item.id)
		if e != nil {
			return out, e
		}
		if h.DispatchAuthority == nil || h.DispatchAuthority.OwnerInstanceID != ownerID || h.DispatchAuthority.State != "active" && h.DispatchAuthority.State != "capacity_wait" {
			return out, storageError()
		}
		detail, e := s.graphDetail(ctx, s.db, item.id)
		if e != nil {
			return out, e
		}
		size := len(encode(detail)) + 1
		if size+128 > 4<<20 {
			return out, domain.Fail("evidence_unavailable", "This complete live registration exceeds its bounded read envelope.")
		}
		if i == limit || bytes+size > 4<<20 {
			out.NextCursor = makeCursor("task_graphs", filter, highwater, last)
			break
		}
		out.Items = append(out.Items, detail)
		bytes += size
		last = item.sequence
	}
	return out, nil
}
