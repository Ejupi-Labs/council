package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
)

func TestRoutingReadsDefaultThirtyAndKeepCursorHighWater(t *testing.T) {
	e, _, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	for i := range 33 {
		r := e.db.CreateRoutingAccount(ctx, domain.AccountCreateInput{RequestID: domain.NewID(), Label: "Declared fixture " + strconv.Itoa(i)})
		if r.Envelope.Error != nil {
			t.Fatal(r.Envelope.Error)
		}
	}
	first := e.readRouting(ctx, "routing.accounts.list", "", nil)
	if first.Status != 200 || first.Envelope.Error != nil || first.Envelope.Receipt != nil {
		t.Fatal(first)
	}
	page := first.Envelope.Data.(domain.Page[domain.DeclaredAccount])
	if len(page.Items) != 30 || page.NextCursor == nil {
		t.Fatalf("Routing default did not return thirty entries: %+v", page)
	}
	if r := e.db.CreateRoutingAccount(ctx, domain.AccountCreateInput{RequestID: domain.NewID(), Label: "After the high-water mark"}); r.Envelope.Error != nil {
		t.Fatal(r.Envelope.Error)
	}
	next := e.readRouting(ctx, "routing.accounts.list", "", map[string]string{"cursor": *page.NextCursor})
	if next.Envelope.Error != nil {
		t.Fatal(next.Envelope.Error)
	}
	last := next.Envelope.Data.(domain.Page[domain.DeclaredAccount])
	if len(last.Items) != 3 || last.NextCursor != nil {
		t.Fatalf("A later account leaked into an existing page walk: %+v", last)
	}
	seen := map[string]bool{}
	for _, account := range append(page.Items, last.Items...) {
		if seen[account.ID] {
			t.Fatal("A routing cursor duplicated an account")
		}
		seen[account.ID] = true
	}
	foreign := e.readRouting(ctx, "routing.reports.list", "", map[string]string{"contextId": "claude-native", "cursor": *page.NextCursor})
	if foreign.Envelope.Error == nil || foreign.Envelope.Error.Code != "invalid_cursor" {
		t.Fatal("An account cursor was accepted as capacity history", foreign)
	}
}

func TestRoutingReadsKeepCapturedAuthorityAndDoNotSpendBudget(t *testing.T) {
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	admitted := e.db.CreateTask(ctx, e.config.OwnerInstanceID, claudeFixtureInput(project, "response"))
	a := decodeAdmission(t, admitted)
	before, err := e.db.TaskBudget(ctx, a.Task.ID)
	if err != nil || before.Budget == nil || before.Budget.ConsumedAdmissions != "1" {
		t.Fatal(before, err)
	}
	for _, op := range []string{"tasks.routing.get", "tasks.routing.decisions.list", "tasks.budget.get", "tasks.budget.ledger.list", "tasks.usage.list"} {
		r := e.readRouting(ctx, op, a.Task.ID, nil)
		if r.Status != 200 || r.Envelope.Error != nil || r.Envelope.Receipt != nil || !domain.ValidID(r.Envelope.RequestID) {
			t.Fatalf("%s: %+v", op, r)
		}
	}
	routingView := e.readRouting(ctx, "tasks.routing.get", a.Task.ID, nil).Envelope.Data.(domain.TaskRoutingView)
	if routingView.AdoptionRequired || routingView.Authority == nil || routingView.LatestDecision == nil || routingView.LatestDecision.AttemptID != a.Attempt.ID || routingView.LatestDecision.SelectedTarget.ContextID != "claude-native" {
		t.Fatal("Read lost the captured routing lineage", routingView)
	}
	contexts := e.readRouting(ctx, "routing.contexts.list", "", nil).Envelope.Data.(domain.Page[domain.ContextView])
	if len(contexts.Items) != 2 || contexts.Items[0].AccountLink.Revision != "1" || contexts.Items[0].AccountLink.DeclaredAccountID != nil {
		t.Fatal("An initial link revision or its unknown identity was lost", contexts)
	}
	reported := e.db.CreateCapacityReport(ctx, domain.CapacityReportInput{RequestID: domain.NewID(), Subject: domain.CapacitySubject{Kind: "context", ID: "codex-native"}, ExpectedRevision: "1", Dimension: "occupancy", State: "busy"})
	if reported.Envelope.Error != nil {
		t.Fatal(reported.Envelope.Error)
	}
	report := reported.Envelope.Data.(domain.CapacityReportResult).Report
	detailRead := e.readRouting(ctx, "routing.reports.get", report.ID, nil)
	if detailRead.Envelope.Error != nil {
		t.Fatal(detailRead.Envelope.Error)
	}
	detail := detailRead.Envelope.Data.(domain.CapacityReportDetail)
	if detail.Report.ID != report.ID || detail.Report.Source != "user_report" || len(detail.UnresolvedScopeKeys) == 0 {
		t.Fatal("Report detail lost its exact evidence or retained bindings", detail)
	}
	profiles := e.readRouting(ctx, "routing.profiles.list", "", map[string]string{"contextId": "claude-native"}).Envelope.Data.(domain.Page[domain.TargetProfile])
	if len(profiles.Items) != 3 {
		t.Fatal("Profile filtering did not retain the fixed context", profiles)
	}
	after, err := e.db.TaskBudget(ctx, a.Task.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("Read-only routing inspection changed budget authority", before, after, err)
	}
}

