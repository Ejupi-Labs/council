# Data model: Verified task delivery

**Feature**: `005-verified-delivery`
**Branch**: `refactor`
**Contract**: API 1 / execution `005-v1` / application `2.0.0-dev.5` / schema 5
**Status**: Design for independent review; no implementation or qualification claim.

This extends the accepted [004 model](../004-economical-routing/data-model.md). The
[specification](spec.md) defines authority and outcomes; the [delivery contract](contracts/workflow.md) defines public
mutations. Existing task/version, attempt/terminal, artifact, source/candidate, routing, capacity,
budget and exact receipt records remain authoritative. This is one finite task workflow, with
one shared owned-work slot. It introduces neither a task graph nor a deployment platform.

## Values, bounds and compatibility

Retain random IDs as 32 lowercase hexadecimal characters; SHA-256 values are 64 lowercase
hexadecimal characters. Revisions, sequences, ordinals and counts are canonical decimal strings
through signed-64 maximum `9223372036854775807`, with checked addition and no floating-point
conversion. Small bounded options are JSON integers. Times are UTC RFC 3339 milliseconds with
`Z`. Original task/source/output bytes retain Unicode, BOMs, line endings and exact contents.

New optional members are absent on historical records; absence is not evidence that an old
workflow ran a check or had delivery authority. Do not inject defaults into old request bodies,
pins, receipt fingerprints, stored JSON or replayed envelopes. Existing successful response-only
acceptance remains valid for its exact recorded scope. Candidate workflow acceptance is a
separate record and cannot reinterpret that historical acknowledgement as review or integration.
The response-only compatibility path below references the existing user acceptance directly.

A workflow captures at most 16 check definitions, at most one correction cycle and four native
model roles: author, initial review, correction and affected re-review. Unneeded roles are not
admitted. Checks can run before and after correction; each execution has its own receipt. The
logical pipeline is bounded, while explicit later operations and attempts remain separately
paged history, never an unbounded array in the workflow row. Each structured review has at most
64 findings and 64 explicit gaps. Preview metadata is bounded to 16 KiB; complete structured
review data is an immutable artifact bounded to 1 MiB. Definition/authority JSON is at most
64 KiB; operation and receipt metadata is at most 64 KiB. Large manifests, source, deltas and
output stay in artifact bodies, not events or authority rows. Events retain the existing
16 KiB limit. Collection reads use stable high-water cursors, limits 1–100 and whole-item
4 MiB response budgeting; full artifact reads use the existing bounded byte-range contract.

Unknown members, wrong JSON types, null required members, explicit empty IDs/enums/counts and
unsafe public numeric values are rejected. Optional absence is not an explicit empty value.
Existing intentional empty text/clearing rules are unchanged. Internal verification proofs are
typed controller inputs, never public JSON that a client or agent can supply as trusted facts.

## Existing entities and owned-stage tags

`Task.originalText`, draft provenance and immutable `TaskVersion` remain the instruction source.
Task user `revision` and lifecycle `stateRevision` stay distinct. A workflow captures one exact
task version and acceptance/condition digest. Guidance or an explicit authority change creates
the normal new version/control revision and makes old automatic advancement proofs stale.
It does not edit an old plan, candidate, review, terminal or receipt.

On a new 005 Start, omitted `workflow` resolves to `auto` only after exact raw-shape receipt
replay. Explicit `author_only` retains standalone authoring without automatic review/check/
delivery or qualified-candidate acceptance. Existing tasks without workflow authority remain
standalone until explicit workflow adoption appends a new version. Adoption admits no work and
grants no budget/window; later Start/advance is a separate captured request. Optional workflow
input can reduce maximum correction cycles to zero and require review, but cannot waive a
required condition or silently expand routing/model-budget authority.

New `Attempt` records may carry these optional members:

| Member | Meaning |
| --- | --- |
| `workflowId`, `stepId` | Exact owning workflow and finite stage execution. |
| `role` | `author`, `review`, `correction`, `check`, `local_delivery`, `external_delivery`. |
| `executorKind` | `native`, `local_check`, `local_application`, `local_external`. |
| `inputCandidateId` | Retained candidate actually materialized for the role; absent for the initial author. |
| `trigger` | `user_request` or `workflow_stage`, with the captured authority/predecessor reference. |

Absent tags describe a historical standalone author for compatibility only; reads do not write
that interpretation back. Native author/review/correction attempts use existing captured pins,
resolved targets, routing decisions, native protocol, usage and admission debits. Reviews use a
fresh review-role process/context without author-session reuse; the same pinned harness/model
is allowed. Correction starts from the selected candidate, not a recapture of unchanged original
project bytes. Source materialization verifies retained manifest/file/index evidence before use.

Local attempts use connection IDs `local-check`, `local-application` and `local-external`.
They have no resolved native target, native routing decision or model-admission debit. Existing
required target-observation members remain structurally present with unknown native facts;
the executor tag identifies their non-applicability. No provider/account/model/session/token
observation is invented. Their local executable/effect identity belongs to the check or delivery
record. Permissions describe actual local execution limits, not a universal OS sandbox.

