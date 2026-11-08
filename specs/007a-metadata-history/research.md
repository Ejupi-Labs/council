# Project Metadata History: Research and Decisions

**Status:** design research for feature 007a; proposed mechanisms require a
concrete contract, independent review and native verification before implementation.
This is incomplete observational history, not evidence export, backup, import,
restore, acceptance proof or a replacement for the remaining FR-042 work.

## Structured source and provenance

**Decision:** capture a finite, project-scoped projection of separately stored
Council-generated fields. Do not select `data`, `authority_data`, `dispatch_data`,
raw database files, opaque JSON, user text, paths, payloads or credentials; do
not use `SELECT *` or extract a field from an opaque value. Existing whole-record
helpers that decode `data` are unsuitable for this feature. Do not call agents
or inspect the project directory to capture or inspect history.

For the first increment, include project ID; task ID, project relationship,
revision, state revision and version; and attempt ID, task relationship, task
version, revision and phase, subject to the provenance checks below. Source
sequence may order records inside the selected project but is not exported;
assign contiguous snapshot-local ordinals independently to tasks and attempts.
`internal/store/execution_schema.go`, `internal/store/tasks.go` and the creation
paths establish the relevant columns and normal generators. The design may add
other fields only after separately tracing their producers, validating their
domain and project membership, and updating the contract and tests. In particular,
do not infer an attempt outcome from `settled`, current ownership from a recorded
phase, graph dependencies from ordinal, acceptance from a reference, or provider
usage from Council counters. Refer to the detailed candidate table and joins in
`../007-scoped-retention/contracts/metadata-history.md`; that research is not
an approved promise to include every candidate.

A 32-hex value is not proof of origin: caller-supplied IDs exist and databases
can be externally modified. Schema 7 has no cryptographic per-row generation
provenance. This proposal explicitly trusts the Council-managed database at its established private
v2 path under the owner lock, including records from prior genuine owner epochs. The
current schema has no origin attestation: it cannot detect a same-user copied, manually
modified or tampered database, and the private path and lock do not prove origin. This
is a stated local trust assumption, not a claim of detection; Council offers no foreign
database import or alternate source in this slice. Reject schema or relationship failures
and omit unproven fields without treating ID format alone as provenance. The
ordinary trusted-store case must still include real eligible task/attempt links.

Column projection prevents application access to excluded values; SQLite can
still read a physical page containing other columns. This design does not claim
zero physical page reads. If the rule is interpreted as forbidding those physical
reads as well, the shared database cannot establish the guarantee and this
approach requires a separate storage boundary.

## Snapshot and bounded storage

**Decision:** use a Store-owned typed projection in one SQLite read transaction
under the existing store mutex and single-connection discipline in
`internal/store/store.go`. Establish project existence and association in that
transaction. Parameterized joins, deterministic ordering, explicit bounds and
validation must include selected relationships exactly once, without emitting
unrelated IDs. Detect inconsistent or cross-project references rather than
silently filtering them into an apparently complete result. Close rows and end
the transaction before serializing or doing filesystem I/O; do not call public
Store methods which acquire the same mutex inside it.

Materialize a bounded typed snapshot in memory and persist only its generated,
validated representation, identity, capture metadata and explicit exclusions in
dedicated owner-managed tables. Save it atomically with its capture receipt;
store no whole original rows, destination paths, free-text error messages from
the source, or opaque blobs. Keep exact revisions and counters as canonical
decimal strings at JSON boundaries. Record explicit limits on rows, bytes and
references; overflow, failed reads or broken links produce a bounded error or
explicit incomplete capture, never silent truncation. An empty project is not a
failed capture. Once committed the snapshot is immutable; inspection and export
must use that identity rather than querying current source as if it were the
same historical snapshot.

Alternatives rejected: paging through independent transactions can mix moments
in time; raw SQLite backup includes excluded data; retaining a long transaction
while writing output holds the only connection and needlessly delays work.
Chunking may be added only with a genuinely retained, bounded read snapshot and
explicit lifetime. Content integrity hashes cover only the newly generated
metadata bytes; they prove neither source provenance nor semantic completeness.

## Destination and publication

**Decision:** begin with an explicit user-selected absent filename in a
Council-managed private history-output namespace in the private v2 directory, showing the full destination
and immutable snapshot ID in a preview. Restrict the initial destination to
that directory, outside legacy state and Council release inputs; verify the
private root and supported filesystem before publication. This bounded choice is
not arbitrary path support. Council never stages or commits output or includes it
in release artifacts; it does not claim that every user home is outside every Git
repository. The preview discloses the actual output location. Do not put the
destination path in the snapshot or output, though the private operation record
needs it to bind and report the requested effect.

Bind preview to the exact snapshot, destination name and parent identity, scope,
and revision. At publication revalidate authority, parent identity and absence;
reject unsafe components, links, reparse points, mount crossings, altered roots,
existing targets and unsupported filesystems. Resolve relative to a retained,
verified directory handle; a lexical prefix check, `os.Root` alone or a prior
`stat` does not prove safe traversal or atomic no-overwrite. Ordinary namespace changes and changing mounts are checked at the effect boundary;
hostile same-user tampering with private state is outside the stated local trust model.
If a required identity or containment guarantee cannot be enforced, fail
closed, not by following a new path. Never inspect an existing target's contents.

