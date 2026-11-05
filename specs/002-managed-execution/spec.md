# Feature Specification: Managed task execution through native Codex

**Feature Branch**: `refactor`

**Feature Directory**: `specs/002-managed-execution`

**Created**: 2026-11-05

**Status**: Draft for planning

**Input**: Make one bounded task executable and inspectable from the existing UI and CLI through
Codex's supported stable native interface. Preserve task instructions, explicit choices, saved
autonomy and dirty project work. Retain results and truthful acceptance status, coordinate local
capacity, and establish cancellation, process ownership, durable admission and recovery without
replay before adding other harnesses or a review/integration workflow.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Start authorized work with minimal configuration (Priority: P1)

A programmer opens an existing project, describes a bounded task and starts it using the saved
autonomous full-access preference. Council uses the supported native Codex connection, displays
the requested and effective settings, and retains the exact instructions used. Optional explicit
model or target choices take precedence over Auto; unavailable choices produce an actionable
blocker rather than a substitute.

**Why this priority**: The foundation becomes useful execution only when a task can actually start
without a repeated settings or permission questionnaire.

**Independent Test**: Start an offline native-Codex fixture from a saved draft in each interface.
Inspect the admitted task, settings and fixture-received instructions; verify one invocation and
no additional model calls for admission or observation.

**Acceptance Scenarios**:

1. **Given** an opened project, a saved autonomous full-access preference and a supported native
   connection, **When** the programmer starts a task with only its original description,
   **Then** one bounded attempt is admitted without a mandatory model/account/transport form or
   plan-confirmation step, and its task, attempt, operation and instruction-version identities
   are visible in both UI and CLI.
2. **Given** original multiline instructions and a shorter execution-deadline override, **When** work is
   admitted, **Then** the complete original text, selected guidance version and captured policy
   are retrievable unchanged; the narrower override governs this attempt.
3. **Given** Auto model selection and no reported effective model, **When** execution starts,
   **Then** Council identifies the native configured selection as unresolved and does not invent
   a small-model choice, provider, account identity or live qualification.
4. **Given** an explicit model, account or harness pin that is unsupported or unavailable,
   **When** admission is attempted, **Then** the affected launch does not happen and the exact
   limitation is shown without substituting another model, account, provider or payment path.
5. **Given** a requested permission mode the native connection cannot provide, **When** admission
   is attempted, **Then** Council reports the blocked capability before launch. In particular,
   a supervised choice does not promise a Council tool-approval interface that is unavailable,
   and a full-access preference does not silently become a narrower mode.
6. **Given** a queued attempt whose task guidance or task policy changes before dispatch,
   **When** its turn arrives, **Then** the obsolete snapshot is not dispatched unnoticed;
   Council identifies the changed version and requires a start against the current version.
   Already dispatched work remains linked to its original snapshot and can be stopped.

### User Story 2 - Inspect useful output without losing existing work (Priority: P1)

A programmer inspects the complete response and any authored change in an isolated working area.
Their existing staged, unstaged and unrelated files stay intact. Council distinguishes a returned
response, an authored candidate and an accepted deliverable, and exposes what still needs to be
verified instead of declaring an implementation complete merely because Codex exited successfully.

**Why this priority**: Useful retained output and honest acceptance are necessary for trusting even
one execution; generating a patch is not the same outcome as verifying or integrating it.

**Independent Test**: Execute fixtures in a project containing staged, unstaged and ordinary
untracked source. Compare the original files, staging and selected branch before and after; inspect
the isolated candidate, full response, usage observations and remaining acceptance action.

**Acceptance Scenarios**:

1. **Given** dirty source on the selected branch, **When** a writing attempt runs, **Then** its
   isolated work uses the relevant current source, including applicable uncommitted changes;
   Council records that source/base and intended destination, and leaves the user's original
   contents, staging and branch unchanged.
2. **Given** current source that cannot be captured safely or a changed physical project identity,
   **When** preparation occurs, **Then** the attempt is blocked before agent dispatch with the
   missing prerequisite; an older clean checkout is not silently used instead.
3. **Given** an authoring fixture that returns a candidate and exits successfully, **When** its
   result is inspected, **Then** the attempt's successful authoring outcome is retained while the
   task says `Verification pending`, identifies the candidate and remaining action, and makes no
   claim of passed checks, independent review, integration or task completion.
