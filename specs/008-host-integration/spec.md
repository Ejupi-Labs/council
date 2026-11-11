# Feature Specification: MCP Host Integration and Interface Parity

**Feature context**: `specs/008-host-integration/`
**Git branch**: `refactor`
**Created**: 2026-11-08
**Status**: Implemented; accepted for its documented offline scope (see [acceptance.md](acceptance.md)); originally "Reviewed for implementation"
**Input**: Provide a useful native MCP host interface to Council's existing shared local workspace, task lifecycle, routing, evidence and controls.

## Scope and meaning

A programmer connects an MCP-compatible host to Council's native local entrypoint and uses the same persistent owner, projects, tasks, attempts, evidence and control records as the local UI and CLI. The host is an interface to the existing Council lifecycle, not a second scheduler, an independent source of task truth or an external phase plan.

This feature covers the currently accepted workspace, execution, routing, workflow, task graph, recovery and incomplete metadata-history capabilities. Supported operations retain their existing authority, data, outcome, failure and uncertainty semantics. Host observation does not start an agent or make a model call. The host interface exposes full retained evidence through bounded explicit reads; the metadata-history export remains incomplete observational history and is not evidence backup, restore or cleanup.

Full scoped evidence export, production backup and verified restore, reviewed cleanup, and the remaining harness qualification remain required before release. They are not satisfied by this feature. No vendor credentials are inspected, copied or included in setup, diagnostics or tests. The supported boundary is a trusted local programmer and local Council owner, not remote multi-user tenancy or a universal operating-system sandbox.

## User Scenarios & Testing

### User Story 1 — Connect a host and work with the same task (Priority: P1)

A programmer connects their existing host using concise English setup instructions, starts work without a separate orchestration plan, and sees the same task and attempt in Council's UI and CLI.

**Independent test**: On a disposable local state directory, connect a real protocol client to the native entrypoint. Create a project and fake-agent task through the host, inspect and control it through CLI/UI, then create a task through another interface and inspect it through the host. Verify stable identities, captured instructions, exact recorded outcomes and one shared owner.

**Acceptance scenarios**:

1. **Given** no compatible owner is running, **When** a host starts Council, **Then** it can connect to the managed owner without requiring a separate manually maintained plan or provider setup wizard.
2. **Given** a compatible owner already serves the UI or CLI, **When** another host connects, **Then** it uses the same records and shared capacity rather than starting competing work under an independent owner.
3. **Given** a stale or incompatible owner or unsupported operation, **When** the host connects or requests that capability, **Then** Council reports an actionable limitation without silently replacing, deleting or migrating state; a separately recognized older maintenance contract continues to permit its narrowly scoped Stop.
4. **Given** ordinary operation, an error or shutdown, **When** the host reads the protocol stream, **Then** every emitted message is protocol-valid and diagnostics cannot corrupt that stream.

### User Story 2 — Observe complete evidence and control work safely (Priority: P1)

A programmer waits for an attempt, inspects its retained evidence and explicitly stops or pauses work when needed, without confusing observation or a partial response with successful completion.

**Independent test**: Run controlled fake attempts with long, multibyte and non-text evidence. Read every bounded byte page through the host and compare reconstructed bytes and metadata with the owner and CLI. Interrupt an observing host, then inspect and explicitly stop the same still-owned task from another interface.

**Acceptance scenarios**:

1. **Given** retained evidence larger than a single response, **When** the host follows the documented bounded pages, **Then** it can reconstruct the exact original bytes in order without clipping, duplication or Unicode-boundary corruption.
2. **Given** a growing evidence record, **When** the host starts an explicit read, **Then** subsequent pages retain the same captured end boundary; later bytes require a new explicit observation and are not silently mixed into that read.
3. **Given** an active attempt, **When** a wait times out, a host request is cancelled, or the host disconnects, **Then** observation ends while the task remains owned and inspectable; none of those events alone requests task Stop.
4. **Given** explicit Stop, **When** the owner acknowledges it, **Then** the host distinguishes accepted cancellation from settled cleanup and preserves partial evidence, earlier primary failures and any unresolved cleanup fence. Late success cannot erase accepted cancellation or failure.
5. **Given** paused work, **When** admitted work drains, **Then** the host exposes the durable checkpoint and requires an explicit authorized resume; it does not replay completed work.

### User Story 3 — Retry an exact request across interfaces (Priority: P1)

A programmer can recover from an interrupted acknowledgement without creating a duplicate task, broadening captured authority or overwriting a newer result.

