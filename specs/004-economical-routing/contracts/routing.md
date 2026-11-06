# Routing, capacity and task-budget contract

**Feature**: `004-economical-routing`
**Version**: API 1 / execution `004-v1` / app `2.0.0-dev.4` / state schema 4
**Status**: Design for independent review; no implementation or live qualification claim

This extends [003 native execution](../../003-claude-interoperability/contracts/native-execution.md).
Its task text/pins, accepted native profiles, process/evidence ownership, controls, exact counters,
strict requests and receipt rules remain unless changed here. [data-model.md](../data-model.md)
defines entities, fixed profiles, scopes, bounds, schema and migration. Paths below are relative
to `/api/v1`; all are private authenticated owner routes.

## Security, compatibility and common representations

Retain numeric-loopback Host/Origin checks, bearer authentication, no redirects/proxies, no-store
responses, CSP and private local state. Mutations require the exact owner Origin. Reject unknown
or duplicate JSON/query members, invalid Unicode, wrong types, explicit null in request fields,
GET bodies, malformed/unsafe numeric values and trailing JSON. Optional fields are omitted; null
is reserved for documented unknown/absent response values. Existing valid text fidelity remains.

Random IDs retain 32 lowercase hexadecimal characters; context/profile identifiers are the fixed
named enums. Counts/revisions/sequences are exact decimal strings, never JSON numbers. Durations
are bounded JSON integers. Reuse the API-1 success/error envelope and globally unique request IDs.
No error has a receipt; a start acknowledgement remains admission rather than authoring success.

`GET /meta` declares app `2.0.0-dev.4`, execution `004-v1`, managed execution
`native_codex_claude`, `maintenanceContract: "owner-stop/1"`, and capabilities
`["workspace_foundation","managed_execution","economical_routing"]`. Capacity remains
`{resourceId:"managed-native",limit:1,activeAttemptId:null|ID}`. Current execution clients require
the exact current app/contract/capability; this is not permission to execute 004 on an old owner.
Extend the existing maintenance-only whitelist with accepted dev.3/API1/003-v1 and its exact
known metadata, retaining dev.2 support and exact old-owner/request binding. No auto-start,
PID kill, lock deletion, active-store migration or broad future-version acceptance is added.

`GET /execution/capabilities` retains the two native connections and bounds. It also declares
`routingContract: "004-v1"`, supported continuation categories `["pre_launch_ineligible"]`,
recognized categories `["pre_launch_ineligible","terminal_transient"]`, actual-account selection
unsupported and paid API execution unsupported. Category B is not qualified by SDK documentation
or a hypothetical fake success. Model selectors remain native selectors, not identity attestations.

## Optional task routing input and immutable capture

Existing `/tasks` and versioned guidance requests keep their old members. Add optional `routing`:

```json
{
  "suitability": "routine",
  "targetProfileId": "claude-haiku-economy",
  "expectedPolicyRevision": "1",
  "budget": {"admissionLimit": "2", "windowSeconds": 1800},
  "continuation": {
    "maxAutomaticAdmissions": "0",
    "categories": [],
    "allowReplacement": false,
    "allowEscalation": false
  }
}
```

Every member is optional; an omitted object has useful defaults. `suitability` defaults unknown;
values are `unknown`, `routine`, `general`. A user hint is attributed as such and cannot override
known conflicting constraints. `targetProfileId` is an explicit selected profile, not a new login
or account pin. It must be compatible with all literal pins. `expectedPolicyRevision`, when
supplied at direct new admission, compares the saved routing policy atomically; mismatch is a
revision conflict with no admission/debit. The saved autonomy policy/draft snapshot and its own
expected revision retain their separate 001/003 meaning.

The initial budget defaults to admissionLimit `"2"`, windowSeconds `1800`; limits are 1–16
admissions and 1–86400 seconds. `continuation` defaults to zero/empty/false/false; positive automatic
limits are at most 15 and cannot exceed total allowed admissions. `terminal_transient` cannot be
enabled on the accepted profiles: return `capability_unavailable` before new work. No native SDK,
paid path, credential handling or fallback option is configured by these fields.

`useCurrentPolicy` is valid only in guidance routing. A guidance
`routing.expectedPolicyRevision` requires `useCurrentPolicy:true` and guards the saved policy being
adopted; it is not a comparison against unrelated current settings when keeping a prior snapshot.
Adoption's expected policy revision has the same saved-policy guard as new-task capture.

