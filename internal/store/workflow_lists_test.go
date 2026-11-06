package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"testing"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Direct SQL below creates labelled immutable DAO records solely to exercise
// read ordering/ownership/enrichment. It supplies no actual filesystem, review,
// process, application or provider qualification proof and invokes no worker.
func workflowListAdmission(t *testing.T, s *Store) domain.Admission {
	t.Helper()
	owner, in, facts := workflowAdmissionFixture(t, s)
	result := s.CreateTaskWithWorkflow(context.Background(), owner, in, facts)
	if result.Status != 201 {
		t.Fatal("controlled list admission prerequisite", result)
	}
	return result.Envelope.Data.(domain.Admission)
}
func workflowListExec(t *testing.T, s *Store, query string, args ...any) {
	t.Helper()
	if _, e := s.db.Exec(query, args...); e != nil {
		t.Fatalf("controlled DAO row failed: %v", e)
	}
}
func workflowListStep(t *testing.T, s *Store, a domain.Admission, sequence int64, role, executor string) domain.WorkflowStep {
	t.Helper()
	step := domain.WorkflowStep{ID: domain.NewID(), WorkflowID: a.Workflow.ID, Ordinal: domain.Decimal(sequence), ExecutionOrdinal: "1", Role: role, ExecutorKind: executor, LogicalKey: role + ":" + domain.Decimal(sequence), Status: "succeeded", CreatedAt: domain.Now(), ReasonCodes: []string{"controlled_dao_metadata_only"}}
	attempt := a.Attempt
	attempt.ID = domain.NewID()
	attempt.StepID = step.ID
	attempt.Role = role
	attempt.ExecutorKind = executor
	attempt.Phase = "settled"
	attempt.SettledAt = ptr(domain.Now())
	attempt.Outcome = ptr("succeeded")
	step.AttemptID = ptr(attempt.ID)
	workflowListExec(t, s, "INSERT INTO attempts(id,task_id,task_version,revision,phase,data) VALUES(?,?,1,1,'settled',?)", attempt.ID, a.Task.ID, encode(attempt))
	workflowListExec(t, s, "INSERT INTO workflow_steps(sequence,id,workflow_id,ordinal,execution_ordinal,logical_key,role,executor_kind,cycle,status,attempt_id,data) VALUES(?,?,?,?,1,?,?,?,0,'succeeded',?,?)", sequence, step.ID, a.Workflow.ID, sequence, step.LogicalKey, role, executor, attempt.ID, encode(step))
	return step
}
func workflowListArtifact(t *testing.T, s *Store, attemptID string) string {
	t.Helper()
	id := domain.NewID()
	hash := strings.Repeat("a", 64)
	meta := domain.ArtifactMetadata{ID: id, AttemptID: attemptID, Kind: "final_response", ByteLength: "0", Closed: true, Completeness: "complete", SHA256: &hash, CreatedAt: domain.Now()}
	workflowListExec(t, s, "INSERT INTO artifacts(id,attempt_id,private_ref,physical_identity,byte_length,data) VALUES(?,?,?,?,0,?)", id, attemptID, "artifacts/"+id+".bin", "controlled-dao-no-os-proof", encode(meta))
	return id
}
func workflowListReview(t *testing.T, s *Store, a domain.Admission, sequence int64, findings int, summary string) domain.ReviewRecord {
	t.Helper()
	step := workflowListStep(t, s, a, sequence+1000, "review", "native")
	subject := workflowListArtifact(t, s, a.Attempt.ID)
	result := workflowListArtifact(t, s, *step.AttemptID)
	r := domain.ReviewRecord{ID: domain.NewID(), WorkflowID: a.Workflow.ID, StepID: step.ID, AttemptID: *step.AttemptID, ReviewContract: domain.ReviewContract, ResultArtifactID: result, ResultSHA256: strings.Repeat("a", 64), ResponseBinding: &domain.ResponseBinding{ProducerAttemptID: a.Attempt.ID, ResponseArtifactID: subject, ResponseSHA256: strings.Repeat("a", 64)}, Outcome: "blocking", CreatedAt: domain.Now(), Findings: []domain.WorkflowFinding{}}
	for i := 0; i < findings; i++ {
		f := domain.ReviewFinding{Key: fmt.Sprintf("controlled-%d", i), Severity: "blocking", Summary: summary, ConditionKeys: []string{"task-instructions"}, Locations: []domain.ReviewLocation{}, EvidenceRefs: []string{"controlled-response"}}
		r.Findings = append(r.Findings, domain.WorkflowFinding{ID: domain.NewID(), ModelKey: f.Key, ReviewID: r.ID, Finding: f})
		r.Report.Findings = append(r.Report.Findings, f)
	}
	workflowListExec(t, s, "INSERT INTO workflow_reviews(sequence,id,workflow_id,step_id,attempt_id,response_artifact_id,result_artifact_id,data) VALUES(?,?,?,?,?,?,?,?)", sequence, r.ID, a.Workflow.ID, r.StepID, r.AttemptID, subject, result, encode(r))
	return r
}
func workflowListErrorCode(e error) string {
	if typed, ok := e.(*domain.Error); ok {
		return typed.Code
	}
	return ""
}

