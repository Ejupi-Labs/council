# Implementation Plan: Economical routing and account capacity

**Branch**: refactor | **Context**: 004-economical-routing | **Date**: 2026-11-06
**Spec**: [spec.md](spec.md)
**Status**: Reviewed design and ordered tasks; record the design commit before runtime changes.
See [design-review.md](design-review.md) for corrected findings, coverage and exclusive ownership.

**Input**: Independently reviewed requirements: 28 RTE requirements, ten success criteria and four
stories. [spec-review.md](spec-review.md) records the six corrected requirements ambiguities.
Feature 003 is accepted at ce8aaa7 after all seven actual native/race jobs passed in run 42153455521.

## Summary

Extend the existing durable authoring lifecycle with a small deterministic routing layer, declared
capacity sources and an exact cumulative task budget. Keep two supported native contexts and the
one global managed-native worker. Select among six dated reference profiles or an explicit native
selector without changing the original pins, native authentication or unrelated conversations.

Normal Start stays project/task plus saved autonomy and useful default routing policy. Unknown
availability is permitted by the documented default; unknown task suitability uses a conservative
native default rather than asserting that a small model is suitable. Optional reports let fresh
busy Codex and available Claude evidence affect selection. Declared account links share policy
capacity and never become proof of which vendor account actually ran.

Add transactional schema-4 state, immutable decisions and budget entries alongside existing
records. An admitted attempt's target cannot be changed in place. Category-A replacement is a
new linked, budgeted admission after a proven pre-launch block and current guards. Category B
is unsupported on the currently qualified exec profiles; do not simulate a positive capability.
Complete authoring still leaves candidates verification pending; feature 005 owns executable
review, verifiers, correction and integration.

## Technical Context

**Language/Version**: Go 1.27.3 runtime and existing strict authored TypeScript 7.0.2 browser UI.
The stack selected in feature 001 remains appropriate; this slice does not reopen that selection.

**Primary Dependencies**: Go standard library, modernc/sqlite 1.61.1 (embedded SQLite 3.54.1) and
existing golang.org/x/sys 0.49.0 Windows primitives. No new runtime dependency, runtime npm package,
model SDK, account broker or cloud service. Node 24.18.1/TypeScript/Playwright remain development tools.

**Storage**: The existing private v2 state root with SQLite WAL/FULL, foreign keys and native owner
lock. Schema 4 adds routing/configuration/report/decision/budget tables and validated migrations.
Historical JSON/receipt bytes, held claims, incidents and artifact files remain unchanged. Private
runtime state remains excluded from Git and packages.

**Testing**: Go unit/property/boundary tests, real owned local fake processes, independent CLI
clients, actual Playwright browser journeys, rendered desktop/360px inspection, exact asset hashes,
extracted-package checks, complete npm build/test/check and all six native/race jobs.

**Target Platform**: Windows/macOS/Linux AMD64 and ARM64. Existing Windows checked metadata/privacy
handles and Unix guardian ownership remain unchanged. Linux AMD64 supports the race detector gate.

**Project Type**: Local single-user native CLI/server with shared HTTP lifecycle and embedded UI.
MCP integration remains feature 008, additional harnesses features 009–011.

**Performance Goals**: Routing/status/report inspection make zero model calls. Candidate evaluation
is bounded by the fixed six-profile catalog/two contexts plus bounded applicable state. A one-file
optional assessment reads only authorized project metadata/content outside transactions. No full
transcript replay, polling model or unbounded price/account crawl.

**Constraints**: Shell-free subprocesses; no credential/config inspection or hidden login; no
vendor version/help/auth/administrative probes during development or routine discovery. No direct
paid API executor. Do not claim hard billing/internal-request/effective-model guarantees. Keep
exact counters and JSON cost lexemes without float64 conversion.

**Scale/Scope**: One owner, one global managed-native worker, two native contexts, six built-in
profiles, bounded declared-account/report/configuration records and paginated task history.
The contract defines finite list/input/snapshot/ledger bounds and exact revisions before code.

