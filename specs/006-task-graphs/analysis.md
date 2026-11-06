# Cross-artifact analysis: Task Graphs

Status: pre-implementation document analysis. No runtime or native qualification is claimed.

The Specify analyze pass read spec, plan, tasks, contracts and the constitution without mutation.
Its report contained34 unique requirements (24 GRA and ten SC),57 unique tasks and34 mapped
traceability rows. There were no duplicate task IDs, unmapped requirements or constitutional
exceptions. The subsequent authorized correction phase made the changes below.

| ID | Severity | Finding | Correction | State |
| --- | --- | --- | --- | --- |
| A006-01 | HIGH | Root capture and early independent execution depended on staged closure/context/materialization tasks placed in later stories. | Moved staged closure to foundation T004 and receiver context/root/materialization/pre-GO checks to T014–T018, before US1 consumers. Renumbered all tasks and all34 mapping rows; preserved57 unique sequential IDs. | Closed by actual dependency-order inspection; final independent consistency review recorded in design-review.md. |
| A006-02 | MEDIUM | Plan document tree listed only one of four normative contracts. | Included graphs.md, operations.md, materialization.md and interface.md, plus the analysis record. | Closed by actual tree inspection. |

Post-correction mechanical inspection reports57 tasks,57 unique IDs,34 requirements,34 trace
rows and zero missing requirement rows. The initial checker needed CRLF-aware row matching;
that tooling correction is not a feature failure or an executable requirement test.

Explicit rebind, all-receiver conditions, graph membership and all public budget ownership
projections were added to the corresponding implementation tasks after design review. All eleven design findings and final foreign consistency reviews are closed in design-review.md
before committing this gate. All implementation tasks remain unchecked
except the document gate once its actual immutable commit exists. Markdown-only analysis does
not repeat the accepted005 runtime test suite or claim006 behavior.
