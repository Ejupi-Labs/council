import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { DatabaseSync } from 'node:sqlite';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { PrivateChatManager, PrivateChatConflict } from '../dist/private-chats.js';

function fixture(t) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-private-chats-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), {
    doctor: async () => [], run: async () => { assert.fail('Private chat transport must never invoke a provider.'); },
  });
  const manager = new PrivateChatManager(service), managers = [manager];
  const room = service.rooms.create({ title: 'Public room', brief: 'Keep private coordination out of this discussion.', members: [{ agent: 'codex', role: 'lead' }], tasks: [] });
  t.after(async () => {
    for (const value of managers) value.close(); await service.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-private-chats-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, service, manager, room,
    peer() { const peer = new PrivateChatManager(service); managers.push(peer); return peer; } };
}
const first = 'codex/task-A', second = 'codex/task-B', third = 'claude/task-C';
const create = (manager, roomId) => manager.create({ participants: [first, second], ...(roomId ? { roomId } : {}) });
const send = (manager, chatId, body, extra = {}) => manager.send({ chatId, from: first, body, ...extra });

test('private chats use canonical distinct task pairs and optional room scopes idempotently across hosts', t => {
  const { manager, peer, room } = fixture(t), other = peer();
  const chat = create(manager, room.id);
  assert.deepEqual(chat.participants, [first, second]); assert.equal(chat.messageCount, 0); assert.equal(chat.lastSequence, 0);
  assert.deepEqual(other.create({ participants: [second, first], roomId: room.id }), chat);
  const unscoped = create(other); assert.notEqual(unscoped.id, chat.id); assert.equal(unscoped.roomId, undefined);
  const thirdChat = manager.create({ participants: [first, third], roomId: room.id });
  assert.notEqual(thirdChat.id, chat.id);
  assert.deepEqual(manager.list(second, room.id).map(item => item.id), [chat.id]);
  assert.equal(manager.list(first).length, 3); assert.deepEqual(manager.list('Unrelated task'), []);
  chat.participants[0] = 'Caller mutation';
  assert.equal(other.list(second, room.id)[0].participants[0], first);
});

test('pair-scoped operations reject unrelated actors and malformed participants without partial changes', async t => {
  const { manager, room } = fixture(t), chat = create(manager, room.id);
  for (const participants of [[first, first], [first], [first, second, third], [' ', second], ['bad\0actor', second], ['\ud800', second]]) {
    assert.throws(() => manager.create({ participants }), /distinct|entries|non-whitespace|format|Unicode/);
  }
  assert.throws(() => manager.create({ participants: [first, second], roomId: 'unknown' }), /404/);
  assert.throws(() => send(manager, chat.id, 'Foreign send', { from: third }), /not a participant/);
  assert.throws(() => manager.page({ chatId: chat.id, actor: third }), /not a participant/);
  await assert.rejects(manager.watch({ chatId: chat.id, actor: third, timeoutMs: 0 }), /not a participant/);
  assert.throws(() => send(manager, chat.id, '  '), /non-whitespace/);
  assert.throws(() => manager.list(' '), /non-whitespace/);
  assert.equal(manager.list(first).length, 1); assert.equal(manager.list(first)[0].messageCount, 0); assert.equal(manager.listeners.size, 0);
});

test('actor discovery and selected-chat metadata expose task identities completely without transcript bodies', t => {
  const { manager, room } = fixture(t), chat = create(manager, room.id);
  manager.create({ participants: [first, third] }); send(manager, chat.id, 'Keep this body out of actor discovery.');
  assert.deepEqual(manager.actors(), [third, first, second].sort()); assert.deepEqual(manager.actors(room.id), [first, second]);
  assert.throws(() => manager.actors('unknown-room'), /404/);
  assert.throws(() => manager.get(chat.id, third), /not a participant/);
  assert.throws(() => manager.get('unknown-chat', first), /404/);
  const metadata = manager.get(chat.id, second); assert.equal(metadata.messageCount, 1); assert.equal(metadata.lastSequence, 1);
  assert.equal(JSON.stringify(metadata).includes('Keep this body'), false);
  metadata.participants[0] = 'Caller mutation'; assert.equal(manager.get(chat.id, first).participants[0], first);
});

