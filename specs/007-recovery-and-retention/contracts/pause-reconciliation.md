# Pause, checkpoint, resume and incident reconciliation contract

Status: proposed 007 design. This contract defines new behavior to implement and qualify; the
accepted 006 runtime does not already provide these operations. It implements RCR-001–010,
RCR-020–025 and SC-001–003/007–009 for this slice. Scoped export, backup, restore and
reviewed artifact cleanup are mandatory in the next consecutive feature; their retained
contract is deferred research, not an active 007 contract or an accepted capability.

## Common rules

The routes below are relative to the existing `/api/v1` prefix and use the current local-owner
authentication and response envelope. A mutation accepts a strict typed JSON body with a fresh
opaque `requestId`; unknown fields, invalid IDs, missing required fields and unsafe numeric
values are rejected. IDs use the existing 32-hex form. Versions, revisions, epochs, counts and
window numbers use the existing checked canonical decimal strings. A request fingerprint includes
the operation, path identity and complete typed request. Controller-only process, evidence and
filesystem proofs are never accepted from the caller as facts.

Exact receipt replay is checked before current-owner, current-state, gate and filesystem
prerequisites. The same request ID with different operation, scope or input returns
`request_conflict` without any new work. A known pre-commit refusal creates no accepted receipt;
an uncertain commit returns `write_uncertain` and permits only the same request identity and
input for receipt reconciliation. Observations, failed requests, restarts and receipt replay never
dispatch work, resume an old process or repeat a filesystem effect. Existing failure, accepted
Stop, claim, budget and evidence rules retain precedence.

Responses identify the exact selected task or graph and use non-colour text for state and the next
permitted action. They never disclose process handles, environment, credentials, private physical
paths or internal proof bytes. A graph-owned task remains under its actual graph membership and
budget; selecting a member task does not create independent standalone authority.

## Pause and checkpoint

| Operation | HTTP | Required request fields | Result |
| --- | --- | --- | --- |
| `tasks.pause` | `POST /tasks/{taskId}/pause` | `requestId`, `taskVersion`, `expectedTaskRevision` | Exact task or graph-member scope, accepted boundary, durable `checkpointId`, state and retained admitted identities. |
| `graphs.pause` | `POST /graphs/{graphId}/pause` | `requestId`, `graphVersion`, `expectedGraphControlRevision`, `expectedBudgetAuthorityEpoch`, `expectedWindowNumber`, complete `nodeAuthorities` | Exact group scope, accepted boundary, durable `checkpointId`, state and retained admitted identities. |
| `checkpoints.get` | `GET /checkpoints/{checkpointId}` | Path identity | Current checkpoint and its scope, state, revision, completed and unfinished identities, available evidence, blockers and next action. |

`nodeAuthorities` uses the accepted 006 ordered vector of `{nodeId,taskVersion,
nodeControlRevision}` and must identify every declared node exactly once. The owner validates the
selected task's actual membership and, for a graph member, binds its checkpoint to the parent graph
and exact member identity. A member pause cannot authorize standalone execution or allow its
descendants to consume a stopped, stale or paused prerequisite. A group pause covers all group
members; a member pause does not silently pause unrelated sibling work. Each member and group
pause has its own scope identity, durable fence and checkpoint. Overlapping fences remain effective
independently; an action on one scope cannot consume or clear another scope's pause.

The owner may return an accepted `pausing` checkpoint while previously admitted work settles, or a
`paused` checkpoint when the exact scope is already drained. The returned boundary and checkpoint
identity are committed with the receipt. `pausing` means admission has closed; it does **not**
claim that a native process is suspended or already stopped. Already admitted work may finish and
its outcomes and evidence remain observable. Pausing itself grants no new native admission, budget,
delivery or continuation authority and does not cancel work. Accepted Stop and earlier
failure retain their own outcomes and precedence.

