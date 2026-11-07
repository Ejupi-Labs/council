# Graph materialization, context and delivery contract

Status: proposed006 design; no implementation or runtime qualification is claimed.
Read with [graphs.md](graphs.md), [data-model.md](../data-model.md) and the accepted
[005 workflow contract](../../005-verified-delivery/contracts/workflow.md).

## 1. Ownership and compatibility

A graph-intent root task represents the user's actual group instructions. It is not an extra
declared worker node. It owns the canonical budget and real `local_graph` capture/composition
operations. These operations have attempt IDs, bounded deadlines, controls and retained outcomes,
but no native invocation, native model observations or native admission debit. They are not
authored candidates. Node work remains an ordinary task/workflow with explicit graph membership.

All new artifacts retain their actual producing attempt ID. Root capture and composition
artifacts belong to their real root-task operation; authored/check-produced artifacts belong to
their actual node execution. Never append evidence to an already settled producer, substitute an
operation ID for an attempt ID, or create a synthetic native execution to satisfy a foreign key.

The005 single-producer `Handoff`, source ownership and same-workflow candidate-binding guards
remain unchanged. Cross-task authority is established by the new graph records below. A receiving
source snapshot is owned by the receiving attempt; its graph input is a separate immutable link.
Its later candidate `RootSourceSnapshotID` remains within that receiving node's own workflow.

## 2. Common reference and control fields

IDs use existing canonical IDs. Counts/byte lengths/revisions use canonical nonnegative decimal
strings, with positive revisions and existing exact signed64-bit bounds. Times use existing UTC
timestamp rules. Digests are lowercase SHA256, except the explicitly named Git SHA1 object ID.
Nullable fields are explicitly typed; `null` is not an unknown-ID wildcard.

`GraphArtifactRef` is `{artifactId, producerAttemptId, sha256, byteLength}`. It names a closed,
complete registered artifact. Its required kind is determined by its containing field. A matching
string/hash without registered ownership and verified physical bytes is insufficient. Private
artifact paths, open handles and physical identities stay private; public references use IDs.
The bindings below are internal persisted/control records. Public projections defined by
`graphs.md` expose stable IDs, hashes and scoped source facts, not private native identities.

Operations carry the `GraphDispatchBinding` defined in the data model: graph/node control
revisions, receiving task version, exact contributor vector, owner instance, budget authority
epoch/window and input ID. Accounting revisions are not substituted for control authority.
Root-only operations omit the node binding, not graph controls. Each operation has a persisted
deadline: an already-open common window bounds it; before the first native admission a fixed
bounded local-preparation deadline applies. Local preparation cannot create/refill the common
native allowance or silently start a new window.

Every filesystem operation runs outside SQL. A durable operation exists before artifact/object
store/worktree effects. SQL transitions bind verified results and current controls; they never
perform filesystem callbacks. Ambiguous persistence acknowledges neither absence nor completion.
Operations retain `effectsKnown`, `evidenceStatus`, current failure and their actual effect-log
reference. The state/control record can be preparing/running/settled; only a complete immutable
result publishes a usable root/input/materialization binding. Uncertain artifact acknowledgement,
object-store effects or operation ownership fence unsafe new work rather than authorizing a
second operation. Check cancellation/deadline before each next local action; settle the actual
in-flight action without holding SQL/control during filesystem I/O. `local_graph` means no native
model launch, not a claim that its shell-free Git plumbing creates no operating-system process.

Canonical seals use UTF8 fixed-field JSON in the field order declared here, no insignificant
whitespace, every declared field present and explicit `null` for nullable absence. Required
arrays are `[]`, never `null`. References use `{artifactId,producerAttemptId,sha256,byteLength}`
field order; unordered contributor vectors sort by node ID, constraints/evidence by stable ref,
and tree/index/provenance/blob entries by their canonical path/key. Do not normalize Unicode or
original byte payloads. A record's own digest is excluded from its seal. The input/context
exception below is explicit; no generic recursive/self-referential hashing is permitted.

The new fixed artifact kinds are `graph_index_blob`, `graph_index_blobs`, `graph_output_closure`, `graph_input_manifest`,
`graph_input_provenance`, `graph_context`, `graph_context_digest` and `graph_operation_effects`.
The receiving-owned receipt artifact kind is `graph_materialization`.
`graph_original` retains a lossless original-text payload and `graph_acceptance` retains the
complete captured `TaskAcceptance` JSON. Only real root `local_graph` preparation creates these
source artifacts. They are not prompt/declaration substitutes and do not widen existing producer
roles; stage context references the exact required original subset.
Final-subject proof artifacts are `graph_conditions`, `graph_cumulative_delta` and
`graph_final_verification`.
Root/receiving source manifests keep `source_manifest`; actual selected outputs keep
`candidate_manifest`; raw indexes keep `index_snapshot`. These additive kinds are restricted to
their real graph-member/root-operation producers. They do not widen old artifact ownership.

## 3. Root, result and input records

### GraphRootBinding

Required fields:

- `graphId`, `rootTaskId`, `operationAttemptId`, `sourceSnapshotId`.
- `manifest`: source-manifest `GraphArtifactRef` containing exact working entries, semantic index
  entries and recorded exclusions; every regular-file member has its closed artifact reference.
- `index`: exact captured raw index-snapshot reference.
- `stagedBlobs`: closed `graph_index_blobs` manifest reference described below.
- `projectId`, private `projectIdentity`/`repositoryIdentity`, `selectedRelativePath`, `baseCommit`,
  `branch` (nullable).
