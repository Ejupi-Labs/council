# Feature Specification: Claude Code interoperability

**Feature Branch**: `refactor`

**Feature Context**: `003-claude-interoperability`

**Created**: 2026-11-06

**Status**: Specification and design/task consistency independently reviewed; implementation pending

**Input**: Add existing Claude Code access to Council's accepted task lifecycle, alongside Codex.
Keep the ordinary journey short, preserve native account boundaries and useful evidence, reuse the
saved autonomy preference, and prepare for later economical routing without claiming it exists.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Give an ordinary task to Claude (Priority: P1)

A programmer selects Claude for a task while continuing their personal Codex conversation
independently. They use the existing project and composer without preparing connection files.

**Why this priority**: A second working harness makes interoperability useful and enables the
later automatic use of suitable available account capacity.

**Independent Test**: Start one labelled local Claude fixture through UI and CLI, inspect its
captured target and policy, and retrieve its authoring result without a provider call.

**Acceptance Scenarios**:

1. **Given** an opened supported project and saved autonomous preference, **When** Claude is
   selected and a task starts, **Then** one explicit attempt preserves complete original text,
   target, guidance, acceptance conditions and policy without a transport questionnaire.
2. **Given** ordinary Auto or previously captured Codex work, **When** Claude becomes available,
   **Then** existing selection meaning and historical work remain unchanged. Economic cross-target
   routing is not implied by adding a connection.
3. **Given** an installed supported native Claude connection, **When** work starts, **Then** native
   authentication and applicable project instructions are preserved without Council reading
   credentials, changing global settings, starting hidden login or installing an agent.
4. **Given** an unsupported requested model, mode or prerequisite, **When** it is evaluated,
   **Then** the affected launch blocks specifically where incompatibility can be established, or
   retains its actual native failure. Another harness/account/payment path is not substituted.

### User Story 2 - Inspect what Claude actually produced (Priority: P1)

The programmer retrieves the full available response, diagnostics and attributable candidate in
the existing task detail. Requested settings, observed model use, native estimates, task acceptance
and process cleanup remain distinct facts.

**Why this priority**: A different output format must not weaken Council's evidence contract.

**Independent Test**: Exercise controlled success, error, partial-output, contradictory-result and
multiple-model cases; compare UI/CLI facts and original project state.

**Acceptance Scenarios**:

1. **Given** usable authoring output with complete required proof, **When** settlement finishes,
   **Then** its response/candidate remains attributable. A write candidate stays verification
   pending; optional response-only acceptance retains its existing separate proof requirements.
2. **Given** a successful-looking native label alongside an error, unusable output, incomplete
   input delivery, nonzero root exit or unresolved cleanup, **When** settlement is attempted,
   **Then** it cannot become successful authoring or accepted work.
3. **Given** output or diagnostics after a final-looking response, **When** the native process
   finishes, **Then** trailing available evidence remains retained. A response is not proof that
   the process exited or its owned background work and spending stopped.
4. **Given** requested model aliases, configured settings and usage listing several actual models,
   **When** details are inspected, **Then** their meanings and sources stay separate. A configured
   model is not proof of all actual use; estimates do not become charges or subscription allowance.
5. **Given** native prose requesting another worker or broader authority, **When** Council receives
   it, **Then** it remains evidence and creates no orchestration command or authorization.
6. **Given** retained Council history and an optional native session identifier, **When** continuity
   is inspected, **Then** supported resume behavior is declared separately. Council history does
   not imply that a native conversation can be resumed, and an unrelated personal chat is untouched.

### User Story 3 - Control mixed-harness work consistently (Priority: P1)

Independent UI/CLI clients start, inspect, guide and stop exact attempts under the existing owner,
queue, recovery and failure rules, without duplicate work or loss of newer edits.

**Why this priority**: Interoperability must preserve understandable ownership and control.

**Independent Test**: Concurrent independent clients admit Codex and Claude fixtures to one shared
managed slot, stop the active attempt, observe safe queue transfer and inspect retained histories.

**Acceptance Scenarios**:

1. **Given** concurrent starts for different harnesses, **When** capacity is assigned, **Then** at
   most one managed native worker is active in this slice. Queued work retains its captured target;
   Council occupancy does not establish external activity or account allowance.
2. **Given** a lost reply, **When** the same start/control/acceptance identity is retried, **Then**
   the original operation is reconciled without duplication. Changed intent conflicts.
3. **Given** new guidance selecting another supported harness, **When** an older version is queued,
   **Then** obsolete dispatch is prevented. A later explicit attempt uses the new version while
   earlier attempts and observations remain unchanged.
4. **Given** Stop, earlier failure, deadline or owner loss racing late output, **When** work settles,
   **Then** ordered authority, bounded scoped cleanup, retained partial evidence, uncertainty fences
   and no-replay recovery remain effective. Observer loss never cancels owned work.

### User Story 4 - Upgrade without stranding existing work (Priority: P2)

