---
name: council-model-map
description: Choose Council workers, reviewers or advisors using the dated model map and source-linked task evidence. Use when model-specific strengths or effort tradeoffs matter; this is host orchestration guidance, not a worker skill or provider benchmark runner.
---

Use `council_model_map` with relevant task and role filters to inspect model priorities and sources without unrelated model evidence. Optional `task`, `model` and `harness` filters are exact; unknown IDs remain unknown. Filtered responses preserve each matching model's complete evidence, cited sources and methodology. Request `includeFullCatalog: true` only when the wider catalog is needed. The CLI fallback is `council model-map` with the same filters and optional `--full-catalog`. Check the snapshot date and read material qualifications before recommending a model.

Benchmark scores measure the recorded cohort, version, harness and effort. Task priorities are proposed planning ordinals; tied priorities do not justify inventing a winner. Do not turn either into an IQ score, automatic vote weight or guarantee of correctness. Distinguish hosted endpoints, downloadable checkpoints and quantizations. An unresolved native default does not identify a model. Preserve the user's exact selections and verify the chosen harness can honor the requested model and native effort.

After narrowing candidates, inspect `council_billing` for their configured alias capabilities, current observations and subscription protection. Reference API prices in the map are not the user's billing or measured task cost. Unknown allowance is not unlimited. Use `council_route_plan` to preview an appropriate configured team and call bound, or set explicit room members/advisor relationships within the user's existing task authorization. The map never changes settings or overrides a blocked alias.

Prefer the lightest configuration that meets the task's acceptance criteria. Escalate for concrete uncertainty or failed verification; a focused advisor may be sufficient. Keep complete relevant evidence and avoid invented context caps. Read-only model-map inspection does not authorize live benchmark runs or unrelated provider calls. For authorized execution, use the normal Council workflow and yield through `council_wait` or `council_delegate_watch` while work runs.

This skill stays in the connected host. Delegated workers use only their configured advice/private-coordination protocols or return findings; they do not invoke Council recursively. User-maintained evidence can be supplied through `.council/model-map.json` using the [override schema](../../schemas/model-map-override.schema.json); it remains planning data and does not configure agents.
