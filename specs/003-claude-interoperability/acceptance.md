# Acceptance evidence: Claude Code interoperability

**Status**: Implementation, independent review and final local developer gates passed. Hosted
native/race qualification remains pending. This feature is not accepted.

Feature context is `003-claude-interoperability` on the physical `refactor` branch. The reviewed
specification, plan, research, data model, native contract and 32 tasks were committed before
application changes in `6d8627a`. The preceding feature 002 was accepted at `f5edd96` after all seven
native/race jobs passed in [run 42140818152](https://github.com/Ejupi-Labs/council/actions/runs/42140818152).
That preceding evidence does not qualify this implementation.

## Implemented boundaries under verification

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
  execution supplies no real provider/account/model/usage qualification.

## Observed checks so far

The following are actual local Windows ARM64 checks, not live provider qualification:

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

## Final local developer qualification

Observed on Windows ARM64 with Go 1.27.3, embedded SQLite 3.54.1 (modernc/sqlite 1.61.1),
Node 24.18.1, TypeScript 7.0.2, Playwright 1.62.0 and pinned Chromium 151.0.7922.34, revision 1234:

| Required gate | Observed result |
| --- | --- |
| `npm run build` | Pass, exit 0; rebuilt embedded browser output and native executable from final source. |
| `npm test` | Pass, exit 0. All Go packages passed; the final CLI run took 117.621s. Unchanged packages reused the preceding successful same-source Go results, including engine 116.170s/store 10.109s. All 23 actual browser tests passed in 78.8037102s; the extracted package passed 1/1 in 10.450045s. |
| `npm run check` | Pass, exit 0: native vet, authored TypeScript no-emit and source formatting. |
| Workflow syntax | Actionlint passed for `.github/workflows/verify.yml`. |

The first complete developer run passed all Go packages and 23/23 browser cases, then failed its
package README assertion because it expected the old single-line wording for the unqualified-live
status. Updated that assertion to preserve the same semantic requirement across line wrapping and
the explicit two-connection wording. The affected extracted-package check passed, then the complete
canonical build/test/check sequence passed again. No native behavior was relaxed to pass it.

The extracted runtime initialized foundation storage with an empty PATH. Managed fake execution
used only the resolved Git installation directory on PATH, excluding Node/npm/Go, and exercised
both Codex and Claude, exact response bytes and hashes, retained evidence after restart and Claude
cancellation. Fixed archive contents, complete notices, reproducible bytes, file/archive checksums
and absence of the developer workspace path passed. No private collaboration state is packaged.

Detailed logs are retained privately as `.council/refactor/003-build.log`, `003-test.log` and
`003-check.log`, with corresponding exit-0 files. The initial corrected failure remains in
`003-test-initial.log` rather than being reported as a pass.

All six actual native platform jobs plus Linux AMD64 race checks against the implementation
commit, followed by final acceptance reconciliation, remain required. No claim of subscription
savings, actual model calls or live Codex/Claude compatibility is made by this record.
