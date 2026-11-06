# Feature Specification: Economical routing and account capacity

**Feature Branch**: refactor
**Feature Context**: 004-economical-routing
**Created**: 2026-11-06
**Status**: Requirements independently reviewed and ready for planning; no design/implementation acceptance

**Input**: Help a programmer use suitable smaller models and existing access across coding
harnesses, including giving background work to Claude while personal Codex is reported busy.
Keep ordinary work automatic, preserve the saved autonomous preference, minimize configuration,
respect explicit choices, bound total managed work and report uncertainty honestly.

## Scope and parent requirements

This slice owns provider/account/harness/model distinctions and economical selection under
FR-004 and FR-013–020. It preserves the short journey, intent and autonomy controls in FR-007–012,
complete evidence and zero-call observation in FR-029/031, version/control/ownership/isolation in
FR-034–038, and interface/review/qualification obligations in FR-045–048.

It extends accepted Codex and Claude authoring from
[003 acceptance](../003-claude-interoperability/acceptance.md), qualified at ce8aaa7 on six actual
native targets plus Linux AMD64 race checks. A routing decision is separate from an actual native
model observation, successful authoring, verified delivery and task acceptance.

Included: deterministic target selection, optional declared account links and sourced capacity
reports, finite cumulative task budgets, explicitly permitted bounded continuation, attributable
routing/usage history, and shared UI/CLI inspection and controls.

Excluded: new harness transports, direct paid API execution, login/account switching, credential
inspection, changing a vendor's global model or unrelated conversation, provider-enforced monetary
caps, model-based routing classifiers, quality leaderboards, parallel task graphs and the
review/verifier/integration workflow owned by feature 005. Account/paid-path eligibility is enforced
without pretending that an unsupported execution path exists. Native conversation resume remains
unsupported in this slice.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Start useful economical work with Auto (Priority: P1)

A programmer opens a project and describes an outcome. Auto uses the saved policy to select an
eligible target, preferring a smaller or less costly suitably capable choice when that preference
is supported. The programmer can inspect why it was selected without maintaining a routing plan.

**Why this priority**: Moving ordinary work to a suitable economical target is the central usage
benefit; routing itself must not consume an additional model call.

**Independent Test**: Use controlled eligible small/general targets, capability declarations,
prices and unknown-data cases. Start through the normal UI and CLI, inspect the captured decision,
and prove zero model calls for selection, waiting and observation.

**Acceptance Scenarios**:

1. **Given** an eligible small target and an eligible larger target that meet a bounded routine
   task's declared requirements, **When** Auto starts the task, **Then** the saved economy policy
   selects the suitable economical target and records its suitability and comparison basis.
2. **Given** that the small target cannot meet a required capability or a declared quality/context
   condition, **When** selection runs, **Then** it is excluded for that reason; cheapness cannot
   override suitability, and a permitted suitable alternative or a specific blocker is shown.
3. **Given** no additional account reports or price configuration, **When** the programmer supplies
   only project and task under the saved autonomous preference, **Then** ordinary Auto admission
   remains usable under the default unknown-availability policy without a setup wizard.
4. **Given** incomplete price or model-access evidence, **When** Auto selects using a documented
   economy preference or existing native configuration, **Then** it identifies that preference
   and the unknowns instead of claiming proven lowest cost, verified entitlement or savings.
5. **Given** the admitted request is repeated after its acknowledgement is lost and policy, prices
   or reports have changed, **When** the same request identity and input are retried, **Then** the
   original admission, route and budget effect are returned exactly once.
6. **Given** a short or vaguely worded task, **When** suitability is considered, **Then** description
   length or a model marketing name alone cannot establish that the work is routine or safe.
7. **Given** task suitability is unknown, **When** Auto selects, **Then** it uses a conservative
   general/native-default target only when all known required capabilities, quality and context
   constraints remain satisfied; otherwise it retains a specific suitability blocker. It does not
   infer that the task is routine, that a small target is suitable or that economy is proven.

---

### User Story 2 - Use reported Claude capacity without disturbing personal Codex (Priority: P1)

