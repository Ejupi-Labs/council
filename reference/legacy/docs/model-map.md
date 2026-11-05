# Model map

Council's Model map is a dated planning reference for choosing a model, task and collaboration role. The 2026-11-05 snapshot contains 24 exact profiles across OpenAI, Anthropic, Google, DeepSeek, Qwen, Kimi and GLM. It combines source-linked measurements, provider guidance and explicitly labeled policy inferences.

Reading the map makes no model calls or account probes. It does not select a running model, change room voting weights, grant permissions, configure billing, or introduce context caps. Existing explicit user choices remain authoritative.

## Open and filter

Open **Model map** in the dashboard. Choose a task, exact model ID/alias and role, then **Apply filters**. **Clear filters** restores the complete reference; **Reload reference** rereads local overrides. Priorities start collapsed by task; selecting an exact task opens its group. The local dashboard requests the full catalogue to keep every filter choice available; this HTTP read makes no model calls and consumes no provider tokens. Complete results remain available through expandable profiles, cohorts and the source register.

The optional harness filter matches explicit profile compatibility declarations. An evaluator's harness label does not prove installed compatibility. When no profile declares compatibility, the dashboard disables this filter and shows **No compatibility declarations**. Benchmark harnesses remain visible in the evidence.

MCP clients use the read-only `council_model_map` tool:

```json
{"task":"repository-coding","role":"worker"}
```

Inspect an exact configuration's reference separately:

```json
{"model":"claude-sonnet-5-5"}
```

CLI examples:

```powershell
council model-map --workspace .
council model-map --workspace . --task frontend-design --role reviewer
council model-map --workspace . --model qwen3.8-max-2026-09-02
council model-map --workspace . --task frontend-design --full-catalog
```

The authenticated dashboard API is `GET /api/model-map`, with optional `task`, `model`, `role`, `harness` and `includeFullCatalog=true|false` query parameters. Filters use exact, case-sensitive identities. An unknown model or task produces a warning and no guessed replacement. By default, filtered MCP/CLI/API responses include matching catalogue profiles, all their related evidence and cited/methodology sources, plus all task definitions. This avoids spending orchestrator context on unrelated evidence while retaining complete selected evidence without clipping. Set MCP `includeFullCatalog: true`, HTTP `includeFullCatalog=true`, or CLI `--full-catalog` to retain the full catalogue alongside filtered `models`, `hierarchy` and `rankings`. `bindings` describe all configured aliases.

## Read the hierarchy correctly

**Task and role priorities** are planning ordinals: a lower number is preferred within the same task and role. Several models can share a priority. Alphabetical display order does not break a tie. A priority is neither IQ nor a voting weight; an advisor's priority cannot be compared directly with a worker's priority. Each recommendation records its basis, confidence, reason and sources.

**Measured benchmark cohorts** show observed point estimates and ranks computed within the observations recorded in this catalog. These may cover only part of a source leaderboard; a catalog cohort rank is not the owner's global rank or a claim of statistical significance. Filtering preserves the original catalog cohort ranks, so visible ranks may have gaps. Compare the named dataset version, metric, harness, effort, date and uncertainty. Preference ratings measure human preference; task pass rates measure that benchmark's acceptance conditions. A composite index measures its versioned mixture of evaluations. Council never averages different cohorts into a universal score. Overlapping intervals and tied point estimates do not establish a winner.

Start bounded edits or extraction with a validated economy worker such as Luna, Haiku or Flash-Lite. Repository worker priority 1 includes Sol 6.1, Sonnet and Opus as alternatives. Visual analysis favors specialist roles for Terra and Argon. Difficult planning can use Astra or Opus as leads, with a focused Fable or other independent advisor. These are starting policies to validate, not automatic dispatch rules. Escalate on concrete ambiguity or failed acceptance checks, and let an orchestrator wait server-side while workers execute.

## Model-by-model starting points

These concise perks summarize the catalogue's proposed uses. Open the profile for evidence and limitations before assigning authority.