4. **Given** a response-only task with a complete retained response, no authored changes and no
   outstanding required checks or independent review, **When** the programmer inspects the
   response and explicitly records acceptance against that response and instruction version,
   **Then** the task can complete with a separate user-acceptance record. A native exit or final
   response alone leaves acceptance pending in this preview. This optional manual path does not
   establish a permanent mandatory confirmation for every answer in the finished product.
5. **Given** a fixture that writes useful partial changes and then fails, **When** the result is
   shown, **Then** the attempt and task remain failed, the partial candidate and full available
   output remain inspectable, and cleanup cannot transform that failure into success.
6. **Given** a long result and a connection that reports no usage or effective identity,
   **When** the programmer opens its evidence, **Then** complete retained output is available
   through explicit bounded reads, previews are labelled, and unreported usage, charge, allowance,
   external occupancy and identity remain unknown rather than zero or inferred.

### User Story 3 - Stop work and see whether it has really settled (Priority: P1)

A programmer stops a task through either interface. Council closes further admission, requests
termination of its owned work, drains the available evidence and shows cleanup until settlement is
verified. Failure and accepted cancellation remain authoritative even if a late result arrives.

**Why this priority**: Full-access execution needs truthful process ownership and stop behavior from
its first delivery; a stopped-looking card must not hide a remaining writer.

**Independent Test**: Use fixtures that exit cooperatively, produce delayed output, leave owned
descendants, ignore a stop request or fail before cancellation. Inspect the retained primary
outcome and cleanup state, then attempt another dispatch.

**Acceptance Scenarios**:

1. **Given** an admitted task waiting for capacity, **When** Stop is accepted, **Then** it never
   launches an agent and reaches a retained cancelled outcome with its identity preserved.
2. **Given** an active attempt, **When** Stop is accepted, **Then** UI and CLI show `Cancelling`
   while owned processes and evidence settle; the acknowledgement alone does not claim the
   process tree has exited, and a late success cannot make the task completed.
3. **Given** an authoritative failure followed by Stop or shutdown, **When** cleanup finishes or
   fails, **Then** the original failure remains primary, later cancellation/cleanup diagnostics
   are linked separately, and already obtained useful evidence is retained.
4. **Given** an owned descendant whose termination cannot be verified, **When** stop or orderly
   shutdown completes its bounded cleanup effort, **Then** Council shows unresolved cleanup,
   preserves the incident and blocks any new dispatch that could overlap that work. It does not
   remove unrelated processes, user changes or retained candidates as automatic repair.
5. **Given** an observer waiting for a task, **When** its wait times out, its CLI disconnects or its
   browser closes, **Then** the owning service retains the task and actual lifecycle; reconnecting
   observes the same attempt without a restart, duplicate invocation or implicit cancellation.
6. **Given** orderly owner shutdown during output, **When** owned work is terminated and available
   output is drained, **Then** final evidence and known outcomes are retained; incomplete evidence
   or unverified cleanup remains explicit instead of a successful shutdown erasing the incident.

### User Story 4 - Share one durable execution across clients and restarts (Priority: P2)

A programmer starts and observes work from several local clients without creating competing
owners or exceeding Council's declared capacity. Lost acknowledgements and owner crashes leave
inspectable records and useful artifacts; they do not authorize replay of an agent call.

**Why this priority**: UI and CLI already share state. Extending that foundation to writes must
preserve the same ownership and durability guarantees rather than adding a second execution path.

**Independent Test**: Race starts from two clients, drop acknowledgements, inject persistence
faults and terminate the owner during a fixture run. Restart and inspect invocation counts,
capacity, retained identities, terminal records and interrupted incidents.

**Acceptance Scenarios**:

1. **Given** two local clients or host processes targeting the same managed connection,
   **When** they submit distinct tasks concurrently, **Then** all clients use one authoritative
   admission/capacity decision and no more than the declared one active attempt runs at once.
2. **Given** a start acknowledgement is lost, **When** the identical operation is repeated,
   **Then** it returns the existing admitted operation or an explicit unresolved/conflict result
   without another invocation. Reusing its identity for different input is a conflict.