test('complete private messages persist in ascending order without entering room guidance or global context', t => {
  const { manager, peer, service, room } = fixture(t), chat = create(manager, room.id), before = service.rooms.get(room.id);
  let publications = 0; const unsubscribe = service.rooms.subscribe(() => { publications++; }); t.after(unsubscribe);
  const body = 'Private evidence is retained completely.\n' + 'Retained message detail. '.repeat(3000);
  const saved = send(manager, chat.id, body), reply = send(manager, chat.id, 'Private reply.', { from: second });
  assert.ok(saved.sequence > 0); assert.ok(reply.sequence > saved.sequence);
  saved.body = 'Caller overwrite';
  const page = peer().page({ chatId: chat.id, actor: second });
  assert.equal(page.messages[0].body, body); assert.equal(page.messages[1].body, 'Private reply.'); assert.equal(page.nextCursor, null);
  assert.deepEqual(service.rooms.get(room.id), before); assert.equal(publications, 0);
  assert.equal(manager.list(first, room.id)[0].messageCount, 2);
  assert.equal(JSON.stringify(manager.list(first)).includes('Private reply'), false, 'List metadata has no transcript bodies.');
  page.messages[0].body = 'Another caller overwrite';
  assert.equal(manager.page({ chatId: chat.id, actor: first }).messages[0].body, body);
});

test('private message pagination retains every interleaved message and rejects foreign or future cursors', t => {
  const { manager, room } = fixture(t), chat = create(manager, room.id), unrelated = manager.create({ participants: [first, third], roomId: room.id });
  const retained = []; let foreign;
  for (let index = 0; index < 107; index++) {
    retained.push(send(manager, chat.id, `Full private message ${index}`, { from: index % 2 === 0 ? first : second }));
    foreign = send(manager, unrelated.id, `Unrelated message ${index}`);
  }
  let after = 0; const all = [];
  do { const page = manager.page({ chatId: chat.id, actor: first, after, limit: 13 }); all.push(...page.messages); after = page.nextCursor; } while (after !== null);
  assert.deepEqual(all.map(message => message.id), retained.map(message => message.id));
  assert.equal(all.at(-1).body, 'Full private message 106');
  assert.throws(() => manager.page({ chatId: chat.id, actor: first, after: foreign.sequence }), /does not belong/);
  assert.throws(() => manager.page({ chatId: chat.id, actor: first, after: 9999999 }), /does not belong/);
  assert.deepEqual(manager.page({ chatId: chat.id, actor: first, after: retained.at(-1).sequence }), { messages: [], nextCursor: null });
  for (const extra of [{ limit: 0 }, { limit: 101 }, { after: -1 }, { after: 1.5 }]) assert.throws(() => manager.page({ chatId: chat.id, actor: first, ...extra }), /private message page/);
});

test('private send retries are idempotent per sender and pair; conflicting reuse never changes stored content', t => {
  const { manager, peer, room } = fixture(t), other = peer(), chat = create(manager, room.id);
  const saved = send(manager, chat.id, 'Exactly once.', { idempotencyKey: 'worker-turn-7' });
  assert.deepEqual(send(other, chat.id, 'Exactly once.', { idempotencyKey: 'worker-turn-7' }), saved);
  assert.throws(() => send(other, chat.id, 'Conflicting retry.', { idempotencyKey: 'worker-turn-7' }), PrivateChatConflict);
  send(other, chat.id, 'Other sender uses its own key.', { from: second, idempotencyKey: 'worker-turn-7' });
  const unrelated = manager.create({ participants: [first, third] });
  send(manager, unrelated.id, 'Same sender and key, separate conversation.', { idempotencyKey: 'worker-turn-7' });
  assert.equal(manager.list(second)[0].messageCount, 2); assert.equal(manager.page({ chatId: chat.id, actor: first }).messages[0].body, 'Exactly once.');
});