| Exact profile ID | Useful starting role / distinguishing consideration |
| --- | --- |
| `gpt-6.1-sol` | Complex repository worker; focused technical advisor; balanced implementation candidate. |
| `gpt-6-astra` | Ambiguous architecture, consequential adjudication and difficult proactive investigation. |
| `gpt-6.1-terra` | Visual/spatial worker, screenshot review and clear prose; validate implementation separately. |
| `gpt-6-luna` | Economy worker for specified edits, extraction and classification with mechanical checks. |
| `gpt-6-sol` | Previous generation; retain explicitly validated workflows and compare before migration. |
| `claude-opus-5-5` | Demanding frontend/architecture lead and writing reviewer; preference is not correctness. |
| `claude-sonnet-5-5` | Balanced code worker and independent reviewer; preserve the observed effort configuration. |
| `claude-fable-5-1` | Sustained-reasoning or architecture advisor; not automatically superior on every task. |
| `claude-haiku-5-5` | Scoped high-volume worker; verify fidelity and total completion cost. |
| `gemini-4-argon` | Writing reviewer and multimodal/reasoning advisor; writing intervals overlap Opus. |
| `gemini-3.8-flash` | Repository or writing worker; preliminary preference evidence needs local validation. |
| `gemini-3.5-flash-lite` | Throughput-oriented extraction/triage; avoid assuming autonomous audit capability. |
| `gemini-3.6-flash` | Previous generation still available; keep existing validated selections. |
| `deepseek-flash` | Cost-sensitive hosted implementation/review candidate; alias targeted 1017 at retrieval. |
| `qwen3.8-max-0902` | Dated hosted frontend/implementation alternative; exact alias also documented. |
| `qwen3.8-max` | Mutable hosted Max offering; do not inherit a pinned snapshot's measurements. |
| `qwen3.8-flash` | Production hosted Flash candidate; distinguish it from downloadable Flash Next. |
| `Qwen/Qwen3.8-2.4T-A95B` | Downloadable Max-class candidate; custom license and unverified deployment feasibility. |
| `Qwen/Qwen3.8-Flash-Next` | Downloadable multimodal/code candidate; vendor best-harness results need qualification. |
| `Qwen/Qwen3.8-27B` | Smaller Apache-2.0 option for scoped trials; size alone does not establish fit. |
| `kimi-k3.1-pro` | Complex implementation/advisor candidate; Kimi Code uses a different identifier namespace. |
| `kimi-k3` | Previous generation; preserve validated work without inheriting Pro scores. |
| `glm-5.3` | Independent text/code worker or reviewer; custom checkpoint license. |
| `glm-5.3-flash` | Scoped multimodal worker; related MIT weights differ from hosted execution. |

The `legacy` status means previous generation, not unavailable. Hosted API, subscription harness, downloaded weights and third-party quantizations are different execution targets. Open weights establish neither unrestricted licensing nor feasible local deployment. A native-default alias has an unresolved model identity; an exact configured match still does not prove the provider honored it. Check effective model, accepted effort, harness/version and permission in real execution evidence. Council native Claude does not accept `xhigh`; a benchmark's `xhigh` cannot silently become native `max`. Kimi Code documents fallback behavior for invalid IDs, making effective identity especially important.

## Local overrides

Put a regular JSON file at `model-map.json` inside the configured Council state directory, normally `.council/model-map.json`. The state directory must remain inside the workspace. Symbolic-link overrides and invalid/ambiguous references are rejected. Overrides stay local under `.council/`. All-store SQLite snapshots cover database records and do not include this JSON overlay; back it up and restore it separately alongside the workspace configuration.

Each optional array—`sources`, `observations`, `models`, `tasks`—replaces complete records with matching IDs or appends new IDs. Omitted records remain; fields and nested arrays are not deep-merged, and an empty array does not delete built-in records. A model's recommendation array is part of that complete replacement record. Exact aliases must remain unique.

Bundled observation IDs identify the recorded cohort, exact model and reported/unreported effort; they do not depend on row order or score. Copy IDs literally from the current catalogue rather than constructing them. Early development drafts used positional observation IDs: refresh any draft override's observation IDs and model `evidenceIds` from the current catalogue before reusing it. Council does not guess aliases for old evidence IDs. A changed evaluation protocol or distinct run needs its own cohort identity.

For example, append a task category:

```json
{
  "tasks": [
    {
      "id": "release-notes",
      "label": "Release notes",
      "description": "Draft release notes from verified changes for a named audience."
    }
  ]
}
```

This defines a category without inventing model competence. To change a priority, obtain the full reference with `council model-map`, copy the complete target record from `catalog.models` into the override's `models` array, and edit its recommendations. Preserve evidence IDs, limitations and all desired recommendations. Add sources/observations for new measurements, with a distinct cohort when protocol or serving target changes. Mark unsupported trial identities `unverified`; they remain visible but are excluded from the hierarchy.

Schemas: [catalogue](../schemas/model-map.schema.json), [local override](../schemas/model-map-override.schema.json), [query](../schemas/model-map-query.schema.json). Invalid overrides fail visibly rather than silently restoring default priorities. Snapshot `asOf` and revision describe the bundled reference; new source retrieval dates remain explicit in override records.

## Research and refresh

Detailed evidence and caveats: [OpenAI](research-models-openai-2026-11-05.md), [Anthropic/Google](research-models-anthropic-google-2026-11-05.md), [DeepSeek/Qwen](research-models-deepseek-qwen-2026-11-05.md), [Kimi/GLM](research-models-kimi-glm-2026-11-05.md). Primary starting points include [Claude's model overview](https://platform.claude.com/docs/en/models/overview), [Google's API catalogue](https://aistudio.google.com/docs/models), [Arena WebDev](https://arena.ai/leaderboard/code/webdev) and [Artificial Analysis methodology](https://artificialanalysis.ai/methodology/intelligence-benchmarking).

Refresh source dates and exact identities before changing policy. Compare representative tasks using acceptance outcomes, regressions, useful evidence, retries, complete token receipts where available, elapsed time and human repair work. Keep dated reference API prices separate from actual account billing and subscription allowance.
