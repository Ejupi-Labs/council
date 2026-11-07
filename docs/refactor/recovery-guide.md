# Pause and recovery guide

Council can pause a task, a member of a task group, or the whole group. Pause stops new work in the selected scope while already admitted work settles. A checkpoint records the boundary, retained work and current blockers. Pause does not suspend a provider process or cancel work already in progress. Use Stop when you want Council to request cancellation; cancellation and cleanup remain separate observed states.

## Pause and resume

In the local UI, open a task or task group and use **Pause task**, **Pause this group member**, **Pause member**, or **Pause group**. The checkpoint shows **pausing** while admitted work or unresolved cleanup remains, then **paused** when the selected scope has safely drained. Inspect it for retained work, blockers and the next action. A member pause does not silently pause unrelated siblings, but a paused prerequisite prevents new dependent work from consuming it.

Resume is an explicit action on a paused checkpoint. Council checks current task and group versions, ownership, dependencies, evidence, Stop, capacity and the shared budget/window before admitting eligible unfinished work. It does not refill the allowance or repeat completed calls or changes. Group and member pauses remain independent: resuming one does not clear the other. If authority changed or a budget expired, inspect the reported blocker before choosing a new action.

A checkpoint survives an owner restart. Restart observes the retained state; it does not resume the checkpoint or replay a worker. Historical failed, cancelled or partial outcomes remain distinct from successful work.

## CLI controls

Use `--json` to inspect structured results and revisions. The CLI and UI address the same local owner and retained records. The required `scope` object for resume contains the current authority for the exact task, member or group; it is not interchangeable with a different checkpoint or a stale selection. `PATH|-` accepts a file or standard input.

```text
council tasks pause TASK_ID --task-version V --expected-revision R [--request-id ID]
council graphs pause GRAPH_ID --graph-version V --graph-control-revision R
  --budget-authority-epoch E --window-number W --node-authorities-file PATH|- [--request-id ID]
council checkpoints get CHECKPOINT_ID
council checkpoints resume CHECKPOINT_ID --checkpoint-revision R --scope-file PATH|- [--request-id ID]
council incidents get INCIDENT_ID
council incidents reconcile INCIDENT_ID --incident-revision R [--request-id ID]
```

`tasks pause` can select a standalone task or an existing group member; use `graphs pause` for the whole group. Get the current task/group authority and checkpoint revision before preparing a resume request. An old revision or changed dependency is a conflict, not permission to substitute a newer target.

For a lost acknowledgement, retain the original request ID and exact input and retry that same request. Council returns the recorded receipt or reports a conflict; a retry is not a new execution. Status, waits, checkpoint reads and incident inspection do not call a model.

## Incidents and evidence

The UI can show a task's incident or **Inspect global recovery incident** when shared dispatch is fenced. Read the incident, its revision, blockers and next action before choosing **Reconcile incident**. The CLI equivalents are `incidents get` and `incidents reconcile`. Reconciliation asks the current Council owner to verify retained evidence; it does not accept a user's statement or a process ID as proof, and it never starts work itself.

The current positive proof path is deliberately narrow: supported watchdog-expiry incidents for a native graph producer with a verified output closure, or a standalone native author with a sealed, complete source/candidate closure. In both cases Council requires the exact retained claim and owner history, a recorded verified cleanup of the owned process tree, complete consistent terminal/effect evidence and freshly verified retained artifacts. The original outcome remains unchanged. If another incident, claim or effect still blocks the shared gate, that blocker remains. A reconciled incident with a remaining known fence can be explicitly rechecked; a read or restart does not open the gate.

Missing, corrupt, incomplete or unsupported proof leaves the incident unresolved and conflicting dispatch fenced. The current proof paths do not automatically resolve general storage failures, unknown owner epochs, arbitrary local-command or external-publication effects, or an old process whose ownership and complete cleanup cannot be established. Council cannot guarantee the effects of arbitrary activity outside its managed scope or claim a universal operating-system sandbox.

## Owner restarts and older state

Before serving work, Council records a durable owner epoch. An orderly shutdown records a clean handoff only after the owner has stopped its scheduler and workers, closed owned handles, and completed the required durable checks. A known unresolved incident can remain fenced across a clean handoff. If an owner disappears without a proven clean handoff, Council records an `owner_epoch_uncertain` incident and keeps dispatch fenced; it does not infer that old work or process cleanup completed. A chain of recorded clean owner handoffs can support a later explicit recheck, but a dirty or untracked epoch cannot be treated as a clean chain.

Existing v6 state predates owner-epoch tracking. Its old shutdown acknowledgement was recorded before all cleanup and handle closure, so an upgrade cannot use that acknowledgement alone as proof of a clean handoff. Council conservatively reports an unknown prior epoch and retains a fence for such state. Inspect the incident and retained work; there is currently no automatic proof path that clears a genuinely unknown epoch. Council does not silently delete or migrate legacy collaboration data, and creating a separate state directory does not prove that an old full-access process cannot touch the same project.

These capabilities are qualified with controlled offline agents. They do not establish live provider availability, subscription capacity, model usage savings or provider-session continuity. Evidence export, backup, restore and reviewed artifact cleanup are planned for the next feature, not part of this recovery slice.
