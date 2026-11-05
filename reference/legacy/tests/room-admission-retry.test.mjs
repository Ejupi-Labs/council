import test from 'node:test';
import assert from 'node:assert/strict';
import { fork } from 'node:child_process';
import { mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { once } from 'node:events';
import { setTimeout as delay } from 'node:timers/promises';
import { CouncilService } from '../dist/service.js';
import { RoomManager } from '../dist/rooms.js';
import { loadConfig } from '../dist/config.js';
import { admissionRefusalOrigin } from '../dist/admission-attempt.js';
import { DispatchQuarantinedError } from '../dist/dispatch-quarantine.js';

function deferred() { let resolve; const promise = new Promise(accept => { resolve = accept; }); return { promise, resolve }; }
async function bounded(promise, label) {
  let timer;
  try { return await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(label + ' did not complete within 3 seconds.')), 3000); })]); }
  finally { clearTimeout(timer); }
}
function fixture(t, { tasks = 2, maxConcurrency = 2, handler } = {}) {
  const parent = realpathSync.native(tmpdir()), root = mkdtempSync(join(parent, 'council-admission-loop-')), calls = [], starts = [], admissions = [], refusals = [], boards = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 32, maxConcurrency }), { doctor: async () => [], run: async request => {
    const marker = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt), data = marker && JSON.parse(marker[1]);
    calls.push(request);
    if (data) return { agent: request.agent, status: 'completed', durationMs: 1, output: JSON.stringify({ vote: 'approve', rationale: 'Offline bounded evidence.', assignments: Object.fromEntries(data.tasks.map(task => [task.id, 'codex'])) }) };
    return handler ? await handler(request) : { agent: request.agent, status: 'completed', output: 'Exactly one offline task call.', durationMs: 1 };
  } });
  const manager = new RoomManager(service), peers = [], writers = [];
  manager.subscribe(room => boards.push(room.tasks.map(task => task.status)));
  const originals = { start: service.start, createRun: service.store.createRun, transaction: service.store.transaction, allowed: service.assertDispatchAllowed, launch: manager.launch };
  service.start = function (input, provenance) {
    const id = /Room task (task-\d+):/.exec(input.task)?.[1]; starts.push(id ?? 'discussion');
    const run = originals.start.call(this, input, provenance); admissions.push({ task: id ?? 'discussion', id: run.id }); return run;
  };
  service.store.transaction = function (...args) {
    try { return originals.transaction.apply(this, args); }
    catch (error) {
      if (admissionRefusalOrigin(error)) {
        refusals.push(error);
        for (const writer of writers) if (writer.release && !writer.releaseRequested) {
          writer.releaseRequested = true;
          queueMicrotask(() => {
            assert.equal(manager.store.db.isTransaction, false); assert.equal(service.store.db.isTransaction, false);
            writer.release();
          });
        }
      }
      throw error;
    }
  };
  t.after(async () => {
    const failures = [];
    service.start = originals.start; service.store.createRun = originals.createRun; service.store.transaction = originals.transaction; service.assertDispatchAllowed = originals.allowed; manager.launch = originals.launch;
    for (const writer of writers) {
      try {
        if (writer.child.exitCode === null && writer.child.signalCode === null) { if (writer.release) writer.release(); else writer.child.kill(); }
        const [code, signal] = await bounded(writer.exited, 'Owned admission writer drain'); assert.equal(code, 0); assert.equal(signal, null);
      }
      catch (error) { failures.push(error); writer.child.kill(); try { await bounded(writer.exited, 'Owned admission writer termination'); } catch (failure) { failures.push(failure); } }
    }
    // Only the inspected synthetic unknown-commit fixture may need explicit settlement.
    try {
      for (const summary of service.store.listRunSummaries(100)) if (summary.status === 'queued' && !service.jobs.has(summary.id)) {
        const run = service.store.getRun(summary.id); service.store.finalizeRun({ ...run, status: 'failed', error: 'Inspected synthetic fixture orphan; no replay.', updatedAt: new Date().toISOString() });
      }
    } catch (error) { failures.push(error); }
    for (const host of [manager, ...peers, service]) try { await host.close(); } catch (error) { failures.push(error); }
    if (failures.length) throw new AggregateError(failures, 'Owned admission fixture cleanup failed; inspect the retained fixture.');
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-admission-loop-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const room = manager.create({ title: 'Single-launch admission', brief: 'Run each bounded offline task once.', autoExecute: false, members: [{ agent: 'codex', role: 'lead' }],
    tasks: Array.from({ length: tasks }, (_, index) => ({ title: 'Task ' + (index + 1), brief: 'Retain exact evidence without replay.', permission: 'read' })) });
  return { root, service, manager, room, calls, starts, admissions, refusals, originals, boards,
    async ready() { manager.startDiscussion(room.id); assert.equal((await manager.wait(room.id, 10000)).status, 'ready'); },
    peer() { const peer = new RoomManager(service); peers.push(peer); return peer; },
    async lock(ms) {
      const releasePath = join(root, 'writer-' + writers.length + '.release');
      let filename, args;
      if (ms === undefined) {
        filename = new URL('./fixtures/admission-writer.mjs', import.meta.url);
        args = [join(service.config.stateDir, 'state.db'), releasePath, 'plain'];
      } else {
        filename = join(root, 'writer-' + writers.length + '.mjs');
        writeFileSync(filename, `import {DatabaseSync} from 'node:sqlite';const db=new DatabaseSync(process.argv[2]);db.exec('PRAGMA busy_timeout=250; BEGIN IMMEDIATE');process.send('locked');setTimeout(()=>{db.exec('ROLLBACK');db.close();process.send('released');process.disconnect();},Number(process.argv[3]));`);
        args = [join(service.config.stateDir, 'state.db'), String(ms)];
      }
      const child = fork(filename, args, { windowsHide: true, stdio: ['ignore', 'ignore', 'pipe', 'ipc'] });
      const exited = once(child, 'close'), locked = once(child, 'message');
      child.stderr.resume();
      const writer = { child, exited, ...(ms === undefined ? { release: () => writeFileSync(releasePath, 'release') } : {}) }; writers.push(writer);
      assert.equal((await bounded(Promise.race([locked, exited.then(([code]) => { throw new Error('Owned admission writer exited before lock: ' + code); })]), 'Native writer acquisition'))[0], 'locked'); return writer;
    },
  };
}

test('real state BEGIN refusal admits each sibling once and keeps assigned retry promises tracked', { timeout: 15000 }, async t => {
  const wire = fixture(t); await wire.ready(); const writer = await wire.lock();
  wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id, 10000); await writer.exited;
  assert.equal(result.status, 'completed'); assert.ok(wire.refusals.length > 0);
  assert.ok(wire.boards.some(tasks => tasks.includes('completed') && tasks.includes('assigned')), 'A sibling completes while another task remains assigned awaiting admission.');
  assert.equal(wire.admissions.length, 3); assert.equal(new Set(result.tasks.map(task => task.runId)).size, 2);
  assert.deepEqual(wire.admissions.filter(value => value.task !== 'discussion').map(value => value.task).sort(), ['task-1', 'task-2']);
  assert.equal(wire.calls.length, 3); assert.equal(result.callsUsed, 3); assert.ok(result.tasks.every(task => task.status === 'completed'));
  assert.equal(wire.service.store.health().counts.activeRuns, 0); assert.equal(wire.service.store.health().counts.slots, 0);
});

