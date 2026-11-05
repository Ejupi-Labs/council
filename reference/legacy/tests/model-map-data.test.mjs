import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { MODEL_MAP_CATALOG, createModelMapCohort } from '../dist/model-map-data.js';
import { modelMapView, validateModelMap } from '../dist/model-map.js';

const observations = (rows, cohort = 'dated-owner-capture') => createModelMapCohort(cohort, 'owner', 'Scoped code benchmark', '2026-11',
  'Resolved', '%', 'Fixed evaluation harness', rows, 'Evaluation evidence, not a universal capability score.');
const identity = observation => JSON.stringify([observation.cohort, observation.modelId, observation.effort]);
function catalog(records) {
  return { asOf: '2026-11-05', revision: 1,
    sources: [{ id: 'owner', title: 'Benchmark owner', url: 'https://benchmark.example.test/results', retrievedAt: '2026-11-05', publisher: 'Owner', type: 'benchmark-owner' }],
    tasks: [], observations: records,
    models: [...new Set(records.map(record => record.modelId))].map(id => ({ id, name: id, provider: 'Fixture provider', status: 'verified',
      strengths: [], limitations: [], recommendations: [], evidenceIds: records.filter(record => record.modelId === id).map(record => record.id) })) };
}

test('curated observation IDs survive inserted and reordered rows and corrected scores', () => {
  const before = observations([['alpha-v1', 90, 'high'], ['beta-v1', 80, 'medium'], ['alpha-v1', 70, null]]);
  const after = observations([['alpha-v1', 35, 'low'], ['alpha-v1', 71, null], ['beta-v1', 82, 'medium'], ['alpha-v1', 91, 'high']]);
  const refreshed = new Map(after.map(record => [identity(record), record]));
  for (const original of before) {
    const current = refreshed.get(identity(original));
    assert.ok(current); assert.equal(current.id, original.id); assert.notEqual(current.score, original.score);
  }
  assert.equal(new Set(after.map(record => record.id)).size, after.length);
});

test('observation identity preserves literal components and separates unreported from reported effort', () => {
  const effortCases = observations([['alpha-v1', 1, null], ['alpha-v1', 2, 'unreported'], ['alpha-v1', 3, 'null'], ['alpha-v1', 4, 'reported']]);
  assert.equal(new Set(effortCases.map(record => record.id)).size, effortCases.length);
  const boundaryCases = [observations([['b:c', 1, 'high']], 'a'), observations([['c', 1, 'high']], 'a:b'),
    observations([['Case', 1, 'high']], 'a'), observations([['case', 1, 'high']], 'a'),
    observations([['é', 1, 'high']], 'a'), observations([['e\u0301', 1, 'high']], 'a')].flat();
  assert.equal(new Set(boundaryCases.map(record => record.id)).size, boundaryCases.length, 'No separator, case or Unicode normalization aliases are introduced.');
});

test('an older full-record observation override updates only its intended model and effort after a catalogue refresh', t => {
  const parent = realpathSync.native(tmpdir()), root = realpathSync.native(mkdtempSync(join(parent, 'council-model-map-data-'))), stateDir = join(root, '.council');
  t.after(() => { assert.equal(dirname(resolve(root)), parent); assert.ok(basename(root).startsWith('council-model-map-data-')); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  const original = observations([['alpha-v1', 90, 'high'], ['beta-v1', 80, 'medium']]);
  const refreshed = observations([['alpha-v1', 30, 'low'], ['beta-v1', 82, 'medium'], ['alpha-v1', 91, 'high']]);
  const override = { ...original[0], score: 95, notes: 'Explicit local correction of alpha high.' };
  mkdirSync(stateDir); writeFileSync(join(stateDir, 'model-map.json'), JSON.stringify({ observations: [override] }));
  const base = catalog(refreshed), config = { workspaceRoot: root, stateDir, agents: {} };
  const view = modelMapView(config, {}, base);
  assert.equal(view.overrideApplied, true); assert.equal(view.catalog.observations.length, 3);
  const actual = new Map(view.catalog.observations.map(record => [identity(record), record]));
  assert.deepEqual(actual.get(identity(override)), override);
  for (const current of refreshed.filter(record => record.effort !== 'high')) assert.deepEqual(actual.get(identity(current)), current, 'Inserted and independently updated evidence must survive an old override.');
  assert.equal(base.observations.find(record => record.effort === 'high').score, 91, 'The refreshed base remains unchanged.');
});

test('every shipped cohort retains its observation identities when regenerated in reverse order', () => {
  validateModelMap(MODEL_MAP_CATALOG);
  const groups = Map.groupBy(MODEL_MAP_CATALOG.observations, record => record.cohort);
  for (const records of groups.values()) {
    const first = records[0];
    const reordered = createModelMapCohort(first.cohort, first.sourceId, first.benchmark, first.version, first.metric, first.unit,
      first.harness, records.toReversed().map(record => [record.modelId, record.score + 1, record.effort, record.uncertainty]), first.notes ?? 'Recorded evidence');
    assert.deepEqual(new Set(reordered.map(record => record.id)), new Set(records.map(record => record.id)), first.cohort);
  }
});
