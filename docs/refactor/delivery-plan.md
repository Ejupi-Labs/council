# Council rebuild delivery plan

Status: features 001–007 are implemented, independently reviewed and accepted for their
documented offline scopes. Feature 007 qualified at `0a963e819ac684d03f5970e118f696eb39d0f497`
in seven native and race jobs in run 42422741975. Metadata-only history 007a is accepted
for its explicitly incomplete scope at `1af4ea1`, with seven passing jobs in run 42434896240.
Full scoped evidence export, useful production backup/restore and reviewed cleanup remain
mandatory before release. Feature 008 is accepted in its documented offline scope while the payload privacy boundary for full
backup awaits clarification; this reorders independent work without accepting or dropping
retention requirements. Its Darwin AMD64 job metadata discrepancy and the prior unexplained Linux ARM64 fixture failure remain recorded for release qualification. Features 009–012 remain planned. The rebuild is incomplete.
See each active feature's acceptance/review records for observed evidence. A later read-only
review and its unresolved findings are recorded in [post-acceptance-review.md](post-acceptance-review.md).

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
numbered feature context without creating or checking out a feature branch. Main remains unchanged until the rebuild is complete and verified; the user has authorized replacing it with the finished refactor. No application runtime, provider
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

Features 001–008, including the 007a metadata-history slice, are accepted in their
documented offline scopes (see the status above and each feature's `acceptance.md`).
The 007 follow-up (scoped retention, backup/restore and reviewed cleanup) and 009–012 are
planned and unaccepted.
The exit descriptions define each slice's acceptance boundary and are not
pass claims. The order is sequential; later slices build on accepted earlier ones and must
preserve their working journeys.

### 001 — Workspace, onboarding and branded foundation

Accepted context: `specs/001-workspace-foundation/`.

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

Accepted context: `specs/002-managed-execution/`.

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

Accepted context: `specs/004-economical-routing/`.

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

Accepted context: `specs/005-verified-delivery/`. Design commit `24b0cf1` preceded implementation.
Checked dev.5/execution/workflow `005-v1` source794f080 passes all six native targets plus Linux
race in run42224386878; its acceptance/review ledgers and35-row evidence matrix close this scope.

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

The accepted path keeps ordinary project/task Start as Auto. A recognized
literal document replacement can use one author and an observed builtin proof; required review
uses a fresh role context, immutable findings and at most one captured correction cycle followed
by affected checks/review. Local checks and no-process application are separate history entries,
not model admissions or measured zero-token calls. Only fresh 005 homes initialize four units;
all existing saved defaults, including two, and retained task budgets/windows stay unchanged.

Review-only delivery is the default. Saved autonomous full access does not supply local-delivery
consent. A prospective saved local scope can authorize compatible text add/replace/delete under
existing parents; unsupported file/index/metadata states and destination conflicts retain a gap
and preserve evidence. External task declarations authorize only their captured command/action/
destination; command completion is separate from remote publication attestation. Plain responses
keep explicit user acceptance, original producer identity and unchanged acknowledgement replay.

Old tasks remain standalone until explicit workflow adoption creates a new version without work
or budget refill. **Run task** starts that adopted version's first author; advance/recheck require
an existing stage and subject. UI/CLI expose current next action/owner and complete retained
evidence without a mandatory account/model/check wizard. Stop and the common clock apply through
required settlement; a retained applied effect cannot erase a failed/cancelled task outcome.

### 006 — Dependent and parallel task graphs with scoped handoffs

Accepted context: `specs/006-task-graphs/`; dev.6/006-v1 qualified offline at `48484fe82e3e62df906bcb999693e4e1f0e94e2c`.
The [evidence matrix](../../specs/006-task-graphs/evidence-matrix.md) records bounded scenarios,
independent review, local commands, hosted jobs and retained limitations.

Optional groups contain up to16 required nodes and64 acyclic file/context dependencies in one
project, with a complete2MiB declaration bound. Default managed capacity is at most two eligible
native contexts, narrowed by graph/account/context restrictions; held migrated legacy claims
remain sole barriers. One captured finite root allowance/common window covers all native
roles/retries. Local scheduling, materialization and derived context add no model admission.
Standalone Auto and existing saved task authority remain intact.

Exact accepted predecessor versions/results supply retained working/index/staged files in the
receiver's real isolated workspace. Context-only edges imply no file incorporation. Compatible
causal fan-in deduplicates shared ancestry and permits proven descendant refinements; divergent
or unverified inputs block before native preparation. Focused stage context retains every
applicable original condition, labelled digests, complete-original access and explicit local
reference/prompt limits without claiming known provider token capacity or peer-created authority.

Separate reviewable results are the default. Only a prospectively designated final node may
apply or externally deliver, after every required result qualifies and file branches reach it
through file edges. Current cumulative graph-root-to-final checks/review precede one application;
intermediate changes are not applied twice. Mixed response/file and pure-response outcomes remain
distinct. External action/destination must be explicit task scope.

Guidance and deliberate Advance operate at captured boundaries. Explicit named rebind creates a
derived dependency_rebind version with unchanged originals/options and retained history, never
fresh allowance or silent completed-work replay. Group Stop cancels admitted work and closes later
admission. Node Stop cancels its own work and blocks unstarted descendants while independent
siblings settle; known-first-GO continuation preserves its captured inputs unless guidance or its
own/group controls change. Restart observes retained state without replay or implicit Advance.

Exit evidence: actual default overlap and narrower-capacity counterexamples; accepted A+B receiving
files/index and diamond/conflict cases; one shared last-unit transaction contest; focused complete
context and missing/oversize access blocks; exact controls/rebind/history/restart; final cumulative
qualification/application and response-only/publication boundaries. Complete actual UI/CLI and
1440px/360px keyboard/focus/reduced-motion/asset review, extracted Git-only native journeys,
independent review/corrections, final build/test/check and all seven exact-source native/race jobs
closed for the accepted offline scope; the documentation-only acceptance commit is required before 007. Live provider qualification remains unperformed.

### 007 — Recovery, checkpoints and reconciliation

Accepted context: `specs/007-recovery-and-retention/`. The directory retains its original name;
the original feature has been split into consecutive slices to make recovery controls deliverable
without treating fixture-only backup as a practical product capability. This slice extends the
fail-closed foundation with durable pause/checkpoint/resume and explicit owner reconciliation
without replay. Preserve completed identities, terminal failures, accepted cancellation, original
edits and retained changes. A replacement attempt is deliberate and links to its predecessor.
Uncertain process cleanup and persistence incidents remain actionable until reconciled; they never
authorize rerunning an agent to reconstruct evidence.

Exit evidence: crash/restart, lost acknowledgements, shutdown during partial output and durable
pause/resume retain accepted records without replaying completed work. Reconciliation includes
both established safe cleanup and unknown cleanup which remains visible and fenced. UI and CLI
provide matching controls and evidence, with independent review and offline acceptance.

### 007 follow-up — Scoped retention, backup and reviewed cleanup

Accepted context: `specs/007a-metadata-history/`; its incomplete read-only metadata history
does not count as complete evidence export, backup, restore, cleanup or completion of FR-042.
The broader `specs/007-scoped-retention/` remains unfinished. Its user-content privacy
boundary is awaiting clarification; to continue independent work, feature 008 is scheduled
next. Before release, complete full scoped evidence export,
project-state backup/restore and artifact cleanup. This follow-up is required, not deferred from
the release. Its design must provide an actually useful production recovery path while honoring the
credential exclusion, not claim that fixture-only provenance solves real backup. Restoring a
verified backup must not silently resume writes. Reviewed cleanup checks ownership and subsequent
edits, preserves unaccepted work and cannot remove another task's files. Backup/export must declare
contents and limits; restore must be verified before effects; cleanup fixtures must preserve
unrelated and subsequently modified artifacts.

### 008 — MCP and host integration with interface parity

Accepted offline context: `specs/008-host-integration/`; see its acceptance record for exact-source evidence and limits.

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
