# Data Model: Scoped Retention and Recovery

This document defines the proposed logical records and invariants for the current feature. It does not claim that storage tables, filesystem operations or public interfaces already exist. Existing task, graph, attempt, artifact, recovery, delivery and receipt records remain authoritative. Field names here describe meaning; exact storage and wire types, limits and supported archive format are determined by the plan and contracts. IDs, revisions, counts and sizes use the project's bounded, lossless conventions.

## Payload eligibility: unresolved design gate

**Eligibility policy is a placeholder pending research and the user's answer.** Every selected payload needs a trustworthy eligibility decision before its bytes are opened, copied, hashed or otherwise inspected. An item with missing, expired, conflicting or unverifiable provenance is `unknown` or `ineligible` and must be reported without content access. User selection, filenames, artifact type, a recorded digest, a closed stream or Council ownership do not establish that vendor credentials are absent. This model neither invents an eligibility classifier nor permits inspecting suspected credentials to decide eligibility.

The policy must identify a practical, independently verifiable production path for the required records and payloads of at least one useful complete project backup and isolated restore. Until that path and its ordinary production provenance are demonstrated, RET-004 and SC-001 remain unsatisfied. Synthetic fixture provenance, a metadata-only projection, or labelling unknown content safe cannot close that gate. Credentials are never inspected, copied or hashed; inability to establish eligibility results in exclusion, an incomplete result or rejection as appropriate.

## Exact selection and review

**SelectionPreview** is an immutable, versioned review record containing:

- Preview ID and revision, operation kind (`evidence_export`, `backup`, `restore` or `cleanup`), creating owner epoch and creation time; selected task or project identity, and graph membership where relevant.
- Explicit selected categories and exact item identities. Each item binds its owning project, task, attempt or other source, immutable version or revision, role, required/optional designation, relationship references and captured eligibility decision plus provenance reference when available. A selection includes only its named scope and declared dependencies.
- A bounded inventory of included candidates, excluded or unavailable candidates and reason codes, anticipated completeness, and declared destination for operations that create output. Unknown sizes or unavailable facts are represented as unknown rather than estimated as complete.
- Source and target authority needed for later revalidation: owner, controls, version/revision and relevant pause, claim, incident or active-writer state; for physical objects, the expected Council-owned root, relative name, native object and parent identities, file kind, length and digest **only when already known or eligible to compute**. Captures are observations, not continuing permission.
- Review disposition and a reference to any required fresh item-specific authority. A new review creates a new ID or revision; it never silently rewrites an accepted review or broadens the selected scope.

A preview is read-only and causes no export, restore or deletion. It is a point-in-time inventory, not a reservation or proof that the source or destination remains unchanged. Missing or ineligible required objects preclude a complete result. Exact inclusion and exclusion lists remain distinguishable from a limited history projection.

**CleanupReview** specializes the preview for one task. Each candidate is bound to its exact owned item and current state, sharing and dependency references, acceptance status, other active uses and any unaccepted or changed-work condition. The record names retained and ineligible objects and why they cannot be disposed. An unknown descendant, changed identity, live use or unresolved dependency is retained; a directory is never authority to recursively dispose of unreviewed entries.

**CleanupAuthority** is a distinct, explicit authorization referencing the review ID/revision and the exact selected item set and reviewed facts. It records its own identity, requester, time and any fresh specific authority for unaccepted or changed work. Authority for one object does not cover a replacement, new descendant, related task, shared dependency or changed item. A changed item requires a fresh review and fresh exact authority, followed by all ordinary ownership and dependency checks; user approval does not override an unknown conflict. Authority is single-purpose and cannot expand at execution or on retry.

## Retention operation and effects

**RetentionOperation** is a durable record containing operation ID and revision; request ID, operation kind and canonical request fingerprint; preview ID/revision and exact scope, item selection and destination; accepting owner epoch; current state; source, authority and destination preconditions; associated manifest or imported-history ID; durable per-item effect IDs; result, exclusions, omissions, completeness and next action; and timestamps. Sensitive payloads, credentials, arbitrary content and private authentication material are not copied into operation metadata or diagnostics. Original audit and primary failure records remain available.

The request fingerprint covers all meaningfully authorized inputs, including preview revision, selected item identities, operation mode, target identity and cleanup-specific authority. A retry of the same request and input first resolves the saved receipt or retained operation. Reusing an ID for different input is a conflict. A lost acknowledgement cannot grant another publication, deletion, restore or writer; if a committed result cannot be established, return an explicitly uncertain outcome and the same inspection identity.

**EffectIntent** is an append-only, ordered record for each externally observable filesystem action, associated with one operation and exact item or bundle. It records effect ID and sequence, action, source and destination identities, captured preconditions, authority and an immutable fingerprint. A durable intent and current exact permit precede each action. The operation does not infer that a prepared or started effect occurred merely because a permit exists.

**EffectObservation** records an effect's actual disposition (`not_started`, `applied`, `retained`, `rejected`, `failed` or `uncertain`), resulting object identity where safely established, bounded diagnostics and evidence references, durability observation, and time. It distinguishes known failure before an effect from unknown outcome after an effect may have started. Observations and recovery decisions are append-only; neither erases a prior failure nor replaces primary outcomes with transport success.

