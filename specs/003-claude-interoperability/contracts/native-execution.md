# Native execution contract: Codex and Claude Code

**Feature**: `003-claude-interoperability`
**Version**: API 1 / execution `003-v1` / application `2.0.0-dev.3` / schema 3
**Status**: Design independently reviewed; no implementation or live qualification claim

This extends [002 execution](../../002-managed-execution/contracts/execution.md) and the
[003 data model](../data-model.md). All existing task/control/acceptance routes, strict mutation
shapes, envelope/status/receipt rules, exact counters, list cursors, byte reads, wait semantics,
owner security and CLI exit behavior remain unless explicitly changed below. Paths are relative
to `/api/v1`. No new task mutation or session-resume endpoint is added.

## Current owner and capability handshake

`GET /meta` retains foundation fields and now declares:

```json
{
  "appVersion": "2.0.0-dev.3",
  "apiVersion": 1,
  "ownerInstanceId": "0123456789abcdef0123456789abcdef",
  "scope": "managed_execution",
  "managedExecution": "native_codex_claude",
  "capabilities": ["workspace_foundation", "managed_execution"],
  "executionContract": "003-v1",
  "maintenanceContract": "owner-stop/1",
  "admission": {"state": "open", "incidentId": null},
  "capacity": {"resourceId": "managed-native", "limit": 1, "activeAttemptId": null}
}
```

These are `data` members in the existing authenticated envelope. Admission remains `open`,
`closing` or `fenced`, with the existing optional incident. `resourceId` is deliberately separate
from actual `connectionId`: either harness consumes the one owner-managed resource. No separate
Codex/Claude/account slots are implied. Claims remain held through verified cleanup; unknown
cleanup/faults fence dispatch rather than allowing the other harness to bypass them.

A 003 task-execution client requires exact current application/execution identity, API 1,
managed-execution capability, the expected shared-resource shape and a verified exact owner.
A 002 owner is not accepted for 003 task execution. API-1 identity alone is insufficient. The
special maintenance verifier below grants only known Stop compatibility. Never kill/replace an
incompatible owner, migrate behind it or reinterpret old metadata as Claude support.

`GET /execution/capabilities` changes from a single connection to this bounded declaration:

```json
{
  "resourceId": "managed-native",
  "limit": 1,
  "defaultHarnessId": "codex",
  "connections": [
    {
      "connectionId": "codex-native",
      "harnessId": "codex",
      "interface": "native_exec",
      "nativeContract": "codex-exec-jsonl/002-v1",
      "minimumVersion": null,
      "documentedVersion": null,
      "versionSource": "accepted_002_contract",
      "supportedPolicyModes": ["autonomous"],
      "modelSelection": "explicit_selector",
      "accountSelection": "unsupported",
      "sessionContinuity": "unsupported",
      "sessionPersistence": "disabled_for_managed_attempt",
      "cancellationScope": "windows_job",
      "liveQualification": "unqualified",
      "fakeMode": true,
      "limits": {
        "defaultTimeoutSeconds": 1800,
        "maxTimeoutSeconds": 86400,
        "promptBytes": 262144,
        "stdoutBytes": 67108864,
        "stderrBytes": 16777216,
        "jsonlLineBytes": 1048576,
        "eventBytes": 16384,
        "chunkBytes": 65536,
        "finalResponseBytesUpperBound": 67108864
      }
    },
    {
      "connectionId": "claude-native",
      "harnessId": "claude-code",
      "interface": "native_exec",
      "nativeContract": "claude-print-stream-json/003-v1",
      "minimumVersion": "2.1.259",
      "documentedVersion": "2.1.318",
      "versionSource": "official_documentation_2026-11-06",
      "supportedPolicyModes": ["autonomous"],
      "modelSelection": "explicit_selector",
      "accountSelection": "unsupported",
      "sessionContinuity": "unsupported",
      "sessionPersistence": "disabled_for_managed_attempt",
      "cancellationScope": "windows_job",
      "liveQualification": "unqualified",
      "fakeMode": true,
      "limits": {
        "defaultTimeoutSeconds": 1800,
        "maxTimeoutSeconds": 86400,
        "promptBytes": 262144,
        "stdoutBytes": 67108864,
        "stderrBytes": 16777216,
        "jsonlLineBytes": 1048576,
        "eventBytes": 16384,
        "chunkBytes": 65536,
        "finalResponseBytesUpperBound": 1048576
      }
    }
  ]
}
```

