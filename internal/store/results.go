package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) SettleAttempt(ctx context.Context, id string, in domain.Settlement) (domain.Attempt, error) {
	var out domain.Attempt
	e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		a, e := s.attempt(ctx, tx, id)
		if e != nil {
			return e
		}
		out, e = s.settle(ctx, tx, a, in)
		return e
	})
	return observe(out), e
}
func (s *Store) settle(ctx context.Context, tx *sql.Tx, a domain.Attempt, in domain.Settlement) (domain.Attempt, error) {
	out, err := s.settleCore(ctx, tx, a, in)
	if err == nil {
		err = s.syncWorkflowTerminal(ctx, tx, out)
	}
	return out, err
}
func (s *Store) settleCore(ctx context.Context, tx *sql.Tx, a domain.Attempt, in domain.Settlement) (domain.Attempt, error) {
	return s.settleCoreWithRecovery(ctx, tx, a, in, false)
}
func (s *Store) settleCoreWithRecovery(ctx context.Context, tx *sql.Tx, a domain.Attempt, in domain.Settlement, recovery bool) (domain.Attempt, error) {
	if a.Phase == "settled" {
		return a, nil
	}
	member, membershipErr := s.graphMembership(ctx, tx, a.TaskID)
	if membershipErr != nil {
		return a, membershipErr
	}
	graphNode := member != nil && member.Kind == "node"
	graphLocalRoot := member != nil && member.Kind == "root" && a.ExecutorKind == "local_graph"
	// Queued graph work has not entered preparation and cannot own local I/O.
	// Root-owned basis/input operations have the same known-zero boundary as nodes.
	if (graphNode || graphLocalRoot) && a.Phase == "queued" && !a.LaunchIntent && a.SourceSnapshotID == nil {
		in.EffectsKnown = true
	}
	graphNodeUnknown := graphNode && !in.EffectsKnown
	if err := s.graphOutputForSettlement(ctx, tx, a, in, nil, nil, nil); err != nil {
		return a, err
	}
	if !recovery && a.ExecutorKind == "local_application" {
		var active int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM delivery_operations WHERE attempt_id=? AND active_effect_id IS NOT NULL", a.ID).Scan(&active); err != nil {
			return a, storageError()
		}
		if active != 0 {
			return a, domain.Fail("effect_uncertain", "A permitted filesystem action remains unresolved; do not release its ownership.")
		}
	}
	switch in.Outcome {
	case "succeeded", "failed", "blocked", "cancelled", "interrupted":
	default:
		return a, domain.Invalid("terminal outcome")
	}
	switch in.Cleanup.State {
	case "verified", "unknown":
	default:
		return a, domain.Invalid("verified or unknown settlement cleanup")
	}
	switch in.Cleanup.Scope {
	case "not_launched", "windows_job", "unix_process_group":
	default:
		return a, domain.Invalid("cleanup scope")
	}
	if a.LaunchIntent && in.Cleanup.Scope == "not_launched" && in.Outcome != "blocked" && !in.NoProcessCreated {
		return a, domain.Invalid("launched process cleanup scope")
	}
	switch in.EvidenceStatus {
	case "complete", "incomplete", "uncertain":
	default:
		return a, domain.Invalid("settled evidence status")
	}
	if a.ExecutorKind == "local_application" && !in.EffectsKnown {
		if err := s.markWorkflowEffectsUnknown(ctx, tx, a); err != nil {
			return a, err
		}
	}
	now := domain.Now()
	if in.Cleanup.FinishedAt == nil {
		in.Cleanup.FinishedAt = ptr(now)
	}
	if in.Failure != nil {
		if e := validateFailure(in.Failure); e != nil {
			return a, e
		}
		seq, e := s.appendEvent(ctx, tx, a.ID, "primary_failure", failureMetadata(in.Failure))
		if e != nil {
			return a, e
		}
		if a.PrimaryFailure == nil && a.Stop == nil {
			failure := *in.Failure
			failure.EventSequence = seq
			failure.ObservedAt = now
			a.PrimaryFailure = &failure
		}
	}
	outcome := in.Outcome
	if a.PrimaryFailure != nil {
		outcome = "failed"
		if in.Outcome == "blocked" && !a.LaunchIntent {
			outcome = "blocked"
		}
	} else if a.Stop != nil {
		outcome = "cancelled"
	}
	if outcome == "succeeded" {
		usable := false
		if a.ResponseArtifactID != nil {
			artifact, e := s.artifact(ctx, tx, *a.ResponseArtifactID)
			usable = e == nil && artifact.AttemptID == a.ID && artifact.Closed && artifact.Completeness == "complete" && artifact.SHA256 != nil && artifact.ByteLength != "0"
		}
		if a.CandidateID != nil {
			c, e := s.candidate(ctx, tx, *a.CandidateID)
			usable = usable || (e == nil && (c.HasDelta || c.HasIndexDelta))
		}
		validSuccess := !graphNodeUnknown && a.LaunchIntent && in.RootTerminal && in.InputComplete && in.ExitCode != nil && *in.ExitCode == 0 && in.UsableOutput && usable
		if a.ExecutorKind == "local_check" {
			validSuccess = in.BuiltinCompleted && !a.LaunchIntent || a.LaunchIntent && in.RootTerminal && in.InputComplete && in.ExitCode != nil && in.UsableOutput
		} else if a.ExecutorKind == "local_application" {
			validSuccess = in.EffectsKnown && !a.LaunchIntent
		} else if a.ExecutorKind == "local_external" {
			validSuccess = a.LaunchIntent && in.RootTerminal && in.InputComplete && in.ExitCode != nil && *in.ExitCode == 0
		}
		if a.ExecutorKind == "local_graph" {
			var ready int
			if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM graph_inputs WHERE operation_attempt_id=? AND state='ready'", a.ID).Scan(&ready); e != nil {
				return a, storageError()
			}
			validSuccess = ready == 1 && !a.LaunchIntent && in.EffectsKnown && in.InputComplete && in.UsableOutput
		}
		if !validSuccess || graphNodeUnknown || in.EvidenceStatus != "complete" || in.Cleanup.State != "verified" {
			outcome = "failed"
			seq, e := s.appendEvent(ctx, tx, a.ID, "primary_failure", failureMetadata(in.Failure))
			if e != nil {
				return a, e
			}
			a.PrimaryFailure = &domain.PrimaryFailure{Code: "result_incomplete", Message: "Authoring did not produce a complete durable usable result with verified root exit and cleanup.", ObservedAt: now, EventSequence: seq}
		}
	}
	if err := s.graphFinalizeOutput(ctx, tx, a, outcome, in); err != nil {
		return a, err
	}
	a.Phase = "settled"
	a.Outcome = ptr(outcome)
	a.SettledAt = ptr(now)
	a.Cleanup = in.Cleanup
	a.EvidenceStatus = in.EvidenceStatus
	materializationUncertain, proofErr := s.graphMaterializationUncertain(ctx, tx, a)
	if proofErr != nil {
		return a, proofErr
	}
	if a.Cleanup.State == "unknown" || in.EvidenceStatus == "uncertain" || a.ExecutorKind == "local_graph" && !in.EffectsKnown || materializationUncertain || graphNodeUnknown {
		incident := domain.ExecutionIncident{ID: domain.NewID(), AttemptID: ptr(a.ID), Code: "settlement_unresolved", Message: "Process cleanup, local effects or evidence durability remains unresolved. Dispatch is fenced.", CreatedAt: now}
		if e := s.fence(ctx, tx, incident); e != nil {
			return a, e
		}
		a.Cleanup.IncidentID = ptr(incident.ID)
	}
	retainEffectClaim, e := s.workflowEffectUncertain(ctx, tx, a)
	if e != nil {
		return a, e
	}
	if (a.ExecutorKind == "local_application" || a.ExecutorKind == "local_graph") && !in.EffectsKnown || materializationUncertain || graphNodeUnknown {
		retainEffectClaim = true
	}
	// A later verified cleanup does not itself reconcile a watchdog incident.
	// Keep this attempt's exact claim until the proof-backed decision commits.
	var unresolved int
	if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM execution_incidents i
		WHERE i.attempt_id=? AND NOT EXISTS (
			SELECT 1 FROM recovery_decisions d
			WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled'
		)`, a.ID).Scan(&unresolved); e != nil {
		return a, storageError()
	}
	retainEffectClaim = retainEffectClaim || unresolved != 0
	if a.Cleanup.State == "verified" && !retainEffectClaim {
		if _, e := tx.ExecContext(ctx, "DELETE FROM capacity_claims WHERE attempt_id=?", a.ID); e != nil {
			return a, storageError()
		}
	}
	if _, e := s.appendEvent(ctx, tx, a.ID, "settled", nil); e != nil {
		return a, e
	}
	if e := s.bumpState(ctx, tx, &a); e != nil {
		return a, e
	}
	if _, e := tx.ExecContext(ctx, "INSERT INTO attempt_terminals VALUES(?,?)", a.ID, encode(struct {
		Attempt    domain.Attempt    `json:"attempt"`
		Settlement domain.Settlement `json:"settlement"`
	}{a, in})); e != nil {
		return a, storageError()
	}
	if e := s.refreshRecoveryCheckpointsForAttempt(ctx, tx, a.ID); e != nil {
		return a, e
	}
	return a, nil
}
func (s *Store) AcceptResponse(ctx context.Context, taskID string, in domain.ResponseAcceptanceInput, proof domain.VerifiedResponse) domain.Result {
	if e := domain.ValidateResponseAcceptance(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	fp := Fingerprint("tasks.response.accept", struct {
		TaskID string
		Input  domain.ResponseAcceptanceInput
	}{taskID, in})
	return s.mutate(ctx, in.RequestID, "tasks.response.accept", fp, func(tx *sql.Tx) (int, any, error) {
		t, e := s.task(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		if t.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(t.Revision)
		}
		if t.CurrentVersion != in.TaskVersion || t.LatestAttemptID == nil || *t.LatestAttemptID != in.AttemptID {
			return 0, nil, domain.Fail("task_version_conflict", "Accept only the latest attempt for the current task version.")
		}
		var existing int
		e = tx.QueryRowContext(ctx, "SELECT 1 FROM response_acceptances WHERE attempt_id=?", in.AttemptID).Scan(&existing)
		if e == nil {
			return 0, nil, domain.Fail("acceptance_conflict", "This response already has a user acceptance record.")
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return 0, nil, storageError()
		}
		v, e := s.version(ctx, tx, taskID, in.TaskVersion)
		if e != nil {
			return 0, nil, e
		}
		a, e := s.attempt(ctx, tx, in.AttemptID)
		if e != nil {
			return 0, nil, e
		}
		eligible := a.TaskID == taskID && a.TaskVersion == in.TaskVersion && a.Phase == "settled" && a.Outcome != nil && *a.Outcome == "succeeded" && a.Cleanup.State == "verified" && a.EvidenceStatus == "complete" && a.ResponseArtifactID != nil && *a.ResponseArtifactID == in.ResponseArtifactID && v.Acceptance.Deliverable == "response_only" && len(v.Acceptance.RequiredEvidence) == 0
		if a.CandidateID != nil {
			candidate, e := s.candidate(ctx, tx, *a.CandidateID)
			if e != nil {
				return 0, nil, e
			}
			eligible = eligible && !candidate.HasDelta && !candidate.HasIndexDelta
		}
		artifact, e := s.artifact(ctx, tx, in.ResponseArtifactID)
		if e != nil {
			return 0, nil, e
		}
		eligible = eligible && artifact.AttemptID == a.ID && artifact.Kind == "final_response" && artifact.Closed && artifact.Completeness == "complete" && artifact.SHA256 != nil && *artifact.SHA256 == in.ResponseSHA256 && proof.ArtifactID == artifact.ID && proof.SHA256 == in.ResponseSHA256 && proof.PhysicalIdentity == artifact.PhysicalIdentity && proof.ByteLength == artifact.ByteLength
		if !eligible {
			return 0, nil, domain.Fail("acceptance_unavailable", "This response lacks a current verified response-only result without authored changes or outstanding requirements.")
		}
		record := domain.ResponseAcceptance{ID: domain.NewID(), RequestID: in.RequestID, TaskID: taskID, TaskVersion: in.TaskVersion, AttemptID: in.AttemptID, ResponseArtifactID: in.ResponseArtifactID, ResponseSHA256: in.ResponseSHA256, Basis: "user_review", Confirmation: in.Confirmation, AcceptedAt: domain.Now()}
		if _, e = tx.ExecContext(ctx, "INSERT INTO response_acceptances VALUES(?,?,?,?,?,?)", record.ID, taskID, intRevision(in.TaskVersion), a.ID, artifact.ID, encode(record)); e != nil {
			return 0, nil, storageError()
		}
		t.Revision, e = increment(t.Revision)
		if e != nil {
			return 0, nil, e
		}
		t.UpdatedAt = record.AcceptedAt
		if e = saveTask(ctx, tx, t); e != nil {
			return 0, nil, e
		}
		return 200, struct {
			Task       domain.TaskSummary        `json:"task"`
			Acceptance domain.ResponseAcceptance `json:"acceptance"`
		}{taskSummary(t, v, &a, true), record}, nil
	})
}
