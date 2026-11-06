package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) bindWorkflowCandidate(ctx context.Context, tx *sql.Tx, a domain.Attempt, in *domain.Candidate) error {
	w, e := s.workflowByID(ctx, tx, a.WorkflowID)
	if e != nil {
		return e
	}
	if w.TaskID != a.TaskID || w.TaskVersion != a.TaskVersion || in.WorkflowID != "" && in.WorkflowID != a.WorkflowID || in.CreatedByStepID != "" && in.CreatedByStepID != a.StepID || in.ConditionsSHA256 != "" && in.ConditionsSHA256 != w.Authority.ConditionsSHA256 {
		return domain.Invalid("candidate workflow/version/step binding")
	}
	step, e := s.workflowStep(ctx, tx, a.StepID)
	if e != nil {
		return e
	}
	if step.AttemptID == nil || *step.AttemptID != a.ID || step.WorkflowID != a.WorkflowID {
		return domain.Invalid("candidate owner step")
	}
	source, e := s.sourceSnapshot(ctx, tx, in.SourceSnapshotID)
	if e != nil {
		return e
	}
	if source.AttemptID != a.ID || in.WorkspaceRef != "workspaces/"+source.ID || !domain.ValidID(in.IndexArtifactID) || in.IndexArtifactID == source.IndexArtifactID {
		return domain.Invalid("separately retained candidate current index")
	}
	index, e := s.artifact(ctx, tx, in.IndexArtifactID)
	if e != nil {
		return e
	}
	if index.AttemptID != a.ID || (index.Kind != "index_snapshot" && index.Kind != "candidate_index") || !index.Closed || index.Completeness != "complete" || index.SHA256 == nil {
		return domain.Invalid("complete same-attempt current candidate index")
	}
	for _, ref := range []struct{ id, kind string }{{in.ManifestArtifactID, "candidate_manifest"}, {in.DeltaArtifactID, "candidate_delta"}} {
		artifact, e := s.artifact(ctx, tx, ref.id)
		if e != nil {
			return e
		}
		if artifact.AttemptID != a.ID || artifact.Kind != ref.kind || !artifact.Closed || artifact.Completeness != "complete" || artifact.SHA256 == nil {
			return domain.Invalid("complete same-attempt candidate artifacts")
		}
	}
	root := source.ID
	inputCandidateID := a.InputCandidateID
	if inputCandidateID == nil && step.InputResponseBinding != nil {
		producer, err := s.attempt(ctx, tx, step.InputResponseBinding.ProducerAttemptID)
		if err != nil {
			return err
		}
		if producer.WorkflowID != w.ID || producer.CandidateID == nil {
			return domain.Invalid("response producer current source lineage")
		}
		inputCandidateID = producer.CandidateID
	}
	if inputCandidateID != nil {
		if in.PredecessorCandidateID != nil && *in.PredecessorCandidateID != *inputCandidateID {
			return domain.Invalid("candidate predecessor binding")
		}
		previous, e := s.candidate(ctx, tx, *inputCandidateID)
		if e != nil {
			return e
		}
		if previous.WorkflowID != a.WorkflowID {
			return domain.Invalid("same-workflow predecessor")
		}
		producer, e := s.attempt(ctx, tx, previous.AttemptID)
		if e != nil {
			return e
		}
		if producer.TaskID != a.TaskID || producer.TaskVersion != a.TaskVersion {
			return domain.Invalid("same task/version predecessor")
		}
		root = previous.RootSourceSnapshotID
		in.PredecessorCandidateID = ptr(previous.ID)
	} else if in.PredecessorCandidateID != nil {
		return domain.Invalid("initial candidate without predecessor")
	}
	if in.RootSourceSnapshotID != "" && in.RootSourceSnapshotID != root {
		return domain.Invalid("candidate original task source basis")
	}
	if _, e = s.sourceSnapshot(ctx, tx, root); e != nil {
		return e
	}
	in.RootSourceSnapshotID = root
	in.WorkflowID = a.WorkflowID
	in.CreatedByStepID = a.StepID
	in.ConditionsSHA256 = w.Authority.ConditionsSHA256
	if in.Completeness == "" {
		in.Completeness = "complete"
	}
	if in.Completeness != "complete" && in.Completeness != "partial" && in.Completeness != "uncertain" {
		return domain.Invalid("candidate completeness")
	}
	return nil
}