All owned stages acquire the same existing `managed-native` claim, despite its historical name.
A busy claim queues other work; it is not route ineligibility or a 004 Category A trigger. Native
roles consume one model admission exactly once. Local check/application/external stages consume
zero model units, but share the current task window, Stop/owner/fence and cleanup constraints.
No local stage opens or renews the common task window. Its deadline is the earlier of the common
deadline and its captured bounded stage timeout. Observation and evidence reads admit no work.
Zero remaining model units alone does not prevent an already authorized local stage while the
common window is open and current controls permit it; it does prevent another native admission.

Settlement is executor-specific. Native roles require their supported input/protocol/output/root
and cleanup facts; command check/external roles require their observed local root/output/cleanup
and declared result contract. A built-in check records controller completion and exact basis
without an invented child/root exit. A local application has no process/root exit or native input
protocol to fabricate: its settlement binds the durable operation, known filesystem effects and
complete owned evidence, with explicit no-process cleanup. Unknown filesystem effects remain
delivery uncertainty independently of the absence of a process tree.

Workflow-native routing decisions retain the existing SQL `kind: "manual"`, denoting authority
from the explicit user-issued task rather than a per-stage click; `Attempt.trigger`/step authority
make workflow advancement explicit. `kind: "automatic"` remains reserved for accepted 004 A/B
continuations. Do not relabel workflow correction as A/B or rebuild the old enum-bearing tables.
Budget ledger `admission` entries and exact counters remain unchanged. Any 004 replacement is
additionally bound to the same logical workflow slot through a new step execution; it never
rebinds the old step or creates a competing next logical stage.

## Prospective delivery preference and captured scope

`DeliveryPreference` is `{localMode, selectionSource, revision, updatedAt}`, with modes
`review_only` and `apply_verified_local`, and selection sources `default` or `user`. Both upgrade and a fresh
installation initialize `review_only`; there is no inferred application consent. A single explicit
prospective preference selection may save local application for subsequent selected-project
tasks. It is independent of `Policy.mode` and never supplies commit/publication authority.

`DeliveryScope` captures `{localMode, preferenceRevision, source, projectId, projectIdentity,
repositoryIdentity, selectedRelativePath, branch, destination, externalAction}`. `source` is
`saved_preference` or `task_scope`; the default review-only case is recorded as `default`.
`destination` identifies the selected project's working destination, not a free agent-supplied
path. Physical identities are owner observations, not trusted client strings. A workflow may
contain at most one optional external/commit action, which must name its captured action and
destination. Applying a verified local delta
authorizes none of commit, push, release or deployment. Peer, project and worker text creates
no such scope. Unsupported declared actions are retained as explicit gaps and are not launched.

The task snapshots prospective delivery consent at admission independently of its captured
permission policy. A historical draft's policy snapshot remains authoritative for permission;
it cannot fabricate a delivery selection it never captured. A new task's explicit delivery input
or current explicitly saved preference supplies that separate snapshot. Receipt replay precedes
reading the current preference, resolving the destination or inspecting current filesystem state.

Fresh, positively identified empty stores initialize the factory model-admission default to four.
Every existing store preserves its saved finite routing default, including a value of two that
looks like an older factory default. Old task budgets/counts/windows and explicit finite task
budgets prevail. No migration grants a correction cycle, local application, review approval,
automatic resume, extra model units or a new common window.

## Workflow authority, steps and current projection

`WorkflowAuthority` is immutable and contains `id`, `taskId`, `taskVersion`, `capturedAt`,
`createdByRequestId`, `instructionSHA256`, `conditionsSHA256`, `routingAuthorityId`, captured
delivery scope, check definitions, stage-selection reasons and the finite correction policy.
The correction policy is `{maxCycles: 0|1, triggers: [...]}`, defaulting to one for a new Auto workflow. Supported
triggers are `blocking_review_finding` and `failed_required_check`. A trigger is evidence under
already captured authority, not a grant. Ordinary native failure, malformed protocol, missing
evidence, storage faults, uncertain cleanup and prose are not correction triggers. Explicit
requirements/pins/budget may prevent an otherwise necessary stage, producing a gap.

`Workflow` exposes that immutable authority and a bounded current projection: `revision`,
`stateRevision`, `state`, `selectedCandidateId`, `selectedResponseBinding`, `currentStepId`,
`nextAction`, `nextOwner`, `correctionCyclesUsed`, `latestAcceptanceId`, optional write-once
response `completion`, `createdAt`, `updatedAt`. States are `queued`,
`running`, `blocked`, `cancelling`, `cancelled`, `failed`, `interrupted`, `completed`. The owner
of the next action is `council` or `user`, accompanied by a concrete reason/action code. A task
is not completed merely because its latest author, check or reviewer exited successfully.

