# Metadata history operation contract

**Status: design draft for 007a.** This contract specifies the proposed bounded
owner, HTTP and CLI surface for implementation and review. It does not claim the
operations exist or that the required native filesystem proof is implemented.
The source is the known Council-managed store held by its native owner lock; this
slice does not authenticate arbitrary imported stores or detect same-user external
database tampering. It never reads vendor authentication files or extracts values
from opaque records. This profile is **incomplete observational history**, not an
evidence export, backup, restore, acceptance proof or executable authority.

## Conventions and limits

All routes are relative to `/api/v1` and use existing local-owner authentication,
origin checks and the Council JSON envelope: `apiVersion`, `requestId`, `data`,
`receipt` or `error`. Mutating requests have one explicit `requestId`; the CLI may
generate one when omitted and must display/retain it. Receipts have `requestId`,
`operation` and `committedAt`. IDs are exactly 32 lowercase hexadecimal characters;
revisions, counts, ordinals and byte lengths are canonical decimal strings within
the positive signed 64-bit range, except counts may be `"0"`. Timestamps are
Council-generated UTC strings in the established millisecond format. Unknown keys,
duplicate keys, invalid UTF-8, invalid IDs, noncanonical numbers, unexpected GET
bodies and unsupported query parameters are rejected before an effect.

A v1 snapshot contains at most 2,048 tasks and 8,192 attempts, and its complete
encoded payload is at most 4 MiB. A request body is at most 16 KiB; a page returns
1–100 entries, default 50; opaque cursors are at most 512 bytes. Bounds apply before
allocation and publication. Exceeding a bound is a `history_too_large` conflict
with no snapshot receipt or output; no list is silently truncated or reported as
complete. These concrete limits require verification against the existing envelope
and native memory controls before design approval; increasing them is a contract
change, not an implicit fallback.

Only dedicated, verified Council-generated source columns qualify: project ID;
task ID, project relationship, revision, state revision and current version; and
attempt ID, task relationship, task version, revision and validated phase. An
attempt's source sequence may order records but is not exported as a global count;
the snapshot assigns per-section canonical local ordinals. Do not infer an outcome,
acceptance, provider usage, current ownership or relationship not contained in these
fields. Unknown source lineage, invalid enum or cross-project membership fails closed
or is explicitly omitted with a fixed reason. Never fetch or decode a `data`/JSON
column to fill a field. SQLite may read a physical page containing excluded bytes;
the claim is no application-level access to excluded values, not zero page reads.

All snapshot bodies use these fixed omissions, in this order:
`task_text`, `project_paths`, `user_files`, `artifact_payloads`, `provider_output`,
`vendor_authentication`, `owner_authority`, `legacy_state`. Additional eligible
omissions use fixed reason codes `unknown_origin`, `unavailable`, or `invalid_link`
with a count; they never echo excluded values. User-supplied paths, free text,
request IDs, filenames, physical identities and error strings never appear in
snapshot or export payloads.

## Records and response schemas

The following names are normative for the draft; all fields are required unless
explicitly marked optional. Any `data` below is wrapped in the normal envelope.

**SnapshotSummary**:

```json
{"snapshotId":"0123456789abcdef0123456789abcdef","revision":"1",
 "projectId":"abcdef0123456789abcdef0123456789","profile":"metadata-history/1",
 "completeness":"incomplete_history",
 "notice":"Incomplete observational history. Not a backup, evidence export, restore, or proof of acceptance.",
 "capturedAt":"2026-11-08T00:00:00.000Z",
 "taskCount":"2","attemptCount":"3",
 "includedCategories":["project_identity","task_metadata","attempt_metadata"],
 "omittedCategories":["task_text","project_paths","user_files","artifact_payloads","provider_output","vendor_authentication","owner_authority","legacy_state"],
 "additionalOmissions":[]}
```

Each `additionalOmissions` item is exactly `{"reason":<fixed reason>,"count":<decimal>}`;
items are unique and ordered by the published reason order. `taskCount` and
`attemptCount` describe emitted records, not total hidden data. An empty valid
snapshot is `"0"`/`"0"`, not an error or a claim that excluded data does not exist.
`notice` is the exact fixed string shown above, not user-provided or derived from
excluded data. It appears in every snapshot summary, including the exported one.

**TaskRow** is exactly `{"ordinal":<decimal>,"taskId":<id>,"projectId":<id>,
"revision":<decimal>,"stateRevision":<decimal>,"currentVersion":<decimal>}`.
**AttemptRow** is exactly `{"ordinal":<decimal>,"attemptId":<id>,"taskId":<id>,
"taskVersion":<decimal>,"revision":<decimal>,"phase":<enum>}` where `phase` is
`queued`, `preparing`, `running`, `cancelling` or `settled`. An ordinal is unique,
contiguous starting at `"1"` in its own section; each attempt references exactly one
emitted task from the selected project. A phase is a capture-time observation,
not proof of completion or current authority. No row may be added by replay.

