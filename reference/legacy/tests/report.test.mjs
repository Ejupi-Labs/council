import test from 'node:test';
import assert from 'node:assert/strict';
import { renderRun } from '../dist/report.js';

test('exported peer text cannot close its report code fence, including maximum-sized repeated delimiters', () => {
  const output = '` x '.repeat(400_000) + '\n``````\n# injected heading';
  const run = { id: 'report-example', status: 'failed', createdAt: '2026-11-03T00:00:00Z', updatedAt: '2026-11-03T00:00:01Z', input: { mode: 'delegate', agents: ['codex'], task: 'Review evidence.', permission: 'read' }, steps: [], output, error: 'Unverified result.' };
  const report = renderRun(run);
  assert.ok(report.includes('```````text\n' + output + '\n```````'));
  assert.match(report, /Status: failed/);
  assert.match(report, /Review evidence and patches/);
});

test('reports retain every failed peer diagnostic alongside partial output with safe fences', () => {
  const error = 'SECOND_FAILURE_DETAIL\n````\n# untrusted error heading';
  const run = {
    id: 'report-failed-peers', status: 'failed', createdAt: '2026-11-03T00:00:00Z', updatedAt: '2026-11-03T00:00:01Z',
    input: { mode: 'parallel', agents: ['codex', 'claude'], task: 'Inspect offline evidence.', permission: 'read' },
    output: 'Bounded selected evidence', error: 'Multiple peers failed.',
    steps: [
      { id: '01-worker-codex', agent: 'codex', role: 'worker', result: { agent: 'codex', status: 'failed', output: 'first partial evidence', error: 'FIRST_FAILURE_DETAIL', durationMs: 1 } },
      { id: '02-worker-claude', agent: 'claude', role: 'worker', result: { agent: 'claude', status: 'failed', output: 'second partial evidence', error, durationMs: 1 } },
    ],
  };
  const report = renderRun(run);
  assert.ok(report.includes('first partial evidence')); assert.ok(report.includes('second partial evidence'));
  assert.ok(report.includes('FIRST_FAILURE_DETAIL')); assert.ok(report.includes('`````text\n' + error + '\n`````'));
  assert.equal(report.split('Reported error:').length - 1, 2);
});

test('a step with only an error prints that diagnostic once without an empty output placeholder', () => {
  const run = {
    id: 'report-error-only', status: 'failed', createdAt: '2026-11-03T00:00:00Z', updatedAt: '2026-11-03T00:00:01Z',
    input: { mode: 'delegate', agents: ['codex'], task: 'Inspect offline evidence.', permission: 'read' }, output: 'Selected result',
    steps: [{ id: '01-delegate-codex', agent: 'codex', role: 'delegate', result: { agent: 'codex', status: 'failed', output: '', error: 'ONLY_STEP_FAILURE', durationMs: 1 } }],
  };
  const report = renderRun(run);
  assert.equal(report.split('ONLY_STEP_FAILURE').length - 1, 1); assert.ok(!report.includes('No output.'));
});
