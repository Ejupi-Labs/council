import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { createContext, runInContext } from 'node:vm';

// The source fallback permits this isolated UI suite before the sole build owner compiles dist.
const moduleUrl = new URL('../dist/dashboard-model-map-ui.js', import.meta.url);
const { dashboardModelMapMarkup, dashboardModelMapStyles, dashboardModelMapScript } = await import(
  existsSync(moduleUrl) ? moduleUrl.href : new URL('../src/dashboard-model-map-ui.ts', import.meta.url).href);

function deferred() { let resolve, reject; const promise = new Promise((accept, fail) => { resolve = accept; reject = fail; }); return { promise, resolve, reject }; }
function fixture(label = 'Current model', count = 1) {
  const source = { id: 'owner', title: 'Benchmark owner', url: 'https://example.org/evidence?metric=code', publisher: 'Owner', type: 'benchmark-owner', retrievedAt: '2026-11-05', publishedAt: '2026-10-27' };
  const models = Array.from({ length: count }, (_, index) => ({ id: 'exact-model-' + index, name: index ? label + ' ' + index : label, provider: 'Provider', aliases: ['exact alias +' + index], status: 'verified', strengths: ['Scoped implementation'], limitations: ['Account access unknown'], effortNotes: 'High is a recorded configuration.', harnesses: ['Exact harness'], evidenceIds: ['observation-' + index], recommendations: [] }));
  const observations = models.map((model, index) => ({ id: 'observation-' + index, modelId: model.id, sourceId: source.id, benchmark: 'ExampleBench', version: '4.0', metric: 'Pass rate', score: 70 - index, unit: '%', direction: 'higher', cohort: 'owner-2026-10-27', harness: 'Exact harness', effort: 'High', uncertainty: '±2 percentage points', sampleSize: 113, notes: 'Independent recorded protocol.' }));
  const hierarchy = models.map((model, index) => ({ task: 'repository-work', role: 'worker', priority: index + 1, basis: 'benchmark', confidence: 'medium', reason: 'Reason for ' + model.name, sourceIds: [source.id], modelId: model.id, modelName: model.name, status: model.status, tied: index === 0 }));
  return { catalog: { asOf: '2026-11-05', revision: 3, sources: [source], observations, models, tasks: [{ id: 'repository-work', label: 'Repository work', description: 'Tasks with repository evidence.' }] }, query: {}, models, hierarchy, rankings: [{ cohort: 'owner-2026-10-27', benchmark: 'ExampleBench', version: '4.0', metric: 'Pass rate', unit: '%', direction: 'higher', entries: observations.map((item, index) => ({ rank: index + 3, observationId: item.id, modelId: item.modelId, score: item.score, harness: item.harness, effort: item.effort })) }], bindings: [{ alias: 'worker-a', profile: 'claude', model: models[0].id, catalogModelId: models[0].id, match: 'exact', enabled: true }, { alias: 'default-b', profile: 'junie', model: null, match: 'unresolved', enabled: false }, { alias: 'custom-c', profile: 'custom', model: 'not-in-catalog', match: 'unknown', enabled: true }], warnings: ['Separate cohorts are never averaged.'], overrideApplied: false, overridePath: 'C:\\workspace\\.council\\model-map.json' };
}
function harness(handler = () => fixture()) {
  const nodes = new Map(), closes = [], calls = [];
  class Node {
    constructor(tag) { this.tag = tag; this.children = []; this.listeners = new Map(); this.textContent = ''; this.value = ''; this.hidden = false; this.open = false; this.disabled = false; }
    append(...children) { this.children.push(...children); }
    replaceChildren(...children) { this.children = children; }
    setAttribute(key, value) { this[key] = value; }
    addEventListener(event, handler) { if (!this.listeners.has(event)) this.listeners.set(event, []); this.listeners.get(event).push(handler); }
    fire(event) { return Promise.all((this.listeners.get(event) ?? []).map(handler => handler({ preventDefault() {} }))); }
    showModal() { this.open = true; }
    close() { if (this.open) { this.open = false; closes.push(() => this.fire('close')); } }
    escape() { this.fire('cancel'); this.close(); }
  }
  const $ = id => { if (!nodes.has(id)) nodes.set(id, new Node(id)); return nodes.get(id); };
  const context = createContext({ Promise, AbortController, URL, URLSearchParams, $, token: 'local-session', document: { createElement: tag => new Node(tag) },
    api: async (path, body, signal) => { calls.push({ path, body, signal }); return await handler(path, body, signal); },
  });
  runInContext(dashboardModelMapScript + '\ninstallModelMapUI();', context);
  return { $, calls, context, run: source => runInContext(source, context), open: () => runInContext('openModelMap()', context),
    async flushClose() { for (const close of closes.splice(0)) await close(); },
  };
}
function text(node) { return [node.textContent, ...node.children.map(text)].join('\n'); }
function findAll(node, predicate) { return [...predicate(node) ? [node] : [], ...node.children.flatMap(child => findAll(child, predicate))]; }

