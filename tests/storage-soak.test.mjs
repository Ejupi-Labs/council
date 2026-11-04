import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, resolve } from 'node:path';
import test from 'node:test';
import { DatabaseSync } from 'node:sqlite';

const project = resolve(import.meta.dirname, '..');
const rowsFrom = output => output.trim().split(/\r?\n/).filter(Boolean).map(line => JSON.parse(line));

test('storage endurance rotates drained verified fixtures without ending the requested workload', { timeout: 30000 }, () => {
  const output = execFileSync(process.execPath, ['scripts/storage-soak.mjs', '--duration-ms', '4500', '--fixture-ms', '1000', '--workers', '2'],
    { cwd: project, encoding: 'utf8', timeout: 25000, windowsHide: true });
  const rows = rowsFrom(output);
  const summary = rows.findLast(row => row.kind === 'complete');
  assert.ok(summary);
  assert.ok(summary.elapsedMs >= 4500);
  assert.ok(summary.fixtures >= 2);
  assert.equal(summary.workersStarted, 2 * summary.fixtures);
  assert.equal(summary.workersExited, summary.workersStarted);
  assert.ok(summary.totals.completed > 0);
  assert.ok(summary.totals.cancelled > 0);
  assert.equal(summary.fixtureRemoved, true);
  assert.ok(summary.peakObservedLogicalFixtureBytes <= summary.fixtureBudgetBytes);
  assert.equal(rows.filter(row => row.kind === 'fixture-complete').length, summary.fixtures);
  assert.ok(summary.fixtureSummaries.some(fixture => fixture.rotationReason === 'fixture-age'));
  for (const fixture of rows.filter(row => row.kind === 'fixture-complete')) {
    for (const key of ['activeRuns', 'slots', 'slotWaiters', 'instances', 'unreadMessages', 'leases']) assert.equal(fixture.health.counts[key], 0, key);
    assert.equal(fixture.runtimeFrozen, true);
    assert.equal(existsSync(fixture.directory), false);
  }
});

test('failed storage fixture cleanup prevents rotation and never publishes completion', { timeout: 30000 }, t => {
  const source = [
    "import fs from 'node:fs';",
    "import { basename } from 'node:path';",
    "import { syncBuiltinESMExports } from 'node:module';",
    "const original = fs.rmSync;",
    "fs.rmSync = (path, options) => { if (basename(path).startsWith('council-storage-soak-')) throw new Error('Injected storage fixture cleanup failure.'); return original(path, options); };",
    "syncBuiltinESMExports();",
    "process.argv = [process.execPath, './scripts/storage-soak.mjs', '--duration-ms', '4500', '--fixture-ms', '1000', '--workers', '2'];",
    "await import('./scripts/storage-soak.mjs');",
  ].join('\n');
  const child = spawnSync(process.execPath, ['--input-type=module', '-e', source], { cwd: project, encoding: 'utf8', timeout: 25000, windowsHide: true });
  const rows = rowsFrom(child.stdout);
  const fixture = rows.find(row => row.kind === 'fixture-start')?.directory;
  assert.ok(fixture);
  let verified = false;
  t.after(() => {
    if (!verified) return;
    assert.equal(dirname(resolve(fixture)), realpathSync.native(tmpdir()));
    assert.ok(basename(fixture).startsWith('council-storage-soak-'));
    rmSync(fixture, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  assert.equal(child.signal, null);
  assert.equal(child.status, 1);
  assert.equal(rows.filter(row => row.kind === 'fixture-start').length, 1);
  assert.equal(rows.some(row => row.kind === 'complete'), false);
  assert.equal(rows.some(row => row.kind === 'fixture-complete'), false);
  const failure = rows.find(row => row.kind === 'failure');
  assert.match(failure.error, /Injected storage fixture cleanup failure/);
  assert.equal(failure.fixturePreserved, true);
  assert.equal(existsSync(fixture), true);
  const database = new DatabaseSync(resolve(fixture, 'state.db'), { readOnly: true });
  try {
    for (const table of ['instances', 'slots', 'slot_waiters']) assert.equal(database.prepare(`SELECT COUNT(*) AS count FROM ${table}`).get().count, 0);
    assert.equal(database.prepare("SELECT COUNT(*) AS count FROM runs WHERE status IN ('queued','running')").get().count, 0);
    assert.equal(database.prepare('PRAGMA quick_check').get().quick_check, 'ok');
  } finally { database.close(); }
  verified = true;
});
