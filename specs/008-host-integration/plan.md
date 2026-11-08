# Implementation Plan: MCP Host Integration and Interface Parity

**Feature context**: `specs/008-host-integration/` | **Git branch**: `refactor` | **Date**: 2026-11-08 | **Spec**: [spec.md](spec.md)

## Summary

Add `council mcp` as a native stdio MCP server that connects to the same persistent authenticated local owner used by CLI and UI. Provide explicit, documented tools for the currently accepted workspace, task/execution, routing, workflow, graph, recovery and incomplete-history operations. Preserve the existing records, exact authority, receipts, bounded evidence bytes, cancellation semantics and honest unsupported capabilities; do not introduce a second engine, hidden orchestrator, provider call or external publication.

## Technical Context

- **Language and runtime**: Go 1.27.3. Continue the embedded authored TypeScript UI and prohibit runtime npm dependencies.
- **Protocol**: Evaluate and pin the official Go MCP SDK `github.com/modelcontextprotocol/go-sdk` v1.8.0, qualifying only protocol versions 2025-06-18 and 2025-11-25 initially unless offline fixtures demonstrate otherwise. Use stdio only; no HTTP-facing MCP endpoint or remote tenancy in this feature. See [research.md](research.md) and official upstream links therein.
- **Dependency governance**: Pin all added Go modules with checksums. Inspect actual selected transitive licenses and record notices before release; the SDK's tagged license states an ongoing MIT/Apache-2.0 transition. No vendor credentials or live provider access are needed.
- **Bounds**: Explicit maximum 64 MiB protocol line, with lower existing operation-specific body, text and paging limits enforced before effects. Reject invalid UTF-8, duplicate or unknown keys, malformed schemas, invalid numeric forms and oversized requests without truncating; verify the actual SDK/transport path enforces these boundaries. Lossless identifiers and counters remain strings.
- **Owner**: Extract or share the existing native owner client/forwarding seam; attach or start the same owner via `owner.Ensure` and `os.Executable`, with existing descriptor verification, local origin/authentication, fixed routes, no proxy/redirect and bounded response. Never expose its bearer token in tools, diagnostics, config examples or output. Initialize and list tools without requiring current-owner compatibility. Attach lazily per operation: a known compatible maintenance contract may service only the narrowly scoped owner Stop, while all execution/history tools refuse an incompatible owner. Preserve an incompatible locked owner and never replace it.
- **Testing**: Local controlled fake agents and a real SDK protocol client over subprocess stdio; no live provider calls. Run `npm run build`, `npm test`, `npm run check`, meaningful app/owner/protocol integration, and six native plus Linux race CI. Preserve committed UI assets and release exclusions.

## Constitution Check

- Useful programming outcomes: host tools use the same project/task, workflow, graph, checkpoint and evidence records as UI/CLI, not a decorative connection or external phase plan.
- Proportionate usage: listing, observation, metadata, waiting, retries and protocol setup do not call models or reroute tasks. Explicit task admission retains the existing routing policy.
- Honest interoperability: declare actual protocol/version/operation support and unsupported native harness features. No claimed subscription saving or provider capability without measured evidence.
- Existing autonomy and scope: carry original bodies and captured authorities unchanged; connecting a host does not change policy, grant permissions, invoke a provider, or publish externally.
- Truth and safety: transport IDs are not durable request IDs; retries matching the established owner-canonical semantic fingerprint replay existing receipts; documented omitted/default equivalents are not falsely described as conflicts. Host cancellation ends only the observer; explicit Stop is a separate operation. Unknown effects remain uncertain and fenced.
- Simple accessible product: concise English host setup, no vendor credentials in configuration and no unnecessary settings. New UI is not required for a stdio integration.
- One feature at a time: this design and tasks precede code; independent review and acceptance precede the next feature. Full retention remains mandatory before release.

## Architecture and Boundaries

