# Economical routing acceptance quickstart

**Status**: Planned functional demonstration and qualification procedure. No implementation,
runtime check, render or live-provider result is claimed by this document.

This procedure follows [spec.md](spec.md) and its closed [requirements review](spec-review.md).
The accepted baseline is feature 003, qualified at `ce8aaa7` in native/race run `42153455521`.
Run these journeys only after feature 004 design is committed and its implementation is ready.
Use disposable projects and temporary private homes, labelled local Codex/Claude workers and
authored observations. Do not run a vendor executable, login/version/help probe or live task,
inspect credentials, or modify an unrelated native conversation for qualification.

## Contract operations and inspection

The following spellings follow [contracts/routing.md](contracts/routing.md) and the entity names
in [data-model.md](data-model.md). They describe the planned `2.0.0-dev.4` / `004-v1` interface,
not commands already implemented or executed by this document. Existing project/task/native
evidence and exact-owner control operations retain their accepted meanings.

| Planned CLI operation | HTTP meaning and inspection |
| --- | --- |
| `routing policy` / `routing policy set --expected-revision R ...` | `GET`/`PUT /routing/policy`; inspect captured profile order, `unknownAvailability`, `defaultBudget`, `continuation` and policy revision. No worker. |
| `routing contexts` / `routing profiles [--context ID]` | `GET /routing/contexts` returns at most two `ContextView` items with `accountLink` and `capacity`; `/routing/profiles` returns at most six fixed profiles. Installation, declared links and actual native identity remain separate. |
| `routing accounts` / `routing account add --label LABEL [--provider LABEL]` | `GET`/`POST /routing/accounts`; paginated declarations or a declared account plus receipt, with no login guarantee. |
| `routing link --context ID --declared-account ID --expected-revision R` | `PUT /routing/contexts/{id}/account-link`; use the link revision, not the context revision. `--unlink` replaces `--declared-account` for removal. No capacity reset. |
| `routing capacity --context ID` / `routing reports --context ID` | `GET /routing/capacity` and `/routing/reports`; `--account ID` replaces `--context ID` for declared-account scope. Read the subject capacity revision and supporting report history. |
| `routing report --context ID --dimension occupancy --state busy --expected-revision R --valid-for-seconds 900` | `POST /routing/reports`; returns `{report,capacity}` plus receipt. Public source is `user_report`, with source/time/scope/validity preserved. |
| `tasks routing ID` / `tasks routing-decisions ID` | `GET /tasks/{id}/routing` returns `{authority,latestDecision,adoptionRequired}`; `/routing/decisions` returns immutable paged decisions. |
| `tasks budget ID` / `tasks budget-ledger ID` | `GET /tasks/{id}/budget` returns `{budget,adoptionRequired}`; `/budget/ledger` returns paged exact ledger entries. An unadopted task has null budget, not exhausted zero. |
| `tasks budget grant ID --expected-revision R --expected-budget-revision B --additional-admissions N [--new-window-seconds N]` | `POST /tasks/{id}/budget-amendments` with `action:"grant"`; explicit additional authority, no worker or automatic re-arming of an older proof. |
| `tasks adopt ID --expected-revision R --additional-admissions N --window-seconds N` | The same amendment route with `action:"adopt"`; a new captured version/authority and additional budget, followed by a separate explicit Start. |
| `tasks usage ID` | `GET /tasks/{id}/usage`; inspect `items`, `nextCursor`, `knownSubtotal`, `coverage`, `estimates`, `referencePrices` and `charges` without combining overlapping scopes. |

New history reads support `--limit`/`--cursor` as documented. Use the API-1 envelope: successful
reads have `data` and no receipt; acknowledged mutations have their exact request ID and receipt;
errors have `error` and no receipt. Counts/revisions remain decimal strings; small durations are
integers. `TaskBudget` uses `grantedAdmissions`, `consumedAdmissions`, `remainingAdmissions`,
`automaticAdmissions`, `historicalAdmissions` and `currentWindow`; its window dates remain null
until the first admission in that window. Admission snapshots do not become current observations
when replayed. A page subtotal cannot be presented as the complete task total.

