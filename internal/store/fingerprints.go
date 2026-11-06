package store

import "github.com/Ejupi-Labs/council/v2/internal/domain"

func TaskStartFingerprint(in domain.StartTaskInput) string {
	return Fingerprint("tasks.start", normalizeStart(in))
}
func WorkflowAdoptFingerprint(id string, in domain.WorkflowAdoptInput) string {
	return Fingerprint("tasks.workflow.adopt", struct {
		TaskID string
		Input  domain.WorkflowAdoptInput
	}{id, in})
}
func WorkflowAdvanceFingerprint(id string, in domain.WorkflowAdvanceInput) string {
	return Fingerprint("tasks.workflow.advance", struct {
		TaskID string
		Input  domain.WorkflowAdvanceInput
	}{id, in})
}
func WorkflowCheckFingerprint(id string, in domain.WorkflowCheckInput) string {
	return Fingerprint("tasks.workflow.check", struct {
		TaskID string
		Input  domain.WorkflowCheckInput
	}{id, in})
}

func GuidanceFingerprint(id string, in domain.GuidanceInput) string {
	return Fingerprint("tasks.guidance", struct {
		TaskID string
		Input  domain.GuidanceInput
	}{id, in})
}
func AttemptStartFingerprint(id string, in domain.StartAttemptInput) string {
	if in.ExecutionMode == "" {
		in.ExecutionMode = "native"
	}
	if in.ExecutionMode == "fake" && in.FakeScenario == "" {
		in.FakeScenario = "response"
	}
	return Fingerprint("tasks.attempts.start", struct {
		TaskID string
		Input  domain.StartAttemptInput
	}{id, in})
}
func ResponseAcceptanceFingerprint(id string, in domain.ResponseAcceptanceInput) string {
	return Fingerprint("tasks.response.accept", struct {
		TaskID string
		Input  domain.ResponseAcceptanceInput
	}{id, in})
}
func StopFingerprint(id, requestID string) string {
	return Fingerprint("attempts.stop", struct {
		AttemptID string
		RequestID string
	}{id, requestID})
}
func ShutdownFingerprint(ownerID, requestID string) string {
	return Fingerprint("service.stop", struct {
		OwnerInstanceID string
		RequestID       string
	}{ownerID, requestID})
}

func normalizeStart(in domain.StartTaskInput) domain.StartTaskInput {
	if in.ExecutionMode == "" {
		in.ExecutionMode = "native"
	}
	if in.ExecutionMode == "fake" && in.FakeScenario == "" {
		in.FakeScenario = "response"
	}
	if in.TimeoutSeconds == nil {
		timeout := 1800
		in.TimeoutSeconds = &timeout
	}
	if in.Pins == nil {
		in.Pins = &domain.ExecutionPins{}
	}
	resolved, _ := domain.ResolveAcceptance(in.Acceptance)
	allowed := resolved.AllowCandidatePreparation
	in.Acceptance = &domain.AcceptanceInput{Deliverable: resolved.Deliverable, ConditionsText: resolved.ConditionsText, RequiredEvidence: resolved.RequiredEvidence, AllowCandidatePreparation: &allowed}
	return in
}
