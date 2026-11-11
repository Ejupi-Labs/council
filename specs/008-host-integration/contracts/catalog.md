# MCP tool catalog and owner mapping

**Status: implemented; accepted for the documented offline scope** (see
[acceptance.md](../acceptance.md) for evidence and limits; this contract is not itself
acceptance evidence). It began as a design draft. This is the closed 008 allowlist, audited against the
current owner routes in `internal/httpapi/{server,execution,routing,workflow,graphs,recovery}.go`
and their application/domain validators. Names below are the MCP tool names implemented
in `internal/mcphost/catalog.go`. The `/api/v1` prefix is implicit in the
owner route column. No tool accepts an arbitrary URL, HTTP method, filesystem
command, bearer token, native process handle or caller-supplied proof of cleanup.

## Argument and response conventions

- Each table row is one distinct tool. `id`, `taskId`, `graphId`, `nodeKey`,
  `inputId`, `dispatchId`, `snapshotId` and `operationId` are explicit named
  outer arguments interpolated into exactly the shown path after existing
  validation. `nodeKey` is the graph declaration key in the URL; a separate
  `nodeId` in a mutation body is the exact captured node identity.
- For a mutation, the only additional outer argument is `input`: the **complete
  exact owner JSON request object** named in the row. Its `requestId` is mandatory,
  a 32-lowercase-hex Council identity distinct from the MCP JSON-RPC ID. IDs in
  body and path must satisfy the current validators. Optional fields retain
  omitted/null/empty/false distinctions accepted by the owner; unknown or
  duplicate members are rejected, never normalized away. The existing DTO and
  validation source named below is normative for every field, enum and bound;
  schemas generated for `tools/list` must enumerate those exact fields rather
  than allowing an untyped object. `input` is forwarded as the owner body and
  the outer path identity participates in the request fingerprint as it does
  today. No other outer keys are accepted.
- For a GET, only the named path and query arguments in the row are allowed;
  there is no body or mutation identity. Query values retain the owner spelling
  and omission semantics. Decimal revisions, counts, offsets and cursors are
  strings, never floating-point JSON numbers. List `limit` and `cursor` follow
  the existing route's bounds and opaque cursor rules; wait `timeoutMs` is the
  existing bounded observer timeout. Invalid path separators, unknown queries,
  duplicate keys, empty-present query values and out-of-range numbers fail before
  forwarding. A colon in this catalog denotes a named argument, not literal text.
- Exact owner `status` and `envelope` are returned through structured content and
  an equivalent text representation; domain errors set `isError`. Never infer a
  completed task from a successful admission or wait response. Owner-local bearer
  material is never part of arguments, results or diagnostics.
- The host supplies complete captured graph/node/budget/checkpoint authority in
  the listed `input`. It retains that unchanged body and request identity for
  retry. Do not perform the CLI's optional automatic proof capture/rebind on a
  retry or substitute a newer revision. A client may explicitly inspect first
  and create a new request with newly selected authority.

## Workspace and owner

| MCP tool | Existing owner operation | Method and route | Strict argument source |
| --- | --- | --- | --- |
| `council_owner_meta` | `meta` | GET `/meta` | no arguments; disclose capability metadata, never descriptor token |
| `council_projects_open` | `projects.open` | POST `/projects/open` | `input {requestId,selectedPath,basePath}`; basePath is absolute; `internal/app/workspace.go` |
| `council_projects_list` | `projects.list` | GET `/projects` | optional `limit,cursor` |
| `council_projects_get` | `projects.get` | GET `/projects/:id` | `id` |
| `council_policy_get` | `policy.get` | GET `/policy` | no arguments |
| `council_policy_set` | `policy.set` | PUT `/policy` | `input {requestId,expectedRevision,mode}`; `internal/app/workspace.go` |
| `council_agents_list` | `agents.list` | GET `/agents` | no arguments; every call is the same fresh inspection as the CLI refresh; current discovery is observational |
| `council_drafts_create` | `drafts.create` | POST `/drafts` | `input {requestId,projectId,originalText}`; `internal/app/drafts.go` |
| `council_drafts_list` | `drafts.list` | GET `/drafts` | optional `projectId,limit,cursor` |
| `council_drafts_get` | `drafts.get` | GET `/drafts/:id` | `id`, optional `revision` |
| `council_drafts_update` | `drafts.update` | PUT `/drafts/:id` | `id`; `input {requestId,expectedRevision,originalText,useCurrentPolicy?}` |
| `council_service_stop` | `service.stop` | POST `/service/stop` | `input {requestId,ownerInstanceId}`; exact captured owner; recognized maintenance contract only |

