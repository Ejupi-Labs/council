import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, relative, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import test from 'node:test';
import { CouncilStore } from '../dist/store.js';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-storage-load-'));
  const store = new CouncilStore(root);
  t.after(() => {
    store.close();
    assert.ok(relative(resolve(tmpdir()), resolve(root)).startsWith('council-storage-load-'));
    rmSync(root, { recursive: true, force: true });
  });
  return { root, store };
}

function activeRun(id, payload) {
  const now = new Date().toISOString();
  return { id, createdAt: now, updatedAt: now, status: 'queued', input: { mode: 'parallel', task: 'Offline storage load fixture.', agents: ['codex'], permission: 'read' },
    steps: Array.from({ length: 4 }, (_, index) => ({ id: `step-${index}`, agent: 'codex', role: 'worker', result: { agent: 'codex', status: 'completed', durationMs: 1, output: payload } })), output: payload };
}

function elapsed(operation, repetitions) {
  const start = performance.now();
  for (let index = 0; index < repetitions; index++) operation();
  return Number((performance.now() - start).toFixed(2));
}

test('large active histories use compact indexed metadata and never parse live run payloads on scheduling paths', t => {
  const { root, store } = fixture(t);
  store.registerInstance('load-owner', process.pid);
  const payload = 'x'.repeat(256 * 1024);
  for (let index = 0; index < 20; index++) store.createRun(activeRun(`load-${index}`, payload), 'load-owner');
  for (let index = 0; index < 16; index++) store.appendEvent('load-0', 'step.finished', { output: 'y'.repeat(128 * 1024) });
  const fullBytes = Buffer.byteLength(JSON.stringify(store.listRuns(20)));
  const summaries = store.listRunSummaries(20);
  const summaryBytes = Buffer.byteLength(JSON.stringify(summaries));
  assert.ok(summaryBytes < fullBytes / 30, `Compact listing must reduce materialized payload by at least 30x (${summaryBytes}/${fullBytes}).`);
  const raw = new DatabaseSync(join(root, 'state.db'));
  const queryPlan = raw.prepare('EXPLAIN QUERY PLAN SELECT summary,revision,cancel_requested FROM runs ORDER BY updated_at DESC,id DESC LIMIT 20').all();
  raw.close();
  assert.ok(queryPlan.some(row => row.detail.includes('runs_recent')));
  assert.ok(queryPlan.every(row => !row.detail.includes('TEMP B-TREE')));

  const nativeParse = JSON.parse;
  let parsedBytes = 0;
  JSON.parse = (text, ...args) => {
    const size = Buffer.byteLength(text);
    assert.ok(size < 100000, `A metadata operation attempted to parse a ${size}-byte run/event payload.`);
    parsedBytes += size;
    return nativeParse(text, ...args);
  };
  const timings = {};
  try {
    timings.list20x10Ms = elapsed(() => store.listRunSummaries(20), 10);
    timings.cancelCheck100Ms = elapsed(() => assert.equal(store.isCancellationRequested('load-0'), false), 100);
    timings.compactEvents10Ms = elapsed(() => store.compactEvents('load-0', 0, 100), 10);
    timings.acquireRelease10Ms = elapsed(() => {
      const token = store.acquireSlot('load-owner', 'load-0', 3);
      assert.ok(token);
      store.releaseSlot(token, 'load-owner');
    }, 10);
    assert.deepEqual(store.recoverOrphans(), []);
    assert.equal(store.health().counts.activeRuns, 20);
    assert.equal(store.getRunSummary('load-0').output.length, 16000);
  } finally { JSON.parse = nativeParse; }
  assert.ok(parsedBytes < fullBytes / 2, 'Repeated metadata operations must not materialize the full history.');
  assert.ok(store.health().preparedStatements <= 96);
  t.diagnostic(JSON.stringify({ fullBytes, summaryBytes, reduction: Number((fullBytes / summaryBytes).toFixed(1)), ...timings }));
});

