# Feature Specification: Scoped Retention and Recovery

**Feature context**: `specs/007-scoped-retention/`
**Git branch**: `refactor`
**Created**: 2026-11-08
**Status**: Draft
**Input**: Deliver useful scoped evidence export, project-state backup and verified recovery, and reviewed cleanup while preserving existing work and never inspecting or copying vendor credentials.

## User Scenarios & Testing

### User Story 1 — Export evidence with a truthful scope (Priority: P1)

A programmer needs to retain or share the evidence of a task or project. They preview the exact included and excluded material, choose a local destination and create an identifiable export without another model call.

**Independent test**: Export eligible retained evidence from one of two populated projects, inspect the bundle and manifest, and verify that unrelated project data, excluded material and vendor authentication are absent. Retry a lost acknowledgement and verify one output.

1. Given retained work with eligible and unavailable material, when the programmer previews an export, then Council identifies the scope, contents, exclusions, reasons, destination and completeness before any publication.
2. Given an eligible complete selection, when the export succeeds, then the bundle contains each selected eligible item exactly once and declares its integrity and compatibility.
3. Given required material is missing, changed, still being written or not eligible, when export is attempted, then the outcome explicitly identifies the incomplete or rejected selection and never claims complete evidence.
4. Given two projects with multiple eligible artifacts, when one project is exported, then every selected eligible member appears exactly once and no member of the other project appears.

### User Story 2 — Restore a project-state backup without replay (Priority: P1)

A programmer preserves a project's Council state and later recovers it in an isolated location, with clear information about data the backup could not include. Restoring never starts agents or modifies unrelated projects.

**Independent test**: Back up an eligible real-use project state, validate its manifest and restore it to a new isolated destination; inspect its retained identities and selected content through Council while observing zero model calls, zero automatic dispatch and no change to a second project. A synthetic fixture alone cannot satisfy this scenario.

1. Given eligible complete project state and an absent destination, when a backup is created and verified, then its selected records, relationships and required eligible evidence are preserved with an explicit scope and integrity information.
2. Given a verified compatible complete backup, when the programmer restores it to an explicitly selected absent destination, then Council preserves its recoverable records and original logical relationships in an isolated namespace without starting old tasks or writers.
3. Given a backup is incomplete, corrupt, incompatible, unsafe or conflicts with destination content or identity, when restore is requested, then Council rejects it before destination changes and explains the reason.
4. Given a project with ordinary retained work, when the programmer exports and imports the supported generated-metadata history profile, then Council makes it inspectable as incomplete history and never represents it as a complete backup or executable recovery.
5. Given actual production material cannot be proven eligible without violating the credential boundary, when the programmer requests a complete backup, then Council identifies the unavailable material and does not call the backup complete; this feature is not accepted on a synthetic-only complete backup path.

### User Story 3 — Review cleanup without losing work (Priority: P2)

A programmer reviews Council-owned artifacts for one task and explicitly selects items to dispose of, preserving unrelated work and material changed since review.

**Independent test**: Preview eligible artifacts for one of two tasks, change an item after review, choose a subset and execute cleanup; verify the unchanged authorized item is disposed, changed and unrelated items remain, and the retained audit records the outcome.

1. Given a selected task, when cleanup is previewed, then Council lists the exact owned eligible items, state, dependencies, exclusions and any additional authority required without deleting data.
2. Given selected items are unchanged and no longer required, when explicitly authorized cleanup runs, then only those verified items are disposed and their audit history remains inspectable.
3. Given an item is shared, still needed, foreign, inaccessible or unverifiable, when cleanup is requested, then Council retains it. A changed or unaccepted item requires a fresh exact review and explicit authority before it may be disposed; uncertainty is reported rather than presented as success.

### User Story 4 — Operate consistently after interruption (Priority: P2)

A programmer can inspect the same retained operation and retry a lost acknowledgement from the UI or CLI without repeating an effect or losing newer selections and drafts.

**Independent test**: Initiate an operation through one interface, lose its response, inspect and retry the identical request through the other, restart the owner at a controlled effect boundary and verify one recorded outcome or an explicit unresolved incident.

1. Given an identical saved request is replayed, when the original outcome is known, then Council returns that outcome without publishing, restoring or deleting again.
2. Given a filesystem effect is interrupted or its outcome is unknown, when Council restarts, then it displays the retained operation and uncertainty, preserves unrelated state and does not automatically repeat the effect.
3. Given an exact request identity is reused with different scope, selection or destination, when it is submitted, then Council rejects the conflict without side effects.

### Edge Cases

- A live source, artifact, destination or owner changes between preview and commit.
- A selection references a missing, shared, unreadable, unverified, hard-linked, symbolic-linked or externally owned object.
- An archive contains duplicates, absolute or traversal paths, links, excessive sizes, undeclared content, unsupported versions or invalid integrity data.
- A fault occurs before or after a publication, quarantine, restore, acknowledgement or durable receipt.
- A previous owner or restored task has uncertain process ownership or incomplete effects.
- The same destination already exists or a logical identity collides with live data.
- A selected or reachable source may contain credentials or lacks trustworthy non-credential provenance.

## Requirements

### Functional Requirements

