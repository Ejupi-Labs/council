# Feature Specification: Pause and Safe Recovery

**Feature context**: `specs/007-recovery-and-retention/`  
**Git branch**: `refactor`  
**Created**: 2026-11-07  
**Status**: Reviewed for implementation  
**Input**: Extend accepted feature 006 with durable scoped pause, checkpoints, explicit resume and evidence-backed reconciliation. Export, backup, restore and reviewed cleanup remain mandatory in the next consecutive feature.

## User Scenarios & Testing

### User Story 1 — Pause and continue unfinished work (Priority: P1)

A programmer pauses a task or group before stepping away. Already admitted work can finish and its results remain inspectable. Later, the programmer explicitly resumes from a durable checkpoint without paying for or repeating completed work.

**Why this priority**: Pausing without losing or duplicating work is the central control promised by this feature.

**Independent test**: Pause a controlled group while one member runs and another waits; restart Council, inspect the retained checkpoint and resume. Count admitted work and compare completed identities before and after.

**Acceptance Scenarios**:

1. **Given** a group with an admitted member and a waiting dependent, **When** pause is accepted, **Then** no new work is admitted under that scope and the view distinguishes pausing from paused while admitted work settles.
2. **Given** the admitted member has settled, **When** the checkpoint is durable, **Then** the user sees paused state, retained completed and unfinished identities, and the next permitted action.
3. **Given** a paused checkpoint after owner restart, **When** the user explicitly resumes, **Then** only eligible unfinished work may advance after current authority, inputs, budget, ownership and controls are checked; completed work is not replayed.
4. **Given** a pause and an accepted stop or failure overlap, **When** the admitted work settles, **Then** accepted stop and the primary failure retain their precedence and the result never becomes successful merely because pause completed.
5. **Given** a task that belongs to a group, **When** that member is paused, **Then** its new admissions stop under the group's authority while independently eligible siblings can continue and no downstream work consumes an unfinished prerequisite.
6. **Given** both a member pause and a group pause are active, **When** the member is resumed, **Then** the group pause still prevents group admission until the group is separately resumed.

### User Story 2 — Reconcile interrupted and uncertain work (Priority: P1)

After a crash, disconnect or cleanup fault, a programmer can see what Council knows, inspect retained results and take a deliberate next step without an ambiguous process being treated as finished.

**Why this priority**: Replaying an uncertain write or losing a failure can damage work and waste account capacity.

**Independent test**: Terminate a controlled owner during a write, preserve partial output and test both known-settled and unresolved process states. Restart and inspect results and admission counts.

**Acceptance Scenarios**:

1. **Given** an owner disappears after work was admitted, **When** Council restarts, **Then** the user sees interrupted or unresolved work, its last established ownership, retained changes and known outcomes; restart alone dispatches no replacement.
2. **Given** process cleanup or durable evidence is uncertain, **When** a conflicting new dispatch is requested, **Then** it is blocked with an actionable reason until the conflicting ownership/evidence is reconciled; unrelated work may proceed only when it is independently established non-conflicting. If that scope cannot be established, the applicable global admission fence remains in force.
3. **Given** completion cannot be established, **When** the user inspects the incident, **Then** Council reports uncertainty and available evidence rather than inferring success, failure, safe cleanup or permission to replay.
4. **Given** a deliberate replacement is permitted after reconciliation, **When** the programmer starts it, **Then** it has a new identity linked to the predecessor and preserves the predecessor's outcome, partial changes and evidence.
5. **Given** an authoritative failure or accepted cancellation precedes a late result, **When** that result arrives, **Then** the earlier outcome and diagnostic remain visible and are not rewritten as success.
6. **Given** an unresolved incident, **When** the user requests reconciliation, **Then** Council displays the evidence and checks available for that incident, records the requested action and actual observations, and moves to reconciled only if ownership, relevant effects and cleanup are established sufficiently to make the next action safe. Missing or conflicting evidence leaves the incident unresolved and fenced; a user assertion alone is not proof.
7. **Given** an interrupted controlled attempt whose exact Council-owned process and complete owned process tree are verifiably terminated, whose durable effects and retained evidence are complete and consistent, and whose cleanup is verified, **When** reconciliation checks those facts and all other relevant claims and blockers, **Then** it records the evidence, transitions that incident to reconciled and releases only its own verified-safe fence or claim. Another incident, shared-capacity claim or other control continues to block where applicable; reconciliation itself does not dispatch work.
8. **Given** the same incident with an unverified child process, incomplete effects or missing retained evidence, **When** reconciliation is requested, **Then** it records the missing proof, remains unresolved and preserves its fence or claim.

