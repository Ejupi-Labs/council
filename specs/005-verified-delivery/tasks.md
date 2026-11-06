# Tasks: Verified task delivery

**Input**: [spec.md](spec.md), [plan.md](plan.md), [data-model.md](data-model.md),
[contracts/workflow.md](contracts/workflow.md), [research.md](research.md) and
[quickstart.md](quickstart.md).
**Branch**: Physical `refactor`; logical feature `005-verified-delivery`.
**Status**: Future ordered implementation work, not completed implementation or acceptance.

Root has completed actual Specify setup-tasks/prerequisite discovery. No runtime edit is allowed
until independent Specify analysis, corrections and the complete design/tasks commit close T001.
VFD-025 requires meaningful failing cases before the corresponding behavior is enabled. Compile,
fixture or selector failures are not feature-red evidence; repair prerequisites and observe the
intended failure. All agents/verifiers/publishers are labelled isolated local fixtures. No SDK,
vendor executable/probe, credential inspection, live call or native global-setting change is allowed.

## Format and ownership rules

Every task uses `- [ ] Tnnn [P?] [USn?] Description with exact paths`. `[P]` permits independent
file authoring after stated prerequisites; it never permits concurrent heavy checks or overlapping
writers. Root owns the single build/test/browser queue and generated `internal/webui/dist/`.
Assign individual shared-file writers before implementation. No graph/recovery framework or
feature-006 runtime is part of this list.

## Phase 1: Setup and reusable offline fixtures

- [x] T001 Close independent Specify analysis and design findings in `specs/005-verified-delivery/design-review.md`, reconcile the six design artifacts and this task list, record exclusive source ownership and commit the reviewed design on `refactor` before any runtime edit.
- [ ] T002 [P] Add populated accepted-schema-4 fixtures in `internal/store/workflow_fixture_test.go`, preserving exact old JSON/receipts/fingerprints, Unicode/BOM/CRLF artifacts, response acceptance, large counters, saved two-unit defaults, held claims/incidents and private disposable project identity.
- [ ] T003 [P] Extend `ui/tests/helpers.mjs` and add `internal/cli/workflow_test_helpers_test.go` for real independent clients, schema-4 seed handoff, controlled stage/effect/acknowledgement barriers and cleanup registered before assertions; retain authenticated exact-owner shutdown and token-redacted private diagnostics.

**Checkpoint**: Accepted state and controlled clients are reproducible without vendor access.

## Phase 2: Foundation — frozen types, durable authority and owned execution

