# Graph execution and public interface contract

Status: proposed006 contract for independent design review; not implemented or qualified.
Exact mutation/view/CLI proof shapes: [operations.md](operations.md).
Companion authority/storage model: [data-model.md](../data-model.md). Physical input/effect
contract: [materialization.md](materialization.md). UI behavior: [interface.md](interface.md).

## Compatibility and unchanged records

The006 owner advertises app2.0.0-dev.6, execution/workflow/graph contracts006-v1 and existing
routing004-v1. Clients require exact current execution contracts/capabilities. Exact known
dev.2–dev.5 owners remain eligible only for their supported owner-Stop maintenance path.
Unknown/future combinations grant no execution or migration authority.

Keep review005-review-v1 and existing same-workflow CandidateBinding/ResponseBinding semantics.
Old task/routing/workflow receipt bytes, saved defaults, failures and windows are unchanged.
Additive graph references do not reinterpret an existing standalone task as graph-owned.

The legitimate root task represents graph intent, owns one budget and real local operations,
and is not a declared worker node. Its ordinary Run route rejects native execution. Node task
membership is server-owned; request omission cannot escape graph controls or create allowance.

## Graph declaration

`POST /api/v1/graphs` accepts a strict whole JSON object:

| Field | Meaning |
| --- | --- |
| `requestId` | Existing canonical mutation ID; exact replay precedes fresh prerequisites. |
| `projectId` | Existing selected project ID. One project per graph. |
| `sharedInstructions` | Optional lossless user constraints applying to all nodes; empty is allowed. |
| `title` | Optional display label, at most160 Unicode characters; never replaces original instructions. |
| `nodes` | One to16 unique node declarations in stable user order. |
| `dependencies` | Zero to64 typed directed edges; omitted means an empty array. |
| `finalNodeKey` | Optional designated final node; must exist and be downstream of every required node. |
| `delivery` | Optional final-only WorkflowDeliveryInput, with existing localMode/externalAction shapes. |
| `routing` | Optional existing RoutingOptions; its budget/continuation apply to the root's shared authority. |
| `expectedPolicyRevision`, `policyOverride` | Existing policy capture semantics; omission uses saved policy. |
| `executionMode` | Existing native/fake distinction; fake is explicitly labelled offline execution. |
| `maxParallelism` | Optional canonical string `1` or `2`, default `2`; narrower capacity still applies. |

A node has `key`, required `originalText`, optional existing `pins`, `acceptance`, `timeoutSeconds`,
`fakeScenario`, node routing overrides and workflow options. Node workflow options allow review,
checks and correction-cycle controls, preserving explicit empty arrays; mode may be omitted or
explicit `auto`, while `author_only` rejects. Node routing
may select suitability/profile/continuation but **cannot contain budget**. Node declarations cannot
contain delivery, external action, adoption, account credentials or a replacement task/graph ID.
Unknown/contradictory nested members reject, including a forbidden member with a null value.

Node keys are case-sensitive ASCII letters/digits/underscore/hyphen,1–48 characters beginning
with a letter or digit; they are
selectors, never filesystem paths. Full instructions use existing lossless task validation.
The complete declaration is bounded to2MiB; required text is never silently shortened.

Each dependency has `from`, `to`, and optional `kind` (`files` by default, or `context`).
Duplicate/self/unknown edges and cycles reject. File incorporation requires actual file edges;
context-only reachability establishes neither changes nor file ancestry.

Every node is required. This slice has no optional/skipped-node success, recursive graph node
or mutable topology. Node guidance versions can change through controlled explicit operations;
original layout/declaration remain immutable.

### Prospective delivery defaults

Without a final node, new graphs visibly capture Separate reviewable results. Local delivery is
review-only and no external action exists. This does not modify standalone preferences.

Selecting a final node can prospectively use the saved local scope, or an explicit final
delivery override. Existing localMode values are `review_only` and `apply_verified_local`. Any explicit delivery object without a compatible final node rejects before
capture/execution; it is never silently downgraded. External scope is always an explicit final
declaration. Intermediate local application and external actions are forbidden.

### Admission result

Return201 with an immutable GraphAdmission containing the graph/definition identity, root task,
stable ordered key-to-node/task mapping, captured authority and initial local preparation operation.
This admits graph intent; it does not claim every child is launched or qualified. Ordinary
observation of the receipt creates no additional work.

