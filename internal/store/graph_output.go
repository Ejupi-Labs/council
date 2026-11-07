package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphOutputRole(a domain.Attempt) bool {
	return a.ExecutorKind == "native" && (a.Role == "author" || a.Role == "correction") || a.ExecutorKind == "local_check" && a.Role == "check"
}

// An association is installed only inside the producer's terminal transaction.
// Actual payload bytes were retained and verified while the producer was open.
func (s *Store) graphOutputForSettlement(ctx context.Context, tx *sql.Tx, a domain.Attempt, in domain.Settlement, f *domain.GraphStepOutputFacts, selectedCandidate *domain.CandidateBinding, selectedResponse *domain.ResponseBinding) error {
	member, e := s.graphMembership(ctx, tx, a.TaskID)
	if e != nil {
		return e
	}
	if member == nil {
		if f != nil {
			return domain.Invalid("graph output facts for a graph producer")
		}
		return nil
	}
	if member.Kind != "node" {
		if f != nil {
			return domain.Invalid("node output facts")
		}
		return nil
	}
	record, e := s.graphDispatchForAttempt(ctx, tx, a.ID)
	if e != nil {
		return e
	}
	if record.Public.OutputObservation != nil {
		return nil
	}
	if !graphOutputRole(a) {
		if f == nil {
			return nil
		}
		o := f.Observation
		if o.ProducerAttemptID != a.ID || f.CandidateClosure != nil || f.ResponseClosure != nil || f.FinalVerification != nil {
			return evidenceUnavailable()
		}
		if o.Candidate != nil && (a.CandidateID == nil || o.Candidate.CandidateID != *a.CandidateID || o.Candidate.State == "complete" || o.Candidate.ClosureRef != nil || o.Candidate.FinalVerificationRef != nil) {
			return evidenceUnavailable()
		}
		if o.Response != nil && (a.ResponseArtifactID == nil || o.Response.ResponseArtifactID != *a.ResponseArtifactID || o.Response.State == "complete" || o.Response.ClosureRef != nil) {
			return evidenceUnavailable()
		}
		if o.Candidate != nil && o.Candidate.State != "incomplete" && o.Candidate.State != "uncertain" || o.Response != nil && o.Response.State != "incomplete" && o.Response.State != "uncertain" {
			return domain.Invalid("nonqualifying supporting-role output")
		}
		if o.Candidate == nil && o.Response == nil {
			return nil
		}
		record.Public.OutputObservation = &o
		return saveGraphDispatch(ctx, tx, record)
	}
	expectedCandidate, expectedResponse := a.CandidateID, a.ResponseArtifactID
	if a.Role == "check" {
		expectedResponse = nil
	}
	if expectedCandidate == nil && expectedResponse == nil {
		if f != nil && (f.Observation.Candidate != nil || f.Observation.Response != nil) {
			return evidenceUnavailable()
		}
		return nil
	}
	effectsKnown := in.EffectsKnown
	qualifying := effectsKnown && in.Outcome == "succeeded" && in.Cleanup.State == "verified" && in.EvidenceStatus == "complete" && in.Failure == nil && a.PrimaryFailure == nil && a.Stop == nil
	observation := domain.GraphOutputObservation{ProducerAttemptID: a.ID}
	if f == nil {
		if qualifying && a.ExecutorKind == "native" {
			return evidenceUnavailable()
		}
		state := "incomplete"
		if in.EvidenceStatus == "uncertain" || in.Cleanup.State == "unknown" || !in.EffectsKnown {
			state = "uncertain"
		}
		if expectedCandidate != nil {
			observation.Candidate = &domain.GraphCandidateOutputObservation{CandidateID: *expectedCandidate, State: state}
		}
		if expectedResponse != nil {
			observation.Response = &domain.GraphResponseOutputObservation{ResponseArtifactID: *expectedResponse, State: state}
		}
	} else {
		observation = f.Observation
		if observation.ProducerAttemptID != a.ID || (observation.Candidate != nil && expectedCandidate == nil) || (observation.Response != nil && expectedResponse == nil) {
			return evidenceUnavailable()
		}
		if observation.Candidate == nil && (f.CandidateClosure != nil || f.FinalVerification != nil) || observation.Response == nil && f.ResponseClosure != nil {
			return evidenceUnavailable()
		}
		if observation.Candidate != nil {
			value := *observation.Candidate
			observation.Candidate = &value
			if value.CandidateID != *expectedCandidate {
				return evidenceUnavailable()
			}
			if e = s.graphOutputClosure(ctx, tx, a, record.Public.Dispatch, "candidate", value.State, value.ClosureRef, f.CandidateClosure, f.VerifiedArtifacts); e != nil {
				return e
			}
			if e = s.graphFinalOutput(ctx, tx, a, record.Public.Dispatch, value, f, selectedCandidate); e != nil {
				return e
			}
		}
		if observation.Response != nil {
			value := *observation.Response
			observation.Response = &value
			if value.ResponseArtifactID != *expectedResponse {
				return evidenceUnavailable()
			}
			if value.State == "complete" && selectedResponse != nil && selectedResponse.ProducerAttemptID == a.ID && selectedResponse.ResponseArtifactID == value.ResponseArtifactID && (f.ResponseClosure == nil || f.ResponseClosure.Index.ArtifactID != selectedResponse.IndexArtifactID || f.ResponseClosure.Index.SHA256 != selectedResponse.IndexSHA256) {
				return evidenceUnavailable()
			}
			if e = s.graphOutputClosure(ctx, tx, a, record.Public.Dispatch, "response", value.State, value.ClosureRef, f.ResponseClosure, f.VerifiedArtifacts); e != nil {
				return e
			}
		}
		if qualifying {
			candidateSelected := selectedCandidate != nil && expectedCandidate != nil && selectedCandidate.CandidateID == *expectedCandidate
			responseSelected := selectedResponse != nil && expectedResponse != nil && selectedResponse.ProducerAttemptID == a.ID && selectedResponse.ResponseArtifactID == *expectedResponse
			if candidateSelected && (observation.Candidate == nil || observation.Candidate.State != "complete") || responseSelected && (observation.Response == nil || observation.Response.State != "complete") {
				return evidenceUnavailable()
			}
		} else {
			// Complete immutable bytes may remain useful after a failed execution, but
			// an observed failed/uncertain terminal cannot qualify downstream output.
			state := "incomplete"
			if in.EvidenceStatus == "uncertain" || in.Cleanup.State == "unknown" || !in.EffectsKnown {
				state = "uncertain"
			}
			if observation.Candidate != nil {
				observation.Candidate.State = state
			}
			if observation.Response != nil {
				observation.Response.State = state
			}
		}
	}
	record.Public.OutputObservation = &observation
	if f != nil {
		record.CandidateClosure = f.CandidateClosure
		record.ResponseClosure = f.ResponseClosure
		record.FinalVerification = f.FinalVerification
	}
	return saveGraphDispatch(ctx, tx, record)
}

