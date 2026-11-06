# Implementation decision amendments

**Feature**: 002 | **Attribution**: root integrator | **Date**: 6 November 2026

These amendments refine the accepted design after integration tests and independent review.
They do not qualify a live provider or expand the feature into routing, verification or integration.

## Durable ownership before native creation

Record the guarded launch intent before creating even a suspended Windows worker or Unix READY
guardian. Recheck that intent under the launch/control mutex immediately before ResumeThread/GO;
an idempotent recheck still validates current version, Stop, deadline, gate and claim. This closes
the crash window where an existing native scope could otherwise look like never-created work.
Recovery treats an unreconciled intent conservatively and does not dispatch it again. A typed
internal `NoProcessCreated` settlement fact can identify an independently known preparation failure;
it is never accepted from HTTP, CLI or agent output. Uncertain preparation retains its claim.

## Input delivery is a separate success requirement

Retain expected/written stdin byte counts and completion/error state independently of root exit,
terminal output and cleanup. Join the bounded input writer during cleanup. A launched empty input
is complete; pre-GO cleanup is not-started rather than a fabricated input failure. A known short
or failed write prevents successful authoring even if the worker emits a valid terminal and exits
zero. The controller also requires durable acknowledgement of the typed delivery-proof event;
uncertain proof persistence fences dispatch and cannot produce success. Distinguish physical
short delivery from a storage acknowledgement fault. Counts attest completed writes to the owned
OS pipe, not that a model consumed or followed the instructions. Native tests drain/hash a full
Unicode input and separately read only one byte before valid-looking exit-zero output; the store
independently rejects success without complete input proof.

## Non-interactive native permission options

The current official exec parser exposes shared sandbox options and command-scoped `--config`,
while `--ask-for-approval` belongs to the interactive CLI. Use `--sandbox danger-full-access` and
`--config approval_policy=\"never\"` for this invocation, rather than passing an unsupported
interactive flag after `exec`. No vendor-global configuration is edited. Native Auto and exact
model pins remain separate from unreported effective model/account facts. Source review is not
live binary/provider qualification.

