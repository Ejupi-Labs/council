import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { PrivateChatManager } from '../dist/private-chats.js';
import { DelegationManager } from '../dist/delegation.js';
import { BillingManager } from '../dist/billing.js';
import { writeStatement } from '../dist/sqlite-write.js';

const HIGH = '9007199254740994', LIMIT = '9007199254740991';
function poison(db) {
  db.exec('CREATE TABLE IF NOT EXISTS metadata_probe(id INTEGER PRIMARY KEY); INSERT OR REPLACE INTO metadata_probe(id) VALUES(' + HIGH + ');');
  assert.equal(last(db), HIGH);
}
function last(db) { return db.prepare('SELECT CAST(last_insert_rowid() AS TEXT) AS value').get().value; }
function fixture(t) {
  const parent = realpathSync.native(tmpdir()), root = mkdtempSync(join(parent, 'council-write-metadata-'));
  const config = loadConfig(root, { backend: 'mock', maxConcurrency: 2 }), managers = [];
  const service = new CouncilService(config, { doctor: async () => [], run: async () => { assert.fail('Write metadata tests must never dispatch a provider.'); } });
  t.after(async () => {
    for (const manager of managers.reverse()) manager.close();
    // These are metadata-only fixture rows; settle a queued fixture if an assertion failed before its normal finalization.
    service.store.db.exec('CREATE TABLE IF NOT EXISTS metadata_probe(id INTEGER PRIMARY KEY); INSERT OR REPLACE INTO metadata_probe(id) VALUES(1);');
    for (const summary of service.store.listRunSummaries(100)) if (summary.status === 'queued' && !service.jobs.has(summary.id)) {
      const run = service.store.getRun(summary.id); service.store.finalizeRun({ ...run, status: 'failed', error: 'Inspected metadata-only fixture; no agent dispatched.', updatedAt: new Date().toISOString() });
    }
    await service.close();
    const db = new DatabaseSync(join(config.stateDir, 'state.db'), { readOnly: true });
    try { for (const table of ['instances', 'slots', 'slot_waiters']) assert.equal(db.prepare('SELECT COUNT(*) AS count FROM ' + table).get().count, 0); } finally { db.close(); }
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-write-metadata-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, config, service, managers };
}
function owned(wire) {
  const now = new Date().toISOString(), run = { id: 'metadata-owned-run', status: 'queued', createdAt: now, updatedAt: now, steps: [],
    input: { mode: 'delegate', agents: ['codex'], task: 'Owned metadata-only fixture.', permission: 'read' } };
  wire.service.store.createRun(run, wire.service.instanceId); return run;
}

test('write metadata uses exact BigInts without changing default or explicitly configured SELECT readers', t => {
  const db = new DatabaseSync(':memory:'); t.after(() => db.close());
  db.exec('CREATE TABLE metadata_probe(id INTEGER PRIMARY KEY); CREATE TABLE target(id TEXT PRIMARY KEY); INSERT INTO target VALUES(\'owned\');');
  const ordinary = db.prepare('SELECT 1 AS value'), exact = db.prepare('SELECT 1 AS value'); exact.setReadBigInts(true);
  poison(db); const statement = db.prepare('DELETE FROM target WHERE id=?'); assert.equal(writeStatement(statement), statement);
  const result = statement.run('owned'); assert.equal(result.changes, 1n); assert.equal(result.lastInsertRowid, BigInt(HIGH));
  assert.equal(ordinary.get().value, 1); assert.equal(exact.get().value, 1n); assert.equal(db.isTransaction, false);
});

test('owned slot cleanup, heartbeat and lease counts survive an unrelated sticky high row ID', async t => {
  const wire = fixture(t), store = wire.service.store, run = owned(wire), token = store.acquireSlot(wire.service.instanceId, run.id, 2);
  poison(store.db); await store.releaseSlotWithRetry(token, wire.service.instanceId);
  assert.equal(store.health().counts.slots, 0); assert.equal(last(store.db), HIGH);
  const heartbeat = store.heartbeat(wire.service.instanceId), count = store.release('unused-fixture-lease-owner');
  assert.equal(heartbeat, true); assert.equal(count, 0); assert.equal(typeof count, 'number'); assert.equal(last(store.db), HIGH);
  assert.equal(typeof store.listRunSummaries(10)[0].revision, 'number');
  store.finalizeRun({ ...run, status: 'completed', updatedAt: new Date().toISOString() });
});

test('existing room, delegation and billing updates retain their numeric read contracts after a high row ID', t => {
  const wire = fixture(t), room = wire.service.rooms.create({ title: 'Before', brief: 'Metadata only.', members: [{ agent: 'codex', role: 'lead' }], tasks: [] });
  const rooms = wire.service.rooms.store; poison(rooms.db);
  const changed = rooms.mutate(room.id, value => { value.title = 'After'; }, { requireIdle: true });
  assert.equal(changed.title, 'After'); assert.equal(typeof changed.revision, 'number'); assert.equal(last(rooms.db), HIGH);
  const delegation = new DelegationManager(wire.service); wire.managers.push(delegation); poison(delegation.db);
  const configured = delegation.configure({ mode: 'agent', delegate: 'Offline actor', mandate: 'Preserve evidence without dispatch.', roomIds: [], expectedRevision: 0 });
  assert.equal(configured.revision, 1); assert.equal(typeof delegation.overview().reportRevision, 'number'); assert.equal(last(delegation.db), HIGH);
  const billing = new BillingManager(wire.config); wire.managers.push(billing);
  const original = billing.update('codex', { mode: 'api', plan: 'Before' }); poison(billing.db);
  const updated = billing.update('codex', { plan: 'After' }, original.revision);
  assert.equal(updated.plan, 'After'); assert.equal(typeof updated.revision, 'number'); assert.equal(last(billing.db), HIGH);
});

test('private numeric sequence exhaustion reaches its explicit guard and rolls back rather than failing write metadata', t => {
  const wire = fixture(t), chats = new PrivateChatManager(wire.service); wire.managers.push(chats);
  const chat = chats.create({ participants: ['actor-A', 'actor-B'] }); chats.send({ chatId: chat.id, from: 'actor-A', body: 'Existing evidence.' });
  chats.db.exec("UPDATE sqlite_sequence SET seq=" + LIMIT + " WHERE name='private_messages'"); poison(chats.db);
  assert.throws(() => chats.send({ chatId: chat.id, from: 'actor-A', body: 'Cannot create an unsafe numeric cursor.' }), /Private message sequence space is exhausted/);
  assert.equal(chats.get(chat.id, 'actor-A').messageCount, 1); assert.equal(chats.db.prepare('SELECT COUNT(*) AS count FROM private_messages').get().count, 1);
  assert.equal(chats.db.prepare("SELECT CAST(seq AS TEXT) AS value FROM sqlite_sequence WHERE name='private_messages'").get().value, LIMIT);
  assert.equal(chats.db.isTransaction, false);
});

test('standalone event numeric cursor exhaustion rolls back the insertion and keeps earlier history', t => {
  const wire = fixture(t), store = wire.service.store, run = owned(wire), before = store.events(run.id);
  store.db.exec("UPDATE sqlite_sequence SET seq=" + LIMIT + " WHERE name='events'"); poison(store.db);
  assert.throws(() => store.appendEvent(run.id, 'metadata.boundary', {}), /Event sequence space is exhausted/);
  assert.deepEqual(store.events(run.id), before); assert.equal(store.db.isTransaction, false);
  assert.equal(store.db.prepare("SELECT CAST(seq AS TEXT) AS value FROM sqlite_sequence WHERE name='events'").get().value, LIMIT);
  store.db.exec("UPDATE sqlite_sequence SET seq=1 WHERE name='events'; INSERT OR REPLACE INTO metadata_probe(id) VALUES(1)");
  store.finalizeRun({ ...run, status: 'completed', updatedAt: new Date().toISOString() });
});
