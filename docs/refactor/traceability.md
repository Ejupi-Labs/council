# Council rebuild requirements traceability

Status: features 001–005 are accepted. Foundation qualified on all six native targets at `93c8c25`;
managed Codex execution qualified on six targets plus Linux AMD64 race at `f5edd96` in run 42140818152.
Claude interoperability qualified on all seven jobs at `ce8aaa7` in
[run 42153455521](https://github.com/Ejupi-Labs/council/actions/runs/42153455521).
Feature004 qualified at `3f4a18e` in run42172669381. Feature005 dev.5/005-v1 qualifies at
`794f08032b96434d1217f8bb3797c6803263f39c` in run42224386878 on six native targets plus race.
Its acceptance/review records retain the first failed run and corrected-source closure. Other parent requirements
remain partial or not implemented as marked. Feature006 dev.6/006-v1 implementation is under
qualification; it has no accepted matching-source local/native release gate. Date: 7 November 2026.

This matrix maps [functional requirements](functional-requirements.md) to the ordered slices in
[the delivery plan](delivery-plan.md). It does not credit the legacy runtime at
`27822baa3aee663366a870bde7fb1190649d3418` as rebuilt implementation or acceptance evidence.
All work remains on `refactor`.

## Reading and maintaining this matrix

The primary slice owns the requirement's complete acceptance scenario and traceability update.
Supporting slices introduce prerequisites, extend the behavior or repeat regression qualification.
This is not permission to postpone foundational controls: task ownership, bounded admission,
failure/cancellation precedence, complete evidence, isolation and fail-closed cleanup exist from
002. Slice 007 adds deliberate recovery and retention operations to that established base.

Features 001–003 acceptance.md and review.md records include their actual hosted qualification,
independent review/corrections and separately unperformed live-provider qualification.
Feature 004 qualified at3f4a18e in run42172669381 on six native targets plus Linux race; its
acceptance/review records and 38-row evidence matrix record actual local/native checks and closed review.
Feature005 design was committed at `24b0cf1` before implementation. Its
[acceptance ledger](../../specs/005-verified-delivery/acceptance.md) and35-row evidence matrix
close the specified workflow scope at794f080, including all six native jobs and Linux race.
Future graph, recovery, host and remaining adapter scopes are not implied by that acceptance.
Feature006 specification/design/tasks were committed before implementation under specs/006-task-graphs.
Its [active review](../../specs/006-task-graphs/review.md) and
[draft evidence matrix](../../specs/006-task-graphs/evidence-matrix.md) record bounded actual offline
checks, independent findings and corrections. Complete visual/client/package qualification, final
local checks and all seven matching-source native/race jobs remain acceptance gates. Features007–012
and their evidence paths remain planned. A path or a focused pass alone is not feature acceptance.
The feature's `spec.md`, `plan.md` and `tasks.md` must trace the same FRs before implementation.
Its `review.md` must record independent inspection, blocking findings, corrections and affected
rechecks. Only the active feature receives those detailed artifacts. No future feature directory,
checked task or successful provider qualification is implied by this document.

A requirement can move to IMPLEMENTED only when its primary acceptance scenarios have observed
terminal evidence against the accepted change, supporting prerequisites are satisfied and the
independent review/fix gate has closed. An unperformed scenario stays unperformed; partial
implementation or a fake adapter response never implies live compatibility. Feature delivery
records separately identify implementation, acceptance and fake/live qualification status, so a
single status cannot hide an unmet claim.

## Requirement-to-evidence matrix

| Requirement | Acceptance responsibility | Primary slice | Supporting slices | Acceptance evidence location | Current status |
| --- | --- | --- | --- | --- | --- |
| FR-001 | Open existing projects and reach the composer without transport setup | 001 | 002, 012 | `specs/001-workspace-foundation/acceptance.md#project-opening` | IMPLEMENTED — six native platform jobs passed at 93c8c25 |
| FR-002 | Separate harness discovery, connection and verified capability status | 001 | 002, 003, 009–012 | `specs/001-workspace-foundation/acceptance.md#harness-readiness-and-authentication-boundary` | PARTIAL — Codex and Claude native contracts/fake qualification accepted; remaining adapters and live qualification remain separate |
| FR-003 | Reuse vendor-owned authentication without accessing credentials or changing global settings | 001 | 002, 003, 009–012 | `specs/001-workspace-foundation/acceptance.md#harness-readiness-and-authentication-boundary` | PARTIAL — native Codex and Claude delegate authentication without credential/global-setting access; live and remaining-harness qualification remain separate |
| FR-004 | Keep provider, account, harness and model identities distinct; link shared accounts | 004 | 002, 003, 009–012 | `specs/004-economical-routing/acceptance.md#target-and-account-identities` | IMPLEMENTED — specified native scope accepted at3f4a18e on six platforms plus race; live provider qualification remains separate |
| FR-005 | Supply all five core lifecycle adapters with truthful optional capabilities and qualification | 012 | 002, 003, 009, 010, 011 | `specs/012-release-qualification/acceptance.md#adapter-capability-matrix` | PARTIAL — Codex and Claude native lifecycles accepted with truthful limitations; DeepSeek Harness, Junie and Antigravity remain planned |
| FR-006 | Keep documented collaboration state outside Git and release artifacts | 001 | 002, 007, 012 | `specs/001-workspace-foundation/acceptance.md#local-state-boundary-and-authority` | IMPLEMENTED — six-platform foundation; later records repeat exclusion checks |
| FR-007 | Admit tasks with project/task and Auto defaults, preserving resolved choices | 002 | 001, 004, 005, 012 | `specs/002-managed-execution/acceptance.md#task-admission-defaults` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-008 | Preserve original task text, constraints and attributable guidance versions | 002 | 006, 007, 012 | `specs/002-managed-execution/acceptance.md#original-instructions-and-guidance` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-009 | Persist and apply scoped autonomous full-access preference without repeat permission prompts | 002 | 001, 003, 009–012 | `specs/002-managed-execution/acceptance.md#effective-execution-policy` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-010 | Report actual harness policy capabilities and restrictions honestly | 002 | 003, 009–012 | `specs/002-managed-execution/acceptance.md#permission-capabilities-and-limits` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-011 | Validate task data and destination before side effects | 002 | 004, 005, 012 | `specs/002-managed-execution/acceptance.md#preflight-and-destination` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-012 | Select proportionate workflow stages without mandatory model planning, debate or review | 005 | 002, 004, 012 | `specs/005-verified-delivery/acceptance.md#proportionate-workflow` | IMPLEMENTED — proportionate Auto/direct proof accepted at794f080 on six native targets plus race |
| FR-013 | Choose the least costly suitably capable configured target deterministically | 004 | 003, 005, 012 | `specs/004-economical-routing/acceptance.md#suitable-economy-selection` | IMPLEMENTED — specified native scope accepted at3f4a18e on six platforms plus race; live provider qualification remains separate |
| FR-014 | Distinguish Council occupancy, external occupancy, allowance and unknown information | 004 | 002, 012 | `specs/004-economical-routing/acceptance.md#occupancy-and-allowance-states` | IMPLEMENTED — specified native scope accepted at3f4a18e on six platforms plus race; live provider qualification remains separate |
| FR-015 | Use suitable idle capacity without disrupting native chats or double-counting shared accounts | 004 | 003, 006, 012 | `specs/004-economical-routing/acceptance.md#idle-subscription-routing` | IMPLEMENTED — specified native scope accepted at3f4a18e on six platforms plus race; live provider qualification remains separate |
| FR-016 | Preserve observation source/freshness and distinguish unknown, exhausted and expired reports | 004 | 007, 012 | `specs/004-economical-routing/acceptance.md#observation-provenance-and-expiry` | IMPLEMENTED — specified native scope accepted at3f4a18e on six platforms plus race; live provider qualification remains separate |
| FR-017 | Bound retries, routing and escalation by captured task policy and budget | 004 | 005, 006, 012 | `specs/004-economical-routing/acceptance.md#bounded-escalation` | IMPLEMENTED — specified native scope accepted at3f4a18e on six platforms plus race; live provider qualification remains separate |
| FR-018 | Honor explicit account/harness/model pins without silent substitution | 004 | 002, 003, 009–012 | `specs/004-economical-routing/acceptance.md#explicit-target-pins` | IMPLEMENTED — specified native scope accepted at3f4a18e on six platforms plus race; live provider qualification remains separate |
| FR-019 | Require configured paid accounts and applicable budgets for Council-managed paid API paths | 004 | 005, 009–012 | `specs/004-economical-routing/acceptance.md#payment-path-authorization` | PARTIAL — native paid-fallback/guarantee refusal qualified; configured paid API execution remains unsupported |
| FR-020 | Separate reported actual usage, estimates, reference prices and unknowns | 004 | 003, 009–012 | `specs/004-economical-routing/acceptance.md#usage-and-cost-provenance` | IMPLEMENTED — specified native scope accepted at3f4a18e on six platforms plus race; live provider qualification remains separate |
| FR-021 | Connect implementation, necessary review, checks and integration with an explicit next owner/action | 005 | 002, 006, 008, 012 | `specs/005-verified-delivery/acceptance.md#workflow-next-action` | IMPLEMENTED — captured stages/next action accepted at794f080; adoption starts no work and requires initial Run |
| FR-022 | Review the actual candidate and accessible evidence; retain unperformed review/check gaps | 005 | 006, 009–012 | `specs/005-verified-delivery/acceptance.md#independent-review-evidence` | IMPLEMENTED — fresh exact-subject review/evidence and named gaps accepted at794f080; live model inspection remains unperformed |
| FR-023 | Link blocking findings to corrections and affected rechecks without replaying unrelated work | 005 | 006, 007, 012 | `specs/005-verified-delivery/acceptance.md#correction-lineage-and-recheck` | IMPLEMENTED — bounded linked correction/recheck and immutable findings accepted at794f080; no implicit budget refill |
| FR-024 | Bind observed deterministic check receipts to the exact checked change/source | 005 | 002, 006, 012 | `specs/005-verified-delivery/acceptance.md#verifier-receipts-and-staleness` | IMPLEMENTED — actual builtin/owned-command receipts, exact applicability and mutating-check invalidation accepted at794f080 |
| FR-025 | Run independent tasks concurrently within capacity and honor dependencies | 006 | 002, 004, 007, 012 | `specs/006-task-graphs/evidence-matrix.md` | PARTIAL — dev.6 real offline two-context overlap, narrower capacity and one root allowance are implemented; complete006 local/client/package/native acceptance remains open |
| FR-026 | Give followers the accepted predecessor change/base within their actual access | 006 | 005, 009–012 | `specs/006-task-graphs/evidence-matrix.md` | PARTIAL — retained exact file/index inputs, actual fan-in/diamond receiving workspaces and cumulative final delivery are implemented with bounded checks;006 release acceptance remains open |
| FR-027 | Retrieve focused context while preserving every applicable original constraint | 006 | 002, 005, 012 | `specs/006-task-graphs/evidence-matrix.md` | PARTIAL — exact receiver/shared/contributed conditions and complete-original access are implemented; actual controlled reads and bounded limit checks exist, with final006 gates open |
| FR-028 | Label derived summaries, retain provenance/originals and handle context capacity explicitly | 006 | 002, 004, 012 | `specs/006-task-graphs/evidence-matrix.md` | PARTIAL — deterministic labelled digests/provenance and complete-original/ref/prompt/access blockers are implemented; final006 client/native qualification remains open, and provider token capacity stays unknown |
| FR-029 | Retain complete linked task, attempt, review, verifier and change evidence | 002 | 005, 006, 007, 008, 012 | `specs/002-managed-execution/acceptance.md#evidence-cancellation-and-recovery` | IMPLEMENTED — accepted task/attempt/source/candidate and005 review/check/delivery evidence with bounded complete reads; future graph extensions remain separate |
| FR-030 | Propagate only relevant authorized context; peer content creates no new authority | 006 | 002, 004, 005, 009–012 | `specs/006-task-graphs/evidence-matrix.md` | PARTIAL — scoped peer data and explicit receiver/final authority boundaries are implemented with bounded checks; final006 review/client/native acceptance remains open |
| FR-031 | Observe progress/history/evidence without model calls using bounded metadata and explicit full reads | 002 | 006, 008, 012 | `specs/002-managed-execution/acceptance.md#evidence-cancellation-and-recovery` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-032 | Preserve distinct lifecycle meanings; timeouts, responses and partial results do not imply acceptance | 002 | 005, 007, 008, 012 | `specs/002-managed-execution/acceptance.md#evidence-cancellation-and-recovery` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-033 | Pause admission/checkpoint/resume unfinished work and cancel admitted work with honest cleanup | 007 | 002, 006, 008, 012 | `specs/007-recovery-and-retention/acceptance.md#pause-resume-and-cancel` | NOT IMPLEMENTED |
| FR-034 | Apply guidance/policy at defined boundaries and recheck controls before dispatch | 002 | 004, 005, 006, 007, 012 | `specs/002-managed-execution/acceptance.md#preflight-and-destination` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-035 | Reconcile ownership, outcomes and changes before replacement; never infer permission to replay | 007 | 002, 006, 012 | `specs/007-recovery-and-retention/acceptance.md#restart-reconciliation` | NOT IMPLEMENTED |
| FR-036 | Coordinate shared host capacity and preserve cancel/failure precedence; fail closed on uncertain cleanup | 002 | 004, 006, 007, 008, 012 | `specs/002-managed-execution/acceptance.md#evidence-cancellation-and-recovery` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-037 | Settle evidence/process ownership on shutdown and faults without destructive repair or model replay | 002 | 006, 007, 012 | `specs/002-managed-execution/acceptance.md#evidence-cancellation-and-recovery` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-038 | Preserve dirty work and branch; isolate writers and record attributable base/destination | 002 | 005, 006, 007, 012 | `specs/002-managed-execution/acceptance.md#preflight-and-destination` | IMPLEMENTED — feature002 native authoring scope accepted on six targets at f5edd96; later extensions retain separate gates |
| FR-039 | Apply verified local results when authorized, otherwise deliver a precise reviewable candidate | 005 | 002, 006, 008, 012 | `specs/005-verified-delivery/acceptance.md#authorized-local-integration` | IMPLEMENTED — precise review-only candidate or authorized supported text application accepted at794f080; autonomous access alone grants no delivery |
| FR-040 | Revalidate evidence/destination changes and preserve both candidate and user work on conflict | 005 | 006, 007, 012 | `specs/005-verified-delivery/acceptance.md#integration-conflicts-and-freshness` | IMPLEMENTED — exact freshness/conflict controls and retained journal/quarantine accepted at794f080; unsupported application states retain precise candidates/gaps |
| FR-041 | Require explicit scope for external delivery and reuse existing applicable authorization | 005 | 002, 004, 008, 012 | `specs/005-verified-delivery/acceptance.md#external-delivery-scope` | IMPLEMENTED — explicit captured publisher scope and exact retry/fault behavior accepted at794f080; command completion does not attest remote publication |
| FR-042 | Export evidence, back up/restore state and clean reviewed artifacts with explicit scope | 007 | 001, 002, 012 | `specs/007-recovery-and-retention/acceptance.md#export-backup-and-cleanup` | NOT IMPLEMENTED |
| FR-043 | Prioritize composer/Auto and use exact approved Ejupi Labs assets and icons in rendered UI | 001 | 002–012 | `specs/001-workspace-foundation/acceptance.md#brand-and-accessible-flow` | PARTIAL — foundation through005 composer/workflow/evidence views and approved assets accepted; future feature views remain planned |
| FR-044 | Write concise actionable English UI/docs/guidance and verify actual accessible interface states | 001 | 002–012 | `specs/001-workspace-foundation/acceptance.md#brand-and-accessible-flow` | PARTIAL — foundation through005 English/keyboard/errors/360px/render scope accepted, including44 inspected005 captures; future feature views remain planned |
| FR-045 | Share project/task/routing/execution/wait/evidence/control semantics across UI, CLI and MCP | 008 | 001–007, 009–012 | `specs/008-host-integration/acceptance.md#entrypoint-semantic-parity` | PARTIAL — UI/CLI execution, waits, controls and evidence accepted; routing accepted at3f4a18e; MCP remains planned |
| FR-046 | Handle stale reads, lost acknowledgements and repeats without duplicate work or erased UI state | 002 | 001, 006, 007, 008, 012 | `specs/002-managed-execution/acceptance.md#original-instructions-and-guidance` | IMPLEMENTED — task/draft/control/acceptance replay and stale clients accepted on six targets at f5edd96 |
| FR-047 | Qualify full offline journeys/platforms and report actual authorized live adapter evidence separately | 012 | 001–011 | `specs/012-release-qualification/acceptance.md#end-to-end-release-journey` | PARTIAL — foundation and native Codex fake journeys accepted on six targets; complete multi-adapter release remains planned |
| FR-048 | Trace each Specify feature through design/tasks/implementation/independent review/fix/acceptance | 012 | 001–011 | `specs/012-release-qualification/acceptance.md#feature-delivery-gates` | PARTIAL —001–005 Specify/design/implementation/review/fix/native acceptance gates closed;006 implementation under qualification;007–012 planned |

## Per-feature review and fix gates

Each row remains open until its own Specify artifacts are committed before implementation,
meaningful checks pass, independent review is recorded and every blocking finding is corrected
and verified. These are development gates, separate from the product's proportionate per-task
review policy. Slice 012 audits earlier gates; it cannot retroactively excuse a missing review.

| Slice | Planned context | Planned independent review/fix record | Gate status |
| --- | --- | --- | --- |
| 001 | `specs/001-workspace-foundation/` | `specs/001-workspace-foundation/review.md` | ACCEPTED — six native targets at 93c8c25 |
| 002 | `specs/002-managed-execution/` | `specs/002-managed-execution/review.md` | ACCEPTED — six native targets plus race at f5edd96 |
| 003 | `specs/003-claude-interoperability/` | `specs/003-claude-interoperability/review.md` | ACCEPTED — six native targets plus race at ce8aaa7; all six implementation findings closed |
| 004 | `specs/004-economical-routing/` | `specs/004-economical-routing/review.md` | ACCEPTED — six native targets plus race at3f4a18e, run42172669381; all independent corrections closed |
| 005 | `specs/005-verified-delivery/` | `specs/005-verified-delivery/review.md` | ACCEPTED — dev.5/005-v1 at794f080, run42224386878, six native targets plus race; all22 findings and35 matrix rows closed |
| 006 | `specs/006-task-graphs/` | `specs/006-task-graphs/review.md` | IMPLEMENTATION UNDER QUALIFICATION — dev.6/006-v1; bounded offline evidence exists, final local/visual/package/seven-job native acceptance open |
| 007 | `specs/007-recovery-and-retention/` | `specs/007-recovery-and-retention/review.md` | NOT IMPLEMENTED; gate open |
| 008 | `specs/008-host-integration/` | `specs/008-host-integration/review.md` | NOT IMPLEMENTED; gate open |
| 009 | `specs/009-acp-deepseek/` | `specs/009-acp-deepseek/review.md` | NOT IMPLEMENTED; gate open |
| 010 | `specs/010-junie-adapter/` | `specs/010-junie-adapter/review.md` | NOT IMPLEMENTED; gate open |
| 011 | `specs/011-antigravity-adapter/` | `specs/011-antigravity-adapter/review.md` | NOT IMPLEMENTED; gate open |
| 012 | `specs/012-release-qualification/` | `specs/012-release-qualification/review.md` | NOT IMPLEMENTED; gate open |

## Acceptance record requirements

Each accepted scenario names its FR IDs, exact checked change/source, method, terminal outcome,
relevant output or inspected artifacts, reviewer identity/independence and remaining limitations.
Findings identify correction revisions and the specific rerun evidence that closes them.
Cancelled, failed, interrupted, blocked, unperformed and completed work remain distinguishable.
An earlier failed attempt is never rewritten as success after a later correction.

Provider records name harness/interface versions and actual supported capabilities. Controlled
fake protocol coverage, source/documentation inspection and authorized live execution are separate
forms of evidence. Missing telemetry stays unknown; fake routing tests are not proof of measured
cost savings. A broad rebuild instruction does not authorize live provider calls.

Machine-local task records, credentials and private project evidence are never committed to fill
this matrix. They remain under their proper local/vendor ownership; committed acceptance records
contain only reviewable non-sensitive material. Final package inspection verifies `.council/`
and local fixture state are absent from distributable artifacts.