for (const intervention of ['pause', 'cancel', 'guidance', 'quarantine']) {
  test(`fresh ${intervention} after a genuine refusal prevents another launch admission`, { timeout: 15000 }, async t => {
    const wire = fixture(t, { tasks: 1 }); await wire.ready(); const writer = await wire.lock();
    const transaction = wire.service.store.transaction; let requested = false;
    wire.service.store.transaction = function (...args) {
      try { return transaction.apply(this, args); } catch (error) {
        if (!requested && admissionRefusalOrigin(error)) {
          requested = true;
          queueMicrotask(() => {
            assert.equal(wire.manager.store.db.isTransaction, false); assert.equal(wire.service.store.db.isTransaction, false);
            if (intervention === 'guidance') wire.peer().postMessage(wire.room.id, { author: 'PM', body: 'Fresh guidance must precede any new task.' });
            else if (intervention === 'quarantine') wire.service.assertDispatchAllowed = () => { throw new DispatchQuarantinedError({ blocked: true, currentRevision: '1', openCount: 1 }); };
            else wire.peer()[intervention](wire.room.id);
          });
        }
        throw error;
      }
    };
    wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id, 10000); await writer.exited;
    assert.equal(requested, true);
    assert.equal(result.status, intervention === 'pause' ? 'paused' : intervention === 'cancel' ? 'cancelled' : intervention === 'guidance' ? 'needs_attention' : 'failed');
    assert.equal(wire.calls.length, 1); assert.equal(wire.admissions.length, 1); assert.equal(result.tasks[0].runId, undefined);
    if (intervention === 'cancel') assert.match(result.error, /cancel/i);
    if (intervention === 'quarantine') assert.match(result.error, /quarantin|cleanup/i);
  });
}