- [ ] T004 Freeze DTOs/controller inputs in `internal/domain/workflow.go` and optional tags in `internal/domain/execution.go`: candidate/response bindings, selected producer, condition registry/digests, role/stepId/executorKind, preferences, checks/reviews/findings, advancement, effects and acceptance. Publish exact Go interfaces before dependent lanes bind them; public inputs cannot supply trusted proofs.
- [ ] T005 Write failing boundaries then implement strict validation in `internal/domain/workflow_validation_test.go` and `internal/domain/workflow_validation.go` for modes, absent/null/duplicate/empty values, exact counters, aggregate bounds, historical selectors, declaration-only fields, whole/subset scope, subject consistency and unsupported remote postconditions.
- [ ] T006 [P] Write migration/compatibility fault cases in `internal/store/workflow_migration_test.go` using T002: exact schema-1–4 rows/receipts/held state survive; every saved finite default stays unchanged; only genuinely fresh initialization gets four; delivery initializes review-only; cancellation/commit uncertainty cannot seed authority or refill work.
- [ ] T007 Implement only the nine additive schema-5 tables plus their indexes and validated migration in `internal/store/workflow_schema.go`, `internal/store/workflow_migration.go`, `internal/store/schema.go` and `internal/store/store.go`, retaining exact DDL checks, owner/privacy/lock/WAL/FULL behavior and refusing unknown/colliding state without fallback or repair.
- [ ] T008 Write durable step/admission/proof tests in `internal/store/workflow_steps_test.go` for once-bound attempts/subjects, native-only debits, local zero-model-unit admissions, immutable terminal/applicability history, exact replay and concurrent last-unit/next-step consumption, including 004A replacement as a NEW step execution.
- [ ] T009 Implement typed state/admission primitives in `internal/store/workflow_state.go`, `internal/store/workflow_admission.go` and `internal/store/workflow_steps.go`: atomic step/attempt/native decision/debit/receipt, unique proof consumption and new replacementOfStepId lineage for 004A without rebinding the predecessor or producing two successors. Preserve original fingerprints and old replay before resolving defaults; no filesystem/process callback runs under SQL.
- [ ] T010 Implement the fixed offline scenarios in `internal/fixture/workflow.go` with Root-owned dispatch hooks in `internal/cli/cli.go`: two public Start fixtures, three controller-only native role fixtures and same-binary `__fixture check`/`__fixture publish` children. Consume actual candidate/finding bytes, label output and forbid arbitrary stage selection, vendor fallback or fabricated provider/usage observations.
- [ ] T011 Update exact current/maintenance contracts in `internal/domain/types.go` (authoritative AppVersion), `internal/domain/execution.go` (authoritative ExecutionContract), `internal/app/types.go`, `internal/engine/observe.go`, `internal/cli/metadata.go`, `internal/cli/execution.go`, `ui/src/api.ts`, `package.json` and `package-lock.json`; add cases in `internal/cli/workflow_compatibility_test.go`. Core is the sole existing-domain writer coordinated with T004; UI owns its api.ts mirror; Root owns remaining metadata/manifests. Require dev.5/005-v1/verified_delivery, preserve exact dev.2/3/4 Stop, raw old receipts and standalone tasks; only a new non-replayed omitted workflow resolves to Auto, while explicit author_only remains unqualified compatibility.
- [ ] T012 Write owned-stage lifecycle cases in `internal/engine/workflow_control_test.go` for native, command-check, no-child builtin/application and external executors sharing one claim/window; Stop permits, output/drain/descendant settlement, late failure, owner loss and persistence faults must preserve authoritative outcomes. Zero model debit is not zero measured native usage.
- [ ] T013 Integrate those executor-specific lifecycle/control proofs in `internal/engine/workflow_execution.go`, `internal/engine/control.go` and `internal/engine/engine.go`, reusing existing process/evidence ownership. Native roles alone route/debit; local stages cannot renew the clock or invent native root/provider/token facts; no next stage crosses unresolved cleanup/effect/owner fences.

**Checkpoint**: Shared types, migration, exact compatibility, single ownership and bounded admission
are usable before story behavior. T004/T005 interfaces are a hard prerequisite for dependent edits.

## Phase 3: US1 — Finish a bounded edit proportionately (P1, MVP)

**Goal**: Normal Auto can prove a supported document change with one author, or retain an honest
gap, without a planning/reviewer ritual. Saved prospective delivery consent is separate from access.
**Independent test**: A real labelled document fixture produces exact expected bytes, builtin proof
and qualified review-only delivery with one native admission. Plain response-only work retains the
exact user-acceptance path. Local-application success joins this checkpoint after US3.

### Red cases before behavior

- [ ] T014 [P] [US1] Write pure policy/proof/convention cases in `internal/workflow/direct_test.go` and `internal/workflow/checks_test.go`: complete literal grammar, exactly one differing old literal, full delta/index/conditions, no short-text/model-tier shortcut, no empty-check/review bypass, supported Go/Cargo/direct Node and unsupported npm/wrapper declarations.
- [ ] T015 [P] [US1] Write actual owner/store direct and response cases in `internal/engine/workflow_direct_test.go` and `internal/store/workflow_acceptance_test.go`: one author/no-child proof, old-two/fresh-four, prospective review-only consent, no false no-op integration, exact AcceptResponse replay/JSON, current response producer and human acceptance after clock expiry without launching work.
- [ ] T016 [P] [US1] Write real CLI/browser minimal-Start/preference/author_only/lost-reply cases in `internal/cli/workflow_integration_test.go` and `ui/tests/workflow-direct.test.mjs`, retaining separate autosave/Start identities, newer text/options/selection, unknown fake facts and no mandatory check/account/model wizard.

### Implementation and qualification

