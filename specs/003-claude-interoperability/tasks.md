# Tasks: Claude Code interoperability

**Branch**: `refactor` | **Context**: `003-claude-interoperability`

**Input**: spec.md, plan.md, research.md, data-model.md, contracts/native-execution.md, quickstart.md.

Tests are required by CLA-020/SC-008 and the repository development rules. Write meaningful failing
protocol/state/control acceptance before enabling its behavior. Existing accepted infrastructure is
reused; no account router, parallel graph, API fallback or future-adapter scaffold belongs here.

## Phase 1 — Setup and shared interfaces

- [x] T001 Freeze and implement the documented optional observation DTOs, harness/connection resolution and parser/result boundary in internal/domain/execution.go, execution_validation.go and internal/adapters/types.go, preserving existing statuses, fingerprints and legacy null/absent meanings.
- [x] T002 Update preview2.0.0-dev.3/003-v1 expectations in internal/domain/types.go, package.json, package-lock.json, scripts/package.mjs and scripts/packagearchive/README.md without new dependencies or product capabilities beyond this slice.

## Phase 2 — Foundation before Claude dispatch

- [x] T003 Write frozen populated schema1/2 claim/receipt/large-counter/migration rollback, malformed-DDL and held-incident preservation cases in internal/store/interop_migration_test.go; assert exact unaffected row/envelope bytes.
- [x] T004 Implement transactional schema3 claim-resource migration and managed-native pool queries in internal/store/schema.go, execution_schema.go, attempts.go, results.go and incidents.go; retain actual historical connection labels and claims without replay/reset.
- [x] T005 [P] Add native-shaped Claude builder/decoder fixtures and tests in internal/adapters/claude_test.go and testdata/claude/, covering success+is_error, startup/error/partial/trailing/repeated/conflicting/session/output/numeric boundaries and no prose commands; retain source provenance.
- [x] T006 Implement the bounded common parser interface and Claude result reducer in internal/adapters/types.go, codex.go and claude.go, requiring EOF-finalized error-free terminal proof and preserving the accepted Codex protocol behavior.
- [x] T007 Implement the Claude launch builder and native executable resolution in internal/adapters/claude.go, resolver.go and internal/discovery/discovery.go; apply exact flags/stdin/selectors, preserve authentication/instructions, report wrapper/incompatible/unknown prerequisites without probes or fallback.
- [x] T008 Implement fixed Claude-shaped fake streams and response/error/control scenarios in internal/fixture/fixture.go and its existing scenario files; select fake protocol by captured harness, never contact a provider and keep provider observations unknown.
- [x] T009 Integrate captured target dispatch, shared claim, parser selection and Claude event-to-artifact response strategy in internal/engine/engine.go and results.go, preserving all input/root/cleanup/evidence/terminal gates and Codex output-reservation cleanup.

## Phase 3 — US1: Give an ordinary task to Claude (P1)

Independent acceptance: optional Claude choice starts one bounded fake through UI/CLI with exact
original intent and policy; existing omitted Auto remains Codex and unsupported choices do not
cause another target, login or configuration path.

- [x] T010 [US1] Add admission/default/explicit-harness/model-policy/prerequisite/request-replay tests in internal/engine/interop_admission_test.go and internal/domain/execution_validation_test.go; changing global defaults or request target cannot reinterpret existing intent.
- [x] T011 [US1] Expose003 metadata/capabilities and optional --harness through internal/app/execution.go, internal/httpapi/execution.go, internal/cli/parse.go, execution.go and metadata.go; keep strict current execution handshake and unchanged request/receipt meanings.
- [x] T012 [US1] Add optional target choice and Claude Agents action in ui/src/tasks.ts, agents.ts, index.html, types.ts and execution-types.ts; freeze the clicked harness in pending intent, preserve newer typing and retain Codex Auto wording.
- [x] T013 [US1] Add direct/draft/policy/unsupported/Auto regression journeys in ui/tests/claude-start.test.mjs and internal/cli/interop_integration_test.go, proving exactly one worker and zero provider/model probes.

