import test from 'node:test';
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { execFile } from 'node:child_process';
import { existsSync, mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { PassThrough } from 'node:stream';
import { setImmediate as nextTurn, setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { loadConfig } from '../dist/config.js';
import { createMcpServer } from '../dist/mcp.js';
import { CouncilService } from '../dist/service.js';
import { VERSION } from '../dist/version.js';

const execute = promisify(execFile);
const cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
const workflow = { mode: 'delegate', agents: ['codex'], task: 'Assess the offline example.', permission: 'read' };
const cutoff = '2025-01-01T00:00:00.000Z';

function workspace(t) {
  const root = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-operations-'));
  const cleanups = [];
  t.after(async () => {
    for (const cleanup of cleanups.toReversed()) await cleanup();
    rmSync(root, { recursive: true, force: true });
  });
  return { root, cleanups };
}

function serviceFor(t, runner) {
  const state = workspace(t);
  const service = new CouncilService(loadConfig(state.root, { backend: 'mock' }), runner);
  state.cleanups.push(() => service.close());
  return { ...state, service };
}

function completed(store, index = 0) {
  const stamp = '2024-01-01T00:00:00.000Z';
  const output = `Private result ${index}: ` + 'x'.repeat(20_000);
  const record = {
    id: randomUUID(), input: { ...workflow, task: `Private task ${index}` }, status: 'completed',
    createdAt: stamp, updatedAt: stamp, output,
    steps: [{ id: `step-${index}`, agent: 'codex', role: 'delegate', result: { agent: 'codex', status: 'completed', output, durationMs: 1 } }],
  };
  store.saveRun(record);
  store.appendEvent(record.id, 'step.completed', { result: record.steps[0].result });
  return record;
}

function countingRunner() {
  let calls = 0;
  return {
    get calls() { return calls; },
    doctor: async () => [],
    async run(request) { calls++; return { agent: request.agent, status: 'completed', output: 'Offline answer.', durationMs: 1 }; },
  };
}

function assertSummary(run) {
  assert.equal(Object.hasOwn(run, 'input'), false);
  assert.equal(Object.hasOwn(run.steps[0] ?? {}, 'result'), false);
  assert.equal(run.fullResultResource, `council://runs/${run.id}`);
}

async function mcpFor(t, service) {
  const input = new PassThrough(), output = new PassThrough();
  const frames = new Map();
  let pending = '', id = 0;
  output.on('data', chunk => {
    pending += chunk.toString('utf8');
    let end;
    while ((end = pending.indexOf('\n')) >= 0) {
      const frame = JSON.parse(pending.slice(0, end));
      pending = pending.slice(end + 1);
      if (Object.hasOwn(frame, 'id')) frames.set(frame.id, frame);
    }
  });
  const server = createMcpServer(service);
  await server.connect(input, output);
  t.after(async () => { await server.close(); input.destroy(); output.destroy(); });
  const request = async (method, params) => {
    const requestId = ++id;
    input.write(JSON.stringify({ jsonrpc: '2.0', id: requestId, method, params }) + '\n');
    const until = Date.now() + 3000;
    while (!frames.has(requestId)) {
      if (Date.now() > until) throw new Error(`MCP request timed out: ${method}`);
      await delay(2);
    }
    const frame = frames.get(requestId); frames.delete(requestId);
    assert.equal(frame.error, undefined, JSON.stringify(frame.error));
    return frame.result;
  };
  await request('initialize', { protocolVersion: '2025-11-25', capabilities: {}, clientInfo: { name: 'operations-tests', version: '1.0.0' } });
  input.write(JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }) + '\n');
  const call = async (name, args = {}) => {
    const result = await request('tools/call', { name, arguments: args });
    assert.notEqual(result.isError, true, result.content?.[0]?.text);
    return JSON.parse(result.content.find(item => item.type === 'text').text);
  };
  return { request, call, input };
}

