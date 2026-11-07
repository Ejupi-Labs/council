# Tasks: Dependent and Parallel Task Graphs

**Input**: Reviewed spec and design under `specs/006-task-graphs/`.
**Prerequisites**: plan.md, research.md, data-model.md and all contracts; independent design
findings and cross-artifact analysis must close and commit before application implementation.
**Tests**: Explicitly required by GRA024/SC010. Use meaningful controlled/offline and actual
filesystem/process evidence; do not mirror implementation or call live vendors.
**Organization**: Setup, shared foundation, four independently inspectable stories and final gates.

## Format and ownership

Each executable item uses `- [ ] Tnnn [P?] [USn?] Description with exact paths`. `[P]` allows
separate-file work only after its named prerequisites/contracts are complete. Root owns the
feature pointer, common integration and the one local test/build/browser queue. Agents author
their assigned files and obtain foreign review; they do not run overlapping heavy commands.

Core owns domain/store except Native's explicitly delegated new graph_seals.go/tests. Native
owns workspace graph modules; client work owns UI/new CLI modules. Root owns fixed graph fixture
modules, engine/app/HTTP/metadata/packaging and coordinates independent review.
Exact module splits may be refined before implementation; update tasks/contracts rather than
claiming planned filenames are actual evidence. All work stays on physical `refactor`.

## Historical bounded implementation audit — 7 November 2026

This section preserves the earlier checkpoint. E60–E61 in the acceptance record supersede its open-gate statements; T053–T056 are closed, and T057 is complete subject to committing this documentation-only follow-up before 007.

Checked implementation items identify completed scoped work and its existing source/check evidence.
They do not accept the feature, close a GRA/SC row, qualify a live provider or replace T050–T057.
This audit used the working source, [draft evidence bundles](evidence-matrix.md), recorded focused
outcomes, the actual full Store51.658s terminal record under
`.council/refactor/006-store-phase-full.jsonl`, and the completed mixed Engine queue in
`.council/refactor/006-ancestor-idle-affected.log`. That111.619s queue has eight passed cases and
one actor failure; it is not an all-green suite. Later focused actor/watchdog19.547s and
correction/watchdog36.273s queues passed; actual mutation28.92s and UI rebind30.465s are separately
qualified. The original full Engine600s timeout still requires global/batch qualification.
No new tests/builds ran during this audit.
An incomplete validation task stays open even when its implementation exists. Missing planned
filenames were replaced with actual equivalent modules without removing required behavior.