Guidance may supply routing options and `routing.useCurrentPolicy: true` to capture current saved
routing preferences explicitly; otherwise its captured policy is retained. A supplied complete
routing option object replaces option choices relative to that captured policy, while omitted
guidance routing retains them. `routing.budget` is invalid on guidance: use the explicit budget
amendment below. A global/guidance policy edit never refills an existing ledger or restarts a clock.
An authority-changing guidance version is attributable and prevents obsolete queued dispatch.

`pins.harnessId`, `pins.model`, `pins.accountId` and `pins.paymentPath` remain literal and unchanged.
An explicit harness permits a valid uncatalogued literal native model selector with unresolved
access/suitability; do not silently replace it. A model-only pin with no unambiguous compatible
context is `pin_unavailable`. No fuzzy name-based harness inference is permitted. With an explicit
targetProfileId, any supplied harness must equal that profile's harness and any supplied model
must equal its fixed selector byte-for-byte; native-default's empty selector conflicts with a
nonempty model pin. Return `pin_unavailable` before admission/debit, never prefer one contradictory
explicit choice or normalize aliases into agreement. A model-only exact catalog-selector match
can establish its declared context; uncatalogued/ambiguous selectors require an explicit harness.
Existing well-formed pins.accountId remains an unsupported actual-account pin, and pins.paymentPath
paid_api remains unsupported; declaring an account association cannot satisfy either. No stronger
effective-model, monetary-cap or model-call-cap request fields are exposed: invented members such
as effectiveModelPin/maxCostUsd/maxModelCalls fail strict decoding as invalid_request. Council's
native-selector contract is not a stronger guarantee. Council-managed paid API execution
is unsupported even with a proposed provider/account/budget; no implicit paid fallback occurs.

New admission success remains `{task,version,attempt}` with receipt, plus optional `routingDecision`
and `budget` snapshots. The decision records actual selected context/selector separately from
original pins. New attempt reads expose optional `routingDecisionId` and `resolvedTarget`.
Old stored response envelopes/JSON are not enriched during replay or migration. A target admitted
to the queue is frozen; later changes cannot alter its connection or selector in place.

## Published decision policy

Apply, in order: known task constraints/capabilities/permissions/pins/payment; current negative
eligibility controls; fresh reported availability over unknown; applicable economical profile
role; comparable current reference prices; captured stable preference order. Preserve all reasons
and unknowns. Routine selection/status/waits invoke zero model calls.

The fixed catalog/selectors and dated sources are in the data model/research. Known sourced native
version incompatibility excludes that profile; unknown version/access is never verified. Unknown
task suitability uses a conservative compatible native-default profile unless explicit profile/
model intent applies. An optional explicit suitability hint or the plan's tightly bounded
whole-task recognized-operation grammar supplies an attributed preference assessment. Filename
existence/content evidence is collected outside SQL; short descriptions, isolated keywords,
marketing names and unknown availability alone cannot establish routine suitability or quality.
If the assessment changes or required constraints cannot be met, block specifically; do not
silently clip context or promote an unsupported small target.

With fresh user-reported busy Codex and suitable available Claude, the eligible Claude context
receives background work. This means a new managed one-shot worker, never alteration/resume of
a personal conversation or a vendor-global model change. With no optional reports/prices, known
compatible native contexts remain eligible under default `unknownAvailability: "allow"`, labelled
unknown. Saved `require_positive` requires fresh available evidence for BOTH occupancy and
allowance covering the selected context. Every applicable negative still wins. An account-positive
report counts only if its immutable bindings include that context and its account link is still
current; relinking cannot transfer/freshen it. Context-direct positives apply only to that context
with original times, not as verified availability of a newly labelled account.

Reference-price comparison is Pareto comparison, not a guessed token mix: both input/output rates
must be known/current with identical currency/unit/basis and within the exact arithmetic domain
(32 significant digits, written exponent -18..18, normalized fractional places <=18). X is cheaper
iff both rates are <= Y and one is lower; equal rates tie. Crossed/missing/expired/out-of-domain
or unlike-basis/currency rates are incomparable and use saved order. Valid 64-byte native/price
lexemes outside the domain are retained, never expanded/clipped into a comparable value. Price
unit is per_million_tokens; basis must explicitly be api_list or declared_marginal, always a user
reference, never actual cost. Missing basis is not a verified marginal-price default.
Absent unit/basis remains unknown/incomparable rather than receiving a price default. Within an
already equal eligibility/availability/tier group, compute the nondominated frontier using all
bounded comparable pairs, then use captured stable order among that frontier. Unknown/incomparable
prices cannot be assumed cheaper or dominated. Do not use Pareto-plus-tie-order as a pairwise
sort comparator: it can cycle and is not a valid strict ordering.

