# MCP Host Integration: Research and Decisions

**Status:** design research for feature 008. The implementation, supported host
matrix and release claims require the feature plan, independent review and actual
offline qualification. Full evidence export, backup/restore and reviewed cleanup
remain separate mandatory pre-release work; host access to incomplete metadata
history does not satisfy them.

## Protocol implementation and compatibility

**Decision:** use the official [MCP Go SDK v1.8.0](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0)
as the protocol implementation for Council's native binary and serve local MCP
over stdio. The module is pinned; the implemented host is accepted for the
offline scope and limitations recorded in acceptance.md. Stdio requires newline-delimited UTF-8 JSON-RPC on stdin and
stdout. Reserve stdout exclusively for protocol messages and send bounded,
non-sensitive diagnostics to stderr. The client-launched host process is a
connection to the continuing Council owner, not another owner or scheduler.
[MCP stdio transport](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)
and the SDK's [server example](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/quick_start.md)
describe these boundaries.

Alternatives considered:

- A handwritten stdio/JSON-RPC implementation avoids a library but assumes
  responsibility for evolving lifecycle, framing, negotiation, errors,
  cancellation, malformed input and concurrency. That creates a disproportionate
  interoperability and maintenance burden; reject it as the default.
- A Node MCP server adds an installed runtime or a separate distribution and
  dependency lifecycle to the native package. It conflicts with the existing
  standalone Go release direction and runtime npm prohibition. Node-based
  development clients can still be used for offline compatibility tests; they
  are not a production runtime dependency.
- Streamable HTTP adds a separately exposed MCP network interface and its own
  origin, authentication and session requirements. Council already uses an
  authenticated private loopback owner API internally; stdio needs no new public
  listener. Remote/multi-user hosting and direct MCP HTTP are outside this slice
  unless separately specified.

The [v1.8.0 compatibility table](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/README.md)
lists protocol versions 2026-07-28, 2025-11-25, 2025-06-18, 2025-03-26 and
2024-11-05. SDK support is not proof of Council or host compatibility. Recommend
initially advertising only **2025-11-25 and 2025-06-18**, using the SDK's
`ServerOptions.SupportedProtocolVersions` to narrow its defaults, and accepting
each only after a real client test. Qualify 2026-07-28 and older versions
separately rather than advertising untested behavior. Unsupported combinations
must receive an actionable protocol-compatible failure. The 2025 lifecycle uses
`initialize`, version/capability negotiation and `notifications/initialized`;
the 2026 protocol introduces discovery and per-request metadata, so it cannot be
assumed identical. See the [2025 lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle)
and [SDK protocol documentation](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/protocol.md).

## Bounded input and lossless schemas

**Decision:** enforce a bounded wire frame and the existing, independently
bounded Council operation inputs. SDK v1.8.0 defaults to a **16 MiB** inbound
frame; Council permits general request bodies up to **32 MiB**, text up to
**4 MiB**, and graph declarations up to **2 MiB**. A naive SDK default can
reject otherwise valid requests. The plan should set a deliberate **64 MiB**
maximum inbound stdio frame, including the JSON-RPC envelope, and retain the
existing operation-specific body/text/graph limits after extraction. This
allows normal compact encoding of a 32 MiB argument object and framing while
remaining bounded. Excessive wire whitespace or alternate escaping can still
exceed the frame limit; document and test that explicit failure. Never truncate
text, authority or evidence to make it fit. Qualify peak allocation and exact
limit behavior, including escaped and multibyte input, before accepting the
number. SDK transport details are in its [v1.8.0 source](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/mcp/transport.go).

Publish a documented, finite tool catalog with explicit object input schemas,
required fields, `additionalProperties: false`, value and array limits, and
operation descriptions. The [MCP tool specification](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)
defaults unspecified JSON Schema versions to 2020-12 and requires a valid object
input schema. Do not rely on inferred Go defaults to capture authority or treat
omitted fields as consent. Council IDs and revisions stay canonical strings,
including values above JavaScript's exact integer range. Effects require a
client-supplied or previously retained Council request ID, distinct from the
JSON-RPC correlation ID; the complete immutable operation, target, body and
captured authority must accompany an exact retry after reconnect. An owner-fingerprint-inequivalent, effect-changing
request under that ID conflicts before an effect; documented omitted/default
equivalents replay the same receipt. Do not regenerate an ID,
refresh a revision or capture a broader current scope to recover a lost reply.

