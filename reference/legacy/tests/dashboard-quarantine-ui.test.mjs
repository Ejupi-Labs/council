import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext } from 'node:vm';
import { dashboardHtml } from '../dist/dashboard-ui.js';
import { QUARANTINE_PERSISTENCE_WARNING } from '../dist/dispatch-quarantine.js';

const html = dashboardHtml('quarantine-ui-fixture');
const source = /<script nonce="[^"]+">([\s\S]*?)<\/script>/.exec(html)[1];
const client = /\/\* QUARANTINE_CLIENT_BEGIN \*\/([\s\S]*?)\/\* QUARANTINE_CLIENT_END \*\//.exec(source)[1];
const incident = (index, extra = {}) => ({ id: 'incident-' + index, generation: String(index), reportKey: 'attempt-' + index,
  instanceId: 'owned-host', stage: 'agent_call', reason: 'process_cleanup_unverified', observedAt: '2026-11-04T00:00:00.000Z', ...extra });
const page = (revision, incidents = [], nextCursor = null, count = incidents.length) => ({ blocked: count > 0,
  currentRevision: String(revision), openCount: count, incidents, nextCursor });
const hostA = '00000000-0000-4000-8000-000000000001', hostB = '00000000-0000-4000-8000-000000000002';
const localStatus = (hostId, localRevision, shared = 0, count = 0, extra = {}) => ({ blocked: count > 0,
  currentRevision: String(shared), openCount: count, hostId, localRevision: String(localRevision), ...extra });
function deferred() { let resolve, reject; const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; }); return { promise, resolve, reject }; }

function harness(handler = () => page(0)) {
  const elements = new Map(), calls = [], workflows = [], windowEvents = new Map();
  function node(tag = 'div', className = '', text = '') {
    let ownText = text;
    const value = { tagName: tag, className, children: [], listeners: new Map(), attributes: new Map(), hidden: false,
      disabled: false, open: false, scrollTop: 0, value: '', title: '', replacements: 0,
      append(...children) { this.children.push(...children); },
      replaceChildren(...children) { this.children = children; this.replacements++; },
      addEventListener(name, callback) { this.listeners.set(name, callback); },
      setAttribute(name, value) { this.attributes.set(name, value); },
      showModal() { this.open = true; },
      close() { this.open = false; this.listeners.get('close')?.(); },
      click() { if (!this.disabled && !this.hidden) return this.listeners.get('click')?.(); },
    };
    Object.defineProperty(value, 'textContent', { get() { return ownText + this.children.map(child => child.textContent).join(''); }, set(text) { ownText = String(text); this.children = []; } });
    Object.defineProperty(value, 'innerHTML', { set() { assert.fail('Incident metadata must never be inserted as HTML.'); } });
    return value;
  }
  const element = id => { if (!elements.has(id)) elements.set(id, node()); return elements.get(id); };
  const context = createContext({ Promise, Map, AbortController, encodeURIComponent, $: element, element: node,
    window: { addEventListener(name, callback) { windowEvents.set(name, callback); } },
    api(path, body, signal) { calls.push({ path, body, signal }); return Promise.resolve(handler(path, calls.length, signal)); },
    openWorkflow: id => workflows.push(id),
  });
  runInContext("var token='offline';", context);
  const link = source.split(/\r?\n/).find(line => line.startsWith('function workflowLink('));
  assert.ok(link, 'Use the actual shipped View workflow control.'); runInContext(link + '\n' + client + '\ninstallQuarantineUI();', context);
  element('messageBody').value = 'Unsent project guidance'; element('taskBrief').value = 'Unsent acceptance checks';
  return { context, calls, workflows, element, run: code => runInContext(code, context),
    apply: value => runInContext('applyQuarantineReceipt(' + JSON.stringify(value) + ')', context),
    click: id => element(id).click(), open: () => element('quarantineStatus').click(), text: id => element(id).textContent };
}