| Checked tasks | Bounded evidence basis |
| --- | --- |
| T001–T005 | Immutable pre-code gate, E01 strict Domain family and E02 literal schema5/migration/full Store evidence; prior completed checks retained. |
| T006–T008, T010–T011, T024, T026, T039 | E01/E04/E05/E13/E15: strict lossless wire/metadata, canonical root ledger/epoch/window, separate-client last-unit contest, actual overlap/narrower capacity, membership and exact bounded reads. Shared native/release gates remain open. |
| T012, T014–T018 | E03/E07/E08: real local root/input operations, scoped complete originals, actual receiving materialization/Handoff, physical inventory/context guards and no added model debit. Witnessed pre-GO settlement truthfulness is now qualified by E17; the entry cancellation guard does not claim to solve cancellation arising midway through a read. |
| T019–T023 | E03/E04/E09/E13/E16/E19: fixed workers, actual stable creation/local preparation/replay, current-owner actor completion, server controls/current phase and strict app/HTTP/CLI transport. The actor fixture's evidence-backed finite seven-stage watchdog passed19.547s with pure deadline tests and foreign source review; full global/client gates remain open. |
| T025, T027, T032, T034, T037 | E13/E14/E20: separate composer/pending state and shared ownership; real CLI overlap/dependency/provenance/retained bytes/exact Start replay; actual browser lost-ack/newer drafts and exact held/failed/foreign receiving-page guards; actual Engine failed-branch retention. This portfolio uses actual smaller modules instead of duplicate named Go files. Context-blocker/final presentation and visual/client gates remain T038/T046/T049/T052. |
| T028–T031 | E06/E09: pure and actual causal composition, complete retained contributions, live bounded claim-release wake, guidance/frozen policy and exact explicit backend rebind/fault/no-clone tests. Client rebind remains T046. |
| T009, T033, T040, T047 | E03/E06/E08/E09/E11/E13/E16/E17: per-attempt ownership, exact known/unknown cleanup, actual group/node effects/Stop, before/after-known-GO ancestor control, stale ancestor guidance, live capacity release and no-replay recovery/fault evidence. IR006-42/43 are independently closed in witnessed scope; the unrelated actor fixture watchdog has separate E19 qualification and does not explain the original full-suite timeout. |
| T013, T036 | E07/E08/E10/E18: actual mutating command28.92s creates a fresh selected after-state/closure/final basis without qualifying its input or changing user/index/root/budget; actual correction36.19s executes selected Handoff with whole original, genuine finding/full failed-check receipt/final access, retains VALID output/history and spends one bounded unit without fabricated acceptance. Root independently closed both tests and fixed test-only helpers. |
| T035 | E07/E08/E10: actual source/access/byte/reference guards and combined prompt limits; provider token capacity remains unknown. |
| T041–T045, T048 | E10/E11/E12/E16: actual no-own-edit cumulative qualification/application/acceptance, all-required projection, nonexecuting recovered acknowledgement, response expiry, mixed/pure response and final-only external ancestry guards. Store metadata and owned filesystem/native evidence retain their distinct scopes. Root's prior T041–T044 checks were retained. |
| T050 | E20: extracted native archive on this host passed73.891s (total74.132s), including existing workflow qualification followed by real graph overlap, A+B→own-zero final with six native admissions, four acknowledged application effects, exact bindings/privacy/Start replay and retained-byte restart reads. Git-only PATH and allowlist/notices/checksums passed; this is not six-target qualification. |
| T051 | Five updated documents match current dev.6/006-v1 CLI/schema/capacity/budget/rebind/scope limits and preserve001–005 accepted/007–012 planned status; comprehensive foreign review remains T053. |
| T046, T052 | E13/E14/E20/E24/E25/E26/E38/E42/E46/E47/E48/E53 retain actual scoped implementation, client control/guidance/rebind/history/final-presentation evidence and the post-review paired capture with two independent 44-image reviews. Graph children use group-scoped controls and pending retries preserve captured intent. E48 records 88 PNGs, 72 keyboard observations, approved assets and source/dist/served UI identities, with explicit tall-image and Go-runtime identity limits. These checked implementation/capture scopes and the independently bounded T038/T049 client journeys do not replace T053–T057 feature acceptance. |

| T038, T049 | E13/E20/E24/E25/E48 and E56/E58 now retain the named actual browser, CLI and extracted-package journeys. Their scoped client checks are complete: read-only UI inspection, CLI before/after dispatch count, package before/after shared-budget count and current visual blocker/final scenes; exact controls/rebind/guidance/replay; actual browser-observed common-window expiry. No direct CLI expiry journey is claimed. E56 is a successful mixed-source command, not an exact-revision or native qualification. |

| Open task(s) | Concrete remaining work at this audit |
| --- | --- |
| T053–T057 | At this historical audit these gates were open. E60–E61 subsequently record completed review, exact-source build/test/check, committed assets and seven hosted jobs. T057 acceptance documentation must be committed before 007. E48 retains its tall-image and Go-runtime identity limits. |

## Phase 1: Setup

