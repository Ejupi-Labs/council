import test from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { WorkflowEngine } from '../dist/engine.js';
import { privateChatCandidate, privateChatRequest } from '../dist/private-chat-protocol.js';

const config = { version: 1, backend: 'mock', workspaceRoot: process.cwd(), stateDir: join(process.cwd(), '.council'),
  maxConcurrency: 1, timeoutMs: 1000, maxCalls: 5, maxOutputBytes: 1048576,
  agents: Object.fromEntries(['worker', 'advisor', 'peer'].map(alias => [alias, { enabled: true, profile: 'codex' }])) };
const input = { mode: 'delegate', task: 'Complete and verify the explicit offline task.', agents: ['worker'], permission: 'read',
  advisors: { worker: { agent: 'advisor' } }, privateChats: { worker: { peers: ['peer'] } } };
const deep = '['.repeat(100) + '0' + ']'.repeat(100);
const wide = '[' + '0,'.repeat(69999) + '0]';

async function observe(output, operation) {
  const original = JSON.parse;
  let rawParses = 0;
  JSON.parse = function (value, ...args) { if (value === output) rawParses++; return original.call(this, value, ...args); };
  try { return { value: await operation(), rawParses }; }
  finally { JSON.parse = original; }
}

async function execute(output) {
  const events = [], calls = [];
  const engine = new WorkflowEngine(config, { async run(request) { calls.push(request); return { agent: request.agent, status: 'completed', output, durationMs: 1 }; } },
    { saveRun() {}, appendEvent(id, type, data) { events.push({ id, type, data }); } });
  const run = await engine.run(input);
  return { run, events, calls };
}

test('intended advisor JSON graphs are bounded before parsing while full rejected evidence remains retained', async () => {
  for (const graph of [deep, wide]) {
    const output = '{"council_advisor_request":{"question":"Verify this boundary.","context":' + graph + '}}';
    const { value: { run, calls }, rawParses } = await observe(output, () => execute(output));
    assert.equal(rawParses, 0); assert.equal(run.status, 'failed'); assert.equal(calls.length, 1);
    assert.match(run.error, /JSON depth 64|65536 structural tokens/);
    assert.equal(run.steps[0].result.output, output);
  }
});

test('private inner graphs fail before parsing and cannot leak their body through public workflow evidence', async () => {
  for (const graph of [deep, wide]) {
    for (const output of ['{"council_private_chat_request":{"peer":"peer","message":"PAIR_ONLY_QUESTION","context":' + graph + '}}',
      '{"ordinary":' + graph + ',"council_private_chat_request":{"peer":"peer","message":"PAIR_ONLY_QUESTION"}}']) {
      const { value: { run, events, calls }, rawParses } = await observe(output, () => execute(output));
      assert.equal(rawParses, 0); assert.equal(run.status, 'failed'); assert.equal(calls.length, 1);
      assert.match(run.error, /Private chat request must contain only valid peer, message and context fields/);
      assert.equal(JSON.stringify({ run, events }).includes('PAIR_ONLY_QUESTION'), false);
    }
  }
});

test('ordinary deep task JSON stays complete and is never materialized by orchestration parsers', async () => {
  for (const output of ['{"document":' + deep + '}', '{"document":' + wide + '}',
    'Example: {"council_private_chat_request":{"peer":"peer","message":"Literal example"}}']) {
    const { value: { run, calls }, rawParses } = await observe(output, () => execute(output));
    assert.equal(rawParses, 0); assert.equal(run.status, 'completed'); assert.equal(run.output, output); assert.equal(calls.length, 1);
  }
});

test('private candidate discovery stays lexical and valid bounded envelopes are parsed exactly once', async () => {
  const ordinary = '{"document":' + deep + '}';
  const plain = await observe(ordinary, () => [privateChatCandidate(ordinary), privateChatRequest(ordinary)]);
  assert.deepEqual(plain.value, [false, undefined]); assert.equal(plain.rawParses, 0);
  const output = JSON.stringify({ council_private_chat_request: { peer: 'peer', message: 'Known pair question.', context: 'Complete bounded context.' } });
  const valid = await observe(output, () => privateChatRequest(output));
  assert.equal(valid.rawParses, 1); assert.deepEqual(valid.value, { peer: 'peer', message: 'Known pair question.', context: 'Complete bounded context.' });
});