Exact retry returns the same canonical receipt before checking changed project files, availability
or current preferences. A conflicting body under the same requestId returns409 without new work.
Known validation failure starts no worker/publisher. Uncertain acknowledgement retains the exact
operation and fences unsafe dispatch; it does not authorize a fresh replacement request.

## Scope, allowance and dispatch authority

GraphBudgetView explicitly contains graphId, budgetOwnerTaskId, canonical TaskBudget,
budgetAuthorityEpoch and bounded nodeConsumption. Counters/revisions/versions/byte lengths are
canonical decimal strings, never rounded public numbers. The complete ledger is explicitly paged.

TaskBudgetView for a graph node has Budget=null, AdoptionRequired=false and GraphBudgetRef with
graphId/budgetOwnerTaskId. It does not return the root TaskBudget as though it belonged to the child.
Graph UI/CLI follows that reference; direct child grants/adoption cannot allocate a new budget.
Every new graph-owned Admission and WorkflowStepAdmission likewise has budget:null plus an
explicit graphBudgetRef. RoutingDecision retains the actual node task ID and scopes its root
balance/revision/window through budgetEffect.graphBudgetRef. Node usage/evidence reads retain
actual node ownership; resolving allowance never redirects measurements to the root. Task and
TaskSummary expose optional GraphRef, including the non-worker root and pending no-workflow nodes,
so clients select graph controls. Server membership guards apply regardless of client metadata.

All native author/review/correction/retry/004A-replacement paths resolve immutable membership
internally and debit the canonical root ledger in the admission transaction. Routing decisions,
launch observations and node debit breakdown retain actual child/attempt identity without
duplicate admissions. Local graph capture/composition/check/application work has zero native
admission units and no fabricated model measurements.

Use the existing first-native-admission window opening. Root preparation has a separate bounded
local ceiling fixed at graph creation plus300 seconds before that window opens; initial local
operations do not renew it. After opening, each local_graph capture/composition operation uses
the earlier of now+300 seconds and the remaining common window. Existing local_check/application
and native receiving preparation retain their captured005/receiving-attempt limits, bounded by
the common window; this rule never silently changes a declared command timeout. Local operations, guidance and new node versions never reopen or refill it.

Graph proofs bind immutable definition/root input, relevant graph/node controls, current accepted
dependency vectors, budget owner/authority epoch and window. Ordinary accounting debits and
read/state revisions do not invalidate unrelated current authority; actual balance is reread
atomically for every admission. Explicit grant/window/control changes invalidate the applicable
proofs. Standalone005 budget-revision proof rules remain unchanged.

Node Run/advance/retry/guidance/recheck/response routes cannot bypass these guards by omitting
graph data. The controller resolves graph membership and current authority before using the
accepted node workflow. Node adoption or author-only downgrade cannot escape graph workflow rules.

## Capacity and owned execution

Default global managed capacity is two, with narrower native-context/account/graph restrictions
checked transactionally. Default native-context occupancy is one. Declared account labels are
not authenticated identities or account-selection capabilities.

Native context and declared-account reservation limits are one; existing disabling/negative
reports can narrow eligibility to zero. The optional captured graph maxParallelism can narrow
global two-slot use to one without adding a new mandatory preference.

For graph native dispatch, target selection/ranking, current capacity validation, reservation,
shared debit and exact dispatch/admission are atomic. Declared-order selection of B therefore
sees A's reserved context and may choose eligible idle Claude under unpinned Auto, rather than
immutably queue both to occupied Codex. No compatible capacity means waiting before native
admission; it is not004A fallback and does not retarget an admitted attempt.

Claims retain captured context/account/link scope and evaluate current declarations too. Unlinking
or relinking cannot free an active captured reservation. Already admitted earlier scopes settle;
new admissions wait when current restrictions are narrower. Unknown external occupancy/allowance
is reported separately from Council's actual claims.

Local operations consume applicable managed capacity without invented native account facts.
Existing held legacy claims remain occupied and conservatively block unknown overlapping scope.
Two hosts cannot each claim the full shared limit.