The service-stop row also supports an already-running, recognized older
maintenance-compatible owner. Negotiate its separate `owner-stop/1` contract
using the maintenance-compatible metadata rules in `internal/cli/metadata.go`;
refuse new execution/history against that older owner, never replace it, acquire
its state directly or erase its lock. An unknown owner cannot be stopped through
an inferred identity. `council --help`, `--version` and launching the local UI
are local CLI presentation/entrypoint commands, not invented owner APIs or
permission to open a browser from an MCP tool.

## Task execution, waiting and evidence

Mutation DTOs in this section are defined in `internal/domain/execution.go` and
validated by `internal/domain/execution_validation.go` and `internal/app/execution.go`.

| MCP tool | Existing owner operation | Method and route | Strict argument source |
| --- | --- | --- | --- |
| `council_execution_capabilities` | `execution.capabilities` | GET `/execution/capabilities` | no arguments |
| `council_tasks_start` | `tasks.start` | POST `/tasks` | `input StartTaskInput`; originalText/project required, accepted Auto/default and optional workflow/routing/pins retained |
| `council_tasks_list` | `tasks.list` | GET `/tasks` | optional `projectId,limit,cursor` |
| `council_tasks_get` | `tasks.get` | GET `/tasks/:taskId` | `taskId` |
| `council_tasks_version_get` | `tasks.version.get` | GET `/tasks/:taskId/versions/:version` | `taskId,version` |
| `council_tasks_guidance` | `tasks.guidance` | PUT `/tasks/:taskId/guidance` | `taskId`, `input GuidanceInput` |
| `council_tasks_attempts_start` | `tasks.attempts.start` | POST `/tasks/:taskId/attempts` | `taskId`, `input StartAttemptInput` |
| `council_tasks_attempts_list` | `tasks.attempts.list` | GET `/tasks/:taskId/attempts` | `taskId`, optional `limit,cursor` |
| `council_tasks_response_accept` | `tasks.response.accept` | POST `/tasks/:taskId/response-acceptance` | `taskId`, `input ResponseAcceptanceInput`, including explicit confirmation, exact artifact and digest |
| `council_attempts_get` | `attempts.get` | GET `/attempts/:id` | `id` |
| `council_attempts_stop` | `attempts.stop` | POST `/attempts/:id/stop` | `id`, `input {requestId}` |
| `council_attempts_wait` | `attempts.wait` | GET `/attempts/:id/wait` | `id`, optional `afterRevision,timeoutMs` |
| `council_attempts_events` | `attempts.events.list` | GET `/attempts/:id/events` | `id`, optional `limit,cursor` |
| `council_attempts_artifacts` | `attempts.artifacts.list` | GET `/attempts/:id/artifacts` | `id`, optional `limit,cursor` |
| `council_artifacts_get` | `artifacts.get` | GET `/artifacts/:id` | `id` |
| `council_artifacts_bytes` | `artifacts.bytes` | GET `/artifacts/:id/bytes` | `id`, optional `offset,through,limit`; bytes are base64, not decoded text |

An artifact page retains the owner's `offset`, `nextOffset`, `through`,
`bytesBase64`, `eof`, `streamClosed`, completeness and digest. Capture the first
page's `through` and pass it unchanged on later pages; each read is at most 65,536
bytes. A later end boundary is a new explicit read. MCP cancellation or stdio
disconnect cancels only the wait/read context, never an implicit Stop; mutations
with lost replies remain uncertain with their original request ID.

## Routing and capacity

