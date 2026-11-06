package cli

import (
	"context"
	json "encoding/json/v2"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func routingCLIError(t *testing.T, f *cliFixture, args ...string) domain.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := f.command(ctx, append(args, "--json")...).Output()
	var envelope domain.Envelope
	if err == nil || json.Unmarshal(out, &envelope) != nil || envelope.Error == nil {
		t.Fatalf("expected retained error envelope: %v %s", err, out)
	}
	if envelope.Receipt != nil {
		t.Fatal("failed routing operation fabricated receipt")
	}
	return envelope
}

func TestRoutingCLIActualOwnerSelectsReportedContextAndDebitsOnce(t *testing.T) {
	f := newCLIFixture(t)
	var opened struct {
		Project domain.Project `json:"project"`
	}
	decodeCLIData(t, f.run("projects", "open", f.repo), &opened)
	var busy domain.CapacityReportResult
	decodeCLIData(t, f.run("routing", "report", "--context", "codex-native", "--expected-revision", "1", "--dimension", "occupancy", "--state", "busy"), &busy)
	var available domain.CapacityReportResult
	decodeCLIData(t, f.run("routing", "report", "--context", "claude-native", "--expected-revision", "1", "--dimension", "occupancy", "--state", "available"), &available)
	decodeCLIData(t, f.run("routing", "report", "--context", "claude-native", "--expected-revision", available.Capacity.Revision, "--dimension", "allowance", "--state", "available"), &available)
	request := domain.NewID()
	args := []string{"tasks", "start", "--project", opened.Project.ID, "--task", "\ufeffExactly retained Auto input\r\n東京", "--suitability", "routine", "--deliverable", "response_only", "--fake", "--timeout-seconds", "30", "--request-id", request}
	first := f.run(args...)
	var admission domain.Admission
	decodeCLIData(t, first, &admission)
	if admission.RoutingDecision == nil || admission.Budget == nil || admission.RoutingDecision.SelectedTarget.ProfileID == nil || *admission.RoutingDecision.SelectedTarget.ProfileID != "claude-haiku-economy" || admission.Attempt.ConnectionID != "claude-native" || admission.Version.Pins.HarnessID != "" || admission.Version.Pins.Model != "" || admission.Budget.ConsumedAdmissions != "1" || admission.Budget.RemainingAdmissions != "1" {
		t.Fatalf("Auto or immutable input incorrect %+v", admission)
	}
	if replay := f.run(args...); !reflect.DeepEqual(first, replay) {
		t.Fatal("exact start replay changed admission or budget")
	}
	if done := f.settled(admission.Attempt.ID); done.Outcome == nil || *done.Outcome != "succeeded" {
		t.Fatalf("fake authoring failed %+v", done)
	}
	var task domain.TaskDetail
	decodeCLIData(t, f.run("tasks", "get", admission.Task.ID), &task)
	if task.Task.OriginalText != "\ufeffExactly retained Auto input\r\n東京" {
		t.Fatal("original text rewritten")
	}
	var second domain.Admission
	decodeCLIData(t, f.run("tasks", "run", task.Task.ID, "--expected-revision", task.Task.Revision, "--task-version", task.Task.CurrentVersion, "--fake"), &second)
	if second.Budget == nil || second.Budget.ConsumedAdmissions != "2" || second.Budget.RemainingAdmissions != "0" || second.Budget.CurrentWindow.Number != admission.Budget.CurrentWindow.Number || second.Budget.CurrentWindow.DeadlineAt == nil || *second.Budget.CurrentWindow.DeadlineAt != *admission.Budget.CurrentWindow.DeadlineAt {
		t.Fatalf("attempt got separate/refilled budget %+v", second.Budget)
	}
	f.settled(second.Attempt.ID)
	decodeCLIData(t, f.run("tasks", "get", task.Task.ID), &task)
	blocked := routingCLIError(t, f, "tasks", "run", task.Task.ID, "--expected-revision", task.Task.Revision, "--task-version", task.Task.CurrentVersion, "--fake")
	if blocked.Error.Code != "budget_exhausted" {
		t.Fatalf("wrong budget blocker %+v", blocked.Error)
	}
	var budget domain.TaskBudgetView
	decodeCLIData(t, f.run("tasks", "budget", task.Task.ID), &budget)
	if budget.Budget == nil || budget.Budget.ConsumedAdmissions != "2" {
		t.Fatal("failed admission debited budget")
	}
	grantID := domain.NewID()
	grantArgs := []string{"tasks", "budget", "grant", task.Task.ID, "--expected-revision", task.Task.Revision, "--expected-budget-revision", budget.Budget.Revision, "--additional-admissions", "1", "--request-id", grantID}
	grant := f.run(grantArgs...)
	if replay := f.run(grantArgs...); !reflect.DeepEqual(grant, replay) {
		t.Fatal("grant replay changed receipt")
	}
	var amendment domain.BudgetAmendmentResult
	decodeCLIData(t, grant, &amendment)
	if amendment.Budget.ConsumedAdmissions != "2" || amendment.Budget.RemainingAdmissions != "1" {
		t.Fatal("grant rewrote prior use")
	}
	decodeCLIData(t, f.run("tasks", "get", task.Task.ID), &task)
	if task.Task.LatestAttemptID == nil || *task.Task.LatestAttemptID != second.Attempt.ID {
		t.Fatal("grant launched a worker")
	}
	var usage domain.UsagePage
	decodeCLIData(t, f.run("tasks", "usage", task.Task.ID), &usage)
	if len(usage.Items) != 2 || len(usage.Coverage.ExcludedFakeAttemptIDs) != 2 || usage.Charges.Status != "unknown" || usage.KnownSubtotal.ByUsageScope.TopLevel.InputTokens.KnownValue != nil {
		t.Fatalf("fake usage became measured %+v", usage)
	}
	var routing domain.TaskRoutingView
	decodeCLIData(t, f.run("tasks", "routing", task.Task.ID), &routing)
	if routing.Authority == nil || routing.LatestDecision == nil || routing.AdoptionRequired {
		t.Fatal("routing authority not retained")
	}
}

