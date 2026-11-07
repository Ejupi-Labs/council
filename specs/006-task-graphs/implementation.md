# Task graph implementation ledger

Status: active, incomplete and unaccepted. No007 implementation is authorized.

The immutable pre-code design/task gate is
`c9b8a0d63730ddb04236f9b31c7182026b062d1f`, following independent closure of all eleven design
findings,34/34 requirement mappings and14/14 specification checklist items. Physical branch is
`refactor`; main remains `27822baa3aee663366a870bde7fb1190649d3418`.

## Foundation ownership

- Core: domain DTO/validation and store migration/budget/capacity/control/records.
- Native: workspace graph closure/materialization/composition and graph native context transport.
- Root: fixed offline graph fixtures (six scenarios across both supported fake native profiles).
- Client: typed UI/CLI/metadata and later actual client journeys.
- Root: per-attempt engine ownership, graph controller, application/HTTP, packaging and one
  serialized local test/build/browser queue. No vendor calls or probes are authorized.

## Observed focused checks

| Scope | Actual command/outcome | Limits |
| --- | --- | --- |
| T009 initial contract | `go test ./internal/engine -run '^TestActiveWorker' -count=1`: expected compile RED for missing per-attempt map/cancellation helpers. | New contract not implemented at this point; not a runtime failure. |
| T002 frozen accepted005 fixture | `go test ./internal/store -run '^TestGraphSchema5FrozenFixture' -count=1 -v`: PASS, store0.548s, actual fixture case0.43s. | Constructs/reads frozen populated schema5 before any006 migration behavior; does not qualify migration. |
| T004 initial staged closure | Four focused `TestGraphIndex` cases compiled and executed: three failed as expected against ErrBlocked stubs; changed-index rejection passed only as stub rejection. Package5.575s. | Meaningful tests-first RED; implementation/GREEN still pending. |
| T003 declaration contract | Initial genuine domain RED0.092s for DAG/presence/body/proof guards; corrected full TestGraph family PASS0.082s, including root projection, claim tuples and Native seals. | Remaining existing DTO projections/controller facts and foreign/full regression stay open; task not yet complete. |
| T005 migration contract | Actual disposable frozen5 migration RED2.440s: version stays5 and explicit fault boundaries disappear under stub; unknownDDL and literal fixture tests pass. | No production migration behavior qualified. |
| Canonical graph seals | Five meaningful tests first RED0.072s on explicit unavailable stubs; unchanged tests then PASS0.077s. | Platform independently closed bounded source review; physical closure/authority are separate facts. |
| T016 root capture helper | Positive dirty/staged/subfolder cases first stub RED2.436s; unchanged three filesystem cases then PASS5.905s. | Root independently reviewed actual source; independent initial-input retention is still outstanding, so T016 remains open. |
| T017 receiving materialization | Actual stub RED7.798s: complete closed-input/distinct-staging positive unavailable; two negative cases pass only stub rejection. | Implementation/GREEN, private receiving identity and actual preGO verification remain pending. |
| T014 scoped conditions helper | Pure receiver/scoped/effective hash and nine contradiction cases PASS engine0.098s. | Platform independently closed this bounded source review; actual packet/original retention/native access integration remains pending. |
| T024 browser metadata | Build passed; actual shipped client first RED1.809s against006 owner, corrected same browser test PASS1.512s, zero skips/cancellations. Corrected Go metadata checks PASS0.194s. | Client metadata scope only; source DTO additions and real graph journeys need final build and broader qualification. |
| T002 literal correction | Corrected frozen005 JSON/literal inserts/old graph-field absence assertions: actual smoke PASS store0.379s, case0.30s. | Root foreign source review closes IR006-01; migration still pending. |
| T004 corrected closure | All four unchanged actual filesystem cases PASS, workspace7.515s. | Root and platform foreign source review close IR006-03. Only staged-closure scope; full graph/native qualification pending. |
| T024/parser expanded negative contract | Actual combined metadata/parser tests compile and produce expected RED CLI0.142s: ten impossible claim tuples accepted, graph command positives unsupported. | IR006-02 predicate/parser implementation and GREEN remain pending. |
| T004 first implementation | Same actual four-case queue: three positives still RED on managed-index/raw-basis mismatch, package5.795s; changed-index rejection passes. | IR006-03 records correction; no relaxation or GREEN claimed. |
| T024 CLI metadata | Focused metadata/old-owner Stop tests: PASS CLI0.147s. | Decoder contract only; IR006-02 adds later impossible-tuple cases, browser/real owner qualification pending. |
| T009 + artifact access | Focused active-worker/closed-artifact/Stop controls: PASS engine2.822s. | Actual closed-artifact producer/kind/hash/length and bounded full-byte tests, not complete graph acceptance. |
| T009 first ownership change | `go test ./internal/engine -run '^Test(ActiveWorker\|StopBudget\|Shutdown\|Close)' -count=1`: PASS, engine2.089s. | Exact cancellation/all-owner handles plus selected existing controls only. Two-slot store admission, real concurrent workers and full regression are still pending; T009 remains incomplete. |

The source now retains cancellation handles per actual attempt until that worker exits; an
individual Stop leaves siblings owned and a shared fence/expired Stop budget reaches every
owned worker. No claim is released merely by cancellation. The unchanged accepted store still
limits actual managed execution until the separate capacity migration is implemented/qualified.

Any subsequent DTO/detail refinement is recorded in its normative contract before that seam is
implemented. Current metadata now fixes the bounded two-claim projection. Its initial engine integration is
authored; client metadata tests produced a genuine expected RED (CLI0.114s), with current0/1/2
pools rejected, a wrong current one-slot pool admitted for maintenance and known dev.5 Stop
missing. Client implementation/GREEN remains pending. IR006-01 reopens the frozen-fixture task
until historical receipt shape is independently fixed; its initial smoke result remains real. Test results here do not imply completion of any GRA/SC or the feature-wide gates.

## Subsequent observed checkpoints (2026-11-06)

These outcomes supersede the earlier pending statements only for the named scope. The feature,
real graph dispatch, all client journeys and final native qualification remain incomplete.

| Scope | Actual outcome | Limits |
| --- | --- | --- |
| Domain/store/routing graph foundation | Full focused `^TestGraph` family PASS: domain0.103s, store3.795s, routing0.080s. | Schema6 migration, fixed schema5 preservation/faults, bounded claims, canonical creation/shared budget and pure capacity; not native graph dispatch. |
| Workspace receiving materialization | Corrected observed inventory and exact source-basis cases PASS9.091s; handoff observer's three physical cases PASS15.776s. | A Windows fixture initially compared lazy file IDs after rename; corrected to eagerly captured native directory identity. Product check was not relaxed. |
| Workspace closure/context/output | Required-access regression first RED4.143s; corrected combined retained input/context/output family PASS33.980s. | Exact original/acceptance/contribution access coverage is now checked in both builder and verifier. Store admission/terminal association remain separate work. |
| CLI immutable intent | Full current `^TestGraph` CLI family PASS1.210s; direct native private-cache cases PASS0.417s. | Initial implementation compile failure and null-bearing response decoder RED0.514s were corrected. Mutation strictness remains unchanged; response decoding accepts nullable response fields with unknown-member rejection. |
| Application/HTTP graph transport | Focused app PASS0.177s, HTTP PASS0.146s after route/body-limit genuine RED0.079s. | Typed/raw transport and reads only; engine mutations were not yet connected. |
| Fixed graph child processes | `go test ./internal/fixture -run '^TestGraph' -count=1`: PASS5.433s,14 owned child-process cases. | Six fixed scenarios across Codex/Claude plus cancellation, actual join inputs and retained partial failure. No live provider or final graph workflow claim. |
| Root creation facts/helpers | Focused engine graph facts/artifact/scoped-condition family PASS3.712s. | Actual read-only creation facts and bounded helpers; no graph execution acceptance. |
| Prepared context transport | Authored-first stubs produced workspace RED17.260s and adapters RED0.171s. Corrected same selectors PASS workspace11.320s, adapters0.093s. | Private complete copies, native prompt/reference bounds and exact Claude context directory; real graph worker preGO use remains pending. |
| Local graph completion DAO | `go test ./internal/store -run '^TestGraphLocal' -count=1`: genuine RED0.369s against unavailable operation/completion. | Implementation underway; no success yet. |
| Local graph root runner | `go test ./internal/engine -run '^TestGraphLocalRoot' -count=1`: genuine RED1.861s, input remains preparing without runner. | Actual dirty bytes/raw-index no-native test; runner implementation underway. |
| Composer browser contract | Fresh `npm run build` PASS. Actual one-browser test RED2089.502ms (overall4282.543ms), missing optional group entry. | UI implementation underway; no skip or cancellation, no successful graph mutation claimed. |

Independent bounded source reviews found no remaining concrete P1/P2 in client intent binding
(Core), native cache privacy (Native), Root application/HTTP/graph reads (client), or receiving
physical closure helpers (client). Root has additionally read the actual migration/claim and
prepared-context implementations. These reviews are scope-limited and do not close T053.

## Owner preparation and UI follow-up

- Root operation effects/proof: genuine RED15.232s, unchanged corrected focused workspace family
  PASS15.573s. Client independently closed the wrapper/proof source. Intent is closed before
  capture; opaque capture errors remain unknown and known completed snapshots survive later errors.
- Current local/artifact/membership/creation DAO family: PASS0.763s after six ordinary bypass
  cases first failed0.492s. Child ledger names the root budget explicitly. T006/T010 remain partial
  until all graph proof/epoch/native admission paths are integrated.
- Actual root runner and strict mutations: initial dirty-root RED1.861s; later known base/branch
  drift negatives failed in the18.612s engine queue. Corrected combined local/mutation family
  PASS16.000s. Root SQL proof facts retain bounded closure anchors after every actual member is
  verified; no required payload verification is truncated to fit the SQL fact bound.
- Owner-level ordinary scope: actual root/node cases with an unavailable selected project first
  failed2.382s, then all12 passed2.464s with post-replay/pre-prerequisite scope checks. Atomic
  transaction guards remain independently required.
- Expanded routing usage case first failed0.053s on local_graph; corrected PASS0.089s, keeping
  local counters/prices out of native model measurements.
- UI builds passed twice after the composer checkpoint. Optional composer browser passed2741.968ms,
  then3114.932ms with expanded tests. Its mutation gate remains private and disabled pending full
  native/control integration. Two-claim capacity initially failed6762.651ms; later30s cancellation
  is retained as unresolved, not success. Denied-cache two-project draft case failed3662.752ms.
- One attempted build overlapped an author's in-progress root wrapper and failed on an unused
  JSON import. The author completed/froze that source and subsequent normal builds passed; the
  partial compile is not runtime qualification. Future builds must wait for all Go dependency
  writers to publish a compilable frozen checkpoint.

Current new receiver-original retention is authored in internal/engine/graph_originals.go but
has not yet been connected to a real node-input operation or qualified. It must not be counted as
complete node context delivery, source inspection or a successful native call.

The next unchanged browser checkpoint passed all three cases (overall9851.668ms, zero skipped or
cancelled): capacity1846.661ms, editor2926.070ms, denied-cache project drafts3789.834ms. The capacity
fixture now opens the real collapsed panel before its bounded5s clicks; no timeout was increased.
Root independently read the exact current capacity predicate and project-keyed draft memory.
The pending creation target is now /graphs; actual POST/retry integration is still unqualified.

Stop/read + membership/local/artifact DAO checkpoint passed0.708s after the three Stop/read
positives first failed0.210s on unavailable stubs. Root's foreign Stop source read found IR006-13
prior-outcome/late-no-op gaps, which remain open despite the narrower green tests. The next
BeginGraphInput DAO test first failed0.453s; implementation and real two-node engine retention
qualification remain pending. The pure causal composition cases first failed0.106s on the explicit
unavailable reducer; implementation is underway and no fan-in success is claimed.

## Closed independent inputs and composition checkpoint

Real independent root/input/capture qualification passed5.345s, then expanded retained convention/
saved preference/scoped conditions cases passed4.927s. The final two-graph/four-receiver empty/
nonempty-shared guard family passed9.997s: opaque shared text cannot become deterministic proof
merely by hashing; empty shared text preserves the existing narrow proof eligibility. Native/client
foreign source reviews closed these initial-receiver helper scopes. No v2-guidance or native agent
execution is implied. T016 is complete in its exact root/independent-retention scope.

Pure causal composition first failed0.106s on stubs, then passed0.120s. Two independent symlink
false-conflict findings each produced real0.083s regression failures. Corrected exclusive pure
family passed0.083s with independent client closure; T028 is complete in pure reduction scope.
An earlier prefix selector accidentally included new GraphComposedInput physical stubs and
failed17.721s; that failure is retained rather than relabelled as the pure pass.

Physical selected-output/composed-index cases first failed16.978s; corrected two-case family
passed61.951s (positive35.69s, conflict26.10s), with actual selected loose-blob restoration and no
workspace on conflict. Existing public index regression passed25.510s after object-core extraction.
Selected-context omission then failed11.414s, corrected positive/loaded omission/authority cases
passed14.634s and received foreign closure. T029 controller/current acceptance integration remains
open. These timings do not establish a performance regression or optimization claim.

Actual browser root registration/read/shared budget/full originals/pending GraphRef/Stop journey
passed5904.522ms after a fresh normal build. Earlier first-Stop200 assertion failed6063.206ms and
was corrected to contractual202 plus reason accepted; no product behavior was relaxed.

Opt-in actual managed reservation, real release provenance and late all-terminal group Stop
first failed routing0.065s/store0.508s. Corrected routing0.155s/store1.734s family passed and client
foreign source review closed the scoped findings. Standalone selection keeps its existing queue
semantics; graph selection can wait without native debit. First-author atomic admission remains
explicit unavailable until its own contracts/guards are implemented.

The fixed context-read worker initially had a Windows fixture privacy-open precondition failure
0.121s. Using the same anchored private handle path as real preparation corrected setup; both real
owned native-profile positives then failed stub9 in2.191s. Corrected full context family passed
2.661s and Native independently reviewed source. These are actual offline reads, not provider or
review acceptance telemetry.

Root added current-owner automatic **local-only** independent-input progression. Its real initial
contract failed in the23.796s combined engine queue (ready basis, nodes still waiting). After the
callback implementation its case passed within a19.053s queue whose only failure was the separate
new output-producer stub. Clean focused terminal qualification is still pending. Native admission
is absent from this callback; ordinary execution is explicitly blocked for graph work until exact
receiving/context/output guards are connected.

## Native receiving/output checkpoint (still partial)

Clean retained-input/context/owner-local helper family passed28.920s. First-author atomic admission
DAO cases passed0.609s and exact materialization association cases passed0.942s. Those DAO cases
are metadata qualification; they do not prove a launched agent. Current-owner progression still
performs local-only preparation, and public native graph execution remains gated.

Actual admitted producer output plus receiving-source materialization/atomic repeated receipt and
physical tamper denial passed36.117s (20.32s and15.49s). The actual output helper verifies selected
candidate and eligible response closures before terminal persistence; Root independently read its
full source and closure walker with no blocking finding in that helper scope. Actual first-author
execution, supporting review and same-terminal failure normalization require separate checks.

Two decisive store tests then failed0.640s: incidental noneligible prose incorrectly blocked a
selected candidate, and an actual failed native terminal retained a qualifying association.
Corrections are underway. These failures are retained and no graph result acceptance is claimed.

Actual mixed inline/file context delivery and cancellation after context-effects acknowledgement
passed inside a77.164s engine queue. Its four context cases passed0.82s,1.19s,13.32s and15.91s.
The same immutable executable produced genuine RED on first-author native execution and all three
pre-GO scenarios because the temporary executor gate still prevented Prepare (zero preparations).
Native-only executor integration is now underway; those failures are not evidence that tampered
files were launched. Root's receiving helper received independent Native closure for its exact
first-author receipt/repeated acknowledgement/physical source recheck scope.

The next store queue passed every selected/incidental-output, actual failed-terminal and unknown
context-effects claim-retention case within0.782s. Its sole failure was the newly added real
worker_started observation test: the node-admission anchor lacked paired first-GO facts. That
correction is underway; later derived-stage ancestor-Stop qualification remains separate.

## Initial native controller and terminal guards

The actual current-owner admission-only controller first failed5.68s on its explicit stub, then
passed6.00s: two genuinely retained ready inputs became two exact distinct-context claims under
one root allowance; observing the same completion again preserved attempt IDs and debit. The actor
was stopped, so this is admission qualification, not parallel worker or workflow acceptance.

Actual candidate-producing NativeRun passed9.89s and the three pre-GO source/context/accepted-Stop
cases passed36.44s. Two exact-output physical negatives passed8.09s and8.42s. Their combined76.373s
queue retained a failure in the newly added acknowledgement-uncertainty test: its assertion called
an eligibility guard that correctly rejects fenced owners. The test now inspects readonly exact
Gate.ActiveClaims instead, and passed10.88s without a production change. These first worker cases
use a response-only declaration with a file-producing worker; they qualify isolated execution,
retention and pre-GO controls, while the workflow correctly refuses task acceptance for that delta.

Known failure-only preparation projection, append-once capacity-wait callback consumption and
unknown-effects output normalization each produced genuine transaction failures in1.178s. The
corrected cases plus frozen-schema5/migration/fault/unknown-DDL cases passed within6.033s. That
queue's sole remaining failure was the now correctly prepared derived-stage test at the unavailable
common registrar. Its earlier missing-root-operation-ID setup failure is not counted as that RED.

A new actual stage-facts test likewise first failed its setup because response-only acceptance
cannot qualify authored files. Its declaration is now captured as candidate acceptance plus opaque
independent-review conditions; no task/version row was edited after capture. Precise missing-graph
stage facts and no-edit response dual-history qualification are running separately.

Stable semantic edge identity golden first failed0.101s and then passed0.120s; Root independently
read its fixed-order typed JSON/strict validation. The fixed graph-response worker first failed
unsupported selection0.122s; both actual native-shaped no-edit profiles then passed0.547s. No live
provider, subscription allowance or savings was measured.

## Derived-role and independent-branch checkpoint

