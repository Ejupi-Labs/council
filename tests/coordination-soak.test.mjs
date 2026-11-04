import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { execFileSync, spawnSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { DatabaseSync } from 'node:sqlite';
import { basename, dirname, join, resolve } from 'node:path';
import { coordinationOptions, fixtureUsage, runtimeDigest } from '../scripts/coordination-soak.mjs';

test('coordination endurance inputs reject ambiguous or unbounded runs before starting work', () => {
  assert.deepEqual(coordinationOptions(['--duration-ms', '1000', '--checkpoint-ms', '100', '--rotation-cycles', '1']), { durationMs: 1000, checkpointMs: 100, rotationCycles: 1 });
  for (const invalid of [['--duration-ms'], ['--duration-ms', '0'], ['--duration-ms', '3600001'], ['--duration-ms', '1000', '--duration-ms', '2000'],
    ['--checkpoint-ms', '99'], ['--rotation-cycles', '0'], ['--rotation-cycles', 'Infinity'], ['--live-agents', '1']]) assert.throws(() => coordinationOptions(invalid));
});

test('coordination evidence fingerprints runtime contents and measures actual nested fixture bytes', t => {
  const parent = realTemporaryParent(), directory = mkdtempSync(join(parent, 'council-coordination-harness-test-'));
  t.after(() => { assert.equal(dirname(resolve(directory)), parent); assert.ok(basename(directory).startsWith('council-coordination-harness-test-')); rmSync(directory, { recursive: true, force: true }); });
  mkdirSync(join(directory, 'dist')); writeFileSync(join(directory, 'package.json'), '{"version":"test"}'); writeFileSync(join(directory, 'dist', 'sample.js'), 'export const value = 1;');
  const first = runtimeDigest(directory), measured = fixtureUsage(directory);
  assert.equal(measured.files, 2); assert.equal(measured.bytes, Buffer.byteLength('{"version":"test"}export const value = 1;'));
  writeFileSync(join(directory, 'dist', 'sample.js'), 'export const value = 2;'); assert.notEqual(runtimeDigest(directory), first);
});

function realTemporaryParent() { return resolve(tmpdir()); }

test('interruption drains both SQLite hosts while close failures remain failed', { timeout: 30000 }, t => {
  for (const injectCloseFailure of [false, true]) {
  const script = [
    "import { DatabaseSync } from 'node:sqlite';",
    "let injected = false, armed = false; const originalClose = DatabaseSync.prototype.close;",
    `DatabaseSync.prototype.close = function (...args) { const value = originalClose.apply(this, args); if (injected && ${injectCloseFailure}) throw new Error('Injected SQLite host close failure.'); return value; };`,
    "const originalWrite = process.stdout.write;",
    "process.stdout.write = function (...args) { const value = originalWrite.apply(this, args); if (!armed && String(args[0]).includes('\\\"event\\\":\\\"started\\\"')) { armed = true; setImmediate(() => { injected = true; process.emit('SIGINT'); }); } return value; };",
    "const { coordinationSoak, coordinationOptions } = await import('./scripts/coordination-soak.mjs');",
    "try { await coordinationSoak(coordinationOptions(['--duration-ms', '1000'])); } catch (error) { process.stderr.write(error.message + '\\n'); process.exitCode = error.status === 'cancelled' ? 130 : 1; }",
  ].join('\n');
  const child = spawnSync(process.execPath, ['--input-type=module', '-e', script], { cwd: resolve(import.meta.dirname, '..'), encoding: 'utf8', timeout: 25000, windowsHide: true });
  const rows = child.stdout.trim().split(/\r?\n/).map(line => JSON.parse(line));
  const fixture = rows.find(row => row.event === 'started')?.fixture;
  if (fixture) t.after(() => { assert.equal(dirname(resolve(fixture)), realTemporaryParent()); assert.ok(basename(fixture).startsWith('council-coordination-soak-')); rmSync(fixture, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  assert.equal(child.signal, null); assert.equal(child.status, injectCloseFailure ? 1 : 130);
  const summary = rows.findLast(row => row.event === 'finished');
  assert.equal(summary.status, injectCloseFailure ? 'failed' : 'cancelled'); assert.equal(summary.interrupted, true);
  if (injectCloseFailure) assert.match(summary.error, /Injected SQLite host close failure/);
  else {
    const db = new DatabaseSync(join(fixture, 'workspace-1', '.council', 'state.db'), { readOnly: true });
    try {
      for (const table of ['instances', 'slots', 'slot_waiters']) assert.equal(db.prepare(`SELECT COUNT(*) AS count FROM ${table}`).get().count, 0);
      assert.equal(db.prepare("SELECT COUNT(*) AS count FROM runs WHERE status IN ('queued','running')").get().count, 0);
    } finally { db.close(); }
  }
  assert.equal(summary.fixture, fixture); assert.equal(existsSync(fixture), true);
  }
});

test('failed final fixture cleanup cannot publish a completed coordination summary', { timeout: 30000 }, t => {
  const script = [
    "import fs from 'node:fs';",
    "import { basename } from 'node:path';",
    "import { syncBuiltinESMExports } from 'node:module';",
    "const original = fs.rmSync;",
    "fs.rmSync = (path, options) => { if (basename(path).startsWith('council-coordination-soak-')) throw new Error('Injected final fixture cleanup failure.'); return original(path, options); };",
    "syncBuiltinESMExports();",
    "const { coordinationSoak, coordinationOptions } = await import('./scripts/coordination-soak.mjs');",
    "try { await coordinationSoak(coordinationOptions(['--duration-ms', '1000'])); } catch (error) { process.stderr.write(error.message + '\\n'); process.exitCode = 1; }",
  ].join('\n');
  const child = spawnSync(process.execPath, ['--input-type=module', '-e', script], { cwd: resolve(import.meta.dirname, '..'), encoding: 'utf8', timeout: 25000, windowsHide: true });
  const rows = child.stdout.trim().split(/\r?\n/).map(line => JSON.parse(line));
  const fixture = rows.find(row => row.event === 'started')?.fixture;
  if (fixture) t.after(() => { assert.equal(dirname(resolve(fixture)), realTemporaryParent()); assert.ok(basename(fixture).startsWith('council-coordination-soak-')); rmSync(fixture, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  assert.equal(child.signal, null); assert.equal(child.status, 1);
  const summary = rows.findLast(row => row.event === 'finished');
  assert.equal(summary.status, 'failed'); assert.match(summary.error, /Injected final fixture cleanup failure/);
  assert.equal(summary.fixture, fixture); assert.equal(existsSync(fixture), true);
  assert.equal(rows.some(row => row.event === 'finished' && row.status === 'completed'), false);
});

test('the bounded offline coordination harness verifies complete lifecycles and removes its successful fixture', { timeout: 30000 }, () => {
  const output = execFileSync(process.execPath, ['scripts/coordination-soak.mjs', '--duration-ms', '1000', '--checkpoint-ms', '1000', '--rotation-cycles', '2'],
    { cwd: resolve(import.meta.dirname, '..'), encoding: 'utf8', timeout: 25000, windowsHide: true });
  const rows = output.trim().split(/\r?\n/).map(line => JSON.parse(line));
  const summary = rows.findLast(row => row.event === 'finished');
  assert.equal(summary.status, 'completed'); assert.equal(summary.liveProviderCalls, 0);
  assert.ok(summary.cycles > 0); assert.equal(summary.completedRuns, 6 * summary.cycles); assert.equal(summary.failedRuns, 2 * summary.cycles); assert.equal(summary.cancelledRuns, summary.cycles);
  assert.equal(summary.fakeAgentCalls, 13 * summary.cycles); assert.equal(summary.completedRooms, summary.cycles); assert.equal(summary.pausedRooms, summary.cycles);
  assert.ok(summary.snapshots >= 2); assert.ok(summary.hotBackupWrites > 0); assert.ok(summary.peakObservedLogicalFixtureBytes < summary.fixtureBudgetBytes);
  assert.equal(existsSync(rows.find(row => row.event === 'started').fixture), false);
});
