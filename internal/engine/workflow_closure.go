package engine

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// Member closure is verified outside SQL. The bounded top-level proof binds the
// immutable manifest/hash and its exact member IDs; SQL never reads a filesystem
// or expands one source tree into unbounded per-member queries/transaction data.
func (e *Engine) verifyWorkflowClosure(ctx context.Context, manifests ...workspace.Manifest) error {
	verified := map[string]evidence.Metadata{}
	for _, manifest := range manifests {
		var total int64
		paths := map[string]bool{}
		for _, entry := range manifest.Entries {
			if !domain.CanonicalWorkflowPath(entry.Path) || paths[entry.Path] || entry.Size < 0 || entry.Size > workspace.MaxFileBytes {
				return evidence.ErrCorrupt
			}
			paths[entry.Path] = true
			if entry.Type == "symlink" {
				continue
			}
			if entry.Type != "file" || !domain.ValidID(entry.ArtifactID) || !domain.ValidSHA256(entry.SHA256) {
				return evidence.ErrCorrupt
			}
			total += entry.Size
			if total > workspace.MaxTotalBytes {
				return evidence.ErrCorrupt
			}
			meta, found := verified[entry.ArtifactID]
			if !found {
				var err error
				meta, err = e.evidence.VerifyClosed(ctx, entry.ArtifactID)
				if err != nil {
					return err
				}
				verified[entry.ArtifactID] = meta
			}
			if meta.Kind != "source_file" || !meta.Closed || meta.Completeness != "complete" || meta.SHA256 == nil || *meta.SHA256 != entry.SHA256 || meta.ByteLength != domain.Decimal(entry.Size) {
				return evidence.ErrCorrupt
			}
		}
	}
	return nil
}
