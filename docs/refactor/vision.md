# Council rebuild: product vision

Status: requirements baseline for the `refactor` branch. Date: 5 November 2026.

## The problem

A programmer already has access to several coding agents, but routinely uses one while another
subscription or useful model sits unused. Moving work between harnesses takes manual prompts,
context preparation, status checks and result comparison. Choosing a large model for every step
spends allowance on work that a smaller suitable model or a deterministic tool could complete.
Adding more agents can increase coordination cost instead of producing a better result.

## The product

Council is a local workspace for giving a programming task to suitable available agents,
coordinating their work and returning a result with evidence. The programmer connects existing
harnesses, opens a project and describes the task. Council chooses a proportionate workflow,
prepares focused context, uses available accounts and escalates when the work warrants it.
The user can inspect progress, changes, checks and observed usage in one EjupiLabs interface,
or control the same work through the CLI or a connected host.

The motivating example is explicit: use appropriate Claude capacity instead of consuming personal
Codex allowance for every task. This does not require pretending that every subscription exposes
its remaining quota or that Council can observe activity in unrelated vendor chats.

## Product priorities

1. Complete useful programming work across different harnesses and providers.
2. Reduce total unnecessary model work while preserving the task's required quality.
3. Offer useful automatic defaults and a short setup, with autonomy as a primary operating mode.
4. Keep task state and acceptance evidence understandable, including failures and interruptions.
5. Make the UI recognizably EjupiLabs using its real identity and icon assets.

No percentage cost reduction is promised without a measured workload. Language and runtime
selection do not by themselves prove lower model consumption.

## Primary user

A programmer working on local Git projects who already uses one or more coding harnesses and
wants to delegate implementation, investigation or review without managing a multi-agent framework.
The initial product is single-user and local. It can manage several projects and agent connections;
multi-user authorization, hosted tenancy and a cloud control plane are outside the first release.

## Primary journey

1. Open a project and discover or connect installed agents using their existing authentication.
2. Select the autonomous operating preference once; see the effective mode on subsequent tasks.
3. Describe the outcome. Keep automatic agent/model/workflow selection unless an explicit choice
   matters for this task.
4. Let Council route work, prepare relevant context, coordinate changes and run the chosen checks.
5. Receive a clear result: what changed or was learned, what was reviewed and verified, what was
   integrated, how much usage is observed and what remains uncertain.
6. If an agent is unavailable or blocked, follow the permitted fallback or see one actionable
   blocker. Stop, recover or continue explicitly without replaying completed work.

## What is being rebuilt

The reference is Council at Git commit `27822baa3aee663366a870bde7fb1190649d3418`.
Its behavior supplies useful examples and operational lessons. Its module layout, five databases,
multiple inboxes, configuration shape, voting rules and six-mode menu are not a migration target.
The replacement must be implemented from the new specifications rather than re-exporting the old
runtime behind a new UI.

Useful outcomes retained include multi-harness execution, economical routing, review/revision,
parallel task handoffs, local history, process cancellation, retained changes, host integration and
truthful recovery. Weighted ballots, a separate executive reporting board, private-chat control
envelopes and a research leaderboard are not required to deliver the first release. They can be
proposed later through a concrete user need and the same Specify workflow.

## Working defaults and assumptions

- Local UI, CLI and host integration operate on the same task records and lifecycle.
- The target harnesses are Codex, Claude Code, DeepSeek Harness, Junie and Antigravity. Each adapter
  reports its supported interface, tested scope and unsupported capabilities explicitly.
- Reuse vendor-owned login state through supported interfaces; Council does not copy credentials.
- Small-model selection depends on suitability, capability, availability and verification.
  Explicit agent/model choices are honored. Included allowance is preferred when appropriate;
  unknown allowance remains visible as unknown.
- The owner prefers autonomous full access. That preference is persisted and mapped to supported
  harness behavior; it does not authorize unrelated publication or deleting user work.
- Parallel writes use managed isolation. Local integration follows the task's stated destination
  and acceptance policy; the UI distinguishes isolated results from integrated changes.
- Context is focused for the active step. Complete original instructions, results and evidence
  remain available rather than being resent wholesale to every participant.
- No live provider calls are needed to design, implement or run the automated acceptance suite.

## Definition of the first release

The planned release is complete when a programmer can connect supported harnesses, submit a real
task, obtain appropriate automatic routing, follow implementation and review/verification, inspect
changes and usage, stop/recover work and use the same lifecycle from the branded UI, CLI and host
interface. Every planned slice must have its Specify artifacts, implementation, independent review,
corrected blocking findings and acceptance evidence. Simulation is labelled and does not establish
live provider compatibility or actual subscription savings.

Technology selection, storage layout and transport contracts belong in the architecture decision
and feature designs after the functional baseline. Those choices must make the product simpler
to install, operate and maintain; novelty is not an acceptance criterion.
