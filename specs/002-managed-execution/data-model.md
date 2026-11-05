# Data model: Managed native Codex execution

**Feature**: 002-managed-execution

**Branch**: `refactor`

**Contract**: API 1, application `2.0.0-dev.2`, state schema 2

**Status**: Design for root review; not implementation or live qualification evidence

This extends the accepted [001 model](../001-workspace-foundation/data-model.md) and implements
[spec.md](spec.md). The wire/CLI contract is [contracts/execution.md](contracts/execution.md).
One explicit execution creates one worker attempt. There is no retry engine, task graph, model
router, review/verifier engine, local integration or MCP interface in this feature.

## Compatibility and representations

Keep all 001 project, policy, draft/version and receipt identities and stored contents. Migrate
schema 1 to 2 transactionally under the existing OS lifetime lock, validating application ID and
schema first. Failure rolls back; unsupported schemas are errors, never reset instructions.
Existing receipts replay their exact stored API-1 envelope/status, including their earlier content.
A fresh schema-2 store initializes the same 001 defaults plus empty execution records.

The private root remains `<user-home>/.council/v2/`, with the same canonical physical-root and
permission checks. Legacy state is not opened, migrated or used as an execution source. Retained
workspaces, byte artifacts and incident evidence are private state, excluded from Git/releases.

| Value | Rule |
| --- | --- |
| Identity | Existing random 128-bit, exactly 32 lowercase hexadecimal characters; never a PID/path. |
| Revision/version/event sequence | Canonical positive decimal string, exact signed 64-bit bound. Persist integer types; overflow rejects before mutation. |
| Offset/byte count/usage count | Canonical nonnegative decimal string, allowing exactly `0`; same exact bound. Never convert through a JavaScript number. |
| Time | Existing UTC RFC 3339 milliseconds with `Z`; owner clock, not a concurrency token. |
| User text | Exact valid Unicode scalars; no trimming, normalization, newline/BOM conversion or malformed-input replacement. |
| Unknown observation | Explicit `null` value with `status: "unknown"`; never guessed identity or zero usage. |
| Source paths | Manifest stores exact relative path bytes as base64 plus a nullable lossless display string; no path normalization can merge distinct files. |

Original text, complete additional guidance and acceptance-conditions text each allow at most
4 MiB UTF-8. The assembled Council prompt, including metadata/brief and the verbatim original,
applicable guidance and conditions, must fit 256 KiB; reject before vendor launch rather than
clip or replace authoritative text with a summary. The complete larger input remains durably
retrievable. This is a Council transport/resource bound, not a token, model-context or charge
guarantee: the native effective model/context may be unknown. Draft storage and execution-input
bounds are intentionally different. HTTP bodies retain the independent 32 MiB bound.
A valid stored task exceeding the assembled execution bound retains its admitted input/version
and a blocked attempt before native launch; it does not lose its original text or rerun a model.
Ordinary source snapshots are limited to 100,000 entries, 64 MiB per regular file and 512 MiB
total captured regular-file bytes; exceeding
any bound blocks preparation explicitly. These are finite supported bounds, not sampling rules.

## Task and immutable task versions

`Task` is the enduring user request; `TaskVersion` is the exact instruction/policy snapshot
eligible for a subsequent explicit execution.

| Task field | Meaning |
| --- | --- |
| `id`, `projectId` | Stable task and immutable project link. |
| `originalText` | Complete immutable original task, stored once; generated output/guidance never replaces it. |
| `sourceDraftRef` | Null, or exact `{draftId, revision}` provenance. The referenced project/text must equal this task's project/text. |
| `currentVersion` | Latest immutable task-version number. |
| `revision` | Optimistic token for explicit task mutations: new execution, guidance/policy update, accepted Stop and response acceptance. |
| `stateRevision` | Independent observation token advanced by owner lifecycle/evidence commits; async progress does not create stale user-edit conflicts. |
| `latestAttemptId` | Null until admission, then latest explicit attempt. Prior attempts remain linked and retrievable. |
| `createdAt`, `updatedAt` | Initial and latest explicit-mutation times; stage observations have their own times. |

