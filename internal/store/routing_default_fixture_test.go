package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"testing"
)

// Tests of accepted004 finite-two behavior explicitly retain that saved choice.
// They do not silently redefine the fresh005 factory default of four.
func routingSavedTwoPolicyFixture(t *testing.T, s *Store) {
	t.Helper()
	policy, err := s.RoutingPolicy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result := s.SetRoutingPolicy(context.Background(), domain.RoutingPolicyInput{RequestID: domain.NewID(), ExpectedRevision: policy.Revision, DefaultBudget: &domain.RoutingBudgetInput{AdmissionLimit: "2"}})
	if result.Status != 200 {
		t.Fatal("explicit saved two-admission fixture", result)
	}
}