- [X] T001 Close all design findings and34 GRA/SC mappings in `specs/006-task-graphs/design-review.md`, `analysis.md` and `tasks.md`; commit complete specification/design/task artifacts before006 runtime edits, recording that immutable gate in `plan.md` and later `acceptance.md`.
- [X] T002 [P] Freeze actual accepted schema5 DDL, populated task/workflow/routing receipts and held-claim fixtures in `internal/store/graph_schema5_fixture_test.go` before modifying migrations; preserve exact original JSON/defaults/counters and foreign-key facts.
- [X] T003 [P] Implement strict graph/domain DTOs, explicit GraphBudgetRef, source/dispatch/output/final-verification bindings and validation in `internal/domain/graph.go`, `graph_validation.go` and `graph_validation_test.go`, covering24 requirements, presence/null/empty distinctions and declared16-node/64-edge/2MiB bounds.

## Phase 2: Shared Foundation

**Goal**: Establish durable scope, budget, capacity and ownership before parallel execution.

- [X] T004 [P] Implement closed staged-blob capture/hash/restore in `internal/workspace/graph_index.go` and `graph_index_test.go`; distinguish Git blob identity, raw index bytes, semantic entries and working content; record verified shared-object/worktree effects without touching user index/refs/files.
- [X] T005 Implement schema6 graph tables/indexes, two-slot claim migration and exact validation in `internal/store/graph_migration.go` and `graph_migration_test.go`; preserve held legacy barriers and old rows/receipts, reject unknown schema and distinguish pre-commit rollback from actual acknowledgement uncertainty.
- [X] T006 Implement canonical root budget resolution and graph-only amendment/read projection in `internal/store/graph_records.go` and `graph_grant.go`; extend `routing_admission.go`, `routing_budget.go`, `routing_state.go` and `routing_reads.go` so every native debit/launch/decision uses the actual root ledger without new child budgets or duplicate usage.
- [X] T007 Implement relevant graph/node control, budget-authority epoch/window and dispatch-proof validation in `internal/store/graph_authority.go` and `internal/domain/graph_bindings.go` and `graph_operations.go`; preserve standalone005 budget-revision semantics and sibling continuation across ordinary debits.
- [X] T008 Implement default two managed slots, native-context/account reservation limits and captured/current link checks in `internal/store/graph_capacity.go`, `attempts.go` and `execution.go`; migrate specific-claim settlement/recovery guards in `results.go` and test concurrent clients/relink/legacy barrier in `graph_capacity_test.go`.
- [X] T009 Replace singular active worker/cancel state with per-attempt ownership in `internal/engine/types.go`, `engine.go` and `control.go`; update existing strong control tests and add `graph_workers_test.go` for exact Stop, all-worker fence/shutdown, claim retention and no false empty pool when two are active.
- [X] T010 Implement immutable membership and scope enforcement in `internal/store/graph_records.go`, `tasks.go`, `workflow_stage_admission.go` and `workflow_routing.go`; guard ordinary root/node Run, retry,004A, adoption/guidance/grant routes internally rather than trusting omitted client graph fields.
- [X] T011 Implement graph/version/node/input/dispatch storage and bounded exact reads in `internal/store/graph_records.go`, `graph_lists.go` and `graph_response_read.go`; qualify signed64-bit/cursor/scalar-JSON/foreign-selector handling through `internal/domain/graph_boundary_test.go`, existing bounded-read helpers, `internal/store/graph_control_test.go` and `internal/engine/graph_members_test.go`, keeping filesystem callbacks outside SQL.
- [X] T012 Implement real zero-native-debit graph basis/input operations, fixed preparation ceilings and known/uncertain settlement in `internal/engine/graph_local.go` and `internal/store/graph_local.go`; use actual root-task attempts/artifact ownership, never fake native calls or authored candidates.
- [X] T013 Implement per-result GraphOutputObservation and append-once materialization associations in `internal/store/graph_output.go`, `graph_materialization.go`, `workflow_settlement.go` and `internal/engine/graph_output_facts.go`; capture candidate/eligible-response staged closure before settlement and install exact refs in the same terminal transaction, including mutating local checks.


