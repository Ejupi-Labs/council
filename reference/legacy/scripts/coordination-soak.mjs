import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { appendFileSync, cpSync, existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, relative, resolve } from 'node:path';
import { performance } from 'node:perf_hooks';
import { DatabaseSync } from 'node:sqlite';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath, pathToFileURL } from 'node:url';

const filename = fileURLToPath(import.meta.url);
const repository = resolve(dirname(filename), '..');
const FIXTURE_BUDGET_BYTES = 256 * 1024 * 1024;
const MAX_FILES = 20000;

export function coordinationOptions(arguments_) {
  const values = { '--duration-ms': 60000, '--checkpoint-ms': 30000, '--rotation-cycles': 20 };
  const seen = new Set();
  for (let index = 0; index < arguments_.length; index += 2) {
    const name = arguments_[index], value = arguments_[index + 1];
    if (!Object.hasOwn(values, name) || seen.has(name) || value === undefined || !/^\d+$/.test(value)) throw new Error('Use unique --duration-ms, --checkpoint-ms and --rotation-cycles integer options.');
    seen.add(name); values[name] = Number(value);
  }
  if (!Number.isSafeInteger(values['--duration-ms']) || values['--duration-ms'] < 1000 || values['--duration-ms'] > 3600000
    || !Number.isSafeInteger(values['--checkpoint-ms']) || values['--checkpoint-ms'] < 100 || values['--checkpoint-ms'] > 3600000
    || !Number.isSafeInteger(values['--rotation-cycles']) || values['--rotation-cycles'] < 1 || values['--rotation-cycles'] > 1000) throw new Error('Duration must be 1000..3600000 ms, checkpoint 100..3600000 ms and rotation 1..1000 cycles.');
  return { durationMs: values['--duration-ms'], checkpointMs: values['--checkpoint-ms'], rotationCycles: values['--rotation-cycles'] };
}

export function fixtureUsage(directory) {
  let bytes = 0, files = 0;
  const visit = path => {
    for (const name of readdirSync(path)) {
      const target = join(path, name), info = lstatSync(target);
      assert.ok(!info.isSymbolicLink(), 'Coordination fixture must not contain links.');
      if (info.isDirectory()) visit(target);
      else { assert.ok(info.isFile(), 'Unexpected fixture entry.'); bytes += info.size; files++; }
      assert.ok(files <= MAX_FILES && bytes <= FIXTURE_BUDGET_BYTES, 'Coordination fixture exceeded its logical storage budget.');
    }
  };
  visit(directory); return { bytes, files };
}

export function runtimeDigest(directory) {
  const hash = createHash('sha256');
  const visit = path => {
    for (const name of readdirSync(path).sort()) {
      const target = join(path, name), info = lstatSync(target);
      assert.ok(!info.isSymbolicLink());
      if (info.isDirectory()) visit(target);
      else if (name.endsWith('.js') || target === join(directory, 'package.json')) hash.update(relative(directory, target).replaceAll('\\', '/')).update('\0').update(readFileSync(target)).update('\0');
    }
  };
  visit(directory); return hash.digest('hex');
}

