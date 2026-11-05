import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile, execFileSync } from 'node:child_process';
import { promisify } from 'node:util';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { CouncilStore } from '../dist/store.js';
import { loadConfig } from '../dist/config.js';
import { GitWorkspaces } from '../dist/workspaces.js';

const task = (overrides = {}) => ({ mode: 'delegate', task: 'Inspect the bounded offline example.', agents: ['codex'], permission: 'read', ...overrides });
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));

function fixture(t, repository = false) {
  const root = mkdtempSync(join(tmpdir(), 'council-service-'));
  const services = [];
  t.after(async () => { await Promise.allSettled(services.map(service => service.close())); rmSync(root, { recursive: true, force: true }); });
  if (repository) {
    const git = (...args) => execFileSync('git', ['-C', root, ...args], { windowsHide: true, encoding: 'utf8' });
    git('init', '--quiet'); git('config', 'user.name', 'Offline Service Test'); git('config', 'user.email', 'test@example.invalid');
    writeFileSync(join(root, '.gitignore'), '.council/\n'); writeFileSync(join(root, 'base.txt'), 'original\n');
    git('add', '.'); git('commit', '--quiet', '-m', 'base');
  }
  return { root, create(runner, overrides = {}) { const service = new CouncilService(loadConfig(root, { backend: 'mock', ...overrides }), runner); services.push(service); return service; } };
}

test('two service instances enforce one workspace-wide agent call at a time', async t => {
  const { create } = fixture(t);
  let active = 0, peak = 0;
  const runner = { doctor: async () => [], run: async request => {
    active++; peak = Math.max(peak, active); await pause(40); active--;
    return { agent: request.agent, status: 'completed', output: 'offline result', durationMs: 40 };
  } };
  const first = create(runner, { maxConcurrency: 1 });
  const second = create(runner, { maxConcurrency: 1 });
  const results = await Promise.all([first.run(task()), second.run(task({ agents: ['junie'] }))]);
  assert.equal(peak, 1); assert.ok(results.every(run => run.status === 'completed'));
});

test('the queued creation timestamp survives transition to running and completion', async t => {
  const { create } = fixture(t);
  const service = create();
  const queued = service.start(task());
  const until = Date.now() + 20;
  while (Date.now() < until) { /* Deliberately delay the engine microtask to separate its timestamp. */ }
  const final = await service.completion(queued.id);
  assert.equal(final.createdAt, queued.createdAt);
  assert.ok(final.updatedAt >= final.createdAt);
});

test('close returns one shared promise and drains cancelled calls before closing state', async t => {
  const { root, create } = fixture(t);
  let started;
  const ready = new Promise(resolve => { started = resolve; });
  const service = create({ doctor: async () => [], run: request => new Promise(resolve => {
    request.signal.addEventListener('abort', () => { setTimeout(() => resolve({ agent: request.agent, status: 'cancelled', output: 'retained partial result', durationMs: 30 }), 30); }, { once: true });
    started();
  }) });
  const queued = service.start(task()); await ready;
  const firstClose = service.close(), secondClose = service.close();
  assert.strictEqual(firstClose, secondClose);
  let closed = false; firstClose.then(() => { closed = true; });
  await pause(5); assert.equal(closed, false);
  await firstClose;
  const reopened = new CouncilStore(join(root, '.council'));
  assert.equal(reopened.getRun(queued.id).status, 'cancelled'); reopened.close();
  assert.throws(() => service.start(task()), /shutting down/);
});

test('continue creates a new read-only task with partial evidence and the explicitly selected model', async t => {
  const { create } = fixture(t);
  const requests = [];
  const service = create({ doctor: async () => [], run: async request => {
    requests.push(request);
    return { agent: request.agent, status: request.agent === 'codex' ? 'failed' : 'completed', output: request.agent === 'codex' ? 'PARTIAL_EVIDENCE' : 'new independent result', durationMs: 1, ...(request.agent === 'codex' ? { error: 'offline simulated failure' } : {}) };
  } }, { agents: { codex: { enabled: true, profile: 'codex', model: 'original-model' }, junie: { enabled: true, profile: 'junie', model: 'user-selected-junie-model' } } });
  const previous = await service.run(task()); assert.equal(previous.status, 'failed');
  const queued = service.continue(previous.id, { mode: 'delegate', agents: ['junie'], task: 'Evaluate the unresolved evidence and suggest a next step.' });
  const next = await service.completion(queued.id);
  assert.notEqual(next.id, previous.id); assert.equal(next.status, 'completed');
  assert.equal(next.input.permission, 'read'); assert.deepEqual(next.input.agents, ['junie']);
  assert.equal(next.input.task, 'Evaluate the unresolved evidence and suggest a next step.');
  assert.match(requests[1].prompt, /PARTIAL_EVIDENCE/); assert.match(requests[1].prompt, /failed/);
  assert.match(requests[1].prompt, /untrusted task data/); assert.equal(requests[1].model, 'user-selected-junie-model');
  assert.equal(requests.length, 2);
  assert.ok(service.store.events(next.id).some(event => event.type === 'run.continued' && event.data.previousId === previous.id));
});

