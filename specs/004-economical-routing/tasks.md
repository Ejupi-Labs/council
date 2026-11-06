# Tasks: Economical routing and account capacity

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[data-model.md](data-model.md), [contracts/routing.md](contracts/routing.md) and
[quickstart.md](quickstart.md).
**Branch**: `refactor`; feature context is separate from Git branching.
**Status**: ACCEPTED at source3f4a18e; all local checks, independent reviews and six native plus Linux race jobs passed. See acceptance.md and evidence-matrix.md.

Tests are explicitly required by all four stories and RTE-028. Write meaningful failing cases
before enabling the corresponding behavior; retain the existing accepted 001–003 regressions.
All provider executions use labelled local fixtures. Do not invoke vendor executables, administrative,
help/version/auth commands or live models, inspect credentials, add an SDK/app-server/API transport,
or change native global settings. Only the documented two native contexts and six profiles exist.

## Format and paths

Every task uses `- [ ] Tnnn [P?] [USn?] Description with exact paths`. `[P]` means different files
can be authored concurrently **after its stated prerequisites**; it does not authorize concurrent
heavy test/build processes or overlapping file ownership. Setup/foundation/final tasks have no
story label. Paths are repository-relative; named new files live in the plan's existing source
boundaries. Preserve unrelated source and existing private state.

## Phase 1: Setup and executable acceptance fixtures

**Purpose**: Close the design gate and prepare reusable offline evidence without new product scope.

- [x] T001 Close Specify analysis/design findings, align the quickstart with the frozen routes, numeric/tier rules and category-B refusal, record explicit implementation ownership in `specs/004-economical-routing/design-review.md`, and confirm the reviewed design/tasks are committed before any runtime edit; use `specs/004-economical-routing/quickstart.md` and `specs/004-economical-routing/plan.md` as the execution gate.
- [x] T002 [P] Add reusable populated schema-3 and exact-row/receipt/held-claim/incident fixtures in `internal/store/routing_fixture_test.go`, including large counters and original Unicode/BOM/CRLF evidence, using the accepted 003 schema rather than fabricated new defaults.
- [x] T003 [P] Extend disposable-home/project and real independent-client/browser helpers in `internal/cli/routing_test_helpers_test.go` and `ui/tests/helpers.mjs` with labelled routing reports, deterministic time/control barriers and authenticated cleanup registered before assertions; reuse the real binary/local fixture path and never substitute installed vendors.

**Checkpoint**: The gate is recorded, fixtures have bounded cleanup, and test data cannot be
mistaken for observed provider access, quota, usage or charges.

## Phase 2: Foundational types, durable state and compatibility

**Purpose**: Freeze shared APIs and implement the durable primitives required by every story.
Do not activate dispatch with incomplete routing/budget controls.

- [x] T004 Freeze the contract DTOs and typed admission/dispatch facts in `internal/domain/routing.go` and optional additions to `internal/domain/execution.go`, separating pins, resolved target, declared account, actual observations, policy/assessment/decision, report heads, budget/window and continuation proof; preserve omitted legacy fields and publish the API before dependent lanes edit.
- [x] T005 Implement strict bounded routing/config/report/amendment validation and exact decimal/count helpers in `internal/domain/routing_validation.go` with boundary cases in `internal/domain/routing_validation_test.go`: fixed IDs, unknown/null/duplicate inputs, explicit false, durations, checked int64 arithmetic, reference comparability bounds and unsupported category-B/account/payment guarantees.
- [x] T006 [P] Implement the fixed two-context/six-profile reference catalog and immutable selector/source/version/role/tier declarations in `internal/routing/catalog.go`, including unknown Codex minima, separate Claude alias/transport assumptions and native-default uncertainty; no discovery/model command or network price lookup is allowed.
- [x] T007 [P] Write additive schema-4 migration/fault tests in `internal/store/routing_migration_test.go` proving exact old row/JSON/receipt/counter/artifact-reference preservation, held-claim/incident retention, no legacy authority seed, unknown/colliding DDL refusal, rollback and lost-acknowledgement reopen behavior.
- [x] T008 Implement validated additive routing tables/indexes/default seeds and schema-1/2/3 upgrade handling in `internal/store/routing_schema.go`, `internal/store/routing_migration.go`, `internal/store/schema.go` and `internal/store/store.go`; retain the existing global claim, native lock/sidecar preflight and WAL/FULL behavior without resetting data or migrating behind a live owner.
- [x] T009 Implement typed bounded current-state/authority/decision and exact budget/window/append-only ledger primitives in `internal/store/routing_state.go` and `internal/store/routing_budget.go`, including checked subtraction, unique admission/launch bindings, detached snapshots and no filesystem/process/network callback under SQL.
- [x] T010 Implement atomic authority/decision/debit/attempt/receipt insertion helpers and stable input fingerprints in `internal/store/routing_admission.go` and `internal/store/fingerprints.go`; consume typed already-assessed facts, freeze resolved targets separately from pins, and make exact replay precede current prerequisites without enriching old envelopes.
- [x] T011 Update current execution/version/capability gating consistently in `internal/app/types.go`, `internal/domain/execution.go`, `internal/engine/observe.go`, `internal/cli/metadata.go`, `ui/src/api.ts` and `package.json` for dev.4/004-v1/schema4, retaining exact dev.2/dev.3 maintenance-only Stop rows and original owner/request binding; API1 alone never permits old/new execution interchange.