- [ ] T017 [US1] Implement deterministic planning, canonical condition registry/digests, literal proof and captured check selection in `internal/workflow/plan.go`, `internal/workflow/conditions.go`, `internal/workflow/direct.go` and `internal/workflow/checks.go`. Preserve full originals; resolve supported native programs outside SQL, keep exact argv and require a named gap instead of shell/script translation or guessed sufficiency.
- [ ] T018 [US1] Implement prospective preference and task/version authority capture in `internal/store/delivery_preferences.go`, `internal/store/workflow_tasks.go` and `internal/store/tasks.go`, including explicit workflow adoption without work/refill, author_only/standalone compatibility and immutable saved/task delivery snapshots after exact raw receipt replay.
- [ ] T019 [US1] Implement observed check and direct/response acceptance records in `internal/store/workflow_checks.go` and `internal/store/workflow_acceptance.go`. Keep builtin completion distinct from process exit, bind current exact subjects/conditions, preserve original ResponseAcceptance/fingerprints/envelopes, and record separate response-only completion rather than candidate integration.
- [ ] T020 [US1] Integrate direct author/check progression and shared application routes in `internal/engine/workflow_direct.go`, `internal/engine/workflow_checks.go`, `internal/app/workflow.go` and `internal/httpapi/workflow.go`; run command checks through real owned execution and builtin proof without a child, project next action/owner and expose saved delivery preference without implicit application consent.
- [ ] T021 [US1] Implement new CLI preference/Start/adoption options in `internal/cli/workflow.go` and `internal/cli/workflow_parse.go`, with Root-owned delegates in `internal/cli/parse.go` and `internal/cli/execution.go`. Preserve explicit values/omission, bounded check/delivery files, mutually exclusive delivery flags, request identity, strict JSON and stdout/diagnostic separation.
- [ ] T022 [US1] Mirror frozen DTOs and implement minimal composition/prospective preference/direct-result UI in `ui/src/workflow-types.ts`, `ui/src/workflow.ts`, `ui/src/tasks.ts`, `ui/src/workspace.ts`, `ui/src/index.html` and `ui/src/style.css`. Keep normal Start simple, full access distinct from delivery consent and pending bodies stable; present the deliverable response producer rather than a supporting stage's latest response.
- [ ] T023 [US1] Run the focused T014–T016 cases serially and record exact one-author/builtin/review-only and plain-response outcomes in `specs/005-verified-delivery/acceptance.md`. Preserve source/index bytes and original manual response acceptance; do not claim local application or reviewed generic changes before their later story checkpoints.

**Checkpoint/MVP**: A complete supported review-only direct outcome and preserved plain-answer
acceptance are usable. All four stories and release gates still remain required for 005 acceptance.

## Phase 4: US2 — Review and correct the actual selected subject (P1)

**Goal**: Fresh independent review, stable blockers and one bounded correction qualify actual
candidate/response bytes; affected evidence is replaced without rewriting prior failures.
**Independent test**: Author A → actual review/blocker → correction B → affected checks/re-review
fits four native admissions; preserved two-unit authority blocks honestly. Corrected response text
cannot reuse stale review just because source files are unchanged.

### Red cases before behavior

- [ ] T024 [P] [US2] Write role/handoff/protocol cases in `internal/adapters/review_test.go` and `internal/workspace/handoff_test.go` for real candidate bytes/index, fresh role contexts, literal pins, unavailable access, strict whole structured output, wrong subjects/refs, modified review workspace, malformed/missing/repeated/trailing terminals and preserved author protocol behavior.
- [ ] T025 [P] [US2] Write immutable review/finding/resolution/applicability cases in `internal/store/workflow_review_test.go`: current candidate-or-response binding, server finding IDs, no author self-approval, no inaccessible-evidence pass, full/declared-subset reuse, source-modifying checks and response-digest changes invalidating prior applicability.
- [ ] T026 [P] [US2] Write actual workflow fault/continuation cases in `internal/engine/workflow_review_test.go` for four-role lineage, required-check failure, cycle/old-two exhaustion, operational failures supplying no trigger, Stop/grant/guidance/report/owner races, old proofs after restart and 004A new-step replacement uniqueness within a workflow.
- [ ] T027 [P] [US2] Write real review/correction/required-response UI and independent-CLI cases in `ui/tests/workflow-review.test.mjs` and `internal/cli/workflow_review_integration_test.go`, including actual changed-file inspection, missing access, final response producer vs reviewer report, stale subject selection and separate explicit advance/check acknowledgements.