`WorkflowStep` is one step execution. It contains `id` (the step-execution ID), `workflowId`,
monotone `ordinal`, positive per-slot `executionOrdinal`, `replacementOfStepId`, `role`, `cycle` (0 or 1),
`logicalKey`, `reasonCodes`, captured definition/authority reference, `inputCandidateId`,
`outputCandidateId`, optional `inputResponseBinding`/`outputResponseBinding`,
`predecessorStepId`, `attemptId`, `status`, timestamps and optional terminal
advancement proof. Once assigned, its attempt, input candidate and captured definition cannot
be rebound. Statuses are `pending`, `ready`, `active`, `succeeded`, `failed`, `cancelled`,
`blocked`, `interrupted`, `not_required`. Terminal outcomes are append-once. `not_required`
has a recorded proportionality reason and is not a passed check/review. A later relevance change
is projected as historical applicability without rewriting the earlier terminal.

A logical slot is identified by `workflowId` plus `logicalKey`; its role, cycle, captured
definition and input candidate/response binding are immutable. Enabled 004 Category A replacement creates a
**new** step-execution ID and new Attempt with that same slot binding, increments the execution
ordinal and references the blocked execution through `replacementOfStepId`. The replacement
Attempt's `previousAttemptId` names the blocked Attempt. New step/attempt binding, consumed
004 proof, model debit and routing decision `kind: "automatic"` commit atomically. The old
blocked step/Attempt remains terminal and owns no 005 advancement proof.

There is one current logical slot and at most one current execution for it. Atomic uniqueness
and current workflow/control comparisons prevent an A replacement, a manual retry and 005
advancement from each admitting a competing successor. After a fresh run fully returns through
deferred cleanup, the owner tries permitted 004 replacement first and returns if one was admitted;
it cannot also advance 005 from that blocked execution. No historical terminal/restart scan
performs either operation. A fresh explicit advance may retry the current blocked slot with a
new execution ID under current budget/controls and captured-user `manual` admission; it never
rebinds an execution or re-arms an old proof.
Currentness lives in the workflow's `currentStepId` projection, not in a rewritten terminal step;
switching to a replacement cannot change the blocked execution's terminal JSON/status.

`WorkflowAdvanceProof` binds the settled predecessor, current owner instance, task user revision,
task version, workflow control revision, budget revision/window, selected candidate or exact
selected response binding and next logical step. Only the final qualifying execution of the current logical slot can produce it:
ordinary forward progression requires successful settlement, while the closed captured correction
triggers additionally permit a valid blocking review or observed failed required check with
complete known settlement. A pre-launch blocked execution awaiting/replaced through 004 A
produces no such proof. It is recorded with the qualifying terminal fact and consumed at most
once, only by the still-current owner after all predecessor output/effect cleanup has returned. There is no restart,
old-terminal, configuration-change or report-change scan that starts stages. Accepted Stop or
an explicit later guidance/grant/control mutation makes the older proof stale. A new explicit
request can authorize a compatible next operation, with its own receipt and current controls;
it does not revive the old proof. This retains the established Stop-before-next-admission ordering.

Workflow get/list reads accept mutually exclusive `workflowId` or `taskVersion` selectors;
omission selects the current version. The selected workflow must belong to the addressed task.
Authority, step executions, findings/resolutions, checks, candidates and delivery history remain
readable after guidance/adoption, with null workflow for a selected standalone version. Cursors
bind resource/task/resolved selector/high-water, so a cursor from an older selection cannot
silently switch to current state. Mutations accept only the current task/version/workflow/control
binding; historical selectors never grant authority or permit historical mutation.

## Candidate revision and applicability

Reuse `Candidate`, its source snapshot, full manifest, delta and index evidence. New candidates
add optional `workflowId`, `predecessorCandidateId`, `createdByStepId`, `conditionsSHA256` and
`completeness` (`complete`, `partial`, `uncertain`). The initial candidate has no predecessor;
correction or a source-modifying check records a new revision with the exact predecessor. A
live workspace path alone never identifies the accepted bytes. Existing candidate `verification`
and `integration` values are historical creation facts; qualified current results come from the
workflow receipts/projection, without editing old candidate JSON.

`CandidateBinding` identifies `{candidateId, sourceSnapshotId, manifestArtifactId,
manifestSHA256, indexArtifactId, indexSHA256, conditionsSHA256}`. Complete source/index bytes
and project/control identity must be available. A partial or uncertain candidate can remain useful
but cannot satisfy acceptance or local integration. A fresh controller verification proves
registered artifact identity, length and hash before review/check materialization or delivery.

`Applicability` captures `{scopeKind, paths, conditionKeys, definitionSHA256, basisSHA256}`.
Default scope is `whole_candidate`; `declared_subset` is permitted only when the check/review
definition captured that sufficient scope before execution. The basis digest covers the exact
in-scope bytes/types/modes/index entries, required conditions, check/review definition and relevant
source/control prerequisites. It is separate from the candidate occurrence ID. Evidence still
names the candidate actually inspected; reuse against a corrected candidate needs a new typed
comparison establishing an identical recorded applicability basis. No retroactive scope narrowing
or generic `resolved` label makes stale evidence applicable. An unknown affected set requires
the affected stage to run again. Source/condition changes never change a prior receipt's outcome.

