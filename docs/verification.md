# Version 1.2 verification record

This record describes local release verification on 3–4 November 2026. Council targets trusted local workspaces under one operating-system account. These results establish the behavior of the tested builds and fixtures; they do not certify a hosted service, an OS sandbox or every vendor account configuration.

## Billing, economical routing and collaboration skills — 4 November 2026

The final feature build passed `npm test`: 600 passed, four platform-limited skips, zero failed (604 total). `npm run check`, the fresh build and generated billing/routing schemas passed on Windows x64 with Node 24.22.1 and SQLite 3.53.4. All four new skill folders passed the skill-creator validator. The installed-tarball acceptance check verifies their packaged Markdown, generated schemas and new MCP tools, then runs a skill-enabled offline task from that installation without production npm dependencies.

New coverage verifies unknown billing defaults, API/subscription modes, nullable prices, shared-account quota storage and linked revision conflicts, alias-local routing/skills, import provenance, observation bounds and expiration, rollback, SQLite contention and schema/runtime boundaries. Routing checks preserve explicit models and effort, avoid model-name and task-length heuristics, reserve the actual workflow budget, refuse silent tier escalation and distinguish obsolete quota from current exhaustion. CLI, authenticated HTTP and official MCP SDK checks exercise billing inspection, edits, planning and routed execution. Worker skills are snapshotted, injected only into selected participants, and preserved through room voting and task execution; unused judge metadata cannot inject an extra participant.

Browser QA used separate ignored mock state. It displayed an explicitly labelled fixture subscription with a 75-percent allowance, saved a plan-label edit without changing its observation timestamp, showed unknown API quota and prices, and assigned an implementation/review skill. A routine economy preview selected only the configured small fixture model, estimated one direct-workflow call, and applied the team only after a manual click. Neither preview nor application invoked an agent. The temporary fixture server was stopped; the native dashboard was refreshed after confirming no active jobs and was left on Agents & billing with unverified records clearly unknown.

A read-only Codex host tool exposed a current subscription usage report during development. Its account-to-CLI association was not assumed or imported without confirmation. Other account data remains user-entered or explicitly imported; this feature does not claim universal live quota refresh or measured charges. No vendor credential files were read, no live provider model calls were made, and DSH remained excluded. Local test logs and screenshots remain under `.council/` rather than in packages.

## Extended correctness and usability review — 4 November 2026

Parallel review covered planning, provider-independent lifecycle handling, MCP/CLI results, room ownership and ballots, and browser state. The final build passed `npm test`: 547 passed, three platform-limited skips, zero failed (550 total). This adds 42 tests to the preceding refinement build. `npm run check`, the fresh build and generated schemas passed on Windows x64 with Node 24.22.1 and SQLite 3.53.4.

| Area | Verified issue and correction |
| --- | --- |
| Planning | Removed fixed three-round/64-call ceilings and the output-budget product restriction. Exact call estimates use safe integers and BigInt arithmetic; large plans no longer allocate metadata for every round before budget validation. |
| Hierarchy | Positive finite weights now support scientific notation outside the old 0.1–10 range. Exact decimal majority checks preserve ties, overflowing totals stay serializable, and call refunds avoid unsafe intermediate arithmetic. |
| Approval | Contradictory duplicate ballot keys, including escaped-equivalent names and duplicate assignments, require attention rather than authorizing execution. Raw evidence remains in the workflow record. |
| Cancellation | The first cancellation/timeout cause survives slow process cleanup. Cancelling blocking advice stops and drains only its newly owned run. Local owned work also stops when cancellation storage fails, while the original persistence error remains visible. |
| Results | Wait responses use one full run snapshot and include every step's metadata beyond summary-preview limits. Missing/pruned results are explicit. CLI waits return nonzero for failed, cancelled or still-pending work. |
| Ownership | Stop operates atomically on the latest room state. Closing retries emergency finalization after storage recovery, preserves newer PM guidance and releases ownership; persistent failure rejects close explicitly while still closing the database. |
| Browser state | Room-specific drafts retain their author, complete text, permissions and dependencies. Generations protect newer drafts from late mutation responses. Closed history/create dialogs ignore obsolete responses; successful creation remains distinct from a later refresh failure. History pages append without rebuilding earlier messages. |
| Handoffs | Peer messages and project notes retain complete evidence beyond the former text caps. A valid message larger than the usual inbox-page budget is returned alone, so accepted handoffs remain retrievable. |