### User Story 3 — Operate and inspect across interfaces (Priority: P2)

A programmer uses the branded local interface or CLI to inspect the same checkpoint or recovery incident, without model calls for observation.

**Why this priority**: Recovery must be practical in existing day-to-day workflows and reveal the same facts from either entry point.

**Independent test**: Initiate a controlled pause or recovery operation through one interface and inspect and continue it through the other, including a repeated request and a lost acknowledgement.

**Acceptance Scenarios**:

1. **Given** a task controlled in either interface, **When** the other reads it, **Then** it reports the same task, checkpoint, operation outcome, uncertainty and next permitted action.
2. **Given** an acknowledgement is lost or the same request is repeated, **When** the caller retries the same operation, **Then** Council returns the existing outcome or an actionable conflict without duplicate dispatch or repeated effects.
3. **Given** an open recovery view, **When** a delayed update arrives after a newer selection or draft, **Then** the newer user state remains intact; failed reads offer a retry.
4. **Given** a keyboard user and a narrow display, **When** they perform the primary pause, resume and reconciliation journeys, **Then** controls have accessible names, visible focus and readable non-colour status labels.

### Edge Cases

- A pause races with initial admission, known first effect, guidance change, budget expiry, group Stop or owner loss; the observed boundary and accepted authority determine what may continue.
- A checkpoint cannot be durably saved or an acknowledgement is lost after effects: show exact known effects and uncertainty, preserve evidence, and do not retry work blindly.
- A process identity cannot be proven to belong to the interrupted attempt or may have been reused: do not terminate it or declare cleanup verified on that basis.
- A concurrent Council host owns the work: respect shared ownership and block conflicting operations until established safe.
- Legacy collaboration state remains separate; do not silently open, migrate or delete it. Unknown provider/session continuity and actual account availability remain unknown.

## Requirements

### Functional Requirements

