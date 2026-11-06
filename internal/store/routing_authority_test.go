package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
)

type routingAuthorityCase struct {
	classification, oldProfile, nextProfile string
	oldPrice, nextPrice                     *domain.ReferencePriceInput
	disabled, enableAfterBlock              []string
	replacement, escalation                 bool
}

func routingAuthorityPrice(input, output string) *domain.ReferencePriceInput {
	return &domain.ReferencePriceInput{Currency: "USD", Unit: "per_million_tokens", Basis: "api_list", InputPerMillion: ptr(input), OutputPerMillion: ptr(output), SourceLabel: "CONTROLLED local reference, not a charge", ObservedAt: domain.Now()}
}
func routingAuthorityProfile(t *testing.T, s *Store, id string) domain.TargetProfile {
	t.Helper()
	page, e := s.RoutingProfiles(context.Background(), "")
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range page.Items {
		if p.ID == id {
			return p
		}
	}
	t.Fatal("missing fixed test profile", id)
	return domain.TargetProfile{}
}
func routingAuthorityUpdateProfile(t *testing.T, s *Store, id string, price *domain.ReferencePriceInput, enabled *bool) {
	t.Helper()
	p := routingAuthorityProfile(t, s, id)
	r := s.UpdateRoutingProfile(context.Background(), id, domain.ProfileUpdateInput{RequestID: domain.NewID(), ExpectedRevision: p.Revision, ReferencePrice: price, Enabled: enabled})
	if r.Status != 200 {
		t.Fatal(r)
	}
}
func routingAuthorityBlocked(t *testing.T, s *Store, tc routingAuthorityCase) (string, domain.Admission, domain.RoutingFacts) {
	t.Helper()
	ctx := context.Background()
	owner := domain.NewID()
	if e := s.RecoverOwner(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if tc.oldPrice != nil {
		routingAuthorityUpdateProfile(t, s, tc.oldProfile, tc.oldPrice, nil)
	}
	if tc.nextPrice != nil {
		routingAuthorityUpdateProfile(t, s, tc.nextProfile, tc.nextPrice, nil)
	}
	no, yes := false, true
	for _, id := range tc.disabled {
		routingAuthorityUpdateProfile(t, s, id, nil, &no)
	}
	p := openFixture(t, s, "routing-authority")
	in := taskInput(p.ID)
	categories := []string{"pre_launch_ineligible"}
	in.Routing = &domain.RoutingOptions{Suitability: tc.classification, Continuation: &domain.ContinuationInput{MaxAutomaticAdmissions: "1", Categories: &categories, AllowReplacement: &tc.replacement, AllowEscalation: &tc.escalation}}
	facts := wrapperRoutingFacts("fake")
	r := s.CreateTaskWithRouting(ctx, owner, in, facts)
	if r.Status != 201 {
		t.Fatal(r)
	}
	first := r.Envelope.Data.(domain.Admission)
	if first.Attempt.ResolvedTarget == nil || first.Attempt.ResolvedTarget.ProfileID == nil || *first.Attempt.ResolvedTarget.ProfileID != tc.oldProfile {
		t.Fatal("fixture did not capture the intended original target", first)
	}
	r = s.UpdateRoutingContext(ctx, "codex-native", domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: &no})
	if r.Status != 200 {
		t.Fatal(r)
	}
	old, e := s.DispatchRouting(ctx, first.Attempt.ID, facts)
	if e != nil || old.Outcome == nil || *old.Outcome != "blocked" || old.LaunchIntent || old.Cleanup.State != "verified" {
		t.Fatal("fixture lacks a real settled pre-launch block", old, e)
	}
	for _, id := range tc.enableAfterBlock {
		routingAuthorityUpdateProfile(t, s, id, nil, &yes)
	}
	var proofs int
	if e = s.db.QueryRow("SELECT count(*) FROM routing_continuation_proofs WHERE attempt_id=?", old.ID).Scan(&proofs); e != nil || proofs != 1 {
		t.Fatal("fixture lacks positive category-A proof", proofs, e)
	}
	view, e := s.TaskRouting(ctx, first.Task.ID)
	if e != nil || view.Authority == nil {
		t.Fatal(view, e)
	}
	snapshot, e := s.RoutingSnapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	snapshot.Policy = view.Authority.PolicySnapshot
	selection, selectionErr := routing.Select(domain.RoutingSelectionInput{Snapshot: snapshot, Pins: first.Version.Pins, Options: view.Authority.OptionSnapshot, Assessment: view.Authority.SuitabilityAssessment, Facts: facts.Contexts, Now: domain.Now()})
	if selectionErr != nil || selection.SelectedTarget.ProfileID == nil || *selection.SelectedTarget.ProfileID != tc.nextProfile {
		t.Fatal("the alternative must be eligible before testing authority refusal", selection, selectionErr)
	}
	return owner, first, facts
}
func routingAuthorityNoEffect(t *testing.T, s *Store, owner string, first domain.Admission, facts domain.RoutingFacts) {
	t.Helper()
	ctx := context.Background()
	before, e := s.TaskBudget(ctx, first.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	next, e := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
	if e != nil || next != nil {
		t.Fatal("unauthorized replacement acquired a managed admission", next, e)
	}
	after, e := s.TaskBudget(ctx, first.Task.ID)
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("refused continuation changed exact budget authority", before, after, e)
	}
	attempts, e := s.Attempts(ctx, first.Task.ID, 100, "")
	if e != nil || len(attempts.Items) != 1 || attempts.Items[0].ID != first.Attempt.ID {
		t.Fatal("refused continuation created a worker identity", attempts, e)
	}
	ledger, e := s.BudgetLedger(ctx, first.Task.ID, 100, "")
	if e != nil {
		t.Fatal(e)
	}
	admissions := 0
	for _, entry := range ledger.Items {
		if entry.Kind == "admission" {
			admissions++
		}
	}
	if admissions != 1 {
		t.Fatal("refused continuation spent a hidden admission unit", admissions)
	}
}

