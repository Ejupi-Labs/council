# Data model: Claude Code interoperability

**Feature**: `003-claude-interoperability`
**Branch**: `refactor`
**Contract**: API 1, application `2.0.0-dev.3`, execution `003-v1`, state schema 3
**Status**: Design independently reviewed; implementation and qualification pending

This extends the accepted [002 model](../002-managed-execution/data-model.md) and implements
[spec.md](spec.md). The wire, native-profile and maintenance contracts are defined in
[contracts/native-execution.md](contracts/native-execution.md). Existing 001/002 entities and
mutation bodies remain authoritative except for the explicit extensions below.

## Compatibility and value representations

Retain the private user-home `.council/v2/` root, application ID, lifetime OS owner lock, physical
identity/private-file checks and WAL/FULL durability. Legacy Council state is not opened. Schema
3 adds no account pool, session store, task graph, router, verification or integration entity.

IDs retain the existing 32 lowercase hexadecimal representation. Revisions, versions, sequence
numbers and nonnegative counts are canonical decimal strings bounded by signed 64-bit maximum
`9223372036854775807`; counts allow `0`. Small bounded options remain JSON integers. Times remain
UTC RFC 3339 milliseconds with `Z`. User text remains exact valid Unicode without trimming,
normalization, newline conversion or BOM removal. Unknown observations are explicit unknowns,
never invented zeroes, account identities or quota estimates.

New observation members are optional (`omitempty` pointer/member semantics). Their absence in a
historical record means not recorded; reads must not fabricate that an old adapter declared or
observed a new fact. Existing `ObservedFact` wire status remains `observed` or `unknown`, matching
the accepted runtime. An observed fact has non-null `value`, `source` and `observedAt`; an unknown
fact has null values. Do not rewrite historical status spellings, JSON or receipts.

## Captured target and resolved connection

`ExecutionPins` and its wire name `pins` are unchanged: optional `harnessId`, `model`, `accountId`
and `paymentPath`. No `effectiveHarness` is injected into the incoming object, immutable version
or receipt fingerprint.

| Captured pin | Resolution in 003 |
| --- | --- |
| Omitted/empty `harnessId` | Existing Auto meaning: resolve to `codex`, including another explicit attempt against an old version. No cross-harness routing. |
| `harnessId: "codex"` | Resolve to actual connection `codex-native`. |
| `harnessId: "claude-code"` | Resolve to actual connection `claude-native`. |
| Other recognized harness | Remains a valid stored choice where 002 permits guidance storage; unsupported execution blocks specifically. No fallback. |

New `Attempt.connectionId` is the actual immutable resolved connection, either `codex-native` or
`claude-native`; `requestedTarget` remains the original version's requested pins. This already
records resolution without another redundant harness field. Historical 002 attempts keep their
literal `codex-native` value and every other stored field. Shared capacity is a separate resource,
not a reinterpretation of connection identity.

`Task`, `TaskVersion`, policy/draft provenance, optimistic `revision`, observation `stateRevision`,
lineage, explicit-start semantics and guidance cancellation of obsolete queued work are unchanged.
A new harness choice requires an explicit new immutable guidance version. It never reroutes an
already admitted attempt. Receipt replay precedes current version/policy/capability checks and
returns the original stored envelope/status.

## Connection capability declarations

`GET /execution/capabilities` returns the two declared connection profiles and one shared resource,
as specified in the contract. This is a declaration of Council behavior, not a vendor probe.
The connection declaration includes these concrete fields:

| Field | Meaning |
| --- | --- |
| `connectionId`, `harnessId` | `codex-native`/`codex` or `claude-native`/`claude-code`. |
| `interface` | Literal `native_exec`. |
| `nativeContract` | `codex-exec-jsonl/002-v1` or `claude-print-stream-json/003-v1`. |
| `minimumVersion` | Claude `2.1.259`; null for the existing Codex profile, whose accepted interface contract is retained. This is a declared profile assumption. |
| `documentedVersion` | Claude `2.1.318`; null for Codex. A dated documentation reference, not the installed version or a live qualification. |
| `versionSource` | Claude `official_documentation_2026-11-06`; Codex `accepted_002_contract`. |
| `supportedPolicyModes` | `["autonomous"]`; supervision remains unsupported. |
| `modelSelection` | `explicit_selector`; native selector semantics, not remote identity attestation. |
| `accountSelection` | `unsupported`; no account or paid-API substitution. |
| `sessionContinuity` | `unsupported`; Council history and a reported native session ID do not grant resume support. |
| `sessionPersistence` | `disabled_for_managed_attempt`; fresh one-shot invocation for each explicit attempt. |
| `cancellationScope` | Runtime `windows_job` or `unix_process_group`. |
| `liveQualification` | `unqualified`; fake/native-platform checks do not change this to live-qualified. |
| `fakeMode` | `true`; a separately requested local execution mode. |
| `limits` | Finite per-connection input/output/line/event/response bounds. |

Existing five-entry discovery order and installation evidence remain unchanged. Claude's
`managedExecution` becomes `native_supported_live_unqualified`, like Codex. Installation remains
`installed`, `unsupported_launcher`, unavailable or unknown according to the existing scanner;
finding a file does not verify its version, authentication or working protocol. UI/CLI join the
declaration to installation/attempt evidence by `harnessId`; no duplicated persisted capability
catalog is needed. A known incompatible prerequisite blocks; an unobserved version remains
unknown rather than becoming a verified supported version. Refresh/capability reads invoke no
vendor executable.

## Attempt target observations

The existing `provider`, `account`, `model`, `nativeSession`, `permissions` and `usage` members
remain. Add the following optional members to `TargetObservations`:

| Member | Type and source |
| --- | --- |
| `configuredModel` | `ObservedFact`; Claude init `model`, source `claude.system.init.model`. It is a setting, not an effective-model proof. |
| `nativeVersion` | `ObservedFact`; Claude init `claude_code_version`, source `claude.system.init.claude_code_version`. It describes that invocation; no separate version command is implied. |
| `nativeModels` | Optional `NativeModelsObservation` described below, sourced from recognized result `modelUsage`. |

Reuse `nativeSession` for a recognized native `session_id`, with source
`claude.system.init.session_id` or `claude.result.session_id`. A conflicting main-session ID is a
protocol error, not another conversation to attach. No native session is resumed, opened by ID or
treated as owned because its ID is visible.

For Claude, existing singular `model` is observed only from attributable main-conversation
assistant `message.model` (`parent_tool_use_id` explicitly null). Repeated observations of the same
nonempty ID are compatible. Different main-model IDs, absent parent attribution or otherwise
ambiguous identity leave the singular fact unknown; raw/model-set evidence is retained. The source
is `claude.assistant.message.model`. An init model, first `modelUsage` key, subagent model or pricing
`canonicalModel` cannot fill this field. It does not claim that all internal work used that model.

The exact captured model selector is passed literally to the native CLI. Alias resolution,
provider remapping and native configuration retain native meaning. A configured/reported string
difference alone cannot establish `pin_mismatch`. 003 adds no inferred exact-model attestation
from Claude init/model-usage data. Any future exact-pin proof must identify an unambiguous actual
primary source rather than requiring every background model to match the request.

### Usage and per-model preview

Extend existing `UsageObservation` with optional `scope` and `cacheCreationInputTokens`:

| Member | Rule |
| --- | --- |
| Existing `inputTokens`, `outputTokens`, `cachedInputTokens` | Null or exact nonnegative decimal string. No float64 conversion or summing repeated assistant messages. |
| `cacheCreationInputTokens` | Optional exact count, mapping a supported native cache-creation counter. |
| `scope` | Optional `top_level`, `whole_tree` or `unknown`. Claude result `usage` is `top_level`. Absent on old records means scope was not recorded. |
| Existing `source`, `observedAt`, `completeness` | Claude source `claude.result.usage`; owner observation time; `complete`, `partial` or `unknown`. Complete means complete for the declared scope/counters, not every provider cost or model call. |