The corrected exact stage-facts case passed11.32s, the actual no-edit owned response dual-history
case passed9.17s, and independent/unaccepted contribution selection passed10.17s in a clean30.769s
engine queue. Native independently closed the response caller fix and physical stage-facts helper;
current accepted-contribution journeys remain unqualified. The exact private response acceptance
reader now uses immutable acknowledgement ID/current completion instead of latest attempt.

The free-capacity graph-stage registrar passed its positive0.14s and six specific negative0.71s
cases within1.023s; the queue's only failure was the new owner-scoped passive live-enumeration stub.
Native independently inspected the registrar/root budget/current input/source guards and exact
response reader with no blocking finding in that bounded source scope. Durable waiting remains
incomplete and no automatic full graph journey is implied.

Actual two-node IR21 reproduced9.11s: B's local input runner receives cancellation before its first
read/I/O and settles known failure without fencing; A's real owned worker still succeeds, but its
next review facts reject the unrelated global registration suspension. Root and Core corrections
are underway. This actual failure is distinct from synthetic registry-state edits.

Actual derived prompt preparation failure retained its completed Handoff/source association and
passed11.65s. The real NodeStop-after-Handoff counterexample failed10.94s because the unassociated
receiving preparation claim was released; its precise default-known-effects correction is underway.
Those results share the31.804s queue and leave IR20 open pending the corrected affected run.

The new fixed graph-review worker produced genuine stub RED0.280s, then all four actual owned
Codex/Claude complete-context/candidate and changed-file cases passed0.593s. It reads every ordinary
manifest file and exact fixed graph result, preserves the workspace and reports no provider usage.
Its tests use explicit synthetic protocol identities; only subsequent engine tests can qualify
persisted dispatch/review/acceptance association. It is not live review-quality evidence.

## Live releases, real reviews and native namespace checkpoint

Live registration enumeration passed0.150s. Actual two-group release first failed6.04s because only
the source group was considered; corrected live-only fan-out passed6.99s. A real ordinary pinned
native-context reservation then exposed the missing waiting-review wake6.40s; its exact new-release
continuation passed6.90s while repeating the old author event created no work. Native independently
read both controller paths and actual fixtures. The automatic actor hook remains pending; these
cases explicitly invoke named real callbacks with the actor stopped.

Explicit required response-only review passed9.13s. Its earlier6.42s failure was fixture setup:
Auto correctly delegates a simple response to existing user review without another model stage;
the test now captures Review required at declaration and leaves that economical default intact.
Actual owned candidate review first failed9.47s using the old005 fixed reviewer, then passed9.131s
with the scoped graph reviewer; source mapping/route received independent closure.

The reviewer selected-subfolder counterexample failed1.164s. Pinned workspace-root argv and actual
cwd-relative selected mapping corrected it; eight root/nested positive/changed cases passed1.244s.
An intermediate compile caught a local variable name collision before runtime and was corrected.
Client and Native independently closed the exact mapping. These are offline wire/physical checks.

Namespace probe first failed8.464s on its stub, then passed7.894s including real copied-control
replacement and before/after metadata-commit lost acknowledgements. Root independently read full
owned-sentinel cleanup/identity/control/effects paths. Adaptive native composition first failed
8.581s, then passed10.671s; equal plans use no probe, known causal conflicts allocate no workspace,
and case-dependent plans qualify/reuse the same actual assembly. Affected original physical fan-in,
conflict and selected-context regressions passed22.536s without changed assertions. Root independently
read the shared finalizer/allocation/population extraction and native policy selector. Engine accepted
composition remains a separate unqualified integration.

Actual Store acknowledgement/outcome projection first failed0.618s across all four meaningful
cases, while real Engine candidate acceptance reached its missing node projection in8.62s.
The corrected projection/output/exact-ack suite passed1.660s. A foreign review then found IR23,
exact selected response after-index binding; its decisive same-producer distinct-index case failed
0.208s and correction is underway. No complete acceptance claim follows from the earlier narrower
projection pass.

Physical acceptance-proof collection first reached its explicit stub in9.24s. Standalone0.88s
passed; the corruption negatives remained stub-only until implementation. Client has now authored
fresh full working/input/closure/scoped-condition proof collection and both acceptance hooks;
actual restart/corruption/accepted-result qualification is pending the next queue.


## Accepted response composition checkpoint

The exact response-after-index IR23 regression passed0.10s within Store1.137s after its decisive
0.208s failure. Native independently closed the exact selected-index guard. Actual candidate
review/acceptance then passed8.859s, preserving original author provenance while the latest attempt
is the supporting review.

Acceptance proof collection passed27.910s across actual owner restart, broken output closure and
changed unstaged payload. Root independently read the collector and both acceptance hooks. Human
acknowledgement after a real Engine close/new owner and captured attempt timeout passed18.55s with
one exact event/replay, unchanged budget and user files. This proves attempt expiry, not common-window
expiry. Fresh-only acknowledgement continuation remains a separately pending controller seam.

Dependent Begin and completion guards passed1.399s and2.016s respectively after meaningful REDs.
Exact acknowledgement-event/old-completion rejection passed3.059s. Native independently read those
bounded transaction guards. Actual upstream source loading passed three cases3.37s/3.42s/3.42s
within34.928s, but the real acknowledged response-to-dependent runner still failed6.06s at its
unimplemented composition branch. Root implemented that branch using the adaptive physical composer;
the actual complete two-node response acknowledgement, source selection, dependent Begin and local
runner passed7.50s (Engine7.577s). Native found IR24 at the final effects-save boundary; this positive
result does not close that integrity race or qualify automatic advancement.

Typed conflicted completion first failed0.06s because its state was unavailable. Corrected positive
passed0.07s and missing-conflict/missing-proof/partial-source negatives passed0.16s. The same Store
queue0.428s retained genuine graph-grant stub REDs: positive0.04s and committed-after-fault0.05s.
Grants remain under implementation. The contract now fixes explicitly granted unopened-window
preparation authority at grant time plus min(300,newWindowSeconds); neither retries nor reads renew
it, and the existing first-native-admission common-window opening rule remains unchanged.


## Fresh dependent progression and last-save integrity checkpoint

Actual candidate author/review/acceptance reached missing dependent progression in8.47s, and the
actual response acknowledgement reached missing local input registration in6.39s. Generalized
current-owner completion and exact fresh response-acknowledgement callbacks now select complete
physical accepted sources under the existing current registration, then consume one per-target
transactional trigger. Candidate dependency passed10.77s and acknowledged response composition
passed7.45s. Client and Native independently closed this bounded controller/source scope.

The acknowledgement callback checks the exact receipt again under mutation control and invokes
continuation only after unlocking. Its full concurrent retry/read/restart/local-input journey
passed7.45s within Engine19.212s. An earlier restart-read503 was an actual test request context
cancelled by the old owner Close; separate owner/request contexts corrected that setup, with no
production guard change. Native and Root independently read the exact callback arm. These tests
stop the scheduler and call controlled real transitions; automatic actor integration remains pending.

IR24 reproduced composed7.61s and independent3.90s under real committed terminal-effects sink
callbacks, changing a non-anchor retained source_file in place. Rehashing complete payload after
the terminal save first corrected the independent3.89s arm; the composed7.93s arm correctly detected
the corruption but could not settle with ancillary unproved context observations. Clearing those
nonqualifying observations corrected it. The unchanged decisive pair passed7.96s/3.71s within
Engine19.212s, preserving actual effects and original working/index bytes. Native independently
closed the full corrected inventory and failure-settlement branches.

Graph grants passed all four functions in Store0.476s: epoch/root-ledger0.06s, commit faults0.11s,
fixed replacement-window preparation deadline0.06s and unsettled/fenced guards0.18s. Root independently
closed the bounded grant source. Private per-version execution options then produced genuine stub
RED0.06s and passed positive0.06s plus seven strict negatives0.42s in Store0.544s after implementation.
Root review found IR25, valid guidance exceeding the reused declaration-size record bound; reader
acceptance remains open until that boundary is corrected.


## Live actor, fan-in and parallel checkpoint

The exact fresh human response acknowledgement after real15s common-window expiry passed18.68s:
it retained acceptance without a dependent input, pending work, model debit or clock refresh. Native
independently closed that test's actual-window scope. The four-node acknowledged-response diamond
passed22.05s with one shared ancestor in the transitive vector, complete original access and exactly
four accepted authors; no duplicate model review was required by the captured Auto policy.

The public-create/live-actor file dependency first failed16.17s with actual root/input ready and no
native admission. Adding only the actual worker-completion call to existing guarded graph admission
made the unchanged complete author/review/dependency journey pass15.84s. Retained two-context
admission and real local-only preparation passed3.49s/3.73s in Engine23.158s. Native independently
closed scheduler ordering and the stopped-actor local-only boundary test, without asserting overlap.

IR25 valid large retained guidance first failed0.73s; separate64MiB serialized-version cap with
SQLite byte preflight passed a full48MiB version1.65s plus historical/strict/oversize cases in
Store2.977s. Root closed that bounded record-size issue. Legitimate routing-guidance validation
then failed0.04s (IR26) and passed0.04s after its vocabulary correction, together with basic explicit
Advance registration/old-source and scope guards in Store0.608s. Real version routing-policy capture
and rebind remain incomplete. IR27 actual claimed local scope after Advance failed0.15s, then its
scope/accepted Stop/source-access/old-trigger regressions passed Store1.394s. Root and Native closed
the precise removal of only future-registration generation/suspension coupling.

Two additional fixed divergence authors first failed their unsupported private GraphArgs contract
in Fixture0.080s, then all18 fixed native-shaped author cases passed1.33s. Their exact-file reviewer
positives first failed within2.88s while unchanged corruption negatives passed; fixed-content support
with retained full manifest/file verification then passed all24 reviewer cases2.76s (Fixture2.821s).
Native independently closed the bounded fixture changes. Actual three-node file fan-in passed15.77s;
actual independently accepted divergent working heads passed9.28s, retaining the exact typed conflict
and blocking receiver native work at four model debits. A subsequent source review tightened the
test's separate graph/input observation boundary by refreshing the graph after terminal input; that
small fixture correction still needs its affected rerun.

Actual default two-context worker overlap passed9.78s with both native ready barriers simultaneously.
Actual shared-declared-account serialization passed15.43s. The one-graph-slot case failed24.21s after
its first author failed and the other completed; the old run's exact child errno is not established.
Review identified a Windows test marker publication/removal race, and a test-only atomic closed-file
release plus richer actual process diagnostics is being qualified without changing8s child or15s
actor bounds. Do not claim all serialization qualification is complete.

Actual members endpoint positives reached explicit unavailable-route RED3.92s/4.02s; corruption
negatives still passed only that stub. A read-only physical member reader and shared strict provenance
helper are under implementation. Browser control/context tests are authored but unrun; the general
UI mutation activation gate remains false. No full006 build/test/check/package/native acceptance is
claimed by these checkpoints.


## Member reads and bounded parallel closure checkpoint

The failed one-graph-slot serialization case passed14.45s after the test-only closed-file atomic
release correction and explicit first-settled-before-second-started timestamp assertion. The full
affected family then passed default overlap9.74s, one-graph-slot16.02s and shared-account15.34s;
no fixed child or actor polling bound changed. Root independently read actual barriers, distinct
contexts/reservations, held contender and timestamps, real author/review acceptance and preserved
user source/index checks. The original failed run's exact errno remains unproven.

Shared provenance-member validation first reached its explicit stub RED6.294s; its real root/input
and closed-but-malformed provenance family passed6.255s. Root independently read exact reducer reuse
and scope. Engine member reads then passed positive3.99s, exact immutable cursor4.18s and actual
working/staged corruption refusal3.83s/3.79s. Public composed two-file member/raw-byte/original-author
and selected-head-seal assertions passed in the real live file-fan-in16.13s; the separate-observation
conflict fixture correction passed9.22s. Native independently closed exact historical source-head
closure/index joins and reviewed those real public member assertions. A separate files-only
reachability guard for mixed context/file origins remains under its own TDD.

The pure internal GraphFinalPlanFacts produced genuine zero-own-delta eligibility RED0.149s, then
all deterministic workflow tests passed0.078s. Native independently closed that bounded policy
extension: standalone/response/author-only behavior and review/check/correction guards remain,
node creation deltas stay truthful, and node-only direct proof cannot stand in for cumulative proof.
No Engine supplies those facts yet. The actual designated no-own-edit final producer then completed
in13.96s but failed its missing same-terminal final verification reference, providing the real
T041 capture RED. Final basis capture/current tuple guards/cumulative applicability/delivery are
still incomplete and must not be inferred from the pure policy pass.


## Final basis capture checkpoint

Read-only complete captured-output proof first failed its candidate/source-response positives in
Workspace18.582s. After implementation the full unchanged actual retained-only, working/staged
corruption and different-closed-envelope family passed18.465s. Root independently read the full
proof inventory and existing physical verification reuse; no mutable worker or invented acceptance
is required. The actual designated final with zero node edits then retained its separate cumulative
basis in13.49s, preserving node flags and root-to-selected working-only changes. The retained actor
and mixed-origin scope predicate passed15.77s/4.41s within Engine33.744s. That selector did not include
the prefixed member family; those physical cases remain evidenced by the preceding queue.

Native's foreign final-capture review identified IR28: raw final/cumulative artifacts carried full
private native root identities. A new actual raw-artifact assertion reproduced13.10s on projectIdentity.
The public proof root now uses GraphRootView plus the original basis seal; complete private root
lookup and physical verification remain private. This correction is unqualified until its affected
runtime/digest tests and foreign closure. The final basis remains a verification basis, not an
accepted review, qualified delivery or accepted final result. No Engine passes final Plan facts yet.

Fresh asset/app build passed7.58s before UI implementation. Three actual browser cases reached their
missing UI selectors: input/context9513.749ms, exact receiving dispatch14685.4904ms and node Stop/control
13056.7775ms (bounded5s selector failures, no timeout changes). Client may now implement those scoped
controls/inspectors; general Start activation remains gated. Store Guidance's first real frozen-policy
admission attempt correctly rejected a fixture missing complete guidance access; adding that exact
fixture source/ref/access without changing the guard made the decisive metadata case pass0.11s.
Public Engine Guidance/Advance and version-option capture remain separately unimplemented.

Subsequent focused qualification supersedes the preceding provisional statuses. All Domain
tests passed0.068s after the public final-root projection correction; actual no-own-edit final
capture passed13.03s, and Native independently closed IR28. The stronger Store final-output
guard passed its positive,17 resealed wrong-tuple cases, six fresh-proof cases, response-only
and nonfinal cases; the actual final capture passed again14.38s. These qualify retained final
basis capture only, not final cumulative review, delivery or acceptance.

The real public Guidance route passed3.51s and ordinary stale-version Run passed3.58s. An actual
new V2 receiving input then exposed reuse of the original declaration's workflow choices3.90s.
Reading the exact private version options and saved routing basis corrected this. The complete
affected Engine family passed25.966s: actual final basis14.33s, public Guidance3.61s, stale Run3.70s
and actual V2 workflow/routing capture4.23s. Native independently closed the three capture files.
Explicit public Advance remains a separate unfinished controller transition.

Store dependency rebind now creates the derived immutable version, provenance, copied captured
options/policy, new registration and real local input operation in the same transaction. Its
first positive reached every runtime assertion but compared typed and decoded receipt objects
in different Go representations. Normalizing complete wire envelopes and independently checking
unchanged stored receipt bytes corrected only that test assertion. The affected Store family
passed8.586s, including the positive0.26s, eight scope/proof refusals2.04s, commit-fault cases0.52s
and unchanged-completed-input refusal0.12s. Native independently closed the bounded source scope.

UI node Stop's exact task-version response join passed the actual lost-reply/control journey
8201.4325ms and received foreign closure for IR29. IR30's exact receiving inspector reproduced
acceptance of a receipt carrying a failure; complete receipt checks were added. A fresh app build
passed5.92s. The unchanged six-mutation browser test then passed its first four malformed receipts
and reproduced acceptance of an impossible calendar timestamp17813.9553ms. A UTC/calendar
correction is awaiting the same fresh-build/browser gate; IR30 remains open until that result.

Fresh explicit manual-stage resume has a real metadata RED: both current-owner and recovered-owner
review cases returned the existing unavailable capability, with seven scope/proof/Stop refusals
passing (Store1.005s). Its implementation and actual native Engine qualification are pending.

The strict UTC/calendar correction subsequently passed the unchanged three browser journeys
after a fresh3.23s app build: original/member inspection6159.1613ms, exact receiving six-mutation
inspection12292.1331ms and lost-reply Stop/grant/ledger9182.0131ms (total28769.9247ms). Native
independently closed the timestamp source correction; IR30 is closed in that bounded inspector
scope. General graph Start/Advance UI activation and comprehensive render/native gates remain open.

A new actual guided-V2 public Advance journey reached the unavailable route3.39s. Its controller
now captures exact root/input/contribution proofs outside SQL and submits only private facts;
ordinary fresh preparation, initial admission and manual-stage Store integration remain unqualified.

The controller then reached the ordinary-preparation Store restriction in the same actual V2
journey3.70s; it was not a successful Advance. The new cumulative-final loader test separately
reached its explicit unavailable stub14.36s after real accepted predecessors and a retained
no-own-edit final. Both concrete missing transitions remain active implementation work.

Manual-stage metadata implementation reached real current/recovered-owner stage admission and
preparation guards, then rejected a fixture missing a receiving-owned staged-blob envelope. The
fixture now registers that exact node_subject envelope/proof; the production materialization
guard was unchanged. The same two positives and seven scope/proof/Stop negatives passed
Store1.015s (current0.14s, recovered0.12s). Root independently reviewed committed Advance receipt
association, inherited current-owner authority and unchanged first-GO anchor identity. Actual
public Engine resume and unopened replacement-window/local-stage qualification remain separate.

Five additional shipped-client graph foundation journeys passed14288.2919ms: exact capacity and
maintenance metadata1102.9995ms, two/one/zero capacity1470.0072ms, composer2255.5177ms, denied-cache
project drafts2279.1359ms and actual registered group reads4889.2472ms. This does not activate
the gated general Start path or replace comprehensive browser/render/native acceptance.

After the ordinary preparation Store correction, its combined preparation/rebind/manual-resume
metadata family passed3.577s. The actual public guided-V2 preparation/replay journey passed3.79s.
Explicit stage capture then resumed the real reviewer after a grant: the native reviewer completed,
but the test compared read-time ObservedAt/RemainingSeconds projections in the old terminal.
Normalizing only those documented observe() fields preserved every persisted-field comparison;
the exact real review/manual-debit/no-author-replay journey passed8.75s (Engine8.826s).