## Configuration and capacity routes

Every mutation requires `requestId`; exact replay precedes revision/configuration checks. All
responses use existing receipts/status conventions. Existing five-entry discovery remains
metadata-only; these routes do not run vendor version/auth/login/model commands.

| Method / route | Request or query | Success data |
| --- | --- | --- |
| `GET /routing/policy` | None | Current RoutingPolicy. |
| `PUT /routing/policy` | `{requestId,expectedRevision,economyOrder?,unknownAvailability?,defaultBudget?,continuation?}`; supplied members update saved defaults only | Updated RoutingPolicy + receipt, 200. |
| `GET /routing/contexts` | None | Fixed `{items:ContextView[]}` with named accountLink/capacity fields below; at most two. |
| `PUT /routing/contexts/{id}` | `{requestId,expectedRevision,label?,enabled?}` | Updated context + receipt, 200. No executable/configuration fields. |
| `GET /routing/profiles` | Optional `contextId`; fixed identifier only | `{items:TargetProfile[]}`, at most six. |
| `PUT /routing/profiles/{id}` | `{requestId,expectedRevision,enabled?,referencePrice?,clearReferencePrice?}`; price and clear mutually exclusive | Updated profile + receipt, 200. Fixed selector/source/role cannot change. |
| `GET /routing/accounts` | Optional `limit,cursor` | Stable `{items:DeclaredAccount[],nextCursor}`. |
| `POST /routing/accounts` | `{requestId,label,providerLabel?,entitlement?}` | DeclaredAccount + receipt, 201. No native binding. |
| `PUT /routing/accounts/{id}` | `{requestId,expectedRevision,label?,providerLabel?,entitlement?}` | Updated declaration + receipt, 200; no capacity reset. |
| `PUT /routing/contexts/{id}/account-link` | `{requestId,expectedRevision,declaredAccountId? ,unlink?}`; exactly account ID or `unlink:true` | AccountLink + receipt, 200; expected revision is the link revision. |
| `GET /routing/capacity` | Exactly one `contextId` or `accountId` | CapacityView with revision, source/age/expiry/current effective facts and separate managed occupancy. |
| `GET /routing/reports` | Exactly one subject filter `contextId`/`accountId`; optional `limit,cursor` | Stable immutable report page. |
| `GET /routing/reports/{id}` | Random report ID | `{report,unresolvedScopeKeys}`; bounded immutable report plus current retained bindings, no new observation. |
| `POST /routing/reports` | CapacityReportInput below | `{report,capacity}` + receipt, 201. |

`economyOrder` contains unique fixed profile IDs, at most six; missing IDs use the published fixed
catalog tie order afterward. `unknownAvailability` is `allow` or `require_positive`. Optional
referencePrice follows the data model; `clearReferencePrice` must be true when supplied. Profiles
can be disabled but not replaced with arbitrary selectors/commands. No account deletion operation
is introduced to bypass negative history.

### Context and capacity response types

The following notation fixes wire names/types, not a new runtime dependency. `ID` is a random ID,
`Time` is an owner UTC timestamp, `Count` is an exact signed-64-bounded decimal string and
`Subject` is `{kind:"context"|"account",id:string}` with the matching validated identifier.

