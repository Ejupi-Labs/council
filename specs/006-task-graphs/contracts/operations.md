# Graph operation and response shapes

Status: proposed 006 design, not implemented or qualified. This is the normative field contract
for the operations linked from [graphs.md](graphs.md). Storage/control invariants remain in
[data-model.md](../data-model.md); complete input, output and materialization bindings remain in
[materialization.md](materialization.md). API version remains 1.

## Common representation

IDs are existing opaque 32-hex strings. Revisions, versions, epochs, counts, window numbers,
ordinals and byte lengths are canonical decimal strings within the existing checked int64 domain.
Positive revisions/versions start at `"1"`. `maxParallelism` is `"1"` or `"2"`. Times are UTC strings.
Existing bounded timeout fields remain safe JSON integers; no public unsafe numeric cursor is
accepted. All request objects are strict: unknown members, null required members and explicit
empty ID/enum/count/time strings reject. An allowed empty collection/string or false is preserved.

Every mutation uses the existing envelope and a stable `requestId`. Fingerprints include the
operation, path identity and complete typed input without injected defaults. Exact replay precedes
current policy, owner-control, filesystem, budget and availability prerequisites. A same-ID
different-input conflict creates no new work. Controller facts and artifact verification proofs
are never client request fields.

All response fields below are required unless explicitly marked optional. A nullable field is
present with JSON `null` when absent. Empty lists encode `[]`, never null. Private paths, physical
identity strings, handles and executable environment are not exposed in these projections.

`GraphNodeAuthority` is `{nodeId, taskVersion, nodeControlRevision}`. A declaration's complete
authority vector contains every node exactly once in declared order. Layout is immutable in 006;
node IDs cannot be replaced by display keys or a latest-task guess.

`GraphRef` is `{graphId,nodeId:null|string,rootTaskId,kind:"root"|"node"}`. Current graph-owned
`Task`/`TaskSummary` responses have additive optional `graphRef`; it is required present for a
known graph member and omitted for standalone tasks. Root kind requires `nodeId:null`; node kind
requires its exact stored ID. The owner derives it from membership, not request fields, and does
not rewrite historical task/receipt JSON. Root and pending no-workflow node surfaces show their
graph scope and suppress standalone Run/workflow-adoption/budget-grant actions. Server guards
still resolve membership on every operation, regardless of client view or absent graph fields.
Graph-scope errors such as `graph_input_stale` may carry the same additive optional `error.graphRef`
as their actionable owner reference; it cannot grant scope or change the request's captured target.

## Mutation requests

| DTO / route | Required fields | Optional fields |
| --- | --- | --- |
| `GraphAdvanceInput`, `POST /graphs/ID/advance` | `requestId`, `graphVersion`, `expectedGraphControlRevision`, `expectedBudgetAuthorityEpoch`, `expectedWindowNumber`, `nodeAuthorities` | `rebindNodeIds` (array of node IDs; omitted means empty). |
| `GraphStopInput`, `POST /graphs/ID/stop` | `requestId`, `expectedGraphControlRevision` | None. |
| `GraphNodeStopInput`, `POST /graphs/ID/nodes/KEY/stop` | `requestId`, `expectedGraphControlRevision`, `nodeId`, `expectedNodeControlRevision` | None. |
| `GraphGuidanceInput`, `POST /graphs/ID/nodes/KEY/guidance` | `requestId`, `expectedGraphControlRevision`, `nodeId`, `expectedNodeControlRevision`, `taskVersion`, existing `expectedRevision` and `guidanceText` | Existing `useCurrentPolicy`, `policyOverride`, `clearPolicyOverride`, `pins`, `acceptance`, `timeoutSeconds`, restricted `routing` and `workflow`. |
| `GraphBudgetAmendmentInput`, `POST /graphs/ID/budget-amendments` | `requestId`, `expectedGraphControlRevision`, `expectedBudgetAuthorityEpoch`, `expectedWindowNumber`, `additionalAdmissions` | `newWindowSeconds`. |

### Advance and live capacity waiting

