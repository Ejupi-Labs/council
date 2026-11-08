# Feature Specification: Project Metadata History

**Feature context**: `specs/007a-metadata-history/`
**Git branch**: `refactor`
**Created**: 2026-11-08
**Status**: Reviewed for implementation
**Input**: Give programmers a useful, project-scoped view and local export of Council-generated structured history while excluding opaque content and preserving unrelated work.

## Scope and meaning

This slice provides a local, observational snapshot of a selected project's eligible
Council-generated structured metadata, its inspection in Council, and an explicit local
export. It is always labelled **incomplete history**: it is not an evidence export,
project backup, verified restore, executable recovery or proof of task acceptance. It
cannot authorize a model call, task, integration, publication, cleanup or any other effect.

Arbitrary archive intake or import, free text, source or destination paths inside the
snapshot/export, user files, artifact contents and vendor credentials are outside this
slice. Inspection means reading Council's own captured metadata snapshot through its
local UI or CLI; it does not mean opening an arbitrary exported file or archive. The
user selects an absent export filename within Council’s verified private export directory;
the resulting local destination is displayed, but is not included as history. Arbitrary
external destination paths are not supported in this slice.

Complete scoped evidence export, useful production backup and verified restore, and
reviewed artifact cleanup remain mandatory consecutive follow-up work before feature 008
and the first release. This slice does not satisfy or close FR-042.

## User Scenarios & Testing

### User Story 1 — Inspect a project's incomplete history (Priority: P1)

A programmer wants a compact, private inventory of their selected project's Council work
without reading prompts, source files or model outputs. They create and inspect a snapshot
showing the task and attempt relationships and recorded states that can be established
from Council-generated fields.

**Why this priority**: A scoped record of activity is useful even when original content
cannot be safely exported.

**Independent test**: Create ordinary controlled work in two projects, capture the first
project, and inspect the same snapshot through the local UI and CLI. Confirm its eligible
relationships and historical states, explicit omissions, and zero records from the second
project; verify no model call or source payload read occurs during the journey.

**Acceptance Scenarios**:

1. **Given** an existing selected project with eligible generated task and attempt metadata,
   **When** the programmer captures a snapshot, **Then** Council shows its identity,
   selected project, capture time, included categories, omissions and incomplete status.
2. **Given** multiple tasks and attempts and a second unrelated project, **When** the
   snapshot is inspected, **Then** all eligible selected relationships are retained once,
   no second-project record appears, and observed historical state is not presented as
   current process ownership or proof of acceptance.
3. **Given** a field or relationship cannot be proven Council-generated and safe before
   access, **When** the snapshot is captured, **Then** it is omitted with an actionable
   reason and cannot silently become trusted because its shape resembles an identifier.
4. **Given** the source changes during capture or exceeds a supported bound, **When** a
   coherent complete selected metadata snapshot cannot be established, **Then** Council
   reports a conflict or incomplete capture rather than silently mixing points in time or
   truncating the inventory as though complete.

### User Story 2 — Export an exact captured snapshot (Priority: P1)

A programmer chooses an unused filename in Council’s private export directory and exports a previously captured snapshot to
retain or share its explicitly limited inventory. The result does not contain task content
or pretend to be a backup.

**Why this priority**: A clear portable inventory provides a useful record outside the
application without granting restored execution authority.

**Independent test**: Preview and export the selected project's captured snapshot to an
absent local destination. Inspect the controlled output in the test and verify its
identity, eligible fields, omissions, integrity and second-project isolation. A retry of
an acknowledgement lost after publication must identify the same single output.

**Acceptance Scenarios**:

1. **Given** a captured snapshot and an explicitly selected absent local destination,
   **When** the programmer previews export, **Then** Council shows exact snapshot identity,
   included and excluded field categories, incomplete-history warning and destination
   before writing the output.
2. **Given** a valid unchanged preview, **When** export completes, **Then** the output
   contains exactly the captured eligible fields once, declares its snapshot identity,
   scope, version, integrity and incomplete status, and excludes unrelated project data.
3. **Given** an existing destination, changed target identity, unsafe path or link, stale
   snapshot authority, **When** export is requested, **Then** Council refuses or reports an
   explicit conflict without overwriting unrelated data or silently changing the selection.
4. **Given** an acknowledgement is lost or the owner stops at an output boundary, **When**
   the exact request is retried, **Then** Council returns its known result or an actionable
   unresolved outcome without publishing or overwriting another file.

### User Story 3 — Continue safely across interfaces and interruption (Priority: P2)

