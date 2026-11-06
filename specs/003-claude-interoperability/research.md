# Research: Claude Code interoperability

**Feature**: `003-claude-interoperability`
**Branch**: `refactor`
**Research date**: 2026-11-06
**Status**: Design decisions resolved; implementation and feature qualification remain separate gates.

This design adds explicit Claude Code authoring to the accepted Council lifecycle. Feature 002's
six native platform jobs and Linux AMD64 race job passed in
[run 42140818152](https://github.com/Ejupi-Labs/council/actions/runs/42140818152) at `f5edd96`.
That evidence qualifies the preceding implementation, not this adapter or live Claude use.

Research used Context7 first, then current official documentation and pinned public supporting
source. No installed vendor process, help/version/auth command, credential file, provider request,
test or build was used. No live qualification is claimed.

## Evidence basis and version assumptions

Context7 resolved Claude Code to `/websites/code_claude` and `/anthropics/claude-code`; three scoped
queries covered native launch flags, structured events and model/session behavior. The indexed
repository versions did not establish current installed compatibility. Direct primary reads on
2026-11-06 established the following references:

| Evidence | Snapshot and meaning |
| --- | --- |
| Current native CLI documentation | [CLI reference](https://code.claude.com/docs/en/cli-reference), [programmatic execution](https://code.claude.com/docs/en/headless), [permission modes](https://code.claude.com/docs/en/permission-modes). Mutable web documentation, read on the research date. |
| Current release metadata | [Claude Code v2.1.318](https://github.com/anthropics/claude-code/releases/tag/v2.1.318), published 2026-11-04. This is a documented release reference, not an observed local installation. |
| Public Claude Code repository | [`90380efca7f9d73e206fac453f841b6acd664f4d`](https://github.com/anthropics/claude-code/tree/90380efca7f9d73e206fac453f841b6acd664f4d), commit timestamp 2026-11-06T00:00:33Z. This source pin does not pin the separately hosted documentation or prove inspection of the closed native implementation. |
| Supporting TypeScript SDK source | [Changelog at `040a7f855da8261c795d975e116f40bb042334f7`](https://github.com/anthropics/claude-agent-sdk-typescript/blob/040a7f855da8261c795d975e116f40bb042334f7/CHANGELOG.md): SDK 0.3.318 records parity with CLI 2.1.318; 0.3.317 records an observation after the final result. |
| Supporting raw-message evidence | [Python parser at `3be907edb9c86269d30d1427a77937dd788d6419`](https://github.com/anthropics/claude-agent-sdk-python/blob/3be907edb9c86269d30d1427a77937dd788d6419/src/claude_agent_sdk/_internal/message_parser.py), and its [authored parser examples](https://github.com/anthropics/claude-agent-sdk-python/blob/3be907edb9c86269d30d1427a77937dd788d6419/tests/test_message_parser.py). Supporting wire-format evidence only; no SDK runtime is introduced. |

**Decision**: declare a minimum profile assumption of Claude Code **2.1.259**, with **2.1.318** as
the current documentation reference. The minimum follows the documented introduction of
`--permission-prompts none`; it is not a claim that every intervening release was tested. Keep
these assumptions separate from a sourced native version emitted by an authorized attempt, and
separate again from live-task qualification. Discovery can report an unknown version. Unknown
must never become verified compatibility, and an actual flag/version failure remains a specific
unsupported-interface or native failure rather than authorizing another launch path.

**Alternatives rejected**: running help/version/auth commands during discovery or refresh; treating
an installed filename or indexed documentation version as a compatibility probe; presenting SDK
documentation as completed CLI qualification.

## Decision 1: use one native print invocation per attempt

Use the resolved absolute native executable with explicit argv elements:

```text
--print
--input-format text
--output-format stream-json
--verbose
--no-session-persistence
--permission-mode bypassPermissions
--permission-prompts none
```

Append `--model` and the captured selector only when explicitly supplied. With no selector, preserve
native configured-model behavior. Supply one complete bounded text prompt through stdin, close
stdin, and retain the existing input-write proof. There is no intervening shell, interactive TTY,
second prompt, native background-session command or bidirectional SDK host.

Print mode supports stdin and JSONL output. Its documented stdin cap is 10 MB, above Council's
existing 256 KiB input bound. A failed stdin read can warn and continue using an argv prompt;
therefore native output cannot replace Council's complete-input proof. An invalid option may
fail on stderr without producing a result. [Native programmatic execution](https://code.claude.com/docs/en/headless).

**Rationale**: this is the smallest native profile supporting structured evidence and unattended
operation while reusing owned process preparation, launch gating, pipes and cleanup. Partial token
events are unnecessary; do not add `--include-partial-messages` to the initial profile. Coarse
assistant/tool observations are adequate progress and do not claim a completion percentage.

**Alternatives rejected**: an SDK/API runtime, interactive terminal automation, shell pipelines,
streaming-input control sessions, extra invocations for status/authentication, and automatic retry
or fallback. These introduce different execution or ownership contracts without a requirement.

## Decision 2: preserve vendor authentication and native instructions

Use neither `--bare` nor `--safe-mode`. Bare mode skips subscription OAuth/keychain login; safe
mode preserves authentication but suppresses CLAUDE.md and ordinary customizations. Both conflict
with silently retaining the requested native context. [Bare-mode behavior](https://code.claude.com/docs/en/headless#start-faster-with-bare-mode),
[safe-mode behavior](https://code.claude.com/docs/en/cli-reference).

Council forwards existing authentication context opaquely and does not read/copy credentials,
inject a key, perform login, edit vendor-global settings, install a harness or choose another
payment path. An externally configured API key can take precedence over subscription login; its
presence is not permission for Council to inspect its value or claim a particular subscription
account was used. [Environment precedence](https://code.claude.com/docs/en/env-vars).

Retain applicable instruction files represented by the existing captured-source support boundary
and launch in the prepared isolated workspace. Native user configuration and managed policy remain
vendor-owned. Required context outside the representable captured-source scope remains explicit;
do not claim it was reproduced or suppress it to make orchestration simpler. Ordinary print mode
can load native hooks/MCP without an interactive trust dialog, so it is not a universal filesystem
or network isolation mechanism.

Map the saved autonomous preference to `bypassPermissions`, with unanswered prompts denied by
`--permission-prompts none`. Native deny rules, managed restrictions and interaction-only tools can
still prevent an action; do not silently downgrade the mode or invent unrestricted access.
Linux/macOS root/sudo bypass refusal remains a native prerequisite failure.
[Permission contract](https://code.claude.com/docs/en/permission-modes).

**Alternatives rejected**: temporary credential homes, copied OAuth files, paid-key fallback,
global configuration changes, instruction suppression, or a per-tool approval bridge. These would
change account/instruction behavior or add an unrequested interaction path.

## Decision 3: share lifecycle proofs and isolate protocol parsing

Keep task/version/policy capture, source isolation, launch intent and final launch guard, Stop
ordering, process ownership, evidence drains, settlement, candidate retention, response acceptance
and no-replay recovery common to Codex and Claude. A small parser boundary can expose the existing
write/finalize/result/failure operations; two authored launch builders and parsers are sufficient.
A plugin registry or second execution engine is unnecessary.

Claude's protocol carries final text in a result event. Decode it into the normal private,
immutable `final_response` artifact after all common proof conditions pass. Do not fabricate a
last-message-file option: no such supported flag was established by this research. Codex's reserved
output target, retained-handle import and identity protections stay unchanged. Raw stdout/stderr
remain independent evidence. [Documented final result](https://code.claude.com/docs/en/agent-sdk/agent-loop#handle-the-result).

**Bound decision**: retain the existing **1 MiB maximum protocol line**, including JSON encoding and
overhead, plus the existing total stream/artifact bounds. A Claude final response inside one JSONL
record is consequently **less than 1 MiB**, and escaping reduces usable text further. A larger
generic artifact bound does not make 64 MiB Claude responses supported. Oversized, malformed,
truncated, invalid-Unicode or duplicate-member essential records produce an explicit evidence or
protocol failure; obtained bounded bytes remain evidence. Do not silently clip a success.

**Alternatives rejected**: treating arbitrary assistant prose/tool output as the final response,
merging all text deltas, weakening Codex output reservation, or increasing limits without an
explicit verified requirement.

## Decision 4: make terminal interpretation conservative

Require exactly one attributable main-conversation terminal result for this one-shot profile.
Multiple main results are unsupported even when labels agree; contradictory results or ambiguous
main-session identity also fail conservatively. Preserve their raw evidence. Do not choose the
first apparent success or the last convenient response. Startup errors without an init record
still remain failures rather than disappearing behind a missing-success diagnosis.

Successful authoring requires all of the following:

- `subtype` is `success` **and** `is_error` is false, with a valid final-result string and no
  authoritative earlier native/protocol/evidence failure. The string may be empty, but usable
  output still requires either a useful nonempty response or attributable useful authored output,
  following the accepted 002 candidate rules; empty response-only work cannot be accepted.
- Complete expected stdin write, verified root exit zero, EOF/finalized bounded protocol, required
  stream drains and durable evidence closure.
- Verified cleanup of the declared owned native scope, with accepted Stop/deadline/primary-failure
  ordering preserved.

The success subtype alone is insufficient: current documentation permits `success` with
`is_error: true` for refusal, unavailable deferred tooling and API failures.
[Result error semantics](https://code.claude.com/docs/en/agent-sdk/python#resultmessage).

Continue consuming trailing observations through bounded stream completion and root exit. Result
arrival does not prove process quiescence or stopped spending. The official loop documentation
allows trailing observations, and the pinned SDK changelog records `session_writes_lost` after the
last result. [Message lifecycle](https://code.claude.com/docs/en/agent-sdk/agent-loop#messages),
[pinned trailing-event evidence](https://github.com/anthropics/claude-agent-sdk-typescript/blob/040a7f855da8261c795d975e116f40bb042334f7/CHANGELOG.md).

Known progress/retry events are observations, not terminal failure by themselves. Unknown additive
events cannot authorize orchestration, another worker, broader access or a payment-path change.
The final error, nonzero exit, unresolved cleanup or missing proof cannot be erased by late text.

## Decision 5: distinguish requests, settings and observed model use

Preserve the requested model selector literally and forward it without substitution. Native
aliases retain their native meaning; comparing an alias to a canonical model string does not
establish a contradiction. Provider configuration, remapping and vendor-owned fallback can affect
reported use. An explicit selector is not an account selector.
[Native model semantics](https://code.claude.com/docs/en/model-config#model-aliases).

Treat `system/init.model` as a configured setting only. Attribute the singular actual main model
only from an unambiguous main-conversation assistant `message.model`, with
`parent_tool_use_id: null`. Missing or conflicting attribution stays unknown. Do not take the first
map key, assign every listed model to the main author, or rewrite the request using a pricing ID.
Preserve `modelUsage` as a separate whole-tree reported model set.
[Pinned assistant-model parsing](https://github.com/anthropics/claude-agent-sdk-python/blob/3be907edb9c86269d30d1427a77937dd788d6419/src/claude_agent_sdk/_internal/message_parser.py#L151).

Tag observations with source, observation time and scope. Result `usage` covers the top-level agent
loop; result `modelUsage` includes nested work. Repeated assistant blocks can share message ID and
usage, so naive summation double-counts. Parse counters from raw JSON numeric lexemes into checked
signed-64 decimal values rather than float64. Keep absent/partial values distinct from zero.
Native cost values are client estimates, not bills, prices guaranteed to this account, remaining
allowance or measured savings. [Usage scopes and estimate limitations](https://code.claude.com/docs/en/agent-sdk/cost-tracking).

Optional observations remain bounded by the existing event/metadata limits. Do not represent
omitted or unsupported observations as a complete measured total. Unknown account, provider,
quota, context occupancy and external activity remain unknown unless the supported native output
actually supplies the relevant sourced fact. The SDK schema is supporting evidence for these
fields, not a new telemetry service or runtime dependency.

## Decision 6: retain structured native failures without probing or fallback

Recognize the documented result error variants and preserve bounded native diagnostics. Known
assistant/retry categories include authentication failure, organization refusal, held account,
billing error, rate limit, overload, invalid request, unavailable model, server error and cloud
credential failure. A 429 differs from a 529; neither account exhaustion nor account identity can
be inferred from an arbitrary message. Some startup failures provide only stderr and nonzero exit.
[Message reference](https://code.claude.com/docs/en/agent-sdk/typescript#sdkresultmessage).

Do not turn an internal retry event into a Council retry, automatic model escalation or another
attempt. Native retry duration remains subject to Council's existing deadline/owned cleanup.
Unknown categories remain retained unknown failures; status, wait, metadata, discovery, history
and evidence inspection make zero model calls. [Native error behavior](https://code.claude.com/docs/en/errors).

**Alternatives rejected**: auth-status/version probes on refresh, parsing credentials to improve
login labels, inferred quota replenishment, a retrying supervisor, and SDK/API fallback.

## Decision 7: keep one global slot and preserve Auto's Codex meaning

Resolve existing Auto/omitted harness choice to Codex until feature 004 defines economical routing.
Claude is an explicit optional target. Capture the resolved actual connection on each new attempt;
do not reinterpret historical Auto attempts using later selection rules.

Use one global managed-native capacity resource shared by Codex, Claude and their labelled fakes.
The resource identity is distinct from actual `codex-native`/`claude-native` connection identity,
an account and provider allowance. A narrow schema-2-to-3 transaction rebuilds the existing
Codex-constrained claim table around this shared resource, retaining any held attempt reference.
No account pool, concurrency expansion, routing model call or parallel task graph is added.

Preserve exact existing task/draft versions, attempts, native observations, artifact/source/candidate
references, claims, incidents, terminal records, event order and stored receipt envelopes. Receipt
replay precedes new capability/policy checks and returns the original result bytes. A migration
does not clear unknown cleanup, declare an old process absent or replay a worker. Keep OS ownership,
private SQLite sidecar protection, WAL/FULL durability, transactional migration failure and
unknown-schema refusal intact. Legacy Council state is not opened or migrated.

**Alternatives rejected**: retaining `codex-native` as the shared pool name for Claude, clearing old
claims to admit new work, rewriting historical JSON, one slot per harness, or economic routing
before its own specified feature.

## Decision 8: separate older-owner maintenance from new execution compatibility

A new client may explicitly stop the accepted feature-002 owner using its known API-1, dev.2,
`002-v1` maintenance shape, or a current compatible owner. This is an operation-specific known
compatibility matrix, not permission to execute Claude through the older owner. Keep new
execution's exact contract checks strict; a future maintenance capability can be advertised
without pretending the old binary already supplied it.

Read the existing private descriptor, authenticate bounded loopback metadata, verify the exact
owner instance/API/known maintenance shape, and send the existing Stop operation with captured
owner/request identities. Permit unknown additive metadata only within that bounded maintenance
decoder; retain strict JSON validity, duplicate rejection and required identity checks. Do not
relax mutation requests.

After an uncertain acknowledgement, retry the same explicit owner target and request identity.
Never rebind that pending intent to a replacement descriptor. Old receipt replay cannot stop a
new owner. This path does not start an owner, open/migrate SQLite behind an old owner, dispatch a
task, kill a PID, delete a lock or take over unknown ownership. Unknown compatibility remains an
actionable refusal. Normal upgrade begins only after the old owner releases native ownership.

**Alternatives rejected**: exact app-version equality for all maintenance, API-version-only
execution compatibility, automatic old-owner shutdown, PID-based takeover and implicit retry
against whichever owner is current.

## Decision 9: require a real native executable and fresh nonpersistent sessions

Resolve and validate a direct native executable. On Windows, `.cmd`/`.ps1` wrappers are unsupported
launch forms unless a separately qualified filesystem resolution finds the actual native EXE;
Council does not invoke cmd/PowerShell to interpret them. Current npm installs can provide the
same native binary through platform packages; Node's installation requirement does not establish
a Node runtime adapter. No stable universal shim-to-EXE mapping is assumed.
[Installation contract](https://code.claude.com/docs/en/setup#install-with-npm),
[launcher troubleshooting](https://code.claude.com/docs/en/troubleshoot-install).

Start a fresh nonpersistent native session per attempt. Preserve a sourced native session ID only
as an observation. Declare native session continuity unsupported: no `--resume`, `--continue`,
private transcript read or unrelated personal-conversation reuse. Council's persisted task/history
and explicitly linked later attempts remain separate from native conversation persistence.

Do not promise zero vendor bookkeeping writes or zero telemetry from `--no-session-persistence`.
Council itself changes no vendor-global setting. Optional environment switches can alter native
features and do not establish universal no-network/background guarantees; they are not added to
this frozen profile. [Environment behavior](https://code.claude.com/docs/en/env-vars).

## Qualification and resolved scope

All design choices above are resolved for this slice; missing observations remain declared unknown
facts rather than requests for configuration or silent substitutions. Implementation qualification
must distinguish:

1. **Deterministic authored fixtures**: Claude-shaped success-with-error, final result, progress,
   retries, partial/malformed/truncated/oversized output, multiple/conflicting main results, session
   mismatch, trailing events, aliases/multiple models, large counters, Unicode response, denial,
   short stdin and Stop/failure races. Fake observations are labelled and never become measured
   provider use.
2. **Shared native/offline proof**: source preservation, response/candidate integrity, independent
   mixed-harness clients sharing one slot, queue transfer, request replay, no-replay recovery,
   preserved schema-2 data/receipts/claims/incidents, exact older-owner Stop, branded keyboard/mobile
   UI and extracted-package journeys. All six supported native targets and the supported race gate
   remain required before feature acceptance.
3. **Live vendor qualification**: still unperformed and unauthorized by the rebuild. It requires a
   specific later user-requested live task. Documentation, fake protocol tests and native ownership
   checks cannot be relabelled as successful live Claude execution or subscription savings.

No additional SDK, paid API path, native resume, automatic routing, extra managed worker,
independent semantic review, deterministic verifier, integration or publication is introduced by
this research decision.
