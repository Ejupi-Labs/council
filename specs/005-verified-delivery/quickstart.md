# Verified task delivery acceptance quickstart

**Status**: Accepted dev.5 scope at checked source `794f08032b96434d1217f8bb3797c6803263f39c`,
after design commit `24b0cf1` and acceptance commit `c5b1801`. Execution/workflow contracts are
`005-v1`. This is the demonstration/qualification procedure; terminal local and all seven native/
race results are recorded in [acceptance.md](acceptance.md), run42224386878. Feature006 is in
planning;007–012 remain planned. This guide establishes no006 capability or live-provider result.

Follow [spec.md](spec.md), [contracts/workflow.md](contracts/workflow.md) and
[data-model.md](data-model.md). The accepted prerequisite is feature 004 at
`3f4a18e11de495f7b3138d61e030525a018868a5`, qualified in seven actual native/race jobs in run
`42172669381`; its acceptance commit is `5671fac`. Those earlier results do not qualify the 005
change. Run these journeys against the matching current binary and compiled UI through the
shared execution queue. Keep the physical branch `refactor`.

Use disposable projects and private temporary homes with labelled local author, reviewer,
corrector, verifier and publisher fixtures. No vendor executable, version/auth/login probe,
credential inspection or live model is part of this procedure. A controlled publisher is a local
fixture writing to a disposable sink, not a live deployment or remote-service qualification.

## Command status and inspection

The normal entry remains a project, task and saved operating preference. Existing project/task,
routing/budget, full-evidence and exact-owner Stop commands retain their accepted meanings.

The command names and fixed fixtures below follow the implemented 005 contract. Listing an
example does not claim that it has run or passed. No example creates an executable, publisher
action or authorization by itself.

| Journey | Contract operation and inspection |
| --- | --- |
| Inspect/save prospective delivery intent | `workflow preferences get` / `workflow preferences update --delivery apply_verified_local --expected-revision R --request-id ID`; `review_only` selects reviewable delivery instead. Choose the prospective named selected-project scope once. |
| Inspect one workflow | `tasks workflow TASK_ID`; stages, captured reason/authority, current candidate, next action and owner. |
| Inspect exact change and lineage | `tasks candidates TASK_ID` / `candidates changes CANDIDATE_ID`; paged immutable change metadata and before/after file artifact links. Complete file bodies use existing full-artifact reads. |
| Inspect stage executions | `tasks steps TASK_ID`; bounded stage/attempt history with canonical `stepId` provenance, role and executor kind. |
| Inspect observed verification | `tasks checks TASK_ID`; definition, exact checked state, terminal result, cleanup and complete output references. |
| Inspect actual review and findings | `tasks reviews TASK_ID` / `tasks findings TASK_ID`; inspected candidate/evidence, blockers, gaps and linked resolution/recheck. |
| Explicitly continue eligible retained work | `tasks advance TASK_ID --expected-revision R --workflow-revision W --task-version V --request-id ID`; act on the one current next stage after a resolved block/grant, without re-arming old automatic work. |
| Explicitly recheck a captured definition | `tasks check TASK_ID --check-id CHECK_DEFINITION_ID --expected-revision R --workflow-revision W --task-version V --request-id ID`; recheck the current selected candidate under its captured definition. |
| Inspect delivery | `tasks deliveries TASK_ID`; captured action/destination, basis, receipt and actual effects, conflict or uncertainty. Necessary authorized delivery advances within the workflow. |
| Adopt a prior task into 005 authority | `tasks workflow-adopt TASK_ID --expected-revision R --task-version V --delivery DELIVERY_MODE --request-id ID`; explicit new-version/workflow authority, no automatic work or budget refill. |
| Start the adopted first author | Read the new task/version, then `tasks run TASK_ID --expected-revision R --task-version V --request-id ID` when the workflow reports `start_author`. UI: **Run task**. Advance/recheck require an existing stage and subject. |
| Qualify a scoped publisher | `tasks start ... --delivery-file FILE`; the explicit bounded `WorkflowDeliveryInput` declares `localMode` and any supported external action/destination/command before work. There is no standalone publisher command. |

