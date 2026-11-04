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
  const root = mkdtempSync(join(tmpdir(), 'council-delegate-integration-')), calls = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), {
    doctor: async () => [], run: async request => {
      calls.push(request);
      const match = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
      const output = match ? JSON.stringify({ vote: 'approve', rationale: 'Offline task has explicit scope and verification.',
        assignments: Object.fromEntries(JSON.parse(match[1]).tasks.map(task => [task.id, 'codex'])) }) : 'Offline task completed and verified.';
      return { agent: request.agent, status: 'completed', durationMs: 1, output };
    },
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
    assert.equal(dirname(resolve(root)), resolve(tmpdir())); assert.ok(basename(root).startsWith('council-delegate-integration-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const begin = (method, params) => {
    const id = ++sequence, promise = new Promise(done => pending.set(id, done));
    input.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n'); return { id, promise };
  };
  const request = async (method, params) => {
    const frame = await begin(method, params).promise;
    assert.equal(frame.error, undefined, JSON.stringify(frame.error)); return frame.result;
  };
  const call = async (name, args = {}) => {
    const result = await request('tools/call', { name, arguments: args });
    assert.notEqual(result.isError, true, result.content?.[0]?.text); return JSON.parse(result.content[0].text);
  };
  await request('initialize', { protocolVersion: '2025-11-25', capabilities: {}, clientInfo: { name: 'delegation-integration', version: '1' } });
  const http = (path, { method = 'GET', body, authorization = token, origin, signal } = {}) => fetch(address.origin + path, { method, signal,
    headers: { ...(authorization ? { Authorization: 'Bearer ' + authorization } : {}), ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}), ...(origin ? { Origin: origin } : {}) },
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}) });
  return { service, calls, call, request, begin, input, frames, http, server };
}

const roomInput = () => ({ title: 'Human delegate integration', brief: 'Coordinate a fully offline review and retain explicit human control.',
  members: [{ agent: 'codex', role: 'lead' }], tasks: [{ title: 'Inspect fixture', brief: 'Inspect the offline fixture and report verification.', permission: 'read' }],
  autoExecute: false, maxCalls: 4 });
const mandate = (roomId, revision = 0) => ({ mode: 'agent', delegate: 'Chief of staff', mandate: 'Coordinate read-only work. Bring consequential decisions to the human.', roomIds: [roomId], expectedRevision: revision });

test('MCP and HTTP round-trip a delegated board, human reply, retained approval and completion', { timeout: 15000 }, async t => {
  const wire = await fixture(t), tools = (await wire.request('tools/list', {})).tools.map(tool => tool.name);
  for (const name of ['status', 'configure', 'report', 'resolve', 'reports', 'changes', 'watch']) assert.ok(tools.includes('council_delegate_' + name));
  const room = await wire.call('council_room_create', roomInput());
  const settings = await wire.call('council_delegate_configure', mandate(room.id)); assert.equal(settings.revision, 1);
  const initial = await wire.call('council_delegate_watch', { timeoutMs: 0 }); assert.equal(initial.reason, 'changed');
  const ready = await wire.call('council_room_start', { id: room.id }); assert.equal(ready.status, 'ready'); assert.equal(wire.calls.length, 1);
  const progress = await wire.call('council_delegate_watch', { cursor: initial.cursor, timeoutMs: 0 });
  assert.equal(progress.reason, 'changed'); assert.equal(progress.rooms[0].status, 'ready');
  const report = await wire.call('council_delegate_report', { author: 'Chief of staff', kind: 'decision', body: 'The board approved the read-only plan. Shall I continue?',
    roomIds: [room.id], expectedRoomRevisions: { [room.id]: ready.revision } });
  const observed = await wire.call('council_delegate_watch', { cursor: progress.cursor, timeoutMs: 0 });
  assert.equal(observed.openQuestions, 1); assert.equal(observed.cursor.reportRevision, 1);
  const replyResponse = await wire.http('/api/delegation/reports/' + report.id + '/resolve', { method: 'POST', body: { author: 'Human', body: 'Proceed within the existing read-only scope.', expectedRevision: report.revision } });
  assert.equal(replyResponse.status, 200); const reply = await replyResponse.json(); assert.equal(reply.resolution.author, 'Human');
  const afterReply = await wire.call('council_delegate_watch', { cursor: observed.cursor, timeoutMs: 0 });
  assert.equal(afterReply.reason, 'changed'); assert.equal(afterReply.openQuestions, 0);
  const changes = await wire.call('council_delegate_changes', { after: observed.cursor.reportRevision, limit: 1 });
  assert.equal(changes.changes.length, 1); assert.equal(changes.changes[0].report.id, report.id);
  assert.equal(changes.changes[0].report.resolution.body, 'Proceed within the existing read-only scope.');
  assert.equal(changes.changes[0].revision, afterReply.cursor.reportRevision);
  const retained = wire.service.rooms.get(room.id);
  assert.equal(retained.revision, ready.revision); assert.equal(retained.instructionRevision, ready.instructionRevision); assert.deepEqual(retained.decision, ready.decision);
  await wire.call('council_room_execute', { id: room.id });
  const completed = await wire.call('council_room_status', { id: room.id, waitMs: 600000 });
  assert.equal(completed.status, 'completed'); assert.equal(completed.tasks[0].status, 'completed'); assert.equal(wire.calls.length, 2);
  const receipt = await wire.call('council_delegate_report', { author: 'Chief of staff', kind: 'completion', body: 'All tracked work completed; the linked workflow retains verification.',
    roomIds: [room.id], expectedRoomRevisions: { [room.id]: completed.revision } });
  const history = await wire.call('council_delegate_reports', { limit: 1 }); assert.equal(history.reports[0].id, report.id); assert.ok(history.nextCursor);
  const later = await wire.call('council_delegate_reports', { cursor: history.nextCursor, limit: 1 }); assert.equal(later.reports[0].id, receipt.id); assert.equal(later.nextCursor, null);
  assert.equal(wire.service.rooms.get(room.id).revision, completed.revision); assert.equal(wire.calls.length, 2);
});

