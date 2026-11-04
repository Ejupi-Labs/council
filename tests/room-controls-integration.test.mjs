import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { serveDashboard } from '../dist/dashboard.js';
import { createMcpServer } from '../dist/mcp.js';
import { CouncilMcpServer } from '../dist/rpc.js';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';

const task = title => ({ title, brief: 'Inspect the complete offline evidence and report verification.', permission: 'read' });
const roomInput = (autoExecute = true) => ({ title: 'Explicit pause control', brief: 'Verify durable pauses without replay or additional model calls.',
  members: [{ agent: 'worker', role: 'lead' }], tasks: [task('First task'), { ...task('Dependent task'), dependsOn: ['task-1'] }], autoExecute, maxCalls: 8 });

function temporary() { return mkdtempSync(join(tmpdir(), 'council-room-controls-')); }
function removeTemporary(root) { assert.equal(dirname(resolve(root)), resolve(tmpdir())); assert.ok(basename(root).startsWith('council-room-controls-')); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); }

async function fixture(t) {
  const root = temporary(), calls = [], workers = [], notifications = [], servers = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxConcurrency: 1, agents: { worker: { enabled: true, profile: 'codex' } } }), {
    doctor: async () => [], run: request => {
      calls.push(request);
      const match = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
      if (match) return Promise.resolve({ agent: request.agent, status: 'completed', durationMs: 1, output: JSON.stringify({ vote: 'approve', rationale: 'The offline tasks have clear acceptance checks.', assignments: Object.fromEntries(JSON.parse(match[1]).tasks.map(task => [task.id, 'worker'])) }) });
      return new Promise(resolveResult => {
        let settled = false;
        const finish = () => { if (settled) return; settled = true; request.signal.removeEventListener('abort', finish); resolveResult({ agent: request.agent, status: request.signal.aborted ? 'cancelled' : 'completed', durationMs: 1, output: 'Complete retained evidence for this task.' }); };
        workers.push({ request, finish }); request.signal.addEventListener('abort', finish, { once: true });
        for (const notify of notifications.splice(0)) notify();
      });
    },
  });
  t.after(async () => { for (const worker of workers) worker.finish(); for (const server of servers) await server.close(); await service.close(); removeTemporary(root); });
  const dashboard = await serveDashboard(service, service.rooms); servers.push(dashboard);
  const url = new URL(dashboard.url), token = new URLSearchParams(url.hash.slice(1)).get('token');
  return { service, calls, workers, servers,
    async entered(count) { while (workers.length < count) await new Promise(done => notifications.push(done)); },
    post: (roomId, action, options = {}) => fetch(url.origin + '/api/rooms/' + roomId + '/' + action, {
      method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...options.headers }, body: '{}',
    }),
  };
}

function registered(t, wire) {
  const handlers = new Map(), original = CouncilMcpServer.prototype.registerTool;
  const spy = t.mock.method(CouncilMcpServer.prototype, 'registerTool', function (definition, handler) { handlers.set(definition.name, handler); return original.call(this, definition, handler); });
  const server = createMcpServer(wire.service); spy.mock.restore(); wire.servers.push(server);
  return (name, args, signal = new AbortController().signal) => handlers.get(name)(args, signal);
}

test('authenticated HTTP pause drains admitted work and resume dispatches only the pending dependent task', { timeout: 15000 }, async t => {
  const wire = await fixture(t), room = wire.service.rooms.create(roomInput());
  wire.service.rooms.startDiscussion(room.id); await wire.entered(1);
  const revision = wire.service.rooms.get(room.id).revision;
  assert.equal((await wire.post(room.id, 'pause', { headers: { Authorization: '' } })).status, 401);
  assert.equal((await wire.post(room.id, 'pause', { headers: { Origin: 'https://example.com' } })).status, 403);
  assert.equal(wire.service.rooms.get(room.id).revision, revision);
  const response = await wire.post(room.id, 'pause'); assert.equal(response.status, 200);
  assert.equal((await response.json()).pauseRequested, true); assert.equal(wire.workers[0].request.signal.aborted, false);
  wire.workers[0].finish(); const paused = await wire.service.rooms.wait(room.id);
  assert.equal(paused.status, 'paused'); assert.equal(paused.tasks[0].status, 'completed'); assert.equal(wire.calls.length, 2);
  const previousRun = paused.tasks[0].runId;
  const resumed = await wire.post(room.id, 'resume'); assert.equal(resumed.status, 200); await wire.entered(2);
  assert.equal(wire.workers[1].request.prompt.includes('Room task task-2'), true);
  wire.workers[1].finish(); const complete = await wire.service.rooms.wait(room.id);
  assert.equal(complete.status, 'completed'); assert.equal(complete.tasks[0].runId, previousRun); assert.equal(complete.callsUsed, 3);
});