test('optional workflow references are validated while successful keyed retries survive later workflow pruning', t => {
  const { manager, peer, service, room } = fixture(t), chat = create(manager, room.id), other = peer();
  assert.throws(() => send(manager, chat.id, 'Unknown workflow.', { runId: 'missing-run' }), /workflow not found/);
  const timestamp = '2000-01-01T00:00:00.000Z';
  service.store.saveRun({ id: 'retained-run', input: { mode: 'delegate', task: 'Retain a full evidence record.', agents: ['codex'], permission: 'read' },
    status: 'completed', createdAt: timestamp, updatedAt: timestamp, steps: [], output: 'Large workflow evidence. '.repeat(50_000) });
  service.getRun = () => { assert.fail('Private-message reference validation must not decode a complete workflow.'); };
  service.store.getRun = service.getRun;
  const saved = send(manager, chat.id, 'Known workflow.', { runId: 'retained-run', idempotencyKey: 'retained-turn' });
  assert.equal(saved.runId, 'retained-run');
  assert.equal(service.store.prune({ before: '2001-01-01T00:00:00.000Z', keepLatest: 0, dryRun: false }).deletedRuns, 1);
  assert.equal(service.hasRun('retained-run'), false);
  assert.deepEqual(send(other, chat.id, 'Known workflow.', { runId: 'retained-run', idempotencyKey: 'retained-turn' }), saved);
  assert.throws(() => send(other, chat.id, 'A new message for a removed workflow.', { runId: 'retained-run' }), /workflow not found/);
  assert.throws(() => send(other, chat.id, 'Known workflow.', { idempotencyKey: 'retained-turn' }), PrivateChatConflict);
  assert.equal(manager.list(first)[0].messageCount, 1);
});

test('private watches wake only for their own chat and return metadata without loading message bodies', async t => {
  const { manager, room } = fixture(t), chat = create(manager, room.id), unrelated = manager.create({ participants: [first, third] });
  let settled = false;
  const wait = manager.watch({ chatId: chat.id, actor: second, after: 0, timeoutMs: 2000 }).then(result => { settled = true; return result; });
  send(manager, unrelated.id, 'Another task pair is talking.');
  await delay(20); assert.equal(settled, false);
  const saved = send(manager, chat.id, 'Only this pair receives this message.');
  const changed = await wait;
  assert.equal(changed.reason, 'changed'); assert.equal(changed.lastSequence, saved.sequence); assert.equal(changed.chat.messageCount, 1);
  assert.equal(JSON.stringify(changed).includes(saved.body), false);
  assert.equal(manager.listeners.size, 0);
  const timed = await manager.watch({ chatId: chat.id, actor: first, after: saved.sequence, timeoutMs: 5 });
  assert.equal(timed.reason, 'timeout'); assert.equal(timed.lastSequence, saved.sequence);
});

test('private watches notice messages sent by another host using SQLite, without model polling', async t => {
  const { manager, peer } = fixture(t), chat = create(manager), other = peer();
  const wait = manager.watch({ chatId: chat.id, actor: second, timeoutMs: 2000 });
  const saved = send(other, chat.id, 'Cross-host private message.');
  const changed = await wait;
  assert.equal(changed.reason, 'changed'); assert.equal(changed.lastSequence, saved.sequence);
  assert.equal(manager.page({ chatId: chat.id, actor: second }).messages[0].body, saved.body);
});

