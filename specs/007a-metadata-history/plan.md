# Implementation Plan: Project Metadata History

**Feature context**: `specs/007a-metadata-history/` | **Git branch**: `refactor` | **Date**: 2026-11-08 | **Spec**: [spec.md](spec.md)

## Summary

Add a project-scoped, incomplete observational metadata snapshot, inspectable through the existing owner, CLI and branded UI, and exportable to a selected unused filename inside a verified Council-private directory. It contains only the explicitly eligible structured fields of the trusted owner-locked Council store, never opaque text, paths, artifact bytes, vendor authentication files or imported history. It is not evidence export, backup, restore or executable authority. Full retention remains mandatory in later slices.

## Technical Context

- **Language**: Go 1.27.3 and authored TypeScript embedded in the binary; no runtime npm dependencies.
- **Persistence**: Current SQLite/store and owner lock in `<user-home>/.council/v2/`; versioned migration with strict validation and exact receipts. Preserve legacy state.
- **Interfaces**: Existing owner API, CLI and HTTP/UI, strict typed requests and lossless decimal counters. No host MCP parity claim before feature 008.
- **Publication**: User selects one safe filename under an owner-controlled private export root; arbitrary local paths, imports, external publication and overwrite are unsupported. Use native anchored identity/containment checks and exclusive publication; fail closed if the platform cannot establish them. Do not assume `os.Root` alone prevents mounts or all internal links. The bounded threat model trusts the Council-private root against hostile same-user namespace tampering; ordinary collisions and detected changes fail closed. No resistance to an arbitrary same-user process modifying private state is claimed.
- **Bounds**: Finite row, field and encoded-byte limits are declared in contract. Capture a consistent selected-project read transaction with explicit columns and validated references. Reject overflow, invalid or cross-project links; do not silently truncate.
- **Verification**: `npm run build`, `npm test`, `npm run check`; focused real-store/owner/filesystem fake fixtures; actual browser keyboard/narrow layout; independent review and six native plus race CI. No live provider calls.

## Constitution Check

- Useful, proportionate work: a genuinely populated project yields a usable inventory with zero model calls; output is explicitly incomplete and non-authoritative.
- Interoperability and privacy: never fetch/decode opaque rows to mine fields, open vendor authentication or inspect payloads. SQLite can read co-located physical pages; no stronger disk-I/O claim is made. The Council-managed database at the established private path under the native owner lock is explicitly trusted; no foreign import is supported, and the schema cannot detect same-user database copying or tampering.
- Autonomy and protection: explicit capture and export scope, no destructive action, external publication, overwrite or automatic dispatch. The private root is physically validated; Council never stages or commits exports or includes them in release inputs. Do not claim the user’s home can never be inside a Git repository.
- Truthfulness: every exported representation states omissions, unknown outcome, exact captured relationships and no restored authority. Every effect is durably journaled and request replay does not republish.
- Design gate: positive no-publication reconciliation requires enforced exclusive publisher, durable append-only permit, frozen operation and validated journal; dirty prior-owner incidents remain separately fenced. See [research.md](research.md) and [contracts/operations.md](contracts/operations.md).

## Architecture and Durable Boundaries

1. Add versioned Store schema for bounded immutable metadata snapshots, request-fingerprinted capture/export receipts and effect journal, using serialized transactions and exact integer representations. Schema migration must preserve v7 rows and validate both upgraded and new databases; never read vendor credentials or legacy state.
2. Capture one selected project using explicit SQL columns only and a single bounded read transaction, recording its capture boundary inside that transaction and validating IDs, enums, project membership and supported relationships. Persist the exact immutable snapshot and omissions; do not call lock-taking Store functions recursively or infer task outcomes, graph edges or provider usage from missing fields.
3. Freeze an explicit safe filename, snapshot identity/revision and selected private export root in a preview. Serialize the output from the captured snapshot only; no model process or source-payload reads. A supported output includes a version, scope, explicit omissions, lossless numbers, incomplete label and digest of the encoded selected data. Private output path is not embedded in the export.
4. Within a registered, lifecycle-drained owner operation, durably record intent, then a single irreversible publication permit before any output-file creation. Create the selected final filename directly with one no-follow, anchored exclusive-create operation under the held verified private directory, then write only through its retained handle. Partial bytes may be visible until completion; report them as incomplete or uncertain, never as success, and do not automatically delete them. Verify the retained handle and final name before recording success. Do not overwrite, follow unexpected links, stage files in Git or include outputs in release artifacts. Require a successful file-content flush and record directory-sync outcome honestly; Windows may report its documented directory sync as unavailable without claiming power-loss namespace durability. Unexpected flush/sync failures remain uncertain. No unmanaged child/background publisher may bypass the permit.
5. Exact retry returns the immutable receipt and current observed state. A permitted or maybe-permitted effect with unknown outcome remains uncertain and fenced; do not replay or delete an unknown output. Reconcile a no-publication case only after atomically freezing the operation, draining all its actors, validating the complete durable journal, verifying no permit ever committed, confirming owner/generation facts and preserving independent incidents. This proves only no Council publication, not the state of arbitrary external paths.
6. Expose snapshot capture, inspect, export preview/commit and operation inspection/reconciliation through one app contract, CLI and HTTP, with explicit named keyboard controls and stable selection/draft handling in UI. Never route historical IDs into mutation authority.

## Project Structure and Ownership

- `internal/domain/`, `internal/store/`: typed history DTOs, schema/migrations, bounded SQL projection, receipts and journal.
- `internal/localstate/`, `internal/engine/`, `internal/app/`: trusted private root, owner lifecycle, capture/export coordination and reconciliation.
- `internal/httpapi/`, `internal/cli/`: strict common routes and CLI commands, using the same authority.
- `ui/src/`, `ui/tests/`, `internal/webui/dist/`: branded history UI, actual browser tests and committed browser build.
- `specs/007a-metadata-history/`: specification, research, contract, tasks, independent review and acceptance.

Implementation ownership is assigned explicitly in tasks; edits to shared contracts or migrations are serialized. Compile assets are committed with authored UI.

## Required Evidence

Tests cover ordinary Council-created task/attempt history in two projects, excluded-content sentinel access, lossless integers and bounds, mixed DAG omissions, source drift and concurrent capture, exact retry/lost acknowledgement, crash before/after permit and publication, output existing/link/swap/changed-root rejection, unsupported native durability, unrelated work preservation, and positive and negative no-publication reconciliation without clearing independent owner incidents. Exercise UI and CLI against the same actual fake owner, including keyboard at 360px, and count zero retention-induced model calls. Run full required local commands, independent review and exact-source hosted native/race checks before acceptance. Full backup/restore, content-bearing evidence and reviewed cleanup remain required and do not inherit this slice's acceptance.
