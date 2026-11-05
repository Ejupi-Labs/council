# Junie ACP ballot compatibility reproduction

This is a prepared provider bug report, not a claim that the remaining defect is fixed. No report has been sent externally.

## Observed environment

- Windows, Council native backend with direct ACP v1.
- Junie CLI 26.7.20, build 2383.10.
- Junie's advertised account defaults: `grok-4.5`, high effort. No model substitution or brave-mode change.
- Council-owned fixture project containing only a small `sample.ts` source file and diagnostic state. No vendor credentials or application sources are required.

Junie's [ACP integration](https://junie.jetbrains.com/docs/junie-cli-acp.html) and [CLI reference](https://junie.jetbrains.com/docs/parameters.html) document the public entry points. The installed session advertises model, effort and brave-mode configuration, with no read-only mode or final-answer selector. The documented print-mode output format does not establish an ACP final-message boundary.

## Reproduction

This reproduction makes a real Junie call. Keep it in a disposable fixture and preserve the owning process until it finishes. Use a supported Council Node/SQLite runtime and the user's existing Junie sign-in. Do not change account settings or grant tool approvals.

1. Create an isolated project with `sample.ts` containing:

   ```typescript
   export function add(a: number, b: number): number { return a + b; }
   ```

2. Configure a Council room with one Junie voting member, one read-only task, one discussion round and `autoExecute: false`.
3. Ask the member to inspect `sample.ts` before deciding whether `add(2, 3)` returns 5. Restrict inspection to project reads.
4. Require a single whole-turn ballot: `vote`, nonempty `rationale` and task-to-agent `assignments`. Forbid acknowledgements, progress or commentary before, between and after tool calls; place all evidence and decision reasons in `rationale`.
5. Start discussion and retain the raw ACP transcript privately. Inspect metadata and the room result without extracting JSON from the assistant response.

The exact Council-level reproduction and receipts from 5 November 2026 are retained locally under `.council/junie-live/whole-turn-room.*`. They are excluded from Git and release packages. The room ID is `ae04fc95-aac6-4c39-857a-f0f86f23a07b`; the discussion workflow is `1e227f0b-8502-481b-b3d0-646bace27b27`. Those IDs refer to local diagnostic state, not a public service.

## Actual sequence

| Event | Observed content or metadata |
| --- | --- |
| Assistant update | 78-character first-person inspection notice |
| Tool updates | One successful in-project read |
| Assistant update | 555-character valid JSON ballot |
| Prompt result | `stopReason: end_turn` |

Both assistant updates use `session/update` with `update.sessionUpdate: agent_message_chunk` and `content: {type: text, text: ...}`. Neither has a message ID, annotation, final/channel flag or distinguishing metadata at event, params, update or content level. The prompt result supplies no final text. The other event discriminator is shared with all session updates and cannot identify the ballot.

Concatenating the text as ACP assistant output produces prose followed by JSON. Council correctly reports `needs_attention`, retains the original response and executes no task. A simpler source-inspection/JSON prompt succeeded in a separate probe, so this is not a claim that every Junie response fails.

## Expected provider behavior

Either honor the whole-turn exact-output instruction without assistant commentary, or expose an authoritative boundary that distinguishes commentary from the final answer. A client cannot safely infer that arbitrary assistant text is disposable service output. Read permissions, full evidence retention and strict ballot validation must remain intact.

Council must not silently remove a prefix, select a JSON substring, treat an invalid ballot as approval or swap models. Switching to print mode also requires an established equivalent permission policy before it can replace the current ACP path.