```typescript
type AccountLink = {
  contextId: string; declaredAccountId: ID | null; revision: Count;
  updatedAt: Time; source: "user_declaration";
};
type ContextView = NativeContext & { accountLink: AccountLink; capacity: CapacityView };
// NativeContext.capabilities is the named accepted native connection declaration.
type ReportState = "busy"|"available"|"unknown"|"exhausted"|"temporary_throttle"|
  "temporary_overload"|"authentication_denied"|"model_denied"|"permission_denied";
type ReportRef = {
  reportId: ID; subject: Subject; dimension: "occupancy"|"allowance"|"service";
  state: ReportState; source: "user_report"|"native_terminal";
  observedAt: Time; receivedAt: Time; expiresAt: Time|null; retryAfter: Time|null;
  freshness: "fresh"|"expired"|"elapsed_cooldown"|"unknown"; retained: boolean;
  inheritedFrom: {accountId: ID; linkRevision: Count; linkedAt: Time}|null;
};
type CapacityDimension = {
  effectiveState: ReportState; latestReport: ReportRef|null;
  blockingReports: ReportRef[]; positiveReportIds: ID[];
};
type CapacityView = {
  subject: Subject; revision: Count; asOf: Time; accountLink: AccountLink|null;
  dimensions: { occupancy: CapacityDimension; allowance: CapacityDimension; service: CapacityDimension };
  reportEligibility: {
    policy: "allow"|"require_positive"; permitted: boolean;
    positiveCoverage: { occupancy: boolean; allowance: boolean }; reasonCodes: string[];
  };
  managedOccupancy: {
    resourceId: "managed-native"; limit: 1; activeAttemptId: ID|null;
    activeContextId: "codex-native"|"claude-native"|null; claimsSubject: boolean;
  };
};
type ReferencePrice = {
  currency: string; unit: "per_million_tokens"|null;
  basis: "api_list"|"declared_marginal"|null;
  inputPerMillion: string|null; outputPerMillion: string|null;
  sourceKind: "user_reference"; sourceLabel: string; sourceUrl: string|null;
  observedAt: Time; validUntil: Time|null;
};
```

Dimension states use exactly the dimension/state enums listed in the data model. Unknown data is
effectiveState unknown and latestReport null when absent. BlockingReports identifies each still-
applicable denial, including stale retained exhaustion; it is not hidden behind the latest head.
An expired positive cannot appear in positiveReportIds. AccountLink is always present in a context
view, initially with null declaredAccountId/revision 1; its own revision guards link PUT. In an
account-scoped CapacityView accountLink is null. ReportEligibility covers only the report policy,
not verified authentication/model entitlement or the complete routing prerequisite set.
Managed occupancy is owner evidence, not external activity; claimsSubject is true only for the
selected context or its current explicitly associated account. Unknown active identity remains null.
No read updates TTL or creates provider usage. Return at most 66 blocking references per dimension;
one CapacityView is bounded to 128 KiB and the fixed contexts response to 256 KiB. Full report
detail is available by ID/history; none of these bounds truncates an active denial into eligibility.
ReferencePrice input omits response-only sourceKind. Currency/sourceLabel/observedAt are required;
unit, basis, rates, sourceUrl and validUntil may be omitted and then remain null/unknown. Provided
unit/basis must use the enums above. Missing unit/basis/rates means incomparable, not a verified
price default. SourceKind is server-stamped user_reference, irrespective of the cited URL.

A capacity report request is:

```json
{
  "requestId": "11111111111111111111111111111111",
  "subject": {"kind": "context", "id": "codex-native"},
  "expectedRevision": "1",
  "dimension": "occupancy",
  "state": "busy",
  "validForSeconds": 900
}
```

Optional members are `observedAt`, `retryAfter`, `note`, `correctionOf`, `correctionAction`.
`observedAt` defaults to owner receipt time; future times reject. `validForSeconds` defaults 900,
range 1–86400, and does not freshen an old observed time. `retryAfter` is a future UTC timestamp
only for a supported temporary service report. `expectedRevision` is the current subject capacity
revision. Public source is always `user_report`, not a caller-selected native/authentication fact.
Allowed dimensions/states are enumerated in the data model; an occupancy available report makes
no numeric allowance claim.

An explicit correction supplies `correctionOf: REPORT_ID`, `correctionAction: "recover"|"withdraw"`
and the same subject/dimension. Recover supplies the newly declared state; withdraw supplies
`unknown`. It may reference ANY still-unresolved retained negative of the same original subject
and dimension, including an older inherited exhaustion when a newer subject head exists. Supply
the CURRENT subject capacity revision, not the old report's creation revision. Stale revisions,
wrong subjects/dimensions and already-resolved reports conflict, except exact receipt replay.
Resolve only that exact report ID wherever currently retained, whether from original bindings
or later link inheritance; never
clear another/newer denial. A newly recorded recovery positive has current bindings and cannot
freshen an old/disconnected association. For example, clearing old N while newer M remains returns
a view still blocked by M. Store the targeted resolution in existing report/current-state JSON
and receipts, without a new table or scanning all history. A positive report without an explicit correction cannot
erase unresolved exhaustion. Unlinking/relabeling carries original bindings forward. The returned
capacity view identifies exact supporting report IDs, unknowns and stale exhaustion, not just a
green readiness label. Native typed evidence may update controls internally only where supported;
fake output never becomes actual provider availability or measured quota.
If A's exhaustion report predates C's link to A, C is nevertheless denied by the current account
negative. Linking copies a sticky reference with accountId/linkRevision/linkedAt into C's current
control JSON; unlink cannot clear it. The old report/original bindings remain immutable. A targeted
correction clears only that ID wherever retained, including these inherited references, while any
newer denial remains effective. Positive availability is NOT inherited onto newly linked contexts.