test('the shipped quarantine receipt updates the banner without reading incidents or offering acknowledgement controls', () => {
  const wire = harness(), revision = '9007199254740993';
  assert.equal(wire.element('quarantineBanner').hidden, true);
  wire.apply({ blocked: true, currentRevision: revision, openCount: 2 });
  assert.equal(wire.calls.length, 0); assert.equal(wire.element('quarantineBanner').hidden, false);
  assert.match(wire.text('quarantineBannerSummary'), /2 cleanup report/);
  assert.equal(wire.text('quarantineCommand'), 'council quarantine-ack --revision ' + revision + ' --reviewed');
  assert.match(html, /run this command from this project's directory/);
  assert.match(source, /if\(data\.quarantine\)applyQuarantineReceipt\(data\.quarantine\)/);
  assert.equal(/<button[^>]*>[^<]*[Aa]cknowledge/.test(html), false);
  wire.apply({ blocked: false, currentRevision: revision, openCount: 0 });
  wire.apply({ blocked: true, currentRevision: revision, openCount: 2 });
  wire.apply({ blocked: true, currentRevision: '1', openCount: 1 });
  assert.equal(wire.element('quarantineBanner').hidden, true, 'Stale receipts cannot reopen an acknowledged generation.');
  assert.equal(wire.text('quarantineCommand'), '');
});

test('a local fence with unverified persistence stays visible when no shared block is recorded', async () => {
  const wire = harness(() => page(0)), fault = '<svg onload="never()">Literal public host fault';
  wire.apply({ blocked: false, currentRevision: '0', openCount: 0, localFault: fault, persistenceWarning: QUARANTINE_PERSISTENCE_WARNING });
  assert.equal(wire.element('quarantineBanner').hidden, false); assert.match(wire.text('quarantineBannerTitle'), /This host/);
  assert.equal(wire.element('quarantineReview').hidden, true); assert.equal(wire.text('quarantineCommand'), '');
  assert.ok(wire.text('quarantineLocalFault').includes(fault)); assert.equal(wire.text('quarantinePersistenceWarning'), QUARANTINE_PERSISTENCE_WARNING);
  await wire.open();
  assert.ok(wire.text('quarantineLocalFault').includes(fault), 'A core-only page cannot erase the local host fence.');
  assert.equal(wire.element('quarantineRestart').hidden, false); assert.match(wire.text('quarantineSharedState'), /No shared dispatch block/);
  assert.equal(wire.element('quarantineReview').hidden, true); assert.equal(wire.calls.length, 1);
});

test('incident pages render literal safe metadata and reuse the shipped workflow viewer without private fields', async () => {
  const literal = '<img src=x onerror="never()">', runId = 'run/"<svg>', value = incident(1, { id: literal, reportKey: literal, instanceId: literal,
    stage: literal, reason: literal, runId, privateBody: 'PRIVATE_BODY_NOT_METADATA' });
  const wire = harness(() => page(1, [value])); await wire.open();
  assert.equal(wire.calls[0].path, '/api/quarantine?limit=25'); assert.equal(wire.calls[0].body, undefined);
  assert.ok(wire.text('quarantineIncidents').includes(literal)); assert.equal(wire.text('quarantineIncidents').includes('PRIVATE_BODY_NOT_METADATA'), false);
  wire.element('quarantineIncidents').children[0].children.at(-1).click(); assert.deepEqual(wire.workflows, [runId]);
  assert.equal(wire.element('messageBody').value, 'Unsent project guidance'); assert.equal(wire.element('taskBrief').value, 'Unsent acceptance checks');
});

test('status changes preserve loaded details and scroll until an explicit refresh', async () => {
  let reads = 0;
  const wire = harness(() => ++reads === 1 ? page(2, [incident(1)], '1', 2) : page(3, [incident(1), incident(2), incident(3)]));
  await wire.open(); const original = wire.element('quarantineIncidents').children[0], replacements = wire.element('quarantineIncidents').replacements;
  wire.element('quarantineBody').scrollTop = 47; wire.apply({ blocked: true, currentRevision: '3', openCount: 3 });
  assert.equal(wire.calls.length, 1); assert.equal(wire.element('quarantineIncidents').children[0], original);
  assert.equal(wire.element('quarantineIncidents').replacements, replacements); assert.equal(wire.element('quarantineBody').scrollTop, 47);
  assert.equal(wire.element('quarantineMore').disabled, true); assert.equal(wire.element('quarantineChanged').hidden, false);
  await wire.click('quarantineRefresh');
  assert.equal(wire.calls.length, 2); assert.equal(wire.calls[1].path, '/api/quarantine?limit=25');
  assert.equal(wire.element('quarantineChanged').hidden, true); assert.equal(wire.element('quarantineIncidents').children.length, 3);
  assert.equal(wire.element('quarantineBody').scrollTop, 47); assert.equal(wire.element('messageBody').value, 'Unsent project guidance');
});

test('failed next-page reads retain complete details and retry the same exact cursor', async () => {
  const failure = deferred();
  const wire = harness((_path, count) => count === 1 ? page(2, [incident(1)], '1', 2) : count === 2 ? failure.promise : page(2, [incident(2)], null, 2));
  await wire.open(); const original = wire.element('quarantineIncidents').children[0], reading = wire.click('quarantineMore');
  assert.equal(wire.element('quarantineMore').disabled, true); failure.reject(new Error('Temporary local read failure.')); await reading;
  assert.equal(wire.element('quarantineIncidents').children[0], original); assert.match(wire.text('quarantineError'), /Temporary local read failure/);
  assert.equal(wire.element('quarantineMore').disabled, false); await wire.click('quarantineMore');
  assert.equal(wire.calls[1].path, '/api/quarantine?limit=25&cursor=1'); assert.equal(wire.calls[2].path, wire.calls[1].path);
  assert.equal(wire.element('quarantineIncidents').children.length, 2); assert.equal(wire.element('quarantineIncidents').children[0], original);
  assert.equal(wire.element('quarantineMore').hidden, true); assert.equal(wire.element('quarantineError').hidden, true);
});

test('late close and reopen responses cannot replace the current dialog session', async () => {
  const old = deferred(), current = deferred();
  const wire = harness((_path, count) => count === 1 ? old.promise : current.promise);
  const previous = wire.open(); wire.click('quarantineClose'); assert.equal(wire.calls[0].signal.aborted, true);
  const fresh = wire.open(); old.resolve(page(99, [incident(99)])); await previous;
  assert.equal(wire.element('quarantineIncidents').children.length, 0); assert.equal(wire.run('quarantineBusy'), true);
  current.resolve(page(2, [incident(2)])); await fresh;
  assert.equal(wire.element('quarantineDialog').open, true); assert.match(wire.text('quarantineIncidents'), /Cleanup report #2/);
  assert.equal(wire.text('quarantineIncidents').includes('report #99'), false); assert.equal(wire.run('quarantineBusy'), false);
});

test('explicit refresh supersedes a pending page and older receipt responses preserve the loaded view', async () => {
  const oldPage = deferred(), refresh = deferred();
  const wire = harness((_path, count) => count === 1 ? page(2, [incident(1)], '1', 2) : count === 2 ? oldPage.promise : refresh.promise);
  await wire.open(); const previous = wire.click('quarantineMore'), fresh = wire.click('quarantineRefresh');
  assert.equal(wire.calls[1].signal.aborted, true); refresh.resolve(page(3, [incident(3)])); await fresh;
  const visible = wire.element('quarantineIncidents').children[0]; oldPage.resolve(page(2, [incident(2)])); await previous;
  assert.equal(wire.element('quarantineIncidents').children[0], visible); assert.match(wire.text('quarantineIncidents'), /report #3/);
  wire.apply({ blocked: true, currentRevision: '4', openCount: 4 });
  await wire.click('quarantineRefresh');
  assert.equal(wire.element('quarantineIncidents').children[0], visible); assert.equal(wire.element('quarantineChanged').hidden, false);
  assert.equal(wire.run('quarantineReceipt.currentRevision'), '4');
});

test('the shipped SSE quarantine handler updates only the receipt and releases its stream reader', async () => {
  const wire = harness(); let reads = 0, cancelled = 0, released = 0;
  const frame = 'event: quarantine\ndata: ' + JSON.stringify({ blocked: true, currentRevision: '7', openCount: 2 }) + '\n\n';
  const reader = { async read() { if (reads++ === 0) return { done: false, value: new TextEncoder().encode(frame) }; wire.context.token = ''; return { done: true }; },
    async cancel() { cancelled++; }, releaseLock() { released++; } };
  Object.assign(wire.context, { TextDecoder, fetch: async () => ({ ok: true, status: 200, body: { getReader: () => reader } }),
    refreshList: async () => {}, connection() {}, error(message) { assert.fail(message); }, renderDelegateOverview() {},
    setTimeout(callback) { callback(); return 0; } });
  const stream = source.split(/\r?\n/).find(line => line.startsWith('async function stream()')); assert.ok(stream);
  wire.run('var streamController,selected=null,room=null;\n' + stream); await wire.run('stream()');
  assert.equal(wire.element('quarantineBanner').hidden, false); assert.equal(wire.run('quarantineReceipt.currentRevision'), '7');
  assert.equal(wire.calls.length, 0, 'Status events must not read incident pages or start workflows.');
  assert.equal(cancelled, 1); assert.equal(released, 1);
});

test('reporting PIDs are literal historical metadata and absent values remain explicitly unknown', async () => {
  const wire = harness(() => page(2, [incident(1, { reportingPid: 12345 }), incident(2)])); await wire.open();
  const cards = wire.element('quarantineIncidents').children;
  assert.match(cards[0].textContent, /Reporting Council PID \(at report time\)12345/);
  assert.match(cards[1].textContent, /Reporting Council PID \(at report time\)Unknown \(not recorded\)/);
  assert.match(html, /current process identity and liveness require inspection/);
  assert.equal(cards.some(card => card.children.some(child => /[Kk]ill|[Ss]top process/.test(child.textContent))), false);
});

test('a delayed lower local revision page cannot rewrite newer same-host fault, details or cursor', async () => {
  const pending = deferred(), old = localStatus(hostA, '9007199254740992', 2, 2, { localFault: 'Earlier storage fault.' });
  const wire = harness((_path, count) => count === 1 ? { ...page(2, [incident(1)], '1', 2), ...old } : pending.promise);
  await wire.open(); const original = wire.element('quarantineIncidents').children[0], reading = wire.click('quarantineMore');
  wire.element('quarantineBody').scrollTop = 37;
  wire.apply(localStatus(hostA, '9007199254740993', 2, 2, { localFault: 'Newer process cleanup fault.', persistenceWarning: QUARANTINE_PERSISTENCE_WARNING }));
  pending.resolve({ ...page(2, [incident(2)], null, 2), ...old }); await reading;
  assert.equal(wire.element('quarantineIncidents').children[0], original); assert.equal(wire.element('quarantineIncidents').children.length, 1);
  assert.equal(wire.run('quarantineCursor'), '1'); assert.equal(wire.element('quarantineBody').scrollTop, 37);
  assert.match(wire.text('quarantineLocalFault'), /Newer process cleanup fault/); assert.equal(wire.text('quarantinePersistenceWarning'), QUARANTINE_PERSISTENCE_WARNING);
  assert.equal(wire.run('quarantineReceipt.localRevision'), '9007199254740993'); assert.equal(wire.element('quarantineChanged').hidden, false);
  assert.equal(wire.calls.length, 2); assert.equal(wire.calls.every(call => call.body === undefined), true);
  wire.apply(localStatus(hostA, '9007199254740994', 2, 2, { localFault: 'Latest refined process fault.' }));
  assert.match(wire.text('quarantineLocalFault'), /Latest refined process fault/); assert.equal(wire.text('quarantinePersistenceWarning'), QUARANTINE_PERSISTENCE_WARNING);
});

test('foreign-host pages are stale while a new non-page session clears old faults and preserves details for refresh', async () => {
  let count = 0;
  const first = { ...page(99, [incident(99)]), ...localStatus(hostA, 4, 99, 1, { localFault: 'Departed host fault.', persistenceWarning: QUARANTINE_PERSISTENCE_WARNING }) };
  const fresh = { ...page(0), ...localStatus(hostB, 0) };
  const wire = harness(() => ++count === 1 ? first : fresh); await wire.open();
  const original = wire.element('quarantineIncidents').children[0]; await wire.click('quarantineRefresh');
  assert.equal(wire.run('quarantineReceipt.hostId'), hostA); assert.match(wire.text('quarantineLocalFault'), /Departed host fault/);
  assert.equal(wire.element('quarantineIncidents').children[0], original); assert.equal(wire.element('quarantineChanged').hidden, false);
  wire.apply(localStatus(hostB, 0));
  assert.equal(wire.run('quarantineReceipt.hostId'), hostB); assert.equal(wire.run('quarantineReceipt.currentRevision'), '0');
  assert.equal(wire.element('quarantineBanner').hidden, true); assert.equal(wire.text('quarantineLocalFault'), '');
  assert.equal(wire.text('quarantinePersistenceWarning'), ''); assert.equal(wire.text('quarantineCommand'), '');
  assert.equal(wire.element('quarantineIncidents').children[0], original); assert.equal(wire.element('quarantineChanged').hidden, false);
  await wire.click('quarantineRefresh'); assert.equal(wire.element('quarantineIncidents').children.length, 0); assert.equal(wire.element('quarantineChanged').hidden, true);
});

test('legacy status pages preserve known local faults and invalid partial host metadata cannot change status', async () => {
  const wire = harness(() => ({ ...page(0), localFault: 'Earlier unversioned page fault.' }));
  wire.apply(localStatus(hostA, 2, 0, 0, { localFault: 'Current scoped process fault.', persistenceWarning: QUARANTINE_PERSISTENCE_WARNING }));
  await wire.open(); assert.match(wire.text('quarantineLocalFault'), /Current scoped process fault/);
  assert.equal(wire.run('quarantineReceipt.hostId'), hostA); assert.equal(wire.run('quarantineReceipt.localRevision'), '2');
  for (const extra of [{ hostId: hostA }, { localRevision: '0' }, { hostId: hostA, localRevision: '01' }]) {
    assert.throws(() => wire.apply({ blocked: false, currentRevision: '0', openCount: 0, ...extra }), /Unexpected local host status/);
  }
  const legacy = harness(() => ({ ...page(0), localFault: 'First legacy page fault.' })); await legacy.open();
  assert.match(legacy.text('quarantineLocalFault'), /First legacy page fault/);
  legacy.apply({ blocked: false, currentRevision: '0', openCount: 0, localFault: 'Newer legacy SSE fault.', persistenceWarning: QUARANTINE_PERSISTENCE_WARNING });
  await legacy.click('quarantineRefresh'); assert.match(legacy.text('quarantineLocalFault'), /Newer legacy SSE fault/);
});

test('quarantine receipts disable new dispatch controls while preserving stop, pause, inspection and drafts', () => {
  const wire = harness(), controls = source.split(/\r?\n/).find(line => line.startsWith('function controls()')); assert.ok(controls);
  wire.run("var room={id:'room',status:'ready',tasks:[{status:'pending'}],decision:{approved:true}},selected='room',roomLoading=false,busy=false,historyBusy=false;function active(){return room.status==='running'||room.status==='discussing';}function backendMismatch(){return false;}\n" + controls);
  wire.run('controls()'); assert.equal(wire.element('discuss').disabled, false); assert.equal(wire.element('execute').disabled, false);
  wire.apply(localStatus(hostA, 1, 1, 1));
  assert.equal(wire.element('discuss').disabled, true); assert.equal(wire.element('execute').disabled, true);
  assert.equal(wire.element('pause').disabled, false); assert.equal(wire.element('historyButton').disabled, false);
  wire.run("room.status='running';controls();"); assert.equal(wire.element('cancel').disabled, false); assert.equal(wire.element('pause').disabled, false);
  wire.run("room.status='paused';controls();"); assert.equal(wire.element('resume').disabled, true); assert.equal(wire.element('cancel').disabled, false);
  wire.apply(localStatus(hostA, 2, 1, 0, { localFault: 'Local cleanup uncertainty.' })); assert.equal(wire.element('resume').disabled, true);
  wire.apply(localStatus(hostB, 0)); assert.equal(wire.element('resume').disabled, false);
  assert.equal(wire.element('messageBody').value, 'Unsent project guidance'); assert.equal(wire.element('taskBrief').value, 'Unsent acceptance checks');
  assert.equal(wire.element('quarantineIncidents').replacements, 0); assert.equal(wire.calls.length, 0);
});