The programmer can optionally report that the current Codex context is busy and Claude capacity
is available. Auto can give suitable background work to Claude. Reports remain attributed and
temporary; an unobserved account is never called idle merely because Council has not used it.

**Why this priority**: This is the user's motivating cross-harness use case and must work without
copying credentials, logging in again or controlling the personal conversation.

**Independent Test**: Report busy Codex and available Claude in controlled local state, start an
ordinary Auto task, and inspect the selected context and evidence. Link aliases to one declared
account pool and test concurrent admission plus stale/negative observations.

**Acceptance Scenarios**:

1. **Given** fresh reported personal Codex occupancy and suitable reported available Claude
   capacity, **When** Auto starts background work, **Then** it chooses eligible Claude under the
   policy without changing the personal Codex model, conversation or authentication.
2. **Given** several model aliases explicitly linked to the same declared account capacity,
   **When** admission checks them, **Then** they share one occupancy/allowance source rather than
   multiplying available capacity by the number of aliases.
3. **Given** two declared accounts associated with the same harness, **When** they are inspected,
   **Then** their identities and reports remain distinct; this does not imply that the native
   connection can select either login.
4. **Given** a native context whose actual authenticated account is unknown, **When** the user
   assigns a display label or account link, **Then** it remains a user declaration, not observed
   authentication identity or new account-selection capability.
5. **Given** an expired positive report, **When** eligibility is reconsidered, **Then** it becomes
   unknown rather than fresh availability. A known exhausted meter is not replenished by a clock
   crossing a reset time, a price edit, relinking aliases or deleting an unrelated report.
6. **Given** an older positive report arrives after a newer applicable exhaustion/denial report,
   **When** it is stored or replayed, **Then** it cannot erase the newer negative evidence.
7. **Given** no external observation, a merely running process or Council inactivity, **When**
   status is shown, **Then** external occupancy and allowance remain unknown. Council's actual
   managed occupancy is shown separately and cannot be cleared by a user availability report.

---

### User Story 3 - Keep choices, retries and escalation inside one task budget (Priority: P1)

The programmer's explicit choices constrain Auto. The task has a finite managed-work budget;
additional work can occur only under captured authority and current controls. Failures retain
their evidence and never become permission to launch an unbounded replacement.

**Why this priority**: An economical first attempt is not useful if hidden retries, substitution
or escalation consume more work than the user authorized.

**Independent Test**: Exercise exact/partial pins, cumulative budget use, an explicitly enabled
bounded continuation and forbidden continuation. Use actual owned local workers and persistence,
Stop, deadline and cleanup fault cases across both harnesses.

**Acceptance Scenarios**:

1. **Given** an explicit harness or native model-selector pin, **When** eligibility or that choice
   fails, **Then** Council does not substitute another harness, selector, account or payment path.
   A partial pin constrains remaining Auto choices; an ambiguous or unsupported combination blocks
   with a specific explanation.
2. **Given** an exact actual-account requirement that the native connection cannot enforce,
   **When** admission is requested, **Then** it blocks without interpreting a declared label as a
   guarantee of which vendor login will run.
3. **Given** a finite task budget, **When** attempts, guidance, prices, target preferences or owner
   lifetimes change, **Then** already consumed or uncertain reserved work remains in that task's
   ledger. Exact replay consumes/reserves once; no change silently refills the budget.
4. **Given** an explicitly enabled continuation with one remaining managed attempt and a recognized
   eligible reason, **When** the prior attempt is terminal with verified cleanup and all current
   controls pass, **Then** at most one permitted next attempt is admitted with its reason,
   target, remaining budget and predecessor linked. The earlier failure is unchanged.
5. **Given** escalation is not authorized, a literal pin forbids the new target, or the applicable
   budget/deadline is exhausted, **When** a stronger or replacement attempt would be needed,
   **Then** no additional worker starts and one actionable blocker is retained.
6. **Given** accepted Stop, closing ownership, unknown cleanup, owner loss, incomplete/corrupt
   evidence or uncertain persistence, **When** continuation is considered, **Then** another
   harness cannot bypass the existing control/fence and no automatic replay occurs.
