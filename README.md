# Council

Council is a local plugin, MCP server and CLI for collaboration between **Codex, Claude Code, DeepSeek Harness, Junie and Antigravity**. It runs real agent runtimes through ACPX and adds bounded workflows, durable results, project notes, a local mailbox and file reservations.

This is version **0.1.0**. The local workflow engine, ACPX integration and MCP interface have automated tests. Real vendor sessions depend on your installed runtimes, account access and authentication; these have not been tested together on this machine.

## Quick start

Requires Node.js **24+**, npm and Git. Windows, macOS and Linux are targeted; the implementation was tested locally on native Windows. CI is configured for all three systems.

```sh
npm ci
npm run build
node dist/cli.js demo
node dist/cli.js doctor
```

The demo executes every mode with deterministic mock agents. It makes **zero model calls**, changes no project files and stores its results in a temporary directory.

For a real task, select your target project with `--workspace` (or run from that directory), create optional settings if `council.config.json` does not already exist, and preview the plan:

```sh
node dist/cli.js init
node dist/cli.js plan --mode review --agents codex,claude --task "Review the authentication design and identify concrete failure cases."
node dist/cli.js run --mode review --agents codex,claude --task "Review the authentication design and identify concrete failure cases."
```

Commands print JSON. `--task-file` and `--context-file` accept longer briefs without shell quoting. `--workspace` selects the project. `--backend mock` is available for offline evaluation. No login or account configuration is changed by Council.

## Collaboration modes

| Mode | Flow | Agent calls |
| --- | --- | --- |
| `delegate` | One bounded task to one agent | 1 |
| `review` | Author draft → independent reviewers → author revision | participants + 1 |
| `debate` | Bounded rounds → judge synthesis | rounds × participants + 1 |
| `parallel` | Independent tasks → optional synthesis | participants, plus 1 with judge |
| `pipeline` | Sequential handoffs | participants |
| `council` | Independent proposals → cross-critique → judge synthesis | 2 × participants + 1 |

`debate` and `council` are read-only. Their default judge is the first participant unless `--judge` selects another configured agent. Use independent participants and an explicit judge when diversity matters; a synthesized answer is not proof of correctness.

```sh
node dist/cli.js run --mode council --agents codex,claude,junie --judge deepseek-harness --task-file decision.md
node dist/cli.js run --mode debate --agents codex,claude --rounds 2 --task "Compare two indexing designs using this repository's actual access patterns."
node dist/cli.js run --mode pipeline --agents codex,claude,junie --task "Build a design, critique it, then produce an implementation checklist."
```

For independent parallel subtasks, provide `--assignments-file tasks.json` with a JSON map such as `{"codex":"Inspect indexing and query behavior","claude":"Review access controls and privacy","junie":"Check integration and test coverage"}`. Each participant receives the global task and its own assignment. Without assignments, participants independently address the same brief, which is useful for comparing alternative solutions.

## Writing code with isolated worktrees

Add `--write` only when the task includes implementation. A Git repository with at least one commit is required.

```sh
node dist/cli.js run --mode parallel --agents codex,claude --write --task "Implement a project notes search feature and add appropriate verification."
```

Write workflows use retained detached worktrees under `.council/worktrees/`:

- Parallel writers receive independent worktrees.
- A review uses the author's worktree; reviewers inspect that same worktree with read policy.
- A write pipeline shares one worktree sequentially so later agents see earlier changes.
- The original checkout and current branch are never switched or merged automatically.
- Worktrees start at the repository's committed HEAD. Uncommitted original-checkout changes are **not included**.
- Returned artifacts include worktree paths and patches covering tracked, untracked, binary and agent-committed changes relative to the captured base.

Inspect patches and verify the retained worktree before accepting changes. Council does not automatically apply patches, install dependencies in worktrees, run tests on your behalf, or remove worktrees containing unreviewed work. Use normal Git worktree commands when finished.

The `read` policy denies tool approval requests; it is **not an OS sandbox** and cannot restrict an adapter's already-authorized tools. Write mode permits agent tool requests inside a worktree but does not provide an OS boundary around that agent. Configure native vendor sandbox settings as appropriate for your task.

## Connect a coding agent

Build the plugin first. To generate explicit connection settings for any MCP client:

```sh
node dist/cli.js connection --workspace /absolute/path/to/project
```

The generated JSON uses absolute executable and entrypoint paths. Add that server to the MCP configuration of Codex, Claude Code, Junie, DSH or Antigravity. This command only prints settings and does not modify global configuration.

The repository also contains:

- Portable Agent Plugins manifests: `plugin.json`, `mcp.json` and `skills/council/`.
- Codex compatibility packaging: `.codex-plugin/plugin.json` and `.mcp.json`.
- Claude Code packaging: `.claude-plugin/`, the Council skill and `/council` command.

For Claude Code local development, launch `claude --plugin-dir /absolute/path/to/council` after building. Other hosts can use the explicit MCP connection above and load the portable skill. Plugin launchers normally inherit the project working directory; use `COUNCIL_WORKSPACE` or an explicit `--workspace` if your host launches from elsewhere. Native plugin installation UI has not been verified in this release.

## Real-agent prerequisites

ACPX **0.21.1** is installed as a pinned dependency. Council invokes its JavaScript entrypoint with Node directly, without a shell or a platform-specific `.cmd` wrapper.

| Council profile | Required runtime/setup |
| --- | --- |
| `codex` | Codex configuration/authentication and the ACPX Codex adapter |
| `claude` | Claude Code configuration/authentication and the ACPX Claude adapter |
| `deepseek-harness` | `dsh` installed and configured for the ACP profile |
| `junie` | Junie CLI installed and authenticated; ACP mode |
| `antigravity` | Google's separate official ACP runtime and matching helper, configured and authenticated |

