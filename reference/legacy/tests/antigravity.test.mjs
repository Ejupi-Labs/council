import test from 'node:test';
import assert from 'node:assert/strict';
import { AntigravityProtocol } from '../dist/antigravity.js';

const conversation = 'offline-conversation';
const init = () => ({ event: 'init', conversation_id: conversation, init: { cwd: process.cwd(), tools: ['ask_permission', 'read_file'], permission_mode: 'proceed-in-sandbox' } });
const step = (overrides = {}) => ({ event: 'step_update', step_update: { conversation_id: conversation, step_index: 1, state: 'DONE', step_type: 'agent_response', text_delta: 'Answer\n', ...overrides } });
const result = (overrides = {}) => ({ event: 'result', result: { conversation_id: conversation, status: 'SUCCESS', response: 'Answer\n', ...overrides } });

function session(initialize = true, prompt = 'Offline protocol fixture.') {
  const sent = [], failures = [], completed = [], output = [], input = [];
  const control = { send: value => sent.push(value), fail: error => failures.push(error), append: text => output.push(text),
    replace: text => { output.splice(0, output.length, text); }, complete: reason => completed.push(reason), endInput: text => input.push(text) };
  const protocol = new AntigravityProtocol(prompt);
  protocol.start(control);
  if (initialize) protocol.event(init(), control);
  const event = value => protocol.event(value, control);
  return { protocol, control, event, sent, failures, completed, output, input };
}

test('Antigravity sends exactly one JSON user turn through stdin and closes input', () => {
  const prompt = 'Unicode 🌍 "quoted"\nsecond line';
  const state = session(false, prompt);
  assert.deepEqual(state.input, [JSON.stringify({ event: 'user', message: { content: prompt } }) + '\n']);
  assert.deepEqual(state.sent, []);
  assert.deepEqual(state.failures, []);
});

test('Antigravity assembles ACTIVE deltas and final DONE text without duplicating terminal output', () => {
  const state = session();
  state.event(step({ state: 'ACTIVE', text_delta: 'COUNCIL_LIVE_' }));
  state.event(step({ state: 'ACTIVE', text_delta: 'ANTIGRAVITY_OK 42' }));
  state.event(step({ text_delta: '\n' }));
  state.event(result({ response: 'COUNCIL_LIVE_ANTIGRAVITY_OK 42\n', duration_seconds: 4.59, num_turns: 1, usage: {} }));
  assert.deepEqual(state.failures, []);
  assert.deepEqual(state.completed, ['end_turn']);
  assert.equal(state.output.join(''), 'COUNCIL_LIVE_ANTIGRAVITY_OK 42\n');
});

test('Antigravity accepts DONE-only answers and keeps user, tool and checkpoint text out of the answer', () => {
  const state = session();
  state.event(step({ step_index: 0, step_type: 'user_input', text_delta: 'User text stays out.' }));
  state.event(step({ step_index: 2, step_type: 'tool', text_delta: 'Tool text stays out.', tool_name: 'read_file', tool_info: { name: 'read_file', output: 'tool output stays out' } }));
  state.event(step({ step_index: 3 }));
  state.event(step({ step_index: 4, step_type: 'checkpoint', text_delta: 'Checkpoint stays out.' }));
  state.event(result({ finished_reason: '' }));
  assert.deepEqual(state.failures, []);
  assert.deepEqual(state.output, ['Answer\n']);
  assert.deepEqual(state.completed, ['end_turn']);
});

test('Antigravity rejects missing, duplicate and mismatched conversation identities', () => {
  for (const event of [
    { ...init(), conversation_id: '' }, { ...init(), conversation_id: 7 },
    { ...init(), init: { conversation_id: 'other-session' } }, step(), result(),
  ]) {
    const state = session(false); state.event(event);
    assert.equal(state.failures.length, 1); assert.deepEqual(state.completed, []);
  }
  for (const event of [init(), step({ conversation_id: 'other' }), result({ conversation_id: 'other' }),
    { ...step(), conversation_id: 'other' }]) {
    const state = session(); state.event(event);
    assert.equal(state.failures.length, 1); assert.deepEqual(state.completed, []);
  }
});