test('model-map dialog exposes accessible English exact filters and distinct evidence sections', async () => {
  assert.match(dashboardModelMapMarkup, /aria-labelledby="modelMapTitle"/);
  assert.match(dashboardModelMapMarkup, /aria-describedby="modelMapDescription"/);
  assert.match(dashboardModelMapMarkup, /Exact model ID or alias/);
  assert.match(dashboardModelMapMarkup, /id="modelMapRole"/);
  assert.match(dashboardModelMapMarkup, /Declared harness compatibility/);
  assert.match(dashboardModelMapMarkup, /role="status" aria-live="polite"/);
  assert.match(dashboardModelMapStyles, /overflow:auto/);
  assert.equal(dashboardModelMapScript.includes('innerHTML'), false);
  const wire = harness(); await wire.open();
  const rendered = text(wire.$('modelMapResults'));
  assert.match(rendered, /Task and role priorities/); assert.match(rendered, /Measured benchmark cohorts/);
  assert.match(rendered, /not measured intelligence scores or runtime authority/);
  assert.match(rendered, /Priority 1 · tied/); assert.match(rendered, /benchmark · medium confidence/);
  assert.match(rendered, /Catalog cohort rank/); assert.match(rendered, /not full source leaderboard positions or statistical significance/);
  assert.match(rendered, /\n3\n/, 'Filtering preserves the catalog cohort rank.');
  assert.match(wire.$('modelMapMetadata').textContent, /as of 2026-11-05 · revision 3/);
});

test('filter controls have stable explicit accessible names independent of their dynamic option lists', () => {
  for (const [control, label] of [['modelMapTask', 'modelMapTaskLabel'], ['modelMapModel', 'modelMapModelLabel'], ['modelMapRole', 'modelMapRoleLabel'], ['modelMapHarness', 'modelMapHarnessLabel']]) {
    assert.match(dashboardModelMapMarkup, new RegExp('<label[^>]+for="' + control + '"'));
    assert.match(dashboardModelMapMarkup, new RegExp('<(?:input|select)[^>]+id="' + control + '"[^>]+aria-labelledby="' + label + '"'));
    assert.match(dashboardModelMapMarkup, new RegExp('<span id="' + label + '">'));
  }
  for (const label of dashboardModelMapMarkup.matchAll(/<label\b[^>]*>[\s\S]*?<\/label>/g)) {
    assert.equal(label[0].includes('<datalist'), false, 'Dynamic model names must not become part of an input label.');
  }
});