test('a failed explicitly selected continuation never substitutes another participant', async t => {
  const { create } = fixture(t);
  const calls = [];
  const service = create({ doctor: async () => [], run: async request => {
    calls.push(request.agent);
    return { agent: request.agent, status: request.agent === 'junie' ? 'failed' : 'completed', output: 'offline evidence', durationMs: 1, error: request.agent === 'junie' ? 'selected agent unavailable' : undefined };
  } });
  const previous = await service.run(task());
  const queued = service.continue(previous.id, { mode: 'delegate', agents: ['junie'], task: 'Check the next question.' });
  const next = await service.completion(queued.id);
  assert.equal(next.status, 'failed'); assert.deepEqual(calls, ['codex', 'junie']);
  assert.deepEqual(next.input.agents, ['junie']); assert.match(next.error, /unavailable/);
});

test('cleanup requires review, protects dirty work and makes repeated cleanup a no-op', async t => {
  const { root, create } = fixture(t, true);
  const service = create({ doctor: async () => [], run: async request => {
    writeFileSync(join(request.cwd, 'base.txt'), 'changed only in retained worktree\n');
    return { agent: request.agent, status: 'completed', output: 'offline write complete', durationMs: 1 };
  } });
  const run = await service.run(task({ permission: 'write' })); assert.equal(run.status, 'completed');
  const artifact = run.workspaces[0];
  await assert.rejects(service.cleanup(run.id, {}), /review/i);
  await assert.rejects(service.cleanup(run.id, { reviewed: true }), /modified|untracked|changes/i);
  assert.ok(existsSync(artifact.path));
  const removed = await service.cleanup(run.id, { reviewed: true, discardChanges: true });
  assert.deepEqual(removed.removed, [artifact.path]); assert.deepEqual(removed.alreadyRemoved, []);
  assert.equal(existsSync(artifact.path), false); assert.ok(existsSync(artifact.patchPath));
  const repeated = await service.cleanup(run.id, { reviewed: true, discardChanges: true });
  assert.deepEqual(repeated.removed, []); assert.deepEqual(repeated.alreadyRemoved, [artifact.path]);
  assert.equal(service.store.events(run.id).filter(event => event.type === 'workspace.cleaned').length, 1);
  assert.equal(readFileSync(join(root, 'base.txt'), 'utf8'), 'original\n');
});

test('close also drains a worktree maintenance operation before releasing SQLite', async t => {
  const { root, create } = fixture(t, true);
  const service = create();
  const run = await service.run(task({ permission: 'write' }));
  const original = GitWorkspaces.prototype.cleanup;
  let entered, release;
  const started = new Promise(resolve => { entered = resolve; });
  const ready = new Promise(resolve => { release = resolve; });
  GitWorkspaces.prototype.cleanup = async function (...args) { entered(); await ready; return original.apply(this, args); };
  try {
    const cleaning = service.cleanup(run.id, { reviewed: true });
    const cancelled = assert.rejects(cleaning, error => error.name === 'AbortError');
    await started;
    let closed = false;
    const closing = service.close().then(() => { closed = true; });
    await pause(10); assert.equal(closed, false);
    assert.equal(service.store.getRun(run.id).id, run.id, 'SQLite remains open while maintenance is pending');
    assert.ok(existsSync(run.workspaces[0].path));
    release(); await cancelled; await closing;
    const reopened = new CouncilStore(join(root, '.council'));
    try {
      assert.ok(existsSync(run.workspaces[0].path), 'cancelled cleanup retains the worktree');
      assert.equal(reopened.getRun(run.id).workspaces[0].path, run.workspaces[0].path);
      assert.equal(reopened.events(run.id).filter(event => event.type === 'workspace.cleaned').length, 0);
    } finally { reopened.close(); }
  } finally { release(); GitWorkspaces.prototype.cleanup = original; }
});