test('private watch cancellation, shutdown and invalid cursors release every waiting listener', async t => {
  const { manager } = fixture(t), chat = create(manager), unrelated = manager.create({ participants: [first, third] });
  const foreign = send(manager, unrelated.id, 'Cursor belongs elsewhere.');
  await assert.rejects(manager.watch({ chatId: chat.id, actor: first, after: foreign.sequence }), /does not belong/);
  const controller = new AbortController(), wait = manager.watch({ chatId: chat.id, actor: second, timeoutMs: Number.MAX_SAFE_INTEGER }, controller.signal);
  controller.abort(new DOMException('Human stopped private monitoring.', 'AbortError'));
  await assert.rejects(wait, /Human stopped/); assert.equal(manager.listeners.size, 0);
  const closing = manager.watch({ chatId: chat.id, actor: second, timeoutMs: 100000000000 }); manager.close();
  await assert.rejects(closing, /shutting down/); assert.equal(manager.listeners.size, 0);
  assert.throws(() => manager.list(first), /closed/); manager.close();
});

test('private storage failures leave no partial message or incorrect metadata and can be retried explicitly', t => {
  const { manager, service } = fixture(t), chat = create(manager);
  const blocker = new DatabaseSync(join(service.config.stateDir, 'private-chats.sqlite'));
  try {
    blocker.exec('BEGIN IMMEDIATE');
    assert.throws(() => send(manager, chat.id, 'No partial insert.', { idempotencyKey: 'blocked-turn' }), error => (error.errcode & 0xff) === 5);
  } finally { if (blocker.isTransaction) blocker.exec('ROLLBACK'); blocker.close(); }
  assert.equal(manager.list(first)[0].messageCount, 0); assert.equal(manager.page({ chatId: chat.id, actor: first }).messages.length, 0);
  send(manager, chat.id, 'No partial insert.', { idempotencyKey: 'blocked-turn' });
  assert.equal(manager.list(first)[0].messageCount, 1);
});

test('future private-chat database versions are refused before changing their journal mode', t => {
  const { manager, service } = fixture(t); manager.close();
  const path = join(service.config.stateDir, 'private-chats.sqlite'), future = new DatabaseSync(path);
  try { future.exec('PRAGMA journal_mode=DELETE; PRAGMA user_version=2;'); } finally { future.close(); }
  assert.throws(() => new PrivateChatManager(service), /newer Council version/);
  const inspect = new DatabaseSync(path);
  try {
    assert.equal(inspect.prepare('PRAGMA journal_mode').get().journal_mode, 'delete');
    assert.equal(inspect.prepare('PRAGMA user_version').get().user_version, 2);
  } finally { inspect.close(); }
});

test('private history pages complete UTF-8 messages by bytes and retain one oversized message intact', t => {
  const { manager } = fixture(t), chat = create(manager);
  const body = 'λ🛠️'.repeat(60_000);
  const inserted = [
    send(manager, chat.id, body),
    send(manager, chat.id, body + 'second'),
    send(manager, chat.id, 'é'.repeat(600_000)),
    send(manager, chat.id, 'A final short message.'),
  ];
  const pages = [], all = [];
  let after = 0;
  do {
    const page = manager.page({ chatId: chat.id, actor: second, after, limit: 100 });
    pages.push(page); all.push(...page.messages);
    assert.ok(page.messages.length);
    assert.ok(Buffer.byteLength(JSON.stringify(page.messages)) <= 1024 * 1024 || page.messages.length === 1);
    assert.equal(page.nextCursor, all.length < inserted.length ? page.messages.at(-1).sequence : null);
    if (page.nextCursor === null) break;
    assert.ok(page.nextCursor > after); after = page.nextCursor;
  } while (pages.length <= inserted.length);
  assert.deepEqual(all, inserted);
  assert.equal(pages.length, 4);
  assert.equal(pages[2].messages[0].body, inserted[2].body);
  assert.ok(Buffer.byteLength(JSON.stringify(pages[2].messages)) > 1024 * 1024);
  assert.deepEqual(manager.page({ chatId: chat.id, actor: first, after: inserted.at(-1).sequence }), { messages: [], nextCursor: null });
});

