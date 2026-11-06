# Data model: Economical routing and account capacity

**Feature**: `004-economical-routing`
**Branch**: `refactor`
**Contract**: API 1 / execution `004-v1` / application `2.0.0-dev.4` / schema 4
**Status**: Design for independent review; not implementation or qualification evidence

This extends [003](../003-claude-interoperability/data-model.md) without rewriting its task,
version, attempt, terminal, source/candidate, observation or receipt JSON. The public contract is
[contracts/routing.md](contracts/routing.md); source assumptions are in [research.md](research.md).
There is still one global `managed-native` worker resource. Accounts are declarations and capacity
associations, not credential containers or new executable login contexts.

## Values, limits and compatibility

Existing random IDs are exactly 32 lowercase hexadecimal characters. Existing connection/resource
identifiers and the six fixed profile identifiers below are named enum values, not random IDs.
Revisions, versions, sequences, admissions and persisted/per-observation counters are canonical exact decimal strings through
`9223372036854775807`; counts permit `0`. Never convert them through a floating-point number.
Small durations/limits remain bounded JSON integers. Dates retain UTC RFC 3339 milliseconds and
`Z`; owner receipt order, not a client clock, serializes control. Text is valid Unicode preserved
without normalization, trimming, BOM removal or newline conversion.

Limits in 004: two fixed contexts, six fixed profiles, at most 64 declared account records;
account labels 128 UTF-8 bytes, user notes 512 bytes, source labels 128 bytes and optional source
URLs 1024 bytes. Reference decimal lexemes are at most 64 ASCII bytes. Initial/additional
admission grants are 1–16 units; all accumulated counters still use checked signed-64 arithmetic.
Window/report durations are 1–86400 seconds. Defaults are two admission units, 1800 seconds for
the task window, 900 seconds for occupancy/positive reports and zero automatic continuations.
Task routing input is at most 16 KiB; one immutable decision is at most 64 KiB. Events retain
their 16 KiB bound and refer to decisions by ID. Lists use existing stable bounded pagination.
The sole larger-count response type is a derived page token subtotal: exact bounded BigInt decimal
through `922337203685477580700` (100 selected per-attempt observations times signed-64 maximum).
It is never persisted or accepted as a revision, cursor, budget or count input. Per-model previews
are displayed separately and are not added to these page subtotals. Sum each token field
separately; cached/input categories can overlap and are not added into a synthetic total.

No new default is inserted into the incoming `pins`, routing request or receipt fingerprint.
Resolved defaults are stored separately as captured authority. New optional read enrichments are
absent for old records. An old envelope replays unchanged before current routing prerequisites.

## Fixed native contexts and profiles

`NativeContext` contains `id`, `harnessId`, `connectionId`, `label`, `enabled`, `revision`,
`updatedAt` and named `capabilities` containing the accepted native connection declaration. IDs are `codex-native` and
`claude-native`; the connection ID equals the context ID in this slice. Harness IDs are `codex`
and `claude-code`. There are no user-supplied executable paths, argv, environment variables,
credential paths, alternate login directories, remote endpoints or extra contexts. Installation
and invocation observations remain separate from this configuration.

`TargetProfile` contains `id`, `contextId`, `modelSelector`, `role`, `enabled`, `revision`,
`source`, `referencePrice` and `updatedAt`. The fixed catalog is:

| Profile ID | Context | Exact native selector | Declared role |
| --- | --- | --- | --- |
| `codex-native-default` | `codex-native` | Empty / omit model flag | `native_default` |
| `codex-luna-economy` | `codex-native` | `gpt-6-luna` | `economy` |
| `codex-sol-general` | `codex-native` | `gpt-6.1-sol` | `general` |
| `claude-native-default` | `claude-native` | Empty / omit model flag | `native_default` |
| `claude-haiku-economy` | `claude-native` | `haiku` | `economy` |
| `claude-sonnet-general` | `claude-native` | `sonnet` | `general` |

