# Implementation Plan: Dependent and Parallel Task Graphs

**Branch**: `refactor` | **Feature context**: `006-task-graphs` | **Date**: 2026-11-06
**Spec**: [spec.md](spec.md)
**Status**: Implementation active, not accepted. The immutable specification/design/task gate
is c9b8a0d63730ddb04236f9b31c7182026b062d1f, committed before any006 application edits.
All eleven design findings and final foreign consistency reviews closed before that gate.
Specify implement prerequisites resolve this absolute feature directory; requirements checklist
passes14/14, and no extension hooks are installed. Bounded implementation evidence is recorded in
implementation.md and evidence-matrix.md; feature-wide local, visual, extracted-package and native
acceptance remains open.

## Summary

Extend accepted005 workflows into bounded task graphs. A real graph-intent root task owns one
canonical budget and actual local capture/composition operations. Declared nodes remain ordinary
managed tasks with explicit graph membership. Introduce default two-worker capacity, exact
qualified dependencies, causal fan-in, receiving-workspace materialization, focused context and
a graph delivery proof around existing safe application.

Retain005 same-workflow candidate guards and single-producer handoff. New cross-task provenance
must be explicit. Original graph source/index, node-local candidate ancestry and cumulative final
delivery are different facts. Scheduling, context digests and local materialization add no model call.

## Technical Context

**Language/Version**: Go1.27.3; authored TypeScript7.0.2. Node24.18.1 is development tooling only.
**Primary Dependencies**: Existing modernc.org/sqlite1.61.1 and golang.org/x/sys0.49.0; no new
runtime dependency or UI framework. Existing development Playwright1.62.0 qualifies the UI.
**Storage**: Existing private per-user `.council/v2` SQLite/evidence store. Additive schema6 plus
a bounded migration of singleton claim storage; old records/receipts retain exact meanings.
**Testing**: Existing Go, native process/workspace, real owner/CLI/browser and extracted-native
tests through `npm test`; `npm run build`, `npm run check`, six native CI jobs and Linux race.
**Target Platform**: Windows, macOS and Linux AMD64/ARM64; no compile-only acceptance.
**Project Type**: Local single-user owner/server, CLI and embedded browser UI.
**Performance Goals**: Demonstrable default two-worker overlap within actual compatible capacity;
zero routing/observation/context-planning model calls; bounded metadata and explicit complete reads.
**Constraints**: Shell-free subprocesses, no vendor credentials/settings/probes/live calls,
exact counters, retained failures/Stop, no replay, isolated writes and explicit final-only effects.
**Scale/Scope**: One project per graph;16 nodes/64 edges; one finite shared allowance/window;
existing workspace limits and explicit context/provenance bounds. No new harness or MCP surface.

## Constitution Check

Pre-research and final contract/task constitution checks: PASS. All34 requirements are mapped;
independent design findings and cross-artifact corrections are closed. No runtime pass is implied.

| Principle | Design obligation |
| --- | --- |
| Useful outcomes | Actual overlapping workers, materialized accepted files and accurate final delivery; no simulated completion. |
| Proportionate usage | One canonical graph budget; no fresh child grants; deterministic scheduling/digests; existing suitable-target selection. |
| Honest interoperability | Reuse declared Codex/Claude capabilities; explicit access/context limits and unknown telemetry. |
| Scoped autonomy | Saved full access remains effective; minimal optional group composer; intermediate effects forbidden and final scope captured. |
| Evidence/no replay | Explicit graph input/delivery bindings, full closure and immutable receipts; controls/faults fence before new effects. |
| Simple approved UI | Standalone Auto unchanged; existing Ejupi Labs assets, ordered cards, English,360px and real keyboard/image qualification. |
| Sequential delivery |005 accepted at794f080/c5b1801; this spec independently reviewed; design/tasks must commit before006 code;007 stays closed until acceptance. |

No justified constitutional exception is requested. Unknown provider context/token/charge facts
will not be invented to make an acceptance gate pass.

## Project Structure

### Documentation (this feature)

```text
specs/006-task-graphs/
  spec.md
  checklists/requirements.md
  spec-review.md
  plan.md
  research.md
  data-model.md
  contracts/graphs.md
  contracts/operations.md
  contracts/materialization.md
  contracts/interface.md
  quickstart.md
  tasks.md
  design-review.md
  analysis.md
  review.md
  acceptance.md
  evidence-matrix.md
```

The last three implementation/acceptance records are created during their actual phase; planned
filenames here do not claim those checks exist or have passed.

### Source Code (repository root)

```text
internal/domain/graph*.go
internal/store/graph*.go
internal/engine/graph*.go
internal/workspace/graph*.go
internal/app/graphs.go
internal/httpapi/graphs.go
internal/cli/graphs*.go
ui/src/graphs*.ts
ui/tests/graphs*.test.mjs
scripts/package-graphs.mjs
internal/webui/dist/
```

Extend existing central budget/claim/control/routing/result files where their invariants apply
to all entrypoints. Do not add a parallel unchecked executor or duplicate provider client.

**Structure Decision**: Keep accepted Go owner/SQLite/TypeScript architecture. Feature-specific
modules expose explicit graph proofs; existing standalone DTOs and raw receipts remain compatible.

## Delivery Sequence

1. Resolve source-backed research and freeze domain/store/workspace/client contracts.
2. Define additive schema and exact legacy migration/control compatibility.
3. Establish canonical budget authority, bounded concurrent claims and per-attempt active ownership.
4. Implement root capture, causal input composition/closure and real receiving materialization.
5. Connect graph admission/progression/controls to accepted node workflows and final delivery.
6. Implement actual UI/CLI, context inspection and extracted offline journeys.
7. Independently review/fix, render/inspect, run full local gates and exact-source seven-job CI.
8. Close all24 GRA/ten SC and parent mappings; commit acceptance before007.

Dependency-ordered task details belong to the tasks phase after contracts stabilize.

## Complexity Tracking

No new process service, model planner, framework or distributed scheduler is needed. A graph
root task is real user intent/evidence ownership, not an extra native worker or synthetic authored
candidate. The new provenance and budget-authority records address specific concurrency and
cross-task integrity requirements; their exact contracts are defined by the research/data model.
