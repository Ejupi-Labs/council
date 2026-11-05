import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, rmSync, statSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { modelMapView, validateModelMap } from '../dist/model-map.js';
import { MODEL_MAP_CATALOG } from '../dist/model-map-data.js';

const source = (id = 'independent-owner', extra = {}) => ({ id, title: 'Official benchmark evidence', url: 'https://benchmark.example.test/results',
  retrievedAt: '2026-11-05', publisher: 'Benchmark owner', type: 'benchmark-owner', ...extra });
const recommendation = (priority = 1, extra = {}) => ({ task: 'implementation', priority, role: 'worker', basis: 'benchmark', confidence: 'medium',
  reason: 'This recorded task benchmark supports a planning preference; it does not establish general intelligence.', sourceIds: ['independent-owner'], ...extra });
const model = (id, extra = {}) => ({ id, name: 'Model ' + id, provider: 'Provider ' + id, status: 'verified', strengths: ['Recorded coding evidence'],
  limitations: ['Recorded harness only'], evidenceIds: [id + '-quality'], recommendations: [recommendation()], harnesses: ['harness-A'], ...extra });
const observation = (id, modelId, score, extra = {}) => ({ id, sourceId: 'independent-owner', modelId, benchmark: 'Code fixture', version: '2026-11',
  metric: 'resolved', score, unit: 'percent', direction: 'higher', harness: 'harness-A', effort: 'high', cohort: 'independent-code-fixture', ...extra });
