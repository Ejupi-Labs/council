import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { PassThrough } from 'node:stream';
import { setImmediate as nextTurn } from 'node:timers/promises';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { createMcpServer } from '../dist/mcp.js';
import { serveDashboard } from '../dist/dashboard.js';

async function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-private-integration-'));
  let providerCalls = 0;
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), {
    doctor: async () => [], run: async () => { providerCalls++; assert.fail('Private transport must not invoke a provider.'); },
  });
  const dashboard = await serveDashboard(service, service.rooms), address = new URL(dashboard.url);
  const token = new URLSearchParams(address.hash.slice(1)).get('token'), input = new PassThrough(), output = new PassThrough();
  const server = createMcpServer(service); await server.connect(input, output);
  let sequence = 0, buffer = '';
  const pending = new Map(), frames = [];
  output.on('data', chunk => {
    buffer += chunk.toString('utf8');
    for (;;) {
      const end = buffer.indexOf('\n'); if (end < 0) break;
      const frame = JSON.parse(buffer.slice(0, end)); buffer = buffer.slice(end + 1); frames.push(frame);
      if (pending.has(frame.id)) { const done = pending.get(frame.id); pending.delete(frame.id); done(frame); }
    }
  });
  t.after(async () => {
    await dashboard.close(); await server.close(); input.destroy(); output.destroy(); await service.close();
    assert.equal(dirname(resolve(root)), resolve(tmpdir())); assert.ok(basename(root).startsWith('council-private-integration-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const begin = (method, params) => {
    const id = ++sequence, promise = new Promise(done => pending.set(id, done));
    input.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n'); return { id, promise };
  };
  const request = async (method, params) => {
    const frame = await begin(method, params).promise; assert.equal(frame.error, undefined, JSON.stringify(frame.error)); return frame.result;
  };
  const call = async (name, args = {}) => {
    const result = await request('tools/call', { name, arguments: args });
    assert.notEqual(result.isError, true, result.content?.[0]?.text); return JSON.parse(result.content[0].text);
  };
  await request('initialize', { protocolVersion: '2025-11-25', capabilities: {}, clientInfo: { name: 'private-chat-integration', version: '1' } });
  const http = (path, { method = 'GET', body, authorization = token, origin, signal } = {}) => fetch(address.origin + path, { method, signal,
    headers: { ...(authorization ? { Authorization: 'Bearer ' + authorization } : {}), ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}), ...(origin ? { Origin: origin } : {}) },
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}) });
  const room = service.rooms.create({ title: 'Private coordination integration', brief: 'Keep peer overlap discussion outside this room.', members: [{ agent: 'codex', role: 'lead' }], tasks: [] });
  return { service, room, call, request, begin, input, frames, http, server, calls: () => providerCalls };
}
const first = 'run:fixture:codex', second = 'run:fixture:claude', third = 'run:fixture:junie';
const history = (id, actor = first, query = '') => '/api/private-chats/' + id + '/messages?actor=' + encodeURIComponent(actor) + query;

test('MCP and HTTP exchange complete private pair messages without changing room approval or shared history', { timeout: 15000 }, async t => {
  const wire = await fixture(t), tools = (await wire.request('tools/list', {})).tools.map(tool => tool.name);
  for (const name of ['create', 'send', 'list', 'actors', 'history', 'watch']) assert.ok(tools.includes('council_chat_' + name));
  wire.service.rooms.addTask(wire.room.id, { title: 'Approved read-only task', brief: 'Preserve this exact approval during private coordination.', permission: 'read' });
  wire.service.rooms.store.mutate(wire.room.id, room => {
    room.status = 'ready'; room.tasks[0].status = 'assigned'; room.tasks[0].assignee = 'codex';
    room.decision = { approved: true, instructionRevision: room.instructionRevision, approveWeight: 1, totalWeight: 1,
      assignments: { 'task-1': 'codex' }, rationale: 'Explicit fixture approval retained.', dissent: [] };
  });
  const chat = await wire.call('council_chat_create', { participants: [first, second], roomId: wire.room.id });
  const before = wire.service.rooms.get(wire.room.id);
  const body = '<script>literal private evidence</script>\n' + 'Full overlap coordination evidence. '.repeat(2000);
  const sent = await wire.call('council_chat_send', { chatId: chat.id, from: first, body, idempotencyKey: 'fixture-turn-1' });
  const httpReply = await wire.http('/api/private-chats/' + chat.id + '/messages', { method: 'POST', body: { from: second, body: 'I will inspect the shared prerequisite.', idempotencyKey: 'peer-reply-1' } });
  assert.equal(httpReply.status, 201); const reply = await httpReply.json();
  const firstPage = await wire.call('council_chat_history', { chatId: chat.id, actor: second, limit: 1 });
  assert.equal(firstPage.messages[0].body, body); assert.equal(firstPage.nextCursor, sent.sequence);
  const nextPageResponse = await wire.http(history(chat.id, first, '&after=' + firstPage.nextCursor + '&limit=1'));
  assert.equal(nextPageResponse.status, 200); const nextPage = await nextPageResponse.json();
  assert.equal(nextPage.messages[0].id, reply.id); assert.equal(nextPage.nextCursor, null);
  const list = await wire.call('council_chat_list', { actor: first, roomId: wire.room.id });
  assert.equal(list[0].messageCount, 2); assert.equal(JSON.stringify(list).includes('literal private evidence'), false);
  assert.deepEqual(await wire.call('council_chat_actors', { roomId: wire.room.id }), [second, first].sort());
  const status = await wire.call('council_chat_watch', { chatId: chat.id, actor: second, after: reply.sequence, timeoutMs: 0 });
  assert.equal(status.reason, 'timeout'); assert.equal(JSON.stringify(status).includes('literal private evidence'), false);
  assert.deepEqual(wire.service.rooms.get(wire.room.id), before); assert.equal(wire.calls(), 0);
});