## Task routing, budget and adoption routes

| Method / route | Input | Success data |
| --- | --- | --- |
| `GET /tasks/{id}/routing` | None | `{authority,latestDecision,adoptionRequired}`; nullable old-task fields stay null. |
| `GET /tasks/{id}/routing/decisions` | Optional `limit,cursor` | Stable `{items:RoutingDecision[],nextCursor}`. |
| `GET /tasks/{id}/budget` | None | `{budget,adoptionRequired}`; budget null for an unadopted legacy task, not a fabricated exhausted zero. |
| `GET /tasks/{id}/budget/ledger` | Optional `limit,cursor` | Stable `{items:BudgetLedgerEntry[],nextCursor}`. |
| `POST /tasks/{id}/budget-amendments` | BudgetAmendmentInput below | `{task,version?,amendment,budget,authority?}` + receipt, 200; no worker. |

All history pages retain the existing cursor contract: default limit 30, range 1–100, opaque
filter-bound high-water/last-seen cursor, stable order, no duplicates/skips, exact strings, empty
array plus null nextCursor at end. Invalid/foreign/unsafe cursors reject without mutation. A full
decision is bounded; event previews contain references, not copied transcripts/catalog histories.

Grant request:

```json
{
  "requestId": "22222222222222222222222222222222",
  "action": "grant",
  "expectedRevision": "2",
  "expectedBudgetRevision": "2",
  "additionalAdmissions": "1"
}
```

Optional `newWindowSeconds` explicitly opens a new finite window after prior work settles with
verified cleanup and current owner/gate controls pass. It does not reset cumulative use; its clock
anchors at the first next budgeted admission. A unit-only grant keeps the existing deadline.
Normal grants advance Task.revision and Budget.revision, not guidance version. Admission grants
are 1–16 exact string units; total arithmetic is checked. No budget is charged in currency, and
an expired/uncertain reservation is not refunded. An amendment does not restart automatically.
It also does not re-arm a previously blocked/expired continuation proof. The next execution after
such an explicit amendment requires a separate Start request; an older terminal row is not a
newly authorized scheduling event.

Legacy adoption request:

```json
{
  "requestId": "33333333333333333333333333333333",
  "action": "adopt",
  "expectedRevision": "4",
  "additionalAdmissions": "2",
  "newWindowSeconds": 1800,
  "routing": {"suitability": "unknown"}
}
```

`adopt` is valid only without existing 004 authority, requires no unsettled/unknown prior ownership,
and requires newWindowSeconds/additionalAdmissions; expectedBudgetRevision is absent. It appends
a new immutable task version preserving original/guidance/pins and captures routing authority.
The returned budget shows known legacy admissions separately and grants exactly the additional
units. Migration/ordinary guidance does not adopt. `routing.budget` is invalid here because the
amendment itself grants authority. An already adopted task uses `grant`, not another legacy baseline.

An optional UI “Adopt and start” action composes two requests: retain the exact adoption request
identity/body until acknowledged, then start the returned exact task revision/version with a
separate retained ID. A lost reply never repeats adoption or creates a second attempt. No extra
plan-confirmation or per-tool permission ritual is introduced for ordinary new tasks.

New task success captures route, authority, initial grant, first window opening, consumed unit,
attempt and receipt in one transaction. Existing-task Start requires current task/version and
budget authority; it creates another immutable decision/debit/attempt without resetting time.
Admitted-but-blocked attempts still consume a unit. The first-admission-anchored common deadline
includes queue/preparation/execution/settlement. Existing timeoutSeconds is a per-attempt ceiling:
new Attempt.deadlineAt is the earlier of that ceiling and the common window deadline. Changing
guidance timeout cannot extend the task window or a prior attempt deadline. Deadline expiry
prevents new dispatch and requests controlled Stop; cleanup can remain visibly pending/fenced.

