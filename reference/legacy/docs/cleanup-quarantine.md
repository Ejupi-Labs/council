# Cleanup quarantine

Council records an incident when an owned agent or Git helper cannot establish process cleanup. This is a report of uncertainty. Inspect the operating-system processes and retained worktrees before starting replacement work or accepting changes.

Committed incidents block later shared dispatch. Active hosts observe the block and request cancellation of their owned calls; those calls keep their slots until they settle. Failed cleanup takes precedence over late cancellation, and automatic patch publication remains pending or failed for inspection. Council never merges retained changes or replays unfinished tasks.

## Inspect the block

Run from the affected project, or supply its explicit workspace:

```sh
council quarantine --workspace "/absolute/path/to/project" --limit 25
council health --workspace "/absolute/path/to/project" --integrity
```

Incident pages contain fixed metadata: stage, reporting instance, optional workflow ID, observed time and generation. They do not contain prompts, private chat bodies or vendor diagnostics. The reporting Council PID is captured from the validated host at report time when available. It is an inspection clue, not proof that the PID is still alive, the identity of a descendant, or permission to terminate a process; unknown historical values stay unknown. Follow `nextCursor` unchanged with the same workspace to inspect more open incidents. Revisions and cursors are decimal strings; preserve every digit.

The dashboard shows shared and local refusal separately and provides a read-only incident view. `council_quarantine` exposes the same bounded inspection over MCP, and authenticated `GET /api/quarantine` serves the local dashboard. These interfaces cannot acknowledge incidents. A fresh inspection host can read shared state without accepting new work.

## Stop, inspect and acknowledge offline

1. Stop every Council host using the workspace. Let owned work drain and inspect remaining OS processes; a dead Council PID alone does not prove its descendants stopped.
2. Preserve the current state and matching repository/worktree artifacts before repairs. Inspect each referenced workflow and retained worktree. Resolve uncertain process ownership before accepting a patch or starting another task.
3. Read the current incident revision. After completing that inspection, run the explicit offline acknowledgement from the project directory:

```sh
council quarantine-ack --revision EXACT_REVISION --reviewed
```

Or add `--workspace "/absolute/path/to/project"`. Replace `EXACT_REVISION` with the returned decimal string. This command opens an inspection store without registering a Council host. It refuses live hosts, call slots, waiters and owned unfinished workflow records after conservative dead-owner recovery. A stale revision cannot clear a later report.

Acknowledgement atomically records an audit receipt and clears only the reviewed shared dispatch block. Exact retries reuse that receipt. It does not change workflow outcomes, repair patches, remove files, restart hosts or resume tasks. Ownerless unfinished records remain unchanged and their count is reported as `unownedUnfinishedRuns`; inspection must cover their possible work too.

Start fresh hosts after acknowledgement, then request any new work explicitly. The previous hosts' in-memory fences are never automatically lifted. If another actual cleanup failure is reported, a new generation blocks dispatch again.

## Limits and failure evidence

The slot transaction orders new admissions against committed incidents. An incident cannot retroactively stop an OS launch already admitted, and cancellation is not proof of immediate quiescence. Abrupt owner death before a report is committed remains a separate recovery boundary.

If SQLite cannot persist a report, the affected host stays fenced and reports: “The cleanup incident could not be persisted. Other hosts may be unaware; a shared dispatch block is not guaranteed.” Stop the other hosts and preserve the failed host's evidence. A new process cannot reconstruct that unpersisted fault from an empty incident list.

Incidents and acknowledgement history are retained independently of instance and workflow deletion. A restored unresolved incident stays blocked. Stop older hosts before upgrading to schema 6; older running code cannot enforce the new protocol. Council coordinates trusted local processes under one OS account and does not provide a universal OS sandbox.