**SnapshotPage** is exactly `{"snapshotId":<id>,"revision":"1",
"completeness":"incomplete_history",
"notice":"Incomplete observational history. Not a backup, evidence export, restore, or proof of acceptance.",
"kind":"tasks"|"attempts","items":[<TaskRow or AttemptRow>],
"nextCursor":<string|null>}`. Each independently inspected page carries the same
fixed notice and completeness label as its snapshot. A cursor
is bound to the immutable snapshot ID, revision, section and position, cannot select
another project, and is rejected when malformed, altered or inconsistent. The owner
validates cursor bounds before a query and returns all pages in stable snapshot order.
The cursor is not authority for mutation and does not reveal raw database sequences.

**ExportPreview** is exactly `{"previewId":<id>,"revision":"1","snapshotId":<id>,
"snapshotRevision":"1","filename":<safe filename>,"destinationKind":"private_history_exports",
"destinationDirectory":<absolute private-root path>,"destinationPath":<absolute selected output path>,
"completeness":"incomplete_history","includedCategories":[...],"omittedCategories":[...],
"additionalOmissions":[...],"targetState":"absent","createdAt":<timestamp>}`.
The private-root directory and resulting full output path are disclosed in the preview,
UI and CLI so the programmer can locate the output. They and the filename are private
operation data, absent from snapshot pages and exported history. `targetState` is a preview observation, not a reservation; the parent and target are
rechecked with held native authority at publication. Controller-only parent identity
and scope proof are never received from the client or included in public JSON.

**ExportOperation** is exactly `{"operationId":<id>,"requestId":<id>,"revision":<decimal>,
"snapshotId":<id>,"previewId":<id>,"state":<enum>,"completeness":"incomplete_history",
"outcome":<null|"published"|"no_publication"|"failed"|"uncertain">,
"durability":<null|{"fileFlush":"succeeded","directorySync":"succeeded"|"unavailable",
"powerLossNamespaceGuarantee":false}>,"blockers":[<fixed code>],"nextAction":<fixed code>,"filename":<safe filename>,
"destinationDirectory":<absolute private-root path>,"destinationPath":<absolute output path>}`.
Operation blocker codes are exactly `owner_epoch_uncertain`, `journal_unproven`,
`actor_not_drained`, `history_effect_uncertain`, `other_blocker`, `target_conflict`,
`root_unavailable` and `request_interrupted`; each occurs at most
once. `nextAction` is exactly `none`, `wait`, `inspect`, `reconcile` or `contact_support`.
The controller never places arbitrary diagnostic text or exception details in these codes.

The typed durability observation is exposed identically by UI and CLI; `unavailable`
does not imply a failed file flush or a power-loss namespace guarantee. `requestId` is
the original export request identity; the disclosed destination fields
are private operation information, not history payload fields. States are `prepared`,
`effect_started`, `published`, `failed`, `uncertain` and `no_publication`. The
operation response contains no private parent identity, secret or source input record. `effect_started` alone
never establishes publication; `published` requires verified exact generated output,
no-overwrite publication, a successful file-content flush and an explicit directory-sync
observation. A state transition must increment revision. On Windows, a documented
`directory_sync_unavailable` observation can accompany `published` when all other checks
succeed; it does not claim namespace survival across a power loss. An attempted sync
that fails unexpectedly is uncertain, not successful.

**ReconciliationResult** is exactly `{"operation":<ExportOperation>,
"decision":"no_publication"|"unresolved","observationCodes":[<fixed code>]}`.
At minimum fixed codes distinguish `no_permit_proven`, `permit_present_or_uncertain`,
`actor_not_drained`, `owner_history_unproven`, `journal_unproven` and
`other_blocker`. They do not expose paths, native IDs or arbitrary exception text.

## Operations and CLI

| Operation | HTTP | Strict request / query | CLI |
| --- | --- | --- | --- |
| Capture a snapshot | `POST /history/snapshots` | `{"requestId":<id>,"projectId":<id>}` | `council history capture --project ID [--request-id ID]` |
| Inspect a snapshot | `GET /history/snapshots/{snapshotId}` | no body or query | `council history show ID` |
| Inspect snapshot rows | `GET /history/snapshots/{snapshotId}/records` | `kind=tasks|attempts`, optional `limit`, `cursor` | `council history records ID --kind tasks|attempts [--limit N] [--cursor C]` |
| Preview one export | `POST /history/exports/preview` | `{"requestId":<id>,"snapshotId":<id>,"snapshotRevision":"1","filename":<safe filename>}` | `council history export-preview --snapshot ID --revision 1 --filename NAME [--request-id ID]` |
| Execute the preview | `POST /history/exports` | `{"requestId":<id>,"previewId":<id>,"previewRevision":"1"}` | `council history export --preview ID --revision 1 [--request-id ID]` |
| Inspect operation | `GET /history/operations/{operationId}` | no body or query | `council history operation ID` |
| Request owner reconciliation | `POST /history/operations/{operationId}/reconciliations` | `{"requestId":<id>,"expectedRevision":<decimal>,"action":"verify_no_publication"}` | `council history reconcile ID --revision R [--request-id ID]` |