**Checkpoint**: Shared types and durable primitives are usable; migration preserves the accepted
runtime, and clients distinguish current execution from known older-owner maintenance.

## Phase 3: User Story 1 — Start useful economical work with Auto (P1, MVP)

**Goal**: Project/task Start deterministically selects an eligible suitable profile, captures its
reason and finite budget, and exposes the same exact intent through UI and CLI without a wizard.

**Independent test**: With local fake contexts and no optional reports/prices, minimal Start uses
the conservative native-default path for unknown suitability. Explicit routine/general fixtures,
known constraints, price/version negatives and literal pins produce the frozen expected decision.
Selection, waiting and observation invoke no model/vendor process; a lost reply replays one result.

### Tests before enabling selection

- [x] T012 [P] [US1] Write deterministic catalog/eligibility/ranking tests in `internal/routing/select_test.go` for routine/general/unknown cases, current and stale facts, profile/partial/uncatalogued pins, model-only ambiguity, known-old versions, disabled contexts/profiles, unsupported guarantees, exact comparable/crossed/incomplete prices and stable tie order.
- [x] T013 [P] [US1] Write complete recognized-operation and source-assessment tests in `internal/routing/assess_test.go` for the plan's whole-task replacement grammar, Unicode literal/path limits, extra instructions/guidance/acceptance, traversal/private/symlink/nonordinary/oversized files and changed identity/content; short text and isolated keywords must not establish routine suitability.
- [x] T014 [P] [US1] Write actual owned-fixture admission/argv/source tests in `internal/engine/routing_admission_test.go` and `internal/adapters/routing_test.go` proving the chosen connection/selector is frozen independently of original pins, full original/guidance text is preserved, real provider facts remain unknown in fake mode, and literal/unsupported pins start no substituted worker.
- [x] T015 [P] [US1] Write actual browser minimal-Auto/optional-profile/pin and lost-acknowledgement cases in `ui/tests/routing-start.test.mjs`, including newer composer edits and retained-task selection during delayed admission and list refresh; assert no mandatory account/quota/price/classification form.

### Implementation and integration

