import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { RoomStore } from '../dist/room-store.js';
import { RoomManager } from '../dist/rooms.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

const budget = 1024 * 1024;
const message = (id, body) => ({ id, author: 'project-manager', kind: 'human', body, createdAt: '2026-11-04T00:00:00.000Z' });
function room(id, messages = []) {
  const now = '2026-11-04T00:00:00.000Z';
  return { id, title: 'Complete evidence', brief: 'Retain complete room history.', status: 'idle', members: [{ agent: 'codex', role: 'worker', weight: 1 }],
    tasks: [], messages, rounds: 1, maxCalls: 16, autoExecute: false, contextLimit: null, callsUsed: 0, revision: 1, createdAt: now, updatedAt: now };
}
function fixture(t) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-history-bounds-')), stores = [];
  t.after(() => {
    for (const store of stores) store.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-history-bounds-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, open() { const store = new RoomStore(root); stores.push(store); return store; } };
}

test('room pages bound serialized UTF-8 bytes while retaining every complete message and advancing over an oversized row', t => {
  const store = fixture(t).open();
  const history = [0, 1, 2].map(index => message('medium-' + index, ('漢"\n\\' + index).repeat(45000)));
  history.push(message('oversized', '🧭'.repeat(budget / 2)), message('after-oversized', 'Complete final evidence.'));
  store.insert(room('main', history));
  store.insert(room('other', [message('other-only', 'Never included in the selected history.')]));
  const collected = [];
  let cursor, pages = 0;
  do {
    const page = store.messagePage('main', 100, cursor);
    pages++;
    assert.ok(page.messages.length > 0);
    if (Buffer.byteLength(JSON.stringify(page.messages)) > budget) {
      assert.equal(page.messages.length, 1); assert.equal(page.messages[0].id, 'oversized');
    }
    collected.push(...page.messages);
    if (page.nextCursor !== null) { assert.ok(Number.isSafeInteger(page.nextCursor)); assert.ok(page.nextCursor > (cursor ?? 0)); }
    cursor = page.nextCursor;
  } while (cursor !== null);
  assert.ok(pages >= 3);
  assert.deepEqual(collected, history);
  const end = store.messagePage('main', 100, Number.MAX_SAFE_INTEGER);
  assert.deepEqual(end, { messages: [], nextCursor: null, nextAfter: Number.MAX_SAFE_INTEGER });
});

test('latest room previews keep a byte-bounded complete suffix and mutations preserve omitted evidence', t => {
  const setup = fixture(t), store = setup.open();
  const history = Array.from({ length: 8 }, (_, index) => message('large-' + index, ('漢"\n\\' + index).repeat(45000)));
  store.insert(room('preview', history));
  const preview = store.get('preview');
  assert.equal(preview.messages[0].id, 'history-preview');
  const suffix = preview.messages.slice(1);
  assert.ok(suffix.length > 0 && suffix.length < history.length);
  assert.ok(Buffer.byteLength(JSON.stringify(suffix)) <= budget);
  assert.deepEqual(suffix, history.slice(-suffix.length));
  const extra = message('new-complete', 'A later human instruction.');
  store.mutate('preview', current => { current.messages.push(extra); });
  const reopened = setup.open(), retained = [];
  let cursor;
  do { const page = reopened.messagePage('preview', 100, cursor); retained.push(...page.messages); cursor = page.nextCursor; } while (cursor !== null);
  assert.deepEqual(retained, [...history, extra]);
  const oversized = message('new-oversized', '🚀'.repeat(budget / 2));
  reopened.mutate('preview', current => { current.messages.push(oversized); });
  assert.deepEqual(reopened.get('preview').messages.slice(1), [oversized]);
});

test('waiting for a foreign room recovers that dead owner without scanning or changing unrelated rooms', { timeout: 15000 }, async t => {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-wait-scope-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' })), manager = new RoomManager(service);
  t.after(async () => {
    await manager.close(); await service.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-wait-scope-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const owner = spawn(process.execPath, ['-e', ''], { windowsHide: true, stdio: 'ignore' });
  await once(owner, 'close');
  const create = title => manager.create({ title, brief: 'A lost owner must never replay its work.', members: [{ agent: 'codex', role: 'worker' }] });
  const awaited = create('Awaited room'), unrelated = create('Unrelated room');
  for (const record of [awaited, unrelated]) manager.store.mutate(record.id, value => { value.status = 'running'; }, { claim: { id: 'departed-owner', pid: owner.pid } });
  const recovered = await manager.wait(awaited.id, 200);
  assert.equal(recovered.status, 'failed'); assert.match(recovered.error, /owning process exited/);
  assert.equal(manager.metadata(unrelated.id).status, 'running');
  manager.recover(unrelated.id);
  assert.equal(manager.metadata(unrelated.id).status, 'failed');
});