Read each context's always-present `accountLink.revision` for the first or later link edit;
`accountLink.declaredAccountId` is null when unlinked. Read `capacity.revision` separately for a
report mutation. Inspect `capacity.asOf`, each occupancy/allowance/service dimension's
`effectiveState`, `latestReport`, `blockingReports` and `positiveReportIds`, then
`reportEligibility.policy`, `permitted`, `positiveCoverage` and `reasonCodes`. Report eligibility
does not prove native authentication, model access or all routing prerequisites. Inspect
`managedOccupancy` separately for Council's one-worker claim. A report reference's `inheritedFrom`
identifies the account/link provenance of a retained denial; full immutable evidence is available
through `GET /routing/reports/{id}` and paged report history.

The ordinary fake journey remains:

```text
council projects open /absolute/path/to/disposable/project --json
council tasks start --project PROJECT_ID --task "Perform this bounded fixture task" --fake --json
council tasks get TASK_ID --json
council tasks attempts TASK_ID --json
council attempts get ATTEMPT_ID --json
council tasks routing TASK_ID --json
council tasks budget TASK_ID --json
```

Use exact returned identities/revisions and retain `--request-id` on every uncertain mutation.
Do not substitute a current route, policy or budget snapshot for the original receipt replay.

## Prepare the baseline and preserve the project

When implementation is ready, run `npm run build`, `npm test` and `npm run check`. Retain existing
Codex/Claude protocol, source isolation, process ownership, strict input, receipt replay, private
state, evidence integrity and package checks.

Prepare a disposable Git project with different HEAD, staged and working contents, ordinary
untracked source and supported project instructions. Retain original file/index bytes, branch
and physical project identity for comparison. Seed deterministic fake context/profile/report
fixtures; their declared capabilities and observations are test data, not native access proof.

## Start with Auto and no extra setup

1. Open the project, retain the saved autonomous full-access preference, enter the original task
   and Start with Auto and labelled fake execution. Do not require an account, quota, price,
   model-class/profile or transport form, or a separate plan-confirmation click.
2. Inspect the admitted task and route. New economical tasks receive exactly two managed attempt
   admission units and one 30-minute wall-clock window anchored at the first budgeted admission.
   Automatic continuation and escalation are OFF by default. Unknown entitlement, external
   occupancy and allowance remain eligible under the default policy unless applicable negatives
   or required capabilities block them; they are not labelled available, idle, verified or free.
3. With unknown task suitability, expect only a constraint-compatible conservative general/native
   default or a specific blocker. A short description or model name cannot establish routine fit.
4. In a separate controlled case, use optional `--suitability routine` and comparable eligible small
   and larger targets. Confirm selection of the suitable economical target, including the
   comparison basis. Make the small target fail a required capability/quality/context condition;
   it must be excluded for that reason. Optional class/profile choices stay optional.
5. Remove price comparability or access evidence. A declared preference or native default may
   remain useful, but the explanation must retain unknowns and make no proven-lowest-cost or
   savings claim. Selection, waiting, refreshing and inspecting history add zero model calls.

The main task view should concisely identify the selected context/harness/selector, saved policy,
route reason, budget and next action. Full candidate comparisons and evidence remain inspectable.
Proposed, admitted, launched and actually observed targets have distinct meanings.

The fixed reference profiles are `codex-native-default`, `codex-luna-economy`,
`codex-sol-general`, `claude-native-default`, `claude-haiku-economy` and
`claude-sonnet-general`. Roles/selectors and dated sources are declarations, not model-access or
quality proof. Optional `--target-profile` must remain compatible with literal harness/model pins.
Unknown work without an explicit profile/model choice uses a compatible native-default profile.

An optional reference-price input requires `currency`, `sourceLabel` and `observedAt`; omit the
response-only `sourceKind`. Missing `unit`, `basis` or rates remain unknown/incomparable, not zero
or an assumed marginal price. Comparable references require identical currency, `per_million_tokens`
unit and explicit `api_list` or `declared_marginal` basis, plus current validity, within the
contract's exact arithmetic bounds. Within an otherwise equal candidate group, find the
nondominated price frontier and use captured stable order among that frontier. Crossed or unknown
rates cannot establish a cheaper target; do not guess a task's token mixture or measured savings.

