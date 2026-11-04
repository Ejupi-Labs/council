import test from 'node:test';
import assert from 'node:assert/strict';
import { request as httpRequest } from 'node:http';
import { randomUUID } from 'node:crypto';
import { Script, createContext, runInContext } from 'node:vm';
import { serveDashboard } from '../dist/dashboard.js';
import { dashboardHtml, readableRoomMessage } from '../dist/dashboard-ui.js';
import { billingInput, billingNumber, billingPatch, billingRaw, billingStable } from '../dist/dashboard-billing-ui.js';

const pause = ms => new Promise(resolve => setTimeout(resolve, ms));

function deferred() {
  let resolve, reject;
  const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

function browserFunctions(prefixes) {
  // Exercise the actual shipped browser functions with deterministic network ordering.
  const script = /<script nonce="[^"]+">([\s\S]*?)<\/script>/.exec(dashboardHtml('browser-fixture'))[1];
  const lines = script.split(/\r?\n/);
  return prefixes.map(prefix => {
    const source = lines.find(line => line.startsWith(prefix));
    assert.ok(source, 'Missing browser fixture function: ' + prefix); return source;
  }).join('\n');
}

function browserRoom(id, revision = 1, status = 'idle') {
  return { id, revision, status, backend: 'mock', tasks: [{ id: 'task-1', status: 'pending' }], messages: [], decision: { approved: true } };
}

function browserHarness(handler) {
  const elements = new Map(), listeners = new Map(), calls = [], errors = [];
  const element = id => {
    if (!elements.has(id)) elements.set(id, { value: '', disabled: false, resets: 0,
      addEventListener(event, callback) { listeners.set(id + ':' + event, callback); },
      reset() { this.resets++; },
    });
    return elements.get(id);
  };
  const context = createContext({ Promise, AbortController, encodeURIComponent, $: element,
    api: (path, body) => { calls.push({ path, body }); return handler(path, body); },
    renderList() {}, renderRoom() {}, updateHistoryControls() {}, clearError() {},
    error: message => errors.push(message), refreshList: async () => {},
  });
  runInContext("var token='fixture',serverBackend='mock',selected='A',busy=false,loading=0,selectionEpoch=0,roomLoading=false,historyOpen=false,historyMessages=[],historyCursor=null,historyBusy=false,historyEpoch=0,historyController,roomList=[],drafts=new Map(),draftOwner='A';", context);
  context.room = browserRoom('A');
  runInContext(browserFunctions(['function backendMismatch()', 'function active()', 'function controls()',
    'function draftFor(', 'function saveVisibleDrafts()', 'function restoreDraft(', 'function clearRoomDraft(',
    'function installDraftListeners()', 'function invalidateHistory()', 'function requestTarget()', 'function sameTarget(', 'function applyRoomResponse(', 'async function loadRoom(id)',
    'async function perform(action)', "$('messageForm').addEventListener('submit'", "$('taskForm').addEventListener('submit'"]), context);
  for (const [id, value] of Object.entries({ messageAuthor: 'PM', messageBody: 'Original A guidance', taskTitle: 'A task', taskBrief: 'A verification brief', taskPermission: 'read', taskDependency: '' })) element(id).value = value;
  runInContext('installDraftListeners();saveVisibleDrafts();', context);
  return { context, calls, errors, element, run: code => runInContext(code, context),
    edit(id, value) { element(id).value = value; listeners.get(id + ':input')(); },
    submit: id => listeners.get(id + ':submit')({ preventDefault() {} }) };
}

function createHarness(handler, refreshList = async () => {}) {
  const elements = new Map(), listeners = new Map(), errors = [], calls = [];
  const element = id => {
    if (!elements.has(id)) elements.set(id, { value: '', hidden: true, open: false, resets: 0, children: [],
      addEventListener: (event, callback) => listeners.set(id + ':' + event, callback),
      reset() { this.resets++; }, replaceChildren() { this.children = []; },
      close() { this.open = false; }, showModal() { this.open = true; }, focus() {},
    });
    return elements.get(id);
  };
  for (const [id, value] of Object.entries({ createRoomTitle: 'Original room', createRoomBrief: 'Original complete objective.', createTaskTitle: 'First task', createTaskBrief: 'Original task brief.', createTaskPermission: 'read', createRounds: '1', createMaxCalls: '', createContextLimit: '' })) element(id).value = value;
  const fields = { '.member-agent': 'developer', '.member-role': 'lead', '.member-weight': '1', '.member-model': '', '.member-effort': '', '.member-advisor': '' };
  element('memberRows').children = [{ querySelector: selector => ({ value: fields[selector] }) }]; element('createDialog').open = true;
  const context = createContext({ Promise, $: element, api: (path, body) => { calls.push({ path, body }); return handler(path, body); },
    renderRoom() {}, saveVisibleDrafts() {}, restoreDraft() {}, invalidateHistory() {}, routingReset() {}, clearError() {}, refreshList,
    error: message => errors.push(message),
  });
  runInContext("var createEpoch=1,createDraftGeneration=0,createPending=new Set(),room={id:'A'},selected='A',selectionEpoch=0,loading=0,roomLoading=false,historyMessages=[],historyCursor=null,agents=[{agent:'developer'}];", context);
  runInContext(browserFunctions(['function invalidateCreateDialog()', 'function dismissCreateDialog()', 'function openCreate()', "$('createForm').addEventListener('submit'"]), context);
  return { context, calls, errors, element, run: code => runInContext(code, context),
    edit(id, value) { element(id).value = value; context.createDraftGeneration++; },
    submit: () => listeners.get('createForm:submit')({ preventDefault() {} }) };
}

