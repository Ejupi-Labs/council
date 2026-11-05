import test from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
import { CouncilService } from '../dist/service.js';
import { loadConfig } from '../dist/config.js';
import { serveDashboard } from '../dist/dashboard.js';
import { collaborationSkills } from '../dist/skills.js';

const cli = fileURLToPath(new URL('../dist/cli.js', import.meta.url));
const economy = 'dont-kill-an-ant-with-a-bazooka';
const evidenceSkills = ['council-evidence-researcher', 'council-clear-explanations', 'council-lazy-genius'];
function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-economy-integration-')), cleanup = [], calls = [];
  const agents = { tiny: { profile: 'codex', model: 'small-fixture' }, power: { profile: 'claude', model: 'large-fixture' } };
  // Child CLI/MCP hosts use the same explicit aliases, without any vendor executable.
  const service = new CouncilService(loadConfig(root, { backend: 'mock', agents }), { doctor: async () => [], run: async request => {
    calls.push(request); return { agent: request.agent, status: 'completed', output: 'OFFLINE_ECONOMY_RESULT', durationMs: 1 };
  } });
  t.after(async () => { for (const close of cleanup.toReversed()) await close(); await service.close(); rmSync(root, { recursive: true, force: true }); });
  service.billing.update('tiny', { mode: 'subscription', plan: 'Fixture subscription', skills: [economy],
    routing: { tier: 'small', capabilities: ['code'], costRank: 1, qualityRank: 1 } });
  service.billing.update('power', { mode: 'api', routing: { tier: 'large', capabilities: ['code'], costRank: 9, qualityRank: 9 } });
  return { root, cleanup, service, calls, agents };
}
const request = { task: 'Correct a small, understood typo and verify the result.', taskClass: 'routine', requiredCapabilities: ['code'] };

test('right-sized service execution preserves exact models, assigned skills and durable routing/billing evidence', async t => {
  const { service, calls } = fixture(t);
  const preview = service.routePlan(request);
  assert.deepEqual(preview.selectedAliases, ['tiny']); assert.equal(preview.estimatedCalls, 1); assert.equal(calls.length, 0);
  const { run } = service.routeRun(request);
  service.billing.update('tiny', { mode: 'api', skills: [] });
  const result = await service.completion(run.id);
  assert.equal(result.status, 'completed'); assert.equal(calls.length, 1);
  assert.equal(calls[0].agent, 'tiny'); assert.equal(calls[0].model, 'small-fixture'); assert.match(calls[0].prompt, /Keep the work proportional/);
  const events = service.store.events(run.id);
  assert.deepEqual(events.find(event => event.type === 'run.routed').data.selectedAliases, ['tiny']);
  assert.equal(events.find(event => event.type === 'run.billing').data.agents[0].mode, 'subscription');
  assert.equal(service.billing.get('tiny').mode, 'api');
});

test('routing rechecks current exhaustion before starting and never silently upgrades a routine task', t => {
  const { service, calls } = fixture(t);
  assert.deepEqual(service.routePlan(request).selectedAliases, ['tiny']);
  const now = new Date().toISOString(), later = new Date(Date.now() + 3_600_000).toISOString();
  service.billing.importSnapshot('tiny', { source: { kind: 'reported', label: 'Offline fixture report' }, observedAt: now, validUntil: later,
    windows: [{ id: 'quota', label: 'Shared allowance', unit: 'requests', remaining: 0, resetAt: later }] });
  assert.throws(() => service.routeRun(request), /escalat|suitable|eligible/i);
  assert.equal(calls.length, 0);
  const escalated = service.routePlan({ ...request, allowEscalation: true });
  assert.deepEqual(escalated.selectedAliases, ['power']); assert.equal(escalated.escalation.allowed, true);
});