- **RCR-001**: Council MUST provide scoped task and group pause requests that close new admission at an established boundary while already admitted work settles.
- **RCR-002**: Council MUST distinguish pausing, paused, interrupted, blocked, failed, cancelled and completed outcomes using retained evidence; pausing MUST NOT claim that an operating process is suspended or already stopped.
- **RCR-003**: Council MUST persist a checkpoint containing the selected scope, completed and unfinished identities, current controls, instruction versions, known outcomes, retained changes and next permitted action sufficient for a safe restart.
- **RCR-004**: Explicit resume MUST retain completed identities and admit only eligible unfinished work after rechecking current ownership, controls, guidance, budget, dependencies and input evidence; it MUST NOT repeat a completed model call or write.
- **RCR-005**: When checkpoint persistence fails or its state cannot be established, Council MUST report the failure, preserve known effects and prevent conflicting admission until reconciled.
- **RCR-006**: Owner loss MUST retain last established ownership, evidence and effects, and MUST NOT itself authorize an automatic replacement or replay.
- **RCR-007**: Recovery MUST distinguish verified settled work from uncertain process ownership, cleanup, outcome or persistence; uncertain conflicting work MUST remain fenced with an actionable explanation. Unrelated work may proceed only when independently established non-conflicting; when the affected scope cannot be established, the applicable global admission fence remains in force.
- **RCR-008**: Reconciliation MUST use established ownership and retained evidence, preserve unrelated processes and paths, and MUST NOT treat an unverified process identity or missing response as proof of cleanup. A controlled attempt with verified termination of its exact owned process and complete owned process tree, complete consistent durable effects/evidence and verified cleanup MUST be eligible for reconciliation: record these facts, check all other relevant blockers, mark that incident reconciled, and release only its own verified-safe fence or claim without dispatching work. An unverified child, incomplete effects or missing evidence MUST remain unresolved and fenced. It MUST offer a supported, auditable reconciliation action which records the request, evidence, observations and outcome; an incident may transition to reconciled only when established ownership, relevant effects and cleanup make the proposed next action safe. Missing or conflicting evidence, including a user assertion without proof, MUST retain the unresolved state and fence.
- **RCR-009**: A permitted user-requested replacement MUST receive a new identity linked to its predecessor and preserve the original failure, accepted cancellation, partial result, changes and evidence.
- **RCR-010**: Stop, failure and evidence incidents MUST retain their established precedence during pause, shutdown, recovery and late completion; partial output or transport success MUST NOT imply acceptance.
- **RCR-020**: Pause, resume and reconciliation MUST expose stable operation identities and safe repeated-request/lost-acknowledgement outcomes, without duplicated effects.
- **RCR-021**: UI and CLI MUST expose equivalent scope, identities, outcomes, uncertainty and next actions for pause, resume and reconciliation; status, waits and evidence inspection MUST make zero model calls.
- **RCR-022**: UI MUST retain existing drafts and selections across stale responses and provide English, actionable, keyboard-operable controls, visible focus and non-colour state labels consistent with approved Ejupi Labs assets.
- **RCR-023**: Workflows MUST honor existing autonomous and captured task policy without additional routine permissions; conflicting authority MUST fail closed.
- **RCR-024**: Existing accepted task, graph, routing, review and delivery records and user edits MUST survive these operations. Council MUST NOT inspect or copy vendor credentials. Legacy state and unrelated project data MUST NOT be silently inspected, migrated or deleted.
- **RCR-025**: Offline acceptance MUST cover controlled owner crash/restart, lost acknowledgement, shutdown during partial output, task and group pause/resume, and concurrent ownership; live provider capability MUST be reported separately.

### Key Entities

- **Pause scope**: The task or group and the accepted boundary on which new admission is stopped.
- **Checkpoint**: A durable, attributable record of completed and unfinished identities, controls, evidence and the next safe action.
- **Recovery incident**: A retained account of ownership, process/effect uncertainty, primary failure and reconciliation state.

## Success Criteria

### Measurable Outcomes

- **SC-001**: In controlled task and group pause tests, zero new work admissions occur after the accepted pause boundary until explicit resume, and 100% of previously completed identities remain unchanged after restart and resume.
- **SC-002**: In each crash, lost-acknowledgement and unknown-cleanup scenario, automatic replacement dispatches and duplicated completed effects total zero; each unresolved conflict names an actionable next step. A fully evidenced safe incident transitions to reconciled and releases only its own claim, while an otherwise identical insufficient-evidence incident remains fenced and other blockers remain effective.
- **SC-003**: In each controlled checkpoint persistence or partial shutdown failure, every known terminal result and primary failure remains retrievable, and uncertainty is visible rather than reported as success.
- **SC-007**: Each primary pause, resume and reconcile journey is available through UI and CLI with matching identities and outcomes; observation and waiting add zero model calls.
- **SC-008**: All primary recovery controls are keyboard-operable at desktop and narrow widths with visible focus, named controls and text state labels; testing uses actual rendered interface states.
- **SC-009**: All feature requirements map to scenario evidence, independent review and correction, with actual offline local/platform results recorded separately from any unperformed live qualification.

## Assumptions

- Feature 006 is accepted at implementation revision `48484fe82e3e62df906bcb999693e4e1f0e94e2c`; its historical uncertainty and scoped visual/client limitations remain documented. Existing behavior is the baseline, not a claim that 007 already exists.
- Council is a local single-user product; explicit pause and reconciliation scope identifies the selected task or group. Existing task policy and current authority continue to govern later effects.
- Pause is an admission control and drain, not a promise of suspending an arbitrary provider process. Resume is deliberate; restarting Council is not a resume request.
- Scope includes existing UI and CLI. The next consecutive feature covers deferred retention; MCP/host parity, additional harnesses and complete release qualification remain in subsequent planned features.
- Acceptance uses controlled offline agents and supported platforms. Live vendor access requires a separate explicit task and is not implied by fake-agent tests.

