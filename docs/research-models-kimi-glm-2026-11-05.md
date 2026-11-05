# Kimi and GLM model research — 5 November 2026

This research separates downloadable weights, license conditions, hosted model IDs, provider claims and measured evaluations. Published model quality does not establish installed Council support, personal subscription access, local hardware suitability or the model actually served by a proxy. No inference calls, weight downloads, account probes or credentials were used.

## Four verified model profiles

| Model / official checkpoint | Hosted identifiers verified in primary documentation | Openness and license | Proposed Council specialization |
| --- | --- | --- | --- |
| [Moonshot Kimi K3.1 Pro](https://huggingface.co/moonshotai/Kimi-K3.1-Pro) | Public Kimi API: `kimi-k3.1-pro`; Kimi Code: `k3.1-pro`. Highspeed has separate hosted IDs. | Weights listed in Moonshot's official repository; custom **Kimi K3.1-Pro License**, not MIT. | Strong implementation / long-horizon agent candidate; independent advisor for complex code or evidence synthesis. Validate factual claims and completion evidence. |
| [Moonshot Kimi K3](https://huggingface.co/moonshotai/Kimi-K3) | Public Kimi API: `kimi-k3`; Kimi Code: `k3` or the explicit `k3-256k` variant. | Weights listed; custom **Kimi K3 License**. | Preserve existing validated K3 workflows; compare with K3.1 Pro before changing the model. Do not inherit the newer model's scores. |
| [Z.ai GLM-5.3](https://huggingface.co/zai-org/GLM-5.3) | `glm-5.3` | Weights listed; custom **GLM-5.3 License**. | Text-based coding, long-running repository work and an independent implementation/review perspective. Lower token prices do not alone prove lower completion cost. |
| [Z.ai GLM-5.3-Flash](https://huggingface.co/zai-org/GLM-5.3-Flash) | `glm-5.3-flash` | Weights listed; **MIT** license verified in the official repository. | Budget-conscious multimodal worker, routine implementation, source-grounded extraction and visual inspection with acceptance checks. |

Kimi's [K3.1 Pro announcement](https://www.kimi.com/en/blog/kimi-k3-1-pro) and [K3 announcement](https://www.kimi.com/en/blog/kimi-k3) establish the public API identifiers. The [canonical Kimi Code documentation](https://www.kimi.com/code/docs/en/kimi-code/models) establishes its different Code identifiers. GLM identifiers come from the [GLM-5.3 API guide](https://docs.z.ai/guides/llm/glm-5.3) and [Flash/FlashX guide](https://docs.z.ai/guides/vlm/glm-5.3-flash).

K3/K3.1 Pro are very large multimodal MoE checkpoints; Flash's card describes 320B total and 18B active parameters. An efficient hosted worker is not automatically a small model suitable for this PC. No hardware or context ceiling is introduced into Council by this report.

## License facts and reproducible weight evidence

The two Kimi licenses retain copyright/permission notices and add conditions for certain commercial deployments: a model-as-a-service business above **$20 million aggregate revenue over a consecutive 12 months** needs a separate Moonshot agreement. Large commercial products above specified user/revenue thresholds have an attribution condition. The licenses explicitly exempt their defined internal use and official/certified inference-partner use from those additional sections. These are custom open-weight licenses, not a family-wide unrestricted open-source grant. [K3.1 Pro license](https://huggingface.co/moonshotai/Kimi-K3.1-Pro/blob/main/LICENSE), [K3 license](https://huggingface.co/moonshotai/Kimi-K3/blob/main/LICENSE).

GLM-5.3's custom license requires notice retention and specifies a Z.ai security review for model-as-a-service businesses exceeding **$10 billion aggregate revenue over a consecutive 12 months**. Flash instead carries the MIT notice and permission terms. Do not apply Flash's license to GLM-5.3, FlashX, third-party quantizations or hosted service terms without checking that artifact. [GLM-5.3 license](https://huggingface.co/zai-org/GLM-5.3/blob/main/LICENSE), [Flash MIT license](https://huggingface.co/zai-org/GLM-5.3-Flash/blob/main/LICENSE).

Public Hugging Face repository metadata retrieved on 5 November lists actual weight artifacts, rather than only a future release promise:

| Repository | Captured revision SHA | Last modified, UTC | Listed `.safetensors`/`.bin` files |
| --- | --- | --- | --- |
| `moonshotai/Kimi-K3.1-Pro` | `d9e99b0792a5e93b78ea9bea62a68167e36ddd9e` | 2026-11-05 07:11:43 | 96 |
| `moonshotai/Kimi-K3` | `f831ab66814297da540d832a5235f8e904f29d06` | 2026-09-02 02:22:41 | 96 |
| `zai-org/GLM-5.3` | `ba32f8c19c8738e2c57c71798c601d477a7bf844` | 2026-10-14 12:22:48 | 141 |
| `zai-org/GLM-5.3-Flash` | `46a050bec13002ad29ee795c16e61cf4fa3c4a9c` | 2026-10-15 06:32:52 | 62 |

These are repository observations, not release dates or a verification that every shard executes correctly. Sources: public metadata for [K3.1 Pro](https://huggingface.co/api/models/moonshotai/Kimi-K3.1-Pro), [K3](https://huggingface.co/api/models/moonshotai/Kimi-K3), [GLM-5.3](https://huggingface.co/api/models/zai-org/GLM-5.3), and [Flash](https://huggingface.co/api/models/zai-org/GLM-5.3-Flash). The K3.1 Pro launch page promised weights by 4 November; the observed artifacts are stronger current evidence than that promise. Some aggregator pages still label it proprietary or unrestricted commercially; the repository and exact license take precedence.

## Effort, identity and quota caveats that affect collaboration

Kimi Code documents `low`, `high`, `max`, with default `high` for K3/K3.1 Pro. Its adapter maps host `medium` to `high` and `xhigh`/`ultra` to `max`; those labels are not equivalent effort scales across vendors. K3.1 Pro rejects disabled thinking. The older K3 series can route disabled-thinking requests to K2.8 Preview. The documentation also says a mistyped model ID can silently fall back to non-thinking K2.8 Preview. Council should verify effective identity and report unverified execution rather than assume the requested model was honored. [Official Code model/routing contract](https://www.kimi.com/code/docs/en/kimi-code/models).

K3.1 Pro Highspeed is a hosted service choice documented as approximately 2–3× output speed with about 2× Code quota consumption; it does not establish faster end-to-end tool execution or a separately licensed public checkpoint. Do not infer an individual subscriber's allowance from this ratio. [Code model table](https://www.kimi.com/code/docs/en/kimi-code/models).

GLM-5.3 requires thinking and accepts `low`/`high`/`max`, default `max`; its API guide recommends max for complex coding. Flash's card also supports these three effort values and says omitted or unrecognized values default to max. Set a supported value explicitly to avoid an accidental expensive thinking setting. FlashX has a separate API ID; no separate FlashX weight artifact was verified here. [GLM API contract](https://docs.z.ai/guides/llm/glm-5.3), [Flash card notes](https://huggingface.co/zai-org/GLM-5.3-Flash).

## Quantitative evidence: preserve evaluation cohorts

### Independent AA comparison, current methodology

Artificial Analysis's current comparison page identifies **Intelligence Index v5.0.1**. Rounded task metrics below come from the same K3.1 Pro versus GLM-5.3 Max comparison page; Kimi's row does not separately name an effort. [Current comparison](https://artificialanalysis.ai/models/comparisons/kimi-k3-1-pro-vs-glm-5-3).

| Metric | Kimi K3.1 Pro | GLM-5.3 Max |
| --- | --- | --- |
| Intelligence Index v5.0.1 | 46 | 32 |
| Terminal-Bench 4.0 | 67% | 42% |
| AutomationBench-AA | 73% | 62% |
| FrontierCode 2.0 | 25% | 22% |
| Weighted evaluation cost / task | $3.68 | $5.01 |
| Output tokens / weighted task | 72.2K | 119K |

AA runs all **66 Terminal-Bench 4.0 tasks** with **mini-SWE-agent**, pass@1 averaged over **three repeats**. The index weights several task families; its score is not an accuracy percentage. The **23 October** launch article's index score **55** belongs to an earlier methodology and must not be ranked against current **46** as though the model regressed. Costs describe AA's tested API workload, not the user's subscription or future Council tasks. [Terminal methodology](https://artificialanalysis.ai/evaluations/terminalbench-4-0), [versioned index methodology](https://artificialanalysis.ai/methodology/intelligence-benchmarking), [dated launch report](https://artificialanalysis.ai/articles/kimi-k3-1-pro-launch-benchmarks/).

### Benchmark-owner terminal board, different harness/configurations

The current owner board reports **GLM-5.3 Max + Claude Code: 41.82% ± 3.23** and **GLM-5.3-Flash, displayed effort `none`, + Claude Code: 35.76% ± 3.54**, with 95% intervals. These rows were verified in the public page's embedded result data. They are not the same configuration as AA's mini-SWE-agent runs. The historical `none` label is not a supported-effort recommendation for the current API. [Terminal-Bench 4.0 owner board](https://www.tbench.ai/?efforts=all&version=4.0).

### Vendor-reported Kimi results, separately labelled

K3.1 Pro's model card reports **65.7% Terminal-Bench 4.0** using **Claude Code**, reasoning `max`, temperature 1.0, top-p 0.95 unless otherwise specified. Its comparison columns combine other releases' best reported results, so this is not a controlled fresh run of every model. Its internal Kimi Code Bench 3.0 Long-horizon reports **61.2% versus K3's 55.95%**; useful directionally, but not independent or public-dataset replication. [Official model card and evaluation notes](https://huggingface.co/moonshotai/Kimi-K3.1-Pro), [vendor announcement](https://www.kimi.com/en/blog/kimi-k3-1-pro).

The vendor's 65.7%, AA's approximately 67%, and other owner-board rows remain separate observations. Do not average them, transplant one model's score into another harness, or compare older Terminal-Bench 2.1/3.0 results with 4.0. No stable numerical voting weight or universal ranking follows from these measurements.

## Suggested Council trial policy

- Treat K3.1 Pro as an alternative complex-work lead/advisor; use a focused independent review rather than automatically repeating every worker's implementation.
- Treat GLM-5.3 as a text/code specialist and Flash as a lower-cost multimodal candidate; test actual pass rates, retries, token receipts and verification effort before assigning a permanent rank.
- Keep existing K3 workflows when explicitly selected; an evaluation of K3.1 Pro does not upgrade their capabilities automatically.
- Preserve exact endpoint, checkpoint/quantization, harness, native effort, effective identity and permission alongside every result. Hosted subscription, hosted API and self-hosted weights are different billing/deployment choices.
- Checkpoint availability does not establish that a Council adapter is implemented or that this PC can host it. Add no silent provider substitution, credential discovery or hard-coded context caps from this research.
