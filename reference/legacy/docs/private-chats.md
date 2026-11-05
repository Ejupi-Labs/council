# Private one-to-one coordination

Council can keep a pair's coordination separate from the shared room conversation. For example, five workers can own independent tasks while the two touching a parser boundary discuss their overlap in a pair thread. Their raw messages do not automatically become context for the other three workers or a third-party judge.

Private delivery is a routing property within this trusted local application. Participant labels are coordination metadata, not authenticated identities or an operating-system confidentiality boundary. The local human can inspect the threads. A participant can deliberately share a decision or quote its own evidence in a public result; Council prevents automatic transcript broadcast, not intentional disclosure by a participant.

## Managed workers

During room creation, **Private peer exchanges per task (optional)** enables bounded private coordination with other room members. Leave it blank to disable the protocol. The API field is `privateChatMaxRequests`, a positive safe integer. This applies to task execution; discussion ballots remain independent and do not use private exchanges.

For a direct workflow, supply explicit allowed peers per worker:

```json
{
  "mode": "parallel",
  "task": "Complete the five assigned investigations and verify their conclusions.",
  "agents": ["alpha", "bravo", "charlie", "delta", "echo"],
  "permission": "read",
  "privateChats": {
    "alpha": { "peers": ["bravo"], "maxRequests": 2 },
    "bravo": { "peers": ["alpha"], "maxRequests": 1 }
  }
}
```

These aliases must already be enabled in the project configuration. Each configured request reserves two possible calls: a peer reply and a sender resumption. The example therefore reserves five ordinary calls plus six private-coordination calls, for an eleven-call maximum. No exchange is called merely because it was configured. `maxRequests` defaults to one when a direct workflow policy omits it; an omitted `privateChats` policy enables none. Existing workflow and room call budgets still apply.

An enabled worker can finish its turn with exactly this JSON envelope:

```json
{
  "council_private_chat_request": {
    "peer": "bravo",
    "message": "Which side owns validation at our shared parser boundary?",
    "context": "The complete evidence needed for this private coordination question."
  }
}
```

Council treats the envelope as a yield, never as a completed task. It releases the worker's execution slots, records the request in the pair thread, and gives the selected peer a separate read-only reply turn. The reply uses that peer's admitted exact model, effort and skills. Council records the answer in the pair thread, then resumes the sender with its original task permission and workspace. Both calls use the ordinary concurrency, cancellation and subscription-protection checks.

The peer reply is a dedicated coordination turn. It does not inject a message into the peer's already-running vendor process or interrupt its other work. Native and ACP workers can use the JSON protocol without receiving new MCP tools or shell access. Neither a private reply nor an advisor may recursively request another private exchange or advisor.

Managed task-instance labels are `run:<workflow-id>:<agent-alias>`. Pair threads from room tasks also retain their room ID, so **Selected room only** can find them. The thread's complete bodies stay in `.council/private-chats.sqlite`. Workflow steps and events retain metadata receipts, while raw pair requests and answers stay outside room history, shared peer lineage and public advisor prompts. A participating worker receives its pair evidence on a subsequent call; a continuation under a new workflow ID does not silently copy another run's private transcript.

Malformed envelopes, unpermitted peers, exhausted request quotas, blocked calls and failed replies leave the workflow incomplete. An unmistakable control response wrapped in a Markdown code block or cut off after a later request key also fails with its raw text kept out of public workflow evidence; it is never silently treated as completed task output. Cancelled or blocked replies retain their actual status even if a provider ignores cancellation and reports success. Complete evidence is preserved without a new context cap; an explicitly insufficient context budget fails before the next provider call rather than silently dropping messages.

## Connected host agents and the dashboard

Host agents that already have Council MCP tools can use pair threads directly. Give each independent task instance its own actor label, so two workers using the same provider alias do not accidentally share an identity.

| Tool | Purpose |
| --- | --- |
| `council_chat_create` | Open an idempotent pair thread with exactly two distinct participants and an optional room ID. |
| `council_chat_send` | Append a complete message as a participant, optionally with a workflow ID and retry key. |
| `council_chat_list_page` | Read bounded complete thread metadata for one exact actor, optionally filtered to a room. |
| `council_chat_actors_page` | Discover bounded complete participant-label pages without reading message bodies. |
| `council_chat_list`, `council_chat_actors` | Existing array APIs, retained for compatibility with earlier hosts. |
| `council_chat_history` | Read complete ascending message pages as a participant. |
| `council_chat_watch` | Wait in the server for new messages and return metadata, without a model call. |

For discovery, start `council_chat_list_page` with `actor` and optional `roomId`, or `council_chat_actors_page` with optional `roomId`. Both accept `limit` (default 50, maximum 100) and return `nextCursor`, which is null at the end. Pass a returned cursor to the same tool with the same exact actor and room scope. Cursors are bound to the local state directory, operation and filters; do not edit them or reuse them for another participant or room. Participant identity is literal and case sensitive, including surrounding spaces. Thread order follows creation time and ID, so new messages do not move a thread between pages. Participant labels follow SQLite binary order. Refresh from the first page to discover labels inserted before an earlier page position. These metadata APIs never load transcript bodies.

Discovery pages target one MiB of complete UTF-8 JSON in addition to the item limit. Labels and metadata are never shortened; a single larger item is returned intact on its own. A short page may still have a cursor. The same MCP response limit described for history below applies to oversized metadata. The authenticated HTTP equivalents are `GET /api/private-chats/page` and `GET /api/private-chats/actors/page`, with the same query fields and page shapes. The original array endpoints remain available for older clients.

`council_chat_send` accepts an optional `idempotencyKey`, scoped to the chat and sender. Retrying the same key and content returns the original message; changing the content under the same key reports a conflict. A send is a local append, not a provider invocation or a push into another vendor chat.

For history, start with `after: 0`, then use `nextCursor` with the same chat and actor until it is null. Pages target one MiB of complete UTF-8 message JSON as well as the requested message count; a single larger message is returned intact on its own. A short page can still have a next cursor. No message is truncated to fit a page. MCP retains its overall response-size limit; an exceptionally large single message can exceed that transport limit and produces an explicit error. Its complete stored text remains available through the authenticated dashboard history endpoint. To wait after reading all messages, pass the last observed sequence to `council_chat_watch`. Reuse its returned `lastSequence` only after explicitly reading the new history; a watch result contains no message bodies. Yield control while waiting instead of generating model-driven polls. A wait timeout or cancellation leaves the peer's work unchanged.

Open **Private chats** in the dashboard, select a participant/task-instance label, then load its thread metadata. Keep **Selected room only** enabled to inspect the chosen room, or turn it off for workspace threads. The dashboard loads the first thread and participant pages, then offers **Load more threads** and **Load more participants** explicitly; it never walks every page automatically. You can type an exact known label without loading every suggestion. Appending discovery pages preserves the selected pair and message draft. Failed pages retain the loaded choices and can be retried. The human can select a managed worker's discovered `run:...` label to inspect its pair history. Opening a pair loads complete message pages explicitly; later notifications show **Load new messages** instead of broadcasting the body into the shared room.

Messages and replies do not modify task permissions, vote weights, room guidance or approvals. If a private agreement changes the shared plan, communicate the authorized decision deliberately through the room's project-manager guidance and obtain a fresh decision where required. Write changes remain in their existing isolated worktrees and never merge automatically.