`cancellationScope` is `unix_process_group` on Unix. The Claude response upper bound is not a
promise that a full 1 MiB decoded response fits: JSON framing/escaping and other required members
make accepted result text strictly smaller than the one-line limit. The prompt bound is not a
token/context guarantee. These declared version/profile assumptions and dates are separate from
an observed installed/invocation version and from fake/platform/live qualification. A null
Codex version requirement does not qualify arbitrary native protocol versions; its accepted
interface contract remains unchanged.

Capability and discovery reads remain filesystem/metadata-only, making zero vendor/model calls.
The five discovery entries retain order and unknown authentication/provider/account/allowance
facts. Codex and Claude declare `native_supported_live_unqualified`; other harnesses remain
`not_implemented`. A shell wrapper alone is not a supported native executable. A detected unknown
version is shown as unknown, not verified compatibility. A known incompatible prerequisite blocks
specifically; an actual unsupported-option/authentication failure is retained as native evidence.

## Request and CLI continuity

Existing `POST /tasks`, `PUT /tasks/{id}/guidance` and `POST /tasks/{id}/attempts` bodies are
unchanged. `pins.harnessId: "claude-code"` now resolves to `claude-native`; `codex` and omitted/empty
Auto resolve to `codex-native`. Other recognized harness choices retain their existing stored
meaning but cannot launch in this slice. Do not insert default pins into request fingerprints.
All other captured defaults/provenance and unknown/null/duplicate-member rejection remain.

CLI `tasks start --harness claude-code` and the existing versioned guidance `--harness` option map
literally to `pins.harnessId`. Existing `--model` maps literally to `pins.model`. No account wizard,
new connection file, harness-specific CLI task command or hidden login is introduced. Running an
existing task uses its current captured version; changing a harness requires explicit guidance
and another explicit start. UI uses the same optional target choice and clearly names the chosen
attempt connection. Auto is not economical cross-target routing until feature 004.

Explicit account selection and Council-managed `paid_api` remain unsupported; do not silently
substitute payment paths. Native configured billing/internal retries remain vendor-owned. Supervised
Council approval remains unavailable: an unsupported requested policy blocks instead of silently
downgrading. Existing request IDs, optimistic revision/version checks and exact receipt replay
before current prerequisites still apply across this upgrade.

## Native Claude profile

Resolve an absolute supported native executable using the existing metadata-only resolver
discipline. Invoke it directly with an argument array, no shell. The profile is:

```text
--print
--input-format text
--output-format stream-json
--verbose
--no-session-persistence
--permission-mode bypassPermissions
--permission-prompts none
[--model <exact captured selector>]
```

The bracketed option is present only for a nonempty explicit selector; the selector is one literal
argument. The native version profile is Claude Code 2.1.259 or later, based on the documented
permission-prompts flag and the dated 2.1.318 protocol reference. This is a bounded documented
assumption, not live proof of all future releases. Do not add fallback-model, continue/resume,
fork/session-ID, remote/cloud, worktree, API-key or interactive approval options.

Supply the same bounded full original/guidance/acceptance prompt through text stdin and prove its
complete write and closure. Do not put task text in argv. Codex retains its current native flags,
ephemeral execution, output reservation and final-response-file strategy; no Claude flags leak
into Codex invocation. A small builder switch and a shared bounded parser interface (`Write`,
`Finish`, `Result`, `Failed`) isolate native differences; process ownership and settlement remain
the same engine path.

Do not select `--bare` or `--safe-mode`. Ordinary native authentication, applicable instructions
and customizations remain native-owned. In particular bare changes subscription-authentication
behavior, and safe mode suppresses CLAUDE.md/customizations. Do not overwrite global settings or
create a substitute project instruction file to make orchestration easier. The applicable source
scope remains 002's captured managed Git source; required excluded/unrepresentable context is an
explicit blocker/limitation. Ordinary native hooks, policy and customizations can impose limits or
perform work; this profile does not claim universal isolation or suppress them.

`bypassPermissions` plus non-interactive prompt denial maps the saved autonomous preference to a
supported native request. Native/managed denials still remain possible and visible. No universal
OS sandbox, guaranteed account allowance, maximum undocumented native model-call count or cost
cap is promised. Native init may report its mode separately from Council's requested/configured
mode. Authentication/version/help commands, credentials and live tasks are not invoked by
development qualification; no provider call is authorized by this feature.

