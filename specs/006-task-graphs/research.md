# Task graph design research

Status: source-backed006 design decisions. Independent design closure and the cross-artifact
analysis gate are recorded separately. These interfaces do not claim implemented capabilities.

## Existing accepted boundaries

Accepted005 source is794f080, acceptancec5b1801. Core inspected admission/budget/schema/active
ownership; Native inspected actual handoff, manifest/index, artifact and application code; the
platform reviewer inspected existing UI/CLI/pending/budget behavior. No live providers or vendor
state were inspected. No new dependency is required by this research.

- Capacity currently has one `managed-native` claim; Engine has singular active ID/cancel fields.
  Existing code does not supply concurrent sibling execution merely because workspaces differ.
- `workspace.Handoff` reconstructs one producer's retained manifest/index and verifies inventory.
  Same-workflow candidate and artifact ownership checks cannot be removed to permit graph input.
- A raw index preserves object IDs, not necessarily closed bytes for different staged content.
  Graph closure must retain and verify staged blobs rather than substituting working-file bytes.
- `PrepareApply` already separates the source basis from selected candidate/producer. This is the
  proper native seam for graph-root-to-final delivery; candidate ancestry must stay node-local.
- Existing workflow continuation proofs bind budget revision, which changes on each admission.
  Sharing a budget without distinguishing authority from accounting would invalidate sibling
  proofs after ordinary debits.
- Existing RoutingView rejects a TaskBudget belonging to another selected task. A root graph
  budget must have an explicit owner/ref, not masquerade as a fresh child budget.

## R1 — Keep the accepted local architecture

**Decision**: Extend Go owner/SQLite/evidence and authored TypeScript UI. Keep pinned dependencies
and shell-free native process/workspace abstractions.

**Rationale**: The new work is durable orchestration/provenance around accepted execution. Native
cross-platform boundaries and full offline release qualification already exist.

**Alternatives**: A separate remote scheduler, another runtime or model-based planner adds state,
deployment, cost and recovery boundaries without solving a requirement of this bounded local slice.
No language rewrite or extra runtime framework is justified.

## R2 — A real graph-intent root owns budget and local evidence

**Decision**: Introduce a typed graph root task representing the programmer's group intent. It is
not one of the declared worker nodes. It owns the single existing canonical TaskBudget/ledger
and real `local_graph` capture/composition attempts. Those perform actual bounded local work,
own artifacts through existing attempt foreign keys and consume zero native admissions.

Every child task has immutable graph membership. Central budget-owner resolution applies to
all admission, routing/launch ledgers, continuation, retry, reads and amendments. Direct ordinary
root execution or a fresh child budget grant is rejected with the actual graph owner.

**Rationale**: This preserves meaningful source/artifact ownership and avoids duplicate allowance
or invented authored candidates. A stopped/failed local operation retains its real result.

**Alternatives**: Per-child default grants violate the group bound. A separate artifact owner type
would widen proven source/evidence foreign keys. Reusing the first child's native attempt as the
group owner conflates independent outcomes and makes root evidence depend on a worker's success.

## R3 — Separate expenditure accounting from authority

**Decision**: Graph continuation carries an explicit graph budget owner, control/authority revision
and common window. Ordinary sibling debits update accounting/balance without invalidating another
node's current authority proof. Explicit budget/control changes invalidate the affected authority;
every admission still rereads actual remaining balance/window atomically.

Standalone005 proof.BudgetRevision semantics and old canonical receipts stay unchanged. Root-budget
resolution must reach helper code that records the admission/launch ledger with the child ID today.

**Rationale**: A shared balance must serialize consumption without making legitimate unrelated
progress stale. An old balance snapshot can never authorize overspending.

**Alternatives**: Ignoring all budget revisions admits stale grants/controls; binding every child
proof to the shared accounting counter stalls otherwise valid concurrent work.

## R4 — Explicit graph input, not cross-task candidate impersonation

