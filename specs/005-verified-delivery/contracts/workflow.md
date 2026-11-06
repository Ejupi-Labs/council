# Verified delivery contract

**Feature**: 005-verified-delivery. API1 / app2.0.0-dev.5 / execution005-v1 / schema5.
**Status**: Design for independent review; no implemented operation or passing runtime implied.

The [specification](../spec.md) governs outcomes; [data-model.md](../data-model.md) governs exact
storage/ownership/applicability. Existing envelopes, private authentication, origin controls,
exact request receipts, bounded reads and error handling remain. No client-supplied proof is trusted.

## Compatibility and current owner

Meta adds capability `verified_delivery` and `workflowContract: "005-v1"`. Current mutation clients
require the exact005 application/execution/capability combination, not API1 alone. Retain exact
dev.2/002, dev.3/003 and dev.4/004 maintenance Stop rules with original owner/request binding.
Known older owners cannot execute005 checks/application/review or accept workflow candidates.

Old row JSON, source/candidate meaning and request fingerprints/envelopes remain unchanged.
Optional new request members are omitted when absent; replay happens against that original shape
before resolving new defaults or current filesystem/preferences. There is no migration-created
workflow, delivery consent, review approval, budget grant or automatic restart.

For a new005 Start, omitted workflow input resolves to Auto after replay. Explicit `author_only`
retains the existing managed-authoring path without automatic checks/review/delivery or a claim
that its candidate is qualified. Existing tasks without workflow authority keep that standalone
meaning until explicit workflow adoption creates a new version. UI/CLI default to Auto; author-only
is an optional compatibility/manual-authoring choice, never a shortcut to workflow acceptance.

## Strict values and bounded objects

IDs are32 lowercase hex; hashes64 lowercase hex; exact counts/revisions are canonical int64
decimal strings. Times are UTC milliseconds endingZ. Public small options are bounded integers.
Unknown/duplicate fields, null required objects, explicit empty IDs/enums/counts and malformed
values fail before authority/worker/effects. Existing intentional empty text rules remain.

Defaults: fresh stores four model admissions; every existing saved finite default is preserved.
Task authority/window remain captured, never replenished by guidance/configuration/upgrade.
New Auto allows one correction cycle for `blocking_review_finding` or `failed_required_check`.
This is separate from004 automatic continuation, which stays pre-launch-A-only/default OFF.

Bounds:16 checks/workflow;64 arguments/command;4096 UTF-8 bytes/argument;16KiB aggregate argv;
64KiB encoded authority/definition/operation/receipt;64 findings and64 gaps/review;1MiB complete
review artifact;16KiB previews/events. Aggregate encoded bounds apply even when individual bounds
fit. Lists default30/max100, stable high-water cursors, complete-item4MiB envelopes, no skipped
items. File contents/deltas/full output use registered artifacts and explicit full reads.

## Start and saved preference

New optional `StartTaskInput.workflow` and explicit adoption input use this shape:

```json
{
  "mode": "auto",
  "delivery": {"localMode": "review_only"},
  "checks": [],
  "review": "auto",
  "maxCorrectionCycles": 1
}
```

`mode`:auto|author_only. Review:auto|required; no skip override can remove an explicit condition
or make an unproven change direct. `maxCorrectionCycles`:0|1 (default1 for Auto). A declaration can
reduce authority; it cannot silently grant additional model units/window. `author_only` rejects
contradictory automatic workflow/delivery declarations; original acceptance requirements remain
pending and the candidate is retained without workflow acceptance.

Omitted `delivery` snapshots current preference after replay; omitted checks trigger supported
convention selection. Explicit empty checks supplies no commands; it cannot waive required checks,
the builtin direct proof or required review. Definitions are captured before execution and cannot
be replaced by generated text. Scope/definitions that do not meet a required condition remain gaps.

`GET /workflow/preferences`, operation `workflow.preferences.get`, returns:

```json
{"localMode":"review_only","selectionSource":"default","revision":"1","updatedAt":"2026-11-06T00:00:00.000Z"}
```

`PATCH /workflow/preferences`, operation `workflow.preferences.update`, accepts:

```json
{"requestId":"11111111111111111111111111111111","expectedRevision":"1","localMode":"apply_verified_local"}
```

