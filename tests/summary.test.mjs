import assert from 'node:assert/strict';
import test from 'node:test';
import { MAX_COMPACT_EVENT_DATA_BYTES, MAX_RUN_SUMMARY_BYTES, summarizeEvent, summarizeRun } from '../dist/summary.js';

function run() {
  return { id: '00000000-0000-4000-8000-000000000001', status: 'failed', createdAt: '2026-01-01T00:00:00.000Z', updatedAt: '2026-01-01T00:00:01.000Z',
    input: { mode: 'parallel', agents: ['codex', 'claude'], task: 'Inspect the implementation.', permission: 'read' }, steps: [], output: 'Evidence.', error: 'One participant failed.' };
}

test('ordinary summaries retain their existing fields and preview behavior', () => {
  const source = { ...run(), output: 'x'.repeat(20000) };
  const summary = summarizeRun(source, 7);
  assert.equal(summary.revision, 7);
  assert.equal(summary.output.length, 16000);
  assert.equal(summary.outputTruncated, true);
  assert.equal(summary.summaryTruncated, undefined);
  assert.equal(summary.workspacesTruncated, undefined);
  assert.equal(summary.status, source.status);
  assert.equal(summary.fullResultResource, `council://runs/${source.id}`);
});

test('maximum Unicode and control-character summaries fit 64 KiB without redirecting retained artifact identities', () => {
  const source = run();
  source.output = '\u0001🦉'.repeat(16000);
  source.error = '\u0002'.repeat(8000);
  source.steps = Array.from({ length: 64 }, (_, index) => ({ id: `${index}-${'\u0003'.repeat(256)}`, agent: `agent-${index}`, role: '\u0004'.repeat(128), result: { agent: 'codex', status: 'failed', durationMs: 1, output: 'full peer evidence', error: '\u0005🦉'.repeat(2048) } }));
  source.workspaces = Array.from({ length: 8 }, (_, index) => ({ agent: `agent-${index}`, path: `/managed/${index}/${'日本語\u0006'.repeat(1000)}`, patchPath: `/patches/${index}/${'🦉\u0007'.repeat(1000)}.patch`, baseCommit: 'a'.repeat(40), changed: true, exportStatus: 'failed', exportError: '\u0008🦉'.repeat(4096) }));
  const original = structuredClone(source);
  const summary = summarizeRun(source);
  assert.ok(Buffer.byteLength(JSON.stringify(summary), 'utf8') <= MAX_RUN_SUMMARY_BYTES);
  assert.equal(summary.id, source.id);
  assert.equal(summary.status, source.status);
  assert.equal(summary.fullResultResource, `council://runs/${source.id}`);
  assert.equal(summary.summaryTruncated, true);
  assert.equal(summary.workspacesTruncated, true);
  for (const artifact of summary.workspaces ?? []) {
    const originalArtifact = source.workspaces.find(value => value.path === artifact.path);
    assert.ok(originalArtifact, 'Retained paths must be complete original paths.');
    for (const key of ['agent', 'path', 'patchPath', 'baseCommit']) assert.equal(artifact[key], originalArtifact[key]);
    assert.ok((artifact.exportError?.length ?? 0) <= 1024);
  }
  for (const step of summary.steps) assert.ok(source.steps.some(value => value.id === step.id), 'Retained step IDs must remain exact.');
  assert.deepEqual(source, original, 'Summary compaction must never mutate the complete result.');
});

test('an oversized individual identity is omitted as a whole entry', () => {
  const source = { ...run(), workspaces: [{ agent: 'codex', path: `/managed/${'x'.repeat(100000)}`, patchPath: '/managed/result.patch', baseCommit: 'b'.repeat(40), changed: true, exportStatus: 'ready' }] };
  const summary = summarizeRun(source);
  assert.deepEqual(summary.workspaces, []);
  assert.equal(summary.workspacesTruncated, true);
  assert.ok(Buffer.byteLength(JSON.stringify(summary)) <= MAX_RUN_SUMMARY_BYTES);
  assert.equal(source.workspaces[0].path.length, 100009);
});

test('eight-KiB event data keeps a maximum thousand-event MCP response below the transport response cap', () => {
  const source = { sequence: 1, runId: '\u0001'.repeat(256), type: '\u0002'.repeat(128), createdAt: '2026-01-01T00:00:00.000Z', data: Object.fromEntries(Array.from({ length: 4 }, (_, index) => [`text${index}`, '"'.repeat(1000)])) };
  const compact = summarizeEvent(source);
  assert.ok(Buffer.byteLength(JSON.stringify(compact.data)) <= MAX_COMPACT_EVENT_DATA_BYTES);
  const events = Array.from({ length: 1000 }, (_, index) => ({ ...compact, sequence: index + 1 }));
  const response = { jsonrpc: '2.0', id: 1, result: { content: [{ type: 'text', text: JSON.stringify(events) }], structuredContent: { value: events } } };
  assert.ok(Buffer.byteLength(JSON.stringify(response)) < 32 * 1024 * 1024);
  const oversized = summarizeEvent({ ...source, data: Object.fromEntries(Array.from({ length: 4 }, (_, index) => [`control${index}`, '\u0001'.repeat(2000)])) });
  assert.ok(Buffer.byteLength(JSON.stringify(oversized.data)) <= MAX_COMPACT_EVENT_DATA_BYTES);
  assert.equal(oversized.data.dataTruncated, true);
  assert.equal(source.data.text0.length, 1000);
});