func TestWorkflowListsStepsCursorExactHighwaterAndHistoricalBinding(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a := workflowListAdmission(t, s)
	base := int64(9007199254740993)
	steps := []domain.WorkflowStep{workflowListStep(t, s, a, base, "review", "native"), workflowListStep(t, s, a, base+1, "check", "local_check"), workflowListStep(t, s, a, base+2, "correction", "native")}
	first, e := s.workflowStepsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, "")
	if e != nil || len(first.Items) != 1 || first.Items[0].ID != a.Step.ID || first.NextCursor == nil {
		t.Fatal("workflow step paging unavailable", first, e)
	}
	late := workflowListStep(t, s, a, base+3, "review", "native")
	ids := []string{}
	cursor := *first.NextCursor
	for i := 0; i < 5; i++ {
		page, e := s.workflowStepsList(ctx, a.Task.ID, domain.WorkflowSelector{TaskVersion: "1"}, 1, cursor)
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range page.Items {
			ids = append(ids, item.ID)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	want := []string{steps[0].ID, steps[1].ID, steps[2].ID}
	if !reflect.DeepEqual(ids, want) {
		t.Fatal("exact highwater skipped/duplicated or admitted a late row", ids, want, late.ID)
	}
	if _, e = s.workflowReviewsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, *first.NextCursor); workflowListErrorCode(e) != "invalid_cursor" {
		t.Fatal("cross-resource cursor accepted", e)
	}
	other := workflowListAdmission(t, s)
	if _, e = s.workflowStepsList(ctx, other.Task.ID, domain.WorkflowSelector{}, 1, *first.NextCursor); workflowListErrorCode(e) != "invalid_cursor" {
		t.Fatal("foreign-task cursor accepted", e)
	}
	if _, e = s.workflowStepsList(ctx, other.Task.ID, domain.WorkflowSelector{WorkflowID: a.Workflow.ID}, 1, ""); e == nil {
		t.Fatal("foreign workflow selector accepted")
	}
	version := a.Version
	version.Version = "2"
	workflowListExec(t, s, "INSERT INTO task_versions(task_id,version,data) VALUES(?,2,?)", a.Task.ID, encode(version))
	task, e := s.task(ctx, s.db, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	task.CurrentVersion = "2"
	workflowListExec(t, s, "UPDATE tasks SET current_version=2,data=? WHERE id=?", encode(task), task.ID)
	if _, e = s.workflowStepsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, *first.NextCursor); workflowListErrorCode(e) != "invalid_cursor" {
		t.Fatal("cursor silently rebound to newer current version", e)
	}
	historical, e := s.workflowStepsList(ctx, a.Task.ID, domain.WorkflowSelector{WorkflowID: a.Workflow.ID}, 100, "")
	if e != nil || len(historical.Items) != 5 {
		t.Fatal("old workflow history lost after guidance", historical, e)
	}
}