The generic SDK `mcp.AddTool` validates a schema and applies defaults before
decoding its typed input. For authority-sensitive requests, prefer the basic
`Server.AddTool` path with its original `req.Params.Arguments` as `json.RawMessage`,
explicit schema validation and Council's strict raw decoder; alternatively use
the generic path only if tests prove untouched raw arguments reach the strict
validator before defaults can alter meaning. The basic path makes argument and
output validation the implementer's responsibility. In either case test missing,
unknown, duplicate and null keys, malformed Unicode and nested records before
any owner call. In particular, investigate whether the SDK's upstream JSON-RPC
decoding rejects duplicate **outer envelope** members: strict validation of the
raw `arguments` alone cannot recover duplicates already collapsed by a parser.
If necessary, validate bounded frames before SDK decoding with an `IOTransport`
adapter that preserves stdio behavior. Do not claim this protection until a real
malformed-wire test passes. See the [SDK handler implementation](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/mcp/server.go).

## Results, evidence and cancellation

**Decision:** map Council's existing envelope to a stable object-valued tool
result containing operation identity, status, data, receipt and bounded error
as applicable. For compatibility with 2025 clients, structured results are JSON
objects and are accompanied by a serialized text content block. Newer protocol
versions allow more general structured values, but that does not change the
declared cross-version envelope. Unknown tool or malformed JSON-RPC is a protocol
error; input, owner and business failures are tool results with `isError: true`.
SDK ordinary handler errors may be exposed as text: return only Council's safe
bounded diagnostic, never raw internal errors, private owner tokens or vendor
authentication. A successful transport result or receipt does not establish a
successful task, accepted response, verified cleanup or published change. See
[MCP tools and error handling](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)
and the [SDK server documentation](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/server.md).

Routine lists and status are bounded and observational. Full retained artifacts
need explicit bounded byte reads with exact offset, next offset, captured end
boundary and completion state; use a binary-safe representation such as base64
in the typed result, with byte lengths and integrity metadata from the existing
owner. A multibyte UTF-8 boundary is a byte boundary, not permission to replace
or clip bytes. A growing artifact must keep one captured end boundary for the
entire read; a later observation is a new read. Respect existing provenance,
project scope and artifact access checks; metadata-history's incomplete inventory
is not a complete evidence export. Test exact reconstruction and hash equality,
including empty, binary, large and growing fixtures.

MCP request cancellation and stdio disconnect terminate the observation or
transport session; they do not implicitly invoke Council Stop or close the
continuing owner. An admitted task remains owned. A mutation whose acknowledgement
was interrupted remains uncertain and must be inspected or retried using the
same durable request ID and input. The SDK maps cancellation into handler contexts;
the receiver should stop processing that RPC and need not send a cancellation
response. An explicit task/group Stop remains a separate authority-checked
operation. See [MCP cancellation](https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/cancellation)
and [SDK cancellation](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/protocol.md).

## Existing owner seam

`internal/app/Service.Execute` already dispatches shared workspace, execution,
routing, workflow, graph, recovery and history operations. `internal/owner.Ensure`
and the private loopback owner in `internal/cli/owner.go` enforce one persistent
owner. At design time, CLI-specific forwarding, descriptor/version checks and exact
intent retention lived in `internal/cli/cli.go`, `metadata.go` and
`graph_intents.go`; they were not yet a host client API. The plan introduced a narrow,
shared internal owner-client seam rather than opening a second Store/Engine,
calling UI endpoints from a new scheduler, or spawning a CLI process per tool.
It must preserve authenticated loopback, exact owner capability checks, stale
owner refusal, durable receipts and the known request/acknowledgement boundary.
Use caller-supplied captured authority, not implicit recapture on retry. A second
host and the UI/CLI must observe one owner and shared capacity/fences. The
descriptor's access token remains private process state, never returned as a tool
result or written to host configuration.

## Dependencies and distribution

