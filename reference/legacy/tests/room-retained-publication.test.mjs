import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { RoomManager } from '../dist/rooms.js';

function fixture(t, { tasks = 1, concurrency = 1, write = false, taskRunner } = {}) {
  const parent = realpathSync.native(tmpdir()), root = mkdtempSync(join(parent, 'council-retained-publication-')), calls = [];
  if (write) {
    const git = (...args) => execFileSync('git', ['-C', root, ...args], { windowsHide: true, stdio: 'pipe' });
    git('init', '--quiet'); git('config', 'user.name', 'Offline Retention Test'); git('config', 'user.email', 'offline@example.invalid');
    git('config', 'core.autocrlf', 'false'); git('config', 'commit.gpgsign', 'false');
    writeFileSync(join(root, '.gitignore'), '.council/\n'); writeFileSync(join(root, 'base.txt'), 'Retained base.\n');
    git('add', '.'); git('commit', '--quiet', '-m', 'offline retention fixture');
  }
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 64, maxConcurrency: concurrency }), {
    doctor: async () => [], run: async request => {
      calls.push(request);
      const marker = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
      const ballot = marker && JSON.parse(marker[1]);
      if (!ballot && taskRunner) return await taskRunner(request);
      if (write && !ballot) writeFileSync(join(request.cwd, 'retained.txt'), 'RETAINED_WRITE_EVIDENCE\n');
      return { agent: request.agent, status: 'completed', durationMs: 1, output: ballot
        ? JSON.stringify({ vote: 'approve', rationale: 'Complete offline acceptance evidence.', assignments: Object.fromEntries(ballot.tasks.map(task => [task.id, 'codex'])) })
        : 'Verified retained task evidence.' };
    },
  });
  const manager = new RoomManager(service), mutate = manager.store.mutate, peers = [];
  const original = { completion: service.completion, getRun: service.getRun, start: service.start };
  t.after(async () => {
    manager.store.mutate = mutate; Object.assign(service, original);
    await manager.close(); for (const peer of peers) await peer.close(); await service.close();
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-retained-publication-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const room = manager.create({ title: 'Preserve admitted work', brief: 'Retain offline results when room publication fails.', autoExecute: false,
    members: [{ agent: 'codex', role: 'lead', advisor: 'claude', advisorMaxRequests: 2 }, { agent: 'claude', role: 'advisor' }],
    tasks: Array.from({ length: tasks }, (_, index) => ({ title: `Task ${index + 1}`, brief: 'Return complete retained evidence without requesting advice.', permission: write ? 'write' : 'read' })) });
  return { service, manager, room, calls, mutate,
    async ready() { manager.startDiscussion(room.id); assert.equal((await manager.wait(room.id)).status, 'ready'); },
    peer() { const peer = new RoomManager(service); peers.push(peer); return peer; },
  };
}

function inject(wire, boundary, mode, { permanent = false, lateCancel = false, finalAck = false } = {}) {
  const { manager, mutate } = wire;
  const state = { injected: false, finalInjected: false, dispatchedRunId: undefined };
  manager.store.mutate = function (id, change, options) {
    if (permanent && state.injected) throw new Error('Injected permanent room writer outage.');
    let hit = false;
    const before = this.metadata(id);
    const wrapped = room => {
      change(room);
      hit = !state.injected && (boundary === 'dispatch'
        ? room.tasks.some((task, index) => task.status === 'running' && task.runId && !before.tasks[index].runId)
        : boundary === 'accounting' ? before.tasks.some(task => task.status === 'running') && room.callsUsed < before.callsUsed
          : room.tasks.some((task, index) => task.status === 'completed' && before.tasks[index].status !== 'completed'));
      if (hit && boundary === 'dispatch') state.dispatchedRunId = room.tasks.find((task, index) => task.status === 'running' && task.runId && !before.tasks[index].runId).runId;
      if (hit && mode === 'rollback') { state.injected = true; throw new Error(`Injected ${boundary} publication rollback.`); }
    };
    let result;
    try { result = mutate.call(this, id, wrapped, options); }
    catch (error) { if (hit && lateCancel) this.requestCancel(id); throw error; }
    if (hit) {
      state.injected = true;
      if (lateCancel) this.requestCancel(id);
      throw new Error(`Injected ${boundary} lost commit acknowledgement.`);
    }
    if (finalAck && state.injected && options?.release && result.status === 'failed' && !state.finalInjected) {
      state.finalInjected = true; throw new Error('Injected final failure release acknowledgement loss.');
    }
    return result;
  };
  return state;
}

function assertCancelledBeforeCall(wire, result, expectedRunId) {
  assert.equal(result.status, 'failed'); assert.equal(result.callsUsed, 1);
  assert.equal(typeof expectedRunId, 'string'); assert.equal(result.tasks[0].runId, expectedRunId);
  const task = result.tasks[0], run = wire.service.getRun(expectedRunId);
  assert.equal(run.id, expectedRunId); assert.equal(run.status, 'cancelled'); assert.deepEqual(run.steps, []); assert.equal(run.output, undefined);
  assert.equal(task.status, 'cancelled'); assert.equal(task.resultPreview, ''); assert.deepEqual(task.workspaces, []);
  assert.equal(wire.calls.length, 1, 'Only the completed approval discussion ran; the failed dispatch never called the task runner.');
  assert.match(wire.calls[0].prompt, /ROOM_BALLOT_DATA_BEGIN/);
  const observer = wire.peer(), history = []; let cursor;
  do { const page = observer.messagePage(result.id, 100, cursor); history.push(...page.messages); cursor = page.nextCursor; } while (cursor !== null);
  const dispatch = history.filter(message => message.runId === expectedRunId && message.body === 'Dispatched task-1.');
  const retained = history.filter(message => message.runId === expectedRunId && message.body === 'Retained workflow evidence from an owned call whose room publication failed.');
  const terminal = history.filter(message => message.runId === expectedRunId && message.body === 'task-1: cancelled\n');
  assert.equal(dispatch.length, 1); assert.equal(retained.length, 1); assert.equal(terminal.length, 1);
  for (const entry of [dispatch[0], retained[0], terminal[0]]) assert.ok(result.messages.some(message => message.id === entry.id && message.runId === expectedRunId));
  const events = wire.service.store.events(expectedRunId);
  assert.equal(events.filter(event => event.type === 'cancel_requested').length, 1);
  assert.equal(events.filter(event => event.type === 'run.cancelled').length, 1);
  assert.equal(events.some(event => event.type === 'step.started' || event.type === 'step.finished'), false);
  assert.equal(wire.service.store.isCancellationRequested(expectedRunId), true);
  const peer = observer.get(result.id);
  assert.equal(peer.status, 'failed'); assert.equal(peer.callsUsed, 1); assert.equal(peer.tasks[0].runId, expectedRunId); assert.equal(peer.tasks[0].status, 'cancelled');
  const counts = wire.service.store.health().counts;
  assert.equal(counts.activeRuns, 0); assert.equal(counts.slots, 0); assert.equal(counts.slotWaiters, 0);
  return { runId: expectedRunId, dispatchId: dispatch[0].id, retainedId: retained[0].id, terminalId: terminal[0].id };
}

function assertRetained(wire, result, expectedCalls = 2) {
  assert.equal(result.status, 'failed'); assert.equal(result.callsUsed, expectedCalls);
  const task = result.tasks[0];
  const run = wire.service.getRun(task.runId);
  assert.ok(['completed', 'cancelled'].includes(run.status)); assert.equal(task.status, run.status);
  assert.equal(task.resultPreview, run.output); assert.match(task.resultPreview, /Verified retained task evidence/); assert.deepEqual(task.workspaces, []);
  assert.ok(result.messages.some(message => message.runId === task.runId));
  const history = []; let cursor;
  do { const page = wire.manager.messagePage(result.id, 100, cursor); history.push(...page.messages); cursor = page.nextCursor; } while (cursor !== null);
  assert.equal(history.filter(message => message.runId === task.runId && message.body === 'Dispatched task-1.').length, 1);
  assert.equal(history.filter(message => message.runId === task.runId && message.body.startsWith(`task-1: ${run.status}\n`)).length, 1);
  assert.equal(wire.calls.length, 2, 'Admitted work is retained without replay or unused advisor calls.');
  assert.throws(() => wire.manager.execute(result.id), /Only an approved ready room/);
}

for (const boundary of ['dispatch', 'accounting', 'result']) for (const mode of ['rollback', 'lost-ack']) {
  test(`retains terminal task evidence after ${boundary} ${mode}`, { timeout: 15000 }, async t => {
    const wire = fixture(t); await wire.ready();
    const state = inject(wire, boundary, mode);
    wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id);
    assert.equal(state.injected, true); assert.match(result.error, /Injected/);
    if (boundary === 'dispatch') {
      assert.equal(result.error, mode === 'rollback' ? 'Injected dispatch publication rollback.' : 'Injected dispatch lost commit acknowledgement.');
      assertCancelledBeforeCall(wire, result, state.dispatchedRunId);
      assert.throws(() => wire.manager.execute(result.id), /Only an approved ready room/);
    } else assertRetained(wire, result);
    const peer = wire.peer().get(result.id);
    assert.equal(peer.status, 'failed'); assert.equal(peer.callsUsed, boundary === 'dispatch' ? 1 : 2); assert.equal(peer.tasks[0].runId, result.tasks[0].runId);
    assert.equal(wire.service.store.health().counts.slots, 0); assert.equal(wire.service.store.health().counts.slotWaiters, 0);
  });
}

