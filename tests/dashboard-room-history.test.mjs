import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext } from 'node:vm';
import { dashboardHtml } from '../dist/dashboard-ui.js';

const source = /<script nonce="[^"]+">([\s\S]*?)<\/script>/.exec(dashboardHtml('room-history-fixture'))[1];
function deferred() {
  let resolve, reject; const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}
function harness(handler) {
  const elements = new Map(), calls = [], loads = [], errors = [];
  const element = id => {
    if (!elements.has(id)) elements.set(id, { children: [], listeners: new Map(), scrollTop: 0, replacements: 0, open: false, hidden: false, disabled: false, value: '', textContent: '',
      replaceChildren() { this.children = []; this.replacements++; this.scrollTop = 0; },
      append(node) { this.children.push(node); this.scrollTop += 100; },
      addEventListener(event, callback) { this.listeners.set(event, callback); },
      click() { if (!this.hidden && !this.disabled) return this.listeners.get('click')?.(); },
      showModal() { this.open = true; }, close() { this.open = false; },
    });
    return elements.get(id);
  };
  const context = createContext({ Promise, AbortController, encodeURIComponent, $: element,
    api: (path, _body, signal) => { calls.push({ path, signal }); return Promise.resolve(handler(path, signal)); },
    messageNode: message => ({ id: message.id, body: message.body }), number: String, error: message => errors.push(message),
  });
  runInContext("var room={id:'A',messages:[]},selected='A',selectionEpoch=0,roomLoading=false,historyBusy=false,historyOpen=false,historyLoaded=false,historyMessages=[],historyCursor=null,historyAfter=0,historyEpoch=0,historyController;", context);
  const prefixes = ['function invalidateHistory()', 'function dismissHistory()', 'function renderHistoryMessages(', 'function updateHistoryControls()', 'async function loadHistory('];
  for (const prefix of prefixes) {
    const line = source.split(/\r?\n/).find(line => line.startsWith(prefix)); assert.ok(line, 'Missing shipped history function ' + prefix); runInContext(line, context);
  }
  const shipped = context.loadHistory;
  context.loadHistory = (...args) => { const load = shipped(...args); loads.push(load); return load; };
  for (const id of ['historyNext', 'historyNewer']) {
    const marker = "$('" + id + "').addEventListener('click',", start = source.indexOf(marker), end = source.indexOf('});', start);
    assert.ok(start >= 0 && end > start, 'Missing actual history button binding ' + id); runInContext(source.slice(start, end + 3), context);
  }
  element('messageBody').value = 'Unsent room guidance'; element('taskBrief').value = 'Unsent task acceptance criteria';
  return { context, calls, errors, element, run: code => runInContext(code, context), open: () => context.loadHistory(false),
    click(id) { const before = loads.length; element(id).click(); assert.equal(loads.length, before + 1, 'The enabled shipped history button must start one read.'); return loads.at(-1); } };
}

test('checking newer room messages appends complete deduplicated nodes and preserves scroll and unsent drafts', async () => {
  const complete = 'Complete newer report 🧭 '.repeat(3000); let reads = 0;
  const wire = harness(() => ++reads === 1 ? { messages: [{ id: 'one', body: 'First report' }, { id: 'two', body: 'Second report' }], nextCursor: null, nextAfter: 20 }
    : reads === 2 ? { messages: [{ id: 'two', body: 'Second report' }, { id: 'three', body: complete }], nextCursor: null, nextAfter: 25 }
      : { messages: [], nextCursor: null, nextAfter: 25 });
  await wire.open();
  const view = wire.element('historyMessages'), original = [...view.children], replacements = view.replacements;
  view.scrollTop = 37; assert.equal(wire.element('historyNewer').hidden, false); assert.equal(wire.calls.length, 1);
  await wire.click('historyNewer');
  assert.match(wire.calls[1].path, /cursor=20$/); assert.equal(view.children.length, 3); assert.equal(view.children[0], original[0]); assert.equal(view.children[1], original[1]);
  assert.equal(view.children[2].body, complete); assert.equal(view.replacements, replacements); assert.equal(view.scrollTop, 37);
  assert.equal(wire.element('messageBody').value, 'Unsent room guidance'); assert.equal(wire.element('taskBrief').value, 'Unsent task acceptance criteria');
  assert.equal(wire.run('historyAfter'), 25); assert.match(wire.element('historyPageNote').textContent, /Loaded 1 newer/);
  await wire.click('historyNewer'); assert.match(wire.calls[2].path, /cursor=25$/);
  assert.equal(wire.run('historyAfter'), 25); assert.equal(view.children.length, 3); assert.equal(view.scrollTop, 37); assert.match(wire.element('historyPageNote').textContent, /No newer messages/);
  assert.equal(reads, 3, 'Only the initial read and explicit user checks may fetch history.');
});

