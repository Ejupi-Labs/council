import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { PassThrough } from 'node:stream';
import { CouncilService } from '../dist/service.js';
import { BillingManager } from '../dist/billing.js';
import { loadConfig } from '../dist/config.js';
import { createMcpServer } from '../dist/mcp.js';
import { serveDashboard } from '../dist/dashboard.js';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'council-billing-response-')), calls = [];
  const config = loadConfig(root, { backend: 'mock', agents: { worker: { enabled: true, profile: 'codex' } } });
  const service = new CouncilService(config, { doctor: async () => [], run: async request => {
    calls.push(request); throw new Error('Billing reads must not invoke a model.');
  } });
  const writer = new BillingManager(config), resources = [];
  const observedAt = new Date(Date.now() - 1000).toISOString(), validUntil = new Date(Date.now() + 3600000).toISOString();
  const observation = remaining => ({ mode: 'subscription', source: { kind: 'reported', label: 'Offline observation ' + remaining }, observedAt, validUntil,
    windows: [{ id: 'allowance', label: 'Shared allowance', unit: 'requests', remaining }] });
  service.billing.importSnapshot('worker', observation(5));
  t.after(async () => {
    for (const resource of resources.toReversed()) await resource.close();
    writer.close(); await service.close();
    assert.equal(dirname(resolve(root)), resolve(tmpdir())); assert.ok(basename(root).startsWith('council-billing-response-'));
    rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  });
  return { service, writer, observation, calls, resources };
}

async function mcp(wire) {
  const input = new PassThrough(), output = new PassThrough(), pending = new Map();
  let sequence = 0, buffer = '';
  output.on('data', chunk => {
    buffer += chunk.toString('utf8');
    for (;;) {
      const end = buffer.indexOf('\n'); if (end < 0) break;
      const frame = JSON.parse(buffer.slice(0, end)); buffer = buffer.slice(end + 1);
      const completion = pending.get(frame.id);
      if (completion) { pending.delete(frame.id); completion(frame); }
    }
  });
  const server = createMcpServer(wire.service); await server.connect(input, output);
  wire.resources.push({ close: async () => { await server.close(); input.destroy(); output.destroy(); } });
  const request = (method, params) => new Promise(resolveFrame => {
    const id = ++sequence; pending.set(id, resolveFrame);
    input.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
  });
  const initialized = await request('initialize', { protocolVersion: '2025-11-25', capabilities: {}, clientInfo: { name: 'billing-response-test', version: '1' } });
  assert.equal(initialized.error, undefined);
  return async (name, args) => {
    const frame = await request('tools/call', { name, arguments: args });
    assert.equal(frame.error, undefined); assert.notEqual(frame.result.isError, true, frame.result.content[0].text);
    return JSON.parse(frame.result.content[0].text);
  };
}

function assertCaptured(record) {
  assert.equal(record.windows[0].remaining, 5);
  assert.equal(record.source.label, 'Offline observation 5');
  assert.equal(record.availability.status, 'available');
  assert.equal(record.availability.windows[0].effectiveRemaining, 5);
}

test('single-agent MCP billing pairs availability with its returned observation under a concurrent writer', { timeout: 15000 }, async t => {
  const wire = fixture(t), call = await mcp(wire), original = wire.service.billing.get.bind(wire.service.billing);
  let injected = false;
  wire.service.billing.get = alias => {
    const captured = original(alias);
    if (!injected) { injected = true; wire.writer.importSnapshot('worker', wire.observation(0)); }
    return captured;
  };
  try { assertCaptured(await call('council_billing', { agent: 'worker' })); }
  finally { wire.service.billing.get = original; }
  const latest = await call('council_billing', { agent: 'worker' });
  assert.equal(latest.windows[0].remaining, 0); assert.equal(latest.availability.status, 'exhausted');
  assert.equal(injected, true); assert.equal(wire.calls.length, 0);
});

test('HTTP billing save pairs availability with the saved revision instead of a later account update', { timeout: 15000 }, async t => {
  const wire = fixture(t), dashboard = await serveDashboard(wire.service, wire.service.rooms); wire.resources.push(dashboard);
  const url = new URL(dashboard.url), token = new URLSearchParams(url.hash.slice(1)).get('token');
  const revision = wire.service.billing.get('worker').revision, original = wire.service.billing.update.bind(wire.service.billing);
  wire.service.billing.update = (...args) => {
    const saved = original(...args); wire.writer.importSnapshot('worker', wire.observation(0)); return saved;
  };
  let response;
  try {
    response = await fetch(url.origin + '/api/billing/worker', { method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' },
      body: JSON.stringify({ expectedRevision: revision, routing: { tier: 'small', capabilities: ['read'], costRank: 1, qualityRank: 1 } }) });
  } finally { wire.service.billing.update = original; }
  assert.equal(response.status, 200); const saved = await response.json(); assertCaptured(saved);
  assert.equal(saved.revision, revision + 1); assert.equal(saved.routing.tier, 'small');
  const latest = wire.service.billing.get('worker');
  assert.equal(latest.windows[0].remaining, 0); assert.equal(latest.revision, saved.revision + 1);
  assert.equal(wire.service.billing.availabilityFor(latest).status, 'exhausted'); assert.equal(wire.calls.length, 0);
});
