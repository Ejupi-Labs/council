import test from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { NativeRunner, AcpxRunner, trustedRunnerFailure } from '../dist/runner.js';
import { ManagedProcess, ProcessCleanupError, PROCESS_CLEANUP_ERROR } from '../dist/process.js';

const root = fileURLToPath(new URL('../', import.meta.url));

function setup(profile, backend = 'native') {
  const filename = profile === 'antigravity' ? 'fake-antigravity.mjs'
    : profile === 'junie' ? 'fake-acp-causal.mjs'
      : backend === 'acpx' ? 'fake-acpx.mjs' : 'fake-native.mjs';
  const fixture = fileURLToPath(new URL('./fixtures/' + filename, import.meta.url));
  const config = {
    version: 1, backend, workspaceRoot: root, stateDir: join(root, '.council'), maxConcurrency: 1,
    maxCalls: 1, timeoutMs: 15000, maxOutputBytes: 16384,
    agents: { peer: { enabled: true, profile, transport: profile === 'junie' ? 'acp' : 'native',
      command: process.execPath, args: [fixture] } },
  };
  return {
    runner: backend === 'acpx' ? new AcpxRunner(config, { acpxEntrypoint: fixture }) : new NativeRunner(config),
    request: (prompt, signal) => ({ agent: 'peer', cwd: root, prompt, permission: 'read', timeoutMs: 15000, signal }),
  };
}

function budget(t) {
  const original = globalThis.setTimeout;
  let expire;
  t.mock.method(globalThis, 'setTimeout', (callback, milliseconds, ...args) => {
    if (milliseconds === 15000 && !expire) expire = () => callback(...args);
    return original(callback, milliseconds, ...args);
  });
  return () => { assert.equal(typeof expire, 'function'); expire(); };
}

const rejections = [
  ['antigravity', 'denied-error-step-wait', /tool execution failed or was denied/],
  ['claude', 'forbidden-tool', /read-mode allowlist/],
  ['junie', 'permission', /permission was denied by read mode.*Reported ACP tool kind: read/],
];
for (const [profile, prompt, pattern] of rejections) for (const later of ['cancel', 'timeout']) {
  test(`${profile} rejection stays failed when ${later} arrives only after cleanup begins`, { timeout: 30000 }, async t => {
    const local = setup(profile), controller = new AbortController(), expire = budget(t);
    const kill = ManagedProcess.prototype.kill;
    let stopped = 0;
    t.mock.method(ManagedProcess.prototype, 'kill', function (...args) {
      const value = kill.apply(this, args);
      if (!stopped++) { if (later === 'cancel') controller.abort(); else expire(); }
      return value;
    });
    const result = await local.runner.run(local.request(prompt, controller.signal));
    assert.ok(stopped > 0); assert.equal(result.status, 'failed'); assert.match(result.error, pattern);
    assert.equal(trustedRunnerFailure(result), 'failed', result.error);
    assert.equal(trustedRunnerFailure({ ...result }), undefined);
    assert.equal(result.error.startsWith(PROCESS_CLEANUP_ERROR), false);
    assert.doesNotMatch(JSON.stringify(result), /PRIVATE_TOOL_TITLE|PROVIDER_SPOOFED_CAUSE/);
  });
}

for (const [profile, backend] of [['antigravity', 'native'], ['junie', 'native'], ['codex', 'acpx']]) {
  test(`${backend}/${profile} provider cancellation survives a deadline during final drain`, { timeout: 30000 }, async t => {
    const local = setup(profile, backend), expire = budget(t), emit = ManagedProcess.prototype.emit;
    let closed = 0;
    t.mock.method(ManagedProcess.prototype, 'emit', function (event, ...args) {
      const value = emit.call(this, event, ...args);
      if (event === 'close' && !closed++) expire();
      return value;
    });
    const result = await local.runner.run(local.request('cancelled'));
    assert.ok(closed > 0); assert.equal(result.status, 'cancelled'); assert.equal(result.stopReason, 'cancelled');
    assert.equal(result.error, 'Agent cancelled its turn.'); assert.equal(trustedRunnerFailure(result), undefined);
  });
}

for (const first of ['cancel', 'timeout']) {
  test(`an accepted ${first} before provider rejection keeps its earlier cause`, { timeout: 30000 }, async t => {
    const local = setup('claude'), controller = new AbortController(), expire = budget(t);
    const emit = ManagedProcess.prototype.emit;
    let spawned = 0;
    t.mock.method(ManagedProcess.prototype, 'emit', function (event, ...args) {
      if (event === 'spawn' && !spawned++) { if (first === 'cancel') controller.abort(); else expire(); }
      return emit.call(this, event, ...args);
    });
    const result = await local.runner.run(local.request('forbidden-tool', controller.signal));
    assert.ok(spawned > 0); assert.equal(result.status, first === 'cancel' ? 'cancelled' : 'timeout');
    assert.equal(trustedRunnerFailure(result), first === 'timeout' ? 'timeout' : undefined);
  });
}