test('MCP release can target one run without releasing another concurrent task using the same owner label', async t => {
  const { service } = serviceFor(t, countingRunner());
  const first = completed(service.store, 1), second = completed(service.store, 2);
  const { call } = await mcpFor(t, service);
  await call('council_reserve', { owner: 'codex', paths: ['src/first.ts'], runId: first.id });
  await call('council_reserve', { owner: 'codex', paths: ['src/second.ts'], runId: second.id });
  assert.equal((await call('council_release', { owner: 'codex', runId: first.id })).released, 1);
  const remaining = await call('council_leases');
  assert.equal(remaining.length, 1); assert.equal(remaining[0].runId, second.id);
  assert.equal((await call('council_release', { owner: 'codex' })).released, 1);
});

test('large inbox history remains readable as complete bounded pages with recipient-bound cursors', async t => {
  const { service } = serviceFor(t, countingRunner());
  const body = '\u0001'.repeat(32000);
  const expected = [];
  for (let index = 0; index < 50; index++) expected.push(service.store.sendMessage({ from: 'codex', to: 'claude', subject: `Evidence ${index}`, body }));
  service.store.sendMessage({ from: 'codex', to: 'junie', subject: 'Other recipient', body: 'Keep separate.' });
  const { request, call } = await mcpFor(t, service);
  const oversized = await request('tools/call', { name: 'council_inbox', arguments: { agent: 'claude' } });
  assert.equal(oversized.isError, true); assert.match(oversized.content[0].text, /council_inbox_page/);
  const received = [];
  let cursor;
  do {
    const page = await call('council_inbox_page', { agent: 'claude', limit: 100, ...(cursor ? { cursor } : {}) });
    assert.ok(Buffer.byteLength(JSON.stringify(page.messages)) <= 1024 * 1024);
    assert.ok(page.messages.length > 0); assert.ok(page.messages.every(message => message.body === body && message.to === 'claude'));
    received.push(...page.messages.map(message => message.id));
    if (!cursor && page.nextCursor) {
      const wrongRecipient = await request('tools/call', { name: 'council_inbox_page', arguments: { agent: 'junie', cursor: page.nextCursor } });
      assert.equal(wrongRecipient.isError, true);
      const wrongFilter = await request('tools/call', { name: 'council_inbox_page', arguments: { agent: 'claude', unreadOnly: false, cursor: page.nextCursor } });
      assert.equal(wrongFilter.isError, true);
    }
    for (const message of page.messages) assert.equal((await call('council_ack', { id: message.id, agent: 'claude' })).acknowledged, true);
    cursor = page.nextCursor;
  } while (cursor);
  assert.deepEqual(received.toSorted(), expected.map(message => message.id).toSorted()); assert.equal(new Set(received).size, 50);
  assert.deepEqual(await call('council_inbox', { agent: 'claude' }), []);
  assert.equal((await call('council_health')).status, 'ok');
});

function cliCall(root, args) {
  return execute(process.execPath, [cli, ...args, '--workspace', root, '--backend', 'mock'], {
    windowsHide: true, timeout: 8000, maxBuffer: 2 * 1024 * 1024,
    env: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'mock' },
  });
}

test('a rejected durable run creation schedules no provider work', async t => {
  const runner = countingRunner();
  const { service } = serviceFor(t, runner);
  service.store.createRun = () => { throw new Error('Injected creation failure.'); };
  assert.throws(() => service.start(workflow), /Injected creation failure/);
  await nextTurn();
  assert.equal(runner.calls, 0);
  assert.equal(service.health().activeLocalRuns, 0);
  assert.deepEqual(service.runPage().runs, []);
});

