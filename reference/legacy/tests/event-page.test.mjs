import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { CouncilStore, RunEventsUnavailableError } from '../dist/store.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { MAX_EVENT_PAGE_BYTES } from '../dist/summary.js';

const record = id => ({ id, status: 'completed', input: { mode: 'delegate', agents: ['codex'], permission: 'read', task: 'Offline timeline evidence.' },
  createdAt: '2026-01-01T00:00:00.000Z', updatedAt: '2026-01-01T00:00:00.000Z', steps: [], output: 'Complete offline evidence.' });
const unavailable = error => error instanceof RunEventsUnavailableError && error.status === 404
  && error.code === 'COUNCIL_RUN_EVENTS_UNAVAILABLE' && /pruned/.test(error.message);
function fixture(t) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-event-page-')), stores = [];
  const open = () => { const store = new CouncilStore(root); stores.push(store); return store; };
  t.after(() => {
    for (const store of stores) store.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-event-page-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, open, store: open() };
}

test('complete event pages preserve payloads and ascending global cursors across run gaps and reopen', t => {
  const { store, open } = fixture(t), run = record('exact-events'); store.saveRun(run); store.saveRun(record('other-run'));
  const payload = { input: { ...run.input, task: 'COMPLETE_TASK '.repeat(600), context: 'COMPLETE_CONTEXT '.repeat(4000) },
    result: { agent: 'codex', status: 'completed', output: 'FULL_OUTPUT '.repeat(12000) + 'UNCLIPPED_TAIL', durationMs: 12 },
    nested: Array.from({ length: 40 }, (_, index) => ({ index, unicode: 'Quoted " evidence\n🧭', values: [null, true, false, 0, -3.5] })) };
  const first = store.appendEvent(run.id, 'step.finished', payload);
  store.appendEvent('other-run', 'step.finished', { privateToOtherRun: true });
  const second = store.appendEvent(run.id, 'workspace.created', { path: 'C:\\exact path\\worktree', baseCommit: 'full-base' });
  const third = store.appendEvent(run.id, 'run.completed', { output: 'Final complete result.' });
  const page = store.eventPage(run.id, 0, 2);
  assert.deepEqual(page.events, [first, second]); assert.equal(page.nextAfter, second.sequence);
  assert.equal(page.hasMore, true); assert.equal(page.nextCursor, second.sequence);
  assert.deepEqual(page.events[0].data, payload); assert.match(page.events[0].data.result.output, /UNCLIPPED_TAIL$/);
  const restored = open().eventPage(run.id, page.nextCursor, 2);
  assert.deepEqual(restored, { events: [third], nextAfter: third.sequence, hasMore: false, nextCursor: null });
  assert.deepEqual(store.eventPage(run.id, third.sequence), { events: [], nextAfter: third.sequence, hasMore: false, nextCursor: null });
  page.events[0].data.result.output = 'Caller mutation'; assert.deepEqual(store.eventPage(run.id, 0, 1).events[0], first);
});

test('exact type filters bind literal values and do not conflate case, substrings or unrelated runs', t => {
  const { store } = fixture(t); store.saveRun(record('filtered')); store.saveRun(record('foreign'));
  const a = store.appendEvent('filtered', 'step.finished', { id: 'a' });
  store.appendEvent('filtered', 'pre.step.finished', {}); store.appendEvent('filtered', 'STEP.FINISHED', {});
  store.appendEvent('foreign', 'step.finished', {});
  const b = store.appendEvent('filtered', 'step.finished', { id: 'b' });
  const injection = store.appendEvent('filtered', "' OR 1=1 --", { literal: true });
  const first = store.eventPage('filtered', 0, 1, 'step.finished');
  assert.deepEqual(first.events, [a]); assert.equal(first.nextCursor, a.sequence);
  assert.deepEqual(store.eventPage('filtered', first.nextCursor, 1, 'step.finished').events, [b]);
  assert.deepEqual(store.eventPage('filtered', 0, 100, "' OR 1=1 --").events, [injection]);
  assert.deepEqual(store.eventPage('filtered', b.sequence, 100, 'absent'), { events: [], nextAfter: b.sequence, hasMore: false, nextCursor: null });
});

