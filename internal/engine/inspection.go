package engine

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Inspection enriches a detached read value; persisted attempts stay append-only
// with their original references and never acquire transcript-sized result bodies.
func (e *Engine) inspectAttempt(ctx context.Context, a domain.Attempt) (domain.Attempt, error) {
	if a.SourceSnapshotID != nil {
		source, err := e.db.SourceSnapshot(ctx, *a.SourceSnapshotID)
		if err != nil {
			return a, err
		}
		a.SourceSnapshot = &source
	}
	if a.CandidateID != nil {
		candidate, err := e.db.Candidate(ctx, *a.CandidateID)
		if err != nil {
			return a, err
		}
		a.Candidate = &candidate
	}
	incidentID := a.Cleanup.IncidentID
	if incidentID == nil {
		gate, err := e.db.Gate(ctx)
		if err != nil {
			return a, err
		}
		incidentID = gate.IncidentID
	}
	if incidentID != nil {
		incident, err := e.db.Incident(ctx, *incidentID)
		if err != nil {
			return a, err
		}
		if incident.AttemptID == nil || *incident.AttemptID == a.ID {
			a.Incident = &incident
		}
	}
	return a, nil
}
