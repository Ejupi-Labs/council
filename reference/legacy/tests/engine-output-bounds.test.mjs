import test from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { WorkflowEngine } from '../dist/engine.js';

const config = { version: 1, backend: 'mock', workspaceRoot: process.cwd(), stateDir: join(process.cwd(), '.council'),
  maxConcurrency: 1, timeoutMs: 1000, maxCalls: 1, agents: { worker: { enabled: true, profile: 'codex' } } };
const input = { mode: 'delegate', task: 'Report the verified offline result.', agents: ['worker'], permission: 'read' };
const engine = (output, budget) => new WorkflowEngine({ ...config, maxOutputBytes: budget },
  { run: async () => ({ agent: 'worker', status: 'completed', output, durationMs: 1 }) }, { saveRun() {}, appendEvent() {} });

function utf8Prefix(value, budget) {
  const bytes = Buffer.from(value);
  if (bytes.length <= budget) return value;
  const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true });
  for (let end = budget; end >= 0; end--) {
    try { return decoder.decode(bytes.subarray(0, end)); }
    catch { /* Only a complete valid UTF-8 prefix is retained. */ }
  }
  assert.fail('The empty prefix is always valid UTF-8.');
}

test('oversized injected output is encoded only within its configured budget and remains failed partial evidence', async () => {
  const output = 'A'.repeat(262144), budget = 64, workflow = engine(output, budget), original = Buffer.from;
  let largestStringEncoding = 0;
  Buffer.from = function (value, ...args) { if (typeof value === 'string') largestStringEncoding = Math.max(largestStringEncoding, value.length); return original.call(this, value, ...args); };
  let result;
  try { result = await workflow.run(input); }
  finally { Buffer.from = original; }
  assert.ok(largestStringEncoding <= budget, `Encoded ${largestStringEncoding} code units with budget ${budget}.`);
  assert.equal(result.status, 'failed'); assert.match(result.error, /output byte limit.*incomplete/);
  assert.equal(result.steps[0].result.output, 'A'.repeat(budget));
});

test('bounded output allocation preserves exact Unicode prefixes and successful results that fit', async () => {
  for (const output of ['ASCII result', 'ééx', '🌍abc', 'a🌍bé', '\uFEFFtext', '漢字カナmix']) {
    for (const budget of [1, 2, 3, 4, 5, 7, 8, 12]) {
      const result = await engine(output, budget).run(input), oversized = Buffer.byteLength(output) > budget;
      assert.equal(result.status, oversized ? 'failed' : 'completed', `${JSON.stringify(output)} / ${budget}`);
      assert.equal(result.steps[0].result.output, utf8Prefix(output, budget));
      assert.ok(Buffer.byteLength(result.steps[0].result.output) <= budget);
      if (!oversized) assert.equal(result.output, output);
    }
  }
});