## Claude JSONL and completion protocol

Raw stdout is written/retained before interpretation. JSONL parsing is incremental across arbitrary
chunks, strict valid UTF-8/JSON with duplicate-member rejection, and bounded by the same 1 MiB line,
64 MiB stdout, 16 MiB stderr and 16 KiB typed-event limits. Do not repair malformed lines, replace
invalid Unicode or extract a plausible JSON substring. Unknown additive valid message members and
message kinds may remain raw evidence; they do not become orchestration commands.

The one-shot profile supplies one prompt and accepts exactly one recognized main-conversation
`type: "result"`. Its `subtype` must be `success` and `is_error` must be present and false for
possible authoring success. A typed native error, incompatible/error result subtype, recognized
assistant error, contradictory terminal facts or authoritative refusal/error diagnostic records
a native failure. Missing/wrong mandatory terminal fields, a missing terminal, repeated main
results (even identical), conflicting sessions, malformed/truncated essential output or an
incomplete final line are `protocol_invalid`. An excessive line is `protocol_limit`. A repeated
result cannot be resolved by taking the first or last response. Unknown optional telemetry does
not manufacture a success or invent counters.

A known main session is established by recognized init/result `session_id`; subsequent recognized
main-session IDs must match. A subagent's explicitly attributed identity is not the main session.
No session observation triggers resume or acquires ownership of an unrelated personal chat.
Native session continuity is unsupported in 003 despite retained Council history.

Do not require the result to be the last physical line. Continue consuming valid bounded trailing
observations, diagnostics and background completion through EOF/root exit and required drains.
Progress/API-retry/rate observations are not completion percentages, terminal failures by
themselves or subscription-exhaustion proof. A trailing typed failure remains authoritative.
Optional native session-persistence-loss diagnostics remain evidence; Council's own artifact
durability and supported no-persistence profile are evaluated separately.

Recognized main assistant `message.model` with explicit null `parent_tool_use_id` can supply an
actual primary-model observation only if all attributable main observations are unambiguous.
Init `model` is configuredModel only. Result `modelUsage` supplies a separate whole-tree reported
model preview; result `usage` supplies top-level counters. Do not sum repeated assistant messages,
choose the first model-map entry, treat a pricing canonical ID as the selected model or label
every internal/subagent model as a primary mismatch. Requested aliases retain literal native
semantics. The model/usage sources, nulls, exact strings, preview omission and raw evidence links
are specified in the data model.

Native-shaped fake output remains readable protocol evidence but cannot populate actual provider,
model, version, session or measured-usage observations. The attempt stays explicitly `fake`, with
unknown real-world facts and no live-qualification claim. Typed UI rendering may be tested with
separately labelled controlled-owner responses.

## Final response and durable settlement

Only a recognized successful result's decoded `result` string supplies the Claude final response.
Preserve exact decoded UTF-8, including valid replacement characters, BOM characters, whitespace
and newlines; no trim/reformat/synthetic newline is allowed. A missing/wrong-type `result` on the
recognized success record is `protocol_invalid`; an empty string is valid text but not useful
response output. Assistant prose, tool output,
partial deltas and native saved transcripts are never substituted. No guessed output file is
reserved, opened or imported for Claude.

After EOF, root exit/drains, verified owned-scope cleanup and complete stdin delivery, publish
usable decoded text with the ordinary immutable evidence `Put` operation as `final_response`.
The file bytes must be durable before final metadata publication; retain its actual identity,
length and SHA256. A Put/Close/verification failure retains available evidence and follows the
existing persistence/evidence incident and fence rules, not a retry. An actual metadata Commit
error or lost acknowledgement remains uncertain even if cancellation also occurred.

Possible authoring success still requires the recognized error-free terminal, zero root exit,
input proof, usable response or attributable useful authored output under 002 rules, complete
required evidence and verified cleanup. Existing primary-failure/Stop/deadline order wins over
late success. Never settle while an owned worker may still append evidence, or release the shared
claim based solely on a result label. Unknown cleanup retains capacity/fence and exact incident.

All source/index/branch/user-edit preservation, candidate attribution, pending verification and
response-only acceptance conditions remain unchanged. Acceptance re-verifies the exact registered
closed response identity/bytes/hash through the controller proof and checks exact attempt/version,
no authored file or index delta and no outstanding required stages. Client fields alone are not
proof. Authoring success never implies checks, review, integration, commit or publication.