- [x] T016 [US1] Implement the conservative published grammar/assessment and bounded authorized file-evidence collection in `internal/routing/assess.go`; retain full task text, collect identity/hash outside transactions, classify all unmatched undeclared work unknown and never use the prompt byte bound as model-context proof.
- [x] T017 [US1] Implement pure bounded eligibility/ranking and reference-price comparison in `internal/routing/select.go` and `internal/routing/prices.go`, following the frozen role/tier/price domain and partial-order rules, fresh-positive versus unknown preference and stable captured tie order; retain excluded reasons and uncertainty without float conversion or a model call.
- [x] T018 [US1] Implement revisioned saved policy/context/profile reads and updates in `internal/store/routing_config.go`, including immutable catalog selectors/source, optional exact reference declarations, captured policy snapshots and same-commit receipts; configuration edits start no work or refill a task budget.
- [x] T019 [US1] Integrate outside-SQL assessment, typed selected-target admission and native launch consumption in `internal/engine/routing.go`, `internal/engine/engine.go`, `internal/engine/results.go`, `internal/store/tasks.go`, `internal/store/execution.go`, `internal/adapters/native.go` and `internal/adapters/codex.go`; preserve original pins/fingerprints, resolve the actual executable and retain all accepted input/root/drain/source/cleanup/evidence success gates.
- [x] T020 [US1] Expose shared policy/context/profile and optional task/guidance routing contracts in `internal/app/routing.go`, `internal/app/execution.go` and `internal/httpapi/routing.go`, with strict request/query/error/receipt handling, bounded detached reads and the existing authentication/Origin/owner protections.
- [x] T021 [US1] Implement the exact policy/context/profile and start/guidance CLI mappings in `internal/cli/routing.go`, `internal/cli/execution.go` and `internal/cli/parse.go`, retaining request identity, explicit false, literal harness/model pins, JSON stdout/diagnostic separation and refusal of unknown/duplicate/unsafe flags; cover them in `internal/cli/routing_test.go`.
- [x] T022 [US1] Mirror the frozen DTOs and add optional suitability/profile/budget details to ordinary Start in `ui/src/routing-types.ts`, `ui/src/tasks.ts`, `ui/src/index.html` and `ui/src/style.css`; capture options once in pending intent and keep project/task plus saved autonomy sufficient.
- [x] T023 [US1] Add compact current policy/profile explanations and admitted route/budget status in `ui/src/routing.ts` and `ui/src/workspace.ts`, distinguishing proposed/admitted/launched/observed targets, native defaults and unknown access without exposing transport configuration or claimed savings.
- [x] T024 [US1] Run the focused pure/domain/adapter/admission/CLI and actual browser story cases from `internal/routing/select_test.go`, `internal/engine/routing_admission_test.go`, `internal/cli/routing_test.go` and `ui/tests/routing-start.test.mjs`, and record checked revision, exact outcomes and fake/zero-call scope in `specs/004-economical-routing/acceptance.md`.

**Checkpoint**: US1 is a usable vertical slice with default finite authority. It is the MVP
demonstration, not permission to accept004 before the remaining stories and release gates.

## Phase 4: User Story 2 — Use reported Claude capacity without disturbing Codex (P1)

**Goal**: Optional declarations/reports affect eligible routing while preserving account/source
distinctions, shared bindings, sticky exhaustion and actual owner capacity.

**Independent test**: Fresh busy Codex/available Claude reports select a suitable Claude fake.
Two aliases share declared controls, two declared accounts remain distinct, stale positives cannot
erase newer exhaustion, and simultaneous independent clients still converge on one actual owner.

### Tests before enabling mutable capacity

- [x] T025 [P] [US2] Write report/link/state boundary and transaction tests in `internal/store/routing_capacity_test.go` for all dimensions, future/stale observations, explicit recovery/withdrawal, sticky expired exhaustion, cooldown expiry, immutable inherited bindings after relink, 64-record/reference bounds and pre-/post-commit faults without a capacity reset.
- [x] T026 [P] [US2] Write a real independent-CLI-process barrier test in `internal/cli/routing_capacity_integration_test.go` for busy-Codex/available-Claude routing, shared declared pools and concurrent admissions, proving one owner/global worker, immutable queued targets and bounded authenticated fixture cleanup without vendor invocation.
- [x] T027 [P] [US2] Write actual browser optional account/link/report/correction cases in `ui/tests/routing-capacity.test.mjs`, including visible source/age/expiry, unknown actual login, stale revision errors and delayed saves while newer report input or task selection changes.

### Implementation and integration

