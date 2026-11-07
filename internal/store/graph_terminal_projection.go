package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Queue expiry, Stop and owner recovery have no fresh continuation facts. They
// still must project their actual terminal observation, without qualifying an
// output, changing controls or granting any new execution authority.
func (s *Store) projectGraphTerminal(ctx context.Context, tx *sql.Tx, a domain.Attempt) error {
	if a.Phase != "settled" || a.Outcome == nil {
		return nil
	}
	member, err := s.graphMembership(ctx, tx, a.TaskID)
	if err != nil || member == nil {
		return err
	}
	if member.Kind == "root" && a.ExecutorKind == "local_graph" {
		return s.projectGraphLocalTerminal(ctx, tx, a)
	}
	if member.Kind != "node" || member.NodeID == nil || a.WorkflowID == "" || a.StepID == "" {
		return nil
	}
	task, err := s.task(ctx, tx, a.TaskID)
	if err != nil {
		return err
	}
	if task.CurrentVersion != a.TaskVersion || task.LatestAttemptID == nil || *task.LatestAttemptID != a.ID {
		return nil
	}
	w, err := s.workflowByID(ctx, tx, a.WorkflowID)
	if err != nil {
		return err
	}
	if w.TaskID != task.ID || w.TaskVersion != a.TaskVersion || w.CurrentStepID == nil || *w.CurrentStepID != a.StepID || w.State == "completed" {
		return nil
	}
	record, err := s.graphDispatchForAttempt(ctx, tx, a.ID)
	if err != nil {
		return err
	}
	d := record.Public.Dispatch
	if d.GraphID != member.GraphID || d.NodeID != *member.NodeID || d.TaskID != a.TaskID || d.TaskVersion != a.TaskVersion || d.WorkflowID != a.WorkflowID || d.StepID != a.StepID || d.OwnerInstanceID != a.OwnerInstanceID || d.Role != a.Role || d.ExecutorKind != attemptExecutor(a) {
		return evidenceUnavailable()
	}
	nodes, err := s.graphNodes(ctx, tx, member.GraphID)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if n.Summary.ID != *member.NodeID {
			continue
		}
		if n.Summary.TaskVersion != a.TaskVersion || n.Summary.LatestAttemptID == nil || *n.Summary.LatestAttemptID != a.ID || n.Summary.InputID == nil || *n.Summary.InputID != d.InputID || n.Summary.NodeAdmissionID == nil || *n.Summary.NodeAdmissionID != d.NodeAdmissionID || n.Summary.AcceptedResult != nil || n.Summary.ControlRevision != d.NodeControlRevision && n.Summary.Stop == nil {
			return nil
		}
		return s.projectGraphWorkflow(ctx, tx, task, &w, nil)
	}
	return evidenceUnavailable()
}

