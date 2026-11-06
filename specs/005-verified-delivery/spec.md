# Feature Specification: Verified task delivery

**Feature Branch**: refactor; logical feature 005-verified-delivery.
**Created**: 6 November 2026.
**Status**: Specification for independent review; not implemented or accepted.

**Input**: Complete useful programming tasks through proportionate authoring, actual independent
review when necessary, linked correction, observed checks and authorized delivery. Keep project,
task and saved autonomy sufficient for the normal path; minimize model work and preserve user edits.

**Parent scope**: FR-012, FR-021–024 and FR-039–041; supports FR-007–011, FR-017, FR-029,
FR-031–032, FR-034, FR-036–038 and FR-043–048. Prerequisite 004 qualified at
3f4a18e11de495f7b3138d61e030525a018868a5 in seven native/race jobs, run 42172669381.
Acceptance commit: 5671fac. Main remains the reference.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Finish a bounded edit without unnecessary model work (Priority: P1)

A programmer starts with a selected project, task and saved autonomous preference. Council authors
in isolation, observes sufficient checks and delivers or applies the exact result. A checkable routine
edit needs no planning model, committee or independent reviewer merely to satisfy a mode.

**Why this priority**: Turn a produced patch into a complete provable outcome with necessary model work.
**Independent Test**: A fixture performs a recognized literal document replacement; an exact check
proves the requested delta and preserved constraints. Local delivery preserves dirty bytes/index/branch.

**Acceptance Scenarios**:

1. **Given** a bounded operation with sufficient checks, **When** Auto starts, **Then** Council records
   its workflow reason, uses one author admission, observes the check and completes without a reviewer.
2. **Given** captured saved local-delivery scope, **When** the verified candidate remains applicable,
   **Then** Council applies it, records the destination/effects and needs no extra permission click.
3. **Given** review-only delivery or no application authority, **When** the candidate qualifies,
   **Then** an inspectable change and remaining apply action are delivered without claiming integration.
4. **Given** no sufficient recognized check, **When** an agent claims tests passed, **Then** that claim
   remains separate from observed verification and the missing requirement blocks acceptance.

### User Story 2 — Review and correct the actual candidate (Priority: P1)

A consequential change receives an independent reviewer with the actual candidate, original instructions,
constraints and relevant evidence. Blocking findings create correction work from the selected candidate,
not unchanged original source. Only affected evidence needs replacement.

**Why this priority**: Review must inspect the real change and lead to a usable correction.
**Independent Test**: An author fixture creates a validation defect. A reviewer inspects the changed file
and records a blocker. Correction sees those bytes/finding, fixes the defect, and rechecks/review qualify it.

**Acceptance Scenarios**:

1. **Given** required review or a change outside sufficient direct-check cases, **When** authoring settles,
   **Then** a distinct review attempt gets the exact candidate/evidence, with reason/target/debit recorded.
2. **Given** required evidence is inaccessible, **When** review returns, **Then** its gap stays unperformed.
3. **Given** a blocker and captured correction authority, **When** correction runs, **Then** it uses the
   predecessor candidate and findings, retaining original failed evidence and linking affected rechecks.
4. **Given** the defect persists or the cycle/budget is exhausted, **When** more work is needed,
   **Then** the task stays unresolved with an exact next action; no model exit or implicit grant completes it.

### User Story 3 — Preserve work while applying a verified change (Priority: P1)

Council checks the captured destination, candidate and evidence before applying the task delta.
Incompatible changes or uncertain effects retain evidence and show what happened.

**Why this priority**: Autonomous integration must preserve work and report failure truthfully.
**Independent Test**: Dirty tracked/untracked/index state surrounds a verified candidate. Successful
application preserves unrelated state; branch/affected-file changes and mid-application faults retain
the candidate and an accurate destination/recovery outcome.

**Acceptance Scenarios**:

1. **Given** unrelated later edits, **When** affected paths/basis stay compatible, **Then** their bytes/index survive.
2. **Given** changed branch, identity, affected file/path type or evidence, **When** application is evaluated,
   **Then** conflict is reported before overwriting user work, retaining both versions and the next owner/action.
