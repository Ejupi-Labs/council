import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext } from 'node:vm';
import { dashboardHtml } from '../dist/dashboard-ui.js';
import { billingClientSource } from '../dist/dashboard-billing-ui.js';
import { runClientSource } from '../dist/dashboard-runs-ui.js';

function deferred() {
  let resolve, reject;
  const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

const browserSource = /<script nonce="[^"]+">([\s\S]*?)<\/script>/.exec(dashboardHtml('dialog-fixture'))[1];
function functions(prefixes) {
  const lines = browserSource.split(/\r?\n/);
  return prefixes.map(prefix => {
    const source = lines.find(line => line.startsWith(prefix));
    assert.ok(source, 'Missing browser function: ' + prefix); return source;
  }).join('\n');
}

function listeners(id, event) {
  // Extract complete actual registration statements, including callback bodies.
  const marker = "$('" + id + "').addEventListener('" + event + "',", statements = [];
  for (let start = browserSource.indexOf(marker); start >= 0; start = browserSource.indexOf(marker, start + marker.length)) {
    let depth = 1, quote = '', escaped = false, end = start + marker.length;
    for (; end < browserSource.length; end++) {
      const char = browserSource[end];
      if (quote) {
        if (escaped) escaped = false;
        else if (char === '\\') escaped = true;
        else if (char === quote) quote = '';
      } else if (char === "'" || char === '"' || char === '`') quote = char;
      else if (char === '(') depth++;
      else if (char === ')' && --depth === 0) break;
    }
    assert.equal(depth, 0); statements.push(browserSource.slice(start, end + 2));
  }
  assert.ok(statements.length, 'Missing ' + id + ' ' + event + ' listener'); return statements.join('\n');
}

function dom() {
  const nodes = new Map(), queuedClose = [];
  class Node {
    constructor(tag = '') {
      this.tag = tag; this.children = []; this.listeners = new Map(); this.textContent = '';
      this.value = ''; this.hidden = false; this.open = false; this.disabled = false;
    }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    setAttribute(key, value) { this[key] = value; }
    addEventListener(key, listener) {
      if (!this.listeners.has(key)) this.listeners.set(key, []); this.listeners.get(key).push(listener);
    }
    dispatch(key) { for (const listener of this.listeners.get(key) ?? []) listener({ preventDefault() {} }); }
    showModal() { this.open = true; }
    close() {
      if (!this.open) return;
      this.open = false;
      // Native close events are queued; a later showModal does not cancel them.
      queuedClose.push(() => this.dispatch('close'));
    }
    escape() { this.dispatch('cancel'); this.close(); }
    focus() {}
    querySelectorAll() { return []; }
  }
  const $ = id => { if (!nodes.has(id)) nodes.set(id, new Node(id)); return nodes.get(id); };
  return { $, nodes, element: (tag, cls, text) => {
    const node = new Node(tag); node.className = cls ?? ''; if (text !== undefined) node.textContent = String(text); return node;
  }, flushClose() { for (const dispatch of queuedClose.splice(0)) dispatch(); } };
}

function runRecord(id = 'run-a', task = 'Latest complete task') {
  return { id, status: 'completed', createdAt: '2026-11-04T12:00:00Z', updatedAt: '2026-11-04T12:01:00Z',
    input: { mode: 'delegate', permission: 'read', agents: ['worker'], task }, steps: [], output: 'Complete retained result.' };
}

function workflowHarness(handler) {
  const view = dom(), calls = [];
  const context = createContext({ Promise, AbortController, encodeURIComponent, $: view.$, element: view.element,
    labels: { completed: 'Completed' }, window: view.element('window'),
    api: (path, _body, signal) => { calls.push({ path, signal }); return handler(path); },
  });
  runInContext(runClientSource() + '\ninstallRunUI();', context);
  return { ...view, context, calls, run: code => runInContext(code, context), open: () => runInContext("openWorkflow('run-a')", context) };
}

test('an old native close event cannot abort a same-workflow reopen or expose its obsolete error', async () => {
  const old = deferred(), current = deferred(); let reads = 0;
  const wire = workflowHarness(() => ++reads === 1 ? old.promise : current.promise);
  const first = wire.open(); wire.run('dismissWorkflow()'); const second = wire.open();
  const epoch = wire.run('workflowEpoch'); wire.flushClose();
  assert.equal(wire.run('workflowEpoch'), epoch); assert.equal(wire.calls[1].signal.aborted, false);
  current.resolve(runRecord()); await second; old.reject(new Error('Obsolete closed workflow failure.')); await first;
  assert.equal(wire.run('workflowRecord.input.task'), 'Latest complete task');
  assert.equal(wire.$('workflowDialog').open, true); assert.equal(wire.$('downloadWorkflow').disabled, false);
  assert.equal(wire.$('workflowError').hidden, true);
});