**Independent test**: Commit a fake task start or control, intentionally lose the host response, reconnect and retry the same stable request and owner-equivalent captured intent through the host and another interface. Compare original receipt, identity and persisted result; try effect-changing input under the same identity and verify a conflict with no second effect.

**Acceptance scenarios**:

1. **Given** a committed mutation with a lost response, **When** the same durable request identity and owner-equivalent captured intent are retried, **Then** Council returns the original recorded receipt and result or a truthful current unresolved outcome, without repeating the effect.
2. **Given** a request identity already bound to a different operation, scope, revision or effect-changing input, **When** it is reused, **Then** Council rejects the changed request before a conflicting effect.
3. **Given** a host-request cancellation or transport loss while mutation outcome is unknown, **When** the user reconnects, **Then** Council retains the original request identity and makes exact retry or inspection possible; it does not report cancellation as proof that a durable effect did not occur.
4. **Given** a stale control revision, missing owner proof or unresolved shared fence, **When** a host submits a control, **Then** the existing conflict or blocked outcome remains visible and no authority is refreshed implicitly.

### User Story 4 — Use accepted orchestration and recovery from a host (Priority: P2)

A programmer can use Council's accepted routing, verified workflow, dependent task groups and recovery controls from the host while retaining the same policies and evidence seen elsewhere.

**Independent test**: With offline fake agents, inspect and amend routing under captured revisions; advance a verified workflow and a graph with parallel siblings and a dependent node; pause, inspect checkpoint, explicitly resume and stop the appropriate scope. Compare receipts, evidence, shared capacity, blockers and outcomes with UI/CLI, including an unrelated incident.

**Acceptance scenarios**:

1. **Given** a routing policy, context/profile, declared account/link, capacity report or task budget, **When** the host inspects or changes it, **Then** the same revision, eligibility, allowance and unknown telemetry semantics apply as in the other interfaces.
2. **Given** a verified workflow or task graph, **When** the host requests an accepted start, advance, guidance, check, grant or Stop action, **Then** Council checks the same captured task, graph, node, budget and workflow authorities and retains the same linked history, review, findings, changes, delivery and evidence.
3. **Given** two host connections or another interface acting concurrently, **When** they request work, **Then** shared capacity, dependencies and existing fences remain effective across them; an independent sibling's result is retained when another fails or is stopped.
4. **Given** an uncertain owner, incident or retained claim, **When** the host inspects or requests reconciliation, **Then** Council uses owner-established evidence, releases only proven safe authority and preserves other blockers. User-supplied assertions do not prove cleanup or effects.
5. **Given** the accepted metadata-history feature, **When** the host captures, inspects, previews, exports or reconciles its scoped operations, **Then** it exposes the same incomplete-history notice, exclusions, destination and limitations as the UI and CLI without claiming backup or restoration.

### User Story 5 — Configure and diagnose without unnecessary settings (Priority: P2)

A programmer installs and connects Council to a supported host using a short, accessible setup flow and can diagnose unsupported versions or an unavailable owner without exposing secrets or changing provider configuration.

**Independent test**: Follow the documentation with a clean disposable profile and a representative offline host client, including a path with spaces and Unicode. Verify a successful connection, an intentionally incompatible client/owner, actionable English errors, and accessible names and keyboard use for any new setup controls.

**Acceptance scenarios**:

1. **Given** a supported host, **When** the user follows the documented configuration, **Then** the host can discover Council's capabilities and complete the first fake task without mandatory low-level transport, agent-panel or model settings.
2. **Given** an unsupported protocol or unavailable capability, **When** the user connects, **Then** Council identifies the supported boundary and next action instead of claiming compatibility or silently altering configuration.
3. **Given** any setup interface, **When** it presents connection, error or retry controls, **Then** they have clear English labels, visible focus and keyboard access; original user instructions and evidence are never translated or rewritten.

### Edge cases

- Two hosts concurrently submit the same durable request, or submit different bodies under the same request identity.
- An owner closes, changes generation, loses its lock or cannot durably record an incident while a host request is in flight.
- A host sends malformed, oversized, duplicate, truncated or unsupported protocol input, or exits midway through a response.
- A response is lost after commit, an observer is interrupted before or after a wait timeout, or the task finishes while an observation is pending.
- Evidence is empty, growing, binary, larger than one page, changes unexpectedly, has a stale boundary, or fails identity/integrity checks.
- Another interface changes task instructions, graph authority, routing capacity, policy, checkpoint or destination after the host observed them.
- A local launch path contains spaces or Unicode; an existing incompatible owner holds the lock; a host cannot access its selected scope.
- An unknown provider capability, quota, price, usage figure or interrupted cleanup is presented without fabricating a value or success.

