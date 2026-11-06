# Managed execution contract

**Feature**: 002-managed-execution

**Branch**: `refactor`

**Application**: `2.0.0-dev.2`; API 1; state schema 2

**Status**: Implementation contract; qualification and remaining limits are recorded in acceptance.md. No live provider qualification is implied.

This extends [foundation.md](../../001-workspace-foundation/contracts/foundation.md) and uses
[data-model.md](../data-model.md). Foundation project/policy/draft routes retain their behavior,
strict validation and exact persisted receipt replay. This contract adds one native Codex worker
attempt per explicit execution. It does not implement another harness, routing, task graphs,
verifiers, automatic integration, publication, MCP or automatic recovery/retry.

## Authority, compatibility and common values

Retain the existing private user-home `.council/v2/` root, OS lifetime lock, authenticated numeric
loopback owner, exact Host/Origin validation, no redirects, no-store responses, fragment bootstrap,
CSP and bounded non-sensitive diagnostics. Every private GET requires the bearer token. Mutations
require the exact owner Origin, including CLI requests. GET bodies, duplicate/unknown query keys,
invalid methods/routes and arbitrary artifact filesystem paths reject without effects.

Use strict UTF-8 JSON objects: reject duplicate/unknown members, wrong types, explicit null in
request fields, malformed Unicode and trailing values. Optional request fields are omitted when
absent; an empty object/array is meaningful only where documented. Preserve original valid text
bytes after JSON decoding without normalization/trimming/BOM or newline conversion. IDs are exactly
32 lowercase hexadecimal characters. Positive revisions/versions and nonnegative offsets/counts
are canonical decimal strings through `9223372036854775807`; never JSON numbers. Small bounded
options such as timeoutSeconds/limit are JSON integers. UTC times retain milliseconds and `Z`.

Reuse the exact foundation envelope: `{apiVersion:1, requestId, data, receipt?}` or
`{apiVersion:1, requestId, error:{code,message,currentRevision?}}`. Data and error never coexist;
errors have no receipt. Messages are actionable English, at most 512 UTF-8 bytes, with no raw
subprocess output, task body, bearer token or credential. Complete evidence is read privately.

### Owner capability handshake

`GET /meta` retains appVersion, apiVersion and ownerInstanceId, and adds the following current
capability-scoped information:

```json
{
  "scope": "managed_execution",
  "managedExecution": "native_codex",
  "capabilities": ["workspace_foundation", "managed_execution"],
  "executionContract": "002-v1",
  "admission": {"state": "open", "incidentId": null},
  "capacity": {"connectionId": "codex-native", "limit": 1, "activeAttemptId": null}
}
```

These are `data` members alongside the retained metadata, not a second envelope. Admission state
is `open`, `closing` or `fenced`. A 002 client may use 001 foundation capabilities with an API-1
001 owner, but must see `managed_execution` and `executionContract: "002-v1"` before execution
mutations. Missing/incompatible capability is `capability_unavailable`; never silently kill/replace
that owner or reinterpret an old `not_implemented` value as execution support. Existing receipt
bodies remain unchanged. No claim of compatibility with a different API major version is made.

`GET /execution/capabilities` returns `{connectionId:"codex-native", harnessId:"codex",
interface:"native_exec", supportedPolicyModes:["autonomous"], modelSelection:"explicit_selector",
accountSelection:"unsupported", cancellationScope, liveQualification:"unqualified", fakeMode:true,
limits}`. cancellationScope is `windows_job` or `unix_process_group` for this runtime. Limits are
the finite values below. These declare adapter behavior, not authentication, quota, effective model
identity or a live connection test; this read invokes no vendor executable/model. If the supported
native interface is unavailable, native admission gets an actionable prerequisite/capability error.
A supervised request blocks in this slice because no interactive Council approval channel exists.

Discovery remains metadata-only with the existing five harness entries and separate unknown
facts. Codex's managedExecution becomes `native_supported_live_unqualified`; the other four remain
`not_implemented`. Finding a launcher or declaring capability does not qualify a live provider.

## Request types and captured defaults

### New task and first execution

`POST /tasks` admits one new Task, version 1 and one queued Attempt atomically:

```json
{
  "requestId": "11111111111111111111111111111111",
  "projectId": "22222222222222222222222222222222",
  "originalText": "Explain the selected module's public entry point.",
  "acceptance": {"deliverable":"response_only"}
}
```

Only requestId, projectId and originalText are required. originalText has the existing non-whitespace
and 4 MiB UTF-8 rules. Optional members are:

| Member | Type / default / validation |
| --- | --- |
| `sourceDraftRef` | `{draftId, revision}`; optional exact provenance, not an alternative text source. The immutable saved draft project/text must exactly equal the supplied projectId/originalText. Mismatch is `source_draft_conflict`, with no admission. |
| `expectedPolicyRevision` | Optional positive revision of the last-shown global policy, direct-text starts only. Compare atomically when capturing policy; a mismatch is revision_conflict with that current policy revision and no admission. Mutually exclusive with sourceDraftRef. |
| `pins` | Object below; omitted or `{}` means Auto. |
| `policyOverride` | Literal `supervised` only, a narrower task request; unsupported supervision blocks this start before admission. An existing task's guidance may retain the narrower request without launching it. No broadening of a supervised inherited preference is accepted. |
| `acceptance` | Object below; omitted uses the documented candidate-preparation default. |
| `timeoutSeconds` | Integer 1–86400, default 1800; absolute admitted deadline includes queue/preparation and is returned in Attempt. |
| `executionMode` | `native` default or explicitly supplied `fake`; no automatic fake fallback. |
| `fakeScenario` | In fake mode only: `response`, `candidate`, `partial_failure`, `slow`, `late_success`, `owned_descendant` or `cleanup_unknown`; default `response`. It selects fixed local fixture behavior, never a command or executable path. |

`pins` accepts optional `harnessId`, `model`, `accountId` and `paymentPath` only. harnessId accepts
the five canonical discovery IDs but only codex can dispatch; omitted means codex in this slice.
model is an exact native selector, 1–256 UTF-8 bytes, with no NUL/control characters or whitespace-only
value; preserve it, never silently trim or substitute. accountId is an opaque ID, and paymentPath is
`native` or `paid_api`. Explicit account selection or paid_api is currently unsupported and blocks
before launch. There is no paid fallback. Different pins can be introduced only in an explicit new
task version, never from agent prose or a launch failure. Native configured billing/overage remains
vendor-owned and is not a Council-enforced spending guarantee.

`acceptance` accepts these optional fields with a complete resolved object retained in TaskVersion:

| Member | Type / default |
| --- | --- |
| `deliverable` | `candidate` default, or `response_only`. Candidate means useful authoring with verification pending. |
| `conditionsText` | Exact user conditions, default empty; at most 4 MiB UTF-8. It remains part of the authoritative prompt. |
| `requiredEvidence` | Unique array from `check`, `independent_review`, `local_integration`, `external_delivery`; default empty. These identify unavailable later stages, not a configurable workflow engine. |
| `allowCandidatePreparation` | Boolean default true. If false and a required final stage is unsupported, reject before launch rather than silently convert the requested final deliverable to partial authoring. |

Native authoring capabilities are checked before dispatch regardless of allowCandidatePreparation.
A candidate or any observed authored delta always stays verification pending. A response-only task
with requiredEvidence still outstanding cannot use manual response acceptance. Declared external
requirements are retained/pending or blocked, never authorized by full access alone and never
implemented as a publication stage here. Native assistant prose cannot add authority or mutate
these fields.

Direct creation snapshots current global policy inside admission. With sourceDraftRef, it captures
that exact draft version's requestedPolicy instead; an optional narrower override is separate.
Neither path changes the draft or global policy. Effective permission/model/account facts are
separately sourced attempt observations, not inferred from the requested snapshot or harness name.
UI direct Start pins the last-shown global preference with expectedPolicyRevision; CLI callers
can supply the same assertion. A policy conflict preserves the clicked intent and requires
reconciliation rather than silently adopting a broader policy. Omitting this optional assertion
on the minimal direct API deliberately captures the global policy current at admission.

### UI Start from a saved or edited draft

