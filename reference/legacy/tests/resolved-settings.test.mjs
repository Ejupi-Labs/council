import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { NativeRunner, AcpxRunner } from '../dist/runner.js';

const workspace = fileURLToPath(new URL('..', import.meta.url));
const fixture = name => fileURLToPath(new URL(`./fixtures/${name}`, import.meta.url));
const request = (overrides = {}) => ({ agent: 'worker', prompt: 'args', cwd: workspace, permission: 'read', timeoutMs: 15000, ...overrides });

function serviceFixture(t, agents, run, backend = 'mock') {
  const root = mkdtempSync(join(tmpdir(), 'council-resolved-')), calls = [];
  const service = new CouncilService(loadConfig(root, { backend, maxCalls: 12, agents }), {
    doctor: async () => [], run: async request => { calls.push(request); return { agent: request.agent, status: 'completed', output: await run(request, calls), durationMs: 1 }; },
  });
  t.after(async () => {
    await service.close();
    assert.equal(dirname(resolve(root)), resolve(tmpdir())); assert.ok(basename(root).startsWith('council-resolved-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { service, calls };
}

test('admission preserves native settings and empty skills for every stage, judge and actual advisor call', async t => {
  const agents = Object.fromEntries(['author', 'peer', 'judge', 'advisor'].map(alias => [alias, { enabled: true, profile: 'codex' }]));
  const { service, calls } = serviceFixture(t, agents, async (request, calls) => request.agent === 'author' && calls.filter(call => call.agent === 'author').length === 1
    ? JSON.stringify({ council_advisor_request: { question: 'Check the offline evidence.', context: 'Complete fixture context.' } }) : 'Verified offline evidence.');
  const queued = service.start({ mode: 'council', task: 'Inspect the offline fixture.', agents: ['author', 'peer'], judge: 'judge', permission: 'read',
    advisors: { author: { agent: 'advisor' } } });
  for (const [alias, profile] of Object.entries(service.config.agents)) {
    assert.deepEqual(queued.input.agentSettings[alias], { skills: [] });
    profile.model = `later-${alias}`; profile.effort = 'high'; profile.skills = ['council-advisor'];
  }
  const completed = await service.completion(queued.id);
  assert.equal(completed.status, 'completed', completed.error); assert.equal(calls.length, 7);
  assert.ok(calls.some(call => call.agent === 'advisor'));
  for (const call of calls) {
    assert.equal(call.resolvedSettings, true); assert.equal(call.model, undefined); assert.equal(call.effort, undefined);
    assert.equal(call.prompt.includes('Selected collaboration skills'), false);
  }
  assert.deepEqual(service.getRun(queued.id).input.agentSettings, queued.input.agentSettings);
});

test('resolved planning does not reject an admitted native effort because a later profile default is unsupported', async t => {
  const { service, calls } = serviceFixture(t, { worker: { enabled: true, profile: 'claude' } }, async () => 'Verified offline.', 'native');
  const queued = service.start({ mode: 'delegate', task: 'Offline fixture only.', agents: ['worker'], permission: 'read' });
  service.config.agents.worker.effort = 'xhigh';
  assert.equal((await service.completion(queued.id)).status, 'completed'); assert.equal(calls[0].effort, undefined);
  assert.throws(() => service.start({ mode: 'delegate', task: 'A new admission validates current defaults.', agents: ['worker'], permission: 'read' }), /unsupported/);
  assert.equal(calls.length, 1);
});

function runnerConfig(profile = 'codex', transport = 'native') {
  return { version: 1, backend: 'native', workspaceRoot: workspace, stateDir: join(workspace, '.council'),
    maxConcurrency: 1, timeoutMs: 15000, maxCalls: 8, maxOutputBytes: 16384,
    agents: { worker: { enabled: true, profile, transport, command: process.execPath,
      args: [fixture(transport === 'acp' ? 'fake-effort-acp.mjs' : 'fake-native.mjs')] } } };
}

test('fake native argv preserves resolved native defaults while direct runner calls still inherit new config defaults', async () => {
  const config = runnerConfig(), runner = new NativeRunner(config);
  const resolved = request({ resolvedSettings: true });
  config.agents.worker.model = 'later-configured-model'; config.agents.worker.effort = 'high';
  const native = await runner.run(resolved), inherited = await runner.run(request());
  assert.equal(native.status, 'completed', native.error); assert.equal(inherited.status, 'completed', inherited.error);
  const nativeArgs = JSON.parse(native.output), inheritedArgs = JSON.parse(inherited.output);
  assert.equal(nativeArgs.includes('--model'), false); assert.equal(nativeArgs.includes('-c'), false);
  assert.equal(inheritedArgs[inheritedArgs.indexOf('--model') + 1], 'later-configured-model');
  assert.ok(inheritedArgs.includes('model_reasoning_effort="high"'));
  const explicit = await runner.run(request({ resolvedSettings: true, model: 'exact-worker-model', effort: 'low' }));
  assert.equal(explicit.status, 'completed', explicit.error);
  assert.equal(JSON.parse(explicit.output)[JSON.parse(explicit.output).indexOf('--model') + 1], 'exact-worker-model');
  assert.ok(JSON.parse(explicit.output).includes('model_reasoning_effort="low"'));
});

test('resolved native requests ignore later unsupported effort defaults without weakening ordinary validation', async () => {
  const config = runnerConfig('claude'); config.agents.worker.effort = 'xhigh';
  const runner = new NativeRunner(config), native = await runner.run(request({ resolvedSettings: true }));
  assert.equal(native.status, 'completed', native.error); assert.equal(JSON.parse(native.output).includes('--effort'), false);
  const inherited = await runner.run(request());
  assert.equal(inherited.status, 'failed'); assert.match(inherited.error, /unsupported/);
});

test('resolved ACP requests avoid both new model negotiation and effort defaults', async () => {
  const config = runnerConfig('junie', 'acp'), runner = new NativeRunner(config);
  config.agents.worker.model = 'later-unadvertised-model'; config.agents.worker.effort = 'high';
  const native = await runner.run(request({ resolvedSettings: true }));
  assert.equal(native.status, 'completed', native.error); assert.deepEqual(JSON.parse(native.output), []);
  assert.equal((await runner.run(request())).status, 'failed', 'Ordinary callers still negotiate their configured model.');
});

test('resolved ACPX requests avoid new model and unsupported effort defaults', async () => {
  const config = { ...runnerConfig(), backend: 'acpx' }, runner = new AcpxRunner(config, { acpxEntrypoint: fixture('fake-acpx.mjs') });
  config.agents.worker.model = 'later-model'; config.agents.worker.effort = 'high';
  const native = await runner.run(request({ resolvedSettings: true }));
  assert.equal(native.status, 'completed', native.error); assert.equal(JSON.parse(native.output).includes('--model'), false);
  const inherited = await runner.run(request());
  assert.equal(inherited.status, 'failed'); assert.match(inherited.error, /unsupported.*ACPX/);
  delete config.agents.worker.effort;
  const ordinary = await runner.run(request());
  assert.equal(ordinary.status, 'completed', ordinary.error);
  const args = JSON.parse(ordinary.output); assert.equal(args[args.indexOf('--model') + 1], 'later-model');
});
