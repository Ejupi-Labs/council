import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, readdirSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import test from 'node:test';
import { CouncilStore } from '../dist/store.js';

function fixture(t, large = true) {
  const temporaryBase = realpathSync(tmpdir());
  const directory = mkdtempSync(join(temporaryBase, 'council-hot-backup-'));
  const store = new CouncilStore(directory);
  const writer = new DatabaseSync(join(directory, 'state.db'));
  writer.exec('PRAGMA busy_timeout=250; CREATE TABLE backup_counter(id INTEGER PRIMARY KEY,value INTEGER NOT NULL); INSERT INTO backup_counter VALUES(1,0)');
  const now = new Date().toISOString();
  if (large) store.saveRun({ id: 'large-backup', status: 'completed', createdAt: now, updatedAt: now,
    input: { mode: 'delegate', task: 'Offline pinned backup fixture.', agents: ['codex'], permission: 'read' }, steps: [], output: 'x'.repeat(8 * 1024 * 1024) });
  t.after(() => {
    writer.close(); store.close();
    const target = realpathSync(directory);
    assert.equal(target, directory);
    assert.equal(dirname(target), temporaryBase);
    assert.ok(basename(target).startsWith('council-hot-backup-'));
    rmSync(target, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { directory, store, writer, write: writer.prepare('UPDATE backup_counter SET value=value+1 WHERE id=1') };
}

function checkpoint(writer) {
  const result = writer.prepare('PRAGMA wal_checkpoint(TRUNCATE)').get();
  assert.equal(result.busy, 0, 'Backup must release its snapshot reader so the WAL can be checkpointed.');
}

function fakeElapsedClock() {
  const descriptor = Object.getOwnPropertyDescriptor(performance, 'now');
  let calls = 0;
  Object.defineProperty(performance, 'now', { configurable: true, value: () => ++calls === 1 ? 0 : 60001 });
  return {
    calls: () => calls,
    restore() {
      if (descriptor) Object.defineProperty(performance, 'now', descriptor);
      else delete performance.now;
    },
  };
}

test('incremental backups retain one snapshot while foreign commits occur at every progress yield', async t => {
  const { directory, store, writer, write } = fixture(t);
  const controller = new AbortController();
  const nativeCheck = controller.signal.throwIfAborted.bind(controller.signal);
  let checks = 0;
  // backupTo checks the supplied signal at each native progress yield. Force a commit at every yield.
  controller.signal.throwIfAborted = () => {
    nativeCheck();
    if (++checks === 1) return;
    write.run();
    store.putContext('application-remains-writable', String(checks), 'fixture');
    assert.equal(store.db.isTransaction, false, 'A pinned backup must not hold the application connection in a transaction.');
    assert.throws(() => store.close(), /Wait for active Council backups/);
    if (checks > 100) throw new Error('Backup kept restarting after foreign commits.');
  };
  const target = join(directory, 'consistent.db');
  await store.backupTo(target, controller.signal);
  assert.ok(checks > 10, 'The fixture must exercise multiple cancellable backup batches.');
  assert.ok(writer.prepare('SELECT value FROM backup_counter WHERE id=1').get().value > 10);
  const snapshot = new DatabaseSync(target, { readOnly: true });
  try {
    assert.equal(snapshot.prepare('SELECT value FROM backup_counter WHERE id=1').get().value, 0, 'Later committed writes must not move the pinned backup snapshot.');
    assert.equal(snapshot.prepare("SELECT COUNT(*) AS count FROM context WHERE key='application-remains-writable'").get().count, 0);
    assert.equal(snapshot.prepare('PRAGMA quick_check').get().quick_check, 'ok');
  } finally { snapshot.close(); }
  checkpoint(writer);
  await assert.rejects(store.backupTo(target), /already exists/);
  assert.ok(readdirSync(directory).every(name => !name.startsWith('.council-backup-')));
});

test('cancelling a pinned backup releases its reader and retains the original cancellation error', async t => {
  const { directory, store, writer, write } = fixture(t);
  const controller = new AbortController();
  const reason = new Error('Expected pinned backup cancellation.');
  const nativeCheck = controller.signal.throwIfAborted.bind(controller.signal);
  let checks = 0;
  controller.signal.throwIfAborted = () => {
    if (++checks === 2) { write.run(); controller.abort(reason); }
    nativeCheck();
  };
  const target = join(directory, 'cancelled.db');
  await assert.rejects(store.backupTo(target, controller.signal), error => error === reason);
  assert.equal(checks, 2);
  assert.equal(existsSync(target), false);
  assert.equal(store.db.isTransaction, false);
  checkpoint(writer);
  store.putContext('after-cancel', 'Reader drained.', 'fixture');
  assert.ok(readdirSync(directory).every(name => !name.startsWith('.council-backup-')));
  assert.deepEqual(store.checkIntegrity(), ['ok']);
  const existing = join(directory, 'existing.txt');
  writeFileSync(existing, 'Preserve explicit destinations.');
  await assert.rejects(store.backupTo(existing), /already exists/);
});

test('a single-batch backup cannot publish after its elapsed deadline', async t => {
  const { directory, store, writer } = fixture(t, false);
  assert.ok(writer.prepare('PRAGMA page_count').get().page_count < 128, 'This fixture must finish in one native batch without progress callbacks.');
  const target = join(directory, 'late.db');
  const clock = fakeElapsedClock();
  try { await assert.rejects(store.backupTo(target), /60-second deadline/); }
  finally { clock.restore(); }
  assert.equal(clock.calls(), 2, 'The elapsed deadline must be checked after the no-progress final batch.');
  assert.equal(existsSync(target), false);
  assert.ok(readdirSync(directory).every(name => !name.startsWith('.council-backup-')));
  checkpoint(writer);
  store.putContext('after-deadline', 'Reader drained.', 'fixture');
});

test('post-copy cancellation keeps its original reason ahead of an elapsed deadline', async t => {
  const { directory, store, writer } = fixture(t, false);
  assert.ok(writer.prepare('PRAGMA page_count').get().page_count < 128);
  const controller = new AbortController();
  const reason = new Error('Expected cancellation after the final native batch.');
  const nativeCheck = controller.signal.throwIfAborted.bind(controller.signal);
  let checks = 0;
  controller.signal.throwIfAborted = () => { if (++checks === 2) controller.abort(reason); nativeCheck(); };
  const target = join(directory, 'late-cancelled.db');
  const clock = fakeElapsedClock();
  try { await assert.rejects(store.backupTo(target, controller.signal), error => error === reason); }
  finally { clock.restore(); }
  assert.equal(checks, 2);
  assert.equal(clock.calls(), 1, 'Cancellation must be observed before the post-copy deadline check.');
  assert.equal(existsSync(target), false);
  assert.ok(readdirSync(directory).every(name => !name.startsWith('.council-backup-')));
  checkpoint(writer);
});