Start may optionally supply `--delivery review_only` or `--delivery apply_verified_local`;
omission uses the saved preference, with no literal `saved` flag. `--delivery-file FILE` supplies
the bounded delivery declaration instead and is mutually exclusive with `--delivery`. A separate
advanced `--checks-file FILE` contains bounded explicit executable/argument-array definitions.
Otherwise use supported direct check conventions. Optional `--review auto|required` cannot waive
an explicit review condition; `--max-corrections 0|1` reduces or captures finite cycle authority,
not extra model units. No check editor or per-task authorization ritual is part of the normal path.

The confirmed delivery mode property is `localMode`, with values `review_only` and
`apply_verified_local`. It expresses user delivery intent; the internal executor name for the
no-process application step is separate. Do not copy internal executor names into a declaration.

The inspection sequence for a qualified 005 fixture is:

```text
council workflow preferences get --json
council tasks workflow TASK_ID --json
council tasks steps TASK_ID --limit 30 --json
council tasks candidates TASK_ID --limit 30 --json
council candidates changes CANDIDATE_ID --limit 30 --json
council tasks checks TASK_ID --limit 30 --json
council tasks reviews TASK_ID --limit 30 --json
council tasks findings TASK_ID --limit 30 --json
council tasks deliveries TASK_ID --limit 30 --json
```

Continue bounded collections with their returned `--cursor`, preserving the subject/filter and
exact identities. Lists default to 30 and allow at most 100 items, within the complete-item 4 MiB
response bound. Candidate changes are metadata pages, not inline copies of every file. Full
change/output reads follow their registered artifact references. Do not
treat a page, preview, successful model exit or local apply acknowledgement as a complete task.
Use `tasks get` for the current task revision/version and `tasks workflow` for the workflow
revision before a new explicit mutation. A historical receipt is not a fresh current-state read.

Workflow and collection reads optionally select a retained `--workflow-id WORKFLOW_ID` or
`--task-version VERSION`, never both; omission selects the current version. The corresponding
HTTP selectors are `workflowId` and `taskVersion`. A selector must belong to that task and remains
bound into the page cursor. After guidance/adoption, read the older authority, checks, reviews,
resolutions, candidates and delivery with its selector. Historical inspection creates no authority;
mutations still require the current task/version/workflow controls.

The ordinary contract command shape is:

```text
council projects open /absolute/path/to/disposable/project --json
council tasks start --project PROJECT_ID --task 'Replace "old" with "new" in docs/note.md only' --fake --fake-scenario workflow-document --json
council tasks get TASK_ID --json
council tasks routing TASK_ID --json
council tasks budget TASK_ID --json
council tasks budget-ledger TASK_ID --json
```

The task quotation works in PowerShell and POSIX shells; other callers use an argument array or
their shell's quoting to preserve the original text verbatim. The workflow-specific public Start scenarios are
`workflow-document` and `workflow-defect`, always selected with `--fake`. They use the current
Council executable's native-protocol child and never an installed vendor. A scenario string cannot
select an arbitrary stage or supply approval/authority.

The controller alone selects internal `workflow-review-blocking`, `workflow-correction` and
`workflow-review-clean` role fixtures. They consume the selected candidate and current supplied
review/finding bindings. They are not public review/approval commands or extra Start selectors.
The fixed local child commands are the same executable's
`__fixture check --case workflow-validation` and
`__fixture publish --case workflow-publication --destination ABSOLUTE_FIXTURE_PATH`.
The verifier observes the actual controlled `council-workflow.txt` INVALID/VALID bytes. Its explicit
declaration uses an unreserved key such as `controlled-validation`; the child fixture case name
stays `workflow-validation`. The publisher writes only one
attributable effect in the disposable authorized sink with `command_completion` basis and labelled
output. Extracted-package qualification resolves these children to the extracted binary, with no
Node/npm/Go runtime prerequisite and no vendor fallback.