func TestWorkflowListsNestedFindingsFullResolutionSnapshotAndNoStoredRewrite(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a := workflowListAdmission(t, s)
	review := workflowListReview(t, s, a, 1, 3, "CONTROLLED DAO finding; no model inspection is claimed")
	var original string
	if e := s.db.QueryRow("SELECT data FROM workflow_reviews WHERE id=?", review.ID).Scan(&original); e != nil {
		t.Fatal(e)
	}
	correction := workflowListStep(t, s, a, 2000, "correction", "native")
	subject := review.ResponseBinding.ResponseArtifactID
	for i := 0; i < 130; i++ {
		hash := fmt.Sprintf("%064x", i+1)
		r := domain.FindingResolution{ID: domain.NewID(), WorkflowID: a.Workflow.ID, FindingID: review.Findings[0].ID, OriginReviewID: ptr(review.ID), CorrectedResponseBinding: &domain.ResponseBinding{ProducerAttemptID: *correction.AttemptID, ResponseArtifactID: subject, ResponseSHA256: hash}, CorrectionStepID: correction.ID, CreatedAt: domain.Now()}
		workflowListExec(t, s, "INSERT INTO workflow_resolutions(sequence,id,workflow_id,finding_id,origin_review_id,corrected_response_artifact_id,corrected_subject_sha256,data) VALUES(?,?,?,?,?,?,?,?)", i+1, r.ID, a.Workflow.ID, r.FindingID, review.ID, subject, hash, encode(r))
	}
	page, e := s.workflowFindingsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, "")
	if e != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal("nested finding read unavailable", page, e)
	}
	if page.Items[0].ID != review.Findings[0].ID || len(page.Items[0].Resolutions) != 130 {
		t.Fatal("resolution enrichment silently clipped retained history", page.Items[0])
	}
	late := workflowListReview(t, s, a, 2, 1, "CONTROLLED late source row")
	lateResolution := domain.FindingResolution{ID: domain.NewID(), WorkflowID: a.Workflow.ID, FindingID: review.Findings[1].ID, OriginReviewID: ptr(review.ID), CorrectedResponseBinding: &domain.ResponseBinding{ProducerAttemptID: *correction.AttemptID, ResponseArtifactID: subject, ResponseSHA256: strings.Repeat("f", 64)}, CorrectionStepID: correction.ID, CreatedAt: domain.Now()}
	workflowListExec(t, s, "INSERT INTO workflow_resolutions(sequence,id,workflow_id,finding_id,origin_review_id,corrected_response_artifact_id,corrected_subject_sha256,data) VALUES(131,?,?,?,?,?,?,?)", lateResolution.ID, a.Workflow.ID, lateResolution.FindingID, review.ID, subject, strings.Repeat("f", 64), encode(lateResolution))
	ids := []string{page.Items[0].ID}
	cursor := page.NextCursor
	for cursor != nil {
		next, e := s.workflowFindingsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, *cursor)
		if e != nil {
			t.Fatal(e)
		}
		for _, f := range next.Items {
			ids = append(ids, f.ID)
			if f.ID == review.Findings[1].ID && len(f.Resolutions) != 0 {
				t.Fatal("old cursor crossed captured resolution highwater", f)
			}
		}
		cursor = next.NextCursor
	}
	if !reflect.DeepEqual(ids, []string{review.Findings[0].ID, review.Findings[1].ID, review.Findings[2].ID}) {
		t.Fatal("nested row cursor skipped findings or crossed source highwater", ids, late.ID)
	}
	fresh, e := s.workflowFindingsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 100, "")
	if e != nil || len(fresh.Items) != 4 || len(fresh.Items[1].Resolutions) != 1 || fresh.Items[1].Resolutions[0].ID != lateResolution.ID {
		t.Fatal("new first page lost retained resolution/source enrichment", fresh, e)
	}
	var after string
	s.db.QueryRow("SELECT data FROM workflow_reviews WHERE id=?", review.ID).Scan(&after)
	if after != original {
		t.Fatal("reading findings rewrote immutable origin record")
	}
}

