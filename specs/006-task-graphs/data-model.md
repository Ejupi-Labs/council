# Task graph data model

Status: design only. Schema 6 extends accepted 005 source `794f080`; no application implementation
is authorized until the independently reviewed plan/contracts/tasks are committed. This document
defines domain/storage invariants; [graphs API](contracts/graphs.md), [operation shapes](contracts/operations.md), [materialization](contracts/materialization.md)
and [interface](contracts/interface.md) define their corresponding boundaries.

## Existing authoritative seams

- `internal/store/tasks.go` creates tasks/versions and admits ordinary attempts. Its graph-node
  path must not call `initialBudget` for each child.
- `routing_admission.go`, `routing_state.go`, `routing_budget.go` and `routing_reads.go` own the
  canonical budget, admission/launch ledger, amendments and projections. `insertRoutingDecision`
  currently derives ledger ownership from the decision's task; graph ownership must be resolved
  there as well as before debit.
- `workflow_stage_admission.go` and `workflow_routing.go` own workflow/004A admission. Existing
  `WorkflowAdvanceProof.BudgetRevision` compares an accounting revision; preserving that rule
  unchanged for shared graph accounting would incorrectly invalidate sibling continuations.
- `execution.go`, `attempts.go`, `results.go` and owner recovery currently assume a singleton
  claim. `internal/engine/engine.go` has singular active-attempt/cancellation fields. Both must
  become genuinely bounded multi-attempt ownership, rather than treating a missing singular ID
  as an empty pool.
- `artifacts` and `source_snapshots` require real owning attempts. Existing 005 handoff and
  candidate guards deliberately bind a single producer/receiving workflow. Graph input is a
  separate provenance object, not a fabricated authored candidate or reassigned root source.

## Representation and bounds

Public IDs use existing opaque 32-hex identities. Counts, revisions, epochs, ordinals, byte lengths
and cursor positions are canonical nonnegative decimal strings backed by checked signed 64-bit
integers; positive revisions start at `1`. Never convert them through floating point. Check
overflow before mutation. Timestamps use the existing UTC representation and parsing rules.

A definition contains 1–16 nodes and 0–64 edges in one project. Node keys are unique,
case-sensitive ASCII letters/digits/underscore/hyphen, 1–48 characters beginning with a letter
or digit; generated opaque node IDs are distinct from keys. Preserve declared order, exact
original text and original evidence. The graph declaration is limited to 2 MiB; existing 4 MiB
per-text and 32 MiB generic-body ceilings do not increase that limit. Reject an oversized declaration before
creating execution authority. Do not truncate it to fit. Unknown/null/explicit-empty members
follow the existing strict presence rules; omission, an empty allowed collection and false are
distinct. Request fingerprints exclude neither original content nor explicit choices and never
inject default fields into historical input.

Routine graph metadata contains bounded summaries and references, not full conversations or all
file members. Full definitions, originals, manifests and artifacts remain explicitly retrievable.
Existing source limits,256 KiB native prompt limit,16 KiB event bound and 4 MiB whole-item page bound
remain. A byte bound is not a provider token-capacity claim.

## Graph ownership and immutable definition

`Graph` has `id`, `rootTaskId`, `projectId`, `graphVersion`, `controlRevision`, `stateRevision`,
`budgetAuthorityEpoch`, `state`, `nextAction`, `nextOwner`, `rootBinding`, `finalNodeId`, `stop`,
`createdAt` and `updatedAt`. Nullable references are explicit where the wire contract requires
them. `rootBinding` is absent while real basis preparation is incomplete.

The root task represents the actual user-supplied graph intent and constraints. It is not one of
the declared worker nodes and receives no fake author role or model admission. Root operations
are real `local_graph` attempts with roles `graph_basis` or `graph_input`; they launch no native
model, but may run shell-free Git subprocesses for actual capture/materialization. Their attempts,
source/artifacts, terminal evidence and cleanup are inspectable. They are not inserted into the
schema 5 `workflow_steps` table whose role/executor checks remain unchanged. Private input work
does not claim successful native execution or delivery to the user checkout.

`GraphDefinition` retains graph ID/version, original intent/constraints, ordered node declarations,
typed edges, captured permission/routing/budget authority, `maxParallelism` and graph delivery
selection. `maxParallelism` is optional canonical string `"1"` or `"2"`, captured as `"2"` when
omitted. Layout,
node membership and edge scopes are immutable in 006. There is no general graph editor/reparenting
API. Newer unsent composition remains a client draft; replay returns the exact prior declaration's
identities. Node guidance changes are separate versioned operations described below.
Capture `executionMode` and each node's fixed fake scenario in its execution authority; later
review/correction/retry derives mode from that authority, never from the latest local operation.
Fake/native distinction and unknown provider/model/account/usage facts retain accepted 005 rules.