New 005 Start defaults to Auto only after exact old-receipt replay. Optional
`--workflow author_only` preserves manual managed authoring without automatic checks, review,
delivery or a qualified workflow result. It cannot waive the original acceptance requirements.
Existing response-only manual acceptance keeps its exact scope and cannot become candidate
integration. Do not use this compatibility choice as a bypass for a required workflow.

For Auto work with a response-only result, no authored/index change and no required evidence,
explicit check/review, external action or other unmet condition, show the existing response
acceptance action. Unknown task wording does not manufacture a review or command. The saved
local-application preference does not turn an unchanged workspace into an applied change.
Qualification must confirm the unchanged `AcceptResponse` action, exact replay and
`response_only_user_review` completion, with no candidate integration record.

When response-only work requires evidence, retain the selected author/corrector response's
closed artifact, digest and source/index/condition binding. Required checks and independent
review must apply to that exact response; corrected text invalidates the old response evidence
even when no file changes. Show the producer's answer rather than the latest review report.
After qualification, use the same response acceptance action and retain
`response_only_user_review_with_evidence` lineage. Old standalone acceptance fingerprints and
response JSON remain unchanged. Also qualify eligible human acceptance after
the work window expires, without launching work, and prevent authored/index changes or an
unmet local-integration requirement from entering this response-only branch.

Retain the actual returned task/version, candidate, finding, check, stage and operation identities.
Reuse each uncertain mutation's original request identity and body. Reads and waits start no
model, check, delivery or publisher. Full byte reads retain their existing exact-byte semantics.

## Prepare source, authority and budgets

Prepare a disposable Git project with an ordinary tracked `docs/note.md` containing the fixture's
unique old literal. Surround it with different HEAD/staged/working bytes in unrelated tracked
files and an ordinary untracked file. Retain their bytes, index, branch and physical project
identity for comparison. Supported local application is ordinary single-link text add/replace/
delete under existing parents, with same-filesystem private staging/quarantine and the platform's
exclusive move/revalidation boundary. No parent creation, cross-volume copy/delete fallback,
index-only/mode/type change, symlink or unsupported path is silently applied. Such a candidate
remains retained with a named gap; a local-application deliverable is not falsely completed or
silently changed into review-only delivery.

Exercise both preference histories:

1. A fresh 005 installation starts with four managed model-admission units and at most one
   captured correction cycle. A direct author/check outcome need not consume the unused units.
2. An upgraded 004 home retains every saved finite default, including two. Do not infer whether
   its old value was factory-selected or user-selected. Existing task grants, use and deadlines
   remain unchanged. An explicit saved-default edit affects future capture; additional work on
   a retained task needs its own finite authority, not a migration refill.

These are managed author/reviewer/corrector admissions, not measured underlying model requests,
tokens, allowance or currency. Checks and delivery share the common deadline and current owner,
Stop, cleanup and uncertainty controls. Workflow selection, observation and waiting add no model call.
Check or local-application attempt records do not become extra model admissions merely because
they appear in the same history. Local application is its own no-process operation; distinguish
its actual local effects from a separately launched external action.
After four native admissions, eligible local checks/application can still finish with zero model
units remaining if their captured scope, common window and current controls remain valid. They
neither open nor renew that window.

Both fresh and upgraded homes initialize review-only delivery without inferred consent. Old
autonomous full access is not local-delivery consent. In the UI, inspect the saved operating
and delivery facts. Without delivery scope, ordinary Start remains usable and produces a
reviewable result with the remaining apply action. Choose prospective verified local application
for the named selected-project scope once; subsequent in-scope tasks reuse that choice. A task
can explicitly select reviewable delivery instead. No account, model-class, check-stage editor
or separate plan-confirmation wizard is required.

Display the captured destination and delivery intent before/during work. Local application does
not authorize a commit, push, publication or deployment. Changing a preference never adds
retrospective delivery or review approval to an existing task.

## Finish the direct document case with one author

1. Open the project, keep Auto and the saved autonomous preference, enter the complete bounded
   literal-replacement task and Start with the supported labelled author fixture.
2. Inspect the compact workflow and its reason: one author followed by the sufficient observed
   document check and the captured delivery action. A recognized sufficient case uses no
   planning model or independent reviewer merely to satisfy a mode.