| TaskVersion field | Meaning |
| --- | --- |
| `taskId`, `version`, `createdAt`, `createdBy` | Composite identity, immutable creation attribution (`user`). |
| `guidanceText` | Complete additional user guidance for this version, initially empty. Always supplied alongside the full original text. |
| `acceptance` | `{deliverable, conditionsText, requiredEvidence, allowCandidatePreparation}` as defined in the contract. |
| `requestedPolicy` | Full 001 global/draft policy snapshot: mode, selectionSource, revision, updatedAt. |
| `taskPolicyOverride` | Null or supported narrower requested mode; stored separately from the inherited snapshot. |
| `pins` | Explicit harness/model/account/payment choices; absence means Auto, not a fabricated effective identity. |
| `timeoutSeconds` | Finite admitted duration; default 1800, range 1–86400. |

Direct task creation captures the global policy atomically. Optional expectedPolicyRevision
compares the last-shown global policy inside that same transaction and rejects a mismatch before
admission, preventing a stale narrower UI choice from silently using newly broadened preference.
Without it, the minimal API explicitly captures the current global policy. It is invalid together
with sourceDraftRef, whose exact immutable policy is already pinned. An exact sourceDraftRef instead
captures that immutable draft version's policy; supplied text/project equality is checked before
admission. Editing the composer does not silently change an existing draft's policy. UI Start may
first save the exact clicked draft intent using the existing draft receipt/revision rules, then
start from that acknowledged exact ref. It uses separate pending save/start request IDs, preserves
newer typing, and cannot claim Started from the draft save alone. No separate Save click is needed.

A guidance update creates a complete new version and advances Task.revision; omitted fields keep
the previous values. `useCurrentPolicy: true` explicitly captures the global policy atomically;
otherwise the inherited snapshot is unchanged. A changed broader preference never rewrites a
captured task. Updates do not launch anything. A queued attempt referencing an obsolete version
becomes terminal `blocked` with `guidance_changed` before dispatch. Dispatched attempts retain the
old version unchanged. The next execution requires an explicit start against the latest version.

Only one unsettled attempt may exist for a task. A manual new execution is allowed after a prior
blocked/failed/cancelled/succeeded attempt has verified cleanup and no dispatch fence. It creates
a new attempt ID, links `previousAttemptId`, and changes neither the previous terminal record nor
its artifacts. Interrupted/unknown cleanup is not a retry permission.

## Attempt, acceptance and cleanup are separate facts

| Attempt field | Meaning |
| --- | --- |
| `id`, `taskId`, `taskVersion`, `previousAttemptId` | Immutable execution lineage. |
| `startRequestId`, `ownerInstanceId`, `connectionId` | Admitted operation, owning service and literal `codex-native` capacity pool. |
| `executionMode` | `native` or explicitly requested `fake`; fixtures cannot be reported as native work. |
| `phase` | `queued`, `preparing`, `running`, `cancelling` or `settled`. |
| `outcome` | Null until settled, then `succeeded`, `failed`, `cancelled`, `blocked` or `interrupted`. Succeeded means authoring only. |
| `revision` | Independent exact owner state counter for coherent attempt reads/waits. |
| `admittedAt`, `deadlineAt`, `startedAt`, `settledAt` | Deadline is admission time + captured timeout, including queue time; others nullable until observed. |
| `observedAt`, `remainingSeconds` | Derived read-time owner observation and nonnegative integer remaining deadline time. Receipt replay retains its original snapshot; fetch current state for current remaining time. |
| `requestedTarget`, `targetObservations` | Requested selectors/pins and separate sourced effective identity/capability facts. |
| `sourceSnapshotId`, `candidateId`, `responseArtifactId` | Null until available; retained stable artifact/change identities. |
| `stop` | Null or `{requestId, acceptedAt, eventSequence}`; durable acceptance order, not proof of termination. |
| `primaryFailure` | Null or bounded `{code, message, observedAt, eventSequence}`; full diagnostics linked as evidence. |
| `cleanup` | Independent state described below. |
| `evidenceStatus` | `collecting`, `complete`, `incomplete` or `uncertain`; missing evidence cannot imply complete success. |