3. **Given** persistence fails before durable admission, **When** start fails, **Then** no agent
   launches. If persistence becomes uncertain after work may have started, Council retains that
   uncertainty, closes conflicting dispatch and does not rerun the agent to recreate evidence.
4. **Given** an unexpected owner exit after launch, **When** Council restarts, **Then** accepted
   terminal records remain unchanged and unfinished ownership is exposed as interrupted with
   retained instructions, artifacts and known evidence. No replacement attempt starts
   automatically and unreconciled ownership blocks conflicting dispatch.
5. **Given** repeated Stop, result inspection or response-acceptance operations, **When** clients
   retry them after lost acknowledgements, **Then** they reconcile with the original operation
   rather than duplicating effects or changing a terminal attempt outcome.
6. **Given** delayed detail/status reads and newer composer edits or a different selected task,
   **When** those reads finish, **Then** the newer draft and selection survive; both interfaces
   continue to show the same task, attempt, acceptance and cleanup facts.

### Edge Cases

- The project path contains spaces or Unicode, the selected folder is below the repository root,
  the repository has an unborn or detached head, or the selected branch changes before dispatch:
  preserve the selected context and either capture a valid source identity or block specifically.
- Required dirty or ordinary untracked source cannot be represented safely in managed isolation.
  Do not silently drop it, stash/reset the user's checkout or include ignored authentication data
  or Council state to make preparation appear successful.
- A pin is accepted by the connection but its reported effective identity contradicts that pin:
  retain the discrepancy as a failed/blocked outcome, never an honored selection.
- The native connection cannot provide supervised approvals, effective permissions, authentication
  identity, model reporting, usage, session continuity or verified descendant cleanup. Report each
  actual limitation separately; support for one capability does not imply support for the others.
- A successful process has no usable final response or candidate, or a task requires checks,
  review, integration or publication that this slice cannot perform. Keep the missing acceptance
  evidence and next action visible rather than completing the requested deliverable.
- Output exceeds the supported bound, evidence storage fills, the final response is malformed or
  the agent exits while output is still draining. Preserve obtained evidence, identify any missing
  portion and fail the affected attempt rather than silently truncating a supposedly complete result.
- Stop races dispatch, a provider failure or terminal persistence. Preserve the accepted control
  order and primary failure; duplicate acknowledgements and late output cannot rewrite it.
- An executable path, process identifier or stale owner record is reused. These observations alone
  are not proof of process ownership or permission to terminate/take over another process.
- A browser refresh, observer timeout, second owner attempt or lost reply occurs during a write.
  None of these events establishes that the task did not run or authorizes automatic replay.

## Requirements *(mandatory)*

### Functional Requirements

- **EXE-001**: UI and CLI MUST start, inspect, wait for and stop the same bounded native-Codex
  execution using shared task, attempt and operation identities. Each execution permits one worker
  attempt and MUST NOT automatically add retries, workers, model escalation or another harness.
  Each admitted attempt MUST capture a finite execution deadline; an expired deadline terminates
  owned work and reports its actual failure/cleanup state without an extra model call or an
  unbounded wait. Ordinary starts use a documented default without a required settings form.
- **EXE-002**: Start MUST require only a supported project and task description on the ordinary
  path. Auto and the saved autonomous full-access preference MUST be visible; optional pins and
  narrower task overrides MUST NOT become mandatory per-task configuration.
- **EXE-003**: Admission MUST preserve complete original instructions, the selected guidance
  version, acceptance conditions, intended destination, explicit pins and captured policy separately
  from generated output. Later guidance/policy versions MUST remain attributable and MUST NOT
  rewrite dispatched snapshots or original draft history.
- **EXE-004**: Before agent dispatch, Council MUST validate the project/source identity, task data,
  intended destination, required supported capabilities and current task version; recheck ownership,
  capacity and cancellation; and establish durable admission. A known incompatible requirement or
  missing prerequisite MUST block that launch with a specific next action. Missing capabilities
  needed for the admitted authoring stage block before dispatch; later verification/integration
  stages may remain explicitly pending when the captured task permits useful candidate preparation.
  A task requiring only a final accepted delivery cannot silently become a partial-stage task.
- **EXE-005**: New task guidance or task-policy changes MUST prevent unnoticed dispatch of an
  obsolete queued version. A change to broader saved preferences alone MUST NOT silently rewrite
  an already captured task policy; adopting that policy requires a new explicit task version.
