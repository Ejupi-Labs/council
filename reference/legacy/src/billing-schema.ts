import { objectSchema, type JsonSchema } from './schema.js';
import { BUILT_IN_SKILL_IDS } from './skills.js';

const text = (maximum?: number): JsonSchema => ({ type: 'string', minLength: 1, ...(maximum === undefined ? {} : { maxLength: maximum }), pattern: '^[^\\u0000]+$' });
const nullableText = (maximum?: number): JsonSchema => ({ ...text(maximum), type: ['string', 'null'] });
const count: JsonSchema = { type: ['number', 'null'], minimum: 0 };
const rank: JsonSchema = { type: ['number', 'null'], exclusiveMinimum: 0 };
export const billingWindowDefinition = objectSchema({ id: text(128), label: text(), unit: text(128),
  scope: { type: 'string', enum: ['subscription', 'credits', 'other'] },
  limit: count, remaining: count, used: count, resetAt: nullableText(64) }, ['id', 'label', 'unit']);
export const billingSourceDefinition: JsonSchema = { ...objectSchema({ kind: { type: 'string', enum: ['manual', 'reported'] }, label: text() }, ['kind', 'label']), type: ['object', 'null'] };
export const billingRoutingDefinition: JsonSchema = { ...objectSchema({ tier: { type: 'string', enum: ['small', 'standard', 'large'] },
  capabilities: { type: 'array', items: text(128) }, costRank: rank, qualityRank: rank }, ['tier']), type: ['object', 'null'] };
export const billingPricesDefinition: JsonSchema = { ...objectSchema({ currency: nullableText(16), inputPerMillion: count, outputPerMillion: count }), type: ['object', 'null'] };
export const billingDefinition = objectSchema({ mode: { type: 'string', enum: ['unknown', 'api', 'subscription', 'local'] },
  accountKey: nullableText(256), plan: nullableText(), source: billingSourceDefinition,
  observedAt: nullableText(64), validUntil: nullableText(64), windows: { type: 'array', items: billingWindowDefinition },
  prices: billingPricesDefinition, routing: billingRoutingDefinition,
  stopAtSubscriptionLimit: { type: 'boolean' }, subscriptionWindowIds: { type: ['array', 'null'], items: text(128) },
  skills: { type: ['array', 'null'], items: { type: 'string', enum: BUILT_IN_SKILL_IDS } } });
export const billingInputDefinition = billingDefinition;
export const billingUpdateDefinition = billingDefinition;
export const billingImportDefinition: JsonSchema = { ...billingDefinition, required: ['source', 'observedAt'] };
