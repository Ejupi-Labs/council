# Implementation Plan: Verified task delivery

**Branch**: refactor (physical Git branch) | **Date**: 6 November 2026 | **Spec**: [spec.md](spec.md).
Specify's logical BRANCH output is005-verified-delivery; it does not change Git branching.
**Status**: Design drafted after closed functional-spec gate; independent design analysis/tasks
remain before runtime edits.

## Summary

Extend the accepted native owner with a bounded task workflow: author, sufficient deterministic
checks, independent review when needed, one authorized correction cycle, and truthful delivery.
Reuse owned attempts/evidence/global capacity instead of introducing a parallel process lifecycle.
Model stages consume the existing task budget; local checks/application/external delivery own
separate tagged attempts and evidence without fabricated model admissions. The common clock,
Stop, owner/fault fences and immutable historical results govern every stage.

A narrow literal-document proof can complete with one model call. Other candidates need applicable
required checks and fresh independent review or an actionable gap. The final task acceptance joins
current candidate/conditions, review/findings, checks, settlement and actual delivery. Latest
attempt success alone is insufficient.

## Technical Context

**Language/Version**: Go1.27.3; authored TypeScript7.0.2; Node24.18.1 development tools.
**Primary Dependencies**: Existing modernc.org/sqlite1.61.1 and x/sys0.49.0; standard library.
No new runtime dependency, vendor SDK, paid API executor or npm runtime requirement.
**Storage**: Existing private home/.council/v2 SQLite schema5 plus immutable artifacts; WAL/FULL,
exact int64 values and owner lock retained. Add present workflow/delivery tables only.
**Testing**: Offline Go fixtures, actual owned subprocesses, independent CLI clients, Playwright,
extracted package, npm build/test/check, six actual native targets and Linux AMD64 race.
**Target Platform**: Existing Windows/macOS/Linux AMD64+ARM64 native binaries.
**Project Type**: Single local service/CLI with embedded browser UI.
**Performance Goals**: Zero-model routing/status/reads; paged default30/max100 immutable records;
finite per-workflow stages/one correction cycle; no full transcript scan at ordinary observation.
**Constraints**: Explicit executables/argv, no intermediate shell, no vendor probes/credentials,
no automatic merge/reset/stash, private state excluded, unchanged approved assets.
**Scale/Scope**: One active owned stage in the established global slot; at most16 checks,64 findings
and a bounded one-cycle sequence. Graph scheduling, broad context retrieval, deliberate recovery/
retention, MCP and additional adapters remain006–012.

## Constitution Check

Pre-Phase0 gate: PASS. Rechecked against the revised functional specification and independent
spec-review.md; no exception is requested.

| Principle | Concrete design obligation |
| --- | --- |
| Useful outcomes | Acceptance joins candidate, required review/checks and actual delivery; no exit-code-only completion. |
| Proportionate use | Narrow exact direct proof, no planning model, one captured correction cycle; native stages share budget. |
| Honest interoperability | Fresh native role contexts with actual candidate materialization, strict review data, explicit capability gaps; no SDK/session-resume/global-setting changes. |
| Explicit autonomy | Saved prospective delivery preference names selected project at capture; old permission mode grants no delivery. External/commit actions need separate exact scope. |
| Evidence/recovery | Separate stage-owned artifacts; immutable prior terminal; write-ahead effect journal; uncertainty prevents automatic replay. |
| Small branded experience | Project/task/saved preference Start; compact next owner/action, optional details; existing Ejupi Labs assets and actual keyboard/mobile QA. |
| Specified delivery | Functional review closed before setup-plan; complete design/tasks/analysis committed before any runtime edit; final independent review/corrections/native acceptance before006. |

## Phase0 research and consequential decisions

The independent native/CLI lane owns research.md using current primary documentation. Resolve the
exact review argv/structured-output forms, supported direct checks and filesystem/Git limits before
the final design gate. No discovery/version/auth command or live task is allowed.

Decisions already frozen from actual source inspection:

1. Add optional workflow/step/role/executor/input-candidate tags to new Attempt rows. Missing tags
mean historical standalone author in interpretation; never rewrite old JSON. Native roles are
author/review/correction; local_check, local_application and local_external are explicit executors.
Native stages alone have captured native target/model admission debit. No local stage fabricates
native protocol/model/provider success or zero measured usage.
2. Each stage owns new evidence. Closed author artifacts are never appended with later review/check/
delivery results. Local application has a no-process evidence/control attempt linked1:1 to the
durable operation; command checks/external actions reuse real owned process settlement. The builtin
literal-document checker is also a no-child local_check with exact controller proof, not a fake root exit.
3. Candidate handoff reconstructs the selected retained manifest/index/bytes into a new managed
worktree, revalidates physical/control identities and publishes its new source lineage. A path,
PreviousAttemptID or report is not proof that predecessor edits are present.
4. A complete literal replacement proof reuses the accepted narrow grammar and verifies the entire
candidate delta, original constraints and exact expected bytes/index (old literal occurs exactly once
and differs from new), not just a successful command
or a routine hint. Unknown work has no direct shortcut.
5. Schema5 is additive. Upgrade positively validates schema4 and preserves every old row/envelope/
held claim/incident/default. Fresh initialization alone uses factory4; existing finite default2 is
not guessed to be untouched. No migration workflow/delivery/budget authority is seeded.
6. Keep existing routing-decision/budget-ledger SQL enum constraints. Native workflow stages arise
from captured user task authority and retain non-category-A manual admission kind, with explicit
stepId/trigger=workflow_stage provenance in new records. The existing automatic kind remains
004 proven pre-launch continuation. This must be explicit in the005 contract, never inferred as a
per-step user click or reinterpretation of old records.
7. Delivery preference is separate from permission mode; initial unspecified/old preference is
review_only. One explicit preference update can choose apply_verified_local prospectively. Each
task captures named selected project/repository/branch and scope. Local application authorizes no
commit, push, release or deployment.
8. Application uses durable operation/effects intents, retained before/after/conflicting bytes,
bounded supported ordinary path types and fresh destination checks. Do not promise atomic multi-file
transactions or exclusion of every uncoordinated external editor. Stop/fault after possible effects
is partial/uncertain with no automatic rollback/re-execution; useful candidate/user evidence survives.
9. Author/review/correction all honor original literal pins and current account/capacity/budget
controls. Review uses a fresh role/context, may use the same harness/model and does not reuse an
author session. Role-specific access and reported effective model stay explicit.

