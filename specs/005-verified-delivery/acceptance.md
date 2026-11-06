# Verified delivery qualification

Status: hosted failures corrected and independently reviewed; corrected-source local release gate passed. New hosted qualification remains required. Feature 005 is **not accepted**.
The immutable design gate is `24b0cf1`. The physical development branch remains `refactor`;
`main` remains `27822baa3aee663366a870bde7fb1190649d3418`.

All execution below uses fixed local agents, checks and publishers. No live provider, vendor
credential, vendor settings or paid API execution is qualified. Logs under `.council/refactor/`
are ignored local work, not packaged product evidence. A focused pass is not the release gate.

## Focused observed results

| Area | Actual local result | Evidence and limits |
| --- | --- | --- |
| Immutable handoff | PASS | Later edits to the original source do not replace the retained predecessor bytes. `005-handoff-red` records the original failing behavior; corrected focused tests pass. |
| Presence and strict inputs | PASS | Explicit empty check arrays survive; bounded whole-object review validation rejects malformed/foreign evidence. Bounded strict array CLI regression and actual review/resume pass in `005-cli-review-green.log` (33.507s). |
| Migration | PASS | Fresh default four native units; saved two/custom authority and old receipts retained. Frozen schema-4 migration fixture is separate from current schema. |
| Pure policy and adapters | Focused PASS; full release rerun pending | Literal proof, independent review schema/permissions, model-selector value preservation and explicit structured results. Fresh finding-review requirement passes the full pure suite; linked raw-check context, adapters and fixed reviewers also pass (`005-linked-check-context-green.log`, engine15.638s/adapters0.066s/fixture0.453s). |
| Candidate proof | PASS | Exact selected input/output index association and cumulative root-to-selected changes; foreign/unsafe cursors and later source edits refused. `005-candidate-index-green.log`, `005-candidate-changes.log`. |
| Store transactions | PASS | Full store suite 7.529s before the narrow current blocked-slot retry follow-up. Includes unknown application effects, generic settlement fences, no-permit recovery, append-only journal, owned prefix receipts and current summary projection. `005-store-all.log`. |
| Exhausted continuation | PASS | Known budget/routing admission failure rolls back admission and projects a user-owned blocked workflow without changing old steps, debits or proofs. Explicit grant alone does not resume work. `005-continuation-block-green.log`, 0.220s. |
| Direct author and reviewed correction | PASS | One native author plus builtin verification; actual failing required command, registered review/check blockers, correction and fresh review; saved two-unit authority blocks before correction. `005-engine-workflow-green.log`, 12.330s. |
| Response acceptance | PASS | Actual selected author/corrector producer, exact old response-acceptance envelope/receipt replay, human acknowledgement after real common-window expiry, no execution admission. `005-response-acceptance.log`, 19.334s. |
| Stop, deadline and member corruption | PASS | Real application boundaries before Ready, after durable permits and after quarantine; known effects survive Stop/deadline without acceptance. Retained unchanged member corruption is refused despite valid manifest hashes. `005-engine-controls-closure-green.log`, 27.905s. |
| Native Windows application | PASS on local Windows ARM64 only | Exact positive add/replace/delete and conflict cases, native relative exclusive rename, protected temporary ACL restoration, EA/explicit-label refusal. `005-windows-application-green.log`, 12.850s. Linux/macOS runtime qualification remains pending. |
| Model-usage coverage | PASS | Local stages remain visible but do not inflate native token coverage or imply measured zero usage/savings. `005-local-usage-green.log`. |
| Independent CLI direct/publisher | PASS | Fixed explicit publisher executes once; replay retains its result. `005-cli-direct-publisher.log`, 27.538s. Actual review/correction/finite explicit resume also passes (`005-cli-review-green.log`, 33.507s). |
| Extracted archive | PASS on local Windows ARM64 only | Exact allowlist/checksums/notices, empty-PATH startup and three actual workflow journeys with Git-only PATH: direct+builtin+explicit publisher/replay, failed-check/four-native correction, saved local application around staged/working user edits. `005-package-extracted.log`, 18.788s; no Node/npm/Go runtime on PATH. |
| Preparation evidence faults | PASS | Actual private destination publication failure before/after SQLite metadata commit fences execution with intact original/index and zero destination permits. `005-application-preparation-green.log`, 3.632s. |
| External acknowledgement faults | PASS | Actual explicit fixed publisher marker survives output-metadata failure before/after Save; uncertain attributed result stays visible, replay and new owner do not repeat execution. `005-external-ack-fault.log`, 5.072s. |
| Explicit native blocked-slot retry | PASS | Domain0.070s/store0.515s plus actual engine1.606s: new immutable execution, same role/cycle/input, finite manual debit/common window, old blocked terminal unchanged; launched/Stopped/uncertain/stale controls denied. `005-native-manual-retry-green.log`, `005-engine-manual-retry-green.log`. |
| Incident linkage | PASS | Known unknown-effects settlement retains the exact newly recorded operation incident; conflicting replacement is refused. `005-operation-incident-green.log`, 0.132s. |
| Minimal browser Start | Focused PASS | Omitted workflow starts Auto; one author/builtin/review-only path, default remains minimal. `005-browser-direct-green.log`, one case, 3.627s. Ten of eleven new cases passed in42.309s; the remaining schema4 test setup assertion was corrected to preserve historical1 + explicit additional2 = granted3/remaining2 while saved policy default remains2, then both history cases passed in9.191s. The two late-response UI races subsequently closed locally in IR005-12/13 and the pre-hosted full release run passed; the new post-hosted suite remains separately pending below. |

