package routing

import "github.com/Ejupi-Labs/council/v2/internal/domain"

// Reservations are actual Council ownership, independent of external reports.
// They restrict graph admission; ordinary routing keeps its captured queue target.
func managedReservationOccupied(snapshot domain.RoutingSnapshot, contextID string) bool {
	claims := snapshot.ManagedOccupancy.ActiveClaims
	if len(claims) >= domain.MaxManagedSlots {
		return true
	}
	var candidateAccount *string
	for _, link := range snapshot.Links {
		if link.ContextID == contextID {
			candidateAccount = link.DeclaredAccountID
		}
	}
	for _, claim := range claims {
		if claim.LegacyBarrier {
			return true
		}
		if claim.ContextID != nil && *claim.ContextID == contextID {
			return true
		}
		if candidateAccount == nil {
			continue
		}
		if claim.CapturedDeclaredAccountID != nil && *claim.CapturedDeclaredAccountID == *candidateAccount {
			return true
		}
		for _, link := range snapshot.Links {
			if claim.ContextID != nil && link.ContextID == *claim.ContextID && link.DeclaredAccountID != nil && *link.DeclaredAccountID == *candidateAccount {
				return true
			}
		}
	}
	return false
}
