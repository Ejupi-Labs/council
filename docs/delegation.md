# Delegated project management

Council supports two management modes: a human can operate the task board directly, or a connected host agent can act as the human's executive delegate. The delegate sets up rooms, follows the board's work and brings decisions, blockers and evidence back to the human. Room leads, workers and advisors retain their existing execution roles.

Open **Human delegate** in the dashboard to save the coordination mandate, add the selected room to the monitored IDs, and read the executive inbox. **Prepare agent handoff** produces a prompt to paste into the host agent connected to Council. It uses the saved mandate, so save any edits before preparing it. The button's live open-question badge updates when a decision or blocker is recorded or answered, without requiring a room change. Complete report bodies and mandate text remain in their explicit views; update notifications carry only compact counts and revision metadata.

The delegate runs in the host conversation connected to Council's MCP server. Selecting agent mode records a durable management mandate; it does not launch a vendor process, keep a model running after its host disconnects or deliver a push message to another vendor chat. The host chooses its own model and effort. Council's configured worker aliases and their model settings remain independent from the delegate label.

## CEO and board workflow

1. Define the outcome and the executive mandate in the host conversation. State which decisions the delegate can make and which disagreements or changes require the human's input. Existing task authorization applies; setup does not require another confirmation merely because management is delegated.
2. Have the host inspect `council_delegate_status`, then create the relevant rooms through the existing room tools. Select appropriate agents, exact models, effort, decision weights and advisors. Tasks retain explicit read/write permissions, dependencies and verification criteria.
3. Configure agent management with a delegate label, mandate and the explicit IDs of the rooms to follow. Configuration uses the revision returned by status so a stale host cannot silently overwrite a newer handover. Tracking a room does not start it.
4. Start discussion. Council collects the members' ballots and executes approved tasks automatically when that room enables automatic execution. The server advances the task graph while the delegate waits through `council_delegate_watch`.
5. When a room needs attention or finishes, the delegate inspects the relevant evidence and submits a durable report. Use an update for useful information, a decision report for a question requiring judgment, a blocked report for work that cannot proceed, and a completion report for the actual outcome. Report failed or cancelled work explicitly.
6. The human reads the report and records a response against its current revision. Resolving a report does not rewrite the room brief, grant a task permission, approve a ballot or dispatch work. The delegate applies any authorized guidance explicitly through the room tools and starts a fresh discussion when instructions changed.
7. Switch back to human mode whenever direct management is preferable. The handover changes who coordinates the work; existing workflows continue under their admitted instructions. Pause drains admitted work before stopping further room dispatch. Stop cancels it. Neither operation merges retained worktrees.

For example, a human can ask: "Coordinate the release review using Claude and Junie. Track the review and verification rooms. Choose routine assignments yourself; bring unresolved architectural disagreements and any proposed expansion of write scope back to me." The delegate can set up and monitor those rooms, then report a concrete decision with its recommendation and links to retained evidence. It does not need to narrate every intermediate step.

## MCP tools

| Tool | Purpose |
| --- | --- |
| `council_delegate_status` | Read the current management mode, mandate, configuration revision and tracked room IDs without invoking a model. |
| `council_delegate_configure` | Save management mode, delegate label, mandate and explicit tracked room IDs using the current settings revision. |
| `council_delegate_report` | Record a durable update, decision, blocker or completion report with the relevant observed room state. |
| `council_delegate_reports` | Read complete reports and their human resolutions using the returned page cursor. |
| `council_delegate_changes` | Read the latest state of reports changed after an observed report revision, including replies to earlier questions. |
| `council_delegate_resolve` | Record an explicit author's response against the report's current revision. |
| `council_delegate_watch` | Wait in the server for actionable changes, using the returned cursor for the next wait. |

Read the tool's advertised input schema for exact request fields. Delegate tools manage coordination and reporting; room tools continue to create tasks, add project guidance and control execution.