test('a genuinely closed workflow and Escape still invalidate pending reads and downloads', async () => {
  for (const escape of [false, true]) {
    const pending = deferred(), wire = workflowHarness(() => pending.promise), loading = wire.open();
    if (escape) wire.$('workflowDialog').escape(); else wire.$('workflowDialog').close();
    wire.flushClose();
    assert.equal(wire.calls[0].signal.aborted, true); assert.equal(wire.run('workflowId'), null);
    pending.resolve(runRecord()); await loading;
    assert.equal(wire.$('workflowDialog').open, false); assert.equal(wire.$('workflowBody').children.length, 0);
    assert.equal(wire.$('downloadWorkflow').disabled, true);
  }
});

function billingHarness(handler) {
  const view = dom(), calls = [], shown = [];
  const context = createContext({ Promise, AbortController, $: view.$, element: view.element, token: 'fixture',
    api: (path, _body, signal) => { calls.push({ path, signal }); return handler(path); }, queueMicrotask,
  });
  runInContext(billingClientSource(), context);
  Object.assign(context, { billingCapture() {}, billingSaveState() {}, billingRenderAliases() {},
    billingShow(alias) { context.billingSelected = alias; shown.push(alias); },
  });
  runInContext('installBillingUI();', context);
  return { ...view, context, calls, shown, run: code => runInContext(code, context) };
}

test('a delayed billing close cannot cancel the new load or discard its local drafts', async () => {
  const first = deferred(), next = deferred(); let reads = 0;
  const wire = billingHarness(() => ++reads === 1 ? first.promise : next.promise);
  wire.run('openBilling();closeBilling();openBilling();');
  const epoch = wire.run('billingEpoch'), draft = { raw: { plan: 'Unsaved plan' }, generation: 8 };
  wire.context.billingDrafts.set('worker', draft); wire.flushClose();
  assert.equal(wire.run('billingEpoch'), epoch); assert.equal(wire.calls[1].signal.aborted, false);
  next.resolve({ agents: [{ alias: 'worker', revision: 2 }], skills: [] }); await new Promise(resolve => setImmediate(resolve));
  first.reject(new Error('Obsolete closed billing failure.')); await new Promise(resolve => setImmediate(resolve));
  assert.equal(wire.$('billingDialog').open, true); assert.deepEqual(wire.shown, ['worker']);
  assert.equal(wire.context.billingDrafts.get('worker'), draft); assert.equal(wire.$('billingError').hidden, true);
  assert.equal(wire.run('billingLoading'), false);
});

test('a direct billing close or Escape still aborts its current local-record read', async () => {
  for (const escape of [false, true]) {
    const pending = deferred(), wire = billingHarness(() => pending.promise); wire.run('openBilling()');
    if (escape) wire.$('billingDialog').escape(); else wire.$('billingDialog').close(); wire.flushClose();
    assert.equal(wire.calls[0].signal.aborted, true); assert.equal(wire.run('billingController'), undefined);
    pending.resolve({ agents: [], skills: [] }); await new Promise(resolve => setImmediate(resolve));
    assert.equal(wire.$('billingDialog').open, false); assert.equal(wire.run('billingLoading'), false);
  }
});

function createHarness(handler) {
  const view = dom(), calls = [], rendered = [];
  view.$('memberRows').children.push({});
  const context = createContext({ Promise, AbortController, $: view.$, element: view.element,
    agents: [{ agent: 'worker' }], error: message => assert.fail(message),
    api: (path, _body, signal) => { calls.push({ path, signal }); return handler(path); },
  });
  runInContext('var createEpoch=0,createDraftGeneration=0,createPending=new Set();' + billingClientSource(), context);
  Object.assign(context, { routingRequest: () => ({ task: 'Complete unchanged objective' }), routingRender: value => rendered.push(value) });
  runInContext(functions(['function invalidateCreateDialog()', 'function dismissCreateDialog()', 'function openCreate()'])
    + listeners('createDialog', 'close') + listeners('createDialog', 'cancel'), context);
  return { ...view, context, calls, rendered, run: code => runInContext(code, context) };
}

test('an old create close event cannot invalidate the reopened draft or its new routing preview', async () => {
  const pending = deferred(), wire = createHarness(() => pending.promise);
  wire.run('openCreate();dismissCreateDialog();openCreate();');
  const preview = wire.run('previewRouting()'), epoch = wire.run('createEpoch'), generation = wire.run('createDraftGeneration');
  wire.flushClose();
  assert.equal(wire.run('createEpoch'), epoch); assert.equal(wire.run('createDraftGeneration'), generation);
  assert.equal(wire.calls[0].signal.aborted, false); assert.equal(wire.$('createDialog').open, true);
  const plan = { selectedAliases: ['worker'], suggestedRoomMembers: [{ agent: 'worker', role: 'lead', weight: 1 }] };
  pending.resolve(plan); await preview;
  assert.equal(wire.run('routingPlan'), plan); assert.equal(wire.$('routingApply').disabled, false);
  assert.deepEqual(wire.rendered, [plan]);
});

