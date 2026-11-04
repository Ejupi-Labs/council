import test from 'node:test';
import assert from 'node:assert/strict';
import { objectSchema, parseSchema, ValidationError } from '../dist/schema.js';
import { configSchema, workflowSchema } from '../dist/config.js';

test('configuration defaults are independent mutable results', () => {
  const first = configSchema.parse({});
  first.agents.codex.enabled = false; first.agents.codex.args = ['mutated'];
  const second = configSchema.parse({});
  assert.equal(second.agents.codex.enabled, true); assert.equal(second.agents.codex.args, undefined);
  assert.notEqual(first.agents, second.agents); assert.notEqual(first.agents.codex, second.agents.codex);
});

test('closed configuration and workflow objects reject unknown keys', () => {
  assert.throws(() => configSchema.parse({ surprise: true }), ValidationError);
  assert.throws(() => configSchema.parse({ agents: { peer: { profile: 'codex', surprise: true } } }), ValidationError);
  assert.throws(() => workflowSchema.parse({ mode: 'delegate', task: 'Review this.', agents: ['codex'], surprise: true }), ValidationError);
});

test('prototype-shaped profile names cannot pollute objects', () => {
  for (const key of ['__proto__', 'prototype', 'constructor']) {
    const source = JSON.parse(`{"agents":{"${key}":{"profile":"codex"}}}`);
    assert.throws(() => configSchema.parse(source), ValidationError);
  }
  const parsed = parseSchema({ type: 'object', additionalProperties: { type: 'string' } }, JSON.parse('{"__proto__":"own-data","constructor":"own-label"}'));
  assert.equal(Object.getPrototypeOf(parsed), Object.prototype);
  assert.equal(Object.hasOwn(parsed, '__proto__'), true); assert.equal(parsed.__proto__, 'own-data');
  assert.equal({}.polluted, undefined);
});

test('finite numbers and exact integer bounds are enforced', () => {
  for (const number of [NaN, Infinity, -Infinity, 1.5, 0, 9]) assert.throws(() => configSchema.parse({ maxConcurrency: number }), ValidationError);
  assert.equal(configSchema.parse({ maxConcurrency: 8 }).maxConcurrency, 8);
  assert.equal(configSchema.parse({ maxCalls: 64, maxOutputBytes: 2_097_152 }).maxCalls, 64);
  assert.throws(() => configSchema.parse({ maxCalls: Number.MAX_SAFE_INTEGER + 1 }), ValidationError);
});

test('configured commands and each argument reject NUL without interpreting shell syntax', () => {
  assert.throws(() => configSchema.parse({ agents: { peer: { profile: 'codex', command: 'node\0.exe' } } }), ValidationError);
  assert.throws(() => configSchema.parse({ agents: { peer: { profile: 'codex', command: 'node', args: ['literal', 'bad\0arg'] } } }), ValidationError);
  const parsed = configSchema.parse({ agents: { peer: { profile: 'codex', command: 'node', args: ['literal $() & ;', ''] } } });
  assert.deepEqual(parsed.agents.peer.args, ['literal $() & ;', '']);
});

test('typed JSON schemas enforce depth and node budgets', () => {
  const recursive = { type: 'array' }; recursive.items = recursive;
  let depth = []; for (let index = 0; index < 70; index++) depth = [depth];
  assert.throws(() => parseSchema(recursive, depth), /structural validation limit/);
  assert.throws(() => parseSchema({ type: 'array', items: { type: 'integer' } }, Array.from({ length: 8193 }, () => 1)), /structural validation limit/);
});

test('unconstrained schema positions also enforce depth/node budgets', () => {
  let nested = 'leaf'; for (let index = 0; index < 70; index++) nested = { child: nested };
  assert.throws(() => parseSchema({}, nested), /structural validation limit/);
  assert.throws(() => parseSchema({ type: 'object', additionalProperties: true }, { payload: Array.from({ length: 8193 }, () => 1) }), /structural validation limit/);
});

test('unconstrained parsed objects cannot alias or mutate caller data', () => {
  const source = { nested: { values: [1, 2] } };
  const parsed = parseSchema({}, source); parsed.nested.values.push(3);
  assert.deepEqual(source.nested.values, [1, 2]);
});

test('workflow structure rejects empty tasks while preserving larger teams and assignment maps', () => {
  const input = { mode: 'delegate', task: 'Review this.', agents: ['codex'] };
  assert.equal(workflowSchema.parse(input).permission, 'read');
  assert.throws(() => workflowSchema.parse({ ...input, task: '   ' }), ValidationError);
  assert.equal(workflowSchema.parse({ ...input, agents: Array.from({ length: 9 }, (_, index) => `peer${index}`) }).agents.length, 9);
  assert.equal(Object.keys(workflowSchema.parse({ ...input, assignments: Object.fromEntries(Array.from({ length: 9 }, (_, index) => [`peer${index}`, 'Task'])) }).assignments).length, 9);
});

test('nullable context budgets accept full context by default and reject invalid numeric values', () => {
  const schema = { type: ['integer', 'null'], minimum: 1, default: null };
  assert.equal(parseSchema(schema, undefined), null); assert.equal(parseSchema(schema, null), null);
  for (const value of [1, 999, 64001, 1000000]) assert.equal(parseSchema(schema, value), value);
  for (const value of ['1000', 0, -1, 1.5, false]) assert.throws(() => parseSchema(schema, value), ValidationError);
  const input = { mode: 'delegate', task: 'Review this.', agents: ['codex'] };
  assert.equal(workflowSchema.parse({ ...input, contextLimit: null }).contextLimit, null);
  assert.equal(workflowSchema.parse(input).contextLimit, undefined);
});
