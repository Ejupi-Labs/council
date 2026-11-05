# Anthropic and Google model evidence

Retrieved 2026-11-05. This is a dated research snapshot for Council routing, not a provider availability probe. Scores describe the stated model, effort, harness, dataset version and evaluation system together. Missing evidence means unranked; separate benchmarks are not averaged into a universal hierarchy.

## Verified identities and access

| Provider model | Documented API ID | Release/update | Access and intended starting role |
| --- | --- | --- | --- |
| Claude Opus 5.5 | `claude-opus-5-5` | 2026-09-22 | General availability; demanding coding/review candidate |
| Claude Sonnet 5.5 | `claude-sonnet-5-5` | 2026-09-28 | General availability; balanced implementation candidate |
| Claude Haiku 5.5 | `claude-haiku-5-5` | 2026-10-07 | General availability; scoped, repetitive subagent work |
| Claude Fable 5.1 | `claude-fable-5-1` | 2026-09-15 | General availability; alternative for difficult sustained reasoning |
| Gemini 4 Argon | `gemini-4-argon` | October 2026 | Stable API; difficult reasoning/multimodal candidate |
| Gemini 3.8 Flash | `gemini-3.8-flash` | September 2026 | Stable API; implementation/latency candidate |
| Gemini 3.6 Flash | `gemini-3.6-flash` | July 2026 | Previous generation; production API remains available |
| Gemini 3.5 Flash-Lite | `gemini-3.5-flash-lite` | July 2026 | Stable API; throughput/extraction candidate |

