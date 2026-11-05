---
name: council-evidence-debate
description: Arrange an evidence-based Council discussion with useful research, proposal and opposing review roles. Use for consequential choices or contested assumptions; a routine factual question usually needs direct work.
---

# Evidence-based debate

This skill stays in the connected host. The host chooses participants and collects the result. Worker skills provide lenses; they do not create agents, grant permissions or change decision weights.

For example, a team must decide whether its app should show messages immediately or refresh periodically. A researcher checks the actual need. An advocate proposes an approach. A skeptic checks what could fail. Add a user or constraint reviewer only if that perspective could change the decision. The judge explains the choice and remaining uncertainty.

Establish the decision, the user's goals and the evidence that would distinguish the alternatives. Read available facts instead of asking the human to repeat them. Reserve questions for information or choices that remain necessary. Use [Model map](../council-model-map/SKILL.md) and billing observations when selecting suitable configured aliases. Preserve exact models, supported effort, permissions and subscription protection.

Assign useful lenses through per-agent `skills`:

| Skill | Responsibility |
| --- | --- |
| `council-evidence-researcher` | Check claims, sources and missing facts, including contrary evidence. |
| `council-proposal-advocate` | Develop a feasible proposal and answer actual objections. |
| `council-constructive-skeptic` | Test important assumptions and realistic failure modes. |
| `council-constraint-reviewer` | Check stated limits, costs and operational conditions. |
| `council-user-advocate` | Check whether the person can understand and complete their task. |
| `council-clear-explanations` | Explain the result without losing evidence or changing exact formats. |

Choose the smallest useful team. A focused advisor can settle a narrow uncertainty. Do not require every lens, an objection quota, fixed rounds or a standing panel. State actual per-agent skills rather than placing an ambiguous alias-to-role map only in shared task prose. Room `lead`, `worker` and `advisor` roles retain their existing participation and authority meanings.

For a design suspected of overengineering, a [Lazy Genius](../council-lazy-genius/SKILL.md) reviewer can compare the proposed complexity with a simpler complete alternative. Preserve required protections and resolve material findings within the assigned review; no extra panel is needed merely to use that lens.

Use `council` for independent proposals followed by cross-critique and synthesis. Use `debate` for an explicitly planned exchange across rounds. `assignments` belongs to `parallel`, not these modes. Preview the actual call bound with `council_plan`; use the configured budget and the task's needs. Distinct aliases can share a model, so report whether diversity comes from separate calls, models or providers. Role-play inside one response is not an independently executed panel.

Keep initial positions independent. In critique, address actual claims and evidence, update positions and preserve material dissent. A participant reused as judge must switch to neutral synthesis. A decision can remain inconclusive when evidence is missing. Explain which objections were accepted or rejected and why; agreement alone does not establish correctness.

For execution, use `council_run` and yield through `council_wait`. Waiting occurs in the server. Do not fill that wait with speculative analysis, repeated polls or progress messages. A completed discussion does not expand implementation permissions. In rooms, normal task permissions and `autoExecute` govern execution; selected lenses must not turn later implementation into another debate. Council retains write artifacts for review and never merges them automatically.

Report the result using [Clear explanations](../council-clear-explanations/SKILL.md). State what was decided, why it helps the person, what remains unresolved and the verified status. Preserve full supporting evidence; avoid an extra model pass just to restyle an answer. Workers return findings or use their configured advice/private-chat yield protocols; they do not invoke this host workflow recursively.

For working examples and adoption evidence, read [Collaboration playbooks](../../docs/collaboration-playbooks.md). This is an original Council adaptation informed by [structured design review](https://github.com/sickn33/agentic-awesome-skills/blob/main/skills/multi-agent-brainstorming/SKILL.md), [LifeOS Council](https://github.com/danielmiessler/LifeOS/blob/main/LifeOS/install/skills/Council/SKILL.md) and the [research record](../../docs/research-debate-skills-2026-11-05.md). No external plugin installation is required.
