import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { CouncilService, loadConfig } from '../dist/index.js';
import { GitWorkspaces } from '../dist/workspaces.js';
import { ProcessCleanupError, PROCESS_CLEANUP_ERROR } from '../dist/process.js';
import { DispatchQuarantinedError, QUARANTINE_PERSISTENCE_WARNING } from '../dist/dispatch-quarantine.js';

const task = { mode: 'delegate', task: 'Offline maintenance failure check.', agents: ['codex'], permission: 'read' };
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; };
async function fixture(t) {
  const root = mkdtempSync(join(realpathSync.native(tmpdir()), 'council-maintenance-fault-'));
  let calls = 0, failCleanup = false;
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxConcurrency: 1 }), {
    doctor: async () => [], run: async request => {
      calls++;
      return { agent: request.agent, status: failCleanup ? 'failed' : 'completed', output: 'Offline retained evidence.', durationMs: 1,
        ...(failCleanup ? { error: PROCESS_CLEANUP_ERROR } : {}) };
    },
  });
  t.after(async () => { await service.close(); rmSync(root, { recursive: true, force: true }); });
  const path = join(root, 'retained'); mkdirSync(path);
  const artifact = { agent: 'codex', path, cwd: path, patchPath: join(root, 'retained.patch'), baseCommit: 'a'.repeat(40), baseHead: 'a'.repeat(40), changed: false, exportStatus: 'ready' };
  t.mock.method(GitWorkspaces.prototype, 'create', async (_id, _agent, _slot, _signal, prepared) => { prepared({ ...artifact, exportStatus: 'pending' }); return artifact; });
  t.mock.method(GitWorkspaces.prototype, 'export', async value => ({ ...value, exportStatus: 'ready' }));
  const run = await service.run({ ...task, permission: 'write' });
  assert.equal(run.status, 'completed');
  return { service, run, artifact, calls: () => calls, fail: () => { failCleanup = true; } };
}

for (const operation of ['export', 'cleanup']) {
  test(`unverified ${operation} helper cleanup fences maintenance and direct engine dispatch`, { timeout: 10000 }, async t => {
    const { service, run, artifact, calls } = await fixture(t);
    const helper = t.mock.method(GitWorkspaces.prototype, operation, async () => { throw new ProcessCleanupError(); });
    const pending = operation === 'export' ? service.exportArtifacts(run.id) : service.cleanup(run.id, { reviewed: true });
    await assert.rejects(pending, ProcessCleanupError);
    assert.equal(helper.mock.callCount(), 1); assert.equal(existsSync(artifact.path), true);
    assert.equal(service.health().status, 'degraded');
    assert.match(service.health().issues[0], /cleanup could not be verified/);
    assert.equal(service.health().state.counts.leases, 0);
    assert.equal(service.health().maintenanceOperations, 0);
    await assert.rejects(service.exportArtifacts(run.id), /cleanup could not be verified/);
    await assert.rejects(service.cleanup(run.id, { reviewed: true }), /cleanup could not be verified/);
    assert.throws(() => service.start(task), /cleanup could not be verified/);
    assert.equal((await service.engine.run(task)).status, 'failed');
    assert.equal(calls(), 1); assert.equal(helper.mock.callCount(), 1);
    const incidents = service.dispatchQuarantinePage().incidents;
    assert.equal(incidents.length, 1);
    assert.equal(incidents[0].stage, operation === 'export' ? 'maintenance_export' : 'maintenance_cleanup');
    assert.equal(incidents[0].runId, run.id);
    if (operation === 'export') assert.equal(service.getRun(run.id).workspaces[0].exportStatus, 'pending');
  });
}

test('an export returning after a concurrent cleanup fault remains pending and cannot publish ready metadata', { timeout: 10000 }, async t => {
  const { service, run, artifact, fail } = await fixture(t), entered = deferred(), released = deferred();
  let exportSignal;
  t.mock.method(GitWorkspaces.prototype, 'export', async (_artifact, signal) => { exportSignal = signal; entered.resolve(); await released.promise; return { ...artifact, changed: true, exportStatus: 'ready' }; });
  const operation = service.exportArtifacts(run.id), observed = assert.rejects(operation, DispatchQuarantinedError);
  try {
    await entered.promise; fail();
    assert.equal((await service.run(task)).status, 'failed');
    assert.equal(exportSignal.aborted, true);
    released.resolve(); await observed;
    const retained = service.getRun(run.id).workspaces[0];
    assert.equal(retained.exportStatus, 'pending'); assert.match(retained.exportError, /Inspect OS processes and retained worktrees/);
    assert.equal(service.store.events(run.id).some(event => event.type === 'workspace.refreshed' && event.data?.exportStatus === 'ready'), false);
    assert.equal(existsSync(artifact.path), true);
    assert.deepEqual(service.dispatchQuarantinePage().incidents.map(incident => incident.stage), ['agent_call']);
  } finally { released.resolve(); await observed; }
});

test('failed maintenance incident persistence retains a local fence and an explicit safe warning', { timeout: 10000 }, async t => {
  const { service, run } = await fixture(t);
  t.mock.method(service.store, 'reportProcessCleanupIncident', () => { throw new Error('PRIVATE_PERSISTENCE_DIAGNOSTIC'); });
  t.mock.method(GitWorkspaces.prototype, 'export', async () => { throw new ProcessCleanupError(); });
  await assert.rejects(service.exportArtifacts(run.id), error => {
    assert(error instanceof ProcessCleanupError);
    assert(error.message.includes(QUARANTINE_PERSISTENCE_WARNING));
    assert(!error.message.includes('PRIVATE_PERSISTENCE_DIAGNOSTIC'));
    return true;
  });
  const health = service.health();
  assert.equal(health.status, 'degraded'); assert.equal(health.quarantine.blocked, false);
  assert(health.issues.includes(QUARANTINE_PERSISTENCE_WARNING));
  assert.throws(() => service.start(task), /cleanup could not be verified/);
  const affected = await service.engine.run(task);
  assert.equal(affected.status, 'failed'); assert(affected.error.includes(QUARANTINE_PERSISTENCE_WARNING));
  assert.equal(service.getRun(run.id).workspaces[0].exportStatus, 'pending');
});

test('ordinary maintenance errors quoting cleanup text do not invent a host cleanup fault', { timeout: 10000 }, async t => {
  const { service, run, calls } = await fixture(t);
  t.mock.method(GitWorkspaces.prototype, 'export', async () => { throw new Error('Ordinary failed check quoting ' + PROCESS_CLEANUP_ERROR); });
  await assert.rejects(service.exportArtifacts(run.id), /Ordinary failed check/);
  assert.equal(service.health().status, 'ok');
  assert.equal((await service.run(task)).status, 'completed'); assert.equal(calls(), 2);
});
