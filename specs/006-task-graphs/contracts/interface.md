# Task graph UI and CLI contract

Status: proposed feature 006 design, requiring independent review before the design/tasks gate.
This document describes planned behavior, not implemented or qualified capabilities. No model
planner, new adapter, runtime UI dependency or implementation is authorized by this document.

[graphs.md](graphs.md) and its normative [operation shapes](operations.md) own exact wire fields,
route names, CLI syntax, validation bounds and mutation proofs. [data-model.md](../data-model.md)
owns persisted identities and authority. The
display meanings below do not introduce a competing set of wire states or endpoints. Every UI
and CLI action uses those common owner operations; neither client is a second scheduler.

## 1. Small optional composer

**UI006-01 — Standalone remains the normal path.** Keep Tasks, Agents and Preferences navigation,
the existing project selector, standalone task draft, saved drafts and Auto Start behavior. Add
a quiet **Create task group** entry under Tasks. Opening or closing it preserves standalone
text, settings, pending requests, selection and saved-draft bindings. Starting a graph does not
write a generated plan into the standalone draft.

The new small `GraphsView` has its own project-scoped editor and pending-operation state. It
initially shows two node instruction fields, **Add task** and **Start group**. Only the selected
project and nonempty node instructions are necessary. A graph can contain 1–16 nodes and 0–64
edges; the initial two fields are a convenience, not a new minimum. A graph title and shared
instructions are optional. Automatic display labels are labelled previews and never replace
the full submitted originals. Existing node keys stay stable as nodes are edited, reordered or
removed; a removed key is not silently reassigned to a new node.

Each node has an optional **Depends on** section listing the other nodes by readable label. An
edge explicitly says **Use verified files** or **Context only**. The default edge is file-change
input; context-only input never promises files. This is a keyboard-operable list of controls,
not a required drag-and-drop canvas. Adding dependencies needs no stage/model/member wizard.

Removing a node must expose any now-invalid dependent references for correction, rather than
silently reconnecting dependencies or discarding their meaning. Whole-declaration validation
identifies the affected node/edge, retains all editor contents and starts no child work. Duplicate
keys, missing endpoints, cycles, excessive limits and contradictory delivery use the same owner
validation and actionable errors through UI and CLI.

## 2. Visible prospective scope

**UI006-02 — A new graph visibly defaults to Separate reviewable results.** This captures fresh
review-only group scope and leaves the saved standalone delivery preference unchanged. The
composer states that separate results cause no local application or external delivery.

An optional **Final output** selector chooses an existing node. Selecting one may prospectively
use the saved local delivery preference; show the resolved mode before Start. A supported local
override and explicit external declaration remain optional deeper controls. Merely selecting a
final node never creates publication authority. Removing/changing it cannot silently retain an
incompatible effect scope: show the contradiction and require correction. Explicit delivery
without a compatible final node rejects before work. Intermediate local application and external
actions are forbidden, including declarations hidden in reused task options.

Before Start, show a concise summary of:

- Effective saved autonomy and any explicit narrower override.
- **Up to two workers**, subject to actual context/account/graph capacity; no promise of two
  available models or accounts.
- The actual saved finite allowance and common window for the whole group, including native
  author, review, correction and retry admissions. Do not hardcode a fresh allowance per node.
- Separate reviewable results, or the selected final node and captured local/external scope.

Optional routing pins, acceptance conditions, checks and bounded authority overrides reuse the
accepted task concepts. Graph-owned expenditure and delivery cannot be overridden through a
child option. No compulsory model call selects the layout, waits, prepares a digest or presents
this summary. All declared nodes are required; no skip/optional-node control is offered.

## 3. Overview and existing task detail

**UI006-03 — Ordered cards are the complete graph navigation.** Use a project-scoped graph list
and selected overview under Tasks. Each overview contains all bounded node summaries and typed
dependency labels. Desktop may place the selected node detail beside the list; 360px uses the
same ordered content in one column. A diagram may be supplementary later, but is not necessary
for creation, understanding dependencies or keyboard operation.