Capture returns `201` with `{"snapshot":<SnapshotSummary>}` and a durable
`history.snapshots.create` receipt. Show returns `200` with the same summary;
records returns `200` and `SnapshotPage`. Preview returns `201` with
`{"preview":<ExportPreview>}` and `history.exports.preview` receipt; preview is a
persisted review only and creates no output file. Execute may return `202` with
`{"operation":<ExportOperation>}` and `history.exports.create` receipt while
work is pending; `200` or `201` may be used only for the same recorded operation
when the committed operation state is known. Inspection returns `200` with the
operation, including current blockers and its original receipt identity.
Reconcile returns `200` with `ReconciliationResult` and a
`history.operations.reconcile` receipt, even if `decision` is `unresolved`;
it never implies publication or a newly authorized effect.

The CLI `--json` output has the same envelope, IDs, omission labels and outcomes as
HTTP and the UI. A CLI retry retains the exact original request, preview and filename,
not a freshly generated preview. The UI retains newer project selection/drafts on a
late response and exposes named keyboard controls and textual status at 360 px and
desktop sizes. All three interfaces use one owner and issue zero model calls.
A known older owner that lacks `metadata_history` and `historyContract: "007a-v1"`
must be refused for history operations; its separately recognized Stop contract
must remain available. Exact capability advertisement is part of the owner/CLI
compatibility checks, not a claim about unsupported harnesses.

## Output bytes and filesystem boundary

The v1 output is one UTF-8 JSON document with a deterministic canonical serializer:
`{"format":"council.metadata-history/1","snapshot":<SnapshotSummary>,
"tasks":[<TaskRow>...],"attempts":[<AttemptRow>...],"integrity":<Integrity>}`.
`Integrity` is exactly `{"algorithm":"sha256","encoding":"hex-lower",
"snapshotSha256":<64 lowercase hex>}`. `snapshotSha256` hashes the UTF-8 canonical
encoding of the exact object `{"snapshot":...,"tasks":...,"attempts":...}`;
the outer `integrity` member is not included in its own digest. Canonical encoding
orders object keys lexicographically by Unicode code point, uses no whitespace,
emits non-ASCII valid Unicode as UTF-8, escapes only JSON-required control characters,
uses the string spellings above, preserves array order and ends with no newline.
The owner hashes only this newly generated allowlisted payload. A matching digest
proves integrity of those bytes, not source eligibility or permission to share.
No request ID, selected filename, original path, output path, private controller
identity or excluded source field occurs in the document.

`filename` is a single ASCII basename of 1–64 bytes matching
`history-[A-Za-z0-9][A-Za-z0-9_-]{0,50}\.json`; `.json` is lowercase. Reject separators,
colon, drive/device syntax, leading/trailing spaces, dot segments, NUL, alternate
streams and Windows reserved device names case-insensitively. On filesystems with
case or Unicode aliasing, reject an equivalent existing name. The client cannot
select a directory or absolute path. The owner uses its already-held Council-private v2 state root,
validates each anchor, directory type/identity, owner, mount/reparse/link constraints
and containment according to native platform support. Council never stages or commits
exports into Git and never includes them in release inputs. It does not claim that a
user's home or private root is outside every Git repository or project tree. Unknown
identity, unsafe mount/link/reparse configuration or unsupported exclusive-create and
file-flush semantics fails closed. `os.Root` or a string
prefix alone is not sufficient proof. The already-held v2 root is used directly; this slice creates no export subdirectory.
The `history-*.json` basename namespace is reserved centrally for output and cannot be
used by Council control or temporary files. Every such output remains private state.

Revalidate the exact snapshot, preview, target absence, held parent identity and
current owner at the effect boundary. Commit and positively acknowledge a single
irreversible publication permit before any operation-owned output-file creation.
Create the selected **final filename directly** with an anchored, no-follow,
exclusive-create operation under the verified held private directory, then write
only through its retained handle. Never stage a second file or publish by rename.
The permitted create, writes and sync observations belong to the same exact
operation and effect journal; the design must durably record the required step
intents and observed outcomes, and no helper or unregistered actor may bypass the
permit. Verify the retained file identity and final name before reporting success.

