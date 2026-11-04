import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { WorkflowEngine } from '../dist/engine.js';
import { parseWorkflow } from '../dist/config.js';

const config = (overrides = {}) => ({ version: 1, backend: 'mock', workspaceRoot: process.cwd(), stateDir: join(process.cwd(), '.council'),
  maxConcurrency: 1, timeoutMs: 15000, maxCalls: 20, maxOutputBytes: 131072,
  agents: { worker: { enabled: true, profile: 'codex', model: 'cheap-default', effort: 'low' },
    thinker: { enabled: true, profile: 'claude', model: 'deep-default', effort: 'high' }, reviewer: { enabled: true, profile: 'junie' } }, ...overrides });
const task = (overrides = {}) => ({ mode: 'delegate', task: 'Implement the assigned change and verify it.', agents: ['worker'], permission: 'read', advisors: { worker: { agent: 'thinker' } }, ...overrides });
const request = (question = 'Which invariant matters?', context = 'Local evidence.') => JSON.stringify({ council_advisor_request: { question, context } });
const answer = (agent, output, status = 'completed') => ({ agent, output, status, durationMs: 1, ...(status === 'completed' ? {} : { error: `Offline ${status} advisor.` }) });
function persistence() {
  const runs = new Map(), log = [];
  return { runs, log, getRun: id => runs.get(id), saveRun: run => runs.set(run.id, structuredClone(run)), appendEvent: (id, type, data) => log.push({ id, type, data }) };
}
function backend(handler) {
  const calls = [];
  return { calls, doctor: async () => [], run: async call => { calls.push(call); return await handler(call, calls.length); } };
}

test('advisor planning reserves initial calls plus both calls for every permitted request', () => {
  const engine = new WorkflowEngine(config(), backend(() => {}), persistence());
  assert.equal(engine.plan(task()).estimatedCalls, 3);
  assert.equal(engine.plan(task({ advisors: { worker: { agent: 'thinker', maxRequests: 2 } } })).estimatedCalls, 5);
  assert.equal(engine.plan(task({ advisors: { worker: { agent: 'thinker', maxRequests: 3 } }, maxCalls: 7 })).estimatedCalls, 7);
  assert.throws(() => engine.plan(task({ advisors: { worker: { agent: 'thinker', maxRequests: 3 } }, maxCalls: 6 })), /requires 7 calls/);
  assert.throws(() => engine.plan(task({ maxCalls: 2 })), /requires 3 calls/);
  const parsed = parseWorkflow(task({ agentSettings: { worker: { model: 'cheap-explicit', effort: 'low' }, thinker: { model: 'deep-explicit', effort: 'max' } }, contextLimit: 1000 }));
  assert.equal(parsed.advisors.worker.maxRequests, 1);
  for (const invalid of [task({ advisors: { other: { agent: 'thinker' } } }), task({ advisors: { worker: { agent: 'missing' } } }),
    task({ advisors: { worker: { agent: 'worker' } } }), task({ advisors: { worker: { agent: 'thinker', maxRequests: 0 } } }),
    task({ agentSettings: { codex: { model: 'wrong-alias' } } }), task({ contextLimit: 0 }), task({ agentSettings: { worker: { effort: 'extreme' } } })]) {
    assert.throws(() => engine.plan(invalid));
  }
});

test('advisor advice releases admission, remains read-only and resumes the same worker with isolated settings', async () => {
  const settings = config(), before = structuredClone(settings), store = persistence();
  let active = 0;
  store.acquireSlot = () => { assert.equal(active, 0); active++; return 'slot'; };
  store.releaseSlot = () => { active--; };
  const runner = backend((call, count) => {
    assert.equal(active, 1);
    if (count === 1) { assert.match(call.prompt, /Council advisor protocol/); return answer(call.agent, request()); }
    if (count === 2) {
      assert.equal(call.agent, 'thinker'); assert.equal(call.permission, 'read');
      assert.ok(!call.prompt.includes('Council advisor protocol'));
      assert.match(call.prompt, /Which invariant matters/);
      return answer(call.agent, 'ADVISOR_INVARIANT: preserve atomicity.');
    }
    assert.equal(call.agent, 'worker'); assert.match(call.prompt, /ADVISOR_INVARIANT/);
    return answer(call.agent, 'Verified final implementation.');
  });
  const result = await new WorkflowEngine(settings, runner, store, { instanceId: 'offline-owner' }).run(task({
    agentSettings: { worker: { model: 'cheap-explicit', effort: 'medium' }, thinker: { model: 'deep-explicit', effort: 'max' } },
  }));
  assert.equal(result.status, 'completed', result.error); assert.equal(result.output, 'Verified final implementation.');
  assert.deepEqual(runner.calls.map(call => [call.agent, call.model, call.effort]), [['worker', 'cheap-explicit', 'medium'], ['thinker', 'deep-explicit', 'max'], ['worker', 'cheap-explicit', 'medium']]);
  assert.equal(new Set(runner.calls.map(call => call.cwd)).size, 1); assert.equal(active, 0);
  assert.equal(result.steps.length, 3); assert.deepEqual(settings, before);
  assert.deepEqual(store.log.filter(event => event.type.startsWith('advisor.')).map(event => event.type), ['advisor.requested', 'advisor.answered']);
});