- [x] T028 [US2] Implement bounded declared-account creation/update, one current link per native context and immutable link snapshots in `internal/store/routing_accounts.go`, retaining expected revisions and receipt-first replay; reject executable/login/credential fields and preserve every existing capacity binding.
- [x] T029 [US2] Implement immutable reports, correction references, owner receipt sequences and bounded sticky current heads in `internal/store/routing_capacity.go`, carrying linked-context/account negative bindings atomically and rejecting stale corrections or unrelated positive attempts to erase exhaustion.
- [x] T030 [US2] Implement pure read/dispatch-time effective capacity evaluation in `internal/routing/capacity.go`, separating actual managed occupancy from reports, default 900-second positive validity, expired unknowns, cooldowns and unresolved exhaustion; no synthetic renewal write, reset-time refill or native process inspection is allowed.
- [x] T031 [US2] Order every active-owner policy/account/link/report/profile mutation through `internal/engine/routing.go` and `internal/engine/control.go`, fencing uncertain effects and rechecking current bounded capacity/profile/link heads before native preparation; report availability cannot release the global native claim or retroactively rewrite running work.
- [x] T032 [US2] Expose exact account/link/capacity/report/history routes and strict filter-bound pagination in `internal/app/routing.go` and `internal/httpapi/routing.go`, with conflict/fault/security cases in `internal/httpapi/routing_test.go`; public reports always remain `user_report` regardless of caller labels/URLs.
- [x] T033 [US2] Implement account/link/capacity/report CLI reads and mutations in `internal/cli/routing.go` and `internal/cli/parse.go`, including exactly-one subject filter, corrections, complete flag spelling, booleans and retained identities, with contract checks in `internal/cli/routing_test.go`.
- [x] T034 [US2] Add optional Agents declarations, links, source-labelled reports and recovery actions in `ui/src/agents.ts`, `ui/src/routing.ts` and `ui/src/index.html`, preserving edits/pending request bodies across conflicts/reload and showing managed occupancy separately from declared availability and unknown login.
- [x] T035 [US2] Run the focused report/link/store, real independent-client and actual browser cases in `internal/store/routing_capacity_test.go`, `internal/cli/routing_capacity_integration_test.go` and `ui/tests/routing-capacity.test.mjs`; record one-owner/source-preservation and no-native-chat/config/credential effects in `specs/004-economical-routing/acceptance.md`.

**Checkpoint**: The motivating cross-harness route is observed with local fixtures and sourced
reports. Declared account grouping still provides no account-selection or live quota guarantee.

## Phase 5: User Story 3 — Keep retries and escalation inside one budget (P1)

**Goal**: Every additional managed attempt requires exact finite authority and current controls;
legacy adoption, grants and category-A replacement preserve prior use and failure lineage.

**Independent test**: Consume two default units in one first-admission-anchored window; block the
third and preserve exact replay. A proven pre-Prepare eligibility change can produce at most one
authorized linked successor. Default OFF, unsupported B, Stop, uncertainty, restart and old proofs
produce none. An old003 task requires explicit adoption before a new attempt.

### Tests before enabling continuation or amendments

- [x] T036 [P] [US3] Write exact budget/window/amendment/replay tests in `internal/store/routing_budget_test.go` for admitted-but-blocked units, concurrent last-unit admission, per-attempt versus common deadline, queue/preparation/settlement time, checked overflow/large counters, unit-only grants, new-window prerequisites and commit/acknowledgement faults without refunds.
- [x] T037 [P] [US3] Write real-owned-fixture guard/continuation fault tests in `internal/engine/routing_continuation_test.go` for affirmative no-Prepare/no-Create A proof, immutable old target/new unit, default OFF, literal pins/tier permissions, unsupported B, misleading prose/retry/HTTP/auth/model errors, Stop/guidance/report races, missing expected evidence, ownership loss and retained partial candidates.
- [x] T038 [P] [US3] Write populated legacy-adoption and no-rearm tests in `internal/store/routing_adoption_test.go`, preserving old versions/pins/receipts and historical use, rejecting ordinary-guidance adoption, retaining exact replay, and proving grants/guidance/adoption/restart cannot reactivate an earlier blocked/expired or consumed continuation proof.
- [x] T039 [P] [US3] Write actual CLI/browser grant/adoption/lost-acknowledgement and current-selection cases in `internal/cli/routing_integration_test.go` and `ui/tests/routing-budget.test.mjs`, proving amendment/adoption starts no worker and composed Adopt-and-start uses two separately frozen request identities.

### Implementation and integration

