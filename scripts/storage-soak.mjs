import assert from 'node:assert/strict';
import { fork } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { appendFileSync, existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, unlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, relative, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { DatabaseSync } from 'node:sqlite';

const filename = fileURLToPath(import.meta.url);
const project = resolve(dirname(filename), '..');
const isBusy = error => typeof error?.errcode === 'number' && [5, 6].includes(error.errcode & 0xff);
const FIXTURE_BUDGET_BYTES = 256 * 1024 * 1024;
const RETAINED_BACKUP_LIMIT = 2;
const CHECKPOINT_INTERVAL_MS = 30000;

function logicalBytes(directory) {
  let bytes = 0;
  for (const name of readdirSync(directory)) {
    const path = join(directory, name);
    const info = lstatSync(path);
    assert.ok(!info.isSymbolicLink(), 'Storage fixture measurement cannot follow symlinks.');
    if (info.isDirectory()) bytes += logicalBytes(path);
    else if (info.isFile()) bytes += info.size;
    else throw new Error('Storage fixture contains an unexpected filesystem entry.');
  }
  return bytes;
}

function fingerprint() {
  const runtime = join(project, 'dist');
  const files = [];
  const visit = directory => {
    for (const name of readdirSync(directory).sort()) {
      const path = join(directory, name);
      const info = lstatSync(path);
      if (info.isDirectory()) visit(path);
      else if (info.isFile() && name.endsWith('.js')) files.push(path);
    }
  };
  visit(runtime);
  assert.ok(files.length, 'Build dist before running the storage soak.');
  const hash = createHash('sha256');
  for (const path of [join(project, 'package.json'), ...files]) {
    hash.update(relative(project, path).replaceAll('\\', '/')).update('\0').update(readFileSync(path)).update('\0');
  }
  return { runtimeDigest: hash.digest('hex'), harnessDigest: createHash('sha256').update(readFileSync(filename)).digest('hex'), runtimeFiles: files.length };
}

function assertFrozen(expected) {
  const current = fingerprint();
  assert.equal(current.runtimeDigest, expected.runtimeDigest, 'Compiled runtime changed during the storage soak; this is not a frozen-build result.');
  assert.equal(current.harnessDigest, expected.harnessDigest, 'Storage soak harness changed while running.');
}

async function loadRuntime(expected) {
  const before = fingerprint();
  if (expected) assert.deepEqual([before.runtimeDigest, before.harnessDigest], [expected.runtimeDigest, expected.harnessDigest], 'Worker runtime differs from the coordinator frozen build.');
  const [{ CouncilStore }, { VERSION }] = await Promise.all([import('../dist/store.js'), import('../dist/version.js')]);
  assertFrozen(before);
  const manifest = JSON.parse(readFileSync(join(project, 'package.json'), 'utf8'));
  assert.equal(VERSION, manifest.version, 'Compiled and package versions differ; rebuild before running the storage soak.');
  return { CouncilStore, metadata: { ...before, version: VERSION, node: process.version, sqlite: process.versions.sqlite, platform: process.platform, arch: process.arch } };
}

function options(arguments_) {
  const values = { '--duration-ms': 10000, '--workers': 3 };
  const seen = new Set();
  for (let index = 0; index < arguments_.length; index += 2) {
    const name = arguments_[index], value = arguments_[index + 1];
    if (!Object.hasOwn(values, name)) throw new Error(`Unknown storage soak option: ${name}.`);
    if (seen.has(name)) throw new Error(`Storage soak option ${name} was supplied more than once.`);
    if (value === undefined || !/^\d+$/.test(value)) throw new Error(`Storage soak option ${name} requires an integer value.`);
    seen.add(name); values[name] = Number(value);
  }
  if (!Number.isInteger(values['--duration-ms']) || values['--duration-ms'] < 1000 || values['--duration-ms'] > 3600000 ||
      !Number.isInteger(values['--workers']) || values['--workers'] < 2 || values['--workers'] > 8) throw new Error('Use --duration-ms between 1000 and 3600000 and --workers between 2 and 8.');
  return { duration: values['--duration-ms'], workerCount: values['--workers'] };
}

async function retry(operation, metrics, deadline = performance.now() + 10000) {
  for (;;) {
    try { return operation(); }
    catch (error) {
      if (!isBusy(error) || performance.now() >= deadline) throw error;
      metrics.busyRetries++;
      await delay(5 + Math.floor(Math.random() * 15));
    }
  }
}

async function worker(directory, durationMs, ordinal, expected) {
  const { CouncilStore, metadata } = await loadRuntime(expected);
  const store = new CouncilStore(directory);
  const instance = `storage-soak-${process.pid}-${ordinal}`;
  const until = performance.now() + durationMs;
  const metrics = { completed: 0, cancelled: 0, busyRetries: 0, leaseConflicts: 0, slotWaits: 0, maximumCycleMs: 0 };
  let stopping = false;
  process.on('message', message => { if (message?.stop) stopping = true; });
  process.on('disconnect', () => { stopping = true; });
  await retry(() => store.registerInstance(instance, process.pid, 2), metrics);
  process.send?.({ kind: 'ready', instance, runtime: metadata });
  try {
    while (!stopping && performance.now() < until) {
      const started = performance.now();
      const now = new Date().toISOString();
      const run = { id: randomUUID(), status: 'queued', createdAt: now, updatedAt: now,
        input: { mode: 'delegate', agents: ['codex'], task: 'Offline storage coordination fixture.', permission: 'read' }, steps: [] };
      let slot;
      const waiterId = randomUUID();
      let leased = false;
      await retry(() => store.createRun(run, instance), metrics);
      try {
        const deadline = performance.now() + 15000;
        while (!(slot = await retry(() => store.acquireSlot(instance, run.id, 2, waiterId), metrics))) {
          if (performance.now() >= deadline) throw new Error('Global slot wait exceeded the fixture deadline.');
          metrics.slotWaits++;
          await delay(10);
        }
        await retry(() => store.saveRun({ ...run, status: 'running' }), metrics);
        for (;;) {
          try { await retry(() => store.reserve('same-agent-label', ['shared/file.ts'], 10000, run.id), metrics); leased = true; break; }
          catch (error) {
            if (!/Reservation conflict/.test(error.message) || performance.now() >= deadline) throw error;
            metrics.leaseConflicts++;
            await delay(10);
          }
        }
        await delay(5);
        const lease = store.leases().find(value => value.paths.includes('shared/file.ts'));
        assert.equal(lease?.runId, run.id, 'A different run acquired this run-scoped lease.');
        await retry(() => store.putContext(instance, JSON.stringify({ runId: run.id, iteration: metrics.completed + metrics.cancelled }), instance), metrics);
        await retry(() => store.sendMessage({ from: instance, to: 'observer', subject: 'Offline evidence', body: run.id, runId: run.id }), metrics);
        await retry(() => store.appendEvent(run.id, 'step.finished', { agent: 'codex', result: { agent: 'codex', status: 'completed', output: 'x'.repeat(4096), durationMs: 1 } }), metrics);
        const cancel = (metrics.completed + metrics.cancelled) % 7 === 0;
        if (cancel) await retry(() => store.requestCancellation(run.id), metrics);
        assert.equal(await retry(() => store.release('same-agent-label', ['shared/file.ts'], run.id), metrics), 1);
        leased = false;
        await retry(() => store.releaseSlot(slot, instance), metrics); slot = undefined;
        const terminal = await retry(() => store.finalizeRun({ ...run, status: 'completed', output: 'Offline fixture complete.' }), metrics);
        assert.equal(terminal.status, cancel ? 'cancelled' : 'completed');
        assert.equal(store.getRunSummary(run.id).status, terminal.status);
        metrics[cancel ? 'cancelled' : 'completed']++;
      } catch (error) {
        try { await retry(() => store.finalizeRun({ ...run, status: 'failed', error: error.message }), metrics); } catch {}
        throw error;
      } finally {
        await retry(() => store.cancelSlotWaiter(instance, waiterId), metrics);
        if (leased) await retry(() => store.release('same-agent-label', ['shared/file.ts'], run.id), metrics);
        if (slot) await retry(() => store.releaseSlot(slot, instance), metrics);
      }
      metrics.maximumCycleMs = Math.max(metrics.maximumCycleMs, Math.round(performance.now() - started));
      if ((metrics.completed + metrics.cancelled) % 100 === 0) process.send?.({ kind: 'progress', instance, metrics });
      if ((metrics.completed + metrics.cancelled) % 200 === 0) await retry(() => store.prune({ before: new Date().toISOString(), keepLatest: 200, dryRun: false }), metrics);
      await delay(15);
    }
    assertFrozen(expected);
    await retry(() => store.unregisterInstance(instance), metrics);
    process.send?.({ kind: 'done', instance, metrics });
  } finally { store.close(); }
}

async function coordinator() {
  const { duration, workerCount } = options(process.argv.slice(2));
  const { CouncilStore, metadata } = await loadRuntime();
  const temporaryRoot = realpathSync.native(tmpdir());
  const directory = realpathSync.native(mkdtempSync(join(temporaryRoot, 'council-storage-soak-')));
  const identity = lstatSync(directory, { bigint: true });
  let observer;
  let failure;
  let finalHealth;
  let backupChecks = 0;
  let measuredBytes = 0;
  let peakObservedBytes = 0;
  const retained = [];
  const removed = [];
  const started = performance.now();
  const reports = new Map();
  const ready = new Map();
  const errors = [];
  const ownMetrics = { busyRetries: 0 };
  const children = [];
  const completions = [];
  const logDirectory = join(project, '.council', 'refinement');
  mkdirSync(logDirectory, { recursive: true });
  const log = join(logDirectory, `storage-soak-${new Date().toISOString().replaceAll(':', '-')}-${process.pid}.jsonl`);
  const record = value => { const line = JSON.stringify({ at: new Date().toISOString(), ...value }); appendFileSync(log, line + '\n'); process.stdout.write(line + '\n'); };
  const verifyManagedDirectory = () => {
    const current = lstatSync(directory, { bigint: true });
    assert.ok(current.isDirectory() && !current.isSymbolicLink(), 'Refusing to operate on a replaced storage fixture.');
    assert.equal(current.dev, identity.dev); assert.equal(current.ino, identity.ino);
    assert.equal(realpathSync.native(directory), directory, 'Storage fixture path no longer resolves to its original target.');
    assert.equal(dirname(directory), temporaryRoot, 'Storage fixture is outside its canonical temporary root.');
    assert.equal(realpathSync.native(temporaryRoot), temporaryRoot);
    assert.ok(basename(directory).startsWith('council-storage-soak-'));
  };
  const measure = () => {
    verifyManagedDirectory();
    measuredBytes = logicalBytes(directory);
    peakObservedBytes = Math.max(peakObservedBytes, measuredBytes);
    assert.ok(measuredBytes <= FIXTURE_BUDGET_BYTES, `Storage fixture exceeds its ${FIXTURE_BUDGET_BYTES}-byte budget; preserve it for inspection.`);
    return measuredBytes;
  };
  const storageEvidence = () => ({ backupChecks, retainedBackups: retained.length, retainedBackupFiles: retained.map(value => basename(value.path)), removedBackupFiles: [...removed], logicalFixtureBytes: measuredBytes, peakObservedLogicalFixtureBytes: peakObservedBytes, fixtureBudgetBytes: FIXTURE_BUDGET_BYTES });
  const checkpointFiles = () => readdirSync(directory).filter(name => /^checkpoint-\d{6}\.db$/.test(name)).sort();
  const deleteCheckpoint = value => {
    verifyManagedDirectory();
    assert.equal(dirname(value.path), directory);
    assert.match(basename(value.path), /^checkpoint-\d{6}\.db$/);
    assert.equal(realpathSync.native(value.path), value.path);
    const current = lstatSync(value.path, { bigint: true });
    assert.ok(current.isFile() && !current.isSymbolicLink());
    assert.equal(current.dev, value.dev); assert.equal(current.ino, value.ino);
    unlinkSync(value.path);
    assert.equal(existsSync(value.path), false);
  };
  try {
    record({ kind: 'start', durationMs: duration, workers: workerCount, directory, evidenceFile: log, runtime: metadata, fixtureBudgetBytes: FIXTURE_BUDGET_BYTES, retainedBackupLimit: RETAINED_BACKUP_LIMIT, checkpointIntervalMs: CHECKPOINT_INTERVAL_MS });
    observer = new CouncilStore(directory);
    measure();
    record({ kind: 'state-ready', schemaVersion: observer.health().schemaVersion, ...storageEvidence() });
    for (let index = 0; index < workerCount; index++) {
      const child = fork(filename, ['--worker', directory, String(duration), String(index), metadata.runtimeDigest, metadata.harnessDigest], { execArgv: [], windowsHide: true, stdio: ['ignore', 'ignore', 'pipe', 'ipc'] });
      let diagnostics = '';
      child.stderr.on('data', chunk => { diagnostics = (diagnostics + chunk).slice(-8192); });
      child.on('message', message => {
        if (message?.instance && message.metrics) reports.set(message.instance, message.metrics);
        if (message?.kind === 'ready') ready.set(message.instance, message.runtime);
      });
      children.push(child);
      completions.push(new Promise(resolveChild => {
        child.once('error', error => { errors.push(error.message); resolveChild(); });
        child.once('close', code => { if (code !== 0) errors.push(`Worker ${index} exited ${code}: ${diagnostics}`); resolveChild(); });
      }));
    }
    const until = performance.now() + duration;
    let nextCheckpointAt = started + CHECKPOINT_INTERVAL_MS;
    while (performance.now() < until && errors.length === 0) {
      await delay(1000);
      const health = observer.health();
      assert.ok(health.counts.slots <= 2, 'Workspace concurrency limit was exceeded.');
      for (const message of observer.inbox('observer')) await retry(() => observer.acknowledge(message.id, 'observer'), ownMetrics);
      measure();
      if (performance.now() >= nextCheckpointAt) {
        nextCheckpointAt += CHECKPOINT_INTERVAL_MS;
        assertFrozen(metadata);
        // Main DB plus WAL bounds the next snapshot conservatively before adding a third copy.
        assert.ok(measuredBytes + health.databaseBytes + health.walBytes <= FIXTURE_BUDGET_BYTES, 'The next checkpoint could exceed the storage fixture budget; preserve existing state.');
        const snapshot = join(directory, `checkpoint-${String(backupChecks + 1).padStart(6, '0')}.db`);
        await observer.backupTo(snapshot);
        const inspection = new DatabaseSync(snapshot);
        try { assert.equal(inspection.prepare('PRAGMA quick_check').get().quick_check, 'ok'); }
        finally { inspection.close(); }
        backupChecks++;
        const info = lstatSync(snapshot, { bigint: true });
        retained.push({ path: snapshot, dev: info.dev, ino: info.ino });
        measure();
        while (retained.length > RETAINED_BACKUP_LIMIT) {
          const oldest = retained[0];
          deleteCheckpoint(oldest);
          retained.shift(); removed.push(basename(oldest.path));
        }
        measure();
        const present = checkpointFiles();
        assert.deepEqual(present, retained.map(value => basename(value.path)).sort());
        record({ kind: 'checkpoint', elapsedMs: Math.round(performance.now() - started), health, workers: Object.fromEntries(reports), runtimeFrozen: true, checkpointFilesPresent: present, ...storageEvidence() });
      }
    }
    for (const child of children) if (child.connected) child.send({ stop: true }, () => { /* A worker may finish at its own monotonic deadline first. */ });
    const drainDeadline = performance.now() + 30000;
    while (children.some(child => child.exitCode === null && child.signalCode === null)) {
      if (performance.now() >= drainDeadline) throw new Error('Storage workers did not drain within 30 seconds.');
      await delay(25);
    }
    await Promise.all(completions);
    assert.deepEqual(errors, []);
    assert.equal(ready.size, workerCount, 'Every worker must verify the frozen runtime before admission.');
    for (;;) {
      const messages = observer.inbox('observer');
      if (!messages.length) break;
      for (const message of messages) await retry(() => observer.acknowledge(message.id, 'observer'), ownMetrics);
    }
    assert.equal(observer.health().counts.activeRuns, 0);
    assert.equal(observer.health().counts.slots, 0);
    assert.equal(observer.health().counts.slotWaiters, 0);
    assert.equal(observer.health().counts.instances, 0);
    assert.equal(observer.health().counts.unreadMessages, 0);
    assert.equal(observer.leases().length, 0);
    assert.deepEqual(observer.checkIntegrity(), ['ok']);
    assertFrozen(metadata);
    finalHealth = observer.health();
    measure();
    record({ kind: 'fixture-verified', elapsedMs: Math.round(performance.now() - started), directory, checkpointFilesPresent: checkpointFiles(), ...storageEvidence() });
  } catch (error) {
    failure = error;
  } finally {
    for (const child of children) if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL');
    await Promise.allSettled(completions);
    try { observer?.close(); } catch (error) { failure ??= error; }
    if (!failure) {
      try {
        measure();
        rmSync(directory, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
      } catch (error) { failure = error; }
    }
    if (failure) {
      try { record({ kind: 'failure', elapsedMs: Math.round(performance.now() - started), error: failure.message, directory, fixturePreserved: existsSync(directory), workers: Object.fromEntries(reports), workerErrors: errors, runtime: metadata, ...storageEvidence() }); }
      catch (error) { process.stderr.write(`Unable to write storage soak evidence: ${error.message}\n`); }
      throw new Error(`Storage soak failed: ${failure.message} Fixture preserved at ${directory}. Evidence: ${log}`, { cause: failure });
    }
    record({ kind: 'complete', elapsedMs: Math.round(performance.now() - started), health: finalHealth, workers: Object.fromEntries(reports), workerRuntimes: Object.fromEntries(ready), observer: ownMetrics, directory, fixtureRemoved: true, runtimeFrozen: true, runtime: metadata, ...storageEvidence() });
  }
}

if (process.argv[2] === '--worker') {
  const expected = { runtimeDigest: process.argv[6], harnessDigest: process.argv[7] };
  worker(process.argv[3], Number(process.argv[4]), Number(process.argv[5]), expected).then(() => process.disconnect?.()).catch(error => { process.stderr.write(error.stack + '\n'); process.exitCode = 1; process.disconnect?.(); });
} else coordinator().catch(error => { process.stderr.write(error.stack + '\n'); process.exitCode = 1; });