- [X] T014 Implement deterministic graph condition/context packets with explicit authority/origin scopes, digest provenance and complete originals in `internal/engine/graph_context.go`, `graph_originals.go` and `internal/domain/graph_bindings.go`; retain no-model default and acyclic exact hashing. Bind generic receiver-scoped GraphConditions before every node workflow capture; shared instructions are an exact closed subset, never an implicit broadcast of unrelated node originals.
- [X] T015 Integrate actual supported inline/file access into native author/review/correction input in `internal/adapters/graph_context.go`, `internal/engine/workflow_context.go` and `workflow_native.go`, preserving literal pins,005 role restrictions and separately reported inspection.
- [X] T016 Implement one graph-root capture and independent initial-input retention in `internal/workspace/graph_root.go` and `graph_root_test.go`, retaining exact dirty files/index/staged content and immutable source identities before any node native admission.
- [X] T017 Implement real receiving materialization and exact source-basis receipts in `internal/workspace/graph_materialize.go`, `graph_handoff.go` and `internal/engine/graph_receiving.go`; seed only first authors/permitted same-input replacements and use005 selected-subject Handoff for later review/correction/check roles.
- [X] T018 Recheck actual managed inventory, semantic index/staged blobs, workspace/Git identity and fresh controls before Prepare/GO in `internal/workspace/graph_verify.go` and `internal/engine/workflow_native.go`; bind observations to the exact receiving step/attempt rather than latest selection or original-input reset.

**Checkpoint**: All native/local graph execution paths share actual scope and ownership. No
parallel model work is enabled through an unchecked alternate executor.

## Phase 3: User Story 1 — Independent Work (P1)

**Independent test**: Default unpinned Auto overlaps two eligible controlled native contexts;
one-slot/shared-account cases serialize, exact repeated Start creates no extra work or allowance.

- [X] T019 [P] [US1] Add fixed graph branch/response/failure/barrier scenarios in `internal/fixture/graph.go` and `graph_test.go`, preserving actual Codex/Claude wire shapes and bounded cancellation-aware test observations; no arbitrary user script or vendor executable.
- [X] T020 [US1] Implement atomic root/node creation and exact GraphAdmission receipts in `internal/store/graph_create.go`, `graph_admission.go`, `graph_budget_test.go` and `graph_admission_test.go`, with one root budget, real local preparation, stable node mapping and replay before changed prerequisites.
- [X] T021 [US1] Implement `CreateGraph`/current-owner progression and atomic target-selection/reservation/shared-debit integration in `internal/engine/graph_create.go`, `graph_progress.go`, `graph_continue.go`, `graph_admission_facts.go` and `internal/store/graph_admission.go`; unpinned second work sees the first reservation, while capacity waiting creates no native debit or004A substitution.
- [X] T022 [US1] Add server group/node control foundations in `internal/engine/graph_mutation.go`, `graph_advance.go` and `internal/store/graph_control.go`, including current controls before preparation/GO, basic Stop propagation and no ordinary-route scope bypass; final client/control qualification follows US4.
- [X] T023 [US1] Expose current graph start/list/detail/definition/budget/wait through `internal/app/graphs.go`, `internal/httpapi/graphs.go` and `internal/cli/graphs.go`; implement strict whole-file/stdin transport and exact request/proof handling from `contracts/operations.md`.
- [X] T024 [P] [US1] Add typed graph wire/view integration and exact current execution metadata in `ui/src/graph-types.ts`, `ui/src/api.ts`, `internal/engine/observe.go`, `internal/domain/types.go`, `execution.go` and `workflow.go`; extend known dev.5 maintenance Stop tests without granting old execution compatibility.
- [X] T025 [US1] Implement minimal separate graph editor/pending state, ordered list/overview and explicit shared-budget ownership in `ui/src/graphs.ts`, `workspace.ts`, `tasks.ts`, `routing.ts`, `index.html` and `style.css`; preserve standalone draft/Auto and approved assets.
- [X] T026 [US1] Prove actual default two-context overlap, one-slot/account serialization, simultaneous last-unit safety and sibling-debit continuation in `internal/engine/graph_parallel_test.go`, `internal/store/graph_budget_race_test.go` and `graph_stage_test.go`; use real owned workers and assert original project/index preservation.
- [X] T027 [US1] Run actual owner/CLI/browser independent-group/lost-start/read-race journeys in `ui/tests/graphs-cli.test.mjs`, `graphs-start.test.mjs`, `graphs-control.test.mjs` and `graphs-context.test.mjs`; retain exact node IDs/body across retry and preserve newer standalone/group text and selection.

