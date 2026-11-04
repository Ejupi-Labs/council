import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { fileURLToPath } from 'node:url';
import { roomSoakDiagnostic, roomSoakEntryFailure, roomSoakTraceStorage } from '../scripts/room-soak.mjs';

const repository = realpathSync.native(fileURLToPath(new URL('../', import.meta.url))), script = realpathSync.native(fileURLToPath(new URL('../scripts/room-soak.mjs', import.meta.url)));
const events = output => output.trim().split(/\r?\n/).filter(line => line.startsWith('{')).map(line => JSON.parse(line));

test('real-process room soak verifies all scenarios and drains every worker before reporting completion', { timeout: 60000 }, () => {
  const output = execFileSync(process.execPath, [script, '--duration-ms', '1000', '--checkpoint-ms', '1000'], {
    cwd: repository, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', timeout: 45000, maxBuffer: 512 * 1024,
  });
  const rows = events(output), summary = rows.at(-1);
  assert.equal(summary.event, 'finished'); assert.equal(summary.status, 'completed');
  assert.equal(summary.cycles, 5); assert.equal(summary.fakeAgentCalls, 25); assert.equal(summary.liveProviderCalls, 0);
  assert.ok(Object.values(summary.scenarios).every(count => count === 1));
  assert.equal(summary.workersStarted, 2); assert.equal(summary.workersExited, 2); assert.equal(summary.fixtureRemoved, true);
  assert.ok(summary.peakObservedLogicalFixtureBytes > 0 && summary.peakObservedLogicalFixtureBytes < summary.fixtureBudgetBytes);
  assert.match(summary.runtimeDigest, /^[a-f0-9]{64}$/); assert.match(summary.harnessDigest, /^[a-f0-9]{64}$/);
  assert.equal(summary.runtimeDigest, rows[0].runtimeDigest); assert.equal(summary.harnessDigest, rows[0].harnessDigest);
});

test('interrupted room soak reports cancellation after process drain and preserves inspectable state', { timeout: 60000 }, () => {
  const url = new URL('../scripts/room-soak.mjs', import.meta.url).href;
  const driver = `import { roomSoak, roomSoakOptions } from ${JSON.stringify(url)};
const controller = new AbortController();
const timer = setTimeout(() => controller.abort(new DOMException('Deliberate offline soak interruption.', 'AbortError')), 1000);
try { await roomSoak(roomSoakOptions(['--duration-ms', '60000', '--checkpoint-ms', '1000']), controller.signal); }
catch (error) { process.stderr.write(error.message + '\\n'); process.exitCode = error.status === 'cancelled' ? 130 : 1; }
finally { clearTimeout(timer); }`;
  let captured;
  try { execFileSync(process.execPath, ['--input-type=module', '--eval', driver], {
    cwd: repository, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', timeout: 45000, maxBuffer: 512 * 1024,
  }); assert.fail('The explicit interruption must not report success.'); } catch (error) { captured = error; }
  assert.equal(captured.status, 130, captured.stderr);
  const rows = events(captured.stdout), summary = rows.at(-1);
  assert.equal(summary.status, 'cancelled'); assert.equal(summary.fixtureRemoved, false); assert.equal(summary.liveProviderCalls, 0);
  assert.equal(summary.workersStarted, summary.workersExited);
  assert.ok(!rows.some(row => row.status === 'completed'));
  const root = realpathSync.native(summary.fixture), parent = realpathSync.native(tmpdir());
  assert.equal(dirname(root), parent); assert.ok(basename(root).startsWith('council-room-soak-'));
  let verified = false;
  try {
    for (const name of readdirSync(root).filter(name => name.startsWith('fixture-'))) {
      const state = join(root, name, 'project', '.council', 'state.db'); if (!existsSync(state)) continue;
      const db = new DatabaseSync(state, { readOnly: true });
      try {
        assert.equal(db.prepare('PRAGMA quick_check').get().quick_check, 'ok');
        assert.equal(db.prepare('SELECT COUNT(*) AS count FROM instances').get().count, 0);
        assert.equal(db.prepare('SELECT COUNT(*) AS count FROM slots').get().count, 0);
        assert.equal(db.prepare('SELECT COUNT(*) AS count FROM slot_waiters').get().count, 0);
        assert.equal(db.prepare("SELECT COUNT(*) AS count FROM runs WHERE status IN ('queued','running')").get().count, 0);
      } finally { db.close(); }
    }
    verified = true;
  } finally { if (verified) rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); }
});