test('permanent room outage retains emergency task metadata and repairs it after the writer returns', { timeout: 15000 }, async t => {
  const wire = fixture(t); await wire.ready(); inject(wire, 'result', 'rollback', { permanent: true });
  wire.manager.execute(wire.room.id); const retained = await wire.manager.wait(wire.room.id);
  assert.equal(retained.status, 'failed'); assert.match(retained.error, /persistence failed/);
  assert.equal(retained.tasks[0].status, 'completed'); assert.equal(retained.callsUsed, 2);
  assert.equal(wire.manager.list(1)[0].completedTasks, 1);
  wire.manager.store.mutate = wire.mutate;
  await wire.manager.close(); const restored = wire.peer().get(retained.id);
  assert.equal(restored.status, 'failed'); assert.equal(restored.tasks[0].status, 'completed');
  assert.equal(restored.tasks[0].runId, retained.tasks[0].runId); assert.equal(restored.tasks[0].resultPreview, 'Verified retained task evidence.');
  assert.equal(restored.callsUsed, 2); assert.equal(wire.calls.length, 2);
});

test('lost final failure acknowledgement repairs a released failed room without taking ownership of new work', { timeout: 15000 }, async t => {
  const wire = fixture(t); await wire.ready(); const state = inject(wire, 'dispatch', 'rollback', { finalAck: true });
  wire.manager.execute(wire.room.id); const retained = await wire.manager.wait(wire.room.id);
  assert.equal(state.finalInjected, true); assert.equal(retained.status, 'failed');
  const evidence = assertCancelledBeforeCall(wire, retained, state.dispatchedRunId);
  const originalCause = 'Injected dispatch publication rollback.';
  assert.equal(wire.peer().messagePage(retained.id, 100).messages.filter(message => message.body === originalCause).length, 1);
  wire.manager.store.mutate = wire.mutate;
  const guidance = wire.peer().postMessage(retained.id, { author: 'PM', body: 'Preserve this newer guidance while repairing the failed room.' });
  const guidanceMessages = guidance.messages.filter(message => message.body === 'Preserve this newer guidance while repairing the failed room.');
  assert.equal(guidanceMessages.length, 1);
  await wire.manager.close();
  const restored = wire.peer().get(retained.id);
  assert.equal(restored.status, 'failed'); assert.equal(restored.callsUsed, 1); assert.equal(restored.tasks[0].runId, retained.tasks[0].runId);
  assert.equal(restored.instructionRevision, guidance.instructionRevision);
  assert.equal(restored.messages.filter(message => message.body === 'Preserve this newer guidance while repairing the failed room.').length, 1);
  assert.equal(restored.messages.find(message => message.body === 'Preserve this newer guidance while repairing the failed room.').id, guidanceMessages[0].id);
  assert.equal(wire.peer().messagePage(retained.id, 100).messages.filter(message => message.body === originalCause).length, 1);
  assert.deepEqual(assertCancelledBeforeCall(wire, restored, state.dispatchedRunId), evidence);
  assert.equal(wire.calls.length, 1);
});