The acceptance transaction installs the scope fence and advances the relevant authority generation
before it acknowledges pause. Every later creation of an attempt, graph input, node admission or
workflow step under that scope must check the fence in the same transaction as that creation; a
previously captured controller fact or in-memory callback is insufficient. This includes automatic
routing replacements, graph input and local-operation preparation, graph continuation and new
delivery stages and broader delivery authority. Work already admitted at the boundary may drain;
recording its actual completion and evidence is not a new admission. A delivery stage admitted before
the boundary may obtain its required per-effect permits only under its original exact captured
authority; pause does not authorize a new stage or broaden that scope. Current owner, control,
instruction, dependency and budget guards remain in force. Completion and human acknowledgement may
retain their actual results while paused, but their callbacks cannot admit subsequent work. Reads,
status refresh, grants and owner restart cannot clear the fence.

The checkpoint retains the exact scope, accepted control boundary, immutable definition and
instruction identities, budget owner/epoch/window, current registration generation, admitted,
completed and unfinished work, known outcomes and associated evidence, and next action. While
admitted work drains, the checkpoint is revised from actual committed outcomes; after verified drain,
its paused snapshot is durable. A missing or uncertain checkpoint write cannot be advertised as a
safe pause or permit conflicting admission. Unverified cleanup, an active effect, a retained claim
or incomplete required evidence remains visibly blocked or unresolved. A stopped, failed,
cancelled or interrupted result is never relabelled successful by checkpoint finalization.

The selected task and graph read projections expose their current checkpoint identity, revision,
state and actionable blockers, so a user can find it without a separately remembered ID. These
additions do not rewrite historical task, graph, workflow, receipt or terminal records. Polling is
read-only and does not make model calls.

## Explicit resume

| Operation | HTTP | Required request fields | Result |
| --- | --- | --- | --- |
| `checkpoints.resume` | `POST /checkpoints/{checkpointId}/resume` | `requestId`, `expectedCheckpointRevision`, `scope` | A receipt identifying the prior checkpoint, resulting control state, any newly admitted unfinished identities and any blockers. |

`scope` is a strict discriminated object, matched against the checkpoint's actual membership:

- A standalone task uses `{kind:"task",taskId,taskVersion,expectedTaskRevision}`.
- A graph member uses `{kind:"member",taskId,taskVersion,expectedTaskRevision,graphId,graphVersion,expectedGraphControlRevision,nodeId,expectedNodeControlRevision,expectedBudgetAuthorityEpoch,expectedWindowNumber}`.
- A group uses `{kind:"graph",graphId,graphVersion,expectedGraphControlRevision,expectedBudgetAuthorityEpoch,expectedWindowNumber,nodeAuthorities}`.

The group vector is complete and ordered as for pause; a member remains subject to the current
parent group and dependency controls. The owner captures and verifies current dependency, input, workspace,
policy and evidence facts itself. These facts are not caller-supplied proof of safety. A stale or
incomplete tuple, a changed instruction/dependency, accepted Stop, expired or
exhausted budget, changed owner, unresolved conflicting incident or unverified evidence must refuse
or report a precise blocker before admission. It must never substitute a newer target or version.

Resume is a deliberate new control, not a synonym for owner recovery, grant, observation or retry
of the pause receipt. The resume transaction consumes the exact checkpoint and installs only the
current owner's freshly checked authority for the eligible unfinished scope. Existing task and graph
rules still decide whether a fresh explicit rebind, budget grant or user acknowledgement is needed.
Completed logical executions, accepted outputs and applied effects retain their original identities
and cannot be replayed or charged again. A permitted replacement is a distinct explicit operation
with a new attempt ID linked to its predecessor; it preserves the predecessor's failure, accepted
Stop, partial output and evidence. When member and group pauses overlap, an accepted member resume
clears only the exact member fence. The enclosing group fence remains active, and the resume result
reports the group blocker without admitting work in that scope. Only a separate authorized group
resume can clear that group fence; remaining member fences still block their members.

If a response is lost after resume commits, the exact receipt may be replayed but cannot re-run the
resume admission or an already committed effect. If the checkpoint or current authority changes
after inspection, the stale resume fails safely; the user reads a new checkpoint and chooses a new
intent. Restart keeps a retained checkpoint paused/interrupted and never performs this operation
implicitly.