## Constitution Check

| Principle | Design response and gate |
| --- | --- |
| Useful programming outcomes | Reuses actual authoring/source/evidence/control behavior. Routing, native completion, review, verification and task acceptance remain distinct. |
| Proportionate usage | Deterministic selection and waiting, suitable economy preference, no classifier call, cumulative task admission/deadline bounds and explicit continuation authority. Unknown counters are not zero or savings. |
| Honest interoperability | Native contexts, declarations, actual identities and selector observations remain separate. Fixed supported native entrypoints, no copied credentials, no invented account switching or paid fallback. |
| Explicit autonomy | Saved full-access preference is reused. No extra normal Start confirmation; capability/payment/budget blockers are explicit. Native settings remain harness-specific. |
| Evidence/recovery | Atomic decision/budget/attempt/receipt, immutable admitted target and terminal history, no replay after uncertain ownership/persistence and no resource bypass through another harness. |
| Small branded experience | Main composer remains simple; routing explanation is progressive detail and reports/links are optional. Approved assets, keyboard focus, readable states and 360px remain required. |
| One specified feature | Requirements reviewed; this design and ordered tasks must pass independent analysis and be committed before application code. Prior features 001–003 remain accepted; feature 005 implementation waits. |

Pre-research gate: passes; no proposed exception. Re-evaluate after data model, contract and tasks
are final. Category-B absence and direct paid-API absence are explicit supported-scope limits,
not invented successful implementations.

## Project Structure

### Documentation

The active directory contains spec.md, its quality checklist/spec-review.md, this plan,
research.md, data-model.md, contracts/routing.md, quickstart.md and generated tasks.md.
Design-review.md will record independent consistency analysis before implementation.
Acceptance/review/evidence mapping follow observed implementation; no future-feature scaffold.

### Runtime source boundaries

- internal/domain: optional routing inputs/DTOs, finite policy/report/budget validation and exact
  count/decimal helpers. Preserve old field omission, structural pins and receipt fingerprints.
- internal/routing: pure bounded catalog/suitability/ranking/decision and usage-coverage logic.
  No process creation, credentials, model calls, SQL transaction ownership or network pricing.
- internal/store: schema 4 migration, configuration/report revisions, immutable routing versions and
  decisions, budget ledger/windows/amendments and atomic admission/continuation guards.
- internal/engine: resolve assessable inputs outside transactions, order all new mutations through
  the existing control/fence boundary, revalidate before launch and coordinate only authorized A.
- internal/adapters and discovery: reuse accepted native launch/parser contracts. Pass a separately
  frozen resolved target; do not rewrite stored user pins or claim category-B/native quota support.
- internal/app, httpapi and cli: one shared bounded contract; strict current owner handshake,
  separately whitelisted known-owner Stop, stable request identities and protocol-valid output.
- ui/src: optional work/profile controls, compact route/budget facts and optional Agents reports/
  account links. Frozen pending intents and current selection survive every awaited phase.
- internal/webui/dist and scripts/package*: regenerate/commit embedded assets and qualify the
  same extracted native behavior; no private project/account data in release inputs.

Exact files and independent ownership are enumerated in tasks.md. Keep configuration/ledger
types owned by one implementation lane before dependent engine/UI work begins.

## Phase 0 — Research and settled scope

Research compares preserving the current Go runtime versus introducing another process/service,
using native exec versus new SDK/app-server transports, deterministic policy versus classification
calls, and declared reports versus inferred external idleness. [research.md](research.md) records
decisions, alternatives, primary references and read dates.

Keep the accepted runtime/process/SQLite architecture. Native exec remains the execution path;
subscription access and model entitlement are not established by the API model catalog. Six
built-in reference profiles use current documented selectors, with dated assumptions and unknown
actual access. No retired Codex mini model is seeded from old examples.

Routine discovery remains metadata-only. User reports and qualified observations are separately
sourced; optional declared account links provide policy grouping without attesting login identity.
Native retry progress, generic HTTP numbers and a clock passing a reset do not prove exhaustion
or fresh allowance. No native SDK/app-server quota transport is added to obtain more telemetry.