test('a late accepted cancellation preserves the real publication failure and completed task result', { timeout: 15000 }, async t => {
  const wire = fixture(t); await wire.ready(); inject(wire, 'result', 'rollback', { lateCancel: true });
  wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id);
  assert.match(result.error, /Injected result publication rollback/); assertRetained(wire, result);
});

test('unreadable terminal workflow retains its identity and estimate with an explicit unverified outcome', { timeout: 15000 }, async t => {
  const wire = fixture(t); await wire.ready(); const taskRuns = new Set();
  const start = wire.service.start.bind(wire.service), completion = wire.service.completion.bind(wire.service), getRun = wire.service.getRun.bind(wire.service);
  wire.service.start = (input, options) => { const run = start(input, options); taskRuns.add(run.id); return run; };
  wire.service.completion = async id => {
    assert.equal(wire.manager.store.db.isTransaction, false, 'Workflow drainage must not hold the room writer.');
    const result = await completion(id); if (taskRuns.has(id)) throw new Error('Injected terminal evidence read failure.'); return result;
  };
  wire.service.getRun = id => {
    if (taskRuns.has(id)) { assert.equal(wire.manager.store.db.isTransaction, false); throw new Error('Injected terminal evidence read failure.'); }
    return getRun(id);
  };
  wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id);
  assert.equal(result.status, 'failed'); assert.equal(result.callsUsed, 6); assert.match(result.error, /completion could not be verified/);
  assert.equal(result.tasks[0].status, 'failed'); assert.ok(taskRuns.has(result.tasks[0].runId));
  assert.match(result.tasks[0].error, /estimated call reservation is retained/);
  assert.ok(result.messages.some(message => message.runId === result.tasks[0].runId)); assert.equal(wire.calls.length, 2);
});