test('cancellation after a thousand events remains constant-size and immediately visible', t => {
  const { store } = fixture(t);
  store.registerInstance('cancel-load-owner', process.pid);
  store.createRun(activeRun('cancel-load', 'evidence'), 'cancel-load-owner');
  for (let index = 0; index < 1001; index++) store.appendEvent('cancel-load', 'progress', { index });
  store.appendEvent('cancel-load', 'cancel_requested', {});
  assert.equal(store.events('cancel-load').some(event => event.type === 'cancel_requested'), false, 'The backward-compatible raw page deliberately stops at 1000 events.');
  const nativeParse = JSON.parse;
  JSON.parse = () => { throw new Error('Cancellation must not deserialize any JSON.'); };
  try {
    assert.equal(store.isCancellationRequested('cancel-load'), true);
    assert.throws(() => store.acquireSlot('cancel-load-owner', 'cancel-load', 1), /Cancellation/);
  } finally { JSON.parse = nativeParse; }
});

test('unread mailbox polls use a partial index instead of rescanning acknowledged history', t => {
  const { root, store } = fixture(t);
  const raw = new DatabaseSync(join(root, 'state.db'));
  raw.exec('BEGIN IMMEDIATE');
  const insert = raw.prepare("INSERT INTO messages(id,sender,recipient,created_at,acknowledged_at,document) VALUES (?,'claude','codex','2020-01-01T00:00:00.000Z','2020-01-02T00:00:00.000Z','{}')");
  for (let index = 0; index < 50000; index++) insert.run(`acknowledged-${index}`);
  raw.exec('COMMIT');
  const plan = raw.prepare('EXPLAIN QUERY PLAN SELECT document, acknowledged_at FROM messages INDEXED BY messages_unread WHERE recipient = ? AND acknowledged_at IS NULL ORDER BY created_at, rowid LIMIT 100').all('codex');
  raw.close();
  assert.ok(plan.some(row => row.detail.includes('messages_unread')));
  assert.ok(plan.every(row => !row.detail.includes('TEMP B-TREE')));
  const message = store.sendMessage({ from: 'claude', to: 'codex', subject: 'Fresh review', body: 'The acknowledged history must not slow unread delivery.' });
  const elapsedMs = elapsed(() => assert.deepEqual(store.inbox('codex').map(value => value.id), [message.id]), 100);
  assert.equal(store.inbox('codex', false).length, 100, 'The explicit historical mailbox remains available.');
  assert.equal(store.health().counts.unreadMessages, 1);
  assert.equal(store.acknowledge(message.id, 'codex'), true);
  assert.deepEqual(store.inbox('codex'), []);
  assert.equal(store.health().counts.unreadMessages, 0);
  t.diagnostic(JSON.stringify({ acknowledgedMessages: 50000, unreadPolls: 100, elapsedMs }));
});

test('expired lease histories use indexed health counts and are removed atomically on the next reservation', t => {
  const { root, store } = fixture(t);
  const raw = new DatabaseSync(join(root, 'state.db'));
  raw.exec('BEGIN IMMEDIATE');
  const insert = raw.prepare('INSERT INTO leases(id,owner,path,path_key,expires_at) VALUES (?,?,?,?,?)');
  for (let index = 0; index < 100000; index++) {
    const path = `expired/${index}.ts`;
    insert.run(`expired-${index}`, 'past-owner', path, path, 1);
  }
  insert.run('live-lease', 'live-owner', 'live.ts', 'live.ts', Date.now() + 60000);
  raw.exec('COMMIT');
  const plan = raw.prepare('EXPLAIN QUERY PLAN SELECT COUNT(*) FROM leases WHERE expires_at > ?').all(Date.now());
  assert.ok(plan.some(row => row.detail.includes('COVERING INDEX leases_expiry')));
  const elapsedMs = elapsed(() => assert.equal(store.health().counts.leases, 1), 100);
  store.reserve('next-owner', ['expired/0.ts'], 60000);
  assert.equal(store.health().counts.leases, 2);
  assert.equal(raw.prepare('SELECT COUNT(*) AS count FROM leases').get().count, 2);
  assert.deepEqual(store.leases().map(value => value.owner).sort(), ['live-owner', 'next-owner']);
  raw.close();
  t.diagnostic(JSON.stringify({ expiredLeases: 100000, healthCalls: 100, elapsedMs }));
});
