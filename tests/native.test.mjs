import test from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { mkdtemp, readFile, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import fs from 'node:fs/promises';
import { syncBuiltinESMExports } from 'node:module';
import { NativeRunner, createRunner } from '../dist/runner.js';

const cwd = fileURLToPath(new URL('..', import.meta.url));
const fixture = fileURLToPath(new URL('./fixtures/fake-native.mjs', import.meta.url));
const adapter = fileURLToPath(new URL('./fixtures/fake-acp-adapter.mjs', import.meta.url));
const config = agents => ({ version: 1, workspaceRoot: cwd, stateDir: join(cwd, '.council'), backend: 'native', maxConcurrency: 2, timeoutMs: 15000, maxOutputBytes: 16384, maxCalls: 8, agents });
const native = profile => ({ enabled: true, profile, command: process.execPath, args: [fixture] });
const request = (agent, prompt, options) => ({ agent, prompt, cwd, permission: 'read', timeoutMs: 15000, ...options });

for (const agent of ['codex', 'claude']) {
  test(`${agent} native JSON returns assistant text exactly once`, async () => {
    const result = await new NativeRunner(config({ [agent]: native(agent) })).run(request(agent, 'hello'));
    assert.equal(result.status, 'completed', result.error); assert.equal(result.output, 'Native 🌍 hello');
  });
  for (const scenario of ['malformed', 'partial', 'nonzero', 'auth-error']) test(`${agent} native rejects ${scenario}`, async () => {
    const result = await new NativeRunner(config({ [agent]: native(agent) })).run(request(agent, scenario));
    assert.equal(result.status, 'failed'); assert.ok(!JSON.stringify(result).includes('SECRET_ACCESS_TOKEN'));
  });
  test(`${agent} native passes prompt through stdin and explicit permission/model flags`, async () => {
    const local = new NativeRunner(config({ [agent]: { ...native(agent), model: 'explicit-fixture-model' } }));
    const result = await local.run(request(agent, 'args'));
    assert.equal(result.status, 'completed', result.error);
    const args = JSON.parse(result.output);
    assert.ok(!args.includes('args')); assert.equal(args[args.indexOf('--model') + 1], 'explicit-fixture-model');
    if (agent === 'codex') { assert.equal(args[args.indexOf('--sandbox') + 1], 'read-only'); assert.ok(args.indexOf('--ask-for-approval') < args.indexOf('exec')); }
    else { assert.equal(args[args.indexOf('--tools') + 1], 'Read,Glob,Grep'); assert.ok(args.includes('dontAsk')); assert.ok(args.includes('mcp__*')); }
  });
}

test('Claude rejects denied permissions and tools outside its read-mode allowlist', async () => {
  const local = new NativeRunner(config({ claude: native('claude') }));
  assert.equal((await local.run(request('claude', 'denied'))).status, 'failed');
  assert.equal((await local.run(request('claude', 'forbidden-tool'))).status, 'failed');
});

for (const profile of ['codex', 'claude', 'deepseek-harness', 'junie', 'antigravity']) test(`direct ACP is available for configured ${profile} adapters`, async () => {
  const local = createRunner(config({ peer: { enabled: true, profile, transport: 'acp', command: process.execPath, args: [adapter] } }));
  const result = await local.run(request('peer', 'hello'));
  assert.equal(result.status, 'completed', result.error); assert.match(result.output, /Local ACP fixture completed/);
});

for (const scenario of ['version', 'auth', 'unknown-request', 'question']) test(`direct ACP fails closed on ${scenario}`, async () => {
  const local = new NativeRunner(config({ peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter] } }),
    { environment: { ...process.env, COUNCIL_TEST_ACP_CASE: scenario } });
  const result = await local.run(request('peer', scenario === 'question' ? 'request-write-permission' : 'hello', { permission: 'write' }));
  assert.equal(result.status, 'failed'); assert.ok(!JSON.stringify(result).includes('SECRET_ACCESS_TOKEN'));
});

test('direct ACP rejects an unadvertised model without changing the requested model', async () => {
  const local = new NativeRunner(config({ peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter], model: 'unavailable-model' } }));
  const result = await local.run(request('peer', 'hello'));
  assert.equal(result.status, 'failed'); assert.match(result.error, /not advertised/);
});

test('native doctor only inspects executable availability', async () => {
  const health = await new NativeRunner(config({ codex: native('codex'), missing: { enabled: true, profile: 'junie', command: join(cwd, 'not-an-executable.exe') } })).doctor();
  assert.equal(health[0].available, true); assert.match(health[0].detail, /Authentication.*were not tested/); assert.equal(health[1].available, false);
});