Graph creation establishes child task identities/versions and prospective captured choices, not
an eagerly finalized workflow for every waiting node. Capture its first immutable workflow authority
after the exact input/scoped conditions are retained and before native admission. An unstarted
child with no workflow can capture that first authority without cloning a task version.
Once workflow authority exists for a task version, do not replace its input/conditions in place:
the existing unique `(task_id,task_version)` workflow and logical steps remain immutable.

Explicit `GraphAdvanceInput.rebindNodeIds` (omitted means empty) authorizes a derived TaskVersion
and new workflow for a named stale node whose existing authority is bound to an old input or
conditions. Validate each ID against the captured current node authority vector and verify the
new exact current accepted input vector/controller facts before mutation. Preserve exact user
original/guidance, literal pins, acceptance, timeout, routing/workflow options (including explicit
empty checks), execution mode/fake scenario and captured policy; record `dependency_rebind` provenance linking
old/new versions, prior input and new contribution-vector digest. The new workflow is captured
after its new input is retained. No allowance/window is added. Update the live registration to the
derived version only for these explicitly named nodes; other versions cannot be substituted.
Automatic Continue never clones/reruns a completed node. Ordinary Run against stale v1 returns
`graph_input_stale` and an actionable graph reference. Stops/fences remain binding; a rebind cannot
resume them or clone unchanged completed work merely to obtain another execution.

Every declared node is required. A `GraphNode` has `id`, `graphId`, `key`, `taskId`, `controlRevision`, `stateRevision`,
current task version/input/result references, Stop and current dependency/action projection. Its
task belongs to exactly one graph for its lifetime. Graph membership comes from stored rows, never
from optional client-supplied graph fields.

Current graph-owned `Task`/`TaskSummary` projections carry optional additive
`GraphRef {graphId,nodeId,rootTaskId,kind}`. It is present for graph members and omitted for
standalone tasks; `kind` is `root|node`, with `nodeId:null` only for the root and the exact node ID
otherwise. Derive it from authoritative membership rows, without rewriting historical task or
receipt JSON. Root and waiting no-workflow node views use this reference to suppress standalone
Run/adoption/grant controls and link their graph controls. UI absence is never enforcement:
every server admission/adoption/grant/control path resolves membership independently, including
requests omitting or forging graph metadata. A graph root is not an extra declared worker.

The visible new-graph default is `separate_reviewable_results`, including when the saved 005 local
delivery preference authorizes application. Selecting a final node prospectively captures its
compatible saved local scope or explicit final external scope. No final node plus an explicit
delivery request is invalid. Intermediate declarations permitting local application or external
action are rejected, not silently downgraded. Permission mode alone creates no delivery consent.

## Canonical budget and authority epochs

The graph root owns exactly one existing `TaskBudget` and existing budget ledger. The captured saved
finite default/explicit graph override grants its authority once. Existing saved two/custom
defaults remain authoritative; no migration or child construction supplies four more units.
Nodes have no independently spendable `task_budgets` rows.

Internal `ExecutionScope` resolves a task to `standalone`, `graph_root` or `graph_node`, together with
`budgetOwnerTaskId`, graph/node IDs, relevant control revisions, budget authority epoch and window.
For standalone tasks the budget owner is the task itself. Resolve membership within the same
transaction as every admission, claim, launch guard, acceptance and budget mutation.

`GraphBudgetRef {graphId,budgetOwnerTaskId}` is explicit in a graph node's `TaskBudgetView`; its
ordinary `budget` is null and `adoptionRequired` is false. It must not masquerade as a child budget.
`GraphBudgetView {graphId,budgetOwnerTaskId,budgetAuthorityEpoch,budget,nodeConsumption}` exposes
the actual root-owned `TaskBudget` and at most 16 exact per-node native-debit counters. Root local
operations remain zero native admissions, not measured zero tokens/charges. Ordinary root-task
and child grant/adoption routes reject with an actionable graph reference. Only the graph amendment route grants or opens
a new shared window; it is an explicit authority change and starts no work.

New graph-owned responses from existing `Admission` and `WorkflowStepAdmission` routes expose
`budget:null` plus `graphBudgetRef`, not a root budget disguised as the actual child task's
budget. Their other actual task/attempt/workflow fields retain their meanings. Keep internal
canonical-budget facts separate from this wire projection. Old standalone response shapes and
all historical receipt bytes remain exact; do not add null/default graph fields globally.
`RoutingDecision.TaskID` remains the actual child. Its `budgetEffect.graphBudgetRef` explicitly
scopes root remaining/revision/window values while its admission units describe this child
attempt. Standalone budget effects omit that new reference.