## Dispatch and closed continuation rules

Gather installation/recognized-operation filesystem facts outside the transaction. The typed
admission/dispatch facts identify exact profile/context/link/report/policy/source versions and
observation time. Under the serialized owner/store boundary recheck current task/version, literal
pins, target compatibility, open owner epoch/global fence, Stop, the single claim, capacity heads,
remaining admission/automatic units and task/attempt deadline. No filesystem/vendor I/O or generic
I/O callback executes inside SQL. Commit durable intent before any native Prepare/Create, then
apply the accepted guarded creation/start ordering without weakening containment.

If a queued target becomes ineligible before dispatch, settle its attempt blocked with an
attributable reason. Do not edit its connection, selector, receipt or consumed unit. Default
continuation is off. A permitted replacement requires a new linked decision/attempt and another
unit; it is not a free replan of admitted work. Reports after launch affect future admission,
not retroactive rewriting of running work or automatic cancellation without an applicable control.

Recognized category A is `pre_launch_ineligible`: current supported installation/capability or
capacity evidence changed eligibility; prior attempt durably settled blocked; affirmative typed
proof says no Prepare/Create attempted, no expected output missing, verified not-launched cleanup.
Storage/identity/source/ownership uncertainty is excluded. The intentional absence of worker output
is the only incomplete-output exception. Missing/corrupt expected evidence never qualifies.

Recognized category B is `terminal_transient`: qualified explicit structured temporary throttle/
overload terminal discriminator, complete expected input/output, known root exit and verified
cleanup. Current profiles do not support it. Enabling it returns `capability_unavailable`; do not
classify retry progress, generic 429/529, prose, authentication/model-access failures or SDK-only
fields as proof or exhaustion. Future review/check correction is outside 004.

Automatic next admission atomically consumes a unique predecessor proof once, under captured
finite authority and current controls. Tiers are economy 0, general 1 and unknown for native-default
or uncatalogued selectors. A changed context+selector always requires allowReplacement and also
allowEscalation if either tier is unknown, the new tier is higher, or its price is higher/dominated
by the old price, missing, crossed or incomparable. Equivalently, a changed target avoids
escalation only when both tiers are known with new<=old AND the new comparable price is cheaper
or equal in both rates.
A lower declared tier alone cannot establish non-escalation with unknown costs. A retry of the
same exact context+selector is exempt from target replacement/escalation comparison; category,
automatic-unit, total-budget and control guards still apply. Initial Auto selection uses its
published preference/fallback rules, not this comparison against a predecessor. Prior failures remain
unchanged; candidates are retained, never assumed integrated into a new workspace. Accepted Stop,
owner closing/loss, evidence/commit ambiguity, corruption or unknown cleanup prevents automatic
replacement through either harness. Restart never creates new authority from old terminal rows.

## Usage views and UI/CLI parity

`GET /tasks/{id}/usage?limit=&cursor=` pages attempt history using the same high-water cursor.
The concrete response below uses existing UsageObservation/NativeModelsObservation from 003.
`PageSum` is an exact BigInt decimal string bounded by 100 times signed-64 maximum
(`922337203685477580700`), response-only; every input, persisted count and per-observation counter
retains its signed-64 bound. Select only ONE explicit usage observation per attempt; modelUsage
previews are displayed but never added into the page subtotal. A null knownValue means no known
contributors; zero is emitted only if reported contributing values sum to zero.

