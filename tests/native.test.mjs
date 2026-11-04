import test from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { mkdtemp, readFile, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import fs from 'node:fs/promises';
import { syncBuiltinESMExports } from 'node:module';
import { NativeRunner, createRunner } from '../dist/runner.js';
import { ManagedProcess, ProcessCleanupError, PROCESS_CLEANUP_ERROR } from '../dist/process.js';

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

for (const backend of ['native', 'acpx', 'mock']) {
  test(`${backend} rejects missing or invalid permission before any dispatch`, async () => {
    const settings = { ...config({ claude: native('claude') }), backend };
    const local = createRunner(settings);
    for (const permission of [undefined, null, 'read-only', 'WRITE', true]) {
      const result = await local.run(request('claude', 'args', { permission }));
      assert.equal(result.status, 'failed');
      assert.match(result.error, /permission.*read.*write/i);
      assert.equal(result.durationMs, 0);
      assert.equal(result.output, '');
    }
  });
}

for (const scenario of ['success-without-result', 'success-with-invalid-result', 'success-with-invalid-error-flag', 'success-with-invalid-denials']) {
  test(`Claude rejects malformed terminal ${scenario} despite earlier assistant text`, async () => {
    const result = await new NativeRunner(config({ claude: native('claude') })).run(request('claude', scenario));
    assert.equal(result.status, 'failed');
    assert.match(result.output, /Native/); // Partial text remains available without implying completion.
    assert.match(result.error, /invalid.*result|invalid.*permission/i);
  });
}

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

test('Junie exact authentication failure text cannot masquerade as successful ACP completion', async () => {
  const local = new NativeRunner(config({ peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter] } }),
    { environment: { ...process.env, COUNCIL_TEST_ACP_CASE: 'provider-auth-answer' } });
  const result = await local.run(request('peer', 'Return the offline marker.'));
  assert.equal(result.status, 'failed');
  assert.match(result.error, /Junie authentication.*Sign in.*CLI/);
  assert.equal(result.output, 'Authentication data is invalid or missing');
  assert.equal(result.stopReason, 'end_turn');
});

for (const [profile, scenario] of [['junie', 'provider-auth-example'], ['codex', 'provider-auth-answer']]) {
  test(`authentication text remains ordinary evidence for ${profile} ${scenario}`, async () => {
    const local = new NativeRunner(config({ peer: { enabled: true, profile, transport: 'acp', command: process.execPath, args: [adapter] } }),
      { environment: { ...process.env, COUNCIL_TEST_ACP_CASE: scenario } });
    const result = await local.run(request('peer', 'Discuss the offline authentication diagnostic.'));
    assert.equal(result.status, 'completed', result.error);
    assert.match(result.output, /Authentication data is invalid or missing/);
  });
}

test('direct ACP rejects an unadvertised model without changing the requested model', async () => {
  const local = new NativeRunner(config({ peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter], model: 'unavailable-model' } }));
  const result = await local.run(request('peer', 'hello'));
  assert.equal(result.status, 'failed'); assert.match(result.error, /not advertised/);
});

test('direct ACP cannot report success from a contradictory result and error envelope', async () => {
  const local = new NativeRunner(config({ peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter] } }),
    { environment: { ...process.env, COUNCIL_TEST_ACP_CASE: 'mixed-result-error' } });
  const result = await local.run(request('peer', 'Read offline evidence.'));
  assert.equal(result.status, 'failed');
  assert.match(result.error, /invalid.*envelope/i);
  assert.match(result.output, /Local ACP fixture completed/);
});

test('native doctor only inspects executable availability', async () => {
  const health = await new NativeRunner(config({ codex: native('codex'), missing: { enabled: true, profile: 'junie', command: join(cwd, 'not-an-executable.exe') } })).doctor();
  assert.equal(health[0].available, true); assert.match(health[0].detail, /Authentication.*were not tested/); assert.equal(health[1].available, false);
});

