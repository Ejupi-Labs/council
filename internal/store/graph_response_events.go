package store

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type graphResponseAcknowledgementMetadata struct {
	AcceptanceID    string `json:"acceptanceId"`
	RequestID       string `json:"requestId"`
	GraphID         string `json:"graphId"`
	NodeID          string `json:"nodeId"`
	TaskVersion     string `json:"taskVersion"`
	OwnerInstanceID string `json:"ownerInstanceId"`
}

// Human acknowledgement is its own fresh event, not a replay of the original
// producer completion. It changes neither that terminal nor the response wire.
func (s *Store) graphResponseAcknowledged(ctx context.Context, tx *sql.Tx, record domain.ResponseAcceptance, ownerID string) error {
	member, e := s.graphMembership(ctx, tx, record.TaskID)
	if e != nil || member == nil {
		return e
	}
	if member.Kind != "node" || member.NodeID == nil {
		return graphScopeError(member)
	}
	metadata := graphResponseAcknowledgementMetadata{AcceptanceID: record.ID, RequestID: record.RequestID, GraphID: member.GraphID, NodeID: *member.NodeID, TaskVersion: record.TaskVersion, OwnerInstanceID: ownerID}
	_, e = s.appendEvent(ctx, tx, record.AttemptID, "graph_response_acknowledged", jsontext.Value(encode(metadata)))
	return e
}

func (s *Store) graphResponseAckEvent(ctx context.Context, q queryer, h graphHeader, acceptanceID, ownerID string) (int64, domain.ResponseAcceptance, error) {
	var record domain.ResponseAcceptance
	record, e := storedWorkflowRecord[domain.ResponseAcceptance](ctx, q, "response_acceptances", acceptanceID)
	if e != nil {
		return 0, record, e
	}
	nodes, e := s.graphNodes(ctx, q, h.Summary.ID)
	if e != nil {
		return 0, record, e
	}
	var selected *graphNodeRecord
	for i := range nodes {
		n := &nodes[i]
		if n.Summary.TaskID == record.TaskID {
			selected = n
			break
		}
	}
	if selected == nil || selected.Summary.Stop != nil || selected.Summary.TaskVersion != record.TaskVersion || selected.Summary.AcceptedResult == nil || selected.Summary.AcceptedResult.AcceptanceKind != "response" || selected.Summary.AcceptedResult.AcceptanceID != acceptanceID || selected.Summary.AcceptedResult.ProducerAttemptID != record.AttemptID {
		return 0, record, graphControlError()
	}
	head, e := s.graphSelectedHead(ctx, q, h, *selected)
	if e != nil || head.Vector.AcceptanceKind != "response" || head.Vector.AcceptanceID != record.ID {
		if e != nil {
			return 0, record, e
		}
		return 0, record, evidenceUnavailable()
	}
	var count int
	var sequence sql.NullInt64
	var raw sql.NullString
	e = q.QueryRowContext(ctx, "SELECT count(*),min(sequence),min(data) FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='graph_response_acknowledged' AND json_extract(data,'$.metadata.acceptanceId')=?", record.AttemptID, record.ID).Scan(&count, &sequence, &raw)
	if e != nil {
		return 0, record, storageError()
	}
	if count != 1 || !sequence.Valid || sequence.Int64 < 1 || !raw.Valid {
		return 0, record, evidenceUnavailable()
	}
	event, e := decode[domain.AttemptEvent](raw.String)
	if e != nil {
		return 0, record, storageError()
	}
	metadata, e := decode[graphResponseAcknowledgementMetadata](string(event.Metadata))
	if e != nil || event.AttemptID != record.AttemptID || event.Sequence != domain.Decimal(sequence.Int64) || event.Kind != "graph_response_acknowledged" || metadata.AcceptanceID != record.ID || metadata.RequestID != record.RequestID || metadata.GraphID != h.Summary.ID || metadata.NodeID != selected.Summary.ID || metadata.TaskVersion != record.TaskVersion || ownerID != "" && metadata.OwnerInstanceID != ownerID {
		return 0, record, evidenceUnavailable()
	}
	return sequence.Int64, record, nil
}

func (s *Store) graphResponseHeadsFresh(ctx context.Context, q queryer, h graphHeader, trigger domain.GraphFreshTrigger, vector []domain.GraphContributorBinding) error {
	if trigger.Kind == "explicit" {
		return nil
	}
	hasResponse := false
	for _, head := range vector {
		hasResponse = hasResponse || head.AcceptanceKind == "response"
	}
	if !hasResponse {
		return nil
	}
	var source int64
	if trigger.Kind == "response_ack" {
		var e error
		source, _, e = s.graphResponseAckEvent(ctx, q, h, trigger.ID, "")
		if e != nil {
			return e
		}
	} else if e := q.QueryRowContext(ctx, "SELECT sequence FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='settled' ORDER BY sequence DESC LIMIT 1", trigger.ID).Scan(&source); e != nil {
		return storageError()
	}
	for _, head := range vector {
		if head.AcceptanceKind != "response" {
			continue
		}
		sequence, _, e := s.graphResponseAckEvent(ctx, q, h, head.AcceptanceID, "")
		if e != nil {
			return e
		}
		if source < sequence {
			return domain.Fail("graph_input_stale", "The selected response was acknowledged after this dispatch source. Use its fresh acknowledgement, a later actual claim release or explicit graph Advance.")
		}
	}
	return nil
}
