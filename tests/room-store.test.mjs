import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { RoomStore } from '../dist/room-store.js';

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), 'council-room-store-'));
  const stores = [];
  t.after(() => { for (const store of stores) store.close(); rmSync(directory, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  return { directory, open() { const store = new RoomStore(directory); stores.push(store); return store; } };
}
function room(id = 'room-fixture') {
  const now = '2026-11-04T00:00:00.000Z';
  return { id, title: 'Project room', brief: 'A bounded offline fixture.', status: 'idle',
    members: [{ agent: 'codex', role: 'worker', weight: 1 }], tasks: [{ id: 'task-1', title: 'Task', brief: 'Inspect.', permission: 'read', status: 'pending' }],
    messages: [], rounds: 1, maxCalls: 16, autoExecute: true, contextLimit: 8000, callsUsed: 0, revision: 1, createdAt: now, updatedAt: now };
}
const message = index => ({ id: `message-${index}`, author: 'pm', kind: 'human', body: `${index}: ${'complete text '.repeat(50)}`, createdAt: '2026-11-04T00:00:00.000Z' });

test('room history stays complete through bounded previews, append operations and ascending pages', t => {
  const store = fixture(t).open(), original = room();
  original.messages = Array.from({ length: 123 }, (_, index) => message(index));
  store.insert(original);
  const preview = store.get(original.id);
  assert.equal(preview.messages.length, 101);
  assert.equal(preview.messages[0].id, 'history-preview');
  assert.equal(preview.messages[1].id, 'message-23');
  store.mutate(original.id, current => { current.messages.push(message(123)); });
  const complete = [];
  let cursor;
  do { const page = store.messagePage(original.id, 31, cursor); complete.push(...page.messages); cursor = page.nextCursor; } while (cursor !== null);
  assert.deepEqual(complete, [...original.messages, message(123)]);
  assert.ok(!complete.some(value => value.id === 'history-preview'));
  assert.throws(() => store.messagePage(original.id, 101), /bounds/);
  assert.throws(() => store.messagePage(original.id, 1, -1), /bounds/);
});

test('room claims fence competing managers and accepted cancellation wins final publication atomically', t => {
  const setup = fixture(t), owner = setup.open(), peer = setup.open();
  owner.insert(room());
  owner.mutate('room-fixture', value => { value.status = 'discussing'; }, { claim: { id: 'owner-a', pid: process.pid } });
  assert.throws(() => peer.mutate('room-fixture', () => {}, { claim: { id: 'owner-b', pid: process.pid } }), /active/);
  assert.throws(() => peer.mutate('room-fixture', () => {}, { owner: 'owner-b' }), /ownership/);
  assert.throws(() => peer.mutate('room-fixture', () => {}, { requireIdle: true }), /active/);
  peer.requestCancel('room-fixture');
  assert.equal(owner.cancelled('room-fixture', 'owner-a'), true);
  const final = owner.mutate('room-fixture', value => { value.status = 'ready'; value.tasks[0].status = 'assigned'; }, { owner: 'owner-a', release: true });
  assert.equal(final.status, 'cancelled');
  assert.equal(final.tasks[0].status, 'cancelled');
  assert.equal(peer.get('room-fixture').status, 'cancelled');
});

test('a failed room mutation rolls back its document and newly appended message together', t => {
  const store = fixture(t).open(); store.insert(room());
  const before = store.get('room-fixture');
  assert.throws(() => store.mutate('room-fixture', value => {
    value.messages.push(message(1), { ...message(2), id: '' }); value.brief = 'Changed brief';
  }), /message ID/);
  assert.deepEqual(store.get('room-fixture'), before);
  assert.deepEqual(store.messagePage('room-fixture').messages, []);
});

test('recovery fails proven-dead owners, retains prior successful work and never replays tasks', t => {
  const setup = fixture(t), store = setup.open();
  store.insert(room());
  store.mutate('room-fixture', value => {
    value.status = 'running'; value.tasks[0].status = 'running'; value.tasks[0].runId = 'retained-workflow'; value.messages.push(message(1));
  }, { claim: { id: 'dead-owner', pid: 987654 } });
  const originalKill = process.kill;
  try {
    process.kill = (pid, signal) => { if (pid === 987654 && signal === 0) throw Object.assign(new Error('No process'), { code: 'ESRCH' }); return originalKill(pid, signal); };
    store.recover();
  } finally { process.kill = originalKill; }
  const recovered = store.get('room-fixture');
  assert.equal(recovered.status, 'failed'); assert.equal(recovered.tasks[0].status, 'failed');
  assert.equal(recovered.tasks[0].runId, 'retained-workflow');
  assert.deepEqual(recovered.messages[0], message(1));
  assert.match(recovered.error, /not replayed/);
  assert.equal(setup.open().get('room-fixture').status, 'failed');
});

test('future room schemas are refused before persistent journal or permission changes', t => {
  const setup = fixture(t), filename = join(setup.directory, 'rooms.sqlite');
  const database = new DatabaseSync(filename);
  database.exec('PRAGMA user_version=999; CREATE TABLE protected(value TEXT); INSERT INTO protected VALUES(\'unchanged\');');
  database.close();
  const mode = statSync(filename).mode;
  assert.throws(() => setup.open(), /newer/);
  assert.equal(statSync(filename).mode, mode);
  const inspect = new DatabaseSync(filename);
  try {
    assert.equal(inspect.prepare('PRAGMA journal_mode').get().journal_mode, 'delete');
    assert.equal(inspect.prepare('SELECT value FROM protected').get().value, 'unchanged');
    assert.equal(inspect.prepare('PRAGMA user_version').get().user_version, 999);
  } finally { inspect.close(); }
});

test('atomic room cancellation handles the latest idle or ready state and preserves terminal outcomes', t => {
  const store = fixture(t).open();
  for (const status of ['idle', 'ready', 'needs_attention']) {
    const record = room(`cancel-${status}`); record.status = status;
    if (status === 'ready') record.tasks[0].status = 'assigned';
    store.insert(record);
    assert.equal(store.requestCancel(record.id, { activeOnly: true }).status, status);
    const cancelled = store.requestCancel(record.id);
    assert.equal(cancelled.status, 'cancelled'); assert.equal(cancelled.tasks[0].status, 'cancelled');
    assert.equal(store.get(record.id).status, 'cancelled');
  }
  for (const status of ['completed', 'failed', 'cancelled']) {
    const record = room(`terminal-${status}`); record.status = status; store.insert(record);
    assert.deepEqual(store.requestCancel(record.id), record);
    assert.equal(store.get(record.id).revision, record.revision);
  }
});