The group header shows the actual aggregate outcome, current next action/owner, running/waiting
counts, shared allowance/window and **Stop group**. Node cards show the stable label, current
version, dependencies, actual phase, specific blocker and **Open task**. A start acknowledgement
can mean admitted or preparing; it must not be rendered as an observed worker launch. A successful
final node never hides an unfinished or failed required sibling.

**UI006-04 — Reuse the task/workflow evidence experience.** A node opens its ordinary task detail,
with a **Part of task group** link and explicit graph/node membership. Reuse complete original
and guidance reads, attempt history, workflow steps/reviews/checks/findings, selected response or
candidate, exact artifact-byte reads and downloads. Separate the selected deliverable producer
from the current controlled execution. A supporting review answer is not the node's deliverable.

Current `Task` and `TaskSummary` responses for known graph members carry `GraphRef`, including
its root/node kind, exact graph/root task and nullable node identity as defined in operations.md.
The legitimate root is graph intent and local evidence ownership, not another native worker.
Root and pending no-workflow node views show their graph link and suppress ordinary standalone
Run, workflow adoption and task-budget grant controls. A missing/mismatched ref on a known graph
member is a view error, never permission to fall back to standalone controls. Historical receipt
bytes and standalone task responses are not rewritten to add graph fields. Server membership
checks remain mandatory even when a request omits graph metadata.

Historical node/task/workflow selection stays visibly **Historical; read only**. The current
group's Stop remains separately identified and available when eligible; no historical button
rearms work. Returning to current state is an explicit read, not an execution action.

## 4. Truthful display states

**UI006-05 — Display meanings must remain distinct.** The owner supplies authoritative state,
current controls and next action/owner using the graph wire contract. Clients may format those
facts but do not infer acceptance from process success, compose their own dependencies or choose
a replacement target. Use text and accessible names as well as colour.

| Observed meaning | Required presentation |
| --- | --- |
| Local unsent group | Draft / Not run; saving browser recovery state is not durable owner admission. |
| Submission pending or uncertain | Identify the captured group request; offer its exact retry while retaining newer local edits. |
| Source/input preparation | Preparing source or inputs; identify the actual local operation and any failure without claiming a native launch. |
| Capacity wait | Waiting for capacity, with the actual context/account restriction or unknown observation. |
| Dependency wait | Name the prerequisite and missing current acceptance; an author exit alone is insufficient. |
| Composed input ready | A sealed composed input exists. Receiver admission, workspace creation and materialization remain separately observed; no receiving workspace is implied. |
| Files materialized for receiving attempt | Show only after an actual receipt binds this input, exact receiving attempt/dispatch and receiving basis. Another execution or earlier retry's receipt cannot qualify the selected attempt. |
| Conflict/context blocker | Name the affected paths, incompatible inputs or unavailable required access; retain both results and a specific next action. |
| Budget/window blocker | Show shared remaining authority or expiry; no per-node refill or automatic resume claim. |
| Native/local work active | Label the role and executor; local capture/check/application consumes no native admission but is not measured zero-token/zero-charge model work. |
| Stop accepted | Cancellation requested; show admitted work and actual output/process/cleanup settlement separately. |
| Failed, cancelled or interrupted | Retain the authoritative outcome, useful evidence and independent sibling results. Unknown cleanup/effects remain explicit. |
| Separate qualified outputs | Reviewable results; distinguish each node's accepted files/response and do not imply combined qualification or integration. |
| Response ready/accepted | Bind acceptance to the complete exact response and its conditions. Pure-response groups can complete without a file candidate. |
| Mixed response/file result | A review-only group may complete with separately qualified files and an accepted summary. Response acceptance cannot qualify or apply the cumulative files. |
| Requested file delivery blocked | State that the response is accepted but cumulative file qualification/delivery is still required; identify the remaining action. |
| Final delivery | Distinguish review-only, actual applied local change and external-command result. Partial/uncertain effects never become completed delivery. |