test('private HTTP routes authenticate first, enforce exact query bounds and reject conflicting retries', { timeout: 15000 }, async t => {
  const wire = await fixture(t);
  assert.equal((await wire.http('/api/private-chats?actor=' + first, { authorization: '' })).status, 401);
  assert.equal((await wire.http('/api/private-chats', { method: 'POST', body: { participants: [first, second] }, origin: 'https://example.com' })).status, 403);
  const create = await wire.http('/api/private-chats', { method: 'POST', body: { participants: [first, second], roomId: wire.room.id } });
  assert.equal(create.status, 201); const chat = await create.json();
  assert.equal((await wire.http('/api/private-chats/actors', { method: 'POST', body: {} })).status, 405);
  assert.equal((await wire.http(history(chat.id, third))).status, 400);
  assert.equal((await wire.http(history('unknown-chat'))).status, 400);
  assert.equal((await wire.http('/api/private-chats/' + chat.id + '/messages', { method: 'POST', body: { from: third, body: 'Foreign sender.' } })).status, 400);
  const path = '/api/private-chats/' + chat.id + '/messages';
  const payload = { from: first, body: 'Private canonical content.', idempotencyKey: 'one-accepted-turn' };
  const saved = await (await wire.http(path, { method: 'POST', body: payload })).json();
  const duplicate = await (await wire.http(path, { method: 'POST', body: payload })).json(); assert.equal(duplicate.id, saved.id);
  assert.equal((await wire.http(path, { method: 'POST', body: { ...payload, body: 'Conflicting retry.' } })).status, 409);
  for (const query of ['&after=-1', '&after=1e2', '&after=01', '&after=1&after=2', '&limit=0', '&limit=101', '&extra=1']) {
    assert.equal((await wire.http(history(chat.id, first, query))).status, 400, query);
  }
  for (const query of ['?actor=' + first + '&actor=' + second, '?actor=' + first + '&unexpected=1']) assert.equal((await wire.http('/api/private-chats' + query)).status, 400, query);
  assert.equal((await wire.http('/api/private-chats/actors?actor=' + first)).status, 400);
  assert.equal(wire.service.privateChats.list(first)[0].messageCount, 1); assert.equal(wire.service.rooms.get(wire.room.id).revision, wire.room.revision); assert.equal(wire.calls(), 0);
});