The cumulative-final loader passed14.63s with real accepted A/B predecessors, true root-to-selected
delta, rejected node-only applicability and corrupted unchanged root member refusal. Root and
Client independently closed the bounded loader source. The expanded same fixture now uses five
units to stop at final-author settlement without racing an automatically launched review. It
reproduced missing cumulative progress and missing terminal next-stage proof15.44s: node-only
candidate_change_required and nil AdvanceProof. Root progress and Store terminal-plan integration
are required together before cumulative review can run; delivery/acceptance remain disabled.

IR31 independently reproduced unrelated-node Stop blocking pending A review: public route6.54s
and Store metadata0.12s, while own selected-node Guidance remained rejected6.57s/0.10s. The
correction is scoped to actual/prospective selected work and declared ancestors. Captured stale
unrelated tuples are retained verbatim, never refreshed into new authority. Runtime/foreign
closure remains pending.

Supplemental final-context TDD reached concrete missing behavior in separate layers. Actual
private one-directory copying failed4.65s at the explicit stub; combined-reference named overflow
failed4.85s because the stub returned a generic block rather than the required limit (Workspace
9.568s). Actual loaded-final context planning failed14.48s, and separately labelled semantic-only
before/staged inventory planning failed0.00s (Engine14.563s). Both owned fake native profiles
omitted the additional complete source reads, and twelve missing/corrupt/foreign/hash/input/basis
counterexamples reached their fixed action (Fixture1.975s). Adapter positive/profile/prompt-byte
tests reached its explicit transport stub; its purported exact64-reference baseline was already
over the ordinary limit and requires a fixture-only correction before qualifying that boundary.
These RED results authorize the corresponding narrow implementations; none is a native final
review, command-check or delivery pass.

Store final-plan TDD independently reproduced missing review proof0.13s and missing separate
cumulative direct planning0.11s; its supporting-stage case could not obtain that first review0.09s.
Missing final association and empty cumulative delta remained denied0.12s/0.11s (Store0.653s).
The common controller-only WorkflowStepFacts.CumulativeDirectProven is separate from node direct
proof, including builtin checks that produce no fabricated GraphOutput. Its Store derivation
and actual producer/role caller integration remain active work.

IR32's seventh exact-inspector mutation changed only materialization.sourceBasis.finalVerificationRef
on a real ordinary receiving receipt. The unchanged built consumer displayed it as exact rather
than retaining the known mismatch18052.1193ms; the preceding six mutations passed. Optional final
basis type/ref validation and exact presence/value comparison require fresh-build/browser closure.

The corrected Store final planner passed0.623s: actual review proof0.11s, separate cumulative
direct facts0.10s, invalid association/empty-delta denials0.10s/0.11s and supporting-stage nil-output
qualification0.12s. Root independently closed its bounded registered-binding/proof derivation and
unchanged node facts. The actual three-node zero-own-edit final then passed16.17s with both fresh
progress and the real terminal next-review proof. Actual context planning passed14.64s and the
separate semantic member reducer0.00s. Fresh grant/public Advance admitted the genuine final
reviewer, which ran one owned process but reproduced missing cumulative payload18.84s. Caller
transport integration remains required; this is not a completed cumulative review.

Private context copying and descriptor checks passed together with retained ordinary context
Workspace19.103s (new copy5.04s, combined-reference pre-effect refusal5.22s). Client independently
closed that bounded source. Both owned fake final readers plus twelve malformed sources and
existing candidate reviewer cases passed Fixture4.925s; ordinary original/context regressions
separately passed2.291s. Root independently closed the reducer/reader source boundaries.

After the adapter's fixture-only actual-ref baseline correction, the decisive65th reference
reached the missing transport stub. Implemented final payload/profile/count/encoded-byte behavior
then passed with ordinary adapter regressions0.137s. The byte test isolates64 actual refs and
preserves existing input_too_large behavior; context reference overflow remains context_capacity.
These are transport/profile gates, not actual live model consumption or final acceptance.

T041 implementation is complete in its cumulative-basis/planner scope: actual unchanged final
capture, private/public proof-root separation, full loaded current basis, pure planner facts and
same-terminal Store next-stage proof are implemented. Actual no-own-edit final progress/author
proof passed16.17s with unchanged node creation flags; Store positive/negative/supporting facts
passed0.623s. Native independently closed Root's progress/stage/explicit optional-applicability
arms, while Root/Client closed the loader and Root closed Store derivation. T042 fresh native/local
verification context, T043–T044 delivery/acceptance and all comprehensive release gates remain open.

The actual final native reviewer subsequently passed19.78s (Engine19.870s): fresh grant/public
Advance, exact basis dispatch, real owned process, full cumulative raw-source read tuples,
cumulative report applicability, zero own node delta and unchanged user working/index state.
Native independently closed the bounded caller source. Local command verification remains open.

Atomic explicit initial-admission Store tests passed0.519s: two contexts/root budget0.12s,
three-node known capacity waiting0.12s, fresh explicit pre-admission blocker retry0.09s and later
invalid-proof rollback0.10s. The public route then admitted and actually ran both current-input
fake workers, retained distinct receiving/output facts and replayed its exact receipt without
new work9.69s (Engine9.787s). No concurrent-runtime claim is made by that sequential worker case.

A fresh complete app build passed6.05s. All three browser journeys passed28018.8761ms, including
the unchanged seven receiving-receipt mutations12216.3271ms, complete input inspection5704.792ms
and lost-reply Stop/grant/ledger8879.3994ms. Root independently closed the optional final-basis
shape/presence/value join; IR32 is closed in that bounded client scope.

All-graph-node context-effects metadata first failed fixture validation because its bare command
name was unsupported. A portable recognized command/absolute captured test program fixed only
setup. The decisive case then passed known-zero effects0.13s but reproduced releasing the exact
local_check claim despite unknown context effects0.11s (Store0.300s). The stronger Store guard
must wait for coherent actual local known-zero/holder caller paths; no process cleanup inference
or local execution qualification is credited yet.

Actual local command tests subsequently reached their genuine admitted local_check after the
owned cumulative review, but the outer native-only graph executor gate stopped every case:
positive20.17s had zero Prepare/command calls; unknown-copy21.26s did not reach its acknowledgement
sink; descriptor-tamper21.08s had zero Prepare/tamper callbacks (Engine62.618s). Only the missing
local receiving/execution path is reproduced by that queue. The actual uncertainty and tamper
boundaries require rerun after their real guarded source/copy path exists; earlier metadata
unknown-ownership evidence is separate.

With the guarded local receiving/context path implemented, the real command descriptor/env/
complete-read case passed23.84s. Its actual lost-context-acknowledgement sink then fired and
reproduced releasing the exact local claim24.04s under the old native-only Store guard. The third
tamper case failed its earlier grant prerequisite15.39s because the fixture owner was fenced;
it did not reach descriptor tampering (Engine63.340s). Native identified the fixture lifecycle:
terminal SQL visibility and empty activeWorkers precede acknowledged continuation callbacks.
The fixture now joins actual workers/continuations under its original25s deadline, checks the
open gate/exhausted budget/no pending work under control, then cancels the scheduler. No sleep
or timeout increase substitutes for that acknowledgement boundary.

Graph-only replacement-window tests reproduced unnecessary model-author prerequisites0.09s each.
The new native-any-role clock opening and fixed-deadline zero-native local continuation passed
0.10s each, together with known-zero local release0.10s and unknown-context exact-claim retention
0.11s after generalizing the affirmative graph-node EffectsKnown guard. Native independently
closed the window-source arms; actual new-build unknown/tamper reruns remain pending.

The actual cumulative local delivery test reached a real reviewed final and claimed local_delivery,
then failed its real user-file assertion19.96s at the still-disabled outer graph application path.
No destination effects occurred. Store delivery tests first needed fixture-only captured-condition
registry and Plan Action apply corrections. Their genuine behavior counterexamples then showed
all14 resealed wrong bindings accepted at Begin1.83s, all seven missing fresh anchors accepted0.74s,
Ready replacing the binding0.10s and a permit accepting a different binding0.10s (Store2.870s).
The exact positive guard reached its explicit unavailable stub0.14s. Store bridge implementation
is now authorized by these failures; the outer application path stays disabled until its guard
qualification. Engine source/binding/application integration is being authored separately, using
the original graph root without changing candidate/node creation facts or the005 native algorithm.

IR31's correction subsequently passed the affected Store family1.633s, then the actual Engine
unrelated-Stop review admission6.82s and changed-selected-Guidance refusal6.96s. The same Engine
queue retained public guided preparation3.99s and actual grant/resumed-review execution9.41s,
total27.261s. Client independently closed Root's selected/transitive scope guard and Root closed
the Store transaction equivalent, including exact prior Stop-only revision and prospective
unfinished registration scope. No unrelated tuple is refreshed into current execution authority.


The current-source local final-context queue passed the actual captured command and descriptor
read path25.40s, and the actual lost-context-acknowledgement case retained its exact local claim
24.60s under the affirmative all-graph-node EffectsKnown guard. Descriptor tampering then reached
its actual Prepare-to-GO boundary and failed28.85s only because an unstarted command receipt
incorrectly retained a non-null empty StartedAt; no command GO, passing check or destination
change occurred (Engine78.945s). A graph-only timestamp correction is pending the same test.

The combined corrected execution/interop/routing/workflow migration, Stop-history and two-context
pool regression passed Store4.488s. Historical row/receipt bytes, exact held legacy ownership,
current schema6 empty graph authority and direct-stage fault snapshots remain separately checked.
Core and Native independently closed each other's bounded test corrections before this run.

The first current enabled graph delivery run failed20.82s before destination actions: controller
materialization/observed-index/effect receipt refs were required as SQL anchors but omitted from
the complete physical inventory. Client independently identified the same source gap. Root now
includes those exact returned refs alongside every workspace output member; the strict verifier
still rejects absent, changed or conflicting anchors. Actual application requalification is pending.


The graph-only unstarted receipt correction passed the unchanged real three-case local queue:
command24.85s, lost acknowledgement23.77s, descriptor tamper27.63s (Engine76.353s). Native
independently closed the predicate and retained ordinary005/success timestamp behavior.

Enabled cumulative delivery next reached Begin with known evidence refusal21.13s and20.84s,
leaving the gate open and destination unchanged. Source review identified missing final-envelope
and actual node-local Source/Root header proofs, distinct from graph root/input refs. Those exact
headers now receive fresh post-last-binding-save and Ready/every-permit checks, with bounded
conflict-aware proof merge and no alteration of the005 application algorithm. The actual case
then passed23.16s: both predecessor-authored files were applied once, original dirty source and raw
index stayed exact, final node own-delta flags/IDs remained truthful, and the operation retained its
sealed graph delivery binding. Native independently closed the complete bounded source repair.
Stop-between-effects and post-save corruption qualification remain pending.

StageFinalRef admission first accepted six missing/unselected/fresh-proof variants and preFS/GO
accepted two cache-association swaps (Store1.145s). Core's exact current selected-final guard passed
all affected stage/final/output/delivery families7.888s, independently source-reviewed by Root and
Native. Historical origin-filter fixture cases now enter through valid admission before explicitly
labelled historical cache-scope injection, retaining raw evidence and filter assertions.

Actual failed-command continuation passed real check execution, complete exit1 receipt/current
correction planning and explicit grant, then reproduced public Advance returning no stage26.88s
(Engine50.131s alongside successful cumulative delivery). Root now considers only an exact complete
failed local check with canonical proof for a correction; its actual rerun remains pending.


The narrow failed-command Advance correction passed the unchanged actual case25.95s
(Engine26.063s). Client independently closed the exact succeeded-or-qualified-failed-check
predicate, preserved canonical proof/control checks and refused operational failures/uncertainty.
This is actual corrective admission; corrective worker execution is not claimed by this case.
The full current Store suite then passed49.347s, including schema migrations/held legacy claims,
current origin filtering, final association, delivery guards and unchanged005 regressions.


The real Stop-between-effects test passed22.55s: one durable applied intent/result prefix, accepted
public group Stop, no second path permit, preserved original source/index and truthful partial
journal with no fabricated uncertainty or allowance debit. Root independently read the test's
actual committed prefix/Stop boundary. The actual same-inode/length non-anchor source corruption
after binding/plan retention first failed23.27s: no effects occurred and the exact claim was fenced,
but immutable unacknowledged plan fields prevented terminal settlement. Root now preserves the
readable durable graph prefix under uncertain proof too, without inferring Ready/effects. The
unchanged fault case passed21.66s (Engine21.753s); Native independently closed the small correction.
T043's implementation and focused qualification are complete; final acceptance/publication and
comprehensive feature/package/native gates remain separate.

T044 metadata qualification reproduced refusal of an actual reviewed/applied own-zero final
acceptance0.28s; missing/uncertain/mismatched delivery negatives passed. Pure-response completion
passed0.09s and mixed review-only acknowledgement0.17s, while mixed apply incorrectly accepted a
response summary0.21s. Intermediate declaration tests first needed fixture-only actual invalid_request
contract and valid captured test executable corrections; those failures do not establish a product
acceptance gap. Candidate and response acceptance corrections are independently owned.

The actual explicitly scoped final external stage/review/claim reached the disabled executor and
failed20.97s. The first guarded Engine path then failed22.02s before receiving acknowledgement;
Native independently identified that the Store artifact producer allowlist excludes the publisher's
receiving artifacts. Root owns narrow final-only external effect/producer guards, and Native owns
their actual registered-stage metadata counterexamples. No external command/live provider ran.

Both new real Start browser journeys reached the unchanged built UI's disabled Start gate and
failed7421.1061ms/7085.8962ms (15146.0123ms total). No POST or fixture bridge was reached. Client
activated only Start/retained-Start retry; exact /graphs route already exists. The test-owned
bridge preserves/captures the original UI body, forwarding only fixed fake-execution fields to the
real service; it never qualifies live vendor execution. Fresh normal build and unchanged browser
rerun remain pending; Advance/guidance controls remain separately gated.


A fresh normal build passed6.47s, then both unchanged actual Start journeys passed18511.049ms:
real owned fake worker/inspection9342.3146ms and real committed lost-ack/exact-request retry with
newer drafts8579.1611ms. The bridge modifies only fixed offline execution fields after recording
the exact original browser body. Root closed gate/path source; Native closed the bounded tests.
No live provider qualification is implied. Explicit Advance/guidance/rebind UI remains separate.

Candidate/mixed-response/intermediate declaration tests passed in Store2.341s after Core's exact
cumulative acceptance and Native's response/file-delivery boundary corrections. Root and Native
independently closed Core's candidate helper, while Root/Core closed Native's response helper.
The first actual physical cumulative application→automatic acceptance case then failed23.82s:
fresh cumulative final and graph_delivery proof were missing in the Engine envelope. A preceding
decimal-count test compile typo was corrected before any runtime result. Root added full current
final/original/contributor physical and node Source/Root header checks plus exact applied binding
proof; Native independently closed that nonexecuting collector.

External receiving artifact support passed Store1.160s, but Native identified that its first scope
negative mutated data.Authority without authority_data: those passes proved representation
refusal only. Corrected consistent private corruption/readback reproduced four genuine semantic
Prepare/GO failures (Store0.978s). The new separate live exact final/scope/current-cohort guard
and receiving-only producer scope passed the corrected acceptance/external family Store2.233s.
The artifact scope deliberately retains already attempted receiving effects after Stop; it grants
no execution. Native independently closed both bounded helper scopes.

The fixed literal graph-document declaration first failed structurally before any worker
(0.82s/0.69s/0.67s). Client added only the exact fixed owned source.txt original-to-updated worker
and literal fixture allowlists. Current-source actual Engine qualification then passed92.528s:
true two-node cheap cumulative document12.70s with exactly two native admissions and a fresh
own-zero final builtin check/acceptance; opaque final12.47s and extra receiver condition13.09s
correctly require review at the exhausted shared allowance; actual captured external command with
complete descriptor read27.95s; actual cumulative application→automatic all-required acceptance
26.23s. Native independently read the fixed worker/test seam; none is a live model task.

The same physical application followed by real owner Close/New recovery and a fresh user
acknowledgement passed23.12s (Engine23.217s). Old settled delivery execution authority remained
denied; exact selected candidate, all-required completion, original dirty/index bytes, allowance
and empty claims remained truthful. This case verifies owner recovery, not elapsed final-file
window expiry; the prior actual response acknowledgement expiry case remains separate.

Root subsequently found missing files-only ancestry in external publisher guards (IR006-40).
Matching recreated file bytes and context-only reachability cannot substitute for declared file
incorporation. The independent SQL-only counterexamples failed1.085s and actual physical
Engine refusal failed26.24s, including unintended Prepare/GO of the fixed read-only publisher.
Both guards now require actual files-only ancestry. Store external coverage passed2.008s and
the actual legal/illegal pair passed48.442s (21.13s refusal,27.22s legal). Core and Native
independently closed the bounded correction. No remote publication was performed.

## Live projection, concurrency and current client qualification

Two independent Store instances contested the last shared admission against one actual SQLite
file. The decisive case first failed0.237s because deferred read-to-write lock upgrade returned
storage failure instead of serializing. The pinned modernc driver supports an immediate transaction
mode: journal.go now sets only _txlock=immediate, preserving busy_timeout and existing transaction
boundaries. The unchanged case passed0.257s; Native independently reviewed the DSN correction.
A full Store run passed54.006s before the subsequent live-phase projection hooks.

The actual live branch Stop test exposed running=0/queued=2 despite two owned running processes.
Phase projection now follows the exact current ClaimAttempt/validated observation transaction,
without changing control, budget or dispatch authority. Historical/stopped/accepted/exceptional
work retains its state. The six matching Store cases passed0.500s, including transaction rollback
and accepted Stop preservation. The actual live Stop/blocked follower/independent completed
review journey passed9.24s (Engine9.311s). Native independently closed the phase helper/hooks;
the test itself remains subject to foreign review. Full Store after those hooks passed51.658s,
retained as .council/refactor/006-store-phase-full.jsonl.

