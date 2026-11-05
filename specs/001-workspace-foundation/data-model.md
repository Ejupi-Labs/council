# Data model: Workspace foundation

**Feature**: 001-workspace-foundation  
**Branch**: `refactor`  
**Contract version**: API 1; application `2.0.0-dev`  
**Status**: Design; not implementation or live-provider qualification evidence

This model implements the preparation journey in [spec.md](spec.md). It introduces no task
execution, agent attempt, verifier receipt, integration record or provider account store. The
wire contract is [contracts/foundation.md](contracts/foundation.md).

## Representation rules

| Value | Representation and invariant |
| --- | --- |
| Entity and request identity | Lowercase, cryptographically random 128-bit identity encoded as exactly 32 hexadecimal characters. Identity is opaque; a path, PID or timestamp is not an entity identity. |
| Revision | Canonical decimal string matching `[1-9][0-9]*`, bounded by signed SQLite 64-bit maximum `9223372036854775807`. Persist as an integer; never convert through a JavaScript number. Entity revisions start at `1`. Overflow rejects the mutation before commit. |
| Timestamp | UTC RFC 3339 string with millisecond precision and `Z`, for example `2026-11-05T20:00:00.000Z`. Use the owner's clock. A timestamp is not a concurrency token. |
| Optional fact | The declared `null`, omitted optional field or explicit `unknown` state; never an invented empty identifier, zero usage, idle account or authenticated connection. |
| User text | A valid Unicode scalar sequence, preserved exactly. No normalization, trimming, newline conversion, BOM removal or replacement of malformed input. Reject malformed UTF-8 and escaped unpaired UTF-16 surrogates. A valid literal U+FFFD is ordinary preserved text. |

JSON uses camelCase. Inputs reject unknown fields, duplicate object keys, wrong types, invalid
enums and noncanonical IDs/revisions. The raw original task is limited to 4 MiB of UTF-8 after
JSON unescaping. The whole HTTP request body is independently limited to 32 MiB so escaping
does not silently reduce that text allowance. Empty or entirely Unicode-whitespace task text
is invalid; validation may inspect whitespace without changing the retained string.

The pinned Go 1.27.3 implementation uses stable `encoding/json/jsontext` strict validity and
`encoding/json/v2` with `RejectUnknownMembers(true)`, preserving the strict defaults rather
than enabling malformed-Unicode or duplicate-name compatibility options. The required input
tests cover the resulting behavior; a custom Unicode scanner is not part of this design.

## Project

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | Identity | Durable project identity. |
| `selectedPath` | String | Exact path supplied for the admitted selection, including its original spelling. |
| `resolvedPath` | Absolute path string | Canonical, symlink-resolved selected working folder; a repository subdirectory remains that subdirectory. |
| `repositoryRoot` | Absolute path string | Git worktree root, resolved separately from the selected folder. |
| `branch` | String or `null` | Observed branch; `null` for detached HEAD. An unborn branch can have a name and a null commit. |
| `commit` | String or `null` | Observed complete Git object ID; `null` for an unborn repository. Do not assume SHA-1 length. |
| `createdAt` | Timestamp | Initial admission time. |
| `updatedAt` | Timestamp | Initial admission time in this slice; project metadata is not silently rewritten by inspection. |
| `observedAt` | Timestamp | Time of the retained admission repository-context observation. It is not proof that the branch is unchanged now. |
| `revision` | Revision | Concurrency version of this project record. |

A project must be an accessible existing local folder in a non-bare Git worktree. Git metadata
inspection is read-only and shell-free. Use `rev-parse`, `symbolic-ref` and `rev-parse HEAD`, with
`--no-optional-locks`, `-c core.fsmonitor=false` and a sanitized Git environment; no Git status
scan is needed. It must not refresh the index, stage, switch branches,
stash, reset, write source files or execute repository hooks. Before admission, validate that
the path can be represented losslessly on the platform and that the folder/repository context
can be inspected. An invalid path creates neither a project nor a successful receipt.

Resolve relative input against the explicit absolute `basePath` carried by the caller; do not
use the service process's current directory or expand `~`, environment variables or shell
syntax. The CLI supplies its actual working directory as `basePath`. Preserve the exact
`selectedPath` string separately from canonical resolution.

Deduplicate by current filesystem identity (`os.SameFile`/platform equivalent) as well as
canonical paths. Re-stat the target and candidate records during admission. Do not deduplicate
by Windows case folding: case-distinct physical directories must remain distinct. Paths are
not durable physical IDs; if a previously retained location has been replaced with a different
directory, report the changed identity rather than silently attaching its drafts to the
replacement. Retain internal platform file-identity evidence for this check where supported.

