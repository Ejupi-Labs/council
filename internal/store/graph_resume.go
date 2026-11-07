package store

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// A new owner may continue a healthy retained workflow only through a committed
// explicit graph Advance. Its private association is inherited by that owner's
// later stages; live registration generations do not revoke admitted work.
func (s *Store) graphDispatchOwnerAuthorization(ctx context.Context, q queryer, anchor, target domain.GraphDispatch) error {
	if anchor.GraphID != target.GraphID || anchor.GraphVersion != target.GraphVersion || anchor.NodeID != target.NodeID || anchor.TaskID != target.TaskID || anchor.TaskVersion != target.TaskVersion || anchor.WorkflowID != target.WorkflowID || anchor.InputID != target.InputID || anchor.InputSHA256 != target.InputSHA256 || anchor.ID != target.NodeAdmissionID {
		return evidenceUnavailable()
	}
	if anchor.OwnerInstanceID == target.OwnerInstanceID {
		return nil
	}
	record, err := s.graphDispatchRecord(ctx, q, target.GraphID, target.ID)
	if err != nil {
		return err
	}
	if record.OwnerResumeRequestID == nil || !domain.ValidID(*record.OwnerResumeRequestID) {
		return evidenceUnavailable()
	}
	request := *record.OwnerResumeRequestID
	var operation, raw string
	var status int
	if err = q.QueryRowContext(ctx, "SELECT operation,status,envelope FROM receipts WHERE request_id=?", request).Scan(&operation, &status, &raw); err != nil {
		return evidenceUnavailable()
	}
	envelope, err := decode[domain.Envelope](raw)
	if err != nil || operation != "graphs.advance" || status != 200 || envelope.RequestID != request || envelope.Receipt == nil || envelope.Receipt.RequestID != request || envelope.Receipt.Operation != operation {
		return evidenceUnavailable()
	}
	result, err := decode[domain.GraphAdvanceResult](encode(envelope.Data))
	if err != nil || result.Graph.ID != target.GraphID || len(result.Dispatches) > domain.MaxGraphNodes {
		return evidenceUnavailable()
	}
	for _, resumed := range result.Dispatches {
		d := resumed.Dispatch
		if d.OwnerInstanceID != target.OwnerInstanceID || d.GraphID != target.GraphID || d.GraphVersion != target.GraphVersion || d.NodeID != target.NodeID || d.TaskID != target.TaskID || d.TaskVersion != target.TaskVersion || d.WorkflowID != target.WorkflowID || d.InputID != target.InputID || d.InputSHA256 != target.InputSHA256 || d.NodeAdmissionID != anchor.ID || d.FreshTriggerID != request {
			continue
		}
		actual, err := s.graphDispatchRecord(ctx, q, d.GraphID, d.ID)
		if err != nil {
			return err
		}
		if actual.OwnerResumeRequestID != nil && *actual.OwnerResumeRequestID == request && reflect.DeepEqual(actual.Public.Dispatch, d) {
			return nil
		}
	}
	return evidenceUnavailable()
}

func (s *Store) advanceGraphStages(ctx context.Context, tx *sql.Tx, ownerID string, h graphHeader, in domain.GraphAdvanceInput, facts []domain.WorkflowStepAdmissionFacts) ([]domain.GraphDispatchView, error) {
	out := []domain.GraphDispatchView{}
	if len(facts) > domain.MaxGraphNodes {
		return nil, domain.Invalid("bounded explicit graph workflow stages")
	}
	seen := map[string]bool{}
	for _, fact := range facts {
		if fact.Graph == nil || fact.Graph.Control.NodeID == nil {
			return nil, graphControlError()
		}
		gf := *fact.Graph
		id := *gf.Control.NodeID
		if seen[id] {
			return nil, domain.Invalid("one current workflow stage per captured node")
		}
		seen[id] = true
		authorized := false
		for _, authority := range in.NodeAuthorities {
			if authority.NodeID == id && gf.Control.TaskVersion != nil && *gf.Control.TaskVersion == authority.TaskVersion && gf.Control.NodeControlRevision != nil && *gf.Control.NodeControlRevision == authority.NodeControlRevision {
				authorized = true
			}
		}
		if !authorized || gf.Control.TaskID == nil {
			return nil, graphControlError()
		}
		task, err := s.task(ctx, tx, *gf.Control.TaskID)
		if err != nil {
			return nil, err
		}
		workflow, err := s.selectedWorkflow(ctx, tx, task.ID, domain.WorkflowSelector{})
		if err != nil {
			return nil, err
		}
		if workflow.Workflow == nil {
			return nil, workflowUnavailable()
		}
		budget, err := s.taskBudget(ctx, tx, task.ID)
		if err != nil {
			return nil, err
		}
		gf.Control.Generation = h.DispatchAuthority.Generation
		gf.Trigger = domain.GraphFreshTrigger{Kind: "explicit", ID: in.RequestID, OwnerGeneration: h.DispatchAuthority.Generation}
		fact.Graph = &gf
		w := *workflow.Workflow
		admission, err := s.admitWorkflowContinuation(ctx, tx, ownerID, &task, &w, budget, fact, in.RequestID, "")
		if err != nil {
			return nil, err
		}
		if admission != nil {
			registered, err := s.graphDispatchForAttempt(ctx, tx, admission.Attempt.ID)
			if err != nil {
				return nil, err
			}
			out = append(out, registered.Public)
		}
	}
	return out, nil
}
