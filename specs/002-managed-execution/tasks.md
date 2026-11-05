# Tasks: Managed native Codex execution

**Branch**: `refactor` | **Input**: spec.md, plan.md, research.md, data-model.md, contracts/execution.md

## Phase 1 — Setup and shared interfaces

- [ ] T001 Define execution DTOs, exact counters, limits, errors and immutable-version/request validation in internal/domain/execution.go and execution_validation.go; freeze engine/process/workspace/evidence interfaces in their package types.go files before parallel implementation.
- [ ] T002 Update preview2.0.0-dev.2 and capability002-v1 metadata in internal/domain/types.go, package.json, package-lock.json and scripts/package.mjs; keep package README/tests and notices accurate without altering existing record identities.
- [ ] T003 Write schema1→2 migration and receipt-preservation tests in internal/store/execution_schema_test.go, including rollback, unknown schema and exact integer boundaries.
- [ ] T004 Implement transactional execution tables/indexes/migration in internal/store/execution_schema.go and integrate internal/store/store.go without resetting001 data or changing receipt behavior.

## Phase 2 — Foundations required before dispatch

- [ ] T005 [P] Write native process ownership/descendant/drain/crash tests in internal/process/process_test.go and platform-specific test files, with real local child fixtures and no vendor calls.
- [ ] T006 Implement the bounded owned-process interface and Windows creation-time job assignment in internal/process/process.go and process_windows.go, including strict stdio handle inheritance, atomic job membership, root observation and active-process accounting.
- [ ] T007 Implement Unix guardian READY/GO/control-EOF/group cleanup in internal/process/process_unix.go and guardian_unix.go; final signals precede Wait/reap, post-reap probes never signal, known escapes retain uncertainty.
- [ ] T008 [P] Write rooted artifact/stream durability, exact byte-read, corruption and failed-flush tests in internal/evidence/evidence_test.go using temporary private state.
- [ ] T009 Implement private rooted artifact creation, durable prefixes, bounded collectors, immutable hashes and exact base64 chunk reads in internal/evidence/files.go and streams.go; coalesce checkpoints and retain incomplete bytes honestly.
- [ ] T010 [P] Write source/index/worktree/filter/hook/racing-source tests in internal/workspace/workspace_test.go with distinct staged/unstaged bytes, ordinary untracked files and unsupported index/source states.
- [ ] T011 Implement current-source inventory/materialization and meaningful staged-index reconstruction in internal/workspace/capture.go and git.go, using os.Root, sanitized shell-free Git, no checkout/filters and immutable snapshot manifests.
- [ ] T012 Implement managed candidate capture and exact delta against the captured current source/index in internal/workspace/candidate.go; retain partial candidates and preserve original source/index/branch.
- [ ] T013 [P] Write strict Codex JSONL terminal/error/unknown-fact/limit tests in internal/adapters/codex_test.go using versioned local transcripts and malformed/late/truncated cases.
- [ ] T014 Implement native Codex argv/launcher/capability/prompt construction and JSONL interpretation in internal/adapters/codex.go and resolver.go, with exact pins, full input, no credentials/config scan or automatic fallback.
- [ ] T015 Implement explicit fixed fake scenarios/internal fixture entrypoint in internal/fixture/fixture.go and cmd/council/main.go; fake/native share real process ownership and never disguise a fixture as a provider result.
- [ ] T016 Write and implement persistent-owner bootstrap, two-client contention and observer-disconnect tests/service handling in internal/owner/owner_test.go, service.go and platform startup files; reuse native lock/private descriptor, no PID takeover or task/token argv.

Checkpoint: native ownership, source fidelity, artifact durability and owner/bootstrap contracts pass
before any feature path can dispatch a worker. Parallel foundation packages have exclusive owners.

## Phase 3 — US1: Start authorized work with minimal configuration (P1)

Independent acceptance: UI/CLI admit one exact task/attempt using project/text and captured policy;
pins/limits/prerequisites are honest, identical retries never start another worker.