func (s *Store) projectGraphLocalTerminal(ctx context.Context, tx *sql.Tx, a domain.Attempt) error {
	op, err := s.graphInputOperation(ctx, tx, a.OwnerInstanceID, a.ID)
	if err != nil {
		return err
	}
	record := op.InputRecord
	// Completion already owns its exact physical result and projection. A later
	// generic observation cannot rewrite it or manufacture a completion seal.
	if record.CompletionFingerprint != "" || record.Public.Summary.State != "preparing" {
		return nil
	}
	inputState, terminalState := "incomplete", "failed"
	switch *a.Outcome {
	case "interrupted":
		inputState, terminalState = "interrupted", "interrupted"
	case "cancelled":
		inputState, terminalState = "blocked", "cancelled"
	case "blocked":
		inputState, terminalState = "blocked", "blocked"
	case "failed":
	default:
		return nil
	}
	wasCurrent := record.Public.Summary.Current
	record.Public.Summary.State = inputState
	record.Public.Summary.Current = false
	record.Public.Summary.UpdatedAt = domain.Now()
	if a.PrimaryFailure != nil {
		record.Public.Summary.Failure = domain.Fail(a.PrimaryFailure.Code, a.PrimaryFailure.Message)
	} else if a.Stop != nil {
		record.Public.Summary.Failure = domain.Fail("operation_cancelled", "Graph preparation was stopped; inspect its retained terminal and actual cleanup.")
	} else {
		record.Public.Summary.Failure = domain.Fail("operation_interrupted", "Graph preparation was interrupted; inspect its retained terminal and actual cleanup.")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE graph_inputs SET state=?,data=? WHERE id=? AND operation_attempt_id=?", inputState, encode(record), op.ID, a.ID); err != nil {
		return storageError()
	}
	h, err := s.graphHeader(ctx, tx, op.GraphID)
	if err != nil {
		return err
	}
	if !wasCurrent || h.Summary.GraphVersion != op.Control.GraphVersion || h.Summary.State == "completed" {
		return nil
	}
	nodes, err := s.graphNodes(ctx, tx, op.GraphID)
	if err != nil {
		return err
	}
	stoppedNode := false
	if op.Kind == "root_basis" {
		if h.Summary.RootInputID == nil || *h.Summary.RootInputID != op.ID {
			return nil
		}
	} else {
		current := false
		for i := range nodes {
			n := &nodes[i]
			if op.Control.NodeID == nil || n.Summary.ID != *op.Control.NodeID {
				continue
			}
			task, err := s.task(ctx, tx, n.Summary.TaskID)
			if err != nil {
				return err
			}
			if op.Control.TaskID == nil || *op.Control.TaskID != task.ID || op.Control.TaskVersion == nil || *op.Control.TaskVersion != task.CurrentVersion || n.Summary.TaskVersion != task.CurrentVersion || n.Summary.InputID == nil || *n.Summary.InputID != op.ID || n.Summary.NodeAdmissionID != nil || n.Summary.AcceptedResult != nil {
				return nil
			}
			if op.Control.NodeControlRevision == nil || *op.Control.NodeControlRevision != n.Summary.ControlRevision && n.Summary.Stop == nil {
				return nil
			}
			current = true
			stoppedNode = n.Summary.Stop != nil
			if !graphRetainedTerminal(n.Summary.State) {
				n.Summary.State = terminalState
				if h.Summary.Stop != nil || stoppedNode {
					n.Summary.State = "cancelled"
				}
				n.Summary.NextOwner = "user"
				if h.Summary.Stop == nil && !stoppedNode {
					n.Summary.NextAction = "Inspect retained input preparation evidence before explicit resumption."
				}
				n.Summary.StateRevision, err = increment(n.Summary.StateRevision)
				if err != nil {
					return err
				}
				if err = saveGraphNode(ctx, tx, *n); err != nil {
					return err
				}
			}
		}
		if !current {
			return evidenceUnavailable()
		}
	}
	h.Summary.Counts, err = graphCounts(nodes)
	if err != nil {
		return err
	}
	if h.Summary.Stop != nil {
		pending, err := s.graphPending(ctx, tx, h)
		if err != nil {
			return err
		}
		h.Summary.State = "cancelled"
		if len(pending) != 0 {
			h.Summary.State = "cancelling"
		}
		h.Summary.NextOwner = "user"
		h.Summary.NextAction = "Inspect stopped work and retained partial results."
	} else if !stoppedNode {
		h.Summary.State = terminalState
		h.Summary.NextOwner = "user"
		h.Summary.NextAction = "Inspect retained graph preparation evidence before explicit resumption."
		// Match ordinary workflow aggregation: a sibling's retained failure has
		// priority over this interrupted preparation, and no node is rewritten.
		if h.Summary.Counts.Failed != "0" {
			h.Summary.State = "failed"
			h.Summary.NextAction = "Inspect the failed required node and retained independent work."
		} else if h.Summary.Counts.Interrupted != "0" {
			h.Summary.State = "interrupted"
			h.Summary.NextAction = "Inspect interrupted required work before explicit resumption."
		}
	}
	h.Summary.StateRevision, err = increment(h.Summary.StateRevision)
	if err != nil {
		return err
	}
	h.Summary.UpdatedAt = domain.Now()
	return saveGraphHeader(ctx, tx, h)
}
