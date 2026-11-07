package domain

// GraphContributorVectorDigest uses the same typed, sorted, nil-normalized
// contributor representation as the input/final seals. It never mutates the
// caller's vector and adds no wrapper or self-referential field.
func GraphContributorVectorDigest(value []GraphContributorBinding) (string, error) {
	return graphSeal(graphSealContributors(value))
}