Advance captures current explicit execution intent. `nodeAuthorities` is the complete bounded
vector read by the caller. Before each selected admission/preparation, compare the selected node
and its transitive dependency controls/versions. An unrelated sibling's accounting/state revision
is not a guard. A relevant mismatch returns conflict before this mutation performs effects or
admissions; the server never substitutes a newer version. Irrelevant vector entries grant no
authority for versions different from their captured tuples.
The known-started descendant exception under Stop below applies to ancestor Stop-only control
changes; generic revision comparison must not reintroduce retroactive invalidation in that case.

`rebindNodeIds` is an explicit exceptional action, not a default or automatic replay. IDs must be
unique and belong to `nodeAuthorities` at the captured current version/control. When an existing
workflow authority is bound to stale predecessor input/conditions, validate the new exact current
accepted input vector/fresh controller facts, then append a derived TaskVersion/new workflow
capture instead of editing the old authority. Preserve exact original/guidance, pins, acceptance,
timeout, routing/workflow choices (including explicit empty checks), execution mode/fake scenario
and captured policy, and record `dependency_rebind` provenance. This grants no units or window. A new workflow
is finalized only after its exact new input is retained; an unstarted child without a workflow
can take its first capture without a derived version. Stop/fence guards and unchanged-completed
work restrictions still apply. Omission never clones a completed node; ordinary stale-v1 Run
returns `graph_input_stale` plus the graph reference.

Start and explicit Advance record a live dispatch registration containing `ownerInstanceId`, a
positive `generation`, graph control/budget epoch/window and the captured node authority vector.
It authorizes only eligible unfinished work under those controls, never retry of a launched
terminal failure, an uncertain effect or a completed logical execution. Existing narrow explicit
workflow retry remains separately guarded. A grant, guidance update, report/configuration change
or read does not create or refresh this registration.

Known capacity waiting keeps the current registration live. A real current-owner completion and
known claim release, including release by an unrelated standalone attempt, may wake matching live
registrations. Select a bounded declared-order batch under actual available capacity; consumption
is unique per receiving node/version/input/logical slot/execution, not once globally per fan-out
predecessor. No graph worker needs to remain active merely to await another owner's local slot.
Known budget/routing/evidence/control blocks suspend the affected automatic scope; a later grant
or report alone cannot rearm it. A new owner preserves the old registration as history and requires
fresh explicit Advance. Reads and scans of retained terminals never generate release events.

When that explicit Advance resumes a healthy retained workflow under a new owner, retain its
original logical admission and known first-GO anchor. A private dispatch association to the
committed Advance receipt must prove the new owner's exact graph/node/version/workflow/input
and resumed dispatch; its later same-owner stages inherit that association. A newer live
registration generation alone neither proves this owner transition nor revokes an already
admitted unchanged stage. Check the durable association during claim preparation and GO as
well as admission. Historical terminals, their owners and original anchor bytes remain intact;
old-owner callbacks receive no renewed execution authority.

Advance returns 200 `GraphAdvanceResult`, including when current intent is acknowledged but work
waits for capacity/dependencies. It is not a false native admission. Genuine validation/control
conflicts and persistence faults retain their existing error semantics.

### Stop

Stop compares only its captured current graph/node control target and ordinary authentication,
ownership and durability requirements. It does not require a positive balance, open window,
budget epoch, current routing availability or permission to create a new worker. Node Stop also
checks that the route key and `nodeId` identify the same stored node.

Stop latches the requested scope durably and requests cancellation; it does not claim immediate
process/cleanup rollback. Group Stop prevents new graph admissions/preparation/effects. Node Stop
targets its own active work, later stages and unlaunched descendants; independently admitted
siblings and retained results keep their own outcomes. Accepted in-flight native/filesystem work
records actual settlement. A node's accepted result does not make Stop a no-op while its requested
scope still includes unlaunched descendants: latch that future-work prohibition without rewriting
the accepted result. A fresh late Stop against a scope with no remaining authorized work reports
`stopAccepted:false`; replay returns the original receipt and target IDs without chasing newer work.

