package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func TestGraphFinalContextSourcesPreserveActualNodeAndCompleteCumulativeHeaders(t *testing.T) {
	e, subject, _, _, _, _ := graphFinalVerificationFixture(t)
	verified, err := e.loadGraphFinalVerification(context.Background(), subject)
	if err != nil || verified == nil {
		t.Fatal("Actual selected cumulative final prerequisite failed", err)
	}
	beforeSubject, beforeBinding := *subject.Candidate, *subject.Binding
	packet, err := e.workspace.VerifyGraphInput(context.Background(), verified.Input)
	if err != nil {
		t.Fatal(err)
	}
	beforePacket := packet.ContextPacket
	sources, err := graphFinalContextSources(verified)
	if err != nil {
		t.Fatal("Actual cumulative final has no complete supplemental context plan", err)
	}
	expected := map[string]domain.GraphArtifactRef{
		"final-verification":        verified.BindingRef,
		"final-cumulative-delta":    verified.Binding.CumulativeDeltaRef,
		"final-root-manifest":       verified.Root.Manifest,
		"final-root-index":          verified.Root.Index,
		"final-root-staged-blobs":   verified.Root.StagedBlobs,
		"final-output-closure":      verified.Binding.OutputClosureRef,
		"final-output-manifest":     verified.OutputClosure.Manifest,
		"final-output-index":        verified.OutputClosure.Index,
		"final-output-staged-blobs": verified.OutputClosure.StagedBlobs,
	}
	seen := map[string]bool{}
	for _, source := range sources {
		if seen[source.Ref] || source.Scope != "cumulative_final" || source.Paths == nil {
			t.Fatal("The final role evidence lost its separate complete scope", source)
		}
		seen[source.Ref] = true
		if wanted, ok := expected[source.Ref]; ok {
			if source.ArtifactRef != wanted || len(source.Paths) != 0 {
				t.Fatal("A cumulative header was substituted or scoped to a partial path", source, wanted)
			}
			delete(expected, source.Ref)
		}
	}
	if len(expected) != 0 || len(sources) < 9 {
		t.Fatal("Complete final/root/output evidence was omitted", expected)
	}
	again, err := graphFinalContextSources(verified)
	if err != nil || !reflect.DeepEqual(sources, again) || !reflect.DeepEqual(beforeSubject, *subject.Candidate) || !reflect.DeepEqual(beforeBinding, *subject.Binding) ||
		!reflect.DeepEqual(beforePacket, packet.ContextPacket) || subject.Candidate.HasDelta || subject.Candidate.HasIndexDelta || !verified.Binding.HasCumulativeWorkingDelta {
		t.Fatal("Planning rewrote node authorship, the original packet, or deterministic supplemental order", err)
	}
	if sources, err := graphFinalContextSources(nil); err != nil || sources != nil {
		t.Fatal("Ordinary context acquired supplemental final evidence", sources, err)
	}
}

// This is only a semantic reducer test. Its synthetic inventory does not claim
// an admitted worker, acceptance, closed-artifact verification or a provider read.
func TestGraphFinalContextMemberSourcesPreserveBeforeAndDistinctStagedBytes(t *testing.T) {
	ref := func(text string) domain.GraphArtifactRef {
		return domain.GraphArtifactRef{ArtifactID: domain.NewID(), ProducerAttemptID: domain.NewID(), SHA256: workflowHash([]byte(text)), ByteLength: strconv.Itoa(len(text))}
	}
	modified, deleted := ref("original working 東京\r\n"), ref("deleted original 🙂\r\n")
	stagedBefore, stagedAfter := ref("staged before\r\n"), ref("staged after\r\n")
	beforeObject, afterObject := "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"
	entry := func(path string, source domain.GraphArtifactRef) *workspace.Entry {
		size, _ := strconv.ParseInt(source.ByteLength, 10, 64)
		return &workspace.Entry{Path: path, Type: "file", Mode: 0600, Size: size, ArtifactID: source.ArtifactID, SHA256: source.SHA256}
	}
	beforeIndex := workspace.IndexEntry{Path: "modified.txt", Mode: "100644", Object: beforeObject}
	afterIndex := workspace.IndexEntry{Path: "modified.txt", Mode: "100644", Object: afterObject}
	changes := []workspace.Change{{Path: "modified.txt", Kind: "modified", Before: entry("modified.txt", modified), After: &workspace.Entry{Path: "modified.txt", Type: "file"}, BeforeIndex: &beforeIndex, AfterIndex: &afterIndex},
		{Path: "deleted.txt", Kind: "deleted", Before: entry("deleted.txt", deleted)}}
	before := workspace.GraphIndexBlobManifest{Contract: "006-index-blobs-v1", Entries: []workspace.GraphIndexBlob{{ObjectID: beforeObject, ContentSHA256: stagedBefore.SHA256, ByteLength: stagedBefore.ByteLength, ArtifactRef: stagedBefore}}}
	after := workspace.GraphIndexBlobManifest{Contract: "006-index-blobs-v1", Entries: []workspace.GraphIndexBlob{{ObjectID: afterObject, ContentSHA256: stagedAfter.SHA256, ByteLength: stagedAfter.ByteLength, ArtifactRef: stagedAfter}}}
	refs := []domain.GraphArtifactRef{modified, deleted, stagedBefore, stagedAfter}
	beforeChanges := append([]workspace.Change{}, changes...)
	sources, err := graphFinalContextMemberSources(changes, before, after, refs)
	if err != nil {
		t.Fatal("Complete before and staged byte inventories have no supplemental plan", err)
	}
	wanted := map[string]string{modified.ArtifactID: "modified.txt", deleted.ArtifactID: "deleted.txt", stagedBefore.ArtifactID: "modified.txt", stagedAfter.ArtifactID: "modified.txt"}
	for _, source := range sources {
		path, ok := wanted[source.ArtifactRef.ArtifactID]
		if !ok || source.Scope != "cumulative_final" || len(source.Paths) != 1 || source.Paths[0] != path {
			t.Fatal("A working-before or staged-before/after source was conflated", source)
		}
		delete(wanted, source.ArtifactRef.ArtifactID)
	}
	if len(sources) != 4 || len(wanted) != 0 || !reflect.DeepEqual(beforeChanges, changes) {
		t.Fatal("A before source was dropped or changed during reduction", sources, wanted)
	}
	for _, missing := range []string{modified.ArtifactID, deleted.ArtifactID, stagedBefore.ArtifactID, stagedAfter.ArtifactID} {
		filtered := []domain.GraphArtifactRef{}
		for _, ref := range refs {
			if ref.ArtifactID != missing {
				filtered = append(filtered, ref)
			}
		}
		if sources, err := graphFinalContextMemberSources(changes, before, after, filtered); err == nil || len(sources) != 0 {
			t.Fatal("A missing before/staged member became a partial usable context", missing, sources, err)
		}
	}
	// A working-file change can carry unchanged index context. Those staged
	// bytes remain independently inspectable even though they are not a delta.
	unchanged := append([]workspace.Change{}, changes[:1]...)
	unchanged[0].AfterIndex = unchanged[0].BeforeIndex
	unchangedSources, err := graphFinalContextMemberSources(unchanged, before, before, []domain.GraphArtifactRef{modified, stagedBefore})
	if err != nil || len(unchangedSources) != 3 {
		t.Fatal("Unchanged staged context was conflated with working-before bytes", unchangedSources, err)
	}
}

