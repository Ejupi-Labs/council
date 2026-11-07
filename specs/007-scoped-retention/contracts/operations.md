# Retention operations contract

**Status: DRAFT — not approved for implementation.** This document proposes interface
shapes for `specs/007-scoped-retention/spec.md`. The production credential-safe
provenance mechanism, trustworthy handling of untrusted backup payloads, and actual
restored-project inspection path are unresolved design gates. The concrete storage,
archive format, lifecycle integration, field encoding and platform primitives require
research, independent review and any required user decision. Nothing below establishes
that a backup, restore or cleanup operation exists or is safe to ship.

## Common operation behavior

UI, CLI and HTTP address the same local owner, previews and durable operations. All
interface copy is English. Inspection and operation control make zero model calls.
Proposed HTTP paths below are relative to `/api/v1`. The final transport must enforce
existing local owner authentication, origin checks, input limits and strict decoding.
Opaque IDs, revisions and bounded numeric fields follow the accepted Council contract;
private paths, process tokens and native physical identities are not public proofs.

A mutation carries a client `requestId`. Its persisted identity and fingerprint bind
operation kind, exact source scope, selected records/items, preview identity and revision,
destination and any explicit per-item authority. The same ID with changed input is a
conflict. An exact retry returns the recorded receipt and known outcome or the current
unresolved operation; it never authorizes a second filesystem effect. A lost response,
owner restart or polling timeout is not evidence that an effect did or did not occur.
A user can inspect the original operation without refreshing its captured authority.

A preview has an identity and revision and is non-destructive. It identifies the selected
project or task, record and artifact categories, eligible items, required and unavailable
items, exclusions and reasons, local destination if applicable, completeness, relevant
snapshot revisions and any fresh authorization required. Execute names that exact
preview and revision. Stale, altered, inaccessible or ambiguous selection or destination
is rejected or explicitly incomplete; it is never silently substituted with a new
snapshot. Previewing itself does not authorize a later effect.

An operation exposes `operationId`, kind, scope, captured preview and request identities,
state, per-item status where applicable, durable observations and actionable next step.
Suggested states are `preparing`, `ready`, `applying`, `complete`, `incomplete`,
`failed` and `uncertain`; state names and transition details remain design work.
`complete` requires all selected required effects and evidence to be verified. For a
history-only projection, operation completion never changes the history's declared
`incomplete` status. An accepted response is not by itself completion.

Before each filesystem effect, the owner durably records exact intent and authority;
afterward it records observed outcome. Receipt replay, source/destination revalidation,
interrupted effects and recovery are coordinated at the actual durable boundaries.
When a write, publication, removal or acknowledgement has an unknown outcome, the
operation remains uncertain, relevant conflicts stay fenced and no automatic replay
occurs. Status, blockers and a supported owner-mediated reconciliation action record
its own request, available observations and decision. An unsupported or insufficient
proof leaves the incident unresolved and fence intact. A supported positive case must
prove that the durable effect permit was never issued, all eligible controller operations
are drained, and the relevant owner epoch is clean; it may record no effect and release
only that operation’s own fence. It must not infer this from elapsed time or missing files. Neither a user assertion, a
reused PID, an unverified path nor mere elapsed time proves an effect safe.

## Payload eligibility and privacy gate

Council must not inspect, copy, hash or scan vendor credentials, including when a
selected item, referenced object or supplied archive points to them. Eligibility must
be established **before reading the selected bytes**, by provenance for the exact value
or payload and its source, not by a client flag, arbitrary label, allowed filename,
selection, project location or a scan of the content. Credential-bearing or unknown
provenance is excluded or reported unavailable without opening the object; if a required
member cannot be established safe, the result is incomplete or rejected, never complete.
This boundary applies to preview, cleanup, restore and reconciliation as well as export
and backup. Current fixtures must use synthetic values and never real credentials.

The source of trustworthy provenance for ordinary production project state is **not yet
established**. A fixture-generated proof or metadata-only history is not a complete
production backup path. Likewise, simply opening or hashing an untrusted user-supplied
backup can inspect credential bytes; the design must first demonstrate a credible,
fail-closed trust and provenance boundary for the archive and its exact members. Do not
assume that a manifest authored by the archive, a digest, encryption, a file extension
or a user's assertion supplies this proof. If no compliant boundary is established,
reject or leave unavailable; RET-004 remains unmet. Research and the relevant user
answer must resolve these gates before an implementation or acceptance claim.

## Proposed interface surface

The following is a draft, not a frozen API. `scope` discriminates an existing task or
project by exact ID. A destination is an explicitly chosen local target, not permission
to publish externally. Preview and execute requests use independent `requestId` values;
execute sends `previewId` and `previewRevision` and any required exact selection and
authority. Changes after preview require a new preview, not an automatically refreshed
execute request. Wire details, path syntax and CLI flags remain subject to design.