`AttemptTerminal` is append-once and keyed uniquely by attemptId. It records outcome, first
primary failure/accepted cancellation ordering, native root terminal/exit observations, final
known artifact references and settlement time. A later manual execution cannot rewrite it.
Native root completion, process exit, output drainage, durable evidence and owned-process cleanup
are distinct observations. Successful authoring requires a usable response/candidate, compatible
root terminal plus successful exit, complete durable evidence and verified settlement. Provider
prose claiming tests ran is retained output, never an observed verifier receipt.

Cleanup is `{state, scope, startedAt, finishedAt, incidentId}` where state is `not_started`,
`active`, `verified` or `unknown`; scope is `windows_job`, `unix_process_group` or `not_launched`.
An OS process scope is ownership/termination machinery, not a universal security sandbox. No
PID-only takeover/termination is allowed. Capacity remains reserved until cleanup is verified;
unknown ownership/cleanup retains the claim and globally fences new dispatch in this slice.

Accepted Stop closes that task's admission and survives late success. A primary fault observed
before Stop remains primary; induced exits and subsequent cleanup faults are linked secondary
events. If Stop was accepted first, retain cancellation as the primary control outcome; late
success cannot clear it, and cleanup/evidence faults remain separately visible and fence dispatch.
Thus a cancelled attempt with unknown cleanup never appears quiescent: its task shows Blocked /
cleanup unresolved, its accepted Stop remains visible, and the Cancelled label is qualified.

| Task presentation | Rule / next action |
| --- | --- |
| Queued / Running / Cancelling | Latest attempt phase, with actual stage and same identity. |
| Verification pending | Succeeded authoring with any authored delta or a candidate deliverable; no task completion. Inspect candidate; feature 005 supplies verification/delivery. |
| Response acceptance pending | Successful response-only attempt, no authored delta, complete response, no outstanding declared checks/review/delivery. Inspect and optionally accept exact response. |
| Completed | Separate response acceptance exists for latest current-version response and all bounded eligibility conditions hold. |
| Failed / Cancelled | Settled corresponding outcome with verified cleanup; partial output stays available. |
| Blocked | Unsupported authoring capability, obsolete queued version, required unavailable outcome or cleanup/evidence/ownership fence; name the next action. |
| Interrupted | Owner lost before finalized settlement; retain incident and known artifacts, no replay. |

Response acceptance is `{id, requestId, taskId, taskVersion, attemptId, responseArtifactId,
responseSha256, basis: "user_review", confirmation, acceptedAt}`. It is permitted only against the
latest current-version successful response-only attempt, no authored delta, verified cleanup,
complete evidence and no outstanding requiredEvidence. The user explicitly confirms inspecting
the response and applicable conditions. It does not waive declared checks, accept prose test
claims or create a permanent mandatory approval phase in the completed product.

If guidance advances after a terminal attempt, that old outcome/acceptance stays historical.
The task identifies the current version as awaiting an explicit new execution; it cannot present
old-version completion as acceptance of the new guidance.

## Current-source snapshot and candidate delta

Record the actual selected project/repository physical identity and selected relative working
folder, original branch, complete HEAD identity, index identity/bytes and dirty source. A manifest
entry records exact relative path, type, executable/file mode, byte size and SHA256/content artifact
for regular files; supported in-root symlinks record their exact target and link type. The source
snapshot's manifest hash identifies the actual source used, not merely HEAD.

