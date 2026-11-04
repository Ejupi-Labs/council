import { objectSchema } from './schema.js';
const text = (minimum = 0) => ({ type: 'string', minLength: minimum, pattern: '^[^\\u0000]*$' });
const revision = { type: 'integer', minimum: 0, maximum: Number.MAX_SAFE_INTEGER };
const roomId = { type: 'string', minLength: 1, maxLength: 100, pattern: '^[a-zA-Z0-9_-]+$' };
export const delegationDefinition = objectSchema({
    mode: { type: 'string', enum: ['human', 'agent'] }, delegate: text(), mandate: text(),
    roomIds: { type: 'array', items: roomId }, expectedRevision: revision,
}, ['mode', 'delegate', 'mandate', 'roomIds', 'expectedRevision']);
export const delegationConfigureDefinition = delegationDefinition;
export const delegationReportDefinition = objectSchema({
    author: text(1), kind: { type: 'string', enum: ['update', 'decision', 'blocked', 'completion'] },
    body: text(1), roomIds: { type: 'array', items: roomId, default: [] },
    expectedRoomRevisions: { type: 'object', propertyNames: { pattern: '^[a-zA-Z0-9_-]{1,100}$' }, additionalProperties: revision },
}, ['author', 'kind', 'body']);
export const delegationResolveDefinition = objectSchema({
    author: text(1), body: text(1), expectedRevision: { ...revision, minimum: 1 },
}, ['author', 'body', 'expectedRevision']);
export const delegationCursorDefinition = objectSchema({
    workspaceId: { type: 'string', minLength: 1 },
    configurationRevision: revision, reportRevision: revision,
    rooms: { type: 'object', propertyNames: { pattern: '^[a-zA-Z0-9_-]{1,100}$' },
        additionalProperties: objectSchema({ revision, instructionRevision: revision }, ['revision', 'instructionRevision']) },
}, ['workspaceId', 'configurationRevision', 'reportRevision', 'rooms']);
export const delegationWatchDefinition = objectSchema({
    cursor: delegationCursorDefinition, timeoutMs: { ...revision, default: 600000 },
});
//# sourceMappingURL=delegation-schema.js.map