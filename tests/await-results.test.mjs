import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

function fixture(t, overrides = {}) {
  const root = mkdtempSync(join(tmpdir(), 'council-wait-results-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock', ...overrides }));
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  return service;
}

test('selected-result waits retain all step metadata beyond status-preview limits without duplicating step output', async t => {
  const agents = Object.fromEntries(Array.from({ length: 70 }, (_, index) => [`peer${index}`, { profile: 'codex' }]));
  const service = fixture(t, { maxCalls: 70, agents });
  const run = service.start({ mode: 'parallel', agents: Object.keys(agents), task: 'Inspect offline evidence.' });
  const result = await service.awaitResults([run.id]);
  assert.equal(result.timedOut, false); assert.equal(result.runs[0].status, 'completed');
  assert.equal(result.runs[0].steps.length, 70); assert.equal(result.runs[0].outputTruncated, false);
  assert.ok(result.runs[0].steps.every(step => step.id && step.status === 'completed' && step.durationMs >= 0 && !Object.hasOwn(step, 'output')));
  assert.equal(result.runs[0].output, service.getRun(run.id).output);
});

test('selected-result waits never combine an older running summary with completed output', async t => {
  const service = fixture(t), id = randomUUID(), stamp = new Date().toISOString();
  service.store.saveRun({ id, status: 'completed', createdAt: stamp, updatedAt: stamp,
    input: { mode: 'delegate', agents: ['codex'], permission: 'read', task: 'Completed elsewhere.' },
    output: 'COMPLETE_FOREIGN_OUTPUT', steps: [{ id: 'last-step', agent: 'codex', role: 'delegate',
      result: { agent: 'codex', status: 'completed', durationMs: 7, output: 'COMPLETE_FOREIGN_OUTPUT' } }] });
  service.awaitRuns = async () => ({ runs: [{ id, status: 'running', steps: [] }], timedOut: true, waitingInServer: true });
  const result = await service.awaitResults([id], 0);
  assert.equal(result.timedOut, false); assert.equal(result.runs[0].status, 'completed');
  assert.equal(result.runs[0].output, 'COMPLETE_FOREIGN_OUTPUT'); assert.equal(result.runs[0].steps[0].id, 'last-step');
});

test('results pruned between waiting and reading are explicitly unavailable', async t => {
  const service = fixture(t), id = randomUUID();
  service.awaitRuns = async () => ({ runs: [{ id, status: 'completed', steps: [] }], timedOut: false, waitingInServer: true });
  await assert.rejects(service.awaitResults([id]), /results are no longer available.*pruned/i);
});

test('complete large peer messages remain retrievable alone through the recommended inbox page API', t => {
  const service = fixture(t);
  const body = 'Complete peer evidence. '.repeat(55_000) + 'LARGE_HANDOFF_TAIL';
  service.store.sendMessage({ from: 'PM', to: 'junie', subject: 'Large complete handoff', body });
  service.store.sendMessage({ from: 'PM', to: 'junie', subject: 'Next handoff', body: 'Next complete evidence.' });
  const messages = [];
  let cursor;
  do {
    const page = service.inboxPage('junie', true, 20, cursor);
    if (page.messages.some(message => message.subject === 'Large complete handoff')) assert.equal(page.messages.length, 1);
    messages.push(...page.messages); cursor = page.nextCursor;
  } while (cursor);
  assert.equal(messages.length, 2);
  assert.equal(messages.find(message => message.subject === 'Large complete handoff').body, body);
  assert.equal(messages.find(message => message.subject === 'Next handoff').body, 'Next complete evidence.');
});