func TestRoutingCLIAccountCorrectionRetainsImmutableReportAndNewerDenial(t *testing.T) {
	f := newCLIFixture(t)
	var account domain.DeclaredAccount
	decodeCLIData(t, f.run("routing", "account", "add", "--label", "Declared fixture capacity", "--provider", "Fixture label", "--entitlement", "unknown"), &account)
	var link domain.AccountLink
	decodeCLIData(t, f.run("routing", "link", "--context", "claude-native", "--declared-account", account.ID, "--expected-revision", "1"), &link)
	var denied domain.CapacityReportResult
	decodeCLIData(t, f.run("routing", "report", "--account", account.ID, "--expected-revision", "1", "--dimension", "allowance", "--state", "exhausted"), &denied)
	var newer domain.CapacityReportResult
	decodeCLIData(t, f.run("routing", "report", "--account", account.ID, "--expected-revision", denied.Capacity.Revision, "--dimension", "service", "--state", "authentication_denied"), &newer)
	original := f.run("routing", "report", "get", denied.Report.ID)
	var before domain.CapacityReportDetail
	decodeCLIData(t, original, &before)
	var corrected domain.CapacityReportResult
	decodeCLIData(t, f.run("routing", "report", "--account", account.ID, "--expected-revision", newer.Capacity.Revision, "--dimension", "allowance", "--state", "available", "--correction-of", denied.Report.ID, "--correction-action", "recover"), &corrected)
	var after domain.CapacityReportDetail
	decodeCLIData(t, f.run("routing", "report", "get", denied.Report.ID), &after)
	if !reflect.DeepEqual(before.Report, after.Report) || len(after.UnresolvedScopeKeys) != 0 {
		t.Fatal("correction rewrote immutable report or retained old denial")
	}
	var capacity domain.CapacityView
	decodeCLIData(t, f.run("routing", "capacity", "--context", "claude-native"), &capacity)
	if capacity.ReportEligibility.Permitted || len(capacity.Dimensions.Allowance.BlockingReports) != 0 || len(capacity.Dimensions.Service.BlockingReports) != 1 || capacity.Dimensions.Service.BlockingReports[0].ReportID != newer.Report.ID {
		t.Fatalf("correction erased another denial %+v", capacity)
	}
	if capacity.AccountLink == nil || capacity.AccountLink.DeclaredAccountID == nil || *capacity.AccountLink.DeclaredAccountID != account.ID {
		t.Fatal("declaration link lost")
	}
}
