# 007a metadata history — acceptance evidence

**Status: accepted for the bounded offline metadata-history scope.** Exact
product/test source: `1af4ea199a9b5ab2f19878ee713ea2f0b0706d0a` on `refactor`.
All seven jobs of [hosted run 42434896240](https://github.com/Ejupi-Labs/council/actions/runs/42434896240)
passed on that exact commit. The local `npm test` completed successfully with
the mixed-source provenance described below; `npm run check` also passed. This
record and accompanying status updates are a documentation-only follow-up;
their commit and push are tracked by T038.

The candidate implements a project-scoped metadata-only inventory and explicit
local export. It is incomplete observational history, not evidence backup,
verified restoration, reviewed cleanup, task acceptance proof or executable
authority. Complete retention remains a mandatory later slice. No live provider
or vendor authentication qualification was performed.

## Requirement-to-evidence map

These include focused observations made during implementation and the completed
local suite below. Source evolved as review findings were corrected; the local
suite's two-stage provenance is recorded explicitly. Hosted checks qualify the
exact product/test commit independently of this documentation-only follow-up.

| Requirements | Relevant implementation and focused evidence | Limits |
| --- | --- | --- |
| MH-001, MH-002, MH-006; SC-001, SC-002 | `history_snapshot.go`, `history_read.go`, `history.go`, `ui/src/history.ts`; `TestHistoryCaptureScopesImmutableSnapshotAndReplay`, `TestHistoryCaptureEmptyLostReceiptAndInvalidRelation`, `ui/tests/history-capture.test.mjs`, `ui/tests/history-export.test.mjs`. Two populated fake-task projects, local CLI/UI inspection, fixed notice and omissions, and no history-triggered task start were exercised. | Source trust is the already managed local store under the owner lock. No arbitrary imported database or same-user database tamper attestation is claimed. |
| MH-003, MH-004, MH-005, MH-014; SC-001 | Explicit structured-column projection in `internal/store/history_snapshot.go`; `history_capture_test.go` and `history_capture_boundary_test.go` cover opaque-record sentinels, malformed/cross-project links, exact decimal values, bounds and serialized capture. | SQLite can load physical pages containing excluded columns; the evidence concerns application-level access, not physical page-level isolation. No vendor credential files are read. |
| MH-007, MH-008, MH-009; SC-002, SC-003 | `internal/domain/history_export.go`, `internal/store/history_export.go`, `internal/historyout/`, `ui/tests/history-export.test.mjs`, `ui/tests/history-partial.test.mjs`, and native publisher tests cover preview, canonical digest, target conflicts, retained-handle publication, collisions, partial bytes, output exclusion from an actual package, and unchanged fixture Git status/index/HEAD. | Private output uses the existing v2 state root; the full path is disclosed. A user's home/private root can itself be inside a Git tree. Council does not stage or commit output or include it in release inputs. Windows directory sync may be unavailable and is disclosed; no general power-loss or network-filesystem guarantee is asserted. |
| MH-010, MH-011, MH-012; SC-004 | `history_reconcile*.go`, `history_fence.go`, `history_reconcile_race_test.go`, `history_reconcile_receipt_test.go`, `history_reconcile_actor_test.go`, `history_drain_test.go`, `history_crash_test.go`, and `ui/tests/history-recovery.test.mjs` cover exact receipts, freeze/permit ordering, actor drain and timeout, no-permit proof, linked incidents, independent blockers, lost acknowledgements and real killed-owner boundaries without replay. | A present or possibly committed permit is not positive proof of no publication. Incomplete/corrupt journals, unproven owner chains or unrecorded fences stay unresolved. An incident with a permit cannot currently be positively reconciled as no-publication. |
| MH-013; SC-005 | `ui/tests/history-capture.test.mjs`, `history-export.test.mjs`, `history-cross-interface.test.mjs`, `history-recovery.test.mjs`, `history-delayed.test.mjs`, `history-storage.test.mjs`, and CLI history tests cover 360-pixel keyboard focus, cross-interface exact receipt replay, saved intent, storage failure refusal, stale-response selection/draft isolation and changed-input conflict. | The CLI-originated scenario retained the original CLI acknowledgement in the test and deliberately lost the subsequent UI replay acknowledgement; it did not drop the original CLI transport response. |
| MH-015; SC-006 | Focused fake-owner, Store, native publisher, browser, migration and process-loss tests listed below; independent reviews identified and prompted corrections to notice, scope, incident binding, actor coordination and retry behavior. The final independent backend/UI review reported no remaining blocker; the local suite passed with the provenance recorded below. | The seven hosted jobs passed. The Linux isolated mount-replacement test was skipped because the hosted environment denied the required namespace; it was not executed on the Windows development host. No general live harness/provider capability is inferred. |

## Local full-suite result

`npm test` exited 0: 26 Engine batches, 96 browser tests, and 3 script and
packaging tests passed. The build performed as part of that command succeeded.
`npm run check` also exited 0. The test run began at `1ce72a8`; the only
subsequent change was the browser test path-normalization fix in `1af4ea1`,
committed before browser test discovery and execution. Go and product files
were unchanged between those commits. This is a mixed-source run across its
stages, not a claim that the command began at exact HEAD.

## Hosted qualification

[Run 42434896240](https://github.com/Ejupi-Labs/council/actions/runs/42434896240)
completed successfully at `1af4ea199a9b5ab2f19878ee713ea2f0b0706d0a`:
Linux, macOS and Windows native AMD64 and ARM64 jobs, plus Linux AMD64 native
race checks, all passed. The Linux isolated mount-replacement case was skipped
because the runner denied the required user/mount namespace. Passing jobs do
not qualify that unexecuted mount scenario or live provider behavior.

## Focused evidence observed

- The focused app/HTTP/CLI history tests passed, including strict request and
  query validation, exact frozen CLI replay after a lost acknowledgement and
  recognized old-owner Stop compatibility. The populated v1/v2 CLI schema
  upgrade regression passed with no replay of old work.
- Domain history export tests passed, including 8,192 generated attempts below
  the 4 MiB output ceiling and refusal of 8,193 attempts. The separate 4 MiB
  over-limit rejection branch itself was not directly exercised. Read-only
  analysis independently derived a maximum of 2,087,758 bytes for a compact
  current-schema payload with 2,048 tasks, 8,192 attempts, longest allowed
  counters/phase and all omission reasons; this is an analysis, not an executed
  combined-maximum boundary test.
- Store history tests passed for cross-project/version provenance, exact attempt
  limit, explicit-column/sentinel capture, migration preservation and rollback,
  concurrent freeze/permit ordering, missing journal proof, independent
  incidents and clean versus dirty intermediate owner chains.
- Windows focused native checks passed for symlink replacement before permit and
  injected unsupported directory sync; the latter retained the output and did
  not claim a successful directory observation. The Linux mount test was not
  run locally and was skipped by the hosted Linux environment because namespace
  creation was denied.
- Real owner tests passed for five explicit process-kill boundaries: create
  receipt, permit, exclusive create, partial write and before directory sync.
  They retained the exact output or absence, original receipt, operation and
  unknown-owner fence. Real actor shutdown drain and the actual 20-second drain
  timeout passed; a registered publisher versus reconciliation barrier passed.
- Focused actual-browser history journeys passed for two-project capture,
  cross-interface replay, export, no-publication reconciliation, and session
  storage fault handling; separate focused delayed-response and real partial-file
  tests also passed. The latter restarted an actual owner, observed the partial
  operation in the UI, preserved its file and fence, and inspected the actual
  package and fixture Git state.

## Historical observations and corrections

- Hosted CI for `1ce72a8` failed the Darwin ARM64 browser test because the test
  compared the temporary `/var` spelling against the owner's canonical
  `/private/var` path. Candidate `1af4ea1` changes only the browser test: it
  resolves the actual private root with `realpath` and still verifies the exact
  output basename. The corrected test was subsequently covered by the successful
  exact-source hosted run `42434896240`.
- The completed local `npm test` started on `1ce72a8`. Go and product
  files are unchanged in `1af4ea1`; the corrected browser test was committed
  before the browser test discovery and execution phase. This is a mixed-source
  run across those stages, not a single exact-HEAD run; its exit-0 result and
  test counts are recorded above.
- Hosted run `42432162494` on `03b707b` failed reported Unix jobs, including
  Linux ARM64, at
  `TestHistoryPublisherUnsupportedSyncNeverClaimsDurability`: the assertion
  assumed `NameVerified` was set before the directory-sync hook, while Unix
  records it afterward. Candidate `1ce72a8` corrects the test to assert the
  shared permit, retained bytes/identity, unknown sync and no-replacement
  guarantees; the corrected focused Windows test passed. The Linux mount test
  was skipped by the prior hosted runner. The earlier local full run was
  deliberately interrupted after that hosted failure and is not a passing run.
- The first focused browser export run reported one pass and one failure:
  the test asserted its retained owner acknowledgement before the interception
  had arrived (`007a-browser-focused.log`). The synchronization was corrected;
  the subsequent two-test run passed (`007a-browser-final.log`). This does not
  on its own establish the later full browser-suite or hosted-CI results.
- The first real process-kill run passed four boundaries but the partial-write
  child had already exited before Windows could terminate it. The fixture's
  permanent blocking `select` was replaced with a live timer so the runtime
  cannot terminate it as globally deadlocked; the real parent process kill and
  partial-write checks then passed at all five boundaries.
- The first browser collision assertion expected `target_conflict`, while the
  actual Windows owner conservatively returned `root_unavailable` for a target
  whose private ACL could not be verified. The test now accepts the documented
  platform-specific refusal and still checks the real unrelated bytes and
  owner-mediated no-publication result.
- An initial Engine fixture used the unsupported fake scenario `success`; it
  was corrected to `response`. An initial `npm run check` reported formatting
  in `types.go` and `observe.go`; formatting was corrected and a revised check
  passed. The later candidate `npm run check` also passed, as recorded above.
- Independent review also found missing per-page/export notice, stale project
  display, session-storage failure handling, reverse-interface retry and
  unbound history persistence incidents. Follow-up implementation and focused
  tests were added. Final independent backend/UI review reported no remaining
  blocker. The hosted qualification subsequently passed; this documentation-only
  acceptance follow-up records the result.

## Delivery follow-up

The separately required evidence export, useful production backup and verified
restore, and reviewed cleanup remain mandatory consecutive work. No live
provider qualification or first-release completion is claimed here.

## Sequencing amendment

After this slice was accepted, independent feature 008 was scheduled before the
remaining retention work while the user-content privacy boundary awaits clarification.
Full evidence export, useful backup/verified restore and reviewed cleanup remain
mandatory release gates. This scheduling amendment changes neither the accepted
007a scope nor the recorded evidence.