**Decision**: Retain GraphRootBinding, AcceptedContribution and GraphInputBinding with exact source,
acceptance/producer, manifest/index and context/provenance references. GraphInput is receiving input
evidence, not an authored candidate or acceptance. Real graph-local operations own assembly artifacts.

A new graph materializer verifies the closed cross-task inputs, reconstructs the receiving workspace
and exact index, inventories it, and publishes a receiving-attempt-owned snapshot/materialization
receipt. Existing005 Handoff and candidate ownership guards remain unchanged.

**Rationale**: Required files become actually available under the receiver's capabilities. Original
checkout edits and mutable old workspaces cannot substitute for retained selected inputs.

**Alternatives**: Passing only paths/prose fails availability. Replacing candidate root IDs with a
foreign graph root weakens existing same-workflow authority. Overlaying full snapshots loses disjoint
changes when one branch carries another's old unchanged file.

## R5 — Causal changes and closed staged content

**Decision**: Compare actual node input to accepted node output. Track working-file and index slots
separately, including deletion, value/type/mode, producing acceptance and inherited origins. At
fan-in, deduplicate inherited effects, allow a proven descendant refinement to supersede its own
ancestor and reject divergent maximal contributions. A context-only edge carries no file ancestry.

Retain bounded staged-blob artifacts with their expected object IDs; working bytes cannot replace
different staged bytes. Verify/reconstruct required Git objects and the receiver's private index
before native preparation. Preserve user branch, index and working files; any supporting object-store
materialization is content-addressed and verified, not an overwrite of user source or refs.

Apply existing100,000-entry/64MiB-file/512MiB workspace bounds, explicit staged/provenance bounds
and at most16 contributing node origins per slot. Unsupported/path/native-name conflicts stop before
a receiver model, with both results retained. No automatic text or Git merge is introduced.

**Alternatives**: Full-root deltas alone cannot distinguish a sibling carrying A unchanged from B
intentionally refining A. Last-writer selection hides conflicts. Object-ID existence without byte
verification cannot prove staged evidence survives source changes or missing objects.

## R6 — Graph proof around existing safe final application

**Decision**: Keep node candidate.RootSourceSnapshotID local. A GraphDeliveryBinding proves current
graph authority, exact original root, other-node acceptances, typed file incorporation and final
pre-delivery verification. The existing application helper receives verified GraphRoot as Source
and the actual final candidate/producer as selected output. Existing journal/action permits, native
metadata checks, quarantine and mandatory Close remain; each effect also rechecks graph controls.

Intermediate local and external actions are forbidden. Other required nodes accept first; final
candidate verification then permits scoped delivery; final node/group acceptance follows actual
required delivery. A response acknowledgement does not qualify retained files. Review-only mixed
results and pure-response completion remain distinct from applied cumulative output.

**Alternatives**: Node-local source-to-final application omits predecessor changes; applying every
intermediate and then the full cumulative delta risks reapplication/conflict. Rewriting candidate
ancestry lies about node creation and breaks005 invariants.

## R7 — Deterministic focused context with actual access

**Decision**: Preserve full authoritative graph/node originals and applicable contributed conditions;
label deterministic digests as derived and link exact originals. Supply scoped source/evidence and
an actual supported inline/file retrieval plan, with a materialization/access receipt. Unrelated full
histories are omitted. Missing/corrupt/oversize required material blocks or uses an already permitted
supported alternative, without a planner/summarizer call.

Keep existing256KiB prompt and64 supplied-reference guards. These are Council byte/reference bounds,
not a tokenizer or proof of a provider's context capacity. Unknown native model telemetry stays unknown.
Peer content remains attributed evidence and creates no execution/payment/publication authority.

**Alternatives**: Full-transcript broadcast wastes context. Unlabelled summaries lose originals and
constraints. Treating a supplied path as read access or a character count as tokens invents evidence.

