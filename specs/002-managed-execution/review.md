# Managed execution review and corrections

Date: 2026-11-06. Branch: refactor. Complete local build/test/check and visual acceptance passed.
Hosted run 42140818152 passed all six native platforms and Linux AMD64 race checks at
f5edd96cbe32a1cb78d01d9b1107205c5abd8a00, as recorded in acceptance.md.

Independent agents reviewed execution, process ownership, workspace/evidence, UI, HTTP/CLI,
private state and durable storage. Package authors are not credited as independent reviewers of
their own implementation. Reports and raw diagnostic logs remain under ignored .council/refactor/;
this document preserves the actionable findings and corrections for a fresh checkout.

## Corrected findings

| Finding | Correction and verification |
| --- | --- |
| Native creation preceded durable launch intent | RecordLaunchIntent precedes process.Prepare, including suspended/READY creation; an idempotent guarded recheck precedes Start. Unknown prepared ownership retains its claim/fence. Independent source re-review and native fixtures cover ordering. |
| Root success could conceal incomplete stdin delivery | Join stdin writer completion, retain exact expected/written counts and fail incomplete delivery. Store success independently requires retained InputComplete. Native prefix-reader and full Unicode/BOM/CRLF fixtures distinguish short write from full pipe acceptance. Independent focused recheck passed. |
| Uncertain immutable metadata publication could release dispatch (DURABILITY-001, P1) | Keep uncertainty sticky through preparation/output/response/candidate settlement; globally fence conflicting admission and permit only same-receipt reconciliation. Publish immutable metadata once after sync/hash/close and never downgrade a possibly committed final record. Independent six-case focused suite passed in 0.197s. |
| Cancellation could be falsely classified as storage uncertainty | Return known precommit cancellation distinctly; retain actual Commit/AfterCommit uncertainty and genuine joined file faults. Configure every replacement SQLite connection with FK/FULL/busy-timeout. Store follow-up passed in 0.377s; engine cancellation/lost-ack/shutdown cases passed in 3.755s. |
| Explicit zero/future wait cursor semantics (P2) | Accept nonnegative afterRevision, report changed only for a strictly greater actual revision, and retain bounded timeout semantics. Focused contract tests cover both edges. |
| Claim committed without an acknowledged worker or incident (P2) | Fence the uncertain claim without dispatch/replay, retain exact attempt/capacity and incident. Deterministic before/after-commit hooks cover both rollback and lost acknowledgement. |
| Repeated full hashing made internal chunk reads quadratic (P2) | Use one-pass actual-byte CopyTo internally. Public cached aligned chunk proofs verify the requested range; acceptance bypasses the cache for fresh whole verification. Independent source review and corruption/linear-copy tests passed. |
| Secondary failure diagnostics were dropped (P2) | Retain bounded typed secondary fault metadata while keeping the first ordered failure or Stop and immutable terminal. Focused secondary-fault tests cover later evidence/cleanup errors. |
| Incident/source/candidate IDs lacked supported inspection (P2) | Enrich exact attempt/current-task/wait detail with retained typed metadata and expose the active admission incident; compact lists remain compact. Inspection tests cover base/manifest/candidate and incident facts. |
| Source blockers and preparation deadlines lost their cause (P2) | Retain bounded safe workspace-specific blocker messages; expired admission deadlines settle failed/deadline_exceeded. Actual Git v4 prefix compression and skipHash are supported rather than treated as malformed. |
| Replayed autosave rewound newer acknowledged draft state (P2) | Reconcile saved revisions monotonically, reserve the shared pending save and retain clicked draft identity. Independent browser regression proves clicked revision 2, editor revision 3 and subsequent save revision 4. |
| Accepted response/guidance left the task list stale (P2) | Refresh task summaries after the acknowledgement. Independent browser regression proves immediate completed/current-version state without manual refresh or implicit new execution. |
| Active task list retained an obsolete preparing phase (P2) | Refresh compact summaries on selected-attempt identity/phase transitions and settlement, rather than every output event. The real slow-fake browser regression passed in 7.81s and preserves newer composer/guidance while rows advance through running, cancelling and cancelled. |
| Private file/ACL normalization could affect aliased outside data | Reject multi-link ordinary files before mutation; operate on retained checked handles and avoid recursive Windows DACL propagation into unchecked children. Check exact SQLite companions without truncation/removal. Independent privacy source review and native Windows regressions closed PRIVACY-001. |
| Windows active-owner acquisition requested conflicting directory rights | A retained os.OpenRoot handle conflicts with unnecessary MAXIMUM_ALLOWED/DELETE access. Reuse an already-private directory after strict read-only verification, including effective OICI inheritance; otherwise normalize through checked handles or fail closed. Independent source review closed the inheritance follow-up (PRIVACY-002). The native live-WAL/root/owned-child and inheritance suite passed in 2.449s. |
| Service-stop receipt replay could strand the current owner or stop another | Replay before owner rejection; signal only the exact current owner's successful receipt. Current-owner persistence uncertainty still initiates authorized orderly stop without claiming receipt success. Old-owner replay remains observational. Focused engine/CLI tests cover these distinctions. |
| Linux could recycle a closed mutable target's inode into a replacement | Retain the original physical response handle until import or explicit release. Recheck held/path/open identity, consume reservations atomically, close all success/error/shutdown paths and preserve real Close faults. Wrong-attempt calls cannot consume another reservation. Independent source review and native Windows resource/replacement tests passed; corrected-head Linux/macOS runtime and Linux race jobs then passed in run42140818152. |
| macOS cleanup stopped at a signal-delivery error before proving group absence | Keep the final signal attempt before reap and require actual guardian reap plus a post-reap ESRCH probe. Retain bounded delivery errors separately; retry transient post-reap permission observations within the original deadline and leave persistent inaccessible/live/reused groups unresolved. Independent source/hook-order review closed the correction; all four Unix native targets and Linux race checks passed in run42140818152. |
| Windows Git-v4 fixture depended on developer-global long-path configuration | Configure core.longpaths only in the disposable repository and isolate global/system Git configuration. Keep the 150-character multi-byte prefix test. The fixture passed on native Windows ARM64 under an intentionally longer temporary root. |