The user can click Start without first clicking Save. When a saved draft's current composer text
is modified, the UI freezes the clicked text/policy intent and saves that exact draft version using
the existing expectedRevision and a retained pending-save requestId. After its acknowledgement it
submits POST /tasks with those exact bytes and sourceDraftRef, using a separate retained pending-start
requestId. Autosave is deterministic and makes no model call or extra confirmation step.

A later keystroke does not alter either pending intent or get overwritten by its acknowledgement.
A lost save/start reply reconciles its original ID/input. A stale draft conflict preserves the
clicked text and shows reconciliation; it does not silently start from a different revision or
switch the draft policy to current global preference. Draft saved/start failed is reported as that
partial preparation, never Started. Direct unsaved task entry may use POST /tasks without a draft
and captures global policy as above. The UI makes the captured policy source visible.

### Existing task guidance and another explicit execution

`PUT /tasks/{id}/guidance` accepts `{requestId, expectedRevision, guidanceText,
useCurrentPolicy?, policyOverride?, pins?, acceptance?, timeoutSeconds?}`. guidanceText is the full
additional guidance for the new version (empty allowed, max 4 MiB), always alongside unchanged
originalText. Omitted optional fields keep the prior values. `useCurrentPolicy` defaults false.
True captures the current global snapshot. policyOverride can set only supervised; to remove an
old override, use `clearPolicyOverride:true` (mutually exclusive with policyOverride). This is an
additional optional member, default false. Complete supplied pins/acceptance objects replace their
prior resolved object using defaults; they do not partially merge with a hidden old pin.

The update creates a new immutable version and no model call. Unlaunched old-version attempts
settle blocked/guidance_changed; already launched work keeps its snapshot and Stop remains available.
No new version automatically runs. The client can retrieve earlier versions explicitly.

`POST /tasks/{id}/attempts` accepts `{requestId, expectedRevision, taskVersion,
executionMode?, fakeScenario?}`. taskVersion must equal the current version; expectedRevision guards
the clicked task mutation intent. It creates exactly one new attempt linked to previousAttemptId,
after verifying no unsettled task attempt, no ownership/cleanup fence and compatible prerequisites.
It does not change guidance, pins, captured policy or prior outcome. Each execution has one worker;
manual execution is not automatic retry. Replaying its requestId returns that admitted attempt.

## HTTP routes and results

All routes are relative to `/api/v1`; undocumented members/queries reject. No GET has a body.

| Method / route | Input | Success `data` / status |
| --- | --- | --- |
| `GET /execution/capabilities` | None | 200: capability declaration above. |
| `POST /tasks` | New task/start input | 201: `{task:TaskSummary, version:TaskVersion, attempt:AttemptSummary}` plus admission receipt. |
| `GET /tasks` | Optional projectId, limit, cursor | 200: stable `{items:TaskSummary[], nextCursor}`. |
| `GET /tasks/{id}` | None | 200: `{task:Task, version:TaskVersion, latestAttempt:AttemptSummary or null, acceptance:ResponseAcceptance or null}`. |
| `GET /tasks/{id}/versions/{version}` | None | 200: `{taskId, originalText, version:TaskVersion}`. |
| `PUT /tasks/{id}/guidance` | Guidance input | 200: `{task:TaskSummary, version:TaskVersion}` plus receipt. |
| `POST /tasks/{id}/attempts` | Explicit existing-task start | 201: `{task:TaskSummary, version:TaskVersion, attempt:AttemptSummary}` plus receipt. |
| `GET /tasks/{id}/attempts` | Optional limit, cursor | 200: `{items:AttemptSummary[], nextCursor}` with all prior outcomes. |
| `GET /attempts/{id}` | None | 200: complete Attempt metadata with target observations, cleanup, source/candidate/evidence refs; no whole transcript. |
| `POST /attempts/{id}/stop` | `{requestId}` | 202: `{attemptId, stopAccepted:true, attemptRevision, cleanupState}` plus receipt; or 200 settled no-op below. |
| `GET /attempts/{id}/wait` | Optional afterRevision, timeoutMs | 200: `{attempt:AttemptSummary, changed, timedOut}`; timeout preserves actual state. |
| `GET /attempts/{id}/events` | Optional limit, cursor | 200: stable `{items:AttemptEvent[], nextCursor}`; bounded metadata/artifact links. |
| `GET /attempts/{id}/artifacts` | Optional limit, cursor | 200: stable `{items:ArtifactMetadata[], nextCursor}`. |
| `GET /artifacts/{id}` | None | 200: ArtifactMetadata; owner validates retained reference before file access. |
| `GET /artifacts/{id}/bytes` | Optional offset, limit, through | 200: exact base64 byte chunk described below. |
| `POST /tasks/{id}/response-acceptance` | Acceptance input below | 200: `{task:TaskSummary, acceptance:ResponseAcceptance}` plus receipt. |
| `POST /service/stop` | `{requestId, ownerInstanceId}` | 202: `{ownerInstanceId, shutdownAccepted:true, admission:"closing"}` plus receipt; not a verified cleanup receipt. |

