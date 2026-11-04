import test from 'node:test';
import assert from 'node:assert/strict';
import { createContext, runInContext, Script } from 'node:vm';
import { billingClientSource, billingInput, billingPatch, billingRaw, billingStable } from '../dist/dashboard-billing-ui.js';

function deferred() {
  let resolve, reject;
  const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

function record(alias, changes = {}) {
  return { alias, revision: 1, enabled: true, profile: 'codex', model: null, mode: 'unknown', accountKey: null,
    plan: 'Original ' + alias.toUpperCase(), source: null, observedAt: null, validUntil: null, stale: false,
    windows: [], prices: null, routing: null, skills: null, ...changes };
}

function draftFor(value, plan) {
  const initial = billingRaw(value);
  return { base: structuredClone(value), initial: structuredClone(initial), raw: plan === undefined ? initial : { ...initial, plan }, generation: plan === undefined ? 0 : 1 };
}

function reloadHarness() {
  const response = deferred(), elements = new Map(), shown = [], errors = [];
  const element = id => {
    if (!elements.has(id)) elements.set(id, { open: id === 'billingDialog', hidden: false, textContent: '' });
    return elements.get(id);
  };
  const a = record('a'), b = record('b');
  let context;
  context = createContext({ Promise, AbortController, billingStable, $: element,
    api: async () => response.promise, billingCapture() {}, billingSaveState() {}, billingRenderAliases() {},
    billingFailure: message => errors.push(message),
    billingShow(alias) {
      shown.push(alias); context.billingSelected = alias;
      if (!context.billingDrafts.has(alias)) context.billingDrafts.set(alias, draftFor(context.billingRecords.get(alias)));
    },
  });
  Object.assign(context, { billingEpoch: 1, billingSelected: 'a', billingLoading: false, billingController: undefined,
    billingRecords: new Map([['a', a], ['b', b]]), billingDrafts: new Map([['a', draftFor(a, 'Unsaved A')], ['b', draftFor(b, 'Unsaved B')]]),
    billingPending: new Map(), billingSkills: [] });
  const lines = billingClientSource().split(/\r?\n/);
  const source = ['function billingMerge(', 'async function billingLoad('].map(prefix => {
    const line = lines.find(value => value.startsWith(prefix)); assert.ok(line, prefix); return line;
  }).join('\n');
  runInContext(source, context);
  return { context, response, shown, errors, load: () => runInContext('billingLoad(true)', context),
    finish() { response.resolve({ agents: [record('a', { revision: 2, plan: 'Latest A' }), record('b', { revision: 2, plan: 'Latest B' })], skills: [] }); },
  };
}

test('discard reload targets the originally selected alias, not an alias selected while loading', async () => {
  const wire = reloadHarness(), originalB = wire.context.billingDrafts.get('b');
  const loading = wire.load(); wire.context.billingSelected = 'b';
  originalB.raw.plan = 'Newer unsaved B'; originalB.generation++;
  wire.finish(); await loading;
  assert.equal(wire.context.billingSelected, 'b'); assert.equal(wire.context.billingDrafts.has('a'), false);
  assert.equal(wire.context.billingDrafts.get('b'), originalB); assert.equal(originalB.raw.plan, 'Newer unsaved B');
  assert.deepEqual(wire.shown, ['b']); assert.deepEqual(wire.errors, []);
});

test('discard reload preserves edits made after the request began', async () => {
  const wire = reloadHarness(), original = wire.context.billingDrafts.get('a');
  const loading = wire.load(); original.raw.plan = 'Keep this later edit'; original.generation++;
  wire.finish(); await loading;
  assert.equal(wire.context.billingDrafts.get('a'), original); assert.equal(original.raw.plan, 'Keep this later edit');
  assert.equal(original.base.revision, 1); assert.equal(wire.context.billingRecords.get('a').revision, 2);
});

test('discard reload preserves a newer generation even when values return to the original baseline', async () => {
  const wire = reloadHarness(), original = wire.context.billingDrafts.get('a');
  const loading = wire.load(); original.raw.plan = original.initial.plan; original.generation++;
  wire.finish(); await loading;
  assert.equal(wire.context.billingDrafts.get('a'), original); assert.equal(original.raw.plan, 'Original A');
});

test('discard reload cannot erase a replacement draft with a reused generation number', async () => {
  const wire = reloadHarness(), loading = wire.load();
  const replacement = draftFor(record('a'), 'Replacement draft'); wire.context.billingDrafts.set('a', replacement);
  wire.finish(); await loading;
  assert.equal(wire.context.billingDrafts.get('a'), replacement); assert.equal(replacement.raw.plan, 'Replacement draft');
});

test('discard reload still replaces its unchanged target with the latest record', async () => {
  const wire = reloadHarness(), loading = wire.load(); wire.finish(); await loading;
  const latest = wire.context.billingDrafts.get('a'); assert.equal(latest.base.revision, 2); assert.equal(latest.raw.plan, 'Latest A');
  assert.equal(wire.context.billingDrafts.get('b').raw.plan, 'Unsaved B');
});

function importedRecord() {
  return record('a', { source: { kind: 'reported', label: 'Imported record' }, observedAt: '2026-11-04T08:00:00.000Z',
    plan: ' Original plan label ', prices: { currency: null, inputPerMillion: 1.25, outputPerMillion: null },
    routing: { tier: 'small', capabilities: ['code, review', ' C++ '], costRank: 1, qualityRank: 2 },
    windows: [
      { id: 'period', label: 'Requests', unit: 'credits', limit: 100, remaining: null, used: 0, resetAt: null },
      { id: ' period ', label: ' Another reported period ', unit: ' requests ', limit: null, remaining: 0, used: null, resetAt: null },
    ] });
}

test('unrelated billing edits preserve exact imported window and capability labels', () => {
  const base = importedRecord(), raw = billingRaw(base); raw.plan = 'Updated plan';
  const next = billingInput(raw, base), patch = billingPatch(base, next);
  assert.deepEqual(patch, { plan: 'Updated plan' });
  assert.deepEqual(next.windows, base.windows.map(window => ({ ...window, scope: 'other' }))); assert.deepEqual(next.routing.capabilities, base.routing.capabilities);
  assert.deepEqual(next.prices, base.prices);
});

test('editing ranks preserves unchanged capability names containing commas or whitespace', () => {
  const base = importedRecord(), raw = billingRaw(base); raw.costRank = '3.5';
  const next = billingInput(raw, base), patch = billingPatch(base, next);
  assert.deepEqual(Object.keys(patch), ['routing']); assert.equal(next.routing.costRank, 3.5);
  assert.deepEqual(next.routing.capabilities, ['code, review', ' C++ ']); assert.equal(next.plan, ' Original plan label ');
  raw.capabilities = 'code, tests'; assert.deepEqual(billingInput(raw, base).routing.capabilities, ['code', 'tests']);
});

test('amount edits preserve exact window identities and the entered observation timestamp', () => {
  const base = importedRecord(), raw = billingRaw(base); raw.windows[1].remaining = '7';
  const patch = billingPatch(base, billingInput(raw, base));
  assert.equal(patch.windows[1].id, ' period '); assert.equal(patch.windows[1].label, ' Another reported period ');
  assert.equal(patch.windows[1].unit, ' requests '); assert.equal(patch.windows[1].remaining, 7);
  assert.equal(patch.observedAt, base.observedAt); assert.equal(Object.hasOwn(patch, 'routing'), false);
  raw.windows[1].unit = ' '; assert.throws(() => billingInput(raw, base), /needs a key, label, and unit/);
});

test('configured prices may retain an unknown currency without guessing one', () => {
  const raw = billingRaw(record('a')); raw.inputPerMillion = '0'; raw.outputPerMillion = '1.25';
  assert.deepEqual(billingInput(raw).prices, { currency: null, inputPerMillion: 0, outputPerMillion: 1.25 });
});

test('billing client remains valid browser JavaScript', () => {
  new Script(billingClientSource());
});