`source` is `{kind, label, url, readDate, minimumNativeVersion}`. Built-in source kind is
`documented_profile`, with dated references from research. Claude's documented alias requirements
are 2.1.293 for Haiku and 2.1.284 for Sonnet, separate from the 2.1.259 transport profile. Codex
selector minimum versions remain null. Unknown installed version/access remains unknown; a sourced
known-old version excludes an incompatible profile. A role is a declared policy assumption, not a
measured intelligence score or entitlement. Catalog identity/selectors/role/source cannot be
edited through preferences; users can enable/disable a profile and attach an optional reference
price. Explicit uncatalogued native selectors still work with an unambiguous compatible harness.

`referencePrice` is null or `{currency, unit, basis, inputPerMillion, outputPerMillion, sourceKind, sourceLabel, sourceUrl,
observedAt, validUntil}`. Currency is a three-uppercase-letter declaration; values are null or
bounded nonnegative JSON-number lexemes stored as strings. A user-supplied reference remains
`user_reference`, even if it cites a vendor URL; Council does not fetch/verify it automatically.
When supplied, `unit` is `per_million_tokens`; `basis` is explicitly `api_list` or `declared_marginal`, both
user-declared references rather than measured charges. Missing basis is never defaulted to a
verified marginal price. Comparable prices require both input/output rates, current validity,
identical currency/unit/basis and exact arithmetic within 32 significant digits, written decimal
exponent -18 through 18, and at most 18 normalized fractional places. Valid stored lexemes outside
that domain remain unchanged but incomparable. X is cheaper only if both its rates are <= Y and
at least one is strictly lower; equal rates tie. Crossed, missing, expired, different-basis/currency
or out-of-domain rates are incomparable and fall back to captured stable order. Do not infer a
workload mixture, expand huge exponents, convert currencies or claim a measured charge/saving.
Absent unit/basis is stored as null/unknown and makes comparison incomparable, never a fabricated
default. Within an otherwise equal eligibility/availability/tier group, compare all bounded
candidate pairs to form the nondominated frontier, then select by captured stable order among
that frontier. Incomparables/unknown prices stay on the frontier unless actually dominated by
comparable evidence. Never implement pairwise Pareto plus tie order as a sort comparator: that
relation is non-transitive and can cycle.

## Declared accounts, links and capacity observations

`DeclaredAccount` contains `id`, `label`, optional `providerLabel`, `entitlement` (`subscription`,
`local`, `unknown`), `revision`, `createdAt`, `updatedAt`. All are user declarations. Actual native
provider/account facts remain sourced invocation observations or unknown. Two account records
can mention the same harness without becoming selectable logins. No deletion/recreation shortcut
is supplied to erase history; an unused declaration can simply remain unlinked.

`AccountLink` contains `contextId`, `declaredAccountId` (nullable), `revision`, `updatedAt` and
`source: "user_declaration"`. Each context has at most one current link; several contexts/profiles
may explicitly share an account. Link updates preserve receipt/history snapshots and negative
observation bindings. No association is inferred from a harness/model name. `pins.accountId`
continues to mean an exact actual-account requirement and remains unsupported; a link cannot
satisfy it.

`CapacityScope` is `context:<fixed-context-id>` or `account:<random-account-id>`. Each has a revision
and a bounded current control view. `CapacityReport` is immutable and contains:

| Field | Meaning |
| --- | --- |
| `id`, `sequence`, `requestId`, `subject`, `dimension` | Stable report, owner receipt sequence and exact subject `{kind:"context"|"account",id}`. |
| `state` | Dimension-specific state below. |
| `source`, `observedAt`, `receivedAt` | Public reports have source `user_report`; native facts use only a qualified internal typed source. A URL/label supplied by a caller does not change source authority. |
| `validForSeconds`, `expiresAt`, `retryAfter` | Finite validity/cooldown facts, nullable where inapplicable. Defaults do not turn an old report into a fresh one. |
| `correctionOf`, `correctionAction` | Null or explicit `recover`/`withdraw` referencing a still-applicable retained report, which need not be the newest head. |
| `bindings` | Immutable context/account scope IDs to which this report applied when accepted, including linked contexts for an account report and the current declared account for a context report. |
| `note` | Optional bounded user explanation; never executable control or proof of provider telemetry. |