function removeOwned(path, parent, prefix) {
  const target = realpathSync.native(path);
  assert.equal(dirname(target), realpathSync.native(parent));
  assert.ok(basename(target).startsWith(prefix));
  assert.ok(!lstatSync(path).isSymbolicLink());
  rmSync(target, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
}

function gate() {
  let release, enter;
  return { entered: new Promise(resolve => { enter = resolve; }), done: new Promise(resolve => { release = resolve; }),
    enter: () => enter(), release: () => release(), claimed: false };
}

export async function coordinationSoak(options) {
  const temporaryBase = realpathSync.native(tmpdir()), temporary = realpathSync.native(mkdtempSync(join(temporaryBase, 'council-coordination-soak-')));
  const runtime = join(temporary, 'runtime'), snapshots = join(temporary, 'snapshots');
  const evidenceDir = join(repository, '.council', 'refinement'); mkdirSync(evidenceDir, { recursive: true }); mkdirSync(snapshots);
  const stamp = new Date().toISOString().replaceAll(/[:.]/g, '-'), log = join(evidenceDir, `coordination-soak-${stamp}.jsonl`);
  const started = performance.now(), control = new AbortController(), retained = [], counts = { cycles: 0, completedRuns: 0, failedRuns: 0, cancelledRuns: 0,
    completedRooms: 0, pausedRooms: 0, privateExchanges: 0, advisorExchanges: 0, protectedDispatches: 0, executiveReports: 0, snapshots: 0, hotBackupWrites: 0, rotations: 0, fakeAgentCalls: 0, peakCalls: 0 };
  let services = [], workspace, pauseGate, cancelGate, activeCalls = 0, peakFixtureBytes = 0, failure, cleanupFailure, interrupted = false, digest, version;
  let lastCheckpoint = -Infinity, workloadStarted, generationCounts;
  const emit = (event, data = {}) => { const row = { event, at: new Date().toISOString(), elapsedMs: Math.round(performance.now() - started), ...data }; appendFileSync(log, JSON.stringify(row) + '\n'); process.stdout.write(JSON.stringify(row) + '\n'); };
  const stop = () => { interrupted = true; control.abort(new Error('Coordination soak interrupted.')); pauseGate?.release(); cancelGate?.release(); for (const service of services) void service.close().catch(() => {}); };
  process.once('SIGINT', stop); process.once('SIGTERM', stop);
  const usage = () => { const measured = fixtureUsage(temporary); peakFixtureBytes = Math.max(peakFixtureBytes, measured.bytes); return measured; };
  const countedRuns = new Set();
  const countRun = (run, expected) => { assert.equal(run.status, expected, run.error); assert.ok(!countedRuns.has(run.id), 'Workflow counted twice.'); countedRuns.add(run.id); counts[`${expected}Runs`]++; return run; };
  const entered = async promise => {
    const timeout = new AbortController();
    try { await Promise.race([promise, delay(10000, undefined, { signal: AbortSignal.any([control.signal, timeout.signal]) }).then(() => { throw new Error('Offline call did not enter its deterministic gate.'); })]); }
    finally { timeout.abort(); }
  };
  const assertDrained = service => { const health = service.health(true); assert.equal(health.status, 'ok', JSON.stringify(health.issues)); assert.equal(health.activeLocalRuns, 0);
    assert.equal(health.state.counts.activeRuns, 0); assert.equal(health.state.counts.slots, 0); assert.equal(health.state.counts.slotWaiters, 0); assert.equal(health.state.counts.leases, 0); assert.equal(activeCalls, 0);
    for (const status of ['completed', 'failed', 'cancelled']) assert.equal(health.state.runStatuses[status] ?? 0, counts[`${status}Runs`] - generationCounts[`${status}Runs`], `Exact ${status} workflow count.`);
    assert.equal(health.state.counts.runs, counts.completedRuns + counts.failedRuns + counts.cancelledRuns - generationCounts.completedRuns - generationCounts.failedRuns - generationCounts.cancelledRuns);
    assert.ok(counts.peakCalls <= 2); };
  try {
    mkdirSync(runtime);
    const sourceDigest = runtimeDigest(join(repository, 'dist')), sourcePackage = readFileSync(join(repository, 'package.json'));
    cpSync(join(repository, 'dist'), join(runtime, 'dist'), { recursive: true }); writeFileSync(join(runtime, 'package.json'), sourcePackage);
    assert.equal(runtimeDigest(join(repository, 'dist')), sourceDigest, 'Compiled source changed during capture; retry after building.');
    assert.equal(runtimeDigest(join(runtime, 'dist')), sourceDigest, 'Captured runtime differs from its source build.');
    assert.ok(readFileSync(join(repository, 'package.json')).equals(sourcePackage), 'Package version changed during runtime capture.');
    digest = runtimeDigest(runtime);
    writeFileSync(log.replace(/\.jsonl$/, '.harness.mjs'), readFileSync(filename));
    const api = await import(pathToFileURL(join(runtime, 'dist', 'index.js')).href);
    const { CouncilService, loadConfig, VERSION, verifyStateSnapshot } = api; version = VERSION;
    assert.equal(typeof verifyStateSnapshot, 'function', 'Build Council with all-store snapshots before running this harness.');
    assert.equal(VERSION, JSON.parse(readFileSync(join(runtime, 'package.json'), 'utf8')).version);
    const turns = new Map(), roomCalls = new Map();
    const runner = { doctor: async () => [], run: async request => {
      assert.equal(request.permission, 'read'); assert.ok(request.model.startsWith('offline-'));
      activeCalls++; counts.fakeAgentCalls++; counts.peakCalls = Math.max(counts.peakCalls, activeCalls);
      try {
        const ballot = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
        let output, status = 'completed';
        if (ballot) { const data = JSON.parse(ballot[1]); output = JSON.stringify({ vote: 'approve', rationale: 'Offline acceptance evidence and permissions are explicit.', assignments: Object.fromEntries(data.tasks.map((task, index) => [task.id, data.members[index % data.members.length].agent])) }); }
        else if (request.prompt.includes('You are private-peer-for-')) { assert.ok(request.prompt.includes('PAIR_ONLY_')); output = 'PAIR_ONLY_REPLY: Preserve the separate task ownership.'; }
        else if (request.prompt.includes('You are advisor-for-')) { assert.ok(!request.prompt.includes('PAIR_ONLY_')); output = 'Public advisory evidence: preserve the verified result.'; }
        else if (request.prompt.includes('COORDINATION_MANAGED_')) {
          const key = /COORDINATION_MANAGED_\d+/.exec(request.prompt)[0], turn = (turns.get(key) ?? 0) + 1; turns.set(key, turn);
          output = turn === 1 ? JSON.stringify({ council_private_chat_request: { peer: 'beta', message: 'PAIR_ONLY_QUESTION: Coordinate responsibility.', context: 'PAIR_ONLY_CONTEXT: Complete pair evidence.' } })
            : turn === 2 ? JSON.stringify({ council_advisor_request: { question: 'Verify the public acceptance boundary.', context: 'Public evidence only.' } }) : 'Completed offline managed task with explicit verification.';
        } else if (request.prompt.includes('COORDINATION_FAILED_')) { status = 'failed'; output = 'Retained intentional failure evidence.'; }
        else if (request.prompt.includes('COORDINATION_CANCEL_')) { cancelGate.enter(); await cancelGate.done; output = 'Retained intentional cancellation evidence.'; }
        else {
          const task = /Room task (task-\d+):[^\n]*COORDINATION_ROOM_(\d+)/.exec(request.prompt);
          if (task) { const key = `${task[2]}:${task[1]}`; roomCalls.set(key, (roomCalls.get(key) ?? 0) + 1);
            if (pauseGate && !pauseGate.claimed) { pauseGate.claimed = true; pauseGate.enter(); await pauseGate.done; } }
          output = 'Completed offline task with dependency evidence.';
        }
        if (request.signal?.aborted) status = 'cancelled';
        await delay(2);
        return { agent: request.agent, output, status, durationMs: 2, ...(status === 'completed' ? {} : { error: `Intentional offline ${status}.` }) };
      } finally { activeCalls--; }
    } };
    const open = () => {
      workspace = join(temporary, `workspace-${++counts.rotations}`); mkdirSync(workspace); generationCounts = { ...counts };
      const agents = Object.fromEntries(['alpha', 'beta', 'gamma'].map(alias => [alias, { enabled: true, profile: 'codex', model: `offline-${alias}`, effort: 'low' }]));
      const config = loadConfig(workspace, { backend: 'mock', maxConcurrency: 2, maxCalls: 32, maxOutputBytes: 262144, agents });
      services = [new CouncilService(config, runner), new CouncilService(config, runner)];
      for (const service of services) { void service.billing; void service.rooms; void service.privateChats; void service.delegation; }
    };
    const close = async () => { await Promise.all(services.map(service => service.close())); services = []; assert.equal(activeCalls, 0); };
    const inspectDatabases = () => {
      const state = join(workspace, '.council'), inspected = [];
      for (const name of ['state.db', 'rooms.sqlite', 'billing.sqlite', 'private-chats.sqlite', 'delegation.sqlite']) {
        assert.ok(existsSync(join(state, name)), name); const db = new DatabaseSync(join(state, name), { readOnly: true });
        try {
          assert.deepEqual(db.prepare('PRAGMA quick_check').all().map(row => row.quick_check), ['ok']);
          if (!services.length && name === 'state.db') for (const table of ['instances', 'slots', 'slot_waiters']) assert.equal(db.prepare(`SELECT COUNT(*) AS count FROM ${table}`).get().count, 0, `${table} must drain after closing both hosts.`);
          inspected.push(name);
        } finally { db.close(); }
      }
      return inspected;
    };
    const checkpoint = async force => {
      if (!force && performance.now() - lastCheckpoint < options.checkpointMs) return;
      const measured = usage(), dbBytes = services[0].health().storageBytes;
      assert.ok(measured.bytes + 2 * dbBytes + 16 * 1024 * 1024 < FIXTURE_BUDGET_BYTES, 'Insufficient projected snapshot headroom.');
      const destination = join(snapshots, `checkpoint-${String(counts.snapshots + 1).padStart(6, '0')}`);
      let writing = true, writerFailure;
      const backupChat = services[0].privateChats.create({ participants: ['snapshot:writer', 'snapshot:observer'] });
      const writer = (async () => {
        try { while (writing && !control.signal.aborted) {
          services[1].store.putContext('snapshot-writes', String(++counts.hotBackupWrites), 'offline-fixture');
          services[1].billing.update('beta', { prices: { inputPerMillion: counts.hotBackupWrites } });
          services[1].privateChats.send({ chatId: backupChat.id, from: 'snapshot:writer', body: `Concurrent snapshot note ${counts.hotBackupWrites}.` });
          await delay(5);
        } } catch (error) { writerFailure = error; }
      })();
      let result, verified;
      try { result = await services[0].snapshot(destination, control.signal); verified = await verifyStateSnapshot(destination, control.signal); }
      finally { writing = false; await writer; }
      if (writerFailure) throw writerFailure;
      assert.equal(verified.valid, true, JSON.stringify(verified.issues)); assert.equal(result.manifest.consistency, 'per-database'); assert.equal(result.manifest.databases.length, 5);
      counts.snapshots++; retained.push(destination);
      while (retained.length > 2) removeOwned(retained.shift(), snapshots, 'checkpoint-');
      assert.equal(runtimeDigest(runtime), digest); lastCheckpoint = performance.now();
      emit('checkpoint', { ...counts, usage: usage(), retainedSnapshots: retained.length, memory: process.memoryUsage() });
    };
    open(); workloadStarted = performance.now();
    emit('started', { version, runtimeDigest: digest, harnessDigest: createHash('sha256').update(readFileSync(filename)).digest('hex'), options, fixture: temporary, node: process.version, sqlite: process.versions.sqlite, platform: process.platform, liveProviderCalls: 0 });
    while (!control.signal.aborted && performance.now() - workloadStarted < options.durationMs) {
      const cycle = counts.cycles + 1, [owner, peer] = services, task = name => ({ mode: 'delegate', agents: ['alpha'], permission: 'read', task: `${name}_${cycle}: verify the explicit offline task.` });
      const now = new Date().toISOString(), future = new Date(Date.now() + 3600000).toISOString();
      owner.billing.update('alpha', { accountKey: 'offline-shared', mode: 'subscription', source: { kind: 'manual', label: 'Offline fixture observation' }, observedAt: now, validUntil: future,
        windows: [{ id: 'included', label: 'Included requests', unit: 'requests', scope: 'subscription', remaining: 100 }], stopAtSubscriptionLimit: true,
        routing: { tier: 'small', capabilities: ['analysis'], costRank: 1, qualityRank: 2 } });
      peer.billing.update('beta', { accountKey: 'offline-shared', routing: { tier: 'small', capabilities: ['analysis'], costRank: 2, qualityRank: 2 } });
      assert.deepEqual(peer.routePlan({ task: 'Inspect bounded evidence.', taskClass: 'routine', requiredCapabilities: ['analysis'] }).selectedAliases, ['alpha']);
      const managed = countRun(await owner.run({ ...task('COORDINATION_MANAGED'), privateChats: { alpha: { peers: ['beta'], maxRequests: 1 } }, advisors: { alpha: { agent: 'gamma' } } }), 'completed');
      assert.equal(managed.steps.length, 5); counts.privateExchanges++; counts.advisorExchanges++;
      assert.ok(!JSON.stringify({ managed, events: owner.store.events(managed.id) }).includes('PAIR_ONLY_'));
      const actor = `run:${managed.id}:alpha`, chats = peer.privateChats.list(actor); assert.equal(chats.length, 1);
      assert.equal(peer.privateChats.page({ chatId: chats[0].id, actor }).messages.length, 2);
      const chat = owner.privateChats.create({ participants: ['human:offline', 'delegate:offline'] });
      const watch = peer.privateChats.watch({ chatId: chat.id, actor: 'delegate:offline', after: chat.lastSequence, timeoutMs: 3000 });
      const message = owner.privateChats.send({ chatId: chat.id, from: 'human:offline', body: `Manual note ${cycle}.`, idempotencyKey: `same-message-${cycle}` });
      assert.equal(owner.privateChats.send({ chatId: chat.id, from: 'human:offline', body: message.body, idempotencyKey: `same-message-${cycle}` }).id, message.id);
      assert.equal((await watch).reason, 'changed');
      const room = owner.rooms.create({ title: `Offline coordination ${cycle}`, brief: 'Complete independent evidence tasks and a dependent review.', rounds: 1, maxCalls: 16, autoExecute: true,
        members: [{ agent: 'alpha', role: 'lead', weight: 2 }, { agent: 'beta', role: 'worker', weight: 1 }],
        tasks: [1, 2, 3].map(index => ({ title: `COORDINATION_ROOM_${cycle} task ${index}`, brief: 'Report read-only acceptance evidence.', permission: 'read', ...(index === 3 ? { dependsOn: ['task-1', 'task-2'] } : {}) })) });
      pauseGate = gate(); owner.rooms.startDiscussion(room.id);
      await entered(pauseGate.entered);
      peer.rooms.pause(room.id); pauseGate.release();
      const paused = await owner.rooms.wait(room.id, 10000, control.signal); assert.equal(paused.status, 'paused'); counts.pausedRooms++;
      assert.ok(paused.tasks.some(value => value.status === 'completed')); assert.ok(paused.tasks.some(value => value.status !== 'completed'));
      pauseGate = undefined; peer.rooms.resume(room.id);
      const completed = await peer.rooms.wait(room.id, 10000, control.signal); assert.equal(completed.status, 'completed', completed.error); counts.completedRooms++;
      assert.ok(completed.tasks.every(value => value.status === 'completed')); assert.equal(completed.callsUsed, 5);
      for (const value of completed.tasks) assert.equal(roomCalls.get(`${cycle}:${value.id}`), 1, 'Room continuation must not replay completed work.');
      const configuration = owner.delegation.get(); owner.delegation.configure({ mode: 'agent', delegate: 'offline-host', mandate: 'Report material blockers to the human.', roomIds: [room.id], expectedRevision: configuration.revision });
      const before = await peer.delegation.watch({ timeoutMs: 0 });
      const boardWatch = peer.delegation.watch({ cursor: before.cursor, timeoutMs: 3000 });
      const report = owner.delegation.report({ author: 'offline-host', kind: 'decision', body: `Review the verified result for cycle ${cycle}.`, roomIds: [room.id], expectedRoomRevisions: { [room.id]: completed.revision } });
      assert.equal((await boardWatch).openQuestions, 1);
      peer.delegation.resolve(report.id, { author: 'human:offline', body: 'Evidence reviewed; no new task permissions granted.', expectedRevision: report.revision }); counts.executiveReports++;
      assert.equal((await owner.delegation.watch({ timeoutMs: 0 })).openQuestions, 0);
      const beforeBlocked = counts.fakeAgentCalls;
      owner.billing.update('alpha', { observedAt: new Date().toISOString(), windows: [{ id: 'included', label: 'Included requests', unit: 'requests', scope: 'subscription', remaining: 0 }] });
      countRun(await peer.run(task('COORDINATION_PROTECTED')), 'failed'); counts.protectedDispatches++; assert.equal(counts.fakeAgentCalls, beforeBlocked);
      owner.billing.update('alpha', { observedAt: new Date().toISOString(), windows: [{ id: 'included', label: 'Included requests', unit: 'requests', scope: 'subscription', remaining: 100 }] });
      countRun(await owner.run(task('COORDINATION_FAILED')), 'failed');
      cancelGate = gate(); const cancelled = owner.start(task('COORDINATION_CANCEL'));
      await entered(cancelGate.entered); peer.cancel(cancelled.id); cancelGate.release(); countRun(await owner.completion(cancelled.id), 'cancelled'); cancelGate = undefined;
      const roomRunIds = new Set(completed.messages.map(message => message.runId).filter(Boolean));
      assert.equal(roomRunIds.size, 4);
      for (const id of roomRunIds) countRun(peer.getRun(id), 'completed');
      countRun(await owner.run({ ...task('COORDINATION_BASELINE'), agents: ['beta'] }), 'completed');
      for (const service of services) assertDrained(service);
      inspectDatabases(); counts.cycles++; usage(); await checkpoint(false);
      if (counts.cycles % options.rotationCycles === 0 && performance.now() - workloadStarted < options.durationMs) {
        await close(); inspectDatabases(); removeOwned(workspace, temporary, 'workspace-'); turns.clear(); roomCalls.clear(); countedRuns.clear(); open();
      }
    }
    control.signal.throwIfAborted(); assert.ok(counts.cycles > 0); await checkpoint(true);
    for (const service of services) assertDrained(service);
    await close(); inspectDatabases(); assert.equal(runtimeDigest(runtime), digest);
  } catch (error) { failure = error; }
  finally {
    pauseGate?.release(); cancelGate?.release();
    const closed = await Promise.allSettled(services.map(service => service.close()));
    for (const outcome of closed) if (outcome.status === 'rejected') cleanupFailure ??= outcome.reason;
    if (cleanupFailure) failure = cleanupFailure;
    process.removeListener('SIGINT', stop); process.removeListener('SIGTERM', stop);
  }
  if (!failure && !interrupted) {
    try { removeOwned(temporary, temporaryBase, 'council-coordination-soak-'); }
    catch (error) { failure = cleanupFailure = error; }
  }
  const status = cleanupFailure ? 'failed' : interrupted ? 'cancelled' : failure ? 'failed' : 'completed';
  const summary = { status, interrupted, version, runtimeDigest: digest, options, ...counts, elapsedMs: Math.round(performance.now() - started), workloadElapsedMs: workloadStarted === undefined ? 0 : Math.round(performance.now() - workloadStarted),
    fixtureBudgetBytes: FIXTURE_BUDGET_BYTES, peakObservedLogicalFixtureBytes: peakFixtureBytes, liveProviderCalls: 0, log,
    ...(failure ? { error: failure.message, fixture: temporary } : {}) };
  writeFileSync(log.replace(/\.jsonl$/, '.summary.json'), JSON.stringify(summary, null, 2) + '\n'); emit('finished', summary);
  if (failure) { const error = new Error(`Coordination soak ${status}: ${failure.message}. Fixture preserved at ${temporary}. Evidence: ${log}`, { cause: failure }); error.status = status; throw error; }
  return summary;
}

if (process.argv[1] && resolve(process.argv[1]) === filename) {
  try { const result = await coordinationSoak(coordinationOptions(process.argv.slice(2))); if (result.status !== 'completed') process.exitCode = 130; }
  catch (error) { process.stderr.write(`${error.stack}\n`); process.exitCode = error.status === 'cancelled' ? 130 : 1; }
}