A programmer switches between UI and CLI or restarts Council while an export is in
progress. They can identify what happened without duplicate publication or loss of newer
selection and draft input.

**Why this priority**: Retained history must be trustworthy in normal interrupted workflows.

**Independent test**: Begin one controlled export from one interface, interrupt its
acknowledgement, inspect and retry through the other, and restart around the durable effect
boundary. Verify one output or explicit uncertainty, no model calls and unchanged unrelated
files; exercise keyboard use on the actual narrow UI.

**Acceptance Scenarios**:

1. **Given** an identical request is retried through either interface, **When** its receipt
   exists, **Then** the original operation identity and known result remain attributable
   to the same request without a second publication.
2. **Given** a request ID is reused with changed scope, snapshot, selection or destination,
   **When** submitted, **Then** Council rejects the conflict without an effect.
3. **Given** an output may have been published but its outcome cannot be established,
   **When** Council restarts, **Then** the operation remains inspectable as uncertain,
   conflicting actions remain fenced and no automatic retry or overwrite occurs.
4. **Given** a delayed response arrives after a newer selection or draft, **When** it is
   displayed, **Then** the newer input remains unchanged. Both interfaces show matching
   snapshot and operation identities, omissions and outcomes.

### Edge Cases

- A task, attempt or project relationship changes during capture or between preview and
  publication; a requested project is inaccessible or not owned by the current authority.
- A syntactically valid value originated with a user, external provider or unknown source;
  a whole opaque record would have to be read to extract a seemingly safe field.
- A destination already exists, is swapped, lies behind a link, points outside the chosen
  local scope, or its publication or acknowledgement fails after an effect begins.
- No eligible task metadata exists, the inventory is oversized, a reference is missing,
  or an owner is lost while an operation or effect is still in flight.
- A user supplies an arbitrary export for import or asks to use this inventory as a live
  recovery source; neither operation is supported by this feature.

## Requirements

### Functional Requirements

- **MH-001**: Council MUST let the programmer select an existing project, capture an
  attributable metadata snapshot and inspect it locally through UI and CLI without
  starting an agent, model call, import or execution.
- **MH-002**: The snapshot MUST be identified as incomplete observational history in
  every UI, CLI and exported representation, and MUST explicitly state its omissions and
  that it is not a complete backup, evidence export, restoration or acceptance proof.
- **MH-003**: Eligible fields MUST be an explicit finite set of separately stored and
  validated Council-generated IDs and relationships, canonical revisions and counters,
  validated state/phase enums and, where independently established as Council-generated,
  capture timestamps and safe record counts. The design MUST identify the source and
  provenance of each included field before reading it; string shape alone is not proof.
- **MH-004**: Council MUST NOT read, decode, scan, copy, hash or include arbitrary task or
  user text, project source paths, labels, project files, prompts, agent input/output, artifact payloads,
  environment or executable arguments, vendor authentication or credentials, legacy
  collaboration state or unrelated project data to create or inspect
  this snapshot or export. The existing local owner transport may transiently read its
  own authentication token for requests; it MUST never include that token in snapshot,
  export, operation diagnostics or provenance. In particular, it MUST NOT fetch an opaque whole record merely
  to extract one purportedly safe field. This is a logical application-access boundary:
  SQLite may load a page that also contains excluded columns, which is not represented as
  proof of zero physical disk-page reads. No vendor authentication file is opened or read.
  User-chosen export destinations may be displayed
  and validated solely to perform the requested local publication, but MUST NOT be embedded
  in the snapshot or exported history.
- **MH-005**: Council MUST establish the selected project's source ownership and a coherent
  capture of eligible relationships and revisions. Missing, unsafe, unknown or unverified
  fields MUST be omitted and explained; incoherent relationships, changed source or
  unsupported limits MUST fail or be reported as incomplete, never silently truncated.
- **MH-006**: Council MUST distinguish an empty eligible inventory from a failed or
  unsupported capture; existing populated projects with genuinely eligible generated task
  and attempt relationships MUST provide those relationships without requiring the user to
  populate a special fixture or supply a provenance assertion.
- **MH-007**: Before export Council MUST present the exact immutable snapshot identity,
  included/excluded categories, scope, explicit local destination in the verified private export directory, incomplete-history
  warning and captured authority. An export request MUST bind that preview and revision;
  changed scope or destination requires a new preview.
- **MH-008**: Export MUST publish only the selected captured snapshot, with bounded,
  versioned machine-readable structure and integrity information, and MUST NOT overwrite
  an existing destination, follow unsafe links, or alter unrelated files or projects.
  User-selected history output is private project data and MUST NOT be silently included in
  Git history, release artifacts or an externally published destination.
