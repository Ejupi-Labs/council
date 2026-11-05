# Foundation API and CLI contract

**Feature**: 001-workspace-foundation  
**Branch**: `refactor`  
**API**: 1, rooted at `/api/v1`  
**Application**: `2.0.0-dev`  
**Status**: Implementable design; no implementation or live-agent qualification is claimed

The local UI and CLI call the same application services and return the same durable facts and
errors. [The data model](../data-model.md) defines entity fields and invariants. This contract
does not start an agent, run a check, create an attempt, integrate a change or publish anything.

## Owner and transport

`council ui` is the foreground, persistent owner. It obtains the native OS-held lifetime lock
before opening `<user-home>/.council/v2/` state, serves the embedded UI on an OS-assigned port
bound only to `127.0.0.1`, and keeps running after an observing browser closes. `--no-open`
suppresses opening a browser; it does not make the service temporary or start a background
daemon. Ctrl+C requests orderly shutdown. A second UI command contacts the existing owner and
opens/reports its UI URL rather than starting a second writer.

Every other stateful CLI command first resolves ownership. With an active compatible owner,
it uses the descriptor's authenticated loopback API. With no active owner, it acquires exclusive
transient ownership, invokes the identical service operation locally, closes the database and
releases the lock. It must never open the database while another owner holds that lock, even for
a read. PID checks and a descriptor's existence are not lock proof. A locked but unreachable,
unverifiable or incompatible owner produces an actionable owner error; no process is killed,
state recreated or competing owner started. A stale descriptor can be replaced only after
exclusive lock acquisition.

The private state directory and descriptor require Unix 0700/0600 permissions or a Windows
DACL restricted to the current owner, SYSTEM and Administrators before database/token writes.
The descriptor records ownerInstanceId, PID,
startedAt, exact origin, apiVersion and a fresh CSPRNG bearer token of at least 256 bits.
An authenticated `GET /meta` verifies the descriptor's instance and API version. Descriptor
format and tokens are internal, not application preferences, and never enter release artifacts.

HTTP security rules:

- Public routes serve only the fixed UI shell and embedded public assets. Every `/api/v1`
  request, including private GETs, requires `Authorization: Bearer <token>`.
- Validate the exact bound numeric loopback Host and port before routing. Do not accept arbitrary
  localhost aliases, forwarded-host headers or non-loopback listeners. Do not follow redirects
  when the CLI contacts an owner.
- Every mutation requires the exact owner Origin; the CLI sends that same loopback Origin.
  Private GETs reject any supplied foreign Origin, including `null`, and may omit Origin. No wildcard
  CORS, cross-origin preflight allowance or cookie-based ambient authentication is provided.
- The launch URL carries the UI bootstrap token in its fragment, never a query. The embedded UI
  consumes it, removes the fragment with history replacement, and retains it only in origin-scoped
  session storage for reloads. No third-party script/assets, analytics or external requests are
  loaded. Use a restrictive same-origin CSP and `Referrer-Policy: no-referrer`.
- The startup/open UI URL is deliberately shown only to the invoking owner. Do not put bearer
  tokens, Authorization values, raw task bodies or vendor state into diagnostics, access logs,
  error messages or public pages. Private API responses use `Cache-Control: no-store`.
- Unknown methods/routes reject without effects; there is no execution endpoint disguised as
  discovery. Requests containing a task can only save or revise a draft in this feature.

## Common encoding and envelopes

Requests and responses are UTF-8 JSON, with `Content-Type: application/json`. Mutating endpoints
accept JSON objects only. Reject invalid raw UTF-8, unpaired escaped UTF-16 surrogates, duplicate
members, unknown members, incorrect types and trailing JSON values. Use Go 1.27.3 stable
`encoding/json/jsontext` strict validity and `encoding/json/v2` with
`RejectUnknownMembers(true)`; do not enable permissive compatibility options. Valid literal
U+FFFD, leading/interior U+FEFF, multiline text, line-ending choices and Unicode spelling survive
unchanged. Paths must also be representable by the target platform without replacement.

An identity is exactly 32 lowercase hexadecimal characters. A revision is a decimal string from
`1` through `9223372036854775807`, with no sign, whitespace, exponent or leading zero. JSON numeric
revisions are invalid. All timestamps use UTC RFC 3339 milliseconds with `Z`.

Every response has `apiVersion: 1` and `requestId`. A read gets a fresh server request identity;
a mutation uses its caller-supplied identity. Success has `data`; failure has `error`, never both.

| Envelope member | Type and rule |
| --- | --- |
| `data` | Operation result specified below. |
| `receipt` | Present only on committed mutations: `{requestId, operation, committedAt}`. |
| `error` | `{code, message}` with optional `currentRevision` only for revision conflicts. |