## Phase 4 — US2: Inspect what Claude actually produced (P1)

Independent acceptance: full exact native bytes and derived response/candidate remain attributable;
native errors cannot become success, model/settings/usage/session meanings stay separate and
fake values cannot become measured provider facts.

- [x] T014 [US2] Add actual-main-model versus configured/modelUsage, exact counts/cost lexemes, duplicate/repeated usage, preview omission/budget and fake-fact tests in internal/adapters/claude_observations_test.go and internal/domain/execution_validation_test.go.
- [x] T015 [US2] Implement sourced configured/version/session/main-model and bounded nativeModels observations in internal/adapters/claude.go and domain observation helpers, including explicit partial/raw-stdout references and top-level versus whole-tree scope without double counting or billed-cost inference.
- [x] T016 [US2] Verify exact Unicode/BOM/CRLF response artifact provenance, trailing diagnostics, malformed/error/limit/empty result, shared response-only acceptance and no synthetic file strategy in internal/engine/interop_results_test.go and internal/store/results_test.go.
- [x] T017 [US2] Render captured harness, configured versus actual models, estimates/scopes, partial-preview references, native version and unsupported continuity in ui/src/tasks.ts and execution-types.ts, preserving compact summaries and full raw reads.
- [x] T018 [US2] Add controlled-observation and actual fake-response browser checks in ui/tests/claude-evidence.test.mjs; label authored test data, keep fake provider usage unknown and prove native prose never starts work or counts as a check.

## Phase 5 — US3: Control mixed-harness work consistently (P1)

Independent acceptance: two independent clients targeting different harnesses share one owner/pool;
Stop and late failure/success preserve authority, newer guidance cannot dispatch an obsolete target,
and reconnect/restart never duplicates or silently resumes work.

- [x] T019 [US3] Add barrier-released independent Codex/Claude CLI starts in internal/cli/concurrent_start_integration_test.go, asserting exact captured connections, one active worker, no queued launch/source evidence and globally ordered transfer only after verified settlement.
- [x] T020 [US3] Apply Stop-before-launch, failure-before-Stop, deadline, input/protocol/evidence/commit uncertainty and restart/no-replay cases to Claude in internal/engine/interop_control_test.go and recovery_test.go; reuse actual owned process fixtures rather than alternate cleanup logic.
- [x] T021 [US3] Verify original source/index/branch, applicable captured instructions, candidate attribution and unsupported source blockers across both harnesses in internal/engine/interop_workspace_test.go; do not suppress instructions or substitute clean HEAD.
- [x] T022 [US3] Add mixed target guidance, queued-version invalidation, lost-start target freeze, stale detail/selection and active Stop browser tests in ui/tests/claude-control.test.mjs using the same lifecycle APIs.

## Phase 6 — US4: Upgrade without stranding existing work (P2)

Independent acceptance: populated previous-preview state and receipt meanings survive; a newer
client explicitly stops only a known compatible exact older owner while refusing incompatible
new execution and unknown takeover.

- [x] T023 [US4] Write current/known002/unknown metadata and exact-target Stop handshake tests in internal/cli/maintenance_test.go, including bounded strict JSON, missing/extra fields, wrong identity, no bootstrap and lost acknowledgement/replacement cases.
- [x] T024 [US4] Implement the separate known-compatible maintenance verifier and Stop-only owner selection in internal/cli/metadata.go, cli.go, owner.go and execution.go; preserve exact request/owner binding and strict current task compatibility without opening/migrating a busy store.
- [x] T025 [US4] Exercise populated schema2-to3 and schema1-to3 state through shared services in internal/store/interop_migration_test.go and internal/cli/interop_integration_test.go; preserve original target JSON, exact receipts, artifacts, counts, claims and incidents.
- [x] T026 [US4] Add a separately running authenticated older-owner fixture for newer-client Stop in internal/cli/maintenance_integration_test.go; lose reply, replace owner and reconcile the same earlier target without stopping the replacement or using PID/lock repair.