test('direct native create closure and Escape still invalidate its draft epoch and routing request', async () => {
  for (const escape of [false, true]) {
    const pending = deferred(), wire = createHarness(() => pending.promise); wire.run('openCreate()');
    const preview = wire.run('previewRouting()'), epoch = wire.run('createEpoch');
    if (escape) wire.$('createDialog').escape(); else wire.$('createDialog').close(); wire.flushClose();
    assert.equal(wire.calls[0].signal.aborted, true); assert.ok(wire.run('createEpoch') > epoch);
    pending.resolve({ selectedAliases: ['worker'] }); await preview;
    assert.equal(wire.$('createDialog').open, false); assert.equal(wire.run('routingPlan'), null);
  }
});

function historyHarness(handler) {
  const view = dom(), calls = [], errors = [];
  const context = createContext({ Promise, AbortController, encodeURIComponent, $: view.$,
    api: (path, _body, signal) => { calls.push({ path, signal }); return handler(path); },
    updateHistoryControls() {}, messageNode: value => value, number: String, error: message => errors.push(message),
  });
  runInContext("var room={id:'A',messages:[]},selected='A',selectionEpoch=0,roomLoading=false,historyBusy=false,historyOpen=false,historyLoaded=false,historyMessages=[],historyCursor=null,historyEpoch=0,historyController;"
    + functions(['function invalidateHistory()', 'function dismissHistory()', 'function renderHistoryMessages(', 'function updateHistoryControls()', 'async function loadHistory('])
    + listeners('historyDialog', 'close') + listeners('historyDialog', 'cancel'), context);
  return { ...view, context, calls, errors, run: code => runInContext(code, context), load: () => runInContext('loadHistory(false)', context) };
}

test('history opens immediately with loading feedback and keeps first-page failures inside the modal', async () => {
  const pending = deferred(), wire = historyHarness(() => pending.promise), loading = wire.load();
  assert.equal(wire.$('historyDialog').open, true); assert.match(wire.$('historyPageNote').textContent, /Loading complete stored messages/);
  pending.reject(new Error('The local history could not be read.')); await loading;
  assert.equal(wire.$('historyDialog').open, true); assert.equal(wire.$('historyError').hidden, false);
  assert.match(wire.$('historyError').textContent, /could not be read/); assert.match(wire.$('historyPageNote').textContent, /could not be loaded/);
  assert.equal(wire.$('historyNewer').hidden, false); assert.equal(wire.$('historyNewer').textContent, 'Retry loading history');
  assert.equal(wire.$('historyPageNote').textContent.includes('Previously loaded'), false);
  assert.deepEqual(wire.errors, []);
});

test('an old history close event cannot abort a reopened first-page request or clear its full response', async () => {
  const first = deferred(), next = deferred(); let reads = 0;
  const wire = historyHarness(() => ++reads === 1 ? first.promise : next.promise), old = wire.load();
  wire.run('dismissHistory()'); const current = wire.load(), epoch = wire.run('historyEpoch');
  assert.equal(wire.$('historyDialog').open, true); wire.flushClose();
  assert.equal(wire.run('historyEpoch'), epoch); assert.equal(wire.calls[1].signal.aborted, false);
  const message = { id: 'complete', body: 'Complete retained message. '.repeat(2000) };
  next.resolve({ messages: [message], nextCursor: null }); await current;
  first.reject(new Error('Obsolete closed history error.')); await old;
  assert.equal(wire.run('historyMessages[0].body'), message.body); assert.equal(wire.$('historyDialog').open, true);
  assert.equal(wire.$('historyError').hidden, true); assert.deepEqual(wire.errors, []);
});

test('history cannot reopen after direct native closure even before its queued close event arrives', async () => {
  const pending = deferred(), wire = historyHarness(() => pending.promise), loading = wire.load();
  wire.$('historyDialog').close(); pending.resolve({ messages: [{ id: 'late', body: 'Late first page.' }], nextCursor: null });
  await loading;
  assert.equal(wire.$('historyDialog').open, false); assert.equal(wire.$('historyMessages').children.length, 0);
  wire.flushClose(); assert.equal(wire.run('historyOpen'), false); assert.equal(wire.run('historyController'), undefined);
});

test('history Escape still aborts the pending page and preserves cancellation cleanup', async () => {
  const pending = deferred(), wire = historyHarness(() => pending.promise), loading = wire.load();
  wire.$('historyDialog').escape(); wire.flushClose();
  assert.equal(wire.calls[0].signal.aborted, true); pending.reject(new Error('Cancelled history response.')); await loading;
  assert.equal(wire.$('historyDialog').open, false); assert.equal(wire.run('historyOpen'), false);
  assert.equal(wire.$('historyError').hidden, true); assert.deepEqual(wire.errors, []);
});