## Phase 1 — Deterministic selection policy

### Fixed profiles and explicit choices

The catalog contains codex-native-default (selector omitted), codex-luna-economy (gpt-6-luna),
codex-sol-general (gpt-6.1-sol), claude-native-default (selector omitted),
claude-haiku-economy (native alias haiku) and claude-sonnet-general (native alias sonnet).
These are policy/reference identities. Native provider, entitlement, actual model and charges can
remain unknown. Minimum interface and dated alias/version assumptions are separate facts.

An explicit supported profile/harness/selector constrains selection. An uncatalogued literal
selector with an explicit compatible harness retains native semantics and unknown suitability/
access; it does not become a curated quality or price promise. An ambiguous model-only choice
requires a specific compatible-target correction. Exact actual-account, effective-model or hard
financial guarantees unsupported by the native interface block. Existing actual-account pins
are not reinterpreted as declared-account preferences.

### Conservative suitability assessment

Capture suitability as declared, narrowly derived or unknown, with the rule version and supporting
facts. Never classify from length, a single keyword, model marketing, fake success or a provider
response that says the task is easy.

The optional user-declared routine/general choice is a statement of intended work, not measured
quality. Known required capabilities, acceptance constraints, pins and stricter guarantees still
apply first. With no declaration, only a complete narrowly bounded operation can derive routine
suitability in this slice:

- The complete original instruction matches one documented literal replacement form: Replace
  "old" with "new" in path only; or In path only, replace "old" with "new". Optional terminal period
  and harmless keyword case/spacing do not admit extra instructions.
- Exactly one clean relative project path ends in .md, .txt or .rst; no traversal, glob, private
  state, directory, symlink or second instruction is admitted. It identifies one ordinary
  authorized file, bounded to 1 MiB for this assessor.
- Old/new are explicit nonempty quoted UTF-8 literals of at most 256 bytes each with no newline or
  control characters. The whole task is at most 2048 UTF-8 bytes.
- Additional guidance, free-form acceptance conditions, required review/check/delivery evidence or
  a contradictory known requirement disables this derived shortcut. Preserve full original text;
  assessment is a separate policy result, not a replacement prompt or safety certificate.
- Read source evidence outside SQL. Recheck the assessed identity/content against the managed
  capture before launch; changed/unknown assessment blocks instead of asserting stale routine
  suitability. Source/identity uncertainty is not an automatic category-A reason.

All other undeclared work is unknown suitability. Select only a constraint-compatible conservative
native-default target, or return a named blocker when known requirements cannot be satisfied.
Do not require a task taxonomy form or claim that the default is proven economical.

Declared general work may use documented general profiles. Declared/derived routine work prefers
eligible economy profiles; a general initial choice can be used when no economical candidate is
eligible and policy permits it, with that reason recorded. No intelligence leaderboard is created.

### Eligibility and ranking

Apply in this order, retaining bounded included/excluded reasons and evidence references:

1. Explicit choices, supported native interface, effective permission and known task capability/
   quality/context constraints, including unsupported strict identity/financial guarantees.
2. Payment eligibility and declared account/context association; never invent another login or
   a Council-managed paid executor.
3. Managed occupancy and applicable denying facts, including unresolved exhaustion even when
   stale. Temporary cooldown expiry is separate. Current ownership/cleanup/fences always dominate
   a user availability report.
4. Unknown availability policy; under default allow it remains explicitly unknown. Prefer fresh
   reported availability over unknown information, without treating it as verified login/quota.
5. Suitable declared economy tier; comparable valid reference prices only when currency/unit/basis
   are actually comparable. Missing/incomparable price is not zero or proven cheapest.
6. The captured saved profile order as a stable deterministic tie-breaker. Council inactivity is
   not external idleness, and aliases do not create extra capacity.