test('MCP private-chat membership errors remain explicit and cancelled long watches do not change messages or rooms', { timeout: 15000 }, async t => {
  const wire = await fixture(t), chat = await wire.call('council_chat_create', { participants: [first, second], roomId: wire.room.id });
  const denied = await wire.request('tools/call', { name: 'council_chat_history', arguments: { chatId: chat.id, actor: third } });
  assert.equal(denied.isError, true); assert.match(denied.content[0].text, /not a participant/);
  const original = wire.service.privateChats.watch.bind(wire.service.privateChats);
  let entered, aborted;
  const started = new Promise(done => { entered = done; }), stopped = new Promise(done => { aborted = done; });
  wire.service.privateChats.watch = (input, signal) => { signal.addEventListener('abort', aborted, { once: true }); entered(); return original(input, signal); };
  const waiting = wire.begin('tools/call', { name: 'council_chat_watch', arguments: { chatId: chat.id, actor: second, timeoutMs: 600000 } });
  await started; assert.equal(wire.server.metrics().pendingRequests, 1);
  wire.input.write(JSON.stringify({ jsonrpc: '2.0', method: 'notifications/cancelled', params: { requestId: waiting.id } }) + '\n');
  await stopped; await nextTurn();
  assert.equal(wire.server.metrics().pendingRequests, 0); assert.equal(wire.frames.some(frame => frame.id === waiting.id), false);
  assert.equal(wire.service.privateChats.listeners.size, 0); assert.equal(wire.service.privateChats.list(first)[0].messageCount, 0);
  assert.equal(wire.service.rooms.get(wire.room.id).revision, wire.room.revision); assert.equal(wire.calls(), 0);
});

test('bounded private discovery pages cross MCP and HTTP without reading message bodies or changing scope', { timeout: 15000 }, async t => {
  const wire = await fixture(t);
  const ids = [];
  for (let i = 0; i < 7; i++) {
    const chat = wire.service.privateChats.create({ participants: [first, 'peer:' + i], roomId: wire.room.id });
    wire.service.privateChats.send({ chatId: chat.id, from: first, body: 'PRIVATE_DISCOVERY_BODY:' + i });
    ids.push(chat.id);
  }
  wire.service.privateChats.create({ participants: [first, third] });
  const toolList = (await wire.request('tools/list', {})).tools;
  for (const name of ['council_chat_list_page', 'council_chat_actors_page']) {
    assert.equal(toolList.find(tool => tool.name === name)?.annotations.readOnlyHint, true);
  }
  let page = await wire.call('council_chat_list_page', { actor: first, roomId: wire.room.id, limit: 2 });
  const seen = [...page.chats];
  assert.equal(page.chats.length, 2); assert.equal(typeof page.nextCursor, 'string');
  const savedCursor = page.nextCursor;
  while (page.nextCursor) {
    const response = await wire.http('/api/private-chats/page?' + new URLSearchParams({ actor: first, roomId: wire.room.id, limit: '2', cursor: page.nextCursor }));
    assert.equal(response.status, 200); page = await response.json(); seen.push(...page.chats);
  }
  assert.deepEqual(seen.map(chat => chat.id).sort(), ids.sort());
  assert.ok(seen.every(chat => chat.messageCount === 1));
  assert.equal(JSON.stringify(seen).includes('PRIVATE_DISCOVERY_BODY:'), false);
  const wrongActor = await wire.request('tools/call', { name: 'council_chat_list_page', arguments: { actor: second, roomId: wire.room.id, cursor: savedCursor } });
  assert.equal(wrongActor.isError, true);
  assert.equal((await wire.http('/api/private-chats/page?' + new URLSearchParams({ actor: first, cursor: savedCursor }))).status, 400);
  let actors = await wire.call('council_chat_actors_page', { roomId: wire.room.id, limit: 3 });
  const labels = [...actors.actors];
  while (actors.nextCursor) {
    const response = await wire.http('/api/private-chats/actors/page?' + new URLSearchParams({ roomId: wire.room.id, limit: '3', cursor: actors.nextCursor }));
    assert.equal(response.status, 200); actors = await response.json(); labels.push(...actors.actors);
  }
  assert.deepEqual(labels, ['peer:0', 'peer:1', 'peer:2', 'peer:3', 'peer:4', 'peer:5', 'peer:6', first].sort());
  assert.equal(wire.calls(), 0);
});

test('private discovery HTTP routes reject ambiguous bounds and preserve exact actor labels', { timeout: 15000 }, async t => {
  const wire = await fixture(t), actor = ' run:exact:actor ';
  wire.service.privateChats.create({ participants: [actor, second] });
  for (const path of ['/api/private-chats/page', '/api/private-chats/actors/page']) {
    assert.equal((await wire.http(path, { authorization: '' })).status, 401);
    assert.equal((await wire.http(path, { method: 'POST', body: {} })).status, 405);
    const base = path.endsWith('/actors/page') ? '?' : '?actor=' + encodeURIComponent(actor) + '&';
    for (const query of ['limit=0', 'limit=101', 'limit=-1', 'limit=01', 'limit=1e1', 'limit=9007199254740992', 'limit=2&limit=3', 'cursor=', 'unexpected=1', 'roomId=&roomId=other']) {
      assert.equal((await wire.http(path + base + query)).status, 400, path + ':' + query);
    }
  }
  assert.equal((await wire.http('/api/private-chats/actors/page?actor=' + encodeURIComponent(actor))).status, 400);
  const exact = await wire.http('/api/private-chats/page?actor=' + encodeURIComponent(actor));
  assert.equal(exact.status, 200); assert.equal((await exact.json()).chats.length, 1);
  const trimmed = await wire.http('/api/private-chats/page?actor=' + encodeURIComponent(actor.trim()));
  assert.equal(trimmed.status, 200); assert.deepEqual((await trimmed.json()).chats, []);
  assert.equal(wire.calls(), 0);
});

