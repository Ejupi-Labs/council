# Changelog

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
