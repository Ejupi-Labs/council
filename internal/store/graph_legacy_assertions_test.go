package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Schema6 changes the physical claim shape. Its retained legacy ownership must
// still identify the exact original attempt/owner/context and block new work.
func assertGraphLegacyClaim(t *testing.T, s *Store, attemptID string) {
	t.Helper()
	ctx := context.Background()
	a, err := s.attempt(ctx, s.db, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := s.graphClaimRecords(ctx, s.db)
	if err != nil || len(claims) != 1 {
		t.Fatal("migration lost the sole held legacy claim", claims, err)
	}
	c := claims[0]
	var expectedContext *string
	if attemptExecutor(a) == "native" && domain.ValidContextID(a.ConnectionID) {
		expectedContext = ptr(a.ConnectionID)
	}
	var expectedAccount, expectedLink *string
	if a.ResolvedTarget != nil && a.ResolvedTarget.DeclaredAccountAssociation != nil {
		expectedAccount = ptr(a.ResolvedTarget.DeclaredAccountAssociation.AccountID)
		expectedLink = ptr(a.ResolvedTarget.DeclaredAccountAssociation.LinkRevision)
	}
	if !reflect.DeepEqual(c.View.CapturedDeclaredAccountID, expectedAccount) || !reflect.DeepEqual(c.View.CapturedLinkRevision, expectedLink) {
		t.Fatal("migration changed the captured declared-account reservation", c, a)
	}
	if c.View.AttemptID != attemptID || c.OwnerInstanceID != a.OwnerInstanceID || c.View.Slot != "1" || !c.View.LegacyBarrier || c.CapturedMaxParallelism != "1" || c.View.ExecutorKind != attemptExecutor(a) || !reflect.DeepEqual(c.View.ContextID, expectedContext) || c.View.GraphID != nil || c.View.NodeID != nil {
		t.Fatal("migration reinterpreted retained legacy ownership as new capacity", c, a)
	}
}

func assertNoMigratedGraphAuthority(t *testing.T, s *Store) {
	t.Helper()
	if err := s.validateRecoverySchemaWith(context.Background(), s.db); err != nil {
		t.Fatal("current exact schema inventory is invalid", err)
	}
	for _, table := range []string{"task_graphs", "graph_versions", "graph_nodes", "graph_inputs", "graph_dispatches", "graph_admission_observations", "graph_delivery_bindings"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("historical migration invented graph authority", table, count, err)
		}
	}
}