### Implementation and qualification

- [ ] T028 [US2] Implement exact predecessor materialization and fresh supported native role profiles in `internal/workspace/handoff.go`, `internal/adapters/review.go`, `internal/adapters/codex.go` and `internal/adapters/claude.go` where present. Retain original instructions/evidence, verify source/index/artifact identity, bound schema/argv and parse only the attributable whole structured result; unsupported profile/access is a gap, not altered native settings or session reuse.
- [ ] T029 [US2] Implement pure review validation, subject applicability and correction decisions in `internal/workflow/review.go`, `internal/workflow/applicability.go` and `internal/workflow/correction.go`; distinguish supplied access from reported inspection, immutable outcome from current applicability and valid evidence triggers from new authority.
- [ ] T030 [US2] Persist immutable reviews/findings/resolutions and candidate/response evidence uses in `internal/store/workflow_reviews.go`, `internal/store/workflow_findings.go` and `internal/store/workflow_checks.go`, assigning stable IDs and accepting resolution only after applicable affected proof. Corrected candidate/response identities retain exact predecessor lineage and complete original conditions.
- [ ] T031 [US2] Implement unique current advancement/recheck transactions in `internal/store/workflow_progress.go` and `internal/store/workflow_steps.go`: finite native debits, one correction cycle, current subject/control/window and consumed proof, new execution for 004A replacement, no old-step rebinding, grant/configuration re-arm or startup scan.
- [ ] T032 [US2] Orchestrate review/correction/rechecks and expose current operations in `internal/engine/workflow_review.go`, `internal/engine/workflow_progress.go`, `internal/app/workflow.go` and `internal/httpapi/workflow.go`. Recheck current guards before effects/admission; required response-only evidence remains conjunctive and uses the selected author/corrector producer for the unchanged AcceptResponse action.
- [ ] T033 [US2] Extend `internal/cli/workflow.go`, `ui/src/workflow.ts` and `ui/src/tasks.ts` with findings, actual inspected scope, gaps, linked correction/recheck and explicit advance/check controls. Preserve current-vs-historical subjects, pending request phases/newer edits, and exact response acceptance without showing a review JSON report as the user's answer.
- [ ] T034 [US2] Run T024–T027 serially and record real predecessor-byte reads, strict invalid-result failures, four-admission lineage, exhausted-two behavior, applicable rechecks and required-response acceptance in `specs/005-verified-delivery/acceptance.md`; original failed terminals and unrelated qualifying work remain unchanged.

**Checkpoint**: Review/correction is useful and finite. It grants no scope/budget, and no result
label or unchanged files can make a stale response review current.

## Phase 5: US3 — Apply safely or retain an honest delivery outcome (P1)

**Goal**: Supported scoped application preserves user work; external command completion is distinct
from remote publication, with durable conflict/partial/uncertain outcomes and no blind replay.
**Independent test**: Apply supported text add/replace/delete around dirty/index state; exercise
exclusive-move/quarantine races and faults. A fixed explicitly scoped publisher runs once; unscoped
prose launches nothing and uncertain acknowledgement never auto-relaunches it.

### Red cases before behavior

- [ ] T035 [P] [US3] Write native application cases in `internal/workspace/apply_test.go` for exact bytes/index/branch/physical identity, existing parents, ordinary single-link text, same-filesystem private staging/quarantine, add/replace/delete, locks/recreated names/live handles and unsupported mode/index/type/metadata/symlink/cross-volume cases; preserve originals and safely retained conflicting evidence.
- [ ] T036 [P] [US3] Write journal/receipt fault cases in `internal/store/delivery_test.go` for intent-before-effect, append-only observations, attempt-owned destination artifacts, before/after Commit uncertainty, partial/interrupted application, exact replay and immutable prior terminals without filesystem work inside SQL.
- [ ] T037 [P] [US3] Write real owner/CLI/browser delivery cases in `internal/engine/workflow_delivery_test.go`, `internal/cli/workflow_delivery_integration_test.go` and `ui/tests/workflow-delivery.test.mjs` for saved scope once, review-only vs applied, zero remaining native units, Stop permits, conflicts, lost replies, unscoped commit/publisher suggestions and fixed publisher unknown-acknowledgement/no-relaunch behavior.