test('local completion wakes a thirty-second summary wait without leaving its timer active', { timeout: 15_000 }, async () => {
  const root = mkdtempSync(join(tmpdir(), 'council-local-wake-'));
  try {
    const source = `
import {CouncilService} from ${JSON.stringify(new URL('../dist/service.js', import.meta.url).href)};
import {loadConfig} from ${JSON.stringify(new URL('../dist/config.js', import.meta.url).href)};
const runner={doctor:async()=>[],run:async request=>{await new Promise(resolve=>setTimeout(resolve,30));return {agent:request.agent,status:'completed',output:'offline done',durationMs:30};}};
const service=new CouncilService(loadConfig(${JSON.stringify(root)},{backend:'mock'}),runner);
try{
 const run=service.start({mode:'delegate',agents:['codex'],task:'Offline summary wait.',permission:'read'});
 let summaryReads=0;const original=service.store.getRunSummary.bind(service.store);service.store.getRunSummary=id=>{summaryReads++;return original(id);};
 const result=await service.waitSummary(run.id,30000);
 console.log(JSON.stringify({status:result.status,summaryReads}));
}finally{await service.close();}
`;
    const response = await promisify(execFile)(process.execPath, ['--input-type=module', '-e', source], { windowsHide: true, timeout: 10_000, env: { ...process.env, COUNCIL_DEPTH: '0' } });
    const result = JSON.parse(response.stdout);
    assert.equal(result.status, 'completed'); assert.ok(result.summaryReads <= 3);
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('explicit artifact repair exports pending cancelled work without changing its outcome or calling agents', { timeout: 60_000 }, async t => {
  const { create } = fixture(t, true);
  let entered, calls = 0;
  const started = new Promise(resolve => { entered = resolve; });
  const service = create({ doctor: async () => [], run: request => new Promise(resolve => {
    calls++;
    writeFileSync(join(request.cwd, 'retained.txt'), 'partial work survives cancellation\n');
    request.signal.addEventListener('abort', () => resolve({ agent: request.agent, status: 'cancelled', output: 'partial evidence', durationMs: 1 }), { once: true });
    entered();
  }) });
  // Use the engine's existing test deadline override to avoid a five-second fixture delay.
  service.engine.options.cancellationArtifactTimeoutMs = 40;
  const original = GitWorkspaces.prototype.export;
  GitWorkspaces.prototype.export = async (_artifact, signal) => await new Promise((_resolve, reject) => {
    if (signal.aborted) reject(signal.reason);
    else signal.addEventListener('abort', () => reject(signal.reason), { once: true });
  });
  let previous;
  try {
    const queued = service.start(task({ permission: 'write' })); await started;
    service.requestCancellation(queued.id); previous = await service.completion(queued.id);
  } finally { GitWorkspaces.prototype.export = original; }
  assert.equal(previous.status, 'cancelled'); assert.equal(previous.workspaces[0].exportStatus, 'pending');
  const repaired = await service.exportArtifacts(previous.id);
  assert.equal(repaired.runId, previous.id); assert.equal(repaired.artifacts[0].exportStatus, 'ready');
  assert.equal(repaired.artifacts[0].changed, true); assert.equal(repaired.artifacts[0].exportError, undefined);
  assert.match(readFileSync(repaired.artifacts[0].patchPath, 'utf8'), /partial work survives cancellation/);
  const next = service.store.getRun(previous.id);
  for (const key of ['status', 'input', 'steps', 'output', 'error', 'createdAt']) assert.deepEqual(next[key], previous[key]);
  assert.equal(next.workspaces[0].exportStatus, 'ready'); assert.equal(calls, 1);
  assert.equal(service.store.events(previous.id).filter(event => event.type === 'workspace.refreshed').length, 1);
  assert.deepEqual(service.store.leases(), []);
});

test('artifact repair rejects active runs and incomplete preparation without creating a checkout', { timeout: 60_000 }, async t => {
  const { create } = fixture(t, true);
  let entered;
  const started = new Promise(resolve => { entered = resolve; });
  const service = create({ doctor: async () => [], run: request => new Promise(resolve => {
    request.signal.addEventListener('abort', () => resolve({ agent: request.agent, status: 'cancelled', output: '', durationMs: 1 }), { once: true });
    entered();
  }) });
  const active = service.start(task()); await started;
  await assert.rejects(service.exportArtifacts(active.id), /terminal state/);
  service.requestCancellation(active.id); await service.completion(active.id);
  const original = GitWorkspaces.prototype.git;
  GitWorkspaces.prototype.git = async function (cwd, args, ...rest) {
    if (args[0] === 'worktree' && args[1] === 'add') throw new Error('local incomplete checkout fixture');
    return original.call(this, cwd, args, ...rest);
  };
  let incomplete;
  try { incomplete = await service.run(task({ permission: 'write' })); }
  finally { GitWorkspaces.prototype.git = original; }
  assert.equal(incomplete.status, 'failed'); assert.equal(incomplete.workspaces[0].exportStatus, 'pending');
  assert.equal(existsSync(incomplete.workspaces[0].path), false);
  await assert.rejects(service.exportArtifacts(incomplete.id), /does not exist.*never creates a checkout/);
  assert.equal(existsSync(incomplete.workspaces[0].path), false);
  assert.equal(service.store.events(incomplete.id).filter(event => event.type === 'workspace.refreshed').length, 0);
  assert.deepEqual(service.store.leases(), []);
});

test('artifact repair retains earlier committed exports when a later worktree has redirected Git metadata', { timeout: 60_000 }, async t => {
  const { root, create } = fixture(t, true);
  let calls = 0;
  const service = create({ doctor: async () => [], run: async request => {
    calls++; return { agent: request.agent, status: 'completed', output: 'offline implementation', durationMs: 1 };
  } });
  const previous = await service.run(task({ mode: 'parallel', agents: ['codex', 'claude'], permission: 'write' }));
  assert.equal(previous.status, 'completed');
  const [first, second] = previous.workspaces;
  writeFileSync(join(first.path, 'base.txt'), 'reviewed follow-up in first worktree\n');
  rmSync(join(second.path, '.git'));
  writeFileSync(join(second.path, '.git'), `gitdir: ${join(root, '.git')}\n`);
  await assert.rejects(service.exportArtifacts(previous.id), /redirected/);
  assert.match(readFileSync(first.patchPath, 'utf8'), /reviewed follow-up in first worktree/);
  const current = service.store.getRun(previous.id);
  assert.equal(current.status, 'completed'); assert.equal(current.workspaces[0].changed, true);
  assert.deepEqual(current.workspaces[1], second); assert.equal(calls, 2);
  assert.equal(service.store.events(previous.id).filter(event => event.type === 'workspace.refreshed').length, 1);
  assert.deepEqual(service.store.leases(), []);
});

test('artifact repair excludes cleanup across services and shutdown drains it before closing SQLite', { timeout: 60_000 }, async t => {
  const { root, create } = fixture(t, true);
  const service = create(), other = create();
  const run = await service.run(task({ permission: 'write' }));
  const original = GitWorkspaces.prototype.export;
  let entered, release;
  const started = new Promise(resolve => { entered = resolve; });
  const ready = new Promise(resolve => { release = resolve; });
  GitWorkspaces.prototype.export = async function (...args) { entered(); await ready; return original.apply(this, args); };
  try {
    const repairing = service.exportArtifacts(run.id);
    const cancelled = assert.rejects(repairing, error => error.name === 'AbortError');
    await started;
    await assert.rejects(other.cleanup(run.id, { reviewed: true }), /Reservation conflict/);
    let closed = false;
    const closing = service.close().then(() => { closed = true; });
    await pause(10); assert.equal(closed, false);
    assert.equal(service.store.getRun(run.id).id, run.id);
    release(); await cancelled; await closing;
    assert.ok(existsSync(run.workspaces[0].path)); assert.ok(existsSync(run.workspaces[0].patchPath));
    const reopened = new CouncilStore(join(root, '.council'));
    try {
      assert.equal(reopened.events(run.id).filter(event => event.type === 'workspace.refreshed').length, 0);
      assert.deepEqual(reopened.leases(), []);
    } finally { reopened.close(); }
  } finally { release(); GitWorkspaces.prototype.export = original; }
});