## Phase 4: User Story 2 — Verified Dependencies (P1)

**Independent test**: A+B qualify, then C actually reads both retained results/index. Diamond
refinement works once; divergent/failed/stale/corrupt input blocks before receiver native preparation.

- [X] T028 [P] [US2] Implement pure per-working/index-slot causal fan-in in `internal/workspace/graph_compose.go` and `graph_compose_test.go`, including own-input-to-full-selected-output deltas, descendant refinement, identical inheritance, divergent heads and path/type/native-name conflicts.
- [X] T029 [US2] Implement accepted-contribution selection, complete working/staged/member closure and immutable input retention in `internal/engine/graph_contributions.go`, `internal/workspace/graph_inputs.go`, `graph_composed_input.go`, `internal/store/graph_input.go` and `graph_dependencies.go`, with typed verification outside SQL and current binding validation inside SQL.
- [X] T030 [US2] Implement bounded per-target fan-out consumption and current-owner live capacity waiting in `internal/store/graph_live.go`, `graph_admission_observations.go`, `graph_stage_wait.go` and `internal/engine/graph_continue.go`; allow real unrelated claim release to wake only live registrations, never reads/restart/history scans or a grant alone.
- [X] T031 [US2] Bind explicit node guidance/current versions and accepted contributor changes in `internal/store/graph_guidance.go`, `graph_rebind.go`, `graph_routing_basis.go`, `internal/engine/graph_workflow_capture.go` and `graph_advance.go`; deferred workflow capture requires sealed input/scoped conditions. Implement explicit Advance rebindNodeIds as derived dependency_rebind task versions with unchanged originals/pins/policy, retained old evidence and no refill; omission and ordinary Run never silently rebind or replay completed work.
- [X] T032 [US2] Add complete graph input/member/dispatch reads and actual node dependency UI/CLI inspection in `internal/httpapi/graphs.go`, `internal/cli/graphs.go`, `ui/src/graph-inputs.ts` and `ui/src/graphs.ts`; exact receiving attempt selects its own materialization, including nonqualifying states.
- [X] T033 [US2] Run actual dirty/staged/Unicode/CRLF dependent workspace, missing-object restoration, diamond refinement and divergent/conflicting/stale/corrupt counterexamples in `internal/engine/graph_fanin_test.go`, `graph_diamond_test.go`, `graph_input_composed_fault_test.go` and `internal/workspace/graph_materialize_test.go`; distinguish physical fixtures from pure tuples.
- [X] T034 [US2] Exercise real dependent/failure/provenance paging and stale selector journeys in `scripts/package-graphs.mjs`, `ui/tests/graphs-context.test.mjs`, `graphs-reads.test.mjs` and `graphs-cli.test.mjs`, preserving failed branch evidence and independent admitted siblings.

## Phase 5: User Story 3 — Focused Context (P2)

**Independent test**: Actual controlled workers open supplied originals and relevant evidence;
all applicable conditions survive, unrelated histories do not, and required access/size failures block.

