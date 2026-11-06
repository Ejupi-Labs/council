package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"strconv"
)

type workflowListCursor struct {
	Resource          string `json:"resource"`
	TaskID            string `json:"taskId"`
	WorkflowID        string `json:"workflowId"`
	TaskVersion       string `json:"taskVersion"`
	PrimaryMaximum    string `json:"primaryMaximum"`
	SecondaryMaximum  string `json:"secondaryMaximum"`
	TertiaryMaximum   string `json:"tertiaryMaximum"`
	ResolutionMaximum string `json:"resolutionMaximum"`
	AfterSequence     string `json:"afterSequence"`
	AfterSource       string `json:"afterSource"`
	AfterIndex        string `json:"afterIndex"`
}
type workflowListPosition struct {
	sequence      int64
	source, index int
}
type workflowListBoundary struct {
	cursor  workflowListCursor
	maximum [4]int64
	after   workflowListPosition
}
type workflowRawListRow struct {
	position workflowListPosition
	id, raw  string
}
type workflowPageItem[T any] struct {
	position workflowListPosition
	item     T
}

func workflowListNumber(raw string) (int64, error) {
	n, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || n < 0 || strconv.FormatInt(n, 10) != raw {
		return 0, cursorError()
	}
	return n, nil
}
func workflowListToken(boundary workflowListBoundary, after workflowListPosition) *string {
	c := boundary.cursor
	c.AfterSequence = domain.Decimal(after.sequence)
	c.AfterSource = strconv.Itoa(after.source)
	c.AfterIndex = strconv.Itoa(after.index)
	raw := base64.RawURLEncoding.EncodeToString([]byte(encode(c)))
	return &raw
}
func workflowListMaximum(ctx context.Context, q queryer, table, column, condition string) (int64, error) {
	// Identifiers/conditions are authored constants, never request values.
	var n int64
	if e := q.QueryRowContext(ctx, "SELECT COALESCE(MAX("+column+"),0) FROM "+table+condition).Scan(&n); e != nil || n < 0 {
		return 0, storageError()
	}
	return n, nil
}
func (s *Store) workflowListSelection(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, raw, resource string) (domain.WorkflowDetail, workflowListBoundary, error) {
	var boundary workflowListBoundary
	if limit < 1 || limit > 100 {
		return domain.WorkflowDetail{}, boundary, domain.Invalid("limit")
	}
	selected, e := s.selectedWorkflow(ctx, s.db, taskID, selector)
	if e != nil {
		return selected, boundary, e
	}
	if selected.Workflow == nil {
		if raw != "" {
			return selected, boundary, cursorError()
		}
		return selected, boundary, nil
	}
	boundary.cursor = workflowListCursor{Resource: resource, TaskID: taskID, WorkflowID: selected.Workflow.ID, TaskVersion: selected.TaskVersion, PrimaryMaximum: "0", SecondaryMaximum: "0", TertiaryMaximum: "0", ResolutionMaximum: "0", AfterSequence: "0", AfterSource: "0", AfterIndex: "0"}
	if raw != "" {
		if len(raw) > 2048 {
			return selected, boundary, cursorError()
		}
		decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
		if err != nil {
			return selected, boundary, cursorError()
		}
		var c workflowListCursor
		if json.Unmarshal(decoded, &c, json.RejectUnknownMembers(true)) != nil || base64.RawURLEncoding.EncodeToString([]byte(encode(c))) != raw || c.Resource != resource || c.TaskID != taskID || c.WorkflowID != selected.Workflow.ID || c.TaskVersion != selected.TaskVersion {
			return selected, boundary, cursorError()
		}
		values := []string{c.PrimaryMaximum, c.SecondaryMaximum, c.TertiaryMaximum, c.ResolutionMaximum, c.AfterSequence, c.AfterSource, c.AfterIndex}
		numbers := make([]int64, len(values))
		for i, value := range values {
			numbers[i], err = workflowListNumber(value)
			if err != nil {
				return selected, boundary, err
			}
		}
		copy(boundary.maximum[:], numbers[:4])
		boundary.after = workflowListPosition{sequence: numbers[4], source: int(numbers[5]), index: int(numbers[6])}
		boundary.cursor = c
		if numbers[5] > 2 || numbers[6] > 63 {
			return selected, boundary, cursorError()
		}
		switch resource {
		case "findings":
			if numbers[5] > 1 || numbers[4] > boundary.maximum[2] || numbers[5] == 1 && numbers[6] != 0 {
				return selected, boundary, cursorError()
			}
		case "deliveries":
			if numbers[6] != 0 || numbers[4] > boundary.maximum[numbers[5]] || numbers[3] != 0 {
				return selected, boundary, cursorError()
			}
		default:
			if numbers[5] != 0 || numbers[6] != 0 || numbers[4] > numbers[0] || numbers[1] != 0 || numbers[2] != 0 || numbers[3] != 0 {
				return selected, boundary, cursorError()
			}
		}
		return selected, boundary, nil
	}
	table, column := "", "sequence"
	switch resource {
	case "steps":
		table = "workflow_steps"
	case "reviews":
		table = "workflow_reviews"
	case "checks":
		table = "workflow_checks"
	case "candidates":
		table, column = "candidates", "rowid"
	case "findings":
		for i, t := range []string{"workflow_reviews", "workflow_checks", "workflow_steps", "workflow_resolutions"} {
			boundary.maximum[i], e = workflowListMaximum(ctx, s.db, t, "sequence", "")
			if e != nil {
				return selected, boundary, e
			}
		}
	case "deliveries":
		boundary.maximum[0], e = workflowListMaximum(ctx, s.db, "delivery_operations", "sequence", "")
		if e != nil {
			return selected, boundary, e
		}
		boundary.maximum[1], e = workflowListMaximum(ctx, s.db, "workflow_steps", "sequence", " WHERE json_type(data,'$.externalResult')='object'")
		if e != nil {
			return selected, boundary, e
		}
		boundary.maximum[2], e = workflowListMaximum(ctx, s.db, "workflow_acceptances", "sequence", "")
		if e != nil {
			return selected, boundary, e
		}
	default:
		return selected, boundary, storageError()
	}
	if table != "" {
		boundary.maximum[0], e = workflowListMaximum(ctx, s.db, table, column, "")
		if e != nil {
			return selected, boundary, e
		}
	}
	boundary.cursor.PrimaryMaximum = domain.Decimal(boundary.maximum[0])
	boundary.cursor.SecondaryMaximum = domain.Decimal(boundary.maximum[1])
	boundary.cursor.TertiaryMaximum = domain.Decimal(boundary.maximum[2])
	boundary.cursor.ResolutionMaximum = domain.Decimal(boundary.maximum[3])
	return selected, boundary, nil
}
func workflowRawRows(ctx context.Context, q schemaQueryer, query string, args ...any) ([]workflowRawListRow, error) {
	rows, e := q.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, storageError()
	}
	defer rows.Close()
	out := []workflowRawListRow{}
	for rows.Next() {
		var row workflowRawListRow
		if rows.Scan(&row.position.sequence, &row.position.source, &row.position.index, &row.id, &row.raw) != nil {
			return nil, storageError()
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, storageError()
	}
	return out, nil
}
func workflowPageTooLarge() *domain.Error {
	return domain.Fail("evidence_unavailable", "A complete retained workflow item exceeds the 4 MiB response bound. Inspect its original review/check and full artifact evidence; the item was not clipped or skipped.")
}
func workflowFinishPage[T any](items []workflowPageItem[T], boundary workflowListBoundary, limit int) (domain.Page[T], error) {
	out := domain.Page[T]{Items: []T{}}
	count := len(items)
	if count > limit {
		count = limit
	}
	more := len(items) > count
	for {
		out.Items = make([]T, count)
		for i := 0; i < count; i++ {
			out.Items[i] = items[i].item
		}
		out.NextCursor = nil
		if more && count > 0 {
			out.NextCursor = workflowListToken(boundary, items[count-1].position)
		}
		encoded, e := json.Marshal(domain.Envelope{APIVersion: domain.APIVersion, RequestID: "00000000000000000000000000000000", Data: out})
		if e != nil {
			return domain.Page[T]{}, storageError()
		}
		if len(encoded)+1 <= domain.MaxUsagePageBytes {
			return out, nil
		}
		if count <= 1 {
			return domain.Page[T]{}, workflowPageTooLarge()
		}
		count--
		more = true
	}
}