Canonical local modes are `review_only` and `apply_verified_local`; executor `local_application`
is a separate concept. Fresh/upgraded preference is review_only. A user update explicitly saves
prospective local-delivery scope, independent of permission mode; task admission captures the
named selected project/repository/branch. Full access alone supplies none of this consent.
Changing a preference does not retarget old workflows or authorize commits/publication.

`WorkflowDeliveryInput` (the Start delivery member) is:

```json
{
  "localMode":"review_only",
  "externalAction":{
    "action":"publish-preview",
    "destination":"example-preview",
    "acceptanceBasis":"command_completion",
    "command":{"executable":"/absolute/native/publisher","arguments":["--destination","example-preview"],"timeoutSeconds":120}
  }
}
```

Only one optional external action is supported in this bounded workflow. Action/destination are
nonempty attributable labels, each at most1024 bytes; neither is silently substituted into argv.
The command is an explicitly declared native executable plus static argv, run inside the verified
managed candidate. No shell/wrapper, expression, environment expansion, generated command or
implicit credential/global-setting change. Runtime resolves physical program identity outsideSQL.
Native binary/path identity and exact observed argv/context are retained. Environment values are
not copied into records; no native login files are read. Command completion is the only supported
external acceptance basis; stronger remote-state postconditions are a named unsupported gap.

Results are `command_succeeded`, `command_failed`, `cancelled`, `unavailable` or `uncertain`.
They attest the owned declared command and observed settlement, not independently verified remote
publication. Unknown acknowledgement remains unknown with no automatic re-execution. Local
application authorizes no Git commit, push, release or deployment. Each such action requires its
corresponding captured declaration; worker/project/peer text is not scope.

## Workflow, stages and operation routes

All task routes validate task ownership and selected task/version/workflow binding. Reads accept
mutually exclusive `workflowId` or `taskVersion` selectors; omission selects the current version.
Selected historical authority, steps, reviews, resolutions and delivery remain fully retrievable
after guidance/adoption. A selector must belong to this task; it never creates execution authority.
List filters include that selector plus limit/cursor. Each cursor binds resource/task/selector/
high-water. Mutations still require the current version/workflow/control revisions.

| Method/path | Operation | Meaning |
| --- | --- | --- |
| GET /tasks/{id}/workflow | tasks.workflow.get | Current workflow authority/projection or null for standalone historical task; no work. |
| GET /tasks/{id}/workflow/steps | tasks.workflow.steps.list | Paged finite step executions. |
| GET /tasks/{id}/workflow/reviews | tasks.workflow.reviews.list | Paged structured review summaries, full artifact links and applicability. |
| GET /tasks/{id}/workflow/checks | tasks.workflow.checks.list | Paged observed check receipts/definitions/applicability. |
| GET /tasks/{id}/workflow/findings | tasks.workflow.findings.list | Stable findings, origin and qualified resolutions; unresolved blockers explicit. |
| GET /tasks/{id}/workflow/deliveries | tasks.workflow.deliveries.list | Local operation/external result summaries and full effect/evidence references. |
| GET /tasks/{id}/workflow/candidates | tasks.workflow.candidates.list | Retained candidate lineage and current qualification without rewriting candidate creation facts. |
| GET /candidates/{id}/changes | candidates.changes.list | Paged full immutable change metadata with before/after file artifact links, never inline whole file bodies. |
| POST /tasks/{id}/workflow/adopt | tasks.workflow.adopt | Explicit new task version and prospective workflow authority, without launching work or refilling budget. |
| POST /tasks/{id}/workflow/advance | tasks.workflow.advance | Explicit current next operation after a block; never revives an old automatic proof. |
| POST /tasks/{id}/workflow/check | tasks.workflow.check | Explicit recheck of one captured required definition against the current selected candidate. |

`tasks.workflow.get` returns `{taskId, taskVersion, workflow}` for the selected version. Workflow contains immutable `authority`, exact
`revision/stateRevision`, state, selectedCandidateId/currentStepId, nextAction/nextOwner,
correctionCyclesUsed/latestAcceptanceId/timestamps. Task summary gains an optional compact
workflow summary and projects workflow state/next action only for current005 authority. Its
latest attempt remains an actual attempt; acceptance never means its latest stage simply exited0.