- [X] T035 [US3] Enforce256KiB/64-reference and complete-original/access limits in `internal/workspace/graph_context.go`, `graph_context_supplement.go`, `internal/adapters/graph_context.go` and `internal/engine/graph_final_context.go`; report unknown token capacity honestly and reject unavailable/oversize required content before launch without silent clipping or model summarization.
- [X] T036 [US3] Verify actual prompt bytes, original file reads, long split evidence, raw checks, source scopes and missing/corrupt/access/overflow blocks in `internal/engine/graph_context_test.go`, `graph_native_run_test.go`, `graph_final_context_test.go`, `graph_command_mutation_test.go` and `internal/adapters/graph_context_test.go`; peer prose must not create account/budget/tool/publication authority.
- [X] T037 [US3] Implement derived-context/original-access views and exact full reads in `ui/src/graph-inputs.ts` and `internal/cli/graphs.go`; preserve loaded data/current selector on failed or late pages and keep provided access separate from model claims.
- [X] T038 [US3] Run actual UI/CLI context/digest/complete-original and capacity-blocker journeys in `ui/tests/graphs-context.test.mjs`, `graphs-cli.test.mjs` and `scripts/package-graphs.mjs`, checking unchanged native admission counts for observation/materialization/digests.

## Phase 6: User Story 4 — Control and Final Delivery (P2)

**Independent test**: Group/node Stop and exact mutations preserve real outcomes; final-node
cumulative delivery applies once while explicit response-only and pure-response outcomes stay distinct.

- [X] T039 [US4] Complete exact graph advance/Stop/nodeStop/guidance/grant bodies/results and old-receipt replay in `internal/domain/graph_operations.go`, `internal/store/graph_control.go`, `graph_guidance.go`, `graph_grant.go` and `internal/engine/graph_mutation.go`, matching `contracts/operations.md` rather than accounting/read revisions; include explicit rebindNodeIds/reboundNodes, optional root/node GraphRef and budget:null plus graphBudgetRef on every new graph-owned admission/decision projection, preserving actual node usage identity.
- [X] T040 [US4] Integrate graph controls through all node execution/preparation/GO/effect/settlement paths in `internal/engine/graph_receiving.go`, `graph_native_context.go`, `workflow_native.go`, `workflow_local.go` and `internal/store/graph_dispatch_controls.go`, `graph_authority.go`; preserve primary failures, accepted Stop and specific unknown-effect claim retention.
- [x] T041 [US4] Capture the real final candidate's GraphFinalVerificationBinding and fresh cumulative graph-root qualification basis in `internal/engine/graph_final_capture.go`, `internal/store/graph_final_output.go` and `internal/workflow/graph_final.go`; permit only the designated non-response-only final's proven cumulative no-own-delta case without changing standalone005 planner or creation facts.
- [x] T042 [US4] Supply actual graph-root-to-selected changes, scoped conditions and exact final-basis applicability to fresh review/check/direct proof in `internal/engine/graph_final_verification.go` and `workflow_context.go`; predecessor receipts and a summary response alone cannot qualify the combined files.
- [x] T043 [US4] Implement GraphDeliveryBinding around unchanged005 `PrepareApply`/actions in `internal/engine/graph_delivery.go`, `workflow_apply.go` and `internal/store/graph_delivery.go`; source is verified graph root, selected output is the actual node candidate, with graph/node/current-result guards at each durable single-effect permit.
- [X] T044 [US4] Enforce no intermediate local/external effects and explicit final-only publisher scope in `internal/engine/graph_delivery.go`, `graph_external.go`, `internal/store/graph_external_controls.go` and `graph_acceptance_response.go`; response-only mixed completion/blocked file delivery and pure-response completion must retain distinct outcomes.
- [X] T045 [US4] Implement all-required aggregate projection and no-replay restart handling in `internal/store/graph_projection.go`, `graph_phase.go`, `graph_live.go`, `incidents.go` and `internal/engine/graph_read.go`; retain current qualified response acknowledgement after window expiry as non-executing/no-refill work.
- [X] T046 [US4] Complete UI/CLI shared grants, exact proof capture, guidance, distinct Stop scopes/current-history controls and final-result presentation in `ui/src/graphs.ts`, `graph-inputs.ts`, `tasks.ts` and `internal/cli/graphs.go` and `graph_intents.go`; cached retries never refresh proofs silently; expose explicit stale-input rebind as a new version, and select graph controls for root/pending node GraphRef instead of standalone budget/adoption controls.
- [X] T047 [US4] Run actual group/node/ancestor Stop, guidance/capacity/fault, lost acknowledgement and restart counterexamples in `internal/engine/graph_control_live_test.go`, `graph_ancestor_live_test.go`, `graph_advance_scope_test.go`, `graph_native_run_test.go` and `internal/store/graph_resume_test.go`, `graph_stop_history_test.go`; include a stopped accepted ancestor with pending versus already dispatched followers under the frozen control rule.
- [X] T048 [US4] Prove actual no-own-edit final cumulative verification/application, dirty-index preservation, final response-only gap, pure-response acceptance and intermediate publisher zero effects in `internal/engine/graph_delivery_test.go`, `graph_final_acceptance_test.go`, `graph_direct_final_test.go`, `internal/store/graph_acceptance_response_test.go` and `internal/workspace/graph_delivery_test.go`; assert node creation flags remain zero while graph cumulative facts are nonzero.
- [X] T049 [US4] Exercise final delivery, shared allowance expiry/grants, node/group Stop and history through actual clients in `ui/tests/graphs-control.test.mjs`, `graphs-resume.test.mjs`, `graphs-rebind.test.mjs`, `graphs-cli.test.mjs` and `scripts/package-graphs.mjs`, including held replies against newer typing/node selection.