Engine ownership is per attempt. Stop, fence, cleanup and shutdown enumerate all owned workers.
A singular compatibility activeAttemptId=null never proves zero claims when several are active;
current metadata includes a bounded active set and actual occupied capacity.

Current metadata requires graphContract:"006-v1" and the task_graphs capability alongside the
existing four capabilities. Its capacity is {resourceId:"managed-native",limit:2,
activeAttemptId:null|ID,occupiedSlots:"0"|"1"|"2",activeClaims:GraphClaimView[]}.
Claims are sorted by canonical slot, at most two, with unique slots/attempt IDs; occupiedSlots
is their exact count. The singular compatibility marker equals the sole claim ID when one is
held, and is null when zero or two are held. Every claim uses operations.md's exact nullable
context/account/graph ownership fields. Known dev.2–dev.5 maintenance compatibility retains
its actual one-slot shape; a newer/future contract never gains execution from these old rules.

## Dependencies, input and acceptance

Ready selection is deterministic in declared order among independently eligible nodes. Exact
accepted versions/results, sealed root/input, current authority, budget and capacity precede
admission. A completed author, reviewer answer or unqualified candidate is insufficient.

GraphDispatchBinding links one receiving node/task version and execution to its accepted input
vector, owning epoch, relevant controls and shared budget window. Root-owned composition/input
retention precedes native admission; the receiving attempt's actual materialization observation
belongs only to that exact attempt. InputID alone is not materialization/execution identity.
Subsequent node review/correction/check preparation consumes its current selected node subject
through accepted005 handoff, retaining graph provenance without resetting to original input.
The materialization contract
defines actual files/index/access proof; those physical checks happen outside SQL, followed by
transactional validation of registered facts before preparation and GO.

Other required nodes must have current accepted results before final delivery. The final
cumulative candidate then needs current applicable pre-delivery checks/review and known prior
settlement. Required local/external delivery follows; final-node and graph acceptance follow
actual required delivery. Final acceptance cannot be its own delivery prerequisite.

Group completion requires every current required node and every captured delivery condition.
It distinguishes known successful completion, failure, cancellation, blocked prerequisites and
interrupted/uncertain work. Independent admitted siblings settle without erasing their results.
An unsatisfied branch cannot be hidden behind a qualified final file or accepted response.

Review-only mixed file results and a final accepted summary may complete as separate reviewable
outputs. A response acknowledgement does not qualify/apply their cumulative file delta. Requested
file delivery remains blocked without that qualification. A pure-response group can complete
without a file candidate, application step or fabricated no-op integration.

## Control operations

| Operation | Required behavior |
| --- | --- |
| Advance graph | Explicit current intent; admit only eligible unfinished work under current proofs. Optional rebindNodeIds explicitly authorizes stale dependency inputs to receive derived task versions with dependency_rebind provenance, unchanged originals/pins/policy and no refill. Omission never clones/replays completed work; uncertainty cannot be repaired automatically. |
| Stop group | Durable graph Stop prevents new admission/preparation/effects and requests cancellation of admitted graph work. Preserve completed results and actual cleanup. |
| Stop node | Durable node Stop cancels its current work and later stages and denies descendants before their known first GO. A descendant whose nodeAdmissionId already records actual first GO may finish under unchanged accepted input; ancestor Stop alone does not stale it. Own/group Stop and changed guidance/input remain binding. Preserve independent siblings and historical qualified results. |
| Node guidance | Reuse lossless versioned guidance through graph membership checks; invalidate affected node/descendant input authority, not unrelated sibling progress. No budget refill or automatic completed-work replay. |
| Budget amendment | Graph-scoped exact receipt; existing finite amendment rules. A grant alone starts no work. No direct child budget grants. |
| Wait/read/history | Compact observation with explicit full evidence/input pages; timeout is not completion, acceptance or new admission. |

Every mutation uses a stable requestId and exact target. Relevant optimistic revisions are
control/authority revisions rather than the aggregate state revision changed by sibling progress.
Request replay precedes fresh guards. A stale target reports conflict and retains the captured
retry identity rather than silently switching task/node/version.

Owner restart exposes retained completed/interrupted work and unresolved incidents without
automatically scanning old ready nodes into execution. The later007 recovery feature remains
separate; no recovered PID, missing acknowledgement or old dispatch record grants replay authority.