test('terminal persistence failure remains observable after completion and blocks new starts', async t => {
  const runner = countingRunner();
  const { service, cleanups } = serviceFor(t, runner);
  const unhandled = [];
  const listener = error => unhandled.push(error);
  process.on('unhandledRejection', listener);
  t.after(() => process.removeListener('unhandledRejection', listener));
  service.store.finalizeRun = () => { throw new Error('Injected terminal persistence failure.'); };
  const queued = service.start(workflow);
  const failed = await service.completion(queued.id);
  assert.equal(failed.status, 'failed');
  assert.match(failed.error, /Run persistence failed/);
  assert.equal(runner.calls, 1);
  assert.equal(service.health().activeLocalRuns, 0);
  assert.equal(service.health().emergencyResults, 1);
  assert.equal(service.health().status, 'degraded');
  // Simulate storage becoming completely unreadable after the job promise has been removed.
  service.store.getRun = () => { throw new Error('No full transcript read is available.'); };
  assert.deepEqual(await service.completion(queued.id), failed);
  assert.equal((await service.waitSummary(queued.id, 0)).status, 'failed');
  assert.equal((await service.wait(queued.id, 0)).status, 'failed');
  assert.throws(() => service.start(workflow), /Run persistence failed/);
  await assert.rejects(service.close(), /unfinished|ownership/i);
  cleanups.pop(); // The expected shutdown rejection was observed; the store has still been closed.
  assert.throws(() => service.store.health(), /closed|not open/i);
  await nextTurn(); await nextTurn();
  assert.equal(runner.calls, 1);
  assert.deepEqual(unhandled, []);
});

test('emergency completion retains new provider output when step and terminal persistence both fail', async t => {
  const runner = countingRunner();
  const { service, cleanups } = serviceFor(t, runner);
  const saveRun = service.store.saveRun.bind(service.store);
  service.store.saveRun = run => {
    if (run.steps.length > 0) throw new Error('Injected step persistence failure.');
    return saveRun(run);
  };
  service.store.finalizeRun = () => { throw new Error('Injected terminal persistence failure.'); };
  const queued = service.start(workflow);
  const failed = await service.completion(queued.id);
  assert.equal(failed.status, 'failed');
  assert.match(failed.error, /Run persistence failed/);
  assert.equal(failed.steps.length, 1);
  assert.equal(failed.steps[0].result.output, 'Offline answer.');
  assert.match(failed.output, /Offline answer/);
  assert.equal((await service.completion(queued.id)).steps[0].result.output, 'Offline answer.');
  assert.equal((await service.waitSummary(queued.id, 0)).steps[0].status, 'completed');
  assert.equal(runner.calls, 1);
  await assert.rejects(service.close(), /unfinished|ownership/i);
  cleanups.pop();
});

test('health inspects metadata and performs integrity work only when explicitly requested', t => {
  const runner = countingRunner();
  const { service } = serviceFor(t, runner);
  completed(service.store);
  service.store.getRun = service.store.listRuns = service.store.events = () => { throw new Error('Full payload read forbidden.'); };
  let scans = 0;
  service.store.checkIntegrity = () => { scans++; return ['ok']; };
  const fast = service.health();
  assert.equal(fast.status, 'ok');
  assert.equal(fast.version, VERSION);
  assert.equal(fast.state.counts.runs, 1);
  assert.equal(fast.integrity, undefined);
  assert.equal(scans, 0);
  assert.deepEqual(service.health(true).integrity, ['ok']);
  assert.equal(scans, 1);
  service.store.checkIntegrity = () => ['Injected malformed page.'];
  assert.equal(service.health(true).status, 'degraded');
  service.store.health = () => { throw new Error('Injected metadata failure.'); };
  assert.match(service.health().issues.join(' '), /could not be inspected/);
  assert.equal(runner.calls, 0);
});

test('run summary pages use stable keyset ordering without loading complete transcripts', async t => {
  const { service } = serviceFor(t, countingRunner());
  const records = Array.from({ length: 5 }, (_, index) => completed(service.store, index));
  service.store.getRun = service.store.listRuns = () => { throw new Error('Full payload read forbidden.'); };
  const ids = [];
  let cursor;
  do {
    const page = service.runPage(2, cursor);
    assert.ok(page.runs.length <= 2);
    page.runs.forEach(assertSummary);
    ids.push(...page.runs.map(run => run.id));
    cursor = page.nextCursor;
  } while (cursor);
  assert.deepEqual(ids, records.map(run => run.id).sort().reverse());
  assert.equal(new Set(ids).size, 5);
  assertSummary(await service.waitSummary(records[0].id, 0));
  assert.equal(service.requestCancellation(records[0].id).requested, false);
  for (const token of ['', 'not-json', Buffer.from(JSON.stringify({ updatedAt: 'invalid', id: 'x' })).toString('base64url')]) {
    assert.throws(() => service.runPage(2, token), /Invalid run page cursor/);
  }
  for (const limit of [0, 101, 1.5]) assert.throws(() => service.runPage(limit), /Page limit/);
});