- **EXE-006**: Council MUST use the supported stable native Codex connection and vendor-owned
  authentication without inspecting/copying vendor credentials, starting hidden login, installing
  an agent or changing vendor-global settings. Discovery and fake qualification MUST remain
  distinct from a separately authorized live qualification.
- **EXE-007**: Requested and effective autonomy MUST be recorded separately. Council MUST apply
  the saved full-access preference through supported native behavior without repeated per-tool
  confirmation. Unsupported requested modes MUST block before launch; there MUST be no silent
  downgrade, false supervised-approval interface or universal OS-sandbox claim.
- **EXE-008**: Explicit harness, model, account and payment choices MUST be honored or blocked
  without substitution. Auto in this slice MUST use the existing native configured selection,
  identify that limited selection rule, and retain an unresolved model as unknown when unreported.
  It MUST NOT claim economical cross-target routing or configure a paid API fallback.
- **EXE-009**: Requested selectors and reported effective provider, account, harness and model
  identities MUST remain distinct and sourced. A contradicted explicit pin MUST prevent successful
  acceptance; unreported identity, allowance and external occupancy MUST remain unknown.
- **EXE-010**: Writing attempts MUST use managed isolation with an attributable current source/base
  and intended destination, preserving original contents, staging, selected branch and unrelated
  files. Applicable dirty source MUST be represented or preparation MUST block. Council MUST NOT
  silently stash/reset user work or copy ignored authentication data, vendor credentials or Council
  state into the working context.
- **EXE-011**: This slice MUST retain authored changes as inspectable candidates and state that
  verification remains pending. It MUST NOT automatically integrate, commit, publish or deploy
  them, or claim checks/review that have not occurred. Full tool access alone MUST NOT create
  authorization beyond the captured task scope.
- **EXE-012**: Attempt outcome, task acceptance and cleanup state MUST be independently visible.
  A returned response or zero process exit alone MUST NOT complete a task. A write candidate MUST
  remain `Verification pending`; required unavailable review/check/integration work MUST have an
  explicit next action rather than a fictitious completed stage.
- **EXE-013**: For a response-only task with no authored changes or outstanding required checks or
  review, UI and CLI MUST allow explicit user acceptance of the exact retained response against
  its task version. This separate acceptance record MUST identify its evidence and user-review
  basis; it MUST NOT relabel agent claims as observed checks or accept a stale response version.
- **EXE-014**: Full available instructions, native output, final response, diagnostics and candidate
  references MUST be retained with stable task/attempt links and remain explicitly retrievable.
  Bounded pages/previews MUST identify omissions and permit complete reads without skipped content.
  Evidence loss or a limit breach MUST be explicit and MUST prevent a false complete result.
- **EXE-015**: Reported usage MUST retain its source and observation time. Unreported tokens, costs,
  remaining allowance or external activity MUST appear as unknown; call counts and fixture outputs
  MUST NOT become measured charges, savings or live entitlement claims.
- **EXE-016**: Lifecycle views MUST distinguish queued, running, blocked/pending acceptance,
  completed, failed, cancelling, cancelled and interrupted meanings as applicable, with the actual
  stage and next action. Partial output MUST remain available without implying successful acceptance.
- **EXE-017**: A single authoritative owner MUST coordinate admission and serialized state changes
  across local UI/CLI/host processes. This slice MUST enforce a declared connection-wide capacity
  of one active attempt across projects/clients; each client MUST NOT receive its own independent
  capacity. External native activity MUST remain separately unknown unless observed.
- **EXE-018**: Starting, stopping and recording response acceptance MUST use stable operation
  identities. An identical retry MUST reconcile with the same operation; different input with that
  identity MUST conflict. An ambiguous acknowledgement MUST NOT authorize duplicate work.
- **EXE-019**: An accepted Stop MUST close further dispatch for the task and request termination
  of admitted owned work. Cancellation MUST remain visibly in progress while process cleanup or
  evidence drainage remains active; queued cancellation MUST prevent a later launch.
- **EXE-020**: Council MUST track ownership of launched native work and its owned descendants,
  perform a bounded termination/drain effort during Stop and orderly shutdown, and expose whether
  settlement is verified. It MUST NOT terminate a process merely from an unverified stale identity
  or delete user work/candidates as process cleanup.
