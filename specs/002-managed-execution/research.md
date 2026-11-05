# Research and architecture decisions: managed execution

**Version**: 1.1.0 | **Date**: 2026-11-05 | **Feature**: 002 | **Branch**: `refactor`

These ADRs are accepted for the 002 design. Implementation, native process qualification and live
provider qualification remain pending. Foundation93c8c25 passed all seven hosted jobs in
run42115752948; its prerequisite validation gate is closed. Changes to a decision require an attributed ADR revision, not an
implicit adapter fallback. The [specification](spec.md) governs product behavior.

## ADR-002-01 v1 — Extend the accepted native core

**Decision:** retain Go 1.27.3, the embedded authored TypeScript UI, x/sys v0.49.0 and the existing
SQLite/private-state ownership model. Release identity becomes `2.0.0-dev.2`; migrate schema 1
to schema 2 transactionally under the OS lifetime lock, preserving 001 records and receipts.
API major 1 remains; execution additionally requires the `managed_execution` capability and
`executionContract: "002-v1"`. An older foundation owner is not execution capable.
**Reason:** one core gives UI/CLI the same identities and admission authority without a framework.
**Rejected:** a second runtime, a separate execution database, schema reset or capability inference
from API major alone. See the [001 decisions](../001-workspace-foundation/research.md).

## ADR-002-02 v1 — Stable native Codex exec contract

