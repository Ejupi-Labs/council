import test from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { NativeRunner, AcpxRunner } from '../dist/runner.js';
import { WorkflowEngine } from '../dist/engine.js';
import { configSchema, parseWorkflow } from '../dist/config.js';

const root = fileURLToPath(new URL('..', import.meta.url));
const fixture = name => fileURLToPath(new URL(`./fixtures/${name}`, import.meta.url));
const config = (profile = 'codex', overrides = {}) => ({ version: 1, backend: 'native', workspaceRoot: root, stateDir: join(root, '.council'),
  maxConcurrency: 1, timeoutMs: 15000, maxCalls: 8, maxOutputBytes: 16384,
  agents: { peer: { enabled: true, profile, transport: 'native', command: process.execPath, args: [fixture(profile === 'antigravity' ? 'fake-antigravity.mjs' : 'fake-native.mjs')] } }, ...overrides });
const request = effort => ({ agent: 'peer', prompt: 'args', cwd: root, permission: 'read', timeoutMs: 15000, effort });

test('schemas preserve exact alias settings, explicit effort, authority and advisor limits', () => {
  const parsed = configSchema.parse({ agents: { cheap: { profile: 'codex', effort: 'low', authority: 0.1 }, deep: { profile: 'claude', effort: 'max', authority: 10 } } });
  assert.equal(parsed.agents.cheap.authority, 0.1); assert.equal(parsed.agents.deep.effort, 'max');
  assert.equal(configSchema.parse({ agents: { peer: { profile: 'codex', authority: 10.1 } } }).agents.peer.authority, 10.1);
  for (const authority of [0, -1, Infinity, 'high', NaN]) assert.throws(() => configSchema.parse({ agents: { peer: { profile: 'codex', authority } } }));
  for (const effort of ['none', '', 3]) assert.throws(() => configSchema.parse({ agents: { peer: { profile: 'codex', effort } } }));
  const input = parseWorkflow({ mode: 'delegate', task: 'A task.', agents: ['cheap'], agentSettings: { cheap: { effort: 'low' }, deep: { model: 'explicit-deep-model', effort: 'max' } }, advisors: { cheap: { agent: 'deep' } }, contextLimit: 1000 });
  assert.equal(input.advisors.cheap.maxRequests, 1); assert.equal(input.agentSettings.deep.model, 'explicit-deep-model');
  assert.equal(parseWorkflow({ mode: 'delegate', task: 'A task.', agents: ['cheap'], advisors: { cheap: { agent: 'deep', maxRequests: 3 } } }).advisors.cheap.maxRequests, 3);
  for (const contextLimit of [null, 1, 999, 64001]) assert.equal(parseWorkflow({ mode: 'delegate', task: 'A task.', agents: ['cheap'], contextLimit }).contextLimit, contextLimit);
  for (const override of [{ contextLimit: 0 }, { contextLimit: -1 }, { advisors: { cheap: { agent: 'deep', maxRequests: 0 } } }, { agentSettings: { cheap: { effort: 'extreme' } } }]) {
    assert.throws(() => parseWorkflow({ mode: 'delegate', task: 'A task.', agents: ['cheap'], ...override }));
  }
});

for (const effort of ['low', 'medium', 'high', 'xhigh', 'max']) test(`Codex native receives explicit ${effort} as one quoted config argument`, async () => {
  const result = await new NativeRunner(config()).run(request(effort));
  assert.equal(result.status, 'completed', result.error);
  const args = JSON.parse(result.output);
  assert.equal(args[args.indexOf('-c') + 1], `model_reasoning_effort=${JSON.stringify(effort)}`);
});

for (const [profile, efforts] of [['claude', ['low', 'medium', 'high', 'max']], ['antigravity', ['low', 'medium', 'high', 'xhigh', 'max']]]) {
  for (const effort of efforts) test(`${profile} native receives explicit ${effort} without model substitution`, async () => {
    const settings = config(profile); settings.agents.peer.model = 'explicit-fixture-model';
    const result = await new NativeRunner(settings).run(request(effort));
    assert.equal(result.status, 'completed', result.error);
    const args = JSON.parse(result.output);
    assert.equal(args[args.indexOf('--effort') + 1], effort);
    assert.equal(args[args.indexOf('--model') + 1], 'explicit-fixture-model');
  });
}

for (const effort of ['low', 'medium', 'high']) test(`Junie ACP receives its supported ${effort} CLI setting`, async () => {
  const settings = config('junie'); settings.agents.peer = { ...settings.agents.peer, transport: 'acp', args: [fixture('fake-effort-acp.mjs')], effort: 'low' };
  const result = await new NativeRunner(settings).run(request(effort));
  assert.equal(result.status, 'completed', result.error);
  assert.deepEqual(JSON.parse(result.output), ['--effort', effort]);
  assert.equal(settings.agents.peer.effort, 'low');
});

test('unsupported effort and transports fail before executable lookup and are rejected during planning', async () => {
  for (const [profile, transport, effort] of [['claude', 'native', 'xhigh'], ['junie', 'acp', 'max'], ['codex', 'acp', 'high'], ['antigravity', 'acp', 'high']]) {
    const settings = config(profile); settings.agents.peer = { enabled: true, profile, transport, command: join(root, 'must-not-launch.exe'), effort };
    const result = await new NativeRunner(settings).run(request(undefined));
    assert.equal(result.status, 'failed'); assert.equal(result.durationMs, 0); assert.match(result.error, /unsupported/);
    const engine = new WorkflowEngine(settings, { doctor: async () => [], run: async () => { throw Error('Must not dispatch.'); } }, { saveRun() {}, appendEvent() {} });
    assert.throws(() => engine.plan({ mode: 'delegate', task: 'Offline only.', agents: ['peer'], permission: 'read' }), /unsupported/);
  }
  const external = new AcpxRunner({ ...config(), backend: 'acpx' }, { acpxEntrypoint: join(root, 'must-not-launch.mjs') });
  const result = await external.run(request('high'));
  assert.equal(result.status, 'failed'); assert.equal(result.durationMs, 0); assert.match(result.error, /unsupported.*ACPX/);
});

test('request effort overrides profile defaults without mutating shared configuration', async () => {
  const settings = config(); settings.agents.peer.effort = 'low';
  const runner = new NativeRunner(settings);
  const explicit = await runner.run(request('max'));
  const inherited = await runner.run(request(undefined));
  assert.equal(explicit.status, 'completed', explicit.error); assert.equal(inherited.status, 'completed', inherited.error);
  assert.ok(JSON.parse(explicit.output).includes('model_reasoning_effort="max"'));
  assert.ok(JSON.parse(inherited.output).includes('model_reasoning_effort="low"'));
  assert.equal(settings.agents.peer.effort, 'low');
});
