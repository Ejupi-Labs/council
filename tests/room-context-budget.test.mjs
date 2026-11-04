import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';

const task = brief => ({ title: 'Inspect complete evidence', brief, permission: 'read' });
const initialMessage = { author: 'system', kind: 'system', body: 'The project manager owns task briefs and permissions. Discussion is read-only; approved work uses the original task permissions.' };
const input = overrides => ({ title: 'Incremental complete context', brief: 'Coordinate complete evidence.',
  members: [{ agent: 'codex', role: 'worker' }], tasks: [task('Report complete findings.')], autoExecute: false, ...overrides });
const message = (index, body) => ({ id: 'evidence-' + index, author: 'Human', kind: 'human', body,
  createdAt: '2026-11-04T00:00:00.000Z' });

function fixture(t) {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-context-budget-')));
  const requests = [];
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), { doctor: async () => [], run: async request => {
    requests.push(request);
    return { agent: request.agent, status: 'failed', output: '', durationMs: 1, error: 'This context-only fixture must not dispatch.' };
  } });
  t.after(async () => {
    await service.close();
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-context-budget-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { service, manager: service.rooms, requests };
}

function assertNoDispatch({ service, manager, requests }, before) {
  const current = manager.metadata(before.id);
  assert.equal(current.status, 'idle'); assert.equal(current.callsUsed, 0); assert.equal(current.revision, before.revision);
  assert.equal(requests.length, 0); assert.equal(service.store.health().counts.activeRuns, 0);
  assert.equal(service.store.health().counts.slots, 0);
}

test('accepted room context preserves exact Unicode, escaping, field order and complete workspace evidence at the character boundary', t => {
  const { manager, service, requests } = fixture(t);
  const brief = 'Room "brief" \\ \n🧭', pending = 'Read "complete" \\ evidence.\n漢字 🧭';
  const artifact = { agent: 'codex', path: 'C:\\retained\\🧭', baseCommit: 'full-base', exportStatus: 'ready', patchPath: 'C:\\retained\\change.patch' };
  const room = manager.create(input({ brief, tasks: [task(pending), { ...task('Retained write task.'), dependsOn: ['task-1'] }, task('Read-only task.'), task('Archived task.'), task('Unverified task.')] }));
  const human = message('unicode', 'Complete "human" \\ evidence.\n🧭漢字');
  const report = { ...message('report', 'Full report.\nNo clipping.'), author: 'codex', kind: 'task', runId: 'written-run' };
  const record = manager.store.mutate(room.id, current => {
    current.tasks[1].status = 'completed'; current.tasks[1].runId = 'written-run'; current.tasks[1].workspaces = [artifact];
    current.tasks[2].status = 'completed'; current.tasks[2].runId = 'read-run';
    current.tasks[3].status = 'completed'; current.tasks[3].runId = 'archived-run'; current.tasks[3].workspaces = [];
    current.tasks[4].status = 'completed'; current.tasks[4].runId = 'unavailable-run';
    current.messages.push(human, report);
  });
  const getRun = service.getRun;
  service.getRun = id => id === 'written-run' ? { id, input: { permission: 'write' }, workspaces: [{ ...artifact, path: 'C:\\different-current-metadata' }] }
    : id === 'read-run' ? { id, input: { permission: 'read' } } : undefined;
  t.after(() => { service.getRun = getRun; });
  const expected = [
    JSON.stringify({ roomBrief: brief }),
    JSON.stringify({ task: 'task-1', brief: pending, status: 'pending' }),
    JSON.stringify({ task: 'task-2', brief: 'Retained write task.', status: 'completed', dependsOn: ['task-1'], runId: 'written-run', workflowRecordAvailable: true,
      fullResultResource: 'council://runs/written-run', workspaces: [artifact], workspaceEvidence: 'Retained export metadata; inspect paths to verify current access and contents.' }),
    JSON.stringify({ task: 'task-3', brief: 'Read-only task.', status: 'completed', runId: 'read-run', workflowRecordAvailable: true,
      fullResultResource: 'council://runs/read-run', workspaces: [], workspaceEvidence: 'The workflow recorded no retained worktrees.' }),
    JSON.stringify({ task: 'task-4', brief: 'Archived task.', status: 'completed', runId: 'archived-run', workflowRecordAvailable: false,
      workspaces: [], workspaceEvidence: 'The workflow recorded no retained worktrees.' }),
    JSON.stringify({ task: 'task-5', brief: 'Unverified task.', status: 'completed', runId: 'unavailable-run', workflowRecordAvailable: false,
      workspaceEvidence: 'Workspace metadata is unavailable; do not assume prerequisite changes are present.' }),
    JSON.stringify(initialMessage),
    JSON.stringify({ author: 'Human', kind: 'human', body: human.body }),
    JSON.stringify({ author: 'codex', kind: 'task', body: report.body, runId: 'written-run' }),
  ].join('\n');
  assert.notEqual(Buffer.byteLength(expected), expected.length, 'The fixture distinguishes character limits from UTF-8 byte limits.');
  for (const contextLimit of [null, expected.length, expected.length + 1]) {
    const actual = manager.context({ ...record, contextLimit });
    assert.equal(actual, expected); assert.deepEqual(Buffer.from(actual), Buffer.from(expected));
  }
  assert.throws(() => manager.context({ ...record, contextLimit: expected.length - 1 }), new RegExp(`requires at least ${expected.length} characters`));
  assert.equal(requests.length, 0);
});

test('an oversized room brief fails before history paging or workflow admission', t => {
  const current = fixture(t), { manager } = current;
  const room = manager.create(input({ contextLimit: 1000, brief: 'Full evidence 🧭 '.repeat(200) }));
  const read = manager.store.messagePage;
  manager.store.messagePage = () => { assert.fail('An already oversized room brief must not page history.'); };
  try {
    const required = JSON.stringify({ roomBrief: room.brief }).length;
    assert.throws(() => manager.startDiscussion(room.id), new RegExp(`requires at least ${required} characters, exceeding the configured context budget 1000`));
    assertNoDispatch(current, room);
  } finally { manager.store.messagePage = read; }
});

test('an oversized task brief fails before looking up later workflow records or paging history', t => {
  const current = fixture(t), { manager, service } = current;
  const room = manager.create(input({ contextLimit: 1000, tasks: [task('Complete task evidence. '.repeat(100)), task('Later retained evidence.')] }));
  const record = manager.store.mutate(room.id, value => { value.tasks[1].runId = 'later-workflow'; });
  const read = manager.store.messagePage, getRun = service.getRun;
  manager.store.messagePage = () => { assert.fail('An already oversized task brief must not page history.'); };
  service.getRun = () => { assert.fail('Later workflow records must not be decoded after the context budget is exceeded.'); };
  try {
    assert.throws(() => manager.startDiscussion(room.id), /requires at least \d+ characters.*increase the context budget/);
    assertNoDispatch(current, record);
  } finally { manager.store.messagePage = read; service.getRun = getRun; }
});

test('first-page context overflow never reads subsequent pages and retains every complete durable message', t => {
  const current = fixture(t), { manager } = current;
  const room = manager.create(input({ contextLimit: 4096 }));
  const history = Array.from({ length: 120 }, (_, index) => message(index, 'Complete evidence. '.repeat(2200) + '\nFULL_MESSAGE_END_' + index));
  const record = manager.store.mutate(room.id, value => { value.messages.push(...history); });
  const read = manager.store.messagePage;
  let pages = 0;
  manager.store.messagePage = function (...args) {
    assert.equal(++pages, 1, 'No later page may be read after the first oversized prefix.');
    return read.apply(this, args);
  };
  try {
    assert.throws(() => manager.startDiscussion(room.id), /requires at least \d+ characters, exceeding the configured context budget 4096/);
    assert.equal(pages, 1); assertNoDispatch(current, record);
  } finally { manager.store.messagePage = read; }
  const retained = [];
  let cursor;
  do { const page = manager.messagePage(room.id, 100, cursor); retained.push(...page.messages); cursor = page.nextCursor; } while (cursor !== null);
  assert.deepEqual(retained, [...room.messages, ...history]);
  assert.ok(retained.at(-1).body.endsWith('FULL_MESSAGE_END_119'));
});

test('later-page overflow reports the complete offending prefix while accepted multi-page contexts remain exact', t => {
  const current = fixture(t), { manager } = current;
  const room = manager.create(input());
  const history = Array.from({ length: 240 }, (_, index) => message(index, `Complete evidence ${index}: "quoted" \\ \n🧭`));
  const record = manager.store.mutate(room.id, value => { value.messages.push(...history); });
  const leading = [JSON.stringify({ roomBrief: room.brief }), JSON.stringify({ task: 'task-1', brief: 'Report complete findings.', status: 'pending' })];
  const messages = [initialMessage, ...history.map(value => ({ author: 'Human', kind: 'human', body: value.body }))].map(value => JSON.stringify(value));
  const expected = [...leading, ...messages].join('\n'), firstPage = [...leading, ...messages.slice(0, 100)].join('\n');
  const offendingPrefix = [...leading, ...messages.slice(0, 101)].join('\n');
  const read = manager.store.messagePage;
  let pages = 0, maximum = 3;
  manager.store.messagePage = function (...args) {
    assert.ok(++pages <= maximum, 'Context construction must stop before the next history page.');
    return read.apply(this, args);
  };
  try {
    assert.equal(manager.context(record), expected); assert.equal(pages, 3);
    pages = 0;
    assert.deepEqual(Buffer.from(manager.context({ ...record, contextLimit: expected.length })), Buffer.from(expected)); assert.equal(pages, 3);
    manager.store.mutate(room.id, value => { value.contextLimit = firstPage.length; });
    const before = manager.metadata(room.id);
    pages = 0; maximum = 2;
    assert.throws(() => manager.startDiscussion(room.id), new RegExp(`requires at least ${offendingPrefix.length} characters, exceeding the configured context budget ${firstPage.length}`));
    assert.equal(pages, 2); assertNoDispatch(current, before);
  } finally { manager.store.messagePage = read; }
});