## R8 — Minimal separate-results default and explicit scope

**Decision**: New Create task group opens two instruction fields under the selected project. Ordinary
standalone composer stays intact. Defaults visibly say Separate reviewable results, up to two workers
and the actual shared allowance/window. Optional depends-on edges distinguish verified files from
context only. Selecting an existing final node can prospectively use saved local delivery; explicit
delivery without a compatible final rejects. External declaration is always explicit/final-only.

Use ordered cards, current/history/input evidence, Stop group and distinct Stop node. Graph drafts,
pending exact requests and read generations stay separate from standalone draft/selection. Budget UI
names the graph owner and node debit breakdown rather than casting a root budget as a child grant.
Reuse approved assets, English,44px controls, visible focus and reduced-motion behavior.

**Alternatives**: A required drag canvas/model planner or multiple setup forms adds mandatory work.
Silently inheriting apply scope without a final output is unsafe; silently changing an explicit
requested delivery into review-only is misleading. Response acceptance and file qualification cannot
be represented by one generic completed badge.

## R9 — Qualification remains source-specific

**Decision**: Use actual owned offline workers for overlap, staged/working fan-in, causality,
context access, current controls, finite budget and cumulative delivery. Keep pure policy and metadata
tests distinct. Complete UI/CLI and extracted journeys, independent source/failure/render review,
required local commands and seven actual hosted jobs at one checked revision before007.

No authored scenario, queued CI job, prior005 pass, compiled foreign target or successful agent
response substitutes for006 terminal evidence. Live provider/account/quota/savings claims remain
unperformed unless separately authorized.

## R10 — Immutable node workflow capture and explicit dependency rebind

**Decision**: Capture a node's immutable workflow authority only after its sealed input and
receiver-scoped GraphConditions exist, using graph-creation defaults rather than later preference
changes. A pending child has no standalone adoption requirement. Once captured, changed dependency
input cannot replace that authority in place. Explicit Advance rebindNodeIds creates a derived
TaskVersion with dependency_rebind provenance and unchanged originals/pins/policy, without units
or a new window. Ordinary Run reports graph_input_stale rather than silently choosing a new version.

**Rationale**: Existing workflows are unique per task/version and their accepted authority is
immutable. Explicit versioning preserves attributable historical execution and the user's intent.

## R11 — Cumulative proof without invented final edits

**Decision**: Accepted005 already retains zero-own-delta candidates. Only the designated file-final
may use internally verified GraphFinalPlanFacts to pass the initial empty-own-delta eligibility
check when its graph-root cumulative delta is actually nonempty. Every remaining fresh cumulative
check, scoped condition, review, finding, settlement and delivery guard remains. Node creation
flags remain zero; graph cumulative changes have separate fields.

All receivers bind graph_shared and receiver authority separately from attributed upstream_local
context. The shared-instructions artifact contains only the exact captured shared field, not
unrelated node instructions. Effective condition hashes are sealed before workflow capture, with
no candidate-to-condition hash cycle.

**Rationale**: Integration can be useful without a dummy edit. Inherited bytes and an unchanged
candidate are a verification basis, never a passed verdict or response-to-file promotion.

## R12 — Complete public ownership and Stop boundaries

**Decision**: Task/TaskSummary expose server-derived GraphRef for root and node members. New
graph-owned admissions expose budget:null and graphBudgetRef; decisions keep actual node task IDs
and explicitly scope balance/window fields to that root. Usage/evidence remains node-owned.
A nodeAdmissionId anchor records actual known first GO. Ancestor Stop denies unstarted descendants;
after known GO, Stop alone cannot stale unchanged accepted input. Own/group Stop still guards
every later stage/effect, and changed guidance/input remains relevant.

**Rationale**: Public projections must not create a child allowance or conceal pending membership.
A completed ancestor's Stop can prohibit future descendants without rewriting an already-started
descendant or borrowing a started marker for another version.
