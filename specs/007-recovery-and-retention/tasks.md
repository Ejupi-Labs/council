# Tasks: Durable pause and recovery

**Input**: `specs/007-recovery-and-retention/spec.md`, `plan.md`, `research.md`, `data-model.md`, and `contracts/pause-reconciliation.md`.
**Feature context**: `specs/007-recovery-and-retention/`; physical Git branch: `refactor`.
**Scope**: Complete and accept pause, checkpoints, explicit resume and evidence-based incident reconciliation. Retention, backup and cleanup remain the mandatory next feature in `docs/refactor/delivery-plan.md`.

## Phase 1: Setup and foundation

- [ ] T001 Verify feature context, requirements checklist, `.gitignore`, `go.mod`, `package.json` and branch `refactor`; preserve private state and user edits.
- [ ] T002 Define typed checkpoint, pause-scope, proof and reconciliation records and strict validation in `internal/domain/recovery.go`.
- [ ] T003 Add transactional schema-v7 migration, schema initializer and exact validation in `internal/store/store.go`, `internal/store/schema.go`, `internal/store/graph_migration.go` and `internal/store/recovery_migration.go`; retain accepted v1–v6 migrations.
- [ ] T004 Test new and old database migrations, validation and no partial migration on error in `internal/store/recovery_migration_test.go`.

## Phase 2: User Story 1 — Pause and resume

**Goal**: Establish a durable scoped admission boundary and explicitly resume unfinished eligible work after drain/restart without repeating completed work.
**Independent test**: Race controlled admissions and completions against task, member and group pause, restart, and resume; assert zero new admissions while paused, unchanged completed identities and exact receipt replay.

- [ ] T005 [US1] Add checkpoint and pause transaction contract tests, including member/group overlap, stale authority, lost acknowledgement and drain in `internal/store/recovery_pause_test.go`.
- [ ] T006 [US1] Implement request-receipted task, member and group pause/checkpoint records with scope/owner/control validation in `internal/store/recovery_pause.go`.
- [ ] T007 [US1] Enforce the persisted scope fence inside all task/workflow/graph admission and continuation transactions in `internal/store/tasks.go`, `internal/store/workflow_mutations.go`, `internal/store/workflow_stage_admission.go`, `internal/store/graph_admission.go`, `internal/store/graph_advance.go`, `internal/store/routing_continuation.go`, `internal/store/graph_local.go`, `internal/store/graph_create.go`, `internal/store/graph_resume.go`, `internal/store/workflow_delivery.go` and `internal/store/graph_delivery.go`; prevent new stage admissions and newly granted authority; permit already-admitted stages to settle under their original exact authority and Stop/failure controls, without authorizing subsequent stages or stale permissions.
- [ ] T008 [US1] Maintain checkpoint drain and outcome projection from committed settlements and retained claims in `internal/store/recovery_checkpoint.go` and relevant settlement paths.
- [ ] T009 [US1] Implement explicit resume with exact checkpoint and current authority, dependency, evidence and budget validation in `internal/store/recovery_resume.go` and `internal/engine/recovery_resume.go`.
- [ ] T010 [US1] Integrate owner-backed pause/checkpoint/resume with existing graph and workflow continuation behavior in `internal/engine/recovery_pause.go` and `internal/engine/graph_continue.go`.
- [ ] T011 [US1] Add controlled fake-worker tests for pause races, running drain, restart, no replay, Stop/failure and user-edit preservation, including pause versus routing continuation and delivery authorization, in `internal/engine/recovery_pause_test.go`.

## Phase 3: User Story 2 — Reconcile interrupted work

**Goal**: Inspect incidents and record proof-bound safe reconciliation without inferring process ownership or releasing unrelated blockers.
**Independent test**: Compare a controlled, fully evidenced process-tree cleanup and retained results with missing proof and another conflicting claim; only the exact safe claim can be released, and no call is started.

- [ ] T012 [US2] Add positive and negative reconciliation, incident replay, other-blocker and unknown-owner tests in `internal/store/recovery_reconcile_test.go` and `internal/engine/recovery_reconcile_test.go`.
- [ ] T013 [US2] Capture durable, exact-owner cleanup and evidence observations at the actual controlled process cleanup boundary in `internal/process/`, `internal/evidence/` and `internal/engine/`; do not rely on user assertions or reusable PIDs.
- [ ] T014 [US2] Implement append-only, revisioned reconciliation and exact claim/fence recomputation in `internal/store/incidents.go` and `internal/store/recovery_reconcile.go`; preserve unresolved incidents and terminal precedence.
- [ ] T015 [US2] Expose owner-mediated inspection and reconciliation without launch or blind replay in `internal/engine/recovery_reconcile.go`.
- [ ] T016 [US2] Verify deliberate replacement identities preserve and link predecessor failures, cancellation, changes and evidence in `internal/engine/recovery_replacement_test.go`; correct supporting code as needed.

## Phase 4: User Story 3 — Use either interface

**Goal**: Provide equivalent task, graph, checkpoint and incident operations through the local UI and CLI.
**Independent test**: Pause from one interface, inspect and resume from the other, retry a lost acknowledgement, and verify keyboard and stale-response behavior using real rendered UI and fake agents.

- [ ] T017 [US3] Route strict owner-mediated checkpoint, pause, resume and incident operations through `internal/app/`, `internal/httpapi/` and associated contract tests.
- [ ] T018 [US3] Implement CLI controls and inspection with exact request identities and structured outcomes in `internal/cli/` and `internal/cli/recovery_test.go`.
- [ ] T019 [US3] Implement accessible branded controls, checkpoint and incident views in `ui/src/`, preserving pending requests, current selection and drafts on stale or failed responses.
- [ ] T020 [US3] Add focused rendered UI and CLI parity tests covering retry, narrow viewport, focus and actual control status in `ui/tests/recovery.test.mjs` and `internal/cli/recovery_test.go`.
- [ ] T021 [US3] Document pause, recovery, limits and no-replay behavior in `README.md` and `docs/`, distinguishing supported fake qualification from live providers.

## Phase 5: Acceptance

- [ ] T022 Obtain an independent review of source, contracts, credential boundaries and crash/race handling; correct all blocking findings in the affected files.
- [ ] T023 Run required offline `npm run build`, `npm test` and `npm run check`, plus focused recovery and actual UI acceptance; commit generated `internal/webui/dist/` with authored UI changes.
- [ ] T024 Record requirements-to-evidence traceability, exact test results, independent review and remaining limitations in `specs/007-recovery-and-retention/acceptance.md`.
- [ ] T025 Commit and push the reviewed implementation on `refactor`, qualify supported native platforms and the race job with exact-source CI, then record actual outcomes in `specs/007-recovery-and-retention/acceptance.md` and commit and push that documentation-only acceptance before activating the mandatory retention follow-up.

## Dependencies and parallel ownership

T001–T004 establish the shared migration and contracts. T005–T011 implement scoped pause; T012–T016 implement reconciliation on the same schema and follow the foundation. T017–T021 integrate the stable backend contracts; implementation affecting the same files is serialized. T022–T025 close this feature before the mandatory retention follow-up. Independent review and UI/CLI work can be delegated to different owners only after agreeing on exact file ownership. No live vendor calls are authorized by these tests.