The fresh built UI's exact Guidance/Advance held-reply journey passed13342.95ms. The real CLI
journey initially failed a fixture prerequisite: inputID can exist while materialization is still
preparing. Waiting for the actual ready binding corrected the prerequisite, not production.
The six-case CLI/Start/context/control regression then passed56687.5509ms, including CLI9052.1519ms,
retained input5932.1345ms, seven malformed receiving receipts12660.7045ms, controls8939.0662ms,
actual Start9214.8244ms and exact lost-acknowledgement retry8607.1605ms. UI explicit dependency
rebind remains a separate unfinished capability.

## Attributable Engine regression execution

The original unbatched Engine run failed its existing600s package deadline and an actual actor
case's15s watchdog; its full timeout stack was not retained. A focused repeat passed15.61s,
which does not explain or close either timing concern. No watchdog or Go timeout was increased.
The developer driver now discovers all packages and exact Engine parent Test/Example/Fuzz names,
executes other packages once and all Engine parents exactly once in escaped batches of eight.
Intrinsic case concurrency and subtests remain unchanged. The effective aggregate deadline is
per batch; batching is not evidence that the earlier timeout was harmless. Pure driver tests
passed184.522ms; independent source review found no dropped/broadened cases. The full batch queue,
complete npm gates and exact native/race CI remain required. Engine execution now adds -v only,
so subsequent retained logs contain actual per-case starts/ends and timeout attribution; discovery
and selectors are unchanged, independently reviewed by Native.

## Actual ancestor control and idle-input corrections

The initial file-ancestor fixture spent most of its observer bound on two preliminary processes;
its known-GO prerequisites failed before the intended race. The fixture now uses one actual
response ancestor, exact human acknowledgement, a context edge to the controlled file author and
a waiting file descendant. Every dependency kind participates in ancestor controls; existing
file handoff journeys remain separate. No worker barrier or observer timeout was increased.
The first changed-version assertion incorrectly required clearing LatestAttemptID; current views
retain the prior attempt as history. The corrected assertion preserves that fact while requiring
version2, no current accepted result/input and no rearmed author.

Actual pre-GO Stop remained a genuine failure: no StartedAt, verified cleanup, nil primary under
ordered Stop precedence, evidence uncertain and a settlement_unresolved incident. Native traced
cancelled receiving artifact reads being normalized by Store to storage_failed before the final
launch guard. Root added only an entry ctx.Err check before physical/SQL reads. Actual unchanged
pre-GO now passed15.82s; known-GO Stop19.30s and new ancestor version16.16s passed, with exact
StopAccepted/reason/request checks preventing a late no-op fixture. Native independently reviewed
the guard; Core independently confirmed the response/context fixture's contractual meaning.
No mid-read cancellation or universal filesystem fault qualification is inferred.

No-target completion actually fenced on corrupt unused root payload4.85s; the eligible-target
negative still detected whole-root corruption4.44s. A metadata-only scan now returns before
physical root access when there is no exact authorized waiting receiver. Every admitting path
keeps full root/selection/fresh-proof and transaction guards. The unchanged no-target case
passed5.22s and eligible-root negative4.59s. Related actual live Stop/sibling13.07s, cross-group
claim release9.32s and fresh response acknowledgement11.35s passed. Native closed the source/test
review. These are eight passes in a111.619s nine-case queue: the actor's15s watchdog FAILED again
at16.71s total with B preparing its review. Polling15ms→50ms preserved the same watchdog and
assertions, but did not resolve the timing failure. Duplicate physical proof work is being
measured before further changes; the queue is not recorded as an overall pass.

UI explicit-rebind testing first failed a fixture wait28.639s: an older Advance acknowledged
status was mistaken for the next request. Joining the exact HTTP response corrected only the
test. The unchanged built UI then genuinely failed38.514s on the absent graph-rebind-nodes
control after actual accepted predecessor version2 and the ordinary Advance no-op. Client
production rebind implementation was then authored within the frozen006 contract.

## Fresh root verification efficiency

The actual counting evidence reader failed3.59s because one VerifyGraphRoot opened its already
verified manifest/index/staged basis twice. Separate actual working/staged corruption and private
source identity guards already passed. Native factored graphRootBasis to return only the current
call's validated inventories; graphRootSource preserves its old full validation API and
VerifyGraphRoot retains its complete working-member proof and inventory. No cache crosses calls,
writes, Prepare/GO, acceptance or delivery. Root independently read the refactor/tests. The
root/materialization/native-composition family passed80.635s, including renewed reads on a second
call, both payload corruption counterexamples, original-input reset refusal and physical identity.
The unchanged actor still failed16.62s afterward, so this is not evidence of a measured whole-flow
speedup.

## Attributed finite actor progression

CPU/block diagnostic profiles were retained privately; Windows pipe/wait/syscall samples overlap
and cannot be read as elapsed stage durations. A separately ignored/build-tagged copy kept the
exact actor declaration/setup and entire original final assertion tail. Seven fixed real operation/
native/acceptance milestones completed afterCreate at2.2496,2.7611,6.5313,9.6112,11.4511,14.7884,
17.8578s. The largest transition was3.7703s. Every four-native/physical/index/provenance/acceptance/
shared-budget assertion passed; total diagnostic case19.44s, Engine19.525s. This demonstrates
steady finite work beyond the fixture-owned15s aggregate and does not explain the old600s timeout.

Native independently compared source slices and actual monotonic evidence before supporting the
specific observer correction. The normal actor now observes exactly seven declared transitions,
each within8s including missing-admission and post-review acceptance gaps, with an immutable56s
ceiling. Polls, revisions, phases, repeated observations and replacement occurrences never renew
that allowance. Bounded asynchronous readonly probes let failure report last snapshots and
runtime stacks without further SQL. Product budgets/deadlines, fixed fake worker bounds and
Go600s remain unchanged. Pure negatives cover missing admission, late completion, duplicate/poll
noise, total ceiling, replacement/skipped/eighth stage. Native closed the tracked helper/loop
source review with unchanged original final assertions. Normal actor passed19.45s (completion
17.6668s afterCreate, maximum3.566s transition), pure cases passed, Engine19.547s. Full regression
batches remain a separate unperformed gate; this focused pass does not retroactively qualify the
old timeout.

## Actual rebind client and command/correction qualification

The fresh rebind UI first exposed a live-read/held-POST race58.989s: committed version2 removed the
captured old choice. Pending historical choices now remain local editable intent until exact
reconciliation and never enter a new version's authority. A one-field null-registration response
then genuinely failed35.113s; nonempty admitted work now requires its live registration. The
initial final assertion incorrectly expected a workflow at remaining0: version2 and its input are
real, but workflow capture commits only with later authorized native admission. The corrected
assertion requires currentV2/workflow:null/nodeAdmission:null, not an invented workflow or grant.
Fresh build3.353s and the complete actual journey passed31.043s (case30.465s), including malformed
receipt retention, lost-network exact retry, third saved receipt, unchanged newer selection,
originals/options/history/index, zero additional native admissions and no refill. Root independently
read the exact pending/current, registration and acknowledgement joins.

Actual mutating command passed28.92s: fixed own-workspace file creation yields a new selected
candidate/closure/final association, preserves old input receipt/candidate/root and user staging,
spends no model unit and cannot qualify its after-state using the old check. The actual correction
first failed39.02s on a TEST schema error (original versus actual originalInstructions); its native
worker had already executed successfully. The corrected stronger case passed36.19s, requiring
whole persisted original equality, genuine failed-check finding/full receipt/raw stdout origin,
exact selected Handoff/final context, actual INVALID→VALID output, unchanged inherited/source/
index/history/root and exactly one existing shared unit. Required post-correction verification
was not executed or claimed successful. Root independently read tests, fixed path-only handler
and nil/empty-default test hooks; no runtime relaxation was made.

Both ignored005/006 render scripts passed node --check; complete paired image capture/review
remained pending at that checkpoint. Extracted-package qualification initially stopped4.94s on a stale README heading assertion
after docs changed to task graph preview. Only that fixture expectation was corrected; actual
extracted runtime qualification then passed74.132s (case73.891s), including actual workflow,
independent overlap, dependent final application, full graph input/binding privacy, exact replay
and retained reads after restart. The allowlist/notices/checksums and Git-only PATH remain intact.
This local extracted pass is not a platform-wide or complete feature acceptance.

## Read cancellation classification and visual qualification attempts

Follow-through on the earlier pre-GO fix found a separate actual pure metadata-read problem:
Store.Artifact normalized QueryRowContext/Scan cancellation during a real sole-connection pool
wait to storage_failed. Initially live cancellation/deadline cases failed2.360s, while malformed
record/closed-database controls already passed. Only actual errors.Is context sentinels are now
returned unchanged; no ctx.Err override, corruption suppression or write/ack change was introduced.
The exact admitted graph, one debit/claim, gate, ledger, artifact, incidents and SQL total_changes
remain unchanged. Targeted coverage passed2.366s, full Store43.643s, affected actual Engine46.841s
(Actor19.60s, pre-GO Stop15.35s, live Stop/sibling11.81s and pure watchdog cases). Native independently
closed source/tests. Fresh current-source build passed5.655s before the next visual attempt.

The ignored tagged visual coordinator creates actual first-acknowledged-effect Stop/partial and
post-binding retained-member-corruption/uncertain states, then serves the exact retained Store
through ordinary authenticated loopback HTTP/embedded UI. A protected descriptor is never printed
with its token; a fixed private release sentinel closes the already-settled coordinator. Client
independently reviewed Native's harness. First/second/third coordinator runs passed72.842s,
117.843s and261.419s, including different amounts of post-readiness coordinator wait; those are
not native worker timings or full visual acceptance.

Capture attempts retained16/22/34 real PNGs before independent QA-script mistakes stopped each
run: a dependency details panel needed reopening after rerender; presentation-text word boundaries
could not reliably recover a selected graph ID; and the actual review literal is005-review-v1,
not005-v1. Fixes affect only ignored tooling, preserve failed manifests/logs/images by copy and
use actual typed metadata/known selected identities. Both review checks now require nonempty
actual records. No complete image suite or independent all-image review is claimed yet.

## Responsive inspector correction and preserved visual attempts

IR006-49 is closed only in its focused responsive scope. The fourth complete-suite attempt
reached the real composed-input inspector but failed its unchanged 360px overflow check before
the mobile image could be retained. A separate actual four-stage layout case reproduced the
product failure: document content 369px versus viewport 360px, with long attributed-source action
buttons measured beyond the viewport. The RED queue took 26.300s (case25.696s), retained in
006-mobile-layout-red.log and the 006-layout-regression/1c68a5a63d6cc6ae3da9d134bc6157e9
image/measurement directory.

The correction in ui/src/style.css adds only three scoped rules: permit the inspector to
shrink and wrap, bound/wrap its buttons, and wrap the raw-artifact surface. Complete labels,
DOM text and raw bytes remain intact; no overflow check was hidden or removed. After the
matching 3.888s build, the unchanged actual layout case passed 28.848s (case28.047s), retained
in 006-mobile-layout-green.log and 006-layout-regression/638397fa1f380fc7bbf6ad756e556f7d.
All four measurements report content 360px with no offending boxes: complete composed input,
member provenance, exact original bytes and raw provenance. Complete contributed references,
BOM/Unicode/CRLF originals, read-only inspection accounting and the original user index remain
asserted. Root and Native independently inspected source/images. This records their bounded
closure and the actual focused measurements, not independent approval of every captured scene.

All six complete-suite attempts below are real local browser/owner runs with fixed offline
workers. Every archived capture-evidence.json says complete:false. The PNG counts are the
retained manifest entries, not a count of approved scenes or a complete paired set. Each archive
under .council/refactor/screenshots/ preserves the failed manifest, log and existing PNGs; later
attempts also retain the ignored coordinator source used for that attempt.

| Attempt and preserved directory | Retained PNGs | Actual stopping condition and bounded disposition |
| --- | --- | --- |
| 1 — 006-first-partial-editor-failure | 16 | Remove dependency could not be clicked after an editor rerender closed its details panel. The ignored capture driver now reopens the real details element; no product correction or completed suite is inferred. |
| 2 — 006-second-partial-identity-failure | 22 | The driver could not recover the selected graph ID using presentation-text word boundaries. It now retains the exact known selected identity/typed metadata. This was an ignored-tool identity error. |
| 3 — 006-third-partial-review-literal-failure | 34 | The driver expected reviewContract `005-v1` instead of actual `005-review-v1`. The ignored assertion now requires nonempty actual reviews and the exact current literal. |
| 4 — 006-fourth-partial-mobile-overflow | 37 | The complete-composed-input-and-context scene overflowed 360px. IR006-49's actual RED and focused responsive correction are recorded above; the complete paired suite still did not pass. |
| 5 — 006-fifth-partial-status-case | 44 | The driver waited for lowercase applied after opening the final task; the actual text is Completed · Applied verified local change. The ignored driver now joins exact task/workflow identities, that exact rendered status and the actual applied local_application journal. No product source or success predicate was weakened. |
| 6 — 006-sixth-partial-budget-state | 50 | The run reached the shared-budget scene after retaining conflicted-input-before-native-preparation. The unchanged owner predicate did not observe consumed 1/remaining 0, node blocked and zero claims together. The later IR006-50 section records the independently reviewed projection cause/correction and bounded actual passes. This failed capture remains incomplete; no full visual pass is inferred. |

A successful fault-state coordinator prepares settled partial/uncertain delivery evidence and
may then wait for the browser. Its terminal duration is not a native worker benchmark or a
successful visual suite. Likewise, a retained PNG records the actual rendered state at capture;
it does not itself prove execution, physical safety, overlap, receipt races or provider quality.

No task checkbox was changed by these partial attempts. At this checkpoint
T038/T046/T049/T052 remained open for the
remaining complete client/context/control/final scenes and paired 1440px/360px keyboard, focus,
reduced-motion, overflow and approved-hash review. Comprehensive final source review, one
matching build/test/check with browser assets, a checked source revision, six native platform
jobs plus Linux AMD64 race and the committed acceptance record remain open. The extracted
native package's earlier local pass does not close these gates. Feature 006 is under qualification;
007 implementation and acceptance are not authorized by this record.


## IR006-50: graph projection of a refused workflow continuation

The sixth visual run stopped at the unchanged shared-budget predicate after50 PNGs. A separate
actual-owner diagnostic with one required-review node and one admission reproduced the mismatch:
the workflow was blocked/budget_exhausted/user, while the graph remained queued and the node
ready/review/council with consumed1, remaining0 and no active claim. Both observations are retained
under ignored006-budget-diagnostic evidence; no screenshot assertion was relaxed.

The actual-author Engine regression first exposed a test fixture's default four-unit allowance,
then a pointer-field compile error. Those failures are retained as setup evidence. With a genuine
one-unit declaration captured before graph creation, it failed at the intended graph projection
assertion in7.803s. All three Store continuation reasons (exhausted, expired and unavailable
routing) failed the same projection assertion in0.450s. The Store expiry case explicitly adjusts
the DAO deadline and is not a wall-clock provider or Engine expiry claim.

The correction only checks successful saveTask and invokes projectGraphWorkflow within the
existing passive transaction after the failed admission rolled back. It changes no admission,
claim, allowance, error classification, predecessor proof or freshness guard. Native independently
reviewed the production call, the test-only optional pre-creation configuration helper and the
complete regression source. The affected actual Engine family passed36.043s; the Store family
passed1.664s, including original standalone refusals, capacity wait, Stop, guidance/version,
recovered owner and a real reserved independent sibling. A matching npm run build succeeded and
the unchanged actual-owner diagnostic then matched blocked/budget_exhausted/user with1 consumed,
0 remaining and0 claims. Replayed callbacks and a grant alone create no new work.

The subsequent full Store queue passed 46.209s (006-budget-projection-store-full.log). Final
matching-source global/browser/native qualification remains separately required. This bounded
closure does not complete the six-attempt visual suite or feature006 acceptance.

## IR006-51: current/history labels for an open retained input

The requirements-to-code audit found an undated false-current claim across real same-graph
updates. The inspector retained the exact selected bytes as intended, but still said
Current retained input, Current composition and current in its selector after guidance made
that input historical. The first actual browser case failed 7.104s only after proving the
owner's current:false record, visible current task version 2, complete BOM/Unicode/CRLF original,
retained selection, unchanged shared allowance, zero native admissions, original user index and
exactly one explicit guidance write. No dated historical-snapshot behavior was rejected.

The second case changes a completed source ancestor while keeping the receiver's task version,
InputID, nodeAdmissionID and latestAttemptID unchanged. Two real offline candidate nodes first
completed their required author/review workflows under four shared units, with separate
review-only acceptance and no delivery operation. Ancestor-only UI guidance then produced the
exact receiver graph_input_stale blocker and marked the actual old input current:false without
Advance. This proves why comparing only the selected receiver's version or InputID is insufficient.

The descendant fixture first failed 1.834s because an explicit review_only delivery declaration
requires a final node; omitting delivery preserves the intended default separate outputs. A
second run failed 23.170s only because settled attempt reads derive observedAt and remainingSeconds
at observation time. The test now normalizes exactly those two top-level fields; full page/cursor,
every persisted attempt identity/revision/control/terminal/evidence field and all nested metadata
remain compared. Native independently reviewed that exact normalizer. Neither setup failure is
a product RED. With real prerequisites established, the descendant case failed its false-current
oracle in 23.845s, retaining the same full-byte/selection/budget/history/user-index/one-guidance-write
invariants.

The correction is confined to ui/src/graph-inputs.ts. One observation predicate joins graph/root
identity, exact input/version/node/task identity and the latest graph_input_stale blocker. Every
selector, Scope fact and feedback paint uses that same predicate. A same-graph observation updates
only those labels, preserving full source/access buttons, focus, exact selected bytes and receiving
execution receipts. It neither reselects evidence nor sends a read-triggered mutation, native
admission, grant or Advance. Native independently closed the source/test scope; Foundation also
read the correction's exact observation/presentation joins without an additional P1/P2 finding.

After the matching 4.134s build, the four-case client queue completed successfully in 65.742s:
own-input freshness 6.335s, unchanged descendant freshness 23.210s, graph-owned pending reads
5.354s and exact selected rebind 29.034s. There were four passes and zero failures, cancellations
or skips; evidence is retained in 006-input-freshness-green.log. Each freshness case checks the
Scope fact, feedback and selected option, while allowing an explicitly dated read snapshot.
Earlier RED/setup logs remain retained under the corresponding input-freshness names.