The output may be visible while incomplete. Partial bytes, a missing acknowledgement,
or an ambiguous file, parent or sync result remain explicitly incomplete or
uncertain, never successful. Do not automatically delete an unverified or partial
output, overwrite it, or retry its creation. Each supported platform must establish
its actual no-follow/exclusive-create, retained-handle, file flush and directory-sync
observation semantics. A documented unavailable Windows directory sync is disclosed,
not represented as a power-loss guarantee. Other unsupported required guarantees fail
closed; no general power-loss or network filesystem durability is promised. The private-root trust model excludes hostile
same-user namespace tampering, but detected changes and ordinary collisions are
still refused.

## Receipts, uncertainty and reconciliation

Snapshot capture, preview, export and reconciliation each persist a fingerprint
binding the operation and all strict request fields. Exact replay returns the
original immutable receipt and original identity before generating work; a known
completed result is returned unchanged, while `history operation` can show the
current durable observation. A reused request ID with any changed input is
`request_conflict` (409). Retain a privately captured CLI/UI request body so a lost
reply is retried without substituting a later selection. An uncertain receipt or
filesystem effect is never a reason to retry an OS call. Pre-effect invalid input,
existing target and stale/unsafe authority return a specific bounded error without
overwriting; `write_uncertain`, `effect_uncertain`, `owner_unavailable` and
`dispatch_fenced` follow the existing 503 family. Add documented status mappings
for new history errors rather than accidentally returning the generic 500.

The supported positive `verify_no_publication` case requires **all** of:

1. A complete trustworthy append-only effect journal for that exact operation shows
   no committed publication permit, and the owner enforces that no operation-owned
   output creation or write is possible without its positively acknowledged durable
   permit. A failed/ambiguous permit acknowledgement never authorizes an OS call.
2. An owner transaction freezes the operation/revision and blocks future permits.
   While retaining an admission barrier, the owner drains or proves terminated all
   registered operation actors and effect-capable handles. A timeout, untracked
   actor, journal gap or in-memory unrecorded fence means `unresolved`.
3. Same-owner reconciliation verifies the current held lock and exact generation;
   a live current epoch is dirty and need not be clean. Cross-owner reconciliation
   requires an unbroken begin/clean chain for the exact prior effect-capable owner
   and all intermediate owners, with this feature's actors, handles and journal
   included in each clean check. Existing 007 epoch checks alone do not provide this
   proof. A dirty, missing or mismatched chain remains unresolved.
4. One final transaction rechecks the freeze, journal, owner, exact request/revision
   and independent claims/incidents, records the decision and releases only this
   operation's proven-safe blocker. A separate blocker remains fenced. A later
   publication requires a new preview and new request; the frozen operation cannot
   receive a permit.

This decision proves no Council-authorized effect for the operation; it does not
assert the target path is absent/unchanged or another process had no effects. A
present or possibly committed publication permit, unsupported journal, missing source
or merely missing file remains unresolved; no automatic read, restart or receipt replay may
clear the fence. All effect and decision audit records survive the decision.

## Required acceptance cases

Exercise real owner/store/fake-agent flows in two populated projects, checking exact
page/encoded bounds and 64-bit canonical values, immutable snapshot pages, no
cross-project leakage, all omissions, excluded application-level access sentinels,
unknown lineage, source drift and no retention-triggered model call. Test existing
and swapped targets, unsafe symlinks/reparse/mounts, invalid filename/device names,
unsupported filesystem guarantees, direct partial-file visibility and no-overwrite
behavior on each supported platform. Race freeze with actor and publication permit,
inject failure before and after the permit/create/write/sync/observation/receipt
boundaries, kill an owner, test delayed worker
drain, clean/dirty generation chains, journal corruption and independent blockers.
Compare UI and CLI for exact identities, lost replies, narrow keyboard focus and
retained newer drafts. Negative cases must preserve unrelated files and must not
mark uncertain results complete. Full evidence backup/restore and cleanup remain
mandatory later features.

Inspection and owner-mediated `verify_no_publication` remain available while this
operation or an unrelated incident fences new dispatch/effects. They validate owner
and journal authority without requiring an open dispatch gate, and must preserve all
unrelated incidents and fences. UI and CLI tests cover that recovery path.

History-specific persistence incidents are durably bound to the exact operation or
pre-effect request identity and fingerprint in the same transaction as their global
fence. Reconciliation may settle only the linked incident after its specific durable
proof, then must recheck every other incident, claim and pending effect before opening
any gate. It must not infer linkage from messages or timestamps. An unbound or
unacknowledged fence remains uncertain and dirty; the history feature does not
automatically clear it. A proven unpermitted operation permits only a no-publication
decision, not execution or removal. Receipt-linked capture/preview/create incidents
require exact receipt proof and cannot themselves authorize publication.