- [ ] T017 [US1] Write task/version/admission/replay/policy/source-draft tests in internal/store/tasks_test.go and internal/engine/admission_test.go, including changed global policy after a lost acknowledgement.
- [ ] T018 [US1] Implement task/version/attempt admission and explicit new-attempt transactions in internal/store/tasks.go and attempts.go; preserve prior terminal records and separate user revision from asynchronous state revision.
- [ ] T019 [US1] Implement engine admission, single connection slot, scheduler/deadline and prelaunch checks in internal/engine/engine.go and scheduler.go; durable launch intent precedes process instructions and no external I/O holds a SQL transaction.
- [ ] T020 [US1] Wire task creation/guidance/manual-start, meta/capability and strict HTTP inputs in internal/app/execution.go and internal/httpapi/execution.go, preserving foundation API/old receipt replay and bounded errors.
- [ ] T021 [US1] Add task start/list/get/version/guidance/run CLI commands and persistent-owner selection in internal/cli/execution.go and cli.go; validate optional flags/UTF-8/policy revisions without an interactive form.
- [ ] T022 [US1] Add primary Start/native Auto/captured-policy and optional task details in ui/src/tasks.ts, workspace.ts and drafts.ts, preserving clicked draft autosave/start intents and newer typing with separate retry identities.
- [ ] T023 [US1] Add real-browser/CLI direct and exact-draft/autosave/policy-conflict/lost-start-response acceptance in ui/tests/execution-start.test.mjs and internal/cli/execution_test.go; assert exactly one worker invocation and zero model observation calls.

## Phase 4 — US2: Inspect useful output without losing existing work (P1)

Independent acceptance: full response/candidate and original source/index remain attributable;
successful authoring is verification pending, partial failure stays failed, unknown facts stay unknown.

- [ ] T024 [US2] Write append-once terminal, usable-result, source-relative delta and response-acceptance eligibility/hash tests in internal/engine/results_test.go and internal/store/results_test.go.
- [ ] T025 [US2] Implement durable event/artifact/source/candidate records and bounded exact paging in internal/store/events.go and artifacts.go, retaining full source/output references instead of transcript-sized summaries.
- [ ] T026 [US2] Implement result/evidence settlement and immutable terminal/acceptance transactions in internal/engine/results.go and internal/store/results.go; require native terminal/root exit, durable usable output and verified scoped cleanup.
- [ ] T027 [US2] Expose attempt/task detail/history/events/artifacts/chunk/response-acceptance HTTP and CLI operations in internal/httpapi/execution.go and internal/cli/execution.go with exact IDs, version/hash guards and truthful observation semantics.
- [ ] T028 [US2] Implement task timeline/full byte-backed output/candidate/usage and optional response acceptance in ui/src/tasks.ts, with current-version/read generations, streaming UTF-8 decoding and no prose-as-check or stale acceptance.
- [ ] T029 [US2] Add browser/CLI exact output/chunk boundaries/partial failure/candidate verification-pending/response acceptance tests in ui/tests/execution-result.test.mjs; compare original staged/unstaged/untracked state unchanged.

## Phase 5 — US3: Stop work and see whether it settled (P1)

Independent acceptance: accepted Stop prevents queued dispatch, failure/cancellation survives late
success, and within15seconds cleanup is verified or explicitly blocked with capacity held.

- [ ] T030 [US3] Write deterministic accepted-control/terminal/deadline race tests in internal/engine/control_test.go and internal/store/control_test.go, including failure-before-Stop and Stop-before-late-success order.
- [ ] T031 [US3] Implement durable Stop/service-shutdown admission gates and precedence in internal/engine/control.go and internal/store/control.go; target exact attempt/owner identities and retain idempotent no-op receipts.
- [ ] T032 [US3] Integrate process termination, bounded concurrent drains, scoped cleanup receipts and unknown-cleanup fences in internal/engine/settlement.go; keep the slot until proof and never use stored PID guesses.
- [ ] T033 [US3] Expose Stop/wait/service-stop through internal/httpapi/execution.go and internal/cli/execution.go; observer timeout/Ctrl+C/disconnect never cancels work, owner shutdown does bounded settlement.
- [ ] T034 [US3] Add visible accessible Stop/cleanup/primary failure/deadline feedback in ui/src/tasks.ts and style.css without erasing original output or presenting acknowledgement as termination.
- [ ] T035 [US3] Add real-browser/CLI queued/active/descendant/late-success/unknown-cleanup/observer-timeout acceptance in ui/tests/execution-control.test.mjs and internal/cli/execution_test.go using only owned local fixtures.

## Phase 6 — US4: Coordinate and recover retained work (P2)

Independent acceptance: two independent clients share one owner/slot; lost replies and restart retain
the same facts and original evidence, with zero automatic replay and explicit uncertainty fences.

