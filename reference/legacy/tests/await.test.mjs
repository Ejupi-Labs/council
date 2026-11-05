import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay, setImmediate as nextTurn } from 'node:timers/promises';
import { PassThrough } from 'node:stream';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { createMcpServer } from '../dist/mcp.js';

const task = agent => ({ mode: 'delegate', task: 'Inspect the gated offline example.', agents: [agent], permission: 'read' });

function fixture(t) {
  const base = realpathSync(tmpdir()), root = mkdtempSync(join(base, 'council-await-'));
  const services = [], cleanup = [];
  t.after(async () => {
    for (const dispose of cleanup.toReversed()) await dispose();
    await Promise.all(services.map(service => service.close()));
    const target = resolve(root);
    assert.equal(dirname(target), resolve(base)); assert.ok(basename(target).startsWith('council-await-'));
    rmSync(target, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, cleanup,
    create(runner, overrides = {}) {
      const settings = loadConfig(root, { backend: 'mock', maxConcurrency: 3, maxCalls: 8, maxOutputBytes: 16384,
        agents: { first: { enabled: true, profile: 'codex' }, second: { enabled: true, profile: 'claude' }, thinker: { enabled: true, profile: 'antigravity', model: 'default-advisor-model', effort: 'low' } }, ...overrides });
      const service = new CouncilService(settings, runner); services.push(service); return service;
    },
    waiter() { const controller = new AbortController(); cleanup.push(() => controller.abort()); return controller; },
  };
}

function gated() {
  const calls = [], pending = new Map(), started = [];
  const runner = { calls, doctor: async () => [], run: request => new Promise(resolveResult => {
    calls.push(request);
    let finished = false;
    const finish = (status = 'completed', output = `${request.agent} offline result.`) => {
      if (finished) return; finished = true; request.signal.removeEventListener('abort', cancel);
      resolveResult({ agent: request.agent, status, output, durationMs: 1 });
    };
    const cancel = () => finish('cancelled', 'Offline job cancellation.');
    if (request.signal.aborted) cancel(); else request.signal.addEventListener('abort', cancel, { once: true });
    pending.set(request.agent, finish);
    for (const notify of started.splice(0)) notify();
  }) };
  return { runner, finish: (agent, status, output) => { assert.ok(pending.has(agent), `Agent ${agent} has not entered its gate.`); pending.get(agent)(status, output); },
    async entered(count) { while (calls.length < count) await new Promise(resolveStarted => started.push(resolveStarted)); },
  };
}

test('a ten-minute server wait idles behind a gated job without additional runner dispatch', async t => {
  const wire = fixture(t), gate = gated(), service = wire.create(gate.runner), controller = wire.waiter();
  const run = service.start(task('first')); await gate.entered(1);
  let settled = false;
  const waiting = service.awaitRuns([run.id], 600000, controller.signal).then(value => { settled = true; return value; });
  for (let index = 0; index < 3; index++) {
    await delay(30); assert.equal(gate.runner.calls.length, 1); assert.equal(settled, false);
  }
  gate.finish('first');
  const result = await waiting;
  assert.equal(result.waitingInServer, true); assert.equal(result.timedOut, false);
  assert.equal(result.runs[0].status, 'completed'); assert.equal(gate.runner.calls.length, 1);
});

test('closing an observer wakes foreign waits before storage closes without cancelling the owner job', async t => {
  const wire = fixture(t), gate = gated(), owner = wire.create(gate.runner), observer = wire.create({ doctor: async () => [], run: async () => { throw new Error('Observer must not dispatch.'); } });
  const run = owner.start(task('first')); await gate.entered(1);
  const waiting = observer.awaitRuns([run.id], 10_000);
  const rejected = assert.rejects(waiting, /shutting down/);
  await observer.close();
  await rejected;
  assert.equal(owner.getRun(run.id).status, 'running');
  gate.finish('first');
  assert.equal((await owner.completion(run.id)).status, 'completed');
});

test('MCP waiting returns the complete selected answer beyond ordinary status-preview lengths', async t => {
  const fullAnswer = 'Evidence '.repeat(6000) + 'FINAL_COMPLETE';
  const wire = fixture(t), service = wire.create({ doctor: async () => [], run: async request => ({ agent: request.agent,
    status: 'completed', output: fullAnswer, durationMs: 1 }) }, { maxOutputBytes: 65536 });
  const input = new PassThrough(), output = new PassThrough(), responses = new Map();
  let pending = '';
  output.on('data', chunk => {
    pending += chunk.toString(); let end;
    while ((end = pending.indexOf('\n')) >= 0) {
      const frame = JSON.parse(pending.slice(0, end)); pending = pending.slice(end + 1);
      responses.get(frame.id)?.(frame);
    }
  });
  const server = createMcpServer(service);
  await server.connect(input, output);
  const request = (id, method, params) => new Promise(resolve => {
    responses.set(id, resolve); input.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
  });
  try {
    await request(1, 'initialize', { protocolVersion: '2025-11-25', clientInfo: { name: 'full-output-check', version: '1' }, capabilities: {} });
    const run = service.start(task('first'));
    const response = await request(2, 'tools/call', { name: 'council_wait', arguments: { ids: [run.id] } });
    assert.equal(response.result.structuredContent.runs[0].output, fullAnswer);
    assert.equal(response.result.structuredContent.runs[0].outputTruncated, false);
  } finally { await server.close(); input.destroy(); output.destroy(); }
});

test('local completion wakes immediately for the first changed run and returns after all runs settle', async t => {
  const wire = fixture(t), gate = gated(), service = wire.create(gate.runner), controller = wire.waiter();
  const first = service.start(task('first')), second = service.start(task('second')); await gate.entered(2);
  const read = service.store.getRunSummary.bind(service.store);
  let reads = 0, settled = false;
  service.store.getRunSummary = id => { reads++; return read(id); };
  const waiting = service.awaitRuns([first.id, second.id], 600000, controller.signal).then(value => { settled = true; return value; });
  const initialReads = reads;
  gate.finish('first'); await service.completion(first.id); await nextTurn();
  assert.ok(reads > initialReads, 'The first local completion should wake the server wait.');
  assert.equal(settled, false, 'The remaining local job must keep an all-results wait pending.');
  gate.finish('second'); await service.completion(second.id); await nextTurn();
  assert.equal(settled, true, 'The final local completion should settle without waiting for a polling interval.');
  const result = await waiting;
  assert.equal(result.timedOut, false); assert.deepEqual(result.runs.map(run => run.status), ['completed', 'completed']);
  assert.equal(gate.runner.calls.length, 2);
});

test('a wait timeout returns running summaries and leaves the underlying job active', async t => {
  const wire = fixture(t), gate = gated(), service = wire.create(gate.runner);
  const run = service.start(task('first')); await gate.entered(1);
  const result = await service.awaitRuns([run.id], 30);
  assert.equal(result.timedOut, true); assert.equal(result.runs[0].status, 'running');
  assert.equal(gate.runner.calls[0].signal.aborted, false);
  assert.ok(!service.store.events(run.id).some(event => event.type === 'cancel_requested'));
  gate.finish('first'); assert.equal((await service.completion(run.id)).status, 'completed');
});

test('aborting an await interrupts only the wait and does not cancel its job', async t => {
  const wire = fixture(t), gate = gated(), service = wire.create(gate.runner), controller = wire.waiter();
  const run = service.start(task('first')); await gate.entered(1);
  const waiting = service.awaitRuns([run.id], 600000, controller.signal);
  controller.abort(new Error('Stop waiting for the offline result.'));
  await assert.rejects(waiting, /Stop waiting/);
  assert.equal(gate.runner.calls[0].signal.aborted, false); assert.equal(service.getRun(run.id).status, 'running');
  assert.ok(!service.store.events(run.id).some(event => event.type === 'cancel_requested'));
  gate.finish('first'); assert.equal((await service.completion(run.id)).status, 'completed');
});

test('foreign service completion is observed through bounded metadata polling without a runner call', { timeout: 10000 }, async t => {
  const wire = fixture(t), gate = gated(), owner = wire.create(gate.runner);
  let observerCalls = 0;
  const observer = wire.create({ doctor: async () => [], run: async () => { observerCalls++; throw new Error('A waiting observer must not dispatch.'); } });
  const run = owner.start(task('first')); await gate.entered(1);
  const read = observer.store.getRunSummary.bind(observer.store);
  let reads = 0; observer.store.getRunSummary = id => { reads++; return read(id); };
  const waiting = observer.awaitRuns([run.id], 5000, wire.waiter().signal);
  gate.finish('first'); await owner.completion(run.id);
  const result = await waiting;
  assert.equal(result.timedOut, false); assert.equal(result.runs[0].status, 'completed');
  assert.equal(observerCalls, 0); assert.equal(gate.runner.calls.length, 1);
  assert.ok(reads >= 2 && reads <= 6, `Expected bounded SQLite summary reads, got ${reads}.`);
});

test('terminal and zero-duration waits return directly and validate bounded run ID lists', async t => {
  const wire = fixture(t), gate = gated(), service = wire.create(gate.runner);
  const run = service.start(task('first')); await gate.entered(1);
  assert.equal((await service.awaitRuns([run.id], 0)).timedOut, true);
  for (const ids of [[], [run.id, run.id], [''], ['bad\0id'], Array(21).fill(run.id)]) await assert.rejects(service.awaitRuns(ids, 0));
  for (const timeout of [-1, 600001, NaN, 1.5]) await assert.rejects(service.awaitRuns([run.id], timeout));
  const aborted = new AbortController(); aborted.abort();
  await assert.rejects(service.awaitRuns([run.id], 0, aborted.signal), error => error.name === 'AbortError');
  gate.finish('first'); await service.completion(run.id);
  const result = await service.awaitRuns([run.id], 600000);
  assert.equal(result.timedOut, false); assert.equal(result.runs[0].status, 'completed');
  assert.equal(gate.runner.calls.length, 1);
});

test('service advise preserves requester and explicit advisor model and effort without mutating defaults', async t => {
  const wire = fixture(t), gate = gated(), service = wire.create(gate.runner), before = structuredClone(service.config);
  const run = service.advise({ requester: 'codex-worker', advisor: 'thinker', question: 'Which invariant should we check?', context: 'OFFLINE_ADVISOR_CONTEXT', model: 'explicit-custom-advisor', effort: 'max' });
  await gate.entered(1);
  const call = gate.runner.calls[0];
  assert.equal(call.agent, 'thinker'); assert.equal(call.permission, 'read');
  assert.equal(call.model, 'explicit-custom-advisor'); assert.equal(call.effort, 'max');
  assert.match(call.prompt, /codex-worker/); assert.match(call.prompt, /Which invariant should we check/); assert.match(call.prompt, /OFFLINE_ADVISOR_CONTEXT/);
  assert.ok(!call.prompt.includes('Council advisor protocol')); assert.equal(run.input.maxCalls, 1);
  const waiting = service.awaitRuns([run.id], 600000, wire.waiter().signal);
  gate.finish('thinker', 'completed', 'Check the atomic commit invariant.');
  const result = await waiting;
  assert.equal(result.runs[0].status, 'completed'); assert.equal(gate.runner.calls.length, 1);
  assert.deepEqual(service.config, before);
});

test('independent advice refuses recursive request envelopes without dispatching another agent', async t => {
  const wire = fixture(t), gate = gated(), service = wire.create(gate.runner);
  const run = service.advise({ requester: 'PM', advisor: 'thinker', question: 'Give one focused opinion.' });
  await gate.entered(1);
  gate.finish('thinker', 'completed', JSON.stringify({ council_advisor_request: { question: 'Ask someone else?', context: '' } }));
  const result = await service.completion(run.id);
  assert.equal(result.status, 'failed'); assert.match(result.error, /none is configured/); assert.equal(gate.runner.calls.length, 1);
});

test('MCP exposes server waiting and independent advice through the official SDK', { timeout: 15000 }, async t => {
  const wire = fixture(t), cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
  const transport = new StdioClientTransport({ command: process.execPath, args: [cli, 'mcp', '--workspace', wire.root, '--backend', 'mock'],
    cwd: wire.root, stderr: 'pipe', env: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'mock' } });
  const client = new Client({ name: 'council-offline-await', version: '1.0.0' });
  transport.stderr?.on('data', () => {});
  wire.cleanup.push(() => client.close());
  await client.connect(transport);
  const tools = (await client.listTools()).tools;
  assert.ok(tools.some(tool => tool.name === 'council_wait'));
  assert.ok(tools.some(tool => tool.name === 'council_advise'));
  const call = async (name, args) => {
    const response = await client.callTool({ name, arguments: args });
    assert.notEqual(response.isError, true, JSON.stringify(response.content));
    return JSON.parse(response.content.find(item => item.type === 'text').text);
  };
  const queued = await call('council_advise', { requester: 'PM', advisor: 'codex', question: 'Inspect the local mock example.', model: 'explicit-mock-model', effort: 'max', waitMs: 0 });
  assert.equal(queued.waitingInServer, true); assert.equal(queued.runs.length, 1);
  const completed = await call('council_wait', { ids: [queued.runs[0].id], timeoutMs: 5000 });
  assert.equal(completed.timedOut, false); assert.equal(completed.runs[0].status, 'completed');
  const evidence = await client.readResource({ uri: `council://runs/${queued.runs[0].id}` });
  const run = JSON.parse(evidence.contents[0].text);
  assert.equal(run.steps.length, 1); assert.equal(run.input.agentSettings.codex.model, 'explicit-mock-model');
  const body = 'Complete peer evidence. '.repeat(4000) + 'PEER_EVIDENCE_TAIL';
  const note = 'Complete project evidence. '.repeat(4000) + 'PROJECT_EVIDENCE_TAIL';
  await call('council_send', { from: 'PM', to: 'junie', subject: 'Complete handoff', body });
  assert.equal((await call('council_inbox_page', { agent: 'junie' })).messages[0].body, body);
  await call('council_context', { key: 'full-project-evidence', value: note, author: 'PM' });
  assert.equal((await call('council_context', { key: 'full-project-evidence' })).value, note);
});
