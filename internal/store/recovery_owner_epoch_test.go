package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestRecoveryOwnerEpochFreshCleanReopen(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	epoch, err := readRecoveryOwnerEpoch(ctx, s.db)
	if err != nil || epoch.State != "clean" || epoch.Generation != 0 {
		t.Fatal(epoch, err)
	}
	first := domain.NewID()
	if err = s.BeginOwnerEpoch(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverOwner(ctx, first); err != nil {
		t.Fatal(err)
	}
	if r := s.Shutdown(ctx, first, domain.NewID()); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	closed, err := s.CompleteOwnerEpoch(ctx, first)
	if err != nil || !closed {
		t.Fatal("clean closure was not durable", closed, err)
	}
	next := domain.NewID()
	if err = s.BeginOwnerEpoch(ctx, next); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverOwner(ctx, next); err != nil {
		t.Fatal(err)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "open" || gate.OwnerInstanceID != next {
		t.Fatal("verified clean restart was fenced", gate, err)
	}
	epoch, err = readRecoveryOwnerEpoch(ctx, s.db)
	if err != nil || epoch.State != "dirty" || epoch.OwnerID != next || epoch.Generation != 2 {
		t.Fatal(epoch, err)
	}
}

func TestRecoveryOwnerEpochDirtyRestartAndLegacyUpgradeFence(t *testing.T) {
	t.Run("dirty", func(t *testing.T) {
		ctx := context.Background()
		s := testStore(t)
		first := domain.NewID()
		if err := s.BeginOwnerEpoch(ctx, first); err != nil {
			t.Fatal(err)
		}
		if err := s.RecoverOwner(ctx, first); err != nil {
			t.Fatal(err)
		}
		next := domain.NewID()
		if err := s.BeginOwnerEpoch(ctx, next); err != nil {
			t.Fatal(err)
		}
		if err := s.RecoverOwner(ctx, next); err != nil {
			t.Fatal(err)
		}
		gate, err := s.Gate(ctx)
		if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
			t.Fatal("unclean prior epoch was not fenced", gate, err)
		}
		incident, err := s.Incident(ctx, *gate.IncidentID)
		if err != nil || incident.Code != "owner_epoch_uncertain" {
			t.Fatal(incident, err)
		}
	})
	t.Run("legacy_v6", func(t *testing.T) {
		ctx := context.Background()
		path := filepath.Join(t.TempDir(), "accepted6.sqlite3")
		db, _ := recoveryV6Fixture(t, path)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		epoch, err := readRecoveryOwnerEpoch(ctx, s.db)
		if err != nil || epoch.State != "untracked" {
			t.Fatal("migration invented clean v6 proof", epoch, err)
		}
		owner := domain.NewID()
		if err = s.BeginOwnerEpoch(ctx, owner); err != nil {
			t.Fatal(err)
		}
		if err = s.RecoverOwner(ctx, owner); err != nil {
			t.Fatal(err)
		}
		gate, err := s.Gate(ctx)
		if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
			t.Fatal(gate, err)
		}
		incident, err := s.Incident(ctx, *gate.IncidentID)
		if err != nil || incident.Code != "owner_epoch_uncertain" {
			t.Fatal(incident, err)
		}
	})
}

func TestRecoveryOwnerEpochFailedCommitAndLostAcknowledgement(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit"}[after], func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			first := domain.NewID()
			fault := func() error { return errors.New("injected epoch commit failure") }
			if after {
				s.AfterCommit = fault
			} else {
				s.BeforeCommit = fault
			}
			err := s.BeginOwnerEpoch(ctx, first)
			s.BeforeCommit = nil
			s.AfterCommit = nil
			if err == nil {
				t.Fatal("epoch persistence fault disappeared")
			}
			epoch, err := readRecoveryOwnerEpoch(ctx, s.db)
			if err != nil {
				t.Fatal(err)
			}
			if after {
				if epoch.State != "dirty" || epoch.OwnerID != first {
					t.Fatal("lost acknowledgement erased committed dirty marker", epoch)
				}
				next := domain.NewID()
				if err = s.BeginOwnerEpoch(ctx, next); err != nil {
					t.Fatal(err)
				}
				if err = s.RecoverOwner(ctx, next); err != nil {
					t.Fatal(err)
				}
				gate, e := s.Gate(ctx)
				if e != nil || gate.State != "fenced" {
					t.Fatal("uncertain committed epoch reopened gate", gate, e)
				}
			} else if epoch.State != "clean" || epoch.Generation != 0 {
				t.Fatal("rolled-back marker became visible", epoch)
			}
		})
	}
}

