package workspace

import "github.com/Ejupi-Labs/council/v2/internal/domain"

// Complete access is an explicit launch plan, not proof that a native process has
// already read the artifact. Source scopes remain separate and are never clipped.
func graphCompleteContextAccess(required []domain.GraphArtifactRef, access []domain.GraphProvidedAccess) error {
	for _, original := range required {
		covered := false
		for _, provided := range access {
			if provided.ArtifactRef == original && len(provided.Paths) == 0 && (provided.Mode == "file" || provided.Mode == "inline") {
				covered = true
				break
			}
		}
		if !covered {
			return ErrBlocked
		}
	}
	return nil
}