Anthropic's [model overview](https://platform.claude.com/docs/en/models/overview) documents the four exact Claude IDs and positions Opus for most workloads, Fable for harder long-horizon reasoning, Sonnet for balance and Haiku for high-volume work. Release dates come from the [model news index](https://www.anthropic.com/news/models). Google's [API model index](https://aistudio.google.com/docs/models), [Argon details](https://aistudio.google.com/docs/models/gemini-4-argon), [Flash details](https://aistudio.google.com/docs/models/gemini-3.8-flash) and [Flash-Lite details](https://aistudio.google.com/docs/models/gemini-3.5-flash-lite) establish the Google IDs and stable status. Flash and Argon expose low/medium/high thinking; Flash does not support minimal.

The [3.6 Flash API page](https://aistudio.google.com/docs/models/gemini-3.6-flash) confirms continued production availability and explicitly directs new Flash users to 3.8. Gemini 4 Carbon was announced October 28, but its [release](https://blog.google/innovation-and-ai/models-and-research/gemini-models/introducing-gemini-4-carbon/) describes restricted access for selected cyber defenders ahead of broader availability. No public runnable API ID was verified. Similarly, [Claude Mythos 5.1](https://www.anthropic.com/claude-fable-and-mythos-5-1) has trusted access, while Fable is generally available. Keep both restricted offerings out of automatic runnable-model defaults.

## Comparable owner snapshots: frontend preference

[Arena WebDev](https://arena.ai/leaderboard/code/webdev), October 27 snapshot. Values are preference ratings with displayed uncertainty, not task pass rates. Effort labels are part of each entry. Overlapping intervals do not establish an ordering.

| Model and effort | Rating | Preliminary |
| --- | ---: | --- |
| Opus 5.5 Max | 1810 ±13 | No |
| Sonnet 5.5 xHigh | 1770 ±11 | No |
| Carbon High | 1768 ±15 | Yes; restricted |
| Fable 5.1 Max | 1746 ±9 | No |
| Sonnet 5.5 High | 1711 ±9 | No |
| Argon High | 1679 ±11 | No |
| Haiku 5.5 High | 1589 ±10 | No |
| Gemini 3.8 Flash High | 1583 ±7 | Yes |

## Comparable owner snapshot: writing

[Arena Writing, Literature & Language](https://arena.ai/leaderboard/chat/text/industry-writing-and-literature-and-language), October 27 snapshot. This occupational category is not a creative-writing-only test. Arena uses [blind paired human preferences](https://arena.ai/how-it-works). The page intermittently returned only its shell on subsequent retrievals; these rows were visible in the full retrieved owner page.

| Model and effort | Rating | Preliminary |
| --- | ---: | --- |
| Carbon High | 1529 ±18 | Yes; restricted |
| Opus 5.5 High | 1519 ±11 | No |
| Argon High | 1517 ±11 | No |
| Fable 5.1 Max | 1497 ±9 | No |
| Gemini 3.8 Flash High | 1492 ±7 | Yes |
| Sonnet 5.5 xHigh | 1472 ±12 | No |

## Comparable composite snapshot: broad reasoning and work

[Artificial Analysis model leaderboard](https://artificialanalysis.ai/leaderboards/models), retrieved November 5. Intelligence Index values are rounded. Costs are that evaluation's USD per task, not generic API prices or subscription charges.

| Model and effort | Index | USD/task | Minutes/task |
| --- | ---: | ---: | ---: |
| Carbon High | 60 | 7.76 | 30.3 |
| Opus 5.5 Max | 59 | 10.04 | 26.2 |
| Opus 5.5 xHigh | 57 | 6.05 | 16.0 |
| Sonnet 5.5 Max | 55 | 10.02 | 30.7 |
| Opus 5.5 High | 52 | 3.46 | 9.1 |
| Argon High | 50 | 9.95 | 26.2 |
| Fable 5.1 xHigh | 50 | 10.85 | 18.6 |
| Sonnet 5.5 xHigh | 50 | 4.09 | 14.7 |
| Fable 5.1 Max | 49 | 12.99 | 22.2 |
| Sonnet 5.5 High | 42 | 1.64 | 6.8 |
| Haiku 5.5 Max | 36 | 0.41 | 14.6 |
| Gemini 3.8 Flash High | 31 | 7.77 | 14.0 |
| Haiku 5.5 High | 30 | 0.16 | 6.7 |
| Haiku 5.5 Medium | 25 | 0.09 | 4.0 |
| Haiku 5.5 Low | 19 | 0.04 | 2.2 |
| Gemini 3.5 Flash-Lite | 12 | 0.69 | 1.7 |

The [v5.0.1 methodology](https://artificialanalysis.ai/methodology/intelligence-benchmarking) combines nine evaluations, weighting knowledge, coding, science and general capabilities at 25% each. It is mainly English/text. It includes FrontierCode 2.0 and Terminal-Bench 4.0, so it is not directly comparable with older index revisions. Repeated pass@1 trials are aggregated; this is not best-of-N. Effort is an observed configuration, not a guaranteed monotonic intelligence multiplier.

## Repository and terminal evidence: separate evaluation groups

Provider reports below are useful but are not independent, identical-harness comparisons. Percentages are the respective benchmark's reported result.

| Source/model | Benchmark/version | Result | Important qualifier |
| --- | --- | ---: | --- |
| Anthropic Opus 5.5 | Terminal-Bench 4.0 | 66.4% | xHigh; ±2.6 percentage-point SE |
| Anthropic Opus 5.5 | FrontierCode 1.1 Main | 54.4% | Provider production system |
| Anthropic Opus 5.5 | CursorBench 4.0 | 57.8% | Provider report |
| Anthropic Sonnet 5.5 | Terminal-Bench 4.0 | 70.6% | Effort not established in extracted release table |
| Anthropic Sonnet 5.5 | FrontierCode 1.1 Main | 46.2% Max / 52.1% xHigh | Different effort entries |
| Anthropic Sonnet 5.5 | CursorBench 4.0 | 55.5% | Same named version, provider report |
| Anthropic Haiku 5.5 | Terminal-Bench 4.0 | 39.2% | Effort not established in extracted release table |
| Anthropic Haiku 5.5 | FrontierCode 1.1 Main | 46.4% | Provider report |
| Anthropic Fable 5.1 | Terminal-Bench 4.0 | 55.8% | Provider production system |
| Google Argon | Terminal-Bench 4.0 | 57.4% | Self-computed, highest thinking |
| Google Argon | DeepSWE 1.1 | 77.9% | mini-SWE-agent, self-computed |
| Google Argon | FrontierSWE v2 / VibeCodeBench | 55.0% / 91.9% | Separate benchmarks |
| Google Argon | RiemannBench | 76.0% | Mathematical reasoning |
| Google Gemini 3.8 Flash | Terminal-Bench 2.1 | 89.4% | Terminus 2; not TB4.0 |
| Google Gemini 3.8 Flash | DeepSWE 1.1 | 73.7% | mini-SWE-agent, High thinking |

Sources: [Opus release](https://www.anthropic.com/claude-opus-5-5), [Sonnet release](https://www.anthropic.com/claude-sonnet-5-5), [Haiku release](https://www.anthropic.com/claude-haiku-5-5), [Fable release](https://www.anthropic.com/claude-fable-and-mythos-5-1), [Argon card](https://deepmind.google/models/model-cards/gemini-4-argon/), [Argon methodology](https://deepmind.google/models/evals-methodology/gemini-4-argon/), [Flash card](https://deepmind.google/models/model-cards/gemini-3-8-flash/) and [Flash methodology](https://deepmind.google/models/evals-methodology/gemini-3-8-flash/). Opus release evaluations generally use Max except noted cases; safeguards can route intervened tasks to older Opus models. Fable reports also include safeguard fallbacks. These are production-system results, not isolated weights. Argon's card is dated October 13; its methodology gives highest-thinking, pass@1 API evaluations except noted cases.

The independent [DeepSWE owner board](https://deepswe.datacurve.ai/) captured September 22 lists 113 tasks across 91 repositories, with a common mini-SWE-agent harness. Gemini 3.8 Flash High is 74% ±1% there, consistent with Google's rounded 73.7%. Current Argon and Claude 5.5 entries were absent from that captured owner table; they remain unranked in this independent group.

Current numeric rows could not be extracted from the [SWE-bench owner board](https://www.swebench.com/), [Terminal-Bench owner board](https://www.tbench.ai/leaderboard/terminal-bench/4.0) or [LiveCodeBench owner board](https://livecodebench.github.io/leaderboard.html). Do not replace these missing rows with estimated aggregator scores. SWE-bench Verified, Pro, Full and Multilingual are different datasets; TB2.1 and TB4.0, CursorBench3.2/4.0, and GDPval-AA v2/v2.1 likewise need separate groups.

## Council routing implications (inferences)

- Use Haiku or Flash-Lite for extraction, classification and tightly bounded edits when local validation passes; promote after concrete failures. A smaller per-turn bill does not establish lower total task cost.
- Sonnet High is a reasonable balanced implementation starting point. Opus Max has the strongest verified WebDev entry in this subset; use it for demanding frontend review or difficult implementation when the extra cost is justified.
- Opus High and Argon High are both plausible writing reviewers; the writing intervals overlap. Match language, artifact and style using local evaluations instead of assigning a universal winner.
- Opus Max leads this subset's accessible broad composite. Fable remains a documented alternative for hard long-horizon cases, not automatically the highest-ranked decision maker for every task. Carbon's benchmark strength does not grant access.

Claude Code, Junie and Antigravity are harnesses, not model identities. In the inspected Council implementation, native Claude supports Low/Medium/High/Max but rejects xHigh; Junie ACP supports explicit Low/Medium/High; native Antigravity passes Council's known effort settings but actual model acceptance is unproven. Benchmark xHigh scores therefore cannot be silently mapped to native Claude Max. ACP models must be advertised by their adapter. An alias without an exact configured/observed model remains a native-default identity with unknown benchmark match. Do not infer this account's entitlement from a public API catalogue, or turn provider context specs into Council hard limits.