- `basisSha256`: canonical digest of the complete fixed-field binding, excluding this digest.

Capture one stable working-tree/index basis, including supported dirty/untracked state. Recheck
source/index/HEAD/branch and physical directory identity during capture using accepted capture
rules. Equal commits do not prove equal dirty state. File branches never independently recapture
the current user checkout as their initial graph basis. A pure-response graph may retain source
for actual authorized access without inventing a file deliverable.

### AcceptedContribution

Required fields:

- `edgeId`, `edgeKind` (`files` or `context`), `fromNodeId`, `taskId`, `taskVersion`.
- `acceptanceKind` (`workflow` or `response`), `acceptanceId`, `producerAttemptId`, `inputId`.
- Exactly one of `candidateBinding` or `responseBinding`, using the complete005 binding.
- `outputClosureRef`: the exact associated closed `graph_output_closure` artifact. Its decoded
  record supplies the candidate or response/source/index/staged-blob references below.
- `constraintRefs` and `evidenceRefs`: bounded explicit references authorized by that edge.

The store validates current graph membership, version and acceptance before selecting this
record. Supporting review answers, partial candidates and native exit0 are not acceptances.
Context edges never enter file ancestry, even when their accepted response mentions files.
Acceptance IDs, producer IDs and bindings stay immutable after selection; later output can block
future use under current controls, but cannot retarget an existing input.

`GraphOutputClosure` is captured as part of actual node output before producer settlement:
`{graphId, nodeId, receivedInputId, kind, producerAttemptId, candidateId, responseArtifact,
manifest, index, stagedBlobs, closureSha256}`. Its seal excludes `closureSha256`. Kind is
`candidate` or `response`. Candidate kind requires the actual candidate ID, candidate manifest and
complete staged-blob closure, with `responseArtifact:null`. Response kind requires the exact
response artifact and its exact `ResponseBinding` source/index references; `candidateId:null`
and no authored candidate is invented. ALL new graph candidate and eligible response outputs
retain exact staged-blob closure, reusing verified identical payload references where possible.
This enables later selected-response handoff without inferring bytes from a different candidate's
after-index. It does not turn a context-only response into a file/index contribution.
An actual incidental candidate remains historical, not a response-created
file contribution. Candidate closure binds the full selected
candidate, not its last relative correction delta. A source-changing local check produces its
own candidate/closure and invalidates earlier applicability as in005. No closed producer is
reopened to fill a missing blob. Missing output closure is an explicit dependency blocker.

`GraphOutputObservation` is an append-once observation on the exact producing node dispatch:
`{producerAttemptId, candidate, response}`. Each nullable branch is present at most once:

- `candidate`: `{candidateId, state, closureRef, finalVerificationRef}`; the last reference is
  nullable and present only for the declared file-final binding below.
- `response`: `{responseArtifactId, state, closureRef}`.

`state` is `complete`, `incomplete` or `uncertain`; only `complete` requires a closed
`graph_output_closure` reference of that exact result kind/ID/producer. Missing references remain
null for incomplete/uncertain observations. A normal author/correction may have BOTH real results;
never store one ambiguous latest-output reference. A mutating local check associates its own
actual candidate closure, with no native debit. Supporting review responses are not eligible
author/correction response outputs. All node-owned workflow attempts, including local checks,
have an exact dispatch row for this association; root operations remain graph-input-owned.

Capture/publish each closure artifact while its producer is unsettled. Install the bounded
observation and producer terminal settlement in the SAME transaction, validating registered
ownership/hash, exact result identity and actual role. Do not append another branch later to a
terminal execution. Lost commit acknowledgement retains/fences the exact observation and cannot
authorize recapture. Ordinary005 candidate/response records and old wire JSON remain unchanged.

Accepted-contribution lookup selects `(producerAttemptId,resultKind,resultId)` and its associated
closure reference; it never scans artifact kinds or chooses the latest producer. Full retained
closure alone is not acceptance: actual terminal outcome, current version and required workflow
qualification remain mandatory. A failed producer's useful evidence remains historical.

### GraphInputBinding

Required fields:

- `id`, `graphId`, `graphVersion`, `rootBasisSha256`, `operationAttemptId`.
- `receivingNodeId`, `receivingTaskId`, `receivingTaskVersion`: the exact intended receiver.
- `contributions`: the exact selected direct inputs, ordered by edge ID; empty for a root node.
- `contributorVector`: deduplicated transitive accepted-result bindings, at most16 node heads.
- `manifest`, `index`, `stagedBlobs`, `provenance`: closed composition-owned artifact references.
- `contextPacket`: closed context-manifest reference.
- `inputSha256`: canonical digest of the fixed file-input fields through `provenance`, including
  IDs, root, contributions and contributor vector, but excluding `inputSha256` and `contextPacket`.

Hash direction is deliberate: seal the file-input digest, build the context packet referencing
that digest, then publish the immutable input row with its separately hashed `contextPacket`.
There is no mutually recursive hash. Dispatch binds the exact immutable input ID and verifies
both the file-input digest and context-packet reference; `inputSha256` alone is not context proof.

This is composed input evidence, never an authored candidate or proof of node acceptance. Root
operations may retain it before node admission. Receiving native admission records its exact ID
and digest. Preparing another input does not mutate an earlier input or replay completed nodes.

## 4. Working and staged blob closure