test('room soak runs through aliased entrypoint and temporary paths using canonical owned fixtures', { timeout: 60000 }, () => {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-room-soak-alias-')));
  const source = join(root, 'source'), alias = join(root, 'source-alias'), actualTemp = join(root, 'actual-temp'), aliasTemp = join(root, 'temp-alias');
  mkdirSync(source); mkdirSync(join(source, 'scripts')); mkdirSync(actualTemp);
  cpSync(join(repository, 'dist'), join(source, 'dist'), { recursive: true }); writeFileSync(join(source, 'package.json'), readFileSync(join(repository, 'package.json')));
  for (const name of ['room-soak.mjs', 'coordination-soak.mjs']) writeFileSync(join(source, 'scripts', name), readFileSync(join(repository, 'scripts', name)));
  symlinkSync(source, alias, process.platform === 'win32' ? 'junction' : 'dir'); symlinkSync(actualTemp, aliasTemp, process.platform === 'win32' ? 'junction' : 'dir');
  let verified = false;
  try {
    const output = execFileSync(process.execPath, [join(alias, 'scripts', 'room-soak.mjs'), '--duration-ms', '1000', '--checkpoint-ms', '1000'], {
      cwd: alias, env: { ...process.env, TMPDIR: aliasTemp, TEMP: aliasTemp, TMP: aliasTemp }, windowsHide: true,
      stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', timeout: 45000, maxBuffer: 512 * 1024,
    });
    const rows = events(output), summary = rows.at(-1);
    assert.equal(summary?.status, 'completed', 'An aliased script entrypoint must actually run, not silently exit without workload evidence.');
    assert.equal(summary.cycles, 5); assert.equal(summary.workersStarted, 2); assert.equal(summary.workersExited, 2); assert.equal(summary.fixtureRemoved, true);
    assert.equal(readdirSync(actualTemp).length, 0, 'The native temporary target must contain no remaining owned fixture.');
    verified = true;
  } finally {
    if (verified) {
      assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-room-soak-alias-'));
      for (const path of [alias, aliasTemp]) assert.ok(realpathSync.native(path).startsWith(root + (process.platform === 'win32' ? '\\' : '/')), 'Alias cleanup must stay inside the owned fixture.');
      rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
    }
  }
});

