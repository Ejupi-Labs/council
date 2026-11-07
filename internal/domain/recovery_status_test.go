package domain

import "testing"

func TestRecoveryErrorStatus(t *testing.T) {
	for _, code := range []string{"scope_paused", "checkpoint_not_paused", "checkpoint_scope_conflict", "checkpoint_blocked", "scope_stopped", "continuation_not_prepared", "cleanup_unknown"} {
		if got := Status(code); got != 409 {
			t.Errorf("Status(%q) = %d, want 409", code, got)
		}
	}
	if got := Status("checkpoint_too_large"); got != 422 {
		t.Errorf("Status(checkpoint_too_large) = %d, want 422", got)
	}
}
