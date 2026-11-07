# Task graph qualification quickstart

Status: dev.6/006-v1 implementation under qualification, not feature acceptance. Features001–005
are accepted;007–012 remain planned. Prerequisite005 source794f080 and acceptancec5b1801 do not
qualify006. The [draft matrix](evidence-matrix.md) and [active review](review.md) record actual
bounded checks and remaining gates. Use one final checked revision for the complete local,
visual/client/package and seven-job native/race acceptance. Keep the physical branch `refactor`.

Read [spec.md](spec.md), [graphs.md](contracts/graphs.md), [operations.md](contracts/operations.md),
[materialization.md](contracts/materialization.md) and [interface.md](contracts/interface.md).
Use disposable Git projects, private temporary homes and fixed offline workers/checks/publishers.
Do not probe vendor versions/login, inspect credentials or invoke real providers during these checks.

## Prerequisites and current source gates

Development uses pinned Go1.27.3, Node24.18.1 and authored TypeScript7.0.2 tooling; extracted
runtime needs only its native binary, Git and a browser for UI. Current execution/workflow/graph
contracts are006-v1 with app2.0.0-dev.6; older known owners have only maintenance Stop compatibility. Existing native Codex/Claude interfaces are exercised through controlled
fixtures. Fixtures create exact source/index contents in their own temporary projects, including
different staged/working blobs, Unicode/CRLF and unrelated dirty files.

Run the serialized local queue against the final source and built UI:

```text
npm run build
npm test
npm run check
```

Record actual commands, source revision, terminal outcomes, zero/nonzero skips/cancellations,
offline/native classification and limits. Cross-compilation alone is not a native gate.

## Small independent group

Open the disposable prepared fixture project with existing `projects open`. The optional UI
Create task group starts with two instruction fields and no title/model/stage wizard. It shows
Separate reviewable results, saved autonomy, up to two workers and the actual shared allowance.
A graph has at most16 required nodes and64 acyclic file/context edges and one finite root allowance/
common window. Complete JSON is bounded at2MiB. The fresh saved admission default remains four;
existing saved defaults and all consumed units stay unchanged. Context/account reservations or an
explicit maxParallelism of1 narrow the default two-slot capacity. A held migrated legacy claim
remains a sole dispatch barrier until settled.

A declaration file uses the contract's exact body, for example:

```json
{
  "sharedInstructions": "Preserve unrelated project files and staged work.",
  "executionMode": "fake",
  "routing": {"budget": {"admissionLimit": "4"}},
  "nodes": [
    {
      "key": "first",
      "originalText": "Produce the controlled first branch result.",
      "pins": {"harnessId": "codex"},
      "fakeScenario": "graph-branch-a",
      "workflow": {"review": "required"},
      "acceptance": {"deliverable": "candidate"}
    },
    {
      "key": "second",
      "originalText": "Produce the controlled second branch result.",
      "pins": {"harnessId": "claude-code"},
      "fakeScenario": "graph-branch-b",
      "workflow": {"review": "required"},
      "acceptance": {"deliverable": "candidate"}
    }
  ],
  "dependencies": []
}
```

Save this as graph.json in the disposable test directory. These implemented fixed offline
fixtures create controlled files and native-shaped responses; they never call a provider or
execute arbitrary user-authored fixture scripts. Use real returned project IDs and a new32-hex
request ID for each intended mutation:

```text
council graphs start --project PROJECT_ID --graph-file graph.json --request-id REQUEST_ID --json
council graphs get GRAPH_ID --json
council graphs wait GRAPH_ID --after-revision REVISION --timeout-ms 1000 --json
council graphs budget GRAPH_ID --json
council graphs budget-ledger GRAPH_ID --json
```

Prove overlap from actual owned execution intervals/barriers, not simultaneous queued badges.
Each native admission debits the root once; local preparation/digests add none. Repeat the exact
start request after a deliberately lost acknowledgement and prove identical graph/node identities,
unchanged budget and no extra worker. A one-slot/shared-account supplement must serialize truthfully.

## Dependencies, provenance and final delivery

