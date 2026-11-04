import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext, Script } from 'node:vm';
import { RUN_DIALOG, runClientSource } from '../dist/dashboard-runs-ui.js';

function deferred() {
  let resolve, reject;
  const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

class Node {
  constructor(tag = '') {
    this.tag = tag; this.children = []; this.listeners = new Map(); this.textContent = '';
    this.value = ''; this.open = false; this.hidden = false; this.disabled = false; this.replacements = 0;
  }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; this.replacements++; }
  setAttribute(key, value) { this[key] = value; }
  addEventListener(key, listener) { this.listeners.set(key, listener); }
  showModal() { this.open = true; }
  close() { this.open = false; this.listeners.get('close')?.(); }
  remove() { this.removed = true; }
  click() { if (!this.disabled) return this.listeners.get('click')?.(); }
}

function record(id = 'run-a') {
  return { id, status: 'running', createdAt: '2026-11-04T12:00:00Z', updatedAt: '2026-11-04T12:01:00Z',
    input: { mode: 'delegate', task: 'Keep the complete task.', agents: ['worker'], permission: 'read' },
    output: 'Complete stored workflow output.', steps: [] };
}

function event(sequence, changes = {}) {
  return { sequence, runId: 'run-a', type: 'agent.output', data: { text: 'Complete event ' + sequence },
    createdAt: '2026-11-04T12:01:00Z', ...changes };
}

function page(events, hasMore = false, after = 0) {
  const nextAfter = events.at(-1)?.sequence ?? after;
  return { events, nextAfter, hasMore, nextCursor: hasMore ? nextAfter : null };
}

function harness(handler = async path => record(path.split('/').at(-1))) {
  const nodes = new Map(), calls = [], downloads = [], revoked = [], timers = [];
  const node = id => { if (!nodes.has(id)) nodes.set(id, new Node(id)); return nodes.get(id); };
  const context = createContext({ Promise, AbortController, encodeURIComponent, Blob,
    $: node,
    element: (tag, cls, text) => { const value = new Node(tag); value.className = cls ?? ''; if (text !== undefined) value.textContent = String(text); return value; },
    labels: { completed: 'Completed', failed: 'Failed', cancelled: 'Cancelled', running: 'Running' },
    api: (path, _body, signal) => { calls.push({ path, signal }); return handler(path, signal); },
    URL: { createObjectURL: blob => { downloads.push(blob); return 'blob:fixture'; }, revokeObjectURL: url => revoked.push(url) },
    document: { body: new Node('body') }, window: new Node('window'), setTimeout: callback => timers.push(callback),
  });
  runInContext(runClientSource() + '\ninstallRunUI();', context);
  return { context, calls, nodes, node, downloads, revoked, timers, run: source => runInContext(source, context),
    open: (id = 'run-a') => runInContext('openWorkflow(' + JSON.stringify(id) + ')', context),
    load: (apply = true) => runInContext('loadWorkflowEvents(' + apply + ')', context),
    filter(type) { node('workflowEventType').value = type; node('workflowEventType').listeners.get('input')(); },
  };
}

test('timeline remains an explicit read and has accessible, valid browser controls', async () => {
  const wire = harness(); await wire.open();
  assert.equal(wire.calls.length, 1); assert.equal(wire.calls[0].path, '/api/runs/run-a');
  assert.match(wire.node('workflowEventsStatus').textContent, /has not been loaded/);
  assert.equal(wire.node('workflowEventsApply').disabled, false);
  assert.equal(wire.node('workflowEventsMore').hidden, true); assert.equal(wire.node('workflowEventsRefresh').hidden, true);
  assert.match(RUN_DIALOG, /aria-labelledby="workflowEventsTitle"/);
  assert.match(RUN_DIALOG, /id="workflowEventsStatus" role="status"/);
  assert.match(RUN_DIALOG, /Type matching is exact and case-sensitive/);
  assert.doesNotThrow(() => new Script(runClientSource()));
});

test('timeline appends every complete event using sequence cursors even when pages are short', async () => {
  const complete = '<script>literal, never executable</script>\n' + 'Complete event evidence. '.repeat(12000);
  let reads = 0;
  const wire = harness(async path => !path.includes('/events?') ? record() : ++reads === 1
    ? page([event(8, { data: { text: complete } })], true)
    : page([event(52), event(81)]));
  await wire.open(); await wire.load();
  const list = wire.node('workflowEventsList'), first = list.children[0], replacements = list.replacements;
  assert.equal(JSON.parse(first.children[1].textContent).text, complete); assert.equal(first.tag, 'details');
  assert.equal(first.children[1].tag, 'pre'); assert.equal(first.children[1].textContent, JSON.stringify({ text: complete }, null, 2));
  first.open = true;
  assert.equal(wire.node('workflowEventsMore').disabled, false);
  assert.equal(wire.node('workflowEventsRefresh').disabled, true);
  await wire.load(false);
  assert.equal(wire.calls[1].path, '/api/runs/run-a/events?after=0&limit=100');
  assert.equal(wire.calls[2].path, '/api/runs/run-a/events?after=8&limit=100');
  assert.equal(list.children.length, 3); assert.equal(list.children[0], first); assert.equal(first.open, true);
  assert.equal(list.replacements, replacements); assert.equal(wire.run('workflowEventsAfter'), 81);
  assert.equal(wire.node('workflowEventsMore').hidden, true); assert.equal(wire.node('workflowEventsRefresh').disabled, false);
  assert.match(wire.node('workflowEventsStatus').textContent, /3 complete events/);
  assert.match(wire.node('workflowEventsStatus').textContent, /at the last read/);
  assert.equal(runClientSource().includes('.innerHTML'), false);
});