1. Extract a small internal owner client with fixed allowlisted route mappings, strict metadata/capability negotiation and preserved older-owner maintenance Stop. Share it with CLI where practical without changing frozen-intent or output behavior. The MCP process must not open the database or create an independent Engine. Repeated clients converge on one owner and do not treat host disconnect as owner shutdown.
2. Define a closed, versioned catalog that maps each documented MCP tool to a concrete accepted owner operation, strictly typed argument schema, permissions and output envelope. Before enabling any dispatcher, generate and review versioned complete closed machine-readable schemas against the cited owner DTOs and validators, including presence and nested conditional rules; require schema-to-owner parity tests. Do not expose a general HTTP or arbitrary shell proxy. Validate complete original raw arguments with Council's duplicate/unknown-key rules before conversion and verify the SDK's protocol envelope handling. The selected SDK does not enforce the initialized notification before tool dispatch; provide a bounded validating, ordered per-session transport gate or equivalent reviewed middleware that rejects pre-notification calls and duplicate/invalid frames before any effect, including concurrent pipelining. Explicit captured authority is mandatory; never synthesize fresh graph/node/budget authority on retry.
3. Register the catalog through the official SDK's low-level raw handler when necessary to avoid schema-default mutation. Expose Council's structured envelope in object `structuredContent` with safe text fallback and `isError` for domain failures; reserve JSON-RPC errors for malformed/unsupported protocol. Sanitize all diagnostics and write nothing except protocol messages to stdout.
4. Keep durable mutation `requestId`, complete captured authority and scope independent from JSON-RPC correlation. Preserve the owner’s existing documented semantic fingerprint normalization (including accepted omitted/default equivalents) rather than imposing an incompatible raw-byte cache. Require caller-retained complete intent or reuse the existing durable intent mechanism. Forward once; an interrupted response is unknown, not permission to make a new request. Tests must show exact cross-interface retry and effect-changing-input conflict.
5. Forward wait/observation with the caller context and finite existing limits. Cancellation or host EOF ends observation only; it never sends Stop or shuts down the owner. Mutations already accepted by the owner retain their recorded outcome after client loss. Explicit Stop/pause/resume and reconciliation retain their existing scope/receipt and independent-fence semantics.
6. Expose existing artifact byte pages as exact base64 bytes with offset, next offset, stable captured `through`, EOF/stream closure and integrity when available. A later read must carry the chosen boundary. Do not clip, decode binary as text, synthesize acceptance, invent telemetry or expose private owner tokens.
7. Provide a discoverable host setup guide with real executable paths, spaces/Unicode handling, exact supported versions and local security boundary. Commit only public examples, generated source/assets and applicable license notices, never state, credentials or private evidence.

## Structure and Explicit Ownership

- `internal/ownerclient/` and selected `internal/cli/` seams: fixed-route owner attachment, compatibility and safe transport. One owner serializes changes to shared CLI files.
- `internal/mcphost/`: protocol transport, catalog, strict schema/argument handling, dispatch, results and cancellation.
- `cmd/council/` or narrowly scoped CLI entrypoint: `council mcp` without emitting a UI URL or diagnostic on stdout.
- `internal/mcphost/*_test.go` and test fixtures: real stdio SDK protocol, fake owner/agent, multi-interface and byte-exact scenarios.
- `docs/refactor/`, `README.md`, licensing/notices and release package checks: English setup and honest supported scope.
- `specs/008-host-integration/`: this feature's specification, design, contract, tasks, review and acceptance.

Explicit ownership is assigned in tasks; shared entrypoint and owner/CLI files are serialized. No automatic branching or live vendor calls.

## Acceptance and Release Gate

Independent review must trace every exposed tool to a real accepted operation, exact request/response/limit and negative tests, including older-owner Stop, authorization, protocol version mismatch, malformed/duplicate requests, owner startup, transport loss, multiple clients, byte reconstruction, exact replay, cancellation versus Stop, graph/recovery and current incomplete-history limits. The full local developer commands and exact-source native/race CI must pass; skipped platforms or limitations are recorded, not inferred. The separate full-retention and remaining harness work remain mandatory before release or replacement of `main`.


### Observed CI metadata exception

The original gate expected each hosted job to expose an individual completed successful conclusion. Exact-source run 42454538785 reports completed/success overall, and Darwin AMD64 exposes 15 completed successful steps and a corroborating complete log, but its individual job/check conclusion is absent and its status remains in_progress. Independent review accepted this narrow combination as equivalent evidence that the native checks executed successfully, while recording the metadata discrepancy without claiming an individual job conclusion. Feature 012 must recheck that metadata as part of final release qualification. All other job conclusions are individually successful; the Linux namespace test skip and the unexplained prior shared-account fixture failure remain explicit limitations.