func TestRecoveryOwnerEpochCleanlyRetainsKnownFencedIncident(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner := domain.NewID()
	if err := s.BeginOwnerEpoch(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	incident := domain.ExecutionIncident{ID: domain.NewID(), Code: "known_unresolved", Message: "Retained independently.", CreatedAt: domain.Now()}
	if err := s.Fence(ctx, incident); err != nil {
		t.Fatal(err)
	}
	if r := s.Shutdown(ctx, owner, domain.NewID()); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	clean, err := s.CompleteOwnerEpoch(ctx, owner)
	if err != nil || !clean {
		t.Fatal("durably recorded blocker was mistaken for an unrecorded cause", clean, err)
	}
	next := domain.NewID()
	if err = s.BeginOwnerEpoch(ctx, next); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverOwner(ctx, next); err != nil {
		t.Fatal(err)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || *gate.IncidentID != incident.ID {
		t.Fatal("known incident was erased or replaced by a fabricated unknown cause", gate, err)
	}
}

// Metadata fixture: the pre-existing resolved incident is synthetic; these
// checks exercise only durable owner-generation provenance and gate recheck.
func TestRecoveryOwnerEpochCleanFencedMultiHopExplicitRecheck(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, settlement := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	if err := s.BeginOwnerEpoch(ctx, owner); err != nil {
		t.Fatal(err)
	}
	incident := domain.ExecutionIncident{ID: domain.NewID(), AttemptID: &admission.Attempt.ID, Code: "settlement_budget_elapsed", Message: "Previously verified fixture.", CreatedAt: domain.Now()}
	if err := s.Fence(ctx, incident); err != nil {
		t.Fatal(err)
	}
	decision := domain.ReconciliationDecision{ID: domain.NewID(), IncidentID: incident.ID, RequestID: domain.NewID(), ObservationID: domain.NewID(), Outcome: "reconciled", CreatedAt: domain.Now()}
	if _, err := s.db.Exec("INSERT INTO recovery_decisions(id,incident_id,request_id,data) VALUES(?,?,?,?)", decision.ID, incident.ID, decision.RequestID, encode(decision)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SettleWorkflowStep(ctx, admission.Attempt.ID, settlement); err != nil {
		t.Fatal(err)
	}
	current := owner
	for i := 0; i < 2; i++ {
		if r := s.Shutdown(ctx, current, domain.NewID()); r.Envelope.Error != nil {
			t.Fatal(r)
		}
		clean, err := s.CompleteOwnerEpoch(ctx, current)
		if err != nil || !clean {
			t.Fatal("known quiescent fence could not close cleanly", clean, err)
		}
		next := domain.NewID()
		if err = s.BeginOwnerEpoch(ctx, next); err != nil {
			t.Fatal(err)
		}
		if err = s.RecoverOwner(ctx, next); err != nil {
			t.Fatal(err)
		}
		gate, err := s.Gate(ctx)
		if err != nil || gate.State != "fenced" || gate.IncidentID == nil || *gate.IncidentID != incident.ID {
			t.Fatal("clean handoff reopened or replaced fence", gate, err)
		}
		current = next
	}
	in := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "2", Action: "verify_and_reconcile"}
	result := s.ReconcileIncident(ctx, current, incident.ID, in, nil)
	if result.Envelope.Error != nil || result.Envelope.Receipt == nil {
		t.Fatal("clean chain could not explicitly recheck", result)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatal("explicit clean-chain recheck did not open gate", gate, err)
	}
}

func TestRecoveryOwnerEpochDirtyGenerationBreaksCleanChain(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	first := domain.NewID()
	if err := s.BeginOwnerEpoch(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverOwner(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := domain.NewID()
	if err := s.BeginOwnerEpoch(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverOwner(ctx, second); err != nil {
		t.Fatal(err)
	}
	clean, err := recoveryCleanOwnerChain(ctx, s.db, first, second)
	if err != nil || clean {
		t.Fatal("unclosed owner epoch was accepted as a clean chain", clean, err)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
		t.Fatal("dirty handoff lost uncertainty", gate, err)
	}
}