test('vendor children cannot inherit Git repository or index redirection outside their assigned cwd', async () => {
  const environment = { ...process.env, GIT_DIR: 'outside-fixture.git', GIT_WORK_TREE: 'outside-fixture',
    GIT_INDEX_FILE: 'outside-fixture.index', GIT_CONFIG_COUNT: '1', GIT_CONFIG_KEY_0: 'core.worktree',
    GIT_CONFIG_VALUE_0: 'outside-fixture', COUNCIL_TEST_KEEP: 'ordinary-environment-is-preserved' };
  const result = await new NativeRunner(config({ codex: native('codex') }), { environment }).run(request('codex', 'git-env'));
  assert.equal(result.status, 'completed', result.error);
  assert.deepEqual(JSON.parse(result.output), { COUNCIL_TEST_KEEP: 'ordinary-environment-is-preserved' });
  assert.equal(environment.GIT_INDEX_FILE, 'outside-fixture.index');
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

test('an unverified cleanup failure remains explicit instead of being hidden by cancellation', { timeout: 20000 }, async () => {
  const temporary = await mkdtemp(join(tmpdir(), 'council-cleanup-diagnostic-'));
  const pidFile = join(temporary, 'pids.json');
  const original = ManagedProcess.prototype.kill;
  const controller = new AbortController();
  let managed, running;
  try {
    ManagedProcess.prototype.kill = function (...args) {
      managed = this;
      this.emit('error', new ProcessCleanupError());
      return original.apply(this, args);
    };
    const local = new NativeRunner(config({ codex: native('codex') }), { environment: { ...process.env, COUNCIL_TEST_PID_FILE: pidFile } });
    running = local.run(request('codex', 'tree-hang', { signal: controller.signal }));
    let ready = false;
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      try { await readFile(pidFile, 'utf8'); ready = true; break; } catch { await new Promise(resolve => setTimeout(resolve, 20)); }
    }
    assert.equal(ready, true, 'The offline tree must start before cancellation.');
    controller.abort();
    const result = await running;
    assert.equal(result.status, 'failed');
    assert.equal(result.error, PROCESS_CLEANUP_ERROR);
  } finally {
    ManagedProcess.prototype.kill = original;
    controller.abort();
    if (running) await running;
    if (managed) await managed.forceKill().catch(() => {});
    await rm(temporary, { recursive: true, force: true });
  }
});

for (const scenario of ['invalid-utf8', 'frame-overflow']) test(`agent framing rejects ${scenario} before accepting terminal success`, async () => {
  const settings = config({ codex: native('codex') }); settings.maxOutputBytes = 1024 * 1024;
  const result = await new NativeRunner(settings).run(request('codex', scenario));
  assert.equal(result.status, 'failed'); assert.match(result.error, scenario === 'invalid-utf8' ? /invalid UTF-8/ : /frame exceeded 4 MiB/);
});

for (const agent of ['codex', 'claude']) {
  test(`${agent} rejects oversized JSON graphs before parsing while retaining partial text`, async () => {
    const parse = JSON.parse;
    let deepParses = 0;
    JSON.parse = (text, ...rest) => {
      if (typeof text === 'string' && text.length > 500_000 && text.includes('"payload":')) deepParses++;
      return parse(text, ...rest);
    };
    try {
      const local = new NativeRunner(config({ [agent]: native(agent) }));
      for (const scenario of ['structure-deep', 'structure-wide']) {
        const result = await local.run(request(agent, scenario));
        assert.equal(result.status, 'failed');
        assert.match(result.error, /JSON depth 64|65536 structural tokens/);
        assert.match(result.output, /Native/);
      }
      assert.equal(deepParses, 0, 'Deep frames must be refused before JSON.parse allocates their graph.');
      assert.equal((await local.run(request(agent, 'structure-quoted'))).status, 'completed');
    } finally { JSON.parse = parse; }
  });
}

test('direct ACP structural limits preserve partial text and permit escaped tool-output strings', async () => {
  for (const scenario of ['structure-deep', 'structure-wide', 'structure-quoted']) {
    const local = new NativeRunner(config({ peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter] } }),
      { environment: { ...process.env, COUNCIL_TEST_ACP_CASE: scenario } });
    const result = await local.run(request('peer', 'Read project evidence.'));
    assert.equal(result.status, scenario === 'structure-quoted' ? 'completed' : 'failed', result.error);
    assert.match(result.output, /Partial offline evidence/);
    if (result.status === 'failed') assert.match(result.error, /JSON depth 64|65536 structural tokens/);
  }
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
