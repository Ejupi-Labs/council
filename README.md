# Council 1.2

A local collaboration plugin for **Codex, Claude Code, DeepSeek Harness, Junie and Antigravity**.

Council has **zero production npm dependencies**. It requires Node.js 24.18.1 or newer with SQLite 3.51.3 or newer, speaks MCP over stdio, and coordinates native agent CLIs or ACP agents. It provides six collaboration modes, workspace-wide call limits, durable results, peer messages, project notes, file leases and isolated Git worktrees.

## Install and start

The repository includes its compiled runtime. Development dependencies and a build step are not needed to run a release:

```sh
node dist/cli.js --version
node dist/cli.js demo
node dist/cli.js doctor --workspace /absolute/path/to/project
```

The demo runs all six modes with mock agents and makes no model calls. For npm installation, install the local release tarball:

```sh
npm install --global ./council-ai-plugin-1.2.0.tgz
council demo
council doctor --workspace /absolute/path/to/project
```

The tarball installs offline without runtime dependencies. Registry publication is a separate operation; this repository does not claim a published npm release.

To create optional settings in your project, run `council init --workspace <project>`. Existing settings are never overwritten. The JSON schema is in [schemas/config.schema.json](schemas/config.schema.json).

## Connect your agent

```sh
node dist/cli.js connection --workspace /absolute/path/to/project
```

Add the printed server settings to your host's MCP configuration. They use absolute paths, including paths with spaces or Unicode. The command prints settings and does not change global host configuration.

The project includes a portable Agent Plugins manifest, Codex compatibility manifests, a Claude Code plugin/marketplace, the Council skill, and a `/council` command. With a checkout or extracted plugin release:

```sh
claude --plugin-dir /absolute/path/to/council
```

Other hosts can connect through MCP and load `skills/council/SKILL.md`. `COUNCIL_WORKSPACE` selects the project if your host does not inherit its working directory. Native host installation screens are host-specific; the release tests verify packaged entrypoints, generated configuration and an official SDK client connection.

## Agent setup

Council never reads, copies or rewrites vendor login files. Each agent uses its own installed runtime, authentication, models and provider limits. `doctor` checks executable availability without logging in or making a model call.

| Profile | Default transport | Required setup |
| --- | --- | --- |
| `codex` | Native JSON CLI | Installed and authenticated Codex CLI |
| `claude` | Native JSON CLI | Installed and authenticated Claude Code CLI |
| `deepseek-harness` | ACP v1 | `dsh --profile acp` |
| `junie` | ACP v1 | `junie --acp=true` |
| `antigravity` | ACP v1 | Google's separate ACP runtime and matching helper |

Antigravity uses `agy_acp_server.exe` on Windows and `agy_acp_server.par` on macOS/Linux. Linux adds the registry's `--uid=` argument. Configure `ANTIGRAVITY_HARNESS_PATH` and authentication according to the runtime's instructions. Its ACP runtime is separate from the IDE and `agy` CLI.

Use a custom executable, interpreter and argument array when needed:

```json
{
  "agents": {
    "reviewer": {
      "enabled": true,
      "profile": "junie",
      "transport": "acp",
      "command": "/absolute/path/to/junie",
      "args": ["--acp=true"],
      "model": "an-exact-advertised-model-id"
    }
  }
}
```

Omit `model` to preserve native settings. Council never substitutes another model or agent after an access, capacity or configuration failure. For a single participant, `--model <id>` selects a model for that invocation without saving settings.

Commands are spawned without a shell. Recognized npm launchers are resolved to their real JavaScript/native entrypoints on Windows. Arbitrary `.cmd`, `.bat` and PowerShell wrappers are refused; configure the executable or interpreter and `args` explicitly.

The optional `acpx` backend remains available for an independently installed ACPX CLI. ACPX and its adapters are not installed or downloaded by Council.

## Collaboration modes