Body DTOs and validators are in `internal/domain/routing.go` and
`internal/domain/routing_validation.go`; task adoption and a grant are the
existing `tasks.budget.amend` action variants, not separate invented routes.

| MCP tool | Existing owner operation | Method and route | Strict argument source |
| --- | --- | --- | --- |
| `council_routing_policy_get` | `routing.policy.get` | GET `/routing/policy` | no arguments |
| `council_routing_policy_update` | `routing.policy.update` | PUT `/routing/policy` | `input RoutingPolicyInput` |
| `council_routing_contexts_list` | `routing.contexts.list` | GET `/routing/contexts` | no arguments |
| `council_routing_context_update` | `routing.context.update` | PUT `/routing/contexts/:id` | context `id`, `input ContextUpdateInput` |
| `council_routing_profiles_list` | `routing.profiles.list` | GET `/routing/profiles` | optional `contextId` |
| `council_routing_profile_update` | `routing.profile.update` | PUT `/routing/profiles/:id` | profile `id`, `input ProfileUpdateInput` |
| `council_routing_accounts_list` | `routing.accounts.list` | GET `/routing/accounts` | optional `limit,cursor` |
| `council_routing_accounts_create` | `routing.accounts.create` | POST `/routing/accounts` | `input AccountCreateInput` |
| `council_routing_accounts_update` | `routing.accounts.update` | PUT `/routing/accounts/:id` | account `id`, `input AccountUpdateInput` |
| `council_routing_links_update` | `routing.links.update` | PUT `/routing/contexts/:id/account-link` | context `id`, `input AccountLinkInput` |
| `council_routing_capacity_get` | `routing.capacity.get` | GET `/routing/capacity` | exactly one of `contextId,accountId` |
| `council_routing_reports_list` | `routing.reports.list` | GET `/routing/reports` | exactly one of `contextId,accountId`, optional `limit,cursor` |
| `council_routing_reports_get` | `routing.reports.get` | GET `/routing/reports/:id` | report `id` |
| `council_routing_reports_create` | `routing.reports.create` | POST `/routing/reports` | `input CapacityReportInput` |
| `council_tasks_routing_get` | `tasks.routing.get` | GET `/tasks/:taskId/routing` | `taskId` |
| `council_tasks_routing_decisions` | `tasks.routing.decisions.list` | GET `/tasks/:taskId/routing/decisions` | `taskId`, optional `limit,cursor` |
| `council_tasks_budget_get` | `tasks.budget.get` | GET `/tasks/:taskId/budget` | `taskId` |
| `council_tasks_budget_ledger` | `tasks.budget.ledger.list` | GET `/tasks/:taskId/budget/ledger` | `taskId`, optional `limit,cursor` |
| `council_tasks_budget_amend` | `tasks.budget.amend` | POST `/tasks/:taskId/budget-amendments` | `taskId`, `input BudgetAmendmentInput`; accepted grant/adopt action and exact revisions |
| `council_tasks_usage` | `tasks.usage.list` | GET `/tasks/:taskId/usage` | `taskId`, optional `limit,cursor`; preserve unknown telemetry |

## Verified workflow and delivery

Workflow DTOs and validation are in `internal/domain/workflow.go` and
`internal/domain/workflow_validation.go`. `workflowId` and `taskVersion` are
optional owner selectors subject to the existing mutual-consistency rules.