### Response-only compatibility and required evidence

`ResponseBinding` is `{producerAttemptId, responseArtifactId, responseSHA256, responseByteLength,
sourceSnapshotId, manifestSHA256, indexArtifactId, indexSHA256, conditionsSHA256}`. The producer
is a settled author or correction, not a later reviewer/check. It identifies the complete exact
final response and unchanged source/index basis; response-only eligibility proves both no
authored worktree delta and no index delta. A retained empty-delta candidate remains inspectable
as history but is not promoted to a qualified candidate, applied change or integration result.
For a response-only workflow, `selectedResponseBinding` is authoritative and
`selectedCandidateId` is null. Response text/hash, as well as source/index/conditions, participates
in review/check applicability: corrected text makes old response review historical even when the
source files are unchanged. Review/check steps materialize this exact response and required
unchanged source/evidence under their actual access contract. A changed materialized response
copy cannot silently replace the registered selected response.

For Auto `response_only` with no worktree/index delta and no explicit required check/review,
external action or other captured evidence requirement, do not add review/check model work
because the task wording is unknown to the direct document-edit recognizer. Pause at the
existing `response_acceptance_pending` action with `nextOwner: "user"`. The user inspects and
accepts the exact current response through the existing `tasks.response.accept` route, IDs,
confirmation and fresh artifact verification. This is the existing product acknowledgement,
not a new workflow approval. Success records the unchanged `ResponseAcceptance` and a write-once
workflow completion `{kind: "response_only_user_review", responseAcceptanceId, responseBinding}`.
It creates no `WorkflowAcceptance` row, local application or publication claim.

If explicit response-only evidence requirements exist, the captured compatible review/check/
external stages must qualify that exact response binding first. Missing/unsupported evidence
remains a gap. An empty `requiredEvidence` array does not waive explicitly captured workflow
checks, required review or external scope. `local_integration` without an actual deliverable
delta is an unsupported/contradictory gap; a saved `apply_verified_local` preference alone
creates no no-op application. A scoped external action proves only its declared command-completion
basis against the verified response/source; stronger postconditions are still unsupported.

After required evidence and known settlement qualify, the same existing user acceptance route
performs the single acknowledgement. Only a current 005 workflow branch may bind the selected
producer response instead of demanding that its producer be the latest supporting-stage Attempt,
and only that branch may admit acceptance after all formerly outstanding requirements are
actually satisfied. It atomically records the existing-format user acceptance plus write-once
completion `{kind: "response_only_user_review_with_evidence", responseAcceptanceId,
responseBinding, reviewUses, checkUses, findingResolutionIds, externalResultRefs}`. These immutable
uses record the conjunction; no candidate `WorkflowAcceptance` row is fabricated. Legacy
standalone guards, response acceptance JSON and raw-shape receipt replay remain unchanged.

Human acceptance may occur after the execution window expires when the exact result/version is
still current and eligible: this non-execution mutation admits no model/local effects and opens
no window. It does not waive Stop/failure, incomplete evidence, changed response, unresolved
requirements or ownership uncertainty. Exact receipt replay does not mutate a newer workflow
or new guidance version. Candidate workflows continue to use `WorkflowAcceptance` below.
`latestAcceptanceId` remains the candidate workflow-acceptance reference; response completion
names its `responseAcceptanceId` explicitly, avoiding an ambiguous cross-table ID. A response-only
check that changes required source/index retains the changed-state evidence and blocks affected
qualification; it cannot convert that side effect into an accepted response-only deliverable.

## Structured review, findings and correction

`ReviewRecord` contains `id`, `workflowId`, `stepId`, `attemptId`, `candidateBinding` or `responseBinding`,
`reviewContract`, `resultArtifactId`, `resultSHA256`, `applicability`, `inspectedEvidence`,
`findings`, `gaps`, `outcome`, `createdAt`. Outcomes are `approved`, `changes_required`,
`incomplete`, `invalid`. The complete structured result is strict validated data, not a JSON
substring extracted from ordinary prose. Root/native/drain/cleanup settlement remains separate
from whether its contents qualify review. The raw response remains available for an invalid result.
Its fixed native result schema is `schemaVersion: "005-review-v1"`, with exact candidate-or-response/condition
binding, supplied inspected references, review-local finding keys, explicit gaps and prior finding
resolution references. Whole-result/native-envelope limits both apply; no truncation repairs an
oversized result. A native resolved-finding list is evidence to validate, not resolution authority.

`inspectedEvidence` identifies the candidate/source/check/artifact references actually supplied
under the review role's access contract, the reported inspected scope and any recorded access
limitations. Supplied access is not invented proof of an individual model tool read. A required
file/check not inspected is an explicit gap; a completed response cannot approve it. Review
does not claim universal read-only enforcement where the harness cannot provide it. Changes
to the inspected workspace are detected against the retained binding and invalidate affected
review applicability rather than silently becoming author work.