### Implementation and qualification

- [ ] T038 [US3] Implement the frozen bounded application algorithm in `internal/workspace/apply.go`, `internal/workspace/apply_linux.go`, `internal/workspace/apply_darwin.go` and `internal/workspace/apply_windows.go`: anchored handles/FDs, exclusive no-replace moves, same-filesystem private staging/quarantine and retained originals/live displaced handles. Refuse unsupported metadata/paths/volumes instead of overwrite, parent creation, copy-delete fallback, index refresh or blind restore.
- [ ] T039 [US3] Implement application operation/intents/outcomes in `internal/store/delivery_operations.go` and `internal/store/delivery_effects.go`, binding one no-process attempt and typed current candidate/evidence/scope to append-only destination effects and artifacts. Known receipts replay; possible effects fence uncertainty rather than fabricate rollback, success or a fresh automatic action.
- [ ] T040 [US3] Integrate local application in `internal/engine/workflow_delivery.go` and `internal/engine/control.go`: acquire the shared slot, persist intent and current Stop/owner/effect permits, revalidate under the native effect boundary, settle no-child proof and preserve primary faults. Partial/quarantine/recreated-path states remain inspectable and cannot satisfy application acceptance.
- [ ] T041 [US3] Implement explicit external execution in `internal/engine/workflow_external.go` using existing owned processes and step storage: one captured native command/action/destination, no shell or generated argv, current verified subject/scope, complete output/cleanup and command_completion result only. Known replay never relaunches; unknown acknowledgement stays uncertain with no remote-attestation claim.
- [ ] T042 [US3] Expose delivery declarations/results and scoped error/receipt semantics through `internal/app/workflow.go`, `internal/httpapi/workflow.go` and `internal/cli/workflow.go`, retaining command-only external outcomes, full effect links, required local applied evidence and explicit unsupported/remaining actions; local application authorizes no commit, push or deployment.
- [ ] T043 [US3] Extend `ui/src/workflow.ts`, `ui/src/tasks.ts` and `ui/src/index.html` with captured scope, reviewable/applied/conflict/partial/uncertain and external-command outcomes, preserving separate pending bodies and newer selection. Keep existing saved access consent distinct, no false no-op integration for response-only and no fallback from required local application into a completed review-only task.
- [ ] T044 [US3] Run T035–T037 and the US1 local-application branch serially; record actual destination/index/metadata/quarantine preservation, fault/Stop ordering, zero native debit and fixed publisher launch/effect evidence in `specs/005-verified-delivery/acceptance.md`. An inaccessible live user's path is retained with a named evidence gap, never claimed copied or frozen.

**Checkpoint**: Qualified reviewable delivery and known local application are distinct. Recovery
beyond bounded truthful evidence remains 007; no universal editor exclusion or exactly-once remote
effect is claimed.

## Phase 6: US4 — Inspect and control one coherent workflow (P2)

**Goal**: UI/CLI show the same current/historical workflow, deliverable producer, next action,
evidence and unknown usage while late replies preserve newer intent and selection.
**Independent test**: Two actual clients inspect retained candidate/response histories, full changed
file/output bytes and controls across guidance/adoption; foreign cursors, stale operations and
late reads cannot alter authority, input, selected subject or acceptance.

### Red cases before complete read surfaces

- [ ] T045 [P] [US4] Write actual browser read/control cases in `ui/tests/workflow-history.test.mjs` for historical workflow-id/task-version selection, whole-item candidate-change paging, full output, selected producer vs latest supporting attempt, stale candidate/response reviews, human acceptance after window expiry, separate pending phases and exact native strings/unknown local facts.
- [ ] T046 [P] [US4] Write strict transport/independent-client cases in `internal/httpapi/workflow_contract_test.go` and `internal/cli/workflow_history_integration_test.go` for wrong/foreign selectors/cursors, null/duplicate/oversized inputs, original old receipts, authority-less reads/author_only, exact owner/version maintenance and zero new work during observation.

### Implementation and qualification

