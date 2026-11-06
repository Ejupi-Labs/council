# Feature Specification: Dependent and Parallel Task Graphs

**Feature Branch**: `refactor` (physical branch; feature context `006-task-graphs`)

**Created**: 2026-11-06

**Status**: Specification reviewed; ready for design

**Input**: Deliver the next roadmap slice: programmers coordinate independent and dependent coding
tasks across suitable configured agents, share bounded expenditure, and give followers actual
verified predecessor files and focused evidence. Preserve standalone Auto, saved autonomy,
useful delivery and the approved Ejupi Labs interface.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Use available agents for independent work (Priority: P1)

A programmer adds two independent changes to an optional group and starts them under saved
policy. Useful work overlaps, including through different compatible harness/account capacity.
Ordinary standalone task creation remains unchanged.

**Why this priority**: This addresses leaving a subscription idle while another agent works.
Scheduling and observation must not spend additional model calls.

**Independent Test**: Observe two actual controlled offline workers overlap with distinct
isolated workspaces and retained results, preserving the original project/index.

**Acceptance Scenarios**:

1. **Given** two independent tasks and eligible distinct native contexts under unchanged default
   preferences, **When** the group starts, **Then** up to two actual owned workers overlap and
   distinct results/shared remaining allowance are visible.
2. **Given** one slot or shared-account restrictions, **When** both tasks are ready,
   **Then** one waits with the actual capacity reason; observation adds no model call.
3. **Given** one failed task with partial edits and an admitted sibling,
   **When** the failure settles, **Then** the sibling retains its own result and no failed
   work becomes qualified through the group's other successes.
4. **Given** a lost start acknowledgement and newer composition,
   **When** the exact request is retried, **Then** existing graph/child identities return
   without duplicate work, extra allowance or erased newer input.

---

### User Story 2 - Build on verified predecessor changes (Priority: P1)

The programmer connects independent branches to a follow-up task. Its real workspace must
contain the accepted files from both branches. Qualified review-only results can be consumed
without first applying them to the programmer's checkout.

**Why this priority**: A patch path or successful author exit cannot prove that required changes
are present in a dependent worker's workspace.

**Independent Test**: Two branches share one captured dirty source/index basis; a real follower
reads both retained outputs. Include conflicting fan-in and shared diamond ancestry.

**Acceptance Scenarios**:

1. **Given** two qualified disjoint changes, **When** the follower starts,
   **Then** its accessible workspace contains both exact outputs and unrelated captured state,
   despite later edits to the original checkout or old workspaces.
2. **Given** a failed, unverified, stale-version or corrupt prerequisite,
   **When** the follower is otherwise ready, **Then** no follower model starts and the exact
   missing qualification/evidence is identified.
3. **Given** overlapping incompatible file or staged changes, **When** inputs are assembled,
   **Then** a named conflict preserves both results and blocks before native preparation,
   without silent Git/text merging or last-writer overwrite.
4. **Given** A changes a file, B refines that inherited change and C carries A unchanged while
   changing another file, **When** D consumes B and C, **Then** B's proven refinement appears once
   alongside C's independent change. Divergent B/C edits block; predecessor checks alone do not
   qualify the composed result.

---

### User Story 3 - Supply small, attributable context (Priority: P2)

Each agent receives relevant task material without every conversation. Original constraints,
acceptance conditions and cited evidence stay complete and available behind derived digests.

**Why this priority**: Focused context reduces unnecessary work while keeping smaller-model
selection useful and preserving correctness requirements.

**Independent Test**: Inspect actual controlled-worker input/access, long original retrieval,
a labelled derived digest, and an excessive required context.

**Acceptance Scenarios**:

1. **Given** graph constraints, node guidance and relevant results, **When** a worker is prepared,
   **Then** every applicable original condition is present in full or by supported complete
   retrieval actually available to that worker; unrelated full histories are absent.
2. **Given** a derived digest, **When** its source is inspected, **Then** the complete retained
   original and exact producer/result are retrievable; a preview never replaces the original.
3. **Given** missing access, corruption or insufficient supported context capacity,
   **When** required input cannot be supplied, **Then** an already authorized supported
   alternative or explicit blocker precedes launch, without silent clipping or a byte-to-token claim.
4. **Given** peer prose suggesting another account, more calls or publication,
   **When** it crosses a dependency, **Then** it remains attributed task data and changes no
   captured execution, payment, budget or delivery authority.

---

### User Story 4 - Control and deliver the whole group honestly (Priority: P2)

The programmer inspects next owner/action, stops the group, updates node guidance and follows
retained results without an external phase plan. A designated final node may deliver the
cumulative qualified outcome without applying intermediate changes twice.