**OperationReceipt** binds the request, fingerprint, operation identity, accepted scope and known result to the same durable transaction as the corresponding metadata transition. Receipt replay is independent of current UI selection. Since a database transaction cannot atomically commit arbitrary filesystem effects, a receipt never claims a complete filesystem result until the required observations and final manifest are durably recorded. Filesystem effects, restart recovery and lost replies use the intent/observation records, not blind re-execution.

### State transitions

- `reviewed` → `accepted` only for a current exact preview, eligibility decision and authority with source and destination preconditions checked at admission.
- `accepted` → `preparing` → `applying` as durable per-effect intents are recorded and effects proceed under their own boundary checks. Verification without effect may finish from `preparing`.
- A fully known result transitions to `completed`, `incomplete`, `rejected` or `failed`; completeness requires the applicable manifest closure and every selected required eligible item. Per-item outcomes remain visible even if the operation is incomplete or failed.
- Any possibly executed but unverified effect, lost persistence acknowledgement, identity ambiguity or incomplete recovery transitions to `uncertain` with an inspectable incident and a conflict fence at the narrowest independently proven safe scope; if that scope cannot be proven, the applicable global fence remains. Restart only observes retained records. No automatic replay or implicit clearance follows owner disappearance.
- A later authorized reconciliation may establish an outcome from independently verifiable retained evidence, append its observation and release only the proven-safe fence. It cannot manufacture a missing receipt, rewrite the old request or resume old execution. If proof is unavailable, uncertainty remains.

Cancellation before effects may settle as cancelled; cancellation or shutdown after an effect may have begun must retain the actual known, partial or uncertain outcome. The current owner lock, lifecycle and serialized store transaction protect authority transitions; active attempts, claims, source changes and other operations remain independent blockers where relevant.

## Export and backup manifest

**RetentionManifest** is a versioned, bounded inventory produced from the final verified operation state. It identifies format and version, product/schema compatibility, source scope and source logical identities, operation and preview IDs, declared categories, required and optional items, excluded/unavailable items and reasons, relationship closure, per-member normalized archive name and type, exact byte count and integrity digest for eligible included bytes, aggregate limits and final completeness. It explicitly states whether project filesystem material and each referenced artifact are included, omitted or unavailable.

Each normalized member name is unique and relative to the bundle namespace. Names are data identifiers, never executable extraction paths. Absolute paths, traversal, aliases, undeclared members, duplicate or colliding normalized names, links, special files and members exceeding configured bounds are rejected. The manifest is not proof of credential eligibility; verification of a manifest or payload occurs only after its source and member eligibility are established by the pending policy.

A `complete_backup` requires all selected required records, relationships and eligible required payloads; it is not valid if any required source is unavailable, unknown, changed or excluded. An `incomplete_backup` or `history_projection` explicitly records omissions and cannot be offered as a complete or executable restoration. Evidence export may similarly be complete only for its declared selected scope; unrelated project material is never included.

## Isolated import and restored history

**RestoreValidation** binds an immutable backup identity and manifest digest to a specific operation and absent destination, compatibility result, member/relationship validation, eligibility decisions and all observed conflicts. Validation checks limits and references before destination effects. The same archive, member identities, current destination and absence of collisions are checked again at the commit/effect boundary; a changed or unverifiable source or target is refused without overwriting existing work.

**ImportedHistory** records a new local import/namespace ID, source backup and source logical identities, the mapping to local records, selected relationships, available and unavailable fields, manifest completeness, source owner/epoch and known interrupted or unresolved states. Preserve original IDs and provenance as historical source identities; if a local namespace uses new IDs, retain an explicit collision-checked mapping rather than confusing those IDs with live authority. Existing local records, tasks and unrelated projects are not overwritten or silently merged.

Imported history is observational: no restored request receipt, pause, process identity, claim, work item or prior approval may authorize a new agent dispatch, integration, cleanup or effect. No provider is invoked and no old writer resumes. Uncertain or interrupted ownership remains visible and fenced where conflicting scope cannot be proved safe. A limited history projection stays explicitly incomplete and is not a recoverable complete backup.

## Cross-record invariants

1. Scope, identity, selected categories, authority, destination and per-item dispositions agree across preview, operation, receipts, manifest and imported history. Newer user drafts and selections are independent of late observations.
2. Eligibility is checked before content access and again as necessary at every read/effect boundary. Provenance, source membership, object identity, parent identity, sharing and dependency state are revalidated; filesystem namespace changes and hard-link/symlink swaps fail closed.
3. Ownership and claims are checked within the relevant durable transaction, while physical effects use bounded, handle-relative operations with immediate revalidation. Never rely on a preflight check alone for destination absence or source stability.
4. Persist a durable intent before an effect, record the actual outcome afterward, and sync the relevant file and namespace as supported. A crash between those points yields an inspectable uncertainty, not a second effect or a fabricated success.
5. Existing shared artifacts, retained incidents, audit records, user edits, unrelated tasks and legacy collaboration state remain untouched. A selection's logical references do not prove exclusive ownership of a physical file.
6. All inventories, manifest parsing, traversal, item counts, names, sizes and public pages have explicit finite limits. Over-limit input is rejected or visibly incomplete; it is never silently truncated into a complete result.
7. Preview, export, verification, restore, cleanup, observation, reconciliation and retry make zero model calls. No raw vendor credential or credential-derived digest is retained in any record.