`error.code` is a documented ASCII enum of at most 64 bytes. `message` is actionable English,
at most 512 UTF-8 bytes, and contains no raw task text, bearer token, arbitrary subprocess output
or credential value. A malformed/missing mutation requestId receives a new diagnostic identity,
not a saved-operation receipt. CLI-local errors use the same bounded envelope in JSON mode.

For example, a stale save is:

```json
{"apiVersion":1,"requestId":"11111111111111111111111111111111","error":{"code":"revision_conflict","message":"This draft has a newer saved version. Reload it before saving.","currentRevision":"3"}}
```

## Mutation admission and replay

`projects.open`, `policy.set`, `drafts.create` and `drafts.update` are the only mutation operations.
Each requires requestId, supplied once per user intent. UI and CLI generate one if the user did
not provide one and retain it for retries. Persist the validated operation/input fingerprint,
receipt and exact success data/status in the same transaction as its changes.

The same requestId and same validated operation/input returns the original HTTP status, data and
receipt, not a new timestamp, refreshed observation or newer resource version. Compare replay
identity before applying a stale-version check. A changed payload or operation with that identity
returns `request_conflict`; use a new request identity for a new intent. JSON formatting/property
order does not alter the fingerprint. Original text/path bytes after valid JSON decoding and all
explicit options, including expectedRevision, do alter it.

New policy/draft updates compare expectedRevision inside the transaction and reject a mismatch
without a write. Filesystem/Git observations are read-only and have no optimistic-write token.
Existing-project open is get-or-create: it does not edit that project, but its admitted request
and receipt are still committed together. New projects/drafts start at revision `1`.

Validation/prerequisite/conflict errors produce no saved receipt. A failed or uncertain commit
must not say “Saved” or “Not saved” without evidence. Return `storage_failed` for a known failure
or `write_uncertain` for an indeterminate outcome; preserve the request identity and advise retry
with that same identity and input. Lost HTTP acknowledgement does not authorize a fresh create.
On replay, clients may explicitly fetch current state after consuming the original receipt.

## HTTP operations

All paths below are relative to `/api/v1`. Undocumented query/body members reject. GET operations
have no request body. Optional query values are individually type/range checked; duplicate query
keys reject. IDs and other path/query strings are decoded once, never recursively.

| Method and route | Input | Success status and `data` |
| --- | --- | --- |
| `GET /meta` | None | 200: `{appVersion, apiVersion, ownerInstanceId, scope: "workspace_foundation", managedExecution: "not_implemented"}`. |
| `POST /projects/open` | `{requestId, selectedPath, basePath}` | 201 for admission; 200 for existing identity: `{project, openSelection, observation}` plus receipt. |
| `GET /projects` | Optional `limit`, `cursor` | 200: `{items: Project[], nextCursor}` using retained admission metadata. |
| `GET /projects/{id}` | None | 200: `{project, observation, observationError}`. |
| `GET /policy` | None | 200: Policy. |
| `PUT /policy` | `{requestId, expectedRevision, mode}` | 200: new Policy plus receipt. |
| `GET /agents` | None | 200: DiscoveryResult; a fresh read-only inspection every time. |
| `POST /drafts` | `{requestId, projectId, originalText}` | 201: complete Draft plus receipt. |
| `GET /drafts` | Optional `projectId`, `limit`, `cursor` | 200: `{items: DraftSummary[], nextCursor}`. |
| `GET /drafts/{id}` | Optional `revision` | 200: complete current Draft, or the requested immutable saved version. |
| `PUT /drafts/{id}` | `{requestId, expectedRevision, originalText, useCurrentPolicy?}` | 200: new complete Draft version plus receipt. |

`basePath` must be absolute even when selectedPath is absolute; it establishes deterministic CLI
retry input. Resolve a relative selectedPath against it without trimming or shell expansion. The
selected folder must be accessible and belong to a supported non-bare Git worktree. Invalid
paths and prerequisites reject before creating a project/receipt. Spaces/Unicode and symlink or
ordinary physical aliases work; no Windows lowercase-path deduplication is permitted.

`openSelection` is `{selectedPath, basePath}` for this request. Project is the durable admission
record defined in the data model. Opening an existing physical identity returns that unchanged
record. A fresh `ProjectObservation` is `{resolvedPath, repositoryRoot, branch, commit, observedAt}`.
The branch and commit are nullable as defined in the data model, and represent this read time,
not an execution destination. No source content, staging or branch changes occur.