**Why this priority**: Concurrency must preserve finite expenditure, user work and accepted
lifecycle controls rather than introduce a weaker second execution path.

**Independent Test**: Actual UI/CLI Stop, guidance, allowance, restart and final-node delivery
journeys, including lost acknowledgements and dirty original state.

**Acceptance Scenarios**:

1. **Given** active and waiting work, **When** group Stop is accepted,
   **Then** no later node/stage starts, admitted workers receive cancellation, completed work
   stays unchanged and actual cleanup remains visible.
2. **Given** changed node guidance before dependent admission, **When** an old attempt finishes,
   **Then** it keeps its old version and cannot silently qualify a current prerequisite;
   unrelated completed work is not rerun.
3. **Given** exhausted shared allowance/window, **When** another author/review/correction/retry
   is needed, **Then** the group blocks without child budget refill. A finite explicit grant
   alone starts no work.
4. **Given** a final node incorporating all required branches and captured compatible delivery,
   **When** its cumulative result has current required verification, **Then** original-basis-to-final
   delivery applies once, preserving unrelated edits/index. Unassembled multiple outputs remain
   precise review-only results rather than an allegedly integrated deliverable.

5. **Given** an accepted file branch and a response-only final summary, **When** the summary is
   accepted, **Then** a review-only graph can retain both as complete separate results, while a
   graph requesting local file delivery remains blocked until the cumulative files are qualified.
   A pure-response graph can complete without a file candidate.
6. **Given** an intermediate node declaring an external publisher, **When** the graph is submitted,
   **Then** validation rejects that contradictory delivery before any worker or publisher starts.
7. **Given** a running node, an independent running sibling and a waiting descendant,
   **When** node Stop is accepted, **Then** that node is cancelled, its descendant cannot start,
   and the independent sibling settles under its own outcome.
8. **Given** a qualified final file node but an unfinished required context-only sibling,
   **When** the group outcome is inspected, **Then** it is not complete and final delivery cannot
   proceed. A context-only dependency path also cannot establish file incorporation.

### Edge Cases

- Empty/duplicate nodes, unknown/self dependencies, cycles, unsupported edge kinds, excessive
  declarations and conflicting request identities.
- Equal commits with different dirty files/indexes; original checkout and old workspace edits
  after capture; corrupt unchanged members despite valid manifest hashes.
- Modify/delete, path ancestry, type/mode and staged-only conflicts; shared diamond ancestry.
- Author completion awaiting review, failed checks with useful files, accepted responses versus
  accepted candidates, and a later acceptance after follower admission.
- Guidance/Stop during materialization or just before preparation; direct node controls trying
  to escape graph allowance, window or Stop.
- Shared account aliases, unknown external occupancy, concurrent hosts, simultaneous completions
  and uncertain cleanup while independent siblings are admitted.
- Oversize required originals, inaccessible references, or a digest omitting a critical condition.
- No single final output, a final node missing branches, response-only final delivery, or already
  applied ancestry that must not be blindly reapplied.
- Lost mutation/read replies, late UI responses, restart with completed and interrupted nodes.

## Requirements *(mandatory)*

### Functional Requirements