test('authenticated billing and routing HTTP endpoints expose unknown data and reject stale edits without dispatch', async t => {
  const { service, cleanup, calls } = fixture(t), dashboard = await serveDashboard(service, service.rooms);
  cleanup.push(() => dashboard.close());
  const url = new URL(dashboard.url), token = new URLSearchParams(url.hash.slice(1)).get('token');
  const api = (path, body) => fetch(url.origin + path, { method: body === undefined ? 'GET' : 'POST', headers: { Authorization: `Bearer ${token}`, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
  const overview = await (await api('/api/billing')).json();
  const tiny = overview.agents.find(agent => agent.alias === 'tiny');
  assert.equal(tiny.mode, 'subscription'); assert.equal(tiny.availability.status, 'unknown'); assert.equal(tiny.windows.length, 0);
  assert.deepEqual(tiny.effectiveSkills, [economy]); assert.deepEqual(overview.skills, collaborationSkills());
  const saved = await api('/api/billing/tiny', { expectedRevision: tiny.revision, plan: 'Renamed plan', skills: evidenceSkills }); assert.equal(saved.status, 200);
  const conflict = await api('/api/billing/tiny', { expectedRevision: tiny.revision, plan: 'Obsolete edit' }); assert.equal(conflict.status, 409);
  assert.equal(service.billing.get('tiny').plan, 'Renamed plan');
  assert.deepEqual(service.billing.get('tiny').skills, evidenceSkills);
  const refreshed = await (await api('/api/billing')).json();
  assert.deepEqual(refreshed.agents.find(agent => agent.alias === 'tiny').effectiveSkills, evidenceSkills);
  const plan = await (await api('/api/routing/plan', request)).json(); assert.deepEqual(plan.selectedAliases, ['tiny']);
  assert.equal(Object.hasOwn(plan.workflow.agentSettings.tiny, 'skills'), false, 'An inherited assignment is resolved at dispatch rather than pinned by the preview.');
  const explicit = await (await api('/api/routing/plan', { ...request, skills: evidenceSkills })).json();
  assert.deepEqual(explicit.workflow.agentSettings.tiny.skills, evidenceSkills);
  assert.equal(explicit.workflow.agentSettings.tiny.model, 'small-fixture');
  assert.equal(calls.length, 0);
});

test('CLI billing and routing commands use local state and preserve explicit choices', t => {
  const { root } = fixture(t);
  writeFileSync(join(root, 'council.config.json'), JSON.stringify({ backend: 'mock', agents: { tiny: { profile: 'codex', model: 'small-fixture' }, power: { profile: 'claude', model: 'large-fixture' } } }));
  const invoke = args => {
    const result = spawnSync(process.execPath, [cli, ...args, '--workspace', root, '--backend', 'mock'], { encoding: 'utf8', windowsHide: true, shell: false, timeout: 15000, env: { ...process.env, COUNCIL_DEPTH: '0' } });
    assert.ifError(result.error); assert.equal(result.status, 0, result.stderr); return JSON.parse(result.stdout);
  };
  assert.equal(invoke(['billing', '--agent', 'tiny']).mode, 'subscription');
  const planned = invoke(['route-plan', '--task', request.task, '--task-class', 'routine', '--requires', 'code']);
  assert.deepEqual(planned.selectedAliases, ['tiny']);
  const assigned = invoke(['route-plan', '--task', request.task, '--task-class', 'routine', '--agents', 'tiny', '--skills', evidenceSkills.join(',')]);
  assert.deepEqual(assigned.workflow.agentSettings.tiny.skills, evidenceSkills);
  assert.equal(assigned.workflow.agentSettings.tiny.model, 'small-fixture');
  const result = invoke(['route', '--task', request.task, '--task-class', 'routine', '--agents', 'power', '--model', 'explicit-expert-fixture']);
  assert.equal(result.run.status, 'completed'); assert.deepEqual(result.selection.selectedAliases, ['power']);
  assert.equal(result.run.input.agentSettings.power.model, 'explicit-expert-fixture');
});

test('official MCP client can inspect billing, persist evidence skills and run a selected model without hidden extra calls', async t => {
  const { root, cleanup, service } = fixture(t);
  writeFileSync(join(root, 'council.config.json'), JSON.stringify({ backend: 'mock', agents: { tiny: { profile: 'codex', model: 'small-fixture' }, power: { profile: 'claude', model: 'large-fixture' } } }));
  const transport = new StdioClientTransport({ command: process.execPath, args: [cli, 'mcp', '--workspace', root, '--backend', 'mock'], cwd: root, stderr: 'pipe', env: { ...process.env, COUNCIL_DEPTH: '0' } });
  transport.stderr?.on('data', () => {});
  const client = new Client({ name: 'billing-economy-integration', version: '1' }); cleanup.push(() => client.close()); await client.connect(transport);
  const call = async (name, args = {}) => {
    const response = await client.callTool({ name, arguments: args }); assert.notEqual(response.isError, true, JSON.stringify(response.content));
    return JSON.parse(response.content.find(item => item.type === 'text').text);
  };
  assert.deepEqual((await call('council_skills')).skills, collaborationSkills());
  const before = await call('council_billing', { agent: 'tiny' }); assert.equal(before.mode, 'subscription');
  await call('council_billing_update', { agent: 'tiny', settings: { skills: evidenceSkills }, expectedRevision: before.revision });
  assert.deepEqual((await call('council_billing', { agent: 'tiny' })).skills, evidenceSkills);
  const preview = await call('council_route_plan', request); assert.deepEqual(preview.selectedAliases, ['tiny']);
  assert.equal(preview.estimatedCalls, 1);
  assert.equal(Object.hasOwn(preview.workflow.agentSettings.tiny, 'skills'), false, 'Preview preserves inheritance until workflow admission.');
  const explicit = await call('council_route_plan', { ...request, skills: evidenceSkills });
  assert.deepEqual(explicit.workflow.agentSettings.tiny.skills, evidenceSkills);
  assert.equal(explicit.workflow.agentSettings.tiny.model, 'small-fixture');
  const queued = await call('council_route_run', request);
  const waited = await call('council_wait', { ids: [queued.run.id], timeoutMs: 5000 });
  assert.equal(waited.runs[0].status, 'completed'); assert.equal(waited.runs[0].steps.length, 1);
  const executed = service.getRun(queued.run.id);
  assert.deepEqual(executed.input.agentSettings.tiny.skills, evidenceSkills);
  assert.equal(executed.input.agentSettings.tiny.model, 'small-fixture');
});
