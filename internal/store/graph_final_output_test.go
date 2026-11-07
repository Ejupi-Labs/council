package store

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These are actual registered Store transactions with typed controller proofs.
// They do not claim filesystem verification, native execution or acceptance.
func graphFinalOutputFixture(t *testing.T, responseOnly bool) (*Store, domain.GraphDispatchView, domain.WorkflowStepFacts) {
	t.Helper()
	s, d, f, _ := graphOutputConfiguredFixture(t, responseOnly, func(in *domain.GraphStartInput) {
		in.Nodes = in.Nodes[:1]
		in.FinalNodeKey = in.Nodes[0].Key
		if !responseOnly {
			in.Nodes[0].Acceptance = &domain.AcceptanceInput{Deliverable: "candidate"}
		}
	})
	graphFinalOutputFacts(t, s, d, &f)
	return s, d, f
}

func graphFinalOutputFacts(t *testing.T, s *Store, d domain.GraphDispatchView, f *domain.WorkflowStepFacts) {
	t.Helper()
	ctx := context.Background()
	root, err := s.GraphRoot(ctx, d.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.GraphInputRecord(ctx, d.Dispatch.GraphID, d.Dispatch.InputID)
	if err != nil || input.Public.InputBinding == nil || input.Public.ConditionsRef == nil || input.ConditionsBinding == nil || input.Public.ContextPacket == nil {
		t.Fatal("Fixture lacks the actual admitted input conditions and access packet", input, err)
	}
	closure := f.GraphOutput.CandidateClosure
	c, err := s.Candidate(ctx, f.GraphOutput.Observation.Candidate.CandidateID)
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Workflow(ctx, d.Dispatch.TaskID, domain.WorkflowSelector{WorkflowID: d.Dispatch.WorkflowID})
	if err != nil || w.Workflow == nil {
		t.Fatal(w, err)
	}
	binding := domain.CandidateBinding{RootSourceSnapshotID: c.RootSourceSnapshotID, CandidateID: c.ID, SourceSnapshotID: c.SourceSnapshotID, ManifestArtifactID: closure.Manifest.ArtifactID, ManifestSHA256: closure.Manifest.SHA256, IndexArtifactID: closure.Index.ArtifactID, IndexSHA256: closure.Index.SHA256, ConditionsSHA256: w.Workflow.Authority.ConditionsSHA256}
	required := map[string]bool{}
	for _, source := range input.ConditionsBinding.Sources {
		required[source.OriginalRef.ArtifactID] = true
		if source.GuidanceRef != nil {
			required[source.GuidanceRef.ArtifactID] = true
		}
		if source.AcceptanceRef != nil {
			required[source.AcceptanceRef.ArtifactID] = true
		}
	}
	access := []domain.GraphProvidedAccess{}
	for _, item := range input.Public.ContextPacket.ProvidedAccess {
		if required[item.ArtifactRef.ArtifactID] {
			access = append(access, item)
			delete(required, item.ArtifactRef.ArtifactID)
		}
	}
	if len(required) != 0 || len(access) < 3 {
		t.Fatal("Fixture does not supply every complete scoped original", required, access)
	}
	delta, deltaProof := graphPayload(t, s, d.Dispatch.AttemptID, "graph_cumulative_delta", []byte(`{"controlled":"root-to-selected cumulative facts"}`))
	final := domain.GraphFinalVerificationBinding{GraphID: d.Dispatch.GraphID, GraphVersion: d.Dispatch.GraphVersion, FinalNodeID: d.Dispatch.NodeID, FinalTaskID: d.Dispatch.TaskID, FinalTaskVersion: d.Dispatch.TaskVersion, FinalWorkflowID: d.Dispatch.WorkflowID, RootBinding: domain.GraphRootPublic(root), FinalCandidateBinding: binding, OutputClosureRef: *f.GraphOutput.Observation.Candidate.ClosureRef, FinalInputID: d.Dispatch.InputID, ContributorVector: input.Public.InputBinding.ContributorVector, GraphConditionsRef: *input.Public.ConditionsRef, GraphConditionsSHA256: input.ConditionsBinding.ConditionsSHA256, OriginalAccess: access, CumulativeDeltaRef: delta, HasCumulativeWorkingDelta: true}
	f.GraphOutput.FinalVerification = &final
	f.GraphOutput.VerifiedArtifacts = append(f.GraphOutput.VerifiedArtifacts, deltaProof)
	refs := []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs, input.Public.InputBinding.Manifest, input.Public.InputBinding.Index, input.Public.InputBinding.StagedBlobs, input.Public.InputBinding.Provenance, input.Public.InputBinding.ContextPacket, *input.Public.ConditionsRef}
	for _, item := range access {
		refs = append(refs, item.ArtifactRef)
	}
	for _, ref := range refs {
		a, err := s.Artifact(ctx, ref.ArtifactID)
		if err != nil || a.SHA256 == nil {
			t.Fatal(ref, a, err)
		}
		f.GraphOutput.VerifiedArtifacts = append(f.GraphOutput.VerifiedArtifacts, domain.VerifiedResponse{ArtifactID: a.ID, SHA256: *a.SHA256, ByteLength: a.ByteLength, PhysicalIdentity: a.PhysicalIdentity})
	}
	graphFinalOutputReseal(t, s, d, f)
}

