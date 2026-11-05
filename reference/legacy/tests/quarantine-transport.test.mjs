import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { existsSync, mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
import { CouncilService, CouncilStore, DispatchQuarantinedError, loadConfig } from '../dist/index.js';
import { serveDashboard } from '../dist/dashboard.js';
import { PROCESS_CLEANUP_ERROR } from '../dist/process.js';
import { QUARANTINE_PERSISTENCE_WARNING } from '../dist/dispatch-quarantine.js';

const execute = promisify(execFile), cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
const task = { mode: 'delegate', agents: ['codex'], task: 'No provider may be dispatched from a quarantined workspace.' };
async function fixture(t, { report = true, cleanupFailure = false } = {}) {
  const root = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-quarantine-wire-'));
  let calls = 0;
  const cleanup = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), {
    doctor: async () => [], run: async request => {
      calls++;
      if (cleanupFailure) return { agent: request.agent, status: 'failed', output: 'Offline failure evidence.', durationMs: 1, error: PROCESS_CLEANUP_ERROR };
      assert.fail('The quarantine fixture must not dispatch any provider.');
    },
  });
  t.after(async () => { for (const close of cleanup.reverse()) await close(); await service.close(); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  const room = service.rooms.create({ title: 'Preserve this pending approval', brief: 'Inspect the dispatch incident before starting work.',
    members: [{ agent: 'codex', role: 'lead' }], tasks: [{ title: 'Read evidence', brief: 'Retain the exact task.', permission: 'read' }] });
  const incident = report ? service.store.reportProcessCleanupIncident({ reportKey: 'transport-incident', instanceId: service.instanceId, stage: 'agent_call' }) : undefined;
  const runCli = args => execute(process.execPath, [cli, ...args, '--workspace', root, '--backend', 'mock'], { windowsHide: true, timeout: 15000 });
  return { root, service, room, incident, runCli, cleanup, calls: () => calls };
}

function events(reader) {
  let buffer = '';
  const decoder = new TextDecoder();
  return async (type, predicate = () => true) => {
    for (;;) {
      const end = buffer.indexOf('\n\n');
      if (end >= 0) {
        const frame = buffer.slice(0, end); buffer = buffer.slice(end + 2);
        if (frame.startsWith('event: ' + type + '\n')) {
          const data = JSON.parse(frame.slice(frame.indexOf('data: ') + 6));
          if (predicate(data)) return data;
        }
      } else {
        const item = await reader.read(); assert.equal(item.done, false, 'The status stream must remain connected.');
        buffer += decoder.decode(item.value, { stream: true }); assert(buffer.length < 65536);
      }
    }
  };
}

test('official MCP SDK reads quarantine and health while blocked calls create no workflow or private evidence', { timeout: 25000 }, async t => {
  const wire = await fixture(t), client = new Client({ name: 'quarantine-offline-fixture', version: '1' });
  wire.cleanup.push(() => client.close());
  const transport = new StdioClientTransport({ command: process.execPath, args: [cli, 'mcp', '--workspace', wire.root, '--backend', 'mock'],
    env: { ...process.env, COUNCIL_DEPTH: '0' }, stderr: 'pipe' });
  await client.connect(transport);
  const tools = (await client.listTools()).tools;
  const tool = tools.find(value => value.name === 'council_quarantine');
  assert.equal(tool?.annotations.readOnlyHint, true);
  assert.equal(tools.some(value => /quarantine.*ack/.test(value.name)), false);
  const call = async (name, args = {}) => {
    const result = await client.callTool({ name, arguments: args });
    assert.notEqual(result.isError, true, result.content?.[0]?.text);
    return JSON.parse(result.content.find(value => value.type === 'text').text);
  };
  const page = await call('council_quarantine', { limit: 1 });
  assert.equal(page.blocked, true); assert.equal(page.openCount, 1);
  assert.equal(page.incidents[0].id, wire.incident.id);
  const health = await call('council_health');
  assert.equal(health.status, 'degraded'); assert.equal(health.quarantine.currentRevision, page.currentRevision);
  const refused = await client.callTool({ name: 'council_run', arguments: task });
  assert.equal(refused.isError, true); assert.match(refused.content[0].text, /quarantined/);
  assert.equal(wire.service.store.health().counts.runs, 0);
  await client.ping(); assert.equal(wire.calls(), 0);
});

test('authenticated HTTP exposes strict read-only incident pages and blocks room admission before approval changes', { timeout: 15000 }, async t => {
  const wire = await fixture(t), dashboard = await serveDashboard(wire.service, wire.service.rooms);
  wire.cleanup.push(() => dashboard.close());
  const address = new URL(dashboard.url), token = new URLSearchParams(address.hash.slice(1)).get('token');
  const request = (path, options = {}) => fetch(address.origin + path, { ...options, headers: { Authorization: 'Bearer ' + token, ...options.headers } });
  assert.equal((await fetch(address.origin + '/api/quarantine')).status, 401);
  assert.equal((await request('/api/quarantine', { method: 'POST' })).status, 405);
  for (const query of ['limit=0', 'limit=101', 'limit=01', 'limit=1e2', 'limit=1&limit=2', 'cursor=', 'cursor=01', 'cursor=9223372036854775808', 'unexpected=1']) {
    assert.equal((await request('/api/quarantine?' + query)).status, 400, query);
  }
  const response = await request('/api/quarantine?limit=1'); assert.equal(response.status, 200);
  assert.equal((await response.json()).incidents[0].id, wire.incident.id);
  const before = wire.service.rooms.get(wire.room.id);
  const refusal = await request('/api/rooms/' + wire.room.id + '/discuss', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
  assert.equal(refusal.status, 409); assert.equal((await refusal.json()).code, 'COUNCIL_DISPATCH_QUARANTINED');
  assert.deepEqual(wire.service.rooms.get(wire.room.id), before);
  assert.throws(() => wire.service.start(task), DispatchQuarantinedError);
  assert.equal(wire.service.store.health().counts.runs, 0); assert.equal(wire.calls(), 0);
});

test('CLI incident inspection registers no host and reviewed acknowledgement requires all hosts stopped', { timeout: 30000 }, async t => {
  const wire = await fixture(t);
  const before = wire.service.store.health().counts.instances;
  const page = JSON.parse((await wire.runCli(['quarantine', '--limit', '1'])).stdout);
  assert.equal(page.blocked, true); assert.equal(page.incidents[0].id, wire.incident.id);
  assert.equal(wire.service.store.health().counts.instances, before);
  await assert.rejects(wire.runCli(['quarantine-ack', '--revision', page.currentRevision, '--reviewed']), error => {
    assert.equal(error.code, 1); assert.match(error.stderr, /offline|host|registered|stop/i); return true;
  });
  assert.equal(wire.service.dispatchQuarantinePage().blocked, true);
  await wire.service.close();
  const acknowledged = JSON.parse((await wire.runCli(['quarantine-ack', '--revision', page.currentRevision, '--reviewed'])).stdout);
  assert.equal(acknowledged.blocked, false); assert.equal(acknowledged.openCount, 0);
  assert.equal(acknowledged.acknowledgment.incidentCount, 1);
  const replay = JSON.parse((await wire.runCli(['quarantine-ack', '--revision', page.currentRevision, '--reviewed'])).stdout);
  assert.deepEqual(replay, acknowledged);
  const inspection = new CouncilStore(join(wire.root, '.council'));
  try { assert.equal(inspection.health().counts.instances, 0); assert.equal(inspection.dispatchQuarantinePage().blocked, false); }
  finally { inspection.close(); }
  assert.equal(wire.calls(), 0);
});

test('CLI validates quarantine controls before creating state and empty inspection is metadata-only', { timeout: 30000 }, async t => {
  const root = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-quarantine-empty-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const run = args => execute(process.execPath, [cli, ...args, '--workspace', root, '--backend', 'mock'], { windowsHide: true, timeout: 15000 });
  const empty = JSON.parse((await run(['quarantine'])).stdout);
  assert.deepEqual(empty, { blocked: false, currentRevision: '0', openCount: 0, incidents: [], nextCursor: null });
  for (const args of [['quarantine', '--limit', '0'], ['quarantine', '--cursor', '01'], ['quarantine-ack', '--revision', '1'],
    ['quarantine-ack', '--revision', '01', '--reviewed'], ['quarantine-ack', '--revision', '9223372036854775808', '--reviewed'], ['quarantine-ack', '--revision', '0', '--reviewed']]) {
    await assert.rejects(run(args), error => error.code === 1);
  }
  assert.equal(existsSync(join(root, '.council')), false);
});

test('status SSE emits compact incident revisions independently of unchanged rooms and excludes incident identities', { timeout: 15000 }, async t => {
  const wire = await fixture(t), dashboard = await serveDashboard(wire.service, wire.service.rooms);
  wire.cleanup.push(() => dashboard.close());
  const address = new URL(dashboard.url), token = new URLSearchParams(address.hash.slice(1)).get('token'), controller = new AbortController();
  const response = await fetch(address.origin + '/api/events', { headers: { Authorization: 'Bearer ' + token }, signal: AbortSignal.any([controller.signal, AbortSignal.timeout(7000)]) });
  const reader = response.body.getReader(), next = events(reader);
  try {
    const first = await next('quarantine', value => value.currentRevision === wire.incident.generation);
    assert.deepEqual(first, { blocked: true, currentRevision: wire.incident.generation, openCount: 1, hostId: wire.service.instanceId, localRevision: '0' });
    wire.service.store.reportProcessCleanupIncident({ reportKey: 'second-transport-incident', instanceId: wire.service.instanceId, stage: 'git_export' });
    const second = await next('quarantine', value => value.openCount === 2);
    assert.equal(second.blocked, true); assert.notEqual(second.currentRevision, first.currentRevision);
    assert.deepEqual(Object.keys(second).sort(), ['blocked', 'currentRevision', 'hostId', 'localRevision', 'openCount']);
    assert.equal(JSON.stringify(second).includes(wire.incident.id), false);
    assert.equal(JSON.stringify(second).includes(wire.incident.reportKey), false);
    assert.equal(wire.service.rooms.get(wire.room.id).revision, wire.room.revision); assert.equal(wire.calls(), 0);
  } finally { controller.abort(); try { await reader.cancel(); } catch {} reader.releaseLock(); }
});

test('host status identifies local sessions and monotonically orders fault promotion independently of shared revision', { timeout: 10000 }, async t => {
  const wire = await fixture(t, { report: false }), initial = wire.service.dispatchStatus();
  t.mock.method(wire.service.store, 'heartbeat', () => false);
  wire.service.heartbeatTick();
  const stateFault = wire.service.dispatchQuarantinePage();
  assert.equal(stateFault.currentRevision, initial.currentRevision);
  assert(BigInt(stateFault.localRevision) > BigInt(initial.localRevision));
  wire.service.engine.fenceProcessCleanup();
  wire.service.engine.noteIncidentPersistenceFailure();
  const processFault = wire.service.dispatchStatus();
  assert.equal(processFault.hostId, initial.hostId);
  assert.equal(processFault.currentRevision, initial.currentRevision);
  assert(BigInt(processFault.localRevision) > BigInt(stateFault.localRevision));
  assert.match(processFault.localFault, /cleanup could not be verified/);
  assert.equal(processFault.persistenceWarning, QUARANTINE_PERSISTENCE_WARNING);
  assert.match(stateFault.localFault, /ownership was lost/);
  assert.notEqual(stateFault.localFault, processFault.localFault);
  await wire.service.close();
  const fresh = new CouncilService(loadConfig(wire.root, { backend: 'mock' })); wire.cleanup.push(() => fresh.close());
  const reopened = fresh.dispatchStatus();
  assert.notEqual(reopened.hostId, initial.hostId); assert.equal(reopened.localRevision, '0');
  assert.equal(reopened.localFault, undefined); assert.equal(reopened.persistenceWarning, undefined);
});

test('status SSE retains local fault honesty and room updates after failed durable incident persistence', { timeout: 15000 }, async t => {
  const wire = await fixture(t, { report: false, cleanupFailure: true }), manager = wire.service.rooms;
  const dashboard = await serveDashboard(wire.service, manager); wire.cleanup.push(() => dashboard.close());
  const address = new URL(dashboard.url), token = new URLSearchParams(address.hash.slice(1)).get('token'), controller = new AbortController();
  const headers = { Authorization: 'Bearer ' + token };
  const response = await fetch(address.origin + '/api/events', { headers, signal: AbortSignal.any([controller.signal, AbortSignal.timeout(7000)]) });
  const reader = response.body.getReader(), next = events(reader);
  try {
    assert.equal((await next('quarantine')).blocked, false);
    t.mock.method(wire.service.store, 'reportProcessCleanupIncident', () => { throw new Error('PRIVATE_REPORT_DIAGNOSTIC'); });
    assert.equal((await wire.service.run(task)).status, 'failed');
    const fault = await next('quarantine', value => Boolean(value.localFault));
    assert.equal(fault.blocked, false); assert.equal(fault.persistenceWarning, QUARANTINE_PERSISTENCE_WARNING);
    assert.match(fault.localFault, /cleanup could not be verified/); assert(!JSON.stringify(fault).includes('PRIVATE_REPORT_DIAGNOSTIC'));
    const metadata = await fetch(address.origin + '/api/quarantine', { headers }); assert.equal(metadata.status, 200);
    assert.equal((await metadata.json()).persistenceWarning, QUARANTINE_PERSISTENCE_WARNING);
    const before = manager.get(wire.room.id).revision;
    manager.postMessage(wire.room.id, { author: 'Fixture', body: 'Keep the room status stream available for inspection.' });
    const rooms = await next('rooms', value => value.some(room => room.id === wire.room.id && room.revision > before));
    assert.equal(rooms.find(room => room.id === wire.room.id).revision, before + 1);
    assert.equal(wire.calls(), 1);
  } finally { controller.abort(); try { await reader.cancel(); } catch {} reader.releaseLock(); }
});