7. **Given** native internal retries, subagents or fallback, **When** Council reports its budget,
   **Then** managed attempt limits are distinguished from unknown underlying requests or charges.
   A strict financial/model-identity guarantee unsupported by the connection blocks rather than
   being represented as enforced.
8. **Given** required independent review, deterministic verification or integration has not been
   implemented or performed, **When** authoring returns useful output, **Then** routing does not
   promote it to verified delivery or task acceptance.

---

### User Story 4 - Inspect truthful routing and usage across changes and upgrades (Priority: P2)

The programmer sees what was requested, why a target was selected, what actually ran and what
usage is known. Later reports or prices do not rewrite historical decisions or hide failed work.

**Why this priority**: Economical operation must be inspectable without confusing reference
prices, native estimates, real charges and hypothetical savings.

**Independent Test**: Inspect the same task through UI and CLI after successful, failed and
cancelled attempts, changed reports and a populated previous-preview upgrade. Use clearly labelled
fake executions and authored UI observations; verify all historical bytes and facts remain truthful.

**Acceptance Scenarios**:

1. **Given** a task with several attempts, **When** usage is inspected, **Then** all applicable
   attempts, including failed/cancelled work, remain linked. Known subtotals identify their source,
   scope and missing portion; unknown values do not become zero.
2. **Given** top-level and overlapping whole-tree native model usage, **When** totals are shown,
   **Then** the same work is not counted twice, and native cost estimates, reference prices and
   any genuinely reported charges remain separately labelled.
3. **Given** a changed price/report/policy, **When** earlier task history is opened, **Then** its
   captured decision and original evidence remain available; current eligibility is a separate view.
4. **Given** a queued Auto plan becomes ineligible before dispatch, **When** controls are rechecked,
   **Then** an explicitly permitted reselection appends a new attributable decision or work blocks;
   the earlier decision and any actual attempt target are not silently rewritten.
5. **Given** a populated feature-003 task or receipt, **When** the application upgrades or the
   original request is replayed, **Then** its captured Auto/Codex meaning, pins, evidence and
   receipt remain unchanged. New economical authority is captured only for a new task or explicit
   adoption, not inferred retrospectively.
6. **Given** a feature-003 task that has not adopted the new budget/routing authority, **When**
   another execution is requested, **Then** inspection, Stop and exact old receipt replay remain
   available, but new execution requires one explicit adoption with its additional authority and
   retained prior use visible. Migration itself does not grant budget or change old Auto meaning.
7. **Given** delayed admission, routing/detail refresh or report-save responses, **When** the user
   edits text/pins/reports or selects another task, **Then** newer input and selection survive.
8. **Given** keyboard navigation at desktop and 360 CSS pixels, **When** the programmer uses Auto,
   optional choices/reports, explanations and Stop, **Then** focus and text state labels remain
   usable with the verified Ejupi Labs identity and icons.

### Edge Cases

- No harness is installed; only one is eligible; both have unknown availability; all are known busy.
- A small profile is documented but unsupported by the installed version, entitlement or provider.
- A native alias remaps or native fallback changes actual models; a selector is not an effective
  model guarantee and no Council-generated selector replaces an explicit one.
- Two aliases or harness contexts share one declared pool; linking/unlinking occurs during work.
- A new label or report cannot release an active managed claim, erase exhaustion or create allowance.
- A stale/future-dated/duplicate report, conflicting observation, unknown reset or expired positive
  report arrives while a task is queued.
- Temporary throttle, overload, model-access denial and actual allowance exhaustion have different
  meanings; a generic error or assistant prose does not establish an exact remaining balance.
- A request is repeated after budget reservation, route persistence, launch acknowledgement or
  terminal acknowledgement is lost.
- A task has no remaining managed attempts, an elapsed deadline or an unsupported strict spending
  guarantee; guidance and owner restart do not silently supply a new allowance.
- A selected target disappears, becomes busy/exhausted or loses a capability before native creation.
- Stop, guidance, budget amendment and capacity reports race with the final dispatch guard.
- A failed small attempt retains a useful candidate; another attempt does not imply that this
  unverified change was integrated into its workspace.