type graphFinalReviewCase struct {
	engine   *Engine
	subject  workflowSubject
	verified *graphFinalVerification
	reviewer domain.Attempt
	dispatch domain.GraphDispatch
	repo     string
	working  []byte
	index    []byte
}

// The later role comes from an actual grant and explicit Advance. Reusing the
// producer's context directory or inventing another attempt would miss this seam.
func graphFinalReviewAttemptFixture(t *testing.T) graphFinalReviewCase {
	t.Helper()
	return graphFinalReviewAttemptWithWorkflowFixture(t, nil)
}

func graphFinalReviewAttemptWithWorkflowFixture(t *testing.T, finalWorkflow *domain.WorkflowInput) graphFinalReviewCase {
	t.Helper()
	return graphFinalReviewAttemptWithOptionsFixture(t, graphFinalVerificationOptions{FinalWorkflow: finalWorkflow})
}

func graphFinalReviewAttemptWithOptionsFixture(t *testing.T, options graphFinalVerificationOptions) graphFinalReviewCase {
	t.Helper()
	e, subject, selected, repo, working, index := graphFinalVerificationWithOptionsFixture(t, options)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.ctx, e.cancel = ctx, cancel
	verified, err := e.loadGraphFinalVerification(ctx, subject)
	if err != nil || verified == nil {
		t.Fatal("Actual selected file-final prerequisite failed", err)
	}
	view, err := e.db.Graph(ctx, selected.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	grant := domain.GraphBudgetAmendmentInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: view.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, AdditionalAdmissions: "1"}
	body, err := json.Marshal(grant)
	if err != nil {
		t.Fatal(err)
	}
	if result := e.MutateGraph(ctx, "graphs.budget.amend", view.Graph.ID, body); result.Status != 200 || result.Envelope.Error != nil {
		t.Fatal("Actual final review grant prerequisite failed", result)
	}
	view, err = e.db.Graph(ctx, view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}}
	for _, node := range view.Nodes {
		in.NodeAuthorities = append(in.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	body, err = json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	result := e.MutateGraph(ctx, "graphs.advance", view.Graph.ID, body)
	var admitted domain.GraphAdvanceResult
	wire, _ := json.Marshal(result.Envelope.Data)
	decodeErr := json.Unmarshal(wire, &admitted, json.RejectUnknownMembers(true))
	if result.Status != 200 || result.Envelope.Error != nil || decodeErr != nil || len(admitted.Dispatches) != 1 || len(admitted.PreparationAttemptIDs) != 0 {
		t.Fatal("Actual final-derived review admission prerequisite failed", result, admitted, decodeErr)
	}
	d := admitted.Dispatches[0].Dispatch
	if d.NodeID != selected.Dispatch.NodeID || d.WorkflowID != subject.Workflow.ID || d.TaskVersion != subject.Workflow.TaskVersion || d.Role != "review" || d.ExecutorKind != "native" ||
		d.SourceBasis.BasisKind != "node_subject" || d.SourceBasis.CandidateBinding == nil || *d.SourceBasis.CandidateBinding != *subject.Binding ||
		d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != verified.BindingRef {
		t.Fatal("The actual derived reviewer does not carry the exact selected final basis", d)
	}
	reviewer, err := e.db.ClaimAttempt(ctx, d.AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	return graphFinalReviewCase{e, subject, verified, reviewer, d, repo, working, index}
}

func TestGraphFinalContextActualNativeReviewReadsCumulativeEvidenceWithoutNodeAuthorship(t *testing.T) {
	f := graphFinalReviewAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	var spec process.NativeSpec
	prepares := 0
	e.config.Prepare = func(ctx context.Context, actual process.NativeSpec) (*process.Owned, error) {
		prepares++
		spec = actual
		spec.Args, spec.Stdin = append([]string{}, actual.Args...), append([]byte{}, actual.Stdin...)
		return process.Prepare(ctx, actual)
	}
	e.run(ctx, f.reviewer)
	var payload struct {
		Final *struct {
			Contract             string                  `json:"contract"`
			ReceivingAttemptID   string                  `json:"receivingAttemptId"`
			FinalVerificationRef domain.GraphArtifactRef `json:"finalVerificationRef"`
			Access               []struct {
				Ref         string                  `json:"ref"`
				ArtifactRef domain.GraphArtifactRef `json:"artifactRef"`
				Scope       string                  `json:"scope"`
				Paths       []string                `json:"paths"`
				Mode        string                  `json:"mode"`
				Path        *string                 `json:"path"`
			} `json:"access"`
		} `json:"graphFinalVerification"`
	}
	line := bytes.IndexByte(spec.Stdin, '\n')
	if prepares != 1 || line < 0 || json.Unmarshal(spec.Stdin[line+1:], &payload) != nil || payload.Final == nil {
		t.Fatal("The actual admitted final reviewer received no cumulative evidence transport", prepares)
	}
	if payload.Final.Contract != "006-final-context-v1" || payload.Final.ReceivingAttemptID != f.reviewer.ID || payload.Final.FinalVerificationRef != f.verified.BindingRef {
		t.Fatal("The actual final context substituted its receiving execution or selected basis", payload.Final)
	}
	sources, err := graphFinalContextSources(f.verified)
	if err != nil || len(payload.Final.Access) != len(sources) {
		t.Fatal("The actual final prompt omitted required whole cumulative sources", err)
	}
	wanted := map[string]workspace.GraphContextSupplement{}
	for _, source := range sources {
		wanted[source.Ref] = source
	}
	for _, access := range payload.Final.Access {
		source, ok := wanted[access.Ref]
		if !ok || access.ArtifactRef != source.ArtifactRef || access.Scope != source.Scope || !reflect.DeepEqual(access.Paths, source.Paths) || access.Mode != "file" || access.Path == nil {
			t.Fatal("The actual final worker was given a substituted or partial source", access)
		}
		delete(wanted, access.Ref)
	}
	current, err := e.db.Attempt(ctx, f.reviewer.ID)
	if err != nil || current.Phase != "settled" || current.Outcome == nil || *current.Outcome != "succeeded" || current.StartedAt == nil || current.Cleanup.State != "verified" || current.EvidenceStatus != "complete" {
		t.Fatal("The real final reviewer did not complete its owned role", current, err)
	}
	artifacts, err := e.db.Artifacts(ctx, f.reviewer.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	var observed []byte
	for _, artifact := range artifacts.Items {
		if artifact.Kind == "stdout" {
			meta, err := e.evidence.VerifyClosed(ctx, artifact.ID)
			if err != nil {
				t.Fatal(err)
			}
			observed, err = e.readAllResponse(meta)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if !bytes.Contains(observed, []byte(`"fixture_context_reads"`)) {
		t.Fatal("No actual final source inspection receipt was retained")
	}
	for _, source := range sources {
		if !bytes.Contains(observed, []byte(source.Ref)) || !bytes.Contains(observed, []byte(source.ArtifactRef.ArtifactID)) || !bytes.Contains(observed, []byte(source.ArtifactRef.SHA256)) {
			t.Fatal("The owned final reviewer did not actually read a complete required source", source.Ref)
		}
	}
	reviews, err := e.db.WorkflowReviews(ctx, f.reviewer.TaskID, domain.WorkflowSelector{WorkflowID: f.reviewer.WorkflowID}, 100, "")
	if err != nil || len(reviews.Items) != 1 || reviews.Items[0].Outcome != "approved" {
		t.Fatal("The cumulative final review was not retained", reviews, err)
	}
	review := reviews.Items[0]
	for _, source := range sources {
		found := false
		for _, inspection := range review.Report.InspectedEvidence {
			if inspection.Ref == source.Ref && reflect.DeepEqual(inspection.Paths, source.Paths) && (len(source.Paths) == 0 && inspection.Scope == "whole_candidate" || len(source.Paths) != 0 && inspection.Scope == "declared_subset") {
				found = true
			}
		}
		if !found {
			t.Fatal("The final report omitted a read or extended the closed inspection scope enum", source.Ref)
		}
	}
	scope := domain.CheckScope{Kind: review.Applicability.ScopeKind, Paths: review.Applicability.Paths}
	cumulative, err := e.workflowApplicability(ctx, f.subject, scope, review.Applicability.ConditionKeys, review.Applicability.DefinitionSHA256, f.verified)
	if err != nil || !workflowpolicy.Applicable(review.Applicability, cumulative) {
		t.Fatal("Actual final review did not qualify the exact cumulative basis", review.Applicability, cumulative, err)
	}
	nodeOnly, err := e.workflowApplicability(ctx, f.subject, scope, review.Applicability.ConditionKeys, review.Applicability.DefinitionSHA256)
	if err != nil || workflowpolicy.Applicable(nodeOnly, cumulative) {
		t.Fatal("A node-only receipt was relabelled cumulative", nodeOnly, cumulative, err)
	}
	candidate, err := e.db.Candidate(ctx, f.subject.Binding.CandidateID)
	if err != nil || candidate.HasDelta || candidate.HasIndexDelta || candidate.SourceSnapshotID != f.subject.Candidate.SourceSnapshotID {
		t.Fatal("Cumulative review manufactured final-node authorship", candidate, err)
	}
	if body, err := os.ReadFile(filepath.Join(f.repo, "source.txt")); err != nil || !bytes.Equal(body, f.working) {
		t.Fatal("Final verification changed user working bytes", err)
	}
	if body, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(body, f.index) {
		t.Fatal("Final verification changed the exact user index", err)
	}
}

const graphFinalContextCheckTestArg = "__council_graph_final_context_check_test_v1"

// TestMain-only fixed command. It accepts no path/script/executable argv. Pass
// and fail are read-only; mutate adds only the fixed controlled file in its own
// managed receiving workspace after verifying all supplied context.
func handleGraphFinalContextCheck(args []string) (bool, int) {
	if len(args) == 0 || args[0] != graphFinalContextCheckTestArg {
		return false, 0
	}
	if len(args) != 3 || args[1] != "--outcome" || args[2] != "pass" && args[2] != "fail" && args[2] != "mutate" {
		return true, 2
	}
	if stdin, err := io.ReadAll(io.LimitReader(os.Stdin, 1)); err != nil || len(stdin) != 0 {
		return true, 2
	}
	descriptorPath := os.Getenv("COUNCIL_GRAPH_CONTEXT_FILE")
	directory := filepath.Dir(descriptorPath)
	if !filepath.IsAbs(descriptorPath) || filepath.Clean(descriptorPath) != descriptorPath || filepath.Base(descriptorPath) != "final-context.json" ||
		!strings.HasPrefix(filepath.Base(directory), "graph-context-") {
		return true, 9
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return true, 9
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return true, 9
	}
	privateErr, closeErr := localstate.VerifyPrivateFile(dir, true), dir.Close()
	if privateErr != nil || closeErr != nil {
		return true, 9
	}
	named, err := root.Lstat("final-context.json")
	if err != nil || !named.Mode().IsRegular() {
		return true, 9
	}
	file, err := root.Open("final-context.json")
	if err != nil {
		return true, 9
	}
	opened, statErr := file.Stat()
	privateErr = localstate.VerifyPrivateFile(file, false)
	body, readErr := io.ReadAll(io.LimitReader(file, workspace.MaxFileBytes+1))
	closeErr = file.Close()
	if statErr != nil || privateErr != nil || readErr != nil || closeErr != nil || !os.SameFile(named, opened) || int64(len(body)) > workspace.MaxFileBytes {
		return true, 9
	}
	var descriptor workspace.GraphFinalContextDescriptor
	if json.Unmarshal(body, &descriptor, json.RejectUnknownMembers(true)) != nil || descriptor.Contract != "006-final-context-v1" ||
		descriptor.ReceivingAttemptID != strings.TrimPrefix(filepath.Base(directory), "graph-context-") || !domain.ValidID(descriptor.ReceivingAttemptID) ||
		!domain.ValidID(descriptor.InputID) || !domain.ValidSHA256(descriptor.InputSHA256) || !domain.ValidSHA256(descriptor.PacketSHA256) ||
		domain.ValidateGraphArtifactRef(descriptor.FinalVerificationRef) != nil || len(descriptor.Access) == 0 || len(descriptor.Access) > 64 {
		return true, 9
	}
	reads := []fixture.GraphContextInspection{}
	seen := map[string]bool{}
	finalCovered := false
	payloads := map[string]domain.GraphArtifactRef{}
	var total int64
	for _, source := range descriptor.Access {
		name := filepath.Base(source.Path)
		if source.Ref == "" || seen[source.Ref] || source.Paths == nil || domain.ValidateGraphArtifactRef(source.ArtifactRef) != nil ||
			filepath.Dir(source.Path) != directory || filepath.Clean(source.Path) != source.Path || len(name) != 10 || name[6:] != ".txt" {
			return true, 9
		}
		seen[source.Ref] = true
		for _, digit := range name[:6] {
			if digit < '0' || digit > '9' {
				return true, 9
			}
		}
		before, err := root.Lstat(name)
		if err != nil || !before.Mode().IsRegular() {
			return true, 9
		}
		copy, err := root.Open(name)
		if err != nil {
			return true, 9
		}
		opened, statErr := copy.Stat()
		privateErr := localstate.VerifyPrivateFile(copy, false)
		if statErr != nil || privateErr != nil || !os.SameFile(before, opened) {
			copy.Close()
			return true, 9
		}
		hash := sha256.New()
		length, readErr := io.Copy(hash, io.LimitReader(copy, workspace.MaxFileBytes+1))
		after, afterErr := copy.Stat()
		current, nameErr := root.Lstat(name)
		privateErr, closeErr := localstate.VerifyPrivateFile(copy, false), copy.Close()
		actualSHA, actualLength := hex.EncodeToString(hash.Sum(nil)), strconv.FormatInt(length, 10)
		if old, exists := payloads[source.ArtifactRef.ArtifactID]; exists {
			if old != source.ArtifactRef {
				return true, 9
			}
		} else {
			total += length
			payloads[source.ArtifactRef.ArtifactID] = source.ArtifactRef
		}
		if readErr != nil || afterErr != nil || nameErr != nil || privateErr != nil || closeErr != nil || length > workspace.MaxFileBytes || total > workspace.MaxTotalBytes ||
			length != opened.Size() || length != after.Size() || !opened.ModTime().Equal(after.ModTime()) || !os.SameFile(before, current) ||
			actualSHA != source.ArtifactRef.SHA256 || actualLength != source.ArtifactRef.ByteLength {
			return true, 9
		}
		reads = append(reads, fixture.GraphContextInspection{Ref: source.Ref, ArtifactID: source.ArtifactRef.ArtifactID, Mode: "file", ReadVia: "file", SHA256: actualSHA, ByteLength: actualLength})
		if source.Ref == "final-verification" && source.Scope == "cumulative_final" && source.ArtifactRef == descriptor.FinalVerificationRef && len(source.Paths) == 0 {
			finalCovered = true
		}
	}
	if !finalCovered || os.Getenv("COUNCIL_TEST_FINAL_CHECK_SENTINEL") != "preserved" {
		return true, 9
	}
	observed, err := json.Marshal(struct {
		Reads         []fixture.GraphContextInspection `json:"fixture_final_context_reads"`
		DescriptorSHA string                           `json:"descriptorSha256"`
		Sentinel      bool                             `json:"inheritedSentinelObserved"`
	}{reads, workflowHash(body), true})
	if err != nil {
		return true, 3
	}
	fmt.Fprintln(os.Stdout, string(observed))
	if args[2] == "mutate" {
		if err := graphCommandWriteFixedMutation(directory); err != nil {
			return true, 9
		}
	}
	if args[2] == "fail" {
		return true, 1
	}
	return true, 0
}

type graphFinalCheckCase struct {
	graphFinalReviewCase
	check      domain.Attempt
	dispatch   domain.GraphDispatch
	definition domain.CheckDefinition
	budget     domain.GraphBudgetView
}

func graphFinalCommandAttemptFixture(t *testing.T) graphFinalCheckCase {
	t.Helper()
	return graphFinalCommandAttemptForOutcomeFixture(t, "pass")
}

func graphFinalCommandAttemptForOutcomeFixture(t *testing.T, outcome string) graphFinalCheckCase {
	t.Helper()
	if outcome != "pass" && outcome != "fail" && outcome != "mutate" {
		t.Fatal("Select one fixed local check outcome")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	timeout := 10
	checks := []domain.CheckDeclaration{{Key: "controlled-final-context", Executable: executable, Arguments: []string{graphFinalContextCheckTestArg, "--outcome", outcome}, RelativeWorkingDirectory: ".", TimeoutSeconds: &timeout}}
	f := graphFinalReviewAttemptWithWorkflowFixture(t, &domain.WorkflowInput{Review: "required", Checks: &checks})
	e, ctx := f.engine, context.Background()
	e.run(ctx, f.reviewer)
	reviewed, err := e.db.Attempt(ctx, f.reviewer.ID)
	if err != nil || reviewed.Phase != "settled" || reviewed.Outcome == nil || *reviewed.Outcome != "succeeded" || reviewed.EvidenceStatus != "complete" {
		t.Fatal("Actual cumulative final review prerequisite failed before local-check admission", reviewed, err)
	}
	view, err := e.db.Graph(ctx, f.dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}}
	for _, node := range view.Nodes {
		in.NodeAuthorities = append(in.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	result := e.MutateGraph(ctx, "graphs.advance", view.Graph.ID, body)
	var advanced domain.GraphAdvanceResult
	wire, _ := json.Marshal(result.Envelope.Data)
	decodeErr := json.Unmarshal(wire, &advanced, json.RejectUnknownMembers(true))
	if result.Status != 200 || result.Envelope.Error != nil || decodeErr != nil || len(advanced.Dispatches) != 1 || len(advanced.PreparationAttemptIDs) != 0 {
		t.Fatal("Actual final local-check admission prerequisite failed", result, advanced, decodeErr)
	}
	d := advanced.Dispatches[0].Dispatch
	if d.ExecutorKind != "local_check" || d.Role != "check" || d.TaskID != f.reviewer.TaskID || d.WorkflowID != f.reviewer.WorkflowID || d.InputID != f.verified.Input.ID ||
		d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != f.verified.BindingRef || !reflect.DeepEqual(advanced.Budget.Budget, view.Budget.Budget) {
		t.Fatal("The local check did not retain its actual selected final and shared zero-unit authority", d, advanced.Budget)
	}
	check, err := e.db.ClaimAttempt(ctx, d.AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	w, err := e.db.Workflow(ctx, check.TaskID, domain.WorkflowSelector{WorkflowID: check.WorkflowID})
	if err != nil || w.Workflow == nil || len(w.Workflow.Authority.Checks) != 1 || w.Workflow.Authority.Checks[0].Command == nil {
		t.Fatal("The actual captured command definition is unavailable", w, err)
	}
	return graphFinalCheckCase{f, check, d, w.Workflow.Authority.Checks[0], advanced.Budget}
}

func graphFinalCheckDescriptor(spec process.NativeSpec) (string, int) {
	value, count := "", 0
	for _, variable := range spec.Env {
		name, content, ok := strings.Cut(variable, "=")
		if !ok {
			continue
		}
		match := name == "COUNCIL_GRAPH_CONTEXT_FILE"
		if runtime.GOOS == "windows" {
			match = strings.EqualFold(name, "COUNCIL_GRAPH_CONTEXT_FILE")
		}
		if match {
			value, count = content, count+1
		}
	}
	return value, count
}

func graphFinalCheckUserState(t *testing.T, f graphFinalCheckCase) {
	t.Helper()
	if body, err := os.ReadFile(filepath.Join(f.repo, "source.txt")); err != nil || !bytes.Equal(body, f.working) {
		t.Fatal("Cumulative check changed user working bytes", err)
	}
	if body, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(body, f.index) {
		t.Fatal("Cumulative check changed the exact user index", err)
	}
}

func TestGraphFinalContextActualCommandCheckReadsDescriptorWithoutChangingCapturedCommand(t *testing.T) {
	f := graphFinalCommandAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	t.Setenv("COUNCIL_GRAPH_CONTEXT_FILE", "must-be-overridden")
	t.Setenv("COUNCIL_TEST_FINAL_CHECK_SENTINEL", "preserved")
	var spec process.NativeSpec
	prepares := 0
	timing := map[string]any{"attemptId": f.check.ID, "admittedAt": f.check.AdmittedAt, "deadlineAt": f.check.DeadlineAt, "commandTimeoutSeconds": f.definition.Command.TimeoutSeconds}
	prepareTimings := []map[string]any{}
	recordAttemptTiming := func(actual domain.Attempt) {
		observation := map[string]any{"phase": actual.Phase, "outcome": actual.Outcome, "startedAt": actual.StartedAt, "settledAt": actual.SettledAt, "evidenceStatus": actual.EvidenceStatus}
		if actual.PrimaryFailure != nil {
			observation["primaryFailureCode"] = actual.PrimaryFailure.Code
		}
		timing["actualAttempt"] = observation
	}
	defer func() {
		timing["failed"] = t.Failed()
		timing["prepareCalls"] = prepareTimings
		body, err := json.Marshal(timing)
		t.Logf("Actual final command boundary timing: %s; marshalErr=%v", body, err)
	}()
	var runEntered time.Time
	e.config.Prepare = func(ctx context.Context, actual process.NativeSpec) (*process.Owned, error) {
		entered := time.Now()
		callTiming := map[string]any{"enteredAt": entered.UTC().Format(time.RFC3339Nano), "elapsedFromRunNanoseconds": entered.Sub(runEntered).Nanoseconds()}
		prepareTimings = append(prepareTimings, callTiming)
		prepares++
		spec = actual
		spec.Args, spec.Stdin, spec.Env = append([]string{}, actual.Args...), append([]byte{}, actual.Stdin...), append([]string{}, actual.Env...)
		owned, err := process.Prepare(ctx, actual)
		returned := time.Now()
		callTiming["returnedAt"], callTiming["durationNanoseconds"], callTiming["returnedError"] = returned.UTC().Format(time.RFC3339Nano), returned.Sub(entered).Nanoseconds(), err != nil
		return owned, err
	}
	runEntered = time.Now()
	timing["runEnteredAt"] = runEntered.UTC().Format(time.RFC3339Nano)
	if admitted, err := time.Parse(time.RFC3339Nano, f.check.AdmittedAt); err == nil {
		timing["admissionToRunNanoseconds"] = runEntered.Sub(admitted).Nanoseconds()
	}
	if deadline, err := time.Parse(time.RFC3339Nano, f.check.DeadlineAt); err == nil {
		timing["deadlineRemainingAtRunNanoseconds"] = deadline.Sub(runEntered).Nanoseconds()
	}
	e.run(ctx, f.check)
	runReturned := time.Now()
	timing["runReturnedAt"], timing["runDurationNanoseconds"] = runReturned.UTC().Format(time.RFC3339Nano), runReturned.Sub(runEntered).Nanoseconds()
	path, reservedCount := graphFinalCheckDescriptor(spec)
	if prepares != 1 || reservedCount != 1 || !filepath.IsAbs(path) || filepath.Base(path) != "final-context.json" ||
		filepath.Base(filepath.Dir(path)) != "graph-context-"+f.check.ID || !reflect.DeepEqual(spec.Args, f.definition.Command.Arguments) || len(spec.Stdin) != 0 {
		diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer diagnosticCancel()
		actual, attemptErr := e.db.Attempt(diagnosticCtx, f.check.ID)
		if attemptErr == nil {
			recordAttemptTiming(actual)
		}
		workflowView, workflowErr := e.db.Workflow(diagnosticCtx, f.check.TaskID, domain.WorkflowSelector{WorkflowID: f.check.WorkflowID})
		step, stepErr := e.db.WorkflowStep(diagnosticCtx, f.check.StepID)
		gate, gateErr := e.db.Gate(diagnosticCtx)
		diagnostic, marshalErr := json.Marshal(map[string]any{"attempt": actual, "workflow": workflowView, "step": step, "gate": gate})
		t.Logf("Actual pre-Prepare failure state: %s; attemptErr=%v workflowErr=%v stepErr=%v gateErr=%v marshalErr=%v", diagnostic, attemptErr, workflowErr, stepErr, gateErr, marshalErr)
		t.Fatal("The actual admitted local check received no exact final descriptor or changed its captured command", prepares, reservedCount)
	}
	current, err := e.db.Attempt(ctx, f.check.ID)
	if err == nil {
		recordAttemptTiming(current)
	}
	if err != nil || current.Phase != "settled" || current.Outcome == nil || *current.Outcome != "succeeded" || current.StartedAt == nil || current.Cleanup.State != "verified" || current.EvidenceStatus != "complete" {
		t.Fatal("The actual final command check did not complete its owned execution", current, err)
	}
	checks, err := e.db.WorkflowChecks(ctx, f.check.TaskID, domain.WorkflowSelector{WorkflowID: f.check.WorkflowID}, 100, "")
	if err != nil || len(checks.Items) != 1 || checks.Items[0].Outcome != "passed" || checks.Items[0].StdoutArtifactID == nil || checks.Items[0].ObservedExecutable == nil ||
		!reflect.DeepEqual(*checks.Items[0].ObservedExecutable, *f.definition.Command) || checks.Items[0].SourceChanged {
		t.Fatal("The captured command observation was substituted or invented", checks, err)
	}
	receipt := checks.Items[0]
	meta, err := e.evidence.VerifyClosed(ctx, *receipt.StdoutArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := e.readAllResponse(meta)
	if err != nil {
		t.Fatal(err)
	}
	var observed struct {
		Reads         []fixture.GraphContextInspection `json:"fixture_final_context_reads"`
		DescriptorSHA string                           `json:"descriptorSha256"`
		Sentinel      bool                             `json:"inheritedSentinelObserved"`
	}
	if json.Unmarshal(stdout, &observed, json.RejectUnknownMembers(true)) != nil || !observed.Sentinel {
		t.Fatal("The fixed command did not actually read the inherited context transport")
	}
	descriptorBytes, err := os.ReadFile(path)
	if err != nil || observed.DescriptorSHA != workflowHash(descriptorBytes) {
		t.Fatal("The command did not read the exact retained private descriptor", err)
	}
	var descriptor workspace.GraphFinalContextDescriptor
	if err = json.Unmarshal(descriptorBytes, &descriptor, json.RejectUnknownMembers(true)); err != nil || len(observed.Reads) != len(descriptor.Access) {
		t.Fatal("The actual final command omitted required complete file accesses", err)
	}
	seen := map[string]fixture.GraphContextInspection{}
	for _, read := range observed.Reads {
		if _, exists := seen[read.Ref]; exists {
			t.Fatal("Duplicate actual command context inspection", read.Ref)
		}
		seen[read.Ref] = read
	}
	for _, source := range descriptor.Access {
		read, ok := seen[source.Ref]
		if !ok || read.ArtifactID != source.ArtifactRef.ArtifactID || read.SHA256 != source.ArtifactRef.SHA256 || read.ByteLength != source.ArtifactRef.ByteLength || read.Mode != "file" || read.ReadVia != "file" {
			t.Fatal("The command did not read the exact complete original/final source", source.Ref)
		}
	}
	scope := domain.CheckScope{Kind: receipt.Applicability.ScopeKind, Paths: receipt.Applicability.Paths}
	basis, err := e.workflowApplicability(ctx, f.subject, scope, receipt.Applicability.ConditionKeys, receipt.DefinitionSHA256, f.verified)
	if err != nil || !workflowpolicy.Applicable(receipt.Applicability, basis) {
		t.Fatal("A local check did not qualify the exact cumulative final basis", receipt.Applicability, basis, err)
	}
	nodeOnly, err := e.workflowApplicability(ctx, f.subject, scope, receipt.Applicability.ConditionKeys, receipt.DefinitionSHA256)
	if err != nil || workflowpolicy.Applicable(nodeOnly, basis) {
		t.Fatal("A node-only check receipt acquired cumulative qualification", nodeOnly, basis, err)
	}
	budget, err := e.db.GraphBudget(ctx, f.dispatch.GraphID)
	if err != nil || !reflect.DeepEqual(budget.Budget, f.budget.Budget) {
		t.Fatal("A local cumulative check consumed or granted model allowance", budget, err)
	}
	graphFinalCheckUserState(t, f)
}

func TestGraphFinalContextUnknownLocalCopyAcknowledgementRetainsClaimAfterNoProcessCleanup(t *testing.T) {
	f := graphFinalCommandAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	t.Setenv("COUNCIL_TEST_FINAL_CHECK_SENTINEL", "preserved")
	sink := &graphNativeRunLostContextEffectsSink{artifactSink: artifactSink{db: e.db}, root: e.lock.Root()}
	artifacts, err := evidence.New(e.lock, sink)
	if err != nil {
		t.Fatal(err)
	}
	managed, err := workspace.New(e.lock, artifacts)
	if err != nil {
		artifacts.Close()
		t.Fatal(err)
	}
	priorEvidence, priorWorkspace := e.evidence, e.workspace
	e.evidence, e.workspace = artifacts, managed
	t.Cleanup(func() {
		e.evidence, e.workspace = priorEvidence, priorWorkspace
		if err := managed.Close(); err != nil {
			t.Error(err)
		}
		if err := artifacts.Close(); err != nil {
			t.Error(err)
		}
	})
	prepares := 0
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		prepares++
		return process.Prepare(ctx, spec)
	}
	e.run(ctx, f.check)
	current, err := e.db.Attempt(ctx, f.check.ID)
	if err != nil || !sink.lost || prepares != 0 || current.Phase != "settled" || current.Outcome == nil || *current.Outcome == "succeeded" ||
		current.StartedAt != nil || current.LaunchIntent || current.Cleanup.State != "verified" || current.EvidenceStatus != "uncertain" {
		t.Fatal("Actual local context acknowledgement uncertainty was hidden or a command began", current, sink.lost, prepares, err)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State == "open" || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != f.check.ID ||
		gate.ActiveClaims[0].GraphID == nil || *gate.ActiveClaims[0].GraphID != f.dispatch.GraphID || gate.ActiveClaims[0].NodeID == nil || *gate.ActiveClaims[0].NodeID != f.dispatch.NodeID {
		t.Fatal("Verified no-process cleanup released the exact unknown local context claim", gate, err)
	}
	record, err := e.db.GraphDispatchForAttempt(ctx, f.check.ID)
	if err != nil || record.Public.OutputObservation != nil && (record.Public.OutputObservation.Candidate != nil && record.Public.OutputObservation.Candidate.State == "complete" ||
		record.Public.OutputObservation.Response != nil && record.Public.OutputObservation.Response.State == "complete") {
		t.Fatal("Unknown local context effects qualified a graph output", record, err)
	}
	graphFinalCheckUserState(t, f)
}

func TestGraphFinalContextLocalDescriptorTamperAfterPreparePreventsCommandGO(t *testing.T) {
	f := graphFinalCommandAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	t.Setenv("COUNCIL_TEST_FINAL_CHECK_SENTINEL", "preserved")
	prepares, tampered := 0, false
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		prepares++
		owned, err := process.Prepare(ctx, spec)
		if err != nil {
			return nil, err
		}
		failPrepared := func(cause error) (*process.Owned, error) {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), process.CleanupLimit)
			defer cancel()
			clean := owned.Cleanup(cleanupCtx)
			closeErr := owned.Close()
			if !clean.Verified || closeErr != nil {
				return nil, errors.Join(cause, closeErr, process.ErrOwnershipUncertain)
			}
			return nil, cause
		}
		path, count := graphFinalCheckDescriptor(spec)
		if count == 1 {
			body, readErr := os.ReadFile(path)
			if readErr != nil || len(body) == 0 {
				return failPrepared(errors.Join(domain.Fail("fixture_precondition", "The controlled exact descriptor prerequisite is unavailable."), readErr))
			}
			body[0] ^= 1
			if writeErr := os.WriteFile(path, body, 0600); writeErr != nil {
				return failPrepared(writeErr)
			}
			tampered = true
		}
		return owned, nil
	}
	e.run(ctx, f.check)
	current, err := e.db.Attempt(ctx, f.check.ID)
	if err != nil || prepares != 1 || !tampered || current.Phase != "settled" || current.StartedAt != nil || current.Outcome == nil || *current.Outcome == "succeeded" || current.Cleanup.State != "verified" {
		t.Fatal("The exact final descriptor was not rechecked between owned Prepare and GO", current, prepares, tampered, err)
	}
	checks, err := e.db.WorkflowChecks(ctx, f.check.TaskID, domain.WorkflowSelector{WorkflowID: f.check.WorkflowID}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range checks.Items {
		if check.Outcome == "passed" {
			t.Fatal("A changed private descriptor produced a qualifying check receipt", check)
		}
		if check.StartedAt != nil {
			t.Fatal("A command rejected before GO acquired a receipt start timestamp", check.StartedAt)
		}
	}
	graphFinalCheckUserState(t, f)
}

func TestGraphFinalContextFailedCommandExplicitAdvanceAdmitsCorrectionWithoutReplay(t *testing.T) {
	f := graphFinalCommandAttemptForOutcomeFixture(t, "fail")
	e, ctx := f.engine, context.Background()
	t.Setenv("COUNCIL_TEST_FINAL_CHECK_SENTINEL", "preserved")
	e.run(ctx, f.check)
	failed, err := e.db.Attempt(ctx, f.check.ID)
	if err != nil || failed.Phase != "settled" || failed.Outcome == nil || *failed.Outcome != "failed" || failed.StartedAt == nil || failed.Cleanup.State != "verified" || failed.EvidenceStatus != "complete" {
		t.Fatal("The actual fixed command did not reach a healthy observed failure", failed, err)
	}
	receipts, err := e.db.WorkflowChecks(ctx, f.check.TaskID, domain.WorkflowSelector{WorkflowID: f.check.WorkflowID}, 100, "")
	if err != nil || len(receipts.Items) != 1 || receipts.Items[0].Outcome != "failed" || receipts.Items[0].ExitCode == nil || *receipts.Items[0].ExitCode != 1 ||
		!receipts.Items[0].RootTerminal || !receipts.Items[0].InputComplete || !receipts.Items[0].OutputComplete || receipts.Items[0].Cleanup.State != "verified" || receipts.Items[0].SourceChanged {
		t.Fatal("The failed command is not an exact complete corrective-work prerequisite", receipts, err)
	}
	receipt := receipts.Items[0]
	workflow, err := e.db.Workflow(ctx, f.check.TaskID, domain.WorkflowSelector{WorkflowID: f.check.WorkflowID})
	if err != nil || workflow.Workflow == nil {
		t.Fatal(workflow, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, f.check.TaskID, *workflow.Workflow)
	if err != nil || progress.decision.Role != "correction" || progress.final == nil || progress.final.BindingRef != f.verified.BindingRef {
		t.Fatal("The actual failed check is not a correction decision on the same retained cumulative final", progress.decision, err)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || len(gate.ActiveClaims) != 0 {
		t.Fatal("The known failed command did not release its own healthy execution scope", gate, err)
	}
	rootBefore, err := e.db.GraphRoot(ctx, f.dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := e.db.Graph(ctx, f.dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	grant := domain.GraphBudgetAmendmentInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: before.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: before.Graph.BudgetAuthorityEpoch,
		ExpectedWindowNumber: before.Budget.Budget.CurrentWindow.Number, AdditionalAdmissions: "1"}
	body, err := json.Marshal(grant)
	if err != nil {
		t.Fatal(err)
	}
	if result := e.MutateGraph(ctx, "graphs.budget.amend", before.Graph.ID, body); result.Status != 200 || result.Envelope.Error != nil {
		t.Fatal("The real correction allowance prerequisite failed", result)
	}
	// Allowance cannot turn a historical failed check into a fresh trigger.
	e.continueWorkflow(f.check.ID)
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 0 {
		t.Fatal("The grant replayed historical completion or admitted corrective work without explicit intent", pending, err)
	}
	view, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}}
	for _, node := range view.Nodes {
		in.NodeAuthorities = append(in.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	body, err = json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	result := e.MutateGraph(ctx, "graphs.advance", view.Graph.ID, body)
	var admitted domain.GraphAdvanceResult
	wire, _ := json.Marshal(result.Envelope.Data)
	decodeErr := json.Unmarshal(wire, &admitted, json.RejectUnknownMembers(true))
	if result.Status != 200 || result.Envelope.Error != nil || decodeErr != nil || len(admitted.Dispatches) != 1 || len(admitted.PreparationAttemptIDs) != 0 || len(admitted.ReboundNodes) != 0 {
		t.Fatal("Fresh explicit Advance did not admit the healthy failed check's exact correction", result, admitted, decodeErr)
	}
	d := admitted.Dispatches[0].Dispatch
	if d.Role != "correction" || d.ExecutorKind != "native" || d.TaskID != f.check.TaskID || d.TaskVersion != f.check.TaskVersion || d.WorkflowID != f.check.WorkflowID ||
		d.NodeID != f.dispatch.NodeID || d.NodeAdmissionID != f.dispatch.NodeAdmissionID || d.InputID != f.dispatch.InputID || d.BudgetOwnerTaskID != f.verified.Root.RootTaskID ||
		d.SourceBasis.BasisKind != "node_subject" || d.SourceBasis.CandidateBinding == nil || *d.SourceBasis.CandidateBinding != *f.subject.Binding ||
		d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != f.verified.BindingRef || d.FreshTriggerID != in.RequestID {
		t.Fatal("Explicit correction substituted or replayed an older logical execution/source/final authority", d)
	}
	correction, err := e.db.ClaimAttempt(ctx, d.AttemptID, e.config.OwnerInstanceID)
	if err != nil || correction.PreviousAttemptID == nil || *correction.PreviousAttemptID != f.check.ID || correction.ResolvedTarget == nil ||
		correction.RoutingDecisionID == nil || correction.ConnectionID != correction.ResolvedTarget.ConnectionID || correction.ResolvedTarget.HarnessID != "codex" {
		t.Fatal("The real corrective native admission lost its captured target/account routing or failed predecessor", correction, err)
	}
	t.Cleanup(func() {
		// Admission is the tested boundary. The correction itself is not run or
		// reported successful; its actual unlaunched claim is settled honestly.
		current, err := e.db.Attempt(context.Background(), correction.ID)
		if err != nil || current.Phase == "settled" {
			if err != nil {
				t.Error(err)
			}
			return
		}
		_, err = e.db.SettleWorkflowStep(context.Background(), correction.ID, domain.WorkflowStepFacts{StepID: correction.StepID, OwnerInstanceID: e.config.OwnerInstanceID,
			Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: true, EvidenceStatus: "incomplete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"},
				Failure: primary("fixture_no_process", "The explicit correction fixture admits but does not execute its native worker.")}})
		if err != nil {
			t.Error(err)
		}
	})
	retained, err := e.db.Attempt(ctx, f.check.ID)
	if err != nil {
		t.Fatal(err)
	}
	// These two fields are read-time projections; all persisted failure,
	// terminal, cleanup, clock and identity facts must remain exactly unchanged.
	retained.ObservedAt, retained.RemainingSeconds = failed.ObservedAt, failed.RemainingSeconds
	if !reflect.DeepEqual(retained, failed) {
		t.Fatal("Explicit correction rewrote the actual failed command history", retained, failed)
	}
	again, err := e.db.WorkflowChecks(ctx, f.check.TaskID, domain.WorkflowSelector{WorkflowID: f.check.WorkflowID}, 100, "")
	if err != nil || len(again.Items) != 1 || !reflect.DeepEqual(again.Items[0], receipt) {
		t.Fatal("Explicit correction rewrote the actual failed check receipt", again, err)
	}
	rootAfter, err := e.db.GraphRoot(ctx, f.dispatch.GraphID)
	if err != nil || !reflect.DeepEqual(rootBefore, rootAfter) {
		t.Fatal("Corrective admission recaptured or changed the immutable graph root", err)
	}
	units, invalid := domain.SubtractRoutingCounts(admitted.Budget.Budget.ConsumedAdmissions, view.Budget.Budget.ConsumedAdmissions)
	if invalid != nil || units != "1" || admitted.Budget.Budget.GrantedAdmissions != view.Budget.Budget.GrantedAdmissions ||
		!reflect.DeepEqual(admitted.Budget.Budget.CurrentWindow, view.Budget.Budget.CurrentWindow) || admitted.Budget.BudgetAuthorityEpoch != view.Budget.BudgetAuthorityEpoch {
		t.Fatal("The correction used child allowance, refilled units, or reset the existing shared window", admitted.Budget, invalid)
	}
	graphFinalCheckUserState(t, f)
}
