import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { RoomManager } from '../dist/rooms.js';

function fixture(t) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-budget-')), calls = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 64, agents: {
    worker: { enabled: true, profile: 'codex' }, advisor: { enabled: true, profile: 'junie' },
  } }), { doctor: async () => [], run: async request => {
    calls.push(request);
    const match = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
    const data = match ? JSON.parse(match[1]) : undefined;
    return { agent: request.agent, status: 'completed', durationMs: 1, output: data
      ? JSON.stringify({ vote: 'approve', rationale: 'The complete offline evidence supports this assignment.',
        assignments: Object.fromEntries(data.tasks.map(task => [task.id, 'worker'])) }) : 'Verified the offline task without needing advice.' };
  } });
  const manager = new RoomManager(service);
  t.after(async () => {
    await manager.close(); await service.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-budget-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const create = overrides => manager.create({ title: 'Manual discussion budget', brief: 'Coordinate the complete offline fixture.',
    members: [{ agent: 'worker', role: 'lead', advisor: 'advisor', advisorMaxRequests: 3 }, { agent: 'advisor', role: 'advisor' }],
    tasks: [{ title: 'Later implementation', brief: 'Complete this task after execution is authorized.', permission: 'read' }],
    autoExecute: false, rounds: 1, ...overrides });
  return { manager, calls, create };
}

test('a manual discussion needs only its own call budget and stops before task or advisor execution', async t => {
  const { manager, calls, create } = fixture(t), room = create({ maxCalls: 1 });
  manager.startDiscussion(room.id);
  const ready = await manager.wait(room.id);
  assert.equal(ready.status, 'ready'); assert.equal(ready.decision.approved, true);
  assert.equal(ready.tasks[0].status, 'assigned'); assert.equal(ready.tasks[0].runId, undefined);
  assert.equal(ready.callsUsed, 1); assert.equal(calls.length, 1); assert.equal(calls[0].permission, 'read');
  assert.match(calls[0].prompt, /ROOM_BALLOT_DATA_BEGIN/);
  assert.throws(() => manager.execute(room.id), /requires up to 7 further calls.*remaining call budget 0/);
  assert.deepEqual(manager.get(room.id), ready, 'Rejected execution leaves the approved room and its evidence unchanged.');
  assert.equal(calls.length, 1);
});

test('manual discussion preserves the configured round bound and refunds rounds skipped after approval', async t => {
  const { manager, calls, create } = fixture(t), insufficient = create({ rounds: 3, maxCalls: 2 });
  assert.throws(() => manager.startDiscussion(insufficient.id), /requires up to 3 further calls/);
  assert.equal(manager.get(insufficient.id).status, 'idle'); assert.equal(calls.length, 0);
  const room = create({ rounds: 3, maxCalls: 3 }); manager.startDiscussion(room.id);
  const ready = await manager.wait(room.id);
  assert.equal(ready.status, 'ready'); assert.equal(ready.callsUsed, 1); assert.equal(calls.length, 1);
  assert.equal(ready.messages.filter(message => message.kind === 'opinion').length, 1);
});

test('manual execution still reserves the full advisor quota even when the worker may not use it', async t => {
  const { manager, calls, create } = fixture(t), insufficient = create({ maxCalls: 7 });
  manager.startDiscussion(insufficient.id); const ready = await manager.wait(insufficient.id);
  assert.equal(ready.status, 'ready'); assert.equal(ready.callsUsed, 1);
  assert.throws(() => manager.execute(insufficient.id), /requires up to 7 further calls.*remaining call budget 6/);
  assert.deepEqual(manager.get(insufficient.id), ready); assert.equal(calls.length, 1);
  const sufficient = create({ maxCalls: 8 }); manager.startDiscussion(sufficient.id); await manager.wait(sufficient.id);
  manager.execute(sufficient.id); const completed = await manager.wait(sufficient.id);
  assert.equal(completed.status, 'completed'); assert.equal(completed.callsUsed, 2);
  assert.equal(completed.tasks[0].status, 'completed'); assert.equal(calls.length, 3);
  assert.equal(calls.some(call => call.agent === 'advisor'), false, 'Unused advisor reservations are refunded, not dispatched.');
});

test('automatic execution retains full discussion, task and advisor reservations before any call', async t => {
  const { manager, calls, create } = fixture(t), insufficient = create({ autoExecute: true, maxCalls: 7 });
  assert.throws(() => manager.startDiscussion(insufficient.id), /requires up to 8 further calls/);
  assert.equal(manager.get(insufficient.id).status, 'idle'); assert.equal(calls.length, 0);
  const sufficient = create({ autoExecute: true, maxCalls: 8 }); manager.startDiscussion(sufficient.id);
  const completed = await manager.wait(sufficient.id);
  assert.equal(completed.status, 'completed'); assert.equal(completed.callsUsed, 2); assert.equal(calls.length, 2);
});