- Automatic continuation is disabled; it is enabled for one recognized reason; the reason is only
  an unauthoritative native text suggestion; cleanup or persistence is uncertain.
- Native usage is absent, partial, duplicated, scope-ambiguous, beyond safe floating precision or
  represented by an extreme valid estimate lexeme.
- All costs are unknown or incomparable; a preference tier must not be presented as measured savings.
- An old owner remains active during upgrade; only known exact-owner maintenance is permitted,
  without lock deletion, takeover, busy-store migration or reinterpreting old execution capability.

## Requirements *(mandatory)*

### Functional Requirements

- **RTE-001**: Council MUST keep provider, declared/observed account, native configuration context,
  harness, requested selector, selected target and observed effective model distinct. A label/link
  MUST NOT attest authentication or create executable account-selection capability.
- **RTE-002**: Ordinary new-task admission MUST require only project and original task, reuse the
  saved autonomy preference and capture useful finite routing defaults without a mandatory account,
  quota, price, benchmark or transport setup form.
- **RTE-003**: Auto MUST apply a published deterministic suitability/economy policy. Required
  capabilities, effective permissions, task constraints, pins and payment eligibility precede
  economic preference; routing, status and waiting MUST require zero model calls.
- **RTE-004**: When comparable suitable eligible small/economical and larger targets exist, Auto
  MUST prefer the economical choice and retain the comparison basis. Unknown suitability/access
  or incomparable prices MUST remain explicit; short prose or model names alone are not proof.
  Unknown task suitability MUST use a conservative general/native default only if all known
  capability/quality/context constraints hold, otherwise a named blocker; it MUST NOT be treated
  as routine-small suitability merely because availability is unknown.
- **RTE-005**: Existing supported native contexts with unknown entitlement/occupancy/allowance MUST
  remain usable under the documented default unknown-availability policy. Fresh applicable known
  negatives and unresolved applicable exhaustion take precedence; exhaustion remains denying even
  when stale until an attributable correction/recovery supersedes it. Unknown MUST NOT be labelled
  idle, available, free or verified.
- **RTE-006**: Optional explicit account links MUST share the associated occupancy/allowance pool
  across aliases without conflating different accounts. Unsupported exact account requirements
  MUST block, and relabelling/link changes MUST NOT refill allowance or release existing claims.
- **RTE-007**: Capacity records MUST separate managed occupancy, reported external occupancy,
  allowance, temporary denial/cooldown, exhaustion and unknown information, retaining source,
  observation/receipt time, applicable scope and validity.
- **RTE-008**: Positive report expiry MUST become unknown. Older positives, price edits, report
  changes unrelated to the denied source and reset-time passage MUST NOT invent replenishment or
  erase newer applicable exhaustion. Expired exhaustion remains denying, with its age visible,
  until explicit attributable correction/recovery; temporary cooldown expiry is a distinct state
  and does not establish replenishment. Recovery/withdrawal must be explicit and attributable.
- **RTE-009**: Fresh busy personal-Codex and suitable available-Claude reports MUST enable the
  motivating background route without altering the personal conversation, vendor-global settings
  or vendor-owned authentication. Reports and discovery MUST NOT inspect/copy credentials.
- **RTE-010**: Explicit/partial harness, model, account and payment choices MUST constrain Auto.
  Council MUST preserve literal native selector semantics; an unsupported stronger guarantee or
  unhonorable/ambiguous pin blocks without silent substitution or an invented account binding.
- **RTE-011**: A Council-managed paid path MUST never be selected/configured without a supported
  execution capability, configured provider/account and applicable captured budget authority.
  This slice supplies no direct paid API execution; unsupported paths block, and native configured
  billing/overage MUST be disclosed as outside Council's enforceable financial guarantee.
