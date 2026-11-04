# Version 1.2 verification record

This record describes local release verification on 3–4 November 2026. Council targets trusted local workspaces under one operating-system account. These results establish the behavior of the tested builds and fixtures; they do not certify a hosted service, an OS sandbox or every vendor account configuration.

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

A native Codex smoke check on Windows completed one read-only delegation using the explicit model `gpt-5.6-sol`, the normal executable resolver and the process supervisor. It returned the requested `COUNCIL_SMOKE_OK` response in 24.2 seconds. The task requested no tools or file inspection. A fresh prerequisite check found Codex and Claude executables; DeepSeek Harness, Junie and Antigravity require installation or explicit executable configuration on this machine. Live Claude, DeepSeek Harness, Junie and Antigravity account integration remains unverified in this session.

Provider authentication, model access and quotas remain native to each installed agent. Fake protocol tests do not establish a live five-vendor collaboration session. Read policy and worktree isolation retain the deployment boundaries documented in the [operations runbook](operations.md), including trusted local trees and OS-level filesystem/process limits.

The [performance record](performance.md) describes reproducible payload and query fixtures, including timing qualifications. Development endurance tools create their own temporary state and never prune a user's existing workspace.
