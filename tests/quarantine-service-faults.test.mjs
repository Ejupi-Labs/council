import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { GitWorkspaces } from '../dist/workspaces.js';
import { ProcessCleanupError, PROCESS_CLEANUP_ERROR } from '../dist/process.js';
import { QUARANTINE_INSPECTION_REQUIRED, QUARANTINE_PERSISTENCE_WARNING } from '../dist/dispatch-quarantine.js';

const task = { mode: 'delegate', agents: ['codex'], permission: 'read', task: 'Offline service cleanup uncertainty qualification.' };
const REPORT_SECRET = 'PRIVATE_REPORT_DATABASE_DIAGNOSTIC_5ac7';
const FINAL_SECRET = 'PRIVATE_FINAL_DATABASE_DIAGNOSTIC_6b08';
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done; }); return { promise, resolve }; };
function fixture(t, cleanupFailure = true) {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-quarantine-fault-')));
  let calls = 0;
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxConcurrency: 2 }), {
    doctor: async () => [], run: async request => {
      calls++;
      return { agent: request.agent, status: cleanupFailure ? 'failed' : 'completed', durationMs: 1,
        output: 'Complete retained offline call evidence.', ...(cleanupFailure ? { error: PROCESS_CLEANUP_ERROR } : {}) };
    },
  });
  const cleanups = [() => service.close()];
  t.after(async () => {
    for (const cleanup of cleanups.toReversed()) await cleanup();
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-quarantine-fault-'));
    rmSync(root, { recursive: true, force: false, maxRetries: 10, retryDelay: 100 });
  });
  return { root, service, cleanups, calls: () => calls };
}
function safeEvidence(...values) {
  const text = JSON.stringify(values);
  assert.equal(text.includes(REPORT_SECRET), false); assert.equal(text.includes(FINAL_SECRET), false);
}
function failedReportHealth(service, emergency) {
  const health = service.health();
  assert.equal(health.status, 'degraded'); assert.equal(health.quarantine.blocked, false);
  assert.ok(health.issues.includes(QUARANTINE_PERSISTENCE_WARNING));
  assert.equal(health.emergencyResults, emergency); assert.equal(health.activeLocalRuns, 0);
  assert.equal(health.state.counts.slots, 0); assert.equal(health.state.counts.slotWaiters, 0);
  safeEvidence(health); return health;
}

test('incident persistence warnings survive a WorkflowPersistenceError when the service can retain the failed terminal result', async t => {
  const { service, calls } = fixture(t);
  t.mock.method(service.store, 'reportProcessCleanupIncident', () => { throw new Error(REPORT_SECRET); });
  const finalize = service.store.finalizeRun.bind(service.store); let attempts = 0;
  t.mock.method(service.store, 'finalizeRun', run => {
    attempts++; if (attempts === 1) throw new Error(FINAL_SECRET);
    return finalize(run);
  });
  const admitted = service.start(task), failed = await service.completion(admitted.id);
  assert.equal(attempts, 2, 'The actual engine finalization failure crosses the service recovery path.');
  assert.equal(failed.status, 'failed'); assert.ok(failed.error.includes(QUARANTINE_PERSISTENCE_WARNING));
  assert.match(failed.error, /persist the final workflow outcome/i); assert.equal(calls(), 1);
  assert.equal(failed.steps[0].result.output, 'Complete retained offline call evidence.');
  assert.equal(failed.steps[0].result.status, 'failed');
  assert.deepEqual(service.getRun(admitted.id), failed);
  const health = failedReportHealth(service, 0); assert.equal(health.state.counts.activeRuns, 0);
  safeEvidence(failed, health, service.store.events(admitted.id));
  assert.throws(() => service.start(task), /cleanup could not be verified/);
  assert.equal(calls(), 1);
});

