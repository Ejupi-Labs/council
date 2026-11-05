import { objectSchema, type JsonSchema } from './schema.js';

const text: JsonSchema = { type: 'string', minLength: 1, pattern: '^[^\\u0000]+$' };
const strings: JsonSchema = { type: 'array', items: text };
const positiveInteger: JsonSchema = { type: 'integer', minimum: 1, maximum: Number.MAX_SAFE_INTEGER };
export const modelMapRoleDefinition: JsonSchema = { type: 'string', enum: ['lead', 'worker', 'reviewer', 'advisor', 'economy'] };
export const modelMapSourceDefinition = objectSchema({
  id: text, title: text, url: text, retrievedAt: text, publisher: text,
  type: { type: 'string', enum: ['benchmark-owner', 'vendor', 'methodology'] }, publishedAt: text,
}, ['id', 'title', 'url', 'retrievedAt', 'publisher', 'type']);
export const modelMapObservationDefinition = objectSchema({
  id: text, sourceId: text, modelId: text, benchmark: text, version: text, metric: text, score: { type: 'number' },
  unit: text, direction: { type: 'string', enum: ['higher', 'lower'] }, harness: text,
  effort: { type: ['string', 'null'], minLength: 1 }, cohort: text, notes: text, sampleSize: positiveInteger, uncertainty: text,
}, ['id', 'sourceId', 'modelId', 'benchmark', 'version', 'metric', 'score', 'unit', 'direction', 'harness', 'effort', 'cohort']);
export const modelMapRecommendationDefinition = objectSchema({
  task: text, priority: positiveInteger, role: modelMapRoleDefinition,
  basis: { type: 'string', enum: ['benchmark', 'vendor-guidance', 'policy-inference'] },
  confidence: { type: 'string', enum: ['high', 'medium', 'low'] }, reason: text,
  sourceIds: { ...strings, minItems: 1 },
}, ['task', 'priority', 'role', 'basis', 'confidence', 'reason', 'sourceIds']);
export const modelMapProfileDefinition = objectSchema({
  id: text, name: text, provider: text, aliases: strings, status: { type: 'string', enum: ['verified', 'unverified', 'legacy'] },
  strengths: strings, limitations: strings, evidenceIds: strings,
  recommendations: { type: 'array', items: modelMapRecommendationDefinition }, effortNotes: text, harnesses: strings,
  referencePricing: objectSchema({ inputPerMillion: { type: 'number', minimum: 0 }, outputPerMillion: { type: 'number', minimum: 0 },
    currency: text, sourceId: text }, ['inputPerMillion', 'outputPerMillion', 'currency', 'sourceId']),
}, ['id', 'name', 'provider', 'status', 'strengths', 'limitations', 'evidenceIds', 'recommendations']);
export const modelMapTaskDefinition = objectSchema({ id: text, label: text, description: text }, ['id', 'label', 'description']);
const records = {
  sources: { type: 'array', items: modelMapSourceDefinition } as JsonSchema,
  observations: { type: 'array', items: modelMapObservationDefinition } as JsonSchema,
  models: { type: 'array', items: modelMapProfileDefinition } as JsonSchema,
  tasks: { type: 'array', items: modelMapTaskDefinition } as JsonSchema,
};
export const modelMapDefinition = objectSchema({ asOf: text, revision: positiveInteger, ...records }, ['asOf', 'revision', 'sources', 'observations', 'models', 'tasks']);
export const modelMapOverlayDefinition = objectSchema(records);
export const modelMapQueryDefinition = objectSchema({ task: text, model: text, role: modelMapRoleDefinition, harness: text, includeFullCatalog: { type: 'boolean' } });