The `graph_index_blobs` artifact is a canonical sorted manifest:
`{contract:"006-index-blobs-v1", entries:[{objectId, contentSha256, byteLength, artifactRef}]}`.
Entries are unique by Git object ID and cover every semantic index entry, including symlink blob
contents. Supported object format remains the accepted SHA1 repository format; modes remain
`100644`, `100755`, `120000`. Unsupported object formats, conflict stages or source kinds block.

`objectId` is verified against Git's exact `blob <decimal-length>\0<bytes>` SHA1 construction;
`contentSha256` is SHA256 of the unmodified payload. Both and exact length must match. A payload
artifact has kind `graph_index_blob`, or may reuse an already closed `source_file` with identical
payload/hash/length and retained producer ownership. Working bytes cannot substitute for different
staged bytes. The blob manifest does not replace the full raw index artifact or its hash.

Root capture reads and retains actual staged blobs with before/after stable-index checks. Node
author/correction/check output captures its actual staged blobs before settlement alongside its
candidate/index. Composition references these verified immutable payloads; it does not read an
old mutable worker workspace to repair missing evidence.

Materialization uses existing isolated Git worktrees and shared repository object storage. For
every selected index object, verify type, size and bytes. If absent, restore only the verified
retained blob through shell-free Git plumbing and verify the returned object ID and readable
content. An unexpected existing object, unavailable repository or mismatched result blocks.
Never install Git, read credentials, change global/vendor settings or derive a command from prose.

Object restoration and managed worktree registration are **non-user-source effects**, not
effect-free reads. Their receipt records `kind`, `objectId` where applicable, `action`
(`already_present` or `restored`), source artifact ID, exact payload length and observed time.
They may add content-addressed objects/worktree administration in the existing shared repository;
they never modify the user's working paths, original index, branch/ref or commit. There is no new
repository architecture and no Git merge. Ambiguous local operation acknowledgement retains its
intent/evidence and fences unsafe dispatch; restart does not replay it automatically.

## 5. Causal composition

The provenance artifact contains sorted slots keyed by `(dimension,path)`, where dimension is
`working` or `index`. A working state is a complete entry or explicit absent tombstone; an index
state is a complete mode/object/blob reference or absent tombstone. Each slot records its selected
state digest and up to16 contributing graph-node origin heads, including source input and exact
accepted-result bindings. The root is an explicit origin. Referenced prior provenance remains
retrievable; previews cannot replace it.

For each accepted node, compare its ORIGINAL graph input to its full selected output. This
includes its accepted correction/check lineage and avoids losing earlier authored changes.
Unchanged slots preserve their input origins. Changed slots record that node's accepted result
and the precise input origins it supersedes. Graph/context reachability alone is not such proof.

At fan-in:

1. Require the same graph root and compatible physical repository/scope/source facts.
2. Deduplicate identical inherited origins. A proven descendant refinement supersedes only the
   contribution inherited through its file input; an unchanged sibling copy cannot undo it.
3. Retain causally maximal contributions. Equal complete states coalesce with all provenance;
   different maximal states conflict. Apply the rule independently to working and index slots.
4. Validate the resulting whole tree/index: path ancestry, duplicate/native name collisions,
   type/mode, exclusions, symlink targets and supported staging. Do not normalize conflicting
   names or pick a last writer. Actual materialization must establish native name compatibility.

Thus A→B/C→D accepts B's refinement of A when C merely carries A unchanged, plus C's independent
change. Divergent B/C edits block. The full root→selected manifests remain available; no Git/text
merge or model consultation resolves a composition conflict.

A conflict record is `{code, dimension, path, contributionRefs, inputId}` with bounded complete
references. It is recorded before any receiver native preparation. Both branches remain intact.

## 6. Actual materialization and verification

`MaterializeGraphInput` seeds only the first author of a node's new task version, or an explicitly
admitted author replacement bound to the same immutable graph input. It is not a general reset
operation. Subsequent review, correction and check preparation must use accepted005 `Handoff`
from the exact current selected node candidate/response and its retained index. The graph input
ID remains the stage's origin/provenance; it never replaces that selected subject. An attempted
original-input reset for these roles rejects before preparation. Do not discard earlier authored
or corrected changes by rebuilding a later stage from `GraphInput.manifest`.

`GraphSourceBasis` is captured in the immutable dispatch at THAT receiving step's admission:
`{basisKind, workflowId, stepId, inputId, inputSha256, candidateBinding, responseBinding,
expectedManifest, expectedIndex, expectedWorkingEntriesSha256, expectedIndexEntriesSha256}`.
`basisKind` is `graph_input` or `node_subject`:

- `graph_input`: only the authorized first-author/replacement cases above; both subject bindings
  are null and the expected source is the exact immutable graph input.
- `node_subject`: exactly one complete candidate/response binding matches the immutable005 step
  input at this receiving admission. The expected source is that selected subject, never whichever
  subject is latest when preparation runs. A later selection/control change may deny preparation;
  it cannot replace the captured subject.

The graph input ID/hash remains origin provenance in BOTH cases. Expected manifest/index fields
are exact retained `GraphArtifactRef`s. Raw source hashes are verified exactly. Semantic working
digest hashes sorted `(path,type,mode,size,contentSHA256,target)` plus recorded exclusion metadata,
excluding artifact IDs/producer identities. Semantic index digest hashes sorted `(path,mode,
objectId)`. These do not claim excluded payloads were copied. Record expected and observed raw
hashes separately: receiving-owned source metadata or rebuilt raw index bytes may differ while
the verified working/index states agree. No artifact hash is normalized or ignored.

