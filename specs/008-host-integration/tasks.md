# Tasks: MCP Host Integration and Interface Parity

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[data-model.md](data-model.md), [contracts/catalog.md](contracts/catalog.md),
[contracts/protocol.md](contracts/protocol.md) and [quickstart.md](quickstart.md).
All work stays on `refactor` and uses local controlled fake agents; no vendor
credentials or live provider calls are authorized. Checkboxes represent work to
be performed, not acceptance evidence.

**Format**: `[P]` means work on distinct owned files may run concurrently after
its stated prerequisites. Story labels refer to the corresponding specification
scenario. Each task names the primary files its owner may edit; new tests are
under the same owning package unless otherwise assigned below.

## Phase 1: Setup and design gate

- [X] T001 Review `specs/008-host-integration/spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/` and `.specify/memory/constitution.md`; resolve any blocking catalog/protocol contradiction before product implementation and record decisions in `specs/008-host-integration/review.md`.
- [ ] T002 Pin and inspect the selected official Go MCP SDK v1.8.0 and its actual transitive modules in `go.mod` and `go.sum`; verify the Go 1.27.3 build and supported SDK APIs before adopting them.
- [ ] T003 Audit licenses of the selected SDK and actual transitive dependency versions, update `third-party/go-NOTICES.txt` and `scripts/packagearchive/notices.mjs` as applicable, and record the chosen SPDX/notices evidence in `specs/008-host-integration/research.md` without adding runtime npm dependencies.

## Phase 2: Shared owner and protocol foundation

**Blocking prerequisite for all stories.** Keep one continuing owner and one
authority store; every effect-capable call carries its own Council request ID.

- [ ] T004 Add a fixed-route authenticated owner transport in `internal/ownerclient/client.go` and `internal/ownerclient/client_test.go`: `owner.Ensure`, exact binary/descriptor ownership, loopback origin/host and bearer in memory, no proxy or redirect, bounded reads and strict status/envelope/receipt validation.
- [ ] T005 Extract/share strict current execution, graph, recovery, history and separate older-owner maintenance compatibility checks into `internal/ownerclient/metadata.go`; preserve `internal/cli/metadata.go` and `internal/cli/cli.go` behavior, frozen graph intents and diagnostic/redaction guarantees with regression tests in `internal/ownerclient/metadata_test.go` and `internal/cli/`.
- [ ] T006 Add bounded newline-delimited raw stdio transport in `internal/mcphost/transport.go` and `internal/mcphost/transport_test.go`: 64 MiB wire limit, UTF-8, malformed/truncated input, duplicate outer/nested members, no non-protocol stdout and serialized concurrent writes.
- [ ] T007 Implement SDK initialization/version negotiation and cancellation dispatch in `internal/mcphost/server.go` and `internal/mcphost/server_test.go`; qualify only 2025-06-18 and 2025-11-25, initialized state, unknown methods, client EOF and protocol-valid errors as specified in `contracts/protocol.md`.
- [ ] T008 Define the closed name/route/permission catalog in `internal/mcphost/catalog.go` and `internal/mcphost/catalog_test.go` using `contracts/catalog.md`; mechanically fail on missing, duplicate or extra current owner mappings and never expose an arbitrary URL, method or shell proxy.
- [ ] T009 Generate and independently review a complete, versioned schema for every catalog tool in `internal/mcphost/schemas/` and `internal/mcphost/schema_test.go` against the actual owner DTOs, validators and presence semantics; reject drift, unknown fields, incorrect bounds or missing required authority before any dispatcher or family handler is implemented.
- [ ] T010 Implement raw schema validation and dispatch in `internal/mcphost/schema.go` and `internal/mcphost/dispatch.go` against the frozen T009 schemas, preserving duplicate/unknown/null/omitted distinctions, canonical decimal strings, `{status,envelope}`, safe text fallback and `isError`; wire `council mcp` through `internal/cli/` and `cmd/council/`, with clean lifecycle/stdout tests in `internal/mcphost/entrypoint_test.go`.

## Phase 3: User Story 1 — Shared task and owner (P1, MVP)

