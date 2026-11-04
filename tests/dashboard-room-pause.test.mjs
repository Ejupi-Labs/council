import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext, Script } from 'node:vm';
import { dashboardHtml } from '../dist/dashboard-ui.js';

function deferred() {
  let resolve, reject;
  const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

const html = dashboardHtml('pause-fixture');
const script = /<script nonce="[^"]+">([\s\S]*?)<\/script>/.exec(html)[1];
function browserFunctions(prefixes) {
  const lines = script.split(/\r?\n/);
  return prefixes.map(prefix => {
    const source = lines.find(line => line.startsWith(prefix));
    assert.ok(source, 'Missing browser function: ' + prefix); return source;
  }).join('\n');
}

function record(id = 'A', status = 'running', revision = 1, extra = {}) {
  return { id, status, revision, backend: 'mock', title: 'Room ' + id, brief: 'Complete objective.',
    tasks: [{ id: 'task-1', status: 'assigned' }], messages: [], members: [],
    decision: { approved: true }, callsUsed: 1, maxCalls: 12, contextLimit: null, autoExecute: true, ...extra };
}

function harness(handler = async () => record()) {
  const nodes = new Map(), calls = [], errors = [], rendered = [], listeners = new Map();
  const node = id => {
    if (!nodes.has(id)) nodes.set(id, { disabled: false, hidden: false, textContent: '', children: [],
      replaceChildren(...children) { this.children = children; },
      addEventListener(event, listener) { listeners.set(id + ':' + event, listener); },
    });
    return nodes.get(id);
  };
  const context = createContext({ Promise, encodeURIComponent, $: node,
    api: (path, body) => { calls.push({ path, body }); return handler(path, body); },
    clearError() {}, error: message => errors.push(message), refreshList: async () => {}, restoreDraft() {},
    renderMessages() {}, updateHistoryControls() {}, renderTasks() {}, emptyConversation() {},
    number: String, initial: String,
    element: (_tag, _class, text) => ({ textContent: text, append() {} }),
  });
  runInContext("var token='fixture',serverBackend='mock',selected='A',selectionEpoch=0,busy=false,roomLoading=false,historyBusy=false;", context);
  context.room = record();
  runInContext(browserFunctions(['var labels=', 'function backendMismatch()', 'function active()', 'function roomWaitNote()',
    'function controls()', 'function renderRoom()', 'function requestTarget()', 'function sameTarget(',
    'function applyRoomResponse(', 'async function perform(action)']), context);
  for (const action of ['pause', 'resume']) {
    const listener = script.match(new RegExp("\\$\\('" + action + "'\\)\\.addEventListener\\('click',function\\(\\)\\{[^\\n]*?\\}\\);"));
    assert.ok(listener, 'Missing ' + action + ' click handler'); runInContext(listener[0], context);
  }
  return { context, calls, errors, nodes, node, rendered,
    run: code => runInContext(code, context),
    render(value = context.room) { context.room = value; runInContext('renderRoom()', context); rendered.push(value); },
    click: action => listeners.get(action + ':click')(),
  };
}

test('pause controls are present and the complete browser script remains valid', () => {
  assert.match(html, /id="pause"[^>]*>Pause<\/button>/);
  assert.match(html, /id="resume"[^>]*>Resume<\/button>/);
  assert.doesNotThrow(() => new Script(script));
});

test('pause is offered only for active or ready rooms, and paused rooms can resume or stop', () => {
  const wire = harness();
  for (const status of ['discussing', 'running', 'ready']) {
    wire.render(record('A', status));
    assert.equal(wire.node('pause').disabled, false, status);
    assert.equal(wire.node('pause').hidden, false);
    assert.equal(wire.node('resume').hidden, true);
  }
  for (const status of ['idle', 'needs_attention', 'completed', 'failed', 'cancelled']) {
    wire.render(record('A', status)); assert.equal(wire.node('pause').disabled, true, status);
    assert.equal(wire.node('resume').disabled, true, status);
  }
  wire.render(record('A', 'paused', 2, { checkpoint: { phase: 'execution' } }));
  assert.equal(wire.node('roomStatus').textContent, 'Paused');
  assert.equal(wire.node('pause').hidden, true); assert.equal(wire.node('resume').hidden, false);
  assert.equal(wire.node('resume').disabled, false); assert.equal(wire.node('cancel').disabled, false);
  assert.equal(wire.node('discuss').disabled, true); assert.equal(wire.node('execute').disabled, true);
  assert.equal(wire.node('addTask').disabled, true); assert.equal(wire.node('sendMessage').disabled, false);
});