Browser QA verified draft isolation and restoration across two offline rooms, positive weights of 1000 and 0.00001, creation-dialog controls, and desktop/mobile layout. The temporary offline UI used separate ignored state; its server and tab were closed. The native dashboard was refreshed after confirming no active room or workflow jobs. No live provider calls were made during this review, and DSH remained excluded.

The first integrated run caught an outdated test expecting weights above 10 to fail. Its expectations were updated to the new positive-finite contract, then the complete suite passed. A separate SSE test was made deterministic after proving that garbage collection had closed unretained client responses; the production connection limit was already correct. Full logs and screenshots remain under ignored `.council/` state.

## Room precision refinements — 4 November 2026

The final refinement build passed `npm test`: 505 passed, three platform-limited skips, zero failed (508 total). `npm run check` and the build passed on Windows x64 with Node 24.22.1 and SQLite 3.53.4. Generated runtime files and schemas accompany the source changes.

Twenty-three new regressions cover room selection and stale response races, update-stream cleanup and authorization failures, omitted authority defaults, stale approvals after project-manager messages, cross-process instruction changes immediately before dispatch, active-work draining, unavailable prerequisites, storage-error classification and slow cancellation. Workflow checks preserve every earlier debate round, relevant advisor lineage and complete continuation evidence. Exact duplicate selected outputs and identical caller/project context are represented once without dropping distinct evidence; an insufficient explicit context budget still rejects before any model call.

Browser QA verified room switching, complete history, the English guidance note, a visible authorization error after restarting the owned dashboard, and recovery through the newly printed dashboard link. Already-admitted workflows retain their dispatched instructions, including calls waiting for a slot; Stop cancels admitted work. New instructions invalidate approval for further room tasks. This refinement used local fake agents and existing saved live evidence; no additional vendor calls were made and DSH remained excluded. Logs and the final screenshot remain under ignored `.council/` state.

## Project rooms, waiting and advisor verification — 4 November 2026

The final feature build passed `npm test`: 482 passed, three platform-limited skips, zero failed. `npm run check`, the fresh build and generated schemas also passed on Windows x64 with Node 24.22.1 and SQLite 3.53.4. Runtime code still has no third-party npm dependencies.

New tests cover server-side idle waits with unchanged model-dispatch counts, prompt cancellation of waits during shutdown, independent advice through the official MCP SDK, complete selected answers beyond status-preview lengths, configurable effort and advisor quotas, admission-slot release and worker resumption in the same real Git worktree. Room regressions cover weighted approval and dissent, dependency ordering, isolated writes, cancellation races, storage rollback/recovery, full history paging, backend provenance, authenticated loopback HTTP and bounded SSE delivery.

The user's later requirements were applied throughout the product: the UI and its messages are English; task, project and peer context is complete by default. Tests preserve task text beyond the former 32,000-character limit, peer/project context beyond 64,000 characters and room messages older than the latest hundred-message display window. An explicitly chosen insufficient context budget fails before dispatch. Interface previews are labelled and do not limit model context. Advisor quotas, call budgets, discussion rounds and context budgets are configurable; no fixed truncation is used as a token-saving policy.

Two additional live, read-only checks completed without invoking DSH or reading vendor credential files:

- Junie yielded an advisor request; native Codex using the exact model `gpt-6.1-sol` at `low` effort answered; Junie resumed and returned the required acceptance marker. Three calls completed in about 91 seconds.
- A native room collected valid weighted proposals from Claude and Junie, approved the task at weight 4/4, assigned it to Junie and executed it automatically. The room completed in exactly three model calls with the expected result and no worktree writes.