- **EXE-021**: Accepted cancellation MUST survive late success. An earlier authoritative failure
  MUST remain primary through later cancellation, shutdown or cleanup errors; those later events
  MUST be retained separately. Finalized attempt outcomes MUST NOT be rewritten by retries.
- **EXE-022**: Unverified process cleanup, unreconciled ownership or an unresolved persistence fault
  MUST remain actionable and block dispatch that cannot be shown safe. The interface MUST identify
  the retained incident and a supported inspection action rather than claiming successful repair.
- **EXE-023**: Persistence failure before durable admission MUST launch no agent. After work may
  have begun, persistence uncertainty MUST close conflicting dispatch, retain known evidence and
  failure context, and MUST NOT trigger replay to reconstruct missing evidence.
- **EXE-024**: Restart MUST preserve accepted terminal records and expose unfinished ownership as
  interrupted with retained artifacts and known outcomes. It MUST NOT infer that writes never
  occurred or automatically resume/retry an interrupted attempt. Full reconciliation/resume is a
  later feature; an unresolved incident MUST remain blocked in this slice.
- **EXE-025**: Closing a browser, disconnecting a CLI or ending a bounded wait MUST NOT cancel owned
  execution. Reconnection MUST inspect the same identities and actual state. A CLI-started task
  MUST have a continuing owner rather than depending on the observing command remaining attached.
- **EXE-026**: Routine admission, status, waiting and evidence inspection MUST make zero model
  calls. Ordinary views MUST use compact metadata and explicit full evidence reads; inspection
  MUST NOT restart the task.
- **EXE-027**: The UI MUST preserve newer drafts, selections and evidence against stale reads and
  lost acknowledgements. Primary start/inspect/stop/response-acceptance flows MUST remain
  keyboard-operable, have accessible names and visible focus, and use English, non-colour state
  labels and the existing approved Ejupi Labs identity.
- **EXE-028**: Execution records and managed artifacts MUST stay in the documented private Council
  state scope, excluded from Git/release assets and separate from legacy state. Errors MUST give
  bounded actionable guidance without disclosing private task bodies as routine diagnostics.
- **EXE-029**: An explicitly labelled fake execution mode MUST exercise the same lifecycle and
  retained evidence semantics without contacting a provider. Development acceptance MUST report
  fake coverage and separately unperformed live qualification honestly; this rebuild request is
  not authorization for a live provider task.

Primary parent requirements addressed, in part or in full: FR-003–FR-011, FR-018, FR-020,
FR-029, FR-031–FR-038, FR-043–FR-048. Acceptance/status boundaries also implement the relevant
parts of FR-021 and FR-024 without providing their later review/verifier workflow. These mappings
do not declare the complete parent requirements satisfied: five-harness coverage, economical
routing, full recovery, host integration and verified delivery retain their ordered later slices.

### Key Entities *(include if feature involves data)*

- **Task version**: Project-linked original task, guidance/acceptance version, explicit choices,
  captured policy and intended destination. Generated material cannot replace its authority.
- **Execution attempt**: One bounded worker invocation, linked task version, requested/effective
  target and permissions, source/candidate identities, lifecycle and immutable terminal outcome.
- **Operation**: Stable identity and exact intent for admission or a control/acceptance change;
  supports reconciliation after a missing acknowledgement without repeating the effect.
- **Managed candidate**: Retained isolated source/base and authored delta, original destination
  reference and verification-pending status. Its existence is not integration or acceptance.
- **Evidence record**: Stable linked native response/output, diagnostic, observed usage or
  acceptance evidence, with source/completeness and bounded retrieval. A preview is not the original.
- **Response acceptance**: Explicit user-reviewed conclusion about one retained response and task
  version, separate from the authoring attempt and unavailable for outstanding required checks.
- **Owner and capacity claim**: Authority over admitted work and its one shared connection slot,
  including process ownership and independently visible cleanup/uncertainty state.
- **Interrupted incident**: Retained ownership/evidence/persistence uncertainty and known artifacts
  that prevent conflicting dispatch until a supported reconciliation can establish safety.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In the ordinary prepared UI and CLI journey, a task starts using only project and
  original task text, reuses the saved operating preference and produces exactly one worker
  invocation; no mandatory transport, model/account or repeated permission questionnaire appears.
