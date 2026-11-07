# Research: Scoped Retention and Recovery

**Status: research draft; payload eligibility decision unresolved.** The current
specification requires a useful complete backup of qualifying real production
project state (RET-004) and forbids inspecting, copying or hashing vendor
credentials (RET-003). The interpretation of eligibility for opaque production
payloads is awaiting the user's answer. This document does not resolve that
decision, relax the credential boundary, approve an implementation plan or claim
that a partial history or synthetic fixture satisfies RET-004.

## Existing evidence and limits

Council's lifetime-locked local owner uses private `<user-home>/.council/v2/`
state; the accepted runtime keeps metadata in SQLite and artifact bytes in
private files. `internal/localstate/state.go` acquires a native owner lock before
opening `workspace.sqlite3` and protects the database and recovery companions.
`internal/store/store.go` uses a serialized connection; writes in
`internal/store/execution.go` distinguish pre-commit failure from uncertain
commit acknowledgement. Raw database files, WAL or SHM are not a coherent scoped
export format and can contain records belonging to multiple projects.

Some useful facts are stored as dedicated structured columns: generated task and
attempt IDs and relationships, numeric revisions, attempt phase and graph
identity/state. Other records serialize arbitrary text into `data` JSON. Artifacts
include prompts, stdout, stderr, final responses, source files, index snapshots,
candidate deltas and destination evidence (`internal/store/artifacts.go`). The
source capture path (`internal/workspace/capture.go`) can retain arbitrary project
file bytes. These locations, closed status, hashes or file names do not establish
that opaque content is credential-free. Nothing in this research inspects actual
user state or vendor authentication.

## Alternatives for useful recovery

| Alternative | Practical value | Credential and completeness boundary | Disposition |
| --- | --- | --- | --- |
| Production metadata-only history | Existing projects can export selected generated IDs, relations, validated states, revisions and artifact references, then inspect them in an isolated read-only history. | Original instructions, free text, user files and artifact payloads are unavailable; mark the history incomplete and non-dispatchable. Do not fetch or decode entire opaque records merely to extract a field. | Useful additional profile; cannot satisfy RET-004 or SC-001 by itself. |
| Opaque payload selected by the programmer | Preserves the task text or evidence that developers actually want to recover, with a simple selection flow. | Existing arbitrary payloads may contain vendor credential material. A path allowlist, selected file, account ownership, user selection, client-supplied label, hash, signature or encryption alone does not prove absence; scanning credentials would itself violate the stated boundary. | **Unresolved:** use only under the user's clarified interpretation and a subsequently reviewed eligibility mechanism. Never describe selection alone as a proven credential-free source. |
| Provenance by construction | A producer can publish a fixed-schema record whose values and exact bytes are derived only from controlled credential-free constants, validated enums, Council-generated IDs or bounded counters, with server-owned provenance bound to the immutable value. This can operate on real installations, rather than merely fake fixtures. | An actual production source must supply every required selected byte and meaningful recoverable project state. The current runtime does not establish this for arbitrary user instructions, agent output, source files or other opaque payloads. Empty or artificial data alone is not evidence of a useful complete backup. | Candidate for narrowly scoped real production data; demonstrate an actual useful qualifying project and exact content coverage before claiming RET-004. |
| Raw SQLite or project-tree copy | Appears simple and captures a broad amount of state. | Shared SQLite data can include other projects and opaque text; copying the database alone misses live WAL and separately stored artifacts. Project trees can contain credentials, links or later user edits. | Reject as the scoped production backup mechanism. |

Council-generated IDs must actually be generated and bound by Council; caller-set
request identities, human-provided keys/labels, paths, notes and any user-controlled
text are not automatically trusted merely because they fit a primitive type. A
provenance record must itself be non-client-forgeable and bound to the exact
immutable content; it cannot retroactively certify unknown historical bytes. The
result should identify selected unavailable items without opening them, and must
not advertise an incomplete export as a complete backup.

