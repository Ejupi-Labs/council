package store

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestRecoveryScopedBlockPreservesSuspendedSharedAuthority(t *testing.T) {
	for _, code := range []string{"capacity_wait", "routing_unavailable"} {
		t.Run(code, func(t *testing.T) {
			s, graph, _, _, _ := graphReadyBasis(t)
			ctx := context.Background()
			err := s.executionWrite(ctx, func(tx *sql.Tx) error {
				h, err := s.graphHeader(ctx, tx, graph.Graph.ID)
				if err != nil {
					return err
				}
				nodes, err := s.graphNodes(ctx, tx, graph.Graph.ID)
				if err != nil {
					return err
				}
				h.DispatchAuthority.State = "suspended"
				before := *h.DispatchAuthority
				if err := s.graphProjectAdmissionBlock(ctx, tx, h, nodes, 0, domain.NewID(), domain.Fail(code, "controlled scoped blocker"), true); err != nil {
					return err
				}
				after, err := s.graphHeader(ctx, tx, graph.Graph.ID)
				if err != nil {
					return err
				}
				if !reflect.DeepEqual(before, *after.DispatchAuthority) {
					t.Fatal("scoped blocker changed suspended shared authority", before, *after.DispatchAuthority)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
