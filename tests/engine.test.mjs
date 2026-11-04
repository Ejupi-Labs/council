import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { execFile, execFileSync } from 'node:child_process';
import { promisify } from 'node:util';
import { WorkflowEngine, WorkflowPersistenceError, MAX_CONTEXT_CHARS } from '../dist/engine.js';
import { CouncilStore } from '../dist/store.js';
import { GitWorkspaces } from '../dist/workspaces.js';

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
    getRun: id => runs.get(id),
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
    input('debate', { rounds: 0 }), input('council', { permission: 'write' }),
    input('parallel', { agents: ['missing'] }), input('parallel', { agents: ['codex', 'codex'] }),
    input('parallel', { context: 7 }), input('parallel', { maxCalls: 21 })]) {
    await assert.rejects(engine.run(bad));
  }
  assert.equal(backend.calls.length, 0);
  assert.throws(() => engine.plan(input('parallel', { judge: 'disabled' })));
});

test('a terminal run ID cannot be reused or have its evidence overwritten', async () => {
  const backend = runner(), persistence = store();
  const engine = new WorkflowEngine(config(), backend, persistence);
  const first = await engine.run(input('delegate'), { id: 'immutable-terminal' });
  const events = persistence.log.length;
  await assert.rejects(engine.run(input('delegate', { task: 'Attempt to replace previous evidence.' }), { id: first.id }), /terminal run ID/);
  assert.equal(backend.calls.length, 1); assert.equal(persistence.log.length, events);
  assert.equal(persistence.getRun(first.id).input.task, first.input.task);
});

test('caller mutation after dispatch cannot change the validated task or later participants', async () => {
  let entered, release;
  const started = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  const backend = runner(async (request, count) => {
    if (count === 1) { entered(); await gate; }
    return { agent: request.agent, status: 'completed', output: 'bounded evidence', durationMs: 1 };
  });
  const task = input('pipeline');
  const running = new WorkflowEngine(config(), backend, store()).run(task);
  await started; task.agents[1] = 'junie'; task.task = 'Changed after validation'; release();
  const result = await running;
  assert.equal(result.status, 'completed');
  assert.deepEqual(backend.calls.map(call => call.agent), ['codex', 'claude']);
  assert.equal(result.input.task, 'Assess the implementation');
  assert.ok(backend.calls.every(call => call.prompt.includes(result.input.task) && !call.prompt.includes(task.task)));
});

test('an active run ID cannot be reused while its existing call is still quiescing', async () => {
  let entered, release;
  const started = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  const persistence = store();
  const backend = runner(async request => { entered(); await gate; return { agent: request.agent, status: 'completed', output: 'first result', durationMs: 1 }; });
  const engine = new WorkflowEngine(config(), backend, persistence);
  const first = engine.run(input('delegate'), { id: 'active-id' }); await started;
  try {
    await assert.rejects(engine.run(input('delegate', { task: 'Replay while active' }), { id: 'active-id' }), /already running/);
    assert.equal(backend.calls.length, 1); assert.equal(persistence.getRun('active-id').input.task, 'Assess the implementation');
  } finally { release(); await first; }
});

test('an intermediate evidence write failure fences queued calls and remains a failed outcome', async () => {
  const persistence = store();
  const original = persistence.appendEvent;
  let failed = false;
  persistence.appendEvent = (id, type, data) => {
    if (type === 'step.finished' && !failed) { failed = true; throw new Error('offline evidence write failure'); }
    return original(id, type, data);
  };
  const backend = runner();
  let result;
  await assert.rejects(new WorkflowEngine(config(process.cwd(), { maxConcurrency: 1 }), backend, persistence).run(input('parallel', { agents })), error => {
    assert.ok(error instanceof WorkflowPersistenceError); result = error.run; return true;
  });
  assert.equal(result.status, 'failed'); assert.deepEqual(backend.calls.map(call => call.agent), ['codex']);
  assert.match(result.error, /Could not persist workflow evidence.*offline evidence write failure/);
  assert.match(result.output, /codex answer/); assert.equal(persistence.getRun(result.id).status, 'failed');
});

test('state-failure notification fences other already queued runs before any provider can dispatch', async () => {
  const persistence = store(), original = persistence.appendEvent;
  const controllers = [new AbortController(), new AbortController()];
  let entered, release, failed = false;
  const started = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  persistence.appendEvent = (id, type, data) => {
    if (id === 'fault-origin' && type === 'step.finished' && !failed) { failed = true; throw new Error('offline cross-run evidence fault'); }
    return original(id, type, data);
  };
  const backend = runner(async request => { entered(); await gate; return { agent: request.agent, status: 'completed', output: 'origin evidence', durationMs: 1 }; });
  const notifications = [];
  const engine = new WorkflowEngine(config(process.cwd(), { maxConcurrency: 1 }), backend, persistence, {
    onStateFailure: error => { notifications.push(error); for (const controller of controllers) controller.abort(error); },
  });
  const first = engine.run(input('delegate'), { id: 'fault-origin', signal: controllers[0].signal }); await started;
  const second = engine.run(input('pipeline', { agents: ['claude', 'junie'] }), { id: 'queued-peer', signal: controllers[1].signal });
  release(); const settled = await Promise.allSettled([first, second]);
  assert.equal(notifications.length, 1); assert.match(notifications[0].message, /cross-run evidence fault/);
  assert.equal(settled[0].status, 'rejected'); assert.ok(settled[0].reason instanceof WorkflowPersistenceError);
  assert.equal(settled[0].reason.run.status, 'failed'); assert.match(settled[0].reason.run.output, /origin evidence/);
  assert.equal(settled[1].status, 'fulfilled'); assert.equal(settled[1].value.status, 'cancelled');
  assert.deepEqual(backend.calls.map(call => call.agent), ['codex']);
});

