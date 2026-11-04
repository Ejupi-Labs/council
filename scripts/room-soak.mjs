import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { appendFileSync, cpSync, existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { fixtureUsage, runtimeDigest } from './coordination-soak.mjs';

const filename = realpathSync.native(fileURLToPath(import.meta.url)), repository = resolve(dirname(filename), '..');
const FIXTURE_BYTES = 128 * 1024 * 1024;
const scenarios = ['pause-resume', 'changed-instructions', 'cancel', 'task-failure', 'owner-exit'];

export function roomSoakOptions(arguments_) {
  const options = { '--duration-ms': 60000, '--checkpoint-ms': 30000 }, seen = new Set();
  for (let index = 0; index < arguments_.length; index += 2) {
    const key = arguments_[index], value = arguments_[index + 1];
    if (!Object.hasOwn(options, key) || seen.has(key) || value === undefined || !/^\d+$/.test(value)) throw new Error('Use unique --duration-ms and --checkpoint-ms integer options.');
    seen.add(key); options[key] = Number(value);
  }
  if (!Number.isSafeInteger(options['--duration-ms']) || options['--duration-ms'] < 1000 || options['--duration-ms'] > 3600000
    || !Number.isSafeInteger(options['--checkpoint-ms']) || options['--checkpoint-ms'] < 100 || options['--checkpoint-ms'] > 3600000) throw new Error('Duration must be 1000..3600000 ms and checkpoint 100..3600000 ms.');
  return { durationMs: options['--duration-ms'], checkpointMs: options['--checkpoint-ms'] };
}

async function workerMain(project, runtime) {
  const { CouncilService, loadConfig } = await import(pathToFileURL(join(runtime, 'dist', 'index.js')).href);
  const gates = new Map();
  let roomId, sequence = 0;
  const send = value => { if (process.connected) process.send(value); };
  const runner = { doctor: async () => [], run: async request => {
    const ballot = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
    const data = ballot ? JSON.parse(ballot[1]) : undefined;
    const taskId = data ? undefined : /Room task (task-\d+):/.exec(request.prompt)?.[1];
    assert.ok(data || taskId, 'The offline worker received an unexpected task.');
    const gateId = String(++sequence);
    const result = await new Promise(resolve_ => {
      const abort = () => resolve_({ status: 'cancelled' });
      request.signal.addEventListener('abort', abort, { once: true });
      gates.set(gateId, value => { request.signal.removeEventListener('abort', abort); resolve_(value); });
      send({ event: 'entered', roomId: data?.roomId ?? roomId, phase: data ? 'discussion' : 'task', taskId, gateId, agent: request.agent });
      if (request.signal.aborted) abort();
    });
    gates.delete(gateId);
    const status = request.signal.aborted ? 'cancelled' : result.status ?? 'completed';
    return { agent: request.agent, status, durationMs: 1,
      output: data && status === 'completed' ? JSON.stringify({ vote: 'approve', rationale: 'The explicit offline tasks have inspectable acceptance evidence.',
        assignments: Object.fromEntries(data.tasks.map(task => [task.id, 'codex'])) }) : `Complete offline evidence for ${taskId ?? 'discussion'}: ${status}.`,
      ...(status === 'failed' ? { error: 'Deliberate offline task failure.' } : {}) };
  } };
  const service = await CouncilService.open(loadConfig(project, { backend: 'mock', maxCalls: 128, maxConcurrency: 2 }), runner);
  process.on('disconnect', () => { void service.close().catch(error => { process.stderr.write(`${error.stack}\n`); process.exitCode = 1; }); });
  process.on('message', message => {
    if (message?.operation === 'release') { gates.get(message.gateId)?.({ status: message.status }); return; }
    void (async () => {
      try {
        if (message.operation === 'close') {
          await service.close();
          if (process.connected) process.send({ reply: message.request, ok: true }, () => process.disconnect());
          return;
        }
        roomId = message.roomId;
        const result = message.operation === 'start' ? service.rooms.startDiscussion(roomId)
          : message.operation === 'execute' ? service.rooms.execute(roomId)
          : message.operation === 'resume' ? service.rooms.resume(roomId) : undefined;
        if (!result) throw new Error('Unknown offline worker operation.');
        send({ reply: message.request, ok: true, id: result.id, status: result.status });
      } catch (error) { send({ reply: message.request, ok: false, error: error.message }); }
    })();
  });
  send({ event: 'ready' });
}

function spawnWorker(project, runtime, ledger, launcher) {
  const child = spawn(process.execPath, [launcher, '--worker', project, runtime], { cwd: project, windowsHide: true, stdio: ['ignore', 'ignore', 'pipe', 'ipc'] });
  const requests = new Map();
  let request = 0, exited = false, stderr = '', readyResolve, readyReject;
  const ready = new Promise((resolve_, reject) => { readyResolve = resolve_; readyReject = reject; });
  const worker = { child, exited: () => exited, expectedExit: false, stderr: () => stderr,
    call(operation, roomId) {
      return new Promise((resolve_, reject) => {
        if (exited || !child.connected) { reject(new Error('Offline worker is not connected.')); return; }
        const id = ++request, timer = setTimeout(() => { requests.delete(id); reject(new Error(`Offline worker ${operation} timed out.`)); }, 10000);
        requests.set(id, { resolve: value => { clearTimeout(timer); resolve_(value); }, reject: error => { clearTimeout(timer); reject(error); } });
        child.send({ operation, roomId, request: id }, error => { if (error) { requests.get(id)?.reject(error); requests.delete(id); } });
      });
    },
    release(entry, status) { if (child.connected) child.send({ operation: 'release', gateId: entry.gateId, status }); },
    ready,
  };
  child.stderr.on('data', chunk => { if (stderr.length < 65536) stderr += chunk.toString('utf8').slice(0, 65536 - stderr.length); });
  child.on('message', message => {
    if (message?.event === 'ready') readyResolve();
    else if (message?.event === 'entered') ledger.push({ ...message, worker });
    else if (message?.reply) { const pending = requests.get(message.reply); requests.delete(message.reply); message.ok ? pending?.resolve(message) : pending?.reject(new Error(message.error)); }
  });
  child.on('error', error => { readyReject(error); for (const value of requests.values()) value.reject(error); requests.clear(); });
  worker.exit = new Promise(resolve_ => child.once('exit', (code, signal) => {
    exited = true; const error = new Error(`Offline worker exited (${code ?? signal}). ${stderr}`);
    readyReject(error); for (const value of requests.values()) value.reject(error); requests.clear(); resolve_({ code, signal });
  }));
  return worker;
}

async function stopWorker(worker, kill = false) {
  if (worker.exited()) return;
  worker.expectedExit = true;
  let failure;
  if (!kill) { try { await worker.call('close'); } catch (error) { failure = error; } }
  if (kill || failure) worker.child.kill('SIGKILL');
  const exited = async () => {
    let timer;
    try { return await Promise.race([worker.exit, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Offline worker termination could not be verified.')), 10000); })]); }
    finally { clearTimeout(timer); }
  };
  try { await exited(); }
  catch (error) {
    failure ??= error;
    // A close acknowledgment alone is insufficient: terminate and observe an
    // owner that retained a handle instead of leaving it behind on failure.
    worker.child.kill('SIGKILL'); await exited();
  }
  if (failure) throw failure;
}

async function until(condition, workers, signal, label) {
  const deadline = performance.now() + 10000;
  for (;;) {
    signal.throwIfAborted();
    for (const worker of workers) if (worker.exited() && !worker.expectedExit) throw new Error(`Unexpected offline worker exit. ${worker.stderr()}`);
    try { const value = condition(); if (value) return value; }
    catch (error) { const code = error?.errcode; if (typeof code !== 'number' || ![5, 6].includes(code & 0xff)) throw error; }
    if (performance.now() >= deadline) throw new Error(`Timed out: ${label}.`);
    await delay(20, undefined, { signal });
  }
}

async function exercise(service, workers, ledger, scenario, signal) {
  signal.throwIfAborted();
  const [first, second] = workers, manager = service.rooms;
  const room = manager.create({ title: 'Offline process room: ' + scenario, brief: 'Inspect only the isolated offline fixture and preserve complete evidence.',
    members: [{ agent: 'codex', role: 'lead' }, { agent: 'claude', role: 'worker' }], rounds: 2, maxCalls: 32, autoExecute: false,
    tasks: [1, 2, 3].map(index => ({ title: 'Task ' + index, brief: 'Return complete offline evidence for this task.', permission: 'read', ...(index === 3 ? { dependsOn: ['task-1'] } : {}) })) });
  const entries = (phase, after = 0) => ledger.slice(after).filter(entry => entry.roomId === room.id && entry.phase === phase);
  const waitEntries = async (phase, count, after = 0) => until(() => { const found = entries(phase, after); return found.length >= count && found; }, workers, signal, phase + ' entry');
  const stable = async status => {
    const result = await manager.wait(room.id, 10000, signal);
    assert.equal(result.status, status, result.error);
    return result;
  };
  await first.call('start', room.id);
  for (const entry of await waitEntries('discussion', 2)) entry.worker.release(entry);
  await stable('ready');
  await first.call('execute', room.id);
  const initial = await waitEntries('task', 2);
  const one = initial.find(entry => entry.taskId === 'task-1'), two = initial.find(entry => entry.taskId === 'task-2');
  assert.ok(one && two);
  let final;
  if (scenario === 'pause-resume') {
    manager.pause(room.id);
    first.release(one); first.release(two);
    const paused = await stable('paused');
    assert.deepEqual(paused.tasks.map(task => task.status), ['completed', 'completed', 'assigned']);
    assert.equal(entries('task').length, 2);
    const resumed = await Promise.allSettled(workers.map(worker => worker.call('resume', room.id)));
    assert.equal(resumed.filter(result => result.status === 'fulfilled').length, 1, 'Only one process may claim a paused room.');
    const third = (await waitEntries('task', 3)).find(entry => entry.taskId === 'task-3'); third.worker.release(third);
    final = await stable('completed'); assert.equal(final.callsUsed, 5);
  } else if (scenario === 'changed-instructions') {
    manager.postMessage(room.id, { author: 'Offline human', body: 'Retain completed evidence and explicitly discuss the remaining task again.' });
    first.release(one); first.release(two);
    const changed = await stable('needs_attention');
    assert.deepEqual(changed.tasks.map(task => task.status), ['completed', 'completed', 'pending']);
    assert.equal(entries('task').length, 2);
    const offset = ledger.length;
    await second.call('start', room.id);
    for (const entry of await waitEntries('discussion', 2, offset)) entry.worker.release(entry);
    await stable('ready'); await second.call('execute', room.id);
    const third = (await waitEntries('task', 3)).find(entry => entry.taskId === 'task-3'); third.worker.release(third);
    final = await stable('completed'); assert.equal(final.callsUsed, 7);
  } else if (scenario === 'cancel') {
    manager.cancel(room.id); final = await stable('cancelled');
    assert.ok(final.tasks.every(task => task.status === 'cancelled')); assert.equal(entries('task').length, 2);
  } else if (scenario === 'task-failure') {
    first.release(one, 'failed');
    await until(() => manager.metadata(room.id).tasks[0].status === 'failed', workers, signal, 'retained task failure');
    assert.equal(entries('task').length, 2); first.release(two);
    final = await stable('failed'); assert.deepEqual(final.tasks.map(task => task.status), ['failed', 'completed', 'cancelled']);
  } else {
    first.release(one);
    await waitEntries('task', 3);
    await stopWorker(first, true);
    final = await stable('failed');
    service.store.recoverOrphans();
    assert.deepEqual(final.tasks.map(task => task.status), ['completed', 'failed', 'failed']);
    assert.equal(service.getRun(final.tasks[0].runId).status, 'completed');
    for (const task of final.tasks.slice(1)) assert.equal(service.getRun(task.runId).status, 'failed');
    assert.match(final.error, /owning process exited/);
  }
  const taskEntries = entries('task');
  for (const id of ['task-1', 'task-2', 'task-3']) assert.ok(taskEntries.filter(entry => entry.taskId === id).length <= 1, 'A task was replayed.');
  const history = [];
  let cursor;
  do { const page = manager.messagePage(room.id, 100, cursor); history.push(...page.messages); cursor = page.nextCursor; } while (cursor !== null);
  for (const task of final.tasks.filter(task => task.status === 'completed')) assert.ok(history.some(message => message.runId === task.runId && message.body.includes('Complete offline evidence')));
  return { roomId: room.id, status: final.status, calls: entries('discussion').length + taskEntries.length };
}

export async function roomSoak(options, signal) {
  const evidence = join(repository, '.council', 'refinement'); mkdirSync(evidence, { recursive: true });
  const log = join(evidence, `room-soak-${new Date().toISOString().replaceAll(':', '-')}.jsonl`);
  const emit = (event, value) => { const line = JSON.stringify({ event, at: new Date().toISOString(), ...value }); appendFileSync(log, line + '\n'); process.stdout.write(line + '\n'); };
  const controller = new AbortController(), stop = () => controller.abort(new DOMException('Room soak interrupted.', 'AbortError'));
  const externalStop = () => controller.abort(signal.reason);
  signal?.addEventListener('abort', externalStop, { once: true }); if (signal?.aborted) externalStop();
  process.once('SIGINT', stop); process.once('SIGTERM', stop);
  const started = performance.now(), counts = Object.fromEntries(scenarios.map(name => [name, 0]));
  const temporaryBase = realpathSync.native(tmpdir()), temporary = realpathSync.native(mkdtempSync(join(temporaryBase, 'council-room-soak-'))), runtime = join(temporary, 'runtime');
  let cycles = 0, fakeAgentCalls = 0, peakBytes = 0, fixtures = 0, workersStarted = 0, workersExited = 0, runtimeHash, harnessHash, version, failure;
  let nextCheckpoint = options.checkpointMs;
  try {
    controller.signal.throwIfAborted();
    mkdirSync(runtime);
    const harness = join(temporary, 'harness'); mkdirSync(harness);
    const captureHash = createHash('sha256');
    for (const name of ['room-soak.mjs', 'coordination-soak.mjs']) {
      const source = join(repository, 'scripts', name), bytes = readFileSync(source);
      writeFileSync(join(harness, name), bytes);
      assert.ok(bytes.equals(readFileSync(source)), 'Harness changed while capturing it.');
      captureHash.update(name).update('\0').update(bytes).update('\0');
    }
    harnessHash = captureHash.digest('hex');
    const launcher = join(harness, 'room-soak.mjs');
    const sourceHash = runtimeDigest(join(repository, 'dist')), packageBytes = readFileSync(join(repository, 'package.json'));
    cpSync(join(repository, 'dist'), join(runtime, 'dist'), { recursive: true }); writeFileSync(join(runtime, 'package.json'), packageBytes);
    assert.equal(runtimeDigest(join(repository, 'dist')), sourceHash, 'Build changed while capturing the runtime.');
    assert.equal(runtimeDigest(join(runtime, 'dist')), sourceHash);
    assert.ok(packageBytes.equals(readFileSync(join(repository, 'package.json'))));
    runtimeHash = runtimeDigest(runtime); version = JSON.parse(packageBytes).version;
    emit('started', { options, runtimeDigest: runtimeHash, harnessDigest: harnessHash, version, liveProviderCalls: 0 });
    const api = await import(pathToFileURL(join(runtime, 'dist', 'index.js')).href);
    do {
      controller.signal.throwIfAborted();
      const fixture = realpathSync.native(mkdtempSync(join(temporary, 'fixture-'))), project = join(fixture, 'project');
      mkdirSync(project); fixtures++;
      let service, workers = [], fixtureFailure;
      const ledger = [];
      try {
        service = await api.CouncilService.open(api.loadConfig(project, { backend: 'mock', maxCalls: 128, maxConcurrency: 2 }), { doctor: async () => [], run: async () => { throw new Error('The observer must never dispatch an agent.'); } });
        workers = [spawnWorker(project, runtime, ledger, launcher), spawnWorker(project, runtime, ledger, launcher)]; workersStarted += workers.length;
        await Promise.all(workers.map(async worker => {
          let timer;
          try { await Promise.race([worker.ready, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Offline worker startup timed out.')), 10000); })]); }
          finally { clearTimeout(timer); }
        }));
        for (const scenario of scenarios) {
          await exercise(service, workers, ledger, scenario, controller.signal);
          cycles++; counts[scenario]++;
          const usage = fixtureUsage(temporary); assert.ok(usage.bytes <= FIXTURE_BYTES, 'Room fixture exceeded its logical storage budget.'); peakBytes = Math.max(peakBytes, usage.bytes);
          if (performance.now() - started >= nextCheckpoint) { emit('checkpoint', { cycles, scenarios: counts, fakeAgentCalls: fakeAgentCalls + ledger.length, peakObservedLogicalFixtureBytes: peakBytes }); nextCheckpoint = performance.now() - started + options.checkpointMs; }
        }
        const health = service.health(); assert.equal(health.state.counts.activeRuns, 0); assert.equal(health.state.counts.slots, 0); assert.equal(health.state.counts.slotWaiters, 0);
      } catch (error) { fixtureFailure = error; }
      finally {
        const results = await Promise.allSettled(workers.map(worker => stopWorker(worker)));
        for (const result of results) if (result.status === 'rejected') fixtureFailure = fixtureFailure
          ? new AggregateError([fixtureFailure, result.reason], 'Offline room work and worker cleanup failed.') : result.reason;
        workersExited += workers.filter(worker => worker.exited()).length; fakeAgentCalls += ledger.length;
        try { service?.store.recoverOrphans(); } catch (error) { fixtureFailure = fixtureFailure
          ? new AggregateError([fixtureFailure, error], 'Offline room work and state recovery failed.') : error; }
        try { await service?.close(); } catch (error) { fixtureFailure = fixtureFailure
          ? new AggregateError([fixtureFailure, error], 'Offline room work and service cleanup failed.') : error; }
      }
      if (fixtureFailure) throw fixtureFailure;
      for (const name of ['state.db', 'rooms.sqlite']) {
        const db = new DatabaseSync(join(project, '.council', name), { readOnly: true });
        try { assert.equal(db.prepare('PRAGMA quick_check').get().quick_check, 'ok'); if (name === 'state.db') assert.equal(db.prepare('SELECT COUNT(*) AS count FROM instances').get().count, 0); }
        finally { db.close(); }
      }
      const resolved = realpathSync.native(fixture); assert.equal(dirname(resolved), realpathSync.native(temporary)); assert.ok(basename(resolved).startsWith('fixture-')); assert.ok(!lstatSync(fixture).isSymbolicLink());
      rmSync(resolved, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
    } while (performance.now() - started < options.durationMs);
  } catch (error) { failure = error; }
  finally { process.off('SIGINT', stop); process.off('SIGTERM', stop); signal?.removeEventListener('abort', externalStop); }
  if (!failure && controller.signal.aborted) failure = controller.signal.reason;
  let fixtureRemoved = false;
  if (!failure) {
    try {
      const resolved = realpathSync.native(temporary); assert.equal(dirname(resolved), temporaryBase); assert.ok(basename(resolved).startsWith('council-room-soak-')); assert.ok(!lstatSync(temporary).isSymbolicLink());
      rmSync(resolved, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); fixtureRemoved = true;
    } catch (error) { failure = error; }
  }
  const interrupted = controller.signal.aborted && (failure === controller.signal.reason || failure?.cause === controller.signal.reason);
  const status = interrupted ? 'cancelled' : failure ? 'failed' : 'completed';
  const summary = { status, version, runtimeDigest: runtimeHash, harnessDigest: harnessHash, options, elapsedMs: Math.round(performance.now() - started), cycles, scenarios: counts, fixtures,
    workersStarted, workersExited, fakeAgentCalls, liveProviderCalls: 0, fixtureBudgetBytes: FIXTURE_BYTES, peakObservedLogicalFixtureBytes: peakBytes, fixtureRemoved, log,
    ...(failure ? { error: failure.message, fixture: temporary } : {}) };
  writeFileSync(log.replace(/\.jsonl$/, '.summary.json'), JSON.stringify(summary, null, 2) + '\n'); emit('finished', summary);
  if (failure) { const error = new Error(`Room soak ${status}: ${failure.message}. Evidence: ${log}. Fixture: ${temporary}`, { cause: failure }); error.status = status; throw error; }
  return summary;
}

if (process.argv[1] && existsSync(resolve(process.argv[1])) && realpathSync.native(resolve(process.argv[1])) === filename) {
  try {
    if (process.argv[2] === '--worker') await workerMain(process.argv[3], process.argv[4]);
    else { const result = await roomSoak(roomSoakOptions(process.argv.slice(2))); if (result.status !== 'completed') process.exitCode = 130; }
  } catch (error) { process.stderr.write(`${error.stack}\n`); process.exitCode = error.status === 'cancelled' ? 130 : 1; }
}