func TestWorkflowListsWholeItemByteCutAndTypedReads(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a := workflowListAdmission(t, s)
	for i := int64(1); i <= 40; i++ {
		workflowListReview(t, s, a, i, 40, strings.Repeat("controlled metadata ", 140))
	}
	page, e := s.workflowReviewsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 100, "")
	if e != nil || len(page.Items) == 0 || len(page.Items) >= 40 || page.NextCursor == nil {
		t.Fatal("whole-item 4MiB page cut unavailable", len(page.Items), page.NextCursor, e)
	}
	seen := len(page.Items)
	for _, r := range page.Items {
		if len(r.Findings) != 40 || len(r.Findings[0].Finding.Summary) != len(strings.Repeat("controlled metadata ", 140)) {
			t.Fatal("byte budget clipped a retained review/finding")
		}
	}
	cursor := page.NextCursor
	for cursor != nil {
		next, e := s.workflowReviewsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 100, *cursor)
		if e != nil {
			t.Fatal(e)
		}
		seen += len(next.Items)
		cursor = next.NextCursor
	}
	if seen != 40 {
		t.Fatal("byte cut skipped or duplicated rows", seen)
	}
	checks, e := s.workflowChecksList(ctx, a.Task.ID, domain.WorkflowSelector{}, 30, "")
	if e != nil || checks.Items == nil || len(checks.Items) != 0 {
		t.Fatal("typed empty checks unavailable", checks, e)
	}
	candidates, e := s.workflowCandidatesList(ctx, a.Task.ID, domain.WorkflowSelector{}, 30, "")
	if e != nil || candidates.Items == nil || len(candidates.Items) != 0 {
		t.Fatal("typed empty candidates unavailable", candidates, e)
	}
	deliveries, e := s.workflowDeliveriesList(ctx, a.Task.ID, domain.WorkflowSelector{}, 30, "")
	if e != nil || deliveries.Items == nil || len(deliveries.Items) != 0 {
		t.Fatal("typed empty deliveries unavailable", deliveries, e)
	}
	for _, limit := range []int{-1, 101} {
		if _, e = s.workflowStepsList(ctx, a.Task.ID, domain.WorkflowSelector{}, limit, ""); e == nil {
			t.Fatal("invalid explicit limit accepted", limit)
		}
	}
	if _, e = s.workflowStepsList(ctx, a.Task.ID, domain.WorkflowSelector{WorkflowID: a.Workflow.ID, TaskVersion: "1"}, 30, ""); e == nil {
		t.Fatal("ambiguous selector accepted")
	}
	var foreignKeys int
	if e = s.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); e != nil || foreignKeys != 1 {
		t.Fatal("controlled fixture bypassed relational checks", foreignKeys, e)
	}
	var attempts int
	if e = s.db.QueryRow("SELECT count(*) FROM attempts WHERE task_id=?", a.Task.ID).Scan(&attempts); e != nil || attempts != 41 {
		t.Fatal("observation started work or fixture changed", attempts, e)
	}
}

func workflowListCloneCandidate(t *testing.T, s *Store, original domain.Candidate, step domain.WorkflowStep) domain.Candidate {
	t.Helper()
	candidate := original
	candidate.ID, candidate.AttemptID, candidate.CreatedByStepID = domain.NewID(), *step.AttemptID, step.ID
	candidate.CreatedAt = domain.Now()
	// Controlled read metadata reuses the immutable root references; this is not
	// a qualified managed handoff or evidence of actual candidate file contents.
	workflowListExec(t, s, "INSERT INTO candidates(id,attempt_id,source_snapshot_id,manifest_artifact_id,delta_artifact_id,workspace_ref,data) VALUES(?,?,?,?,?,?,?)", candidate.ID, candidate.AttemptID, candidate.SourceSnapshotID, candidate.ManifestArtifactID, candidate.DeltaArtifactID, candidate.WorkspaceRef, encode(candidate))
	return candidate
}