This is bounded current/history closure, not a complete client or visual acceptance. The seventh
actual visual coordinator/capture was being pursued separately at this checkpoint; its later
incomplete outcome is recorded under IR006-52. At that checkpoint T038/T046/T049/T052 and
comprehensive final source review, matching global build/test/check/assets, checked source
identity, all six native platform jobs plus Linux AMD64 race and committed acceptance remained
open. The tenth post-copy visual section below records the temporary T052 closure, subsequently
reopened after the later production correction, without
closing those remaining gates. No task checkbox was changed and no 007 implementation started.

## IR006-52: clarify response review before required file delivery

The seventh actual paired capture stopped incomplete after 66 retained PNGs
(006-seventh-partial-response-application/capture-evidence.json). It reached pure-response
completion and separate mixed review-only results, then waited incorrectly for a response-only
final with requested file application to report application_unsupported before acknowledgement.
The focused unchanged-predicate diagnostic retained at
006-response-application-diagnostic/d0a16c8d48fe10c4bcc208e5cf59e87c/observation.json
instead shows response_acceptance_pending/user, graph blocked, no workflow completion/acceptance,
three native units consumed, no claims and preserved working/index bytes. This failed expectation
does not establish a product acceptance, application, API refusal or complete visual pass.

The engineering wording also contained a contradiction: US4 scenario5 and interface states/scenes
described an already accepted summary while requested file delivery remained unqualified.
Constitution I distinguishes a response, authored patch, check and integrated change, with task
acceptance tied to its stated outcome. FR-021, FR-024, FR-039 and FR-040 require conjunctive workflow evidence
and truthful integration. GRA-020 explicitly puts required delivery before final-node/group
acceptance. The accepted 005 response acknowledgement atomically records ResponseAcceptance and
the matching workflow completion; it has no separately committed partial response-acceptance
state. The existing mixed-response Store test already requires zero acknowledgement rows after
the graph final-response guard refuses required file application.

Foundation and Native independently reviewed the conflict and approved the bounded clarification.
US4.5, the two interface rows and the quickstart now distinguish two outcomes: review-only can
accept the exact summary and complete separate qualified results; requested file application
retains the reviewed response/file evidence but refuses final acknowledgement while the required
cumulative qualification/delivery is absent. The final node and group stay unaccepted. Complete
response/candidate evidence and the precise remaining action must remain available. Rechecking
files alone does not automatically resume an incompatible response-only final; any different
subject or scope follows the existing explicit boundaries.

No production guard, schema, acknowledgement atomicity or success state changed. A retained or
reviewed response is not renamed accepted. domain.Status maps application_unsupported to HTTP 422;
the earlier suggested 409 was an incorrect source inference, not an observed API result. The
corrected focused actual client journey then passed, retained in 006-response-ack-refusal.log and
006-response-ack-refusal/60c089ad68aab561a881a81b9c0b8fce/observation.json with complete:true.
It read the whole exact response, submitted its actual captured acknowledgement, observed
HTTP 422/application_unsupported and displayed the exact error/request identity. Complete before/
after public graph, task, workflow, attempt, event, artifact, step, delivery and root-ledger
snapshots matched; only real attempts' observedAt/remainingSeconds read-time fields were
normalized, preserving immutable event/failure/evidence timestamps. The accepted file result,
three consumed units, zero claims and original working/index bytes were preserved. Native
independently closed the helper. This qualifies the actual public refusal/immutability boundary;
it does not infer a raw database acknowledgement-row count from public events. The earlier
Store test's zero-row assertion remains separate evidence.

This is an explicit cross-artifact design correction, not a waived safety scenario or feature
acceptance. The positive pure-response and review-only accepted-summary branches remain required.
No task checkbox changes; complete visual review, final source/global/revision/assets and all
six native platform jobs plus Linux AMD64 race remain open. The eighth actual capture was
separate pending qualification at that checkpoint; its later completion/copy finding and
post-copy qualification boundary are recorded under IR006-53. The focused refusal pass does not
turn any archived incomplete capture into a complete visual result.

## IR006-53: truthful pending graph-member copy after complete visual review

The eighth actual capture completed all 88 PNGs, 72 keyboard observations, ten asset checks,
76 code hashes and twelve local states; every tail case completed. Native and Foundation
independently reviewed 44 exact images each. Foundation viewed indices44–87 at original
resolution and verified every PNG SHA-256 before and after viewing. Its bounded findings and
exact inventory remain in the ignored 006-visual-review-last44.md note. Pixels do not substitute
for the actual lifecycle, keyboard, reduced-motion, filesystem or accounting assertions.

The sole blocking finding from the combined image review was pending-member entries 32/33:
the correct task-group callout coexisted with secondary Standalone/Legacy/explicit-adoption
instructions. The whole capture/source evidence is archived unchanged at
006-eighth-complete-before-pending-copy-fix. The actual read-only graph-member journey then
failed 11.372s, reporting all three misleading messages before any copy correction.

Root changed only graph-aware no-workflow/no-route/helper copy in the workflow/routing surfaces
and truthful shared-budget observation. Known graph roots/nodes now identify group ownership,
including unavailable budget observation with its known root; absence is not labelled legacy,
unadopted or measured zero. Existing standalone/legacy branches and every eligibility, mutation,
adoption, grant and execution guard remain unchanged. Native independently reviewed source/test.

The matching build took 3.984s command wall time. The same actual reads/Stop/index journey passed:
case 5.2876607s, TAP total 5.9352455s (006-pending-membership-copy-green.log). The earlier 6.577s
report was that same tool call's 6.5767653s wall time, not a different run or test duration.
Whole originals, explicit current graph ownership, suppressed standalone controls, zero inspection
writes/native admissions, exact Stop receipt and preserved user working/index bytes remain
asserted. This closes the bounded source/client copy finding. Fresh matching post-copy paired
images and complete global/native qualification remain required; no task checkbox changes.

## IR006-54: current-schema synthetic historical CLI fixture

The first final npm-test run encountered an obsolete schema5 prerequisite in the explicitly
synthetic CLI downgrade fixture: it starts with current APIs, which now correctly produce schema6.
This fixture is distinct from the literal frozen schema5 Store fixtures. The test now checks
current6, the seven known graph tables are empty and no capacity claim exists before removing
only those known empty additions within its existing owned FK-enabled transaction. It restores
the exact historical inventory and frozen claim DDL, preserving the existing row/receipt/artifact/
attempt byte comparisons and refusal to discard graph authority or a held claim.

Native independently reviewed the exact test-only teardown and Go source's FK-enabled transaction.
Actual foundation1 and execution2 upgrades through the real CLI then passed 10.027s
(006-cli-current-schema-fixture-green-first.log). Production migration and literal schema5
fixtures remain unchanged; the upgraded fixture must create no new graph/claim/attempt authority.
This focused pass does not relabel the first global run as successful.

## IR006-55: actual CLI shared pool and captured-context serialization

The first global run also failed the obsolete assertion that two independent CLI starts cannot
have two native launch intents. The fresh fixture has two compatible contexts, no sole legacy
barrier and no shared account; default managed capacity is two. Its 5.31s failure is an obsolete
test assumption, not evidence of a violated one-slot restriction.

Native replaced only that test with actual two-worker overlap plus a third captured Codex task.
The third waits while the global pool is full and remains queued when only the other context is
released; it starts only after verified release of its own captured context. Exact bootstrap,
shared owner, targets, receipts, Stop/cleanup, release ordering and global event order remain
asserted. The existing 75s/60s bounds and 25ms polling are unchanged. Core independently reviewed
the complete test. It passed 11.85s within CLI 11.937s
(006-cli-shared-pool-green-first.log), without a production capacity/control/deadline change.

## First global outcome and attributable fresh test execution

006-final-npm-test-first.log is terminal FAILED on those two CLI fixture assertions. Other
reported Go packages include Store61.105s and Workspace443.850s passes; Engine, browser and
Node-script stages were not reached. Go script-package results in that first stage do not imply
that later Node-script qualification ran. Focused CLI corrections cannot replace a complete
fresh global pass.

Root recorded an unattributed parent-Go CPU/postprocessing delay after the child exited; no
cancellation occurred in that run. The unrelated-package test driver now adds -count=1 and -v.
Native and Core independently reviewed the exact change and locally pinned Go behavior:
-count=1 disables test-result caching and its test-log/post-hash work; -v exposes attributable
case output. Build caching, discovered package scope, -p1, race selection, Engine batching and
the 600s package bound remain unchanged. This is fresh execution/observability work, not a proven
cause, speedup or exoneration of the earlier delay or 600s timeout.

At that checkpoint the post-copy 005 visual regression and ninth 006 paired capture were
still being qualified. Their subsequent actual outcomes are recorded below; the first failed
global run remains failed.

## Tenth post-copy visual evidence and retained ninth uncertainty

The fully instrumented tenth 006 capture passed at session 36751 exit 0
(006-render-qa-tenth.log). At that checkpoint its manifest at
`.council/refactor/screenshots/006-task-graphs/capture-evidence.json` reported complete:true,
88 PNGs, 72 keyboard observations, ten approved ordinary assets, 76 code identities and twelve
actual local scenes. Its recorded SHA-256 is
`591164d06c78bee5e1992664ac87ef0ffc02c39aab0391b60d8cfad27a66d46f`.
Root's 006-capture-integrity-tenth.json independently records verification of all 88 PNGs,
76 code identities, ten ordinary asset chains, twenty fault-asset records and twenty current
asset files. No product, deadline, authority or state-assertion relaxation was made to obtain
this passing capture.

Native's new 006-visual-independent-review-tenth-first44.md and Foundation's new
006-visual-review-tenth-last44.md cover all 88 exact tenth-capture images, 44 per reviewer.
Each PNG was viewed at original detail and checked against its manifest SHA-256 before and
after inspection; the manifest remained unchanged. No concrete P1/P2 visual defect was found.
IR006-53's copy correction is now visible in current entries 32/33: the pending task is a
task-group member, workflow/routing help directs it to the current group, and the shared owner
and zero child admissions remain distinct from standalone adoption. The earlier eighth full
capture remains archived under 006-eighth-complete-before-pending-copy-fix; its findings are
not overwritten or reused as approval of the tenth-capture images.

The separate post-copy 005 capture also has a completed independent review of all 44 current
PNGs in 005-post-copy-visual-independent-review.md. That note retains its own exact file/hash
inventory, approved-asset checks and presentation limits; it is regression evidence for the
current shared copy changes, not acceptance of 006 or six native platforms.

The ninth capture remains archived under
`.council/refactor/screenshots/006-ninth-partial-initial-receiver-wait/`, complete:false after
80 PNGs. Its initial two-node 30-second observation failed, and the graph state at that failed
observation was not retained before cleanup. There is no identified root cause and no proven fix.
Four unchanged non-UI probes, four unchanged exact-browser probes and the complete same-bound
tenth capture subsequently passed. Diagnostics now retain pre-cleanup observations so a
future failure can supply the missing state. This diagnostic improvement does not recover
the ninth state or establish what caused its failed observation.

At the tenth-capture checkpoint, both independent reviewers found no demonstrated runtime
invariant violation in their bounded source/evidence/image review. That is not proof of the absence of intermittence,
nor a claim that the ninth failure was harmless, fixed or explained. Preserve the historical
uncertainty beside the later successful observations. Pixels do not independently prove worker
overlap, cleanup, filesystem safety, authority, accounting, replay or live-provider quality.
Native also records that the image tool resized one very tall original despite the original
detail request; stable hashed PNGs remain available. Crops, shortened native selected-option
text and wide checkbox focus rows retain the limits described by the two exact review notes.

T052's capture/image requirement was complete for that captured runtime. Complete current-source global
regression, final source review, checked revision and matching committed browser assets, seven
native/race jobs and feature acceptance remain open. Fresh npm test session 17313 was still
running at that checkpoint; its subsequent failed outcome and focused observations are retained
below. This docs-only update changes no task checkbox and starts no next feature.

## Resumed qualification: UI failures and bounded timing observations

The fresh global attempt in 006-final-npm-test-second.log terminated FAILED in Engine batch
7/22, with the positive actual command-check case failing its descriptor/preparation assertion
and the descriptor-tamper case failing during the cumulative-final prerequisite. The batch
reported 193.900s. An unchanged isolated eight-case repeat passed 184.508s
(006-engine-batch7-diagnostic-repeat.log). Those later passes do not identify a cause, fix or
exoneration for the original batch failures; complete current-source global qualification
remains open.

The separately coordinated UI-only suite then terminated with 67 of 69 tests passing and two
failures, zero cancellations/skips, in 514765.641ms (006-current-ui-suite.log). These are an
observed queued-Claude prerequisite failure before guidance, and a later graph-guidance/Advance
observation failure. This is a failed suite, not a complete client pass.

IR006-57 qualifies the first as an obsolete fixture assumption: the slow first task selected
Codex, so the new two-slot pool could also run the distinct Claude context instead of leaving
the second task queued. Core changed only ui/tests/claude-control.test.mjs to select Claude for
the first slow blocker and require both its returned connectionId and resolvedTarget.contextId
to equal claude-native. Foundation independently reviewed the one-hunk tracked diff and frozen
hash effa5a2881739fd03bffcab85f04bfd02d8e657a1fceadaef36e283e769a584c. Every line from
the first running observation onward remains unchanged: queued historical Claude, explicit
Codex guidance/version 2, retained Claude history, Stop, historical-version blocker and later
explicit version-2 Codex execution. Existing 90s test limits, waits and assertions remain.
The complete four-case file passed 28247.0162ms (006-claude-control-after-pool-fix.log), including
the corrected case in 9720.6093ms. This is a test-only correction, with no runtime pool change.

IR006-58 began with the graph-guidance failure. The UI suite exhausted its unchanged
30-second owner observation after the explicit Advance, while waiting for both the original
and derived-version attempts to be settled. Source inspection identified a possible omission
in blocked-initial-Advance handling. Root then ran the separate actual owned-worker
graph_blocked_initial_advance_test.go counterexample: current V2 ready input and persisted
budget block, followed by a grant and fresh explicit Advance, returned zero initial dispatch.
The positive case failed genuinely in 9.57s; its stale-version negative passed 11.72s, total
21.361s (006-blocked-initial-advance-red.log). Native authored Root's approved narrow
graph_advance.go fact-capture correction for ready-or-blocked initial nodes, preserving the
current input/proof/Store guards. Core independently closed the exact source/test correction.
The positive and stale-version-negative actual cases then passed 17.933s (10.83s/7.03s;
006-blocked-initial-advance-green.log), and a current matching build succeeded. The formerly
failing actual UI journey passed 13792.6957ms case/14381.3709ms total
(006-graph-resume-after-blocked-fix.log). Production changed only the explicit initial-admission
fact branch; Store scope/current input/proof/Stop gates and no-auto-rearm remain unchanged.
This closes the witnessed blocked-initial-Advance Engine/UI scope. The original UI suite remains
FAILED 67/69, and these focused passes do not replace complete current-source regression.

Core then strengthened only the pre-grant observations in the same actual UI case. Before
grant, the exact A/task/V2 must have a current input, no node admission, blocked state, zero
claims and a budget_exhausted blocker matching that node and input. The actual input route
must then return the same ready/current input with the exact V2 receiver node/task binding.
Foundation and Native independently reviewed frozen SHA
915e9af3ccd136eb95a5ced4176f91723f3198126a2033bbc62df1cb332303b0. Existing 30-second graph
and completion observations and the 90-second enclosing test remain; the ready-input poll is
also bounded at 30 seconds. All later exact receipt/proof, no-auto-resume, history, accounting,
window, sibling, text and user-index assertions remain. Core supplied the two literal edit
inputs; there is no independent full pre-edit file snapshot. No state is invented or written
by these prerequisite reads. The strengthened actual case passed 14145.48ms case/14701.1649ms
total (006-graph-resume-deterministic-block.log), without closing feature-wide gates.

The production graph_advance.go correction above postdates the tenth visual capture. That
capture and both independent image reviews remain valid historical evidence, but they are
not a full capture of the current binary. UI and source assets did not change. Root reopened
T052 pending a fresh capture and independent review matching the corrected runtime; do not
infer visual qualification of the current runtime from the earlier unchanged pixels.
The second full UI-only suite is still running as session 52452 with no failure reported so
far. No terminal result, global/native qualification or feature acceptance is claimed.

IR006-59 records test-only timing instrumentation in graph_final_context_test.go and
graph_final_verification_test.go. Native added fixed in-memory boundary marks outside the
polling loop, reused existing success/failure attempt reads and preserved the single underlying
Prepare call per wrapper invocation and its exact return. Foundation independently inspected
both frozen files and the retrospective 11-operation literal-edit ledger. There are no new
polling-loop database reads, locks, sleeps or retries. Existing 25s fixture observer, 10s captured
command timeout, 15ms polling and 5s failure diagnostic remain; exact context/source/check/
applicability/budget/user-index assertions are preserved. No independent pre-edit file snapshot
exists, so this is not a before-file hash/equality attestation.

The instrumented two-case command/tamper run passed 64.645s
(006-final-command-timed-first.log). The positive case took 31.20s including its fixture; its
e.run call reached the Prepare wrapper after 3.407485s and returned after 5.0432736s, with a
captured 10s deadline and about 9.993s remaining at entry. The one Prepare invocation returned
without error; the actual attempt settled succeeded with complete evidence and unchanged
functional assertions. The tamper case passed 33.36s. These successful observations do not
reproduce or explain the earlier batch-7 failures and do not justify changing a deadline.

The subsequent instrumented eight-case batch did not pass
(006-engine-batch7-timed-current.log): seven cases passed and one failed, total 162.201s.
Its positive command case passed 28.56s, with e.run taking 6.6211382s and entering Prepare
after 4.832534s, a distinct successful sample within the same captured 10s deadline. The
unknown-copy-ack case instead failed in its shared cumulative-final prerequisite after 27.21s.
The intentional lost-context-ack sink is installed only after that fixture returns, so this
failure is not evidence from the intended injected-ack path. At the unchanged 25s observer,
the retained snapshot shows A/B completed and accepted, final waiting with no input, an open
gate, no claims and no pending attempts. It does not establish whether later continuation
would have succeeded, permanent lost continuation or another cause. In the original global
failure, the positive command had observed zero Prepare calls and zero reserved descriptors;
the separate tamper case also failed in its common prerequisite. Those observations remain
failed and unresolved rather than being replaced by later successful samples.

