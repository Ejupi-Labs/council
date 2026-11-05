# Requirements baseline review

Date: 5 November 2026. Branch: `refactor`. Scope: product and development requirements before
replacement implementation.

## Reviewed artifacts

The independent review inspected the vision, 48 functional requirements, functional specification,
design identity, delivery plan, traceability, constitution and branch development instructions.
It checked user priorities, observable acceptance, all FR mappings, feature sequencing, unknown
telemetry, autonomy, local integration, publication scope and fake/live qualification boundaries.

## Findings and corrections

| Finding | Correction | Independent closure |
| --- | --- | --- |
| R1: the delivery plan allowed replacement verification commands without explicitly retaining the required existing entrypoints. | Step 4 now keeps `npm test` and `npm run check` as working compatibility entrypoints, delegating to meaningful checks of the selected toolchain. The development workflow states the same rule. | Re-review verified the corrected text and reported no unresolved blocking findings. |

Earlier document cross-checks also clarified that local integration may be automatic under the
saved task policy, publication authority must enter explicit task scope, a later correction does
not rewrite an earlier failed attempt, and Council cannot guarantee the billing behavior of an
externally configured native harness. The corresponding artifacts contain those clarifications.

## Planning inputs retained

- Review the documented automation feasibility of all five target harnesses before committing the
  architecture; preserve their individually supported capabilities and known limitations.
- Feature 004 must define unknown-cost and equal-cost selection examples rather than pretending
  that unknown values establish a lowest price.

These are feature-design inputs, not evidence that the replacement or its adapters are implemented.
All 48 requirements are mapped in traceability and remain NOT IMPLEMENTED at this baseline.
The detailed machine-local review, checked hashes and closure record are retained under
`.council/refactor/reviews/requirements-review.md`; that private working evidence is excluded from
Git and release artifacts.
