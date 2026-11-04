# Council operations

Run these examples from the Council installation directory. Replace the project, backup and run-ID placeholders. Add an absolute path to `dist/cli.js` when running from another directory. Node.js 24.18.1 or newer with SQLite 3.51.3 or newer is required; use a current maintained Node patch release.

## Supported deployment model

Council coordinates trusted local processes for one operating-system user. Hosts for the same canonical workspace share SQLite state and agent-call limits. It is a local stdio integration, not an authenticated network service or a boundary between mutually untrusted users. Actor names and advisory file leases do not authenticate a process.

Keep state on a local disk, with every sharing process on the same machine. SQLite WAL requires shared memory and [does not support network filesystems](https://sqlite.org/wal.html).

Council refuses older SQLite runtimes before opening state because its multi-process workload uses WAL. SQLite documents a rare [WAL reset corruption race fixed in 3.51.3](https://sqlite.org/wal.html#walresetbug). Council uses that version as its minimum baseline rather than detecting vendor-specific backports. Health reports the embedded SQLite version.

Use native vendor account configuration. `doctor` inspects launch prerequisites without making model calls; availability does not prove authentication or model access. Keep workspace permissions and backups restricted to the intended user: saved tasks, peer replies and notes are local plaintext data.

Read policy and retained Git worktrees are not a universal OS sandbox. The ACP file API checks project containment, size, metadata paths and static symlink/junction escapes. It is **not raceproof against a concurrent process with the same user privileges retargeting an ancestor symlink or junction** between checks and filesystem operations. Use a separately configured OS account, container or VM when the repository or another local process is untrusted. Council does not provide or certify that isolation.

## Upgrade to schema 5

1. Stop every older Council MCP host and CLI process using the workspace. Let owned agent processes drain; inspect any remaining work before proceeding.
2. Before opening the state with the new build, make an offline copy of the entire configured state directory, normally `.council`, to an unused backup directory. Older builds may not have the `backup` command. Preserve the matching repository and Git worktree metadata if retained artifacts matter.
3. Install/build the same release for all hosts. The first state open migrates older supported schemas transactionally to schema 5. Do not run older hosts against the upgraded database. Schema 5 adds durable call waiters and instance ownership for internal maintenance leases; existing transcripts and ordinary user file leases remain intact.
4. Check the resulting schema and integrity before accepting new work:

```sh
node dist/cli.js health --workspace "/absolute/path/to/project" --integrity
node dist/cli.js doctor --workspace "/absolute/path/to/project"
```

`health` reports state/WAL sizes, counts, limits, memory, heartbeat and storage issues. `--integrity` performs SQLite's quick integrity check; expect `integrity: ["ok"]`. A degraded health result exits nonzero. A fresh CLI health command describes its own process; use the connected host's `council_health` tool to inspect that running MCP host. Neither check certifies provider access or production security.

`state.counts.slotWaiters` exposes queued calls. Admission reserves free capacity for older waiters so a fast pipeline cannot repeatedly overtake a waiting peer; available parallel capacity can serve the eligible waiter prefix. A host advertises its configured concurrency limit at startup. Call limits bound execution, while queue waiting has no separate deadline; cancel unwanted work explicitly.

Short SQLite BUSY/LOCKED heartbeat failures retry within five seconds, with `heartbeatContentions` showing outstanding contention incidents. Confirmed ownership loss and non-contention heartbeat errors degrade immediately. Shutdown also retries brief unregister contention after work drains. Persistent failure is reported instead of silently releasing ownership. `filesystem` reports pending OS metadata lookups and their process-wide limit of 128; cancelled lookups remain counted until the underlying OS request settles.

Current-schema WAL opens avoid migration write locks. Cold initialization and upgrades allow five-second SQLite lock waits before restoring the 250 ms runtime bound. The CLI uses `CouncilService.open`, an asynchronous factory that retries brief state initialization contention before any workflow is accepted; embedded integrations can use the same factory. Initialization retries never replay agent calls. Large migrations and OS filesystem delays remain separate from lock-wait budgets.

## Observe, cancel and recover

Prefer summaries during routine monitoring. Full records remain available with `show` and the MCP run resource.

```sh
node dist/cli.js runs --workspace "/absolute/path/to/project" --summary --limit 20
node dist/cli.js events RUN_ID --workspace "/absolute/path/to/project" --summary --after 0 --limit 100
node dist/cli.js show RUN_ID --workspace "/absolute/path/to/project"
node dist/cli.js cancel RUN_ID --workspace "/absolute/path/to/project"
node dist/cli.js recover --workspace "/absolute/path/to/project"
```

`runs --summary` returns a page and an optional `nextCursor`; pass it back with `--cursor`. Summary events return an incremental cursor. Cancellation is durable across hosts and does not roll back edits. `recover` marks work owned by proven-dead processes as failed; it never replays tasks. PID reuse is handled conservatively, so heartbeat age alone does not authorize taking over work.

Workflow evidence or slot-state persistence failures, and failed heartbeat/ownership checks, degrade the running host and prevent new runs. Ordinary mail, context and lease errors are reported to their caller without automatically degrading the host. If a terminal write fails, the host retains an in-memory failed result with the latest available evidence; the durable record may lag behind. Shutdown can report an ownership-release failure while still closing SQLite. Stop that process, preserve the state directory, inspect storage, and recover only after the former owner is proven dead. A fresh health CLI cannot inspect another host's in-memory emergency results.

For mail history, use `council_inbox_page` with `agent`, `unreadOnly` and optional `limit`. Continue with `nextCursor` using the same recipient and filter; pages contain complete bodies and at most 1 MiB of serialized messages. Acknowledge processed messages with `council_ack`. Start a fresh query after reaching the end to discover later messages. For concurrent tasks using the same agent, use unique lease owners and supply `runId` to `council_release` to restrict release to that task.

For a follow-up, use `continue RUN_ID --task "New explicit task"` and select the intended participants. This starts a new workflow with bounded prior evidence; it does not resume a vendor session or automatically repeat unfinished writes.

## Back up and restore

Create the destination's parent directory first and choose a filename that does not exist:

```sh
node dist/cli.js backup --workspace "/absolute/path/to/project" --output "/absolute/path/to/backups/project-state.db"
```

Backup uses SQLite's online backup API and publishes a consistent database snapshot exclusively. It refuses existing destinations and active database/journal paths. It may run while trusted hosts are active. It copies database state, not project files, retained worktrees, patch files, manifests or vendor credentials. Preserve a separate matching filesystem snapshot when those artifacts are needed. Do not copy only a live `state.db` while ignoring its WAL.

Each backup pins a separate read snapshot near the start of the operation; later writes continue on the application connection and stay outside that snapshot. This prevents ongoing writers from repeatedly restarting the copy. Cancellation and a 60-second budget are checked between copy steps. SQLite retains WAL pages needed by the reader until backup cleanup finishes, so allow temporary WAL headroom under write pressure.

To restore:

1. Stop **all** Council hosts and remaining owned agent work for this workspace.
2. Preserve the current state directory as a separate offline safety copy. Move it aside instead of overwriting a database with potentially stale `-wal` or `-shm` files beside it.
3. Create a fresh configured state directory and copy the selected database snapshot into it as `state.db`. Restore retained artifacts only from the matching filesystem snapshot, keeping their expected repository paths and Git metadata. A database backup alone cannot reconstruct them.
4. Use a build compatible with the backup schema. Run `health --integrity`, inspect runs/events, and use `recover` for proven-dead owners before enabling new work. Review artifacts before any cleanup or replacement task.

## Retention and reviewed worktree cleanup

Pruning is explicit and previews by default. Supply an ISO timestamp with a timezone and review the candidate IDs:

```sh
node dist/cli.js prune --workspace "/absolute/path/to/project" --before "2026-10-01T00:00:00Z" --keep-latest 100
node dist/cli.js prune --workspace "/absolute/path/to/project" --before "2026-10-01T00:00:00Z" --keep-latest 100 --execute
```

The default preserves the newest 100 runs. Only older terminal runs without retained artifacts are candidates; active runs and records with artifacts, including pending ones, are protected. Each invocation considers at most 1,000 candidates. Execution deletes selected run records and their events; it does not remove messages, project context or worktree directories. Physical database size need not shrink immediately after deletion.

Inspect each retained worktree and patch before requesting cleanup:

```sh
node dist/cli.js cleanup RUN_ID --workspace "/absolute/path/to/project" --reviewed
```

Dirty work requires the additional explicit `--discard` flag. Cleanup retains patch files and reconciles an interrupted Git removal when its managed worktree and registration are both absent. It refuses arbitrary paths and does not remove the original checkout. Cleaning a worktree does not automatically make its historical run eligible for pruning.

Internal cleanup/patch-repair leases are bound to their Council instance. Graceful unregister or proven-dead owner recovery releases them; ordinary user file leases keep their expiry semantics. Recovery never removes a maintenance lease merely because a live owner's heartbeat is old.

Patch formatting ignores repository display settings and inherited Git repository/index/config redirection, while preserving normal network and authentication environment. Dirty tracked or untracked submodule contents require separate review and export; parent patch publication and ordinary cleanup refuse to hide them. Clean gitlink updates still require the referenced nested commits to be available when applied. Worktrees for nested projects require that folder to exist in the captured commit.

To regenerate a missing, pending or stale patch for an existing terminal run:

```sh
node dist/cli.js patches RUN_ID --workspace "/absolute/path/to/project"
node dist/cli.js show RUN_ID --workspace "/absolute/path/to/project"
```

The MCP equivalent is `council_patches` with the run ID. This operation verifies the retained worktrees, exports their current changes against each captured base, and updates artifact status with a `workspace.refreshed` event. It does not call agents or change the original workflow outcome. Review the resulting patch files before cleanup or application. `export RUN_ID --output review.md` creates a Markdown report; it does not regenerate Git patches.

Patch repair and cleanup share an exclusive maintenance lease. Concurrent maintenance fails with a reservation conflict; retry after the first operation settles. Exports commit one artifact at a time, so a failure preserves earlier successful repairs. An active run must finish first. If a preparation intent points to a nonexistent worktree, repair stops with inspection instructions and does not create a replacement checkout. Preserve the record, inspect `git worktree list` and the matching manifest, and recover the repository manually before trying again. Redirected metadata is refused. A removed worktree cannot be reconstructed from database state alone.

## Shutdown and pending artifacts

Graceful SIGINT/SIGTERM, MCP input closure and cancellation requests stop bundled agent work and drain supervised descendants. Windows or an abrupt OS kill may prevent a final response; inspect/recover state on the next start. Avoid starting a replacement merely because its former host disappeared.

Windows tree cleanup allows five seconds for each native helper, coalesces overlapping attempts, and preserves a requested cooperative stop before ordered escalation. When OS helpers fail, stop/escalation can take up to 16 seconds. Unverified cleanup is reported explicitly; inspect remaining processes and retained worktrees before starting a replacement. A signal being sent alone is not proof that every descendant has stopped.

Git create/export/cleanup operations have a 60-second operation deadline. Final patch exports share a 60-second budget, shortened to five seconds after run cancellation, followed by process cleanup. Maintenance is also aborted during service shutdown. Kernel or filesystem failures can still delay OS operations; these are bounded application policies, not universal OS guarantees.

On Windows, rename operations for manifest replacements, patches and ACP files retry transient sharing refusals for up to one second. Initial exclusive manifest publication uses a hard link and reports link errors directly. Retries preserve the existing target and honor cancellation; they never delete the target or change its permissions as a fallback. A mutation already issued to the OS must settle before cleanup. Cancellation observed after a completed publication still returns a cancelled outcome, so inspect retained artifacts before retrying.

An artifact with `exportStatus: pending` is retained for inspection and later export. Its path may not exist yet if preparation failed before checkout. Read `exportError` and the run error; do not assume that `changed: false` means a pending worktree has no edits. Preserve its record and files while investigating. A failed or cancelled run is never successful completion.

Custom `AgentRunner` implementations must honor cancellation and settle only after their work is quiescent. Returning early while a child keeps running would release a shared call slot and allow cleanup to race that work. Council cannot safely force-stop an arbitrary injected implementation that ignores this contract.

For an offline, machine-labeled efficiency comparison without accessing existing project state, run `npm run benchmark` from a development checkout with development dependencies installed. The benchmark source and build tools are not included in the runtime release. Results depend on machine load and are not a production certification.