function historyHarness(handler) {
  const initial = { id: 'first', body: 'Complete first message.' }, dialog = { open: true, shown: 0, showModal() { this.open = true; this.shown++; }, close() { this.open = false; } };
  const view = { children: [initial], replacements: 0, replaceChildren() { this.children = []; this.replacements++; }, append(node) { this.children.push(node); } };
  const elements = new Map([['historyDialog', dialog], ['historyMessages', view]]), errors = [], signals = [];
  const element = id => { if (!elements.has(id)) elements.set(id, { hidden: false }); return elements.get(id); };
  const context = createContext({ Promise, AbortController, encodeURIComponent, $: element,
    api: (path, _body, signal) => { signals.push(signal); return handler(path); },
    updateHistoryControls() {}, messageNode: message => message, number: String, error: message => errors.push(message),
  });
  runInContext("var room={id:'A'},selected='A',selectionEpoch=0,roomLoading=false,historyBusy=false,historyOpen=true,historyCursor=100,historyEpoch=0,historyController;", context);
  context.historyMessages = [initial];
  runInContext(browserFunctions(['function invalidateHistory()', 'function dismissHistory()', 'function renderHistoryMessages(', 'async function loadHistory(']), context);
  return { context, dialog, view, errors, signals, run: code => runInContext(code, context) };
}

function streamHarness({ fetch, refreshList = async () => {}, stopAfter = 1 }) {
  const errors = [], connections = [], signals = [];
  let waits = 0, context;
  context = createContext({ Promise, AbortController, TextDecoder,
    fetch: (path, options) => { signals.push(options.signal); return fetch(path, options); },
    refreshList, error: message => errors.push(message), connection: (ok, text) => connections.push({ ok, text }),
    renderList() {}, loadRoom() {},
    setTimeout: callback => { if (++waits >= stopAfter) context.token = ''; queueMicrotask(callback); return waits; },
  });
  runInContext("var token='fixture',streamController,selected=null,room=null;" + browserFunctions(['async function stream()']), context);
  return { context, errors, connections, signals, waits: () => waits, run: () => runInContext('stream()', context) };
}

function accountingRecord(alias = 'a', settings = {}) {
  return { alias, enabled: true, profile: 'codex', model: null, revision: 1, mode: 'unknown', accountKey: null, plan: null,
    source: null, observedAt: null, validUntil: null, stale: false, windows: [], prices: null, routing: null, skills: null,
    effectiveSkills: [], availability: { status: 'unknown', reasons: ['No current observation.'], windows: [] }, ...settings };
}

function billingHarness(handler) {
  const elements = new Map(), calls = [], shown = [], metadata = [];
  const element = id => { if (!elements.has(id)) elements.set(id, { hidden: true, open: id === 'billingDialog', textContent: '' }); return elements.get(id); };
  const record = accountingRecord(), raw = billingRaw(record), draft = { base: structuredClone(record), initial: structuredClone(raw), raw: { ...raw, plan: 'Submitted plan' }, generation: 1 };
  const context = createContext({ Promise, AbortController, encodeURIComponent, $: element, billingInput, billingPatch, billingRaw, billingStable,
    api: (path, body, signal) => { calls.push({ path, body, signal }); return handler(path, body); },
    billingCapture() {}, billingSaveState() {}, billingRenderAliases() {},
    billingShow: alias => shown.push(alias), billingMetadata: value => metadata.push(value), billingWriteForm() {}, billingRefreshLinked: async () => {},
  });
  Object.assign(context, { billingSelected: 'a', billingEpoch: 1, billingLoading: false, billingPending: new Map(), billingDrafts: new Map([['a', draft]]), billingRecords: new Map([['a', record]]), billingSkills: [], billingController: undefined, billingSyncController: undefined });
  runInContext(browserFunctions(['function billingClone(', 'function billingFailure(', 'function billingDraftFor(', 'async function saveBilling(']), context);
  return { context, calls, shown, metadata, element, draft, run: code => runInContext(code, context),
    save: () => runInContext('saveBilling({preventDefault(){}})', context),
    edit(field, value) { const current = context.billingDrafts.get(context.billingSelected); current.raw[field] = value; current.generation++; } };
}

function routingHarness(handler) {
  const elements = new Map(), calls = [], rendered = [], members = [];
  const element = id => {
    if (!elements.has(id)) elements.set(id, { value: '', checked: false, hidden: true, open: id === 'createDialog', disabled: false,
      replaceChildren() { members.length = 0; }, append() {},
      get lastElementChild() { return members.at(-1)?.row; },
    });
    return elements.get(id);
  };
  for (const [id, value] of Object.entries({ createRoomBrief: 'Complete user objective.', routingTaskClass: 'standard', routingStrategy: 'balanced', routingCollaboration: 'review', routingCapabilities: 'code, tests', createTaskPermission: 'read', createRounds: '1', routingParticipants: '' })) element(id).value = value;
  const context = createContext({ Promise, AbortController, encodeURIComponent, $: element,
    api: (path, body) => { calls.push({ path, body }); return handler(path, body); },
    routingRender: plan => rendered.push(plan), element: () => ({}), updateMemberRows() {},
    addMember(alias) { const fields = new Map(); const row = { dataset: {}, querySelector(selector) { if (!fields.has(selector)) fields.set(selector, { value: '' }); return fields.get(selector); } }; members.push({ alias, row }); context.createDraftGeneration++; },
  });
  Object.assign(context, { createEpoch: 1, createDraftGeneration: 0, routingEpoch: 0, routingController: undefined, routingPlan: null, routingGeneration: null, routingCreateEpoch: null, routingBusy: false, agents: [{ agent: 'a' }, { agent: 'b' }] });
  runInContext(browserFunctions(['function routingState()', 'function routingRequest()', 'async function previewRouting()', 'function applyRouting()']), context);
  return { context, calls, rendered, members, element, run: code => runInContext(code, context) };
}