**Goal:** A host connects to the existing owner, starts a fake task and shares
its identity and outcome with UI/CLI. **Independent test:** two real stdio
clients and the actual local fake owner exercise both directions with one owner.

- [ ] T011 [P] [US1] Add SDK-client subprocess tests for no-owner bootstrap, existing-owner reuse, two concurrent hosts, malformed/stale owner metadata and stdout purity in `internal/mcphost/owner_integration_test.go`.
- [ ] T012 [P] [US1] Add real host→CLI/UI and CLI/UI→host project/task fixture journeys with original Unicode instructions, exact identities and no extra agent calls in `ui/tests/mcp-workspace.test.mjs` and `internal/mcphost/workspace_integration_test.go`.
- [ ] T013 [US1] Implement catalog schemas and dispatch for owner metadata, project open/list/get, policy get/set, the single fresh observational agents-list operation and draft create/update/get/list in `internal/mcphost/catalog_workspace.go` and `internal/mcphost/catalog_workspace_test.go`.
- [ ] T014 [US1] Implement accepted task start/list/get/version/guidance/attempt start and inspect routes with exact owner DTOs and request IDs in `internal/mcphost/catalog_execution.go` and `internal/mcphost/catalog_execution_test.go`.
- [ ] T015 [US1] Verify compatible owner lifecycle and same-record handoff through the real entrypoint, including no owner replacement on disconnect, in `internal/mcphost/workspace_integration_test.go` and `ui/tests/mcp-workspace.test.mjs`.

## Phase 4: User Story 2 — Evidence, wait and explicit controls (P1)

**Goal:** Bounded exact evidence and observer cancellation preserve continuing
work. **Independent test:** reconstruct real fake-owner bytes and explicitly
stop a still-running attempt after a cancelled host wait.

- [ ] T016 [P] [US2] Add real binary, multibyte, empty, growing and >65,536-byte evidence paging tests in `internal/mcphost/evidence_integration_test.go`, comparing concatenated bytes/digest and a fixed `through` boundary with owner/CLI results.
- [ ] T017 [P] [US2] Add actual wait timeout, MCP cancellation, host EOF and separate explicit Stop/pause scenarios in `internal/mcphost/cancellation_integration_test.go`; verify work remains owned and partial/failure/cleanup precedence survives.
- [ ] T018 [US2] Implement task attempts/events/artifacts, bounded artifact byte reads, waits, explicit Stop and response acceptance in `internal/mcphost/catalog_observation.go` and `internal/mcphost/catalog_observation_test.go`; preserve base64, decimal offsets, EOF, stream closure and integrity without decoding bytes as text.
- [ ] T019 [US2] Implement task/graph pause, checkpoint inspection and explicit resume tools with exact `PauseTaskInput`, `PauseGraphInput` and `ResumeCheckpointInput` authorities in `internal/mcphost/catalog_recovery.go` and `internal/mcphost/catalog_recovery_test.go`.
- [ ] T020 [US2] Verify observer cancellation never sends Stop or shuts down the owner, and that accepted cancellation, late completion, blocked cleanup and checkpoint drain remain distinguishable through the real protocol in `internal/mcphost/cancellation_integration_test.go`.

## Phase 5: User Story 3 — Exact cross-interface retry (P1)

**Goal:** Lost responses never broaden authority or cause a second effect.
**Independent test:** drop a real committed response, reconnect with the same
Council ID/body and a different JSON-RPC ID, then test an effect-changing-input conflict.