test('active summary waits and cancellation do not read full result documents', async t => {
  let notify;
  const started = new Promise(resolve => { notify = resolve; });
  const runner = {
    doctor: async () => [],
    run: request => new Promise(resolve => {
      const finish = () => resolve({ agent: request.agent, status: 'cancelled', output: 'Partial offline answer.', durationMs: 1 });
      if (request.signal.aborted) finish();
      else request.signal.addEventListener('abort', finish, { once: true });
      notify();
    }),
  };
  const { service } = serviceFor(t, runner);
  const queued = service.start(workflow);
  await started;
  const getRun = service.store.getRun;
  let fullReads = 0;
  service.store.getRun = () => { fullReads++; throw new Error('Full payload read forbidden.'); };
  try {
    const before = await service.waitSummary(queued.id, 5);
    assert.equal(before.status, 'running');
    const pending = service.waitSummary(queued.id, 3000);
    const request = service.requestCancellation(queued.id);
    assert.equal(request.requested, true);
    assert.equal(request.run.cancellationRequested, true);
    assertSummary(request.run);
    const terminal = await pending;
    assert.equal(terminal.status, 'cancelled');
    assertSummary(terminal);
    assert.equal(fullReads, 0);
  } finally { service.store.getRun = getRun; }
});

test('MCP monitoring tools default to summaries and keep protocol usable after invalid cursors', async t => {
  const runner = countingRunner();
  const { service } = serviceFor(t, runner);
  const saved = completed(service.store);
  service.store.getRun = service.store.listRuns = service.store.events = () => { throw new Error('Full payload read forbidden.'); };
  const wire = await mcpFor(t, service);
  const health = await wire.call('council_health');
  assert.equal(health.status, 'ok');
  assert.equal(health.integrity, undefined);
  assert.ok(health.transport);
  assert.deepEqual((await wire.call('council_health', { integrity: true })).integrity, ['ok']);
  assertSummary(await wire.call('council_status', { id: saved.id }));
  assertSummary((await wire.call('council_run_page', { limit: 1 })).runs[0]);
  const cancelled = await wire.call('council_cancel', { id: saved.id });
  assert.equal(cancelled.requested, false);
  assertSummary(cancelled.run);
  const events = await wire.call('council_events', { id: saved.id });
  assert.equal(Object.hasOwn(events[0].data.result, 'output'), false);
  const bad = await wire.request('tools/call', { name: 'council_run_page', arguments: { cursor: 'bad-cursor' } });
  assert.equal(bad.isError, true);
  assert.equal((await wire.call('council_health')).status, 'ok');
  assert.equal(runner.calls, 0);
});

test('MCP backup is consistent and exclusive while prune defaults to a non-destructive preview', async t => {
  const { root, service } = serviceFor(t, countingRunner());
  const saved = completed(service.store);
  const wire = await mcpFor(t, service);
  const destination = join(root, 'snapshot.db');
  assert.equal((await wire.call('council_backup', { destination })).destination, destination);
  const snapshot = new DatabaseSync(destination, { readOnly: true });
  try {
    assert.equal(snapshot.prepare('SELECT COUNT(*) AS count FROM runs').get().count, 1);
    assert.equal(snapshot.prepare('PRAGMA quick_check').get().quick_check, 'ok');
  } finally { snapshot.close(); }
  const duplicate = await wire.request('tools/call', { name: 'council_backup', arguments: { destination } });
  assert.equal(duplicate.isError, true);
  assert.match(duplicate.content[0].text, /already exists/);
  const preview = await wire.call('council_prune', { before: cutoff, keepLatest: 0 });
  assert.deepEqual(preview.candidateIds, [saved.id]);
  assert.equal(preview.deletedRuns, 0);
  assert.equal(service.runPage().runs.length, 1);
  const applied = await wire.call('council_prune', { before: cutoff, keepLatest: 0, execute: true });
  assert.equal(applied.deletedRuns, 1);
  assert.equal(applied.deletedEvents, 1);
  assert.deepEqual(service.runPage().runs, []);
});

