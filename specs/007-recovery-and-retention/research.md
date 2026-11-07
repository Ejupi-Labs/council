# Research: Pause and Recovery

Date: 2026-11-07. Decisions use accepted 006 source; this document describes planned behavior, not passed tests.

## Pause is an admission boundary

**Decision**: Add durable scoped pause/checkpoint state. Commit pause under the same Store transaction and serialized owner authority as every native/local admission and continuation, including graph input and effect admission. Let already admitted work drain, retain Stop/failure precedence, and require explicit current-authority resume.  
**Rationale**: Existing Engine and graph actor paths use guarded owner operations; `Engine.Close` cancels workers and is not pause. A UI flag alone leaves races.  
**Alternatives**: Suspend arbitrary processes (unsupported across harnesses); cancel running work (wrong semantics); wait for restart (loses durable intent).

## Standalone, member and group scopes

**Decision**: Persist distinct standalone-task, graph-member and whole-graph pause scopes. A member retains its parent graph and shared-budget authority. A group pause covers all members; a member pause does not stop unrelated siblings. Overlapping checkpoints remain independent: resuming either cannot clear the other's fence.  
**Rationale**: `contracts/pause-reconciliation.md` freezes actual graph membership and scope-specific authority.  
**Alternatives**: Treat every member as a standalone task or use a single last-writer-wins pause flag; either can bypass an active parent/child constraint.

## Reconcile only actual evidence

**Decision**: Retain exact incident, claim, owner generation, process cleanup observation and effect/evidence proof; atomically release only an individually proven claim after checking every other blocker. A controlled positive case uses an actual still-held `process.Owned.Cleanup` result for the exact owned process tree and complete durable effects/evidence. Persist proof before its handle disappears. If no authoritative proof survives owner death, expose unresolved state and an inspection action without fabricating resolution.  
**Rationale**: `internal/store/incidents.go` fences globally and `RecoverOwner` never dispatches. PID reuse or a missing response cannot establish full tree cleanup.  
**Alternatives**: User assertion, timeout, guessed process scan or automatic reset can overlap an existing writer. Pretending restart reattaches an old owner is unsupported.

## Reuse owner-backed clients

**Decision**: Extend `internal/app/graphs.go` and owner routing, `internal/cli/graphs.go`/`execution.go`, and `ui/src/graphs.ts`/`tasks.ts`/`api.ts`. Mutations use existing request IDs, exact receipts and captured revision/proof. Add pause/resume beside Stop/Advance, clearly distinguishing drain from cancel; display incident, available proof, blockers and next action. Polling remains bounded and makes no model call.  
**Rationale**: Existing graph clients retain pending intents and selection generations, and the owner is the authority boundary.  
**Alternatives**: A second daemon, CLI-only controls, or UI-only admission fence weakens parity or safety.

## Migration and qualification

**Decision**: Schema 6→7 uses existing migration machinery and strict schema validation, freezing a populated accepted-v6 fixture and preserving all earlier rows and unresolved claims. Test controlled races, restarts and actual native/browser/package behavior, then run final exact-source build/test/check, all six native jobs and Linux AMD64 race.  
**Rationale**: Current Store validates exact schema and recovery relies on durable records; compile-only evidence cannot qualify the feature.  
**Alternatives**: Auto-repair unknown schemas, count fake provider activity as live, or declare tests passed without terminal evidence.

## Mandatory next feature: retention

Scoped evidence export, project backup, verified restore and reviewed cleanup remain required immediately after this feature. Existing 006 does not establish credential-free provenance for arbitrary opaque user files or free text. No complete production backup profile is established merely by a fixture, path allowlist or unverified client assertion. The next feature must resolve this gap and provide useful explicitly scoped recovery without ever inspecting/copying vendor credentials; a limited metadata-only profile must not be presented as a complete content backup or as permission to run incomplete restored work. This unresolved future design issue is not a pause/reconciliation implementation gate.