Final delivery eligibility is not final acceptance. All other required nodes must have current
accepted results, and the final cumulative candidate must have its required pre-delivery checks/
review and known prior settlement. Required delivery then precedes final-node and group acceptance.
Context-only reachability cannot prove incorporation of a file branch.

## 5. Inputs, context and provenance

**UI006-06 — Input detail belongs to an exact receiving execution.** Provide a compact **Inputs
and context** view linked from the node. Its immutable selector and owner response identify:

- The graph definition/basis, receiving node/task version and actual receiving execution/input.
- Each contributing predecessor version, accepted result/producer and file-change or context-only
  edge; shared ancestors are attributable once.
- Retained manifest/index/staged-blob and evidence bindings, composition result and actual
  materialization/access status. A path or digest alone is not availability proof.
- Specific conflict paths or missing, corrupt, inaccessible or excessive required material.
- Labelled derived context, its exact sources and supported access to complete originals.

Routine overview reads remain compact. Member lists, full manifests, originals and raw evidence
use bounded pages/sections or the existing exact-byte reader; previews never replace retained
data. Failure to read more preserves loaded content, the exact requested selector and its retry.
Peer prose remains attributed data and cannot add execution, account, payment, budget, tool or
publication authority. Do not display a generated digest as original user instructions.

Every node has a `GraphConditionsBinding`, not only the final node. Present graph-shared user
instructions and the receiver's own original/guidance/acceptance as authoritative in their declared
scopes. Present `upstream_local` obligations as complete attributed predecessor-local context;
they are not automatically final-state requirements or receiver permissions. Scope comes from
the captured declaration, never model extraction or prose heuristics. The shared-instructions
original contains only exact `sharedInstructions`, including explicit empty text; it is not the
whole declaration, title or sibling instructions. A digest never substitutes for these originals.
A node-only literal proof cannot qualify an additional unassessed shared condition: show the
actual review/blocker rather than silently marking the whole scope verified.

## 6. Explicit input rebinding

A stale workflow input exposes **Use updated predecessor inputs and advance** only for eligible
explicitly selected stale nodes. The control shows which node versions/inputs will be rebound
and that Advance may admit eligible unfinished work under the captured graph authority; it is
not a passive refresh. An ordinary refresh, Run, guidance save or retry never opts into rebind.

Capture `rebindNodeIds` explicitly with the complete current node authority vector and other
Advance proofs from operations.md. Omission means the empty rebind set. The acknowledged
`GraphReboundNode` records the actual old/new TaskVersion and `dependency_rebind` provenance;
show those versions and preserve links to earlier input/history. Do not replace immutable
workflow authority in place or imply that unchanged originals, guidance, pins or policy changed.
An unstarted node without workflow authority may capture its first input without a derived
version. Rebinding adds no budget/window and cannot escape Stop/fence or replay unchanged completed
work. The receipt's rebound versions and separately observed preparation/dispatch records must
not be represented as proof that a worker already launched.

An uncertain rebind/Advance retry preserves the exact selected IDs, authority vector and request
body. A newly accepted predecessor or newer local selection cannot silently extend that set.

## 7. One budget owner and distinct Stop controls

**UI006-07 — Shared ownership is explicit.** A graph child `TaskBudgetView` carries
`GraphBudgetRef`; its ordinary `budget` is explicitly JSON `null` (never omitted), and
`adoptionRequired` is `false`. It must never return the root budget disguised as a budget
belonging to the child. Preserve the existing selected-task ID check whenever an ordinary task
budget is present.

This projection also applies to every NEW graph-owned public `Admission` and
`WorkflowStepAdmission`: `budget:null` plus the exact `graphBudgetRef`, with actual child task,
workflow, step and attempt identities. Do not silently accept a root TaskBudget as child budget
or infer a fresh allowance from a nullable field. Graph-owned routing decisions likewise require
`budgetEffect.graphBudgetRef`; balances/revisions/window describe that referenced root while
admission units describe the actual child attempt. These additive new graph projections do not
change standalone omission rules or old serialized receipts. Scope/budget errors expose the
contract's graph or budget owner reference as an actionable link, not replacement authority.

