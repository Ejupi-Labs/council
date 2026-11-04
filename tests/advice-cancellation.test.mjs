import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { PassThrough } from 'node:stream';
import { setImmediate as nextTurn } from 'node:timers/promises';
import { CouncilService } from '../dist/service.js';
import { createMcpServer } from '../dist/mcp.js';
import { CouncilMcpServer } from '../dist/rpc.js';
import { loadConfig } from '../dist/config.js';

const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; };
const advice = waitMs => ({ requester: 'PM', advisor: 'advisor', question: 'Inspect the offline invariant.', waitMs });
const task = { mode: 'delegate', agents: ['other'], task: 'Independent offline work.', permission: 'read' };

function fixture(t, delayedDrain = false) {
  const base = realpathSync(tmpdir()), root = mkdtempSync(join(base, 'council-advice-cancel-'));
  const calls = [], notifications = [], servers = [], streams = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock', agents: {
    advisor: { enabled: true, profile: 'codex' }, other: { enabled: true, profile: 'claude' },
  } }), { doctor: async () => [], run: request => new Promise(resolveResult => {
    const aborted = deferred(), drain = deferred();
    let settled = false;
    const finish = (status = 'completed', output = 'Complete offline evidence.') => {
      if (settled) return; settled = true; request.signal.removeEventListener('abort', cancel);
      resolveResult({ agent: request.agent, status, output, durationMs: 1 });
    };
    const cancel = () => {
      aborted.resolve();
      if (delayedDrain && request.agent === 'advisor') void drain.promise.then(() => finish('cancelled', 'CANCELLED_ADVISOR_PARTIAL'));
      else finish('cancelled', 'Cancelled offline evidence.');
    };
    calls.push({ request, aborted, drain, finish });
    request.signal.addEventListener('abort', cancel, { once: true });
    if (request.signal.aborted) cancel();
    for (const notify of notifications.splice(0)) notify();
  }) });
  t.after(async () => {
    for (const call of calls) { call.drain.resolve(); call.finish(call.request.signal.aborted ? 'cancelled' : 'completed'); }
    await Promise.all(servers.map(server => server.close())); await service.close();
    for (const stream of streams) stream.destroy();
    assert.equal(dirname(resolve(root)), resolve(base)); assert.ok(basename(root).startsWith('council-advice-cancel-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { service, calls, servers, streams,
    async entered(count) { while (calls.length < count) await new Promise(done => notifications.push(done)); },
    advisorRun() { return service.store.listRuns(20).find(run => run.input.agents[0] === 'advisor'); },
  };
}

// Capture the public registration boundary so pre-aborted handler signals can
// be exercised without relying on private server fields or a vendor process.
function registeredTools(t, wire) {
  const handlers = new Map(), register = CouncilMcpServer.prototype.registerTool;
  const spy = t.mock.method(CouncilMcpServer.prototype, 'registerTool', function (definition, handler) {
    handlers.set(definition.name, handler); return register.call(this, definition, handler);
  });
  const server = createMcpServer(wire.service); spy.mock.restore(); wire.servers.push(server);
  return { server, invoke: (name, args, signal) => handlers.get(name)(args, signal) };
}

test('pre-aborted independent advice does not create a run or dispatch a runner', async t => {
  const wire = fixture(t), tools = registeredTools(t, wire), controller = new AbortController(); controller.abort();
  await assert.rejects(tools.invoke('council_advise', advice(600000), controller.signal), error => error.name === 'AbortError');
  assert.equal(wire.calls.length, 0); assert.deepEqual(wire.service.store.listRuns(20), []);
});

test('aborting blocking advice cancels and drains only its newly created run', { timeout: 10000 }, async t => {
  const wire = fixture(t, true), tools = registeredTools(t, wire);
  const independent = wire.service.start(task); await wire.entered(1);
  const controller = new AbortController(); let settled = false;
  const pending = tools.invoke('council_advise', advice(600000), controller.signal);
  pending.then(() => { settled = true; }, () => { settled = true; });
  const rejected = assert.rejects(pending, error => error.name === 'AbortError');
  await wire.entered(2); const own = wire.calls.find(call => call.request.agent === 'advisor');
  const ownRun = wire.advisorRun(); controller.abort(); await own.aborted.promise; await nextTurn();
  assert.equal(settled, false, 'The cancelled handler must wait until its owned work drains.');
  assert.equal(wire.service.getRun(ownRun.id).status, 'running');
  assert.equal(wire.calls[0].request.signal.aborted, false, 'Unrelated work must remain active.');
  own.drain.resolve(); await rejected;
  const final = wire.service.getRun(ownRun.id);
  assert.equal(final.status, 'cancelled'); assert.match(final.output, /CANCELLED_ADVISOR_PARTIAL/);
  assert.equal(wire.service.store.isCancellationRequested(ownRun.id), true);
  assert.equal(wire.service.store.isCancellationRequested(independent.id), false);
  wire.calls[0].finish(); assert.equal((await wire.service.completion(independent.id)).status, 'completed');
});

test('advice wait timeout returns a recoverable ID and detaches request cancellation ownership', async t => {
  const wire = fixture(t), tools = registeredTools(t, wire), controller = new AbortController();
  const result = await tools.invoke('council_advise', advice(0), controller.signal); await wire.entered(1);
  assert.equal(result.timedOut, true); assert.equal(result.runs.length, 1);
  const id = result.runs[0].id; assert.equal(id, wire.advisorRun().id);
  controller.abort(); await nextTurn();
  assert.equal(wire.calls[0].request.signal.aborted, false);
  assert.equal(wire.service.store.isCancellationRequested(id), false);
  wire.calls[0].finish(); assert.equal((await wire.service.completion(id)).status, 'completed');
});

test('cancelling council_wait continues to stop only the wait rather than its underlying run', async t => {
  const wire = fixture(t), tools = registeredTools(t, wire), controller = new AbortController();
  const run = wire.service.start(task); await wire.entered(1);
  const pending = tools.invoke('council_wait', { ids: [run.id], timeoutMs: 600000 }, controller.signal);
  controller.abort(); await assert.rejects(pending, error => error.name === 'AbortError');
  assert.equal(wire.calls[0].request.signal.aborted, false);
  assert.equal(wire.service.store.isCancellationRequested(run.id), false);
  wire.calls[0].finish(); assert.equal((await wire.service.completion(run.id)).status, 'completed');
});

for (const operation of ['getRunSummary', 'requestCancellation']) {
  test(`advice abort stops its local job when ${operation} fails and retains the original storage error`, async t => {
    const wire = fixture(t), tools = registeredTools(t, wire), controller = new AbortController();
    const other = wire.service.start(task); await wire.entered(1);
    const pending = tools.invoke('council_advise', advice(600000), controller.signal);
    const rejected = assert.rejects(pending, error => /cancellation could not be persisted/i.test(error.message)
      && error.cause?.message === `Injected ${operation} failure.`);
    await wire.entered(2); const run = wire.advisorRun();
    const original = wire.service.store[operation].bind(wire.service.store);
    const spy = t.mock.method(wire.service.store, operation, (...args) => {
      if (args[0] === run.id) throw new Error(`Injected ${operation} failure.`);
      return original(...args);
    });
    controller.abort(); spy.mock.restore(); // Restore reads before completion inspects retained state.
    await rejected;
    assert.equal(wire.calls.find(call => call.request.agent === 'advisor').request.signal.aborted, true);
    assert.equal(wire.service.getRun(run.id).status, 'cancelled');
    assert.equal(wire.calls[0].request.signal.aborted, false);
    assert.equal(wire.service.store.isCancellationRequested(other.id), false);
    wire.calls[0].finish(); assert.equal((await wire.service.completion(other.id)).status, 'completed');
  });
}

test('MCP cancellation notifications stop orphaned blocking advice and preserve the session', { timeout: 10000 }, async t => {
  const wire = fixture(t), { server } = registeredTools(t, wire);
  const input = new PassThrough(), output = new PassThrough(); wire.streams.push(input, output);
  const responses = new Map(), waiters = new Map(); let buffer = '';
  output.on('data', data => {
    buffer += data.toString(); let end;
    while ((end = buffer.indexOf('\n')) >= 0) {
      const message = JSON.parse(buffer.slice(0, end)); buffer = buffer.slice(end + 1);
      responses.set(message.id, message); waiters.get(message.id)?.(message);
    }
  });
  await server.connect(input, output);
  const send = message => input.write(JSON.stringify({ jsonrpc: '2.0', ...message }) + '\n');
  const request = (id, method, params) => new Promise(done => { waiters.set(id, done); send({ id, method, params }); });
  await request(1, 'initialize', { protocolVersion: '2025-11-25', clientInfo: { name: 'advice-cancel-fixture', version: '1' }, capabilities: {} });
  send({ method: 'notifications/initialized' });
  send({ id: 2, method: 'tools/call', params: { name: 'council_advise', arguments: advice(600000) } });
  await wire.entered(1); const run = wire.advisorRun();
  send({ method: 'notifications/cancelled', params: { requestId: 2, reason: 'Host stopped waiting.' } });
  await wire.calls[0].aborted.promise;
  assert.equal((await wire.service.completion(run.id)).status, 'cancelled'); await nextTurn();
  assert.equal(responses.has(2), false, 'Cancelled requests need not receive their lost run handle.');
  assert.equal(server.metrics().pendingRequests, 0); assert.equal(wire.service.health().activeLocalRuns, 0);
  assert.ok((await request(3, 'ping', {})).result);
});
