# DeepSeek and Qwen model evidence

Retrieved 2026-11-05. Companion to [Anthropic and Google evidence](research-models-anthropic-google-2026-11-05.md). Primary provider cards establish identities and licensing; benchmark owners establish their own measurements. An API offering, downloadable checkpoint, benchmark entry and installed harness are separate identities.

## Verified offerings

| Offering | Exact documented identifier | Date | Distribution |
| --- | --- | --- | --- |
| DeepSeek V4.1 Flash 1017 | API `deepseek-flash` | 2026-10-17 update | Current hosted alias |
| DeepSeek V4.1 Flash 1017 weights | `deepseek-ai/DeepSeek-V4.1-Flash-1017` | 2026-10-17 | Open weights, MIT |
| Qwen3.8 Max | API `qwen3.8-max` | Current catalogue | Hosted production model |
| Qwen3.8 Max 0902 | API `qwen3.8-max-0902`; alias `qwen3.8-max-2026-09-02` | 2026-09-02 | Hosted dated snapshot |
| Qwen3.8 Max-class weights | `Qwen/Qwen3.8-2.4T-A95B` | August 2026 | Open weights, custom `qwen3.8-max` license |
| Qwen3.8 Flash | API `qwen3.8-flash` | 2026-08-26 launch | Hosted production model |
| Qwen3.8 Flash Next weights | `Qwen/Qwen3.8-Flash-Next` | August 2026 | Open weights, custom `qwen-community-1.0` license |
| Qwen3.8 27B | Weights `Qwen/Qwen3.8-27B`; QwenCloud `qwen3.8-27b` | 2026-08-19 API listing | Open weights, Apache-2.0 |

The [DeepSeek changelog](https://api-docs.deepseek.com/updates/) explicitly maps `deepseek-flash` to the October 17 update. Compatibility aliases for retired Flash models should not be treated as pinned older checkpoints. Its [official checkpoint](https://huggingface.co/deepseek-ai/DeepSeek-V4.1-Flash-1017) documents MIT-licensed weights and encoding/inference resources.

The [QwenCloud changelog](https://docs.qwencloud.com/changelog/models) confirms the API names and regional rollout; a catalogue entry does not establish entitlement in the user's region/account. [Max weights](https://huggingface.co/Qwen/Qwen3.8-2.4T-A95B) underlie the API Max offering, which adds features including vision and built-in tools. [Flash Next weights](https://huggingface.co/Qwen/Qwen3.8-Flash-Next) similarly underlie the production Flash API; they are not automatically identical evaluation targets. Flash Next supports low/medium/xhigh reasoning with xhigh as its documented default. Its card cautions that reducing per-turn reasoning can increase retries and total task time.

Open weights does not mean an unrestricted license, a fully open training process, or a practical installation on this PC. The [Max license](https://huggingface.co/Qwen/Qwen3.8-2.4T-A95B/blob/main/LICENSE) contains a separate commercial-license condition for certain large model-service/AI-assistant businesses. Preserve the exact license identifier and link; do not label every Qwen model Apache-2.0 or every Max model closed-weight. The smaller [27B checkpoint](https://huggingface.co/Qwen/Qwen3.8-27B) specifically uses Apache-2.0; its capabilities and resource requirements need separate validation.

## Comparable frontend preference snapshot

[Arena WebDev](https://arena.ai/leaderboard/code/webdev), October 27. These ratings share the snapshot used in the companion report. Confidence intervals overlap for several nearby entries; preliminary results need separate labeling.

| Benchmark entry | Rating | Status |
| --- | ---: | --- |
| Qwen3.8-Max 0902 | 1676 ±7 | Preliminary |
| Qwen3.8-Max | 1672 ±12 | Preliminary |
| DeepSeek-V4.1-Flash-20261017 Max | 1665 ±8 | Published |
| Qwen3.8-Flash-Next | 1628 ±8 | Preliminary |

## Composite reasoning/work evidence

The current Artificial Analysis methodology is v5.0.1; see the companion report for scope. These are evaluation-specific task costs and times, not subscription charges or a promise for Council workloads.

| Exact benchmark configuration | Intelligence Index | USD/task | Minutes/task | Owner source |
| --- | ---: | ---: | ---: | --- |
| DeepSeek V4.1 Flash 1017 Max | 33 | 0.60 | 13.3 | [AA model page](https://artificialanalysis.ai/models/deepseek-v4-1-flash-1017) |
| Qwen3.8 Max 0902 | 34 | 10.02 | 60.8 | [AA model page](https://artificialanalysis.ai/models/qwen3-8-max) |

The [AA leaderboard](https://artificialanalysis.ai/leaderboards/models) listed Flash Next and the 2.4T open checkpoint without an Intelligence Index value; they remain unranked in that group. Qwen3.8 27B xHigh scored 20. Do not transfer the hosted Max 0902 score to its downloadable base checkpoint.

## Provider-reported benchmark groups

| Model | Benchmark | Result | Harness/method qualifier |
| --- | --- | ---: | --- |
| DeepSeek V4.1 Flash 1017 | Terminal-Bench 4.0 | 42.3% | Claude Code |
| DeepSeek V4.1 Flash 1017 | DeepSWE 1.1 | 77.0% | Mini-SWE-V2 |
| DeepSeek V4.1 Flash 1017 | HLE, no tools | 42.6% | Full reported benchmark; text-only subset 44.0% is separate |
| Qwen3.8 Flash Next | DeepSWE 1.1 | 58.7% | Best of two harnesses; mini-SWE-agent wins |
| Qwen3.8 Flash Next | SWE-bench Pro | 62.5% | Claude Code; provider corrected tasks and re-evaluated baselines |
| Qwen3.8 Flash Next | SWE-bench Multilingual | 81.0% | mini-SWE-agent |
| Qwen3.8 Flash Next | LiveCodeBench v6 | 91.9% | Provider card, not independently retrieved owner result |
| Qwen3.8 Max-class release | Terminal-Bench 2.1 | 86.6% | Provider card; separate from TB4.0 |
| Qwen3.8 Max-class release | DeepSWE 1.1 | 56.6% | Provider card; not Max 0902 proof |
| Qwen3.8 Max-class release | SWE-bench Pro | 67.7% | Provider card; not Verified or Multilingual |

Sources: [DeepSeek update](https://api-docs.deepseek.com/updates/), [Flash Next card](https://huggingface.co/Qwen/Qwen3.8-Flash-Next) and [Max-class card](https://huggingface.co/Qwen/Qwen3.8-2.4T-A95B). Qwen's Flash Next coding evaluations use temperature 1.0/top-p 0.95 and a 256K evaluation context; these are experimental settings, not Council runtime caps. The Flash card chooses the best DeepSWE result across Claude Code and mini-SWE-agent. DeepSeek's Mini-SWE-V2 result and the common mini-SWE-agent owner cohort therefore need distinct comparison groups even though both say DeepSWE 1.1.

## Council routing implications (inferences)

DeepSeek 1017 is a credible cost-sensitive implementation/reviewer candidate when an explicit compatible endpoint and harness are available; its newer checkpoint must be distinguished from September Flash. Qwen Flash Next is a candidate for self-hosting or scoped multimodal/code work, while Max 0902 deserves separate hosted-model evidence. Current AA Max 0902 cost/time does not support assuming that every model named Qwen is cheap or fast. Nearby Arena ratings do not establish a decisive frontend winner.

No installed-model availability, provider credentials, live inference, or GPU capability was inspected for this research. Any Council configuration must preserve exact provider/model/harness identity, observed capabilities, billing type, user overrides and local validation evidence. Do not auto-enable an agent because a public benchmark row exists.
