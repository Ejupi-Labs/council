# Working with project rooms

A room keeps a project brief, a team, explicit task permissions, dependencies, decisions and complete result history together. Use it for a bounded piece of work: diagnose a defect, implement a focused change, or collect independent reviews. Keep unrelated projects in separate rooms.

Room creation and inspection make no model calls. Discussion and execution use the configured agent profiles and their existing authentication. Run `council doctor --workspace /absolute/path/to/project` first to check executable availability; that check does not verify account access. Use exact configured aliases and models that your account can run.

## Create a reviewable plan

Start from [the diagnosis-and-fix example](../examples/room-fix.json). Replace the generic defect description with a reproduction, expected behavior and acceptance criteria before starting discussion. Its two configured aliases are examples; replace them if your project uses other profiles. Save the edited file as `room-fix.json` in the target project, or pass an absolute path. Relative `--input-file` paths resolve from the selected workspace.

```sh
council room-create --workspace /absolute/path/to/project --input-file room-fix.json
council rooms --workspace /absolute/path/to/project
council room-show ROOM_ID --workspace /absolute/path/to/project
```

Copy the ID from the creation result into later commands. `rooms` gives compact metadata; `room-show` returns task identities, retained workflow/worktree metadata, the latest complete-message preview, and any decision or error. Earlier messages remain available through `room-history`.

Tasks receive stable IDs in creation order: `task-1`, `task-2`, and so on. `dependsOn` names those IDs. A dependency becomes ready only when its prerequisites complete. Independent tasks can use available concurrency while another task is still running. Missing, failed or unverified prerequisite evidence stops dependent admission.

The example uses `autoExecute: false`, so discussion stops at an approved ready plan:

```sh
council room-discuss ROOM_ID --workspace /absolute/path/to/project
council room-show ROOM_ID --workspace /absolute/path/to/project
council room-execute ROOM_ID --workspace /absolute/path/to/project
```

Each voting member must return a valid ballot. Approval requires more than half the total decision weight. The highest-weight approving proposal supplies the assignments; the lead and then original member order break equal-weight assignment ties. The full rationale and dissent remain in history. Weights express your decision policy and do not prove correctness. Advisor-only members do not vote or receive task assignments.

`room-discuss`, `room-execute` and a work-starting `room-resume` keep their locally owned work in the foreground until it reaches a stable state. Leave that process running; Ctrl+C cancels its admitted work and preserves partial evidence. For unattended coordination use the persistent MCP host or dashboard, then wait in the server instead of asking a model to poll repeatedly.

Starting commands report success only for their intended outcome: execution must complete; discussion or resumption must reach an approved ready plan or completed work. An intervening pause, failure, cancellation or request for attention is reported as incomplete with a nonzero exit code. `room-wait` instead reports whether its wait reached a stable state; check the returned room status, and treat `timedOut: true` as unfinished work. Read commands return the stored evidence without claiming task success.

Set `autoExecute: true` only when the task permissions and scope already authorize execution after approval. A majority cannot expand those permissions. Member model, effort, skill, advisor and private-exchange settings remain subject to the configured call budget and provider access.

## Implement and review one change in the same worktree

For one focused change, the `review` workflow gives read-only reviewers the author's actual isolated worktree, then lets the author address their findings in that same worktree:

```sh
council run --workspace /absolute/path/to/project --mode review --agents codex,claude --write --task "Implement the reproduced empty-input fix, have the reviewer inspect the changed files and acceptance evidence, then address its concrete findings and run the required checks. Do not merge or publish."
```

The first selected alias is the author. Later aliases review the changed files without editing them; the author performs the final revision. Inspect the final status, complete reports and patch before accepting changes.

Room dependencies retain separate workflow/worktree evidence. An ACP file service rooted in the original project can block access to another task's retained `.council` files. A dependent reader must report that access limitation honestly. Choose the shared-worktree review workflow when reviewers need direct access to one author's edits; room task graphs remain useful for independently scoped work and its explicit prerequisites.

## Read the evidence

```sh
council room-history ROOM_ID --workspace /absolute/path/to/project --limit 20
council show RUN_ID --workspace /absolute/path/to/project
council export RUN_ID --workspace /absolute/path/to/project --output review.md
```

Follow `nextCursor` with `room-history --cursor NUMBER` until it is `null` for the complete ascending history. Keep `nextAfter` from the last response and pass it as `--cursor` for a later explicit check; an empty check preserves that position. Reuse cursors with the same room. Pages preserve whole messages and normally use at most 1 MiB for their serialized messages. A single larger message is returned intact on its own page. The routine room preview keeps a bounded suffix; it does not limit the evidence retained for future tasks.

In the dashboard, finish loading the history pages, then use **Check newer messages** when needed. New complete messages append without rebuilding earlier messages or moving your reading position. The control makes one explicit read and does not poll or invoke models. Messages stored after the last check appear when you check again.

Read each task's `runId` and inspect its full workflow record. A brief preview or a room's approved decision is insufficient evidence that work passed its acceptance checks. Completed, failed and cancelled results remain distinct, including partial outputs and artifact-export errors.

When a discussion cannot complete, its attention message identifies the workflow and available public failure details. Each failed opinion retains its public error and workflow link alongside any partial output. Inspect that diagnostic before starting another discussion with the same unavailable profile or model. Private pair payloads remain outside shared room diagnostics.