test('newer checks retain loaded history and allow later events after an empty read', async () => {
  let reads = 0;
  const wire = harness(async path => !path.includes('/events?') ? record() : ++reads === 1
    ? page([event(4)]) : reads === 2 ? page([], false, 4) : page([event(105)]));
  await wire.open(); await wire.load(); const first = wire.node('workflowEventsList').children[0];
  await wire.load(false); assert.equal(wire.run('workflowEventsAfter'), 4);
  await wire.load(false);
  assert.equal(wire.calls[2].path, '/api/runs/run-a/events?after=4&limit=100');
  assert.equal(wire.calls[3].path, '/api/runs/run-a/events?after=4&limit=100');
  assert.equal(wire.node('workflowEventsList').children[0], first); assert.equal(wire.node('workflowEventsList').children.length, 2);
});

test('an empty filtered page does not claim the workflow ended and preserves exact type matching', async () => {
  const type = ' Tool.Output/<script> ';
  const wire = harness(async path => path.includes('/events?') ? page([]) : record());
  await wire.open(); wire.filter(type); await wire.load();
  assert.equal(wire.calls[1].path, '/api/runs/run-a/events?after=0&limit=100&type=' + encodeURIComponent(type));
  assert.equal(wire.node('workflowEventType').value, type);
  assert.match(wire.node('workflowEventsStatus').textContent, /No matching events were stored at the last read/);
  assert.equal(wire.node('workflowEventsRefresh').disabled, false); assert.equal(wire.run('workflowEventsAfter'), 0);
});

test('changing a filter aborts and ignores its pending page, then starts the new type at zero', async () => {
  const pending = deferred(); let reads = 0;
  const wire = harness(async path => !path.includes('/events?') ? record() : ++reads === 1
    ? page([event(4)], true) : reads === 2 ? pending.promise : page([event(88, { type: 'run.completed' })]));
  await wire.open(); await wire.load(); const old = wire.load(false);
  wire.filter('run.completed'); assert.equal(wire.calls[2].signal.aborted, true);
  assert.equal(wire.node('workflowEventsMore').disabled, true);
  assert.match(wire.node('workflowEventsStatus').textContent, /Filter changed/);
  await wire.load(); const current = wire.node('workflowEventsList').children[0];
  pending.resolve(page([event(9)], true)); await old;
  assert.equal(wire.calls[3].path, '/api/runs/run-a/events?after=0&limit=100&type=run.completed');
  assert.equal(wire.node('workflowEventsList').children.length, 1); assert.equal(wire.node('workflowEventsList').children[0], current);
  assert.equal(wire.run('workflowEventsAfter'), 88); assert.equal(wire.node('workflowEventsError').hidden, true);
});

test('stale filter failures cannot replace a newer filter result, including the original type again', async () => {
  const oldPage = deferred(); let reads = 0;
  const wire = harness(async path => !path.includes('/events?') ? record() : ++reads === 1 ? oldPage.promise : page([event(12)]));
  await wire.open(); const old = wire.load(); wire.filter('run.failed'); wire.filter('');
  await wire.load(false); assert.equal(reads, 2);
  oldPage.reject(new Error('Obsolete filter failure.')); await old;
  assert.equal(wire.node('workflowEventsError').hidden, true); assert.equal(wire.run('workflowEventsCount'), 1);
});

test('closing and reopening the same workflow rejects late pages and errors without reopening', async () => {
  for (const fail of [false, true]) {
    const pending = deferred(); let reads = 0;
    const wire = harness(async path => !path.includes('/events?') ? record() : ++reads === 1 ? pending.promise : page([event(77)]));
    await wire.open(); const old = wire.load(); wire.node('closeWorkflow').click();
    assert.equal(wire.calls[1].signal.aborted, true); assert.equal(wire.node('workflowDialog').open, false);
    await wire.open(); await wire.load();
    if (fail) pending.reject(new Error('Old closed timeline error.')); else pending.resolve(page([event(9)], true));
    await old;
    assert.equal(wire.run('workflowEventsCount'), 1); assert.equal(wire.run('workflowEventsAfter'), 77);
    assert.equal(wire.node('workflowEventsError').hidden, true); assert.equal(wire.node('workflowDialog').open, true);
  }
});