## Requirements

### Functional Requirements

- **HI-001**: Council MUST provide a native local MCP entrypoint with protocol-compliant standard-input/standard-output communication, capability discovery and documented supported host setup. Standard output MUST contain only valid protocol messages; diagnostics MUST use a separate channel and MUST NOT expose secrets.
- **HI-002**: The host MUST use the established persistent managed owner and the same project, task, attempt, receipt, evidence, policy and control records as UI and CLI. A host connection MUST NOT create a parallel owner, independent scheduler or separate task lifecycle.
- **HI-020**: A known older owner with a separately recognized maintenance contract MUST retain its narrowly scoped Stop path even when new host execution is incompatible. Council MUST preserve the running owner and lock and MUST NOT replace or clear it to gain compatibility.
- **HI-003**: The host MUST expose accepted workspace capabilities: open/list/inspect projects; inspect/update autonomy policy where supported; list/refresh available agents; and create, update, list and inspect retained drafts with the existing revision and original-text semantics.
- **HI-004**: The host MUST expose accepted task and attempt capabilities: start, list, inspect and inspect versions; record guidance; explicitly start a later attempt; inspect attempts, events and artifacts; wait; request Stop; and accept a response only with its existing explicit evidence and authority checks.
- **HI-005**: The host MUST expose accepted routing and capacity capabilities: inspect/update routing policy, contexts and profiles; manage declared accounts and links; inspect/report capacity; inspect routing decisions, budget ledger and usage; and grant or adopt task budget only under existing revision and policy rules. Unsupported pins, telemetry and paid paths remain honestly identified.
- **HI-006**: The host MUST expose accepted verified-workflow capabilities, including preferences, adoption, advance, explicit check and inspection of stage, review, finding, check, candidate, change and delivery history. It MUST preserve the difference between agent claims, observed checks, acceptance and actual local or external delivery.
- **HI-007**: The host MUST expose accepted graph capabilities: start/list/inspect definitions and shared budget; inspect inputs, members, dispatches and ledger; explicitly advance, guide, grant, stop a node or stop a group under the same exact graph/node/control/budget authority and dependency rules as UI/CLI.
- **HI-008**: The host MUST expose accepted pause, checkpoint, explicit resume, incident inspection and reconciliation for supported task, member and graph scopes. Existing durable drain, ownership, evidence, primary-failure, unknown-cleanup and no-replay semantics MUST remain effective.
- **HI-009**: The host MUST expose accepted metadata-history capture, summary/page inspection, export preview, explicit export, operation inspection and no-publication reconciliation with the same exact identities, notices, omissions, disclosed destination, authority and limitations as UI/CLI. It MUST NOT present this inventory as complete evidence, backup or restore.
- **HI-010**: Every effect-capable host request MUST carry or retain a stable durable request identity distinct from its transport correlation identity, bind the existing owner-canonical operation, target, intent and authority, and return the existing receipt/result for an equivalent retry. Effect-changing reuse MUST conflict before another effect; established owner-equivalent omitted/default spellings may replay the same receipt and MUST be documented, lost replies MUST remain inspectable or safely unresolved. The complete immutable input and captured graph/owner/node/budget authority MUST either be explicit and retained by the caller or durably frozen before effect; retries across interfaces MUST NOT silently re-observe or rebind fresher authority.
- **HI-011**: A timeout, protocol cancellation or loss of a host observation MUST stop only the observation unless the programmer explicitly requests a supported task control. For an in-flight mutation with an unknown acknowledgement, Council MUST report uncertainty and retain exact retry/inspection; it MUST NOT infer that the effect was cancelled or did not commit.
- **HI-012**: Explicit task or group Stop MUST preserve the accepted control, cleanup state, partial results, primary failure and retained evidence under existing owner rules. A late completion MUST NOT reverse cancellation or failure, and unresolved cleanup or persistence MUST continue to fence conflicting work.
- **HI-013**: Routine status, wait and list operations MUST be bounded and MUST NOT invoke a model. All accepted retained task, attempt, event, review, verifier, change and artifact evidence MUST be retrievable through stable linked identities and bounded pages without substituting a preview for the original.
- **HI-014**: Explicit artifact byte reads MUST provide an exact offset, bounded data, next offset, captured end boundary and completion indication. A client MUST be able to reconstruct all original bytes, including binary and multibyte sequences, without implicit truncation, repeated bytes or unrelated data; integrity or identity failures MUST be reported rather than repaired by rerunning work.
- **HI-015**: Multiple hosts and UI/CLI MUST share admission, graph dependencies, budget, owner epochs and capacity fences. Concurrent duplicates, stale authorities, owner restart and unavailable state MUST NOT authorize additional dispatch, overwrite user edits or erase unrelated results.
- **HI-016**: Protocol requests and responses MUST enforce documented size, shape, version, identity and paging limits. Invalid, unsupported or incompatible requests MUST fail safely and actionably without silently discarding input, changing state or claiming task success.
- **HI-017**: Setup, capability descriptions and diagnostics MUST be concise, English, accessible and sufficient for a supported host to connect without unrelated configuration. Unsupported harness features and unverified host/provider versions MUST be disclosed. Existing user choices, task text and original evidence remain verbatim.
- **HI-018**: Council MUST NOT inspect, copy, disclose or log vendor authentication material. Host setup MUST use existing local owner protections without leaking its private access material into protocol diagnostics, task evidence or generated configuration examples. No external publication, provider call or policy change is authorized merely by connecting a host.
- **HI-019**: Offline acceptance MUST use controlled fake agents and exercise a real host client, at least two concurrent host connections, UI/CLI interoperation in both directions, lost acknowledgements, malformed input, wait/cancel versus Stop, complete multi-page evidence, graph/recovery controls, owner restart and retained user work. Live provider qualification requires a separately requested live task.

