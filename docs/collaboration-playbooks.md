# Debate roles and clear explanations

Council can give agents different jobs in a discussion and explain the result in language the human can use. The new skills work through existing profile, workflow and room settings. They add guidance, not another execution engine or an automatic rewriting service.

For example, the project manager asks: “Should our app show new messages immediately or refresh periodically?” The team checks the actual need, proposes a solution, tests its weaknesses and explains the choice. The explanation must say whether the choice is only proposed, implemented or tested.

## Choose the useful perspectives

The connected host can use [Evidence-based debate](../skills/council-evidence-debate/SKILL.md) to arrange the discussion. It chooses only the perspectives that could change the decision.

| Assignable worker skill | Responsibility in the example |
| --- | --- |
| [Evidence researcher](../skills/council-evidence-researcher/SKILL.md) | Check how quickly people need messages and what the current app does. |
| [Proposal advocate](../skills/council-proposal-advocate/SKILL.md) | Explain a feasible approach, its benefits and its tradeoffs. |
| [Constructive skeptic](../skills/council-constructive-skeptic/SKILL.md) | Check what happens when a connection stops or messages arrive late. |
| [Constraint reviewer](../skills/council-constraint-reviewer/SKILL.md) | Check the stated cost, reliability and operational requirements. |
| [User advocate](../skills/council-user-advocate/SKILL.md) | Check whether the person understands the message state and can recover from errors. |
| [Clear explanations](../skills/council-clear-explanations/SKILL.md) | Explain the decision and remaining questions without hiding important evidence. |

For implementation complexity, add [Lazy Genius](lazy-genius.md) only when a focused simplicity review would help. It can request a simpler complete design while preserving required behavior and protections. Its review finding does not mechanically stop the scheduler.

A lens is a responsibility, not a permanent personality. A skeptic can agree with a well-supported proposal. A researcher can report that evidence is inconclusive. A participant reused as judge must assess all positions fairly. If a room later assigns implementation, the same agent carries out that task instead of continuing to debate it.

Room member roles still mean `lead`, `worker` or `advisor`. They control participation and authority. Select debate lenses through `skills`; do not replace these room roles with names such as `skeptic`.

Separate agent calls do not necessarily use different models or providers. Report the actual setup. One model role-playing several people is not an independently executed panel. Neither a majority vote nor a model's reputation proves a claim.

## Assign skills

In the dashboard, open **Agents & billing** and select skills for an existing alias. New workflows use a snapshot of those settings. Existing workflows retain their recorded choices. Profiles accept `skills`, workflows accept `agentSettings[alias].skills`, and room members accept `skills`.

The following MCP input assumes `researcher`, `designer` and `challenger` are already configured aliases. Replace them with the exact aliases authorized for your project. The example makes no provider or model substitution.

```json
{
  "mode": "council",
  "task": "Compare immediate message updates with periodic refresh for our app. Use the actual requirements and current behavior. Explain the recommendation, evidence, strongest unresolved objection and next useful check in Italian. Do not implement changes.",
  "agents": ["researcher", "designer", "challenger"],
  "judge": "designer",
  "permission": "read",
  "agentSettings": {
    "researcher": { "skills": ["council-evidence-researcher"] },
    "designer": { "skills": ["council-proposal-advocate", "council-clear-explanations"] },
    "challenger": { "skills": ["council-constructive-skeptic"] }
  }
}
```

Pass the input to `council_plan` first to inspect its stages and call bound. Use `council_run` for authorized execution, then wait through `council_wait`. The host yields while the server works. There is no need for repeated status questions or generated commentary during the wait.

`council` collects independent positions, shares them for cross-critique, then synthesizes a result. `debate` supports a planned exchange across rounds. `parallel` suits separate investigations and is the mode that accepts per-agent `assignments`. A focused advisor is often enough for one uncertain point. No new fixed number of participants, rounds, context size or words is imposed by these skills.

For rooms, normal task permissions and `autoExecute` govern later execution. A discussion does not authorize additional writes or publication. Council retains separate write artifacts and never merges them automatically. The host-only debate skill is not in the assignable worker catalog, so worker prompts cannot launch it recursively.

## Explain the result to the human

[Clear explanations](../skills/council-clear-explanations/SKILL.md) uses the user's supplied clarity rules as its foundation. Keep the requested language and depth. Start with the answer, explain necessary terms and connect causes, actions and results through one early example.

Explain each component's job before its program name or location. A room holds the shared plan. A private chat holds a pair's exchange. An agent performs its assigned work. Those are separate responsibilities even when programs run on the same computer.

Make status claims precise. For a hypothetical change tested only in a Windows fixture, say: “The agent made the change. The Windows checks passed. The patch is ready for review; it has not been merged.” Do not describe this as a published feature or verification in every environment.

Keep exact commands, identifiers, UI labels, quoted errors, numbers and necessary qualifications. If the task requires JSON, plain language belongs only in its permitted text fields. Do not wrap a room ballot or advisor/private-chat request in an explanation. Complete supporting evidence remains available; no automatic model pass rewrites or shortens it.

## Research and reuse

The source review covers popular repositories and actual skill contents. GitHub stars measure repository interest. Skills-directory installs are reported installations, not active users or proof of quality. The [debate report](research-debate-skills-2026-11-05.md) and [writing report](research-clear-writing-2026-11-05.md) preserve dated counts, exact source files, licenses and caveats.

The closest debate match was [multi-agent-brainstorming](https://github.com/sickn33/agentic-awesome-skills/blob/main/skills/multi-agent-brainstorming/SKILL.md). [LifeOS Council](https://github.com/danielmiessler/LifeOS/blob/main/LifeOS/install/skills/Council/SKILL.md) contributed topic-specific discussion and preserved dissent. [wait-what](https://github.com/mattpocock/skills/blob/main/skills/productivity/wait-what/SKILL.md) was the closest writing match; [Humanizer](https://github.com/blader/humanizer/blob/main/SKILL.md) contributed editing cautions. Council uses original, self-contained adaptations. It does not install these projects or copy their fixed panel sizes, output caps, approval ceremonies or model pins.

To install an original skill separately, first review its instructions and license. These optional commands download external skills; they are not required for Council and were not run in this development session:

```sh
npx skills add sickn33/agentic-awesome-skills --skill multi-agent-brainstorming
npx skills add mattpocock/skills --skill wait-what
```

The original `wait-what` is manually invoked in compatible hosts. It references project glossary files when available; it does not bundle the STE standard or establish full compliance. Council's clarity skill preserves the user's language, including Italian, without hard sentence-length checks.
