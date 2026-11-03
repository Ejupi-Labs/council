import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { promisify } from 'node:util';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
import { loadConfig } from '../dist/config.js';
import { CouncilService } from '../dist/service.js';
import { CouncilStore } from '../dist/store.js';

const execute = promisify(execFile);
const cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
const workflow = { mode: 'council', agents: ['codex', 'claude'], task: 'Assess this example without changing files.', permission: 'read' };

function workspace(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-integration-'));
  t.councilCleanup = [];
  t.after(async () => {
    for (const cleanup of t.councilCleanup.toReversed()) await cleanup();
    rmSync(root, { recursive: true, force: true });
  });
  return root;
}

async function connect(t, root, entrypoint = cli, cliMode = true) {
  const transport = new StdioClientTransport({ command: process.execPath,
    args: cliMode ? [entrypoint, 'mcp', '--workspace', root, '--backend', 'mock'] : [entrypoint],
    cwd: root, stderr: 'pipe', env: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_BACKEND: 'mock' } });
  const client = new Client({ name: 'council-offline-integration', version: '1.0.0' });
  const errors = [];
  client.onerror = error => errors.push(error.message);
  transport.stderr?.on('data', () => { /* Keep protocol stdout separate from process diagnostics. */ });
  await client.connect(transport);
  let closed = false;
  const close = async () => { if (!closed) { closed = true; await client.close(); } };
  t.councilCleanup.push(close);
  const call = async (name, args = {}) => {
    const response = await client.callTool({ name, arguments: args });
    assert.notEqual(response.isError, true, `${name}: ${response.content?.map(item => item.text ?? '').join(' ')}`);
    const text = response.content.find(item => item.type === 'text')?.text;
    assert.equal(typeof text, 'string');
    return JSON.parse(text);
  };
  return { client, transport, call, close, errors };
}

test('real MCP stdio CLI plans without starting runs, then completes and exposes events/resources', { timeout: 15_000 }, async t => {
  const root = workspace(t);
  const { client, call, close, errors } = await connect(t, root);
  const tools = await client.listTools();
  assert.ok(tools.tools.some(tool => tool.name === 'council_run'));
  assert.ok(tools.tools.some(tool => tool.name === 'council_context'));
  const health = await call('council_agents');
  assert.equal(health.backend, 'mock');
  const plan = await call('council_plan', workflow);
  assert.equal(plan.estimatedCalls, 5);
  assert.deepEqual(await call('council_runs'), []);
  const queued = await call('council_run', workflow);
  assert.equal(queued.status, 'queued');
  const run = await call('council_status', { id: queued.id, waitMs: 3000, includeResults: true });
  assert.equal(run.status, 'completed');
  assert.equal(run.steps.length, 5);
  assert.ok(run.steps.every(step => step.result.output.includes('[MOCK')));
  const events = await call('council_events', { id: run.id });
  assert.equal(events[0].type, 'queued');
  assert.ok(events.some(event => event.type === 'run.started'));
  assert.ok(events.some(event => event.type === 'run.completed'));
  assert.ok(events.every((event, index) => index === 0 || event.sequence > events[index - 1].sequence));
  assert.deepEqual(await call('council_events', { id: run.id, after: events.at(-1).sequence }), []);
  const resource = await client.readResource({ uri: `council://runs/${run.id}` });
  assert.equal(JSON.parse(resource.contents[0].text).status, 'completed');
  const modes = await client.readResource({ uri: 'council://modes' });
  assert.equal(JSON.parse(modes.contents[0].text).modes.length, 6);
  assert.equal(errors.length, 0, `MCP protocol errors: ${errors.join('; ')}`);
  await close();
  const reopened = new CouncilStore(join(root, '.council'));
  assert.equal(reopened.getRun(run.id).status, 'completed');
  reopened.close();
});

test('MCP mailbox acknowledgements, context and lease errors preserve the server session', { timeout: 15_000 }, async t => {
  const root = workspace(t);
  const { client, call } = await connect(t, root);
  const message = await call('council_send', { from: 'codex', to: 'claude', subject: 'Review request', body: 'Please inspect the bounded task.' });
  assert.equal((await call('council_inbox', { agent: 'claude' }))[0].id, message.id);
  assert.equal((await call('council_ack', { id: message.id, agent: 'junie' })).acknowledged, false);
  assert.equal((await call('council_inbox', { agent: 'claude' })).length, 1);
  assert.equal((await call('council_ack', { id: message.id, agent: 'claude' })).acknowledged, true);
  assert.deepEqual(await call('council_inbox', { agent: 'claude' }), []);
  assert.ok((await call('council_inbox', { agent: 'claude', unreadOnly: false }))[0].acknowledgedAt);
  assert.equal(await call('council_context', { key: 'design' }), null);
  await call('council_context', { key: 'design', value: 'Use a small local index.', author: 'codex' });
  assert.equal((await call('council_context', { key: 'design' })).value, 'Use a small local index.');
  await call('council_reserve', { owner: 'codex', paths: ['src/**'], ttlMs: 5000 });
  const conflict = await client.callTool({ name: 'council_reserve', arguments: { owner: 'claude', paths: ['src/main.ts'], ttlMs: 5000 } });
  assert.equal(conflict.isError, true);
  assert.match(conflict.content[0].text, /conflict/i);
  assert.equal((await call('council_release', { owner: 'codex' })).released, 1);
  assert.deepEqual(await call('council_leases'), []);
  const badPlan = await client.callTool({ name: 'council_plan', arguments: { ...workflow, maxCalls: 1 } });
  assert.equal(badPlan.isError, true);
  assert.match(badPlan.content[0].text, /maxCalls/);
  const missing = await client.callTool({ name: 'council_status', arguments: { id: '00000000-0000-4000-8000-000000000000' } });
  assert.equal(missing.isError, true);
  assert.equal((await call('council_agents')).backend, 'mock');
  assert.deepEqual(await call('council_runs'), []);
});

