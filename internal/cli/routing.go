package cli

import (
	"io"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const routingHelp = `
Economical routing (native selectors; account access and actual costs remain unknown):
  council routing policy | policy set --expected-revision R [routing preferences]
  council routing contexts | context set ID --expected-revision R [--label TEXT] [--enabled true|false]
  council routing profiles [--context ID] | profile set ID --expected-revision R [--enabled true|false]
    [--reference-price-file PATH|- | --clear-reference-price true]
  council routing accounts [--limit N] [--cursor CURSOR]
  council routing account add --label TEXT [--provider TEXT] [--entitlement unknown|subscription|local]
  council routing account set ID --expected-revision R [--label TEXT] [--provider TEXT] [--entitlement VALUE]
  council routing link --context ID (--declared-account ID | --unlink true) --expected-revision R
  council routing capacity (--context ID | --account ID)
  council routing reports (--context ID | --account ID) [--limit N] [--cursor CURSOR]
  council routing report get ID
  council routing report (--context ID | --account ID) --expected-revision R --dimension VALUE --state VALUE
    [--valid-for-seconds N] [--observed-at TIME] [--retry-after TIME] [--note TEXT]
    [--correction-of ID --correction-action recover|withdraw]
  council tasks routing ID | routing-decisions ID | budget ID | budget-ledger ID | usage ID
  council tasks budget grant ID --expected-revision R --expected-budget-revision B --additional-admissions N
    [--new-window-seconds N]
  council tasks adopt ID --expected-revision R --additional-admissions N --window-seconds N [routing choices]

Start/guidance routing choices: --suitability unknown|routine|general, --target-profile ID,
--expected-routing-policy-revision R, --max-automatic-admissions N,
repeatable unique --allow-continuation pre_launch_ineligible,
--allow-replacement true|false and --allow-escalation true|false.
Start additionally accepts --budget-admissions N and --budget-window-seconds N.
Guidance accepts --use-current-routing-policy true|false; budget changes require an explicit grant.
Policy set accepts --unknown-availability allow|require_positive, repeatable unique --prefer-profile ID,
--default-admission-limit N, --default-window-seconds N and continuation choices.
Budgets count managed attempt admissions within one shared wall-clock window, including queue time.
Automatic continuation defaults OFF. No monetary cap, paid API fallback or verified account pin is offered.
`

var continuationFlags = []string{"max-automatic-admissions", "allow-continuation", "allow-replacement", "allow-escalation"}
var taskRoutingFlags = []string{"suitability", "target-profile", "expected-routing-policy-revision", "max-automatic-admissions", "allow-continuation", "allow-replacement", "allow-escalation"}

func addRoutingFlags(values map[string]bool) {
	for _, key := range []string{"enabled", "unlink", "clear-reference-price", "allow-replacement", "allow-escalation", "use-current-routing-policy", "suitability", "target-profile", "expected-routing-policy-revision", "budget-admissions", "budget-window-seconds", "max-automatic-admissions", "allow-continuation", "prefer-profile", "unknown-availability", "default-admission-limit", "default-window-seconds", "context", "declared-account", "label", "provider", "entitlement", "reference-price-file", "dimension", "state", "observed-at", "retry-after", "note", "correction-of", "correction-action", "valid-for-seconds", "expected-budget-revision", "additional-admissions", "new-window-seconds", "window-seconds"} {
		values[key] = true
	}
}
func repeatableFlag(key string) bool {
	return key == "require-evidence" || key == "prefer-profile" || key == "allow-continuation" || key == "rebind-node"
}
func routingCommand(name string) bool {
	return strings.HasPrefix(name, "routing ") || name == "tasks routing" || name == "tasks routing-decisions" || name == "tasks budget" || name == "tasks budget-ledger" || name == "tasks usage" || name == "tasks adopt"
}
func boolFlag(flags map[string]string, key string) (*bool, *domain.Error) {
	value, ok := flags[key]
	if !ok {
		return nil, nil
	}
	if value != "true" && value != "false" {
		return nil, domain.Invalid("explicit true or false for --" + key)
	}
	b := value == "true"
	return &b, nil
}
func uniqueRepeated(values []string) *domain.Error {
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] {
			return domain.Invalid("unique repeated option values")
		}
		seen[value] = true
	}
	return nil
}
func continuationOptions(flags map[string]string, repeated map[string][]string) (*domain.ContinuationInput, *domain.Error) {
	var in *domain.ContinuationInput
	for _, key := range continuationFlags {
		if _, ok := flags[key]; ok {
			in = &domain.ContinuationInput{}
			break
		}
	}
	if in == nil {
		return nil, nil
	}
	in.MaxAutomaticAdmissions = flags["max-automatic-admissions"]
	if _, ok := flags["allow-continuation"]; ok {
		values := append([]string{}, repeated["allow-continuation"]...)
		if err := uniqueRepeated(values); err != nil {
			return nil, err
		}
		in.Categories = &values
	}
	var err *domain.Error
	in.AllowReplacement, err = boolFlag(flags, "allow-replacement")
	if err != nil {
		return nil, err
	}
	in.AllowEscalation, err = boolFlag(flags, "allow-escalation")
	if err != nil {
		return nil, err
	}
	return in, domain.ValidateContinuationInput(in)
}
func routingOptions(flags map[string]string, repeated map[string][]string, mode string) (*domain.RoutingOptions, *domain.Error) {
	var in *domain.RoutingOptions
	keys := append(append([]string{}, taskRoutingFlags...), "budget-admissions", "budget-window-seconds", "use-current-routing-policy")
	for _, key := range keys {
		if _, ok := flags[key]; ok {
			in = &domain.RoutingOptions{}
			break
		}
	}
	if in == nil {
		return nil, nil
	}
	in.Suitability, in.TargetProfileID, in.ExpectedPolicyRevision = flags["suitability"], flags["target-profile"], flags["expected-routing-policy-revision"]
	var err *domain.Error
	in.Continuation, err = continuationOptions(flags, repeated)
	if err != nil {
		return nil, err
	}
	in.UseCurrentPolicy, err = boolFlag(flags, "use-current-routing-policy")
	if err != nil {
		return nil, err
	}
	if _, a := flags["budget-admissions"]; a {
		in.Budget = &domain.RoutingBudgetInput{AdmissionLimit: flags["budget-admissions"]}
	}
	if value, ok := flags["budget-window-seconds"]; ok {
		n, e := boundedInteger(value, 1, 86400)
		if e != nil {
			return nil, e
		}
		if in.Budget == nil {
			in.Budget = &domain.RoutingBudgetInput{}
		}
		in.Budget.WindowSeconds = &n
	}
	return in, domain.ValidateRoutingOptions(in, mode)
}