func TestWorkflowListsCheckBlockerAndTypedDeliverySources(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a, _, facts := workflowAuthorSettlementFixture(t, s)
	checkStep := workflowListStep(t, s, a, 3000, "check", "local_check")
	check := domain.CheckReceipt{ID: domain.NewID(), WorkflowID: a.Workflow.ID, StepID: checkStep.ID, AttemptID: *checkStep.AttemptID, DefinitionID: a.Workflow.Authority.Checks[0].ID, InputCandidateBinding: facts.CandidateBinding, Outcome: "failed", BuiltinCompleted: true, CreatedAt: domain.Now()}
	finding := domain.WorkflowFinding{ID: domain.NewID(), ModelKey: "controlled-check-blocker", CheckID: check.ID, Finding: domain.ReviewFinding{Key: "controlled-check-blocker", Severity: "blocking", Summary: "CONTROLLED DAO failed required check metadata; no process/OS qualification", ConditionKeys: []string{"task-instructions"}}}
	check.Finding = &finding
	workflowListExec(t, s, "INSERT INTO workflow_checks(sequence,id,workflow_id,step_id,attempt_id,definition_id,candidate_id,data) VALUES(1,?,?,?,?,?,?,?)", check.ID, a.Workflow.ID, check.StepID, check.AttemptID, check.DefinitionID, facts.CandidateBinding.CandidateID, encode(check))
	checks, e := s.workflowChecksList(ctx, a.Task.ID, domain.WorkflowSelector{}, 30, "")
	if e != nil || len(checks.Items) != 1 || checks.Items[0].Finding == nil || checks.Items[0].Finding.ID != finding.ID {
		t.Fatal("failed-check blocker not returned with typed receipt", checks, e)
	}
	findings, e := s.workflowFindingsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, "")
	if e != nil || len(findings.Items) != 1 || findings.Items[0].ID != finding.ID || findings.Items[0].CheckID != check.ID {
		t.Fatal("check-origin finding was omitted or confused with review origin", findings, e)
	}

	local := workflowListStep(t, s, a, 4000, "local_delivery", "local_application")
	op := domain.DeliveryOperation{ID: domain.NewID(), WorkflowID: a.Workflow.ID, StepID: local.ID, AttemptID: *local.AttemptID, RequestID: domain.NewID(), CandidateBinding: *facts.CandidateBinding, Scope: a.Workflow.Authority.Delivery, Status: "conflict", CreatedAt: domain.Now(), LastEffectSequence: "0", ConflictCount: "1", NextAction: "inspect_controlled_metadata_gap"}
	workflowListExec(t, s, "INSERT INTO delivery_operations(sequence,id,workflow_id,step_id,attempt_id,candidate_id,revision,status,data) VALUES(9007199254740993,?,?,?,?,?,1,'conflict',?)", op.ID, a.Workflow.ID, op.StepID, op.AttemptID, facts.CandidateBinding.CandidateID, encode(op))
	external := workflowListStep(t, s, a, 5000, "external_delivery", "local_external")
	external.ExternalResult = &domain.ExternalDeliveryRecord{WorkflowID: a.Workflow.ID, StepID: external.ID, AttemptID: *external.AttemptID, Action: domain.ExternalActionInput{Action: "controlled DAO metadata only", Destination: "controlled-no-execution", AcceptanceBasis: "command_completion"}, Outcome: "command_failed", CreatedAt: domain.Now()}
	workflowListExec(t, s, "UPDATE workflow_steps SET data=? WHERE id=?", encode(external), external.ID)
	acceptance := domain.WorkflowAcceptance{ID: domain.NewID(), WorkflowID: a.Workflow.ID, TaskID: a.Task.ID, TaskVersion: a.Version.Version, CandidateBinding: *facts.CandidateBinding, DeliveryResult: "review_only", AcceptedAt: domain.Now(), AcceptedByRequestID: domain.NewID()}
	workflowListExec(t, s, "INSERT INTO workflow_acceptances(sequence,id,workflow_id,task_id,task_version,candidate_id,data) VALUES(1,?,?,?,1,?,?)", acceptance.ID, a.Workflow.ID, a.Task.ID, facts.CandidateBinding.CandidateID, encode(acceptance))
	first, e := s.workflowDeliveriesList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, "")
	if e != nil || len(first.Items) != 1 || first.NextCursor == nil || first.Items[0].Kind != "local_application" || first.Items[0].Operation == nil || first.Items[0].Operation.Status != "conflict" || first.Items[0].External != nil {
		t.Fatal("typed local operation branch missing or manufactured success", first, e)
	}
	late := workflowListStep(t, s, a, 6000, "external_delivery", "local_external")
	late.ExternalResult = &domain.ExternalDeliveryRecord{WorkflowID: a.Workflow.ID, StepID: late.ID, AttemptID: *late.AttemptID, Outcome: "uncertain", CreatedAt: domain.Now()}
	workflowListExec(t, s, "UPDATE workflow_steps SET data=? WHERE id=?", encode(late), late.ID)
	kinds := []string{first.Items[0].Kind}
	cursor := first.NextCursor
	for cursor != nil {
		page, e := s.workflowDeliveriesList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, *cursor)
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range page.Items {
			kinds = append(kinds, item.Kind)
			if item.Kind == "external_command" && (item.External == nil || item.External.StepID != external.ID || item.External.Outcome != "command_failed" || item.Operation != nil) {
				t.Fatal("external result alias or source highwater changed", item)
			}
			if item.Kind == "review_only" && (item.Acceptance == nil || item.Acceptance.ID != acceptance.ID || item.StepID != nil || item.AttemptID != nil || item.Operation != nil) {
				t.Fatal("review-only acceptance acquired an invented process or operation", item)
			}
		}
		cursor = page.NextCursor
	}
	if !reflect.DeepEqual(kinds, []string{"local_application", "external_command", "review_only"}) {
		t.Fatal("typed delivery source skipped, duplicated or admitted late result", kinds)
	}
}