| MCP tool | Existing owner operation | Method and route | Strict argument source |
| --- | --- | --- | --- |
| `council_workflow_preferences_get` | `workflow.preferences.get` | GET `/workflow/preferences` | no arguments |
| `council_workflow_preferences_update` | `workflow.preferences.update` | PATCH `/workflow/preferences` | `input DeliveryPreferenceInput` |
| `council_tasks_workflow_get` | `tasks.workflow.get` | GET `/tasks/:taskId/workflow` | `taskId`, optional `workflowId,taskVersion` |
| `council_tasks_workflow_steps` | `tasks.workflow.steps.list` | GET `/tasks/:taskId/workflow/steps` | `taskId`, optional `workflowId,taskVersion,limit,cursor` |
| `council_tasks_workflow_reviews` | `tasks.workflow.reviews.list` | GET `/tasks/:taskId/workflow/reviews` | same selector/page arguments |
| `council_tasks_workflow_checks` | `tasks.workflow.checks.list` | GET `/tasks/:taskId/workflow/checks` | same selector/page arguments |
| `council_tasks_workflow_findings` | `tasks.workflow.findings.list` | GET `/tasks/:taskId/workflow/findings` | same selector/page arguments |
| `council_tasks_workflow_deliveries` | `tasks.workflow.deliveries.list` | GET `/tasks/:taskId/workflow/deliveries` | same selector/page arguments |
| `council_tasks_workflow_candidates` | `tasks.workflow.candidates.list` | GET `/tasks/:taskId/workflow/candidates` | same selector/page arguments |
| `council_candidates_changes` | `candidates.changes.list` | GET `/candidates/:id/changes` | candidate `id`, optional `limit,cursor` |
| `council_tasks_workflow_adopt` | `tasks.workflow.adopt` | POST `/tasks/:taskId/workflow/adopt` | `taskId`, `input WorkflowAdoptInput` |
| `council_tasks_workflow_advance` | `tasks.workflow.advance` | POST `/tasks/:taskId/workflow/advance` | `taskId`, `input WorkflowAdvanceInput` |
| `council_tasks_workflow_check` | `tasks.workflow.check` | POST `/tasks/:taskId/workflow/check` | `taskId`, `input WorkflowCheckInput` |

## Graphs and dependencies

Body types are in `internal/domain/graph_operations.go`, with constraints in
`internal/domain/graph_validation.go`. Start takes the bounded complete declaration;
advance and controls require the complete captured revisions/authorities, not an
unreviewed fresh read hidden by the MCP client.

| MCP tool | Existing owner operation | Method and route | Strict argument source |
| --- | --- | --- | --- |
| `council_graphs_start` | `graphs.start` | POST `/graphs` | `input GraphStartInput`, including exact node/dependency declaration |
| `council_graphs_list` | `graphs.list` | GET `/graphs` | optional `projectId,limit,cursor` |
| `council_graphs_get` | `graphs.get` | GET `/graphs/:graphId` | `graphId` |
| `council_graphs_definition` | `graphs.definition.get` | GET `/graphs/:graphId/definition` | `graphId` |
| `council_graphs_wait` | `graphs.wait` | GET `/graphs/:graphId/wait` | `graphId`, optional `afterRevision,timeoutMs` |
| `council_graphs_advance` | `graphs.advance` | POST `/graphs/:graphId/advance` | `graphId`, `input GraphAdvanceInput` |
| `council_graphs_stop` | `graphs.stop` | POST `/graphs/:graphId/stop` | `graphId`, `input GraphStopInput` |
| `council_graphs_nodes_stop` | `graphs.nodes.stop` | POST `/graphs/:graphId/nodes/:nodeKey/stop` | `graphId,nodeKey`, `input GraphNodeStopInput` |
| `council_graphs_nodes_guidance` | `graphs.nodes.guidance` | POST `/graphs/:graphId/nodes/:nodeKey/guidance` | `graphId,nodeKey`, flattened `input GraphGuidanceInput` |
| `council_graphs_budget_get` | `graphs.budget.get` | GET `/graphs/:graphId/budget` | `graphId` |
| `council_graphs_budget_ledger` | `graphs.budget.ledger` | GET `/graphs/:graphId/budget-ledger` | `graphId`, optional `limit,cursor` |
| `council_graphs_budget_amend` | `graphs.budget.amend` | POST `/graphs/:graphId/budget-amendments` | `graphId`, `input GraphBudgetAmendmentInput` |
| `council_graphs_inputs_list` | `graphs.inputs.list` | GET `/graphs/:graphId/inputs` | `graphId`, optional `limit,cursor` |
| `council_graphs_inputs_get` | `graphs.inputs.get` | GET `/graphs/:graphId/inputs/:inputId` | `graphId,inputId` |
| `council_graphs_inputs_members` | `graphs.inputs.members` | GET `/graphs/:graphId/inputs/:inputId/members` | `graphId,inputId`, optional `limit,cursor` |
| `council_graphs_dispatches_list` | `graphs.dispatches.list` | GET `/graphs/:graphId/dispatches` | `graphId`, optional `inputId,nodeKey,limit,cursor` |
| `council_graphs_dispatches_get` | `graphs.dispatches.get` | GET `/graphs/:graphId/dispatches/:dispatchId` | `graphId,dispatchId` |

