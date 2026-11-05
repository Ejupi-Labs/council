# Council rebuild requirements traceability

Status: feature001 foundation implemented/reviewed on Windows ARM64. FR-001 is implemented for
that qualified platform; other parent requirements remain partial or not implemented as marked.
Date: 5 November 2026.

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

Feature001 evidence exists in its acceptance.md and review.md. Other evidence paths are
**planned and not yet created**; a path alone is not evidence.
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

| Requirement | Acceptance responsibility | Primary slice | Supporting slices | Planned acceptance evidence | Current status |
| --- | --- | --- | --- | --- | --- |
| FR-001 | Open existing projects and reach the composer without transport setup | 001 | 002, 012 | `specs/001-workspace-foundation/acceptance.md#project-opening` | IMPLEMENTED — Windows ARM64; other runtime targets pending |
| FR-002 | Separate harness discovery, connection and verified capability status | 001 | 002, 003, 009–012 | `specs/001-workspace-foundation/acceptance.md#harness-readiness-and-authentication-boundary` | PARTIAL — installation/unknown capability separation; execution qualification pending |
| FR-003 | Reuse vendor-owned authentication without accessing credentials or changing global settings | 001 | 002, 003, 009–012 | `specs/001-workspace-foundation/acceptance.md#harness-readiness-and-authentication-boundary` | PARTIAL — no credential access; native execution authentication reuse pending |
| FR-004 | Keep provider, account, harness and model identities distinct; link shared accounts | 004 | 002, 003, 009–012 | `specs/004-economical-routing/acceptance.md#target-and-account-identities` | NOT IMPLEMENTED |
| FR-005 | Supply all five core lifecycle adapters with truthful optional capabilities and qualification | 012 | 002, 003, 009, 010, 011 | `specs/012-release-qualification/acceptance.md#adapter-capability-matrix` | NOT IMPLEMENTED |
| FR-006 | Keep documented collaboration state outside Git and release artifacts | 001 | 002, 007, 012 | `specs/001-workspace-foundation/acceptance.md#local-state-boundary-and-authority` | IMPLEMENTED — Windows ARM64 foundation; later records repeat exclusion checks |
| FR-007 | Admit tasks with project/task and Auto defaults, preserving resolved choices | 002 | 001, 004, 005, 012 | `specs/002-managed-execution/acceptance.md#task-admission-defaults` | NOT IMPLEMENTED |
| FR-008 | Preserve original task text, constraints and attributable guidance versions | 002 | 006, 007, 012 | `specs/002-managed-execution/acceptance.md#original-instructions-and-guidance` | PARTIAL — exact immutable draft versions; execution/guidance pending |
| FR-009 | Persist and apply scoped autonomous full-access preference without repeat permission prompts | 002 | 001, 003, 009–012 | `specs/002-managed-execution/acceptance.md#effective-execution-policy` | PARTIAL — requested preference saved; effective execution pending |
| FR-010 | Report actual harness policy capabilities and restrictions honestly | 002 | 003, 009–012 | `specs/002-managed-execution/acceptance.md#permission-capabilities-and-limits` | NOT IMPLEMENTED |
| FR-011 | Validate task data and destination before side effects | 002 | 004, 005, 012 | `specs/002-managed-execution/acceptance.md#preflight-and-destination` | NOT IMPLEMENTED |
| FR-012 | Select proportionate workflow stages without mandatory model planning, debate or review | 005 | 002, 004, 012 | `specs/005-verified-delivery/acceptance.md#proportionate-workflow` | NOT IMPLEMENTED |
| FR-013 | Choose the least costly suitably capable configured target deterministically | 004 | 003, 005, 012 | `specs/004-economical-routing/acceptance.md#suitable-economy-selection` | NOT IMPLEMENTED |
| FR-014 | Distinguish Council occupancy, external occupancy, allowance and unknown information | 004 | 002, 012 | `specs/004-economical-routing/acceptance.md#occupancy-and-allowance-states` | NOT IMPLEMENTED |
| FR-015 | Use suitable idle capacity without disrupting native chats or double-counting shared accounts | 004 | 003, 006, 012 | `specs/004-economical-routing/acceptance.md#idle-subscription-routing` | NOT IMPLEMENTED |
| FR-016 | Preserve observation source/freshness and distinguish unknown, exhausted and expired reports | 004 | 007, 012 | `specs/004-economical-routing/acceptance.md#observation-provenance-and-expiry` | NOT IMPLEMENTED |
| FR-017 | Bound retries, routing and escalation by captured task policy and budget | 004 | 005, 006, 012 | `specs/004-economical-routing/acceptance.md#bounded-escalation` | NOT IMPLEMENTED |
| FR-018 | Honor explicit account/harness/model pins without silent substitution | 004 | 002, 003, 009–012 | `specs/004-economical-routing/acceptance.md#explicit-target-pins` | NOT IMPLEMENTED |
| FR-019 | Require configured paid accounts and applicable budgets for Council-managed paid API paths | 004 | 005, 009–012 | `specs/004-economical-routing/acceptance.md#payment-path-authorization` | NOT IMPLEMENTED |
| FR-020 | Separate reported actual usage, estimates, reference prices and unknowns | 004 | 003, 009–012 | `specs/004-economical-routing/acceptance.md#usage-and-cost-provenance` | NOT IMPLEMENTED |
| FR-021 | Connect implementation, necessary review, checks and integration with an explicit next owner/action | 005 | 002, 006, 008, 012 | `specs/005-verified-delivery/acceptance.md#workflow-next-action` | NOT IMPLEMENTED |
| FR-022 | Review the actual candidate and accessible evidence; retain unperformed review/check gaps | 005 | 006, 009–012 | `specs/005-verified-delivery/acceptance.md#independent-review-evidence` | NOT IMPLEMENTED |
| FR-023 | Link blocking findings to corrections and affected rechecks without replaying unrelated work | 005 | 006, 007, 012 | `specs/005-verified-delivery/acceptance.md#correction-lineage-and-recheck` | NOT IMPLEMENTED |
| FR-024 | Bind observed deterministic check receipts to the exact checked change/source | 005 | 002, 006, 012 | `specs/005-verified-delivery/acceptance.md#verifier-receipts-and-staleness` | NOT IMPLEMENTED |
| FR-025 | Run independent tasks concurrently within capacity and honor dependencies | 006 | 002, 004, 007, 012 | `specs/006-task-graphs/acceptance.md#parallelism-and-dependencies` | NOT IMPLEMENTED |
| FR-026 | Give followers the accepted predecessor change/base within their actual access | 006 | 005, 009–012 | `specs/006-task-graphs/acceptance.md#workspace-handoff` | NOT IMPLEMENTED |
| FR-027 | Retrieve focused context while preserving every applicable original constraint | 006 | 002, 005, 012 | `specs/006-task-graphs/acceptance.md#scoped-context-and-constraints` | NOT IMPLEMENTED |
| FR-028 | Label derived summaries, retain provenance/originals and handle context capacity explicitly | 006 | 002, 004, 012 | `specs/006-task-graphs/acceptance.md#derived-context-and-capacity` | NOT IMPLEMENTED |
| FR-029 | Retain complete linked task, attempt, review, verifier and change evidence | 002 | 005, 006, 007, 008, 012 | `specs/002-managed-execution/acceptance.md#complete-linked-evidence` | NOT IMPLEMENTED |
| FR-030 | Propagate only relevant authorized context; peer content creates no new authority | 006 | 002, 004, 005, 009–012 | `specs/006-task-graphs/acceptance.md#context-authorization-boundary` | NOT IMPLEMENTED |
| FR-031 | Observe progress/history/evidence without model calls using bounded metadata and explicit full reads | 002 | 006, 008, 012 | `specs/002-managed-execution/acceptance.md#observation-without-model-calls` | NOT IMPLEMENTED |
| FR-032 | Preserve distinct lifecycle meanings; timeouts, responses and partial results do not imply acceptance | 002 | 005, 007, 008, 012 | `specs/002-managed-execution/acceptance.md#lifecycle-and-terminal-outcomes` | NOT IMPLEMENTED |
| FR-033 | Pause admission/checkpoint/resume unfinished work and cancel admitted work with honest cleanup | 007 | 002, 006, 008, 012 | `specs/007-recovery-and-retention/acceptance.md#pause-resume-and-cancel` | NOT IMPLEMENTED |
| FR-034 | Apply guidance/policy at defined boundaries and recheck controls before dispatch | 002 | 004, 005, 006, 007, 012 | `specs/002-managed-execution/acceptance.md#admission-boundary-revalidation` | NOT IMPLEMENTED |
| FR-035 | Reconcile ownership, outcomes and changes before replacement; never infer permission to replay | 007 | 002, 006, 012 | `specs/007-recovery-and-retention/acceptance.md#restart-reconciliation` | NOT IMPLEMENTED |
| FR-036 | Coordinate shared host capacity and preserve cancel/failure precedence; fail closed on uncertain cleanup | 002 | 004, 006, 007, 008, 012 | `specs/002-managed-execution/acceptance.md#ownership-capacity-and-failure-precedence` | NOT IMPLEMENTED |
| FR-037 | Settle evidence/process ownership on shutdown and faults without destructive repair or model replay | 002 | 006, 007, 012 | `specs/002-managed-execution/acceptance.md#shutdown-and-evidence-faults` | NOT IMPLEMENTED |
| FR-038 | Preserve dirty work and branch; isolate writers and record attributable base/destination | 002 | 005, 006, 007, 012 | `specs/002-managed-execution/acceptance.md#isolation-and-dirty-work` | NOT IMPLEMENTED |
| FR-039 | Apply verified local results when authorized, otherwise deliver a precise reviewable candidate | 005 | 002, 006, 008, 012 | `specs/005-verified-delivery/acceptance.md#authorized-local-integration` | NOT IMPLEMENTED |
| FR-040 | Revalidate evidence/destination changes and preserve both candidate and user work on conflict | 005 | 006, 007, 012 | `specs/005-verified-delivery/acceptance.md#integration-conflicts-and-freshness` | NOT IMPLEMENTED |
| FR-041 | Require explicit scope for external delivery and reuse existing applicable authorization | 005 | 002, 004, 008, 012 | `specs/005-verified-delivery/acceptance.md#external-delivery-scope` | NOT IMPLEMENTED |
| FR-042 | Export evidence, back up/restore state and clean reviewed artifacts with explicit scope | 007 | 001, 002, 012 | `specs/007-recovery-and-retention/acceptance.md#export-backup-and-cleanup` | NOT IMPLEMENTED |
| FR-043 | Prioritize composer/Auto and use exact approved Ejupi Labs assets and icons in rendered UI | 001 | 002–012 | `specs/001-workspace-foundation/acceptance.md#brand-and-accessible-flow` | PARTIAL — branded Auto composer/agents; progress/evidence/usage pending |
| FR-044 | Write concise actionable English UI/docs/guidance and verify actual accessible interface states | 001 | 002–012 | `specs/001-workspace-foundation/acceptance.md#brand-and-accessible-flow` | PARTIAL — foundation copy/keyboard/error states qualified |
| FR-045 | Share project/task/routing/execution/wait/evidence/control semantics across UI, CLI and MCP | 008 | 001–007, 009–012 | `specs/008-host-integration/acceptance.md#entrypoint-semantic-parity` | PARTIAL — UI/CLI preparation parity; execution/MCP pending |
| FR-046 | Handle stale reads, lost acknowledgements and repeats without duplicate work or erased UI state | 002 | 001, 006, 007, 008, 012 | `specs/002-managed-execution/acceptance.md#idempotency-and-stale-clients` | PARTIAL — draft/policy/open replay and stale-client handling; task start pending |
| FR-047 | Qualify full offline journeys/platforms and report actual authorized live adapter evidence separately | 012 | 001–011 | `specs/012-release-qualification/acceptance.md#end-to-end-release-journey` | PARTIAL — foundation offline journey; complete release qualification pending |
| FR-048 | Trace each Specify feature through design/tasks/implementation/independent review/fix/acceptance | 012 | 001–011 | `specs/012-release-qualification/acceptance.md#feature-delivery-gates` | PARTIAL — feature001 specified/reviewed/accepted; later features pending |

## Per-feature review and fix gates

Each row remains open until its own Specify artifacts are committed before implementation,
meaningful checks pass, independent review is recorded and every blocking finding is corrected
and verified. These are development gates, separate from the product's proportionate per-task
review policy. Slice 012 audits earlier gates; it cannot retroactively excuse a missing review.

| Slice | Planned context | Planned independent review/fix record | Gate status |
| --- | --- | --- | --- |
| 001 | `specs/001-workspace-foundation/` | `specs/001-workspace-foundation/review.md` | NOT IMPLEMENTED; gate open |
| 002 | `specs/002-managed-execution/` | `specs/002-managed-execution/review.md` | NOT IMPLEMENTED; gate open |
| 003 | `specs/003-claude-interoperability/` | `specs/003-claude-interoperability/review.md` | NOT IMPLEMENTED; gate open |
| 004 | `specs/004-economical-routing/` | `specs/004-economical-routing/review.md` | NOT IMPLEMENTED; gate open |
| 005 | `specs/005-verified-delivery/` | `specs/005-verified-delivery/review.md` | NOT IMPLEMENTED; gate open |
| 006 | `specs/006-task-graphs/` | `specs/006-task-graphs/review.md` | NOT IMPLEMENTED; gate open |
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