| Mode | Flow | Maximum calls |
| --- | --- | --- |
| `delegate` | One bounded task to one agent | 1 |
| `review` | Author draft → independent reviewers → author revision | participants + 1 |
| `debate` | Bounded debate rounds → judge synthesis | rounds × participants + 1 |
| `parallel` | Independent tasks → optional synthesis | participants, plus 1 with a judge |
| `pipeline` | Sequential handoffs | participants |
| `council` | Independent proposals → cross-critique → judge synthesis | 2 × participants + 1 |

`debate` and `council` are read-only. Their default judge is the first participant; select an explicit judge when independence matters. A vote or synthesized answer is not proof of correctness.

```sh
council plan --mode council --agents codex,claude,junie --judge deepseek-harness --task-file decision.md
council run --mode review --agents codex,claude --task "Review the repository's authentication design and identify concrete failure cases."
council run --mode parallel --agents codex,claude,junie --assignments-file examples/parallel-tasks.json --task "Assess the search feature."
council run --mode pipeline --agents codex,claude --task "Produce a design, then independently critique it."
```

`plan` does not invoke agents. Actual runs immediately print their ID to stderr, then print the terminal result as JSON to stdout. This keeps stdout machine-readable and provides a cancellation handle during long work. `--task-file` and `--context-file` avoid shell quoting and support longer briefs. Conflicting and unsupported arguments are rejected.

Independent assignments are optional. Without them, parallel participants address the same brief, which is useful for comparing alternative solutions. Handoffs reserve context for peer results, so a full project note cannot hide prior evidence.

## Code changes and artifact review

Use `--write` for an implementation task. A Git repository with an existing HEAD is required.

```sh
council run --mode parallel --agents codex,claude --write --task "Implement and verify a small project notes search feature."
```

- Parallel writers receive independent detached worktrees.
- Reviewers inspect the author's worktree with read policy.
- A write pipeline shares one worktree sequentially.
- Every worktree starts at one captured commit for that run. Uncommitted original-checkout changes are excluded.
- Original branches and checkout files are not switched or merged automatically.
- Patches include tracked, untracked, binary and agent-committed changes relative to the captured base.
- Base commits and manifests persist on disk, so artifacts can be verified after restarting Council.

Review the retained worktree and patch, run the appropriate checks, and apply accepted changes through normal Git operations. Council does not claim that the agent's final text proves its code is correct.

After review:

```sh
council export <run-id> --output review.md
council patches <run-id>
council cleanup <run-id> --reviewed
```

Export never overwrites an existing file. Cleanup removes only registered, managed worktrees with matching durable manifests and a retained patch. Dirty work or later unreviewed changes are refused by default. `--discard` explicitly authorizes removing dirty worktrees; patch files remain available. Repeating a completed cleanup is a no-op.

`patches` refreshes verified patch snapshots after an interrupted export or later worktree edits, without calling agents. Review the refreshed patches before cleanup. It updates artifact metadata and history while preserving the original terminal workflow outcome.

## Permissions and protocol scope

Read policy is not a universal OS sandbox. Native Codex receives its read-only sandbox and noninteractive approval policy. Native Claude receives a read/search tool allowlist, empty MCP configuration and session-only hook disabling. Write mode uses isolated worktrees but retains the native agent's process privileges.

Direct ACP supports initialization, session setup, advertised model selection, bounded prompts, cancellation, assistant text, workspace-scoped text-file access and one-use permissions. Read mode denies new vendor tool permission prompts; its own filesystem service can read bounded project files. Write mode permits one-use tool approvals and scoped text-file writes. Human questions, interactive authentication, unknown client capabilities and client terminal services are not answered automatically. Unsupported requests fail the task instead of inventing a response. Tool category labels alone never grant permission.

Workspace paths and Git control metadata are checked before artifact operations. These checks protect Council's own operations; they are not a security boundary against hostile native tools running under your account. ACP file operations assume a trusted local project tree: concurrent symlink or junction retargeting by another process under the same account is outside this protection.