test('a worker can use its configured advisor quota and requests beyond the reserved quota fail', async () => {
  for (const maxRequests of [1, 2, 3]) {
    const runner = backend(call => answer(call.agent, call.agent === 'thinker' ? 'Bounded advice.' : request()));
    const result = await new WorkflowEngine(config(), runner, persistence()).run(task({ advisors: { worker: { agent: 'thinker', maxRequests } } }));
    assert.equal(result.status, 'failed'); assert.match(result.error, /advisor request limit/);
    assert.equal(runner.calls.length, 1 + 2 * maxRequests);
    assert.equal(result.steps.length, runner.calls.length);
  }
});

test('three explicit advisor requests complete successfully within a seven-call user budget', async () => {
  let workerCalls = 0;
  const runner = backend(call => answer(call.agent, call.agent === 'thinker' ? 'Focused advice.' : ++workerCalls <= 3 ? request() : 'Verified final result.'));
  const result = await new WorkflowEngine(config({ maxCalls: 7 }), runner, persistence()).run(task({ advisors: { worker: { agent: 'thinker', maxRequests: 3 } }, maxCalls: 7 }));
  assert.equal(result.status, 'completed', result.error); assert.equal(result.output, 'Verified final result.');
  assert.equal(runner.calls.length, 7); assert.equal(workerCalls, 4);
});

test('advisor failure, timeout and cancellation preserve partial evidence and prevent worker resumption', async () => {
  for (const status of ['failed', 'timeout', 'cancelled']) {
    const runner = backend(call => answer(call.agent, call.agent === 'worker' ? request() : 'Retained advisor partial evidence.', call.agent === 'worker' ? 'completed' : status));
    const result = await new WorkflowEngine(config(), runner, persistence()).run(task());
    assert.equal(result.status, status === 'cancelled' ? 'cancelled' : 'failed');
    assert.equal(runner.calls.length, 2); assert.match(result.output, /Retained advisor partial evidence/);
    assert.match(result.error, /Offline/);
  }
});

test('advisor recursion and unconfigured requests cannot become successful final task output', async () => {
  for (const advisors of [undefined, { worker: { agent: 'thinker' } }]) {
    const runner = backend(call => answer(call.agent, request()));
    const result = await new WorkflowEngine(config(), runner, persistence()).run(task({ advisors }));
    assert.equal(result.status, 'failed'); assert.match(result.error, advisors ? /recursion is disabled/ : /none is configured/);
    assert.equal(runner.calls.length, advisors ? 2 : 1);
    if (!advisors) assert.ok(!runner.calls[0].prompt.includes('Council advisor protocol'));
  }
});

test('only an exact advisor envelope invokes control flow and malformed request fields fail honestly', async () => {
  for (const output of [`Example:\n${request()}`, JSON.stringify({ council_advisor_request: { question: 'Example' }, explanation: 'ordinary data' })]) {
    const runner = backend(call => answer(call.agent, output));
    const result = await new WorkflowEngine(config(), runner, persistence()).run(task());
    assert.equal(result.status, 'completed'); assert.equal(runner.calls.length, 1);
  }
  for (const body of [null, {}, { question: ' ' }, { question: 1 }, { question: 'q', context: [] }, { question: 'q', extra: true }]) {
    const runner = backend(call => answer(call.agent, JSON.stringify({ council_advisor_request: body })));
    const result = await new WorkflowEngine(config(), runner, persistence()).run(task());
    assert.equal(result.status, 'failed'); assert.equal(runner.calls.length, 1);
  }
});

test('project, worker and advisor evidence remain full beyond former context and question caps', async () => {
  const project = 'Project evidence. '.repeat(10000) + 'PROJECT_END';
  const question = 'Question detail. '.repeat(600) + 'QUESTION_END';
  const workerContext = 'Worker evidence. '.repeat(5000) + 'WORKER_END';
  const advice = 'Advisor advice. '.repeat(5000) + 'ADVISOR_END';
  const runner = backend((call, count) => {
    assert.ok(call.prompt.includes(project));
    if (count === 2) { assert.ok(call.prompt.includes(question)); assert.ok(call.prompt.includes(workerContext)); }
    if (count === 3) { assert.ok(call.prompt.includes(workerContext)); assert.ok(call.prompt.includes(advice)); }
    return answer(call.agent, count === 1 ? request(question, workerContext) : count === 2 ? advice : 'Final result.');
  });
  const result = await new WorkflowEngine(config(), runner, persistence()).run(task({ context: project, contextLimit: null }));
  assert.equal(result.status, 'completed', result.error); assert.equal(runner.calls.length, 3);
});

