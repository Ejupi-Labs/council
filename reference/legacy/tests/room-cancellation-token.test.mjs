import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { RoomManager } from '../dist/rooms.js';
import { loadConfig } from '../dist/config.js';

function deferred() { let resolve; const promise = new Promise(accept => { resolve = accept; }); return { promise, resolve }; }
async function entered(promise) {
  let timer;
  try { await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Both offline tasks must enter.')), 3000); })]); }
  finally { clearTimeout(timer); }
}
const input = (title, tasks = 1) => ({ title, brief: 'Inspect only the disposable offline fixture.', autoExecute: false,
  members: [{ agent: 'codex', role: 'lead' }], tasks: Array.from({ length: tasks }, (_, index) => ({ title: 'Task ' + (index + 1), brief: 'Retain honest offline evidence.', permission: 'read' })) });
function fixture(t, run) {
  const parent = realpathSync.native(tmpdir()), root = mkdtempSync(join(parent, 'council-cancel-token-')), calls = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 32, maxConcurrency: 2 }), {
    doctor: async () => [], run: async request => {
      calls.push(request);
      const marker = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
      if (marker) {
        const ballot = JSON.parse(marker[1]);
        return { agent: request.agent, status: 'completed', durationMs: 1, output: JSON.stringify({ vote: 'approve', rationale: 'Bounded offline evidence.', assignments: Object.fromEntries(ballot.tasks.map(task => [task.id, 'codex'])) }) };
      }
      return await run(request);
    },
  });
  const manager = new RoomManager(service), mutate = manager.store.mutate;
  t.after(async () => {
    manager.store.mutate = mutate; await manager.close(); await service.close();
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-cancel-token-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { service, manager, mutate, calls };
}

for (const order of ['fault-first', 'cancel-first', 'cancel-first-with-publication-error']) {
  test(`actual cross-room emergency retains real failure with ${order}`, { timeout: 15000 }, async t => {
    const both = deferred(), gates = [deferred(), deferred()]; let taskCalls = 0;
    const wire = fixture(t, async request => {
      const ordinal = taskCalls++; if (taskCalls === 2) both.resolve();
      if (!request.signal.aborted) await new Promise(resolve => request.signal.addEventListener('abort', resolve, { once: true }));
      await gates[ordinal].promise;
      return { agent: request.agent, status: 'cancelled', output: 'Retained cancelled fake task evidence.', durationMs: 1 };
    });
    let observed, firstPublished = false, publicationFailed = false, cancelAccepted = false, emergencyObserved = false;
    try {
      const target = wire.manager.create(input('Target room', 2)), trigger = wire.manager.create(input('Fault source room'));
      wire.manager.startDiscussion(target.id); assert.equal((await wire.manager.wait(target.id)).status, 'ready');
      wire.manager.store.mutate = function (id, change, options) {
        if (id === trigger.id && options?.owner) throw new Error('Injected permanent writer outage for only the trigger room.');
        const before = this.metadata(id);
        const result = wire.mutate.call(this, id, room => {
          change(room);
          if (order === 'cancel-first-with-publication-error' && id === target.id && firstPublished && !publicationFailed
            && before.tasks[1].status === 'running' && room.tasks[1].status === 'cancelled') {
            publicationFailed = true; throw new Error('Later real task publication rollback after cross-room fault.');
          }
        }, options);
        if (id === target.id && !firstPublished && before.tasks[0].status === 'running' && result.tasks[0].status === 'cancelled') {
          firstPublished = true; setImmediate(gates[1].resolve);
        }
        return result;
      };
      wire.manager.subscribe(room => {
        if (room.id !== trigger.id || room.status !== 'failed' || emergencyObserved) return;
        emergencyObserved = true;
        const job = wire.manager.jobs.get(target.id);
        if (order.startsWith('cancel-first')) { wire.manager.cancel(target.id); cancelAccepted = true; }
        // The emergency publication precedes the stock cross-room propagation loop.
        queueMicrotask(() => {
          if (order === 'fault-first') { wire.manager.cancel(target.id); cancelAccepted = true; }
          observed = { token: job.acceptedCancellationReason, reason: job.controller.signal.reason, fault: job.failure };
          gates[0].resolve();
        });
      });
      wire.manager.execute(target.id); await entered(both.promise); wire.manager.startDiscussion(trigger.id);
      const result = await wire.manager.wait(target.id);
      assert.equal((await wire.manager.wait(trigger.id)).status, 'failed'); assert.equal(cancelAccepted, true);
      assert.ok(observed.fault instanceof Error); assert.equal(result.status, 'failed');
      if (order === 'fault-first') { assert.equal(observed.token, undefined); assert.equal(observed.reason, observed.fault); }
      else { assert.ok(observed.token instanceof Error); assert.equal(observed.reason, observed.token); assert.notEqual(observed.reason, observed.fault); }
      if (order === 'cancel-first-with-publication-error') {
        assert.equal(publicationFailed, true); assert.equal(result.error, 'Later real task publication rollback after cross-room fault.');
      } else assert.equal(result.error, observed.fault.message);
      assert.match(result.error, /persistence failed|publication rollback/); assert.doesNotMatch(result.error, /Room cancelled|Cancellation was accepted/);
      assert.ok(result.tasks.every(task => task.runId && task.status === 'cancelled'));
      assert.ok(result.tasks.every(task => wire.service.getRun(task.runId).status === 'cancelled'));
      assert.equal(result.callsUsed, 3); assert.equal(wire.calls.length, 3); assert.equal(wire.manager.store.metadata(target.id).status, 'failed');
      assert.equal(wire.service.store.health().counts.activeRuns, 0); assert.equal(wire.service.store.health().counts.slots, 0);
    } finally { for (const gate of gates) gate.resolve(); }
  });
}

test('an undefined rejection and a legacy cancellation flag cannot fabricate an accepted cancellation token', { timeout: 15000 }, async t => {
  const wire = fixture(t, async () => { throw new Error('No task call should be dispatched.'); });
  const queue = wire.manager.queue.bind(wire.manager);
  wire.manager.queue = (id, _operation, snapshot) => queue(id, async job => { job.cancelRequested = true; throw undefined; }, snapshot);
  const room = wire.manager.create(input('Undefined rejection'));
  wire.manager.startDiscussion(room.id); const result = await wire.manager.wait(room.id);
  assert.equal(result.status, 'failed'); assert.equal(result.error, 'Room execution failed.');
  assert.equal(result.callsUsed, 0); assert.equal(wire.calls.length, 0);
});
