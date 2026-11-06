# Acceptance evidence: Claude Code interoperability

**Status**: Accepted on 2026-11-06 at corrected implementation
`ce8aaa7af66e3e4beb3a043c2e54fe580a25cb04`. Complete local developer gates and all six actual
native platform jobs plus Linux AMD64 race checks passed. Live Codex and Claude qualification
remains unperformed and unqualified.

Feature context is `003-claude-interoperability` on the physical `refactor` branch. The reviewed
specification, plan, research, data model, native contract and 32 tasks were committed before
application changes in `6d8627a`. The preceding feature 002 was accepted at `f5edd96` after all seven
native/race jobs passed in [run 42140818152](https://github.com/Ejupi-Labs/council/actions/runs/42140818152).
That preceding evidence does not qualify this implementation.

## Accepted source and hosted qualification

Corrected implementation `ce8aaa7af66e3e4beb3a043c2e54fe580a25cb04` passed
[run 42153455521](https://github.com/Ejupi-Labs/council/actions/runs/42153455521). Root independently
verified the workflow API result as completed/success and each of its seven jobs as success,
bound to that exact implementation commit. These are actual native runtime and supported race
jobs, not cross-compilation or reuse of the preceding feature's results.

| Qualification job | Job ID | Observed result |
| --- | --- | --- |
| Linux AMD64 race | [127681934661](https://github.com/Ejupi-Labs/council/actions/runs/42153455521/job/127681934661) | Success |
| Windows ARM64 native | [127681934917](https://github.com/Ejupi-Labs/council/actions/runs/42153455521/job/127681934917) | Success |
| macOS AMD64 native | [127681934948](https://github.com/Ejupi-Labs/council/actions/runs/42153455521/job/127681934948) | Success |
| Windows AMD64 native | [127681935050](https://github.com/Ejupi-Labs/council/actions/runs/42153455521/job/127681935050) | Success |
| Linux ARM64 native | [127681935076](https://github.com/Ejupi-Labs/council/actions/runs/42153455521/job/127681935076) | Success |
| macOS ARM64 native | [127681935129](https://github.com/Ejupi-Labs/council/actions/runs/42153455521/job/127681935129) | Success |
| Linux AMD64 native | [127681935159](https://github.com/Ejupi-Labs/council/actions/runs/42153455521/job/127681935159) | Success |

The failed initial implementation/run and the Windows correction proof remain recorded below.
They are correction history, not current qualification blockers. All 21 CLA requirements, eight
success criteria, four stories and 32 tasks have reviewed acceptance coverage in the
[evidence matrix](evidence-matrix.md). The six implementation findings IR-003-01–06, their
corrections and affected rechecks are preserved in [review.md](review.md), separately from the
completed [design review](design-review.md).

## Accepted implementation boundaries

- Optional `claude-code` pins resolve to `claude-native`; omitted Auto retains Codex. Captured
  request pins and historical receipt bytes are not normalized to new defaults.
- Both harnesses share the single managed-native resource. Schema 3 migrates the old claim key
  transactionally while retaining its held attempt and unrelated rows exactly.
- Claude runs through the existing owned-process/source/evidence lifecycle. It receives no
  response-file path. Only an EOF-finalized valid main result can supply a response artifact;
  root exit, complete input/output, verified scoped cleanup and durable evidence remain separate
  required proofs. Useful candidates remain verification pending, with no automatic integration.
- Current execution compatibility is distinct from the narrow known-002/current maintenance Stop
  whitelist. Uncertain Stop diagnostics retain the captured instance and request for an exact
  retry; the replacement owner must never be stopped by replaying an old receipt.
- Native model settings, actual main model, whole-tree model usage, client cost estimates,
  version assumptions and native conversation continuity have distinct meanings. Local fake
  execution supplies no real provider/account/model/usage qualification. Native conversation
  resume remains unsupported; retained Council history or a session identifier does not add it.

## Historical focused local checks

The following actual Windows ARM64 checks are the initial local development checkpoints, before
the hosted Windows privacy finding and its correction. Their timing and scope are retained as
history. The corrected-source local gates and hosted results qualify the accepted implementation.
None of these checks is live provider qualification.

| Check | Observed result |
| --- | --- |
| `go test ./internal/domain ./internal/store -count=1` | Pass: domain 0.068s, store 1.267s, before later observation-boundary additions. |
| Focused model-argument/legacy-receipt regressions | Pass: domain 0.067s, store 0.099s. Structural legacy pins remain replayable; new dispatch rejects option-like model arguments. |
| `go test ./internal/adapters ./internal/discovery ./internal/fixture -count=1` | Pass: adapters 0.101s, discovery 0.077s, fixture 3.951s. Includes parent/session attribution, trailing/duplicate/missing terminal, exact Unicode/CRLF response, numeric/model preview and fixed fake scenarios. |
| Authored TypeScript `--noEmit` and new browser-test syntax checks | Pass. Integrated browser behavior is recorded separately below. |
| Focused engine interoperability cases | Pass in 25.004s after correcting an integration compile error (`Stream.ID()` is a method). Includes decoded result provenance, empty/error/trailing results, shared controls, source and index preservation. |
| Focused CLI interoperability and maintenance cases | Pass in 88.493s. Actual independent mixed-harness clients, captured older-profile Stop with lost reply/replacement, exact retry diagnostics, no-owner/no-bootstrap, populated schema 1/2 upgrades, saved-draft Claude execution and explicit retarget history. |
| Preliminary joint `npm run build` | Pass in 5.822s for the integrated binary and matching embedded UI, before the final adapter correction. Superseded by the final rebuilt gates below. |
| Nine new actual browser cases | Pass: 9/9 in 34.318s. Includes Auto, explicit Claude, unsupported supervised policy, exact pending target, guidance retarget, Stop, controlled telemetry, earlier-owner refusal and both stale-selection delays. |
| Publication-fault, running-deadline and owned-cleanup cases | Pass: store 0.215s, engine 16.166s. Real final-response commit/lost-acknowledgement and precommit faults cannot be laundered by a useful candidate; cleanup and raw/input evidence remain truthful. Includes actual owned descendant Stop, unknown-cleanup retained claim and five-second running deadline. |
| Final omission-provenance/preview correction | Pass: adapters 0.080s. Oversized optional model/version values without modelUsage retain explicit bounded stdout links and remain unknown. Independently rechecked in source. |
| Unsupported source on both harnesses | Pass: engine 1.763s. Sparse-index blockers occur before source registration/native launch and preserve source/index bytes. An initial fixture used Go's Windows `NUL` spelling for Git configuration; corrected to Git's portable `/dev/null` spelling before this pass. |
| Rendered UI and approved identity | Pass: 28 actual screenshots at 1440×1000 and 360×800; nine Tab/Enter/Space focus checks, reduced motion, no horizontal overflow, and all ten assets matched authored/embedded/served/pinned-original SHA-256. All renders inspected by the UI owner; root independently inspected seven selected composer/brand/result/error/Stop/controlled-telemetry images with no additional blocking finding. Source/index bytes remained unchanged. |

Render evidence is retained privately under `.council/refactor/screenshots/003-claude-interoperability/`
with capture timestamp `2026-11-06T02:20:29.485Z`. Controlled observation overlays were visibly labelled
test data and verified absent from retained actual observations after removal. These images and
fixtures are not distributed as native provider evidence. The final rebuilt integrated source
passed the complete developer gates described below.

## Initial implementation developer qualification

Observed on Windows ARM64 with Go 1.27.3, embedded SQLite 3.54.1 (modernc/sqlite 1.61.1),
Node 24.18.1, TypeScript 7.0.2, Playwright 1.62.0 and pinned Chromium 151.0.7922.34, revision 1234:

| Required gate | Observed result |
| --- | --- |
| `npm run build` | Pass, exit 0; rebuilt embedded browser output and native executable for the initial implementation. |
| `npm test` | Pass, exit 0. All Go packages passed; the initial implementation's canonical CLI run took 117.621s. Unchanged packages reused the preceding successful same-source Go results, including engine 116.170s/store 10.109s. All 23 actual browser tests passed in 78.8037102s; the extracted package passed 1/1 in 10.450045s. |
| `npm run check` | Pass, exit 0: native vet, authored TypeScript no-emit and source formatting. |
| Workflow syntax | Actionlint passed for `.github/workflows/verify.yml`. |

The first complete developer run passed all Go packages and 23/23 browser cases, then failed its
package README assertion because it expected the old single-line wording for the unqualified-live
status. Updated that assertion to preserve the same semantic requirement across line wrapping and
the explicit two-connection wording. The affected extracted-package check passed in 10.168s, then
the complete canonical build/test/check sequence passed again. No native behavior was relaxed
to pass it.

The extracted runtime initialized foundation storage with an empty PATH. Managed fake execution
used only the resolved Git installation directory on PATH, excluding Node/npm/Go, and exercised
both Codex and Claude, exact response bytes and hashes, retained evidence after restart and Claude
cancellation. Fixed archive contents, complete notices, reproducible bytes, file/archive checksums
and absence of the developer workspace path passed. No private collaboration state is packaged.

Detailed logs are retained privately as `.council/refactor/003-build.log`, `003-test.log` and
`003-check.log`, with corresponding exit-0 files. The initial corrected failure remains in
`003-test-initial.log` rather than being reported as a pass.

## Hosted finding and correction gate

Initial implementation `20685065371cacea965d2dc41488e809f32332f7` was pushed and qualified in
[run 42151238672](https://github.com/Ejupi-Labs/council/actions/runs/42151238672). Windows ARM64
failed the existing `TestConcurrentBootstrapConvergesAndObserverLossDoesNotStopOwner`: a transient
ordinary Go directory verification handle conflicted with the MAXIMUM_ALLOWED security open used
to normalize first-owner privacy. The resulting sharing error prevented one observer from
converging. The test also registered shutdown too late to clean up a successfully started fixture
after that assertion. The initial run is not a passing feature qualification.

Native Windows ARM64 reproduced the old concurrent test failure 3 times in 30 iterations. A new
deterministic retained-verification-handle case failed with the same sharing error before correction.
Read-only verification now uses checked metadata-only rights with full sharing; normalization keeps
its existing rights and no-child-propagation constraints. A stale normalization sharing failure
requires a fresh complete private-root proof before proceeding. Identity, reparse, hard-link,
owner, DACL and effective OICI inheritance checks remain enforced. Test fixture shutdown is now
registered before concurrent assertions. See [review.md](review.md), IR-003-06 and primary sources.

Focused corrected-source native proof passed: all localstate tests in 2.361s; the original owner
concurrency test repeated 100 times in 6.652s; all owner tests in 0.389s. Independent source review
closed the correction without a production blocker. The first hosted run completed with five
native targets plus race passing and Windows ARM64 failing; it is retained as correction history.

## Corrected-source local developer qualification

The accepted corrected source passed `npm run build`, `npm test` and `npm run check`, all exit 0.
Fresh Go results include CLI 109.196s, engine 97.386s, localstate 3.104s, owner 0.471s, store 3.832s,
evidence 1.086s and workspace 18.807s; unchanged packages reused valid cached results. All 23
actual browser cases passed in 78.2652789s and the extracted package passed 1/1 in 9.7570666s.
Logs are `.council/refactor/003-correction-{build,test,check}.log` with exit-0 files. Embedded UI
output was regenerated and remains unchanged; this correction adds no product setting or vendor call.

The succeeding seven-job hosted run against the exact corrected commit closes the native/race
gate. Independent review, affected corrections, complete developer gates, rendered UI, extracted
package behavior and requirement mapping support acceptance of feature 003's declared offline
native execution scope. Root commits this acceptance and its companion review/task/traceability
reconciliation before beginning the next implementation slice.

Live Codex and Claude provider qualification remains **unqualified**. No actual provider/model
calls, subscription savings, measured allowance or live compatibility are claimed. The native
permission and version assumptions remain documented capabilities, and native session resume
remains unsupported.