- [ ] T047 [US4] Implement bounded selected-workflow/history/candidate-change reads in `internal/store/workflow_reads.go` and `internal/store/cursor.go`: default30/max100, task/selector/high-water binding, complete-item 4 MiB pages without clipping/skips and artifact-linked file bodies. Historical authority stays retrievable; read enrichment never rewrites old rows/receipts or creates authority.
- [ ] T048 [US4] Expose current/historical views and derived task outcome in `internal/engine/observe.go`, `internal/app/workflow.go` and `internal/httpapi/workflow.go`. Keep actual latest attempt distinct from selected candidate/response producer, standalone workflow null, response user-acceptance after eligible settlement/expiry, stage-owned evidence and native/local usage semantics; observing never launches work.
- [ ] T049 [US4] Complete new CLI reads/selectors/paging in `internal/cli/workflow.go` and `internal/cli/workflow_parse.go`, including steps/reviews/checks/findings/deliveries/candidates and candidate changes, exact strings, full artifact references, current mutation controls and mutually exclusive historical selectors with no protocol stdout contamination.
- [ ] T050 [US4] Complete typed progress/subject/evidence views in `ui/src/workflow-types.ts`, `ui/src/workflow.ts`, `ui/src/tasks.ts`, `ui/src/native-observations.ts` and `ui/src/style.css`. Preserve generation guards, original bytes/downloads, current-vs-historical applicability, response producer, saved delivery scope and distinct stage outcomes; zero model debit is not reported zero tokens/charges, and native estimate lexemes stay exact strings.
- [ ] T051 [US4] Run T045–T046 plus retained 001–004 browser/CLI regressions serially and record actual interface agreement, stale-input protection, historical/current subject correctness, complete byte reads and no observation-driven execution in `specs/005-verified-delivery/acceptance.md`; fix real regressions rather than weakening original acceptance/replay requirements.
- [ ] T052 [US4] Prepare/run ignored `.council/refactor/005-render-qa.mjs` after the queue grants a window; capture and inspect real 1440px/360px workflow/check/review/correction/response/delivery/error/Stop states under `.council/refactor/screenshots/005-verified-delivery/`. Verify keyboard/focus, text states, readable contrast, reduced motion, overflow and approved source/dist/served asset hashes; obtain independent image inspection and record actual evidence in `specs/005-verified-delivery/acceptance.md`.

**Checkpoint**: Each story is inspectable through the same records. No latest-stage exit, fake
result, partial output, preview, page total or unknown metric becomes acceptance or publication.

## Phase 7: Release, independent review and acceptance

- [ ] T053 Extend extracted-native/version/exclusion qualification in `scripts/package.test.mjs` and `scripts/package.mjs` for direct/review/correction/check/application and fixed scoped-publisher journeys, exact Stop and empty-PATH startup. Fixed children use the extracted binary without Node/npm/Go runtime prerequisites; exclude private state/screenshots/credentials/vendor SDKs/native build outputs from source packages.
- [ ] T054 Reconcile supported workflows, response branches, prospective preferences, saved-two/fresh-four budgets, check/application/external limits and evidence links in `README.md`, `docs/refactor/traceability.md`, `docs/refactor/delivery-plan.md` and `specs/005-verified-delivery/quickstart.md`, retaining approved brand notices and separating offline/native results from unperformed live qualification.
- [ ] T055 Obtain independent source/failure-path/UI review in `specs/005-verified-delivery/review.md`, correct every blocking finding in its assigned source/test files, rerun affected actual checks/images and obtain independent closure before source freeze. The author cannot approve their own correction.
- [ ] T056 Run final `npm run build`, `npm test` and `npm run check` serially; regenerate and verify `internal/webui/dist/` against current source/metadata and complete all offline/package regressions. Record exact terminal results in `specs/005-verified-delivery/acceptance.md`; later shipped-source corrections require matching assets and affected requalification.
- [ ] T057 Commit and push the independently reviewed implementation and matching `internal/webui/dist/` to origin's physical `refactor` branch before hosted qualification, recording the exact checked revision in `specs/005-verified-delivery/acceptance.md`. Keep native binaries/private work ignored and do not merge into main or infer publication authority from this development commit.
- [ ] T058 Qualify that exact revision through all six actual Windows/macOS/Linux AMD64/ARM64 jobs and supported Linux AMD64 race job in `.github/workflows/verify.yml`; record seven terminal source/job results in `specs/005-verified-delivery/acceptance.md`. Correct/review/rerun real failures; unavailable runners, compile-only output and fakes never substitute for actual platform/provider evidence.
- [ ] T059 Close all 25 VFD and ten SC mappings with actual source/test/review/native evidence in `specs/005-verified-delivery/acceptance.md`, `specs/005-verified-delivery/review.md`, `specs/005-verified-delivery/evidence-matrix.md` and `docs/refactor/traceability.md`; update parent delivery status and commit acceptance records. Keep 006 implementation closed until every required gate and blocking finding is resolved.

