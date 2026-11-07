package domain

// GraphDeliveryFacts is private controller evidence for one exact delivery
// transaction. Registered tuples are checked in SQL; physical verification is
// performed by the controller before supplying these fresh bounded proofs.
type GraphDeliveryFacts struct {
	Binding           GraphDeliveryBinding
	BindingRef        GraphArtifactRef
	VerifiedArtifacts []VerifiedResponse
}

// GraphDeliveryScopeDigest seals the exact captured typed scope in declared
// field order. The full private destination scope is never copied into graph
// delivery artifacts; only this digest and the public graph root are retained.
func GraphDeliveryScopeDigest(scope DeliveryScope) (string, error) {
	return graphSeal(scope)
}
