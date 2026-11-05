# Tasks: Workspace foundation

**Branch**: `refactor` | **Input**: spec.md, plan.md, research.md, data-model.md, contracts/foundation.md

## Phase 1 — Setup

- [ ] T001 Preserve reference27822ba and remove only explicitly tracked obsolete application/test/plugin artifacts; retain all ignored state and new design tooling, recording exact scope in specs/001-workspace-foundation/replacement.md.
- [ ] T002 Initialize go.mod/go.sum, package.json/package-lock.json, ui/tsconfig.json, scripts/build.mjs and meaningful npm test/check wrappers for the selected pinned toolchains; update AGENTS.md stack rules and ignore rules without hiding embedded browser assets.
- [ ] T003 [P] Copy only approved public brand artwork and selected MIT icons into ui/src/assets/, retaining exact hashes and notices in third-party/brand-assets.md and third-party/ejupilabs-icons/LICENSE.

## Phase 2 — Shared foundation

- [ ] T004 Define contract DTOs, bounded English errors, opaque identities, strict validation and exact revision parsing in internal/app/types.go and internal/app/validation.go; add Unicode/size/revision boundary tests in internal/app/validation_test.go.
- [ ] T005 Write failing native lock/privacy/descriptor subprocess tests in internal/localstate/localstate_test.go, including existing-owner exclusion and stale-descriptor recovery without lock deletion.
- [ ] T006 Implement owned v2 root, native locks, Windows protected DACL/Unix modes and atomic private descriptors in internal/localstate/state.go, state_windows.go and state_unix.go; never inspect legacy/vendor data.
- [ ] T007 Write transaction/replay/schema/overflow/cursor failure tests in internal/store/store_test.go with deterministic fault injection and temporary databases.
- [ ] T008 Implement serialized SQLite lifetime/schema, checked int64 metadata, exact bounded cursors and atomic idempotency receipts in internal/store/store.go and internal/store/schema.go; no external I/O in transactions.
- [ ] T009 [P] Write strict JSON/authority/body/query/method contract tests in internal/httpapi/server_test.go, including private GET, Host/Origin, escaped surrogates, unknown/duplicate fields and bounds.
- [ ] T010 Implement loopback transport, strict JSON envelopes, authentication/Host/Origin, fixed assets/security headers and safe errors in internal/httpapi/server.go, using service interfaces until story operations exist.
- [ ] T011 Implement shell-free CLI entrypoint/parser, API version/help/errors, owner forwarding/transient mode and foreground UI lifecycle in cmd/council/main.go and internal/cli/cli.go; add parser/owner-forwarding tests in internal/cli/cli_test.go.

Checkpoint: setup and shared authority/persistence tests pass before story implementations.

## Phase 3 — US1: Open a project and retain operating preference (P1)

Independent acceptance: CLI/UI reopen the same physical project and saved policy after restart;
Unicode paths/subdirectories remain correct and existing source/index/branch bytes unchanged.

- [ ] T012 [P] [US1] Write physical alias/replacement/subdirectory/unborn/detached/Git-hook/index sentinel tests in internal/project/project_test.go, using only disposable Git fixtures.
- [ ] T013 [US1] Implement lossless path resolution, platform physical identity and shell-free read-only Git metadata in internal/project/project.go, identity_windows.go and identity_unix.go.
- [ ] T014 [US1] Implement project admission/get/list and policy get/set with replay/revision semantics in internal/app/workspace.go and internal/store/workspace.go; cover unchanged existing admission plus separate fresh observation in internal/app/workspace_test.go.
- [ ] T015 [US1] Expose project/policy HTTP and CLI operations in internal/httpapi/workspace.go and internal/cli/workspace.go; test cross-interface retrieval and safe owner restart in internal/cli/workspace_test.go.
- [ ] T016 [US1] Create branded shell/project selection/policy UI with labelled controls, visible focus and narrow layout in ui/src/index.html, ui/src/style.css and ui/src/workspace.ts; use approved assets from T003.
- [ ] T017 [US1] Add real-browser project/policy restart and keyboard acceptance in ui/tests/workspace.test.mjs and record its checkpoint in specs/001-workspace-foundation/acceptance.md.

## Phase 4 — US2: Understand existing agent setup (P2)

Independent acceptance: exactly five ordered entries distinguish installed/missing/unsupported/error
from unknown auth/model/usage; invocation and credential sentinels remain untouched.

- [ ] T018 [P] [US2] Write controlled PATH/native metadata discovery tests in internal/discovery/discovery_test.go for all five harnesses, shell wrappers, isolated errors and zero executable invocation.
- [ ] T019 [US2] Implement bounded filesystem-only installation discovery in internal/discovery/discovery.go with platform lookup helpers; no launcher reads, version/auth/model calls or persistent discovery history.
- [ ] T020 [US2] Wire fresh GET agents and CLI list/refresh through internal/httpapi/agents.go and internal/cli/agents.go, preserving per-entry unavailable errors without false missing/ready claims.
- [ ] T021 [US2] Add truthful installation cards and explicit refresh in ui/src/agents.ts, with pending/stale/error labels and no composer/selection reset; test via ui/tests/agents.test.mjs.

