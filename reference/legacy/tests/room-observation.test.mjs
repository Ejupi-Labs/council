import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

function fixture(t) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-observation-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' })), manager = service.rooms;
  t.after(async () => {
    await service.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-observation-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { manager };
}

test('coordination observations preserve exact task counts and control state without decoding task briefs or message previews', t => {
  const { manager } = fixture(t), large = 'DO_NOT_DECODE_THE_LARGE_ROOM_DOCUMENT ' + 'complete private task detail 🧭 '.repeat(12000);
  const record = manager.create({ title: 'Compact coordination', brief: large, members: [{ agent: 'codex', role: 'worker' }],
    tasks: Array.from({ length: 4 }, (_, index) => ({ title: 'Task ' + index, brief: large, permission: 'read' })) });
  const stored = manager.store.mutate(record.id, room => {
    room.status = 'running'; room.instructionRevision = 7; room.pauseRequested = true; room.callsUsed = 9;
    room.error = 'Full control failure evidence.';
    ['completed', 'failed', 'cancelled', 'pending'].forEach((status, index) => { room.tasks[index].status = status; });
  }, { claim: { id: 'live-fixture-owner', pid: process.pid } });
  const parse = JSON.parse;
  JSON.parse = function (source, ...args) {
    assert.ok(!String(source).includes('DO_NOT_DECODE_THE_LARGE_ROOM_DOCUMENT'), 'Coordination must not transfer the whole room document into JavaScript.');
    return parse.call(this, source, ...args);
  };
  let observed;
  try {
    observed = manager.observation(record.id);
    assert.equal(manager.messagePage(record.id).messages.length, 1);
    manager.recover(record.id);
  } finally { JSON.parse = parse; }
  assert.deepEqual(observed, { id: record.id, title: record.title, status: 'running', revision: stored.revision, instructionRevision: 7,
    taskCount: 4, completedTasks: 1, failedTasks: 2, callsUsed: 9, maxCalls: record.maxCalls, error: stored.error, pauseRequested: true });
  assert.ok(Buffer.byteLength(JSON.stringify(observed)) < 1024);
  observed.title = 'Caller overwrite';
  assert.equal(manager.observation(record.id).title, record.title);
  manager.store.mutate(record.id, room => { delete room.instructionRevision; room.error = ''; room.pauseRequested = false; });
  const legacy = manager.observation(record.id);
  assert.equal(legacy.instructionRevision, 0); assert.equal(legacy.error, undefined); assert.equal(legacy.pauseRequested, undefined);
});

test('emergency observations retain authoritative in-memory failures and independent scalar snapshots', t => {
  const { manager } = fixture(t);
  const record = manager.create({ title: 'Retained emergency', brief: 'Preserve failed partial evidence.', members: [{ agent: 'codex', role: 'worker' }],
    tasks: [{ title: 'Partial task', brief: 'Inspect only.', permission: 'read' }] });
  const emergency = structuredClone(record);
  emergency.status = 'failed'; emergency.revision += 3; emergency.error = 'Durable state unavailable.';
  emergency.tasks[0].status = 'failed'; emergency.callsUsed = 1;
  manager.emergency.set(record.id, emergency);
  const read = manager.store.observation;
  manager.store.observation = () => { assert.fail('Emergency evidence must not be replaced by stale durable state.'); };
  try {
    const first = manager.observation(record.id);
    assert.equal(first.status, 'failed'); assert.equal(first.failedTasks, 1); assert.equal(first.error, emergency.error);
    first.error = 'Caller overwrite'; first.failedTasks = 0;
    assert.equal(manager.observation(record.id).error, emergency.error);
    assert.equal(manager.observation(record.id).failedTasks, 1);
  } finally { manager.store.observation = read; manager.emergency.delete(record.id); }
  for (const invalid of ['', 'x'.repeat(101), null, {}]) assert.throws(() => manager.observation(invalid), /Room not found/);
  assert.throws(() => manager.observation('missing'), /Room not found/);
});