## Exact-owner backward maintenance Stop

`POST /service/stop` remains `{requestId, ownerInstanceId}` with the original operation/fingerprint,
202 acknowledgement and receipt semantics. The CLI uses a separate maintenance handshake rather
than granting old task-execution compatibility.

Known accepted combinations are deliberately enumerated:

| Owner | Required compatibility |
| --- | --- |
| Current `2.0.0-dev.3` | API 1, exact instance, `003-v1`, `maintenanceContract: "owner-stop/1"`, current managed-execution metadata/resource shape. |
| Accepted `2.0.0-dev.2` | API 1, exact instance, `002-v1`, scope `managed_execution`, `managedExecution: "native_codex"`, foundation/execution capabilities, known admission enum and capacity `{connectionId:"codex-native",limit:1,...}`. No new maintenance field is required retrospectively. |
| Anything else | Refuse as unknown/incompatible; no PID signal, lock removal, takeover, auto-start or new execution. |

The current task verifier remains strict for 003 app/execution compatibility. Do not accept an
app-version prefix, arbitrary API-1 owner or future maintenance string without a known compatible
operation definition. An older dev.2 CLI is not retroactively made future-compatible.

Maintenance procedure:

1. Read the existing private descriptor. Do not call an Ensure/start-owner path or open SQLite.
   Request bounded authenticated `/meta` at its exact numeric loopback origin with bearer token,
   proxy/redirect behavior disabled as in 002. Require matching API 1 in envelope/data, a valid
   response request ID and exact descriptor `ownerInstanceId`.
2. Decode metadata with strict JSON/Unicode/duplicate/type validation and an 8192-byte limit.
   Tolerate additive unknown metadata members only in the maintenance decoder; require every
   known identity/compatibility member for the selected whitelist row. This does not relax
   mutation request validation or the task-execution handshake.
3. Bind the exact intended owner once. With no supplied `--owner-instance`, capture the verified
   descriptor instance before sending. With an explicit earlier target, retain it. Send the
   existing Stop body/request ID to the authenticated owner with the exact required Origin.
4. On lost acknowledgement or another uncertain result, retain/report the captured instance and
   request ID for an exact retry (`service stop --owner-instance ID --request-id ID`). An actionable
   bounded error can carry this non-secret retry target; no token/task body enters diagnostics.
   Do not silently reread/rebind an unresolved Stop intent to a replacement owner.
5. If a replacement compatible owner is explicitly contacted to reconcile the earlier intent,
   the body still targets the old instance. Existing exact stored receipt replay occurs before
   current-owner checking and may return the old acknowledgement; it must never trigger current
   owner shutdown. If that receipt does not exist, reject the mismatched owner target without
   stopping the replacement. No automatic retarget or new Stop request is synthesized.

Unknown authentication/instance/API/contract or a changed owner fails safely with actionable
foreground/manual shutdown guidance. A request accepted by the old owner initiates its existing
bounded cleanup; the acknowledgement is not a cleanup-complete receipt. Only after it releases the
lifetime lock may current startup open/migrate supported state and perform no-replay recovery.

## Preserved control, faults and contract evidence

Existing exact counters/cursors, strict greater-than wait revisions (including zero/future cursor),
bounded reads, append-once terminals, queued deadlines, guidance invalidation, Stop watchdog,
persisted/in-memory fences and explicit next-attempt requirements apply unchanged to both
harnesses. No filesystem or process I/O executes inside store transactions. Known precommit
cancellation remains distinct from actual Commit/after-Commit uncertainty and real durability
faults; uncertainty never authorizes automatic replay.

The 003 implementation must provide offline fixture evidence for both launch shapes, Claude
terminal/error/trailing/multiple-result behavior, input and response fidelity, exact counters,
primary-versus-whole-tree models, partial metadata previews/raw links, fake telemetry separation,
one mixed-harness slot, Stop/deadline/fault/recovery, preserved source edits/candidates/acceptance,
schema migration/receipt preservation and the enumerated older-owner Stop/replacement cases.
UI/CLI must expose the same captured target, capabilities, observations, outcome, uncertainty and
next action while preserving delayed-reply/edit guards. Supported native/race/build/browser gates
remain required; no development result counts as a live vendor qualification or saving.
