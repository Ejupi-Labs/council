---
name: dont-kill-an-ant-with-a-bazooka
description: Route Council work to an appropriately sized model and collaboration pattern to save tokens. Use when choosing agents, effort or advisors, especially for routine edits; preserve explicitly requested models and sufficient verification.
---

# Don't kill an ant with a bazooka

Choose the smallest suitable configured worker before starting work. Judge complexity from uncertainty, consequences, dependencies and verification needs; file size and word count are weak signals. A spelling fix rarely needs a large reasoning model or a committee. A small authentication change may warrant expert review.

Use [Lazy Genius](../council-lazy-genius/SKILL.md) when the implementation itself needs a simplicity review. It challenges unnecessary machinery while retaining the complete required result; this skill instead selects proportionate workers and effort.

For model-specific choices, the host can use the [Model map](../council-model-map/SKILL.md) to compare dated task evidence and supported efforts. Low token prices alone do not establish the lowest verified completion cost; include retries, tool use and review work.

Read `council_billing` for configured tiers, capabilities, cost preferences and quota observations. Unknown prices or limits remain unknown. Subscription allowance is shared with work outside Council and is not equivalent to free, unlimited execution.

Use `council_route_plan` with a task class and the required capabilities. Prefer `economy` and `direct` for a routine, well-understood edit. Preview the selected aliases and reasons. Missing configuration requires an explicit profile choice or configuration; do not guess a model's cost or capability from its name. Preserve the user's exact model and effort when supplied.

Start an accepted selection with `council_route_run`, then yield through one `council_wait`. Do not spend orchestrator turns polling or speculating while a worker runs. Use focused advice or an implementation/review workflow when uncertainty warrants the extra calls. Allow escalation explicitly and retain verification; do not save tokens by clipping evidence or accepting partial work as success.

Routing changes Council's selected workers. It does not change the model of the host conversation or freeze the host's scheduler.

<!-- council-agent-instructions -->
Keep the work proportional to the task. Make the smallest correct change, run relevant verification, and report concrete results and unresolved uncertainty concisely. Preserve complete evidence needed for correctness. Use a configured focused advisor only when an actual uncertainty warrants its token cost. Do not invent prices or quotas, change an explicitly chosen model, expand permissions, or delegate recursively. Explain a capability gap instead of producing a confident incomplete answer.
<!-- /council-agent-instructions -->
