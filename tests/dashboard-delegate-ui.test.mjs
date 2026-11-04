import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext } from 'node:vm';
import { dashboardHtml } from '../dist/dashboard-ui.js';
import { delegateClientSource } from '../dist/dashboard-delegate-ui.js';

function deferred() { let resolve, reject; const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; }); return { promise, resolve, reject }; }
const turn = () => new Promise(resolve => setImmediate(resolve));
function configuration(revision = 1, changes = {}) { return { mode: 'agent', delegate: 'Chief of staff', mandate: 'Coordinate the board and escalate decisions.', roomIds: ['room-a'], revision, updatedAt: '2026-11-04T12:00:00Z', ...changes }; }
function report(id, changes = {}) { return { id, revision: 1, kind: 'decision', author: 'Chief of staff', body: 'Complete evidence for ' + id, createdAt: '2026-11-04T12:00:00Z', observedRooms: [{ id: 'room-a', revision: 7, instructionRevision: 2, status: 'ready' }], ...changes }; }
function harness(handler = path => path === '/api/delegation' ? configuration() : { reports: [], nextCursor: null }) {
  const nodes = new Map(), closes = [], calls = [];
  class Node {
    constructor(tag) { this.tag = tag; this.children = []; this.listeners = new Map(); this.textContent = ''; this.value = ''; this.hidden = false; this.open = false; this.disabled = false; this.checked = false; }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    setAttribute(key, value) { this[key] = value; }
    addEventListener(event, handler) { if (!this.listeners.has(event)) this.listeners.set(event, []); this.listeners.get(event).push(handler); }
    fire(event) { return Promise.all((this.listeners.get(event) ?? []).map(handler => handler({ preventDefault() {} }))); }
    showModal() { this.open = true; }
    close() { if (this.open) { this.open = false; closes.push(() => this.fire('close')); } }
    escape() { this.fire('cancel'); this.close(); }
    focus() {}
  }
  const $ = id => { if (!nodes.has(id)) nodes.set(id, new Node(id)); return nodes.get(id); };
  $('delegateOpenOnly').checked = true; $('delegateReportAuthor').value = 'Human';
  const context = createContext({ Promise, AbortController, encodeURIComponent, $,
    element: (tag, cls, content) => { const node = new Node(tag); node.className = cls ?? ''; if (content !== undefined) node.textContent = String(content); return node; },
    api: async (path, body, signal) => { calls.push({ path, body, signal }); return await handler(path, body, signal); },
    room: { id: 'room-a' }, roomLoading: false, selected: 'room-a',
  });
  runInContext(delegateClientSource() + '\ninstallDelegateUI();', context);
  return { $, nodes, calls, context, run: source => runInContext(source, context), open: () => runInContext('openDelegate()', context),
    edit(id, value) { $(id).value = value; return $('delegateForm').fire('input'); },
    async flushClose() { for (const close of closes.splice(0)) await close(); },
  };
}
function child(node, tag) { if (node.tag === tag) return node; for (const value of node.children) { const found = child(value, tag); if (found) return found; } }

test('human delegate controls and the actual client are included in the shipped English dashboard', () => {
  const html = dashboardHtml('delegate-ui-test');
  assert.match(html, /id="humanDelegate"/); assert.match(html, /id="delegateDialog"/);
  assert.match(html, /Mandate and escalation rules/); assert.match(html, /Executive inbox/);
  assert.ok(html.includes(delegateClientSource()));
});

test('delegate overview updates the executive badge and announces changes without overwriting drafts or issuing requests', async () => {
  const wire = harness(); await wire.open();
  wire.run('renderDelegateOverview({openQuestions:0,reportRevision:1,configurationRevision:1})');
  await wire.edit('delegateMandate', 'Unsubmitted executive mandate.');
  const reads = wire.calls.length;
  wire.run('renderDelegateOverview({openQuestions:3,reportRevision:2,configurationRevision:1})');
  assert.equal(wire.$('humanDelegate').textContent, 'Human delegate · 3 open');
  assert.equal(wire.$('delegateUpdates').hidden, false); assert.match(wire.$('delegateUpdates').textContent, /Refresh reports or reload/);
  assert.equal(wire.$('delegateMandate').value, 'Unsubmitted executive mandate.'); assert.equal(wire.calls.length, reads);
  wire.run('dismissDelegate();renderDelegateOverview({openQuestions:0,reportRevision:3,configurationRevision:2})');
  assert.equal(wire.$('humanDelegate').textContent.trim(), 'Human delegate'); assert.equal(wire.calls.length, reads);
});