The ignored tagged measurement-only diagnostic then passed
(006-final-fixture-milestone-diagnostic.log). All seven exact prerequisite milestones reached
acknowledged idle in 14.6895351s after Create returned, with original25Crossed=false. The whole
case took 16.18s and the Engine command reported 16.259s; these are different timing scopes.
The milestone elapsed values were 1.9493757s root ready, 2.3629865s both independent inputs,
6.4593366s both actual authors, 9.6219459s both actual reviews/acceptances, 11.2731999s final
input, 14.4208651s final author/basis, and 14.6895351s continuation acknowledgement/idle.
The largest observed milestone gap was 4.0963501s. Five distinct actual offline native
prerequisites, zero claims/pending work, consumed five/remaining zero and preserved user
working/index bytes were required by the diagnostic.

This measurement stops before granting or admitting the final reviewer/local check. Its
separate layered observer with one read probe at a time and 50ms between probes is not the
tracked fixture's 15ms observation loop. The original 25s boundary was recorded and never
crossed in this run. The result therefore supplies no evidence of required progress beyond
25s and does not justify extending the tracked observer. It neither reproduces nor explains
the earlier global or instrumented failures. No production, fixture deadline or assertion was
changed by this evidence update; IR006-59 remains open.

Monotonic elapsed values describe observed fixture/wrapper/run boundaries, not exclusive
process runtime or a product performance guarantee. Admission/deadline differences parsed
from persisted timestamps use wall clock. Prepare duration includes wrapper/spec-copy work;
fixtureExited follows existing failure diagnostics, and failed:false at fixture exit precedes
any later caller assertions. Missing timing marks remain missing observations. The original
600s unbatched timeout, batch-7 failures and ninth-capture uncertainty remain separately
unexplained. All feature-wide global/source/revision/native/acceptance gates remain open;
this update changes no task checkbox, product source or next-feature scope.

## Third full npm-test attempt: deliberate stop during source review

Root intentionally stopped session 42625 after new source-review findings. The coordinator
exited 1 because of deliberate termination during Engine batch 3/22; this is not an observed
test assertion failure or a completed global pass. The retained terminal boundary is
006-final-npm-test-third.log. Non-Engine Go packages passed, including Store 38.192s and
Workspace 289.118s. Engine batch 1 passed 21.542s and batch 2 passed 111.482s. Batch 3 retained
several completed passes but was still running its next case when terminated, so no terminal
batch outcome is inferred. Remaining Engine and later browser/Node-script stages were not
completed by this attempt. Earlier failed runs and the new source findings retain their
separate status; these partial passes resolve neither. All final global/source/revision/native/
acceptance gates remain open, with no task checkbox, code or next-feature change in this update.

## Explicit ancestor Stop and early root-manifest integrity corrections

Two independent Engine findings now have measured TDD and foreign source review (IR006-60/61,
E33/E34). Neither is an attributed cause of the unresolved batch-7/shared-fixture timing failures,
the ninth capture's observation failure or the original 600s timeout.

The actual ancestor regression captures a candidate producer, runs its real author and required
review, accepts the selected result, and physically finishes the receiver input before accepting
the producer's Node Stop. A fresh public Advance then wrongly dispatches both the blocked,
never-admitted receiver and an independent sibling. The genuine RED took 13.83s in
006-ancestor-manifest-regressions-red.log. Core changed only Engine fact capture: never-admitted
nodes with a stopped declared ancestor contribute no preparation/admission facts. The bounded
reverse traversal covers transitive files and context edges; independent siblings remain eligible.
Admitted workflows retain their existing first-GO path, while explicit rebind still requires its
separate current-contribution/preparation/Store validation. Native independently reviewed the
source and actual test. The same case passed 13.63s in 006-ancestor-green-manifest-red.log.
This regression directly exercises one files edge and one independent sibling; source traversal
review does not invent runtime qualification of every transitive, context or rebind combination.

The second finding is an earlier integrity boundary in continueGraphInputsFromTrigger.
graphRootSourceSnapshot loads and decodes the retained root manifest before verifyGraphRootClosure.
Its error branch previously returned silently, so actual corrupted complete manifest bytes bypassed
the following integrity fence. The first combined run's two manifest corruption cases failed only
in setup because a reused source_file-only helper refused source_manifest; those failures are not
guard RED. Native corrected only that fault fixture to mutate the exact registered manifest body
in place, retain identity/length/header and require actual ErrCorrupt before invoking continuation.
The eligible case then genuinely failed 3.77s with gate open. The no-target case passed 4.49s;
post-real-metadata-read cancellation and a plain ordinary metadata-read failure passed 3.66s/3.58s
(006-ancestor-green-manifest-red.log).

After that RED, Native added only the same uncertainEvidence(err)-gated
graph_root_evidence_uncertain fence at the early snapshot-read error branch. Core independently
reviewed source/tests. No-target completion still returns before physical reads; exact cancellation
and plain read failures remain quiet. Error classifiers, DAO behavior, fresh physical verification,
input/admission authority and original terminal records are unchanged. This is not a claim that
all SourceSnapshot DAO cancellation/error mappings are corrected.

The complete affected Engine group then passed 83.607s in
006-ancestor-manifest-affected-green.log: sibling-only explicit Advance 13.69s; actual ancestor Stop
before first GO 16.01s; Stop after known GO 17.80s; current blocked-initial resume 12.08s; stale
version refusal 8.23s; eligible manifest corruption 3.79s; no-target corruption 4.38s; and the
cancel/plain-read controls 3.72s/3.83s (their enclosing test 7.55s). Existing strict behavioral,
history, accounting, user-working/index and no-effect assertions remain. The integrity case
requires the exact durable incident/source, no usable receiving input/native debit/pending work,
unchanged evidence inventory and unchanged original terminal.

The separate fresh-post-Stop Store boundary is qualified below; it is not inferred from the
Engine run. IR006-59, complete current-source global regression, matching capture/revision,
native jobs and feature acceptance remain open. No task checkbox, product deadline, model
call or next-feature scope changes through this documentation update.

## IR006-60 Store boundary: fresh initial authority cannot override ancestor Stop

E35 records a distinct SQLite counterexample in graph_admission_ancestor_stop_test.go. Existing
revision/live-authority checks already reject stale pre-Stop facts. The new case instead uses
actual Store predecessor acceptance, dependent-input completion and accepted ancestor Stop,
then supplies the freshly current control vector while retaining the exact sealed input and
registered proofs. A valid independent C is first in the batch, followed by stopped-dependent B,
under distinct captured contexts and sufficient shared allowance.

006-store-ancestor-stop-red.log is genuine RED: Store 0.390s. The identical unstopped two-context
control passed 0.16s, eliminating invalid proof or capacity setup as the refusal reason. The
fresh post-Stop case failed 0.17s because the batch was wrongly acknowledged with admission
and changes to attempts/events, ledger/claims, graph nodes/dispatches/observations, receipts,
routing/task/budget rows and workflows/steps. This is a semantic fresh-authority defect, not a
claim that an old-vector late-Stop race escaped existing revision checks.

Foundation changed only graph_admission.go. When the current node has no initial admission,
graphAdmissionControl traverses declared file/context ancestors through the same transaction
and rejects any ancestor Stop with graph_control_conflict. The check follows exact current
input/contributor/response-head checks and precedes preparation-failure observations or native
allocations. Its error rolls back the entire explicit batch, including C's earlier reservation
and the new registration. Shared contributor validation and existing admitted-workflow/
known-first-GO behavior remain unchanged.

Core independently reviewed the complete metadata regression and narrow production guard,
with no P1/P2. Frozen source SHA-256:
`8b66e78ab53dce526bb55680e570633c09ba50b933ed072d9dd139992ed26cee`;
test SHA-256: `35fe85baead6fff25931c180393b20770732775a851f0579d29a23d1bb88fde6`.
Root's complete Store suite passed 43.733s exit 0
(006-store-ancestor-stop-full-green.log); the new control/refusal cases passed 0.12s/0.14s.
The refusal requires status 409 graph_control_conflict with no receipt, exact before/after raw
rows for every logical SQL table, then one C-only debit/claim with A/B and retained input/
provenance intact. Exact C-only replay must return the saved envelope and change no SQL row.

These fixtures exercise real SQLite transactions with registered metadata proofs, not actual
native processes, artifact-file access, Git bytes or filesystem composition. The earlier
83.607s Engine group is a separate checkpoint before this Store guard. The complete local run
for the working checkpoint followed below; fresh matching capture/review, checked revision, seven native/
race jobs and feature acceptance remain open. No attribution or fix is claimed for IR006-59
or the ninth capture.

## Fourth full local command checkpoint: passing, feature still under qualification

Root's fourth full npm-test attempt, session 56603, exited 0
(006-final-npm-test-fourth.log). All non-Engine Go packages and all 23/23 discovered Engine
batches passed. The terminal log has 23 Engine batch declarations and 23 successful Engine
package outcomes, then reaches and completes both later Node stages.

| Observed local phase | Exact terminal evidence |
| --- | --- |
| UI suite | 69/69 passed, zero failed/cancelled/skipped; duration 559353.4819ms |
| Script suite | 3/3 passed, zero failed/cancelled/skipped; duration 105339.1351ms |
| Native-preview/package case within the script suite | Passed 104617.7359ms; recorded execution PATH excludes Node/npm/Go and supplies verified Git |
| Post-fix build | Exit 0; 006-after-stop-fixes-build.log |
| Post-fix check | Exit 0; 006-after-stop-fixes-check.log |

This is current-host offline qualification. The Node durations are their recorded suite/case
values, not an inferred total command wall time or native-worker benchmark. The earlier
67/69 UI failure and deliberately terminated third attempt keep their own recorded outcomes.
No terminal result for the earlier standalone UI session is inferred from this distinct run.

The formerly failing batch-7 positive command, unknown-copy-ack and descriptor-tamper cases
passed 29.55s/32.28s/33.75s in the fourth command. The Store initial ancestor-Stop control and
refusal cases passed 0.12s/0.13s. These results establish a successful full local command after
the independently reviewed corrections; they do not identify a cause or prove a fix for
IR006-59's earlier failures, the original 600s unbatched timeout or the ninth capture's
unexplained observation. No deadline or assertion is changed by this evidence update.

E36 is a passing working-checkpoint record, not feature acceptance. Final matching-runtime
paired capture/review, comprehensive final source/review identity, matching committed browser
assets and checked source, all six hosted native platform jobs plus Linux AMD64 race and the
committed acceptance record remain open. No task checkbox or next-feature scope changes here.

## Eleventh capture, independent integrity and remaining mobile finding

E37 records the completed eleventh capture after the later runtime corrections. Root's Node
capture session 96597 and tagged fault coordinator session 98760 terminated successfully in
006-render-qa-eleventh.log and 006-visual-fault-eleventh.log. The latter reports the actual
TestGraphVisualQADestinationFaultStates run passing in 89.825s. The pre-wrap manifest is now
archived at .council/refactor/screenshots/006-eleventh-before-mobile-wrap/capture-evidence.json, captured at
2026-11-07T14:00:48.780Z, SHA-256
40ad3f28bf53d7bafa1bffebb56ee710daa510d6514ba95acad95fda9608734c.

Foundation's independent 006-capture-integrity-eleventh.json has SHA-256
7bcf2a98bf730c4055b68c77e1b4d583c52d09c3a7671106e02c431d7d800093. It verifies all 88 PNGs
(44 desktop/mobile pairs), 72 recorded keyboard checks (43 desktop/29 mobile), ten approved
ordinary asset chains plus twenty fault-owner asset records, 76 UI code identities and twelve
actual local scenes/reduced-motion records. The asset comparison includes twenty current
source/dist asset files and the ten-entry approved catalog. The code inventory is nineteen
authored UI sources, nineteen ordinary dist/served records and thirty-eight fault dist/served
records: 38 unique local code files and 57 retained served-code comparisons. All 147 unique
local files and the manifest were stable across the independent before/after hashes. Integrity
failures and recorded layout/console/network errors are zero. Recorded keyboard, scene and
served-body observations were checked structurally and against files, without a new browser
run, HTTP fetch or lifecycle execution by this reviewer.

Native's 006-visual-independent-review-eleventh-first44.md individually covers indices 0–43;
Core's 006-visual-independent-review-eleventh-last44.md covers 44–87. Both requested original
detail and verified every assigned PNG plus the manifest before/after viewing. Neither pixel
review demonstrated a P1/P2 visual defect; the pending-member copy in entries 32/33 remains
corrected. The last-44 reviewer explicitly left long identifier endings in mobile crops 55
and 83 unqualified because element-crop pixels alone do not establish full-page visibility.
The historical tenth notes and ninth incomplete run keep their separate recorded outcomes;
this later complete capture does not attribute the ninth observation or IR006-59 failures.

The subsequent actual expanded-details diagnostic now demonstrates a mobile defect, recorded
in 006-mobile-identifier-expanded.log. At 360px, actual-root-ledger has thirteen tokens outside
the target box but zero visible viewport-overflow tokens. Collapsed exact-pending-rebind has
four non-painted out-of-viewport tokens. Expanded exact-pending-rebind has four painted,
visible tokens beyond the viewport, with no clipping classification hiding them. Its actual
viewport image is `screenshots/006-mobile-identifier-diagnostic/784bbfd76216b950604e70b1e43c2890/actual-exact-pending-rebind-expanded-360-viewport.png`,
SHA-256 67d3d1d5de6e25838d2eaead45c570db2b53c9c1737991ca646cc817847ff32e. This is a
demonstrated expanded-state overflow, not an inference from the earlier named-element crops.
T052 and the visual gate remain open pending correction and qualification. No task checkbox,
acceptance or source change is made by this evidence update.

The separate read-only provenance audit distinguishes both Go runtime paths. Ordinary scenes
spawn build/council.exe via ui/tests/helpers.mjs; scripts/build.mjs builds that path from
./cmd/council. Fault scenes serve the real Engine and HTTP API with webui.Files() inside the
tagged Go test process, not that CLI executable. Retained matching graph identities, terminal
test output, build/launch ordering and the recorded source freeze support current-checkpoint
freshness. The manifest's 76 code identities concern embedded UI bytes; fixed app/contract
versions are not unique builds. Neither capture manifest nor logs record ordinary startup
executable hashes, the fault test-executable hash/build ID or an exact dirty Go-source
fingerprint. A separately inspected current council.exe hash cannot identify the fault process
or retrospectively bind ordinary startup bytes. No cryptographic Go-runtime/source binding,
pixel proof of lifecycle/accounting, native-platform qualification or live-provider evidence
is claimed. Final visual, source/revision/native and feature-acceptance gates remain open.

## Focused mobile pending-details CSS correction

E38 follows the expanded-state finding above with actual browser TDD evidence. The new
ui/tests/graphs-pending-details.test.mjs uses actual owned fake workers and accepted source
output, real guidance and a retained exhausted-budget rebind. It holds the committed reply,
changes the current local choice, releases a lost acknowledgement, and opens the actual
retained request details at 360px. No runtime state or acceptance is invented. In
006-pending-details-mobile-red.log the genuine geometry assertion failed: document width was
494px at a 360px viewport, with the complete JSON and visible identifier ranges recorded.
The case took 27578.5842ms; total was 28295.1049ms, one failure and zero passes.

The correction is scoped to ui/src/style.css: pending controls and ledger text receive
min-inline-size:0/overflow-wrap:anywhere, and pending-request preformatted text receives a
bounded inline size, white-space:pre-wrap and overflow-wrap:anywhere. Complete text is
preserved; no control, request payload, authority or runtime admission rule changes. Root
confirmed independent review of the scoped source. The matching
006-mobile-wrap-build.log completed exit 0; no internal build duration is inferred.

The same actual regression passed in 006-pending-details-mobile-green.log: 1/1, zero failures,
cancellations or skips, case 31113.3395ms and total 31720.2841ms. It requires the complete exact
captured JSON and every identifier to be rendered within the actual viewport without hidden
clipping. Opening details sends no retry or new graph request; the later explicit retry keeps
the identical raw request and original receipt. Receiver history, input provenance, consumed
5/granted5/remaining0 allowance, deadline/authority, absent new admission/worker, working bytes
and staged index remain checked. Existing 90s test/default30s waits and actual polling bounds
remain; focused success is not a new full UI/global result.

This closes only the witnessed expanded-request CSS regression. The eleventh capture and its
integrity/image reviews remain valid evidence of their earlier checkpoint and predate this
CSS change. A new diagnostic and matching capture are underway; no result is inferred. T052,
visual/global qualification, checked source/revision/native and feature acceptance remain
open. No task checkbox or product/test source is edited by this documentation update.

## Post-fix acknowledgement observer correction and diagnostic scope

E39 preserves the post-fix diagnostic failure in
006-mobile-identifier-expanded-after-fix.log and its complete:false observation under
screenshots/006-mobile-identifier-diagnostic/caad03c112aeb813a502028a596670fe/. That run's
three actual geometry captures already record zero visible viewport-overflow tokens. The
failure was the observer's immediate request count, 1 !== 2, after the Retry label became
Reconciling but before the exact second reply had been acknowledged. Its retained final
observation contains two identical body/status200/envelope records. This is a witnessed
observer race, not an additional CSS or runtime replay counterexample; the failed run remains
failed and is not relabelled complete.

Both observers, in ui/tests/graphs-pending-details.test.mjs and the ignored
006-mobile-identifier-diagnostic.mjs, now wait for the exact POST path/captured request ID and
verify the original successful response before observing pending-row reconciliation. Root
confirmed independent review. A single fixed 30s deadline covers response and subsequent
reconciliation using only its remaining time; no reset, retry, product change or deadline
increase is introduced. Existing complete-text/viewport/clipping, raw request/reply equality,
history/budget/authority and user-state assertions remain. The updated focused test passed
1/1, zero failures/cancellations/skips, case 29648.33ms and total 30324.5238ms in
006-pending-details-mobile-ack-green.log.

