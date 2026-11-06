# Task graph qualification quickstart

Status: planned006 validation procedure. Commands/fixtures below must be implemented and
observed before they become acceptance evidence. No006 runtime capability or provider call is
claimed by this design document. Prerequisite005 source794f080 and acceptancec5b1801 are accepted;
their seven-job pass does not qualify006. Keep the physical branch `refactor`.

Read [spec.md](spec.md), [graphs.md](contracts/graphs.md), [operations.md](contracts/operations.md),
[materialization.md](contracts/materialization.md) and [interface.md](contracts/interface.md).
Use disposable Git projects, private temporary homes and fixed offline workers/checks/publishers.
Do not probe vendor versions/login, inspect credentials or invoke real providers during these checks.

## Prerequisites and current source gates

Development uses pinned Go/Node/TypeScript tooling; extracted runtime needs only its native binary,
Git and a browser for UI. Existing native Codex/Claude interfaces are exercised through controlled
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

A declaration file uses the contract's exact body, for example:

```json
{
  "sharedInstructions": "Preserve unrelated project files and staged work.",
  "executionMode": "fake",
  "nodes": [
    {
      "key": "first",
      "originalText": "Produce the controlled first branch result.",
      "pins": {"harnessId": "codex"},
      "fakeScenario": "graph-branch-a"
    },
    {
      "key": "second",
      "originalText": "Produce the controlled second branch result.",
      "pins": {"harnessId": "claude-code"},
      "fakeScenario": "graph-branch-b"
    }
  ],
  "dependencies": []
}
```

The named graph fixtures are planned fixed cases implemented by the qualification tasks. They
are not arbitrary user-generated scripts or external executables. Use real returned IDs:

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

The test driver adds a `graph-join` node with file dependencies on both branches. Before admission,
both exact current predecessor results must qualify. Inspect full graph input records and actual
receiving files/index:

```text
council graphs inputs GRAPH_ID --json
council graphs input GRAPH_ID --input INPUT_ID --json
council graphs input-members GRAPH_ID --input INPUT_ID --limit 30 --json
council graphs definition GRAPH_ID --json
```

Use complete artifact reads for source/context/digest originals. Verify receiving workspace bytes
and staged blob content independently from the recorded manifest hash. Mutate the original checkout
and old managed workspaces after capture; neither can replace the retained selected inputs.

For the single final-output journey, select the join as finalNodeKey and explicitly capture
compatible existing `apply_verified_local` scope. Intermediate local/external effects are forbidden.
The final candidate receives current required checks/review before graph-root-to-final application.
Then final-node/group acceptance records the actual delivery, preserving unrelated user work/index.

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
| Response-only final after file branch | Separate review-only outputs can complete; requested unqualified file delivery remains blocked. |
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