function abortableRunner(onStart = () => {}) {
  return {
    doctor: async () => [],
    run: request => new Promise(resolve => {
      const finish = () => { clearTimeout(timer); resolve({ agent: request.agent, status: 'cancelled', output: 'Partial offline work retained.', durationMs: 1 }); };
      const timer = setTimeout(() => resolve({ agent: request.agent, status: 'completed', output: 'Unexpected uncancelled completion.', durationMs: 30_000 }), 30_000);
      if (request.signal.aborted) finish();
      else request.signal.addEventListener('abort', finish, { once: true });
      onStart();
    }),
  };
}

test('a separate CLI process cancels an active service through durable SQLite events', { timeout: 15_000 }, async t => {
  const root = workspace(t);
  let notifyStart;
  const started = new Promise(resolve => { notifyStart = resolve; });
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), abortableRunner(notifyStart));
  t.councilCleanup.push(() => service.close());
  const queued = service.start({ ...workflow, mode: 'delegate', agents: ['codex'] });
  assert.equal(queued.status, 'queued');
  await started;
  const response = await execute(process.execPath, [cli, 'cancel', queued.id, '--workspace', root, '--backend', 'mock'], { windowsHide: true, timeout: 5000, env: { ...process.env, COUNCIL_DEPTH: '0' } });
  assert.equal(JSON.parse(response.stdout).cancellationRequested, true);
  const run = await service.wait(queued.id, 3000);
  assert.equal(run.status, 'cancelled');
  assert.match(run.output, /Partial offline work retained/);
  assert.ok(service.store.events(run.id).some(event => event.type === 'cancel_requested'));
});

test('service shutdown cancels active calls, persists terminal state and rejects new work', { timeout: 10_000 }, async t => {
  const root = workspace(t);
  let notifyStart;
  const started = new Promise(resolve => { notifyStart = resolve; });
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), abortableRunner(notifyStart));
  const queued = service.start({ ...workflow, mode: 'delegate', agents: ['codex'] });
  await started;
  await service.close();
  assert.throws(() => service.start(workflow), /shutting down/);
  const reopened = new CouncilStore(join(root, '.council'));
  assert.equal(reopened.getRun(queued.id).status, 'cancelled');
  reopened.close();
});

test('stdio host disconnect cancels active work before server process exits', { timeout: 15_000 }, async t => {
  const root = workspace(t);
  const entrypoint = join(root, 'offline-server.mjs');
  const moduleUrl = name => pathToFileURL(fileURLToPath(new URL(`../dist/${name}.js`, import.meta.url))).href;
  writeFileSync(entrypoint, `
import { CouncilService } from ${JSON.stringify(moduleUrl('service'))};
import { loadConfig } from ${JSON.stringify(moduleUrl('config'))};
import { serveMcp } from ${JSON.stringify(moduleUrl('mcp'))};
const runner = {
  doctor: async () => [],
  run: request => new Promise(resolve => {
    const timer = setTimeout(() => resolve({ agent: request.agent, status: 'completed', output: 'not cancelled', durationMs: 30000 }), 30000);
    request.signal.addEventListener('abort', () => {
      clearTimeout(timer);
      resolve({ agent: request.agent, status: 'cancelled', output: 'cancelled on EOF', durationMs: 1 });
    }, { once: true });
  }),
};
await serveMcp(new CouncilService(loadConfig(${JSON.stringify(root)}, { backend: 'mock' }), runner));
`);
  const { call, close } = await connect(t, root, entrypoint, false);
  const queued = await call('council_run', { ...workflow, mode: 'delegate', agents: ['codex'] });
  assert.equal((await call('council_status', { id: queued.id, waitMs: 0 })).status, 'running');
  await close();
  const reopened = new CouncilStore(join(root, '.council'));
  const run = reopened.getRun(queued.id);
  reopened.close();
  assert.equal(run.status, 'cancelled', 'Host disconnection must settle active work before the transport kills its process.');
  assert.match(run.output, /cancelled on EOF/);
});