A direct child budget-ledger request returns an actionable `graph_budget_required` reference,
not an empty fabricated child ledger or an unlabelled redirected root ledger. The graph ledger
route reads the one canonical root ledger. Budget-owner resolution is NOT a general task-query
redirection: node usage/token/evidence reads remain the actual node's retained attempts. Any
group usage/accounting aggregate deduplicates attempt IDs; never sum a root/group total plus
its already included node breakdown or count inherited output references as new model use.

The existing budget accounting `revision` changes with debits and ledger observations.
`budgetAuthorityEpoch` changes only when shared budget authority changes, including explicit
grants/new windows. Node guidance/Stop use the affected node controls; they do not increment this
shared epoch or invalidate unrelated sibling authority.
Ordinary sibling admissions must not invalidate unrelated fresh continuation authority. New graph
proofs capture that epoch and window, plus relevant graph/node controls; they retain the observed
accounting revision as evidence but do not require it to remain unchanged. Admission always reads
the current remaining allowance atomically. Existing standalone 005 proof semantics are unchanged.

The first budgeted native admission opens the existing common wall-clock window, including queue
time. Each attempt deadline is the earlier of that window and its captured per-attempt ceiling.
Initial zero-debit basis/input preparation has the fixed ceiling `graph.createdAt + 300s` before
that window exists; another local operation does not renew it. After the common window opens,
each local graph operation's deadline is `min(operation.createdAt + 300s, commonDeadlineAt)`.
It cannot run a model or renew the window. Later local input/check/delivery stages
obey the open common window even with zero remaining native admissions. A new window requires
known settlement of all work owning this budget, not merely the root task's latest attempt.

All native paths share this resolution: first author, ordinary task Run, explicit workflow
advance/recheck where native, correction/review, narrow pre-launch retry, 004A replacement and
automatic workflow continuation. Save counters, per-node debit, exact routing decision, canonical
root ledger entry, immutable attempt/dispatch and receipt in one transaction. Keep the existing
unique admission/launch ledger indexes; graph/node views never count the same attempt twice.

## Concurrent capacity and owner state

Default global managed capacity is two, with a default native-context limit of one for each of
`codex-native` and `claude-native`. Narrower captured graph/current declared account/context
restrictions prevail. A declared account has a fixed managed-reservation limit of one across
captured and current links; this is a Council reservation rule, never a provider quota claim.
Captured `maxParallelism` limits this graph to one or two managed workers. Disabled contexts and
effective negative reports can reduce eligibility to zero. No new numeric account/context settings
API is introduced. Local executors need managed ownership but claim no native context/account
facts. Capacity is dispatch eligibility, not a fabricated provider allowance or task budget grant.

For a new graph-native admission, select its target, reserve an available slot/context/account,
check current proofs and debit the shared allowance atomically in one transaction. A subsequent
unpinned ready node observes the earlier reservation and can choose the other eligible idle native
context. It must not admit both nodes to the same context merely because neither has launched.
When no compatible capacity is available, keep the node waiting before native admission. This is
not routing ineligibility or a category-A continuation reason. An already admitted attempt keeps
its immutable target and reservation; ordinary standalone queued-admission behavior is preserved.

Schema 6 replaces `capacity_claims` with these logical fields:

| Field | Constraint/purpose |
| --- | --- |
| `slot` | Integer primary key, restricted to 1 or 2; unique occupied global slot. |
| `attempt_id` | Unique FK to the real owning attempt. |
| `owner_instance_id` | Exact owner epoch. |
| `context_id` | Nullable native context FK; unique when non-null. Local operations have none. |
| `captured_account_id`, `captured_link_revision` | Nullable declared account FK and captured link revision, not authenticated identity. |
| `graph_id` | Nullable graph FK; standalone remains identifiable. |
| `legacy_barrier` | Boolean; retained old claims cannot be treated as newly available capacity. |
| `data` | Exact claim facts, including creation time and effective captured restrictions. |

Claim evaluation reads at most two active claims and current links/restrictions in the same
transaction. A captured account reservation continues until known release; a current relink may
add a restriction but cannot free that captured reservation. Deduplicate a claim when both point
to the same account. Declared labels/links do not select actual vendor login or prove identity,
quota or external occupancy. Known negative reports and accepted 004 freshness/sticky-exhaustion
rules still apply. Current restriction changes never retarget an already admitted native target.

`ExecutionGate`/metadata expose a bounded active-claim collection and effective limits. A retained
singular compatibility field is never an authoritative empty-pool test. Claim/launch/control/
settlement helpers must check ownership of the specific attempt. Claimed local filesystem effects
remain occupied on unknown effects independently of process cleanup, preserving 005 semantics.