**Decision:** use only shell-free native `codex exec`, with an absolute discovered executable,
explicit cwd/argument vector, prompt through stdin `-`, `--json`, `--ephemeral` and an owned
`--output-last-message` path. Record a versioned adapter contract and observed native version;
unsupported flags/protocol produce a capability failure, never a changed transport.
**Reason:** the current official reference labels exec stable. Its documented JSONL stream has
thread/turn/item/error events; retain raw events and parse supported fields without converting
assistant prose into control authority. Root exit, terminal event and final message are separate
observations. [CLI reference](https://learn.chatgpt.com/docs/cli/reference),
[non-interactive output](https://learn.chatgpt.com/docs/non-interactive-mode)
**Rejected:** SDK/runtime API fallback, an app-server default, interactive TUI automation,
automatic resume/retry, and the removed `--full-auto` shortcut. Fake fixtures validate this exact
contract; they do not qualify an installed vendor binary or a provider account.

## ADR-002-03 v1 — Apply saved autonomy and preserve unknowns

**Decision:** map saved autonomous full access to supported `--sandbox danger-full-access` and
`--ask-for-approval never`. The documented `--sandbox read-only` option does not provide
supervision and is not advertised as a supported 002 policy mode. Unsupported supervision blocks
before launch: 002 provides no interactive tool-approval relay.
Honor an explicit `--model` selector or block it; Auto uses native configured selection, with
unreported effective model/provider/account/usage retained as unknown. Distinguish requested,
effective and observed values; a contradicted pin cannot satisfy acceptance.
**Reason:** these are documented native controls, not a Council OS sandbox or billing guarantee.
Vendor authentication stays vendor-owned; no credential reads/copies, hidden login, global-config
changes, paid API fallback or invented account choice. [Native options](https://learn.chatgpt.com/docs/cli/reference),
[permission behavior](https://learn.chatgpt.com/docs/non-interactive-mode)

## ADR-002-04 v1 — Continuing owner and shared admission

**Decision:** reuse the existing foreground UI owner; a CLI asynchronous start with no owner
self-spawns the exact executable in a hidden internal persistent-owner mode. Use the known
private v2 root, a private bootstrap descriptor, the native lifetime lock and authenticated
published owner descriptor. Competing candidates converge on the lock winner before SQLite opens.
Client/browser disconnection only ends observation; explicit owner stop closes admission and
settles work. Detach client standard handles and request contexts from task lifetime.
**Reason:** this meets continuing execution without an OS-service installer or daemon framework.
Enclosing OS jobs/session shutdown can still restrict lifetime; report such limits. No stale-PID
takeover, lock deletion or second writer. [Windows launch flags](https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags)
One `codex-native` claim spans all projects/clients; native and fake executions share its limit of
one active attempt. Queueing creates no extra model calls. Recheck task version, policy, source,
deadline, cancellation, ownership and claim before durable launch intent and dispatch.

## ADR-002-05 v1 — Atomic Windows job membership

**Decision:** create each attempt directly in one unnamed job using native `CreateProcessW`,
`STARTUPINFOEX` and `PROC_THREAD_ATTRIBUTE_JOB_LIST`; set `KILL_ON_JOB_CLOSE`, disable breakaway,
and inherit only explicit child stdio handles. The owner retains the non-inheritable job handle.
This requires Windows 10+/Server 2016+ APIs; state the supported floor and qualify AMD64/ARM64.
**Reason:** creation-time assignment removes the orphan window between suspended creation and
later job assignment. Go exec does not expose this attribute and closes the primary-thread handle;
use a narrow x/sys launcher, native handle sizes and required documented SDK definitions only.
The missing job-list attribute constant is `uintptr(0x0002000d)`.
[SDK definition](https://github.com/microsoft/win32metadata/blob/main/generation/WinSDK/RecompiledIdlHeaders/um/WinBase.h)
[Microsoft creation example](https://devblogs.microsoft.com/oldnewthing/20230209-00/?p=107812),
[job-list attribute](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-updateprocthreadattribute),
[Go 1.27.3 source](https://github.com/golang/go/blob/go1.27.3/src/syscall/exec_windows.go)
After termination, wait/release root handles and verify the still-owned job's active count reaches
zero; API success/root exit is insufficient. Crash closes the last handle but loses the receipt,
so restart retains an interrupted incident. Brokers/services remain outside a universal tree or
security claim. [Job limits](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects),
[active accounting](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-jobobject_basic_accounting_information)

## ADR-002-06 v1 — Unix guardian with protected group identity

**Decision:** use one same-binary guardian per attempt with `Setpgid: true, Pgid: 0`; the owner
stays outside that group. Guardian READY/GO gates the vendor launch, excludes private control
handles from vendor inheritance, reports the vendor root outcome, then stays as the PID anchor.
The owner sends final TERM/KILL while that child remains unreaped, then calls Wait/reap. After
reaping it may only probe `kill(-PGID, 0)`, never signal again. ESRCH confirms the managed group
is empty; subsequent PGID reuse can cause conservative blocking, not killing an unrelated group.
**Reason:** Go establishes the group before exec; retaining the guardian identity closes the
cached-PGID signalling race. A background Wait must not reap the anchor before the last signal.
These ordering conclusions require native race evidence. [Linux launch](https://github.com/golang/go/blob/go1.27.3/src/syscall/exec_linux.go),
[macOS launch](https://github.com/golang/go/blob/go1.27.3/src/syscall/exec_bsd.go),
[group signal contract](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/kill.2.html)
Owner-control EOF before GO launches no agent; afterward guardian KILLs its group including itself.
No post-cleanup receipt survives that path: restart blocks an interrupted incident without replay.
**Limit:** group membership is not arbitrary-descendant containment. setsid/setpgid/services can
escape; known escape or lost identity leaves cleanup unresolved. No cgroup/service requirement,
second guardian framework or universal OS-sandbox claim. [Session escape primitive](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/setsid.2.html)

## ADR-002-07 v2 — Capture current source and staged semantics without repository filters

Revision attribution: root integrator, 2026-11-05; preserve the actual staged index for tasks such
as reviewing staged changes, rather than retaining it only as a separate artifact.

**Decision:** enumerate tracked metadata and ordinary unignored paths with sanitized shell-free
Git, then capture actual current bytes/modes/supported in-root links using Go filesystem APIs.
Record physical identity, HEAD/branch/index evidence and a hashed source manifest. Exclude Git
control, Council state, vendor credentials and ignored authentication data; required excluded or
unsafe/unrepresentable input blocks specifically. Detect capture races; never substitute clean HEAD.
Create a detached `git worktree add --no-checkout` at the known base and load its index with
`read-tree` without `-u`. Reconstruct the original staged blob/mode state using a cached binary
diff with external diff/textconv disabled and index-only application, then copy exact current
working bytes through rooted Go filesystem APIs. Tests prove the worker sees the same staged and
unstaged versions while original files/index/branch stay unchanged and no clean/smudge/hook runs.
Unsupported split/sparse/intent-to-add/unmerged/submodule/unborn states block specifically.
**Reason:** a normal clean checkout omits dirty inputs; checkout/filter execution can transform
bytes or run repository-controlled programs. Derive the candidate delta against the captured
source manifest, not HEAD, so user edits never become the agent's alleged changes.
[Worktree options](https://git-scm.com/docs/git-worktree), [index-only tree read](https://git-scm.com/docs/git-read-tree),
[path enumeration/exclusions](https://git-scm.com/docs/git-ls-files),
[cached diff controls](https://git-scm.com/docs/git-diff), [index-only patch application](https://git-scm.com/docs/git-apply).
Pinned Go1.27.3 os.Root documentation was also inspected: rooted opens reject symlink escapes,
but link creation needs separate target validation and Root is not a mount/device sandbox.
**Rejected:** stash/reset, original-index mutation, checkout-index/smudge materialization, and a
plain HEAD diff. Retain partial candidates; 002 does not commit, integrate, publish or delete them.

## ADR-002-08 v1 — Independent outcomes, cancellation and durability

**Decision:** keep task outcome, immutable attempt outcome, acceptance, cleanup and evidence
durability separate. Serialize durable Stop acceptance with terminal decisions; earlier primary
failure and accepted cancellation survive late success. Expected termination exits are secondary
cleanup evidence. Old-attempt events cannot overwrite the latest task-version result.
Use existing exact receipts for admission/Stop/acceptance and conflict on changed same-ID intent.
Persist admission before launch; publish evidence offsets only after bytes are durable. A storage
fault closes dispatch, preserves known output and retains uncertainty without replaying a model.
**Reason:** a final event, zero exit, acknowledged operation or incomplete result is not acceptance.
Write candidates show Verification pending for feature 005. The optional response-only manual
acceptance path names the exact current task version and closed response/hash; it cannot waive
required checks/review or become a permanent approval gate for the finished product.

## ADR-002-09 v1 — Finite execution and evidence bounds

**Decision:** apply the following bounds, retaining complete originals within storage limits and
blocking an oversized assembled prompt before any model invocation. Exceeding output bounds
requests termination and retains obtained bytes plus explicit missing/incomplete evidence.

| Resource | 002 limit |
| --- | --- |
| Total admitted execution, including queue/preparation | Default 1,800 seconds; explicit 1–86,400 seconds |
| Stop/shutdown settlement | 2-second grace within 15 seconds total termination/drain/reporting |
| Captured source | 100,000 entries; 64 MiB/regular file; 512 MiB regular-file total |
| Council-assembled prompt | 256 KiB UTF-8; no silent shortening of original instructions |
| Raw stdout / stderr / native JSONL line | 64 MiB / 16 MiB / 1 MiB |
| Evidence read | At most 64 KiB raw bytes per base64 chunk, with exact offsets/completeness |

These are byte/resource/transport bounds, not model-context, token, charge or provider-spend caps.
Drain stdout/stderr concurrently with owned readers through cleanup; final JSON does not close
them. Own the drain deadline explicitly: `WaitDelay` can force-close pipes and does not prove
descendant cleanup or manage caller-owned readers. Deadline expiry yields a failed timeout or
unresolved cleanup as actually observed, never fabricated quiescence. [Go exec semantics](https://pkg.go.dev/os/exec@go1.27.3)

## ADR-002-10 v1 — Qualify claims before acceptance

**Decision:** keep fake/native process fixtures on the same owner, admission, adapter, persistence
and evidence path. Cover duplicate/lost acknowledgements, two-client capacity, dirty source,
cancel/failure/late-success races, early final events, retained output tails, process descendants,
guardian/job crash boundaries, restart fences and no replay. Unresolved cleanup globally fences
new dispatch while retaining inspection. Native Windows/Linux/macOS results are reported per
actual runner; cross-compilation proves compilation only. Live Codex qualification remains
unperformed until a specific user-requested live task, with no credential inspection.
**Reason:** documentation and fixtures define a testable contract, not a claim of provider access,
telemetry completeness or OS containment. Run the required build/test/check commands and inspect
the branded accessible UI, obtain independent review, correct blockers and retain acceptance
evidence before feature 003. No application implementation is authorized by this research alone.
