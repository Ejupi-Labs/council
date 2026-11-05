import test from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { WorkflowEngine } from '../dist/engine.js';

const config = { version: 1, backend: 'mock', workspaceRoot: process.cwd(), stateDir: join(process.cwd(), '.council'),
  maxConcurrency: 1, timeoutMs: 15000, maxCalls: 3, maxOutputBytes: 131072,
  agents: { worker: { enabled: true, profile: 'codex' }, advisor: { enabled: true, profile: 'claude' } } };
const input = { mode: 'delegate', task: 'Finish the assigned task and provide verification.', agents: ['worker'], permission: 'read',
  advisors: { worker: { agent: 'advisor' } } };

async function run(output, reply = 'Independent advisory evidence.') {
  const calls = [];
  const runner = { async run(request) {
    calls.push(request);
    return { agent: request.agent, status: 'completed', durationMs: 1,
      output: calls.length === 1 ? output : request.agent === 'advisor' ? reply : 'Completed and verified the assigned task.' };
  } };
  const result = await new WorkflowEngine(config, runner, { saveRun() {}, appendEvent() {} }).run(input);
  return { result, calls };
}

test('cut-off or Markdown-wrapped advisor requests cannot become successful task results or dispatch another call', async () => {
  for (const output of ['{"council_advisor_request":{"question":"Still need help"',
    '{"council\\u005fadvisor_request":{"question":"Still need help"}',
    '{"council_advisor_request":{"question":"Still need help"}} trailing',
    '```json\n{"council_advisor_request":{"question":"Still need help"}}\n```',
    '```\n{"council_advisor_request":{"question":"Still need help"']) {
    const { result, calls } = await run(output);
    assert.equal(result.status, 'failed', output);
    assert.match(result.error, /complete request envelope.*incomplete/);
    assert.equal(calls.length, 1);
    assert.equal(result.steps[0].result.output, output, 'Complete raw worker evidence remains available.');
  }
});

test('ambiguous duplicate advisor keys are rejected before advice, including escaped equivalents', async () => {
  for (const output of ['{"council_advisor_request":{"question":"First","question":"Second"}}',
    '{"council_advisor_request":{"question":"First","questi\\u006fn":"Second"}}',
    '{"council_advisor_request":{"question":"First"},"council\\u005fadvisor_request":{"question":"Second"}}']) {
    const { result, calls } = await run(output);
    assert.equal(result.status, 'failed');
    assert.match(result.error, /duplicate JSON object keys/);
    assert.equal(calls.length, 1);
    assert.equal(result.steps[0].result.output, output);
  }
});

test('quoted JSON examples in advisor evidence preserve their meaning and finish through normal resumption', async () => {
  const evidence = 'Check this literal example: {"question":"First","question":"Second"}; braces } and escaped quotes " are data.';
  const output = JSON.stringify({ council_advisor_request: { question: evidence, context: evidence } });
  const { result, calls } = await run(output);
  assert.equal(result.status, 'completed');
  assert.equal(calls.length, 3);
  assert.ok(calls[1].prompt.includes(evidence));
  for (const ordinary of ['Example:\n' + output, JSON.stringify({ council_advisor_request: { question: 'Illustration' }, explanation: 'Ordinary task evidence' })]) {
    const outcome = await run(ordinary);
    assert.equal(outcome.result.status, 'completed');
    assert.equal(outcome.result.output, ordinary);
    assert.equal(outcome.calls.length, 1);
  }
});

test('an incomplete advisor reply never resumes the worker or completes its task', async () => {
  const { result, calls } = await run(JSON.stringify({ council_advisor_request: { question: 'Check the boundary.' } }),
    '{"council_advisor_request":{"question":"Recursively incomplete"');
  assert.equal(result.status, 'failed');
  assert.equal(calls.length, 2);
  assert.match(result.error, /complete request envelope/);
});
