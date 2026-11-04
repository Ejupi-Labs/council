import test from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { WorkflowEngine } from '../dist/engine.js';

const config = (overrides = {}) => ({ version: 1, backend: 'mock', workspaceRoot: process.cwd(), stateDir: join(process.cwd(), '.council'),
  maxConcurrency: 2, maxCalls: 20, timeoutMs: 15000, maxOutputBytes: 131072,
  agents: Object.fromEntries(['a', 'b', 'c', 'advisor', 'judge'].map(agent => [agent, { enabled: true, profile: 'codex' }])), ...overrides });
const store = () => ({ saveRun() {}, appendEvent() {} });
const input = overrides => ({ mode: 'pipeline', task: 'Use all prior evidence and verify the final conclusion.', agents: ['a', 'b'], permission: 'read', ...overrides });
const role = call => /You are (\S+)/.exec(call.prompt)[1];
const request = agent => JSON.stringify({ council_advisor_request: { question: `Assess the ${agent} evidence.`, context: `WORKER_REQUEST_${agent}_EVIDENCE` } });
const occurrences = (text, marker) => text.split(marker).length - 1;
function backend(handler) {
  const calls = [];
  return { calls, doctor: async () => [], run: async call => {
    calls.push(call); return { agent: call.agent, status: 'completed', durationMs: 1, output: await handler(call, calls.length) };
  } };
}

test('every debate round and its judge receive all earlier rounds without duplicate transcripts', async () => {
  for (const rounds of [2, 3]) {
    const facts = [];
    const runner = backend(call => {
      const current = role(call);
      if (current === 'judge') {
        for (const fact of facts) assert.equal(occurrences(call.prompt, fact), 1, fact);
        return 'Final synthesis, unchanged by evidence retention.';
      }
      const round = Number(current.slice(current.lastIndexOf('-') + 1));
      for (let earlier = 1; earlier < round; earlier++) {
        for (const agent of ['a', 'b']) assert.equal(occurrences(call.prompt, `ROUND_${earlier}_${agent}_FACT`), 1);
      }
      if (round === 1) assert.ok(!call.prompt.includes('ROUND_1_'), 'Initial peers must remain independent.');
      const fact = `ROUND_${round}_${call.agent}_FACT`; facts.push(fact); return fact;
    });
    const result = await new WorkflowEngine(config(), runner, store()).run(input({ mode: 'debate', judge: 'judge', rounds }));
    assert.equal(result.status, 'completed', result.error);
    assert.equal(result.output, 'Final synthesis, unchanged by evidence retention.');
    assert.equal(runner.calls.length, rounds * 2 + 1);
  }
});

test('parallel advisors preserve full worker lineage for the judge while concurrent peers remain independent', async () => {
  const workerCalls = new Map();
  const runner = backend(call => {
    const current = role(call);
    if (call.agent === 'judge') {
      for (const agent of ['a', 'b']) {
        assert.equal(occurrences(call.prompt, `WORKER_REQUEST_${agent}_EVIDENCE`), 1);
        assert.equal(occurrences(call.prompt, `ADVISOR_${agent}_FACT`), 1);
      }
      return 'Selected parallel synthesis.';
    }
    if (call.agent === 'advisor') {
      const worker = current.slice('advisor-for-'.length), other = worker === 'a' ? 'b' : 'a';
      assert.ok(call.prompt.includes(`WORKER_REQUEST_${worker}_EVIDENCE`));
      assert.ok(!call.prompt.includes(`WORKER_REQUEST_${other}_EVIDENCE`));
      assert.ok(!call.prompt.includes(`ADVISOR_${other}_FACT`));
      return `ADVISOR_${worker}_FACT`;
    }
    const count = (workerCalls.get(call.agent) ?? 0) + 1; workerCalls.set(call.agent, count);
    const other = call.agent === 'a' ? 'b' : 'a';
    assert.ok(!call.prompt.includes(`WORKER_REQUEST_${other}_EVIDENCE`));
    assert.ok(!call.prompt.includes(`ADVISOR_${other}_FACT`));
    if (count === 1) return request(call.agent);
    assert.ok(call.prompt.includes(`ADVISOR_${call.agent}_FACT`));
    return `Final ${call.agent} answer without repeating its intermediate evidence.`;
  });
  const result = await new WorkflowEngine(config({ maxConcurrency: 1 }), runner, store()).run(input({ mode: 'parallel', judge: 'judge',
    advisors: { a: { agent: 'advisor' }, b: { agent: 'advisor' } } }));
  assert.equal(result.status, 'completed', result.error); assert.equal(runner.calls.length, 7);
  assert.equal(result.output, 'Selected parallel synthesis.');
});

test('pipeline handoffs retain transitive advisor evidence once even when later workers omit it', async () => {
  const workerCalls = new Map();
  const runner = backend(call => {
    if (call.agent === 'advisor') return `ADVISOR_${role(call).slice('advisor-for-'.length)}_FACT`;
    const count = (workerCalls.get(call.agent) ?? 0) + 1; workerCalls.set(call.agent, count);
    if (call.agent === 'c') {
      for (const agent of ['a', 'b']) {
        assert.equal(occurrences(call.prompt, `WORKER_REQUEST_${agent}_EVIDENCE`), 1);
        assert.equal(occurrences(call.prompt, `ADVISOR_${agent}_FACT`), 1);
        assert.equal(occurrences(call.prompt, `FINAL_${agent}_RESULT`), 1);
      }
      return 'Final pipeline result.';
    }
    if (call.agent === 'b') {
      assert.equal(occurrences(call.prompt, 'ADVISOR_a_FACT'), 1);
      assert.equal(occurrences(call.prompt, 'WORKER_REQUEST_a_EVIDENCE'), 1);
    }
    return count === 1 ? request(call.agent) : `FINAL_${call.agent}_RESULT`;
  });
  const result = await new WorkflowEngine(config(), runner, store()).run(input({ agents: ['a', 'b', 'c'], advisors: { a: { agent: 'advisor' }, b: { agent: 'advisor' } } }));
  assert.equal(result.status, 'completed', result.error); assert.equal(runner.calls.length, 7);
  assert.equal(result.output, 'Final pipeline result.');
});

test('reviewers and revisions retain advisor evidence without repeating shared author ancestry', async () => {
  const workerCalls = new Map();
  const runner = backend(call => {
    if (call.agent === 'advisor') return `ADVISOR_${role(call).slice('advisor-for-'.length)}_FACT`;
    const count = (workerCalls.get(call.agent) ?? 0) + 1; workerCalls.set(call.agent, count);
    if (role(call) === 'revision') {
      for (const agent of ['a', 'b']) {
        assert.equal(occurrences(call.prompt, `WORKER_REQUEST_${agent}_EVIDENCE`), 1);
        assert.equal(occurrences(call.prompt, `ADVISOR_${agent}_FACT`), 1);
      }
      return 'Selected author revision.';
    }
    if (call.agent === 'b') assert.equal(occurrences(call.prompt, 'ADVISOR_a_FACT'), 1);
    return count === 1 ? request(call.agent) : `FINAL_${call.agent}_RESULT`;
  });
  const result = await new WorkflowEngine(config(), runner, store()).run(input({ mode: 'review', advisors: { a: { agent: 'advisor' }, b: { agent: 'advisor' } } }));
  assert.equal(result.status, 'completed', result.error); assert.equal(runner.calls.length, 7);
  assert.equal(result.output, 'Selected author revision.');
});