The [v1.8.0 `go.mod`](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/go.mod)
declares Go 1.25.0; Council declares Go 1.27.3, so the stated language minimum is
compatible. On 2026-11-08, Council pinned this exact SDK version and its selected
transitive checksums. Go 1.27.3 on Windows ARM64 successfully built the SDK's
`mcp` package and `go mod verify` verified the module cache. Dependency listing
for that package on Windows, macOS and Linux, each on AMD64 and ARM64, selected
the following module union; this is dependency selection, not six native builds
or a claim that the unimplemented host is qualified:

| Module | Selected version | Upstream license distribution |
| --- | --- | --- |
| `github.com/modelcontextprotocol/go-sdk` | `v1.8.0` | MIT / Apache-2.0 transition described below |
| `github.com/google/jsonschema-go` | `v0.4.3` | MIT |
| `github.com/segmentio/asm` | `v1.1.3` | MIT |
| `github.com/segmentio/encoding` | `v0.5.4` | MIT |
| `github.com/yosida95/uritemplate/v3` | `v3.0.2` | BSD-3-Clause |
| `golang.org/x/oauth2` | `v0.35.0` | BSD-3-Clause |
| `golang.org/x/sync` | `v0.24.0` | BSD-3-Clause and patent notice |
| `golang.org/x/sys` | `v0.49.0` | BSD-3-Clause and patent notice |
| `golang.org/x/time` | `v0.15.0` | BSD-3-Clause and patent notice |

Minimal version selection retains Council's existing `x/sys` version and
selects `x/sync` v0.24.0, newer than the SDK's v0.20.0 requirement. The package
notice generator records the exact upstream license and applicable patent texts
from these pinned public module distributions in `third-party/go-NOTICES.txt`.
Re-audit the actual linked dependency graph, security advisories, reproducible
offline builds and release packaging after host integration. Council has no
runtime npm dependencies; the Go SDK does not change that fact.

The SDK's [tagged LICENSE](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/LICENSE)
describes an ongoing transition from MIT to Apache-2.0, with some contributions
remaining MIT and documentation under CC-BY-4.0. Do not label the whole upstream
source unconditionally as one license. Review the selected source, transitive
licenses and required notices; verify the actual release package includes them
without copying unrelated files, user state or credentials. This inventory is
not a legal conclusion.

The SDK's default server options also advertise logging and, when tools are
inferred, tool list-change notifications. Supply explicit capabilities for the
implemented surface and verify the initialize response rather than relying on
those defaults. The SDK exposes `StdioTransport.MaxLineLength`,
`ServerOptions.SupportedProtocolVersions`, `InitializedHandler`, and the raw
`Server.AddTool` handler needed for the design; implementation and wire tests
must still establish their behavior in Council.

## Required offline evidence

1. Connect an actual independent stdio protocol client to a freshly built native
   binary, including paths with spaces and Unicode. Assert supported and
   unsupported negotiation, `tools/list` schemas and calls, valid framing and
   protocol-only stdout from startup through errors and shutdown.
2. Create a fake task in each UI/CLI/MCP direction; observe/control it through
   the others with the same identities, receipts, outcomes and one owner. Exercise
   each accepted operation family and two simultaneous host connections; verify
   capacity, independent failures, graph and recovery fences and no extra model
   work during observation.
3. Inject duplicate/unknown/malformed/oversized frames, invalid schema inputs,
   mismatched or stale owners, lost responses, exact replay, owner-equivalent omitted/default values and effect-changing
   request conflicts. Confirm no side effect for invalid input and no implicit authority
   refresh, including unusual escaping and size limits.
4. Interrupt wait, request and host connection while controlled fake work is
   active; verify no implicit Stop. Separately exercise explicit Stop, pause,
   checkpoint/resume, cleanup uncertainty and preserved failure precedence.
5. Reconstruct long, binary, multibyte and growing artifact bytes across pages
   using one captured boundary, and compare the complete digest with the owner.
   Verify no silent truncation, duplication or evidence substitution.
6. Run required build, tests and checks plus actual native/platform jobs; review
   packaging and offline fake/live boundaries separately. A fake MCP client or
   SDK conformance test is not live provider qualification or proof that every
   external host/version works.
