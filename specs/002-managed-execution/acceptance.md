# Managed execution acceptance

Date: 2026-11-06. Branch: refactor. Application 2.0.0-dev.2 / API 1 / execution contract 002-v1 /
schema 2. Go 1.27.3, embedded SQLite 3.54.1, Node 24.18.1 development tooling and Chromium
151.0.7922.34 through Playwright 1.62.0.

Status: implemented, independently reviewed and accepted on all six native platforms plus Linux
AMD64 race checks at implementation f5edd96cbe32a1cb78d01d9b1107205c5abd8a00. Complete local
regression, visual inspection and the active-row correction recheck also passed.
This record does not qualify an installed Codex binary or a provider account. All execution tests
use explicitly selected local fakes and owned native process fixtures, with zero live model calls.

## Observed checks

| Check | Result |
| --- | --- |
| npm run build after final persistence corrections | Passed on native Windows ARM64 |
| Context, physical-connection settings, literal database path and hot WAL recovery checks | Passed; focused store batch 0.381s and final context follow-up 0.377s |
| Independent immutable final-publication, acknowledgement-loss and cancellation checks | Passed; six focused cases, 0.197s |
| Admission acknowledgement loss, joined faults, pre-preparation Stop and exact-owner shutdown | Passed; five engine tests, 3.755s |
| Actual concurrent starts from two independent CLI processes | Passed; native Windows ARM64, 25.012s package run, one owner/slot and ordered Stop-driven worker transfer |
| npm test against final source | Passed: all Go packages, fourteen real Chromium cases and the extracted native preview; CLI 56.765s, engine 55.437s, browser 60.394s and package 10.146s |
| npm run check against final source | Passed: go vet, strict authored TypeScript and non-mutating Go format validation |
| Actual execution desktop and 360px renders | Passed; eighteen inspected captures, six transition renders rechecked after correction |
| actionlint .github/workflows/verify.yml | Passed |
| [Hosted run 42140818152](https://github.com/Ejupi-Labs/council/actions/runs/42140818152) | Passed all six native Windows/macOS/Linux AMD64/ARM64 jobs and Linux AMD64 race checks at f5edd96 |

An intermediate full run exposed foundation CLI acquisition returning a non-busy error before
forwarding to a healthy owner. The Windows retained-root/MAXIMUM_ALLOWED conflict is corrected
with strict read-only directory reuse and an effective OICI inheritance guard. Its native focused
suite passed in 2.449s and all four affected browser/CLI journeys passed in the final complete run.
An earlier engine fixture used a safety fence to pause scheduling; it now stops the scheduler
explicitly. The final complete run includes that correction and the actual concurrent CLI case.

Earlier complete browser/package runs and intermediate failed runs are diagnostic history, not
substitutes for the final gates. Raw local logs, diagnostic benchmark results and screenshots are
kept under ignored .council/refactor/. Tests and the substantive review findings are versioned.

## Hosted qualification history

[Run 42138479256](https://github.com/Ejupi-Labs/council/actions/runs/42138479256) tested
implementation 6dffccf293ecf26846c39268704f2ed257780ea9. All seven jobs completed with failures:
Linux AMD64/ARM64 and the race job exposed mutable-response file-identity reuse; both macOS
architectures exposed guardian cleanup proof failures; both Windows architectures exposed the
150-character Git-v4 fixture's dependence on a developer long-path setting. These are actual
runtime failures, not successful cross-compilation evidence.

The corrections retain response file handles until import/release, require actual post-reap group
absence independently of signal-delivery status, and explicitly enable long paths only in the
isolated Git fixture. Independent source review found no remaining blocking issue. Evidence and
process test binaries compile for all four Unix targets; this is compile-only. The Windows Git
fixture passed with a deliberately longer temporary root (0.667s). The corrected complete Windows
ARM64 regression passed, including all fourteen browser cases and extraction. The corrected-head
[run 42140818152](https://github.com/Ejupi-Labs/council/actions/runs/42140818152) then passed every
native and race job at f5edd96, directly qualifying these runtime paths. The earlier failed run is
retained as correction history, not acceptance evidence.

## Task admission defaults

The ordinary UI and CLI path takes a project and complete task text, reuses the saved autonomous
policy and uses the native configured model selection. Optional pins/details remain optional.
Task, immutable task version, attempt and mutation receipt are distinct. One explicit attempt
starts one worker; observation, retries of an existing receipt and reconnect never add workers.
The connection-wide capacity is one across projects and clients, including fake/native modes.

The CLI starts or forwards to one exact-self continuing owner. An observing command, browser or
bounded wait can end while that owner continues. A service Stop targets its exact owner identity;
replaying a previous owner's shutdown receipt cannot stop a replacement owner.

## Original instructions and guidance

Keep original Unicode instructions, source-draft reference, guidance, acceptance conditions, pins
and captured policy separately from generated output. User revision and asynchronous state revision
are separate. Guidance creates a new version and prevents an obsolete queued version from launching.
Changing global preferences alone does not rewrite a task snapshot. Browser Start retains separate
autosave/start request identities, the clicked text and newer composer edits. Replaying an older
autosave acknowledgement never rewinds a newer saved revision.

Storage accepts original text up to 4 MiB; dispatch blocks an assembled prompt above 256 KiB while
preserving its original instructions. Limits never turn a clipped instruction into a complete run.

## Effective execution policy

The native invocation uses exec JSONL, ephemeral mode, danger-full-access and command-scoped
approval_policy="never", with prompt on stdin and a private last-message target. No global vendor
configuration or credential is inspected or changed. An explicit model selector is passed exactly;
unsupported account/payment/harness or supervision requests block without substitution. Requested
selection and reported effective facts remain separate; unreported model/account/usage is unknown.
The capability says native supported, live unqualified. No API billing fallback is configured.

## Permission capabilities and limits

Windows ownership uses a suspended root, restricted inherited handles and an atomic owned job.
Unix ownership uses a same-executable guardian, READY/GO and an unreaped process-group anchor until
the final scoped termination effort. Cleanup records declare their actual scope. Descendants that
escape a Unix group or otherwise cannot be proved quiescent remain an explicit limitation; no
universal OS sandbox, PID-based takeover or post-reap group signalling is promised.

Native stdin proof records expected/written byte counts and joined pipe completion. Success
requires complete retained delivery evidence in addition to root exit, protocol terminal and usable
output. Kernel pipe acceptance is not proof that a model understood or obeyed the instructions.

## Preflight and destination

Capture the current selected project identity, base, meaningful staging and dirty/unignored source.
Managed worktrees are constructed without checkout hooks or external filters; original source,
index and branch stay unchanged. SHA-1 Git indexes 2/3/4 and documented all-zero skipHash checksums
are supported. Split/sparse/unmerged/intent-to-add/submodule/unborn/unsupported object formats and
unsafe or oversized source fail with a specific blocker. No older clean base silently replaces
dirty input. Candidate deltas compare against captured source and staging rather than only HEAD.

Candidates remain verification pending and integration not performed. A successful authoring
attempt does not complete a write task. For a response-only task, a separate explicit reviewed
acceptance binds the latest current-version successful response, complete fresh hash/identity and
confirmation. Authored file/index changes or outstanding checks prevent this shortcut. Registered
corruption fences dispatch; an unknown or mismatched user-supplied acceptance ID does not.

## Evidence, cancellation and recovery

Artifacts retain bounded exact bytes with decimal offsets and fixed-through reads; the UI streams
UTF-8 across chunk boundaries and offers exact-byte download. Source manifests, candidate metadata,
diagnostics and incidents are inspectable. Internal complete-file copying hashes the bytes actually
copied in one pass. Cached public range proofs verify returned aligned chunks against trusted
digests; their expected whole SHA is not a claim of fresh verification of unrelated current bytes.
Response acceptance always performs fresh complete-file verification.

Full limits: stdout 64 MiB, stderr 16 MiB, JSONL line 1 MiB, event 16 KiB, chunk 64 KiB; source
100,000 entries, 64 MiB per file and 512 MiB total. Ordinary attempts have an absolute admission
deadline of 1,800 seconds, optionally 1–86,400 seconds. Limits and absent evidence stay explicit.

Stop acknowledgement remains distinct from settlement. Ordered primary failure/Stop survives late
success and later faults; secondary diagnostics are retained without rewriting primary authority.
The 15-second termination watchdog includes evidence settlement, exposes an unresolved incident
and holds capacity if cleanup cannot be proved. Filesystem operations are not hard real time.

SQLite WAL/FULL and per-connection foreign keys/FULL/busy-timeout preserve durable receipts after
physical connection replacement. Immutable bytes sync/hash/close before one final publication;
streamed prefixes keep their checkpoints. Known precommit cancellation remains distinct from
uncertain commit acknowledgement. Genuine joined file/commit faults stay uncertain. Memory fences
close new admission even when the incident itself cannot be committed. Recovery preserves terminal
records and artifacts, marks unfinished work interrupted, and never reruns a model to recreate data.

Private state remains under user-home/.council/v2, separate from legacy state and release inputs.
Existing journal/WAL/SHM files are checked without deletion; hard-linked files are rejected before
permission mutation. Windows-created SQLite companions inherit the restricted directory ACL.
Crash recovery tests exercise abrupt process exit with a hot WAL, not a simulated power failure.

## Brand and accessible flow

Inspected eighteen actual 1440×1000 and 360×800 captures covering composer, response, acceptance,
running, cancelling, cancelled, partial failure and blocked Start. Eight keyboard focus checks
exercise Start, read, review, accept and Stop using Tab/Enter/Space, with visible 3px Lilac focus.
No horizontal overflow or clipped primary action was observed; reduced-motion computes auto/none/
0s. All ten approved assets match source, embedded output, served bytes and the pinned original.
The active-list phase correction has a passing dedicated real-browser regression; all six updated
transition renders were reinspected with list/detail agreement. Root independently reviewed its
phase-only refresh and stale-generation guards; unchanged-phase output adds no list request.

Approved identity and notices are inherited from feature 001; no alternate logo or icon family is
introduced. This bounded acceptance is not a comprehensive assistive-technology certification.

## Requirement evidence

The following implemented test locations map to the passed final checks above. File consolidation
from the planned task filenames is recorded in review.md; no missing planned filename is treated
as evidence by itself.

| Requirement | Primary executable evidence |
| --- | --- |
| EXE-001 | engine/results_test.go, cli/execution_integration_test.go, ui/tests/execution.test.mjs |
| EXE-002 | store/tasks_test.go, cli/execution_test.go, ui/tests/execution.test.mjs |
| EXE-003 | store/tasks_test.go, engine/admission_test.go, ui/tests/execution-autosave-recovery.test.mjs |
| EXE-004 | engine/admission_test.go, engine/fault_test.go, store/control_test.go |
| EXE-005 | store/tasks_test.go, engine/admission_test.go, ui/tests/execution-version.test.mjs |
| EXE-006 | adapters/codex_test.go, discovery/discovery_test.go, native CLI source/argv review |
| EXE-007 | adapters/codex_test.go, domain/execution_validation_test.go |
| EXE-008 | adapters/codex_test.go, engine/admission_test.go |
| EXE-009 | adapters/codex_test.go, store/results_test.go |
| EXE-010 | workspace/workspace_test.go, workspace/index_v4_test.go, engine/workspace_test.go |
| EXE-011 | engine/results_test.go, workspace/workspace_test.go, ui/tests/execution.test.mjs |
| EXE-012 | store/results_test.go, engine/results_test.go, ui/tests/execution-failure.test.mjs |
| EXE-013 | store/results_test.go, engine/fault_test.go, ui/tests/execution-autosave-recovery.test.mjs |
| EXE-014 | evidence/evidence_test.go, evidence/proofs_test.go, ui/tests/execution-bytes.test.mjs |
| EXE-015 | adapters/codex_test.go, ui/tests/execution.test.mjs |
| EXE-016 | engine/results_test.go, ui/tests/execution-failure.test.mjs |
| EXE-017 | owner/owner_test.go, cli/concurrent_start_integration_test.go, engine/results_test.go |
| EXE-018 | store/tasks_test.go, engine/mutation_ack_test.go, UI autosave/recovery tests |
| EXE-019 | store/control_test.go, engine/control_test.go, cli/execution_integration_test.go |
| EXE-020 | process/process_windows_test.go, process/process_unix_test.go, engine/control_test.go |
| EXE-021 | store/control_test.go, store/secondary_faults_test.go, engine/results_test.go |
| EXE-022 | engine/fault_test.go, engine/recovery_test.go, engine/observation_regression_test.go |
| EXE-023 | engine/fault_test.go, engine/mutation_ack_test.go, evidence/immutable_ack_test.go |
| EXE-024 | store/recovery_test.go, engine/recovery_test.go, cli/execution_integration_test.go |
| EXE-025 | cli/execution_integration_test.go, owner/owner_test.go, UI execution tests |
| EXE-026 | engine/observation_regression_test.go, discovery/discovery_test.go, CLI/browser fixture observations |
| EXE-027 | ui/tests/execution-autosave-recovery.test.mjs, ui/tests/execution-version.test.mjs, ui/tests/execution-live-list.test.mjs, inspected renders |
| EXE-028 | localstate/recovery_test.go, localstate/protection_windows_test.go, localstate/protection_unix_test.go, scripts/package.test.mjs |
| EXE-029 | fixture package, process fixtures, CLI/browser/package explicit-fake journeys |

Go paths above are relative to internal/; browser/package paths retain their stated prefix.

| Success criterion | Evidence grouping |
| --- | --- |
| SC-001 | Ordinary task admission defaults; UI/CLI explicit-fake journeys |
| SC-002 | Immutable original/guidance/pins and invalid-preflight tests |
| SC-003 | Dirty source/index/candidate comparisons and separate response acceptance |
| SC-004 | Native termination, ordered controls and absolute watchdog tests |
| SC-005 | cli/concurrent_start_integration_test.go passed actual barrier-released independent CLI starts, one owner/slot and persisted nonoverlapping lifecycles; receipt reconciliation is separately tested |
| SC-006 | Commit/evidence/native-owner faults, interrupted recovery and no replay |
| SC-007 | Continuing owner, bounded reads/waits and explicit byte-backed evidence |
| SC-008 | Real browser stale-state tests and final inspected brand/keyboard renders |
| SC-009 | This matrix, review.md, final local/hosted gates and explicit live qualification limit |