test('an older mandate load cannot replace a newer response and regress its CAS revision', async () => {
  const old = deferred(), current = deferred(); let reads = 0;
  const wire = harness(path => path === '/api/delegation' ? ++reads === 1 ? old.promise : current.promise : { reports: [], nextCursor: null });
  const opening = wire.open(), reloading = wire.run('delegateLoadConfig(delegateSession)');
  current.resolve(configuration(2, { mandate: 'Current executive mandate.' })); await reloading;
  old.resolve(configuration(1, { mandate: 'Obsolete executive mandate.' })); await opening;
  assert.equal(wire.run('delegateConfig.revision'), 2); assert.equal(wire.$('delegateMandate').value, 'Current executive mandate.');
});

test('a delayed native close event cannot invalidate a reopened human delegate session', async () => {
  const old = deferred(), current = deferred(); let reads = 0;
  const wire = harness(path => path === '/api/delegation' ? ++reads === 1 ? old.promise : current.promise : { reports: [], nextCursor: null });
  const opening = wire.open(); wire.run('dismissDelegate()'); const reopening = wire.open(), session = wire.run('delegateSession');
  await wire.flushClose();
  assert.equal(wire.run('delegateSession'), session); assert.equal(wire.calls.filter(call => call.path === '/api/delegation')[1].signal.aborted, false);
  current.resolve(configuration(2)); await reopening; old.reject(new Error('Obsolete closed read error.')); await opening;
  assert.equal(wire.$('delegateDialog').open, true); assert.equal(wire.run('delegateConfig.revision'), 2);
  assert.equal(wire.$('delegateError').hidden, true); assert.equal(wire.$('delegateSave').disabled, false);
});

test('a direct native close and Escape abort the delegate reads and reject late state', async () => {
  for (const escape of [false, true]) {
    const pending = deferred(), wire = harness(() => pending.promise), opening = wire.open();
    if (escape) wire.$('delegateDialog').escape(); else wire.$('delegateDialog').close();
    await wire.flushClose(); assert.ok(wire.calls.every(call => call.signal.aborted));
    pending.resolve(configuration(2)); await opening;
    assert.equal(wire.$('delegateDialog').open, false); assert.equal(wire.run('delegateConfig'), undefined);
    assert.equal(wire.$('delegateReports').children.length, 0);
  }
});

test('mandate loads preserve edits made during the request and saves preserve newer unsaved edits', async () => {
  const loading = deferred(), saving = deferred();
  const wire = harness((path, body) => path !== '/api/delegation' ? { reports: [], nextCursor: null } : body ? saving.promise : loading.promise);
  const opening = wire.open(); await wire.edit('delegateMandate', 'Draft written while loading.');
  loading.resolve(configuration(3)); await opening;
  assert.equal(wire.$('delegateMandate').value, 'Draft written while loading.'); assert.equal(wire.run('delegateConfig.revision'), 3);
  const save = wire.$('delegateForm').fire('submit');
  assert.equal(wire.calls.find(call => call.body).body.expectedRevision, 3);
  assert.equal(wire.calls.find(call => call.body).body.mandate, 'Draft written while loading.');
  await wire.edit('delegateMandate', 'Newer unsaved draft.');
  saving.resolve(configuration(4, { mandate: 'Draft written while loading.' })); await save;
  assert.equal(wire.$('delegateMandate').value, 'Newer unsaved draft.'); assert.equal(wire.run('delegateConfig.revision'), 4);
  assert.match(wire.$('delegateStatus').textContent, /newer edits remain unsaved/);
});

test('a failed stale mandate save retains the draft and does not retry with a newer revision silently', async () => {
  const wire = harness((path, body) => path !== '/api/delegation' ? { reports: [], nextCursor: null } : body ? Promise.reject(new Error('Delegation revision conflict: current revision is 8. Reload before saving.')) : configuration(3));
  await wire.open(); await wire.edit('delegateMandate', 'Keep this executive draft.'); await wire.$('delegateForm').fire('submit');
  assert.equal(wire.$('delegateMandate').value, 'Keep this executive draft.'); assert.equal(wire.run('delegateConfig.revision'), 3);
  assert.equal(wire.$('delegateError').hidden, false); assert.match(wire.$('delegateError').textContent, /revision conflict/);
  assert.equal(wire.calls.filter(call => call.body).length, 1); assert.equal(wire.$('delegateSave').disabled, false);
});

