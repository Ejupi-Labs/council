# Implementation review: Claude Code interoperability

**Status**: Independent source review, affected corrections and final integrated developer gates
closed. Hosted native/race qualification remains open; this is not feature acceptance.

The design review is recorded separately in [design-review.md](design-review.md). Implementation
review covers the same frozen feature and includes independent readers of files outside their
implementation ownership. Static review does not establish runtime or live provider success.

| Finding | Priority | Correction and current evidence |
| --- | --- | --- |
| IR-003-01: option-like model validation before receipt replay could reject an accepted feature-002 input rather than returning its original acknowledgement. | P2 | Retained old structural validation; moved the new argument guard to post-replay capability/launch checks. Legacy exact-receipt and fresh unsafe-selector regressions pass. |
| IR-003-02: malformed present `parent_tool_use_id` could be treated as an unattributed main result, and a trailing recognized main message could carry a conflicting session. | P1 | Native reducer rejects malformed essential attribution and conflicting recognized main sessions. Focused adapter tests pass; unknown future event metadata remains raw evidence without invented identity. |
| IR-003-03: uncertain Stop diagnostics omitted the captured owner ID needed by a new CLI process to retry the original intent. | P2 | Added bounded non-secret exact retry command for transport/malformed/server uncertainty and panic recovery. Ordinary targetless Stop lost-acknowledgement/replacement and acknowledgement-shape regressions pass in the focused CLI batch. |
| IR-003-04: a delayed successful Start or subsequent list refresh could reopen its task after the user selected another retained task. | P2 | Captured project/detail generation and selection at the click, then rechecked after the awaited list refresh. Actual browser regressions cover both delayed phases and preserve newer selection and composer input. All nine new Claude browser tests pass. |
| IR-003-05: optional configured-model/version omissions had a diagnostic but no explicit raw stdout reference when modelUsage was absent. | P2 | Added the exact registered stdout ID inside the existing event budget loop. Independent source recheck closed the finding; no-modelUsage oversized model/version and preview-budget regressions pass (adapters 0.080s). |

The independent engine publication reading found no additional blocker in captured target checks,
Claude's absence of output-file reservation, EOF/root/input/drain/cleanup publication gates or
ordered failure/Stop handling. The maintenance reading found no additional blocker in descriptor
and authenticated metadata-only compatibility checks, absence of owner bootstrap/SQLite access in
Stop, or old receipt replay before the replacement-owner mismatch check. A fresh independent
adapter/domain/discovery reading found only IR-003-05 and rechecked its correction. The final
read-only Specify analysis mapped all 21 CLA requirements, eight success criteria, four stories
and 32 tasks with no Critical, High or Medium artifact gap or constitution conflict. See the
[evidence matrix](evidence-matrix.md) for concrete test references and pending qualification.
Final integrated build/test/check passed, including 23 browser cases and the independently
extracted package. Hosted native/race results are still required.
