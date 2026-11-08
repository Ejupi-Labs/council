# Data Model: MCP Host Integration

**Feature**: `specs/008-host-integration/`
**Status**: Design input; no independent host authority or persistence is implied.

The host presents existing Council records through a protocol session. The
managed owner remains the sole authority for admission, durable receipts,
ownership, revisions, evidence, controls and recovery. A connection, catalog
entry, transport request or cached response never itself authorizes an effect.
Exact protocol schemas, version negotiation and transport limits belong to the
operation contract and plan; this model does not claim those are already chosen.

## Host connection and owner observation

| Concept | Identity and content | Lifecycle and authority |
| --- | --- | --- |
| Host connection | One local protocol session, negotiated protocol/capability set and connection-local request correlations. | Created when a host connects; ended by disconnect or protocol shutdown. It does not own the Council task, cancel the owner or establish a new scheduler. A later connection is a new transport session, not a new durable task. |
| Owner observation | The current managed owner instance identity, API/application and capability-contract versions, admission/fence and shared-capacity observations, obtained through existing owner discovery and compatibility checks. | A bounded, point-in-time observation. The actual lock, current owner epoch and owner-side transaction remain authoritative. A cached descriptor, PID, old observation or protocol connection is not proof that a later mutation is safe. |
| Private owner connection material | Controller-private local endpoint and authentication material held only as needed to communicate with the owner. | Never placed in the host tool catalog, public result, errors, evidence, setup examples or logs. It is not a vendor credential and does not grant a host authority beyond the existing owner interface. |

An incompatible or unavailable owner is an explicit failure, not permission to
open an independent store, replace a live owner, silently migrate old state or
retry a mutation under newly guessed authority. Owner changes and capability
observations are revalidated at the actual operation boundary. A prior durable
receipt may still be replayable after a clean or fenced owner change when the
existing operation permits it; a fresh observation never rewrites the original
request or clears a fence.

## Tool catalog

Each advertised tool describes one supported operation or a precisely bounded
group of operations: stable external name and description, strict input shape,
result shape, read/mutation classification, relevant limits and current required
owner capability. The catalog is derived from the implemented host contract and
the compatible owner's supported operation families, not from model claims or a
host-maintained plan. No advertised capability implies a provider feature or
telemetry that Council cannot establish. Unsupported methods and arguments receive
an explicit error instead of being interpreted as another operation.

| Existing operation family | Authoritative records and controls represented |
| --- | --- |
| Workspace | Project open/list/detail; saved policy; agent discovery; draft create/update/list/detail. |
| Tasks and evidence | Task start/list/detail/version/guidance and later attempts; attempt status, wait, events, artifacts and Stop; explicit response acceptance under its existing authority; complete artifact-byte pages. |
| Routing and capacity | Policy, context/profile, declared account/link, capacity reports, task routing decisions, budget/ledger, usage, explicit budget grant and routing adoption. |
| Verified workflow | Preferences, adoption, advance, check, and linked step/review/finding/check/candidate/change/delivery observations. |
| Task graph | Declaration and definitions, graph/node/control/budget authority, graph start/advance, node or group Stop, guidance and grant; inputs, members, dispatches and budget ledger. |
| Recovery | Task/member/graph pause, durable checkpoint, explicit resume, incidents and owner-mediated reconciliation. |
| Metadata history | Capture, bounded summary/pages, exact export preview and operation, explicit local export and no-publication reconciliation. Its fixed incomplete-history notice and exclusions remain part of the result. |
| Owner maintenance | Only explicitly invoked, already supported owner controls; observing, cancelling a protocol request or disconnecting does not invoke service Stop. |

Within each family the final contract must enumerate the concrete accepted
operations and their exact existing inputs/outputs, including omissions and
unsupported cases; naming a family alone is not proof of complete parity.
Tools have no authority to inspect or copy vendor credentials, automatically
publish externally, broaden a task policy or convert the metadata-history slice
into backup/restore.

## Request identities and captured intent

| Concept | Scope and validation | Invariants |
| --- | --- | --- |
| Transport correlation identity | The protocol request identifier, unique within its active connection as required by the negotiated protocol. | Used to match a response or cancellation to that call. It can change on reconnect and is never substituted for a durable Council request ID. |
| Durable request identity | The existing Council operation `requestId`: a 32-character lowercase hexadecimal identity, shared with UI and CLI. | Each effect-capable operation carries or durably retains it before forwarding. It identifies the same request across sessions and interfaces, including an uncertain reply. |
| Captured intent | Exact operation and target plus validated input, scope and applicable task/graph/node, policy, checkpoint, budget or destination revisions supplied/captured for the request. | Freeze the same meaning for an exact retry. Never refresh stale revisions, infer extra permissions or turn a different target, effect-changing body or operation into a replay. Document and retain the owner’s existing equivalent omitted/default spellings. The owner uses its existing operation fingerprint and receipt transaction as the authoritative conflict check. |
| Pending acknowledgement | A submitted intent for which no trustworthy owner acknowledgement is available to the host. | It is neither proof of rollback nor proof of commit. Preserve enough exact request information to retry or inspect without launching another effect; if durable intent retention or its integrity cannot be established, fail closed and report uncertainty. Do not retain private authentication material in this record. |