Browser QA exercised English room creation, member weights and advisor/model settings, a project-manager message, a dependent task graph, automatic offline execution and full-history viewing. The final UI also displayed the successful native room with explicit live/mock provenance and full-context status. Screenshots and full live receipts remain under ignored `.council/` state. The local UI makes no provider calls until the user starts work. Holding an MCP wait does not dispatch an orchestrator model; host tool deadlines and host inference scheduling remain outside the plugin's control.

## Unreleased regression check — 4 November 2026

A follow-up check on native Windows x64 used Node 24.22.1 and SQLite 3.53.4, selected through a process-local `PATH`. The system's default Node 24.14.1 is below the supported minimum and was left unchanged.

- `npm test`: 371 passed, three skipped, zero failed; fresh runtime and schema generation included. The skips are two POSIX file-permission cases and one file-symlink case requiring Windows Developer Mode or elevated rights.
- `npm run check` and `npm run build`: passed. Generated `dist/` changes accompany the source fixes; schema contents are unchanged.
- Seven CI-helper tests and the six-mode offline CLI demo: passed, with zero live provider calls.
- Concurrent cold SQLite initialization: passed ten additional consecutive two-process runs after the startup fix.

The initial suite exposed three Git fixture failures caused by inherited Windows line-ending settings; the fixtures now set their own `core.autocrlf`. A later full run and an isolated repeat reproduced an immediate SQLite busy error during cold WAL initialization. New deterministic regressions cover transient and persistent WAL contention, preservation of non-contention errors and restoration of the 250 ms runtime timeout. Other new regressions cover provider cancellation and its short artifact deadline, mixed failed/cancelled peers, empty CLI selections and input paths, concurrent worktree base capture and POSIX cleanup errors. POSIX kill-error branches are simulated on this Windows run; this follow-up did not execute a native POSIX suite.

## Requested live collaboration check — 4 November 2026

The user explicitly requested live testing on this PC and subsequently prohibited DSH for the day. DSH was never invoked. No vendor credential files were read or copied, and no models were overridden. The user renewed Claude and Junie authentication through their native CLIs after initial authentication failures.

| Agent | Tested Council transport | Exact-answer smoke check | Three-agent pipeline stage |
| --- | --- | --- | --- |
| Claude | Native JSON CLI | Completed, 4.2 seconds | Completed, 13.8 seconds |
| Junie 26.7.20 (2383.10) | Direct ACP through the installed native executable | Completed, 9.1 seconds | Completed, 38.7 seconds |
| Antigravity | Native `agy` stdin NDJSON | Completed, 13.4 seconds | Completed, 50.0 seconds |

All calls used read permission. A Claude → Junie pipeline completed first; the subsequent Claude → Junie → Antigravity pipeline completed in about 103 seconds. Claude generated a fresh eight-character nonce absent from the task. Both later agents reproduced it and explicitly acknowledged receipt of peer evidence. They inspected the permitted project files and refined a validation checklist; Antigravity challenged the peers' incorrect suggestion that every step status must equal the aggregate workflow status. This establishes an actual handoff and peer critique for this bounded task, not arbitrary task correctness or a five-agent session. No live write workflow was attempted.

The live checks exposed two integration gaps: the standard Windows Junie batch launcher was not recognized, and this machine had the Antigravity CLI rather than its separate ACP runtime. Council now resolves Junie's installer layout without executing the batch and supports an explicitly selected native Antigravity transport. Junie also once returned an authentication-error answer with an ACP `end_turn`; that exact provider error now fails instead of masquerading as a completed task. Regression fixtures preserve each failure case.