test('MCP artifact refresh returns an empty terminal artifact set without dispatch or state changes', async t => {
  const runner = countingRunner();
  const { service } = serviceFor(t, runner);
  const saved = completed(service.store);
  const before = service.store.getRun(saved.id);
  const events = service.store.events(saved.id);
  const wire = await mcpFor(t, service);
  assert.deepEqual(await wire.call('council_patches', { id: saved.id }), { runId: saved.id, artifacts: [] });
  assert.deepEqual(service.store.getRun(saved.id), before);
  assert.deepEqual(service.store.events(saved.id), events);
  assert.deepEqual(service.store.leases(), []);
  assert.equal(service.health().maintenanceOperations, 0);
  assert.equal(service.health().activeLocalRuns, 0);
  assert.equal(runner.calls, 0);
  assert.equal((await wire.call('council_health')).status, 'ok');
});

test('maintenance cancellation is propagated and drained without an unhandled rejection', async t => {
  const { service } = serviceFor(t, countingRunner());
  let started;
  const ready = new Promise(resolve => { started = resolve; });
  service.store.backupTo = async (_destination, signal) => {
    started();
    await delay(30_000, undefined, { signal });
    throw new Error('Backup unexpectedly ignored cancellation.');
  };
  const controller = new AbortController();
  const work = service.backup('never-published.db', controller.signal);
  const failure = assert.rejects(work, /abort/i);
  await ready;
  assert.equal(service.health().maintenanceOperations, 1);
  controller.abort();
  await failure;
  await nextTurn();
  assert.equal(service.health().maintenanceOperations, 0);
  const draining = service.backup('shutdown-cancelled.db');
  const shutdownFailure = assert.rejects(draining, /abort/i);
  await service.close();
  await shutdownFailure;
});

test('CLI validates operational flags before creating state', { timeout: 20_000 }, async t => {
  const { root } = workspace(t);
  for (const [args, pattern] of [
    [['backup'], /Backup requires --output/],
    [['prune'], /Prune requires --before/],
    [['runs', '--cursor', 'bad'], /requires runs --summary/],
    [['runs', '--integrity'], /not supported by runs/],
    [['health', '--execute'], /not supported by health/],
  ]) {
    await assert.rejects(cliCall(root, args), error => { assert.match(error.stderr, pattern); return true; });
    assert.equal(existsSync(join(root, '.council')), false);
  }
});

test('CLI exposes health, summary cancellation, backups and explicit retention execution', { timeout: 20_000 }, async t => {
  const { root, service } = serviceFor(t, countingRunner());
  const saved = completed(service.store);
  await service.close();
  const call = async args => JSON.parse((await cliCall(root, args)).stdout);
  const health = await call(['health', '--integrity']);
  assert.equal(health.status, 'ok');
  assert.deepEqual(health.integrity, ['ok']);
  assertSummary((await call(['runs', '--summary', '--limit', '1'])).runs[0]);
  assert.deepEqual(await call(['patches', saved.id]), { runId: saved.id, artifacts: [] });
  assert.equal((await call(['events', saved.id])).length, 1);
  const cancellation = await call(['cancel', saved.id, '--summary']);
  assert.equal(cancellation.cancellationRequested, false);
  assertSummary(cancellation.run);
  assert.equal((await call(['cancel', saved.id])).run.input.task, saved.input.task);
  const destination = join(root, 'cli-snapshot.db');
  assert.equal((await call(['backup', '--output', destination])).destination, destination);
  const preview = await call(['prune', '--before', cutoff, '--keep-latest', '0']);
  assert.equal(preview.dryRun, true);
  assert.equal(preview.deletedRuns, 0);
  const applied = await call(['prune', '--before', cutoff, '--keep-latest', '0', '--execute']);
  assert.equal(applied.dryRun, false);
  assert.equal(applied.deletedRuns, 1);
  assert.deepEqual((await call(['runs', '--summary'])).runs, []);
});
