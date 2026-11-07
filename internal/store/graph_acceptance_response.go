package store

import (
	"context"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// A response acknowledgement accepts its actual response producer. It cannot
// replace required cumulative file delivery merely because this receiver made
// no additional edits to its already materialized file input.
func (s *Store) graphResponseFinalAcceptance(ctx context.Context, q queryer, h graphHeader, node graphNodeRecord, w domain.Workflow, input domain.GraphInputBinding) error {
	if h.Summary.FinalNodeID == nil || *h.Summary.FinalNodeID != node.Summary.ID {
		return nil
	}
	definition, err := s.graphDefinition(ctx, q, h.Summary.ID, h.Summary.GraphVersion)
	if err != nil {
		return err
	}
	if definition.Original.FinalNodeKey != node.Summary.Key || !reflect.DeepEqual(definition.Delivery, w.Authority.Delivery) ||
		w.TaskID != node.Summary.TaskID || w.TaskVersion != node.Summary.TaskVersion || w.SelectedResponseBinding == nil || w.SelectedCandidateID != nil ||
		input.GraphID != h.Summary.ID || input.GraphVersion != h.Summary.GraphVersion || input.ReceivingNodeID != node.Summary.ID ||
		input.ReceivingTaskID != w.TaskID || input.ReceivingTaskVersion != w.TaskVersion {
		return evidenceUnavailable()
	}
	version, err := s.version(ctx, q, w.TaskID, w.TaskVersion)
	if err != nil {
		return err
	}
	if version.Acceptance.Deliverable != "response_only" {
		return evidenceUnavailable()
	}
	// graphAcceptedResult has already joined this exact vector to current
	// accepted producer bindings. No file ancestry or delivery is inferred from
	// response text, matching bytes or the receiver's own no-delta flags.
	if definition.Delivery.LocalMode == "apply_verified_local" {
		for _, contributor := range input.ContributorVector {
			if contributor.CandidateBinding != nil {
				return domain.Fail("application_unsupported", "A response-only final cannot acknowledge the captured application of required file results.")
			}
		}
	}
	return nil
}