Resolve the reference through `GraphBudgetView`, validating both graph and budget-owner identity.
Render one group allowance/remaining balance, current common window and bounded per-node
consumption breakdown. Retain the authority epoch in binding proofs and complete authority
records rather than making it a routine setting. Node consumption is **included in the group total**; never sum
the root total and node breakdown as additional expenditure. A node detail links to **Shared
group budget** instead of offering child adoption/grant controls. Exact counters/revisions remain
strings; comparisons use exact counter semantics, not unsafe JavaScript numbers.

Only group-level amendments may grant additional authority or request a supported new window.
Capture the current graph/budget proofs from the wire contract. Acknowledging a grant alone
starts no work, restores no spent units and rearms no old proof. The next eligible explicit
workflow action remains separate. Ordinary sibling debits are accounting observations, not a
reason to treat unchanged sibling authority as a user amendment.

**UI006-08 — Stops name their scope and captured target.**

| Control | Target and observed consequence |
| --- | --- |
| Stop group | The current graph/control identity. Prevent later admissions/effects and request cancellation of admitted work; retain completed results and expose actual cleanup. |
| Stop node | The current graph node/control identity. Cancel that node's own work/further stages and deny descendants before their known first GO. A known-started descendant may finish under unchanged accepted inputs; its own/group Stop and changed guidance/input still bind. Independent siblings retain their own outcomes. |
| Inspect historical execution | Read only. It never selects a different Stop target or changes a current graph/node control. |

Use captured node/group controls rather than substituting the existing weaker attempt-only Stop.
Any direct task/attempt entrypoint must still obey graph ownership, Stop and shared authority.
Known Stop receipts survive late success, lost replies and a user selecting another node. No
Pause/resume/checkpoint control is introduced by this feature.

## 8. Pending operations and observation guards

**UI006-09 — Preserve exact intent across every asynchronous boundary.** Reuse the accepted
pending-request/session and read-generation patterns, with graph-specific state:

1. Keep graph editors separate from standalone editors. Key local recovery by project/editor
   identity and retain stable node keys plus an edit generation. Browser recovery is best-effort;
   a denied/full cache is not a durable save or admission acknowledgement.
2. At a click, freeze the operation, complete body, request ID, target graph/node/task versions,
   authority proofs and relevant editor/selection generations. Never rebuild an uncertain request
   from newer fields, current defaults or another selected node. Retry the exact wire operation.
3. Pending keys distinguish group and node operations and their captured identity. A pending
   operation for graph A cannot become an operation on graph B because selection changed.
4. Clear an exact acknowledged pending entry regardless of current selection. Refresh its captured
   project's list as appropriate, but paint success, select a result or reconcile editor text only
   if the captured project/graph/node/history and edit/selection generations still match.
5. Guard every read and every continuation after an awaited workflow, budget, input, artifact or
   owner read. Reject mismatched identities and older exact state/version counters before paint;
   repeat the guard after later awaits. A delayed same-node read cannot rewind newer guidance.
6. Keep graph-list, selected-detail, node/task, historical selector and paginated-resource read
   generations distinct. A refresh must not retarget an in-flight complete evidence read.
7. Graph observation follows the owner's continuation/settlement facts. A single child terminal,
   or transient blocked/Council-owned transition, does not end observation while graph work or
   cleanup is still progressing. Reads never launch, grant, retry or advance work.
8. Switching project/selection or disconnecting can end client observation without sending Stop.
   Reconnection reads retained state; restart never automatically retries a write or worker.
9. Surface unresolved operation identity and an exact retry action without dumping private
   prompts, raw security metadata or credentials into routine status messages. Retain newer draft
   text, selected evidence and the captured request after a read failure.

## 9. UI/CLI capability mapping