## Phase 7 — Review and qualification

- [x] T027 Inspect desktop/360px target/progress/result/error/Stop/version/session/usage renders, keyboard focus, reduced motion and approved asset hashes; record actual screenshots and findings in specs/003-claude-interoperability/acceptance.md.
- [x] T028 Extend scripts/package.test.mjs and scripts/packagearchive/README.md with extracted explicit Claude fake execution/evidence and003 metadata; preserve exact allowlist/notices/checksums, empty-PATH foundation startup and no Node/npm/Go runtime requirement.
- [x] T029 Run npm run build, npm test and npm run check against final source, recording actual Go/SQLite/Node/browser versions and outcomes in specs/003-claude-interoperability/acceptance.md; commit matching internal/webui/dist output.
- [ ] T030 Qualify all six actual native platform jobs and supported Linux AMD64 race checks via .github/workflows/verify.yml against the implementation commit; distinguish runtime from compilation and unperformed live provider qualification.
- [x] T031 Obtain independent source/contract/failure-path review in specs/003-claude-interoperability/review.md, correct blocking findings and recheck affected behavior before acceptance.
- [ ] T032 Complete CLA-001–021 and SC-001–008 observed evidence mapping, update docs/refactor/traceability.md, delivery-plan.md and README.md honestly, mark tasks from actual checks and commit reviewed source/embedded assets before feature004.

## Dependencies and parallel ownership

T001 freezes shared DTO/parser/target contracts. T003 precedes T004. T005 defines the native protocol
before T006/T007; T008 follows its fixed protocol. T009 waits for the pool, builders, parser and
fake emitter. No Claude dispatch is enabled before that checkpoint. US1 entrypoints use it; US2
extends the same observation/results path; US3 qualifies mixed controls; US4 closes the upgrade
boundary. Complete every story and the qualification gate before feature004 application code.

After T001, a store owner can handle T003–004 while an adapter/fake owner handles T005–008 on
separate files. Root integrates engine/app/CLI and manifests. A UI owner can prepare T012/T017
against the frozen contract, then exercise real shared APIs after T009/T011. Shared domain/types,
CLI, adapter and UI files have one explicit writer at a time; do not parallelize dependent edits.
The [P] marker only authorizes its independent initial test-definition work after shared freeze.

For US1, prepare UI controls while root wires routes, then run the actual joint journey. For US2,
protocol/metadata tests can be authored separately from UI presentation after type freeze. For US3,
CLI mixed-client and browser stale-state tests may be authored independently but execute resource-
heavy suites sequentially on the local machine. For US4, migration preservation and authenticated
older-owner fixture definitions are independent; their integration follows the maintenance verifier.

## Requirement coverage

| Requirements | Tasks |
| --- | --- |
| CLA-001–003 | T001, T004–013, T019, T022, T025 |
| CLA-004–006 | T005–009, T010, T013, T021 |
| CLA-007–010 | T001, T005–006, T009, T014–018 |
| CLA-011–012 | T005–006, T009, T016, T018, T021 |
| CLA-013–015 | T003–004, T009–011, T019–022, T025 |
| CLA-016–017 | T005, T007, T010–013, T017–018, T027–030 |
| CLA-018–019 | T003–004, T023–026 |
| CLA-020–021 | T005–008, T013–018, T020, T027–032 |
| SC-001–008 | Independent story checkpoints and T027–032 |

## Delivery strategy

The first usable increment is a labelled Claude fake through the existing authoring path, with
truthful result/candidate and ownership facts. It is not accepted implementation, live account
qualification or economical routing. Tasks are checked only against observed evidence. T030 and T032 remain open until hosted qualification and final acceptance reconciliation are complete.
Use Specify analyze read-only for the final artifact gate, separately apply authorized corrections,
and commit the design/task artifacts before application implementation. No extension or automatic
Git branching hooks are configured. Development authorizes no vendor/model call.
