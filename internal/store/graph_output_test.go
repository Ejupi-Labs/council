package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"reflect"
	"testing"
)

func graphOutputSettlementFixture(t *testing.T, responseOnly ...bool) (*Store, domain.GraphDispatchView, domain.WorkflowStepFacts, string) {
	t.Helper()
	response := len(responseOnly) > 0 && responseOnly[0]
	return graphOutputConfiguredFixture(t, response)
}

func graphOutputConfiguredFixture(t *testing.T, response bool, options ...func(*domain.GraphStartInput)) (*Store, domain.GraphDispatchView, domain.WorkflowStepFacts, string) {
	t.Helper()
	configure := func(in *domain.GraphStartInput) {
		if response {
			in.Nodes[0].Acceptance = &domain.AcceptanceInput{Deliverable: "response_only"}
		}
		for _, option := range options {
			option(in)
		}
	}
	s, d, materialization, owner := graphMaterializationFixture(t, configure)
	return s, d, graphOutputForDispatch(t, s, d, materialization, owner, response), owner
}

func graphOutputForDispatch(t *testing.T, s *Store, d domain.GraphDispatchView, materialization domain.GraphMaterializationFacts, owner string, response bool) domain.WorkflowStepFacts {
	t.Helper()
	ctx := context.Background()
	if e := s.RecordGraphMaterialization(ctx, owner, d.Dispatch.AttemptID, materialization); e != nil {
		t.Fatal(e)
	}
	if _, e := s.RecordLaunchIntent(ctx, d.Dispatch.AttemptID); e != nil {
		t.Fatal(e)
	}
	a, e := s.Attempt(ctx, d.Dispatch.AttemptID)
	if e != nil {
		t.Fatal(e)
	}
	manifest, mp := graphPayload(t, s, a.ID, "candidate_manifest", []byte("controlled candidate metadata"))
	delta, dp := graphPayload(t, s, a.ID, "candidate_delta", []byte("controlled delta metadata"))
	index, ip := graphPayload(t, s, a.ID, "candidate_index", []byte("controlled candidate current index"))
	blobs, bp := graphPayload(t, s, a.ID, "graph_index_blobs", []byte("controlled selected staged closure"))
	c := domain.Candidate{ID: domain.NewID(), AttemptID: a.ID, WorkflowID: a.WorkflowID, SourceSnapshotID: materialization.SourceSnapshot.ID, ManifestArtifactID: manifest.ArtifactID, DeltaArtifactID: delta.ArtifactID, IndexArtifactID: index.ArtifactID, WorkspaceRef: "workspaces/" + materialization.SourceSnapshot.ID, HasDelta: !response, Completeness: "complete", CreatedAt: domain.Now()}
	if e = s.PutCandidate(ctx, c); e != nil {
		t.Fatal(e)
	}
	c, e = s.Candidate(ctx, c.ID)
	if e != nil {
		t.Fatal(e)
	}
	w, e := s.Workflow(ctx, a.TaskID, domain.WorkflowSelector{})
	if e != nil || w.Workflow == nil {
		t.Fatal(w, e)
	}
	binding := domain.CandidateBinding{CandidateID: c.ID, RootSourceSnapshotID: c.RootSourceSnapshotID, SourceSnapshotID: c.SourceSnapshotID, ManifestArtifactID: manifest.ArtifactID, ManifestSHA256: manifest.SHA256, IndexArtifactID: index.ArtifactID, IndexSHA256: index.SHA256, ConditionsSHA256: w.Workflow.Authority.ConditionsSHA256}
	closure := domain.GraphOutputClosure{GraphID: d.Dispatch.GraphID, NodeID: d.Dispatch.NodeID, ReceivedInputID: d.Dispatch.InputID, Kind: "candidate", ProducerAttemptID: a.ID, CandidateID: &c.ID, Manifest: manifest, Index: index, StagedBlobs: blobs}
	closure.ClosureSHA256, e = domain.GraphOutputDigest(closure)
	if e != nil {
		t.Fatal(e)
	}
	closureRef, cp := graphPayload(t, s, a.ID, "graph_output_closure", []byte(encode(closure)))
	graphFacts := &domain.GraphStepOutputFacts{Observation: domain.GraphOutputObservation{ProducerAttemptID: a.ID, Candidate: &domain.GraphCandidateOutputObservation{CandidateID: c.ID, State: "complete", ClosureRef: &closureRef}}, CandidateClosure: &closure, VerifiedArtifacts: []domain.VerifiedResponse{mp, ip, bp, cp}}
	task, e := s.Task(ctx, a.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	exit := 0
	facts := domain.WorkflowStepFacts{GraphOutput: graphFacts, StepID: a.StepID, OwnerInstanceID: owner, TaskRevision: task.Task.Revision, WorkflowRevision: w.Workflow.Revision, CandidateBinding: &binding, VerifiedArtifacts: append(materialization.VerifiedArtifacts, mp, dp, ip, bp, cp), HasTaskDelta: !response, Settlement: domain.Settlement{Outcome: "succeeded", EffectsKnown: true, RootTerminal: true, InputComplete: true, ExitCode: &exit, UsableOutput: true, Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "complete"}}
	if response {
		raw, rp := graphPayload(t, s, a.ID, "final_response", []byte("Exact response-only metadata fixture"))
		if _, e = s.RecordAttempt(ctx, a.ID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &raw.ArtifactID}); e != nil {
			t.Fatal(e)
		}
		rb := domain.ResponseBinding{ProducerAttemptID: a.ID, ResponseArtifactID: raw.ArtifactID, ResponseSHA256: raw.SHA256, ResponseByteLength: raw.ByteLength, SourceSnapshotID: materialization.SourceSnapshot.ID, ManifestSHA256: materialization.SourceSnapshot.ManifestSHA256, IndexArtifactID: index.ArtifactID, IndexSHA256: index.SHA256, ConditionsSHA256: w.Workflow.Authority.ConditionsSHA256}
		rc := domain.GraphOutputClosure{GraphID: d.Dispatch.GraphID, NodeID: d.Dispatch.NodeID, ReceivedInputID: d.Dispatch.InputID, Kind: "response", ProducerAttemptID: a.ID, ResponseArtifact: &raw, Manifest: *materialization.Receipt.ObservedManifest, Index: index, StagedBlobs: blobs}
		rc.ClosureSHA256, e = domain.GraphOutputDigest(rc)
		if e != nil {
			t.Fatal(e)
		}
		ref, proof := graphPayload(t, s, a.ID, "graph_output_closure", []byte(encode(rc)))
		graphFacts.ResponseClosure = &rc
		graphFacts.Observation.Response = &domain.GraphResponseOutputObservation{ResponseArtifactID: raw.ArtifactID, State: "complete", ClosureRef: &ref}
		graphFacts.VerifiedArtifacts = append(graphFacts.VerifiedArtifacts, materialization.VerifiedArtifacts...)
		graphFacts.VerifiedArtifacts = append(graphFacts.VerifiedArtifacts, rp, proof)
		facts.VerifiedArtifacts = append(facts.VerifiedArtifacts, rp, proof)
		facts.CandidateBinding = nil
		facts.ResponseBinding = &rb
	}
	return facts
}
func TestGraphOutputAssociationSharesProducerTerminalAndEpoch(t *testing.T) {
	s, d, f, _ := graphOutputSettlementFixture(t)
	ctx := context.Background()
	if _, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, f); e != nil {
		t.Fatal(e)
	}
	stored, e := s.GraphDispatchForAttempt(ctx, d.Dispatch.AttemptID)
	if e != nil || !reflect.DeepEqual(stored.Public.OutputObservation, &f.GraphOutput.Observation) {
		t.Fatal("terminal did not attach exact closed output", stored, e)
	}
	a, e := s.Attempt(ctx, d.Dispatch.AttemptID)
	if e != nil || a.Phase != "settled" || a.Outcome == nil || *a.Outcome != "succeeded" {
		t.Fatal(a, e)
	}
	step, e := s.WorkflowStep(ctx, d.Dispatch.StepID)
	if e != nil || step.AdvanceProof == nil || step.AdvanceProof.GraphBudgetAuthorityEpoch != "1" {
		t.Fatal("fresh graph proof omitted explicit authority epoch", step, e)
	}
}
func TestGraphOutputMissingOrWrongClosureRollsBackProducerTerminal(t *testing.T) {
	for _, kind := range []string{"missing_observation", "missing_fresh_closure", "wrong_closed_candidate"} {
		t.Run(kind, func(t *testing.T) {
			s, d, f, _ := graphOutputSettlementFixture(t)
			switch kind {
			case "missing_observation":
				f.GraphOutput = nil
			case "missing_fresh_closure":
				f.GraphOutput.VerifiedArtifacts = f.GraphOutput.VerifiedArtifacts[:len(f.GraphOutput.VerifiedArtifacts)-1]
			case "wrong_closed_candidate":
				wrong := domain.NewID()
				f.GraphOutput.CandidateClosure.CandidateID = &wrong
				seal, e := domain.GraphOutputDigest(*f.GraphOutput.CandidateClosure)
				if e != nil {
					t.Fatal(e)
				}
				f.GraphOutput.CandidateClosure.ClosureSHA256 = seal
				ref, proof := graphPayload(t, s, d.Dispatch.AttemptID, "graph_output_closure", []byte(encode(f.GraphOutput.CandidateClosure)))
				f.GraphOutput.Observation.Candidate.ClosureRef = &ref
				f.GraphOutput.VerifiedArtifacts = append(f.GraphOutput.VerifiedArtifacts, proof)
			}
			if _, e := s.SettleWorkflowStep(context.Background(), d.Dispatch.AttemptID, f); e == nil {
				t.Fatal("unsafe graph output became terminal")
			}
			a, e := s.Attempt(context.Background(), d.Dispatch.AttemptID)
			if e != nil || a.Phase == "settled" {
				t.Fatal("output validation failed after partial terminal commit", a, e)
			}
			stored, e := s.GraphDispatchForAttempt(context.Background(), d.Dispatch.AttemptID)
			if e != nil || stored.Public.OutputObservation != nil {
				t.Fatal(stored, e)
			}
		})
	}
}