- [ ] T021 [P] [US3] Add real post-commit dropped-acknowledgement, reconnect, concurrent duplicate, owner-canonical omitted/default equivalence and effect-changing input/scope conflict tests in `internal/mcphost/retry_integration_test.go`, comparing original owner/CLI receipt, attempt ID and effect count.
- [ ] T022 [P] [US3] Add graph, recovery and history frozen-authority retry fixtures in `internal/mcphost/authority_retry_test.go`, including changed or stale task/node/graph/budget authority, owner-accepted default equivalences and unchanged unrelated work.
- [ ] T023 [US3] Implement mandatory durable `input.requestId` distinct from JSON-RPC correlation, complete body/path retention or caller-retained explicit intent, one forward and bounded unknown-acknowledgement handling in `internal/mcphost/dispatch.go` and `internal/mcphost/dispatch_test.go`; preserve the owner's documented canonical default equivalences without refreshing authority.
- [ ] T024 [US3] Preserve the CLI's existing private frozen graph/recovery/history intent behavior while ensuring MCP never silently recaptures newer proof in `internal/ownerclient/` and `internal/cli/graph_intents.go`; verify same-ID cross-interface replay for accepted canonical equivalences and conflict for changed authority in `internal/cli/` and `internal/mcphost/authority_retry_test.go`.
- [ ] T025 [US3] Verify persistence uncertainty, owner restart and in-flight mutation cancellation never authorize automatic replay or claim non-commit in `internal/mcphost/retry_integration_test.go`.

## Phase 6: User Story 4 — Routing, workflow, graphs and recovery (P2)

**Goal:** Every accepted family in the audited catalog has equivalent owner
semantics. **Independent test:** controlled routing, workflow and dependent graph
exercise the actual owner, two hosts and independent incident/fence.

- [ ] T026 [P] [US4] Add offline owner-backed routing policy/context/profile, declared account/link, capacity report, budget grant/adoption and unknown-telemetry tests in `internal/mcphost/routing_integration_test.go`.
- [ ] T027 [P] [US4] Add fake-worker workflow/review/check/candidate/delivery and sibling/dependency graph journeys in `internal/mcphost/orchestration_integration_test.go`, comparing exact authorities, retained outcomes and shared capacity across interfaces.
- [ ] T028 [P] [US4] Add independent incident, checkpoint resume and incomplete-history preview/export/reconcile tests in `internal/mcphost/recovery_history_integration_test.go`; preserve unrelated fences and explicit non-backup notice.
- [ ] T029 [US4] Implement all routing and capacity tool mappings from `contracts/catalog.md` in `internal/mcphost/catalog_routing.go` and `internal/mcphost/catalog_routing_test.go`, using exact owner DTOs and selector/page constraints.
- [ ] T030 [US4] Implement workflow preferences, adoption, advance/check and stage/review/finding/check/candidate/change/delivery inspection mappings in `internal/mcphost/catalog_workflow.go` and `internal/mcphost/catalog_workflow_test.go`.
- [ ] T031 [US4] Implement graph declaration/list/detail/wait, input/member/dispatch/budget inspection and explicit advance/guidance/grant/node/group Stop mappings in `internal/mcphost/catalog_graphs.go` and `internal/mcphost/catalog_graphs_test.go`.
- [ ] T032 [US4] Complete exact incident inspection/reconciliation and metadata-history capture/page/preview/export/operation/reconcile mappings in `internal/mcphost/catalog_recovery.go`, `internal/mcphost/catalog_history.go` and their tests; never invent checkpoint/incident listing routes.
- [ ] T033 [US4] Exercise every catalog row and compare its route, body, query, status, receipt and error against the authenticated real owner in `internal/mcphost/catalog_parity_test.go`; an omitted, extra, unsupported or permissive operation blocks acceptance.

## Phase 7: User Story 5 — Setup and honest compatibility (P2)

**Goal:** A programmer can connect a supported host without vendor credentials
or unnecessary settings. **Independent test:** follow the exact guide using an
offline real client with Unicode/spaces and incompatible-owner fixtures.

- [ ] T034 [P] [US5] Add known older maintenance-owner Stop, unsupported protocol/capability and incompatible locked-owner preservation tests in `internal/mcphost/compatibility_integration_test.go`; never replace, delete or silently migrate the live owner.
- [ ] T035 [US5] Publish concise English stdio setup, executable path/quoting examples, request/retry/evidence usage and truthful tested host/protocol matrix in `docs/refactor/mcp-host-guide.md` and `README.md`; never include bearer tokens or vendor authentication.
- [ ] T036 [US5] Execute the setup guide with a disposable Unicode/space path and an actual offline client; verify actionable diagnostics, protocol-only stdout and any new UI's accessible names/focus in `internal/mcphost/setup_integration_test.go` and, only if UI is added, `ui/tests/mcp-setup.test.mjs`.

