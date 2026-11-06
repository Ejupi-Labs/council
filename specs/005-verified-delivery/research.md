# Research: verified task delivery

**Read date**: 2026-11-06. **Scope**: design research, not implementation or live qualification.

Context7 was consulted first for `/openai/codex`, `/websites/code_claude` and
`/nodejs/node`, followed by the primary pages linked below. Current documentation
is dated evidence, not an observed installed version. No vendor executable,
credential file, login/version command, provider request or local test was used.
The accepted 004 baseline is source `3f4a18e11de495f7b3138d61e030525a018868a5`,
seven native/race jobs in run 42172669381, acceptance commit `5671fac`; that evidence
does not qualify the proposed 005 profiles.

## 1. Reuse native execution and make roles explicit

**Decision**: retain Go 1.27.3, existing SQLite/evidence/process ownership and the
two native transports. Author, review and correction are distinct model Attempts;
each consumes one captured model-admission unit. Checks, local application and
external delivery use executorKind `local_check`, `local_application` and
`local_external`, with their own artifacts, global owned slot and common deadline,
consuming zero model units. A local
application has no child process; its delivery journal, rather than a fictitious
process exit, proves effects. Never append new evidence to a settled author.

Fresh installations may use four admissions for author/review/correction/review.
Existing saved finite defaults and task budgets retain their meaning. There is no
reliable persisted distinction between an untouched old default of two and an
explicitly selected two; do not infer permission to increase either. A valid
blocking finding or failed required check can consume prospectively captured,
finite correction authority. Protocol, execution, ownership and persistence
errors cannot create that authority.

**Rationale**: this separates task-stage progress from model accounting and keeps
004's owner, receipts and no-replay rules meaningful. A new SDK, app-server
transport, paid API executor, session-resume path or dependency is unnecessary.
Native internal calls/retries remain outside the managed admission count.

## 2. Review the actual candidate in a fresh context

**Decision**: materialize a disposable review workspace from the selected immutable
candidate revision, including its actual changed bytes and required base/index
evidence. Supply original instructions, current conditions and linked check
receipts in full or as explicitly accessible retained evidence. Record a manifest
of what Council provided, its digest, paths and candidate identity. A reference
to an artifact in an inaccessible private directory is not access.

Start a fresh review-role process/context without resume, fork or author-session
reuse. The pinned harness/model may be reused. Reviewer-declared inspected
references must match the provided candidate/evidence and required coverage;
missing access, modified review source, unresolved gaps or an invalid result block
approval. A model's claim of inspection is not proof of cognition or a separately
observed check. Do not promote author claims or reviewer command suggestions into
check receipts or execution authority.

**Rationale**: a distinct ID alone is insufficient independence, and a change
description alone is insufficient candidate access. A separate immutable input
and role supplies independence without requiring another provider or overriding
literal pins. Correction materializes the selected predecessor candidate and
findings, rather than recapturing unchanged original project source.

### Codex review profile

The native CLI documents fresh noninteractive execution, a read-only sandbox,
ephemeral sessions, JSONL events and schema-constrained final messages. Its exec
approval override is a configuration argument, not an assumed interactive-only
flag. Use an argv array with these proposed fixed arguments:

```text
exec --json --ephemeral --sandbox read-only
--config approval_policy="never"
--output-schema PRIVATE_SCHEMA_PATH
--output-last-message PRIVATE_RESULT_PATH
[--model LITERAL_CAPTURED_SELECTOR] -
```

