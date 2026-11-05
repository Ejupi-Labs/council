import { objectSchema, type JsonSchema } from './schema.js';
import { BUILT_IN_SKILL_IDS } from './skills.js';

const text = (minLength: number, maxLength: number): JsonSchema => ({ type: 'string', minLength, maxLength });
export const effortDefinition: JsonSchema = { type: 'string', enum: ['low', 'medium', 'high', 'xhigh', 'max'] };
export const roomMessageDefinition = objectSchema({
  author: { type: 'string', minLength: 1, maxLength: 80, pattern: '^[^\\u0000]+$' },
  body: { type: 'string', minLength: 1, pattern: '^[^\\u0000]+$' },
}, ['author', 'body']);
export const roomTaskDefinition = objectSchema({ title: text(1, 200), brief: { type: 'string', minLength: 1 },
  permission: { type: 'string', enum: ['read', 'write'], default: 'read' },
  dependsOn: { type: 'array', items: text(1, 256) },
}, ['title', 'brief']);
export const roomMemberDefinition = objectSchema({ agent: text(1, 64),
  role: { type: 'string', enum: ['lead', 'worker', 'advisor'], default: 'worker' },
  weight: { type: 'number', exclusiveMinimum: 0 }, model: text(1, 200),
  effort: effortDefinition, advisor: text(1, 64), advisorMaxRequests: { type: 'integer', minimum: 1 },
  skills: { type: 'array', items: { type: 'string', enum: BUILT_IN_SKILL_IDS } },
}, ['agent']);
export const roomDefinition = objectSchema({ title: text(1, 200), brief: { type: 'string', minLength: 1 },
  members: { type: 'array', items: roomMemberDefinition, minItems: 1 },
  tasks: { type: 'array', items: roomTaskDefinition },
  rounds: { type: 'integer', minimum: 1, default: 1 },
  maxCalls: { type: 'integer', minimum: 1 },
  autoExecute: { type: 'boolean', default: true },
  contextLimit: { type: ['integer', 'null'], minimum: 1, default: null },
  privateChatMaxRequests: { type: 'integer', minimum: 1, maximum: Number.MAX_SAFE_INTEGER },
}, ['title', 'brief', 'members']);