The subsequent diagnostic completed exit 0 in 006-mobile-identifier-ack-after-fix.log. Its
complete:true observation and three PNGs are retained under
screenshots/006-mobile-identifier-diagnostic/09ecb8a8adc666f46c4bfa93a9d2715c/. At 360px,
actual-root-ledger, actual-exact-pending-rebind and actual-exact-pending-rebind-expanded each
record zero target-box, viewport and visible viewport-overflow tokens. The collapsed details
contain four non-painted tokens; expanded details contain none. Rectangular/complex clip
counts are zero. The expanded PNG SHA-256 is
e5fc7c58ea1415128d9b02294a3470c19a82331c4ffe16416da1f787555296a5. This is actual local
geometry and exact-reply/reconciliation evidence, not independent all-scene pixel review,
keyboard/lifecycle/provider qualification or a global suite.

The pre-wrap eleventh capture is archived separately and retains its original hashes/reviews.
The twelfth matching visual capture is running; no terminal outcome is inferred. T052,
visual/global/source/revision/native and feature-acceptance gates remain open. No task checkbox
or production/test source changes are made by this documentation update.

## Twelfth archived capture, integrity and independent pixel review

E40 records the complete twelfth capture in 006-render-qa-twelfth.log: 88 PNGs, 72 recorded
keyboard checks, ten approved ordinary assets, 76 UI code identities and twelve actual local
scenes. The tagged actual fault coordinator separately passed in 64.693s in
006-visual-fault-twelfth.log. The archive is
.council/refactor/screenshots/006-twelfth-before-recovery-review/capture-evidence.json,
captured at 2026-11-07T14:33:55.543Z, SHA-256
682959f26a44527142c5514c95ef2f2696b5e3786da2edd3e337428b23b604d2.

Native's 006-visual-independent-review-twelfth-first44.md covers exact indices 0–43; Foundation's
006-visual-independent-review-twelfth-last44.md covers 44–87. Each reviewer individually
requested original detail for 44 PNGs and verified every assigned PNG plus the manifest before
and after viewing. Neither pixel review demonstrates a P1/P2 visual defect. Mobile root-ledger
identifier endings visibly wrap; pending request summary and exact-retry action are readable.
The request details are collapsed in those images: expanded JSON remains the distinct E38/E39
regression/diagnostic scope, not an inferred extra pixel approval. Review-note SHA-256 values
are e6e7bd406f9f5f420337a2cb4899da4b453f68a37457873dd3a07fdfb3541303 (first44) and
f0c59f47811cbddb11795d23958a084cda4e855289e03fad5728a1215b2a25d5 (last44).

Foundation's new 006-capture-integrity-twelfth.json has SHA-256
0b8649759816b97d69bc9706d27731077938cfb38dd46928d73c5682fe23674d. The independent audit
verifies 88 unique valid PNGs/44 pairs, 72 focus records (43 desktop/29 mobile), ten approved
ordinary asset chains plus twenty fault-owner asset records and twenty source/dist asset
files, 76 UI code records (19 source/19 ordinary dist-served/38 fault dist-served), twelve
complete structural scenes and twelve reduced-motion records. Exact recursive source/dist
inventories contain nineteen files each, with 38 unique code files and 57 recorded served-code
comparisons. All 147 unique local files and the manifest remain stable across both hash
passes; integrity failures and recorded layout/console/network errors are zero.

This is file/retained-observation verification, without fresh HTTP fetch, browser action or
lifecycle/keyboard replay. Ordinary owners use build/council.exe; fault HTTP/Engine owners run
inside the tagged test executable. UI/asset hashes, fixed contract labels and temporal
build/source-freeze provenance do not cryptographically identify those Go processes or the
dirty source. The twelfth archive explicitly predates the later recovery production fix and
later keyboard-focus correction. It remains valid checkpoint evidence and does not qualify
the current binary or close T052/visual/global/source/revision/native/acceptance gates.

## Queued local graph recovery: known-zero boundary

E41 records the real Store regression TestGraphLocalRecoveryDistinguishesNeverClaimedFromPreparingEffects
in internal/store/graph_local_recovery_test.go. Both root-basis and node-input fixtures use
actual graph registration/claim/recovery transactions and controlled metadata. Queued local
operations are genuinely root-owned, cannot pass CheckGraphLocalOperation, and have neither a
claim/preparation event nor launch intent/source snapshot. The claimed controls enter actual
preparing state through ClaimAttempt and pass the local-I/O permission guard. No filesystem
composition, native worker or user-byte verification is manufactured by this Store fixture.

006-local-recovery-red.log genuinely failed Store 0.248s: the never-claimed root_basis and
node_input cases each failed 0.05s because interrupted recovery invented unknown effects and
fenced the global gate. Claimed/preparing controls passed 0.04s/0.05s, retaining uncertainty.
The source correction in internal/store/results.go extends the existing graph-node queued
known-zero rule to actual graph-root local_graph operations only when phase remains queued,
launch intent is false and source snapshot is nil. Root confirmed independent review; no
claimed/preparing effect uncertainty or unrelated executor recovery is relaxed.

Focused qualification passed Store 0.291s in 006-local-recovery-green.log, all four subcases
passing. The complete Store suite then passed 51.630s in
006-local-recovery-full-store-green.log. Assertions distinguish known-zero interruption from
potential effects; preserve exact prior claims/fences for claimed work, original owner and
terminal history, captured source/receipts/authority, native consumed0 with unopened window,
and unchanged history/no redispatch across a second owner recovery. Old-owner permission stays
refused. This is scoped transactional closure, not physical/native recovery, a new global
pass, matching visual evidence or acceptance.

## Pending-control keyboard focus: actual RED and focused closure

E42 records 006-control-focus-red.log. The actual test
ui/tests/graphs-control-focus.test.mjs reaches pending exact replay with keyboard Tab/Enter and
a held real reply. Its positive case failed 8050.6859ms because focus became BODY, outside the
actual pending request and without meaningful focus, while the response was held. This fails
before the later same-request restoration/focus-visible assertions; those are not recorded as
passing. The separate user-moves-elsewhere control passed 6835.3668ms, preserving the chosen
graph-refresh focus after another lost acknowledgement. Total is 15535.7915ms, one pass/one
failure and zero cancellations/skips.

The subsequent independently reviewed ui/src/graphs.ts correction keys pending rows by the
exact operation/request identity, retains a focusable announced busy action behind the existing
dispatch guard, and restores only focus displaced synchronously to BODY by repaint. A user
move elsewhere is not restored later. No request/authority payload or mutation rule is changed.
Root confirmed matching build/check exit 0 in 006-recovery-focus-build.log and
006-recovery-focus-check.log. The actual affected queue passed 3/3 with zero failures,
cancellations/skips in 006-focus-mobile-affected-green.log, total 45213.9466ms: positive retry
6575.3108ms, move-away control 6926.315ms, and complete expanded mobile request/exact retry
30540.3906ms. Original RED and its limited reached assertions remain preserved separately.
This closes the witnessed focused keyboard scope. The failed interaction demonstrates a
boundary static focus-ring images did not qualify; full global qualification and fresh
matching visual/source identity still remain required. T052 and all feature-wide gates remain
open; no task checkbox or product/test source
is edited by this documentation update. Earlier ninth/IR006-59 timing uncertainties retain
their independent unresolved history.

## Thirteenth current-build capture: scoped T052 evidence

E43 records the completed thirteenth capture at the post-recovery/focus build checkpoint.
Its current manifest is .council/refactor/screenshots/006-task-graphs/capture-evidence.json,
captured at 2026-11-07T14:51:45.314Z, SHA-256
2bc9db0992edd192ec3d57fdd2c8abdcc7149a6d014218c5bc303ab176ffcd6c. The retained
006-render-qa-thirteenth.log reports 88 PNGs/72 keyboard checks/ten ordinary assets/76 code
identities/twelve actual local scenes. Twenty separate fault-owner asset records are retained
alongside the ten ordinary chains; these are not thirty distinct approved assets.

Foundation's independent 006-capture-integrity-thirteenth.json, SHA-256
c610f47b923a1d0f0d4bdbb9c1c5493242a68cf3b86005fdd7b2a817fca87894, verifies all 88 valid
unique PNGs/44 pairs, 72 focus records (43 desktop/29 mobile), exact approved ordinary/fault
asset chains and twenty local asset files, 76 UI identities, twelve complete structural scenes
and twelve reduced-motion records. The code records comprise nineteen authored sources,
nineteen ordinary dist/served records and thirty-eight fault dist/served records. Recursive
inventories are exactly nineteen source/nineteen dist files: 38 unique local code files and
57 retained served-code comparisons. All 147 unique local files and the manifest match before
and after; integrity failures and recorded layout/console/network errors are zero. No new HTTP
fetch, keyboard action, scene or lifecycle execution is performed by this audit.

Native's 006-visual-independent-review-thirteenth-first44.md covers 0–43; Core's
006-visual-independent-review-thirteenth-last44.md covers 44–87. Every assigned image is
individually opened with original detail requested, with matching before/after PNG and
manifest hashes. Their review-note SHA-256 values are
9690f253c6819391f1c6fb31424b46b456fedf08eb68d03c932d9c30cfa04741 and
16d337534ef58af48950cdb81e7f49c4fdb6a133e93ce5cd77a671b7203a1885. Neither review
demonstrates a concrete P1/P2 pixel defect. Long mobile ledger/request endings visibly wrap;
current/historical context, refusal, final-only delivery and retained pending scope remain
distinct in the reviewed images.

The first-44 note explicitly limits entry 37: physical PNG 294×7174, tool display 246×6000
despite requesting original detail. Overall section structure, wrapping and labels were
inspected; exact transcription of every identifier glyph in the resized display is not
claimed. Named-element crops do not prove complete viewport/scroll/hidden-state behavior.
Closed pending JSON and static focus rings do not replace the actual expanded-details and
keyboard TDD/diagnostic evidence in E38/E39/E42. No process, accounting, byte-preservation,
model-inspection or Go runtime identity assertion is inferred from the pixels.

T052 requires the ignored paired 1440px/360px scene capture, retained scenario/source/asset
identity, keyboard/focus/reduced-motion/overflow observations and independent actual PNG
inspection. Plan's approved-UI obligation and interface302–329 separate that bounded work
from serial suites, package/native/race and feature acceptance. E43 plus the separately
qualified expanded-state/keyboard cases satisfies that capture/image requirement with the
stated limits; T052 is scoped passed in this evidence record. Task checkboxes remain Root's
task-audit lane. There is no claim of per-glyph transcription or exhaustive UI behavior.

The thirteenth manifest fingerprints UI/evidence files, not Go source/executables. Ordinary
owners launch build/council.exe; fault HTTP/Engine owners execute inside the tagged Go test.
Recorded current build/launch ordering and source freeze support freshness, without captured
process hashes or an exact dirty-Go-source binding. This does not substitute one executable's
hash for the other. The fifth full attempt was subsequently deliberately stopped as E44
records below, without a full-suite pass or assertion failure. Full current-source commands,
comprehensive source review, checked revision/browser assets, all seven native/race jobs and
acceptance remain open; historical ninth/IR006-59 uncertainties retain their separate limits.

## Fifth full command: deliberately interrupted after source findings

E44 records 006-final-npm-test-fifth.log, coordinator session 9440. Root deliberately terminated
the verified owned process tree after two independently source-confirmed findings. The session
closed exit 1 because of that termination; Root verified child PID 21952 no longer remained.
This is an incomplete run, not a test assertion failure or a passing suite. Source findings
still require their own actual regression/correction/foreign-review evidence; termination
does not establish their runtime behavior or excuse unfinished checks.

The retained log has sixteen terminal non-Engine package passes: adapters, app, CLI,
discovery, domain, evidence, fixture, HTTP API, localpath, localstate, owner, process, project,
routing, Store and workflow. CLI passed 82.897s and Store 43.962s. cmd/council and webui have
no tests. Workspace was active, ending with RUN for
TestApplicationScopeBranchChangeBeforePreparePreservesDestination and no package terminal.
No Engine-batch, browser or script stage was reached, and the log contains no assertion-failure
marker. These reached results retain their scope; absence of a marker does not pass Workspace
or any later stage. Original earlier failures, third deliberate stop and fourth pre-change
full pass remain separately recorded.

E43's current-build capture/image requirement is scoped passed at its recorded checkpoint.
It neither closes the independent source findings nor qualifies a later corrected runtime.
The complete current full suite, comprehensive final source review, checked revision/assets,
all seven hosted native/race jobs and committed acceptance remain open. T052 task-checkbox
updates remain Root's task-audit lane; no task, source or test edits are made here.

## E45: terminal graph projection after owner recovery and queued expiry

IR006-62 records the Native source finding after the fifth full command was stopped.
`RecoverOwner` and ordinary queue/expiry settlement saved an interrupted/failed attempt and
workflow but did not project that terminal observation into the public graph/input/node.
`006-recovery-projection-red.log` records all nine actual Store cases failing, package0.744s:
root-basis/node-input queued and claimed recovery, native queued-reserved/claimed recovery
with an actually accepted predecessor, and root/input/native queued expiry.

The correction adds `graph_terminal_projection.go` and only two calls in
`workflow_control.go`'s `syncWorkflowTerminal`: the no-workflow root operation path and the
existing current-step workflow save. It runs in the same transaction, joins exact current
task/latest attempt/workflow/dispatch/input/admission/control, and reuses ordinary node
workflow projection. Unfinished local operations retain their exact refs while becoming
nonqualifying interrupted/incomplete/blocked inputs; no binding or completion seal is invented.
Completed physical input results, newer versions, Stop, accepted siblings, claims, uncertainty,
budget and immutable terminal/receipt/source history remain protected.

Audit and Core independently reviewed the actual regression/helper/hooks with no concrete
P1/P2 in this scope. `006-recovery-projection-green.log` passed thirteen focused subcases in
1.024s: the new nine plus the four existing known-zero/claimed-uncertainty controls.
`006-recovery-projection-full-store.log` subsequently passed the complete Store package in
44.290s. These DAO tests use registered metadata proofs; their explicitly adjusted expiry
deadline is not evidence of elapsed native/model execution or physical cleanup. This closes
the witnessed projection defect, not the full feature or matching global/visual/native gates.

## E46: captured Graph Start acknowledgement before reconciliation

IR006-63 records seven real committed Start replies with one altered identity/scope join.
The generic receipt request ID remained valid, but the old UI removed pending intent or
painted success for a foreign envelope request/receipt operation/project/definition graph/
node membership/root-budget owner/registration node version. `006-start-ack-red.log` is
terminal,33.669s: all seven mismatches failed and the genuine original-owner receipt after
actual owner recovery passed.

Core added a pure admission response join in `ui/src/graphs.ts`, called before pending
deletion and success. It checks the captured Start and internal graph/node/root-budget/
registration associations, while preserving immutable historical owner receipts and newer
drafts. A mismatch remains uncertain and retains the exact request for explicit replay;
no server authority, request, control or current-owner comparison changes. Native independently
reviewed the actual tests and current source.

The first build, `006-ack-projection-build.log`, failedTS2532 on indexed declaration access.
An explicit captured declaration presence check corrected that type error; the second build
in `006-ack-projection-build-second.log` exited0. `006-start-ack-green.log` then passed all
eight actual browser cases in51.607s, with no failures/cancellations/skips. Exact body/saved
envelope replay, newer editor/standalone text, history, shared allowance and source/index
preservation remain asserted. Busy occupancy is an explicit fixture report, not provider
measurement, and the offline bridge uses fixed fake workers.

E47 below supplies the subsequent definition-poll RED/correction/pass. E43's thirteenth
capture precedes E45/E46/E47 source changes, so fresh matching visual,
complete global commands, checked revision/assets, native/race and feature acceptance remain
open. E44's deliberately interrupted command and historical ninth/IR006-59 uncertainty are
not relabelled by these focused passes.

## IR006-64: preserve inspected originals during graph observation

E47 records the actual browser regression in ui/tests/graphs-definition-observation.test.mjs,
corrected frozen SHA-256 f3cdf29099bbc406ba6b50ab85c45b6feedce1c0d957db3f8e70d86bfbac036d.
The initial 006-definition-observation-red.log failed only fixture creation with HTTP400
invalid_request because workflow.review:none is unsupported. Domain validation permits
empty/auto/required; changing only that fixture value to auto allowed the genuine observation
case. The initial failure is retained as setup evidence, not product RED.

006-definition-observation-red-second.log then genuinely failed: 12220.4951ms case and
12828.8009ms total. A real owned fixed graph-wait author was running with StartedAt before
the test opened the exact complete original by keyboard. Its real owner wait response was
unchanged at the exact requested state revision. The test uses the supported shorter GET
timeoutMs=1 for that observation, without changing the response body or the fixed 8s worker
bound. The next actual loop request proves the first response passed through renderDetail
before the oracle. The complete original remained byte-identical, but details.open became
false and keyboard focus moved to BODY. Later changed/Stop/cross-graph assertions were not
reached in this RED. Native independently closed the corrected fixture/RED boundary.

Core's scoped ui/src/graphs.ts correction retains the actual definition DOM only while exact
graph ID, root task, definition version and declaration SHA identity match. Absent/mismatched
definition or a new selected graph clears the key and rebuilds. Node facts and controls still
update; no focus call, cross-graph open-state cache, mutation or authority change is added.
Definition success feedback now follows the actual validated definition GET, rather than
every observation paint. Native independently reviewed the fix; Audit's additional frozen
source read found no concrete blocker. The frozen source SHA-256 is
1deeb0ba63d9efd8f2753454c3ab0bbada2a980af76a68dd2eba6747f29599c1.

The matching build in 006-definition-preservation-build.log exited 0. The same corrected test
passed 1/1 in 006-definition-observation-green.log, case 15639.1377ms and total 16245.0415ms,
with zero failures/cancellations/skips. It now reaches and checks both genuine changed:false
and changed:true paints, exact accepted public Stop and verified worker cancellation, stable
original allowance/deadline/one-attempt history, complete text and keyboard focus, zero
browser mutation requests from inspection/navigation, and unchanged user working/index bytes.
The second real graph uses the same node key/version with a different original, proving it
cannot inherit the first definition/open state. Graph definitions are immutable in 006; no
synthetic version mutation or server-state fabrication is used or qualified.