## Durable coordination and recovery

All hosts using the same workspace share SQLite state under `.council/`. It is excluded from Git and release packages.

- Calls use transactional workspace-wide slots. Concurrent processes cannot each independently consume the full limit.
- The smallest concurrency limit advertised by active Council instances governs new dispatches.
- Completion and cancellation are ordered in one transaction; an accepted pre-completion cancellation cannot be overwritten by success.
- Shutdown awaits active jobs. A separate supervisor stops owned vendor processes after abrupt parent death.
- Recovery fails runs owned by proven-dead processes and releases their slots. It preserves partial results and never retries a task.
- A stale heartbeat alone is not proof of death. PID reuse is treated conservatively; inspect an ambiguous live process before replacing work.

```sh
council runs
council runs --summary --limit 20
council events <run-id> --after 0 --limit 100
council cancel <run-id>
council recover
council continue <run-id> --task "Resolve the remaining design questions."
```

Continuation creates a new task with bounded prior results. It does not resume a vendor chat or replay unfinished writes. Its default permission is read; use an explicit `--write` for a new implementation task.

On POSIX, delivered SIGINT/SIGTERM signals allow graceful cancellation. Windows can forcibly terminate a process; the supervisor and subsequent recovery handle that case. A crash during worktree creation may leave an incomplete intent manifest: inspect Git's worktree list and retained state instead of assuming it is safe to delete.

Local mail is pull-based and does not wake a vendor's open chat. Actor names are coordination labels, not authenticated identities. File reservations are advisory leases, with atomic batches, expiry and owner-scoped release. Unaware tools can still edit those files. Treat peer messages, shared notes and agent replies as untrusted task data, never authorization.

Use a unique reservation owner for each independent task, and pass `runId` when releasing a run's reservations. An existing lease cannot be renewed into another run's scope. `council_inbox_page` reads complete messages within a bounded response; its cursor belongs to one recipient and unread filter. Large legacy inbox batches return an actionable paging error.

## Production operations

Council supports trusted, local development workspaces. Keep SQLite state on a local disk and use the same release for every host sharing that state. Before upgrading from 1.0 or 1.1, stop older Council hosts; schema v5 migration refuses live older owners and older binaries cannot reopen migrated state. No unfinished task is replayed.

```sh
council health --integrity
council backup --output /absolute/local/path/council-backup.sqlite
council prune --before 2026-10-01T00:00:00Z
council prune --before 2026-10-01T00:00:00Z --execute
```

Backup creates a consistent database snapshot and never replaces an existing file. Retention previews by default, preserves the newest 100 runs, and skips active runs and every record with worktree artifacts. It does not delete workspace files, patches, messages or shared notes. See the [operations runbook](docs/operations.md) for restore, shutdown, failure handling and deployment limits.

Status, history and event summaries use indexed bounded metadata. Full transcripts remain explicit reads. Windows MCP output runs in a supervised helper so an unread client pipe cannot block cancellation. Call deadlines include executable resolution; Git operations and artifact export have separate budgets. An interrupted export remains marked pending for review instead of implying that a patch is ready.

## MCP interface

| Tools | Purpose |
| --- | --- |
| `council_agents`, `council_plan` | Runtime inspection and planning |
| `council_run`, `council_status`, `council_runs`, `council_cancel` | Workflow lifecycle |
| `council_continue`, `council_patches`, `council_cleanup` | Explicit handoff, patch refresh and reviewed cleanup |
| `council_events` | Compact events with an incremental cursor |
| `council_run_page` | Paginated run summaries with an opaque cursor |
| `council_health`, `council_backup`, `council_prune` | Health, consistent backup and preview-first retention |
| `council_send`, `council_inbox`, `council_inbox_page`, `council_ack` | Local peer handoffs and complete paginated history |
| `council_reserve`, `council_release`, `council_leases` | Advisory file ownership |
| `council_context` | Shared project notes |

