# Council rebuild

This is the English product baseline for a clean rebuild on branch `refactor`. It is an active
development goal, not a claim that the replacement is complete. The reference implementation is
commit `27822baa3aee663366a870bde7fb1190649d3418` on main.

Read these documents in order:

1. [Vision](vision.md): the programmer's problem, priorities and first-release boundary.
2. [Functional requirements](functional-requirements.md): 48 stable, testable requirements.
3. [Functional specification](functional-specification.md): defaults, journeys, state meanings,
   routing, review, integration, recovery and qualification behavior.
4. [Design identity](design-identity.md): verified Ejupi Labs assets, tokens, icon family and notices.
5. [Delivery plan](delivery-plan.md): twelve ordered features and the review/fix gate for each.
6. [Traceability](traceability.md): requirement ownership and planned acceptance evidence.
7. [Development workflow](development-workflow.md): reproducible Specify setup and phase discipline.

The [constitution](../../.specify/memory/constitution.md) records the governing principles. A feature
receives its own specification, design, tasks, implementation, independent review and corrections
before the next feature is implemented. Technology is selected in the first feature's design;
the existing runtime is retained only as a reference until the replacement work begins.
