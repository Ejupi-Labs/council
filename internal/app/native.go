package app

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/discovery"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/project"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

func newNativeService(db *store.Store) *Service {
	return &Service{
		Store: db,
		InspectProject: func(ctx context.Context, selectedPath, basePath string) (domain.ResolvedProject, error) {
			inspection, e := project.Inspect(ctx, selectedPath, basePath)
			if e != nil {
				return domain.ResolvedProject{}, e
			}
			o := inspection.Observation
			return domain.ResolvedProject{PhysicalIdentity: inspection.Identity, Observation: domain.ProjectObservation{ResolvedPath: o.ResolvedPath, RepositoryRoot: o.RepositoryRoot, Branch: o.Branch, Commit: o.Commit, ObservedAt: o.ObservedAt}}, nil
		},
		ValidateProjectIdentity: project.ValidateIdentity,
		InspectAgents:           discovery.Discover,
	}
}