The transport must not discard a caller-supplied durable identity, create a new
one on a retry, or make a previously unavailable outcome appear successful.
Where the client must supply or retain an identity and exact input, document that
contract before submission. If the adapter generates or captures them, they must
be recoverable after an interrupted acknowledgement before the effect is sent;
implementation-specific retention must preserve existing private-state and
receipt semantics. An error before admission is not a committed receipt. Two
hosts submitting owner-equivalent requests obtain one authoritative result; an
effect-changing input under the same identity yields the existing request conflict
and no new effect. Existing omitted/default equivalents replay the original receipt. Existing owner-side receipts and per-operation replay checks remain the
source of truth.

## Result, receipt and state observations

The existing `domain.Envelope` contains `apiVersion`, `requestId`, either
`data` or `error`, and a `receipt` where the operation committed one. A Council
receipt contains the durable request ID, operation and committed timestamp.
The host preserves that identity, meaning and original receipt when wrapping a
result for the protocol; it does not replace it with its own transport ID.
Existing error codes, relevant conflict revision/graph context and actionable
messages remain distinguishable without leaking raw private diagnostics.

A receipt proves the recorded operation, not that a queued task completed, a
partial output succeeded, a checkpoint is resumed, a process is quiescent or a
change was accepted. An exact replay ordinarily returns the saved receipt and
recorded response. Some existing operations, including metadata-history export,
retain that original receipt while presenting a later truthful durable operation
state; the host must preserve both distinctions rather than freezing a stale
success view or replaying an effect.

The host carries the existing task/attempt states and linked outcomes,
checkpoints, incidents, claims, capacity, routing and budget records without
inventing new success, quota, model or price values. Observation timestamps and
remaining time can change on repeated reads; they are not immutable receipt
fields. Unknown cleanup, unrecorded fence causes and unsupported proof remain
unknown and blocked. Explicit reconciliation records its own request and
decision and can release only the exact proven-safe authority under the owner's
existing global-blocker checks.

## Evidence page and byte boundary

An artifact is identified by its existing stable artifact and attempt identities
and retained metadata; paths or host strings alone are not evidence authority.
The existing byte operation accepts `artifactId`, a nonnegative canonical
decimal `offset`, optional captured `through` boundary, and a byte limit from
1 through 65,536. Omitted offset begins at zero; omitted boundary captures the
currently retained end. Counts and offsets are canonical decimal strings within
the owner's supported 64-bit range, not lossy floating-point values.

An existing `evidence.Chunk` carries `artifactId`, `offset`, `nextOffset`,
`through`, `bytesBase64`, `eof`, `streamClosed`, `completeness` and optional
`sha256`. Its bytes are base64-encoded exact bytes, not UTF-8 character slices.
For a successful page, `0 <= offset <= nextOffset <= through`, decoded byte
length equals `nextOffset - offset`, and `eof` means `nextOffset == through`.
An empty artifact or empty retained prefix can have `offset == through == 0`.
The next page uses the returned `nextOffset` and the same `through` with the
same artifact identity. Later growth is a separate explicit read boundary.

`eof` concerns the captured prefix, not necessarily a closed or successful
stream. `streamClosed`, `completeness` and optional hash retain their owner
meanings; a hash for the complete closed artifact must not be claimed as a hash
of an earlier shorter captured prefix. Identity or integrity failure returns the
owner's evidence-unavailable/fence outcome; the adapter must not synthesize
missing bytes, silently shorten the boundary or rerun work to reconstruct them.
Response framing must accommodate base64 expansion and protocol/envelope
overhead without clipping a legal page. Other record lists retain their existing
bounded limits and opaque cursor semantics.

## Cancellation and lifecycle

1. **Observer wait/read**: protocol cancellation, timeout or connection loss
   ends that observation. It must not issue task Stop, owner shutdown, checkpoint
   resume or another control. A later read observes the current owner state.
2. **Effect-capable call**: cancellation/disconnect may interrupt its response
   while an owner transaction or effect is already underway. It must preserve
   the exact durable identity/input and report uncertainty where acknowledgement
   is unknown; it cannot assert rollback or silently resubmit a changed intent.
3. **Explicit task/graph control**: an authorized Stop, pause, resume or
   reconciliation is a separate durable operation. Its accepted response and
   eventual terminal/cleanup state remain distinct. Existing owner sequencing,
   retained evidence, primary failure and other independent fences govern it.
4. **Owner loss/restart**: the owner epoch and existing recovery contracts decide
   what is known and what remains fenced. A host session disappearing or a new
   session appearing supplies no cleanup proof and never automatically replays
   an unfinished effect.

## Security and scope boundaries

- Protocol stdout carries only protocol messages; diagnostics use the separate
  diagnostic channel and must not include access material or arbitrary task
  content as a side effect of failure reporting.
- Task/evidence content is disclosed only when requested through the existing
  local capability and selected scope; observation and setup do not invoke a
  model or create external publication authority.
- Original user text and exact evidence remain verbatim. UI/CLI/host labels and
  outcomes must not substitute generated summaries for retained originals.
- Existing incomplete history remains private observational metadata. Full
  evidence export, backup/restore, cleanup and live provider qualification are
  separate required work and are not new entities authorized by this model.
