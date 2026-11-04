import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { CouncilService } from '../dist/service.js';
import { loadConfig, parseWorkflow } from '../dist/config.js';
import { privateChatCandidate, privateChatRequest } from '../dist/private-chat-protocol.js';

const envelope = (peer = 'bravo', message = 'PRIVATE_QUESTION: Coordinate the shared parser boundary.', context = 'PRIVATE_CONTEXT: Complete evidence for the pair.') =>
  JSON.stringify({ council_private_chat_request: { peer, message, context } });
const answer = (agent, output, status = 'completed') => ({ agent, output, status, durationMs: 1 });
const input = (extra = {}) => ({ mode: 'delegate', task: 'Complete the assigned offline task and verify it.', agents: ['alpha'], permission: 'read',
  privateChats: { alpha: { peers: ['bravo'], maxRequests: 1 } }, ...extra });
function fixture(t, handler, overrides = {}) {
  const root = mkdtempSync(join(tmpdir(), 'council-private-protocol-')), calls = [];
  const agents = Object.fromEntries(['alpha', 'bravo', 'charlie', 'delta', 'echo'].map(alias => [alias, { enabled: true, profile: 'codex', model: 'offline-' + alias, effort: 'low' }]));
  const service = new CouncilService(loadConfig(root, { backend: 'mock', maxConcurrency: 1, maxCalls: 20, maxOutputBytes: 1048576, agents, ...overrides }), {
    doctor: async () => [], run: async request => { calls.push(request); return await handler(request, calls); },
  });
  t.after(async () => {
    await service.close(); assert.equal(dirname(resolve(root)), resolve(tmpdir())); assert.ok(basename(root).startsWith('council-private-protocol-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { service, calls };
}
function pairHistory(service, runId, alias = 'alpha') {
  const actor = `run:${runId}:${alias}`, chats = service.privateChats.list(actor), bodies = [];
  for (const chat of chats) {
    let after = 0;
    for (;;) {
      const page = service.privateChats.page({ chatId: chat.id, actor, after, limit: 100 });
      bodies.push(...page.messages.map(message => message.body)); if (page.nextCursor === null) break; after = page.nextCursor;
    }
  }
  return { chats, bodies };
}
function assertNoRawPrivateEvidence(service, run) {
  const publicEvidence = JSON.stringify({ run, events: service.store.events(run.id) });
  for (const marker of ['PRIVATE_QUESTION', 'PRIVATE_CONTEXT', 'PRIVATE_REPLY']) assert.equal(publicEvidence.includes(marker), false, marker);
}

test('private yield parser accepts only exact complete unambiguous envelopes', () => {
  assert.deepEqual(privateChatRequest(envelope()), { peer: 'bravo', message: 'PRIVATE_QUESTION: Coordinate the shared parser boundary.', context: 'PRIVATE_CONTEXT: Complete evidence for the pair.' });
  assert.equal(privateChatRequest('Example: ' + envelope()), undefined);
  assert.equal(privateChatCandidate('Example: ' + envelope()), false);
  assert.equal(privateChatRequest(JSON.stringify({ council_private_chat_request: { peer: 'bravo', message: 'Question' }, other: true })), undefined);
  assert.equal(privateChatCandidate('{"council_private_chat_request":{"message":"cut off'), true);
  assert.equal(privateChatCandidate('{"council\\u005fprivate_chat_request":{"message":"cut off'), true);
  for (const value of [
    '{"council_private_chat_request":{"peer":"bravo","peer":"charlie","message":"Question"}}',
    '{"council_private_chat_request":{"peer":"bravo","message":"Question","messa\\u0067e":"Other question"}}',
    '{"council_private_chat_request":{"peer":"bravo","message":"Question"},"council_private_chat_request":{"peer":"charlie","message":"Other"}}',
    JSON.stringify({ council_private_chat_request: { peer: ' ', message: 'Question' } }),
    JSON.stringify({ council_private_chat_request: { peer: 'bravo', message: '\t' } }),
    JSON.stringify({ council_private_chat_request: { peer: 'bravo', message: 'Question', extra: true } }),
  ]) assert.throws(() => privateChatRequest(value));
});

test('private planning reserves reply and resumption calls and validates every explicit peer', t => {
  const { service, calls } = fixture(t, () => assert.fail('Planning must not invoke models.'));
  assert.equal(service.engine.plan(input()).estimatedCalls, 3);
  const parsed = parseWorkflow(input({ privateChats: { alpha: { peers: ['bravo', 'charlie'] } } }));
  assert.equal(parsed.privateChats.alpha.maxRequests, 1); assert.equal(service.engine.plan(parsed).estimatedCalls, 3);
  assert.equal(service.engine.plan(input({ privateChats: { alpha: { peers: ['bravo'], maxRequests: 3 } }, maxCalls: 7 })).estimatedCalls, 7);
  assert.throws(() => service.engine.plan(input({ maxCalls: 2 })), /requires 3 calls/);
  for (const privateChats of [{ other: { peers: ['bravo'] } }, { alpha: { peers: ['alpha'] } }, { alpha: { peers: ['missing'] } },
    { alpha: { peers: [] } }, { alpha: { peers: ['bravo', 'bravo'] } }, { alpha: { peers: ['bravo'], maxRequests: 0 } },
    { alpha: { peers: ['bravo'], maxRequests: Number.MAX_SAFE_INTEGER } }]) assert.throws(() => service.engine.plan(input({ privateChats })));
  assert.equal(calls.length, 0);
});

test('five workers privately coordinate an overlap without exposing raw pair turns to the other workers or judge', { timeout: 15000 }, async t => {
  const { service, calls } = fixture(t, request => {
    if (request.prompt.includes('You are private-peer-for-alpha')) {
      assert.equal(request.agent, 'bravo'); assert.equal(request.permission, 'read');
      assert.match(request.prompt, /PRIVATE_QUESTION/); assert.match(request.prompt, /PRIVATE_CONTEXT/);
      assert.equal(request.prompt.includes('Council private coordination protocol'), false);
      return answer(request.agent, 'PRIVATE_REPLY: I will keep the other side of that parser boundary stable.');
    }
    if (request.agent === 'alpha' && !request.prompt.includes('PRIVATE_REPLY')) return answer(request.agent, envelope());
    if (request.agent === 'alpha') assert.match(request.prompt, /PRIVATE_REPLY/);
    if (!['alpha', 'bravo'].includes(request.agent)) {
      for (const marker of ['PRIVATE_QUESTION', 'PRIVATE_CONTEXT', 'PRIVATE_REPLY']) assert.equal(request.prompt.includes(marker), false, request.agent + ': ' + marker);
    }
    return answer(request.agent, 'Public verification complete for ' + request.agent + '.');
  });
  const run = await service.run(input({ mode: 'parallel', agents: ['alpha', 'bravo', 'charlie', 'delta', 'echo'], judge: 'echo' }));
  assert.equal(run.status, 'completed', run.error); assert.equal(calls.length, 8); assert.equal(run.steps.length, 8);
  assertNoRawPrivateEvidence(service, run);
  const { chats, bodies } = pairHistory(service, run.id);
  assert.equal(chats.length, 1); assert.equal(chats[0].messageCount, 2); assert.equal(bodies.length, 2);
  assert.match(bodies[0], /PRIVATE_QUESTION/); assert.match(bodies[0], /PRIVATE_CONTEXT/); assert.match(bodies[1], /PRIVATE_REPLY/);
  assert.deepEqual(service.privateChats.list(`run:${run.id}:charlie`), []);
});

test('private peers retain admitted exact settings and empty skills after later profile edits', async t => {
  let workerCalls = 0;
  const { service, calls } = fixture(t, request => answer(request.agent, request.agent === 'alpha' && ++workerCalls === 1 ? envelope() : request.agent === 'bravo' ? 'PRIVATE_REPLY: Scoped answer.' : 'Public result.'));
  const queued = service.start(input({ agentSettings: { bravo: { model: 'exact-peer', effort: 'medium', skills: [] } } }));
  for (const profile of Object.values(service.config.agents)) { profile.model = 'later-model'; profile.effort = 'high'; profile.skills = ['council-advisor']; }
  const result = await service.completion(queued.id); assert.equal(result.status, 'completed', result.error);
  const peer = calls.find(request => request.agent === 'bravo');
  assert.equal(peer.model, 'exact-peer'); assert.equal(peer.effort, 'medium'); assert.equal(peer.resolvedSettings, true);
  assert.equal(peer.prompt.includes('Selected collaboration skills'), false);
  assert.deepEqual(queued.input.agentSettings.bravo, { model: 'exact-peer', effort: 'medium', skills: [] });
  assert.ok(calls.filter(request => request.agent === 'alpha').every(request => request.model === 'offline-alpha' && request.effort === 'low'));
  assertNoRawPrivateEvidence(service, result);
});

test('private request quotas prevent a repeated yield from becoming successful final text', async t => {
  const { service, calls } = fixture(t, request => answer(request.agent, request.agent === 'alpha' ? envelope() : 'PRIVATE_REPLY: Scoped answer.'));
  const result = await service.run(input({ privateChats: { alpha: { peers: ['bravo'], maxRequests: 2 } } }));
  assert.equal(result.status, 'failed'); assert.match(result.error, /private-chat request limit/); assert.equal(calls.length, 5);
  assert.equal(pairHistory(service, result.id).bodies.length, 4); assertNoRawPrivateEvidence(service, result);
});

test('unconfigured and unpermitted private requests fail without public raw evidence or extra calls', async t => {
  for (const policy of [undefined, { alpha: { peers: ['charlie'] } }]) {
    const { service, calls } = fixture(t, request => answer(request.agent, envelope()));
    const result = await service.run(input({ privateChats: policy }));
    assert.equal(result.status, 'failed'); assert.match(result.error, /not explicitly permitted/); assert.equal(calls.length, 1);
    assertNoRawPrivateEvidence(service, result); assert.equal(pairHistory(service, result.id).chats.length, 0);
  }
});

test('malformed or oversized private candidate output cannot leak or become successful task output', async t => {
  for (const output of ['{"council_private_chat_request":{"message":"PRIVATE_QUESTION cut off',
    '{"council_private_chat_request":{"peer":"bravo","message":"PRIVATE_QUESTION"},"other":true}',
    '{"council_private_chat_request":{"peer":"bravo","peer":"charlie","message":"PRIVATE_QUESTION"}}',
    JSON.stringify({ council_private_chat_request: { peer: 'bravo', message: 'PRIVATE_QUESTION', PRIVATE_CONTEXT: 'Private evidence in an unsupported field.' } }),
    envelope('bravo', 'PRIVATE_QUESTION', 'PRIVATE_CONTEXT ' + 'x'.repeat(2000))]) {
    const { service, calls } = fixture(t, request => answer(request.agent, output), { maxOutputBytes: 1024 });
    const result = await service.run(input()); assert.equal(result.status, 'failed'); assert.equal(calls.length, 1); assertNoRawPrivateEvidence(service, result);
  }
});

test('private peer turns reject recursive advisor or private requests and retain them only in the pair chat', async t => {
  for (const output of [envelope('alpha'), JSON.stringify({ council_advisor_request: { question: 'PRIVATE_QUESTION recursive advice?', context: 'PRIVATE_CONTEXT' } })]) {
    const { service, calls } = fixture(t, request => answer(request.agent, request.agent === 'alpha' ? envelope() : output));
    const result = await service.run(input()); assert.equal(result.status, 'failed'); assert.match(result.error, /recursion/); assert.equal(calls.length, 2);
    assertNoRawPrivateEvidence(service, result); assert.equal(pairHistory(service, result.id).bodies.length, 2);
  }
});

test('an advisor cannot secretly enter the private-chat protocol', async t => {
  const { service, calls } = fixture(t, request => answer(request.agent, request.agent === 'alpha'
    ? JSON.stringify({ council_advisor_request: { question: 'Check the ordinary invariant.' } }) : envelope('charlie')));
  const result = await service.run(input({ advisors: { alpha: { agent: 'bravo' } } }));
  assert.equal(result.status, 'failed'); assert.match(result.error, /recursion.*advisor/); assert.equal(calls.length, 2);
  assertNoRawPrivateEvidence(service, result); assert.equal(pairHistory(service, result.id).bodies.length, 0);
});

test('private pair context is not automatically injected into a later public advisor turn using the same peer alias', async t => {
  let workerCalls = 0;
  const { service, calls } = fixture(t, request => {
    if (request.prompt.includes('You are private-peer-for-alpha')) return answer(request.agent, 'PRIVATE_REPLY: The private parser boundary is stable.');
    if (request.prompt.includes('You are advisor-for-alpha') || request.agent === 'charlie') {
      for (const marker of ['PRIVATE_QUESTION', 'PRIVATE_CONTEXT', 'PRIVATE_REPLY']) assert.equal(request.prompt.includes(marker), false);
      return answer(request.agent, 'Public independent review: preserve the ordinary invariant.');
    }
    if (++workerCalls === 1) return answer(request.agent, envelope());
    if (workerCalls === 2) return answer(request.agent, JSON.stringify({ council_advisor_request: { question: 'Check the ordinary public invariant.', context: 'Public evidence only.' } }));
    return answer(request.agent, 'Public draft or revision verified.');
  });
  const result = await service.run(input({ mode: 'review', agents: ['alpha', 'charlie'], advisors: { alpha: { agent: 'bravo' } } }));
  assert.equal(result.status, 'completed', result.error); assert.equal(calls.length, 7); assert.equal(workerCalls, 4);
  assertNoRawPrivateEvidence(service, result); assert.equal(pairHistory(service, result.id).bodies.length, 2);
});

test('cancellation during a private reply drains the peer without resuming the sender or publishing partial private text', { timeout: 15000 }, async t => {
  let entered;
  const peerEntered = new Promise(done => { entered = done; });
  const { service, calls } = fixture(t, request => {
    if (request.agent === 'alpha') return answer(request.agent, envelope());
    entered(); return new Promise(done => {
      const finish = () => done(answer(request.agent, 'PRIVATE_REPLY: Partial interrupted answer.', 'cancelled'));
      if (request.signal.aborted) finish(); else request.signal.addEventListener('abort', finish, { once: true });
    });
  });
  const queued = service.start(input()); await peerEntered; service.requestCancellation(queued.id);
  const result = await service.completion(queued.id); assert.equal(result.status, 'cancelled'); assert.equal(calls.length, 2);
  assertNoRawPrivateEvidence(service, result); assert.match(pairHistory(service, result.id).bodies[1], /PRIVATE_REPLY/);
});

test('private evidence stays complete beyond previous context caps', async t => {
  const context = 'PRIVATE_CONTEXT complete evidence. '.repeat(3000) + 'PRIVATE_CONTEXT_END';
  const reply = 'PRIVATE_REPLY complete reply. '.repeat(3000) + 'PRIVATE_REPLY_END';
  let workerCalls = 0;
  const { service } = fixture(t, request => {
    if (request.agent === 'bravo') { assert.ok(request.prompt.includes(context)); return answer(request.agent, reply); }
    if (++workerCalls === 1) return answer(request.agent, envelope('bravo', 'PRIVATE_QUESTION: Inspect all evidence.', context));
    assert.ok(request.prompt.includes(context)); assert.ok(request.prompt.includes(reply)); return answer(request.agent, 'Public verification complete.');
  });
  const result = await service.run(input({ contextLimit: null })); assert.equal(result.status, 'completed', result.error);
  const history = pairHistory(service, result.id).bodies; assert.ok(history[0].includes(context)); assert.ok(history[1].includes(reply));
  assertNoRawPrivateEvidence(service, result);
});

test('an explicit context budget blocks private reply dispatch without dropping evidence', async t => {
  const { service, calls } = fixture(t, request => answer(request.agent, envelope('bravo', 'PRIVATE_QUESTION', 'PRIVATE_CONTEXT ' + 'x'.repeat(5000))));
  const result = await service.run(input({ contextLimit: 1000 }));
  assert.equal(result.status, 'failed'); assert.match(result.error, /contextLimit/); assert.match(result.error, /Increase the budget/); assert.equal(calls.length, 1);
  assert.equal(pairHistory(service, result.id).bodies.length, 1); assertNoRawPrivateEvidence(service, result);
});

test('a peer that ignores a quota abort cannot persist a successful private reply', { timeout: 15000 }, async t => {
  let blockPeer = false;
  const { service, calls } = fixture(t, request => {
    if (request.agent === 'alpha') return answer(request.agent, envelope());
    blockPeer = true;
    return new Promise(done => request.signal.addEventListener('abort', () => done(answer(request.agent, 'PRIVATE_REPLY: The provider ignored cancellation and claimed success.')), { once: true }));
  });
  service.engine.options.assertAgentDispatch = alias => { if (alias === 'bravo' && blockPeer) throw new Error('The peer account is currently exhausted.'); };
  const result = await service.run(input()); assert.equal(result.status, 'failed'); assert.equal(calls.length, 2);
  const reply = JSON.parse(pairHistory(service, result.id).bodies[1]); assert.equal(reply.status, 'failed'); assert.match(reply.error, /exhausted/);
  assertNoRawPrivateEvidence(service, result);
});

test('rooms opt into private coordination only for task execution and preserve global room history isolation', async t => {
  let taskCalls = 0;
  const { service } = fixture(t, request => {
    const match = /ROOM_BALLOT_DATA_BEGIN\n([^\n]+)\nROOM_BALLOT_DATA_END/.exec(request.prompt);
    if (match) {
      assert.equal(request.prompt.includes('Council private coordination protocol'), false);
      return answer(request.agent, JSON.stringify({ vote: 'approve', rationale: 'Clear offline task ownership.', assignments: Object.fromEntries(JSON.parse(match[1]).tasks.map(task => [task.id, 'alpha'])) }));
    }
    if (request.agent === 'bravo') return answer(request.agent, 'PRIVATE_REPLY: Keep the shared parser stable.');
    return answer(request.agent, ++taskCalls === 1 ? envelope() : 'Public verification complete.');
  });
  const room = service.rooms.create({ title: 'Scoped private coordination', brief: 'Two overlapping tasks may coordinate in a private pair thread.',
    members: [{ agent: 'alpha', role: 'lead' }, { agent: 'bravo', role: 'worker', model: 'exact-room-peer', effort: 'medium' }],
    privateChatMaxRequests: 1, maxCalls: 5, tasks: [{ title: 'Inspect parser', brief: 'Inspect the shared parser and verify your own work.', permission: 'read' }] });
  service.rooms.startDiscussion(room.id); const completed = await service.rooms.wait(room.id);
  assert.equal(completed.status, 'completed', completed.error); assert.equal(completed.callsUsed, 5);
  const run = service.getRun(completed.tasks[0].runId); assert.equal(run.input.privateChats.alpha.maxRequests, 1);
  assert.equal(run.input.agentSettings.bravo.model, 'exact-room-peer'); assert.equal(run.input.agentSettings.bravo.effort, 'medium');
  for (const marker of ['PRIVATE_QUESTION', 'PRIVATE_CONTEXT', 'PRIVATE_REPLY']) assert.equal(JSON.stringify(completed.messages).includes(marker), false);
  assertNoRawPrivateEvidence(service, run); const privateHistory = pairHistory(service, run.id);
  assert.equal(privateHistory.bodies.length, 2); assert.equal(privateHistory.chats[0].roomId, room.id);
  assert.equal(service.privateChats.list(`run:${run.id}:alpha`, room.id)[0].id, privateHistory.chats[0].id);
});

test('private control candidates redact fenced responses and cut-off later keys without consuming ordinary examples', () => {
  const exact = envelope();
  for (const output of [
    '```json\n' + exact + '\n```',
    '```\n' + exact + '\n```',
    '  ```JSON\r\n' + exact + '\r\n```',
    '```json\n{"other":true,"council_private_chat_request":{"message":"PRIVATE_QUESTION cut off',
    '{"other":{"nested":[1,2,{"value":"not a key"}]},"council_private_chat_request":{"message":"PRIVATE_QUESTION cut off',
    '{"other":"quoted \\"council_private_chat_request\\" is data","council\\u005fprivate_chat_request":{"message":"PRIVATE_QUESTION cut off',
  ]) {
    assert.equal(privateChatCandidate(output), true, output);
    assert.equal(privateChatRequest(output), undefined);
  }
  for (const output of [
    'Example: ' + exact,
    'This is an example.\n```json\n' + exact + '\n```',
    '{"example":{"council_private_chat_request":{"message":"Ordinary example"}}}',
    '{"example":{"council_private_chat_request":{"message":"Ordinary cut-off example',
    '{"description":"council_private_chat_request: ordinary prose"}',
    '{"other":[{"council_private_chat_request":"not at the root"}]}',
  ]) assert.equal(privateChatCandidate(output), false, output);
});

test('mistaken Markdown or a cut-off later private key cannot complete or publish pair evidence', async t => {
  for (const output of [
    '```json\n' + envelope() + '\n```',
    '{"other":true,"council_private_chat_request":{"peer":"bravo","message":"PRIVATE_QUESTION cut off',
  ]) {
    const { service, calls } = fixture(t, request => answer(request.agent, output));
    const run = await service.run({ mode: 'delegate', task: 'Complete the assigned investigation.', agents: ['alpha'], permission: 'read',
      privateChats: { alpha: { peers: ['bravo'], maxRequests: 1 } } });
    assert.equal(run.status, 'failed');
    assert.equal(calls.length, 1);
    assert.doesNotMatch(JSON.stringify(run), /PRIVATE_QUESTION|PRIVATE_CONTEXT/);
  }
});