Some earlier failures were test setup/compiler failures, not behavioral RED: incorrect JSON case
tags, reserved declaration keys, unavailable APIs and reading a destination while the application
still held exclusive Windows handles. They are not counted as proof of a product defect.

## Current full-run and supplementary evidence

The first broad Go run caught stale legacy fixtures plus a required internal empty-array shape;
original policy/target/replay assertions were retained and strengthened. Focused legacy CLI/engine
cases pass41.789s/1.740s; the new omitted-Auto replay and original strong cases pass2.134s with
unchanged canonical wire/SQL receipt bytes. A subsequent full npm test failed only concurrent
Git startup on this Windows host; Engine191.009s, Store23.286s and Workspace37.755s passed.
The unchanged concurrent case then exposed actual process exit0xc0000142, not an established
timeout. After the independently reviewed Windows headless Git correction, three runs pass22.083s.
Temporary diagnostic capture is removed. A new full required suite is still in progress.

Supplementary actual owner tests close the matrix's two identified coverage gaps: owned checks
that mutate source or staging produce a new complete subject and invalidate stale evidence
(Engine5.396s); four branch/root/nested-project physical identity refusals preserve both user
repositories and exact indexes (Workspace7.620s). The required-response browser journey also
passes5.427s, distinguishing an inspected author answer from the supporting reviewer JSON,
retaining exact producer acceptance with two model units and no fake local application.

The actual six-scene visual run produced44 desktop/mobile captures,35 successful real Tab/focus
checks, ten matching source/dist/served approved asset hashes, reduced motion and zero recorded
horizontal overflow. Root and the independent reviewer inspected captures. Visual inspection found
a semantic error: a tracked working-file edit was labelled an index delta because unchanged
index context was mistaken for a change. The actual tracked-file regression failed2.176s before
the correction; tracked/no-index plus actual staged/index-changing positives pass10.992s afterward.
Root and the independent reviewer re-inspected the fresh44 captures (capturedAt12:18:22.461Z),
which now accurately show no task index delta. All35 keyboard and ten approved asset checks pass;
no console/network/overflow error is recorded. T052 is closed locally; final release/native gates
remain separate.

Completed task checkboxes denote implemented/authored items and their focused evidence. The
story-recording, final regression, visual, packaging, independent review, build/test/check,
source push, seven hosted jobs and complete acceptance tasks remain open as marked.

