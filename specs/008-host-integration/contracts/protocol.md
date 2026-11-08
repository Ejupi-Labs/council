# MCP Stdio Protocol Contract

**Status: design contract for feature 008; its specified offline implementation
is accepted with the limits in the acceptance record.** This contract describes the protocol boundary for `council mcp`; the
companion catalog contract must enumerate each concrete tool, owner operation,
complete argument schema and result mapping. It introduces no separate engine,
direct database access, public MCP listener, remote tenancy or new authority.

The upstream references are the official MCP specifications for [stdio
transport](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports),
[2025-06-18 lifecycle](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle),
[2025-11-25 lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle),
[tools](https://modelcontextprotocol.io/specification/2025-11-25/server/tools) and
[cancellation](https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/cancellation).
Council's bounded validation and private-owner rules below are additional product
requirements; they do not grant an operation that the owner does not support.

## Framing and lifecycle

The host launches the native executable with arguments `mcp`, without a shell.
The server accepts one UTF-8 JSON-RPC 2.0 message per newline-delimited frame on
stdin and emits one complete JSON-RPC message per newline on stdout. Embedded
literal newlines inside a frame are invalid; JSON string escapes remain valid.
Concurrent handlers must serialize complete output frames so bytes never
interleave. Human-readable messages, startup URLs, stack traces, subprocess
output and diagnostics never go to protocol stdout. Stderr may contain bounded,
sanitized diagnostics without private owner material or vendor credentials.

The inbound frame limit is **64 MiB (67,108,864 bytes)** including its newline
delimiter and JSON-RPC envelope. Enforce it before unbounded allocation or
decoding. Existing smaller operation limits remain in force after extraction,
including the owner's 32 MiB general body, 4 MiB text, 2 MiB graph declaration,
16 KiB history request and 65,536-byte evidence page where applicable. Escaping
or whitespace can make an otherwise semantically valid object exceed the frame
limit; report the boundary, never truncate it or submit a partial body. A frame
without its terminating newline at EOF is incomplete and must not dispatch. For
oversized or invalid frames, either send an appropriate protocol error when a
trustworthy correlation ID can be identified or close the connection safely;
never fabricate an ID, emit a partial reply, or execute a truncated request.

Initialization is the first interaction: the client sends `initialize`, the
server returns its negotiated version, implementation information and only
implemented capabilities, then the client sends `notifications/initialized`.
Initially the server qualifies **2025-06-18** and **2025-11-25** only. If the
requested version is supported, reply with that version. Otherwise the official
2025 lifecycle requires the server to return a supported alternative, normally
the latest supported version; a client unable to use that reply should
disconnect. Do not silently claim support for other versions or interpret an
incompatible client as having consented to a different operation contract.
Normal tool requests cannot dispatch before initialization. Protocol `ping` is
handled according to the negotiated version. The server advertises tools only;
do not claim resources, prompts, sampling, task-augmented calls or notifications
that are not implemented and verified. EOF/shutdown closes this connection, not
the independent managed owner.

## Raw schema and validation boundary

Validate complete raw frame bytes before any parser can collapse duplicate
members. Reject invalid UTF-8, duplicate keys at every nesting depth, invalid
JSON/JSON-RPC shapes and unsupported methods. Preserve JSON-RPC correlation
IDs exactly according to the negotiated protocol; do not coerce them into
floating-point values or durable operation IDs. Notifications have no response
ID and must not receive a fabricated response.

For supported protocol envelopes, accept only the members defined for the
negotiated method and its documented extensions. Protocol-defined extensible
containers, including permitted capability/metadata extension members, are
handled according to that version rather than treating them as Council authority.
No unknown envelope member may select an unadvertised operation. Unknown,
duplicate, null where disallowed, ill-typed and out-of-range Council tool
arguments are rejected before any owner call; arbitrary nested application input
is not excused merely because the protocol supports metadata extensions.

`tools/list` returns a finite versioned catalog. Every listed tool has an explicit
object `inputSchema` with `additionalProperties: false`, required fields,
canonical ID/revision types and applicable object/array/string limits. Nested
objects are likewise closed except for specifically defined extension points.
No schema default, implicit owner read or SDK coercion may add missing authority
or modify the original raw arguments before Council validation. The companion
catalog identifies the authoritative owner DTOs and validators; a versioned
complete closed machine-readable schema per tool must be generated, reviewed and
checked against those sources before its dispatcher is implemented or enabled. Bare
struct inference and an untyped arbitrary-JSON or HTTP proxy are not substitutes. Validate output against its advertised schema when one
is advertised. Any SDK wrapper must be tested at the raw stdio boundary, not
only after JSON or schema decoding.

## Tool call, result and error mapping

The following illustrates the shape, not a commitment to a specific catalog
name. The tool name and every argument member are fixed by the companion catalog;
`requestId` shown here is the durable Council operation identity, not JSON-RPC
`id`. The exact owner mutation body is nested under `input`, with path
identifiers at the outer level when the operation requires them:

```json
{"jsonrpc":"2.0","id":17,"method":"tools/call","params":{"name":"<catalog mutation tool>","arguments":{"input":{"requestId":"0123456789abcdef0123456789abcdef","...":"<exact declared input>"}}}}
```

Each successful or business-failed owner call returns a protocol result with an
object `structuredContent` with exact shape `{"status":<owner status integer>,
"envelope":<safe Council envelope>}`. The envelope preserves its `apiVersion`,
`requestId`, `data` or `error` and any `receipt`, including meaningful nullable
fields, without inventing an owner receipt. Include a JSON text content block containing the same logical object
for clients that cannot consume structured results. The enclosing JSON-RPC `id`
correlates this call only. A normal tool execution or validation failure has
`isError: true` and retains the safe Council error code, request ID and current
revision/graph context where the existing owner provides them. A successful tool
result preserves the owner's receipt when one exists; a read has no invented
receipt. Transport success, admission, wait completion or a committed receipt
does not imply task acceptance, process cleanup, delivery or completed work.

Unknown tools, malformed JSON-RPC, invalid protocol request shapes and unsupported
methods are protocol-level errors. Input validation and owner/business failures
are tool results, not claims that an effect succeeded. Neither type may include
raw exception text, filesystem internals beyond the operation's existing public
result, private tokens, provider authentication or arbitrary unrelated data.
If the owner acknowledgement, result serialization or transport delivery is
uncertain, return/report only a safe uncertainty tied to the exact durable
request where possible. Never turn an incomplete result into a success.

## Durable operation identity and owner binding

Every mutation requires a Council `requestId` and all exact owner-required
operation, target, input and captured authority fields. It is independent of
the JSON-RPC correlation identity and remains a canonical 32-character lowercase
hex string. Reconnects and other interfaces may use a different transport ID
while supplying the same durable identity and exact intent. Before any effect,
the complete input must be available for exact retry: either the caller supplies
and retains it or a proven protected durable intent mechanism does. Do not
silently generate a replacement request ID, resample current graph/node/budget
authority, substitute a different operation or broaden permissions on retry.

Forward an accepted intent once to the existing authenticated managed owner.
The owner's receipt/fingerprint transaction, not the host process, decides
idempotency: same ID and input replay the original receipt/result or documented
current unresolved operation; a different operation, target, effect-changing input or authority
with that ID returns `request_conflict` without a new effect. A lost response or
cancelled in-flight call is not evidence of rollback. In-flight owner operations
and their records survive the client connection; if exact input or a trustworthy
owner acknowledgement cannot be retained, surface uncertainty rather than
guessing, silently retrying or starting another effect.

The host uses the established owner discovery and lock rules, authenticated
loopback and fixed operation routes. It never reads vendor credentials or emits
the private descriptor/token. Attach to a compatible owner or start the same
managed owner when none exists; a live incompatible locked owner remains intact
and is not silently replaced. Verify owner instance and relevant capability
versions before operations and recheck owner authority at the operation boundary.
For a recognized older owner with its independently supported maintenance
contract, the explicit owner-bound Stop operation remains available; no newer
task, graph, recovery or history capability is inferred from Stop support.
Version/owner changes must not refresh a previous mutation's captured authority.

## Evidence and observer cancellation

Artifact-byte tools preserve the owner's byte-page shape: exact artifact ID,
canonical decimal `offset`, optional captured `through`, limit 1–65,536,
`nextOffset`, `through`, `bytesBase64`, `eof`, `streamClosed`, `completeness`
and optional `sha256`. The next read uses the returned offset and unchanged
boundary. `eof` means the selected boundary was reached, not that a growing
artifact is closed or successful; a complete-artifact digest is not represented
as a digest of an earlier shorter prefix. Binary and multibyte bytes are never
decoded as character pages. An invalid range, missing artifact or failed retained
identity/integrity verification is an explicit error and may retain an existing
owner fence; no model is called to regenerate missing evidence.

`notifications/cancelled` identifies the in-flight JSON-RPC request in the same
session. It cancels that observation/response when possible; under the 2025 MCP
cancellation guidance a receiver should not send a response for a cancelled
request. Unknown/already-finished cancellation may be ignored as allowed by the
protocol. An observer's timeout, cancellation or EOF never sends a task Stop,
resumes a checkpoint or shuts down the owner. A cancelled mutation's durable
result may still commit; exact receipt replay or explicit safe inspection remains
available after reconnect. Only an explicit authority-checked Stop/pause/resume
or reconciliation operation can alter the corresponding Council lifecycle.

## Required real-protocol checks

Use a real protocol client and controlled fake owner/agents; no vendor access.
The implementation and acceptance must verify at least:

1. Both qualified versions, supported-version fallback, initialized state,
   tools discovery and unknown method/tool behavior; stale/incompatible owner
   refusal and recognized older-owner Stop without accidental new capabilities.
2. UTF-8 newline framing, exactly bounded frame and one-byte overflow, truncated
   EOF, malformed JSON, duplicate outer/nested members, unknown arguments,
   invalid nulls and multibyte/escaped limits. Confirm no owner effect and no
   non-protocol stdout on startup, failure, concurrency and shutdown.
3. Create a fake task through each UI, CLI and host entrypoint; inspect/control
   it through the others with the same IDs, receipts, revisions and outcomes.
   Two host connections share one owner, admission and capacity/fences.
4. Commit then lose an acknowledgement, reconnect and replay the exact request
   with a different JSON-RPC ID; reject effect-changing body/scope under the same durable
   ID, including concurrent duplicate submissions and stale authority; verify that
   owner-equivalent omitted/default values replay the same existing receipt.
5. Reconstruct empty, long, binary, multibyte and growing retained evidence across
   bounded pages; verify captured boundary, exact bytes and available full-output
   integrity. Reject invalid cursors/ranges and preserve evidence failures.
6. Cancel/time out a wait and disconnect while owned work continues; explicitly
   Stop separate active/queued work, preserve cancellation and partial evidence,
   and verify cleanup uncertainty, independent incidents and restart do not
   trigger automatic replay. Cover a mutation interrupted around commit.
7. Exercise the complete catalog's accepted workspace, routing, workflow, graph,
   checkpoint/recovery and incomplete-history operations with current and stale
   authorities. Verify the full original text and project scope, no extra model
   calls for observation, no credential access, and no release/user-data leak.

No test described here has passed merely because it is specified. Supported host
versions, actual operating systems, test skips and live-provider qualification
must be stated separately in the feature's eventual acceptance evidence.

Initialization and tool discovery must not require a current-owner capability check. Owner attachment and version qualification are performed per tool; a recognized older maintenance owner may receive only its supported owner Stop while current execution/history calls refuse without replacement. An unknown locked owner is never stopped or displaced.

The selected SDK version alone does not enforce the `notifications/initialized`
gate before tool dispatch. A per-session ordered validating transport or equivalent
middleware must reject tool calls before a valid initialized transition, including
pipelined/concurrent input, with zero owner effects. It must not infer completion from
merely receiving an `initialize` request; raw stdio tests exercise invalid order,
duplicate notifications and supported negotiation. Source: [official SDK server](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/server.go).

Existing owner fingerprints are semantic for selected operations: documented omitted
versus default values may be equivalent and replay the same receipt. The MCP adapter
must not claim every raw-body change conflicts or impose a local-only fingerprint that
differs across interfaces. Forward the full original intent and captured authority;
any effect-changing difference must conflict under the existing owner contract. Test
both omitted/default equivalence and changed captured authority through MCP and CLI.