After these changes, `npm test` passed 398 tests, skipped three platform-limited cases and failed none. `npm run check` passed; the build regenerated the shipped runtime and schemas. The local transcripts, run IDs and exported report remain under ignored `.council/` state rather than in the release package. Strict Antigravity diagnostic handling and native permission limits are described in the README.

## Complete release checks

Both environments ran `npm run verify`: TypeScript checking, a fresh build, the complete automated suite and the production dependency audit.

| Environment | Runtime | Passed | Skipped | Failed |
| --- | --- | ---: | ---: | ---: |
| Native Windows ARM64 | Node 24.18.1, SQLite 3.53.1, Git 2.54 | 363 | 2 | 0 |
| Isolated Linux ARM64 container | Node 24.22.2, SQLite 3.53.4, Git 2.39.5 | 349 | 16 | 0 |

Windows skips the two POSIX permission cases. Linux skips fifteen Windows-specific cases and one relative-worktree-pointer case because its installed Git lacks that behavior. Both POSIX permission cases pass on Linux, and the relative-pointer regression passes on Windows. All 80 generated files under `dist` and `schemas` match byte for byte between the two builds. CI is configured for Windows, Linux and macOS; this session did not execute macOS.

Seven separate CI-helper tests pass on both platforms, and both workflow files pass actionlint. They cover fixed shell-free workload arguments, invalid inputs launching no child, exact exit propagation and generated-file checks against real Git fixtures, including staged, untracked, ignored and redirected-index cases. The optional endurance workflow requires manual dispatch, runs one selected OS/workload, and uploads only allowlisted logs. No remote CI workflow was triggered in this session.

The suite uses real local processes, Git worktrees, binary patches and SQLite databases, with offline fake vendor protocols. It includes an official MCP SDK client connection and an installation of the release tarball from an empty npm cache using npm's offline mode. Unicode paths and spaces are included. No production npm dependencies are installed.

## Previous-release upgrade rehearsal

Actual preserved local plugin ZIPs were extracted into isolated Windows fixtures and used to generate historical state with offline runners. Both rehearsals exercised all six read modes and one write artifact containing tracked, untracked and binary changes.

| Starting release | Schema migration | Exactly preserved runs / events / messages / context / leases |
| --- | --- | --- |
| 1.0.0 | 2 → 5 | 7 / 63 / 2 / 1 / 1 |
| 1.1.0 | 3 → 5 | 7 / 64 / 2 / 1 / 1 |

Each rehearsal confirmed live-old-host refusal before schema advancement, exact record/event and mail-acknowledgement preservation after shutdown, unchanged artifact base and patch hashes, a successful Git patch apply-check, reviewed cleanup and old-binary refusal after migration. Integrity returned `ok`; the original fixture checkout and historical ZIP hashes remained unchanged. The rehearsals use sampled historical state; separate fault regressions cover specific interrupted writes and failed migrations.

## Fault and boundary coverage

- Cross-process call admission, reserved queue capacity, cancellation races, dead-owner recovery, scoped leases and crash-interrupted maintenance.
- Real SQLite writer contention, cold concurrent initialization, failed evidence writes, disk-full rollback and future-schema refusal without modifying unsupported state.
- Native/ACP/ACPX result validation, bounded JSON graphs, malformed UTF-8, oversized frames, backpressure, closed sessions and blocked output.
- Windows short-name metadata aliases and case-sensitive NTFS root identity; POSIX database/sidecar permissions.
- Git environment redirection, independent applicable patches, nested projects, dirty submodules, retained partial evidence and cancelled atomic publication.
- Owned descendant termination after cancellation and abrupt parent death, plus explicit diagnostics when OS cleanup cannot be verified.

Sustained testing found and regressed several actual faults: transient heartbeat contention permanently degraded a healthy host; concurrent cold startup acquired unnecessary migration locks; Windows sharing refusals interrupted atomic publication; a one-second native kill-helper budget could leave a Git filter running after cancellation returned; and foreign writes could repeatedly restart an incremental backup until its deadline. The corresponding regressions cover bounded retries, current-schema startup, cancellable atomic publication, ordered process-tree cleanup and a dedicated pinned backup snapshot. Functional ACPX fixtures allow cold startup independently of the dedicated deadline tests, which retain their explicit bounds.