func TestGraphOutputSelectedCandidateIgnoresIncidentalResponseEligibility(t *testing.T) {
	for _, state := range []string{"omitted", "incomplete"} {
		t.Run(state, func(t *testing.T) {
			s, d, f, _ := graphOutputSettlementFixture(t)
			response, _ := graphPayload(t, s, d.Dispatch.AttemptID, "final_response", []byte("incidental authored-file explanation"))
			if _, e := s.RecordAttempt(context.Background(), d.Dispatch.AttemptID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &response.ArtifactID}); e != nil {
				t.Fatal(e)
			}
			if state == "incomplete" {
				f.GraphOutput.Observation.Response = &domain.GraphResponseOutputObservation{ResponseArtifactID: response.ArtifactID, State: "incomplete"}
			}
			if _, e := s.SettleWorkflowStep(context.Background(), d.Dispatch.AttemptID, f); e != nil {
				t.Fatal("incidental raw prose blocked the selected candidate", e)
			}
			stored, e := s.GraphDispatchForAttempt(context.Background(), d.Dispatch.AttemptID)
			if e != nil || stored.Public.OutputObservation == nil || stored.Public.OutputObservation.Candidate == nil || stored.Public.OutputObservation.Candidate.State != "complete" {
				t.Fatal(stored, e)
			}
			if response := stored.Public.OutputObservation.Response; response != nil && (response.State != "incomplete" || response.ClosureRef != nil) {
				t.Fatal("incidental response acquired false qualifying closure", response)
			}
		})
	}
}
func TestGraphOutputActualFailedTerminalCannotKeepQualifyingAssociation(t *testing.T) {
	s, d, f, _ := graphOutputSettlementFixture(t)
	badExit := 17
	f.Settlement.ExitCode = &badExit
	if _, e := s.SettleWorkflowStep(context.Background(), d.Dispatch.AttemptID, f); e != nil {
		t.Fatal(e)
	}
	stored, e := s.GraphDispatchForAttempt(context.Background(), d.Dispatch.AttemptID)
	if e != nil || stored.Public.OutputObservation == nil || stored.Public.OutputObservation.Candidate == nil || stored.Public.OutputObservation.Candidate.State == "complete" {
		t.Fatal("actual failed native result retained qualifying output", stored, e)
	}
}