Adopt request: `{requestId, expectedRevision, taskVersion, workflow}`. Existing task must have
known settled ownership and no active conflicting stage. It creates an immutable new task version
with retained original/guidance/conditions and new authority. It does not convert an old author
response/candidate into accepted evidence or restore budget. Missing old budget authority requires
the existing explicit budget adoption first. A later Start/advance is a separate frozen request.

Advance request: `{requestId, expectedRevision, workflowRevision, taskVersion}`. The controller
derives the one current compatible next stage from stored authority/evidence; callers cannot
choose an arbitrary command, candidate, role, override policy or add a correction cycle. Known
completed request replays before prerequisites. A new explicit request can authorize a current
permitted stage after a grant, but a grant alone never starts it or re-arms an older proof.

Recheck request: the same control fields plus `checkId`. It admits one existing definition against
the current selected complete candidate. Replacing a definition/conditions requires a new task
version and applicability decision. A recheck is not correction authority or implicit acceptance.

Step roles are author/review/correction/check/local_delivery/external_delivery. Executor kinds are
native/local_check/local_application/local_external. Optional new Attempt tags bind workflowId,
stepId, role, executorKind, inputCandidateId and trigger(user_request|workflow_stage). Historical
absence means standalone author in interpretation only. All stages use the same existing global
claim and deadline/fence controls. Native stages alone consume model units; local stages may run
with zero remaining model units if their captured scope and common window remain valid. They
never open or renew that window. Native workflow admission retains SQL kind manual as captured
user-issued task authority plus explicit workflow_stage provenance; automatic remains004A.

For enabled004A, a proven pre-launch block owns an immutable failed/blocked step execution.
Its replacement creates a NEW step ID and attempt, preserving logicalKey/role/cycle/check
definition/inputCandidateId and adding replacementOfStepId plus PreviousAttemptId lineage. That
step binding, new model debit, automatic routing decision and unique004A-proof consumption commit
atomically. The old step is never rebound. A blocked predecessor produces no005 advancement proof.
Only the final qualifying execution of that logical slot can produce the appropriate005 proof:
ordinary forward progression requires success; a completely settled valid blocking review or
observed failed required check may instead produce the already authorized correction proof.
One current logical slot and unique consumption prevent004A/manual/005 competing successors.
After a fresh run returns through deferred cleanup, the owner handles004A first and returns if it
admitted a replacement; it cannot also advance005 from that blocked execution. No restart scan.
An explicit later advance request may retry the currently blocked slot with a new execution ID
and captured-user/manual admission under current controls/budget, never by reviving old proof.

## Proportionate finite flow

Task usage retains every stage identity but only native-agent stages contribute to model-counter
coverage. Local executor facts are not applicable to managed native-model counters, never zero
tokens or measured savings. Separate returned stage/model-stage counts prevent local checks from
inflating missing native-usage coverage. Declared programs' own internal billing/side effects stay
outside Council's managed model-admission count and are not claimed measured or absent.

1. Capture original conditions, checks, scope and finite stage authority. Start author once in a
   source-isolated workspace, preserving requested literal pins and all routing controls.
2. Retain its complete candidate. The builtin direct proof may qualify only the accepted complete
   literal-document grammar, exact expected bytes, no extra delta/index change or other condition.
   A routine hint or small output cannot qualify this shortcut.
3. Otherwise require fresh independent review and supported required checks. A check can precede
   review to avoid reviewing a reproducible failing candidate; no unrelated stages are replayed.
   No sufficient check convention means an explicit gap, not an invented test or required command.
4. A valid blocking review finding or observed failed required check can use one previously
   captured correction cycle. Materialize the selected candidate/findings, create a new native
   correction attempt/unit, retain old failures and rerun affected checks/review. Infrastructure,
   malformed protocol, missing evidence or unknown cleanup creates no automatic correction.
5. When conditions/review/checks/blocker resolutions all qualify, deliver the retained change or
   apply its supported delta under scope. Execute an external declaration only when its current
   verified basis and explicit scope qualify. Record each outcome separately.
6. Accept only the current conjunction of conditions, exact candidate/applicability, known
   settlement and requested delivery. Response-only manual acceptance retains its prior exact
   semantics and must not be silently promoted into candidate integration.

