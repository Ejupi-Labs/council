# Metadata-Only History — Research Draft

**Status: draft proposal, not an approved contract or implemented feature.**
This profile explores useful inspection of selected current project history
without fetching opaque stored record bodies or artifact bytes. It is explicitly
**incomplete and non-executable**; it does not by itself satisfy RET-004 or
SC-001, preserve complete project state, establish payload eligibility or resolve
the user's pending decision about explicitly selected opaque production content.

## Source boundary and provenance

The candidate source is the accepted schema7 SQLite database opened by the current
locked Council owner. Use only expressly selected dedicated columns below. Do not
select `data`, `authority_data`, `dispatch_data`, envelopes, private path fields,
credentials or raw database/WAL/SHM files, and do not use `SELECT *` or
`json_extract` on opaque values to synthesize supposedly safe fields. Do not open,
read, hash or copy artifact payloads, project files, agent output or vendor
authentication. Whole-row helpers in the existing Store often decode `data` and
are not suitable for this profile.

Council normally creates project, draft, task, attempt, graph, node and many other
logical IDs internally (for example `internal/store/workspace.go`, `tasks.go`,
`graph_create.go` and `internal/evidence/files.go`). Validate their exact expected
format and ensure each is associated with a row whose Council-generated origin
and selected-project association are established, including legitimate earlier
owner epochs. A 32-hex shape alone is not proof of generated provenance:
request IDs and other fields may be caller supplied, and imported or tampered
records need separate treatment. Do not export request IDs, arbitrary node keys,
logical slot names, user-authored labels, account IDs, paths, notes, environment
or owner handles merely because a column is a string.

This proposal's no-opaque-read boundary means **no application-level access,
decoding, copying or export of opaque column values**. SQLite may physically
read shared disk pages containing other columns while satisfying a selected-column
query; this is not proof of zero physical page reads. If the user's credential
boundary requires avoiding even such physical page access, the existing shared
SQLite layout cannot establish that stronger guarantee and this proposal must
remain blocked until a different storage boundary is specified.

## Candidate dedicated columns

These are candidate fields, not blanket claims that all historical/imported rows
have sufficient provenance. Every emitted ID, revision and enum needs validation;
unproven IDs or inconsistent references fail the selected snapshot or are reported
as omitted without leaking unrelated data. Sequences and revisions use canonical
decimal strings, not JavaScript floating-point numbers.

