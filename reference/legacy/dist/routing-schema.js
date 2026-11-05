import { objectSchema, parseSchema } from './schema.js';
import { REASONING_EFFORTS } from './types.js';
import { BUILT_IN_SKILL_IDS } from './skills.js';
const alias = { type: 'string', minLength: 1, maxLength: 64 };
const positiveInteger = { type: 'integer', minimum: 1, maximum: Number.MAX_SAFE_INTEGER };
const skills = { type: 'array', items: { type: 'string', enum: BUILT_IN_SKILL_IDS } };
export const routingProfileDefinition = objectSchema({
    tier: { type: 'string', enum: ['small', 'standard', 'large'] },
    capabilities: { type: 'array', items: { type: 'string', minLength: 1 }, default: [] },
    costRank: { type: ['number', 'null'], exclusiveMinimum: 0, default: null },
    qualityRank: { type: ['number', 'null'], exclusiveMinimum: 0, default: null },
}, ['tier']);
export const routingDefinition = objectSchema({
    task: { type: 'string', minLength: 1 },
    taskClass: { type: 'string', enum: ['routine', 'standard', 'complex'] },
    strategy: { type: 'string', enum: ['economy', 'balanced', 'quality'], default: 'economy' },
    collaboration: { type: 'string', enum: ['direct', 'review', 'parallel', 'council'], default: 'direct' },
    requiredCapabilities: { type: 'array', items: { type: 'string', minLength: 1 }, default: [] },
    agents: { type: 'array', items: alias, minItems: 1 },
    modelSettings: { type: 'object', additionalProperties: objectSchema({
            model: { type: 'string', minLength: 1, maxLength: 200 }, effort: { type: 'string', enum: REASONING_EFFORTS }, skills,
        }) },
    skills,
    permission: { type: 'string', enum: ['read', 'write'], default: 'read' },
    context: { type: 'string' },
    contextLimit: { type: ['integer', 'null'], minimum: 1, maximum: Number.MAX_SAFE_INTEGER },
    maxCalls: positiveInteger, rounds: positiveInteger, participants: positiveInteger,
    allowEscalation: { type: 'boolean', default: false },
}, ['task', 'taskClass']);
export function parseRoutingInput(value) {
    return parseSchema(routingDefinition, value, 'routing');
}
export function parseRoutingProfile(value) {
    return parseSchema(routingProfileDefinition, value, 'routing profile');
}
//# sourceMappingURL=routing-schema.js.map