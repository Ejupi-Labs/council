# Workspace foundation acceptance

Date: 2026-11-05. Branch: refactor. Scope: feature001 only.
Result: implemented and reviewed, with native runtime CI acceptance on Windows, macOS and Linux,
each AMD64/ARM64, at implementation commit93c8c2530f99ff97f35e7c55180df9b42d97fcb6.
Application2.0.0-dev / API1 / Go1.27.3 / bundled SQLite3.54.1.

## Observed checks

| Check | Observed result |
| --- | --- |
| npm run build | Passed; independent native executable with compiled embedded TypeScript UI |
| npm test | Passed: all Go cmd/internal/script packages; five real Chromium browser journeys; one native archive/extracted-offline journey |
| npm run check | Passed: go vet, strict TypeScript and non-mutating Go formatting check |
| actionlint .github/workflows/verify.yml | Passed |
| npm run release:local | Passed; allowlisted Windows ARM64 preview and SHA256 manifest |
| Native lock/privacy/project/discovery scoped tests and vet | Passed on actual Windows ARM64 |
| Native package test-binary cross-compilation | Passed for Windows/macOS/Linux, AMD64/ARM64; compile evidence only |
| [Hosted runtime CI42115752948](https://github.com/Ejupi-Labs/council/actions/runs/42115752948) | Passed all six native platform jobs plus Linux AMD64 race checks against93c8c25 |
| Independent correction rechecks | All eight CLI/HTTP findings and both reproduced UI findings closed; see review.md |

The reproducible checks are committed tests. Raw local logs, exit markers and inspected renders
are retained under ignored .council/refactor/ (001-npm-test.log,001-npm-check.log,screenshots/).
Tests use temporary Git projects, fake installation metadata and local test servers, not providers.

## Project opening

CLI and browser open paths with spaces/Unicode, retain a selected subdirectory separately from its
repository root, resolve physical aliases without case-fold guesses, handle unborn/detached HEAD
and reject changed physical identity. Tests compare original staged/index/source/branch evidence;
no source or index mutation, hook invocation, fsmonitor invocation or Git redirection escapes occur.
Reopening preserves admission metadata and returns separately timestamped fresh observations.

## Harness readiness and authentication boundary

Exactly five ordered harness entries are returned. Controlled metadata covers installed, missing,
shell-wrapper-only and failed inspections. Finding a launcher does not run it. Invocation sentinels
remain untouched; no vendor credential or global settings are read or copied. Auth/model/provider/
account/allowance/usage/effective permissions/external occupancy remain unknown. Every adapter says
managed execution not implemented; no decorative card is qualified as an execution integration.

## Durable drafts and client parity

UI-created drafts are read by CLI, then survive owner restart and UI reopening. CLI task text keeps
complete valid UTF-8, BOM markers, literal U+FFFD, leading/trailing whitespace and CRLF. Native
browser textarea newline normalization is distinguished from server preservation; the saved text
matches exactly what the browser submitted. Saved versions and their policy snapshots remain
immutable. Explicit useCurrentPolicy affects only a new version. Draft/Auto never implies a run.

Storage tests prove receipt and state atomicity, immutable history, stale-write conflicts, known
rollback and lost-ack reconciliation. Exact revisions/cursors above2^53 remain strings/int64, with
overflow/REAL promotion rejected. Unsupported schemas and malformed databases fail without reset.
An uncertain save retains its ID and is reconciled without creating duplicate work.

Browser tests delay reads while editing, provoke concurrent stale saves and reconcile explicitly;
refresh failure preserves the composer/selection. The reproduced lost-ack→401→unverified→reconnect
case now repeats three identical POST payloads and leaves one saved draft, preserving newer input.
An explicit delayed policy read cannot overwrite a newer choice. Both fixes were independently
rechecked against the corrected implementation.

## Local state boundary and authority

New state uses private user-home/.council/v2, separate from legacy state. Actual Windows subprocess
tests prove LockFileEx exclusion and lock release, protected DACL scope, private descriptor
publication and owner-matched removal. PID checks/descriptor files do not authorize takeover.
UI and transient CLI share services; active-owner CLI forwards instead of opening another writer.

Contract tests reject unauthenticated private GET, wrong Host, foreign/null/missing mutation Origin,
duplicate/unknown members/queries, invalid Unicode, unexpected methods and oversized text/body.
Valid literal replacement/BOM characters survive. Errors are bounded English and avoid task/token
or raw subprocess disclosure. Receipt-less/malformed acknowledgements cannot report Saved.

## Brand and accessible flow

Inspected actual1440×1000 and360×900 viewports plus full-page renders. Approved company vector
geometry and exact palette are retained, with240px logo and MIT filled icon family. Root verified
logo SHA256d697a045d059366aaeafe7c2a9a8f5f4a589c0c698e5e7c32757d5b1c8e81c7a and symbol
SHA25687bb971fa3b8e5f484d149b93b60fff068c6bdd161635845fe22f9ac740f7bf9 against shipped assets.
Notices/provenance are included; no private brand documents or invented Council identity is shipped.

Browser acceptance verifies no horizontal overflow at360px, named controls, keyboard skip-to-task,
focus and keyboard save, reduced-motion mode, readable non-colour feedback and preserved errors.
The offscreen skip-link screenshot artifact was corrected and renders reinspected. This is bounded
flow verification, not a claim of comprehensive assistive-technology certification.

## Extracted release

Archive has exactly seven allowlisted entries: native binary, preview README, Council LICENSE,
brand provenance, icon license, compiled Go/dependency notices and SHA256SUMS.txt. Each file and
archive checksum is independently checked. Fixed input produces identical ZIP bytes. No reference
tree, .council data, source task text, developer workspace path or credentials are packaged.

An extracted executable runs version, initializes private policy state and serves embedded UI from
another working directory with an empty PATH: no Node, Go, Git or development packages available.
Project inspection normally requires Git. Build/release generation is development-only.
Latest local archive: council-2.0.0-dev-windows-arm64.zip, SHA256
65a30fdff02d46d5f1ac0b661db2d09afea3f3a03628643b1822d1d87356af91.
This unreleased preview is not an npm publication or the completed product release.

## Requirement evidence

| Requirement | Evidence |
| --- | --- |
| WS-001–002 | internal/project/project_test.go; app workspace tests; browser workspace journey; project-opening section |
| WS-003–004 | policy persistence/revision/snapshot tests, browser restart/policy-race journey; no effective-capability claim |
| WS-005–006 | internal/discovery/discovery_test.go; browser agents refresh journey; harness-readiness section |
| WS-007–008 | app/store draft tests; browser draft/restart journeys; exact text, immutable history and Draft/Not run |
| WS-009–010 | pinned asset hashes/notices, inspected renders and keyboard/narrow/reduced-motion browser journey |
| WS-011 | single owner, cross-interface/restart, stale/conflict/replay/lost-ack regressions |
| WS-012 | native privacy/exclusion tests and archive allowlist; separate untouched legacy state |
| WS-013 | internal/httpapi/server_test.go authority/method/query/body tests |
| WS-014 | CLI parser/acknowledgement/panic tests and injected storage rollback/uncertainty; reviewed failure copy |
| WS-015 | extracted empty-PATH native startup plus meaningful npm test/check wrappers |
| SC-001–007 | all sections above, independent review.md and checked executable tasks.md |

## CI correction follow-through

Initial run42110748901 passed Linux ARM64 and Linux race coverage, and exposed macOS/Windows
ancestor-alias spelling assumptions. Private ownership now resolves legitimate ancestor aliases
while validating an ordinary same-file v2 leaf. Both owner and project inspection share native
canonical-path metadata handling. A real Windows junction project regression failed before the
fix and passes after it; invalid physical-path Unicode is rejected rather than replaced.
The corrected complete npm test/check suite and extracted-package journey pass on Windows ARM64.
Six affected target test binaries compile. Corrected runtime CI remains pending and is not counted
as qualification here. See review.md for independent diff inspection and the bounded correction.

Run42113529269 subsequently passed both macOS and both Linux native jobs plus Linux race
coverage. Its Windows ARM64 runner exposed the elevated-token default-owner group case. The
follow-up recognizes only the exact current-token owner-eligible default group and normalizes
ownership to the current user before private writes, alongside the protected DACL. Foreign-user,
SYSTEM and unrelated/deny-only group ownership remain rejected. Native/policy tests and complete
local npm test/check pass; corrected elevated-host CI is separately pending.

The same correction includes UI/server Unicode White_Space parity. Actual browser acceptance
saves U+FEFF-only text and retrieves it exactly through the CLI.

## Remaining product work

No agent task, automatic model routing, account capacity claim, verified integration or MCP host
interface is delivered by001. All five live adapters remain unqualified. Six-platform native CI and Linux AMD64 race checks passed in run42115752948.
Native Windows ARM64 race detection is unsupported and was not claimed as a pass.
Feature002 may begin after the reviewed foundation commit. The full rebuild goal remains active.