- [x] T040 [US3] Implement explicit grants, shared-window advancement and legacy adoption transactions in `internal/store/routing_budget.go` and `internal/store/routing_adoption.go`, with exact revision checks, append-only baseline/grant/window records, verified prior settlement, retained historical use and no implicit execution or migration authority.
- [x] T041 [US3] Integrate captured routing policy/options into immutable guidance/task versions in `internal/store/tasks.go` and `internal/store/routing_admission.go`, preserving policy unless `useCurrentPolicy` explicitly adopts it, enforcing the correct saved-policy revision and rejecting guidance budget fields without changing cumulative use/deadlines.
- [x] T042 [US3] Implement immutable owner-epoch/category-A proofs and unique predecessor consumption in `internal/store/routing_continuation.go`, recording settlement-time task/budget revisions and affirmative no-preparation/creation facts; automatic decision/debit/attempt/receipt commit together and stale, consumed or control-invalidated proofs cannot re-arm.
- [x] T043 [US3] Integrate all final dispatch guards and assessed-source revalidation in `internal/engine/routing.go` and `internal/engine/results.go` before any native Prepare/Create and again under the accepted launch ordering; distinguish known ineligibility from source/storage/identity uncertainty and retain complete expected evidence or an explicit fence.
- [x] T044 [US3] Implement finite opt-in category-A scheduling in `internal/engine/routing_continuation.go`, requiring prior durable blocked/not-launched settlement, current owner epoch/version/Stop/fence/capacity/budget/deadline and replacement/escalation permission; preserve old outcomes and never derive category B or feature005 correction authority from native output.
- [x] T045 [US3] Expose budget/ledger/amendment/adoption through `internal/app/routing.go` and `internal/httpapi/routing.go`, returning the frozen snapshots/errors/receipts, enforcing absent versus required budget revisions and keeping exact old replay available before adoption checks.
- [x] T046 [US3] Implement exact budget, adoption and continuation CLI commands/flags in `internal/cli/routing.go`, `internal/cli/execution.go` and `internal/cli/parse.go`, including canonical zero automatic counts, category uniqueness, explicit false, guidance restrictions and a separate explicit Start after acknowledged authority changes.
- [x] T047 [US3] Add compact budget/current-window/history controls and explicit legacy adoption in `ui/src/routing.ts`, `ui/src/tasks.ts` and `ui/src/index.html`, preserving pending grant/adoption/Start identities independently and requiring no extra ordinary-task confirmation or provider configuration wizard.
- [x] T048 [US3] Enforce the shared window and existing per-attempt ceiling in `internal/engine/engine.go` and `internal/engine/control.go`, rejecting new work at expiry, requesting controlled Stop, allowing bounded cleanup/evidence settlement without new authority and blocking new windows when cleanup/owner/commit evidence is uncertain.
- [x] T049 [US3] Run focused store/engine fault cases and actual CLI/browser journeys from `internal/store/routing_budget_test.go`, `internal/engine/routing_continuation_test.go`, `internal/cli/routing_integration_test.go` and `ui/tests/routing-budget.test.mjs`; record exact units, predecessor identities, no-rearm/no-B workers, source/index preservation and cleanup/fence outcomes in `specs/004-economical-routing/acceptance.md`.

**Checkpoint**: Additional authoring remains bounded and attributable. Category B is visibly
unsupported, legacy work gains no authority by migration, and a retained candidate is never
assumed integrated into a successor or promoted to verified delivery.

## Phase 6: User Story 4 — Inspect truthful routing and usage across upgrades (P2)

**Goal**: Bounded UI/CLI views explain requested, decided and actual facts, cumulative work and
usage coverage without rewriting historical evidence or newer local work.

**Independent test**: Inspect mixed successful/failed/cancelled/interrupted histories, huge exact
counts and extreme estimate lexemes through paged UI/CLI reads. Overlapping scopes are not summed,
fake usage stays unknown, old receipts remain byte-identical, and delayed reads preserve selection.

### Tests before enabling aggregation/history surfaces

