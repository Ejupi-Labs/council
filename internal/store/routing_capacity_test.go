package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func routingTestAccount(t *testing.T, s *Store) domain.DeclaredAccount {
	t.Helper()
	r := s.CreateRoutingAccount(context.Background(), domain.AccountCreateInput{RequestID: domain.NewID(), Label: "Declared only", Entitlement: "subscription"})
	if r.Status != 201 {
		t.Fatal(r)
	}
	return r.Envelope.Data.(domain.DeclaredAccount)
}
func routingTestLink(t *testing.T, s *Store, contextID, accountID string, unlink bool) domain.AccountLink {
	t.Helper()
	snap, e := s.RoutingSnapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	revision := ""
	for _, l := range snap.Links {
		if l.ContextID == contextID {
			revision = l.Revision
		}
	}
	in := domain.AccountLinkInput{RequestID: domain.NewID(), ExpectedRevision: revision, DeclaredAccountID: accountID}
	if unlink {
		yes := true
		in.Unlink = &yes
	}
	r := s.UpdateAccountLink(context.Background(), contextID, in)
	if r.Status != 200 {
		t.Fatal(r)
	}
	return r.Envelope.Data.(domain.AccountLink)
}
func routingTestReport(t *testing.T, s *Store, subject domain.CapacitySubject, dimension, state string, edit func(*domain.CapacityReportInput)) domain.CapacityReportResult {
	t.Helper()
	v, e := s.Capacity(context.Background(), subject)
	if e != nil {
		t.Fatal(e)
	}
	in := domain.CapacityReportInput{RequestID: domain.NewID(), Subject: subject, ExpectedRevision: v.Revision, Dimension: dimension, State: state}
	if edit != nil {
		edit(&in)
	}
	r := s.CreateCapacityReport(context.Background(), in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	return r.Envelope.Data.(domain.CapacityReportResult)
}

func TestRoutingCapacityRetainedOldCorrectionAndLinkInheritance(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	account := routingTestAccount(t, s)
	subject := domain.CapacitySubject{Kind: "account", ID: account.ID}
	routingTestLink(t, s, "codex-native", account.ID, false)
	old := routingTestReport(t, s, subject, "allowance", "exhausted", func(in *domain.CapacityReportInput) {
		one := 1
		in.ValidForSeconds = &one
		in.ObservedAt = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	})
	originalBytes := encode(old.Report)
	link := routingTestLink(t, s, "claude-native", account.ID, false)
	linked, e := s.Capacity(ctx, domain.CapacitySubject{Kind: "context", ID: "claude-native"})
	if e != nil || linked.ReportEligibility.Permitted || len(linked.Dimensions.Allowance.BlockingReports) != 1 {
		t.Fatal("linked inherited exhaustion missing", linked, e)
	}
	inherited := linked.Dimensions.Allowance.BlockingReports[0].InheritedFrom
	if inherited == nil || inherited.AccountID != account.ID || inherited.LinkRevision != link.Revision || inherited.LinkedAt != link.UpdatedAt {
		t.Fatal("current account merge erased original link inheritance provenance", linked, e)
	}
	detail, e := s.CapacityReport(ctx, old.Report.ID)
	if e != nil || len(detail.UnresolvedScopeKeys) != 3 {
		t.Fatal(detail, e)
	}
	for _, binding := range detail.Report.Bindings {
		if binding == "context:claude-native" {
			t.Fatal("link rewrote original report bindings", detail)
		}
	}
	routingTestLink(t, s, "claude-native", "", true)
	claude, e := s.Capacity(ctx, domain.CapacitySubject{Kind: "context", ID: "claude-native"})
	if e != nil || claude.ReportEligibility.Permitted || len(claude.Dimensions.Allowance.BlockingReports) != 1 || claude.Dimensions.Allowance.BlockingReports[0].InheritedFrom == nil {
		t.Fatal("unlink erased old exhaustion", claude, e)
	}
	newer := routingTestReport(t, s, subject, "allowance", "exhausted", nil)
	routingTestReport(t, s, subject, "allowance", "available", func(in *domain.CapacityReportInput) { in.CorrectionOf = old.Report.ID; in.CorrectionAction = "recover" })
	a, e := s.Capacity(ctx, subject)
	if e != nil || a.ReportEligibility.Permitted || len(a.Dimensions.Allowance.BlockingReports) != 1 || a.Dimensions.Allowance.BlockingReports[0].ReportID != newer.Report.ID {
		t.Fatal("old correction cleared newer denial", a, e)
	}
	detail, e = s.CapacityReport(ctx, old.Report.ID)
	if e != nil || len(detail.UnresolvedScopeKeys) != 0 || encode(detail.Report) != originalBytes {
		t.Fatal("correction rewrote immutable report", detail, e)
	}
	routingTestReport(t, s, subject, "allowance", "available", func(in *domain.CapacityReportInput) {
		in.CorrectionOf = newer.Report.ID
		in.CorrectionAction = "recover"
	})
	a, e = s.Capacity(ctx, subject)
	if e != nil || !a.ReportEligibility.Permitted {
		t.Fatal(a, e)
	}
}
func TestRoutingCapacityReportFaultReplayAndStaleRevision(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	subject := domain.CapacitySubject{Kind: "context", ID: "codex-native"}
	in := domain.CapacityReportInput{RequestID: domain.NewID(), Subject: subject, ExpectedRevision: "1", Dimension: "occupancy", State: "busy"}
	s.BeforeCommit = func() error { return errors.New("write failed") }
	r := s.CreateCapacityReport(ctx, in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "storage_failed" {
		t.Fatal(r)
	}
	s.BeforeCommit = nil
	view, _ := s.Capacity(ctx, subject)
	if view.Revision != "1" || !view.ReportEligibility.Permitted {
		t.Fatal("rollback changed capacity", view)
	}
	s.AfterCommit = func() error { return errors.New("reply lost") }
	r = s.CreateCapacityReport(ctx, in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" {
		t.Fatal(r)
	}
	s.AfterCommit = nil
	r = s.CreateCapacityReport(ctx, in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	saved := r.Envelope.Data
	r = s.CreateCapacityReport(ctx, in)
	if r.Status != 201 || r.Envelope.Receipt == nil {
		t.Fatal(r)
	}
	// Replayed envelopes decode as maps. Compare the same typed value rather
	// than map serialization order, which JSON v2 intentionally leaves unstable.
	original, decodeErr := decode[domain.CapacityReportResult](encode(saved))
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	replayed, decodeErr := decode[domain.CapacityReportResult](encode(r.Envelope.Data))
	if decodeErr != nil || !reflect.DeepEqual(original, replayed) {
		t.Fatal("replay refreshed report", r, decodeErr)
	}
	in.RequestID = domain.NewID()
	r = s.CreateCapacityReport(ctx, in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "revision_conflict" {
		t.Fatal(r)
	}
	page, e := s.CapacityReports(ctx, subject, 100, "")
	if e != nil || len(page.Items) != 1 {
		t.Fatal(page, e)
	}
}
func TestRoutingConfigurationChangesDoNotRefillCapturedTask(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "captured")
	in := taskInput(p.ID)
	r := s.CreateTask(ctx, domain.NewID(), in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	admission := r.Envelope.Data.(domain.Admission)
	before, e := s.TaskBudget(ctx, admission.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	policy, _ := s.RoutingPolicy(ctx)
	limit := "16"
	r = s.SetRoutingPolicy(ctx, domain.RoutingPolicyInput{RequestID: domain.NewID(), ExpectedRevision: policy.Revision, DefaultBudget: &domain.RoutingBudgetInput{AdmissionLimit: limit}})
	if r.Status != 200 {
		t.Fatal(r)
	}
	no := false
	r = s.UpdateRoutingProfile(ctx, "codex-native-default", domain.ProfileUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: &no})
	if r.Status != 200 {
		t.Fatal(r)
	}
	after, e := s.TaskBudget(ctx, admission.Task.ID)
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("configuration refilled authority", before, after, e)
	}
	r = s.CreateTaskWithRouting(ctx, "other-owner", in, domain.RoutingFacts{})
	if r.Status != 201 {
		t.Fatal("current disabled target prevented exact replay", r)
	}
}

func TestRoutingDeclaredAccountBoundDoesNotEraseCapacityOrOldReceipt(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	var first domain.DeclaredAccount
	var lastInput domain.AccountCreateInput
	for i := range domain.MaxDeclaredAccounts {
		in := domain.AccountCreateInput{RequestID: domain.NewID(), Label: fmt.Sprintf("Declared%d", i)}
		r := s.CreateRoutingAccount(ctx, in)
		if r.Status != 201 {
			t.Fatal(i, r)
		}
		if i == 0 {
			first = r.Envelope.Data.(domain.DeclaredAccount)
		}
		lastInput = in
	}
	r := s.CreateRoutingAccount(ctx, domain.AccountCreateInput{RequestID: domain.NewID(), Label: "Beyond bound"})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "routing_unavailable" || r.Envelope.Receipt != nil {
		t.Fatal("unbounded declarations accepted", r)
	}
	if r = s.CreateRoutingAccount(ctx, lastInput); r.Status != 201 || r.Envelope.Receipt == nil {
		t.Fatal("full catalog blocked old exact receipt", r)
	}
	subject := domain.CapacitySubject{Kind: "account", ID: first.ID}
	routingTestReport(t, s, subject, "allowance", "exhausted", nil)
	label := "Renamed declaration"
	r = s.UpdateRoutingAccount(ctx, first.ID, domain.AccountUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Label: &label})
	if r.Status != 200 {
		t.Fatal(r)
	}
	view, e := s.Capacity(ctx, subject)
	if e != nil || view.ReportEligibility.Permitted || view.Dimensions.Allowance.EffectiveState != "exhausted" {
		t.Fatal("label update reset capacity", view, e)
	}
	page, e := s.RoutingAccounts(ctx, 64, "")
	if e != nil || len(page.Items) != 64 || page.NextCursor != nil {
		t.Fatal("bounded account page", len(page.Items), e)
	}
}