For a dependent final, make a new declaration from the example above: append the node below,
replace dependencies with the two file edges, set finalNodeKey and capture delivery. Set the
single root routing.budget.admissionLimit to `"6"` to cover the three required author/review pairs.
Do not add a child budget. These are fields in the same complete declaration, not separate requests:

```json
{
  "finalNodeKey": "final",
  "delivery": {"localMode": "apply_verified_local"},
  "dependencies": [
    {"from": "first", "to": "final", "kind": "files"},
    {"from": "second", "to": "final", "kind": "files"}
  ]
}
```

Append this object to nodes, retaining the first/second nodes and complete shared instructions:

```json
{
  "key": "final",
  "originalText": "Inspect both controlled predecessor files without changing them; preserve original working and staged bytes.",
  "pins": {"harnessId": "codex"},
  "fakeScenario": "graph-join-no-edit",
  "workflow": {"review": "required"},
  "acceptance": {"deliverable": "candidate"}
}
```

Save the complete result as final-graph.json and start it with a distinct request ID. Every
required node must qualify, and file incorporation requires file edges. A context-only edge can
supply an accepted response/evidence without supplying file contents. The real receiving workspace
is reconstructed from retained exact inputs; original checkout paths or old workspaces are not
availability proof. Inspect full graph records and actual receiving files/index:

```text
council graphs inputs GRAPH_ID --json
council graphs input GRAPH_ID --input INPUT_ID --json
council graphs input-members GRAPH_ID --input INPUT_ID --limit 30 --json
council graphs definition GRAPH_ID --json
```

Use complete artifact reads for source/context/digest originals. Focused packets keep graph-wide,
receiver and applicable upstream conditions with exact producer/original references; derived digests
are labelled. The combined graph/workflow prompt is limited to256KiB and64 supplied references.
Missing required access or excess material blocks without clipping; these byte/reference limits do
not claim a known provider token capacity. Verify receiving workspace bytes
and staged blob content independently from the recorded manifest hash. Mutate the original checkout
and old managed workspaces after capture; neither can replace the retained selected inputs.

For the single final-output journey, select the join as finalNodeKey and explicitly capture
compatible existing `apply_verified_local` scope. Intermediate local/external effects are forbidden.
The final candidate receives current required checks/review before graph-root-to-final application.
Then final-node/group acceptance records the actual delivery, preserving unrelated user work/index.

## Guidance, Advance, shared grants and Stop

Use separate intended mutations and request IDs; execute only the control needed for the scenario:

```text
council graphs guide GRAPH_ID --node first --guidance-file guidance.txt --request-id GUIDANCE_REQUEST_ID --json
council graphs advance GRAPH_ID --request-id ADVANCE_REQUEST_ID --json
council graphs advance GRAPH_ID --rebind-node final --request-id REBIND_REQUEST_ID --json
council graphs grant GRAPH_ID --additional-admissions 2 --request-id GRANT_REQUEST_ID --json
council graphs stop-node GRAPH_ID --node first --request-id NODE_STOP_REQUEST_ID --json
council graphs stop GRAPH_ID --request-id GROUP_STOP_REQUEST_ID --json
```

With proof flags omitted, the CLI captures the current complete proof group once and stores the
exact intent privately before sending. Retrying the same request ID uses that capture even after
owner recovery; changed input with that ID conflicts. It never silently refreshes a failed/uncertain
request from newer graph state. To supply proofs explicitly, Advance requires all of
`--graph-version`, `--graph-control-revision`, `--budget-authority-epoch`, `--window-number` and
`--node-authorities-file`. That UTF-8 JSON file contains the complete array of current
`{nodeId, taskVersion, nodeControlRevision}` tuples, with string counters. Other controls likewise
accept their full documented proof group or none; partial groups reject.

Guidance appends a new version without starting work or clearing historical attempt identities.
Advance prepares or admits only eligible unfinished current work within the same allowance/window.
It does not replay completed work. Explicit repeatable `--rebind-node KEY` names the captured
stale-input nodes to derive new dependency_rebind versions. Originals, guidance, pins, acceptance,
checks and captured policy remain; old workflow/input/result evidence stays readable. Omitting
rebind keys never rewrites an existing workflow. A grant adds only finite authority; it starts no
stage, creates no child budget and rearms no old automatic callback. Follow it with a new deliberate
Advance when eligible work should proceed.

