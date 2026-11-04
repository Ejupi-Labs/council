# Billing observations and routing metadata

Council stores user-entered billing information in `.council/billing.sqlite`. It does not contact providers, inspect credentials, infer a subscription allowance, or decrement a quota after a workflow. All configured agent aliases have a record; an untouched record has revision `0`, mode `unknown`, and unknown capacity.

`BillingManager.update(alias, input, expectedRevision?)` accepts partial edits. `importSnapshot` accepts the same fields but requires an explicit `reported` source and `observedAt`. The following values illustrate the format; replace them with the account information and timestamps actually observed:

```json
{
  "mode": "subscription",
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

Every nonempty replacement `windows` list requires an explicit observation timestamp. Metadata edits retain the existing timestamp, so changing a price, skill, or routing tier cannot make an old quota look newly observed. Report imports are explicit normalized data: a source label records provenance but does not establish which CLI account the observation describes.

`accountKey` is a non-secret grouping label. Aliases with the same key reference one stored account and its limit windows. Editing that account updates every linked alias and increments all affected revisions. A link-only update uses the existing target account; a previously unused key starts with unknown billing information. Setting `accountKey` to `null` selects an independent alias account without copying another account's quota. Routing and skill overrides remain local to each alias.

Pass the revision returned by `get` when saving an editor. A stale revision throws `BillingRevisionConflict` with status `409`, code `COUNCIL_BILLING_REVISION_CONFLICT`, and the current revision. `remove` clears the alias's billing link, routing, and skill overrides while advancing its revision. Other aliases retain their shared account. An account with no remaining links is removed.

`availability(alias, now?)` evaluates the observation without changing it:

- A valid observation needs a known source, a timestamp that is not in the future, and a future `validUntil` or window `resetAt`.
- A passed reset or validity date makes that observation unknown. Council never assumes that a reset replenished the account.
- Remaining capacity comes from an explicit remaining value or from observed limit minus observed usage. Missing counters remain unknown.
- A current zero remaining value means exhausted. An obsolete zero value remains historical data and does not block routing as current exhaustion.
- Every window must show current positive capacity for the account to be available. Any current exhausted window is sufficient to mark it exhausted; otherwise incomplete evidence remains unknown.

The record retains the exact stored counters even when availability is unknown. Routing tier, capabilities, and positive finite cost/quality ranks are user choices. Unknown ranks remain `null`. Skill overrides use built-in skill IDs: `null` inherits the agent configuration, and `[]` explicitly disables its inherited skills.
