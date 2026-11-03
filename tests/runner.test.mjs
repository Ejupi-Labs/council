import test from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { AcpxRunner, NativeRunner, MockRunner, createRunner } from '../dist/runner.js';

const fixture = fileURLToPath(new URL('./fixtures/fake-acpx.mjs', import.meta.url));
const cwd = fileURLToPath(new URL('..', import.meta.url));
const config = overrides => ({ version: 1, workspaceRoot: cwd, stateDir: join(cwd, '.council'), backend: 'acpx', maxConcurrency: 2,
  timeoutMs: 5000, maxOutputBytes: 16_384, maxCalls: 8, agents: { codex: { enabled: true, profile: 'codex' }, off: { enabled: false, profile: 'claude' } }, ...overrides });
const request = (prompt, overrides) => ({ agent: 'codex', prompt, cwd, permission: 'read', timeoutMs: 5000, ...overrides });
const runner = overrides => new AcpxRunner(config(overrides), { acpxEntrypoint: fixture });

test('ACPX parses only assistant text and preserves Unicode split across process chunks', async () => {
  const result = await runner().run(request('ciao'));
  assert.equal(result.status, 'completed'); assert.equal(result.output, 'Hello 🌍: ciao'); assert.equal(result.stopReason, 'end_turn');
});

test('ACPX passes prompt over stdin and explicit denial policy for read runs', async () => {
  const result = await runner().run(request('args'));
  const args = JSON.parse(result.output);
  assert.ok(args.includes('--deny-all')); assert.ok(args.includes('--json-strict')); assert.ok(args.includes('--file'));
  assert.equal(args[args.indexOf('--permission-policy') + 1], '{"defaultAction":"deny"}');
  assert.deepEqual(args.slice(-4), ['codex', 'exec', '--file', '-']); assert.ok(!args.includes('args'));
});

test('write permission and model are explicit argv values', async () => {
  const result = await runner().run(request('args', { permission: 'write', model: 'custom-model' }));
  const args = JSON.parse(result.output);
  assert.ok(args.includes('--approve-all')); assert.ok(!args.includes('--deny-all'));
  assert.equal(args[args.indexOf('--model') + 1], 'custom-model');
});

for (const scenario of ['malformed', 'error', 'oversize', 'stderr-oversize', 'answer-oversize', 'nonzero', 'partial', 'empty', 'max-tokens', 'wrong-id', 'late-text']) {
  test(`ACPX rejects ${scenario} instead of reporting partial success`, async () => {
    const result = await runner().run(request(scenario));
    assert.equal(result.status, 'failed'); assert.ok(result.error); assert.ok(!JSON.stringify(result).includes('SECRET_ACCESS_TOKEN'));
  });
}

test('large echoed prompts do not consume the assistant-answer byte allowance', async () => {
  const result = await runner({ maxOutputBytes: 1024 }).run(request(`large-input:${'x'.repeat(32_000)}`));
  assert.equal(result.status, 'completed'); assert.equal(result.output, 'Large input accepted.');
});

test('startup dependency failures provide an actionable summary without leaking provider diagnostics', async () => {
  const result = await runner().run(request('dependency-error'));
  assert.equal(result.status, 'failed'); assert.match(result.error, /dependency is unavailable from npm/);
  assert.ok(!JSON.stringify(result).includes('SECRET_ACCESS_TOKEN'));
});

test('a complete known provider failure envelope cannot masquerade as successful end_turn', async () => {
  const result = await runner().run(request('provider-model-error'));
  assert.equal(result.status, 'failed'); assert.match(result.error, /provider rejected the configured model/);
  assert.equal(result.output, ''); assert.ok(!JSON.stringify(result).includes('SECRET_ACCESS_TOKEN'));
});

test('ordinary review text containing provider-error examples remains a successful answer', async () => {
  const result = await runner().run(request('provider-error-example'));
  assert.equal(result.status, 'completed'); assert.match(result.output, /model_not_found/);
});