- [x] T050 [P] [US4] Write exact usage/coverage and bounded-history tests in `internal/routing/usage_test.go` for overlapping top-level/whole-tree scopes, duplicate observations, missing/omitted data, failed/cancelled work, values beyond floating precision, extreme valid estimate lexemes and partial/unaggregatable results without clipping originals or inventing zeroes.
- [x] T051 [P] [US4] Write strict current/old-owner and read-contract regressions in `internal/httpapi/routing_test.go` and `internal/cli/maintenance_integration_test.go`, covering current004 execution versus exact known002/003 Stop, request binding, invalid/foreign cursors, duplicate/null/unsafe inputs, mutation failures and zero vendor calls during metadata/history reads.
- [x] T052 [P] [US4] Write actual browser routing/usage/history tests in `ui/tests/routing-history.test.mjs` for sourced estimates/reference prices/unknown charges, exact literal counters, decision versus actual-model facts, historical/current separation and delayed route/detail/report replies while task/pin/selection state changes.

### Implementation and integration

- [x] T053 [US4] Implement bounded nonoverlapping usage projection and exact subtotal/estimate representation in `internal/routing/usage.go`, preserving complete original lexemes and missing coverage; an unaggregatable value stays linked/partial instead of being evaluated through floats, omitted silently or used to fail authoring.
- [x] T054 [US4] Implement stable bounded routing/decision/budget-ledger/task-usage reads in `internal/store/routing_reads.go` and `internal/store/cursor.go`, retaining filter/high-water cursor semantics, exact counters, nullable unadopted budgets, per-page coverage and immutable references without full transcript/history scans at admission.
- [x] T055 [US4] Expose the complete shared inspection contract in `internal/engine/observe.go`, `internal/app/routing.go`, `internal/httpapi/routing.go` and `internal/cli/routing.go`, including task usage pagination, known subtotals/coverage, source-age validity, separate estimates/prices/charges and current versus historical route facts; observations make no model/vendor call.
- [x] T056 [US4] Render compact route/budget/usage facts with explicit complete reads in `ui/src/routing.ts`, `ui/src/tasks.ts`, `ui/src/routing-types.ts` and `ui/src/native-observations.ts`, separating requested selectors, frozen decisions and sourced actual observations, preserving exact strings and validating artifact/attempt provenance and pending-selection generations.
- [x] T057 [US4] Qualify the focused read/usage/maintenance and actual browser cases in `internal/routing/usage_test.go`, `internal/httpapi/routing_test.go` and `ui/tests/routing-history.test.mjs`, then inspect desktop/360px keyboard, focus, reduced-motion and approved asset hashes; record actual results and private screenshot references in `specs/004-economical-routing/acceptance.md`.

**Checkpoint**: Every story has a usable independently inspectable result. No view turns a route,
fake response, client estimate, declared tier or page subtotal into acceptance, billing or savings.

## Phase 7: Release, independent review and acceptance

- [x] T058 Extend the extracted-native package smoke in `scripts/package.test.mjs` and version/exclusion checks in `scripts/package.mjs` for routed Claude and pinned Codex local fakes, report/budget/evidence inspection and exact service Stop, retaining the empty-PATH startup/runtime prerequisites and excluding private state, credentials, vendor SDKs and native build outputs from releases.
- [x] T059 [P] Reconcile user-facing setup/commands/capability limits and RTE/SC evidence links in `README.md`, `docs/refactor/traceability.md`, `docs/refactor/delivery-plan.md` and `specs/004-economical-routing/quickstart.md`; state A-only continuation, six-profile assumptions, no direct paid executor, unknown live access and no measured savings without changing approved brand notices.
- [x] T060 Regenerate the embedded browser output including `internal/webui/dist/index.html` with `npm run build`, check it matches `ui/src/` and current metadata/package declarations, and retain native binaries/private screenshots only as ignored outputs; rebuild again after any later source correction that changes shipped assets.
- [x] T061 Obtain independent source/failure-path/UI review against all four stories and the constitution, record concrete inspected revisions/findings in `specs/004-economical-routing/review.md`, correct every blocker in its owned source/test paths, rerun affected checks and obtain closure before final source freeze.
- [x] T062 Run final `npm run build`, `npm test` and `npm run check` serially against the reviewed source and matching `internal/webui/dist/`, including all offline Go/browser/extracted-package regressions; commit the reviewed implementation/assets on `refactor` and record the exact checked implementation revision in `specs/004-economical-routing/acceptance.md` without claiming live provider qualification.
- [x] T063 Qualify that implementation revision through all six actual Windows/macOS/Linux AMD64/ARM64 jobs and supported Linux AMD64 race job in `.github/workflows/verify.yml`, record terminal job/commit evidence in `specs/004-economical-routing/acceptance.md`, and correct/re-review/requalify real failures without skips, compile-only/native equivalence claims or automatic vendor calls.
- [x] T064 Close all 28 RTE and ten SC mappings with actual scenario/test/review/native evidence in `specs/004-economical-routing/acceptance.md`, `specs/004-economical-routing/review.md` and `docs/refactor/traceability.md`, update parent delivery status and commit the acceptance records; keep feature005 implementation closed until every required gate and finding is resolved.