test('a newer-message check can continue through bounded pages before another newer check is offered', async () => {
  let reads = 0;
  const wire = harness(() => ++reads === 1 ? { messages: [{ id: 'one', body: 'One' }], nextCursor: null, nextAfter: 1 }
    : reads === 2 ? { messages: [{ id: 'two', body: 'Two' }], nextCursor: 2, nextAfter: 2 }
      : { messages: [{ id: 'three', body: 'Three' }], nextCursor: null, nextAfter: 3 });
  await wire.open(); await wire.click('historyNewer');
  assert.equal(wire.element('historyNext').hidden, false); assert.equal(wire.element('historyNewer').hidden, true);
  await wire.run('loadHistory(true,true)'); assert.equal(reads, 2, 'A check must not skip an outstanding continuation page.');
  await wire.click('historyNext'); assert.match(wire.calls[2].path, /cursor=2$/);
  assert.deepEqual(wire.element('historyMessages').children.map(node => node.id), ['one', 'two', 'three']);
  assert.equal(wire.element('historyNext').hidden, true); assert.equal(wire.element('historyNewer').hidden, false); assert.equal(wire.run('historyAfter'), 3);
});

test('failed newer-message checks preserve loaded evidence and their exact retry cursor', async () => {
  const pending = deferred(); let reads = 0;
  const wire = harness(() => ++reads === 1 ? { messages: [{ id: 'one', body: 'Retained report' }], nextCursor: null, nextAfter: 7 }
    : reads === 2 ? pending.promise : { messages: [{ id: 'two', body: 'A later report' }], nextCursor: null, nextAfter: 9 });
  await wire.open(); const original = wire.element('historyMessages').children[0], checking = wire.click('historyNewer');
  assert.equal(wire.element('historyNewer').disabled, true); assert.match(wire.element('historyPageNote').textContent, /Checking newer/);
  pending.reject(new Error('Local history temporarily unavailable.')); await checking;
  assert.equal(wire.element('historyMessages').children[0], original); assert.equal(wire.run('historyAfter'), 7); assert.equal(wire.element('historyNewer').disabled, false);
  assert.match(wire.element('historyError').textContent, /temporarily unavailable/); assert.deepEqual(wire.errors, []);
  await wire.click('historyNewer'); assert.match(wire.calls[2].path, /cursor=7$/); assert.equal(wire.run('historyAfter'), 9);
});

test('late newer-history responses cannot reopen a closed dialog or overwrite a reopened session', async () => {
  const stale = deferred(), fresh = deferred(); let reads = 0;
  const wire = harness(() => ++reads === 1 ? { messages: [{ id: 'one', body: 'Old session' }], nextCursor: null, nextAfter: 5 }
    : reads === 2 ? stale.promise : fresh.promise);
  await wire.open(); const previous = wire.click('historyNewer'); wire.run('dismissHistory()');
  assert.equal(wire.calls[1].signal.aborted, true); const current = wire.open();
  stale.resolve({ messages: [{ id: 'obsolete', body: 'Never displayed' }], nextCursor: null, nextAfter: 99 }); await previous;
  assert.equal(wire.run('historyAfter'), 0); assert.equal(wire.element('historyMessages').children.length, 0); assert.equal(wire.run('historyBusy'), true); assert.equal(wire.run('historyLoaded'), false);
  fresh.resolve({ messages: [{ id: 'fresh', body: 'Current complete report' }], nextCursor: null, nextAfter: 10 }); await current;
  assert.equal(wire.run('historyAfter'), 10); assert.equal(wire.run('historyLoaded'), true); assert.equal(wire.element('historyMessages').children[0].id, 'fresh'); assert.equal(wire.element('historyDialog').open, true); assert.equal(wire.element('historyError').hidden, true);
});

