package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type graphAdmissionObservation struct {
	ID               string
	Sequence         string
	GraphID          string
	NodeID           string
	TaskVersion      string
	InputID          string
	LogicalSlot      string
	ExecutionOrdinal string
	Generation       string
	Trigger          domain.GraphFreshTrigger
	ConsumptionKey   string
	Outcome          string
	DispatchID       *string
	Failure          *domain.Error
	ObservedAt       string
}

func graphAdmissionObservationKey(graphID string, f domain.GraphNodeAdmissionFacts, trigger domain.GraphFreshTrigger) string {
	triggerClass := "settled"
	if trigger.Kind == "explicit" {
		triggerClass = "explicit"
	}
	return Fingerprint("graphs.node.admission.observation", struct{ Graph, Node, Version, Input, Slot, Execution, Generation, TriggerClass, TriggerID string }{graphID, f.NodeID, f.TaskVersion, f.InputBinding.ID, "author:0", "1", f.Control.Generation, triggerClass, trigger.ID})
}
func graphAdmissionObserved(ctx context.Context, q queryer, key string) (bool, error) {
	var raw, stored string
	e := q.QueryRowContext(ctx, "SELECT consumption_key,data FROM graph_admission_observations WHERE consumption_key=?", key).Scan(&stored, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, storageError()
	}
	observation, e := decode[graphAdmissionObservation](raw)
	if e != nil || stored != key || observation.ConsumptionKey != key {
		return false, storageError()
	}
	return true, nil
}
func storeGraphAdmissionObservation(ctx context.Context, tx *sql.Tx, graphID string, f domain.GraphNodeAdmissionFacts, trigger domain.GraphFreshTrigger, dispatch *string, block *domain.Error) error {
	key := graphAdmissionObservationKey(graphID, f, trigger)
	seq, e := nextSequence(ctx, tx, "graph_admission_observations")
	if e != nil {
		return e
	}
	outcome := "admitted"
	if block != nil {
		outcome = "blocked"
		if block.Code == "capacity_wait" || block.Code == "attempt_active" {
			outcome = "capacity_wait"
		}
	}
	observation := graphAdmissionObservation{ID: domain.NewID(), Sequence: domain.Decimal(seq), GraphID: graphID, NodeID: f.NodeID, TaskVersion: f.TaskVersion, InputID: f.InputBinding.ID, LogicalSlot: "author:0", ExecutionOrdinal: "1", Generation: f.Control.Generation, Trigger: trigger, ConsumptionKey: key, Outcome: outcome, DispatchID: dispatch, Failure: block, ObservedAt: domain.Now()}
	_, e = tx.ExecContext(ctx, "INSERT INTO graph_admission_observations VALUES(?,?,?,?,?,?,?,?,?,?)", seq, observation.ID, graphID, f.NodeID, intRevision(f.TaskVersion), f.InputBinding.ID, key, dispatch, outcome, encode(observation))
	if e != nil {
		return storageError()
	}
	return nil
}