for (const scenario of ['malformed', 'nonzero', 'partial']) {
  test(`EOF ${scenario} failure is observed before close drainage yields to cancellation`, { timeout: 30000 }, async t => {
    const local = setup('claude'), controller = new AbortController(), emit = ManagedProcess.prototype.emit;
    let closed = 0;
    t.mock.method(ManagedProcess.prototype, 'emit', function (event, ...args) {
      const value = emit.call(this, event, ...args);
      if (event === 'close' && !closed++) controller.abort();
      return value;
    });
    const result = await local.runner.run(local.request(scenario, controller.signal));
    assert.ok(closed > 0); assert.equal(result.status, 'failed'); assert.equal(trustedRunnerFailure(result), 'failed');
    assert.match(result.error, scenario === 'malformed' ? /malformed or partial JSON/
      : scenario === 'nonzero' ? /exited with code 2/ : /without a successful terminal/);
  });
}

test('end_turn does not promise success before cleanup or permit a later protocol error', { timeout: 30000 }, async t => {
  const local = setup('codex', 'acpx'), controller = new AbortController(), kill = ManagedProcess.prototype.kill;
  let stopped = 0;
  t.mock.method(ManagedProcess.prototype, 'kill', function (...args) {
    const value = kill.apply(this, args);
    if (!stopped++) controller.abort();
    return value;
  });
  const result = await local.runner.run(local.request('late-text', controller.signal));
  assert.equal(result.status, 'failed'); assert.match(result.error, /after terminal completion/);
  assert.equal(trustedRunnerFailure(result), 'failed');
});

test('known end_turn awaiting cleanup remains subject to the actual deadline', { timeout: 30000 }, async t => {
  const local = setup('claude'), expire = budget(t), emit = ManagedProcess.prototype.emit;
  let closed = 0;
  t.mock.method(ManagedProcess.prototype, 'emit', function (event, ...args) {
    const value = emit.call(this, event, ...args);
    if (event === 'close' && !closed++) expire();
    return value;
  });
  const result = await local.runner.run(local.request('hello'));
  assert.equal(result.status, 'timeout'); assert.equal(trustedRunnerFailure(result), 'timeout');
});

test('cleanup uncertainty remains failed even when provider cancellation was first', { timeout: 30000 }, async t => {
  const local = setup('junie'), emit = ManagedProcess.prototype.emit;
  let stopped = 0;
  t.mock.method(ManagedProcess.prototype, 'emit', function (event, ...args) {
    if (event === 'close' && !stopped++) this.emit('error', new ProcessCleanupError());
    return emit.call(this, event, ...args);
  });
  const result = await local.runner.run(local.request('cancelled'));
  assert.ok(stopped > 0); assert.equal(result.status, 'failed'); assert.ok(result.error.startsWith(PROCESS_CLEANUP_ERROR));
});

test('provider-controlled cause fields and copied/custom result objects never create trusted failure evidence', { timeout: 30000 }, async () => {
  const local = setup('junie'), result = await local.runner.run(local.request('hello'));
  assert.equal(result.status, 'completed'); assert.equal(trustedRunnerFailure(result), undefined);
  assert.equal(trustedRunnerFailure({ agent: 'peer', status: 'failed', error: 'trusted', durationMs: 0,
    output: '', trustedRunnerFailure: 'failed' }), undefined);
});

test('cleanup uncertainty preserves an earlier denial after late cancellation', { timeout: 30000 }, async t => {
  const local = setup('antigravity'), controller = new AbortController();
  const kill = ManagedProcess.prototype.kill;
  let stopped = 0;
  t.mock.method(ManagedProcess.prototype, 'kill', function (...args) {
    if (!stopped++) this.emit('error', new ProcessCleanupError());
    const value = kill.apply(this, args);
    controller.abort();
    return value;
  });
  const result = await local.runner.run(local.request('denied-error-step-wait', controller.signal));
  assert.ok(stopped > 0);
  assert.equal(controller.signal.aborted, true);
  assert.equal(result.status, 'failed');
  assert.ok(result.error.startsWith(PROCESS_CLEANUP_ERROR));
  assert.match(result.error, /Earlier failure: .*tool execution failed or was denied/);
  assert.equal(trustedRunnerFailure(result), undefined);
  assert.equal(trustedRunnerFailure({ ...result }), undefined);
  assert.ok(result.error.length <= 4096);
  assert.doesNotMatch(JSON.stringify(result), /SECRET_ACCESS_TOKEN|PRIVATE_TOOL_TITLE|PROVIDER_SPOOFED_CAUSE/);
});