### Deferred mandatory retention scope

The next consecutive feature MUST specify, design, implement, review, correct and provide acceptance evidence for scoped evidence export, project-state backup, verified restore and reviewed artifact cleanup. The split does not waive or satisfy FR-042 or the original retention requirements below. Original identifiers are retained for traceability; they are **not** acceptance requirements for pause/recovery feature 007. The next feature must also carry over the retention-specific parts of the original cross-cutting requirements: exact lost-acknowledgement outcomes, UI/CLI parity and accessibility, explicit destination and disposal authority, preservation of existing records, vendor credential exclusion and offline failure-path acceptance. The credential-free provenance of real production data and the destination/owner through which restored records become inspectable are unresolved design questions for that feature; synthetic fixture provenance alone is not a production solution.

- **RCR-011**: Council MUST expose scoped evidence export with source identities, included and excluded categories, and an explicit result for missing or unavailable selected evidence.
- **RCR-012**: Council MUST provide a project-state backup whose declared scope, source, format compatibility, completeness and integrity can be inspected before restore. Its manifest MUST explicitly state whether selected project filesystem material and referenced artifacts are included or omitted, identify unavailable references. Neither backup nor evidence export may inspect or copy vendor credentials, even when selected data references them; Council MUST report the exclusion and fail closed when it cannot establish that the selected material excludes credentials.
- **RCR-013**: An export or backup MUST include only the requested authorized scope and MUST report failures without describing incomplete material as complete or verified.
- **RCR-014**: Restore MUST verify completeness, integrity, compatibility, source identity and destination conflicts before changing the destination; a rejected restore before effects MUST preserve the original backup and destination. Council MUST revalidate the selected backup and destination at the commit boundary and reject newly discovered conflicts before applying effects.
- **RCR-015**: Restore MUST NOT overwrite newer or unrelated destination work or silently resume writers, model calls, external delivery or unfinished execution. If restore fails or is interrupted after effects begin, Council MUST retain an operation record and known effects, identify uncertain effects, fence conflicting work and provide auditable reconciliation without blind replay or representing a partial restore as complete.
- **RCR-016**: Restored uncertain work MUST remain visibly interrupted or unresolved until separately reconciled; explicit future admission MUST follow current authority and controls.
- **RCR-017**: Cleanup MUST first present a task-scoped review of exact owned candidates, their state and exclusions without removing them.
- **RCR-018**: Cleanup MUST apply only the reviewed, still-matching owned selection, rechecking identity, ownership and later modifications before each affected removal; it MUST preserve unrelated, changed or unverified items.
- **RCR-019**: Unaccepted or subsequently modified work MUST be retained unless explicit disposal authority covers that exact work; cleanup MUST report removed, retained, failed and uncertain items separately. Disposal of a previously unaccepted or modified item requires a fresh review and explicit authorization for its exact current identity and state, rechecked at removal; the review, authorization and result MUST remain auditable.

- **SC-004**: In scoped export and backup scenarios containing at least two projects and multiple artifacts, 100% of selected readable records are represented and zero unrelated unselected artifacts are included; omissions are explicitly reported.
- **SC-005**: In valid, corrupt, incomplete, incompatible and destination-conflict restore scenarios, valid selected data is recoverable, pre-effect rejected cases leave the destination unchanged, a destination change before commit is refused, interrupted partial effects remain visible and fenced, and automatic resumed writers total zero.
- **SC-006**: In cleanup scenarios with at least two tasks and one changed-after-review item, zero unrelated or unauthorized changed items are removed, fresh exact-item disposal authority is required for changed or unaccepted work, and every selected item and authorization has a truthful auditable disposition.

### Parent requirements and boundaries

This slice primarily implements FR-033 and FR-035 and extends FR-029, FR-031–032, FR-034, FR-036–038 and FR-043–048 within the stated UI/CLI and offline scope. FR-042 retention work is deferred intact to the next feature; FR-045 host integration and other planned capabilities remain subsequent work. Pause does not imply provider-session continuation. Future recovery enhancements and unsupported capabilities must be identified instead of silently promised.
