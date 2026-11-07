package workflow

import "github.com/Ejupi-Labs/council/v2/internal/domain"

// GraphFinalPlanFacts is internal controller input. The engine and settlement
// store must verify the exact final candidate and cumulative basis before
// supplying it; public requests never decode this type.
type GraphFinalPlanFacts struct {
	VerificationBindingRef    domain.GraphArtifactRef
	DeclaredFileFinal         bool
	CurrentSubject            domain.CandidateBinding
	HasCumulativeWorkingDelta bool
	HasCumulativeIndexDelta   bool
	CumulativeDirectProven    bool
}

func qualifyingGraphFinalPlanFacts(in PlanInput) *GraphFinalPlanFacts {
	facts := in.GraphFinal
	if in.ResponseOnly || facts == nil || !facts.DeclaredFileFinal || !facts.HasCumulativeWorkingDelta && !facts.HasCumulativeIndexDelta {
		return nil
	}
	if domain.ValidateGraphArtifactRef(facts.VerificationBindingRef) != nil {
		return nil
	}
	size, invalid := domain.ParseCount(facts.VerificationBindingRef.ByteLength)
	if invalid != nil || size == 0 {
		return nil
	}
	subject := facts.CurrentSubject
	for _, id := range []string{subject.RootSourceSnapshotID, subject.CandidateID, subject.SourceSnapshotID, subject.ManifestArtifactID, subject.IndexArtifactID} {
		if !domain.ValidID(id) {
			return nil
		}
	}
	for _, hash := range []string{subject.ManifestSHA256, subject.IndexSHA256, subject.ConditionsSHA256} {
		if !domain.ValidSHA256(hash) {
			return nil
		}
	}
	if subject.ConditionsSHA256 != in.Authority.ConditionsSHA256 {
		return nil
	}
	return facts
}