Proposed workspace interfaces; names/signatures describe implementation obligations, not existing
methods. Request/result types are internal typed records, not arbitrary public filesystem inputs.

```go
CaptureGraphRoot(ctx context.Context, req GraphRootCaptureRequest) (GraphRootCapture, error)
CaptureGraphOutput(ctx context.Context, req GraphOutputCaptureRequest) (GraphOutputClosure, error)
ComposeGraphInput(root GraphRootBinding, inputs []VerifiedContribution) (CompositionPlan, []InputConflict, error)
RetainGraphInput(ctx context.Context, req GraphInputRetainRequest) (GraphInputBinding, error)
MaterializeGraphInput(ctx context.Context, req GraphMaterializeRequest) (GraphMaterialization, error)
ObserveGraphHandoff(ctx context.Context, req GraphHandoffObservationRequest) (GraphMaterialization, error)
VerifyGraphInput(ctx context.Context, binding GraphInputBinding) (VerifiedGraphInput, error)
RecheckGraphMaterialization(ctx context.Context, receipt GraphMaterializationReceipt, snapshot *Snapshot) (MaterializationProof, error)
BuildGraphContext(ctx context.Context, req GraphContextRequest) (GraphContextPacket, error)
```

- Capture request: real operation attempt, graph/root-task IDs, captured project/scope and current
  operation control/deadline binding. Output: ordinary retained snapshot plus `GraphRootBinding`.
- Output-capture request: still-unsettled producer attempt, actual candidate/index and immutable
  receiving-input binding. Output: exact closed working/staged closure; no new model admission.
- Composition input: previously store-selected accepted tuples plus physically verified closure;
  pure composition performs no SQL, filesystem, process launch or authority mutation.
- Retain request: real root-task `local_graph` operation, pure composition plan and exact control
  binding. It reconstructs/validates the logical index in a private managed workspace, retains the
  actual raw index/manifest/provenance/context, and publishes complete `GraphInputBinding` only
  after durable acknowledgement. This occurs before receiver native admission and adds no debit.
- Materialize request: exact retained input, actual already-durable receiving attempt, immutable
  source facts and control/deadline binding. Here `operationAttemptId == receivingAttemptId`:
  materialization is preparation of that real receiving execution, not a second hidden attempt.
  Its new source artifacts belong to the receiver. It adds no additional native admission; a
  previously admitted native attempt that fails preparation retains its actual debit and known
  no-process outcome. Public callers cannot supply paths/open handles.
- Verification returns an opaque internal verified value plus bounded artifact proofs. It is
  evidence of the checked tuple and current bytes, not a durable authorization lease.

Concrete internal request fields are:

| Type | Required fields |
| --- | --- |
| `GraphRootCaptureRequest` | `graphId, rootTaskId, operationAttemptId, projectId, selectedRelativePath, dispatchBinding, deadlineAt`; project paths/identities resolve from current server records. |
| `GraphOutputCaptureRequest` | `graphId, nodeId, producerAttemptId, receivedInputId, resultKind, candidateId, responseArtifact, manifest, index, dispatchBinding`; candidate/response fields follow the exact-kind rules above and the actual producer remains unsettled. |
| `GraphInputRetainRequest` | `graphId, graphVersion, rootTaskId, operationAttemptId, receivingNodeId, receivingTaskId, receivingTaskVersion, rootBinding, compositionPlan, contextRequest, dispatchBinding, deadlineAt`. |
| `GraphMaterializeRequest` | `graphId, inputBinding, receivingNodeId, receivingTaskId, receivingTaskVersion, operationAttemptId, receivingAttemptId, sourceBasis, dispatchBinding, deadlineAt`; attempt IDs are equal and sourceBasis is `graph_input`. |
| `GraphHandoffObservationRequest` | `graphId, inputBinding, receivingAttemptId, sourceBasis, sourceSnapshot, dispatchBinding, deadlineAt`; basis is `node_subject` and snapshot is the actual result of005 `Handoff`, not a public supplied path. |
| `GraphContextRequest` | `graphId, inputId, inputSha256, receivingNodeId, receivingTaskVersion, graphInstructionsRef, nodeInstructionsRef, guidanceRef, acceptanceRef, constraintRefs, contributedEvidence, supportedAccess`; no peer-defined execution policy. |

`GraphRootCapture` returns `{binding, sourceSnapshot}`; `GraphMaterialization` returns
`{receipt, sourceSnapshot}`. Snapshot paths/handles/private identities are internal runtime fields,
not public serialized proof. A root persistence row may use the separate `GraphRootBinding`
variant; a node `GraphInputBinding` always identifies its exact receiving task/version.

Materialization creates a fresh private worktree, restores/validates staged objects, reconstructs
its semantic index, materializes exact closed working members, then inventories and compares the
complete result. It retains its own actual index and a receiving-attempt-owned source manifest
and snapshot. It does not relabel predecessor artifacts as newly authored files.
`ObserveGraphHandoff` records/verifies the equivalent observation after subsequent005 selected-
subject handoff; it does not call the graph-input seeder or admit another execution. Both paths
use the same store recording/launch guard and check the receipt against the immutable dispatch's
`GraphSourceBasis` and step input.

