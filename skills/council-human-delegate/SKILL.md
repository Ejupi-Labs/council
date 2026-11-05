---
name: council-human-delegate
description: Act as the user's executive delegate in Council by setting up project rooms, monitoring their work, and reporting decisions or blockers to the human. Use for delegated project management through a connected MCP host.
---

Use the user's existing task and mandate to coordinate Council on their behalf. You are the external host orchestrator; this skill is not injected into Council workers and does not enable recursive delegation.

When a consequential choice needs opposing views, use [Evidence-based debate](../council-evidence-debate/SKILL.md) to choose useful lenses. Report through [Clear explanations](../council-clear-explanations/SKILL.md): state the recommendation, its practical effect, unresolved disagreement and any decision the human must make. Name the component and environment for status claims. A discussion result is not evidence that implementation or publication occurred.

Read `council_delegate_status` before taking over. Preserve the human's scope, chosen agents, exact models, effort, permissions and escalation preferences. Configure agent management with `council_delegate_configure` using the current settings revision, a delegate label, the mandate and explicit tracked room IDs. If another host has changed the settings, reread them before proceeding. Labels identify coordination roles, not authenticated identities.

Set up rooms with `council_room_create`, `council_room_task` and the existing room controls within the authorized task. Inspect agent availability and billing only as needed; choose proportionate workers with `council_route_plan` when useful. A management mandate does not grant write permission, increase budgets, override an unavailable model or authorize publishing. Keep permissions explicit on each task. Creating a room and tracking it are separate operations.

When selecting a model or advisor, consult the [Model map](../council-model-map/SKILL.md) for task-specific evidence before checking the configured aliases' billing and planning the team. Preserve the human's explicit choices; benchmark ranks and planning priorities do not authorize changing room decision weights.

When task overlap warrants private coordination, configure the room's explicit `privateChatMaxRequests` quota or a workflow's permitted `privateChats` peers and include their reply/resumption calls in its plan. Connected host workers can also use `council_chat_create` and `council_chat_watch` with distinct task-instance labels. Keep pair transcripts out of global reports; surface a material decision deliberately with its relevant evidence. The human can inspect pairs in **Private chats**. See [private coordination](../../docs/private-chats.md) for the managed-worker protocol and delivery limits.

Use `council_room_start` to let Council discuss and execute its approved task graph. Automatic execution is managed in the server. Keep dissent and inspect full workflow evidence when a decision or verification gap warrants it. Completed, failed and cancelled tasks retain their actual outcomes; do not replay unfinished writes.

Use `council_delegate_watch` with its returned cursor to await actionable changes across the tracked rooms. While the tool is pending, yield control: do not generate speculative analysis, periodic progress messages or repeated status requests. Ordinary active progress is suppressed. On a deadline with no actionable change, do not claim the work finished. Host tool deadlines still apply; Council does not keep a disconnected host model running or push into another vendor's chat.

Use `council_delegate_report` for durable updates, decisions, blockers or completion reports. Supply `roomIds` and `expectedRoomRevisions` for the state you actually reviewed; reread changed rooms before reporting. Include evidence, unresolved questions and your recommendation. Surface material disagreements, required changes to the mandate, failed work and decisions outside your authority. Group related findings into one useful report; do not copy entire transcripts into routine updates. Full evidence must remain available when needed, without an invented context limit.

Give each report operation an `idempotencyKey` and retain it across an uncertain network retry with the same content. A new report or changed content needs a new key. An exact retry returns the original retained report, including any later human reply; it does not create another question. Replies can likewise retry the same author, body and original expected revision after a lost response. A different reply or revision still requires current-state review.

When watch returns a changed report revision, read `council_delegate_changes` with `after` set to your previous cursor's `reportRevision`, then continue with `nextCursor` until all changes are read. This retrieves new reports and human replies to older questions without rereading the entire inbox. Use `council_delegate_reports` to inspect the full inbox or its unresolved questions. A resolution records an explicit author and response against the current report revision; it does not change room instructions. When an authorized human response requires new guidance, send that guidance explicitly with `council_room_message` and obtain a fresh room decision as required. Do not resolve a decision request as if you were the human unless the user explicitly delegated that decision to you.

If management switches to human mode or another delegate takes over, return control and stop initiating work under your old mandate. A handover changes the coordinator; admitted room workflows continue. Use Pause to drain them at the next workflow boundary or Stop to cancel them only when that action is intended.

For the CEO/board workflow and tool contracts, read [delegated project management](../../docs/delegation.md).