3. Read the candidate's exact change and source/index basis. Builtin
   `literal_document_replace/1` checks the complete expected delta and preserved constraints/index under its own
   no-child local-check attempt. Its receipt contains controller completion and exact byte proof,
   with no invented executable, process exit or model use. Command checks separately record
   captured executable/arguments/context, checked state, terminal result, output and owned cleanup.
   An author saying “tests passed” is displayed separately and supplies no verifier receipt.
4. With saved local-application scope and a compatible destination, let the qualified delta apply
   without another permission click. Inspect before/after destination identities and effects.
   Unrelated tracked, untracked and staged bytes and the selected branch must remain intact.
5. Repeat under reviewable delivery. The result exposes the complete inspectable change and
   precise remaining apply action. It may satisfy that requested deliverable while explicitly
   reporting that local application was not performed.

Record one consumed author admission, zero planning/reviewer admissions and the actual check and
delivery receipts. Do not equate a successful author exit with verified delivery or task acceptance.
An unmatched short task, extra unsupported condition or missing sufficient check retains a review
requirement or named gap; text length and model tier cannot establish sufficient verification.

For command-check journeys, inspect the frozen supported direct Go/Cargo/Node forms in
[research.md](research.md) and the contract's explicit declaration array. Arbitrary npm/run/test
shell scripts are unsupported; invoking an npm shim or translating a project/peer command does
not make it a supported direct check. Definitions are captured before launch with bounded argv,
managed-relative working context, timeout, success-exit criterion and applicability. Program exit
and full output are observed without inventing test counts, coverage or completion of a required
test that never ran. A missing tool or unsupported required definition stays a named gap.

## Review, correct and recheck within four admissions

Use a fresh-005 or explicitly four-unit fixture. The same journey with a preserved two-unit
default must block at exhaustion and name the additional finite work needed; it cannot grow
the budget automatically.
Use public `--fake --fake-scenario workflow-defect` for the controlled review/blocker journey;
the controller selects the internal role fixtures against its actual retained candidate.

For the four-native-stage qualification, set `--review required` and supply this strict
`--checks-file` array with the actual absolute Council fixture executable. Its native arguments
are not a shell command, and the local check consumes no managed model unit:

```json
[
  {
    "key": "controlled-validation",
    "executable": "/absolute/path/to/council",
    "arguments": ["__fixture", "check", "--case", "workflow-validation"],
    "relativeWorkingDirectory": ".",
    "timeoutSeconds": 30,
    "successExitCodes": [0],
    "required": true
  }
]
```

Use a native absolute path such as `C:\\fixture\\council.exe` in Windows JSON. Keep the absolute
path in the explicit declaration; no executable or vendor fallback is inferred. Required fresh
review observes candidate A before the first required command check in this controlled path.
The failed required check still runs before correction; then candidate B receives the affected
check and fresh clean review. Review record outcome `changes_required` is distinct from finding
severity `blocking`. At two consumed units, wait for actual user-owned `budget_exhausted` and the
settled required-check receipt, rather than a transient Council-owned settled transition.

For the controlled case that has spent two units on author/reviewer and needs two more, the
accepted budget grant and explicit continuation have this shape:

```text
council tasks budget grant TASK_ID --expected-revision TASK_REVISION --expected-budget-revision BUDGET_REVISION --additional-admissions 2 --request-id GRANT_REQUEST_ID --json
council tasks get TASK_ID --json
council tasks workflow TASK_ID --json
council tasks advance TASK_ID --expected-revision CURRENT_TASK_REVISION --workflow-revision CURRENT_WORKFLOW_REVISION --task-version CURRENT_TASK_VERSION --request-id ADVANCE_REQUEST_ID --json
```

Read current identities after the grant. This changes neither earlier use nor the common window.
Changing the saved default to four is a separate explicit future-task preference edit; it cannot
fund this retained task or restart its expired deadline.