async function fixture(t) {
  const records = new Map(), calls = [], ownership = { service: 0, rooms: 0 };
  const service = { config: { backend: 'mock', maxCalls: 32, agents: {
    developer: { enabled: true, profile: 'codex', model: 'explicit-model', effort: 'high', authority: 2 },
    reviewer: { enabled: true, profile: 'claude' }, disabled: { enabled: false, profile: 'junie' },
  } }, close: async () => { ownership.service++; } };
  const summaries = () => [...records.values()].map(room => ({ id: room.id, title: room.title, status: room.status,
    taskCount: room.tasks.length, completedTasks: room.tasks.filter(task => task.status === 'completed').length,
    callsUsed: room.callsUsed, maxCalls: room.maxCalls, revision: room.revision, updatedAt: room.updatedAt }));
  const mutate = (id, action, change) => {
    calls.push(action);
    const room = records.get(id); if (!room) throw new Error('Room not found (404).');
    change(room); room.revision++; return structuredClone(room);
  };
  const rooms = {
    list: summaries,
    get: id => { const room = records.get(id); if (!room) throw new Error('Room not found (404).'); return structuredClone(room); },
    messagePage: (id, limit = 100, cursor = 0) => {
      const room = records.get(id); if (!room) throw new Error('Room not found (404).');
      return { messages: structuredClone(room.messages.slice(cursor, cursor + limit)), nextCursor: cursor + limit < room.messages.length ? cursor + limit : null };
    },
    create: input => {
      calls.push('create');
      const now = new Date().toISOString();
      const room = { id: randomUUID(), ...input, backend: service.config.backend, tasks: input.tasks ?? [], messages: [], status: 'idle',
        callsUsed: 0, maxCalls: input.maxCalls ?? service.config.maxCalls, contextLimit: input.contextLimit ?? null,
        rounds: input.rounds ?? 1, autoExecute: input.autoExecute ?? true, revision: 1, createdAt: now, updatedAt: now };
      records.set(room.id, room); return structuredClone(room);
    },
    postMessage: (id, value) => mutate(id, 'message', room => room.messages.push({ id: randomUUID(), kind: 'human', ...value, createdAt: new Date().toISOString() })),
    addTask: (id, value) => mutate(id, 'task', room => room.tasks.push({ id: randomUUID(), ...value, status: 'pending' })),
    startDiscussion: id => mutate(id, 'discuss', room => { room.status = 'discussing'; }),
    execute: id => mutate(id, 'execute', room => { room.status = 'running'; }),
    cancel: id => mutate(id, 'cancel', room => { room.status = 'cancelled'; }),
    close: async () => { ownership.rooms++; },
  };
  const dashboard = await serveDashboard(service, rooms);
  t.after(() => dashboard.close());
  const url = new URL(dashboard.url), token = new URLSearchParams(url.hash.slice(1)).get('token');
  return { dashboard, origin: url.origin, token, records, rooms, calls, ownership,
    api: (path, options = {}) => fetch(url.origin + path, { ...options, headers: { Authorization: 'Bearer ' + token, ...options.headers } }),
    post: (path, body = {}) => fetch(url.origin + path, { method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
  };
}

function raw(origin, path, headers = {}) {
  return new Promise((resolve, reject) => {
    const request = httpRequest(origin + path, { headers }, response => {
      const chunks = []; response.on('data', chunk => chunks.push(chunk));
      response.once('end', () => resolve({ status: response.statusCode, body: Buffer.concat(chunks).toString('utf8') }));
    });
    request.once('error', reject); request.end();
  });
}

test('dashboard serves a public CSP-protected shell and keeps its access token out of responses', async t => {
  const wire = await fixture(t);
  assert.equal(new URL(wire.dashboard.url).hostname, '127.0.0.1');
  assert.equal(Buffer.from(wire.token, 'base64url').length, 32);
  const response = await fetch(wire.origin);
  assert.equal(response.status, 200); assert.equal(response.headers.get('cache-control'), 'no-store');
  const html = await response.text(), csp = response.headers.get('content-security-policy');
  assert.ok(csp.includes("default-src 'none'")); assert.ok(csp.includes("frame-ancestors 'none'"));
  assert.ok(!csp.includes('unsafe-inline')); assert.ok(!csp.includes('unsafe-eval'));
  const script = /<script nonce="([^"]+)">([\s\S]*?)<\/script>/.exec(html);
  assert.ok(script); assert.ok(csp.includes("'nonce-" + script[1] + "'"));
  new Script(script[2]);
  assert.equal(html.includes(wire.token), false); assert.equal(html.includes('.innerHTML'), false);
  assert.ok(html.includes('sessionStorage')); assert.ok(html.includes('history.replaceState'));
  assert.ok(html.includes('New room')); assert.ok(html.includes('First task'));
  assert.ok(html.includes('aria-label="Rooms"')); assert.equal(html.includes('Stanze'), false);
  assert.ok(html.includes('Full context (default)'));
  assert.ok(html.includes('Advisor requests'));
  assert.ok(html.includes('Server: offline simulation')); assert.ok(html.includes('Room backend: '));
  for (const id of ['createMaxCalls', 'createContextLimit']) {
    const input = new RegExp('<input id="' + id + '"[^>]*>').exec(html)?.[0];
    assert.ok(input); assert.equal(/\s(?:max|value|required)(?:\s|=|>)/.test(input), false);
  }
  assert.equal(/<textarea[^>]+maxlength=/.test(html), false);
  assert.equal(/<input id="createRounds"[^>]*\smax=/.test(html), false);
  assert.equal(html.includes('8 voting members'), false);
  assert.equal(html.includes('it-IT'), false);
  assert.deepEqual(wire.calls, []);
});

test('readable ballots retain the complete rationale and assignments without protocol formatting', () => {
  const rationale = 'Detailed evidence, conditions, and verification. '.repeat(800);
  const ballot = JSON.stringify({ vote: 'approve', rationale, assignments: { 'task-1': 'developer', 'task-2': 'reviewer' } });
  const readable = readableRoomMessage(ballot);
  assert.ok(readable.startsWith('Vote: Approve\n\nRationale\n'));
  assert.ok(readable.includes(rationale)); assert.ok(readable.includes('task-1 → developer'));
  assert.ok(readable.includes('task-2 → reviewer')); assert.equal(readable.includes('"assignments"'), false);
  const unrecognized = JSON.stringify({ vote: 'approve', rationale, assignments: {}, additionalEvidence: 'Keep every field.' });
  assert.equal(readableRoomMessage(unrecognized), unrecognized);
  assert.equal(readableRoomMessage('A complete plain-text opinion.'), 'A complete plain-text opinion.');
});