Ancestor Stop is checked before a receiver's first native GO. Its logical node admission is
anchored separately from later per-stage dispatches by `nodeAdmissionId`. After actual first GO
is known and durably recorded, ancestor Stop alone does not cancel or stale an already-admitted
descendant workflow whose exact accepted predecessor versions/results/input remain unchanged.
Graph Stop and own-node Stop always prevent its later stages/effects. Ancestor guidance or a
changed current accepted input still prevents stale qualification. Unknown GO/acknowledgement is
not proof of either safe absence or eligible continuation; preserve evidence/ownership and fence.
Qualification must exercise ancestor Stop both before first GO and after the descendant starts,
plus own/group Stop and changed-ancestor-version counterexamples.

### Guidance and allowance

`expectedRevision` remains the actual node Task revision, not the aggregate graph state revision.
`guidanceText` is required and may be empty, preserving the established clear/update distinction.
The existing optional input rules remain. A graph-node `routing.budget`, any `workflow.delivery`
or `workflow.mode:"author_only"` rejects, including forbidden members supplied as null. Graph
delivery scope/layout cannot be expanded through the ordinary task guidance route. Graph guidance patches workflow choices field by field: omitted workflow objects or omitted fields
inherit the current captured node options. A nonempty explicit review choice replaces review;
`review:"auto"` resets it to Auto. Explicit `checks:[]` clears declared checks, and explicit
`maxCorrectionCycles:0` clears correction allowance. Other omitted choices remain captured.
Standalone005 guidance retains its established whole-object replacement behavior.
Guidance appends a task/workflow version and invalidates affected node/descendant input authority;
it starts no worker, creates no allowance and does not refresh a live registration.

A graph grant uses the budget authority epoch/window, never an accounting revision changed by
sibling debits. `additionalAdmissions` is an existing bounded `"1"`–`"16"` addition; cumulative
totals remain checked int64. Optional `newWindowSeconds` uses the existing 1–86400 safe-integer
bound. New-window authority requires known settlement of all work owning the graph budget and
current unfenced controls. The grant increments budget authority and records the existing
root-owned amendment/ledger; it creates no task version, admission or automatic dispatch.
An initial unopened window has the fixed graph-creation preparation deadline of 300 seconds.
An explicitly granted replacement unopened window captures a new fixed preparation deadline at
grant time plus `min(300, newWindowSeconds)` seconds. Reads, retries and individual preparation
operations cannot renew that deadline. The first native admission still opens the shared execution
window; the grant alone neither opens it nor refreshes a live dispatch registration.
Ordinary root-task and child grant/adoption routes reject with the graph budget reference.

## Response DTOs

