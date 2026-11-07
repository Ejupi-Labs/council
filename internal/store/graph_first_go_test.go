package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphKnownFirstGOIsAnchoredToNodeAdmission(t *testing.T) {
	s, d, f, owner := graphMaterializationFixture(t)
	ctx := context.Background()
	if e := s.RecordGraphMaterialization(ctx, owner, d.Dispatch.AttemptID, f); e != nil {
		t.Fatal(e)
	}
	before, e := s.GraphDispatch(ctx, d.Dispatch.GraphID, d.Dispatch.NodeAdmissionID)
	if e != nil || before.Materialization == nil {
		t.Fatal(before, e)
	}
	if _, e = s.RecordLaunchIntent(ctx, d.Dispatch.AttemptID); e != nil {
		t.Fatal(e)
	}
	anchor, e := s.GraphDispatchForAttempt(ctx, d.Dispatch.AttemptID)
	if e != nil || anchor.KnownFirstGOAt != nil || anchor.KnownFirstGOAttemptID != nil {
		t.Fatal("preparation invented first native GO", anchor, e)
	}
	// A controlled typed controller observation proves DAO ordering, not an OS
	// launch. Engine tests separately observe actual owned native Start.
	at := domain.Now()
	if _, e = s.RecordAttempt(ctx, d.Dispatch.AttemptID, domain.AttemptUpdate{Kind: "worker_started", Phase: "running", StartedAt: &at}); e != nil {
		t.Fatal(e)
	}
	anchor, e = s.GraphDispatchForAttempt(ctx, d.Dispatch.AttemptID)
	if e != nil || anchor.KnownFirstGOAt == nil || *anchor.KnownFirstGOAt != at || anchor.KnownFirstGOAttemptID == nil || *anchor.KnownFirstGOAttemptID != d.Dispatch.AttemptID {
		t.Fatal("actual first-GO observation was not paired on immutable node-admission anchor", anchor, e)
	}
}