Fresh automatic advancement is produced/consumed once within the current owner epoch after the
predecessor's complete deferred cleanup. It binds task/version/user/control/workflow/budget/window,
selected candidate and logical next step. Stop/guidance/grant/report/policy/owner changes are
rechecked and may invalidate it. No startup scan, old terminal or read launches a stage.

## Exact review and check evidence

### Captured condition registry

The authority captures a deterministic registry of at most32 conditions; it never asks a model to
split or rewrite free-form requirements. `task-instructions` covers the complete original text and
guidance; `task-conditions` covers nonempty complete acceptance text. Existing required-evidence
flags map to required-check, independent-review, local-integration and external-delivery. Auto
adds workflow-verification and, where needed, workflow-review; captured delivery adds
delivery-local or delivery-external. Each explicit required check key adds its command-completion
condition, e.g.project-tests. User keys may not collide with reserved task-/required-/workflow-/
delivery- prefixes or the named required-evidence keys. A missing required condition stays a gap.

Each registry entry is `{key,kind,source,evidenceRefs}` and references complete original/configured
material, not a generated summary. Instruction digest is SHA256 of a fixed-field canonical JSON
object containing verbatim originalText/guidanceText. Conditions digest is SHA256 of a fixed-field
object containing resolved acceptance, ordered registry and captured check-definition digests.
Canonical encoding is the authored typed JSON encoding, with stable field order and canonical
sorted registry/check keys, no maps/floats/concatenation ambiguity. Digests bind actual values;
candidate/version occurrence IDs are recorded separately. Complete bodies remain retrievable.

Explicit check declarations cover their own completion condition by default; additional known
coveredConditionKeys are captured claims of that declared check's scope, not permission to waive
global instructions, explicit review or delivery. Review must account for the full task/conditions
and supplied required evidence, even when no finer semantic condition split exists. Acceptance
joins observed checks with required review/conditions; it does not pretend arbitrary prose has
been deterministically proved. Literal direct proof is the only sufficient shortcut in005.

An observed failed required command means actually started and completely settled execution
with nonaccepted captured exit code and complete applicable output/state, not launch/drain/
timeout/storage/cleanup failure. A valid changes_required review with inspected required evidence,
blocking findings and no gaps may trigger correction. Incomplete/invalid reports do not. These
are evidence triggers under existing one-cycle scope, never new authority.

Native review uses a fresh process/context and role-specific supported access. Candidate files,
manifest/index and required original/evidence are actually materialized/verified, not merely
named in prose. Supplied material/access proof and reviewer-reported inspected paths are distinct
facts; neither claims to observe a model's internal reading. Complete original instructions are
preserved. Unrepresentable required context is a named gap, not silently clipped material.

The private fixed review schema includes `schemaVersion: "005-review-v1"`, exact selected subject,
conditionsSHA256, inspected evidence references, findings, gaps and resolvedFindingIds. Reports
must be whole strict JSON with matching bindings, known supplied references, unique bounded keys,
valid condition/path references and no contradictory approval with blockers/gaps. Server-assigned
stable finding IDs are separate from model keys. No extra command/policy/budget fields are accepted.
Full report bytes remain a stage-owned artifact; previews never replace them. A final response
without valid structured review remains a response, not review approval. Required resolution also
needs applicable affected checks/review; a resolved-ID list alone does not close blockers.

CheckDefinition kind is builtin|command. Builtin `literal_document_replace/1` has no fabricated
process/exit code and completes under an owned no-child local_check attempt. Command definitions
capture ID/source/recognizer, exact executable/argv, managed-relative working directory, scope,
coveredConditionKeys, timeout and successExitCodes(default[0]). Timeout1–1800 seconds is additionally
capped by the common window. No definition can increase the task clock or waive a condition.

Public explicit check declarations contain only the following client fields; IDs/digests/observed
program facts are assigned by Council and cannot be submitted as trusted proof:

```json
{
  "key":"project-tests",
  "executable":"go",
  "arguments":["test","-count=1","-json","-mod=readonly","./..."],
  "relativeWorkingDirectory":".",
  "scope":{"kind":"whole_candidate","paths":[]},
  "coveredConditionKeys":["project-tests"],
  "timeoutSeconds":300,
  "successExitCodes":[0],
  "required":true
}
```