test('ready HTTP pause and resume preserve approval and do not execute tasks', async t => {
  const wire = await fixture(t), room = wire.service.rooms.create(roomInput(false));
  wire.service.rooms.startDiscussion(room.id); const ready = await wire.service.rooms.wait(room.id); assert.equal(ready.status, 'ready');
  assert.equal((await wire.post(room.id, 'pause')).status, 200);
  assert.equal(wire.service.rooms.get(room.id).status, 'paused');
  assert.equal((await wire.post(room.id, 'resume')).status, 200);
  const resumed = wire.service.rooms.get(room.id); assert.equal(resumed.status, 'ready'); assert.deepEqual(resumed.decision, ready.decision);
  assert.equal(wire.calls.length, 1); assert.equal(wire.workers.length, 0);
});

test('pre-aborted MCP pause and resume leave room state unchanged', async t => {
  const wire = await fixture(t), invoke = registered(t, wire), room = wire.service.rooms.create(roomInput(false));
  wire.service.rooms.startDiscussion(room.id); const ready = await wire.service.rooms.wait(room.id);
  const controller = new AbortController(); controller.abort();
  await assert.rejects(invoke('council_room_pause', { id: room.id, waitMs: 0 }, controller.signal), error => error.name === 'AbortError');
  assert.equal(wire.service.rooms.get(room.id).revision, ready.revision);
  const paused = wire.service.rooms.pause(room.id);
  await assert.rejects(invoke('council_room_resume', { id: room.id, waitMs: 0 }, controller.signal), error => error.name === 'AbortError');
  assert.equal(wire.service.rooms.get(room.id).revision, paused.revision); assert.equal(wire.calls.length, 1);
});

test('cancelling an MCP pause wait preserves the durable pause request and admitted workflow', { timeout: 15000 }, async t => {
  const wire = await fixture(t), invoke = registered(t, wire), room = wire.service.rooms.create(roomInput());
  wire.service.rooms.startDiscussion(room.id); await wire.entered(1);
  const controller = new AbortController(), pending = invoke('council_room_pause', { id: room.id, waitMs: 600000 }, controller.signal);
  controller.abort(); await assert.rejects(pending, error => error.name === 'AbortError');
  assert.equal(wire.service.rooms.get(room.id).pauseRequested, true); assert.equal(wire.workers[0].request.signal.aborted, false);
  wire.workers[0].finish(); assert.equal((await wire.service.rooms.wait(room.id)).status, 'paused');
  const resumed = await invoke('council_room_resume', { id: room.id, waitMs: 0 }); assert.equal(resumed.messages, undefined);
  await wire.entered(2); wire.workers[1].finish(); assert.equal((await wire.service.rooms.wait(room.id)).status, 'completed');
});

test('official MCP client discovers durable room controls and round-trips a paused approval', { timeout: 15000 }, async t => {
  const root = temporary(), cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
  const transport = new StdioClientTransport({ command: process.execPath, args: [cli, 'mcp', '--workspace', root, '--backend', 'mock'], cwd: root,
    stderr: 'pipe', env: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'mock' } });
  transport.stderr?.on('data', () => {});
  const client = new Client({ name: 'council-pause-acceptance', version: '1' }); t.after(async () => { await client.close(); removeTemporary(root); }); await client.connect(transport);
  const names = (await client.listTools()).tools.map(tool => tool.name);
  assert.ok(names.includes('council_room_pause')); assert.ok(names.includes('council_room_resume'));
  const call = async (name, args) => { const result = await client.callTool({ name, arguments: args }); assert.notEqual(result.isError, true); return JSON.parse(result.content.find(item => item.type === 'text').text); };
  const room = await call('council_room_create', { title: 'Offline approval checkpoint', brief: 'Test persisted control without live providers.', members: [{ agent: 'codex', role: 'lead' }], tasks: [task('Inspect fixture')], autoExecute: false, maxCalls: 4 });
  const ready = await call('council_room_start', { id: room.id }); assert.equal(ready.status, 'ready');
  const paused = await call('council_room_pause', { id: room.id }); assert.equal(paused.status, 'paused'); assert.equal(paused.messages, undefined);
  const resumed = await call('council_room_resume', { id: room.id }); assert.equal(resumed.status, 'ready'); assert.equal(resumed.callsUsed, ready.callsUsed);
  await call('council_room_execute', { id: room.id }); const completed = await call('council_room_status', { id: room.id, waitMs: 10000 });
  assert.equal(completed.status, 'completed'); assert.equal(completed.callsUsed, 2);
});