test('rare filtered events use the covering type index without materializing an unbounded nonmatching history', t => {
  const { root, store } = fixture(t); store.saveRun(record('sparse'));
  const raw = new DatabaseSync(join(root, 'state.db'));
  try {
    raw.exec('BEGIN IMMEDIATE');
    const insert = raw.prepare("INSERT INTO events(run_id,type,data,created_at,summary) VALUES ('sparse','progress',?,'2026-01-01T00:00:00.000Z','null')");
    // Invalid nonmatching payloads make any accidental decode or JS history filter fail.
    for (let index = 0; index < 12000; index++) insert.run('This deliberately is not JSON.');
    raw.exec('COMMIT');
    const expected = store.appendEvent('sparse', 'rare.finished', { complete: true, evidence: 'EXACT_RARE_TAIL' });
    const plan = raw.prepare('EXPLAIN QUERY PLAN SELECT sequence FROM events INDEXED BY events_run_type_sequence WHERE run_id = ? AND type = ? AND sequence > ? ORDER BY sequence LIMIT ?')
      .all('sparse', 'rare.finished', 0, 101);
    assert.ok(plan.some(row => row.detail.includes('COVERING INDEX events_run_type_sequence')));
    assert.ok(plan.every(row => !/SCAN |TEMP B-TREE/.test(row.detail)));
    const parse = JSON.parse; let decoded = 0;
    JSON.parse = (...args) => { decoded++; return parse(...args); };
    try { assert.deepEqual(store.eventPage('sparse', 0, 100, 'rare.finished').events, [expected]); }
    finally { JSON.parse = parse; }
    assert.equal(decoded, 1, 'Only the returned exact payload is decoded; run metadata and nonmatches stay in SQLite.');
    assert.ok(store.health().preparedStatements <= 96);
  } finally { raw.close(); }
});

test('byte-bounded pages return every event intact and an oversized event alone without skipping its cursor', t => {
  const { store } = fixture(t); store.saveRun(record('large'));
  const expected = [
    store.appendEvent('large', 'progress', { text: '🧭'.repeat(150000) + 'FIRST_TAIL' }),
    store.appendEvent('large', 'progress', { text: '🧭'.repeat(150000) + 'SECOND_TAIL' }),
    store.appendEvent('large', 'progress', { text: 'L'.repeat(MAX_EVENT_PAGE_BYTES * 2) + 'OVERSIZED_TAIL' }),
    store.appendEvent('large', 'progress', { text: 'LAST_COMPLETE_EVENT' }),
  ];
  const parse = JSON.parse; let decoded = 0;
  JSON.parse = (...args) => { decoded++; return parse(...args); };
  let first;
  try { first = store.eventPage('large', 0, 100); } finally { JSON.parse = parse; }
  assert.equal(decoded, 1, 'A candidate exceeding the remaining byte budget is not decoded.');
  assert.deepEqual(first.events, [expected[0]]); assert.equal(first.nextAfter, expected[0].sequence);
  const actual = [...first.events], pages = [first]; let next = first.nextCursor;
  while (next !== null) {
    const page = store.eventPage('large', next, 100); assert.ok(page.events.length);
    assert.ok(Buffer.byteLength(JSON.stringify(page.events)) <= MAX_EVENT_PAGE_BYTES || page.events.length === 1);
    actual.push(...page.events); pages.push(page); next = page.nextCursor;
  }
  assert.deepEqual(actual, expected); assert.deepEqual(pages.map(page => page.events.length), [1, 1, 1, 1]);
  assert.ok(Buffer.byteLength(JSON.stringify(pages[2].events)) > MAX_EVENT_PAGE_BYTES);
  assert.equal(pages[2].nextAfter, expected[2].sequence); assert.equal(pages[3].hasMore, false);
});

test('a read snapshot stays consistent when another connection prunes between existence and payload lookup', t => {
  const { store, open } = fixture(t), writer = open(); store.saveRun(record('pruned-between-reads'));
  const event = store.appendEvent('pruned-between-reads', 'step.finished', { output: 'Retained in this read snapshot.' });
  const statement = store.statement.bind(store); let pruned = false;
  store.statement = sql => {
    const prepared = statement(sql);
    if (sql !== 'SELECT 1 FROM runs WHERE id = ?' || pruned) return prepared;
    return { get(...args) {
      const exists = prepared.get(...args);
      assert.equal(store.db.isTransaction, true); pruned = true;
      assert.equal(writer.prune({ before: '2099-01-01T00:00:00Z', keepLatest: 0, dryRun: false }).deletedRuns, 1);
      return exists;
    } };
  };
  assert.deepEqual(store.eventPage('pruned-between-reads').events, [event]);
  assert.equal(writer.getRun('pruned-between-reads'), undefined);
  assert.throws(() => store.eventPage('pruned-between-reads'), unavailable);
});

