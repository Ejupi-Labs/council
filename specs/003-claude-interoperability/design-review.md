# Claude interoperability specification and design review

Date: 2026-11-06. Branch: refactor. Scope: Specify artifacts before application implementation.

Independent specification review and a separate read-only Specify consistency review are complete.
Both used the product baseline and constitution. Raw review notes are retained under ignored
.council/refactor/reviews/003-spec-review.md and 003-design-gate.md; the findings are preserved here
for a fresh checkout. Reviewers made no application edits, vendor calls, tests or builds.

| Finding | Correction and independent recheck |
| --- | --- |
| S003-01, P2: native session continuity was not explicit | CLA-021, US2.6, the native-session entity and SC-005 distinguish observed native session identity and unsupported resume from retained Council history and unrelated personal chats. Recheck passed. |
| DG-003-01, High: research required nonempty response even when the accepted candidate-only path was useful | Research Decision4 now requires a valid native result string and either usable nonempty response or attributable useful authored output, matching the contract/data model and accepted002 rules. Empty response-only work cannot be accepted. Recheck passed. |
| DG-003-02, Medium: the plan could reject every malformed optional telemetry counter as essential protocol | The plan now rejects invalid essential numeric/protocol fields and applies the explicit partial/unknown optional-preview rules to telemetry. Full raw evidence remains available. Recheck passed. |

Specification precision also makes observation time and documented versus observed version/live
qualification explicit. The design gate independently checked 21 CLA requirements, eight success
criteria, four stories and 32 unchecked tasks: all are mapped, with no uncovered requirement,
unmapped task or unresolved placeholder.

The review confirms one global managed-native resource, narrow schema3 claim migration preserving
all prior record/receipt meanings, literal native permission/account/instruction boundaries,
one-shot EOF/error/trailing-output rules, per-message response bounds, requested/configured/actual
model distinctions, scoped native estimates and the exact older-owner Stop whitelist. Dependencies
and separate file ownership are explicit. No constitution exception or additional user decision
is required.

No Critical, High or Medium design finding remains open. This gate permits implementation only
after these artifacts are committed. It does not qualify the new adapter, change the application
version/schema, check an installed account or establish a live provider result. Source review,
full build/test/check, rendered UI and all supported native/race qualification remain later gates.