Capture tracked current file bytes plus ordinary unignored source without invoking Git filters,
repository hooks or fsmonitor. Read index metadata with sanitized shell-free Git commands. Build
the managed detached workspace at the known base without a checkout/filter pass. Reconstruct its
meaningful original staged blob/mode state using a read-only cached binary diff with external
conversion disabled and index-only patch application, then populate exact current working bytes
through rooted filesystem operations. Verify worker/original staged and unstaged semantic parity;
archiving the old index alone is insufficient. Preserve original raw index/staging evidence too.
The worker's selected subfolder maps to the same relative subfolder in this isolated workspace.
Never copy Council state, Git-control files, vendor credential material or ignored authentication
state into it. Include an exclusion manifest with reasons; a required excluded/uncapturable source
blocks rather than silently disappearing. Unsafe paths, outside symlinks, split/sparse/unmerged
or intent-to-add indexes,
submodules, unborn repositories or concurrently changing source block specifically when this slice
cannot represent them safely. An old clean HEAD is never a dirty-source fallback.

Before/after source observations detect a racing user change while capturing; retrying the local
read is bounded and makes no model call, otherwise preparation blocks. No source/index mutation,
stash/reset, branch switch or original-project hook invocation is permitted. Later user edits to
the original checkout do not become part of an already captured attempt.

`SourceSnapshot` contains its immutable manifest, base/index/branch identities and physical project
observation. `Candidate` contains sourceSnapshotId, after-manifest/staged-index state, delta artifact and managed
workspace reference. Index-only changes are attributable changes too; they cannot make a
response-only attempt appear to have authored nothing. The delta is against the captured current-source manifest, including staged,
unstaged and ordinary untracked inputs, never a plain HEAD diff that includes the user's earlier
work. Candidate presentation records `verification: "pending"`, `integration: "not_performed"`.
Partial candidates from failed/cancelled work remain attributable and are not discarded.

## Evidence bytes, observations and events

Artifacts are owned immutable identities with `kind`, task/attempt link, private storage reference,
retained byte length, completeness and final SHA256. Append-only stream artifacts expose only the
known durable prefix; closed artifacts are immutable. Standard kinds are `prompt`, `stdout`,
`stderr`, `final_response`, `source_manifest`, `source_file`, `index_snapshot`, `candidate_manifest`
and `candidate_delta`. API callers use artifact IDs, never arbitrary filesystem read paths.

Raw stdout and stderr are retained as exact bytes, independently of JSONL interpretation. Bounds
are 64 MiB stdout, 16 MiB stderr and 1 MiB for one native JSONL line. A breach requests termination,
retains the exact obtained prefix and marks evidence incomplete/failed. Invalid protocol/Unicode
cannot be repaired by extracting a plausible JSON substring; retain raw bytes and the protocol
fault. A parsed final response is separately linked to its exact UTF-8 artifact/hash.

Artifact lookup verifies its owner-relative reference, physical file identity and known byte
extent, rejecting redirected/symlink leaves and missing/changed closed artifacts. Response
acceptance rechecks the exact closed response bytes against their stored SHA256; a stale cached
hash cannot accept a subsequently changed file. Record a corruption incident rather than serving
changed bytes under an old complete-artifact identity.

Append stream bytes to private storage and make them durable before committing the corresponding
known-prefix metadata/event. Never publish metadata claiming nonexistent bytes. An orphaned tail,
missing file, uncertain commit or failed flush is an evidence incident, not permission to replay.
Keep obtainable prefix/tail artifacts, stop conflicting admission and settle owned work. The
launch-intent record already prevents a restart from treating the attempt as never launched.

Attempt events have unique exact increasing sequence, kind, observation time and at most 16 KiB
of typed metadata, with artifact references rather than duplicated whole transcripts. Examples include
admitted, preparation_started, launch_intent, process_started, output_available, root_terminal,
exit_observed, stop_accepted, primary_failure, cleanup_verified, cleanup_unknown and settled.
They preserve serialized accepted control order; a provider wall-clock timestamp is not authority
for rewriting it. Event inspection and waits make no model calls.

