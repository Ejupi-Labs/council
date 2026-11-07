package workspace

import (
	"path/filepath"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Observed inventories have no artifact IDs: semantic state is independent from
// retained ownership. Closed-artifact verification remains a separate obligation.
func graphValidateSemanticManifest(manifest Manifest) error {
	if len(manifest.Entries) > MaxEntries || len(manifest.Index) > MaxEntries || len(manifest.Exclusions) > MaxEntries {
		return ErrBlocked
	}
	previous := ""
	var total int64
	for _, entry := range manifest.Entries {
		if !safePath(entry.Path) || excluded(entry.Path) != "" || entry.Path <= previous || entry.Size < 0 || entry.Size > MaxFileBytes || entry.Mode&^0777 != 0 {
			return ErrBlocked
		}
		previous = entry.Path
		switch entry.Type {
		case "file":
			if !domain.ValidSHA256(entry.SHA256) || entry.Target != "" {
				return ErrBlocked
			}
			total += entry.Size
			if total > MaxTotalBytes {
				return ErrBlocked
			}
		case "symlink":
			resolved := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(entry.Path), entry.Target)))
			if entry.Target == "" || filepath.IsAbs(entry.Target) || resolved != "." && !safePath(resolved) || excluded(resolved) != "" || entry.Mode != 0 || entry.SHA256 != "" || entry.Size != int64(len(entry.Target)) {
				return ErrBlocked
			}
		default:
			return ErrBlocked
		}
	}
	previous = ""
	for _, entry := range manifest.Index {
		if !safePath(entry.Path) || excluded(entry.Path) != "" || entry.Path <= previous || !graphObjectID(entry.Object) || entry.Mode != "100644" && entry.Mode != "100755" && entry.Mode != "120000" {
			return ErrBlocked
		}
		previous = entry.Path
	}
	previous = ""
	for _, entry := range manifest.Exclusions {
		if !safePath(entry.Path) || entry.Path <= previous || entry.Reason == "" {
			return ErrBlocked
		}
		previous = entry.Path
	}
	return nil
}
