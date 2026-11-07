# Backup Source Intake — Research Draft

**Status: draft research only; not an approved contract or implementation.**
This document considers how a local backup source could be selected and parsed
without opening a vendor-authentication object. The user's answer about the
eligibility of explicitly selected opaque data remains pending. Neither this
draft nor a user-supplied pathname weakens the prohibition on inspecting,
hashing or copying vendor credentials.

## Threat model and essential distinction

The input file, its path components, archive contents, manifest and destination
are untrusted. A path selected for import can directly name a vendor-authentication
file, reach one through a symbolic link, reparse point, junction or mount, or
refer to the same underlying object through a hard link. A path or file can also
change between validation and opening. Archive data may be malicious or malformed.
The design must never read a candidate's header to discover whether it is a
backup **before** source eligibility is established: even reading a magic value
would already open and inspect the selected object.

Two independent questions must be answered:

1. **Source access:** is the actual filesystem object safe to open, including
   every alias and race at the access boundary?
2. **Payload eligibility:** are all bytes that will be read, hashed, copied or
   restored eligible under the user's eventual interpretation of the credential
   boundary?

Directory containment alone does not answer the second question. A correctly
signed header, extension, checksum, encrypted wrapper, user selection, ownership
bit or claimed provenance does not establish that opaque data cannot contain a
credential. A plain SHA-256 verifies selected bytes against the manifest; it is
not authentication or proof that those bytes are safe to inspect or copy.

## Bounded source option and portability limit

The conservative candidate is to accept an opaque Council-issued backup ID only
for a previously registered, exclusively created output whose original safe
file handle remains held by the current owner. Establish eligibility for that
output's exact data independently; the registry itself does not establish it.
Do not accept an arbitrary external path, unregistered inbox file or stream as
proof of eligibility. Copying an unknown candidate into private staging is not a
safe way to check its source: the copy already opens and reads the candidate.

This constrained option can demonstrate a same-owner registered round trip. It
does **not** provide cross-machine portable restore, recovery after the last
trusted handle is lost, or a full backup of arbitrary current project state. A
registry record and a later pathname reopen are insufficient if an untrusted
writer can replace the name or substitute a hard link before the new open. An
OS-protected non-writable source, native identity and platform-specific handle
rules may support a broader threat model, but must be designed and proven for
each supported platform; they are not assumed here. If a same-user adversary can
replace names and the rule forbids even opening a credential alias before the
identity comparison, fail closed when those guarantees cannot be established.

Consequently this source option alone does not satisfy RET-004 or the full
portable-backup user journey. The user-facing scope and the production payload
eligibility question require an explicit decision before design approval. The
limitation must not be hidden by a fixture-only successful round trip or called
portable recovery.

## Path and platform considerations

- Go's [`os.Root` documentation](https://pkg.go.dev/os#Root) describes useful
  root-relative containment but states that methods follow in-root symbolic
  links and do not exclude filesystem boundaries, Linux bind mounts or device
  files. The [Go traversal guidance](https://go.dev/blog/osroot) also explains
  why resolving a pathname and opening it later permits a swap race. Do not
  interpret `filepath.IsLocal`, `EvalSymlinks`, or an `Lstat`/open pair as a
  universal no-credential-open proof.
- On Linux, a held directory descriptor and `openat2` resolution constraints
  such as `RESOLVE_BENEATH`, `RESOLVE_NO_SYMLINKS`, `RESOLVE_NO_MAGICLINKS` and
  `RESOLVE_NO_XDEV` described in the [Linux kernel interface](https://code.googlesource.com/linux/torvalds/linux/+/68ff4a3ccda9f98c74f23c70c8c7c581f9eee931/include/uapi/linux/openat2.h) are candidates for a narrowly defined local-filesystem
  boundary. Availability, flags and fallback behavior require native tests;
  unsupported or unprovable cases fail closed. These flags do not independently
  establish that file contents are credential-free or rule out hard links.
- macOS lacks Linux `openat2`; a component-by-component held-handle design and
  its mount, filesystem and no-follow behavior need separate analysis and
  qualification. Do not silently substitute a path-prefix check or claim Linux
  guarantees on macOS.
- On Windows, review held directory/file handles, native file identity, link
  count, reparse tags, alternate data streams, reserved device names, case and
  short-name aliases, volume aliases and network/UNC paths. Microsoft describes
  the effect of `FILE_FLAG_OPEN_REPARSE_POINT` in its
  [reparse-point guidance](https://learn.microsoft.com/en-us/windows/win32/fileio/reparse-points-and-file-operations)
  and [CreateFile documentation](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilea).
  Bypassing normal reparse processing for an opened component is not itself a
  proof about every intermediate component or an already-existing hard link.
- Native metadata inspection and a no-follow open need a precisely documented
  policy when a candidate might itself be an alias of a credential. In
  particular, opening a swapped hard link and rejecting it only after comparing
  its identity does not satisfy a threat model that prohibits the open itself.
  Do not inspect real credential files to create a protected-identity list.

## Parsing after source eligibility

Only after the source-access and payload-eligibility decisions permit access,
read through the held source identity into bounded Council-controlled staging.
An open handle does not prevent a concurrent writer modifying a file; detect
change and report uncertainty, or require an independently enforceable immutable
source. A before/after timestamp alone is not proof of immutable content.

Use a versioned format with limits on input and expanded bytes, member count,
path length/depth, metadata size and processing time. Reject duplicate member
names, undeclared members, invalid encodings, absolute or traversal paths,
symlinks/hard links/special files, unsafe platform-specific names, case or
normalization collisions, inconsistent member sizes and invalid reference
closure. Verify compatibility, all declared members and digests before any
destination effects. A structurally valid but incomplete bundle remains
incomplete; it does not become a complete backup by passing syntax checks.

Restore to an explicitly selected absent isolated destination. Revalidate the
held source and destination identities and all authority at the effect boundary;
do not overwrite, merge into a live owner or auto-resume writers. Stage and
journal each filesystem effect under the single owner and retain a fence for an
unknown effect or acknowledgement. Record actual directory-sync/platform
limitations and never claim an unverified rollback or model call.

## Controlled verification

- Create a disposable **fake** credential sentinel outside a disposable source
  root. Exercise direct selection, links, parent-link swaps, final-component
  swaps, hard links, junctions/reparse points, short or case aliases, alternate
  streams and unsupported device/network forms where available. Assert no
  credential-target open, read, hash or copy occurs; no real vendor files or
  user state are accessed.
- Native test helpers should observe the actual opened object/handle at the OS
  boundary where supported. In-process mocked I/O hooks alone demonstrate only
  calls passing through those hooks and must not be reported as universal
  syscall evidence. Mount/bind tests that need privileges must be explicitly
  skipped and reported when unavailable, not treated as passing evidence.
- Include a positive same-owner registered fake backup round trip to demonstrate
  that the protection does not merely reject every source. Separately test a
  replaced registry path, changed source during parsing, lost acknowledgement,
  crash after each effect, already-existing destination, malformed/oversized
  archives, duplicate names, path traversal, decompression amplification and
  no automatic dispatch. Preserve retained state on every uncertain case.
- Test the required Windows, macOS and Linux architectures using actual native
  filesystem fixtures; report unsupported filesystems and unperformed cases.
  The positive fake round trip is not evidence of production credential-free
  payload provenance or cross-machine portability.
