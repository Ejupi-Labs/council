import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { RoomStore } from '../dist/room-store.js';

const now = '2026-11-04T00:00:00.000Z';
const message = id => ({ id, author: 'Human', kind: 'human', body: 'Complete evidence ' + id, createdAt: now });
const room = (id, messages) => ({ id, title: 'Incremental history', brief: 'Keep complete history on request.', members: [{ agent: 'codex', role: 'worker', weight: 1 }],
  tasks: [], messages, status: 'idle', rounds: 1, maxCalls: 8, autoExecute: false, contextLimit: null, callsUsed: 0, revision: 1, createdAt: now, updatedAt: now });

test('completed room history pages retain a high-water cursor for later checks, including empty checks and foreign appends', t => {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-room-history-after-')));
  const store = new RoomStore(root), foreign = new RoomStore(root);
  t.after(() => {
    foreign.close(); store.close();
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-room-history-after-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  store.insert(room('selected', [message('first'), message('second')]));
  store.insert(room('unrelated', [message('unrelated')]));
  const first = store.messagePage('selected', 1);
  assert.equal(first.messages[0].id, 'first'); assert.equal(first.nextAfter, first.nextCursor);
  const end = store.messagePage('selected', 100, first.nextCursor);
  assert.deepEqual(end.messages, [message('second')]); assert.equal(end.nextCursor, null); assert.ok(end.nextAfter > first.nextAfter);
  assert.deepEqual(store.messagePage('selected', 100, end.nextAfter), { messages: [], nextCursor: null, nextAfter: end.nextAfter });
  foreign.mutate('selected', current => { current.messages.push(message('later')); });
  const later = store.messagePage('selected', 100, end.nextAfter);
  assert.deepEqual(later.messages, [message('later')]); assert.equal(later.nextCursor, null); assert.ok(later.nextAfter > end.nextAfter);
  store.db.exec('PRAGMA query_only=ON');
  assert.deepEqual(store.messagePage('selected', 100, later.nextAfter), { messages: [], nextCursor: null, nextAfter: later.nextAfter });
});