Attempt GET, bounded-wait and current-task detail additionally resolve optional `sourceSnapshot`,
`candidate` and `incident` objects. Source metadata includes baseCommit, branch, manifest hash and
selected relative folder. Candidate metadata includes managed workspaceRef, delta/index flags,
verification and integration state. Incident includes its exact retained id/code/message/time;
metadata admission also exposes its optional incident. These are descriptive typed records, never
arbitrary filesystem read targets. Lists remain compact; no full artifact body is embedded.

TaskSummary excludes original/guidance/conditions bodies. It includes id, projectId, currentVersion,
revision, stateRevision, latestAttemptId, captured requested mode/source, timestamps, status, stage
and nextAction. Task detail exposes complete original text and the current exact version.
AttemptSummary includes immutable IDs/taskVersion/previousAttemptId, phase, outcome, revision,
executionMode, admission/deadline/start/settlement times, bounded primary failure, accepted Stop,
cleanup/evidence status and response/candidate IDs. Summaries never imply full evidence was read.
Current reads include owner observedAt and nonnegative integer remainingSeconds so the UI can
show the captured deadline and remaining time, including queue consumption, without another model
call or settings form. Replayed receipts contain their original observation, not refreshed time.

Target observations use `{status:"known"|"unknown", value, source, observedAt}` per provider,
account, model and native session fact. Unknown has value/source/observedAt null. Requested model
selector is separate from reported model identity. Usage has separate reported token counts as
decimal strings or null, source/time/completeness, never estimated charges from call counts.
Permission data records requested mode, adapter mapping/configured mode, any native-reported mode
and limitations separately. Mapping native full-access behavior does not attest universal access.

### Stop, shutdown and precedence

Stop targets the exact attempt ID and requires no progress revision. This avoids losing a valid
Stop merely because output arrived after the button was drawn. Atomically accept Stop, close
further admission for this attempt/task execution and record its order before signalling owned
processes. A queued attempt cannot launch after that transaction. Duplicate same-ID Stop replays
its original result; a new Stop against a settled attempt commits a 200 no-op receipt containing
`{attemptId,stopAccepted:false,reason:"already_settled",attemptRevision,cleanupState}` without
rewriting outcome. Existing primary failure and accepted cancellation outrank late success.

Cleanup grace is 2 seconds, total stop/termination/drain effort 15 seconds. Within that total the
owner exposes verified settlement or a retained unresolved incident. Output/evidence/process
cleanup states remain separate. Unknown cleanup globally fences new dispatch; the slot is not
released because an HTTP acknowledgement arrived. A primary failure observed before Stop stays
primary. Induced exits/later cleanup faults remain linked secondary events; a Stop accepted first
still remains accepted if later cleanup/evidence faults produce an explicit blocked incident.

CLI `service stop --owner-instance ID` optionally preserves an explicit owner target for exact
receipt replay; otherwise the verified current descriptor supplies it. A replay is checked before
current-owner compatibility and cannot close a replacement owner.

Service stop validates the exact current ownerInstanceId, durably closes admission and requests
settlement for its owned work before shutting down. Request identity includes the target owner.
Replay of an old service-stop receipt cannot stop a new owner. The public command contacts an
existing owner only; it does not create a new service merely to stop it. At restart, uncertain
ownership/evidence produces an interrupted incident/fence; no agent is automatically replayed.

### Response acceptance

