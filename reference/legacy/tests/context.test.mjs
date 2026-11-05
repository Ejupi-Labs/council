import test from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { WorkflowEngine } from '../dist/engine.js';
import { configSchema, parseWorkflow } from '../dist/config.js';

const config = (overrides = {}) => ({ version: 1, backend: 'mock', workspaceRoot: process.cwd(), stateDir: join(process.cwd(), '.council'),
  maxConcurrency: 2, timeoutMs: 15000, maxCalls: 20, maxOutputBytes: 131072,
  agents: { first: { enabled: true, profile: 'codex' }, second: { enabled: true, profile: 'claude' }, judge: { enabled: true, profile: 'junie' } }, ...overrides });
const store = () => ({ saveRun() {}, appendEvent() {} });
const input = overrides => ({ mode: 'pipeline', agents: ['first', 'second'], task: 'Preserve every supplied fact.', permission: 'read', ...overrides });
function backend(handler) {
  const calls = [];
  return { calls, doctor: async () => [], run: async call => { calls.push(call); return { agent: call.agent, status: 'completed', durationMs: 1, output: await handler(call, calls.length) }; } };
}

test('default and null context modes preserve tasks over 32k, context over 64k and full earlier output', async () => {
  for (const contextLimit of [undefined, null]) {
    const task = 'Task detail. '.repeat(4000) + 'TASK_END';
    const context = 'Project evidence. '.repeat(8000) + 'PROJECT_END';
    const prior = 'Earlier output. '.repeat(5000) + 'EARLIER_END';
    const runner = backend((call, count) => {
      assert.ok(call.prompt.includes(task)); assert.ok(call.prompt.includes(context));
      if (count === 2) assert.ok(call.prompt.includes(prior));
      assert.ok(!call.prompt.includes('[truncated by Council]'));
      return count === 1 ? prior : 'Verified final result.';
    });
    const parsed = parseWorkflow(input({ task, context, contextLimit }));
    const result = await new WorkflowEngine(config(), runner, store()).run(parsed);
    assert.equal(result.status, 'completed', result.error); assert.equal(runner.calls.length, 2);
    assert.equal(result.steps[0].result.output, prior);
  }
});

test('parallel assignments and council handoffs keep all supplied text instead of allocating excerpts', async () => {
  const assignment = 'Independent assignment detail. '.repeat(2500) + 'ASSIGNMENT_END';
  const project = 'Project detail. '.repeat(7000) + 'PROJECT_END';
  const outputs = new Map();
  const runner = backend((call, count) => {
    if (call.agent !== 'judge') { assert.ok(call.prompt.includes(assignment)); outputs.set(call.agent, `${call.agent}: ` + 'Complete worker evidence. '.repeat(3000) + 'WORKER_END'); return outputs.get(call.agent); }
    assert.ok(call.prompt.includes(project));
    for (const output of outputs.values()) assert.ok(call.prompt.includes(output));
    return 'Complete synthesized answer.';
  });
  const parsed = parseWorkflow(input({ mode: 'parallel', judge: 'judge', context: project, assignments: { first: assignment, second: assignment } }));
  const result = await new WorkflowEngine(config(), runner, store()).run(parsed);
  assert.equal(result.status, 'completed', result.error); assert.equal(runner.calls.length, 3);
});

test('an explicit shared-context budget fails before dispatch and never silently shortens data', async () => {
  const context = 'Complete evidence. '.repeat(100);
  const runner = backend(() => 'Must not run.');
  const result = await new WorkflowEngine(config(), runner, store()).run(input({ mode: 'delegate', agents: ['first'], context, contextLimit: 1 }));
  assert.equal(result.status, 'failed'); assert.equal(runner.calls.length, 0);
  assert.match(result.error, /explicit contextLimit 1.*Increase the budget/);
  assert.equal(result.input.context, context);
});

test('team size follows the configured call budget rather than an eight- or sixteen-agent ceiling', async () => {
  const agents = Object.fromEntries(Array.from({ length: 33 }, (_, index) => [`peer${index}`, { enabled: true, profile: 'codex' }]));
  const parsedConfig = configSchema.parse({ backend: 'mock', maxCalls: 40, maxOutputBytes: 16384, agents });
  const settings = config({ ...parsedConfig, stateDir: join(process.cwd(), '.council') });
  const names = Object.keys(agents), runner = backend(call => `${call.agent}: complete`);
  const parsed = parseWorkflow(input({ mode: 'parallel', agents: names }));
  const engine = new WorkflowEngine(settings, runner, store());
  assert.equal(engine.plan(parsed).estimatedCalls, 33);
  const result = await engine.run(parsed);
  assert.equal(result.status, 'completed', result.error); assert.equal(runner.calls.length, 33);
  assert.throws(() => engine.plan({ ...parsed, maxCalls: 32 }), /requires 33 calls/);
});
