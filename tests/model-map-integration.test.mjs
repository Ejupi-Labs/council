import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { PassThrough } from 'node:stream';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';
import { CouncilService, modelMapView, modelMapQueryDefinition, modelMapOverlayDefinition } from '../dist/index.js';
import { loadConfig } from '../dist/config.js';
import { createMcpServer } from '../dist/mcp.js';
import { serveDashboard } from '../dist/dashboard.js';

const execute = promisify(execFile), cli = new URL('../dist/cli.js', import.meta.url);
function workspace(t, beforeCleanup = async () => {}) {
  const root = mkdtempSync(join(tmpdir(), 'council-model-map-integration-'));
  t.after(async () => { await beforeCleanup(); assert.equal(dirname(root), resolve(tmpdir())); assert.ok(basename(root).startsWith('council-model-map-integration-')); rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); });
  return root;
}
function fixture(t) {
  let service;
  const root = workspace(t, () => service?.close()), calls = [];
  service = new CouncilService(loadConfig(root, { backend: 'mock', agents: { exact: { enabled: true, profile: 'codex', model: 'gpt-6.1-sol', effort: 'high', authority: 7 }, native: { enabled: true, profile: 'claude' } } }),
    { doctor: async () => { assert.fail('A model map must not inspect provider runtimes.'); }, run: async request => { calls.push(request); assert.fail('A model map must never dispatch a provider.'); } });
  return { root, service, calls };
}
async function mcp(t, service) {
  const input = new PassThrough(), output = new PassThrough(), server = createMcpServer(service);
  await server.connect(input, output);
  let sequence = 0, buffer = ''; const pending = new Map();
  output.on('data', chunk => {
    buffer += chunk.toString('utf8');
    for (;;) { const end = buffer.indexOf('\n'); if (end < 0) break; const frame = JSON.parse(buffer.slice(0, end)); buffer = buffer.slice(end + 1);
      if (pending.has(frame.id)) { const done = pending.get(frame.id); pending.delete(frame.id); done(frame); } }
  });
  const request = (method, params) => { const id = ++sequence, result = new Promise(done => pending.set(id, done)); input.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n'); return result; };
  t.after(async () => { await server.close(); input.destroy(); output.destroy(); });
  const initial = await request('initialize', { protocolVersion: '2025-11-25', capabilities: {}, clientInfo: { name: 'model-map-integration', version: '1' } });
  assert.equal(initial.error, undefined);
  return { request, async call(args) { const frame = await request('tools/call', { name: 'council_model_map', arguments: args }); assert.equal(frame.error, undefined); return frame.result; } };
}
function selected(view) {
  const model = view.catalog.models.find(model => model.status !== 'unverified' && model.recommendations.length);
  assert.ok(model); const recommendation = model.recommendations[0];
  return { model: model.id, task: recommendation.task, role: recommendation.role };
}
function cliCall(root, args) {
  return execute(process.execPath, [fileURLToPath(cli), 'model-map', ...args, '--workspace', root],
    { windowsHide: true, timeout: 15000, maxBuffer: 8 * 1024 * 1024, env: { ...process.env, COUNCIL_BACKEND: 'mock', COUNCIL_DEPTH: '0' } });
}

test('service and public exports provide exact filtered model evidence without mutating agent settings, billing or tasks', async t => {
  const { service, calls } = fixture(t), before = structuredClone(service.config.agents), initial = service.modelMap({ includeFullCatalog: true }), query = selected(initial);
  const filtered = service.modelMap(query);
  assert.deepEqual(filtered.query, query); assert.deepEqual(filtered.models.map(model => model.id), [query.model]);
  assert.deepEqual(filtered.catalog.models.map(model => model.id), [query.model]);
  assert.equal(filtered.catalogScope, 'matching-models'); assert.equal(filtered.totalModels, initial.catalog.models.length);
  assert.deepEqual(filtered.catalog.observations, initial.catalog.observations.filter(observation => observation.modelId === query.model), 'Filtering retains every observation for the selected model.');
  const sourceIds = new Set(filtered.catalog.sources.map(source => source.id)), profile = filtered.models[0];
  for (const source of initial.catalog.sources.filter(source => source.type === 'methodology')) assert.ok(sourceIds.has(source.id), 'Methodology remains available.');
  for (const observation of filtered.catalog.observations) assert.ok(sourceIds.has(observation.sourceId));
  for (const recommendation of profile.recommendations) for (const sourceId of recommendation.sourceIds) assert.ok(sourceIds.has(sourceId));
  if (profile.referencePricing) assert.ok(sourceIds.has(profile.referencePricing.sourceId));
  for (const evidenceId of profile.evidenceIds) assert.ok(filtered.catalog.observations.some(observation => observation.id === evidenceId));
  assert.deepEqual(filtered.catalog.tasks, initial.catalog.tasks);
  const full = service.modelMap({ ...query, includeFullCatalog: true });
  assert.equal(full.catalogScope, 'full'); assert.deepEqual(full.catalog, initial.catalog); assert.deepEqual(full.models, filtered.models);
  assert.ok(filtered.hierarchy.length); assert.ok(filtered.hierarchy.every(row => row.modelId === query.model && row.task === query.task && row.role === query.role));
  assert.equal(filtered.bindings.find(binding => binding.alias === 'exact').catalogModelId, 'gpt-6.1-sol');
  assert.equal(filtered.bindings.find(binding => binding.alias === 'native').match, 'unresolved');
  assert.deepEqual(service.config.agents, before); assert.equal(existsSync(join(service.config.stateDir, 'billing.sqlite')), false);
  assert.deepEqual(service.store.listRuns(), []); assert.deepEqual(calls, []);
  assert.equal(modelMapView(service.config, query).models[0].id, query.model);
});

test('MCP advertises a read-only strict model-map tool, preserves relevant evidence and explicitly opts into the full catalog', async t => {
  const { service, calls } = fixture(t), wire = await mcp(t, service);
  const listing = await wire.request('tools/list', {}), tool = listing.result.tools.find(tool => tool.name === 'council_model_map');
  assert.ok(tool); assert.equal(tool.annotations.readOnlyHint, true); assert.equal(tool.annotations.destructiveHint, false); assert.equal(tool.annotations.openWorldHint, false);
  assert.equal(tool.inputSchema.additionalProperties, false);
  const result = await wire.call({ model: 'not-an-invented-model' }); assert.notEqual(result.isError, true);
  const view = JSON.parse(result.content[0].text); assert.deepEqual(view.models, []); assert.ok(view.warnings.some(warning => warning.includes('No substitute was selected')));
  assert.ok(view.catalog.sources.every(source => source.type === 'methodology')); assert.deepEqual(view.catalog.observations, []);
  const initial = service.modelMap({ includeFullCatalog: true }), query = selected(initial), full = JSON.parse((await wire.call({ ...query, includeFullCatalog: true })).content[0].text);
  assert.deepEqual(full.query, { ...query, includeFullCatalog: true }); assert.deepEqual(full.catalog, initial.catalog);
  assert.deepEqual(full.models.map(model => model.id), [query.model]); assert.equal(full.catalogScope, 'full');
  const matching = JSON.parse((await wire.call({ ...query, includeFullCatalog: false })).content[0].text);
  assert.deepEqual(matching.query, { ...query, includeFullCatalog: false }); assert.equal(matching.catalogScope, 'matching-models');
  assert.deepEqual(matching.catalog.models.map(model => model.id), [query.model]);
  for (const query of [{ role: 'administrator' }, { task: '   ' }, { execute: true }, { includeFullCatalog: 'true' }, { includeFullCatalog: 0 }, { includeFullCatalog: null }]) {
    const rejected = await wire.call(query); assert.equal(rejected.isError, true, JSON.stringify(query));
  }
  assert.deepEqual(calls, []); assert.deepEqual(service.store.listRuns(), []);
});

test('authenticated HTTP model map rejects mutations, duplicate and invalid filters while preserving exact query semantics', async t => {
  const { service, calls } = fixture(t), dashboard = await serveDashboard(service, service.rooms); t.after(() => dashboard.close());
  const address = new URL(dashboard.url), token = new URLSearchParams(address.hash.slice(1)).get('token');
  const http = (query = '', options = {}) => fetch(address.origin + '/api/model-map' + query, { ...options, headers: { Authorization: 'Bearer ' + token, ...options.headers } });
  assert.equal((await fetch(address.origin + '/api/model-map')).status, 401);
  assert.equal((await http('', { headers: { Origin: 'https://example.com' } })).status, 403);
  const query = selected(service.modelMap()), response = await http('?' + new URLSearchParams(query)); assert.equal(response.status, 200);
  const view = await response.json(); assert.deepEqual(view.query, query); assert.deepEqual(view.models.map(model => model.id), [query.model]);
  assert.deepEqual(view.catalog.models.map(model => model.id), [query.model]);
  for (const fullCatalog of ['false', 'true']) {
    const response = await http('?' + new URLSearchParams({ ...query, includeFullCatalog: fullCatalog })); assert.equal(response.status, 200);
    const result = await response.json(); assert.deepEqual(result.query, { ...query, includeFullCatalog: fullCatalog === 'true' });
    assert.deepEqual(result.models.map(model => model.id), [query.model]);
    assert.equal(result.catalog.models.length, fullCatalog === 'true' ? service.modelMap().catalog.models.length : 1);
  }
  for (const query of ['?model=x&model=y', '?task=x&task=y', '?role=lead&role=lead', '?harness=codex&harness=codex', '?includeFullCatalog=true&includeFullCatalog=false', '?unexpected=1', '?role=Lead', '?task=', '?model=%20%20', '?harness=%00', '?includeFullCatalog=', '?includeFullCatalog=1', '?includeFullCatalog=0', '?includeFullCatalog=True', '?includeFullCatalog=FALSE', '?includeFullCatalog=%20true', '?includeFullCatalog=null']) {
    assert.equal((await http(query)).status, 400, query);
  }
  assert.equal((await http('', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })).status, 405);
  assert.equal((await http('', { method: 'DELETE' })).status, 405);
  const unknown = await http('?harness=unverified-harness'); assert.equal(unknown.status, 200);
  const missing = await unknown.json(); assert.deepEqual(missing.models, []); assert.ok(missing.warnings.some(warning => warning.includes('No catalog profile declares harness')));
  assert.deepEqual(calls, []); assert.deepEqual(service.store.listRuns(), []);
});

