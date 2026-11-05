# Implementation Plan: Managed native Codex execution

**Branch**: `refactor` | **Date**: 2026-11-05 | **Spec**: [spec.md](spec.md)

## Summary

Extend the accepted workspace with one bounded native-Codex worker per explicit execution.
Preserve current source/staging in a managed worktree, retain exact input/output and candidates,
coordinate one connection-wide slot, and make cancellation, cleanup and uncertain ownership honest.
A candidate remains Verification pending; an optional exact response-only user acceptance provides
a narrow completion path until feature005. No automatic retry, second harness, routing, merge,
publication or MCP is implemented here.

Foundation implementation93c8c2530f99ff97f35e7c55180df9b42d97fcb6 passed all seven jobs in
[run42115752948](https://github.com/Ejupi-Labs/council/actions/runs/42115752948): six native platform
journeys and Linux AMD64 race checks. This closes the prerequisite gate.

## Technical context

| Concern | Decision |
| --- | --- |
| Runtime | Existing Go1.27.3 core and embedded strict TypeScript UI |
| Dependencies | Existing modernc SQLite1.61.1/libc1.79.0/xsys0.49.0; no new runtime npm/SDK dependency |
| Preview / storage | Application2.0.0-dev.2, API1 with executionContract002-v1; transactional schema1→2 |
| Native interface | Stable Codex exec JSONL, explicit argv and stdin; native authentication remains vendor-owned |
| Owner | Existing OS lock/private root; foreground UI owner or hidden exact-self background owner for asynchronous CLI |
| Execution | One scheduler/connection slot shared by native and explicitly labelled fake attempts |
| Process control | Windows creation-time owned job; Unix anchored process-group guardian with ordered cleanup |
| Source | Current tracked/unignored bytes and meaningful staged index in detached managed worktree; exact source-relative delta |
| Evidence | Private byte artifacts, exact cursors, bounded typed events, immutable terminal observations |
| Verification | Local fake harnesses, actual native descendants/crashes, browser/CLI parity, six native CI targets plus supported race |

Use [research](research.md), [data model](data-model.md), [execution contract](contracts/execution.md)
and [quickstart](quickstart.md). A stable native interface is documentation feasibility;
fake qualification and a user-authorized live task remain separate.

## Constitution check

Pre-design and post-design checks pass with the following concrete obligations.

1. Useful work: retain candidate/full response and actual next action; process success is never
   accepted implementation. No automatic commit/integration/publication.
2. Economy: one explicit worker, no model admission/classifier/wait/status call, no automatic
   retries or account/payment substitution. Native-internal call counts remain unknown unless
   reported; a worker count is not a token/charge cap.
3. Interoperability: requested selectors, configured native mode and reported effective facts
   remain separate. Unknown account/model/usage stays unknown. No vendor credential read/copy.
4. Autonomy: saved requested full access maps to supported native flags; unsupported supervision
   blocks clearly. Per-task pins/limits are optional details, never an onboarding questionnaire.
5. Truthful recovery: durable launch intent precedes process instructions, accepted control order
   preserves primary failure/cancellation, unknown cleanup fences dispatch, restart never replays.
6. Brand: extend the approved UI with accessible execution/detail/Stop and pending-input guards.
7. Delivery: specification/design/tasks committed before source; meaningful fake/native tests,
   independent review and corrected blocking findings before003.

No constitution exception or new user permission checkpoint is required.

## Application boundaries

The foundation services/store remain authoritative for projects, policy and drafts. Add execution
DTOs/validation to domain and transaction methods to store. An engine owns scheduling/lifecycle;
it depends on process, workspace, evidence and the Codex adapter, not on HTTP or CLI. App routes
delegate to engine services; CLI/HTTP expose the same durable facts. No generic plugin framework.

```text
internal/domain/execution*.go
internal/store/execution_schema.go tasks.go attempts.go events.go artifacts.go
internal/engine/           # scheduler, admission/control, settlement and restart fences
internal/process/          # Windows native job launcher / Unix guardian and drains
internal/workspace/        # source/index capture, safe materialization, manifest delta
internal/evidence/         # private streams, durable prefixes, immutable artifact reads
internal/adapters/         # native Codex argv/JSONL, supported launcher resolution
internal/fixture/          # explicitly fake deterministic scenarios/internal fixture entrypoint
internal/owner/            # hidden service bootstrap, foreground reuse, shutdown
internal/app/              # execution operation delegation; foundation behavior retained
internal/httpapi/          # strict new route/query dispatch and wait observation
internal/cli/              # task/attempt/evidence/service commands, independent observers
ui/src/tasks.ts            # admission/details/events/result/Stop/guidance/acceptance
ui/tests/execution*.mjs
specs/002-managed-execution/
  spec.md plan.md research.md data-model.md quickstart.md tasks.md contracts/
```

Keep neutral DTOs in domain to avoid import cycles. The main executable dispatches tightly scoped
internal owner/guardian/fixture modes before ordinary CLI parsing; no public arbitrary-executable
endpoint is introduced. Fake scenarios are a fixed whitelist and always visibly tagged.

## Admission, versions and scheduling

POSTtasks validates typed input, project/source-draft relationships and selectors, then commits
task/version/attempt/event/receipt atomically. Same request/input replays before new policy or stale
version checks. Direct input can pin expectedPolicyRevision; exact draft provenance captures its
immutable policy. UI Start may auto-save a changed saved draft's clicked intent first, preserving
separate pending save/start IDs and newer typing. Saving a draft alone never means Started.

Task original text never changes. New guidance/policy choices create immutable versions; a queued
old-version attempt blocks before dispatch. Running attempts retain their original snapshot. A
new explicit execution may create another linked attempt after settlement and verified cleanup;
there is one worker per such execution and no automatic retry. Async stateRevision is separate
from user-mutation revision so progress does not invalidate Stop.

One scheduler actor serializes accepted control/outcome order. One capacity claim covers
codex-native across projects/clients and native/fake modes. Claim/recheck gate, cancellation,
version, deadline and source before launch. No file/process/network wait occurs inside SQL
transactions. Durable preparation/launch intent and current source references precede GO/creation.
A process spawn cannot be atomic with SQL; a launch-intent crash is potentially executed and
cannot be automatically replayed.

Default deadline is admission+1800seconds, including queue/preparation; optional1–86400seconds.
Show that captured deadline/remaining time. These bounds control Council waiting, not provider
billing or kernel termination. Do not start after expiry.

## Current-source materialization

Use sanitized shell-free Git commands with no optional locks, disabled fsmonitor and owned empty
hooks path. No checkout, user filter, external diff, textconv, stash, reset or original branch/index
write during capture. Validate physical project/HEAD/selected subfolder and source before/after.

Create a detached worktree at the exact base using --no-checkout. Reconstruct its meaningful staged
index from the original staged blob/mode state, then copy exact current working bytes and ordinary
unignored source. The selected subfolder maps identically. A cached binary diff with disabled
external/text conversions plus index-only application is the selected initial index strategy;
tests must prove no clean/smudge/hook invocation and equivalent staged/unstaged semantics.
Unsupported split/sparse/unmerged/intent-to-add/submodule/unborn states block specifically rather
than importing a misleading index. Original index bytes and staged-entry identity remain evidence.

Use Go os.Root for rooted source/artifact/workspace access, ordinary-file checks and exclusive
private creation. Root follows only in-root relative symlinks, but is not a filesystem/OS sandbox:
reject special files and unsupported links/mount layouts explicitly where capture cannot be safe.
Root.Symlink does not validate its target, so verify supported link targets separately. Never copy
Council state or known vendor authentication material. Retain exclusions and source-limit blockers.
No hardlinks that let worker writes alter originals.

Snapshot exact regular bytes/mode, link targets and staged state with hashes. Bounds:100kentries,
64MiB/file,512MiB total regular bytes. Source capture may retry a racing read finitely with zero
model calls, then blocks. Candidate delta is against that actual captured source/index state,
not against clean HEAD, so it excludes pre-existing user work. Retain failed/partial candidates.

## Native adapter and process ownership

Resolve supported native executables explicitly, including known native package layouts when
appropriate; shell wrappers remain unsupported. No config/auth scan or hidden login. Qualify fixed
version/help contracts with fake metadata fixtures. Pass model pin exactly; configure autonomous
execution with supported native permissions and no approval prompt. Native configured model,
provider/account/billing remain unknown unless reliably reported. Do not use removed full-auto,
experimental app-server, account substitution or a Council-managed paid API path.

Assemble full original text, applicable guidance/conditions and a short scope/workspace brief.
Cap Council prompt bytes at256KiB before native launch; larger storage-valid input stays retained
with a blocked attempt. Never truncate/replace constraints. The prompt includes only this work,
with original→managed project path mapping; it is not the entire collaboration history.

Windows: create one unnamed no-breakaway job, kill-on-last-handle-close. Use STARTUPINFOEX job-list
assignment during CreateProcess, with only child stdio handles inherited; job/DB/lock/control
handles are excluded. Do not use a suspended orphan window or PID-based takeover. Query owned
job active-process accounting before reporting scoped cleanup verified.

Unix: one same-binary guardian starts in a new process group and launches no vendor work until
private READY/GO. It stays as PGID anchor after vendor-root outcome, excludes control endpoints
from vendor inheritance, and handles owner EOF. Final TERM/KILL signals precede guardian Wait/reap;
after reaping only probe, never signal the old PGID. Owner loss triggers group self-KILL and an
interrupted fence on restart. Escaped groups/services are outside this process scope; known
escapes/uncertainty block. No universal descendant or OS-sandbox claim.

Drain stdout/stderr concurrently through bounded collectors, retaining exact bytes. Codex JSONL
parser records documented typed events and optional sourced usage; malformed output is a fault,
not a plausible substring extraction. Require compatible terminal event, actual root exit, usable
output, durable evidence and scoped cleanup for successful authoring. Native prose is evidence,
never a command or observed check.

## Evidence, cancellation and restart

Stdout64MiB, stderr16MiB, JSONLline1MiB. Chunk artifact reads use64KiB byte limits/base64 and exact
decimal offsets with a fixed durable-prefix bound. Retain incomplete/invalid raw bytes explicitly.
Close/hash artifacts before accepting immutable response evidence; detect redirected/corrupt files.
Coalesce durable stream checkpoints rather than writing one SQL row per token; final/fault/control
boundaries flush immediately. Expose only committed known prefixes. A flush/storage fault stops
conflicting dispatch and never reruns a model to recreate evidence.

Stop commits accepted control before signalling. Earlier authoritative primary failure stays
primary, accepted cancellation defeats late success. Root outcome, evidence durability and cleanup
remain distinct. Grace2seconds is included within total15second stop/drain effort. Expiry/query
failure yields unresolved cleanup and retained capacity/fence, never a false successful Stop.
Do not acknowledge finalized delivery while streams/processes remain unsettled.

An observer timeout/disconnect cancels observation only. CLI asynchronous starts ensure a continuing
hidden exact-self owner first; concurrent bootstrap contenders reconcile via the existing OS lock
and authenticated descriptor. Metadata-only commands may retain transient ownership. No task/token
in argv, no OS service installation, no PID takeover. Explicit service stop targets owner instance,
closes admission, cancels/settles work and reports uncertainty honestly.

At startup migrate schema transactionally, retain accepted terminals, mark unfinished prior work
interrupted and fence uncertain ownership/evidence. Queued old work does not auto-dispatch. A clean
closed owner with no claims/incidents may reopen admission for NEW explicit work. No automatic
artifact disposal or full incident reconciliation/resume is added before007.

## UI, CLI and acceptance boundary

Primary path: project + task + Start; Auto/native configuration and saved requested policy visible.
Task detail exposes current stage/next action, exact output/evidence, candidate, deadline and Stop.
Use compact summaries and explicit full reads, guarded resource/edit generations and stable pending
intents. Keep accessible keyboard/360px/reduced-motion behavior and approved brand assets/notices.

Candidate success remains Verification pending. Response-only acceptance is optional, explicit,
version/hash-bound and permitted only with no authored delta/outstanding requirements and verified
settlement. It is not an automatic test receipt or the final product's permanent manual gate.
CLI admission/read/wait success means that operation was acknowledged; all task/attempt outcomes
remain explicit in data. No partial result or fake scenario implies real task acceptance.

## Validation and delivery

Follow tasks.md in dependency order. Meaningful tests cover schema1receipt preservation, replay,
exact counters, guidance/cancel races, source/index/filter/hook parity, native ownership and crash
boundaries, stream faults/tails, global capacity, observer independence, restart without replay,
UI clicked-intent preservation and actual narrow/keyboard flows. Fake-only calls; no live provider.

Run npmrunbuild, npmtest, npmruncheck; keep embedded UI/notices/version/package README current.
Qualify native Windows/macOS/Linux AMD64/ARM64 plus supported Linux race checks. Independently
review the actual source/evidence, correct blocking findings, update traceability and commit before003.