func TestRoutingContinuationRequiresCapturedReplacementAndEscalationAuthority(t *testing.T) {
	for _, tc := range []struct {
		name string
		routingAuthorityCase
	}{
		{"replacement_denied", routingAuthorityCase{classification: "routine", oldProfile: "codex-luna-economy", nextProfile: "claude-haiku-economy", oldPrice: routingAuthorityPrice("10", "10"), nextPrice: routingAuthorityPrice("10", "10"), replacement: false, escalation: true}},
		{"missing_prices", routingAuthorityCase{classification: "routine", oldProfile: "codex-luna-economy", nextProfile: "claude-haiku-economy", replacement: true}},
		{"higher_price", routingAuthorityCase{classification: "routine", oldProfile: "codex-luna-economy", nextProfile: "claude-haiku-economy", oldPrice: routingAuthorityPrice("10", "10"), nextPrice: routingAuthorityPrice("20", "20"), replacement: true}},
		{"crossed_prices", routingAuthorityCase{classification: "routine", oldProfile: "codex-luna-economy", nextProfile: "claude-haiku-economy", oldPrice: routingAuthorityPrice("10", "30"), nextPrice: routingAuthorityPrice("20", "20"), replacement: true}},
		{"both_tiers_unknown", routingAuthorityCase{classification: "unknown", oldProfile: "codex-native-default", nextProfile: "claude-native-default", oldPrice: routingAuthorityPrice("10", "10"), nextPrice: routingAuthorityPrice("10", "10"), replacement: true}},
		{"higher_tier_even_if_cheaper", routingAuthorityCase{classification: "routine", oldProfile: "codex-luna-economy", nextProfile: "claude-sonnet-general", oldPrice: routingAuthorityPrice("10", "10"), nextPrice: routingAuthorityPrice("1", "1"), disabled: []string{"claude-haiku-economy"}, replacement: true}},
		{"old_tier_unknown_new_known_cheaper", routingAuthorityCase{classification: "routine", oldProfile: "codex-native-default", nextProfile: "claude-haiku-economy", oldPrice: routingAuthorityPrice("10", "10"), nextPrice: routingAuthorityPrice("1", "1"), disabled: []string{"codex-luna-economy", "codex-sol-general", "claude-haiku-economy", "claude-sonnet-general"}, enableAfterBlock: []string{"claude-haiku-economy"}, replacement: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			owner, first, facts := routingAuthorityBlocked(t, s, tc.routingAuthorityCase)
			routingAuthorityNoEffect(t, s, owner, first, facts)
		})
	}
}
func TestRoutingKnownNonEscalatingReplacementRemainsUsable(t *testing.T) {
	for _, tc := range []struct {
		name string
		routingAuthorityCase
	}{
		{"equal_price", routingAuthorityCase{classification: "routine", oldProfile: "codex-luna-economy", nextProfile: "claude-haiku-economy", oldPrice: routingAuthorityPrice("10", "10"), nextPrice: routingAuthorityPrice("10", "10"), replacement: true}},
		{"lower_price", routingAuthorityCase{classification: "routine", oldProfile: "codex-luna-economy", nextProfile: "claude-haiku-economy", oldPrice: routingAuthorityPrice("20", "20"), nextPrice: routingAuthorityPrice("10", "10"), disabled: []string{"claude-haiku-economy"}, enableAfterBlock: []string{"claude-haiku-economy"}, replacement: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			owner, first, facts := routingAuthorityBlocked(t, s, tc.routingAuthorityCase)
			next, e := s.ContinueRouting(context.Background(), owner, first.Attempt.ID, facts)
			if e != nil || next == nil || next.RoutingDecision.Kind != "automatic" || next.Budget.ConsumedAdmissions != "2" || next.Budget.AutomaticAdmissions != "1" {
				t.Fatal("known equal/lower replacement was unnecessarily forbidden", next, e)
			}
		})
	}
}
func TestRoutingCurrentPriceCannotRewriteCapturedEscalationBasis(t *testing.T) {
	s := testStore(t)
	tc := routingAuthorityCase{classification: "routine", oldProfile: "codex-luna-economy", nextProfile: "claude-haiku-economy", oldPrice: routingAuthorityPrice("10", "10"), nextPrice: routingAuthorityPrice("20", "20"), replacement: true}
	owner, first, facts := routingAuthorityBlocked(t, s, tc)
	routingAuthorityUpdateProfile(t, s, tc.oldProfile, routingAuthorityPrice("100", "100"), nil)
	view, e := s.TaskRouting(context.Background(), first.Task.ID)
	if e != nil || view.LatestDecision == nil {
		t.Fatal(view, e)
	}
	var oldPrice *domain.ReferencePrice
	for _, candidate := range view.LatestDecision.CandidateAssessments {
		if candidate.ProfileID != nil && *candidate.ProfileID == tc.oldProfile {
			oldPrice = candidate.ProfileSnapshot.ReferencePrice
		}
	}
	current := routingAuthorityProfile(t, s, tc.oldProfile)
	if oldPrice == nil || oldPrice.InputPerMillion == nil || *oldPrice.InputPerMillion != "10" || current.ReferencePrice == nil || *current.ReferencePrice.InputPerMillion != "100" {
		t.Fatal("fixture does not distinguish captured10 from mutable100", oldPrice, current)
	}
	// New20 is cheaper than today's old100, but more expensive than admitted
	// old10. Consulting mutable configuration would wrongly authorize this case.
	routingAuthorityNoEffect(t, s, owner, first, facts)
}
func TestRoutingGrantAndGuidanceCannotRearmOldProofButExplicitStartWorks(t *testing.T) {
	for _, action := range []string{"grant", "guidance"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			tc := routingAuthorityCase{classification: "routine", oldProfile: "codex-luna-economy", nextProfile: "claude-haiku-economy", replacement: true, escalation: true}
			owner, first, facts := routingAuthorityBlocked(t, s, tc)
			detail, e := s.Task(ctx, first.Task.ID)
			if e != nil {
				t.Fatal(e)
			}
			budget, e := s.TaskBudget(ctx, first.Task.ID)
			if e != nil {
				t.Fatal(e)
			}
			if action == "grant" {
				r := s.AmendBudget(ctx, first.Task.ID, domain.BudgetAmendmentInput{RequestID: domain.NewID(), Action: "grant", ExpectedRevision: detail.Task.Revision, ExpectedBudgetRevision: budget.Budget.Revision, AdditionalAdmissions: "1"})
				if r.Status != 200 {
					t.Fatal(r)
				}
			} else {
				r := s.UpdateGuidanceWithRouting(ctx, first.Task.ID, domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, GuidanceText: "Preserve these explicit revised instructions."}, facts)
				if r.Status != 200 {
					t.Fatal(r)
				}
			}
			routingAuthorityNoEffect(t, s, owner, first, facts)
			detail, e = s.Task(ctx, first.Task.ID)
			if e != nil {
				t.Fatal(e)
			}
			r := s.StartAttemptWithRouting(ctx, owner, first.Task.ID, domain.StartAttemptInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, TaskVersion: detail.Task.CurrentVersion, ExecutionMode: "fake"}, facts)
			if r.Status != 201 {
				t.Fatal("explicit Start must remain usable under new authority", r)
			}
			next := r.Envelope.Data.(domain.Admission)
			if next.Attempt.ID == first.Attempt.ID || next.Attempt.ConnectionID != "claude-native" || next.RoutingDecision.Kind != "manual" || next.Budget.ConsumedAdmissions != "2" || next.Budget.AutomaticAdmissions != "0" || *next.Budget.CurrentWindow.DeadlineAt != *first.Budget.CurrentWindow.DeadlineAt {
				t.Fatal("explicit Start reset time/history or became automatic", next)
			}
			old, e := s.Attempt(ctx, first.Attempt.ID)
			if e != nil || old.Outcome == nil || *old.Outcome != "blocked" || old.PrimaryFailure == nil || old.PrimaryFailure.Code != "routing_unavailable" {
				t.Fatal("authority amendment rewrote predecessor failure", old, e)
			}
		})
	}
}