test('an explicit context budget rejects oversized evidence instead of silently omitting it', async () => {
  for (const initialContext of ['', 'Project evidence. '.repeat(1000)]) {
    const runner = backend((call, count) => answer(call.agent, count === 1 ? request('Question?', 'e'.repeat(500)) : 'Advice. '.repeat(100)));
    const result = await new WorkflowEngine(config(), runner, persistence()).run(task({ context: initialContext, contextLimit: 1000 }));
    assert.equal(result.status, 'failed'); assert.match(result.error, /Increase the budget or set contextLimit to null/);
    assert.equal(result.input.context, initialContext);
    assert.equal(runner.calls.length, initialContext ? 0 : 2);
  }
});

test('clipping an oversized advisor envelope cannot turn an unfinished request into successful text', async () => {
  const runner = backend(call => answer(call.agent, request('A focused question?', 'e'.repeat(2000))));
  const result = await new WorkflowEngine(config({ maxOutputBytes: 1024 }), runner, persistence()).run(task());
  assert.equal(result.status, 'failed'); assert.equal(runner.calls.length, 1);
  assert.match(result.error, /Agent output.*byte limit/);
  assert.ok(Buffer.byteLength(result.steps[0].result.output) <= 1024);
});

test('advisor quota applies across all stages for a worker rather than resetting after a review', async () => {
  let workerCalls = 0;
  const runner = backend(call => {
    if (call.agent === 'worker') return answer(call.agent, ++workerCalls === 2 ? 'Initial draft.' : request());
    return answer(call.agent, 'Review or advice.');
  });
  const input = task({ mode: 'review', agents: ['worker', 'reviewer'] });
  const engine = new WorkflowEngine(config(), runner, persistence());
  assert.equal(engine.plan(input).estimatedCalls, 5);
  const result = await engine.run(input);
  assert.equal(result.status, 'failed'); assert.match(result.error, /advisor request limit/);
  assert.equal(runner.calls.length, 5); assert.equal(runner.calls.filter(call => call.agent === 'thinker').length, 1);
});

test('external cancellation while awaiting advice never dispatches a worker continuation', async () => {
  const controller = new AbortController();
  const runner = backend(call => {
    if (call.agent === 'worker') return answer(call.agent, request());
    controller.abort(); return answer(call.agent, 'Cancelled advisor evidence.', 'cancelled');
  });
  const result = await new WorkflowEngine(config(), runner, persistence()).run(task(), { signal: controller.signal });
  assert.equal(result.status, 'cancelled'); assert.equal(runner.calls.length, 2);
});

test('write advice observes the retained worker checkout and continuation edits that same checkout', { timeout: 60000 }, async t => {
  const root = mkdtempSync(join(tmpdir(), 'council-advisor-write-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const git = (...args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', windowsHide: true });
  git('init', '--quiet'); git('config', 'user.email', 'test@example.invalid'); git('config', 'user.name', 'Council Test');
  git('config', 'core.autocrlf', 'false');
  writeFileSync(join(root, 'base.txt'), 'original\n'); git('add', '.'); git('commit', '--quiet', '-m', 'initial');
  const runner = backend((call, count) => {
    if (count === 1) { writeFileSync(join(call.cwd, 'change.txt'), 'draft\n'); return answer(call.agent, request('Review draft?', 'change.txt is ready.')); }
    assert.equal(readFileSync(join(call.cwd, 'change.txt'), 'utf8'), 'draft\n');
    if (count === 2) { assert.equal(call.permission, 'read'); return answer(call.agent, 'Make the final edit.'); }
    writeFileSync(join(call.cwd, 'change.txt'), 'final\n'); return answer(call.agent, 'Final edit verified.');
  });
  const result = await new WorkflowEngine(config({ workspaceRoot: root, stateDir: join(root, '.council') }), runner, persistence()).run(task({ permission: 'write' }));
  assert.equal(result.status, 'completed', result.error); assert.equal(result.workspaces.length, 1);
  assert.equal(new Set(runner.calls.map(call => call.cwd)).size, 1); assert.notEqual(runner.calls[0].cwd, root);
  assert.match(readFileSync(result.workspaces[0].patchPath, 'utf8'), /final/);
  assert.equal(readFileSync(join(root, 'base.txt'), 'utf8'), 'original\n');
});
