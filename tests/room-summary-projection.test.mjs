import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { RoomStore } from '../dist/room-store.js';

test('room summaries project exact bounded metadata without decoding full briefs or task evidence', t => {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-summary-projection-'));
  const store = new RoomStore(root);
  t.after(() => {
    store.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-summary-projection-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const body = 'DO_NOT_DECODE_THE_FULL_ROOM_BRIEF ' + 'retained task evidence 🧭 '.repeat(20000);
  const states = ['completed', 'failed', 'pending', 'completed'];
  const records = Array.from({ length: 4 }, (_, index) => ({ id: 'room-' + index, backend: index % 2 ? 'mock' : undefined,
    title: 'Exact metadata ' + index, brief: body, status: index === 3 ? 'completed' : 'idle', members: [{ agent: 'codex', role: 'worker', weight: 1 }],
    tasks: states.map((status, task) => ({ id: 'task-' + task, title: 'Task ' + task, brief: body, permission: 'read', status,
      ...(status === 'completed' ? { runId: 'retained-' + task, resultPreview: body, workspaces: [] } : {}) })),
    messages: [], rounds: 1, maxCalls: 32, autoExecute: false, contextLimit: null, callsUsed: index * 3, revision: index + 1,
    createdAt: '2026-11-04T00:00:00.000Z', updatedAt: `2026-11-04T00:00:0${index}.000Z` }));
  for (const record of records) store.insert(record);
  const parse = JSON.parse;
  JSON.parse = function (source, ...args) {
    assert.ok(!String(source).includes('DO_NOT_DECODE_THE_FULL_ROOM_BRIEF'), 'Routine summaries must not materialize full room or task documents.');
    return parse.call(this, source, ...args);
  };
  let summaries;
  try { summaries = store.list(2); } finally { JSON.parse = parse; }
  assert.deepEqual(summaries, [...records].reverse().slice(0, 2).map(record => ({ id: record.id, backend: record.backend,
    title: record.title, status: record.status, taskCount: 4, completedTasks: 2, callsUsed: record.callsUsed, maxCalls: 32,
    revision: record.revision, updatedAt: record.updatedAt })));
  assert.ok(Buffer.byteLength(JSON.stringify(summaries)) < 1024);
  summaries[0].title = 'Caller mutation';
  assert.equal(store.list(1)[0].title, records[3].title);
  assert.equal(store.list().length, 4);
  for (const limit of [0, -1, 1.5, 1001, Infinity, NaN]) assert.throws(() => store.list(limit), /summary limit/);
});
