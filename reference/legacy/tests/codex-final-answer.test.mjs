import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { NativeRunner } from '../dist/runner.js';
import { WorkflowEngine } from '../dist/engine.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

const workspace = fileURLToPath(new URL('..', import.meta.url));
const fake = fileURLToPath(new URL('./fixtures/fake-codex-final.mjs', import.meta.url));
const agent = role => ({ enabled: true, profile: 'codex', transport: 'native', command: process.execPath, args: [fake, role] });
const configuration = () => ({ version: 1, backend: 'native', workspaceRoot: workspace, stateDir: join(workspace, '.council'),
  maxConcurrency: 2, timeoutMs: 15000, maxCalls: 8, maxOutputBytes: 16384, agents: { worker: agent('worker'), advisor: agent('advisor') } });
const request = prompt => ({ agent: 'worker', prompt, cwd: workspace, permission: 'read', timeoutMs: 15000 });

test('Codex selects its last completed message without prefixing progress narration to the final JSON', async () => {
  const result = await new NativeRunner(configuration()).run(request('Return the final offline JSON.'));
  assert.equal(result.status, 'completed', result.error);
  assert.deepEqual(JSON.parse(result.output), { result: 'Complete offline final answer.' });
  assert.equal(result.output.includes('I am inspecting'), false);
});

test('an empty last Codex message fails instead of falling back to an earlier nonempty preamble', async () => {
  const result = await new NativeRunner(configuration()).run(request('empty-final'));
  assert.equal(result.status, 'failed'); assert.match(result.error, /empty answer/); assert.equal(result.output, '');
});

test('Codex preambles cannot turn an advisor request into a completed one-call workflow', async () => {
  const config = configuration(), native = new NativeRunner(config), calls = [];
  const runner = { doctor: async () => [], run: async request => { calls.push(request); return native.run(request); } };
  const engine = new WorkflowEngine(config, runner, { saveRun() {}, appendEvent() {} });
  const result = await engine.run({ mode: 'delegate', task: 'CODEX_ADVISOR_FINAL_FIXTURE: verify the offline task.', agents: ['worker'], permission: 'read',
    advisors: { worker: { agent: 'advisor' } }, maxCalls: 3 });
  assert.equal(result.status, 'completed', result.error); assert.deepEqual(calls.map(call => call.agent), ['worker', 'advisor', 'worker']);
  assert.equal(result.steps.length, 3); assert.equal(calls[1].permission, 'read');
  assert.ok(calls[2].prompt.includes('The independent offline evidence supports completing the task.'));
  assert.equal(result.output, 'Worker verified the task after assessing the advisor evidence.');
  assert.equal(result.steps.some(step => step.result.output.includes('I am inspecting')), false);
});

test('room discussion accepts complete Codex JSON ballots following progress messages without dispatching tasks', async t => {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-codex-final-'));
  const config = loadConfig(root, { backend: 'native', agents: { worker: agent('worker'), peer: agent('peer') } });
  const native = new NativeRunner(config), calls = [];
  const service = new CouncilService(config, { doctor: async () => [], run: async request => { calls.push(request); return native.run(request); } });
  t.after(async () => {
    await service.close(); assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-codex-final-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const room = service.rooms.create({ title: 'Codex final ballot', brief: 'Inspect complete offline evidence.',
    members: [{ agent: 'worker', role: 'lead' }, { agent: 'peer', role: 'worker' }],
    tasks: [{ title: 'Later implementation', brief: 'Execute only after explicit approval.', permission: 'read' }], autoExecute: false, maxCalls: 2 });
  service.rooms.startDiscussion(room.id); const ready = await service.rooms.wait(room.id);
  assert.equal(ready.status, 'ready', ready.error); assert.equal(ready.decision.approved, true);
  assert.equal(ready.callsUsed, 2); assert.equal(calls.length, 2); assert.equal(ready.tasks[0].runId, undefined);
  assert.equal(ready.messages.filter(message => message.kind === 'opinion').length, 2);
});