| Entity | Candidate dedicated columns | Truthful meaning and limits |
| --- | --- | --- |
| Project | `projects.id` | Logical source identity only; `physical_identity`, `resolved_path` and project `data` are excluded. No repository is restored. |
| Draft | `drafts.id, project_id, revision`; `draft_versions.draft_id, revision` | Recorded draft/version identities only. Text, titles and policy are unavailable. |
| Task | `tasks.id, project_id, revision, state_revision, current_version`; `task_versions.task_id, version` | Recorded versions and revisions only; task instructions, guidance, acceptance conditions, current policy and explicit latest-attempt pointer are unavailable. |
| Attempt | `attempts.id, task_id, task_version, revision, phase, sequence` | Sequence is insertion order. `settled` does **not** prove success, failure, cancellation or verified cleanup; all such outcomes and evidence status remain unavailable. Do not derive an authoritative latest attempt from sequence alone. |
| Artifact reference | `artifacts.id, attempt_id, byte_length, sequence` | Recorded logical reference and length only. Exclude `private_ref`, `physical_identity`, metadata JSON and all bytes. The reference does not prove payload existence, integrity, accessibility or eligibility. Whether exposing length for an unknown-provenance object is permitted remains a policy decision. |
| Snapshot/candidate reference | `source_snapshots.id, attempt_id, manifest_artifact_id, index_artifact_id`; `candidates.id, attempt_id, source_snapshot_id, manifest_artifact_id, delta_artifact_id` | Logical associations only; source base, paths, file material, candidate content and verification state are unavailable. `workspace_ref` is excluded. |
| Group | `task_graphs.id, project_id, root_task_id, graph_version, control_revision, state_revision, budget_epoch, max_parallelism, state, root_input_id, final_node_id` | Historical graph identity, recorded state and root/node references; no active dispatch authority. Exclude owner/generation/dispatch JSON. |
| Group member | `graph_nodes.id, graph_id, task_id, ordinal, control_revision, state_revision, native_debits, automatic_debits, current_input_id, node_admission_id, state` | Membership, recorded ordering/state and Council accounting; exclude `node_key` and `data`. Ordinal is **not** a dependency edge. |
| Group input/dispatch | `graph_inputs.id, graph_id, graph_version, node_id, operation_attempt_id, root_input_id, kind, state`; `graph_dispatches.id, graph_id, graph_version, node_id, task_id, task_version, input_id, attempt_id, workflow_id, step_id, node_admission_id, execution_ordinal` | Recorded identity and stage links. No materialized bytes, arbitrary `logical_slot`, owner identity, budget authority, proofs or output bodies. |
| Workflow | `workflows.id, task_id, task_version, revision, state_revision, state`; `workflow_steps.id, workflow_id, ordinal, execution_ordinal, role, executor_kind, cycle, status, attempt_id, input_candidate_id, input_response_artifact_id, predecessor_step_id, replacement_of_step_id` | Historical step lineage and recorded status only. Original task scope, check definitions, review findings and validity of evidence cannot be reconstructed. Exclude `logical_key`, authority JSON and free text. |
| Acceptance reference | `response_acceptances.id, task_id, task_version, attempt_id, response_artifact_id`; optionally `workflow_acceptances.id, workflow_id, task_id, task_version, candidate_id` after provenance and scope checks | A stored relationship exists; it does not establish that unavailable payload bytes or checks are valid. Do not present it as fresh verification or current delivery authority. |
| Checkpoint reference | `recovery_checkpoints.id, scope_kind, task_id, graph_id, node_id, revision, state` | Historical recorded scope/state only; checkpoint `data`, original authority and owner are omitted. Imported references cannot resume or release a fence. |
| Council budget history, optional | `task_budgets.task_id, revision, granted, consumed, automatic, historical, window_number` | Recorded Council accounting only, not native provider usage, remaining quota or current account availability. Never import as active authority. |

Even structurally valid metadata can be sensitive. The selection UI and manifest
must disclose these categories and require an explicit destination. A generated
ID or enum is not an authorization to export the entire record around it.

## Scoped joins and validation

Use parameterized queries from the selected project identity, preferably using
project-scoped IDs and indexed joins. Illustrative projections, omitting error
handling and explicit limits, are:

```sql
SELECT a.id, a.task_id, a.task_version, a.revision, a.phase, a.sequence
FROM attempts AS a JOIN tasks AS t ON t.id = a.task_id
WHERE t.project_id = ? ORDER BY a.sequence;

SELECT n.id, n.graph_id, n.task_id, n.ordinal, n.control_revision,
       n.state_revision, n.native_debits, n.automatic_debits, n.state
FROM graph_nodes AS n JOIN task_graphs AS g ON g.id = n.graph_id
JOIN tasks AS t ON t.id = n.task_id
WHERE g.project_id = ? AND t.project_id = ? ORDER BY g.sequence, n.ordinal;

SELECT a.id, a.attempt_id, a.byte_length, a.sequence
FROM artifacts AS a JOIN attempts AS x ON x.id = a.attempt_id
JOIN tasks AS t ON t.id = x.task_id
WHERE t.project_id = ? ORDER BY a.sequence;
```