- **RET-001**: Council MUST expose task/project evidence export, project backup and restore, and reviewed task-scoped cleanup through a coherent UI and CLI backed by the same local authority.
- **RET-002**: Council MUST preview exact scope, selected categories, exclusions with reasons, completeness and local destination before an effect and MUST reject stale or changed captured authority.
- **RET-003**: Council MUST NOT inspect, copy or hash vendor credentials; it MUST establish eligibility before reading selected content and disclose unknown or ineligible provenance without treating user selection, path names or arbitrary stored output as proof.
- **RET-004**: Council MUST provide a useful complete backup and verified restore path for qualifying real production project state, not solely for specially generated test fixtures; the design MUST identify and demonstrate how its required source data obtains non-credential provenance in normal operation.
- **RET-005**: Council MUST preserve every selected eligible required record, relationship and artifact in a complete export or backup; missing, changed, inaccessible or unqualified required data MUST make the result incomplete or rejected.
- **RET-006**: Council MUST declare format, version, source scope, member identities, size, integrity, compatibility, omissions and completeness in a verifiable manifest; it MUST explicitly identify selected project filesystem material and referenced payloads as included, omitted or unavailable, and MUST distinguish a limited history projection from a complete recoverable backup.
- **RET-007**: Council MUST validate all backup contents, integrity, references, limits, destination and collisions before restoring effects and MUST reject unsafe member paths and link types.
- **RET-008**: Council MUST restore only to an explicitly selected isolated absent destination and MUST NOT overwrite unrelated files or live state, silently merge projects, launch agents or resume previous writers.
- **RET-009**: Council MUST explicitly identify a restored project's available and unavailable history and MUST retain interrupted or uncertain ownership without automatic dispatch.
- **RET-010**: Council MUST preview exact task-owned cleanup items and require explicit selected-item authority; changed or unaccepted work requires fresh explicit authority and all dependency and ownership checks.
- **RET-011**: Council MUST preserve other tasks, shared or still-required evidence, unrelated paths, subsequent edits and prior audit history; it MUST NOT recursively remove unreviewed or unknown contents.
- **RET-012**: Council MUST revalidate source, selected item, authority and destination identity at the relevant effect boundary and MUST report per-item outcomes, retained objects and uncertainty.
- **RET-013**: Council MUST persist exact operation intent, identity, scope and observed outcomes so identical retries return the original receipt and current inspectable operation status without replay; independently authorized reconciliation may update the status, and conflicting request reuse MUST fail.
- **RET-014**: Council MUST retain and expose incomplete, failed and uncertain effects after interruption and MUST fence conflicting work where safety is unproven; acknowledgement or partial success MUST NOT imply completion.
- **RET-019**: Council MUST durably record intent before each effect and observed outcome afterwards, and provide an auditable owner-mediated action to inspect and reconcile interrupted or uncertain effects. At least a bounded no-effect case MUST be reconcilable: all operation-owned effects are governed by a validated append-only durable permit protocol; the exact operation is frozen against future permits; its actors are drained; its complete journal proves no permit was issued; and the relevant owner generation and other blockers are verified. The decision releases only that operation’s own fence and does not establish that external actors left a path unchanged. Insufficient proof MUST leave the operation unresolved and conflicting work fenced; reconciliation MUST NOT blindly replay an effect.
- **RET-015**: Retention previews, export, verification, restore, cleanup, inspection and retries MUST NOT make model calls or read vendor authentication state.
- **RET-016**: Council MUST preserve current draft and selection on stale or failed responses, provide keyboard-operable branded controls, and present the same operation identities and states across UI and CLI.
- **RET-017**: Tests MUST cover real owner and filesystem boundaries with fake agents, production-eligible and ineligible sources, two-project/task isolation, lost responses, source and destination drift, unsafe archives, interruption and zero replay.
- **RET-018**: Council MUST keep legacy state separate and MUST NOT silently migrate, delete or include it in an export or restore.

### Key Entities

- **Selection preview**: Immutable reviewed scope, eligible items, exclusions, destination, revision and any required explicit authority.
- **Retention operation**: Durable identity, exact request, intent, observed effects, result, completeness and unresolved incident.
- **Backup or export manifest**: Versioned inventory, scope, integrity, relationships, declared exclusions and completeness.
- **Restored project history**: Isolated recovered data with source relationships, unavailable fields and ownership state distinct from live executable authority.
- **Cleanup review**: Exact task-owned item identities, current states, dependency checks, captured authority and per-item outcomes.

## Success Criteria

- **SC-001**: A qualified production-use project and a separate unrelated project demonstrate a complete backup and isolated restore with 100% of selected eligible required records and content preserved and no changes to the unrelated project.
- **SC-002**: Every selected unsupported or unqualified required item is visibly identified and prevents a false complete result; no vendor authentication object is opened, hashed or copied in controlled access tests.
- **SC-003**: Corrupt, incompatible, colliding and unsafe backups are rejected before destination effects in all defined acceptance scenarios.
- **SC-004**: Cleanup preserves all unselected, shared, changed since their most recent authorized review, and unrelated items in the defined review and race scenarios, and exposes every selected item's actual result.
- **SC-005**: Repeated identical requests across UI, CLI and owner restart execute each durable effect identity or stage at most once, produce at most one logical published, restored or disposed result per selected item, and make zero model calls.
- **SC-006**: UI and CLI demonstrate the same saved selection, operation identity, status and outcome, including a 360-pixel keyboard-operable flow.

## Assumptions

- Local operation and English interface are in scope; publishing to external services and live-provider calls are not.
- Exact eligibility must be established by a trustworthy production mechanism before payload access; where it cannot be established Council reports the limitation instead of inspecting credentials or inventing completeness.
- An isolated restored history cannot itself authorize execution, integration or cleanup.
- The credential-safe production provenance mechanism is a design gate: a fixture-only path or metadata-only projection cannot close RET-004.