Open is get-or-create. Opening an existing physical project returns that unchanged record and
does not advance its revision or replace its original selectedPath. An `openSelection` in the
result records this request's exact selectedPath and basePath. Open/get also return a fresh
`ProjectObservation` with resolvedPath, repositoryRoot, branch, commit and observedAt. This is
read-only evidence, separate from the retained admission fields. Observation failure is reported
without rewriting or deleting the retained project. Lists return compact admission metadata;
details label its observation time rather than presenting stored branch/commit as live state.

## Autonomy preference

One owner-wide policy is sufficient for feature 001. There is no project override hierarchy or
settings taxonomy in this slice.

| Field | Type | Meaning |
| --- | --- | --- |
| `mode` | `autonomous` or `supervised` | Requested operating preference. Autonomous is displayed as “Autonomous full access”; supervised as “Supervised”. |
| `selectionSource` | `default` or `user` | Distinguishes the fresh-install default from an explicit choice. |
| `revision` | Revision | Policy revision. |
| `updatedAt` | Timestamp | Durable time of this revision. |

The initialized preference is autonomous, source default, revision `1`. The UI visibly presents
that requested choice; it does not claim verified effective capability. Setting either mode explicitly records source user and advances
the revision, including when the mode is unchanged. A set requires `expectedRevision`.

This preference grants no verified effective harness capability and does not authorize unrelated
publication or destructive work. Feature 002 resolves actual execution policy at admission.

## Connection observations

Exactly five canonical harness identifiers are supported by discovery:

| `harnessId` | Display name |
| --- | --- |
| `codex` | Codex |
| `claude-code` | Claude Code |
| `deepseek-harness` | DeepSeek Harness |
| `junie` | Junie |
| `antigravity` | Antigravity |

A discovery result has `observedAt`, `source` and exactly five `observations`, one for each
harness in the order above. Discovery and refresh are the same read-only operation: obtain
current file/installation metadata without writing snapshots, revisions or mutation receipts to
the database. While a request is pending the UI says “Inspecting installations”; before it has
any result it says “Not inspected”, rather than inventing missing installations.

Each `ConnectionObservation` contains:

| Field | Type | Meaning |
| --- | --- | --- |
| `harnessId` | Canonical harness ID | Harness identity only, not provider/model/account identity. |
| `installationStatus` | `installed`, `missing` or `unsupported_launcher` | A discovered executable candidate, no candidate, or detected installation whose available launcher requires unsupported shell handling. |
| `launcherPath` | Optional absolute string | Candidate path when found; omitted when missing. Finding it does not run it. |
| `source` | `path_and_native_install_metadata` | Discovery uses PATH entries and documented platform-native installation metadata only. |
| `observedAt` | Timestamp | Local discovery time. |
| `managedExecution` | `not_implemented` | Honest adapter limitation for every harness in feature 001. |
| `authentication`, `provider`, `account`, `model`, `effectivePermissions`, `externalOccupancy`, `allowance`, `usage` | Literal `unknown` for each field | No verified evidence is acquired by this feature. Unknown usage is not zero usage. |
| `nextAction` | `install_with_vendor`, `review_launcher_support` or `await_managed_execution` | Fixed, English setup guidance chosen from the observed installation state. No automatic install, login or live probe follows. |

`installed` is installation evidence only. Do not invoke a vendor binary even for `--version`,
help, authentication or model inspection. Do not open vendor authentication files or infer a
provider/account from a product name. PATH search examines file metadata, not file contents.
Metadata failures are isolated per harness; where inspection cannot establish any of the three
installation states, return a per-harness `discovery_unavailable` error alongside the successfully
observed harnesses rather than storing a false missing observation. The result still has exactly
five entries, each containing either an observation or that bounded error. A UI may retain its
last known result labelled with its old observation time; it must not present that result as new.

## Task draft and immutable versions

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | Identity | Stable draft identity across edits. |
| `projectId` | Project identity | Required, immutable foreign key. |
| `originalText` | String | Complete user-authored text at this saved revision. Never a preview or generated summary. |
| `revision` | Revision | Monotonically increasing saved version; starts at `1`. |
| `status` | Literal `draft` | UI displays “Draft / Not run”. |
| `selection` | Literal `auto` | Requested default; no harness/model/account selection has occurred. |
| `requestedPolicy` | Policy snapshot | Exact mode, selectionSource, revision and updatedAt of the selected global preference. |
| `createdAt` | Timestamp | First saved draft time; unchanged by edits. |
| `updatedAt` | Timestamp | Time this version was saved. |