func (s *Store) graphOutputClosure(ctx context.Context, q queryer, a domain.Attempt, d domain.GraphDispatch, kind, state string, ref *domain.GraphArtifactRef, closure *domain.GraphOutputClosure, proofs []domain.VerifiedResponse) error {
	switch state {
	case "complete", "incomplete", "uncertain":
	default:
		return domain.Invalid("observed graph output state")
	}
	if ref == nil {
		if state == "complete" || closure != nil {
			return evidenceUnavailable()
		}
		return nil
	}
	if closure == nil || ref.ProducerAttemptID != a.ID || !graphRefHasBytes(*ref, []byte(encode(closure))) {
		return evidenceUnavailable()
	}
	if invalid := domain.ValidateGraphOutputClosure(*closure); invalid != nil {
		return invalid
	}
	seal, e := domain.GraphOutputDigest(*closure)
	if e != nil || seal != closure.ClosureSHA256 {
		return evidenceUnavailable()
	}
	if closure.GraphID != d.GraphID || closure.NodeID != d.NodeID || closure.ReceivedInputID != d.InputID || closure.Kind != kind || closure.ProducerAttemptID != a.ID || closure.Manifest.ProducerAttemptID != a.ID || closure.Index.ProducerAttemptID != a.ID {
		return evidenceUnavailable()
	}
	if e = s.verifyGraphRef(ctx, q, *ref, proofs, "graph_output_closure"); e != nil {
		return e
	}
	if kind == "candidate" {
		if a.CandidateID == nil || closure.CandidateID == nil || *closure.CandidateID != *a.CandidateID || closure.ResponseArtifact != nil {
			return evidenceUnavailable()
		}
		candidate, e := s.candidate(ctx, q, *a.CandidateID)
		if e != nil {
			return e
		}
		if closure.Manifest.ArtifactID != candidate.ManifestArtifactID || closure.Index.ArtifactID != candidate.IndexArtifactID {
			return evidenceUnavailable()
		}
		if e = s.verifyGraphRef(ctx, q, closure.Manifest, proofs, "candidate_manifest"); e != nil {
			return e
		}
	} else {
		if a.ResponseArtifactID == nil || closure.CandidateID != nil || closure.ResponseArtifact == nil || closure.ResponseArtifact.ArtifactID != *a.ResponseArtifactID || closure.ResponseArtifact.ProducerAttemptID != a.ID || a.SourceSnapshotID == nil {
			return evidenceUnavailable()
		}
		source, e := s.sourceSnapshot(ctx, q, *a.SourceSnapshotID)
		if e != nil {
			return e
		}
		if closure.Manifest.ArtifactID != source.ManifestArtifactID {
			return evidenceUnavailable()
		}
		if e = s.verifyGraphRef(ctx, q, *closure.ResponseArtifact, proofs, "final_response"); e != nil {
			return e
		}
		if e = s.verifyGraphRef(ctx, q, closure.Manifest, proofs, "source_manifest"); e != nil {
			return e
		}
	}
	if e = s.verifyGraphRef(ctx, q, closure.Index, proofs, "candidate_index", "index_snapshot"); e != nil {
		return e
	}
	return s.verifyGraphRef(ctx, q, closure.StagedBlobs, proofs, "graph_index_blobs")
}
func (s *Store) graphFinalizeOutput(ctx context.Context, tx *sql.Tx, a domain.Attempt, outcome string, in domain.Settlement) error {
	member, e := s.graphMembership(ctx, tx, a.TaskID)
	if e != nil || member == nil || member.Kind != "node" {
		return e
	}
	if outcome == "succeeded" && in.EvidenceStatus == "complete" && in.Cleanup.State == "verified" && in.EffectsKnown {
		return nil
	}
	record, e := s.graphDispatchForAttempt(ctx, tx, a.ID)
	if e != nil {
		return e
	}
	if record.Public.OutputObservation == nil {
		return nil
	}
	state := "incomplete"
	if in.EvidenceStatus == "uncertain" || in.Cleanup.State == "unknown" || !in.EffectsKnown {
		state = "uncertain"
	}
	if c := record.Public.OutputObservation.Candidate; c != nil && (c.State == "complete" || state == "uncertain") {
		c.State = state
	}
	if r := record.Public.OutputObservation.Response; r != nil && (r.State == "complete" || state == "uncertain") {
		r.State = state
	}
	return saveGraphDispatch(ctx, tx, record)
}
