import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync } from 'node:fs';
import { arch, availableParallelism, platform, release, tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { DatabaseSync, constants } from 'node:sqlite';
import { fileURLToPath } from 'node:url';
import { CouncilStore } from '../dist/store.js';

// This diagnostic changes only its own temporary database and emits fixed metadata.
const runtime = fileURLToPath(new URL('../dist/', import.meta.url));
const nativeExec = DatabaseSync.prototype.exec;
const connections = [];
let directory, store;
const metadata = {
  node: process.version, platform: platform(), arch: arch(), osRelease: release(), parallelism: availableParallelism(),
  runtimeFiles: Object.fromEntries(['store.js', 'admission-attempt.js', 'sqlite-write.js'].map(name => [name,
    createHash('sha256').update(readFileSync(join(runtime, name))).digest('hex')])),
  results: [], cleanupErrors: [],
};
const errorMetadata = error => ({
  name: error instanceof Error ? error.name.slice(0, 64) : 'NonError',
  ...(typeof error?.code === 'string' && /^[A-Z0-9_]{1,64}$/.test(error.code) ? { code: error.code } : {}),
  ...(Number.isSafeInteger(error?.errcode) ? { sqliteCode: error.errcode } : {}),
});
const configuration = connection => Object.fromEntries(['busy_timeout', 'journal_mode', 'synchronous', 'locking_mode'].map(key =>
  [key, Object.values(connection.prepare(`PRAGMA ${key}`).get())[0]]));
const connect = options => {
  const connection = new DatabaseSync(join(directory, 'state.db'), options ?? {});
  connections.push(connection);
  return connection;
};
function observe(label, connection, operation) {
  const authorizations = { begin: 0, commit: 0, rollback: 0, writes: 0 };
  connection.setAuthorizer((action, arg1) => {
    if (action === constants.SQLITE_TRANSACTION) {
      const key = String(arg1).toLowerCase();
      if (Object.hasOwn(authorizations, key)) authorizations[key]++;
    }
    if ([constants.SQLITE_INSERT, constants.SQLITE_UPDATE, constants.SQLITE_DELETE].includes(action)) authorizations.writes++;
    return constants.SQLITE_OK;
  });
  const start = performance.now();
  let error;
  try { operation(); } catch (caught) { error = caught; }
  const elapsedMs = performance.now() - start;
  connection.setAuthorizer(null);
  const result = { label, elapsedMs, configuration: configuration(connection), authorizations,
    transaction: connection.isTransaction, error: errorMetadata(error) };
  metadata.results.push(result);
  assert.equal(error?.code, 'ERR_SQLITE_ERROR');
  assert.equal(error.errcode & 0xff, 5);
  assert.equal(connection.isTransaction, false);
  assert.equal(authorizations.begin, 1);
  assert.equal(authorizations.writes, 0);
}

let stage = 'fixture';
let failure;
try {
  const parent = realpathSync.native(tmpdir());
  directory = realpathSync.native(mkdtempSync(join(parent, 'council-busy-probe-')));
  assert.equal(dirname(directory), parent);
  assert.ok(basename(directory).startsWith('council-busy-probe-'));
  stage = 'store-open';
  store = new CouncilStore(directory);
  const db = store.db;
  metadata.sqlite = db.prepare('SELECT sqlite_version() AS version, sqlite_source_id() AS sourceId').get();
  metadata.compileOptions = db.prepare('PRAGMA compile_options').all().map(row => row.compile_options);
  metadata.configuration = configuration(db);
  const descriptor = Object.getOwnPropertyDescriptor(db, 'isTransaction');
  metadata.transactionGetter = { own: Boolean(descriptor), configurable: descriptor?.configurable, getter: typeof descriptor?.get === 'function' };
  assert.equal(metadata.configuration.busy_timeout, 250);
  assert.equal(metadata.configuration.journal_mode, 'wal');
  const before = db.prepare('SELECT COUNT(*) AS value FROM events').get().value;
  const blocker = connect();
  blocker.exec('BEGIN IMMEDIATE');
  stage = 'held-writer';
  for (let index = 0; index < 2; index++) {
    observe(`native250-before-${index}`, db, () => nativeExec.call(db, 'BEGIN IMMEDIATE'));
    observe(`council-append-${index}`, db, () => store.appendEvent('fixed-probe', 'evidence', {}));
    observe(`native250-after-${index}`, db, () => nativeExec.call(db, 'BEGIN IMMEDIATE'));
  }
  for (const [label, options, timeout] of [
    ['native-constructor250', { timeout: 250 }, undefined],
    ['native-pragma250', undefined, 250],
    ['native-pragma0', undefined, 0],
    ['native-pragma5000', undefined, 5000],
  ]) {
    const connection = connect(options);
    if (timeout !== undefined) connection.exec(`PRAGMA busy_timeout=${timeout}`);
    observe(label, connection, () => nativeExec.call(connection, 'BEGIN IMMEDIATE'));
  }
  assert.equal(db.prepare('SELECT COUNT(*) AS value FROM events').get().value, before);
  assert.equal(db.prepare('PRAGMA busy_timeout').get().timeout, 250);
  stage = 'recovery';
  blocker.exec('ROLLBACK');
  store.appendEvent('fixed-probe', 'recovered', {});
  assert.equal(db.prepare('SELECT COUNT(*) AS value FROM events').get().value, before + 1);
  metadata.recovery = { exactlyOneEvent: true, transaction: db.isTransaction, busyTimeout: db.prepare('PRAGMA busy_timeout').get().timeout };
} catch (error) {
  failure = { stage, ...errorMetadata(error) };
} finally {
  for (const connection of connections.toReversed()) {
    try { if (connection.isTransaction) connection.exec('ROLLBACK'); }
    catch (error) { metadata.cleanupErrors.push({ stage: 'rollback', ...errorMetadata(error) }); }
    try { connection.close(); }
    catch (error) { metadata.cleanupErrors.push({ stage: 'connection-close', ...errorMetadata(error) }); }
  }
  try { store?.close(); }
  catch (error) { metadata.cleanupErrors.push({ stage: 'store-close', ...errorMetadata(error) }); }
  if (directory && !failure && metadata.cleanupErrors.length === 0) {
    try {
      // Recheck the exact owned directory before recursive removal.
      assert.equal(realpathSync.native(directory), directory);
      assert.ok(basename(directory).startsWith('council-busy-probe-'));
      rmSync(directory, { recursive: true, force: true });
      metadata.fixtureRemoved = !existsSync(directory);
      assert.equal(metadata.fixtureRemoved, true);
    } catch (error) { metadata.cleanupErrors.push({ stage: 'fixture-remove', ...errorMetadata(error) }); }
  } else if (directory) metadata.fixtureRemoved = false;
  metadata.status = failure || metadata.cleanupErrors.length ? 'failed' : 'completed';
  if (failure) metadata.failure = failure;
  process.stdout.write(`${JSON.stringify(metadata)}\n`);
  if (metadata.status !== 'completed') process.exitCode = 1;
}