test('absolute accounting remains exact near the safe integer limit after a lost refund acknowledgement', { timeout: 15000 }, async t => {
  const wire = fixture(t); await wire.ready();
  wire.manager.store.mutate(wire.room.id, room => { room.maxCalls = Number.MAX_SAFE_INTEGER; room.callsUsed = Number.MAX_SAFE_INTEGER - 5; }, { requireIdle: true });
  inject(wire, 'accounting', 'lost-ack'); wire.manager.execute(wire.room.id);
  const result = await wire.manager.wait(wire.room.id);
  assertRetained(wire, result, Number.MAX_SAFE_INTEGER - 4);
});

test('simultaneous admitted siblings retain each result and settle their reservations once after rollback', { timeout: 15000 }, async t => {
  const wire = fixture(t, { tasks: 2, concurrency: 2 }); await wire.ready();
  wire.manager.store.mutate(wire.room.id, room => { room.maxCalls = 11; }, { requireIdle: true });
  inject(wire, 'dispatch', 'rollback'); wire.manager.execute(wire.room.id);
  const result = await wire.manager.wait(wire.room.id), runs = wire.service.store.listRunSummaries(10).map(summary => wire.service.getRun(summary.id));
  assert.equal(result.status, 'failed'); assert.equal(result.callsUsed, runs.reduce((sum, run) => sum + run.steps.length, 0));
  assert.ok(result.tasks.every(task => task.runId)); assert.equal(new Set(result.tasks.map(task => task.runId)).size, 2);
  assert.ok(result.tasks.every(task => ['completed', 'failed', 'cancelled'].includes(task.status)));
  assert.equal(wire.calls.length, result.callsUsed); assert.ok(wire.calls.length <= 3);
  assert.equal(wire.service.store.health().counts.slots, 0); assert.equal(wire.service.store.health().counts.slotWaiters, 0);
});