test('thread discovery pages preserve exact actors, scope and stable creation order across updates and hosts', t => {
  const { manager, peer, room } = fixture(t), actor = ' actor " literal λ ';
  const chats = Array.from({ length: 7 }, (_, index) => manager.create({ participants: [actor, 'peer-' + index], roomId: room.id }));
  manager.create({ participants: [actor, 'workspace-peer'] });
  const unrelated = manager.create({ participants: ['unrelated-a', 'unrelated-b'], roomId: room.id });
  const expected = manager.list(actor, room.id).map(value => value.id), firstPage = manager.listPage({ actor, roomId: room.id, limit: 2 });
  assert.equal(firstPage.chats.length, 2); assert.ok(firstPage.nextCursor.length < 512);
  manager.send({ chatId: chats.at(-1).id, from: actor, body: 'Metadata updates must not reorder a thread.' });
  const all = [...firstPage.chats]; let cursor = firstPage.nextCursor;
  const other = peer();
  while (cursor !== null) { const page = other.listPage({ actor, roomId: room.id, limit: 2, cursor }); all.push(...page.chats); cursor = page.nextCursor; }
  assert.deepEqual(all.map(value => value.id), expected);
  assert.equal(new Set(all.map(value => value.id)).size, chats.length);
  assert.ok(all.every(value => value.participants.includes(actor) && value.roomId === room.id));
  assert.throws(() => manager.listPage({ actor: actor.trim(), roomId: room.id, cursor: firstPage.nextCursor }), /cursor.*scope/);
  assert.throws(() => manager.listPage({ actor, cursor: firstPage.nextCursor }), /cursor.*scope/);
  const decoded = JSON.parse(Buffer.from(firstPage.nextCursor, 'base64url').toString('utf8'));
  const foreign = Buffer.from(JSON.stringify({ ...decoded, chatId: unrelated.id })).toString('base64url');
  assert.throws(() => manager.listPage({ actor, roomId: room.id, cursor: foreign }), /unavailable or outside/);
  const missing = Buffer.from(JSON.stringify({ ...decoded, chatId: 'missing-anchor' })).toString('base64url');
  assert.throws(() => manager.listPage({ actor, roomId: room.id, cursor: missing }), /unavailable or outside/);
  const elsewhere = fixture(t);
  assert.throws(() => elsewhere.manager.listPage({ actor, cursor: firstPage.nextCursor }), /cursor.*scope/);
});

test('participant discovery deduplicates exact labels in binary order with restartable room-bound cursors', t => {
  const { manager, peer, room } = fixture(t), owner = 'owner', labels = ['Human', ' Human ', 'a%_', '🦊', '\uE000', 'a"b', 'same actor'];
  for (const label of labels) manager.create({ participants: [owner, label], roomId: room.id });
  manager.create({ participants: ['same actor', 'Human'], roomId: room.id });
  manager.create({ participants: [owner, 'workspace-only'] });
  const expected = [...new Set([owner, ...labels])].sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)));
  const firstPage = manager.actorsPage({ roomId: room.id, limit: 2 });
  manager.close(); const reopened = peer();
  const all = [...firstPage.actors]; let cursor = firstPage.nextCursor;
  while (cursor !== null) { const page = reopened.actorsPage({ roomId: room.id, limit: 2, cursor }); all.push(...page.actors); cursor = page.nextCursor; }
  assert.deepEqual(all, expected);
  assert.throws(() => reopened.actorsPage({ cursor: firstPage.nextCursor }), /cursor.*scope/);
  assert.throws(() => reopened.listPage({ actor: owner, roomId: room.id, cursor: firstPage.nextCursor }), /cursor.*scope/);
  assert.deepEqual(reopened.actors(), [...new Set([owner, ...labels, 'workspace-only'])].sort(), 'The existing array API remains complete.');
});