### Key Entities

- **Host connection**: A protocol session to the local Council interface, with capabilities and correlation identities but no independent task ownership.
- **Durable request and receipt**: Stable operation identity, captured input/authority and committed outcome shared across interfaces and independent of one transport session.
- **Task, attempt and orchestration state**: Existing Council records and linked routing, workflow, graph, budget, checkpoint and incident authorities.
- **Evidence read boundary**: An identified retained artifact and bounded ordered byte range that can be followed to reconstruct the captured original.

## Success Criteria

### Measurable Outcomes

- **SC-001**: In offline fixtures, a host-created task and tasks created through both UI and CLI are each observed through the other interfaces with identical stable task/attempt identities, recorded outcomes and receipts where applicable.
- **SC-002**: Across repeated, interrupted and concurrent mutation fixtures, each durable request causes at most one admitted effect; every effect-changing reuse is rejected while documented owner-equivalent default spellings replay and every lost acknowledgement is replayable or explicitly unresolved.
- **SC-003**: For each controlled long, binary and multibyte artifact, concatenating all bounded host pages through one captured end boundary produces exactly the retained bytes and digest; no page silently skips or duplicates content.
- **SC-004**: In all controlled wait timeout, request cancellation and host disconnect cases, observation ends without an implicit task Stop; explicit Stop, pause and resume preserve the owner-reported state, evidence and required fences.
- **SC-005**: A real offline host client can exercise each accepted operation family in HI-003–HI-009 with the same captured authority and observable result as the shared owner, including concurrent capacity and an independent incident.
- **SC-006**: Protocol validation fixtures observe no non-protocol standard-output bytes on startup, ordinary operation, error or shutdown; unsupported requests receive an actionable outcome without side effects.
- **SC-007**: Supported-host setup succeeds from a clean controlled profile with spaces and Unicode in its paths; any new setup UI is keyboard-operable with visible focus and English labels.
- **SC-008**: Independent review, focused and full offline checks, exact-source hosted results and platform limitations are recorded before acceptance; no unperformed live provider qualification, complete retention or release completion is claimed.

## Assumptions

- The existing owner is local to the programmer and retains authority independently of individual host observations; no multi-user remote service is introduced.
- The host can supply or retain a stable operation request identity for effects. Transport request identifiers alone are not assumed durable across reconnects.
- The initial supported protocol and host-version compatibility set will be specified and verified during design, with unsupported combinations reported explicitly rather than guessed.
- Established Council capabilities and limits remain authoritative; this feature does not enable unsupported paid APIs, universal sandboxing, provider quota telemetry or additional harness integrations.
- Full evidence export, backup/verified restore and reviewed cleanup remain separate mandatory pre-release work; metadata-history export does not fulfill them.