- **MH-009**: Export MUST revalidate the original captured authority and destination at
  the physical effect boundary. If file identity, publication, durability or concurrent
  changes are uncertain, Council MUST preserve the known outcome and prevent conflicting
  automatic publication until explicitly resolved; partial output is never labelled success.
- **MH-010**: Each mutation MUST carry a stable request identity bound to exact scope,
  snapshot, preview, selection and destination. An identical retry MUST return the same
  recorded receipt and known outcome or current unresolved operation without repeating an
  effect; reuse with changed input MUST be rejected without a side effect.
- **MH-011**: Council MUST durably record publication intent before the effect and its
  observed outcome afterward. An owner crash, uncertain commit or lost acknowledgement
  MUST retain an inspectable operation and MUST NOT automatically recreate, overwrite or
  delete an output. An owner-mediated reconciliation action MUST record its request,
  controller-established observations and decision; insufficient proof remains unresolved
  and conflicting effects remain fenced. The design MUST establish at least one concrete
  positive no-effect proof without relying on user assertions or absent files alone.
- **MH-012**: Snapshot inspection, export previews, retries and operation status MUST be
  observational: historical identifiers and states MUST NOT grant present ownership,
  dispatch, integration, external publication, cleanup or other live authority.
- **MH-013**: UI and CLI MUST expose matching project and snapshot identity, omissions,
  incomplete status, request/operation outcome and uncertainty. The branded UI MUST
  preserve newer drafts/selections across stale responses and support named keyboard
  controls, visible focus and text status at desktop and narrow widths.
- **MH-014**: Council MUST preserve all original project records and user edits; it MUST
  keep the metadata output and local operation history separate from unrelated project and
  legacy state, vendor credentials and release inputs.
- **MH-015**: Verification MUST exercise two populated projects, genuinely eligible
  generated relationships, controlled fake agents, excluded-content access sentinels,
  source/destination drift, unsafe links, existing destinations, lost acknowledgement,
  interruption before and after publication, concurrent requests and zero model calls.
  Native platform coverage and any untested capability MUST be reported separately.

### Key Entities

- **Metadata snapshot**: Immutable identity, selected project, eligible structured fields,
  retained relationships, capture boundary, exclusions and incomplete-history status.
- **Export preview**: Exact snapshot/revision, selected local destination, declared contents,
  omissions and preconditions, prior to any output effect.
- **History export operation**: Stable request and operation identities, captured preview,
  intent, observed output or uncertainty, receipt and actionable next step.

## Success Criteria

- **SC-001**: In a two-project controlled scenario, 100% of the first project's eligible
  generated task/attempt identities and relationships in the captured scope appear once;
  zero second-project records, excluded application-level content accesses or vendor
  authentication file reads occur.
- **SC-002**: A successful exported snapshot and the UI/CLI inspection expose the same
  snapshot identity, eligible metadata and explicit incomplete-history label; no model
  call or automatic dispatch occurs.
- **SC-003**: Existing targets, unsafe links, stale selections and changed destinations
  are rejected before overwrite in all defined test cases; unrelated files remain intact.
- **SC-004**: In controlled lost-response and crash cases, each durable publication effect
  occurs at most once for the exact request; uncertain outcomes remain visible and fenced.
- **SC-005**: The primary capture, inspection and export controls are keyboard-operable
  with visible focus and readable text labels at desktop and 360-pixel viewport widths.
- **SC-006**: The feature has an independent review, resolved blocking findings and actual
  offline platform results; it makes no live provider, complete-backup or release claim.

## Assumptions and Dependencies

- Project data is private; structured identifiers and relationships are not anonymized.
  Export is local and explicit and does not imply permission to publish externally.
- The source is the Council-managed database at its established private v2 location under the
  native owner lock. This is an explicit trusted-local-state assumption: the current schema has
  no origin attestation and cannot detect a database copied or tampered with by the same user.
  Council provides no import or foreign-database source in this slice. Only fields whose generation and provenance can be established without reading excluded
  content qualify. Legacy or unknown-origin data can be omitted with a reason; the selected
  ordinary production scenario must still yield useful generated relationships.
- The physical output procedure and positive no-effect reconciliation proof must be defined
  and independently reviewed in the feature plan before implementation. No user assertion
  or elapsed time substitutes for proof.
- This slice has no arbitrary archive intake or import. Full evidence export, backup,
  verified restore and reviewed cleanup remain required, separately specified work.
