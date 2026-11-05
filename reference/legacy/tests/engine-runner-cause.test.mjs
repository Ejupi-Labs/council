import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { WorkflowEngine } from '../dist/engine.js';
import { NativeRunner, trustedRunnerFailure } from '../dist/runner.js';
import { ManagedProcess } from '../dist/process.js';
import { AcpProtocol } from '../dist/acp.js';
import { PrivateChatManager } from '../dist/private-chats.js';

const root = fileURLToPath(new URL('../', import.meta.url));
const nativeFixture = fileURLToPath(new URL('./fixtures/fake-native.mjs', import.meta.url));
const acpFixture = fileURLToPath(new URL('./fixtures/fake-acp-causal.mjs', import.meta.url));
const config = (extra = {}) => ({
  version: 1, backend: 'native', workspaceRoot: root, stateDir: join(root, '.council'), maxConcurrency: 2,
  maxCalls: 8, timeoutMs: 15000, maxOutputBytes: 4096,
  agents: {
    fault: { enabled: true, profile: 'claude', transport: 'native', command: process.execPath, args: [nativeFixture] },
    peer: { enabled: true, profile: 'claude', transport: 'native', command: process.execPath, args: [nativeFixture] },
  }, ...extra,
});
const input = extra => ({ mode: 'delegate', agents: ['fault'], permission: 'read', task: 'Offline causal workflow regression.', ...extra });
function memory() {
  const runs = new Map(), log = [];
  return { runs, log, getRun: id => runs.get(id), saveRun: run => runs.set(run.id, structuredClone(run)),
    appendEvent: (runId, type, data) => log.push({ runId, type, data: structuredClone(data) }) };
}
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; };
async function bounded(promise, label) {
  let timer;
  try { return await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(label + ' exceeded 3 seconds.')), 3000); })]); }
  finally { clearTimeout(timer); }
}

test('actual native protocol failure remains an aggregate failure after cancellation during owned drain', { timeout: 30000 }, async t => {
  const settings = config(), controller = new AbortController(), native = new NativeRunner(settings);
  const kill = ManagedProcess.prototype.kill;
  let observed, kills = 0;
  t.mock.method(ManagedProcess.prototype, 'kill', function (...args) {
    const stopped = kill.apply(this, args);
    if (!kills++) controller.abort();
    return stopped;
  });
  const runner = { doctor: async () => [], run: async request => {
    observed = await native.run({ ...request, prompt: 'forbidden-tool' }); return observed;
  } };
  const run = await new WorkflowEngine(settings, runner, memory()).run(input(), { signal: controller.signal });
  assert.equal(trustedRunnerFailure(observed), 'failed'); assert.equal(run.status, 'failed');
  assert.equal(run.steps[0].result.status, 'failed');
  assert.equal(run.error, 'Claude requested a tool outside the read-mode allowlist.');
});

test('a true native timeout before later caller cancellation remains an aggregate failure', { timeout: 30000 }, async t => {
  const settings = config(), controller = new AbortController(), native = new NativeRunner(settings);
  const emit = ManagedProcess.prototype.emit, kill = ManagedProcess.prototype.kill, timer = globalThis.setTimeout;
  let expire, spawned = 0;
  t.mock.method(globalThis, 'setTimeout', (callback, milliseconds, ...args) => {
    if (milliseconds === 15000 && !expire) expire = () => callback(...args);
    return timer(callback, milliseconds, ...args);
  });
  t.mock.method(ManagedProcess.prototype, 'emit', function (event, ...args) {
    if (event === 'spawn' && !spawned++) expire();
    return emit.call(this, event, ...args);
  });
  t.mock.method(ManagedProcess.prototype, 'kill', function (...args) {
    const result = kill.apply(this, args); controller.abort(); return result;
  });
  const runner = { doctor: async () => [], run: request => native.run({ ...request, prompt: 'hang' }) };
  const run = await new WorkflowEngine(settings, runner, memory()).run(input(), { signal: controller.signal });
  assert.equal(run.status, 'failed'); assert.equal(run.steps[0].result.status, 'timeout'); assert.match(run.error, /total time limit/);
});

for (const throws of [false, true]) {
  test(`custom runner ${throws ? 'throw' : 'failed result'} after accepted cancellation cannot forge first-failure authority`, async () => {
    const controller = new AbortController();
    const runner = { doctor: async () => [], run: async request => {
      controller.abort();
      if (throws) throw new Error('Late custom error.');
      return { agent: request.agent, status: 'failed', output: 'custom partial', error: 'Late custom error.',
        durationMs: 1, trustedRunnerFailure: 'failed' };
    } };
    const run = await new WorkflowEngine(config(), runner, memory()).run(input(), { signal: controller.signal });
    assert.equal(run.status, 'cancelled');
  });
}