Input is `{requestId, expectedRevision, taskVersion, attemptId, responseArtifactId,
responseSha256, confirmation:"reviewed_response_and_conditions"}`. SHA256 is exactly 64 lowercase
hexadecimal characters. The owner validates all identities/hash and requires: latest attempt,
current task version, succeeded authoring, response_only deliverable, no authored delta, complete
durable response/evidence, verified cleanup and no outstanding requiredEvidence. Missing evidence,
a stale version, failure, cancellation or an unverified change cannot be accepted by this route.
The acceptance check reads the closed response bytes and verifies their stored SHA256 and
physical artifact identity; a cached old hash cannot accept a subsequently modified artifact.
Changed/missing/redirected evidence produces an explicit retained corruption incident.

The receipt records an explicit user-review conclusion linked to that response and applicable
conditions, rather than a process-exit acceptance or an observed test result. It cannot waive a
required check or independent review. Same-ID replay is exact; a different operation attempting
to accept an already accepted same response returns `acceptance_conflict` instead of a second
acceptance record. This is an optional bounded preview completion path, not a permanent approval
step for every answer in the finished product.

### Bounded waits, lists and byte reads

Lists reuse foundation limit 1–100/default 25, opaque ASCII cursor up to 2048 bytes, immutable
ascending admission/event sequence and first-page inclusive maximum. Filter/resource identity is
bound into the cursor. Later inserts do not create skipped rows. Changing metadata may be newer;
no point-in-time whole-task snapshot is claimed. No deletion/retention pruning is introduced.

Wait uses afterRevision as a nonnegative decimal string (default `0`) and timeoutMs integer
0–30000 (default 30000). Return immediately if the attempt revision is greater than afterRevision,
it is settled, or timeoutMs is zero. Otherwise wait for a committed change or bounded timeout.
`timedOut:true` is observation only, never failure, cancellation or task acceptance. Disconnecting
a wait cancels that observation, not the attempt. All observation is deterministic, zero model calls.

ArtifactMetadata is `{id,attemptId,kind,byteLength,closed,completeness,sha256,createdAt}`; hash is
null until a closed complete artifact is finalized. completeness is collecting/complete/incomplete/
uncertain. Managed candidate metadata also identifies its source snapshot, after-manifest, exact
source-relative delta and private workspace location. A workspace path is descriptive metadata,
not an arbitrary path accepted by an evidence read endpoint.

Byte offset defaults `0`; byte limit is integer 1–65536/default 65536. through is an optional
nonnegative decimal snapshot bound. An initial read fixes through to the currently known durable
length; subsequent chunks reuse that exact bound. Reject offset > through, through > known durable
length and arithmetic overflow. Return:

```json
{"artifactId":"33333333333333333333333333333333","offset":"0","nextOffset":"12","through":"12","bytesBase64":"SGVsbG8gd29ybGQh","eof":true,"streamClosed":true,"completeness":"complete","sha256":"<the complete artifact hash>"}
```

The example's hash placeholder is explanatory, not a permitted wire value. eof means the requested
snapshot bound was reached; streamClosed separately says whether additional bytes can arrive.
Offsets are byte counts, so chunks need not end on UTF-8 boundaries. Clients reassemble bytes or
use a streaming decoder; invalid raw output remains available without replacement corruption.
For closed complete artifacts, sha256 is the retained expected whole-artifact hash. The first
range read derives bounded aligned-chunk digests in the same pass that verifies that whole hash.
Subsequent reads reopen/validate physical identity and size and hash the complete aligned chunks
spanning the requested range before returning bytes. This proves requested-range integrity; damage
to an unrelated chunk is detected when accessed or by fresh whole-artifact verification. Response
acceptance always verifies the complete current file and bypasses the range cache. Internal source
materialization and full-response copying hash the bytes actually copied in one linear pass.

Reading a live artifact again without through obtains a newer durable prefix. An explicit empty
artifact returns empty base64 and equal offsets. No result preview replaces retained bytes.

## Mutation receipts, faults and source integrity