test('Antigravity refuses malformed steps, changed step types and duplicate completed steps', () => {
  for (const override of [{ step_index: -1 }, { step_index: 1.5 }, { step_index: Number.MAX_SAFE_INTEGER + 1 },
    { state: 'WAITING' }, { state: null }, { step_type: null }, { step_type: '' }, { text_delta: null }, { tool_info: [] }]) {
    const state = session(); state.event(step(override));
    assert.equal(state.failures.length, 1); assert.deepEqual(state.output, []);
  }
  for (const second of [step(), step({ state: 'ACTIVE', text_delta: 'late' })]) {
    const state = session(); state.event(step()); state.event(second);
    assert.equal(state.failures.length, 1); assert.deepEqual(state.output, ['Answer\n']);
  }
  const changed = session();
  changed.event(step({ state: 'ACTIVE' })); changed.event(step({ step_type: 'tool' }));
  assert.equal(changed.failures.length, 1);
});

test('Antigravity retains body-free field shapes for invalid steps and distinguishes unsupported state strings', () => {
  const cases = [
    [{ step_index: 'PRIVATE_INDEX' }, /invalid step update.*step_index=string/],
    [{ state: null }, /invalid step update.*state=null/],
    [{ state: 'PRIVATE_FUTURE_STATE' }, /unsupported step state.*state=unsupported-string/],
    [{ step_type: '' }, /invalid step update.*step_type=empty-string/],
    [{ step_type: ['PRIVATE_CATEGORY'] }, /invalid step update.*step_type=array/],
    [{ text_delta: { command: 'PRIVATE_COMMAND' } }, /invalid step update.*text_delta=object/],
    [{ tool_info: ['PRIVATE_TOOL'] }, /invalid or failed tool step.*tool_info=array/],
  ];
  for (const [override, expected] of cases) {
    const state = session();
    state.event(step({ ...override, parameters: { command: 'PRIVATE_COMMAND', path: 'PRIVATE_PATH' }, title: 'PRIVATE_TITLE' }));
    state.event(result());
    assert.match(state.failures[0], expected);
    assert.equal(JSON.stringify(state.failures).includes('PRIVATE_'), false);
    assert.deepEqual(state.output, []); assert.deepEqual(state.completed, []);
  }
  const unknown = session();
  unknown.event(step({ step_index: null, step_type: 'PRIVATE_CATEGORY', text_delta: 'PRIVATE_BODY', tool_info: { parameters: 'PRIVATE_PARAMETERS' } }));
  assert.match(unknown.failures[0], /step_type=other-string; text_delta=string; tool_info=object/);
  assert.equal(JSON.stringify(unknown.failures).includes('PRIVATE_'), false);
});

test('Antigravity requires a complete assistant stream that exactly matches the terminal response', () => {
  for (const [updates, terminal] of [
    [[], result()], [[step({ state: 'ACTIVE' })], result()], [[step()], result({ response: 'Different answer\n' })],
    [[step({ text_delta: ' ' })], result({ response: ' ' })], [[step()], result({ response: null })],
    [[step(), step({ step_index: 2, step_type: 'tool', state: 'ACTIVE' })], result()],
  ]) {
    const state = session(); for (const update of updates) state.event(update); state.event(terminal);
    assert.equal(state.failures.length, 1); assert.deepEqual(state.completed, []);
  }
});

test('Antigravity never accepts a successful terminal envelope containing an error or partial finish reason', () => {
  for (const error of [null, false, '', 'provider failure', {}, { message: 'failure' }]) {
    for (const terminal of [result({ error }), { ...result(), error }]) {
      const state = session(); state.event(step()); state.event(terminal);
      assert.equal(state.failures.length, 1); assert.deepEqual(state.completed, []);
      assert.deepEqual(state.output, ['Answer\n']);
    }
  }
  for (const finished_reason of ['print_timeout', 'interrupted', 'error', null, false]) {
    const state = session(); state.event(step()); state.event(result({ finished_reason }));
    assert.equal(state.failures.length, 1); assert.deepEqual(state.completed, []);
  }
});