```typescript
type UsageItem = {
  attemptId: ID; attemptSequence: Count; executionMode: "native"|"fake";
  outcome: "succeeded"|"failed"|"cancelled"|"blocked"|"interrupted"|null;
  selectedUsageScope: "top_level"|"whole_tree"|"unknown"|"fake_unmeasured";
  usage: UsageObservation; nativeModels: NativeModelsObservation|null;
  evidenceArtifactIds: ID[];
};
type CounterSubtotal = {
  knownValue: PageSum|null; contributingAttemptIds: ID[]; missingAttemptIds: ID[];
};
type ScopeSubtotal = {
  inputTokens: CounterSubtotal; outputTokens: CounterSubtotal;
  cachedInputTokens: CounterSubtotal; cacheCreationInputTokens: CounterSubtotal;
};
type EstimateItem = {
  attemptId: ID; model: string; currency: "USD"; value: string;
  scope: "whole_tree_model"; source: string; observedAt: Time;
  evidenceArtifactIds: ID[]; previewCompleteness: "complete"|"partial"|"unknown";
};
type UsagePage = {
  taskId: ID; items: UsageItem[]; nextCursor: string|null;
  knownSubtotal: {
    scope: "returned_attempts";
    byUsageScope: { top_level: ScopeSubtotal; whole_tree: ScopeSubtotal };
  };
  coverage: {
    scope: "returned_attempts"; status: "complete"|"partial"|"unknown";
    includedAttemptIds: ID[]; unknownAttemptIds: ID[]; excludedFakeAttemptIds: ID[];
  };
  estimates: EstimateItem[];
  referencePrices: {attemptId: ID; decisionId: ID; profileId: string|null; referencePrice: ReferencePrice}[];
  charges: { status: "unknown"; items: [] };
};
```

Select the existing explicit UsageObservation scope if it is top_level/whole_tree with valid
sourced counters; otherwise keep unknown. Legacy absent scope is not silently inferred from a
model name. Fake items have selectedUsageScope fake_unmeasured and contribute no provider sum.
Sum each token field independently within its selected-scope bucket; never add input/cache fields
into a synthetic total. NativeModels is preserved as a separately scoped preview, not a second
aggregate contribution. At most 100 observations therefore bound each PageSum; no overflow, float
conversion, clipping or full-history scan is needed. Exact 16-entry model previews and their
costEstimateUsd lexemes (including 1e999) remain unchanged. Estimates merely reference/display
those native cost entries; they are not rounded or treated as charges/comparable reference prices.

Coverage complete means all selected native observations/counters in THIS page are represented,
not complete underlying provider work or a complete task total. Missing scope/counters or fake/
unknown entries keep coverage partial/unknown as appropriate. Arrays name returned-page attempt
IDs only; referencePrices contains captured decision references that actually exist, not current
price substitutions. Charges is always unknown for the accepted native interfaces; empty items
means no charge receipts, not zero cost. The response is bounded to 4 MiB with byte-budget paging:
include at most the requested limit, stopping BEFORE the next whole attempt item plus all its
derived estimate/reference/coverage/subtotal fields would exceed the encoded response bound.
Return nextCursor for the last included attempt with the same high-water mark; do not clip an
item/lexeme or skip the first excluded row. The next page considers that row normally. Recompute
subtotals/coverage only for included items. One item is bounded by existing observation/catalog
limits and fits alone; an empty history returns empty items/null cursor, not a synthetic count.
This accounts for JSON escaping as well as field lengths. Ledger/history is not scanned or
materialized into a new aggregate table. Even if one
page happens to contain all history, knownSubtotal remains labelled returned_attempts.

Include successful, failed, cancelled and interrupted work. Do not add overlapping top-level usage
to whole-tree modelUsage, duplicate repeated native messages, erase failed-work usage on retry or
evaluate extreme estimate lexemes into unbounded/rounded values. Known subtotal and missing portion
stay separate. Fake observations remain unmeasured; curated references, client estimates and any
genuinely sourced charges are differently labelled. No quota decrement or saving is inferred from
managed admission count, selector names or a changed reference price.

CLI mappings extend existing commands without altering old option meanings:

| Command | HTTP meaning |
| --- | --- |
| `routing policy` / `routing policy set --expected-revision R ...` | Read/update saved routing preferences; optional flags map the documented fields. |
| `routing contexts` / `routing profiles [--context ID]` | Fixed declarations and unknowns; no probe/login. |
| `routing accounts` / `routing account add --label LABEL [--provider LABEL]` | Inspect/create declared accounts; no actual-account guarantee. |
| `routing link --context ID --declared-account ID --expected-revision R` | Explicit context link; `--unlink` replaces declared-account for removal. |
| `routing report get ID` | `GET /routing/reports/{id}` returns the immutable report and its current unresolved scope keys without renewing it. |
| `routing report --context ID --dimension occupancy --state busy --expected-revision R [--valid-for-seconds N]` | User report; `--account ID` replaces context for account scope. |
| `routing report --context ID --dimension allowance --state available --expected-revision R --correction-of REPORT --correction-action recover` | Explicit sourced correction, not automatic refill. |
| `tasks routing ID` / `tasks routing-decisions ID [--limit N --cursor C]` | Current authority and immutable decision history. |
| `tasks budget ID` / `tasks budget-ledger ID [--limit N --cursor C]` | Exact budget/ledger; native calls/charges remain distinct. |
| `tasks budget grant ID --expected-revision R --expected-budget-revision B --additional-admissions N [--new-window-seconds N]` | Explicit additional authority, no implicit worker. |
| `tasks adopt ID --expected-revision R --additional-admissions N --window-seconds N` | Explicit legacy adoption; optional routing choices, separate later Start. |
| `tasks usage ID [--limit N --cursor C]` | Sourced non-overlapping usage/unknown coverage. |