New operation names are tasks.start, tasks.guidance, tasks.attempts.start, attempts.stop,
tasks.response.accept and service.stop. They share the existing global receipt namespace and
canonical validated-input fingerprint rules. Same-ID same-intent replay is checked before stale
revision/capability/prerequisite checks and returns exact original status/data/receipt, not a
refreshed status or new invocation. Fingerprints include path target, expected revision/version,
original text, every explicit option and exact sourceDraftRef. Normalize documented defaults
consistently; retain optional presence where it changes intent. Compare original client intent
before resolving current policy/observations again, so a lost-ack replay cannot acquire a new
policy or fail merely because the global policy changed. Different input/operation with the same
identity conflicts.

Receipt commits durable requested intent/admission, not process success. Start task/version/attempt
and receipt commit together. Actual dispatch additionally requires durable capacity/launch intent,
current version/policy, uncancelled state and an open gate. Known pre-admission faults have no
receipt and launch no agent; preparation faults after admission retain blocked/failed attempts.
Unknown post-launch writes close conflicting dispatch and never trigger another call to recreate
output. A lost reply keeps its same pending request ID/input; clients cannot claim Not started
merely because transport failed.

Source and candidate artifacts obey the data model: actual dirty/unignored current source plus
original HEAD/index/branch identity; candidate delta against that snapshot rather than HEAD.
The worker's staged index is reconstructed without checkout/filters, then exact current working
bytes are copied. Its staged/unstaged semantic distinction must match the original; an archived
index alone is insufficient. Index-only authored changes remain visible and block response-only
acceptance. Original source/index/branch remain unchanged. Safe isolation limitations or source bounds block
before launch, with no older-base fallback. Raw stdout limit is 64 MiB, stderr 16 MiB, native JSONL
line 1 MiB. Limit/protocol/evidence faults retain exact obtained bytes and prevent false success.
The assembled Council prompt, including metadata/brief plus verbatim original text, applicable
guidance and acceptance conditions, is capped at 256 KiB before vendor launch. Oversized inputs
remain durably retained under their 4 MiB-per-text storage caps, but execution blocks explicitly;
there is no clipping or summary substitution. This is a Council transport/resource limit, not a
token/model-context/charge guarantee for an unknown native model. Capability limits and errors
explain that distinction without adding mandatory tuning settings.
A valid stored task over the assembled prompt bound keeps its admitted original/version and a
blocked attempt with zero native invocation; a 4 MiB storage-valid input does not silently vanish
because it is too large for this execution transport.
No arbitrary assistant prose is parsed as a new orchestration command or task authorization.

## Errors and CLI parity

Retain all foundation status/code mappings and add:

| HTTP / code | Meaning |
| --- | --- |
| 409 `source_draft_conflict`, `task_version_conflict`, `attempt_active`, `acceptance_conflict` | Reconcile exact source/version/attempt; preserve current work and request intent. |
| 422 `capability_unavailable`, `pin_unavailable`, `permission_unavailable`, `source_capture_blocked`, `acceptance_unavailable` | Named unsupported capability/choice/source or unmet acceptance condition; no affected new launch. |
| 503 `dispatch_fenced`, `owner_closing` | Inspect retained incident or allow orderly shutdown; no unsafe dispatch. |

Authoring/provider/process failures after admission are retained Attempt outcomes, not an HTTP
error pretending the admission never happened. Full diagnostic output is private evidence. Task
and attempt reads return 200 with truthful failed/blocked/cleanup state. They do not erase artifacts.

The release entrypoint remains council. Foundation commands keep their behavior. All new commands
support `--json`, `--api-version 1`, strict unknown/duplicate-flag rejection and one newline-terminated
envelope on stdout; diagnostics go to stderr. `--task-file -` and `--guidance-file -` explicitly read
strict UTF-8 stdin. File names are not retained as task provenance. No shell invocation is involved.