Create snapshots the current global preference in the same writer transaction as the draft.
Changing the global policy never rewrites a draft snapshot. Update requires `expectedRevision`,
preserves projectId/status/selection, and stores a complete new immutable draft version. It keeps
the prior requestedPolicy unless the caller explicitly requests `useCurrentPolicy: true`, in
which case the current preference is captured atomically. There is no arbitrary policy object
accepted from a client. An unchanged text save still creates a new revision if accepted.

Retain every saved version, including originalText and its policy snapshot. The draft's current
pointer advances only with its new version and receipt in one transaction. Historical retrieval
is explicit by revision. Earlier saved text is never silently deleted by a later edit, and a
global preference change cannot retroactively imply that an earlier draft ran with another mode.

Draft list rows are compact metadata: id, projectId, revision, status, selection, requestedPolicy,
createdAt and updatedAt. They contain neither full text nor an unlabelled text substitute.

## Mutation receipt and concurrency

Each admitted mutation carries a caller-generated `requestId`. Persist an operation namespace,
validated input fingerprint, HTTP-equivalent result/status, receipt and complete response data in
the same transaction as its changes. The receipt has requestId, operation and committedAt; it is
a durable saved-operation acknowledgement, never an execution/check receipt.

- The request identity is global within this v2 store. Same requestId, same operation and same
  validated input returns the original status, data and receipt, even after later edits.
- Same identity with a different operation or input returns `request_conflict`, without a write.
  Fingerprinting includes expectedRevision, originalText, original selectedPath/basePath and all
  explicit options; JSON whitespace or property order is immaterial.
- Check the stored request before evaluating an already-committed retry's expectedRevision. A
  lost acknowledgement must not turn an accepted mutation into a duplicate or stale-write error.
- For a new user-authored update, compare expectedRevision inside the writer transaction. A
  mismatch returns `revision_conflict` and the current revision, without changing any content.
- Reject malformed input before admission. A rejected, uncommitted request has no successful
  receipt. A later correction uses a fresh request identity.
- Commit failure or uncertain acknowledgement is never “Saved”. Preserve the request identity;
  retrying that identity reconciles the transaction. Do not claim that no change occurred when
  commit status is uncertain.

Receipts are retained for the lifetime of this feature's records; feature 001 has no pruning,
deletion or implicit request-identity expiry. Internal receipt rows are private because their
replay data may include original task text.

## Local owner and persistence boundary

The private state root is `<user-home>/.council/v2/`, containing SQLite state (including its
journal/WAL companions), the native ownership lock and an ephemeral owner connection descriptor.
Create/verify private directory permissions before opening the database or writing a bearer
token: Unix mode 0700 with 0600 files, and a Windows DACL restricted to the current owner, SYSTEM
and Administrators. Do not place the new store under the selected repository or read legacy
`.council/` data.

The chosen native Go core uses modernc.org/sqlite with one serialized writer. An OS-held lifetime
lock precedes every database open, including transient CLI reads. A PID, descriptor file or stale
timestamp alone is not lock ownership. The foreground `council ui` process holds the lock while
serving authenticated loopback clients; otherwise one CLI command holds transient ownership.
Clients that find an active owner call that owner rather than opening a second database writer.
If neither authenticated contact nor safe lock acquisition succeeds, return an owner error.

Use foreign keys, transactions and exact 64-bit write metadata. Enforce stored integer revision
types and bounds so SQLite arithmetic cannot promote an overflowed revision to REAL. Schema compatibility is checked
before use; an unsupported schema is an error, not an instruction to recreate state. New-store
initialization is transactional. Never open, migrate, delete or package legacy collaboration data.
No owner takeover kills a process, removes unrelated state or changes project files.

The owner descriptor contains a unique owner instance ID, PID, start time, exact loopback origin,
API version and a cryptographically random bearer token. It is runtime coordination data, not a
product configuration file. Only the owning process removes its matching descriptor during
orderly shutdown. Ownership loss closes the database; no second writer is admitted on a PID guess.

All local state, private drafts, receipt replay bodies, machine paths, tokens, fixture databases,
vendor credentials and legacy state are excluded from Git, npm contents and release archives.
Shipped binaries embed only authored UI and approved public brand assets/notices.

## Relationships and permitted transitions

`Project 1 -> many Draft`; `Draft 1 -> many immutable DraftVersion`; the current policy is a
singleton, with its values copied into each draft version. Receipt identity is independent of
entity identity. Connection observations reference only canonical harness IDs, not account records.

Permitted foundation transitions are project admission, policy revision, draft creation and draft
revision. Reopen, project inspection and installation refresh are read-only observations. Draft remains draft. Starting work, cancellation,
checks, local integration, commits, external publication and provider qualification are outside
this model and must not acquire simulated records here.