func graphFinalOutputReseal(t *testing.T, s *Store, d domain.GraphDispatchView, f *domain.WorkflowStepFacts) {
	t.Helper()
	final := f.GraphOutput.FinalVerification
	var err error
	final.BindingSHA256, err = domain.GraphFinalVerificationDigest(*final)
	if err != nil {
		t.Fatal(err)
	}
	ref, proof := graphPayload(t, s, d.Dispatch.AttemptID, "graph_final_verification", []byte(encode(final)))
	f.GraphOutput.Observation.Candidate.FinalVerificationRef = &ref
	f.GraphOutput.VerifiedArtifacts = append(f.GraphOutput.VerifiedArtifacts, proof)
}

func TestGraphFinalOutputAssociatesExactFileBasisInProducerTerminal(t *testing.T) {
	s, d, f := graphFinalOutputFixture(t, false)
	ctx := context.Background()
	if _, err := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, f); err != nil {
		t.Fatal("Exact final-file basis could not settle atomically", err)
	}
	stored, err := s.GraphDispatchForAttempt(ctx, d.Dispatch.AttemptID)
	if err != nil || stored.Public.OutputObservation == nil || !reflect.DeepEqual(stored.FinalVerification, f.GraphOutput.FinalVerification) || !reflect.DeepEqual(stored.Public.OutputObservation.Candidate.FinalVerificationRef, f.GraphOutput.Observation.Candidate.FinalVerificationRef) {
		t.Fatal("Terminal did not retain the exact final basis", stored, err)
	}
	a, err := s.Attempt(ctx, d.Dispatch.AttemptID)
	if err != nil || a.Phase != "settled" || a.Outcome == nil || *a.Outcome != "succeeded" {
		t.Fatal(a, err)
	}
	if _, err = s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, f); err != nil {
		t.Fatal("Exact terminal replay could not reconcile", err)
	}
	var terminals int
	if err = s.db.QueryRow("SELECT count(*) FROM attempt_terminals WHERE attempt_id=?", a.ID).Scan(&terminals); err != nil || terminals != 1 {
		t.Fatal("Replay created a second terminal", terminals, err)
	}
}

func TestGraphFinalOutputRejectsResealedWrongTupleWithoutTerminalCommit(t *testing.T) {
	for _, change := range []string{"graph", "graph_version", "task", "task_version", "workflow", "input", "unexpected_unaccepted_vector", "public_root", "selected_candidate_source", "output_closure", "cumulative_ref", "conditions_ref", "conditions_digest", "missing_original", "original_scope", "subset_original", "unrelated_original"} {
		t.Run(change, func(t *testing.T) {
			s, d, f := graphFinalOutputFixture(t, false)
			final := f.GraphOutput.FinalVerification
			switch change {
			case "graph":
				final.GraphID = domain.NewID()
			case "graph_version":
				final.GraphVersion = "2"
			case "task":
				final.FinalTaskID = domain.NewID()
			case "task_version":
				final.FinalTaskVersion = "2"
			case "workflow":
				final.FinalWorkflowID = domain.NewID()
			case "input":
				final.FinalInputID = domain.NewID()
			case "unexpected_unaccepted_vector":
				// This real node has no acceptance. Do not manufacture an ID or
				// present its producer tuple as an accepted upstream contributor.
				final.ContributorVector = []domain.GraphContributorBinding{{NodeID: d.Dispatch.NodeID, TaskID: d.Dispatch.TaskID, TaskVersion: d.Dispatch.TaskVersion, ProducerAttemptID: d.Dispatch.AttemptID, InputID: d.Dispatch.InputID, CandidateBinding: f.CandidateBinding, OutputClosureRef: final.OutputClosureRef}}
			case "public_root":
				final.RootBinding.BasisSHA256 = strings.Repeat("c", 64)
			case "selected_candidate_source":
				final.FinalCandidateBinding.SourceSnapshotID = domain.NewID()
			case "output_closure":
				ref, proof := graphPayload(t, s, d.Dispatch.AttemptID, "graph_output_closure", []byte(encode(f.GraphOutput.CandidateClosure)))
				final.OutputClosureRef = ref
				f.GraphOutput.VerifiedArtifacts = append(f.GraphOutput.VerifiedArtifacts, proof)
			case "cumulative_ref":
				final.CumulativeDeltaRef = final.RootBinding.Manifest
			case "conditions_ref":
				final.GraphConditionsRef = final.RootBinding.Manifest
			case "conditions_digest":
				final.GraphConditionsSHA256 = strings.Repeat("d", 64)
			case "missing_original":
				final.OriginalAccess = final.OriginalAccess[1:]
			case "original_scope":
				final.OriginalAccess[0].Scope = "receiver"
			case "subset_original":
				final.OriginalAccess[0].Paths = []string{"source.txt"}
			case "unrelated_original":
				final.OriginalAccess[0].ArtifactRef = f.GraphOutput.CandidateClosure.Manifest
			}
			// The wrong tuple is genuinely registered with matching bytes, seal
			// and proof, so a rejection must bind authority rather than hash alone.
			graphFinalOutputReseal(t, s, d, &f)
			graphFinalOutputRequireRollback(t, s, d, f)
		})
	}
}