The prompt is bounded complete stdin followed by EOF. Keep the existing guarded
private output reservation. Read and strictly validate the whole final JSON file;
do not extract JSON from arbitrary agent-message text. Prefer this generic exec
profile over a separate review/resume command so Council controls the schema,
candidate and literal selector explicitly. These are documented capabilities,
not a claim about every installed native version. [Native execution](https://learn.chatgpt.com/docs/non-interactive-mode),
[CLI reference](https://learn.chatgpt.com/docs/cli/reference).

Native sandbox permission does not establish a universal read-only environment
for configured integrations. Hooks and MCP have separate configuration; current
documentation provides per-server controls and a hook feature switch. Do not
invent a wildcard MCP-disable flag, inspect native configuration/credentials or
change global configuration. Do not silently use `--ignore-user-config`, since
discarding configured provider/model settings can change native semantics.
Expose limitations and fail an unsupported required access/policy contract.
[Hooks](https://learn.chatgpt.com/docs/hooks),
[MCP configuration](https://learn.chatgpt.com/docs/extend/mcp),
[configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference).

### Claude review profile

Propose a fresh print invocation, preserving existing native authentication:

```text
-p --input-format text --output-format stream-json --verbose
--no-session-persistence --permission-mode dontAsk --permission-prompts none
--tools Read,Glob,Grep --allowedTools Read,Glob,Grep
--disallowedTools mcp__* --json-schema INLINE_COUNCIL_SCHEMA
[--model LITERAL_CAPTURED_SELECTOR]
```

Each displayed value is one argv element; no quoting shell is involved.
`--tools` restricts built-in tools, whereas `--allowedTools` controls prompting.
The tools flag does not restrict MCP tools, hence the separate MCP deny pattern.
`dontAsk`/`permission-prompts none` deny unanswered approval requests. File reads
outside actual permitted access become gaps. Hooks/managed configuration and
native internals still require honest limits; this is not an OS sandbox. Do not
use bare/safe mode or replace the system prompt to manufacture compatibility.
The existing 2.1.259 transport assumption and dated documentation do not prove
this complete review profile on an installed version. [CLI flags](https://code.claude.com/docs/en/cli-reference),
[permission behavior](https://code.claude.com/docs/en/permission-modes).

Claude's schema result is `structured_output`; ordinary `result` text is a
different channel. Missing structured output can occur even with a schema request.
Add a role-specific typed terminal path while preserving 003 author response
handling. This requires no runtime SDK. [Native structured output](https://code.claude.com/docs/en/headless).

### Council's result validation remains authoritative

Use a small authored closed object schema and strict Go DTO validation, with no
new schema dependency. Native Codex sends the supplied output schema using strict
Structured Outputs; its documented subset requires every property to be listed
in `required` and every object to set `additionalProperties: false`. Nullable
unions can represent optional values, but this fixed review wire needs none.
Use required `ref`, `scope`, `paths` for every inspected-evidence entry (whole
subjects use `paths: []`), and required `path`, `startLine`, `endLine` for every
location, with actual positive integer ranges. Condition-only findings, or those
without an observed exact range, use an empty locations array and known condition
keys; never invent line numbers. Every other nested member is required too, with
empty arrays where appropriate. The uniform subject is always `{kind,id,sha256}`;
no root union, `oneOf` or conditional schema is needed. Enforce scope/range/binding
relationships and byte/count limits locally rather than relying on schema support.
The native exec request test at OpenAI source commit
`55897425e839d943a744c658701445ff732568a0` explicitly asserts `strict: true`;
reading that test is source evidence, not executing it or qualifying a live CLI.
[Native exec schema test](https://github.com/openai/codex/blob/55897425e839d943a744c658701445ff732568a0/codex-rs/exec/tests/suite/output_schema.rs),
[Structured Outputs subset](https://developers.openai.com/api/docs/guides/structured-outputs).

Bind the result to the candidate-or-response identity/digest, inspected evidence
references, findings and explicit gaps. Enforce all field, count, byte
and enum bounds again locally. Schema validity proves shape, not semantic review,
authority or successful verification. Claude documents `format` as an annotation,
not an enforced validator. Never recover a plausible JSON substring.

Require exactly one attributable main terminal, valid structured data, full stdin
delivery, root success, complete EOF/drains, unchanged applicable input and verified
cleanup. Retain trailing observations and raw output; conflicting terminals or
late failure defeat success. Keep the existing 1 MiB JSONL line bound: structured
Claude output plus its envelope must fit it. Bound the inline schema and Windows
argv size; reject an oversized invocation rather than truncate it. A malformed or
missing result is a protocol gap/failure, not permission for a correction cycle.

## 3. Observe a bounded supported check, not a model claim

**Decision**: capture typed check identity, exact executable/argv, working scope,
nonsecret environment policy, deadline and expected applicability before launch.
Discovery reads only project metadata. It does not execute help/version probes,
install dependencies or accept generated command text. Every actual child uses
existing process-group/Job Object ownership and full retained output/drain rules.

Proposed convention forms are deliberately small:

| Profile | Direct executable and argv | Scope/limits |
| --- | --- | --- |
| Literal document replacement | Council-authored deterministic checker; no model or child required | Existing whole-task grammar, one ordinary authorized document, old literal occurs exactly once and differs from new; expected bytes are that replacement only, with no extra path/mode/index change. Multiple occurrences or contradictory requirements remain unsupported for this sufficient shortcut. |
| Go module tests | Resolved `go[.exe]`: `test -count=1 -json -mod=readonly ./...` | Managed module root and captured module/lock context; no invented alternate workspace or install command. Native toolchain/dependency behavior belongs to the captured check scope. Missing prerequisites are unavailable, not passed. |
| Cargo tests | Resolved `cargo[.exe]`: `test --locked --message-format=json` | Managed package/workspace root with captured manifest/lock. Native dependency network access may occur under the authorized check scope; no lock rewrite. Compilation is not proof tests ran. |
| Node tests | Resolved `node[.exe]`: `--test --test-reporter=tap` followed by captured explicit relative test files | Bounded enumerated files, not a shell glob or invented package-script translation. Retain raw output; no version-independent test-count parser. |
| Explicit Node script | Resolved `node[.exe]`: `./AUTHORIZED_SCRIPT_FILE` plus captured argv | Explicit declaration or exactly recognized supported convention; script bytes are part of applicability. Observe program exit/output without inventing test counts or sufficiency. |

`-count=1` defeats Go's successful-test cache; `-json` provides structured observed
events. Cargo's locked mode refuses dependency-resolution changes, but does not
promise offline operation. Its optional frozen mode additionally requests offline
behavior and can be unavailable with missing dependencies; it is not the default.
Project build scripts and tests are executable code, not a sandbox. Test selection and actual
test working directories matter. An exit zero with no required tests performed
cannot establish their completion. These check tools are optional project
prerequisites, not Council runtime dependencies. [Go 1.27.3 command reference](https://pkg.go.dev/cmd/go@go1.27.3),
[Cargo test](https://doc.rust-lang.org/cargo/commands/cargo-test.html),
[Cargo build scripts](https://doc.rust-lang.org/cargo/reference/build-scripts.html).

Node reporter text is explicitly version-sensitive. Retain the complete TAP output
and owned command-completion receipt without inventing parsed test counts,
coverage or a universal reporter parser. Do not add a custom reporter module in
005. Exit zero plus complete ownership/output is observed command completion,
not a test-coverage benchmark or proof that a requested but unperformed test ran.
Plain explicit program checks have the same honest boundary.
[Node 24 test runner](https://nodejs.org/docs/latest-v24.x/api/test.html).

**Rejected alternative: arbitrary npm test/run passthrough.** npm runs its scripts
through `/bin/sh` or `cmd.exe`. On Windows its `.cmd` launcher also cannot be
directly executed like a native executable. Invoking `node npm-cli.js` removes only
that outer shim, not npm's inner shell. `ignore-scripts` does not suppress the
requested test/run script. Report a named unsupported check for arbitrary scripts;
do not invent a shell parser or silently substitute a different check.
[npm 12.2 run semantics](https://docs.npmjs.com/cli/v12/commands/npm-run/),
[Windows process launch](https://nodejs.org/api/child_process.html).

Council launches the declared program directly. Project tests/build scripts may
themselves execute code or subprocesses; no universal no-shell/no-network process
tree is promised. Capture before/after source/index fingerprints. Modified source
creates a new applicability state and cannot inherit a pass for earlier bytes.
Affected checks/review rerun; unrelated immutable receipts remain historical.

## 4. Apply only the retained delta, with durable uncertainty

**Decision**: initially support add, replace and delete of single-link ordinary
UTF-8 text files without NUL, under existing ordinary parents. Preserve exact
BOM, replacement-character and CRLF bytes. Anchor destination access and capture
exact before/after content, metadata, branch/project identity and affected index
basis. Unsupported binary, mode/type/index-only, directory, redirect, stream,
private-path or unpreservable security changes retain the candidate with a named
gap. Preserve unrelated tracked, untracked and staged state; do not use reset,
stash, checkout, automatic three-way merging or index refresh.

Git apply's `--check` is applicability checking, not a lock spanning a later apply.
`--index` requires relevant worktree/index entries to match and changes the index;
`--3way` can imply index changes and conflict content. These semantics do not fit
silently preserving arbitrary staged/dirty state. A successful Git preflight or
patch command is not a crash-atomic multi-file transaction.
[Git apply](https://git-scm.com/docs/git-apply).

Prepare intended after files and unique quarantine destinations under the existing
private Council runtime root, on the same mounted filesystem as the project.
Cross-volume publication is unsupported: no copy/delete fallback or project-local
temporary `.council` directory. Before any destination move, prove the staged
file's supported owner, security and basic-mode semantics will preserve the
original destination metadata, or the explicit new-file destination rule. A moved
private staging file must not silently install Council-private permissions into a
user project. Unsupported ACLs, extended attributes or streams block before
quarantine; this is a bounded eligibility proof, not a generic metadata copier.

Use native no-replace moves and an append-only delivery journal:

1. Recheck current authority, window, selected candidate/evidence, project/repo/
   branch, anchored parents and the exact affected destination/index basis.
   Retain before/after manifests and expected bytes before effects.
2. For an add, publish the staged file only if the destination is still absent.
   For a replace or delete, first move the actual original to its unique private
   quarantine without overwriting anything. Verify the moved identity, type,
   bytes and supported metadata against the recorded before state.
3. Only a matching replacement may publish its staged after file through a
   separately journaled no-replace move into the now-absent destination. A delete
   stops after verified quarantine. Persist intent before each move and observed
   effects afterward; recheck Stop/owner/deadline at each boundary.
4. Verify resulting content, metadata and unchanged index, and apply qualified
   file/directory synchronization. A move/sync/commit whose effects cannot be
   established is uncertain; never translate an error into proof of no effects.

The control boundary durably permits one filesystem action at a time under the
current owner/revision/Stop guards, then releases `Engine.control` before native
I/O. Stop prevents new permits and remains responsive while an already permitted
action settles to its actual partial/uncertain result. That result cannot advance
the workflow, authorize a later publication or create acceptance after Stop. A
replacement's publication needs its own fresh guard and permit after quarantine.
Retain ownership/claim until in-flight work settles; a deadline does not authorize
another worker or replay. This is request-based cancellation, not instant rollback.

The Windows primitive is `SetFileInformationByHandle(FileRenameInfo)` on a
retained checked source handle, with `ReplaceIfExists=false` and a relative name
under the verified destination directory handle in `RootDirectory`. This binds
the rename to the checked source and destination objects; path-only rename is not
an anchored fallback. The equivalent Unix candidates are Linux
`renameat2(RENAME_NOREPLACE)` and macOS exclusive renaming with retained directory
descriptors. Filesystem support must be qualified, with no existence-check plus
overwriting-rename fallback. Linux and Apple documentation expressly make
exclusive renaming a filesystem capability. Windows `MoveFileExW` documents the
same-volume/copy limitation only: copy-allowed moves can become copy/delete, which
is excluded. These dated API facts do not constitute native qualification.
[Windows handle-relative rename](https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_rename_info),
[SetFileInformationByHandle](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-setfileinformationbyhandle),
[Windows move/copy limits](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw),
[Linux no-replace rename](https://man7.org/linux/man-pages/man2/rename.2.html),
[Apple exclusive-renaming capability](https://developer.apple.com/documentation/foundation/urlresourcevalues/volumesupportsexclusiverenaming).

Replacement has a brief missing-name window. A concurrent recreation is preserved
and publication fails; a last-moment changed original may already have moved to
quarantine when verification detects the conflict. Stop between moves retains both
the quarantined original and intended after bytes. No automatic restoration may
overwrite a newer destination. A quarantined file is a live object: previously
opened external handles can still change it. Retained snapshots attest observed
bytes at a stated time, not a global editor lock or frozen future writes.

SQLite cannot transact filesystem effects, and per-file moves do not make a
multi-file change atomic. Go itself does not promise atomic rename on non-Unix
systems. Qualify namespace/durability support rather than infer a universal
power-loss guarantee. Retain precise partial effects, snapshots and live
quarantine references, fence unsafe progression and replay only known receipts.
There is no automatic quarantine deletion, rollback or effect re-execution in
005; deliberate recovery/retention tools are later scope.
[Go 1.27.3 Rename](https://pkg.go.dev/os@go1.27.3#Rename).

## 5. Keep external scope separate

**Decision**: local application never implies commit, push or deployment. The
supported external form is an explicit per-task delivery declaration, supplied by
the user through a delivery file: a named action/destination, a native executable,
static argv and `acceptanceBasis: command_completion`. It is not a provider-specific
deployment integration. No intervening shell, generated command or model-written
publisher prose supplies executable authority. A stronger remote postcondition is
unsupported and stays a named gap; command completion cannot waive an existing
task condition requiring independent remote proof.

Each action has its own `local_external` Attempt, operation identity, captured input
revision, owned child and durable effects/result journal. Outcomes distinguish
`command_succeeded`, failed and uncertain; success does not attest remote
publication merely because a command or its stdout says it occurred. Qualification uses labelled
local publisher fixtures and controlled destinations, not live credentials or
services. Known acknowledgement loss replays the stored receipt. Unknown external
acknowledgement remains uncertain with no automatic relaunch; no distributed
exactly-once service guarantee is inferred.

**Alternatives rejected**: interpreting arbitrary author/reviewer/publisher prose
as authority; mutating a settled author's evidence; reusing its session as review;
running arbitrary shell snippets; treating source modification as a passed check;
silently refilling old budgets; automatic application replay after uncertain effects.

## Qualification work carried into design/tasks

Use native-shaped fixed review fixtures for both harnesses, malformed/missing/
conflicting structured terminals, inaccessible evidence and source mutations.
Prove correction receives predecessor bytes/findings, not original source. Use real
owned verifier children, dirty/index preservation, Stop and descendant drainage,
application faults between journal/effect/receipt boundaries and publisher
acknowledgement-loss cases. Include version/permission unavailability, exact receipt
replay, old budgets/receipts, complete artifact reads, independent source/failure/UI
reviews and all six native targets plus Linux race. None of those fake/platform
results establishes live provider compatibility, account access, quality or savings.