The programmer retains previous tasks, drafts, receipts and possibly an older continuing owner
when installing the Claude-capable preview. They can deliberately stop a known compatible owner.

**Why this priority**: Useful work must survive the transition from a background-owning release.

**Independent Test**: Upgrade populated previous-preview state and exercise newer-client Stop
against a separately running known older-owner fixture.

**Acceptance Scenarios**:

1. **Given** existing Codex records, evidence, claims and incidents, **When** state is upgraded,
   **Then** their exact identities and meaning remain unchanged without replay or silent reset.
2. **Given** an older owner supporting a known maintenance Stop contract, **When** the user requests
   service Stop through the newer client, **Then** it targets that exact owner safely. Stop
   compatibility does not falsely establish compatibility for new Claude execution.
3. **Given** unknown or incompatible ownership, **When** compatibility cannot be established,
   **Then** Council provides actionable guidance without automatic termination, lock deletion or
   inference that unfinished work did not run.

### Edge Cases

- Only a shell launcher is discoverable, the native interface is too old, or a managed installation
  denies the requested operating mode.
- Authentication is unavailable or interactive, or a rate limit/internal retry occurs. Preserve
  actual limitations without reconstructing allowance or starting another worker.
- A native option would bypass existing account authentication or suppress applicable project
  instructions. It cannot be silently selected to simplify execution.
- Aliases resolve differently by provider, usage lists several models, or a primary model cannot
  be attributed. Do not invent a contradiction or claim all work used the requested model.
- Results contain simultaneous success/error facts, are missing or contradictory, appear more than
  once, or are followed by observations. Ambiguous completion cannot imply success.
- Output exceeds supported message/total bounds, is malformed or loses evidence. Preserve obtained
  bytes, expose the actual limit and fail the affected result rather than silently clipping success.
- New guidance, newer typing or another selected task races delayed replies. Preserve both the
  captured execution intent and newer local work.
- Stop races native start, evidence writes or another client; preserve one owner and ordered facts.
- Existing state is unsupported, corrupted or retains uncertain ownership during upgrade. Preserve
  it and block incompatible work rather than resetting or reassigning claims.

## Requirements *(mandatory)*

### Functional Requirements

- **CLA-001**: Claude Code MUST use the same UI/CLI task lifecycle as Codex, with one explicit
  managed worker per attempt and no automatic retry, escalation or fallback.
- **CLA-002**: Claude selection MUST be optional per task and use the ordinary project/task journey.
  Existing Auto and captured Codex selections retain their documented meaning until feature 004.
- **CLA-003**: Complete original instructions, target, guidance, acceptance conditions, choices and
  policy MUST stay attributable and immutable per dispatched version. Guidance prevents obsolete
  queued dispatch without rewriting history.
- **CLA-004**: Saved autonomous full access MUST map only to supported native behavior without
  repeated tool confirmation. Native denials and restrictions remain visible; no silent downgrade
  or universal OS-sandbox claim is permitted.
- **CLA-005**: Authentication MUST remain vendor-owned. Council MUST NOT read/copy credentials,
  change global vendor settings, start hidden login, install an agent or silently replace existing
  subscription access with an API-credential path.
- **CLA-006**: Native instruction behavior MUST be preserved within the declared captured-source
  scope. Instruction/customization suppression cannot be silently chosen for orchestration;
  unrepresentable required context remains explicit.
- **CLA-007**: Supported explicit model selectors MUST keep their native meaning or block without
  substitution. Requested selectors, configured settings and sourced actual models remain distinct,
  including aliases and several reported models.
- **CLA-008**: Successful authoring MUST require absence of an authoritative native error, verified
  root exit, complete input delivery, usable output and the existing durable-evidence/scoped-cleanup
  proof. A native success label alone is insufficient.
- **CLA-009**: Full available response, output, diagnostics, source/candidate references and native
  observations MUST remain retrievable with exact attempt provenance, including trailing evidence.
- **CLA-010**: Usage MUST retain source, observation time and scope. Estimates, actual model observations, configured
  settings, account identity, allowance and charges MUST NOT be conflated. Unknown stays unknown;
  fake data never becomes measured provider usage.
- **CLA-011**: Native prose and peer outputs MUST remain evidence, not executable orchestration,
  expanded authorization or permission to add an account/payment path.
- **CLA-012**: Existing source isolation, original edit/staging/branch preservation, candidate
  attribution and response-only acceptance rules MUST apply to Claude. This slice does not
  integrate, commit, publish or claim unperformed review/checks automatically.
- **CLA-013**: One authoritative owner MUST coordinate one global managed-native slot across both
  harnesses. Actual harness/connection identity remains distinct from shared capacity and unknown
  external activity or allowance.
- **CLA-014**: Stable request identities MUST reconcile repeated starts, controls and acceptance
  across clients/upgrades without duplicate effects or changing the captured target.
- **CLA-015**: Stop, deadline, primary failure, evidence faults and owner loss MUST retain existing
  ordered outcomes, bounded cleanup, fences and no-replay recovery. Observer loss never cancels work.
