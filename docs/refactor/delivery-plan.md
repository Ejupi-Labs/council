# Council rebuild delivery plan

Status: features 001–003 are implemented, independently reviewed and accepted. Feature 001 has
six-platform native qualification; features 002–003 also have Linux race qualification.
Feature 003 qualified at `ce8aaa7` in
[run 42153455521](https://github.com/Ejupi-Labs/council/actions/runs/42153455521).
Features 004–012 remain planned.
Date: 6 November 2026.
See each active feature's acceptance/review records for observed evidence.

## Release boundary

This plan delivers a local, single-user developer product through an Ejupi Labs UI, CLI and MCP
host integration. The first release covers Codex, Claude Code, DeepSeek Harness, Junie and
Antigravity through individually declared and tested adapter capabilities. It includes economical
selection of suitable targets, implementation, proportionate review and verification, authorized
local integration, controlled parallel work and recovery with retained evidence.

The source of product scope is [the vision](vision.md), [functional requirements](functional-requirements.md),
[functional specification](functional-specification.md), [design identity](design-identity.md) and
[constitution](../../.specify/memory/constitution.md). The reference implementation is Git commit
`27822baa3aee663366a870bde7fb1190649d3418`. It supplies examples and lessons, not a module layout,
storage migration target or checklist of legacy features to reproduce.

All work stays on the branch named exactly `refactor`. Spec Kit feature activation must select a
numbered feature context without creating or checking out a feature branch. Main remains the
reference implementation until the user requests integration. No application runtime, provider
call or rebuilt capability is established by these planning documents.

Feature001 selected Go1.27.3 with an embedded authored TypeScript UI, after comparing suitable
options in its research.md. Subsequent consequential changes must compare suitable options against local installation, Windows process control,
shared UI/CLI/host semantics, accessible UI, persistence, packaging and maintenance requirements.
The legacy implementation language, database layout and dependency policy are inputs to evaluate,
not an automatic architecture choice. Every selected dependency must have a concrete purpose,
compatible distribution terms and recorded provenance.

## Required delivery gate for every slice

The requirements baseline and this bounded roadmap are committed before replacement implementation
begins. A feature is then activated one at a time. Only the active feature receives detailed
Specify artifacts; the directory names below reserve future scope and do not authorize generating
twelve specifications or an unfinished application scaffold in advance.

Every slice follows the same sequence:

1. Use Specify to write an English `spec.md` with its FR IDs, observable acceptance scenarios,
   bounded scope and explicit limitations. Reconcile it with the requirements and constitution.
2. Write the design/implementation `plan.md`, contracts or other necessary design artifacts and
   consequential decisions. Then generate dependency-ordered executable `tasks.md` entries.
   Commit these reviewable documents before implementing that feature.
3. Implement the active slice, including its working UI and CLI path where applicable. Use local
   fake agents for automated acceptance. Do not call live agents under this broad rebuild task.
4. Run meaningful acceptance checks and the selected toolchain's required checks. While the current
   repository commands apply, this includes `npm test` and `npm run check`; source changes also
   require `npm run build` before staging built `dist/` and `schemas/`. A replacement toolchain must
   document equivalent required checks and shipped entrypoint generation, while keeping the
   `npm test` and `npm run check` compatibility entrypoints working as required by `AGENTS.md`.
5. Obtain independent review of the actual change, acceptance evidence and constitutional
   compliance. Record inspected revisions, findings, unperformed checks and actual outcomes in
   `review.md`. Independence is required for development of every slice; the product itself uses
   review proportionately to a submitted task's needs.
6. Correct every blocking finding, rerun affected checks against the corrected change and obtain
   the necessary independent re-review. Keep the original findings and link their resolutions;
   a ticked task list or successful agent exit is insufficient acceptance.
7. Record terminal acceptance evidence in `acceptance.md`, update [traceability](traceability.md),
   and commit the reviewed feature with required generated release assets and notices. Begin the
   next slice only after this gate is closed. No additional user confirmation is introduced for
   already authorized development or documentation work.

Parallel implementation is allowed only within the active feature, with explicit ownership and
dependencies. A blocking unresolved review, missing required evidence or unfinished prerequisite
keeps that feature open. Scope changes update the baseline and affected feature artifacts rather
than silently deferring a required behavior.

## Ordered first-release slices

Every slice below has status **NOT IMPLEMENTED**. Its exit describes required future evidence,
not a result already achieved. The order is sequential; later slices build on accepted earlier
ones and must preserve their working journeys.

### 001 — Workspace, onboarding and branded foundation

Planned context: `specs/001-workspace-foundation/`.

Deliver a launchable local UI and CLI that open an existing project, retain a task draft and make
the selected project and setup status clear. The main screen prioritizes the composer, recent
work, Agents and Preferences. Use exact approved Ejupi Labs assets and generic icons, the recorded
palette, English copy, accessible control names and visible keyboard focus. Pin selected assets
with source, commit, path, hash and retained notices. Do not invent a Council logo or borrow another
product's app tile.

Provide harness discovery without model calls, separate installed/connected/verified capability
states, supported user-facing setup guidance, a small preference surface and persistence of the
selected autonomous policy. Reuse vendor-owned authentication paths without inspecting credentials
or changing vendor-global configuration. Define `.council/` storage scope and exclude local state
from Git and release artifacts. Record the initial architecture and installation decisions here.

Exit evidence: fresh setup and project paths containing spaces/Unicode work in UI and CLI;
drafts and policy survive reopening; missing prerequisites and unverified harnesses are labelled;
rendered screens match the approved identity; keyboard, focus and responsive composer checks pass.
The composer states clearly that managed execution arrives in 002. This slice is useful for
workspace/setup preparation and must not display a simulated task as a completed real run.

### 002 — Managed task execution through stable native Codex

Planned context: `specs/002-managed-execution/`.

Make a bounded task executable and inspectable from UI and CLI using Codex's supported stable
native interface. Preserve original instructions and versioned guidance, record effective
provider/account/harness/model identity and policy, validate inputs and destination before effects,
and run writes in managed isolation that preserves dirty work and the user's selected branch.
Honor the saved full-access preference only through supported harness behavior and show limits
truthfully. Include an explicitly labelled fake mode for deterministic offline development.

Establish the execution foundation now: stable task/attempt/operation identities, idempotent
admission, durable ownership, capacity coordination between local hosts, serialized state changes,
clear terminal outcomes, complete linked evidence and process ownership. Recheck current guidance,
policy and cancellation before dispatch. Cancellation remains visible through cleanup; accepted
cancellation and primary failures survive late success. A persistence fault, uncertain cleanup or
unreconciled owner prevents conflicting dispatch and automatic replay. Restart can expose a
retained interrupted incident without claiming safe resumption. These controls cannot wait for 007.

Exit evidence: an offline Codex fixture completes a bounded task with observed evidence; a failed
attempt with partial changes remains failed; duplicate starts do not duplicate work; two hosts
respect shared admission; late results cannot erase cancellation/failure; shutdown and injected
ownership/evidence faults retain useful output and fail closed. UI and CLI show the same identities,
results and cleanup state. No real Codex call is required for this gate.

### 003 — Claude Code interoperability

Accepted context: `specs/003-claude-interoperability/`. Reviewed design was committed before
application implementation; all seven native/race jobs passed at `ce8aaa7`. Its acceptance/review
records retain the earlier Windows failure, deterministic correction proof and live limitations.

Add Claude Code through its supported interface to the existing lifecycle, with explicit
permission, model-selection, result, usage, cancellation and continuity capabilities. Use existing
vendor authentication and preserve native limitations. Keep provider prose as evidence; it cannot
become an orchestration command or new authorization. Expose this target through the working
Agents, task composer and CLI without adding mandatory transport configuration.

Exit evidence: controlled Claude fixtures exercise task execution, errors, partial output,
cancellation, retained evidence and declared unsupported capabilities. The adapter's permission
mapping and version/interface assumptions are documented. Fake protocol results and any separately
authorized live qualification have distinct status; absent live testing is explicitly unperformed.

### 004 — Economical routing, account capacity and observed usage

Planned context: `specs/004-economical-routing/`.

Add deterministic target selection using suitability, available capability, explicit pins, account
preferences, declared budgets and payment policy. Prefer a smaller suitable model or appropriate
included subscription capacity when supported. Distinguish provider, account, harness and model;
link shared account capacity explicitly. Differentiate Council occupancy, sourced external reports,
allowance, exhausted information and unknowns, with timestamps and expiry rules.

Use the accepted Codex and Claude adapters to demonstrate the motivating background-routing
journey without touching the user's unrelated native conversation. Bound permitted retries and
escalation, preserve explicit target pins and require a configured account plus applicable budget
for a Council-managed paid API path. Show reported usage, estimates, reference prices and unknown
values separately. Routing and routine observation must make no model call.

Exit evidence: fixtures select suitable Claude capacity while Codex is reported busy, do not count
model aliases as additional accounts, honor pins and exhausted observations, reject unconfigured
paid fallback and show unknown telemetry honestly. A routing receipt explains the actual choice
and budget; no percentage savings claim is inferred from fake calls or call counts.

### 005 — Review, correction, verifiers and authorized local integration

Planned context: `specs/005-verified-delivery/`.

Connect the task outcome to a proportionate executable workflow: implementation, independent
review when needed, correction, observed deterministic checks and delivery. Review receives the
actual candidate and relevant evidence with access suited to its checks. Findings create linked
correction work, and only affected acceptance evidence is invalidated and rerun. Record the exact
checked source/change and distinguish observed verifier receipts from agent claims.

Integrate a verified result into the agreed local destination when the captured task policy
authorizes it. Otherwise retain an immediately reviewable candidate and state the remaining
acceptance action. Revalidate destination changes, retain both candidate and user work on conflict,
and report actual integration status. External publication or deployment requires explicit scoped
authorization; existing applicable authorization is reused without another permission loop.

Exit evidence: a trivial task avoids needless model review; a consequential fixture traverses
implementation, review, a blocking finding, correction, affected verification and authorized local
integration. Another fixture retains a review-only candidate. Stale checks, inaccessible review
evidence and changed destinations prevent false acceptance or overwrite. Failed attempts remain
visible in the corrected task's lineage.

### 006 — Dependent and parallel task graphs with scoped handoffs

Planned context: `specs/006-task-graphs/`.

Run independent work concurrently within the existing ownership and account capacity controls.
Honor explicit dependencies and block followers of failed or unverified prerequisites while
admitted siblings settle. Handoffs identify the accepted predecessor base/change and make it
available in the follower's actual workspace; a path or prose summary does not prove availability.

Assemble focused stage context with original constraints, scoped retrieval and provenance-linked
summaries. Preserve full originals and make context limits explicit. Propagate only authorized
relevant evidence. Apply guidance and policy updates at admission boundaries, and support stopping
new graph admission while in-flight work settles.

Exit evidence: two independent fixtures overlap, a dependent fixture starts only after both
required predecessors qualify, a failed branch retains sibling evidence, and a follower inspects
the intended predecessor files. Context tests retain every applicable constraint, retrieve cited
originals, expose insufficient capacity and reject peer-created payment/publication authority.

### 007 — Recovery, checkpoints, backup and reviewed cleanup

Planned context: `specs/007-recovery-and-retention/`.

Extend the fail-closed foundation with supported durable pause/checkpoint/resume, explicit owner
reconciliation and recovery without replay. Preserve completed identities, terminal failures,
accepted cancellation, original edits and retained changes. A replacement attempt is deliberate
and links to its predecessor. Uncertain process cleanup and persistence incidents remain actionable
until reconciled; they never authorize rerunning an agent to reconstruct evidence.

Provide documented scoped evidence export, project-state backup/restore and artifact cleanup.
Restoring a verified backup must not silently resume writes. Reviewed cleanup checks ownership and
subsequent edits, preserves unaccepted work and cannot remove another task's files.

Exit evidence: crash/restart, lost acknowledgements, shutdown during partial output and durable
pause/resume retain accepted records without replaying completed work. Backup/export declare their
contents and limits; restore is verified before dispatch; cleanup fixtures preserve unrelated and
subsequently modified artifacts. Unknown cleanup stays visible and blocks conflicting work.

### 008 — MCP and host integration with interface parity

Planned context: `specs/008-host-integration/`.

Expose the accepted project/task, routing, execution, waiting, full evidence and control semantics
through MCP and documented host setup. All entrypoints use the same records and lifecycle; no
external phase plan is needed to determine the actual next owner/action. Keep MCP stdout
protocol-only and CLI diagnostics separate from machine-readable results. Bound routine metadata
and provide explicit complete evidence reads.

Exit evidence: offline interoperability tests create a task in each entrypoint and inspect/control
it through the others with identical identities and outcomes. Wait timeouts preserve active work;
repeated operations are idempotent; protocol validation and stale-client tests pass. UI/CLI parity
has grown throughout earlier slices; this gate adds and verifies host parity rather than deferring
the UI or CLI until now.

### 009 — ACP v1 and DeepSeek Harness

Planned context: `specs/009-acp-deepseek/`.

Implement an explicitly versioned ACP v1 capability contract and the DeepSeek Harness adapter
using a documented supported interface. Map task execution, model and permission controls, file
access, evidence, errors and cancellation honestly. Unsupported optional capabilities must be
visible before a stage is selected; neither ACP nor a preference implies a universal OS sandbox.

Exit evidence: deterministic ACP contract fixtures and DeepSeek adapter fixtures cover execution,
failure, cancellation, retained evidence and permission/file-service boundaries. Version negotiation
and incompatible capability requests fail clearly. Actual supported versions, fake coverage and
separately authorized live qualification are recorded independently.

### 010 — Junie adapter

Planned context: `specs/010-junie-adapter/`.

Add Junie through its verified supported interface to the same task, policy, routing, graph,
evidence and control contracts. Reuse its existing user authentication. Declare installation,
headless/background, model-selection, tool-access, usage and cancellation limits instead of
inventing protocol or telemetry support.

Exit evidence: controlled Junie lifecycle/error/cancellation fixtures pass through UI, CLI and host
semantics, with incompatible stages blocked before launch. If no supported interface can satisfy
the required core lifecycle, record the concrete blocker and keep this slice open; do not substitute
a decorative connection card or unsupported compatibility claim. Live qualification is separately
reported and requires a specific user-requested task.

### 011 — Antigravity adapter

Planned context: `specs/011-antigravity-adapter/`.

Add Antigravity through its verified supported interface under the same capability and evidence
rules. Preserve native account boundaries and the user's ongoing conversations. Declare actual
automation, model, permission, usage, cancellation and continuity support.

Exit evidence: controlled Antigravity lifecycle/error/cancellation fixtures pass through the shared
entrypoints and preserve retained results. Unsupported optional stages have actionable capability
errors. A missing supported core interface is a release blocker rather than a simulated adapter
success; fake coverage and authorized live qualification remain separately labelled.

### 012 — Packaging and release qualification

Planned context: `specs/012-release-qualification/`.

Qualify an installable release and its documented platform support. Exercise the complete useful
programmer journey offline: fresh setup, two configured targets, suitable idle-account routing,
implementation, necessary review/correction, deterministic checks, authorized integration and
restart/evidence inspection. Include all five adapters in a capability/version matrix and report
their actual automated and authorized live test scope separately.

Audit the actual rendered UI for brand fidelity, keyboard operation, focus, contrast, non-colour
state cues, reduced motion, coarse-pointer use and narrow layouts. Exercise long evidence reads,
concurrent hosts, cancellation/restart and sustained local operation. Verify generated entrypoints,
licenses/notices and package exclusions for `.council/`, fixture state, credentials and private work.

Exit evidence: reproducible terminal results and inspected UI artifacts support every claimed
platform and acceptance scenario; the FR matrix and all twelve independent review gates are closed
with no unresolved blocking findings. Unavailable runners, fake calls and unperformed live tests
are never counted as passes. Release notes state supported interfaces and remaining limitations;
the release cannot claim an untested provider capability or measured subscription saving.

## Evidence and release claims

The planned feature directories contain `spec.md`, `plan.md`, `tasks.md`, `review.md` and
`acceptance.md` once activated. Other design artifacts are added only when that feature needs them.
`review.md` preserves findings, correction links and independent re-review outcomes.
`acceptance.md` records scenario IDs/FRs, exact checked revision or artifact identity, commands or
inspection method, terminal outcome, relevant output, fake/live classification and unresolved limits.
Machine-local execution evidence stays under `.council/`; committed records contain only the
reviewable non-sensitive evidence needed to substantiate the feature.

The first release is bounded by these twelve useful slices, not by matching the old feature count.
Weighted ballots, universal mode/catalog menus, multiple inbox systems and recursive agent
organizations are outside this plan. Required execution integrity, full evidence, policy boundaries
and accessibility are built into the affected slices and requalified in 012; they are not deferred
to a final hardening phase.
