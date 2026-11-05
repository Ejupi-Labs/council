import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { CouncilStore } from '../dist/store.js';

function fixture(t) {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-quarantine-migration-'))), stores = [];
  t.after(() => {
    for (const store of stores) { try { store.close(); } catch {} }
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-quarantine-migration-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { root, open() { const store = new CouncilStore(root); stores.push(store); return store; } };
}

function versionFive(setup, { live = false } = {}) {
  const store = setup.open(), now = '2026-01-01T00:00:00.000Z';
  store.saveRun({ id: 'retained', status: 'completed', createdAt: now, updatedAt: now,
    input: { mode: 'delegate', task: 'Complete original task.', agents: ['codex'], permission: 'read' }, steps: [], output: 'Full retained evidence. '.repeat(4000) });
  store.putContext('contract', 'Complete original project contract.', 'Human');
  if (live) store.registerInstance('old-host', process.pid);
  store.close();
  const database = new DatabaseSync(join(setup.root, 'state.db'));
  database.exec('DROP TABLE process_cleanup_incidents; DROP TABLE dispatch_quarantine_acknowledgments; PRAGMA user_version=5; VACUUM;');
  const original = database.prepare('SELECT document, summary, revision FROM runs WHERE id = ?').get('retained');
  database.close(); return original;
}

test('schema five upgrades with exact retained records and no metadata rebuild', t => {
  const setup = fixture(t), original = versionFive(setup);
  const migrate = CouncilStore.prototype.migrateMetadata;
  CouncilStore.prototype.migrateMetadata = () => { assert.fail('Schema five already has run metadata and must not decode/rebuild it.'); };
  let upgraded;
  try { upgraded = setup.open(); } finally { CouncilStore.prototype.migrateMetadata = migrate; }
  assert.equal(upgraded.health().schemaVersion, 6);
  assert.deepEqual(upgraded.db.prepare('SELECT document, summary, revision FROM runs WHERE id = ?').get('retained'), original);
  assert.equal(upgraded.getContext('contract').value, 'Complete original project contract.');
  assert.deepEqual(upgraded.dispatchQuarantinePage(), { blocked: false, currentRevision: '0', openCount: 0, incidents: [], nextCursor: null });
  assert.deepEqual(upgraded.checkIntegrity(), ['ok']);
});

test('a known live schema-five host blocks quarantine migration before incident tables are created', t => {
  const setup = fixture(t), original = versionFive(setup, { live: true });
  assert.throws(() => setup.open(), /Stop older Council hosts/);
  const database = new DatabaseSync(join(setup.root, 'state.db'));
  try {
    assert.equal(database.prepare('PRAGMA user_version').get().user_version, 5);
    assert.equal(database.prepare("SELECT COUNT(*) AS count FROM sqlite_master WHERE name IN ('process_cleanup_incidents', 'dispatch_quarantine_acknowledgments')").get().count, 0);
    assert.deepEqual(database.prepare('SELECT document, summary, revision FROM runs WHERE id = ?').get('retained'), original);
    database.prepare('DELETE FROM instances WHERE id = ?').run('old-host');
  } finally { database.close(); }
  assert.equal(setup.open().health().schemaVersion, 6);
});

test('actual SQLite FULL during schema-six creation rolls back the migration and supports a clean retry', t => {
  const setup = fixture(t), original = versionFive(setup);
  const exec = DatabaseSync.prototype.exec;
  DatabaseSync.prototype.exec = function (sql) {
    if (sql.includes('CREATE TABLE IF NOT EXISTS dispatch_quarantine_acknowledgments')) {
      const pages = this.prepare('PRAGMA page_count').get().page_count;
      exec.call(this, `PRAGMA max_page_count=${pages}`);
    }
    return exec.call(this, sql);
  };
  try { assert.throws(() => setup.open(), /database or disk is full/i); }
  finally { DatabaseSync.prototype.exec = exec; }
  const inspection = new DatabaseSync(join(setup.root, 'state.db'));
  try {
    assert.equal(inspection.prepare('PRAGMA user_version').get().user_version, 5);
    assert.equal(inspection.prepare("SELECT COUNT(*) AS count FROM sqlite_master WHERE name IN ('process_cleanup_incidents', 'dispatch_quarantine_acknowledgments')").get().count, 0);
    assert.deepEqual(inspection.prepare('SELECT document, summary, revision FROM runs WHERE id = ?').get('retained'), original);
  } finally { inspection.close(); }
  const upgraded = setup.open(); assert.equal(upgraded.health().schemaVersion, 6); assert.deepEqual(upgraded.checkIntegrity(), ['ok']);
});

test('unpublished schema-six prototypes upgrade the nullable historical PID only after old hosts stop', t => {
  const setup = fixture(t), owner = setup.open(); owner.registerInstance('prototype-host', process.pid);
  const input = { reportKey: 'prototype-attempt', instanceId: 'prototype-host', stage: 'agent_call' };
  owner.reportProcessCleanupIncident(input); owner.close();
  const database = new DatabaseSync(join(setup.root, 'state.db'));
  database.exec('ALTER TABLE process_cleanup_incidents DROP COLUMN reporting_pid;');
  const before = database.prepare('SELECT id, generation, report_key, instance_id, observed_at, acknowledgment_id FROM process_cleanup_incidents').all();
  try {
    assert.throws(() => setup.open(), /Stop older Council hosts/);
    assert.equal(database.prepare("SELECT COUNT(*) AS count FROM pragma_table_info('process_cleanup_incidents') WHERE name='reporting_pid'").get().count, 0);
    assert.equal(database.prepare('PRAGMA user_version').get().user_version, 6);
    database.prepare('DELETE FROM instances WHERE id = ?').run('prototype-host');
  } finally { database.close(); }
  const upgraded = setup.open();
  assert.equal(upgraded.health().schemaVersion, 6);
  assert.deepEqual(upgraded.db.prepare('SELECT id, generation, report_key, instance_id, observed_at, acknowledgment_id FROM process_cleanup_incidents').all(), before);
  assert.equal(upgraded.db.prepare('SELECT reporting_pid FROM process_cleanup_incidents').get().reporting_pid, null);
  assert.equal(Object.hasOwn(upgraded.dispatchQuarantinePage().incidents[0], 'reportingPid'), false);
  assert.equal(Object.hasOwn(upgraded.reportProcessCleanupIncident(input), 'reportingPid'), false);
  upgraded.acknowledgeDispatchQuarantine({ expectedRevision: '1', reviewed: true });
  assert.equal(Object.hasOwn(upgraded.reportProcessCleanupIncident(input), 'reportingPid'), false);
});
