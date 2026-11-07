package engine

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/project"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

// createGraph registers intent and its real local preparation. Native node
// admission is a later current-owner transition after exact input retention.
func (e *Engine) createGraph(ctx context.Context, in domain.GraphStartInput) domain.Result {
	if invalid := domain.ValidateGraphStartInput(in); invalid != nil {
		return e.failure(in.RequestID, invalid)
	}
	if prior, err := e.db.Replay(ctx, in.RequestID, "graphs.start", store.GraphStartFingerprint(in)); err != nil {
		return e.failure(in.RequestID, err)
	} else if prior != nil {
		return *prior
	}
	e.prepareGate.Lock()
	defer e.prepareGate.Unlock()
	facts, err := e.captureGraphCreateFacts(ctx, in.ProjectID)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		return e.failure(in.RequestID, domain.Fail("dispatch_fenced", "Resolve retained ownership or persistence uncertainty before registering graph work."))
	}
	result := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, in, facts)
	e.guardMutationLocked(result)
	e.control.Unlock()
	if result.Envelope.Error == nil {
		e.signal()
	}
	return result
}

func (e *Engine) captureGraphCreateFacts(ctx context.Context, projectID string) (domain.GraphCreateFacts, error) {
	var zero domain.GraphCreateFacts
	saved, identity, err := e.db.Project(ctx, projectID)
	if err != nil {
		return zero, err
	}
	if err = project.ValidateIdentity(saved.ResolvedPath, identity); err != nil {
		return zero, domain.Fail("project_identity_changed", "The selected project directory changed. Restore its captured identity before creating a graph.")
	}
	observed, err := project.Inspect(ctx, saved.ResolvedPath, saved.ResolvedPath)
	if err != nil {
		return zero, err
	}
	if observed.Identity != identity {
		return zero, domain.Fail("project_identity_changed", "The selected project directory changed while graph prerequisites were inspected.")
	}
	if observed.Observation.Commit == nil {
		return zero, domain.Fail("prerequisite_missing", "Commit an initial repository base before creating an isolated task graph.")
	}
	repository, err := project.Inspect(ctx, observed.Observation.RepositoryRoot, observed.Observation.RepositoryRoot)
	if err != nil {
		return zero, err
	}
	if repository.Observation.Commit == nil || *repository.Observation.Commit != *observed.Observation.Commit {
		return zero, domain.Fail("project_identity_changed", "The repository base changed during graph prerequisite inspection.")
	}
	selected, err := filepath.Rel(repository.Observation.ResolvedPath, observed.Observation.ResolvedPath)
	if err != nil || filepath.IsAbs(selected) || selected == ".." || strings.HasPrefix(filepath.ToSlash(selected), "../") {
		return zero, domain.Fail("project_identity_changed", "The selected project is no longer inside its captured repository.")
	}
	var branch *string
	if observed.Observation.Branch != nil {
		value := "refs/heads/" + *observed.Observation.Branch
		branch = &value
	}
	return domain.GraphCreateFacts{ProjectIdentity: identity, RepositoryIdentity: repository.Identity, SelectedRelativePath: filepath.ToSlash(selected), BaseCommit: *observed.Observation.Commit, Branch: branch}, nil
}