test('new appends appear after the previous cursor on explicit refresh without duplicating older events', t => {
  const { store, open } = fixture(t), writer = open(); store.saveRun(record('live-append'));
  const first = store.appendEvent('live-append', 'selected.type', { ordinal: 1 });
  const page = store.eventPage('live-append', 0, 100, 'selected.type'); assert.equal(page.hasMore, false);
  writer.appendEvent('live-append', 'not.selected', { ignored: true });
  const second = writer.appendEvent('live-append', 'selected.type', { ordinal: 2 });
  assert.deepEqual(store.eventPage('live-append', page.nextAfter, 100, 'selected.type').events, [second]);
  assert.deepEqual(store.eventPage('live-append', first.sequence, 100, 'unknown.type').events, []);
  const empty = store.eventPage('live-append', second.sequence, 100, 'selected.type');
  const third = writer.appendEvent('live-append', 'selected.type', { ordinal: 3 });
  assert.deepEqual(store.eventPage('live-append', empty.nextAfter, 100, 'selected.type').events, [third]);
});

test('missing and pruned records fail explicitly while retained runs with no events return an empty page', t => {
  const { store } = fixture(t); store.saveRun(record('empty'));
  assert.deepEqual(store.eventPage('empty'), { events: [], nextAfter: 0, hasMore: false, nextCursor: null });
  assert.throws(() => store.eventPage('missing'), unavailable);
  store.appendEvent('orphan-without-run', 'progress', { orphan: true });
  assert.throws(() => store.eventPage('orphan-without-run'), unavailable);
  store.saveRun(record('pruned')); store.appendEvent('pruned', 'completed', { complete: true });
  store.prune({ before: '2099-01-01T00:00:00Z', keepLatest: 0, dryRun: false });
  assert.throws(() => store.eventPage('pruned'), unavailable);
});

test('full event pagination validates safe cursor bounds, page size and exact UTF-8 type bounds', t => {
  const { store } = fixture(t); store.saveRun(record('validation'));
  for (const after of [-1, 0.1, Number.NaN, Number.POSITIVE_INFINITY, Number.MAX_SAFE_INTEGER + 1, '1', null]) {
    assert.throws(() => store.eventPage('validation', after), /cursor/);
  }
  for (const limit of [0, 101, 1.5, Number.NaN, '1', null]) assert.throws(() => store.eventPage('validation', 0, limit), /limit/);
  for (const type of ['', '   ', null, [], 'nul\0type', '🧭'.repeat(33)]) assert.throws(() => store.eventPage('validation', 0, 100, type), /Event type/);
  const type = '🧭'.repeat(32), event = store.appendEvent('validation', type, { exact: true });
  assert.deepEqual(store.eventPage('validation', 0, 100, type).events, [event]);
  assert.deepEqual(store.eventPage('validation', Number.MAX_SAFE_INTEGER), { events: [], nextAfter: Number.MAX_SAFE_INTEGER, hasMore: false, nextCursor: null });
});

test('service event pages are read-only, reusable and never invoke an agent', async t => {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-service-event-page-')); let calls = 0;
  const service = new CouncilService(loadConfig(root, { backend: 'mock' }), { doctor: async () => [], run: async () => { calls++; throw new Error('No provider call is authorized by an event read.'); } });
  t.after(async () => { await service.close(); assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-service-event-page-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  const run = record('service-timeline'); service.store.saveRun(run);
  const event = service.store.appendEvent(run.id, 'step.finished', { result: { output: 'Exact reusable service event.' } });
  const before = service.store.getRunSummary(run.id), count = service.store.health().counts;
  assert.deepEqual(service.eventPage(run.id, 0, 1, 'step.finished'), { events: [event], nextAfter: event.sequence, hasMore: false, nextCursor: null });
  assert.deepEqual(service.store.getRun(run.id), run); assert.deepEqual(service.store.getRunSummary(run.id), before);
  assert.deepEqual(service.store.health().counts, count); assert.equal(calls, 0);
  service.prune({ before: '2099-01-01T00:00:00Z', keepLatest: 0, dryRun: false });
  assert.throws(() => service.eventPage(run.id), unavailable); assert.equal(calls, 0);
});