## Implementation clarifications

- Application compares the first author's captured root source to the selected complete subject.
  A correction candidate's immediate creation delta is retained separately.
- The journal first records preparing, then a closed attempt-owned plan and ready. Each native
  quarantine/publication action requires its own durable permit; a shared per-path effect ID links
  both actions. No permit can start during preparing or after Stop/fence.
- Windows uses `NtSetInformationFile(FileRenameInformation)` with a retained relative directory
  handle, one leaf name and replacement disabled. The tested Win32 wrapper rejected that relative
  form. This is a native API substitution, not overwrite or copy/delete fallback.
- A protected temporary ACL derives the effective supported DACL for private staging. The same
  already permitted publication restores and queries the exact original owner/DACL/control flags.
  Query failures, extended attributes and explicit mandatory labels refuse application.
- All retained application handles close before final settlement and claim release. A destination
  read before Close is not a valid Windows assertion of the eventual publicly readable result.
- Public macOS FD metadata is the supported boundary; malformed hidden filesystem records that
  the kernel does not return are not forensic evidence. Actual macOS runtime checks remain required.

## Final source qualification progress

The full pre-group-guard Windows ARM64 run completed successfully: all Go packages (CLI56.809s,
Engine194.786s), all53 browser cases in195.449s and the extracted archive case in23.441s, with
zero failed/skipped/cancelled browser/archive tests. The final independent Unix review then
identified the Linux new-file group ambiguity described in IR005-19. Its portable typed-fact
counterexample failed0.075s before the conservative guard, then passed0.074s; matching-group
and setgid positives remain supported. The final workspace test binaries compile for Linux
AMD64/ARM64, Darwin AMD64/ARM64 and Windows AMD64 in9.326s. These are compile-only results,
not actual Unix filesystem qualification. The independent reviewer closed the source correction;
the complete source-matching local suite has now passed before push.

## Pre-hosted f512368 local release gate

Pre-hosted source Windows ARM64 commands completed successfully:

| Command | Terminal evidence |
| --- | --- |
| `npm run build` | PASS; regenerated embedded UI and native binary. `.council/refactor/005-build.log`. |
| `npm test` | PASS all Go packages, including CLI57.096s, Engine194.276s and Workspace42.677s; all53 browser cases passed in186.596s; extracted native archive case passed in22.091s. Browser/archive failures, skips and cancellations are all zero. `.council/refactor/005-npm-test-release.log`. |
| `npm run check` | PASS3.191s: Go vet, authored TypeScript no-emit and Go formatting. `.council/refactor/005-npm-check.log`. |
| Staged source integrity | Exactly scoped source/docs/generated assets; no private `.council/`, native `build/` or reference tree staged; whitespace check passes. Final commit/push identity is recorded with hosted qualification. |

The archive case verifies exact allowlist, complete notices, reproducible ZIP/checksums,
empty-PATH startup and actual extracted direct/check/review/correction/local-application/
explicit-publisher journeys. Its execution PATH contains only the resolved Git installation
directory and excludes Node, npm and Go. This is local Windows ARM64 evidence, not the six
required hosted native results. The final staged fixture EOF cleanup is whitespace only and
does not change the accepted SQLite DDL statements.

## First hosted qualification: failed, retained