## Phase 7: Regression, Review and Acceptance

- [X] T050 Extend extracted native graph qualification in `scripts/package-graphs.mjs` and `scripts/package.test.mjs` for independent overlap, dependent exact inputs and final verified delivery; retain exact allowlist/notices/checksums and Git-only execution PATH without Node/npm/Go.
- [X] T051 Update actual supported behavior/limits, default separate results, shared allowance, final-only scope and parent traceability in `README.md`, `scripts/packagearchive/README.md`, `docs/refactor/traceability.md`, `delivery-plan.md` and `specs/006-task-graphs/quickstart.md`; preserve partial future adapter/paid API claims.
- [X] T052 Prepare/run ignored `.council/refactor/006-render-qa.mjs` and inspect all actual desktop1440px/mobile360px scenes listed in `contracts/interface.md`; retain exact scenario/source/asset identities, keyboard/focus, reduced motion, overflow and independent image review under `.council/refactor/screenshots/006-task-graphs/`.
- [X] T053 Obtain independent source/failure-path/interface review in `specs/006-task-graphs/review.md`, correct every blocking finding in owned implementation files, rerun meaningful affected checks/images and obtain foreign closure; no author self-approval.
- [X] T054 Run final serialized `npm run build`, `npm test` and `npm run check`; verify matching `internal/webui/dist/`, complete retained001–005/006 browser/CLI/package regressions and record exact terminal results in `specs/006-task-graphs/acceptance.md`. No timeout increase may mask a known hang or unresolved failure.
- [X] T055 Commit and push independently reviewed source and generated `internal/webui/dist/` to physical `refactor`, recording the exact checked revision in `specs/006-task-graphs/acceptance.md`; keep native binaries/private state/reference evidence ignored and main unchanged.
- [X] T056 Qualify that exact source through all six actual native Windows/macOS/Linux AMD64/ARM64 jobs plus supported Linux AMD64 race in `.github/workflows/verify.yml`; retain failures, correct/review/rerun and record seven terminal jobs in `specs/006-task-graphs/acceptance.md`.
- [X] T057 (conditional on committing this documentation-only follow-up before 007) Close all24 GRA/ten SC rows with actual source/case/review/native evidence in `specs/006-task-graphs/evidence-matrix.md`, `acceptance.md` and `review.md`; update parent requirements and commit acceptance before007. Completing006 does not complete the twelve-feature goal.

## Dependencies and Parallel Work

T001 is the immutable design gate. T002/T003 can then proceed independently; T004–T018 establish
shared invariants before graph worker dispatch. Domain/store and workspace/client files may be
authored in parallel only against frozen DTOs and with explicit ownership. Native/code tests use
Root's serialized queue after the affected source freezes.