Dimensions/states are:

- `occupancy`: `busy`, `available`, `unknown`.
- `allowance`: `available`, `exhausted`, `unknown` (not a fabricated numeric quota balance).
- `service`: `temporary_throttle`, `temporary_overload`, `authentication_denied`, `model_denied`,
  `permission_denied`, `unknown`.

The concrete CapacityView/ContextView wire definitions in the contract are authoritative. Context
views always expose `accountLink` with its own revision, even when declaredAccountId is null;
that link revision, not the context/capacity revision, guards link edits. Capacity views expose
`asOf`, independent dimension report references, retained blocking IDs and separate managed occupancy.

The current view keeps separate references for each dimension's latest observation and unresolved
exhaustion; an availability report cannot erase another dimension. Managed occupancy is derived
only from the global claim/attempt and is not writable by a report. Report freshness is derived
at read/dispatch time using the owner clock without a model call or a synthetic renewal write.

Reject future-dated observations and stale/conflicting normal updates against the supplied scope
revision. Retain immutable receipt order and explicit correction references. An older positive
cannot supersede newer negative evidence, including when it arrives later. Expired occupancy or
positive availability becomes unknown. A temporary cooldown becoming elapsed is a distinct fact,
not replenishment. Expired non-exhaustion reports do not prove that their cause was fixed.
Exhaustion remains denying, with its age visible, until explicit attributable recovery/withdrawal;
passing reset/expiry, editing prices, relinking or clearing an unrelated report cannot remove it.

Bindings are carried into the current views atomically. Relinking cannot drop a context's retained
denial/exhaustion reference inherited from its earlier association. A correction may reference
any still-unresolved retained negative for the same original subject/dimension, even when a newer
subject head exists. It must supply the current subject capacity revision. Resolve only that exact
report ID wherever currently retained, from origin bindings or later link inheritance; newer/other
denial IDs stay active. This can clear an
older inherited exhaustion without resetting a newer denial. Positive recovery coverage uses
current bindings/current links only, never historical associations. Already-resolved references
conflict except for exact receipt replay. Resolution is retained in current-state JSON, the new
correction report and its receipt; prior report bodies remain immutable. No extra table or
history scan is required.
When account A is already exhausted before context C links to A, C is denied by the current account
negative and receives a sticky reference `{reportId,accountId,linkRevision,linkedAt}` in its current
control JSON. Unlinking C cannot remove it. Do not rewrite the report's immutable original bindings
to pretend C was linked at observation time. An exact correction of that report removes its ID
from origin and link-inherited retained references, without touching newer IDs. Positive reports
are never inherited this way: original-binding-plus-current-link coverage remains mandatory.
The raw history remains immutable. At most 64 account-origin negative references can be attached
to either fixed context; routine views expose bounded summaries/references and page report history.

`require_positive` requires fresh available reports for both occupancy and allowance covering the
selected context, with every applicable negative still taking precedence. An account-positive
report covers a context only when its immutable bindings include that context and its account
link is still current; relinking cannot copy or freshen it. Context-direct positives retain their
original times and apply only to their declared context, not a newly labelled account.

## Captured routing authority and decisions

`RoutingPolicy` is revisioned saved configuration, separate from 001 autonomy policy. It includes
`economyOrder` (a permutation/subset of fixed profile IDs), `unknownAvailability` (`allow` or
`require_positive`), `defaultBudget`, `continuation` and `updatedAt`. The default allows unknown
native availability, with fresh known negatives and unresolved exhaustion taking precedence.
It does not assert that unknown access is authenticated, idle, free or available.