Implementation `f512368924eaec3b36f9683768055858d25a9e57` was committed and pushed to
`refactor`. [Run 42220170361](https://github.com/Ejupi-Labs/council/actions/runs/42220170361)
finished with all seven jobs failed. None is credited as native acceptance:

| Actual job | Observed failure |
| --- | --- |
| Windows ARM64, Windows AMD64 | Canonical-path fixture compared the selected TEMP alias with the saved canonical path. Three actual replacement-publication cases also returned uncertain before retained observed evidence; the deadline case failed before its deadline. The underlying metadata phase is under investigation. |
| Darwin ARM64, Darwin AMD64 | Same canonical-path fixture mismatch (`/var` versus `/private/var`); engine package then reached its ten-minute timeout during the owned index-mutating check case. |
| Linux ARM64, Linux AMD64, Linux AMD64 race | Engine package reached its ten-minute timeout during the owned index-mutating check case. The stack includes a blocked fixture channel send and engine shutdown waiting for the owned operation. No race-detector report is present; a timeout is not a race pass. |

The narrow path assertion now requires the exact saved canonical path/project identity and
`os.SameFile` binding to the originally selected directory. Its local focused case passes0.859s;
the foreign reviewer closed its source scope. The Unix cause is JSON v2's nil-slice encoding at
the private guardian boundary: inherited `Env=nil` became explicit `Env=[]`, so a local check
lost PATH and failed its Git subprocess. A second fixture observation then blocked on its full
one-slot channel and masked the failed check during shutdown. The wire now omits only a zero
(nil) environment, retaining explicit empty environments; a real owned-child regression checks
inherited/empty/selected behavior using only a test-owned sentinel. The fixture retains bounded
repeat diagnostics and cancellation-aware sends without hiding extra invocations. Focused local
Windows process0.448s and engine7.400s pass; those do not qualify the actual Unix correction.

The Windows legacy-control fixture independently reproduced exact metadata drift after a valid
setup: expected control8004 versus actual8404, with owner/group/mode unchanged. Ambient,
CREATOR_OWNER and mixed explicit/inherited fixtures pass locally. A candidate universal native
setter failed its prerequisite in the opposite direction (8404→8004), so it is not activated as
a universal replacement. The final correction selects the existing setter for auto-inherited
descriptors and the retained-handle native OWNER/DACL setter only for unprotected legacy
descriptors; protected behavior is unchanged and exact final metadata equality remains required.
Actual production application/helper cases pass Workspace26.690s and the deadline/settlement
case passes Engine15.938s. A final post-edit production current/legacy plus future-parent
inheritance case passes0.101s. The foreign platform reviewer closed the exact final source/test
scope; corrected hosted Windows confirmation is still required. No exact original hosted
descriptor bytes were available, so the controlled reproduction is not claimed to be that
runner's inspected descriptor.
Known failures will not be hidden with skips,
weakened application evidence or a blanket timeout increase. Corrected source must receive a new
commit and a complete matching seven-job run; this failed revision stays in the ledger.

## Corrected-source local gate

The first hosted failures produced three bounded corrections, independently source-reviewed:
exact physical/canonical fixture binding, preserved nil/empty Unix command environments with
cancellation-aware fixture observation, and exact Windows descriptor-model restoration.

| Corrected-source command | Terminal evidence |
| --- | --- |
| `npm run build` | PASS3.485s; regenerated embedded UI/native entrypoint. `005-hosted-corrections-build.log`. |
| `npm run check` | PASS3.153s; source formatting, Go vet and authored TypeScript. `005-hosted-corrections-check.log`. |
| `npm test` | PASS all Go packages, including CLI58.190s, Engine195.143s, Process13.800s, Store12.265s and Workspace42.935s. All53 browser cases pass195.317s; the extracted archive case passes22.103s. Browser/archive failures, skips and cancellations are all zero. `005-hosted-corrections-npm-test.log`. |

The UI source/assets did not change in these corrections; the recorded 44-image visual review
remains scoped evidence. Generated release output still requires the current build and integrity
check. Actual corrected Unix and Windows runtimes require the complete new hosted revision.

## Remaining gates

The previously recorded visual/keyboard/brand evidence remains scoped to its source/captures.
All three hosted-failure corrections have foreign source review and affected actual local
evidence; the complete corrected-source build/test/check/archive gate now passes. The corrected
implementation must be committed and pushed to `refactor`, then
pass all six actual native jobs and the supported Linux AMD64 race job at the exact revision.
The evidence matrix must close all 25 VFD and ten SC mappings before acceptance is committed.
Feature 006 implementation remains closed until those gates pass. Completion of 005 does not
complete the parent twelve-feature goal.
