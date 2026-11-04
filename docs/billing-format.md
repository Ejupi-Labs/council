# Billing observations and routing metadata

Council stores user-entered billing information in `.council/billing.sqlite`. It does not contact providers, inspect credentials, infer a subscription allowance, or decrement a quota after a workflow. All configured agent aliases have a record; an untouched record has revision `0`, mode `unknown`, and unknown capacity.

`BillingManager.update(alias, input, expectedRevision?)` accepts partial edits. `importSnapshot` accepts the same fields but requires an explicit `reported` source and `observedAt`. The following values illustrate the format; replace them with the account information and timestamps actually observed:

```json
{
  "mode": "subscription",
  "stopAtSubscriptionLimit": true,
  "subscriptionWindowIds": ["configured-window"],
  "accountKey": "team-subscription",
  "plan": "User-entered plan label",
  "source": { "kind": "manual", "label": "Account owner observation" },
  "observedAt": "2030-01-15T12:00:00.000Z",
  "validUntil": "2030-01-15T13:00:00.000Z",
  "windows": [
    {
      "id": "configured-window",
      "label": "User-configured request window",
      "unit": "requests",
      "scope": "subscription",
      "limit": 100,
      "used": 10,
      "remaining": 90,
      "resetAt": "2030-01-16T12:00:00.000Z"
    }
  ],
  "prices": {
    "currency": null,
    "inputPerMillion": null,
    "outputPerMillion": null
  },
  "routing": {
    "tier": "standard",
    "capabilities": ["implementation", "review"],
    "costRank": 2,
    "qualityRank": 3
  },
  "skills": null
}
```

Billing modes are `unknown`, `api`, `subscription`, and `local`. Neither a mode nor a plan label establishes an allowance. Unknown prices and limits use `null`; zero is an explicit value. Prices are user-configured amounts per one million input or output tokens. They are not estimates of the next task's cost. A partial prices edit preserves its other configured price fields.

Reference rates describe the alias you configured. A per-run model override does not look up replacement prices or turn those rates into a measured charge.

Every nonempty replacement `windows` list requires an explicit observation timestamp. Metadata edits retain the existing timestamp, so changing a price, skill, or routing tier cannot make an old quota look newly observed. Report imports are explicit normalized data: a source label records provenance but does not establish which CLI account the observation describes.

`accountKey` is a non-secret grouping label. Aliases with the same key reference one stored account and its limit windows. Editing that account updates every linked alias and increments all affected revisions. A link-only update uses the existing target account; a previously unused key starts with unknown billing information. Setting `accountKey` to `null` selects an independent alias account without copying another account's quota. Prices, routing, skill overrides and subscription-protection choices remain local to each alias. Linking or unlinking an account preserves that alias's own prices; a price-only or protection-only edit does not refresh quota observations or invalidate another alias's revision.

Pass the revision returned by `get` when saving an editor. A stale revision throws `BillingRevisionConflict` with status `409`, code `COUNCIL_BILLING_REVISION_CONFLICT`, and the current revision. `remove` clears the alias's billing link, prices, routing, and skill overrides while advancing its revision. Other aliases retain their shared account. Enabled subscription protection and its retained exhaustion evidence survive removal; missing allowance data blocks calls until restored or the user explicitly disables protection.

Billing schema 3 adds alias-local subscription-protection settings. Schema 2 moved reference prices from shared accounts to aliases. Stop older Council hosts before upgrading. The transactional migration handles either previous version, copying version-1 shared prices to linked aliases and preserving amounts, revisions and quota provenance. Review copied rates for aliases using different models. The separate workflow and room databases keep their own schema versions.

`availability(alias, now?)` evaluates the observation without changing it:

- A valid observation needs a known source, a timestamp that is not in the future, and a future `validUntil` or window `resetAt`.
- A passed reset or validity date makes that observation unknown. Council never assumes that a reset replenished the account.
- Remaining capacity comes from an explicit remaining value or from observed limit minus observed usage. Missing counters remain unknown.
- A current zero remaining value means exhausted. An obsolete zero remains historical data for ordinary availability; enabled subscription protection retains its stop condition as described below.
- Every window must show current positive capacity for the account to be available. Any current exhausted window is sufficient to mark it exhausted; otherwise incomplete evidence remains unknown.

For subscription accounts, that aggregate uses subscription-scoped windows. Paid-credit and other measurements remain visible individually, but an empty paid-credit balance cannot block an available included allowance or make exhausted included allowance available.

The record retains the exact stored counters even when availability is unknown. Routing tier, capabilities, and positive finite cost/quality ranks are user choices. Unknown ranks remain `null`. Skill overrides use built-in skill IDs: `null` inherits the agent configuration, and `[]` explicitly disables its inherited skills.

Billing views, routing and admission receipts evaluate availability from the same captured observation as their displayed counters and metadata. A concurrent account update appears on the next read rather than mixing two revisions in one view. `availabilityFor(record, now?)` classifies a supplied record without another database read; `availability(alias, now?)` explicitly reads the alias first. Neither operation changes a quota or measures spending.

## Stop at included subscription limits

Enable `stopAtSubscriptionLimit` for each alias that must stop rather than consume paid credits. It defaults to `false` for existing configurations. A window's `scope` distinguishes included `subscription` allowance, paid `credits`, and `other` measurements. Legacy windows in a subscription record default to `subscription`; legacy windows in other billing modes default to `other`. Check imported window classifications before enabling protection.

`subscriptionWindowIds` optionally selects specific included allowance windows. `null` and `[]` select every recorded subscription window, including retained exhaustion evidence. Credit windows never provide included capacity. Every selected window must have a current positive observation; unknown counters, missing selected IDs, future timestamps, expired validity, passed resets or a non-subscription billing mode block calls. Policy edits preserve the original observation source, time and counters.

An observed zero, including observed limit minus usage reaching zero, retains an exhaustion timestamp on the shared account. A reset date passing does not clear it. Older or stale positive reports cannot clear it either: a current positive observation at least as new as the exhaustion observation is required. `subscriptionExhausted` exposes this retained evidence for review and is read-only. Shared aliases see the same quota evidence while keeping independent enable/disable choices.

`subscriptionProtection(alias, now?)` reads and evaluates the current policy; `subscriptionProtectionFor(record, now?)` classifies a captured record without another read. The receipt reports `enabled`, `allowed`, `status`, selected `windowIds` and complete `reasons`. A blocked actual dispatch records a `step.dispatch_blocked` event with code `COUNCIL_SUBSCRIPTION_LIMIT` and its receipt. The workflow remains failed, with partial evidence retained.

Council checks the live policy after acquiring provider concurrency slots, covering direct runs, room discussion and tasks, advisors, private peer exchanges, judges and continuations. While a provider call is active it checks again every 250 ms; newly blocking reports or enabling protection with unknown capacity abort that call's signal and prevent its partial output from counting as success. Routing also excludes a protected alias whose capacity cannot be verified.

This guard uses explicit reports supplied to Council; it does not probe providers, inspect credentials or estimate quota decrements. An observation can lag actual provider consumption, and a vendor harness may make internal requests before Council receives a blocking report or processes cancellation. Enabling the guard therefore does not establish an absolute provider spending cap, change a provider's own paid-credit setting, or control calls made outside Council. For a provider-enforced cap, also configure that account's own credit/spending controls where available.