A finding is `{id, modelKey, severity, conditionKeys, locations, summary, evidenceRefs}`. The
owner assigns `id`; the bounded review-local `modelKey` identifies the native structured entry
without allowing a model to assign/reuse arbitrary persisted IDs. Severity is
`blocking` or `advisory`. Locations name bounded relative paths/ranges or a condition; references
must resolve within this workflow's supplied evidence. Findings have stable immutable identity.
A required-check failure can create an equivalent system finding linked to its observed receipt.
Ordinary operational failures do not become review/check blockers eligible for correction.

`FindingResolution` contains `id`, `workflowId`, `findingId`, `originReviewId` or `originCheckId`,
`correctedCandidateId` or exact `correctedResponseBinding`, `correctionStepId`, `qualifyingReviewIds`, `qualifyingCheckIds`, exact
applicability-use records, `createdAt`. The controller records resolution only after the affected
required checks/review qualify the corrected candidate. Reviewer/author claims alone do not
resolve a blocker. Earlier finding, failed check and author artifacts stay unchanged. Failure of
the one captured cycle or insufficient budget leaves the task unresolved with its next action.

## Deterministic check definition and observed receipt

`CheckDefinition` captures `id`, `kind` (`builtin` or `command`), `source`
(`recognized_convention` or `task_declaration`), `recognizerId`, `builtinId`, `executable`,
`arguments`, `workingDirectory`, `successExitCodes`, `scope`, `timeoutSeconds`, `required`,
`coveredConditionKeys`, `definitionSHA256` and declaration evidence references. The supported
built-in identity is `literal_document_replace/1`: it proves the entire exact expected delta and
preserved constraints/index for the recognized bounded operation, not a routine hint. It has no
executable/argv/process result. Command definitions have no built-in identity and capture a
nonempty bounded success exit-code set, default `[0]`. Both own a `local_check` attempt.
The definition comes from a supported sufficient convention or explicit captured task scope;
commands found only in generated worker/reviewer/project text have no execution authority.
The executable is resolved outside SQL to an explicit native path; arguments are an array, with
no intervening shell. The working directory is inside the materialized managed candidate.
No vendor credentials, secret environment values or native global configuration are copied into
the record. A command can have at most 64 arguments, each at most 4096 UTF-8 bytes, with a
16 KiB aggregate definition argv bound; unsupported or contradictory definitions become gaps.

`CheckReceipt` contains `id`, `workflowId`, `stepId`, `attemptId`, `definitionId`,
`definitionSHA256`, `inputCandidateBinding` or `inputResponseBinding`, `applicability`, `observedExecutable`,
`startedAt`, `finishedAt`, `outcome`, `exitCode`, `stdoutArtifactId`, `stderrArtifactId`,
`rootTerminal`, `inputComplete`, `outputComplete`, `builtinCompleted`, `cleanup`,
`resultingCandidateId`, `sourceChanged`, `createdAt`. Outcomes are
`passed`, `failed`, `cancelled`, `unavailable`, `not_run`, `interrupted`. Historical/stale is a
separate applicability projection, not a replacement terminal outcome. Launch failure is
`unavailable`; no observed execution is `not_run`; neither is a passed check. A positive receipt
requires the definition-specific observed completion, complete durable evidence and known owned
cleanup. A command needs observed root completion and the captured exit criterion; a built-in
needs its controller's exact-byte proof and `builtinCompleted`, with no exit code/native root
fact or invented stdout/stderr artifacts. Fields requiring a process are inapplicable to that
built-in, while complete proof evidence is still required. Agent claims are retained as claims
and never used as observed receipts.

Checks receive exact materialized candidate bytes and indexed state. A check that changes
required source/index retains a new candidate revision and invalidates affected prior evidence.
Its successful exit alone does not qualify a later different state; relevant verification runs
against the resulting candidate. Output is captured through existing owned streams with full
bounded reads, including partial output on failure/Stop and sticky durability uncertainty.

## Delivery operation and append-only effects

`DeliveryOperation` is linked 1:1 to a `local_application` attempt and contains `id`, `workflowId`,
`stepId`, `attemptId`, `requestId`, `candidateBinding`, accepted evidence references, captured
delivery scope/authority digest, `status`, `beforeDestination`, `afterDestination`, timestamps,
`lastEffectSequence`, bounded `conflicts` preview, exact `conflictCount`, `incidentId`, and the precise remaining action. Statuses are
`preparing`, `ready`, `applying`, `applied`, `conflict`, `partial`, `cancelled`, `interrupted`,
`uncertain`. Review-only delivery records a qualified retained candidate/remaining apply action;
it creates no filesystem application operation. Local application has no native process, but
its unsettled attempt owns any newly retained destination/conflict/recovery artifacts.

