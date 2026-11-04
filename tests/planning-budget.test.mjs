import test from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { WorkflowEngine } from '../dist/engine.js';

const config = (overrides = {}) => ({ version: 1, backend: 'mock', workspaceRoot: process.cwd(), stateDir: join(process.cwd(), '.council'),
  maxConcurrency: 2, maxCalls: 100, timeoutMs: 15000, maxOutputBytes: 16384,
  agents: Object.fromEntries(['a', 'b', 'advisor'].map(agent => [agent, { enabled: true, profile: 'codex' }])), ...overrides });
const input = overrides => ({ mode: 'debate', task: 'Use the explicit user budget.', agents: ['a', 'b'], permission: 'read', ...overrides });
function fixture(overrides) {
  const calls = [];
  const runner = { doctor: async () => [], run: async request => { calls.push(request); return { agent: request.agent, status: 'completed', durationMs: 1, output: `Complete result ${calls.length}.` }; } };
  return { calls, engine: new WorkflowEngine(config(overrides), runner, { saveRun() {}, appendEvent() {} }) };
}

test('four debate rounds run within the requested budget and expose a compact exact plan', async () => {
  const { calls, engine } = fixture();
  const task = input({ rounds: 4, maxCalls: 9 });
  const plan = engine.plan(task);
  assert.equal(plan.estimatedCalls, 9); assert.deepEqual(plan.stages, ['4 debate rounds', 'judge synthesis']);
  const result = await engine.run(task);
  assert.equal(result.status, 'completed', result.error); assert.equal(calls.length, 9);
  assert.ok(calls.some(call => call.prompt.includes('You are debater-round-4')));
  assert.equal(result.output, 'Complete result 9.');
});

test('huge rejected round counts report their exact call cost without expanding a stage array', () => {
  const { calls, engine } = fixture({ maxCalls: Number.MAX_SAFE_INTEGER });
  assert.throws(() => engine.plan(input({ rounds: Number.MAX_SAFE_INTEGER })),
    /requires 18014398509481983 calls, exceeding maxCalls 9007199254740991/);
  assert.throws(() => engine.plan(input({ rounds: 1000000000, maxCalls: 100 })), /requires 2000000001 calls/);
  assert.equal(calls.length, 0);
});

test('large approved round budgets remain exact and compact without allocating per-round metadata', () => {
  const { engine } = fixture({ maxCalls: Number.MAX_SAFE_INTEGER });
  const rounds = (Number.MAX_SAFE_INTEGER - 1) / 2;
  const plan = engine.plan(input({ rounds }));
  assert.equal(plan.estimatedCalls, Number.MAX_SAFE_INTEGER);
  assert.deepEqual(plan.stages, [`${rounds} debate rounds`, 'judge synthesis']);
});

test('advisor reservations use exact arithmetic at the safe-integer boundary', () => {
  const { engine } = fixture({ maxCalls: Number.MAX_SAFE_INTEGER });
  const task = input({ mode: 'delegate', agents: ['a'], advisors: { a: { agent: 'advisor', maxRequests: (Number.MAX_SAFE_INTEGER - 1) / 2 } } });
  assert.equal(engine.plan(task).estimatedCalls, Number.MAX_SAFE_INTEGER);
  assert.throws(() => engine.plan({ ...task, advisors: { a: { agent: 'advisor', maxRequests: Number.MAX_SAFE_INTEGER } } }),
    /requires 18014398509481983 calls/);
});

test('unsafe and fractional call, round, context and advisor budgets are rejected before dispatch', () => {
  for (const value of [Number.MAX_SAFE_INTEGER + 1, Infinity, NaN, 1.5, 0, -1]) {
    assert.throws(() => fixture({ maxCalls: value }), /positive safe integer/);
    const { engine, calls } = fixture({ maxCalls: Number.MAX_SAFE_INTEGER });
    assert.throws(() => engine.plan(input({ rounds: value })), /positive safe integer/);
    assert.throws(() => engine.plan(input({ maxCalls: value })), /safe integer/);
    assert.throws(() => engine.plan(input({ contextLimit: value })), /positive safe integer/);
    assert.throws(() => engine.plan(input({ advisors: { a: { agent: 'advisor', maxRequests: value } } })), /positive safe integer/);
    assert.equal(calls.length, 0);
  }
});

test('explicit call budgets above sixty-four permit the complete selected team', async () => {
  const agents = Object.fromEntries(Array.from({ length: 70 }, (_, index) => [`peer${index}`, { enabled: true, profile: 'codex' }]));
  const { engine, calls } = fixture({ agents });
  const task = input({ mode: 'parallel', agents: Object.keys(agents), maxCalls: 70 });
  assert.equal(engine.plan(task).estimatedCalls, 70);
  const result = await engine.run(task);
  assert.equal(result.status, 'completed', result.error); assert.equal(calls.length, 70);
  assert.equal(result.steps.length, 70);
});