## Phase 5 — US3: Prepare a branded task draft (P3)

Independent acceptance: multiline Unicode draft and saved policy snapshot survive revision/restart
and UI/CLI cross-reading; stale reads/conflicts cannot erase newer user input. Status is Not run.

- [ ] T022 [P] [US3] Write draft immutable-version/policy snapshot/replay/stale-save tests in internal/app/drafts_test.go and internal/store/drafts_test.go, including rollback and uncertain commit evidence.
- [ ] T023 [US3] Implement draft create/update/get/version/list in internal/store/drafts.go and internal/app/drafts.go with complete text, atomic policy snapshot, immutable history and exact bounded pagination.
- [ ] T024 [US3] Expose draft HTTP/CLI operations in internal/httpapi/drafts.go and internal/cli/drafts.go; strict UTF-8 task files/stdin preserve BOM/U+FFFD/CRLF and reject lossy encoding, duplicate flags and unknown options.
- [ ] T025 [US3] Implement Auto composer, saved draft list/reopen/history, policy snapshot and explicit conflict reconciliation in ui/src/drafts.ts; preserve pending request IDs and local edits using guarded read generations.
- [ ] T026 [US3] Add browser stale-response/failure/conflict/refresh and CLI cross-read/restart acceptance in ui/tests/drafts.test.mjs and internal/cli/drafts_test.go; task text must match exactly.

## Phase 6 — Review and qualification

- [ ] T027 Wire committed compiled UI through internal/webui/embed.go and internal/webui/dist/, verify build reproducibility and no development assets/external requests at runtime in scripts/build.mjs.
- [ ] T028 Add explicit allowlisted native preview packaging/checksums/notices in scripts/package.mjs and package-content/offline extracted-binary tests in scripts/package.test.mjs, excluding all .council/ and vendor/private data.
- [ ] T029 Add meaningful Windows/macOS/Linux build/check jobs in .github/workflows/verify.yml, supported Linux race checks and exact toolchain pins; distinguish compile-only from runtime qualification.
- [ ] T030 Inspect actual desktop/360px renders, keyboard/focus/reduced-motion/error feedback and approved asset hashes; record screenshots and findings in specs/001-workspace-foundation/acceptance.md without private paths/tokens.
- [ ] T031 Run npm run build, npm test and npm run check; exercise quickstart.md with fake-only fixtures and record actual platform/SQLite/commands/results/limitations in specs/001-workspace-foundation/acceptance.md.
- [ ] T032 Obtain independent implementation/contract review in specs/001-workspace-foundation/review.md, correct blocking findings, rerun affected checks and retain closure evidence.
- [ ] T033 Complete feature-level WS-001–015 evidence mapping in specs/001-workspace-foundation/acceptance.md, update docs/refactor/traceability.md and README.md honestly, mark tasks only from evidence and commit source plus embedded assets before002.

## Dependencies and parallel execution

T001 → T002; T003 may run alongside T004–T011 after T002. T004 precedes store/HTTP/CLI work.
T005 precedes T006; T007 precedes T008; T009 precedes T010; T006/T008/T010 precede T011.
Shared foundation precedes stories. US1 → US2 → US3 checkpoints preserve coherent UI increments.
Within US1, T012 and shell styling can proceed independently after shared DTOs are frozen; T013
precedes T014, then T015/T016 and T017. Within US2, isolated discovery and UI card work may be
parallel once the contract is fixed. Within US3, store/service and UI composer ownership may be
parallel after tests and DTOs; integration acceptance follows both. Never edit the same file from
two agents simultaneously. Root owns task status, manifests, command wiring and integration.
T027–T031 follow stories; review/fixes precede final evidence/commit and feature002.

## Requirement coverage

| Requirements | Tasks |
| --- | --- |
| WS-001–002 | T012–017 |
| WS-003–004 | T004, T014–017 |
| WS-005–006 | T018–021 |
| WS-007–008 | T022–026 |
| WS-009–010 | T003, T016–017, T021, T025–026, T030 |
| WS-011 | T005–015, T020, T022–026 |
| WS-012–014 | T004–011, T015, T020, T024, T028, T031–032 |
| WS-015 | T002, T027–031 |
| SC-001–007 | Story checkpoints, T028–033 and final acceptance mapping |

## Implementation strategy

US1 is the first testable increment, not the entire feature. Finish all three stories plus review
before moving to002. Tests requested by the specification target observable failures and data
integrity; they do not duplicate implementation details. No live vendor task is authorized by
this development goal. Specify extension hooks are absent and therefore skipped.
