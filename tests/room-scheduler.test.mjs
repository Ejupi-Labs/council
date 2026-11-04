import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { CouncilService } from '../dist/service.js';
import { RoomManager } from '../dist/rooms.js';
import { loadConfig } from '../dist/config.js';

function deferred() { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; }
function fixture(t, handler, { dependent = true } = {}) {
  const root = mkdtempSync(join(tmpdir(), 'council-room-scheduler-'));
  const requests = [], releases = [];
  const runner = { doctor: async () => [], run: async request => {
    const ballot = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
    if (ballot) {
      const data = JSON.parse(ballot[1]);
      return { agent: request.agent, status: 'completed', durationMs: 1, output: JSON.stringify({ vote: 'approve', rationale: 'Offline tasks have clear evidence.', assignments: Object.fromEntries(data.tasks.map(task => [task.id, 'codex'])) }) };
    }
    const id = /Room task (task-\d+):/.exec(request.prompt)?.[1];
    assert.ok(id); requests.push({ id, request });
    const output = await handler(id, request);
    return { agent: request.agent, status: request.signal.aborted ? 'cancelled' : 'completed', durationMs: 1, output: 'Complete evidence for ' + id, ...output };
  } };
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 32, maxConcurrency: 2 }), runner);
  const manager = new RoomManager(service);
  t.after(async () => {
    for (const release of releases) release();
    await manager.close(); await service.close(); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const gate = () => { const value = deferred(); releases.push(value.resolve); return value; };
  const room = manager.create({ title: 'Dependency scheduler', brief: 'Schedule bounded offline tasks.', members: [{ agent: 'codex', role: 'worker' }],
    tasks: [1, 2, 3].map(index => ({ title: 'Task ' + index, brief: 'Inspect the offline fixture and retain evidence.', permission: 'read', ...(index === 3 && dependent ? { dependsOn: ['task-1'] } : {}) })) });
  return { manager, service, room, requests, gate };
}
async function entered(promise, label) {
  const stop = new AbortController();
  try {
    const result = await Promise.race([promise.then(() => true), delay(3000, false, { signal: stop.signal }).catch(() => false)]);
    assert.equal(result, true, label);
  } finally { stop.abort(); }
}

test('a completed prerequisite releases a dependency immediately while an unrelated slow task is still running', { timeout: 10000 }, async t => {
  const started = [deferred(), deferred(), deferred()]; let gates;
  const state = fixture(t, async (id, request) => {
    const index = Number(id.slice(5)) - 1; started[index].resolve();
    if (id === 'task-3') assert.match(request.prompt, /task-1: completed\\nComplete evidence for task-1/);
    await gates[index].promise; return {};
  });
  gates = [state.gate(), state.gate(), state.gate()];
  state.manager.startDiscussion(state.room.id);
  await entered(Promise.all([started[0].promise, started[1].promise]), 'Both initial tasks must start.');
  gates[0].resolve(); await entered(started[2].promise, 'A free slot must admit the newly ready dependency without waiting for task-2.');
  const current = state.manager.get(state.room.id);
  assert.deepEqual(current.tasks.map(task => task.status), ['completed', 'running', 'running']);
  gates[2].resolve(); gates[1].resolve();
  const result = await state.manager.wait(state.room.id);
  assert.equal(result.status, 'completed'); assert.equal(result.callsUsed, 4);
  assert.deepEqual(state.requests.map(call => call.id), ['task-1', 'task-2', 'task-3']);
});

test('a failed task stops new admission while already admitted independent work drains and retains its outcome', { timeout: 10000 }, async t => {
  const started = [deferred(), deferred()]; let gates;
  const state = fixture(t, async (id) => {
    const index = Number(id.slice(5)) - 1; started[index]?.resolve();
    await gates[index].promise;
    return id === 'task-1' ? { status: 'failed', output: 'Retained failed evidence.', error: 'Offline task failure.' } : {};
  }, { dependent: false });
  gates = [state.gate(), state.gate()];
  const failed = deferred(); state.manager.subscribe(room => { if (room.tasks[0].status === 'failed') failed.resolve(); });
  state.manager.startDiscussion(state.room.id); await entered(Promise.all(started.map(value => value.promise)), 'Initial tasks must start.');
  gates[0].resolve(); await entered(failed.promise, 'The failed result must be retained.');
  assert.equal(state.requests.length, 2); assert.equal(state.requests[1].request.signal.aborted, false);
  gates[1].resolve(); const result = await state.manager.wait(state.room.id);
  assert.equal(result.status, 'failed'); assert.match(result.error, /Offline task failure/);
  assert.deepEqual(result.tasks.map(task => task.status), ['failed', 'completed', 'cancelled']);
  assert.equal(result.tasks[2].runId, undefined);
});

test('simultaneous task rejections are observed and a real task failure outranks a pending pause', { timeout: 10000 }, async t => {
  const started = [deferred(), deferred()]; let finish;
  const state = fixture(t, async id => {
    const index = Number(id.slice(5)) - 1; started[index]?.resolve(); await finish.promise;
    throw new Error('Offline concurrent failure from ' + id);
  }, { dependent: false });
  finish = state.gate(); state.manager.startDiscussion(state.room.id);
  await entered(Promise.all(started.map(value => value.promise)), 'Both failing tasks must already be admitted.');
  state.manager.pause(state.room.id); finish.resolve();
  const result = await state.manager.wait(state.room.id);
  assert.equal(result.status, 'failed'); assert.match(result.error, /Offline concurrent failure/);
  assert.deepEqual(result.tasks.map(task => task.status), ['failed', 'failed', 'cancelled']);
  assert.equal(result.checkpoint, undefined); assert.equal(result.pauseRequested, undefined);
  assert.equal(state.requests.length, 2); assert.ok(result.tasks.slice(0, 2).every(task => task.runId));
});

for (const intervention of ['pause', 'guidance', 'cancel']) {
  test(`${intervention} stops dependency admission while concurrent tasks drain without replay`, { timeout: 10000 }, async t => {
    const started = [deferred(), deferred()]; let gates;
    const state = fixture(t, async id => {
      const index = Number(id.slice(5)) - 1; started[index]?.resolve();
      if (index < 2) await gates[index].promise;
      return {};
    });
    gates = [state.gate(), state.gate()];
    state.manager.startDiscussion(state.room.id); await entered(Promise.all(started.map(value => value.promise)), 'Initial tasks must start.');
    if (intervention === 'guidance') state.manager.postMessage(state.room.id, { author: 'PM', body: 'Use the updated acceptance criteria.' });
    else state.manager[intervention](state.room.id);
    gates[0].resolve(); gates[1].resolve();
    const result = await state.manager.wait(state.room.id);
    assert.equal(result.status, intervention === 'guidance' ? 'needs_attention' : intervention === 'pause' ? 'paused' : 'cancelled');
    assert.equal(state.requests.length, 2); assert.equal(result.tasks[2].runId, undefined);
    if (intervention === 'cancel') {
      assert.ok(state.requests.every(call => call.request.signal.aborted));
      assert.ok(result.tasks.every(task => task.status === 'cancelled')); return;
    }
    assert.deepEqual(result.tasks.slice(0, 2).map(task => task.status), ['completed', 'completed']);
    assert.ok(state.requests.every(call => !call.request.signal.aborted));
    if (intervention === 'pause') state.manager.resume(state.room.id);
    else state.manager.startDiscussion(state.room.id);
    assert.equal((await state.manager.wait(state.room.id)).status, 'completed');
    assert.deepEqual(state.requests.map(call => call.id), ['task-1', 'task-2', 'task-3']);
  });
}