Destination evidence identifies the selected project/repository physical identity, captured
branch/HEAD and selected subfolder and closed manifest references for affected paths and expected
before/after content/type/mode and relevant index entries. Expected bytes refer to complete
retained artifacts. Add artifact kinds `destination_manifest` and `destination_file` for these
inventories and retained destination bytes; each must belong to this still-unsettled application
attempt. The complete conflicting-path list is in the manifest, with at most 64 inline preview
entries within the operation's 64 KiB bound. Fresh conflicting
destination bytes are retained under the application attempt when safely readable; if evidence
cannot be captured, retain the live user path and an explicit inaccessible/uncertain evidence gap.
There is no claim that a preview or old path name retains an unreadable user's current bytes.

`DeliveryEffect` is an append-only observation with `{operationId, sequence, effectId, kind,
relativePath, action, state, beforeEvidence, intendedAfterEvidence, observedAfterEvidence, observedAt,
incidentId}`. Kinds are the supported filesystem effects selected in the plan; state is `intent`,
`applied`, `conflict`, `failed`, `uncertain`. An intent is durably committed before that effect,
and each observed result is appended under the same effect ID. Existing observations are never
rewritten. The controller performs filesystem I/O outside SQL and supplies typed results after
fresh identity/path/content verification. No filesystem callback runs under a transaction.

### Supported application algorithm — resolved design decision

The 005 application scope is text, ordinary single-link regular-file add/replace/delete under
existing ordinary parents. Text is valid UTF-8 without NUL bytes; BOMs, replacement characters
and line endings remain exact. Existing workspace per-file/aggregate limits still apply. No
directory creation/deletion, index-only change, mode/type change, symlink/reparse path, unsafe
parent, private/excluded path, unsupported stream or unpreservable security metadata is applied.
The complete candidate is retained with `application_unsupported` and the precise manual action.

1. **Preflight and retained basis.** Under the owned application stage, freshly verify its exact
   candidate/evidence/conditions, captured scope, current controls/window, selected project/repo/
   branch/control identity and all affected path/type/content/index entries. Compare against the
   captured source, including pre-existing dirty bytes. Unrelated files/index entries are not
   rewritten. Record complete before/after destination manifests, expected contents and metadata
   as immutable artifacts. A known conflict at this point causes no destination move.
2. **Private same-filesystem preparation.** Stage intended after files and a unique quarantine
   under the existing private runtime state root, with identity-checked owned directories/files.
   This must be the same mounted filesystem as the affected destination. Cross-volume work is
   unsupported; do not fall back to copy/delete, create a project temp namespace, alter `.gitignore`
   or open/migrate a legacy project `.council` directory. Retain staged identity, actual metadata,
   exact hashes/lengths and quarantine references in the private operation record. No untrusted
   stored path supplies authority to open an arbitrary location after restart.
3. **Metadata eligibility.** Before any destination effect, establish that publication preserves
   supported basic mode, owner and security semantics; for new files capture the supported
   destination metadata rule. Moving a staged Windows file does not acquire the prior target's
   inherited ACL. A publisher that cannot preserve the applicable owner/DACL or unsupported
   alternate streams must reject the operation, not substitute Council-private ACLs, normalize
   the user target or silently lose metadata. The same rule covers unsupported Unix ACL/extended
   metadata. Native qualification must prove both the supported case and the named refusal.
4. **Write-ahead path actions.** For each affected path in captured stable order, recheck current
   Stop/owner/control/deadline and the anchored parent/leaf basis. Append and commit a per-path
   intent before its next native move. `kind` is `add`, `replace` or `delete`; `action` distinguishes
   `quarantine_before` and `publish_after`. Persist the observed quarantine result before any
   publication intent. Each intent includes exact expected source identity, private target and
   destination absence/content basis, not just a human-readable path.
   The control boundary durably permits exactly one in-flight action by committing its intent
   with current role/owner/revision/Stop guards, then releases `Engine.control` before native
   filesystem I/O. A blocked native move must not hold that control lock or prevent Stop
   acknowledgement. Accepted Stop prevents every new permit; an action already permitted or
   in flight may settle afterward and must record its actual partial/uncertain result without
   advancing, permitting the separate `publish_after`, or accepting the workflow. Publication
   requires a fresh guard and separate permit. Ownership/claim remain held until in-flight work
   settles; timeout cannot create another worker or replay. Preserve the accepted cancellation
   and earlier primary failure ordering. Qualification uses barriers for Stop before permit,
   during the native action and after quarantine, proving responsive acknowledgement and no
   subsequent publication. This is request-based Stop, not instant filesystem rollback.
5. **Exclusive move, never overwrite.** Add uses a no-replace move of its staged after file into
   the expected absent destination. Replace first no-replace-moves the actual existing file into
   its unique private quarantine, then freshly verifies the moved identity/bytes/type against
   the expected before basis. Only a match permits a separately journaled no-replace publication
   of the after file into the now-absent destination. Delete performs only the quarantine move
   and moved-object verification. There is no in-place truncate, overwriting rename, hard-link
   trick or unguarded `Remove` of a user's live destination.