test('Antigravity preserves failed and incomplete terminal states without exposing provider diagnostics', () => {
  for (const status of ['ERROR', 'INTERRUPTED', 'INVALID', 'WAITING', 'RUNNING', 'future_status', null]) {
    const state = session(); state.event(step({ state: 'ACTIVE', text_delta: 'Partial evidence.' }));
    state.event(result({ status, response: 'Partial evidence.', error: 'PRIVATE_PROVIDER_DIAGNOSTIC' }));
    assert.equal(state.failures.length, 1); assert.deepEqual(state.completed, []);
    assert.deepEqual(state.output, ['Partial evidence.']);
    assert.equal(state.failures[0].includes('PRIVATE_PROVIDER_DIAGNOSTIC'), false);
  }
});

test('Antigravity preserves explicit cancellation and partial text for either cancellation spelling', () => {
  for (const status of ['CANCELED', 'CANCELLED']) {
    const state = session(); state.event(step({ state: 'ACTIVE', text_delta: 'Partial evidence.' }));
    state.event(result({ status, response: 'Partial evidence.', error: 'cancelled by user' }));
    assert.deepEqual(state.failures, []); assert.deepEqual(state.completed, ['cancelled']);
    assert.deepEqual(state.output, ['Partial evidence.']);
  }
});

test('Antigravity rejects failed tools and human permission requests without inventing replies', () => {
  for (const update of [step({ step_type: 'tool', tool_name: 'ask_permission' }),
    step({ step_type: 'tool', tool_name: 'ask_custom_permission' }),
    step({ step_type: 'tool', tool_info: { name: 'ask_question' } }),
    step({ step_type: 'tool', tool_info: { name: 'AskUserQuestion' } }),
    step({ step_type: 'tool', tool_info: { name: 'read_file', error: { type: 'permission_denied', message: 'PRIVATE_TOOL_DIAGNOSTIC' } } }),
    { event: 'control_request', request: { subtype: 'can_use_tool' } }]) {
    const state = session(); state.event(update); state.event(result());
    assert.ok(state.failures.length > 0); assert.deepEqual(state.completed, []);
    assert.deepEqual(state.sent, []); assert.deepEqual(state.output, []);
    assert.equal(state.failures.some(error => error.includes('PRIVATE_TOOL_DIAGNOSTIC')), false);
  }
});

test('Antigravity classifies observed native ERROR tool frames as failures despite later terminal SUCCESS', () => {
  for (const partial of [false, true]) {
    const state = session();
    if (partial) state.event(step({ step_index: 1, text_delta: 'Partial evidence.' }));
    const active = step({ step_index: 6, state: 'ACTIVE', step_type: 'tool',
      tool_name: 'read_file', tool_info: { name: 'read_file', parameters: { Path: 'PRIVATE_PATH' } } });
    delete active.step_update.text_delta;
    state.event(active);
    const failure = step({ step_index: 6, state: 'ERROR', step_type: 'tool', tool_name: 'read_file',
      tool_info: { name: 'read_file', parameters: { Path: 'PRIVATE_PATH' }, error: { type: 'permission_denied', message: 'PRIVATE_DIAGNOSTIC' } } });
    delete failure.step_update.text_delta;
    state.event(failure);
    state.event(result({ response: partial ? 'Partial evidence.' : '' }));
    assert.match(state.failures[0], /tool execution failed or was denied.*state=ERROR.*step_type=tool/);
    assert.equal(JSON.stringify(state.failures).includes('PRIVATE_'), false);
    assert.deepEqual(state.completed, []);
    assert.deepEqual(state.output, partial ? ['Partial evidence.'] : []);
    assert.deepEqual(state.sent, []);
  }
});