test('a conflicting admitted task association forces failure even when changed guidance would allow a fresh discussion', { timeout: 15000 }, async t => {
  const wire = fixture(t); await wire.ready(); let injected = false, retainedId;
  wire.manager.store.mutate = function (id, change, options) {
    let admitted = false;
    try {
      return wire.mutate.call(this, id, room => {
        change(room);
        if (!injected && room.tasks[0].status === 'running' && room.tasks[0].runId) {
          injected = true; admitted = true; retainedId = room.tasks[0].runId;
          wire.manager.assertInstructions({ ...room, instructionRevision: (room.instructionRevision ?? 0) + 1 }, wire.manager.jobs.get(id));
        }
      }, options);
    } catch (error) {
      if (admitted) wire.mutate.call(this, id, room => {
        room.tasks[0].runId = 'other-retained-workflow'; room.tasks[0].status = 'running';
        room.instructionRevision = (room.instructionRevision ?? 0) + 1;
      }, options);
      throw error;
    }
  };
  wire.manager.execute(wire.room.id); const result = await wire.manager.wait(wire.room.id);
  assert.equal(injected, true); assert.equal(result.status, 'failed'); assert.match(result.error, /conflicts with this task association/);
  assert.equal(result.tasks[0].runId, 'other-retained-workflow'); assert.equal(result.tasks[0].status, 'failed');
  assert.ok(result.messages.some(message => message.runId === retainedId));
  assert.equal(wire.service.getRun(retainedId).status, 'completed'); assert.equal(wire.calls.length, 2);
  assert.throws(() => wire.manager.startDiscussion(result.id), /current state/);
});

test('a rolled-back task publication retains the real exported worktree and patch without replaying writes', { timeout: 30000 }, async t => {
  const wire = fixture(t, { write: true }); await wire.ready(); inject(wire, 'result', 'rollback');
  wire.manager.execute(wire.room.id); const room = await wire.manager.wait(wire.room.id);
  assert.equal(room.status, 'failed'); assert.equal(room.tasks[0].status, 'completed');
  const run = wire.service.getRun(room.tasks[0].runId);
  assert.equal(run.status, 'completed'); assert.equal(run.workspaces.length, 1);
  assert.deepEqual(room.tasks[0].workspaces, run.workspaces);
  assert.equal(run.workspaces[0].exportStatus, 'ready'); assert.equal(run.workspaces[0].changed, true);
  assert.match(readFileSync(run.workspaces[0].patchPath, 'utf8'), /\+RETAINED_WRITE_EVIDENCE/);
  assert.equal(readFileSync(join(run.workspaces[0].path, 'retained.txt'), 'utf8'), 'RETAINED_WRITE_EVIDENCE\n');
  assert.equal(wire.calls.length, 2); assert.equal(room.callsUsed, 2);
  assert.deepEqual(wire.peer().get(room.id).tasks[0].workspaces, run.workspaces);
});