This closes only the witnessed definition-observation boundary, consistent with the interface
requirement that polling never steals focus. The combined affected UI run subsequently
passed 12/12, zero failures/cancellations/skips, total 117633.8909ms in
006-current-ui-affected.log, session 64255 exit 0. Its scope is eight exact Start acknowledgement
cases, two keyboard-focus cases, the definition-observation case and expanded mobile request
case. Current npm run check exited 0 in 006-current-post-review-check.log. These are an actual
affected subset and working-checkpoint check, not a complete matching full suite or feature
acceptance. E48 below records the subsequently completed fourteenth capture and its scoped
integrity/image qualification.
E43's thirteenth capture predates this source change and remains historical checkpoint
evidence. A complete current full suite, fresh matching visual, checked revision/assets,
all seven native/race jobs and feature acceptance remain open.

## Fourteenth current-build capture: scoped visual qualification

E48 records the completed fourteenth capture after E45/E46/E47 production corrections.
The current manifest is .council/refactor/screenshots/006-task-graphs/capture-evidence.json,
captured at 2026-11-07T15:30:54.239Z, SHA-256
37bc2fb06f2a7af3bbee6de5413ec5a0367f923f49c7aecd5f22da20d99282a1. The retained
006-render-qa-fourteenth.log records 88 PNGs, 72 keyboard checks, ten approved ordinary assets,
76 UI code identities and twelve actual local scenes. Twenty additional fault-owner asset
records cover the same ten approved assets on the two fault owners, not twenty new assets.

Foundation's 006-capture-integrity-fourteenth.json has SHA-256
35e119d1dd3f4b972586d26d9e8753b130bf8a3a1f58ede66668e90451ed25d6. Its independent file
audit verifies all 88 valid unique PNGs/44 pairs, 72 recorded focus checks (43 desktop/29
mobile), ten ordinary plus twenty fault asset chains, twenty current source/dist asset files,
76 UI records and twelve complete structural scene/reduced-motion records. Recursive code
inventories are exactly nineteen authored sources/nineteen dist files; records comprise
nineteen source, nineteen ordinary dist/served and thirty-eight fault dist/served entries,
38 unique local code files and 57 retained served comparisons. All 147 unique local files
and the manifest match before/after. Integrity failures and recorded layout/console/network
errors are zero. Served, scene and focus records are retained observations compared with
files/structure; the audit performs no fresh HTTP, keyboard or lifecycle execution.

Native's 006-visual-independent-review-fourteenth-first44.md and Core's
006-visual-independent-review-fourteenth-last44.md individually inspect 0–43 and 44–87 with
original detail requested and all exact PNG/manifest hashes matching before/after. Their
review-note SHA-256 values are 68c1bb8838233c5a65ca9f6a0fde5451cac591d4a5c6cdcdf6b2942e63be3c2a
and ab91a6f0dc06400069162b680286c6f65d9f5e0691904966aecd957effee7c1e. Neither review
demonstrates a concrete P1/P2 visual defect. Current/pending/history, response/refusal/file
delivery and shared allowance remain distinct; long mobile ledger/request endings visibly
wrap. Actual definition polling, exact Start acknowledgement, expanded details and keyboard
focus retain their separate E38/E39/E42/E46/E47 runtime evidence.

Native explicitly records entry 37 as a physical 294×7264 PNG displayed at 243×6000 by the
tool despite the original-detail request. Overall layout, sections, wrapping and labels were
inspected; exact transcription of every long identifier at the reduced display scale is not
claimed. Named-element crops do not prove off-crop scrolling, hidden/expanded states or focus
behavior. Static rings and closed details do not replace the meaningful actual tests.

The plan/interface support T052's bounded paired capture/image requirement as scoped passed
at this current-build checkpoint. Task checkboxes remain Root's audit lane. UI file/asset
hashes do not fingerprint the Go source or capture-time executable: ordinary scenes launch
build/council.exe, while fault HTTP/Engine servers execute inside the tagged test process.
Recorded build/run ordering and source freeze support freshness without cryptographic dirty
source/process binding, and one executable's hash cannot identify the other.

Current working-checkpoint build/check already passed. Full npm test session 41097 subsequently
failed as E49 records below. E48 does not close the full
suite, comprehensive source review, checked revision/assets, all seven actual native/race
jobs or feature acceptance. Historical ninth/IR006-59 uncertainties and earlier failed/stopped
runs keep their separate outcomes. No source, tests or task checkboxes are edited here.

## Sixth full command: genuine pre-Stop prerequisite failure

E49 records 006-final-npm-test-sixth.log, session 41097 terminal exit 1 from an actual test
failure. This differs from E44's deliberate interruption. All non-Engine Go packages passed,
including Store 49.357s, Workspace 362.173s and the Go packagearchive package. Engine batch
1/23 passed 21.448s and batch 2/23 passed 124.124s. Batch 3/23 failed 91.093s: seven top-level
cases passed, and TestGraphAncestorStopBeforeFirstGOPreventsPreparedFollowerStart failed
10.24s at its fixed 8s pre-Stop observation. Batches 4–23, browser and Node script stages were
not reached. The run is not a complete matching suite pass, and later stages inherit no pass.

The failed predicate was that the first fixed ancestor response had actually settled for
acknowledgement, before the case sends Stop. Its retained snapshot shows required producer A
running, its workflow step active and no terminal outcome. The later Stop/follower assertions
were not reached, so this run is not evidence that accepted Stop allowed a follower to start.
The case's 10.24s includes its fixed observer and subsequent diagnostics/cleanup; it is not a
measured 10.24s model execution or proof that the native deadline expired.

Native's ongoing source investigation notes that both candidate and response artifacts exist
despite the persisted running phase. The traced ordering places those artifacts after the
clean owned-process cleanup returned, while candidate closure and terminal/graph proof work
can still precede terminal persistence. That distinguishes the persisted phase from exclusive
process runtime; artifact presence does not itself prove completed closure or terminal commit.
It does not identify why this 8s prerequisite was missed, establish a hang, justify a deadline
change, or exonerate this failure. No common cause with the earlier IR006-59 failures is claimed.

The exact production cause remains unproven. E50 below adds a measurement-only result, and
E51 records a subsequent bounded test-observer correction without retroactively passing the
original aggregate 8s qualifier. E48's fourteenth
capture and the earlier focused/check/package outcomes retain their bounded scopes. The full
current suite, comprehensive source review, checked revision/assets, all seven hosted native/
race jobs and feature acceptance remain open. No code, tests, deadlines or task checkboxes
are changed by this evidence update.

## Same-position ancestor diagnostic: measured completion beyond the original bound

E50 records the measurement-only replacement run, session 63506 terminal PASS, Engine
97.786s in 006-ancestor-diagnostic-same-position.log. The preflight
006-ancestor-diagnostic-order.log lists the same eight positions as the failed batch, replacing
only the fourth qualification case with TestGraphAncestorPrerequisiteMilestoneDiagnostic.
The three preceding and four following actual cases retain their order. This is a diagnostic
batch, not the original qualification batch or a full npm result; the preflight listing is
ordering evidence, not a runtime pass.

The original 8s aggregate qualification boundary was crossed at diagnostic elapsed
8.0015839s, explicitly logged as not a pass. At that instant A remained persisted running,
with candidate/response identities and no terminal. The captured stack was in
CaptureGraphOutput → inventory → Manager.git, waiting for Git at capture.go:266; that call
is ls-files --cached --others --exclude-standard -z. It observes actual output-closure work
in this diagnostic at the crossing, without establishing the location/cause of the earlier
sixth-run miss or a stuck worker.

The seven actual identity-bound milestones, measured from the diagnostic's fixed origin,
were:

| Milestone | Elapsed seconds | Gap seconds | Actual claims |
| --- | --- | --- | --- |
| Root ready | 2.6859474 | 2.6859474 | 0 |
| Ancestor input ready | 3.3018586 | 0.6159112 | 0 |
| Ancestor admitted | 3.6576940 | 0.3558354 | 1 |
| Ancestor actual start | 7.0406324 | 3.3829384 | 1 |
| Ancestor cleanup started | 7.0928374 | 0.0522050 | 1 |
| Ancestor response and candidate retained | 7.5613468 | 0.4685094 | 1 |
| Ancestor terminal response ready | 8.1592965 | 0.5979497 | 0 |

Each observed gap is below 8s; the cumulative original bound is nevertheless exceeded. The
replacement case completed at 18.35s including the exact original Stop/follower follow-up
assertions after diagnostic readiness. Terminal metadata retains the actual step/attempt,
settlement, graph-output closure artifacts and zero remaining claims. These observations do
not convert the original 8s prerequisite into a pass or qualify a future changed watchdog.

The extra database observations, milestone logging and stack snapshot may perturb the run.
This single same-position measurement does not identify the exact sixth-run cause, prove a
fix or exonerate its failure; no relationship to earlier IR006-59 failures is established.
E49 remains a genuine failed full attempt. Full/current-source qualification, checked
revision/assets, all seven native/race jobs and acceptance remain open. No source, tests,
deadlines or task checkboxes are changed by this documentation update.

## Test-only ancestor prerequisite watchdog with fixed job bounds

E51 records the independently reviewed test-only replacement for the shared ancestor
prerequisite's aggregate 8s observer. E49's sixth full-run failure and E50's same-position
measurement remain unchanged. The new clock in graph_ancestor_watchdog_test.go recognizes
exactly three complete real jobs: root-basis capture, ancestor input preparation and the
complete ancestor author. Each job has a fixed 8s observation allowance, with one immutable
24s absolute bound. Only a new ordered complete job with its exact identity advances the
clock. Polling, duplicates, phase changes, admission/start/cleanup and retained-output
observations do not renew the author or total bound; missing/skipped/replacement/extra jobs
cannot extend it.

Metadata probes require exact graph/root/receiver/version, control/budget epoch/window,
current complete input/root bindings, actual successful verified local settlements, and the
real healthy author/workflow/dispatch/source/output tuple. Complete author settlement remains
required; partial response/candidate retention is not enough. Physical verification stays in
the actual jobs, not manufactured by the probes. Core independently reviewed the clock,
strict facts and preservation of the original workflow/Stop/follower assertions and real
execution deadlines. This is a fixture observation correction, not a production scheduling,
claim, budget, process or delivery change.

006-ancestor-clock-tests.log passed the pure clock test and all five subcases, Engine 0.063s:
missing/late job; duplicate/phase noise; author intermediate progress; skipped/replacement/
extra jobs; and immutable total after completion. These use controlled time values to prove
the bounded clock; they do not measure Engine performance.

006-ancestor-batch-after-watchdog.log then passed the same-position original untagged eight
cases, Engine 98.588s, without replacing the qualification case by E50's diagnostic. The
before-first-GO Stop case passed 18.44s. Its actual three prerequisite completions were
2.8937106s, 3.4103989s and 8.1362403s, with gaps 2.8937106s, 0.5166883s and 4.7258414s.
That is qualification under the new finite job clock; it is not a pass of the old aggregate
8s observer. All original real workflow/Stop/deadline assertions remain. Current npm run
check passed in 006-watchdog-check.log, exit 0 confirmed by Root.

Only test files changed. Production Go, authored UI and compiled browser runtime are
unchanged, so E48's fourteenth visual runtime evidence retains its scope and is not invalidated
by this watchdog-only change. Checked-source qualification still includes the new tests.
Seventh full npm test session 24881 subsequently failed as E52 records below. No exact cause,
production fix, exoneration or common cause for the sixth
failure/IR006-59 is established. Full current suite/source review/checked revision/seven
native-race jobs/acceptance remain open. No code or tests are edited by this documentation
update, and no task checkboxes change.

## Seventh full command: all Go batches pass, one UI case fails

E52 records 006-final-npm-test-seventh.log, session 24881 terminal exit 1 from an actual UI
test failure. All non-Engine Go packages and all 23/23 Engine batches passed. This includes
the normal current tests under E51's bounded fixture clock; it does not retroactively pass
E49's sixth run or identify any earlier timing cause.

The browser stage passed 80/81 cases, with one failure, zero cancellations/skips and total
587089.2655ms. The failing case, graph guidance keeps exact pending intent and Advance
prepares only the new version under shared allowance, failed 10461.0568ms with TypeError:
Cannot read properties of undefined (reading 'data'), at graphs-resume.test.mjs:135:64.
That retained error is the current observation. No underlying owner, product, transport or
test-fixture cause was identified from that failure alone. E53 below adds subsequent source
analysis and a test-only correction; it does not establish the failed run's product outcome.

Node script tests were not reached after the UI failure. Earlier package/script passes,
the separate affected UI 12/12 pass and E48's scoped visual evidence retain their own outcomes;
they cannot be combined with these Go passes to relabel the seventh full command successful.
The current complete suite, comprehensive source review, checked revision/assets, seven
actual native/race jobs and acceptance remain open. No code, tests, deadlines or task
checkboxes are changed by this documentation update.

## Exact guidance acknowledgement: test-only observer correction

E53 retains E52's genuine seventh full-command failure while recording the subsequent
Core/Native independent source review. The test treated disappearance of the Retry label
as acknowledgement. The busy label can become Reconciling before the second saved reply
arrives, so that predicate permits access to guidanceReplies[1].data while the entry is
still absent. This is a source-confirmed observer race; the failed early observation does
not establish the product's guidance/receipt outcome or a production defect/fix.

Only ui/tests/graphs-resume.test.mjs changes. Before the explicit retry, it starts one fixed
10s deadline and waits for the exact POST path plus captured request ID. It verifies the
actual second successful response against the first saved response, then waits for removal
of that exact pending row using only the remaining deadline. Busy text alone is no longer
an oracle. The original 10s bound is not reset or extended; current/newer selection and
draft, exact request/replay, version/history, shared-budget/deadline and user-state assertions
remain. Core and Native independently reviewed the correction.

006-resume-ack-observer-green.log passed the complete focused case 1/1, zero failures,
cancellations/skips, case 15293.4934ms and total 16002.4024ms. Current npm run check exited 0
in 006-resume-observer-check.log. This qualifies the corrected observer and reached actual
focused behavior; it does not relabel E52 or constitute a full-suite pass. Production Go/UI/
dist runtime did not change, so E48's visual runtime scope remains valid at its checkpoint.

Eighth full npm session 30177 exited 1 in Engine batch 15 (E54). Complete current suite, comprehensive source review, checked revision/assets,
all seven actual native/race jobs and acceptance remain open. Earlier sixth/IR006-59 causes
remain unproven; no code, tests, deadlines or task checkboxes are edited by this documentation
update.




## E54: mixed-harness capacity fixture failure

The eighth full `npm test` session 30177 exited 1 in `006-final-npm-test-eighth.log`. All
non-Engine packages passed, including Store in 46.792s and Workspace in 353.726s; Engine
batches 1–14 passed. Batch 15 failed in 38.106s: seven top-level cases passed, while
`TestInteropMixedCapacityStopAndEarlierFailureAuthority` failed at `interop_control_test.go:32`
after 5.00s. The test expected the second Codex attempt to remain queued behind a Claude
attempt, but observed it preparing. Independent source analysis identifies an obsolete
single-slot fixture assumption against the current two-slot pool. That analysis does not
turn the failed run into a pass or alone establish a production capacity violation. E55 records the later focused fixture correction and pass; no matching full-suite pass is claimed. Batches 16–23, browser and
Node-script tests were not reached; checked revision, native/race and acceptance remain open.

## E55: explicit shared-account test fixture

The corrected mixed-harness test explicitly creates one shared declared account and links
both native contexts using their current revisions. It verifies the exact Claude account
claim, pins the subsequent Codex request and checks that it captured the same account;
Codex must stay queued without launch or source while Claude holds that reservation.
The original Stop replay, earlier-failure precedence, verified cleanup and later Codex
success assertions remain. This changes the test fixture only and does not reduce the
two-slot product pool. Independent review found no blocker. The focused case passed in
6.75s, Engine package 6.828s, in `006-interop-shared-account-focused.log`. Current
`npm run build` and `npm run check` exited 0 in `006-final-current-build.log` and
`006-final-current-check.log`. E54 remains a failed full command; this focused pass does
not establish a matching complete suite. Revision, seven native/race jobs and acceptance
remain open.

## E56–E57: local full run and hosted job allowance

The ninth local `npm test` command exited 0 in `006-final-npm-test-ninth.log` (session
91083): all non-Engine packages and all 23 Engine batches passed, UI passed 81/81 in
600541.5472ms and scripts passed 3/3 in 76888.1106ms. Store took 45.883s and Workspace
344.169s. This run began at HEAD `ee7128d`; graph-rebind/start/control browser tests
changed before its UI phase, and a macOS-specific Go test changed after its Engine case.
It is a successful mixed-source command, not qualification of an exact commit or of that
later macOS edit. The current canonical-executable focused case separately passed on this
host in 28.251s (`006-canonical-executable-focused.log`), not as a macOS platform run.
Earlier failures and their stated uncertainties retain their actual outcomes.

The Linux AMD64 race job 128480915091 in Actions run 42398483639 was cancelled after
about 20 minutes under its then 20-minute job limit; its log says the operation was
cancelled, not that race checks passed. Earlier native failures also remain historical.
Commit `267309e` adjusts only outer job allowances: native 30→60 minutes and race 20→90
minutes. Individual test/product deadlines, assertions, selectors and coverage are not
changed. The run for that commit (42402338856) was queued at the observed checkpoint;
all six native jobs and the Linux AMD64 race job must finish successfully for the same
source before the native gate closes. Checked revision and feature acceptance stay open.

## E58: scoped client journey reconciliation

The ninth mixed-source command (E56) passed the actual named browser, CLI and extracted
package cases. UI inspection checks complete originals, provenance and exact receiving
materialization while issuing zero browser writes. The actual CLI checks two native
dispatches before and after its input/member reads; the extracted package verifies
materialization and provenance digests and retains six consumed admissions before and
after its inspections. These are the assertions actually made, not a claim that every
individual UI read separately samples a native counter.

The browser control/guidance/rebind cases exercise exact group/node Stop, grant, held or
lost responses, newer selection/text and preserved history. The extracted package exercises
final delivery. E48 also records the actual 15-second common-window expiry scene after
the deadline with one retained attempt. This is browser/owner evidence; no direct CLI
expiry journey is claimed. T038 and T049 are checked for these scoped client journeys.
E56 remains mixed-source, and comprehensive review, exact revision, seven hosted
native/race jobs and overall feature acceptance remain open.
