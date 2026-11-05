import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { AcpProtocol } from '../dist/acp.js';
import { NativeRunner } from '../dist/runner.js';
import { loadConfig } from '../dist/config.js';

const denied = 'ACP tool permission was denied by read mode; the task is incomplete.';
const kinds = ['read', 'edit', 'delete', 'move', 'search', 'execute', 'think', 'fetch', 'switch_mode', 'other'];
function session(permission = 'read') {
  const sent = [], failures = [], output = [], completed = [];
  const control = { send: value => sent.push(value), fail: value => failures.push(value), append: value => output.push(value),
    replace: value => output.splice(0, output.length, value), complete: value => completed.push(value), endInput() {} };
  const protocol = new AcpProtocol({ agent: 'fixture', cwd: process.cwd(), prompt: 'Offline protocol fixture.', permission, timeoutMs: 1000 });
  protocol.start(control);
  protocol.event({ jsonrpc: '2.0', id: 'council-initialize', result: { protocolVersion: 1 } }, control);
  protocol.event({ jsonrpc: '2.0', id: 'council-new', result: { sessionId: 'fixture-session' } }, control);
  return { protocol, control, sent, failures, output, completed };
}
function permissionRequest(toolCall) {
  return { jsonrpc: '2.0', id: 'permission-request', method: 'session/request_permission', params: {
    sessionId: 'fixture-session', toolCall, options: [{ optionId: 'allow-fixture', kind: 'allow_once', name: 'Allow' },
      { optionId: 'deny-fixture', kind: 'reject_once', name: 'Reject' }],
  } };
}
function update(state, kind, text) {
  state.protocol.event({ jsonrpc: '2.0', method: 'session/update', params: { sessionId: 'fixture-session',
    update: { sessionUpdate: kind, content: { type: 'text', text } } } }, state.control);
}
function finish(state) { state.protocol.event({ jsonrpc: '2.0', id: 'council-prompt', result: { stopReason: 'end_turn' } }, state.control); }

test('read-mode permission failures preserve only the validated reported ACP kind and deny every kind', () => {
  for (const kind of kinds) {
    const state = session();
    state.protocol.event(permissionRequest({ kind, toolCallId: 'PRIVATE_TOOL_ID', title: 'SECRET_ACCESS_TOKEN=do-not-print',
      rawInput: { command: 'PRIVATE_COMMAND', path: 'C:\\PRIVATE_PATH', credential: 'PRIVATE_CREDENTIAL' },
      rawOutput: 'PRIVATE_TOOL_RESULT' }), state.control);
    assert.deepEqual(state.failures, [denied + ` Reported ACP tool kind: ${kind}.`]);
    assert.deepEqual(state.sent.at(-1), { jsonrpc: '2.0', id: 'permission-request', result: { outcome: { outcome: 'selected', optionId: 'deny-fixture' } } });
    assert.deepEqual(state.completed, []);
    assert.doesNotMatch(JSON.stringify(state.failures), /PRIVATE_|SECRET_ACCESS_TOKEN/);
  }
});

test('a failed read tool does not determine the category of a distinct permission request', () => {
  const state = session();
  for (const progress of [
    { sessionUpdate: 'tool_call', toolCallId: 'PRIVATE_READ_TOOL_ID', title: 'Read PRIVATE_PATH', kind: 'read', status: 'in_progress' },
    { sessionUpdate: 'tool_call_update', toolCallId: 'PRIVATE_READ_TOOL_ID', kind: 'read', status: 'failed' },
  ]) state.protocol.event({ jsonrpc: '2.0', method: 'session/update', params: { sessionId: 'fixture-session', update: progress } }, state.control);
  state.protocol.event(permissionRequest({ toolCallId: 'PRIVATE_PERMISSION_TOOL_ID', kind: 'other',
    title: 'Allow PRIVATE_FILE_ACCESS', status: 'pending' }), state.control);
  assert.deepEqual(state.failures, [denied + ' Reported ACP tool kind: other.']);
  assert.equal(state.sent.at(-1).result.outcome.optionId, 'deny-fixture');
  assert.deepEqual(state.completed, []);
  assert.deepEqual(state.output, []);
  assert.doesNotMatch(JSON.stringify(state.failures), /PRIVATE_|Allow|Read PRIVATE/);
});

