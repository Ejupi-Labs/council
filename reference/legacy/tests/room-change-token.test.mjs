import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { RoomStore } from '../dist/room-store.js';
import { RoomManager } from '../dist/rooms.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

const record = id => ({ id, title: 'Cache invalidation fixture', brief: 'Complete retained brief.', status: 'idle', members: [{ agent: 'codex', role: 'worker', weight: 1 }],
  tasks: [], messages: [], rounds: 1, maxCalls: 8, autoExecute: false, contextLimit: null, callsUsed: 0, revision: 1,
  createdAt: '2026-11-04T00:00:00.000Z', updatedAt: '2026-11-04T00:00:00.000Z' });
function fixture(t) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-change-token-')), stores = [];
  t.after(() => {
    for (const store of stores) store.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-change-token-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { open() { const store = new RoomStore(root); stores.push(store); return store; } };
}

test('room change tokens detect own and foreign commits while reads and foreign rollbacks leave cached data valid', t => {
  const setup = fixture(t), own = setup.open(), foreign = setup.open();
  const empty = own.changeToken(); own.insert(record('room-token'));
  const inserted = own.changeToken(); assert.notEqual(inserted, empty);
  own.get('room-token'); own.metadata('room-token'); own.observation('room-token'); own.list(1); own.messagePage('room-token'); own.has('room-token');
  assert.equal(own.changeToken(), inserted);
  foreign.mutate('room-token', room => { room.title = 'Foreign committed title'; });
  const changed = own.changeToken(), committed = own.get('room-token'); assert.notEqual(changed, inserted); assert.equal(committed.title, 'Foreign committed title');
  foreign.db.exec('BEGIN IMMEDIATE');
  foreign.db.prepare('UPDATE rooms SET updated_at=? WHERE id=?').run('2099-01-01T00:00:00Z', 'room-token');
  foreign.db.exec('ROLLBACK');
  assert.equal(own.changeToken(), changed, 'An uncommitted foreign edit must not make the old data appear obsolete.');
  own.db.exec('BEGIN IMMEDIATE'); own.db.prepare('UPDATE rooms SET updated_at=? WHERE id=?').run('2099-01-01T00:00:00Z', 'room-token'); own.db.exec('ROLLBACK');
  const afterOwnRollback = own.changeToken(); assert.equal(own.changeToken(), afterOwnRollback);
  assert.equal(own.get('room-token').updatedAt, committed.updatedAt); assert.equal(own.list(1)[0].updatedAt, committed.updatedAt);
});

test('room cache tokens stay scalar and read-only even when room and history JSON cannot be decoded', t => {
  const store = fixture(t).open(); store.insert(record('unreadable'));
  store.db.prepare('UPDATE rooms SET document=? WHERE id=?').run('{ deliberately invalid room JSON', 'unreadable');
  store.db.prepare('INSERT INTO room_messages(room_id,id,document) VALUES(?,?,?)').run('unreadable', 'invalid-history', '{ invalid history JSON');
  store.db.exec('PRAGMA query_only=ON');
  const parse = JSON.parse; JSON.parse = () => { assert.fail('A cache invalidator must not inspect room or message JSON.'); };
  let token;
  try { token = store.changeToken(); for (let index = 0; index < 100; index++) assert.equal(store.changeToken(), token); }
  finally { JSON.parse = parse; }
  assert.match(token, /^\d+:\d+$/); assert.equal(store.db.isTransaction, false);
  assert.throws(() => store.get('unreadable'), SyntaxError);
});

test('manager read APIs do not publish cache changes and a foreign manager invalidates its observer', async t => {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-manager-token-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' })), manager = service.rooms, peer = new RoomManager(service);
  t.after(async () => {
    await peer.close(); await service.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-manager-token-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const room = manager.create({ title: 'Observed room', brief: 'Keep unrelated reads cheap.', members: [{ agent: 'codex', role: 'worker' }] });
  const before = manager.changeToken();
  manager.get(room.id); manager.metadata(room.id); manager.observation(room.id); manager.list(1); manager.messagePage(room.id); manager.recover(room.id);
  assert.equal(manager.changeToken(), before);
  peer.postMessage(room.id, { author: 'Foreign human', body: 'A new committed instruction.' });
  assert.notEqual(manager.changeToken(), before);
  manager.publicationVersion = BigInt(Number.MAX_SAFE_INTEGER) + 1n;
  const large = manager.changeToken(); manager.publish(manager.get(room.id)); const later = manager.changeToken();
  assert.notEqual(later, large); assert.equal(later.split(':').at(-1), '9007199254740993');
});