The engine keeps an attempt-keyed active-worker/cancellation map. Group/node Stop, service shutdown,
fencing and recovery enumerate exact owned attempts. Process/filesystem I/O never occurs under a
SQL transaction or a long-held control lock. Retain 005 serialization between current eligibility
checks and native preparation; accepted Stop remains responsive and prevents new effects permits.

## Root, accepted contribution, input and receiving bindings

The materialization contract is authoritative for exact hashing, causal slots and filesystem
algorithms. These are internal persisted/control bindings; public projections expose stable IDs,
hashes and scoped source facts while private physical identities, handles and paths remain private.
Common names and ownership are:

- `GraphArtifactRef {artifactId,sha256,byteLength,producerAttemptId}` names a complete closed
  registered artifact. Public refs carry no private executable path or native handle.
- `GraphRootBinding {graphId,rootTaskId,operationAttemptId,sourceSnapshotId,manifest,index,
  stagedBlobs,projectId,projectIdentity,repositoryIdentity,selectedRelativePath,baseCommit,branch,basisSha256}`
  describes the once-captured exact dirty original. All root artifacts belong to its real basis
  operation attempt.
- `AcceptedContribution` binds node/task/version, acceptance kind `workflow|response`, acceptance
  ID, exact selected producer/candidate-or-response, producer `inputId` and
  `GraphOutputClosure {graphId,nodeId,receivedInputId,kind,producerAttemptId,candidateId,
  responseArtifact,manifest,index,stagedBlobs,closureSha256}`.
  Exactly one candidate/response branch is present. Candidate closure requires the actual candidate
  and complete staged blobs; a context-only response carries its exact response/source/index and
  staged blobs without inventing a candidate or contributing file slots. The seal excludes itself.
  A supporting reviewer or latest attempt is not substituted for the selected producer.
- `GraphInputBinding {id,graphId,graphVersion,rootBasisSha256,operationAttemptId,receivingNodeId,
  receivingTaskId,receivingTaskVersion,contributions,contributorVector,manifest,index,stagedBlobs,
  provenance,contextPacket,inputSha256}` is a real root-operation-owned prepared input, not a
  candidate. Only file-change edges affect file/index slot origins; context edges cannot
  manufacture file incorporation. `inputSha256` hashes the fixed file-input fields through
  `provenance`, including identities and the contributor vector, and excludes itself and
  `contextPacket`. Seal that digest before constructing the separately hashed context packet;
  dispatch verifies both references. This avoids a recursive context/input hash.
- `GraphConditionsBinding {graphId,graphVersion,receiverTaskId,receiverTaskVersion,sources,
  conditionsSha256}` is generic to every node, not final-only. Bind its exact closed reference
  and scoped digest before first immutable workflow capture and direct-proof assessment. Retain
  the ordinary node condition digest separately; the effective digest binds it plus this scoped
  graph digest. Candidate/review/check applicability uses that effective scope consistently.
  Shared/receiver constraints are authoritative; upstream-local conditions remain attributed
  context. An unsupported direct proof of the complete authoritative scope is not proven.
  Old standalone authority/digests stay unchanged; explicit rebind captures a fresh version.
- `GraphMaterializationReceipt` records input/receiving attempt/source-snapshot identity, actual
  resulting manifest/index hashes, completion time, operation effects and state. The receiving
  attempt owns its ordinary source snapshot; no prior terminal producer gains appended artifacts.
  Root input retention finishes before native admission; actual receiving materialization runs
  during that already durable receiving attempt's preparation and adds no additional native
  debit. Failure can honestly settle the admitted attempt without creating a process; it neither
  fabricates a successful source handoff nor refunds/repeats the admission automatically.
  Its bounded metadata points to a complete `graph_operation_effects` artifact instead of
  embedding an unbounded effects array. Only state `complete` proves materialization; `blocked`,
  `incomplete` and `uncertain` remain nonqualifying observations.
- `GraphSourceBasis` is captured in every node dispatch at its exact workflow-step admission.
  `basisKind:graph_input` is limited to the first author of a version or an authorized same-input
  author replacement. Later review/correction/check/delivery roles use `basisKind:node_subject`
  and the immutable step's exact selected candidate/response through accepted 005 Handoff; they
  never rebuild from the original graph input and discard node work. Both bases retain graph
  input origin plus expected manifest/index refs and semantic working/index digests. Record
  expected/observed raw artifact hashes separately: receiving ownership and raw-index rebuilding
  may change raw JSON/index bytes, while fully verified semantic state must agree. Re-inventory
  the actual managed source and Git-control identity before Prepare and GO; closed artifacts
  alone do not establish continuing materialized availability.