func TestWorkflowListsCandidateCreationHighwaterExcludesLateResultOfExistingAttempt(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a, _, facts := workflowAuthorSettlementFixture(t, s)
	original, e := s.Candidate(ctx, facts.CandidateBinding.CandidateID)
	if e != nil {
		t.Fatal(e)
	}
	secondStep := workflowListStep(t, s, a, 2000, "correction", "native")
	lateStep := workflowListStep(t, s, a, 2001, "correction", "native")
	second := workflowListCloneCandidate(t, s, original, secondStep)
	first, e := s.workflowCandidatesList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, "")
	if e != nil || len(first.Items) != 1 || first.NextCursor == nil || first.Items[0].ID != original.ID {
		t.Fatal("candidate creation snapshot unavailable", first, e)
	}
	late := workflowListCloneCandidate(t, s, original, lateStep)
	page, e := s.workflowCandidatesList(ctx, a.Task.ID, domain.WorkflowSelector{TaskVersion: "1"}, 100, *first.NextCursor)
	if e != nil || len(page.Items) != 1 || page.Items[0].ID != second.ID || page.NextCursor != nil {
		t.Fatal("candidate page used attempt creation instead of candidate creation highwater", page, e, late.ID)
	}
	fresh, e := s.workflowCandidatesList(ctx, a.Task.ID, domain.WorkflowSelector{}, 100, "")
	if e != nil || len(fresh.Items) != 3 {
		t.Fatal("fresh candidate page lost retained late result", fresh, e)
	}
}

func TestWorkflowListsRejectMalformedCanonicalCursorWithoutWrites(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a := workflowListAdmission(t, s)
	workflowListStep(t, s, a, 2, "review", "native")
	page, e := s.workflowStepsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, "")
	if e != nil || page.NextCursor == nil {
		t.Fatal(page, e)
	}
	decoded, e := base64.RawURLEncoding.DecodeString(*page.NextCursor)
	if e != nil {
		t.Fatal(e)
	}
	var initial workflowListCursor
	if json.Unmarshal(decoded, &initial) != nil {
		t.Fatal("server cursor is not JSON")
	}
	mutations := []func(*workflowListCursor){
		func(c *workflowListCursor) { c.AfterSequence = "+1" },
		func(c *workflowListCursor) { c.AfterSequence = "01" },
		func(c *workflowListCursor) { c.AfterSequence = "-1" },
		func(c *workflowListCursor) { c.AfterSequence = "9223372036854775808" },
		func(c *workflowListCursor) { c.AfterSource = "3" },
		func(c *workflowListCursor) { c.AfterIndex = "64" },
		func(c *workflowListCursor) { c.SecondaryMaximum = "1" },
		func(c *workflowListCursor) { c.PrimaryMaximum = "0" },
		func(c *workflowListCursor) { c.Resource = "reviews" },
		func(c *workflowListCursor) { c.WorkflowID = domain.NewID() },
		func(c *workflowListCursor) { c.TaskVersion = "2" },
	}
	for i, mutate := range mutations {
		cursor := initial
		mutate(&cursor)
		raw := base64.RawURLEncoding.EncodeToString([]byte(encode(cursor)))
		if _, e := s.workflowStepsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, raw); workflowListErrorCode(e) != "invalid_cursor" {
			t.Fatalf("unsafe/foreign cursor %d accepted: %v", i, e)
		}
	}
	unsafeNumber := strings.Replace(string(decoded), `"afterSequence":"1"`, `"afterSequence":1`, 1)
	for _, raw := range []string{base64.RawURLEncoding.EncodeToString([]byte(unsafeNumber)), *page.NextCursor + "\n", strings.Repeat("a", 2049)} {
		if _, e := s.workflowStepsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 1, raw); workflowListErrorCode(e) != "invalid_cursor" {
			t.Fatal("numeric/noncanonical/unbounded cursor accepted", e)
		}
	}
	legacyLimit, limitErr := domain.ParseLimit("")
	if limitErr != nil || legacyLimit != 25 {
		t.Fatal("accepted older route default changed", legacyLimit, limitErr)
	}
	for sequence := int64(3); sequence <= 32; sequence++ {
		workflowListStep(t, s, a, sequence, "review", "native")
	}
	defaultPage, defaultErr := s.workflowStepsList(ctx, a.Task.ID, domain.WorkflowSelector{}, 0, "")
	if defaultErr != nil || len(defaultPage.Items) != 30 || defaultPage.NextCursor == nil {
		t.Fatal("new workflow omitted default did not return thirty complete rows", defaultPage, defaultErr)
	}
	if tasks, attempts, steps, workflows := workflowCounts(t, s); tasks != 1 || attempts != 32 || steps != 32 || workflows != 1 {
		t.Fatal("malformed observation mutated workflow/admission state", tasks, attempts, steps, workflows)
	}
}