`NativeModelsObservation` has the following concrete shape:

```json
{
  "scope": "whole_tree",
  "source": "claude.result.modelUsage",
  "observedAt": "2026-11-06T12:00:00.000Z",
  "completeness": "complete",
  "evidenceArtifactIds": ["0123456789abcdef0123456789abcdef"],
  "omittedModelCount": "0",
  "models": [{
    "model": "native-reported-model-id",
    "inputTokens": "12",
    "outputTokens": "4",
    "cachedInputTokens": null,
    "cacheCreationInputTokens": null,
    "costEstimateUsd": "0.00012"
  }]
}
```

This is a bounded preview of native-reported whole-tree model use, separate from singular primary
identity and top-level usage. Each entry's counters are exact strings or null; optional native
cost maps to `costEstimateUsd`, a client estimate, never a charge, price guarantee or subscription
usage. Retain a valid nonnegative finite JSON-number lexeme as a string (at most 64 ASCII bytes),
without float conversion or an invented currency conversion. Zero is retained only if actually
reported. Aggregate `total_cost_usd` and unsupported extra cost/provider/canonical-model fields
remain in raw evidence; do not assign them the top-level usage scope or invent a new billing DTO.

Preview bounds: at most 16 model entries, each model ID at most 256 UTF-8 bytes; deterministic
byte-lexicographic ordering is presentation only, never primary-model selection. A longer ID is
omitted, not truncated into a different identity. `omittedModelCount` is the exact omitted-entry
count, or null if the supported decoder cannot establish it. `evidenceArtifactIds` contains the
same attempt's registered raw stdout artifact (one ID in this profile); no private path is public.
Every preview omission, unsupported/malformed optional counter or additional serialized-budget
omission makes `completeness: "partial"` (or `unknown` when no reliable model observation exists).
Do not mark a partial preview complete or silently replace unknown counters with zero.
Here `complete` means the full native-reported model set and every supported counter actually
reported for it were represented; an absent optional native cost/counter is still null, not a
claim that all possible telemetry exists. The observation does not claim native accounting is
independently complete.

All typed observation metadata together must fit the existing 16 KiB event bound. Reduce only
optional preview entries/details, updating completeness/omission facts, to make room for required
identity/permission/failure fields and the evidence link. If necessary omit all preview entries
and retain an explicit partial summary. Full JSONL stays available through stdout within its own
64 MiB bound. Never put final-response text, tool transcripts or whole user instructions in event
metadata. Malformed essential protocol, output loss and line/total limit breaches still fail the
attempt; explicit optional-preview reduction is not loss of the raw artifact.

New optional model/version fact values are at most 256 UTF-8 bytes; an oversized optional value
stays unknown with a bounded omission diagnostic and raw evidence link, never a truncated identity.
The Claude main session identifier is a required supported identity when reported: a non-string,
empty or over-256-byte session identifier is `protocol_invalid`. Fixed source identifiers and
owner timestamps are bounded independently, so optional native strings cannot exhaust the event
budget or displace an authoritative failure.

### Fake execution observations

`executionMode: "fake"` remains immutable and visible. Native-shaped fixture fields are protocol
test evidence, not observations about a provider/account/model/version/session or measured usage.
Keep those actual facts unknown/absent, including `configuredModel`, `nativeVersion` and
`nativeModels`; do not promote fixture counters into usage. The raw stdout fixture and explicit
local-fake capability/test provenance remain retrievable. Configured Council permission mapping
may still be shown as configuration, without implying actual provider permission. Controlled
owner/UI test fixtures may expose authored observation shapes, labelled as controlled test data.

## Response, candidate and lifecycle invariants

Artifact kinds, source/candidate identity, hash/closed-file verification and separate response
acceptance remain unchanged. Codex retains its physical output reservation/import. Claude's
authoritative decoded successful result string becomes a normal immutable `final_response`
artifact only after bounded EOF, root exit, stream drain, complete input delivery and verified
owned-scope cleanup. Raw stdout/stderr and final-response bytes have independent artifact IDs.
Do not synthesize an output file path, borrow a native transcript or extract arbitrary prose.