test('billing editor preserves unknown quota and price values and never infers remaining quota', () => {
  const unknown = billingInput(billingRaw(accountingRecord()));
  assert.equal(unknown.mode, 'unknown'); assert.equal(unknown.observedAt, null); assert.equal(unknown.prices, null);
  assert.equal(unknown.routing, null); assert.equal(unknown.skills, null); assert.deepEqual(unknown.windows, []);
  const raw = billingRaw(accountingRecord('a', { windows: [{ id: 'custom', label: 'Custom reporting period', unit: 'credits', limit: 100, used: 0, remaining: null, resetAt: null }] }));
  const value = billingInput(raw); assert.equal(value.windows[0].remaining, null); assert.equal(value.windows[0].used, 0);
  assert.equal(billingNumber('', 'Amount'), null); assert.equal(billingNumber('0', 'Amount'), 0);
  assert.throws(() => billingNumber('-1', 'Amount'), /non-negative/);
  raw.currency = 'EUR'; raw.inputPerMillion = '0'; raw.outputPerMillion = '1.25';
  assert.deepEqual(billingInput(raw).prices, { currency: 'EUR', inputPerMillion: 0, outputPerMillion: 1.25 });
});

test('plan and routing updates do not refresh quota provenance or timestamps', () => {
  const record = accountingRecord('a', { mode: 'subscription', source: { kind: 'reported', label: 'Imported observation' },
    observedAt: '2026-01-01T12:00:00Z', validUntil: '2026-01-01T13:00:00Z',
    windows: [{ id: 'custom', label: 'Custom period', unit: 'requests', limit: 40, remaining: 20, used: null, resetAt: '2026-01-01T13:00:00Z' }] });
  const raw = billingRaw(record); raw.plan = 'An explicit local plan label'; raw.tier = 'standard'; raw.capabilities = 'code, tests'; raw.costRank = '1'; raw.qualityRank = '1e100';
  const patch = billingPatch(record, billingInput(raw)); assert.deepEqual(Object.keys(patch).sort(), ['plan', 'routing']);
  assert.equal(Object.hasOwn(patch, 'source'), false); assert.equal(Object.hasOwn(patch, 'observedAt'), false);
  raw.windows[0].remaining = '0';
  const windowPatch = billingPatch(record, billingInput(raw)); assert.equal(windowPatch.observedAt, record.observedAt);
  assert.equal(windowPatch.windows[0].remaining, 0); assert.equal(Object.hasOwn(windowPatch, 'validUntil'), false);
  raw.observedAt = ''; assert.throws(() => billingPatch(record, billingInput(raw)), /observed-at timestamp/);
});

test('billing skills distinguish inheritance from an explicit empty assignment', () => {
  const record = accountingRecord('a', { effectiveSkills: ['council-advisor'] }), raw = billingRaw(record);
  assert.deepEqual(raw.skills, ['council-advisor']); assert.equal(billingInput(raw).skills, null);
  raw.overrideSkills = true; raw.skills = []; assert.deepEqual(billingPatch(record, billingInput(raw)).skills, []);
});

test('billing saves send revision-guarded patches and preserve newer edits', async () => {
  const save = deferred(), wire = billingHarness(() => save.promise);
  const pending = wire.save(); assert.deepEqual(Object.keys(wire.calls[0].body).sort(), ['expectedRevision', 'plan']);
  assert.equal(wire.calls[0].body.expectedRevision, 1); wire.edit('plan', 'A newer unsaved plan');
  save.resolve(accountingRecord('a', { revision: 2, plan: 'Submitted plan' })); await pending;
  assert.equal(wire.draft.base.revision, 2); assert.equal(wire.draft.raw.plan, 'A newer unsaved plan');
  assert.equal(wire.shown.length, 0); assert.match(wire.element('billingStatus').textContent, /newer edits remain unsaved/i);
});

test('billing conflicts preserve drafts and late saves cannot overwrite another alias or dialog', async () => {
  const conflict = billingHarness(async () => { throw Object.assign(new Error('Conflict'), { status: 409 }); });
  await conflict.save(); assert.equal(conflict.draft.raw.plan, 'Submitted plan'); assert.equal(conflict.draft.base.revision, 1);
  assert.match(conflict.element('billingError').textContent, /draft is preserved.*reload selected/i);
  const save = deferred(), other = billingHarness(() => save.promise), pending = other.save();
  other.context.billingSelected = 'b'; other.context.billingEpoch++;
  const recordB = accountingRecord('b'), rawB = billingRaw(recordB); other.context.billingRecords.set('b', recordB);
  other.context.billingDrafts.set('b', { base: recordB, initial: billingRaw(recordB), raw: { ...rawB, plan: 'Keep B draft' }, generation: 1 });
  save.resolve(accountingRecord('a', { revision: 2, plan: 'Submitted plan' })); await pending;
  assert.equal(other.context.billingSelected, 'b'); assert.equal(other.context.billingDrafts.get('b').raw.plan, 'Keep B draft');
  assert.equal(other.shown.length, 0); assert.equal(other.metadata.length, 0);
});

test('closing and reopening billing during linked-record refresh prevents stale view updates', async () => {
  const refresh = deferred(), entered = deferred();
  const wire = billingHarness(async () => accountingRecord('a', { revision: 2, plan: 'Submitted plan' }));
  wire.context.billingRefreshLinked = () => { entered.resolve(); return refresh.promise; };
  const saving = wire.save(); await entered.promise;
  wire.context.billingEpoch++; wire.edit('plan', 'New dialog draft'); wire.element('billingStatus').textContent = 'Keep the new dialog status.';
  refresh.resolve(); await saving;
  assert.equal(wire.shown.length, 0); assert.equal(wire.metadata.length, 0);
  assert.equal(wire.element('billingStatus').textContent, 'Keep the new dialog status.');
});

test('shared-account refresh retains dirty drafts and never regresses a known record revision', async () => {
  const updatedA = accountingRecord('a', { revision: 3, plan: 'Newer known A' }), updatedB = accountingRecord('b', { revision: 2 });
  const wire = billingHarness(async () => ({ agents: [accountingRecord('a', { revision: 2 }), updatedB], skills: [] }));
  wire.context.billingRecords.set('a', updatedA);
  wire.run(browserFunctions(['function billingMerge(', 'async function billingRefreshLinked(']));
  await wire.run('billingRefreshLinked(1)');
  assert.equal(wire.context.billingRecords.get('a').revision, 3); assert.equal(wire.context.billingRecords.get('b').revision, 2);
  assert.equal(wire.draft.raw.plan, 'Submitted plan'); assert.equal(wire.draft.base.revision, 1);
});