test('model-map renders complete literal evidence, bindings, pricing and dated sources using safe links', async () => {
  const view = fixture('<img src=x onerror=alert(1)>'), literal = '<script>literal evidence</script>\n' + 'Complete context. '.repeat(2000);
  view.models[0].strengths = [literal]; view.models[0].limitations = [literal]; view.catalog.observations[0].notes = literal;
  view.models[0].referencePricing = { inputPerMillion: 0.125, outputPerMillion: 0.5, currency: 'USD', sourceId: 'owner' };
  view.catalog.sources.push({ ...view.catalog.sources[0], id: 'unsafe', title: 'Unsafe literal source', url: 'javascript:alert(1)' }, { ...view.catalog.sources[0], id: 'credentials', title: 'Credential URL', url: 'https://user:password@example.org/' });
  const wire = harness(() => view); await wire.open();
  const root = wire.$('modelMapResults'), rendered = text(root), links = findAll(root, node => node.tag === 'a');
  assert.ok(rendered.includes(literal)); assert.ok(rendered.includes(view.models[0].name));
  assert.match(rendered, /Harness: Exact harness · Effort: High/); assert.match(rendered, /Sample size: 113/);
  assert.match(rendered, /±2 percentage points/); assert.match(rendered, /retrieved 2026-11-05 · published 2026-10-27/);
  assert.match(rendered, /USD per million tokens: input 0.125 · output 0.5/);
  assert.match(rendered, /worker-a · claude/); assert.match(rendered, /default-b · junie/);
  assert.match(rendered, /Native default · unresolved/); assert.match(rendered, /not-in-catalog/);
  assert.equal(findAll(root, node => node.tag === 'script' || node.tag === 'img').length, 0);
  assert.ok(links.length > 0); assert.ok(links.every(node => node.href === 'https://example.org/evidence?metric=code' && node.rel === 'noopener noreferrer' && node.target === '_blank'));
  assert.equal(links.some(node => node.textContent === 'Unsafe literal source' || node.textContent === 'Credential URL'), false);
  assert.ok(wire.calls.every(call => call.body === undefined && call.path.startsWith('/api/model-map')));
});

test('model-map sends exact task/model/role/harness filters without inferred or trimmed substitutions', async () => {
  const wire = harness(); await wire.open();
  wire.$('modelMapTask').value = 'repository-work'; wire.$('modelMapModel').value = ' exact alias +0 ';
  wire.$('modelMapRole').value = 'reviewer'; wire.$('modelMapHarness').value = 'Exact harness / custom';
  await wire.$('modelMapFilters').fire('submit');
  const request = new URL(wire.calls.at(-1).path, 'http://localhost');
  assert.equal(request.searchParams.get('includeFullCatalog'), 'true', 'Complete local filter choices are explicitly requested.');
  assert.equal(request.searchParams.get('task'), 'repository-work'); assert.equal(request.searchParams.get('model'), ' exact alias +0 ');
  assert.equal(request.searchParams.get('role'), 'reviewer'); assert.equal(request.searchParams.get('harness'), 'Exact harness / custom');
  assert.equal(wire.$('modelMapModel').value, ' exact alias +0 ');
  await wire.$('modelMapReset').fire('click'); assert.equal(wire.calls.at(-1).path, '/api/model-map?includeFullCatalog=true');
});

test('task priorities start collapsed by task, selected tasks open, and cohort identifiers disambiguate summaries', async () => {
  const view = fixture('Grouped model', 3);
  view.catalog.tasks.push({ id: 'writing-editing', label: 'Writing and editing', description: 'Audience-specific prose.' });
  view.hierarchy[2].task = 'writing-editing';
  const wire = harness(() => view); await wire.open();
  const planning = wire.$('modelMapResults').children[0], groups = planning.children.filter(node => node.tag === 'details');
  assert.equal(groups.length, 2); assert.ok(groups.every(node => node.open === false));
  assert.match(text(groups[0].children[0]), /Repository work · 2 planning entries/);
  assert.match(text(groups[1].children[0]), /Writing and editing · 1 planning entry/);
  assert.equal(findAll(planning, node => node.tag === 'tr').length, 5, 'Collapsed groups retain all rows and both headers.');
  const measured = wire.$('modelMapResults').children[1], cohort = measured.children.find(node => node.tag === 'details');
  assert.match(text(cohort.children[0]), /owner-2026-10-27/);
  view.query = { task: 'writing-editing' }; view.hierarchy = view.hierarchy.filter(item => item.task === view.query.task);
  await wire.$('modelMapReload').fire('click');
  const selectedGroups = wire.$('modelMapResults').children[0].children.filter(node => node.tag === 'details');
  assert.equal(selectedGroups.length, 1); assert.equal(selectedGroups[0].open, true);
  assert.match(text(wire.$('modelMapResults')), /Legacy means previous generation; it does not mean unavailable/);
});