test('a failed report filter reset cannot reuse a previous filter cursor and skip complete reports', async () => {
  let reads = 0;
  const wire = harness(path => path === '/api/delegation' ? configuration() : ++reads === 1 ? { reports: [report('old-open')], nextCursor: 41 }
    : reads === 2 ? Promise.reject(new Error('Offline filtered-page failure.')) : { reports: [report('first-all', { kind: 'update' })], nextCursor: null });
  await wire.open(); assert.equal(wire.run('delegateReportsCursor'), 41);
  wire.$('delegateOpenOnly').checked = false; await wire.$('delegateOpenOnly').fire('change'); await turn();
  assert.equal(wire.run('delegateReportsCursor'), null); assert.equal(wire.$('delegateMoreReports').hidden, true);
  await wire.run('delegateLoadReports(false)');
  assert.ok(wire.calls.at(-1).path.includes('openOnly=false')); assert.ok(!wire.calls.at(-1).path.includes('cursor='));
  assert.equal(wire.$('delegateReports').children.length, 1); assert.match(wire.$('delegateReportsStatus').textContent, /All matching reports/);
});

test('late report pages from a previous filter cannot replace current cards or expose obsolete errors', async () => {
  const old = deferred(), current = deferred(); let reads = 0;
  const wire = harness(path => path === '/api/delegation' ? configuration() : ++reads === 1 ? old.promise : current.promise);
  const opening = wire.open(); wire.$('delegateOpenOnly').checked = false; await wire.$('delegateOpenOnly').fire('change');
  current.resolve({ reports: [report('current')], nextCursor: null }); await turn();
  old.reject(new Error('Obsolete filter error.')); await opening;
  assert.equal(wire.$('delegateReports').children.length, 1); assert.equal(child(wire.$('delegateReports').children[0], 'p').textContent, 'Complete evidence for current');
  assert.equal(wire.$('delegateError').hidden, true); assert.equal(wire.run('delegateReportsBusy'), false);
});

test('resolving one executive question preserves draft replies to other complete reports', async () => {
  let reads = 0;
  const wire = harness((path, body) => path === '/api/delegation' ? configuration() : body ? { ...report('first'), resolution: { author: body.author, body: body.body } }
    : ++reads === 1 ? { reports: [report('first'), report('second')], nextCursor: null } : { reports: [report('second')], nextCursor: null });
  await wire.open();
  const [first, second] = wire.$('delegateReports').children;
  child(first, 'textarea').value = 'Explicit reply to the first decision.';
  child(second, 'textarea').value = 'Unsubmitted reply to the second decision.';
  await child(second, 'textarea').fire('input'); await child(first, 'form').fire('submit');
  assert.equal(wire.$('delegateReports').children.length, 1);
  assert.equal(child(wire.$('delegateReports').children[0], 'textarea').value, 'Unsubmitted reply to the second decision.');
  const write = wire.calls.find(call => call.path.endsWith('/first/resolve'));
  assert.equal(write.body.expectedRevision, 1); assert.equal(write.body.author, 'Human');
});

test('a reply saved during an older inbox refresh supersedes that read and shows the current resolved state', async () => {
  const stale = deferred(); let reads = 0;
  const wire = harness((path, body) => path === '/api/delegation' ? configuration() : body ? { ...report('decision'), resolution: { author: body.author, body: body.body } }
    : ++reads === 1 ? { reports: [report('decision')], nextCursor: null } : reads === 2 ? stale.promise : { reports: [], nextCursor: null });
  await wire.open(); const card = wire.$('delegateReports').children[0], refreshing = wire.run('delegateLoadReports(false)');
  child(card, 'textarea').value = 'Current executive reply.'; await child(card, 'textarea').fire('input'); await child(card, 'form').fire('submit');
  stale.resolve({ reports: [report('decision')], nextCursor: null }); await refreshing;
  assert.equal(wire.$('delegateReports').children.length, 0, 'An obsolete open report must not reappear after its reply commits.');
  assert.match(wire.$('delegateReportsStatus').textContent, /No matching reports/);
  assert.ok(reads >= 3, 'The successful reply must refresh the inbox after its commit.');
});

test('complete report bodies remain literal text and failed appended pages retain a retryable cursor', async () => {
  const body = '<script>untrusted executive evidence</script> ' + 'Complete report. '.repeat(10000); let reads = 0;
  const wire = harness(path => path === '/api/delegation' ? configuration() : ++reads === 1 ? { reports: [report('full', { body, kind: 'update' })], nextCursor: 23 }
    : reads === 2 ? Promise.reject(new Error('Append failed.')) : { reports: [report('last', { kind: 'completion' })], nextCursor: null });
  await wire.open(); assert.equal(child(wire.$('delegateReports').children[0], 'p').textContent, body);
  await wire.run('delegateLoadReports(true)'); assert.equal(wire.run('delegateReportsCursor'), 23); assert.equal(wire.$('delegateReports').children.length, 1);
  await wire.run('delegateLoadReports(true)'); assert.match(wire.calls.at(-1).path, /cursor=23/);
  assert.equal(wire.$('delegateReports').children.length, 2); assert.equal(wire.run('delegateReportsCursor'), null);
});