`RoutingOptions` is optional input: `suitability` (`unknown`, `routine`, `general`),
`targetProfileId`, `expectedPolicyRevision`, initial `budget` and `continuation`. Guidance can
explicitly request `useCurrentPolicy`; ordinary guidance preserves captured routing policy.
Supplying initial budget options in guidance is rejected: existing budget changes use an explicit
amendment. A complete supplied routing option object replaces option choices against the captured
policy; omitted guidance routing keeps them. No replacement silently resets the budget or deadline.

`TaskRoutingAuthority` is immutable per task version: `id`, `taskId`, `taskVersion`,
`capturedAt`, `policySnapshot`, `optionSnapshot`, `suitabilityAssessment` and `createdBy` (`user`
or explicit `legacy_adoption`). An assessment records `classification`, `source` (`user_declared`,
`recognized_operation`, `unknown`), `ruleId` and bounded evidence references. A whole-task
recognized-operation rule must satisfy the plan's conservative published rubric; a short string,
keyword or marketing name is not a rule. File existence/content assessment happens outside SQL
and carries source identity/hash. Invalidated assessment blocks; source/identity uncertainty is
not a category-A continuation proof.

Selection order is published and deterministic: known constraints/pins/payment; eligible known
capacity controls; fresh reported availability over unknown; applicable economical role; comparable
reference price; then captured stable configured order. Unknown task suitability uses only a
compatible conservative native-default profile unless an explicit profile/model choice applies.
It cannot silently classify unknown work as routine-small. Explicit model pins are literal native
selectors, not effective-model guarantees. A model-only pin with no unambiguous compatible context
blocks rather than inferring a harness from marketing spelling. An explicit compatible harness
permits an uncatalogued literal selector, with suitability/access/comparison still unknown.
An explicit profile and explicit harness/model must agree literally with the profile's fixed
context/selector; a conflict is `pin_unavailable` before admission/debit. A native-default profile
has an empty selector: supplying a model pin with it is a conflict, not an override. No alias
normalization or profile preference may replace contradictory explicit intent.

`RoutingDecision` is immutable: `id`, `taskId`, `taskVersion`, `attemptId`, `requestId`,
`authorityId`, `kind` (`manual`, `automatic`), nullable `previousAttemptId`, `createdAt`,
`candidateAssessments`, `selectedTarget`, `budgetEffect`, `evidenceSnapshot`, `uncertainties`.
Each of at most six candidate assessments records its profile/version snapshot, applicable
capability/report/link/price facts, eligibility and bounded reason codes. Shared `capacityEvidence`
contains at most two compact context snapshots: revision/asOf, declared account/link revision,
effective dimension states, positive coverage, report eligibility and deduplicated blocking/supporting
report IDs. Candidates refer to their context ID. Do not repeat full CapacityView bodies in every
candidate; immutable report IDs retain complete evidence without defeating the 64 KiB decision
bound. Validate the encoded decision bound before admission commits. An explicit uncatalogued
selector is one additional ephemeral assessment, never a new catalog/account/context.

`selectedTarget` is `{contextId, connectionId, harnessId, modelSelector, profileId,
declaredAccountAssociation}`; `profileId` is nullable for an explicit uncatalogued selector.
This is separate from original `pins`/`requestedTarget` and observed native identity. New attempt
reads expose optional `routingDecisionId`/`resolvedTarget`; historical rows remain absent. The
launch builder consumes this frozen resolution, checks it against literal pins and current controls,
and does not reinterpret Auto using the latest catalog. Never overwrite a task version's pins to
implement routing. An admitted target is immutable even while queued. If it becomes ineligible
before dispatch, block that attempt; a permitted replacement is a new decision/attempt/unit.
An uncommitted preview/proposal has no worker identity or budget charge and is labelled as such.