func TestRoutingReadRejectsMalformedIdentitiesFiltersAndLimits(t *testing.T) {
	e, _, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	for _, tc := range []struct {
		op, id string
		query  map[string]string
		code   string
	}{
		{"routing.policy.get", "", map[string]string{"unknown": "x"}, "invalid_request"},
		{"routing.policy.get", "unexpected", nil, "invalid_request"},
		{"routing.profiles.list", "", map[string]string{"contextId": "unknown-context"}, "invalid_request"},
		{"routing.capacity.get", "", nil, "invalid_request"},
		{"routing.capacity.get", "", map[string]string{"contextId": "codex-native", "accountId": domain.NewID()}, "invalid_request"},
		{"routing.reports.list", "", map[string]string{"accountId": "account-label"}, "invalid_request"},
		{"routing.reports.get", "codex-native", nil, "invalid_request"},
		{"tasks.routing.get", "missing", nil, "invalid_request"},
		{"routing.accounts.list", "", map[string]string{"cursor": ""}, "invalid_cursor"},
		{"routing.accounts.list", "", map[string]string{"limit": ""}, "invalid_request"},
		{"routing.accounts.list", "", map[string]string{"limit": "01"}, "invalid_request"},
		{"routing.accounts.list", "", map[string]string{"limit": "101"}, "invalid_request"},
		{"routing.accounts.list", "", map[string]string{"limit": "9007199254740993"}, "invalid_request"},
		{"routing.missing", "", nil, "not_found"},
	} {
		t.Run(tc.op+"/"+tc.code+"/"+tc.id, func(t *testing.T) {
			r := e.readRouting(context.Background(), tc.op, tc.id, tc.query)
			if r.Envelope.Error == nil || r.Envelope.Error.Code != tc.code || r.Envelope.Receipt != nil {
				t.Fatalf("Malformed read reached a different contract: %+v", r)
			}
		})
	}
}

func TestRoutingHTTPReadsAndMutationsKeepAuthorityAndReceipts(t *testing.T) {
	e, _, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	service := app.New(e.db)
	service.Execution = e
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler = httpapi.New(service, httpapi.Config{Host: server.Listener.Addr().String(), Token: "routing-test-token", OwnerInstanceID: e.config.OwnerInstanceID})
	server.Start()
	t.Cleanup(server.Close)
	body := []byte(`{"requestId":"` + domain.NewID() + `","label":"HTTP declared fixture"}`)
	request := func(method, route string, payload []byte, token, origin string) (int, domain.Envelope) {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+"/api/v1"+route, bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		r.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		encoded, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		var envelope domain.Envelope
		if json.Unmarshal(encoded, &envelope) != nil || strings.Contains(string(encoded), "routing-test-token") {
			t.Fatalf("Invalid or secret-bearing response: %s", encoded)
		}
		return response.StatusCode, envelope
	}
	status, metadata := request("GET", "/meta", nil, "routing-test-token", "")
	if status != 200 || metadata.Error != nil || metadata.Data.(map[string]any)["ownerInstanceId"] != e.config.OwnerInstanceID {
		t.Fatal("Routing observer contacted a different owner", status, metadata)
	}
	status, original := request("POST", "/routing/accounts", body, "routing-test-token", server.URL)
	if status != 201 || original.Error != nil || original.Receipt == nil {
		t.Fatal("Routing mutation did not return its real receipt", status, original)
	}
	status, replay := request("POST", "/routing/accounts", body, "routing-test-token", server.URL)
	if status != 201 || !reflect.DeepEqual(original, replay) {
		t.Fatal("HTTP replay changed the original account admission", status, replay)
	}
	for _, route := range []string{"/routing/policy", "/routing/contexts", "/routing/profiles?contextId=claude-native", "/routing/accounts?limit=1", "/routing/capacity?contextId=codex-native", "/routing/reports?contextId=claude-native"} {
		status, result := request("GET", route, nil, "routing-test-token", "")
		if status != 200 || result.Error != nil || result.Receipt != nil {
			t.Fatal(route, status, result)
		}
	}
	for _, tc := range []struct {
		route, token, origin string
		status               int
	}{
		{"/routing/accounts", "", "", 401},
		{"/routing/accounts", "routing-test-token", "http://foreign.invalid", 403},
		{"/routing/accounts?limit=1&limit=2", "routing-test-token", "", 400},
		{"/routing/profiles?contextId=", "routing-test-token", "", 400},
		{"/routing/accounts?limit=101", "routing-test-token", "", 400},
		{"/routing/capacity?contextId=codex-native&accountId=" + domain.NewID(), "routing-test-token", "", 400},
	} {
		if code, result := request("GET", tc.route, nil, tc.token, tc.origin); code != tc.status || result.Error == nil || result.Receipt != nil {
			t.Fatal(tc.route, code, result)
		}
	}
	accounts, err := e.db.RoutingAccounts(context.Background(), 100, "")
	if err != nil || len(accounts.Items) != 1 {
		t.Fatal("Read/replay/rejected HTTP requests changed declaration count", accounts, err)
	}
}
