package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// This is a real database/sql pool wait, not a mocked query or a pre-cancelled
// call. Registered proofs are metadata only; no worker or provider is launched.
func TestGraphArtifactReadCancellationDuringPoolWaitPreservesContextAndState(t *testing.T) {
	for _, kind := range []string{"cancel", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			s, graph, root, proofs, owner := graphReadyBasis(t)
			background := context.Background()
			facts, trigger := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[0])
			dispatch, err := s.AdmitGraphNode(background, owner, graph.Graph.ID, trigger, facts)
			if err != nil || dispatch.Dispatch.AttemptID == "" {
				t.Fatal("could not retain a real admission before the read", dispatch, err)
			}
			beforeGraph, err := s.Graph(background, graph.Graph.ID)
			if err != nil || beforeGraph.Budget.Budget.ConsumedAdmissions != "1" {
				t.Fatal("fixture has no admitted shared-budget unit", beforeGraph, err)
			}
			beforeClaims, err := s.GraphClaims(background)
			if err != nil || len(beforeClaims) != 1 || beforeClaims[0].AttemptID != dispatch.Dispatch.AttemptID {
				t.Fatal("fixture has no exact retained native claim", beforeClaims, err)
			}
			beforeLedger, err := s.GraphBudgetLedger(background, graph.Graph.ID, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			beforeGate, err := s.Gate(background)
			if err != nil || beforeGate.State != "open" {
				t.Fatal("fixture gate was not open", beforeGate, err)
			}
			beforeArtifact, err := s.Artifact(background, root.Manifest.ArtifactID)
			if err != nil || !beforeArtifact.Closed || beforeArtifact.Completeness != "complete" {
				t.Fatal("fixture root artifact was not readable", beforeArtifact, err)
			}

			if s.db.Stats().MaxOpenConnections != 1 {
				t.Fatal("test must reserve the Store's sole SQL connection")
			}
			held, err := s.db.Conn(background)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Close()
			var beforeChanges, beforeIncidents int64
			if err = held.QueryRowContext(background, "SELECT total_changes(),(SELECT count(*) FROM execution_incidents)").Scan(&beforeChanges, &beforeIncidents); err != nil {
				t.Fatal(err)
			}
			waitCount := s.db.Stats().WaitCount
			var ctx context.Context
			var cancel context.CancelFunc
			want := context.Canceled
			if kind == "deadline" {
				ctx, cancel = context.WithTimeout(background, 2*time.Second)
				want = context.DeadlineExceeded
			} else {
				ctx, cancel = context.WithCancel(background)
			}
			defer cancel()
			if ctx.Err() != nil {
				t.Fatal("read must begin with a live context")
			}
			type result struct {
				artifact domain.ArtifactRecord
				err      error
			}
			finished := make(chan result, 1)
			go func() {
				artifact, err := s.Artifact(ctx, root.Manifest.ArtifactID)
				finished <- result{artifact, err}
			}()

			// WaitCount changes only once database/sql has queued this call for
			// the unavailable connection. Keep it held through cancellation and
			// the result so releasing it cannot make the query succeed instead.
			observationDeadline := time.NewTimer(time.Second)
			ticker := time.NewTicker(time.Millisecond)
			defer observationDeadline.Stop()
			defer ticker.Stop()
		waitForPool:
			for {
				if s.db.Stats().WaitCount > waitCount {
					if ctx.Err() != nil {
						t.Fatal("context ended before a live pool wait was observed", ctx.Err())
					}
					break waitForPool
				}
				select {
				case got := <-finished:
					t.Fatal("read returned before the sole connection was released or the context ended", got.err)
				case <-observationDeadline.C:
					t.Fatal("read never reached the SQL connection wait")
				case <-ticker.C:
				}
			}
			if kind == "cancel" {
				cancel()
			}
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("read context did not end while its SQL connection remained held")
			}
			select {
			case got := <-finished:
				if got.err != want || !errors.Is(got.err, ctx.Err()) {
					t.Errorf("cancelled pure metadata read returned %T %v, want exact %v", got.err, got.err, want)
				}
				if !reflect.DeepEqual(got.artifact, domain.ArtifactRecord{}) {
					t.Error("interrupted read returned artifact evidence", got.artifact)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Artifact did not return cancellation while the SQL connection remained held")
			}

			var afterChanges, afterIncidents int64
			if err = held.QueryRowContext(background, "SELECT total_changes(),(SELECT count(*) FROM execution_incidents)").Scan(&afterChanges, &afterIncidents); err != nil {
				t.Fatal(err)
			}
			if afterChanges != beforeChanges || afterIncidents != beforeIncidents {
				t.Error("a cancelled metadata read wrote SQL state or retained an incident", beforeChanges, afterChanges, beforeIncidents, afterIncidents)
			}
			if err = held.Close(); err != nil {
				t.Fatal(err)
			}
			afterGraph, err := s.Graph(background, graph.Graph.ID)
			if err != nil || !reflect.DeepEqual(afterGraph, beforeGraph) {
				t.Error("cancelled read changed graph progress, revisions, or allowance", beforeGraph, afterGraph, err)
			}
			afterClaims, err := s.GraphClaims(background)
			if err != nil || !reflect.DeepEqual(afterClaims, beforeClaims) {
				t.Error("cancelled read changed the admitted claim", beforeClaims, afterClaims, err)
			}
			afterLedger, err := s.GraphBudgetLedger(background, graph.Graph.ID, 100, "")
			if err != nil || !reflect.DeepEqual(afterLedger, beforeLedger) {
				t.Error("cancelled read changed the shared-budget ledger", beforeLedger, afterLedger, err)
			}
			afterGate, err := s.Gate(background)
			if err != nil || !reflect.DeepEqual(afterGate, beforeGate) {
				t.Error("cancelled read fenced the existing owner", beforeGate, afterGate, err)
			}
			afterArtifact, err := s.Artifact(background, root.Manifest.ArtifactID)
			if err != nil || !reflect.DeepEqual(afterArtifact, beforeArtifact) {
				t.Error("cancelled read changed retained artifact evidence", beforeArtifact, afterArtifact, err)
			}
		})
	}
}

func TestGraphArtifactReadPreservesActualStorageAndCorruptionErrors(t *testing.T) {
	for _, kind := range []string{"corrupt_record", "closed_database"} {
		t.Run(kind, func(t *testing.T) {
			s, _, root, _, _ := graphReadyBasis(t)
			ctx := context.Background()
			if kind == "corrupt_record" {
				if _, err := s.db.ExecContext(ctx, "UPDATE artifacts SET data=? WHERE id=?", "{", root.Manifest.ArtifactID); err != nil {
					t.Fatal(err)
				}
			} else if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			_, err := s.Artifact(ctx, root.Manifest.ArtifactID)
			var failure *domain.Error
			if !errors.As(err, &failure) || failure.Code != "storage_failed" || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("actual storage/corruption error was weakened to context cancellation", err)
			}
		})
	}
}