## Cumulative budget, windows and adoption

`TaskBudget` contains `taskId`, `revision`, `grantedAdmissions`, `consumedAdmissions`,
`remainingAdmissions`, `automaticAdmissions`, `historicalAdmissions`, `currentWindow`,
`latestAmendmentId`, `updatedAt`. `remainingAdmissions` is derived by checked subtraction;
none of these counters measures underlying model calls, tokens, billing or allowance.

Initial defaults grant two admissions. The first budgeted admission opens the common 1800-second
wall-clock window. Every accepted attempt consumes one unit atomically with its decision/receipt,
including later preparation blocks. No refund follows failure, cancellation, a price/report edit,
missing acknowledgement, unknown launch or owner restart. Existing exact replay consumes once.
All attempts use the same current window until explicit additional authority changes it.

`currentWindow` is `{number, windowSeconds, firstAdmissionAt, deadlineAt}`; both dates are null
before its first admission. Preserve `timeoutSeconds` as the existing per-attempt ceiling:

`Attempt.deadlineAt = min(common window deadlineAt, admittedAt + captured timeoutSeconds)`.

Both defaults are 1800 seconds but are different controls. Retry/guidance cannot extend the
common window or a prior attempt deadline. Expiry forbids dispatch and requests controlled Stop;
cleanup/evidence settlement may finish afterward under existing bounded/fenced rules. The timer
does not pause for cleanup and is not a hard real-time OS completion promise.

`BudgetAmendment` contains `id`, `taskId`, `requestId`, `action` (`grant`, `adopt`),
`additionalAdmissions`, optional `newWindowSeconds`, applicable task/budget revisions, prior/current
budget snapshots, authority/version references and `createdAt`. A normal grant advances task and
budget revisions without changing guidance version unless routing authority changes. Unit-only
grants preserve the current window. Opening a new window requires prior attempts settled with
verified cleanup, no fence/owner uncertainty and an explicit duration; it anchors at the next
budgeted admission and retains the earlier window/history. No amendment starts a worker.

Schema migration grants old tasks nothing. Old reads, Stop, acceptance and exact receipt replay
remain available; new execution is `routing_adoption_required` until explicit adoption. Adoption
requires verified settled ownership, appends a new task version preserving original/guidance/pins,
captures new routing authority and grants additional admissions plus a new window. Record the
known historical attempt count separately from measured usage. The initial adopted ledger shows
the legacy baseline count as both historical granted/consumed, then the explicit additional grant;
remaining authority is exactly the new grant. Do not rewrite old versions/attempts or infer live
usage. Adoption and a following explicit Start are two distinct retained request identities.

The append-only `BudgetLedgerEntry` records `id`, `sequence`, `taskId`, `budgetRevision`, `kind`,
`requestId`, nullable `attemptId`, grant/consumption deltas, window snapshot and `createdAt`.
Kinds are `legacy_baseline`, `grant`, `adoption`, `admission`, `window_open`, `window_expired`,
`launch_intent`, `launch_started`. Intent/known launch are distinct observations, not new admission
charges or proof of native request counts. Each admission and each launch fact has a unique
attempt binding; append/replay cannot debit twice. Overflow rejects the whole transaction.

## Continuation proof and admission

`ContinuationPolicy` contains `maxAutomaticAdmissions` (exact count 0–15), `categories` (unique
`pre_launch_ineligible`, `terminal_transient`), `allowReplacement`, `allowEscalation`. Default is
zero/empty/false/false. The automatic count is cumulative per task; grants do not silently reset
it. Changes require explicit captured routing authority. A positive limit requires a supported
nonempty category and cannot exceed permitted task admissions. Larger/unknown-comparison target
tiers require escalation authority as defined by the specification. Policy tiers are economy 0,
general 1 and unknown for native-default/uncatalogued selectors. A changed context/selector pair
always needs allowReplacement; it additionally needs allowEscalation if either tier is unknown,
the new tier is higher, or the new reference price is higher/dominated by the old price or is
missing/crossed/incomparable. A lower tier alone does not bypass an
unknown cost comparison. Retrying the same exact context+selector is not target replacement and
is exempt from this target-change test, but still needs category/limit/budget/control authority.