## Dependencies and execution order

1. T001 is a hard design/commit gate. T002–T003 prepare distinct offline fixtures after it.
2. T004 freezes the shared DTO/fact API; T005 then fixes validation rules. T006 and T007 can be
   authored in parallel after those definitions. T008 follows T007 and catalog/default freeze;
   T009–T010 follow validated schema/types. T011 uses the frozen capability contract. Finish the
   foundation before activating story dispatch.
3. US1 T012–T015 may be authored independently after T004–T005 freeze shared types/validation;
   execution and activation wait for the full foundation. T016/T017 implement their pure
   cases; T018 and T019 use those results plus the durable primitives. HTTP/CLI/UI integration
   T020–T023 follows the frozen interfaces; T024 proves the vertical slice.
4. US2 is independently exercised with pre-existing/native-default profiles but integrates the
   US1 router. T025–T027 can be authored concurrently after foundation. T028–T031 establish
   actual state/guards before exposing mutable routes and UI in T032–T034; T035 qualifies it.
5. US3 tests T036–T039 can be authored after foundation while US1/US2 are implemented. Its runtime
   integration requires their admission/report APIs: T040/T041 before T042–T044, T045–T048 before
   T049. A proof cannot trigger a successor until the durable guards and real no-Prepare ordering
   are complete. Budget changes never substitute for a new request or re-arm an older proof.
6. US4 tests T050–T052 can be authored once its DTO/history surface is frozen. T053/T054 then
   supply the views consumed by T055/T056; T057 requires all historical/control fixtures. It
   does not depend on new provider telemetry or a positive unsupported-B implementation.
7. Final tasks require all four story checkpoints. T058/T059 can be authored in distinct files;
   all builds/tests share one execution queue. T060–T064 are ordered release gates, with any
   correction invalidating and rerunning the affected evidence/assets before acceptance.

### Explicit ownership before parallel implementation

| Lane | Exclusive source/test boundaries until handoff | Coordination rule |
| --- | --- | --- |
| Domain/store | `internal/domain/routing*.go`, optional domain execution additions, `internal/store/routing*.go`, existing store schema/task/attempt/fingerprint integration | Freeze DTOs/transaction signatures first; one writer for shared existing store files. |
| Pure routing/assessment | `internal/routing/` and its tests | No SQL/process/network ownership; consume frozen DTOs. Publish immutable decision/assessment results. |
| Owner/engine/interfaces | `internal/engine/`, `internal/app/`, `internal/httpapi/`, `internal/cli/`, metadata/manifests and package scripts | One writer for shared engine/control/parser files; independent test-only files may be assigned separately. |
| Native integration | Specifically assigned `internal/adapters/native.go`, `internal/adapters/codex.go` and `internal/adapters/routing_test.go` | Agree the resolved-target signature with engine/domain before editing; preserve native process/privacy/evidence code. |
| UI | `ui/src/`, `ui/tests/`, generated `internal/webui/dist/` | UI owns TS DTO mirrors after Go/contract freeze. Serialize Tasks/Agents/shared helper edits; one owner regenerates assets. |
| Independent review | Review/evidence records only, unless a correction is explicitly reassigned | Reviewer must not approve their own source. Native/runtime claims require actual observed results. |

Parallel labels grant no overlapping ownership. Explicitly assign individual agents/files at
implementation start; this table is a boundary plan, not an implicit delegation of every file.
Do not modify `internal/process/`, `internal/localstate/` or `internal/evidence/` as speculative
hardening for routing; any necessary consequential change requires reconciled scope and review.

