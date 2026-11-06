# Research: economical routing and account capacity

**Feature**: `004-economical-routing`
**Branch**: `refactor`
**Research date**: 2026-11-06
**Status**: Design decisions; implementation, independent review and qualification remain separate gates.

The requirements in [spec.md](spec.md) and the closed [requirements review](spec-review.md) are the
authority for this design. Accepted feature 003 qualified six native targets and Linux AMD64 race
checks at `ce8aaa7` in [run 42153455521](https://github.com/Ejupi-Labs/council/actions/runs/42153455521).
That is platform/offline qualification, not evidence of live account access, model suitability or
subscription savings. This research changes no application source and runs no vendor process,
administrative/help/version/auth command, credential inspection, model request, test or build.

## Evidence and version boundaries

Research read AGENTS.md, the product vision, functional requirements/specification, delivery plan,
constitution, accepted native contracts and relevant existing source. Context7 resolved
`/openai/codex` and `/websites/code_claude` before three scoped documentation queries. Current
primary pages were then opened on **2026-11-06**. OpenAI research used the OpenAI Docs skill.

| Primary evidence | Meaning and limitation |
| --- | --- |
| [Codex models](https://learn.chatgpt.com/docs/models), [pricing](https://learn.chatgpt.com/docs/pricing), [authentication](https://learn.chatgpt.com/docs/auth) | Current official destinations of the corresponding `developers.openai.com/codex/` URLs. Documented native options, roles and access/payment distinctions; not the user's installed catalogue or entitlement. |
| [Codex non-interactive execution](https://learn.chatgpt.com/docs/non-interactive-mode), [developer commands](https://learn.chatgpt.com/docs/developer-commands?surface=cli) | Native invocation and JSONL contract references. Examples are not completed Council live qualification. |
| [Claude model configuration](https://code.claude.com/docs/en/model-config), [CLI reference](https://code.claude.com/docs/en/cli-reference), [programmatic execution](https://code.claude.com/docs/en/headless) | Literal model arguments, alias/version behavior, print events and limits. Native configuration/provider restrictions can affect actual behavior. |
| [Claude errors](https://code.claude.com/docs/en/errors), [costs](https://code.claude.com/docs/en/costs), [authentication](https://code.claude.com/docs/en/authentication) | Distinguish failures, quota, client estimates and payment contexts. They do not expose this machine's login or complete external activity. |
| [Codex app-server](https://learn.chatgpt.com/docs/app-server), [Claude TypeScript SDK reference](https://code.claude.com/docs/en/agent-sdk/typescript) | Boundary/supporting references only. Their methods/types are not automatically capabilities of the accepted native exec profiles. No app-server or SDK transport is added in 004. |

These web pages are mutable and have a read date, not a fabricated source-commit pin. Feature 003's
[pinned supporting source and documented release reference](../003-claude-interoperability/research.md#evidence-basis-and-version-assumptions)
remain attributable to that research: they do not pin today's separately hosted model/usage pages.
The inherited Claude contract assumes protocol support from 2.1.259 and uses 2.1.318 as its dated
documentation reference; neither number is an observed local installation.

## Decision 1: extend the accepted stack and owner

**Decision**: retain Go 1.27.3, authored embedded TypeScript, SQLite through `modernc.org/sqlite`
v1.61.1 and `golang.org/x/sys` v0.49.0. Use the existing store transaction/receipt, process, evidence,
workspace and shared owner boundaries. Add no runtime dependency, second service or worker pool.
Routing and metadata operations are deterministic local computation; filesystem/vendor/process
work stays outside SQL transactions.

**Rationale**: budget reservation, immutable route capture and actual launch need the same
authoritative owner and exact durable ordering already qualified in 003. Replacing that foundation
would add Windows process/privacy and recovery risk without advancing the routing requirement.

**Alternatives rejected**: another language/runtime, a separate scheduler, a plugin framework,
runtime model SDKs, and a cloud routing service. None is needed for two native contexts or
zero-model-call selection. Existing platform, packaging, privacy and developer-check gates remain.

## Decision 2: keep the two native exec profiles

**Decision**: continue `codex-exec-jsonl/002-v1` and `claude-print-stream-json/003-v1` with their
accepted shell-free executable/argv, complete stdin, source capture, stream bounds, one-shot
terminal parsing, response retention, root exit, drainage and owned-cleanup proofs. Per-attempt
selection changes only a captured supported model argument; it does not change another chat.

Codex app-server offers account-dependent `model/list` and `account/rateLimits/read`, but these are
different contracts from exec JSONL. They are excluded from 004. Claude SDK-shaped experimental
usage reports likewise do not establish native CLI support. Capacity remains unknown unless a
supported sourced observation/report supplies it. [App-server reference](https://learn.chatgpt.com/docs/app-server),
[supporting Claude reference](https://code.claude.com/docs/en/agent-sdk/typescript).

**Rationale**: accepted process/ownership and source boundaries remain meaningful without a new
bidirectional protocol, runtime package or background metadata invocation. Normal selection,
refresh, waiting and inspection need no vendor command or model call.

**Alternatives rejected**: TUI scraping; `/model`, `/usage`, login/auth/version probes during
development or routine refresh; reading vendor configuration/credential caches; adding an API key;
using SDK/app-server types as if exec had emitted them. No vendor-global settings are rewritten.

## Decision 3: publish six small, dated reference profiles

**Decision**: use these exact built-in policy-profile identities and selectors. Empty selector
means **omit** `--model`; it is not the literal string `default` or an invented model identity.

| Profile ID | Native context | Model selector | Declared routing role |
| --- | --- | --- | --- |
| `codex-native-default` | `codex-native` | Empty | Unresolved native configuration; conservative fallback when compatible with known requirements. |
| `codex-luna-economy` | `codex-native` | `gpt-6-luna` | Documented economy candidate for suitable focused work. |
| `codex-sol-general` | `codex-native` | `gpt-6.1-sol` | General-work reference when the routine/economy fit is not established. |
| `claude-native-default` | `claude-native` | Empty | Unresolved native configuration; no claim of economical selection. |
| `claude-haiku-economy` | `claude-native` | `haiku` | Native family alias documented for suitable simple work. |
| `claude-sonnet-general` | `claude-native` | `sonnet` | Native family alias documented for general/daily coding. |

Current Codex documentation describes Luna as efficient for focused coding/repeatable work, and
Sol for broader work at lower cost than Astra. It documents both selector spellings. Access varies
by plan/client/workspace. The page retired ChatGPT-authenticated `gpt-5.4-mini` on 2026-08-31 and names
Luna as a replacement when available; stale mini examples are not catalogue seeds. It establishes
no minimum Codex CLI version for the two selected spellings, so the catalogue must not invent one.
[Codex models](https://learn.chatgpt.com/docs/models).

Claude documents `haiku` for fast/efficient simple work and `sonnet` for daily coding. Their current
Anthropic 5.5 alias revisions require respectively 2.1.293 and 2.1.284; this is distinct from print
protocol minimum 2.1.259. Providers, native alias overrides and versions can resolve them differently.
Known applicable older-version evidence excludes the corresponding curated current-reference
profile; it does not rewrite a user pin or prove that every older alias is intrinsically unusable.
Absent version evidence remains unknown, as in the inherited unverified native profile; it is not
a verified minimum or an invented incompatibility. Applicable known version negatives block the
curated profile. Unknown entitlement remains eligible by default with its uncertainty visible.
[Claude model configuration](https://code.claude.com/docs/en/model-config).

Record a reference revision, primary URL, read date, role, declared preference tier and relevant
version assumption. The profile ID is Council policy identity; the selector remains literal native
input. Neither is a measured quality score, actual provider/account/model observation or account
entitlement. A known version observation must identify its invocation/context and validity; a
changed binary/context cannot inherit another invocation's compatibility claim silently.

**Rationale**: four concrete documented choices plus two honest native defaults cover economy and
conservative operation without asking the user to maintain a universal model catalogue.

**Alternatives rejected**: obsolete mini names, auto-discovered credentials, inferring availability
from API model lists or an installed filename, pinning an alias to a guessed effective version,
or mapping an unknown account to the model list of a plan with the same marketing name.

## Decision 4: publish a conservative deterministic suitability rubric

**Decision**: evaluate necessary capabilities, effective permissions, task constraints, literal
pins, supported account/payment requirements and current negative evidence before economy.
An explicit routine declaration or a narrowly recognized bounded operation with known scope and
compatible constraints may establish the routine policy class. Text length, a keyword match or a
model's name alone cannot do so. The plan/contracts must enumerate the recognized operations and
their bounded conditions; an unrecognized case has unknown suitability, not inferred routine fit.

When suitability is unknown, select a compatible general/native-default profile or record a named
blocker. Do not advertise that choice as proven small-model suitability or minimum cost. Unknown
entitlement/occupancy/allowance is separately eligible under the captured default policy; it does
not satisfy a missing required capability, quality condition or known context requirement.
The 256 KiB prompt pipe bound is not a guarantee of a model's context capacity.

Among candidates that pass those gates, apply the captured declared economy/capacity preference
and a stable final tie-break. Comparable validated reference prices may inform the comparison;
otherwise retain the declared preference and explicit unknown/incomparable basis. A known busy
Codex context and suitable reported available Claude context therefore route to Claude without
changing personal Codex. The decision retains candidates, exclusions, sources and unknowns.

**Rationale**: useful defaults require neither a planning/classifier model call nor mandatory
model/account/price forms. A declared policy comparison is useful without claiming measured
intelligence, an account guarantee or a universal cost ordering.

**Alternatives rejected**: prompt-length heuristics as proof; global leaderboards; hidden model
consultation; multiplying allowance across aliases; silently treating every unknown task as easy;
or making every ordinary task supply a classification questionnaire.

## Decision 5: keep native identity, payment and capacity claims separate

**Decision**: maintain two native configuration contexts. A user may supply explicit declared
account associations and optional sourced reports; those do not select or verify a vendor login.
Exact actual-account requirements unsupported by the context block. No login/account switching,
direct paid API executor, automatic credit purchase or new provider/payment configuration is added.
Native configured billing/overage remains outside Council's enforceable financial guarantees.

Codex subscription and API-key authentication have different billing/access meanings. Claude also
supports subscription, Console and cloud contexts. Reusing native authentication opaquely does not
identify the active one. [Codex authentication](https://learn.chatgpt.com/docs/auth),
[Claude authentication](https://code.claude.com/docs/en/authentication).

Keep Council-managed occupancy separate from reported external occupancy, allowance and temporary
cooldown. User availability cannot release a managed claim. Shared declared account pools retain
their own identity across aliases and relabelling. Positive/occupancy reports default to 15-minute
validity; expiry becomes unknown. Unresolved exhaustion remains denying even when stale until an
attributable correction/recovery, with age visible. Reset-time passage is not replenishment.
Observation time and owner receipt time are both retained: receipt order alone cannot let a late
old positive erase a newer negative. Concurrent report/recovery changes need revision checking.

**Rationale**: this meets the idle-capacity use case with honest user-sourced evidence while
neither manufacturing telemetry nor requiring an account-maintenance ritual for normal starts.

**Alternatives rejected**: interpreting process presence/inactivity as external idleness, treating
a label as OAuth identity, resetting negative evidence through alias relinking, or equating a
temporary throttle with zero subscription balance.

## Decision 6: reserve managed work durably and keep authority finite

**Decision**: for new/adopted 004 work, capture exactly two default managed attempt-admission units
and one 30-minute wall-clock window beginning at the first budgeted admission. Queue, preparation,
execution and settlement consume that window; all attempts share it. Expiry stops new work and
requests controlled Stop. Bounded owned cleanup may finish afterward without extending authoring
authority or promising an OS hard real-time completion time. Automatic continuation/escalation is
OFF by default.

Reserve admission, immutable target/route and cumulative budget effect atomically with the
existing durable request receipt. Accepted-but-blocked preparation consumes a unit. Unknown commit
effects are not refunded. Replaying exact input returns the original result before current
prerequisites; changed input conflicts. Guidance, reports, prices, aliases or restart cannot refill
the ledger. Additional authority is a linked explicit amendment with prior use visible.

An admitted target is immutable. If eligibility changes before dispatch, settle the old attempt
blocked. Any authorized replacement uses a new linked attempt/decision and another admission unit;
it is not a free rewrite of the queued target. A clearly labelled pre-admission proposal is not an
attempt, native worker or budget charge.

**Rationale**: this bounds Council-managed work, preserves lost-acknowledgement safety and gives
one intelligible task budget without asserting control of every native internal model request.

**Alternatives rejected**: per-attempt deadline resets, refunding a failed preparation automatically,
charging only successful launches, overwriting queued targets, and resetting accounting during an
upgrade or explicit retry. No filesystem/vendor work occurs inside the accounting transaction.

## Decision 7: qualify category A; leave category B unsupported on these profiles

**Decision**: keep the specification's two closed continuation categories and require captured
opt-in, remaining authority and every current dispatch guard. For this feature's accepted native
exec profiles, category support is:

| Category | Support and required proof |
| --- | --- |
| A: changed eligibility before native creation | Supported by Council-owned metadata/control facts. A current applicable eligibility observation identifies the change; the old attempt durably settles blocked with verified `not_launched` cleanup and affirmative proof that **no native Prepare/process creation was attempted**. Expected source/control evidence must be complete; intentionally absent worker output is the sole missing-output exception. |
| B: terminal transient native service rejection | **Unsupported** for both accepted 003 exec profiles. Their qualified result surface has generic native failure and no retained, qualified terminal transient discriminator sufficient for this authority. Requesting category B cannot manufacture support. |

For category A, replacement must still fit pins, target-change/escalation permission, remaining
units/deadline, current version, capacity, payment, Stop and global fence. Native creation includes
the preparation/guardian/job phase, not only vendor GO: a prepare/start error cannot be described
as known no-process work merely because no model response appeared. Any ownership/storage/identity
uncertainty blocks continuation. Preserve the original blocked attempt and its charged unit.

An explicit policy request enabling category B on these profiles blocks with
`capability_unavailable`; it is not silently ignored or made successful by a fixture.

For category B, Codex app-server typed errors are a different transport. Claude's documented
`system/api_retry.error` is progress, not terminal proof; a generic `rate_limit`/HTTP 429 does not
distinguish temporary throttling from all quota/billing cases. Existing terminal error fields do
not supply qualified transient authority simply because supporting SDK types have useful names.
[Codex app-server errors](https://learn.chatgpt.com/docs/app-server),
[Claude retry events](https://code.claude.com/docs/en/headless#handle-api-retries),
[Claude error distinctions](https://code.claude.com/docs/en/errors).

Do not add a fake-only production capability or enabled-B success fixture that real supported
profiles cannot execute. Test category-B refusal and misleading/progress/prose cases; qualify
enabled continuation through category A using actual owned local workers and real control paths.
A later profile extension would need a documented, versioned native terminal shape, qualified
parser/fixture evidence and complete root/input/output/cleanup proof before enabling B.

**Rationale**: a finite retry budget is not proof that a retry is justified. Missing output, an
HTTP number, a native text suggestion or a policy-provided reason string cannot create authority.

**Alternatives rejected**: retrying all errors, reconstructing a cause from prose, treating model
access/authentication/exhaustion as temporary overload, retrying while native internal retries are
still active, and importing feature 005's not-yet-built verifier/correction authority.

## Decision 8: preserve exact observations and show their scope

**Decision**: keep managed admission/launch counters exact and separate from native token/cost
observations. Counts remain validated canonical nonnegative decimal strings backed by bounded
integers, never floating-point JSON counters. Preserve valid native estimate lexemes exactly,
including extreme finite decimals, without parsing them into `float64` or evaluating unbounded
exponents. Unsupported/missing telemetry remains null/unknown or explicitly partial.

Retain failed/cancelled/interrupted attempts and coverage gaps. Do not add overlapping top-level
and whole-tree totals or duplicate observations. Reference prices carry units/currency/source/
read time/validity; comparisons need a separately bounded comparable decimal representation.
Otherwise use a labelled declared preference and unknown comparison, not lossy arithmetic or
invented savings. Raw output and bounded typed preview provenance remain retrievable.

Codex's estimated message ranges are not fixed quota, and model/speed affect consumption.
[Codex pricing](https://learn.chatgpt.com/docs/pricing). Claude's session dollars are client
estimates rather than a subscription bill, and its native estimated-budget flag may overshoot;
turn limits do not enumerate every model call. [Claude costs](https://code.claude.com/docs/en/costs),
[CLI limits](https://code.claude.com/docs/en/cli-reference). Native internal retries, fallback,
subagents and configured overage remain distinct from Council continuation and its managed budget.
Strict effective-model/account or financial guarantees unsupported by the interface block.

**Rationale**: exact accounting and useful policy ranking do not require pretending that native
clients provide billing receipts, complete call counts or remaining subscription balance.

**Alternatives rejected**: dollar estimates as charges, missing tokens as zero, call counts as
quota decrements, decimal rounding through JavaScript numbers, summing overlapping scopes, and
percentage savings without a measured comparable workload.

## Decision 9: preserve legacy authority and historical evidence

**Decision**: old 003 tasks remain inspectable/controllable and exact old requests replay unchanged.
Any new attempt requires explicit 004 adoption first, naming additional permitted work and a new
window while retaining known prior use. Migration grants no new budget or routing authority,
rewrites no old Auto/Codex meaning and does not change retained receipt bodies or observations.
New/adopted work captures policy/catalogue references and immutable decision evidence.

**Rationale**: the same old request cannot acquire new economic authority because current prices,
reports or defaults changed. Current eligibility and historical choice are separate views.

**Alternatives rejected**: silently applying new Auto to stored pins, deriving budget permission
from a storage default, clearing prior admissions, migrating behind a live old owner's lock, or
replaying native calls to rebuild missing routing history. Existing exact-owner maintenance,
privacy/sidecar preflight, WAL/FULL durability and failure/cleanup fences remain in force.

## Planning handoff and qualification

The decisions above resolve research choices without adding application source. Root owns the
policy rubric/plan; the data model and contracts own final field names. Nonbinding conceptual
members are profile/reference revision, literal selector, declared role/tier, source/read date,
version assumption, unknown eligibility, observation/receipt time, declared account association,
budget amendment, predecessor and immutable decision evidence. No new SDK/API capability follows
from a suggested member name.

Tasks must cover exact six-profile selectors and unknown/version-negative cases; deterministic
unknown-suitability/general fallback; busy-Codex/available-Claude reports; alias pools and stale
exhaustion; replay/reservation races; explicit legacy adoption; category-A opt-in replacement and
category-B unsupported refusal; source/evidence/cleanup/Stop failures; exact usage and UI/CLI parity.
Use labelled local fixtures and actual owned native children. Final build/test/check, independent
review and all supported native/race gates remain required. No local fixture or platform pass is
a live provider access, account-availability, billing-cap or measured savings claim.