## Incident inspection and reconciliation

| Operation | HTTP | Required request fields | Result |
| --- | --- | --- | --- |
| `incidents.get` | `GET /incidents/{incidentId}` | Path identity | Exact incident, revision, affected scope/attempt/claim, retained facts, missing checks and next available action. |
| `incidents.reconcile` | `POST /incidents/{incidentId}/reconciliations` | `requestId`, `expectedIncidentRevision`, `action` | Durable reconciliation record, actual observations, `reconciled` or `unresolved` outcome, remaining blockers and current fence/claim disposition. |

The initially supported action is `verify_and_reconcile`. It requests the owner's checks; it is not
a user assertion that cleanup or effects succeeded, permission to signal an arbitrary process, or
permission to rerun work. Both a successful reconciliation and a completed check with missing or
conflicting evidence are auditable outcomes. An unavailable proof is reported as unavailable, not
converted to success. The exact request receipt is safe to replay without repeating effects.

A positive reconciliation requires authoritative observations bound to the original owner and
attempt: the exact Council-owned process and complete owned process tree are verified terminated;
cleanup is verified; relevant durable effects and journals are complete and consistent; and required
retained evidence is complete, identity-bound and freshly verified. The proof must come from the real ownership/cleanup and evidence
paths. A PID alone, elapsed time, an absent response, a user's statement, an unverified child, a
reused process identity, or an unacknowledged write is not such proof. If the old process scope can
no longer be proven after a crash, report the missing proof and retain the fence; do not inspect,
kill or infer ownership of a potentially unrelated process. Conversely, when the full controlled
proof exists, the operation must be able to record it and reconcile rather than remaining
permanently blocked merely because an incident exists.

Reconciliation records the exact evidence, observations, owner/scope and decision atomically with
the incident revision and receipt. It releases only the matching claim or fence whose safety was
established. Before opening any shared gate it checks all other unresolved incidents, claims,
controls and effect journals; another blocker remains effective. If affected scope or independence
cannot be proven, retain the applicable global fence. Reconciliation does not start a model call,
resume a writer, publish, delete evidence or mark an interrupted/failed/cancelled attempt successful.

## CLI and UI parity

The CLI exposes `tasks pause TASK_ID`, `graphs pause GRAPH_ID`, `checkpoints get CHECKPOINT_ID`,
`checkpoints resume CHECKPOINT_ID`, `incidents get INCIDENT_ID` and `incidents reconcile INCIDENT_ID`.
Arguments supply the same exact revisions, authority tuple, action and optional explicit request ID
as their HTTP counterparts; the CLI does not infer a newer target on retry. Existing JSON envelopes,
exit conventions, saved client intent and explicit owner selection apply. A read/wait timeout ends
only the observation, not a pause, reconciliation or admitted worker.

The branded UI presents the same scope, checkpoint ID, status, blockers and exact next action;
pause and resume are separate from Stop. It keeps user drafts, selected tasks and pending exact
requests across stale replies or a lost acknowledgement. The user can inspect evidence and request
reconciliation without a model call. A later response cannot replace a newer selection or silently
submit a new action. Controls must be named and keyboard-operable, with visible focus, text state
labels and readable narrow layouts. Tests use actual local fake workers and exact saved receipts;
live provider qualification is not implied.

## Required boundaries and evidence

Offline acceptance exercises pause racing an admission, first GO, completion callback, human
acknowledgement, guidance, Stop, expiry and owner loss; controlled running and queued work; final
checkpoint write failure and lost acknowledgements; and explicit resume after restart. It compares
native admission/debit counts, exact original and accepted identities, claims, failures, user working
and index bytes before and after. It includes both a fully evidenced positive reconciliation and an
otherwise comparable unverified-child, incomplete-effect or missing-evidence rejection, as well as
another unresolved blocker that must remain fenced. A test with only an artificially populated
database record does not by itself prove physical process termination or filesystem safety.