Only `pre_launch_ineligible` is supported by accepted execution profiles. Explicitly enabling
`terminal_transient` fails `capability_unavailable`; generic status, SDK-only facts, retry progress
or prose are not a discriminator. This is distinct from falsely qualifying a positive B fixture.

`ContinuationProof` is immutable per predecessor attempt: category, exact owner epoch, task/version,
terminal/cleanup references, observed eligibility-change or qualified terminal evidence, and
preparation/creation/input/output facts. A proof is a typed controller record, never decoded from
agent text or a public request. Category A requires affirmative durable no-Prepare/no-Create facts,
settled blocked/not-launched cleanup and intentionally absent worker output. Missing expected
evidence is not that exception. Category B remains unsupported until a separately reviewed native
profile can establish the complete required structured transient/root/input/output/cleanup proof.

Automatic admission is one transaction under the authoritative owner: exact current task/version,
latest settled predecessor, no accepted Stop, current open owner/gate, compatible pins/target,
current capacity/report/link/profile revisions, current captured continuation authority, unique
predecessor consumption, remaining units/automatic limit and unexpired window. It inserts the
new attempt, decision, debit and events together. No auto continuation crosses owner epochs;
restart interrupts retained work and never scans old failures into permission to replay.
The proof also records the task/budget revisions at its settlement-time eligibility evaluation.
A later explicit amendment/guidance/adoption cannot re-arm that older proof: it requires an
explicit next Start, not a background retry merely because new units appeared. A proof blocked
for lack of authority/budget or invalidated by a control mutation stays historical. Ordinary
observation refreshes do not themselves create a continuation request.

## Schema 4 records and constraints

All new tables use existing checked integer representations and valid encoded JSON. Existing
tables, `capacity_claims(resource_id='managed-native',attempt_id)` and its unique/FK constraints
remain unchanged. The new schema comprises:

| Table | Key/columns/index and purpose |
| --- | --- |
| `routing_policy` | Singleton PK checked `1`; positive `revision`; `data`. Current saved defaults; decisions contain immutable copies. |
| `routing_contexts` | `id` PK restricted to the two context IDs; unique `harness_id`; positive `revision`; boolean `enabled`; `data`. |
| `routing_profiles` | `id` PK restricted to the six profile IDs; `context_id` FK; positive `revision`; boolean `enabled`; `data`. Identity/selector/source are fixed. |
| `routing_accounts` | Positive immutable `sequence` PK; random `id` UNIQUE; positive `revision`; `data`. Stable account paging. |
| `routing_account_links` | `context_id` PK/FK; nullable `account_id` FK; positive `revision`; `data`. One current association per context. |
| `routing_capacity_state` | `scope_key` PK; nullable `context_id` UNIQUE FK and `account_id` UNIQUE FK with exactly-one CHECK; positive `revision`; `data`. Separate bounded current heads/sticky bindings, never full-history scans at admission. |
| `routing_capacity_reports` | Positive `sequence` PK; random `id` UNIQUE; `scope_key` FK; dimension CHECK; `data`. Index `(scope_key,sequence)` for stable filtered history. |
| `routing_report_bindings` | `(report_id,scope_key)` PK/FKs. Immutable report application scopes; relinking cannot remove them. |
| `task_routing_versions` | Random `id` UNIQUE; `(task_id,task_version)` PK/composite FK to `task_versions`; `data`. Immutable authority snapshots. |
| `task_budgets` | `task_id` PK/FK; positive `revision`/window number; checked nonnegative `granted`, `consumed`, `automatic`, `historical`; `consumed<=granted`, `automatic<=consumed`, `historical<=consumed`; nullable paired first-admission/deadline dates; `data`. |
| `routing_decisions` | Positive `sequence` PK; random `id` UNIQUE; task/version FK; `attempt_id` UNIQUE FK; nullable `previous_attempt_id` FK; `kind` CHECK; `data`; index `(task_id,sequence)`; unique partial index on `previous_attempt_id` where kind automatic. |
| `routing_continuation_proofs` | `attempt_id` PK/FK; `owner_instance_id`; category CHECK; `data`. Typed immutable proof with same-attempt evidence references. |
| `budget_amendments` | Random `id` PK; task FK; positive budget revision; `data`; unique `(task_id,budget_revision)` for one authority mutation. |
| `budget_ledger` | Positive `sequence` PK; random `id` UNIQUE; task FK; budget revision; kind CHECK; nullable attempt FK; `data`; index `(task_id,sequence)`; unique partial attempt indexes separately for admission/launch_intent/launch_started. |