Sources: [official command reference](https://learn.chatgpt.com/docs/cli/reference),
[official exec argument parser](https://github.com/openai/codex/blob/main/codex-rs/exec/src/cli.rs),
[shared native options](https://github.com/openai/codex/blob/main/codex-rs/utils/cli/src/shared_options.rs).

## Normal Git index configurations

Support SHA-1 index versions 2, 3 and 4, including bounded v4 prefix compression and Git's documented
all-zero `index.skipHash` checksum. Verify nonzero index checksums, retain the original raw index
and independently hash captured source evidence. Neither v4 nor `feature.manyFiles` requires a user
configuration change. Split, sparse, intent-to-add, unmerged, submodule, unborn and unsupported
object formats remain specific blockers. Tests use actual Git-generated v4 compressed paths,
multi-byte prefix lengths, optional checksums and malformed prefix bounds.

Sources: [Git index format](https://git-scm.com/docs/index-format),
[Git index configuration](https://git-scm.com/docs/git-config#Documentation/git-config.txt-indexskipHash).

## Linear evidence reads with explicit proof scope

Internal `CopyTo` opens a private physical artifact and hashes the bytes actually copied in one
linear pass, checking exact length, expected hash, closed completeness and cancellation. Source
materialization and final-response reading use this operation. This removes the previous quadratic
full-file hashing from repeated 64 KiB reads.

Public closed-artifact range reads derive aligned 64 KiB chunk digests in the same pass that
validates the retained expected whole hash. Cache at most 128 digest tables and 4 MiB of digests,
keyed by artifact ID, physical identity, expected hash and length. Each request reopens and checks
the file's identity/size, hashes every aligned chunk spanning the requested range, compares the
trusted digests and only then slices the returned bytes. This proves the returned range. Damage
elsewhere is detected when accessed or by fresh whole verification; a range response does not
assert current whole-file freshness. The returned SHA is the retained expected whole-artifact hash.
Response acceptance always performs fresh complete-file verification outside a SQL transaction,
bypassing this cache. Independent correction review and focused corruption/copy tests cover this
distinction. Live/incomplete artifacts remain bounded to their acknowledged durable prefix.

## Termination budget and secondary faults

The absolute 15-second termination budget includes post-process evidence handling. A watchdog
starts from durable Stop acceptance or the beginning of normal termination, using the earlier
deadline. If settlement remains incomplete, latch dispatch closed, retain an incident and unknown
cleanup with its exact scope, and keep capacity. Do not invent a terminal while evidence writers
may remain active. Cooperative contexts bound native cleanup and subsequent capture; no hard
real-time guarantee is made for a nonresponsive OS/filesystem. Closing the owner ends its watchers
only after its worker lifecycle is joined.

Always retain bounded secondary failure code/message/time in event metadata while preserving the
first ordered primary failure or accepted Stop. Expected cancellation and resource-limit outcomes
remain distinct from uncertain persistence; unknown durable writes fence dispatch without replay.

## Inspectable blockers and provenance

Attempt detail, wait and current task detail resolve optional source snapshot, candidate and
incident records. Metadata also exposes the active retained incident. UI/CLI can inspect source
base/branch/manifest, managed workspace reference, pending verification/integration and actionable
incident code/message without an arbitrary-path endpoint. Preserve bounded safe workspace blocker
reasons, distinguish deadline expiration during preparation and keep observation independent of
task execution. Unknown requested acceptance identities do not constitute evidence corruption;
registered evidence that fails fresh/range integrity does retain a corruption incident.

## Durable writes without repeated immutable publications

Use SQLite WAL with synchronous=FULL (readback 2). Enable WAL only after validating the existing
database application identity, schema and tables; unsupported modes fail without resetting or
silently falling back. Apply foreign_keys=1, synchronous=2 and busy_timeout=5000 to every physical
connection through the pinned driver's documented constant `_pragma` parameters. Construct an
escaped absolute file URI from the literal state path so Unicode and URI punctuation cannot add
parameters. WAL activation remains outside that connection URI and follows schema validation.

Immutable artifact Put synchronizes the newly created file and directory entry, writes all bytes,
synchronizes and hashes them, closes the writer, then publishes one final closed metadata record.
It does not publish collecting rows or intermediate 256 KiB checkpoints. Ordinary streamed Create
retains those checkpoints and acknowledged-prefix semantics. A failed immutable write may salvage
provable incomplete bytes without a complete hash; it never reruns the writer or agent. A final
metadata acknowledgement failure is returned without retrying or downgrading a potentially
committed closed row. Repeated Close returns the same result.

Known cancellation before a metadata commit remains distinguishable from write_uncertain after
Commit or its acknowledgement. Genuine file durability, corruption or commit uncertainty wins
over an adjacent cancellation in a joined error. Fence admission in memory before the scheduler
can observe an uncertain admission/control write, retain its incident where storage permits, and
permit reconciliation of the original receipt without authorizing another run. Orderly shutdown
targets the exact current owner even when its receipt acknowledgement is uncertain; replaying an
old owner's receipt never stops the replacement owner.

A native Windows ARM64 preparation trace located a slow immutable index-artifact metadata commit
inside FlushFileBuffers. The local eight-artifact diagnostic compared DELETE/WAL and one/two
publications under FULL synchronization; it did not reproduce the 20-second outlier. Cold/warm
cache and fixed run order confound timings. It supports removing redundant metadata publications,
not a general speedup percentage or a hard latency guarantee. Abrupt-process-exit tests retain a
nonempty hot WAL, preserve its exact bytes through owner-lock preparation, then recover the
committed policy/receipt and check database integrity. They do not simulate power failure.

## Private SQLite companions and physical identity

After acquiring the native owner lock, inspect existing exact database journal/WAL/SHM companions
without creating, truncating or deleting them. Reject redirected objects and ordinary files with
multiple hard links before changing permissions. Normalize eligible private files on the retained
checked object handle. On Windows the private directory DACL restricts newly created SQLite
companions by inheritance; do not claim each new companion has a separately protected DACL.
Directory ACL correction must not propagate into unchecked children. Descriptor reads verify the
actual opened handle before reading bearer material. Tests retain outside file bytes, identity and
permissions when rejecting aliases, and verify recovery does not discard committed WAL bytes.

On Windows, a continuing owner retains os.OpenRoot. Its directory handle does not share DELETE;
reopening that directory with MAXIMUM_ALLOWED can request DELETE and fail before native lock
arbitration. Reuse a directory only after read-only verification of its ordinary physical identity,
current owner, protected restricted DACL and effective object/container inheritance. Reject
inherit-only or no-propagation grants. A directory requiring normalization still takes the checked
MAXIMUM_ALLOWED no-propagation path or fails closed. Do not weaken rooted-handle sharing or mutate
an already-private ACL merely to discover an active owner. The native regression combines a live
WAL, retained root and actual running owned fake child, then requires ErrLocked without changing
the database or interrupting that child. Separate inheritance cases verify normalization/blocking.

## Retained response identity across mutable writes

Keep the newly created response file physically open until one matching import consumes its
reservation or the engine explicitly releases it. A cached device/inode or file ID after closing
the original object cannot prove identity: the filesystem may reuse that identifier for a new file
at the same path. Compare the live reserved object with both the path and opened import handle,
then recheck identity after copying. Native same-object writes remain supported. Import, invalid
bounds, cancellation, copy/metadata faults, unlaunched preparation and owner shutdown all release
their owned handles; mismatched attempt IDs cannot consume another attempt's reservation. Close
faults remain typed durability faults, with raw expected cancellation preserved when no real fault
coexists. The engine defers idempotent ReleaseOutput immediately after successful reservation.

## macOS signal delivery versus cleanup proof

XNU's killpg1 excludes zombie members and can report EPERM for an existing zombie-only group.
The original macOS CI failures are consistent with that kernel behavior; their earlier bounded
messages did not retain errno, so the exact failed syscall category was not directly observed.
The corrected code retains bounded TERM/KILL categories and does not equate signal delivery with
group emptiness. Attempt final KILL while the owned guardian is still unreaped, then reap it and
require an actual signal-0 ESRCH probe. Retry transient EPERM/EACCES observations on the existing
20 ms ticker within the original deadline, since orphan zombies can remain briefly after guardian
reap. Persistent denial or another unresolved observation stays unknown. A live, inaccessible or
reused group never becomes verified from a delivery error, and no nonzero signal is sent after
reap. Pre-GO abort uses the same ordering.

Primary source: [XNU killpg1 at verified commit f6217f8](https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/kern/kern_sig.c#L1710).
Fault-injected native fixtures require actual exit/absence, preserve root observations and reject
permission-denied probes or skipped reap. Compilation is not macOS/Linux runtime qualification.
