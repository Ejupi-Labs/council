import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { WorkflowEngine, MAX_CONTEXT_CHARS } from '../dist/engine.js';

const agents = ['codex', 'claude', 'junie'];
function config(root = process.cwd(), overrides = {}) {
  return { version: 1, workspaceRoot: root, stateDir: join(root, '.council'), backend: 'mock', maxConcurrency: 2,
    timeoutMs: 1000, maxOutputBytes: 4096, maxCalls: 20,
    agents: Object.fromEntries(agents.map(name => [name, { enabled: true, profile: name }])), ...overrides };
}
function store() {
  const runs = new Map();
  const log = [];
  return { runs, log, saveRun: run => runs.set(run.id, structuredClone(run)),
    appendEvent: (runId, type, data) => log.push({ runId, type, data, sequence: log.length + 1 }),
    events: (runId, after = 0) => log.filter(event => event.runId === runId && event.sequence > after) };
}
function runner(handler = async request => ({ agent: request.agent, status: 'completed', output: `${request.agent} answer`, durationMs: 1 })) {
  const calls = [];
  return { calls, doctor: async () => [], run: request => { calls.push(request); return handler(request, calls.length); } };
}
const input = (mode, overrides = {}) => ({ mode, task: 'Assess the implementation', agents: mode === 'delegate' ? ['codex'] : ['codex', 'claude'], permission: 'read', ...overrides });

test('all six workflows execute the planned call count and stage permissions', async () => {
  for (const [mode, expected] of [['delegate', 1], ['review', 3], ['debate', 5], ['parallel', 2], ['pipeline', 2], ['council', 5]]) {
    const backend = runner();
    const persistence = store();
    const engine = new WorkflowEngine(config(), backend, persistence);
    assert.equal(engine.plan(input(mode)).estimatedCalls, expected);
    const result = await engine.run(input(mode));
    assert.equal(result.status, 'completed', mode);
    assert.equal(result.steps.length, expected, mode);
    assert.equal(backend.calls.length, expected, mode);
    assert.ok(backend.calls.every(call => call.permission === 'read'));
    assert.equal(persistence.runs.get(result.id).status, 'completed');
  }
});

test('budget and input validation run before any agent dispatch', async () => {
  const backend = runner();
  const engine = new WorkflowEngine(config(), backend, store());
  for (const bad of [input('council', { maxCalls: 4 }), input('delegate', { agents: ['codex', 'claude'] }),
    input('debate', { rounds: 4 }), input('council', { permission: 'write' }),
    input('parallel', { agents: ['missing'] }), input('parallel', { agents: ['codex', 'codex'] }),
    input('parallel', { context: 'x'.repeat(MAX_CONTEXT_CHARS + 1) }), input('parallel', { maxCalls: 21 })]) {
    await assert.rejects(engine.run(bad));
  }
  assert.equal(backend.calls.length, 0);
  assert.throws(() => engine.plan(input('parallel', { judge: 'disabled' })));
});

test('one semaphore bounds agent calls across multiple simultaneous workflows', async () => {
  let active = 0;
  let peak = 0;
  const backend = runner(async request => {
    active++;
    peak = Math.max(peak, active);
    await new Promise(resolve => setTimeout(resolve, 15));
    active--;
    return { agent: request.agent, status: 'completed', output: 'done', durationMs: 15 };
  });
  const engine = new WorkflowEngine(config(), backend, store());
  const results = await Promise.all([engine.run(input('council')), engine.run(input('parallel', { agents }))]);
  assert.ok(results.every(run => run.status === 'completed'));
  assert.equal(peak, 2);
});

