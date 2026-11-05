package app

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

type createDraftRequest struct {
	RequestID    string `json:"requestId"`
	ProjectID    string `json:"projectId"`
	OriginalText string `json:"originalText"`
}
type updateDraftRequest struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision string `json:"expectedRevision"`
	OriginalText     string `json:"originalText"`
	UseCurrentPolicy *bool  `json:"useCurrentPolicy,omitzero"`
}

func (s *Service) createDraft(ctx context.Context, p Params) domain.Result {
	var input createDraftRequest
	if e := domain.StrictDecode(p.Body, &input); e != nil {
		return domain.Failure(domain.MutationIdentity(p.Body), e)
	}
	id := requestIdentity(input.RequestID)
	if !domain.ValidID(input.RequestID) {
		return domain.Failure(id, domain.Invalid("requestId"))
	}
	if e := domain.ValidateQuery(p.Query); e != nil {
		return domain.Failure(id, e)
	}
	if !domain.ValidID(input.ProjectID) {
		return domain.Failure(id, domain.Invalid("project identity"))
	}
	if e := domain.ValidateText(input.OriginalText); e != nil {
		return domain.Failure(id, e)
	}
	return s.Store.CreateDraft(ctx, id, store.Fingerprint("drafts.create", input), input.ProjectID, input.OriginalText)
}
func (s *Service) updateDraft(ctx context.Context, p Params) domain.Result {
	var input updateDraftRequest
	if e := domain.StrictDecode(p.Body, &input); e != nil {
		return domain.Failure(domain.MutationIdentity(p.Body), e)
	}
	id := requestIdentity(input.RequestID)
	if !domain.ValidID(input.RequestID) {
		return domain.Failure(id, domain.Invalid("requestId"))
	}
	if e := domain.ValidateQuery(p.Query); e != nil {
		return domain.Failure(id, e)
	}
	if !domain.ValidID(p.ID) {
		return domain.Failure(id, domain.Invalid("draft identity"))
	}
	if _, e := domain.ParseRevision(input.ExpectedRevision); e != nil {
		return domain.Failure(id, e)
	}
	if e := domain.ValidateText(input.OriginalText); e != nil {
		return domain.Failure(id, e)
	}
	useCurrent := input.UseCurrentPolicy != nil && *input.UseCurrentPolicy
	// The immutable draft identity belongs to the operation input as well as its body.
	fp := store.Fingerprint("drafts.update", struct {
		ID    string             `json:"id"`
		Input updateDraftRequest `json:"input"`
	}{p.ID, input})
	return s.Store.UpdateDraft(ctx, id, fp, p.ID, input.ExpectedRevision, input.OriginalText, useCurrent)
}
func (s *Service) getDraft(ctx context.Context, p Params) domain.Result {
	id := domain.NewID()
	if len(p.Body) > 0 {
		return domain.Failure(id, domain.Invalid("empty GET body"))
	}
	if e := domain.ValidateQuery(p.Query, "revision"); e != nil {
		return domain.Failure(id, e)
	}
	if !domain.ValidID(p.ID) {
		return domain.Failure(id, domain.Invalid("draft identity"))
	}
	if raw, ok := p.Query["revision"]; ok {
		if _, e := domain.ParseRevision(raw); e != nil {
			return domain.Failure(id, e)
		}
	}
	draft, e := s.Store.Draft(ctx, p.ID, p.Query["revision"])
	if e != nil {
		return serviceFailure(id, e)
	}
	return domain.Success(200, id, draft, nil)
}
func (s *Service) listDrafts(ctx context.Context, p Params) domain.Result {
	id := domain.NewID()
	if len(p.Body) > 0 {
		return domain.Failure(id, domain.Invalid("empty GET body"))
	}
	if e := domain.ValidateQuery(p.Query, "projectId", "limit", "cursor"); e != nil {
		return domain.Failure(id, e)
	}
	if projectID, ok := p.Query["projectId"]; ok && !domain.ValidID(projectID) {
		return domain.Failure(id, domain.Invalid("project identity"))
	}
	if value, present := p.Query["limit"]; present && value == "" {
		return domain.Failure(id, domain.Invalid("limit from 1 through 100"))
	}
	if value, present := p.Query["cursor"]; present && value == "" {
		return domain.Failure(id, domain.Fail("invalid_cursor", "Supply the complete list cursor or omit it to start a list."))
	}
	limit, e := domain.ParseLimit(p.Query["limit"])
	if e != nil {
		return domain.Failure(id, e)
	}
	page, err := s.Store.Drafts(ctx, p.Query["projectId"], limit, p.Query["cursor"])
	if err != nil {
		return serviceFailure(id, err)
	}
	return domain.Success(200, id, page, nil)
}