test('pending pause distinguishes draining workflows from suspension and prevents duplicate requests', async () => {
  const pending = deferred(), wire = harness(() => pending.promise);
  wire.render(); wire.click('pause'); wire.click('pause');
  assert.equal(wire.calls.length, 1); assert.equal(wire.calls[0].path, '/api/rooms/A/pause');
  pending.resolve(record('A', 'running', 2, { pauseRequested: true }));
  await new Promise(resolve => setImmediate(resolve));
  assert.match(wire.node('roomStatus').textContent, /Pause requested.*Tasks in progress/);
  assert.equal(wire.node('pause').textContent, 'Pause requested'); assert.equal(wire.node('pause').disabled, true);
  assert.equal(wire.node('resume').disabled, true); assert.equal(wire.node('cancel').disabled, false);
  assert.match(wire.node('waitNote').textContent, /internal agent calls may continue/);
  assert.match(wire.node('waitNote').textContent, /processes are not suspended/);
});

test('pause and resume respect selection loading, pending actions and recorded backend', () => {
  const wire = harness();
  for (const status of ['running', 'paused']) {
    wire.render(record('A', status));
    for (const change of [
      { roomLoading: true }, { busy: true }, { selected: 'B' }, { serverBackend: 'native' },
    ]) {
      Object.assign(wire.context, { roomLoading: false, busy: false, selected: 'A', serverBackend: 'mock' }, change);
      wire.run('controls()');
      assert.equal(wire.node('pause').disabled, true); assert.equal(wire.node('resume').disabled, true);
    }
    Object.assign(wire.context, { roomLoading: false, busy: false, selected: 'A', serverBackend: 'mock' });
  }
  wire.render(record('A', 'paused', 2, { backend: undefined }));
  assert.equal(wire.node('resume').disabled, true);
});

test('resume notes explain each checkpoint without promising process suspension or replay', () => {
  const wire = harness();
  wire.render(record('A', 'paused', 2, { checkpoint: { phase: 'ready' } }));
  assert.match(wire.node('waitNote').textContent, /without starting tasks/);
  wire.render(record('A', 'paused', 3, { checkpoint: { phase: 'discussion', nextRound: 2 } }));
  assert.match(wire.node('waitNote').textContent, /saved round/);
  assert.match(wire.node('waitNote').textContent, /New guidance clears this checkpoint/);
  wire.render(record('A', 'paused', 4, { checkpoint: { phase: 'execution' } }));
  assert.match(wire.node('waitNote').textContent, /without replaying completed tasks/);
  assert.equal(wire.node('contextMetric').textContent, 'Full context (default)');
});

test('late pause response cannot replace another selected room or its controls', async () => {
  const pending = deferred(), wire = harness(() => pending.promise);
  wire.render(); const action = wire.run("perform('pause')");
  wire.context.selected = 'B'; wire.context.selectionEpoch++; wire.render(record('B', 'paused', 3));
  pending.resolve(record('A', 'paused', 4)); await action;
  assert.equal(wire.context.room.id, 'B'); assert.equal(wire.context.room.revision, 3);
  assert.equal(wire.node('roomTitle').textContent, 'Room B'); assert.equal(wire.node('resume').disabled, false);
  assert.deepEqual(wire.errors, []);
});

test('late resume failure does not overwrite a new selection error or leave controls busy', async () => {
  const pending = deferred(), wire = harness(() => pending.promise);
  wire.render(record('A', 'paused', 2)); const action = wire.run("perform('resume')");
  assert.equal(wire.calls[0].path, '/api/rooms/A/resume');
  wire.context.selected = 'B'; wire.context.selectionEpoch++; wire.render(record('B', 'ready', 8));
  wire.errors.push('Keep the current B error.'); pending.reject(new Error('Obsolete resume failed.')); await action;
  assert.equal(wire.context.room.id, 'B'); assert.equal(wire.context.busy, false);
  assert.equal(wire.node('pause').disabled, false); assert.deepEqual(wire.errors, ['Keep the current B error.']);
});

test('resume success cannot replace newer guidance invalidating the paused checkpoint', async () => {
  const pending = deferred(), wire = harness(() => pending.promise);
  wire.render(record('A', 'paused', 2)); const action = wire.run("perform('resume')");
  wire.render(record('A', 'needs_attention', 4, { decision: undefined }));
  pending.resolve(record('A', 'running', 3)); await action;
  assert.equal(wire.context.room.status, 'needs_attention'); assert.equal(wire.context.room.revision, 4);
  assert.equal(wire.node('resume').disabled, true); assert.equal(wire.node('execute').disabled, true);
});

test('resume rejection on the current room is visible and permits another deliberate action', async () => {
  const wire = harness(async () => { throw new Error('The saved checkpoint is unavailable. Start a new discussion.'); });
  wire.render(record('A', 'paused', 2)); await wire.run("perform('resume')");
  assert.match(wire.errors[0], /checkpoint is unavailable/); assert.equal(wire.context.busy, false);
  assert.equal(wire.node('resume').disabled, false); assert.equal(wire.node('cancel').disabled, false);
});