- `GraphDispatchBinding` records graph/version/node/task version/input, exact accepted-contributor
  vector, owner epoch, graph/node control revisions, budget authority epoch/window and receiving
  attempt. It is the immutable admission provenance, not permission supplied by peer prose.
- `GraphDeliveryBinding` verifies original graph root to actual selected final candidate together
  with current graph/node authority and contributing accepted results. It wraps 005 delivery
  guards/permits rather than rewriting a candidate's same-workflow `RootSourceSnapshotID`.
- `GraphFinalVerificationBinding` separately seals the declared file-final node's actual candidate,
  its output closure, graph root, exact contributor vector, scoped graph conditions and verified
  root-to-final cumulative working/index delta. An actual zero-creation-delta final candidate
  remains valid retained evidence; it is not fake authorship or a reason to require a dummy edit.
  The internal graph-final planner may bypass only 005's early own-delta requirement when this
  verified nonempty cumulative file basis exists. Node/Candidate delta flags and root ancestry
  remain honest, and standalone or explicit `response_only` behavior is unchanged. Graph-shared
  and receiver conditions are authoritative; upstream-local conditions remain attributed context,
  not automatically promoted receiver obligations. Bind the effective final condition digest to
  the ordinary node digest plus the scoped graph digest before execution/review/checks.

Working and staged state are distinct causal slots. Root and selected output capture retain the
Git staged blobs, not only raw index OIDs or working-file bytes. `stagedBlobs` references a closed
`graph_index_blobs` manifest of unique object ID, content SHA256/length and artifact reference;
verify the Git blob object hash separately from raw-content SHA256. New graph native/check outputs
retain their staged-blob closure before producer settlement. Missing objects later are a blocker,
not permission to reconstruct bytes from an unrelated working file or append to a terminal attempt.
The combined working/staged payload limit is 512 MiB: count working bytes plus unique staged
content not already present with the same SHA256 and byte length among working members. The
independent working-tree 512 MiB cap and the materialization contract's other bounds still apply.

Node contribution delta is its original graph receiving input to its full selected accepted
candidate, including correction/check lineage. It is never just the last candidate's immediate
creation delta. Shared ancestry appears once; a proven descendant refinement supersedes only
that inherited contribution. An unchanged sibling copy does not conflict; divergent sibling
heads do. Provenance has at most 16 contributing graph-node heads per slot, with ordinary 005
attempt lineage still fully retrievable.

Focused context binds graph constraints, current node instructions/conditions and declared relevant
contributions. Derived digests are labelled, with exact source/original access. Retain complete
originals behind bounded references actually accessible to the receiving role. Context preparation
uses no model call; unavailable access or required capacity is an explicit pre-launch blocker.
No mutable checkout path or summary substitutes for fresh closed-file verification.
`graphInstructionsRef` is a closed lossless artifact of ONLY the captured `sharedInstructions`
field, including allowed empty text. Retain the full graph declaration separately for human
provenance; never broadcast it or unrelated sibling originals as shared instructions. Empty shared
scope preserves an otherwise valid 005 direct shortcut without inventing another model call.

## New graph tables and relational invariants

All existing schema 5 tables and row JSON stay intact, except the deliberately migrated claim
table. Five graph tables are sufficient; bounded edges/context/contributor vectors remain in
immutable definition/input artifacts, rather than introducing an unbounded scheduler framework.

| Table | Indexed scalar columns and retained data |
| --- | --- |
| `task_graphs` | Global positive sequence PK; unique ID/root-task FK; project FK; graph version, control/state revisions and budget-authority epoch; live dispatch owner/generation and bounded authorized node vector; state; current root-input FK; nullable final-node ID; current projection JSON. |
| `graph_versions` | `(graph_id,version)` PK, graph FK, immutable declaration/authority JSON; creation request/time. Version 1 layout is immutable in 006. |
| `graph_nodes` | `(graph_id,node_key)` unique; unique node ID and task FK; declared ordinal, node-control/state revisions, native-debit count; current input/node-admission/result refs, Stop/projection JSON. Node task version/revision comes from the authoritative existing task row. |
| `graph_inputs` | Positive sequence PK; unique input ID; graph/version FK, optional receiving-node FK; unique real operation-attempt FK; root-input FK; state; sealed manifest/index/staged-blob/provenance/context artifact FKs and binding JSON. Root source snapshot FK when this is the root basis. |
| `graph_dispatches` | Positive sequence PK; unique ID and admitted-attempt FK; graph/version/node/input FKs; task version FK; node-admission anchor, fresh-trigger ID, logical slot/execution ordinal, relevant control/epoch/window; nullable unique native-consumption key and immutable admission/source-basis JSON; separate append-once known-first-GO/materialization/source-snapshot refs and bounded output-observation JSON. |