test('routing previews reject empty objectives and ignore results after the create draft changes', async () => {
  const result = deferred(), wire = routingHarness(() => result.promise);
  wire.element('createRoomBrief').value = ' '; await wire.run('previewRouting()'); assert.equal(wire.calls.length, 0);
  assert.match(wire.element('routingError').textContent, /enter the shared objective/i);
  wire.element('createRoomBrief').value = 'Complete user objective.'; const pending = wire.run('previewRouting()');
  assert.equal(wire.calls[0].path, '/api/routing/plan'); assert.equal(wire.calls[0].body.permission, 'read');
  wire.context.createDraftGeneration++; result.resolve({ selectedAliases: ['a'], suggestedRoomMembers: [{ agent: 'a', role: 'lead' }] }); await pending;
  assert.equal(wire.context.routingPlan, null); assert.equal(wire.element('routingApply').disabled, true); assert.equal(wire.rendered.length, 0);
});

test('routing applies only a current explicit member suggestion and never executes a workflow', async () => {
  const plan = { workflow: { mode: 'review' }, selectedAliases: ['a', 'b'], reasons: ['Explicit capability match.'], warnings: ['Availability is unknown.'], estimatedCalls: 3,
    suggestedRoomMembers: [{ agent: 'a', role: 'lead', weight: 0.5, model: 'configured-model', effort: 'high', skills: ['council-implementation-review'] }, { agent: 'b', role: 'worker', weight: 2 }] };
  const wire = routingHarness(async () => plan); await wire.run('previewRouting()');
  assert.equal(wire.members.length, 0); wire.run('applyRouting()');
  assert.deepEqual(wire.members.map(member => member.alias), ['a', 'b']);
  assert.equal(wire.members[0].row.querySelector('.member-model').value, 'configured-model');
  assert.deepEqual(JSON.parse(wire.members[0].row.dataset.routingSkills), ['council-implementation-review']);
  assert.match(wire.members[0].row.querySelector('.member-skills-note').textContent, /council-implementation-review/);
  assert.equal(wire.element('createRoomBrief').value, 'Complete user objective.');
  assert.deepEqual(wire.calls.map(call => call.path), ['/api/routing/plan']);
  wire.context.createDraftGeneration++; wire.run('applyRouting()');
  assert.match(wire.element('routingError').textContent, /preview is no longer current/i);
});

test('room creation retains explicit skills from an applied team suggestion', async () => {
  const wire = createHarness(async () => browserRoom('created'));
  wire.element('memberRows').children[0].dataset = { routingSkills: JSON.stringify(['council-implementation-review']) };
  await wire.submit(); assert.deepEqual(Array.from(wire.calls[0].body.members[0].skills), ['council-implementation-review']);
});

test('browser blocks actions, messages and tasks until the selected room has loaded', async () => {
  const loadingB = deferred();
  const wire = browserHarness(path => {
    assert.equal(path, '/api/rooms/B'); return loadingB.promise;
  });
  const navigation = wire.run("loadRoom('B')");
  assert.equal(wire.context.selected, 'B'); assert.equal(wire.context.roomLoading, true);
  for (const id of ['discuss', 'execute', 'sendMessage', 'addTask']) assert.equal(wire.element(id).disabled, true);
  await wire.run("perform('discuss')"); await wire.submit('messageForm'); await wire.submit('taskForm');
  assert.deepEqual(wire.calls.map(call => call.path), ['/api/rooms/B']);
  loadingB.resolve(browserRoom('B')); await navigation;
  assert.equal(wire.context.room.id, 'B'); assert.equal(wire.context.roomLoading, false);
  assert.equal(wire.element('sendMessage').disabled, false);
});

test('late room mutations cannot replace another selected room or erase its drafts', async () => {
  for (const action of ['discuss', 'messageForm', 'taskForm']) {
    const mutation = deferred();
    const wire = browserHarness(path => path === '/api/rooms/B' ? Promise.resolve(browserRoom('B')) : mutation.promise);
    const pending = action === 'discuss' ? wire.run("perform('discuss')") : wire.submit(action);
    assert.ok(wire.calls[0].path.startsWith('/api/rooms/A/'));
    await wire.run("loadRoom('B')");
    wire.element('messageBody').value = 'New B guidance'; wire.element('taskTitle').value = 'New B task';
    mutation.resolve(browserRoom('A', 2, 'discussing')); await pending;
    assert.equal(wire.context.selected, 'B'); assert.equal(wire.context.room.id, 'B');
    assert.equal(wire.element('messageBody').value, 'New B guidance');
    assert.equal(wire.element('taskTitle').value, 'New B task'); assert.equal(wire.element('taskForm').resets, 0);
  }
});

test('message and task drafts are isolated by room and survive navigation and same-room refresh', async () => {
  const wire = browserHarness(path => Promise.resolve(browserRoom(path.split('/')[3], 2)));
  const guidance = '  Complete A guidance.\nKeep the final newline.\n';
  wire.edit('messageBody', guidance); wire.edit('taskTitle', 'A-only task');
  wire.edit('taskBrief', '  Complete A task brief.\n'); wire.edit('taskPermission', 'write'); wire.edit('taskDependency', 'task-1');
  await wire.run("loadRoom('B')");
  assert.equal(wire.element('messageBody').value, ''); assert.equal(wire.element('taskTitle').value, '');
  assert.equal(wire.element('taskPermission').value, 'read'); assert.equal(wire.element('taskDependency').value, '');
  wire.edit('messageBody', 'B-only guidance'); wire.edit('taskTitle', 'B-only task');
  await wire.run("loadRoom('A')"); await wire.run("loadRoom('A')");
  assert.equal(wire.element('messageBody').value, guidance); assert.equal(wire.element('taskTitle').value, 'A-only task');
  assert.equal(wire.element('taskPermission').value, 'write'); assert.equal(wire.element('taskDependency').value, 'task-1');
  await wire.submit('messageForm');
  const message = wire.calls.find(call => call.path.endsWith('/messages'));
  assert.equal(message.path, '/api/rooms/A/messages'); assert.equal(message.body.body, guidance);
  await wire.run("loadRoom('B')");
  assert.equal(wire.element('messageBody').value, 'B-only guidance'); assert.equal(wire.element('taskTitle').value, 'B-only task');
});