test('Antigravity ERROR cannot become a successful state without a valid structured tool failure', () => {
  for (const override of [
    { state: 'ERROR' }, { state: 'ERROR', step_type: 'tool' },
    { state: 'ERROR', step_type: 'tool', tool_info: { error: null } },
    { state: 'ERROR', step_type: 'tool', tool_info: { error: { type: 'permission_denied', message: {} } } },
  ]) {
    const state = session(); state.event(step(override)); state.event(result());
    assert.match(state.failures[0], /unsupported step state/);
    assert.deepEqual(state.completed, []); assert.deepEqual(state.output, []);
  }
});

test('Antigravity diagnostics prevent partial success without exposing stderr contents', () => {
  const clean = session();
  for (const text of ['', ' ', '\r\n\t']) clean.protocol.diagnostic(text, clean.control);
  clean.event(step()); clean.event(result());
  assert.deepEqual(clean.failures, []); assert.deepEqual(clean.completed, ['end_turn']);
  for (const completed of [false, true]) {
    const state = session(); state.event(step());
    if (completed) state.event(result());
    state.protocol.diagnostic('Permission denied: PRIVATE_DIAGNOSTIC', state.control);
    assert.equal(state.failures.length, 1);
    assert.match(state.failures[0], /diagnostics.*withheld/i);
    assert.equal(state.failures[0].includes('PRIVATE_DIAGNOSTIC'), false);
    if (!completed) { state.event(result()); assert.deepEqual(state.completed, []); }
  }
});

test('Antigravity recognizes the exact native headless denial diagnostic without retaining provider content', () => {
  const state = session();
  state.protocol.diagnostic('PRIVATE_TOOL required a permission that headless mode cannot prompt for, so it was auto-denied. PRIVATE_PATH PRIVATE_DIAGNOSTIC', state.control);
  state.event(result({ response: '' }));
  assert.match(state.failures[0], /denied a tool permission request in headless mode.*workspace scope.*native CLI permissions.*withheld/);
  assert.equal(JSON.stringify(state.failures).includes('PRIVATE_'), false);
  assert.deepEqual(state.completed, []); assert.deepEqual(state.output, []);
});

test('Antigravity never waits for a split diagnostic marker or classifies loose permission text as the native denial', () => {
  const marker = 'permission that headless mode cannot prompt for, so it was auto-denied.';
  for (let split = 1; split < marker.length; split++) {
    const state = session();
    state.protocol.diagnostic(marker.slice(0, split), state.control);
    assert.match(state.failures[0], /emitted diagnostics.*withheld/);
    state.protocol.diagnostic(marker.slice(split), state.control);
    state.event(result());
    assert.deepEqual(state.completed, []); assert.deepEqual(state.output, []);
  }
  for (const text of ['Permission denied PRIVATE_DIAGNOSTIC', 'Tool permission debug PRIVATE_DIAGNOSTIC', marker.slice(0, -1),
    marker.replace('auto-denied.', 'not auto-denied. PRIVATE_PATH')]) {
    const state = session(); state.protocol.diagnostic(text, state.control);
    assert.match(state.failures[0], /emitted diagnostics.*withheld/);
    assert.equal(JSON.stringify(state.failures).includes('PRIVATE_'), false);
    assert.deepEqual(state.completed, []);
  }
});

test('Antigravity rejects ambiguous envelopes and all events after terminal completion or cancellation', () => {
  for (const event of [{ ...step(), result: result().result }, { event: 'result', result: [] }, { event: null }]) {
    const state = session(); state.event(event);
    assert.equal(state.failures.length, 1); assert.deepEqual(state.completed, []);
  }
  for (const late of [result(), step({ step_index: 2 }), init()]) {
    const state = session(); state.event(step()); state.event(result()); state.event(late);
    assert.equal(state.failures.length, 1); assert.deepEqual(state.output, ['Answer\n']);
  }
  const cancelled = session(); cancelled.protocol.cancel(); cancelled.event(step());
  assert.equal(cancelled.failures.length, 1); assert.deepEqual(cancelled.output, []);
});