The stricter require_positive setting requires fresh available facts for both occupancy and
allowance covering the chosen context. Applicable negatives always win. An account-origin positive
requires its immutable context binding and the same current link; relinking cannot renew it.
The default allow setting does not require these optional reports.

Reference comparison uses the exact bounded Pareto rule in data-model.md: both rates must be
known and comparable; crossed or missing rates and out-of-domain decimals are incomparable.
Within an otherwise tied candidate group, find the nondominated price front, then apply captured
stable order to that front. Do not build a non-transitive pairwise sort by mixing Pareto dominance
with order for incomparable pairs. Do not guess a task token mix. Continuation tiers are economy 0, general 1 and native-default or
uncatalogued unknown. A changed target with a higher or unknown tier, or a higher/incomparable
reference cost, requires explicit escalation authority as well as any replacement authority.

Capture every chosen reason/reference before admission. Prices or reports changed later do not
rewrite old decisions. A pre-admission preview is labelled a proposal, never a launch or reservation.

## Phase 1 — Persistence, budgets and dispatch

### Schema 4 and immutable lineage

The data model defines additive routing/policy/account/report/decision/budget tables, exact
revisions and validated known-schema upgrades. Keep the schema3 managed-native claim unchanged.
Preserve all existing JSON/receipt/artifact bytes, task versions, claims and incidents. Migration
does not recover work, prove cleanup, grant a budget or reinterpret a legacy Auto task.

New tasks atomically capture routing authority and the first decision, spend one managed admission
unit, create the immutable attempt/resolved target and store the original receipt. Read-only
enrichment never changes an old mutation receipt. Unsupported/unsafe numeric inputs fail before
commit. No filesystem/vendor I/O occurs inside a transaction.

Legacy tasks retain inspection/control/exact replay, but cannot admit new execution until explicit
adoption. Adoption appends a new task/routing version and grants explicitly stated additional
authority; known historical use remains visible. Budget grant/adoption is not implicit execution.
A UI "Adopt and start" action may compose two frozen mutation identities, like the existing
save-draft/Start flow, and must reconcile each acknowledgement before the dependent mutation.

### Shared time and admission accounting

The default is two managed attempt admissions and one30-minute task wall-clock window anchored
at its first budgeted admission. Queue/preparation/execution/settlement do not pause the clock.
Existing timeoutSeconds remains a per-attempt ceiling: each attempt deadline is the earlier of
the task-window deadline and its own admission time plus the captured attempt timeout. Changing
guidance or retrying never changes the common task deadline or an old attempt's deadline.

Accepted admissions spend their declared unit even if later preparation blocks. It is not a
measured billable model-call count. Exact request replay spends once; an uncertain commit/reservation
is retained and never refunded or used as replay permission. All attempts and future workflow
stages use the same task ledger, including failures and cancellations.

Explicit additional budget authority appends an amendment, never deletes prior use. A new window
can be granted only after prior work is settled with verified cleanup, begins at the first next
budgeted admission and preserves old windows/history. Expiry forbids new dispatch and requests
controlled Stop; bounded owned cleanup may finish later and retains fences when necessary.
No OS hard-real-time or provider spending guarantee is implied.

### Category A and forbidden replacement

Before any native preparation/creation, recheck current task version, Stop, ownership, global
fences, budget/deadline, target capability/installation and applicable account/report controls.
Never change an admitted target in place. If it became ineligible, durably settle the original
attempt blocked and retain its decision.

Only explicitly enabled finite category-A policy may create a new linked decision/attempt after
the specified affirmative no-preparation/no-creation/not-launched proof. Charge another admission
unit and recheck all guards. Default automatic continuation/escalation is OFF. No source/evidence/
identity uncertainty, Stop, owner loss, generic native error or incomplete expected evidence is A.

Category B remains unsupported on both current exec profiles; requesting it explicitly yields the
documented capability blocker. Do not invent terminal transient authority from SDK/app-server
types, retry-progress messages, raw prose or generic429/529. No automatic quality-check correction
is introduced before feature 005.

### Engine and adapter boundary

