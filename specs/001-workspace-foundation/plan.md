# Implementation Plan: Workspace foundation

**Branch**: `refactor` | **Date**: 2026-11-05 | **Spec**: [spec.md](spec.md)

## Summary

Deliver project onboarding, one saved autonomy preference, honest five-harness installation
discovery and versioned task drafts through one Ejupi Labs UI and CLI. No agent execution.
Replace legacy source only after this specification, design and tasks are committed.
Reference main remains recoverable at `27822baa3aee663366a870bde7fb1190649d3418`; legacy data
and frozen release archives remain untouched.

## Technical context

| Concern | Decision |
| --- | --- |
| Core | Go1.27.3; module github.com/Ejupi-Labs/council/v2; application2.0.0-dev |
| UI | Authored strict TypeScript ES modules, HTML/CSS, system reading font; Go embed of compiled browser assets |
| Runtime | Go standard library; modernc.org/sqlite v1.61.1 with matching libc v1.79.0; x/sys v0.49.0; locked go.sum |
| Storage | Bundled SQLite3.54.1; private user-home/.council/v2; one serialized writer connection |
| JSON | Stable encoding/json/v2 + jsontext strict defaults and RejectUnknownMembers |
| Transport | Standard net/http, 127.0.0.1 ephemeral port, private bearer APIv1, embedded assets |
| Distribution | Native binary; release users need no Go, Node or npm development installation |
| Development | Node24.18.1+ and pinned TypeScript/Playwright only; meaningful npm test/check wrappers |
| Tests | Go test/vet, strict tsc, fake discovery, real-browser acceptance; race on supported Linux CI |
| Targets | Windows/macOS/Linux ARM64/AMD64; runtime qualification recorded separately from cross-compilation |

Autonomous full access is the visible requested default, not a claim about effective harness
permissions. Setup makes zero model calls. The language decision makes no token-saving or
unmeasured runtime-performance claim. See [research.md](research.md) for alternatives/sources.

## Constitution check

Pre-design and post-design gates pass by design; acceptance evidence remains pending.

1. Specification/design/tasks committed before source replacement; ordered feature delivery.
2. Five installation observations distinguish unknown readiness; no credential read/vendor call.
3. Deterministic setup, no model classifier, polling or review call.
4. One saved autonomy preference, Auto drafts, no transport/voting questionnaire.
5. Draft receipts mean saved preparation only; no invented attempts or accepted changes.
6. OS owner lock precedes every database open; UI/CLI share services and durable records.
7. Approved brand/notices, actual browser review, failure tests and independent review before002.

No constitution exception. Branch AGENTS permits stack changes and retains npm test/check.

## Architecture

CLI/HTTP delegate to app services, which use store/project/discovery. Localstate owns privacy,
native lock, database lifetime and owner transport. There is no scheduler/plugin framework,
account database or task graph built speculatively in001.

UI is a foreground persistent owner, surviving browser closure. With no UI owner, a CLI command
holds transient exclusive ownership; with one, it verifies the private descriptor through
authenticated meta and forwards. A lock held by an unreachable/incompatible process yields a
clear owner error; never kill it or open a competing store. Use LockFileEx/flock on a persistent
file, not PID or lock-file deletion. Publish descriptor atomically and remove only own instance.

Before state/token writes verify Unix0700/0600 or Windows protected DACL current owner, SYSTEM,
Administrators. chmod is insufficient on Windows. Operate only on owned v2 scope, never legacy
data. Never inspect/copy vendor credentials. No hostile multi-user OS sandbox is promised.

One dedicated database connection applies foreign_keys, busy_timeout and transactions. Revision
and sequence values are checked signed int64, decimal strings at API boundary. Receipt/fingerprint,
draft version and pointer commit atomically. Replay precedes expectedRevision conflict checks.
Never report success on failed/uncertain commit; retry same request identity. Unknown schemas
fail without recreating the store. No external I/O inside transactions.

Project resolution uses explicit basePath, canonical paths and physical file identity; never
Windows case-fold deduplication. Persist physical evidence and reject replacement folders.
Shell-free Git rev-parse/symbolic-ref uses --no-optional-locks, disabled fsmonitor and sanitized
Git redirection environment. No status scan, index refresh, hooks, staging, branch switch or
source change. Existing open keeps admission metadata and returns a separate fresh observation.

Discovery inspects filesystem/native install metadata, never launcher contents or invocation.
Shell-only wrappers are unsupported. Errors differ from missing installations. Read-only refresh
creates no persisted snapshot/revision. Account/model/allowance/external occupancy stay unknown.

Exact numeric Host/port before routing; bearer for all private API requests; exact owner Origin
on every mutation including CLI. Foreign supplied Origin rejects private reads. No redirects,
cookies, external assets or permissive CORS. Fragment token is removed and retained only in
session storage. Private responses no-store; bounded diagnostics omit tokens/task text/raw output.
Strict request validation rejects unknown/duplicate JSON/query fields, malformed Unicode and
oversized input before admission. Text cap4MiB and body cap32MiB reject without clipping.

UI guards read responses with resource/edit generations, preserves input on save/read failure,
retains mutation IDs for unresolved retries and exposes conflicts without erasing edits. Display
Draft / Not run and “Task execution is unavailable in this preview”, with no feature IDs in UI.

## Project structure

```text
cmd/council/main.go
internal/domain/       # neutral shared DTOs, errors, exact validation
internal/app/          # service operations, native conversion and aliases
internal/store/        # SQLite transactions, replay, exact cursors
internal/project/      # identity and read-only Git metadata
internal/discovery/    # five filesystem-only observations
internal/localstate/   # privacy, OS lock, descriptor, owner/client
internal/httpapi/      # contract, authority checks, UI delivery
internal/cli/          # parsing, forwarding, human/JSON output
internal/webui/embed.go
internal/webui/dist/   # committed compiled browser/public assets
ui/src/               # TypeScript, HTML/CSS, approved asset subset
ui/tests/             # browser acceptance
scripts/              # development/build/check/package wrappers
.github/workflows/    # new meaningful matrix checks
third-party/          # retained licenses and brand provenance
specs/001-workspace-foundation/
  spec.md plan.md research.md data-model.md quickstart.md tasks.md contracts/
```

## Source replacement and packaging

After design commit, optionally archive reference27822ba under ignored scratch. Retire explicitly
tracked legacy source/tests/dist/schemas/manifests and obsolete implementation docs by relocation
to reference/legacy/. Bulk deletion was rejected by automatic review; preserve the reference
instead. It is excluded from compilation and release allowlists.
Preserve Git, all ignored state, requirements/Specify, notices and original release archives.
Never recursively clean an unverified computed path. Replacement commands retain npm test/check.
Compile UI before native build; commit embedded browser assets, ignore native build outputs.
Native release uses explicit file allowlist/checksum manifest/notices. An extracted preview must
start without development dependencies; it is not a completed2.0 product or npm runtime wrapper.

## Validation and handoff

[Quickstart](quickstart.md) and [tasks](tasks.md) cover replay/rollback/uncertain commit,
Unicode/limits, exact revisions/cursors, actual OS exclusion/privacy, physical aliases/replacement,
Git state preservation, no vendor invocation, API authority, UI/CLI restart parity, stale UI edits,
brand/keyboard/narrow rendering and packaged offline startup. Review closes blocking findings and
maps every WS requirement to evidence before002. All changes stay on refactor; main unchanged.

## Complexity tracking

Native ownership/privacy and transactions protect durable private work. A single core with plain
compiled UI avoids a separate frontend service or desktop browser runtime.