## Dependencies and execution order

1. T001 gates every runtime/test edit. T002–T003 can then be authored independently.
2. T004 freezes shared types before T005 and all dependent bindings. T006 can be authored beside
   validation after T002; T007 follows their compatible definitions. T008 precedes T009. T010/T011
   require frozen input/profile definitions; T012 precedes T013. Finish foundation before story
   dispatch. Native-role and local-effect proofs never become public caller-supplied facts.
3. US1 red cases T014–T016 precede behavior. T017 policy/proof and T018/T019 durable records
   precede T020 engine/routes; T021/T022 bind those published interfaces. T023 proves the direct
   review-only/plain-response MVP; application success is intentionally qualified at T044.
4. US2 tests T024–T027 can be authored after foundation; runtime integration requires the US1
   source/subject/receipt primitives. Handoff/validation T028/T029 and records T030/T031 precede
   T032 controller progression and T033 client binding; T034 closes the four-role/response cases.
5. US3 tests T035–T037 can be authored in their separate files while prior stories land. Platform
   application T038 and durable journal T039 precede effect execution T040; T041 independently
   uses the owned-process foundation plus qualifying subjects. T042/T043 expose actual outcomes;
   T044 closes application, publisher and the deferred US1 application outcome.
6. US4 tests T045/T046 follow frozen read DTOs. T047/T048 supply current/historical reads before
   T049/T050 bind clients. T051/T052 require all four story fixtures and preserve the older tests.
7. T053/T054 may be authored in separate owned files after story interfaces stabilize. T055–T059
   are ordered review/freeze/developer/native/acceptance gates. Any correction invalidates its
   affected evidence/assets; it does not erase earlier results or authorize automatic stage replay.

### Explicit future implementation ownership

| Lane | Exclusive boundaries until explicit handoff | Coordination |
| --- | --- | --- |
| Core | `internal/domain/workflow*.go`, assigned optional domain tags, `internal/store/workflow*.go`, `delivery*.go`, schema/tasks/fingerprint/cursor integration | Publish DTO/transaction signatures first; one writer for existing shared store files. |
| Native | `internal/workflow/`, `internal/adapters/review*.go`, assigned native adapter changes, `internal/workspace/handoff*.go`, `apply*.go`, fixed new `internal/fixture/workflow.go` | No SQL/process-owner duplication; coordinate fixed-child dispatch hooks with Root. |
| UI/CLI | `ui/src/`, `ui/tests/`, new `internal/cli/workflow*.go` and their specifically assigned tests | Mirror published DTOs; one Tasks/Workspace/helper writer. Existing CLI dispatch/parser/execution/metadata hooks stay Root-owned. |
| Root | `internal/engine/`, `internal/app/`, `internal/httpapi/`, shared CLI hooks, metadata/manifests/scripts, `.github/workflows/verify.yml`, generated dist and Git | Serialize engine/control edits and the sole heavy execution queue; no other lane rebuilds shipped assets without handoff. |
| Independent review | Assigned review/evidence records and read-only source/image inspection | Corrections require explicit file reassignment; actual runtime/native claims require terminal evidence. |

Task paths specify the necessary work, not permission for two writers. Where a task spans test
lanes, split its listed files explicitly before authoring; the checkbox closes only after all
listed parts qualify. Do not edit unrelated process/privacy/evidence infrastructure speculatively.

### Parallel authoring examples

- Foundation: Core writes T006 migration cases while UI writes T003 client helpers; after types
  freeze, Native can author fixed fixture bodies while Root authors lifecycle tests in separate files.
- US1: Native T014 pure tests, Root/Core T015 assigned integration/store tests and UI T016 actual
  browser/new-CLI tests can be authored independently. Their runs remain serial.