Create the user-selected single final filename directly, relative to a retained verified
private directory handle, with one platform-native exclusive-create and no-follow
operation after its durable permit. Write the generated bounded bytes through that
retained file handle only, sync as supported and recheck its identity and the final
name before recording success. A crash or failed write can leave a visible partial
file; retain it, report the operation incomplete or uncertain and never automatically
delete or overwrite it. The output and database are not one atomic transaction.

Rejected alternative: staging a temporary path and renaming it adds extra effect
boundaries and substitution questions, while an ordinary rename can overwrite an
existing object. An absence check followed by creation is not exclusive publication.
No path-based staging or rename is part of the selected design. A checksum covers
the generated payload and is not proof against substitution.

The explicit threat model trusts the Council-private root against hostile same-user
namespace tampering, as it already trusts the owner-managed local database. Ordinary
existing names, unexpected links and observed identity changes fail closed. We do not
claim resistance if an arbitrary same-user process deliberately rewrites the private
state or newly created file, and must not claim path identity proves such resistance.
The actual exclusive-create, retained-handle, recheck and sync behavior must still pass
native Windows/Linux/macOS tests; unsupported volume behavior remains unsupported.

`internal/workspace/mount_linux.go` addresses bind mounts that `os.Root` alone
does not constrain. Existing native workspace filesystem code provides useful
identity and validation research, not a proven reusable export implementation.
Windows reparse points, sharing modes, file identity and volume rules, Darwin
mount/ACL behavior, Linux mount IDs, and each filesystem's exclusive-create and
directory-sync behavior need native tests. The existing Windows
`applySyncDirectory` explicitly does not promise directory flush durability.
No universal crash/power-loss durability or support for network, removable or
unusual filesystems may be asserted; where the required evidence is unavailable,
retain uncertainty or report unsupported instead of success.

## Owner journal, retries and reconciliation

**Decision:** use durable Store-managed operation rows distinct from snapshots,
bound to the exact request ID, operation, snapshot, preview, destination and
authority. Reusing an ID with different input conflicts; an identical retry or
cross-interface lookup returns the original receipt or current unresolved state,
never starts a second write. `internal/store/schema.go` and existing receipt
logic are patterns, not a substitute for an effect journal.

Within the locked owner, commit `prepared` before any destination mutation;
durably and positively acknowledge the publication permit and `effect_started`
before creating the final file, and journal each subsequent effect-capable stage.
Create the final file at most once, then persist the observed identity, sync result
and outcome. If the commit or owner lifetime is uncertain, fence the
operation and conflicting destination effects; do not retry, overwrite, remove
or infer completion from a missing acknowledgement. Failure after the marker can
remain uncertain even if the target is absent. A crash between publication and
outcome commit is likewise uncertain; a matching filename or hash alone does not
prove that Council produced the current object.

A concrete positive **no-effect proof** requires the complete validated durable
journal to show no publication or stage permit ever committed; a `prepared` row
alone is insufficient. The owner must freeze the exact operation and prevent new
permits, drain all registered effect-capable actors and handles, establish current
owner/generation facts and, across owners, an unbroken proven clean epoch chain,
then atomically recheck these conditions and record the decision without clearing
independent incidents. All OS mutation paths require positively acknowledged
durable permits, and an ambiguous permit is fenced. This proves only that this
operation began no Council-authorized destination mutation; it does not prove the
target remains absent, so any newly authorized attempt requires a fresh preview
and boundary checks. A merely absent file, elapsed time, user assertion, or an
unresolved old owner is not proof. `internal/store/recovery_migration.go`,
`internal/store/store.go` and `internal/engine/recovery*.go` show existing owner,
receipt and recovery patterns; their authority must be explicitly adapted, not
assumed to cover this new effect.

## Integration and verification

- Add a feature-specific schema migration, typed domain model and bounded Store
  projections/receipts under `internal/store` and `internal/domain`; verify schema
  upgrades preserve existing rows without decoding their payloads.
- Add an owner-controlled history service under `internal/engine`, exposed via
  narrow interfaces in `internal/app`, strict local routes in `internal/httpapi`,
  and matching frozen-intent CLI commands in `internal/cli`. A separate native
  publication package should enforce the chosen filesystem boundary.
- Add branded accessible capture, inspection, preview and operation views under
  `ui/src`, preserving newer selection and input against stale responses. UI and
  CLI inspect the same immutable owner-managed snapshot; no arbitrary file intake
  or import exists in this slice. Build and commit the normal embedded UI assets.
- Exercise real owner and fake-agent flows for two populated projects; instrument
  SQL and file access to confirm the projection does not fetch excluded values or
  open artifact bytes. Verify limits, large integers, corrupt cross-project links,
  empty results and concurrent changes. Tests can establish only the operations
  and native platforms they actually observe.
- Inject loss or crash before and after the journal transition, publication and
  receipt commit; test exact replay, changed request conflicts, no-effect proof,
  target replacement and unsafe links, and absence of model calls. Check UI/CLI
  identities, keyboard use and 360px layout. Report Linux, macOS and Windows
  results individually; reject unsupported capabilities and do not imply live
  provider qualification.