func TestGraphOutputUnknownContextEffectsCannotQualifySuccessfulProcess(t *testing.T) {
	s, d, f, _ := graphOutputSettlementFixture(t)
	ctx := context.Background()
	f.Settlement.EffectsKnown = false
	if _, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, f); e != nil {
		t.Fatal(e)
	}
	stored, e := s.GraphDispatchForAttempt(ctx, d.Dispatch.AttemptID)
	if e != nil || stored.Public.OutputObservation == nil || stored.Public.OutputObservation.Candidate == nil || stored.Public.OutputObservation.Candidate.State == "complete" {
		t.Fatal("unknown context effects left qualifying output despite process completion", stored, e)
	}
	a, e := s.Attempt(ctx, d.Dispatch.AttemptID)
	if e != nil || a.Outcome == nil || *a.Outcome == "succeeded" || a.Cleanup.State != "verified" {
		t.Fatal("unknown effects implied overall success or invented process uncertainty", a, e)
	}
	gate, e := s.Gate(ctx)
	if e != nil || gate.State != "fenced" || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != d.Dispatch.AttemptID {
		t.Fatal("uncertain output lost exact ownership", gate, e)
	}
	step, e := s.WorkflowStep(ctx, d.Dispatch.StepID)
	if e != nil || step.AdvanceProof != nil {
		t.Fatal("uncertain output produced new execution authority", step, e)
	}
}