test('service, MCP and HTTP read updated validated local overrides without treating them as provider or voting changes', async t => {
  const { service, root } = fixture(t), initial = service.modelMap(), before = structuredClone(service.config.agents), wire = await mcp(t, service);
  const dashboard = await serveDashboard(service, service.rooms); t.after(() => dashboard.close());
  const address = new URL(dashboard.url), token = new URLSearchParams(address.hash.slice(1)).get('token');
  const source = initial.catalog.sources[0], path = join(service.config.stateDir, 'model-map.json');
  writeFileSync(path, JSON.stringify({ sources: [{ ...source, title: 'Explicit local source revision A' }] }));
  assert.equal(service.modelMap().overrideApplied, true);
  const one = JSON.parse((await wire.call({})).content[0].text); assert.equal(one.catalog.sources.find(value => value.id === source.id).title, 'Explicit local source revision A');
  writeFileSync(path, JSON.stringify({ sources: [{ ...source, title: 'Explicit local source revision B' }] }));
  const response = await fetch(address.origin + '/api/model-map', { headers: { Authorization: 'Bearer ' + token } });
  assert.equal(response.status, 200); const two = await response.json(); assert.equal(two.catalog.sources.find(value => value.id === source.id).title, 'Explicit local source revision B');
  writeFileSync(path, '{"sources":[],"sources":[]}');
  assert.equal((await fetch(address.origin + '/api/model-map', { headers: { Authorization: 'Bearer ' + token } })).status, 400);
  assert.deepEqual(service.config.agents, before); assert.deepEqual(service.store.listRuns(), []); assert.ok(path.startsWith(root));
});