Write tasks use isolated Git worktrees. Their patches and captured base commits are review artifacts. Another task's worktree is separate evidence: its changes are never automatically present in a dependent task's checkout. A reviewer must inspect the referenced patch/files within its tool policy and report blocked access or unverified integration honestly. Council never merges a room's changes automatically. Follow the [artifact inspection and reviewed cleanup procedure](operations.md#retention-and-reviewed-worktree-cleanup) when finished.

## Pause, redirect or stop work

```sh
council room-pause ROOM_ID --workspace /absolute/path/to/project
council room-wait ROOM_ID --workspace /absolute/path/to/project --wait-ms 600000
council room-resume ROOM_ID --workspace /absolute/path/to/project
council room-cancel ROOM_ID --workspace /absolute/path/to/project
```

Pause blocks the next room-workflow admission. Already admitted workflows drain, including internal calls queued for an execution slot. The paused checkpoint retains completed tasks, current approvals and the remaining work. Resume continues that checkpoint without replaying completed writes. A paused ready plan resumes to ready and still requires explicit execution. Stop requests cancellation of admitted work instead. Neither action rolls back edits.

To redirect the project, put the complete instruction in a JSON file:

```json
{ "author": "Project manager", "body": "Keep the current implementation evidence. Before continuing, revise the acceptance check to cover the reproduced empty-input failure." }
```

```sh
council room-message ROOM_ID --workspace /absolute/path/to/project --input-file guidance.json
```

A new project-manager message invalidates a ready or paused approval. During active work, admitted tasks retain their original instructions and drain; subsequent admission requires a fresh discussion. Use Stop if those tasks should also be cancelled. Private pair messages and executive-report replies do not change room instructions or authorize work; send a deliberate room instruction when the shared plan must change.

The same Council build and backend must own a resumed room. A room created with the mock backend remains a simulation and cannot later execute through native profiles. Paused checkpoints survive normal host restart. They do not authorize retrying failed or cancelled work.

## Extend completed work or recover a lost owner

A completed room can accept a new explicit task. For example, `follow-up.json` can contain:

```json
{
  "title": "Review the retained fix",
  "brief": "Inspect the retained task-2 workflow and patch. Check the stated acceptance evidence and report unresolved risks. Do not modify files or claim the patch has been merged.",
  "permission": "read",
  "dependsOn": ["task-2"]
}
```

```sh
council room-task ROOM_ID --workspace /absolute/path/to/project --input-file follow-up.json
council room-discuss ROOM_ID --workspace /absolute/path/to/project
```

The new discussion covers only pending work. Completed tasks and their retained reports/artifact identities remain available as prerequisites. Manual rooms again stop at ready before execution.

If an owning process disappears, preserve the state and inspect it before starting replacement work:

```sh
council room-show ROOM_ID --workspace /absolute/path/to/project
council room-recover ROOM_ID --workspace /absolute/path/to/project
council room-show ROOM_ID --workspace /absolute/path/to/project
```

Recovery settles only a proven-dead owner. A live PID, including conservative PID-reuse handling, prevents takeover. Recovery retains partial evidence and never replays a workflow. Server-side waiting also checks the awaited room's owner. Neither operation assumes that a stale heartbeat proves process death.

Failed and cancelled rooms retain history and cannot accept replacement tasks or resume. Inspect their task workflow IDs, full reports, pending exports and retained worktrees first. For a new read-only investigation, use an explicit continuation task:

```sh
council continue RUN_ID --workspace /absolute/path/to/project --task "Inspect this failed result and its retained artifacts read-only. Identify what completed, what remains unverified, and a safe next task. Do not repeat edits."
```

A continuation starts a new workflow; it does not resume the vendor session. Define any replacement work in a new room with fresh briefs, acceptance criteria and permissions. Review existing edits before authorizing overlapping writes.

## Connected-host equivalents

The main MCP tools are `council_rooms`, `council_room_create`, `council_room_status`, `council_room_history`, `council_room_message`, `council_room_task`, `council_room_start`, `council_room_execute`, `council_room_pause`, `council_room_resume` and `council_room_cancel`. Use the returned room ID and recorded task IDs rather than inventing identities.

`council_room_start` and `council_room_status` support a server-side `waitMs` window. Reuse `council_room_status` after a timeout while the persistent host continues to own work. Cancelling an ordinary status wait stops that wait; it does not cancel admitted room tasks. Use the explicit room cancellation control to stop them. For delegated management across several explicitly tracked rooms, use the [human-delegate workflow](delegation.md).

## Verify coordination without provider calls

From a development checkout, build once and run the offline process stress harness:

```sh
npm run build
node scripts/room-soak.mjs --duration-ms 60000
```

It captures an immutable runtime and uses two real Node worker processes with fake agents plus an observer. Each fixture checks foreign pause/resume, competing ownership claims, changed instructions, cancellation, failed-task draining and recovery after an owning process exits. Completed tasks must not replay. Both SQLite databases are checked after drain; every started worker must exit before successful completion is reported.

Logs and a final JSON summary are written under `.council/refinement/`. Successful fixtures are removed; failed or interrupted fixtures remain for inspection. Fixtures rotate after each five-scenario group and have a 128 MiB observed logical storage budget. The harness finishes its current group before a normal duration limit, so elapsed time can exceed the requested duration. This verifies offline coordination behavior and does not certify vendor authentication, model quality or platform sandboxing.