test('authenticated delegation HTTP rejects stale writes and invalid page bounds without model calls', { timeout: 15000 }, async t => {
  const wire = await fixture(t), room = wire.service.rooms.create(roomInput());
  assert.equal((await wire.http('/api/delegation', { authorization: '' })).status, 401);
  assert.equal((await wire.http('/api/delegation', { method: 'POST', body: mandate(room.id), origin: 'https://example.com' })).status, 403);
  const savedResponse = await wire.http('/api/delegation', { method: 'POST', body: mandate(room.id) }); assert.equal(savedResponse.status, 200);
  assert.equal((await wire.http('/api/delegation', { method: 'POST', body: mandate(room.id) })).status, 409);
  assert.equal((await wire.http('/api/delegation')).status, 200);
  const reportResponse = await wire.http('/api/delegation/reports', { method: 'POST', body: { author: 'Chief of staff', kind: 'blocked', body: 'Human decision required.', roomIds: [room.id] } });
  assert.equal(reportResponse.status, 201); const report = await reportResponse.json();
  const resolvePath = '/api/delegation/reports/' + report.id + '/resolve';
  assert.equal((await wire.http(resolvePath, { method: 'POST', body: { author: 'Human', body: 'Continue.', expectedRevision: 0 } })).status, 400);
  assert.equal((await wire.http(resolvePath, { method: 'POST', body: { author: 'Human', body: 'Continue.', expectedRevision: 5 } })).status, 409);
  assert.equal((await wire.http(resolvePath, { method: 'POST', body: { author: 'Human', body: 'Continue.', expectedRevision: 1 } })).status, 200);
  assert.equal((await wire.http(resolvePath, { method: 'POST', body: { author: 'Human', body: 'Stale response.', expectedRevision: 1 } })).status, 409);
  for (const query of ['?limit=1&limit=2', '?cursor=-1', '?cursor=1e2', '?limit=0', '?openOnly=1', '?unexpected=1']) {
    assert.equal((await wire.http('/api/delegation/reports' + query)).status, 400, query);
  }
  for (const query of ['?after=-1', '?after=1&after=2', '?limit=0', '?cursor=1']) {
    assert.equal((await wire.http('/api/delegation/changes' + query)).status, 400, query);
  }
  const changedResponse = await wire.http('/api/delegation/changes?after=1&limit=1'); assert.equal(changedResponse.status, 200);
  const changes = await changedResponse.json(); assert.equal(changes.changes[0].report.id, report.id); assert.equal(changes.changes[0].report.resolution.author, 'Human');
  assert.equal((await wire.call('council_delegate_status')).revision, 1);
  assert.equal(wire.service.rooms.get(room.id).revision, room.revision); assert.equal(wire.calls.length, 0);
});