US1 uses foundation and actual root input/materialization. US2 extends those sealed inputs with
qualified causal dependencies; US3 completes focused context on the same inputs. US4 consumes all
three for aggregate acceptance and final delivery. Stop/evidence/budget enforcement is foundational,
not deferred until the final story. T050–T057 apply across all stories and cannot be replaced by
their focused passes.

Parallel examples: after T002/T003, Core's store foundation and Native's T004 workspace primitive
use separate files and frozen domain contracts. After the foundation is complete, client/test
modules can be authored against frozen operation DTOs while unrelated implementation files are
owned elsewhere; integration waits for its real prerequisites. Root freezes the relevant dependency
packages before targeted verification. Actual heavy tests/build/browser runs never overlap locally.
A new source change after a passing gate requires matching output and affected requalification.

## Requirement Traceability

| Requirement | Primary executable tasks |
| --- | --- |
| GRA-001 | T003, T020–T027, T045–T049 |
| GRA-002 | T003, T020, T023, T025, T027 |
| GRA-003 | T004, T016–T018, T029, T033 |
| GRA-004 | T008–T009, T021, T026–T027, T030 |
| GRA-005 | T004, T006–T018, T020–T021, T028–T030, T035–T038 |
| GRA-006 | T006–T007, T010, T012–T013, T026, T039–T040, T047 |
| GRA-007 | T006, T011, T024–T027, T037–T038, T046 |
| GRA-008 | T017–T018, T029–T034, T039–T045, T048–T049 |
| GRA-009 | T003–T004, T014–T015, T017–T018, T028–T029, T032–T038, T044–T049 |
| GRA-010 | T011–T013, T017–T018, T029–T034, T041–T045 |
| GRA-011 | T004, T016–T018, T029, T033–T034, T036 |
| GRA-012 | T004, T017–T018, T028–T029, T033–T034 |
| GRA-013 | T017–T018, T030–T031, T041–T044, T048–T049 |
| GRA-014 | T014–T015, T035–T038, T041–T042 |
| GRA-015 | T004, T014–T015, T017–T018, T029, T035–T038 |
| GRA-016 | T003, T007, T010, T014–T015, T035–T040, T044 |
| GRA-017 | T008–T013, T030–T034, T039–T049 |
| GRA-018 | T009–T010, T018, T022, T030–T031, T039–T040, T046–T049 |
| GRA-019 | T007, T010, T030–T034, T039–T040, T045–T049 |
| GRA-020 | T003, T013, T017–T018, T029, T039–T049 |
| GRA-021 | T002, T005–T013, T017–T018, T020–T023, T029–T031, T039–T049 |
| GRA-022 | T011, T023–T027, T032–T034, T037–T038, T046–T049 |
| GRA-023 | T024–T025, T027, T032, T034, T037–T038, T046, T049, T052 |
| GRA-024 | T001–T057 |

| Success criterion | Primary actual evidence tasks |
| --- | --- |
| SC-001 | T008–T009, T021, T026–T027, T050 |
| SC-002 | T004, T016–T018, T029, T033–T034, T050 |
| SC-003 | T004, T017–T018, T028–T034, T036, T047 |
| SC-004 | T004, T017–T018, T028–T029, T033–T034, T041–T042 |
| SC-005 | T014–T015, T035–T038, T041–T042 |
| SC-006 | T006–T007, T010, T012–T013, T026, T038–T040, T047 |
| SC-007 | T009–T013, T020–T023, T030–T031, T039–T040, T045–T049 |
| SC-008 | T041–T050 |
| SC-009 | T023–T027, T032–T034, T037–T038, T046, T049, T052 |
| SC-010 | T001, T050–T057 |

## Implementation Strategy

The first useful increment is US1 with real bounded overlap and honest separate results. It is
not feature acceptance or the parent goal. Complete dependencies, context and controlled final
delivery, then the independent/local/native/record gates. No007 implementation begins while any
required006 capability or blocking finding remains open.