test('discovery pages bound complete UTF-8 and escaped metadata while long actor anchors keep cursors small', t => {
  const { manager } = fixture(t), actor = 'owner';
  const peers = ['λ🛠️'.repeat(60_000), 'é'.repeat(600_000), '\n"\\'.repeat(180_000), 'A short peer.'];
  const inserted = peers.map(peer => manager.create({ participants: [actor, peer] }));
  const chats = []; let cursor;
  do {
    const page = manager.listPage({ actor, limit: 100, ...(cursor ? { cursor } : {}) });
    assert.ok(page.chats.length); chats.push(...page.chats);
    assert.ok(Buffer.byteLength(JSON.stringify(page.chats)) <= 1024 * 1024 || page.chats.length === 1);
    cursor = page.nextCursor; if (cursor) assert.ok(cursor.length < 512);
  } while (cursor !== null);
  assert.deepEqual(new Set(chats.map(value => value.id)), new Set(inserted.map(value => value.id)));
  const actors = []; cursor = undefined;
  do {
    const page = manager.actorsPage({ limit: 100, ...(cursor ? { cursor } : {}) });
    assert.ok(page.actors.length); actors.push(...page.actors);
    assert.ok(Buffer.byteLength(JSON.stringify(page.actors)) <= 1024 * 1024 || page.actors.length === 1);
    cursor = page.nextCursor; if (cursor) assert.ok(cursor.length < 512, 'A large label must not be copied into its cursor.');
  } while (cursor !== null);
  assert.deepEqual(new Set(actors), new Set([actor, ...peers]));
});

test('invalid and deeply nested discovery cursors fail before JSON graph parsing or any storage change', t => {
  const { manager } = fixture(t);
  manager.create({ participants: ['owner', 'first'] }); manager.create({ participants: ['owner', 'second'] });
  const firstPage = manager.listPage({ actor: 'owner', limit: 1 }), decoded = JSON.parse(Buffer.from(firstPage.nextCursor, 'base64url').toString('utf8'));
  for (const cursor of ['', '=', 'x'.repeat(1025), Buffer.from([0xff]).toString('base64url'), Buffer.from('[]').toString('base64url'),
    Buffer.from(JSON.stringify({ ...decoded, unknown: true })).toString('base64url'),
    Buffer.from('{"version":1,"version":1,' + JSON.stringify(decoded).slice(1)).toString('base64url')]) {
    assert.throws(() => manager.listPage({ actor: 'owner', cursor }));
  }
  const raw = JSON.stringify(decoded).slice(0, -1) + ',"extra":' + '['.repeat(70) + '0' + ']'.repeat(70) + '}';
  const cursor = Buffer.from(raw).toString('base64url'), original = JSON.parse; let fullParses = 0;
  JSON.parse = function (value, ...args) { if (value === raw) fullParses++; return original.call(this, value, ...args); };
  try { assert.throws(() => manager.listPage({ actor: 'owner', cursor }), /Invalid private discovery cursor/); }
  finally { JSON.parse = original; }
  assert.equal(fullParses, 0);
  assert.equal(manager.list('owner').length, 2);
});

test('large private discovery reads only requested metadata and never reads transcript bodies', t => {
  const { manager } = fixture(t), actor = 'long-history-owner', count = 5000;
  manager.db.exec('BEGIN IMMEDIATE');
  try {
    const insert = manager.db.prepare('INSERT INTO private_chats(id,participant_a,participant_b,room_id,created_at) VALUES(?,?,?,?,?)');
    for (let index = 0; index < count; index++) insert.run('history-' + String(index).padStart(6, '0'), actor, 'worker-' + String(index).padStart(6, '0'), '', '2026-11-04T12:00:00.000Z');
    manager.db.exec('COMMIT');
  } catch (error) { manager.db.exec('ROLLBACK'); throw error; }
  const prepare = manager.db.prepare;
  manager.db.prepare = function (sql) { assert.doesNotMatch(sql, /private_messages/i, 'Discovery must never query private transcript storage.'); return prepare.call(this, sql); };
  try {
    const threads = manager.listPage({ actor, limit: 7 }), actors = manager.actorsPage({ limit: 7 });
    assert.equal(threads.chats.length, 7); assert.ok(threads.nextCursor);
    assert.equal(actors.actors.length, 7); assert.ok(actors.nextCursor);
    assert.equal(manager.list(actor).length, count);
  } finally { manager.db.prepare = prepare; }
});