func parseRouting(c command, pos []string, flags map[string]string, repeated map[string][]string, stdin io.Reader) (command, *domain.Error) {
	allowed := map[string]bool{"json": true, "api-version": true}
	allow := func(keys ...string) {
		for _, key := range keys {
			allowed[key] = true
		}
	}
	count := 2
	identityKind := ""
	identityIndex := 2
	c.method = "GET"
	switch c.name {
	case "routing policy":
		c.op, c.route = "routing.policy.get", "/routing/policy"
		if len(pos) > 2 && pos[2] == "set" {
			count = 3
			c.op, c.method = "routing.policy.update", "PUT"
			allow("request-id", "expected-revision", "prefer-profile", "unknown-availability", "default-admission-limit", "default-window-seconds")
			allow(continuationFlags...)
		}
	case "routing contexts":
		c.op, c.route = "routing.contexts.list", "/routing/contexts"
	case "routing context":
		count, identityIndex, identityKind = 4, 3, "context"
		c.op, c.method = "routing.context.update", "PUT"
		allow("request-id", "expected-revision", "label", "enabled")
		if len(pos) < 3 || pos[2] != "set" {
			return c, domain.Invalid("routing context set command")
		}
	case "routing profiles":
		c.op, c.route = "routing.profiles.list", "/routing/profiles"
		allow("context")
	case "routing profile":
		count, identityIndex, identityKind = 4, 3, "profile"
		c.op, c.method = "routing.profile.update", "PUT"
		allow("request-id", "expected-revision", "enabled", "reference-price-file", "clear-reference-price")
		if len(pos) < 3 || pos[2] != "set" {
			return c, domain.Invalid("routing profile set command")
		}
	case "routing accounts":
		c.op, c.route = "routing.accounts.list", "/routing/accounts"
		allow("limit", "cursor")
	case "routing account":
		c.route = "/routing/accounts"
		allow("request-id", "label", "provider", "entitlement")
		if len(pos) > 2 && pos[2] == "add" {
			count = 3
			c.op, c.method = "routing.accounts.create", "POST"
		} else if len(pos) > 2 && pos[2] == "set" {
			count, identityIndex, identityKind = 4, 3, "account"
			c.op, c.method = "routing.accounts.update", "PUT"
			allow("expected-revision")
		} else {
			return c, domain.Invalid("routing account add or set")
		}
	case "routing link":
		c.op, c.method = "routing.links.update", "PUT"
		allow("request-id", "context", "declared-account", "unlink", "expected-revision")
	case "routing capacity":
		c.op, c.route = "routing.capacity.get", "/routing/capacity"
		allow("context", "account")
	case "routing reports":
		c.op, c.route = "routing.reports.list", "/routing/reports"
		allow("context", "account", "limit", "cursor")
	case "routing report":
		if len(pos) > 2 && pos[2] == "get" {
			count, identityIndex, identityKind = 4, 3, "report"
			c.op = "routing.reports.get"
		} else {
			c.op, c.route, c.method = "routing.reports.create", "/routing/reports", "POST"
			allow("request-id", "context", "account", "expected-revision", "dimension", "state", "observed-at", "retry-after", "note", "correction-of", "correction-action", "valid-for-seconds")
		}
	case "tasks routing":
		count, identityKind = 3, "task"
		c.op = "tasks.routing.get"
	case "tasks routing-decisions":
		count, identityKind = 3, "task"
		c.op = "tasks.routing.decisions.list"
		allow("limit", "cursor")
	case "tasks budget":
		count, identityKind = 3, "task"
		c.op = "tasks.budget.get"
		if len(pos) > 2 && pos[2] == "grant" {
			count, identityIndex = 4, 3
			c.op, c.method = "tasks.budget.amend", "POST"
			allow("request-id", "expected-revision", "expected-budget-revision", "additional-admissions", "new-window-seconds")
		}
	case "tasks budget-ledger":
		count, identityKind = 3, "task"
		c.op = "tasks.budget.ledger.list"
		allow("limit", "cursor")
	case "tasks usage":
		count, identityKind = 3, "task"
		c.op = "tasks.usage.list"
		allow("limit", "cursor")
	case "tasks adopt":
		count, identityKind = 3, "task"
		c.op, c.method = "tasks.budget.amend", "POST"
		allow("request-id", "expected-revision", "additional-admissions", "window-seconds")
		allow(taskRoutingFlags...)
	default:
		return c, domain.Invalid("documented routing command")
	}
	if len(pos) != count {
		return c, domain.Invalid("routing command arguments")
	}
	for key := range flags {
		if !allowed[key] {
			return c, domain.Invalid("options supported by this routing command")
		}
	}
	if identityKind != "" {
		c.params.ID = pos[identityIndex]
		valid := domain.ValidID(c.params.ID)
		if identityKind == "context" {
			valid = domain.ValidContextID(c.params.ID)
		}
		if identityKind == "profile" {
			valid = domain.ValidProfileID(c.params.ID)
		}
		if !valid {
			return c, domain.Invalid("routing resource identity")
		}
		switch identityKind {
		case "context":
			c.route = "/routing/contexts/" + c.params.ID
		case "profile":
			c.route = "/routing/profiles/" + c.params.ID
		case "account":
			c.route += "/" + c.params.ID
		case "report":
			c.route = "/routing/reports/" + c.params.ID
		case "task":
			suffix := map[string]string{"tasks.routing.get": "routing", "tasks.routing.decisions.list": "routing/decisions", "tasks.budget.get": "budget", "tasks.budget.ledger.list": "budget/ledger", "tasks.usage.list": "usage", "tasks.budget.amend": "budget-amendments"}[c.op]
			c.route = "/tasks/" + c.params.ID + "/" + suffix
		}
	}
	if c.method != "GET" {
		var e *domain.Error
		c.requestID, e = mutationRequestID(flags)
		if e != nil {
			return c, e
		}
	}
	if strings.HasSuffix(c.op, ".list") {
		if e := validatePageFlags(&c, flags); e != nil {
			return c, e
		}
	}
	subject := domain.CapacitySubject{}
	if c.op == "routing.capacity.get" || c.op == "routing.reports.list" || c.op == "routing.reports.create" {
		context, hasContext := flags["context"]
		account, hasAccount := flags["account"]
		if hasContext == hasAccount {
			return c, domain.Invalid("exactly one --context or --account")
		}
		subject = domain.CapacitySubject{Kind: "context", ID: context}
		if hasAccount {
			subject = domain.CapacitySubject{Kind: "account", ID: account}
		}
		if e := domain.ValidateCapacitySubject(subject); e != nil {
			return c, e
		}
		if c.method == "GET" {
			c.params.Query[subject.Kind+"Id"] = subject.ID
		}
	}
	switch c.op {
	case "routing.policy.update":
		in := domain.RoutingPolicyInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], UnknownAvailability: flags["unknown-availability"]}
		if _, ok := flags["prefer-profile"]; ok {
			v := append([]string{}, repeated["prefer-profile"]...)
			in.EconomyOrder = &v
		}
		if value, ok := flags["default-admission-limit"]; ok {
			in.DefaultBudget = &domain.RoutingBudgetInput{AdmissionLimit: value}
		}
		if value, ok := flags["default-window-seconds"]; ok {
			n, e := boundedInteger(value, 1, 86400)
			if e != nil {
				return c, e
			}
			if in.DefaultBudget == nil {
				in.DefaultBudget = &domain.RoutingBudgetInput{}
			}
			in.DefaultBudget.WindowSeconds = &n
		}
		var e *domain.Error
		in.Continuation, e = continuationOptions(flags, repeated)
		if e != nil {
			return c, e
		}
		if e = domain.ValidateRoutingPolicyInput(in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	case "routing.context.update":
		in := domain.ContextUpdateInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"]}
		if value, ok := flags["label"]; ok {
			in.Label = &value
		}
		var e *domain.Error
		in.Enabled, e = boolFlag(flags, "enabled")
		if e != nil {
			return c, e
		}
		if e = domain.ValidateContextUpdate(c.params.ID, in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	case "routing.profile.update":
		in := domain.ProfileUpdateInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"]}
		var e *domain.Error
		in.Enabled, e = boolFlag(flags, "enabled")
		if e != nil {
			return c, e
		}
		in.ClearReferencePrice, e = boolFlag(flags, "clear-reference-price")
		if e != nil {
			return c, e
		}
		if filename, ok := flags["reference-price-file"]; ok {
			raw, e := readExecutionFile(filename, stdin)
			if e != nil {
				return c, e
			}
			in.ReferencePrice = &domain.ReferencePriceInput{}
			if e = domain.StrictDecode([]byte(raw), in.ReferencePrice); e != nil {
				return c, e
			}
		}
		if e = domain.ValidateProfileUpdate(c.params.ID, in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	case "routing.profiles.list":
		if value, ok := flags["context"]; ok {
			if !domain.ValidContextID(value) {
				return c, domain.Invalid("native context identity")
			}
			c.params.Query["contextId"] = value
		}
	case "routing.accounts.create":
		in := domain.AccountCreateInput{RequestID: c.requestID, Label: flags["label"], ProviderLabel: flags["provider"], Entitlement: flags["entitlement"]}
		if e := domain.ValidateAccountCreate(in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	case "routing.accounts.update":
		in := domain.AccountUpdateInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], Entitlement: flags["entitlement"]}
		if value, ok := flags["label"]; ok {
			in.Label = &value
		}
		if value, ok := flags["provider"]; ok {
			in.ProviderLabel = &value
		}
		if e := domain.ValidateAccountUpdate(c.params.ID, in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	case "routing.links.update":
		c.params.ID = flags["context"]
		c.route = "/routing/contexts/" + c.params.ID + "/account-link"
		in := domain.AccountLinkInput{RequestID: c.requestID, ExpectedRevision: flags["expected-revision"], DeclaredAccountID: flags["declared-account"]}
		var e *domain.Error
		in.Unlink, e = boolFlag(flags, "unlink")
		if e != nil {
			return c, e
		}
		if e = domain.ValidateAccountLink(c.params.ID, in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	case "routing.reports.create":
		in := domain.CapacityReportInput{RequestID: c.requestID, Subject: subject, ExpectedRevision: flags["expected-revision"], Dimension: flags["dimension"], State: flags["state"], ObservedAt: flags["observed-at"], RetryAfter: flags["retry-after"], Note: flags["note"], CorrectionOf: flags["correction-of"], CorrectionAction: flags["correction-action"]}
		if value, ok := flags["valid-for-seconds"]; ok {
			n, e := boundedInteger(value, 1, 86400)
			if e != nil {
				return c, e
			}
			in.ValidForSeconds = &n
		}
		if e := domain.ValidateCapacityReport(in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	case "tasks.budget.amend":
		in := domain.BudgetAmendmentInput{RequestID: c.requestID, Action: "grant", ExpectedRevision: flags["expected-revision"], ExpectedBudgetRevision: flags["expected-budget-revision"], AdditionalAdmissions: flags["additional-admissions"]}
		windowKey := "new-window-seconds"
		if c.name == "tasks adopt" {
			in.Action = "adopt"
			windowKey = "window-seconds"
			var e *domain.Error
			in.Routing, e = routingOptions(flags, repeated, "adopt")
			if e != nil {
				return c, e
			}
		}
		if value, ok := flags[windowKey]; ok {
			n, e := boundedInteger(value, 1, 86400)
			if e != nil {
				return c, e
			}
			in.NewWindowSeconds = &n
		}
		if e := domain.ValidateBudgetAmendment(in); e != nil {
			return c, e
		}
		return encodeExecution(c, in)
	}
	return c, nil
}