## Additional integration corrections

Keep proof-specific process cleanup scope separate from wire labels, join workers/watchers on
idempotent Close, preserve accepted Stop before late success, and retain unknown cleanup instead
of repeatedly creating shutdown receipts for an orphan launched attempt. The absolute termination
watchdog exposes uncertainty without fabricating terminal settlement while evidence is active.
Lifecycle/protocol write failures fence without recursively taking the control mutex.
Prelaunch guard rejection and known process-preparation failures also propagate joined stdout/
stderr Close faults into settlement, so expected cancellation cannot conceal uncertain evidence.

The actual independent-client concurrency proof starts two native CLI processes together, including
continuing-owner bootstrap. Both admissions retain separate request/task/attempt identities and the
same owner. One stays queued without source/launch evidence while the other runs; Stop verifies
settlement before the second preparation claim. Both retain exactly one ordered worker lifecycle.
The focused Windows ARM64 case passed in 25.012s; it is distinct from sequential single-client
capacity tests and concurrent owner-bootstrap-only tests.

Official native exec source was checked for the actual command-scoped permission syntax; the
unsupported interactive approval flag was removed. Unknown account/model/usage remains unknown,
and discovery says native supported/live unqualified. No vendor process was run for this review.

## Plan adjustments

Keep one execution contract but consolidate scheduler/recovery helpers into engine.go, settlement
into results.go, browser story cases into execution.test.mjs and dedicated bytes/failure/version/
autosave-recovery tests. CLI owner behavior is in owner.go and real process integration tests in
execution_integration_test.go. These replace planned filenames rather than duplicate wrappers.
The native ownership, evidence, workspace, store and UI packages retain explicit file ownership.

The WAL/FULL and single immutable-publication change follows a real preparation stack trace and
the bounded local diagnostic described in implementation-decisions.md. Timings are not a universal
speedup claim. Existing unsupported database files are validated before WAL activation. The
abrupt-exit proof tests process-crash recovery, not hardware power-loss behavior.

## Final disposition

Complete local build/test/check, all fourteen browser cases, extracted native packaging and the
inspected execution renders have passed after the active-owner acquisition correction. No blocking
source finding remains. All seven hosted jobs passed against the corrected implementation commit
f5edd96 in run 42140818152. The feature gate is closed; feature 003 may now follow Specify.
Cross-compilation and source inspection cannot replace native runtime evidence. No installed
Codex binary/provider, external account capacity or production live task has been qualified.
