import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

test('room metadata validates IDs, excludes history and clones durable or emergency state without provider calls', async t => {
  const root = mkdtempSync(join(tmpdir(), 'council-room-metadata-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), { doctor: async () => [], run: async () => { assert.fail('Metadata must never start a provider.'); } });
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  const manager = service.rooms;
  const room = manager.create({ title: 'Metadata', brief: 'Monitor coordination without chat body reads.', members: [{ agent: 'codex', role: 'worker' }] });
  manager.postMessage(room.id, { author: 'PM', body: 'Complete retained chat evidence. '.repeat(10000) });
  const original = manager.get(room.id), { messages, ...expected } = original;
  const metadata = manager.metadata(room.id); assert.deepEqual(metadata, expected); assert.equal(Object.hasOwn(metadata, 'messages'), false);
  metadata.members[0].agent = 'Consumer mutation'; assert.deepEqual(manager.metadata(room.id), expected);
  for (const invalid of ['', undefined, 'x'.repeat(101), 'missing-room']) assert.throws(() => manager.metadata(invalid), /404/);
  const emergency = { ...original, status: 'failed', error: 'Retained emergency state.' };
  manager.emergency.set(room.id, emergency);
  const saved = manager.metadata(room.id); assert.equal(saved.status, 'failed'); assert.equal(saved.error, emergency.error);
  assert.equal(Object.hasOwn(saved, 'messages'), false); saved.members[0].agent = 'Other consumer mutation';
  assert.equal(manager.metadata(room.id).members[0].agent, 'codex');
  manager.emergency.delete(room.id);
});

test('targeted recovery detects an owner lost after manager startup without replaying work or decoding history', async t => {
  const root = mkdtempSync(join(tmpdir(), 'council-room-targeted-recovery-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), { doctor: async () => [], run: async () => { assert.fail('Recovery must never start or replay a provider.'); } });
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  const manager = service.rooms;
  const input = { title: 'Targeted recovery', brief: 'Retain completed evidence and reconcile only a selected dead owner.', members: [{ agent: 'codex', role: 'worker' }],
    tasks: [1, 2, 3].map(index => ({ title: 'Task ' + index, brief: 'Offline recovery evidence.', permission: 'read' })) };
  const target = manager.create(input), other = manager.create(input);
  for (const room of [target, other]) manager.store.mutate(room.id, current => {
    current.status = 'running'; current.tasks[0].status = 'completed'; current.tasks[0].runId = 'retained-completed-run';
    current.tasks[0].workspaces = []; current.tasks[0].resultPreview = 'Exact retained success.';
    current.tasks[1].status = 'running'; current.tasks[1].runId = 'retained-unfinished-run';
  }, { claim: { id: 'dead-' + room.id, pid: 987654 } });
  const originalKill = process.kill;
  try {
    process.kill = (pid, signal) => { if (pid === 987654 && signal === 0) throw Object.assign(new Error('Proven-dead fixture owner'), { code: 'ESRCH' }); return originalKill(pid, signal); };
    assert.equal(manager.metadata(target.id).status, 'running', 'The owner was lost after this long-lived manager initialized.');
    const originalGet = manager.store.get;
    manager.store.get = () => { assert.fail('Targeted recovery must not read chat history.'); };
    try { manager.recover(target.id); } finally { manager.store.get = originalGet; }
    assert.equal(manager.metadata(target.id).status, 'failed'); assert.equal(manager.metadata(other.id).status, 'running', 'Recovery is scoped to the requested room.');
    const recovered = manager.get(target.id);
    assert.equal(recovered.tasks[0].status, 'completed'); assert.equal(recovered.tasks[0].runId, 'retained-completed-run');
    assert.deepEqual(recovered.tasks[0].workspaces, []); assert.equal(recovered.tasks[0].resultPreview, 'Exact retained success.');
    assert.equal(recovered.tasks[1].status, 'failed'); assert.equal(recovered.tasks[1].runId, 'retained-unfinished-run');
    assert.match(recovered.error, /owning process exited/i); assert.match(recovered.messages.at(-1).body, /not replayed/);
    assert.throws(() => manager.recover('missing-room'), /404/);
    const revision = recovered.revision; manager.recover(target.id); assert.equal(manager.metadata(target.id).revision, revision, 'Recovery remains idempotent.');
  } finally { process.kill = originalKill; }
});