test('parallel assignments route distinct briefs while retaining the global task and context', async () => {
  const backend = runner();
  const engine = new WorkflowEngine(config(), backend, store());
  const request = input('parallel', {
    task: 'Assess the release readiness.', context: 'The release deadline is Friday.',
    assignments: { codex: 'Inspect the HTTP API only.', claude: 'Inspect the persistence layer only.' },
  });
  assert.equal(engine.plan(request).estimatedCalls, 2);
  const run = await engine.run(request);
  assert.equal(run.status, 'completed');
  assert.match(backend.calls[0].prompt, /Inspect the HTTP API only/);
  assert.doesNotMatch(backend.calls[0].prompt, /Inspect the persistence layer only/);
  assert.match(backend.calls[1].prompt, /Inspect the persistence layer only/);
  assert.doesNotMatch(backend.calls[1].prompt, /Inspect the HTTP API only/);
  assert.ok(backend.calls.every(call => call.prompt.includes(request.task) && call.prompt.includes(request.context)));
});

test('invalid assignments fail before dispatch and cannot target a judge or an unrelated agent', async () => {
  const backend = runner();
  const engine = new WorkflowEngine(config(), backend, store());
  for (const request of [
    input('delegate', { assignments: { codex: 'An assignment' } }),
    input('parallel', { assignments: { junie: 'Not selected' }, judge: 'junie' }),
    input('parallel', { assignments: { codex: ' ' } }),
    input('parallel', { assignments: { codex: 'x'.repeat(32_001) } }),
    input('parallel', { agents, assignments: Object.fromEntries(agents.map(agent => [agent, 'x'.repeat(21_334)])) }),
    input('parallel', { assignments: [] }),
  ]) await assert.rejects(engine.run(request), /assignments?|Assignment/);
  assert.equal(backend.calls.length, 0);
});

test('failed peers preserve partial results and prevent dependent synthesis', async () => {
  const backend = runner(async request => ({ agent: request.agent, status: request.agent === 'claude' ? 'timeout' : 'completed', output: 'partial evidence', durationMs: 1, error: request.agent === 'claude' ? 'timed out' : undefined }));
  const result = await new WorkflowEngine(config(), backend, store()).run(input('parallel', { judge: 'junie' }));
  assert.equal(result.status, 'failed');
  assert.equal(result.steps.length, 2);
  assert.match(result.error, /timed out/);
  assert.match(result.output, /partial evidence/);
  assert.ok(!backend.calls.some(call => call.agent === 'junie'));
});

test('exceptions from agents become failed steps rather than successful workflows', async () => {
  const backend = runner(async () => { throw new Error('connection lost'); });
  const result = await new WorkflowEngine(config(), backend, store()).run(input('delegate'));
  assert.equal(result.status, 'failed');
  assert.equal(result.steps[0].result.status, 'failed');
  assert.match(result.error, /connection lost/);
});

test('handoff context and final output are bounded including multi-byte text', async () => {
  const backend = runner(async request => ({ agent: request.agent, status: 'completed', output: '🌍'.repeat(10000), durationMs: 1 }));
  const result = await new WorkflowEngine(config(process.cwd(), { maxOutputBytes: 100 }), backend, store()).run(input('pipeline', { context: 'x'.repeat(MAX_CONTEXT_CHARS) }));
  assert.equal(result.status, 'completed');
  assert.ok(Buffer.byteLength(result.output) <= 100);
  assert.ok(result.steps.every(step => Buffer.byteLength(step.result.output) <= 100));
  assert.ok(backend.calls.every(call => call.prompt.length < MAX_CONTEXT_CHARS + 2000));
  assert.ok(!result.output.includes('\ufffd'));
});

test('abort cancels running work and never dispatches queued agents', async () => {
  const controller = new AbortController();
  const backend = runner(request => new Promise(resolve => {
    request.signal.addEventListener('abort', () => resolve({ agent: request.agent, status: 'cancelled', output: 'partial before cancellation', durationMs: 5 }), { once: true });
    setTimeout(() => controller.abort(), 10);
  }));
  const result = await new WorkflowEngine(config(process.cwd(), { maxConcurrency: 1 }), backend, store()).run(input('parallel', { agents }), { signal: controller.signal });
  assert.equal(result.status, 'cancelled');
  assert.equal(backend.calls.length, 1);
  assert.equal(result.steps.length, 1);
  assert.match(result.output, /partial before cancellation/);
});