Use `typeof(...)=integer`, nonnegative/positive checks and FKs consistent with accepted tables.
States are explicit finite enums. Verify scalar indexes against decoded JSON on reads. Append-only
definition/dispatch/binding histories never silently change producer/version/authority. An input
may transition through preparation, but its sealed ready binding is immutable; later failure or
staleness is a separate current projection, not rewritten artifact provenance. Deferred creation
references (graph header to its root input, final node) are installed only after their referenced
rows exist inside the creating transaction; foreign-key enforcement is never disabled.

The materialization receipt is a closed receiving-attempt-owned `graph_materialization` artifact.
Its separate dispatch
observation references are installed at most once after real materialization and source
registration; the original admission-binding JSON remains byte-immutable. Recording a different
receipt/source for the same dispatch is a conflict, and a lost acknowledgement never reruns Git
or filesystem materialization. Failed preparation stays attributable through ordinary attempt
events/artifacts without an invented complete receipt. A nonqualifying receipt retains explicit
state, effects-known status, evidence status and failure, with unavailable snapshot/effects
references absent. Only complete evidence, known effects and no failure qualify the receipt.

Every node workflow execution has a dispatch row, including local checks/application/external
commands; local dispatch has no native consumption key or model debit. Root `local_graph`
operations remain owned by `graph_inputs`. `GraphOutputObservation` is append-once
`{producerAttemptId,candidate,response}` with at most one nullable branch of each kind:
`candidate:{candidateId,state,closureRef,finalVerificationRef}` and
`response:{responseArtifactId,state,closureRef}`. `finalVerificationRef` is nullable and names the
exact eligible final-file basis when present. Seal output closure before final verification basis,
then install the observation; there is no recursive hash dependency.
States are `complete|incomplete|uncertain`; only complete requires the exact closed
`graph_output_closure` reference. Capture both eligible author/correction branches and mutating
local-check candidates while the actual producer is unsettled. Install the association in the
same transaction as producer terminal settlement. A later accepted-contribution lookup uses
exact producer/kind/result ID, never latest artifact or latest supporting attempt. A complete
closure still requires actual current outcome/acceptance; partial useful results are historical.
Supporting review responses cannot become deliverable response branches. Old Candidate/response
rows and wire JSON remain unchanged, and a terminal producer gains no later artifacts/branches.

Required indexes: graph creation sequence; nodes by graph/declared ordinal; inputs by graph and
sequence; dispatches by graph/node and sequence; unique task membership, operation/receiving
attempt, definition version, dispatch consumption and target node/version/input/logical-slot/
execution-ordinal. No status view scans full attempt/artifact
history to reconstruct counters or discover new work. Per-node counters update with native debit.

## Transaction/controller boundaries

The following are typed store responsibilities; public route/input spelling belongs to the graphs
contract. Controller proof structures are not decoded from client JSON.

1. `CreateGraph(ctx,ownerID,input,captureFacts)` replays the exact receipt first. On new intent it
   creates root/node task identities, immutable definition/captured scope, one root budget and a
   real initial local operation, without child budgets or native calls. The admission response
   identifies retained preparation; it does not claim files are already materialized.
2. `BeginGraphInput(ctx,ownerID,graphID,nodeID,preparationFacts)` checks the fresh live trigger,
   exact current node/accepted-contributor vector/root and deadline, then creates the real root-task
   `local_graph` operation and preparing input row before Git/object/artifact writes. It returns
   a typed operation/plan with no native debit; controller filesystem work runs afterward.
   `CompleteGraphInput(ctx,ownerID,operationAttemptID,facts)` checks current operation ownership,
   registered artifact ownership/hashes and complete fresh controller verification, then seals
   the binding and atomically settles its local operation. Native/filesystem I/O occurs before
   this transaction; uncertain acknowledgement fences and is never automatically retried.
3. `StartGraph`/`AdvanceGraph` validate explicit current intent. `ContinueGraph` consumes only a
   fresh current-owner completion/preparation proof. In one transaction, recheck current accepted
   dependency vector, relevant versions/controls, ready input, routing and shared budget; consume
   the unique dispatch key; create exact attempt/dispatch/receipt and debit once. Merely polling
   a ready graph or scanning retained terminals cannot call this consumer.
   When an authorized node lacks a sealed current input, return/create the bounded preparation
   operation first rather than pretending native admission already occurred. Its known completion
   supplies the fresh trigger for later atomic target reservation/admission.
   Explicit Advance may atomically append the separately requested stale-node rebind versions
   before creating their input-preparation operations; report every old/new version/provenance.
   This is an acknowledged prospective version change, not a claim that its new workflow/input
   is already materialized or a native attempt has already run.
