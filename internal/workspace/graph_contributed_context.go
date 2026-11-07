package workspace

import (
	json "encoding/json/v2"
	"sort"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// A selected contribution already declares the context authorized for this
// edge. Retention may not omit it or promote its scope. This is prospective
// complete access, not evidence that a native worker has inspected the bytes.
func graphSelectedContextCoverage(contributions []domain.AcceptedContribution, constraints []domain.GraphScopedReference, evidence []domain.GraphContributedEvidence, access []domain.GraphProvidedAccess) error {
	constraintKeys := map[string]bool{}
	for _, source := range constraints {
		key, err := graphScopedContextKey(source)
		if err != nil {
			return err
		}
		constraintKeys[key] = true
	}
	evidenceKeys := map[string]bool{}
	for _, source := range evidence {
		key, err := graphEvidenceContextKey(source)
		if err != nil {
			return err
		}
		evidenceKeys[key] = true
	}
	complete := func(ref domain.GraphArtifactRef, scope string) bool {
		for _, supplied := range access {
			if supplied.ArtifactRef == ref && supplied.Scope == scope && len(supplied.Paths) == 0 && (supplied.Mode == "file" || supplied.Mode == "inline") {
				return true
			}
		}
		return false
	}
	for _, contribution := range contributions {
		for _, source := range contribution.ConstraintRefs {
			key, err := graphScopedContextKey(source)
			if err != nil {
				return err
			}
			if !constraintKeys[key] || !complete(source.ArtifactRef, source.AuthorityScope) {
				return ErrBlocked
			}
		}
		for _, source := range contribution.EvidenceRefs {
			key, err := graphEvidenceContextKey(source)
			if err != nil {
				return err
			}
			if !evidenceKeys[key] || !complete(source.ArtifactRef, source.Scope) {
				return ErrBlocked
			}
		}
	}
	return nil
}

func graphScopedContextKey(source domain.GraphScopedReference) (string, error) {
	source.Paths = append([]string{}, source.Paths...)
	sort.Strings(source.Paths)
	body, err := json.Marshal(source)
	return string(body), err
}

func graphEvidenceContextKey(source domain.GraphContributedEvidence) (string, error) {
	source.Paths = append([]string{}, source.Paths...)
	sort.Strings(source.Paths)
	body, err := json.Marshal(source)
	return string(body), err
}
