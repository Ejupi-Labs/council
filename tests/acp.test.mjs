import test from 'node:test';
import assert from 'node:assert/strict';
import { AcpProtocol, validAcpEnvelope } from '../dist/acp.js';

function session(result = {}, model) {
  const sent = [], failures = [], completed = [], output = [];
  const control = { send: value => sent.push(value), fail: error => failures.push(error), append: text => output.push(text),
    replace: text => { output.splice(0, output.length, text); }, complete: reason => completed.push(reason), endInput() {} };
  const protocol = new AcpProtocol({ agent: 'fixture', cwd: process.cwd(), prompt: 'Offline protocol fixture.', permission: 'read', timeoutMs: 1000 }, model);
  protocol.start(control);
  protocol.event({ jsonrpc: '2.0', id: 'council-initialize', result: { protocolVersion: 1 } }, control);
  protocol.event({ jsonrpc: '2.0', id: 'council-new', result: { sessionId: 'fixture-session', ...result } }, control);
  return { protocol, control, sent, failures, completed, output };
}

const modelSelector = (overrides = {}) => ({ id: 'model', type: 'select', currentValue: 'old-model', name: 'Model',
  options: [{ value: 'requested-model', name: 'Requested model' }], ...overrides });
const provider = { id: 'provider', type: 'select', category: 'model', currentValue: 'provider-a', name: 'Provider', options: [{ value: 'provider-a', name: 'Provider A' }] };

for (const [label, configOptions] of [
  ['missing optional category', [modelSelector()]],
  ['provider before model', [provider, modelSelector({ category: 'model' })]],
  ['provider before uncategorized model', [provider, modelSelector()]],
  ['unrecognized control before model', [modelSelector({ id: 'future', category: 'model', type: 'future-input' }), modelSelector()]],
]) test(`ACP selects the explicitly advertised model with ${label}`, () => {
  const state = session({ configOptions }, 'requested-model');
  assert.deepEqual(state.failures, []);
  const selection = state.sent.at(-1);
  assert.equal(selection.method, 'session/set_config_option');
  assert.equal(selection.params.configId, 'model');
  assert.equal(selection.params.value, 'requested-model');
  assert.equal(state.sent.some(message => message.method === 'session/prompt'), false);
  state.protocol.event({ jsonrpc: '2.0', id: 'council-model', result: {} }, state.control);
  assert.equal(state.sent.at(-1).method, 'session/prompt');
});

test('ACP custom model selectors support grouped advertised values and preserve legacy models', () => {
  const grouped = session({ configOptions: [modelSelector({ id: 'custom-model', category: 'model', options: [{ group: 'recommended', name: 'Recommended', options: [{ value: 'requested-model', name: 'Requested model' }] }] })] }, 'requested-model');
  assert.deepEqual(grouped.failures, []);
  assert.equal(grouped.sent.at(-1).params.configId, 'custom-model');
  const legacy = session({ models: { availableModels: [{ modelId: 'requested-model', name: 'Requested model' }] } }, 'requested-model');
  assert.equal(legacy.sent.at(-1).method, 'session/set_model');
  const denied = session({ configOptions: [modelSelector()] }, 'unadvertised-model');
  assert.match(denied.failures[0], /not advertised/);
  assert.equal(denied.sent.some(message => message.method === 'session/prompt'), false);
});

for (const error of [null, 'provider failure', 7, [], {}, { code: -32000, message: 'Provider failure.' }]) {
  test(`ACP refuses terminal success combined with error ${JSON.stringify(error)}`, () => {
    const state = session();
    state.protocol.event({ jsonrpc: '2.0', method: 'session/update', params: { sessionId: 'fixture-session', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'Partial evidence.' } } } }, state.control);
    state.protocol.event({ jsonrpc: '2.0', id: 'council-prompt', result: { stopReason: 'end_turn' }, error }, state.control);
    assert.deepEqual(state.completed, []);
    assert.match(state.failures[0], /invalid.*envelope/i);
    assert.deepEqual(state.output, ['Partial evidence.']);
  });
}

test('ACP envelopes reject invalid identities/methods and preserve valid extension notifications and errors', () => {
  for (const event of [
    { id: 'response', result: {}, method: null }, { id: true, result: {} }, { id: 1.5, result: {} },
    { id: Number.MAX_SAFE_INTEGER + 1, method: 'session/prompt' }, { result: {} }, { id: 'response' },
    { id: 'response', error: { code: 'bad', message: 'Failure' } },
  ]) assert.equal(validAcpEnvelope({ jsonrpc: '2.0', ...event }), false);
  for (const event of [
    { method: 'fixture/extension', params: {} }, { id: 1, method: 'fixture/request' },
    { id: 1, result: false }, { id: null, error: { code: -32700, message: 'Parse error' } },
  ]) assert.equal(validAcpEnvelope({ jsonrpc: '2.0', ...event }), true);
});