test('cancelling an MCP delegate watch releases its wait and preserves rooms, mandate and reports', { timeout: 15000 }, async t => {
  const wire = await fixture(t), room = wire.service.rooms.create(roomInput());
  await wire.call('council_delegate_configure', mandate(room.id));
  const before = await wire.call('council_delegate_watch', { timeoutMs: 0 }), original = wire.service.delegation.watch.bind(wire.service.delegation);
  let abortObserved;
  const aborted = new Promise(done => { abortObserved = done; });
  wire.service.delegation.watch = (input, signal) => { signal.addEventListener('abort', abortObserved, { once: true }); return original(input, signal); };
  const waiting = wire.begin('tools/call', { name: 'council_delegate_watch', arguments: { cursor: before.cursor, timeoutMs: 600000 } });
  await nextTurn(); assert.equal(wire.server.metrics().pendingRequests, 1);
  wire.input.write(JSON.stringify({ jsonrpc: '2.0', method: 'notifications/cancelled', params: { requestId: waiting.id } }) + '\n');
  await aborted; await nextTurn();
  assert.equal(wire.server.metrics().pendingRequests, 0); assert.equal(wire.frames.some(frame => frame.id === waiting.id), false);
  assert.equal((await wire.call('council_delegate_status')).revision, before.configuration.revision);
  const after = await wire.call('council_delegate_watch', { cursor: before.cursor, timeoutMs: 0 }); assert.equal(after.reason, 'timeout');
  assert.equal(after.openQuestions, 0); assert.equal(wire.service.rooms.get(room.id).revision, room.revision); assert.equal(wire.calls.length, 0);
});

test('SSE sends compact delegate overview immediately and updates questions without any room change', { timeout: 15000 }, async t => {
  const wire = await fixture(t), room = wire.service.rooms.create(roomInput());
  const privateMandate = 'PRIVATE_MANDATE: Review the complete internal release evidence.';
  await wire.call('council_delegate_configure', { ...mandate(room.id), mandate: privateMandate });
  assert.equal((await wire.http('/api/events', { authorization: '' })).status, 401);
  const controller = new AbortController(), response = await wire.http('/api/events', { signal: controller.signal });
  assert.equal(response.status, 200); assert.match(response.headers.get('content-type'), /text\/event-stream/);
  const reader = response.body.getReader(), decoder = new TextDecoder(), frames = [];
  let buffer = '';
  const next = async (type, predicate = () => true) => {
    for (;;) {
      const end = buffer.indexOf('\n\n');
      if (end >= 0) {
        const frame = buffer.slice(0, end); buffer = buffer.slice(end + 2);
        if (!frame.startsWith('event: ')) continue;
        const name = /^event: ([^\n]+)/.exec(frame)[1], data = JSON.parse(/\ndata: ([\s\S]*)/.exec(frame)[1]);
        frames.push({ name, data }); if (name === type && predicate(data)) return data;
      } else {
        const item = await reader.read(); assert.equal(item.done, false, 'The update stream must remain connected.');
        buffer += decoder.decode(item.value, { stream: true });
      }
    }
  };
  try {
    const initial = await next('delegation');
    assert.equal(frames[0].name, 'delegation', 'The initial overview must precede the periodic room publication.');
    assert.deepEqual(initial, { mode: 'agent', delegate: 'Chief of staff', configurationRevision: 1, reportRevision: 0, openQuestions: 0 });
    assert.equal(JSON.stringify(initial).includes(privateMandate), false); assert.equal(Object.hasOwn(initial, 'rooms'), false);
    // Synchronize the room-only publication before changing just delegation state.
    const rooms = await next('rooms'); assert.equal(rooms[0].id, room.id);
    const roomFrameCount = frames.filter(frame => frame.name === 'rooms').length, roomSummaries = wire.service.rooms.list();
    const privateBody = 'PRIVATE_REPORT_BODY: Preserve complete evidence only in the executive inbox.';
    const report = await wire.call('council_delegate_report', { author: 'Chief of staff', kind: 'decision', body: privateBody, roomIds: [room.id] });
    const question = await next('delegation', value => value.reportRevision === 1);
    assert.equal(question.openQuestions, 1); assert.equal(question.configurationRevision, 1);
    assert.equal(JSON.stringify(question).includes(privateBody), false); assert.equal(Object.hasOwn(question, 'reports'), false);
    assert.equal(frames.filter(frame => frame.name === 'rooms').length, roomFrameCount);
    await wire.call('council_delegate_resolve', { id: report.id, author: 'Human', body: 'The human decision remains in the complete report.', expectedRevision: 1 });
    const resolved = await next('delegation', value => value.reportRevision === 2);
    assert.equal(resolved.openQuestions, 0); assert.equal(resolved.delegate, 'Chief of staff');
    assert.equal(frames.filter(frame => frame.name === 'rooms').length, roomFrameCount);
    assert.deepEqual(wire.service.rooms.list(), roomSummaries); assert.equal(wire.calls.length, 0);
    for (const frame of frames) {
      assert.equal(JSON.stringify(frame).includes(privateBody), false); assert.equal(JSON.stringify(frame).includes(privateMandate), false);
    }
  } finally {
    controller.abort(); await reader.cancel().catch(() => {}); reader.releaseLock();
  }
});