6. **Race and partial outcomes.** Replacement deliberately permits a brief missing-name interval.
   A concurrent recreation makes no-replace publication fail and leaves that new user path
   intact. A last-moment source change discovered after quarantine is an explicit partial/conflict
   result; retain the actual displaced object and fresh readable conflict bytes. Stop between
   moves likewise retains the quarantined original and intended after bytes. Do not silently
   restore over a recreated path or claim that the operation had no effects. Earlier preflight
   conflicts and later partial conflicts remain distinguishable.
7. **Observed completion.** Sync staged/published files and directory metadata where the platform
   provides a meaningful supported durability operation; inspect actual resulting identity/content/
   metadata and unchanged required index basis. Append the exact observed path result, then
   advance only after its durable acknowledgement and all required path results. Unsupported
   namespace durability is explicit; neither a rename nor SQLite alone is a universal power-loss
   transaction across filesystem and database. Any ambiguous move/sync/commit acknowledgement
   is uncertain, fences unsafe progress and permits no automatic effect retry.

Use native exclusive primitives anchored to retained checked objects/directories: Windows
`SetFileInformationByHandle(FileRenameInfo)` with `ReplaceIfExists=false`, a checked source
handle and relative name under the verified destination `RootDirectory`; Linux
`renameat2(..., RENAME_NOREPLACE)` and macOS `renameatx_np(..., RENAME_EXCL)` with retained
directory descriptors. Path-only rename is not an anchored fallback when a redirect race cannot
be excluded. Exclusive rename support depends on the filesystem; no unsafe fallback is allowed.
The Windows handle-relative/no-replace contract is documented in
[FILE_RENAME_INFO](https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_rename_info).
The additional same-volume/no-copy/no-replace boundaries are documented by
[Microsoft](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw),
[Linux](https://man7.org/linux/man-pages/man2/rename.2.html) and
[Apple](https://developer.apple.com/documentation/foundation/urlresourcevalues/volumesupportsexclusiverenaming).

Displaced originals remain live quarantined objects, with immutable before/after snapshots as
observed at their recorded times. Existing external handles can continue to refer to a moved
inode; snapshots do not claim to freeze those handles or preserve all future writes as an
immutable past observation. Retain quarantine conservatively, with no automatic cleanup or
rollback in 005. Later deliberate retention/recovery is 007. Its private physical identity and
location remain inspectable by the owner without exporting an executable client path.

Only supported task deltas can be applied, preserving unrelated dirty/untracked/staged state.
Branch/identity/affected-path/index conflicts are detected before supported writes. This is not
a universal atomic multi-file transaction or a lock against arbitrary external editors. Every
supported write must use the platform's documented ownership/revalidation boundary; if safety
cannot be established it is a conflict/unsupported gap, not consent to overwrite. Partial effects
remain explicit, preserving candidate/before evidence and remaining user work. No blind reset,
stash or rollback overwrites edits made after the recorded before state.

Once any effect might have occurred, missing commit acknowledgement is `uncertain`, never
known rollback. The owner fences conflicting automatic work and retains the claim as needed for
unknown owned work/effects. Restart performs no effect replay. Read-only reconciliation can
append fresh observed state under explicit reconciliation authority; it does not rewrite an old
terminal/receipt, automatically accept a task or silently run the next effect. Deliberate write
recovery outside these bounded delivery rules remains 007 work.

`ExternalAction` captures an explicit action/destination, native executable, static arguments,
working context and `acceptanceBasis: "command_completion"`. Arguments cannot be supplied by
worker/peer output or project text. This slice does not attest a remote deployment/publication
state. `ExternalDeliveryRecord` uses its own `local_external` attempt/step and records that exact
definition, full output, observed root/cleanup outcome and acknowledgement evidence. Outcomes
are `command_succeeded`, `command_failed`, `cancelled`, `unavailable`, `uncertain`. An interrupted
underlying attempt keeps that terminal; unknown external effects project as `uncertain`.
It can be stored as the immutable terminal data of that workflow step, avoiding a separate table.
The existing attempt terminal remains authoritative. Unknown acknowledgement stays unknown,
with no automatic retry. A successful local command proves only the captured command-completion
basis, never that the remote destination is published/deployed. A task requiring stronger remote
attestation has an explicit unsupported evidence gap instead of a fabricated publication result.

## Acceptance and derived task state

`WorkflowAcceptance` contains `id`, `workflowId`, `taskId`, `taskVersion`, `candidateBinding`,
`authoritySHA256`, `reviewUses`, `checkUses`, `findingResolutionIds`, `deliveryResult`,
`acceptedAt`, `acceptedByRequestId`. Each evidence use names its immutable record, the original
inspected candidate and the freshly established applicability to the selected candidate. Store
validation checks exact task/version/control, registered evidence ownership and digests, all
required conditions, no unresolved blocking findings, known stage settlement, delivery policy
and all current fences. Controller filesystem/process proofs are reverified outside SQL before
the typed atomic acceptance mutation; clients/agents cannot provide trusted proof fields.

For `review_only`, completion means the requested qualified retained change was delivered with
the exact remaining application action; it never means integrated. For local-application scope,
acceptance additionally requires the exact supported operation's known `applied` result and
matching destination evidence. Scoped external actions have separate qualifying outcomes and
cannot be inferred from local application. Earlier failures remain in history even if a later
correction satisfies the task. Stop or authoritative failure on an attempt stays unchanged;
late success cannot launch integration or complete that stopped workflow.

## Schema 5 additions and indexes

Existing schema1–4 tables and enum constraints are not rebuilt. New tables use the existing
checked-integer constraints, foreign keys, JSON/column agreement checks and exact DDL validation.
The nine additions are present needs, with no generic DAG, inbox, session or framework table:

| Table | Indexed identity and purpose |
| --- | --- |
| `delivery_preferences` | Singleton `1`, positive revision, preference JSON. |
| `workflows` | ID primary key; task/version foreign key and unique pair; positive control/state revisions, state, immutable authority JSON and mutable projection JSON. |
| `workflow_steps` | Execution ID primary key; workflow foreign key; positive ordinal unique per workflow; positive execution ordinal unique within `(workflow_id, logical_key)`; role/status checks; attempt foreign key unique when bound; input/output candidate references; predecessor/replacement references; unique replacement/advancement consumption keys when present. Index `(workflow_id, ordinal)` plus one-current-execution constraint for the current logical slot. |
| `workflow_reviews` | ID primary key; workflow/step/attempt foreign keys; exactly one candidate FK or response-artifact FK with checked response binding; attempt unique; immutable validated result/artifact bindings. Index `(workflow_id, id)`. Findings remain bounded within immutable review JSON. |
| `workflow_checks` | ID primary key; workflow/step/attempt foreign keys; exactly one candidate FK or response-artifact FK with checked response binding; attempt unique; immutable definition/application/result binding. Index `(workflow_id, id)`. |
| `workflow_resolutions` | ID primary key; workflow and corrected-candidate/response-artifact foreign keys; origin review/check reference and exact stable finding ID; qualifying evidence JSON. Unique `(workflow_id, finding_id, corrected_subject_sha256)`, with subject kind/binding validated. |
| `workflow_acceptances` | ID primary key; workflow/task/version/candidate foreign keys; immutable evidence uses and delivery result. Index `(task_id, task_version)`. |
| `delivery_operations` | ID primary key; workflow/step/attempt/candidate foreign keys; attempt unique; status and current effect projection; immutable captured authority/binding. Index `(workflow_id, id)`. |
| `delivery_effects` | Primary key `(operation_id, sequence)`; operation foreign key; positive checked sequence; effect ID/path/state and immutable observation JSON. Index `(operation_id, effect_id, sequence)`. |

Cross-reference validation additionally requires matching workflow/task/version, candidate lineage,
step role/executor/definition and actual registered artifact kind/ownership/completeness. Same
random ID format is not a substitute for that validation. An effect cannot be reassigned to
another operation/path; a step cannot acquire a different attempt; the single shared claim and
terminal uniqueness remain enforced. Workflow summaries are bounded point-read projections,
not an unbounded scan of every past attempt or evidence body.

## Migration, mutation and failure boundaries

1. Acquire the existing native owner lock; apply private-file/companion identity guards before
   SQLite opens state. Read application ID/version, validate the exact accepted schema/foreign
   keys and refuse unknown objects, colliding names or malformed current rows without repair.
2. Bring accepted older versions through their existing migrations; then, in one schema4→5
   transaction, create only the nine new objects/indexes, insert review-only delivery preference
   and set `user_version=5`. Preserve every old row JSON, receipt envelope/status/fingerprint,
   observation, active claim, gate/incident and budget byte/count. A genuinely empty initialization
   selects fresh factory four before it creates its routing default; upgrades never patch that row.
3. Validate exact schema5 DDL, required singleton, foreign keys and new row/enum invariants.
   Preserve WAL/FULL, per-connection foreign keys/FULL/busy timeout and protected retained
   companions. No mode fallback, reset, journal deletion or legacy-state migration is introduced.
4. Known pre-commit cancellation/failure rolls back with no new stage/debit/authority. An actual
   Commit/AfterCommit error remains `write_uncertain`, including when cancellation also exists.
   Before/after-commit migration fault tests must distinguish schema4 rollback from uncertain
   committed schema5 and verify exact old history after reopen.

Every public mutation uses the existing exact request receipt/fingerprint mechanism before fresh
policy/filesystem/version prerequisites. A transaction admits a model attempt, routing decision,
budget debit and step binding atomically; local attempts bind a step without inventing a debit.
Progress/fault/terminal/proof and derived task state update coherently. New effect intent and
acceptance mutations atomically compare current task/workflow/budget/owner controls and required
registered immutable facts. Neither a model call nor process/filesystem/vendor I/O occurs inside
SQL. Persistence uncertainty latches the existing owner fence; retries with the same identity can
replay known receipts, and absent acknowledgements never authorize new execution/effects.