Keys/condition keys are unique1–64-byte lowercase alphanumeric/underscore/hyphen values. Relative
cwd is dot or a canonical managed-relative ordinary directory. Scope kind is whole_candidate or
declared_subset; at most256 canonical relative paths and16 condition keys, captured before execution.
Whole-candidate has empty paths; subset requires a nonempty explicit sufficient scope. Omitted
scope conservatively means whole candidate. Success codes are1–16 distinct integer values0–255.
Executable is a native absolute path or one of the recognized go/cargo/node names, resolved to an
explicit native path. Arbitrary script/wrapper paths require an explicitly declared supported
native interpreter and script-file argv, not shell execution. Omission defaults never erase
explicit malformed/empty/null values. A command declaration cannot supply a builtin result.

Convention selection uses captured ordinary project metadata in the selected project root:
go.mod selects Go; Cargo.toml with the applicable lock selects Cargo; a package.json test script
exactly declaring direct Node tests selects bounded explicit regular test files from the retained
manifest. No shell/glob translation, unbounded discovery or invented substitute for an unsupported
script. Every selected convention records marker bytes/hashes, recognizer and exact argv. More
than one applicable language convention may produce required checks; source-changing definitions
or unknown required scope remain an explicit gap. Sufficient literal proof bypasses unrelated
project test suites only when no explicit task condition demands them.

The closed review DTO below uses all required top-level fields. Findings have no server ID until
validated; key uniqueness, known condition/evidence references and all bounds are enforced locally.

```json
{
  "schemaVersion":"005-review-v1",
  "subject":{"kind":"candidate","id":"11111111111111111111111111111111","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
  "conditionsSHA256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "outcome":"approved",
  "inspectedEvidence":[{"ref":"candidate-manifest","scope":"whole_candidate","paths":[]}],
  "findings":[],
  "gaps":[],
  "resolvedFindingIds":[]
}
```

Subject kind is candidate|response. Candidate subject ID/hash bind the selected candidate ID and
closed manifest SHA256; response subject ID/hash bind its selected final-response artifact and
bytes SHA256. The controller retains the full CandidateBinding or ResponseBinding including
source/index/producer/conditions and checks exact correspondence; model-supplied facts are not
trusted proof. Review, check and resolution records use exactly one candidate-or-response binding.
Ordinary candidate roles cannot claim a response subject to evade source review, or vice versa.

Outcome is approved|changes_required|incomplete. Each supplied evidence ref is an opaque1–128-byte
key in Council's supplied access manifest, never an arbitrary path/URL. Inspected scope is
whole_candidate|whole_response|declared_subset, matching the selected subject, with required paths
array (empty for whole scope, nonempty captured paths for a subset). Findings contain
`{key,severity,conditionKeys,locations,summary,evidenceRefs}`; severity blocking|advisory. Locations
are `{path,startLine,endLine}` with all three fields required; an empty locations array is allowed
for a condition-only finding whose nonempty conditionKeys identify its subject. Paths are canonical;
ranges are positive integers through2147483647 with endLine>=startLine. Summaries/gap reasons are1–4096 UTF-8 bytes; gap entries
are `{key,conditionKeys,reason,evidenceRefs}`. Up to64 inspected refs,64 findings,64 gaps and64
existing resolved finding IDs fit the full1MiB result bound. Required evidence not reported inspected
creates a gap; approval with blockers/gaps is rejected. changes_required requires a blocker;
incomplete requires a gap. Author-created IDs, unknown references and generic resolved labels
cannot close existing findings. Data content never creates an execution command or authority. The native private schema uses the
strict supported subset: every object has additionalProperties:false and every property in required,
including each nested subject/inspection/finding/location/gap. No nullable optional fields or oneOf
are needed. All arrays are present (possibly empty where allowed). Go revalidates complete shape,
all bindings/semantics and bounds; a provider schema request is not Council acceptance proof.

Supported recognized commands are native Go test, Cargo test and direct Node test forms enumerated
in research.md. Arbitrary npm/run/test shell scripts are unsupported; invoking npm-cli.js does not
remove its script shell. Explicit native executable/script-file declarations remain possible under
captured scope. Plain command receipts retain exit/output without inventing test counts/coverage.
No TAP/prose parsing creates observed tests or approval. A marker/manifest file alone is not proof
that the program ran. Launch failure is unavailable, not-run is not passed, and all command
success requires root/input/output/tree settlement and complete durable artifacts.

