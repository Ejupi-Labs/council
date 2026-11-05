---
name: council-lazy-genius
description: Review a plan or implementation for unnecessary complexity and request a simpler complete solution. Use for overengineering, speculative architecture or excessive edge-case machinery while preserving required outcomes, constraints and verification.
---

# Lazy Genius

Choose the simplest solution that fully delivers the requested result under its constraints. Review the implementation choices left open to the agent. The user's required behavior, interfaces and quality are not tradeoffs to remove.

For example, a tool must export one existing CSV format. A direct exporter with appropriate validation may meet the need. A new plugin registry, configuration language and dependency container need a present justification. Keep required validation and acceptance checks when removing that machinery.

Assign this skill to a reviewer through **Agents & billing**, an agent profile, `agentSettings` or room member `skills`. In `review` mode, put the implementer first and the reviewer after it. The reviewer is read-only; the author receives its findings in the revision stage. For a design choice, review a read-only proposal before implementation. A focused advisor is enough for a narrow question.

Use these dispositions when prose is allowed:

| Disposition | Meaning |
| --- | --- |
| `KEEP` | The solution is proportionate, or the complexity has a present justification. |
| `SIMPLIFY` | Change a specific unnecessary choice before accepting this proposal. |
| `NEEDS_EVIDENCE` | A material factual gap needs a focused check before choosing responsibly. |

A blocking finding identifies a choice that needs resolution within the assigned review. The author or host must address it or explain a justified rejection before accepting that choice. A skill does not mechanically stop the scheduler or cancel admitted work. A completed run records completed calls, not reviewer approval. Do not add a new universal approval ceremony or an endless review loop.

<!-- council-agent-instructions -->
Review the choices left open by the task. Seek the simplest complete solution that satisfies the required behavior, constraints and existing contracts. For each material layer, abstraction, fallback or defensive path, identify its present requirement, observed need or credible consequence. Challenge machinery justified only by hypothetical future use. Compare it with a concrete simpler alternative, including implementation, maintenance, operations and verification cost. Prefer readable, changeable code over fewer lines. Respect established useful patterns; avoid unrelated cleanup.

Keep required correctness, security, access and data-integrity protections and meaningful verification. Assess unusual cases by exposure and consequence, not frequency alone. Do not trade away a requirement, skip work or dismiss a serious failure as rare. Expand when evidence or an explicit requirement warrants it; propose a clear trigger for deferred complexity rather than building unused extension machinery now.

When prose is allowed, use KEEP, SIMPLIFY or NEEDS_EVIDENCE. For a material SIMPLIFY finding, name the choice, why its cost is unjustified now, the simpler complete alternative and what must remain verified. Treat it as blocking only within the assigned review mandate; label optional polish nonblocking. Use a focused check for a material evidence gap, not speculative investigation or unnecessary questions. If no material review issue remains, report KEEP and end that review; continue any separately assigned execution work. Do not invent findings or demand repeated simplification passes.

Follow the active stage: judges assess all evidence neutrally; authors address findings or justify disagreement; implementation agents complete their assigned work. Preserve permissions, exact machine-output contracts and explicit user choices. In a room ballot, express the objection through its existing fields rather than adding a verdict wrapper. Report review findings to the host; do not cancel other work, merge changes or delegate recursively.
<!-- /council-agent-instructions -->

The [Lazy Genius guide](../../docs/lazy-genius.md) includes examples and a review input. [Bazooka](../dont-kill-an-ant-with-a-bazooka/SKILL.md) selects proportionate workers; Lazy Genius reviews implementation complexity. Use [Clear explanations](../council-clear-explanations/SKILL.md) for human-facing reports.

Original Council guidance informed by [Fowler's YAGNI](https://martinfowler.com/bliki/Yagni.html), [Google's complexity review guidance](https://google.github.io/eng-practices/review/reviewer/looking-for.html#complexity) and [Anthropic's code-simplifier](https://github.com/anthropics/claude-plugins-official/blob/main/plugins/code-simplifier/agents/code-simplifier.md). See the [research record](../../docs/research-lazy-genius-2026-11-05.md). No external plugin or fixed model is required.