4. `RecordGraphMaterialization(ctx,ownerID,receivingAttemptID,facts)` rechecks current dispatch/
   input/source-basis/control ownership and verifies registered source/receipt artifact tuples, then installs
   its append-once observation. The specific launch guard requires this complete exact binding
   before any receiving native preparation, using graph seeding only for `graph_input` and 005
   selected-subject Handoff for `node_subject`. No public boolean or supplied path is availability
   proof; filesystem verification and artifact closure precede the SQL transaction.
5. Existing workflow continuation/retry/admission paths resolve graph scope internally. They may
   progress a node only under its current dispatch/input authority and canonical budget. They
   cannot admit another first author or child budget through a standalone-looking request.
6. `StopGraph` atomically latches graph Stop and requested cancellation of admitted/queued work.
   `StopGraphNode` latches node Stop, prevents further stages/unlaunched descendants and targets
   only its actual active attempts. Independent admitted siblings and historical accepted results
   remain unchanged. Cancellation acknowledgement is not cleanup proof.
7. `UpdateGraphNodeGuidance` appends an ordinary immutable task/workflow version without running
   it or adding allowance. Invalidate the node and affected descendants' input/control bindings,
   not unrelated siblings. Old dispatched work remains attributable to its old input/version and
   cannot silently qualify the current node. Rebinding to newly accepted inputs is explicit fresh
   provenance, never an edit of old acceptance/dispatch records.
8. `AmendGraphBudget` checks root/group authority, canonical counters and relevant live claims.
   Its new authority epoch invalidates old automatic proofs; the grant itself starts no work.
   Node consumption/accounting remains cumulative across guidance, retry, grants and restart.

Recheck graph/node Stop, owner/fence, exact task/input version, claim and current restrictions at
admission, launch, effects and acceptance. Recheck the common deadline before every new execution/
preparation/effect permit. Preserve the accepted 005 exception for a current fully qualified human
response acknowledgement after the execution window expires: it creates no execution, effect,
allowance or new window. Existing known-precommit cancellation versus actual-commit uncertainty
distinctions remain unchanged.

Distinguish a node's logical admission from its individual workflow stage dispatches. Each stage
records `nodeAdmissionId`, the first-author dispatch for that node/task-version/input execution
(or its separately authorized author replacement). The anchor receives an append-once known
first-GO observation when actual owned Start is durably acknowledged; LaunchIntent/Prepare alone
never establishes that observation. Before its first GO, an ancestor Stop denies the receiver's
launch and unlaunched descendant admission. After known first GO, an ancestor Stop alone does not
retroactively stale this admitted descendant or deny its later stages while the exact accepted
predecessor versions/results/input remain unchanged. Do not cascade own-node control increments
into these already-started descendants merely for ancestor Stop. Graph Stop and own-node Stop
still deny every new stage/effect; ancestor guidance or changed current accepted input still
invalidates stale qualification. Unknown GO/acknowledgement retains actual evidence/claim/fence
and authorizes no later stage. First-author replacements receive a fresh anchor and all current
pre-GO guards; an old started marker cannot be borrowed for a new input or task version.

A fresh trigger can enable several followers. Process a deterministic declared-order batch,
bounded by the available managed claims (at most two); consumption is per target execution, not
once globally for the predecessor. Bind each consumption to the fresh trigger ID plus target
node/task version, sealed input ID, logical slot and execution ordinal. Independently enforce
uniqueness of that target slot/execution, so another trigger cannot duplicate it. Supporting
workflow roles retain their existing logical-slot uniqueness. Explicit/004A replacements use
their separately authorized next execution ordinal and preserve the predecessor.
Capacity-waiting followers remain visible; a later fresh current-owner completion or explicit
advance may select them. Observation, a new owner or a scan of old completed rows cannot manufacture
the missing fresh trigger or re-admit an already executed target.

Start and explicit Advance register live current-owner graph intent: owner instance, generation,
captured graph/budget controls and bounded authorized node version/control vector. A real known
claim release from ANY current-owner execution, including an unrelated standalone task, may wake
only these matching registrations when capacity becomes available. The graph need not keep a
worker alive solely to await capacity. Reads, guidance, grants, reports/configuration and restart
cannot create/refresh this registration. Capacity wait retains it; a known budget/routing/evidence/
Stop/fence block suspends the affected scope until new explicit intent. Node guidance leaves old
tuples stale only for the affected node/descendants, without expanding live authority or stopping
unrelated authorized siblings. An old owner's persisted registration never authorizes a new owner.

## Aggregate state, delivery and recovery

Graph states use `preparing`, `queued`, `running`, `blocked`, `cancelling`, `cancelled`, `failed`,
`interrupted` and `completed`, with next owner/action and node counts. A failed/cancelled required
node cannot become graph success; independent active siblings drain with visible ongoing cleanup.
Blocked and running sibling counts are separate from the aggregate outcome. No terminal label
implies that another owned process or filesystem effect has settled.