All new mutable policy/account/link/report/budget operations used by an active owner pass through
the engine's control ordering and mutation-uncertainty fence before dispatch can observe them.
Avoid introducing bare app/store writes that can commit uncertain new eligibility while scheduling
continues. Read-only views remain detached bounded observations.

Keep resolved harness/model/profile/association on the admitted decision/attempt separately from
original literal pins. Supply the frozen resolved native target to the builder without modifying
stored TaskVersion.Pins, original instructions, historical fingerprints or receipts. Re-resolve the
actual executable and preserve the accepted source/input/root/drain/cleanup/evidence success gates.

## Phase 1 — Usage and interface contracts

The contract defines shared bounded routing policy/context/profile/account/link/report operations,
task decision/budget reads, explicit amendments/adoption, strict machine-readable results and
current-owner verification. App version 2.0.0-dev.4 / API 1 / execution 004-v1 / schema 4 identify new execution.
Keep separate known-owner maintenance Stop for accepted002/003 plus the current row; API1 alone
does not authorize unknown execution compatibility or takeover.

Usage combines only nonoverlapping known scopes, always retains failed/cancelled/interrupted
attempts and identifies missing coverage. Keep native client estimates, dated reference prices,
genuine actual charges if available and unknown amounts separate. Underlying native requests/
subagents/overages cannot be derived from Council attempt counts or vendor model names.

Use exact integer/decimal representations. Historical valid extreme estimate lexemes remain
retrievable; if bounded arithmetic cannot aggregate a value, label partial/unaggregatable coverage
instead of converting to float, zero, clipping the original or failing the task. Never add the
whole-tree model preview on top of overlapping top-level usage, nor treat an omitted preview entry
as zero. Full raw evidence remains authoritative and retrievable.

UI keeps ordinary Start simple. Optional details show suitability/profile/pins and finite task
budget; Agents offers optional capacity reports/account declarations. Task detail shows requested,
decided and actual observations distinctly, with decision history, budget/current window, source/
age/expiry and one actionable blocker. Do not expose an executor/configuration-file wizard.
Current report edits, pending Start/adoption identities and selected tasks survive every async phase.

## Validation and delivery sequence

1. Freeze domain/contract/catalog/assessment/ranking and budget units before dependent edits.
2. Write populated migration/replay/concurrency/negative-report/budget/continuation cases before
   enabling dispatch; retain deterministic Windows privacy regression from accepted003.
3. Implement pure selection and store transactions, then shared engine/native launch integration.
4. Implement UI/CLI on the frozen contract, with independent-client and actual browser journeys.
5. Exercise negative/expired/zero reports, aliases/shared pool, literal pins, unsupported paths,
   exact ledger/replay, category-A replacement and unsupported-B cases with local fake targets.
6. Verify full source/index/branch preservation, actual owned controls/cleanup, budget expiry,
   mutation/commit uncertainty, no replay and populated legacy adoption.
7. Inspect desktop/360px, keyboard/reduced-motion and approved asset hashes; check literal counts/
   estimate strings, unknowns and delayed phase/selection responses.
8. Run final npm build/test/check, extracted package and all six native plus race jobs against the
   reviewed implementation head. Record no live provider qualification or measured savings.
9. Close independent source/failure-path review, corrections, requirement mapping and parent status.
   Commit source and matching embedded assets; only then proceed to feature 005.

## Design completion gate

Research, data model, contract and quickstart must use the same field names, catalog, unit/time/
legacy rules and supported triggers. Generate tasks using the actual Specify task workflow and
verify coverage of all 28 RTE requirements, ten success criteria and four stories. Independent
Specify analysis must close material inconsistencies; commit all design/task artifacts before
changing application version, schema or runtime. No unresolved design placeholder is permission
to start implementation.

## Complexity Tracking

No constitution violation or new dependency exception is proposed. Conditional unsupported paid
execution, absent native telemetry and unsupported category B are explicit capability boundaries.
Any material change to the finite scope requires spec/contract/task reconciliation before code.
