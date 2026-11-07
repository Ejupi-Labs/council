# Implementation Plan: Scoped Retention and Recovery

**Feature context**: `specs/007-scoped-retention/` | **Git branch**: `refactor` | **Date**: 2026-11-08 | **Spec**: [spec.md](spec.md)

**Status: draft; the production payload eligibility decision remains open. No implementation is authorized by this draft until the design gate and independent review are closed.**

## Summary

Provide project- and task-scoped export, verified project backup/restore, and reviewed artifact cleanup through the existing single local owner, UI and CLI. Maintain durable exact-request identity and explicit effect outcomes; preserve unrelated projects and work; never silently replay work. A useful complete production backup path is mandatory, and a fixture-only proof or metadata-only history cannot establish it. Determine which production content can be captured within the credential boundary before designing or implementing payload reads.

## Technical Context

- **Language and platform**: Go 1.27.3, embedded authored TypeScript, Windows/Linux/macOS on supported amd64/arm64 targets.
- **Storage**: Existing local owner and SQLite store under user-home `.council/v2/`, with private managed evidence and workspaces. Legacy state is separate. Do not copy a live database, WAL, or owner files.
- **Dependencies**: Existing Go dependencies and development-only npm tools; no runtime npm dependencies. Evaluate and record any new dependency before selection.
- **Interfaces**: Same owner and exact operation receipts for CLI, HTTP and branded browser UI; host MCP parity remains feature 008.
- **Tests**: Controlled fake-agent and real filesystem/store/owner tests; `npm run build`, `npm test`, `npm run check`; browser keyboard/narrow layout and six-platform/race CI. No live providers.
- **Scope**: One active local user and explicitly selected task or project. Explicit local-only output; no external publication.
- **Unresolved design gate**: User-visible ordinary project/evidence bytes can contain secrets, and existing artifacts have integrity but no proof of credential-free origin. A user selection, filename, digest, encryption or opaque copy does not prove absence. Obtain and encode the user's precise interpretation of selected project content versus vendor-authentication material, then assess it against the constitution; without a safe production path RET-004 stays open.

## Constitution Check

- Useful programmer outcome and traceable restore: required; an empty project or metadata-only projection is not evidence of full production backup.
- Proportionate use: no model calls for retention operations.
- Vendor credentials remain with vendors: establish permitted source and archive handling before opening, hashing or copying a candidate; unknown provenance cannot become a success claim.
- Autonomous work respects explicit selected scope and does not delete unreviewed, unaccepted or changed work without fresh authority.
- Truthful states and no replay: record intents before effects, observed effects afterwards, and keep uncertainty visible and fenced. Restore grants no execution authority.
- UI/CLI parity, accessible Ejupi Labs identity, preservation of unrelated projects and legacy data, and local-only fake qualification are required.
- **Post-design gate: OPEN** pending provenance decision, complete per-effect contracts and independent consistency review.

## Phase 0 — Research

See [research.md](research.md). Record source eligibility, manifest/completeness, safe archive parsing, owner and filesystem lifecycle, cross-platform atomicity, and cleanup without assuming a path check is an identity proof. A real production positive scenario must demonstrate the selected policy rather than simply manufacture fixture provenance.

## Phase 1 — Design

Specify typed selections, project snapshot, durable operations, receipts and effects, archive manifests, read-only imported history, restored ownership and reviewed cleanup in [data-model.md](data-model.md) and `contracts/`. Determine how untrusted backup input is accepted without inspecting or copying vendor-authentication objects and how the real owner exposes restored data without executable authority. Specify explicit limits, revalidation, no-overwrite publication, failure states and owner-mediated reconciliation. The design must not claim filesystem and SQLite changes are one atomic transaction.

## Project Structure

- `internal/store/`: selection snapshots, persistence, operation/receipt/effect authority and history views.
- `internal/engine/`, `internal/evidence/`, `internal/workspace/`: owner-guarded capture, validation, restore and cleanup, using established platform-native identity controls.
- `internal/app/`, `internal/httpapi/`, `internal/cli/`: one operation contract with strict inputs and replay semantics.
- `ui/src/`, `ui/tests/`, `internal/webui/dist/`: branded UI, accessible interaction and committed generated assets.
- `specs/007-scoped-retention/`: this specification, research, plan, contracts, task list and acceptance evidence.

Exact file ownership and decomposition will be fixed in the task list after research and contract review. A supported positive path and its cross-platform safety proof are gating requirements, not optional follow-up work.

## Verification and Release Gate

Tests must include two populated projects and tasks, real eligible production-shaped sources, every selected member and omission, source/destination changes, incomplete streams, no duplicate effects on lost acknowledgement, owner crash at each journal boundary, malformed/oversized/traversal/link archives, collision and unrelated work protection, UI–CLI parity and zero automatic admission. Independent review and corrected blockers precede acceptance. The exact source must pass the required local commands and hosted native/race matrix. Record any unsupported policy case and unperformed live test explicitly; do not accept or start feature 008 while mandatory scope remains unresolved.