Project get retains access to a saved project even if fresh inspection fails. It returns
`observation: null` and a bounded `observationError: {code, message}` in that case; on success the
error is null. A missing/moved/inaccessible path or changed physical identity cannot be silently
treated as the original project. Opening such a changed identity returns a prerequisite/conflict
error; it does not attach earlier drafts to a replacement folder.

Policy has mode, selectionSource, revision and updatedAt. The fresh default is autonomous/default;
an accepted set marks selectionSource user and advances revision. Neither value proves effective
harness permissions. No client-supplied extra policy fields are accepted.

DiscoveryResult has observedAt, source `path_and_native_install_metadata`, and exactly five
entries in `observations`, ordered `codex`, `claude-code`, `deepseek-harness`, `junie`,
`antigravity`. Each entry is `{harnessId, observation, error}` with exactly one non-null result.
Observation uses the fields in the data model. A per-harness error is the bounded
`{code: "discovery_unavailable", message}`; other harnesses remain inspectable. A global scan
failure returns the normal error envelope. Both agents list and refresh invoke this GET; no
snapshot history, revision or mutation receipt is created. No vendor executable is invoked,
including for version/help/authentication/model discovery, and no credentials are inspected.

Draft creation requires an existing projectId and complete originalText; it atomically snapshots
the global requested policy. Text must contain at least one non-whitespace Unicode scalar and
at most 4 MiB of UTF-8. Preserve all leading/trailing characters and line endings. The entire
HTTP body is capped at 32 MiB independently; both caps reject rather than truncate.

Draft update replaces the current user-authored draft text by storing a new immutable version;
it never discards earlier versions. `useCurrentPolicy` is optional boolean, default false. True
captures current global policy atomically; false keeps the previous draft policy snapshot.
ProjectId is immutable. Status remains draft, selection remains auto, and no execution is implied.
DraftSummary omits originalText and includes all other Draft fields. Complete text is fetched
explicitly by get; no summary is an authoritative substitute for it.

### Bounded lists

`limit` is an integer from 1 through 100, default 25. `cursor` is an opaque string, at most
2048 ASCII bytes; null nextCursor means the page is last. Reject malformed cursors, numeric
overflow and a cursor reused with a different project filter. Clients must not parse cursors or
convert embedded sequence/revision values to JavaScript numbers.

Lists order by an immutable internal admission sequence ascending. The first page fixes an
inclusive maximum sequence; subsequent pages retain that bound and resume strictly after the
last returned sequence. Thus new admissions cannot move rows between pages or cause skips.
Membership is stable, while metadata can reflect a later draft revision; no point-in-time text
snapshot is claimed. Sequence arithmetic and cursor decoding retain exact signed 64-bit bounds.
Feature 001 has no deletion. Empty pages return an empty items array and null nextCursor.

## Status and error mapping

| HTTP | `error.code` | Meaning and next action |
| --- | --- | --- |
| 400 | `invalid_request`, `invalid_encoding`, `invalid_cursor` | Correct the indicated input; no mutation admitted. |
| 401 | `unauthenticated` | Obtain access from this local owner's UI/CLI launch path; disclose no private resource facts. |
| 403 | `invalid_host`, `forbidden_origin` | Use the exact loopback owner origin. |
| 404 | `not_found` | Project/draft/version/route does not exist; no fabricated draft or agent result. |
| 405 | `method_not_allowed` | Use the documented operation; include Allow for a known route. |
| 409 | `revision_conflict`, `request_conflict`, `project_identity_changed`, `revision_exhausted` | Reload/reconcile, use a new request identity for a new intent, or resolve the named identity/limit issue; preserve current state. |
| 413 | `input_too_large` | Reduce the task/body explicitly; no truncation or saved acknowledgement. |
| 415 | `unsupported_media_type` | Send application/json with UTF-8. |
| 422 | `project_unavailable`, `not_git_project`, `prerequisite_missing`, `discovery_unavailable` | Correct the path/prerequisite or use vendor-owned setup instructions. No agent is started. |
| 503 | `owner_unavailable`, `storage_failed`, `write_uncertain` | Reconnect/reconcile ownership or storage; preserve a mutation's requestId when retrying. |
| 500 | `internal_error` | Report the bounded request identity; never expose a raw stack/task body. |

Authenticate and validate Host/Origin before disclosing private resource existence. Errors never
carry receipt. A 200 discovery/project read may contain the explicitly labelled partial
observation errors above; it is not a successful installation or live-connection qualification.

## CLI parity

The replacement entrypoint is `council`. A release needs no compiler, npm installation or runtime
development dependencies. Command arguments are parsed without an intervening shell. Human
output is concise English; `--json` emits exactly one success/error envelope to stdout with one
trailing newline. Diagnostics go to stderr and never contain task text or secrets.