`GraphMaterializationReceipt` is `{id, graphId, inputId, inputSha256, operationAttemptId,
receivingAttemptId, sourceBasis, sourceSnapshotId, observedManifest, observedIndex,
observedWorkingEntriesSha256, observedIndexEntriesSha256, stagedBlobsSha256,
operationEffects, effectCount, effectsKnown, evidenceStatus, failure, state, completedAt}`.
`operationEffects` is a closed
`graph_operation_effects` artifact reference, not an unbounded inline array; `effectCount` is an
exact count of at most100,001 object/worktree observations. `state` is `complete`, `blocked`,
`incomplete` or `uncertain`. Only complete receipts with known effects, complete evidence and no
failure carry a qualifying materialization; they require every snapshot/hash/effect-log field.
Observed semantic working/index digests must equal the expected semantic digests in `sourceBasis`,
and staged blob payloads must match the expected source. Expected/observed raw manifest/index
artifact hashes each remain exact evidence; equality of independently produced raw bytes is not
substituted for semantic state verification.
Otherwise unavailable snapshot/hash/effect-log fields are nullable, never fabricated empty hashes.
The receipt is retained as a closed receiving-owned `graph_materialization` artifact. Its ID and
source snapshot are append-once observations on the exact dispatch row; the original admission
JSON is not rewritten. A durably retained receipt describing uncertainty does not resolve that
uncertainty. Private runtime records additionally retain workspace identity/Git-control hash and
native effect detail. Partial bytes stay retained.

Verify all closed working members and staged payloads, not just top-level manifest hashes. Resolve
producer ownership against the recorded root/output/contributor chain. Deduplicate repeated member
verification without dropping origins. Filesystem closure runs outside SQL; bounded top-level
proofs bind immutable manifest/index/provenance hashes in the transaction. Reverify closure and
fresh graph/node/predecessor controls before native `Prepare` and GO. Stop during assembly may
leave attributable private preparation, never authority to launch.
At both boundaries, `RecheckGraphMaterialization` also re-inventories the actual receiving working
tree/index against its captured `sourceBasis` (graph input OR selected node subject), and checks
workspace/Git-control identity against the receipt; closed artifact hashes
alone do not prove the managed copy is still present. A mismatch blocks without replacing the
binding. This is a checked execution boundary, not a universal filesystem lock after GO.

## 7. Context packet and access

`GraphContextPacket` contains `{id, graphId, inputId, inputSha256, receivingTaskVersion,
graphInstructionsRef, nodeInstructionsRef, guidanceRef, acceptanceRef, constraintRefs,
contributedEvidence, derivedDigestRef, providedAccess, packetSha256}`. Optional absent material is
explicit. Each contributed reference identifies edge, exact source/producer/result, required
scope and original access. A digest is labelled derived and is generated deterministically with
zero model calls; it never replaces authoritative originals or observed verification.

`graphInstructionsRef` names a closed, lossless artifact containing ONLY the exact declared
`sharedInstructions` text, including an explicitly empty text when applicable. It is not a
reference to the entire graph declaration, title, sibling originals or unrelated conversations.
Retain the whole declaration separately for human provenance, but do not broadcast it to every
worker. Shared text is copied verbatim from its captured field, never extracted by an LLM,
summarized, normalized or reconstructed from other node instructions.

`providedAccess` entries are `{ref, artifactRef, scope, paths, mode}`, with mode `inline` or
`file`; whole scope has an empty path list. File access uses a controller-resolved private path in
the actual native launch context, never a trusted public path. Existing bounded artifact reads
serve human/API retrieval; they do not pretend a native process has an unavailable API tool.
Originals required by a stage are inline in full or available through supported complete file
access. Native fixtures must open/read them. A missing access capability blocks before launch.

Keep complete graph constraints, receiving original/guidance/conditions and applicable explicitly
contributed constraints. Do not broadcast unrelated transcripts or infer permissions from peer
commands. The immutable packet joins author/review/correction context without rewriting original
task text, changing literal pins or promoting predecessor policy to receiver authority.
For EVERY graph node, graph-shared plus current receiver constraints belong to its effective
instruction/condition binding. A direct deterministic proof must cover that complete authoritative
scope; a node-only literal proof cannot silently discharge an additional unassessed shared
constraint. Unsupported proof remains not-proven and follows the proportionate review/blocking
path. Empty shared instructions preserve eligible005 shortcuts without extra model calls.
Every node's `GraphConditionsBinding` below is retained before capturing its first immutable
workflow authority and before direct-proof assessment. It binds that exact receiver task/version,
not only a final node. Explicit dependency rebind captures a fresh version/condition binding;
it never replaces an existing workflow's authority in place.

## 8. Graph final delivery binding

### Actual unchanged final candidate and cumulative verification

Accepted005 already captures an actual candidate after a clean execution even when its creation
delta is empty. Its normal non-response planner returns action `candidate_change_required` with
reason code `no_authored_candidate_delta`.
For a designated FILE-final that inspects/joins inherited changes without editing, retain that
real complete observed candidate and its normal node-owned source. Do not manufacture an edit,
candidate, native call, admission debit or authorship claim to make integration possible.

`GraphConditionsBinding` is the generic closed `graph_conditions` artifact for EVERY node:
`{graphId, graphVersion, receiverTaskId, receiverTaskVersion, sources, conditionsSha256}`. Each source is
`{ref, authorityScope, nodeId, taskVersion, originalRef, guidanceRef, acceptanceRef}`; absent
node/guidance fields are explicit null. `authorityScope` is:

- `graph_shared`: user-declared graph-wide instructions/conditions, authoritative for every node.
- `receiver`: the current receiver's own original/guidance/acceptance, authoritative for it.
- `upstream_local`: complete attributed predecessor-local conditions and qualification context;
  they were obligations of that predecessor, not automatically final-state requirements or
  receiver permissions. Their historical outcome can be intentionally refined downstream.

Scope follows the captured user declaration, never an LLM extraction or prose heuristic.
Graph-wide constraints remain authoritative throughout. Preserve every original and relevant
upstream obligation in full with scope/provenance; do not silently promote node-local text to
global authority or clip it. Native graph context and that receiver's condition/applicability digests bind
this complete scoped record. The manifest seal excludes its own `conditionsSha256`.
For every newly captured graph node workflow, retain its ordinary node-condition digest separately
and seal the effective condition digest over that digest plus this scoped graph-conditions digest.
Capture that effective authority before workflow capture/direct proof and execution; candidate/review/check
bindings and native review `conditionsSHA256` use it consistently. Its inputs do not include the
later candidate/final-verification digest, avoiding a hash cycle. Existing standalone authority,
condition registries and historical hashes are not rewritten. The scope tags distinguish which
source text is authoritative versus attributed context; hashing context never grants permissions.
The final candidate's distinct node/task/workflow identity remains in `GraphFinalVerificationBinding`;
the generic condition record does not create a final-only exception for other nodes.

`GraphFinalVerificationBinding` is a closed `graph_final_verification` artifact containing:

- `graphId`, `graphVersion`, `finalNodeId`, `finalTaskId`, `finalTaskVersion`, `finalWorkflowId`.
- `rootBinding`, actual `finalCandidateBinding`, `outputClosureRef`, `finalInputId` and exact
  `contributorVector`.
- `graphConditionsRef`, `graphConditionsSha256` and the exact scoped original access references.
- `cumulativeDeltaRef`: closed `graph_cumulative_delta` computed from verified original graph
  root to actual selected full manifest/index, including retained before/after bindings.
- `hasCumulativeWorkingDelta`, `hasCumulativeIndexDelta` and `bindingSha256` (seal excludes itself).

In `graph_cumulative_delta`, `graph_final_verification` and graph delivery proof artifacts,
`rootBinding` is the existing public `GraphRootView`: stable IDs, retained artifact references,
source scope and the original `basisSha256`. It excludes private `projectIdentity` and
`repositoryIdentity`. The controller/store must load and compare the complete private root from
persisted state for physical verification; the root seal binds that private identity without
serializing it into a readable artifact. Raw control artifacts have no privacy exception.

This is a verification BASIS, not a passed verdict. Construct in acyclic order while the actual
candidate producer remains unsettled: output closure first, then this binding referencing it,
then the candidate branch's `finalVerificationRef` in the append-once output observation. Install
that association with terminal settlement. This also covers a new final candidate produced by a
mutating local check/correction; an older candidate's basis cannot qualify the new subject.

Every final review/check dispatch records that exact basis reference, retaining selected-subject
access plus actual original graph root, scoped conditions and relevant contributed evidence.
Its applicability binds root, final candidate, contributor vector and graph conditions. Existing
node/input-only receipts or predecessor receipts do not automatically become cumulative proof.
Verify physical closure outside SQL, then current registered binding/control tuples inside SQL.

For these final `node_subject` stages, the optional `GraphSourceBasis.finalVerificationRef`
records the exact selected basis; ordinary dispatches omit it. The actual stage controller
rechecks the association and complete physical closure before preparation, GO and settlement.
Final review/check applicability uses the existing applicability shape with a distinct
`006-final-applicability-v1` source-control digest over the exact final reference and binding
seal. The original check/review definition and effective conditions digests remain unchanged.
Do not apply this loader globally while a new author/correction output is still being captured:
its own final basis is associated only at terminal settlement, so an older basis cannot grant
evidence reuse to that new candidate.

Role context supplies the retained final binding, cumulative delta and root manifest/index/staged
envelopes, plus complete changed/deleted before payloads and distinct staged before/after payloads.
Unchanged root bytes may use identical physically verified selected bytes. Transport stays in the
existing private per-attempt context and shares its total64-reference/256KiB prompt bounds with
ordinary subject/context references. Excess or unavailable access blocks explicitly; no private
state path, shortened original or predecessor verdict substitutes for cumulative verification.

For a captured command check on this final graph subject, preserve executable, argv and stdin.
Provide the single reserved `COUNCIL_GRAPH_CONTEXT_FILE` environment variable pointing to an
owner-authored descriptor in that same private context directory. The descriptor identifies
copied fixed ordinal files, exact closed references and authority scopes, including complete
original/condition access through the existing packet. Inherit other process environment values
without logging or persisting them; replace only this reserved key using the platform's key
comparison. Ordinary005 command checks retain their existing environment behavior. Availability
of this descriptor is access evidence, not proof that a command read it. Unknown context-copy or
acknowledgement effects prevent qualification and retain their exact effects/ownership even when
the command process itself was cleaned up successfully.

