import test from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { NativeRunner } from '../dist/runner.js';

const cwd = fileURLToPath(new URL('..', import.meta.url));
const fixture = fileURLToPath(new URL('./fixtures/fake-antigravity.mjs', import.meta.url));
const config = { version: 1, workspaceRoot: cwd, stateDir: join(cwd, '.council'), backend: 'native', maxConcurrency: 2,
  timeoutMs: 15000, maxOutputBytes: 16384, maxCalls: 8,
  agents: { peer: { enabled: true, profile: 'antigravity', transport: 'native', command: process.execPath, args: [fixture], model: 'explicit-fixture-model' } } };
const request = (prompt, permission = 'read') => ({ agent: 'peer', prompt, cwd, permission, timeoutMs: 15000 });

test('native Antigravity delivers its prompt through stdin and returns verified assistant text', async () => {
  const result = await new NativeRunner(config).run(request('hello'));
  assert.equal(result.status, 'completed', result.error);
  assert.equal(result.output, 'Native Antigravity 🌍 hello');
  assert.equal(result.stopReason, 'end_turn');
});

for (const permission of ['read', 'write']) test(`native Antigravity selects explicit ${permission} flags and preserves the requested model`, async () => {
  const result = await new NativeRunner(config).run(request('args', permission));
  assert.equal(result.status, 'completed', result.error);
  const args = JSON.parse(result.output);
  assert.equal(args[args.indexOf('--input-format') + 1], 'stream-json');
  assert.equal(args[args.indexOf('--output-format') + 1], 'stream-json');
  assert.equal(args[args.indexOf('--mode') + 1], permission === 'read' ? 'plan' : 'accept-edits');
  assert.equal(args[args.indexOf('--model') + 1], 'explicit-fixture-model');
  assert.ok(args.includes('--sandbox'));
  assert.ok(!args.includes('--print') && !args.includes('args') && !args.includes('--dangerously-skip-permissions'));
});

for (const scenario of ['partial', 'mismatch', 'cutoff', 'diagnostics', 'late', 'nonzero']) test(`native Antigravity rejects ${scenario} rather than returning partial success`, async () => {
  const result = await new NativeRunner(config).run(request(scenario));
  assert.equal(result.status, 'failed');
  assert.ok(result.error);
  assert.ok(!JSON.stringify(result).includes('SECRET_ACCESS_TOKEN'));
});

test('native Antigravity provider cancellation remains distinct from failure', async () => {
  const result = await new NativeRunner(config).run(request('cancelled'));
  assert.equal(result.status, 'cancelled');
  assert.equal(result.stopReason, 'cancelled');
  assert.match(result.output, /cancelled/);
});

test('native Antigravity retains safe invalid-update diagnostics without accepting a later successful result', async () => {
  const result = await new NativeRunner(config).run(request('invalid-step'));
  assert.equal(result.status, 'failed');
  assert.match(result.error, /unsupported step state.*state=unsupported-string.*text_delta=string.*tool_info=object/);
  assert.equal(result.output, '');
  assert.equal(JSON.stringify(result).includes('SECRET_ACCESS_TOKEN'), false);
});

test('native Antigravity preserves an ERROR tool denial despite a later SUCCESS envelope', async () => {
  const result = await new NativeRunner(config).run(request('denied-error-step'));
  assert.equal(result.status, 'failed');
  assert.match(result.error, /tool execution failed or was denied.*state=ERROR.*step_type=tool/);
  assert.equal(result.output, '');
  assert.equal(JSON.stringify(result).includes('SECRET_ACCESS_TOKEN'), false);
});

test('native Antigravity retains an actionable denial when the known stderr diagnostic wins the race', async () => {
  const result = await new NativeRunner(config).run(request('native-permission-diagnostic'));
  assert.equal(result.status, 'failed');
  assert.match(result.error, /denied a tool permission request in headless mode.*workspace scope.*native CLI permissions/);
  assert.equal(JSON.stringify(result).includes('SECRET_ACCESS_TOKEN'), false);
});