test('rendering another room dependency selector cannot rewrite the previous room draft', async () => {
  const wire = browserHarness(path => Promise.resolve(browserRoom(path.split('/')[3], 2)));
  wire.edit('taskDependency', 'task-5');
  wire.context.renderRoom = () => { wire.element('taskDependency').value = ''; };
  await wire.run("loadRoom('B')"); await wire.run("loadRoom('A')");
  assert.equal(wire.element('taskDependency').value, 'task-5');
});

test('accepted mutations clear only their matching room draft generation, including identical newer text', async () => {
  for (const [form, field, original] of [['messageForm', 'messageBody', 'Original A guidance'], ['taskForm', 'taskTitle', 'A task']]) {
    const mutation = deferred(), wire = browserHarness(() => mutation.promise);
    const submitting = wire.submit(form);
    wire.edit(field, 'A newer draft'); wire.edit(field, original);
    mutation.resolve(browserRoom('A', 2)); await submitting;
    assert.equal(wire.element(field).value, original, 'A newer generation must survive even when its text matches the submitted draft.');
  }
  const mutation = deferred(), wire = browserHarness(path => path === '/api/rooms/B' || path === '/api/rooms/A' ? Promise.resolve(browserRoom(path.split('/')[3], 2)) : mutation.promise);
  const submitting = wire.submit('messageForm'); await wire.run("loadRoom('B')"); wire.edit('messageBody', 'Preserve B draft.');
  mutation.resolve(browserRoom('A', 2)); await submitting;
  assert.equal(wire.element('messageBody').value, 'Preserve B draft.'); assert.equal(wire.context.drafts.get('A').messageBody, '');
  await wire.run("loadRoom('A')"); assert.equal(wire.element('messageBody').value, '');
});

test('closed history dialogs ignore late page responses and errors without reopening', async () => {
  for (const failed of [false, true]) {
    const page = deferred(), wire = historyHarness(() => page.promise);
    const loading = wire.run('loadHistory(true)'); wire.run('dismissHistory()');
    assert.equal(wire.signals[0].aborted, true);
    if (failed) page.reject(new Error('Late history failure.'));
    else page.resolve({ messages: [{ id: 'late', body: 'Complete late page.' }], nextCursor: null });
    await loading;
    assert.equal(wire.dialog.open, false); assert.equal(wire.dialog.shown, 0); assert.deepEqual(wire.errors, []);
    assert.equal(wire.view.children.length, 1); assert.equal(wire.context.historyBusy, false);
  }
});

test('history paging appends complete new messages without rebuilding earlier pages', async () => {
  const completeBody = 'Complete historical evidence. '.repeat(1200);
  const wire = historyHarness(async () => ({ messages: [{ id: 'second', body: completeBody }], nextCursor: null }));
  const originalNode = wire.view.children[0]; await wire.run('loadHistory(true)');
  assert.equal(wire.view.replacements, 0); assert.equal(wire.view.children[0], originalNode);
  assert.equal(wire.view.children[1].body, completeBody); assert.equal(wire.context.historyMessages.length, 2);
});

test('closed and reopened create dialogs keep their newer draft and selection after stale success or failure', async () => {
  for (const failed of [false, true]) {
    const creation = deferred(), wire = createHarness(() => creation.promise);
    const pending = wire.submit(); wire.run('dismissCreateDialog()');
    wire.context.selected = 'B'; wire.context.room = browserRoom('B'); wire.context.selectionEpoch++;
    wire.run('openCreate()'); wire.edit('createRoomTitle', 'A newer create draft');
    if (failed) creation.reject(new Error('Old creation failed.'));
    else creation.resolve(browserRoom('created'));
    await pending;
    assert.equal(wire.context.selected, 'B'); assert.equal(wire.context.room.id, 'B');
    assert.equal(wire.element('createDialog').open, true); assert.equal(wire.element('createRoomTitle').value, 'A newer create draft');
    assert.equal(wire.element('createForm').resets, 0); assert.equal(wire.element('memberRows').children.length, 1);
    assert.equal(wire.element('createError').hidden, true); assert.deepEqual(wire.errors, []);
  }
});

test('room creation preserves newer edits in the same dialog and prevents duplicate concurrent submission', async () => {
  const creation = deferred(), wire = createHarness(() => creation.promise);
  const pending = wire.submit(); await wire.submit(); wire.edit('createRoomBrief', '  Preserve newer complete objective.\n');
  creation.resolve(browserRoom('created')); await pending;
  assert.equal(wire.calls.length, 1); assert.equal(wire.calls[0].body.brief, 'Original complete objective.');
  assert.equal(wire.context.selected, 'A'); assert.equal(wire.element('createDialog').open, true);
  assert.equal(wire.element('createRoomBrief').value, '  Preserve newer complete objective.\n'); assert.equal(wire.element('createForm').resets, 0);
  assert.match(wire.element('createStatus').textContent, /created.*newer edits.*preserved/i);
});

test('a successful room creation is not reported as a creation failure when list refresh fails', async () => {
  const wire = createHarness(async () => browserRoom('created'), async () => { throw new Error('Refresh failed.'); });
  await wire.submit(); assert.equal(wire.calls.length, 1); assert.equal(wire.context.selected, 'created');
  assert.equal(wire.element('createDialog').open, false); assert.equal(wire.element('createForm').resets, 1);
  assert.equal(wire.element('createError').hidden, true); assert.match(wire.errors[0], /room was created.*list could not be refreshed/i);
});

test('stale GET snapshots and errors cannot overwrite newer room evidence or another selection', async () => {
  const snapshot = deferred();
  const wire = browserHarness(path => path === '/api/rooms/A' ? snapshot.promise : Promise.resolve(browserRoom('A', 3, 'discussing')));
  const refreshing = wire.run("loadRoom('A')"); await wire.run("perform('discuss')");
  snapshot.resolve(browserRoom('A', 2)); await refreshing;
  assert.equal(wire.context.room.revision, 3); assert.equal(wire.context.room.status, 'discussing');
  const stale = deferred();
  const other = browserHarness(path => path === '/api/rooms/A' ? stale.promise : Promise.resolve(browserRoom('B')));
  const oldGet = other.run("loadRoom('A')"); await other.run("loadRoom('B')");
  stale.reject(new Error('Old A request failed.')); await oldGet;
  assert.equal(other.context.room.id, 'B'); assert.deepEqual(other.errors, []);
});

