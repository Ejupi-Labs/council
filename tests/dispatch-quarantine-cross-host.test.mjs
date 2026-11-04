import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { copyFileSync, mkdirSync, mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';
import { CouncilStore } from '../dist/store.js';
import { createStateSnapshot, verifyStateSnapshot } from '../dist/state-snapshot.js';

const hostScript = fileURLToPath(new URL('./fixtures/quarantine-host.mjs', import.meta.url));
const PRIVATE_SENTINEL = 'PRIVATE_QUARANTINE_TEST_PAIR_BODY_91d890';
function bounded(work, milliseconds, label, signal) {
  return new Promise((resolve, reject) => {
    let settled = false;
    const finish = (error, value) => { if (settled) return; settled = true; clearTimeout(timer); signal?.removeEventListener('abort', abort); error ? reject(error) : resolve(value); };
    const abort = () => finish(signal.reason ?? new Error('Test cancelled.'));
    const timer = setTimeout(() => finish(new Error('Timed out waiting for ' + label)), milliseconds);
    signal?.addEventListener('abort', abort, { once: true });
    if (signal?.aborted) abort();
    Promise.resolve(work).then(value => finish(undefined, value), error => finish(error));
  });
}
function fixture(t) {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-quarantine-'))), hosts = [];
  let passed = false;
  t.after(async () => {
    let failure;
    for (const host of hosts) { try { await host.stop(); } catch (error) { failure ??= error; } }
    if (!passed || failure) { t.diagnostic('Preserved owned quarantine fixture: ' + root); if (failure) throw failure; return; }
    assert.equal(dirname(realpathSync.native(root)), parent); assert.ok(basename(root).startsWith('council-quarantine-'));
    rmSync(root, { recursive: true, force: false, maxRetries: 10, retryDelay: 100 });
  });
  async function host(concurrency = 1, workspace = root) {
    const child = spawn(process.execPath, [hostScript, workspace, String(concurrency)], {
      cwd: workspace, windowsHide: true, shell: false, stdio: ['ignore', 'pipe', 'pipe', 'ipc'], env: { ...process.env, COUNCIL_DEPTH: '0' },
    });
    const pending = new Map(), events = [], waiters = new Set();
    let sequence = 0, exited = false, output = '', terminalError;
    const exit = new Promise(resolve => child.once('exit', (code, signal) => { exited = true; terminalError ??= new Error('Fake host exited: ' + code + '/' + signal + (output ? '\n' + output : '')); resolve({ code, signal });
      for (const item of pending.values()) item.reject(terminalError); pending.clear();
      for (const item of waiters) item.reject(terminalError); waiters.clear(); }));
    child.stdout.on('data', chunk => { output = (output + chunk).slice(-8192); });
    child.stderr.on('data', chunk => { output = (output + chunk).slice(-8192); });
    child.on('error', error => { terminalError = error; for (const item of pending.values()) item.reject(error); for (const item of waiters) item.reject(error); });
    child.on('message', message => {
      if (typeof message?.id === 'number') {
        const item = pending.get(message.id); if (!item) return; pending.delete(message.id);
        if (message.error) item.reject(Object.assign(new Error(message.error.message), message.error)); else item.resolve(message.result);
        return;
      }
      events.push(message);
      if (message?.event === 'fatal') terminalError = Object.assign(new Error(message.error.message), message.error);
      for (const item of [...waiters]) {
        if (terminalError) { waiters.delete(item); item.reject(terminalError); }
        else if (item.match(message)) { waiters.delete(item); item.resolve(message); }
      }
    });
    function command(command, input = {}, signal = t.signal) {
      if (exited || terminalError) return Promise.reject(terminalError ?? new Error('Fake host is closed.'));
      const id = ++sequence, work = new Promise((resolve, reject) => { pending.set(id, { resolve, reject }); child.send({ id, command, ...input }, error => { if (error) { pending.delete(id); reject(error); } }); });
      return bounded(work, 10000, 'fake host ' + command, signal).finally(() => pending.delete(id));
    }
    function event(name, call) {
      const match = value => value?.event === name && (call === undefined || value.call === call), seen = events.find(match);
      if (seen) return Promise.resolve(seen);
      if (terminalError || exited) return Promise.reject(terminalError ?? new Error('Fake host is closed.'));
      let waiter;
      const work = new Promise((resolve, reject) => { waiter = { match, resolve, reject }; waiters.add(waiter); });
      return bounded(work, 10000, name, t.signal).finally(() => waiters.delete(waiter));
    }
    const value = { command, event, events,
      async stop() {
        if (exited) return;
        try { await command('close', {}, null); await bounded(exit, 3000, 'fake host close'); }
        catch (error) { if (!exited) child.kill('SIGKILL'); await bounded(exit, 3000, 'owned fake host termination'); throw error; }
      },
      async crash() { if (!exited) child.kill('SIGKILL'); await bounded(exit, 3000, 'owned fake host exit'); },
    };
    hosts.push(value); await event('ready'); return value;
  }
  return { root, host, success() { passed = true; } };
}
async function until(t, predicate, label) {
  const deadline = performance.now() + 5000;
  do { if (predicate()) return; await delay(10, undefined, { signal: t.signal }); } while (performance.now() < deadline);
  assert.fail('Timed out waiting for ' + label);
}
const reportInput = incident => ({ reportKey: incident.reportKey, instanceId: incident.instanceId, stage: incident.stage,
  ...(incident.runId === undefined ? {} : { runId: incident.runId }) });
function drained(store) { const { slots, slotWaiters, activeRuns } = store.health().counts; assert.deepEqual({ slots, slotWaiters, activeRuns }, { slots: 0, slotWaiters: 0, activeRuns: 0 }); }
function quarantined(error) { assert.equal(error.code, 'COUNCIL_DISPATCH_QUARANTINED'); assert.equal(error.receipt?.blocked, true); return true; }

test('a committed foreign cleanup incident refuses a durable FIFO waiter without entering its runner', { timeout: 25000 }, async t => {
  const setup = fixture(t), first = await setup.host(1), second = await setup.host(1), store = new CouncilStore(join(setup.root, '.council'));
  try {
    const owned = await first.command('start', { behavior: 'cleanup' }); await first.event('runner-entered', 1);
    const queued = await second.command('start');
    await until(t, () => store.health().counts.slotWaiters === 1, 'a real durable FIFO waiter');
    await first.command('release', { call: 1 });
    const [failed, refused] = await Promise.all([first.command('completion', { runId: owned.id }), second.command('completion', { runId: queued.id })]);
    assert.equal(failed.status, 'failed'); assert.equal(refused.status, 'failed');
    assert.ok(refused.steps.every(step => step.result.status !== 'completed'), 'Refused admission never becomes completed evidence.');
    assert.equal((await second.command('health')).fixtureCalls, 0); assert.match(refused.error, /cleanup|quarantin/i);
    assert.doesNotMatch(refused.error, /state persistence failed/i); drained(store);
    const page = store.dispatchQuarantinePage(); assert.equal(page.blocked, true); assert.equal(page.openCount, 1);
    assert.equal(page.incidents[0].runId, owned.id); assert.equal(page.incidents[0].stage, 'agent_call');
    assert.equal(page.incidents[0].reportingPid, first.events.find(event => event.event === 'ready').pid);
    assert.equal(page.currentRevision, page.incidents[0].generation, 'Foreign observation does not report another owned incident.');
    assert.throws(() => store.acknowledgeDispatchQuarantine({ expectedRevision: page.currentRevision, reviewed: true }), { code: 'COUNCIL_QUARANTINE_OFFLINE_REQUIRED' });
    setup.success();
  } finally { store.close(); }
});

for (const cancellationFirst of [false, true]) test(`foreign cleanup remains failed when an active fake ${cancellationFirst ? 'was already cancelled and draining' : 'ignores abort and returns completed'}`, { timeout: 25000 }, async t => {
  const setup = fixture(t), first = await setup.host(2), second = await setup.host(2), store = new CouncilStore(join(setup.root, '.council'));
  try {
    const owned = await first.command('start', { behavior: 'cleanup' }), active = await second.command('start');
    await Promise.all([first.event('runner-entered', 1), second.event('runner-entered', 1)]);
    if (cancellationFirst) { await second.command('cancel', { runId: active.id }); await second.event('runner-aborted', 1); }
    await first.command('release', { call: 1 }); await first.command('completion', { runId: owned.id });
    await second.event('runner-aborted', 1);
    assert.equal((await second.command('health')).fixtureWaiting, 1, 'An abort request is not a claim that the fake call has drained.');
    assert.equal(store.health().counts.slots, 1, 'The still-draining owned call retains its real shared slot.');
    assert.equal(store.health().counts.activeRuns, 1);
    await second.command('release', { call: 1 }); const result = await second.command('completion', { runId: active.id });
    assert.equal(result.status, 'failed'); assert.match(result.error, /cleanup|quarantin/i); drained(store);
    assert.equal(store.dispatchQuarantinePage().openCount, 1); setup.success();
  } finally { store.close(); }
});

test('quarantine survives owner exit, recovery, restart and all-store restore without replaying failed work', { timeout: 30000 }, async t => {
  const setup = fixture(t), first = await setup.host(), state = join(setup.root, '.council');
  const owned = await first.command('start', { behavior: 'cleanup' }); await first.event('runner-entered', 1); await first.command('release', { call: 1 });
  const original = await first.command('completion', { runId: owned.id }), before = await first.command('quarantine');
  const reportingPid = first.events.find(event => event.event === 'ready').pid;
  assert.equal(before.incidents[0].reportingPid, reportingPid, 'The original reporter PID is captured before exit.');
  await first.crash();
  const store = new CouncilStore(state);
  try {
    store.recoverOrphans(); assert.deepEqual(store.dispatchQuarantinePage(), before); drained(store);
    const restarted = await setup.host();
    assert.equal((await restarted.command('quarantine')).incidents[0].reportingPid, reportingPid, 'Restart inspection never substitutes its own PID.');
    await assert.rejects(restarted.command('start'), quarantined); assert.equal((await restarted.command('health')).fixtureCalls, 0);
    assert.equal(store.health().counts.runs, 1, 'An early refused start creates no replacement run.');
    await restarted.stop();
    const snapshot = await createStateSnapshot(state, join(setup.root, 'snapshot'));
    assert.equal((await verifyStateSnapshot(snapshot.destination)).valid, true);
    const restored = join(setup.root, 'restored'); mkdirSync(join(restored, '.council'), { recursive: true });
    for (const entry of snapshot.manifest.databases) copyFileSync(join(snapshot.destination, entry.name), join(restored, '.council', entry.name));
    const restoredHost = await setup.host(1, restored);
    const restoredPage = await restoredHost.command('quarantine');
    assert.deepEqual(restoredPage, before); assert.equal(restoredPage.incidents[0].reportingPid, reportingPid, 'Snapshot restore keeps the historical reporter PID.');
    await assert.rejects(restoredHost.command('start'), quarantined);
    assert.equal((await restoredHost.command('health')).fixtureCalls, 0); await restoredHost.stop();
    const acknowledged = store.acknowledgeDispatchQuarantine({ expectedRevision: before.currentRevision, reviewed: true });
    assert.equal(acknowledged.blocked, false); assert.equal(acknowledged.acknowledgment.incidentCount, 1);
    const fresh = await setup.host(), explicit = await fresh.command('start', { task: 'Explicitly new work after operator review.' });
    await fresh.event('runner-entered', 1); await fresh.command('release', { call: 1 }); assert.equal((await fresh.command('completion', { runId: explicit.id })).status, 'completed');
    assert.deepEqual(store.getRun(owned.id), original, 'Acknowledgement never rewrites or replays the failed run.');
    const restoredStore = new CouncilStore(join(restored, '.council'));
    try { assert.equal(restoredStore.dispatchQuarantinePage().blocked, true); } finally { restoredStore.close(); }
    setup.success();
  } finally { store.close(); }
});

test('acknowledged report retries remain idempotent and stale operator review cannot clear a later incident', { timeout: 30000 }, async t => {
  const setup = fixture(t), first = await setup.host(), state = join(setup.root, '.council');
  const one = await first.command('start', { behavior: 'cleanup' }); await first.event('runner-entered', 1); await first.command('release', { call: 1 }); await first.command('completion', { runId: one.id });
  const old = await first.command('quarantine'), reportingPid = first.events.find(event => event.event === 'ready').pid;
  assert.equal(old.incidents[0].reportingPid, reportingPid); await first.stop();
  const store = new CouncilStore(state);
  try {
    const ack = store.acknowledgeDispatchQuarantine({ expectedRevision: old.currentRevision, reviewed: true });
    assert.deepEqual(store.acknowledgeDispatchQuarantine({ expectedRevision: old.currentRevision, reviewed: true }).acknowledgment, ack.acknowledgment);
    const repeated = store.reportProcessCleanupIncident(reportInput(old.incidents[0]));
    assert.equal(repeated.id, old.incidents[0].id); assert.equal(repeated.generation, old.currentRevision); assert.ok(repeated.acknowledgedAt);
    assert.equal(repeated.reportingPid, reportingPid, 'An unbound acknowledged retry retains the departed reporter PID.');
    assert.equal(store.dispatchQuarantinePage().blocked, false);
    const second = await setup.host(), two = await second.command('start', { behavior: 'cleanup' });
    await second.event('runner-entered', 1); await second.command('release', { call: 1 }); await second.command('completion', { runId: two.id }); await second.stop();
    const current = store.dispatchQuarantinePage(); assert.ok(BigInt(current.currentRevision) > BigInt(old.currentRevision)); assert.equal(current.openCount, 1);
    assert.throws(() => store.acknowledgeDispatchQuarantine({ expectedRevision: old.currentRevision, reviewed: true }), { code: 'COUNCIL_QUARANTINE_REVISION_CONFLICT' });
    assert.deepEqual(store.dispatchQuarantinePage(), current); drained(store); setup.success();
  } finally { store.close(); }
});

test('private cleanup diagnostics never enter public quarantine receipts or workflow evidence', { timeout: 25000 }, async t => {
  const setup = fixture(t), first = await setup.host();
  const owned = await first.command('start', { behavior: 'private-cleanup' });
  await first.event('runner-entered', 1); await first.command('release', { call: 1 });
  await first.event('runner-entered', 2); await first.command('release', { call: 2 });
  const result = await first.command('completion', { runId: owned.id }); assert.equal(result.status, 'failed');
  const evidence = [result, await first.command('health'), await first.command('quarantine'), await first.command('events', { runId: owned.id })];
  assert.equal(JSON.stringify(evidence).includes(PRIVATE_SENTINEL), false);
  const second = await setup.host();
  assert.ok(JSON.stringify(await second.command('private-history', { runId: owned.id })).includes(PRIVATE_SENTINEL), 'A fresh inspection host retains complete private pair evidence.');
  await assert.rejects(second.command('start'), error => { quarantined(error); assert.equal(error.message.includes(PRIVATE_SENTINEL), false); assert.equal(JSON.stringify(error).includes(PRIVATE_SENTINEL), false); return true; });
  assert.equal((await second.command('health')).fixtureCalls, 0); setup.success();
});

test('ordinary fake failures and untrusted cleanup quotations leave cross-host dispatch healthy', { timeout: 25000 }, async t => {
  const setup = fixture(t), first = await setup.host(), store = new CouncilStore(join(setup.root, '.council'));
  try {
    for (const [index, behavior] of ['ordinary-failure', 'untrusted-failure', 'success'].entries()) {
      const run = await first.command('start', { behavior }); await first.event('runner-entered', index + 1); await first.command('release', { call: index + 1 });
      assert.equal((await first.command('completion', { runId: run.id })).status, behavior === 'success' ? 'completed' : 'failed');
      assert.equal(store.dispatchQuarantinePage().blocked, false);
    }
    const second = await setup.host(), explicit = await second.command('start');
    await second.event('runner-entered', 1); await second.command('release', { call: 1 }); assert.equal((await second.command('completion', { runId: explicit.id })).status, 'completed');
    drained(store); setup.success();
  } finally { store.close(); }
});

test('each already admitted owned cleanup uncertainty is durably reported after the first local fence', { timeout: 25000 }, async t => {
  const setup = fixture(t), host = await setup.host(2), store = new CouncilStore(join(setup.root, '.council'));
  try {
    const first = await host.command('start', { behavior: 'cleanup', task: 'First independently admitted offline call.' });
    const second = await host.command('start', { behavior: 'cleanup', task: 'Second independently admitted offline call.' });
    await Promise.all([host.event('runner-entered', 1), host.event('runner-entered', 2)]);
    await host.command('release', { call: 1 }); assert.equal((await host.command('completion', { runId: first.id })).status, 'failed');
    assert.equal(store.dispatchQuarantinePage().openCount, 1);
    await host.command('release', { call: 2 }); assert.equal((await host.command('completion', { runId: second.id })).status, 'failed');
    const page = store.dispatchQuarantinePage();
    assert.equal(page.openCount, 2); assert.equal(new Set(page.incidents.map(incident => incident.reportKey)).size, 2);
    assert.deepEqual(new Set(page.incidents.map(incident => incident.runId)), new Set([first.id, second.id]));
    assert.ok(BigInt(page.incidents[1].generation) > BigInt(page.incidents[0].generation));
    assert.equal((await host.command('health')).fixtureCalls, 2); drained(store); setup.success();
  } finally { store.close(); }
});