| Managed admission | Observe the actual work and retained evidence |
| --- | --- |
| 1 — Author | Produce candidate A with the fixture's validation defect; preserve original instructions/constraints and its exact source/change/index identity. |
| 2 — Independent reviewer | Start a distinct fresh review-role process/context with candidate A and relevant evidence. Confirm it reads the changed file, not unchanged original source, and produces an attributable structured blocker with stable identity and any unperformed checks. The same pinned harness/model may be used without author-session reuse or self-approval. |
| 3 — Corrector | Under the captured one-cycle authority, start from candidate A and that blocker. Produce linked candidate B with the fix; retain candidate A, the original finding and all failed evidence. |
| 4 — Re-review | Inspect candidate B and its affected observed rechecks, resolve the exact blocker with qualifying evidence, and retain the original review/failure unchanged. |

Actual deterministic checks run where required and bind their observed receipts to the correct
candidate. Reuse unaffected evidence only with an identical recorded applicability basis. Do not
rerun unrelated successful tasks or count a generic “resolved” label as re-review/check evidence.
The controlled reviewer fixture must actually consume the changed candidate bytes. For native
review records, distinguish the materialized evidence/access supplied from the scope the reviewer
reports inspecting and any access limitation; supplying a path is not proof of a model tool read.

Also qualify the alternate authorized trigger: an observed failed required check against the
selected candidate can lead to the captured bounded correction. A valid finding/check failure
supplies evidence for existing authority, not authority of its own. Execution, protocol, storage,
missing/corrupt evidence, unknown cleanup, ordinary prose and unsupported retry categories supply
no correction trigger. This is separate from 004 pre-launch continuation.

If the defect persists, review lacks access, its result is malformed, the cycle is spent or budget
is exhausted, retain the unresolved state and exact next action/owner. Do not complete through a
successful process exit. Inspect original and corrected candidates, findings, check output and
resolution lineage through both UI and an independent CLI client.

After a block is explicitly resolved or finite budget is granted, use a new acknowledged
`tasks advance` against the current task/workflow revisions when the next action permits it.
A grant alone starts no stage and re-arms no old result; it does not reset the spent correction
cycle. An explicit check re-executes only the selected captured check definition against the
current candidate under current controls. No generic “correct again” or “publish now” request
can replace the captured next action and authority.

## Keep stale evidence and destination conflicts truthful

Qualify each boundary separately with controlled barriers rather than timing assumptions:

- Change required candidate/source/index bytes after a passed check. Preserve the receipt as
  historical and prevent it from qualifying the different current bytes. A check that itself
  modifies required bytes retains resulting-state evidence and invalidates affected applicability.
- Leave unrelated destination edits compatible with the task delta; verify their bytes/index
  survive application. Then change the branch, project/repository identity, affected path or path
  type before application. Retain both candidate and user state, report conflict and name the
  remaining action. No reset, stash, overwrite or silent merge resolves it.
- Interrupt application around actual file effects and receipt/acknowledgement boundaries.
  Replay a known receipt without another effect. If effects may have occurred but cannot be
  established, retain an uncertain operation and evidence. Missing acknowledgement proves
  neither success, rollback nor that no write happened; no automatic re-execution follows.

Native application holds exclusive destination/quarantine handles through Close. While a
controlled application barrier is held, inspect its complete registered observed
`destination_file` artifact and hash rather than reopening a live path. Compare destination and
private quarantine bytes after actual Close/settlement. Also let required settlement cross the
real captured common deadline after an already completed/recorded publication: retain the actual
applied result and bytes while the task fails without acceptance. Do not describe this as a
native rename completing after expiry or infer rollback from the failed task outcome.

The result separately states what was authored, reviewed, checked, delivered, applied locally
and externally performed. Current required evidence, resolved blockers, known settlement and the
captured deliverable/delivery must all qualify before acceptance. An earlier failed attempt stays
failed even when its linked correction completes the task.

## Qualify explicit external scope with a local publisher fixture

Supply the contract's supported captured fixture action and disposable destination before work.
Keep this distinct from local application. Observe the real owned publisher process, exact scope,
output and terminal result, and verify Council launches it once under that retained operation.
Previously captured authorization is reused without an extra generic permission ritual.
The accepted external basis is command completion. A `command_succeeded` result reports that
captured command's completion; it does not attest remote publication or deployment. Inspect the
controlled local sink as fixture evidence without claiming an independently verified remote effect.
A task requiring stronger remote attestation retains an explicit unsupported evidence gap instead
of a fabricated “published” or “deployed” outcome.