| Command | HTTP-equivalent behavior |
| --- | --- |
| `council tasks start --project ID (--task TEXT|--task-file PATH)` | POST /tasks; optional --source-draft ID with --source-draft-revision R, or --expected-policy-revision R; --model SELECTOR, --harness ID, --account ID, --payment-path VALUE, --policy supervised, --deliverable candidate|response_only, --conditions-file PATH, --require-evidence VALUE (repeatable unique), --final-only, --timeout-seconds N, --fake [--fake-scenario VALUE], --request-id ID. |
| `council tasks list [--project ID] [--limit N] [--cursor VALUE]` | GET task summaries. |
| `council tasks get ID` | GET full original/current version plus latest bounded state. |
| `council tasks version ID --version V` | GET immutable task version. |
| `council tasks guidance ID --expected-revision R (--guidance TEXT|--guidance-file PATH)` | PUT guidance; optional --use-current-policy, --policy supervised or --clear-policy-override, and the same versioned pin/acceptance/deadline options. No run. |
| `council tasks run ID --expected-revision R --task-version V` | POST another explicit attempt; optional --fake/--fake-scenario and --request-id. |
| `council tasks attempts ID [--limit N] [--cursor VALUE]` | GET complete attempt lineage through bounded pages. |
| `council attempts get ID` | GET actual metadata/outcome and evidence refs. |
| `council attempts wait ID [--after-revision R] [--timeout-ms N]` | Bounded observation only; disconnect/timeout does not Stop. |
| `council attempts stop ID [--request-id ID]` | Exact-attempt Stop, no stale progress-revision requirement. |
| `council attempts events ID [--limit N] [--cursor VALUE]` | GET bounded event metadata. |
| `council attempts artifacts ID [--limit N] [--cursor VALUE]` | GET stable artifact metadata. |
| `council artifacts get ID` | GET metadata only. |
| `council artifacts read ID [--offset N] [--limit N] [--through N]` | GET byte chunk; JSON keeps base64/exact decimal strings. Human output labels byte offsets and does not claim an incomplete preview is the full result. |
| `council tasks accept-response ID --expected-revision R --task-version V --attempt ID --response ID --response-sha256 HASH --reviewed` | POST exact response acceptance; --reviewed maps to the required literal confirmation, no interactive approval wizard. Optional --request-id. |
| `council service stop [--request-id ID]` | Contact the current authenticated owner, capture its exact ownerInstanceId, POST shutdown intent; never launch a service merely to stop it. |

--request-id on every mutation is generated once if absent and retained/reported on uncertainty.
Paired/mutually exclusive source/file flags are validated before contacting the owner. Repeated
--require-evidence is the only documented repeatable option. --final-only sets
allowCandidatePreparation false. The CLI must not read unrelated credential/configuration files.

Execution starts require a persistent owner: reuse the compatible UI/background owner or self-spawn
the same executable in an internal service mode with no task/token in argv. Publish/access its
private descriptor after lock/schema readiness. Concurrent launchers converge on one owner.
Read-only foundation commands may keep transient ownership when no service/task exists. The
service's lifetime is independent of the submitting/observing CLI and browser. Foreground `ui`
Ctrl+C explicitly shuts down its owner, which is different from closing an observing browser.

Exit 0 means the requested admission/control/read operation was acknowledged, never task acceptance.
Start displays Admitted/Queued, not Completed. A wait timeout exits 0 with timedOut and actual state.
Retain foundation exit categories: 1 internal/storage/evidence-write uncertainty, 2 invalid input,
3 missing/prerequisite/unsupported capability, 4 revision/request/task-state conflicts, 5 owner/
authority/dispatch-fence errors. Ctrl+C of an observer exits 130 without cancelling the task;
explicit owner Ctrl+C performs orderly shutdown. An attempt read can exit 0 while its data says
failed/cancelled/verification pending. Automation must inspect those explicit outcome fields.

## Required contract evidence

Use deterministic local fixtures only. Cover direct text and exact-draft/autosave starts, original
bytes/policy fidelity, explicit pins/modes, dirty/untracked source and source-relative delta, immutable
versions/previous outcomes, finite deadlines, duplicate start/control receipts, two-client shared
capacity, lost acknowledgements, UI stale reads, long/chunk-boundary byte reads, output/protocol
faults, late success/primary failure ordering, descendants, orderly shutdown, owner crash and
persistence uncertainty. Verify browser/CLI identity/outcome/cleanup parity and retained evidence
across restart with zero automatic replay. Recheck private-root/authority/strict JSON/cursor/large
revision boundaries and release exclusions. Fake mode is visibly fake; live Codex remains
unqualified until a separate specific user-authorized live task. No acceptance passes are claimed
by this design document.