Stop group cancels admitted work and prevents later admissions/effects. Stop node cancels its own
execution and blocks unlaunched descendants while independent admitted siblings settle normally.
An ancestor Stop after a receiver's known first GO does not retroactively invalidate that unchanged
captured input; changed ancestor guidance/input, receiver Stop and group Stop still bind. Inspect
actual cleanup and retained evidence. Reading/waiting or restarting never grants fresh execution.

## Required counterexamples

| Scenario | Required observed outcome |
| --- | --- |
| Invalid/cyclic/oversize graph or intermediate publisher declaration | Validation error; zero worker/publisher effects; fields retained. |
| Failed or unverified predecessor | Receiver cannot prepare; admitted independent sibling retains its result. |
| Wrong version/producer, corrupt unchanged member or missing staged blob | Exact named input blocker; no receiver native preparation or fake availability proof. |
| Divergent edits, modify/delete, path/type/index conflict | Both accepted results retained; no silent Git/text merge or last-writer selection. |
| A→B/C→D diamond with B refining A and C carrying A unchanged | B's refinement and C's independent change appear once; divergent refinements block. |
| One sibling's ordinary budget debit | Other sibling's valid continuation survives; an actual last-unit race admits at most one native worker. |
| Direct node grant/run/continue/retry bypass | Canonical shared budget/control enforced; no fresh child allowance or Stop escape. |
| Node Stop with running independent sibling and waiting descendant | Cancel own work; descendant cannot start; independent admitted work settles honestly. |
| Ancestor Stop before versus after receiver first GO | Before known GO deny launch; after known GO preserve unchanged admitted descendant continuation. Own/group Stop and changed ancestor guidance/input still bind. |
| Changed accepted input after workflow capture | Ordinary Run reports graph_input_stale; explicit named Advance rebind creates a derived task version/provenance without allowance/refill, preserving old evidence. |
| Group Stop during materialization/preparation/permitted delivery | No later admission/effect; actual prior effect/cleanup retained with truthful outcome. |
| File-final verifies inherited files without own edits | Retain real zero-own-delta candidate/flags; fresh cumulative basis/checks/review qualify graph-root changes before exact delivery. |
| Missing/oversize/inaccessible required context | Named blocker or permitted supported alternative; no clipped original or model summary call. |
| Response-only final after file branch | Review-only may accept the exact summary and complete separate outputs. Requested file application refuses that acknowledgement with application_unsupported while required cumulative file qualification/delivery is absent; retain both results without final acceptance or effects. File checks alone do not automatically resume an incompatible final. |
| Pure-response group | Complete only through actual response acceptance, without fabricated candidate/application. |
| Required context-only sibling unfinished | Group/final delivery cannot complete; context reachability is not file incorporation. |
| Lost mutation/materialization acknowledgement or restart | Same retained identities/evidence; no automatic replay or reconstructed model output. |
| Dirty/further-edited destination, partial or uncertain application | Preserve both sides/index; report actual conflict/partial/uncertain effects, never success. |

## UI and extracted runtime

Verify actual owner-backed group composer/overview/node/input/budget/control/delivery and history
journeys through UI and CLI. Hold real responses to test newer typing/project/node/history selection,
captured request retries and Stop targets independent of historical response producers.

Capture desktop1440px and360px states listed in interface.md. Inspect all actual images independently;
verify keyboard/focus, English text states,44px controls, reduced motion, no overflow and matching
approved source/dist/served assets. Source hashes alone are not visual acceptance.

The extracted archive must pass exact allowlist/notices/checksums and native graph journeys under
a Git-only execution PATH excluding Node/npm/Go. Keep private state, fixture files and screenshots
outside Git/packages. No package publication or deployment is part of qualification.

Finally push the independently reviewed source/assets to `refactor` and follow all six actual
Windows/macOS/Linux AMD64/ARM64 jobs plus Linux AMD64 race at that exact revision. Retain failures,
fix/review/rerun them and close all24 GRA/ten SC rows before007. Do not mark the parent twelve-feature
goal complete at this slice. Live provider/account/quota/token/charge/savings evidence stays separate.