test('a throwing state-failure notifier cannot replace the retained storage fault', async () => {
  const persistence = store(), original = persistence.appendEvent;
  let failed = false;
  persistence.appendEvent = (id, type, data) => {
    if (!failed && type === 'step.finished') { failed = true; throw new Error('original storage fault'); }
    return original(id, type, data);
  };
  await assert.rejects(new WorkflowEngine(config(), runner(), persistence, { onStateFailure: () => { throw new Error('notification bug'); } }).run(input('delegate')), error => {
    assert.ok(error instanceof WorkflowPersistenceError); assert.equal(error.run.status, 'failed');
    assert.match(error.run.error, /original storage fault/); assert.doesNotMatch(error.run.error, /notification bug/); return true;
  });
});

test('evidence write failure drains active siblings and retains their partial output before finalizing', async () => {
  const persistence = store();
  const original = persistence.saveRun;
  let failed = false, siblingStopped = false, siblingEntered;
  const started = new Promise(resolve => { siblingEntered = resolve; });
  persistence.saveRun = run => {
    if (!failed && run.steps.length > 0) { failed = true; throw new Error('offline snapshot write failure'); }
    return original(run);
  };
  const backend = runner(async request => {
    if (request.agent === 'codex') { await started; return { agent: request.agent, status: 'completed', output: 'first evidence', durationMs: 1 }; }
    return await new Promise(resolve => {
      siblingEntered();
      request.signal.addEventListener('abort', () => setTimeout(() => {
        siblingStopped = true; resolve({ agent: request.agent, status: 'cancelled', output: 'drained sibling evidence', durationMs: 20 });
      }, 20), { once: true });
    });
  });
  let result;
  await assert.rejects(new WorkflowEngine(config(), backend, persistence).run(input('parallel', { agents })), error => {
    assert.ok(error instanceof WorkflowPersistenceError); result = error.run; return true;
  });
  assert.equal(result.status, 'failed'); assert.equal(siblingStopped, true);
  assert.deepEqual(backend.calls.map(call => call.agent), ['codex', 'claude']);
  assert.match(result.output, /first evidence/); assert.match(result.output, /drained sibling evidence/);
});

test('shared slot release failure retains the returned step and fences later calls before raising a storage fault', async () => {
  const persistence = store();
  persistence.acquireSlot = () => 'owned-token';
  persistence.releaseSlot = () => { throw new Error('offline slot release failure'); };
  const backend = runner();
  let retained;
  await assert.rejects(new WorkflowEngine(config(process.cwd(), { maxConcurrency: 1 }), backend, persistence, { instanceId: 'test-host' }).run(input('parallel', { agents })), error => {
    assert.ok(error instanceof WorkflowPersistenceError); retained = error.run; return true;
  });
  assert.equal(retained.status, 'failed'); assert.deepEqual(backend.calls.map(call => call.agent), ['codex']);
  assert.equal(retained.steps.length, 1); assert.equal(retained.steps[0].result.output, 'codex answer');
  assert.match(retained.error, /Could not release a shared workflow call slot/);
  assert.equal(persistence.getRun(retained.id).status, 'failed');
});

test('a missing terminal event preserves already committed completion without inventing cancellation', async () => {
  const persistence = store();
  persistence.finalizeRun = run => { persistence.saveRun(run); return structuredClone(run); };
  const original = persistence.appendEvent;
  persistence.appendEvent = (id, type, data) => {
    if (type === 'run.completed') throw new Error('offline terminal event failure');
    return original(id, type, data);
  };
  await assert.rejects(new WorkflowEngine(config(), runner(), persistence).run(input('delegate')), error => {
    assert.ok(error instanceof WorkflowPersistenceError);
    assert.equal(error.run.status, 'completed'); assert.equal(error.run.output, 'codex answer');
    assert.equal(persistence.getRun(error.run.id).status, 'completed'); return true;
  });
});

