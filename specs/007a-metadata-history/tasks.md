# Tasks: Project Metadata History

**Feature context**: `specs/007a-metadata-history/`; physical Git branch: `refactor`.
**Input**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/operations.md` and `quickstart.md` in this directory.
**Scope**: Project-scoped, incomplete, non-authoritative Council-generated metadata snapshot, local inspection and explicit private-root export. No archive intake, opaque evidence, backup/restore, reviewed cleanup, model calls or live-provider qualification. Full retention remains mandatory before feature 008.

## Phase 1: Setup and design gates

- [X] T001 Verify active Specify context, `refactor`, feature checklist, `.specify/memory/constitution.md` and excluded private state against `specs/007a-metadata-history/spec.md` and `specs/007a-metadata-history/plan.md` without opening runtime user data or vendor credentials.
- [X] T002 Review and reconcile exact field provenance, 2,048-task/8,192-attempt/4 MiB bounds, fixed omissions, local ordinals, native no-follow create and single-permit/direct-output semantics across `specs/007a-metadata-history/research.md`, `specs/007a-metadata-history/data-model.md` and `specs/007a-metadata-history/contracts/operations.md`; independently resolve blocking design discrepancies before product implementation.

## Phase 2: Shared foundation

- [X] T003 Define bounded typed snapshot, project/task/attempt rows, preview, operation, receipt, journal and reconciliation inputs/results with strict validation and lossless decimal handling in `internal/domain/history.go` and `internal/domain/history_test.go`.
- [X] T004 Add the next schema version, exact fresh/current schema validation and v7-to-current migration for immutable snapshots, exact receipts, previews, operations and append-only effect observations in `internal/store/history_migration.go`, `internal/store/store.go` and `internal/store/schema.go`; preserve every older upgrade chain without decoding opaque payloads.
- [X] T005 Add synthetic v7 upgrade, fresh/reopen, rollback, stale/unknown schema and strict DDL tests in `internal/store/history_migration_test.go`; update only necessary schema-version fixtures in `internal/store/` and `internal/cli/` without touching user state.
- [X] T006 Define lifecycle/owner integration for registered history actors, held root ownership, dirty/clean generation checks and no unregistered effect-capable work in `internal/engine/types.go`, `internal/engine/engine.go` and `internal/store/recovery_owner_epoch.go`; serialize edits with T004–T005.
- [X] T007 Verify operation admission/drain and owner epoch transition behavior, including actor timeout, failed fence persistence and dirty generation, in `internal/engine/history_drain_test.go`, `internal/engine/recovery_owner_epoch_test.go` and `internal/store/history_owner_epoch_test.go`.

## Phase 3: User Story 1 — Capture and inspect incomplete history (P1)

**Goal**: An ordinary owner-managed populated project yields an immutable, scoped, explicitly incomplete metadata inventory without accessing excluded content or invoking a model.

**Independent test**: Capture one of two populated fake-agent projects, inspect the same snapshot through UI and CLI, and verify all eligible selected relationships exactly once, no other-project records, fixed omissions and no application-level excluded-content access.

- [X] T008 [P] [US1] Add real-store tests for two-project isolation, trusted write-path provenance, dedicated-column projection sentinels, empty/unknown source, malformed/cross-project relations, 64-bit values, bounds and coherent concurrent capture in `internal/store/history_capture_test.go` and `internal/store/history_capture_boundary_test.go`.
- [X] T009 [US1] Implement scoped explicit-column, one-transaction bounded capture and immutable snapshot persistence with exact request replay, validated relationships, safe omission codes and project-local ordering in `internal/store/history_snapshot.go`.
- [X] T010 [US1] Implement stable snapshot summary/section paging with exact snapshot-bound cursors and no fresh-source requery in `internal/store/history_read.go` and `internal/store/history_capture_test.go`.
- [X] T011 [US1] Add owner-backed capture and read operations with lifecycle guard and zero model calls in `internal/engine/history.go` and `internal/engine/history_integration_test.go`.
- [X] T012 [US1] Route strict snapshot capture, inspect and page contracts through `internal/app/history.go`, `internal/httpapi/server.go` and their `history_test.go` files, preserving existing owner authentication and response envelope.
- [X] T013 [US1] Implement `history capture`, `history show` and `history records` with exact IDs, bounded cursor/limits and structured output in `internal/cli/history.go` and `internal/cli/history_test.go`.
- [X] T014 [US1] Implement branded project capture/inspection, incomplete labels, fixed exclusions and stable selection/draft handling in `ui/src/history.ts`, `ui/src/types.ts` and `ui/src/workspace.ts`; commit generated `internal/webui/dist/` in T036.
- [X] T015 [US1] Exercise actual rendered UI and CLI against the same fake owner, including two-project isolation, zero model calls and 360-pixel keyboard/focus behavior in `ui/tests/history-capture.test.mjs` using the real owner and CLI.

## Phase 4: User Story 2 — Preview and export the exact snapshot (P1)

**Goal**: Publish exactly one bounded safe metadata document to the user's selected absent filename in the verified private root, with no replacement or source payload access.

**Independent test**: Preview, publish and independently compare controlled output to the immutable snapshot, then replay a lost acknowledgement and verify one output, explicit omissions and no unrelated changes.

- [X] T016 [P] [US2] Add canonical encoding/digest, fixed omissions, local ordinals, 4 MiB/row bounds, filename/device-name and unsafe input tests in `internal/domain/history_export_test.go` and `internal/domain/history_test.go`. The 8,192-attempt test produces less than 4 MiB and rejects 8,193 attempts; the separate 4 MiB over-limit rejection branch has not been directly exercised.
- [X] T017 [US2] Implement deterministic exact output encoding and digest solely from eligible snapshot fields, excluding filename, paths, request IDs and opaque data in `internal/domain/history_export.go`.
- [X] T018 [P] [US2] Add controlled native private-root and exclusive final-file creation tests for existing names, case aliases, symlink/reparse/mount and parent identity changes, partial write and unsupported sync guarantees in `internal/historyout/publish_test.go`, `internal/historyout/publish_links_test.go` and `internal/historyout/publish_mount_linux_test.go`. The isolated Linux mount test is platform-dependent; hosted Linux runners skipped it because the required namespace was denied.
- [X] T019 [US2] Implement verified private root and anchored no-follow exclusive final-name creation with retained-handle writes and native platform branches in `internal/historyout/publish.go`, `internal/historyout/publish_windows.go`, `internal/historyout/publish_unix.go`, `internal/historyout/publish_linux.go`, `internal/historyout/publish_darwin.go` and `internal/historyout/publish_unsupported.go`; fail closed on unsupported targets, disclose full path, and do not stage a second file or rename.
- [X] T020 [US2] Implement exact preview, immutable snapshot/destination binding, target conflict and current authority checks in `internal/store/history_export.go` and `internal/store/history_export_test.go`.
- [X] T021 [US2] Implement exact export request/receipt and one irreversible publication permit before any output create; journal create/write/sync observations and retain partial/unknown objects without overwrite, removal or replay in `internal/store/history_export.go` and `internal/store/history_export_test.go`.
- [X] T022 [US2] Coordinate owner-scoped export from T020–T021 with the registered T006 lifecycle and T019 retained handle in `internal/engine/history_export.go` and `internal/engine/history_integration_test.go`; never call a helper or background publisher outside the journal.
- [X] T023 [US2] Add strict preview/execute/operation-inspection routes and versioned owner capability checks in `internal/app/history.go`, `internal/httpapi/server.go`, `internal/engine/observe.go` and their history tests; preserve older recognized Stop-only owners.
- [X] T024 [US2] Add `history export-preview`, `history export` and `history operation` with privately retained exact retry intent and correct owner compatibility in `internal/cli/history.go`, `internal/cli/owner.go`, `internal/cli/metadata.go` and `internal/cli/history_test.go`.
- [X] T025 [US2] Add export preview, disclosed full private output path, incomplete warning, busy/error/partial states and exact pending-request retry in `ui/src/history.ts` and `ui/src/types.ts`; preserve current selection and do not embed paths in the exported payload.
- [X] T026 [US2] Verify actual UI-to-CLI and CLI-to-UI preview/export/replay against a real fake owner, deterministic contents, visible partial output and no Git/release publication in `ui/tests/history-export.test.mjs`, `ui/tests/history-cross-interface.test.mjs`, `ui/tests/history-partial.test.mjs` and `internal/engine/history_crash_test.go`.

## Phase 5: User Story 3 — Reconcile interruption without replay (P2)

**Goal**: Preserve exact operation receipts and safe, inspectable outcomes across owner loss, without repeating ambiguous publication or losing newer UI input.

**Independent test**: Lose a reply, retry through the other interface, interrupt at permit and file boundaries, and verify one output or unresolved fence; independently demonstrate the bounded positive no-publication proof.

- [X] T027 [P] [US3] Add journal and owner tests for pre-/post-commit ACK loss, freeze-versus-permit race in both orders, delayed actor, dirty/broken clean chain, missing/corrupt journal and independent blocker in `internal/store/history_reconcile_test.go`, `internal/store/history_reconcile_race_test.go`, `internal/store/history_reconcile_receipt_test.go`, `internal/store/history_chain_boundary_test.go`, `internal/engine/history_reconcile_actor_test.go` and `internal/engine/history_drain_test.go`.
- [X] T028 [US3] Implement atomic operation freeze, validated append-only no-permit proof, same-owner and complete historical clean-chain checks, exact receipt replay and narrow blocker release in `internal/store/history_reconcile.go`; any permit or unproven owner remains unresolved.
- [X] T029 [US3] Implement owner-mediated registered-actor admission barrier, drain and reconciliation with existing unrecorded-fence protection in `internal/engine/history_export.go`, `internal/engine/history_fence.go` and `internal/engine/history_integration_test.go`; extend `internal/engine/engine.go` and `internal/store/recovery_owner_epoch.go` only under the same owner as T006.
- [X] T030 [US3] Verify controlled owner crash before/after permit, exclusive create, partial write, sync and receipt commit, including real process restart and no automatic deletion or replay in `internal/engine/history_crash_test.go` and `internal/historyout/publish_test.go`.
- [X] T031 [US3] Route strict revision-bound reconcile action and inspectable unresolved/decision responses through `internal/app/history.go`, `internal/httpapi/server.go` and their history tests; keep independent blockers fenced.
- [X] T032 [US3] Implement exact-request CLI reconciliation and actionable status in `internal/cli/history.go` and `internal/cli/history_test.go`; never accept caller-supplied filesystem or owner proof.
- [X] T033 [US3] Add pending operation, status and reconciliation controls that preserve current project/draft and support keyboard navigation in `ui/src/history.ts`; verify cross-interface retry, changed-request conflict and delayed-response isolation in `ui/tests/history-recovery.test.mjs`, `ui/tests/history-cross-interface.test.mjs` and `ui/tests/history-delayed.test.mjs`.

## Phase 6: Review, qualification and delivery

- [X] T034 Document operation scope, private-root path disclosure, partial-file behavior, unknown ownership, no arbitrary import and deferred complete retention in `docs/refactor/metadata-history-guide.md` and `README.md`.
- [X] T035 Obtain independent review of source and `specs/007a-metadata-history/` against MH-001–015/SC-001–006, specifically field provenance, project isolation, native root/create safety, lifecycle, journal/no-effect proof, UI/CLI and credential boundary; correct and rerun all blocking findings in their owned files.
- [X] T036 Run `npm run build`, complete `npm test` and `npm run check`, plus appropriate focused owner/Store/native and actual rendered UI tests; commit generated `internal/webui/dist/` with authored sources and record commands/results in `specs/007a-metadata-history/acceptance.md`.
- [X] T037 Record requirements-to-test traceability, actual platform limitations, independent-review disposition, user-data/credential boundaries, historical failures and unperformed live-provider scope in `specs/007a-metadata-history/acceptance.md`; do not claim full evidence backup, restore, cleanup or release completion.
- [X] T038 Commit and push the reviewed exact product/test source on `refactor`, obtain actual passing six native plus Linux race hosted results, then commit and push the documentation-only acceptance follow-up in `specs/007a-metadata-history/acceptance.md` before activating the next mandatory retention slice.

## Dependencies and explicit file ownership

T001–T002 finish the design gate; T003–T007 establish shared types, store schema and owner lifecycle before feature work. US1 (T008–T015) is the independently testable MVP. US2 (T016–T026) depends on the immutable snapshot and receipt foundation. US3 (T027–T033) depends on the actual journal, publisher and owner lifecycle; it extends rather than substitutes for T021–T022. T034–T038 close the single feature before further retention implementation.

Parallel work is allowed only when assigned exclusive files. After T003–T007 are frozen, T008, T016 and T018 may be prepared in parallel as separate new test files; T009–T015 must follow their actual prerequisites. Assign one Store migration/schema owner to T004–T005, one owner/lifecycle maintainer to T006–T007/T029, and one native publisher owner to T018–T019/T030. The app/HTTP, CLI and UI owners each serialize their own story edits to `internal/app/history.go` and `internal/httpapi/server.go`, `internal/cli/history.go`, and `ui/src/history.ts` respectively; T012/T023/T031, T013/T024/T032 and T014/T025/T033 must not edit the same files concurrently. Shared test-file additions and `internal/webui/dist/` are similarly serialized. A parent coordinator schedules one test/build slot at a time; no live provider, real credential or private user state is inspected.

Complete each story's independent test before treating it as delivered. After local review and fixes, qualify the exact source and preserve historical failures honestly; a docs-only acceptance commit cannot stand in for successful product-source CI. The remaining full evidence export, useful production backup/verified restore and reviewed cleanup stay mandatory and are not satisfied by this MVP.
