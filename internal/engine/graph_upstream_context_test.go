package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// This is an actual fresh independent input operation. It contains no invented
// accepted contributor or dependency vector, and invokes no native process.
func graphUpstreamIndependentOperationFixture(t *testing.T) (*Engine, domain.GraphInputOperation, domain.TaskVersionDetail) {
	t.Helper()
	original := "\ufeff" + strings.Repeat("Complete receiver original 東京🙂\r\n", 700)
	f := graphOutputAdmittedNodeFixture(t,
		domain.GraphNodeInput{Key: "producer", OriginalText: "Keep this distinct producer original out of shared instructions."},
		domain.GraphNodeInput{Key: "receiver", OriginalText: original, Acceptance: &domain.AcceptanceInput{ConditionsText: "Keep the complete declared receiver conditions 東京\r\n"}})
	e := f.engine
	ctx, cancel := context.WithCancel(context.Background())
	e.ctx, e.cancel = ctx, cancel
	t.Cleanup(cancel)
	e.continueGraphInputs(f.rootBinding.OperationAttemptID)
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil {
		t.Fatal(err)
	}
	var queued *domain.Attempt
	for i := range pending {
		a := pending[i]
		if a.ExecutorKind != "local_graph" || a.Phase != "queued" {
			continue
		}
		if queued != nil {
			t.Fatal("The controlled independent input fixture has an unexpected extra local operation")
		}
		copy := a
		queued = &copy
	}
	if queued == nil {
		t.Fatal("The real root completion did not register its independent receiving input", pending)
	}
	attempt, err := e.db.ClaimAttempt(ctx, queued.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	op, err := e.db.GraphInputOperationForAttempt(ctx, e.config.OwnerInstanceID, attempt.ID)
	if err != nil || op.Kind != "node_input" || op.Control.TaskID == nil || op.Control.TaskVersion == nil || op.RootBinding == nil ||
		len(op.Contributions) != 0 || len(op.ContributorVector) != 0 {
		t.Fatal("Actual claimed independent operation prerequisite failed", op, err)
	}
	receiver, err := e.db.TaskVersion(ctx, *op.Control.TaskID, *op.Control.TaskVersion)
	if err != nil || receiver.OriginalText != original {
		t.Fatal("The fixture did not capture the complete receiving original", receiver, err)
	}
	t.Cleanup(func() {
		current, err := e.db.Attempt(context.Background(), attempt.ID)
		if err != nil || current.Phase == "settled" {
			return
		}
		artifacts, err := e.db.Artifacts(context.Background(), attempt.ID, 100, "")
		if err != nil {
			t.Error(err)
			return
		}
		known := len(artifacts.Items) == 0
		state, status := "blocked", "incomplete"
		if !known {
			// Direct writer-unit cases may have created evidence without an input
			// completion receipt. Preserve that unresolved preparation ownership.
			state, status = "uncertain", "uncertain"
		}
		if err = e.db.CompleteGraphInput(context.Background(), e.config.OwnerInstanceID, attempt.ID, domain.GraphInputCompletionFacts{
			State: state, EffectCount: "0", EffectsKnown: known, EvidenceStatus: status, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"},
			Failure: domain.Fail("fixture_unqualified_input", "This controlled context fixture created no native process and does not claim complete input or acceptance.")}); err != nil {
			t.Error(err)
		}
	})
	return e, op, receiver
}

func TestGraphUpstreamContextIndependentReadIsEmptyAndWritesNothing(t *testing.T) {
	e, op, _ := graphUpstreamIndependentOperationFixture(t)
	ctx := context.Background()
	before, err := e.db.Artifacts(ctx, op.Attempt.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	budget, err := e.db.GraphBudget(ctx, op.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := e.loadGraphUpstreamContext(ctx, op)
	if err != nil || len(loaded.Sources) != 0 || len(loaded.ConstraintRefs) != 0 || len(loaded.ContributedEvidence) != 0 || len(loaded.SupportedAccess) != 0 || len(loaded.ArtifactRefs) != 0 {
		t.Fatal("A real independent input requires no fabricated upstream source", loaded, err)
	}
	after, err := e.db.Artifacts(ctx, op.Attempt.ID, 100, "")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("Read-only upstream loading wrote evidence", after, err)
	}
	afterBudget, err := e.db.GraphBudget(ctx, op.GraphID)
	if err != nil || !reflect.DeepEqual(budget, afterBudget) {
		t.Fatal("Read-only context loading changed shared model allowance", afterBudget, err)
	}
}

func TestGraphUpstreamContextRejectsAlteredPrivateOperationBeforeReceiverWrites(t *testing.T) {
	e, op, receiver := graphUpstreamIndependentOperationFixture(t)
	ctx := context.Background()
	for _, kind := range []string{"operation_identity", "root_seal", "captured_shared_instructions"} {
		t.Run(kind, func(t *testing.T) {
			altered := op
			switch kind {
			case "operation_identity":
				altered.Control.OperationAttemptID = domain.NewID()
			case "root_seal":
				root := *op.RootBinding
				root.BasisSHA256 = strings.Repeat("f", 64)
				altered.RootBinding = &root
			case "captured_shared_instructions":
				definition := op.Definition
				original := definition.Original
				different := "These are not the captured shared instructions."
				original.SharedInstructions = &different
				definition.Original = original
				altered.Definition = definition
			}
			before, err := e.db.Artifacts(ctx, op.Attempt.ID, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			result, err := e.retainGraphReceiverContext(ctx, altered, receiver)
			if err == nil || result.ConditionsRef.ArtifactID != "" {
				t.Fatal("An altered private operation published receiver originals before validation", kind, result, err)
			}
			after, err := e.db.Artifacts(ctx, op.Attempt.ID, 100, "")
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("Refused operation acquired evidence writes before its guard", kind, after, err)
			}
		})
	}
}

func TestGraphUpstreamContextIndependentReceiverPreservesFullOwnSources(t *testing.T) {
	e, op, receiver := graphUpstreamIndependentOperationFixture(t)
	ctx := context.Background()
	retained, err := e.retainGraphReceiverContext(ctx, op, receiver)
	if err != nil || len(retained.Conditions.Sources) != 2 || len(retained.Request.ContributedEvidence) != 0 {
		t.Fatal("The independent receiver lost its existing complete source shape", retained, err)
	}
	shared, err := e.readGraphArtifact(ctx, retained.Request.GraphInstructionsRef, "graph_original", 64<<20)
	if err != nil || len(shared) != 0 {
		t.Fatal("Shared instructions included unrelated node originals", string(shared), err)
	}
	original, err := e.readGraphArtifact(ctx, retained.Request.NodeInstructionsRef, "graph_original", 64<<20)
	if err != nil || string(original) != receiver.OriginalText {
		t.Fatal("The receiving original was clipped or normalized", err)
	}
	if retained.Request.AcceptanceRef == nil {
		t.Fatal("The complete receiving acceptance source is absent")
	}
	acceptance, err := e.readGraphArtifact(ctx, *retained.Request.AcceptanceRef, "graph_acceptance", 64<<20)
	want, marshalErr := json.Marshal(receiver.Version.Acceptance)
	if err != nil || marshalErr != nil || !bytes.Equal(acceptance, want) {
		t.Fatal("The complete structured receiving acceptance changed", err, marshalErr)
	}
	found := false
	for _, access := range retained.Request.SupportedAccess {
		if access.Ref == "node-instructions" {
			found = access.ArtifactRef == retained.Request.NodeInstructionsRef && access.Mode == "file" && access.Scope == "receiver" && len(access.Paths) == 0
		}
	}
	if !found {
		t.Fatal("The complete large original lost the existing bounded file-access policy", retained.Request.SupportedAccess)
	}
}