**UI006-10 — One operation has one meaning in both clients.** Use the exact command/route names
and DTOs from [graphs.md](graphs.md). This table defines presentation and required semantics,
not an additional API or CLI grammar.

| Capability | UI presentation | CLI behavior |
| --- | --- | --- |
| Start declared graph | Start group with the captured form | Bounded strict declaration file/stdin plus project and request identity; preserve the exact original declaration. |
| List/detail/history | Group list, ordered nodes and explicit historical selector | Compact bounded reads with stable graph/node/version identities and the same current/historical distinction. |
| Wait/refresh | Read-only overview observation | Bounded wait returns current identity/revision; timeout or transport loss does not cancel work or imply completion. |
| Inspect node | Open existing task/workflow detail | Existing task/evidence operations retain graph membership and shared owner. |
| Input provenance | Inputs and context; explicit complete reads | Exact receiving-input/execution selector and bounded full records; no implicit latest-result retargeting. |
| Stop | Distinct Stop group and Stop node | Separate captured scope; exact replay reconciles that target instead of selecting a newer execution. |
| Amend authority | Shared group budget controls | Group-only amendment; child grants/adoption cannot escape shared authority. |
| Guidance/advance/check/response acceptance | Existing eligible workflow controls with graph ownership | Reuse common validated operations and graph proofs; neither client creates an unchecked alternate lifecycle. |

CLI machine-readable output retains the common envelope/receipts and lossless string counters;
diagnostics remain separate. Request conflicts identify the conflict rather than issuing a fresh
request automatically. UI and CLI must expose the same blocker, actual result, next owner/action
and complete evidence, while provider/model/account/token/charge unknowns remain unknown.

## 10. Approved appearance and interaction

**UI006-11 — Extend the current approved surfaces.** Reuse the vendored Ejupi Labs dark-colour
logo/symbol, existing generic icons and retained notices specified in
[design-identity.md](../../../docs/refactor/design-identity.md). Use the current Night/Room,
Chalk/Pear/Lilac/Muted/Error tokens, system reading font and 12px panel geometry. No new Council
logo, unrelated icon family, font download or runtime framework is needed.

Retain 44px controls, visible 3px focus, meaningful accessible names, labelled fields, text
outcomes and polite status announcements. Keep the full logo legible and decorative icons hidden
from assistive reading. Node cards and facts wrap IDs, paths and long English/original labels
without horizontal page overflow. At 360px, controls stack, selection remains explicit and both
Stop scopes remain reachable without a canvas or hover-only affordance. Adding/removing a draft
node deliberately manages focus; polling never steals focus or announces every unchanged count.
Reduced motion removes animation/transitions and forced smooth scrolling.

## 11. Actual qualification record

**UI006-12 — Capture real state and separately verify behavior.** Extend the accepted offline
owner/worker/browser journey and capture tooling during implementation. Screenshots alone do not
prove overlap, access, Stop, race or filesystem semantics. Each scene links the current source/
dist/served assets and actual local graph/node/attempt/input/effect identities used by its checks.
Fixed fake workers are labelled; no live provider, login, allowance, savings or charge is qualified.

Capture desktop (1440px) and narrow (360px) views, pairing each applicable scene below. Record
actual viewport and whether an image is a named complete element or a viewport; do not crop away
the control or blocker being qualified.