func TestGraphFinalOutputRequiresFreshRootConditionsDeltaAndOriginalProofs(t *testing.T) {
	for _, missing := range []string{"final", "delta", "original", "conditions", "root", "input_context"} {
		t.Run(missing, func(t *testing.T) {
			s, d, f := graphFinalOutputFixture(t, false)
			final := f.GraphOutput.FinalVerification
			id := f.GraphOutput.Observation.Candidate.FinalVerificationRef.ArtifactID
			switch missing {
			case "delta":
				id = final.CumulativeDeltaRef.ArtifactID
			case "original":
				id = final.OriginalAccess[0].ArtifactRef.ArtifactID
			case "conditions":
				id = final.GraphConditionsRef.ArtifactID
			case "root":
				id = final.RootBinding.Manifest.ArtifactID
			case "input_context":
				record, err := s.GraphInputRecord(context.Background(), d.Dispatch.GraphID, d.Dispatch.InputID)
				if err != nil {
					t.Fatal(err)
				}
				id = record.Public.InputBinding.ContextPacket.ArtifactID
			}
			kept := []domain.VerifiedResponse{}
			removed := false
			for _, proof := range f.GraphOutput.VerifiedArtifacts {
				if proof.ArtifactID == id {
					removed = true
				} else {
					kept = append(kept, proof)
				}
			}
			if !removed {
				t.Fatal("Fixture did not remove a real supplied fresh proof", id)
			}
			f.GraphOutput.VerifiedArtifacts = kept
			graphFinalOutputRequireRollback(t, s, d, f)
		})
	}
}

func TestGraphFinalOutputRejectsResponseOnlyFinalBasis(t *testing.T) {
	s, d, f := graphFinalOutputFixture(t, true)
	graphFinalOutputRequireRollback(t, s, d, f)
}

func graphFinalOutputRequireRollback(t *testing.T, s *Store, d domain.GraphDispatchView, f domain.WorkflowStepFacts) {
	t.Helper()
	ctx := context.Background()
	before, err := s.GraphBudget(ctx, d.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, f); err == nil {
		t.Fatal("A wrong final-file basis became terminal with a usable association")
	}
	a, err := s.Attempt(ctx, d.Dispatch.AttemptID)
	if err != nil || a.Phase == "settled" {
		t.Fatal("Rejected final basis partly committed a producer terminal", a, err)
	}
	stored, err := s.GraphDispatchForAttempt(ctx, d.Dispatch.AttemptID)
	if err != nil || stored.Public.OutputObservation != nil || stored.FinalVerification != nil {
		t.Fatal("Rejected final basis partly installed its output association", stored, err)
	}
	after, err := s.GraphBudget(ctx, d.Dispatch.GraphID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("Rejected terminal changed the shared native authority", before, after, err)
	}
	var terminals int
	if err = s.db.QueryRow("SELECT count(*) FROM attempt_terminals WHERE attempt_id=?", a.ID).Scan(&terminals); err != nil || terminals != 0 {
		t.Fatal("Rejected output retained a terminal row", terminals, err)
	}
}

func TestGraphFinalOutputNonFinalRegistrationRemainsProhibited(t *testing.T) {
	// This existing artifact-role guard is a scope regression, not a claim that
	// the old FinalVerificationRef subblock itself was exercised by this case.
	s, d, _, _ := graphOutputSettlementFixture(t)
	record := graphArtifactMetadata(d.Dispatch.AttemptID, "graph_final_verification")
	if err := s.PutArtifact(context.Background(), record); err == nil {
		t.Fatal("A real nonfinal producer registered a final verification artifact")
	}
}