Each receipt binds exact candidate/source/index/conditions/check definition and before/after
state. Source-modifying checks retain a new candidate and invalidate affected evidence. Reuse of
unaffected evidence requires an identical captured subset applicability basis; otherwise recheck
the relevant required evidence conservatively. Historical is applicability, not a rewritten outcome.

## Response-only compatibility

An Auto response_only result with no worktree/index delta, no required evidence, no explicit
check/review/external action and no other captured condition pauses at the existing exact
response_acceptance_pending action owned by the user. Unknown wording alone does not add a model
review or command. Saved local-application preference creates no fictitious no-op integration.
Existing AcceptResponse performs the same fresh artifact verification/confirmation and records
the unchanged ResponseAcceptance; the current workflow completion separately references it as
response_only_user_review. It creates no candidate WorkflowAcceptance or integration claim.

Required-evidence response-only workflows retain a ResponseBinding: producerAttemptId(author or
correction), exact final-response artifact/hash/length and unchanged source/index/condition basis.
Review/check applicability includes that response digest as well as source, so corrected text
cannot reuse a stale review merely because files stayed unchanged. Required checks/review remain
conjunctive. Once qualified, the same existing response-acceptance action is the user's next step.
A narrow current005 branch binds the selected response producer rather than the latest supporting
review/check attempt, verifies all satisfied requirements and known settlement, and records
completion kind response_only_user_review_with_evidence plus immutable evidence-use/resolution
references. Old standalone guards, fingerprints, replayed envelopes and ResponseAcceptance JSON
remain unchanged. An authored/index delta cannot enter either response-only branch. A demanded
local-integration outcome with no applicable delta is a contradictory gap, not invented success.

Human acceptance after the execution window expires remains possible for a current eligible
settled result: it launches no stage or effect, grants no budget and still performs fresh evidence/
control checks. External declarations can qualify only their captured command_completion result,
not fabricated remote state. Task detail selects the deliverable producer's response; a later
reviewer's structured report is not displayed as the user's new answer.

## Local application and uncertainty

Each operation owns a dedicated no-process local_application attempt, destination manifests/file
artifacts and append-only per-path intents/outcomes. No later evidence is appended to the settled
author. Intent commits before each effect, with filesystem work outsideSQL. Preflight verifies
candidate/required evidence, captured scope, project/repository/branch/control identity, affected
paths/types/content/index and supported operation bounds. Unrelated destination bytes/index are
preserved; no blanket reset/stash/merge, automatic commit or silent fallback to another destination.

The resolved algorithm in data-model.md supports ordinary single-link text create/replace/delete
under existing parents on the same filesystem as private state staging/quarantine. It uses checked
directory handles/FDs and native exclusive no-replace rename, retaining displaced originals/live
quarantine. It refuses unsupported metadata/streams/ACL, parent creation, index-only/mode/type/
redirect changes, cross-volume or unavailable exclusive rename instead of copy-delete fallback.
Before/after/conflicting observed bytes remain evidence; a live external file handle is not claimed
frozen. Replacement can have a brief missing-name interval, and concurrent name recreation is
preserved as partial/conflict. No universal atomic multi-file or editor-exclusion claim is made.
An unsupported/conflicting delta is retained with a precise remaining action. Partial or
possibly applied effects are not rollback/success; missing acknowledgement becomes uncertain.
Do not blindly restore backups over later user edits. Restart performs no effect replay. The
operation and all known effects remain inspectable; write recovery beyond this scope belongs007.

Review-only completion records qualified candidate delivery and its remaining apply action with
no local write operation. Local application completion requires known applied operation and matching
fresh destination/evidence. External command completion is separate and never labels local files
integrated or remotely published. Unknown effect/cleanup/persistence fences prevent false acceptance.