An empty or metadata-only project may demonstrate mechanics, but the accepted
specification explicitly requires a **useful** complete real-production path. It
is not yet established whether provenance-by-construction provides enough real
selected content to meet that bar. That question remains open alongside the
user's pending payload-eligibility answer.

## Owner and snapshot considerations

- The existing owner and SQLite transaction boundary, rather than a second
  coordinator or UI preflight, must govern selection, durable operation identity,
  exact receipt replay and any destructive effect. A reused request ID with
  different scope or destination must conflict.
- Capture selected records and their referential closure from one consistent
  project-scoped database snapshot; do not export the shared database wholesale.
  Active/collecting artifacts and changing references require a coherent capture
  strategy or an explicit incomplete result. A SQLite transaction by itself does
  not make external artifact files an atomic snapshot.
- Under the chosen eligibility rule, bind each included file to exact logical
  owner, current physical identity, length and bytes actually copied; recheck
  source scope and required revisions before publishing. Unsupported, missing,
  changed or ineligible selected material must be reported, never silently
  replaced with current data or labelled complete.
- A portable manifest should declare format/version compatibility, source and
  selected scope, member identity, reference graph, exact lengths and integrity,
  omissions and completeness. A digest verifies copied bytes; it does not prove
  credential safety or authorize sharing. A local destination must be explicit,
  with no implicit upload, overwrite or publication.

## Verified restore and reviewed cleanup

Restore must validate format, compatibility, integrity, selected closure, sizes,
duplicates and destination conflicts before effects; reject path traversal,
absolute or drive-qualified names, unsafe links and alias collisions. Use an
explicit absent isolated destination, revalidate held source and destination
identities at the effect boundary and preserve the input backup. A restored
history must be accessible through Council, with original logical relationships
and explicit missing data, but must not import live owner/process authority,
restart agents, repeat effects or silently merge with current projects. Existing
physical artifact identities cannot be trusted on a new filesystem; establish
new identities for any actually restored eligible files.

Database and filesystem operations do not share a single atomic commit. Durable
intent, exact effect identity, per-effect observations, no-overwrite primitives
and a retained unresolved fence are needed when publication or restoration is
interrupted or acknowledgement is uncertain. Do not claim rollback unless its
result is verified. Platform-specific directory durability limitations must be
stated.

Cleanup should start from a task-owned reviewed inventory and require explicit
selection. Recheck identity, ownership, dependencies, contents and current
authority at the destructive boundary. Shared evidence, unrelated tasks and
subsequently modified or unverified material remain protected; a changed or
unaccepted object needs a fresh exact review and authority before any removal.
Use native anchored handles and verified quarantine rather than trusting an
earlier pathname check, retain audit separately and report known, failed and
uncertain outcomes. Credential-boundary implications of reading a candidate to
verify its contents are part of the same unresolved eligibility decision.

## Evidence needed before acceptance

1. A genuine non-fixture production eligibility mechanism and a useful real-use
   project selection that is complete under the user's clarified rule, with all
   selected required relationships and content restored through Council and zero
   automatically resumed writers. A synthetic fixture is useful for testing but
   is not proof of this path.
2. A separately labelled metadata-only existing-project round trip with a second
   unrelated project, explicit omitted opaque data and no model calls; this proves
   only the incomplete-history profile.
3. Controlled access tests using fake credential-path sentinels, proving no vendor
   authentication object is opened, hashed or copied; unknown required provenance
   must produce an observable failure or incomplete result before payload access.
4. Actual owner/filesystem cases for concurrent source drift, lost response,
   partial effects, destination conflicts, malformed or oversized archives,
   traversal/link/swap races, cleanup of two tasks and edits made after review.
   No live vendor calls are needed or authorized for these checks.

## Decision gate

The user's answer on how selected opaque production data may be eligible is
pending. Until it arrives, do not freeze a payload inclusion design, assume that
ordinary user data is credential-free, count an incomplete history as RET-004,
or start implementation of an eligibility-dependent feature. Independent work
on specification analysis and concrete alternatives can continue without that
decision; no elapsed time constitutes an answer.