function catalog() {
  return { asOf: '2026-11-05', revision: 1,
    sources: [source(), source('vendor', { title: 'Official provider guide', url: 'https://vendor.example.test/models', publisher: 'Provider', type: 'vendor' }),
      source('methodology', { title: 'Methodology and limitations', url: 'https://benchmark.example.test/method', type: 'methodology' })],
    tasks: [{ id: 'implementation', label: 'Implementation', description: 'Implement and verify a scoped change.' },
      { id: 'review', label: 'Review', description: 'Independently review retained evidence.' }],
    observations: [observation('alpha-quality', 'alpha-v1', 95), observation('beta-quality', 'beta-v1', 80),
      observation('alpha-cost', 'alpha-v1', 9, { benchmark: 'Measured cost fixture', metric: 'cost', score: 9, unit: 'USD', direction: 'lower', cohort: 'independent-cost-fixture' }),
      observation('beta-cost', 'beta-v1', 2, { benchmark: 'Measured cost fixture', metric: 'cost', score: 2, unit: 'USD', direction: 'lower', cohort: 'independent-cost-fixture' })],
    models: [model('alpha-v1', { aliases: ['alpha-exact-alias'], evidenceIds: ['alpha-quality', 'alpha-cost'] }),
      model('beta-v1', { evidenceIds: ['beta-quality', 'beta-cost'], recommendations: [recommendation(1), recommendation(2, { task: 'review', role: 'reviewer', basis: 'policy-inference', sourceIds: ['methodology'] })] }),
      model('unverified-name', { status: 'unverified', evidenceIds: [], strengths: [], limitations: ['No verified model identity or benchmark'],
        recommendations: [recommendation(1, { basis: 'policy-inference', sourceIds: ['methodology'] })] })] };
}
function fixture(t, base = catalog()) {
  const parent = resolve(tmpdir()), root = mkdtempSync(join(parent, 'council-model-map-')), stateDir = join(root, '.council');
  const config = { workspaceRoot: root, stateDir, agents: {
    native: { enabled: true, profile: 'codex' }, exact: { enabled: true, profile: 'junie', model: 'alpha-v1' },
    alias: { enabled: false, profile: 'claude', model: 'alpha-exact-alias' }, unknown: { enabled: true, profile: 'antigravity', model: 'Gemini 3.8' },
  } };
  t.after(() => { assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-model-map-')); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  return { root, stateDir, config, base, view: input => modelMapView(config, input, base),
    overlay(value) { mkdirSync(stateDir, { recursive: true }); writeFileSync(join(stateDir, 'model-map.json'), typeof value === 'string' ? value : JSON.stringify(value)); } };
}

test('benchmark cohorts rank independently in their declared direction and task hierarchy keeps explicit ordinal ties', t => {
  const wire = fixture(t), view = wire.view();
  const quality = view.rankings.find(group => group.cohort === 'independent-code-fixture'), cost = view.rankings.find(group => group.cohort === 'independent-cost-fixture');
  assert.deepEqual(quality.entries.map(entry => [entry.modelId, entry.rank]), [['alpha-v1', 1], ['beta-v1', 2]]);
  assert.deepEqual(cost.entries.map(entry => [entry.modelId, entry.rank]), [['beta-v1', 1], ['alpha-v1', 2]]);
  assert.equal(view.rankings.length, 2); assert.equal(Object.hasOwn(view, 'intelligenceScore'), false); assert.equal(Object.hasOwn(view, 'overallRank'), false);
  const workers = view.hierarchy.filter(item => item.task === 'implementation' && item.role === 'worker');
  assert.deepEqual(workers.map(item => [item.modelId, item.priority, item.tied]), [['alpha-v1', 1, true], ['beta-v1', 1, true]]);
  assert.equal(view.hierarchy.some(item => item.modelId === 'unverified-name'), false);
  assert.ok(view.warnings.some(value => value.includes('not intelligence scores')));
  assert.ok(view.warnings.some(value => value.includes('subset of the owner') && value.includes('not owner-global ranks')),
    'Ranks describe curated cohort entries, not the omitted owner-board models or statistical significance.');
});

test('model and task filters preserve the matching profile evidence and original cohort ranks without returning unrelated models', t => {
  const wire = fixture(t), filtered = wire.view({ model: 'beta-v1', task: 'implementation', role: 'worker' });
  assert.deepEqual(filtered.models.map(item => item.id), ['beta-v1']); assert.equal(filtered.hierarchy.length, 1); assert.equal(filtered.hierarchy[0].tied, true);
  assert.equal(filtered.rankings.find(group => group.cohort === 'independent-code-fixture').entries[0].rank, 2);
  assert.equal(filtered.rankings.find(group => group.cohort === 'independent-cost-fixture').entries[0].rank, 1);
  assert.deepEqual(filtered.catalog.models.map(model => model.id), ['beta-v1']); assert.equal(filtered.catalog.observations.length, 2);
  assert.equal(filtered.catalog.models[0].recommendations.length, 2, 'The selected model retains recommendations outside this display task.');
  assert.equal(filtered.catalog.tasks.length, wire.base.tasks.length); assert.equal(filtered.totalModels, 3); assert.equal(filtered.catalogScope, 'matching-models');
  const full = wire.view({ model: 'beta-v1', task: 'implementation', role: 'worker', includeFullCatalog: true });
  assert.deepEqual(full.catalog, validateModelMap(wire.base), 'Explicit full-catalog requests retain every validated source and observation.');
  assert.equal(full.catalogScope, 'full'); assert.equal(full.totalModels, 3);
  assert.deepEqual(wire.view({ model: 'alpha-exact-alias' }).models.map(item => item.id), ['alpha-v1']);
  assert.deepEqual(wire.view({ task: 'review', role: 'reviewer' }).hierarchy.map(item => item.modelId), ['beta-v1']);
});

test('matching-model catalogs retain every selected observation, recommendation, pricing citation and methodology source', t => {
  const base = catalog(), complete = 'Complete selected-profile evidence.\n'.repeat(3000);
  base.sources.push(source('unrelated-vendor', { type: 'vendor' }), source('extra-methodology', { type: 'methodology' }));
  base.models[1].referencePricing = { inputPerMillion: 1, outputPerMillion: 3, currency: 'USD', sourceId: 'vendor' };
  base.models[1].limitations = [complete]; base.observations[1].notes = complete;
  const wire = fixture(t, base), view = wire.view({ model: 'beta-v1', task: 'implementation', role: 'worker' });
  assert.deepEqual(view.catalog.sources.map(source => source.id), ['independent-owner', 'vendor', 'methodology', 'extra-methodology']);
  assert.equal(view.catalog.models[0].limitations[0], complete); assert.equal(view.catalog.observations.find(observation => observation.id === 'beta-quality').notes, complete);
  assert.deepEqual(view.catalog.observations.map(observation => observation.id), ['beta-quality', 'beta-cost']);
  assert.equal(view.catalog.models[0].recommendations.length, 2); assert.equal(view.catalog.models[0].referencePricing.sourceId, 'vendor');
  assert.deepEqual(validateModelMap(view.catalog), view.catalog, 'Projected evidence retains referential integrity.');
  assert.equal(view.bindings.length, Object.keys(wire.config.agents).length, 'Configuration bindings remain visible independently of reference filters.');
  const full = wire.view({ model: 'beta-v1', includeFullCatalog: true }); assert.equal(full.catalog.sources.length, 5); assert.equal(full.catalog.models.length, 3);
});

test('full-catalog mode is an explicit boolean opt-in and unknown models do not carry unrelated profile transcripts', t => {
  const wire = fixture(t), unknown = wire.view({ model: 'constructor' });
  assert.equal(Object.hasOwn(wire.view().query, 'includeFullCatalog'), false, 'No implicit property is inserted into an ordinary query.');
  assert.equal(unknown.catalogScope, 'matching-models'); assert.equal(unknown.totalModels, 3);
  assert.deepEqual(unknown.catalog.models, []); assert.deepEqual(unknown.catalog.observations, []);
  assert.deepEqual(unknown.catalog.sources.map(source => source.id), ['methodology']); assert.equal(unknown.catalog.tasks.length, 2);
  const full = wire.view({ model: 'constructor', includeFullCatalog: true }); assert.equal(full.catalogScope, 'full'); assert.equal(full.catalog.models.length, 3);
  assert.deepEqual(full.models, []); assert.deepEqual(full.hierarchy, []);
  assert.equal(wire.view({ includeFullCatalog: false }).catalogScope, 'matching-models');
  assert.throws(() => wire.view({ includeFullCatalog: 'true' }), /boolean/);
});

test('a compatibility harness filter retains the published benchmark harness without claiming a local replication', t => {
  const base = catalog(); base.models[0].harnesses = ['local-wrapper'];
  const wire = fixture(t, base), view = wire.view({ harness: 'local-wrapper' });
  assert.deepEqual(view.models.map(item => item.id), ['alpha-v1']);
  assert.equal(view.rankings.length, 2); assert.equal(view.rankings[0].entries[0].harness, 'harness-A');
  assert.ok(view.warnings.some(value => value.includes('does not establish local replication')));
});

test('exact bindings leave native defaults, generic labels and prototype names unresolved without guessing a substitute', t => {
  const wire = fixture(t), view = wire.view();
  assert.deepEqual(view.bindings.map(binding => [binding.alias, binding.match, binding.catalogModelId]), [
    ['native', 'unresolved', undefined], ['exact', 'exact', 'alpha-v1'], ['alias', 'exact', 'alpha-v1'], ['unknown', 'unknown', undefined],
  ]);
  assert.equal(view.bindings[0].model, null); assert.equal(view.bindings[2].enabled, false);
  for (const name of ['ALPHA-V1', 'Gemini 3.8', 'constructor', '__proto__', 'codex', ' alpha-v1']) {
    const unknown = wire.view({ model: name }); assert.deepEqual(unknown.models, []); assert.deepEqual(unknown.hierarchy, []);
    assert.deepEqual(unknown.rankings, []); assert.ok(unknown.warnings.some(value => value.includes('No substitute')));
  }
  assert.deepEqual(wire.view({ harness: 'codex' }).models, [], 'A native harness name is never treated as a model identity.');
  const unverified = wire.view({ model: 'unverified-name' }); assert.equal(unverified.models[0].status, 'unverified'); assert.deepEqual(unverified.hierarchy, []);
  assert.ok(unverified.warnings.some(value => value.includes('unverified')));
});

test('catalog identity and evidence validation reject duplicates, foreign observations and unsupported claims', () => {
  const cases = [
    [value => value.sources.push(structuredClone(value.sources[0])), /Source IDs must be unique/],
    [value => value.models[1].aliases = ['alpha-v1'], /ambiguous/],
    [value => value.models[0].aliases = ['alpha-v1'], /ambiguous/],
    [value => value.models[0].evidenceIds = ['beta-quality'], /foreign benchmark evidence/],
    [value => value.observations[0].sourceId = 'missing', /missing source/],
    [value => value.observations[0].modelId = 'alpha-exact-alias', /exact catalog model ID/],
    [value => value.models[0].recommendations[0].task = 'unknown-task', /unknown task/],
    [value => value.models[0].recommendations[0].sourceIds = ['missing'], /missing source/],
    [value => value.models[0].recommendations[0].sourceIds = ['vendor'], /linked observation evidence/],
    [value => value.models[0].recommendations[0].basis = 'vendor-guidance', /vendor source/],
    [value => value.models[0].recommendations.push({ ...value.models[0].recommendations[0], priority: 2 }), /same task and role/],
    [value => { value.models[0].evidenceIds = []; value.models[0].recommendations = []; }, /requires benchmark evidence/],
    [value => value.models[0].referencePricing = { inputPerMillion: 1, outputPerMillion: 2, currency: 'USD', sourceId: 'missing' }, /pricing has a missing source/],
  ];
  for (const [change, expected] of cases) { const value = catalog(); change(value); assert.throws(() => validateModelMap(value), expected); }
});

test('cohorts cannot mix benchmark versions, metrics, units, score directions or harnesses and scores never become inferred precision', () => {
  for (const [field, replacement] of [['benchmark', 'Other benchmark'], ['version', 'other-version'], ['metric', 'latency'], ['unit', 'seconds'], ['direction', 'lower'], ['harness', 'Other evaluation harness']]) {
    const value = catalog(); value.observations[1][field] = replacement; assert.throws(() => validateModelMap(value), /mixes incompatible/);
  }
  for (const badPriority of [0, -1, 1.5, Infinity]) {
    const value = catalog(); value.models[0].recommendations[0].priority = badPriority; assert.throws(() => validateModelMap(value), /integer|at least|JSON-compatible/);
  }
  const value = catalog(); value.observations[0].score = NaN; assert.throws(() => validateModelMap(value), /JSON-compatible/);
  const unknownSize = catalog(); unknownSize.observations[0].sampleSize = 0; assert.throws(() => validateModelMap(unknownSize), /at least/);
});

test('tied benchmark scores keep equal ranks while effort and harness evidence stays attached to each observation', t => {
  const base = catalog(); base.observations[1].score = 95; base.observations[1].effort = null;
  const wire = fixture(t, base), quality = wire.view().rankings.find(group => group.cohort === 'independent-code-fixture');
  assert.deepEqual(quality.entries.map(entry => entry.rank), [1, 1]);
  assert.equal(quality.entries.find(entry => entry.modelId === 'beta-v1').effort, null);
  assert.equal(quality.entries[0].harness, 'harness-A');
});

test('local overlays append or fully replace IDs, retain omitted evidence and reload without changing source data', t => {
  const wire = fixture(t), original = JSON.stringify(wire.base), replacement = structuredClone(wire.base.models[1]);
  replacement.recommendations[0].priority = 4; replacement.recommendations[0].reason = 'Explicit local planning preference with its retained benchmark source.';
  wire.overlay({ sources: [source('local-method', { type: 'methodology' })], models: [replacement], tasks: [{ id: 'local-task', label: 'Local task', description: 'An explicit user-defined task category.' }] });
  const first = wire.view({ includeFullCatalog: true }); assert.equal(first.overrideApplied, true); assert.equal(first.catalog.sources.length, 4); assert.equal(first.catalog.models.length, 3);
  assert.equal(first.hierarchy.find(item => item.modelId === 'beta-v1' && item.task === 'implementation').priority, 4);
  assert.equal(first.catalog.observations.length, wire.base.observations.length); assert.equal(JSON.stringify(wire.base), original);
  wire.overlay({ models: [] }); assert.equal(wire.view().catalog.models.length, 3, 'An empty overlay array does not implicitly delete base records.');
  wire.overlay({ models: [replacement] }); replacement.recommendations[0].priority = 7; wire.overlay({ models: [replacement] });
  assert.equal(wire.view().hierarchy.find(item => item.modelId === 'beta-v1' && item.task === 'implementation').priority, 7);
});

test('overlay validation rejects ambiguous identities, partial replacement records, duplicate JSON keys and unknown fields', t => {
  const wire = fixture(t), duplicate = structuredClone(wire.base.models[1]); duplicate.aliases = ['alpha-v1'];
  wire.overlay({ models: [duplicate] }); assert.throws(() => wire.view(), /ambiguous/);
  wire.overlay({ models: [{ id: 'beta-v1', name: 'Partial replacement' }] }); assert.throws(() => wire.view(), /missing required/);
  wire.overlay('{"models":[],"models":[]}'); assert.throws(() => wire.view(), /duplicate JSON object keys/);
  wire.overlay({ deleteModels: ['alpha-v1'] }); assert.throws(() => wire.view(), /unknown property/);
  wire.overlay({ sources: [source('duplicate'), source('duplicate')] }); assert.throws(() => wire.view(), /unique/);
  wire.overlay('not-json'); assert.throws(() => wire.view(), /must contain valid JSON/);
});

test('no override initializes state, reads accounts or mutates an existing caller catalog', t => {
  const wire = fixture(t), original = structuredClone(wire.base), view = wire.view({ includeFullCatalog: true });
  assert.equal(view.overrideApplied, false); assert.equal(existsSync(wire.stateDir), false);
  assert.equal(view.bindings.some(binding => Object.hasOwn(binding, 'availability') || Object.hasOwn(binding, 'remaining')), false);
  view.catalog.models[0].strengths.push('Caller modification'); view.catalog.sources[0].title = 'Caller modification';
  assert.deepEqual(wire.base, original); assert.deepEqual(wire.view({ includeFullCatalog: true }).catalog, original);
});

test('model map refuses state paths outside the workspace even when file symbolic links are unavailable', t => {
  const wire = fixture(t);
  assert.throws(() => modelMapView({ ...wire.config, stateDir: resolve(wire.root, '..') }, {}, wire.base), /must stay in the workspace/);
  assert.equal(existsSync(wire.stateDir), false);
});

test('model map overrides refuse symlink files', t => {
  const wire = fixture(t); mkdirSync(wire.stateDir); const target = join(wire.root, 'explicit-other-file.json'); writeFileSync(target, JSON.stringify({ models: [] }));
  try { symlinkSync(target, join(wire.stateDir, 'model-map.json'), 'file'); }
  catch (error) { if (['EPERM', 'EACCES', 'ENOTSUP'].includes(error.code)) { t.skip('File symbolic links require platform support.'); return; } throw error; }
  assert.throws(() => wire.view(), /regular local JSON file/);
});

test('model map cannot read a case-distinct sibling state through a Windows case-folded root prefix', t => {
  const wire = fixture(t);
  if (process.platform === 'win32') {
    try { execFileSync(join(process.env.SystemRoot ?? 'C:\\Windows', 'System32', 'fsutil.exe'), ['file', 'setCaseSensitiveInfo', wire.root, 'enable'], { windowsHide: true, stdio: 'pipe' }); }
    catch { t.skip('Per-directory NTFS case sensitivity is unavailable.'); return; }
  }
  const root = join(wire.root, 'project'), outside = join(wire.root, 'PROJECT'); mkdirSync(root);
  if (existsSync(outside)) {
    const lower = statSync(root, { bigint: true }), upper = statSync(outside, { bigint: true });
    if (lower.dev === upper.dev && lower.ino === upper.ino) { t.skip('Case-distinct sibling directories are unavailable.'); return; }
  }
  const state = join(outside, '.council'); mkdirSync(state, { recursive: true });
  writeFileSync(join(state, 'model-map.json'), JSON.stringify({ tasks: [{ id: 'outside-task', label: 'Outside task', description: 'Must not be imported from another physical workspace.' }] }));
  const config = { ...wire.config, workspaceRoot: root, stateDir: state };
  assert.throws(() => modelMapView(config, {}, wire.base), /outside the workspace|must stay in the workspace/);
  const redirected = join(root, '.council');
  symlinkSync(state, redirected, process.platform === 'win32' ? 'junction' : 'dir');
  assert.throws(() => modelMapView({ ...config, stateDir: redirected }, {}, wire.base), /outside the workspace/);
  const contained = join(root, 'retained-evidence'), containedAlias = join(root, 'contained-alias'); mkdirSync(contained);
  writeFileSync(join(contained, 'model-map.json'), JSON.stringify({ tasks: [{ id: 'inside-task', label: 'Inside task', description: 'Explicit project evidence through a contained state alias.' }] }));
  symlinkSync(contained, containedAlias, process.platform === 'win32' ? 'junction' : 'dir');
  assert.equal(modelMapView({ ...config, stateDir: containedAlias }, {}, wire.base).catalog.tasks.at(-1).id, 'inside-task',
    'Contained physical state aliases remain supported.');
});

test('evidence URLs and dates remain explicit and invalid or embedded-credential sources fail validation', () => {
  for (const url of ['javascript:alert(1)', 'file:///private/model.json', 'https://user:password@example.test/evidence', 'not-a-url']) {
    const value = catalog(); value.sources[0].url = url; assert.throws(() => validateModelMap(value), /HTTP or HTTPS/);
  }
  for (const retrievedAt of ['2026-02-30', '2026-02-30T12:00:00Z', '2026-11-05T24:00:00Z', 'November 5, 2026']) {
    const value = catalog(); value.sources[0].retrievedAt = retrievedAt; assert.throws(() => validateModelMap(value), /ISO date|calendar date/);
  }
  const value = catalog(); value.models[0].referencePricing = { inputPerMillion: -1, outputPerMillion: 2, currency: 'USD', sourceId: 'vendor' };
  assert.throws(() => validateModelMap(value), /at least 0/);
});

test('complete long evidence and limitations survive narrow filters without context caps or fallback matching', t => {
  const base = catalog(), evidence = 'Evidence must remain complete.\n'.repeat(3000);
  base.observations[0].notes = evidence; base.observations[0].uncertainty = 'No confidence interval was published.';
  base.models[0].limitations = [evidence]; const wire = fixture(t, base), view = wire.view({ model: 'beta-v1', includeFullCatalog: true });
  assert.equal(view.catalog.observations.find(item => item.id === 'alpha-quality').notes, evidence);
  assert.equal(view.catalog.models[0].limitations[0], evidence); assert.equal(view.catalog.observations[0].uncertainty, 'No confidence interval was published.');
  assert.deepEqual(wire.view({ task: 'Implementation' }).hierarchy, []); assert.ok(wire.view({ task: 'Implementation' }).warnings.some(value => value.includes('No task category')));
});

test('large valid catalogs and evidence lists do not inherit a generic whole-JSON node cap', t => {
  const base = catalog();
  for (let index = 0; index < 1000; index++) base.observations.push(observation('historical-' + index, 'alpha-v1', index, { cohort: 'historical-cohort-' + index }));
  base.models[0].limitations = Array.from({ length: 9000 }, (_, index) => 'Explicit retained limitation ' + index);
  const wire = fixture(t, base), view = wire.view({ model: 'beta-v1', includeFullCatalog: true });
  assert.equal(view.catalog.observations.length, 1004); assert.equal(view.catalog.models[0].limitations.length, 9000);
  assert.equal(view.catalog.models[0].limitations.at(-1), 'Explicit retained limitation 8999');
});

test('local JSON reads accept a UTF-8 BOM, reject invalid bytes and bound only excessive structural nesting', t => {
  const wire = fixture(t);
  wire.overlay('\uFEFF' + JSON.stringify({ models: [] })); assert.equal(wire.view().overrideApplied, true);
  const encoded = JSON.stringify({ sources: [source('utf8-source', { title: 'INVALID_BYTE_HERE' })] });
  const [before, after] = encoded.split('INVALID_BYTE_HERE');
  writeFileSync(join(wire.stateDir, 'model-map.json'), Buffer.concat([Buffer.from(before), Buffer.from([0xff]), Buffer.from(after)]));
  assert.throws(() => wire.view(), /valid UTF-8/);
  wire.overlay('{"models":' + '['.repeat(65) + ']'.repeat(65) + '}');
  assert.throws(() => wire.view(), /JSON nesting safety depth/);
});

test('the shipped map keeps recorded configurations, namespaces, uncertain evidence and missing models distinct', t => {
  const validated = validateModelMap(MODEL_MAP_CATALOG), wire = fixture(t, validated), view = wire.view();
  assert.ok(validated.models.length >= 20); assert.ok(validated.sources.length > 30); assert.ok(validated.observations.length > 50);
  const modelIds = new Set(validated.models.map(model => model.id));
  assert.ok(modelIds.has('kimi-k3.1-pro')); assert.equal(modelIds.has('k3.1-pro'), false, 'Kimi Code namespace does not inherit public API measurements.');
  assert.ok(modelIds.has('qwen3.8-max-0902')); assert.ok(modelIds.has('Qwen/Qwen3.8-2.4T-A95B'));
  assert.equal(view.bindings.find(binding => binding.alias === 'native').match, 'unresolved');
  for (const id of ['Gemini 3.8', 'gpt-6.1-luna', 'gpt-6.1-sol-latest']) {
    assert.deepEqual(wire.view({ model: id }).models, []); assert.ok(wire.view({ model: id }).warnings.some(warning => warning.includes('No substitute')));
  }
  const owner = view.rankings.find(group => group.cohort === 'tb4-codex-owner-2026-11-05');
  const maxima = owner.entries.filter(entry => entry.score === 58.18);
  assert.equal(maxima.length, 2); assert.equal(maxima[0].rank, maxima[1].rank);
  assert.ok(validated.observations.filter(observation => observation.cohort.startsWith('tb4-')).every(observation => observation.uncertainty));
  assert.ok(validated.models.every(model => !model.harnesses?.length), 'Published benchmark harnesses do not establish local wrapper compatibility.');
  assert.ok(validated.models.flatMap(model => model.recommendations).every(recommendation => recommendation.sourceIds.length));
});
