# OpenAI model research for Council — 5 November 2026

This is a dated research snapshot, not a runtime model resolver or a universal intelligence ranking. **Documented** means current official OpenAI specifications or positioning; **measured** identifies a particular evaluation; **proposal** is a Council task-assignment inference. No live model calls, account probes or credential inspection were performed.

## Verified models and practical roles

Prices below are published Standard API input/output prices per million tokens, before processing-mode, cache, regional, tool or long-context adjustments. They are reference prices, not the user's subscription billing or a forecast of cost per completed task.

| Exact model ID | Documented emphasis | Supported API effort | API input / output | Proposed Council role |
| --- | --- | --- | --- | --- |
| `gpt-6-astra` | Difficult reasoning, coding, research, computer use and documents | `low`, `medium`, `high`, `xhigh`, `max` | $10 / $50 | Escalation advisor, ambiguous architecture, difficult investigation, final review of consequential unresolved disagreement. [Official model page](https://developers.openai.com/api/docs/models/gpt-6-astra) |
| `gpt-6.1-sol` | Complex coding, computer use and professional work at lower token prices than Astra | `low`, `medium` default, `high`, `xhigh`, `max` | $2 / $10 | General implementation lead, repeated audits, coordination with substantive technical judgment. [Official model page](https://developers.openai.com/api/docs/models/gpt-6.1-sol) |
| `gpt-6.1-terra` | Visual reasoning, accurate screen interaction and clear writing | `none`, `low`, `medium` default, `high`, `xhigh`, `max` | $1 / $5 | Visual/UI specialist, diagram and screen interpretation, constrained app workflows, concise writing; evaluate scoped code fixes locally. [Official model page](https://developers.openai.com/api/docs/models/gpt-6.1-terra) |
| `gpt-6-luna` | Focused, frequent work | `none`, `low`, `medium` default, `high`, `xhigh`, `max` | $0.10 / $0.50 | Extraction, classification, triage, small specified patches and mechanical verification with explicit acceptance criteria. [Official model page](https://developers.openai.com/api/docs/models/gpt-6-luna) |
| `gpt-6-sol` | Previous Sol for coding and agent workflows; official page points to 6.1 Sol | `none`, `low`, `medium` default, `high`, `xhigh`, `max` | $2 / $10 | Preserve explicitly selected or already validated workflows; compare with 6.1 Sol before migration. [Official model page](https://developers.openai.com/api/docs/models/gpt-6-sol) |

`gpt-6.1-luna` was **not verified**: the exact model URL returned 404 and the current catalog/API model enumeration did not establish that ID. Do not fabricate an alias or silently substitute another model. The earlier `gpt-5.6-sol`, `gpt-5.6-terra` and `gpt-5.6-luna` remain referenced in official guidance, but that does not establish access for this user's account. [Catalog](https://developers.openai.com/api/docs/models/all), [current family guide](https://developers.openai.com/api/docs/guides/latest-model), [Responses model enumeration](https://developers.openai.com/api/reference/resources/responses/methods/create).

## Quantitative evidence, kept in separate comparison groups

### A. Terminal-Bench 4.0: terminal-based agent tasks

The benchmark owner's current leaderboard identifies the harness as **Codex** and reports the following resolution rates with **95% confidence intervals**. Exact Codex versions and all execution controls are not exposed in these summary rows. The fetched public page's embedded result metadata confirms the numbers; no task corpus was downloaded. Values are a 5 November snapshot, not a guarantee for Council's Windows setup. [Terminal-Bench 4.0, all efforts](https://www.tbench.ai/?efforts=all&version=4.0).

| Model | Effort | Resolution rate | Submitted evaluation's total cost |
| --- | --- | --- | --- |
| GPT-6 Astra | low | 50.61% ± 2.75 | $1,557.30 |
| GPT-6 Astra | medium | 54.24% ± 2.66 | $1,914.80 |
| GPT-6 Astra | high | 57.88% ± 2.97 | $2,269.42 |
| GPT-6 Astra | xhigh | 57.88% ± 2.72 | $2,350.51 |
| GPT-6 Astra | max | 58.18% ± 2.79 | $3,267.18 |
| GPT-6.1 Sol | max | 58.18% ± 3.11 | $634.24 |
| GPT-6 Sol | max | 49.39% ± 3.23 | $1,451.32 |
| GPT-6 Luna | max | 16.36% ± 2.72 | $72.14 |

The small differences among Astra's high/xhigh/max results do not establish a statistically reliable advantage for max. Terra 6.1 is absent from this table. These evaluation totals are not interchangeable with per-task prices or subscription usage.

### B. A real plugin: model and harness interaction

OpenAI's **23 September 2026** Cookbook study uses 108 held-out BLS questions across 56 surveys, Codex **0.159.2**, promptfoo **0.123.1**, and a `gpt-5.6-sol` answer judge. Table 1 has one run per model; the summary does not state each effort. [Harness-aware evaluation of plugins](https://developers.openai.com/cookbook/examples/partners/harness_aware_plugin_evals/harness_aware_plugin_evals).

| Model in Codex | Pass rate | Tool calls / question | Reasoning tokens / question |
| --- | --- | --- | --- |
| GPT-6.1 Sol | 0.94 | 4.02 | 19 |
| GPT-6 Astra | 0.94 | 3.74 | 14 |
| GPT-6 Luna | 0.88 | 5.54 | 1,229 |

Astra and Sol differed by one answer, insufficient to rank them reliably here. Luna's three repeats scored 0.88, 0.89 and 0.86; individual outcomes varied. Luna scored 0.74 in a standalone loop versus 0.88 under Codex, so a Council harness change can change the comparison. Lower token pricing alone does not prove fewer tokens, fewer tool calls or less repair work.

### C. Repository maintenance: accepted findings and actual evaluation cost

OpenAI's **14 October 2026** engineering case study compares the same Codex stale-experiment audit over more than 6,000 files and one million lines of Python, TypeScript, Go and Rust. The author manually reviewed findings. Costs include research subagents and automatic follow-ups. [Maintaining software with the GPT-6 family](https://developers.openai.com/blog/maintaining-software-with-the-gpt-6-family).

| Author's model/mode label | Accepted findings | Evaluation cost | Reported cost / finding |
| --- | --- | --- | --- |
| GPT-6 Astra Ultra | 142 | $109.06 | $0.77 |
| GPT-6.1 Sol Ultra | 131 | $15.12 | $0.12 |
| GPT-6.1 Sol Medium | 86 | $5.27 | $0.06 |

Sol Ultra found about 92% as many accepted issues at 14% of the reported cost; Astra caught some indirect feature-flag dependencies Sol missed. This is one team's case study, not a randomized general coding benchmark. **Ultra is a product orchestration mode using subagents**, not an API effort spelling or proof of a single-model comparison. [Official product model controls](https://learn.chatgpt.com/docs/models).

### D. Spatial reasoning and research mathematics

These scores come from Epoch AI's individual model scorecards retrieved on 5 November, rounded as displayed. They measure different workloads and must remain separate columns. Each model link below supports its row.

| Model | Furniture Assembly accuracy | FrontierMath Tier 4 (v2) score |
| --- | --- | --- |
| [GPT-6 Astra](https://epoch.ai/models/gpt-6-astra) | 80% | 98% |
| [GPT-6.1 Sol](https://epoch.ai/models/gpt-6-1-sol) | 80% | 100% |
| [GPT-6.1 Terra](https://epoch.ai/models/gpt-6-1-terra) | 79% | 93% |
| [GPT-6 Sol](https://epoch.ai/models/gpt-6-sol) | 58% | 90% |
| [GPT-6 Luna](https://epoch.ai/models/gpt-6-luna) | 44% | 56% |

Furniture Assembly uses **60 photographs**, assembly-manual PDFs, zoom and Python in an agent sandbox. Models use the highest reasoning effort their API supports; grading checks mistake location and uses `gpt-5.6-sol` to judge its description. The benchmark's 80-step budget is an evaluation condition, **not a proposed Council limit**. A 79% versus 80% point estimate does not establish superiority or equivalence. These tasks support evaluating Terra for spatial/visual specialization, not assuming it is a weaker general-purpose rung. [Benchmark-owner methodology](https://epoch.ai/benchmarks/furniture-assembly).

FrontierMath **v2**, updated **12 June 2026**, has 43 Tier-4 problems in the full set, including two public examples; hub results generally use the private set. The harness supplies Python and answer submission. The summary model cards do not state the exact effort, repeat count or confidence interval per listed result. This is research mathematics, not code-maintenance accuracy, and must not be mixed with v1 results. [Benchmark-owner methodology and version history](https://epoch.ai/benchmarks/frontiermath-tier-4-v2).

### E. Astra's performance depends strongly on reasoning-state handling

ARC Prize's **3 September 2026** evaluation reports Astra's best ARC-AGI-3 Semi-Private results as **62.7% at max** with the Standard harness ($26,098 total), versus **99.9% at high** with a Provider Adapter ($18,817). The adapter preserves opaque reasoning state and compacts long conversations. Model effort and harness both change, so these rows do not isolate an effort effect. They show why Council must record the harness, state handling and model together. [Benchmark-owner verified results](https://arcprize.org/results/openai-gpt-6-astra), [methodological discussion](https://arcprize.org/blog/astra).

Third-party result labels can include settings absent from the current API contract. Astra's official API efforts exclude `none`; do not translate an evaluator's historical labels directly into Council configuration.

## Proposed task hierarchy and collaboration perks

This is a starting policy to validate on Council tasks, not measured Council performance or an immutable voting hierarchy:

| Task | Initial assignment | Escalation / collaboration perk |
| --- | --- | --- |
| Renames, narrow formatting, supplied-data extraction, log classification | Luna low; compare medium if tool use is involved | Mechanical acceptance checks; ask Sol only on genuine ambiguity or failed verification. |
| Screens, diagrams, app records, concise source-grounded prose | Terra medium | Visual specialist independently verifies saved state; use Sol for broader implementation dependencies. |
| Multi-file implementation, repeated maintenance, ordinary technical PM work | Sol 6.1 medium | Lead owns task decomposition and evidence; Terra reviews visual work; Luna handles small verified subtasks. |
| Conflicting evidence, subtle dependency analysis, difficult debugging | Sol 6.1 high/xhigh, then Astra advisor if unresolved | Send the specific uncertainty, full evidence and attempted approaches; avoid an entire second implementation by default. |
| Ambiguous architecture, broad root-cause investigation, consequential final disagreement | Astra medium/high; compare stronger efforts only when needed | Focused advisor or final reviewer with evidence-based reasons; human retains authorization and escalation decisions. |
| Existing explicitly selected GPT-6 Sol workflow | Keep GPT-6 Sol | Run a controlled comparison before changing an established model/harness combination. |

Official selection guidance suggests Luna low for scoped edits, Sol medium for complex technical work, and stronger efforts for harder deliverables, while recommending representative comparisons. These are defaults to experiment with, not restrictions. No evidence here supports a fixed rule that Astra always overrules Terra, or that max effort is always worth its added work. [Official model-selection guidance](https://developers.openai.com/api/docs/guides/model-selection).

For a Council trial, record the exact model and effort actually honored, harness/version, permissions, task family, acceptance outcome, regressions, useful evidence, complete provider token receipts when available, tool calls, elapsed time and human repair time. Compare identical inputs and repeat representative cases. Prefer the least expensive **validated result**, rather than the smallest model or lowest listed token rate. Keep the real account/subscription allowance separate from reference API prices. An orchestrator awaiting admitted work should yield to server-side waiting instead of producing model turns.

## Unknowns and exclusions

- No personal account access, billing limit, supported installed-harness model or current remaining allowance was inferred from documentation.
- No controlled same-Codex terminal score for Terra 6.1 was found in the opened Terminal-Bench 4.0 table.
- No universal latency guarantee, calibrated numeric voting weight or cross-vendor effort equivalence follows from these sources.
- ECI's live index and individual model pages showed differing rounded Sol 6.1 values during retrieval; this report avoids treating those inconsistent snapshots as a stable numeric rank.
- Published context specifications are not copied into Council as hard-coded context caps. Existing explicit model choices, full evidence and user configuration remain authoritative.
