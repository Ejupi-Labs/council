# Implementation Plan: Pause and Recovery

**Branch**: `refactor` | **Date**: 2026-11-07 | **Spec**: [spec.md](spec.md)  
**Baseline**: accepted feature 006 implementation `48484fe82e3e62df906bcb999693e4e1f0e94e2c`, with acceptance commit `7ff6357`.

## Summary

Implement durable scoped pause/drain/checkpoint, deliberate resume and evidence-backed incident reconciliation. Keep the existing lifetime-locked owner, transactional admission and exact-receipt model. No restart, observation or reconciliation automatically dispatches work. Unprovable ownership or cleanup stays visibly fenced. Scope is standalone tasks, graph members and groups. Scoped export, backup, restore and reviewed cleanup are mandatory in the immediately following feature and are not implemented or accepted here.

## Technical Context

**Language/Version**: Existing Go 1.27.3 and authored TypeScript UI.  
**Primary dependencies**: Existing repository dependencies; no runtime npm dependencies.  
**Storage**: Private `<user-home>/.council/v2/` and existing SQLite metadata; explicit schema 6→7 migration with strict schema validation. Legacy state stays separate.  
**Testing**: Controlled fake agents, actual process and owner fixtures, Go/domain/store/engine/app/CLI and Playwright tests; `npm run build`, `npm test`, `npm run check`.  
**Target Platform**: Windows, macOS and Linux AMD64/ARM64, plus Linux AMD64 race.  
**Project Type**: Local owner service, embedded branded UI and native CLI.  
**Performance**: Observation makes zero model calls; reads remain bounded. No invented latency promise.  
**Constraints**: No automatic replay or budget refill; preserve dirty work, exact receipts, outcomes, other incidents and claims. Do not inspect vendor credentials or legacy state.  
**Scale**: Existing task, graph, authority-vector and bounded read limits remain.

## Constitution Check

| Principle | Design disposition |
| --- | --- |
| Useful outcomes and proportionate usage | Pause and observation spend no model admission; resume admits only eligible unfinished work under current controls. |
| Honest harness capability | Pause drains admitted work; it does not suspend arbitrary provider processes. Unprovable process ownership remains unknown. No live provider testing without an explicit user task. |
| Autonomy | Honor saved task authority without routine approval prompts. Pause or reconciliation does not create permission for unrelated effects. |
| Preserve evidence and no replay | Transactional scope fences preserve Stop, failures, completed identities and uncertain effects. Positive reconciliation uses actual retained owned-process and durable effect proof, never PID or user assertion. |
| Branded experience | Extend existing UI with approved Ejupi Labs identity, English status/next actions, accessible keyboard/focus and narrow layouts. |
| One feature at a time | Finish Specify, design, tasks, implementation, independent review/correction and acceptance before the immediately following retention slice. Work remains on `refactor`; no branch hook. Do not change main until the full rebuild is finished, committed and verified. |
| State and interoperability | Preserve 001–006 records and UI/CLI parity; use private state outside Git/release. Host integration remains later. |

**Pre-design gate**: PASS for the scoped feature.  
**Post-design gate**: PASS. Independent cross-artifact review identified and verified corrections for credential exclusion, member/group scope, admission and effect boundaries, and exact-source acceptance. Commit this design and task list before application implementation.

## Project Structure

### Documentation

```text
specs/007-recovery-and-retention/
├── spec.md, checklists/requirements.md
├── plan.md, research.md, data-model.md, quickstart.md
├── contracts/pause-reconciliation.md
└── tasks.md                     # generated after design review
```

Retention contracts are archived for the immediately following feature and do not qualify this slice.

### Source Code

```text
internal/domain/        # pause/checkpoint/incident DTOs and validation
internal/store/         # v7 migration, admission gates, checkpoint and reconciliation transactions
internal/engine/        # owner-side pause/drain, explicit resume, exact evidence capture
internal/process/       # exact owned process-tree cleanup; no PID takeover
internal/app/           # owner-routed mutations and bounded reads
internal/cli/           # equivalent controls and exact retry
ui/src/                 # task/graph pause and incident controls
internal/webui/dist/    # committed compiled browser assets
scripts/, ui/tests/     # offline native/package and browser qualification
```

**Structure Decision**: Extend current owner, Store and UI patterns. Do not add a second coordinator or let client-side preflight substitute for transactional authority. Assign exact file ownership in tasks.

## Design and verification commitments

1. Commit pause and a durable `pausing` checkpoint under the same serialized Store authority as every task/graph initial admission, graph input, continuation, local workflow step and final-delivery admission. A scope fence blocks new work; already admitted work drains under existing Stop/failure precedence. The checkpoint becomes `paused` only after known settlement and durable evidence. Owner restart preserves the fence and never dispatches.
2. Support standalone task, graph-member and whole-graph scopes. Member pause retains parent identity, dependencies and shared-budget authority. A member pause does not pause independent siblings; overlapping group/member pauses remain independently effective. Resuming either scope cannot bypass another active pause or blocker.
3. Resume is a new explicit idempotent request with exact scope and checkpoint revision. Recheck current owner, task/graph revisions, instructions, Stop, input/dependency evidence, capacity, shared budget/window and all fences in the same authority transaction. It does not refill budgets, replay completed attempts or effects, or infer permission from an owner restart.
4. Schema 6→7 introduces pause/checkpoint and reconciliation evidence/decision records. Freeze a populated accepted-v6 fixture, validate exact schema and indexes, preserve prior rows/receipts and unresolved claims, and reject unknown schema. A migration cannot infer old incidents resolved.
5. Reconciliation uses the authenticated owner and actual exact-scope proof. A controlled positive case retains the real still-held `process.Owned.Cleanup` result for the complete owned process tree, together with complete durable effect/evidence checks; it atomically records proof and releases only the verified-safe claim after recomputing all remaining blockers. Missing proof, another active blocker or unknown scope remains fenced. A PID, elapsed time, user assertion or absent reply is not proof. After owner loss, a vanished process handle cannot be re-created by assertion; incidents may remain unresolved.
6. Extend `ui/src/graphs.ts`, `ui/src/tasks.ts`, `ui/src/api.ts`, `internal/cli/graphs.go`, `execution.go`, and owner/app routing with matching operation IDs, receipts, stale-response guards, proof-captured exact retry and visible pending/unknown states. Owner/Store enforce authority. Pause is visibly distinct from Stop and Advance.
7. Verify actual admitted-work drain, pause-versus-admission/first-GO/control races, overlapping scopes, lost acknowledgement, owner loss, checkpoint persistence failure, explicit resume, positive physical reconciliation and paired missing-proof refusal, retained unrelated blocker, preserved edits and unchanged debit counts. Test UI/CLI parity and real keyboard/narrow states with fake agents. Run focused affected checks, then exact-source full build/test/check and six native platform jobs plus Linux AMD64 race, followed by independent review and acceptance.

## Deferred next slice

Scoped evidence export, project backup, verified restore and reviewed cleanup remain required immediately after 007, with their original failure/authority requirements intact. The accepted baseline has no concrete mechanism establishing arbitrary user text/files credential-free; fixture output, a path allowlist or an unverified client assertion is insufficient. The following feature must resolve that production provenance gap honestly, provide usable explicitly bounded recovery profiles, and never inspect/copy vendor credentials. This limitation is not a gate for implementing the present pause/reconciliation slice and is not represented as delivered.

## Complexity Tracking

No planned constitutional exception. Durable scope fences and proof-bound reconciliation are necessary for safe crash behavior; UI-only pause or PID-based claim release is not an acceptable simplification.