Repeat with only a suggestion in worker/reviewer/project text, or with absent/wrong action or
destination. No publication, deployment, push or commit is authorized. An unsupported requested
action blocks honestly. For a possible effect with an unknown external acknowledgement, retain
uncertainty and perform no automatic retry. Local fixture success is not a universal exactly-once
external guarantee or live-service qualification.

## Stop, replay and explicit legacy authority

Accept Stop during authoring, review, correction and checks, and at controlled boundaries before
delivery effects. Inspect real settlement and owned descendants. Accepted cancellation, primary
failure, owner closing/loss, evidence/persistence uncertainty, expired authority and unknown
cleanup prevent unsafe next stages; late success cannot clear the earlier terminal or authorize
application. Restart must inspect retained work rather than scan historical results into replay.

Lose acknowledgements separately for Start, correction, check and delivery. Retry each captured
identity/body and inspect its original worker/debit/receipt/effects. During the delayed reply,
edit composer/guidance/delivery controls and select another task/candidate. The late result must
not replace newer input or selection, silently rebind to a newer version, or create another action.
Independent CLI clients and the UI must observe the same operation and next action.

Upgrade populated 004 fixtures. Compare projects, drafts, versions, pins, budgets, observations,
artifacts and exact old receipts. Standalone read/control/authoring and old replay meanings remain
intact under their previous authority; they do not become verified-workflow qualification. No migration
adds integration approval, workflow/correction authority, resume permission or budget. Adopt a
retained task explicitly through the frozen new-version/workflow operation before new 005 work;
obtain any necessary additional finite budget explicitly. Adoption, budget authority and a
following explicit current-next-stage action have their own acknowledgements. The
workflow-adoption command creates a new version and starts no work. A standalone old task has
`workflow: null` until adoption; reads never seed authority. Adoption retains original/guidance/
conditions and does not turn old author output into qualifying check/review evidence. Re-read
task/workflow/budget. An adopted workflow has no current author step and reports `start_author`:
use UI **Run task** or the existing `tasks run` command with the adopted current task version and
revision. It must capture that workflow's execution mode/fixture scenario, not a later composer
choice. Advance and recheck remain unavailable until a current author subject exists. Later use
`tasks advance` for the eligible current next stage; do not infer another operation from a
historical stage result.
None of those acknowledgements can be replaced by an automatic fresh request after a lost reply.
If an older task also lacks routing/budget authority, perform the existing explicit budget
adoption first; workflow adoption does not supply that missing authority.

## UI and release qualification still to close

Capture and visually inspect real desktop and 360px renders of minimal composition, direct
verification, pending review, a blocking finding, correction/recheck lineage, reviewable delivery,
successful local application, destination conflict, uncertainty and Stop/cleanup. Visibly label
controlled local fixtures; actual fake provider/model/usage facts remain unknown. Keep current
and historical check/review/delivery evidence distinguishable without relying on colour alone.

Use the keyboard through Start, selected stage/finding, full output, correction and delivery
actions. Verify accessible names, visible focus, stable focus during asynchronous progress,
readable text/contrast, coarse-pointer targets, reduced motion and no horizontal overflow at
360px. Delayed reads must preserve selected task/candidate and newer local controls. Verify the
approved Ejupi Labs asset/icon SHA-256 values in source, compiled output and served bytes and
retain their notices. A captured image is evidence to inspect, not a visual pass by itself.

Run the required build/test/check and extracted-native package journeys serially against the
reviewed implementation and matching compiled browser assets. Qualify all six actual native
targets and supported race job; record exact source identities, commands, terminal results,
scenario/VFD/SC mappings, independent findings/corrections and actual image review. Missing
runners, compile-only checks, fake responses and unperformed live work are never counted as
provider/native qualification. Close 005 acceptance before 006 implementation.
