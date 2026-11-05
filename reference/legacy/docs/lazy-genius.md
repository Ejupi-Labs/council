# Lazy Genius

Lazy Genius reviews the implementation choices that the user has left open. Its goal is a complete solution with the least justified complexity. Required behavior, constraints and quality stay part of the task.

For example, a tool must export one existing CSV format. The reviewer can challenge a new extension framework when a direct exporter already meets the need. It must preserve appropriate input validation, correct CSV behavior and meaningful acceptance checks. Fewer lines are not an improvement if the result is fragile or harder to understand.

## What it reviews

For a consequential design choice, the reviewer asks what current requirement or evidence justifies its cost. It considers the work to implement, maintain, operate and verify the choice. It looks for a simpler alternative that still meets the complete task.

| Situation | Expected response |
| --- | --- |
| One export format gets a registry, custom configuration language and unused extension hooks. | Request the direct complete exporter; identify what future requirement would justify extension support. |
| Someone removes required atomic saves because crashes are uncommon. | Preserve the data-integrity guarantee and reject the unsafe shortcut. |
| The user explicitly requires independently supplied extensions. | Keep an appropriate extension interface; it serves a current requirement. |
| A shared helper already serves several callers and matches project conventions. | Keep it when dismantling it would add churn or duplication without helping the task. |
| A cache is proposed without an observed problem or a current performance requirement. | Defer the cache without creating unnecessary benchmark work. |
| A current response-time requirement matters, but direct-query latency is unknown. | Request the smallest useful measurement before choosing additional state and invalidation logic. |
| Measurements show the current implementation misses a required response time. | Accept justified complexity when the simpler alternatives cannot meet that requirement. |

These are context-sensitive judgments. There is no fixed line count, file count, probability cutoff or ban on abstractions. A rare event can still violate a critical requirement. Existing guarantees and meaningful tests must remain intact.

Optional polish stays optional. The reviewer stops when the solution is proportionate and the relevant requirements are verified. It does not create findings to fill a quota, start a broad refactor or repeatedly review a satisfactory solution.

## Use it as a reviewer

After updating Council, restart its process to load the new skill catalog. If you run the dashboard through the CLI, keep its owning terminal process open while using it.

Select **Lazy Genius** in **Agents & billing**, or assign `council-lazy-genius` through an existing profile, workflow or room member's `skills`. The skill works with any suitable configured agent. It does not select a model, lower effort, grant permissions or start a background monitor.

This MCP example assumes `implementer` and `simplicity-reviewer` are already configured aliases. Replace them with the exact aliases authorized for your project. The example reviews a proposal in read mode; it does not authorize file changes.

```json
{
  "mode": "review",
  "agents": ["implementer", "simplicity-reviewer"],
  "task": "Propose the smallest complete implementation of the requested CSV export. Review unnecessary abstractions and speculative features. Preserve current format behavior, input validation and required verification. Address each material finding or justify disagreement. Do not modify files.",
  "permission": "read",
  "agentSettings": {
    "simplicity-reviewer": {
      "skills": ["council-lazy-genius", "council-clear-explanations"]
    }
  }
}
```

Preview the input with `council_plan`. Use `council_run` for the authorized task, then yield through `council_wait`. In review mode, the first alias authors the proposal, the reviewer inspects it read-only, and the author receives the findings in its revision stage. Normal write workflows use isolated worktrees and retain patches for review; Council never merges automatically.

## What “blocking” means

When prose is allowed, the reviewer uses:

- `KEEP`: the design is proportionate or its complexity is justified.
- `SIMPLIFY`: a named unnecessary choice needs correction before accepting this proposal.
- `NEEDS_EVIDENCE`: a material gap needs a specific useful check.

A material finding explains the choice, its unnecessary cost, a simpler complete alternative and the verification that must remain. A deferred feature can have a concrete trigger for reconsideration, such as a second required export format. The author or host resolves the finding within the assigned review mandate by addressing it or giving a justified rejection.

The skill provides review instructions, not a scheduler lock. It does not pause queued tasks, cancel running work or add a publication gate. A completed workflow says its calls completed; the host still checks the revision and unresolved findings. In rooms, existing ballot fields express the objection and normal approval rules govern the next step. An exact JSON or code contract takes precedence over the prose disposition format.

When the agent later acts as judge, it assesses evidence fairly. When assigned implementation, it completes that work. The skill does not keep an agent opposing every subsequent task.

## Related skills and sources

[Bazooka](../skills/dont-kill-an-ant-with-a-bazooka/SKILL.md) chooses proportionate models and collaboration. [Constructive skeptic](../skills/council-constructive-skeptic/SKILL.md) challenges important claims more broadly. [Constraint reviewer](../skills/council-constraint-reviewer/SKILL.md) checks the design against stated requirements. Lazy Genius specifically examines whether added implementation machinery earns its present cost.

The [research record](research-lazy-genius-2026-11-05.md) links the primary references and reuse terms. Council uses an original adaptation; it does not install another simplifier, force an expensive model or run an automatic rewriting pass after every edit.