- US2: T024 protocol/handoff, T025 store lineage, T026 engine races and T027 client journeys are
  separate authoring lanes; no client activation precedes the matching durable/controller APIs.
- US3: T035 platform boundary, T036 journal faults and T037 assigned engine/client fixtures are
  independent authoring targets. No application/publisher runs overlap another heavy batch.
- US4: T045 browser and T046 assigned HTTP/CLI regressions can be authored beside finalized read
  implementations; generated assets, real rendering and all release gates stay one queue.

## Requirement traceability

T055–T059 independently close the evidence for every row; a task checkbox alone is not proof.

| Requirement | Specific implementation and meaningful evidence tasks |
| --- | --- |
| VFD-001 | T004, T008–T009, T017–T020, T032–T034, T047–T051 |
| VFD-002 | T014–T017, T020, T023, T024, T029, T034 |
| VFD-003 | T004–T005, T014, T017–T019, T024–T025, T028–T030 |
| VFD-004 | T002, T006–T009, T012–T013, T015, T026, T031, T034, T044 |
| VFD-005 | T008–T013, T026, T031–T032, T036–T041, T044 |
| VFD-006 | T004, T008, T015, T018–T019, T024–T025, T028–T030, T035–T040, T047 |
| VFD-007 | T024, T027–T030, T032–T034 |
| VFD-008 | T024–T025, T028–T030, T034, T045, T050–T051 |
| VFD-009 | T025–T031, T032–T034 |
| VFD-010 | T025–T026, T029–T034, T045, T047–T051 |
| VFD-011 | T005, T010, T014, T017, T020–T023, T037, T041–T042, T046, T049 |
| VFD-012 | T012–T013, T015, T019–T020, T023, T025–T026, T030–T034, T045, T048–T051 |
| VFD-013 | T014–T015, T024–T025, T028–T030, T034, T045, T047, T051 |
| VFD-014 | T005–T007, T015–T016, T018, T021–T023, T037, T042–T044 |
| VFD-015 | T035–T040, T042–T044 |
| VFD-016 | T035–T040, T044 |
| VFD-017 | T006, T008–T009, T015, T026, T031, T036–T041, T044–T046, T051 |
| VFD-018 | T005, T010, T037, T040–T044 |
| VFD-019 | T015, T019–T020, T023, T025, T030–T034, T036, T039–T044, T045, T048–T051 |
| VFD-020 | T008–T013, T026, T031–T032, T035–T037, T040–T041, T044–T046, T051 |
| VFD-021 | T005–T006, T008–T009, T011, T016, T026–T027, T031–T033, T036–T037, T045–T051 |
| VFD-022 | T002, T006–T007, T011, T015–T016, T018–T019, T021–T023, T045–T051 |
| VFD-023 | T003, T011, T016, T021–T023, T027, T033–T034, T037, T042–T043, T045–T052 |
| VFD-024 | T016, T022, T027, T033, T037, T043, T045, T050–T054 |
| VFD-025 | T001–T003, T010, T023, T034, T044, T051–T059 |

| Success criterion | Primary actual evidence tasks |
| --- | --- |
| SC-001 | T014–T016, T023, T035–T037, T044 |
| SC-002 | T024–T027, T034 |
| SC-003 | T014–T015, T024–T027, T034, T035–T037, T044–T046, T051 |
| SC-004 | T035–T037, T044 |
| SC-005 | T006, T008, T015–T016, T026–T027, T034, T036–T037, T044–T046, T051 |
| SC-006 | T012, T026, T034, T035–T037, T044–T046, T051 |
| SC-007 | T010, T037, T041–T044, T053 |
| SC-008 | T016, T027, T037, T045–T046, T051–T052 |
| SC-009 | T002, T006–T007, T011, T015–T016, T023, T045–T046, T051 |
| SC-010 | T001, T023, T034, T044, T051–T059 |

## Implementation strategy

Deliver foundation, then US1 review-only direct/plain-response MVP. Add actual independent
review/correction, supported application/external effects and complete current/historical UI/CLI
inspection in order. Distinct test authoring can overlap once DTOs freeze; durable behavior and
client binding follow their real prerequisites. Keep every requested story within this slice,
then finish independent closure, matching assets, developer/package/native/race evidence and
acceptance before any 006 implementation.