test('an admitted parallel peer drains normally after a trusted native failure', { timeout: 30000 }, async () => {
  const settings = config(), native = new NativeRunner(settings), entered = deferred(), release = deferred(), failed = deferred();
  let peerSignal, settled = false;
  const runner = { doctor: async () => [], run: async request => {
    if (request.agent === 'peer') {
      peerSignal = request.signal; entered.resolve(); await release.promise;
      return { agent: 'peer', status: 'completed', output: 'Admitted peer finished normally.', durationMs: 1 };
    }
    await entered.promise;
    const result = await native.run({ ...request, prompt: 'forbidden-tool' }); failed.resolve(); return result;
  } };
  const pending = new WorkflowEngine(settings, runner, memory()).run(input({ mode: 'parallel', agents: ['fault', 'peer'] }));
  pending.then(() => { settled = true; }, () => { settled = true; });
  try {
    await bounded(failed.promise, 'Native failure'); assert.equal(settled, false); assert.equal(peerSignal.aborted, false);
    release.resolve(); const run = await bounded(pending, 'Parallel result drain');
    assert.equal(run.status, 'failed'); assert.equal(run.steps.length, 2);
    assert.match(run.output, /Admitted peer finished normally/); assert.equal(peerSignal.aborted, false);
  } finally { release.resolve(); await bounded(pending, 'Owned parallel fixture drain'); }
});

for (const replyTurn of [false, true]) {
  test(`trusted private ${replyTurn ? 'reply' : 'request'} failure retains raw pair evidence while public status remains redacted`, { timeout: 30000 }, async t => {
    const parent = realpathSync.native(tmpdir()), temporary = mkdtempSync(join(parent, 'council-causal-private-'));
    const settings = config({ workspaceRoot: temporary, stateDir: join(temporary, '.council'), agents: {
      fault: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [acpFixture] },
      peer: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [acpFixture] },
    } });
    const store = memory(), manager = new PrivateChatManager({ config: settings, hasRun: id => store.runs.has(id) });
    const native = new NativeRunner(settings), controller = new AbortController();
    const kill = ManagedProcess.prototype.kill, event = AcpProtocol.prototype.event;
    const envelope = JSON.stringify({ council_private_chat_request: { peer: 'peer', message: 'PRIVATE_QUESTION', context: 'PRIVATE_CONTEXT' } });
    let calls = 0;
    t.mock.method(AcpProtocol.prototype, 'event', function (value, control) {
      if (value.method === 'session/update' && value.params?.update?.sessionUpdate === 'agent_message_chunk') {
        control.append(replyTurn ? 'PRIVATE_REPLY' : envelope); control.fail('PRIVATE_CAUSAL_DIAGNOSTIC'); return;
      }
      return event.call(this, value, control);
    });
    t.mock.method(ManagedProcess.prototype, 'kill', function (...args) {
      const result = kill.apply(this, args); controller.abort(); return result;
    });
    const runner = { doctor: async () => [], run: request => {
      calls++;
      if (replyTurn && calls === 1) return Promise.resolve({ agent: request.agent, status: 'completed', output: envelope, durationMs: 1 });
      return native.run({ ...request, prompt: 'hello' });
    } };
    try {
      const run = await new WorkflowEngine(settings, runner, store, { privateChats: () => manager })
        .run(input({ privateChats: { fault: { peers: ['peer'], maxRequests: 1 } } }), { signal: controller.signal });
      assert.equal(run.status, 'failed'); assert.equal(calls, replyTurn ? 2 : 1);
      const publicEvidence = JSON.stringify({ run, events: store.log });
      for (const marker of ['PRIVATE_QUESTION', 'PRIVATE_CONTEXT', 'PRIVATE_REPLY', 'PRIVATE_CAUSAL_DIAGNOSTIC']) {
        assert.equal(publicEvidence.includes(marker), false, marker);
      }
      if (replyTurn) {
        const actor = `run:${run.id}:fault`, chats = manager.list(actor); assert.equal(chats.length, 1);
        const reply = JSON.parse(manager.page({ chatId: chats[0].id, actor }).messages.at(-1).body);
        assert.equal(reply.status, 'failed'); assert.equal(reply.body, 'PRIVATE_REPLY'); assert.equal(reply.error, 'PRIVATE_CAUSAL_DIAGNOSTIC');
      }
    } finally {
      manager.close(); assert.equal(dirname(realpathSync.native(temporary)), parent);
      assert.ok(basename(temporary).startsWith('council-causal-private-'));
      rmSync(temporary, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
    }
  });
}