The serialized before-effect boundary is one durable guarded in-flight permit. The controller
checks current authority/Stop/window under control, commits that action's intent/permit, then
releases control before native filesystem I/O. Stop stays responsive and prevents every later
permit, publication, advancement or acceptance. An already permitted action may settle afterward;
its actual partial/uncertain result is recorded without changing primary cancellation/failure.
Quarantine and publication require separate permits. Ownership remains held until the in-flight
action settles; timeout/restart cannot spawn a replacement worker or replay it.

## CLI and UI mapping

- `workflow preferences get|update --delivery review_only|apply_verified_local --expected-revision R --request-id ID`.
- `tasks start ...` defaults Auto; optional `--workflow auto|author_only`, `--delivery MODE`,
  `--delivery-file FILE`, `--checks-file FILE`, `--review auto|required`, `--max-corrections 0|1`.
- `--delivery` and `--delivery-file` are mutually exclusive. Delivery file is exactly the delivery
  DTO above; checks file is a strict array of at most16 declared definitions. No shell interpolation.
- `tasks workflow TASK_ID`; `tasks steps|reviews|checks|findings|deliveries|candidates TASK_ID`
  with optional mutually exclusive workflow-id/task-version selectors and list limit/cursor.
  `candidates changes CANDIDATE_ID` gives full paged change metadata.
- `tasks workflow-adopt TASK_ID --expected-revision R --task-version V` with captured workflow options.
- `tasks advance TASK_ID --expected-revision R --workflow-revision W --task-version V --request-id ID`.
- `tasks check TASK_ID --check-id ID --expected-revision R --workflow-revision W --task-version V --request-id ID`.

Existing task/budget/grant/routing/artifact/Stop commands remain. Mutations use retained request IDs;
new scalar booleans are explicit true|false if introduced. CLI JSON is one envelope, diagnostics stderr.
UI shares these operations and frozen dependent request identities. Ordinary Start requires no
check/action configuration. Optional details expose reasons, saved prospective delivery scope,
explicit declarations and finite budget. The board shows next owner/action and distinct authored,
reviewed, verified, delivered, applied and external-command outcomes, with full evidence links.
Actual keyboard/focus,360px,reduced-motion and approved asset hashes remain acceptance gates.

## Error and qualification boundaries

Reuse invalid_request/revision_conflict/write_uncertain/budget and owner fences. Named workflow
errors distinguish authority_required, workflow_revision_conflict, evidence_unavailable,
review_unavailable, check_unavailable, candidate_stale, integration_conflict,
application_unsupported, delivery_scope_required, external_postcondition_unsupported and
effect_uncertain. Public messages state a concrete next action; partial outcomes never imply success.

All automated tests use real isolated local fixtures, including native review roles, required
checks and scoped publisher. No vendor auth/help/version probes or live calls. Native metadata,
quota/model/billing and savings stay unqualified/unknown as previously declared. Actual final
source, independent reviews, source/dist correspondence and seven terminal hosted jobs are required.

### Fixed offline fixtures

Public fake workflow scenarios are `workflow-document` and `workflow-defect`, selected through
the existing --fake/--fake-scenario path. They always launch the current Council executable's
fixed native-protocol fixture, never an installed vendor. workflow-document edits the exact
recognized ordinary document operation; workflow-defect writes a bounded known validation defect
inside the managed fixture project. The workflow controller chooses the role; a scenario string
cannot select an arbitrary stage or grant authority.

Internal role cases are `workflow-review-blocking`, `workflow-correction` and
`workflow-review-clean`. They read the actual selected managed candidate and current supplied
review bindings/finding IDs: report the known defect, correct it, and qualify the corrected bytes.
They emit the selected Codex/Claude protocol and strict matching review data; unchanged-original
or merely reported corrections must fail the meaningful fixtures. They are not standalone public
review/approval commands. All observations remain labelled fake/unmeasured.

The fixed local verifier child is the same executable's `__fixture check` with an explicitly
captured managed-relative file/rule; it returns failure for the actual bad bytes and success for
the corrected bytes. The fixed local publisher is `__fixture publish --destination ABSOLUTE_FIXTURE_PATH`,
declared as an explicit external action with command_completion basis. It records one attributable
effect in a disposable authorized fixture destination and returns labelled output; it never
publishes remotely. Native packaging tests resolve these fixed children to the extracted binary
without Node/npm/Go runtime prerequisites. No fixture path can fall back to a vendor or invent a
real quota/token/charge/test-count observation.