| Intent | Proposed HTTP | Proposed CLI |
| --- | --- | --- |
| Preview evidence export | `POST /retention/exports/preview` | `retention export-preview` |
| Execute captured export | `POST /retention/exports` | `retention export` |
| Preview project backup | `POST /retention/backups/preview` | `retention backup-preview` |
| Execute captured backup | `POST /retention/backups` | `retention backup` |
| Preview backup verification and restore | `POST /retention/restores/preview` | `retention restore-preview` |
| Restore captured preview | `POST /retention/restores` | `retention restore` |
| Review task cleanup | `POST /retention/cleanups/preview` | `retention cleanup-preview` |
| Dispose exact reviewed items | `POST /retention/cleanups` | `retention cleanup` |
| Inspect operation and blockers | `GET /retention/operations/{operationId}` | `retention inspect` |
| Request reconciliation of an operation | `POST /retention/operations/{operationId}/reconciliations` | `retention reconcile` |

Export and backup previews bind `requestId`, exact scope and local destination. Execute
binds `requestId`, `previewId` and `previewRevision`. Restore preview binds exact backup
source and absent isolated destination; execution binds its preview and revision. Cleanup
preview binds the task; execution binds the exact preview, selected item IDs and any
fresh per-item disposal authorization. Reconciliation binds the exact operation and
current revision. Controller-generated observations and native handle proofs never arrive
as user-supplied authority. A concrete schema and response/status mapping must be
specified and reviewed before implementation.

The UI must offer the same exact scope, selection, destination, completeness, operation
ID, replay, incident and outcome information as the CLI. Keep newer drafts and selections
when old responses arrive. Controls use the approved Ejupi Labs identity, accessible
names and text states, visible keyboard focus and usable narrow-screen layout. Both
interfaces must show omitted material and distinguish a verified complete backup from an
incomplete history projection. A read or wait must not cause a model call or resume work.

## Export, backup and incomplete history

A complete export or project backup includes every selected eligible required record,
relationship and payload, with no unselected unrelated-project data. The manifest lists
version, source scope and identity, supported compatibility, member identities, sizes,
integrity, exclusions, unavailable references and completeness; it explicitly marks
whether selected project filesystem material and referenced artifact payloads are
included, omitted or unavailable. Never label missing, changing, incomplete or ineligible
required material as complete. A digest proves bytes match a captured value, not that
those bytes were safe or credential-free.

Capture uses a consistent scoped record selection and complete referential closure;
filesystem content and database state are not presumed atomic. Freeze the exact selected
identities/revisions, verify eligible closed material through controller-held identity
checks and revalidate source, selection and destination before publishing. Publication
must not overwrite an existing target. If identity, durability or publication is
ambiguous, preserve observed outcomes and fence relevant conflicting work rather than
claim a complete artifact or repeat an effect.

A constrained metadata-only history is required as an explicitly incomplete profile as a separately identified,
read-only, explicitly incomplete operation with its own provenance rules. Its imported
records are historical observations, cannot authorize execution, delivery or cleanup,
and must not be accepted by the complete backup restore path. This is supplemental;
it does not meet RET-004's useful complete production-backup gate.

## Verified restore

Restore is allowed only for a complete, eligible, compatible backup after validating
source identity, required closure, member integrity and limits, and after rejecting
unsafe paths, links, duplicate names or IDs, unrecognized members and destination
conflicts. Verification must honor the eligibility gate before touching payload bytes.
Preview performs no restore effect. At commit the owner revalidates the exact held source,
destination and captured authority; changes or pre-existing targets reject before an
effect. Restore uses an explicitly selected absent isolated destination, never overwrites
or silently merges live project state, and does not transfer old owners or authority.

An interrupted restore journals actual known and uncertain effects, remains visibly
incomplete and fences conflicting work until auditable reconciliation proves an action
safe. It does not silently retry filesystem writes, resume agents, publish externally or
promote uncertain ownership to settled. A real current-owner inspection path must expose
retained source identities, relationships and unavailable information without granting
execution, integration or cleanup authority. Its precise destination, ownership and
integration with existing recovery fences remain unresolved design gates.

## Reviewed cleanup

Preview does not delete. It reports exact task-owned candidates, state, dependencies,
exclusions and fresh authorization required. The user selects a subset of the reviewed
items; changed or unaccepted work requires a new review and explicit authority covering
its exact current identity and state. General full access, an old preview or a broad
selection does not grant disposal authority.

Shared, still-required, unrelated, foreign, inaccessible, credential-bearing or
unverifiable objects are hard exclusions; fresh authority cannot override those checks.
Before every removal, verify task ownership, current references, exact reviewed state,
identity, link/type and the latest explicit authority without inspecting vendor
credentials. Do not recursively remove unreviewed contents or follow links. The design
must establish race-resistant controller-held object identity and a safe quarantine or
other equivalent platform-specific procedure; a pathname checked before deletion is not
sufficient. Changed or unverified items stay retained, and per-item results distinguish
removed, retained, failed and uncertain. Reviews, authority, existing accepted records
and audit observations remain available after removal.

## Qualification gates

Acceptance must cover an actual qualifying production-use project and unrelated project
for complete backup and restore, not merely synthetic fixture provenance. Tests use fake
agents and controlled synthetic credential paths without opening real credentials. Cover
two-project export, two-task cleanup, drift and deletion races, corruption and unsafe
archives, pre-effect rejection, interrupted/ambiguous effects, lost acknowledgements,
explicit reconciliation, zero model calls/replay and actual UI/CLI parity. The plan must
provide a concrete credential-safe way to qualify and exercise production data, and a
real owner inspection path; until then these are blocking design and delivery gates.
