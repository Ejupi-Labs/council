<!--
Sync Impact Report
Version: unratified template -> 1.0.0
Principles adopted: useful outcomes; proportionate usage; interoperable agents;
explicit autonomy; truthful evidence; simple branded experience; feature-by-feature delivery.
Sections added: product constraints; delivery workflow; governance.
Sections removed: none; placeholders replaced.
Dependent templates are unchanged and read this constitution at execution time.
Deferred constitution fields: none.
-->
# Council Constitution

## Core Principles

### I. Deliver useful programming outcomes

Council MUST help a programmer complete a task with the agents and provider access they already
have. The existing implementation is a functional reference, not the architecture or default
feature list for this rebuild. A provider response, an authored patch, a review, a passed check
and an integrated change MUST remain distinguishable. Task acceptance MUST follow the task's
stated outcome and evidence requirements; a successful process exit alone is insufficient.

### II. Spend proportionately to the work

The default workflow MUST select a suitable smaller or less costly available model when supported
by its capabilities and the task's verification needs. It MUST account for total work, including
planning, context, reviews, retries and escalation, rather than merely the first call. Routine
routing, admission, waiting and status updates MUST NOT require model calls. Escalation MUST have
a recorded reason and remain within the task's declared policy. Explicit user choices take
precedence. Unknown usage, quota, price, model identity or external activity MUST remain unknown.

Context MUST be scoped to the current work. Original instructions and constraints MUST remain
available in full. Summaries and references MUST identify their supporting evidence; they MUST
NOT silently replace authoritative instructions or claim that an agent inspected missing files.

### III. Interoperate through honest capability contracts

Council MUST distinguish provider, account, harness, model and agent session. Adapters MUST declare
what their supported interfaces can actually do, including permissions, cancellation, model
selection, usage reporting and session continuity. Unsupported functionality MUST be explicit.
Existing vendor authentication MUST stay with the vendor. Council MUST NOT inspect or copy vendor
credentials, invent telemetry, silently change vendor-global settings or treat arbitrary assistant prose as
an authorized orchestration command. Cross-agent handoffs MUST identify the actual workspace,
relevant artifacts, constraints, checks and remaining uncertainty.

### IV. Make autonomy a primary operating mode

The product MUST support an explicitly selected autonomous full-access preference, persist that
choice and run authorized local work without repeated per-tool confirmation where the harness
supports it. The onboarding and task interface MUST make the effective mode visible without a
large settings surface. A harness limitation MUST NOT silently downgrade or falsely claim full
access. Permission modes are harness-specific; Council MUST NOT promise a universal OS sandbox.

Autonomy does not erase task scope, user changes or provider limits. Parallel writes MUST be
coordinated through isolated workspaces or explicit ownership. Local integration MUST follow the
task's destination and acceptance policy. External publication and destructive actions require
explicit task scope. An inherited policy can supply that scope only when it names the authorized
action and destination and is captured with the task. No additional confirmation is required
when that authorization already exists.

### V. Preserve evidence and recover without replay

Completed, failed, cancelled, interrupted and blocked work MUST remain distinct. An earlier
attempt's authoritative failure MUST NOT become success or generic cancellation during cleanup.
A later correction may satisfy the task, while the earlier failed attempt and its evidence remain
unchanged and linked to that correction. Unverified
process cleanup MUST stay visible and prevent dispatch that cannot be shown safe to resume.
Persistence failures MUST NOT imply that work never happened or authorize automatic replay.
Original user edits, retained artifacts and accepted terminal records MUST survive recovery.
Usage observations MUST carry their source and freshness; estimates MUST be labelled separately.

### VI. Keep the experience small and recognizably EjupiLabs

The normal path MUST be choosing a project, describing a task and starting it with useful automatic
defaults. Harness connection and exceptional controls MUST NOT become mandatory configuration for
every task. The interface MUST use verified EjupiLabs identity references and approved icons,
retain their notices and support keyboard use, readable contrast and accessible state labels.
Product copy, requirements, specifications and engineering documentation MUST be in English.

### VII. Finish one specified feature before starting the next

For each implementation slice, development MUST use Specify to prepare a feature specification,
design/implementation plan and executable task list before application implementation begins.
Requirements MUST be traceable to acceptance scenarios and verification evidence. Each slice MUST
receive independent review; blocking findings MUST be corrected and verified before implementation
of the next feature begins. Parallel work within the active feature is allowed where ownership
and dependencies are explicit. A large unfinished scaffold is not a completed feature.

## Product Constraints

- This is a clean rebuild on the Git branch `refactor`. Main remains the reference implementation
  until the user requests integration. Spec Kit feature context MUST NOT change this branch.
- Technology is selected from requirements and a documented architecture decision. Rust,
  TypeScript and other suitable options may be evaluated; the legacy stack is not mandatory.
- The first release MUST provide a useful local developer workflow through a UI, CLI and host
  integration, with the target harnesses qualified individually rather than through a universal
  compatibility claim.
- Local collaboration state belongs under `.council/` and MUST be excluded from Git and release
  contents. Machine-local state, vendor credentials and private work evidence are not source assets.
- Development and automated acceptance use local fake agents. A live provider task requires a
  specific user-requested live task; a broad rebuild instruction is not a usage measurement.
- Dependencies MUST have a present purpose, documented provenance and compatible distribution
  terms. Reusing reference code or assets MUST retain applicable attribution and notices.
- Process invocation MUST use explicit executables and argument arrays without an intervening
  shell. Supported Windows behavior is a release requirement.

## Delivery Workflow

1. Maintain product requirements, functional specifications, assumptions and a bounded release
   roadmap before implementing the replacement.
2. Activate one numbered feature using Specify's feature context on `refactor`.
3. Run specification, design/plan and task generation in that order. Record consequential choices
   and expose actual unresolved questions; use reasonable documented defaults for routine choices.
4. Review artifact consistency, then implement the active feature and its meaningful acceptance
   checks. Run the toolchain's required checks; never substitute a mock success for real evidence.
5. Obtain independent review, resolve blocking findings and record validation and remaining limits.
6. Commit the reviewed feature and required release assets, update traceability and proceed to the
   next feature. Documentation approval does not require an extra user permission checkpoint.

## Governance

User instructions take precedence. Amendments MUST record their reason, affected requirements and
any migration or validation impact. Use semantic versioning for this constitution: a major version
for incompatible principles, a minor version for new obligations and a patch for clarifications.
Feature reviews MUST check this constitution and document justified exceptions rather than
silently bypassing it. Architecture and dependency decisions are recorded separately from functional
requirements. Generated Specify templates remain upstream tools; project-specific behavior belongs
in this constitution and the feature artifacts.

**Version**: 1.0.0 | **Ratified**: 2026-11-05 | **Last Amended**: 2026-11-05
