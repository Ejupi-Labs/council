import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { RoomManager } from '../dist/rooms.js';

const task = (title, extra = {}) => ({ title, brief: `Verify ${title} with complete evidence.`, permission: 'read', ...extra });
const input = extra => ({ title: 'Durable pause fixture', brief: 'Preserve completed work and explicit project-manager approval.',
  members: [{ agent: 'codex', role: 'lead' }], tasks: [task('First')], ...extra });
const complete = (request, output) => ({ agent: request.agent, status: 'completed', output, durationMs: 1 });
const data = request => {
  const match = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
  return match ? JSON.parse(match[1]) : undefined;
};
const ballot = (value, vote = 'approve') => JSON.stringify({ vote, rationale: `Complete round ${value.round} evidence.`,
  assignments: Object.fromEntries(value.tasks.map(task => [task.id, 'codex'])) });
function deferred() { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; }
function fixture(t, handler, { git = false, maxConcurrency = 2 } = {}) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-room-pause-')), requests = [];
  if (git) {
    const gitRun = (...args) => execFileSync('git', ['-C', root, ...args], { windowsHide: true, stdio: 'pipe' });
    gitRun('init', '--quiet'); gitRun('config', 'user.name', 'Offline Pause'); gitRun('config', 'user.email', 'offline@example.invalid');
    gitRun('config', 'commit.gpgsign', 'false'); gitRun('config', 'core.autocrlf', 'false');
    writeFileSync(join(root, '.gitignore'), '.council/\n'); writeFileSync(join(root, 'base.txt'), 'Original fixture.\n');
    gitRun('add', '.'); gitRun('commit', '--quiet', '-m', 'offline pause fixture');
  }
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxCalls: 64, maxConcurrency }), {
    doctor: async () => [], run: async request => {
      requests.push(request);
      if (handler) return handler(request, requests);
      return complete(request, data(request) ? ballot(data(request)) : 'Verified the complete offline task.');
    },
  });
  const manager = new RoomManager(service), managers = [manager], services = [service];
  t.after(async () => {
    await Promise.all(managers.map(value => value.close())); await Promise.all(services.map(value => value.close()));
    assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-room-pause-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { service, manager, requests, peer() { const other = new RoomManager(service); managers.push(other); return other; },
    otherBackend() { const otherService = new CouncilService({ ...service.config, backend: 'native' }, { doctor: async () => [], run: async () => { throw new Error('Live provider must never run.'); } });
      const other = new RoomManager(otherService); managers.push(other); services.push(otherService); return other; } };
}

test('pause before first admission persists an idle continuation without any provider calls', async t => {
  const { manager, requests } = fixture(t), room = manager.create(input({}));
  assert.throws(() => manager.pause(room.id), /Only an active or approved ready room/);
  manager.startDiscussion(room.id); const requested = manager.pause(room.id);
  assert.equal(requested.status, 'discussing'); assert.equal(requested.pauseRequested, true);
  const paused = await manager.wait(room.id);
  assert.equal(paused.status, 'paused'); assert.deepEqual(paused.checkpoint, { phase: 'discussion', nextRound: 1 });
  assert.equal(paused.pauseRequested, undefined); assert.equal(requests.length, 0); assert.equal(paused.callsUsed, 0);
  assert.deepEqual(manager.pause(room.id), paused, 'Repeated pause is idempotent.');
  await delay(120); assert.equal(requests.length, 0); assert.equal(manager.jobs.size, 0);
  manager.resume(room.id); const completed = await manager.wait(room.id);
  assert.equal(completed.status, 'completed'); assert.equal(completed.checkpoint, undefined); assert.equal(requests.length, 2);
});

test('foreign pause drains the admitted discussion and preserves its approval before execution', async t => {
  const entered = deferred(), finish = deferred(); let signal;
  const { manager, peer, requests } = fixture(t, async request => {
    if (data(request)) { signal = request.signal; entered.resolve(); await finish.promise; return complete(request, ballot(data(request))); }
    return complete(request, 'Task completed once.');
  });
  const other = peer(), room = manager.create(input({})); manager.startDiscussion(room.id); await entered.promise;
  assert.equal(other.pause(room.id).pauseRequested, true); assert.equal(signal.aborted, false);
  finish.resolve(); const paused = await manager.wait(room.id);
  assert.equal(paused.status, 'paused'); assert.deepEqual(paused.checkpoint, { phase: 'execution' });
  assert.equal(paused.decision.approved, true); assert.equal(paused.tasks[0].status, 'assigned'); assert.equal(paused.callsUsed, 1);
  assert.ok(paused.messages.some(message => message.kind === 'opinion')); assert.equal(requests.length, 1); assert.equal(signal.aborted, false);
  other.resume(room.id); const completed = await other.wait(room.id);
  assert.equal(completed.status, 'completed'); assert.equal(requests.length, 2);
});

test('discussion continuation survives close and resumes the next round within its remaining budget', async t => {
  const entered = deferred(), finish = deferred(), rounds = [];
  const { manager, peer, requests } = fixture(t, async request => {
    const value = data(request); rounds.push(value.round);
    if (value.round === 1) { entered.resolve(); await finish.promise; }
    return complete(request, ballot(value, value.round === 1 ? 'revise' : 'approve'));
  });
  const room = manager.create(input({ autoExecute: false, rounds: 3, maxCalls: 3 })); manager.startDiscussion(room.id); await entered.promise;
  manager.pause(room.id); finish.resolve(); const paused = await manager.wait(room.id);
  assert.equal(paused.status, 'paused'); assert.deepEqual(paused.checkpoint, { phase: 'discussion', nextRound: 2 });
  assert.equal(paused.callsUsed, 1); await manager.close();
  const reopened = peer(); assert.equal(reopened.get(room.id).status, 'paused');
  reopened.resume(room.id); const ready = await reopened.wait(room.id);
  assert.equal(ready.status, 'ready'); assert.deepEqual(rounds, [1, 2]); assert.equal(ready.callsUsed, 2); assert.equal(requests.length, 2);
});

test('pause drains every admitted parallel task and resumes only the remaining DAG tasks', async t => {
  const both = deferred(), finish = deferred(); let taskCalls = 0; const signals = [];
  const { manager, peer, requests } = fixture(t, async request => {
    if (data(request)) return complete(request, ballot(data(request)));
    taskCalls++; signals.push(request.signal); if (taskCalls === 2) both.resolve();
    if (taskCalls <= 2) await finish.promise;
    return complete(request, `Complete task evidence ${taskCalls}.`);
  });
  const room = manager.create(input({ tasks: [task('First'), task('Second'), task('Third', { dependsOn: ['task-1'] }), task('Fourth', { dependsOn: ['task-2'] })] }));
  manager.startDiscussion(room.id); await both.promise; peer().pause(room.id);
  assert.ok(signals.every(signal => !signal.aborted)); finish.resolve(); const paused = await manager.wait(room.id);
  assert.equal(paused.status, 'paused'); assert.deepEqual(paused.tasks.map(task => task.status), ['completed', 'completed', 'assigned', 'assigned']);
  const runIds = paused.tasks.slice(0, 2).map(task => task.runId); assert.ok(runIds.every(Boolean));
  assert.deepEqual(paused.tasks.slice(0, 2).map(task => task.workspaces), [[], []]); assert.equal(taskCalls, 2); assert.equal(paused.callsUsed, 3);
  manager.resume(room.id); const completed = await manager.wait(room.id);
  assert.equal(completed.status, 'completed'); assert.equal(taskCalls, 4); assert.equal(requests.length, 5);
  assert.deepEqual(completed.tasks.slice(0, 2).map(task => task.runId), runIds);
});

test('paused write worktree evidence survives reopening and its write is never replayed', { timeout: 60000 }, async t => {
  const entered = deferred(), finish = deferred(); let writeCalls = 0;
  const { manager, peer, service } = fixture(t, async request => {
    if (data(request)) return complete(request, ballot(data(request)));
    if (request.permission === 'write') {
      writeCalls++; writeFileSync(join(request.cwd, 'retained.txt'), 'Retained prerequisite work.\n');
      entered.resolve(); await finish.promise; return complete(request, 'FULL_PAUSED_WRITE_EVIDENCE');
    }
    assert.match(request.prompt, /FULL_PAUSED_WRITE_EVIDENCE/); assert.match(request.prompt, /retained workflow\/worktree\/patch/);
    return complete(request, 'Inspected retained worktree metadata without claiming automatic integration.');
  }, { git: true, maxConcurrency: 1 });
  const room = manager.create(input({ tasks: [task('Write prerequisite', { permission: 'write' }), task('Verify prerequisite', { dependsOn: ['task-1'] })] }));
  manager.startDiscussion(room.id);
  await Promise.race([entered.promise, manager.wait(room.id).then(result => assert.fail(`Writer was not admitted: ${JSON.stringify(result)}`))]);
  manager.pause(room.id); finish.resolve();
  const paused = await manager.wait(room.id), artifact = paused.tasks[0].workspaces[0], runId = paused.tasks[0].runId;
  assert.equal(paused.status, 'paused'); assert.equal(artifact.exportStatus, 'ready'); assert.ok(artifact.patchPath);
  assert.deepEqual(paused.tasks[0].workspaces, service.getRun(runId).workspaces); await manager.close();
  const reopened = peer(); assert.deepEqual(reopened.get(room.id).tasks[0].workspaces, [artifact]);
  reopened.resume(room.id); const completed = await reopened.wait(room.id);
  assert.equal(completed.status, 'completed'); assert.equal(writeCalls, 1); assert.equal(completed.tasks[0].runId, runId);
});

test('ready pause and resume preserve approval without dispatch and backend provenance is checked', async t => {
  const { manager, requests, otherBackend } = fixture(t), room = manager.create(input({ autoExecute: false }));
  manager.startDiscussion(room.id); const ready = await manager.wait(room.id);
  const paused = manager.pause(room.id); assert.equal(paused.status, 'paused'); assert.deepEqual(paused.checkpoint, { phase: 'ready' });
  assert.throws(() => otherBackend().resume(room.id), /differs from this service backend/); assert.equal(requests.length, 1);
  const restored = manager.resume(room.id);
  assert.equal(restored.status, 'ready'); assert.deepEqual(restored.decision, ready.decision); assert.equal(requests.length, 1);
  manager.execute(room.id); assert.equal((await manager.wait(room.id)).status, 'completed'); assert.equal(requests.length, 2);
});

test('guidance on a paused room invalidates approval and requires a fresh discussion', async t => {
  const { manager, peer, requests } = fixture(t), room = manager.create(input({ autoExecute: false }));
  manager.startDiscussion(room.id); await manager.wait(room.id); manager.pause(room.id);
  const changed = peer().postMessage(room.id, { author: 'PM', body: 'Revised acceptance criteria must be considered before execution.' });
  assert.equal(changed.status, 'needs_attention'); assert.equal(changed.checkpoint, undefined); assert.equal(changed.decision, undefined);
  assert.equal(changed.tasks[0].status, 'pending'); assert.equal(changed.tasks[0].assignee, undefined);
  assert.throws(() => manager.resume(room.id), /Only a durably paused room/); assert.equal(requests.length, 1);
  manager.startDiscussion(room.id); const ready = await manager.wait(room.id);
  assert.equal(ready.status, 'ready'); assert.equal(ready.decision.instructionRevision, 1); assert.equal(requests.length, 2);
});

test('foreign pause accepted immediately before admission is checked in the same transaction as dispatch', async t => {
  const { manager, service, peer, requests } = fixture(t), other = peer(), room = manager.create(input({}));
  const nativeMutate = manager.store.mutate.bind(manager.store); let injected = false;
  manager.store.mutate = (id, change, options) => {
    if (!injected && options?.owner && manager.get(id).status === 'running') { injected = true; other.pause(id); }
    return nativeMutate(id, change, options);
  };
  const start = service.start.bind(service);
  service.start = value => { assert.equal(manager.store.db.isTransaction, true); assert.notEqual(manager.get(room.id).pauseRequested, true); return start(value); };
  manager.startDiscussion(room.id); const paused = await manager.wait(room.id);
  assert.equal(injected, true); assert.equal(paused.status, 'paused'); assert.equal(requests.length, 1); assert.equal(paused.tasks[0].runId, undefined);
});

test('accepted cancellation outranks a pending pause and permanently rejects resume', async t => {
  const entered = deferred(), finish = deferred(); let signal;
  const { manager, peer } = fixture(t, async request => { signal = request.signal; entered.resolve(); await finish.promise;
    return { ...complete(request, 'Retained cancellation evidence.'), status: 'cancelled' }; });
  const room = manager.create(input({})); manager.startDiscussion(room.id); await entered.promise;
  manager.pause(room.id); peer().cancel(room.id); assert.equal(signal.aborted, false, 'Foreign cancellation is observed by the bounded owner poll.');
  finish.resolve(); const cancelled = await manager.wait(room.id);
  assert.equal(cancelled.status, 'cancelled'); assert.equal(cancelled.checkpoint, undefined); assert.equal(cancelled.pauseRequested, undefined);
  assert.throws(() => manager.resume(room.id), /Only a durably paused room/);
});

test('task failure and changed instructions outrank pause after admitted work drains', async t => {
  for (const outcome of ['failure', 'instructions']) {
    const entered = deferred(), finish = deferred();
    const { manager, peer, requests } = fixture(t, async request => {
      if (data(request)) return complete(request, ballot(data(request)));
      entered.resolve(); await finish.promise;
      return outcome === 'failure' ? { ...complete(request, 'Partial failed evidence.'), status: 'failed', error: 'Offline task failure.' }
        : complete(request, 'Complete work under the original instructions.');
    }, { maxConcurrency: 1 });
    const room = manager.create(input({ tasks: [task('First'), task('Second')] })); manager.startDiscussion(room.id); await entered.promise;
    manager.pause(room.id);
    if (outcome === 'instructions') peer().postMessage(room.id, { author: 'PM', body: 'Stop further admission until fresh discussion.' });
    finish.resolve(); const result = await manager.wait(room.id);
    assert.equal(result.status, outcome === 'failure' ? 'failed' : 'needs_attention'); assert.equal(result.checkpoint, undefined);
    assert.equal(result.pauseRequested, undefined); assert.equal(requests.length, 2); assert.equal(result.tasks[1].runId, undefined);
  }
});

test('a synchronous resume subscriber cannot remove the newly queued job from lifecycle tracking', async t => {
  const entered = deferred(), finish = deferred(); let resumed = false;
  const { manager, requests } = fixture(t, async request => {
    if (data(request)) return complete(request, ballot(data(request)));
    entered.resolve(); await finish.promise; return complete(request, 'Completed the single resumed task.');
  });
  const room = manager.create(input({}));
  manager.subscribe(value => { if (value.id === room.id && value.status === 'paused' && !resumed) { resumed = true; manager.resume(room.id); } });
  manager.startDiscussion(room.id); manager.pause(room.id); await entered.promise;
  assert.equal(resumed, true); assert.equal(manager.jobs.has(room.id), true); assert.equal(manager.get(room.id).status, 'running');
  finish.resolve(); assert.equal((await manager.wait(room.id)).status, 'completed'); assert.equal(requests.length, 2);
});

test('legacy active owners without a durable checkpoint refuse pause without claiming it was accepted', t => {
  const { manager } = fixture(t), room = manager.create(input({}));
  manager.store.mutate(room.id, stored => { stored.status = 'running'; }, { claim: { id: 'legacy-owner', pid: process.pid } });
  assert.throws(() => manager.pause(room.id), /owner does not support durable pause checkpoints/);
  assert.equal(manager.get(room.id).pauseRequested, undefined);
  manager.store.mutate(room.id, stored => { stored.status = 'failed'; }, { owner: 'legacy-owner', release: true });
});

test('pause during a manual discussion settles to paused ready with no pending request marker', async t => {
  const entered = deferred(), finish = deferred();
  const { manager, requests } = fixture(t, async request => { entered.resolve(); await finish.promise; return complete(request, ballot(data(request))); });
  const room = manager.create(input({ autoExecute: false })); manager.startDiscussion(room.id); await entered.promise;
  manager.pause(room.id); finish.resolve(); const paused = await manager.wait(room.id);
  assert.equal(paused.status, 'paused'); assert.deepEqual(paused.checkpoint, { phase: 'ready' }); assert.equal(paused.pauseRequested, undefined);
  assert.equal(manager.resume(room.id).status, 'ready'); assert.equal(requests.length, 1);
});

test('final admitted completion and invalid final ballots outrank pause and clear all continuation markers', async t => {
  for (const outcome of ['completed', 'invalid-ballot']) {
    const entered = deferred(), finish = deferred();
    const { manager } = fixture(t, async request => {
      if (data(request) && outcome === 'completed') return complete(request, ballot(data(request)));
      entered.resolve(); await finish.promise;
      return complete(request, outcome === 'completed' ? 'Verified final task.' : 'Incomplete ballot prose.');
    });
    const room = manager.create(input({})); manager.startDiscussion(room.id); await entered.promise;
    manager.pause(room.id); finish.resolve(); const result = await manager.wait(room.id);
    assert.equal(result.status, outcome === 'completed' ? 'completed' : 'needs_attention');
    assert.equal(result.checkpoint, undefined); assert.equal(result.pauseRequested, undefined);
  }
});

test('pause drains queued internal calls of an already admitted discussion workflow', async t => {
  const entered = deferred(), finish = deferred(); const signals = [];
  const { manager, requests } = fixture(t, async request => {
    signals.push(request.signal);
    if (request.agent === 'codex') { entered.resolve(); await finish.promise; }
    return complete(request, ballot(data(request)));
  }, { maxConcurrency: 1 });
  const room = manager.create(input({ members: [{ agent: 'codex', role: 'lead' }, { agent: 'claude', role: 'worker' }] }));
  manager.startDiscussion(room.id); await entered.promise; manager.pause(room.id);
  assert.equal(requests.length, 1); finish.resolve(); const paused = await manager.wait(room.id);
  assert.equal(paused.status, 'paused'); assert.deepEqual(paused.checkpoint, { phase: 'execution' });
  assert.equal(paused.callsUsed, 2); assert.equal(requests.length, 2); assert.ok(signals.every(signal => !signal.aborted));
  assert.equal(paused.messages.filter(message => message.kind === 'opinion').length, 2);
  await delay(120); assert.equal(requests.length, 2, 'No task workflow is admitted after the paused checkpoint.');
});

test('two managers racing to resume a checkpoint admit its remaining work only once', async t => {
  const { manager, peer, requests } = fixture(t), other = peer(), room = manager.create(input({}));
  manager.startDiscussion(room.id); manager.pause(room.id); assert.equal((await manager.wait(room.id)).status, 'paused');
  const mutate = manager.store.mutate.bind(manager.store); let raced = false;
  manager.store.mutate = (id, change, options) => {
    if (!raced && options?.claim) { raced = true; other.resume(id); }
    return mutate(id, change, options);
  };
  assert.throws(() => manager.resume(room.id), /already active|cannot be changed while active/i);
  assert.equal(raced, true); assert.equal((await other.wait(room.id)).status, 'completed'); assert.equal(requests.length, 2);
});

test('storage failure while a pause drains exposes failed evidence without stale pause markers', async t => {
  const entered = deferred(), finish = deferred();
  const { manager } = fixture(t, async request => { entered.resolve(); await finish.promise; return complete(request, ballot(data(request))); });
  const room = manager.create(input({})); manager.startDiscussion(room.id); await entered.promise; manager.pause(room.id);
  const mutate = manager.store.mutate.bind(manager.store);
  manager.store.mutate = () => { throw new Error('Injected room persistence outage while draining pause.'); };
  finish.resolve(); const failed = await manager.wait(room.id);
  manager.store.mutate = mutate;
  assert.equal(failed.status, 'failed'); assert.equal(failed.pauseRequested, undefined); assert.equal(failed.checkpoint, undefined);
  assert.match(failed.error, /persistence failed/); assert.throws(() => manager.resume(room.id), /persistence failed/);
});