test('invalid room-soak options fail before starting a workload or creating a progress log', () => {
  let captured;
  try { execFileSync(process.execPath, [script, '--duration-ms', '0'], { cwd: repository, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', timeout: 10000 });
    assert.fail('A zero-duration workload must be rejected.'); } catch (error) { captured = error; }
  assert.equal(captured.status, 1); assert.equal(captured.stdout, ''); assert.match(captured.stderr, /Duration must be/);
});

test('SQLite lock provenance is deferred until transaction unwind and excludes SQL and error bodies', async () => {
  for (const phase of ['begin', 'commit', 'operation']) {
    const secret = 'PRIVATE_SQL_AND_ERROR_BODY', captured = [];
    const failure = Object.assign(new Error(secret), { errcode: 5 });
    failure.stack = 'Error: ' + secret + '\n    at CouncilStore.createRun (file:///fixture/store.js:1:1)\n    at RoomManager.launch (file:///fixture/rooms.js:1:1)';
    class ProbeStore {
      constructor() { this.db = { isTransaction: false, exec: sql => {
        if (sql === 'BEGIN IMMEDIATE') { if (phase === 'begin') throw failure; this.db.isTransaction = true; }
        if (sql === 'COMMIT' && phase === 'commit') throw failure;
        if (sql === 'COMMIT' || sql === 'ROLLBACK') this.db.isTransaction = false;
      } }; }
      transaction() {
        this.db.exec('BEGIN IMMEDIATE');
        try { if (phase === 'operation') throw failure; this.db.exec('COMMIT'); }
        catch (error) { this.db.exec('ROLLBACK'); throw error; }
      }
    }
    const store = new ProbeStore();
    roomSoakTraceStorage(store, 'state', value => { assert.equal(store.db.isTransaction, false); captured.push(value); });
    assert.throws(() => store.transaction(), error => error === failure);
    assert.equal(captured.length, 0, 'Diagnostics must not send while a writer is held.');
    await Promise.resolve();
    assert.equal(captured.length, 1, 'One native error propagating through exec and transaction is recorded once.');
    assert.equal(captured[0].area, 'state'); assert.equal(captured[0].phase, phase); assert.equal(captured[0].code, 5);
    assert.deepEqual(captured[0].frames, ['CouncilStore.createRun', 'RoomManager.launch']);
    assert.equal(JSON.stringify(captured).includes(secret), false);
    assert.equal(Object.hasOwn(captured[0], 'sql'), false); assert.equal(Object.hasOwn(captured[0], 'message'), false);
  }
});

test('entry failure detection preserves public terminal errors without misclassifying expected live states', () => {
  assert.match(roomSoakEntryFailure({ status: 'failed', error: 'database is locked' }).message, /^Room failed before task entry: database is locked$/);
  assert.match(roomSoakEntryFailure({ status: 'cancelled', error: 'Cancellation was accepted.' }).message, /Cancellation was accepted/);
  assert.match(roomSoakEntryFailure({ status: 'failed', error: 'database is locked' }, 'discussion').message, /^Room failed before discussion entry: database is locked$/);
  for (const status of ['idle', 'ready', 'running', 'paused', 'completed', 'needs_attention']) assert.equal(roomSoakEntryFailure({ status }), undefined);
});

test('failure diagnostics bound complete metadata and exclude prompts, output, private bodies and raw stderr', () => {
  const secret = 'PRIVATE_RAW_USER_CONTENT', control = '\u0001'.repeat(4000);
  const row = { id: control, status: 'failed', runId: control, error: 'database is locked', input: { task: secret }, output: secret, body: secret };
  const diagnostic = roomSoakDiagnostic({ scenario: 'cancel', phase: 'task entry (2)', error: 'database is locked',
    room: { ...row, tasks: Array.from({ length: 100 }, () => ({ ...row })), privateBody: secret },
    runs: Array.from({ length: 100 }, () => ({ ...row, steps: [{ ...row, error: '{"council_private_reply":{"body":"' + secret + '"}}' }] })),
    slots: Array.from({ length: 100 }, () => ({ token: control, instance_id: control, run_id: control, body: secret })),
    waiters: Array.from({ length: 100 }, () => ({ instance_id: control, run_id: control, waiter_id: control })),
    instances: Array.from({ length: 100 }, () => ({ id: control, pid: 12345 })),
    workers: [{ pid: 12345, connected: true, exited: false, stderr: secret + '\nstate.degraded\ndatabase is locked\n', output: secret }],
    counts: { runs: 150, activeRuns: 0, slots: 100, slotWaiters: 100, instances: 100, privateBody: secret },
    readFailures: [{ area: 'runs', code: 5, body: secret }, { area: secret, code: 6 }],
    storageErrorCount: 100, storageErrors: Array.from({ length: 100 }, () => ({ area: 'state', phase: 'begin', code: 5, at: 123,
      sql: secret, message: secret, frames: ['CouncilStore.createRun', secret] })),

  });
  const json = JSON.stringify(diagnostic);
  assert.ok(Buffer.byteLength(json) < 64 * 1024); assert.equal(json.includes(secret), false);
  assert.equal(diagnostic.error, 'database is locked'); assert.equal(diagnostic.truncated, true);
  assert.ok(diagnostic.omitted.tasks > 0); assert.equal(diagnostic.omitted.runs, 150 - diagnostic.runs.length);
  assert.deepEqual(diagnostic.readFailures, [{ area: 'runs', code: 5 }]);
  assert.equal(diagnostic.storageErrors.length, 8); assert.equal(diagnostic.omitted.storageErrors, 92);
  assert.deepEqual(diagnostic.storageErrors[0].frames, ['CouncilStore.createRun']);
  assert.equal(diagnostic.workers[0].stderr.hasContent, true);
  assert.ok(diagnostic.workers[0].stderr.signals.includes('state.degraded'));
});

test('a real child pre-dispatch failure reports its cause and pre-cleanup state while draining every worker', { timeout: 60000 }, () => {
  for (const phase of ['task', 'discussion']) {
  const parent = realpathSync.native(tmpdir()), source = realpathSync.native(mkdtempSync(join(parent, 'council-room-soak-failure-')));
  mkdirSync(join(source, 'scripts')); cpSync(join(repository, 'dist'), join(source, 'dist'), { recursive: true });
  writeFileSync(join(source, 'package.json'), readFileSync(join(repository, 'package.json')));
  writeFileSync(join(source, 'scripts', 'coordination-soak.mjs'), readFileSync(join(repository, 'scripts', 'coordination-soak.mjs')));
  const original = readFileSync(script, 'utf8'), site = "  process.on('disconnect', () => { void service.close()";
  assert.equal(original.split(site).length, 2);
  const condition = phase === 'task' ? "input.task.startsWith('Room task ')" : "input.mode === 'parallel'";
  const injection = "  const start = service.start.bind(service); service.start = input => { if (" + condition + ") throw Object.assign(new Error('Deliberate offline pre-dispatch failure.'), { errcode: 5 }); return start(input); };\n";
  writeFileSync(join(source, 'scripts', 'room-soak.mjs'), original.replace(site, injection + site));
  let captured, retained, verified = false;
  try {
    try { execFileSync(process.execPath, [join(source, 'scripts', 'room-soak.mjs'), '--duration-ms', '1000', '--checkpoint-ms', '1000'], {
      cwd: source, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', timeout: 45000, maxBuffer: 512 * 1024,
    }); assert.fail('Injected pre-dispatch failure must fail the soak.'); } catch (error) { captured = error; }
    assert.equal(captured.status, 1, captured.stderr);
    const rows = events(captured.stdout), summary = rows.at(-1), diagnostic = rows.find(row => row.event === 'failure_diagnostic')?.diagnostic;
    assert.equal(summary.status, 'failed'); assert.ok(summary.error.includes('Room failed before ' + phase + ' entry: Deliberate offline pre-dispatch failure'));
    assert.equal(summary.error.includes('Timed out: ' + phase + ' entry'), false); assert.equal(summary.fakeAgentCalls, phase === 'task' ? 2 : 0);
    assert.equal(summary.workersStarted, 2); assert.equal(summary.workersExited, 2); assert.equal(summary.fixtureRemoved, false);
    assert.equal(diagnostic.room.status, 'failed'); assert.equal(diagnostic.room.error, 'Deliberate offline pre-dispatch failure.');
    assert.ok(diagnostic.phase.startsWith(phase + ' entry')); assert.equal(diagnostic.workers.length, 2);
    assert.ok(diagnostic.workers.every(worker => worker.exited === false), 'Failure state must be captured before worker shutdown.');
    assert.ok(diagnostic.workers.some(worker => worker.lastReply?.status === (phase === 'task' ? 'running' : 'discussing')));
    assert.ok(diagnostic.workers.some(worker => worker.storageErrors.some(error => error.area === 'rooms' && error.phase === 'operation' && error.code === 5)));
    assert.equal(diagnostic.runs.length, phase === 'task' ? 1 : 0); if (phase === 'task') assert.equal(diagnostic.runs[0].status, 'completed');
    assert.ok(diagnostic.room.tasks.every(task => task.runId === undefined));
    retained = realpathSync.native(summary.fixture); assert.equal(dirname(retained), parent); assert.ok(basename(retained).startsWith('council-room-soak-'));
    for (const name of readdirSync(retained).filter(name => name.startsWith('fixture-'))) {
      const database = new DatabaseSync(join(retained, name, 'project', '.council', 'state.db'), { readOnly: true });
      try {
        for (const table of ['instances', 'slots', 'slot_waiters']) assert.equal(database.prepare('SELECT COUNT(*) AS count FROM ' + table).get().count, 0);
        assert.equal(database.prepare("SELECT COUNT(*) AS count FROM runs WHERE status IN ('queued','running')").get().count, 0);
      } finally { database.close(); }
    }
    verified = true;
  } finally {
    if (verified) {
      assert.equal(dirname(realpathSync.native(source)), parent); assert.ok(basename(source).startsWith('council-room-soak-failure-'));
      rmSync(retained, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
      rmSync(source, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
    }
  }
  }
});

test('a diagnostic write failure outranks interruption after actual spawned workers drain', { timeout: 60000 }, () => {
  const url = new URL('../scripts/room-soak.mjs', import.meta.url).href;
  const driver = `import fs from 'node:fs';import childProcess from 'node:child_process';import {syncBuiltinESMExports} from 'node:module';
import {roomSoak,roomSoakOptions} from ${JSON.stringify(url)};
const controller=new AbortController(),append=fs.appendFileSync,spawn=childProcess.spawn;let timer;
fs.appendFileSync=function(path,data,...rest){if(String(data).includes('"event":"failure_diagnostic"'))throw new Error('Injected diagnostic write failure.');return append(path,data,...rest);};
childProcess.spawn=function(...args){const child=spawn(...args);if(Array.isArray(args[1])&&args[1].includes('--worker')&&!timer)timer=setTimeout(()=>controller.abort(new DOMException('Deliberate interruption after spawn.','AbortError')),50);return child;};syncBuiltinESMExports();
try{await roomSoak(roomSoakOptions(['--duration-ms','60000','--checkpoint-ms','1000']),controller.signal);}
catch(error){process.stderr.write(error.message+'\\n');process.exitCode=error.status==='cancelled'?130:1;}
finally{clearTimeout(timer);fs.appendFileSync=append;childProcess.spawn=spawn;syncBuiltinESMExports();}`;
  let captured, root, verified = false;
  try {
    try { execFileSync(process.execPath, ['--input-type=module', '--eval', driver], {
      cwd: repository, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', timeout: 45000, maxBuffer: 512 * 1024,
    }); assert.fail('Diagnostic write failure must fail the interrupted soak.'); } catch (error) { captured = error; }
    assert.equal(captured.status, 1, captured.stderr);
    const summary = events(captured.stdout).at(-1);
    assert.equal(summary.status, 'failed'); assert.match(summary.error, /Deliberate interruption after spawn/); assert.match(summary.error, /Injected diagnostic write failure/);
    assert.equal(summary.workersStarted, 2); assert.equal(summary.workersExited, 2); assert.equal(summary.fixtureRemoved, false);
    root = realpathSync.native(summary.fixture); const parent = realpathSync.native(tmpdir());
    assert.equal(dirname(root), parent); assert.ok(basename(root).startsWith('council-room-soak-'));
    for (const name of readdirSync(root).filter(name => name.startsWith('fixture-'))) {
      const database = new DatabaseSync(join(root, name, 'project', '.council', 'state.db'), { readOnly: true });
      try {
        for (const table of ['instances', 'slots', 'slot_waiters']) assert.equal(database.prepare('SELECT COUNT(*) AS count FROM ' + table).get().count, 0);
        assert.equal(database.prepare("SELECT COUNT(*) AS count FROM runs WHERE status IN ('queued','running')").get().count, 0);
      } finally { database.close(); }
    }
    verified = true;
  } finally { if (verified) rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); }
});