- [ ] T036 [US4] Write two-client start/slot contention, guidance-change-before-dispatch and clean/unclean owner restart tests in internal/engine/recovery_test.go and internal/owner/owner_test.go.
- [ ] T037 [US4] Implement recovery/incident/gate/claim persistence in internal/store/incidents.go and internal/engine/recovery.go, preserving terminals and marking unfinished work interrupted without redispatch.
- [ ] T038 [US4] Inject storage/commit/stream/flush/owner-loss faults in internal/engine/fault_test.go; retain known partial evidence and prevent conflicting launch rather than reconstructing output with a model call.
- [ ] T039 [US4] Implement bounded wait/change notification and stable metadata snapshots in internal/engine/observe.go and internal/store/attempts.go; reads/polls make zero model calls and never mutate user task versions.
- [ ] T040 [US4] Add incident/restart/current-version/history/remaining-action UI in ui/src/tasks.ts plus stale task/status/composer guards and retained pending operation recovery.
- [ ] T041 [US4] Add end-to-end CLI-background-owner continuation, second-client shared capacity, restart/evidence/no-replay and stale UI tests in ui/tests/execution-recovery.test.mjs and internal/cli/execution_test.go.

## Phase 7 — Review and qualification

- [ ] T042 Verify schema1 data/receipt upgrade, existing foundation commands/browser journeys, strict authority/JSON/large counters and private-file rules in internal/store/execution_schema_test.go, internal/httpapi/execution_test.go and existing foundation tests.
- [ ] T043 Verify actual native Windows AMD64/ARM64 job creation/handle/descendant/crash and Linux/macOS guardian/PGID/no-post-reap-signal cases through .github/workflows/verify.yml; distinguish runtime from compile evidence and declare process scope limits.
- [ ] T044 Extend extracted native package/empty-development-PATH checks with explicit fake execution and retained evidence in scripts/package.test.mjs, update scripts/packagearchive/README.md/version expectations, and confirm no state/workspaces/credentials enter archives.
- [ ] T045 Inspect actual desktop/360px execution/Stop/result/error renders, keyboard/focus/reduced motion and approved asset hashes; record screenshots/findings in specs/002-managed-execution/acceptance.md.
- [ ] T046 Run npm run build, npm test and npm run check; exercise quickstart.md and record exact native/SQLite/browser versions, zero-live-call scope and actual commands/results in specs/002-managed-execution/acceptance.md.
- [ ] T047 Obtain independent source/contract/failure-path review in specs/002-managed-execution/review.md, correct blocking findings and recheck the affected evidence before acceptance.
- [ ] T048 Complete EXE-001–029 and SC-001–009 evidence mapping in specs/002-managed-execution/acceptance.md; update docs/refactor/traceability.md and README.md honestly, mark tasks from observed evidence and commit source/embedded assets before003.

## Dependencies and parallel ownership

T001 freezes shared types/interfaces, then T002–004 establish compatible persistence. T005→T006–007,
T008→T009, T010→T011–012 and T013→T014 may run in parallel on disjoint packages. T015/016 integrate
their stable interfaces. No scheduling dispatch is enabled until that foundation checkpoint passes.
US1 precedes US2, US3 and US4 integration checkpoints; implementation within each uses tests before
the corresponding core behavior. Store/engine/app route files have one explicit owner at a time.
Native process, workspace/evidence and UI work may be delegated while the root integrates engine,
store and command wiring. Root owns task status, manifests and delivery records.

Examples: native process owner works T005–007 while workspace owner works T010–012; evidence owner
can work T008–009 after DTO freeze. UI Start/details can use the frozen API while backend story
services are implemented. Actual cross-interface tests follow both sides. Terminal/cleanup tests
must pass before declaring any workflow complete, regardless of parallel task success.

## Requirement coverage

| Requirements | Tasks |
| --- | --- |
| EXE-001–005 | T001, T017–023, T036–041 |
| EXE-006–009 | T013–015, T017–023, T042–043 |
| EXE-010–011 | T010–012, T024–029 |
| EXE-012–016 | T008–009, T024–029, T032–035 |
| EXE-017–018 | T003–004, T016–021, T030–031, T036–041 |
| EXE-019–024 | T005–009, T019, T030–038, T043 |
| EXE-025–026 | T016, T033, T036–041 |
| EXE-027–029 | T015, T022–023, T028–029, T034–035, T040–048 |
| SC-001–009 | Story checkpoints and T042–048 |

## Delivery strategy

The first runnable increment is one fake/native-contract authoring attempt with honest evidence,
not a completed production task. Finish all four stories and correction/qualification gates before
003. All48tasks are unchecked until their actual evidence exists. Use Specify analyze read-only
for the artifact gate, then separately apply authorized corrections and commit design before code.
No before/after Specify extension hooks are configured. No live provider call is authorized.