test('native-shaped unknown postcommit createRun failure without returned ID never retries', { timeout: 15000 }, async t => {
  const wire = fixture(t, { tasks: 1 }); await wire.ready(); let writes = 0;
  wire.service.store.createRun = function (...args) { writes++; wire.originals.createRun.apply(this, args); throw Object.assign(new Error('Unknown postcommit acknowledgement.'), { code: 'ERR_SQLITE_ERROR', errcode: 5 }); };
  wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id, 10000);
  assert.equal(result.status, 'failed'); assert.match(result.error, /Unknown postcommit/); assert.equal(writes, 1);
  assert.equal(wire.calls.length, 1); assert.equal(wire.starts.filter(value => value === 'task-1').length, 1);
  assert.equal(wire.service.store.listRunSummaries(10).length, 2); assert.equal(result.tasks[0].runId, undefined);
});

for (const reason of [new Error('Permanent sibling admission failure.'), undefined]) {
  test(`permanent sibling ${reason ? 'error' : 'undefined rejection'} stops only unadmitted retries while an admitted sibling drains`, { timeout: 15000 }, async t => {
    const entered = deferred(), finish = deferred(), allowAdmissions = deferred(); let activeRequest;
    const wire = fixture(t, { tasks: 3, maxConcurrency: 3, handler: async request => {
      assert.match(request.prompt, /Room task task-1:/); activeRequest = request; entered.resolve(); await finish.promise;
      return { agent: request.agent, status: 'completed', output: 'Previously admitted sibling drained normally.', durationMs: 1 };
    } });
    try {
      await wire.ready();
      const launch = wire.manager.launch;
      wire.manager.launch = async function (...args) { if (args[3] !== 'task-1') await allowAdmissions.promise; return await launch.apply(this, args); };
      const start = wire.service.start;
      wire.service.start = function (input, provenance) { if (/Room task task-3:/.test(input.task)) throw reason; return start.call(this, input, provenance); };
      wire.manager.execute(wire.room.id); await bounded(entered.promise, 'Admitted sibling entry');
      const writer = await wire.lock(); allowAdmissions.resolve();
      const refusalDeadline = performance.now() + 3000;
      while (!wire.refusals.length) { assert.ok(performance.now() < refusalDeadline, 'The real BEGIN refusal must be observed.'); await delay(5); }
      await delay(60); assert.equal(activeRequest.signal.aborted, false, 'Admission-only stop cannot cancel an already admitted sibling.');
      await writer.exited; finish.resolve(); const result = await wire.manager.wait(wire.room.id, 10000);
      assert.equal(result.status, 'failed'); assert.equal(result.tasks[0].status, 'completed');
      assert.equal(result.tasks[1].runId, undefined); assert.equal(result.tasks[2].runId, undefined);
      assert.equal(wire.starts.filter(id => id === 'task-2').length, 1); assert.equal(wire.calls.length, 2); assert.equal(result.callsUsed, 2);
      assert.equal(activeRequest.signal.aborted, false);
      assert.equal(result.error, reason ? reason.message : 'Room execution failed.');
    } finally { allowAdmissions.resolve(); finish.resolve(); }
  });
}

test('fixed deadline is rechecked after a successful fresh authority read before another admission', { timeout: 15000 }, async t => {
  const wire = fixture(t, { tasks: 1 }); await wire.ready(); const writer = await wire.lock();
  const originalNow = Object.getOwnPropertyDescriptor(performance, 'now'), nativeNow = performance.now.bind(performance);
  let offset = 0, advanced = false;
  Object.defineProperty(performance, 'now', { configurable: true, value: () => nativeNow() + offset });
  const allowed = wire.service.assertDispatchAllowed;
  wire.service.assertDispatchAllowed = function () { const result = allowed.call(this); if (wire.refusals.length && !advanced) { advanced = true; offset = 6000; } return result; };
  try {
    wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id, 10000);
    assert.equal(advanced, true); assert.equal(result.status, 'failed'); assert.match(result.error, /five-second retry window/);
    assert.equal(wire.starts.filter(id => id === 'task-1').length, 1); assert.equal(wire.admissions.length, 1); assert.equal(wire.calls.length, 1);
  } finally { if (originalNow) Object.defineProperty(performance, 'now', originalNow); else delete performance.now; await writer.exited; }
});