- **RTE-012**: Every new or explicitly adopted economical task MUST capture a finite cumulative
  managed-work budget and deadline. The default is two managed attempt admissions and one
  30-minute wall-clock window anchored at the first budgeted admission, including queue,
  preparation, execution and settlement time; all attempts share that window. Expiry forbids new
  dispatch and requests controlled Stop; the timer does not pause for cleanup. Bounded owned
  cleanup may finish later and remains visible/fenced as required, without creating more task
  authority or promising a hard real-time OS completion deadline.
  Managed attempts/reservations and known native launches MUST remain separately inspectable from
  underlying native model calls, tokens and charges that the connection cannot attest.
- **RTE-013**: Budget accounting MUST be durable and exact under concurrent admission and request
  replay. Accepted managed attempts consume the declared admission unit even when preparation
  later blocks; uncertain effects MUST NOT be refunded or replayed as though they never happened.
- **RTE-014**: Guidance, routing/report/price changes, target aliases, retries, escalation, manual
  replacement and owner restart MUST NOT silently erase cumulative use, reset the common task
  deadline or rewrite an earlier attempt deadline.
  Explicit additional authority creates a linked budget amendment; original history remains.
- **RTE-015**: Default Council-managed automatic continuation/escalation MUST be disabled. Explicitly enabled
  bounded continuation MUST name finite limits, permitted target changes and eligible recognized
  reasons from the closed categories below, with the required authoritative proof rather than
  merely a policy-supplied reason string. It MUST preserve original instructions and link the
  new attempt to its predecessor.
- **RTE-016**: Every next dispatch/reselection MUST recheck current guidance/version, ownership,
  Stop, global fence, capacity/account reports, target capability, payment eligibility, remaining
  budget and deadline. An admitted attempt's target is immutable. If it becomes ineligible before dispatch, the
  old attempt settles blocked; an authorized replacement requires a new linked decision/attempt
  and consumes another admission unit. With default continuation disabled, no such automatic
  replacement occurs. A clearly labelled pre-admission proposal is not a worker or budget charge.
- **RTE-017**: Stop, owner closing/loss, unverified cleanup, evidence corruption/incompleteness and
  uncertain persistence MUST prevent automatic replacement or alternate-harness bypass. Prior
  terminal failures/cancellations remain unchanged; no recovery replay is introduced. The only
  incomplete-output exception is intentionally absent worker output for a fully proven pre-launch
  block under category A below; missing/corrupt expected evidence is never that exception.
- **RTE-018**: A continuation MUST require prior settlement and verified owned cleanup. Native
  prose, peer suggestions, temporary retry progress and an advertised model name MUST NOT create
  escalation, payment, budget or task-acceptance authority.
- **RTE-019**: A route receipt MUST retain the request/task/version, applicable policy and evidence,
  included/excluded candidate reasons, selected native target, declared account association,
  payment meaning, budget effect and remaining uncertainty, separately from actual observations.
- **RTE-020**: An exact repeated request MUST return its original admission/decision/budget effect
  before applying current prerequisites; changed input with that identity MUST conflict rather than
  launch another worker or mutate the original decision.
- **RTE-021**: Usage views MUST include linked successful, failed, cancelled and interrupted work,
  distinguish known subtotals from full coverage, preserve exact counts/estimate representations,
  and avoid summing overlapping top-level/whole-tree reports or duplicate native messages.
- **RTE-022**: Native client estimates, dated reference prices, declared policy preferences,
  genuinely reported charges and unknown data MUST remain separate. Fake calls, model labels and
  attempt counts MUST NOT create measured token use, quota decrements, charges or savings.
- **RTE-023**: UI and CLI MUST expose the same routing, account/report, budget, waiting, evidence
  and control meanings. Historical/current views and delayed responses MUST preserve newer edits,
  selection and pending request identity without promoting a proposed route to a launched target.
- **RTE-024**: Previous-preview data, captured Auto/Codex semantics, counters, source/candidate/
  response references and exact receipts MUST survive upgrade unchanged. Old tasks adopt new
  economical authority only through an explicit new captured decision, not a storage rewrite.
  Unadopted legacy tasks remain inspectable/controllable and exact requests replay normally, but
  any new attempt requires explicit adoption first. Adoption names additional allowed work and a
  new window without erasing known historical admissions/use or granting authority by migration.