test('CLI model-map uses the offline reference path and never creates a workflow database or registered host', async t => {
  const root = workspace(t), state = join(root, '.council');
  const initial = JSON.parse((await cliCall(root, ['--full-catalog'])).stdout), query = selected(initial);
  const response = await cliCall(root, ['--task', query.task, '--model', query.model, '--role', query.role]);
  const view = JSON.parse(response.stdout); assert.deepEqual(view.query, query); assert.deepEqual(view.models.map(model => model.id), [query.model]);
  assert.deepEqual(view.catalog.models.map(model => model.id), [query.model]); assert.equal(view.catalogScope, 'matching-models');
  const full = JSON.parse((await cliCall(root, ['--task', query.task, '--model', query.model, '--role', query.role, '--full-catalog'])).stdout);
  assert.deepEqual(full.query, { ...query, includeFullCatalog: true }); assert.deepEqual(full.catalog, initial.catalog);
  assert.deepEqual(full.models.map(model => model.id), [query.model]); assert.equal(full.catalogScope, 'full');
  assert.equal(response.stderr, ''); assert.equal(existsSync(state), false);
  for (const args of [['--write'], ['--task-file', 'not-read.txt'], ['--role', 'Administrator'], ['--model', ''], ['--harness', '   '], ['--full-catalog=false'], ['--full-catalog=1']]) {
    await assert.rejects(cliCall(root, args), error => { assert.equal(error.stdout, ''); assert.match(error.stderr, /Council:/); assert.ok(!error.stderr.includes('run.started')); return true; });
  }
  assert.equal(existsSync(state), false);
});

test('generated public schemas include strict model-map query, catalog and local override contracts', () => {
  const schema = name => JSON.parse(readFileSync(new URL('../schemas/' + name + '.schema.json', import.meta.url), 'utf8'));
  const query = schema('model-map-query'), override = schema('model-map-override'), catalog = schema('model-map');
  assert.equal(query.additionalProperties, false); assert.deepEqual(query.properties, modelMapQueryDefinition.properties);
  assert.equal(override.additionalProperties, false); assert.deepEqual(override.properties, modelMapOverlayDefinition.properties);
  assert.deepEqual(Object.keys(query.properties).sort(), ['harness', 'includeFullCatalog', 'model', 'role', 'task']);
  assert.equal(query.properties.includeFullCatalog.type, 'boolean');
  assert.ok(catalog.required.includes('observations')); assert.ok(catalog.required.includes('sources'));
});