## Phase 8: Independent review, qualification and delivery

- [ ] T037 Independently review `internal/mcphost/`, `internal/ownerclient/`, CLI changes and every `contracts/catalog.md` row against HI-001–020, raw protocol security, captured authority, cancellation, evidence and credential boundaries; record concrete findings and corrections in `specs/008-host-integration/review.md`.
- [ ] T038 Correct and re-run every blocking finding and meaningful affected check; record actual results and exact source identity in `specs/008-host-integration/review.md`, preserving unperformed host/vendor qualification as unknown.
- [ ] T039 Run focused actual SDK-client, owner/CLI, two-host, cross-interface, frozen-retry, binary paging and cancellation suites from `internal/mcphost/`, `internal/ownerclient/`, `internal/cli/` and `ui/tests/mcp-workspace.test.mjs`; do not replace real-client evidence with mocks alone.
- [ ] T040 Run `npm run build`, complete `npm test` and `npm run check` on the final product source, including package/extracted-offline and release-data exclusion checks in `scripts/package.test.mjs`; commit any changed `internal/webui/dist/` generated assets with their source.
- [ ] T041 Record requirement-to-test traceability, command outcomes, exact source revision, platform limitations, skips, dependency notices and unperformed live-provider scope in `specs/008-host-integration/acceptance.md`; retain full evidence export, verified backup/restore and reviewed cleanup as mandatory future work.
- [ ] T042 Commit and push the reviewed exact product/test source on `refactor`, observe all six native hosted jobs plus Linux race on that exact head, and correct/requalify failures before stating success in `specs/008-host-integration/acceptance.md`.
- [ ] T043 Commit and push a documentation-only acceptance follow-up containing the actual final hosted run/results and remaining limitations in `specs/008-host-integration/acceptance.md`; do not claim full release or start the next feature before this gate.

## Dependencies, sequencing and ownership

T001–T003 establish the approved design and dependency/license baseline. T004–T010
are shared foundations and must be coherent before a story uses them. T008 freezes
the catalog, then T009 generates and independently reviews every versioned schema;
T010 and all family handlers must not start before this schema gate. US1
(T011–T015) is the independently testable MVP; US2 and US3 build on its real
owner path, and US4/US5 complete the mandatory feature rather than becoming
optional release work. T037–T043 follow implementation and independent story
checks; hosted qualification is for the exact product source, followed only by
the documented acceptance commit.

Assign **one owner-client/CLI maintainer** T004–T005 and T024, with exclusive
ownership of `internal/ownerclient/` and shared `internal/cli/` extraction files.
Assign **one MCP foundation/entrypoint maintainer** T006–T010 and T023, owning
`internal/mcphost/{transport,server,catalog,schema,dispatch}*.go` and the narrowly
scoped `cmd/council/`/CLI entrypoint while coordinated with the CLI maintainer.
Assign **story implementers** disjoint `catalog_workspace`, `catalog_execution`,
`catalog_observation`, `catalog_routing`, `catalog_workflow`, `catalog_graphs`,
`catalog_recovery` and `catalog_history` files; T019 and T032 serialize on
`catalog_recovery.go`. Assign **independent test owners** new test files named
in T011–T012, T016–T017 and T021–T022/T026–T028, avoiding concurrent edits to a
shared fixture. Root owns specs, final integration, docs/notice coordination and
the one-at-a-time full test/build/CI slot. T002–T003 serialize `go.mod`,
`go.sum` and notices; never allow two agents to edit a shared source file at once.

Examples of safe parallel work after the foundation freezes: T011 with T012;
T016 with T017; T021 with T022; and T026, T027 and T028 on their own new test
files. Implementations touching `catalog_recovery.go`, the entrypoint, shared
CLI transport or package checks remain sequential. A story's test files may be
prepared in parallel, but run tests only when the coordinator has frozen the
relevant source and scheduled a test slot.