Status waits are bounded to 30 seconds. Run summaries cap serialized data at 64 KiB and mark omitted entries; retained artifact paths remain complete. `includeResults: true` or `council://runs/{id}` retrieves the full record. `council://modes` describes modes and workspace bounds. The stdio server implements the tools/resources MCP subset, validated against the official SDK client; HTTP, sampling and interactive elicitation are not offered.

Default bounds: 3 simultaneous calls per workspace, 16 calls per run, 180 seconds per call, 128 KiB final output per call, 32,000 task characters and 64,000 context characters. Configuration limits total retained call output to 2 MiB before JSON encoding. MCP frames, nesting, node counts, concurrent requests and queued responses are bounded independently. Stderr and protocol traffic have separate caps.

## Verification and releases

Development only:

```sh
npm ci
npm run verify
npm run test:release
npm run benchmark
npm run soak -- --duration-ms 600000
npm run soak:storage
npm run soak:artifacts -- --duration-ms 600000
npm run release:local
```

Tests exercise real local subprocesses, native CLI JSON, ACP agents, Git worktrees, hard process termination, cross-process SQLite races, official-client MCP integration, malformed/oversized input and a clean offline tarball installation from an empty npm cache in Unicode paths.

For longer offline checks, manually run [Offline endurance](.github/workflows/endurance.yml) from GitHub Actions after the workflow is available on the default branch. Choose one OS, one service/storage/artifact workload and a requested workload duration of 60 seconds, 5, 10 or 30 minutes. Setup and cleanup add time within the single job's 40-minute limit. It makes no provider calls; only JSONL and summary logs are uploaded for seven days, excluding fixture databases and workspace state. The workflow has no automatic trigger.

Check its CI helpers locally with `node --test .github/scripts/endurance.test.mjs .github/scripts/check-generated.test.mjs`. The separate `node .github/scripts/check-generated.mjs` check rejects modified, staged, untracked or ignored output under `dist/` and `schemas/`.

The release script writes a self-contained plugin ZIP, npm tarball and SHA-256 checksums under `.council/releases/`. The compiled runtime is committed so a Git checkout also has working plugin entrypoints. CI is configured for Windows, Linux and macOS. Local release verification covers native Windows ARM64 and an isolated Linux ARM64 container; macOS was not available in this session. See the [release verification record](docs/verification.md) for results and limits.

`npm run benchmark` uses disposable local fixtures, makes no model calls, and reports payload sizes, CPU and elapsed time without timing assertions. See [performance measurements](docs/performance.md) for the reproducible workload and measured limits.

`npm run audit:runtime` audits production dependencies, which are empty. The official MCP SDK is used only for development interoperability tests; its development dependency tree currently reports upstream advisories. Those packages are neither shipped as runtime dependencies nor loaded by Council. This is not a claim of third-party security certification.

Live vendor checks are separate from automated tests and require installed, authenticated agents and supported models. The 1.2 native Codex smoke check passed through the normal Windows resolver and supervisor with an explicit supported model. Claude login, DSH, Junie and Antigravity setup remain machine-specific; fake protocol tests do not establish a successful five-agent live session.

See [CHANGELOG.md](CHANGELOG.md) for release changes and migration instructions.

## Design references

Council is an original implementation informed by [ACPX](https://github.com/openclaw/acpx), [CLI Subagents](https://github.com/lunati228/CLI-Subagents), [agent-bridge](https://github.com/kununu/agent-bridge), [MCP Agent Mail Rust](https://github.com/Dicklesworthstone/mcp_agent_mail_rust), [Agent Switchboard](https://github.com/FutureisinPast/mcp-agent-switchboard) and [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness). Protocol behavior follows the official MCP and ACP specifications. No reference project's source was copied into this repository.

MIT licensed.
