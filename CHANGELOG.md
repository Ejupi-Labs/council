# Changelog

## 1.2.0 — 4 November 2026

### Fair coordination and recovery

- Add durable call waiters and fair admission that reserves capacity for older requests while using available parallel slots.
- Advertise each host's concurrency limit at startup, scope lease renewal/release by run, and bind internal maintenance leases to their owning instance.
- Recover dead cleanup/patch-repair owners without clearing ordinary user file leases.
- Migrate supported state to schema 5 after stopping older hosts; inspect future schema versions before changing journal mode or file permissions.
- Keep health queries in one read snapshot, use read-only transactions for retention previews, and index unread mail and lease expiry.
- Add complete, byte-bounded inbox pages with recipient/filter-bound cursors; oversized legacy batches suggest the paged tool.
- Retry brief heartbeat and shutdown-unregister contention within five seconds while retaining immediate failure for lost ownership and fatal storage errors.
- Avoid migration locks for current-schema WAL opens and add startup-only asynchronous state retries before accepting any workflow.
- Pin a dedicated read snapshot for chunked online backups so continuous writes cannot repeatedly restart the copy; retain cancellation and the existing deadline.

### Evidence and artifacts

- Snapshot workflow inputs, reject active run ID replay, validate bounded runner results, and prevent caller mutation of queued/emergency outcomes.
- Stop all queued host work immediately after an evidence-write failure, drain active calls, preserve partial results and keep the originating outcome failed.
- Preserve bounded earlier pipeline evidence and fairly allocate parallel result/context budgets among participants.
- Isolate Git environment redirection, generate canonical applicable patches, validate committed nested workspaces and repair relative Git pointers.
- Detect dirty submodule contents before patch publication or ordinary cleanup; honor cancellation around atomic patch publication and preserve peer errors in Markdown reports.

### Runtime and verification

- Remove closed-session stream listeners, stop queued input parsing during shutdown and enforce actual readable backpressure with bounded parsing turns.
- Validate Claude terminal envelopes, direct-runner permissions and ACP/ACPX response consistency; support optional ACP model-category metadata.
- Bound unresolved filesystem metadata work and expose its pressure in health; cancel side-effect-free lookups without abandoning issued file mutations.
- Reject Windows short-name metadata bypasses and verify canonical directory identity where case-insensitive aliases could cross a case-sensitive root.
- Bound agent JSON graphs before allocation and accelerate large quoted payload scans without weakening escape/structural validation.
- Tighten supported SQLite database and sidecar permissions on POSIX without changing parent-directory permissions.
- Retry transient Windows atomic-publication sharing failures within a bounded, cancellable window while preserving the previous file.
- Coalesce Windows process-tree termination, preserve the supervisor during native cleanup and report unverified termination instead of disguising surviving descendants as ordinary cancellation.
- Add reusable offline service, storage and artifact endurance workloads plus regression coverage for actual contention, disk-full rollback, crash recovery and cross-platform behavior.
- Add a manual, single-job endurance CI workflow with bounded durations and log-only uploads; pin official actions, restrict CI permissions and reject uncommitted generated modules, including untracked or ignored files.

## 1.1.0 — 3 November 2026

### Operational readiness

- Add runtime/storage health, explicit integrity checks, consistent cancellable online backups and preview-first retention through CLI and MCP.
- Add bounded history pagination while preserving existing CLI full-record commands.
- Migrate to SQLite schema v3 with atomic run admission, exact instance fencing, immutable terminal workflow outcomes and a workspace cap of 100 active runs.
- Refuse schema upgrades while an older registered host is alive; stop every old host before upgrading shared state.
- Degrade and reject new work after ownership or persistence failures; retain an honest in-memory failed outcome when durable storage fails.
- Track and cancel maintenance before closing state; preserve worktree intents before filesystem side effects and reconcile interrupted cleanup.
- Add explicit `patches` / `council_patches` repair for retained exports, sharing cleanup leases and preserving terminal workflow outcomes.

### Performance and bounded shutdown

- Persist indexed, bounded run/event summaries and a cancellation flag; polling and scheduling no longer deserialize full live transcripts.
- Bound SQLite lock waits, statement caching, process lookup and platform kill helpers.
- Include executable resolution in call deadlines and validate bounded NDJSON frames with strict UTF-8 decoding and linear scanning.
- Isolate platform stdout writes in an owned output helper with an independent parent-death watchdog.
- Bound Git and artifact-export work, retain pending export diagnostics, and back off workspace slot contention.
- Add reproducible offline benchmarks and regression tests for storage load, upgrade contention, maintenance, blocked output and subprocess failure.

### Deployment scope

- Document trusted local project trees, operational restore procedures and custom runner cancellation requirements.
- ACP path checks do not defend against concurrent ancestor junction/symlink retargeting by a hostile process under the same account. Native tools retain their own process privileges.
- Keep zero production npm dependencies and ship all operational documentation in the release package.
- Require Node 24.18.1+ and reject embedded SQLite below 3.51.3 before opening state, avoiding the documented multi-writer WAL reset corruption race.

## 1.0.0 — 3 November 2026

### Runtime and installation

- Remove all production npm dependencies and root-only dependency overrides.
- Add native Codex/Claude JSON execution and direct ACP execution for DSH, Junie and Antigravity.
- Preserve optional external ACPX compatibility without installing or downloading adapters.
- Ship compiled, self-contained plugin entrypoints and schema files.
- Verify offline tarball installation from an empty npm cache and official MCP client interoperability.

### Reliable collaboration

- Share transactional concurrency slots across all Council processes in a workspace.
- Track run owners, migrate SQLite schema v1 to v2, and recover proven-dead owners without retrying tasks.
- Give accepted cancellations precedence over final completion.
- Stop owned vendor trees through a supervisor when the Council parent is forcibly terminated.
- Preserve queued timestamps and prevent terminal run ID reuse.
- Preserve peer evidence when project context reaches its maximum size.
- Persist worktree base commits and manifests; verify Git pointers and reject redirected artifacts.
- Add reviewed worktree cleanup, explicit continuation, event inspection and Markdown reports.
- Fix lease expiry ownership races and await concurrent shutdown callers consistently.

### Protocol and validation

- Bound MCP input, output queues, pending requests and schema graph traversal.
- Validate native/ACP terminal states, sessions, advertised models and safe one-use permissions.
- Add project-scoped ACP text-file operations with path and metadata protections.
- Reject ambiguous CLI arguments and expose active run IDs immediately on stderr.

### Migration from 0.1

- The default backend is now `native`; change a project setting of `"backend":"acpx"` to `"native"` to use the self-contained runtime. Explicit `acpx` settings continue to require an independently installed ACPX CLI.
- Codex and Claude require their installed vendor CLIs; their adapters are no longer bundled.
- Configuration version remains 1. Optional `transport`, `command` and `args` fields support custom launchers.
- Existing SQLite state upgrades automatically. Partial results and retained worktrees remain on disk; no task is resumed or retried automatically.
- New worktrees carry durable manifests. Older 0.1 artifacts without these manifests must be reviewed and managed through normal Git commands.
- Concurrency is now enforced across processes. The smallest advertised active limit governs new dispatches.
- Public runtime exports no longer depend on Zod or the MCP SDK. Use `configDefinition` and `workflowDefinition` for JSON Schema, and the exported parse helpers for validation.

## 0.1.0

Initial six-mode collaboration engine, ACPX execution, MCP/CLI interface, local coordination state and isolated worktrees.
