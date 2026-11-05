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

const DIAGNOSTIC_BYTES = 64 * 1024;
function diagnosticText(value, maximum = 256) {
  if (typeof value !== 'string') return undefined;
  if (/council_private_(?:chat|request|reply|response)|\"privateBody\"/i.test(value)) return '[Private protocol content omitted from endurance diagnostics.]';
  if (value.length <= maximum) return value;
  let preview = value.slice(0, maximum);
  if (/[\uD800-\uDBFF]$/.test(preview)) preview = preview.slice(0, -1);
  return preview + ' [truncated; inspect the retained fixture]';
}

/** A durable terminal outcome is more useful than a later generic entry timeout. */
export function roomSoakEntryFailure(room, phase = 'task') {
  if (!room || !['failed', 'cancelled'].includes(room.status)) return undefined;
  return new Error(`Room ${room.status} before ${phase === 'discussion' ? 'discussion' : 'task'} entry: ${diagnosticText(room.error, 2048) || 'No public room error was recorded.'}`);
}

/** Allowlisted fake-workload metadata only; never prompts, output or private bodies. */
export function roomSoakDiagnostic(input) {
  const number = value => Number.isSafeInteger(value) && value >= 0 ? value : undefined;
  const text = value => diagnosticText(value, 128);
  const rows = (value, limit) => Array.isArray(value) ? value.slice(0, limit) : [];
  const storageErrors = value => rows(value, 8).filter(entry => ['state', 'rooms'].includes(entry?.area)).map(entry => ({
    area: entry.area, phase: ['begin', 'commit', 'rollback', 'statement', 'operation'].includes(entry.phase) ? entry.phase : 'operation',
    code: number(entry.code), at: number(entry.at),
    frames: rows(entry.frames, 8).filter(frame => typeof frame === 'string' && /^(?:CouncilStore|RoomStore|RoomManager|CouncilService)\.[A-Za-z_$][A-Za-z0-9_$]{0,95}$/.test(frame)),
  }));
  const room = input.room;
  const result = { scenario: text(input.scenario), phase: text(input.phase), error: diagnosticText(input.error, 2048),
    storageErrors: storageErrors(input.storageErrors), storageErrorCount: number(input.storageErrorCount),
    room: room ? { id: text(room.id), status: text(room.status), revision: number(room.revision), callsUsed: number(room.callsUsed),
      error: diagnosticText(room.error, 2048), taskCount: room.tasks?.length,
      tasks: rows(room.tasks, 16).map(task => ({ id: text(task.id), status: text(task.status), runId: text(task.runId), error: diagnosticText(task.error) })) } : undefined,
    counts: Object.fromEntries(['runs', 'activeRuns', 'slots', 'slotWaiters', 'instances'].map(key => [key, number(input.counts?.[key])])),
    runs: rows(input.runs, 16).map(run => ({ id: text(run.id), status: text(run.status), error: diagnosticText(run.error),
      steps: rows(run.steps, 4).map(step => ({ id: text(step.id), agent: text(step.agent), status: text(step.status), error: diagnosticText(step.error) })) })),
    slots: rows(input.slots, 32).map(row => ({ token: text(row.token), instanceId: text(row.instance_id), runId: text(row.run_id), acquiredAt: number(row.acquired_at) })),
    waiters: rows(input.waiters, 32).map(row => ({ instanceId: text(row.instance_id), runId: text(row.run_id), waiterId: text(row.waiter_id) })),
    instances: rows(input.instances, 8).map(row => ({ id: text(row.id), pid: number(row.pid), heartbeatAt: number(row.heartbeat_at) })),
    workers: rows(input.workers, 4).map(worker => { const stderr = typeof worker.stderr === 'string' ? worker.stderr : '';
      return { pid: number(worker.pid), exited: Boolean(worker.exited), connected: Boolean(worker.connected),
        lastRequest: worker.lastRequest ? { operation: text(worker.lastRequest.operation), roomId: text(worker.lastRequest.roomId), request: number(worker.lastRequest.request) } : undefined,
        lastReply: worker.lastReply ? { ok: Boolean(worker.lastReply.ok), status: text(worker.lastReply.status), error: diagnosticText(worker.lastReply.error) } : undefined,
        storageErrors: storageErrors(worker.storageErrors), storageErrorCount: number(worker.storageErrorCount),
        storageErrorsOmitted: Math.max(0, (number(worker.storageErrorCount) ?? 0) - storageErrors(worker.storageErrors).length),
        stderr: { bytes: Buffer.byteLength(stderr), hasContent: Boolean(stderr),
          signals: ['state.degraded', 'process.degraded', 'database is locked', 'SQLITE_BUSY', 'SQLITE_LOCKED', 'cleanup could not be verified'].filter(value => stderr.includes(value)) } };
    }), readFailures: rows(input.readFailures, 6).filter(value => ['room', 'counts', 'runs', 'slots', 'waiters', 'instances'].includes(value.area)).map(value => ({ area: value.area, code: number(value.code) })), omitted: {}, truncated: false };
  const collections = { tasks: result.room?.tasks ?? [], runs: result.runs, slots: result.slots, waiters: result.waiters, instances: result.instances, workers: result.workers, storageErrors: result.storageErrors };
  while (Buffer.byteLength(JSON.stringify(result)) > DIAGNOSTIC_BYTES - 2048) {
    const collection = Object.values(collections).filter(value => value.length).sort((left, right) => Buffer.byteLength(JSON.stringify(right)) - Buffer.byteLength(JSON.stringify(left)))[0];
    if (!collection) throw new Error('The fixed room diagnostic metadata exceeds its byte bound.');
    collection.pop(); result.truncated = true;
  }
  for (const [name, collection] of Object.entries(collections)) {
    const original = name === 'tasks' ? room?.tasks : input[name];
    const total = name === 'waiters' ? input.counts?.slotWaiters : name === 'storageErrors' ? input.storageErrorCount : input.counts?.[name];
    result.omitted[name] = Math.max(0, (Array.isArray(original) ? original.length : 0) - collection.length, (number(total) ?? 0) - collection.length);
    if (result.omitted[name]) result.truncated = true;
  }
  if (result.workers.some(worker => worker.storageErrorsOmitted > 0)) result.truncated = true;
  return result;
}

/** Capture only SQLite lock provenance; defer IPC until synchronous transactions unwind. */
export function roomSoakTraceStorage(store, area, onError, refusalOrigin) {
  const seen = new WeakSet(), database = store.db, originalExec = database.exec, originalTransaction = store.transaction;
  const capture = (error, phase) => {
    if (!error || typeof error !== 'object' || !Number.isInteger(error.errcode) || ![5, 6].includes(error.errcode & 0xff) || seen.has(error)) return;
    seen.add(error);
    const frames = [...String(error.stack ?? '').matchAll(/\bat (?:async )?((?:CouncilStore|RoomStore|RoomManager|CouncilService)\.[A-Za-z_$][A-Za-z0-9_$]*)\b/g)].slice(0, 8).map(match => match[1]);
    let origin;
    try { origin = refusalOrigin?.(error); } catch { /* Diagnostics never replace the original failure. */ }
    if (origin?.area === area && origin.phase === 'begin' && origin.code === error.errcode) phase = 'begin';
    const value = { area, phase, code: error.errcode, at: Date.now(), frames };
    queueMicrotask(() => onError(value));
  };
  database.exec = function (sql, ...args) {
    try { return originalExec.call(this, sql, ...args); }
    catch (error) {
      const verb = typeof sql === 'string' ? /^\s*(BEGIN(?: IMMEDIATE)?|COMMIT|ROLLBACK)\s*$/i.exec(sql)?.[1]?.split(' ')[0]?.toLowerCase() : undefined;
      capture(error, verb ?? 'statement'); throw error;
    }
  };
  store.transaction = function (...args) {
    try { return originalTransaction.apply(this, args); }
    catch (error) { capture(error, 'operation'); throw error; }
  };
  for (const name of ['releaseSlot', 'cancelSlotWaiter']) {
    const operation = store[name];
    if (typeof operation !== 'function') continue;
    store[name] = function (...args) {
      try { return operation.apply(this, args); }
      catch (error) { capture(error, 'statement'); throw error; }
    };
  }
}

function failureDiagnostic(service, manager, workers, progress, error) {
  const data = { ...progress, error: error.message, workers: workers.map(worker => ({ pid: worker.child.pid, exited: worker.exited(),
    connected: worker.child.connected, lastRequest: worker.lastRequest, lastReply: worker.lastReply, stderr: worker.stderr(),
    storageErrors: worker.storageErrors, storageErrorCount: worker.storageErrorCount })), readFailures: [] };
  const read = (area, operation) => { try { return operation(); } catch (failure) { data.readFailures.push({ area, code: failure?.errcode }); return undefined; } };
  data.room = progress.roomId && read('room', () => manager.metadata(progress.roomId));
  data.counts = read('counts', () => service.store.health().counts);
  data.runs = read('runs', () => service.store.listRunSummaries(16));
  for (const [name, table, limit] of [['slots', 'slots', 32], ['waiters', 'slot_waiters', 32], ['instances', 'instances', 8]]) {
    data[name] = read(name, () => service.store.db.prepare(`SELECT * FROM ${table} LIMIT ${limit}`).all());
  }
  return roomSoakDiagnostic(data);
}

async function workerMain(project, runtime) {
  const { CouncilService, loadConfig } = await import(pathToFileURL(join(runtime, 'dist', 'index.js')).href);
  const { admissionRefusalOrigin } = await import(pathToFileURL(join(runtime, 'dist', 'admission-attempt.js')).href);
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
  const storageError = value => send({ event: 'storage_error', value });
  roomSoakTraceStorage(service.store, 'state', storageError, admissionRefusalOrigin);
  roomSoakTraceStorage(service.rooms.store, 'rooms', storageError, admissionRefusalOrigin);
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
  const worker = { child, exited: () => exited, expectedExit: false, stderr: () => stderr, storageErrors: [], storageErrorCount: 0,
    call(operation, roomId) {
      return new Promise((resolve_, reject) => {
        if (exited || !child.connected) { reject(new Error('Offline worker is not connected.')); return; }
        const id = ++request, timer = setTimeout(() => { requests.delete(id); reject(new Error(`Offline worker ${operation} timed out.`)); }, 10000);
        requests.set(id, { resolve: value => { clearTimeout(timer); resolve_(value); }, reject: error => { clearTimeout(timer); reject(error); } });
        worker.lastRequest = { operation, roomId, request: id };
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
    else if (message?.event === 'storage_error') { worker.storageErrorCount++; worker.storageErrors.push(message.value); if (worker.storageErrors.length > 8) worker.storageErrors.shift(); }
    else if (message?.reply) { worker.lastReply = { ok: message.ok, status: message.status, error: message.error }; const pending = requests.get(message.reply); requests.delete(message.reply); message.ok ? pending?.resolve(message) : pending?.reject(new Error(message.error)); }
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

async function exercise(service, workers, ledger, scenario, signal, progress = {}) {
  signal.throwIfAborted();
  const [first, second] = workers, manager = service.rooms;
  const room = manager.create({ title: 'Offline process room: ' + scenario, brief: 'Inspect only the isolated offline fixture and preserve complete evidence.',
    members: [{ agent: 'codex', role: 'lead' }, { agent: 'claude', role: 'worker' }], rounds: 2, maxCalls: 32, autoExecute: false,
    tasks: [1, 2, 3].map(index => ({ title: 'Task ' + index, brief: 'Return complete offline evidence for this task.', permission: 'read', ...(index === 3 ? { dependsOn: ['task-1'] } : {}) })) });
  progress.scenario = scenario; progress.roomId = room.id; progress.phase = 'created';
  const entries = (phase, after = 0) => ledger.slice(after).filter(entry => entry.roomId === room.id && entry.phase === phase);
  const waitEntries = async (phase, count, after = 0) => {
    progress.phase = phase + ' entry (' + count + ')'; let nextStatusCheck = 0;
    return until(() => {
      const found = entries(phase, after); if (found.length >= count) return found;
      if (performance.now() >= nextStatusCheck) {
        nextStatusCheck = performance.now() + 100;
        const failure = roomSoakEntryFailure(manager.observation(room.id), phase); if (failure) throw failure;
      }
      return false;
    }, workers, signal, phase + ' entry');
  };
  const stable = async status => {
    progress.phase = 'stable ' + status;
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
  let cycles = 0, fakeAgentCalls = 0, peakBytes = 0, fixtures = 0, workersStarted = 0, workersExited = 0, runtimeHash, harnessHash, version, failure, diagnosticWriteFailed = false;
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
    const { admissionRefusalOrigin } = await import(pathToFileURL(join(runtime, 'dist', 'admission-attempt.js')).href);
    do {
      controller.signal.throwIfAborted();
      const fixture = realpathSync.native(mkdtempSync(join(temporary, 'fixture-'))), project = join(fixture, 'project');
      mkdirSync(project); fixtures++;
      let service, manager, workers = [], fixtureFailure;
      const ledger = [], progress = { storageErrors: [], storageErrorCount: 0 };
      try {
        service = await api.CouncilService.open(api.loadConfig(project, { backend: 'mock', maxCalls: 128, maxConcurrency: 2 }), { doctor: async () => [], run: async () => { throw new Error('The observer must never dispatch an agent.'); } });
        manager = service.rooms;
        const storageError = value => { progress.storageErrorCount++; progress.storageErrors.push(value); if (progress.storageErrors.length > 8) progress.storageErrors.shift(); };
        roomSoakTraceStorage(service.store, 'state', storageError, admissionRefusalOrigin);
        roomSoakTraceStorage(manager.store, 'rooms', storageError, admissionRefusalOrigin);
        workers = [spawnWorker(project, runtime, ledger, launcher), spawnWorker(project, runtime, ledger, launcher)]; workersStarted += workers.length;
        await Promise.all(workers.map(async worker => {
          let timer;
          try { await Promise.race([worker.ready, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Offline worker startup timed out.')), 10000); })]); }
          finally { clearTimeout(timer); }
        }));
        for (const scenario of scenarios) {
          progress.scenario = scenario; progress.phase = 'room setup'; delete progress.roomId;
          await exercise(service, workers, ledger, scenario, controller.signal, progress);
          cycles++; counts[scenario]++;
          const usage = fixtureUsage(temporary); assert.ok(usage.bytes <= FIXTURE_BYTES, 'Room fixture exceeded its logical storage budget.'); peakBytes = Math.max(peakBytes, usage.bytes);
          if (performance.now() - started >= nextCheckpoint) { emit('checkpoint', { cycles, scenarios: counts, fakeAgentCalls: fakeAgentCalls + ledger.length, peakObservedLogicalFixtureBytes: peakBytes }); nextCheckpoint = performance.now() - started + options.checkpointMs; }
        }
        const health = service.health(); assert.equal(health.state.counts.activeRuns, 0); assert.equal(health.state.counts.slots, 0); assert.equal(health.state.counts.slotWaiters, 0);
      } catch (error) {
        fixtureFailure = error;
        try { emit('failure_diagnostic', { diagnostic: failureDiagnostic(service, manager, workers, progress, error) }); }
        catch (diagnosticError) { diagnosticWriteFailed = true; fixtureFailure = new Error(error.message + '; bounded diagnostic write failed: ' + diagnosticText(diagnosticError.message), { cause: error }); }
      }
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
  const interrupted = !diagnosticWriteFailed && controller.signal.aborted && (failure === controller.signal.reason || failure?.cause === controller.signal.reason);
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