test('disconnecting an authenticated HTTP private watch releases the server wait without dispatching peer work', { timeout: 15000 }, async t => {
  const wire = await fixture(t), chat = await wire.call('council_chat_create', { participants: [first, second] });
  const original = wire.service.privateChats.watch.bind(wire.service.privateChats); let entered, aborted;
  const started = new Promise(done => { entered = done; }), stopped = new Promise(done => { aborted = done; });
  wire.service.privateChats.watch = (input, signal) => { signal.addEventListener('abort', aborted, { once: true }); entered(); return original(input, signal); };
  const controller = new AbortController(), pending = wire.http('/api/private-chats/watch', { method: 'POST', body: { chatId: chat.id, actor: second, timeoutMs: Number.MAX_SAFE_INTEGER }, signal: controller.signal });
  await started; controller.abort(); await assert.rejects(pending, /abort/i); await stopped; await nextTurn();
  assert.equal(wire.service.privateChats.listeners.size, 0); assert.equal(wire.service.privateChats.list(first)[0].messageCount, 0); assert.equal(wire.calls(), 0);
});

test('global room SSE never broadcasts private bodies or private-thread metadata', { timeout: 15000 }, async t => {
  const wire = await fixture(t), controller = new AbortController(), response = await wire.http('/api/events', { signal: controller.signal });
  assert.equal(response.status, 200); const reader = response.body.getReader(), decoder = new TextDecoder(); let received = '';
  try {
    while (!received.includes('event: rooms\n')) { const item = await reader.read(); assert.equal(item.done, false); received += decoder.decode(item.value, { stream: true }); }
    const chat = await wire.call('council_chat_create', { participants: [first, second], roomId: wire.room.id });
    const secret = 'PRIVATE_PEER_BODY: Keep this outside every room broadcast.';
    await wire.call('council_chat_send', { chatId: chat.id, from: first, body: secret });
    wire.service.rooms.postMessage(wire.room.id, { author: 'Human', body: 'Public update to synchronize the stream.' });
    while (!received.includes('"revision":2')) { const item = await reader.read(); assert.equal(item.done, false); received += decoder.decode(item.value, { stream: true }); }
    assert.equal(received.includes(secret), false); assert.equal(received.includes(chat.id), false); assert.equal(received.includes(first), false);
    const history = wire.service.rooms.messagePage(wire.room.id, 100); assert.equal(JSON.stringify(history).includes(secret), false); assert.equal(wire.calls(), 0);
  } finally { controller.abort(); try { await reader.cancel(); } catch {} reader.releaseLock(); }
});

test('an oversized complete private message fails MCP transport honestly and remains readable without truncation', { timeout: 15000 }, async t => {
  const wire = await fixture(t);
  const chat = wire.service.privateChats.create({ participants: [first, second] });
  const body = 'Complete private evidence. '.repeat(700_000);
  assert.ok(Buffer.byteLength(body) > 16 * 1024 * 1024);
  const saved = wire.service.privateChats.send({ chatId: chat.id, from: first, body });
  const frame = await wire.begin('tools/call', { name: 'council_chat_history', arguments: { chatId: chat.id, actor: second } }).promise;
  assert.equal(frame.result, undefined);
  assert.match(frame.error?.message ?? '', /MCP response exceeds 32 MiB/);
  assert.doesNotMatch(frame.error.message, /Complete private evidence/);
  const complete = await wire.http(history(chat.id, second));
  assert.equal(complete.status, 200);
  const page = await complete.json();
  assert.equal(page.nextCursor, null);
  assert.deepEqual(page.messages, [saved]);
  assert.deepEqual(await wire.request('ping', {}), {});
  assert.equal(wire.calls(), 0);
});