All integer PK/revision/count columns check `typeof(...)='integer'` and their stated bounds.
These fourteen tables support current requirements rather than future scaffolding: policy/catalog
hold saved fixed declarations; account/link/report/current-state/binding records preserve alias
and negative provenance; authority/decision rows preserve legacy JSON and frozen targets; budget/
amendment/ledger rows separate explicit authority from cumulative effects; the proof row prevents
prose or reconstructed history from authorizing another worker. Usage is a bounded read over
existing attempt records, not another table. No paid executor, plugin registry, verifier or graph
state is introduced.
Scope keys must match their context/account column and kind; typed writes also validate all JSON
references, fixed catalog membership, source ownership, limits and revision correspondence.
There are no process/filesystem/network callbacks under a transaction. Installation/rubric facts
are gathered outside SQL, then passed as bounded typed evidence and rechecked at dispatch.

## Additive migration and fault behavior

Under the already acquired OS lock and private sidecar checks, validate the exact known schema-3
application ID, required tables/indexes/constraints, global claim DDL/cardinality/reference,
foreign-key consistency and absence of colliding/unknown routing objects before schema changes.
Unknown/future/incompatible schemas are errors, never reset instructions. The schema-3-to-4
transaction creates the new tables/indexes and seeds only the default routing policy, two native
contexts, six profiles, unlinked associations and empty context capacity views. It creates no task
budget, adoption, automatic decision or account declaration for legacy work. Set user_version 4
only after validating new constraints/defaults and foreign keys, in that transaction.

Do not update any existing row JSON, receipt/envelope/fingerprint, counter, gate, incident, held
claim or artifact reference. An unresolved claim remains held and fenced under ordinary recovery.
Fresh state initializes these defaults; supported schema-1/2 state follows its validated earlier
migrations before the additive step, preserving every old record. No unsupported schema is opened
or journal-mutated to obtain compatibility. WAL/FULL and per-connection pragmas remain unchanged.

Known failure before Commit rolls back the additive tables/defaults/version. Actual Commit or
after-Commit acknowledgement failure is uncertain: fail startup, preserve files and reopen for
schema validation without assuming no commit or attempting destructive repair. Reopening a fully
committed schema 4 recognizes it without seeding duplicate authority. New mutation receipts,
decisions and budget effects commit together; uncertainty fences conflicting dispatch and never
refunds or repeats a model worker to repair a missing acknowledgement.

Required design verification includes populated schema-3 byte preservation/held claims/incidents,
unknown-DDL refusal, atomic rollback/lost acknowledgement, large exact ledger arithmetic,
race-safe alias/report/budget mutation, durable A proof before any Prepare/Create, unsupported B,
legacy explicit adoption, frozen target/replay, mixed-harness global capacity and unchanged native
evidence/Stop/recovery behavior. This document supplies design obligations, not test results.
