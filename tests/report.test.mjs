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