- **GRA-001**: Offer an optional bounded task group with stable graph/node identities, original instructions and explicit dependencies. Every declared node is required. Group acceptance conjunctively requires all current node outcomes, required input qualification, known settlement and captured delivery; one successful final node cannot hide unfinished work. Standalone Start remains Auto without graph configuration.
- **GRA-002**: Validate the whole declaration before execution: nonempty unique nodes, known endpoints, acyclic dependencies, supported input scopes and declared limits. Rejection grants no child execution authority.
- **GRA-003**: Capture one exact original project/source/index basis for file-producing branches. Retain dirty state, project identity and source facts; equal commits alone never establish equal inputs.
- **GRA-004**: Default to at most two concurrent managed workers across eligible native contexts, subject to narrower effective account/context/graph restrictions. Demonstrate overlap without preference changes. Account aliases and unknown external occupancy never invent capacity.
- **GRA-005**: Reuse accepted workflows, routing, explicit pins, prospective autonomy and proportionate verification. Scheduling, observation, materialization and default context digests use no model call.
- **GRA-006**: Enforce one captured finite group allowance/common window across all native author/review/correction/retry admissions, including manual node controls. Nodes, guidance and graph revisions never grant fresh expenditure.
- **GRA-007**: Retain node/group accounting without duplicate admission counts. Local stages remain visible but are not measured zero-token/zero-charge model work; unknown provider telemetry stays unknown.
- **GRA-008**: Admit dependents only from exact current qualified predecessor versions/results. Review-only accepted candidates may supply files; process completion, unverified changes and supporting review answers cannot replace acceptance.
- **GRA-009**: Distinguish file-change and context-only dependencies. Accepted responses can provide declared context but imply no materialized files; required input must be accessible through the receiver's supported capabilities.
- **GRA-010**: Retain immutable receiving-input provenance linking graph basis, predecessor acceptance/producer, full manifest/index/evidence and receiving execution. Later results never silently retarget an admitted receiver.
- **GRA-011**: Reconstruct required files in the receiver's isolated workspace from closed retained evidence, reverify required members and record actual materialization. Paths, summaries and old mutable workspaces are not availability proof.
- **GRA-012**: Compose supported fan-in changes against compatible retained bases, preserving unrelated source/index and shared ancestry once. A proven descendant refinement supersedes only its own inherited contribution; an unchanged sibling copy does not conflict with it. Divergent sibling edits, incompatible staging/path/type or basis block before receiver native preparation without automatic Git/text merging.
- **GRA-013**: Qualify the receiving composed result through its own applicable workflow. Predecessor checks/reviews remain linked historical evidence and never automatically qualify a different combined subject.
- **GRA-014**: Supply focused authorized context with graph-wide constraints, receiving original/guidance conditions and applicable contributed-result constraints. Preserve complete retrievable originals and omit unrelated full conversations.
- **GRA-015**: Label derived digests/summaries with exact sources and retained-original access. Default summarization spends no model call; missing required access, corruption or insufficient supported capacity causes a named blocker or an already authorized supported alternative.
- **GRA-016**: Peer content and predecessor authority create no receiver execution, account, payment, budget, tool or publication scope. Only explicit captured user authority controls admission/effects.
- **GRA-017**: Failed/cancelled/unverified prerequisites block descendants while admitted independent siblings settle with their own evidence. Unknown ownership/effects/cleanup fence unsafe new work without erasing failures or sibling results.
- **GRA-018**: Distinguish group Stop from node Stop and recheck both before admission/preparation/effects. Group Stop cancels admitted work and prevents later work. Node Stop cancels its own active execution, prevents its further stages and unlaunched descendants, and preserves independent admitted work and historical qualified results; already dispatched work is never retconned. Show actual cleanup.
- **GRA-019**: Apply guidance/version changes at explicit boundaries. Old executions keep their instructions; stale dependency bindings block further use. Deliberate replacement/retry is explicit, linked and bounded, never automatic completed-work replay.
- **GRA-020**: Capture group delivery prospectively. Intermediate nodes permit neither local application nor external delivery/action; reject contradictory declarations before execution. Only the designated final node may deliver under captured scope after all other required nodes have current accepted results and its cumulative original-basis-to-final candidate has current required pre-delivery checks/review and known prior settlement. Required delivery then precedes final-node and group acceptance; acceptance is never its own delivery prerequisite. Every node must reach it, and file incorporation requires file-change paths, never context-only reachability. Accepting a response alone cannot qualify/apply retained file changes. Review-only groups can complete with separately qualified files and an accepted summary; requested local delivery with a nonempty unqualified file delta blocks explicitly. Pure-response groups may complete without a file candidate or fabricated integration.
- **GRA-021**: Preserve exact idempotent receipts, shared-host ownership/capacity and fail-closed persistence for graph/node mutations. Restart and late observation expose retained state without launching, rearming history or model replay.
- **GRA-022**: UI/CLI share graph/node identity, dependencies, actual state, next action/owner, budget, input provenance, complete evidence and eligible controls. Lost replies/late reads preserve captured intent and newer drafts/selection.
- **GRA-023**: Extend approved English Ejupi Labs UI without a mandatory graph/stage/model wizard. Verify actual creation, waiting/conflict/context/budget/Stop/history/delivery states at desktop/360px with keyboard/focus, reduced motion and approved hashes.
- **GRA-024**: Complete Specify design/tasks before implementation, independently review/fix changes, and qualify full offline journeys plus six actual native targets and Linux race at one checked revision before 007. Preserve 001–005 behavior/private-state exclusions; live provider/model/account/savings claims remain separate.

### Key Entities *(include if feature involves data)*

- **Task graph**: programmer-owned group, original intent/layout, current controls, captured
  source/delivery authority, shared allowance/window and attributable aggregate outcome.
- **Graph node**: stable key linked to a managed task/version, dependency state, execution
  lineage, qualified result and current next action.
- **Dependency**: declared prerequisite and authorized file/context scope, bound to exact
  accepted inputs before receiver execution.