Acceptance requires every declared node's current qualified outcome and exact required input
bindings. Separate reviewable results can complete without application. For final delivery,
all OTHER nodes have current accepted results; the final cumulative candidate has current required
pre-delivery checks/review and known prior settlement; then required captured delivery occurs;
then final-node and graph acceptance may complete. Acceptance is not its own delivery prerequisite.
Every node reaches the final node; every file contribution reaches it through file-change paths.
A response-only final summary cannot qualify retained file changes for requested application.
Pure-response graphs complete through their actual response acceptances without fake integration.

The graph-specific delivery binding supplies the captured graph original and selected final
candidate to existing 005 application preflight, Ready/plan, separate action permits, exclusive
publication, actual effects and acceptance checks. Preserve dirty/unrelated/index state. Never
apply intermediate changes separately, reinterpret a context edge as a file edge, or bypass the
retained-root versus selected-candidate verification because a final node succeeded.

Recovery enumerates all claims/operations. Known local preparation without permits records
interruption; unknown ownership/effects retain claims and incidents, with dispatch fenced. Keep
complete sibling results/receipts unchanged. A new owner neither scans old accepted nodes into
new work nor replays input operations/native calls/effects. Durable pause/resume/reconciliation
and cleanup remain 007; explicit 006 controls cannot erase an unreconciled fence or stopped history.

## Migration, compatibility and bounded reads

Freeze an actual known schema 5 fixture before implementing schema 6. Under the existing private
lock, validate application ID, exact accepted DDL/indexes, FKs, scalar/JSON consistency and claim
integrity. Reject unknown objects/definitions rather than repair/reset. In one transaction add
the five graph tables/indexes and replace claims. Copy a held legacy claim to occupied slot 1
with exact attempt/owner/context facts and `legacy_barrier=true`; preserve its reservation/fence
until recovery establishes known safe settlement. Never silently clear it to advertise slot 2.
While any legacy barrier remains, deny every new managed claim, including apparently free slot 2;
its old singleton contract did not prove safe overlap with another execution.
No existing task is adopted into a graph, granted authority or rewritten to new JSON defaults.

Revalidate exact schema 6/foreign keys and preserved claim identities before `user_version=6`.
Known rollback/cancellation returns a known interrupted operation; actual Commit or lost
acknowledgement remains uncertain. Keep WAL/FULL, connection pragmas and pre-open private-sidecar
guards unchanged. No filesystem/process/vendor operation runs inside migration or mutation SQL.

API 1 remains. Current 006 execution metadata is strict and advertises actual bounded claims;
maintenance Stop retains exact 005/dev.5 and prior accepted known-owner whitelists. Authentication,
descriptor owner identity and request binding remain exact. A maintenance-only connection cannot
execute tasks, migrate/start another owner, delete a lock or kill a recovered PID.

Graph/node/input/dispatch histories use explicit graph/version/resource-bound cursors, canonical
int64 positions and a captured high-water mark. Maximum 100 items and 4 MiB encoded whole-item pages:
stop before the next complete item would exceed the bound; never clip or skip it. Reuse the
existing exact artifact reader for complete large bodies. Root and node budget/usage projections
name their actual scope; inherited evidence references never duplicate native usage or imply
measured zero tokens, real quota, exact native account selection or cost savings.
Dispatch history is a bounded read, and a materialization read selects the exact receiving
attempt/dispatch. An input ID alone cannot select a materialization because several executions
may legitimately consume the same retained input. Reads never create a dispatch or a receipt.

## Meaningful implementation qualification

Before acceptance, obtain actual RED/GREEN and final platform results for: known-schema 5 migration
with populated history/receipts and a held claim; unknown DDL and before/after-commit faults;
simultaneous last-unit admissions; sibling debit preserving another node's continuation; all
ordinary node Run/004A/retry/grant bypass routes; global/context/captured-and-current account
limits across clients; group/node Stop during input and preparation; stale guidance/contributor
binding; exact lost-ack replay; and restart with accepted plus interrupted nodes. Materialization
qualification must distinguish ancestor Stop before receiver GO from Stop after known first GO:
the former launches no receiver, the latter preserves already-admitted descendant stages under
unchanged accepted input, while own/group Stop and ancestor guidance still deny stale work.
Final-file qualification includes a real zero-own-delta candidate with nonempty graph cumulative
delta, and a separately declared response-only summary that cannot qualify/apply those files.
Materialization
and final-delivery qualification additionally follow the native contract. Portable typed tuples,
cross-compilation and fake model observations are never substituted for actual filesystem/process
results on the six native targets plus the supported race job.