The 1 MiB native JSONL line bound makes Claude's decoded final response strictly smaller than
1 MiB after JSON overhead; the 64 MiB Codex file limit does not apply to Claude result text.
Preserve decoded UTF-8 bytes exactly without trimming or newline addition. Empty/unusable text is
not a usable response. Existing useful candidate rules, authored-index-delta checks and optional
response-only acceptance proof still apply. No task acceptance, verification, integration, commit
or publication is added.

One accepted Stop, primary failure order, fixed admitted deadline, append-once terminal record,
uncertain-write fence, owner epoch and restart-without-replay rules apply identically to both
harnesses. A result cannot release capacity while owned descendants, drains or evidence writes
remain uncertain. No lifecycle table or new outcome enum is needed.

## Schema 2 to 3: one shared capacity resource

Only the capacity-claim table changes shape:

```sql
CREATE TABLE capacity_claims_v3 (
  resource_id TEXT PRIMARY KEY CHECK(resource_id='managed-native'),
  attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id)
);
```

The final table name remains `capacity_claims`. Existing empty/one-row schema-2 claims map from
`connection_id='codex-native'` to `resource_id='managed-native'`, preserving `attempt_id`. The pool
key is not copied into `Attempt.connectionId`. All scheduling/gate queries use the shared key;
the unique key plus unique attempt reference enforce one claimed worker across both harnesses.

Migration is an explicit transaction under the already acquired OS lifetime lock, with SQLite
foreign keys enabled and no filesystem/process/vendor I/O inside the transaction:

1. Check application ID and exact supported source schema before migration. Validate expected
   foundation/execution tables, current claim table DDL (including its CHECK/unique/FK constraints),
   column shape, absence of unexpected claim-table indexes/triggers, valid claim cardinality and
   referenced attempt. A matching column probe alone is insufficient to establish compatible DDL.
2. Create the new constrained table, copy the old claim with the fixed new resource key, verify
   copied cardinality/reference, drop only the validated old claim table and rename the new table.
   Do not disable foreign keys, delete the claimed attempt or reset gate/incident state.
3. Validate final claim DDL/constraints and `foreign_key_check`, then set `user_version=3` in the
   same transaction. No task, version, attempt, terminal, event, artifact, candidate, policy, draft,
   receipt, incident or `execution_control` row/JSON is rewritten.
4. Commit once. A known failure before Commit rolls back schema/version/data. An actual Commit
   error is uncertain: fail startup, retain the original files and require reopening/revalidation;
   do not assume migration did not commit or try reset/destructive repair. A successful migration
   followed by lost acknowledgement is safely recognized as validated schema 3 on next open.

Fresh state initializes schema 3 directly. Schema 1 follows the existing validated foundation-to-
execution migration and then the same claim transformation, without changing preserved foundation
records. Unknown/future application IDs, schemas or incompatible DDL fail closed before journal
configuration/mutation; they are never silently rebuilt. Retain the existing compatible-file
preflight before SQLite opens companions. After supported schema validation/migration, verify
WAL and synchronous FULL as in 002, with per-connection foreign keys/FULL/busy timeout surviving
connection replacement. No NORMAL fallback or unsafe sidecar deletion is introduced.

Migration preserves a held claim and any unresolved gate/incident. Current-owner recovery, outside
the DDL transaction, records interrupted work and cleanup uncertainty under the accepted rules;
DDL alone cannot prove cleanup or release capacity. An older running owner must first be stopped
through the explicit maintenance operation and release the owner lock. No concurrent migration
or lock takeover is permitted.

## Design verification obligations

The implementation tasks must cover populated schema-2 preservation (including exact receipt
envelopes, large counters, held claims and incidents), rollback/uncertain migration, fresh/schema-1
paths, unknown-DDL refusal, shared mixed-harness capacity, unchanged old receipt replay before
current policy checks, typed preview completeness, fake-observation separation and all common
cleanup/recovery invariants. This document records the required behavior, not a test result.