| DTO | Exact fields and nullable branches |
| --- | --- |
| `GraphDefinitionRef` | `graphId`, `graphVersion`, `declarationSha256`. Complete declaration/captured authority is read through `/definition`. |
| `GraphCounts` | `required`, `waiting`, `ready`, `preparing`, `queued`, `running`, `blocked`, `cancelling`, `cancelled`, `failed`, `interrupted`, `completed`; all decimal strings. Node-state counts partition `required`. |
| `GraphSummary` | `id`, `rootTaskId`, `projectId`, `graphVersion`, `controlRevision`, `stateRevision`, `budgetAuthorityEpoch`, `maxParallelism`, `state`, `nextAction`, `nextOwner`, `rootInputId:null|string`, `finalNodeId:null|string`, `stop:null|GraphStopRecord`, `counts`, `createdAt`, `updatedAt`. |
| `GraphNodeSummary` | `id`, `graphId`, `key`, `taskId`, `taskVersion`, `taskRevision`, `controlRevision`, `stateRevision`, `state`, `nextAction`, `nextOwner`, `inputId:null|string`, `nodeAdmissionId:null|string`, `latestAttemptId:null|string`, `acceptedResult:null|GraphAcceptedResultRef`, `stop:null|GraphStopRecord`. Latest attempt is the actual current/supporting attempt, not a renamed deliverable producer. |
| `GraphAcceptedResultRef` | `acceptanceKind:"workflow"|"response"`, `acceptanceId`, `taskVersion`, `producerAttemptId`, `candidateId:null|string`, `responseArtifactId:null|string`, `inputId`; exactly one candidate/response branch. Full accepted bindings remain in exact input/output records. |
| `GraphStopRecord` | `requestId`, `acceptedAt`, `graphControlRevision`, `nodeControlRevision:null|string`, `targetAttemptIds:[]`. It records accepted scope, not cleanup completion. |
| `GraphDispatchAuthorityView` | `ownerInstanceId`, `generation`, `graphControlRevision`, `budgetAuthorityEpoch`, `windowNumber`, `nodeAuthorities:[]`, `state:"active"|"capacity_wait"|"suspended"`. This readonly record never grants client-supplied authority. |
| `GraphAdmission` | `graph:GraphSummary`, `definition:GraphDefinitionRef`, `nodes:GraphNodeSummary[]`, `preparationAttemptId:string`, `budget:GraphBudgetView`, `dispatchAuthority:GraphDispatchAuthorityView`. Receipt freezes initial identities/authority; materialization is not implied. |
| `GraphDetail` | `graph`, `definition`, `nodes`, `budget`, `activeClaims:GraphClaimView[]`, `dispatchAuthority:null|GraphDispatchAuthorityView`, `blockers:GraphBlockerSummary[]`. |
| `GraphAdvanceResult` | `graph`, `preparationAttemptIds:[]`, `dispatches:GraphDispatchView[]`, `reboundNodes:GraphReboundNode[]`, `budget`, `dispatchAuthority:null|GraphDispatchAuthorityView`. Empty work lists with an acknowledged waiting state are valid. |
| `GraphStopResult` | `graph`, `node:null|GraphNodeSummary`, `stopAccepted:boolean`, `reason:"accepted"|"already_stopped"|"already_settled"`, `targetAttemptIds:[]`, `affectedNodeIds:[]`. |
| `GraphGuidanceResult` | `graph`, `node:GraphNodeSummary`, `result:GuidanceResult` using the unchanged existing `{task,version}` data. |
| `GraphBudgetAmendmentResult` | `graph`, `budget:GraphBudgetView`, `amendment:BudgetAmendment` with the real root Task ID and existing exact amendment fields. |

Graph states are `preparing`, `queued`, `running`, `blocked`, `cancelling`, `cancelled`, `failed`,
`interrupted`, `completed`. Node states additionally allow `waiting` and `ready`; a node in either
state is unfinished. `nextOwner` is `council` or `user`. `nextAction` is the owner's machine-readable
named action, including existing workflow actions; clients display unknown actions conservatively
instead of inferring execution authority. `GraphBlockerSummary` is
`{code,message,nodeId:null|string,inputId:null|string,artifactId:null|string}`: at most one current
primary blocker per node plus one graph blocker. It is a primary summary, not an assertion that
all conflict paths/evidence were included; exact input/operation artifacts retain full details.

`GraphReboundNode` is `{nodeId,taskId,fromTaskVersion,toTaskVersion,provenance}` with provenance
`{kind:"dependency_rebind",requestId,previousInputId,contributorVectorSha256}`. It reports the
actual derived version and retained source association; it does not claim first-author admission
or materialization. The new live registration may name this server-derived version only because
the request explicitly authorized that rebind; replay returns the same versions/provenance.

`GraphClaimView` is `{slot,attemptId,graphId:null|string,nodeId:null|string,executorKind,
contextId:null|string,capturedDeclaredAccountId:null|string,capturedLinkRevision:null|string,
legacyBarrier}`. There are at most two, describing the actual global pool including unrelated
standalone/graph occupancy, rather than only the selected group's claims. It reports Council reservations, never authenticated
accounts, external occupancy or provider quota. Private native scope/effect records remain private.