- **Graph basis/materialized input**: retained original source/index and verified receiving
  files/evidence with contributing ancestor/result identities and access status.
- **Context digest**: derived focused material, exact source/original access, applicable original
  constraints and explicit capacity/access limits.
- **Graph receipt**: exact mutation identity, control/budget transition or aggregate outcome;
  status alone never establishes acceptance or external delivery.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Two actual controlled workers overlap under the default two-slot policy without changing preferences; one-slot/shared-account counterexamples never exceed effective capacity or add observation model calls.
- **SC-002**: A two-predecessor follower starts only after both exact current results qualify and reads both retained file changes, including Unicode/CRLF and staged facts, with original source/index intact.
- **SC-003**: Failed/unverified/stale/corrupt prerequisites and incompatible fan-in each prevent receiver native preparation with a named blocker and retained sibling/predecessor evidence.
- **SC-004**: In A→B/C→D, B may refine A's file while C carries it unchanged; D receives B's refinement once plus C's independent change. Divergent B/C edits block. Predecessor receipts never qualify the changed combined subject without current required verification.
- **SC-005**: Actual receiver input preserves every applicable original condition, omits unrelated full histories and permits complete cited-original retrieval; missing/excessive required context blocks without silent clipping.
- **SC-006**: Every graph-owned native admission obeys the same finite allowance/window, including node controls, reviews/corrections/retries; local scheduling/context/materialization adds zero model admissions.
- **SC-007**: Group/node Stop, guidance, lost acknowledgements, faults and restart preserve identities, prior outcomes and useful files without duplicate work, budget refill or automatic historical replay. Actual node Stop cancels that worker and prevents its waiting descendant while an independent admitted sibling settles normally.
- **SC-008**: Qualified final-node delivery applies original-basis-to-final changes once while preserving unrelated edits/index. Missing required context siblings, context-only file paths, intermediate publisher declarations and response-only acceptance cannot bypass file qualification/delivery. Review-only mixed outputs and pure-response groups retain their distinct complete outcomes without fabricated integration.
- **SC-009**: Actual UI/CLI journeys agree on identities/evidence/controls and pass desktop/360px, keyboard/focus, reduced-motion, late-response and approved-hash checks without mandatory standalone settings.
- **SC-010**: Every GRA/SC has actual source/check/independent-review evidence; final build/test/check/package and all seven matching native/race jobs pass without blocking findings before 007.

## Assumptions

- Accepted005 source is `794f080`, with acceptance commit `c5b1801`. This slice owns parent
  FR-025–FR-028/FR-030 and extends FR-029 and shared control/delivery/UI requirements.
- Programmers supply explicit useful tasks/dependencies. A compulsory model planner, autonomous
  task invention and recursive agent organizations are outside this slice.
- Support up to 16 nodes/64 dependency edges in one project with declared supported source kinds.
  Visible limits cause rejection rather than silent shortening; parallelism has an automatic default.
- File branches share an exact captured basis; conservative composition blocks incompatible edits.
- Shared allowance/window comes from saved finite policy or explicit bounded override, never a
  fresh grant per child. Existing payment/budget semantics remain.
- Default dependencies consume qualified candidate changes and relevant evidence. Optional
  context-only dependencies require accepted results and imply no patch.
- A new graph visibly defaults to Separate reviewable results, capturing review-only delivery
  independently of the unchanged standalone saved preference. Selecting a final node can
  prospectively use the saved local delivery scope. Explicit delivery without a compatible final
  node is rejected, never silently downgraded; external delivery always needs explicit final scope.
- Every declared node is required; there are no optional/skippable nodes in this slice. A final
  delivery node must be downstream of every node; file branches need file-change paths.
  Context-only reachability never proves file incorporation. Intermediate local and external
  delivery is forbidden; contradictory declarations are rejected rather than silently ignored.
- Review-only graphs may complete with separate qualified file outputs and an accepted final
  summary. With nonempty retained file changes and requested local delivery, a response alone
  leaves a named file-qualification blocker. Pure-response graphs complete through response
  acceptance without claiming file integration.
- Saved full access remains supported through declared harness capabilities; it grants neither
  unscoped publication nor a universal operating-system sandbox.
- Use accepted Codex/Claude contracts and fixed offline workers. New adapters, MCP, durable
  pause/resume/checkpoint/backup and reviewed cleanup stay in later slices; fail-closed ownership
  and no-replay restart behavior remain mandatory now.
- Product code/UI/docs remain English; original instructions/code/evidence stay verbatim.
  Approved assets/notices and private-state/package exclusions remain.