## Pause, checkpoint and incidents

The DTOs and exact scope rules are `internal/domain/recovery.go`. Task pause
derives a graph-member scope from the selected task's recorded membership;
there is no separate invented member-pause route.

| MCP tool | Existing owner operation | Method and route | Strict argument source |
| --- | --- | --- | --- |
| `council_tasks_pause` | `tasks.pause` | POST `/tasks/:taskId/pause` | `taskId`, `input PauseTaskInput` |
| `council_graphs_pause` | `graphs.pause` | POST `/graphs/:graphId/pause` | `graphId`, `input PauseGraphInput` |
| `council_checkpoints_get` | `checkpoints.get` | GET `/checkpoints/:id` | checkpoint `id` |
| `council_checkpoints_resume` | `checkpoints.resume` | POST `/checkpoints/:id/resume` | checkpoint `id`, `input ResumeCheckpointInput`, including captured complete `scope` |
| `council_incidents_get` | `incidents.get` | GET `/incidents/:id` | incident `id` |
| `council_incidents_reconcile` | `incidents.reconcile` | POST `/incidents/:id/reconciliations` | incident `id`, `input ReconciliationInput`; controller obtains proof |

## Incomplete metadata history

The strict request shapes are in `internal/domain/history.go`; use the owner
`metadata_history`/`007a-v1` capability before these tools. No arbitrary path or
claim of evidence backup, restore or cleanup is authorized.

| MCP tool | Existing owner operation | Method and route | Strict argument source |
| --- | --- | --- | --- |
| `council_history_capture` | `history.snapshots.create` | POST `/history/snapshots` | `input CaptureHistoryInput` |
| `council_history_snapshot_get` | `history.snapshots.get` | GET `/history/snapshots/:snapshotId` | `snapshotId` |
| `council_history_records` | `history.snapshots.records` | GET `/history/snapshots/:snapshotId/records` | `snapshotId`, required `kind`, optional `limit,cursor` |
| `council_history_preview` | `history.exports.preview` | POST `/history/exports/preview` | `input PreviewHistoryExportInput` |
| `council_history_export` | `history.exports.create` | POST `/history/exports` | `input ExecuteHistoryExportInput` |
| `council_history_operation_get` | `history.operations.get` | GET `/history/operations/:operationId` | `operationId` |
| `council_history_reconcile` | `history.operations.reconcile` | POST `/history/operations/:operationId/reconciliations` | `operationId`, `input ReconcileHistoryOperationInput` |

## Design-time audit and implementation blockers

At design time these checks were still required; the later implementation and acceptance evidence is recorded in acceptance.md.

- This draft enumerates all current fixed application routes in the source
  files named above. The SDK catalog and exact schemas are not implemented or
  validated by this document. Before implementation acceptance, a mechanical
  route-to-tool audit must fail on any missing, extra or duplicate mapping and
  exercise every listed tool against the actual owner.
- The CLI's optional graph proof capture and rebind is implemented in
  `internal/cli/graph_intents.go`, not exposed as an owner endpoint. The MCP
  contract above requires explicit exact authority; implementing a convenience
  auto-capture without durable original-intent retention would be a blocker.
- There is no incident-list or checkpoint-list owner route in the inspected
  router. The existing tools inspect identities obtained from owner outcomes,
  checkpoints or admission metadata. Do not claim an invented listing operation;
  if required for the host journey, specify and implement a separately reviewed
  owner capability before claiming parity.
- Exact response verification, capability negotiation, protocol cancellation,
  multi-host concurrency and old-owner Stop require actual tests. A mocked tool
  catalog or syntactically valid protocol alone is not acceptance evidence.
