import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, relative, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import test from 'node:test';
import { CouncilStore } from '../dist/store.js';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-storage-fault-'));
  const stores = [];
  t.after(() => {
    for (const store of stores) { try { store.close(); } catch {} }
    assert.ok(relative(resolve(tmpdir()), resolve(root)).startsWith('council-storage-fault-'));
    rmSync(root, { recursive: true, force: true });
  });
  return { root, open: () => { const store = new CouncilStore(root); stores.push(store); return store; } };
}

function record(id) {
  const now = new Date().toISOString();
  return { id, status: 'queued', createdAt: now, updatedAt: now, input: { mode: 'delegate', task: 'Offline persistence fault fixture.', agents: ['codex'], permission: 'read' }, steps: [] };
}

function constrainGrowth(database) {
  const pages = database.prepare('PRAGMA page_count').get().page_count;
  database.exec(`PRAGMA max_page_count = ${pages + 2}`);
}

test('a real SQLite FULL error rolls back partial run writes without changing durable metadata', t => {
  const store = fixture(t).open();
  store.registerInstance('full-write-owner', process.pid, 1);
  const run = record('full-write');
  store.createRun(run, 'full-write-owner');
  const before = store.getRunSummary(run.id);
  constrainGrowth(store.db);
  assert.throws(() => store.saveRun({ ...run, status: 'running', output: 'x'.repeat(2 * 1024 * 1024) }), /database or disk is full/i);
  assert.deepEqual(store.getRun(run.id), run);
  assert.deepEqual(store.getRunSummary(run.id), before);
  assert.equal(store.db.isTransaction, false);
  store.db.exec('PRAGMA max_page_count = 1000000');
  store.saveRun({ ...run, status: 'running', output: 'x'.repeat(2 * 1024 * 1024) });
  assert.equal(store.getRunSummary(run.id).revision, before.revision + 1);
  assert.equal(store.getRun(run.id).output.length, 2 * 1024 * 1024);
  assert.deepEqual(store.checkIntegrity(), ['ok']);
});

test('a failed finalization preserves cancellation and a later finalization cannot publish stale success', t => {
  const store = fixture(t).open();
  store.registerInstance('full-finalize-owner', process.pid, 1);
  const run = record('full-finalize');
  store.createRun(run, 'full-finalize-owner');
  store.requestCancellation(run.id);
  const before = store.getRunSummary(run.id);
  constrainGrowth(store.db);
  const completed = { ...run, status: 'completed', output: 'y'.repeat(2 * 1024 * 1024) };
  assert.throws(() => store.finalizeRun(completed), /database or disk is full/i);
  assert.equal(store.isCancellationRequested(run.id), true);
  assert.deepEqual(store.getRunSummary(run.id), before);
  store.db.exec('PRAGMA max_page_count = 1000000');
  const final = store.finalizeRun(completed);
  assert.equal(final.status, 'cancelled');
  assert.equal(final.output.length, completed.output.length);
  assert.equal(store.getRunSummary(run.id).revision, before.revision + 1);
  assert.deepEqual(store.checkIntegrity(), ['ok']);
});

test('a SQLite FULL error during a v2 migration leaves the old schema and records intact for a clean retry', t => {
  const { root, open } = fixture(t);
  const path = join(root, 'state.db');
  const legacy = new DatabaseSync(path);
  legacy.exec(`CREATE TABLE runs(id TEXT PRIMARY KEY,updated_at TEXT NOT NULL,document TEXT NOT NULL,owner_instance TEXT,owner_pid INTEGER,cancel_requested INTEGER NOT NULL DEFAULT 0);
    CREATE TABLE events(sequence INTEGER PRIMARY KEY AUTOINCREMENT,run_id TEXT NOT NULL,type TEXT NOT NULL,data TEXT NOT NULL,created_at TEXT NOT NULL);PRAGMA user_version=2;`);
  const payload = 'z'.repeat(128 * 1024);
  for (let index = 0; index < 20; index++) {
    const run = { ...record(`legacy-${index}`), status: 'completed', output: payload };
    legacy.prepare('INSERT INTO runs(id,updated_at,document) VALUES(?,?,?)').run(run.id, run.updatedAt, JSON.stringify(run));
  }
  legacy.close();
  const migrate = CouncilStore.prototype.migrateMetadata;
  CouncilStore.prototype.migrateMetadata = function () { constrainGrowth(this.db); return migrate.call(this); };
  try { assert.throws(() => new CouncilStore(root), /database or disk is full/i); }
  finally { CouncilStore.prototype.migrateMetadata = migrate; }
  const inspection = new DatabaseSync(path);
  assert.equal(inspection.prepare('PRAGMA user_version').get().user_version, 2);
  assert.equal(inspection.prepare('PRAGMA table_info(runs)').all().some(column => column.name === 'summary'), false);
  assert.equal(inspection.prepare('SELECT COUNT(*) AS count FROM runs').get().count, 20);
  assert.equal(JSON.parse(inspection.prepare('SELECT document FROM runs WHERE id = ?').get('legacy-0').document).output, payload);
  inspection.close();
  const upgraded = open();
  assert.equal(upgraded.health().schemaVersion, 5);
  assert.equal(upgraded.listRunSummaries(20).length, 20);
  assert.equal(upgraded.getRun('legacy-0').output, payload);
  assert.deepEqual(upgraded.checkIntegrity(), ['ok']);
});