The same pattern scopes drafts and tasks directly by `project_id`, graph inputs
and dispatches through `task_graphs`, and workflows/steps through `tasks`. A
checkpoint with a task scope is associated through the selected task; a graph
scope through its selected graph; a member must match **both** the selected task
and graph and their exact node membership. Validate these associations and every
included cross-reference before publication. A corrupt, missing, cross-project or
unavailable relation must not be silently filtered while claiming complete
metadata. Detect it without returning unrelated project values. Global incidents,
claims, account reports and owner epochs do not belong to one selected project
and are excluded unless a later separately approved model defines safe scope.

No standalone dependency-edge table exists in the accepted graph schema. Graph
definitions and dependency kinds are held in opaque graph-version data; this
profile must explicitly mark dependency topology and conditions unavailable.
Likewise attempt outcome, earlier failure, cancellation precedence, cleanup
proof, source content, review findings, check results, applied effects and
native usage cannot be reconstructed reliably from the listed columns. Missing
information must remain `unavailable`, not a default success, empty dependency
list, zero usage or inferred latest record.

## Snapshot, bounds and observation API

The likely seam is a new Store-owned projection function using `s.mu` and a single
SQLite read transaction over explicitly selected columns. The current Store uses
one connection; avoid calling lock-taking public Store methods while holding the
snapshot or transaction. Enumerate a bounded selection under one transaction,
copy only allowed typed values into bounded memory, end the transaction, then
serialize and stage the manifest. A long archive write must not hold the Store
mutex. Validate all counters as exact signed 64-bit values, apply explicit record,
reference, memory and byte limits, and fail/report incomplete if any limit or
read fails.

Pagination requires special care: independently fetching pages after the read
transaction ends is **not** a coherent snapshot merely because each page has a
cursor or a high-water mark. Either materialize the entire bounded selection in
one read snapshot, or define a genuinely retained snapshot with clear ownership,
lifetime and invalidation. Never present a multi-transaction mixed revision as a
single complete history. A production limit that prevents selection must be
visible and must not silently drop rows.

As a research-only API separation, a future UI/CLI could expose distinct
`metadata-history preview/create/inspect/import` operations and a read-only
history view. Its manifest should explicitly state `profile: metadata_history`,
`completeness: incomplete`, source project identity, schema/format version,
included categories, referenced unavailable content and exclusions. A completed
**operation** only means that this declared projection was written or imported;
it does not mean a complete project backup. Keep this endpoint and archive type
separate from complete-backup verification/restore, and do not manufacture live
task, attempt, checkpoint, workflow, budget, receipt, owner or dispatch authority
on import. UI and CLI must expose the same source identities, omissions and
read-only limitation. Source-intake and destination safeguards remain separately
unresolved in [source-intake.md](source-intake.md).

## Meaningful offline evidence

1. Create two disposable real-owner projects with task, graph, workflow,
   checkpoint and artifact references. Put fake sentinel strings in opaque JSON
   and fake artifact payloads. Instrument the projection and file I/O seams to
   prove the application does not select/decode opaque bodies or open/hash/copy
   those payloads; confirm the selected project's allowed fields and relationships
   are present and unrelated project records are absent. Hook evidence is limited
   to the mediated operations it actually observes.
2. Include values beyond `2^53`, empty sets, duplicate/tampered logical IDs,
   inconsistent cross-project links and limit overflow. Verify exact decimal
   encoding, non-leaking rejection and no silent loss or fabricated defaults.
3. Create a graph with actual dependency edges and completed/failed attempts;
   assert that the projection explicitly labels dependency topology, outcomes,
   checks and unavailable payloads as unknown, instead of inferring them from
   order, phase or record existence. Verify reported budget counters are Council
   accounting and never native usage measurements.
4. Exercise concurrent mutation at capture/page boundaries and an actual owner
   restart, showing one coherent selected snapshot or explicit failure. Import
   the projection to an isolated read-only namespace and verify inspection in UI
   and CLI, unchanged unrelated projects and zero automatic worker/model calls.
   This acceptance proves the incomplete-history profile only, not a complete
   production backup or unrestricted portable restore.