## Final-build endurance

These offline workloads capture or verify unchanged compiled inputs and record their version and digest. Successful fixtures are removed only after drain and integrity checks; failures preserve their fixtures for investigation.

| Workload | Duration | Result |
| --- | --- | --- |
| Two embedded services and two real MCP clients | 64 minutes | 8,043 workflows, including 639 deliberate cancellations; 96 integrity-checked backups across five isolated state fixtures; maximum observed call concurrency three; final active runs, instances, slots, waiters, leases and unread mail all zero. |
| Storage coordination, four writers and an observer | 30 minutes | 33,069 workflows: 28,344 completed and 4,725 cancelled; 60 integrity-checked online backups; final active runs, instances, slots, waiters, leases and unread mail all zero. |
| Git artifacts across parallel, review and pipeline modes | 30 minutes | 183 cycles, 61 per mode; 36 cancellations; 232 nonempty applicable patches; no retained subprocess resources after each cycle. |

The storage harness retains its latest two verified checkpoint copies and observed a peak logical fixture size of 106.3 MB against a 256 MiB stop budget. The artifact fixture peaked at 11.1 MB against 128 MiB. These are development-fixture budgets and observed sizes, with filesystem and timing qualifications described in the [performance record](performance.md).

The service workload exercises all six modes, durable cancellation, complete paged mail, scoped leases, project context, retention and repeated MCP client replacement. History is trimmed at checkpoints and the fixture changes every fifteen minutes, so this establishes sustained behavior under that retention policy.

An earlier captured build stopped after twenty minutes and 37 successful backups when continuous foreign writes prevented the incremental copy from finishing within its deadline. That failure and its database were preserved. The final repeat used a pinned read snapshot, grew its database to 25.1 MB and completed all sixty backup checks while writers continued. Earlier captured-build results are kept separately from final-build qualification.

## Archive checks

The local release produces a self-contained plugin ZIP, an npm tarball and external SHA-256 checksums. Two consecutive builds with unchanged inputs and toolchain reproduce each archive's hash. Independent Python verification checks ZIP CRCs, duplicate/unsafe paths, required runtime helpers and operational documentation, then runs the extracted CLI version and integrity health commands. Private state, test fixtures and development dependencies stay outside the runtime archive.

The complete platform suites also install the tarball from an empty npm cache using offline mode and connect to its MCP entrypoint with the official SDK client. Local packaging does not publish to npm or GitHub; registry publication remains a separate operation.

## Dependencies and deployment limits

The production audit reports zero vulnerabilities in both environments. Development dependencies include the official MCP SDK for interoperability tests. The Windows development audit reported nine affected packages (four high, five moderate); Linux npm reported four (two high, two moderate). Those development packages are excluded from runtime installation. Required patched transitive versions were unavailable in the checked registry, so this release does not force incompatible major-version overrides or claim a clean development dependency tree.

A native Codex smoke check on Windows completed one read-only delegation using the explicit model `gpt-5.6-sol`, the normal executable resolver and the process supervisor. It returned the requested `COUNCIL_SMOKE_OK` response in 24.2 seconds. The task requested no tools or file inspection. That initial release check found Codex and Claude executables and did not verify the other live integrations. The requested follow-up above records the later Claude, Junie and Antigravity results and the explicit DSH exclusion.

Provider authentication, model access and quotas remain native to each installed agent. Fake protocol tests do not establish a live five-vendor collaboration session. Read policy and worktree isolation retain the deployment boundaries documented in the [operations runbook](operations.md), including trusted local trees and OS-level filesystem/process limits.

The [performance record](performance.md) describes reproducible payload and query fixtures, including timing qualifications. Development endurance tools create their own temporary state and never prune a user's existing workspace.
