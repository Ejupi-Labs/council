import test from 'node:test';
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { setTimeout as delay } from 'node:timers/promises';
import { basename, dirname, join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { CouncilService } from '../dist/service.js';
import { CouncilStore } from '../dist/store.js';
import { loadConfig } from '../dist/config.js';
import { NativeRunner } from '../dist/runner.js';
import { WorkflowEngine } from '../dist/engine.js';

const nativeFixture = fileURLToPath(new URL('./fixtures/fake-room-native.mjs', import.meta.url));
const writerFixture = fileURLToPath(new URL('./fixtures/slot-release-writer.mjs', import.meta.url));
const timeoutSignal = (t, ms = 5000) => AbortSignal.any([t.signal, AbortSignal.timeout(ms)]);
async function bounded(promise, ms, message) {
  const stop = new AbortController();
  try { return await Promise.race([promise, delay(ms, undefined, { signal: stop.signal }).then(() => { throw new Error(message); })]); }
  finally { stop.abort(); }
}
function temporary() {
  const parent = realpathSync.native(tmpdir()), root = mkdtempSync(join(parent, 'council-slot-release-'));
  return { root, remove() { assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-slot-release-')); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); } };
}
function releaseWriter(writer) { if (writer?.child.connected) writer.child.send({ type: 'release' }, () => {}); }
async function startWriter(t, database, marker) {
  const child = spawn(process.execPath, [writerFixture, database, marker], { windowsHide: true, stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
  child.stdout.resume(); child.stderr.resume();
  const done = new Promise(resolve => child.once('close', (...values) => resolve(values)));
  child.on('error', () => {});
  const writer = { child, done };
  try {
    const [message] = await once(child, 'message', { signal: timeoutSignal(t) });
    assert.equal(message.type, 'locked'); return writer;
  } catch (error) { await drainWriter(writer); throw error; }
}
async function drainWriter(writer) {
  releaseWriter(writer);
  let result;
  try { result = await bounded(writer.done, 5000, 'The owned SQLite writer did not stop.'); }
  catch (error) {
    writer.child.kill(); await bounded(writer.done, 5000, 'The owned SQLite writer could not be drained after termination.'); throw error;
  }
  assert.equal(result[0], 0, 'The owned SQLite writer must exit cleanly.');
}
function nativeFixtureService(t, onResult, onRelease) {
  const owned = temporary(), writers = [], releases = [];
  const config = loadConfig(owned.root, { backend: 'native', agents: { codex: { profile: 'codex', command: process.execPath, args: [nativeFixture] } } });
  const native = new NativeRunner(config); let calls = 0, service;
  service = new CouncilService(config, { doctor: async () => [], run: async request => {
    calls++; const result = await native.run(request); assert.equal(result.status, 'completed');
    await onResult?.({ service, config, writers, root: owned.root }); return result;
  } });
  const originalRelease = service.store.releaseSlot, originalSettle = service.store.releaseSlotWithRetry;
  if (onRelease) service.store.releaseSlotWithRetry = async (...args) => {
    assert.equal(service.store.db.isTransaction, false);
    const slot = service.store.db.prepare('SELECT run_id AS runId, instance_id AS instanceId FROM slots WHERE token=?').get(args[0]);
    assert.equal(slot?.instanceId, args[1], 'The exact owned slot must still exist before release contention begins.');
    const saved = service.store.getRun(slot.runId);
    assert.equal(calls, 1); assert.equal(saved.steps.length, 1, 'Actual returned step evidence must commit before the release writer starts.');
    assert.equal(service.store.events(slot.runId).filter(event => event.type === 'step.finished' && event.data.id === saved.steps[0].id).length, 1);
    await onRelease({ service, config, writers, root: owned.root });
    return await originalSettle.apply(service.store, args);
  };
  t.after(async () => {
    const failures = [];
    try {
      const drained = await Promise.allSettled(writers.map(drainWriter));
      for (const result of drained) if (result.status === 'rejected') failures.push(result.reason);
    } finally {
      service.store.releaseSlot = originalRelease; service.store.releaseSlotWithRetry = originalSettle;
      try { await service.close(); assert.equal(service.store.db.isOpen, false); } catch (error) { failures.push(error); }
      let inspect;
      try {
        inspect = new CouncilStore(config.stateDir);
        assert.equal(inspect.health().counts.slots, 0); assert.equal(inspect.health().counts.slotWaiters, 0);
      } catch (error) { failures.push(error); }
      finally { try { inspect?.close(); } catch (error) { failures.push(error); } }
    }
    if (failures.length === 1) throw failures[0];
    if (failures.length) throw new AggregateError(failures, 'Native slot fixture cleanup failed.', { cause: failures[0] });
    owned.remove();
  });
  return { service, config, writers, releases, root: owned.root, calls: () => calls };
}
const task = { mode: 'delegate', agents: ['codex'], task: 'Return the offline fixture marker once without editing files.', permission: 'read' };

for (const mode of ['normal', 'cancel', 'quarantine']) test(`native slot cleanup settles transient contention without replay during ${mode}`, { timeout: 20000 }, async t => {
  let runId, writer;
  const wire = nativeFixtureService(t, ({ service }) => {
    if (mode === 'cancel') service.requestCancellation(runId);
    if (mode === 'quarantine') service.store.reportProcessCleanupIncident({ reportKey: 'owned-release-quarantine', instanceId: service.instanceId, runId, stage: 'agent_call' });
  }, async ({ config, writers, root }) => {
    writer = await startWriter(t, join(config.stateDir, 'state.db'), join(root, 'unused-marker.json')); writers.push(writer);
  });
  const release = wire.service.store.releaseSlot.bind(wire.service.store);
  wire.service.store.releaseSlot = (...args) => {
    assert.equal(wire.service.store.db.isTransaction, false);
    try { release(...args); }
    catch (error) {
      wire.releases.push({ code: error.code, errcode: error.errcode });
      assert.equal(wire.service.store.db.isTransaction, false); releaseWriter(writer); throw error;
    }
  };
  const run = wire.service.start(task); runId = run.id; const result = await wire.service.completion(run.id);
  assert.equal(result.status, mode === 'normal' ? 'completed' : mode === 'cancel' ? 'cancelled' : 'failed');
  assert.equal(wire.calls(), 1); assert.equal(result.steps.length, 1);
  if (mode !== 'quarantine') assert.equal(result.steps[0].result.status, 'completed');
  assert.ok(wire.releases.some(error => error.code === 'ERR_SQLITE_ERROR' && (error.errcode & 0xff) === 5));
  assert.equal(wire.service.store.health().counts.slots, 0); assert.equal(wire.service.store.health().counts.slotWaiters, 0);
  assert.equal(wire.service.store.db.prepare('PRAGMA busy_timeout').get().timeout, 250);
  if (mode === 'quarantine') assert.equal(wire.service.dispatchQuarantinePage().blocked, true);
});

test('cancellation accepted during persistent slot contention cannot hide cleanup failure at the fixed deadline', { timeout: 25000 }, async t => {
  let runId, writer;
  const wire = nativeFixtureService(t, undefined, async ({ config, writers, root }) => {
    writer = await startWriter(t, join(config.stateDir, 'state.db'), join(root, 'accepted.json')); writers.push(writer);
  });
  const release = wire.service.store.releaseSlot.bind(wire.service.store), settle = wire.service.store.releaseSlotWithRetry.bind(wire.service.store);
  let attempts = 0;
  wire.service.store.releaseSlot = (...args) => {
    if (++attempts === 2) {
      writer.child.send({ type: 'cancel-and-relock', runId });
      const marker = join(wire.root, 'accepted.json'), deadline = performance.now() + 2000, sleeper = new Int32Array(new SharedArrayBuffer(4));
      while (!existsSync(marker)) { assert.ok(performance.now() < deadline, 'The owned writer must acknowledge durable cancellation.'); Atomics.wait(sleeper, 0, 0, 5); }
      assert.equal(JSON.parse(readFileSync(marker, 'utf8')).accepted, true);
      assert.equal(wire.service.store.isCancellationRequested(runId), true);
    }
    return release(...args);
  };
  wire.service.store.releaseSlotWithRetry = async (...args) => {
    try { return await settle(...args); }
    catch (error) { await drainWriter(writer); throw error; }
  };
  const run = wire.service.start(task); runId = run.id; const result = await wire.service.completion(run.id);
  assert.equal(result.status, 'failed'); assert.match(result.error, /Could not release a shared workflow call slot: database is locked/);
  assert.equal(wire.calls(), 1); assert.deepEqual(result.steps.map(step => step.result.status), ['completed']);
  assert.ok(attempts > 1); assert.equal(wire.service.store.health().counts.slots, 1, 'A failed settlement does not claim the shared slot was released.');
});

function storeFixture(t) {
  const owned = temporary(), store = new CouncilStore(join(owned.root, '.council')), foreign = new CouncilStore(join(owned.root, '.council'));
  const owner = randomUUID(), other = randomUUID(); store.registerInstance(owner, process.pid, 4); foreign.registerInstance(other, process.pid, 4);
  const queued = () => { const now = new Date().toISOString(); return { id: randomUUID(), input: task, status: 'queued', createdAt: now, updatedAt: now, steps: [] }; };
  const run = queued(), otherRun = queued(); store.createRun(run, owner); foreign.createRun(otherRun, other);
  const nativeDb = store.db, release = store.releaseSlot, settle = store.releaseSlotWithRetry;
  const restore = () => { Object.defineProperty(store, 'db', { value: nativeDb, configurable: true, writable: true }); store.releaseSlot = release; store.releaseSlotWithRetry = settle; };
  t.after(() => {
    restore(); if (nativeDb.isTransaction) nativeDb.exec('ROLLBACK');
    store.finalizeRun({ ...run, status: 'cancelled' }); foreign.finalizeRun({ ...otherRun, status: 'cancelled' });
    store.unregisterInstance(owner); foreign.unregisterInstance(other); foreign.close(); store.close(); owned.remove();
  });
  return { store, foreign, owner, other, otherRun, nativeDb, restore, acquire: () => store.acquireSlot(owner, run.id, 4),
    release: (...args) => release.call(store, ...args), held: token => nativeDb.prepare('SELECT instance_id FROM slots WHERE token=?').get(token) };
}
const acknowledgementLoss = () => Object.assign(new Error('Synthetic post-delete acknowledgement loss, not a native COMMIT result.'), { code: 'ERR_SQLITE_ERROR', errcode: 5 });

test('slot settlement preserves exact token and owner identity across uncertain acknowledgements', async t => {
  const wire = storeFixture(t); let token = wire.acquire(), attempts = 0, newer;
  wire.store.releaseSlot = (...args) => {
    wire.release(...args);
    if (++attempts === 1) { newer = wire.acquire(); assert.notEqual(newer, token); throw acknowledgementLoss(); }
  };
  await wire.store.releaseSlotWithRetry(token, wire.owner); assert.equal(wire.held(token), undefined); assert.ok(wire.held(newer)); wire.release(newer, wire.owner);
  token = wire.acquire(); attempts = 0;
  wire.store.releaseSlot = (...args) => {
    wire.release(...args);
    if (++attempts === 1) {
      const replacement = wire.foreign.acquireSlot(wire.other, wire.otherRun.id, 4);
      wire.foreign.db.prepare('UPDATE slots SET token=? WHERE token=?').run(token, replacement); throw acknowledgementLoss();
    }
  };
  await wire.store.releaseSlotWithRetry(token, wire.owner); assert.equal(wire.held(token).instance_id, wire.other); wire.foreign.releaseSlot(token, wire.other);
  wire.restore(); token = wire.acquire();
  await wire.store.releaseSlotWithRetry(token, wire.other); await wire.store.releaseSlotWithRetry(randomUUID(), wire.owner);
  assert.equal(wire.held(token).instance_id, wire.owner); wire.release(token, wire.owner);
});

test('slot settlement refuses open or unknown transaction state without rolling back unrelated work', async t => {
  const wire = storeFixture(t), token = wire.acquire();
  wire.nativeDb.exec('BEGIN IMMEDIATE'); await assert.rejects(wire.store.releaseSlotWithRetry(token, wire.owner), /inside an open transaction/);
  assert.equal(wire.nativeDb.isTransaction, true); wire.nativeDb.exec('ROLLBACK');
  wire.store.releaseSlot = (...args) => { wire.nativeDb.exec('BEGIN IMMEDIATE'); wire.release(...args); };
  await assert.rejects(wire.store.releaseSlotWithRetry(token, wire.owner), /left an open transaction/);
  assert.equal(wire.nativeDb.isTransaction, true); wire.nativeDb.exec('ROLLBACK'); assert.ok(wire.held(token));
  const primary = acknowledgementLoss();
  wire.store.releaseSlot = () => { wire.nativeDb.exec('BEGIN IMMEDIATE'); throw primary; };
  await assert.rejects(wire.store.releaseSlotWithRetry(token, wire.owner), error => error === primary);
  assert.equal(wire.nativeDb.isTransaction, true); wire.nativeDb.exec('ROLLBACK');
  let unknown = false, attempts = 0; const inspection = new Error('Injected transaction inspection failure.');
  Object.defineProperty(wire.store, 'db', { configurable: true, get() { if (unknown) throw inspection; return wire.nativeDb; } });
  wire.store.releaseSlot = () => { attempts++; queueMicrotask(() => { unknown = true; }); throw primary; };
  await assert.rejects(wire.store.releaseSlotWithRetry(token, wire.owner), error => error === primary); assert.equal(attempts, 1);
  wire.restore(); unknown = false;
  Object.defineProperty(wire.store, 'db', { configurable: true, get() { if (unknown) throw inspection; return wire.nativeDb; } });
  wire.store.releaseSlot = (...args) => { wire.release(...args); unknown = true; };
  await assert.rejects(wire.store.releaseSlotWithRetry(token, wire.owner), error => error === inspection); wire.restore();
});

test('slot settlement does not retry permanent or unverified error lookalikes', async t => {
  const wire = storeFixture(t), token = wire.acquire();
  for (const error of [new Error('database is locked'), { code: 'ERR_SQLITE_ERROR', errcode: 5 },
    ...[10, -251, 0x100000005, NaN].map(errcode => Object.assign(new Error('Injected non-retryable slot error.'), { code: 'ERR_SQLITE_ERROR', errcode }))]) {
    let attempts = 0; wire.store.releaseSlot = () => { attempts++; throw error; };
    await assert.rejects(wire.store.releaseSlotWithRetry(token, wire.owner), actual => actual === error); assert.equal(attempts, 1); assert.ok(wire.held(token));
  }
});

test('awaiting a legacy asynchronous release keeps local execution capacity until cleanup settles', async t => {
  const owned = temporary(); let releaseFirst, entered, calls = 0, released = 0;
  const gate = new Promise(resolve => { releaseFirst = resolve; }), firstEntered = new Promise(resolve => { entered = resolve; });
  const records = new Map(), backend = { doctor: async () => [], run: async request => { calls++; return { agent: request.agent, status: 'completed', output: 'Offline legacy-store result.', durationMs: 1 }; } };
  const persistence = { saveRun: run => records.set(run.id, structuredClone(run)), appendEvent() {}, acquireSlot: () => randomUUID(),
    async releaseSlot() { if (++released === 1) { entered(); await gate; } } };
  const engine = new WorkflowEngine(loadConfig(owned.root, { backend: 'mock', maxConcurrency: 1 }), backend, persistence, { instanceId: 'legacy-release-test' });
  const outcomes = Promise.all([engine.run(task, { signal: t.signal }), engine.run(task, { signal: t.signal })]); outcomes.catch(() => {});
  t.after(async () => { releaseFirst(); await outcomes; owned.remove(); });
  await bounded(firstEntered, 5000, 'Legacy release was not reached.');
  await delay(25, undefined, { signal: t.signal }); assert.equal(calls, 1, 'Another local call cannot start while the first durable cleanup is unresolved.');
  releaseFirst(); const results = await outcomes; assert.deepEqual(results.map(result => result.status), ['completed', 'completed']); assert.equal(calls, 2); assert.equal(released, 2);
});
