import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { setImmediate as nextTurn } from 'node:timers/promises';
import { WorkflowEngine } from '../dist/engine.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { GitWorkspaces } from '../dist/workspaces.js';
import { ProcessCleanupError, PROCESS_CLEANUP_ERROR } from '../dist/process.js';

const task = extra => ({ mode: 'delegate', task: 'Verify bounded offline cleanup failure handling.', agents: ['codex'], permission: 'read', ...extra });
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; };
const result = (agent, extra = {}) => ({ agent, status: 'failed', output: 'Retained partial evidence.', error: PROCESS_CLEANUP_ERROR, durationMs: 1, ...extra });
function fixture(t) {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-cleanup-test-'))), services = [];
  t.after(async () => {
    await Promise.all(services.map(service => service.close()));
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-cleanup-test-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  const config = loadConfig(root, { backend: 'mock', maxConcurrency: 1, maxCalls: 20 });
  return { root, config, create(runner) { const service = new CouncilService(config, runner); services.push(service); return service; } };
}
function memoryStore() {
  const runs = new Map(), events = [];
  return { runs, log: events, saveRun: run => runs.set(run.id, structuredClone(run)), getRun: id => runs.get(id), appendEvent: (runId, type, data) => events.push({ runId, type, data }) };
}

test('cleanup failure fences local queued dispatch before releasing its permit and outranks cancellation', { timeout: 10000 }, async t => {
  const { config } = fixture(t), entered = deferred(), release = deferred(), controller = new AbortController(), store = memoryStore();
  let calls = 0, notified = 0, stateFailures = 0;
  const runner = { doctor: async () => [], run: async request => { calls++; entered.resolve(); await release.promise; return result(request.agent); } };
  const engine = new WorkflowEngine(config, runner, store, {
    onProcessCleanupFailure(error) { notified++; assert.equal(error.message, PROCESS_CLEANUP_ERROR); throw new Error('Deliberate notification failure.'); },
    onStateFailure() { stateFailures++; },
  });
  const first = engine.run(task(), { signal: controller.signal });
  try {
    await entered.promise;
    const second = engine.run(task()); await nextTurn();
    assert.equal(calls, 1); assert.equal(store.runs.size, 2);
    controller.abort(); release.resolve();
    const outcomes = await Promise.all([first, second]);
    assert.deepEqual(outcomes.map(run => run.status), ['failed', 'failed']);
    assert.ok(outcomes.every(run => run.error === PROCESS_CLEANUP_ERROR));
    assert.equal(outcomes[0].steps[0].result.status, 'failed'); assert.equal(outcomes[1].steps.length, 0);
    assert.equal((await engine.run(task())).status, 'failed');
    assert.equal(calls, 1); assert.equal(notified, 1); assert.equal(stateFailures, 0);
  } finally { release.resolve(); await first; }
});

test('cleanup failure takes precedence over a dispatch protection change during the active call', { timeout: 10000 }, async t => {
  const { config } = fixture(t), entered = deferred(), release = deferred();
  let blocked = false;
  t.mock.timers.enable({ apis: ['setInterval'] });
  const engine = new WorkflowEngine(config, { doctor: async () => [], run: async request => { entered.resolve(); await release.promise; return result(request.agent); } }, memoryStore(), {
    assertAgentDispatch() { if (blocked) throw new Error('Offline subscription protection changed.'); },
  });
  const running = engine.run(task());
  try {
    await entered.promise; blocked = true; t.mock.timers.tick(250); release.resolve();
    const run = await running;
    assert.equal(run.status, 'failed'); assert.equal(run.error, PROCESS_CLEANUP_ERROR);
    assert.equal(run.steps[0].result.error, PROCESS_CLEANUP_ERROR);
  } finally { release.resolve(); t.mock.timers.reset(); await running; }
});

test('a service degrades, drains its queued run and retains failed cleanup after durable cancellation', { timeout: 10000 }, async t => {
  const { create } = fixture(t), entered = deferred(), release = deferred();
  let calls = 0;
  const service = create({ doctor: async () => [], run: async request => { calls++; entered.resolve(); await release.promise; return result(request.agent); } });
  const first = service.start(task());
  try {
    await entered.promise; const second = service.start(task()); await nextTurn();
    assert.equal(service.requestCancellation(first.id).requested, true); release.resolve();
    const runs = await Promise.all([service.completion(first.id), service.completion(second.id)]);
    assert.deepEqual(runs.map(run => run.status), ['failed', 'failed']); assert.equal(calls, 1);
    assert.ok(runs.every(run => run.error === PROCESS_CLEANUP_ERROR));
    const health = service.health(); assert.equal(health.status, 'degraded'); assert.match(health.issues[0], /cleanup could not be verified/);
    assert.equal(health.state.counts.slots, 0); assert.equal(health.state.counts.slotWaiters, 0); assert.equal(health.state.counts.activeRuns, 0);
    assert.throws(() => service.start(task()), /cleanup could not be verified/);
  } finally { release.resolve(); await service.completion(first.id); }
});

test('a later evidence persistence failure cannot replace the first process cleanup host fault', { timeout: 10000 }, async t => {
  const { create } = fixture(t), service = create({ doctor: async () => [], run: async request => result(request.agent) });
  const append = service.store.appendEvent;
  t.mock.method(service.store, 'appendEvent', function (id, type, data) {
    if (type === 'step.finished') throw new Error('Deliberate offline step event failure.');
    return append.call(this, id, type, data);
  });
  const run = await service.run(task()); assert.equal(run.status, 'failed');
  const health = service.health(); assert.equal(health.status, 'degraded');
  assert.match(health.issues[0], /cleanup could not be verified/); assert.doesNotMatch(health.issues[0], /state persistence failed/);
  assert.equal(run.steps[0].result.error, PROCESS_CLEANUP_ERROR);
});

for (const peerReply of [false, true]) {
  test(`private ${peerReply ? 'reply' : 'request'} cleanup failure keeps only its generic diagnostic public`, { timeout: 10000 }, async t => {
    const { create } = fixture(t), envelope = JSON.stringify({ council_private_chat_request: { peer: 'claude', message: 'PRIVATE_QUESTION', context: 'PRIVATE_CONTEXT' } });
    let calls = 0;
    const service = create({ doctor: async () => [], run: async request => {
      calls++;
      if (peerReply && calls === 1) return result(request.agent, { status: 'completed', output: envelope, error: undefined });
      return result(request.agent, { output: peerReply ? 'PRIVATE_REPLY' : envelope, error: PROCESS_CLEANUP_ERROR + ' Earlier failure: PRIVATE_DIAGNOSTIC' });
    } });
    const privateChats = service.privateChats; // Retain read access for evidence after this host fences itself.
    const run = await service.run(task({ privateChats: { codex: { peers: ['claude'], maxRequests: 1 } } }));
    assert.equal(run.status, 'failed'); assert.equal(run.error, PROCESS_CLEANUP_ERROR); assert.equal(calls, peerReply ? 2 : 1);
    assert.equal(run.steps.at(-1).result.error, PROCESS_CLEANUP_ERROR);
    const publicEvidence = JSON.stringify({ run, events: service.store.events(run.id), health: service.health() });
    for (const marker of ['PRIVATE_QUESTION', 'PRIVATE_CONTEXT', 'PRIVATE_REPLY', 'PRIVATE_DIAGNOSTIC']) assert.equal(publicEvidence.includes(marker), false, marker);
    if (peerReply) {
      const actor = `run:${run.id}:codex`, chats = privateChats.list(actor);
      assert.equal(chats.length, 1);
      const reply = JSON.parse(privateChats.page({ chatId: chats[0].id, actor }).messages.at(-1).body);
      assert.equal(reply.status, 'failed'); assert.equal(reply.error, PROCESS_CLEANUP_ERROR); assert.equal(reply.body, 'PRIVATE_REPLY');
    }
  });
}

test('cleanup uncertainty leaves the isolated worktree pending inspection without automatically exporting', { timeout: 10000 }, async t => {
  const { root, config } = fixture(t), path = join(root, '.council', 'worktrees', 'retained'), patchPath = join(root, '.council', 'patches', 'retained.patch');
  mkdirSync(path, { recursive: true });
  const handle = { agent: 'codex', path, cwd: path, patchPath, baseCommit: 'a'.repeat(40), baseHead: 'a'.repeat(40), changed: false };
  t.mock.method(GitWorkspaces.prototype, 'create', async (_id, _agent, _slot, _signal, prepared) => { prepared({ ...handle, exportStatus: 'pending' }); return handle; });
  const exported = t.mock.method(GitWorkspaces.prototype, 'export', async () => { assert.fail('A worktree with unverified process cleanup must not be exported automatically.'); });
  const controller = new AbortController();
  const engine = new WorkflowEngine(config, { doctor: async () => [], run: async request => {
    writeFileSync(join(request.cwd, 'retained.txt'), 'Preserve work until process cleanup is inspected.'); controller.abort(); return result(request.agent);
  } }, memoryStore());
  const run = await engine.run(task({ permission: 'write' }), { signal: controller.signal });
  assert.equal(run.status, 'failed'); assert.equal(exported.mock.callCount(), 0); assert.equal(run.workspaces.length, 1);
  assert.equal(run.workspaces[0].path, path); assert.equal(run.workspaces[0].exportStatus, 'pending');
  assert.match(run.workspaces[0].exportError, /Inspect the retained worktree/); assert.equal(existsSync(patchPath), false);
  assert.equal(readFileSync(join(path, 'retained.txt'), 'utf8'), 'Preserve work until process cleanup is inspected.');
});

test('ordinary task text and non-prefix provider failures do not falsely fence a host', { timeout: 10000 }, async t => {
  const { create } = fixture(t); let calls = 0;
  const service = create({ doctor: async () => [], run: async request => {
    calls++;
    return result(request.agent, calls === 2 ? { error: 'Ordinary provider failure quoting: ' + PROCESS_CLEANUP_ERROR }
      : { status: 'completed', output: PROCESS_CLEANUP_ERROR, error: undefined });
  } });
  assert.equal((await service.run(task())).status, 'completed'); assert.equal((await service.run(task())).status, 'failed');
  assert.equal((await service.run(task())).status, 'completed'); assert.equal(calls, 3); assert.equal(service.health().status, 'ok');
});

for (const phase of ['create', 'export']) for (const cancel of [false, true]) {
  test(`trusted ${phase} helper cleanup failure fences the engine${cancel ? ' despite cancellation' : ''}`, { timeout: 10000 }, async t => {
    const { root, config } = fixture(t), path = join(root, 'retained-helper'), patchPath = join(root, 'retained.patch'), controller = new AbortController(), persistence = memoryStore();
    let calls = 0, notifications = 0, stateFailures = 0;
    const handle = { agent: 'codex', path, cwd: path, patchPath, baseCommit: 'a'.repeat(40), baseHead: 'a'.repeat(40), changed: false };
    const fail = () => { if (cancel) controller.abort(); throw new ProcessCleanupError(); };
    t.mock.method(GitWorkspaces.prototype, 'create', async (_id, _agent, _slot, _signal, prepared) => {
      mkdirSync(path); writeFileSync(join(path, 'retained.txt'), 'Keep helper preparation evidence.');
      prepared({ ...handle, exportStatus: 'pending' });
      if (phase === 'create') fail();
      return handle;
    });
    const exported = t.mock.method(GitWorkspaces.prototype, 'export', async () => fail());
    const engine = new WorkflowEngine(config, { doctor: async () => [], run: async request => { calls++; return result(request.agent, { status: 'completed', error: undefined }); } }, persistence, {
      onProcessCleanupFailure(error) { notifications++; assert.equal(error.message, PROCESS_CLEANUP_ERROR); }, onStateFailure() { stateFailures++; },
    });
    const run = await engine.run(task({ permission: 'write' }), { signal: controller.signal });
    assert.equal(run.status, 'failed'); assert.equal(run.error, PROCESS_CLEANUP_ERROR);
    assert.equal(notifications, 1); assert.equal(stateFailures, 0); assert.equal(calls, phase === 'create' ? 0 : 1);
    assert.equal(exported.mock.callCount(), phase === 'create' ? 0 : 1);
    assert.equal(run.workspaces[0].path, path); assert.equal(run.workspaces[0].exportStatus, 'pending');
    assert.match(run.workspaces[0].exportError, /Inspect the retained worktree/); assert.equal(existsSync(patchPath), false);
    assert.equal(readFileSync(join(path, 'retained.txt'), 'utf8'), 'Keep helper preparation evidence.');
    assert.equal(persistence.log.some(event => event.type === 'workspace.exported'), false);
    assert.equal((await engine.run(task())).status, 'failed'); assert.equal(calls, phase === 'create' ? 0 : 1); assert.equal(notifications, 1);
  });
}

for (const phase of ['create', 'export']) {
  test(`a textual ${phase} helper error cannot impersonate a trusted cleanup failure`, { timeout: 10000 }, async t => {
    const { root, config } = fixture(t), path = join(root, 'ordinary-helper'); let notifications = 0;
    t.mock.method(GitWorkspaces.prototype, 'create', async (_id, _agent, _slot, _signal, prepared) => {
      if (phase === 'create') throw new Error(PROCESS_CLEANUP_ERROR);
      mkdirSync(path); const handle = { agent: 'codex', path, cwd: path, patchPath: join(root, 'ordinary.patch'), baseCommit: 'a'.repeat(40), baseHead: 'a'.repeat(40), changed: false };
      prepared({ ...handle, exportStatus: 'pending' }); return handle;
    });
    t.mock.method(GitWorkspaces.prototype, 'export', async () => { throw new Error(PROCESS_CLEANUP_ERROR); });
    const engine = new WorkflowEngine(config, { doctor: async () => [], run: async request => result(request.agent, { status: 'completed', error: undefined }) }, memoryStore(), {
      onProcessCleanupFailure() { notifications++; },
    });
    assert.equal((await engine.run(task({ permission: 'write' }))).status, 'failed');
    assert.equal((await engine.run(task())).status, 'completed'); assert.equal(notifications, 0);
  });
}

test('a concurrent cleanup failure prevents an in-flight export from publishing a ready artifact', { timeout: 10000 }, async t => {
  const { root, config } = fixture(t), path = join(root, 'concurrent-export'), exporting = deferred(), release = deferred(), controller = new AbortController(), persistence = memoryStore();
  t.mock.method(GitWorkspaces.prototype, 'create', async (_id, _agent, _slot, _signal, prepared) => {
    mkdirSync(path); const handle = { agent: 'codex', path, cwd: path, patchPath: join(root, 'concurrent.patch'), baseCommit: 'a'.repeat(40), baseHead: 'a'.repeat(40), changed: false };
    prepared({ ...handle, exportStatus: 'pending' }); return handle;
  });
  const exported = t.mock.method(GitWorkspaces.prototype, 'export', async (handle, signal) => {
    exporting.resolve(); await release.promise; signal.throwIfAborted(); return { ...handle, changed: true, exportStatus: 'ready' };
  });
  const engine = new WorkflowEngine({ ...config, maxConcurrency: 2 }, { doctor: async () => [], run: async request => result(request.agent,
    request.prompt.includes('SECOND_CLEANUP') ? {} : { status: 'completed', error: undefined }) }, persistence, { onProcessCleanupFailure() { controller.abort(); } });
  const first = engine.run(task({ permission: 'write', task: 'FIRST_EXPORT' }), { signal: controller.signal });
  try {
    await exporting.promise; assert.equal((await engine.run(task({ task: 'SECOND_CLEANUP' }))).status, 'failed'); release.resolve();
    const run = await first; assert.equal(run.status, 'failed'); assert.equal(run.error, PROCESS_CLEANUP_ERROR);
    assert.equal(controller.signal.aborted, true); assert.equal(exported.mock.callCount(), 1); assert.equal(run.workspaces[0].path, path);
    assert.equal(run.workspaces[0].exportStatus, 'pending'); assert.equal(run.workspaces[0].changed, false); assert.match(run.workspaces[0].exportError, /Inspect the retained worktree/);
    assert.equal(persistence.log.some(event => event.runId === run.id && event.type === 'workspace.exported'), false);
  } finally { release.resolve(); await first; }
});

test('an explicit trusted cleanup fence is idempotent and prevents even write preparation', { timeout: 10000 }, async t => {
  const { config } = fixture(t); let notified = 0;
  const created = t.mock.method(GitWorkspaces.prototype, 'create', async () => { assert.fail('A fenced engine must not start Git preparation.'); });
  const engine = new WorkflowEngine(config, { doctor: async () => [], run: async () => { assert.fail('A fenced engine must not dispatch a runner.'); } }, memoryStore(), {
    onProcessCleanupFailure() { notified++; engine.fenceProcessCleanup(); },
  });
  engine.fenceProcessCleanup(); engine.fenceProcessCleanup();
  const run = await engine.run(task({ permission: 'write' }));
  assert.equal(run.status, 'failed'); assert.equal(run.error, PROCESS_CLEANUP_ERROR); assert.equal(notified, 1); assert.equal(created.mock.callCount(), 0);
});
