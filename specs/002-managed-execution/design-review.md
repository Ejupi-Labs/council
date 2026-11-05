# Managed execution design gate

Date: 2026-11-05. Branch: refactor. Status: clear for implementation after this design commit.

Specify specification, plan, research, data model, contract, quickstart and task generation ran in
order. check-prerequisites -Json -RequireTasks -IncludeTasks located this feature independently of
the Git branch. Specification checklist16/16 complete; all48implementation tasks remain unchecked.

Independent read-only review covered the constitution and all active artifacts:29EXE requirements
and9success criteria have task coverage (38/38); no unmapped task or unresolved Critical/High/Medium
finding. The ignored detailed working report is .council/refactor/reviews/002-design-gate.md.

| Finding | Correction | Closure |
| --- | --- | --- |
| Spec S1, Medium | Capture a finite default execution deadline without a mandatory form; design chooses30minutes including queue/preparation, with explicit optional bounded override. Byte/time bounds are not token/charge guarantees. | Reflected in EXE-001, model, contract, research, plan and tests/tasks. |
| Spec S2, Medium | Distinguish unsupported admitted authoring capability from explicitly pending downstream verification; never silently weaken a final-only request. | EXE-004 and acceptance/allowCandidatePreparation contract are explicit. |
| DG-002-01, Medium | Original source-index archive alone loses meaningful worker staged/unstaged semantics. | ADR07v2, model, contract, plan and tests require safe staged-index reconstruction plus exact current file bytes; index-only authored changes remain visible. Independently rechecked. |
| DG-002-02, Low | Research still described001validation as pending. | Records closed foundation gate at93c8c25 and all seven passing jobs in run42115752948. Independently rechecked. |

The optional response-only manual acceptance is a bounded preview path, not a permanent per-answer
confirmation requirement. Write candidates remain verification pending until005. Native ownership,
Stop/cleanup/failure precedence, persistence fences and restart without replay are required now.

No002application implementation or fake/live qualification is claimed by this record. All native
process, source/evidence, browser, fault and package checks must run against the implementation;
independent implementation review/corrections and acceptance remain required before003.