The pure planner gains optional INTERNAL `GraphFinalPlanFacts`:
`{verificationBindingRef, declaredFileFinal, currentSubject, hasCumulativeWorkingDelta,
hasCumulativeIndexDelta, cumulativeDirectProven}`. Only controller/store-verified facts for the
designated non-`response_only` file-final with a nonempty cumulative delta can satisfy the early
file-subject eligibility test despite an empty node delta. A public boolean cannot enable it.
All remaining required check/review/finding/settlement/delivery guards still apply, using current
cumulative applicability/direct proof rather than a zero-delta shortcut. Ordinary standalone and
explicit response-only planning retain their exact005 behavior. A response acknowledgement never
promotes inherited files into qualified delivery.
`currentSubject` is the complete actual final `CandidateBinding`, not a supplied true/false flag.
The controller's private `WorkflowStepFacts.cumulativeDirectProven` is separate from ordinary
node `directProven`, including builtin/supporting stages with no new graph output. Store ignores
that fact unless it independently derives this exact registered selected final basis. Producers
use their newly captured same-terminal basis; later stages use their persisted source-basis
final reference and fresh proofs. No duplicate or fabricated output is required for a builtin.

Keep `Candidate.HasDelta`, `HasIndexDelta` and node/workflow creation/change facts truthful to their
existing node-local bases; they remain false when the node did not edit. Graph views separately
label inherited/cumulative graph changes. In particular, working changes must not be relabelled
staged changes because index context exists. A zero-own-delta final still needs fresh cumulative
qualification before delivery/acceptance; it does not inherit an approval by doing nothing.

### Final effects and acceptance

`GraphDeliveryBinding` is `{graphId, graphVersion, graphControlRevision, rootBinding,
finalNodeId, finalTaskId, finalTaskVersion, finalWorkflowId, finalCandidateBinding,
finalInputId, contributorVector, finalVerificationRef, otherNodeAcceptances, preDeliveryEvidenceUses,
deliveryScopeSha256, bindingSha256}`. It is derived from current server records. All other nodes
must have exact current accepted outcomes; required file ancestry must be incorporated through
file edges. The final cumulative candidate requires current applicable pre-delivery checks/review
and known prior settlement. Its own acceptance is not a prerequisite for its delivery.

The graph delivery operation records an additive optional `graphDeliveryRef` to the closed
binding; standalone005 operations omit it and historical records stay byte-exact. The controller
supplies private `GraphDeliveryFacts` (typed binding, exact binding reference and bounded fresh
artifact proofs), never a public effect-enable flag. Graph Begin/Ready and each durable permit
must join that exact operation binding, current selected final, required other acceptances and
graph/node/window controls. Complete retained-source physical verification runs before those
transactions outside SQL/control; the transaction checks registered tuples before granting the
unchanged005 single-effect permit. A changed/stopped scope receives no next permit.

The final candidate keeps its ordinary005 node-owned source ancestry. The graph binding separately
authorizes comparing original graph root to actual final selected manifest. The controller uses
existing `PrepareApply` with verified graph-root snapshot as `Source` and the actual final
candidate/producer as `SelectedCandidateID`/`SelectedAttemptID`. No native application algorithm
or005 candidate guard is relaxed. Unsupported005 file/index/metadata application remains a named
blocker; graph composition does not expand supported destination mutations.

The real final local-application attempt owns delivery artifacts and the existing operation/action
journal. Each single-effect permit atomically checks graph/node controls, exact graph delivery
binding, current final authority, shared budget/window and no conflicting operation. Native I/O
runs outside control/SQL. Stop prevents the next permit; an already permitted compound publication
finishes required metadata restoration and records its actual partial/unknown outcome. Close and
known settlement precede capacity release. Never replay an uncertain destination/external effect.

Intermediate local AND external actions are forbidden. Only explicit captured final scope may run
an external command, under005 command-completion semantics without remote attestation. Required
delivery precedes final-node and group acceptance. Review-only mixed outputs may complete as
separate qualified files plus an accepted response; a response alone cannot qualify/apply nonempty
cumulative file changes. Pure responses need no fabricated candidate/integration.

## 9. Bounds and rejection

| Item | Limit/rule |
| --- | --- |
| Graph | 16 declared nodes,64 edges; no extra implicit worker node. |
| Tree/index | Existing100,000 entries per manifest/index;64MiB per regular-file or staged-blob payload. |
| Working bytes | Existing512MiB sum of working regular-file sizes. |
| Combined working/staged |512MiB: working sum plus unique staged payloads not already represented by identical SHA256/length working content. Count each additional staged content once. |
| Manifest/raw index/blob manifest |64MiB each; complete validation, no shortening. |
| Provenance |64MiB per complete artifact; at most16 contributing node heads per slot and16 transitive selected node results. Ordinary within-node005 lineage remains separately retained. |
| Supplied context | Existing64 provided references,256 paths per declared subset and256KiB final native prompt. Original artifacts remain completely retrievable. No byte-to-token inference. |
| SQL filesystem proofs | Existing bounded top-level proof envelope (at most256); member verification stays outside SQL. Overflow blocks, never omits a denial/member/condition. |
| Operation effects/conflicts | Complete closed artifact up to64MiB; public metadata is bounded/paged by exact input/operation identity. No unbounded event/prompt expansion. |

Bounds apply before corresponding dispatch/effects. Known unsupported/racing/missing input becomes
a named blocker. Durability/ownership/effect uncertainty is retained and fences unsafe new work;
it is never downgraded to an ordinary missing prerequisite to enable retry. A new explicit request
cannot duplicate an already accepted or uncertain effect; exact replay returns its old receipt.

## 10. Required actual qualification

1. Capture a dirty root whose staged blob differs from working bytes; retain both, remove the
   staged object from a disposable fixture repository, restore from closed evidence and prove
   actual follower working bytes AND `git show :path` content. Original index remains byte-exact.