test('unreported, malformed and future ACP kinds are not echoed or inferred from sensitive titles and inputs', () => {
  for (const kind of [undefined, null, '', 'READ', ' read', 'execute PRIVATE_COMMAND', 'C:\\PRIVATE_PATH', false, 7, {}, ['read']]) {
    const state = session();
    state.protocol.event(permissionRequest({ kind, title: 'Read PRIVATE_PATH', rawInput: { command: 'PRIVATE_COMMAND' } }), state.control);
    assert.deepEqual(state.failures, [denied]);
    assert.equal(state.sent.at(-1).result.outcome.optionId, 'deny-fixture');
    assert.deepEqual(state.completed, []);
  }
  const inherited = session();
  inherited.protocol.event(permissionRequest(Object.create({ kind: 'execute' })), inherited.control);
  assert.deepEqual(inherited.failures, [denied]);
});

test('human-interaction denial still fails in read mode and may report only its coarse category', () => {
  const state = session(), event = permissionRequest({ kind: 'other', title: 'AskUserQuestion PRIVATE_QUESTION', rawInput: { secret: 'PRIVATE_INPUT' } });
  state.protocol.event(event, state.control);
  assert.deepEqual(state.failures, ['ACP agent requires a human interaction that unattended Council cannot answer. Reported ACP tool kind: other.']);
  assert.equal(state.sent.at(-1).result.outcome.optionId, 'deny-fixture');
  assert.deepEqual(state.completed, []);
});

test('safe diagnostics do not change one-time write permission selection', () => {
  const state = session('write');
  state.protocol.event(permissionRequest({ kind: 'edit', title: 'Write a fixture file' }), state.control);
  assert.deepEqual(state.failures, []);
  assert.equal(state.sent.at(-1).result.outcome.optionId, 'allow-fixture');
});

test('ACP keeps reasoning, user, tool and plan updates outside the selected assistant response', () => {
  const state = session(), ballot = '{"vote":"approve","rationale":"Offline evidence.","assignments":{}}';
  for (const kind of ['agent_thought_chunk', 'user_message_chunk']) update(state, kind, 'NON_ANSWER_PROGRESS');
  for (const progress of [
    { sessionUpdate: 'tool_call', toolCallId: 'fixture-tool', title: 'NON_ANSWER_PROGRESS', kind: 'read', status: 'in_progress' },
    { sessionUpdate: 'tool_call_update', toolCallId: 'fixture-tool', status: 'completed' },
    { sessionUpdate: 'plan', entries: [{ content: 'NON_ANSWER_PROGRESS', priority: 'medium', status: 'completed' }] },
  ]) state.protocol.event({ jsonrpc: '2.0', method: 'session/update', params: { sessionId: 'fixture-session', update: progress } }, state.control);
  update(state, 'agent_message_chunk', ballot.slice(0, 15));
  update(state, 'agent_message_chunk', ballot.slice(15));
  finish(state);
  assert.equal(state.output.join(''), ballot); assert.deepEqual(JSON.parse(state.output.join('')), JSON.parse(ballot));
  assert.deepEqual(state.failures, []); assert.deepEqual(state.completed, ['end_turn']);
});

test('ACP preserves an assistant-channel preamble rather than guessing a final JSON answer', () => {
  const state = session(), preamble = 'I will inspect the request first.\n', ballot = '{"vote":"approve","rationale":"Offline evidence.","assignments":{}}';
  for (const [messageId, text] of [['progress-message', preamble], ['ballot-message', ballot]]) {
    state.protocol.event({ jsonrpc: '2.0', method: 'session/update', params: { sessionId: 'fixture-session',
      update: { sessionUpdate: 'agent_message_chunk', messageId, content: { type: 'text', text } } } }, state.control);
  }
  finish(state);
  assert.equal(state.output.join(''), preamble + ballot);
  assert.throws(() => JSON.parse(state.output.join('')));
  assert.deepEqual(state.completed, ['end_turn'], 'Protocol completion is distinct from satisfying the downstream ballot contract.');
});

test('native ACP supervision retains the safe kind while a read-only adapter remains failed', async t => {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-acp-diagnostics-'));
  t.after(() => { assert.equal(dirname(root), parent); assert.ok(basename(root).startsWith('council-acp-diagnostics-')); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  const adapter = fileURLToPath(new URL('./fixtures/fake-acp-adapter.mjs', import.meta.url));
  const config = loadConfig(root, { backend: 'native', agents: { fixture: { enabled: true, profile: 'junie', transport: 'acp', command: process.execPath, args: [adapter] } } });
  const runner = new NativeRunner(config, { environment: { ...process.env, COUNCIL_DEPTH: '0', COUNCIL_TEST_ACP_CASE: 'read-permission' } });
  const result = await runner.run({ agent: 'fixture', prompt: 'request-write-permission', cwd: root, permission: 'read', timeoutMs: 15_000 });
  assert.equal(result.status, 'failed'); assert.match(result.error, /permission was denied by read mode/);
  assert.match(result.error, /Reported ACP tool kind: read\./);
  assert.doesNotMatch(JSON.stringify(result), /Write a fixture file/);
});
