import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { WorkflowEngine, WorkflowPersistenceError } from '../dist/engine.js';
import { CouncilService } from '../dist/service.js';
import { CouncilStore } from '../dist/store.js';
import { GitWorkspaces } from '../dist/workspaces.js';
import { ProcessCleanupError, PROCESS_CLEANUP_ERROR } from '../dist/process.js';
import { DispatchQuarantinedError, QUARANTINE_PERSISTENCE_WARNING, QUARANTINE_INSPECTION_REQUIRED } from '../dist/dispatch-quarantine.js';

const config = (extra = {}) => ({ version: 1, workspaceRoot: process.cwd(), stateDir: join(process.cwd(), '.council'), backend: 'mock', maxConcurrency: 2,
  timeoutMs: 1000, maxOutputBytes: 4096, maxCalls: 20, agents: { codex: { enabled: true, profile: 'codex' }, claude: { enabled: true, profile: 'claude' } }, ...extra });
const task = extra => ({ mode: 'delegate', task: 'Offline durable cleanup integration.', agents: ['codex'], permission: 'read', ...extra });
const answer = (agent, extra = {}) => ({ agent, status: 'completed', output: 'Offline evidence.', durationMs: 1, ...extra });
const cleanup = agent => answer(agent, { status: 'failed', error: PROCESS_CLEANUP_ERROR });
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; };
const refusal = () => new DispatchQuarantinedError({ blocked: true, currentRevision: '9007199254740993', openCount: 1 });
function memory(extra = {}) {
  const runs = new Map(), log = [];
  return { runs, log, saveRun: run => runs.set(run.id, structuredClone(run)), getRun: id => runs.get(id), appendEvent: (runId, type, data) => log.push({ runId, type, data }), ...extra };
}
function shared(extra = {}) {
  const reports = [], order = [], slotOwners = new Map(); let blocked = false, slots = 0;
  const store = memory({
    reports, order,
    acquireSlot(_instance, runId) { if (blocked) throw refusal(); const token = `slot-${++slots}`; slotOwners.set(token, runId); return token; },
    releaseSlot(token) { order.push(['release', token, slotOwners.get(token)]); }, cancelSlotWaiter() {},
    assertDispatchUnblocked() { if (blocked) throw refusal(); },
    reportProcessCleanupIncident(input) { reports.push(structuredClone(input)); order.push(['report', input.reportKey, input.runId]); blocked = true;
      return { ...input, id: `incident-${reports.length}`, generation: String(reports.length), reason: 'process_cleanup_unverified', observedAt: '2026-11-04T00:00:00.000Z' }; }, ...extra,
  });
  return store;
}
function fixture(t) {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-engine-quarantine-')));
  t.after(() => { assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-engine-quarantine-')); rmSync(root, { recursive: true, force: true }); });
  return root;
}

test('each actual cleanup report is persisted before its shared call permit is released', async () => {
  const store = shared(); let notified = 0;
  const engine = new WorkflowEngine(config(), { doctor: async () => [], run: async request => cleanup(request.agent) }, store, {
    instanceId: 'host', onProcessCleanupFailure() { notified++; throw new Error('Notification is deliberately unavailable.'); },
  });
  const run = await engine.run(task()); assert.equal(run.status, 'failed'); assert.equal(store.reports.length, 1); assert.equal(notified, 1);
  assert.deepEqual(store.order.map(entry => entry[0]), ['report', 'release']);
  assert.equal(store.reports[0].instanceId, 'host'); assert.equal(store.reports[0].runId, run.id); assert.equal(store.reports[0].stage, 'agent_call');
  assert.equal(typeof store.reports[0].reportKey, 'string'); assert.ok(store.reports[0].reportKey.length > 0);
  assert.deepEqual(Object.keys(store.reports[0]).sort(), ['instanceId', 'reportKey', 'runId', 'stage']);
  assert.equal(run.error.includes(QUARANTINE_PERSISTENCE_WARNING), false);
});

test('already active calls each report their own uncertainty after the once-only host fence', { timeout: 10000 }, async () => {
  const entered = deferred(), release = deferred(), store = shared(); let calls = 0, notified = 0;
  const engine = new WorkflowEngine(config(), { doctor: async () => [], run: async request => { if (++calls === 2) entered.resolve(); await release.promise; return cleanup(request.agent); } }, store,
    { instanceId: 'host', onProcessCleanupFailure() { notified++; } });
  const first = engine.run(task()), second = engine.run(task({ agents: ['claude'] }));
  try {
    await entered.promise; release.resolve(); const runs = await Promise.all([first, second]);
    assert.ok(runs.every(run => run.status === 'failed')); assert.equal(calls, 2); assert.equal(notified, 1); assert.equal(store.reports.length, 2);
    assert.equal(new Set(store.reports.map(report => report.reportKey)).size, 2); assert.equal(new Set(store.reports.map(report => report.runId)).size, 2);
    const releases = store.order.filter(entry => entry[0] === 'release'); assert.equal(releases.length, 2);
    for (const released of releases) {
      const report = store.order.findIndex(entry => entry[0] === 'report' && entry[2] === released[2]);
      assert.ok(report >= 0 && report < store.order.indexOf(released), 'Every owned cleanup report must precede its own slot release.');
    }
  } finally { release.resolve(); await Promise.all([first, second]); }
});

for (const missing of [false, true]) {
  test(`a ${missing ? 'missing' : 'failed'} mandatory report preserves a safe warning across affected runs`, async () => {
    let warnings = 0, calls = 0;
    const store = shared({ reportProcessCleanupIncident: missing ? undefined : () => { throw new Error('PRIVATE_REPORT_FAILURE'); } });
    const engine = new WorkflowEngine(config(), { doctor: async () => [], run: async request => { calls++; return cleanup(request.agent); } }, store, {
      instanceId: 'host', onIncidentPersistenceFailure(error) { warnings++; assert.equal(error.message, QUARANTINE_PERSISTENCE_WARNING); throw new Error('Ignored notification failure.'); },
    });
    const first = await engine.run(task()), second = await engine.run(task());
    for (const run of [first, second]) { assert.equal(run.status, 'failed'); assert.ok(run.error.includes(QUARANTINE_PERSISTENCE_WARNING)); assert.equal(JSON.stringify(run).includes('PRIVATE_REPORT_FAILURE'), false); }
    assert.equal(calls, 1); assert.equal(warnings, 1); assert.equal(store.order.at(-1)[0], 'release');
  });
}

test('a persistence-failure snapshot retains the fixed warning when terminal evidence also fails', async () => {
  const store = shared({ reportProcessCleanupIncident() { throw new Error('PRIVATE_INCIDENT_FAILURE'); } }), save = store.saveRun;
  store.saveRun = run => { if (run.status === 'failed') throw new Error('PRIVATE_FINAL_FAILURE'); return save(run); };
  const engine = new WorkflowEngine(config(), { doctor: async () => [], run: async request => cleanup(request.agent) }, store, { instanceId: 'host' });
  await assert.rejects(engine.run(task()), error => {
    assert.ok(error instanceof WorkflowPersistenceError); assert.equal(error.run.status, 'failed'); assert.ok(error.run.error.includes(QUARANTINE_PERSISTENCE_WARNING));
    assert.equal(JSON.stringify(error.run).includes('PRIVATE_INCIDENT_FAILURE'), false); assert.equal(JSON.stringify(error.run).includes('PRIVATE_FINAL_FAILURE'), false); return true;
  });
});

test('foreign shared-slot refusal is operational and never creates an owned incident or state fault', async () => {
  let reports = 0, stateFailures = 0, calls = 0, cancelledWaiters = 0, diagnosis;
  const store = memory({ acquireSlot() { throw refusal(); }, releaseSlot() { assert.fail('No slot was admitted.'); }, cancelSlotWaiter() { cancelledWaiters++; },
    reportProcessCleanupIncident() { reports++; assert.fail('A foreign observation is not an owned report.'); } });
  const run = await new WorkflowEngine(config(), { doctor: async () => [], run: async request => { calls++; return answer(request.agent); } }, store,
    { instanceId: 'host', onStateFailure() { stateFailures++; }, onProcessCleanupFailure(error) { diagnosis = error; } }).run(task());
  assert.equal(run.status, 'failed'); assert.equal(calls, 0); assert.equal(reports, 0); assert.equal(stateFailures, 0); assert.equal(cancelledWaiters, 1);
  assert.ok(diagnosis instanceof DispatchQuarantinedError); assert.equal(diagnosis.receipt.currentRevision, '9007199254740993');
  const blocked = store.log.find(event => event.type === 'step.dispatch_blocked'); assert.equal(blocked.data.code, 'COUNCIL_DISPATCH_QUARANTINED');
  assert.equal(blocked.data.receipt.currentRevision, '9007199254740993'); assert.equal(run.error.includes(QUARANTINE_PERSISTENCE_WARNING), false);
});

test('a service observing a foreign incident exposes its workspace diagnosis without reporting its own cleanup', { timeout: 10000 }, async t => {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-foreign-diagnosis-'))), entered = deferred();
  const observer = new CouncilService(config({ workspaceRoot: root, stateDir: join(root, '.council') }), {
    doctor: async () => [], run: request => new Promise(done => {
      request.signal.addEventListener('abort', () => done(answer(request.agent, { status: 'cancelled' })), { once: true }); entered.resolve();
    }),
  });
  const writer = new CouncilStore(join(root, '.council'));
  writer.registerInstance('reporting-host', process.pid);
  t.after(async () => {
    await observer.close(); writer.unregisterInstance('reporting-host'); writer.close();
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-foreign-diagnosis-'));
    rmSync(root, { recursive: true, force: true });
  });
  const observedReports = t.mock.method(observer.store, 'reportProcessCleanupIncident', () => { assert.fail('Observing a foreign report must not create an own incident.'); });
  const queued = observer.start(task()); await entered.promise;
  const incident = writer.reportProcessCleanupIncident({ reportKey: 'foreign-diagnosis', instanceId: 'reporting-host', stage: 'agent_call' });
  const run = await observer.completion(queued.id); assert.equal(run.status, 'failed'); assert.equal(observedReports.mock.callCount(), 0);
  const status = observer.dispatchStatus(), expected = new DispatchQuarantinedError({ blocked: true, currentRevision: incident.generation, openCount: 1 }).message;
  assert.equal(status.localFault, expected); assert.equal(observer.health().issues[0], expected);
  assert.throws(() => observer.start(task()), /Workflow dispatch is quarantined/);
  const page = writer.dispatchQuarantinePage(); assert.equal(page.openCount, 1); assert.equal(page.incidents[0].instanceId, 'reporting-host');
});

test('an active foreign incident aborts its call and remains failed after external cancellation', { timeout: 10000 }, async t => {
  const entered = deferred(), controller = new AbortController(); let blocked = false, reports = 0, stateFailures = 0, aborted = false;
  const store = memory({ assertDispatchUnblocked() { if (blocked) throw refusal(); }, reportProcessCleanupIncident() { reports++; assert.fail('Foreign refusal must not report itself.'); } });
  t.mock.timers.enable({ apis: ['setInterval'] });
  const engine = new WorkflowEngine(config(), { doctor: async () => [], run: request => new Promise(done => {
    request.signal.addEventListener('abort', () => { aborted = true; done(answer(request.agent, { status: 'cancelled' })); }, { once: true }); entered.resolve();
  }) }, store, { instanceId: 'host', onStateFailure() { stateFailures++; } });
  const running = engine.run(task(), { signal: controller.signal });
  try {
    await entered.promise; blocked = true; t.mock.timers.tick(250); controller.abort(); const run = await running;
    assert.equal(aborted, true); assert.equal(run.status, 'failed'); assert.equal(run.steps[0].result.status, 'failed'); assert.equal(reports, 0); assert.equal(stateFailures, 0);
    assert.ok(store.log.some(event => event.type === 'step.dispatch_blocked' && event.data.active && event.data.code === 'COUNCIL_DISPATCH_QUARANTINED'));
  } finally { controller.abort(); t.mock.timers.reset(); await running; }
});

test('the independent final probe sees an incident committed after the cancellation-drain check', async () => {
  const controller = new AbortController(); let committed = false, cancelledProbe = false, reports = 0;
  const store = memory({ assertDispatchUnblocked() {
    if (committed) throw refusal();
    if (controller.signal.aborted) { cancelledProbe = true; committed = true; } // A later committed view follows this read.
  }, reportProcessCleanupIncident() { reports++; assert.fail('An observed incident must not create another.'); } });
  const run = await new WorkflowEngine(config(), { doctor: async () => [], run: async request => { controller.abort(); return answer(request.agent, { status: 'cancelled' }); } }, store,
    { instanceId: 'host' }).run(task(), { signal: controller.signal });
  assert.equal(cancelledProbe, true); assert.equal(run.status, 'failed'); assert.equal(run.steps[0].result.status, 'cancelled'); assert.equal(reports, 0);
});

for (const stage of ['git_create', 'git_export']) {
  test(`an actual ${stage} failure reports its fixed stage without exporting private diagnostics`, async t => {
    const root = fixture(t), path = join(root, 'retained'), store = shared();
    t.mock.method(GitWorkspaces.prototype, 'create', async (_id, agent, _slot, _signal, prepared) => {
      mkdirSync(path); writeFileSync(join(path, 'evidence.txt'), 'Keep local work.');
      const handle = { agent, path, cwd: path, patchPath: join(root, 'retained.patch'), baseCommit: 'a'.repeat(40), baseHead: 'a'.repeat(40), changed: false };
      prepared({ ...handle, exportStatus: 'pending' }); if (stage === 'git_create') throw new ProcessCleanupError(); return handle;
    });
    t.mock.method(GitWorkspaces.prototype, 'export', async () => { throw new ProcessCleanupError(); });
    const run = await new WorkflowEngine(config({ workspaceRoot: root, stateDir: join(root, '.council') }), { doctor: async () => [], run: async request => answer(request.agent) }, store,
      { instanceId: 'host' }).run(task({ permission: 'write' }));
    assert.equal(run.status, 'failed'); assert.equal(store.reports.length, 1); assert.equal(store.reports[0].stage, stage); assert.equal(store.reports[0].runId, run.id);
    assert.equal(run.workspaces[0].exportStatus, 'pending'); assert.ok(run.workspaces[0].exportError.includes(QUARANTINE_INSPECTION_REQUIRED));
  });
}

test('a foreign post-export refusal cannot be reported as a new Git cleanup incident', async t => {
  const root = fixture(t), path = join(root, 'retained'); let blocked = false, reports = 0;
  const store = memory({ assertDispatchUnblocked() { if (blocked) throw refusal(); }, reportProcessCleanupIncident() { reports++; assert.fail('The Git helper succeeded.'); } });
  t.mock.method(GitWorkspaces.prototype, 'create', async (_id, agent, _slot, _signal, prepared) => {
    mkdirSync(path); const handle = { agent, path, cwd: path, patchPath: join(root, 'ready.patch'), baseCommit: 'a'.repeat(40), baseHead: 'a'.repeat(40), changed: false };
    prepared({ ...handle, exportStatus: 'pending' }); return handle;
  });
  t.mock.method(GitWorkspaces.prototype, 'export', async handle => { blocked = true; return { ...handle, changed: true, exportStatus: 'ready' }; });
  const run = await new WorkflowEngine(config({ workspaceRoot: root, stateDir: join(root, '.council') }), { doctor: async () => [], run: async request => answer(request.agent) }, store,
    { instanceId: 'host' }).run(task({ permission: 'write' }));
  assert.equal(run.status, 'failed'); assert.equal(reports, 0); assert.equal(run.workspaces[0].exportStatus, 'pending');
  assert.equal(store.log.some(event => event.type === 'workspace.exported'), false);
});

test('ordinary failures and standalone memory cleanup keep their explicit local-only semantics', async () => {
  let calls = 0;
  const engine = new WorkflowEngine(config(), { doctor: async () => [], run: async request => { calls++; return calls === 1 ? answer(request.agent, { status: 'failed', error: 'Ordinary offline failure.' }) : cleanup(request.agent); } }, memory());
  assert.equal((await engine.run(task())).status, 'failed'); const run = await engine.run(task());
  assert.equal(calls, 2); assert.equal(run.status, 'failed'); assert.equal(run.error, PROCESS_CLEANUP_ERROR); assert.equal(run.error.includes(QUARANTINE_PERSISTENCE_WARNING), false);
});

test('private cleanup and failed incident persistence expose only fixed public diagnostics', async () => {
  const store = shared({ reportProcessCleanupIncident() { throw new Error('PRIVATE_REPORT_DIAGNOSTIC'); } });
  const run = await new WorkflowEngine(config(), { doctor: async () => [], run: async request => answer(request.agent, {
    status: 'failed', error: PROCESS_CLEANUP_ERROR + ' PRIVATE_PROVIDER_DIAGNOSTIC',
    output: JSON.stringify({ council_private_chat_request: { peer: 'claude', message: 'PRIVATE_QUESTION', context: 'PRIVATE_CONTEXT' } }),
  }) }, store, { instanceId: 'host' }).run(task({ privateChats: { codex: { peers: ['claude'] } } }));
  assert.equal(run.status, 'failed'); assert.ok(run.error.includes(QUARANTINE_PERSISTENCE_WARNING));
  assert.ok(run.steps[0].result.error.includes(QUARANTINE_PERSISTENCE_WARNING));
  const publicEvidence = JSON.stringify({ run, events: store.log });
  for (const marker of ['PRIVATE_REPORT_DIAGNOSTIC', 'PRIVATE_PROVIDER_DIAGNOSTIC', 'PRIVATE_QUESTION', 'PRIVATE_CONTEXT']) assert.equal(publicEvidence.includes(marker), false, marker);
});

test('inspection guidance does not erase an already failed export diagnosis', async t => {
  const root = fixture(t), store = shared(); let exports = 0;
  t.mock.method(GitWorkspaces.prototype, 'create', async (_id, agent, _slot, _signal, prepared) => {
    const path = join(root, agent); mkdirSync(path);
    const handle = { agent, path, cwd: path, patchPath: join(root, agent + '.patch'), baseCommit: 'a'.repeat(40), baseHead: 'a'.repeat(40), changed: false };
    prepared({ ...handle, exportStatus: 'pending' }); return handle;
  });
  t.mock.method(GitWorkspaces.prototype, 'export', async () => { if (++exports === 1) throw new Error('Original patch export failed.'); throw new ProcessCleanupError(); });
  const run = await new WorkflowEngine(config({ workspaceRoot: root, stateDir: join(root, '.council') }), { doctor: async () => [], run: async request => answer(request.agent) }, store,
    { instanceId: 'host' }).run(task({ mode: 'parallel', agents: ['codex', 'claude'], permission: 'write' }));
  assert.equal(run.status, 'failed'); assert.equal(exports, 2); assert.equal(store.reports.length, 1);
  assert.equal(run.workspaces[0].exportStatus, 'failed'); assert.match(run.workspaces[0].exportError, /Original patch export failed/);
  assert.ok(run.workspaces.every(artifact => artifact.exportError.includes(QUARANTINE_INSPECTION_REQUIRED))); assert.equal(run.workspaces[1].exportStatus, 'pending');
});