test('benchmark harness labels never become compatibility choices when profiles have no declarations', async () => {
  const view = fixture(); delete view.models[0].harnesses;
  const wire = harness(() => view); await wire.open();
  assert.equal(wire.$('modelMapHarness').disabled, true);
  assert.equal(wire.$('modelMapHarnessLabel').textContent, 'No compatibility declarations');
  assert.equal(wire.$('modelMapHarnesses').children.length, 0);
  assert.match(text(wire.$('modelMapResults')), /Harness: Exact harness · Effort: High/);
  assert.match(wire.$('modelMapStatus').textContent, /1 model profiles/);
});

test('a harness filter remains editable after a reference reload removes its compatibility declarations', async () => {
  let declared = true;
  const wire = harness(path => {
    const view = fixture(), selected = new URL(path, 'http://localhost').searchParams.get('harness');
    if (!declared) delete view.catalog.models[0].harnesses;
    if (selected) {
      view.query.harness = selected;
      if (!declared) { view.models = []; view.hierarchy = []; view.rankings = []; view.warnings = ['No catalog profile declares this exact harness.']; }
    }
    return view;
  });
  await wire.open(); wire.$('modelMapHarness').value = 'Exact harness';
  await wire.$('modelMapFilters').fire('submit');
  declared = false; await wire.$('modelMapReload').fire('click');
  assert.equal(wire.$('modelMapHarness').value, 'Exact harness', 'An active exact filter is not silently rewritten.');
  assert.equal(wire.$('modelMapHarness').disabled, false, 'The obsolete filter must remain available to remove.');
  assert.equal(wire.$('modelMapHarnesses').children.length, 0);
  assert.match(text(wire.$('modelMapWarnings')), /No catalog profile declares/);
  wire.$('modelMapHarness').value = ''; await wire.$('modelMapFilters').fire('input');
  await wire.$('modelMapFilters').fire('submit');
  assert.equal(new URL(wire.calls.at(-1).path, 'http://localhost').searchParams.has('harness'), false);
  assert.equal(wire.$('modelMapHarness').disabled, true);
  assert.match(wire.$('modelMapStatus').textContent, /1 model profiles/);
});

test('close/reopen aborts old reads and ignores late failures and queued native close events', async () => {
  const old = deferred(), current = deferred(); let calls = 0;
  const wire = harness(() => ++calls === 1 ? old.promise : current.promise);
  const opening = wire.open(); wire.run('dismissModelMap()'); const reopening = wire.open(), epoch = wire.run('modelMapEpoch');
  await wire.flushClose(); assert.equal(wire.run('modelMapEpoch'), epoch); assert.equal(wire.calls[0].signal.aborted, true); assert.equal(wire.calls[1].signal.aborted, false);
  old.reject(new Error('Obsolete model-map read failure.')); await opening;
  assert.equal(wire.$('modelMapApply').disabled, true); assert.equal(wire.$('modelMapError').hidden, true);
  current.resolve(fixture('Reopened model')); await reopening;
  assert.ok(text(wire.$('modelMapResults')).includes('Reopened model')); assert.equal(wire.$('modelMapError').hidden, true);
  assert.equal(wire.$('modelMapApply').disabled, false);
});

