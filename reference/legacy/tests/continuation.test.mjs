import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-continuation-'));
  const requests = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), {
    doctor: async () => [],
    run: async request => {
      requests.push(request);
      return { agent: request.agent, status: 'completed', output: 'Follow-up complete.', durationMs: 1 };
    },
  });
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  return { service, requests };
}

test('continuation preserves full task, caller context, prior context and all peer evidence', async t => {
  const { service, requests } = fixture(t);
  const stamp = new Date().toISOString(), id = randomUUID();
  const selected = 'x'.repeat(30_000) + 'SELECTED_RESULT_TAIL';
  service.store.saveRun({ id, createdAt: stamp, updatedAt: stamp, status: 'failed',
    input: { mode: 'pipeline', agents: ['codex', 'junie'], permission: 'read',
      task: 't'.repeat(5000) + 'PREVIOUS_TASK_TAIL', context: 'p'.repeat(70_000) + 'PREVIOUS_CONTEXT_TAIL',
      assignments: { codex: 'ASSIGNMENT_EVIDENCE' } },
    output: selected, error: 'Aggregate failure with retained peer evidence.',
    steps: [
      { id: 'advisor', agent: 'claude', role: 'advisor', result: { agent: 'claude', status: 'completed', output: 'ADVISOR_EVIDENCE', durationMs: 1 } },
      { id: 'partial', agent: 'junie', role: 'pipeline-2', result: { agent: 'junie', status: 'failed', output: 'PARTIAL_EVIDENCE', error: 'SPECIFIC_PEER_ERROR', durationMs: 1 } },
      { id: 'selected', agent: 'codex', role: 'pipeline-1', result: { agent: 'codex', status: 'completed', output: selected, durationMs: 1 } },
    ],
    workspaces: [{ agent: 'codex', path: '/retained/work', patchPath: '/retained/work.patch', changed: true,
      baseCommit: 'BASE_COMMIT_EVIDENCE', exportStatus: 'failed', exportError: 'e'.repeat(2000) + 'EXPORT_ERROR_TAIL' }],
  });
  const next = service.continue(id, { mode: 'delegate', agents: ['junie'], task: 'Reconcile the retained evidence.',
    context: 'c'.repeat(40_000) + 'CALLER_CONTEXT_TAIL' });
  assert.equal((await service.completion(next.id)).status, 'completed');
  assert.equal(requests.length, 1);
  for (const marker of ['PREVIOUS_TASK_TAIL', 'PREVIOUS_CONTEXT_TAIL', 'CALLER_CONTEXT_TAIL', 'SELECTED_RESULT_TAIL',
    'ASSIGNMENT_EVIDENCE', 'ADVISOR_EVIDENCE', 'PARTIAL_EVIDENCE', 'SPECIFIC_PEER_ERROR', 'BASE_COMMIT_EVIDENCE', 'EXPORT_ERROR_TAIL']) {
    assert.ok(requests[0].prompt.includes(marker), `Missing ${marker}`);
  }
  assert.equal(requests[0].prompt.split('SELECTED_RESULT_TAIL').length - 1, 1, 'selected step output is not duplicated');
});

test('an explicit continuation context budget rejects complete evidence rather than clipping it', async t => {
  const { service, requests } = fixture(t);
  const stamp = new Date().toISOString(), id = randomUUID();
  service.store.saveRun({ id, createdAt: stamp, updatedAt: stamp, status: 'completed', steps: [], output: 'e'.repeat(5000),
    input: { mode: 'delegate', agents: ['codex'], permission: 'read', task: 'Prior task' } });
  const next = service.continue(id, { mode: 'delegate', agents: ['junie'], task: 'Follow up', contextLimit: 1000 });
  const final = await service.completion(next.id);
  assert.equal(final.status, 'failed');
  assert.match(final.error, /context.*budget|context.*limit/i);
  assert.equal(requests.length, 0);
});

test('continuation deduplicates identical project context and exact parallel aggregates without losing advisor evidence', async t => {
  const { service, requests } = fixture(t);
  const stamp = new Date().toISOString(), id = randomUUID();
  const step = (id, agent, role, output) => ({ id, agent, role, result: { agent, status: 'completed', output, durationMs: 1 } });
  service.store.saveRun({ id, createdAt: stamp, updatedAt: stamp, status: 'completed',
    input: { mode: 'parallel', agents: ['codex', 'junie'], permission: 'read', task: 'Compare the evidence.', context: 'IDENTICAL_PROJECT_CONTEXT' },
    output: '[codex]\nCODEX_SELECTED_FACT\n\n[junie]\nJUNIE_SELECTED_FACT',
    // Completion order can differ from the selected result's agent order.
    steps: [step('junie-final', 'junie', 'worker', 'JUNIE_SELECTED_FACT'),
      step('advisor-answer', 'claude', 'advisor-for-codex', 'ADVISOR_ONLY_FACT'),
      step('codex-final', 'codex', 'worker', 'CODEX_SELECTED_FACT')],
  });
  const next = service.continue(id, { mode: 'delegate', agents: ['junie'], task: 'Follow up.', context: 'IDENTICAL_PROJECT_CONTEXT' });
  assert.equal((await service.completion(next.id)).status, 'completed');
  for (const marker of ['IDENTICAL_PROJECT_CONTEXT', 'CODEX_SELECTED_FACT', 'JUNIE_SELECTED_FACT', 'ADVISOR_ONLY_FACT']) {
    assert.equal(requests[0].prompt.split(marker).length - 1, 1, `${marker} is preserved once`);
  }
  assert.match(requests[0].prompt, /codex-final, junie-final/);
});
