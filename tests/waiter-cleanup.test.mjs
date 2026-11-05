import test from 'node:test';
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { spawn } from 'node:child_process';
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

const writerFixture = fileURLToPath(new URL('./fixtures/slot-release-writer.mjs', import.meta.url));
const nativeFixture = fileURLToPath(new URL('./fixtures/fake-room-native.mjs', import.meta.url));
const input = { mode: 'delegate', agents: ['codex'], task: 'Never repeat or dispatch cancelled queued work.', permission: 'read' };
const snapshotSql = 'SELECT CAST(sequence AS TEXT) AS generation FROM slot_waiters WHERE instance_id = ? AND waiter_id = ?';
const deleteSql = 'DELETE FROM slot_waiters WHERE instance_id = ? AND waiter_id = ? AND sequence = ?';
function temporary() {
  const parent = realpathSync.native(tmpdir()), root = mkdtempSync(join(parent, 'council-waiter-cleanup-'));
  return { root, state: join(root, '.council'), remove() { assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-waiter-cleanup-')); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); } };
}
async function bounded(promise, ms, message) {
  const stop = new AbortController();
  try { return await Promise.race([promise, delay(ms, undefined, { signal: stop.signal }).then(() => { throw new Error(message); })]); }
  finally { stop.abort(); }
}
function queued() { const now = new Date().toISOString(); return { id: randomUUID(), input, status: 'queued', steps: [], createdAt: now, updatedAt: now }; }
function heldFixture() {
  const owned = temporary(), holder = new CouncilStore(owned.state), owner = randomUUID(), run = queued();
  holder.registerInstance(owner, process.pid, 1); holder.createRun(run, owner); const token = holder.acquireSlot(owner, run.id, 1); assert.ok(token);
  return { ...owned, holder, finish() {
    const failures = [];
    try { holder.releaseSlot(token, owner); holder.finalizeRun({ ...run, status: 'cancelled' }); holder.unregisterInstance(owner); }
    catch (error) { failures.push(error); }
    finally { try { holder.close(); } catch (error) { failures.push(error); } }
    if (failures.length === 1) throw failures[0];
    if (failures.length) throw new AggregateError(failures, 'Held-slot fixture cleanup failed.', { cause: failures[0] });
  } };
}
function releaseWriter(writer) { if (writer?.child.connected) writer.child.send({ type: 'release' }, () => {}); }
async function drainWriter(writer) {
  if (!writer) return;
  releaseWriter(writer);
  let result;
  try { result = await bounded(writer.done, 5000, 'The waiter cleanup writer did not exit.'); }
  catch (error) { writer.child.kill(); await bounded(writer.done, 5000, 'The waiter cleanup writer could not be drained.'); throw error; }
  assert.equal(result[0], 0);
}
function lockWriter(owned) {
  const marker = join(owned.root, 'locked.json');
  const child = spawn(process.execPath, [writerFixture, join(owned.state, 'state.db'), join(owned.root, 'unused-cancel.json'), marker], { windowsHide: true, stdio: ['ignore', 'pipe', 'pipe', 'ipc'] });
  child.stdout.resume(); child.stderr.resume(); child.on('error', () => {});
  const writer = { child, done: new Promise(resolve => child.once('close', (...values) => resolve(values))) };
  const deadline = performance.now() + 3000, sleeper = new Int32Array(new SharedArrayBuffer(4));
  try {
    while (!existsSync(marker)) { assert.ok(performance.now() < deadline, 'The owned writer did not reach its lock.'); Atomics.wait(sleeper, 0, 0, 5); }
    assert.equal(JSON.parse(readFileSync(marker, 'utf8')).locked, true); return writer;
  } catch (error) { releaseWriter(writer); throw Object.assign(error, { writer }); }
}
function nativeStatementProxy(prepared, run) {
  return { setReadBigInts(value) { prepared.setReadBigInts(value); return this; }, run };
}

for (const persistent of [false, true]) test(`queued shutdown ${persistent ? 'fails honestly at the cleanup deadline' : 'settles brief writer contention without dispatch'}`, { timeout: 25000 }, async t => {
  const owned = heldFixture(), config = loadConfig(owned.root, { backend: 'native', maxConcurrency: 1,
    agents: { codex: { profile: 'codex', command: process.execPath, args: [nativeFixture] } } });
  const native = new NativeRunner(config); let calls = 0, writer, closing, triggered = false, snapshots = 0, refusals = 0;
  const service = new CouncilService(config, { doctor: async () => [], run: request => { calls++; return native.run(request); } });
  const acquire = service.store.acquireSlot, statement = service.store.statement, settle = service.store.cancelSlotWaiterWithRetry;
  t.after(async () => {
    const failures = [];
    try { await drainWriter(writer); } catch (error) { failures.push(error); }
    finally {
      service.store.acquireSlot = acquire; service.store.statement = statement; service.store.cancelSlotWaiterWithRetry = settle;
      try { await service.close(); assert.equal(service.store.db.isOpen, false); } catch (error) { failures.push(error); }
      try { owned.finish(); } catch (error) { failures.push(error); }
    }
    let inspect;
    try { inspect = new CouncilStore(owned.state); const health = inspect.health(); assert.equal(health.counts.slotWaiters, 0); assert.equal(health.counts.slots, 0); }
    catch (error) { failures.push(error); }
    finally { try { inspect?.close(); } catch (error) { failures.push(error); } }
    if (failures.length === 1) throw failures[0];
    if (failures.length) throw new AggregateError(failures, 'Queued shutdown fixture cleanup failed.', { cause: failures[0] });
    owned.remove();
  });
  service.store.statement = function (sql) {
    if (sql === snapshotSql) snapshots++;
    const prepared = statement.call(this, sql); if (sql !== deleteSql) return prepared;
    return nativeStatementProxy(prepared, (...args) => {
      assert.equal(service.store.db.isTransaction, false);
      try { return prepared.run(...args); }
      catch (error) {
        assert.equal(error.code, 'ERR_SQLITE_ERROR'); assert.equal(error.errcode & 0xff, 5); refusals++;
        if (!persistent) releaseWriter(writer); throw error;
      }
    });
  };
  service.store.acquireSlot = function (...args) {
    const token = acquire.apply(this, args);
    if (!token && !triggered) {
      triggered = true;
      try { writer = lockWriter(owned); } catch (error) { writer = error.writer; throw error; }
      closing = service.close(); closing.catch(() => {});
    }
    return token;
  };
  if (persistent) service.store.cancelSlotWaiterWithRetry = async function (...args) {
    try { return await settle.apply(this, args); }
    catch (error) { await drainWriter(writer); throw error; }
  };
  const run = service.start(input), result = await service.completion(run.id); await closing;
  assert.equal(triggered, true); assert.equal(calls, 0); assert.equal(result.steps.length, 0); assert.equal(snapshots, 1); assert.ok(refusals > 0);
  assert.equal(result.status, persistent ? 'failed' : 'cancelled');
  if (persistent) assert.match(result.error, /Could not remove a shared workflow call waiter: database is locked/);
  assert.equal(service.store.db.isOpen, false);
});

function metadataFixture(t) {
  const owned = heldFixture(), store = new CouncilStore(owned.state), owner = randomUUID(), first = queued(), second = queued();
  store.registerInstance(owner, process.pid, 1); store.createRun(first, owner); store.createRun(second, owner);
  const statement = store.statement;
  t.after(() => {
    const failures = [];
    try {
      store.statement = statement; if (store.db.isTransaction) store.db.exec('ROLLBACK');
      store.finalizeRun({ ...first, status: 'cancelled' }); store.finalizeRun({ ...second, status: 'cancelled' }); store.unregisterInstance(owner);
    } catch (error) { failures.push(error); }
    finally {
      try { store.close(); } catch (error) { failures.push(error); }
      try { owned.finish(); } catch (error) { failures.push(error); }
    }
    if (failures.length === 1) throw failures[0];
    if (failures.length) throw new AggregateError(failures, 'Waiter metadata fixture cleanup failed.', { cause: failures[0] });
    owned.remove();
  });
  const row = key => store.db.prepare('SELECT CAST(sequence AS TEXT) AS generation,run_id FROM slot_waiters WHERE instance_id=? AND waiter_id=?').get(owner, key);
  return { ...owned, store, owner, first, second, statement, row, queue(run, key) { assert.equal(store.acquireSlot(owner, run.id, 1, key), undefined); return row(key); } };
}
const acknowledgementLoss = () => Object.assign(new Error('Synthetic post-delete acknowledgement loss; not a native COMMIT claim.'), { code: 'ERR_SQLITE_ERROR', errcode: 5 });

test('waiter cleanup pins one generation across lost responses and legitimate same-ID reuse', async t => {
  const wire = metadataFixture(t), { store, owner } = wire;
  for (const sameRun of [false, true]) {
    const key = 'reused-' + sameRun, original = wire.queue(wire.first, key); let snapshots = 0, deletions = 0, newer;
    store.statement = function (sql) {
      if (sql === snapshotSql) snapshots++;
      const prepared = wire.statement.call(this, sql); if (sql !== deleteSql) return prepared;
      return nativeStatementProxy(prepared, (...args) => {
        assert.equal(typeof args[2], 'bigint'); const result = prepared.run(...args);
        if (++deletions === 1) { newer = wire.queue(sameRun ? wire.first : wire.second, key); throw acknowledgementLoss(); }
        return result;
      });
    };
    await store.cancelSlotWaiterWithRetry(owner, key);
    assert.equal(snapshots, 1); assert.notEqual(original.generation, newer.generation); assert.equal(wire.row(key).generation, newer.generation);
    store.statement = wire.statement; store.cancelSlotWaiter(owner, key);
  }
});

test('waiter cleanup never retries an uncertain initial snapshot or rolls back its caller transaction', async t => {
  const wire = metadataFixture(t), { store, owner } = wire, key = 'failed-snapshot', error = acknowledgementLoss();
  wire.queue(wire.first, key); let snapshots = 0, deletes = 0, replacement;
  store.statement = function (sql) {
    if (sql === deleteSql) deletes++;
    if (sql === snapshotSql) { snapshots++; store.cancelSlotWaiter(owner, key); replacement = wire.queue(wire.second, key); throw error; }
    return wire.statement.call(this, sql);
  };
  await assert.rejects(store.cancelSlotWaiterWithRetry(owner, key), actual => actual === error);
  assert.equal(snapshots, 1); assert.equal(deletes, 0); assert.equal(wire.row(key).generation, replacement.generation);
  store.statement = wire.statement;
  store.db.exec('BEGIN IMMEDIATE'); await assert.rejects(store.cancelSlotWaiterWithRetry(owner, key), /snapshot inside an open transaction/);
  assert.equal(store.db.isTransaction, true); store.db.exec('ROLLBACK');
  store.statement = function (sql) {
    const prepared = wire.statement.call(this, sql); if (sql !== snapshotSql) return prepared;
    return { get(...args) { const value = prepared.get(...args); store.db.exec('BEGIN IMMEDIATE'); return value; } };
  };
  await assert.rejects(store.cancelSlotWaiterWithRetry(owner, key), /snapshot left an open transaction/);
  assert.equal(store.db.isTransaction, true); store.db.exec('ROLLBACK'); store.statement = wire.statement; assert.ok(wire.row(key));
});

test('waiter cleanup preserves64-bit generations, owner authority and cleanup under quarantine', async t => {
  const wire = metadataFixture(t), { store, owner } = wire, key = 'owner-loss'; wire.queue(wire.first, key); let deletes = 0;
  store.statement = function (sql) {
    const prepared = wire.statement.call(this, sql); if (sql !== deleteSql) return prepared;
    return nativeStatementProxy(prepared, (...args) => {
      const result = prepared.run(...args);
      if (++deletes === 1) { const remove = store.db.prepare('DELETE FROM instances WHERE id=?'); remove.setReadBigInts(true); remove.run(owner); throw acknowledgementLoss(); }
      return result;
    });
  };
  await assert.rejects(store.cancelSlotWaiterWithRetry(owner, key), /not registered to this live process/); assert.equal(deletes, 1);
  store.statement = wire.statement; store.registerInstance(owner, process.pid, 1);
  const quarantineKey = 'quarantine-existing'; wire.queue(wire.first, quarantineKey);
  await store.cancelSlotWaiterWithRetry(owner, randomUUID()); assert.ok(wire.row(quarantineKey));
  await assert.rejects(store.cancelSlotWaiterWithRetry(randomUUID(), quarantineKey), /not registered to this live process/); assert.ok(wire.row(quarantineKey));
  const insert = store.db.prepare('INSERT INTO slot_waiters(sequence,instance_id,run_id,waiter_id) VALUES(?,?,?,?)'); insert.setReadBigInts(true);
  for (const [generation, label] of [[9007199254740997n, 'above-safe-integer'], [9223372036854775807n, 'max-sqlite-integer']]) {
    insert.run(generation, owner, wire.first.id, label); assert.equal(wire.row(label).generation, generation.toString());
    await store.cancelSlotWaiterWithRetry(owner, label); assert.equal(wire.row(label), undefined);
  }
  store.reportProcessCleanupIncident({ reportKey: 'owned-waiter-cleanup-quarantine', instanceId: owner, runId: wire.first.id, stage: 'agent_call' });
  await store.cancelSlotWaiterWithRetry(owner, quarantineKey); assert.equal(wire.row(quarantineKey), undefined); assert.equal(store.dispatchQuarantinePage().blocked, true);
});

test('legacy asynchronous waiter cleanup holds local capacity until its cancellation drains', async t => {
  const owned = temporary(); let firstQueued, cleanupEntered, releaseCleanup, calls = 0;
  const queuedGate = new Promise(resolve => { firstQueued = resolve; }), entered = new Promise(resolve => { cleanupEntered = resolve; }), gate = new Promise(resolve => { releaseCleanup = resolve; });
  const firstId = randomUUID(), controller = new AbortController(), records = new Map();
  const persistence = { saveRun: run => records.set(run.id, structuredClone(run)), appendEvent() {},
    acquireSlot(_owner, runId) { if (runId === firstId) { firstQueued(); return undefined; } return randomUUID(); },
    async cancelSlotWaiter() { cleanupEntered(); await gate; }, releaseSlot() {} };
  const runner = { doctor: async () => [], run: async request => { calls++; return { agent: request.agent, status: 'completed', output: 'Offline legacy waiter evidence.', durationMs: 1 }; } };
  const engine = new WorkflowEngine(loadConfig(owned.root, { backend: 'mock', maxConcurrency: 1 }), runner, persistence, { instanceId: 'legacy-waiter-owner' });
  const first = engine.run(input, { id: firstId, signal: controller.signal }); first.catch(() => {}); let second;
  t.after(async () => { controller.abort(); releaseCleanup(); await bounded(Promise.allSettled([first, second]), 5000, 'The legacy waiter fixture did not drain.'); owned.remove(); });
  await bounded(queuedGate, 5000, 'The first legacy waiter did not queue.'); controller.abort();
  await bounded(entered, 5000, 'Legacy waiter cleanup was not reached.');
  second = engine.run(input, { signal: t.signal }); second.catch(() => {});
  await delay(25, undefined, { signal: t.signal }); assert.equal(calls, 0, 'The next local agent must wait for cancelled waiter cleanup.');
  releaseCleanup(); const results = await Promise.all([first, second]); assert.deepEqual(results.map(result => result.status), ['cancelled', 'completed']); assert.equal(calls, 1);
});