research.md resolves native role argv/structured terminal validation and Go/Cargo/direct Node
conventions. Arbitrary npm scripts remain shell-dependent and unsupported. Supported application
is ordinary single-link text add/replace/delete under existing parents, with same-filesystem
private staging/quarantine and platform exclusive no-replace moves. Preserve all originals and
conflicting recreated paths; do not overwrite, copy-delete across volumes, create parents or apply
index/mode/type/redirect changes. Those cases are retained named gaps. data-model.md specifies the
write-ahead per-path algorithm, live-quarantine retention and partial/race/fault semantics.
A no-process application/check has executor-specific proof, never fabricated native root success.

Post-Phase1 constitution gate: PASS for the designed scope. Research/data-model/contracts now
resolve fresh native roles, closed candidate/response review subjects, direct checks, finite
correction/004A replacement, no-child evidence owners and anchored exclusive application with
metadata eligibility. Stop orders one durable in-flight effect permit and releases control during
filesystem I/O; later permits/acceptance are prohibited after Stop. Existing response-only user
acceptance is preserved with selected producer/evidence bindings, without fabricated integration.
No dependency, new transport, generic graph or constitutional exception is introduced. Independent
cross-artifact task analysis and immutable design commit still precede runtime edits.

## Phase1 design and contracts

Root owns plan.md and contracts/workflow.md. Domain/store lane owns data-model.md; native/CLI lane
owns research.md; UI lane owns quickstart.md. All files must agree on public DTOs, fixed finite
stage transitions, captured authority, bounded strict input, replay and evidence applicability.

The projection records the current selected candidate rather than relying on latest native
attempt. A valid structured review is evidence consumed under previously captured correction
authority; it grants no command, scope or budget. Required check failure can trigger the same
one-cycle correction; infrastructure/protocol/evidence/cleanup uncertainty cannot.

Ordinary observation exposes paged steps/reviews/checks/findings/deliveries and compact task next
owner/action. New API1 metadata requires app2.0.0-dev.5/execution005-v1/verified_delivery, separately
from known exact004/dev.4 maintenance Stop. Old API1 alone cannot launch005 work.

### Documentation

- spec.md, checklists/requirements.md, spec-review.md: closed functional gate.
- plan.md, research.md, data-model.md, contracts/workflow.md, quickstart.md: active design.
- tasks.md: generated only after design contracts align.
- design-review.md: independent Specify analysis/closure before source commit.
- review.md, acceptance.md and evidence-matrix.md: actual implementation/final qualification later.

### Source structure

- internal/domain/workflow*.go: strict bounded stage/authority/check/review/delivery DTOs; optional new execution tags.
- internal/workflow/: pure finite workflow policy, review validation, check identity/applicability and direct proof.
- internal/store/workflow*.go: additive state/migrations, once-bound stage admission, immutable review/check/finding/acceptance records.
- internal/store/delivery*.go: preferences, operations/effects intents and truthful terminal projection.
- internal/workspace/handoff*.go, apply*.go: exact selected-candidate materialization and supported local delta application.
- internal/adapters/review*.go: role-specific fresh native builders/strict structured review parsing; author parser unchanged.
- internal/engine/workflow*.go: current boundary guards, shared owned execution, local checks/application/external effects, acceptance.
- internal/app/workflow.go, internal/httpapi/workflow.go, internal/cli/workflow*.go: shared operations and strict current-owner gating.
- ui/src/workflow*.ts plus existing tasks/workspace: compact progress/next action, evidence/findings/checks/delivery/pending intent.
- ui/tests/workflow*.test.mjs, focused Go tests, scripts/package.test.mjs: actual offline journeys.
- internal/webui/dist/: regenerated committed assets; no native binaries/private evidence in Git.

## Implementation and release strategy

Do not edit runtime until spec/design/tasks/independent analysis are complete and committed.
Implement meaningful failing vertical cases before behavior: direct proof/delivery, actual review/
correction, conflict/uncertainty and multi-interface observation. Shared DTO and transaction
signatures freeze before parallel source lanes. One heavy build/test/browser queue runs serially.

Run actual npm build/test/check, render1440px/360px keyboard/focus/reduced-motion/approved asset
checks, independent source/failure-path review and affected corrections. Commit source+embedded
assets, then qualify that exact source on six native jobs and Linux race. Close every VFD/SC and
parent status honestly before006. Fake results never establish live provider access or savings.

## Complexity Tracking

No constitutional exception. Existing Attempt/process/evidence ownership is reused deliberately;
nine present workflow/delivery tables replace a parallel lifecycle or general graph framework.