Claim invariants are explicit: local executors have null native context/account/link fields;
captured declared account and link revision are present as a pair. A non-legacy native reservation
has its actual supported context; a migrated legacy barrier may retain unknown context. A legacy
barrier is the sole held claim in slot1 and prevents all new claims. local_graph identifies a root
graph member (graphId present, nodeId null); other graph-owned executors identify their exact node.
A standalone claim has null graph/node IDs. These fields are observations, never caller authority.

### Budget projections

`GraphBudgetRef` is `{graphId,budgetOwnerTaskId}`. A child `TaskBudgetView` is exactly
`{budget:null,adoptionRequired:false,graphBudgetRef:GraphBudgetRef}`. Existing standalone responses
keep their previous fields; they do not gain default graph fields or have historical receipts
rewritten. Public root-budget grants are still restricted to the graph route.

Existing execution-route `Admission` and `WorkflowStepAdmission` data for NEW graph-owned
executions keep their actual task/attempt/workflow/step identities, expose required `budget:null`
and add required `graphBudgetRef:GraphBudgetRef`. They never return the canonical root budget
as if it belonged to the child. Internal admission/controller facts may carry the actual root
budget separately; that internal value is not this wire field. Existing standalone fields,
omissions and historical serialized receipts remain unchanged. In particular, implementing this
projection must not globally inject graph fields or change old optional-budget omission rules.

For a graph-owned native decision, `routingDecision.taskId` is the actual child and
`routingDecision.budgetEffect.graphBudgetRef` is required. The existing `admissionUnits` describes
that attempt; `remainingAdmissions`, `budgetRevision` and `window` describe the referenced root
budget. Standalone `BudgetEffect` omits `graphBudgetRef`. Local stages still have no invented
native routing decision or native debit.

A child `/tasks/ID/budget-ledger` read returns 409 `graph_budget_required` with the additive
optional error field `graphBudgetRef:GraphBudgetRef` pointing to `/graphs/GRAPH/budget-ledger`;
it does not return an empty child page or silently substitute a root ledger. The existing error
fields/standalone behavior stay unchanged, and absent graph error references remain omitted.
Graph-owned mutations that reject direct budget/adoption likewise expose this actionable ref.

Budget scope never redirects ordinary usage/token reads: `/tasks/ID/usage` and corresponding
CLI output remain that actual node's retained attempts and measured/unknown telemetry. Group
summaries deduplicate exact attempt IDs and label their scope; root/group totals and included
node consumption are never added together. Inherited evidence/output links are not new usage,
and local graph operations do not turn absent model measurements into zero tokens/charges.

`GraphBudgetView` is `{graphId,budgetOwnerTaskId,budgetAuthorityEpoch,budget:TaskBudget,
nodeConsumption:GraphNodeConsumption[]}`. `budget.taskId` must equal `budgetOwnerTaskId`, which
must equal the stored graph root task. `GraphNodeConsumption` is
`{nodeId,taskId,consumedAdmissions,automaticAdmissions}` in declared order, at most sixteen.
These are included in root totals, never extra expenditure. Local graph/workflow stages do not
increase them or pretend to measure zero tokens/charges. Historical ledger/read data stays paged.

### Public input and member views

Internal/persisted `GraphRootBinding` retains exact private physical identities for verification
and sealing. Public reads use typed `GraphRootView`, never serialize that internal record and
never hide fields through a generic map. `GraphRootView` has `graphId,rootTaskId,operationAttemptId,
sourceSnapshotId,manifest,index,stagedBlobs,projectId,selectedRelativePath,baseCommit,branch,basisSha256`;
`branch` is nullable. Private project/repository physical identities, absolute paths, handles and
workspace/Git-control fingerprints are absent. It is an inspection projection, not a client proof.

`GraphInputSummary` has `id,graphId,graphVersion,rootTaskId,operationAttemptId,kind,state,current,
receivingNodeId,receivingTaskId,receivingTaskVersion,rootBasisSha256,inputSha256,failure,createdAt,
updatedAt`. `kind` is `root_basis|node_input`; receiver fields are nullable for a root basis.
Unobserved hashes and failure are nullable. `state` is
`preparing|ready|blocked|incomplete|uncertain|interrupted`. `current` is a computed boolean:
retained ready history does not become current merely because its bytes remain complete.