### Parallel examples by story

- US1: after T004–T011, author T012 pure ranking, T013 assessment, T014 engine/adapter fixtures and
  T015 browser cases concurrently in their distinct new test files. Run their checks serially.
- US2: author T025 report transactions, T026 independent CLI clients and T027 actual browser
  report cases concurrently; hand report types to engine/UI only after store/API signatures freeze.
- US3: author T036 budget faults, T037 real native control, T038 adoption and T039 interface lost
  acknowledgements concurrently. Do not run multiple native integration/build batches together.
- US4: author T050 exact projection, T051 strict transport/maintenance and T052 browser-history
  tests concurrently; shared transport/UI implementation edits wait for the relevant writer.

## Requirement traceability

Each mapping identifies implementation and meaningful evidence tasks; T061–T064 independently
review and close every row rather than treating task completion as proof.

| Requirement | Tasks |
| --- | --- |
| RTE-001 | T004, T006, T014, T028, T034, T056 |
| RTE-002 | T011, T015, T020–T024 |
| RTE-003 | T005–T006, T012–T019, T024 |
| RTE-004 | T012–T013, T016–T017, T023–T024 |
| RTE-005 | T012, T017, T025, T030–T031, T035 |
| RTE-006 | T025–T026, T028–T031, T034–T035 |
| RTE-007 | T004–T005, T025, T029–T030, T032–T035 |
| RTE-008 | T025, T029–T031, T034–T035 |
| RTE-009 | T026, T031, T034–T035 |
| RTE-010 | T005, T012, T014, T019–T021, T037 |
| RTE-011 | T005, T012, T014, T020–T021, T033, T059 |
| RTE-012 | T009–T010, T036, T040, T047–T049 |
| RTE-013 | T010, T036, T040, T042–T044, T049 |
| RTE-014 | T036, T038–T041, T044, T048–T049 |
| RTE-015 | T005, T037, T042–T044, T046–T049 |
| RTE-016 | T019, T031, T037, T041–T044, T048–T049 |
| RTE-017 | T025–T026, T031, T037, T042–T044, T048–T049 |
| RTE-018 | T037, T042–T044, T049 |
| RTE-019 | T004, T010, T017–T019, T054–T056 |
| RTE-020 | T010, T018, T025, T036, T038–T040, T051 |
| RTE-021 | T050, T053–T057 |
| RTE-022 | T005, T017, T050, T053–T059 |
| RTE-023 | T020–T023, T027, T032–T035, T039, T045–T047, T051–T057 |
| RTE-024 | T002, T007–T011, T038–T040, T047, T051, T054, T057 |
| RTE-025 | T014, T019, T026, T037, T043–T044, T048–T049, T058 |
| RTE-026 | T015, T022–T023, T027, T034, T039, T047, T052, T056–T060 |
| RTE-027 | T005–T006, T012–T019, T030, T051, T054–T055 |
| RTE-028 | T001–T003, T024, T035, T049, T057–T064 |

| Success criterion | Primary evidence tasks |
| --- | --- |
| SC-001 | T015, T020–T024 |
| SC-002 | T012–T017, T024 |
| SC-003 | T026, T028–T035 |
| SC-004 | T025, T029–T035 |
| SC-005 | T005, T012, T014, T020–T021, T033, T037, T049 |
| SC-006 | T036–T049 |
| SC-007 | T026, T031, T037, T042–T044, T048–T049 |
| SC-008 | T050, T053–T057 |
| SC-009 | T002, T007–T011, T038–T040, T047, T051–T052, T054, T057 |
| SC-010 | T001, T059–T064 |

## Implementation strategy

Deliver the foundation, then US1 as the smallest complete demonstrable increment. Add the US2
optional reports and shared declared pools, US3 explicit cumulative authority/closed continuation,
then US4 complete historical/usage inspection. Tests may be authored in parallel on frozen types;
activation and shared-file edits follow the dependency order. No story is declared accepted by
an isolated model/fixture exit. Finish independent review, corrections, exact asset generation,
all developer/native/race gates and evidence mapping before feature005 implementation.