| Command | Service operation and options |
| --- | --- |
| `council ui [--no-open] [--json]` | Acquire/reuse persistent foreground owner; open/report the private bootstrap URL. JSON startup result contains apiVersion, requestId, and data `{url, ownerInstanceId}` once; subsequent diagnostics use stderr. |
| `council projects open PATH [--request-id ID] [--json]` | projects.open, preserving PATH and supplying the command's absolute working directory as basePath. |
| `council projects list [--limit N] [--cursor CURSOR] [--json]` | GET projects. |
| `council projects get ID [--json]` | GET project and fresh repository observation. |
| `council agents list [--json]` | GET agents, fresh metadata inspection. |
| `council agents refresh [--json]` | Same read-only GET agents; a user-facing refresh alias. |
| `council policy get [--json]` | GET policy. |
| `council policy set autonomous\|supervised --expected-revision R [--request-id ID] [--json]` | policy.set. |
| `council drafts create --project ID (--task TEXT\|--task-file PATH) [--request-id ID] [--json]` | drafts.create. |
| `council drafts list [--project ID] [--limit N] [--cursor CURSOR] [--json]` | GET draft summaries. |
| `council drafts get ID [--revision R] [--json]` | GET complete saved draft/version. |
| `council drafts update ID --expected-revision R (--task TEXT\|--task-file PATH) [--use-current-policy] [--request-id ID] [--json]` | drafts.update; absent --use-current-policy means false. |
| `council --version [--json]` | Local version output only; no owner/database/model access. JSON data has appVersion and apiVersion. |

All foundation commands accept `--api-version 1`. Any other requested version fails locally
before state access. `--help` documents only supported operations and the private data root.
Unknown/missing/duplicate flags reject. `--task` and `--task-file` are mutually exclusive and one
is required. A task file is read as strict UTF-8, with no BOM stripping, line conversion or silent
replacement; `--task-file -` explicitly reads stdin. File/argument text passes the same size,
Unicode and required-content validation as HTTP. Source filenames are not sent or persisted.

A mutation without --request-id generates one before dispatch. On uncertain transport/write
outcome, JSON/human errors show that identity and the instruction to retry the same command with
--request-id; do not silently retry with a new identity. Same receipt/output semantics hold in
transient-owner mode. Expected revision is mandatory on policy/draft updates; the CLI does not
fetch a newer revision behind the caller's back and overwrite it.

| Exit code | Meaning |
| --- | --- |
| 0 | Requested operation/read completed; discovery observation errors remain explicit in its data. This never means an agent task ran. |
| 1 | Internal/storage failure, including uncertain persistence; inspect code/requestId. |
| 2 | Invalid command, input, encoding, size, cursor or requested API version. |
| 3 | Missing project/draft/version, unavailable path or required local prerequisite. |
| 4 | Revision/request/physical-identity conflict or exhausted revision. |
| 5 | Owner cannot be safely acquired/contacted, or local API access is refused. |
| 130 | Foreground UI interrupted by Ctrl+C after shutdown; no draft execution/cancellation claim. |

## Client behavior and verification boundary

The UI retains selected project and unsaved input across unrelated discovery/read errors. It
applies a response only when its query identity, selected resource and edit generation remain
current. A late read cannot overwrite typed text, a newer selection or an acknowledged save.
Conflicts show the retained local input and offer explicit reload/reconciliation; failure does
not clear the composer. A pending mutation reuses its request identity until its outcome is
resolved. Cards say “Draft / Not run”, Auto, the saved requested policy and “Task execution is unavailable in this preview”, without suggesting that an installation card is a working adapter.

Contract acceptance must cover CLI/UI cross-reading after restart; alias equality and distinct
physical folders; unchanged source/staging/branch; policy snapshot/version fidelity; stale update
and lost-ack replay; same-ID different-input conflict; known/uncertain write failures; active-owner
exclusion; per-harness missing/unsupported/discovery-error fixtures; no vendor invocation or
credential access; malformed UTF-8, lone surrogates, duplicate/unknown members, valid U+FFFD/BOM,
4 MiB text and 32 MiB body limits; exact large revisions/cursors; unauthorized GET/mutations and
Host/Origin attacks. Use only deterministic fixtures/fake installations. No acceptance result is
claimed until recorded in the feature's later acceptance evidence.

Private v2/legacy state, tokens, task text, machine paths, fixture databases and vendor state must
remain absent from Git/npm/release contents. No migrate, delete, login, install, agent start,
task start, check, merge or publish command is added by this contract.