test('the total agent deadline includes executable resolution and reports honest elapsed time', async () => {
  const original = fs.stat;
  try {
    fs.stat = async (...args) => { if (args[0] === process.execPath) await new Promise(resolve => setTimeout(resolve, 1000)); return original(...args); };
    syncBuiltinESMExports();
    const started = performance.now();
    const result = await new NativeRunner(config({ codex: native('codex') })).run(request('codex', 'hello', { timeoutMs: 50 }));
    assert.equal(result.status, 'timeout'); assert.match(result.error, /total time limit/);
    assert.ok(performance.now() - started < 700, 'A stalled lookup must not delay the agent deadline.');
    assert.ok(result.durationMs >= 35 && result.durationMs < 700);
  } finally { fs.stat = original; syncBuiltinESMExports(); }
});

test('cancellation interrupts executable resolution before a vendor is launched', async () => {
  const original = fs.stat;
  const controller = new AbortController();
  try {
    fs.stat = async (...args) => { if (args[0] === process.execPath) await new Promise(resolve => setTimeout(resolve, 1000)); return original(...args); };
    syncBuiltinESMExports();
    const started = performance.now();
    const run = new NativeRunner(config({ codex: native('codex') })).run(request('codex', 'hello', { signal: controller.signal }));
    setTimeout(() => controller.abort(), 25);
    assert.equal((await run).status, 'cancelled'); assert.ok(performance.now() - started < 700);
  } finally { fs.stat = original; syncBuiltinESMExports(); }
});

for (const scenario of ['invalid-utf8', 'frame-overflow']) test(`agent framing rejects ${scenario} before accepting terminal success`, async () => {
  const settings = config({ codex: native('codex') }); settings.maxOutputBytes = 1024 * 1024;
  const result = await new NativeRunner(settings).run(request('codex', scenario));
  assert.equal(result.status, 'failed'); assert.match(result.error, scenario === 'invalid-utf8' ? /invalid UTF-8/ : /frame exceeded 4 MiB/);
});

test('ACP read permissions never trust presentation metadata as authorization', async () => {
  const local = new NativeRunner(config({ peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter] } }),
    { environment: { ...process.env, COUNCIL_TEST_ACP_CASE: 'read-permission' } });
  const result = await local.run(request('peer', 'request-write-permission'));
  assert.equal(result.status, 'failed'); assert.match(result.error, /permission was denied/);
});

test('ACP client filesystem reads a real project file with path-scoped capabilities', async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-acp-client-fs-'));
  try {
    const filename = join(temporary, 'project.txt'); await writeFile(filename, 'Project evidence.\n');
    const local = new NativeRunner(config({ peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter] } }),
      { environment: { ...process.env, COUNCIL_TEST_ACP_CASE: 'fs-read', COUNCIL_TEST_FILE: filename } });
    const result = await local.run(request('peer', 'Read project evidence.', { cwd: temporary }));
    assert.equal(result.status, 'completed', result.error); assert.equal(JSON.parse(result.output).content, 'Project evidence.\n');
  } finally { await rm(temporary, { recursive: true, force: true }); }
});

test('ACP client filesystem rejects read-mode writes and allows write-mode project writes', async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-acp-client-write-'));
  try {
    const filename = join(temporary, 'project.txt'); await writeFile(filename, 'Original.');
    const local = new NativeRunner(config({ peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter] } }),
      { environment: { ...process.env, COUNCIL_TEST_ACP_CASE: 'fs-write', COUNCIL_TEST_FILE: filename } });
    const denied = await local.run(request('peer', 'Write fixture.', { cwd: temporary }));
    assert.equal(denied.status, 'failed'); assert.equal(await readFile(filename, 'utf8'), 'Original.');
    const allowed = await local.run(request('peer', 'Write fixture.', { cwd: temporary, permission: 'write' }));
    assert.equal(allowed.status, 'completed', allowed.error); assert.deepEqual(JSON.parse(allowed.output), {});
    assert.equal(await readFile(filename, 'utf8'), 'changed by ACP fixture');
  } finally { await rm(temporary, { recursive: true, force: true }); }
});

for (const scenario of ['fs-session', 'fs-flood']) test(`ACP client filesystem rejects ${scenario}`, async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-acp-client-limit-'));
  try {
    const filename = join(temporary, 'project.txt'); await writeFile(filename, 'Evidence.');
    const local = new NativeRunner(config({ peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter] } }),
      { environment: { ...process.env, COUNCIL_TEST_ACP_CASE: scenario, COUNCIL_TEST_FILE: filename } });
    const result = await local.run(request('peer', 'Read project evidence.', { cwd: temporary }));
    assert.equal(result.status, 'failed');
  } finally { await rm(temporary, { recursive: true, force: true }); }
});