Codex and Claude ACP adapters are pinned local dependencies so their launch does not depend on an unpinned `npx` download. Antigravity's ACP runtime requires separate setup from its IDE/CLI. Interactive authentication and some user questions need a compatible interactive client. `doctor` checks local launch prerequisites only; it does not prove login, account availability, model access or ACP compatibility.

Council does not read, copy, rotate or rewrite vendor credentials. Agent processes retain their native account setup. Existing accounts, usage limits and provider terms still apply.

## MCP tools

| Tools | Purpose |
| --- | --- |
| `council_agents`, `council_plan` | Inspect local prerequisites and preview work |
| `council_run`, `council_status`, `council_runs`, `council_cancel` | Start, inspect and cancel workflows |
| `council_events` | Incremental progress using a sequence cursor |
| `council_send`, `council_inbox`, `council_ack` | Local agent handoffs |
| `council_reserve`, `council_release`, `council_leases` | Advisory file ownership |
| `council_context` | Compact shared project notes |

Runs return an ID immediately. `council_status` accepts `waitMs` up to 30 seconds. Summaries omit full step payloads and cap final output; `includeResults: true` or the `council://runs/{id}` resource retrieves the full record. The `council://modes` resource describes project bounds.

Mail is pull-based: it does not inject prompts into an already-open vendor chat or wake sleeping agents. Actor names are labels within a trusted local workspace, not authenticated accounts. Peer messages and shared notes are untrusted task data, not permission or instructions to override the user.

File leases are atomic across local processes, case-insensitive and time-limited. Reserve exact relative files or a `directory/**` subtree. A batch conflicts atomically, and only its owner can release a lease. Reservations are advisory; unaware tools can still edit files. Worktree isolation protects independent write workflows instead.

## Configuration and limits

`council.config.json` is optional; `init` creates the defaults without replacing an existing file. You can disable profiles or define named aliases using one of the five supported ACPX profiles. An optional `model` preserves an explicit user choice; omitted models use the agent's own configuration.

Default bounds:

- 3 simultaneous agent calls **per Council service process**.
- 16 agent calls per run, checked before dispatch.
- 180 seconds per agent call.
- 128 KiB final output per agent, with separate bounded ACP traffic and stderr buffers.
- 32,000 task characters and 64,000 context characters.
- At most 2 MiB for `maxCalls × maxOutputBytes`; this bounds retained results.

Completed, failed and cancelled runs remain distinct. Timeouts and protocol errors fail the run while retaining partial results. Cancellation works across Council processes via durable events. Closing the MCP connection cancels child work. No agent is retried, replaced or resumed automatically, so a failed call is not silently duplicated.

Calls use temporary ACPX sessions; multi-step continuity is passed through bounded briefs rather than resuming vendor chats. Nested Council delegation is disabled using inherited process depth. This is a coordination guard, not a security boundary against hostile local code.

SQLite stores runs, messages, events, notes and leases under `.council/`. That directory is ignored by Git and excluded from npm distribution. Store only information needed for the task; prompts and results remain locally persisted. Multiple Council processes share mailbox/leases/state, while concurrency limits apply independently per process. After a hard process or machine crash, an interrupted record may remain marked running; inspect its events and external processes before starting a replacement.

## Development and verification

```sh
npm run check
npm test
npm run demo
npm pack --dry-run
```

Tests use deterministic agents, real local subprocesses, real Git worktrees, independent SQLite connections/processes and MCP protocol clients. ACPX integration tests use a local ACP adapter, with no vendor calls or credentials.

A separate live smoke check on 3 November 2026 completed a Codex task through the bundled adapter and returned the exact requested nonce. It used an explicit model advertised by the runtime in a temporary profile. The globally configured Codex model was unavailable to that CLI account, so it was left unchanged. Claude's native CLI required login; DSH, Junie and the Antigravity ACP runtime were not installed. These are setup limits for this machine, not evidence of a successful five-agent live session.

Some upstream manifests currently request versions absent from the npm registry. `package.json` pins compatible published `tsx`, `zod` and `qs` versions using overrides, alongside local Codex/Claude ACP adapter versions. ACPX's `exec` protocol path is covered by the tests; its separate TypeScript flow-authoring feature is not used. The current registry audit reports advisories in this dependency tree, including Zod and transitive HTTP packages. Patched requested versions are not all published in the available registry. A bounded stdin guard caps frame bytes, JSON nodes and nesting before SDK validation to mitigate oversized collection inputs. This release is a local stdio integration, not a network service; do not describe it as security-audited or production-certified. Recheck `npm audit` and upstream compatibility when updating dependencies. Source installation with `npm ci` is the supported installation path; npm publishing remains deferred until upstream ranges are consistent, because dependency-package overrides are not inherited by consuming projects.

## Design references

Council is an original implementation informed by these projects:

- [ACPX](https://github.com/openclaw/acpx): structured ACP agent execution; used as a runtime dependency.
- [CLI Subagents](https://github.com/lunati228/CLI-Subagents): cross-harness delegation and useful lifecycle behavior.
- [agent-bridge](https://github.com/kununu/agent-bridge): compact peer handoffs and portable skill guidance.
- [MCP Agent Mail Rust](https://github.com/Dicklesworthstone/mcp_agent_mail_rust): durable messages and advisory file reservations.
- [Agent Switchboard](https://github.com/FutureisinPast/mcp-agent-switchboard): multiple collaboration modes and bounded shared context.
- [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness): native subagent and ACP composition.

Reference repositories were consulted online. Their source code was not copied into this repository. Council does not claim to be affiliated with any agent vendor.

MIT licensed.