## Routes and CLI parity

CLI graph files use the same declaration fields. `--project` and `--request-id` may supply absent
transport fields; conflicting file/flag values reject, never silently replace them. A generated
request ID is exposed with the exact retry identity before an uncertain response can be reported.
Whole files/stdin are bounded to2MiB and strictly decoded; no shell or generated command is parsed.

All routes use the existing authenticated local owner and response envelope. Errors/diagnostics
retain existing machine-readable versus stderr separation.

| HTTP under /api/v1 | CLI family | Purpose |
| --- | --- | --- |
| POST /graphs | graphs start --project ID --graph-file PATH\|- | Whole declaration and exact admission. |
| GET /graphs?projectId=ID | graphs list --project ID | Bounded current graph identities/status. |
| GET /graphs/ID | graphs get ID | Current group/detail with bounded node summaries. |
| GET /graphs/ID/definition | graphs definition ID | Complete captured declaration/authority. |
| GET /graphs/ID/wait | graphs wait ID | afterRevision/timeoutMs observation only. |
| POST /graphs/ID/advance | graphs advance ID | Explicit eligible progression. |
| POST /graphs/ID/stop | graphs stop ID | Exact group Stop. |
| POST /graphs/ID/nodes/KEY/stop | graphs stop-node ID --node KEY | Node/descendant Stop scope. |
| POST /graphs/ID/nodes/KEY/guidance | graphs guide ID --node KEY | Current guidance under graph guards. |
| GET /graphs/ID/budget | graphs budget ID | Canonical shared authority. |
| GET /graphs/ID/budget-ledger | graphs budget-ledger ID | Complete bounded ledger. |
| POST /graphs/ID/budget-amendments | graphs grant ID | Finite amendment, no automatic launch. |
| GET /graphs/ID/inputs | graphs inputs ID | Immutable root/composed input records. |
| GET /graphs/ID/inputs/INPUT | graphs input ID --input INPUT | Exact input/access/materialization detail. |
| GET /graphs/ID/inputs/INPUT/members | graphs input-members ID --input INPUT | Full bounded member/origin/blob metadata. |
| GET /graphs/ID/dispatches | graphs dispatches ID --input INPUT --node KEY | Bounded exact receiving executions/observations; input/node filters optional. |
| GET /graphs/ID/dispatches/DISPATCH | graphs dispatch ID --dispatch DISPATCH | Exact immutable admission plus its append-once receiving-attempt materialization observation. |

Dispatch lists use exact optional inputId and nodeKey query selectors; CLI --input and --node map
to those fields without substituting latest inputs or node IDs. Internal operations are graphs.start,
list/get/definition.get/wait/budget.get/budget.ledger, inputs.list/get/members, dispatches.list/get
and the corresponding exact mutation names. These names are controller dispatch, not vendor tools.

Input views link exact dispatch/receivingAttemptId records; never label the latest materialization
of one retry as proof for another. An absent observation is explicitly not observed, not complete;
foreign dispatch/input/attempt combinations reject. Member provenance remains bound to the input.

Node task/workflow/candidate/artifact reads and eligible controls reuse existing routes with
mandatory internal graph authority. Historical node selection identifies task version/execution;
group current controls remain distinct from the selected historical response producer.

Lists default to30/max100 whole items, with explicit byte bounds and typed canonical cursors bound
to graph/project/selector/high-water. Oversize whole items return an explicit readable gap, never
silently disappear. Graph detail has at most16 node summaries/64 edges and no full transcript.
Full originals and bytes use explicit retained-artifact reads. Reject unsafe numeric selectors,
foreign input IDs, malformed cursors and unknown fields before any mutation commit.

## Required observable negative cases

Invalid declaration or intermediate publisher starts zero workers/effects. Missing or corrupt
closed input blocks receiver preparation. Current guidance/Stop/budget/account changes deny stale
admission. A sibling debit alone does not stale another node's valid next stage. A later accepted
result cannot silently replace an admitted receiver's immutable input. Conflicting fan-in retains
both outputs; pure responses do not become patches. Uncertain materialization/delivery retains
actual evidence and fences without rerun. Every claim remains source-specific offline/native
qualification, never proof of live login, quota, price or savings.
