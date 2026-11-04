import test from 'node:test';
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { spawn } from 'node:child_process';
import { mkdtempSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService, loadConfig } from '../dist/index.js';

const input = () => ({ mode: 'delegate', agents: ['codex'], permission: 'read', task: 'Inspect the original bounded task.' });
function fixture(t, runner, overrides = {}) {
  const temporaryBase = realpathSync(tmpdir());
  const root = mkdtempSync(join(temporaryBase, 'council-service-contract-'));
  const service = new CouncilService(loadConfig(root, { backend: 'mock', ...overrides }), runner);
  const additional = [];
  service.createPeer = (runner, maxConcurrency) => {
    const peer = new CouncilService({ ...service.config, maxConcurrency }, runner);
    additional.push(peer); return peer;
  };
  t.after(async () => {
    await Promise.all([service, ...additional].map(instance => instance.close()));
    const target = resolve(root);
    assert.equal(dirname(target), resolve(temporaryBase)); assert.ok(basename(target).startsWith('council-service-contract-'));
    rmSync(target, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return service;
}

test('mutating a returned queued record cannot change the task or participants that actually execute', async t => {
  const requests = [];
  const service = fixture(t, { doctor: async () => [], run: async request => {
    requests.push(request);
    return { agent: request.agent, status: 'completed', output: 'Original task inspected.', durationMs: 1 };
  } });
  const requested = input();
  const queued = service.start(requested);
  const admittedInput = structuredClone(queued.input);
  queued.input.task = 'Execute a different task.';
  queued.input.agents[0] = 'junie';
  queued.input.mode = 'council';
  queued.input.agentSettings.codex.skills.push('council-advisor');
  queued.input.agentSettings.codex.model = 'caller-mutated-model';
  queued.status = 'failed';
  requested.task = 'Caller also changed the original input.';
  const result = await service.completion(queued.id);
  assert.equal(result.status, 'completed'); assert.deepEqual(result.input, admittedInput);
  assert.equal(requests.length, 1); assert.equal(requests[0].agent, 'codex');
  assert.equal(requests[0].model, undefined); assert.equal(requests[0].prompt.includes('Selected collaboration skills'), false);
  assert.ok(requests[0].prompt.includes(input().task)); assert.ok(!requests[0].prompt.includes('different task'));
  assert.deepEqual(service.getRun(queued.id).input, admittedInput);
});

test('an idle newly configured host advertises its smaller shared call limit before its first task', async t => {
  let active = 0, peak = 0, idleCalls = 0;
  const runner = { doctor: async () => [], run: async request => {
    active++; peak = Math.max(peak, active);
    try {
      await new Promise(resolve => setTimeout(resolve, 25));
      return { agent: request.agent, status: 'completed', output: 'Bounded shared dispatch.', durationMs: 25 };
    } finally { active--; }
  } };
  const service = fixture(t, runner);
  service.createPeer({ doctor: async () => [], run: async () => { idleCalls++; throw new Error('The idle host must not be called.'); } }, 1);
  const results = await Promise.all(Array.from({ length: 3 }, () => service.run(input())));
  assert.ok(results.every(run => run.status === 'completed')); assert.equal(peak, 1); assert.equal(idleCalls, 0);
});

test('a state evidence failure degrades the host even when its terminal failed result can be saved', async t => {
  const service = fixture(t);
  const append = service.store.appendEvent.bind(service.store);
  let injected = false;
  service.store.appendEvent = (id, type, data) => {
    if (!injected && type === 'step.started') { injected = true; throw new Error('Injected evidence failure.'); }
    return append(id, type, data);
  };
  const run = await service.run(input());
  assert.equal(injected, true); assert.equal(run.status, 'failed');
  assert.equal(service.getRun(run.id).status, 'failed'); assert.equal(service.health().status, 'degraded');
  assert.throws(() => service.start(input()), /persistence failed|restart/i);
});

test('a state fault immediately fences other queued runs before the originating run finishes draining', async t => {
  const calls = [];
  let faulted = false;
  const service = fixture(t, { doctor: async () => [], run: async request => {
    calls.push({ agent: request.agent, afterFault: faulted });
    return { agent: request.agent, status: 'completed', output: 'Retained evidence.', durationMs: 1 };
  } }, { maxConcurrency: 1 });
  const append = service.store.appendEvent.bind(service.store);
  service.store.appendEvent = (id, type, data) => {
    if (!faulted && type === 'step.finished') { faulted = true; throw new Error('Injected evidence failure after provider return.'); }
    return append(id, type, data);
  };
  const first = service.start(input());
  const second = service.start({ ...input(), agents: ['claude'] });
  const results = await Promise.all([service.completion(first.id), service.completion(second.id)]);
  assert.equal(results[0].status, 'failed'); assert.equal(results[1].status, 'cancelled');
  assert.equal(calls.length, 1); assert.deepEqual(calls, [{ agent: 'codex', afterFault: false }]);
  assert.equal(service.health().status, 'degraded'); assert.equal(service.store.health().counts.slotWaiters, 0);
});

test('a foreign status wait uses elapsed time even when the system wall clock stops advancing', async t => {
  const service = fixture(t);
  const stamp = new Date().toISOString(), id = randomUUID();
  service.store.saveRun({ id, input: input(), status: 'running', createdAt: stamp, updatedAt: stamp, steps: [] });
  const original = Date.now;
  const frozen = original();
  Date.now = () => frozen;
  const safety = new AbortController();
  const timer = setTimeout(() => safety.abort(new Error('Wall clock stopped the bounded wait.')), 1000);
  try {
    const summary = await service.waitSummary(id, 20, safety.signal);
    assert.equal(summary.status, 'running'); assert.equal(safety.signal.aborted, false);
  } finally { clearTimeout(timer); Date.now = original; }
});

test('mutating an emergency result cannot forge a successful outcome in later status or resource reads', async t => {
  const service = fixture(t);
  const finalize = service.store.finalizeRun.bind(service.store);
  service.store.finalizeRun = () => { throw new Error('Injected unavailable final persistence.'); };
  const queued = service.start(input());
  try {
    const result = await service.completion(queued.id);
    assert.equal(result.status, 'failed');
    result.status = 'completed'; result.output = 'Forged external mutation.';
    const resource = service.getRun(queued.id);
    assert.equal(resource.status, 'failed'); assert.notEqual(resource.output, result.output);
    resource.status = 'completed'; resource.steps.length = 0;
    assert.equal((await service.waitSummary(queued.id, 0)).status, 'failed');
    assert.equal((await service.completion(queued.id)).status, 'failed');
  } finally {
    service.store.finalizeRun = finalize;
    finalize(service.getRun(queued.id));
  }
});

test('brief SQLite heartbeat contention is retried and cleared after ownership is confirmed', t => {
  const service = fixture(t);
  const heartbeat = service.store.heartbeat.bind(service.store);
  service.store.heartbeat = () => { throw Object.assign(new Error('database is locked'), { code: 'ERR_SQLITE_ERROR', errcode: 261 }); };
  service.heartbeatTick();
  assert.equal(service.health().status, 'ok'); assert.equal(service.health().heartbeatContentions, 1);
  service.store.heartbeat = heartbeat;
  service.heartbeatTick();
  assert.equal(service.health().heartbeatContentions, 0); assert.equal(service.health().status, 'ok');
});

test('sustained SQLite heartbeat contention still fails closed after the bounded grace interval', t => {
  let elapsed = 0;
  t.mock.method(performance, 'now', () => elapsed);
  const service = fixture(t);
  service.store.heartbeat = () => { throw Object.assign(new Error('database is locked'), { errcode: 6 }); };
  elapsed = 4999; service.heartbeatTick(); assert.equal(service.health().status, 'ok');
  elapsed = 5000; service.heartbeatTick(); assert.equal(service.health().status, 'degraded');
  assert.throws(() => service.start(input()), /heartbeat failed/);
});

test('ownership loss and non-contention storage errors never receive a heartbeat retry grace', t => {
  for (const fail of [() => false, () => { throw Object.assign(new Error('database is full'), { errcode: 13 }); }]) {
    const service = fixture(t);
    service.store.heartbeat = fail; service.heartbeatTick();
    assert.equal(service.health().status, 'degraded'); assert.throws(() => service.start(input()), /ownership|heartbeat/);
  }
});

test('closing an embedded host retries brief real writer contention and removes its restrictive instance row', { timeout: 15000 }, async t => {
  const service = fixture(t, undefined, { maxConcurrency: 1 });
  const peer = service.createPeer(undefined, 3);
  const child = spawn(process.execPath, ['--input-type=module', '-e', `
import {DatabaseSync} from 'node:sqlite';
const db=new DatabaseSync(process.argv[1]);db.exec('PRAGMA busy_timeout=2000; BEGIN IMMEDIATE');
process.send({locked:true});
process.on('message',()=>{db.exec('ROLLBACK');db.close();process.disconnect();});
process.on('disconnect',()=>process.exit());
`, join(service.config.stateDir, 'state.db')], { windowsHide: true, stdio: ['ignore', 'ignore', 'ignore', 'ipc'] });
  const closed = new Promise(resolve => child.once('close', resolve));
  let timer;
  try {
    await new Promise((resolveReady, reject) => { child.once('message', resolveReady); child.once('error', reject); child.once('exit', code => reject(new Error(`Writer exited before locking: ${code}`))); });
    timer = setTimeout(() => { if (child.connected) child.send({ release: true }); }, 400);
    await service.close();
    await closed;
    assert.equal(peer.store.health().counts.instances, 1);
    const result = await peer.run(input()); assert.equal(result.status, 'completed');
  } finally {
    clearTimeout(timer);
    if (child.connected) child.send({ release: true });
    if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL');
    await closed;
  }
});

test('async service initialization retries a held writer before accepting work and never calls a provider', { timeout: 15000 }, async t => {
  const existing = fixture(t);
  let calls = 0, opened, timer;
  const runner = { doctor: async () => [], run: async () => { calls++; throw new Error('Initialization must not call a provider.'); } };
  const child = spawn(process.execPath, ['--input-type=module', '-e', `
import {DatabaseSync} from 'node:sqlite';
const db=new DatabaseSync(process.argv[1]);db.exec('PRAGMA busy_timeout=2000; BEGIN IMMEDIATE');process.send({locked:true});
process.on('message',()=>{db.exec('ROLLBACK');db.close();process.disconnect();});process.on('disconnect',()=>process.exit());
`, join(existing.config.stateDir, 'state.db')], { windowsHide: true, stdio: ['ignore', 'ignore', 'ignore', 'ipc'] });
  const closed = new Promise(resolve => child.once('close', resolve));
  try {
    await new Promise((ready, reject) => { child.once('message', ready); child.once('error', reject); child.once('exit', code => reject(new Error(`Writer exited before readiness: ${code}`))); });
    timer = setTimeout(() => { if (child.connected) child.send({ release: true }); }, 400);
    opened = await CouncilService.open(existing.config, runner);
    await closed;
    assert.equal(calls, 0); assert.equal(opened.health().status, 'ok'); assert.equal(opened.store.health().counts.instances, 2);
  } finally {
    clearTimeout(timer);
    if (child.connected) child.send({ release: true });
    if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL');
    await closed; await opened?.close();
  }
});

test('continuation makes pending patch status explicit in peer context without repairing artifacts', async t => {
  let received;
  const service = fixture(t, { doctor: async () => [], run: async request => {
    received = request.prompt;
    return { agent: request.agent, status: 'completed', output: 'Follow-up inspected.', durationMs: 1 };
  } });
  const stamp = new Date().toISOString(), id = randomUUID();
  const artifact = { agent: 'codex', path: '/retained/incomplete', patchPath: '/retained/unavailable.patch', changed: false,
    exportStatus: 'pending', exportError: 'The original patch export was interrupted.' };
  service.store.saveRun({ id, input: input(), status: 'cancelled', createdAt: stamp, updatedAt: stamp, steps: [], workspaces: [artifact] });
  const queued = service.continue(id, input());
  const result = await service.completion(queued.id);
  assert.equal(result.status, 'completed'); assert.ok(received.includes('export: pending'));
  assert.ok(received.includes(artifact.exportError)); assert.deepEqual(service.getRun(id).workspaces, [artifact]);
  assert.ok(service.store.events(id).every(event => event.type !== 'workspace.refreshed'));
});