- **RTE-025**: Existing managed source isolation, staging/branch preservation, global native
  capacity and cleanup/evidence/failure precedence MUST apply equally to routed and pinned work.
  No candidate integration, independent check or task acceptance is inferred from routing success.
- **RTE-026**: The normal path MUST retain approved Ejupi Labs assets, readable English copy,
  keyboard access, text state labels and 360px usability. Exceptional controls must be optional,
  compact and understandable rather than requiring many per-task settings.
- **RTE-027**: Admission/observation/report inspection MUST remain bounded and make no model call.
  Source/account/report failures must yield specific recoverable limitations without inventing
  availability or silently changing another context.
- **RTE-028**: Development qualification MUST use labelled local fake targets, authored capacity/
  usage evidence and actual shared ownership/control paths, with zero live provider tasks or
  credential inspection. Spec/design/tasks, independent review, affected corrections, full developer
  and native/race qualification MUST complete before the next feature implementation.

### Recognized continuation categories and proof

These are the complete automatic continuation categories in this slice. A saved policy may
disable them or authorize a finite subset; naming another reason does not make it eligible.

| Category | Required proof | Permitted consequence under captured authority |
| --- | --- | --- |
| A — selected target became ineligible before native creation | A current applicable installation/capability or capacity observation identifies the changed eligibility; the attempt has durably settled blocked, no native process preparation/creation was attempted, no expected worker output is missing, and cleanup is verified as not launched. Storage, identity or ownership uncertainty is not such proof. | Select another eligible target only within pins, remaining task budget/deadline and the explicitly allowed replacement/escalation policy. Create a new linked attempt and charge its admission unit. Preserve the blocked attempt and original decision. |
| B — observed terminal transient service rejection | The qualified native interface provides an explicit supported temporary throttle/overload reason with a well-formed terminal failure, complete expected input/output evidence, known root exit and verified cleanup. Retry-progress messages, prose, generic status numbers and model-access/authentication failures do not establish this category; none establishes allowance exhaustion by itself. | Admit at most the authorized retry within its target restrictions and applicable cooldown, budget/deadline and current controls. Any larger/different target additionally needs the captured replacement/escalation authority. Preserve the earlier failure. |

This slice introduces no automatic quality-check correction trigger; observed verifiers and
review corrections belong to feature 005. A profile that cannot establish the proof above cannot
enable that category by guessing. Unexpected protocol truncation, evidence/commit ambiguity,
unknown cleanup, owner interruption and accepted Stop are never continuation triggers.

Escalation means moving to a stronger or more expensive declared policy tier, or changing to a
target whose equal-or-lower tier cannot be established. It requires explicit captured authority;
the tier comparison is a policy statement, not a measured intelligence score or billed-cost claim.

### Key Entities *(include if feature involves data)*

- **Native configuration context**: A supported invocation context for an installed harness,
  separate from proof of provider/login identity, model entitlement or external activity.
- **Declared account association**: An explicit user link between contexts/aliases and a capacity
  source. It is policy evidence; observed authentication identity remains separately unknown or sourced.
- **Capacity observation**: Managed ownership or a sourced external/allowance/denial report with
  scope, observation/receipt time, validity and precedence. It does not create an OS sandbox or quota.
- **Routing policy**: Captured suitability/economy preferences, unknown-availability treatment,
  supported payment conditions, finite managed-work bounds and explicit continuation authority.
- **Task budget ledger**: Cumulative admitted-work/reservation and known-launch records, shared
  by all task attempts/stages with linked explicit amendments and an attributable deadline.
- **Routing decision/receipt**: An immutable explanation of candidates, exclusions, evidence,
  selected context/selector and budget effect, distinct from execution and actual-model observations.
- **Usage record**: Attributable reported counts, scope, estimates, prices, actual charges when
  genuinely available, coverage gaps and exact references across the task's complete history.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Ordinary controlled new-task UI/CLI journeys start from project and task with saved
  autonomy/Auto and no mandatory new account, quota or transport configuration.
