import { objectSchema, type JsonSchema } from './schema.js';

const text: JsonSchema = { type: 'string', minLength: 1, pattern: '^[^\\u0000]+$' };
const id: JsonSchema = { ...text, maxLength: 100, pattern: '^[a-zA-Z0-9_-]+$' };
const cursor: JsonSchema = { type: 'integer', minimum: 0, maximum: Number.MAX_SAFE_INTEGER, default: 0 };
export const privateChatCreateDefinition = objectSchema({
  participants: { type: 'array', items: text, minItems: 2, maxItems: 2 }, roomId: id,
}, ['participants']);
export const privateChatSendDefinition = objectSchema({
  chatId: id, from: text, body: text, runId: { ...text, maxLength: 256 }, idempotencyKey: text,
}, ['chatId', 'from', 'body']);
export const privateChatPageDefinition = objectSchema({
  chatId: id, actor: text, after: cursor, limit: { type: 'integer', minimum: 1, maximum: 100, default: 100 },
}, ['chatId', 'actor']);
export const privateChatWatchDefinition = objectSchema({
  chatId: id, actor: text, after: cursor,
  timeoutMs: { type: 'integer', minimum: 0, maximum: Number.MAX_SAFE_INTEGER, default: 600000 },
}, ['chatId', 'actor']);
export const privateChatListDefinition = objectSchema({ actor: text, roomId: id }, ['actor']);
export const privateChatActorsDefinition = objectSchema({ roomId: id });
export const privateChatGetDefinition = objectSchema({ chatId: id, actor: text }, ['chatId', 'actor']);