test('emergency terminal persistence retains the fixed missing-incident warning and complete failed evidence without raw diagnostics', async t => {
  const { service, cleanups, calls } = fixture(t);
  t.mock.method(service.store, 'reportProcessCleanupIncident', () => { throw new Error(REPORT_SECRET); });
  t.mock.method(service.store, 'finalizeRun', () => { throw new Error(FINAL_SECRET); });
  const admitted = service.start(task), failed = await service.completion(admitted.id);
  assert.equal(failed.status, 'failed'); assert.match(failed.error, /Run persistence failed/);
  assert.ok(failed.error.includes(QUARANTINE_PERSISTENCE_WARNING)); assert.equal(calls(), 1);
  assert.equal(failed.steps[0].result.output, 'Complete retained offline call evidence.');
  assert.equal(failed.steps[0].result.status, 'failed');
  const health = failedReportHealth(service, 1);
  assert.equal(health.state.counts.activeRuns, 1, 'Emergency memory does not imply the incomplete durable run was finalized.');
  t.mock.method(service.store, 'getRun', () => { throw new Error('The durable transcript is unavailable.'); });
  assert.deepEqual(await service.completion(admitted.id), failed);
  const summary = await service.waitSummary(admitted.id, 0);
  assert.equal(summary.status, 'failed'); assert.ok(summary.error.includes(QUARANTINE_PERSISTENCE_WARNING));
  safeEvidence(failed, health, summary, service.store.events(admitted.id));
  assert.throws(() => service.start(task), /cleanup could not be verified/);
  await assert.rejects(service.close(), /unfinished|ownership/i);
  cleanups.pop(); // Expected shutdown refusal was observed; close still closed the database.
  assert.throws(() => service.store.health(), /closed|not open/i);
  assert.equal(calls(), 1);
});

test('two independently admitted maintenance helpers each report their later cleanup uncertainty despite the first local fence', { timeout: 15000 }, async t => {
  const { root, service, calls } = fixture(t, false);
  t.mock.method(GitWorkspaces.prototype, 'create', async (runId, agent, _slot, _signal, prepared) => {
    const path = join(root, runId); mkdirSync(path);
    const artifact = { agent, path, cwd: path, patchPath: join(root, runId + '.patch'), baseCommit: 'a'.repeat(40), baseHead: 'a'.repeat(40), changed: false, exportStatus: 'ready' };
    prepared({ ...artifact, exportStatus: 'pending' }); return artifact;
  });
  t.mock.method(GitWorkspaces.prototype, 'export', async value => ({ ...value, exportStatus: 'ready' }));
  const runs = [await service.run({ ...task, permission: 'write' }), await service.run({ ...task, permission: 'write' })];
  assert.ok(runs.every(run => run.status === 'completed'));
  const gates = [deferred(), deferred()]; let entered = 0;
  t.mock.method(GitWorkspaces.prototype, 'export', async () => {
    const index = entered++; await gates[index].promise; throw new ProcessCleanupError();
  });
  const first = service.exportArtifacts(runs[0].id).then(() => assert.fail('Cleanup uncertainty cannot return a ready patch.'), error => error);
  const second = service.exportArtifacts(runs[1].id).then(() => assert.fail('Cleanup uncertainty cannot return a ready patch.'), error => error);
  try {
    const deadline = performance.now() + 5000;
    while (entered < 2 && performance.now() < deadline) await delay(10, undefined, { signal: t.signal });
    assert.equal(entered, 2, 'Both actual helper awaits were admitted before the first cleanup uncertainty.');
    gates[0].resolve(); assert.ok((await first) instanceof ProcessCleanupError);
    assert.equal(service.dispatchQuarantinePage().openCount, 1);
    assert.equal(service.health().maintenanceOperations, 1, 'The other admitted helper remains draining after host fencing.');
    gates[1].resolve(); assert.ok((await second) instanceof ProcessCleanupError);
    const page = service.dispatchQuarantinePage();
    assert.equal(page.openCount, 2); assert.equal(new Set(page.incidents.map(incident => incident.reportKey)).size, 2);
    assert.deepEqual(new Set(page.incidents.map(incident => incident.runId)), new Set(runs.map(run => run.id)));
    assert.ok(page.incidents.every(incident => incident.stage === 'maintenance_export'));
    assert.ok(BigInt(page.incidents[1].generation) > BigInt(page.incidents[0].generation));
    for (const run of runs) {
      const retained = service.getRun(run.id); assert.equal(retained.status, 'completed', 'Historical terminal results remain unchanged.');
      assert.equal(retained.workspaces[0].exportStatus, 'pending'); assert.ok(retained.workspaces[0].exportError.includes(QUARANTINE_INSPECTION_REQUIRED));
    }
    const health = service.health(); assert.equal(health.maintenanceOperations, 0); assert.equal(health.state.counts.leases, 0);
    assert.equal(health.state.counts.slots, 0); assert.equal(health.state.counts.slotWaiters, 0); assert.equal(calls(), 2);
    assert.throws(() => service.start(task), /cleanup could not be verified/);
  } finally { for (const gate of gates) gate.resolve(); await Promise.allSettled([first, second]); }
});