- **SC-002**: Across all controlled admission cases, original valid multiline Unicode instructions,
  explicit pins and captured task/policy versions remain unchanged and retrievable; unsupported
  pins/modes and invalid prerequisites produce zero affected agent launches.
- **SC-003**: All dirty-project authoring cases preserve the original source, staging and branch
  exactly, retain an inspectable attributable candidate, and show verification pending with no
  claim of integration. The response-only case completes only after separate recorded acceptance.
- **SC-004**: In cooperative and uncooperative offline stop cases, pending control feedback appears
  immediately, and within 15 seconds of the accepted Stop the task shows either verified settled
  ownership or an explicit unresolved-cleanup blocker. No case claims quiescence from an
  acknowledgement alone, and every tested late-success/primary-failure race preserves authority.
- **SC-005**: Concurrent starts from at least two independent local clients never exceed one
  active managed attempt on the connection. Repeated starts/controls after lost acknowledgements
  produce zero duplicate invocations or effects in every controlled reconciliation case.
- **SC-006**: Every injected pre-launch persistence fault, owner crash, evidence fault and unknown
  cleanup case retains its known outcome/artifacts, prevents unsafe conflicting dispatch and
  causes zero automatic replays after restart. Already accepted terminal records stay unchanged.
- **SC-007**: Repeated waits, status refreshes, evidence reads and observer disconnect/reconnect
  produce exactly zero additional model calls and preserve the original execution identity.
  UI and CLI agree on lifecycle, acceptance, retained result and cleanup facts in every case.
- **SC-008**: The primary execution/detail/stop flows work by keyboard at 360 CSS px and ordinary
  desktop widths, with inspected approved-brand renders, visible focus and meaningful text status;
  delayed responses do not erase a newer draft or select an obsolete task.
- **SC-009**: Each EXE requirement is linked to implementation tasks and actual acceptance evidence;
  independent review closes every blocking finding before feature 003 begins. Fake qualification,
  supported platforms and unperformed live tests are identified separately.

## Assumptions

- Feature 001's accepted project identity, private state ownership, drafts, saved policy and
  branded UI/CLI form the dependency. Spec Kit feature context remains independent of `refactor`.
- The supported project remains a local Git project. A source state that cannot be represented
  safely is a specific blocker, including unsupported repository states; dirty work is not itself
  permission to replace it with an older committed base.
- This slice supplies one native Codex worker attempt per execution and one active managed
  connection slot. Queueing is allowed; task graphs, extra workers, fallback and retry/escalation
  policies are later work. Existing unrelated native conversations remain outside Council's
  occupancy knowledge and are not modified to start this task.
- Auto uses the existing native configured selection for now. Model-first explicit user choices
  remain available when supported; selecting a suitably small model across configured targets
  belongs to feature 004 and is not fabricated by this slice.
- The default deliverable is an inspectable response/candidate. A write candidate remains pending
  verification; feature 005 supplies checks, review/correction and authorized local integration.
  Explicit user acceptance of a response-only deliverable supplies a small useful completion path
  without pretending that semantic review or required checks ran automatically.
- Supported supervision is established per connection, not inferred from the saved preference.
  Unsupported interactive approval requirements block clearly; no Council approval interface is
  implied. Full access remains the primary saved preference where the native connection supports it.
- Stop is provided now. Durable pause/checkpoint/resume, owner reconciliation, backup/export and
  reviewed artifact disposal are feature 007. Restart in this slice may expose a blocked interrupted
  incident; it does not need to claim that safe continuation is implemented.
- Other harnesses, account-aware routing, MCP/host integration, dependent handoffs and external
  publication stages follow their roadmap slices. They are not scaffolded as working features here.
- Full native output is retained within a documented finite supported bound. Exceeding that bound
  is a visible evidence fault with retained obtained output, never a silently clipped successful run.
- Native vendor authentication and billing remain vendor-owned. No subscription availability,
  paid-overage prevention, live compatibility, unknown usage value or savings is inferred from
  installation metadata, a fixture run or the harness's name.
- Tests use local fake agents and temporary projects; no live vendor call or credential inspection
  is required or authorized. Legacy collaboration data is neither opened nor migrated.