test('switching workflows resets timeline state and rejects late events from the prior workflow', async () => {
  const pending = deferred();
  const wire = harness(async path => path.includes('/events?') ? pending.promise : record(path.split('/').at(-1)));
  await wire.open(); const old = wire.load(); await wire.open('run-b');
  assert.equal(wire.calls[1].signal.aborted, true); assert.equal(wire.run('workflowEventsStarted'), false);
  assert.equal(wire.node('workflowEventsList').children.length, 0);
  pending.resolve(page([event(42)])); await old;
  assert.equal(wire.run('workflowId'), 'run-b'); assert.equal(wire.node('workflowEventsList').children.length, 0);
  assert.equal(wire.node('workflowEventsError').hidden, true);
});

test('snapshot reload retains loaded timeline and its cursor while aborting an outstanding page', async () => {
  const pending = deferred(); let reads = 0;
  const wire = harness(async path => !path.includes('/events?') ? record() : ++reads === 1
    ? page([event(6)], true) : reads === 2 ? pending.promise : page([event(18)]));
  await wire.open(); await wire.load(); const first = wire.node('workflowEventsList').children[0], old = wire.load(false);
  await wire.open(); assert.equal(wire.calls[2].signal.aborted, true);
  assert.equal(wire.node('workflowEventsList').children[0], first); assert.equal(wire.run('workflowEventsAfter'), 6);
  pending.reject(new Error('Obsolete pre-reload error.')); await old;
  assert.equal(wire.node('workflowEventsError').hidden, true);
  await wire.load(false);
  assert.equal(wire.calls[4].path, '/api/runs/run-a/events?after=6&limit=100');
  assert.equal(wire.node('workflowEventsList').children[0], first); assert.equal(wire.run('workflowEventsCount'), 2);
});

test('page errors preserve loaded evidence and the retry cursor; pruning is explicit', async () => {
  let reads = 0;
  const wire = harness(async path => {
    if (!path.includes('/events?')) return record();
    if (++reads === 1) return page([event(7)], true);
    if (reads === 2) throw Object.assign(new Error('Missing'), { status: 404 });
    return page([event(31)]);
  });
  await wire.open(); await wire.load(); const first = wire.node('workflowEventsList').children[0];
  await wire.load(false);
  assert.match(wire.node('workflowEventsError').textContent, /pruned.*Previously loaded events remain visible/);
  assert.equal(wire.run('workflowEventsAfter'), 7); assert.equal(wire.node('workflowEventsMore').disabled, false);
  await wire.load(false);
  assert.equal(wire.calls[3].path, '/api/runs/run-a/events?after=7&limit=100');
  assert.equal(wire.node('workflowEventsList').children[0], first); assert.equal(wire.node('workflowEventsError').hidden, true);
});

test('duplicate reads and invalid ordering cannot duplicate or silently skip retained evidence', async () => {
  const pending = deferred(); let reads = 0;
  const wire = harness(async path => !path.includes('/events?') ? record() : ++reads === 1 ? pending.promise : page([event(3), event(2)]));
  await wire.open(); const first = wire.load(); await wire.load(); assert.equal(reads, 1);
  pending.resolve(page([event(1)], true)); await first;
  await wire.load(false);
  assert.equal(wire.run('workflowEventsCount'), 1); assert.equal(wire.run('workflowEventsAfter'), 1);
  assert.equal(wire.node('workflowEventsList').children.length, 1);
  assert.match(wire.node('workflowEventsError').textContent, /inconsistent event ordering/);
});

test('timeline refuses a cursor beyond the returned evidence without advancing past unread events', async () => {
  const wire = harness(async path => path.includes('/events?')
    ? { events: [event(3)], nextAfter: 100, nextCursor: null, hasMore: false } : record());
  await wire.open(); await wire.load();
  assert.equal(wire.run('workflowEventsAfter'), 0); assert.equal(wire.run('workflowEventsCount'), 0);
  assert.equal(wire.node('workflowEventsList').children.length, 0);
  assert.match(wire.node('workflowEventsError').textContent, /cursor beyond the last event/);
});

test('snapshot JSON download stays exact and separate from loaded event pages', async () => {
  const full = record();
  const wire = harness(async path => path.includes('/events?') ? page([event(3)]) : full);
  await wire.open(); await wire.load(); wire.node('downloadWorkflow').click();
  assert.deepEqual(JSON.parse(await wire.downloads[0].text()), full);
  assert.equal(wire.revoked.length, 0); wire.timers[0](); assert.deepEqual(wire.revoked, ['blob:fixture']);
  assert.equal(wire.calls.length, 2);
});

test('pagehide aborts pending timeline reads and leaves no controller or reopened view', async () => {
  const pending = deferred(), wire = harness(async path => path.includes('/events?') ? pending.promise : record());
  await wire.open(); const loading = wire.load(); wire.context.window.listeners.get('pagehide')();
  assert.equal(wire.calls[1].signal.aborted, true); pending.resolve(page([event(100)])); await loading;
  assert.equal(wire.run('workflowEventsController'), undefined); assert.equal(wire.run('workflowEventsLoading'), false);
  assert.equal(wire.node('workflowEventsList').children.length, 0); assert.equal(wire.run('workflowId'), null);
});