- **SC-002**: Every controlled suitability/economy comparison selects the eligible suitable
  economical candidate or records a specific blocker/unknown comparison; selection/status/wait
  introduce exactly zero model consultations. Unknown task suitability uses only a constraint-
  compatible conservative general/native default or a named blocker, never inferred routine fit.
- **SC-003**: The fresh busy-Codex/available-Claude case selects Claude without modifying the
  personal conversation; every shared-alias/concurrent-admission case uses one applicable capacity
  pool and retains unknown actual-account identity where it cannot be attested.
- **SC-004**: All controlled stale, expired, zero/exhausted, reset, temporary-denial and out-of-order
  reports preserve their distinct meanings, source/freshness and negative-evidence precedence.
- **SC-005**: Every pin/payment/unsupported-guarantee case honors the captured choice or blocks
  without unauthorized target/account/model/payment substitution.
- **SC-006**: Controlled budget exhaustion, exact replay and admitted-but-blocked work produce one
  exact ledger effect; policy/guidance/owner changes never silently refill it. An enabled bounded
  continuation launches no more than authorized; forbidden continuation launches zero workers.
  Default tasks receive exactly two admission units and one first-admission-anchored 30-minute
  wall-clock window, including queue/preparation, with no automatic replacement. A permitted
  queued-target replacement retains the old attempt/target and consumes a distinct new unit.
- **SC-007**: All Stop, owner-loss, deadline, uncertain-commit, evidence and unknown-cleanup cases
  retain prior authority/evidence and prevent alternate-harness replay or excess capacity.
- **SC-008**: Controlled usage with failed/cancelled attempts, missing counters, overlapping scopes
  and large/decimal estimates is shown without double counting, invented zeroes or claimed savings.
- **SC-009**: Populated previous-preview upgrade, lost acknowledgement and delayed UI/report
  responses preserve exact historical intent/receipts and newer input/selection; all core journeys
  remain keyboard-operable at desktop/360px with approved identity asset hashes. An unadopted old
  task admits no new work, while exact old replay/control remains available until explicit adoption.
- **SC-010**: Every RTE requirement maps to specified tasks and observed acceptance evidence;
  independent review closes blocking findings, final build/test/check and six native plus supported
  race jobs pass, and live qualification remains separate before the next feature.

## Assumptions

- Existing supported Codex/Claude native execution, one global managed-native worker and the
  accepted source/evidence/control lifecycle are prerequisites. Parallel graphs and additional
  harnesses remain later features; one managed worker can still route around reported personal
  external use of another harness.
- New tasks use the documented economical default; old captured tasks/receipts retain their prior
  meaning. New execution of an old task requires explicit budget/routing adoption with the
  additional authority visible; migration provides none. Selection is not vendor access proof.
- Existing native contexts with unknown external availability are eligible by default unless
  fresh applicable negatives, unresolved exhaustion or required capabilities block them. No report-maintenance ritual
  is required merely to start; the uncertainty is visible.
- The actual default for new economical work is two managed attempt admissions per task in one
  30-minute wall-clock window beginning at the first budgeted admission, including queue,
  preparation and settlement, with no automatic continuation/escalation. Explicit
  additional authority is possible without deleting cumulative history. An admitted-but-blocked
  attempt still consumes that managed-work unit; it is not represented as a billed model call.
- Optional occupancy/positive-availability reports use a visible 15-minute validity period by default. Exhaustion is not refilled by
  expiry/reset-time passage; a newer attributable recovery report or explicit correction is required.
- Native selectors and aliases have vendor-owned semantics and may map/fallback differently by
  provider/version/configuration. Strict effective-account/model or monetary guarantees that cannot
  be enforced by the interface block; ordinary native selection is not such a guarantee.
- Direct paid API execution and universal subscription telemetry are optional unsupported paths
  in this slice. Eligibility refusal is implemented; a hypothetical configured paid-policy example
  is not evidence that an API executor or actual billing receipt exists.
- Reference model/price capability information is dated declared evidence. It must not be confused
  with installed version, authenticated access, measured task quality or actual subscription usage.
- No extra user permission checkpoint is needed for this feature's development. Tests remain local
  and fake; a live provider task requires a separate specific user-requested task.