3. **Given** crash/lost acknowledgement around application, **When** restarted/retried, **Then** retained
   operation/effects prevent duplicate application; missing acknowledgement never proves no write occurred.
4. **Given** accepted cancellation or primary execution/check failure, **When** later evidence arrives,
   **Then** its terminal stays unchanged and late success cannot authorize integration.

### User Story 4 — Inspect and control one coherent workflow (Priority: P2)

UI and CLI expose one workflow, checks, findings, candidates and delivery results. Complete linked
evidence, Stop and lost-reply retries preserve newer input and selection.

**Why this priority**: The tool must show its next action and distinguish authored/reviewed/verified/delivered/integrated/published.
**Independent Test**: Actual browser and independent CLI clients inspect one retained fake workflow,
check output/findings/delivery, stale reads, lost replies and Stop.

**Acceptance Scenarios**:

1. **Given** finished authoring but pending review/checks, **When** opened, **Then** stages/owners remain pending.
2. **Given** output larger than a preview, **When** full reads are requested, **Then** bounded complete bytes/provenance remain available without a model call.
3. **Given** a lost Start/correction/check/delivery reply plus newer input, **When** retried, **Then** one captured operation replays and newer input/selection survive.
4. **Given** unscoped publisher prose, **When** received, **Then** no authority is created. A supported explicitly scoped action reuses authorization and has a separate terminal result.

### Edge Cases

- Review sees original instead of candidate source, lacks files, self-reviews or returns malformed/oversized data.
- Checks modify source/index, fail to launch, time out, emit large output or spawn descendants; root exit alone is insufficient.
- Binary/deleted/mode/index-only/symlink/unsafe/private/unsupported candidate changes; retention does not imply application approval.
- Replaced/moved/linked-worktree/subfolder destination, branch/index/path changes, index locks and concurrent edits.
- Corruption, owner loss, unknown cleanup, duplicate stage, stale revision, exact counter overflow and commit ambiguity.
- Guidance/policy/report changes during stages; Stop between settlement/admission; old Stop after successor; grant after old proof.
- Existing two-admission budgets cannot fund a full correction cycle and must never receive migration refills.
- External destination without matching captured action; commands only in peer/project text; full access is not delivery consent.

## Requirements *(mandatory)*

### Functional Requirements