test('an initial history failure offers an explicit retry of the original URL before newer-message checks', async () => {
  const initial = deferred(), retry = deferred(); let reads = 0;
  const wire = harness(() => ++reads === 1 ? initial.promise : reads === 2 ? retry.promise
    : { messages: [{ id: 'new', body: 'A message added after the initial read.' }], nextCursor: null, nextAfter: 18 });
  const loading = wire.open();
  assert.equal(wire.run('historyLoaded'), false); assert.equal(wire.element('historyNewer').hidden, true);
  initial.reject(new Error('The first local read failed.')); await loading;
  assert.equal(wire.element('historyNewer').hidden, false); assert.equal(wire.element('historyNewer').textContent, 'Retry loading history');
  assert.match(wire.element('historyPageNote').textContent, /Retry loading history/); assert.equal(wire.element('historyPageNote').textContent.includes('Previously loaded'), false);
  const reloading = wire.click('historyNewer');
  assert.equal(wire.calls[1].path, wire.calls[0].path); assert.equal(wire.calls[1].path.includes('cursor='), false);
  assert.equal(wire.element('historyNewer').hidden, true); assert.equal(wire.run('historyLoaded'), false);
  retry.resolve({ messages: [{ id: 'old', body: 'Complete previously stored history.' }], nextCursor: null, nextAfter: 17 }); await reloading;
  assert.equal(wire.run('historyLoaded'), true); assert.equal(wire.element('historyNewer').textContent, 'Check newer messages');
  assert.match(wire.element('historyPageNote').textContent, /Showing 1 complete messages/); assert.equal(wire.element('historyPageNote').textContent.includes('newer'), false);
  await wire.click('historyNewer'); assert.match(wire.calls[2].path, /cursor=17$/); assert.match(wire.element('historyPageNote').textContent, /Loaded 1 newer messages/);
  assert.equal(wire.element('messageBody').value, 'Unsent room guidance'); assert.equal(wire.element('taskBrief').value, 'Unsent task acceptance criteria');
  assert.equal(wire.calls.length, 3, 'Every read must come from an explicit open, retry or newer-message check.');
});

test('an accepted empty history page remains a successful read when later checks fail and retry', async () => {
  const failed = deferred(); let reads = 0;
  const wire = harness(() => ++reads === 1 ? { messages: [], nextCursor: null, nextAfter: 0 } : reads === 2 ? failed.promise
    : { messages: [{ id: 'first', body: 'First message after an empty read.' }], nextCursor: null, nextAfter: 1 });
  await wire.open(); assert.equal(wire.run('historyLoaded'), true); assert.equal(wire.element('historyNewer').textContent, 'Check newer messages');
  const checking = wire.click('historyNewer'); failed.reject(new Error('Later check temporarily unavailable.')); await checking;
  assert.equal(wire.run('historyLoaded'), true); assert.equal(wire.element('historyMessages').children.length, 0);
  assert.equal(wire.element('historyNewer').textContent, 'Check newer messages'); assert.match(wire.element('historyPageNote').textContent, /current place are preserved/);
  await wire.click('historyNewer'); assert.equal(wire.calls[1].path, wire.calls[2].path); assert.match(wire.calls[2].path, /cursor=0$/);
  assert.equal(wire.run('historyLoaded'), true); assert.equal(wire.element('historyMessages').children[0].id, 'first');
});

test('a failed continuation page retains its exact cursor, complete loaded nodes and scroll', async () => {
  const failed = deferred(); let reads = 0;
  const wire = harness(() => ++reads === 1 ? { messages: [{ id: 'one', body: 'Retained first page.' }], nextCursor: 7, nextAfter: 7 }
    : reads === 2 ? failed.promise : { messages: [{ id: 'two', body: 'Complete second page.' }], nextCursor: null, nextAfter: 9 });
  await wire.open(); const original = wire.element('historyMessages').children[0]; wire.element('historyMessages').scrollTop = 37;
  const reading = wire.click('historyNext'); failed.reject(new Error('Continuation read temporarily unavailable.')); await reading;
  assert.equal(wire.run('historyLoaded'), true); assert.equal(wire.run('historyCursor'), 7); assert.equal(wire.element('historyMessages').children[0], original);
  assert.equal(wire.element('historyNext').disabled, false); assert.equal(wire.element('historyNewer').hidden, true);
  await wire.click('historyNext'); assert.equal(wire.calls[1].path, wire.calls[2].path); assert.match(wire.calls[2].path, /cursor=7$/);
  assert.equal(wire.element('historyMessages').children[0], original); assert.equal(wire.element('historyMessages').children.length, 2);
  assert.equal(wire.element('historyMessages').scrollTop, 37); assert.equal(wire.element('historyNewer').textContent, 'Check newer messages');
  assert.equal(wire.element('messageBody').value, 'Unsent room guidance'); assert.equal(wire.element('taskBrief').value, 'Unsent task acceptance criteria');
});
