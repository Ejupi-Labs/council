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

The installed `agy` CLI can also be used directly with an explicit native transport. The configuration in this checkout selects it; the schema defaults retain ACP compatibility:

```json
{
  "agents": {
    "antigravity": { "profile": "antigravity", "transport": "native" }
  }
}
```

Native Antigravity sends one stdin NDJSON turn using the [documented CLI protocol](https://antigravity.google/docs/cli/headless/), with `plan` mode for read tasks or `accept-edits` for isolated write tasks and a requested terminal sandbox. Native settings and platform sandbox availability still apply. Council never passes automatic approval flags. Permission notices can accompany a successful CLI exit, so any non-whitespace stderr diagnostic conservatively fails this transport instead of certifying a possibly incomplete result. Assistant deltas, conversation identity and the final result must agree; cut-off or contradictory replies fail.

On Windows, Council recognizes Junie's standard `.local/bin/junie.bat` installation layout, validates its bounded version pointer and launches the matching `junie.exe` directly. It does not execute the batch file. Vendor authentication is still handled through the native CLI.

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

Native Codex JSON output uses the last completed agent message as the selected answer. Progress narration is not prepended to machine-readable advisor requests or room ballots, and an empty final answer fails rather than reusing an earlier preamble. This follows the final-answer distinction in the [official non-interactive guidance](https://learn.chatgpt.com/docs/non-interactive-mode); configured output and transport guards still apply.

The optional `acpx` backend remains available for an independently installed ACPX CLI. ACPX and its adapters are not installed or downloaded by Council.

## Project rooms and server-side waiting

Start the local project-manager interface with:

```sh
node dist/cli.js ui --workspace /absolute/path/to/project
```

Open its printed loopback URL. `--backend mock` provides an explicitly labelled offline simulation. The interface runs locally, requires its generated access token, and keeps room state and full chat history in `.council/rooms.sqlite`. Stop the foreground server with Ctrl+C. `council_ui` opens the same interface from a connected MCP session.

Opening a freshly printed complete link in an existing dashboard tab refreshes authorization, including a tab previously opened without its token. The token stays in the URL fragment during navigation and is cleared after initialization. Unrelated fragment changes do not reload the dashboard.

Create a room, select lead/worker/advisor profiles, and define tasks with read/write permissions and dependencies. Each voting member submits a proposal; an explicit weighted majority approves the plan. The highest-weight approving proposal supplies assignments, with the lead and then member order breaking assignment ties. Disagreements and alternative assignments remain in history. Advisor-only members neither vote nor execute tasks. Weights express the project manager's decision policy; they are not automatic intelligence scores or proof of correctness.

An omitted member weight inherits the profile's configured `authority`, or 1 when none is configured. A new project-manager message invalidates a ready approval. If instructions change during discussion or execution, already-dispatched workflows drain under their original instructions, including calls waiting for an execution slot; their evidence stays available and further room tasks require a fresh discussion. Use Stop to cancel admitted work. Completed tasks and retained writes are never replayed automatically. Failed or cancelled rooms retain their history and artifacts; create a new room for replacement work.

Weights accept any positive finite number, including scientific notation. Majority checks use exact decimal arithmetic, so rounding and overflowing totals cannot turn a tie into approval. Totals that exceed the numeric range are retained as scientific strings. Unsent message and task drafts stay separate for each room within the current tab; switching rooms does not send or move a draft.

Each room records whether it belongs to the native, ACPX or mock backend. Another backend can inspect its history, but cannot start or execute it. The UI labels both the current server and the recorded room backend so an offline simulation cannot be mistaken for live agent work.

Approved tasks execute automatically by default. The server advances discussion rounds and dependent tasks without asking an orchestrating model to poll. Write tasks retain isolated worktrees for review and never merge automatically. Invalid ballots, failed calls or an unresolved vote require attention rather than authorizing partial work as success.

**Pause** blocks admission of further room workflows. Already admitted workflows, including calls waiting for a slot and their internal advisor/resumption calls, finish under their original instructions. The room then saves a checkpoint and releases its active job; no model calls or polling turns are needed while it remains paused. This control does not suspend vendor processes. **Stop** cancels admitted work instead.

**Resume** continues the saved discussion round or unfinished approved tasks without replaying completed tasks or retained writes. A paused ready plan returns to ready and still needs **Execute tasks**. Paused rooms survive a normal host restart. New project-manager guidance invalidates the paused checkpoint and requires a fresh discussion. Failed or cancelled work is never resumed automatically; backend, approval and remaining call-budget checks still apply. Use the same Council build in every host; legacy active owners without durable checkpoints cannot accept a pause.

The MCP controls `council_room_pause` and `council_room_resume` wait in the server by default, up to the chosen `waitMs` deadline; `waitMs: 0` returns immediately. Cancelling that wait does not undo an accepted pause or cancel the room's work. Manual rooms (`autoExecute: false`) reserve discussion calls when starting discussion, then check the full task/advisor budget when execution is explicitly requested. Automatic rooms retain the combined preflight.

Dependent tasks receive complete room reports with workflow identities and retained artifact metadata. Worktree paths identify separate evidence; predecessor changes are not automatically present in the follower's workspace. A failed or unverified prerequisite export stops dependent dispatch. Workers must report blocked file access and unverified integration as incomplete work.

Context is complete by default. Room history, task briefs and peer handoffs are not shortened to save tokens. Context and call budgets can be chosen explicitly; an insufficient context budget fails before dispatch instead of silently omitting evidence. UI task cards and recent-history windows are labelled previews, with full history available separately. Model and provider context limits remain native to the selected runtime. Transport and persistence errors are reported explicitly.

Use **View workflow** on a task or linked chat message to inspect its full selected result, every recorded step and retained worktree/patch paths. Failed and cancelled work stays labelled as incomplete evidence. **Download snapshot JSON** exports that exact stored snapshot, including task/context and recorded settings, without starting an agent call. Reloading the snapshot is an explicit read; a pruned workflow is reported as unavailable rather than replaced by its preview.

The **Event timeline** loads complete stored events only when requested. It shows sequence numbers, timestamps and expandable JSON payloads, including recorded routing/billing observations and step outcomes. Apply an exact, case-sensitive event type to inspect one category; leave the filter empty for all types. **Load more events** continues the current history and **Check for newer events** reads later events explicitly. Snapshot reloads retain already loaded timeline entries. These controls invoke no model and do not poll.

Event pages bound download size, not stored history or payload precision. Each event is returned intact, including oversized events on their own page, and cursors never skip an unread event. The library API is `service.eventPage(runId, after, limit, type?)`, also available as authenticated `GET /api/runs/:id/events`. It returns `{ events, nextAfter, hasMore, nextCursor }`; use `nextAfter` when explicitly checking for later appends, even after an empty page. The JSON download exports the workflow snapshot; event pages are separate reads. Full event data is never substituted with a compact summary.

**Full history** opens immediately with a loading indicator and displays read errors inside the dialog. Closing a view stops its pending read, and late responses or native close events from an earlier opening cannot invalidate a newly reopened view. History pages retain complete message bodies.

Debate rounds retain every earlier round. Later stages receive the relevant advisor request and answer alongside the worker's result, even when the worker omits those details. Explicit continuations preserve the previous task, project context, all step outcomes and retained artifact metadata, together with the new caller context. Shared step evidence appears once per handoff.

For direct delegation, start with `council_run` and then use one `council_wait`, or run `council wait <id>` from the CLI. The server holds the wait until completion or the chosen deadline. It makes no additional model calls while idle. Council controls its own scheduler; the host controls when its orchestrating model runs and how long a tool request may remain open. For a manually registered Codex MCP server, the [host configuration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli) supports a longer tool deadline:

```toml
[mcp_servers.council]
# Keep the existing command and args.
tool_timeout_sec = 660
```

This setting is an example for a ten-minute Council wait. It is not written to your host configuration automatically. If the host cannot hold a long request, use the local UI to observe autonomous work instead of spending model turns on status polling.

`council_wait` returns the complete selected answer and every step's status metadata from a consistent run snapshot. Step bodies stay in the full run resource to avoid repeating their tokens. `council wait` exits nonzero for failed, cancelled or still-pending work; a wait timeout leaves the workflow running.

Embedded callers of `service.completion(id)` receive independent copies of full results. Editing one returned object cannot alter another consumer's task, steps, retained artifact metadata or status. Artifact cleanup checks its complete indexed cleanup history, so an earlier successful cleanup remains an idempotent no-op even after many later events.

## Private one-to-one coordination

Five workers can pursue their tasks while two with overlapping responsibilities coordinate in a separate pair thread. Open **Private chats** to inspect pair messages as the local human. Participant/task-instance labels and room filters keep their delivery separate from global room history; labels are coordination metadata, not authenticated identities or an operating-system secrecy boundary.

Enable **Private peer exchanges per task (optional)** when creating a room, or configure explicit workflow `privateChats` peers and request quotas. Managed workers yield an exact `council_private_chat_request` JSON envelope; Council gives the peer a separate read-only reply turn and resumes the sender in its existing workspace. Each request reserves two possible calls. Raw pair turns are kept in the private-chat database and excluded from shared workflow lineage, room history and public advisor prompts. Existing call limits, exact settings, cancellation and subscription protection still apply.

Connected host agents can instead use `council_chat_create`, `council_chat_send`, `council_chat_list`, `council_chat_actors`, `council_chat_history` and `council_chat_watch`. The watch blocks in the server without model polling. Messages do not inject into an already-running vendor session, modify room instructions or approve work. Read the [protocol, scope and examples](docs/private-chats.md).

## Human or agent management

Manage Council directly, or ask the connected host agent to act as your executive delegate: set up the board, select agents, follow explicit tracked rooms and report material decisions or blockers to you. The management mandate and report inbox are durable. Reports stay separate from room instructions, and explicit human responses do not automatically change task permissions or approve a room plan.

Open **Human delegate** to save a mandate, track rooms, read or answer executive reports, and use **Prepare agent handoff** to generate a prompt for the connected host. Its live badge shows open decisions and blockers without loading their complete report bodies into routine updates.

`council_delegate_status`, `council_delegate_configure`, `council_delegate_report`, `council_delegate_reports`, `council_delegate_resolve`, `council_delegate_changes` and `council_delegate_watch` expose this workflow over MCP. The watch waits in the server for actionable changes while suppressing ordinary active progress, so the host can yield instead of polling. The change feed retrieves replies to earlier reports without rereading the full inbox. Switching to human management changes the coordinator; use the existing Pause or Stop controls when you also intend to affect room execution.

Agent management uses the connected external host. It does not spawn a vendor process, keep the host model running after disconnection or push into another vendor chat. Labels and mandates are coordination metadata, not authenticated identities or new grants of permission. Read the [CEO/board workflow](docs/delegation.md) or use the [Human delegate skill](skills/council-human-delegate/SKILL.md).

## Model profiles

Profiles may name an exact `model`, supported `effort`, and a default `authority` for room setup. Room settings and `agentSettings` override model/effort for that work without changing the profile defaults. Multiple aliases can target the same harness with different models. Model IDs are passed unchanged; Council does not substitute another model after a failure.

New workflows snapshot the configured model, effort and effective skills of their actual participants, judge and advisors when admitted. Later profile edits cannot change those recorded choices. Unconfigured native model/effort defaults remain unrecorded; historical runs may also omit inherited settings.

## Billing, limits and appropriately sized models

Open **Agents & billing** in the dashboard to record each alias's billing mode (`api`, `subscription`, `local` or `unknown`), plan, optional token prices and allowance windows. Windows have user-defined labels and units; Council does not assume that a subscription has a particular weekly limit. Enter an observation time and either a reset time or validity bound. Expired, future or unbounded observations remain visible as historical data and become unknown for routing. A passed reset time never implies replenishment. Blank quantities and prices mean unknown, including API accounts with no usage report.

Enable **Stop at subscription limit** per alias to block Council calls when included allowance is exhausted or cannot be verified. Classify included windows as `subscription` and paid balances as `credits`; paid credits never count toward the guard's allowance. An observed zero stays blocked across expired/reset dates until a fresh positive report arrives. The guard checks queued dispatches and active calls, including room tasks, advisors and private peer exchanges. It uses explicit reports rather than live provider probes and cannot impose a provider-side spending cap; see the [protection contract and limits](docs/billing-format.md#stop-at-included-subscription-limits).

Link aliases with the same **shared account key** when they draw from the same allowance. Their quota is stored once, while model prices, skills and model-routing preferences stay separate per alias. Shared-account edits invalidate linked views' revisions, so an obsolete form cannot overwrite a newer report. Account links are explicit; matching harness names do not establish account identity. Billing state stays in `.council/billing.sqlite`, outside Git and release packages.

Provider observations can be imported through `council_billing_import` or `council billing-import --agent <alias> --input-file <snapshot.json>`. Use a normalized report from an available host connector or another trusted source and verify the alias-to-account link. Council does not read vendor credentials, scrape undocumented account APIs or pretend that every provider exposes live limits. The [billing format](docs/billing-format.md) describes manual edits, reported observations and freshness. Configured prices are reference rates, not measured charges. Council does not decrement reported subscription allowance from an invented tokens-per-call estimate.

For economical model selection, configure each alias's tier, capabilities and comparative cost/quality ranks in that dialog. Lower cost rank and higher quality rank express your preferences; they are not prices or automatic intelligence measurements. Pin the intended model in the alias's project configuration, or supply an explicit per-run model. Multiple aliases can use the same harness with different exact models. Unpinned native defaults produce a routing warning because they can change outside Council.

```sh
council billing --agent quick
council billing-set --agent quick --input-file examples/billing-routing-small.json
council route-plan --task "Correct the known typo and verify it" --task-class routine --requires code --write
council route --task "Correct the known typo and verify it" --task-class routine --requires code --write
```

The `quick` alias must already exist in `council.config.json` with a model available to you. The [small-profile example](examples/billing-routing-small.json) contains preferences, with billing left unknown. `route-plan` makes no model or classifier calls. `route` selects, records its reasons, starts the chosen workflow and waits in the server. The four collaboration presets are `direct`, `review`, `parallel` and `council`; their call costs come from the actual workflow plan. The room creation helper previews a team and applies it only when requested; rooms retain their existing weighted discussion and task-execution behavior.

Routine economy routing prefers suitable configured small profiles. Task class comes from the caller's assessment of uncertainty, consequences and verification needs, rather than task length or model-name heuristics. Known current exhaustion excludes an account; unknown allowance is labelled and is never presented as verified capacity. Larger-tier fallback requires `--allow-escalation` (or `allowEscalation: true`). Explicit aliases, models and effort remain authoritative. Missing profiles, insufficient capabilities and invalid plans fail clearly instead of silently substituting another model.

## Collaboration skills

The plugin ships four worker collaboration skills:

- [Don't kill an ant with a bazooka](skills/dont-kill-an-ant-with-a-bazooka/SKILL.md): choose proportionate models, effort and collaboration, then wait without polling.
- [Implementation and review](skills/council-implementation-review/SKILL.md): implementation, independent read-only review and revision.
- [Independent perspectives](skills/council-independent-perspectives/SKILL.md): preserve independent analyses, evidence and material disagreement.
- [Focused advisor](skills/council-advisor/SKILL.md): resolve a concrete uncertainty with read-only cross-model advice.

[Human delegate](skills/council-human-delegate/SKILL.md) is a separate host orchestration skill for delegated project management and reporting. It is not part of the worker skill catalog and is never injected into worker prompts.

Assign skills in **Agents & billing**, through an agent profile's `skills` array, or per run with `agentSettings[alias].skills` / CLI `--skills`. Room members also accept explicit skill arrays. A local `null` assignment inherits project defaults; `[]` disables inherited skills. Only the selected skills' short worker guidance is injected into that agent's prompt. Host orchestration instructions are not injected into workers, and skills never expand task permissions or permit recursive delegation. Effective assignments are snapshotted in run input. Routing and configured billing observations are recorded atomically with admission; billing observations are not charge receipts.

The economy skill chooses Council workers. It cannot change the model already running the host conversation, the host's scheduling policy or the provider's own context and usage rules.

The [open-source comparison](docs/oss-comparison.md) records design references from OpenHands, Ruflo, AutoGen, CrewAI, LangGraph and oh-my-claudecode, with dated GitHub popularity snapshots.

## Cross-model advisors and explicit reasoning effort

An independent consultation is available through `council_advise` or:

```sh
council advise --requester junie --advisor codex --model gpt-6.1-sol --effort high --question "Review this decision and explain its risks."
```

A worker may request advice during a workflow when its advisor mapping is enabled:

```json
{
  "mode": "delegate",
  "agents": ["junie"],
  "permission": "read",
  "task": "Investigate the issue; ask the configured advisor when an independent opinion is needed.",
  "agentSettings": {"codex": {"model": "gpt-6.1-sol", "effort": "high"}},
  "advisors": {"junie": {"agent": "codex", "maxRequests": 1}}
}
```

The worker yields a complete `{"council_advisor_request":{"question":"...","context":"..."}}` answer. Council releases its call slot, invokes the advisor read-only, and resumes the worker in the same workspace with the full advice. The request quota is configurable; each possible consultation reserves two additional calls for advice and resumption. Advisors cannot recurse, permission is never expanded, and a failed consultation cannot turn into a successful task. The UI exposes the advisor and its request quota per member.

Cancelling a blocking `council_advise` request also cancels the read-only job it just created. A normal wait timeout returns its run ID and leaves that job available for a later wait. Cancelling `council_wait` stops only that wait.

Effort is sent through native Codex, Claude, Antigravity or Junie's supported CLI options. Unsupported harness/effort combinations fail before dispatch. Higher effort can use more reasoning tokens; select it deliberately rather than treating it as a voting weight. Current [Codex configuration guidance](https://learn.chatgpt.com/docs/config-file/config-reference) describes model-dependent reasoning levels.

## Workflow modes

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

Independent assignments are optional. Without them, parallel participants address the same brief, which is useful for comparing alternative solutions. Handoffs preserve full project and peer evidence by default. Configured advisor quotas add their reserved consultation/resumption calls to the mode's ordinary call bound.

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
| `council_wait`, `council_advise` | Server-side waiting and independent cross-harness advice |
| `council_billing`, `council_billing_update`, `council_billing_import` | Local billing, shared-account quotas and explicit observations |
| `council_skills`, `council_route_plan`, `council_route_run` | Assignable collaboration guidance and deterministic cost-aware routing |
| `council_ui`, `council_rooms`, `council_room_create`, `council_room_status` | Local project rooms and dashboard |
| `council_room_message`, `council_room_task`, `council_room_start`, `council_room_execute`, `council_room_cancel`, `council_room_history` | Project-manager instructions, autonomous task execution and complete history |
| `council_continue`, `council_patches`, `council_cleanup` | Explicit handoff, patch refresh and reviewed cleanup |
| `council_events` | Compact events with an incremental cursor |
| `council_run_page` | Paginated run summaries with an opaque cursor |
| `council_health`, `council_backup`, `council_prune` | Health, consistent backup and preview-first retention |
| `council_send`, `council_inbox`, `council_inbox_page`, `council_ack` | Local peer handoffs and complete paginated history |
| `council_reserve`, `council_release`, `council_leases` | Advisory file ownership |
| `council_context` | Shared project notes |

Status waits are bounded to 30 seconds. Run summaries cap serialized data at 64 KiB and mark omitted entries; retained artifact paths remain complete. `includeResults: true` or `council://runs/{id}` retrieves the full record. `council://modes` describes modes and workspace bounds. The stdio server implements the tools/resources MCP subset, validated against the official SDK client; MCP over HTTP, sampling and interactive elicitation are not offered.

Default runtime budgets: 3 simultaneous calls per workspace, 16 calls per run, 180 seconds per call and 128 KiB final output per call. Task text and shared context have no built-in character ceiling; full context is the default. An explicit `contextLimit` is checked rather than used to truncate content. Configuration, MCP framing, structural validation, output retention and protocol traffic retain their separately reported resource guards. Exceeding a configured output budget fails the task instead of reporting a shortened answer as completed.

Round counts and call budgets accept positive safe integers rather than fixed workflow caps. Plans reserve the exact worst-case call count before dispatch, including configured advisor requests, without allocating a stage entry for every debate round. Peer messages and project notes also preserve complete text up to their explicit transport and stored-record byte guards; the former 32 KiB/64 KiB text caps do not apply.

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