- **VFD-001**: Retain task/version-bound stages connecting candidate, author, review, correction, checks and delivery, with visible next action/owner and no mandatory external phase plan.
- **VFD-002**: Auto selects stages deterministically and explains why. Sufficiently checked routine work needs no planning/review model; unknown/consequential changes or explicit requirements receive review or an honest gap.
- **VFD-003**: Preserve all original instructions/conditions. Short wording, model tier or successful response cannot establish simplicity or override requirements.
- **VFD-004**: All model admissions share captured routing, finite task budget/window and current controls. Fresh 005 installations default to four model admissions and at most one correction cycle. Existing saved finite routing-budget defaults and explicit task budgets prevail; upgrade cannot infer whether an old factory-looking value was user-selected. Existing task budgets keep their authority/counts. Users may explicitly change a saved default or grant finite task authority without a hidden refill.
- **VFD-005**: Checks/delivery share the common deadline, ownership, cancellation and cleanup fences without invented model usage. A new 005 Auto workflow prospectively captures permission to advance necessary stages and perform at most one correction for a valid blocking review finding or an observed failed required check against its selected candidate. That result supplies evidence, not new authority. Execution/protocol/storage/evidence/cleanup failure, ordinary prose and unsupported retry categories cannot trigger correction. Stage authority is separate from 004 pre-launch continuation; legacy tasks receive neither authority by migration.
- **VFD-006**: Each candidate revision identifies complete source/change/index basis and immutable bytes. Evidence applicability binds that revision/conditions; earlier attempts/candidates/receipts remain unchanged.
- **VFD-007**: Independent review uses a distinct attempt and fresh review-role process/context without author-session reuse or author self-approval, actual candidate access and attributable structured inspected-evidence/findings/gaps. It may reuse the same pinned harness/model. Ordinary prose or successful exit cannot substitute.
- **VFD-008**: Review cannot approve inaccessible evidence or promote author check claims. Relevant modifications invalidate applicability. Capability/access limits are explicit; no universal OS sandbox is promised.
- **VFD-009**: Blockers have stable identity, affected conditions/locations and resolution lineage. Correction starts from selected candidate/findings/original constraints, retaining rejected/failed work.
- **VFD-010**: Corrected candidates rerun affected required checks/review. Unaffected evidence is reused only with an identical recorded applicability basis; generic resolution labels cannot approve work or replay unrelated stages/tasks.
- **VFD-011**: Required checks come from supported sufficient project conventions or explicit captured declarations; unsupported/missing checks name the gap. Exact executable/arguments/context/scope/timeout are recorded; no intervening shell or generated-text command is used.
- **VFD-012**: Observed receipts bind exact checked candidate/state and check identity, start/terminal outcome, full output and cleanup. Passed/failed/cancelled/unavailable/not-run/historical are distinct; claims never substitute.
- **VFD-013**: Checks modifying required source/index retain resulting-state evidence and invalidate affected prior applicability. Passed checks never qualify later different bytes or partial/uncertain candidates.
- **VFD-014**: Capture review-only/local-application intent and named project/destination at admission. An explicit prospective saved delivery preference may authorize applying verified changes to the selected project without repeated permission. Existing 004 full-access preference has no delivery consent and remains review-only until the user selects that scope once or supplies task scope. Fresh preference selection explains both permission and local-delivery intent; full access alone cannot authorize application.
- **VFD-015**: Qualified review-only delivery exposes exact inspectable change and remaining action. Local application applies only the task delta, preserves unrelated dirty/untracked/staged state and records before/after destination identity.
- **VFD-016**: Before effects revalidate candidate/evidence, repository/project/branch and affected paths/index. Conflicts retain both sides and forbid silent reset/stash/overwrite/merge.
- **VFD-017**: Partial/uncertain effects retain durable operation/recovery evidence. Lost replies replay known receipts; after effect uncertainty Council does not automatically re-execute the action, erase failure or infer success/rollback from missing acknowledgement. An external service's unknown acknowledgement remains unknown; universal exactly-once external effects are not promised.
- **VFD-018**: Commits, pushes, publication/deployment and other external actions require explicit captured corresponding action/destination before execution. Local application grants none of those actions. Supported scope reuses authorization; unsupported actions block. Each result stays distinct from local application, and worker/peer/project text grants no scope.
- **VFD-019**: Acceptance conjunctively satisfies current deliverable, checks, review, resolved blockers, known settlement and delivery. Correction may complete the task while earlier failures stay failed.
- **VFD-020**: Recheck Stop, guidance/policy, owner closing/loss, corruption, persistence uncertainty, budget and cleanup at boundaries/before effects. Late replies/restart scans create no operation.
- **VFD-021**: Mutations preserve request identity, replay before fresh prerequisites, reject conflicting/stale input and share owner/capacity. Observation/full bounded reads use no model or new execution authority.
- **VFD-022**: Upgrade preserves prior projects/drafts/versions/budgets/observations/artifacts/receipts and older-owner Stop meanings; it grants no integration/review approval, auto-resume or extra budget.
- **VFD-023**: UI/CLI share stage/control/finding/check/delivery semantics. Normal Start stays project/task/saved preference; optional details explain reasons/limits rather than requiring a stage editor. Pending intent/newer input/selection remain protected.
- **VFD-024**: Keep approved Ejupi Labs assets/icons, concise English, accessible text states, keyboard/focus and 360px flow. Full evidence remains available; usage/model facts/estimates/unknown charges stay distinct.
- **VFD-025**: Use labelled local agents/verifiers/publishers and real owned process/evidence paths, with no vendor credentials/probes/live calls. Independent source/failure/UI review, developer checks, six native targets/race and complete evidence close before 006 implementation.