test('browser task submission is disabled for failed and cancelled rooms', async () => {
  const wire = browserHarness(() => { throw new Error('No request should be dispatched.'); });
  for (const status of ['failed', 'cancelled']) {
    wire.context.room = browserRoom('A', 2, status); wire.run('controls()');
    assert.equal(wire.element('addTask').disabled, true); await wire.submit('taskForm');
  }
  assert.equal(wire.calls.length, 0);
});

test('browser room creation accepts any positive finite weight and rejects zero, negative or infinite weights', async () => {
  for (const weight of ['0', '-1', 'Infinity', '0.00001', '1e150']) {
    const elements = new Map(), requests = [];
    let submit;
    const element = id => {
      if (!elements.has(id)) elements.set(id, { value: '', addEventListener: (_event, listener) => { submit = listener; } });
      return elements.get(id);
    };
    for (const [id, value] of Object.entries({ createRoomTitle: 'Weight fixture', createRoomBrief: 'A complete objective.', createTaskTitle: 'Task', createTaskBrief: 'Verify.', createTaskPermission: 'read', createRounds: '1', createMaxCalls: '', createContextLimit: '' })) element(id).value = value;
    const fields = { '.member-agent': 'developer', '.member-role': 'lead', '.member-weight': weight, '.member-model': '', '.member-effort': '', '.member-advisor': '' };
    element('memberRows').children = [{ querySelector: selector => ({ value: fields[selector] }) }];
    element('createDialog').open = true;
    const context = createContext({ $: element, api: async (_path, input) => { requests.push(input); throw new Error('Accepted client validation.'); } });
    runInContext("var createEpoch=1,createDraftGeneration=0,createPending=new Set();" + browserFunctions(["$('createForm').addEventListener('submit'"]), context);
    await submit({ preventDefault() {} });
    if (Number(weight) > 0 && Number.isFinite(Number(weight))) {
      assert.equal(requests.length, 1); assert.equal(requests[0].members[0].weight, Number(weight));
    } else {
      assert.equal(requests.length, 0); assert.match(element('createError').textContent, /weights must be positive finite/);
    }
  }
});

test('browser SSE retries clean up every opened response when initial refresh fails', async () => {
  let cancelled = 0;
  const wire = streamHarness({ stopAfter: 3,
    fetch: async () => ({ ok: true, status: 200, body: { cancel: async () => { cancelled++; }, getReader() { throw new Error('Refresh must happen before reading.'); } } }),
    refreshList: async () => { throw new Error('Temporary refresh failure.'); },
  });
  await wire.run(); assert.equal(wire.signals.length, 3); assert.equal(cancelled, 3);
  assert.ok(wire.signals.every(signal => signal.aborted)); assert.equal(wire.context.streamController, undefined);
});

test('browser SSE releases an acquired reader on invalid events before reconnecting', async () => {
  let cancelled = 0, released = 0;
  const reader = { read: async () => ({ done: false, value: new TextEncoder().encode('event: rooms\ndata: {invalid}\n\n') }),
    cancel: async () => { cancelled++; }, releaseLock: () => { released++; } };
  const wire = streamHarness({ fetch: async () => ({ ok: true, status: 200, body: { getReader: () => reader } }) });
  await wire.run(); assert.equal(cancelled, 1); assert.equal(released, 1); assert.equal(wire.signals[0].aborted, true);
});

test('browser SSE authorization failures stop retries and show an actionable page error', async () => {
  for (const status of [401, 403]) {
    let cancelled = 0;
    const wire = streamHarness({ fetch: async () => ({ ok: false, status, body: { cancel: async () => { cancelled++; } } }) });
    await wire.run(); assert.equal(wire.signals.length, 1); assert.equal(wire.waits(), 0);
    assert.equal(cancelled, 1); assert.equal(wire.signals[0].aborted, true);
    assert.match(wire.errors[0], /reopen the full dashboard link/i);
    assert.equal(wire.connections.at(-1).text, 'Authorization required');
  }
  const refreshAuth = streamHarness({ fetch: async () => ({ ok: true, status: 200, body: { cancel: async () => {} } }),
    refreshList: async () => { throw Object.assign(new Error('Unauthorized.'), { status: 401 }); },
  });
  await refreshAuth.run(); assert.equal(refreshAuth.waits(), 0); assert.match(refreshAuth.errors[0], /reopen the full dashboard link/i);
});

test('dashboard authenticates every API and rejects foreign Host and Origin before side effects', async t => {
  const wire = await fixture(t);
  for (const path of ['/api/bootstrap', '/api/rooms', '/api/events', '/api/unknown']) {
    assert.equal((await fetch(wire.origin + path)).status, 401);
    assert.equal((await fetch(wire.origin + path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })).status, 401);
  }
  assert.equal((await wire.api('/api/bootstrap', { headers: { Authorization: 'Bearer wrong-token' } })).status, 401);
  assert.equal((await wire.api('/api/bootstrap', { headers: { Origin: 'https://untrusted.example' } })).status, 403);
  assert.equal((await raw(wire.origin, '/api/bootstrap', { Host: 'localhost:' + new URL(wire.origin).port, Authorization: 'Bearer ' + wire.token })).status, 403);
  assert.equal((await raw(wire.origin, '/', { Host: 'attacker.example' })).status, 403);
  assert.equal((await wire.api('/api/bootstrap', { headers: { Origin: wire.origin } })).status, 200);
  assert.deepEqual(wire.calls, []);
});