2. Two actual independent workers overlap; their accepted outputs form a follower's real input.
   Later checkout/old-workspace edits and corrupt unchanged members cannot become trusted input.
3. A→B/C→D refinement succeeds once; divergent siblings, modify/delete, path/type/mode/staging,
   native name and different-basis cases block before native preparation with both results retained.
4. Missing/wrong-owner/mismatched blob or full member closure fails; no synthetic hash tuple is
   accepted as physical proof. Lost operation acknowledgement/restart causes no automatic replay.
5. Actual native-role fixtures read provided originals and raw relevant evidence; critical
   conditions survive derived context, and missing/oversize input produces no native admission.
6. Final root→candidate application preserves unrelated original edits/index; one-effect Stop,
   guidance/final-version races, unknown acknowledgement and metadata refusals retain005 behavior.
   Intermediate publisher declarations produce zero effects. Final acceptance follows delivery.
7. A declared file-final actually reads/verifies its inherited A/B files without editing. Its
   retained real candidate has zero node creation delta; fresh cumulative verification and graph
   root→final delivery succeed without a dummy edit. Explicit response-only summary, standalone
   zero-delta, stale basis and predecessor-only verification counterexamples do not bypass guards.

All tests use controlled local fixtures and supported Git/native helpers. New006 behavior needs
actual six-platform/race qualification after implementation; accepted005 results do not qualify it.

## 11. Native context transport clarification

The immutable packet is an access plan, not evidence that a model consumed its contents.
Every required original, guidance/acceptance record, constraint and contributed evidence reference
requires complete supported inline/file access. Identical artifact aliases may share a complete
transport while their source scopes remain distinct. Both packet construction and physical input
verification reject a missing plan. Actual native preparation verifies the transport separately.

`PrepareGraphContext(ctx, GraphContextPrepareRequest{receivingAttemptId,inputBinding})` returns
private `GraphPreparedContext` facts: packet, directory/identity, entries with exact source refs,
file paths or inline text, effect reference, effectsKnown, evidenceStatus and state.
`RecheckGraphContext` revalidates the private copies before `Prepare` and GO and after cleanup.
Paths and native identities do not enter the public packet. A held private directory root is
released through `Close`; the actual controller owns lifetime and current dispatch authorization.

File transport uses a fresh private sibling directory
`workspaces/graph-context-<receivingAttemptId>`, outside every candidate worktree. Only selected
closed artifacts are copied to fixed controller-generated names; no hardlinks, symlinks,
unrelated histories or whole-state-directory grants are introduced. Existing unresolved directories
are retained and rejected. Copies remain attributable preparation effects. Per-file64MiB and
aggregate512MiB copy bounds are local I/O limits, not estimates of provider token capacity.

The adapter adds a separate `graphContext` property to the existing native prompt JSON without
rewriting original task fields, pins, permissions or role. Complete receiver original/guidance/
acceptance bytes already present identically in that base JSON are referenced there rather than
duplicated. The TOTAL graph plus ordinary workflow subject/check/review references remain at most64;
the actual complete encoded prompt remains at most256KiB. Missing access or excess size is an
explicit pre-native-Prepare blocker; no clipping, model summarization or silent pin change occurs.
An earlier base-builder size refusal is also an explicit capacity blocker, not permission to
replace stored task text.

For Claude, only the exact prepared directory is passed through the documented `--add-dir`
file-access mechanism, retaining the existing review Read/Glob/Grep profile. Codex `--add-dir`
grants writable roots and is not added to a read-only review. Installed readable-root policies,
managed restrictions and native sandbox accounts may still deny access. Council does not relax
private ACLs, rewrite vendor settings or promise a universal sandbox to bypass them. Such failures
remain access gaps; documented profile support and controlled fake reads are not live qualification.

## Stable internal edge identity

An immutable declared edge has one deterministic identity across complete reads, exact retries and
input preparation. `GraphEdgeID` hashes UTF-8 fixed-field-order JSON with keys `graphId`,
`graphVersion`, `fromNodeId`, `toNodeId`, `kind`, in that order, using the persisted opaque node
identities and canonical graph version. Its ID is the first 32 lowercase hexadecimal characters of
SHA-256. Inputs must have canonical valid IDs/version and kind `files` or `context`. This internal
identity rule grants no dependency, acceptance or publication authority; each selection must still
match the actual immutable declaration and current accepted contributor. Reads allocate no IDs.

## Receiving namespace qualification

Composition does not infer filesystem case behavior from an operating-system name. The engine
must preserve supported case-dependent trees. If both explicit case policies produce identical
successful semantic plans, or the same known causal conflict, no additional probe is necessary.
When their outcomes differ, qualify the actual newly allocated managed assembly directory before
selecting a policy. Existing causal conflicts that are independent of case remain reportable
before assembly allocation.

`ProbeGraphNamespace` is a private local preparation operation scoped to that exact managed
snapshot/native directory identity and retained Git controls. Its scope is
`ascii_case_in_exact_directory`, not a claim about every Unicode collation or other directories.
Retain an acknowledged intent before exclusive random sentinel I/O, determine same-file versus
distinct-file behavior using retained handles, remove only exact owned sentinels, verify absence
and unchanged identity/controls, and acknowledge terminal effects. Failed acknowledgement or
uncertain cleanup yields no usable case capability and preserves ownership. Private native identity
is absent from the effects artifact and public projections. Complete actual materialization and
pre-GO inventory/index checks remain required for all native name/Unicode compatibility.
