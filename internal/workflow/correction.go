package workflow

import (
	"slices"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func CanCorrect(policy domain.CorrectionPolicy, used int, trigger string) bool {
	return policy.MaxCycles == 1 && used == 0 && (trigger == "blocking_review_finding" || trigger == "failed_required_check") && slices.Contains(policy.Triggers, trigger)
}
