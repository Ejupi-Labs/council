import { objectSchema } from './schema.js';
import { BUILT_IN_SKILL_IDS } from './skills.js';
const text = (maximum) => ({ type: 'string', minLength: 1, ...(maximum === undefined ? {} : { maxLength: maximum }), pattern: '^[^\\u0000]+$' });
const nullableText = (maximum) => ({ ...text(maximum), type: ['string', 'null'] });
const count = { type: ['number', 'null'], minimum: 0 };
const rank = { type: ['number', 'null'], exclusiveMinimum: 0 };
export const billingWindowDefinition = objectSchema({ id: text(128), label: text(), unit: text(128),
    limit: count, remaining: count, used: count, resetAt: nullableText(64) }, ['id', 'label', 'unit']);
export const billingSourceDefinition = { ...objectSchema({ kind: { type: 'string', enum: ['manual', 'reported'] }, label: text() }, ['kind', 'label']), type: ['object', 'null'] };
export const billingRoutingDefinition = { ...objectSchema({ tier: { type: 'string', enum: ['small', 'standard', 'large'] },
        capabilities: { type: 'array', items: text(128) }, costRank: rank, qualityRank: rank }, ['tier']), type: ['object', 'null'] };
export const billingPricesDefinition = { ...objectSchema({ currency: nullableText(16), inputPerMillion: count, outputPerMillion: count }), type: ['object', 'null'] };
export const billingDefinition = objectSchema({ mode: { type: 'string', enum: ['unknown', 'api', 'subscription', 'local'] },
    accountKey: nullableText(256), plan: nullableText(), source: billingSourceDefinition,
    observedAt: nullableText(64), validUntil: nullableText(64), windows: { type: 'array', items: billingWindowDefinition },
    prices: billingPricesDefinition, routing: billingRoutingDefinition,
    skills: { type: ['array', 'null'], items: { type: 'string', enum: BUILT_IN_SKILL_IDS } } });
export const billingInputDefinition = billingDefinition;
export const billingUpdateDefinition = billingDefinition;
export const billingImportDefinition = { ...billingDefinition, required: ['source', 'observedAt'] };
//# sourceMappingURL=billing-schema.js.map