function deferred() {
  let resolve;
  const promise = new Promise(accept => { resolve = accept; });
  return { promise, resolve };
}
async function awaitEntry(promise) {
  let timer;
  try { await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Both offline task calls must enter.')), 3000); })]); }
  finally { clearTimeout(timer); }
}
async function aborted(signal) {
  if (!signal.aborted) await new Promise(resolve => signal.addEventListener('abort', resolve, { once: true }));
}

test('a real sibling publication failure after accepted cancellation keeps the room failed', { timeout: 15000 }, async t => {
  const entered = deferred(), second = deferred(); let taskCalls = 0, firstPublished = false, injected = false;
  const wire = fixture(t, { tasks: 2, concurrency: 2, taskRunner: async request => {
    const ordinal = ++taskCalls; if (ordinal === 2) entered.resolve();
    await aborted(request.signal); if (ordinal === 2) await second.promise;
    return { agent: request.agent, status: 'cancelled', output: 'Retained cancelled offline task evidence.', durationMs: 1 };
  } });
  try {
    await wire.ready();
    wire.manager.store.mutate = function (id, change, options) {
      const before = this.metadata(id); let publishesFirst = false;
      const result = wire.mutate.call(this, id, room => {
        change(room);
        if (!firstPublished && before.tasks[0].status === 'running' && room.tasks[0].status === 'cancelled') publishesFirst = true;
        if (!injected && firstPublished && before.tasks[1].status === 'running' && room.tasks[1].status === 'cancelled') {
          injected = true; throw new Error('Real sibling publication failure after accepted cancellation.');
        }
      }, options);
      if (publishesFirst) { firstPublished = true; setImmediate(second.resolve); }
      return result;
    };
    wire.manager.execute(wire.room.id); await awaitEntry(entered.promise); wire.manager.cancel(wire.room.id);
    const result = await wire.manager.wait(wire.room.id);
    assert.equal(firstPublished, true); assert.equal(injected, true); assert.equal(result.status, 'failed');
    assert.match(result.error, /Real sibling publication failure after accepted cancellation/);
    assert.ok(result.tasks.every(task => task.status === 'cancelled' && task.runId));
    assert.ok(result.tasks.every(task => wire.service.getRun(task.runId).status === 'cancelled'));
    assert.equal(result.callsUsed, 3); assert.equal(wire.calls.length, 3);
    assert.equal(wire.peer().get(result.id).status, 'failed');
  } finally { second.resolve(); }
});

test('a first real controller fault keeps its priority when cancellation and another publication failure arrive later', { timeout: 15000 }, async t => {
  const entered = deferred(), first = deferred(), second = deferred();
  const initialFault = new Error('First real coordination fault before user cancellation.');
  let taskCalls = 0, firstPublished = false, injected = false, cancellationAccepted = false;
  const wire = fixture(t, { tasks: 2, concurrency: 2, taskRunner: async request => {
    const ordinal = ++taskCalls; if (ordinal === 2) entered.resolve();
    if (ordinal === 1) await first.promise;
    else { await aborted(request.signal); await second.promise; }
    return { agent: request.agent, status: ordinal === 1 ? 'completed' : 'cancelled', output: 'Retained offline fault ordering evidence.', durationMs: 1 };
  } });
  try {
    await wire.ready();
    wire.manager.store.mutate = function (id, change, options) {
      const before = this.metadata(id); let publishesFirst = false;
      const result = wire.mutate.call(this, id, room => {
        change(room);
        if (!firstPublished && before.tasks[0].status === 'running' && room.tasks[0].status === 'completed') publishesFirst = true;
        if (!injected && firstPublished && before.tasks[1].status === 'running' && room.tasks[1].status === 'cancelled') {
          injected = true; throw new Error('Later real sibling publication failure must not replace the first fault.');
        }
      }, options);
      if (publishesFirst) {
        firstPublished = true;
        const job = wire.manager.jobs.get(id);
        // Reproduce the existing poll-failure transition before a later human cancel.
        assert.equal(job.cancelRequested, false); job.failure = initialFault; job.controller.abort(initialFault); wire.manager.abortRuns(job);
        setImmediate(() => {
          wire.manager.cancel(id); cancellationAccepted = true;
          assert.equal(job.controller.signal.reason, initialFault); second.resolve();
        });
      }
      return result;
    };
    wire.manager.execute(wire.room.id); await awaitEntry(entered.promise); first.resolve();
    const result = await wire.manager.wait(wire.room.id);
    assert.equal(firstPublished, true); assert.equal(cancellationAccepted, true); assert.equal(injected, true);
    assert.equal(result.status, 'failed'); assert.equal(result.error, initialFault.message);
    assert.deepEqual(result.tasks.map(task => task.status), ['completed', 'cancelled']);
    assert.ok(result.tasks.every(task => task.runId)); assert.equal(result.callsUsed, 3); assert.equal(wire.calls.length, 3);
    assert.equal(wire.peer().get(result.id).status, 'failed');
  } finally { first.resolve(); second.resolve(); }
});