`GraphInputDetail` is `{summary,rootBinding,inputBinding,conditionsRef,contextPacket,operationEffects,
conflictArtifact}`. `rootBinding:null|GraphRootView` and `inputBinding:null|GraphInputBinding` are
mutually exclusive when a complete binding exists; both may be null during nonqualifying
preparation. The remaining fields are nullable closed `GraphArtifactRef`s except `contextPacket`,
which is a nullable complete `GraphContextPacket`. Conditions/context must name this exact
receiver/version. The input detail never substitutes a latest materialization: follow exact
dispatch/receiving-attempt links for that separate observation.

`GraphMemberView` is `{dimension,path,state,type,mode,byteLength,sha256,target,objectId,artifactRef,
origins}`. `dimension` is `working|index`, state `present|absent`; `path` is the canonical retained
manifest/provenance path. Type/mode/byteLength/sha256/target/objectId/artifactRef are nullable when
not applicable or absent. A regular working payload and a staged index blob retain distinct
refs/hashes; a symlink target is not silently followed. Raw file bytes remain in exact artifacts.
Working `mode` is the canonical decimal string of the retained native mode bits; index `mode`
preserves the exact Git mode string. A client may display working permissions in octal without
changing that wire value.
`origins` contains `GraphMemberOrigin {kind,nodeId,taskId,taskVersion,acceptanceKind,acceptanceId,
producerAttemptId,inputId,stateSha256}`. Kind is `root|node`; node/acceptance/version fields are
nullable for the explicit root origin. There are at most sixteen graph-node heads plus the root;
ordinary within-node attempt lineage stays separately retrievable. These are complete row
provenance references, not claims that a model inspected all original evidence. Origin `stateSha256`
identifies the source head: the root basis digest for `root`, and the exact contributing node's
verified sealed `GraphOutputClosure.closureSha256` for `node` (loaded through that exact historical
contributor's `outputClosureRef`). It is neither the closure-file hash, a per-path state hash nor
an attribution hash.
Root origins identify the root task, root operation and root input; node origins must match the
input's complete exact contributor vector. Raw payload references come from verified working and
staged manifests, never from an unchecked provenance reference.

Summary/member pages use the existing whole-item byte/count limits and immutable input-bound
cursors. Rows do not include private destination/checkout identities. Unsupported or excessive
complete detail is an explicit gap/artifact link, never a clipped row presented as full proof.

### Public declaration and captured delivery scope

`GET /graphs/{graphId}/definition` returns `GraphDefinitionView`:
`{graphId,graphVersion,rootTaskId,original,requestedPolicy,routingPolicy,delivery,createdAt}`.
`original` is the complete exact `GraphStartInput`; omitted declaration choices remain omitted.
`delivery` is `GraphDeliveryScopeView {localMode,preferenceRevision?,source,projectId,
selectedRelativePath,branch,destination,externalAction?}`. The external action is only the
explicit authored action/command declaration. Public scope contains no native project/repository
identity keys, resolved executable identity, private checkout paths or empty identity placeholders.

The complete persisted `GraphDefinition` retains private `DeliveryScope` identity facts for
verification/recovery. The controller-only `GraphDefinitionPrivate` getter returns that exact
record; it is never serialized as an API result. Public projection does not modify persisted
definition bytes, declaration hash or captured scope.

### Exact receiving execution and output

`GraphDispatchView` is `{dispatch:GraphDispatch,materialization:null|GraphMaterializationReceipt,
outputObservation:null|GraphOutputObservation}`. `GraphDispatch` is the immutable public admission:

`{id,graphId,graphVersion,nodeId,taskId,taskVersion,nodeAdmissionId,workflowId,stepId,attemptId,executorKind,role,
inputId,inputSha256,contextPacket,contributorVector,ownerInstanceId,graphControlRevision,
nodeControlRevision,budgetOwnerTaskId,budgetAuthorityEpoch,windowNumber,observedBudgetRevision,sourceBasis,
freshTriggerId,logicalSlot,executionOrdinal,nativeConsumptionKey:null|string,createdAt}`.

`nodeAdmissionId` names the logical first-author anchor, distinct from this particular stage's
dispatch/attempt. The exact anchor's attempt/append-once first-GO observation remains retrievable;
neither its LaunchIntent nor another execution's start proves this node input started.
`contextPacket` is its closed `GraphArtifactRef`; contributor bindings use the materialization
contract. `nativeConsumptionKey` is the attempt ID of its unique native-admission ledger entry
for a native execution; it is null for local executors. Every node workflow execution has a
dispatch, including zero-native-debit local checks/delivery. Root `local_graph` operations instead
belong to the root/input records. Claim observations and later artifacts never rewrite admission.

Materialization and output observations are append-once facts of this exact receiving/producing
attempt. `/dispatches/DISPATCH` validates graph/input/attempt ownership; input ID alone cannot
select an execution receipt. An absent receipt is not observed, never complete. The materialization
contract defines `graph_input` versus `node_subject` basis and expected/actual semantic equality;
supporting roles cannot reset to the original graph input. `GraphOutputObservation` retains
candidate and eligible response branches independently, at most one each. The candidate branch's
nullable `finalVerificationRef` names the separately sealed whole-graph file-final basis when
present. An actual final node may retain a zero-own-delta candidate without dummy edits; the
fresh root-to-final verification qualifies its cumulative files. Explicit response-only summaries
and standalone workflows retain their existing semantics. Closure artifacts are
captured before producer settlement and associated in the same terminal transaction; no latest
artifact lookup or append to a terminal producer is permitted.

## CLI proof capture and observation

The command families/routes remain those in graphs.md. Flags map without renaming semantics:

| Operation | Explicit proof flags |
| --- | --- |
| Advance | `--graph-version`, `--graph-control-revision`, `--budget-authority-epoch`, `--window-number`, `--node-authorities-file FILE`; optional repeatable `--rebind-node KEY` explicitly adds captured node IDs to `rebindNodeIds`. |
| Group Stop | `--graph-control-revision`. |
| Node Stop | `--graph-control-revision`, `--node-id`, `--node-control-revision`. |
| Node guidance | Above node controls plus existing `--expected-revision` and `--task-version`; existing guidance/options flags retain their meanings. |
| Grant | `--graph-control-revision`, `--budget-authority-epoch`, `--window-number`, plus required `--additional-admissions` and optional existing `--window-seconds`. |

`--request-id` is common. For a command's proof group, supply all flags or none; explicit empty,
mixed partial, conflicting file/flag values and unsafe counters reject. With no proof group, the
CLI performs one readonly current detail read, validates exact IDs, freezes these proof fields and
the full request, then dispatches once. This avoids mandatory manual settings while preserving
optimistic controls. It never refreshes proofs and silently resends after conflict/uncertainty.
Retain the exact frozen body for retry and expose the operation/request identity and safe proof
fields. Routine diagnostics do not dump prompts or private evidence. A retry must reuse the
original unchanged input/body and ID; unavailable original input is unresolved intent, not
permission to rebuild it from edited files or current defaults.
`--node-authorities-file` is a strict bounded JSON array of the complete node vector, at most sixteen.
`--rebind-node` resolves immutable keys through that captured graph detail; unknown/duplicate keys
reject. It is separate from the proof group and never implied by omitted flags or an empty vector.

Graph lists/dispatches default to 30 and maximum 100 whole items, bounded to 4 MiB. Cursors bind
resource, graph/project/filter/selector and captured high-water; counters remain strings. Input
members/raw originals and full operation/conflict artifacts use explicit bounded retained reads.
Wait uses the existing safe `afterRevision`/`timeoutMs` semantics on graph `stateRevision`: return
only on a strictly greater observation or explicit timeout. Wait/read creates no registration,
admission, budget, effect or retry. Stale/foreign selectors reject without silently retargeting.