test('editing filters invalidates pending results and older reads cannot repaint a newer query', async () => {
  const old = deferred(), current = deferred(); let calls = 0;
  const wire = harness(() => ++calls === 1 ? old.promise : current.promise);
  const opening = wire.open(); wire.$('modelMapModel').value = 'new exact model'; await wire.$('modelMapFilters').fire('input');
  assert.equal(wire.calls[0].signal.aborted, true); assert.equal(wire.$('modelMapResults').children.length, 0);
  const reloading = wire.$('modelMapFilters').fire('submit'); old.resolve(fixture('Obsolete model')); await opening;
  assert.equal(wire.$('modelMapResults').children.length, 0); assert.equal(wire.$('modelMapApply').disabled, true);
  const latest = fixture('Latest model'); latest.query = { model: 'new exact model' }; current.resolve(latest); await reloading;
  assert.ok(text(wire.$('modelMapResults')).includes('Latest model')); assert.equal(text(wire.$('modelMapResults')).includes('Obsolete model'), false);
  assert.equal(wire.$('modelMapModel').value, 'new exact model'); assert.match(wire.$('modelMapStatus').textContent, /model=new exact model/);
});

test('Escape and direct native close invalidate pending responses before they settle', async () => {
  for (const escape of [false, true]) {
    const pending = deferred(), wire = harness(() => pending.promise), opening = wire.open();
    if (escape) wire.$('modelMapDialog').escape(); else wire.$('modelMapDialog').close();
    await wire.flushClose(); assert.equal(wire.calls[0].signal.aborted, true);
    pending.resolve(fixture('Closed result')); await opening;
    assert.equal(wire.$('modelMapResults').children.length, 0); assert.equal(wire.$('modelMapDialog').open, false);
  }
});

test('errors remain retryable and empty exact matches preserve warnings instead of substituting a model', async () => {
  let calls = 0; const empty = fixture(); empty.models = []; empty.hierarchy = []; empty.rankings = []; empty.query = { model: 'unknown-id' }; empty.warnings = ['Unknown exact model ID. No substitute selected.'];
  const wire = harness(() => ++calls === 1 ? Promise.reject(new Error('Local model-map override is invalid.')) : empty);
  await wire.open(); assert.equal(wire.$('modelMapError').hidden, false); assert.match(wire.$('modelMapError').textContent, /override is invalid/);
  assert.equal(wire.$('modelMapApply').disabled, false); await wire.$('modelMapReload').fire('click');
  assert.equal(wire.$('modelMapError').hidden, true); assert.equal(wire.$('modelMapWarnings').hidden, false);
  assert.match(text(wire.$('modelMapWarnings')), /No substitute selected/);
  assert.match(text(wire.$('modelMapResults')), /Missing evidence remains unranked/);
  assert.match(wire.$('modelMapStatus').textContent, /0 model profiles · 0 planning entries · 0 measured cohorts/);
});

test('model-map renders every returned profile, priority, source and cohort entry without hard data clipping', async () => {
  const view = fixture('Full catalogue model', 137); view.query.includeFullCatalog = true; const wire = harness(() => view); await wire.open();
  const rendered = text(wire.$('modelMapResults'));
  assert.match(wire.$('modelMapStatus').textContent, /137 model profiles · 137 planning entries/);
  assert.ok(rendered.includes('Full catalogue model 136')); assert.ok(rendered.includes('Reason for Full catalogue model 136'));
  assert.equal(findAll(wire.$('modelMapModels'), node => node.tag === 'option').length, 274);
  assert.equal(findAll(wire.$('modelMapResults'), node => node.tag === 'tr').length, 280, 'All priority/ranking/binding rows plus headers are present.');
  assert.ok(wire.calls.every(call => call.body === undefined)); assert.equal(wire.calls.length, 1);
  assert.equal(new URL(wire.calls[0].path, 'http://localhost').searchParams.get('includeFullCatalog'), 'true');
  assert.equal(wire.$('modelMapStatus').textContent.includes('includeFullCatalog'), false, 'Transport scope is not displayed as a user filter.');
});