Also exercise the plan's whole-task recognized-operation shortcut with an ordinary file such as
`docs/note.md`: `Replace "old" with "new" in docs/note.md only`. The complete task must fit the
published grammar and its file/literal bounds; extra instructions, guidance, free-form conditions,
required evidence, unsafe paths or changed source disable the shortcut or block its stale proof.
A keyword within unrelated prose never suffices. The original task/prompt remains complete.

## Report busy Codex and available Claude optionally

In Agents, record user-sourced busy personal-Codex and available-Claude observations with the
visible default 15-minute validity. Start suitable Auto work and inspect its Claude choice and
captured evidence. The personal Codex conversation, model, settings and authentication remain
untouched. No extra account or login wizard is required.

The equivalent planned commands use each subject's current capacity revision:

```text
council routing capacity --context codex-native --json
council routing report --context codex-native --dimension occupancy --state busy --expected-revision CODEX_CAPACITY_REVISION --valid-for-seconds 900 --request-id BUSY_REQUEST_ID --json
council routing capacity --context claude-native --json
council routing report --context claude-native --dimension occupancy --state available --expected-revision CLAUDE_CAPACITY_REVISION --valid-for-seconds 900 --request-id AVAILABLE_REQUEST_ID --json
```

An account-scoped report substitutes `--account ACCOUNT_ID`. Explicit exhaustion recovery uses
`--dimension allowance --state available --correction-of REPORT_ID --correction-action recover`
and the current original subject's capacity revision; withdrawal uses `unknown` and `withdraw`.
Inspect immutable history with `routing reports`, or read the exact report and its
`unresolvedScopeKeys` through `GET /routing/reports/{id}`. A normal positive without the required
correction cannot erase unresolved exhaustion.

Inspect both managed occupancy and reported external occupancy. A merely running process,
Council inactivity or an available report cannot establish external idleness, clear an active
managed claim or prove remaining allowance. A context display label or declared account link is
not an observed login and does not add native account-selection capability.

Use deterministic time/report fixtures to check these boundaries without waiting in real time:

- Ordinary busy/available occupancy reports expire to unknown at their stated validity. Report
  age, source and validity remain visible; default unknown eligibility does not fabricate a
  fresh positive observation.
- Exhaustion remains denying after its report ages or a reset time passes, with that age visible,
  until an explicit attributable correction/recovery supersedes it. A price edit, alias relink or
  unrelated report removal cannot refill the meter. Temporary cooldown expiry is separate and
  does not prove replenishment.
- An older positive arriving after a newer applicable exhaustion/denial cannot erase the newer
  negative. Invalid, future-dated and duplicate/stale report cases follow the frozen contract.
- Correct an older still-retained exhaustion even when a newer subject head exists, including a
  denial inherited through a later account link. Name its exact report ID and original
  subject/dimension with the current subject revision. Clear only that report wherever retained;
  a different newer denial must remain blocking. Unlinking cannot remove the inherited denial.
- Under optional `require_positive`, fresh available reports must cover BOTH occupancy and
  allowance. An account-positive counts only for its original context bindings while the same
  account link remains current; relinking cannot transfer or freshen it. Default `allow` still
  permits unknowns when no applicable negative or other prerequisite blocks the target.
- Several model aliases explicitly associated with one declared account share its applicable
  occupancy/allowance evidence. Two declared accounts remain distinct even when their harness is
  the same. Link changes cannot release managed work or create fresh allowance.

Release independent clients together and inspect the actual common owner, global native worker
limit, shared declared-account controls and exact budget effects. Demonstrate that aliases and
concurrent clients do not each obtain an independent copy of capacity or remaining task budget.

## Preserve exact choices and supported guarantees

Use optional harness/model pins from the composer and equivalent CLI intent. A partial pin
constrains the remaining Auto choices. An unavailable, busy, exhausted, ambiguous or unsupported
combination must retain its literal choice and report the authorized wait/block action without
another account, selector, harness or payment path being substituted.