test('custom runner diagnostics cannot bypass retained output bounds through extra result fields', async () => {
  const backend = runner(async request => ({ agent: request.agent, status: 'failed', output: 'small evidence',
    durationMs: Infinity, error: 'problem '.repeat(100_000), stopReason: 'reason '.repeat(1000), diagnostics: 'unbounded extra data '.repeat(100_000) }));
  const persistence = store();
  const result = await new WorkflowEngine(config(), backend, persistence).run(input('delegate'));
  assert.equal(result.status, 'failed'); assert.ok(result.error.length <= 4096);
  assert.ok(result.steps[0].result.error.length <= 4096); assert.ok(result.steps[0].result.stopReason.length <= 512);
  assert.ok(Number.isFinite(result.steps[0].result.durationMs));
  assert.equal(Object.hasOwn(result.steps[0].result, 'diagnostics'), false);
  assert.ok(Buffer.byteLength(JSON.stringify(persistence.getRun(result.id))) < 20_000);
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
    input('parallel', { assignments: null }),
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

test('provider cancellation preserves the workflow outcome and prevents dependent stages', async () => {
  for (const mode of ['delegate', 'pipeline', 'review', 'parallel', 'debate', 'council']) {
    const persistence = store();
    const backend = runner(async request => ({ agent: request.agent, status: 'cancelled', output: 'provider partial evidence', durationMs: 1, error: 'Provider cancelled its turn.' }));
    const result = await new WorkflowEngine(config(), backend, persistence).run(input(mode, mode === 'parallel' ? { judge: 'junie' } : {}));
    const expectedCalls = ['delegate', 'pipeline', 'review'].includes(mode) ? 1 : 2;
    assert.equal(result.status, 'cancelled', mode);
    assert.equal(backend.calls.length, expectedCalls, mode);
    assert.ok(result.steps.every(step => step.result.status === 'cancelled'), mode);
    assert.match(result.error, /Provider cancelled its turn\./, mode);
    assert.match(result.output, /provider partial evidence/, mode);
    assert.equal(persistence.getRun(result.id).status, 'cancelled', mode);
    assert.equal(persistence.log.at(-1).type, 'run.cancelled', mode);
  }
});

test('failed and timed-out peers take precedence over provider cancellation in either participant order', async () => {
  for (const status of ['failed', 'timeout']) {
    for (const failingAgent of ['codex', 'claude']) {
      const backend = runner(async request => ({ agent: request.agent,
        status: request.agent === failingAgent ? status : 'cancelled', output: `${request.agent} partial evidence`, durationMs: 1,
        error: request.agent === failingAgent ? `Provider ${status} its turn.` : 'Provider cancelled its turn.' }));
      const result = await new WorkflowEngine(config(), backend, store()).run(input('parallel', { judge: 'junie' }));
      assert.equal(result.status, 'failed', `${status}: ${failingAgent}`);
      assert.equal(backend.calls.length, 2);
      assert.match(result.error, new RegExp(`${failingAgent} worker: Provider ${status} its turn\\.`));
      assert.match(result.output, /codex partial evidence/);
      assert.match(result.output, /claude partial evidence/);
      assert.ok(!backend.calls.some(call => call.agent === 'junie'));
    }
  }
});

test('exceptions from agents become failed steps rather than successful workflows', async () => {
  const backend = runner(async () => { throw new Error('connection lost'); });
  const result = await new WorkflowEngine(config(), backend, store()).run(input('delegate'));
  assert.equal(result.status, 'failed');
  assert.equal(result.steps[0].result.status, 'failed');
  assert.match(result.error, /connection lost/);
});

test('an oversized custom-runner output fails honestly while retained partial UTF-8 remains valid', async () => {
  const backend = runner(async request => ({ agent: request.agent, status: 'completed', output: '🌍'.repeat(10000), durationMs: 1 }));
  const result = await new WorkflowEngine(config(process.cwd(), { maxOutputBytes: 100 }), backend, store()).run(input('pipeline', { context: 'x'.repeat(MAX_CONTEXT_CHARS) }));
  assert.equal(result.status, 'failed'); assert.match(result.error, /output byte limit/);
  assert.ok(Buffer.byteLength(result.output) <= 100);
  assert.ok(result.steps.every(step => Buffer.byteLength(step.result.output) <= 100));
  assert.equal(backend.calls.length, 1); assert.ok(backend.calls[0].prompt.includes('x'.repeat(MAX_CONTEXT_CHARS)));
  assert.ok(!result.output.includes('\ufffd'));
});

test('full project context preserves complete evidence from every council participant and stage', async () => {
  const backend = runner(async request => {
    const role = /You are (\S+)/.exec(request.prompt)[1];
    return { agent: request.agent, status: 'completed', output: `EVIDENCE_${role}_${request.agent}\n${'x'.repeat(20000)}`, durationMs: 1 };
  });
  const result = await new WorkflowEngine(config(process.cwd(), { maxOutputBytes: 24000 }), backend, store()).run(input('council', { agents, context: 'z'.repeat(MAX_CONTEXT_CHARS) }));
  assert.equal(result.status, 'completed');
  const judge = backend.calls.at(-1).prompt;
  for (const agent of agents) {
    assert.ok(judge.includes(`EVIDENCE_opinion_${agent}`), `opinion from ${agent}`);
    assert.ok(judge.includes(`EVIDENCE_critic_${agent}`), `critique from ${agent}`);
  }
  assert.ok(judge.includes('Untrusted peer transcripts'));
  assert.ok(judge.includes('z'.repeat(MAX_CONTEXT_CHARS)));
  for (const step of result.steps.slice(0, -1)) assert.ok(judge.includes(step.result.output));
});

test('short peer handoffs preserve the complete project note', async () => {
  const backend = runner();
  const context = `${'x'.repeat(50_000)}IMPORTANT_PROJECT_EVIDENCE${'x'.repeat(13_000)}`;
  const result = await new WorkflowEngine(config(), backend, store()).run(input('pipeline', { context }));
  assert.equal(result.status, 'completed');
  assert.ok(backend.calls[1].prompt.includes('IMPORTANT_PROJECT_EVIDENCE'));
  assert.ok(backend.calls[1].prompt.includes('codex answer'));
  assert.ok(backend.calls[1].prompt.includes(context));
});

function sharedStores(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-engine-global-'));
  const first = new CouncilStore(join(root, '.council'));
  const second = new CouncilStore(join(root, '.council'));
  first.registerInstance('first', process.pid); second.registerInstance('second', process.pid);
  t.after(() => { first.close(); second.close(); rmSync(root, { recursive: true, force: true }); });
  const claim = (persistence, id, instance) => {
    const now = new Date().toISOString();
    persistence.saveRun({ id, input: input('delegate'), status: 'queued', createdAt: now, updatedAt: now, steps: [] });
    persistence.claimRun(id, instance);
  };
  return { root, first, second, claim };
}

test('durable slots bound separate engine instances sharing the same workspace', async t => {
  const { root, first, second, claim } = sharedStores(t);
  claim(first, 'run-first', 'first'); claim(second, 'run-second', 'second');
  let active = 0, peak = 0;
  const backend = runner(async request => {
    active++; peak = Math.max(peak, active);
    await new Promise(resolve => setTimeout(resolve, 40));
    active--;
    return { agent: request.agent, status: 'completed', output: 'done', durationMs: 40 };
  });
  const settings = config(root, { maxConcurrency: 1 });
  const outcomes = await Promise.all([
    new WorkflowEngine(settings, backend, first, { instanceId: 'first' }).run(input('delegate'), { id: 'run-first' }),
    new WorkflowEngine(settings, backend, second, { instanceId: 'second' }).run(input('delegate'), { id: 'run-second' }),
  ]);
  assert.equal(peak, 1);
  assert.ok(outcomes.every(run => run.status === 'completed'));
  claim(first, 'probe', 'first');
  const token = first.acquireSlot('first', 'probe', 1);
  assert.ok(token, 'completed calls released their durable slots');
  first.releaseSlot(token, 'first');
});

test('durable cancellation stops a queued global-slot waiter before provider dispatch', async t => {
  const { root, first, second, claim } = sharedStores(t);
  claim(first, 'holder', 'first'); claim(second, 'waiter', 'second');
  const held = first.acquireSlot('first', 'holder', 1);
  const backend = runner();
  setTimeout(() => first.requestCancellation('waiter'), 30);
  const result = await new WorkflowEngine(config(root, { maxConcurrency: 1 }), backend, second, { instanceId: 'second' }).run(input('delegate'), { id: 'waiter' });
  first.releaseSlot(held, 'first');
  assert.equal(result.status, 'cancelled');
  assert.equal(backend.calls.length, 0);
  assert.equal(second.health().counts.slotWaiters, 0);
});

test('a foreign queued call receives the next durable slot before an incumbent pipeline reacquires it', { timeout: 10_000 }, async t => {
  const { root, first, second, claim } = sharedStores(t);
  claim(first, 'incumbent-pipeline', 'first'); claim(second, 'foreign-call', 'second');
  let entered, release;
  const started = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  const order = [];
  const incumbent = runner(async (request, count) => {
    order.push(`incumbent-${request.agent}`);
    if (count === 1) { entered(); await gate; }
    return { agent: request.agent, status: 'completed', output: 'pipeline evidence', durationMs: 1 };
  });
  const foreign = runner(async request => { order.push('foreign'); return { agent: request.agent, status: 'completed', output: 'foreign evidence', durationMs: 1 }; });
  const settings = config(root, { maxConcurrency: 1 });
  const firstRun = new WorkflowEngine(settings, incumbent, first, { instanceId: 'first' }).run(input('pipeline', { agents }), { id: 'incumbent-pipeline' });
  await started;
  const secondRun = new WorkflowEngine(settings, foreign, second, { instanceId: 'second' }).run(input('delegate'), { id: 'foreign-call' });
  try {
    const until = Date.now() + 3000;
    while (first.health().counts.slotWaiters < 1 && Date.now() < until) await new Promise(resolve => setTimeout(resolve, 5));
    assert.equal(first.health().counts.slotWaiters, 1); release();
    const outcomes = await Promise.all([firstRun, secondRun]);
    assert.ok(outcomes.every(run => run.status === 'completed'));
    assert.deepEqual(order, ['incumbent-codex', 'foreign', 'incumbent-claude', 'incumbent-junie']);
    assert.equal(first.health().counts.slotWaiters, 0); assert.equal(first.health().counts.slots, 0);
  } finally { release(); await Promise.allSettled([firstRun, secondRun]); }
});

test('independent Node processes share the workspace agent-call limit', { timeout: 15_000 }, async t => {
  const temporaryBase = realpathSync(tmpdir());
  const root = mkdtempSync(join(temporaryBase, 'council-process-slots-'));
  let passed = false;
  t.after(() => {
    if (!passed) { t.diagnostic(`Preserved independent-process fixture: ${root}`); return; }
    const target = realpathSync(root);
    assert.equal(dirname(target), temporaryBase); assert.ok(basename(target).startsWith('council-process-slots-'));
    rmSync(target, { recursive: true, force: true });
  });
  // Cold migrations have dedicated tests; this case isolates concurrent agent dispatch.
  const initialized = new CouncilStore(join(root, '.council')); initialized.close();
  const execute = promisify(execFile);
  const source = id => `
import {appendFileSync,existsSync,writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {setTimeout as delay} from 'node:timers/promises';
import {WorkflowEngine} from ${JSON.stringify(new URL('../dist/engine.js', import.meta.url).href)};
import {CouncilStore} from ${JSON.stringify(new URL('../dist/store.js', import.meta.url).href)};
const root=${JSON.stringify(root)}, id=${JSON.stringify(id)};
const config=${JSON.stringify(config(root, { maxConcurrency: 1 }))};
const store=new CouncilStore(config.stateDir);
store.registerInstance(id,process.pid);
const input={mode:'delegate',task:'Offline concurrency check.',agents:['codex'],permission:'read'};
const now=new Date().toISOString();
store.saveRun({id,input,status:'queued',createdAt:now,updatedAt:now,steps:[]});store.claimRun(id,id);
writeFileSync(join(root,id+'.ready'),'ready');
while(!existsSync(join(root,'dispatch.ready')))await delay(10);
const runner={doctor:async()=>[],run:async request=>{appendFileSync(join(root,'calls.log'),'start '+id+'\\n');await delay(80);appendFileSync(join(root,'calls.log'),'end '+id+'\\n');return {agent:request.agent,status:'completed',output:'offline done',durationMs:80};}};
const result=await new WorkflowEngine(config,runner,store,{instanceId:id}).run(input,{id});
if(result.status!=='completed')throw new Error(result.error);
store.unregisterInstance(id);store.close();
`;
  const jobs = []; let dispatchReleased = false, observedFailure, setupError;
  const terminatePeers = () => {
    for (const job of jobs) if (!job.settled) { job.parentTerminated = true; job.child?.kill(); }
  };
  const startChild = id => {
    const work = execute(process.execPath, ['--input-type=module', '-e', source(id)], { windowsHide: true, timeout: 10_000 });
    const job = { id, child: work.child }; jobs.push(job);
    const finish = result => {
      job.settled = result;
      if (result.status === 'rejected' || !dispatchReleased) {
        observedFailure ??= { id, phase: dispatchReleased ? 'dispatch' : 'admission' }; terminatePeers();
      }
      return result;
    };
    job.outcome = work.then(value => finish({ status: 'fulfilled', value }), reason => finish({ status: 'rejected', reason }));
    return job;
  };
  const waitReady = async job => {
    while (!existsSync(join(root, job.id + '.ready'))) {
      if (observedFailure) throw new Error(`Child ${observedFailure.id} exited during ${observedFailure.phase}.`);
      await new Promise(resolve => setTimeout(resolve, 10));
    }
    if (observedFailure) throw new Error(`Child ${observedFailure.id} exited during ${observedFailure.phase}.`);
  };
  try {
    await waitReady(startChild('one')); await waitReady(startChild('two'));
    writeFileSync(join(root, 'dispatch.ready'), 'dispatch'); dispatchReleased = true;
  } catch (error) { setupError = error.message; terminatePeers(); }
  const settled = await Promise.all(jobs.map(job => job.outcome));
  const diagnostics = settled.map((result, index) => result.status === 'fulfilled'
    ? { id: jobs[index].id, status: result.status, stdout: result.value.stdout, stderr: result.value.stderr }
    : { id: jobs[index].id, status: result.status, parentTerminated: jobs[index].parentTerminated === true, message: result.reason.message?.split('\n')[0], code: result.reason.code,
      signal: result.reason.signal, killed: result.reason.killed, stdout: result.reason.stdout, stderr: result.reason.stderr });
  const evidence = { fixture: root, dispatchReleased, observedFailure, setupError, children: diagnostics };
  writeFileSync(join(root, 'child-results.json'), JSON.stringify(evidence, null, 2) + '\n');
  assert.deepEqual(settled.map(result => result.status), ['fulfilled', 'fulfilled'], JSON.stringify(evidence, null, 2));
  const events = readFileSync(join(root, 'calls.log'), 'utf8').trim().split('\n');
  let active = 0, peak = 0;
  for (const event of events) { active += event.startsWith('start') ? 1 : -1; peak = Math.max(peak, active); }
  assert.equal(events.length, 4); assert.equal(active, 0); assert.equal(peak, 1);
  passed = true;
});

test('atomic finalization honors cancellation arriving after the last engine check', async t => {
  const { root, first, second, claim } = sharedStores(t);
  claim(first, 'finalization-race', 'first');
  const finalize = first.finalizeRun.bind(first);
  first.finalizeRun = run => { assert.equal(second.requestCancellation(run.id), true); return finalize(run); };
  const result = await new WorkflowEngine(config(root), runner(), first, { instanceId: 'first' }).run(input('delegate'), { id: 'finalization-race' });
  assert.equal(result.status, 'cancelled');
  assert.equal(first.getRun(result.id).status, 'cancelled');
  assert.equal(first.events(result.id).at(-1).type, 'run.cancelled');
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

test('cancellation polling prefers small metadata over event payloads', async () => {
  const persistence = store(); let metadataReads = 0;
  persistence.isCancellationRequested = () => { metadataReads++; return false; };
  persistence.events = () => { throw new Error('Full event payloads must not be polled.'); };
  const result = await new WorkflowEngine(config(), runner(), persistence).run(input('council'));
  assert.equal(result.status, 'completed'); assert.ok(metadataReads > 0);
});

test('a failed cancellation read prevents further dispatch instead of silently ignoring it', async () => {
  const persistence = store(), backend = runner();
  persistence.isCancellationRequested = () => { throw new Error('offline injected database error'); };
  const result = await new WorkflowEngine(config(), backend, persistence).run(input('delegate'));
  assert.equal(result.status, 'failed'); assert.match(result.error, /Cannot verify durable cancellation/);
  assert.equal(backend.calls.length, 0);
});

test('a dispatch cancellation-read fault fences queued siblings while transient failure preserves active work', async () => {
  const persistence = store();
  let firstFinished = false, failedRead = false, siblingEntered, activeWasAborted = false;
  const ready = new Promise(resolve => { siblingEntered = resolve; });
  persistence.isCancellationRequested = () => {
    if (firstFinished && !failedRead) { failedRead = true; throw new Error('offline transient cancellation read'); }
    return false;
  };
  const backend = runner(async request => {
    if (request.agent === 'codex') {
      await ready; firstFinished = true;
      return { agent: request.agent, status: 'completed', output: 'first bounded evidence', durationMs: 1 };
    }
    siblingEntered(); request.signal.addEventListener('abort', () => { activeWasAborted = true; }, { once: true });
    await new Promise(resolve => setTimeout(resolve, 30));
    return { agent: request.agent, status: 'completed', output: 'active work preserved', durationMs: 30 };
  });
  const result = await new WorkflowEngine(config(), backend, persistence).run(input('parallel', { agents }));
  assert.equal(result.status, 'failed'); assert.match(result.error, /Cannot verify durable cancellation state/);
  assert.deepEqual(backend.calls.map(call => call.agent), ['codex', 'claude']);
  assert.equal(activeWasAborted, false); assert.match(result.output, /active work preserved/);
});

test('a transient polling error preserves active work while persistent failures stop it', { timeout: 10_000 }, async () => {
  for (const persistent of [false, true]) {
    const persistence = store(); let running = false, failures = 0;
    persistence.isCancellationRequested = () => {
      if (running && (persistent || failures === 0)) { failures++; throw new Error('offline transient read failure'); }
      return false;
    };
    const backend = runner(request => new Promise(resolve => {
      running = true;
      const timer = setTimeout(() => resolve({ agent: request.agent, status: 'completed', output: 'active work completed', durationMs: 650 }), persistent ? 6000 : 650);
      request.signal.addEventListener('abort', () => { clearTimeout(timer); resolve({ agent: request.agent, status: 'cancelled', output: 'partial evidence retained', durationMs: 1 }); }, { once: true });
    }));
    const result = await new WorkflowEngine(config(), backend, persistence).run(input('delegate'));
    assert.equal(result.status, persistent ? 'failed' : 'completed');
    if (persistent) { assert.ok(failures >= 3); assert.match(result.error, /Cannot verify durable cancellation/); assert.match(result.output, /partial evidence/); }
  }
});

test('global-slot polling backs off while cancellation remains responsive', { timeout: 2000 }, async () => {
  const persistence = store(), backend = runner(), controller = new AbortController(); let attempts = 0;
  persistence.acquireSlot = () => { attempts++; return undefined; }; persistence.releaseSlot = () => {};
  const timer = setTimeout(() => controller.abort(), 500);
  try {
    const result = await new WorkflowEngine(config(), backend, persistence, { instanceId: 'offline' }).run(input('delegate'), { signal: controller.signal });
    assert.equal(result.status, 'cancelled'); assert.ok(attempts <= 5, `expected backed-off polling, received ${attempts} attempts`); assert.equal(backend.calls.length, 0);
  } finally { clearTimeout(timer); }
});

test('global-slot retries preserve one durable waiter ID and successful acquisition does not issue redundant cleanup', async () => {
  const persistence = store();
  const seen = [], removed = [], released = [];
  persistence.acquireSlot = (instanceId, runId, limit, waiterId) => { seen.push({ instanceId, runId, limit, waiterId }); return seen.length < 3 ? undefined : 'granted-slot'; };
  persistence.cancelSlotWaiter = (_instanceId, waiterId) => { removed.push(waiterId); };
  persistence.releaseSlot = (token, instanceId) => { released.push({ token, instanceId }); };
  const result = await new WorkflowEngine(config(), runner(), persistence, { instanceId: 'fifo-owner' }).run(input('delegate'));
  assert.equal(result.status, 'completed'); assert.equal(seen.length, 3);
  assert.equal(new Set(seen.map(value => value.waiterId)).size, 1); assert.match(seen[0].waiterId, /^[a-f0-9-]{36}$/);
  assert.ok(seen.every(value => value.instanceId === 'fifo-owner' && value.runId === result.id));
  assert.deepEqual(removed, []); assert.deepEqual(released, [{ token: 'granted-slot', instanceId: 'fifo-owner' }]);
});

test('cancelling a global-slot wait removes its durable waiter before finalizing', async () => {
  const persistence = store(), controller = new AbortController();
  let entered, waiter;
  const started = new Promise(resolve => { entered = resolve; });
  const removed = [];
  persistence.acquireSlot = (_instanceId, _runId, _limit, waiterId) => { waiter = waiterId; entered(); return undefined; };
  persistence.cancelSlotWaiter = (instanceId, waiterId) => { removed.push({ instanceId, waiterId }); };
  persistence.releaseSlot = () => { assert.fail('No slot was granted.'); };
  const backend = runner();
  const running = new WorkflowEngine(config(), backend, persistence, { instanceId: 'waiting-owner' }).run(input('delegate'), { signal: controller.signal });
  await started; controller.abort(); const result = await running;
  assert.equal(result.status, 'cancelled'); assert.equal(backend.calls.length, 0);
  assert.deepEqual(removed, [{ instanceId: 'waiting-owner', waiterId: waiter }]);
});

test('pipeline stages receive full earlier evidence even when the immediate predecessor omits it', async () => {
  const backend = runner(async (request, count) => ({ agent: request.agent, status: 'completed', output: count === 1 ? 'UNIQUE_FIRST_STAGE_EVIDENCE' : count === 2 ? 'Second-stage handoff omits the earlier fact.' : 'Final stage result.', durationMs: 1 }));
  const result = await new WorkflowEngine(config(), backend, store()).run(input('pipeline', { agents, context: 'P'.repeat(MAX_CONTEXT_CHARS) }));
  assert.equal(result.status, 'completed'); assert.equal(result.output, 'Final stage result.');
  assert.ok(backend.calls[2].prompt.includes('UNIQUE_FIRST_STAGE_EVIDENCE'));
  assert.ok(backend.calls[2].prompt.includes('Second-stage handoff'));
  assert.ok(backend.calls[2].prompt.includes('P'.repeat(MAX_CONTEXT_CHARS)));
});

test('short peer replies and the full project note survive together without truncation', async () => {
  const backend = runner(async request => ({ agent: request.agent, status: 'completed', output: `COMPLETE_${request.agent}_REPLY`, durationMs: 1 }));
  const result = await new WorkflowEngine(config(), backend, store()).run(input('parallel', { judge: 'junie', context: 'P'.repeat(MAX_CONTEXT_CHARS) }));
  assert.equal(result.status, 'completed');
  const judge = backend.calls.at(-1).prompt;
  assert.ok(judge.includes('COMPLETE_codex_REPLY')); assert.ok(judge.includes('COMPLETE_claude_REPLY'));
  assert.ok(judge.includes('P'.repeat(MAX_CONTEXT_CHARS)));
});

test('parallel selected output that exceeds its configured budget fails with complete retained worker evidence', async () => {
  const backend = runner(async request => ({ agent: request.agent, status: 'completed', output: `UNIQUE_${request.agent}\n` + '🌍'.repeat(200), durationMs: 1 }));
  const result = await new WorkflowEngine(config(process.cwd(), { maxOutputBytes: 1024 }), backend, store()).run(input('parallel', { agents }));
  assert.equal(result.status, 'failed'); assert.match(result.error, /Selected workflow output exceeds configured maxOutputBytes/);
  assert.ok(Buffer.byteLength(result.output) <= 1024);
  assert.ok(!result.output.includes('\uFFFD'));
  for (const agent of agents) {
    const step = result.steps.find(step => step.agent === agent);
    assert.equal(step.result.status, 'completed'); assert.equal(step.result.output, `UNIQUE_${agent}\n` + '🌍'.repeat(200));
  }
});

test('parallel output within its configured budget includes complete short and long worker answers', async () => {
  const backend = runner(async request => ({ agent: request.agent, status: 'completed', output: request.agent === 'codex' ? 'short' : 'long evidence '.repeat(30), durationMs: 1 }));
  const result = await new WorkflowEngine(config(process.cwd(), { maxOutputBytes: 1024 }), backend, store()).run(input('parallel'));
  assert.equal(result.status, 'completed');
  assert.equal(result.output, '[codex]\nshort\n\n[claude]\n' + 'long evidence '.repeat(30));
});

test('a full durable waiter queue is an operational failure rather than an evidence-write fault', async () => {
  const persistence = store(), backend = runner(); let cleared = 0;
  persistence.acquireSlot = () => { throw Object.assign(new Error('Shared call waiter capacity reached.'), { code: 'COUNCIL_SLOT_QUEUE_FULL' }); };
  persistence.cancelSlotWaiter = () => { cleared++; }; persistence.releaseSlot = () => {};
  const result = await new WorkflowEngine(config(), backend, persistence, { instanceId: 'queue-owner' }).run(input('delegate'));
  assert.equal(result.status, 'failed'); assert.match(result.error, /waiter capacity reached/);
  assert.equal(backend.calls.length, 0); assert.equal(cleared, 1);
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
  assert.equal(result.status, 'completed', result.error);
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
  assert.equal(result.status, 'completed', result.error);
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
  assert.equal(result.status, 'completed', result.error);
  assert.equal(result.workspaces.length, 1);
  assert.equal(backend.calls[0].cwd, backend.calls[1].cwd);
});

test('cancelled finalization has a bounded artifact budget and retains an exportable worktree', { timeout: 60_000 }, async t => {
  const { root } = repository(t);
  const controller = new AbortController();
  const original = GitWorkspaces.prototype.export;
  GitWorkspaces.prototype.export = async function (_artifact, signal) {
    return await new Promise((_resolve, reject) => {
      if (signal.aborted) reject(signal.reason);
      else signal.addEventListener('abort', () => reject(signal.reason), { once: true });
    });
  };
  let result;
  try {
    const backend = runner(async request => {
      writeFileSync(join(request.cwd, 'retained.txt'), 'evidence survives cancellation\n');
      controller.abort(); return { agent: request.agent, status: 'completed', output: 'partial offline work', durationMs: 1 };
    });
    result = await new WorkflowEngine(config(root), backend, store(), { cancellationArtifactTimeoutMs: 40 }).run(input('delegate', { permission: 'write' }), { signal: controller.signal });
  } finally { GitWorkspaces.prototype.export = original; }
  assert.equal(result.status, 'cancelled'); assert.equal(result.workspaces[0].exportStatus, 'pending');
  assert.match(result.error, /deadline exceeded/); assert.match(result.error, /Workflow cancelled/);
  assert.equal(readFileSync(join(result.workspaces[0].path, 'retained.txt'), 'utf8'), 'evidence survives cancellation\n');
  const artifact = await new GitWorkspaces(root, join(root, '.council')).export(result.workspaces[0]);
  assert.equal(artifact.exportStatus, 'ready'); assert.match(readFileSync(artifact.patchPath, 'utf8'), /evidence survives cancellation/);
});

test('provider cancellation applies the short artifact deadline without an external abort', { timeout: 60_000 }, async t => {
  const { root } = repository(t);
  const original = GitWorkspaces.prototype.export;
  GitWorkspaces.prototype.export = async function (_artifact, signal) {
    t.mock.timers.tick(40);
    signal.throwIfAborted();
    throw new Error('Provider cancellation did not apply its artifact deadline.');
  };
  let result;
  try {
    const backend = runner(async request => {
      writeFileSync(join(request.cwd, 'retained.txt'), 'evidence survives provider cancellation\n');
      t.mock.timers.enable({ apis: ['setTimeout'] });
      return { agent: request.agent, status: 'cancelled', output: 'partial provider work', durationMs: 1, error: 'Provider cancelled its turn.' };
    });
    result = await new WorkflowEngine(config(root), backend, store(), { artifactTimeoutMs: 1000, cancellationArtifactTimeoutMs: 40 }).run(input('delegate', { permission: 'write' }));
  } finally { t.mock.timers.reset(); GitWorkspaces.prototype.export = original; }
  assert.equal(result.status, 'cancelled'); assert.equal(result.workspaces[0].exportStatus, 'pending');
  assert.match(result.error, /Provider cancelled its turn/); assert.match(result.error, /deadline exceeded/);
  assert.equal(readFileSync(join(result.workspaces[0].path, 'retained.txt'), 'utf8'), 'evidence survives provider cancellation\n');
  const artifact = await new GitWorkspaces(root, join(root, '.council')).export(result.workspaces[0]);
  assert.equal(artifact.exportStatus, 'ready'); assert.match(readFileSync(artifact.patchPath, 'utf8'), /evidence survives provider cancellation/);
});

test('failed worktree creation retains its pending artifact before filesystem effects', async t => {
  const { root } = repository(t);
  const original = GitWorkspaces.prototype.git;
  const persistence = store(), backend = runner();
  let pending;
  GitWorkspaces.prototype.git = async function (cwd, args, ...rest) {
    if (args[0] === 'worktree' && args[1] === 'add') {
      pending = [...persistence.runs.values()][0].workspaces[0];
      assert.ok(pending); assert.equal(pending.exportStatus, 'pending');
      throw new Error('offline checkout failure');
    }
    return original.call(this, cwd, args, ...rest);
  };
  let result;
  try { result = await new WorkflowEngine(config(root), backend, persistence).run(input('delegate', { permission: 'write' })); }
  finally { GitWorkspaces.prototype.git = original; }
  assert.equal(result.status, 'failed'); assert.equal(result.workspaces.length, 1);
  assert.equal(result.workspaces[0].exportStatus, 'pending');
  assert.match(result.workspaces[0].exportError, /preparation did not finish/);
  assert.equal(backend.calls.length, 0);
});
