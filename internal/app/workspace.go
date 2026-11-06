package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

type openRequest struct {
	RequestID    string `json:"requestId"`
	SelectedPath string `json:"selectedPath"`
	BasePath     string `json:"basePath"`
}
type policyRequest struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision string `json:"expectedRevision"`
	Mode             string `json:"mode"`
}

func requestIdentity(id string) string {
	if domain.ValidID(id) {
		return id
	}
	return domain.NewID()
}
func serviceFailure(id string, err error) domain.Result {
	var e *domain.Error
	if errors.As(err, &e) {
		return domain.Failure(id, e)
	}
	return domain.Failure(id, domain.Fail("internal_error", "The operation could not complete. Report this request identity."))
}
func (s *Service) Execute(ctx context.Context, operation string, p Params) domain.Result {
	if executionOperation(operation) {
		return s.executeManaged(ctx, operation, p)
	}
	switch operation {
	case "projects.open":
		return s.openProject(ctx, p)
	case "projects.list":
		return s.listProjects(ctx, p)
	case "projects.get":
		return s.getProject(ctx, p)
	case "policy.get":
		id := domain.NewID()
		if len(p.Body) > 0 {
			return domain.Failure(id, domain.Invalid("empty GET body"))
		}
		if e := domain.ValidateQuery(p.Query); e != nil {
			return domain.Failure(id, e)
		}
		v, e := s.Store.Policy(ctx)
		if e != nil {
			return serviceFailure(id, e)
		}
		return domain.Success(200, id, v, nil)
	case "policy.set":
		return s.setPolicy(ctx, p)
	case "agents.list", "agents.refresh":
		id := domain.NewID()
		if len(p.Body) > 0 {
			return domain.Failure(id, domain.Invalid("empty GET body"))
		}
		if e := domain.ValidateQuery(p.Query); e != nil {
			return domain.Failure(id, e)
		}
		v, e := s.InspectAgents(ctx)
		if e != nil {
			return serviceFailure(id, e)
		}
		return domain.Success(200, id, v, nil)
	case "drafts.create":
		return s.createDraft(ctx, p)
	case "drafts.update":
		return s.updateDraft(ctx, p)
	case "drafts.get":
		return s.getDraft(ctx, p)
	case "drafts.list":
		return s.listDrafts(ctx, p)
	default:
		return domain.Failure(domain.NewID(), domain.Fail("not_found", "The requested operation does not exist. Use a documented foundation operation."))
	}
}
func validPath(s string) bool { return s != "" && utf8.ValidString(s) && !strings.ContainsRune(s, 0) }
func (s *Service) openProject(ctx context.Context, p Params) domain.Result {
	var input openRequest
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
	if !validPath(input.SelectedPath) || !validPath(input.BasePath) || !filepath.IsAbs(input.BasePath) {
		return domain.Failure(id, domain.Invalid("selectedPath and absolute basePath"))
	}
	fp := store.Fingerprint("projects.open", input)
	previous, e := s.Store.Replay(ctx, id, "projects.open", fp)
	if e != nil {
		return serviceFailure(id, e)
	}
	if previous != nil {
		return *previous
	}
	resolved, e := s.InspectProject(ctx, input.SelectedPath, input.BasePath)
	if e != nil {
		return serviceFailure(id, e)
	}
	saved, e := s.Store.ProjectByIdentity(ctx, resolved.PhysicalIdentity)
	if e != nil {
		return serviceFailure(id, e)
	}
	if saved != nil {
		if e = s.ValidateProjectIdentity(saved.ResolvedPath, resolved.PhysicalIdentity); e != nil {
			return serviceFailure(id, e)
		}
	}
	return s.Store.OpenProject(ctx, id, fp, domain.OpenSelection{SelectedPath: input.SelectedPath, BasePath: input.BasePath}, resolved)
}
func (s *Service) listProjects(ctx context.Context, p Params) domain.Result {
	id := domain.NewID()
	if len(p.Body) > 0 {
		return domain.Failure(id, domain.Invalid("empty GET body"))
	}
	if e := domain.ValidateQuery(p.Query, "limit", "cursor"); e != nil {
		return domain.Failure(id, e)
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
	page, err := s.Store.Projects(ctx, limit, p.Query["cursor"])
	if err != nil {
		return serviceFailure(id, err)
	}
	return domain.Success(200, id, page, nil)
}
func (s *Service) getProject(ctx context.Context, p Params) domain.Result {
	id := domain.NewID()
	if len(p.Body) > 0 {
		return domain.Failure(id, domain.Invalid("empty GET body"))
	}
	if e := domain.ValidateQuery(p.Query); e != nil {
		return domain.Failure(id, e)
	}
	if !domain.ValidID(p.ID) {
		return domain.Failure(id, domain.Invalid("project identity"))
	}
	saved, identity, e := s.Store.Project(ctx, p.ID)
	if e != nil {
		return serviceFailure(id, e)
	}
	data := domain.ProjectGet{Project: saved}
	if e = s.ValidateProjectIdentity(saved.ResolvedPath, identity); e != nil {
		data.ObservationError = safeObservationError(e)
		return domain.Success(200, id, data, nil)
	}
	resolved, e := s.InspectProject(ctx, saved.ResolvedPath, filepath.Dir(saved.ResolvedPath))
	if e != nil {
		data.ObservationError = safeObservationError(e)
	} else if resolved.PhysicalIdentity != identity {
		data.ObservationError = domain.Fail("project_identity_changed", "This folder has a different physical identity. Restore the original project folder.")
	} else {
		data.Observation = &resolved.Observation
	}
	return domain.Success(200, id, data, nil)
}
func safeObservationError(err error) *domain.Error {
	var e *domain.Error
	if errors.As(err, &e) {
		return e
	}
	return domain.Fail("project_unavailable", "The saved project cannot be inspected. Restore access to its original folder and retry.")
}
func (s *Service) setPolicy(ctx context.Context, p Params) domain.Result {
	var input policyRequest
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
	if _, e := domain.ParseRevision(input.ExpectedRevision); e != nil {
		return domain.Failure(id, e)
	}
	if input.Mode != "autonomous" && input.Mode != "supervised" {
		return domain.Failure(id, domain.Invalid("policy mode autonomous or supervised"))
	}
	return s.Store.SetPolicy(ctx, id, store.Fingerprint("policy.set", input), input.ExpectedRevision, input.Mode)
}