Target observations distinguish requested model selector from reported effective model, provider,
account and native session ID. Each known fact carries source and observedAt; missing facts remain
unknown. Mapping full-access flags is adapter configuration evidence, not a claim that an OS or
provider restriction was overridden. Usage values are exact sourced counts or unknown; cost,
remaining allowance and external occupancy are unknown unless explicitly observed. No estimates,
reference-price engine, account alias pool or paid fallback is introduced here.

## Persistence records and admission protocol

Add the following bounded records to the existing schema; use foreign keys, exact integer checks
and one serialized writer, retaining the current receipt table/atomic replay behavior.

| Table | Key/index and purpose |
| --- | --- |
| `tasks` | Immutable admission sequence, id, project_id FK, revision, state_revision, current_version, latest_attempt_id and original/provenance data. |
| `task_versions` | `(task_id, version)` unique immutable complete snapshot, FK to task. |
| `attempts` | Immutable admission sequence/id, task/version FK, previous_attempt_id FK, phase/revision and current bounded observation data. |
| `attempt_terminals` | Unique attempt_id FK, append-once authoritative outcome/evidence summary. |
| `attempt_events` | Immutable exact sequence, attempt_id FK, kind/time/typed metadata; stable filtered paging. |
| `artifacts` | id, attempt_id FK, kind, owner-relative private reference, known durable length, closed/completeness/hash metadata. |
| `source_snapshots`, `candidates` | Stable IDs, attempt/source FKs and manifest/delta artifact links. |
| `response_acceptances` | Unique attempt/response acceptance, task/version FK and immutable user-review evidence. |
| `execution_control` | Singleton dispatch gate `open`, `closing` or `fenced`, with owner epoch and incident reference. |
| `capacity_claims` | Unique literal `codex-native` connection claim and unique attempt_id; held through verified cleanup. |
| `execution_incidents` | Stable unresolved ownership/evidence/persistence incident, affected attempt and known evidence. |

For direct start, task + version + queued attempt + admitted event + receipt are one transaction.
For an existing-task explicit start, expected task revision/current version + attempt + updated
latest pointer + event + receipt are one transaction. Receipt data represents durable admission,
not subsequent worker completion; a replay returns the exact original acknowledgement.

Dispatch reserves the single capacity claim and records preparation, source snapshot and durable
launch intent before invoking native work. Cancellation/version/gate checks occur again at the
launch boundary. No external spawn can be atomic with a database transaction: a crash after
launch intent is therefore potentially executed/interrupted even when no PID was recorded. A
known spawn failure can settle failed; an uncertain spawn cannot be retried automatically.

At owner restart, finalize unfinished ownership as interrupted without replacing accepted terminal
records, retain artifacts and fence dispatch if cleanup/ownership is unverified. Prior queued work
also does not auto-dispatch merely because the owner restarted; mark it interrupted/not-launched
with verified cleanup when that fact is durable. Full owner reconciliation/resume remains 007.
An orderly closed owner with all capacity released and no unresolved incident permits the new
owner to open admission for new explicit starts. A persisted closing marker alone does not leave
an otherwise verified clean service permanently fenced; unfinished old attempts are still not
automatically dispatched.

## Ownership and continuing service

UI and CLI contact the authenticated existing owner rather than opening another writer. A CLI
start with no owner establishes a persistent background service before admission; observer exit
cannot end the task. Self-spawn carries no task text, bearer token or vendor credential in argv.
The child publishes its private authenticated descriptor only after lifetime-lock acquisition and
schema validation. Concurrent launchers reconcile with the one owner; no PID takeover is allowed.

A compatible API-1 owner must explicitly advertise managed-execution capability before a client
sends an execution mutation. A 001-only owner is a clear capability blocker; it is not replaced or
silently killed. Explicit service stop closes admission, records control, settles owned processes
and evidence within the bounded effort, then removes only its matching descriptor. Stop grace is
2 seconds; by 15 seconds report verified settlement or an unresolved incident/fence. No automatic
artifact disposal, repair or live qualification occurs during shutdown/restart.