| Scene | Required actual assertions and visible evidence |
| --- | --- |
| Standalone baseline | Existing minimal Auto composer, saved preference and typed draft preserved when graph UI opens/closes. |
| Minimal graph | Two independent instruction fields, Separate reviewable results, actual saved shared bound and up-to-two-worker summary; no compulsory advanced input. |
| Dependencies/final output | Typed file/context edges, final-only scope and visible whole-declaration errors; retained draft and focus after invalid cycles/limits/removal. |
| Concurrent overview | Two actual compatible owned workers overlap; distinct workspace/input/attempt identities and one shared budget. |
| Waiting | One-slot/account capacity restriction and separate unqualified-prerequisite wait, even after an author process completed. |
| Input/context detail | Actual composed/materialized files, diamond/refinement provenance, derived digest and available complete originals; unrelated histories absent. |
| Conflict/access | Real incompatible fan-in and missing/corrupt/excessive required input block before receiver native preparation; both useful results remain inspectable. |
| Budget/window | Exhausted/expired shared authority; group grant acknowledgement starts no stage and child controls cannot refill allowance. |
| Stop group | Active plus waiting work; captured Stop acknowledgement and actual process/output/cleanup settlement with prior results preserved. |
| Stop node | Its worker cancelled and waiting descendant prevented while independent admitted work settles under its own outcome. |
| Responses and mixed results | Pure-response completion without candidate; mixed review-only completion; requested-file-delivery blocker despite an accepted summary. |
| Final delivery | Qualified cumulative local application preserving unrelated source/index, distinct review-only outcome and actual partial/uncertain effect presentation. |
| Current/history and graph ownership | Read-only historical node/input evidence versus current controls/producer; actual root and pending no-workflow node TaskSummary/Task GraphRef suppress standalone Run/adoption/grants, and all graph admissions expose budget:null plus the correct shared reference. |
| Explicit rebind | Changed accepted predecessor input stales a captured workflow; omitted rebind IDs do not mutate/relaunch it. Explicit named rebinding yields a new derived TaskVersion/provenance without refill, preserves old history and rejects Stop/unchanged-completed replay. Held rebind/Advance reply cannot capture newer selected IDs or proofs. |
| Pending/late replies | Actual held owner replies/reads reproduce selection changes, newer typing and same-node guidance; exact receipts reconcile without repaint/duplicate work. |
| Layout/accessibility | Long originals/IDs/paths and maximum supported node/edge controls remain usable at 360px; real Tab focus, accessible names, reduced motion and approved asset hashes. |

Keyboard checks cover composer entry/exit, node creation/deletion, dependency kind, final output,
Start/retry, node navigation, complete input/evidence reads, both Stop scopes, group authority
amendment and current/history selection. New UI source changes require matching compiled browser
assets. Independent review must inspect the actual paired PNGs; automation counts alone do not
close visual acceptance. Serial suites, package journeys and matching native/race qualification
remain the feature-wide gates in the plan.

## 12. Existing source seams and traceability

The design reuses, rather than bypasses, the accepted implementation patterns:

- `ui/src/workspace.ts`: project/owner wiring and separate view callbacks.
- `ui/src/drafts.ts`: captured edit intent and acknowledgement-safe reconciliation patterns;
  graph editor state remains a separate owner, not a repurposed standalone draft.
- `ui/src/tasks.ts`: exact pending Start/control, task/attempt/evidence selection, generation and
  exact-version guards, full response reading and acceptance.
- `ui/src/workflow.ts`: current/historical workflow guards, pending mutation acknowledgement and
  stage/review/check/finding/delivery evidence readers.
- `ui/src/routing.ts`: routing observations, exact budget counters and the selected-task budget
  identity guard; graph-owned presentation must be explicit.
- `ui/src/api.ts`, `ui/src/style.css`, `ui/src/index.html`: common owner envelope, verified writes,
  session recovery, accessible branded components and responsive/reduced-motion behavior.
- `internal/cli/{execution,workflow,routing}.go` and `internal/httpapi/`: one validated owner
  operation model, strict input, exact request identity and bounded complete reads.

| Contract items | Specification coverage |
| --- | --- |
| UI006-01–02 | GRA001–006/016/020/023; US1 and US4.5–6; SC001/006/008/009. |
| UI006-03–05 | GRA007–008/013/017–023; US1–2/4; SC001–004/007–009. |
| UI006-06 | GRA009–016/022; US2–3; SC002–005/009. |
| UI006-07–10 | GRA004–007/016–022; US1/4; SC006–009. |
| UI006-11–12 | GRA023–024; SC009–010 and feature-wide independent/native qualification. |
