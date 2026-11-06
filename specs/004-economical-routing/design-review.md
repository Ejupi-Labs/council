# Economical routing design review

**Feature**: `004-economical-routing`
**Branch**: `refactor`
**Status**: Design review complete; implementation may begin after this design is committed

This records the design gate after the independently reviewed requirements in
[spec-review.md](spec-review.md). The feature is not implemented or qualified by these documents.
Feature 003 remains the accepted runtime baseline. Completing this feature will not complete the
full rebuild goal; features 005–012 remain in the release roadmap.

## Review scope and workflow

Specify's actual `setup-plan.ps1` and `setup-tasks.ps1 -Json` resolved logical feature 004 on the
physical `refactor` branch. The latter found research, the data model, contracts and quickstart
and resolved the upstream tasks template. There are no registered extension hooks. Independent
Specify analysis ran after the generated 64-task list existed, using the actual
`check-prerequisites.ps1 -Json -RequireTasks -IncludeTasks` result. Research uses dated primary
documentation; no vendor executable, credential or live provider task was used.

The independent holistic reader reviewed the specification, plan, research, data model and routing
contract. A separate UI/quickstart reader reviewed whether the CLI/API supply enough concrete
information for the intended journeys. The root reviewed the transaction, native creation,
compatibility, budget and bounded-inspection boundaries.

## Findings and corrections

| ID | Severity | Finding | Required correction | State |
| --- | --- | --- | --- | --- |
| D004-H01 | P2 | Economy ordering lacks crossed-rate, bounded decimal and escalation-tier rules. | Use exact bounded Pareto comparison, fixed declared tiers and explicit authority for unknown/incomparable changed targets. | Closed after written correction and independent recheck |
| D004-H02 | P2 | `require_positive` does not define independent dimensions or account bindings. | Require fresh effective occupancy and allowance positives, retain overriding negatives and require immutable binding plus current account link. | Closed after written correction and independent recheck |
| D004-U01 | P2 | Context reads lack a named account-link revision; capacity/usage responses lack concrete shapes. | Freeze wire response fields and bounded/page-scoped usage coverage before dependent clients. | Closed after written correction and independent recheck |
| D004-R01 | P2 | Recovery of an inherited negative must remain possible without clearing a newer denial. | Define correction references and applicable heads precisely; retain immutable history. | Closed after written correction and independent recheck |

Fourteen schema tables are not a finding by themselves: the proposed records serve current
mutable preferences, bounded current controls, immutable history and atomic task authority.
No future paid executor, new native transport or feature-005 verifier is justified by this gate.

Unsupported effective-model or financial guarantees do not need invented configuration fields.
No such structured fields are exposed; strict unknown-member validation rejects them. Existing
literal model selectors retain native semantics; actual-account/payment pins retain their stated
unsupported-capability behavior. Ordinary Start requires no such optional controls.

RTE-014's deadline wording was clarified to distinguish the shared task window from the earlier-of
per-attempt ceiling. This does not grant additional work or change the accepted cumulative policy.

## Specify analysis and completion

The actual prerequisites check resolved the specification, plan, tasks and all supporting
artifacts. Independent analysis mapped all 28 RTE requirements and ten success criteria to the
64 sequential tasks; all four stories have independent checks, implementation and acceptance
checkpoints. No task is unmapped. See the complete requirement/task tables in [tasks.md](tasks.md).
The reader found no remaining dependency, scope or constitution conflict. The separate UI reader
closed the wire-contract finding after rechecking the concrete quickstart examples.

| Metric | Reviewed result |
| --- | --- |
| Functional requirements / success criteria | 28 / 10 |
| Ordered tasks / parallel authoring opportunities | 64 / 19 |
| Requirement and criterion task coverage | 38 of 38 (100%) |
| Unmapped tasks | 0 |
| Independent P2 design findings | 4 corrected and rechecked |
| Remaining Critical / High / Medium findings | 0 / 0 / 0 |

The price rule explicitly uses a nondominated frontier followed by stable order, avoiding a
non-transitive pairwise comparator. Escalation includes either unknown tier and higher or
incomparable prices. Capacity correction reaches both original and later link-inherited retained
references without changing original report bindings. Usage totals select one explicit observation
per attempt, group scopes separately and describe returned attempts only. Exact model previews
remain separate; whole-item byte-budget pagination preserves the high-water cursor and every
excluded row. None of these choices creates measured usage or a financial guarantee.

## Implementation ownership and gate

After this design is committed, assign exclusive lanes as follows:

- Domain/store: `internal/domain/routing*.go`, optional domain execution additions and
  `internal/store/`, including migration, snapshots, budget, admission and proof transactions.
  Publish Go DTOs and transaction signatures before dependent source edits.
- Pure routing/CLI: `internal/routing/` and `internal/cli/`, including catalog, assessment,
  ranking, capacity, bounded usage and strict command mappings. Consume the frozen shared DTOs.
- Root integration: `internal/engine/`, `internal/app/`, `internal/httpapi/`, `internal/owner/`,
  specifically needed adapter integration, manifests and packaging. Serialize shared control and
  native launch changes against the published store and selection signatures.
- UI: `ui/src/` and `ui/tests/`; mirror the frozen contracts and preserve pending intents/newer
  edits. Root performs the serialized final build that regenerates `internal/webui/dist/`.
- Independent reviewers inspect source authored by another lane after implementation; the design
  review does not replace that source, failure-path, browser or native qualification.

T001 remains unchecked until Git confirms this design commit. T002–T011 then establish the
foundation before story activation. No application version, schema, runtime source or embedded
UI asset was changed to create this gate. Final local/native/race validation and all four stories
remain required before feature 005 implementation.
