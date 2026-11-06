# Task graph specification review

Status: all seven independent specification findings corrected and closed. Ready for planning; no implementation is present.
Root authored the specification. Core, Native and the platform/UI reviewer inspected it without
editing the artifact. The physical branch remains `refactor` and feature005 acceptance is committed.

| ID | Severity | Finding | Functional clarification | State |
| --- | --- | --- | --- | --- |
| SR006-01 | P2 | Final-node qualification did not explicitly require every graph outcome, including a separate context-only node. | Every declared node is required; group acceptance is conjunctive. Final delivery must include every node, and context-only reachability does not prove file incorporation. GRA001/020, US4.8, SC008. | Closed by the assigned independent reviewer. |
| SR006-02 | P2 | Useful default parallelism was not measurable without configuring slots first. | Default at most two managed workers across eligible native contexts, respecting narrower restrictions; unchanged-preference overlap is required. GRA004, US1.1, SC001. | Closed by the assigned independent reviewer. |
| SR006-03 | P2 | Node Stop did not explicitly cancel its own active work or distinguish waiting descendants from dispatched work. | Cancel the node's own execution, prevent later stages/unlaunched descendants, preserve independent admitted work and historical outcomes. GRA018, US4.7, SC007. | Closed by the assigned independent reviewer. |
| SR006-04 | P2 | In accepted005, local review-only mode can coexist with an explicit external action, so intermediate delivery was not actually forbidden. | Intermediate local and external delivery is forbidden; contradictory declarations reject before execution. Only captured final delivery is eligible. GRA020, US4.6, SC008. | Closed by the assigned independent reviewer. |
| SR006-05 | P2 | Diamond ancestry did not define a downstream refinement versus an unchanged sibling copy. | Proven descendant refinement supersedes its inherited contribution; unchanged copies dedupe, divergent sibling edits block. GRA012, US2.4, SC004. | Closed by the assigned independent reviewer. |
| SR006-06 | P2 | An accepted response-only final result did not define how retained file changes or pure-response graphs complete. | Review-only mixed results can complete separately; requested file delivery remains blocked without cumulative file qualification. Pure responses can complete without a candidate or fake integration. GRA020, US4.5, SC008. | Closed by the assigned independent reviewer. |
| SR006-07 | P2 | Requiring all nodes to qualify before final delivery could include the final node's own delivery-dependent acceptance. | Other required nodes must have current accepted results; the final candidate passes current pre-delivery checks/review with known prior settlement, then required delivery precedes final-node/group acceptance. GRA020. | Closed by Core after inspecting the final sequencing clause. |

The corrections define user-visible behavior, not an implementation algorithm. Existing handoff,
single capacity claim and singular active-worker handling remain unchanged until the design/task
gate is committed. Fake/native/provider qualification will be recorded separately during implementation.