test('child environments increment Council depth to prevent recursive delegation', async () => {
  const local = new AcpxRunner(config(), { acpxEntrypoint: fixture, environment: { ...process.env, COUNCIL_DEPTH: '3' } });
  const result = await local.run(request('depth'));
  assert.equal(result.status, 'completed'); assert.equal(result.output, '4');
});

test('direct ACP executes a local adapter with explicit model and permission handling', async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-acp-smoke-'));
  try {
    const adapter = fileURLToPath(new URL('./fixtures/fake-acp-adapter.mjs', import.meta.url));
    const local = new NativeRunner(config({ backend: 'native', agents: { codex: { enabled: true, profile: 'codex', transport: 'acp', command: process.execPath, args: [adapter] } } }), { environment: { ...process.env, COUNCIL_DEPTH: '0' } });
    const read = await local.run(request('A local fixture smoke test; no model calls.', { cwd: temporary, timeoutMs: 15_000 }));
    assert.equal(read.status, 'completed', read.error); assert.match(read.output, /depth=1/);
    const denied = await local.run(request('request-write-permission', { cwd: temporary, timeoutMs: 15_000 }));
    assert.equal(denied.status, 'failed'); assert.match(denied.error, /permission was denied/);
    const write = await local.run(request('request-write-permission', { cwd: temporary, permission: 'write', model: 'fixture-alternate', timeoutMs: 15_000 }));
    assert.equal(write.status, 'completed', write.error); assert.match(write.output, /"optionId":"allow"/);
  } finally { await rm(temporary, { recursive: true, force: true }); }
});

test('ACPX preserves an agent cancellation terminal state', async () => {
  assert.equal((await runner().run(request('cancelled'))).status, 'cancelled');
});

test('ACPX timeout kills a hung process tree', async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-runner-'));
  const pidFile = join(temporary, 'child.pid');
  try {
    const local = new AcpxRunner(config(), { acpxEntrypoint: fixture, environment: { ...process.env, COUNCIL_TEST_PID_FILE: pidFile } });
    const result = await local.run(request('tree-hang', { timeoutMs: 700 }));
    assert.equal(result.status, 'timeout');
    const pid = Number(await readFile(pidFile, 'utf8'));
    await new Promise(resolve => setTimeout(resolve, 100));
    assert.throws(() => process.kill(pid, 0), /ESRCH|EPERM/);
  } finally { await rm(temporary, { recursive: true, force: true }); }
});

test('ACPX supports pre-aborted and active cancellation', async () => {
  const already = new AbortController(); already.abort();
  assert.equal((await runner().run(request('hang', { signal: already.signal }))).status, 'cancelled');
  const controller = new AbortController();
  const running = runner().run(request('hang', { signal: controller.signal }));
  setTimeout(() => controller.abort(), 100);
  assert.equal((await running).status, 'cancelled');
});

test('ACPX handles unavailable entrypoints and disabled agents without throwing', async () => {
  const missing = new AcpxRunner(config(), { acpxEntrypoint: join(cwd, 'does-not-exist.mjs') });
  assert.equal((await missing.run(request('hello'))).status, 'failed');
  assert.equal((await runner().run(request('hello', { agent: 'off' }))).status, 'failed');
});

test('doctor checks an external ACPX path without invoking agents', async () => {
  const local = new AcpxRunner(config(), { acpxEntrypoint: fixture, environment: { PATH: '' } });
  const health = await local.doctor();
  assert.equal(health[0].available, true); assert.match(health[0].detail, /External ACPX executable found/);
  assert.equal(health[1].enabled, false);
});

test('mock backend is deterministic, offline and respects disabled/aborted requests', async () => {
  const local = createRunner(config({ backend: 'mock' })); assert.ok(local instanceof MockRunner);
  const first = await local.run(request('task')); const second = await local.run(request('task'));
  assert.deepEqual(first, second); assert.match(first.output, /MOCK/); assert.equal(first.status, 'completed');
  assert.equal((await local.run(request('task', { agent: 'off' }))).status, 'failed');
  const controller = new AbortController(); controller.abort();
  assert.equal((await local.run(request('task', { signal: controller.signal }))).status, 'cancelled');
});