test('durable cancel_requested event aborts work started by another process', async () => {
  const persistence = store();
  const backend = runner(request => new Promise(resolve => {
    request.signal.addEventListener('abort', () => resolve({ agent: request.agent, status: 'cancelled', output: 'cancelled externally', durationMs: 1 }), { once: true });
    setTimeout(() => persistence.appendEvent('durable-cancel', 'cancel_requested', {}), 10);
  }));
  const result = await new WorkflowEngine(config(), backend, persistence).run(input('delegate'), { id: 'durable-cancel' });
  assert.equal(result.status, 'cancelled');
  assert.match(result.output, /cancelled externally/);
});

function repository(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-engine-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const git = (...args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', windowsHide: true });
  git('init', '--quiet');
  git('config', 'user.email', 'test@example.invalid');
  git('config', 'user.name', 'Council Test');
  writeFileSync(join(root, 'tracked.txt'), 'base\n');
  git('add', '.');
  git('commit', '--quiet', '-m', 'initial');
  return { root, git };
}

test('write review shares author worktree with read-only reviewers and retains patch', async t => {
  const { root, git } = repository(t);
  const branch = git('symbolic-ref', 'HEAD');
  const backend = runner(async (request, count) => {
    if (request.permission === 'write') writeFileSync(join(request.cwd, 'new.txt'), `revision ${count}\n`);
    else assert.match(readFileSync(join(request.cwd, 'new.txt'), 'utf8'), /revision/);
    return { agent: request.agent, status: 'completed', output: `step ${count}`, durationMs: 1 };
  });
  const result = await new WorkflowEngine(config(root), backend, store()).run(input('review', { permission: 'write' }));
  assert.equal(result.status, 'completed');
  assert.deepEqual(backend.calls.map(call => call.permission), ['write', 'read', 'write']);
  assert.equal(new Set(backend.calls.map(call => call.cwd)).size, 1);
  assert.notEqual(backend.calls[0].cwd, root);
  assert.equal(result.workspaces.length, 1);
  assert.equal(result.workspaces[0].changed, true);
  assert.match(readFileSync(result.workspaces[0].patchPath, 'utf8'), /new.txt/);
  assert.equal(git('symbolic-ref', 'HEAD'), branch);
  assert.equal(readFileSync(join(root, 'tracked.txt'), 'utf8'), 'base\n');
});

test('write parallel isolates each worker and read-only judge', async t => {
  const { root } = repository(t);
  const backend = runner(async request => {
    if (request.permission === 'write') writeFileSync(join(request.cwd, 'worker.txt'), request.agent);
    return { agent: request.agent, status: 'completed', output: 'done', durationMs: 1 };
  });
  const result = await new WorkflowEngine(config(root), backend, store()).run(input('parallel', { permission: 'write', judge: 'junie' }));
  assert.equal(result.status, 'completed');
  assert.equal(result.workspaces.length, 2);
  assert.notEqual(backend.calls[0].cwd, backend.calls[1].cwd);
  assert.equal(backend.calls[2].permission, 'read');
  assert.equal(backend.calls[2].cwd, root);
});

test('write pipeline continues in one retained workspace', async t => {
  const { root } = repository(t);
  const backend = runner(async (request, count) => {
    if (count === 1) writeFileSync(join(request.cwd, 'handoff.txt'), 'first stage');
    else assert.equal(readFileSync(join(request.cwd, 'handoff.txt'), 'utf8'), 'first stage');
    return { agent: request.agent, status: 'completed', output: 'handoff complete', durationMs: 1 };
  });
  const result = await new WorkflowEngine(config(root), backend, store()).run(input('pipeline', { permission: 'write' }));
  assert.equal(result.status, 'completed');
  assert.equal(result.workspaces.length, 1);
  assert.equal(backend.calls[0].cwd, backend.calls[1].cwd);
});