Configuration supplies `mode` (`human` or `agent`), `delegate`, `mandate`, `roomIds` and `expectedRevision`. Agent mode requires a nonempty delegate label and mandate. Reports supply `author`, `kind`, `body` and optional `roomIds`. When reporting on evidence actually reviewed, also supply `expectedRoomRevisions`, keyed by every referenced room ID; Council rejects a stale reviewed revision. The stored `observedRooms` records the state captured at submission, without claiming that the author reviewed it.

When a report submission may be retried after a lost response, supply an `idempotencyKey` of 1–128 ASCII letters, digits, dots, underscores, colons or hyphens. Reuse the same key and exact author label for that operation. An identical retry returns the original report in its latest retained state, including a later human response, without creating another question or advancing report revisions. Changing the body, kind, associated rooms or reviewed-revision preconditions under that key reports a conflict. Room association order and reviewed-map key order do not change the request; an omitted reviewed map remains distinct from an explicit one. Retrying the original operation does not review a newer room state. Keys are local retry identities, not authentication or permission grants. Without a key, each submission creates a separate report.

Only `decision` and `blocked` reports are open questions. Resolve one with its `id`, explicit `author`, `body` and current `expectedRevision`. If its response is lost, retrying the identical author, body and original revision returns that retained answer without changing it or notifying the delegate again. A different response or author still conflicts, and submitting another answer against the resolved revision remains invalid. Updates and completion reports remain historical records and cannot be resolved as questions. A report page is ordered by creation sequence; resolutions update existing reports without appending a new sequence. Report and change pages target one MiB of complete UTF-8 JSON, so follow `nextCursor` even when fewer than the requested number of entries are returned. One larger report or response is returned intact on its own; pagination never truncates evidence.

After a watch reports a changed `reportRevision`, call `council_delegate_changes` with `after` set to the previous cursor's `reportRevision`. It returns complete `{ revision, report }` entries and `nextCursor` for further pages, so a human reply on an old question remains discoverable without rereading the inbox. Use `nextCursor` to continue until the end of the batch. This feed returns the latest retained state of each changed report, rather than a journal of every intermediate mutation. Keep creation-order `council_delegate_reports` pages for inbox browsing and `openOnly: true` for unresolved questions; continuing an old creation cursor alone cannot discover a reply on an earlier report.

## Waiting and evidence

Start `council_delegate_watch` without a cursor to read the current tracked-room snapshot. Reuse the returned `cursor` for the next wait rather than reconstructing it. It records the observed settings, reporting and tracked-room revisions and belongs to that workspace's delegation store. Configuration changes, human guidance, reports and actionable room outcomes can wake the waiting host. Ordinary active progress does not repeatedly wake a large model. The server performs local observation without model calls; while the tool is pending, the host should yield control.

A timeout ends the wait, not the room's work. It is not a failed task, a successful task or a human decision. Host MCP deadlines can end a request earlier. If a host cannot hold a suitable wait, the human can follow the local UI while Council advances existing room work autonomously. A model-driven polling loop defeats the purpose of delegation.

Routine monitoring uses room summaries and report metadata. Read full room history and workflow results when their evidence is needed. Report text remains separate from project-manager instructions, so publishing a summary does not invalidate a room's approval. A later room change can make a report's recorded observation historical; it must not be presented as fresh evidence for a newer state.

## Authority and persistence

The management record and reports stay under the workspace's ignored `.council/` directory and survive host restarts. Explicit tracked IDs determine the monitored rooms; unrelated rooms are not implicitly added to a mandate. Selecting agent mode does not change a room's configured backend or the models, effort and skills already admitted to its workflows.

Council is a local tool for trusted processes under one operating-system user. Delegate and report-author labels are coordination metadata, not authentication. The mandate guides the connected agent; it is not a credential or an operating-system access boundary. Task permissions, call budgets, isolated worktrees and explicit room controls continue to govern execution. Peer output, votes and reports never create new authorization.

The [human-delegate skill](../skills/council-human-delegate/SKILL.md) supplies this host workflow. It is separate from the four worker collaboration skills and cannot be assigned through a worker's `skills` array.