func (s *Store) workflowStepsList(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.WorkflowStep], error) {
	if limit == 0 {
		limit = 30
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := domain.Page[domain.WorkflowStep]{Items: []domain.WorkflowStep{}}
	selected, boundary, e := s.workflowListSelection(ctx, taskID, selector, limit, cursor, "steps")
	if e != nil || selected.Workflow == nil {
		return empty, e
	}
	rows, e := workflowRawRows(ctx, s.db, "SELECT sequence,0,0,id,data FROM workflow_steps WHERE workflow_id=? AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", selected.Workflow.ID, boundary.after.sequence, boundary.maximum[0], limit+1)
	if e != nil {
		return empty, e
	}
	items := []workflowPageItem[domain.WorkflowStep]{}
	for _, row := range rows {
		step, e := s.workflowStep(ctx, s.db, row.id)
		if e != nil || step.WorkflowID != selected.Workflow.ID {
			return empty, storageError()
		}
		items = append(items, workflowPageItem[domain.WorkflowStep]{row.position, step})
	}
	return workflowFinishPage(items, boundary, limit)
}
func workflowReviewForList(ctx context.Context, q queryer, id, workflowID string) (domain.ReviewRecord, error) {
	var raw, wf, step, attempt, result string
	var candidate, response sql.NullString
	if q.QueryRowContext(ctx, "SELECT workflow_id,step_id,attempt_id,candidate_id,response_artifact_id,result_artifact_id,data FROM workflow_reviews WHERE id=?", id).Scan(&wf, &step, &attempt, &candidate, &response, &result, &raw) != nil {
		return domain.ReviewRecord{}, storageError()
	}
	record, e := decode[domain.ReviewRecord](raw)
	if e != nil || record.ID != id || record.WorkflowID != workflowID || wf != workflowID || record.StepID != step || record.AttemptID != attempt || record.ResultArtifactID != result || !workflowSubjectMatches(record.CandidateBinding, record.ResponseBinding, candidate, response) {
		return record, storageError()
	}
	return record, nil
}
func workflowCheckForList(ctx context.Context, q queryer, id, workflowID string) (domain.CheckReceipt, error) {
	var raw, wf, step, attempt, definition string
	var candidate, response sql.NullString
	if q.QueryRowContext(ctx, "SELECT workflow_id,step_id,attempt_id,definition_id,candidate_id,response_artifact_id,data FROM workflow_checks WHERE id=?", id).Scan(&wf, &step, &attempt, &definition, &candidate, &response, &raw) != nil {
		return domain.CheckReceipt{}, storageError()
	}
	record, e := decode[domain.CheckReceipt](raw)
	if e != nil || record.ID != id || record.WorkflowID != workflowID || wf != workflowID || record.StepID != step || record.AttemptID != attempt || record.DefinitionID != definition || !workflowSubjectMatches(record.InputCandidateBinding, record.InputResponseBinding, candidate, response) {
		return record, storageError()
	}
	return record, nil
}
func workflowSubjectMatches(candidate *domain.CandidateBinding, response *domain.ResponseBinding, candidateID, responseID sql.NullString) bool {
	return candidate != nil && response == nil && candidateID.Valid && !responseID.Valid && candidate.CandidateID == candidateID.String || response != nil && candidate == nil && responseID.Valid && !candidateID.Valid && response.ResponseArtifactID == responseID.String
}
func (s *Store) workflowReviewsList(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.ReviewRecord], error) {
	if limit == 0 {
		limit = 30
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := domain.Page[domain.ReviewRecord]{Items: []domain.ReviewRecord{}}
	selected, boundary, e := s.workflowListSelection(ctx, taskID, selector, limit, cursor, "reviews")
	if e != nil || selected.Workflow == nil {
		return empty, e
	}
	rows, e := workflowRawRows(ctx, s.db, "SELECT sequence,0,0,id,data FROM workflow_reviews WHERE workflow_id=? AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", selected.Workflow.ID, boundary.after.sequence, boundary.maximum[0], limit+1)
	if e != nil {
		return empty, e
	}
	items := []workflowPageItem[domain.ReviewRecord]{}
	for _, row := range rows {
		record, e := workflowReviewForList(ctx, s.db, row.id, selected.Workflow.ID)
		if e != nil {
			return empty, e
		}
		items = append(items, workflowPageItem[domain.ReviewRecord]{row.position, record})
	}
	return workflowFinishPage(items, boundary, limit)
}
func (s *Store) workflowChecksList(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.CheckReceipt], error) {
	if limit == 0 {
		limit = 30
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := domain.Page[domain.CheckReceipt]{Items: []domain.CheckReceipt{}}
	selected, boundary, e := s.workflowListSelection(ctx, taskID, selector, limit, cursor, "checks")
	if e != nil || selected.Workflow == nil {
		return empty, e
	}
	rows, e := workflowRawRows(ctx, s.db, "SELECT sequence,0,0,id,data FROM workflow_checks WHERE workflow_id=? AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", selected.Workflow.ID, boundary.after.sequence, boundary.maximum[0], limit+1)
	if e != nil {
		return empty, e
	}
	items := []workflowPageItem[domain.CheckReceipt]{}
	for _, row := range rows {
		record, e := workflowCheckForList(ctx, s.db, row.id, selected.Workflow.ID)
		if e != nil {
			return empty, e
		}
		items = append(items, workflowPageItem[domain.CheckReceipt]{row.position, record})
	}
	return workflowFinishPage(items, boundary, limit)
}

func workflowFindingResolutions(ctx context.Context, q schemaQueryer, workflowID string, finding domain.WorkflowFinding, maximum int64) ([]domain.FindingResolution, error) {
	rows, e := q.QueryContext(ctx, "SELECT id,origin_review_id,origin_check_id,corrected_candidate_id,corrected_response_artifact_id,corrected_subject_sha256,data FROM workflow_resolutions WHERE workflow_id=? AND finding_id=? AND sequence<=? ORDER BY sequence", workflowID, finding.ID, maximum)
	if e != nil {
		return nil, storageError()
	}
	defer rows.Close()
	out := []domain.FindingResolution{}
	total := 0
	for rows.Next() {
		var id, subject, raw string
		var review, check, candidate, response sql.NullString
		if rows.Scan(&id, &review, &check, &candidate, &response, &subject, &raw) != nil {
			return nil, storageError()
		}
		record, e := decode[domain.FindingResolution](raw)
		if e != nil || record.ID != id || record.WorkflowID != workflowID || record.FindingID != finding.ID || !optionalStringMatches(record.OriginReviewID, review) || !optionalStringMatches(record.OriginCheckID, check) || !optionalStringMatches(record.CorrectedCandidateID, candidate) || review.Valid && review.String != finding.ReviewID || check.Valid && check.String != finding.CheckID {
			return nil, storageError()
		}
		if record.CorrectedResponseBinding != nil {
			if candidate.Valid || !response.Valid || record.CorrectedResponseBinding.ResponseArtifactID != response.String || record.CorrectedResponseBinding.ResponseSHA256 != subject {
				return nil, storageError()
			}
		} else if !candidate.Valid || response.Valid {
			return nil, storageError()
		}
		total += len(raw)
		if total > domain.MaxUsagePageBytes {
			return nil, workflowPageTooLarge()
		}
		out = append(out, record)
	}
	if rows.Err() != nil {
		return nil, storageError()
	}
	return out, nil
}
func (s *Store) workflowFindingsList(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.WorkflowFinding], error) {
	if limit == 0 {
		limit = 30
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := domain.Page[domain.WorkflowFinding]{Items: []domain.WorkflowFinding{}}
	selected, boundary, e := s.workflowListSelection(ctx, taskID, selector, limit, cursor, "findings")
	if e != nil || selected.Workflow == nil {
		return empty, e
	}
	wf := selected.Workflow.ID
	// Membership highwaters are independent of parent-step creation. The tuple
	// permits a page cut inside one immutable review without dropping findings.
	query := `WITH entries AS (
 SELECT st.sequence AS seq,0 AS source,CAST(f.key AS INTEGER) AS idx,r.id AS parent,f.value AS data,r.workflow_id AS workflow,st.workflow_id AS step_workflow,r.attempt_id AS attempt,st.attempt_id AS step_attempt,st.role AS role,json_extract(r.data,'$.workflowId') AS data_workflow,json_extract(r.data,'$.id') AS data_parent
 FROM workflow_reviews r JOIN workflow_steps st ON st.id=r.step_id JOIN json_each(r.data,'$.findings') f WHERE r.workflow_id=? AND r.sequence<=?
 UNION ALL
 SELECT st.sequence,1,0,c.id,json_extract(c.data,'$.finding'),c.workflow_id,st.workflow_id,c.attempt_id,st.attempt_id,st.role,json_extract(c.data,'$.workflowId'),json_extract(c.data,'$.id')
 FROM workflow_checks c JOIN workflow_steps st ON st.id=c.step_id WHERE c.workflow_id=? AND c.sequence<=? AND json_type(c.data,'$.finding')='object'
 ) SELECT seq,source,idx,parent,data,workflow,step_workflow,attempt,step_attempt,role,data_workflow,data_parent FROM entries
 WHERE seq>? OR (seq=? AND (source>? OR (source=? AND idx>?))) ORDER BY seq,source,idx LIMIT ?`
	rows, e := s.db.QueryContext(ctx, query, wf, boundary.maximum[0], wf, boundary.maximum[1], boundary.after.sequence, boundary.after.sequence, boundary.after.source, boundary.after.source, boundary.after.index, limit+1)
	if e != nil {
		return empty, storageError()
	}
	items := []workflowPageItem[domain.WorkflowFinding]{}
	for rows.Next() {
		var position workflowListPosition
		var parent, raw, owner, stepOwner, attempt, role, dataOwner, dataParent string
		var stepAttempt sql.NullString
		if rows.Scan(&position.sequence, &position.source, &position.index, &parent, &raw, &owner, &stepOwner, &attempt, &stepAttempt, &role, &dataOwner, &dataParent) != nil {
			rows.Close()
			return empty, storageError()
		}
		finding, e := decode[domain.WorkflowFinding](raw)
		if e != nil || !domain.ValidID(finding.ID) || owner != wf || stepOwner != wf || dataOwner != wf || dataParent != parent || !stepAttempt.Valid || attempt != stepAttempt.String || position.index < 0 || position.index > 63 || position.source == 0 && (finding.ReviewID != parent || finding.CheckID != "" || role != "review") || position.source == 1 && (finding.CheckID != parent || finding.ReviewID != "" || role != "check") {
			rows.Close()
			return empty, storageError()
		}
		finding.Resolutions = nil
		items = append(items, workflowPageItem[domain.WorkflowFinding]{position, finding})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return empty, storageError()
	}
	for i := range items {
		items[i].item.Resolutions, e = workflowFindingResolutions(ctx, s.db, wf, items[i].item, boundary.maximum[3])
		if e != nil {
			return empty, e
		}
	}
	return workflowFinishPage(items, boundary, limit)
}

func workflowCandidateForList(ctx context.Context, q queryer, id string, selected domain.WorkflowDetail) (domain.Candidate, error) {
	var raw, attempt, source, manifest, delta, workspace, task string
	var version int64
	e := q.QueryRowContext(ctx, "SELECT c.data,c.attempt_id,c.source_snapshot_id,c.manifest_artifact_id,c.delta_artifact_id,c.workspace_ref,a.task_id,a.task_version FROM candidates c JOIN attempts a ON a.id=c.attempt_id WHERE c.id=?", id).Scan(&raw, &attempt, &source, &manifest, &delta, &workspace, &task, &version)
	if e != nil {
		return domain.Candidate{}, storageError()
	}
	candidate, e := decode[domain.Candidate](raw)
	if e != nil || candidate.ID != id || candidate.AttemptID != attempt || candidate.SourceSnapshotID != source || candidate.ManifestArtifactID != manifest || candidate.DeltaArtifactID != delta || candidate.WorkspaceRef != workspace || candidate.WorkflowID != selected.Workflow.ID || !domain.ValidID(candidate.RootSourceSnapshotID) || task != selected.TaskID || domain.Decimal(version) != selected.TaskVersion {
		return candidate, storageError()
	}
	return candidate, nil
}
func (s *Store) workflowCandidatesList(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.Candidate], error) {
	if limit == 0 {
		limit = 30
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := domain.Page[domain.Candidate]{Items: []domain.Candidate{}}
	selected, boundary, e := s.workflowListSelection(ctx, taskID, selector, limit, cursor, "candidates")
	if e != nil || selected.Workflow == nil {
		return empty, e
	}
	// Accepted candidates has no sequence column. Private append-only rowid freezes
	// candidate creation instead of including a future result of an old attempt.
	rows, e := workflowRawRows(ctx, s.db, "SELECT c.rowid,0,0,c.id,c.data FROM candidates c JOIN attempts a ON a.id=c.attempt_id WHERE a.task_id=? AND a.task_version=? AND json_extract(c.data,'$.workflowId')=? AND c.rowid>? AND c.rowid<=? ORDER BY c.rowid LIMIT ?", taskID, intRevision(selected.TaskVersion), selected.Workflow.ID, boundary.after.sequence, boundary.maximum[0], limit+1)
	if e != nil {
		return empty, e
	}
	items := []workflowPageItem[domain.Candidate]{}
	for _, row := range rows {
		candidate, e := workflowCandidateForList(ctx, s.db, row.id, selected)
		if e != nil {
			return empty, e
		}
		items = append(items, workflowPageItem[domain.Candidate]{row.position, candidate})
	}
	return workflowFinishPage(items, boundary, limit)
}

func (s *Store) workflowDeliveriesList(ctx context.Context, taskID string, selector domain.WorkflowSelector, limit int, cursor string) (domain.Page[domain.WorkflowDeliveryView], error) {
	if limit == 0 {
		limit = 30
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := domain.Page[domain.WorkflowDeliveryView]{Items: []domain.WorkflowDeliveryView{}}
	selected, boundary, e := s.workflowListSelection(ctx, taskID, selector, limit, cursor, "deliveries")
	if e != nil || selected.Workflow == nil {
		return empty, e
	}
	wf := selected.Workflow.ID
	// Stable source discriminator and append sequence, with separate source
	// highwaters. Review-only acceptance has no invented process or step identity.
	query := `WITH entries AS (
 SELECT sequence AS seq,0 AS source,0 AS idx,id,data FROM delivery_operations WHERE workflow_id=? AND sequence<=?
 UNION ALL SELECT sequence,1,0,id,data FROM workflow_steps WHERE workflow_id=? AND sequence<=? AND json_type(data,'$.externalResult')='object'
 UNION ALL SELECT sequence,2,0,id,data FROM workflow_acceptances WHERE workflow_id=? AND sequence<=? AND json_extract(data,'$.deliveryOperationId') IS NULL
 ) SELECT seq,source,idx,id,data FROM entries WHERE source>? OR (source=? AND seq>?) ORDER BY source,seq LIMIT ?`
	rows, e := workflowRawRows(ctx, s.db, query, wf, boundary.maximum[0], wf, boundary.maximum[1], wf, boundary.maximum[2], boundary.after.source, boundary.after.source, boundary.after.sequence, limit+1)
	if e != nil {
		return empty, e
	}
	items := []workflowPageItem[domain.WorkflowDeliveryView]{}
	for _, row := range rows {
		view := domain.WorkflowDeliveryView{ID: row.id}
		switch row.position.source {
		case 0:
			operation, e := decode[domain.DeliveryOperation](row.raw)
			if e != nil || operation.ID != row.id || operation.WorkflowID != wf {
				return empty, storageError()
			}
			var owner, step, attempt, candidate, status string
			if s.db.QueryRowContext(ctx, "SELECT workflow_id,step_id,attempt_id,candidate_id,status FROM delivery_operations WHERE id=?", row.id).Scan(&owner, &step, &attempt, &candidate, &status) != nil || owner != wf || operation.StepID != step || operation.AttemptID != attempt || operation.CandidateBinding.CandidateID != candidate || operation.Status != status {
				return empty, storageError()
			}
			view.Kind = "local_application"
			view.StepID = ptr(operation.StepID)
			view.AttemptID = ptr(operation.AttemptID)
			view.Operation = &operation
		case 1:
			step, e := s.workflowStep(ctx, s.db, row.id)
			if e != nil || step.WorkflowID != wf || step.Role != "external_delivery" || step.ExecutorKind != "local_external" || step.AttemptID == nil || step.ExternalResult == nil {
				return empty, storageError()
			}
			external := step.ExternalResult
			if external.WorkflowID != wf || external.StepID != step.ID || external.AttemptID != *step.AttemptID {
				return empty, storageError()
			}
			view.Kind = "external_command"
			view.StepID = ptr(step.ID)
			view.AttemptID = ptr(external.AttemptID)
			view.External = external
		case 2:
			acceptance, e := decode[domain.WorkflowAcceptance](row.raw)
			if e != nil || acceptance.ID != row.id || acceptance.WorkflowID != wf || acceptance.TaskID != taskID || acceptance.TaskVersion != selected.TaskVersion || acceptance.DeliveryOperationID != nil || selected.Workflow.Authority.Delivery.LocalMode != "review_only" {
				return empty, storageError()
			}
			view.Kind = "review_only"
			view.Acceptance = &acceptance
		default:
			return empty, storageError()
		}
		items = append(items, workflowPageItem[domain.WorkflowDeliveryView]{row.position, view})
	}
	return workflowFinishPage(items, boundary, limit)
}