- **CLA-016**: Metadata, status, waits, routing-free admission checks and evidence inspection MUST
  make zero model calls. Prerequisite inspection is bounded and does not imply authentication,
  live-task qualification or measured allowance.
- **CLA-017**: Discovery and task views MUST distinguish installed, supported, incompatible, unknown
  and separately qualified states. UI/CLI expose the same target, lifecycle, evidence and next
  action in concise English with accessible controls and approved Ejupi Labs identity. Documented
  interface/version assumptions, sourced observed versions and qualification evidence MUST remain
  distinct; an unknown version cannot become verified compatibility.
- **CLA-018**: Upgrade MUST preserve previous task/draft versions, observations, receipt results,
  terminal records, evidence, claims and incidents without reinterpretation, replay or silent reset.
- **CLA-019**: Explicit service Stop MUST support a known compatible older owner by exact identity,
  independently of compatibility for new execution. Unknown compatibility authorizes neither
  termination/lock removal nor new Claude dispatch.
- **CLA-020**: Development qualification MUST use labelled local Claude fixtures for native-shaped
  success/error/partial output, control and unsupported capabilities. Fake coverage, native platform
  checks and unperformed live qualification are reported separately; no live task is authorized.
- **CLA-021**: Native session identity and continuity capabilities MUST be declared separately from
  retained Council task/history. Unsupported native resume MUST remain explicit; no existing
  unrelated native conversation may be resumed, reconfigured or treated as owned work implicitly.

Primary parent requirements addressed in part or in full: FR-002–FR-005, FR-007–FR-011, FR-018,
FR-020, FR-029, FR-031–FR-038 and FR-043–FR-048. Later routing, integration and host features retain
their own completion gates.

### Key Entities *(include if feature involves data)*

- **Captured native target**: Harness and model selection of an exact task version, separate from
  later reported provider/account/model observations.
- **Native observation**: Sourced settings, actual-model usage, diagnostics and lifecycle evidence,
  without inferred billing, permission or allowance facts.
- **Shared capacity**: The owner's one managed-native slot, independent of the occupying harness
  and of external vendor conversations.
- **Claude result**: Attributable response/candidate evidence with separate attempt outcome, task
  acceptance and cleanup conclusions.
- **Native session observation**: An optional sourced native identifier and declared continuity
  capability, separate from Council history and unrelated existing conversations.
- **Maintenance compatibility**: Known operation support and exact older-owner identity permitting
  explicit Stop without granting compatibility for another operation or owner.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Prepared UI/CLI Claude journeys require only project/task plus optional Claude choice,
  produce exactly one worker and introduce no mandatory connection file, wizard or repeated prompt.
- **SC-002**: Every controlled success/error/partial/ambiguous case preserves its original text and
  available evidence; missing proof or a native label alone never produces false success/acceptance.
- **SC-003**: Concurrent independent clients targeting different harnesses never exceed one managed
  worker; retries add zero duplicates and queue transfer preserves exact captured targets.
- **SC-004**: Controlled Claude Stop/deadline/failure/owner-loss cases satisfy the existing bounded
  settlement-or-explicit-blocker, ordered-outcome and no-replay rules; Codex regressions still pass.
- **SC-005**: Every controlled alias/multiple-model/usage case separates requested and reported
  information and native continuity capability. No fake or estimated value appears as a charge,
  live quota, login success or saving, and retained Council history never implies native resume.
- **SC-006**: Populated previous-preview state upgrades without changing original records or exact
  receipt results. Newer-client Stop reaches a known compatible older owner; incompatible new
  execution and unknown takeover remain blocked.
- **SC-007**: Selection/start/inspect/Stop work by keyboard on desktop and 360 CSS px, retain newer
  edits/selection under delayed replies and preserve approved branding.
- **SC-008**: All CLA requirements map to tasks and observed evidence, independent review closes
  blockers and all supported native/race gates pass before the next feature's implementation.

## Assumptions

- Feature 002 is accepted: all seven native/race jobs passed at f5edd96 in run42140818152.
- This slice adds native Claude authoring, without a Council-managed API/SDK execution path,
  automatic economical routing, account allowance estimates, extra workers, dependent handoffs,
  independent semantic review, deterministic verification or local integration.
- One global managed slot remains intentional. Account routing/capacity and parallel task graphs
  follow in features 004 and 006.
- Auto retains Codex's configured-native meaning until feature 004. Claude uses its configured
  native model unless the user supplies a supported explicit selector.
- Native authentication, billing, user customizations and internal retries remain vendor-owned.
  Worker invocation count is not an invented cap on undocumented model calls or charges.
- Response bounds can differ by adapter; a total bound does not override a stricter message bound.
  Either limit produces an explicit evidence failure when exceeded.
- Existing source-support boundaries, private state, English copy and branded UI remain the
  prerequisite. Legacy Council data is neither opened nor migrated.
- Development uses local fakes only. Real Claude/Codex work requires a specifically authorized
  live task beyond this rebuild request.