Existing start/guidance add optional `--suitability`, `--target-profile`, and routing policy/budget/
continuation flags corresponding exactly to allowed request members; existing `--harness`/`--model`
still produce literal pins. Budget flags are invalid on guidance. Every mutation also supports
retained `--request-id`, `--json` and `--api-version 1`; strict duplicate/unknown flag validation
and stdout/diagnostic separation remain. The normal UI keeps these optional controls out of the
project/task path, shows concise decision reasons and preserves newer edits/selections under
delayed reads or lost mutation acknowledgements.

The exact optional flag spellings are `--expected-routing-policy-revision`, `--budget-admissions`,
`--budget-window-seconds`, `--max-automatic-admissions`, repeatable unique `--allow-continuation`,
`--allow-replacement` and `--allow-escalation`; guidance adds `--use-current-routing-policy`.
`routing policy set` uses `--unknown-availability`, repeatable unique `--prefer-profile`,
`--default-admission-limit`, `--default-window-seconds` and the same continuation flags. Context
and account edits are `routing context set ID` / `routing account set ID` with `--expected-revision`
and their allowed label/enabled/declaration flags. Profile edits are `routing profile set ID`
with `--expected-revision`, optional `--enabled true|false`, and either `--reference-price-file PATH`
or `--clear-reference-price`. The optional price file contains exactly the documented price object;
it is never required for task start. `routing capacity` and `routing reports` take exactly one
`--context`/`--account`; reports additionally support `--limit`/`--cursor`. A report mutation's
optional flags are `--observed-at`, `--retry-after`, `--note`, `--correction-of`,
`--correction-action` and `--valid-for-seconds`. Enum values use the wire spellings. Omitting a
boolean retains its documented default/current value; an explicitly supplied false is represented
as false rather than being mistaken for omission.

## Errors, receipts and required evidence

Reuse existing `revision_conflict`, `request_conflict`, `pin_unavailable`, `capability_unavailable`,
`dispatch_fenced`, `owner_closing`, `storage_failed`, `write_uncertain` and known precommit
`operation_interrupted` meanings. Add 409 `routing_adoption_required`, 409 `budget_conflict`,
422 `budget_exhausted`, 422 `budget_expired`, 422 `routing_unavailable` and 409 `report_conflict`.
Explain the exact next action without task bodies, credentials, tokens or raw vendor diagnostics.

Global receipt operations are `routing.policy.update`, `routing.context.update`,
`routing.profile.update`, `routing.accounts.create`, `routing.accounts.update`, `routing.links.update`,
`routing.reports.create`, `tasks.budget.amend`; existing start/guidance/attempt/control/acceptance
operation names remain unchanged. Fingerprints include exact route target, supplied expected
revisions and input; do not inject current defaults/catalog/report state into retry fingerprints.
Append snapshots and budget effects in the same commit as their original operation receipt.
Automatic decisions use durable owner-generated request identities plus unique predecessor
consumption; losing an acknowledgement never creates another worker or budget effect.

Verification must cover fresh minimal Auto, known/unknown/routine suitability, busy-Codex/available-
Claude, explicit aliases/account links and sticky reports, literal/unsupported pins/payment, large
exact counts, pre-admission proposals versus immutable queued targets, two-unit/shared-window
accounting, A proof before creation, unsupported B, Stop/fault/owner-loss fences, explicit adoption,
byte-preserving additive migration, bounded usage/estimate displays and UI/CLI parity with actual
owned local fixtures. The regular developer/native/race/browser gates remain required. This
contract authorizes no live provider task and supplies no measured saving or qualification result.
