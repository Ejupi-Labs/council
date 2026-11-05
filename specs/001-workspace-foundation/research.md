# Architecture decisions: Workspace foundation

Date: 2026-11-05. Feature: `001-workspace-foundation`. Git branch: exactly `refactor`.
Status: design decisions; implementation, measurements and live-provider qualification remain pending.
Inputs: product vision, functional requirements, constitution, verified design identity, discovery
record, this feature's specification, data model and foundation contract.

## ADR-001 — Native owner and an embedded browser interface

**Decision:** Use Go 1.27.3 for the replacement core, CLI, local HTTP service and persistence owner.
Author the browser UI in TypeScript with ordinary HTML/CSS; compile its JavaScript for embedding
in the release executable. The running product has no third-party npm dependencies, npm install,
Node runtime, compiler, external asset server or frontend framework requirement. Development-only
TypeScript/browser verification tooling is pinned separately and does not enter runtime archives.

Go's standard HTTP, process, filesystem, cryptographic and embedding facilities fit the bounded
local product. `CGO_ENABLED=0` release builds and a pure-Go SQLite driver avoid distributing a C
toolchain or separate SQLite library. This is an installation/maintenance decision, with no claim
of faster orchestration, lower memory, smaller model context or token savings. [Go embedding](https://pkg.go.dev/embed),
[Go build documentation](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies).

| Alternative considered | Reason for this choice |
| --- | --- |
| TypeScript/Node core | Familiar JSON/SDK ecosystem, but adds a runtime/version distribution obligation; legacy structure is no constraint. |
| Rust 1.96 with MSVC, available locally | Strong native integration; would add Cargo/async dependency choices for a service covered largely by Go's standard library. Rust remains feasible. |
| TypeScript plus native supervisor | Requires two runtime components and a versioned IPC boundary before a demonstrated need. |
| Electron/Tauri shell | Desktop shell and webview packaging are not needed by this foundation's accepted browser journey. |

The previous discovery recommendation to compare TypeScript and Rust was advisory. This decision
adds Go against the same requirements; no equal-load benchmark has been run. Native process-tree
ownership is still a separate feature-002 obligation, not a property inferred from language choice.
The legacy source, built entrypoints, databases and behavior remain untouched by this design.

## ADR-002 — A single durable owner and serialized SQLite

**Decision:** Store replacement state under `<user-home>/.council/v2/`, separate from project-local
and legacy `.council/` state. `.council/refactor/` is ignored development scratch, not runtime state.
Use `modernc.org/sqlite` v1.61.1, a pure-Go driver documenting SQLite 3.54.1 and Windows ARM64/x64
support. Pin its matching `modernc.org/libc` v1.79.0 and `golang.org/x/sys` v0.49.0; retain module
checksums and required notices. Its documented libc version coupling makes unreviewed independent
upgrades inappropriate. [Driver/platform documentation](https://pkg.go.dev/modernc.org/sqlite@v1.61.1),
[exact upstream module requirements](https://gitlab.com/cznic/sqlite/-/blob/v1.61.1/go.mod).

One owner holds a native OS lifetime lock before any database open, including reads. Persistent
`council ui` owns the store; other commands use its authenticated API. With no owner, a CLI command
acquires transient ownership and calls the same application service. PID files and timestamps are
discovery hints, never lock authority. Use Windows `LockFileEx` and the appropriate Unix lock;
an unreachable locked owner fails explicitly without takeover or process killing. [Windows lock API](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex).

Serialize database operations with one bounded connection/owner path. Transactions cover schema
initialization, draft/version changes and their exact replay receipts. No network/process waiting
occurs inside a transaction. Enable foreign keys and preserve signed 64-bit revision/write metadata;
public revisions remain canonical decimal strings. Lost acknowledgement retries replay the same
request identity; stale new writes conflict. Unknown commit status must never produce “Saved”.
Unsupported schemas fail without recreation. This slice performs no legacy migration or cleanup.

## ADR-003 — Private local transport before private data

**Decision:** Verify private state-directory permissions before database or token writes: Unix
0700 directories/0600 files; Windows protected DACL granting the current owner, SYSTEM and
Administrators, without inherited access for other users. Failure is a setup error, not a degraded
public store. This protects against unrelated local users, not software already acting as the
owner or an administrator. [Windows security information API](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-setnamedsecurityinfow).

Bind the HTTP owner only to `127.0.0.1` on an OS-selected port. Require the private 256-bit-or-stronger
bearer token for every API request, exact numeric Host/port validation and exact Origin checks for
mutations. Public routes expose only embedded public assets. No permissive CORS, ambient cookies,
external scripts or unrestricted process-execution endpoint is introduced. The contract defines
fragment bootstrap, origin-scoped session storage, restrictive CSP, no-referrer/no-store behavior,
descriptor verification and secret-free diagnostics. Loopback alone is insufficient authorization.

## ADR-004 — Strict inputs and exact original evidence

**Decision:** Use stable Go 1.27 `encoding/json/jsontext` and `encoding/json/v2`, without experimental
flags or a handwritten Unicode parser. Preserve strict defaults rejecting malformed Unicode and
duplicate members; enable `RejectUnknownMembers(true)` for typed request objects. Reject trailing
JSON values, wrong types, invalid IDs/revisions and oversized input before mutation admission.
[Go 1.27 JSON release notes](https://go.dev/doc/go1.27#encodingjsonv2),
[unknown-member option](https://pkg.go.dev/encoding/json/v2#RejectUnknownMembers).

Keep valid user text unchanged, including line endings, U+FEFF and literal U+FFFD. The contract's
4 MiB decoded-task and 32 MiB whole-body limits reject rather than truncate. Immutable draft
versions retain exact requested-policy snapshots. Project identity uses physical filesystem
equality and canonical paths, never unconditional Windows case folding; retain the chosen working
folder separately from repository root. Shell-free Git observation must preserve source and index.

## ADR-005 — Five honest installation observations, then ordered adapters

**Decision:** Foundation discovery examines PATH/native installation file metadata only and returns
exactly five harness entries. It never invokes a vendor binary, even for help/version, reads
credentials, starts login, installs software or changes global vendor settings. Each entry reports
installed/missing/unsupported launcher or a bounded discovery error. Authentication, provider,
account, model, effective permissions, external occupancy, allowance and usage remain unknown;
managed execution is explicitly `not_implemented` for all five in feature 001.

| Ordered feature | Documented feasible interface | Qualification boundary |
| --- | --- | --- |
| 002: Codex | Stable native `codex exec` JSONL automation. | Pin installed contract; own cancellation/drainage. Experimental app-server is not the production default. [Official commands](https://learn.chatgpt.com/docs/developer-commands). |
| 003: Claude Code | `claude -p` with structured/streaming output. | Validate flags, permissions and terminal errors for the installed version; SDK availability does not require adding its runtime. [Official programmatic CLI](https://code.claude.com/docs/en/headless). |
| 009: DeepSeek Harness | `dsh --profile acp`, using ACP v1. | Profiles are not separate executables; startup can initialize state. Developer-preview changes require pinned fixtures and explicit capability limits. [Official CLI](https://github.com/deepseek-ai/deepseek-harness/blob/master/apps/cli/README.md). |
| 010: Junie | Documented `junie --acp true`. | Negotiate actual session options; final stop reason matters. Assistant prose is not an authoritative structured artifact. [Official ACP mode](https://junie.jetbrains.com/docs/junie-cli-acp.html). |
| 011: Antigravity | Headless `agy -p` JSON/NDJSON. | Preserve terminal status and pinned-model refusal; validate artifact independently of provider success. [Official headless contract](https://antigravity.google/docs/cli/headless/). |

ACP v1 is the initial documented protocol; v2 remains Draft. Negotiate capabilities and opaque
model/mode options instead of assuming universal permissions, continuation, telemetry or output
schemas. [ACP v1 overview](https://agentclientprotocol.com/protocol/v1/overview).
These are documentation feasibility findings. Offline fake contracts and specifically authorized
live provider qualification are separate evidence; neither installation nor this table proves a
working account or remaining subscription allowance. No vendor process was run for this ADR.

## ADR-006 — Requested autonomy, branded UI and release evidence

**Decision:** Initialize the visible owner-wide preference to Autonomous full access with source
`default`; explicit selection records source `user`. Persist it and snapshot drafts, while keeping
effective harness capabilities unknown until an execution adapter establishes them. This does not
authorize unrelated publication, destructive work or a Council-managed paid API fallback.

Use the verified Ejupi Labs assets, exact palette, filled generic icons and system reading stack
from `docs/refactor/design-identity.md`. Embed only selected assets with hashes/provenance, the icon
MIT notice and the actual company-artwork attribution. Keyboard/focus, readable contrast, 360 CSS
px layout and reduced motion are acceptance checks, not presumed consequences of brand reuse.

Ship prepared executable archives so release users need no development toolchain. Preserve working
`npm test`, `npm run check` and build entrypoints as development orchestration. Acceptance uses fake
installations, strict-input/security fixtures, storage faults, owner contention, cross-client restart
and extracted-release smoke tests. Native Windows ARM64/x64 behavior requires platform evidence.
Go's race detector does not support native Windows ARM64: run race coverage on supported Linux CI
with its required C compiler and report absent CI evidence honestly. [Official race requirements](https://go.dev/doc/articles/race_detector#Requirements).
Independent review, corrected blockers and acceptance evidence precede feature 002 implementation.
