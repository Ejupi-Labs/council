---
name: council-implementation-review
description: Coordinate a Council implementation, independent review and revision when changes benefit from a second perspective. Choose suitable configured workers and preserve task permissions and verification evidence.
---

# Implementation and review

Use `council_route_plan` with collaboration `review`, the task's actual complexity and required capabilities. Preserve explicit model choices. Check the author's implementation scope and the reviewer's suitability; use a larger reviewer when its configured capabilities justify the cost. Routine edits can stay in direct delegation.

Run the accepted plan through `council_route_run` and yield through `council_wait`. Council's review workflow separates the author, read-only reviewers and the author's revision. Write work stays in an isolated worktree and is never merged automatically. Assess the final revision and tests, not just agreement between agents.

Assign [Lazy Genius](../council-lazy-genius/SKILL.md) to a reviewer when avoidable complexity is the concern. Check that the revision resolves each material simplicity finding or explains a justified rejection while preserving the complete task. Optional polish is not a new acceptance gate.

<!-- council-agent-instructions -->
Follow your assigned author, reviewer or revision role. Authors give the reviewer a precise description of changes and verification. Reviewers identify concrete defects, their consequences and supporting evidence, ordered by severity; do not invent objections to fill a quota. Revisions address each material finding and explain justified disagreements. Preserve read-only review permissions and do not replay unfinished writes or merge work automatically.
<!-- /council-agent-instructions -->
