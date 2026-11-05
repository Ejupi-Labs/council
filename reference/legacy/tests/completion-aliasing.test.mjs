import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

function deferred() { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; }
function fixture(t, handler) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-completion-copy-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), { doctor: async () => [], run: handler });
  t.after(async () => {
    await service.close(); assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-completion-copy-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return service;
}
const input = () => ({ mode: 'delegate', agents: ['codex'], task: 'Retain exact independent completion evidence.', permission: 'read',
  agentSettings: { codex: { model: 'offline-model', skills: ['council-advisor'] } } });

test('concurrent normal completion readers receive independent full records and nested evidence', async t => {
  const entered = deferred(), finish = deferred();
  const service = fixture(t, async request => { entered.resolve(); await finish.promise;
    return { agent: request.agent, status: 'completed', output: 'FULL_ORIGINAL_EVIDENCE '.repeat(1000), durationMs: 1 }; });
  const queued = service.start(input()); await entered.promise;
  const firstWaiting = service.completion(queued.id), secondWaiting = service.completion(queued.id), thirdWaiting = service.completion(queued.id);
  finish.resolve(); const [first, second, third] = await Promise.all([firstWaiting, secondWaiting, thirdWaiting]);
  const durable = service.getRun(queued.id);
  assert.notEqual(first, second); assert.notEqual(second, third); assert.deepEqual(first, durable);
  first.status = 'failed'; first.output = 'Caller forged output.'; first.input.task = 'Caller changed task.';
  first.input.agentSettings.codex.skills.push('council-independent-perspectives');
  first.steps[0].result.output = 'Caller forged step evidence.'; first.workspaces.push({ agent: 'forged', path: 'caller path', patchPath: 'caller patch', changed: true });
  assert.deepEqual(second, durable); assert.deepEqual(third, durable); assert.deepEqual(service.getRun(queued.id), durable);
  assert.deepEqual(await service.completion(queued.id), durable);
});

test('an earlier completion consumer cannot rewrite evidence seen by a later waiter during resolution', async t => {
  const entered = deferred(), finish = deferred();
  const service = fixture(t, async request => { entered.resolve(); await finish.promise;
    return { agent: request.agent, status: 'failed', output: 'Retained partial failure evidence.', error: 'Offline provider failure.', durationMs: 1 }; });
  const queued = service.start(input()); await entered.promise;
  const first = service.completion(queued.id).then(result => {
    result.status = 'completed'; result.output = 'Forged successful result.'; result.steps.length = 0; delete result.error;
    return result;
  });
  const second = service.completion(queued.id); finish.resolve();
  const [mutated, untouched] = await Promise.all([first, second]);
  assert.equal(mutated.status, 'completed'); assert.equal(untouched.status, 'failed'); assert.equal(untouched.steps.length, 1);
  assert.equal(untouched.steps[0].result.output, 'Retained partial failure evidence.'); assert.match(untouched.error, /Offline provider failure/);
  assert.deepEqual(untouched, service.getRun(queued.id));
});

test('simultaneous cancellation completion readers cannot turn one another into success', async t => {
  const entered = deferred();
  const service = fixture(t, request => new Promise(resolve => {
    const finish = () => resolve({ agent: request.agent, status: 'cancelled', output: 'Retained cancelled evidence.', durationMs: 1 });
    if (request.signal.aborted) finish(); else request.signal.addEventListener('abort', finish, { once: true }); entered.resolve();
  }));
  const queued = service.start(input()); await entered.promise;
  const one = service.completion(queued.id), two = service.completion(queued.id);
  assert.equal(service.requestCancellation(queued.id).requested, true);
  const [first, second] = await Promise.all([one, two]);
  assert.equal(first.status, 'cancelled'); first.status = 'completed'; first.steps[0].result.output = 'Forged cancellation evidence.';
  assert.equal(second.status, 'cancelled'); assert.equal(second.steps[0].result.output, 'Retained cancelled evidence.');
  assert.deepEqual(second, service.getRun(queued.id));
});