Request an exact actual-account binding that the native interface cannot enforce; it must block
without promoting a declared account label to authentication proof. Likewise, an unsupported
actual-account/payment choice through its supported request field must return the documented pin
or capability blocker. No structured strict effective-model or financial-guarantee field is
exposed: unknown request members/flags fail strict input validation rather than being ignored.
Ordinary `pins.model` / `--model` is a literal native selector with vendor-owned alias/fallback
meaning, not an effective-model guarantee. Do not infer stronger authorization from task prose.
Direct Council-managed paid API execution is unsupported; refusal is not a working API executor.
Native billing/overage/internal requests remain outside Council's enforceable financial guarantee.

## Account for admitted work and the common deadline

Inspect admission units separately from known native launches and unreported underlying model
calls, tokens or charges. An admitted attempt consumes its unit even if preparation later blocks
and no worker launches. A labelled pre-admission proposal consumes no attempt unit and is never
presented as a started worker. Lost acknowledgements or uncertain effects are not refunds.

Repeat a start with its exact request identity after a lost reply and changed reports/prices/
preferences. The original route, admission and budget effect replay once; changed intent with the
same identity conflicts. Guidance, manual replacement, aliases, owner restart and policy edits
never silently replenish cumulative use or restart the task window. Explicit additional authority
is a linked amendment with original use/history retained.

Include queue, preparation, execution and settlement time in the same first-admission-anchored
30-minute window. At expiry, forbid new dispatch and request controlled Stop. The timer does not
pause for cleanup. Bounded owned cleanup may finish later, with its actual state/incident visible;
this neither extends authority nor promises a hard real-time OS completion deadline.

The existing `timeoutSeconds` / `--timeout-seconds` remains a separate per-attempt ceiling. Each
attempt's deadline is the earlier of that ceiling and the common window deadline; guidance cannot
extend the window or an already admitted deadline. New task overrides use `--budget-admissions`
and `--budget-window-seconds`. Budget options are invalid on guidance. A unit-only `tasks budget
grant` keeps the deadline; an explicit new window requires settled verified cleanup and starts at
the next budgeted admission. A grant does not automatically launch work or re-arm an old proof.

## Qualify continuation OFF, supported A and unsupported B

First prove that the default OFF policy launches no automatic replacement after a failure or a
queued target becoming ineligible. Its original admitted target/decision remains immutable.

Then explicitly opt into finite category A (`pre_launch_ineligible`), target restrictions and
any replacement/escalation authority. For example, a planned new-task fixture can supply:

```text
--max-automatic-admissions 1 --allow-continuation pre_launch_ineligible --allow-replacement
```

Add `--allow-escalation` only when that additional authority is intended. Use the real owner/
control/evidence path with qualified local fixtures. Each permitted successor requires prior
settlement, verified cleanup, current controls, remaining budget/deadline and a new linked
decision/attempt consuming another unit. Ordinary guidance preserves captured routing policy;
`--use-current-routing-policy` explicitly adopts current saved routing preferences, guarded by
`--expected-routing-policy-revision` when supplied. It never grants budget implicitly.

| Case | Required proof and expected outcome |
| --- | --- |
| A: selected target becomes ineligible before native creation | A current applicable capability/capacity change plus durable blocked outcome, no native preparation/creation attempt, no missing expected output and verified not-launched cleanup. The intentionally absent worker output is not evidence loss. With authority, admit one linked eligible replacement; without authority or a remaining unit, retain the blocker. Never rewrite the original target to avoid the next admission unit. |
| B: `terminal_transient` | Unsupported on every accepted 004 profile. Enabling it with a positive automatic limit returns `capability_unavailable` before new work. SDK documentation, a hypothetical positive fake, generic 429/529, retry progress or native prose cannot qualify B. Its complete structured terminal/input/output/root/cleanup proof remains a requirement for a separately reviewed future capability, not an implemented retry path. |
| Forbidden apparent reasons | Prose, retry-progress messages, generic status numbers, authentication/model-access failure, malformed/truncated output, missing/corrupt evidence, uncertain commit, owner loss, accepted Stop or unknown cleanup supply no A/B authority and start no replacement. |