test('a resumed execution retains completed workflow identities while retrying only its remaining launch', { timeout: 15000 }, async t => {
  const wire = fixture(t, { maxConcurrency: 1 }); await wire.ready(); let requested = false;
  wire.manager.store.mutate(wire.room.id, room => { room.maxCalls = 3; }, { requireIdle: true });
  wire.manager.subscribe(room => { if (!requested && room.tasks[0].status === 'completed' && room.tasks[1].status === 'assigned') { requested = true; wire.manager.pause(room.id); } });
  wire.manager.execute(wire.room.id); const paused = await wire.manager.wait(wire.room.id, 10000);
  assert.equal(paused.status, 'paused'); const previous = paused.tasks[0].runId;
  const writer = await wire.lock(); wire.manager.resume(wire.room.id); const result = await wire.manager.wait(wire.room.id, 10000); await writer.exited;
  assert.equal(result.status, 'completed'); assert.equal(result.tasks[0].runId, previous); assert.equal(result.callsUsed, 3);
  assert.equal(wire.calls.length, 3); assert.equal(wire.admissions.length, 3); assert.ok(wire.refusals.length > 0);
  assert.equal(wire.admissions.filter(value => value.task === 'task-1').length, 1); assert.equal(wire.admissions.filter(value => value.task === 'task-2').length, 1);
});

test('sustained native writer contention expires one fixed launch window instead of sliding until release', { timeout: 15000 }, async t => {
  const wire = fixture(t, { tasks: 1 }); await wire.ready();
  // Isolate this launch deadline from the independent host-heartbeat fault below.
  clearInterval(wire.service.heartbeat);
  const writer = await wire.lock(6500);
  wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id, 10000);
  assert.equal(result.status, 'failed'); assert.match(result.error, /five-second retry window/);
  assert.ok(wire.refusals.length >= 3); assert.equal(wire.admissions.length, 1); assert.equal(wire.calls.length, 1);
  assert.equal(result.tasks[0].runId, undefined); assert.equal(result.callsUsed, 1);
  await writer.exited;
});

test('live-heartbeat writer contention still stops admission for a fresh host fault or the launch deadline', { timeout: 15000 }, async t => {
  const wire = fixture(t, { tasks: 1 }); await wire.ready(); const writer = await wire.lock(6500);
  wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id, 10000);
  assert.equal(result.status, 'failed'); assert.match(result.error, /five-second retry window|state heartbeat failed/);
  assert.equal(wire.admissions.length, 1); assert.equal(wire.calls.length, 1); assert.equal(result.callsUsed, 1);
  assert.equal(result.tasks[0].runId, undefined);
  await writer.exited;
});

test('an admitted publication subscriber can cancel its parent and still create an independent room', { timeout: 15000 }, async t => {
  const wire = fixture(t, { tasks: 1 }); await wire.ready(); let observed = false, independent, unexpected;
  wire.manager.subscribe(room => {
    if (observed || room.id !== wire.room.id || room.tasks[0].status !== 'running') return;
    observed = true; wire.manager.cancel(room.id);
    try { independent = wire.manager.create({ title: 'Independent room', brief: 'This board is independent of the cancelled launch.', members: [{ agent: 'codex', role: 'lead' }], tasks: [] }); }
    catch (error) { unexpected = error; }
  });
  wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id, 10000);
  assert.equal(observed, true); assert.equal(unexpected, undefined); assert.equal(independent.status, 'idle');
  assert.notEqual(independent.id, result.id); assert.equal(wire.manager.get(independent.id).status, 'idle');
  assert.equal(result.status, 'cancelled'); assert.equal(result.tasks[0].status, 'cancelled');
  const admitted = wire.admissions.filter(value => value.task === 'task-1');
  assert.equal(admitted.length, 1); assert.equal(result.tasks[0].runId, admitted[0].id);
  assert.equal(wire.service.getRun(admitted[0].id).status, 'cancelled');
  const history = wire.manager.messagePage(result.id, 100).messages;
  assert.equal(history.filter(message => message.runId === admitted[0].id && message.body === 'Dispatched task-1.').length, 1);
  assert.equal(wire.service.store.health().counts.activeRuns, 0); assert.equal(wire.service.store.health().counts.slots, 0);
});