test('dashboard bootstrap exposes configured profiles and explicit limits without provider probes', async t => {
  const wire = await fixture(t), response = await wire.api('/api/bootstrap'), value = await response.json();
  assert.deepEqual(value, { backend: 'mock', agents: [
    { agent: 'developer', profile: 'codex', model: 'explicit-model', effort: 'high', authority: 2 },
    { agent: 'reviewer', profile: 'claude', model: null, effort: null, authority: 1 },
  ], rooms: [], defaults: { maxCalls: 32, contextLimit: null }, limits: { requestBytes: 262144 } });
  assert.equal(response.headers.get('access-control-allow-origin'), null);
  assert.deepEqual(wire.calls, []);
});

test('dashboard routes room creation, PM messages, task additions and lifecycle actions', async t => {
  const wire = await fixture(t);
  const input = { title: '<script>untrusted title</script>', brief: 'Offline task brief', members: [{ agent: 'developer', role: 'lead', weight: 2 }], tasks: [] };
  const created = await wire.post('/api/rooms', input); assert.equal(created.status, 201);
  const room = await created.json(); assert.equal(room.title, input.title);
  assert.equal(room.backend, 'mock');
  assert.equal((await (await wire.api('/api/rooms')).json()).length, 1);
  assert.equal((await (await wire.api('/api/rooms/' + room.id)).json()).id, room.id);
  const message = await (await wire.post('/api/rooms/' + room.id + '/messages', { author: 'PM', body: 'A human constraint.' })).json();
  assert.equal(message.messages.at(-1).body, 'A human constraint.');
  const task = await (await wire.post('/api/rooms/' + room.id + '/tasks', { title: 'Verify', brief: 'Use an offline fixture.', permission: 'read' })).json();
  assert.equal(task.tasks.at(-1).title, 'Verify');
  for (const [action, status] of [['discuss', 'discussing'], ['execute', 'running'], ['cancel', 'cancelled']]) {
    const response = await wire.post('/api/rooms/' + room.id + '/' + action);
    assert.equal(response.status, 200); assert.equal((await response.json()).status, status);
  }
  assert.deepEqual(wire.calls, ['create', 'message', 'task', 'discuss', 'execute', 'cancel']);
  assert.equal((await wire.api('/api/rooms/missing')).status, 404);
  assert.equal((await wire.api('/api/rooms/' + room.id + '/cancel')).status, 405);
});

test('dashboard preserves full context defaults and exposes complete paged history', async t => {
  const wire = await fixture(t);
  const response = await wire.post('/api/rooms', { title: 'Full context', brief: 'Keep all relevant evidence.', members: [], tasks: [] });
  const room = await response.json(); assert.equal(room.contextLimit, null); assert.equal(room.maxCalls, 32);
  const stored = wire.records.get(room.id);
  stored.messages = Array.from({ length: 105 }, (_, index) => ({ id: String(index), author: 'PM', kind: 'human', body: 'Complete message ' + index, createdAt: new Date().toISOString() }));
  const first = await (await wire.api('/api/rooms/' + room.id + '/history')).json();
  assert.equal(first.messages.length, 100); assert.equal(first.messages[0].body, 'Complete message 0'); assert.equal(first.nextCursor, 100);
  const second = await (await wire.api('/api/rooms/' + room.id + '/history?cursor=' + first.nextCursor)).json();
  assert.equal(second.messages.length, 5); assert.equal(second.messages.at(-1).body, 'Complete message 104'); assert.equal(second.nextCursor, null);
  assert.equal((await wire.api('/api/rooms/' + room.id + '/history?cursor=-1')).status, 400);
});

test('dashboard rejects oversized, malformed and structurally excessive JSON before invoking rooms', async t => {
  const wire = await fixture(t);
  assert.equal((await wire.post('/api/rooms', { brief: 'x'.repeat(256 * 1024) })).status, 413);
  assert.equal((await wire.api('/api/rooms', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{broken}' })).status, 400);
  assert.equal((await wire.post('/api/rooms', [])).status, 400);
  assert.equal((await wire.api('/api/rooms', { method: 'POST', body: '{}' })).status, 415);
  let depth = 'leaf'; for (let index = 0; index < 70; index++) depth = { child: depth };
  assert.equal((await wire.post('/api/rooms', depth)).status, 400);
  assert.deepEqual(wire.calls, []);
});

test('dashboard SSE emits room changes only and closing the HTTP UI preserves parent ownership', { timeout: 7000 }, async t => {
  const wire = await fixture(t), controller = new AbortController();
  t.after(() => controller.abort());
  const response = await wire.api('/api/events', { signal: controller.signal });
  assert.equal(response.headers.get('content-type'), 'text/event-stream; charset=utf-8');
  const reader = response.body.getReader();
  assert.match(new TextDecoder().decode((await reader.read()).value), /^: connected/);
  let received = false;
  const next = reader.read().then(value => { received = true; return value; });
  await pause(900); assert.equal(received, false, 'Unchanged local polling must not emit room snapshots.');
  const room = wire.rooms.create({ title: 'A real change', brief: 'Fixture', members: [], tasks: [] });
  const event = new TextDecoder().decode((await next).value);
  assert.ok(event.startsWith('event: rooms\ndata: ')); assert.ok(event.includes(room.id));
  controller.abort(); await reader.cancel().catch(() => {});
  const close = wire.dashboard.close(); assert.equal(close, wire.dashboard.close()); await close;
  assert.deepEqual(wire.ownership, { service: 0, rooms: 0 });
});

test('dashboard bounds concurrent authenticated SSE connections', { timeout: 7000 }, async t => {
  const wire = await fixture(t), controllers = [], readers = [];
  try {
    for (let index = 0; index < 16; index++) {
      const controller = new AbortController(); controllers.push(controller);
      const response = await wire.api('/api/events', { signal: controller.signal });
      assert.equal(response.status, 200);
      // Retain each live body; otherwise fetch may cancel an abandoned stream during GC.
      const reader = response.body.getReader(); readers.push(reader);
      assert.match(new TextDecoder().decode((await reader.read()).value), /^: connected/);
    }
    global.gc?.(); await pause(0);
    const refused = await wire.api('/api/events'); assert.equal(refused.status, 429);
    assert.match((await refused.json()).error, /16/);
  } finally {
    for (const controller of controllers) controller.abort();
    await Promise.allSettled(readers.map(reader => reader.cancel()));
    for (const reader of readers) reader.releaseLock();
  }
});