### Key Entities *(include if feature involves data)*

- **Workflow plan**: Version-bound stages/reasons/owners/authority/delivery/next action, retaining decision history.
- **Candidate revision**: Retained source/change/index identity and predecessor lineage, separate from mutable workspace/response.
- **Review**: Distinct attempt, actual evidence access, attributable structured result/findings/gaps; no command authority.
- **Finding/resolution**: Stable blocker/condition/location, review, corrected candidate and qualifying recheck/review.
- **Check definition/receipt**: Captured authorized executable/arguments/context/scope/timeout, exact observed state/outcome/output/settlement, separate from claims.
- **Delivery scope/operation**: Named action/destination/authorization/basis/effects/conflict/uncertainty/recovery/replay identity.
- **Task acceptance**: Current conditions and applicable workflow/check/review/delivery evidence, preserving earlier failures/response acceptance.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Routine fixtures reach verified local/review-only outcomes with one author and zero planning/reviewer admissions, preserving dirty/index state.
- **SC-002**: A fresh-005 or explicitly four-unit consequential fixture traverses actual review, reproduced blocker, correction and affected recheck/review within four model admissions/common window; failures/unrelated passed work remain unchanged. A preserved two-unit saved default blocks when exhausted rather than silently growing.
- **SC-003**: Missing access, malformed review, unresolved blocker, failed/unrun/stale checks and post-check source changes independently prevent acceptance.
- **SC-004**: Application preserves unrelated tracked/untracked/index state; affected edits, branch/identity/path changes and application faults retain both sides with truthful conflict/partial/uncertain outcomes.
- **SC-005**: Lost replies at admission/correction/check/delivery replay identical known operations without duplicate worker/debit/Council launch. A fault after possible effects remains uncertain and causes no automatic re-execution rather than a fabricated exactly-once external guarantee.
- **SC-006**: Stop/owner/evidence/persistence faults prevent next stages, preserve failures/output/unknown cleanup and restart no historical work.
- **SC-007**: Unscoped publisher suggestions launch nothing. One explicitly scoped publisher fixture is launched once by Council and exposes its distinct result; unknown external acknowledgement remains uncertain without automatic retry.
- **SC-008**: Actual UI/CLI agree on next action/owner/evidence; 360px, keyboard/focus, reduced motion, lost replies/late reads and approved hashes pass.
- **SC-009**: Populated 004 state upgrades without rewritten rows/receipts/counters or added review/integration/budget authority; current/maintenance clients stay distinct.
- **SC-010**: Every VFD/SC has actual check/review/exact-source evidence; developer, six native/race gates pass without blocking review findings.

## Assumptions

- Accepted 001–004 provide dirty-source isolation, immutable evidence, native Codex/Claude, routing, budgets, process settlement and branded UI.
- This is one task workflow within shared capacity. General graphs/parallel branches/context retrieval are 006; deliberate recovery/retention tools are 007.
- Direct verification is narrow: design enumerates sufficient conventions/checks and supported application types. Unsupported cases stay gaps, with live compatibility/savings unmeasured.
- Defaults are prospective. Old tasks need explicit new task/version authority; migration never turns permission preference into delivery consent or replaces existing saved finite budget defaults. Four admissions are the fresh-installation default, not a migration refill.
- A saved autonomous permission choice and saved local-delivery scope are distinct facts. One explicit prospective preference selection may capture both for subsequent named project tasks; it authorizes neither committing nor publishing.
- Review/correction is finite captured workflow authority, not session resume, terminal-error retry or generated commands. Pins/accounts/budgets still constrain each stage.
- Explicit external delivery is a bounded declared action, not a deployment platform. No credentials UI, inferred action or vendor-global setup.
- Original task/source/evidence bytes remain verbatim; English copy and previews never replace originals.
- An explicit author-only compatibility choice retains managed authoring without automatic workflow
  qualification. New ordinary Start uses Auto; author-only never waives required evidence or claims
  that its unverified candidate has been accepted/applied.