A changed context/selector requires replacement authority and also requires escalation authority
unless both tiers are known, the new tier is equal/lower, and its comparable price is equal/lower
in both rates. Missing, crossed or incomparable prices therefore require escalation authority
even for a declared lower tier. Retrying the same exact context/selector is exempt from those
target-change comparisons, while all category, automatic-unit, budget and control guards remain.
Tiers are policy statements, not measured intelligence or charges. This slice adds no automatic
quality-review/check correction trigger. An earlier failed/cancelled attempt remains unchanged.
A retained candidate is not silently integrated into a successor workspace or promoted to
verified delivery.

## Inspect history, usage, upgrades and stale intent

Task history must retain successful, failed, cancelled and interrupted work with its original
route evidence. Later reports, prices and eligibility are a separate current view. Each usage
item selects ONE explicit sourced `UsageObservation`; `selectedUsageScope` distinguishes
`top_level`, `whole_tree`, `unknown` and `fake_unmeasured`. Its `nativeModels` preview remains
separate and contributes no second aggregate. Inspect exact per-counter `knownValue`,
`contributingAttemptIds` and `missingAttemptIds` in `knownSubtotal.byUsageScope`, and the explicit
included/unknown/excluded-fake attempt IDs in `coverage`. Both carry scope `returned_attempts`:
even complete page coverage is neither complete provider work nor a complete-task total. Null
known values mean no known contributors; zero requires sourced zero values.

Follow `nextCursor` and the contract's whole-item byte-budget paging rules; pages may contain
fewer items than requested without clipping or skipping an attempt. Preserve exact native
estimate lexemes and separately labelled `estimates` and captured-decision `referencePrices`;
neither is a measured charge. For the accepted native interfaces `charges.status` is always
`unknown` with empty items, not zero cost. Never add overlapping top-level/whole-tree metrics,
model previews, repeated messages or input/cache fields into a synthetic total. Fake workers and
admission counts imply no token measurement, quota decrement, charge or savings percentage.

Upgrade populated feature-003 fixtures and compare original versions, Auto/Codex meaning, pins,
receipts, evidence and known use. Inspection, Stop and exact old replay remain available. A new
attempt on an unadopted legacy task must require explicit adoption naming additional work and a
new window while retaining prior known use; migration grants no new authority. Stopping a known
older owner uses exact-owner maintenance, without takeover or migration under its busy lock.

The planned explicit sequence is:

```text
council tasks adopt TASK_ID --expected-revision TASK_REVISION --additional-admissions 2 --window-seconds 1800 --request-id ADOPTION_REQUEST_ID --json
council tasks routing TASK_ID --json
council tasks budget TASK_ID --json
council tasks run TASK_ID --expected-revision RETURNED_TASK_REVISION --task-version RETURNED_TASK_VERSION --fake --request-id START_REQUEST_ID --json
```

Read the adoption response's `task.revision` and `task.currentVersion` for that later Start. Its
receipt and the Start receipt have separate retained identities. Before Start, the new window's
first-admission/deadline dates remain null and remaining admissions equal the additional grant;
the known legacy baseline is separately historical and included in retained cumulative use.
An optional UI Adopt and start action must reconcile the exact adoption acknowledgement before
sending its frozen dependent Start. Lost replies, newer guidance or changed selection cannot
repeat adoption, silently target another version or replace newer local work.

Delay admission, route/list/detail and report-save replies while changing text, pins, reports and
selected tasks. Preserve clicked pending intent and all newer local input/selection, including
after reload/retry. A queued plan that becomes ineligible cannot silently overwrite its original
route or bypass Stop, guidance, budget, account or cleanup controls.

## Render and final qualification gate

After implementation, inspect real desktop and 360px Auto, optional choice/report, route/budget,
history, blocker and Stop states. Exercise keyboard focus, reduced motion, readable text state
labels and the approved Ejupi Labs assets/notices. Controlled observations must be visibly labelled
test data; actual fake provider/model/usage facts stay unknown. No extra per-task setup ritual is
part of the ordinary journey.

Qualify the extracted native package's ordinary routed fake and pinned fake journeys with its
existing runtime/exclusion guarantees. Run final build/test/check, six actual native platform jobs
and supported Linux race checks against the reviewed implementation commit. Record commands,
terminal results, RTE/SC mappings, independent findings/corrections and remaining limitations in
the feature acceptance/review records. Fake and compile-only checks never count as live provider
qualification. Close that gate before feature 005 implementation.
