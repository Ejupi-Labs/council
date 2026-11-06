# Implementation review: Claude Code interoperability

**Status**: Closed and accepted at corrected implementation `ce8aaa7`. All six findings have
corrections, independent source rechecks and affected regression evidence. Complete developer
gates and all seven native/race jobs passed in
[run 42153455521](https://github.com/Ejupi-Labs/council/actions/runs/42153455521).

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
| IR-003-06: concurrent first-owner Windows privacy verification could deny the DELETE sharing needed by the no-propagation directory normalizer. | P1 | Hosted Windows ARM64 failed the existing concurrent bootstrap test. Original test reproduced 3 failures in 30 native iterations; a retained verification-handle regression reproduced the same sharing error deterministically before correction. Read-only verification now requests only READ_CONTROL/FILE_READ_ATTRIBUTES and shares read/write/delete, preserving checked identity and full privacy proof. Normalization retains MAXIMUM_ALLOWED and its no-child-propagation behavior. A sharing failure is accepted only after fresh complete privacy verification; other errors remain failures. Focused localstate passed in 2.361s, original owner test passed 100 iterations in 6.652s, all owner tests passed in 0.389s. Independent production-source correction review closed without a blocker. Complete corrected-source build/test/check passed; all seven corrected native/race jobs passed at ce8aaa7. |

The independent engine publication reading found no additional blocker in captured target checks,
Claude's absence of output-file reservation, EOF/root/input/drain/cleanup publication gates or
ordered failure/Stop handling. The maintenance reading found no additional blocker in descriptor
and authenticated metadata-only compatibility checks, absence of owner bootstrap/SQLite access in
Stop, or old receipt replay before the replacement-owner mismatch check. A fresh independent
adapter/domain/discovery reading found only IR-003-05 and rechecked its correction. The final
read-only Specify analysis mapped all 21 CLA requirements, eight success criteria, four stories
and 32 tasks with no Critical, High or Medium artifact gap or constitution conflict. See the
[evidence matrix](evidence-matrix.md) for concrete test references and qualification limits.
The initial integrated build/test/check passed, including 23 browser cases and the independently
extracted package, before IR-003-06. The corrected source also passed complete build/test/check,
23 browser cases and the extracted package, followed by all six actual native platform jobs and
Linux AMD64 race checks against its exact `ce8aaa7` implementation. Live provider qualification
remains unperformed and separate.

The Windows correction uses the documented bidirectional desired-access/sharing rules in
[CreateFileW](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew).
Sharing delete does not grant DELETE or mutation rights; the verification handle requests neither.
READ_CONTROL supports owner/DACL inspection through
[GetSecurityInfo](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-getsecurityinfo).
Keeping MAXIMUM_ALLOWED for normalization preserves the explicit no-child-ACE-propagation rule in
[SetSecurityInfo](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-setsecurityinfo).
This targeted correction does not establish synchronization against arbitrary external security
descriptor changes or a universal OS sandbox.
